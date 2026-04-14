// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestConfig_applyConfig_Good — a manifest declaring a template whose
// file exists on the medium passes the validation gate. The render
// step itself is stubbed (see applyConfig TODO).
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
