// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestConfig_applyConfig_Good — a manifest declaring a template whose
// file exists on the medium triggers the render → write pipeline. The
// rendered output drops the `.tmpl` suffix, expanded vars are
// substituted, and the source template is left untouched.
func TestConfig_applyConfig_Good(t *testing.T) {
	root := t.TempDir()
	must(t, coreio.Local.EnsureDir(root+"/conf"))
	must(t, coreio.Local.Write(root+"/conf/thumbs.json.tmpl", `{"size": "{{ .size }}"}`))

	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars":     map[string]any{"size": "128"},
			},
		},
	}
	if err := applyConfig(c, m, coreio.Local, root); err != nil {
		t.Fatalf("applyConfig: %v", err)
	}

	// Rendered file must exist next to the template (suffix stripped).
	rendered, err := coreio.Local.Read(root + "/conf/thumbs.json")
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}
	if rendered != `{"size": "128"}` {
		t.Errorf("rendered = %q; want %q", rendered, `{"size": "128"}`)
	}

	// Source template must be untouched.
	src, err := coreio.Local.Read(root + "/conf/thumbs.json.tmpl")
	if err != nil {
		t.Fatalf("read source template: %v", err)
	}
	if src != `{"size": "{{ .size }}"}` {
		t.Errorf("source mutated: %q", src)
	}
}

// TestConfig_applyConfig_Bad — a declared template whose file does not
// exist fails the boot with a useful message.
func TestConfig_applyConfig_Bad(t *testing.T) {
	root := t.TempDir()
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"missing": map[string]any{
				"template": "conf/never-there.tmpl",
			},
		},
	}
	if err := applyConfig(c, m, coreio.Local, root); err == nil {
		t.Fatal("applyConfig should fail on missing template file")
	}
}

// TestConfig_applyConfig_Ugly — a config entry that isn't a map
// (scalar, list) is rejected rather than silently ignored.
func TestConfig_applyConfig_Ugly(t *testing.T) {
	root := t.TempDir()
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"wrong-shape": "this should be a map, not a string",
		},
	}
	if err := applyConfig(c, m, coreio.Local, root); err == nil {
		t.Fatal("applyConfig should reject non-map entries")
	}
}

// TestConfig_asTemplateEntry_Good — a well-shaped YAML map narrows.
func TestConfig_asTemplateEntry_Good(t *testing.T) {
	raw := map[string]any{
		"template": "foo.tmpl",
		"vars":     map[string]any{"key": "value"},
	}
	entry, ok := asTemplateEntry(raw)
	if !ok {
		t.Fatal("asTemplateEntry should accept a {template, vars} map")
	}
	if entry.Template != "foo.tmpl" {
		t.Errorf("Template = %q; want %q", entry.Template, "foo.tmpl")
	}
	if entry.Vars["key"] != "value" {
		t.Errorf("Vars[key] = %v; want %q", entry.Vars["key"], "value")
	}
}

// TestConfig_asTemplateEntry_Bad — a non-map value fails narrowing.
func TestConfig_asTemplateEntry_Bad(t *testing.T) {
	if _, ok := asTemplateEntry("not-a-map"); ok {
		t.Error("asTemplateEntry should reject a string")
	}
}

// TestConfig_asTemplateEntry_Ugly — a map missing both known keys
// still narrows (empty entry) — the caller decides whether to accept.
func TestConfig_asTemplateEntry_Ugly(t *testing.T) {
	entry, ok := asTemplateEntry(map[string]any{"unknown": "field"})
	if !ok {
		t.Fatal("asTemplateEntry should accept a map even with no template key")
	}
	if entry.Template != "" {
		t.Errorf("Template = %q; want empty", entry.Template)
	}
}

// TestConfig_renderTemplate_Good — `{{ .name }}` placeholders expand
// against the supplied vars in every supported spelling.
func TestConfig_renderTemplate_Good(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"port: {{ .port }}", "port: 9000"},
		{"port: {{.port}}", "port: 9000"},
		{"port: {{ .port}}", "port: 9000"},
		{"port: {{.port }}", "port: 9000"},
		{"port: {{port}}", "port: 9000"},
		{"port: {{ port }}", "port: 9000"},
	}
	for _, tc := range cases {
		got := renderTemplate(tc.body, map[string]any{"port": "9000"})
		if got != tc.want {
			t.Errorf("renderTemplate(%q) = %q; want %q", tc.body, got, tc.want)
		}
	}
}

// TestConfig_renderTemplate_Bad — empty body / empty vars short-circuit
// without panic; unknown placeholders are left in place to make the gap
// visible.
func TestConfig_renderTemplate_Bad(t *testing.T) {
	if got := renderTemplate("", map[string]any{"x": 1}); got != "" {
		t.Errorf("empty body should stay empty; got %q", got)
	}
	if got := renderTemplate("hello", nil); got != "hello" {
		t.Errorf("empty vars should pass through; got %q", got)
	}
	if got := renderTemplate("{{ .missing }}", map[string]any{"x": 1}); got != "{{ .missing }}" {
		t.Errorf("unknown placeholder should remain visible; got %q", got)
	}
}

// TestConfig_renderTemplate_Ugly — number / bool / nil values all
// stringify cleanly via core.Sprint.
func TestConfig_renderTemplate_Ugly(t *testing.T) {
	out := renderTemplate(
		"int={{ .a }} bool={{ .b }} nil={{ .c }}",
		map[string]any{"a": 42, "b": true, "c": nil},
	)
	want := "int=42 bool=true nil="
	if out != want {
		t.Errorf("renderTemplate ugly = %q; want %q", out, want)
	}
}

// TestConfig_destinationOf_Good — the .tmpl suffix is stripped to
// produce the destination path.
func TestConfig_destinationOf_Good(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"conf/thumbs.json.tmpl", "conf/thumbs.json"},
		{"conf/app.toml.tmpl", "conf/app.toml"},
		{"plain.txt.tmpl", "plain.txt"},
	}
	for _, tc := range cases {
		if got := destinationOf(tc.in); got != tc.want {
			t.Errorf("destinationOf(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConfig_destinationOf_Bad — a path missing the .tmpl suffix gets
// `.rendered` appended so the source isn't accidentally overwritten.
func TestConfig_destinationOf_Bad(t *testing.T) {
	if got := destinationOf("conf/thumbs.json"); got != "conf/thumbs.json.rendered" {
		t.Errorf("destinationOf without .tmpl = %q; want suffixed", got)
	}
}

// TestConfig_destinationOf_Ugly — empty path returns the .rendered
// suffix only (defensive, never leaks an invalid write target).
func TestConfig_destinationOf_Ugly(t *testing.T) {
	if got := destinationOf(""); got != ".rendered" {
		t.Errorf("destinationOf(empty) = %q; want %q", got, ".rendered")
	}
}

// TestConfig_isReservedConfigKey_Good — the framework-reserved names
// all return true.
func TestConfig_isReservedConfigKey_Good(t *testing.T) {
	for _, name := range []string{
		"services", "source", "type", "url", "display",
		"short_name", "theme", "locale", "icon", "shim",
		"gui_gates", "ipc_channels", "main", "entry",
	} {
		if !isReservedConfigKey(name) {
			t.Errorf("expected %q to be reserved", name)
		}
	}
}

// TestConfig_isReservedConfigKey_Bad — user names are not reserved.
func TestConfig_isReservedConfigKey_Bad(t *testing.T) {
	if isReservedConfigKey("thumbnails") {
		t.Error("user keys should not be reserved")
	}
}

// TestConfig_isReservedConfigKey_Ugly — empty key is not reserved.
func TestConfig_isReservedConfigKey_Ugly(t *testing.T) {
	if isReservedConfigKey("") {
		t.Error("empty key should not be reserved")
	}
}

// TestConfig_applyConfig_ReservedKeys — wrap-emitted Config keys
// (services, source, type, etc.) are skipped without error so PluginBoot
// can boot a wrapped manifest whose Config carries provenance metadata.
func TestConfig_applyConfig_ReservedKeys(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Config: map[string]any{
			"services": []any{"io", "store"},
			"source":   "wrap:pwa:https://app.example.com",
			"type":     "pwa",
		},
	}
	if err := applyConfig(c, m, coreio.Local, t.TempDir()); err != nil {
		t.Errorf("applyConfig should skip reserved keys; got %v", err)
	}
}
