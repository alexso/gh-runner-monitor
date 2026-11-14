// Copyright (c) 2026 Alex So.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		var out bytes.Buffer
		if e := run([]string{arg}, &out); e != nil || out.Len() == 0 {
			t.Fatal(e)
		}
	}
	var out bytes.Buffer
	p := filepath.Join(t.TempDir(), "config.toml")
	if e := run([]string{"init", "--config", p}, &out); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"init", "--config", p}, &out); e == nil {
		t.Fatal("overwritten")
	}
	if e := run([]string{"--config", p + "missing"}, &out); e == nil || !strings.Contains(e.Error(), "init") {
		t.Fatal(e)
	}
}
