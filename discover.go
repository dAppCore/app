// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// discover is Step 1 of the 7-step boot — locate `.core/view.yaml` by
// walking upward from the start directory and parse it into a
// ViewManifest. Delegates discovery to core/config (CoreDirs,
// FindManifest, LoadManifest) so the `.core/` walk rules stay in one
// place.
//
//	manifest, root, err := discover(coreio.Local, "./")
//
// Returns:
//   - manifest: the parsed view.yaml contents
//   - root:     the project directory (parent of the .core/ that won)
//   - err:      coreerr.E-wrapped if the walk produces nothing or parse fails
func discover(medium coreio.Medium, start string) (config.ViewManifest, string, error) {
	if medium == nil {
		medium = coreio.Local
	}

	// FindManifest walks .core/ directories upward from start and returns
	// the first match. Empty string means no view.yaml anywhere on the
	// path — the caller is not inside a CoreApp.
	path := config.FindManifest(medium, start, config.FileView)
	if path == "" {
		return config.ViewManifest{}, "", coreerr.E(
			"app.discover",
			"no .core/view.yaml found walking up from "+start,
			nil,
		)
	}

	var manifest config.ViewManifest
	if err := config.LoadManifest(medium, path, &manifest); err != nil {
		return config.ViewManifest{}, "", coreerr.E(
			"app.discover",
			"failed to parse "+path,
			err,
		)
	}

	// Root = parent of the .core/ directory that held view.yaml.
	// path is .../<root>/.core/view.yaml → dirname twice = root.
	root := core.PathDir(core.PathDir(path))

	return manifest, root, nil
}

// discoverCompiled is the prod-mode counterpart to `discover` — it
// searches for `core.json` walking upward from start and, if found,
// hydrates a ViewManifest from the compiled artifact. Falls back to
// the YAML discover path when no core.json exists on the walk.
//
// The RFC §4.1 boot sequence reads core.json in prod mode and
// .core/view.yaml in dev mode; this helper centralises that preference
// without leaking compile/runtime shape details into discover.
//
//	manifest, root, err := discoverCompiled(coreio.Local, "./", ModeProd)
func discoverCompiled(medium coreio.Medium, start string, mode Mode) (config.ViewManifest, string, error) {
	if medium == nil {
		medium = coreio.Local
	}

	if mode == ModeDev {
		// Developers iterate on view.yaml; never read a stale core.json.
		return discover(medium, start)
	}

	// Walk upward looking for core.json at the project root. If we hit
	// a .core/ first, that directory's parent is the natural root.
	dirs := config.CoreDirs(medium, start)
	for _, coreDir := range dirs {
		root := core.PathDir(coreDir)
		if path := core.Path(root, CompiledFileName); medium.Exists(path) {
			cm, err := LoadCompiled(medium, root)
			if err != nil {
				return config.ViewManifest{}, "", coreerr.E(
					"app.discoverCompiled",
					"failed to parse "+path,
					err,
				)
			}
			return compiledToManifest(cm), root, nil
		}
	}

	// No core.json on the walk — fall back to view.yaml so operators
	// who only have a source tree can still boot.
	return discover(medium, start)
}

// compiledToManifest projects a CompiledManifest back into the
// ViewManifest shape the rest of the boot pipeline already consumes.
// Slots go from string → any (the YAML-flexible shape) so the layout
// step re-uses the same validator without special-casing the origin.
//
// Config is copied through verbatim so the template renderer (RFC §4.1
// step 6) and the Config-backed permission flags (`store`, `write`,
// `services`) behave identically whether the runtime booted from
// `.core/view.yaml` or from the compiled `core.json`.
//
//	view := compiledToManifest(cm)
func compiledToManifest(cm *CompiledManifest) config.ViewManifest {
	if cm == nil {
		return config.ViewManifest{}
	}
	var slots map[string]any
	if len(cm.Slots) > 0 {
		slots = make(map[string]any, len(cm.Slots))
		for k, v := range cm.Slots {
			slots[k] = v
		}
	}
	var cfg map[string]any
	if len(cm.Config) > 0 {
		cfg = make(map[string]any, len(cm.Config))
		for k, v := range cm.Config {
			cfg[k] = v
		}
	}
	return config.ViewManifest{
		Code:        cm.Code,
		Name:        cm.Name,
		Version:     cm.Version,
		Sign:        cm.Sign,
		Layout:      cm.Layout,
		Slots:       slots,
		Modules:     append([]string(nil), cm.Modules...),
		Permissions: cm.Permissions,
		Config:      cfg,
	}
}

// DiscoverInstalled resolves an installed package code to its
// directory under `<home>/.core/apps/<code>/` and returns the absolute
// path. Used by `core run <app-code>` (CLI side) and any host that
// needs to locate a package without the user pointing at it.
//
//	dir, err := app.DiscoverInstalled(coreio.Local, home, "photo-browser")
//
// Rules:
//
//   - Empty code → typed error.
//
//   - Empty home → falls back to `$DIR_HOME`.
//
//   - Package missing → typed error naming the expected location so the
//     CLI message points the user at the right `pkg install` command.
func DiscoverInstalled(medium coreio.Medium, home, code string) (string, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if code == "" {
		return "", coreerr.E("app.DiscoverInstalled", "empty code", nil)
	}
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return "", coreerr.E("app.DiscoverInstalled", "cannot resolve home directory", nil)
	}

	dir := core.Path(home, ".core", AppsDirName, code)
	if !medium.IsDir(dir) {
		return "", coreerr.E(
			"app.DiscoverInstalled",
			"package not installed: "+code+" (expected "+dir+")",
			nil,
		)
	}
	return dir, nil
}

// DiscoverInstalledManifest hydrates the ViewManifest of an installed
// package by combining DiscoverInstalled with the regular manifest
// loader. Convenience wrapper for hosts that want one call to go from
// "code" to "loaded manifest" without the intermediate path step.
//
//	manifest, dir, err := app.DiscoverInstalledManifest(coreio.Local, home, "photo-browser")
func DiscoverInstalledManifest(medium coreio.Medium, home, code string) (config.ViewManifest, string, error) {
	dir, err := DiscoverInstalled(medium, home, code)
	if err != nil {
		return config.ViewManifest{}, "", err
	}
	manifest, _, err := discover(medium, dir)
	if err != nil {
		return config.ViewManifest{}, dir, err
	}
	return manifest, dir, nil
}
