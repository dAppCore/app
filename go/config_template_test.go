// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// TestConfigTemplate_renderManifestConfigTemplates_Good — a manifest
// template whose vars point at hydrated store values renders to disk
// with those values substituted.
func TestConfigTemplate_renderManifestConfigTemplates_Good(t *testing.T) {
	_ = "renderManifestConfigTemplates"
	root := t.TempDir()
	home := t.TempDir()

	must(t, coreio.Local.EnsureDir(core.Path(root, "conf")))
	must(t, coreio.Local.Write(
		core.Path(root, "conf", "thumbs.json.tmpl"),
		`{"size":{{ .size }},"quality":{{ .quality }}}`,
	))

	manifest := &config.ViewManifest{
		Code: "config-template-good",
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars": map[string]any{
					"size":    "{{ .user.thumbnail_size }}",
					"quality": "{{ .user.quality }}",
				},
			},
		},
	}

	ws := seedConfigTemplateWorkspace(t, home, manifest.Code, map[string]string{
		"user.thumbnail_size": "256",
		"user.quality":        "85",
	})

	if err := renderManifestConfigTemplatesWithMode(core.New(), manifest, coreio.Local, root, ws, ModeProd); err != nil {
		t.Fatalf("renderManifestConfigTemplatesWithMode: %v", err)
	}

	rendered, err := coreio.Local.Read(core.Path(root, "conf", "thumbs.json"))
	if err != nil {
		t.Fatalf("read rendered config: %v", err)
	}
	if rendered != `{"size":256,"quality":85}` {
		t.Errorf("rendered = %q; want %q", rendered, `{"size":256,"quality":85}`)
	}
}

// TestConfigTemplate_renderManifestConfigTemplates_Bad — a missing
// store-backed var fails with an error that names the missing path.
func TestConfigTemplate_renderManifestConfigTemplates_Bad(t *testing.T) {
	_ = "renderManifestConfigTemplates"
	root := t.TempDir()
	home := t.TempDir()

	must(t, coreio.Local.EnsureDir(core.Path(root, "conf")))
	must(t, coreio.Local.Write(
		core.Path(root, "conf", "thumbs.json.tmpl"),
		`{"size":{{ .size }}}`,
	))

	manifest := &config.ViewManifest{
		Code: "config-template-bad",
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars": map[string]any{
					"size": "{{ .user.thumbnail_size }}",
				},
			},
		},
	}

	ws := seedConfigTemplateWorkspace(t, home, manifest.Code, nil)

	err := renderManifestConfigTemplatesWithMode(core.New(), manifest, coreio.Local, root, ws, ModeProd)
	if err == nil {
		t.Fatal("renderManifestConfigTemplatesWithMode should fail on a missing store path")
	}
	if !core.Contains(err.Error(), "user.thumbnail_size") {
		t.Fatalf("missing-path error = %q; want path user.thumbnail_size", err)
	}
}

// TestConfigTemplate_renderManifestConfigTemplates_Ugly — malformed
// template syntax fails cleanly instead of panicking.
func TestConfigTemplate_renderManifestConfigTemplates_Ugly(t *testing.T) {
	_ = "renderManifestConfigTemplates"
	root := t.TempDir()
	home := t.TempDir()

	must(t, coreio.Local.EnsureDir(core.Path(root, "conf")))
	must(t, coreio.Local.Write(
		core.Path(root, "conf", "thumbs.json.tmpl"),
		`{"size":{{ .size }`,
	))

	manifest := &config.ViewManifest{
		Code: "config-template-ugly",
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars": map[string]any{
					"size": "{{ .user.thumbnail_size }}",
				},
			},
		},
	}

	ws := seedConfigTemplateWorkspace(t, home, manifest.Code, map[string]string{
		"user.thumbnail_size": "256",
	})

	err := renderManifestConfigTemplatesWithMode(core.New(), manifest, coreio.Local, root, ws, ModeProd)
	if err == nil {
		t.Fatal("renderManifestConfigTemplatesWithMode should fail on malformed template syntax")
	}
	if !core.Contains(err.Error(), "malformed template") {
		t.Fatalf("malformed-template error = %q; want syntax failure", err)
	}
}

func seedConfigTemplateWorkspace(t *testing.T, home, code string, entries map[string]string) *Workspace {
	t.Helper()

	ws, err := OpenWorkspace(coreio.Local, home, code)
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}

	store := newWorkspaceObjectStore(ws)
	defer store.Close()

	for path, value := range entries {
		parts := core.SplitN(path, ".", 2)
		if len(parts) != 2 {
			t.Fatalf("seed path %q is not group.key", path)
		}
		if err := store.Set(parts[0], parts[1], value); err != nil {
			t.Fatalf("store.Set(%q): %v", path, err)
		}
	}

	return ws
}
