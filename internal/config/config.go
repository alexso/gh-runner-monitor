// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	GitHub struct {
		Organization string   `toml:"organization"`
		Account      string   `toml:"account"`
		Repositories []string `toml:"repositories"`
		RunnerGroup  string   `toml:"runner_group"`
		Runners      []string `toml:"runners"`
	} `toml:"github"`
	Display struct {
		RefreshInterval string `toml:"refresh_interval"`
		QueueSort       string `toml:"queue_sort"`
	} `toml:"display"`
}

func DefaultPath() (string, error) {
	var p string
	var e error
	if runtime.GOOS == "windows" {
		p, e = os.UserConfigDir()
	} else {
		p = os.Getenv("XDG_CONFIG_HOME")
		if p == "" {
			var home string
			home, e = os.UserHomeDir()
			p = filepath.Join(home, ".config")
		}
	}
	if e != nil {
		return "", e
	}
	return filepath.Join(p, "gh-runner-monitor", "config.toml"), nil
}
func Load(path string) (Config, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Config{}, fmt.Errorf("read config %s: %w (run gh-runner-monitor init)", path, e)
	}
	return Parse(b)
}
func Parse(b []byte) (Config, error) {
	var c Config
	if e := toml.Unmarshal(b, &c); e != nil {
		return c, fmt.Errorf("invalid TOML: %w", e)
	}
	if c.Display.RefreshInterval == "" {
		c.Display.RefreshInterval = "2m"
	}
	if c.Display.QueueSort == "" {
		c.Display.QueueSort = "oldest"
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	if !valid.MatchString(c.GitHub.Organization) || len(c.GitHub.Repositories) == 0 {
		return c, fmt.Errorf("github.organization and a nonempty github.repositories list are required")
	}
	seen := map[string]bool{}
	for _, r := range c.GitHub.Repositories {
		if !valid.MatchString(r) || r == "." || r == ".." || seen[r] {
			return c, fmt.Errorf("invalid or duplicate repository %q: use repository names without owner", r)
		}
		seen[r] = true
	}
	d, e := time.ParseDuration(c.Display.RefreshInterval)
	if e != nil || (d != 0 && d < time.Minute) {
		return c, fmt.Errorf("display.refresh_interval must be 0s (manual only) or a duration of at least 1m, such as 2m or 1h")
	}
	if c.Display.QueueSort != "oldest" {
		return c, fmt.Errorf("display.queue_sort must be oldest")
	}
	return c, nil
}
func (c Config) Interval() time.Duration {
	d, _ := time.ParseDuration(c.Display.RefreshInterval)
	return d
}

const Example = `[github]
# Required: GitHub organization login, for example "my-organization".
organization = ""

# Optional: existing gh account username. Empty uses the active gh account.
account = ""

# Required: repository names without owner, for example ["project-a", "project-b"].
# Wildcards are not supported.
repositories = []

# Optional: exact runner group name. Empty monitors all groups.
# Jobs with unknown groups remain visible.
runner_group = ""

# Optional: exact known runner names, for example ["runner-1", "runner-2"].
# These remain visible as Unknown when their status cannot be verified.
# Wildcards are not supported.
runners = []

[display]
# Automatic refresh: at least 1m. Units: s = seconds, m = minutes, h = hours.
# Examples: "120s", "2m", "1h". Default: "2m".
# "0s" fetches once at startup, then only when you press r (no automatic retries).
refresh_interval = "2m"

# Queue sorting: "oldest" is the only supported value (and the default).
# Uses job creation time, not GitHub's exact scheduling order.
queue_sort = "oldest"
`

func Init(path string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fmt.Errorf("create config (existing files are never overwritten): %w", e)
	}
	_, e = f.WriteString(Example)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
