// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOptionalShortcut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink shortcut is for macOS/Linux")
	}
	home := setupHome(t)
	link := filepath.Join(home, ".local", "bin", "grm")
	var out bytes.Buffer
	if err := offerShortcut(strings.NewReader("\n"), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("installed without consent")
	}
	out.Reset()
	if err := offerShortcut(strings.NewReader("yes\n"), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "export PATH=") {
		t.Fatal("missing PATH guidance")
	}
	if err := installShortcut("another-target", link); err == nil {
		t.Fatal("existing shortcut replaced")
	}
	out.Reset()
	if err := offerShortcut(strings.NewReader("yes\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Fatal(out.String())
	}
}
func TestShortcutKeepsExistingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink shortcut is for macOS/Linux")
	}
	setupHome(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if err := os.WriteFile(filepath.Join(bin, "grm"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := offerShortcut(strings.NewReader("yes\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "existing command") {
		t.Fatal(out.String())
	}
}
