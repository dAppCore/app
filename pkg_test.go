// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
	"gopkg.in/yaml.v3"
)

// TestPkg_PkgList_Good installs two wrapped apps into a fake home tree
// and asserts both show up in the returned list with the expected type
// and source fields.
func TestPkg_PkgList_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	// Install A — a PWA wrap.
	pwaManifest := &config.ViewManifest{
		Code:    "a",
		Name:    "A",
		Version: "0.1.0",
		Config: map[string]any{
			"type":   "pwa",
			"source": "marketplace:a",
			"url":    "https://a.example.com",
		},
	}
	writeInstalled(t, medium, home, "a", pwaManifest)

	// Install B — a native app.
	nativeManifest := &config.ViewManifest{
		Code:    "b",
		Name:    "B",
		Version: "1.0.0",
	}
	writeInstalled(t, medium, home, "b", nativeManifest)

	entries, err := app.PkgList(medium, home)
	if err != nil {
		t.Fatalf("PkgList: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d; want 2", len(entries))
	}

	byName := map[string]app.PkgEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	if byName["a"].Type != app.PackageTypePWA {
		t.Errorf("a.Type = %v; want PWA", byName["a"].Type)
	}
	if byName["a"].Source != "marketplace:a" {
		t.Errorf("a.Source = %q; want 'marketplace:a'", byName["a"].Source)
	}
	if byName["b"].Type != app.PackageTypeNative {
		t.Errorf("b.Type = %v; want Native", byName["b"].Type)
	}
	if byName["b"].Source != "local" {
		t.Errorf("b.Source = %q; want 'local' fallback", byName["b"].Source)
	}
}

// TestPkg_PkgList_Bad rejects empty home; missing apps dir returns an
// empty slice (not an error).
func TestPkg_PkgList_Bad(t *testing.T) {
	if _, err := app.PkgList(coreio.Local, ""); err == nil {
		t.Error("PkgList(\"\") returned no error")
	}
	home := t.TempDir()
	// No .core/apps subtree — expect nil slice, nil error.
	entries, err := app.PkgList(coreio.Local, home)
	if err != nil {
		t.Errorf("PkgList on empty home returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("PkgList on empty home returned %d entries; want 0", len(entries))
	}
}

// TestPkg_PkgList_Ugly skips hidden dirs, skips dirs without a
// view.yaml, and keeps going when one install has a broken manifest.
func TestPkg_PkgList_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	// Good install.
	writeInstalled(t, medium, home, "good", &config.ViewManifest{
		Code: "good", Name: "Good", Version: "0.1.0",
	})
	// Hidden directory — should be skipped.
	if err := medium.EnsureDir(core.Path(home, ".core", app.AppsDirName, ".hidden")); err != nil {
		t.Fatalf("EnsureDir hidden: %v", err)
	}
	// Directory without view.yaml — should be skipped.
	if err := medium.EnsureDir(core.Path(home, ".core", app.AppsDirName, "empty", ".core")); err != nil {
		t.Fatalf("EnsureDir empty: %v", err)
	}
	// Broken manifest — should be skipped (not fatal).
	if err := medium.EnsureDir(core.Path(home, ".core", app.AppsDirName, "broken", ".core")); err != nil {
		t.Fatalf("EnsureDir broken: %v", err)
	}
	if err := medium.Write(core.Path(home, ".core", app.AppsDirName, "broken", ".core", "view.yaml"), "not: yaml: at: all: ::::"); err != nil {
		t.Fatalf("Write broken: %v", err)
	}

	entries, err := app.PkgList(medium, home)
	if err != nil {
		t.Fatalf("PkgList: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("len(entries) = %d; want 1 (good only)", len(entries))
	}
	if entries[0].Name != "good" {
		t.Errorf("entries[0].Name = %q; want 'good'", entries[0].Name)
	}
}

// TestPkg_PkgRemove_Good installs a package and confirms Remove deletes
// the entire install directory.
func TestPkg_PkgRemove_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	writeInstalled(t, medium, home, "to-delete", &config.ViewManifest{
		Code: "to-delete", Name: "X", Version: "0.1.0",
	})
	path := core.Path(home, ".core", app.AppsDirName, "to-delete")
	if !medium.IsDir(path) {
		t.Fatalf("install dir missing: %s", path)
	}
	if err := app.PkgRemove(medium, home, "to-delete"); err != nil {
		t.Fatalf("PkgRemove: %v", err)
	}
	if medium.IsDir(path) {
		t.Errorf("install dir still exists after PkgRemove: %s", path)
	}
}

// TestPkg_PkgRemove_Bad rejects empty home, empty name, name with path
// separators, and not-installed packages.
func TestPkg_PkgRemove_Bad(t *testing.T) {
	if err := app.PkgRemove(coreio.Local, "", "x"); err == nil {
		t.Error("empty home produced no error")
	}
	if err := app.PkgRemove(coreio.Local, t.TempDir(), ""); err == nil {
		t.Error("empty name produced no error")
	}
	if err := app.PkgRemove(coreio.Local, t.TempDir(), "../escape"); err == nil {
		t.Error("name with path separator produced no error")
	}
	if err := app.PkgRemove(coreio.Local, t.TempDir(), "missing"); err == nil {
		t.Error("missing package produced no error")
	}
}

// TestPkg_PkgRemoveWith_Purge_Good — Purge=true removes both the
// install tree and the workspace data tree so a reinstall starts
// clean. Matches the `pkg remove --purge` affordance referenced in
// RFC §16.3 and the workspace.Wipe contract.
func TestPkg_PkgRemoveWith_Purge_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	writeInstalled(t, medium, home, "purge-me", &config.ViewManifest{
		Code: "purge-me", Name: "X", Version: "0.1.0",
	})
	// Seed the workspace data tree so the purge path has something to
	// delete — OpenWorkspace creates the layout subdirs lazily.
	if _, err := app.OpenWorkspace(medium, home, "purge-me"); err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	dataPath := core.Path(home, ".core", app.DataDirName, "purge-me")
	if !medium.IsDir(dataPath) {
		t.Fatalf("workspace data tree missing at %s", dataPath)
	}
	if err := app.PkgRemoveWith(medium, home, "purge-me", app.PkgRemoveOptions{
		Purge: true,
	}); err != nil {
		t.Fatalf("PkgRemoveWith(Purge=true): %v", err)
	}
	if medium.IsDir(core.Path(home, ".core", app.AppsDirName, "purge-me")) {
		t.Errorf("install tree survived purge")
	}
	if medium.IsDir(dataPath) {
		t.Errorf("workspace data tree survived purge: %s", dataPath)
	}
}

// TestPkg_PkgRemoveWith_Purge_NoData_Good — Purge=true against a
// package with no workspace data tree is a silent success (the user
// never booted the app so no data was written).
func TestPkg_PkgRemoveWith_Purge_NoData_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	writeInstalled(t, medium, home, "no-data", &config.ViewManifest{
		Code: "no-data", Name: "X", Version: "0.1.0",
	})
	if err := app.PkgRemoveWith(medium, home, "no-data", app.PkgRemoveOptions{
		Purge: true,
	}); err != nil {
		t.Fatalf("PkgRemoveWith(Purge=true, no data): %v", err)
	}
	if medium.IsDir(core.Path(home, ".core", app.AppsDirName, "no-data")) {
		t.Errorf("install tree survived purge")
	}
}

// TestPkg_PkgRemoveWith_Bad — the options-aware entry point shares the
// validation surface with PkgRemove; every rejection case still fires.
func TestPkg_PkgRemoveWith_Bad(t *testing.T) {
	if err := app.PkgRemoveWith(coreio.Local, "", "x", app.PkgRemoveOptions{}); err == nil {
		t.Error("empty home produced no error")
	}
	if err := app.PkgRemoveWith(coreio.Local, t.TempDir(), "", app.PkgRemoveOptions{}); err == nil {
		t.Error("empty name produced no error")
	}
	if err := app.PkgRemoveWith(coreio.Local, t.TempDir(), "x/y", app.PkgRemoveOptions{}); err == nil {
		t.Error("name with slash produced no error")
	}
	if err := app.PkgRemoveWith(coreio.Local, t.TempDir(), "missing", app.PkgRemoveOptions{
		Purge: true,
	}); err == nil {
		t.Error("missing package with Purge=true produced no error")
	}
}

// TestPkg_InstallWrappedPWA_Good installs a wrapped PWA and confirms
// the view.yaml lands in the expected place with the source stamp.
func TestPkg_InstallWrappedPWA_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	manifest := app.WrapPWA(&app.PWAManifest{
		Name:     "Play",
		StartURL: "https://play.example.com/",
	}, app.WrapPWAOptions{})

	dest, err := app.InstallWrappedPWA(medium, manifest,
		app.PkgInstallOptions{Home: home, Source: "marketplace:play"})
	if err != nil {
		t.Fatalf("InstallWrappedPWA: %v", err)
	}
	if dest == "" {
		t.Fatal("InstallWrappedPWA returned empty dest")
	}
	if !medium.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Fatalf("view.yaml missing at %s", dest)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:play" {
		t.Errorf("source stamp = %q; want 'marketplace:play'", src)
	}
	if round.Sign == "" {
		t.Error("wrapped install was left unsigned")
	}
}

// TestPkg_InstallWrappedPWA_ProdBootSigned_Good confirms wrapped
// installs are auto-signed with the per-home default keypair so a prod
// boot can verify them immediately after install.
func TestPkg_InstallWrappedPWA_ProdBootSigned_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	manifest := app.WrapPWA(&app.PWAManifest{
		Name:     "Signed Play",
		StartURL: "https://play.example.com/",
	}, app.WrapPWAOptions{})
	dest, err := app.InstallWrappedPWA(medium, manifest, app.PkgInstallOptions{Home: home})
	if err != nil {
		t.Fatalf("InstallWrappedPWA: %v", err)
	}

	if !medium.Exists(core.Path(home, ".core", "keys", app.DefaultKeyName)) {
		t.Fatalf("default private key missing after install")
	}
	if !medium.Exists(core.Path(home, ".core", "keys", "default.pub")) {
		t.Fatalf("default public key missing after install")
	}

	inst, err := app.Boot(context.Background(), dest,
		app.WithTrustedKeysDir(core.Path(home, ".core", "keys")),
		app.WithWorkspaceHome(home),
	)
	if err != nil {
		t.Fatalf("Boot(prod): %v", err)
	}
	if inst.Manifest.Sign == "" {
		t.Fatal("booted wrapped manifest had empty signature")
	}
}

// TestPkg_InstallWrappedPWA_Bad rejects nil manifest, empty code, and
// installs that cannot be written (e.g. bad path).
func TestPkg_InstallWrappedPWA_Bad(t *testing.T) {
	if _, err := app.InstallWrappedPWA(coreio.Local, nil, app.PkgInstallOptions{Home: "/tmp"}); err == nil {
		t.Error("nil manifest produced no error")
	}
	// Manifest without a code — should fail.
	if _, err := app.InstallWrappedPWA(coreio.Local,
		&config.ViewManifest{Name: "x"}, app.PkgInstallOptions{Home: t.TempDir()}); err == nil {
		t.Error("empty code produced no error")
	}
}

// TestPkg_InstallWrappedPWA_Ugly covers the Force=true replace flow.
func TestPkg_InstallWrappedPWA_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	manifest := &config.ViewManifest{
		Code:    "dup",
		Name:    "First",
		Version: "0.1.0",
	}
	if _, err := app.InstallWrappedPWA(medium, manifest, app.PkgInstallOptions{Home: home}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	// Second install without Force → error.
	manifest.Name = "Second"
	if _, err := app.InstallWrappedPWA(medium, manifest, app.PkgInstallOptions{Home: home}); err == nil {
		t.Error("duplicate install without Force produced no error")
	}
	// With Force → succeeds, and the new name survives.
	if _, err := app.InstallWrappedPWA(medium, manifest,
		app.PkgInstallOptions{Home: home, Force: true}); err != nil {
		t.Fatalf("force install: %v", err)
	}
	var round config.ViewManifest
	path := core.Path(home, ".core", app.AppsDirName, "dup", ".core", "view.yaml")
	if err := app.LoadViewManifest(medium, path, &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Name != "Second" {
		t.Errorf("force-replaced Name = %q; want 'Second'", round.Name)
	}
}

// TestPkg_InstallWrappedPWA_AssetsCopied confirms that wrapped local
// PWAs carry their asset tree into the installed package, not just the
// generated manifest.
func TestPkg_InstallWrappedPWA_AssetsCopied(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(srcDir, "manifest.json"), `{"name":"Play","short_name":"play","start_url":"/index.html"}`); err != nil {
		t.Fatalf("Write manifest.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>play</html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "play",
		Name:    "Play",
		Version: "0.1.0",
		Config:  map[string]any{"type": "pwa"},
	}
	dest, err := app.InstallWrappedPWA(medium, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		Source:      "wrap:pwa:" + core.Path(srcDir, "manifest.json"),
		AssetSource: srcDir,
	})
	if err != nil {
		t.Fatalf("InstallWrappedPWA with assets: %v", err)
	}
	if !medium.Exists(core.Path(dest, "manifest.json")) || !medium.Exists(core.Path(dest, "index.html")) {
		t.Fatalf("PWA assets were not copied into %s", dest)
	}
}

// TestPkg_InstallWrappedElectron_Good installs a wrapped Electron app
// and confirms the view.yaml lands in the expected place. Mirrors the
// PWA wrap path so the three install entry points share their
// invariants (one entry per wrap type for clearer CLI errors).
func TestPkg_InstallWrappedElectron_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	srcDir := t.TempDir()
	if err := medium.Write(core.Path(srcDir, "package.json"), `{"name":"electron-wrap"}`); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "main.js"), `console.log("ok")`); err != nil {
		t.Fatalf("Write main.js: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "electron-wrap",
		Name:    "Electron Wrap",
		Version: "0.1.0",
		Config: map[string]any{
			"type": "electron",
		},
	}

	dest, err := app.InstallWrappedElectron(medium, manifest,
		app.PkgInstallOptions{
			Home:        home,
			Source:      "wrap:electron:github.com/foo/bar",
			AssetSource: srcDir,
		})
	if err != nil {
		t.Fatalf("InstallWrappedElectron: %v", err)
	}
	if dest == "" {
		t.Fatal("InstallWrappedElectron returned empty dest")
	}
	if !medium.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Fatalf("view.yaml missing at %s", dest)
	}
	if !medium.Exists(core.Path(dest, "package.json")) || !medium.Exists(core.Path(dest, "main.js")) {
		t.Fatalf("Electron assets were not copied into %s", dest)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "wrap:electron:github.com/foo/bar" {
		t.Errorf("source stamp = %q; want 'wrap:electron:github.com/foo/bar'", src)
	}
	if hash, _ := round.Config["asset_hash"].(string); hash == "" {
		t.Error("asset_hash missing from installed wrapped Electron manifest")
	}
}

// TestPkg_InstallWrappedElectron_ExplicitSign_Good confirms that an
// explicit signing key is applied AFTER the renderer asset hash has been
// bound into the manifest, so the installed artifact still verifies and
// boots in prod mode.
func TestPkg_InstallWrappedElectron_ExplicitSign_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	srcDir := t.TempDir()
	if err := medium.Write(core.Path(srcDir, "package.json"), `{"name":"electron-explicit-sign"}`); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "main.js"), `console.log("signed")`); err != nil {
		t.Fatalf("Write main.js: %v", err)
	}

	keysDir := t.TempDir()
	keyPath, pubPath, err := app.Keygen(medium, keysDir, "explicit-sign")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	pubHex, err := medium.Read(pubPath)
	if err != nil {
		t.Fatalf("Read pub key: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "electron-explicit-sign",
		Name:    "Electron Explicit Sign",
		Version: "0.1.0",
		Config:  map[string]any{"type": "electron"},
	}
	dest, err := app.InstallWrappedElectron(medium, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		AssetSource: srcDir,
		SignKeyPath: keyPath,
	})
	if err != nil {
		t.Fatalf("InstallWrappedElectron explicit sign: %v", err)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadViewManifest: %v", err)
	}
	if round.Sign == "" {
		t.Fatal("Sign field empty after explicit install signing")
	}
	if hash, _ := round.Config["asset_hash"].(string); hash == "" {
		t.Fatal("asset_hash missing after explicit install signing")
	}

	if _, err := app.Boot(context.Background(), dest,
		app.WithPublicKey(core.Trim(pubHex)),
		app.WithWorkspaceHome(home),
	); err != nil {
		t.Fatalf("Boot(prod, explicit-sign Electron wrap): %v", err)
	}
}

// TestPkg_InstallWrappedElectron_AssetsCopied pins the asset-aware
// install path independently of the source stamp checks above.
func TestPkg_InstallWrappedElectron_AssetsCopied(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	medium := coreio.Local

	if err := medium.EnsureDir(core.Path(srcDir, "assets")); err != nil {
		t.Fatalf("EnsureDir assets: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "assets", "renderer.js"), `window.app = true`); err != nil {
		t.Fatalf("Write renderer.js: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "electron-assets",
		Name:    "Electron Assets",
		Version: "0.1.0",
		Config:  map[string]any{"type": "electron"},
	}
	dest, err := app.InstallWrappedElectron(medium, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		AssetSource: srcDir,
	})
	if err != nil {
		t.Fatalf("InstallWrappedElectron with assets: %v", err)
	}
	if !medium.Exists(core.Path(dest, "assets", "renderer.js")) {
		t.Fatalf("renderer asset missing at %s", core.Path(dest, "assets", "renderer.js"))
	}
}

// TestPkg_WriteWrappedAppWithOptions_ExplicitSign_Good pins the
// non-install wrap path used by `pkg wrap --dest/--no-install`: staged
// Electron assets participate in the signed envelope there too.
func TestPkg_WriteWrappedAppWithOptions_ExplicitSign_Good(t *testing.T) {
	medium := coreio.Local
	srcDir := t.TempDir()
	if err := medium.Write(core.Path(srcDir, "package.json"), `{"name":"manual-electron-sign"}`); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "renderer.js"), `window.manual = true`); err != nil {
		t.Fatalf("Write renderer.js: %v", err)
	}

	keysDir := t.TempDir()
	keyPath, pubPath, err := app.Keygen(medium, keysDir, "manual-sign")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	pubHex, err := medium.Read(pubPath)
	if err != nil {
		t.Fatalf("Read pub key: %v", err)
	}

	dest := core.Path(t.TempDir(), "wrapped-manual-electron")
	manifest := &config.ViewManifest{
		Code:    "manual-electron-sign",
		Name:    "Manual Electron Sign",
		Version: "0.1.0",
		Config:  map[string]any{"type": "electron"},
	}
	if err := app.WriteWrappedAppWithOptions(medium, dest, manifest, app.WriteWrappedOptions{
		AssetSource: srcDir,
		SignKeyPath: keyPath,
	}); err != nil {
		t.Fatalf("WriteWrappedAppWithOptions: %v", err)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadViewManifest: %v", err)
	}
	if round.Sign == "" {
		t.Fatal("Sign field empty after manual wrap signing")
	}
	if hash, _ := round.Config["asset_hash"].(string); hash == "" {
		t.Fatal("asset_hash missing after manual wrap signing")
	}

	if _, err := app.Boot(context.Background(), dest,
		app.WithPublicKey(core.Trim(pubHex)),
		app.WithWorkspaceHome(t.TempDir()),
	); err != nil {
		t.Fatalf("Boot(prod, manual explicit-sign Electron wrap): %v", err)
	}
}

// TestPkg_InstallWrappedElectron_ProdBootTamper_Bad confirms the
// installed renderer tree participates in the trusted envelope: once
// an asset changes on disk, prod boot rejects the wrapped Electron app.
func TestPkg_InstallWrappedElectron_ProdBootTamper_Bad(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	srcDir := t.TempDir()
	if err := medium.Write(core.Path(srcDir, "package.json"), `{"name":"electron-tamper"}`); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "main.js"), `console.log("v1")`); err != nil {
		t.Fatalf("Write main.js: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "electron-tamper",
		Name:    "Electron Tamper",
		Version: "0.1.0",
		Config:  map[string]any{"type": "electron"},
	}
	dest, err := app.InstallWrappedElectron(medium, manifest, app.PkgInstallOptions{
		Home:        home,
		AssetSource: srcDir,
	})
	if err != nil {
		t.Fatalf("InstallWrappedElectron: %v", err)
	}

	if _, err := app.Boot(context.Background(), dest,
		app.WithTrustedKeysDir(core.Path(home, ".core", "keys")),
		app.WithWorkspaceHome(home),
	); err != nil {
		t.Fatalf("Boot(prod, pristine Electron wrap): %v", err)
	}

	if err := medium.Write(core.Path(dest, "main.js"), `console.log("tampered")`); err != nil {
		t.Fatalf("tamper main.js: %v", err)
	}

	if _, err := app.Boot(context.Background(), dest,
		app.WithTrustedKeysDir(core.Path(home, ".core", "keys")),
		app.WithWorkspaceHome(home),
	); err == nil {
		t.Fatal("Boot(prod) should fail after Electron asset tampering")
	}
}

// TestPkg_InstallWrappedWeb_AssetsCopied confirms that a wrapped web
// install carries the site files alongside the generated manifest.
func TestPkg_InstallWrappedWeb_AssetsCopied(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>hello</html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "app.css"), "body { color: red; }"); err != nil {
		t.Fatalf("Write app.css: %v", err)
	}

	manifest := &config.ViewManifest{
		Code:    "web-assets",
		Name:    "Web Assets",
		Version: "0.1.0",
		Config:  map[string]any{"type": "web", "entry": "index.html"},
	}
	dest, err := app.InstallWrappedWeb(medium, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		AssetSource: srcDir,
	})
	if err != nil {
		t.Fatalf("InstallWrappedWeb with assets: %v", err)
	}
	if !medium.Exists(core.Path(dest, "index.html")) || !medium.Exists(core.Path(dest, "app.css")) {
		t.Fatalf("web assets were not copied into %s", dest)
	}
}

// TestPkg_InstallWrappedElectron_Bad rejects a nil manifest and a
// manifest without a code — same invariants the PWA path checks so
// the package-type-specific entry points share their guarantees.
func TestPkg_InstallWrappedElectron_Bad(t *testing.T) {
	if _, err := app.InstallWrappedElectron(coreio.Local, nil,
		app.PkgInstallOptions{Home: "/tmp"}); err == nil {
		t.Error("nil manifest produced no error")
	}
	if _, err := app.InstallWrappedElectron(coreio.Local,
		&config.ViewManifest{Name: "no-code"},
		app.PkgInstallOptions{Home: t.TempDir()}); err == nil {
		t.Error("empty code produced no error")
	}
}

// TestPkg_InstallWrappedElectron_Ugly covers the Force=true replace
// flow — re-installing without Force errors, with Force overwrites.
func TestPkg_InstallWrappedElectron_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	manifest := &config.ViewManifest{
		Code:    "elec-dup",
		Name:    "Original",
		Version: "0.1.0",
	}
	if _, err := app.InstallWrappedElectron(medium, manifest,
		app.PkgInstallOptions{Home: home}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	manifest.Name = "Replaced"
	if _, err := app.InstallWrappedElectron(medium, manifest,
		app.PkgInstallOptions{Home: home}); err == nil {
		t.Error("duplicate install without Force produced no error")
	}
	if _, err := app.InstallWrappedElectron(medium, manifest,
		app.PkgInstallOptions{Home: home, Force: true}); err != nil {
		t.Fatalf("force install: %v", err)
	}
	var round config.ViewManifest
	path := core.Path(home, ".core", app.AppsDirName, "elec-dup", ".core", "view.yaml")
	if err := app.LoadViewManifest(medium, path, &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Name != "Replaced" {
		t.Errorf("force-replaced Name = %q; want 'Replaced'", round.Name)
	}
}

// TestPkg_PkgUpdate_Good updates an installed package that has a
// recorded source and confirms the function succeeds (actual refetch
// is CLI-driven).
func TestPkg_PkgUpdate_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	writeInstalled(t, medium, home, "u", &config.ViewManifest{
		Code:    "u",
		Name:    "U",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "marketplace:u",
		},
	})
	path, err := app.PkgUpdate(context.Background(), medium, home, "u")
	if err != nil {
		t.Fatalf("PkgUpdate: %v", err)
	}
	if path == "" {
		t.Error("PkgUpdate returned empty path")
	}
}

// TestPkg_PkgUpdate_Bad rejects empty inputs and a package without a
// recorded source.
func TestPkg_PkgUpdate_Bad(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	ctx := context.Background()
	if _, err := app.PkgUpdate(ctx, medium, "", "x"); err == nil {
		t.Error("empty home produced no error")
	}
	if _, err := app.PkgUpdate(ctx, medium, home, ""); err == nil {
		t.Error("empty name produced no error")
	}
	if _, err := app.PkgUpdate(ctx, medium, home, "missing"); err == nil {
		t.Error("missing package produced no error")
	}
	// Installed with no recorded source — PkgUpdate should refuse.
	writeInstalled(t, medium, home, "no-source", &config.ViewManifest{
		Code:    "no-source",
		Name:    "X",
		Version: "0.1.0",
	})
	if _, err := app.PkgUpdate(ctx, medium, home, "no-source"); err == nil {
		t.Error("no-source package produced no error")
	}
}

// TestPkg_PkgUpdate_Ugly verifies a wrapped PWA install whose source is
// `wrap:pwa:<url>` triggers a fresh fetch + rewrap when the upstream
// manifest changes.
func TestPkg_PkgUpdate_Ugly(t *testing.T) {
	current := `{"name":"V1","short_name":"v","start_url":"/"}`
	srv := newPWAManifestServer(t, &current)
	defer srv.Close()

	home := t.TempDir()
	medium := coreio.Local
	source := "wrap:pwa:" + srv.URL + "/manifest.json"

	// Plant an installed PWA that recorded its source.
	writeInstalled(t, medium, home, "v", &config.ViewManifest{
		Code:    "v",
		Name:    "V0",
		Version: "0.1.0",
		Config: map[string]any{
			"source": source,
		},
	})

	// Upstream changed → bump the served name.
	current = `{"name":"V2","short_name":"v","start_url":"/"}`

	if _, err := app.PkgUpdate(context.Background(), medium, home, "v"); err != nil {
		t.Fatalf("PkgUpdate (PWA refresh): %v", err)
	}

	var round config.ViewManifest
	path := core.Path(home, ".core", app.AppsDirName, "v", ".core", "view.yaml")
	if err := app.LoadViewManifest(medium, path, &round); err != nil {
		t.Fatalf("reload after update: %v", err)
	}
	if round.Name != "V2" {
		t.Errorf("after PkgUpdate Name = %q; want V2", round.Name)
	}
}

// TestPkg_PkgUpdate_LocalPWA verifies that a wrapped local PWA manifest
// path (not an HTTP URL) can be refreshed in place.
func TestPkg_PkgUpdate_LocalPWA(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	srcDir := t.TempDir()
	manifestPath := core.Path(srcDir, "manifest.json")

	if err := medium.Write(manifestPath, `{"name":"Local V1","short_name":"local-v","start_url":"/"}`); err != nil {
		t.Fatalf("Write manifest.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>v1</html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}
	writeInstalled(t, medium, home, "local-v", &config.ViewManifest{
		Code:    "local-v",
		Name:    "Local V0",
		Version: "0.1.0",
		Config: map[string]any{
			"source": "wrap:pwa:" + manifestPath,
		},
	})

	if err := medium.Write(manifestPath, `{"name":"Local V2","short_name":"local-v","start_url":"/next"}`); err != nil {
		t.Fatalf("Rewrite manifest.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html>v2</html>"); err != nil {
		t.Fatalf("Rewrite index.html: %v", err)
	}
	if _, err := app.PkgUpdate(context.Background(), medium, home, "local-v"); err != nil {
		t.Fatalf("PkgUpdate (local PWA): %v", err)
	}

	var round config.ViewManifest
	view := core.Path(home, ".core", app.AppsDirName, "local-v", ".core", "view.yaml")
	if err := app.LoadViewManifest(medium, view, &round); err != nil {
		t.Fatalf("reload local PWA after update: %v", err)
	}
	if round.Name != "Local V2" {
		t.Errorf("after local PkgUpdate Name = %q; want Local V2", round.Name)
	}
	if !medium.Exists(core.Path(home, ".core", app.AppsDirName, "local-v", "index.html")) {
		t.Fatal("PkgUpdate (local PWA) did not copy the asset tree into the install")
	}
}

// TestPkg_ParseInstallSpec_Good covers each detection branch: HTTPS
// URL → PWA, github.com/owner/repo → Electron, file:// or relative
// path → local install (Type unknown until DetectPackageType sees the
// directory), plain code → marketplace native lookup.
func TestPkg_ParseInstallSpec_Good(t *testing.T) {
	cases := []struct {
		in       string
		wantType app.PackageType
		wantURL  string
		wantRepo string
		wantCode string
		wantPath string
	}{
		{"https://app.example.com", app.PackageTypePWA, "https://app.example.com", "", "", ""},
		{"http://localhost:8080/app", app.PackageTypePWA, "http://localhost:8080/app", "", "", ""},
		{"github.com/owner/repo", app.PackageTypeElectron, "", "github.com/owner/repo", "", ""},
		{"gitlab.com/owner/repo", app.PackageTypeElectron, "", "gitlab.com/owner/repo", "", ""},
		{"plain-code", app.PackageTypeNative, "", "", "plain-code", ""},
		{"core/photo-browser", app.PackageTypeNative, "", "", "core/photo-browser", ""},
		{"./my-app", app.PackageTypeUnknown, "", "", "", "./my-app"},
		{"../sibling-app", app.PackageTypeUnknown, "", "", "", "../sibling-app"},
		{"/srv/app", app.PackageTypeUnknown, "", "", "", "/srv/app"},
		{"file:///srv/app", app.PackageTypeUnknown, "", "", "", "/srv/app"},
	}
	for _, tc := range cases {
		spec := app.ParseInstallSpec(tc.in)
		if spec.Type != tc.wantType {
			t.Errorf("ParseInstallSpec(%q).Type = %v; want %v", tc.in, spec.Type, tc.wantType)
		}
		if spec.URL != tc.wantURL {
			t.Errorf("ParseInstallSpec(%q).URL = %q; want %q", tc.in, spec.URL, tc.wantURL)
		}
		if spec.Repo != tc.wantRepo {
			t.Errorf("ParseInstallSpec(%q).Repo = %q; want %q", tc.in, spec.Repo, tc.wantRepo)
		}
		if spec.Code != tc.wantCode {
			t.Errorf("ParseInstallSpec(%q).Code = %q; want %q", tc.in, spec.Code, tc.wantCode)
		}
		if spec.Path != tc.wantPath {
			t.Errorf("ParseInstallSpec(%q).Path = %q; want %q", tc.in, spec.Path, tc.wantPath)
		}
	}
}

// TestPkg_ParseInstallSpec_Bad — empty input returns an unknown spec.
func TestPkg_ParseInstallSpec_Bad(t *testing.T) {
	spec := app.ParseInstallSpec("")
	if spec.Type != app.PackageTypeUnknown {
		t.Errorf("empty input Type = %v; want PackageTypeUnknown", spec.Type)
	}
	spec = app.ParseInstallSpec("   ")
	if spec.Type != app.PackageTypeUnknown {
		t.Errorf("whitespace input Type = %v; want PackageTypeUnknown", spec.Type)
	}
}

// TestPkg_ParseInstallSpec_Ugly handles git@ remote-style references and
// odd whitespace around otherwise valid input.
func TestPkg_ParseInstallSpec_Ugly(t *testing.T) {
	spec := app.ParseInstallSpec("git@github.com:owner/repo.git")
	if spec.Type != app.PackageTypeElectron {
		t.Errorf("git@ ref Type = %v; want PackageTypeElectron", spec.Type)
	}
	spec = app.ParseInstallSpec("  https://x.example.com/  ")
	if spec.Type != app.PackageTypePWA {
		t.Errorf("trim-then-detect Type = %v; want PackageTypePWA", spec.Type)
	}
	if spec.URL != "https://x.example.com/" {
		t.Errorf("trim-then-detect URL = %q; want trimmed URL", spec.URL)
	}
}

// TestPkg_DisplaySource_Good — every documented prefix is stripped
// to the human-friendly form per RFC §16.3 (`pkg list` example).
func TestPkg_DisplaySource_Good(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"wrap:pwa:https://play.example.com", "https://play.example.com"},
		{"wrap:electron:github.com/foo/bar", "github.com/foo/bar"},
		{"wrap:web:./my-webapp", "./my-webapp"},
		{"marketplace:photo-browser", "marketplace"},
	}
	for _, tc := range cases {
		got := app.PkgEntry{Source: tc.raw}.DisplaySource()
		if got != tc.want {
			t.Errorf("DisplaySource(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
}

// TestPkg_DisplaySource_Bad — undocumented prefixes pass through
// untouched so an operator can spot a stale install rather than
// having the value silently rewritten.
func TestPkg_DisplaySource_Bad(t *testing.T) {
	got := app.PkgEntry{Source: "weird:custom:value"}.DisplaySource()
	if got != "weird:custom:value" {
		t.Errorf("DisplaySource(weird) = %q; want pass-through", got)
	}
}

// TestPkg_DisplaySource_Ugly — empty Source and the legacy `local:`
// stamp both render as "local" so `pkg list` always shows a value.
func TestPkg_DisplaySource_Ugly(t *testing.T) {
	if got := (app.PkgEntry{}).DisplaySource(); got != "local" {
		t.Errorf("DisplaySource(empty) = %q; want local", got)
	}
	if got := (app.PkgEntry{Source: "local:/srv/app"}).DisplaySource(); got != "local" {
		t.Errorf("DisplaySource(local:) = %q; want local", got)
	}
}

// writeInstalled is a helper that plants a view.yaml under
// `<home>/.core/apps/<code>/.core/view.yaml` so PkgList can find it.
//
//	writeInstalled(t, medium, home, "code", manifest)
func writeInstalled(t *testing.T, medium coreio.Medium, home, code string, manifest *config.ViewManifest) {
	t.Helper()
	path := core.Path(home, ".core", app.AppsDirName, code, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := medium.Write(path, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// newPWAManifestServer spins up an httptest server that always serves
// the contents of the supplied pointer as `application/manifest+json`.
// The pointer indirection lets tests mutate the served body between
// requests (e.g. to test PkgUpdate's re-fetch behaviour).
//
//	body := `{"name":"V1"}`
//	srv := newPWAManifestServer(t, &body)
//	defer srv.Close()
//	body = `{"name":"V2"}` // next fetch sees V2
func newPWAManifestServer(t *testing.T, body *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write([]byte(*body))
	}))
}

// TestPkg_InstalledApps_Good writes two installed plugin trees and
// asserts that InstalledApps returns both fully-parsed manifests in
// lexicographic code order.
func TestPkg_InstalledApps_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	for _, entry := range []struct {
		code    string
		modules []string
	}{
		{"zebra", []string{"core/media"}},
		{"alpha", []string{"core/fs", "core/media"}},
	} {
		m := &config.ViewManifest{
			Code:    entry.code,
			Name:    entry.code,
			Version: "0.1.0",
			Modules: entry.modules,
			Permissions: config.ViewPermissions{
				Read: []string{"./data/"},
			},
		}
		body, err := yaml.Marshal(m)
		if err != nil {
			t.Fatalf("yaml.Marshal: %v", err)
		}
		dest := core.Path(home, ".core", "apps", entry.code, ".core", "view.yaml")
		if err := medium.EnsureDir(core.PathDir(dest)); err != nil {
			t.Fatalf("EnsureDir: %v", err)
		}
		if err := medium.Write(dest, string(body)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	apps, err := app.InstalledApps(medium, home)
	if err != nil {
		t.Fatalf("InstalledApps: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
	if apps[0].Manifest.Code != "alpha" {
		t.Errorf("apps[0].Code = %q; want 'alpha' (sorted ordering)", apps[0].Manifest.Code)
	}
	if apps[1].Manifest.Code != "zebra" {
		t.Errorf("apps[1].Code = %q; want 'zebra'", apps[1].Manifest.Code)
	}
	if len(apps[0].Manifest.Modules) != 2 {
		t.Errorf("apps[0].Modules = %v; wanted 2 entries from the manifest", apps[0].Manifest.Modules)
	}
	if apps[0].Path == "" {
		t.Error("apps[0].Path should carry the absolute install dir")
	}
}

// TestPkg_InstalledApps_Bad — an empty home directory is rejected
// up-front so callers don't get an ambiguous empty slice for a bug
// they can't tell apart from "no apps installed".
func TestPkg_InstalledApps_Bad(t *testing.T) {
	if _, err := app.InstalledApps(coreio.Local, ""); err == nil {
		t.Error("empty home should return a typed error")
	}
}

// TestPkg_InstalledApps_Ugly — a missing apps directory returns nil
// slice + nil error so a fresh user rendering a plugin drawer does
// not need to branch on a synthetic "no installs" error.
func TestPkg_InstalledApps_Ugly(t *testing.T) {
	apps, err := app.InstalledApps(coreio.Local, t.TempDir())
	if err != nil {
		t.Fatalf("InstalledApps: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected empty slice; got %d entries", len(apps))
	}
}

// TestPkg_PkgList_SortOrder_Good — entries come back in lexicographic
// Name order regardless of the order medium.List returns them in.
// Deterministic order matters for CLI tables and JSON consumers that
// diff successive runs.
func TestPkg_PkgList_SortOrder_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	// Deliberately plant in non-alphabetical order so the test catches a
	// regression that returns the filesystem's enumeration order.
	for _, code := range []string{"zebra", "alpha", "mango", "beta"} {
		writeInstalled(t, medium, home, code, &config.ViewManifest{
			Code: code, Name: code, Version: "0.1.0",
		})
	}
	entries, err := app.PkgList(medium, home)
	if err != nil {
		t.Fatalf("PkgList: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d; want 4", len(entries))
	}
	want := []string{"alpha", "beta", "mango", "zebra"}
	for i, e := range entries {
		if e.Name != want[i] {
			t.Errorf("entries[%d].Name = %q; want %q", i, e.Name, want[i])
		}
	}
}

// TestPkg_PkgInfo_Good writes an installed package, provisions a
// workspace, and confirms PkgInfo returns the summary, manifest,
// permissions and the workspace path.
func TestPkg_PkgInfo_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	manifest := &config.ViewManifest{
		Code: "viewer", Name: "Viewer", Version: "1.2.3",
		Layout:  "HLCRF",
		Modules: []string{"core/media"},
		Permissions: config.ViewPermissions{
			Read:          []string{"./photos/"},
			Net:           []string{"api.example.com:443"},
			Run:           []string{"ffmpeg", "device.location"},
			Notifications: true,
			Clipboard:     true,
			Camera:        true,
		},
		Config: map[string]any{
			"type":   "native",
			"source": "marketplace:viewer",
			"store":  true,
			"write":  []any{"./photos/.thumbs/"},
		},
	}
	writeInstalled(t, medium, home, "viewer", manifest)
	if _, err := app.OpenWorkspace(medium, home, "viewer"); err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}

	info, err := app.PkgInfo(medium, home, "viewer")
	if err != nil {
		t.Fatalf("PkgInfo: %v", err)
	}
	if info.Entry.Name != "viewer" || info.Entry.Version != "1.2.3" {
		t.Errorf("summary row mismatch: %+v", info.Entry)
	}
	if info.Entry.Type != app.PackageTypeNative {
		t.Errorf("summary type = %v; want native", info.Entry.Type)
	}
	if info.Entry.Source != "marketplace:viewer" {
		t.Errorf("summary source = %q; want 'marketplace:viewer'", info.Entry.Source)
	}
	if info.Manifest.Layout != "HLCRF" {
		t.Errorf("manifest.Layout = %q; want HLCRF", info.Manifest.Layout)
	}
	if info.Workspace == "" {
		t.Error("Workspace is empty — OpenWorkspace should have populated it")
	}

	// The flattened permission summary must include at least one entry
	// per declared capability. The assertion is existence-based so the
	// ordering rule inside ManifestPermissionSummary can evolve without
	// breaking this case.
	want := []string{
		"read: ./photos/",
		"write: ./photos/.thumbs/",
		"net: api.example.com:443",
		"run: ffmpeg",
		"store",
		"notifications",
		"clipboard",
		"camera",
		"location",
	}
	seen := map[string]bool{}
	for _, p := range info.Permissions {
		seen[p] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("permission summary missing %q; got %v", w, info.Permissions)
		}
	}
}

// TestPkg_PkgInfo_Bad rejects empty home, empty name, and
// not-installed packages with typed errors.
func TestPkg_PkgInfo_Bad(t *testing.T) {
	if _, err := app.PkgInfo(coreio.Local, "", "x"); err == nil {
		t.Error("empty home produced no error")
	}
	if _, err := app.PkgInfo(coreio.Local, t.TempDir(), ""); err == nil {
		t.Error("empty name produced no error")
	}
	if _, err := app.PkgInfo(coreio.Local, t.TempDir(), "missing"); err == nil {
		t.Error("missing package produced no error")
	}
}

// TestPkg_PkgInfo_Ugly — a package with no workspace on disk returns
// info.Workspace="" so the caller can distinguish "never booted" from
// "provisioned". The rest of the projection is still populated.
func TestPkg_PkgInfo_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	writeInstalled(t, medium, home, "fresh", &config.ViewManifest{
		Code: "fresh", Name: "Fresh", Version: "0.1.0",
	})

	info, err := app.PkgInfo(medium, home, "fresh")
	if err != nil {
		t.Fatalf("PkgInfo: %v", err)
	}
	if info.Workspace != "" {
		t.Errorf("Workspace = %q; want empty for never-booted install", info.Workspace)
	}
	if info.Entry.Name != "fresh" {
		t.Errorf("Entry.Name = %q; want 'fresh'", info.Entry.Name)
	}
}

// TestPkg_ManifestPermissionSummary_Good exercises the typed-field
// and config-backed branches in one call — covers read / write / net /
// run / store / notifications / clipboard / camera / microphone /
// location plus the catch-all bools (filesystem, network).
func TestPkg_ManifestPermissionSummary_Good(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read:          []string{"./a/", "./b/"},
			Net:           []string{"host:443"},
			Run:           []string{"bin", "device.location"},
			Filesystem:    true,
			Network:       true,
			Notifications: true,
			Clipboard:     true,
			Camera:        true,
			Microphone:    true,
		},
		Config: map[string]any{
			"store": true,
			"write": []any{"./c/"},
		},
	}
	out := app.ManifestPermissionSummary(m)
	expected := []string{
		"read: ./a/",
		"read: ./b/",
		"write: ./c/",
		"net: host:443",
		"run: bin",
		"filesystem",
		"network",
		"store",
		"notifications",
		"clipboard",
		"camera",
		"microphone",
		"location",
	}
	seen := map[string]bool{}
	for _, p := range out {
		seen[p] = true
	}
	for _, e := range expected {
		if !seen[e] {
			t.Errorf("missing %q from summary; got %v", e, out)
		}
	}
	// `device.location` must not surface under the raw `run: …` list
	// because the summary renders it under its semantic name.
	for _, p := range out {
		if p == "run: device.location" {
			t.Error("device.location should be rendered as 'location', not 'run: device.location'")
		}
	}
}

// TestPkg_ManifestPermissionSummary_Bad — a nil manifest returns nil
// rather than panicking. Zero-value manifests surface as an empty
// slice because nothing is declared.
func TestPkg_ManifestPermissionSummary_Bad(t *testing.T) {
	if out := app.ManifestPermissionSummary(nil); out != nil {
		t.Errorf("nil manifest produced %v; want nil", out)
	}
	if out := app.ManifestPermissionSummary(&config.ViewManifest{}); len(out) != 0 {
		t.Errorf("zero manifest produced %v; want empty", out)
	}
}

// TestPkg_ManifestPermissionSummary_Ugly — empty strings inside
// permission lists are skipped, and a malformed Config["write"] shape
// is ignored rather than crashing the summary.
func TestPkg_ManifestPermissionSummary_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"", "./a/"},
			Net:  []string{""},
			Run:  []string{"", "bin"},
		},
		Config: map[string]any{
			"write": "not-a-list", // wrong shape — must be silently ignored
		},
	}
	out := app.ManifestPermissionSummary(m)
	for _, p := range out {
		switch p {
		case "read: ", "net: ", "run: ":
			t.Errorf("empty permission entry leaked into summary: %q", p)
		}
	}
	// Positive checks — valid entries still surface.
	seen := map[string]bool{}
	for _, p := range out {
		seen[p] = true
	}
	if !seen["read: ./a/"] {
		t.Errorf("read: ./a/ missing; got %v", out)
	}
	if !seen["run: bin"] {
		t.Errorf("run: bin missing; got %v", out)
	}
}
