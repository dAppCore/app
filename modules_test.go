// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
)

// TestModules_modules_Good — a manifest with no modules is a no-op.
// This is the common case while the module registry is still stub.
func TestModules_modules_Good(t *testing.T) {
	c := core.New()
	if err := modules(context.Background(), c, &config.ViewManifest{}); err != nil {
		t.Fatalf("modules with no declarations should succeed: %v", err)
	}
}

// TestModules_modules_Bad — a manifest declaring a module that cannot
// be resolved errors with the missing name included.
func TestModules_modules_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{Modules: []string{"core/never-registered"}}
	err := modules(context.Background(), c, m)
	if err == nil {
		t.Fatal("modules should fail on unresolved module")
	}
}

// TestModules_modules_Ugly — nil core errors cleanly (no panic) so
// callers passing an uninitialised container see a useful message.
func TestModules_modules_Ugly(t *testing.T) {
	err := modules(context.Background(), nil, &config.ViewManifest{})
	if err == nil {
		t.Fatal("modules should fail on nil core")
	}
}

// TestModules_resolveModules_Good — no names → empty missing slice.
func TestModules_resolveModules_Good(t *testing.T) {
	c := core.New()
	missing := resolveModules(c, nil)
	if len(missing) != 0 {
		t.Errorf("missing = %v; want empty", missing)
	}
}

// TestModules_resolveModules_Bad — every name is unresolved until the
// registry lands (see stub in moduleResolved).
func TestModules_resolveModules_Bad(t *testing.T) {
	c := core.New()
	missing := resolveModules(c, []string{"core/a", "core/b"})
	if len(missing) != 2 {
		t.Errorf("missing = %v; want 2 entries", missing)
	}
}

// TestModules_resolveModules_Ugly — duplicate entries both come back
// (no dedup happens here; dedup is a registry-layer concern).
func TestModules_resolveModules_Ugly(t *testing.T) {
	c := core.New()
	missing := resolveModules(c, []string{"core/a", "core/a"})
	if len(missing) != 2 {
		t.Errorf("missing = %v; want 2 entries (duplicates kept)", missing)
	}
}

// TestModules_moduleResolved_Good — the stub returns false for every
// name. Pinned here so a real implementation has to update this test
// alongside the resolver.
func TestModules_moduleResolved_Good(t *testing.T) {
	c := core.New()
	if moduleResolved(c, "core/media") {
		t.Error("stub resolver should report unresolved")
	}
}

// TestModules_moduleResolved_Bad — nil core still returns false,
// keeping the helper panic-free.
func TestModules_moduleResolved_Bad(t *testing.T) {
	if moduleResolved(nil, "core/media") {
		t.Error("stub resolver with nil core should be false")
	}
}

// TestModules_moduleResolved_Ugly — empty module name is false too.
func TestModules_moduleResolved_Ugly(t *testing.T) {
	c := core.New()
	if moduleResolved(c, "") {
		t.Error("stub resolver with empty name should be false")
	}
}
