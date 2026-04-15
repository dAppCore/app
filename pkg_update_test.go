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

// TestPkgUpdate_PkgUpdate_Good_Web — `wrap:web:<dir>` source triggers a
// fresh WrapWeb against the recorded directory and replaces the install.
// Mutating the wrapped source's index.html between updates is observable
// via the manifest's Code (slug derived from the dir name).
func TestPkgUpdate_PkgUpdate_Good_Web(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	// Plant a wrapped web directory the update will rewrap from.
	srcDir := core.Path(t.TempDir(), "site")
	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>v1</html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	// First wrap + install so PkgUpdate has a starting state.
	manifest, err := app.WrapWeb(medium, srcDir, app.WrapWebOptions{
		Code: "site", Name: "Site", Version: "0.1.0",
	})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}
	if _, err := app.InstallWrappedWeb(medium, manifest, app.PkgInstallOptions{
		Home:   home,
		Force:  true,
		Source: "wrap:web:" + srcDir,
	}); err != nil {
		t.Fatalf("InstallWrappedWeb: %v", err)
	}

	// Mutate the source so the update has something to refresh.
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>v2</html>"); err != nil {
		t.Fatalf("Write index.html v2: %v", err)
	}

	dest, err := app.PkgUpdate(context.Background(), medium, home, "site")
	if err != nil {
		t.Fatalf("PkgUpdate (web): %v", err)
	}
	if dest == "" {
		t.Fatal("PkgUpdate (web) returned empty dest")
	}

	var round config.ViewManifest
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Code != "site" {
		t.Errorf("Code = %q; want 'site'", round.Code)
	}
	if src, _ := round.Config["source"].(string); src != "wrap:web:"+srcDir {
		t.Errorf("source stamp lost after update: %q", src)
	}
}

// TestPkgUpdate_PkgUpdate_Bad_Web — a wrap:web source whose recorded
// directory has been deleted fails with a typed error so the operator
// knows the rewrap cannot proceed.
func TestPkgUpdate_PkgUpdate_Bad_Web(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	missing := core.Path(t.TempDir(), "deleted-source")
	manifest := &config.ViewManifest{
		Code:    "ghost-web",
		Name:    "Ghost Web",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "wrap:web:" + missing,
		},
	}
	writeInstalled(t, medium, home, "ghost-web", manifest)

	if _, err := app.PkgUpdate(context.Background(), medium, home, "ghost-web"); err == nil {
		t.Error("missing web source produced no error")
	}
}

// TestPkgUpdate_PkgUpdate_Good_Electron — `wrap:electron:<dir>` source
// re-runs ScanElectronRenderer against the recorded directory and
// rewrites the install. The renderer JS scan picks up new API patterns
// (e.g. an added `clipboard.writeText`) which become permission flags
// on the rewrapped manifest.
func TestPkgUpdate_PkgUpdate_Good_Electron(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	// Plant a minimal Electron source: package.json + main.js with
	// an Electron API the scanner recognises (require('fs')).
	srcDir := core.Path(t.TempDir(), "electron-src")
	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir electron-src: %v", err)
	}
	pkgJSON := `{"name":"electron-update","version":"0.1.0","main":"main.js"}`
	if err := medium.Write(core.Path(srcDir, "package.json"), pkgJSON); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	mainJS := `const fs = require('fs'); fs.readFileSync('hello');`
	if err := medium.Write(core.Path(srcDir, "main.js"), mainJS); err != nil {
		t.Fatalf("Write main.js: %v", err)
	}

	// Plant the install with the wrap:electron source recorded.
	manifest := &config.ViewManifest{
		Code:    "electron-update",
		Name:    "Electron Update",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "wrap:electron:" + srcDir,
		},
	}
	writeInstalled(t, medium, home, "electron-update", manifest)

	dest, err := app.PkgUpdate(context.Background(), medium, home, "electron-update")
	if err != nil {
		t.Fatalf("PkgUpdate (electron): %v", err)
	}
	if dest == "" {
		t.Fatal("PkgUpdate (electron) returned empty dest")
	}

	var round config.ViewManifest
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Code != "electron-update" {
		t.Errorf("Code = %q; want 'electron-update'", round.Code)
	}
	// The fs.readFileSync usage should now appear as a read permission.
	if len(round.Permissions.Read) == 0 && !round.Permissions.Filesystem {
		t.Errorf("rewrap should detect filesystem usage; perms = %+v", round.Permissions)
	}
}

// TestPkgUpdate_PkgUpdate_Bad_Electron — wrap:electron sources that
// reference a missing dir or repo fail with a typed error.
func TestPkgUpdate_PkgUpdate_Bad_Electron(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	missing := core.Path(t.TempDir(), "deleted-electron")
	manifest := &config.ViewManifest{
		Code:    "ghost-electron",
		Name:    "Ghost Electron",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "wrap:electron:" + missing,
		},
	}
	writeInstalled(t, medium, home, "ghost-electron", manifest)

	if _, err := app.PkgUpdate(context.Background(), medium, home, "ghost-electron"); err == nil {
		t.Error("missing electron source produced no error")
	}
}

// TestPkgUpdate_PkgUpdate_Ugly_Electron — a wrap:electron source whose
// directory exists but lacks package.json fails cleanly. Useful for
// catching half-extracted release archives early.
func TestPkgUpdate_PkgUpdate_Ugly_Electron(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	emptyDir := core.Path(t.TempDir(), "no-package")
	if err := medium.EnsureDir(emptyDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "incomplete-electron",
		Name:    "Incomplete",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "wrap:electron:" + emptyDir,
		},
	}
	writeInstalled(t, medium, home, "incomplete-electron", manifest)

	if _, err := app.PkgUpdate(context.Background(), medium, home, "incomplete-electron"); err == nil {
		t.Error("missing package.json produced no error")
	}
}

// TestPkgUpdate_PkgUpdate_Good_Local — `local:<path>` source re-runs
// PkgInstallLocal so iterative development on a local CoreApp can be
// pushed into the install tree without manual reinstalls.
func TestPkgUpdate_PkgUpdate_Good_Local(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	srcDir := core.Path(t.TempDir(), "local-src")
	if err := medium.EnsureDir(core.Path(srcDir, ".core")); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, ".core", "view.yaml"),
		"code: local-up\nname: Local Up V2\nversion: 0.2.0\n"); err != nil {
		t.Fatalf("Write source view.yaml: %v", err)
	}

	// Initial install so PkgUpdate has a starting state.
	if _, err := app.PkgInstallLocal(medium, srcDir, app.PkgInstallOptions{
		Home:   home,
		Force:  true,
		Source: "local:" + srcDir,
	}); err != nil {
		t.Fatalf("PkgInstallLocal: %v", err)
	}

	// Mutate the source — bump the version so the update is observable.
	if err := medium.Write(core.Path(srcDir, ".core", "view.yaml"),
		"code: local-up\nname: Local Up V3\nversion: 0.3.0\n"); err != nil {
		t.Fatalf("Write source view.yaml v3: %v", err)
	}

	dest, err := app.PkgUpdate(context.Background(), medium, home, "local-up")
	if err != nil {
		t.Fatalf("PkgUpdate (local): %v", err)
	}
	if dest == "" {
		t.Fatal("PkgUpdate (local) returned empty dest")
	}

	var round config.ViewManifest
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Version != "0.3.0" {
		t.Errorf("Version = %q; want '0.3.0' (update did not re-copy)", round.Version)
	}
}

// TestPkgUpdate_PkgUpdate_Bad_Local — local source that vanished from
// the filesystem fails with a typed error so the operator knows the
// re-copy cannot proceed.
func TestPkgUpdate_PkgUpdate_Bad_Local(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	missing := core.Path(t.TempDir(), "deleted-local")
	manifest := &config.ViewManifest{
		Code:    "ghost-local",
		Name:    "Ghost Local",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "local:" + missing,
		},
	}
	writeInstalled(t, medium, home, "ghost-local", manifest)

	if _, err := app.PkgUpdate(context.Background(), medium, home, "ghost-local"); err == nil {
		t.Error("missing local source produced no error")
	}
}
