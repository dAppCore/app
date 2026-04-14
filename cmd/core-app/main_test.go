// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"

	"dappco.re/go/app"
)

// TestMain_parseArgs_Good — the default (no args) lands on ModeProd
// and the current directory. This is the "run me in a CoreApp dir"
// behaviour.
func TestMain_parseArgs_Good(t *testing.T) {
	mode, start := parseArgs(nil)
	if mode != app.ModeProd {
		t.Errorf("mode = %v; want %v", mode, app.ModeProd)
	}
	if start != "./" {
		t.Errorf("start = %q; want %q", start, "./")
	}
}

// TestMain_parseArgs_Bad — a positional arg becomes start, regardless
// of whether it exists. The Boot step surfaces the missing manifest.
func TestMain_parseArgs_Bad(t *testing.T) {
	mode, start := parseArgs([]string{"./never-there"})
	if mode != app.ModeProd {
		t.Errorf("mode = %v; want %v", mode, app.ModeProd)
	}
	if start != "./never-there" {
		t.Errorf("start = %q; want %q", start, "./never-there")
	}
}

// TestMain_parseArgs_Ugly — --dev flag lifts mode, and a trailing
// path still wins as start.
func TestMain_parseArgs_Ugly(t *testing.T) {
	mode, start := parseArgs([]string{"--dev", "./photo-browser"})
	if mode != app.ModeDev {
		t.Errorf("mode = %v; want %v", mode, app.ModeDev)
	}
	if start != "./photo-browser" {
		t.Errorf("start = %q; want %q", start, "./photo-browser")
	}
}
