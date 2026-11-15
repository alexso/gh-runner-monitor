// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package ui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexso/gh-runner-monitor/internal/monitor"
	"github.com/charmbracelet/x/ansi"
)

type Fetch func(context.Context) (monitor.State, error)
type result struct {
	state monitor.State
	err   error
}
type tick time.Time
type opened struct{ err error }
type Model struct {
	ctx           context.Context
	fetch         Fetch
	interval      time.Duration
	state         monitor.State
	err           string
	loading, help bool
	helpOffset    int
	focus         int
	selection     [2]int
	width, height int
}

func New(ctx context.Context, fetch Fetch, interval time.Duration) Model {
	return Model{ctx: ctx, fetch: fetch, interval: interval, width: 80, height: 24, loading: true}
}
func (m Model) load() tea.Cmd { return func() tea.Msg { s, e := m.fetch(m.ctx); return result{s, e} } }
func pulse() tea.Cmd          { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tick(t) }) }
func (m Model) Init() tea.Cmd { return tea.Batch(m.load(), pulse()) }
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
	case tick:
		if m.interval > 0 && !m.loading && m.err == "" && (m.state.Updated.IsZero() || time.Since(m.state.Updated) >= m.interval) {
			m.loading = true
			return m, tea.Batch(m.load(), pulse())
		}
		return m, pulse()
	case result:
		m.loading = false
		if x.err != nil {
			m.err = x.err.Error() // Retry at normal cadence, not every UI tick.
			return m, m.retry()
		}
		ids := [2]string{m.selectedID(0), m.selectedID(1)}
		m.state = x.state
		m.err = ""
		for f := 0; f < 2; f++ {
			for i := 0; i < m.count(f); i++ {
				if m.id(f, i) == ids[f] {
					m.selection[f] = i
					break
				}
			}
			m.selection[f] = min(m.selection[f], max(0, m.count(f)-1))
		}
	case retry:
		if m.interval > 0 && !m.loading && m.err != "" {
			m.loading = true
			return m, m.load()
		}
	case opened:
		if x.err != nil {
			m.err = "Could not open browser: " + x.err.Error()
			return m, m.retry()
		}
	case tea.KeyPressMsg:
		if m.help {
			switch x.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "?", "esc":
				m.help = false
				m.helpOffset = 0
			case "down", "j":
				m.helpOffset = min(m.helpOffset+1, max(0, len(m.helpLines())-m.helpRows()))
			case "up", "k":
				m.helpOffset = max(0, m.helpOffset-1)
			}
			return m, nil
		}
		switch x.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if !m.loading {
				m.loading = true
				return m, m.load()
			}
		case "tab":
			m.focus = 1 - m.focus
		case "?":
			m.help = true
			m.helpOffset = 0
		case "up", "k":
			m.selection[m.focus] = max(0, m.selection[m.focus]-1)
		case "down", "j":
			m.selection[m.focus] = min(max(0, m.count(m.focus)-1), m.selection[m.focus]+1)
		case "enter":
			u := m.selectedURL()
			if u != "" {
				return m, func() tea.Msg { return opened{openURL(m.ctx, u)} }
			}
		}
	}
	return m, nil
}

// Manual-only mode must not schedule retries, including after an initial failure.
func (m Model) retry() tea.Cmd {
	if m.interval <= 0 {
		return nil
	}
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return retry{} })
}

type retry struct{}

func (m Model) count(f int) int {
	if f == 0 {
		return len(m.state.Runners)
	}
	return len(m.state.Queue)
}
func (m Model) id(f, i int) string {
	if f == 0 {
		return m.state.Runners[i].Name
	}
	j := m.state.Queue[i]
	return fmt.Sprintf("%s/%d", j.Repository, j.ID)
}
func (m Model) selectedID(f int) string {
	if m.selection[f] >= m.count(f) {
		return ""
	}
	return m.id(f, m.selection[f])
}
func (m Model) selectedURL() string {
	if m.count(m.focus) == 0 {
		return ""
	}
	i := m.selection[m.focus]
	if m.focus == 1 {
		return m.state.Queue[i].URL
	}
	if j := m.state.Runners[i].Job; j != nil {
		return j.URL
	}
	return ""
}
func openURL(ctx context.Context, s string) error {
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" {
		return fmt.Errorf("unsupported job URL")
	}
	name := "xdg-open"
	args := []string{s}
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	if runtime.GOOS == "windows" {
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", s}
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 128 && r < 160) {
			return ' '
		}
		return r
	}, s)
}
func fit(s string, w int) string { return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(safe(s)) }

// tableRow clips by terminal cell width, so Unicode names cannot shift columns.
func tableRow(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		value := ansi.Truncate(safe(cells[i]), width, "…")
		parts[i] = value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
	}
	return strings.Join(parts, "  ")
}

func (m Model) panel(f, w, h int) string {
	title := "Runners"
	if f == 1 {
		title = "Job queue"
	}
	title = fmt.Sprintf("%s: %d", title, m.count(f))
	if m.focus == f {
		title = "▸ " + title
	}
	contentWidth := max(1, w-4)
	// Reserve two cells for the selection marker and two between each column.
	available := max(4, contentWidth-8)
	var headers []string
	var widths []int
	if f == 0 {
		nameWidth := 10
		for _, r := range m.state.Runners {
			nameWidth = max(nameWidth, lipgloss.Width(safe(r.Name)))
		}
		nameWidth = min(nameWidth, 20, max(4, available/4))
		statusWidth := min(8, max(3, available/5))
		durationWidth := min(10, max(3, available/5))
		widths = []int{nameWidth, statusWidth, durationWidth, max(1, available-nameWidth-statusWidth-durationWidth)}
		headers = []string{"NAME", "STATUS", "DURATION", "REPOSITORY / BRANCH / JOB"}
	} else {
		statusWidth := min(6, max(3, available/6))
		ageWidth := min(10, max(3, available/5))
		repoWidth := min(26, max(4, (available-statusWidth-ageWidth)/3))
		widths = []int{statusWidth, ageWidth, repoWidth, max(1, available-statusWidth-ageWidth-repoWidth)}
		headers = []string{"STATUS", "AGE", "REPOSITORY", "BRANCH / WORKFLOW / JOB"}
	}
	lines := []string{accent.Render(fit(title, contentWidth)), lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render(fit("  "+tableRow(headers, widths), contentWidth))}
	n := m.count(f)
	rows := max(1, h-4) // borders and two header lines
	start := max(0, m.selection[f]-rows+1)
	now := time.Now()
	for i := start; i < min(n, start+rows); i++ {
		prefix := "  "
		selected := i == m.selection[f] && m.focus == f
		if selected {
			prefix = "› "
		}
		var cells []string
		status := "Queued"
		if f == 0 {
			r := m.state.Runners[i]
			status = r.Status
			displayStatus := status
			if status == "Unknown" {
				displayStatus = "—"
			}
			duration, details := "—", "—"
			if r.Job != nil {
				duration = monitor.Age(now, r.Job.Started)
				branch := r.Job.Branch
				if branch == "" {
					branch = "—"
				}
				details = r.Job.Repository + " / " + branch + " / " + r.Job.Name
			}
			cells = []string{r.Name, displayStatus, duration, details}
		} else {
			j := m.state.Queue[i]
			branch := j.Branch
			if branch == "" {
				branch = "—"
			}
			details := branch + " / " + j.Workflow + " / " + j.Name
			if j.Group != "" {
				details += " (group: " + j.Group + ")"
			}
			cells = []string{"Queued", "~" + monitor.Age(now, j.Created), j.Repository, details}
		}
		color := "245"
		switch status {
		case "Running":
			color = "78"
		case "Idle":
			color = "75"
		case "Offline":
			color = "174"
		case "Queued":
			color = "180"
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(selected)
		lines = append(lines, style.Render(fit(prefix+tableRow(cells, widths), contentWidth)))
	}
	if n == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("  No entries"))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(max(2, w)).Height(max(2, h)).Render(strings.Join(lines, "\n"))
}
func refreshedAgo(now, updated time.Time) string {
	if updated.IsZero() {
		return "never"
	}
	seconds := max(0, int(now.Sub(updated).Seconds()))
	if seconds < 60 {
		return fmt.Sprintf("%ds ago", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm ago", seconds/60)
	}
	return fmt.Sprintf("%dh ago", seconds/3600)
}

func (m Model) helpLines() []string {
	body := "q           Quit\nr           Refresh now\nTab         Switch panel\n↑ / k       Move up\n↓ / j       Move down\nEnter       Open selected job\n? / Esc     Close help\n\n— Runner status is unknown.\nQueue age is approximate; it is not scheduling order. Unassigned job groups are unknown.\nCounts cover monitored repositories; inventory may include other busy runners.\nWithout organization runner access, running jobs are still monitored but idle/offline status cannot be verified."
	width := max(1, min(58, m.width-2)-4)
	return strings.Split(lipgloss.NewStyle().Width(width).Render(body), "\n")
}
func (m Model) helpRows() int { return max(1, m.height-8) }
func (m Model) helpPopup() string {
	lines := m.helpLines()
	start := min(m.helpOffset, max(0, len(lines)-m.helpRows()))
	end := min(len(lines), start+m.helpRows())
	body := accent.Render("Keyboard shortcuts") + "\n\n" + strings.Join(lines[start:end], "\n") + "\n\n? / Esc close"
	if len(lines) > m.helpRows() {
		body += " · ↑↓ scroll"
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("75")).Background(lipgloss.Color("235")).Padding(0, 1).Width(max(4, min(58, m.width-2))).Render(body)
}

// overlay keeps the dashboard visible while drawing a centered, opaque dialog.
func (m Model) overlay(content, popup string) string {
	width, height := max(1, m.width), max(1, m.height)
	canvas := lipgloss.NewCanvas(width, height)
	dimmed := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(ansi.Strip(content))
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(dimmed),
		lipgloss.NewLayer(popup).X(max(0, (width-lipgloss.Width(popup))/2)).Y(max(0, (height-lipgloss.Height(popup))/2)).Z(1),
	))
	return canvas.Render()
}
func (m Model) loadingScreen() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸"}
	label := frames[time.Now().Unix()%int64(len(frames))] + " Loading dashboard…"
	width, height := max(1, m.width), max(1, m.height)
	return accent.Render(fit("gh-runner-monitor", width)) + "\n" +
		lipgloss.Place(width, max(1, height-2), lipgloss.Center, lipgloss.Center, accent.Render(fit(label, width))) + "\n" +
		fit("q quit · ? help", width)
}

func (m Model) View() tea.View {
	running, idle := 0, 0
	for _, r := range m.state.Runners {
		if r.Status == "Running" {
			running++
		}
		if r.Status == "Idle" {
			idle++
		}
	}
	idleText := "Unknown"
	if m.state.Inventory {
		idleText = fmt.Sprint(idle)
	}
	last := "never"
	if !m.state.Updated.IsZero() {
		last = m.state.Updated.Local().Format("15:04:05") + " (" + refreshedAgo(time.Now(), m.state.Updated) + ")"
	}
	summary := fmt.Sprintf("Running: %d · Idle: %s · Queued: %d", running, idleText, len(m.state.Queue))
	freshness := "Updated: " + last
	if m.state.Updated.IsZero() {
		summary = "Running: Unknown · Idle: Unknown · Queued: Unknown"
	}
	if !m.state.Inventory && !m.state.Updated.IsZero() {
		summary = strings.Replace(summary, "Running: ", "Observed running: ", 1)
	}
	if m.interval == 0 {
		freshness += " · Manual refresh"
	}
	if m.loading {
		freshness += " · Refreshing…"
	}
	notes := "Queue age is approximate; group may be Unknown. ? help"
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	if m.state.Warning != "" {
		notes = strings.ReplaceAll(m.state.Warning, "Runner inventory restricted; unverified runners are Unknown", "Idle/offline status unknown (limited GitHub permissions)")
	}
	if m.err != "" {
		notes = "STALE · " + m.err
		noteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
	}
	footer := "q quit · r refresh · Tab focus · ↑↓/jk move · Enter open · ? help"
	h := max(5, m.height-7)
	var panels string
	if m.height >= 30 {
		panels = m.panel(0, m.width, h/2) + "\n" + m.panel(1, m.width, h-h/2)
	} else {
		panels = m.panel(m.focus, m.width, h)
	}
	content := accent.Render("gh-runner-monitor") + "\n" + fit(summary, m.width) + "\n" + fit(freshness, m.width) + "\n" + panels + "\n" + noteStyle.Render(fit(notes, m.width)) + "\n" + fit(footer, m.width)
	if m.help {
		content = m.overlay(content, m.helpPopup())
	} else if m.loading && m.state.Updated.IsZero() {
		content = m.loadingScreen()
	}
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	return v
}
