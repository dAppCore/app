// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	"dappco.re/go/io/cube"
)

// DataDirName is the component that follows `$DIR_HOME/.core/` for the
// per-app data tree. Mirrors the dAppServer convention
// (`~/Lethean/data/{lthnHash}/`) ported to the `.core` root with an
// app-code subdirectory so each CoreApp's storage is name-spaced by
// identity rather than a hashed enclave id.
//
//	dir := core.Path(home, ".core", app.DataDirName, "photo-browser")
const DataDirName = "data"

// WorkspaceLayout names a sub-folder convention within a workspace
// directory. The layout matches the dAppServer enclave layout — keys
// for cryptographic material, store for the SQLite KV, cache for
// transient blobs, and tmp for scratch files the runtime can purge
// without warning.
//
//	keys := workspace.Path(app.WorkspaceLayoutKeys)
type WorkspaceLayout string

const (
	// WorkspaceLayoutKeys holds workspace-scoped key material (PGP
	// keypairs, ed25519 device keys). Encrypted at rest via Sigil
	// when go-io exposes the workspace key derivation API.
	WorkspaceLayoutKeys WorkspaceLayout = "keys"

	// WorkspaceLayoutStore is where go-store's SQLite database lives
	// for this app. One database per app, scoped by manifest.code.
	WorkspaceLayoutStore WorkspaceLayout = "store"

	// WorkspaceLayoutCache is for transient blobs (HTTP responses,
	// thumbnails, computed artefacts). May be evicted by the runtime
	// without notifying the app — applications must treat it as a
	// best-effort cache, not a primary store.
	WorkspaceLayoutCache WorkspaceLayout = "cache"

	// WorkspaceLayoutTmp is scratch space for the duration of an app
	// run. Cleared on next boot.
	WorkspaceLayoutTmp WorkspaceLayout = "tmp"
)

// Workspace is the per-app data directory the runtime reserves for an
// installed CoreApp. It encapsulates the path layout
// (`~/.core/data/<code>/{keys,store,cache,tmp}`) so callers don't
// duplicate the convention. Workspaces are created lazily — Open()
// returns a ready-to-use directory tree, EnsureDir taking care of any
// missing folders.
//
//	ws, err := app.OpenWorkspace(coreio.Local, home, "photo-browser")
//	if err != nil { return err }
//	storePath := ws.Path(app.WorkspaceLayoutStore)
type Workspace struct {
	// Code is the manifest.code of the owning app — also the workspace
	// directory name under DataDirName.
	Code string
	// Root is the absolute workspace root
	// (`<home>/.core/<DataDirName>/<Code>`). Reading or writing outside
	// this root is a permission violation that go-io's SASE containment
	// catches at the medium boundary.
	Root string
	// medium is retained so Path / Resolve calls stay symmetric with the
	// boot-time storage abstraction (matters for tests that hand in a
	// MockMedium).
	medium coreio.Medium

	sandboxOnce   sync.Once
	sandboxMedium coreio.Medium
	sandboxErr    error
}

// OpenWorkspace resolves a workspace for the supplied app code and
// guarantees the layout sub-directories exist. If `home` is empty it
// falls back to `$DIR_HOME` so callers in CLI-land don't repeat the
// lookup.
//
//	ws, err := app.OpenWorkspace(coreio.Local, "", "photo-browser")
//
// Rules:
//
//   - Empty `code` → typed error (the workspace name is identity).
//
//   - Empty `home` and empty `$DIR_HOME` → typed error (the runtime
//     cannot resolve a default location).
//
//   - The medium's EnsureDir creates the layout sub-folders. Existing
//     directories are left untouched.
func OpenWorkspace(medium coreio.Medium, home, code string) (*Workspace, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if code == "" {
		return nil, core.E("app.OpenWorkspace", "empty app code", nil)
	}
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return nil, core.E("app.OpenWorkspace", "cannot resolve home directory", nil)
	}

	root := core.Path(home, ".core", DataDirName, code)
	ws := &Workspace{Code: code, Root: root, medium: medium}

	for _, layout := range []WorkspaceLayout{
		WorkspaceLayoutKeys,
		WorkspaceLayoutStore,
		WorkspaceLayoutCache,
		WorkspaceLayoutTmp,
	} {
		if err := medium.EnsureDir(ws.Path(layout)); err != nil {
			return nil, core.E("app.OpenWorkspace", "ensure layout dir failed: "+string(layout), err)
		}
	}
	return ws, nil
}

// Path returns the absolute path of one of the workspace layout
// sub-directories. Returns the workspace root for an unknown layout so
// callers don't accidentally write outside the workspace boundary.
//
//	store := ws.Path(app.WorkspaceLayoutStore)
//	// "<home>/.core/data/photo-browser/store"
func (w *Workspace) Path(layout WorkspaceLayout) string {
	if w == nil {
		return ""
	}
	if layout == "" {
		return w.Root
	}
	return core.Path(w.Root, string(layout))
}

// Resolve joins a relative path against a layout sub-directory. The
// helper is the recommended way to compose paths inside a workspace —
// straight string concatenation skips the layout boundary check below.
//
//	dbPath := ws.Resolve(app.WorkspaceLayoutStore, "kv.db")
//	// "<home>/.core/data/photo-browser/store/kv.db"
//
// Rules:
//
//   - An empty layout writes to the workspace root.
//
//   - A relative input is appended via core.Path so platform separators
//     are normalised.
//
//   - The function does NOT walk symlinks or validate the joined path;
//     SASE containment at the medium level is the security boundary.
func (w *Workspace) Resolve(layout WorkspaceLayout, rel string) string {
	if w == nil {
		return ""
	}
	if rel == "" {
		return w.Path(layout)
	}
	return core.Path(w.Path(layout), rel)
}

// Sandboxed returns a Medium whose root is pinned to the workspace
// directory and whose file payloads are encrypted at rest. The returned
// medium honours go-io's SASE containment — reads and writes outside
// the workspace root are rejected at the medium boundary. Useful when
// an app handler wants a `coreio.Medium` it can hand to a sub-system
// without re-doing path checks.
//
//	m, err := ws.Sandboxed()
//	if err != nil { return err }
//	_ = m.Write("config.json", body) // encrypted at <root>/config.json
//
// On a MemoryMedium / MockMedium where Sandboxed isn't available the
// underlying medium is returned unchanged so tests compose naturally.
func (w *Workspace) Sandboxed() (coreio.Medium, error) {
	if w == nil {
		return nil, core.E("app.Workspace.Sandboxed", "nil workspace", nil)
	}
	if w.medium != coreio.Local {
		// Mock and memory mediums have no notion of an OS root —
		// returning the medium itself preserves the caller's existing
		// expectations during tests.
		return w.medium, nil
	}
	w.sandboxOnce.Do(func() {
		w.sandboxMedium, w.sandboxErr = w.openSandboxed()
	})
	if w.sandboxErr != nil {
		return nil, w.sandboxErr
	}
	return w.sandboxMedium, nil
}

func (w *Workspace) openSandboxed() (coreio.Medium, error) {
	sandbox, err := coreio.NewSandboxed(w.Root)
	if err != nil {
		return nil, core.E("app.Workspace.Sandboxed", "sandbox workspace failed", err)
	}
	secret, err := deriveWorkspaceSecret(w)
	if err != nil {
		return nil, core.E("app.Workspace.Sandboxed", "derive workspace key failed", err)
	}
	defer zeroBytes(secret)
	encrypted, err := cube.New(cube.Options{Inner: sandbox, Key: secret})
	if err != nil {
		return nil, core.E("app.Workspace.Sandboxed", "encrypt workspace medium failed", err)
	}
	return encrypted, nil
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// Wipe removes every layout sub-directory and the workspace root
// itself. Used by `core pkg remove --purge` and the test suite's
// per-test cleanup helpers. Missing roots are treated as success — a
// fresh user calling Wipe should not see an error.
//
//	err := ws.Wipe()
func (w *Workspace) Wipe() error {
	if w == nil {
		return core.E("app.Workspace.Wipe", "nil workspace", nil)
	}
	if !w.medium.IsDir(w.Root) {
		return nil
	}
	if err := w.medium.DeleteAll(w.Root); err != nil {
		return core.E("app.Workspace.Wipe", "delete workspace failed: "+w.Root, err)
	}
	return nil
}

// WorkspaceForManifest is a convenience that opens the workspace for
// the given manifest. Equivalent to OpenWorkspace(medium, home, m.Code)
// with a clearer call site at the boot edge.
//
//	ws, err := app.WorkspaceForManifest(coreio.Local, home, &inst.Manifest)
func WorkspaceForManifest(medium coreio.Medium, home string, m *config.ViewManifest) (*Workspace, error) {
	if m == nil {
		return nil, core.E("app.WorkspaceForManifest", "nil manifest", nil)
	}
	return OpenWorkspace(medium, home, m.Code)
}
