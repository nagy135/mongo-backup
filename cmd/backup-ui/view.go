package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	// Explicit fallbacks avoid mapping unrelated RGB colors to the same ANSI
	// palette entry when Docker's terminal only advertises 16-color support.
	accent = lipgloss.CompleteColor{TrueColor: "#93C5FD", ANSI256: "111", ANSI: "12"}
	muted  = lipgloss.CompleteColor{TrueColor: "#CBD5E1", ANSI256: "252", ANSI: "7"}
	border = lipgloss.CompleteColor{TrueColor: "#94A3B8", ANSI256: "246", ANSI: "7"}
	danger = lipgloss.CompleteColor{TrueColor: "#FDA4AF", ANSI256: "210", ANSI: "9"}
	// Reverse the terminal's default color pair, rather than relying on two
	// independently remapped colors to contrast. This also works without color.
	selected = lipgloss.NewStyle().Reverse(true).Bold(true)
	subtle   = lipgloss.NewStyle().Foreground(muted)
	strong   = lipgloss.NewStyle().Foreground(accent).Bold(true)
)

type geometry struct {
	width, left, right, top, logHeight, listRows, logRows, details int
}

func (m model) layout() geometry {
	w := max(1, m.width-2)
	logHeight := min(9, max(5, m.height/4))
	top := max(8, m.height-logHeight-5)
	left := w * 63 / 100
	details := 0
	if top >= 18 && w >= 100 {
		details = 10
	}
	return geometry{width: w, left: left, right: w - left, top: top,
		logHeight: logHeight, listRows: max(1, top-5), logRows: max(1, logHeight-3), details: details}
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}

func fit(s string, width int) string {
	return ansi.Truncate(s, max(0, width), "…")
}

func bytesLabel(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= 1024
		if value < 1024 || unit == "TiB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return ""
}

func ageLabel(timestamp, now time.Time, compact bool) string {
	if timestamp.IsZero() {
		return "unknown"
	}
	age := now.Sub(timestamp)
	if age < 0 {
		return "in the future"
	}
	if age < time.Minute {
		if compact {
			return "<1m"
		}
		return "just now"
	}
	minutes := int(age / time.Minute)
	hours := int(age / time.Hour)
	days := hours / 24
	if compact {
		switch {
		case days > 0 && hours%24 > 0:
			return fmt.Sprintf("%dd %dh", days, hours%24)
		case days > 0:
			return fmt.Sprintf("%dd", days)
		case hours > 0:
			return fmt.Sprintf("%dh", hours)
		default:
			return fmt.Sprintf("%dm", minutes)
		}
	}
	unit := func(value int, name string) string {
		if value != 1 {
			name += "s"
		}
		return fmt.Sprintf("%d %s", value, name)
	}
	switch {
	case days > 0 && hours%24 > 0:
		return unit(days, "day") + " and " + unit(hours%24, "hour") + " ago"
	case days > 0:
		return unit(days, "day") + " ago"
	case hours > 0:
		return unit(hours, "hour") + " ago"
	default:
		return unit(minutes, "minute") + " ago"
	}
}

// Panels have fixed cell dimensions so resize, selection, and mouse targets
// share the same geometry. Each line is clipped before padding.
func frame(title string, lines []string, width, height int, focused bool) string {
	inner := max(1, width-2)
	color := border
	if focused {
		color = accent
	}
	edge := lipgloss.NewStyle().Foreground(color)
	title = fit(" "+title+" ", inner)
	rows := []string{edge.Render("╭" + title + strings.Repeat("─", max(0, inner-lipgloss.Width(title))) + "╮")}
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = fit(lines[i], inner)
		}
		rows = append(rows, edge.Render("│")+line+strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))+edge.Render("│"))
	}
	rows = append(rows, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(rows, "\n")
}

func (m model) listStart(g geometry) int {
	return max(0, m.selected-g.listRows+1)
}

func (m model) archivesView(g geometry) string {
	inner := g.left - 2
	now := time.Now()
	showSize := inner >= 55
	ageWidth := len("AGE")
	ages := make([]string, len(m.visible))
	for i, a := range m.visible {
		timestamp, _ := a.backupTime()
		ages[i] = ageLabel(timestamp, now, true)
		ageWidth = max(ageWidth, len(ages[i]))
	}
	nameWidth := inner - 4 - ageWidth
	if showSize {
		nameWidth -= 11 // Size column and its separating space.
	}
	nameWidth = max(1, nameWidth)
	row := func(prefix, name, size, age string) string {
		name = fit(name, nameWidth)
		line := prefix + name + strings.Repeat(" ", max(0, nameWidth-lipgloss.Width(name)))
		if showSize {
			line += fmt.Sprintf("  %9s", size)
		}
		return line + "  " + fmt.Sprintf("%*s", ageWidth, age)
	}
	search := subtle.Render(" / Filter archives")
	if m.searching {
		search = " " + m.filter.View()
	} else if m.filter.Value() != "" {
		search = strong.Render(" / "+clean(m.filter.Value())) + subtle.Render("  esc clear")
	}
	lines := []string{search, subtle.Render(row("  ", "ARCHIVE", "SIZE", "AGE"))}
	start := m.listStart(g)
	for i := 0; i < g.listRows; i++ {
		index := start + i
		line := ""
		if index < len(m.visible) {
			a := m.visible[index]
			prefix := "  "
			if index == m.selected {
				prefix = "› "
			}
			line = row(prefix, clean(a.name), bytesLabel(a.size), ages[index])
			if index == m.selected {
				line = selected.Width(g.left - 2).Render(line)
			}
		} else if i == 0 {
			switch {
			case m.loading:
				line = subtle.Render(" Loading archives…")
			case m.loadErr != "":
				line = lipgloss.NewStyle().Foreground(danger).Render(" Cannot read backup directory")
			case m.filter.Value() != "":
				line = subtle.Render(" No matching archives. Esc clears filter.")
			default:
				line = subtle.Render(" No archives yet. Press b to create one.")
			}
		}
		lines = append(lines, line)
	}
	var total int64
	for _, a := range m.archives {
		total += a.size
	}
	position := 0
	if len(m.visible) > 0 {
		position = m.selected + 1
	}
	lines = append(lines, subtle.Render(fmt.Sprintf(" %d/%d selected · %s total", position, len(m.visible), bytesLabel(total))))
	return frame(fmt.Sprintf("1 Archives · %d", len(m.archives)), lines, g.left, g.top, m.focus == archivesPanel)
}

func (m model) detailsView(g geometry) string {
	lines := []string{subtle.Render(" No archive selected"), "", " Press b to create a backup."}
	if a, ok := m.selectedArchive(); ok {
		timestamp, captured := a.backupTime()
		ageHeading, timeHeading := " Age       ", " Captured  "
		if !captured {
			ageHeading, timeHeading = " File age  ", " Modified  "
		}
		lines = []string{
			" " + strong.Render(fit(clean(a.name), g.right-4)), "",
			subtle.Render(ageHeading) + ageLabel(timestamp, time.Now(), false),
			subtle.Render(" Size      ") + fmt.Sprintf("%s (%d bytes)", bytesLabel(a.size), a.size),
			subtle.Render(timeHeading) + timestamp.UTC().Format("2006-01-02 15:04 UTC"),
			subtle.Render(" Format    ") + "gzip · MongoDB archive", "",
			subtle.Render(" ") + fit(clean(a.path), g.right-4),
		}
	}
	return frame("Selected archive", lines, g.right, g.details, false)
}

var actionNames = []string{"Create backup", "Restore selected", "Rename selected", "Refresh archives", "Print dates and quit"}
var actionKeys = []string{"b", "r", "n", "R", "p"}

func (m model) actionsView(g geometry) string {
	lines := []string{""}
	for i, name := range actionNames {
		label := "  [" + actionKeys[i] + "] " + name
		if m.focus == actionsPanel && i == m.action {
			label = selected.Width(g.right - 2).Render("›" + label[1:])
		} else if m.busy != "" {
			label = subtle.Render(label)
		}
		lines = append(lines, label)
	}
	lines = append(lines, "")
	if g.details == 0 {
		if a, ok := m.selectedArchive(); ok && g.top >= 12 {
			lines = append(lines, subtle.Render(" Selected archive"), " "+fit(clean(a.name), g.right-4),
				" "+bytesLabel(a.size)+subtle.Render(" · "+a.modified.UTC().Format("Jan 02 15:04 UTC")), "")
		}
	}
	lines = append(lines, subtle.Render(" Enter runs highlighted action"))
	return frame("2 Actions", lines, g.right, g.top-g.details, m.focus == actionsPanel)
}

func (m model) activityView(g geometry) string {
	end := max(0, len(m.logs)-m.logOffset)
	start := max(0, end-g.logRows)
	lines := []string{}
	for _, entry := range m.logs[start:end] {
		style := lipgloss.NewStyle()
		if entry.failed {
			style = style.Foreground(danger)
		}
		lines = append(lines, " "+subtle.Render(entry.time.Format("15:04:05"))+"  "+style.Render(clean(entry.message)))
	}
	title := "3 Activity"
	if m.busy != "" {
		title += " · " + m.spinner.View() + " " + m.busy
	} else if m.logOffset > 0 {
		title += " · scroll down for latest"
	}
	return frame(title, lines, g.width, g.logHeight, m.focus == activityPanel)
}

func (m model) View() string {
	if m.width < 60 || m.height < 20 {
		return fit("MongoDB Backups · enlarge terminal to 60×20 · p print dates · q quit", m.width)
	}
	g := m.layout()
	if m.dialog != noDialog {
		return m.dialogView(g)
	}
	state := strong.Render("● READY")
	if m.busy != "" {
		state = strong.Render(m.spinner.View() + " WORKING")
	} else if m.locked {
		state = subtle.Render("● OPERATION LOCKED")
	}
	brand := strong.Render("MONGODB BACKUPS") + subtle.Render("  /  "+clean(m.config.destination()))
	header := fit(brand, max(1, g.width-lipgloss.Width(state)-2))
	header += strings.Repeat(" ", max(1, g.width-lipgloss.Width(header)-lipgloss.Width(state))) + state
	configuration := subtle.Render(fit("Archives: "+clean(m.config.directory)+"   Cron: "+clean(m.config.schedule)+"   Retention: "+clean(m.config.retention), g.width))
	right := m.actionsView(g)
	if g.details > 0 {
		right = m.detailsView(g) + "\n" + right
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.archivesView(g), right)
	statusStyle := subtle
	if m.failed {
		statusStyle = lipgloss.NewStyle().Foreground(danger)
	}
	status := statusStyle.Render(fit(" "+clean(m.status), g.width))
	footer := subtle.Render(fit(" tab panels  ↑↓/jk navigate  b backup  r restore  n rename  p print dates  / filter  ? help  q quit", g.width))
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join([]string{header, configuration, body, m.activityView(g), status, footer, ""}, "\n"))
}

func (m model) dialogView(g geometry) string {
	width := min(g.width, 76)
	contentWidth := width - 6
	var title string
	var lines []string
	switch m.dialog {
	case helpDialog:
		title = "Keyboard shortcuts"
		lines = []string{
			"", "  Tab / Shift+Tab     Next / previous panel", "  1 / 2 / 3          Archives / actions / activity",
			"  ↑↓ or j/k          Navigate or scroll", "  Home/End · PgUp/Dn  Jump through the list",
			"  b                  Create backup", "  r                  Preflight and restore selected archive",
			"  n                  Rename selected archive with a suffix", "  R                  Refresh archives",
			"  p                  Print all backup dates and quit",
			"  /                  Filter archives by name", "  Esc                Clear filter / close dialog",
			"  q                  Quit when idle", "  Ctrl+C             Stop active operation and quit", "",
			"  Click panels, archive rows, or actions to select them.", "  Click an action again to run it. Mouse wheel scrolls.", "",
			strong.Render("  Enter / Esc to return"), "",
		}
	case renameDialog:
		title = "Rename archive"
		lines = []string{"", "  " + fit(clean(m.target.name), contentWidth), "",
			"  Add a suffix to the existing filename.", subtle.Render("  Letters, numbers, hyphens, and underscores."), "",
			"  " + m.suffix.View(), "",
			subtle.Render("  ") + fit(clean(strings.TrimSuffix(m.target.name, ".archive.gz")+"-"+m.suffix.Value()+".archive.gz"), contentWidth), ""}
		if m.dialogErr != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(danger).Render("  "+fit(clean(m.dialogErr), contentWidth)), "")
		}
		lines = append(lines, strong.Render("  Enter rename")+subtle.Render("   Esc cancel"), "")
	case restoreDialog:
		title = "Restore archive"
		cancel, confirm := "[ Cancel ]", "[ Restore ]"
		if m.confirmed {
			confirm = selected.Render(confirm)
		} else {
			cancel = selected.Render(cancel)
		}
		lines = []string{"", "  " + fit(clean(m.target.name), contentWidth), "",
			strong.Render(fmt.Sprintf("  Preflight passed · %d collections", m.count)), "",
			lipgloss.NewStyle().Foreground(danger).Bold(true).Render("  Matching collections will be dropped and replaced."),
			"  Destination: " + fit(clean(m.config.destination()), contentWidth-15), "",
			"  " + cancel + "   " + confirm, "", subtle.Render("  Tab / ←→ choose   Enter confirm   Esc cancel"), ""}
	}
	if len(lines)+2 > m.height-2 {
		// Compact help keeps the operational shortcuts visible in short terminals.
		lines = lines[:min(len(lines), m.height-5)]
		lines = append(lines, strong.Render("  Esc close"))
	}
	box := frame(title, lines, width, len(lines)+2, true)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.dialog != noDialog || m.searching || m.quitting || m.width < 60 || m.height < 20 {
		return m, nil
	}
	g := m.layout()
	x, y := msg.X-1, msg.Y-2
	previousFocus := m.focus
	if x < 0 || x >= g.width || y < 0 {
		return m, nil
	}
	if y >= g.top && y < g.top+g.logHeight {
		m.focus = activityPanel
	} else if y < g.top && x < g.left {
		m.focus = archivesPanel
	} else if y >= g.details && y < g.top && x >= g.left {
		m.focus = actionsPanel
	} else {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.move(-3)
	case tea.MouseButtonWheelDown:
		m.move(3)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if m.focus == archivesPanel {
			index := m.listStart(g) + y - 3
			if y >= 3 && y < 3+g.listRows && index < len(m.visible) {
				m.selected = index
			}
		} else if m.focus == actionsPanel {
			index := y - g.details - 2
			if index >= 0 && index < len(actionNames) {
				if previousFocus == actionsPanel && m.action == index {
					return m, m.activate(index)
				}
				m.action = index
			}
		}
	}
	return m, nil
}
