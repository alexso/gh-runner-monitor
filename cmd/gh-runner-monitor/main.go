// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alexso/gh-runner-monitor/internal/config"
	gh "github.com/alexso/gh-runner-monitor/internal/github"
	"github.com/alexso/gh-runner-monitor/internal/monitor"
	"github.com/alexso/gh-runner-monitor/internal/ui"
)

var version = "dev"

func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, "gh-runner-monitor:", e)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("gh-runner-monitor", flag.ContinueOnError)
	fs.SetOutput(out)
	path := fs.String("config", "", "configuration file path")
	check := fs.Bool("check", false, "validate configuration, authentication, and API access")
	ver := fs.Bool("version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: gh-runner-monitor [init] [--config path] [--check] [--version]\nMonitor GitHub Actions self-hosted runners and queued jobs.")
		fs.PrintDefaults()
	}
	init := len(args) > 0 && args[0] == "init"
	if init {
		args = args[1:]
	}
	if e := fs.Parse(args); e != nil {
		if e == flag.ErrHelp {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *ver {
		fmt.Fprintln(out, version)
		return nil
	}
	if *path == "" {
		p, e := config.DefaultPath()
		if e != nil {
			return e
		}
		*path = p
	}
	if init {
		if e := config.Init(*path); e != nil {
			return e
		}
		fmt.Fprintln(out, "Created", *path)
		fmt.Fprintln(out, "Edit github.organization and github.repositories before running.")
		return nil
	}
	cfg, e := config.Load(*path)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	authCtx, authCancel := context.WithTimeout(ctx, 30*time.Second)
	client, e := gh.New(authCtx, cfg.GitHub.Account, nil)
	authCancel()
	if e != nil {
		return e
	}
	fetch := func(ctx context.Context) (monitor.State, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		d, e := client.Fetch(ctx, cfg)
		if e != nil {
			return monitor.State{}, e
		}
		if *check {
			running := 0
			for _, j := range d.Jobs {
				if j.Status != "in_progress" {
					continue
				}
				running++
				runner, group := j.Runner, j.Group
				if runner == "" {
					runner = "Unknown"
				}
				if group == "" {
					group = "Unknown"
				}
				note := ""
				if cfg.GitHub.RunnerGroup != "" && j.Group != "" && j.Group != cfg.GitHub.RunnerGroup {
					note = " (excluded: runner group mismatch)"
				}
				fmt.Fprintf(out, "Running job: %s / %s; runner=%q; group=%q%s\n", j.Repository, j.Name, runner, group, note)
			}
			fmt.Fprintf(out, "Observed in-progress jobs: %d\n", running)
		}
		return monitor.Build(d, cfg, time.Now()), nil
	}
	if *check {
		fmt.Fprintf(out, "Authenticated GitHub account: %s\n", client.Login())
		s, e := fetch(ctx)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "OK: repository Actions access verified; %d queued jobs, %d known/discovered runners\n", len(s.Queue), len(s.Runners))
		if s.Warning != "" {
			fmt.Fprintln(out, s.Warning)
		}
		return nil
	}
	_, e = tea.NewProgram(ui.New(ctx, fetch, cfg.Interval()), tea.WithContext(ctx)).Run()
	return e
}
