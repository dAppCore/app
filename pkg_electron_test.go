// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
)

// TestPkgElectron_ScanElectronRenderer_Good plants a tiny renderer
// tree that uses every Electron API pattern we detect and asserts the
// scan result flips the expected booleans.
func TestPkgElectron_ScanElectronRenderer_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	if err := medium.EnsureDir(core.Path(dir, "src")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	files := map[string]string{
		"main.js": `
			const fs = require('fs');
			const net = require('net');
			const { clipboard, dialog, shell } = require('electron');
			clipboard.readText();
			clipboard.writeText("hi");
			dialog.showOpenDialog({});
			dialog.showSaveDialog({});
			shell.openExternal("https://example.com");
			new Notification("hi");
		`,
		"preload.js": `
			const { ipcRenderer } = require('electron');
			ipcRenderer.send('miner:start', 1);
			ipcRenderer.invoke('miner:stop');
		`,
		"src/panel.js": `
			import fs from 'node:fs';
			fetch('https://api.example.com/');
		`,
	}
	for name, body := range files {
		p := core.Path(dir, name)
		if err := medium.EnsureDir(core.PathDir(p)); err != nil {
			t.Fatalf("EnsureDir %s: %v", p, err)
		}
		if err := medium.Write(p, body); err != nil {
			t.Fatalf("Write %s: %v", p, err)
		}
	}

	scan, err := app.ScanElectronRenderer(medium, dir)
	if err != nil {
		t.Fatalf("ScanElectronRenderer: %v", err)
	}
	if !scan.FS {
		t.Error("scan.FS = false; want true")
	}
	if !scan.Net {
		t.Error("scan.Net = false; want true")
	}
	if !scan.ClipboardRead || !scan.ClipboardWrite {
		t.Errorf("clipboard read/write = %v/%v; want true/true", scan.ClipboardRead, scan.ClipboardWrite)
	}
	if !scan.DialogOpen || !scan.DialogSave {
		t.Errorf("dialog open/save = %v/%v; want true/true", scan.DialogOpen, scan.DialogSave)
	}
	if !scan.ShellOpen {
		t.Error("scan.ShellOpen = false; want true")
	}
	if !scan.Notification {
		t.Error("scan.Notification = false; want true")
	}
	if len(scan.IPCChannels) != 2 {
		t.Errorf("IPCChannels = %v; want two entries", scan.IPCChannels)
	}
	// Channels sorted.
	if scan.IPCChannels[0] != "miner:start" || scan.IPCChannels[1] != "miner:stop" {
		t.Errorf("IPCChannels not sorted: %v", scan.IPCChannels)
	}
}

// TestPkgElectron_ScanElectronRenderer_Bad rejects empty dir and
// missing directories.
func TestPkgElectron_ScanElectronRenderer_Bad(t *testing.T) {
	if _, err := app.ScanElectronRenderer(coreio.Local, ""); err == nil {
		t.Error("empty dir produced no error")
	}
	// A path that doesn't exist — medium.List returns an error which
	// propagates up.
	if _, err := app.ScanElectronRenderer(coreio.Local, "/definitely/does/not/exist/"); err == nil {
		t.Error("missing dir produced no error")
	}
}

// TestPkgElectron_ScanElectronRenderer_Ugly walks a directory with NO
// recognisable Electron patterns and confirms we return a clean-zero
// scan instead of false positives.
func TestPkgElectron_ScanElectronRenderer_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	files := map[string]string{
		"main.js": `console.log("hi"); // no electron APIs`,
		"readme":  "just a plain file, should be skipped by extension filter",
	}
	for name, body := range files {
		if err := medium.Write(core.Path(dir, name), body); err != nil {
			t.Fatalf("Write %s: %v", name, err)
		}
	}
	scan, err := app.ScanElectronRenderer(medium, dir)
	if err != nil {
		t.Fatalf("ScanElectronRenderer: %v", err)
	}
	if scan.FS || scan.Net || scan.ClipboardRead || scan.ClipboardWrite ||
		scan.DialogOpen || scan.DialogSave || scan.ShellOpen || scan.Notification {
		t.Errorf("clean renderer produced false positives: %+v", scan)
	}
	if len(scan.IPCChannels) != 0 {
		t.Errorf("IPCChannels = %v; want empty", scan.IPCChannels)
	}
}

// TestPkgElectron_WrapElectron_Good projects a fully-populated
// ElectronPackageJSON + scan into a ViewManifest and checks every
// mapping from RFC §16.2.
func TestPkgElectron_WrapElectron_Good(t *testing.T) {
	pkg := &app.ElectronPackageJSON{
		Name:        "nicehash-quickminer",
		ProductName: "NiceHash QuickMiner",
		Version:     "4.1.0",
		Main:        "main.js",
	}
	scan := &app.ElectronScanResult{
		FS:            true,
		Net:           true,
		ClipboardRead: true,
		Notification:  true,
		IPCChannels:   []string{"app:ready", "miner:start"},
	}
	m := app.WrapElectron(pkg, scan, app.WrapElectronOptions{})
	if m == nil {
		t.Fatal("WrapElectron returned nil")
	}
	if m.Code != "nicehash-quickminer" {
		t.Errorf("Code = %q", m.Code)
	}
	if m.Name != "NiceHash QuickMiner" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "4.1.0" {
		t.Errorf("Version = %q", m.Version)
	}
	// FS → narrow read/write declarations under the data dir only.
	if len(m.Permissions.Read) != 1 || m.Permissions.Read[0] != "./data/" {
		t.Errorf("Permissions.Read = %v", m.Permissions.Read)
	}
	if m.Permissions.Filesystem {
		t.Error("Permissions.Filesystem = true; want false (RFC §16.2 keeps fs scoped to ./data/)")
	}
	// RFC §16.2 — `require('fs')` should also produce a write list so
	// the entitlement gate honours per-path writes without widening the
	// manifest to the filesystem catch-all.
	writeList, ok := m.Config["write"].([]any)
	if !ok || len(writeList) != 1 || writeList[0] != "./data/" {
		t.Errorf("Config[write] = %v; want [./data/] (RFC §16.2)", m.Config["write"])
	}
	if m.Permissions.Network {
		t.Error("Permissions.Network = true; want false (wildcard net list is the RFC surface)")
	}
	// RFC §16.2 — `require('net')` should produce `net: ["*"]`
	// (wildcard) without the legacy `network: true` companion.
	if len(m.Permissions.Net) != 1 || m.Permissions.Net[0] != "*" {
		t.Errorf("Permissions.Net = %v; want [*] (RFC §16.2 wildcard)", m.Permissions.Net)
	}
	if !m.Permissions.Clipboard {
		t.Error("Permissions.Clipboard = false; want true (clipboard detected)")
	}
	if !m.Permissions.Notifications {
		t.Error("Permissions.Notifications = false; want true")
	}
	if err := app.CheckAccess(m, app.AccessRead, "./data/state.json"); err != nil {
		t.Errorf("AccessRead ./data/state.json denied: %v", err)
	}
	if err := app.CheckAccess(m, app.AccessWrite, "./data/state.json"); err != nil {
		t.Errorf("AccessWrite ./data/state.json denied: %v", err)
	}
	if err := app.CheckAccess(m, app.AccessRead, "./secrets.txt"); err == nil {
		t.Error("AccessRead outside ./data/ should be denied")
	}
	if err := app.CheckAccess(m, app.AccessWrite, "./secrets.txt"); err == nil {
		t.Error("AccessWrite outside ./data/ should be denied")
	}
	// Config carries the Electron-specific shape.
	if m.Config["type"] != "electron" {
		t.Errorf("Config[type] = %v; want 'electron'", m.Config["type"])
	}
	shim, ok := m.Config["shim"].(map[string]any)
	if !ok {
		t.Fatal("Config[shim] not a map")
	}
	if shim["electron"] != true {
		t.Errorf("shim[electron] = %v; want true", shim["electron"])
	}
	if shim["fs_proxy"] != true {
		t.Errorf("shim[fs_proxy] = %v; want true", shim["fs_proxy"])
	}
	channels, ok := m.Config["ipc_channels"].([]any)
	if !ok || len(channels) != 2 {
		t.Errorf("Config[ipc_channels] = %v", m.Config["ipc_channels"])
	}
}

// TestPkgElectron_WrapElectron_Bad handles nil inputs.
func TestPkgElectron_WrapElectron_Bad(t *testing.T) {
	if app.WrapElectron(nil, nil, app.WrapElectronOptions{}) != nil {
		t.Error("WrapElectron(nil, nil) returned non-nil")
	}
	// A package.json without a scan still produces a baseline manifest.
	m := app.WrapElectron(&app.ElectronPackageJSON{Name: "x"}, nil, app.WrapElectronOptions{})
	if m == nil {
		t.Fatal("WrapElectron returned nil with non-nil pkg")
	}
	if m.Code != "x" {
		t.Errorf("Code = %q; want 'x'", m.Code)
	}
	// No scan → no permissions.
	if m.Permissions.Filesystem || m.Permissions.Network {
		t.Errorf("no-scan manifest has unexpected permissions: %+v", m.Permissions)
	}
}

// TestPkgElectron_WrapElectron_Ugly covers the option overrides.
func TestPkgElectron_WrapElectron_Ugly(t *testing.T) {
	pkg := &app.ElectronPackageJSON{ProductName: "Weird Name?!"}
	m := app.WrapElectron(pkg, &app.ElectronScanResult{FS: true},
		app.WrapElectronOptions{Code: "custom-code", DataDir: "./store/"})
	if m.Code != "custom-code" {
		t.Errorf("Code = %q; want override", m.Code)
	}
	if m.Permissions.Read[0] != "./store/" {
		t.Errorf("Permissions.Read[0] = %q; want override dir", m.Permissions.Read[0])
	}
	// RFC §16.2 — Config[write] should follow the override dir so the
	// per-path declaration matches the read list (not the default ./data/).
	writeList, ok := m.Config["write"].([]any)
	if !ok || len(writeList) != 1 || writeList[0] != "./store/" {
		t.Errorf("Config[write] = %v; want [./store/]", m.Config["write"])
	}
}

// TestPkgElectron_WriteElectronWrap_Good round-trips a wrapped Electron
// manifest to disk and reloads it via config.LoadManifest.
func TestPkgElectron_WriteElectronWrap_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local
	m := app.WrapElectron(&app.ElectronPackageJSON{Name: "x", Version: "1.0.0"},
		&app.ElectronScanResult{FS: true}, app.WrapElectronOptions{})

	if err := app.WriteElectronWrap(medium, dir, m); err != nil {
		t.Fatalf("WriteElectronWrap: %v", err)
	}
	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dir, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if round.Code != "x" {
		t.Errorf("round-trip Code = %q", round.Code)
	}
}

// TestPkgElectron_WriteElectronWrap_Bad catches the nil-manifest guard.
func TestPkgElectron_WriteElectronWrap_Bad(t *testing.T) {
	if err := app.WriteElectronWrap(coreio.Local, t.TempDir(), nil); err == nil {
		t.Error("WriteElectronWrap(nil) returned no error")
	}
}
