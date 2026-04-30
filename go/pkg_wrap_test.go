// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	coreio "dappco.re/go/io"
)

func TestPkgWrap_PWA_Good(t *testing.T) {
	srcDir := core.Path(t.TempDir(), "pwa-site")
	dest := t.TempDir()
	medium := coreio.Local

	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html><body>PWA</body></html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	manifest := app.WrapPWA(&app.PWAManifest{
		Name:        "Wrapped Play",
		ShortName:   "wrapped-play",
		StartURL:    "https://play.example.com/app",
		Permissions: []string{"notifications", "clipboard-write", "camera"},
	}, app.WrapPWAOptions{TargetURL: "https://play.example.com/app"})
	if manifest == nil {
		t.Fatal("WrapPWA returned nil")
	}

	if err := app.WriteWrappedAppWithOptions(medium, dest, manifest, app.WriteWrappedOptions{
		AssetSource: srcDir,
	}); err != nil {
		t.Fatalf("WriteWrappedAppWithOptions: %v", err)
	}

	body, err := medium.Read(core.Path(dest, ".core", "view.yaml"))
	if err != nil {
		t.Fatalf("Read view.yaml: %v", err)
	}
	for _, want := range []string{
		"type: pwa",
		"store: true",
		"gui.notification.send: true",
		"gui.clipboard.write: true",
		"device.camera: true",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("wrapped PWA YAML missing %q:\n%s", want, body)
		}
	}
	for _, path := range []string{
		core.Path(dest, "core-sw.js"),
		core.Path(dest, "core-pwa.js"),
	} {
		if !medium.Exists(path) {
			t.Errorf("PWA runtime asset missing: %s", path)
		}
	}
}

func TestPkgWrap_PWA_Bad(t *testing.T) {
	if app.WrapPWA(nil, app.WrapPWAOptions{}) != nil {
		t.Error("WrapPWA(nil) returned non-nil")
	}
	if err := app.WritePWAWrap(coreio.Local, t.TempDir(), nil); err == nil {
		t.Error("WritePWAWrap(nil) returned no error")
	}
}

func TestPkgWrap_PWA_Ugly(t *testing.T) {
	srcDir := core.Path(t.TempDir(), "local-pwa")
	if err := coreio.Local.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}

	pwa := &app.PWAManifest{
		Name:     "Local Wrapped Play",
		StartURL: "/spa/index.html",
	}
	resolved := app.ResolvePWAAppURL(core.Path(srcDir, "manifest.webmanifest"), pwa)
	if !strings.HasSuffix(resolved, "/local-pwa/spa/index.html") {
		t.Fatalf("ResolvePWAAppURL = %q; want suffix /local-pwa/spa/index.html", resolved)
	}

	manifest := app.WrapPWA(pwa, app.WrapPWAOptions{TargetURL: resolved})
	if manifest == nil {
		t.Fatal("WrapPWA returned nil")
	}
	if len(manifest.Permissions.Net) != 0 {
		t.Errorf("local wrapped PWA should not declare network access; got %v", manifest.Permissions.Net)
	}
}

func TestPkgWrap_Electron_Good(t *testing.T) {
	srcDir := core.Path(t.TempDir(), "electron-app")
	dest := t.TempDir()
	medium := coreio.Local

	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "package.json"), `{"name":"electron-wrap","main":"main.js","dependencies":{"electron":"^30.0.0"}}`); err != nil {
		t.Fatalf("Write package.json: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "main.js"), `console.log("electron")`); err != nil {
		t.Fatalf("Write main.js: %v", err)
	}

	manifest := app.WrapElectron(&app.ElectronPackageJSON{
		Name:        "electron-wrap",
		ProductName: "Electron Wrap",
		Version:     "1.2.3",
		Main:        "main.js",
		Dependencies: map[string]string{
			"electron": "^30.0.0",
		},
	}, &app.ElectronScanResult{
		FS:            true,
		ClipboardRead: true,
		ShellOpen:     true,
		IPCChannels:   []string{"app:ready"},
	}, app.WrapElectronOptions{})
	if manifest == nil {
		t.Fatal("WrapElectron returned nil")
	}

	if err := app.WriteWrappedAppWithOptions(medium, dest, manifest, app.WriteWrappedOptions{
		AssetSource: srcDir,
	}); err != nil {
		t.Fatalf("WriteWrappedAppWithOptions: %v", err)
	}

	body, err := medium.Read(core.Path(dest, ".core", "view.yaml"))
	if err != nil {
		t.Fatalf("Read view.yaml: %v", err)
	}
	for _, want := range []string{
		"type: electron",
		"main: main.js",
		"gui.clipboard.read: true",
		"gui.browser.open: true",
		"ipc_channels:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("wrapped Electron YAML missing %q:\n%s", want, body)
		}
	}
}

func TestPkgWrap_Electron_Bad(t *testing.T) {
	if app.WrapElectron(nil, nil, app.WrapElectronOptions{}) != nil {
		t.Error("WrapElectron(nil, nil) returned non-nil")
	}
	if err := app.WriteElectronWrap(coreio.Local, t.TempDir(), nil); err == nil {
		t.Error("WriteElectronWrap(nil) returned no error")
	}
}

func TestPkgWrap_Electron_Ugly(t *testing.T) {
	manifest := app.WrapElectron(&app.ElectronPackageJSON{
		ProductName: "Electron Metadata",
		Version:     "4.5.6",
		Main:        "src/main.mjs",
		Dependencies: map[string]string{
			"electron": "^31.0.0",
		},
	}, nil, app.WrapElectronOptions{})
	if manifest == nil {
		t.Fatal("WrapElectron returned nil")
	}
	if manifest.Name != "Electron Metadata" {
		t.Errorf("Name = %q; want package metadata name", manifest.Name)
	}
	if manifest.Version != "4.5.6" {
		t.Errorf("Version = %q; want package metadata version", manifest.Version)
	}
	if got := manifest.Config["main"]; got != "src/main.mjs" {
		t.Errorf("Config[main] = %v; want src/main.mjs", got)
	}
	if len(manifest.Permissions.Read) != 0 || len(manifest.Permissions.Net) != 0 {
		t.Errorf("metadata-only wrap should not infer permissions without a scan: %+v", manifest.Permissions)
	}
}

func TestPkgWrap_Web_Good(t *testing.T) {
	srcRoot := t.TempDir()
	srcDir := core.Path(srcRoot, "marketing-site")
	dest := t.TempDir()
	medium := coreio.Local

	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "index.html"), "<html><body>Landing</body></html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	manifest, err := app.WrapWeb(medium, srcDir, app.WrapWebOptions{})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}
	if manifest.Code != "marketing-site" {
		t.Errorf("Code = %q; want marketing-site", manifest.Code)
	}
	if manifest.Name != "Marketing Site" {
		t.Errorf("Name = %q; want Marketing Site", manifest.Name)
	}

	if err := app.WriteWrappedAppWithOptions(medium, dest, manifest, app.WriteWrappedOptions{
		AssetSource: srcDir,
	}); err != nil {
		t.Fatalf("WriteWrappedAppWithOptions: %v", err)
	}

	body, err := medium.Read(core.Path(dest, ".core", "view.yaml"))
	if err != nil {
		t.Fatalf("Read view.yaml: %v", err)
	}
	for _, want := range []string{
		"type: web",
		"entry: index.html",
		"read:",
		"- ./",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("wrapped web YAML missing %q:\n%s", want, body)
		}
	}
	if !medium.Exists(core.Path(dest, "index.html")) {
		t.Fatalf("wrapped web assets missing index.html")
	}
}

func TestPkgWrap_Web_Bad(t *testing.T) {
	if _, err := app.WrapWeb(coreio.Local, "", app.WrapWebOptions{}); err == nil {
		t.Error("WrapWeb(\"\") returned no error")
	}
	dir := t.TempDir()
	if _, err := app.WrapWeb(coreio.Local, dir, app.WrapWebOptions{}); err == nil {
		t.Error("WrapWeb without index.html returned no error")
	}
}

func TestPkgWrap_Web_Ugly(t *testing.T) {
	srcRoot := t.TempDir()
	srcDir := core.Path(srcRoot, "docs-site")
	medium := coreio.Local

	if err := medium.EnsureDir(srcDir); err != nil {
		t.Fatalf("EnsureDir src: %v", err)
	}
	if err := medium.Write(core.Path(srcDir, "home.html"), "<html><body>Docs</body></html>"); err != nil {
		t.Fatalf("Write home.html: %v", err)
	}

	manifest, err := app.WrapWeb(medium, srcDir, app.WrapWebOptions{Entry: "home.html"})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}
	if manifest.Name != "Docs Site" {
		t.Errorf("Name = %q; want Docs Site", manifest.Name)
	}
	if got := manifest.Config["entry"]; got != "home.html" {
		t.Errorf("Config[entry] = %v; want home.html", got)
	}
}
