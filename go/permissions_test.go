// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// captureLog redirects the default core logger to a buffer for the
// duration of fn, then asserts the emitted log body contains `phrase`
// exactly `wantCount` times. The default logger is process-global so
// this helper restores stderr when fn returns — parallel tests that
// share the default logger should serialise their captures rather than
// race each other.
//
// Captured body is surfaced on mismatch so the failing test can show
// what was actually emitted.
func captureLog(t *testing.T, fn func(), phrase string, wantCount int) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := core.Default()
	logger.SetOutput(buf)
	defer logger.SetOutput(stderrFallback())

	fn()

	got := strings.Count(buf.String(), phrase)
	if got != wantCount {
		t.Errorf("expected %d occurrences of %q in log output, got %d\n--log--\n%s\n-------",
			wantCount, phrase, got, buf.String())
	}
}

// stderrFallback returns the Writer the default logger was pointing at
// before captureLog redirected it. `core.Default()` does not expose an
// accessor for the current Writer, so the fallback goes to `io.Discard`
// — the test harness already surfaces logs via t.Logf elsewhere and we
// would rather drop test noise than pick a Writer that might surprise a
// caller with a closed handle.
func stderrFallback() io.Writer {
	return io.Discard
}

func TestPermissions_Field_String_Good(t *testing.T) {
	cases := []struct {
		field permissionField
		want  string
	}{
		{fieldRead, "read"},
		{fieldWrite, "write"},
		{fieldNet, "net"},
		{fieldRun, "run"},
		{fieldStore, "store"},
		{fieldNotification, "notifications"},
		{fieldClipboardRead, "clipboard"},
		{fieldDialogOpen, "gui.dialog.open"},
		{fieldCamera, "camera"},
		{fieldLocation, "location"},
	}
	for _, tc := range cases {
		if got := tc.field.String(); got != tc.want {
			t.Fatalf("%v.String() = %q; want %q", tc.field, got, tc.want)
		}
	}
}

func TestPermissions_Field_String_Bad(t *testing.T) {
	if got := permissionField(99).String(); got != "unknown" {
		t.Fatalf("permissionField(99).String() = %q; want unknown", got)
	}
}

func TestPermissions_Field_String_Ugly(t *testing.T) {
	if got := permissionField(-1).String(); got != "unknown" {
		t.Fatalf("permissionField(-1).String() = %q; want unknown", got)
	}
}

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
// when `permissions.store: true` is not declared.
func TestPermissions_newCheckerForManifest_Bad(t *testing.T) {
	checker := newCheckerForManifest(&config.ViewManifest{}, ModeProd)
	e := checker("store.get", 1, context.Background())
	if e.Allowed {
		t.Error("store.get should be denied without explicit store permission")
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

	// Filesystem access does not imply store access.
	m2 := &config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}
	checker2 := newCheckerForManifest(m2, ModeProd)
	e2 := checker2("store.delete", 1, context.Background())
	if e2.Allowed {
		t.Error("Filesystem=true should not satisfy the store gate")
	}
}

// TestPermissions_hasManifestStorePermission_Good — explicit
// `permissions.store: true` reports declared.
func TestPermissions_hasManifestStorePermission_Good(t *testing.T) {
	m := &config.ViewManifest{Config: map[string]any{"store": true}}
	if !hasManifestStorePermission(m) {
		t.Error("Config[store]=true should report declared")
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
	if hasManifestStorePermission(&config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}) {
		t.Error("Filesystem=true should not report declared store access")
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

// TestPermissions_WriteList_Good — Config["write"] satisfies the
// fs.write entitlement gate without flipping the catch-all Filesystem
// flag. Mirrors RFC §2.2 `write: ["./photos/.thumbnails/"]` example.
func TestPermissions_WriteList_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"write": []any{"./photos/.thumbnails/"},
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("fs.write"); !e.Allowed {
		t.Errorf("fs.write should be allowed when Config[write] is set; reason=%q", e.Reason)
	}
	if e := c.Entitled("fs.delete"); !e.Allowed {
		t.Errorf("fs.delete should be allowed (write gate) when Config[write] is set; reason=%q", e.Reason)
	}
}

// TestPermissions_WriteList_Bad — empty Config["write"] denies the
// write gate in prod mode (same as no permission at all).
func TestPermissions_WriteList_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"write": []any{}, // present but empty
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("fs.write"); e.Allowed {
		t.Error("fs.write should be denied when Config[write] is empty")
	}
}

// TestPermissions_WriteList_Ugly — Config["write"] of the wrong type
// doesn't crash the checker; the gate falls back to the slot check
// (which is also unset here, so the write is denied).
func TestPermissions_WriteList_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"write": "should-be-a-list",
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("fs.write"); e.Allowed {
		t.Error("fs.write should be denied when Config[write] is the wrong type")
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

// TestPermissions_ProcessLifecycle_Good — every dAppServer process verb
// (RFC §9.4) gates against permissions.run; when the manifest declares
// `run: ["miner"]` each verb is allowed.
func TestPermissions_ProcessLifecycle_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Run: []string{"miner"}},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	for _, action := range []string{
		"process.run",
		"process.add",
		"process.start",
		"process.stop",
		"process.kill",
		"process.list",
		"process.get",
		"process.stdout.subscribe",
		"process.stdin.write",
	} {
		if e := c.Entitled(action); !e.Allowed {
			t.Errorf("%s should be allowed when run is declared; reason=%q", action, e.Reason)
		}
	}
}

// TestPermissions_ProcessLifecycle_Bad — without a run declaration, every
// process verb is denied in prod mode with a reason.
func TestPermissions_ProcessLifecycle_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	for _, action := range []string{
		"process.add",
		"process.kill",
		"process.list",
		"process.get",
		"process.stdout.subscribe",
		"process.stdin.write",
	} {
		e := c.Entitled(action)
		if e.Allowed {
			t.Errorf("%s should be denied without a run declaration", action)
		}
		if e.Reason == "" {
			t.Errorf("%s denial should explain why", action)
		}
	}
}

// TestPermissions_ProcessLifecycle_Ugly — `process.kill` and similar are
// gated by the same prefix that `process.run` uses, so a manifest's Run
// list grants them collectively. This stops a manifest accidentally
// gating only `process.run` while a handler that needs `process.kill`
// would have been silently denied.
func TestPermissions_ProcessLifecycle_Ugly(t *testing.T) {
	gate, ok := gateFor("process.list")
	if !ok || gate.field != fieldRun {
		t.Errorf("process.list should be gated under fieldRun; got (%v, %v)", gate, ok)
	}
	gate, ok = gateFor("process.stdout.subscribe")
	if !ok || gate.field != fieldRun {
		t.Errorf("process.stdout.subscribe should be gated under fieldRun; got (%v, %v)", gate, ok)
	}
}

// TestPermissions_UngatedActions_Good — IPC, auth and crypto actions are
// not gated by the manifest (the host owns those surfaces). They must
// always pass the entitlement gate even when the manifest declares
// nothing.
func TestPermissions_UngatedActions_Good(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	for _, action := range []string{
		"ipc.pub.publish",
		"ipc.pub.subscribe",
		"ipc.req.send",
		"ipc.push.send",
		"auth.create",
		"auth.login",
		"auth.delete",
		"crypto.pgp.generateKeyPair",
		"crypto.pgp.encrypt",
		"crypto.pgp.decrypt",
		"crypto.pgp.sign",
		"crypto.pgp.verify",
	} {
		e := c.Entitled(action)
		if !e.Allowed {
			t.Errorf("%s should be allowed (ungated, host-managed); reason=%q", action, e.Reason)
		}
	}
}

// TestPermissions_UngatedActions_Bad — gateFor returns false for the
// host-managed surfaces so the entitlement walker treats them as
// always-allowed.
func TestPermissions_UngatedActions_Bad(t *testing.T) {
	for _, action := range []string{
		"ipc.pub.publish",
		"auth.login",
		"crypto.pgp.encrypt",
	} {
		if _, ok := gateFor(action); ok {
			t.Errorf("%s should NOT be gated (host-managed surface)", action)
		}
	}
}

// TestPermissions_UngatedActions_Ugly — the unknown-action escape hatch
// returns Unlimited so a custom action a host registers later is
// always-allowed without a permissions row. Mirrors the "unknown
// commands are not denied" rule from RFC §9.
func TestPermissions_UngatedActions_Ugly(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	e := c.Entitled("custom.host.action")
	if !e.Allowed || !e.Unlimited {
		t.Errorf("custom action should be Allowed+Unlimited; got %+v", e)
	}
}

// TestPermissions_DialogGates_Good — Electron-wrapped apps declare
// explicit gui dialog gates. When present, both dialog actions are
// entitled in prod mode.
func TestPermissions_DialogGates_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"gui_gates": map[string]any{
				"gui.dialog.open": true,
				"gui.dialog.save": true,
			},
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.dialog.open"); !e.Allowed {
		t.Errorf("gui.dialog.open should be allowed with gui_gates declared; reason=%q", e.Reason)
	}
	if e := c.Entitled("gui.dialog.save"); !e.Allowed {
		t.Errorf("gui.dialog.save should be allowed with gui_gates declared; reason=%q", e.Reason)
	}
}

// TestPermissions_DialogGates_Bad — without explicit gui dialog gates,
// both actions are denied in prod mode.
func TestPermissions_DialogGates_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.dialog.open"); e.Allowed {
		t.Errorf("gui.dialog.open should be denied without gui_gates; reason=%q", e.Reason)
	}
	if e := c.Entitled("gui.dialog.save"); e.Allowed {
		t.Errorf("gui.dialog.save should be denied without gui_gates; reason=%q", e.Reason)
	}
}

// TestPermissions_BrowserOpenGate_Good — `gui.browser.open` is its own
// entitlement. A wrapped manifest declaring the explicit gui gate lets
// the action through without widening the app into full `net.fetch`.
func TestPermissions_BrowserOpenGate_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"gui_gates": map[string]any{
				"gui.browser.open": true,
			},
		},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.browser.open"); !e.Allowed {
		t.Errorf("gui.browser.open should be allowed with gui_gates declared; reason=%q", e.Reason)
	}
	if e := c.Entitled("net.fetch"); e.Allowed {
		t.Errorf("gui.browser.open should NOT widen into net.fetch; reason=%q", e.Reason)
	}
}

// TestPermissions_BrowserOpenGate_RoundTrip keeps the YAML
// marshal→unmarshal path honest: a persisted gui.browser.open gate must
// survive reload without turning into the broader `network` capability.
func TestPermissions_BrowserOpenGate_RoundTrip(t *testing.T) {
	body, err := yamlMarshalBytes(&config.ViewManifest{
		Code:    "browser-wrap",
		Name:    "Browser Wrap",
		Version: "0.1.0",
		Config: map[string]any{
			"gui_gates": map[string]any{
				"gui.browser.open": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("yamlMarshalBytes: %v", err)
	}

	var round config.ViewManifest
	if err := UnmarshalViewManifest(body, &round); err != nil {
		t.Fatalf("UnmarshalViewManifest: %v", err)
	}
	if round.Permissions.Network {
		t.Fatal("gui.browser.open should not hydrate into permissions.network")
	}

	c := core.New()
	if err := permissions(c, &round, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.browser.open"); !e.Allowed {
		t.Errorf("gui.browser.open should stay allowed after round-trip; reason=%q", e.Reason)
	}
	if e := c.Entitled("net.fetch"); e.Allowed {
		t.Errorf("round-tripped gui.browser.open should NOT allow net.fetch; reason=%q", e.Reason)
	}
}

// TestPermissions_BrowserOpenGate_Bad — without either the explicit gui
// gate or a legacy `net` declaration, `gui.browser.open` is denied in
// prod mode.
func TestPermissions_BrowserOpenGate_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.browser.open"); e.Allowed {
		t.Errorf("gui.browser.open should be denied without gui.browser.open; reason=%q", e.Reason)
	}
}

// TestPermissions_BrowserOpenGate_Ugly — legacy manifests that still
// declare `net` for browser-open continue to work for compatibility.
func TestPermissions_BrowserOpenGate_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Net: []string{"*"}},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("gui.browser.open"); !e.Allowed {
		t.Errorf("legacy net declaration should still allow gui.browser.open; reason=%q", e.Reason)
	}
}

// TestPermissions_BrowserOpenGate_Dev — dev mode warns instead of
// denying, so a developer iterating without the explicit gate still sees
// the action fire but the reason surfaces the would-be denial.
func TestPermissions_BrowserOpenGate_Dev(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeDev); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	e := c.Entitled("gui.browser.open")
	if !e.Allowed {
		t.Errorf("dev mode should allow gui.browser.open; reason=%q", e.Reason)
	}
	if e.Reason == "" {
		t.Error("dev mode should still surface the would-be denial reason")
	}
}

// TestPermissions_BrainRecallGate_Good — `brain.recall` dispatches to
// OpenBrain over the network, so RFC §9.3 gates it behind the `net`
// permission. A manifest declaring `net` allows the action through.
func TestPermissions_BrainRecallGate_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Net: []string{"api.openbrain:443"}},
	}
	if err := permissions(c, m, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("brain.recall"); !e.Allowed {
		t.Errorf("brain.recall should be allowed with net declared; reason=%q", e.Reason)
	}
}

// TestPermissions_BrainRecallGate_Bad — without `net`, prod mode denies
// `brain.recall` so an app cannot silently query OpenBrain without
// declaring the network capability.
func TestPermissions_BrainRecallGate_Bad(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeProd); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	if e := c.Entitled("brain.recall"); e.Allowed {
		t.Errorf("brain.recall should be denied without net; reason=%q", e.Reason)
	}
}

// TestPermissions_BrainRecallGate_Ugly — dev mode lets the action
// through but surfaces the would-be denial on Entitlement.Reason so
// the developer can spot the missing manifest declaration in logs.
func TestPermissions_BrainRecallGate_Ugly(t *testing.T) {
	c := core.New()
	if err := permissions(c, &config.ViewManifest{}, ModeDev); err != nil {
		t.Fatalf("permissions: %v", err)
	}
	e := c.Entitled("brain.recall")
	if !e.Allowed {
		t.Errorf("dev mode should allow brain.recall; reason=%q", e.Reason)
	}
	if e.Reason == "" {
		t.Error("dev mode should still surface the would-be denial reason")
	}
}

// TestPermissions_DevModeWarnDedup_Good — the dev-mode permission
// checker emits `core.Warn` the first time an undeclared action is
// called but stays quiet on subsequent hits so a 500ms hot-reload loop
// does not produce one warning per tick (RFC §4.2).
//
// Captures stderr through a pipe because the default logger writes
// there; an easier API (`SetDefault(NewLog(WithOutput(buf)))`) is TBD in
// core/go — until then the pipe gives us a reliable assertion point.
func TestPermissions_DevModeWarnDedup_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "dedup-good"}
	captureLog(t, func() {
		c := core.New()
		if err := permissions(c, m, ModeDev); err != nil {
			t.Fatalf("permissions: %v", err)
		}
		// First call → expected to warn once.
		_ = c.Entitled("fs.read")
		// Repeat the same action → no new warning.
		_ = c.Entitled("fs.read")
	}, "permission gate bypassed in dev mode", 1)
}

// TestPermissions_DevModeWarnDedup_Bad — different actions each trigger
// their own warning so a developer sees every missing capability, not
// only the first one they exercised.
func TestPermissions_DevModeWarnDedup_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "dedup-bad"}
	captureLog(t, func() {
		c := core.New()
		if err := permissions(c, m, ModeDev); err != nil {
			t.Fatalf("permissions: %v", err)
		}
		_ = c.Entitled("fs.read")
		_ = c.Entitled("net.fetch")
		_ = c.Entitled("store.get")
	}, "permission gate bypassed in dev mode", 3)
}

// TestPermissions_DevModeWarnDedup_Ugly — prod mode does not emit
// warnings at all (the denial itself is the signal; logging here would
// be redundant with the caller's error path).
func TestPermissions_DevModeWarnDedup_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "dedup-ugly"}
	captureLog(t, func() {
		c := core.New()
		if err := permissions(c, m, ModeProd); err != nil {
			t.Fatalf("permissions: %v", err)
		}
		_ = c.Entitled("fs.read")
		_ = c.Entitled("fs.read")
	}, "permission gate bypassed in dev mode", 0)
}
