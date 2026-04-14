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
