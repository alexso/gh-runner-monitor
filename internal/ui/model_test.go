// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	gh "github.com/alexso/gh-runner-monitor/internal/github"
	"github.com/alexso/gh-runner-monitor/internal/monitor"
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
