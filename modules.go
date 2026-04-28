// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// modules is Step 4 of the 7-step boot — load the modules declared in
// the manifest onto the Core container. Each entry in `manifest.modules`
// names a Core module (e.g. "core/media", "core/fs"); the host
// pre-registers a ModuleFactory for every name it supports via
// `app.RegisterModule`. This step resolves those factories, applies
// them to the container and surfaces a typed error listing any module
// the host did not advertise.
//
//	app.RegisterModule("core/media", func() core.CoreOption {
//	    return core.WithService(media.Register)
//	})
//	err := modules(ctx, c, &manifest)
//
// Mode handling:
//
//   - Prod: any unresolved module is fatal — the manifest asked for a
//     capability the host cannot provide.
//
//   - Dev: unresolved modules are logged via core.Warn and the boot
//     keeps going so the developer can iterate on a partial host.
func modules(ctx context.Context, c *core.Core, m *config.ViewManifest) error {
	return loadModules(ctx, c, m, ModeProd)
}

// modulesWithMode mirrors modules() but lets the caller pick the
// regime. Used by tests and by `PluginBoot` whose mode preference may
// not match the package-level Boot path.
//
//	err := modulesWithMode(ctx, c, &manifest, ModeDev)
func modulesWithMode(ctx context.Context, c *core.Core, m *config.ViewManifest, mode Mode) error {
	return loadModules(ctx, c, m, mode)
}

// resolveModules checks which of the manifest's declared module names
// the host's package registry knows about. Returns the names that
// remain unresolved so the caller can log / error / fall back. Kept
// for backwards-compatibility with code that pre-dates the registry.
//
//	missing := resolveModules(c, []string{"core/media", "core/fs"})
func resolveModules(_ *core.Core, names []string) []string {
	_, missing := resolveModulesFromRegistry(names)
	return missing
}

// moduleResolved reports whether the supplied module name is known to
// the package registry. Single decision point so the registry lookup
// stays in one place; modules() and resolveModules both go through it.
//
//	if moduleResolved(c, "core/media") { /* available */ }
func moduleResolved(_ *core.Core, name string) bool {
	_, ok := LookupModule(name)
	return ok
}
