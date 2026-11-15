// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
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
	"github.com/charmbracelet/x/term"
)

var version = "dev"

func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, "gh-runner-monitor:", e)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	return runWithInput(args, os.Stdin, out, term.IsTerminal(os.Stdin.Fd()))
}

func runWithInput(args []string, in io.Reader, out io.Writer, interactive bool) error {
	in = bufio.NewReader(in)
	fs := flag.NewFlagSet("gh-runner-monitor", flag.ContinueOnError)
	fs.SetOutput(out)
	path := fs.String("config", "", "configuration file path")
	check := fs.Bool("check", false, "validate configuration, authentication, and API access")
	ver := fs.Bool("version", false, "print version")
	shortcut := fs.Bool("install-shortcut", false, "offer to install grm in ~/.local/bin (macOS/Linux), then exit")
	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: gh-runner-monitor [--config path] [--check] [--version] [--install-shortcut]\nMonitor GitHub Actions self-hosted runners and queued jobs.\nA missing configuration starts first-run setup automatically.")
		fs.PrintDefaults()
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
	if *shortcut {
		if !interactive {
			return fmt.Errorf("--install-shortcut requires an interactive terminal to confirm installation")
		}
		return offerShortcut(in, out)
	}
	defaultLocation := *path == ""
	selectedLocation := false
	if defaultLocation {
		p, e := config.ResolveDefaultPath()
		if e != nil {
			return e
		}
		*path = p
		if interactive && !*check {
			if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
				chosen, err := chooseConfigPath(in, out, p)
				if err != nil {
					return err
				}
				*path = chosen
				selectedLocation = true
			}
		}
	}
	created, err := ensureConfig(*path, out)
	if err != nil {
		return err
	}
	if defaultLocation && (created || selectedLocation) {
		if err := config.RememberPath(*path); err != nil {
			return fmt.Errorf("config is at %s but its location could not be saved: %w", *path, err)
		}
	}
	if created {
		if interactive && !*check {
			if err := offerShortcut(in, out); err != nil {
				fmt.Fprintln(out, "Shortcut was not installed:", err)
			}
		}
		if *check {
			return fmt.Errorf("configuration template created at %s; fill in github.organization and github.repositories before --check", *path)
		}
		return nil
	}

	cfg, e := config.Load(*path)
	if e != nil {
		return fmt.Errorf("configuration %s: %w; edit this file and try again", *path, e)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var client *gh.Client
	fetch := func(ctx context.Context) (monitor.State, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		// Authenticate inside the first asynchronous fetch so startup stays visible.
		if client == nil {
			authCtx, authCancel := context.WithTimeout(ctx, 30*time.Second)
			authenticated, err := gh.New(authCtx, cfg.GitHub.Account, nil)
			authCancel()
			if err != nil {
				return monitor.State{}, err
			}
			client = authenticated
			if *check {
				fmt.Fprintf(out, "Authenticated GitHub account: %s\n", client.Login())
			}
		}
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
				fmt.Fprintf(out, "Running job: %s / %s; runner=%q; group=%q; branch=%q%s\n", j.Repository, j.Name, runner, group, j.Branch, note)
			}
			fmt.Fprintf(out, "Observed in-progress jobs: %d\n", running)
		}
		return monitor.Build(d, cfg, time.Now()), nil
	}
	if *check {
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
