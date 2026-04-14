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
// (WithMode, WithMedium, WithCore) so defaults are obvious.
//
//	opts := app.NewOptions(
//	    app.WithMode(app.ModeDev),
//	    app.WithMedium(coreio.Local),
//	)
type Options struct {
	Mode   Mode          // enforcement regime (prod | dev)
	Medium coreio.Medium // storage abstraction — defaults to coreio.Local
	Core   *core.Core    // optional pre-built Core instance
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
	return o
}

// Instance is a booted CoreApp. Holds the manifest, the Core container,
// and the resolved project root. Start() unblocks the entry action; the
// caller is responsible for the lifecycle beyond that.
//
//	inst, err := app.Boot(ctx, "./")
//	if err != nil { return err }
//	_ = inst.Start(ctx)
type Instance struct {
	Manifest config.ViewManifest // parsed .core/view.yaml
	Core     *core.Core          // DI container (passed-in or constructed)
	Root     string              // absolute project root (directory of .core/)
	Mode     Mode                // regime used during Boot
	medium   coreio.Medium       // retained for Start + post-boot reads
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

	// Step 1 — Discover
	manifest, root, err := discover(o.Medium, start)
	if err != nil {
		return nil, coreerr.E("app.Boot", "discover failed", err)
	}
	inst.Manifest = manifest
	inst.Root = root

	// Step 2 — Verify
	if err := verify(&manifest, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "verify failed", err)
	}

	// Step 3 — Permissions
	if err := permissions(c, &manifest, o.Mode); err != nil {
		return nil, coreerr.E("app.Boot", "permission binding failed", err)
	}

	// Step 4 — Modules
	if err := modules(ctx, c, &manifest); err != nil {
		return nil, coreerr.E("app.Boot", "module load failed", err)
	}

	// Step 5 — Layout
	if err := layout(c, &manifest); err != nil {
		return nil, coreerr.E("app.Boot", "layout composition failed", err)
	}

	// Step 6 — Config
	if err := applyConfig(c, &manifest, o.Medium, root); err != nil {
		return nil, coreerr.E("app.Boot", "config template failed", err)
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
