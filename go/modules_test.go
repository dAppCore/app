// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// TestModules_modules_Good — a manifest with no modules is a no-op.
// The most common case until apps start declaring core/media etc.
func TestModules_modules_Good(t *testing.T) {
	c := core.New()
	if err := modules(context.Background(), c, &config.ViewManifest{}); err != nil {
		t.Fatalf("modules with no declarations should succeed: %v", err)
	}
}

// TestModules_modules_Bad — a manifest declaring a module that is not
// in the host's registry errors with the missing name included.
func TestModules_modules_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{Modules: []string{"core/never-registered"}}
	err := modules(context.Background(), c, m)
	if err == nil {
		t.Fatal("modules should fail on unresolved module in prod mode")
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

// TestModules_modulesWithMode_Good — when every declared module is in
// the registry, the boot-mode resolver succeeds in both regimes.
func TestModules_modulesWithMode_Good(t *testing.T) {
	const name = "core/test-modulesWithMode-good"
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	defer UnregisterModule(name)

	c := core.New()
	m := &config.ViewManifest{Modules: []string{name}}
	if err := modulesWithMode(context.Background(), c, m, ModeProd); err != nil {
		t.Fatalf("prod resolve should succeed: %v", err)
	}
	if err := modulesWithMode(context.Background(), c, m, ModeDev); err != nil {
		t.Fatalf("dev resolve should succeed: %v", err)
	}
}

// TestModules_modulesWithMode_Bad — prod errors on an unresolved
// module; dev mode logs a warning and keeps booting.
func TestModules_modulesWithMode_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{Modules: []string{"core/unknown-module"}}
	if err := modulesWithMode(context.Background(), c, m, ModeProd); err == nil {
		t.Error("prod mode should reject an unresolved module")
	}
	if err := modulesWithMode(context.Background(), c, m, ModeDev); err != nil {
		t.Errorf("dev mode should tolerate an unresolved module: %v", err)
	}
}

// TestModules_modulesWithMode_Ugly — the boot helper handles nil
// inputs without panic.
func TestModules_modulesWithMode_Ugly(t *testing.T) {
	if err := modulesWithMode(context.Background(), nil, &config.ViewManifest{}, ModeProd); err == nil {
		t.Fatal("nil core should error")
	}
	c := core.New()
	if err := modulesWithMode(context.Background(), c, nil, ModeProd); err == nil {
		t.Fatal("nil manifest should error")
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

// TestModules_resolveModules_Bad — names absent from the registry come
// back in the missing slice.
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

// TestModules_moduleResolved_Good — a registered module name reports
// resolved=true; an unregistered one reports false.
func TestModules_moduleResolved_Good(t *testing.T) {
	const name = "core/test-moduleResolved-good"
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	defer UnregisterModule(name)

	c := core.New()
	if !moduleResolved(c, name) {
		t.Error("registered module should resolve")
	}
	if moduleResolved(c, "core/never-registered-here") {
		t.Error("unregistered module should not resolve")
	}
}

// TestModules_moduleResolved_Bad — nil core still works, the registry
// lookup is name-only.
func TestModules_moduleResolved_Bad(t *testing.T) {
	if moduleResolved(nil, "core/unknown") {
		t.Error("unregistered module with nil core should be false")
	}
}

// TestModules_moduleResolved_Ugly — empty module name is false.
func TestModules_moduleResolved_Ugly(t *testing.T) {
	c := core.New()
	if moduleResolved(c, "") {
		t.Error("empty name should be false")
	}
}
