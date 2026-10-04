package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T) config {
	t.Helper()
	dir := t.TempDir()
	return config{directory: dir, uri: "mongodb://user:secret@mongo:27017/test?authSource=admin", lockPath: filepath.Join(dir, "operation.lock")}
}

func writeArchive(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("sample archive"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadArchives(t *testing.T) {
	c := testConfig(t)
	for _, name := range []string{"mongodb-2026-01-01.archive.gz", "mongodb-2026-01-02-before-import.archive.gz", "external.archive.gz", "mongodb-partial.archive.gz.partial", "notes.txt"} {
		writeArchive(t, c.directory, name)
	}
	if err := os.Mkdir(filepath.Join(c.directory, "directory.archive.gz"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(c.directory, "external.archive.gz"), filepath.Join(c.directory, "link.archive.gz")); err != nil {
		t.Fatal(err)
	}
	archives, err := readArchives(c.directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range archives {
		names = append(names, a.name)
		if a.size != 14 || a.modified.IsZero() {
			t.Fatalf("missing metadata: %+v", a)
		}
	}
	want := []string{"mongodb-2026-01-02-before-import.archive.gz", "mongodb-2026-01-01.archive.gz", "external.archive.gz"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v; want %v", names, want)
	}
	if archives, err = readArchives(filepath.Join(c.directory, "missing")); err != nil || len(archives) != 0 {
		t.Fatalf("missing directory should be empty: %v %v", archives, err)
	}
}

func TestRenameArchive(t *testing.T) {
	c := testConfig(t)
	path := writeArchive(t, c.directory, "mongodb-test.archive.gz")
	for _, suffix := range []string{"", "../bad", "white space", "-bad", "a/b"} {
		if _, err := renameArchive(path, suffix); err == nil {
			t.Fatalf("accepted invalid suffix %q", suffix)
		}
	}
	collision := writeArchive(t, c.directory, "mongodb-test-existing.archive.gz")
	if _, err := renameArchive(path, "existing"); err == nil {
		t.Fatal("overwrote an existing archive")
	}
	if data, _ := os.ReadFile(collision); string(data) != "sample archive" {
		t.Fatal("collision changed existing archive")
	}
	renamed, err := renameArchive(path, "before-import_1")
	if err != nil || filepath.Base(renamed) != "mongodb-test-before-import_1.archive.gz" {
		t.Fatalf("rename failed: %s %v", renamed, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old filename still exists")
	}
	if data, _ := os.ReadFile(renamed); string(data) != "sample archive" {
		t.Fatal("rename changed archive contents")
	}
}

func TestPreflight(t *testing.T) {
	c := testConfig(t)
	path := filepath.Join(c.directory, "external.archive.gz")
	output := "found collection test.customers bson to restore to\nfound collection test.orders bson to restore to\n"
	for _, tt := range []struct {
		name, output string
		err          error
		count        int
		wantError    bool
	}{
		{"valid", output, nil, 2, false},
		{"empty", "no collections", nil, 0, true},
		{"corrupt", output, errors.New("bad gzip"), 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			run := func(_ context.Context, name string, args ...string) (string, error) {
				want := []string{"--uri=" + c.uri, "--archive=" + path, "--gzip", "--dryRun", "--verbose"}
				if name != "mongorestore" || !reflect.DeepEqual(args, want) {
					t.Fatalf("unexpected preflight command: %s %v", name, args)
				}
				return tt.output, tt.err
			}
			count, gotOutput, err := preflight(context.Background(), c, path, run)
			if count != tt.count || gotOutput != tt.output || (err != nil) != tt.wantError {
				t.Fatalf("preflight returned %d, %q, %v", count, gotOutput, err)
			}
		})
	}
}

func TestRestoreLockAndCommand(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			c := testConfig(t)
			path := filepath.Join(c.directory, "external.archive.gz")
			run := func(_ context.Context, name string, args ...string) (string, error) {
				if _, err := os.Stat(c.lockPath); err != nil {
					t.Fatal("restore ran without lock")
				}
				want := []string{"--uri=" + c.uri, "--archive=" + path, "--gzip", "--drop"}
				if name != "mongorestore" || !reflect.DeepEqual(args, want) {
					t.Fatalf("unexpected restore command: %s %v", name, args)
				}
				if fail {
					return "restore output", errors.New("restore failed")
				}
				return "restore output", nil
			}
			_, err := restore(context.Background(), c, path, run)
			if (err != nil) != fail {
				t.Fatalf("unexpected restore result: %v", err)
			}
			if _, err := os.Stat(c.lockPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("restore left lock behind")
			}
		})
	}
	c := testConfig(t)
	if err := os.Mkdir(c.lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := restore(context.Background(), c, "archive", func(context.Context, string, ...string) (string, error) {
		t.Fatal("restore ran while lock was held")
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected lock conflict: %v", err)
	}
	if _, err := os.Stat(c.lockPath); err != nil {
		t.Fatal("removed another operation's lock")
	}
}

func TestCommandCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	output, err := runCommand(ctx, "sh", "-c", "trap 'echo cleanup; exit 0' TERM; sleep 30 & wait")
	if err == nil || !strings.Contains(output, "cleanup") || time.Since(started) > 3*time.Second {
		t.Fatalf("command cancellation did not allow cleanup: %q, %v", output, err)
	}
}

func TestDestinationHidesCredentials(t *testing.T) {
	if got := testConfig(t).destination(); got != "mongo:27017 / test" {
		t.Fatalf("unexpected destination: %s", got)
	}
}
