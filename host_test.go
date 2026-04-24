// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
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

// newSignedHostTestFixture writes a signed installed plugin and returns
// the host home, the install root, the public key hex and the private
// key used to sign the manifest.
func newSignedHostTestFixture(t *testing.T, code, name string) (string, string, string, ed25519.PrivateKey) {
	t.Helper()
	home := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	manifest := config.ViewManifest{
		Code:    code,
		Name:    name,
		Version: "0.1.0",
		Layout:  "C",
	}
	if err := SignManifest(&manifest, priv); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	body, err := yamlMarshal(&manifest)
	if err != nil {
		t.Fatalf("yamlMarshal: %v", err)
	}
	root := core.Path(home, ".core", AppsDirName, code)
	dest := core.Path(root, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(dest)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(dest, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return home, root, hex.EncodeToString(pub), priv
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

// TestHost_Launch_ProdVerify_Good — prod hosts verify the plugin
// manifest against the configured trust key before PluginBoot.
func TestHost_Launch_ProdVerify_Good(t *testing.T) {
	home, _, pubHex, _ := newSignedHostTestFixture(t, "signed-host", "Signed Host")
	h := NewHost(HostOptions{
		Home:           home,
		Mode:           ModeProd,
		PublicKeyHex:   pubHex,
		DisableKeyLoad: true,
	})
	inst, err := h.Launch(context.Background(), "signed-host", LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch(prod verify): %v", err)
	}
	if inst == nil || inst.Manifest.Code != "signed-host" {
		t.Fatalf("Launch returned wrong instance: %#v", inst)
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

// TestHost_Launch_ProdVerify_Bad — a prod host refuses to launch a
// signed plugin when no trust root is configured.
func TestHost_Launch_ProdVerify_Bad(t *testing.T) {
	home, _, _, _ := newSignedHostTestFixture(t, "untrusted-host", "Untrusted Host")
	h := NewHost(HostOptions{
		Home:           home,
		Mode:           ModeProd,
		DisableKeyLoad: true,
	})
	if _, err := h.Launch(context.Background(), "untrusted-host", LaunchOptions{}); err == nil {
		t.Fatal("prod Launch without trust root should fail")
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

// TestHost_Launch_StartOverride_Good — LaunchOptions.Start is honoured
// even when the caller did not also supply a parsed manifest.
func TestHost_Launch_StartOverride_Good(t *testing.T) {
	project := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "override-plugin",
		Name:    "Override Plugin",
		Version: "0.1.0",
		Layout:  "C",
	}
	body, err := yamlMarshal(&manifest)
	if err != nil {
		t.Fatalf("yamlMarshal: %v", err)
	}
	path := core.Path(project, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(path)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(path, body); err != nil {
		t.Fatalf("Write: %v", err)
	}

	h := NewHost(HostOptions{Home: t.TempDir(), Mode: ModeDev})
	inst, err := h.Launch(context.Background(), "override-plugin", LaunchOptions{
		Start: project,
	})
	if err != nil {
		t.Fatalf("Launch(Start override): %v", err)
	}
	if inst.Root != project {
		t.Errorf("Root = %q; want %q", inst.Root, project)
	}
}

// TestHost_Launch_ProdPrefersCompiled_Good — prod hosts use the signed
// core.json artifact when present, even if view.yaml has changed since
// the last compile.
func TestHost_Launch_ProdPrefersCompiled_Good(t *testing.T) {
	home, root, pubHex, priv := newSignedHostTestFixture(t, "compiled-host", "Compiled Name")

	var manifest config.ViewManifest
	viewPath := core.Path(root, ".core", "view.yaml")
	if err := LoadViewManifest(coreio.Local, viewPath, &manifest); err != nil {
		t.Fatalf("LoadViewManifest: %v", err)
	}
	cm, err := Compile(&manifest, CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := WriteCompiled(coreio.Local, root, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	manifest.Name = "Source Name"
	if err := SignManifest(&manifest, priv); err != nil {
		t.Fatalf("SignManifest(view): %v", err)
	}
	body, err := yamlMarshal(&manifest)
	if err != nil {
		t.Fatalf("yamlMarshal(view): %v", err)
	}
	if err := coreio.Local.Write(viewPath, body); err != nil {
		t.Fatalf("Write(view): %v", err)
	}

	h := NewHost(HostOptions{
		Home:           home,
		Mode:           ModeProd,
		PublicKeyHex:   pubHex,
		DisableKeyLoad: true,
	})
	inst, err := h.Launch(context.Background(), "compiled-host", LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch(prod compiled): %v", err)
	}
	if inst.Manifest.Name != "Compiled Name" {
		t.Errorf("Manifest.Name = %q; want %q", inst.Manifest.Name, "Compiled Name")
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
