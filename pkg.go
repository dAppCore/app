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
//	    fmt.Println(e.Name, e.Type, e.Version, e.Source)
//	}
type PkgEntry struct {
	Name    string      // manifest.code — the user-facing slug
	Type    PackageType // native | pwa | electron | web
	Version string      // manifest.version
	Source  string      // marketplace | https://... | github.com/...
	Path    string      // absolute install path (<home>/.core/apps/<code>)
}

// PkgList returns every installed package by scanning
// `<home>/.core/apps/*/.core/view.yaml`. Missing directories are not
// errors — a fresh user with no installs gets an empty slice.
//
//	entries, err := app.PkgList(coreio.Local, "/Users/me")
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
	return out, nil
}

// pkgEntryFromManifest loads a view.yaml and projects it into a PkgEntry.
// The helper reads the optional `type` and `source` keys from the
// Config map (where PWA/Electron wraps stash their provenance).
//
//	pe, err := pkgEntryFromManifest(medium, viewPath, appPath)
func pkgEntryFromManifest(medium coreio.Medium, viewPath, appPath string) (PkgEntry, error) {
	var manifest config.ViewManifest
	if err := config.LoadManifest(medium, viewPath, &manifest); err != nil {
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
	}
	if entry.Source == "" {
		entry.Source = "local"
	}
	return entry, nil
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
func PkgRemove(medium coreio.Medium, home, name string) error {
	if medium == nil {
		medium = coreio.Local
	}
	if home == "" {
		return coreerr.E("app.PkgRemove", "empty home directory", nil)
	}
	if name == "" || core.Contains(name, "/") || core.Contains(name, "\\") {
		return coreerr.E("app.PkgRemove", "invalid package name: "+name, nil)
	}
	appPath := core.Path(home, ".core", AppsDirName, name)
	if !medium.IsDir(appPath) {
		return coreerr.E("app.PkgRemove", "package not installed: "+name, nil)
	}
	if err := medium.DeleteAll(appPath); err != nil {
		return coreerr.E("app.PkgRemove", "delete failed: "+appPath, err)
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

	path := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return dest, coreerr.E("app.installWrap", "ensure dir failed", err)
	}

	body, err := yaml.Marshal(manifest)
	if err != nil {
		return dest, coreerr.E("app.installWrap", "marshal failed", err)
	}
	if err := medium.Write(path, string(body)); err != nil {
		return dest, coreerr.E("app.installWrap", "write failed", err)
	}
	return dest, nil
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
	if err := config.LoadManifest(medium, viewPath, &manifest); err != nil {
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

	// `wrap:pwa:<url>` is the convention recorded by `core pkg wrap
	// --pwa` — re-fetch the manifest and rewrite the install in place.
	if url, ok := stripPrefix(source, "wrap:pwa:"); ok {
		if ctx == nil {
			ctx = context.Background()
		}
		pwa, err := FetchPWAManifest(ctx, url)
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

	// Non-PWA sources (`wrap:web:`, `wrap:electron:`, `marketplace:`)
	// hand back the install path; the CLI can re-invoke the original
	// fetch path with the recorded source argument.
	return appPath, nil
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
}

// ParseInstallSpec inspects the `core pkg install <source>` argument
// and produces a typed PkgInstallSpec the installer can dispatch on.
// Matches RFC §16.4 detection table for non-local sources (URL → PWA,
// github.com/... → Electron, plain code → marketplace lookup).
//
//	spec := app.ParseInstallSpec("github.com/bitwarden/clients")
//	if spec.Type == app.PackageTypeElectron { /* fetch release */ }
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
	default:
		spec.Type = PackageTypeNative // marketplace listing — type confirmed at install time
		spec.Code = src
	}
	return spec
}
