// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
)

// TestPermissions_permissions_Good — a manifest that declares `read`
// results in fs.read being Allowed; an unmapped action (gui.*) is
// always allowed because it isn't gated.
func TestPermissions_permissions_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
			Net:  []string{"api.example.com:443"},
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions returned error: %v", err)
	}
	if e := c.Entitled("fs.read"); !e.Allowed {
		t.Errorf("fs.read Allowed = false; want true. Reason: %q", e.Reason)
	}
	if e := c.Entitled("gui.window.create"); !e.Allowed {
		t.Errorf("gui.* should bypass gate; Allowed = false")
	}
}

// TestPermissions_permissions_Bad — a manifest without the required
// permission denies the corresponding action in prod mode.
func TestPermissions_permissions_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{} // no permissions declared
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions returned error: %v", err)
	}
	if e := c.Entitled("fs.read"); e.Allowed {
		t.Error("fs.read should be denied without a read permission in prod mode")
	}
}

// TestPermissions_permissions_Ugly — dev mode never blocks but still
// reports the reason, so devs see what they're missing.
func TestPermissions_permissions_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{} // no permissions declared
	if err := permissions(c, m, ModeDev); err != nil {
		t.Fatalf("permissions returned error: %v", err)
	}
	e := c.Entitled("fs.read")
	if !e.Allowed {
		t.Error("dev mode should never deny")
	}
	if e.Reason == "" {
		t.Error("dev mode should surface the would-have-denied reason")
	}
}

// TestPermissions_gateFor_Good — the prefix matcher picks the right
// field for a known action.
func TestPermissions_gateFor_Good(t *testing.T) {
	gate, ok := gateFor("fs.read")
	if !ok {
		t.Fatal("fs.read should be gated")
	}
	if gate.field != fieldRead {
		t.Errorf("fs.read field = %v; want %v", gate.field, fieldRead)
	}
}

// TestPermissions_gateFor_Bad — an unknown action returns ok=false
// (so the caller can allow-through without a bogus gate).
func TestPermissions_gateFor_Bad(t *testing.T) {
	if _, ok := gateFor("gui.window.create"); ok {
		t.Error("gui.* should not be gated")
	}
}

// TestPermissions_gateFor_Ugly — the matcher accepts a prefix followed
// by a dotted sub-action (process.run.foo → matches process.run).
func TestPermissions_gateFor_Ugly(t *testing.T) {
	gate, ok := gateFor("process.run.long.chain")
	if !ok {
		t.Fatal("process.run.foo should be gated under process.run")
	}
	if gate.field != fieldRun {
		t.Errorf("matched field = %v; want %v", gate.field, fieldRun)
	}
}

// TestPermissions_hasPermission_Good — a non-empty slot returns true.
func TestPermissions_hasPermission_Good(t *testing.T) {
	p := config.ViewPermissions{Read: []string{"./"}}
	if !hasPermission(p, fieldRead) {
		t.Error("hasPermission fieldRead should be true when Read is set")
	}
}

// TestPermissions_hasPermission_Bad — an unknown field returns false.
func TestPermissions_hasPermission_Bad(t *testing.T) {
	p := config.ViewPermissions{}
	if hasPermission(p, permissionField(42)) {
		t.Error("unknown field should return false")
	}
}

// TestPermissions_hasPermission_Ugly — the Filesystem bool satisfies
// the read slot. (Legacy API compatibility — ViewPermissions grew the
// bool before the typed lists.)
func TestPermissions_hasPermission_Ugly(t *testing.T) {
	p := config.ViewPermissions{Filesystem: true}
	if !hasPermission(p, fieldRead) {
		t.Error("Filesystem=true should satisfy fieldRead")
	}
}

// TestPermissions_newChecker_Good — the checker closure gates an
// action based on the manifest it was built from, across goroutines.
// This is a smoke test, not a race-detector pass.
func TestPermissions_newChecker_Good(t *testing.T) {
	checker := newChecker(
		config.ViewPermissions{Read: []string{"./"}},
		ModeProd,
	)
	e := checker("fs.read", 1, context.Background())
	if !e.Allowed {
		t.Errorf("fs.read should be allowed; reason: %q", e.Reason)
	}
}

// TestPermissions_newChecker_Bad — prod-mode denial on an undeclared
// permission sets a reason.
func TestPermissions_newChecker_Bad(t *testing.T) {
	checker := newChecker(config.ViewPermissions{}, ModeProd)
	e := checker("net.fetch", 1, context.Background())
	if e.Allowed {
		t.Error("net.fetch should be denied without a net permission")
	}
	if e.Reason == "" {
		t.Error("denial should explain why")
	}
}

// TestPermissions_newChecker_Ugly — dev mode allows-with-reason even on
// missing permissions.
func TestPermissions_newChecker_Ugly(t *testing.T) {
	checker := newChecker(config.ViewPermissions{}, ModeDev)
	e := checker("net.fetch", 1, context.Background())
	if !e.Allowed {
		t.Error("dev mode should allow every gate")
	}
	if e.Reason == "" {
		t.Error("dev mode should still surface the reason")
	}
}
