// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
	"gopkg.in/yaml.v3"
)

// TestPkgLocal_PkgInstallLocal_Good — copying a CoreApp source tree
// into the install root produces a usable package and stamps the
// source path so `pkg list` can show provenance.
func TestPkgLocal_PkgInstallLocal_Good(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	// Plant a manifest at the source.
	manifest := config.ViewManifest{
		Code:    "local-good",
		Name:    "Local Good",
		Version: "0.1.0",
	}
	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(src, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}

	// Drop a sibling asset so we know the recursive copy works.
	assetPath := core.Path(src, "data", "hello.txt")
	if err := medium.EnsureDir(core.PathDir(assetPath)); err != nil {
		t.Fatalf("EnsureDir asset: %v", err)
	}
	if err := medium.Write(assetPath, "world"); err != nil {
		t.Fatalf("Write asset: %v", err)
	}

	dest, err := app.PkgInstallLocal(medium, src, app.PkgInstallOptions{Home: home})
	if err != nil {
		t.Fatalf("PkgInstallLocal: %v", err)
	}
	wantDest := core.Path(home, ".core", app.AppsDirName, "local-good")
	if dest != wantDest {
		t.Errorf("dest = %q; want %q", dest, wantDest)
	}

	// Manifest copied across.
	installedView := core.Path(dest, ".core", "view.yaml")
	if !medium.Exists(installedView) {
		t.Errorf("installed view.yaml missing at %q", installedView)
	}
	// Sibling asset copied across.
	installedAsset := core.Path(dest, "data", "hello.txt")
	got, err := medium.Read(installedAsset)
	if err != nil {
		t.Fatalf("read installed asset: %v", err)
	}
	if got != "world" {
		t.Errorf("asset body = %q; want %q", got, "world")
	}

	// Source stamp recorded in Config.
	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, installedView, &round); err != nil {
		t.Fatalf("reload installed manifest: %v", err)
	}
	src2, _ := round.Config["source"].(string)
	if src2 != "local:"+src {
		t.Errorf("source stamp = %q; want %q", src2, "local:"+src)
	}
}

// TestPkgLocal_PkgInstallLocal_Bad — invalid input shapes (empty src,
// missing manifest, existing install without Force) all error.
func TestPkgLocal_PkgInstallLocal_Bad(t *testing.T) {
	medium := coreio.Local

	if _, err := app.PkgInstallLocal(medium, "", app.PkgInstallOptions{Home: t.TempDir()}); err == nil {
		t.Error("empty src should error")
	}

	emptyDir := t.TempDir()
	if _, err := app.PkgInstallLocal(medium, emptyDir, app.PkgInstallOptions{Home: t.TempDir()}); err == nil {
		t.Error("source without view.yaml should error")
	}

	// Plant a manifest, install once, then re-install without Force.
	src := t.TempDir()
	home := t.TempDir()
	manifest := config.ViewManifest{Code: "dup", Version: "0.1.0"}
	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(src, ".core", "view.yaml")
	_ = medium.EnsureDir(core.PathDir(viewPath))
	_ = medium.Write(viewPath, string(body))

	if _, err := app.PkgInstallLocal(medium, src, app.PkgInstallOptions{Home: home}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := app.PkgInstallLocal(medium, src, app.PkgInstallOptions{Home: home}); err == nil {
		t.Error("second install without Force should error")
	}
}

// TestPkgLocal_PkgInstallLocal_Ugly — Force=true overwrites an existing
// install, and an empty Source string defaults to a useful "local:..."
// stamp.
func TestPkgLocal_PkgInstallLocal_Ugly(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	// Plant a manifest.
	manifest := config.ViewManifest{
		Code:    "force",
		Version: "0.1.0",
	}
	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(src, ".core", "view.yaml")
	_ = medium.EnsureDir(core.PathDir(viewPath))
	_ = medium.Write(viewPath, string(body))

	// First install.
	dest, err := app.PkgInstallLocal(medium, src, app.PkgInstallOptions{Home: home})
	if err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Mutate the source then re-install with Force.
	manifest.Name = "Force Updated"
	body, _ = yaml.Marshal(&manifest)
	_ = medium.Write(viewPath, string(body))
	if _, err := app.PkgInstallLocal(medium, src, app.PkgInstallOptions{
		Home:  home,
		Force: true,
	}); err != nil {
		t.Fatalf("force install: %v", err)
	}

	// Verify the destination picked up the mutated body.
	var round config.ViewManifest
	installedView := core.Path(dest, ".core", "view.yaml")
	if err := app.LoadViewManifest(medium, installedView, &round); err != nil {
		t.Fatalf("reload after force: %v", err)
	}
	if round.Name != "Force Updated" {
		t.Errorf("Name after force = %q; want Force Updated", round.Name)
	}
}
