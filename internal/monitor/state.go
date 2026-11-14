// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package monitor

import (
	"github.com/alexso/gh-runner-monitor/internal/config"
	gh "github.com/alexso/gh-runner-monitor/internal/github"
	"sort"
	"time"
)

type Runner struct {
	Name, Status string
	Job          *gh.Job
}
type State struct {
	Runners   []Runner
	Queue     []gh.Job
	Inventory bool
	Warning   string
	Updated   time.Time
}

func Build(d gh.Data, c config.Config, now time.Time) State {
	s := State{Inventory: d.Inventory, Warning: d.Warning, Updated: now}
	runners := map[string]Runner{}
	for _, n := range c.GitHub.Runners {
		runners[n] = Runner{Name: n, Status: "Unknown"}
	}
	// A group inventory is authoritative only for runners actually returned in it.
	for _, r := range d.Runners {
		status := "Unknown"
		if r.Status == "offline" {
			status = "Offline"
		} else if r.Status == "online" {
			if r.Busy {
				status = "Running"
			} else {
				status = "Idle"
			}
		}
		runners[r.Name] = Runner{Name: r.Name, Status: status}
	}
	for _, j := range d.Jobs {
		if j.Status == "queued" {
			s.Queue = append(s.Queue, j)
			continue
		}
		if j.Status != "in_progress" || j.Runner == "" {
			continue
		}
		// Missing group metadata cannot disprove an observed assignment.
		if c.GitHub.RunnerGroup != "" && j.Group != "" && j.Group != c.GitHub.RunnerGroup {
			continue
		}
		// Known names, inventory membership or self-hosted labels distinguish hosted runners.
		_, known := runners[j.Runner]
		self := j.Group != ""
		for _, l := range j.Labels {
			if l == "self-hosted" {
				self = true
			}
		}
		if !known && !self {
			continue
		}
		job := j
		runners[j.Runner] = Runner{Name: j.Runner, Status: "Running", Job: &job}
	}
	for _, r := range runners {
		s.Runners = append(s.Runners, r)
	}
	sort.Slice(s.Runners, func(i, j int) bool { return s.Runners[i].Name < s.Runners[j].Name })
	sort.Slice(s.Queue, func(i, j int) bool {
		a, b := s.Queue[i], s.Queue[j]
		if a.Created.IsZero() != b.Created.IsZero() {
			return !a.Created.IsZero()
		}
		if a.Created.Equal(b.Created) {
			if a.Repository != b.Repository {
				return a.Repository < b.Repository
			}
			return a.ID < b.ID
		}
		return a.Created.Before(b.Created)
	})
	return s
}
func Age(now, start time.Time) string {
	if start.IsZero() {
		return "Unknown"
	}
	d := now.Sub(start)
	if d < 0 {
		d = 0
	}
	return d.Truncate(time.Second).String()
}
