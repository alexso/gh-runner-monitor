// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
