// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

// Package github collects read-only Actions data through the authenticated gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/alexso/gh-runner-monitor/internal/config"
)

type Job struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Workflow   string    `json:"workflow_name"`
	Branch     string    `json:"head_branch"`
	Status     string    `json:"status"`
	URL        string    `json:"html_url"`
	Created    time.Time `json:"created_at"`
	Started    time.Time `json:"started_at"`
	Runner     string    `json:"runner_name"`
	Group      string    `json:"runner_group_name"`
	Labels     []string  `json:"labels"`
	Repository string
}
type Runner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}
type Data struct {
	Jobs      []Job
	Runners   []Runner
	Inventory bool
	Warning   string
}
type Execute func(context.Context, []string, []string) ([]byte, error)
type Client struct {
	execute Execute
	env     []string
	login   string
}

func Run(ctx context.Context, args, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if e := cmd.Run(); e != nil {
		if errors.Is(e, exec.ErrNotFound) {
			return nil, fmt.Errorf("GitHub CLI (gh) is missing; install it and run gh auth login")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("gh request failed: %s", strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}
func New(ctx context.Context, account string, execute Execute) (*Client, error) {
	if execute == nil {
		execute = Run
	}
	env := os.Environ()
	clean := []string{}
	for _, s := range env {
		if !strings.HasPrefix(s, "GH_TOKEN=") && !strings.HasPrefix(s, "GITHUB_TOKEN=") {
			clean = append(clean, s)
		}
	}
	env = clean
	args := []string{"auth", "token", "--hostname", "github.com"}
	if account != "" {
		args = append(args, "--user", account)
	}
	token, e := execute(ctx, args, env)
	if e != nil || len(bytes.TrimSpace(token)) == 0 {
		return nil, fmt.Errorf("cannot obtain gh credential for account %q; install GitHub CLI and run gh auth login --hostname github.com", account)
	}
	env = append(env, "GH_TOKEN="+strings.TrimSpace(string(token)))

	c := &Client{execute: execute, env: env}
	var user struct {
		Login string `json:"login"`
	}
	if e := c.get(ctx, "user", &user); e != nil {
		return nil, fmt.Errorf("GitHub authentication failed; run gh auth login --hostname github.com: %w", e)
	}
	if user.Login == "" {
		return nil, fmt.Errorf("GitHub authentication returned no user; run gh auth login")
	}
	if account != "" && !strings.EqualFold(account, user.Login) {
		return nil, fmt.Errorf("requested GitHub account %q but credential authenticated as %q", account, user.Login)
	}
	c.login = user.Login
	return c, nil
}

// Login returns the username confirmed by the GitHub user API.
func (c *Client) Login() string { return c.login }

func (c *Client) get(ctx context.Context, path string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	b, e := c.execute(ctx, []string{"api", "--hostname", "github.com", "--method", "GET", "-H", "Accept: application/vnd.github+json", path}, c.env)
	if e != nil { // Scrub even unexpected CLI diagnostics before exposing them.
		msg := e.Error()
		for _, s := range c.env {
			if strings.HasPrefix(s, "GH_TOKEN=") || strings.HasPrefix(s, "GITHUB_TOKEN=") {
				t := strings.SplitN(s, "=", 2)[1]
				if t != "" {
					msg = strings.ReplaceAll(msg, t, "[redacted]")
				}
			}
		}
		return fmt.Errorf("%s: %s (check Actions read access, SSO authorization, and API rate limits)", path, msg)
	}
	if e = json.Unmarshal(b, v); e != nil {
		return fmt.Errorf("decode %s: %w", path, e)
	}
	return nil
}
func permission(e error) bool {
	s := e.Error()
	if strings.Contains(strings.ToLower(strings.Split(s, " (check")[0]), "rate limit") {
		return false
	}
	return strings.Contains(s, "403") || strings.Contains(s, "404")
}
func (c *Client) Fetch(ctx context.Context, cfg config.Config) (Data, error) {
	var d Data
	inventoryPath := "orgs/" + url.PathEscape(cfg.GitHub.Organization) + "/actions/runners"
	if cfg.GitHub.RunnerGroup != "" {
		found := false
		for p := 1; ; p++ {
			var x struct {
				Groups []struct {
					ID     int64  `json:"id"`
					Name   string `json:"name"`
					Branch string `json:"head_branch"`
				} `json:"runner_groups"`
			}
			e := c.get(ctx, fmt.Sprintf("orgs/%s/actions/runner-groups?per_page=100&page=%d", url.PathEscape(cfg.GitHub.Organization), p), &x)
			if e != nil {
				if permission(e) {
					d.Warning = "Runner inventory restricted; unverified runners are Unknown"
					break
				}
				return d, e
			}
			for _, g := range x.Groups {
				if g.Name == cfg.GitHub.RunnerGroup {
					inventoryPath = fmt.Sprintf("orgs/%s/actions/runner-groups/%d/runners", url.PathEscape(cfg.GitHub.Organization), g.ID)
					found = true
				}
			}
			if len(x.Groups) < 100 {
				if !found {
					d.Warning = "Runner group not found; inventory unavailable"
				}
				break
			}
		}
	}
	if d.Warning == "" {
		d.Inventory = true
		for p := 1; ; p++ {
			var x struct {
				Runners []Runner `json:"runners"`
			}
			e := c.get(ctx, fmt.Sprintf("%s?per_page=100&page=%d", inventoryPath, p), &x)
			if e != nil {
				if permission(e) {
					d.Inventory = false
					d.Runners = nil
					d.Warning = "Runner inventory restricted; unverified runners are Unknown"
					break
				}
				return d, e
			}
			d.Runners = append(d.Runners, x.Runners...)
			if len(x.Runners) < 100 {
				break
			}
		}
	}
	var mu sync.Mutex
	var first error
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, repo := range cfg.GitHub.Repositories {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				if first == nil {
					first = ctx.Err()
				}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			jobs, e := c.repository(ctx, cfg.GitHub.Organization, repo)
			mu.Lock()
			defer mu.Unlock()
			if e != nil && first == nil {
				first = fmt.Errorf("repository %s: %w", repo, e)
			}
			d.Jobs = append(d.Jobs, jobs...)
		}(repo)
	}
	wg.Wait()
	return d, first
}
func (c *Client) repository(ctx context.Context, org, repo string) ([]Job, error) {
	base := "repos/" + url.PathEscape(org) + "/" + url.PathEscape(repo) + "/actions"
	seen := map[int64]bool{}
	var jobs []Job
	for _, status := range []string{"queued", "in_progress", "waiting", "pending", "requested"} {
		for p := 1; ; p++ {
			var runs struct {
				Runs []struct {
					ID     int64  `json:"id"`
					Name   string `json:"name"`
					Branch string `json:"head_branch"`
				} `json:"workflow_runs"`
			}
			if e := c.get(ctx, fmt.Sprintf("%s/runs?status=%s&per_page=100&page=%d", base, status, p), &runs); e != nil {
				return nil, e
			}
			for _, run := range runs.Runs {
				if seen[run.ID] {
					continue
				}
				seen[run.ID] = true
				for jp := 1; ; jp++ {
					var x struct {
						Jobs []Job `json:"jobs"`
					}
					if e := c.get(ctx, fmt.Sprintf("%s/runs/%d/jobs?filter=latest&per_page=100&page=%d", base, run.ID, jp), &x); e != nil {
						return nil, e
					}
					for _, j := range x.Jobs {
						if j.Status == "queued" || j.Status == "in_progress" {
							j.Repository = repo
							if j.Branch == "" {
								j.Branch = run.Branch
							}
							if j.Workflow == "" {
								j.Workflow = run.Name
							}
							jobs = append(jobs, j)
						}
					}
					if len(x.Jobs) < 100 {
						break
					}
				}
			}
			if len(runs.Runs) < 100 {
				break
			}
		}
	}
	return jobs, nil
}
