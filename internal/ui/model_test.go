// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	gh "github.com/alexso/gh-runner-monitor/internal/github"
	"github.com/alexso/gh-runner-monitor/internal/monitor"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"time"
)

func TestDashboard(t *testing.T) {
	m := New(context.Background(), func(context.Context) (monitor.State, error) { return monitor.State{}, nil }, time.Second)
	m.state = monitor.State{Updated: time.Now(), Runners: []monitor.Runner{{Name: "runner-one", Status: "Unknown"}}, Queue: []gh.Job{{ID: 2, Repository: "repo", Name: "test", Status: "queued", Created: time.Now()}}}
	m.loading = false
	for _, size := range []tea.WindowSizeMsg{{Width: 140, Height: 40}, {Width: 80, Height: 35}, {Width: 60, Height: 20}, {Width: 20, Height: 8}} {
		updated, _ := m.Update(size)
		m = updated.(Model)
		v := m.View()
		if v.Content == "" {
			t.Fatal("empty view")
		}
	}
	m.width = 120
	m.height = 35
	if !strings.Contains(m.View().Content, "Unknown") {
		t.Fatal("missing unknown")
	}
	m.focus = 1
	m.selection[1] = 0
	updated, _ := m.Update(result{state: monitor.State{Queue: []gh.Job{{ID: 1, Repository: "repo"}, {ID: 2, Repository: "repo"}}}})
	m = updated.(Model)
	if m.selection[1] != 1 {
		t.Fatal("selection lost")
	}
	updated, _ = m.Update(result{err: context.DeadlineExceeded})
	m = updated.(Model)
	if len(m.state.Queue) != 2 || !strings.Contains(m.View().Content, "STALE") {
		t.Fatal("stale data lost")
	}
}

func TestManualRefresh(t *testing.T) {
	calls := 0
	m := New(context.Background(), func(context.Context) (monitor.State, error) { calls++; return monitor.State{Updated: time.Now()}, nil }, 0)
	// Init schedules the initial fetch along with the clock used to update job ages.
	initial := m.Init()().(tea.BatchMsg)
	var cmd tea.Cmd
	model, cmd := m.Update(initial[0]())
	m = model.(Model)
	if calls != 1 || m.loading || cmd != nil {
		t.Fatal("initial fetch failed")
	}
	model, _ = m.Update(tick(time.Now().Add(time.Hour)))
	m = model.(Model)
	if m.loading || calls != 1 {
		t.Fatal("manual mode refreshed on tick")
	}
	model, cmd = m.Update(result{err: context.DeadlineExceeded})
	m = model.(Model)
	if cmd != nil {
		t.Fatal("manual mode scheduled automatic retry")
	}
	model, cmd = m.Update(retry{})
	m = model.(Model)
	if cmd != nil || m.loading {
		t.Fatal("manual mode accepted automatic retry")
	}
	model, cmd = m.Update(tea.KeyPressMsg{Code: 'r'})
	m = model.(Model)
	if cmd == nil || !m.loading {
		t.Fatal("manual refresh key did not fetch")
	}
	model, _ = m.Update(cmd())
	m = model.(Model)
	if calls != 2 || m.loading || m.err != "" {
		t.Fatal("manual refresh did not recover")
	}
}
func TestAutomaticRefresh(t *testing.T) {
	m := New(context.Background(), func(context.Context) (monitor.State, error) { return monitor.State{}, nil }, time.Minute)
	m.loading = false
	m.state.Updated = time.Now().Add(-2 * time.Minute)
	updated, cmd := m.Update(tick(time.Now()))
	if !updated.(Model).loading || cmd == nil {
		t.Fatal("automatic refresh not scheduled")
	}
	_, cmd = m.Update(result{err: context.DeadlineExceeded})
	if cmd == nil {
		t.Fatal("automatic retry not scheduled")
	}
}

func TestTableColumnsAndPanelBounds(t *testing.T) {
	// A long Unicode value must not push the following fixed-width column.
	first := tableRow([]string{"long界界界界name", "Running", "1m0s", "project / build"}, []int{10, 8, 10, 20})
	second := tableRow([]string{"r1", "—", "—", "—"}, []int{10, 8, 10, 20})
	if lipgloss.Width(first) != 54 || lipgloss.Width(second) != 54 || !strings.Contains(first, "…") {
		t.Fatalf("unexpected cell widths: %q / %q", first, second)
	}
	m := New(context.Background(), nil, time.Minute)
	m.state.Runners = []monitor.Runner{{Name: "runner-1", Status: "Running", Job: &gh.Job{Repository: strings.Repeat("project-", 20), Name: strings.Repeat("build-", 20), Started: time.Now()}}}
	m.state.Queue = []gh.Job{{Repository: strings.Repeat("project-", 20), Workflow: "CI", Name: strings.Repeat("test-", 20), Created: time.Now()}}
	for _, width := range []int{40, 60, 100, 160} {
		for focus := 0; focus < 2; focus++ {
			m.focus = focus
			panel := m.panel(focus, width, 12)
			if lipgloss.Width(panel) > width || lipgloss.Height(panel) != 12 {
				t.Fatalf("panel %d at width %d: %dx%d", focus, width, lipgloss.Width(panel), lipgloss.Height(panel))
			}
			title := "Runners: 1"
			if focus == 1 {
				title = "Job queue: 1"
			}
			if !strings.Contains(panel, title) || strings.Contains(panel, "entries") || !strings.Contains(panel, "…") {
				t.Fatalf("unexpected panel: %s", panel)
			}
		}
	}
}

func TestRefreshedAgo(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		elapsed time.Duration
		want    string
	}{{0, "0s ago"}, {42 * time.Second, "42s ago"}, {90 * time.Second, "1m ago"}, {2 * time.Hour, "2h ago"}, {-time.Minute, "0s ago"}} {
		if got := refreshedAgo(now, now.Add(-tc.elapsed)); got != tc.want {
			t.Fatalf("%v: got %q want %q", tc.elapsed, got, tc.want)
		}
	}
	if refreshedAgo(now, time.Time{}) != "never" {
		t.Fatal("zero timestamp")
	}
}

func TestHelpPopup(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 12}, {Width: 60, Height: 20}, {Width: 100, Height: 35}} {
		m := New(context.Background(), nil, time.Minute)
		m.width, m.height = size.Width, size.Height
		updated, _ := m.Update(tea.KeyPressMsg{Code: '?'})
		m = updated.(Model)
		if !m.help {
			t.Fatal("help did not open")
		}
		popup := m.helpPopup()
		if lipgloss.Width(popup) > m.width || lipgloss.Height(popup) > m.height {
			t.Fatalf("popup %dx%d exceeds %dx%d", lipgloss.Width(popup), lipgloss.Height(popup), m.width, m.height)
		}
		if !strings.Contains(m.View().Content, "Keyboard shortcuts") {
			t.Fatal("overlay missing")
		}
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		m = updated.(Model)
		if m.selection[0] != 0 {
			t.Fatal("help navigation moved runner selection")
		}
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if updated.(Model).help {
			t.Fatal("escape did not close help")
		}
	}
}

func TestLoadingAndDimmedOverlay(t *testing.T) {
	m := New(context.Background(), nil, time.Minute)
	m.width, m.height = 100, 30
	initial := m.View().Content
	if !strings.Contains(initial, "Loading dashboard") || strings.Contains(initial, "Runners: 0") || !strings.Contains(initial, "q quit") || strings.Contains(initial, "╭") {
		t.Fatalf("initial loading screen should show the centered indicator, title, and quit hint: %q", ansi.Strip(initial))
	}
	linesAtStartup := strings.Split(ansi.Strip(initial), "\n")
	if len(linesAtStartup) != m.height || !strings.Contains(linesAtStartup[len(linesAtStartup)-1], "q quit") {
		t.Fatal("quit hint should remain on the bottom line")
	}
	updated, _ := m.Update(result{state: monitor.State{Updated: time.Now()}})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "Loading dashboard") {
		t.Fatal("initial loading indicator remained after success")
	}
	m.loading = true
	if strings.Contains(m.View().Content, "Loading dashboard") || !strings.Contains(m.View().Content, "Refreshing") {
		t.Fatal("later refresh should remain unobtrusive")
	}
	// Verify centering against an otherwise empty backdrop at a deterministic size.
	popup := "POPUP"
	plain := ansi.Strip(m.overlay("background", popup))
	lines := strings.Split(plain, "\n")
	expectedY := (m.height - 1) / 2
	expectedX := (m.width - len(popup)) / 2
	if len(lines) <= expectedY || strings.Index(lines[expectedY], popup) != expectedX || !strings.Contains(plain, "background") {
		t.Fatal("popup is not centered over the retained backdrop")
	}
}
