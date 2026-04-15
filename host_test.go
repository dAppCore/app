// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// newHostTestFixture writes a stub installed plugin under
// <home>/.core/apps/<code>/.core/view.yaml so Launch can discover it.
// Returns (home, cleanup) — the cleanup deletes the temp tree.
//
//	home, cleanup := newHostTestFixture(t, "photo-browser")
//	defer cleanup()
func newHostTestFixture(t *testing.T, code string) string {
	t.Helper()
	home := t.TempDir()
	manifest := config.ViewManifest{
		Code:    code,
		Name:    code,
		Version: "0.1.0",
		Layout:  "C",
	}
	body, err := yamlMarshal(&manifest)
	if err != nil {
		t.Fatalf("yamlMarshal: %v", err)
	}
	dest := core.Path(home, ".core", AppsDirName, code, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(dest)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(dest, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return home
}

// TestHost_Launch_Good — a fresh host launches an installed plugin
// by code and registers it under Running().
func TestHost_Launch_Good(t *testing.T) {
	home := newHostTestFixture(t, "photo-browser")
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	inst, err := h.Launch(context.Background(), "photo-browser", LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if inst == nil || inst.Core == nil {
		t.Fatal("Launch returned nil instance / core")
	}
	got := h.Running()
	if len(got) != 1 || got[0] != "photo-browser" {
		t.Errorf("Running() = %v; want [photo-browser]", got)
	}
	if _, ok := h.Get("photo-browser"); !ok {
		t.Error("Get(photo-browser) = !ok after Launch")
	}
}

// TestHost_Launch_Bad — re-launching the same code without a prior
// Stop returns a typed error so a confused UI can't clobber the
// existing Instance.
func TestHost_Launch_Bad(t *testing.T) {
	home := newHostTestFixture(t, "markdown-editor")
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	if _, err := h.Launch(context.Background(), "markdown-editor", LaunchOptions{}); err != nil {
		t.Fatalf("first Launch: %v", err)
	}
	if _, err := h.Launch(context.Background(), "markdown-editor", LaunchOptions{}); err == nil {
		t.Fatal("second Launch should have errored — plugin already running")
	}
}

// TestHost_Launch_Ugly — empty code is rejected up front so the host
// never registers an entry under the empty string.
func TestHost_Launch_Ugly(t *testing.T) {
	h := NewHost(HostOptions{Home: t.TempDir(), Mode: ModeDev})
	if _, err := h.Launch(context.Background(), "", LaunchOptions{}); err == nil {
		t.Error("empty code should produce a typed error")
	}
}

// TestHost_Stop_Good — Stop tears down the Instance and removes it
// from Running() so the code can be re-Launched afterwards.
func TestHost_Stop_Good(t *testing.T) {
	home := newHostTestFixture(t, "calculator")
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	inst, err := h.Launch(context.Background(), "calculator", LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	// Drive Start so stop has a lifecycle to wind down.
	if r := inst.Start(context.Background()); !r.OK {
		t.Fatalf("Start: %v", r.Value)
	}
	r := h.Stop(context.Background(), "calculator")
	if !r.OK {
		t.Fatalf("Stop: %v", r.Value)
	}
	if got := h.Running(); len(got) != 0 {
		t.Errorf("Running after Stop = %v; want []", got)
	}
	// Should be re-Launchable now.
	if _, err := h.Launch(context.Background(), "calculator", LaunchOptions{}); err != nil {
		t.Fatalf("relaunch after Stop failed: %v", err)
	}
}

// TestHost_Stop_Bad — Stop on an unknown code returns a typed error
// (not a panic).
func TestHost_Stop_Bad(t *testing.T) {
	h := NewHost(HostOptions{Home: t.TempDir(), Mode: ModeDev})
	r := h.Stop(context.Background(), "ghost-plugin")
	if r.OK {
		t.Error("Stop on unknown code should fail")
	}
}

// TestHost_Stop_Ugly — empty code is rejected.
func TestHost_Stop_Ugly(t *testing.T) {
	h := NewHost(HostOptions{Home: t.TempDir(), Mode: ModeDev})
	r := h.Stop(context.Background(), "")
	if r.OK {
		t.Error("Stop with empty code should fail")
	}
}

// TestHost_Shutdown_Good — Shutdown stops every running plugin and
// leaves Running() empty.
func TestHost_Shutdown_Good(t *testing.T) {
	home := t.TempDir()
	for _, code := range []string{"a-plugin", "b-plugin", "c-plugin"} {
		newHostTestPluginAt(t, home, code)
	}
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	for _, code := range []string{"a-plugin", "b-plugin", "c-plugin"} {
		inst, err := h.Launch(context.Background(), code, LaunchOptions{})
		if err != nil {
			t.Fatalf("Launch %s: %v", code, err)
		}
		if r := inst.Start(context.Background()); !r.OK {
			t.Fatalf("Start %s: %v", code, r.Value)
		}
	}
	r := h.Shutdown(context.Background())
	if !r.OK {
		t.Fatalf("Shutdown: %v", r.Value)
	}
	if got := h.Running(); len(got) != 0 {
		t.Errorf("Running after Shutdown = %v; want []", got)
	}
}

// TestHost_Shutdown_Bad — Shutdown on an empty host is a no-op OK.
func TestHost_Shutdown_Bad(t *testing.T) {
	h := NewHost(HostOptions{Home: t.TempDir()})
	if r := h.Shutdown(context.Background()); !r.OK {
		t.Errorf("empty-host Shutdown: %v", r.Value)
	}
	// Second call must stay OK.
	if r := h.Shutdown(context.Background()); !r.OK {
		t.Errorf("second Shutdown: %v", r.Value)
	}
}

// TestHost_Shutdown_Ugly — a nil receiver produces a typed Result
// instead of a panic.
func TestHost_Shutdown_Ugly(t *testing.T) {
	var h *Host
	if r := h.Shutdown(context.Background()); r.OK {
		t.Error("nil-host Shutdown should fail")
	}
}

// TestHost_Dispatch_Good — routing an action that the target plugin
// registered returns the handler's result to the caller.
func TestHost_Dispatch_Good(t *testing.T) {
	home := newHostTestFixture(t, "editor")
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	inst, err := h.Launch(context.Background(), "editor", LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	// Register a cross-plugin action — the source plugin will call it.
	inst.Core.Action("editor.save", func(_ context.Context, _ core.Options) core.Result {
		return core.Result{Value: "saved", OK: true}
	})

	r := h.Dispatch(context.Background(), "callerapp", "editor", "editor.save", core.NewOptions())
	if !r.OK {
		t.Fatalf("Dispatch: %v", r.Value)
	}
	if got, _ := r.Value.(string); got != "saved" {
		t.Errorf("Dispatch result = %q; want 'saved'", got)
	}
}

// TestHost_Dispatch_Bad — dispatching to a target that did not
// register the action returns a typed error so the caller can surface
// a "feature not available" message.
func TestHost_Dispatch_Bad(t *testing.T) {
	home := newHostTestFixture(t, "target")
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	if _, err := h.Launch(context.Background(), "target", LaunchOptions{}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	r := h.Dispatch(context.Background(), "src", "target", "missing.action", core.NewOptions())
	if r.OK {
		t.Error("Dispatch to missing handler should fail")
	}
}

// TestHost_Dispatch_Ugly — an empty sourceCode / targetCode / action
// is rejected before any lookup so a buggy caller cannot silently
// dispatch into the void.
func TestHost_Dispatch_Ugly(t *testing.T) {
	h := NewHost(HostOptions{Home: t.TempDir()})
	cases := []struct {
		name                            string
		source, target, action, wantErr string
	}{
		{"empty source", "", "t", "a", "sourceCode"},
		{"empty target", "s", "", "a", "targetCode"},
		{"empty action", "s", "t", "", "action"},
	}
	for _, c := range cases {
		r := h.Dispatch(context.Background(), c.source, c.target, c.action, core.NewOptions())
		if r.OK {
			t.Errorf("%s: Dispatch should fail", c.name)
		}
	}
}

// TestHost_Each_Good — Each walks plugins in lexicographic order so
// tests can assert a stable render sequence.
func TestHost_Each_Good(t *testing.T) {
	home := t.TempDir()
	for _, code := range []string{"zulu", "alpha", "mike"} {
		newHostTestPluginAt(t, home, code)
	}
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	for _, code := range []string{"zulu", "alpha", "mike"} {
		if _, err := h.Launch(context.Background(), code, LaunchOptions{}); err != nil {
			t.Fatalf("Launch %s: %v", code, err)
		}
	}

	seen := []string{}
	h.Each(func(code string, _ *Instance) bool {
		seen = append(seen, code)
		return true
	})
	want := []string{"alpha", "mike", "zulu"}
	if len(seen) != len(want) {
		t.Fatalf("Each iteration = %v; want %v", seen, want)
	}
	for i, w := range want {
		if seen[i] != w {
			t.Fatalf("Each iteration[%d] = %s; want %s", i, seen[i], w)
		}
	}
}

// TestHost_Each_Bad — a false return from fn stops the iteration
// early.
func TestHost_Each_Bad(t *testing.T) {
	home := t.TempDir()
	for _, code := range []string{"a", "b", "c"} {
		newHostTestPluginAt(t, home, code)
	}
	h := NewHost(HostOptions{Home: home, Mode: ModeDev})
	for _, code := range []string{"a", "b", "c"} {
		if _, err := h.Launch(context.Background(), code, LaunchOptions{}); err != nil {
			t.Fatalf("Launch: %v", err)
		}
	}
	count := 0
	h.Each(func(_ string, _ *Instance) bool {
		count++
		return false // stop immediately
	})
	if count != 1 {
		t.Errorf("Each stopped after %d; want 1", count)
	}
}

// TestHost_Each_Ugly — nil fn or nil receiver is a no-op, not a panic.
func TestHost_Each_Ugly(t *testing.T) {
	var h *Host
	h.Each(nil) // must not panic
	h2 := NewHost(HostOptions{})
	h2.Each(nil) // also no-op on a live host
}

// newHostTestPluginAt is a tiny test helper that writes a stub
// installed plugin under the supplied home directory. Kept separate
// from newHostTestFixture so callers building multi-plugin trees can
// reuse a single home across several codes.
//
//	home := t.TempDir()
//	newHostTestPluginAt(t, home, "alpha")
//	newHostTestPluginAt(t, home, "beta")
func newHostTestPluginAt(t *testing.T, home, code string) {
	t.Helper()
	manifest := config.ViewManifest{
		Code:    code,
		Name:    code,
		Version: "0.1.0",
		Layout:  "C",
	}
	body, err := yamlMarshal(&manifest)
	if err != nil {
		t.Fatalf("yamlMarshal: %v", err)
	}
	dest := core.Path(home, ".core", AppsDirName, code, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(dest)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(dest, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
}
