// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
	"gopkg.in/yaml.v3"
)

// ElectronPackageJSON is the subset of an Electron app's package.json we
// need to produce a CoreApp view.yaml. The fields line up with RFC §16.2
// permission auto-detection and identity resolution.
//
//	var pkg ElectronPackageJSON
//	_ = core.JSONUnmarshal(body, &pkg)
type ElectronPackageJSON struct {
	Name            string            `json:"name"`
	ProductName     string            `json:"productName,omitempty"`
	Version         string            `json:"version,omitempty"`
	Description     string            `json:"description,omitempty"`
	Main            string            `json:"main,omitempty"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
}

// ElectronScanResult captures what we detected about an Electron app's
// renderer — which Electron APIs it uses and therefore which CoreApp
// permissions it needs. Populated by ScanElectronRenderer; consumed by
// WrapElectron.
//
//	scan, _ := app.ScanElectronRenderer(medium, "./src/")
//	m := app.WrapElectron(pkgJSON, scan, app.WrapElectronOptions{})
type ElectronScanResult struct {
	// FS true → `require('fs')` or `require('node:fs')` seen; triggers
	// read/write on ./data/ per RFC §16.2.
	FS bool
	// Net true → `require('net')` / `require('http')` / `fetch(` seen.
	Net bool
	// Clipboard read / write — from `clipboard.readText()` /
	// `clipboard.writeText()` or ipcRenderer clipboard channels.
	ClipboardRead  bool
	ClipboardWrite bool
	// Dialogs — `dialog.showOpenDialog` / `dialog.showSaveDialog`.
	DialogOpen bool
	DialogSave bool
	// Shell — `shell.openExternal(...)` → browser open permission.
	ShellOpen bool
	// Notifications — `new Notification(...)`.
	Notification bool
	// IPCChannels is the sorted unique list of channel names used with
	// `ipcRenderer.send(...)` / `ipcMain.on(...)`.
	IPCChannels []string
}

// ScanElectronRenderer walks a directory of renderer assets (HTML/JS)
// and collects the Electron API patterns named in RFC §16.2. The walker
// is deliberately shallow — one level of indirection through medium.List
// — because `core pkg wrap --electron` downloads the renderer to a
// scratch directory we control, so an N-level recursion isn't needed.
//
//	scan, err := app.ScanElectronRenderer(coreio.Local, "./renderer")
//
// The scan is a substring match, not a full AST pass. The RFC
// acknowledges that — "scans main.js for Electron API usage patterns".
// It favours false positives (extra permissions) over false negatives
// (missing capability) because a wrapped Electron app that's missing a
// capability dies at first use.
func ScanElectronRenderer(medium coreio.Medium, dir string) (*ElectronScanResult, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return nil, coreerr.E("app.ScanElectronRenderer", "empty dir", nil)
	}

	res := &ElectronScanResult{}
	seenChannels := map[string]bool{}

	walk := func(path string) {
		body, err := medium.Read(path)
		if err != nil || body == "" {
			return
		}
		scanElectronBody(body, res, seenChannels)
	}

	// Walk the directory recursively, but only the files that plausibly
	// carry Electron API calls — JS, MJS, TS, HTML and JSON. Binaries
	// (*.node, *.dll, *.so) are skipped to save IO.
	if err := walkFiles(medium, dir, func(p string) {
		switch core.Lower(core.PathExt(p)) {
		case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".html", ".htm", ".json":
			walk(p)
		}
	}); err != nil {
		return nil, coreerr.E("app.ScanElectronRenderer", "walk failed", err)
	}

	// Sort the IPC channel list for stable test output.
	res.IPCChannels = sortedKeys(seenChannels)

	return res, nil
}

// scanElectronBody does the substring matching for one file. Kept as a
// separate helper so the tests can hand in canned strings.
//
//	scanElectronBody("clipboard.readText()", &res, seenIPC)
func scanElectronBody(body string, res *ElectronScanResult, seenChannels map[string]bool) {
	switch {
	case core.Contains(body, "require('fs')"),
		core.Contains(body, "require(\"fs\")"),
		core.Contains(body, "require('node:fs')"),
		core.Contains(body, "require(\"node:fs\")"),
		core.Contains(body, "from 'fs'"),
		core.Contains(body, "from \"fs\""),
		core.Contains(body, "from 'node:fs'"),
		core.Contains(body, "from \"node:fs\""):
		res.FS = true
	}

	switch {
	case core.Contains(body, "require('net')"),
		core.Contains(body, "require(\"net\")"),
		core.Contains(body, "require('http')"),
		core.Contains(body, "require(\"http\")"),
		core.Contains(body, "require('https')"),
		core.Contains(body, "require(\"https\")"),
		core.Contains(body, "fetch("),
		core.Contains(body, "XMLHttpRequest"),
		core.Contains(body, "new WebSocket"):
		res.Net = true
	}

	if core.Contains(body, "clipboard.readText") || core.Contains(body, "clipboard.read(") {
		res.ClipboardRead = true
	}
	if core.Contains(body, "clipboard.writeText") || core.Contains(body, "clipboard.write(") {
		res.ClipboardWrite = true
	}
	if core.Contains(body, "dialog.showOpenDialog") || core.Contains(body, "showOpenDialogSync") {
		res.DialogOpen = true
	}
	if core.Contains(body, "dialog.showSaveDialog") || core.Contains(body, "showSaveDialogSync") {
		res.DialogSave = true
	}
	if core.Contains(body, "shell.openExternal") || core.Contains(body, "shell.openPath") {
		res.ShellOpen = true
	}
	if core.Contains(body, "new Notification(") || core.Contains(body, "= new Notification(") {
		res.Notification = true
	}

	collectIPCChannels(body, seenChannels)
}

// collectIPCChannels does a naive extract of the channel name argument
// in `ipcRenderer.send('channel', ...)` / `ipcMain.on('channel', ...)`.
//
//	collectIPCChannels("ipcRenderer.send('miner:start', 1)", m)
func collectIPCChannels(body string, seen map[string]bool) {
	for _, prefix := range []string{
		"ipcRenderer.send('",
		"ipcRenderer.send(\"",
		"ipcRenderer.invoke('",
		"ipcRenderer.invoke(\"",
		"ipcMain.on('",
		"ipcMain.on(\"",
		"ipcMain.handle('",
		"ipcMain.handle(\"",
	} {
		idx := 0
		for {
			match := core.Contains(body[idx:], prefix)
			if !match {
				break
			}
			start := indexOf(body[idx:], prefix) + idx
			open := start + len(prefix)
			if open >= len(body) {
				break
			}
			quote := prefix[len(prefix)-1]
			end := -1
			for j := open; j < len(body); j++ {
				if body[j] == quote {
					end = j
					break
				}
			}
			if end <= open {
				break
			}
			seen[body[open:end]] = true
			idx = end
		}
	}
}

// walkFiles recursively visits every file beneath dir on the supplied
// medium, calling `visit` for each. Kept local to this file so the
// walker behaviour (skip hidden, skip non-files) matches the RFC intent
// without burdening coreio.
//
//	_ = walkFiles(medium, dir, func(p string) { fmt.Println(p) })
func walkFiles(medium coreio.Medium, dir string, visit func(string)) error {
	if medium == nil {
		medium = coreio.Local
	}
	entries, err := medium.List(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "" || name[0] == '.' {
			continue // skip hidden and "." / ".."
		}
		p := core.Path(dir, name)
		if e.IsDir() {
			if err := walkFiles(medium, p, visit); err != nil {
				return err
			}
			continue
		}
		visit(p)
	}
	return nil
}

// sortedKeys returns the map keys in lexicographic order. Deterministic
// output keeps golden-file tests stable.
//
//	sortedKeys(map[string]bool{"b": true, "a": true}) // []string{"a", "b"}
func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Tiny insertion sort — the map is small and avoids importing
	// sort (not banned but trivial at this size).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// WrapElectronOptions tunes the Electron wrap. Zero values are fine.
//
//	opts := app.WrapElectronOptions{
//	    Code:    "bitwarden",
//	    DataDir: "./data/",
//	}
type WrapElectronOptions struct {
	// Code overrides the generated slug.
	Code string
	// DataDir is the relative path the wrapped app gets read/write to
	// when ScanResult.FS is true. Defaults to "./data/".
	DataDir string
}

// WrapElectronRepoOptions tunes WrapElectronRepo. ScratchDir is where
// the release asset is downloaded and, when archived, extracted before
// scanning and wrapping.
type WrapElectronRepoOptions struct {
	Code       string
	Name       string
	Version    string
	ScratchDir string
}

// WrapElectron projects an Electron package.json + renderer scan into
// a CoreApp ViewManifest per RFC §16.2. The returned manifest is
// unsigned — signing is a later step.
//
//	m := app.WrapElectron(pkg, scan, app.WrapElectronOptions{})
//
// Rules:
//
//   - `type=electron` and `shim.electron=true` are recorded in Config so
//     CoreGUI knows to inject the Electron compat layer.
//
//   - `permissions.run` remains empty — an Electron app inside CoreGUI
//     should never shell out; the wrapper is the one running.
func WrapElectron(pkg *ElectronPackageJSON, scan *ElectronScanResult, opts WrapElectronOptions) *config.ViewManifest {
	if pkg == nil {
		return nil
	}

	code := core.Trim(opts.Code)
	if code == "" {
		code = slugify(coalesce(pkg.ProductName, pkg.Name, "electron-app"))
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = "./data/"
	}

	m := &config.ViewManifest{
		Code:    code,
		Name:    coalesce(pkg.ProductName, pkg.Name, code),
		Version: coalesce(pkg.Version, "0.1.0"),
		Layout:  "C",
	}

	if scan != nil {
		if scan.FS {
			m.Permissions.Read = []string{dataDir}
			// ViewPermissions has no per-path write list yet; set the
			// Filesystem catch-all when we detect fs usage so the
			// permission gate doesn't deny write access at runtime.
			m.Permissions.Filesystem = true
		}
		if scan.Net {
			// RFC §16.2 — `require('net')` → `net: ["*"]` wildcard
			// declaration (the spec table lists the explicit literal so a
			// wrapped Electron app keeps unrestricted network access).
			// Network=true mirrors the boolean catch-all so legacy
			// permission gates that consult ViewPermissions.Network still
			// see the capability without re-walking the Net slice.
			m.Permissions.Net = []string{"*"}
			m.Permissions.Network = true
		}
		// RFC §16.2 — `require('fs')` → `write: ["./data/"]` per-path
		// declaration. ViewPermissions has no typed write list yet so the
		// Config["write"] map carries the per-path slice; the entitlement
		// gate's manifestWriteList helper consults it before falling back
		// to the Filesystem catch-all. Recording the write path keeps the
		// per-path wider check (access.go) symmetric with the Read slice.
		if scan.ClipboardRead || scan.ClipboardWrite {
			m.Permissions.Clipboard = true
		}
		if scan.Notification {
			m.Permissions.Notifications = true
		}
	}

	// Pack the remaining Electron-specific metadata into Config. Keeps
	// ViewManifest lossless without needing the schema upstream.
	cfg := map[string]any{
		"type": PackageTypeElectron.String(),
		"shim": map[string]any{
			"electron": true,
			"fs_proxy": scan != nil && scan.FS,
		},
	}
	if scan != nil {
		// RFC §16.2 — `require('fs')` produces both a read and a write
		// declaration for the data directory. The write list lands under
		// Config["write"] because ViewPermissions has no typed write-path
		// slot yet (access.go's manifestWriteList knows where to look).
		if scan.FS {
			cfg["write"] = []any{dataDir}
		}
		gates := map[string]any{}
		if scan.DialogOpen {
			gates["gui.dialog.open"] = true
		}
		if scan.DialogSave {
			gates["gui.dialog.save"] = true
		}
		if scan.ShellOpen {
			gates["gui.browser.open"] = true
		}
		if scan.Notification {
			gates["gui.notification.send"] = true
		}
		if scan.ClipboardRead {
			gates["gui.clipboard.read"] = true
		}
		if scan.ClipboardWrite {
			gates["gui.clipboard.write"] = true
		}
		if len(gates) > 0 {
			cfg["gui_gates"] = gates
		}
		if len(scan.IPCChannels) > 0 {
			channels := make([]any, 0, len(scan.IPCChannels))
			for _, c := range scan.IPCChannels {
				channels = append(channels, c)
			}
			cfg["ipc_channels"] = channels
		}
	}
	if pkg.Main != "" {
		cfg["main"] = pkg.Main
	}
	m.Config = cfg
	return m
}

// WrapElectronRepo fetches the latest renderer asset from a GitHub
// release reference, extracts it when needed, scans the unpacked tree,
// and returns the wrapped manifest plus the renderer directory that
// should be copied into the install root.
func WrapElectronRepo(ctx context.Context, medium coreio.Medium, ref string, opts WrapElectronRepoOptions) (*config.ViewManifest, string, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if ref == "" {
		return nil, "", coreerr.E("app.WrapElectronRepo", "empty repo reference", nil)
	}
	if opts.ScratchDir == "" {
		return nil, "", coreerr.E("app.WrapElectronRepo", "empty scratch dir", nil)
	}

	host, owner, repo, ok := ParseGitHubRepo(ref)
	if !ok {
		return nil, "", coreerr.E("app.WrapElectronRepo", "cannot parse repo reference: "+ref, nil)
	}
	if !isGitHubReleaseHost(host) {
		return nil, "", coreerr.E(
			"app.WrapElectronRepo",
			"repo host does not expose GitHub releases: "+host,
			nil,
		)
	}
	if medium.IsDir(opts.ScratchDir) {
		if err := medium.DeleteAll(opts.ScratchDir); err != nil {
			return nil, "", coreerr.E("app.WrapElectronRepo", "clear scratch dir failed", err)
		}
	}
	if err := medium.EnsureDir(opts.ScratchDir); err != nil {
		return nil, "", coreerr.E("app.WrapElectronRepo", "ensure scratch dir failed", err)
	}

	rel, err := FetchElectronRelease(ctx, host, owner, repo)
	if err != nil {
		return nil, "", coreerr.E("app.WrapElectronRepo", "release fetch failed", err)
	}
	asset, ok := SelectRendererAsset(rel)
	if !ok {
		return nil, "", coreerr.E(
			"app.WrapElectronRepo",
			"release "+rel.TagName+" has no renderer-shaped asset",
			nil,
		)
	}

	archivePath, err := DownloadAsset(ctx, medium, asset, opts.ScratchDir)
	if err != nil {
		return nil, "", coreerr.E("app.WrapElectronRepo", "asset download failed", err)
	}

	rendererDir := opts.ScratchDir
	if isArchivePath(asset.Name) {
		rendererDir = ArchiveExtractedDir(opts.ScratchDir, asset.Name)
		if err := ExtractArchive(medium, archivePath, rendererDir); err != nil {
			return nil, "", coreerr.E("app.WrapElectronRepo", "archive extract failed", err)
		}
	}

	var pkg ElectronPackageJSON
	pkgPath := core.Path(rendererDir, "package.json")
	if medium.Exists(pkgPath) {
		if body, readErr := medium.Read(pkgPath); readErr == nil {
			r := core.JSONUnmarshal([]byte(body), &pkg)
			if !r.OK {
				// A malformed package.json should not block wrapping when
				// the renderer scan can still infer the permissions.
				_ = r
			}
		}
	}
	if pkg.Name == "" && pkg.ProductName == "" {
		pkg.Name = repo
	}
	scan, err := ScanElectronRenderer(medium, rendererDir)
	if err != nil {
		return nil, "", coreerr.E("app.WrapElectronRepo", "renderer scan failed", err)
	}

	manifest := WrapElectron(&pkg, scan, WrapElectronOptions{Code: opts.Code})
	if manifest == nil {
		return nil, "", coreerr.E("app.WrapElectronRepo", "WrapElectron returned nil", nil)
	}
	if manifest.Code == "" || manifest.Code == "electron-app" {
		manifest.Code = slugify(coalesce(opts.Code, repo))
	}
	if opts.Name != "" {
		manifest.Name = opts.Name
	}
	if opts.Version != "" {
		manifest.Version = opts.Version
	}
	return manifest, rendererDir, nil
}

// isGitHubReleaseHost returns true for github.com and GitHub Enterprise
// style hosts. GitLab and other forge hosts are excluded — their
// release APIs differ, so Electron wrapping falls back elsewhere.
func isGitHubReleaseHost(host string) bool {
	host = core.Lower(core.Trim(host))
	if host == "" {
		return false
	}
	return host == "github.com" || core.Contains(host, "github")
}

// WriteElectronWrap materialises a wrapped Electron manifest to
// `<dest>/.core/view.yaml`. Mirrors WritePWAWrap for the CLI path.
//
//	err := app.WriteElectronWrap(coreio.Local, "/.../apps/bitwarden", manifest)
func WriteElectronWrap(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if manifest == nil {
		return coreerr.E("app.WriteElectronWrap", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return coreerr.E("app.WriteElectronWrap", "marshal failed", err)
	}
	path := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return coreerr.E("app.WriteElectronWrap", "ensure dir failed", err)
	}
	if err := medium.Write(path, string(body)); err != nil {
		return coreerr.E("app.WriteElectronWrap", "write failed", err)
	}
	return nil
}
