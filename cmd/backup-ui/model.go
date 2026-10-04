package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type panel int

const (
	archivesPanel panel = iota
	actionsPanel
	activityPanel
)

type dialog int

const (
	noDialog dialog = iota
	renameDialog
	restoreDialog
	helpDialog
)

type archivesMsg struct {
	archives []archive
	locked   bool
	err      error
}
type archiveDatesMsg struct {
	archives []archive
	err      error
}
type refreshMsg struct{}
type interruptMsg struct{}
type operationMsg struct {
	kind, path, output string
	collections        int
	err                error
}
type logEntry struct {
	time    time.Time
	message string
	failed  bool
}

type model struct {
	ctx       context.Context
	config    config
	run       commandRunner
	width     int
	height    int
	focus     panel
	archives  []archive
	visible   []archive
	selected  int
	action    int
	filter    textinput.Model
	searching bool
	suffix    textinput.Model
	dialog    dialog
	dialogErr string
	target    archive
	confirmed bool
	count     int
	busy      string
	cancel    context.CancelFunc
	quitting  bool
	locked    bool
	loading   bool
	loadErr   string
	spinner   spinner.Model
	logs      []logEntry
	logOffset int
	status    string
	failed    bool

	printedDates string
}

func newModel(ctx context.Context, c config, run commandRunner) model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "Filter archives…"
	filter.PromptStyle = strong
	filter.PlaceholderStyle = subtle
	filter.Cursor.Style = strong
	suffix := textinput.New()
	suffix.Prompt = "> "
	suffix.Placeholder = "e.g. before-import"
	suffix.PromptStyle = strong
	suffix.PlaceholderStyle = subtle
	suffix.Cursor.Style = strong
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = strong
	m := model{ctx: ctx, config: c, run: run, width: 100, height: 30,
		filter: filter, suffix: suffix, spinner: s, loading: true, status: "Loading archives…"}
	m.log("Backup workspace opened", false)
	return m
}

func (m model) Init() tea.Cmd { return tea.Batch(m.loadArchives(), refreshAfter()) }

func refreshAfter() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshMsg{} })
}

func (m model) loadArchives() tea.Cmd {
	return func() tea.Msg {
		archives, err := readArchives(m.config.directory)
		_, lockErr := os.Stat(m.config.lockPath)
		return archivesMsg{archives: archives, locked: lockErr == nil, err: err}
	}
}

func (m *model) log(message string, failed bool) {
	m.logs = append(m.logs, logEntry{time: time.Now(), message: message, failed: failed})
	if len(m.logs) > 500 {
		m.logs = m.logs[len(m.logs)-500:]
	}
	m.logOffset = 0
}

func (m *model) notify(message string, failed bool) {
	m.status, m.failed = message, failed
	m.log(message, failed)
}

func (m model) selectedArchive() (archive, bool) {
	if m.selected < 0 || m.selected >= len(m.visible) {
		return archive{}, false
	}
	return m.visible[m.selected], true
}

func (m *model) applyFilter(preferred string) {
	m.visible = nil
	query := strings.ToLower(m.filter.Value())
	for _, a := range m.archives {
		if strings.Contains(strings.ToLower(a.name), query) {
			m.visible = append(m.visible, a)
		}
	}
	m.selected = min(max(0, m.selected), max(0, len(m.visible)-1))
	for i, a := range m.visible {
		if a.path == preferred {
			m.selected = i
			break
		}
	}
}

func (m *model) startOperation(kind string, target archive) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.busy = kind
	m.target = target
	m.dialog = noDialog
	titles := map[string]string{"backup": "Creating backup…", "preflight": "Checking archive contents…", "restore": "Restoring backup…"}
	m.notify(titles[kind], false)
	c, run := m.config, m.run
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		result := operationMsg{kind: kind, path: target.path}
		switch kind {
		case "backup":
			result.output, result.err = run(ctx, "backup-now")
		case "preflight":
			result.collections, result.output, result.err = preflight(ctx, c, target.path, run)
		case "restore":
			result.output, result.err = restore(ctx, c, target.path, run)
		}
		return result
	})
}

func (m *model) activate(action int) tea.Cmd {
	if m.busy != "" {
		return nil
	}
	if action == 4 {
		m.busy = "listing"
		m.notify("Reading all backup dates…", false)
		directory := m.config.directory
		return func() tea.Msg {
			archives, err := readArchives(directory)
			return archiveDatesMsg{archives: archives, err: err}
		}
	}
	if action == 3 {
		m.status, m.failed = "Refreshing archives…", false
		return m.loadArchives()
	}
	if action == 0 {
		return m.startOperation("backup", archive{})
	}
	a, ok := m.selectedArchive()
	if !ok {
		m.notify("Select an archive first", true)
		return nil
	}
	m.target, m.dialogErr = a, ""
	if action == 1 {
		return m.startOperation("preflight", a)
	}
	m.dialog = renameDialog
	m.suffix.SetValue("")
	return m.suffix.Focus()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.filter.Width = max(1, m.layout().left-6)
		m.suffix.Width = max(1, min(56, m.width-14))
	case archivesMsg:
		previous, _ := m.selectedArchive()
		m.locked, m.loading = msg.locked, false
		if msg.err != nil {
			if m.loadErr != msg.err.Error() {
				m.notify("Cannot read archives: "+msg.err.Error(), true)
			}
			m.loadErr = msg.err.Error()
			return m, nil
		}
		recovered := m.loadErr != ""
		m.archives, m.loadErr = msg.archives, ""
		m.applyFilter(previous.path)
		if recovered || m.status == "Loading archives…" || m.status == "Refreshing archives…" {
			m.status, m.failed = "Ready", false
		}
	case archiveDatesMsg:
		m.busy = ""
		if m.quitting {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.notify("Cannot print backup dates: "+msg.err.Error(), true)
			return m, nil
		}
		output, err := formatArchiveDates(msg.archives)
		if err != nil {
			m.notify("Cannot print backup dates: "+err.Error(), true)
			return m, nil
		}
		m.printedDates = output
		m.quitting = true
		return m, tea.Quit
	case refreshMsg:
		return m, tea.Batch(m.loadArchives(), refreshAfter())
	case spinner.TickMsg:
		if m.busy != "" {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case operationMsg:
		if m.cancel != nil {
			m.cancel()
		}
		m.cancel, m.busy = nil, ""
		for _, line := range strings.Split(strings.TrimSpace(m.redact(msg.output)), "\n") {
			if line != "" {
				m.log(line, msg.err != nil)
			}
		}
		if m.quitting {
			return m, tea.Quit
		}
		label := map[string]string{"backup": "Backup", "preflight": "Preflight", "restore": "Restore"}[msg.kind]
		if msg.err != nil {
			m.notify(label+" failed: "+m.redact(msg.err.Error()), true)
		} else if msg.kind == "preflight" {
			m.count, m.dialog, m.confirmed = msg.collections, restoreDialog, false
			m.notify(fmt.Sprintf("Archive contains %d collections", msg.collections), false)
		} else {
			m.notify(label+" completed", false)
		}
		return m, m.loadArchives()
	case interruptMsg:
		return m.exit()
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m.exit()
		}
		if m.quitting {
			return m, nil
		}
		if m.width < 60 || m.height < 20 {
			if msg.String() == "p" {
				return m, m.activate(4)
			}
			if msg.String() == "q" && m.busy == "" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.dialog != noDialog {
			return m.handleDialog(msg)
		}
		if m.searching {
			switch msg.String() {
			case "enter", "esc":
				m.searching = false
				m.filter.Blur()
				if msg.String() == "esc" {
					m.filter.SetValue("")
					m.applyFilter("")
				}
			default:
				var cmd tea.Cmd
				m.filter, cmd = m.filter.Update(msg)
				m.selected = 0
				m.applyFilter("")
				return m, cmd
			}
			return m, nil
		}
		switch msg.String() {
		case "q":
			if m.busy == "" {
				return m, tea.Quit
			}
		case "tab", "l", "right":
			m.focus = (m.focus + 1) % 3
		case "shift+tab", "h", "left":
			m.focus = (m.focus + 2) % 3
		case "1":
			m.focus = archivesPanel
		case "2":
			m.focus = actionsPanel
		case "3":
			m.focus = activityPanel
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-m.layout().listRows)
		case "pgdown":
			m.move(m.layout().listRows)
		case "home", "g":
			m.move(-len(m.visible) - len(m.logs))
		case "end", "G":
			m.move(len(m.visible) + len(m.logs))
		case "/":
			m.focus, m.searching = archivesPanel, true
			return m, m.filter.Focus()
		case "esc":
			m.filter.SetValue("")
			m.applyFilter("")
		case "?":
			m.dialog = helpDialog
		case "b":
			return m, m.activate(0)
		case "r":
			return m, m.activate(1)
		case "n":
			return m, m.activate(2)
		case "R":
			return m, m.activate(3)
		case "p":
			return m, m.activate(4)
		case "enter", " ":
			if m.focus == actionsPanel {
				return m, m.activate(m.action)
			}
			m.focus = actionsPanel
		}
	}
	return m, nil
}

func (m model) exit() (tea.Model, tea.Cmd) {
	if m.busy == "" {
		return m, tea.Quit
	}
	m.quitting = true
	m.notify("Stopping operation and releasing its lock…", false)
	if m.cancel != nil {
		m.cancel()
	}
	return m, nil
}

func (m *model) move(delta int) {
	switch m.focus {
	case archivesPanel:
		m.selected = min(max(0, m.selected+delta), max(0, len(m.visible)-1))
	case actionsPanel:
		m.action = min(max(0, m.action+delta), len(actionNames)-1)
	case activityPanel:
		m.logOffset = min(max(0, m.logOffset-delta), max(0, len(m.logs)-m.layout().logRows))
	}
}

func (m model) handleDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" || (m.dialog != renameDialog && key == "q") {
		if m.dialog == restoreDialog {
			m.notify("Restore cancelled", false)
		}
		m.dialog = noDialog
		m.suffix.Blur()
		return m, nil
	}
	switch m.dialog {
	case helpDialog:
		if key == "?" || key == "enter" {
			m.dialog = noDialog
		}
	case renameDialog:
		if key == "enter" {
			renamed, err := renameArchive(m.target.path, m.suffix.Value())
			if err != nil {
				m.dialogErr = err.Error()
				return m, nil
			}
			m.dialog = noDialog
			m.suffix.Blur()
			m.filter.SetValue("")
			// Preserve the renamed archive's selection when the list reloads.
			m.visible = []archive{{path: renamed}}
			m.selected = 0
			m.notify("Renamed to "+filepath.Base(renamed), false)
			return m, m.loadArchives()
		}
		var cmd tea.Cmd
		m.suffix, cmd = m.suffix.Update(msg)
		return m, cmd
	case restoreDialog:
		switch key {
		case "tab", "shift+tab", "left", "right", "h", "l":
			m.confirmed = !m.confirmed
		case "enter":
			if m.confirmed {
				return m, m.startOperation("restore", m.target)
			}
			m.dialog = noDialog
			m.notify("Restore cancelled", false)
		}
	}
	return m, nil
}

func (m model) redact(value string) string {
	if m.config.uri != "" {
		value = strings.ReplaceAll(value, m.config.uri, "[MongoDB URI]")
	}
	return value
}
