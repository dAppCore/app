// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// PluginOptions tunes PluginBoot. Plugins are isolated `core.New()`
// instances spun up by a host (CoreGUI, core-agent) for each
// discovered .core/view.yaml. Per RFC §11.1 the host calls PluginBoot
// to get a sandboxed Instance whose service surface matches the
// manifest exactly.
//
//	opts := app.PluginOptions{
//	    Manifest:    manifest,
//	    ProjectRoot: appPath,
//	    Mode:        app.ModeProd,
//	    Services:    []core.CoreOption{core.WithService(io.Register)},
//	}
type PluginOptions struct {
	// Manifest is the parsed view.yaml the host already loaded.
	Manifest config.ViewManifest
	// ProjectRoot is the absolute directory containing .core/.
	ProjectRoot string
	// Mode selects prod (default) or dev enforcement.
	Mode Mode
	// Services are the CoreOption registrations the host wants pinned
	// onto this plugin's container before WithServiceLock fires. The
	// host filters its master service list by what the manifest's
	// `services` field declares — the plugin gets exactly what it
	// asked for, nothing more.
	Services []core.CoreOption
	// Medium overrides the storage medium for permission/config steps.
	Medium coreio.Medium
}

// PluginBoot constructs an isolated Core container locked to the
// services the manifest declares, then drives the regular boot
// pipeline (Permissions → Modules → Layout → Config) without re-doing
// Discover/Verify (the host already did those when it scanned the
// installed-apps tree).
//
//	inst, err := app.PluginBoot(ctx, app.PluginOptions{
//	    Manifest:    manifest,
//	    ProjectRoot: "/Users/me/.core/apps/markdown-editor",
//	    Mode:        app.ModeProd,
//	    Services:    hostServices,
//	})
//	if err != nil { return err }
//	go inst.Start(ctx)
//
// Rules:
//
//   - Empty ProjectRoot is rejected — a plugin must know its sandbox.
//
//   - The constructed Core has WithServiceLock applied AFTER the
//     supplied services are registered. The plugin cannot grow its own
//     surface area at runtime.
//
//   - Permissions, modules and layout run as in Boot. The host is
//     responsible for sandboxing the medium (e.g. handing in a
//     coreio.Sandboxed(root)) so any read/write the plugin performs
//     stays inside its directory.
func PluginBoot(ctx context.Context, opts PluginOptions) (*Instance, error) {
	if opts.ProjectRoot == "" {
		return nil, coreerr.E("app.PluginBoot", "empty ProjectRoot", nil)
	}
	if opts.Manifest.Code == "" {
		return nil, coreerr.E("app.PluginBoot", "manifest.code is empty", nil)
	}

	medium := opts.Medium
	if medium == nil {
		medium = coreio.Local
	}

	// Build the Core with the supplied services + the lock so the
	// plugin can never register additional ones.
	coreOpts := append([]core.CoreOption{}, opts.Services...)
	coreOpts = append(coreOpts, core.WithServiceLock())
	c := core.New(coreOpts...)

	inst := &Instance{
		Manifest: opts.Manifest,
		Core:     c,
		Root:     opts.ProjectRoot,
		Mode:     opts.Mode,
		medium:   medium,
	}

	// Steps 3-6 of the boot pipeline. Step 1 (Discover) is the host's
	// job — it found and parsed view.yaml during its scan. Step 2
	// (Verify) is also the host's job — it decided the manifest was
	// trustworthy before calling PluginBoot. Step 7 (Start) is the
	// caller's explicit trigger.
	if err := permissions(c, &inst.Manifest, opts.Mode); err != nil {
		return nil, coreerr.E("app.PluginBoot", "permission binding failed", err)
	}
	if err := modulesWithMode(ctx, c, &inst.Manifest, opts.Mode); err != nil {
		// modulesWithMode already short-circuits the dev path internally
		// (logs + returns nil); a non-nil error here is therefore a
		// genuine prod failure and should bubble up.
		return nil, coreerr.E("app.PluginBoot", "module load failed", err)
	}
	spec, err := resolveLayout(c, &inst.Manifest)
	if err != nil {
		return nil, coreerr.E("app.PluginBoot", "layout composition failed", err)
	}
	inst.Layout = spec
	if err := applyConfigWithMode(c, &inst.Manifest, medium, opts.ProjectRoot, opts.Mode); err != nil {
		return nil, coreerr.E("app.PluginBoot", "config template failed", err)
	}
	if err := registerRuntimeActions(inst); err != nil {
		return nil, coreerr.E("app.PluginBoot", "runtime action registration failed", err)
	}

	return inst, nil
}

// PluginServices is the parsed form of the manifest's `services:` list
// (see RFC §11.1). The list is a free-form set of strings the host
// turns into CoreOption registrations — `io`, `i18n`, `crypt`, `store`,
// `api` etc. Held as its own type so a future config.ViewManifest field
// can carry it without breaking callers of PluginBoot.
//
//	required := app.PluginServices{"io", "store"}
//	opts := app.SelectServices(hostServices, required)
type PluginServices []string

// PluginServiceSpec records one host-side service factory keyed by the
// short name the manifest uses. Hosts build a registry once and reuse
// it across plugins:
//
//	registry := []app.PluginServiceSpec{
//	    {Name: "io",    Factory: core.WithService(io.Register)},
//	    {Name: "i18n",  Factory: core.WithService(i18n.Register)},
//	}
//	opts := app.SelectServices(registry, app.PluginServices{"io"})
type PluginServiceSpec struct {
	Name    string          // "io", "store", "api" etc. — matches manifest.services entry
	Factory core.CoreOption // the registration option the host wants applied
}

// SelectServices filters a host's service registry by the names the
// plugin's manifest declared. Unknown names are silently ignored — the
// host's registry is the trust root, and a plugin asking for a service
// the host doesn't expose is the plugin's problem to handle (typically
// via a graceful "feature unavailable" message).
//
//	opts := app.SelectServices(registry, manifest.Services)
//	inst, err := app.PluginBoot(ctx, app.PluginOptions{
//	    Services: opts,
//	    ...
//	})
func SelectServices(registry []PluginServiceSpec, requested PluginServices) []core.CoreOption {
	if len(registry) == 0 || len(requested) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, name := range requested {
		want[name] = true
	}
	out := make([]core.CoreOption, 0, len(registry))
	for _, spec := range registry {
		if want[spec.Name] {
			out = append(out, spec.Factory)
		}
	}
	return out
}

// PluginServicesFromManifest extracts the manifest's declared service
// names, looking in the canonical Config["services"] slot since
// config.ViewManifest does not yet expose a top-level Services field.
//
//	required := app.PluginServicesFromManifest(&manifest)
//
// The lookup tolerates either []string or []any (YAML decodes mixed
// scalar arrays as []any in the Config map).
func PluginServicesFromManifest(m *config.ViewManifest) PluginServices {
	if m == nil || m.Config == nil {
		return nil
	}
	raw, ok := m.Config["services"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return PluginServices(append([]string(nil), v...))
	case []any:
		out := make(PluginServices, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
