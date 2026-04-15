// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
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
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:play" {
		t.Errorf("source stamp = %q; want 'marketplace:play'", src)
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
	if err := config.LoadManifest(medium, path, &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Name != "Second" {
		t.Errorf("force-replaced Name = %q; want 'Second'", round.Name)
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
	if err := config.LoadManifest(medium, path, &round); err != nil {
		t.Fatalf("reload after update: %v", err)
	}
	if round.Name != "V2" {
		t.Errorf("after PkgUpdate Name = %q; want V2", round.Name)
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
