package main

import (
	"testing"
	"time"
)

func TestBackupAgeSurvivesRenameAndCopy(t *testing.T) {
	now := time.Date(2026, 10, 4, 18, 20, 0, 0, time.UTC)
	for _, name := range []string{
		"mongodb-2026-10-02T13-20-00Z.archive.gz",
		"mongodb-2026-10-02T13-20-00Z-before-import.archive.gz",
	} {
		a := archive{name: name, modified: now}
		captured, fromName := a.backupTime()
		if !fromName || ageLabel(captured, now, false) != "2 days and 5 hours ago" {
			t.Fatalf("%s used the copy time instead of snapshot time", name)
		}
		if ageLabel(captured, now, true) != "2d 5h" {
			t.Fatal("compact age lost the days or hours")
		}
	}
	for _, name := range []string{"external.archive.gz", "mongodb-2026-99-02T13-20-00Z.archive.gz"} {
		a := archive{name: name, modified: now.Add(-20 * time.Minute)}
		timestamp, fromName := a.backupTime()
		if fromName || ageLabel(timestamp, now, false) != "20 minutes ago" {
			t.Fatalf("%s did not fall back to its file timestamp", name)
		}
	}
}
