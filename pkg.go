// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// AppsDirName is the component that follows `$DIR_HOME/.core/` for the
// installed-packages tree. Matches the dAppServer convention
// (`~/Lethean/apps/`) ported to the `.core` root.
//
//	dir := core.Path(home, ".core", app.AppsDirName)
const AppsDirName = "apps"

// PkgEntry describes one installed package. The shape mirrors the RFC
// §16.3 `core pkg list` columns so CLI rendering is a zero-conversion
// read of the slice.
//
//	entries, _ := app.PkgList(coreio.Local, homeDir)
//	for _, e := range entries {
//	    fmt.Println(e.Name, e.Type, e.Version, e.DisplaySource())
//	}
//
// Source is the raw provenance tag (`wrap:pwa:<url>`,
// `marketplace:<code>`, `local:<path>`, …) so `pkg update` can dispatch
// on the correct strategy. Use DisplaySource() when surfacing the
// value to a human — it strips the `wrap:TYPE:` prefix per RFC §16.3.
type PkgEntry struct {
	Name     string      // manifest.code — the user-facing slug
	Type     PackageType // native | pwa | electron | web
	Version  string      // manifest.version
	Source   string      // marketplace:CODE | wrap:pwa:URL | wrap:electron:REF | local:PATH
	Category string      // marketplace category the install came from (empty for wraps / local)
	Path     string      // absolute install path (<home>/.core/apps/<code>)
}

// DisplaySource returns the human-facing rendering of Source per the
// RFC §16.3 `pkg list` example:
//
//	marketplace:photo-browser            → "marketplace"
//	wrap:pwa:https://play.example.com    → "https://play.example.com"
//	wrap:electron:github.com/foo/bar     → "github.com/foo/bar"
//	wrap:web:./my-webapp                 → "./my-webapp"
//	local:/srv/app                       → "local"
//	(unknown / empty)                    → "local"
//
// The raw Source is preserved so `pkg update` keeps its dispatch
// information; this method exists purely to format the value.
//
//	fmt.Println(e.DisplaySource()) // "https://play.example.com"
func (e PkgEntry) DisplaySource() string {
	switch {
	case e.Source == "":
		return "local"
	case core.HasPrefix(e.Source, "wrap:pwa:"):
		return e.Source[len("wrap:pwa:"):]
	case core.HasPrefix(e.Source, "wrap:electron:"):
		return e.Source[len("wrap:electron:"):]
	case core.HasPrefix(e.Source, "wrap:web:"):
		return e.Source[len("wrap:web:"):]
	case core.HasPrefix(e.Source, "marketplace:"):
		return "marketplace"
	case core.HasPrefix(e.Source, "local:"):
		return "local"
	default:
		return e.Source
	}
}

// PkgList returns every installed package by scanning
// `<home>/.core/apps/*/.core/view.yaml`. Missing directories are not
// errors — a fresh user with no installs gets an empty slice.
//
//	entries, err := app.PkgList(coreio.Local, "/Users/me")
//
// Entries are sorted lexicographically by Name so CLI tables and JSON
// consumers see a deterministic order across runs (matches the
// InstalledApps contract).
func PkgList(medium coreio.Medium, home string) ([]PkgEntry, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return nil, coreerr.E("app.PkgList", "empty home directory", nil)
	}
	appsDir := core.Path(home, ".core", AppsDirName)
	if !medium.IsDir(appsDir) {
		return nil, nil
	}

	entries, err := medium.List(appsDir)
	if err != nil {
		return nil, coreerr.E("app.PkgList", "list apps dir failed", err)
	}

	var out []PkgEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		appPath := core.Path(appsDir, name)
		viewPath := core.Path(appPath, ".core", "view.yaml")
		if !medium.Exists(viewPath) {
			continue
		}
		pe, err := pkgEntryFromManifest(medium, viewPath, appPath)
		if err != nil {
			// Skip unreadable installs but keep scanning — one broken
			// install shouldn't hide the others in the list.
			continue
		}
		out = append(out, pe)
	}
	// Deterministic order — same rule InstalledApps uses so `pkg list`
	// and `pkg info` render consistent output on repeated runs.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Name > out[j].Name; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}

// InstalledApp pairs a parsed ViewManifest with the install path on
// disk. Returned by InstalledApps when a host wants to introspect
// every installed plugin without launching them (CoreGUI plugin
// drawer, `core-agent` fleet announce, diagnostic tooling).
//
//	apps, _ := app.InstalledApps(coreio.Local, home)
//	for _, a := range apps {
//	    core.Info("installed", "code", a.Manifest.Code, "path", a.Path)
//	}
type InstalledApp struct {
	// Manifest is the fully-parsed ViewManifest (permissions, layout,
	// modules, config all populated).
	Manifest config.ViewManifest
	// Path is the absolute install root — the directory containing
	// `.core/view.yaml`. Callers needing the manifest file itself can
	// join PathDir with `.core/view.yaml`.
	Path string
}

// InstalledApps walks `<home>/.core/apps/*/.core/view.yaml` and returns
// every installed package's fully-parsed manifest. Use PkgList when a
// summary row (name, type, version, source) is enough; use this when
// the host needs to inspect permissions, modules or layout without
// taking a Launch round-trip.
//
//	apps, err := app.InstalledApps(coreio.Local, "/Users/me")
//
// Rules:
//
//   - Missing apps directory → nil slice + nil error. A fresh user
//     should not have to handle an error just to render an empty list.
//
//   - Unreadable / malformed manifests are skipped silently, the same
//     tolerance PkgList uses — a single broken install must not hide
//     the rest of the tree.
//
//   - The returned slice is sorted by manifest.Code so tests and UIs
//     get a deterministic rendering order.
func InstalledApps(medium coreio.Medium, home string) ([]InstalledApp, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return nil, coreerr.E("app.InstalledApps", "empty home directory", nil)
	}
	appsDir := core.Path(home, ".core", AppsDirName)
	if !medium.IsDir(appsDir) {
		return nil, nil
	}
	entries, err := medium.List(appsDir)
	if err != nil {
		return nil, coreerr.E("app.InstalledApps", "list apps dir failed", err)
	}
	var out []InstalledApp
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		appPath := core.Path(appsDir, name)
		viewPath := core.Path(appPath, ".core", "view.yaml")
		if !medium.Exists(viewPath) {
			continue
		}
		var manifest config.ViewManifest
		if err := LoadViewManifest(medium, viewPath, &manifest); err != nil {
			// Skip unreadable installs so one bad manifest does not
			// hide the rest of the tree — matches PkgList's policy.
			continue
		}
		out = append(out, InstalledApp{Manifest: manifest, Path: appPath})
	}
	// Deterministic order so hosts that render the list get stable
	// rows. Using manifest.Code (rather than directory name) means a
	// rename on disk does not reshuffle the UI.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Manifest.Code > out[j].Manifest.Code; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}

// pkgEntryFromManifest loads a view.yaml and projects it into a PkgEntry.
// The helper reads the optional `type` and `source` keys from the
// Config map (where PWA/Electron wraps stash their provenance).
//
//	pe, err := pkgEntryFromManifest(medium, viewPath, appPath)
func pkgEntryFromManifest(medium coreio.Medium, viewPath, appPath string) (PkgEntry, error) {
	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, viewPath, &manifest); err != nil {
		return PkgEntry{}, err
	}
	entry := PkgEntry{
		Name:    manifest.Code,
		Type:    PackageTypeNative,
		Version: manifest.Version,
		Path:    appPath,
	}
	if manifest.Config != nil {
		if t, ok := manifest.Config["type"].(string); ok {
			entry.Type = ParsePackageType(t)
		}
		if s, ok := manifest.Config["source"].(string); ok {
			entry.Source = s
		}
		if entry.Source == "" {
			if u, ok := manifest.Config["url"].(string); ok {
				entry.Source = u
			}
		}
		if c, ok := manifest.Config["category"].(string); ok {
			entry.Category = c
		}
	}
	if entry.Source == "" {
		entry.Source = "local"
	}
	return entry, nil
}

// PkgDetails is the full describe-an-installed-package projection. It
// pairs the summary row (PkgEntry) with the fully-parsed manifest, the
// resolved workspace paths (when a workspace exists on disk), and a
// human-ordered permission summary so a host UI (or a CI report) can
// render every relevant field without re-opening view.yaml.
//
//	info, err := app.PkgInfo(coreio.Local, "/Users/me", "photo-browser")
//	for _, perm := range info.Permissions { core.Println(perm) }
//
// Fields:
//
//   - Entry is the same PkgEntry PkgList would return (name, type,
//     version, source, path) so CLI tables and JSON output share the
//     summary shape.
//
//   - Manifest is the fully-parsed ViewManifest. Handlers that need to
//     inspect modules / layout / slots read from here.
//
//   - Permissions is the human-sorted permission summary
//     (`read: ./data/`, `net: api.example.com:443`, `store`, etc.). One
//     entry per declared capability so a single print loop renders the
//     full security surface.
//
//   - Workspace is the absolute path to `<home>/.core/data/<code>/` when
//     present on disk. Empty when the workspace has not been provisioned
//     (fresh install that has never booted).
type PkgDetails struct {
	Entry       PkgEntry            // summary row — same shape PkgList emits
	Manifest    config.ViewManifest // full manifest (permissions, modules, layout, slots, config)
	Permissions []string            // human-sorted permission summary
	Workspace   string              // absolute path to the per-app data tree (empty when missing)
}

// PkgInfo returns everything a caller needs to describe a single
// installed package — the summary row, the full manifest, a flattened
// permission summary, and the workspace path when provisioned. Used by
// `core pkg info NAME` and any host UI that shows a package detail
// pane.
//
//	info, err := app.PkgInfo(coreio.Local, "/Users/me", "photo-browser")
//
// Rules:
//
//   - Missing `home` → typed error (the caller cannot resolve the apps
//     tree without one).
//
//   - Missing `name` → typed error so callers that forget the argument
//     get a useful message instead of an empty result.
//
//   - Package not installed → typed error naming the expected path so
//     the CLI message points the user at the right `pkg install`
//     command.
//
//   - Workspace lookup is best-effort — a missing data tree returns
//     Workspace="" but does not fail the call. A first-boot package
//     wouldn't have one yet.
func PkgInfo(medium coreio.Medium, home, name string) (*PkgDetails, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return nil, coreerr.E("app.PkgInfo", "empty home directory", nil)
	}
	if name == "" {
		return nil, coreerr.E("app.PkgInfo", "empty package name", nil)
	}

	appPath := core.Path(home, ".core", AppsDirName, name)
	viewPath := core.Path(appPath, ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		return nil, coreerr.E(
			"app.PkgInfo",
			"package not installed: "+name+" (expected "+viewPath+")",
			nil,
		)
	}

	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, viewPath, &manifest); err != nil {
		return nil, coreerr.E("app.PkgInfo", "parse manifest failed", err)
	}

	entry, err := pkgEntryFromManifest(medium, viewPath, appPath)
	if err != nil {
		return nil, coreerr.E("app.PkgInfo", "project entry failed", err)
	}

	info := &PkgDetails{
		Entry:       entry,
		Manifest:    manifest,
		Permissions: ManifestPermissionSummary(&manifest),
	}

	workspace := core.Path(home, ".core", DataDirName, manifest.Code)
	if medium.IsDir(workspace) {
		info.Workspace = workspace
	}
	return info, nil
}

// ManifestPermissionSummary flattens a manifest's permission declarations
// into a human-sorted slice of `capability: detail` strings. Each entry
// is self-describing so a UI can render the full security surface with
// a single print loop.
//
//	for _, p := range app.ManifestPermissionSummary(&manifest) {
//	    core.Println(p)
//	}
//
// Output forms:
//
//   - `read: ./photos/`          — one entry per declared read prefix
//   - `write: ./photos/.thumbs/` — one entry per declared write prefix
//   - `net: api.example.com:443` — one entry per declared host:port
//   - `run: phpstan`             — one entry per declared binary
//   - `store`                    — single entry when store is declared
//   - `filesystem`               — legacy catch-all bool
//   - `network`                  — legacy catch-all bool
//   - `notifications`            — bool permissions render as one entry
//   - `clipboard`                — (read and write share one field today)
//   - `camera`, `microphone`     — typed booleans
//   - `location`                 — derived from the `device.location`
//     entry in permissions.run (RFC §9.3
//     legacy placement until ViewPermissions
//     grows a typed slot)
//
// Sort order is stable: fields are emitted in the order above so a diff
// tool comparing two manifests shows a consistent delta.
func ManifestPermissionSummary(m *config.ViewManifest) []string {
	if m == nil {
		return nil
	}
	var out []string

	for _, p := range m.Permissions.Read {
		if p != "" {
			out = append(out, "read: "+p)
		}
	}
	for _, p := range manifestWriteList(m) {
		if p != "" {
			out = append(out, "write: "+p)
		}
	}
	for _, p := range m.Permissions.Net {
		if p != "" {
			out = append(out, "net: "+p)
		}
	}
	for _, p := range m.Permissions.Run {
		if p == "" {
			continue
		}
		if p == "device.location" {
			// Location lives in the Run-list until ViewPermissions grows a
			// typed slot; render it under its semantic name rather than as
			// an opaque run entry.
			continue
		}
		out = append(out, "run: "+p)
	}
	if m.Permissions.Filesystem {
		out = append(out, "filesystem")
	}
	if m.Permissions.Network {
		out = append(out, "network")
	}
	if hasManifestStorePermission(m) {
		out = append(out, "store")
	}
	if m.Permissions.Notifications {
		out = append(out, "notifications")
	}
	if m.Permissions.Clipboard {
		out = append(out, "clipboard")
	}
	if m.Permissions.Camera {
		out = append(out, "camera")
	}
	if m.Permissions.Microphone {
		out = append(out, "microphone")
	}
	for _, p := range m.Permissions.Run {
		if p == "device.location" {
			out = append(out, "location")
			break
		}
	}
	return out
}

// PkgRemove deletes an installed package directory. Matches the RFC §16.3
// command — removes `<home>/.core/apps/<name>/` recursively.
//
//	err := app.PkgRemove(coreio.Local, home, "bitwarden-clients")
//
// Rules:
//
//   - Name must be a simple slug (no path separators). Anything else
//     is rejected before any filesystem write.
//
//   - Missing packages return a typed error — the CLI can print a
//     friendly "not installed" message instead of silently succeeding.
//
// Equivalent to PkgRemoveWith with a zero PkgRemoveOptions; callers that
// want to delete the workspace data tree at the same time should use
// PkgRemoveWith with Purge=true.
func PkgRemove(medium coreio.Medium, home, name string) error {
	return PkgRemoveWith(medium, home, name, PkgRemoveOptions{})
}

// PkgRemoveOptions tunes PkgRemoveWith. A zero value mirrors the
// historical PkgRemove behaviour (install tree only). Purge also wipes
// `$DIR_HOME/.core/data/<name>/` so a fresh reinstall starts clean.
//
//	err := app.PkgRemoveWith(coreio.Local, home, "bitwarden-clients", app.PkgRemoveOptions{Purge: true})
type PkgRemoveOptions struct {
	// Purge removes the workspace data tree
	// (`$DIR_HOME/.core/data/<name>/`) alongside the install tree.
	// Matches `core pkg remove --purge` and the workspace.Wipe
	// convention referenced by RFC §16.3.
	Purge bool
}

// PkgRemoveWith is the options-aware entry point for `core pkg remove`.
// Same validation as PkgRemove but also honours the Purge flag so a CLI
// invocation can drop the persisted data tree in one call instead of
// running a manual workspace Wipe afterwards.
//
//	err := app.PkgRemoveWith(coreio.Local, home, "bitwarden-clients",
//	    app.PkgRemoveOptions{Purge: true})
//
// Rules:
//
//   - Missing install tree (and missing workspace when Purge) → typed
//     error so the CLI can differentiate "not installed" from a real
//     failure.
//
//   - Purge failures surface the workspace error directly so the caller
//     can tell the install tree removal succeeded before the data tree
//     cleanup hit a problem.
func PkgRemoveWith(medium coreio.Medium, home, name string, opts PkgRemoveOptions) error {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return coreerr.E("app.PkgRemoveWith", "empty home directory", nil)
	}
	if name == "" || core.Contains(name, "/") || core.Contains(name, "\\") {
		return coreerr.E("app.PkgRemoveWith", "invalid package name: "+name, nil)
	}
	appPath := core.Path(home, ".core", AppsDirName, name)
	if !medium.IsDir(appPath) {
		return coreerr.E("app.PkgRemoveWith", "package not installed: "+name, nil)
	}
	if err := medium.DeleteAll(appPath); err != nil {
		return coreerr.E("app.PkgRemoveWith", "delete failed: "+appPath, err)
	}
	if opts.Purge {
		dataPath := core.Path(home, ".core", DataDirName, name)
		if medium.IsDir(dataPath) {
			if err := medium.DeleteAll(dataPath); err != nil {
				return coreerr.E("app.PkgRemoveWith", "purge data tree failed: "+dataPath, err)
			}
		}
	}
	return nil
}

// PkgInstallOptions tunes PkgInstall. The zero value installs into
// `$DIR_HOME/.core/apps/<code>/` using the default medium.
//
//	opts := app.PkgInstallOptions{Home: "/Users/me", Force: true}
type PkgInstallOptions struct {
	// Home overrides the base directory. Defaults to $DIR_HOME.
	Home string
	// Force replaces an existing install of the same code.
	Force bool
	// Source records the provenance in the installed manifest's
	// Config["source"] field. Typical values: "marketplace",
	// "github.com/user/repo", "https://app.example.com".
	Source string
	// AssetSource optionally points at a directory whose contents should
	// be copied into the installed app root before `.core/view.yaml` is
	// written. Web and Electron wraps use this so the install contains
	// the renderer assets as well as the generated manifest.
	AssetSource string
}

// InstallWrappedPWA persists a wrapped PWA manifest into the installed
// apps tree at `<home>/.core/apps/<code>/.core/view.yaml`. Used by
// `core pkg install <pwa-url>` after WrapPWA has built the manifest.
//
//	err := app.InstallWrappedPWA(coreio.Local, manifest,
//	    app.PkgInstallOptions{Home: "/Users/me", Source: "marketplace"})
func InstallWrappedPWA(medium coreio.Medium, manifest *config.ViewManifest, opts PkgInstallOptions) (string, error) {
	return installWrap(medium, manifest, opts)
}

// InstallWrappedElectron persists a wrapped Electron manifest into the
// installed apps tree. Mirrors InstallWrappedPWA for the Electron case.
//
//	path, err := app.InstallWrappedElectron(coreio.Local, manifest, opts)
func InstallWrappedElectron(medium coreio.Medium, manifest *config.ViewManifest, opts PkgInstallOptions) (string, error) {
	return installWrap(medium, manifest, opts)
}

// InstallWrappedWeb persists a wrapped plain-web manifest into the
// installed apps tree.
//
//	path, err := app.InstallWrappedWeb(coreio.Local, manifest, opts)
func InstallWrappedWeb(medium coreio.Medium, manifest *config.ViewManifest, opts PkgInstallOptions) (string, error) {
	return installWrap(medium, manifest, opts)
}

// installWrap is the shared body for the three installer paths. The
// three public entry points exist so the CLI error messages can name
// the package type without any reflection.
//
//	dest, err := installWrap(medium, manifest, opts)
func installWrap(medium coreio.Medium, manifest *config.ViewManifest, opts PkgInstallOptions) (string, error) {
	if manifest == nil {
		return "", coreerr.E("app.installWrap", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	if manifest.Code == "" {
		return "", coreerr.E("app.installWrap", "manifest.code is empty", nil)
	}

	home := opts.Home
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return "", coreerr.E("app.installWrap", "cannot resolve home directory", nil)
	}

	dest := core.Path(home, ".core", AppsDirName, manifest.Code)
	if medium.IsDir(dest) {
		if !opts.Force {
			return dest, coreerr.E(
				"app.installWrap",
				"already installed at "+dest+" (use Force to replace)",
				nil,
			)
		}
		if err := medium.DeleteAll(dest); err != nil {
			return dest, coreerr.E("app.installWrap", "remove existing failed", err)
		}
	}
	// Record provenance in Config so `core pkg list` can show it.
	if opts.Source != "" {
		if manifest.Config == nil {
			manifest.Config = map[string]any{}
		}
		manifest.Config["source"] = opts.Source
	}
	if err := WriteWrappedApp(medium, dest, manifest, opts.AssetSource); err != nil {
		return dest, coreerr.E("app.installWrap", "materialise wrap failed", err)
	}
	return dest, nil
}

// WriteWrappedApp materialises a wrapped app at `dest`, optionally
// copying a directory of renderer assets first and then writing the
// generated `.core/view.yaml`.
func WriteWrappedApp(medium coreio.Medium, dest string, manifest *config.ViewManifest, assetSource string) error {
	if manifest == nil {
		return coreerr.E("app.WriteWrappedApp", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	if dest == "" {
		return coreerr.E("app.WriteWrappedApp", "empty dest", nil)
	}
	if assetSource != "" {
		if !medium.IsDir(assetSource) {
			return coreerr.E(
				"app.WriteWrappedApp",
				"asset source is not a directory: "+assetSource,
				nil,
			)
		}
		if err := copyTree(medium, assetSource, dest); err != nil {
			return coreerr.E("app.WriteWrappedApp", "copy asset tree failed", err)
		}
	}

	path := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return coreerr.E("app.WriteWrappedApp", "ensure dir failed", err)
	}
	body, err := yamlMarshalBytes(manifest)
	if err != nil {
		return coreerr.E("app.WriteWrappedApp", "marshal failed", err)
	}
	if err := medium.Write(path, string(body)); err != nil {
		return coreerr.E("app.WriteWrappedApp", "write failed", err)
	}
	return nil
}

// PkgUpdate re-reads an installed package's source (when remembered in
// Config["source"]) and re-wraps it. PWA sources (`wrap:pwa:<url>` or
// `marketplace:<code>` resolved as PWA) trigger a fresh fetch + wrap;
// other source types return the install path so the caller can decide
// what to do.
//
// The function returns the fully-qualified destination path on success
// even when only the manifest was touched, so the caller can surface a
// consistent "updated at <path>" message.
//
//	path, err := app.PkgUpdate(ctx, coreio.Local, "/Users/me", "my-web-app")
func PkgUpdate(ctx context.Context, medium coreio.Medium, home, name string) (string, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return "", coreerr.E("app.PkgUpdate", "empty home directory", nil)
	}
	if name == "" {
		return "", coreerr.E("app.PkgUpdate", "empty package name", nil)
	}

	appPath := core.Path(home, ".core", AppsDirName, name)
	viewPath := core.Path(appPath, ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		return "", coreerr.E("app.PkgUpdate", "package not installed: "+name, nil)
	}

	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, viewPath, &manifest); err != nil {
		return "", coreerr.E("app.PkgUpdate", "parse manifest failed", err)
	}

	var source string
	if manifest.Config != nil {
		if s, ok := manifest.Config["source"].(string); ok {
			source = s
		}
	}
	if source == "" {
		return "", coreerr.E("app.PkgUpdate", "no source recorded for "+name, nil)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// `wrap:pwa:<url>` is the convention recorded by `core pkg wrap
	// --pwa` — re-fetch the manifest and rewrite the install in place.
	if url, ok := stripPrefix(source, "wrap:pwa:"); ok {
		pwa, err := loadPWASource(ctx, medium, url)
		if err != nil {
			return appPath, coreerr.E("app.PkgUpdate", "PWA refetch failed", err)
		}
		updated := WrapPWA(pwa, WrapPWAOptions{
			TargetURL: url,
			Code:      manifest.Code,
			Version:   manifest.Version,
		})
		if updated == nil {
			return appPath, coreerr.E("app.PkgUpdate", "WrapPWA returned nil", nil)
		}
		_, err = installWrap(medium, updated, PkgInstallOptions{
			Home:   home,
			Force:  true,
			Source: source,
		})
		if err != nil {
			return appPath, err
		}
		return appPath, nil
	}

	// `wrap:web:<dir>` — re-wrap the local web directory and reinstall.
	// The user's source directory may have changed (new index.html etc.)
	// so we re-run WrapWeb against the recorded path.
	if dir, ok := stripPrefix(source, "wrap:web:"); ok {
		if !medium.IsDir(dir) {
			return appPath, coreerr.E(
				"app.PkgUpdate",
				"web source directory missing: "+dir+" (recorded by `pkg wrap --web`)",
				nil,
			)
		}
		updated, err := WrapWeb(medium, dir, WrapWebOptions{
			Code:    manifest.Code,
			Name:    manifest.Name,
			Version: manifest.Version,
		})
		if err != nil {
			return appPath, coreerr.E("app.PkgUpdate", "web rewrap failed", err)
		}
		if _, err := installWrap(medium, updated, PkgInstallOptions{
			Home:        home,
			Force:       true,
			Source:      source,
			AssetSource: dir,
		}); err != nil {
			return appPath, err
		}
		return appPath, nil
	}

	// `wrap:electron:<dir|repo>` — for local directories we re-scan the
	// renderer and rebuild the manifest in place. Repo-based sources
	// require the user to point at the unpacked directory because the
	// release-asset download is not idempotent here.
	if dir, ok := stripPrefix(source, "wrap:electron:"); ok {
		if !medium.IsDir(dir) {
			if _, _, _, ok := ParseGitHubRepo(dir); ok {
				installed, err := installElectronRepoSource(ctx, medium, home, manifest.Code, manifest.Name, manifest.Version, dir, true)
				if err != nil {
					return appPath, err
				}
				return installed, nil
			}
			return appPath, coreerr.E(
				"app.PkgUpdate",
				"electron source not a directory: "+dir+" (re-extract the release first)",
				nil,
			)
		}
		pkgPath := core.Path(dir, "package.json")
		if !medium.Exists(pkgPath) {
			return appPath, coreerr.E(
				"app.PkgUpdate",
				"package.json missing under electron source: "+pkgPath,
				nil,
			)
		}
		body, err := medium.Read(pkgPath)
		if err != nil {
			return appPath, coreerr.E("app.PkgUpdate", "read package.json failed", err)
		}
		var pkg ElectronPackageJSON
		r := core.JSONUnmarshal([]byte(body), &pkg)
		if !r.OK {
			cause, _ := r.Value.(error)
			return appPath, coreerr.E("app.PkgUpdate", "decode package.json failed", cause)
		}
		scan, err := ScanElectronRenderer(medium, dir)
		if err != nil {
			return appPath, coreerr.E("app.PkgUpdate", "scan renderer failed", err)
		}
		updated := WrapElectron(&pkg, scan, WrapElectronOptions{Code: manifest.Code})
		if updated == nil {
			return appPath, coreerr.E("app.PkgUpdate", "WrapElectron returned nil", nil)
		}
		updated.Version = manifest.Version
		if updated.Name == "" {
			updated.Name = manifest.Name
		}
		if _, err := installWrap(medium, updated, PkgInstallOptions{
			Home:        home,
			Force:       true,
			Source:      source,
			AssetSource: dir,
		}); err != nil {
			return appPath, err
		}
		return appPath, nil
	}

	// `local:<path>` — copy the source tree over the install. Used by
	// `pkg install <local-dir>` so the operator can iterate without
	// re-running install manually.
	if path, ok := stripPrefix(source, "local:"); ok {
		if !medium.IsDir(path) {
			return appPath, coreerr.E(
				"app.PkgUpdate",
				"local source missing: "+path,
				nil,
			)
		}
		if _, err := PkgInstallLocal(medium, path, PkgInstallOptions{
			Home:   home,
			Force:  true,
			Source: source,
		}); err != nil {
			return appPath, err
		}
		return appPath, nil
	}

	// `marketplace:<code>` and any other remaining sources hand back the
	// install path; the CLI can dispatch into MarketplaceUpdate which
	// owns the git pull / re-verify pipeline.
	return appPath, nil
}

// loadPWASource resolves a `wrap:pwa:` source into a PWAManifest. URLs
// are fetched over HTTP(S); local paths and file:// URLs are read from
// disk so a wrapped local PWA can be updated without a local web
// server.
func loadPWASource(ctx context.Context, medium coreio.Medium, source string) (*PWAManifest, error) {
	if medium == nil {
		medium = coreio.Local
	}
	path := trimLocalPrefix(source)
	if isLocalSource(source) || medium.Exists(path) {
		body, err := medium.Read(path)
		if err != nil {
			return nil, coreerr.E("app.loadPWASource", "read local manifest failed", err)
		}
		var pwa PWAManifest
		r := core.JSONUnmarshal([]byte(body), &pwa)
		if !r.OK {
			cause, _ := r.Value.(error)
			return nil, coreerr.E("app.loadPWASource", "decode local manifest failed", cause)
		}
		return &pwa, nil
	}
	return FetchPWAManifest(ctx, source)
}

// installElectronRepoSource wraps the latest renderer asset from a
// remote Electron repo and installs it as a CoreApp, recording the repo
// reference as the package source for future updates.
func installElectronRepoSource(ctx context.Context, medium coreio.Medium, home, code, name, version, ref string, force bool) (string, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return "", coreerr.E("app.installElectronRepoSource", "cannot resolve home directory", nil)
	}

	scratch := core.Path(home, ".core", ".wrap", "repo-"+slugify(coalesce(code, ref)))
	manifest, rendererDir, err := WrapElectronRepo(ctx, medium, ref, WrapElectronRepoOptions{
		Code:       code,
		Name:       name,
		Version:    version,
		ScratchDir: scratch,
	})
	if err != nil {
		return "", coreerr.E("app.installElectronRepoSource", "wrap repo failed", err)
	}
	installed, err := InstallWrappedElectron(medium, manifest, PkgInstallOptions{
		Home:        home,
		Force:       force,
		Source:      "wrap:electron:" + ref,
		AssetSource: rendererDir,
	})
	if medium.IsDir(scratch) {
		_ = medium.DeleteAll(scratch)
	}
	if err != nil {
		return installed, coreerr.E("app.installElectronRepoSource", "install failed", err)
	}
	return installed, nil
}

// stripPrefix returns (rest, true) when `s` starts with `prefix`. Used
// by PkgUpdate to peel `wrap:pwa:` etc. without bringing strings.
//
//	rest, ok := stripPrefix("wrap:pwa:https://x", "wrap:pwa:") // "https://x", true
func stripPrefix(s, prefix string) (string, bool) {
	if !core.HasPrefix(s, prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// PkgInstallSpec describes a single `core pkg install` argument. The
// Source string is whatever the user typed (`vendor/repo`, a full URL,
// or a marketplace code) and Type is the auto-detected kind.
//
//	spec := app.ParseInstallSpec("https://app.example.com")
//	spec.Type // PackageTypePWA
type PkgInstallSpec struct {
	Source string      // raw user input
	Type   PackageType // auto-detected kind
	URL    string      // populated for PWA / web-fetch installs
	Repo   string      // populated for github.com/owner/name electron installs
	Code   string      // populated for marketplace lookups
	Path   string      // populated for local-directory installs
}

// ParseInstallSpec inspects the `core pkg install <source>` argument
// and produces a typed PkgInstallSpec the installer can dispatch on.
// Matches RFC §16.4 detection table for non-local sources (URL → PWA,
// github.com/... → Electron, plain code → marketplace lookup).
//
//	spec := app.ParseInstallSpec("github.com/bitwarden/clients")
//	if spec.Type == app.PackageTypeElectron { /* fetch release */ }
//
// Detection precedence:
//
//  1. `https://` / `http://`      → PWA (manifest fetch)
//  2. `github.com/` / `gitlab.com/` / `git@` → Electron (release fetch)
//  3. `file://`, `./`, `../`, `/` → Local directory; ParseInstallSpec
//     records the path and leaves Type=PackageTypeUnknown so the caller
//     can DetectPackageType against the directory.
//  4. anything else                → marketplace listing (code lookup)
func ParseInstallSpec(in string) PkgInstallSpec {
	src := core.Trim(in)
	if src == "" {
		return PkgInstallSpec{Type: PackageTypeUnknown}
	}
	spec := PkgInstallSpec{Source: src}

	switch {
	case core.HasPrefix(src, "https://"), core.HasPrefix(src, "http://"):
		spec.Type = PackageTypePWA
		spec.URL = src
	case core.HasPrefix(src, "github.com/"), core.HasPrefix(src, "gitlab.com/"), core.HasPrefix(src, "git@"):
		spec.Type = PackageTypeElectron
		spec.Repo = src
	case isLocalSource(src):
		// Local install — caller runs DetectPackageType against the
		// path to decide between native, pwa, electron and web. We set
		// Type to Unknown so the dispatcher knows to call the detector.
		spec.Type = PackageTypeUnknown
		spec.Path = trimLocalPrefix(src)
	default:
		spec.Type = PackageTypeNative // marketplace listing — type confirmed at install time
		spec.Code = src
	}
	return spec
}

// isLocalSource reports whether `s` references a directory on the
// local filesystem (file URL, relative or absolute path). Marketplace
// codes that happen to start with `.` are not common, so the relative
// patterns are reasonably exclusive.
//
//	isLocalSource("./my-app")              // true
//	isLocalSource("file:///srv/app")       // true
//	isLocalSource("photo-browser")         // false
func isLocalSource(s string) bool {
	if s == "" {
		return false
	}
	switch {
	case core.HasPrefix(s, "file://"),
		core.HasPrefix(s, "./"),
		core.HasPrefix(s, "../"),
		s == ".", s == "..":
		return true
	}
	if core.PathIsAbs(s) {
		return true
	}
	return false
}

// trimLocalPrefix strips the `file://` scheme so the caller has a plain
// filesystem path to walk.
//
//	trimLocalPrefix("file:///srv/app") // "/srv/app"
//	trimLocalPrefix("./my-app")        // "./my-app"
func trimLocalPrefix(s string) string {
	if core.HasPrefix(s, "file://") {
		return s[len("file://"):]
	}
	return s
}

// PkgInstallLocal copies a local directory into
// `<home>/.core/apps/<code>/`. The source must already be a CoreApp
// (have a `.core/view.yaml`); for PWA/Electron/Web wraps run the
// matching `pkg wrap` command first then point the install at the
// wrapped output.
//
//	dest, err := app.PkgInstallLocal(coreio.Local, src, app.PkgInstallOptions{Home: home})
//
// Rules:
//
//   - The source manifest's Code is used for the destination directory
//     name.
//
//   - Existing installs are rejected unless Force=true.
//
//   - The function uses the medium's Read/Write rather than a syscall
//     copy so MockMedium and MemoryMedium round-trip cleanly in tests.
func PkgInstallLocal(medium coreio.Medium, src string, opts PkgInstallOptions) (string, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if src == "" {
		return "", coreerr.E("app.PkgInstallLocal", "empty source", nil)
	}
	if !medium.IsDir(src) {
		return "", coreerr.E("app.PkgInstallLocal", "source is not a directory: "+src, nil)
	}

	manifestPath := core.Path(src, ".core", "view.yaml")
	if !medium.Exists(manifestPath) {
		return "", coreerr.E(
			"app.PkgInstallLocal",
			"source has no .core/view.yaml: "+src,
			nil,
		)
	}

	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, manifestPath, &manifest); err != nil {
		return "", coreerr.E("app.PkgInstallLocal", "parse source manifest failed", err)
	}
	if manifest.Code == "" {
		return "", coreerr.E("app.PkgInstallLocal", "source manifest.code is empty", nil)
	}

	home := opts.Home
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return "", coreerr.E("app.PkgInstallLocal", "cannot resolve home directory", nil)
	}

	dest := core.Path(home, ".core", AppsDirName, manifest.Code)
	if medium.IsDir(dest) {
		if !opts.Force {
			return dest, coreerr.E(
				"app.PkgInstallLocal",
				"already installed at "+dest+" (use Force to replace)",
				nil,
			)
		}
		if err := medium.DeleteAll(dest); err != nil {
			return dest, coreerr.E("app.PkgInstallLocal", "remove existing failed", err)
		}
	}

	if err := copyTree(medium, src, dest); err != nil {
		return dest, coreerr.E("app.PkgInstallLocal", "copy tree failed", err)
	}

	// Stamp the source so `pkg list` shows where the install came from.
	if opts.Source == "" {
		opts.Source = "local:" + src
	}
	if err := stampSource(medium, dest, opts.Source); err != nil {
		// Best-effort; the install itself succeeded.
		_ = err
	}
	return dest, nil
}

// copyTree recursively copies every file beneath `src` into `dest`,
// preserving the directory layout. Directories are created via
// EnsureDir so missing intermediate folders are not an error.
//
//	err := copyTree(medium, src, dest)
func copyTree(medium coreio.Medium, src, dest string) error {
	if medium == nil {
		medium = coreio.Local
	}
	if err := medium.EnsureDir(dest); err != nil {
		return err
	}
	entries, err := medium.List(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}
		srcPath := core.Path(src, name)
		dstPath := core.Path(dest, name)
		if e.IsDir() {
			if err := copyTree(medium, srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		body, err := medium.Read(srcPath)
		if err != nil {
			return err
		}
		if err := medium.Write(dstPath, body); err != nil {
			return err
		}
	}
	return nil
}
