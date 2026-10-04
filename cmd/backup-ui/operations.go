package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const lockDirectory = "/tmp/mongodb-backup.lock"

type config struct {
	directory, uri, schedule, retention string
	lockPath                            string
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func configFromEnv() config {
	retention := os.Getenv("BACKUP_RETENTION_POLICY")
	if retention == "" {
		retention = envOr("BACKUP_RETENTION_DAYS", "7") + " days"
	}
	return config{
		directory: envOr("BACKUP_DIRECTORY", "/backups"),
		uri:       os.Getenv("MONGODB_URI"),
		schedule:  envOr("BACKUP_CRON_SCHEDULE", "0 2 * * *") + " (UTC)",
		retention: retention,
		lockPath:  lockDirectory,
	}
}

// Only the host and database are shown; credentials and URI query parameters
// stay out of the terminal interface.
func (c config) destination() string {
	u, err := url.Parse(c.uri)
	if err != nil || u.Host == "" {
		return "MONGODB_URI not configured"
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		database = "all databases"
	}
	return u.Host + " / " + database
}

type archive struct {
	path, name string
	size       int64
	modified   time.Time
}

// Renaming or copying a backup must not make its database snapshot look newer.
// External archives without our timestamp format fall back to their file time.
func (a archive) backupTime() (time.Time, bool) {
	const prefix = "mongodb-"
	const layout = "2006-01-02T15-04-05Z"
	if strings.HasPrefix(a.name, prefix) && strings.HasSuffix(a.name, ".archive.gz") && len(a.name) >= len(prefix)+len(layout) {
		if captured, err := time.Parse(layout, a.name[len(prefix):len(prefix)+len(layout)]); err == nil {
			return captured, true
		}
	}
	return a.modified, false
}

func readArchives(directory string) ([]archive, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	archives := make([]archive, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".archive.gz") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue // A scheduled prune may have removed it.
		}
		if err != nil {
			return nil, err
		}
		archives = append(archives, archive{
			path: filepath.Join(directory, entry.Name()), name: entry.Name(),
			size: info.Size(), modified: info.ModTime(),
		})
	}
	sort.Slice(archives, func(i, j int) bool { return archives[i].name > archives[j].name })
	return archives, nil
}

type commandRunner func(context.Context, string, ...string) (string, error)

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// Cancel the whole process group, including the shell backup job's children,
	// and let its existing signal trap remove partial archives and the lock.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	output, err := cmd.CombinedOutput()
	return string(output), err
}

var collectionPattern = regexp.MustCompile(`found collection .* bson to restore to`)
var suffixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func preflight(ctx context.Context, c config, path string, run commandRunner) (int, string, error) {
	if c.uri == "" {
		return 0, "", errors.New("MONGODB_URI must be set")
	}
	output, err := run(ctx, "mongorestore", "--uri="+c.uri, "--archive="+path, "--gzip", "--dryRun", "--verbose")
	if err != nil {
		return 0, output, fmt.Errorf("archive preflight failed; restore cancelled: %w", err)
	}
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if collectionPattern.MatchString(line) {
			count++
		}
	}
	if count == 0 {
		return 0, output, errors.New("archive contains no documents; restore cancelled")
	}
	return count, output, nil
}

func restore(ctx context.Context, c config, path string, run commandRunner) (string, error) {
	if c.uri == "" {
		return "", errors.New("MONGODB_URI must be set")
	}
	if err := os.Mkdir(c.lockPath, 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", errors.New("another backup or restore operation is already running")
		}
		return "", err
	}
	defer os.Remove(c.lockPath)
	return run(ctx, "mongorestore", "--uri="+c.uri, "--archive="+path, "--gzip", "--drop")
}

func renameArchive(path, suffix string) (string, error) {
	if !suffixPattern.MatchString(suffix) {
		return "", errors.New("suffix must contain only letters, numbers, hyphens, and underscores")
	}
	renamed := strings.TrimSuffix(path, ".archive.gz") + "-" + suffix + ".archive.gz"
	if _, err := os.Lstat(renamed); err == nil {
		return "", fmt.Errorf("%s already exists", filepath.Base(renamed))
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(path, renamed); err != nil {
		return "", err
	}
	return renamed, nil
}
