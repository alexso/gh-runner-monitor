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
			m.help = !m.help
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
func (m Model) panel(f, w, h int) string {
	title := "Runners"
	header := "NAME · STATUS · REPOSITORY / JOB · DURATION"
	if f == 1 {
		title = "Job queue"
		header = "REPOSITORY · WORKFLOW / JOB · STATUS · AGE"
	}
	if m.focus == f {
		title = "▸ " + title
	}
	lines := []string{accent.Render(title), fit(header, w-4)}
	n := m.count(f)
	rows := max(1, h-5) // borders, two header lines, and entry count
	start := max(0, m.selection[f]-rows+1)
	now := time.Now()
	for i := start; i < min(n, start+rows); i++ {
		prefix := "  "
		if i == m.selection[f] {
			prefix = "› "
		}
		var line, status string
		if f == 0 {
			r := m.state.Runners[i]
			status = r.Status
			displayStatus := r.Status
			if displayStatus == "Unknown" {
				displayStatus = "—"
			}
			line = r.Name + " · " + displayStatus
			if r.Job != nil {
				line += " · " + r.Job.Repository + " / " + r.Job.Name + " · " + monitor.Age(now, r.Job.Started)
			}
		} else {
			j := m.state.Queue[i]
			status = "Queued"
			group := j.Group
			if group == "" {
				group = "Unknown"
			}
			line = j.Repository + " · " + j.Workflow + " / " + j.Name + " · Queued · ~" + monitor.Age(now, j.Created) + " · group: " + group
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
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(fit(prefix+line, w-4)))
	}
	if n == 0 {
		lines = append(lines, "  No entries")
	}
	lines = append(lines, fit(fmt.Sprintf("%d entries", n), w-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(max(1, w-2)).Height(max(1, h-2)).Render(strings.Join(lines, "\n"))
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
		last = m.state.Updated.Local().Format("15:04:05")
	}
	summary := fmt.Sprintf("Running: %d · Idle: %s · Queued: %d · Updated: %s", running, idleText, len(m.state.Queue), last)
	if m.state.Updated.IsZero() {
		summary = "Running: Unknown · Idle: Unknown · Queued: Unknown · Updated: never"
	}
	if !m.state.Inventory && !m.state.Updated.IsZero() {
		summary = strings.Replace(summary, "Running: ", "Observed running: ", 1)
	}
	if m.interval == 0 {
		summary += " · Manual refresh"
	}
	if m.loading {
		summary += " · Refreshing…"
	}
	notes := "Queue age is approximate; group may be Unknown. ? help"
	if m.state.Warning != "" {
		notes = strings.ReplaceAll(m.state.Warning, "Runner inventory restricted; unverified runners are Unknown", "Runner inventory access restricted")
	}
	if m.err != "" {
		notes = "STALE · " + m.err
	}
	footer := "q quit · r refresh · Tab focus · ↑↓/jk move · Enter open · ? help"
	if m.help {
		footer += "\nQueued jobs can target unknown groups or hosted runners. Counts cover monitored repositories."
	}
	h := max(5, m.height-7)
	var panels string
	if m.width >= 110 {
		left := m.width / 2
		panels = lipgloss.JoinHorizontal(lipgloss.Top, m.panel(0, left, h), m.panel(1, m.width-left, h))
	} else if m.height >= 30 {
		panels = m.panel(0, m.width, h/2) + "\n" + m.panel(1, m.width, h-h/2)
	} else {
		panels = m.panel(m.focus, m.width, h)
	}
	content := accent.Render("gh-runner-monitor") + "\n" + fit(summary, m.width) + "\n" + panels + "\n" + fit(notes, m.width) + "\n" + fit(footer, m.width)
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	return v
}
