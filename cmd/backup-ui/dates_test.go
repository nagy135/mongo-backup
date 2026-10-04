package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestArchiveDatesUseCaptureTimesAndKeepEveryArchive(t *testing.T) {
	modified := time.Date(2026, 10, 4, 21, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	archives := []archive{
		{name: "mongodb-2026-10-02T13-20-00Z-before-import.archive.gz", modified: modified},
		{name: "external.archive.gz", modified: modified},
		{name: "mongodb-2026-10-04T18-21-36Z.archive.gz", modified: modified},
		{name: "mongodb-2026-10-02T13-20-00Z.archive.gz", modified: modified},
	}
	want := "Available backup dates (Europe/Berlin, newest first):\n" +
		"2026-10-04 21:00:00 CEST (+02:00) (file modification time; backup date unknown)\n" +
		"2026-10-04 20:21:36 CEST (+02:00)\n" +
		"2026-10-02 15:20:00 CEST (+02:00)\n" +
		"2026-10-02 15:20:00 CEST (+02:00)\n"
	if got, err := formatArchiveDates(archives); err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
	if archives[0].name != "mongodb-2026-10-02T13-20-00Z-before-import.archive.gz" {
		t.Fatal("printing reordered the UI's archive list")
	}
	if got, err := formatArchiveDates([]archive{{name: "external.archive.gz"}}); err != nil || !strings.HasSuffix(got, "Unknown backup date\n") {
		t.Fatalf("missing capture and file time should stay unknown: %q, %v", got, err)
	}
}

func TestArchiveDatesUseCentralEuropeanDaylightSavingTime(t *testing.T) {
	archives := []archive{
		{name: "mongodb-2026-01-04T18-21-36Z.archive.gz"},
		{name: "mongodb-2026-07-04T18-21-36Z.archive.gz"},
		// The clocks go back: these different instants have the same local time.
		{name: "mongodb-2026-10-25T00-30-00Z.archive.gz"},
		{name: "mongodb-2026-10-25T01-30-00Z.archive.gz"},
	}
	want := "Available backup dates (Europe/Berlin, newest first):\n" +
		"2026-10-25 02:30:00 CET (+01:00)\n" +
		"2026-10-25 02:30:00 CEST (+02:00)\n" +
		"2026-07-04 20:21:36 CEST (+02:00)\n" +
		"2026-01-04 19:21:36 CET (+01:00)\n"
	if got, err := formatArchiveDates(archives); err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}

func TestPrintDatesRereadsAllArchivesAndQuits(t *testing.T) {
	c := testConfig(t)
	oldName := "mongodb-2026-10-02T13-20-00Z-before-import.archive.gz"
	old := archive{path: writeArchive(t, c.directory, oldName), name: oldName}
	m := newModel(context.Background(), c, nil)
	m.archives = []archive{old}
	m.filter.SetValue("before-import")
	m.applyFilter("")
	// These archives appeared after the last UI refresh and don't match the filter.
	for i := 0; i < 40; i++ {
		captured := time.Date(2026, 10, 4, 18, 21, i, 0, time.UTC)
		writeArchive(t, c.directory, "mongodb-"+captured.Format("2006-01-02T15-04-05Z")+".archive.gz")
	}
	m, cmd := key(m, "p")
	if cmd == nil || m.printedDates != "" {
		t.Fatal("printing should read the directory before exiting")
	}
	next, quit := m.Update(cmd())
	m = next.(model)
	if quit == nil {
		t.Fatal("printing did not quit")
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("printing did not request a clean terminal shutdown")
	}
	lines := strings.Split(strings.TrimSpace(m.printedDates), "\n")
	if len(lines) != 42 || lines[1] != "2026-10-04 20:21:39 CEST (+02:00)" || lines[41] != "2026-10-02 15:20:00 CEST (+02:00)" {
		t.Fatalf("printed list was stale, filtered, clipped, or misordered: %q", m.printedDates)
	}
}

func TestPrintDatesEmptyDirectory(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	m.focus, m.action = actionsPanel, 4
	m, cmd := key(m, "enter")
	if cmd == nil {
		t.Fatal("Print dates action did not run without a selected archive")
	}
	next, quit := m.Update(cmd())
	if next.(model).printedDates != "No backup archives available.\n" || quit == nil {
		t.Fatal("empty directory did not print an explicit empty result and quit")
	}
}

func TestPrintDatesReadFailureStaysInUI(t *testing.T) {
	c := testConfig(t)
	c.directory = writeArchive(t, c.directory, "not-a-directory")
	m := newModel(context.Background(), c, nil)
	m, cmd := key(m, "p")
	next, quit := m.Update(cmd())
	m = next.(model)
	if quit != nil || m.printedDates != "" || !m.failed || m.busy != "" || !strings.Contains(m.status, "Cannot print backup dates") {
		t.Fatalf("read failure exited with a misleading list: %+v", m)
	}
}

func TestPrintDatesDoesNotInterruptOperationsOrTextInput(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	cancelled := false
	m.busy, m.cancel = "restore", func() { cancelled = true }
	m, cmd := key(m, "p")
	if cmd != nil || m.busy != "restore" || cancelled {
		t.Fatal("print shortcut interrupted the restore")
	}
	m.busy = ""
	m, _ = key(m, "/")
	m, cmd = key(m, "p")
	if m.busy != "" || m.filter.Value() != "p" || m.printedDates != "" {
		t.Fatal("print shortcut prevented typing a filter")
	}
	m.searching = false
	m, cmd = key(m, "q")
	if cmd == nil || m.printedDates != "" {
		t.Fatal("ordinary quit should not print backup dates")
	}
}

func TestPrintDatesCancellationDoesNotPrint(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	m, read := key(m, "p")
	next, quit := m.Update(interruptMsg{})
	m = next.(model)
	if quit != nil || !m.quitting {
		t.Fatal("interrupt should wait for the pending directory read")
	}
	next, quit = m.Update(read())
	if quit == nil || next.(model).printedDates != "" {
		t.Fatal("cancelled listing should quit without printing")
	}
}
