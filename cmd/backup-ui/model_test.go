package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func key(m model, value string) (model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	switch value {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

func TestRestoreRequiresPreflightAndExplicitConfirmation(t *testing.T) {
	c := testConfig(t)
	m := newModel(context.Background(), c, func(context.Context, string, ...string) (string, error) {
		t.Fatal("operation executed before confirmation")
		return "", nil
	})
	a := archive{path: writeArchive(t, c.directory, "external.archive.gz"), name: "external.archive.gz"}
	m.archives, m.visible = []archive{a}, []archive{a}
	m, cmd := key(m, "r")
	if m.busy != "preflight" || cmd == nil || m.dialog != noDialog {
		t.Fatal("restore did not begin with preflight")
	}
	next, _ := m.Update(operationMsg{kind: "preflight", path: a.path, collections: 4})
	m = next.(model)
	if m.dialog != restoreDialog || m.confirmed || m.count != 4 {
		t.Fatal("restore confirmation should default to Cancel")
	}
	cancelled, cmd := key(m, "enter")
	if cancelled.dialog != noDialog || cancelled.busy != "" || cmd != nil {
		t.Fatal("Enter on Cancel ran a restore")
	}
	m, _ = key(m, "tab")
	m, cmd = key(m, "enter")
	if m.busy != "restore" || cmd == nil || m.target.path != a.path {
		t.Fatal("explicit confirmation did not start restore of preflighted archive")
	}
	m.cancel()
}

func TestFailedPreflightCannotOpenRestoreDialog(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	next, _ := m.Update(operationMsg{kind: "preflight", err: errors.New("invalid archive")})
	m = next.(model)
	if m.dialog != noDialog || !m.failed || !strings.Contains(m.status, "invalid archive") {
		t.Fatal("failed preflight offered a restore")
	}
}

func TestBackupFailureIsVisible(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	next, _ := m.Update(operationMsg{kind: "backup", err: errors.New("dump failed"), output: "cannot connect"})
	m = next.(model)
	if !m.failed || strings.Contains(m.status, "completed") || !strings.Contains(m.status, "dump failed") {
		t.Fatalf("backup failure reported as success: %s", m.status)
	}
}

func TestFilterAndRefreshKeepSelection(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	m.archives = []archive{{path: "/b", name: "mongodb-b.archive.gz"}, {path: "/a", name: "mongodb-a-before-import.archive.gz"}}
	m.applyFilter("")
	m.selected = 1
	next, _ := m.Update(archivesMsg{archives: append([]archive{{path: "/c", name: "mongodb-c.archive.gz"}}, m.archives...)})
	m = next.(model)
	if a, _ := m.selectedArchive(); a.path != "/a" {
		t.Fatal("refresh changed selected archive")
	}
	m.filter.SetValue("BEFORE-IMPORT")
	m.applyFilter("")
	if len(m.visible) != 1 || m.visible[0].path != "/a" {
		t.Fatal("case insensitive filter did not match")
	}
	m, _ = key(m, "esc")
	if len(m.visible) != 3 {
		t.Fatal("Escape did not clear filter")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, dimensions := range [][2]int{{60, 20}, {80, 24}, {120, 36}, {180, 50}} {
		m := newModel(context.Background(), testConfig(t), nil)
		m.width, m.height = dimensions[0], dimensions[1]
		m.archives = []archive{{path: "/backups/a", name: strings.Repeat("long-", 40) + ".archive.gz"}}
		m.applyFilter("")
		for _, dialog := range []dialog{noDialog, helpDialog, renameDialog, restoreDialog} {
			m.dialog = dialog
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) > m.height {
				t.Fatalf("%dx%d dialog %d is %d lines tall", m.width, m.height, dialog, len(lines))
			}
			for _, line := range lines {
				if width := ansi.StringWidth(line); width > m.width {
					t.Fatalf("%dx%d dialog %d line is %d cells wide", m.width, m.height, dialog, width)
				}
			}
		}
	}
}

func TestMouseSelectsActionBeforeRunningIt(t *testing.T) {
	m := newModel(context.Background(), testConfig(t), nil)
	m.width, m.height = 120, 36
	g := m.layout()
	click := tea.MouseMsg{X: g.left + 3, Y: 2 + g.details + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, cmd := m.Update(click)
	m = next.(model)
	if cmd != nil || m.focus != actionsPanel || m.busy != "" {
		t.Fatal("first click should select the action")
	}
	next, cmd = m.Update(click)
	m = next.(model)
	if cmd == nil || m.busy != "backup" {
		t.Fatal("second click should run selected action")
	}
	m.cancel()
}
