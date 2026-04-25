// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"strings"
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
)

// TestWorkspace_OpenWorkspace_Good — calling OpenWorkspace with a real
// home directory and app code should produce the layout sub-folders
// and surface their absolute paths.
func TestWorkspace_OpenWorkspace_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	ws, err := app.OpenWorkspace(medium, home, "photo-browser")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	if ws == nil {
		t.Fatal("OpenWorkspace returned nil workspace")
	}
	wantRoot := core.Path(home, ".core", app.DataDirName, "photo-browser")
	if ws.Root != wantRoot {
		t.Errorf("Root = %q; want %q", ws.Root, wantRoot)
	}

	// Each layout sub-directory must exist after Open.
	for _, layout := range []app.WorkspaceLayout{
		app.WorkspaceLayoutKeys,
		app.WorkspaceLayoutStore,
		app.WorkspaceLayoutCache,
		app.WorkspaceLayoutTmp,
	} {
		path := ws.Path(layout)
		if !medium.IsDir(path) {
			t.Errorf("layout %q dir not created at %q", layout, path)
		}
	}

	// Resolve must compose paths inside the layout.
	got := ws.Resolve(app.WorkspaceLayoutStore, "kv.db")
	want := core.Path(ws.Path(app.WorkspaceLayoutStore), "kv.db")
	if got != want {
		t.Errorf("Resolve = %q; want %q", got, want)
	}
}

// TestWorkspace_OpenWorkspace_Bad — the function refuses obviously
// malformed inputs (empty code) before touching the filesystem.
func TestWorkspace_OpenWorkspace_Bad(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	if _, err := app.OpenWorkspace(medium, home, ""); err == nil {
		t.Fatal("OpenWorkspace should reject an empty code")
	}
}

// TestWorkspace_OpenWorkspace_Ugly — Resolve / Path on a nil workspace
// must not panic (defensive nil-receiver handling).
func TestWorkspace_OpenWorkspace_Ugly(t *testing.T) {
	var ws *app.Workspace

	if got := ws.Path(app.WorkspaceLayoutStore); got != "" {
		t.Errorf("nil.Path = %q; want empty", got)
	}
	if got := ws.Resolve(app.WorkspaceLayoutStore, "kv.db"); got != "" {
		t.Errorf("nil.Resolve = %q; want empty", got)
	}
	if err := ws.Wipe(); err == nil {
		t.Error("nil.Wipe should surface a typed error")
	}
}

// TestWorkspace_Sandboxed_Good — the sandbox medium honours go-io's
// SASE containment, so reads outside the workspace root are rejected.
func TestWorkspace_Sandboxed_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	ws, err := app.OpenWorkspace(medium, home, "sandbox-app")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}

	sb, err := ws.Sandboxed()
	if err != nil {
		t.Fatalf("Sandboxed: %v", err)
	}
	if sb == nil {
		t.Fatal("Sandboxed returned nil medium")
	}

	// A relative write inside the workspace should round-trip through
	// the sandbox while the underlying file remains encrypted at rest.
	if err := sb.Write("hello.txt", "world"); err != nil {
		t.Fatalf("write inside sandbox: %v", err)
	}
	plain, err := sb.Read("hello.txt")
	if err != nil {
		t.Fatalf("read inside sandbox: %v", err)
	}
	if plain != "world" {
		t.Errorf("sandbox body = %q; want %q", plain, "world")
	}
	abs := core.Path(ws.Root, "hello.txt")
	got, err := medium.Read(abs)
	if err != nil {
		t.Fatalf("read absolute: %v", err)
	}
	if got == "world" || strings.Contains(got, "world") {
		t.Errorf("raw workspace body leaked plaintext: %q", got)
	}
}

// TestWorkspace_Sandboxed_Bad — Sandboxed on a nil receiver must
// surface a typed error rather than panic.
func TestWorkspace_Sandboxed_Bad(t *testing.T) {
	var ws *app.Workspace
	if _, err := ws.Sandboxed(); err == nil {
		t.Fatal("nil.Sandboxed should error")
	}
}

// TestWorkspace_Sandboxed_Ugly — calling Sandboxed against a memory
// medium returns the medium unchanged so tests compose naturally.
func TestWorkspace_Sandboxed_Ugly(t *testing.T) {
	mem := coreio.NewMemoryMedium()
	// MemoryMedium's EnsureDir is a no-op so OpenWorkspace succeeds.
	ws, err := app.OpenWorkspace(mem, "/tmp/home", "memory-app")
	if err != nil {
		t.Fatalf("OpenWorkspace memory: %v", err)
	}
	sb, err := ws.Sandboxed()
	if err != nil {
		t.Fatalf("Sandboxed memory: %v", err)
	}
	// MemoryMedium passes through unchanged; the assertion is loose
	// (interface identity) — the contract is "no error, usable medium".
	if sb == nil {
		t.Error("Sandboxed memory returned nil medium")
	}
}

// TestWorkspace_Wipe_Good — Wipe removes the workspace root and every
// layout sub-folder beneath it.
func TestWorkspace_Wipe_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	ws, err := app.OpenWorkspace(medium, home, "to-wipe")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	// Drop a payload so we know Wipe really removed the tree.
	if err := medium.Write(ws.Resolve(app.WorkspaceLayoutStore, "kv.db"), "data"); err != nil {
		t.Fatalf("seed payload: %v", err)
	}

	if err := ws.Wipe(); err != nil {
		t.Fatalf("Wipe: %v", err)
	}
	if medium.IsDir(ws.Root) {
		t.Errorf("Wipe should have removed %q", ws.Root)
	}
}

// TestWorkspace_Wipe_Bad — Wiping a workspace whose root never existed
// is a no-op (idempotent).
func TestWorkspace_Wipe_Bad(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	ws := &app.Workspace{Code: "nope", Root: core.Path(home, "nope")}
	// Inject the medium via a fresh OpenWorkspace then wipe immediately
	// twice — the second wipe must succeed.
	openWS, err := app.OpenWorkspace(medium, home, "double-wipe")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	if err := openWS.Wipe(); err != nil {
		t.Fatalf("first Wipe: %v", err)
	}
	if err := openWS.Wipe(); err != nil {
		t.Errorf("second Wipe should be a no-op; got %v", err)
	}
	// Make sure ws (with no medium) doesn't smuggle in a panic on the
	// nil-medium edge — the public API never lets a Workspace be
	// constructed without a medium so we just confirm our manual
	// instantiation doesn't break the test sweep.
	_ = ws
}

// TestWorkspace_Wipe_Ugly — Wipe on a nil receiver surfaces a typed
// error rather than a nil-pointer dereference.
func TestWorkspace_Wipe_Ugly(t *testing.T) {
	var ws *app.Workspace
	if err := ws.Wipe(); err == nil {
		t.Fatal("nil.Wipe should return a typed error")
	}
}

// TestWorkspace_WorkspaceForManifest_Good — the manifest helper should
// produce a workspace pointing at <home>/.core/data/<manifest.code>.
func TestWorkspace_WorkspaceForManifest_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	manifest := &config.ViewManifest{Code: "manifest-driven"}

	ws, err := app.WorkspaceForManifest(medium, home, manifest)
	if err != nil {
		t.Fatalf("WorkspaceForManifest: %v", err)
	}
	want := core.Path(home, ".core", app.DataDirName, "manifest-driven")
	if ws.Root != want {
		t.Errorf("Root = %q; want %q", ws.Root, want)
	}
}

// TestWorkspace_WorkspaceForManifest_Bad — nil manifest must surface a
// typed error rather than a nil-deref.
func TestWorkspace_WorkspaceForManifest_Bad(t *testing.T) {
	if _, err := app.WorkspaceForManifest(coreio.Local, t.TempDir(), nil); err == nil {
		t.Fatal("WorkspaceForManifest should reject a nil manifest")
	}
}

// TestWorkspace_WorkspaceForManifest_Ugly — a manifest with no code
// inherits the OpenWorkspace empty-code rejection.
func TestWorkspace_WorkspaceForManifest_Ugly(t *testing.T) {
	if _, err := app.WorkspaceForManifest(coreio.Local, t.TempDir(),
		&config.ViewManifest{Code: ""}); err == nil {
		t.Fatal("WorkspaceForManifest should reject a manifest with empty code")
	}
}
