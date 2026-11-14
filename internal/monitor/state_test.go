// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package monitor

import (
	"github.com/alexso/gh-runner-monitor/internal/config"
	gh "github.com/alexso/gh-runner-monitor/internal/github"
	"testing"
	"time"
)

func TestState(t *testing.T) {
	c, _ := config.Parse([]byte("[github]\norganization='org'\nrepositories=['repo']\nrunners=['known','busy']"))
	now := time.Now()
	d := gh.Data{Jobs: []gh.Job{{ID: 2, Status: "queued", Created: now}, {ID: 1, Status: "queued", Created: now.Add(-time.Minute)}, {Status: "in_progress", Runner: "busy", Name: "build", Started: now.Add(-time.Minute)}}}
	s := Build(d, c, now)
	if s.Queue[0].ID != 1 || s.Runners[0].Status != "Running" || s.Runners[0].Job.Name != "build" || s.Runners[1].Status != "Unknown" {
		t.Fatal(s)
	}
	d.Inventory = true
	d.Runners = []gh.Runner{{Name: "known", Status: "online"}, {Name: "off", Status: "offline"}}
	s = Build(d, c, now)
	if s.Runners[1].Status != "Idle" || s.Runners[2].Status != "Offline" {
		t.Fatal(s)
	}
	if Age(now, now.Add(-time.Minute)) != "1m0s" || Age(now, time.Time{}) != "Unknown" || Age(now, now.Add(time.Minute)) != "0s" {
		t.Fatal("age")
	}
	d.Jobs[2].Group = "confirmed-group"
	c.GitHub.RunnerGroup = "other"
	s = Build(d, c, now)
	if s.Runners[0].Status != "Unknown" || len(s.Queue) != 2 {
		t.Fatal(s)
	}
}

func TestRunningAssignmentGroupFilter(t *testing.T) {
	for _, tc := range []struct {
		name, group, status string
		want                string
	}{
		{"missing group", "", "in_progress", "Running"},
		{"matching group", "team", "in_progress", "Running"},
		{"different group", "other", "in_progress", "Unknown"},
		{"completed job", "team", "completed", "Unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := config.Parse([]byte("[github]\norganization='org'\nrepositories=['repo']\nrunners=['runner']\nrunner_group='team'"))
			s := Build(gh.Data{Jobs: []gh.Job{{Status: tc.status, Runner: "runner", Group: tc.group, Repository: "repo", Name: "build"}}}, c, time.Now())
			if len(s.Runners) != 1 || s.Runners[0].Status != tc.want {
				t.Fatalf("%+v; want %s", s.Runners, tc.want)
			}
			if tc.want == "Running" && s.Runners[0].Job == nil {
				t.Fatal("assignment missing")
			}
		})
	}
}
