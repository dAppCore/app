// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	coreio "dappco.re/go/io"
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

// TestApp_WithCore_Good — an app pre-built Core instance is reused by
// Boot rather than constructed fresh. Hosts (CoreGUI, core-agent) that
// already own a container use this to graft an app onto their service
// surface.
func TestApp_WithCore_Good(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `
code: with-core-good
name: With Core Good
version: 0.1.0
`)

	c := core.New()
	inst, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithCore(c),
		app.WithoutWorkspace(),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if inst.Core != c {
		t.Error("Boot did not reuse the supplied Core instance")
	}
}

// TestApp_WithCore_Bad — passing a nil Core falls through to the
// default constructor. The functional option is permissive so callers
// can pass the result of an optional getter without a guard.
func TestApp_WithCore_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithCore(nil))
	if opts.Core != nil {
		t.Errorf("WithCore(nil) set Core = %v; want nil so Boot constructs a fresh container", opts.Core)
	}
}

// TestApp_WithCore_Ugly — last-write-wins under functional options:
// the second WithCore overrides the first.
func TestApp_WithCore_Ugly(t *testing.T) {
	c1 := core.New()
	c2 := core.New()
	opts := app.NewOptions(app.WithCore(c1), app.WithCore(c2))
	if opts.Core != c2 {
		t.Errorf("WithCore last-write-wins broken: got %v want %v", opts.Core, c2)
	}
}

// TestApp_WithoutWorkspace_Good — Boot with WithoutWorkspace skips the
// per-app data tree bootstrap. Inst.Workspace stays nil so handlers
// know they have no guaranteed scratch directory.
func TestApp_WithoutWorkspace_Good(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `
code: without-workspace-good
name: Without Workspace Good
version: 0.1.0
`)

	inst, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithoutWorkspace(),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if inst.Workspace != nil {
		t.Errorf("Workspace = %v; want nil when WithoutWorkspace was passed", inst.Workspace)
	}
}

// TestApp_WithoutWorkspace_Bad — without the option, a dev-mode boot
// should still try to bootstrap a workspace under the supplied home.
// The provisioned workspace makes this the contrast case for the Good
// test above.
func TestApp_WithoutWorkspace_Bad(t *testing.T) {
	_ = "WithoutWorkspace"
	dir := t.TempDir()
	writeYAML(t, coreio.Local, dir+"/.core/view.yaml", `
code: without-workspace-bad
name: Without Workspace Bad
version: 0.1.0
`)
	home := t.TempDir()

	inst, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithWorkspaceHome(home),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if inst.Workspace == nil {
		t.Fatal("Workspace = nil; expected a provisioned workspace when the option is omitted")
	}
	if inst.Workspace.Code != "without-workspace-bad" {
		t.Errorf("Workspace.Code = %q; want %q", inst.Workspace.Code, "without-workspace-bad")
	}
}

// TestApp_WithoutWorkspace_Ugly — applying WithoutWorkspace twice is a
// no-op (the underlying flag is a bool toggle, not an accumulator).
func TestApp_WithoutWorkspace_Ugly(t *testing.T) {
	opts := app.NewOptions(app.WithoutWorkspace(), app.WithoutWorkspace())
	if !opts.SkipWorkspace {
		t.Error("two WithoutWorkspace calls did not leave SkipWorkspace=true")
	}
}

func TestApp_WithMode_Good(t *testing.T) {
	opts := app.NewOptions(app.WithMode(app.ModeDev))
	if opts.Mode != app.ModeDev {
		t.Fatalf("WithMode(ModeDev) = %v; want ModeDev", opts.Mode)
	}
}

func TestApp_WithMode_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithMode(app.Mode(99)))
	if opts.Mode.String() != "prod" {
		t.Fatalf("unknown mode String() = %q; want prod", opts.Mode.String())
	}
}

func TestApp_WithMode_Ugly(t *testing.T) {
	opts := app.NewOptions(app.WithMode(app.ModeDev), app.WithMode(app.ModeProd))
	if opts.Mode != app.ModeProd {
		t.Fatalf("last WithMode should win; got %v", opts.Mode)
	}
}

func TestApp_WithMedium_Good(t *testing.T) {
	medium := coreio.NewMemoryMedium()
	opts := app.NewOptions(app.WithMedium(medium))
	if opts.Medium != medium {
		t.Fatal("WithMedium did not preserve the supplied medium")
	}
}

func TestApp_WithMedium_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithMedium(nil))
	if opts.Medium == nil {
		t.Fatal("nil WithMedium should fall back to a usable medium")
	}
}

func TestApp_WithMedium_Ugly(t *testing.T) {
	first := coreio.NewMemoryMedium()
	second := coreio.NewMemoryMedium()
	opts := app.NewOptions(app.WithMedium(first), app.WithMedium(second))
	if opts.Medium != second {
		t.Fatal("last WithMedium should win")
	}
}

func TestApp_WithPublicKey_Good(t *testing.T) {
	opts := app.NewOptions(app.WithPublicKey("abc123"))
	if opts.PublicKeyHex != "abc123" {
		t.Fatalf("PublicKeyHex = %q; want abc123", opts.PublicKeyHex)
	}
}

func TestApp_WithPublicKey_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithPublicKey(""))
	if opts.PublicKeyHex != "" {
		t.Fatalf("empty WithPublicKey should be a no-op, got %q", opts.PublicKeyHex)
	}
}

func TestApp_WithPublicKey_Ugly(t *testing.T) {
	opts := app.NewOptions(app.WithPublicKey("first"), app.WithPublicKey(""), app.WithPublicKey("second"))
	if opts.PublicKeyHex != "second" {
		t.Fatalf("PublicKeyHex = %q; want second", opts.PublicKeyHex)
	}
}

func TestApp_WithTrustedKeysDir_Good(t *testing.T) {
	dir := t.TempDir()
	opts := app.NewOptions(app.WithTrustedKeysDir(dir))
	if opts.TrustedKeysDir != dir {
		t.Fatalf("TrustedKeysDir = %q; want %q", opts.TrustedKeysDir, dir)
	}
}

func TestApp_WithTrustedKeysDir_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithTrustedKeysDir(""))
	if opts.TrustedKeysDir != "" && !core.HasSuffix(opts.TrustedKeysDir, core.Path(".core", "keys")) {
		t.Fatalf("empty WithTrustedKeysDir fallback = %q; want empty or conventional suffix", opts.TrustedKeysDir)
	}
}

func TestApp_WithTrustedKeysDir_Ugly(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	opts := app.NewOptions(app.WithTrustedKeysDir(first), app.WithTrustedKeysDir(second))
	if opts.TrustedKeysDir != second {
		t.Fatalf("TrustedKeysDir = %q; want %q", opts.TrustedKeysDir, second)
	}
}

func TestApp_WithoutKeyLoad_Good(t *testing.T) {
	opts := app.NewOptions(app.WithoutKeyLoad())
	if !opts.DisableKeyLoad {
		t.Fatal("WithoutKeyLoad did not set DisableKeyLoad")
	}
}

func TestApp_WithoutKeyLoad_Bad(t *testing.T) {
	_ = "WithoutKeyLoad"
	opts := app.NewOptions()
	if opts.DisableKeyLoad {
		t.Fatal("DisableKeyLoad should default false")
	}
}

func TestApp_WithoutKeyLoad_Ugly(t *testing.T) {
	opts := app.NewOptions(app.WithoutKeyLoad(), app.WithoutKeyLoad())
	if !opts.DisableKeyLoad {
		t.Fatal("WithoutKeyLoad should be idempotent")
	}
}

func TestApp_WithWorkspaceHome_Good(t *testing.T) {
	home := t.TempDir()
	opts := app.NewOptions(app.WithWorkspaceHome(home))
	if opts.WorkspaceHome != home {
		t.Fatalf("WorkspaceHome = %q; want %q", opts.WorkspaceHome, home)
	}
}

func TestApp_WithWorkspaceHome_Bad(t *testing.T) {
	opts := app.NewOptions(app.WithWorkspaceHome(""))
	if opts.WorkspaceHome != "" {
		t.Fatalf("empty WithWorkspaceHome should be a no-op, got %q", opts.WorkspaceHome)
	}
}

func TestApp_WithWorkspaceHome_Ugly(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	opts := app.NewOptions(app.WithWorkspaceHome(first), app.WithWorkspaceHome(second))
	if opts.WorkspaceHome != second {
		t.Fatalf("WorkspaceHome = %q; want %q", opts.WorkspaceHome, second)
	}
}

func TestApp_Instance_Stop_Good(t *testing.T) {
	inst := &app.Instance{Core: core.New()}
	r := inst.Stop(context.Background())
	if !r.OK {
		t.Fatalf("Stop returned !OK: %v", r.Value)
	}
}

func TestApp_Instance_Stop_Bad(t *testing.T) {
	var inst *app.Instance
	r := inst.Stop(context.Background())
	if r.OK {
		t.Fatal("Stop on nil instance should fail")
	}
}

func TestApp_Instance_Stop_Ugly(t *testing.T) {
	inst := &app.Instance{}
	r := inst.Stop(context.Background())
	if r.OK {
		t.Fatal("Stop with nil Core should fail")
	}
}
