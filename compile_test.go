// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"
	"time"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
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
// (code, version) are rejected with a coreerr.E scope so the CLI can
// print a helpful message.
func TestCompile_Compile_Bad(t *testing.T) {
	if _, err := Compile(nil, CompileOptions{}); err == nil {
		t.Fatal("Compile(nil) should error")
	}

	if _, err := Compile(&config.ViewManifest{Version: "0.1.0"}, CompileOptions{}); err == nil {
		t.Fatal("Compile without code should error")
	}
	if _, err := Compile(&config.ViewManifest{Code: "x"}, CompileOptions{}); err == nil {
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
	ok := &config.ViewManifest{Code: "ok", Version: "0.1.0"}
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
	cm, err := Compile(&config.ViewManifest{Code: "x", Version: "0.1.0"}, CompileOptions{})
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
