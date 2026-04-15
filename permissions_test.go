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

// TestPermissions_newCheckerForManifest_Good — a manifest with
// `permissions.store: true` recorded in Config["store"] satisfies the
// store gate.
func TestPermissions_newCheckerForManifest_Good(t *testing.T) {
	m := &config.ViewManifest{
		Config: map[string]any{"store": true},
	}
	checker := newCheckerForManifest(m, ModeProd)
	e := checker("store.get", 1, context.Background())
	if !e.Allowed {
		t.Errorf("store.get should be allowed when Config[store]=true; reason=%q", e.Reason)
	}
}

// TestPermissions_newCheckerForManifest_Bad — the store gate denies
// when neither Filesystem nor Config["store"] is set.
func TestPermissions_newCheckerForManifest_Bad(t *testing.T) {
	checker := newCheckerForManifest(&config.ViewManifest{}, ModeProd)
	e := checker("store.get", 1, context.Background())
	if e.Allowed {
		t.Error("store.get should be denied without store / filesystem permission")
	}
}

// TestPermissions_newCheckerForManifest_Ugly — Config["store"] of the
// wrong type doesn't crash the checker; it falls back to the slot
// check (which is also empty here, so the gate denies).
func TestPermissions_newCheckerForManifest_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Config: map[string]any{"store": "yes"}, // wrong type
	}
	checker := newCheckerForManifest(m, ModeProd)
	e := checker("store.set", 1, context.Background())
	if e.Allowed {
		t.Error("store.set should be denied when Config[store] is the wrong type")
	}

	// Filesystem=true still satisfies the gate by the legacy fallback.
	m2 := &config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}
	checker2 := newCheckerForManifest(m2, ModeProd)
	e2 := checker2("store.delete", 1, context.Background())
	if !e2.Allowed {
		t.Error("Filesystem=true should satisfy the store gate (legacy fallback)")
	}
}

// TestPermissions_hasManifestStorePermission_Good — bool true and the
// Filesystem fallback both report declared.
func TestPermissions_hasManifestStorePermission_Good(t *testing.T) {
	m := &config.ViewManifest{Config: map[string]any{"store": true}}
	if !hasManifestStorePermission(m) {
		t.Error("Config[store]=true should report declared")
	}
	m2 := &config.ViewManifest{Permissions: config.ViewPermissions{Filesystem: true}}
	if !hasManifestStorePermission(m2) {
		t.Error("Filesystem=true should report declared")
	}
}

// TestPermissions_hasManifestStorePermission_Bad — nil manifest and
// no flags both report not-declared.
func TestPermissions_hasManifestStorePermission_Bad(t *testing.T) {
	if hasManifestStorePermission(nil) {
		t.Error("nil manifest should not report declared")
	}
	if hasManifestStorePermission(&config.ViewManifest{}) {
		t.Error("empty manifest should not report declared")
	}
}

// TestPermissions_hasManifestStorePermission_Ugly — wrong-type
// Config["store"] reports not-declared without panicking.
func TestPermissions_hasManifestStorePermission_Ugly(t *testing.T) {
	m := &config.ViewManifest{Config: map[string]any{"store": "yes"}}
	if hasManifestStorePermission(m) {
		t.Error("Config[store] of wrong type should not report declared")
	}
}

// TestPermissions_NotificationGate_Good — gui.notification.send is
// allowed when permissions.notifications: true is declared.
func TestPermissions_NotificationGate_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Notifications: true},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.notification.send"); !e.Allowed {
		t.Errorf("gui.notification.send should be allowed; reason=%q", e.Reason)
	}
}

// TestPermissions_NotificationGate_Bad — undeclared notification
// permission denies in prod mode and emits a reason.
func TestPermissions_NotificationGate_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	e := c.Entitled("gui.notification.send")
	if e.Allowed {
		t.Error("gui.notification.send should be denied without notifications permission")
	}
	if e.Reason == "" {
		t.Error("denial should explain why")
	}
}

// TestPermissions_NotificationGate_Ugly — dev mode allows-with-reason
// even on missing notification permission.
func TestPermissions_NotificationGate_Ugly(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeDev); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	e := c.Entitled("gui.notification.send")
	if !e.Allowed {
		t.Error("dev mode should allow even without notifications permission")
	}
	if e.Reason == "" {
		t.Error("dev mode should still surface the would-have-denied reason")
	}
}

// TestPermissions_ClipboardGate_Good — clipboard read and write are
// both allowed when permissions.clipboard: true is declared.
func TestPermissions_ClipboardGate_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Clipboard: true},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.clipboard.read"); !e.Allowed {
		t.Errorf("gui.clipboard.read should be allowed; reason=%q", e.Reason)
	}
	if e := c.Entitled("gui.clipboard.write"); !e.Allowed {
		t.Errorf("gui.clipboard.write should be allowed; reason=%q", e.Reason)
	}
}

// TestPermissions_ClipboardGate_Bad — clipboard gates are denied when
// the permission is undeclared.
func TestPermissions_ClipboardGate_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.clipboard.read"); e.Allowed {
		t.Error("gui.clipboard.read should be denied without clipboard permission")
	}
	if e := c.Entitled("gui.clipboard.write"); e.Allowed {
		t.Error("gui.clipboard.write should be denied without clipboard permission")
	}
}

// TestPermissions_ClipboardGate_Ugly — clipboard read+write share one
// declaration; declaring it for one direction enables the other.
func TestPermissions_ClipboardGate_Ugly(t *testing.T) {
	// Direct hasPermission round-trip — both clipboard fields read from
	// the same Clipboard bool slot, so declaring it covers both.
	p := config.ViewPermissions{Clipboard: true}
	if !hasPermission(p, fieldClipboardRead) {
		t.Error("Clipboard=true should satisfy fieldClipboardRead")
	}
	if !hasPermission(p, fieldClipboardWrite) {
		t.Error("Clipboard=true should satisfy fieldClipboardWrite")
	}
}

// TestPermissions_DeviceGates_Good — Camera, Microphone and Location
// gates honour their declarations.
func TestPermissions_DeviceGates_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Camera:     true,
			Microphone: true,
			Run:        []string{"device.location"},
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	for _, name := range []string{
		"device.camera",
		"device.microphone",
		"device.location",
	} {
		if e := c.Entitled(name); !e.Allowed {
			t.Errorf("%s should be allowed; reason=%q", name, e.Reason)
		}
	}
}

// TestPermissions_DeviceGates_Bad — undeclared device permissions are
// rejected with a reason.
func TestPermissions_DeviceGates_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	for _, name := range []string{
		"device.camera",
		"device.microphone",
		"device.location",
	} {
		e := c.Entitled(name)
		if e.Allowed {
			t.Errorf("%s should be denied without declaration", name)
		}
		if e.Reason == "" {
			t.Errorf("%s denial should explain why", name)
		}
	}
}

// TestPermissions_DeviceGates_Ugly — location lives in the Run-list as
// `device.location`, so a stray Run entry doesn't accidentally grant a
// random device.* action.
func TestPermissions_DeviceGates_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Run: []string{"ffmpeg"}},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("device.location"); e.Allowed {
		t.Error("device.location should be denied — Run list does not contain device.location")
	}
}
