// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestPkgMarketplace_runMarketplaceUpdate_Good — `--help` on the new
// update verb returns 0. Full update path requires DIR_HOME wiring
// (covered by app.MarketplaceUpdate tests in the parent package).
func TestPkgMarketplace_runMarketplaceUpdate_Good(t *testing.T) {
	if rc := runMarketplaceUpdate([]string{"--help"}); rc != 0 {
		t.Errorf("update --help rc = %d; want 0", rc)
	}
}

// TestPkgMarketplace_runMarketplaceUpdate_Bad — missing CODE, unknown
// flag, and dispatch through runMarketplace are all rejected with the
// usage exit code.
func TestPkgMarketplace_runMarketplaceUpdate_Bad(t *testing.T) {
	if rc := runMarketplaceUpdate(nil); rc != 64 {
		t.Errorf("update with no code rc = %d; want 64", rc)
	}
	if rc := runMarketplaceUpdate([]string{"--what"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
	if rc := runMarketplace([]string{"update"}); rc != 64 {
		t.Errorf("dispatch update with no code rc = %d; want 64", rc)
	}
}

// TestPkgMarketplace_runMarketplaceUpdate_Ugly — DIR_HOME with no
// marketplace cache produces a clean error (rc=1) rather than a panic.
// The actual marketplace lookup needs a planted cache; without one the
// command must still fail safely.
func TestPkgMarketplace_runMarketplaceUpdate_Ugly(t *testing.T) {
	if home := core.Env("DIR_HOME"); home == "" {
		t.Skip("DIR_HOME unset — covered by app.MarketplaceUpdate tests")
	}
	if rc := runMarketplaceUpdate([]string{"definitely-not-installed-pkg"}); rc == 0 {
		t.Errorf("missing pkg rc = 0; want non-zero")
	}
}

// TestPkgMarketplace_runMarketplaceInstalled_Good — `--help` returns
// 0; the verb routes to runPkgList under the hood so any further
// behaviour is covered by the pkg list tests.
func TestPkgMarketplace_runMarketplaceInstalled_Good(t *testing.T) {
	if rc := runMarketplaceInstalled([]string{"--help"}); rc != 0 {
		t.Errorf("installed --help rc = %d; want 0", rc)
	}
}

// TestPkgMarketplace_runMarketplaceInstalled_Bad — unknown flags are
// rejected with EX_USAGE.
func TestPkgMarketplace_runMarketplaceInstalled_Bad(t *testing.T) {
	if rc := runMarketplaceInstalled([]string{"--what"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
}

// TestPkgMarketplace_runMarketplace_Ugly — the new marketplace verbs
// (`update`, `remove`, `installed`) all dispatch from runMarketplace
// without erroring out at the dispatcher level. Full behaviour
// is covered by the per-verb tests above.
func TestPkgMarketplace_runMarketplace_Ugly(t *testing.T) {
	if rc := runMarketplace([]string{"installed", "--help"}); rc != 0 {
		t.Errorf("installed --help via dispatcher rc = %d; want 0", rc)
	}
	if rc := runMarketplace([]string{"update", "--help"}); rc != 0 {
		t.Errorf("update --help via dispatcher rc = %d; want 0", rc)
	}
	// `remove` delegates to runPkgRemove which requires a NAME, so a
	// no-args call should land on EX_USAGE.
	if rc := runMarketplace([]string{"remove"}); rc != 64 {
		t.Errorf("remove with no name rc = %d; want 64", rc)
	}
}

// TestPkgMarketplace_stripStringPrefix_Good — the prefix-stripper picks
// the suffix off recognised CLI source tags so `pkg update` can route
// `marketplace:CODE` into MarketplaceUpdate.
func TestPkgMarketplace_stripStringPrefix_Good(t *testing.T) {
	cases := []struct {
		in, prefix, want string
		ok               bool
	}{
		{"marketplace:photo-browser", "marketplace:", "photo-browser", true},
		{"wrap:web:./my-site", "wrap:web:", "./my-site", true},
		{"local:/srv/app", "local:", "/srv/app", true},
	}
	for _, tc := range cases {
		got, ok := stripStringPrefix(tc.in, tc.prefix)
		if ok != tc.ok {
			t.Errorf("stripStringPrefix(%q, %q) ok = %v; want %v", tc.in, tc.prefix, ok, tc.ok)
		}
		if got != tc.want {
			t.Errorf("stripStringPrefix(%q, %q) = %q; want %q", tc.in, tc.prefix, got, tc.want)
		}
	}
}

// TestPkgMarketplace_stripStringPrefix_Bad — non-matching prefixes
// return ("", false).
func TestPkgMarketplace_stripStringPrefix_Bad(t *testing.T) {
	got, ok := stripStringPrefix("marketplace:photo", "wrap:")
	if ok {
		t.Errorf("stripStringPrefix should not match wrong prefix; got=%q", got)
	}
	got, ok = stripStringPrefix("", "marketplace:")
	if ok {
		t.Errorf("stripStringPrefix on empty should fail; got=%q", got)
	}
}

// TestPkgMarketplace_stripStringPrefix_Ugly — empty prefix matches every
// non-empty string (HasPrefix returns true on "") so the whole input
// passes through. Documented quirk; the caller never feeds an empty
// prefix in practice.
func TestPkgMarketplace_stripStringPrefix_Ugly(t *testing.T) {
	got, ok := stripStringPrefix("marketplace:foo", "")
	if !ok {
		t.Errorf("empty prefix should always match; got ok=false")
	}
	if got != "marketplace:foo" {
		t.Errorf("empty prefix should pass through; got %q", got)
	}
}

// TestPkgMarketplace_readInstalledSource_Good — reads the recorded
// `Config["source"]` from an installed package's view.yaml so
// runPkgUpdate can pick the right update path.
func TestPkgMarketplace_readInstalledSource_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	view := core.Path(home, ".core", app.AppsDirName, "with-source", ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(view)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := "code: with-source\nname: With Source\nversion: 0.1.0\nconfig:\n  source: marketplace:with-source\n"
	if err := medium.Write(view, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	src := readInstalledSource(medium, home, "with-source")
	if src != "marketplace:with-source" {
		t.Errorf("source = %q; want 'marketplace:with-source'", src)
	}
}

// TestPkgMarketplace_readInstalledSource_Bad — empty inputs and a
// missing install both return an empty string (caller falls back).
func TestPkgMarketplace_readInstalledSource_Bad(t *testing.T) {
	if got := readInstalledSource(nil, "", ""); got != "" {
		t.Errorf("nil medium should yield empty; got %q", got)
	}
	if got := readInstalledSource(coreio.Local, "", "foo"); got != "" {
		t.Errorf("empty home should yield empty; got %q", got)
	}
	if got := readInstalledSource(coreio.Local, t.TempDir(), ""); got != "" {
		t.Errorf("empty name should yield empty; got %q", got)
	}
	if got := readInstalledSource(coreio.Local, t.TempDir(), "missing"); got != "" {
		t.Errorf("missing install should yield empty; got %q", got)
	}
}

// TestPkgMarketplace_readInstalledSource_Ugly — a valid view.yaml
// without a `source` key returns an empty string. A malformed view
// yaml ditto (parse failure swallowed because the caller treats this
// as best-effort routing intel).
func TestPkgMarketplace_readInstalledSource_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	// Manifest with no source field.
	plant(t, home, "no-source", &config.ViewManifest{
		Code: "no-source", Name: "No Source", Version: "0.1.0",
	})
	if got := readInstalledSource(medium, home, "no-source"); got != "" {
		t.Errorf("no-source should yield empty; got %q", got)
	}

	// Malformed manifest body.
	malformed := core.Path(home, ".core", app.AppsDirName, "broken", ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(malformed)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(malformed, "not: [valid: yaml: at all"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := readInstalledSource(medium, home, "broken"); got != "" {
		t.Errorf("malformed should yield empty; got %q", got)
	}
}

// TestPkgMarketplace_runMarketplaceSearch_Bad — missing QUERY returns
// EX_USAGE so the shell knows to surface the usage line.
func TestPkgMarketplace_runMarketplaceSearch_Bad(t *testing.T) {
	if rc := runMarketplaceSearch(nil); rc != 64 {
		t.Errorf("missing QUERY rc = %d; want 64", rc)
	}
}

// TestPkgMarketplace_runMarketplaceSearch_Ugly — a query against a
// non-existent marketplace cache returns rc=1 (unable to read the
// missing index.json). Pinned so a regression that silently turns the
// missing-index path into "no results" gets caught.
func TestPkgMarketplace_runMarketplaceSearch_Ugly(t *testing.T) {
	if home := core.Env("DIR_HOME"); home == "" {
		t.Skip("DIR_HOME unset — search path requires it")
	}
	root := core.Path(core.Env("DIR_HOME"), ".core", "marketplace")
	if coreio.Local.IsDir(root) {
		t.Skip("user has a real marketplace cache; skipping the missing-cache path")
	}
	if rc := runMarketplaceSearch([]string{"never-there"}); rc != 1 {
		t.Errorf("missing-cache search rc = %d; want 1", rc)
	}
}

// TestPkgMarketplace_runMarketplaceFetch_Bad — missing --url and
// dangling --url both return EX_USAGE; unknown flag too.
func TestPkgMarketplace_runMarketplaceFetch_Bad(t *testing.T) {
	if rc := runMarketplaceFetch(nil); rc != 64 {
		t.Errorf("missing --url rc = %d; want 64", rc)
	}
	if rc := runMarketplaceFetch([]string{"--url"}); rc != 64 {
		t.Errorf("dangling --url rc = %d; want 64", rc)
	}
	if rc := runMarketplaceFetch([]string{"--what"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
}

// TestPkgMarketplace_runMarketplaceFetch_Good — `--help` returns 0.
// The full clone path requires git on PATH and is exercised by the
// MarketplaceFetch tests in the parent package.
func TestPkgMarketplace_runMarketplaceFetch_Good(t *testing.T) {
	if rc := runMarketplaceFetch([]string{"--help"}); rc != 0 {
		t.Errorf("--help rc = %d; want 0", rc)
	}
}

// TestPkgMarketplace_runMarketplaceFetch_Ugly — a fetch with --url
// pointing at a definitely-unreachable host returns rc=1 (clone
// fails). Pins the error path so a regression that swallows the
// failure gets caught. We require DIR_HOME to be writable.
func TestPkgMarketplace_runMarketplaceFetch_Ugly(t *testing.T) {
	if home := core.Env("DIR_HOME"); home == "" {
		t.Skip("DIR_HOME unset — fetch path requires it")
	}
	// Use a TLD that will not resolve — every git binary returns
	// non-zero quickly without trying to authenticate.
	if rc := runMarketplaceFetch([]string{"--url", "https://example.invalid/does-not-exist.git"}); rc == 0 {
		t.Errorf("unreachable URL rc = 0; want non-zero")
	}
}
