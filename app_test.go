// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
)

// TestApp_Boot_Good — a manifest exists under start/.core/view.yaml and
// describes a minimal CLI-only CoreApp (no layout, no modules, no
// templates). Boot should walk every step cleanly and return an
// Instance whose identity matches the manifest.
func TestApp_Boot_Good(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `
code: test-app
name: Test App
version: 0.1.0
`)

	inst, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
	)
	if err != nil {
		t.Fatalf("Boot returned error: %v", err)
	}
	if inst == nil {
		t.Fatal("Boot returned nil Instance")
	}
	if inst.Manifest.Code != "test-app" {
		t.Errorf("Manifest.Code = %q; want %q", inst.Manifest.Code, "test-app")
	}
	if inst.Manifest.Version != "0.1.0" {
		t.Errorf("Manifest.Version = %q; want %q", inst.Manifest.Version, "0.1.0")
	}
	if inst.Mode != app.ModeDev {
		t.Errorf("Mode = %v; want %v", inst.Mode, app.ModeDev)
	}
	if inst.Core == nil {
		t.Error("Core should be non-nil after Boot")
	}
}

// TestApp_Boot_Bad — no .core/view.yaml anywhere on the path. Boot
// surfaces an error that names the starting directory so the caller
// can see exactly what failed.
func TestApp_Boot_Bad(t *testing.T) {
	dir := t.TempDir()

	_, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
	)
	if err == nil {
		t.Fatal("Boot should fail without .core/view.yaml")
	}
}

// TestApp_Boot_Ugly — manifest exists but is malformed YAML. Boot
// surfaces the parse failure rather than returning a partial Instance.
func TestApp_Boot_Ugly(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `::: this is not yaml :::`)

	_, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
	)
	if err == nil {
		t.Fatal("Boot should fail on malformed manifest")
	}
}

// TestApp_NewOptions_Good — defaults land on ModeProd and coreio.Local
// when the caller passes no options.
func TestApp_NewOptions_Good(t *testing.T) {
	opts := app.NewOptions()
	if opts.Mode != app.ModeProd {
		t.Errorf("default Mode = %v; want %v", opts.Mode, app.ModeProd)
	}
	if opts.Medium == nil {
		t.Error("default Medium should not be nil")
	}
}

// TestApp_NewOptions_Bad — nil medium option doesn't blow up; Options
// falls back to coreio.Local.
func TestApp_NewOptions_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithMedium(nil))
	if opts.Medium == nil {
		t.Error("nil medium should fall back to coreio.Local")
	}
}

// TestApp_NewOptions_Ugly — a later option overrides the first. This
// is the expected functional-options behaviour; the test pins it.
func TestApp_NewOptions_Ugly(t *testing.T) {
	opts := app.NewOptions(
		app.WithMode(app.ModeDev),
		app.WithMode(app.ModeProd),
	)
	if opts.Mode != app.ModeProd {
		t.Errorf("last-write-wins: Mode = %v; want %v", opts.Mode, app.ModeProd)
	}
}

// TestApp_Mode_String_Good — covers both enum values.
func TestApp_Mode_String_Good(t *testing.T) {
	if got := app.ModeProd.String(); got != "prod" {
		t.Errorf("ModeProd.String() = %q; want %q", got, "prod")
	}
	if got := app.ModeDev.String(); got != "dev" {
		t.Errorf("ModeDev.String() = %q; want %q", got, "dev")
	}
}

// TestApp_Mode_String_Bad — an unrecognised Mode value defaults to
// "prod" (we never want the denial-by-default gate to flip silently).
func TestApp_Mode_String_Bad(t *testing.T) {
	m := app.Mode(99)
	if got := m.String(); got != "prod" {
		t.Errorf("unknown Mode = %q; want %q", got, "prod")
	}
}

// TestApp_Mode_String_Ugly — negative values also fall through to
// "prod".
func TestApp_Mode_String_Ugly(t *testing.T) {
	m := app.Mode(-1)
	if got := m.String(); got != "prod" {
		t.Errorf("negative Mode = %q; want %q", got, "prod")
	}
}

// TestApp_Instance_Start_Good — an Instance with a live Core returns
// OK from Start (the step-7 stub just broadcasts ActionAppStarted).
func TestApp_Instance_Start_Good(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `
code: start-good
name: Start Good
version: 0.1.0
`)
	inst, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
	)
	if err != nil {
		t.Fatalf("Boot returned error: %v", err)
	}
	if r := inst.Start(context.Background()); !r.OK {
		t.Errorf("Start.OK = false; want true. Value=%v", r.Value)
	}
}

// TestApp_Instance_Start_Bad — a nil Instance reports the failure via
// Result instead of panicking.
func TestApp_Instance_Start_Bad(t *testing.T) {
	var inst *app.Instance
	r := inst.Start(context.Background())
	if r.OK {
		t.Error("Start on nil Instance should fail")
	}
}

// TestApp_Instance_Start_Ugly — an Instance with a nil Core also fails
// gracefully (no nil-pointer deref in the broadcast path).
func TestApp_Instance_Start_Ugly(t *testing.T) {
	inst := &app.Instance{} // Core intentionally nil
	r := inst.Start(context.Background())
	if r.OK {
		t.Error("Start with nil Core should fail")
	}
}

// writeYAML is a tiny helper for the test manifests. Keeps the
// per-test wiring a single line.
func writeYAML(t *testing.T, m coreio.Medium, path, body string) {
	t.Helper()
	if err := m.EnsureDir(core.PathDir(path)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := m.Write(path, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
}
