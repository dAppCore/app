// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	coreio "dappco.re/go/core/io"
)

// TestDiscover_discover_Good — a manifest at start/.core/view.yaml is
// found, parsed, and the root resolves to the directory containing the
// .core/ folder.
func TestDiscover_discover_Good(t *testing.T) {
	dir := t.TempDir()
	must(t, coreio.Local.EnsureDir(dir+"/.core"))
	must(t, coreio.Local.Write(dir+"/.core/view.yaml", `
code: disc-good
name: Discover Good
version: 0.1.0
`))

	manifest, root, err := discover(coreio.Local, dir)
	if err != nil {
		t.Fatalf("discover returned error: %v", err)
	}
	if manifest.Code != "disc-good" {
		t.Errorf("manifest.Code = %q; want %q", manifest.Code, "disc-good")
	}
	if root != dir {
		t.Errorf("root = %q; want %q", root, dir)
	}
}

// TestDiscover_discover_Bad — directory without a .core/view.yaml
// anywhere on the walk up. discover returns an error, not a zero
// manifest.
func TestDiscover_discover_Bad(t *testing.T) {
	dir := t.TempDir()

	_, _, err := discover(coreio.Local, dir)
	if err == nil {
		t.Fatal("discover should fail without a manifest on the path")
	}
}

// TestDiscover_discover_Ugly — a parent .core/view.yaml is used when
// the start directory has none. Walk-up semantics are what the
// .core/ convention promises; a regression here would break every
// nested app.
func TestDiscover_discover_Ugly(t *testing.T) {
	root := t.TempDir()
	must(t, coreio.Local.EnsureDir(root+"/.core"))
	must(t, coreio.Local.Write(root+"/.core/view.yaml", `
code: disc-ugly
name: Discover Ugly
version: 0.1.0
`))
	// .git marker halts the walk at the repo boundary, so the parent
	// .core/ must be above .git. We only test the simple nested case
	// here — start is a child of the directory holding .core/.
	must(t, coreio.Local.EnsureDir(root+"/sub"))

	manifest, resolved, err := discover(coreio.Local, root+"/sub")
	if err != nil {
		t.Fatalf("discover from child returned error: %v", err)
	}
	if manifest.Code != "disc-ugly" {
		t.Errorf("manifest.Code = %q; want %q", manifest.Code, "disc-ugly")
	}
	if resolved != root {
		t.Errorf("root = %q; want %q", resolved, root)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
}
