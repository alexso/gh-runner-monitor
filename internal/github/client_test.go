// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package github

import (
	"context"
	"fmt"
	"github.com/alexso/gh-runner-monitor/internal/config"
	"strings"
	"testing"
)

func TestAccount(t *testing.T) {
	t.Setenv("GH_TOKEN", "wrong")
	t.Setenv("GITHUB_TOKEN", "also-wrong")
	calls := 0
	client, e := New(context.Background(), "alice", func(_ context.Context, args, env []string) ([]byte, error) {
		calls++
		a := strings.Join(args, " ")
		if strings.Contains(a, "switch") {
			t.Fatal(a)
		}
		joined := strings.Join(env, "\n")
		if strings.Contains(joined, "wrong") {
			t.Fatal("inherited token")
		}
		if args[0] == "auth" {
			if !strings.Contains(a, "--user alice") {
				t.Fatal(a)
			}
			return []byte("secret\n"), nil
		}
		if !strings.Contains(joined, "GH_TOKEN=secret") {
			t.Fatal("credential missing")
		}
		return []byte(`{"login":"alice"}`), nil
	})
	if e != nil || calls != 2 {
		t.Fatal(e, calls)
	}
	if client.Login() != "alice" {
		t.Fatal("incorrect authenticated account")
	}
}
func TestInvalidAuth(t *testing.T) {
	for _, failToken := range []bool{true, false} {
		_, e := New(context.Background(), "", func(_ context.Context, args, env []string) ([]byte, error) {
			if args[0] == "auth" && !failToken {
				return []byte("secret"), nil
			}
			return nil, fmt.Errorf("missing gh or invalid secret")
		})
		if e == nil || strings.Contains(e.Error(), "invalid secret") {
			t.Fatal(e)
		}
	}
}
func TestFetchFallbackAndJobs(t *testing.T) {
	cfg, _ := config.Parse([]byte("[github]\norganization='org'\nrepositories=['repo']"))
	seenProgress := false
	c := &Client{execute: func(_ context.Context, args, env []string) ([]byte, error) {
		p := args[len(args)-1]
		switch {
		case strings.HasPrefix(p, "orgs/"):
			return nil, fmt.Errorf("HTTP 403 forbidden")
		case strings.Contains(p, "/jobs?"):
			return []byte(`{"jobs":[{"id":4,"head_branch":"feature/test","status":"queued","created_at":"2026-01-01T00:00:00Z"},{"id":5,"status":"in_progress","runner_name":"r1","runner_group_name":"g","labels":["self-hosted"]},{"id":6,"status":"completed"}]}`), nil
		case strings.Contains(p, "status=in_progress"):
			seenProgress = true
			return []byte(`{"workflow_runs":[{"id":1,"name":"CI","head_branch":"main"}]}`), nil
		default:
			return []byte(`{"workflow_runs":[]}`), nil
		}
	}}
	d, e := c.Fetch(context.Background(), cfg)
	if e != nil || d.Inventory || d.Warning == "" || len(d.Jobs) != 2 || !seenProgress {
		t.Fatalf("%+v %v", d, e)
	}
	if d.Jobs[0].Created.IsZero() || d.Jobs[1].Group != "g" || d.Jobs[0].Workflow != "CI" || d.Jobs[0].Branch != "feature/test" || d.Jobs[1].Branch != "main" {
		t.Fatal(d.Jobs)
	}
}
func TestPagination(t *testing.T) {
	c := &Client{execute: func(_ context.Context, args, env []string) ([]byte, error) {
		p := args[len(args)-1]
		if strings.HasPrefix(p, "orgs/") {
			if strings.Contains(p, "&page=1") {
				return []byte(`{"runners":[` + strings.TrimSuffix(strings.Repeat(`{"name":"r","status":"online"},`, 100), ",") + `]}`), nil
			}
			return []byte(`{"runners":[{"name":"last","status":"offline"}]}`), nil
		}
		return []byte(`{"workflow_runs":[]}`), nil
	}}
	cfg, _ := config.Parse([]byte("[github]\norganization='org'\nrepositories=['repo']"))
	d, e := c.Fetch(context.Background(), cfg)
	if e != nil || len(d.Runners) != 101 || !d.Inventory {
		t.Fatal(len(d.Runners), e)
	}
}

func TestMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Run(context.Background(), []string{"auth", "token"}, nil)
	if err == nil || !strings.Contains(err.Error(), "GitHub CLI (gh) is missing") {
		t.Fatal(err)
	}
}
func TestRateLimitIsNotPermission(t *testing.T) {
	if permission(fmt.Errorf("HTTP 403 API rate limit exceeded")) {
		t.Fatal("rate limit treated as permission denial")
	}
}

func TestMismatchedAccount(t *testing.T) {
	_, err := New(context.Background(), "alice", func(_ context.Context, args, env []string) ([]byte, error) {
		if args[0] == "auth" {
			return []byte("secret"), nil
		}
		return []byte(`{"login":"bob"}`), nil
	})
	if err == nil || !strings.Contains(err.Error(), "authenticated as") {
		t.Fatal(err)
	}
}
