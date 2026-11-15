// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alexso/gh-runner-monitor/internal/config"
)

// A missing configuration is the only first-run marker. Deleting it restarts setup.
func ensureConfig(path string, out io.Writer) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect config %s: %w", path, err)
	}
	if err = config.Create(path); err != nil {
		// Another process may have created the file after our initial check.
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	fmt.Fprintf(out, `Welcome to gh-runner-monitor!

Created configuration:
  %s

Before starting:
  1. Install GitHub CLI (gh): https://cli.github.com/
  2. Authenticate: gh auth login --hostname github.com
  3. Edit the configuration above and fill in:
     github.organization — your organization login
     github.repositories — repository names without the owner

For example:
  [github]
  organization = "my-organization"
  repositories = ["project-a", "project-b"]

Optional: set github.account to an existing gh account username.
An empty account uses the active gh account. Do not put tokens in the config.
The template includes comments for every setting.

Then run gh-runner-monitor --check to verify access, followed by gh-runner-monitor.
If you supplied --config, keep using it on both commands.
Locations chosen at the setup prompt are remembered automatically.

`, path)
	return true, nil
}

func offerShortcut(in io.Reader, out io.Writer) error {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(out, "Optional shortcut: add a grm alias for gh-runner-monitor in your shell profile.")
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	link := filepath.Join(home, ".local", "bin", "grm")
	if _, err := os.Lstat(link); err == nil {
		fmt.Fprintf(out, "Optional grm shortcut skipped: %s already exists.\n", link)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if existing, err := exec.LookPath("grm"); err == nil {
		fmt.Fprintf(out, "Optional grm shortcut skipped: an existing command was found at %s.\n", existing)
		return nil
	}
	fmt.Fprintf(out, "Optional shortcut: install grm at %s?\nThis creates a symlink without editing shell files. Existing shell aliases or functions named grm take precedence; choose no if you already use that name.\nInstall grm? [y/N]: ", link)
	answer, err := readLine(in)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(out, "Skipped grm shortcut. You can keep using gh-runner-monitor.")
		return nil
	}
	target, err := shortcutTarget()
	if err != nil {
		return err
	}
	if err = installShortcut(target, link); err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s → %s\n", link, target)
	if !onPath(filepath.Dir(link)) {
		fmt.Fprintln(out, `To use grm, add this directory to PATH in your shell configuration:
  export PATH="$HOME/.local/bin:$PATH"`)
	}
	return nil
}

func shortcutTarget() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Keep Homebrew's stable bin path rather than a version-specific Cellar path.
	if path, e := exec.LookPath("gh-runner-monitor"); e == nil {
		a, ea := os.Stat(path)
		b, eb := os.Stat(current)
		if ea == nil && eb == nil && os.SameFile(a, b) {
			return filepath.Abs(path)
		}
	}
	return current, nil
}
func installShortcut(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	// Symlink itself is exclusive, including when the existing link is broken.
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("create shortcut without overwriting existing files: %w", err)
	}
	return nil
}
func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p != "" {
			abs, err := filepath.Abs(p)
			if err == nil && abs == dir {
				return true
			}
		}
	}
	return false
}

// Reuse the reader across setup prompts so buffered answers are not discarded.
func readLine(in io.Reader) (string, error) {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader.ReadString('\n')
	}
	return bufio.NewReader(in).ReadString('\n')
}
func chooseConfigPath(in io.Reader, out io.Writer, suggested string) (string, error) {
	fmt.Fprintf(out, "No configuration found. Store the config here?\n  %s\nPress Enter to accept, or enter a different file path: ", suggested)
	answer, err := readLine(in)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	path := strings.TrimSpace(answer)
	if path == "" {
		return suggested, nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(out, "Using %s. This location will be remembered for future runs.\n", path)
	return path, nil
}
