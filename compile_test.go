// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"
	"time"

	core "dappco.re/go/core"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// TestCompile_Compile_Good — a populated view manifest compiles into a
// CompiledManifest that carries every field the runtime needs at boot
// (code, name, version, layout, slots → components map, permissions,
// timestamp, compiler identity).
func TestCompile_Compile_Good(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "photo-browser",
		Name:    "Photo Browser",
		Version: "0.1.0",
		Layout:  "HLCRF",
		Slots: map[string]any{
			"H": "nav-breadcrumb",
			"C": "photo-grid",
		},
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
		},
		Modules: []string{"core/media", "core/fs"},
	}

	fixed := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	cm, err := Compile(m, CompileOptions{
		Now:        func() time.Time { return fixed },
		CompiledBy: "core v0.8.0",
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	if cm.Code != "photo-browser" {
		t.Errorf("Code = %q; want %q", cm.Code, "photo-browser")
	}
	if cm.CompiledAt != "2026-03-27T10:00:00Z" {
		t.Errorf("CompiledAt = %q; want %q", cm.CompiledAt, "2026-03-27T10:00:00Z")
	}
	if cm.CompiledBy != "core v0.8.0" {
		t.Errorf("CompiledBy = %q; want %q", cm.CompiledBy, "core v0.8.0")
	}
	if len(cm.Slots) != 2 || cm.Slots["H"] != "nav-breadcrumb" || cm.Slots["C"] != "photo-grid" {
		t.Errorf("Slots mismatch: %+v", cm.Slots)
	}
	if cm.Components["nav-breadcrumb"].Tag != "nav-breadcrumb" {
		t.Errorf("Components[nav-breadcrumb].Tag = %q; want %q",
			cm.Components["nav-breadcrumb"].Tag, "nav-breadcrumb")
	}
	if !cm.Components["nav-breadcrumb"].Shadow {
		t.Error("default Shadow should be true")
	}
	if len(cm.Modules) != 2 {
		t.Errorf("Modules = %v; want 2 entries", cm.Modules)
	}
}

// TestCompile_Compile_Bad — nil manifest and missing required fields
// (code, name, version) are rejected with a coreerr.E scope so the CLI
// can print a helpful message.
func TestCompile_Compile_Bad(t *testing.T) {
	if _, err := Compile(nil, CompileOptions{}); err == nil {
		t.Fatal("Compile(nil) should error")
	}

	if _, err := Compile(&config.ViewManifest{Version: "0.1.0"}, CompileOptions{}); err == nil {
		t.Fatal("Compile without code should error")
	}
	if _, err := Compile(&config.ViewManifest{Code: "x", Version: "0.1.0"}, CompileOptions{}); err == nil {
		t.Fatal("Compile without name should error")
	}
	if _, err := Compile(&config.ViewManifest{Code: "x", Name: "X"}, CompileOptions{}); err == nil {
		t.Fatal("Compile without version should error")
	}
}

// TestCompile_Compile_Ugly — a slot whose component value is not a
// string (YAML flexibility permits any value) is rejected cleanly
// rather than being coerced into a bogus entry. Also covers the zero
// opts path to pin the default CompiledBy + Now behaviour.
func TestCompile_Compile_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "bad-slot",
		Name:    "Bad Slot",
		Version: "0.1.0",
		Slots: map[string]any{
			"C": 42, // int, not string — must fail
		},
	}
	if _, err := Compile(m, CompileOptions{}); err == nil {
		t.Fatal("Compile with non-string slot should error")
	}

	// Zero-opts happy path — defaults kick in. This pins the default
	// CompiledBy to CompiledVersion and Now() to a real timestamp.
	ok := &config.ViewManifest{Code: "ok", Name: "OK", Version: "0.1.0"}
	cm, err := Compile(ok, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile(zero opts) should succeed: %v", err)
	}
	if cm.CompiledBy != CompiledVersion {
		t.Errorf("CompiledBy default = %q; want %q", cm.CompiledBy, CompiledVersion)
	}
	if cm.CompiledAt == "" {
		t.Error("CompiledAt should be populated by default Now()")
	}
}

// TestCompile_resolveSlots_Good — a map of string-valued slots compiles
// into parallel slots + components maps.
func TestCompile_resolveSlots_Good(t *testing.T) {
	raw := map[string]any{
		"H": "header-bar",
		"C": "centre-view",
	}
	slots, components, err := resolveSlots(raw)
	if err != nil {
		t.Fatalf("resolveSlots: %v", err)
	}
	if slots["H"] != "header-bar" || slots["C"] != "centre-view" {
		t.Errorf("slots mismatch: %+v", slots)
	}
	if components["header-bar"].Tag != "header-bar" || !components["header-bar"].Shadow {
		t.Errorf("components[header-bar] = %+v", components["header-bar"])
	}
}

// TestCompile_resolveSlots_Bad — nil map yields (nil, nil, nil) so a
// headless CLI manifest compiles cleanly.
func TestCompile_resolveSlots_Bad(t *testing.T) {
	slots, components, err := resolveSlots(nil)
	if err != nil {
		t.Fatalf("resolveSlots(nil) should succeed: %v", err)
	}
	if slots != nil || components != nil {
		t.Errorf("expected nil maps; got slots=%+v components=%+v", slots, components)
	}
}

// TestCompile_resolveSlots_Ugly — a nil value inside the map is a YAML
// mistake (slot names a non-existent component); reject it explicitly.
func TestCompile_resolveSlots_Ugly(t *testing.T) {
	_, _, err := resolveSlots(map[string]any{"C": nil})
	if err == nil {
		t.Fatal("resolveSlots should reject a nil slot value")
	}
}

// TestCompile_WriteCompiled_Good — WriteCompiled emits core.json at the
// expected path; LoadCompiled round-trips the same identity back.
func TestCompile_WriteCompiled_Good(t *testing.T) {
	dir := t.TempDir()

	m := &config.ViewManifest{Code: "round-trip", Name: "Round Trip", Version: "0.1.0"}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(coreio.Local, dir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	path := core.Path(dir, CompiledFileName)
	if !coreio.Local.Exists(path) {
		t.Fatalf("core.json not written at %q", path)
	}

	back, err := LoadCompiled(coreio.Local, dir)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if back.Code != "round-trip" || back.Name != "Round Trip" || back.Version != "0.1.0" {
		t.Errorf("LoadCompiled lost identity: %+v", back)
	}
}

// TestCompile_WriteCompiled_Bad — nil compiled manifest is rejected
// before any filesystem writes; nil medium falls back to coreio.Local.
func TestCompile_WriteCompiled_Bad(t *testing.T) {
	if err := WriteCompiled(coreio.Local, t.TempDir(), nil); err == nil {
		t.Fatal("WriteCompiled(nil) should error")
	}

	// Nil medium → default Local. Prove we don't crash.
	dir := t.TempDir()
	cm, err := Compile(&config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0"}, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(nil, dir, cm); err != nil {
		t.Fatalf("WriteCompiled(nil medium) should fall back: %v", err)
	}
}

// TestCompile_WriteCompiled_Ugly — pretty-printed output is still valid
// JSON that LoadCompiled can read back. Serves as a regression guard
// against the hand-rolled indentJSON walker.
func TestCompile_WriteCompiled_Ugly(t *testing.T) {
	dir := t.TempDir()
	m := &config.ViewManifest{
		Code:    "pretty",
		Name:    "Pretty",
		Version: "0.1.0",
		Layout:  "HLCRF",
		Slots: map[string]any{
			"H": "header-{escaped}",
			"C": `content-"double-quoted"`,
		},
		Permissions: config.ViewPermissions{
			Read: []string{"./data/", "./config/"},
		},
		Modules: []string{"core/fs"},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(coreio.Local, dir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	back, err := LoadCompiled(coreio.Local, dir)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if back.Slots["C"] != `content-"double-quoted"` {
		t.Errorf("quote-escape lost: %+v", back.Slots)
	}
	if len(back.Permissions.Read) != 2 {
		t.Errorf("permissions array truncated: %+v", back.Permissions.Read)
	}
}

// TestCompile_LoadCompiled_Good — the happy path round-trips and was
// already exercised by WriteCompiled_Good, but this pins the direct
// LoadCompiled contract (no Write before the call).
func TestCompile_LoadCompiled_Good(t *testing.T) {
	dir := t.TempDir()
	path := core.Path(dir, CompiledFileName)
	if err := coreio.Local.Write(path, `{"code":"hand-crafted","name":"Hand Crafted","version":"0.9.0","compiled_at":"2026-01-01T00:00:00Z","compiled_by":"test","permissions":{}}`); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cm, err := LoadCompiled(coreio.Local, dir)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if cm.Code != "hand-crafted" || cm.Version != "0.9.0" {
		t.Errorf("LoadCompiled mismatch: %+v", cm)
	}
}

// TestCompile_LoadCompiled_Bad — no core.json present is an error, not
// a zero value. The caller must see the difference.
func TestCompile_LoadCompiled_Bad(t *testing.T) {
	if _, err := LoadCompiled(coreio.Local, t.TempDir()); err == nil {
		t.Fatal("LoadCompiled on empty dir should error")
	}
}

// TestCompile_LoadCompiled_Ugly — malformed JSON surfaces the decode
// error rather than returning a partial struct.
func TestCompile_LoadCompiled_Ugly(t *testing.T) {
	dir := t.TempDir()
	path := core.Path(dir, CompiledFileName)
	if err := coreio.Local.Write(path, "{not valid json"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := LoadCompiled(coreio.Local, dir); err == nil {
		t.Fatal("LoadCompiled on malformed JSON should error")
	}
}

// TestCompile_indentJSON_Good — compact JSON is rewritten with two-space
// indentation on opening braces and commas.
func TestCompile_indentJSON_Good(t *testing.T) {
	out := indentJSON([]byte(`{"a":1,"b":[2,3]}`))
	want := "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}"
	if out != want {
		t.Errorf("indent mismatch:\n got: %q\nwant: %q", out, want)
	}
}

// TestCompile_indentJSON_Bad — an empty input returns empty output
// without panicking.
func TestCompile_indentJSON_Bad(t *testing.T) {
	if got := indentJSON(nil); got != "" {
		t.Errorf("indentJSON(nil) = %q; want empty", got)
	}
}

// TestCompile_indentJSON_Ugly — strings with braces/commas/quotes are
// not reformatted — only the outer structure is indented. This is the
// reason the walker tracks inStr / escape state.
func TestCompile_indentJSON_Ugly(t *testing.T) {
	in := []byte(`{"k":"val,ue with \"quotes\" and {braces}"}`)
	out := indentJSON(in)
	if !core.Contains(out, `"val,ue with \"quotes\" and {braces}"`) {
		t.Errorf("string contents mutated: %q", out)
	}
}

// TestCompile_Compile_Config_Good — Config is copied into the
// CompiledManifest so template blocks (RFC §4.1 step 6) and Config-backed
// permission flags survive the compile→core.json round-trip. Without
// this the runtime would silently lose `config:` entries when booting
// from the compiled artefact.
func TestCompile_Compile_Config_Good(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "c-app",
		Name:    "C App",
		Version: "0.1.0",
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars":     map[string]any{"size": "256"},
			},
			"store": true,
			"write": []any{"./photos/.thumbnails/"},
		},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cm.Config == nil {
		t.Fatal("Config missing from CompiledManifest")
	}
	if cm.Config["store"] != true {
		t.Errorf("Config[store] = %v; want true", cm.Config["store"])
	}
	if _, ok := cm.Config["thumbnails"]; !ok {
		t.Error("Config[thumbnails] missing — template block was dropped")
	}
}

// TestCompile_Compile_Config_Bad — an empty Config map produces a nil
// Config slot on the CompiledManifest so the JSON `omitempty` rule fires
// and core.json doesn't carry a redundant `"config": {}`.
func TestCompile_Compile_Config_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Code: "empty", Name: "Empty", Version: "0.1.0",
		Config: map[string]any{},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cm.Config != nil {
		t.Errorf("Config should be nil for empty input; got %v", cm.Config)
	}
}

// TestCompile_Compile_Config_Ugly — mutating the source manifest after
// Compile does not leak into the CompiledManifest. The copyConfig
// helper does a shallow clone so the caller owns their copy.
func TestCompile_Compile_Config_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code: "iso", Name: "Iso", Version: "0.1.0",
		Config: map[string]any{"k": "original"},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	m.Config["k"] = "mutated"
	if cm.Config["k"] != "original" {
		t.Errorf("CompiledManifest.Config was aliased: got %q", cm.Config["k"])
	}
}

// TestCompile_WriteCompiled_CompatibilityFields_Good confirms the
// RFC-facing core.json layout hoists compatibility fields back out of
// Config instead of hiding them all under `config`.
func TestCompile_WriteCompiled_CompatibilityFields_Good(t *testing.T) {
	dir := t.TempDir()
	m := &config.ViewManifest{
		Code:    "compat",
		Name:    "Compat",
		Version: "0.1.0",
		Config: map[string]any{
			"store":    true,
			"write":    []any{"./cache/"},
			"services": []any{"store", "notification"},
			"type":     "pwa",
			"url":      "https://play.example.com/",
			"source":   "wrap:pwa:https://play.example.com/",
		},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(coreio.Local, dir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	body, err := coreio.Local.Read(core.Path(dir, CompiledFileName))
	if err != nil {
		t.Fatalf("Read core.json: %v", err)
	}
	var raw map[string]any
	if r := core.JSONUnmarshal([]byte(body), &raw); !r.OK {
		t.Fatalf("Decode core.json: %v", r.Value)
	}

	perms, ok := raw["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions = %T; want object", raw["permissions"])
	}
	if perms["store"] != true {
		t.Errorf("permissions.store = %v; want true", perms["store"])
	}
	writeList, ok := perms["write"].([]any)
	if !ok || len(writeList) != 1 || writeList[0] != "./cache/" {
		t.Errorf("permissions.write = %v; want [./cache/]", perms["write"])
	}
	if raw["type"] != "pwa" {
		t.Errorf("type = %v; want pwa", raw["type"])
	}
	if raw["url"] != "https://play.example.com/" {
		t.Errorf("url = %v; want https://play.example.com/", raw["url"])
	}
	services, ok := raw["services"].([]any)
	if !ok || len(services) != 2 {
		t.Fatalf("services = %v; want [store notification]", raw["services"])
	}
	cfg, ok := raw["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %T; want object", raw["config"])
	}
	if cfg["source"] != "wrap:pwa:https://play.example.com/" {
		t.Errorf("config.source = %v; want wrap source", cfg["source"])
	}
	if _, ok := cfg["store"]; ok {
		t.Error("config.store should be hoisted into permissions")
	}
	if _, ok := cfg["services"]; ok {
		t.Error("config.services should be hoisted to the top level")
	}
}

// TestCompile_WriteCompiled_CompatibilityFields_DeviceGates confirms
// the compiled core.json permission object preserves the RFC-native
// device permission keys and does not re-emit the compatibility-only
// `device.location` run entry.
func TestCompile_WriteCompiled_CompatibilityFields_DeviceGates(t *testing.T) {
	dir := t.TempDir()
	m := &config.ViewManifest{
		Code:    "device-compat",
		Name:    "Device Compat",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Run:        []string{"ffmpeg", "device.location"},
			Camera:     true,
			Microphone: true,
		},
		Config: map[string]any{
			"device_gates": map[string]any{
				"device.camera":     true,
				"device.microphone": true,
				"device.location":   true,
			},
		},
	}
	cm, err := Compile(m, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(coreio.Local, dir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	body, err := coreio.Local.Read(core.Path(dir, CompiledFileName))
	if err != nil {
		t.Fatalf("Read core.json: %v", err)
	}
	var raw map[string]any
	if r := core.JSONUnmarshal([]byte(body), &raw); !r.OK {
		t.Fatalf("Decode core.json: %v", r.Value)
	}

	perms, ok := raw["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions = %T; want object", raw["permissions"])
	}
	for _, key := range []string{"device.camera", "device.microphone", "device.location"} {
		if perms[key] != true {
			t.Errorf("permissions[%q] = %v; want true", key, perms[key])
		}
	}
	runList, ok := perms["run"].([]any)
	if !ok || len(runList) != 1 || runList[0] != "ffmpeg" {
		t.Errorf("permissions.run = %v; want [ffmpeg]", perms["run"])
	}
}

// TestCompile_LoadCompiled_CompatibilityFields_Good confirms
// LoadCompiled accepts the RFC-facing core.json shape and folds the
// compatibility fields back into the in-memory Config / Permissions
// form the runtime uses today.
func TestCompile_LoadCompiled_CompatibilityFields_Good(t *testing.T) {
	dir := t.TempDir()
	path := core.Path(dir, CompiledFileName)
	body := `{
		"code":"compat-load",
		"name":"Compat Load",
		"version":"0.1.0",
		"compiled_at":"2026-04-15T00:00:00Z",
		"compiled_by":"test",
		"type":"pwa",
		"url":"https://play.example.com/",
		"services":["store"],
		"permissions":{"read":["./"],"write":["./cache/"],"store":true}
	}`
	if err := coreio.Local.Write(path, body); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cm, err := LoadCompiled(coreio.Local, dir)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if cm.Config["store"] != true {
		t.Errorf("Config[store] = %v; want true", cm.Config["store"])
	}
	if cm.Config["type"] != "pwa" {
		t.Errorf("Config[type] = %v; want pwa", cm.Config["type"])
	}
	if cm.Config["url"] != "https://play.example.com/" {
		t.Errorf("Config[url] = %v; want https://play.example.com/", cm.Config["url"])
	}
	writeList, ok := cm.Config["write"].([]any)
	if !ok || len(writeList) != 1 || writeList[0] != "./cache/" {
		t.Errorf("Config[write] = %v; want [./cache/]", cm.Config["write"])
	}
}

// TestCompile_copyConfig_Good — copyConfig returns an independent map
// with the same keys so mutations on the source don't reach the copy.
func TestCompile_copyConfig_Good(t *testing.T) {
	src := map[string]any{"a": 1, "b": "two"}
	dst := copyConfig(src)
	if len(dst) != 2 {
		t.Fatalf("len(dst) = %d; want 2", len(dst))
	}
	dst["a"] = 99
	if src["a"] != 1 {
		t.Errorf("source mutated via copy: %v", src["a"])
	}
}

// TestCompile_copyConfig_Bad — nil and empty inputs yield a nil output
// so the caller's JSON omitempty tag fires.
func TestCompile_copyConfig_Bad(t *testing.T) {
	if got := copyConfig(nil); got != nil {
		t.Errorf("copyConfig(nil) = %v; want nil", got)
	}
	if got := copyConfig(map[string]any{}); got != nil {
		t.Errorf("copyConfig({}) = %v; want nil", got)
	}
}

// TestCompile_copyConfig_Ugly — values are copied by reference (shallow
// clone). Nested maps are NOT deep-copied; that matches the Go convention
// and keeps Compile allocation-light.
func TestCompile_copyConfig_Ugly(t *testing.T) {
	nested := map[string]any{"inner": 1}
	src := map[string]any{"n": nested}
	dst := copyConfig(src)
	nested["inner"] = 2
	if got := dst["n"].(map[string]any)["inner"]; got != 2 {
		t.Errorf("nested map should be aliased (shallow copy); got %v", got)
	}
}
