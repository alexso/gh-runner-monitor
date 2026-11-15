// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexso/gh-runner-monitor/internal/config"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("PATH", t.TempDir())
	return home
}
func TestCLI(t *testing.T) {
	setupHome(t)
	for _, arg := range []string{"--help", "--version"} {
		var out bytes.Buffer
		if err := runWithInput([]string{arg}, strings.NewReader(""), &out, false); err != nil || out.Len() == 0 {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := runWithInput([]string{"--config", path}, strings.NewReader(""), &out, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gh auth login") || !strings.Contains(out.String(), "github.repositories") {
		t.Fatal(out.String())
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = runWithInput([]string{"--config", path}, strings.NewReader(""), &out, false); err == nil {
		t.Fatal("unedited config accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(original, after) {
		t.Fatal("existing config overwritten")
	}
	if err = runWithInput([]string{"init"}, strings.NewReader(""), &out, false); err == nil {
		t.Fatal("removed init command accepted")
	}
}
func TestFirstRunLocationAndDeletion(t *testing.T) {
	home := setupHome(t)
	custom := filepath.Join(home, "elsewhere", "monitor.toml")
	var out bytes.Buffer
	if err := runWithInput(nil, strings.NewReader(custom+"\n"), &out, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.ResolveDefaultPath()
	if err != nil || resolved != custom {
		t.Fatal(resolved, err)
	}
	if strings.Contains(out.String(), "shortcut") {
		t.Fatal("setup must not offer shortcut installation")
	}
	if err = os.Remove(custom); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err = runWithInput(nil, strings.NewReader("\n"), &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No configuration found") || !strings.Contains(out.String(), "Welcome") {
		t.Fatal("deleted config did not trigger setup")
	}
	if _, err = os.Stat(custom); err != nil {
		t.Fatal(err)
	}
}
func TestCheckCreatesMissingConfigWithoutPrompt(t *testing.T) {
	setupHome(t)
	var out bytes.Buffer
	if err := runWithInput([]string{"--check"}, strings.NewReader(""), &out, true); err == nil || !strings.Contains(err.Error(), "template created") {
		t.Fatal(err)
	}
	path, _ := config.DefaultPath()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Store the config here?") {
		t.Fatal("--check prompted")
	}
}
func TestExplicitConfigDoesNotChangeRememberedPath(t *testing.T) {
	home := setupHome(t)
	saved := filepath.Join(home, "saved.toml")
	if err := config.RememberPath(saved); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(home, "other.toml")
	var out bytes.Buffer
	if err := runWithInput([]string{"--config", other}, strings.NewReader(""), &out, false); err != nil {
		t.Fatal(err)
	}
	got, err := config.ResolveDefaultPath()
	if err != nil || got != saved {
		t.Fatal(got, err)
	}
}

func TestShortcutInstallerRemoved(t *testing.T) {
	setupHome(t)
	var out bytes.Buffer
	if err := runWithInput([]string{"--help"}, strings.NewReader(""), &out, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "install-shortcut") {
		t.Fatal("removed flag remains in help")
	}
	out.Reset()
	if err := runWithInput([]string{"--install-shortcut"}, strings.NewReader(""), &out, false); err == nil {
		t.Fatal("removed flag accepted")
	}
	path, _ := config.DefaultPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("removed flag should not create a config")
	}
}
