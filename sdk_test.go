// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestSdk_ParseSDKLanguage_Good — every supported alias resolves to the
// canonical SDKLanguage and round-trips through String().
func TestSdk_ParseSDKLanguage_Good(t *testing.T) {
	cases := map[string]SDKLanguage{
		"openapi":    SDKLanguageOpenAPI,
		"oas":        SDKLanguageOpenAPI,
		"swagger":    SDKLanguageOpenAPI,
		"ts":         SDKLanguageTypeScript,
		"typescript": SDKLanguageTypeScript,
		"go":         SDKLanguageGo,
		"golang":     SDKLanguageGo,
		"php":        SDKLanguagePHP,
		"python":     SDKLanguagePython,
		"py":         SDKLanguagePython,
	}
	for in, want := range cases {
		got, ok := ParseSDKLanguage(in)
		if !ok {
			t.Errorf("ParseSDKLanguage(%q) ok=false", in)
			continue
		}
		if got != want {
			t.Errorf("ParseSDKLanguage(%q) = %v; want %v", in, got, want)
		}
	}
}

// TestSdk_ParseSDKLanguage_Bad — unknown tokens return ok=false rather
// than silently picking a default.
func TestSdk_ParseSDKLanguage_Bad(t *testing.T) {
	if _, ok := ParseSDKLanguage(""); ok {
		t.Error("ParseSDKLanguage('') should be !ok")
	}
	if _, ok := ParseSDKLanguage("rust"); ok {
		t.Error("ParseSDKLanguage('rust') should be !ok")
	}
}

// TestSdk_ParseSDKLanguage_Ugly — case insensitivity and whitespace
// trimming match the CLI argv parsing convention.
func TestSdk_ParseSDKLanguage_Ugly(t *testing.T) {
	if got, ok := ParseSDKLanguage("  TS  "); !ok || got != SDKLanguageTypeScript {
		t.Errorf("ParseSDKLanguage('  TS  ') = (%v, %v); want (ts, true)", got, ok)
	}
}

// TestSdk_String_Good — every variant returns the canonical CLI token so
// the round-trip with ParseSDKLanguage is symmetric.
func TestSdk_String_Good(t *testing.T) {
	cases := map[SDKLanguage]string{
		SDKLanguageOpenAPI:    "openapi",
		SDKLanguageTypeScript: "ts",
		SDKLanguageGo:         "go",
		SDKLanguagePHP:        "php",
		SDKLanguagePython:     "python",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Errorf("%v.String() = %q; want %q", in, got, want)
		}
	}
}

// TestSdk_String_Bad — an out-of-range value falls back to "unknown"
// rather than panicking.
func TestSdk_String_Bad(t *testing.T) {
	if got := SDKLanguage(99).String(); got != "unknown" {
		t.Errorf("SDKLanguage(99).String() = %q; want 'unknown'", got)
	}
}

// TestSdk_String_Ugly — ParseSDKLanguage of String() round-trips for the
// canonical five values, which is the contract callers rely on.
func TestSdk_String_Ugly(t *testing.T) {
	for _, lang := range []SDKLanguage{
		SDKLanguageOpenAPI, SDKLanguageTypeScript,
		SDKLanguageGo, SDKLanguagePHP, SDKLanguagePython,
	} {
		got, ok := ParseSDKLanguage(lang.String())
		if !ok || got != lang {
			t.Errorf("round-trip lang=%v ok=%v got=%v", lang, ok, got)
		}
	}
}

// TestSdk_SelectActions_Good — only actions whose Permission matches the
// manifest's declared permission slot (or actions that need none) are
// included.
func TestSdk_SelectActions_Good(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
		},
	}
	actions := SelectActions(DefaultSDKActions(), m, false)

	hasRead := false
	hasWrite := false
	hasNoPerm := false
	for _, a := range actions {
		switch a.Name {
		case "fs.read":
			hasRead = true
		case "fs.write":
			hasWrite = true
		case "gui.dialog.confirm":
			hasNoPerm = true
		}
	}
	if !hasRead {
		t.Error("fs.read should be included when read is declared")
	}
	if hasWrite {
		t.Error("fs.write should NOT be included when only read is declared")
	}
	if !hasNoPerm {
		t.Error("gui.dialog.confirm (no permission) should always be included")
	}
}

// TestSdk_SelectActions_Bad — nil manifest with includeAll=false still
// returns the unfiltered list (the function treats nil as "I cannot
// filter, hand everything back").
func TestSdk_SelectActions_Bad(t *testing.T) {
	out := SelectActions(DefaultSDKActions(), nil, false)
	if len(out) != len(DefaultSDKActions()) {
		t.Errorf("nil manifest filter dropped actions: got %d want %d",
			len(out), len(DefaultSDKActions()))
	}
}

// TestSdk_SelectActions_Ugly — an empty manifest with includeAll=true
// emits everything; with includeAll=false it emits only the no-permission
// actions.
func TestSdk_SelectActions_Ugly(t *testing.T) {
	all := SelectActions(DefaultSDKActions(), &config.ViewManifest{}, true)
	if len(all) != len(DefaultSDKActions()) {
		t.Errorf("includeAll=true should emit everything; got %d", len(all))
	}

	none := SelectActions(DefaultSDKActions(), &config.ViewManifest{}, false)
	for _, a := range none {
		if a.Permission != "" {
			t.Errorf("no-permission filter leaked %q (perm=%q)", a.Name, a.Permission)
		}
	}
	// And that there is at least one action with no permission so the
	// test isn't vacuously true.
	if len(none) == 0 {
		t.Error("expected at least one no-permission action in the catalogue")
	}
}

// TestSdk_RenderOpenAPI_Good — the emitted spec advertises the right
// version, info block and a path for every action.
func TestSdk_RenderOpenAPI_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "test", Name: "Test", Version: "1.2.3"}
	actions := []SDKAction{
		{
			Name: "fs.read", Permission: "read",
			Request: []SDKArg{{Name: "path", Type: "string", Required: true}},
		},
	}
	body, err := RenderOpenAPI(m, actions)
	if err != nil {
		t.Fatalf("RenderOpenAPI: %v", err)
	}
	if !core.Contains(body, `"openapi": "3.1.0"`) {
		t.Errorf("missing openapi version: %s", body)
	}
	if !core.Contains(body, `"/actions/fs.read"`) {
		t.Errorf("missing path: %s", body)
	}
	if !core.Contains(body, `"x-permission": "read"`) {
		t.Errorf("missing x-permission: %s", body)
	}
	if !core.Contains(body, `"version": "1.2.3"`) {
		t.Errorf("missing version: %s", body)
	}
}

// TestSdk_RenderOpenAPI_Bad — nil manifest is rejected before any work.
func TestSdk_RenderOpenAPI_Bad(t *testing.T) {
	if _, err := RenderOpenAPI(nil, nil); err == nil {
		t.Fatal("RenderOpenAPI(nil) should error")
	}
}

// TestSdk_RenderOpenAPI_Ugly — actions with no permission still emit
// `x-permission: none` so downstream tooling can rely on the field
// being present.
func TestSdk_RenderOpenAPI_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	actions := []SDKAction{{Name: "i18n.translate"}}
	body, err := RenderOpenAPI(m, actions)
	if err != nil {
		t.Fatalf("RenderOpenAPI: %v", err)
	}
	if !core.Contains(body, `"x-permission": "none"`) {
		t.Errorf("missing x-permission: none for an action without permission: %s", body)
	}
}

// TestSdk_RenderTypeScript_Good — the generated TS contains a typed
// wrapper for each action with the right interface names.
func TestSdk_RenderTypeScript_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "test", Version: "0.1.0"}
	actions := []SDKAction{
		{
			Name: "fs.read", Permission: "read",
			Request:  []SDKArg{{Name: "path", Type: "string", Required: true}},
			Response: []SDKArg{{Name: "content", Type: "string"}},
		},
	}
	out := RenderTypeScript(m, actions)
	for _, want := range []string{
		"export interface FsReadRequest",
		"path: string;",
		"export interface FsReadResponse",
		"content: string;",
		"export async function fsRead",
		`Core.Action("fs.read", opts)`,
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

// TestSdk_RenderTypeScript_Bad — empty actions still emits the runtime
// declaration so the file imports cleanly.
func TestSdk_RenderTypeScript_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	out := RenderTypeScript(m, nil)
	if !core.Contains(out, "declare const Core") {
		t.Errorf("expected runtime declaration even with no actions: %s", out)
	}
}

// TestSdk_RenderTypeScript_Ugly — actions without a response shape emit
// a Promise<void> wrapper rather than a phantom Response interface.
func TestSdk_RenderTypeScript_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	actions := []SDKAction{
		{
			Name: "fs.delete", Permission: "write",
			Request: []SDKArg{{Name: "path", Type: "string", Required: true}},
		},
	}
	out := RenderTypeScript(m, actions)
	if !core.Contains(out, "Promise<void>") {
		t.Errorf("expected Promise<void> for response-less action: %s", out)
	}
	if core.Contains(out, "FsDeleteResponse") {
		t.Errorf("response-less action leaked Response interface: %s", out)
	}
}

// TestSdk_RenderGo_Good — the Go source carries the expected package
// identifier, CoreClient interface and one method per action.
func TestSdk_RenderGo_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "test", Version: "0.1.0"}
	actions := []SDKAction{
		{Name: "fs.read", Permission: "read"},
	}
	out := RenderGo(m, actions)
	for _, want := range []string{
		"package sdk",
		"type CoreClient interface",
		"func (s *SDK) FsRead(",
		`s.client.Action(ctx, "fs.read", opts)`,
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

// TestSdk_RenderGo_Bad — empty action list still emits a usable package
// stub (CoreClient + SDK type), so a no-permission manifest produces a
// valid Go file rather than nothing.
func TestSdk_RenderGo_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	out := RenderGo(m, nil)
	for _, want := range []string{"package sdk", "type CoreClient interface"} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in stub output: %s", want, out)
		}
	}
}

// TestSdk_RenderGo_Ugly — header carries the manifest identity so the
// generated file is self-describing in a downstream `git blame`.
func TestSdk_RenderGo_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "round-trip", Version: "9.8.7"}
	out := RenderGo(m, nil)
	if !core.Contains(out, "CoreApp: round-trip v9.8.7") {
		t.Errorf("missing identity comment: %s", out)
	}
}

// TestSdk_RenderPHP_Good — the PHP class carries the namespace, ctor,
// and one method per action.
func TestSdk_RenderPHP_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "test", Version: "0.1.0"}
	actions := []SDKAction{{Name: "store.get", Permission: "store"}}
	out := RenderPHP(m, actions)
	for _, want := range []string{
		"namespace Core\\Sdk;",
		"class Sdk",
		"public function storeGet(",
		"$this->core->action('store.get', $opts)",
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

// TestSdk_RenderPHP_Bad — an empty action list emits just the class
// stub (matches the Go renderer behaviour).
func TestSdk_RenderPHP_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	out := RenderPHP(m, nil)
	if !core.Contains(out, "class Sdk") {
		t.Errorf("missing class stub: %s", out)
	}
}

// TestSdk_RenderPHP_Ugly — strict types and the EUPL header are present
// so the generated file passes linters expecting them.
func TestSdk_RenderPHP_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	out := RenderPHP(m, nil)
	if !core.Contains(out, "declare(strict_types=1);") {
		t.Errorf("missing strict types: %s", out)
	}
	if !core.Contains(out, "SPDX-License-Identifier: EUPL-1.2") {
		t.Errorf("missing licence header: %s", out)
	}
}

// TestSdk_RenderPython_Good — the Python module carries the Protocol,
// the SDK class, and one snake_case method per Action. Matches RFC §8's
// "Client SDKs (TS, Python, Go, PHP)" pipeline output. The `Optional`
// spelling on the opts parameter keeps the module importable on Python
// 3.8 (the `X | Y` shorthand is a 3.10 syntax error).
func TestSdk_RenderPython_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "test", Version: "0.1.0"}
	actions := []SDKAction{
		{Name: "fs.read", Permission: "read", Description: "Read file"},
		{Name: "gui.dialog.confirm"},
	}
	out := RenderPython(m, actions)
	for _, want := range []string{
		"class CoreClient(Protocol):",
		"class SDK:",
		"def fs_read(",
		"def gui_dialog_confirm(",
		`self._client.action("fs.read"`,
		"Permission: read",
		"Optional[Dict[str, Any]] = None",
		"from typing import Any, Dict, Optional, Protocol",
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	// Guard against a regression to the 3.10-only `X | None` shorthand.
	if core.Contains(out, "Dict[str, Any] | None") {
		t.Errorf("Python SDK uses 3.10-only `| None` syntax; expected Optional[]:\n%s", out)
	}
}

// TestSdk_RenderPython_Bad — empty action list still emits a usable
// module (Protocol + SDK stub) so the output imports cleanly even when
// the manifest declares no permissions.
func TestSdk_RenderPython_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	out := RenderPython(m, nil)
	for _, want := range []string{
		"class CoreClient(Protocol):",
		"class SDK:",
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in stub output:\n%s", want, out)
		}
	}
}

// TestSdk_RenderPython_Ugly — header carries the EUPL marker and the
// manifest identity so the generated file is self-describing; the
// `from __future__ import annotations` line keeps 3.8+ compat.
func TestSdk_RenderPython_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "round-trip", Version: "9.8.7"}
	out := RenderPython(m, nil)
	for _, want := range []string{
		"# SPDX-License-Identifier: EUPL-1.2",
		"# CoreApp: round-trip v9.8.7",
		"from __future__ import annotations",
	} {
		if !core.Contains(out, want) {
			t.Errorf("missing %q in header: %s", want, out)
		}
	}
}

// TestSdk_pythonFnName_Good — canonical snake_case derivation mirrors
// operationIDOf's camelCase output (fs.read → fs_read, not fsRead).
func TestSdk_pythonFnName_Good(t *testing.T) {
	cases := map[string]string{
		"fs.read":                  "fs_read",
		"gui.dialog.confirm":       "gui_dialog_confirm",
		"process.stdout.subscribe": "process_stdout_subscribe",
	}
	for in, want := range cases {
		if got := pythonFnName(in); got != want {
			t.Errorf("pythonFnName(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestSdk_pythonFnName_Bad — empty input returns an empty string rather
// than "_" so the caller can detect the miss.
func TestSdk_pythonFnName_Bad(t *testing.T) {
	if got := pythonFnName(""); got != "" {
		t.Errorf("pythonFnName('') = %q; want empty string", got)
	}
}

// TestSdk_pythonFnName_Ugly — hyphens and spaces are normalised to
// underscores so exotic action names still produce valid identifiers.
func TestSdk_pythonFnName_Ugly(t *testing.T) {
	if got := pythonFnName("Weird-Action Name"); got != "weird_action_name" {
		t.Errorf("pythonFnName('Weird-Action Name') = %q; want 'weird_action_name'", got)
	}
}

// TestSdk_SDKGenerate_Good — a default SDKGenerate run writes one file
// per language under `<root>/.core/sdk/<lang>/`.
func TestSdk_SDKGenerate_Good(t *testing.T) {
	dir := t.TempDir()
	m := &config.ViewManifest{
		Code:    "play",
		Name:    "Play",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Read: []string{"./data/"},
		},
	}
	if err := SDKGenerate(coreio.Local, dir, m, SDKGenerateOptions{}); err != nil {
		t.Fatalf("SDKGenerate: %v", err)
	}
	for lang, file := range map[SDKLanguage]string{
		SDKLanguageOpenAPI:    "openapi.json",
		SDKLanguageTypeScript: "index.ts",
		SDKLanguageGo:         "sdk.go",
		SDKLanguagePHP:        "Sdk.php",
		SDKLanguagePython:     "sdk.py",
	} {
		path := core.Path(dir, ".core", "sdk", lang.String(), file)
		if !coreio.Local.Exists(path) {
			t.Errorf("missing %s output at %s", lang.String(), path)
		}
	}
}

// TestSdk_SDKGenerate_Bad — nil manifest and empty root are rejected
// before any filesystem write.
func TestSdk_SDKGenerate_Bad(t *testing.T) {
	if err := SDKGenerate(coreio.Local, "", nil, SDKGenerateOptions{}); err == nil {
		t.Fatal("SDKGenerate(nil manifest) should error")
	}
	if err := SDKGenerate(coreio.Local, "", &config.ViewManifest{Code: "x"}, SDKGenerateOptions{}); err == nil {
		t.Fatal("SDKGenerate(empty root) should error")
	}
}

// TestSdk_SDKGenerate_Ugly — explicit Languages + OutDir override the
// default so a CI script can emit only the language it ships.
func TestSdk_SDKGenerate_Ugly(t *testing.T) {
	dir := t.TempDir()
	m := &config.ViewManifest{Code: "single", Name: "Single", Version: "0.1.0"}
	if err := SDKGenerate(coreio.Local, dir, m, SDKGenerateOptions{
		Languages: []SDKLanguage{SDKLanguageTypeScript},
		OutDir:    core.Path(dir, "build", "sdk"),
	}); err != nil {
		t.Fatalf("SDKGenerate: %v", err)
	}
	want := core.Path(dir, "build", "sdk", "ts", "index.ts")
	if !coreio.Local.Exists(want) {
		t.Errorf("custom OutDir was ignored; expected %s", want)
	}
	// The other languages should NOT be emitted.
	other := core.Path(dir, "build", "sdk", "openapi", "openapi.json")
	if coreio.Local.Exists(other) {
		t.Errorf("language filter ignored — found %s", other)
	}
}

// TestSdk_DefaultSDKActions_Good — every primitive in the catalogue has a
// non-empty Name. Acts as a smoke test for the table.
func TestSdk_DefaultSDKActions_Good(t *testing.T) {
	for _, a := range DefaultSDKActions() {
		if a.Name == "" {
			t.Errorf("primitive action with empty Name: %+v", a)
		}
	}
}

// TestSdk_DefaultSDKActions_Bad — the catalogue is value-returned so a
// caller mutating the slice does not affect the next caller.
func TestSdk_DefaultSDKActions_Bad(t *testing.T) {
	first := DefaultSDKActions()
	first[0].Name = "mutated"
	again := DefaultSDKActions()
	if again[0].Name == "mutated" {
		t.Error("DefaultSDKActions returned a shared backing slice")
	}
}

// TestSdk_DefaultSDKActions_Ugly — the catalogue includes the four pillars
// from RFC §9.3 so SDK consumers can rely on them, plus the §9.4 RPC
// surface (process lifecycle, IPC bus, auth, crypto.pgp.*) that ports
// the dAppServer surface verbatim.
func TestSdk_DefaultSDKActions_Ugly(t *testing.T) {
	want := map[string]bool{
		"fs.read":            false,
		"store.get":          false,
		"net.fetch":          false,
		"gui.dialog.confirm": false,
		// RFC §16 mappings (PWA + Electron) — consumers expect these to
		// be present so the SDK can surface the same surface as the
		// permission gates.
		"gui.dialog.open":       false,
		"gui.dialog.save":       false,
		"gui.browser.open":      false,
		"gui.notification.send": false,
		"gui.clipboard.read":    false,
		"gui.clipboard.write":   false,
		"device.location":       false,
		"device.camera":         false,
		"device.microphone":     false,
		// RFC §9.4 — process lifecycle. process.run was already in the
		// surface; the rest extend it to cover the dAppServer
		// process-as-service contract.
		"process.run":              false,
		"process.add":              false,
		"process.start":            false,
		"process.stop":             false,
		"process.kill":             false,
		"process.list":             false,
		"process.get":              false,
		"process.stdout.subscribe": false,
		"process.stdin.write":      false,
		// RFC §9.4 — IPC / event bus. Always allowed (host-managed).
		"ipc.pub.publish":   false,
		"ipc.pub.subscribe": false,
		"ipc.req.send":      false,
		"ipc.push.send":     false,
		// RFC §9.4 — auth + crypto. Identity and PGP primitives.
		"auth.create":                false,
		"auth.login":                 false,
		"auth.delete":                false,
		"crypto.pgp.generateKeyPair": false,
		"crypto.pgp.encrypt":         false,
		"crypto.pgp.decrypt":         false,
		"crypto.pgp.sign":            false,
		"crypto.pgp.verify":          false,
	}
	for _, a := range DefaultSDKActions() {
		if _, ok := want[a.Name]; ok {
			want[a.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("default catalogue missing %q", name)
		}
	}
}

// TestSdk_SelectActions_DevicePermissions — when the manifest declares
// camera + microphone, the matching device.* actions are selected and
// the unrelated ones (location, notification) are filtered out.
func TestSdk_SelectActions_DevicePermissions(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Camera:     true,
			Microphone: true,
		},
	}
	actions := SelectActions(DefaultSDKActions(), m, false)

	hasCamera := false
	hasMic := false
	hasLocation := false
	hasNotification := false
	for _, a := range actions {
		switch a.Name {
		case "device.camera":
			hasCamera = true
		case "device.microphone":
			hasMic = true
		case "device.location":
			hasLocation = true
		case "gui.notification.send":
			hasNotification = true
		}
	}
	if !hasCamera {
		t.Error("device.camera should be included when camera is declared")
	}
	if !hasMic {
		t.Error("device.microphone should be included when microphone is declared")
	}
	if hasLocation {
		t.Error("device.location should NOT be included without a location declaration")
	}
	if hasNotification {
		t.Error("gui.notification.send should NOT be included without a notifications declaration")
	}
}

// TestSdk_SelectActions_NotificationsClipboard — Notifications + Clipboard
// declarations include the gui.* actions that depend on them.
func TestSdk_SelectActions_NotificationsClipboard(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Notifications: true,
			Clipboard:     true,
		},
	}
	actions := SelectActions(DefaultSDKActions(), m, false)
	want := map[string]bool{
		"gui.notification.send": false,
		"gui.clipboard.read":    false,
		"gui.clipboard.write":   false,
	}
	for _, a := range actions {
		if _, ok := want[a.Name]; ok {
			want[a.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected %q in selected actions", name)
		}
	}
}

// TestSdk_SelectActions_LocationViaRun — `device.location` declared via
// the legacy permissions.run entry surfaces device.location in the SDK.
func TestSdk_SelectActions_LocationViaRun(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Run: []string{"device.location"},
		},
	}
	actions := SelectActions(DefaultSDKActions(), m, false)
	for _, a := range actions {
		if a.Name == "device.location" {
			return
		}
	}
	t.Error("device.location should be selected when permissions.run lists it")
}

// TestSdk_operationIDOf_Good — dotted action names compress into camelCase
// identifiers ready for OpenAPI / TS / Go.
func TestSdk_operationIDOf_Good(t *testing.T) {
	cases := map[string]string{
		"fs.read":            "fsRead",
		"gui.dialog.confirm": "guiDialogConfirm",
		"store.delete":       "storeDelete",
		"plain":              "plain",
	}
	for in, want := range cases {
		if got := operationIDOf(in); got != want {
			t.Errorf("operationIDOf(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestSdk_operationIDOf_Bad — empty input returns empty without panic.
func TestSdk_operationIDOf_Bad(t *testing.T) {
	if got := operationIDOf(""); got != "" {
		t.Errorf("operationIDOf('') = %q; want empty", got)
	}
}

// TestSdk_operationIDOf_Ugly — underscores and hyphens are treated as
// separators just like dots.
func TestSdk_operationIDOf_Ugly(t *testing.T) {
	if got := operationIDOf("foo_bar-baz"); got != "fooBarBaz" {
		t.Errorf("operationIDOf('foo_bar-baz') = %q; want fooBarBaz", got)
	}
}

// TestSdk_tsTypeOf_Good — the JSON Schema → TypeScript map lines up with
// what the renderer emits.
func TestSdk_tsTypeOf_Good(t *testing.T) {
	cases := map[string]string{
		"string":  "string",
		"integer": "number",
		"number":  "number",
		"boolean": "boolean",
		"bool":    "boolean",
		"array":   "unknown[]",
		"object":  "Record<string, unknown>",
	}
	for in, want := range cases {
		if got := tsTypeOf(in); got != want {
			t.Errorf("tsTypeOf(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestSdk_tsTypeOf_Bad — empty input returns the unknown fallback.
func TestSdk_tsTypeOf_Bad(t *testing.T) {
	if got := tsTypeOf(""); got != "unknown" {
		t.Errorf("tsTypeOf('') = %q; want unknown", got)
	}
}

// TestSdk_tsTypeOf_Ugly — mixed case is handled (the renderer normalises
// before lookup).
func TestSdk_tsTypeOf_Ugly(t *testing.T) {
	if got := tsTypeOf("STRING"); got != "string" {
		t.Errorf("tsTypeOf('STRING') = %q; want string", got)
	}
}

// TestSdk_jsonSchemaFromArgs_Good — a populated args slice produces a
// schema with all fields present and required listed when applicable.
func TestSdk_jsonSchemaFromArgs_Good(t *testing.T) {
	args := []SDKArg{
		{Name: "path", Type: "string", Required: true},
		{Name: "depth", Type: "integer"},
	}
	got := jsonSchemaFromArgs(args)
	if got["type"] != "object" {
		t.Errorf("type = %v; want object", got["type"])
	}
	props, ok := got["properties"].(map[string]any)
	if !ok || len(props) != 2 {
		t.Errorf("properties = %v; want map of 2 entries", got["properties"])
	}
	required, ok := got["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Errorf("required = %v; want ['path']", got["required"])
	}
}

// TestSdk_jsonSchemaFromArgs_Bad — empty args produces an empty object
// schema (no `properties` / `required` fields), matching the OpenAPI
// "empty body" convention.
func TestSdk_jsonSchemaFromArgs_Bad(t *testing.T) {
	got := jsonSchemaFromArgs(nil)
	if got["type"] != "object" {
		t.Errorf("type = %v; want object", got["type"])
	}
	if _, has := got["properties"]; has {
		t.Errorf("empty schema should not include 'properties'")
	}
	if _, has := got["required"]; has {
		t.Errorf("empty schema should not include 'required'")
	}
}

// TestSdk_jsonSchemaFromArgs_Ugly — args without Required end up in the
// properties map but not in the required list.
func TestSdk_jsonSchemaFromArgs_Ugly(t *testing.T) {
	args := []SDKArg{{Name: "opt", Type: "string"}}
	got := jsonSchemaFromArgs(args)
	if _, ok := got["required"].([]string); ok {
		t.Error("schema should not list optional args as required")
	}
}

// TestSdk_renderSDK_Good — the dispatcher returns the right body and
// filename for every supported language.
func TestSdk_renderSDK_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	cases := map[SDKLanguage]string{
		SDKLanguageOpenAPI:    "openapi.json",
		SDKLanguageTypeScript: "index.ts",
		SDKLanguageGo:         "sdk.go",
		SDKLanguagePHP:        "Sdk.php",
	}
	for lang, want := range cases {
		body, file, err := renderSDK(lang, m, nil)
		if err != nil {
			t.Errorf("renderSDK(%v): %v", lang, err)
			continue
		}
		if file != want {
			t.Errorf("renderSDK(%v) file = %q; want %q", lang, file, want)
		}
		if body == "" {
			t.Errorf("renderSDK(%v) body is empty", lang)
		}
	}
}

// TestSdk_renderSDK_Bad — out-of-range language is rejected with a
// typed error rather than producing a partial file.
func TestSdk_renderSDK_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Version: "0.1.0"}
	if _, _, err := renderSDK(SDKLanguage(99), m, nil); err == nil {
		t.Fatal("renderSDK with bogus language should error")
	}
}

// TestSdk_renderSDK_Ugly — Go and PHP renderers tolerate empty action
// lists (a no-permission manifest still emits the package stub).
func TestSdk_renderSDK_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "stub", Version: "0.1.0"}
	for _, lang := range []SDKLanguage{SDKLanguageGo, SDKLanguagePHP} {
		body, _, err := renderSDK(lang, m, nil)
		if err != nil {
			t.Errorf("renderSDK(%v) with empty actions: %v", lang, err)
		}
		if body == "" {
			t.Errorf("renderSDK(%v) emitted empty stub", lang)
		}
	}
}

// TestSdk_SDKCatalogue_Good — the catalogue covers every primitive
// Action in DefaultSDKActions and every entry carries a non-empty Name
// + Tag. The deterministic sort order means a golden-file test could
// compare against a fixed expected list without flakes.
func TestSdk_SDKCatalogue_Good(t *testing.T) {
	got := SDKCatalogue()
	if len(got) != len(DefaultSDKActions()) {
		t.Fatalf("SDKCatalogue() has %d entries; DefaultSDKActions() has %d", len(got), len(DefaultSDKActions()))
	}
	for _, e := range got {
		if e.Name == "" {
			t.Errorf("catalogue entry with empty Name: %+v", e)
		}
		if e.Tag == "" {
			t.Errorf("catalogue entry with empty Tag: %+v", e)
		}
	}
}

// TestSdk_SDKCatalogue_Bad — entries are sorted lexicographically so
// CLI tables and golden-file tests stay stable across runs.
func TestSdk_SDKCatalogue_Bad(t *testing.T) {
	got := SDKCatalogue()
	for i := 1; i < len(got); i++ {
		if got[i-1].Name > got[i].Name {
			t.Errorf("SDKCatalogue() not sorted: %q after %q", got[i].Name, got[i-1].Name)
		}
	}
}

// TestSdk_SDKCatalogue_Ugly — the catalogue surfaces the permission
// field verbatim (empty string for ungated actions like i18n.translate)
// so downstream formatters can decide how to render "no permission".
func TestSdk_SDKCatalogue_Ugly(t *testing.T) {
	got := SDKCatalogue()
	sawFsRead := false
	sawI18nTranslate := false
	for _, e := range got {
		if e.Name == "fs.read" {
			sawFsRead = true
			if e.Permission != "read" {
				t.Errorf("fs.read permission = %q; want %q", e.Permission, "read")
			}
			if e.Tag != "fs" {
				t.Errorf("fs.read tag = %q; want %q", e.Tag, "fs")
			}
		}
		if e.Name == "i18n.translate" {
			sawI18nTranslate = true
			if e.Permission != "" {
				t.Errorf("i18n.translate permission = %q; want empty", e.Permission)
			}
		}
	}
	if !sawFsRead {
		t.Error("SDKCatalogue missing fs.read")
	}
	if !sawI18nTranslate {
		t.Error("SDKCatalogue missing i18n.translate")
	}
}
