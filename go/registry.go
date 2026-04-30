// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// ModuleFactory returns a CoreOption that wires a named module onto a
// Core container. Hosts register every module they support once at
// startup; the registry then resolves manifest-declared module names
// during Step 4 (Modules) of the boot pipeline.
//
//	app.RegisterModule("core/media", func() core.CoreOption {
//	    return core.WithService(media.Register)
//	})
type ModuleFactory func() core.CoreOption

// moduleRegistry is the package-level table mapping `manifest.modules`
// names → factories. Hosts populate it via RegisterModule before
// calling Boot. Lookup is read-mostly so a sync.RWMutex keeps the hot
// path lock-free in steady state.
var moduleRegistry = struct {
	mu sync.RWMutex
	m  map[string]ModuleFactory
}{m: map[string]ModuleFactory{}}

// RegisterModule inserts a module name → factory mapping into the
// process-wide registry. Called once per supported module — typically
// at host init time.
//
//	func init() {
//	    app.RegisterModule("core/media", func() core.CoreOption {
//	        return core.WithService(media.Register)
//	    })
//	}
//
// Rules:
//
//   - Empty name → no-op (silently ignored to keep init code simple).
//
//   - Re-registering the same name overwrites the previous factory —
//     the host owns the registry and a deliberate override matters
//     more than a "first wins" rule.
func RegisterModule(name string, factory ModuleFactory) {
	if name == "" || factory == nil {
		return
	}
	moduleRegistry.mu.Lock()
	moduleRegistry.m[name] = factory
	moduleRegistry.mu.Unlock()
}

// UnregisterModule removes a name from the registry. Used by tests
// that need to scope a registration to a single test run; production
// code should call RegisterModule once at init and let the entry
// outlive the process.
//
//	app.UnregisterModule("core/media")
func UnregisterModule(name string) {
	if name == "" {
		return
	}
	moduleRegistry.mu.Lock()
	delete(moduleRegistry.m, name)
	moduleRegistry.mu.Unlock()
}

// LookupModule returns the factory registered for `name`, if any.
// Returns (nil, false) for unknown names so callers can decide on a
// fallback path.
//
//	factory, ok := app.LookupModule("core/media")
func LookupModule(name string) (ModuleFactory, bool) {
	moduleRegistry.mu.RLock()
	defer moduleRegistry.mu.RUnlock()
	f, ok := moduleRegistry.m[name]
	return f, ok
}

// RegisteredModules returns the sorted list of module names known to
// the registry. Used by `core-app modules` (when wired) and the
// diagnostic surface to show the host's capability set.
//
//	for _, m := range app.RegisteredModules() { fmt.Println(m) }
func RegisteredModules() []string {
	moduleRegistry.mu.RLock()
	defer moduleRegistry.mu.RUnlock()
	if len(moduleRegistry.m) == 0 {
		return nil
	}
	out := make([]string, 0, len(moduleRegistry.m))
	for name := range moduleRegistry.m {
		out = append(out, name)
	}
	// Tiny insertion sort — module count is small (tens, maybe).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// resolveModulesFromRegistry consults the package registry for every
// declared module name and returns the matched factories alongside the
// names that did not resolve. Used by modules() to load any modules
// the host registered without forcing the manifest to know about Go
// type identities.
//
//	factories, missing := resolveModulesFromRegistry([]string{"core/media", "core/fs"})
func resolveModulesFromRegistry(names []string) ([]ModuleFactory, []string) {
	if len(names) == 0 {
		return nil, nil
	}
	matched := make([]ModuleFactory, 0, len(names))
	var missing []string
	for _, name := range names {
		f, ok := LookupModule(name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		matched = append(matched, f)
	}
	return matched, missing
}

// applyModuleFactories invokes every factory and registers the
// resulting CoreOption against the container. The factory pattern (a
// closure that returns a CoreOption) keeps registration deferred so a
// host can register dozens of module factories at init without paying
// the cost of every WithService call until a manifest actually asks
// for one.
//
//	if err := applyModuleFactories(c, factories); err != nil { return err }
func applyModuleFactories(c *core.Core, factories []ModuleFactory) error {
	if c == nil {
		return core.E("app.applyModuleFactories", "nil core", nil)
	}
	for _, f := range factories {
		if f == nil {
			continue
		}
		opt := f()
		if opt == nil {
			continue
		}
		// CoreOption is a closure that mutates the Core in-place and
		// returns a Result — we invoke it directly rather than
		// re-running core.New so the existing service surface is
		// preserved. A non-OK result surfaces as a typed error so the
		// caller can fail-fast on a misbehaving factory.
		if r := opt(c); !r.OK {
			cause, _ := r.Value.(error)
			return core.E("app.applyModuleFactories", "module option failed", cause)
		}
	}
	return nil
}

// loadModules is the registry-aware replacement for the modules() stub
// in modules.go. It resolves declared module names against the
// package registry, applies the matched factories to `c`, and surfaces
// a typed error listing the names that did not resolve. Dev mode
// callers can ignore the error; prod callers must treat it as fatal.
//
//	err := loadModules(ctx, c, &manifest, ModeProd)
func loadModules(_ context.Context, c *core.Core, m *config.ViewManifest, mode Mode) error {
	if c == nil {
		return core.E("app.loadModules", "nil core", nil)
	}
	if m == nil {
		return core.E("app.loadModules", "nil manifest", nil)
	}
	if len(m.Modules) == 0 {
		return nil
	}

	factories, missing := resolveModulesFromRegistry(m.Modules)

	if err := applyModuleFactories(c, factories); err != nil {
		return err
	}

	if len(missing) == 0 {
		return nil
	}

	// Prod mode rejects unresolved modules — the manifest asked for a
	// capability the host does not advertise, so silently dropping it
	// would leave the app crashing later when it dispatches an action.
	if mode == ModeProd {
		return core.E(
			"app.loadModules",
			"unresolved module(s): "+core.Join(", ", missing...),
			nil,
		)
	}
	// Dev mode logs the miss and keeps booting; the developer wants to
	// iterate even if a module is temporarily unavailable.
	core.Warn("unresolved manifest modules (dev mode)",
		"missing", core.Join(",", missing...),
		"app", m.Code,
	)
	return nil
}
