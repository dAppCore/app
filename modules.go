// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// modules is Step 4 of the 7-step boot — load the modules declared in
// the manifest onto the Core container. Each entry in `manifest.modules`
// names a Core module (e.g. "core/media", "core/fs") whose Register
// function should have been wired through core.WithService() by the
// host binary.
//
//	err := modules(ctx, c, &manifest)
//
// TODO(app): the RFC mandates a module registry so the manifest list
// resolves to Service factories without the host having to pre-register
// every possible module. Options:
//
//  1. core.ModuleRegistry — a map[string]core.ServiceFactory that hosts
//     populate at init; this step looks up and applies.
//  2. Go plugin/soname loading (plugin.Open) — heavier, brings dlopen.
//  3. Deno sidecar modules (ts RFC §CoreDeno) — TypeScript modules loaded
//     into the CoreTS runtime, not the Go process.
//
// Until the registry lands this step is a strict name-check: it records
// the requested modules and errors if a prod boot asks for modules we
// cannot resolve. Dev mode logs and continues.
func modules(_ context.Context, c *core.Core, m *config.ViewManifest) error {
	if c == nil {
		return coreerr.E("app.modules", "nil core", nil)
	}
	if m == nil {
		return coreerr.E("app.modules", "nil manifest", nil)
	}
	if len(m.Modules) == 0 {
		return nil
	}

	missing := resolveModules(c, m.Modules)
	if len(missing) == 0 {
		return nil
	}

	// At the keystone-skeleton stage the module registry does not yet
	// exist — see function doc. Return a typed error so the caller can
	// decide whether to treat missing modules as fatal.
	return coreerr.E(
		"app.modules",
		"cannot resolve "+core.Sprint(len(missing))+" declared module(s): "+core.Join(", ", missing...),
		nil,
	)
}

// resolveModules checks which of the manifest's declared module names
// the Core container already knows about. Returns the names that remain
// unresolved so the caller can log / error / fall back.
//
//	missing := resolveModules(c, []string{"core/media", "core/fs"})
func resolveModules(c *core.Core, names []string) []string {
	var missing []string
	for _, name := range names {
		if !moduleResolved(c, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

// moduleResolved is the single decision point for "do we know this
// module?" — kept as its own function so the registry lookup can be
// swapped in without touching resolveModules.
//
// TODO(app): query c.RegistryOf("modules") once the registry is live.
// For now every module is unresolved.
func moduleResolved(_ *core.Core, _ string) bool {
	return false
}
