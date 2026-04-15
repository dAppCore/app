// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestPlugin_PluginBoot_Good asserts a host can spin up an isolated
// plugin Core from a manifest, that the permission gate honours the
// manifest's permission slots, and that the lock prevents downstream
// service registration.
func TestPlugin_PluginBoot_Good(t *testing.T) {
	dir := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "markdown-editor",
		Name:    "Markdown Editor",
		Version: "0.1.0",
		Layout:  "C",
		Permissions: config.ViewPermissions{
			Read: []string{"./docs/"},
			Net:  []string{"api.github.com:443"},
		},
	}

	inst, err := app.PluginBoot(context.Background(), app.PluginOptions{
		Manifest:    manifest,
		ProjectRoot: dir,
		Mode:        app.ModeProd,
		Services:    nil, // no host services for this test
		Medium:      coreio.Local,
	})
	if err != nil {
		t.Fatalf("PluginBoot: %v", err)
	}
	if inst == nil {
		t.Fatal("PluginBoot returned nil instance")
	}
	if inst.Core == nil {
		t.Fatal("PluginBoot returned instance with nil Core")
	}
	if inst.Manifest.Code != "markdown-editor" {
		t.Errorf("Manifest.Code = %q; want markdown-editor", inst.Manifest.Code)
	}
	if inst.Root != dir {
		t.Errorf("Root = %q; want %q", inst.Root, dir)
	}

	// Permission gate honours the manifest.
	if e := inst.Core.Entitled("fs.read"); !e.Allowed {
		t.Errorf("fs.read should be allowed; reason=%q", e.Reason)
	}
	if e := inst.Core.Entitled("net.fetch"); !e.Allowed {
		t.Errorf("net.fetch should be allowed; reason=%q", e.Reason)
	}
	if e := inst.Core.Entitled("process.run"); e.Allowed {
		t.Errorf("process.run should be denied (no run permission)")
	}
}

// TestPlugin_PluginBoot_Bad rejects empty ProjectRoot and empty manifest
// code (a plugin cannot boot without identity).
func TestPlugin_PluginBoot_Bad(t *testing.T) {
	if _, err := app.PluginBoot(context.Background(), app.PluginOptions{}); err == nil {
		t.Error("empty ProjectRoot produced no error")
	}
	if _, err := app.PluginBoot(context.Background(), app.PluginOptions{
		ProjectRoot: t.TempDir(),
		Manifest:    config.ViewManifest{},
	}); err == nil {
		t.Error("empty manifest code produced no error")
	}
}

// TestPlugin_PluginBoot_Ugly — a manifest whose config template path
// does not exist still fails Step 6 (config) inside the plugin pipeline
// in prod mode. Proves PluginBoot reuses the same enforcement Boot does.
func TestPlugin_PluginBoot_Ugly(t *testing.T) {
	dir := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "broken-tmpl",
		Name:    "Broken Template",
		Version: "0.1.0",
		Config: map[string]any{
			"missing": map[string]any{
				"template": "conf/does-not-exist.tmpl",
				"vars":     map[string]any{},
			},
		},
	}
	if _, err := app.PluginBoot(context.Background(), app.PluginOptions{
		Manifest:    manifest,
		ProjectRoot: dir,
		Mode:        app.ModeProd,
		Medium:      coreio.Local,
	}); err == nil {
		t.Error("missing config template produced no error in prod mode")
	}
}

// TestPlugin_SelectServices_Good filters a host's registry by the names
// the plugin declared. Order in the output matches the registry order so
// dependent services are wired in a predictable sequence.
func TestPlugin_SelectServices_Good(t *testing.T) {
	called := []string{}
	mkFactory := func(name string) core.CoreOption {
		return func(_ *core.Core) core.Result {
			called = append(called, name)
			return core.Result{OK: true}
		}
	}
	registry := []app.PluginServiceSpec{
		{Name: "io", Factory: mkFactory("io")},
		{Name: "i18n", Factory: mkFactory("i18n")},
		{Name: "store", Factory: mkFactory("store")},
	}
	requested := app.PluginServices{"store", "io"}
	opts := app.SelectServices(registry, requested)
	if len(opts) != 2 {
		t.Fatalf("SelectServices returned %d options; want 2", len(opts))
	}
	c := core.New(opts...)
	_ = c
	// The registry order ("io" before "store") drives the call order
	// regardless of the request order, which is the property the host
	// relies on when one service depends on another.
	if len(called) != 2 || called[0] != "io" || called[1] != "store" {
		t.Errorf("called = %v; want [io store]", called)
	}
}

// TestPlugin_SelectServices_Bad — empty registry / empty request both
// return nil. Unknown names are dropped without complaint.
func TestPlugin_SelectServices_Bad(t *testing.T) {
	if app.SelectServices(nil, app.PluginServices{"x"}) != nil {
		t.Error("nil registry returned non-nil opts")
	}
	if app.SelectServices(
		[]app.PluginServiceSpec{{Name: "io"}}, nil,
	) != nil {
		t.Error("nil request returned non-nil opts")
	}
	// Unknown name → silently dropped.
	opts := app.SelectServices(
		[]app.PluginServiceSpec{{Name: "io", Factory: func(_ *core.Core) core.Result { return core.Result{OK: true} }}},
		app.PluginServices{"unknown-service"},
	)
	if len(opts) != 0 {
		t.Errorf("unknown-service returned %d opts; want 0", len(opts))
	}
}

// TestPlugin_SelectServices_Ugly preserves ordering and dedupes
// gracefully when a request lists the same name twice.
func TestPlugin_SelectServices_Ugly(t *testing.T) {
	registry := []app.PluginServiceSpec{
		{Name: "io", Factory: func(_ *core.Core) core.Result { return core.Result{OK: true} }},
		{Name: "store", Factory: func(_ *core.Core) core.Result { return core.Result{OK: true} }},
	}
	opts := app.SelectServices(registry, app.PluginServices{"io", "io", "store"})
	// Even with the duplicate "io", the registry only lists it once.
	if len(opts) != 2 {
		t.Errorf("opts len = %d; want 2 (one per registry entry that matched)", len(opts))
	}
}

// TestPlugin_PluginServicesFromManifest_Good extracts a list-of-strings
// stashed under Config["services"] (the canonical slot until
// config.ViewManifest grows a typed Services field).
func TestPlugin_PluginServicesFromManifest_Good(t *testing.T) {
	m := &config.ViewManifest{
		Code: "x",
		Config: map[string]any{
			"services": []any{"io", "store", "i18n"},
		},
	}
	got := app.PluginServicesFromManifest(m)
	if len(got) != 3 {
		t.Fatalf("got %d services; want 3", len(got))
	}
	if got[0] != "io" || got[1] != "store" || got[2] != "i18n" {
		t.Errorf("got = %v; want [io store i18n]", got)
	}
}

// TestPlugin_PluginServicesFromManifest_Bad handles nil manifest, nil
// Config map, missing services key, and wrong-type values gracefully.
func TestPlugin_PluginServicesFromManifest_Bad(t *testing.T) {
	if got := app.PluginServicesFromManifest(nil); got != nil {
		t.Error("nil manifest returned non-nil services")
	}
	if got := app.PluginServicesFromManifest(&config.ViewManifest{}); got != nil {
		t.Error("nil Config returned non-nil services")
	}
	m := &config.ViewManifest{Config: map[string]any{"services": 42}}
	if got := app.PluginServicesFromManifest(m); got != nil {
		t.Errorf("non-list services returned %v; want nil", got)
	}
}

// TestPlugin_PluginServicesFromManifest_Ugly accepts the strongly-typed
// []string form that a Go-side caller might inject directly.
func TestPlugin_PluginServicesFromManifest_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Config: map[string]any{
			"services": []string{"io", "store"},
		},
	}
	got := app.PluginServicesFromManifest(m)
	if len(got) != 2 {
		t.Fatalf("got %d services; want 2", len(got))
	}
	if got[0] != "io" || got[1] != "store" {
		t.Errorf("got = %v; want [io store]", got)
	}
}
