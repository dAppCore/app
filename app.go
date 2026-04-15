// SPDX-License-Identifier: EUPL-1.2

// Package app implements the CoreApp keystone runtime — the 7-step boot
// that turns a directory with a `.core/view.yaml` manifest into a running
// application.
//
// The sequence (Discover → Verify → Permissions → Modules → Layout → Config
// → Start) is defined in docs/RFC.md. Each step is a thin orchestrator over
// a dependency package; real work lives in core/config, core/go/scm,
// core/gui, and core/go-html.
//
// Typical entrypoint:
//
//	instance, err := app.Boot(ctx, "./")
//	if err != nil { return err }
//	instance.Start(ctx)
package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// Mode selects which enforcement regime the boot uses.
//
// ModeProd requires a signed manifest and enforces every declared
// permission. ModeDev skips the signature check, logs permission
// violations as warnings, and keeps the entry action hot-reloadable.
//
//	// production
//	_, err := app.Boot(ctx, "./", app.WithMode(app.ModeProd))
//
//	// development
//	_, err := app.Boot(ctx, "./", app.WithMode(app.ModeDev))
type Mode int

const (
	// ModeProd is the default — verify signature, enforce permissions.
	ModeProd Mode = iota

	// ModeDev skips signature verification and warns on permission
	// violations instead of denying them.
	ModeDev
)

// String returns the lowercase mode name for logs and CLI output.
//
//	app.ModeProd.String() // "prod"
//	app.ModeDev.String()  // "dev"
func (m Mode) String() string {
	switch m {
	case ModeDev:
		return "dev"
	default:
		return "prod"
	}
}

// Options configure a Boot call. Construct with functional helpers
// (WithMode, WithMedium, WithCore, WithPublicKey, WithTrustedKeysDir) so
// defaults are obvious.
//
//	opts := app.NewOptions(
//	    app.WithMode(app.ModeDev),
//	    app.WithMedium(coreio.Local),
//	)
type Options struct {
	Mode           Mode          // enforcement regime (prod | dev)
	Medium         coreio.Medium // storage abstraction — defaults to coreio.Local
	Core           *core.Core    // optional pre-built Core instance
	PublicKeyHex   string        // explicit hex-encoded ed25519 trust key
	TrustedKeysDir string        // directory of *.pub keys (default: $DIR_HOME/.core/keys/)
	DisableKeyLoad bool          // skip auto-loading TrustedKeysDir (test-only escape hatch)
	WorkspaceHome  string        // override $DIR_HOME for the per-app data tree
	SkipWorkspace  bool          // skip the per-app data tree (test-only escape hatch)
}

// Option mutates Options during Boot setup.
type Option func(*Options)

// WithMode selects the enforcement regime.
//
//	app.WithMode(app.ModeDev)
func WithMode(m Mode) Option {
	return func(o *Options) { o.Mode = m }
}

// WithMedium overrides the storage abstraction (defaults to coreio.Local).
//
//	app.WithMedium(mock)
func WithMedium(m coreio.Medium) Option {
	return func(o *Options) { o.Medium = m }
}

// WithCore attaches a pre-built Core instance instead of letting Boot
// construct one. Used by hosts (CoreGUI, core-agent) that already own a
// Core container.
//
//	c := core.New(core.WithService(agentic.Register))
//	app.WithCore(c)
func WithCore(c *core.Core) Option {
	return func(o *Options) { o.Core = c }
}

// WithPublicKey attaches an explicit hex-encoded ed25519 public key to
// trust when verifying the manifest signature. Callers that already know
// which key to trust (CLI `--trust-key`, marketplace index lookup, test
// fixtures) bypass the filesystem-keyring scan.
//
//	app.WithPublicKey("3b6a…")
//
// An empty string is a no-op so CLI wiring can pass the flag value
// unconditionally.
func WithPublicKey(hexKey string) Option {
	return func(o *Options) {
		if hexKey != "" {
			o.PublicKeyHex = hexKey
		}
	}
}

// WithTrustedKeysDir overrides the trust-key directory Boot scans during
// Step 2 (Verify). Each *.pub file in the directory holds a single
// hex-encoded ed25519 public key. Defaults to `$DIR_HOME/.core/keys/` so
// a user can drop marketplace-issued keys into a well-known place.
//
//	app.WithTrustedKeysDir("/tmp/test-keys")
func WithTrustedKeysDir(dir string) Option {
	return func(o *Options) { o.TrustedKeysDir = dir }
}

// WithoutKeyLoad disables the default filesystem keyring scan. Prod mode
// without an explicit WithPublicKey AND this option set will reject any
// signed manifest (no trust roots = no trust). Test-only escape hatch.
//
//	app.WithoutKeyLoad()
func WithoutKeyLoad() Option {
	return func(o *Options) { o.DisableKeyLoad = true }
}

// WithWorkspaceHome overrides the directory under which the per-app
// data tree (`<home>/.core/data/<code>/`) is created. Defaults to
// `$DIR_HOME` when empty so production paths follow the dAppServer
// convention without explicit wiring.
//
//	app.WithWorkspaceHome(t.TempDir())
func WithWorkspaceHome(home string) Option {
	return func(o *Options) {
		if home != "" {
			o.WorkspaceHome = home
		}
	}
}

// WithoutWorkspace skips the workspace bootstrap step. Hosts that
// already provisioned the data tree (CoreGUI multi-plugin runs) or
// tests asserting against the boot pipeline use this to avoid touching
// the filesystem.
//
//	app.WithoutWorkspace()
func WithoutWorkspace() Option {
	return func(o *Options) { o.SkipWorkspace = true }
}

// NewOptions applies options to a fresh Options struct.
//
//	opts := app.NewOptions(app.WithMode(app.ModeDev))
func NewOptions(opts ...Option) Options {
	o := Options{Mode: ModeProd, Medium: coreio.Local}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Medium == nil {
		o.Medium = coreio.Local
	}
	if o.TrustedKeysDir == "" {
		o.TrustedKeysDir = defaultTrustedKeysDir()
	}
	return o
}

// defaultTrustedKeysDir resolves the filesystem location to scan for
// `*.pub` trust keys. `$DIR_HOME/.core/keys/` matches the dAppServer
// convention and RFC §6 (marketplace-delivered keys).
//
//	dir := defaultTrustedKeysDir() // "/Users/me/.core/keys"
func defaultTrustedKeysDir() string {
	home := core.Env("DIR_HOME")
	if home == "" {
		return ""
	}
	return core.Path(home, ".core", "keys")
}

// Instance is a booted CoreApp. Holds the manifest, the Core container,
// and the resolved project root. Start() unblocks the entry action; the
// caller is responsible for the lifecycle beyond that.
//
//	inst, err := app.Boot(ctx, "./")
//	if err != nil { return err }
//	_ = inst.Start(ctx)
type Instance struct {
	Manifest  config.ViewManifest // parsed .core/view.yaml
	Core      *core.Core          // DI container (passed-in or constructed)
	Root      string              // absolute project root (directory of .core/)
	Mode      Mode                // regime used during Boot
	Workspace *Workspace          // per-app data tree (~/.core/data/<code>/)
	Layout    *LayoutSpec         // resolved HLCRF layout (nil = headless CLI app)
	medium    coreio.Medium       // retained for Start + post-boot reads
	started   bool                // RFC §11.5 — toggled by start/stop so the lifecycle does not run twice
}

// Boot runs the 7-step boot sequence against the project rooted at `start`.
// Returns an Instance that the caller can Start(), inspect, or hand to a
// host (CoreGUI) for windowing.
//
//	inst, err := app.Boot(ctx, "./", app.WithMode(app.ModeDev))
//	if err != nil {
//	    return core.E("app.Boot", "cannot boot", err)
//	}
//	if r := inst.Start(ctx); !r.OK {
//	    core.Error("start failed", "err", r.Value)
//	}
func Boot(ctx context.Context, start string, opts ...Option) (*Instance, error) {
	o := NewOptions(opts...)

	c := o.Core
	if c == nil {
		c = core.New()
	}

	inst := &Instance{
		Core:   c,
		Mode:   o.Mode,
		medium: o.Medium,
	}

	// Step 1 — Discover. Prod mode prefers core.json (the compiled,
	// distribution-ready artifact); dev mode always reads .core/view.yaml
	// so a running app picks up YAML edits on reboot without a recompile.
	manifest, root, err := discoverCompiled(o.Medium, start, o.Mode)
	if err != nil {
		return nil, coreerr.E("app.Boot", "discover failed", err)
	}
	inst.Manifest = manifest
	inst.Root = root

	// Step 2 — Verify
	trusted, err := resolveTrustedKeys(o)
	if err != nil {
		return nil, coreerr.E("app.Boot", "resolve trusted keys failed", err)
	}
	if err := verify(&manifest, o.Mode, trusted); err != nil {
		return nil, coreerr.E("app.Boot", "verify failed", err)
	}
	if err := verifyAssetIntegrity(o.Medium, root, &manifest, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "asset integrity failed", err)
	}

	// Step 3 — Permissions
	if err := permissions(c, &manifest, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "permission binding failed", err)
	}

	// Step 4 — Modules
	if err := modulesWithMode(ctx, c, &manifest, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "module load failed", err)
	}

	// Step 5 — Layout. The resolved spec is stashed on the Instance so
	// core/gui can compose the window without re-parsing the manifest.
	spec, err := resolveLayout(c, &manifest)
	if err != nil {
		return nil, coreerr.E("app.Boot", "layout composition failed", err)
	}
	inst.Layout = spec

	// Step 6 — Config
	if err := applyConfigWithMode(c, &manifest, o.Medium, root, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "config template failed", err)
	}

	// Workspace lifecycle — provision `<home>/.core/data/<code>/` so the
	// app has a guaranteed place for its store, cache, keys and tmp by
	// the time Start() fires. Failures here are non-fatal in dev mode
	// (the developer iterates without a writable home) but block prod.
	if !o.SkipWorkspace {
		ws, wsErr := OpenWorkspace(o.Medium, o.WorkspaceHome, manifest.Code)
		if wsErr != nil {
			if o.Mode == ModeProd {
				return nil, coreerr.E("app.Boot", "workspace bootstrap failed", wsErr)
			}
			// Dev mode: workspace is best-effort so the boot keeps
			// flowing when DIR_HOME is empty (CI containers, sandbox
			// tests). Inst.Workspace stays nil — handlers that need
			// the data tree must check.
			_ = wsErr
		}
		inst.Workspace = ws
	}

	// Note: Step 7 (Start) is the caller's explicit trigger — Boot returns
	// a ready-to-run Instance rather than blocking in the entry action.
	return inst, nil
}

// Start unblocks the entry action for this app. For a desktop app this
// hands the composed window to core/gui; for a CLI app it runs the first
// registered command. Start is separate from Boot so hosts can do extra
// setup (e.g. add more services) between boot and launch.
//
//	r := inst.Start(ctx)
//	if !r.OK { core.Error("start failed", "err", r.Value) }
func (inst *Instance) Start(ctx context.Context) core.Result {
	if inst == nil || inst.Core == nil {
		return core.Result{Value: coreerr.E("app.Instance.Start", "nil instance", nil), OK: false}
	}
	return start(ctx, inst)
}

// Stop is the symmetric tear-down for Start — broadcasts
// ActionAppStopping so subscribers (core/gui windows, core-agent fleet
// bus) can flush state and then drives `c.ServiceShutdown(ctx)` so
// every registered `Stoppable` runs OnStop in registration order. Hosts
// that orchestrate plugin lifecycles (RFC §11.5) call inst.Stop on
// idle, restart, or shutdown.
//
//	r := inst.Stop(ctx)
//	if !r.OK { core.Error("stop failed", "err", r.Value) }
//
// Rules:
//
//   - nil instance / nil core → typed Result.
//
//   - The Stop broadcast is fire-and-forget; subscribers that need to
//     veto a shutdown should respond via a separate Query handler (Core
//     IPC supports both). The bus does not block on listeners.
//
//   - When Start was never called the lifecycle is skipped — calling
//     ServiceShutdown without ServiceStartup is harmless on the Core
//     side, but skipping keeps Stop semantics symmetric and avoids
//     surprising Stoppable callbacks that never paired with an OnStart.
//
//   - Stop does NOT close the workspace — the host owns the data tree
//     beyond the app's lifetime so it can recover state on the next
//     boot.
func (inst *Instance) Stop(ctx context.Context) core.Result {
	if inst == nil || inst.Core == nil {
		return core.Result{Value: coreerr.E("app.Instance.Stop", "nil instance", nil), OK: false}
	}
	return stop(ctx, inst)
}
