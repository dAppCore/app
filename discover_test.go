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

// TestDiscover_discoverCompiled_Good — prod mode finds core.json at the
// project root and hydrates the ViewManifest from it without parsing
// the YAML source. Pins the RFC §4.1 "prod reads core.json" rule.
func TestDiscover_discoverCompiled_Good(t *testing.T) {
	dir := t.TempDir()

	// Drop a .core/ marker so CoreDirs picks up the root. The YAML is
	// deliberately a different code than the core.json we write so we
	// can tell which source won.
	must(t, coreio.Local.EnsureDir(dir+"/.core"))
	must(t, coreio.Local.Write(dir+"/.core/view.yaml", `
code: yaml-source
name: YAML Source
version: 0.1.0
`))

	// Compile + write core.json at the root. This is what prod mode
	// should read.
	cm := &CompiledManifest{
		Code:       "compiled-source",
		Name:       "Compiled Source",
		Version:    "0.2.0",
		CompiledAt: "2026-04-08T00:00:00Z",
		CompiledBy: "test",
	}
	must(t, WriteCompiled(coreio.Local, dir, cm))

	manifest, root, err := discoverCompiled(coreio.Local, dir, ModeProd)
	if err != nil {
		t.Fatalf("discoverCompiled: %v", err)
	}
	if manifest.Code != "compiled-source" {
		t.Errorf("prod mode read the wrong source: %q", manifest.Code)
	}
	if root != dir {
		t.Errorf("root = %q; want %q", root, dir)
	}
}

// TestDiscover_discoverCompiled_Bad — dev mode never reads core.json
// (even when present). The YAML is the source of truth during
// iteration.
func TestDiscover_discoverCompiled_Bad(t *testing.T) {
	dir := t.TempDir()
	must(t, coreio.Local.EnsureDir(dir+"/.core"))
	must(t, coreio.Local.Write(dir+"/.core/view.yaml", `
code: yaml-source
name: YAML Source
version: 0.1.0
`))

	cm := &CompiledManifest{Code: "compiled-source", Version: "0.2.0", Name: "X"}
	must(t, WriteCompiled(coreio.Local, dir, cm))

	manifest, _, err := discoverCompiled(coreio.Local, dir, ModeDev)
	if err != nil {
		t.Fatalf("discoverCompiled(dev): %v", err)
	}
	if manifest.Code != "yaml-source" {
		t.Errorf("dev mode should read YAML; got code %q", manifest.Code)
	}
}

// TestDiscover_discoverCompiled_Ugly — prod mode with no core.json
// falls back to view.yaml so operators who never compiled can still
// boot.
func TestDiscover_discoverCompiled_Ugly(t *testing.T) {
	dir := t.TempDir()
	must(t, coreio.Local.EnsureDir(dir+"/.core"))
	must(t, coreio.Local.Write(dir+"/.core/view.yaml", `
code: yaml-only
name: YAML Only
version: 0.1.0
`))

	manifest, _, err := discoverCompiled(coreio.Local, dir, ModeProd)
	if err != nil {
		t.Fatalf("discoverCompiled(prod, no core.json): %v", err)
	}
	if manifest.Code != "yaml-only" {
		t.Errorf("fallback should read YAML; got code %q", manifest.Code)
	}
}

// TestDiscover_compiledToManifest_Good — projecting a CompiledManifest
// back into the boot-pipeline shape preserves identity + layout.
func TestDiscover_compiledToManifest_Good(t *testing.T) {
	cm := &CompiledManifest{
		Code:    "project",
		Name:    "Project",
		Version: "0.1.0",
		Layout:  "HCF",
		Slots: map[string]string{
			"H": "head",
			"C": "centre",
			"F": "foot",
		},
		Modules: []string{"core/fs"},
	}
	v := compiledToManifest(cm)
	if v.Code != "project" || v.Layout != "HCF" {
		t.Errorf("projection mismatch: %+v", v)
	}
	if v.Slots["H"] != "head" {
		t.Errorf("slot H = %v; want 'head'", v.Slots["H"])
	}
	if len(v.Modules) != 1 || v.Modules[0] != "core/fs" {
		t.Errorf("modules: %+v", v.Modules)
	}
}

// TestDiscover_compiledToManifest_Bad — a nil input returns a
// zero-value manifest, not a panic.
func TestDiscover_compiledToManifest_Bad(t *testing.T) {
	v := compiledToManifest(nil)
	if v.Code != "" || v.Layout != "" || len(v.Slots) != 0 {
		t.Errorf("nil in should yield zero value; got %+v", v)
	}
}

// TestDiscover_compiledToManifest_Ugly — an empty slots map results in
// a nil slots value so downstream ranges see the "headless" shape.
func TestDiscover_compiledToManifest_Ugly(t *testing.T) {
	cm := &CompiledManifest{Code: "no-slots", Version: "0.1.0"}
	v := compiledToManifest(cm)
	if v.Slots != nil {
		t.Errorf("empty compiled slots should yield nil slots; got %+v", v.Slots)
	}
}
