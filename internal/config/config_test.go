// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParse(t *testing.T) {
	c, e := Parse([]byte("[github]\norganization='org'\nrepositories=['repo']"))
	if e != nil || c.Display.RefreshInterval != "2m" || c.Display.QueueSort != "oldest" {
		t.Fatalf("%+v %v", c, e)
	}
	for _, s := range []string{"bad = [", "[github]\norganization='org'", "[github]\norganization='org'\nrepositories=['a/b']", Example + "\nqueue_sort='newest'", "[github]\norganization='org'\nrepositories=['repo']\n[display]\nrefresh_interval='30s'"} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
func TestPathAndCreate(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", d)
	p, e := DefaultPath()
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS != "windows" && p != filepath.Join(d, "gh-runner-monitor", "config.toml") {
		t.Fatal(p)
	}
	p = filepath.Join(d, "nested", "config.toml")
	if e = Create(p); e != nil {
		t.Fatal(e)
	}
	if e = Create(p); e == nil {
		t.Fatal("overwrote config")
	}
	if _, e = Load(p); e == nil {
		t.Fatal("unedited template should require organization and repositories")
	}
	if _, e = Load(p + "missing"); e == nil {
		t.Fatal("missing accepted")
	}
	_ = os.Remove(p)
}

func TestDefaultPathWithoutXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses AppData")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := DefaultPath()
	want := filepath.Join(home, ".config", "gh-runner-monitor", "config.toml")
	if err != nil || got != want {
		t.Fatalf("DefaultPath() = %q, %v; want %q", got, err, want)
	}
}

func TestRefreshIntervals(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"0s", true}, {"1m", true}, {"2m", true}, {"120s", true}, {"1h", true},
		{"30s", false}, {"59s", false}, {"-1m", false}, {"invalid", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			c, err := Parse([]byte("[github]\norganization='org'\nrepositories=['repo']\n[display]\nrefresh_interval='" + tc.value + "'"))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
			if err == nil && tc.value == "0s" && c.Interval() != 0 {
				t.Fatal(c.Interval())
			}
		})
	}
}
func TestTemplateHasNoPersonalSettings(t *testing.T) {
	var c Config
	if err := toml.Unmarshal([]byte(Example), &c); err != nil {
		t.Fatal(err)
	}
	if c.GitHub.Organization != "" || c.GitHub.Account != "" || c.GitHub.RunnerGroup != "" || len(c.GitHub.Repositories) != 0 || len(c.GitHub.Runners) != 0 {
		t.Fatal("template contains GitHub settings")
	}
	if c.Display.RefreshInterval != "2m" || c.Display.QueueSort != "oldest" {
		t.Fatal(c.Display)
	}
}

func TestRememberedLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	defaultPath, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveDefaultPath(); err != nil || got != defaultPath {
		t.Fatal(got, err)
	}
	custom := filepath.Join(home, "custom", "settings.toml")
	if err := RememberPath(custom); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveDefaultPath(); err != nil || got != custom {
		t.Fatal(got, err)
	}
	if err := RememberPath(defaultPath); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveDefaultPath(); err != nil || got != defaultPath {
		t.Fatal(got, err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(defaultPath), "config-path"), []byte("relative.toml"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDefaultPath(); err == nil {
		t.Fatal("invalid saved path accepted")
	}
}
