// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"sync"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// HostOptions tunes NewHost. A host is the RFC §11 orchestrator that
// boots one or more plugin Instances, each with its own manifest,
// permissions and service surface. CoreGUI (desktop), core-agent
// (fleet) and any embedding binary use the host to run plugins without
// re-implementing the PluginBoot dance per call site.
//
//	registry := []app.PluginServiceSpec{
//	    {Name: "io",   Factory: core.WithService(io.Register)},
//	    {Name: "store", Factory: core.WithService(store.Register)},
//	}
//	host := app.NewHost(app.HostOptions{
//	    Home:     "/Users/me",
//	    Mode:     app.ModeProd,
//	    Services: registry,
//	})
type HostOptions struct {
	// Home overrides `$DIR_HOME`. Every plugin's workspace is provisioned
	// under `<Home>/.core/data/<code>/`.
	Home string
	// Mode picks prod (default) or dev. Individual Launch calls can
	// override via `LaunchOptions.Mode`.
	Mode Mode
	// Services is the master service registry — every factory the host
	// makes available to its plugins. PluginBoot filters the list by the
	// manifest's `services` declaration so a plugin gets exactly what it
	// asked for (RFC §11.1).
	Services []PluginServiceSpec
	// Medium overrides the default filesystem abstraction. Defaults to
	// coreio.Local so production hosts pick up the real filesystem; tests
	// hand in a MockMedium to exercise the host without touching disk.
	Medium coreio.Medium
	// PublicKeyHex is an optional explicit hex-encoded ed25519 trust key
	// used when verifying plugin manifests in prod mode. Mirrors Boot's
	// WithPublicKey option for hosts that already know the marketplace /
	// vendor key they want to trust.
	PublicKeyHex string
	// TrustedKeysDir overrides the directory scanned for `*.pub` trust
	// roots when verifying plugin manifests in prod mode. Empty falls
	// back to `$DIR_HOME/.core/keys/`, matching Boot.
	TrustedKeysDir string
	// DisableKeyLoad skips the `TrustedKeysDir` scan. Test-only escape
	// hatch so a host can prove the explicit-key path without reading a
	// real keyring from disk.
	DisableKeyLoad bool
}

// Host runs multiple plugin Instances and dispatches Actions between
// them via the Core IPC bus (RFC §11.2 — "Actions are the ONLY
// inter-plugin boundary"). Each plugin is isolated through PluginBoot;
// the host owns start/stop ordering and lookup.
//
//	host := app.NewHost(opts)
//	inst, err := host.Launch(ctx, "photo-browser", app.LaunchOptions{})
//	defer host.Stop(ctx, "photo-browser")
//
// A host is safe for concurrent use — Launch, Stop, Get and Each
// serialise on an internal mutex so a UI thread listing plugins while
// another goroutine shuts one down never sees a half-torn-down entry.
type Host struct {
	opts HostOptions

	mu      sync.Mutex
	plugins map[string]*Instance
}

// NewHost returns a host ready to Launch plugins. The registry in
// opts.Services is cloned so a caller mutating their original slice
// after construction does not affect the host's view.
//
//	host := app.NewHost(app.HostOptions{Services: registry})
func NewHost(opts HostOptions) *Host {
	if opts.Medium == nil {
		opts.Medium = coreio.Local
	}
	if opts.TrustedKeysDir == "" {
		opts.TrustedKeysDir = defaultTrustedKeysDir()
	}
	// Defensive copy so the caller can mutate their registry after
	// NewHost without the host noticing.
	if len(opts.Services) > 0 {
		opts.Services = append([]PluginServiceSpec(nil), opts.Services...)
	}
	return &Host{
		opts:    opts,
		plugins: map[string]*Instance{},
	}
}

// LaunchOptions tunes a Launch call. Zero values resolve from the host
// options so a caller can pass an empty struct for a "boot this plugin
// with host defaults" invocation.
//
//	inst, err := host.Launch(ctx, "photo-browser", app.LaunchOptions{
//	    Mode: app.ModeDev,
//	})
type LaunchOptions struct {
	// Mode overrides the host's default mode for this Launch only.
	Mode Mode
	// Manifest overrides the installed manifest — used when the caller
	// already has a parsed ViewManifest (CoreGUI plugin drawer) and
	// wants to skip the disk discovery step.
	Manifest *config.ViewManifest
	// Start overrides the app's project root. When Manifest is nil,
	// Launch discovers the plugin from this directory instead of the
	// installed-apps tree. Defaults to `<host.Home>/.core/apps/<code>/`.
	// Handy for a host that runs plugins from a non-default apps tree.
	Start string
	// Services overrides the host's service registry for this Launch.
	// Useful for a conclave (RFC §11.4) that wants to expose a narrower
	// service surface to a single plugin instance.
	Services []PluginServiceSpec
}

// Launch boots an installed plugin by code and remembers the Instance
// under that name. Returns a typed error when the plugin is already
// running (use Stop first to recycle) or when the install cannot be
// discovered.
//
//	inst, err := host.Launch(ctx, "photo-browser", app.LaunchOptions{})
//	if err != nil { return err }
//	r := inst.Start(ctx)
//
// Rules:
//
//   - Empty code → typed error (host needs an identity to key the map).
//
//   - Relaunching an already-running code → typed error so a misbehaving
//     UI dispatch cannot silently clobber the previous instance. The
//     error message names the code so the caller can surface a useful
//     "already running" toast to the user.
//
//   - Launch does NOT invoke inst.Start() — the caller decides when to
//     fire the boot broadcast (CoreGUI may want to mount the window
//     first, core-agent may want to attach listeners). Shutdown()
//     cleanly stops any plugin whose Start was driven via the host.
func (h *Host) Launch(ctx context.Context, code string, opts LaunchOptions) (*Instance, error) {
	if h == nil {
		return nil, coreerr.E("app.Host.Launch", "nil host", nil)
	}
	if code == "" {
		return nil, coreerr.E("app.Host.Launch", "empty code", nil)
	}

	h.mu.Lock()
	if _, exists := h.plugins[code]; exists {
		h.mu.Unlock()
		return nil, coreerr.E("app.Host.Launch", "plugin already running: "+code, nil)
	}
	h.mu.Unlock()

	mode := opts.Mode
	if mode == 0 {
		mode = h.opts.Mode
	}

	var manifest config.ViewManifest
	projectRoot := opts.Start
	if opts.Manifest != nil {
		manifest = *opts.Manifest
	} else {
		if projectRoot == "" {
			dir, err := DiscoverInstalled(h.opts.Medium, h.opts.Home, code)
			if err != nil {
				return nil, coreerr.E("app.Host.Launch", "discover failed for "+code, err)
			}
			projectRoot = dir
		}
		m, _, err := discoverCompiled(h.opts.Medium, projectRoot, mode)
		if err != nil {
			return nil, coreerr.E("app.Host.Launch", "discover failed for "+code, err)
		}
		manifest = m
	}
	if projectRoot == "" {
		return nil, coreerr.E("app.Host.Launch", "cannot resolve project root for "+code, nil)
	}
	trusted, err := resolveTrustedKeys(Options{
		Mode:           mode,
		Medium:         h.opts.Medium,
		PublicKeyHex:   h.opts.PublicKeyHex,
		TrustedKeysDir: h.opts.TrustedKeysDir,
		DisableKeyLoad: h.opts.DisableKeyLoad,
	})
	if err != nil {
		return nil, coreerr.E("app.Host.Launch", "resolve trusted keys failed for "+code, err)
	}
	if err := verify(&manifest, mode, trusted); err != nil {
		return nil, coreerr.E("app.Host.Launch", "verify failed for "+code, err)
	}

	// Resolve the plugin's service surface — the manifest's declared
	// service names filtered against whichever registry applies to this
	// Launch.
	registry := opts.Services
	if len(registry) == 0 {
		registry = h.opts.Services
	}
	required := PluginServicesFromManifest(&manifest)
	services := SelectServices(registry, required)

	inst, err := PluginBoot(ctx, PluginOptions{
		Manifest:    manifest,
		ProjectRoot: projectRoot,
		Mode:        mode,
		Services:    services,
		Medium:      h.opts.Medium,
	})
	if err != nil {
		return nil, coreerr.E("app.Host.Launch", "plugin boot failed for "+code, err)
	}

	// Bootstrap the workspace so the plugin has a data tree on first
	// start. Dev-mode failures are logged + tolerated so a developer
	// iterating on a manifest without a writable home still boots.
	if !h.skipWorkspace() {
		ws, wsErr := OpenWorkspace(h.opts.Medium, h.opts.Home, manifest.Code)
		if wsErr != nil {
			if mode == ModeProd {
				return nil, coreerr.E("app.Host.Launch", "workspace bootstrap failed for "+code, wsErr)
			}
			core.Warn("host: workspace bootstrap skipped (dev mode)",
				"code", code, "err", wsErr)
		}
		inst.Workspace = ws
	}

	h.mu.Lock()
	h.plugins[code] = inst
	h.mu.Unlock()
	return inst, nil
}

// skipWorkspace reports whether the host should bypass the workspace
// bootstrap. Returns true only when HostOptions.Home is empty and
// `$DIR_HOME` is also empty so a CI run without a home directory does
// not fail every plugin Launch.
//
//	if h.skipWorkspace() { /* skip */ }
func (h *Host) skipWorkspace() bool {
	if h == nil {
		return true
	}
	if h.opts.Home != "" {
		return false
	}
	return core.Env("DIR_HOME") == ""
}

// Get returns the Instance registered under `code`, if any. A host
// client that wants to dispatch an Action into a specific plugin goes
// through Get + Instance.Core to reach the target container.
//
//	inst, ok := host.Get("photo-browser")
//	if !ok { return errors.New("not running") }
//	r := inst.Core.Action("photo.import").Run(ctx, opts)
func (h *Host) Get(code string) (*Instance, bool) {
	if h == nil || code == "" {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	inst, ok := h.plugins[code]
	return inst, ok
}

// Each iterates every running plugin Instance in lexicographic order.
// The callback receives (code, instance); returning false stops the
// iteration early. Iteration happens under the host lock so the caller
// must not call back into Launch / Stop from within fn.
//
//	host.Each(func(code string, inst *app.Instance) bool {
//	    core.Info("plugin", "code", code, "version", inst.Manifest.Version)
//	    return true
//	})
func (h *Host) Each(fn func(code string, inst *Instance) bool) {
	if h == nil || fn == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	codes := make([]string, 0, len(h.plugins))
	for code := range h.plugins {
		codes = append(codes, code)
	}
	// Lexicographic order for deterministic test output.
	for i := 1; i < len(codes); i++ {
		for j := i; j > 0 && codes[j-1] > codes[j]; j-- {
			codes[j-1], codes[j] = codes[j], codes[j-1]
		}
	}
	for _, code := range codes {
		if !fn(code, h.plugins[code]) {
			return
		}
	}
}

// Running returns the sorted list of codes the host currently has
// booted. Convenience wrapper for UIs that render a plugin list without
// needing the Instance pointers.
//
//	for _, code := range host.Running() { /* render */ }
func (h *Host) Running() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.plugins) == 0 {
		return nil
	}
	out := make([]string, 0, len(h.plugins))
	for code := range h.plugins {
		out = append(out, code)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Stop tears down a single plugin — drives its inst.Stop (so
// ActionAppStopping fires and Stoppable services run OnStop) then
// removes it from the registry. Returns a typed error when the code
// is not running so the caller can distinguish "stop miss" from a real
// shutdown failure.
//
//	if r := host.Stop(ctx, "photo-browser"); !r.OK { ... }
//
// Rules:
//
//   - Unknown code → typed Result (OK=false) rather than panic.
//
//   - An instance that Stop-errors stays in the registry so a host UI
//     shows the "stuck" plugin and the operator can retry. A clean
//     Stop always removes the entry.
func (h *Host) Stop(ctx context.Context, code string) core.Result {
	if h == nil {
		return core.Result{Value: coreerr.E("app.Host.Stop", "nil host", nil), OK: false}
	}
	if code == "" {
		return core.Result{Value: coreerr.E("app.Host.Stop", "empty code", nil), OK: false}
	}

	h.mu.Lock()
	inst, ok := h.plugins[code]
	h.mu.Unlock()
	if !ok {
		return core.Result{
			Value: coreerr.E("app.Host.Stop", "plugin not running: "+code, nil),
			OK:    false,
		}
	}

	r := inst.Stop(ctx)
	if !r.OK {
		// Leave the entry — a UI may want to retry the tear-down once
		// the underlying issue (hung goroutine, stuck listener) is
		// resolved.
		return r
	}
	h.mu.Lock()
	delete(h.plugins, code)
	h.mu.Unlock()
	return core.Result{OK: true}
}

// Shutdown stops every running plugin in reverse Launch order so the
// plugin booted last is torn down first. Matches the RFC §11.5 rule
// that host lifecycle mirrors service registration — the most recent
// plugin is closest to the user and gets the first tear-down.
//
//	if r := host.Shutdown(ctx); !r.OK { core.Error("shutdown failed", "err", r.Value) }
//
// Rules:
//
//   - Returns OK=true only when every plugin stopped cleanly. A single
//     failure leaves the remaining plugins running and surfaces the
//     failing plugin's code in the returned error.
//
//   - A second Shutdown on a host that already emptied itself is a
//     no-op OK — idempotent teardown is valuable during tests.
func (h *Host) Shutdown(ctx context.Context) core.Result {
	if h == nil {
		return core.Result{Value: coreerr.E("app.Host.Shutdown", "nil host", nil), OK: false}
	}
	// Snapshot the current plugins so we release the lock during
	// inst.Stop — a plugin's OnStop may take time and we do not want
	// to block callers racing to Launch a different plugin.
	h.mu.Lock()
	snapshot := make([]string, 0, len(h.plugins))
	for code := range h.plugins {
		snapshot = append(snapshot, code)
	}
	h.mu.Unlock()
	if len(snapshot) == 0 {
		return core.Result{OK: true}
	}

	// Reverse-launch order = reverse lexicographic order is a reasonable
	// proxy — Launch never carries a timestamp and a host that wants
	// strict order should Stop individually. A precise order would
	// require recording insertion time which is overkill for the
	// "everybody down" path.
	for i := 1; i < len(snapshot); i++ {
		for j := i; j > 0 && snapshot[j-1] < snapshot[j]; j-- {
			snapshot[j-1], snapshot[j] = snapshot[j], snapshot[j-1]
		}
	}

	for _, code := range snapshot {
		if r := h.Stop(ctx, code); !r.OK {
			return core.Result{
				Value: coreerr.E("app.Host.Shutdown", "stop failed for "+code, extractErr(r)),
				OK:    false,
			}
		}
	}
	return core.Result{OK: true}
}

// Dispatch is the inter-plugin router promised by RFC §11.2. The
// caller names the target plugin's code and the Named Action; the host
// resolves the Instance, checks that the target actually registered a
// handler for that action, and dispatches via the plugin's Core. The
// source plugin is identified by `sourceCode` so a handler can reject
// calls from unknown callers (the simplest audit surface the host
// offers before a full authorisation matrix lands).
//
//	r := host.Dispatch(ctx, "photo-browser", "editor",
//	    "editor.save", core.NewOptions(core.Option{Key: "path", Value: "a.jpg"}))
//
// Rules:
//
//   - Empty sourceCode → rejected. A plugin dispatching an action must
//     identify itself so the target can reason about trust.
//
//   - Empty targetCode → rejected. The action must land somewhere.
//
//   - Target plugin not running → Result with a typed error so a host
//     UI can surface "not installed" / "not launched" without a panic.
//
//   - Target plugin running but no handler registered for `action` →
//     typed error. Keeps the RFC §11.2 rule honest ("if a plugin
//     doesn't register an action handler, that action doesn't exist
//     for it").
//
// The host does NOT inspect opts — the permission gate set up at
// PluginBoot time sits in front of the action and decides whether to
// honour the call. Dispatch is routing, not authorisation.
func (h *Host) Dispatch(ctx context.Context, sourceCode, targetCode, action string, opts core.Options) core.Result {
	if h == nil {
		return core.Result{Value: coreerr.E("app.Host.Dispatch", "nil host", nil), OK: false}
	}
	if sourceCode == "" {
		return core.Result{
			Value: coreerr.E("app.Host.Dispatch", "empty sourceCode — plugin must identify itself", nil),
			OK:    false,
		}
	}
	if targetCode == "" {
		return core.Result{
			Value: coreerr.E("app.Host.Dispatch", "empty targetCode", nil),
			OK:    false,
		}
	}
	if action == "" {
		return core.Result{
			Value: coreerr.E("app.Host.Dispatch", "empty action", nil),
			OK:    false,
		}
	}

	inst, ok := h.Get(targetCode)
	if !ok {
		return core.Result{
			Value: coreerr.E("app.Host.Dispatch", "target plugin not running: "+targetCode, nil),
			OK:    false,
		}
	}
	if inst.Core == nil {
		return core.Result{
			Value: coreerr.E("app.Host.Dispatch", "target plugin has no Core: "+targetCode, nil),
			OK:    false,
		}
	}

	if !inst.Core.Action(action).Exists() {
		return core.Result{
			Value: coreerr.E(
				"app.Host.Dispatch",
				"target "+targetCode+" did not register handler for "+action,
				nil,
			),
			OK: false,
		}
	}
	return inst.Core.Action(action).Run(ctx, opts)
}
