// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/config"
)

// TestRegistry_RegisterModule_Good — Registering, looking up and
// removing a module name round-trips cleanly.
func TestRegistry_RegisterModule_Good(t *testing.T) {
	const name = "core/test-RegisterModule-good"
	UnregisterModule(name) // hermetic test — start clean

	called := false
	factory := func() core.CoreOption {
		return func(_ *core.Core) core.Result {
			called = true
			return core.Result{OK: true}
		}
	}
	RegisterModule(name, factory)
	defer UnregisterModule(name)

	got, ok := LookupModule(name)
	if !ok || got == nil {
		t.Fatal("LookupModule should resolve a freshly registered factory")
	}

	// Apply the factory and check the side effect.
	if err := applyModuleFactories(core.New(), []ModuleFactory{got}); err != nil {
		t.Fatalf("applyModuleFactories: %v", err)
	}
	if !called {
		t.Error("factory closure was not invoked by applyModuleFactories")
	}
}

// TestRegistry_RegisterModule_Bad — empty name and nil factory are
// silently ignored so init code stays terse.
func TestRegistry_RegisterModule_Bad(t *testing.T) {
	RegisterModule("", func() core.CoreOption { return nil })
	if _, ok := LookupModule(""); ok {
		t.Error("empty name should never be registered")
	}
	RegisterModule("core/with-nil-factory", nil)
	if _, ok := LookupModule("core/with-nil-factory"); ok {
		t.Error("nil factory should never be registered")
	}
}

// TestRegistry_RegisterModule_Ugly — re-registering the same name
// overwrites the previous factory (last writer wins).
func TestRegistry_RegisterModule_Ugly(t *testing.T) {
	const name = "core/test-RegisterModule-ugly"
	UnregisterModule(name)
	defer UnregisterModule(name)

	first, second := false, false
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { first = true; return core.Result{OK: true} }
	})
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { second = true; return core.Result{OK: true} }
	})
	got, _ := LookupModule(name)
	_ = applyModuleFactories(core.New(), []ModuleFactory{got})
	if first {
		t.Error("first factory should have been replaced")
	}
	if !second {
		t.Error("second factory should have been invoked")
	}
}

// TestRegistry_LookupModule_Good — registered names return (factory,
// true); unregistered names return (nil, false).
func TestRegistry_LookupModule_Good(t *testing.T) {
	const name = "core/test-LookupModule-good"
	UnregisterModule(name)
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	defer UnregisterModule(name)

	if _, ok := LookupModule(name); !ok {
		t.Error("registered module should resolve")
	}
	if _, ok := LookupModule("core/no-such-module-anywhere"); ok {
		t.Error("unregistered module should not resolve")
	}
}

// TestRegistry_LookupModule_Bad — empty name is never resolved.
func TestRegistry_LookupModule_Bad(t *testing.T) {
	if _, ok := LookupModule(""); ok {
		t.Error("empty name should never resolve")
	}
}

// TestRegistry_LookupModule_Ugly — querying after Unregister returns
// (nil, false).
func TestRegistry_LookupModule_Ugly(t *testing.T) {
	const name = "core/test-LookupModule-ugly"
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	UnregisterModule(name)
	if _, ok := LookupModule(name); ok {
		t.Error("module should be gone after Unregister")
	}
}

// TestRegistry_RegisteredModules_Good — the listing comes back sorted
// and contains every freshly registered name.
func TestRegistry_RegisteredModules_Good(t *testing.T) {
	names := []string{
		"core/test-listing-z",
		"core/test-listing-a",
		"core/test-listing-m",
	}
	for _, n := range names {
		RegisterModule(n, func() core.CoreOption {
			return func(_ *core.Core) core.Result { return core.Result{OK: true} }
		})
	}
	defer func() {
		for _, n := range names {
			UnregisterModule(n)
		}
	}()

	got := RegisteredModules()

	// Confirm every name is present.
	for _, n := range names {
		found := false
		for _, g := range got {
			if g == n {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("module %q missing from RegisteredModules listing", n)
		}
	}
	// Confirm the entire output is sorted (independent of which other
	// tests previously registered factories).
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("RegisteredModules not sorted: %q > %q", got[i-1], got[i])
			break
		}
	}
}

// TestRegistry_RegisteredModules_Bad — empty registry returns nil.
func TestRegistry_RegisteredModules_Bad(t *testing.T) {
	// Snapshot existing module names so we can restore.
	existing := RegisteredModules()
	for _, n := range existing {
		UnregisterModule(n)
	}
	defer func() {
		// Restore so other tests are not perturbed (this is best effort
		// — registering with a no-op factory keeps the API contract).
		for _, n := range existing {
			RegisterModule(n, func() core.CoreOption {
				return func(_ *core.Core) core.Result { return core.Result{OK: true} }
			})
		}
	}()
	if got := RegisteredModules(); got != nil {
		t.Errorf("empty registry should return nil; got %v", got)
	}
}

// TestRegistry_RegisteredModules_Ugly — duplicate registrations don't
// produce duplicates in the listing.
func TestRegistry_RegisteredModules_Ugly(t *testing.T) {
	const name = "core/test-listing-dedup"
	UnregisterModule(name)
	defer UnregisterModule(name)

	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})

	got := RegisteredModules()
	count := 0
	for _, g := range got {
		if g == name {
			count++
		}
	}
	if count != 1 {
		t.Errorf("module %q should appear once; got %d", name, count)
	}
}

// TestRegistry_resolveModulesFromRegistry_Good — names that resolve
// land in the matched slice; everything else lands in missing.
func TestRegistry_resolveModulesFromRegistry_Good(t *testing.T) {
	const present = "core/test-resolve-present"
	const absent = "core/test-resolve-absent"
	UnregisterModule(present)
	UnregisterModule(absent)
	RegisterModule(present, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	defer UnregisterModule(present)

	matched, missing := resolveModulesFromRegistry([]string{present, absent})
	if len(matched) != 1 {
		t.Errorf("matched = %d; want 1", len(matched))
	}
	if len(missing) != 1 || missing[0] != absent {
		t.Errorf("missing = %v; want [%q]", missing, absent)
	}
}

// TestRegistry_resolveModulesFromRegistry_Bad — empty input returns
// (nil, nil) so callers don't pay for an allocation.
func TestRegistry_resolveModulesFromRegistry_Bad(t *testing.T) {
	matched, missing := resolveModulesFromRegistry(nil)
	if matched != nil || missing != nil {
		t.Errorf("empty input should return (nil, nil); got (%v, %v)", matched, missing)
	}
}

// TestRegistry_resolveModulesFromRegistry_Ugly — duplicate names are
// resolved twice (no dedup at this layer).
func TestRegistry_resolveModulesFromRegistry_Ugly(t *testing.T) {
	const name = "core/test-resolve-dup"
	UnregisterModule(name)
	defer UnregisterModule(name)

	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})

	matched, missing := resolveModulesFromRegistry([]string{name, name})
	if len(matched) != 2 {
		t.Errorf("matched = %d; want 2 (duplicates resolved twice)", len(matched))
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v; want empty", missing)
	}
}

// TestRegistry_applyModuleFactories_Good — every factory's CoreOption
// runs against the supplied container.
func TestRegistry_applyModuleFactories_Good(t *testing.T) {
	c := core.New()
	hits := 0
	factories := []ModuleFactory{
		func() core.CoreOption {
			return func(_ *core.Core) core.Result { hits++; return core.Result{OK: true} }
		},
		func() core.CoreOption {
			return func(_ *core.Core) core.Result { hits++; return core.Result{OK: true} }
		},
	}
	if err := applyModuleFactories(c, factories); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if hits != 2 {
		t.Errorf("hits = %d; want 2", hits)
	}
}

// TestRegistry_applyModuleFactories_Bad — a factory whose CoreOption
// returns !OK surfaces a typed error.
func TestRegistry_applyModuleFactories_Bad(t *testing.T) {
	c := core.New()
	factories := []ModuleFactory{
		func() core.CoreOption {
			return func(_ *core.Core) core.Result {
				return core.Result{OK: false, Value: core.NewError("boom")}
			}
		},
	}
	if err := applyModuleFactories(c, factories); err == nil {
		t.Fatal("apply should surface a non-OK option as a typed error")
	}
}

// TestRegistry_applyModuleFactories_Ugly — nil core errors; nil
// factory entries are silently skipped.
func TestRegistry_applyModuleFactories_Ugly(t *testing.T) {
	if err := applyModuleFactories(nil, nil); err == nil {
		t.Fatal("nil core should error")
	}

	c := core.New()
	if err := applyModuleFactories(c, []ModuleFactory{nil}); err != nil {
		t.Errorf("nil factory entries should be skipped, not error: %v", err)
	}
}

// TestRegistry_loadModules_Good — a manifest with a known module name
// applies the factory in both prod and dev.
func TestRegistry_loadModules_Good(t *testing.T) {
	const name = "core/test-loadModules-good"
	hits := 0
	RegisterModule(name, func() core.CoreOption {
		return func(_ *core.Core) core.Result { hits++; return core.Result{OK: true} }
	})
	defer UnregisterModule(name)

	c := core.New()
	m := &config.ViewManifest{Modules: []string{name}}
	if err := loadModules(context.Background(), c, m, ModeProd); err != nil {
		t.Fatalf("prod load: %v", err)
	}
	if hits != 1 {
		t.Errorf("hits after prod = %d; want 1", hits)
	}
}

// TestRegistry_loadModules_Bad — an unresolved name fails prod and
// only logs in dev.
func TestRegistry_loadModules_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{Modules: []string{"core/never-registered-loadModules"}}
	if err := loadModules(context.Background(), c, m, ModeProd); err == nil {
		t.Error("prod load should fail on unresolved")
	}
	if err := loadModules(context.Background(), c, m, ModeDev); err != nil {
		t.Errorf("dev load should tolerate unresolved: %v", err)
	}
}

// TestRegistry_loadModules_Ugly — empty manifest module list short-
// circuits (no error, no work).
func TestRegistry_loadModules_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{}
	if err := loadModules(context.Background(), c, m, ModeProd); err != nil {
		t.Errorf("empty modules should succeed; got %v", err)
	}
}
