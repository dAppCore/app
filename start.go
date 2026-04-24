// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	coreerr "dappco.re/go/log"
)

// start is Step 7 of the 7-step boot — unblock the app's entry point.
// What "entry" means depends on the host:
//
//   - Desktop (core/gui): compose the HLCRF layout into a window and
//     run the Wails event loop.
//
//   - CLI headless: run the first registered command (inst.Core.Run()
//     gives the CLI router a chance to parse argv).
//
//   - Web / PWA: bind the HTTP handler and Serve.
//
//     r := start(ctx, inst)
//     if !r.OK { return r }
//
// Order:
//
//  1. Drive the Core lifecycle (RFC §11.5) — every registered service
//     with `OnStart` (the `Startable` interface) runs first, in
//     registration order. A failing OnStart aborts the boot before
//     ActionAppStarted is broadcast so downstream subscribers never
//     see a partially-initialised app.
//
//  2. Broadcast `ActionAppStarted` so handlers registered through
//     c.RegisterAction know boot finished. The broadcast is
//     fire-and-forget — slow listeners do not block the caller.
//
//  3. (TODO core/gui) Hand the composed window spec to core/gui.
//     Until that constructor lands, Start returns immediately; the
//     CLI caller (cmd/core-app) prints the booted identity and the
//     host owns the lifetime via Stop().
//
// inst.started is flipped on a successful pass so a re-entrant Start
// is a no-op for the lifecycle (services do not run OnStart twice).
func start(ctx context.Context, inst *Instance) core.Result {
	if inst == nil || inst.Core == nil {
		return core.Result{
			Value: coreerr.E("app.start", "nil instance or core", nil),
			OK:    false,
		}
	}

	c := inst.Core

	// Step 7a — Run Startable.OnStart for every registered service
	// (RFC §11.5). Services that opened a DB, started a goroutine, or
	// dialled an upstream service get a chance to fail fast here.
	if !inst.started {
		if r := c.ServiceStartup(ctx, nil); !r.OK {
			return core.Result{
				Value: coreerr.E("app.start", "service startup failed", extractErr(r)),
				OK:    false,
			}
		}
		inst.started = true
	}

	// Step 7b — broadcast the app-started signal. Services that
	// registered a RegisterAction handler pick it up via type-switch.
	// The resolved LayoutSpec travels with the message so core/gui can
	// compose the window without re-parsing the manifest — RFC §4.1
	// step 7 ("hand the composed window spec to core/gui") is honoured
	// via the IPC bus rather than a direct call so RFC §11.2 stays
	// intact ("Actions are the ONLY inter-plugin boundary"). Root is
	// stamped so subscribers that resolve project-relative assets
	// (theme CSS, slot components) don't repeat the Root lookup.
	c.ACTION(ActionAppStarted{
		Code:    inst.Manifest.Code,
		Name:    inst.Manifest.Name,
		Version: inst.Manifest.Version,
		Mode:    inst.Mode.String(),
		Root:    inst.Root,
		Layout:  inst.Layout,
	})

	return core.Result{OK: true}
}

// stop is the symmetric tear-down for start — it broadcasts
// `ActionAppStopping` and then drives the Core lifecycle's
// `ServiceShutdown` so every registered `Stoppable` runs OnStop in
// registration order.
//
// The Stop broadcast goes first so subscribers (core/gui windows,
// core-agent fleet farewell) can flush ephemeral state while their
// owning services are still alive. Once the broadcast returns we drain
// the lifecycle.
//
//	r := stop(ctx, inst)
//	if !r.OK { core.Error("stop failed", "err", r.Value) }
func stop(ctx context.Context, inst *Instance) core.Result {
	if inst == nil || inst.Core == nil {
		return core.Result{
			Value: coreerr.E("app.stop", "nil instance or core", nil),
			OK:    false,
		}
	}

	c := inst.Core

	// Notify subscribers first. Symmetrical with Start which broadcasts
	// AFTER lifecycle init; on the way down we want subscribers to see
	// the message while their services are still wired in.
	c.ACTION(ActionAppStopping{
		Code:    inst.Manifest.Code,
		Name:    inst.Manifest.Name,
		Version: inst.Manifest.Version,
		Mode:    inst.Mode.String(),
	})
	if inst.runtime != nil {
		inst.runtime.shutdown()
	}

	// Skip the lifecycle when Start was never called — calling
	// ServiceShutdown on a Core that never had ServiceStartup is fine,
	// but skipping keeps the Stop semantics symmetric with start() and
	// avoids surprising "Stoppable.OnStop" calls that never paired with
	// an OnStart.
	if !inst.started {
		return core.Result{OK: true}
	}
	inst.started = false

	if r := c.ServiceShutdown(ctx); !r.OK {
		return core.Result{
			Value: coreerr.E("app.stop", "service shutdown failed", extractErr(r)),
			OK:    false,
		}
	}
	return core.Result{OK: true}
}

// ActionAppStarted is the IPC broadcast at the end of step 7. Handlers
// registered with c.RegisterAction pick it up via a type-switch. Use it
// to signal core/gui to mount the window, or core/agent to announce on
// the fleet bus.
//
//	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
//	    if evt, ok := msg.(app.ActionAppStarted); ok {
//	        core.Info("app started",
//	            "code", evt.Code,
//	            "mode", evt.Mode,
//	            "layout", evt.Layout.Variant)
//	        // core/gui composes the window from evt.Layout + evt.Root
//	    }
//	    return core.Result{OK: true}
//	})
//
// Root and Layout carry everything core/gui needs to compose the
// window at Step 7 without reopening the manifest — the LayoutSpec
// already has slot → component → order resolved, and Root is the
// absolute directory the composition should reference for slot
// asset lookups.
type ActionAppStarted struct {
	Code    string      // manifest.code
	Name    string      // manifest.name
	Version string      // manifest.version
	Mode    string      // "prod" or "dev"
	Root    string      // absolute project root (parent of .core/)
	Layout  *LayoutSpec // resolved HLCRF layout (nil = headless CLI app)
}

// ActionAppStopping is the symmetric broadcast (RFC §11.5 "Stoppable")
// the host fires before tearing the app down. Subscribers flush state
// (window position, in-flight requests, fleet-bus farewell) and return
// quickly — the bus does not wait for slow listeners.
//
//	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
//	    if evt, ok := msg.(app.ActionAppStopping); ok {
//	        core.Info("app stopping", "code", evt.Code)
//	    }
//	    return core.Result{OK: true}
//	})
type ActionAppStopping struct {
	Code    string // manifest.code
	Name    string // manifest.name
	Version string // manifest.version
	Mode    string // "prod" or "dev"
}
