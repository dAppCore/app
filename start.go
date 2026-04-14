// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	coreerr "dappco.re/go/core/log"
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
// TODO(core/gui): once core/gui exposes a NewWindowFromLayout
// constructor, call it here. Until then Start is a thin gate that:
//
//  1. Runs Core's own startup (ActionServiceStartup) via c.Run().
//  2. Broadcasts an ActionAppStarted event so downstream services know
//     boot finished.
//  3. Blocks on ctx.Done so the caller controls lifetime.
func start(ctx context.Context, inst *Instance) core.Result {
	if inst == nil || inst.Core == nil {
		return core.Result{
			Value: coreerr.E("app.start", "nil instance or core", nil),
			OK:    false,
		}
	}

	c := inst.Core

	// Broadcast the app-started signal. Services that registered a
	// RegisterAction handler pick it up via type-switch.
	c.ACTION(ActionAppStarted{
		Code:    inst.Manifest.Code,
		Name:    inst.Manifest.Name,
		Version: inst.Manifest.Version,
		Mode:    inst.Mode.String(),
	})

	// TODO(core/gui): hand the window spec over. Until then the boot
	// returns immediately; the CLI caller (cmd/core-app) prints the
	// booted identity and exits.
	return core.Result{OK: true}
}

// ActionAppStarted is the IPC broadcast at the end of step 7. Handlers
// registered with c.RegisterAction pick it up via a type-switch. Use it
// to signal core/gui to mount the window, or core/agent to announce on
// the fleet bus.
//
//	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
//	    if evt, ok := msg.(app.ActionAppStarted); ok {
//	        core.Info("app started", "code", evt.Code, "mode", evt.Mode)
//	    }
//	    return core.Result{OK: true}
//	})
type ActionAppStarted struct {
	Code    string // manifest.code
	Name    string // manifest.name
	Version string // manifest.version
	Mode    string // "prod" or "dev"
}
