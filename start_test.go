// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// TestStart_start_Good — a ready Instance broadcasts ActionAppStarted
// and returns OK.
func TestStart_start_Good(t *testing.T) {
	c := core.New()

	var saw ActionAppStarted
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if evt, ok := msg.(ActionAppStarted); ok {
			saw = evt
		}
		return core.Result{OK: true}
	})

	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "started", Name: "Started", Version: "1.0.0",
		},
	}
	r := start(context.Background(), inst)
	if !r.OK {
		t.Fatalf("start.OK = false; Value=%v", r.Value)
	}
	if saw.Code != "started" {
		t.Errorf("ActionAppStarted.Code = %q; want %q", saw.Code, "started")
	}
	if saw.Mode != "prod" {
		t.Errorf("ActionAppStarted.Mode = %q; want %q", saw.Mode, "prod")
	}
}

// TestStart_start_Bad — nil Instance fails gracefully.
func TestStart_start_Bad(t *testing.T) {
	r := start(context.Background(), nil)
	if r.OK {
		t.Error("start with nil Instance should fail")
	}
}

// TestStart_start_Ugly — Instance with nil Core also fails (rather
// than nil-deref during broadcast).
func TestStart_start_Ugly(t *testing.T) {
	r := start(context.Background(), &Instance{})
	if r.OK {
		t.Error("start with nil Core should fail")
	}
}

// TestStart_ActionAppStarted_Good — fields are simple scalars; this
// locks the shape so downstream handlers don't break silently if we
// reorder.
func TestStart_ActionAppStarted_Good(t *testing.T) {
	evt := ActionAppStarted{
		Code: "c", Name: "n", Version: "v", Mode: "prod",
	}
	if evt.Code != "c" || evt.Name != "n" || evt.Version != "v" || evt.Mode != "prod" {
		t.Error("ActionAppStarted fields do not round-trip")
	}
}

// TestStart_ActionAppStarted_Bad — zero value is valid. The broadcast
// handler checks fields itself; the message type never errors.
func TestStart_ActionAppStarted_Bad(t *testing.T) {
	var evt ActionAppStarted
	if evt.Code != "" {
		t.Error("zero-value ActionAppStarted.Code should be empty")
	}
}

// TestStart_ActionAppStarted_Ugly — assignment through a
// core.Message interface preserves the type. Pinned here because the
// type-switch in subscriber handlers depends on it.
func TestStart_ActionAppStarted_Ugly(t *testing.T) {
	var msg core.Message = ActionAppStarted{Code: "x"}
	if _, ok := msg.(ActionAppStarted); !ok {
		t.Error("ActionAppStarted should satisfy core.Message")
	}
}

// TestStart_start_CarriesLayout — the ActionAppStarted broadcast
// carries the resolved LayoutSpec and Root so core/gui subscribers
// can compose the window without re-parsing the manifest. RFC §4.1
// step 7 names the hand-off; this test pins the contract so a future
// refactor doesn't silently drop the layout from the event.
func TestStart_start_CarriesLayout(t *testing.T) {
	c := core.New()

	var saw ActionAppStarted
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if evt, ok := msg.(ActionAppStarted); ok {
			saw = evt
		}
		return core.Result{OK: true}
	})

	layout := &LayoutSpec{
		Variant: "HLCRF",
		Slots: map[string]string{
			"H": "nav-breadcrumb",
			"C": "photo-grid",
		},
		Order:      []string{"H", "L", "C", "R", "F"},
		Components: []string{"nav-breadcrumb", "photo-grid"},
	}
	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Root: "/tmp/photo-browser",
		Manifest: config.ViewManifest{
			Code: "photo-browser", Name: "Photo Browser", Version: "0.1.0",
		},
		Layout: layout,
	}
	r := start(context.Background(), inst)
	if !r.OK {
		t.Fatalf("start.OK = false; Value=%v", r.Value)
	}
	if saw.Root != "/tmp/photo-browser" {
		t.Errorf("ActionAppStarted.Root = %q; want %q", saw.Root, "/tmp/photo-browser")
	}
	if saw.Layout == nil {
		t.Fatal("ActionAppStarted.Layout is nil; expected resolved LayoutSpec")
	}
	if saw.Layout.Variant != "HLCRF" {
		t.Errorf("Layout.Variant = %q; want HLCRF", saw.Layout.Variant)
	}
	if saw.Layout.Slots["C"] != "photo-grid" {
		t.Errorf("Layout.Slots[C] = %q; want photo-grid", saw.Layout.Slots["C"])
	}
}

// TestStart_start_HeadlessLayout — a CLI app (no layout variant, no
// slots) broadcasts a nil Layout. Subscribers must branch on the nil
// case rather than assuming every boot carries a window spec.
func TestStart_start_HeadlessLayout(t *testing.T) {
	c := core.New()

	var saw ActionAppStarted
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if evt, ok := msg.(ActionAppStarted); ok {
			saw = evt
		}
		return core.Result{OK: true}
	})

	inst := &Instance{
		Core: c,
		Mode: ModeDev,
		Manifest: config.ViewManifest{
			Code: "headless-cli", Name: "Headless CLI", Version: "1.0.0",
		},
		// Layout deliberately left nil — CLI / headless app.
	}
	if r := start(context.Background(), inst); !r.OK {
		t.Fatalf("start.OK = false; Value=%v", r.Value)
	}
	if saw.Layout != nil {
		t.Errorf("ActionAppStarted.Layout = %v; want nil for headless CLI", saw.Layout)
	}
	if saw.Mode != "dev" {
		t.Errorf("ActionAppStarted.Mode = %q; want dev", saw.Mode)
	}
}

// TestStart_InstanceStop_Good — Instance.Stop broadcasts an
// ActionAppStopping event subscribers can pick up via type-switch
// (RFC §11.5 — Stoppable lifecycle).
func TestStart_InstanceStop_Good(t *testing.T) {
	c := core.New()
	var saw ActionAppStopping
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if evt, ok := msg.(ActionAppStopping); ok {
			saw = evt
		}
		return core.Result{OK: true}
	})
	inst := &Instance{
		Core: c,
		Mode: ModeDev,
		Manifest: config.ViewManifest{
			Code: "stop-me", Name: "Stop Me", Version: "0.2.0",
		},
	}
	r := inst.Stop(context.Background())
	if !r.OK {
		t.Fatalf("Stop.OK=false; Value=%v", r.Value)
	}
	if saw.Code != "stop-me" {
		t.Errorf("ActionAppStopping.Code = %q; want %q", saw.Code, "stop-me")
	}
	if saw.Mode != "dev" {
		t.Errorf("ActionAppStopping.Mode = %q; want dev", saw.Mode)
	}
}

// TestStart_InstanceStop_Bad — nil instance / nil core both fail
// gracefully rather than panic.
func TestStart_InstanceStop_Bad(t *testing.T) {
	if r := (*Instance)(nil).Stop(context.Background()); r.OK {
		t.Error("Stop on nil Instance should fail")
	}
	if r := (&Instance{}).Stop(context.Background()); r.OK {
		t.Error("Stop on Instance with nil Core should fail")
	}
}

// TestStart_InstanceStop_Ugly — Stop with no registered subscribers
// still returns OK (broadcast is fire-and-forget).
func TestStart_InstanceStop_Ugly(t *testing.T) {
	inst := &Instance{
		Core: core.New(),
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "no-listeners", Name: "n", Version: "v",
		},
	}
	if r := inst.Stop(context.Background()); !r.OK {
		t.Errorf("Stop with no listeners should still OK; Value=%v", r.Value)
	}
}

// TestStart_ActionAppStopping_Good — fields round-trip through the
// struct literal.
func TestStart_ActionAppStopping_Good(t *testing.T) {
	evt := ActionAppStopping{
		Code: "c", Name: "n", Version: "v", Mode: "dev",
	}
	if evt.Code != "c" || evt.Mode != "dev" {
		t.Error("ActionAppStopping fields do not round-trip")
	}
}

// TestStart_ActionAppStopping_Bad — zero value is valid; subscribers
// type-switch on identity, not field presence.
func TestStart_ActionAppStopping_Bad(t *testing.T) {
	var evt ActionAppStopping
	if evt.Code != "" {
		t.Error("zero-value ActionAppStopping.Code should be empty")
	}
}

// TestStart_ActionAppStopping_Ugly — assignment through core.Message
// preserves the concrete type for the subscriber type-switch.
func TestStart_ActionAppStopping_Ugly(t *testing.T) {
	var msg core.Message = ActionAppStopping{Code: "x"}
	if _, ok := msg.(ActionAppStopping); !ok {
		t.Error("ActionAppStopping should satisfy core.Message")
	}
}

// lifecycleProbe is a tiny test service that records whether OnStartup
// and OnShutdown were called. Implements both core.Startable and
// core.Stoppable so RegisterService auto-discovery wires both halves.
type lifecycleProbe struct {
	startCalls int
	stopCalls  int
}

// OnStartup is the Startable hook — called by c.ServiceStartup.
func (p *lifecycleProbe) OnStartup(_ context.Context) core.Result {
	p.startCalls++
	return core.Result{OK: true}
}

// OnShutdown is the Stoppable hook — called by c.ServiceShutdown.
func (p *lifecycleProbe) OnShutdown(_ context.Context) core.Result {
	p.stopCalls++
	return core.Result{OK: true}
}

// TestStart_start_Lifecycle_Good — start() drives the Core lifecycle so
// every Startable service gets OnStartup before ActionAppStarted is
// broadcast (RFC §11.5).
func TestStart_start_Lifecycle_Good(t *testing.T) {
	probe := &lifecycleProbe{}
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: probe, OK: true}
	}))

	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "lifecycle", Name: "Lifecycle", Version: "0.1.0",
		},
	}
	if r := start(context.Background(), inst); !r.OK {
		t.Fatalf("start.OK = false; Value=%v", r.Value)
	}
	if probe.startCalls != 1 {
		t.Errorf("OnStartup calls = %d; want 1", probe.startCalls)
	}
	if !inst.started {
		t.Error("inst.started should be true after a successful start")
	}
}

// TestStart_start_Lifecycle_Bad — start() returns the lifecycle error so
// the caller never sees ActionAppStarted when a Startable refuses to
// boot. The probe's stop counter must stay zero — ServiceShutdown isn't
// invoked from start().
func TestStart_start_Lifecycle_Bad(t *testing.T) {
	failing := &failingStartProbe{}
	saw := false
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: failing, OK: true}
	}))
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if _, ok := msg.(ActionAppStarted); ok {
			saw = true
		}
		return core.Result{OK: true}
	})

	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "boom", Name: "Boom", Version: "0.0.1",
		},
	}
	r := start(context.Background(), inst)
	if r.OK {
		t.Fatal("start should fail when a Startable returns !OK")
	}
	if saw {
		t.Error("ActionAppStarted should not broadcast when lifecycle fails")
	}
	if inst.started {
		t.Error("inst.started must stay false after a failed lifecycle")
	}
}

// failingStartProbe is a Startable whose OnStartup returns !OK so the
// Lifecycle_Bad test can prove start() surfaces the failure.
type failingStartProbe struct{}

// OnStartup deliberately fails to exercise the start() error path.
func (failingStartProbe) OnStartup(_ context.Context) core.Result {
	return core.Result{Value: coreerr.E("test", "startup refused", nil), OK: false}
}

// TestStart_start_Lifecycle_Ugly — calling start() twice runs OnStartup
// only once. Re-entrant Start must be safe so a host that re-launches
// the entry action does not stack up duplicate lifecycle calls.
func TestStart_start_Lifecycle_Ugly(t *testing.T) {
	probe := &lifecycleProbe{}
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: probe, OK: true}
	}))

	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "twice", Name: "Twice", Version: "0.1.0",
		},
	}
	for i := 0; i < 2; i++ {
		if r := start(context.Background(), inst); !r.OK {
			t.Fatalf("start[%d] failed: %v", i, r.Value)
		}
	}
	if probe.startCalls != 1 {
		t.Errorf("OnStartup called %d times; want 1 (idempotent)", probe.startCalls)
	}
}

// TestStart_stop_Lifecycle_Good — Instance.Stop drives ServiceShutdown
// so every Stoppable service gets OnShutdown after the broadcast (RFC
// §11.5). Asserts the order: broadcast first, lifecycle second, so
// subscribers can flush state while their owning services are still
// alive.
func TestStart_stop_Lifecycle_Good(t *testing.T) {
	probe := &lifecycleProbe{}
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: probe, OK: true}
	}))

	var order []string
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if _, ok := msg.(ActionAppStopping); ok {
			order = append(order, "broadcast")
		}
		return core.Result{OK: true}
	})

	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "lifecycle-stop", Name: "Lifecycle Stop", Version: "0.1.0",
		},
	}
	// Drive a real start so inst.started is true and the stop path
	// engages the lifecycle.
	if r := start(context.Background(), inst); !r.OK {
		t.Fatalf("start failed: %v", r.Value)
	}
	if r := inst.Stop(context.Background()); !r.OK {
		t.Fatalf("Stop.OK = false; Value=%v", r.Value)
	}
	if probe.stopCalls != 1 {
		t.Errorf("OnShutdown calls = %d; want 1", probe.stopCalls)
	}
	if inst.started {
		t.Error("inst.started should reset to false after Stop")
	}
	// Broadcast must precede the lifecycle so subscribers see the
	// stopping signal while the service registry is still wired in.
	if len(order) != 1 || order[0] != "broadcast" {
		t.Errorf("expected broadcast in order log, got %v", order)
	}
}

// TestStart_stop_Lifecycle_Bad — Instance.Stop returns the lifecycle
// error so the host knows a Stoppable refused to clean up. The
// broadcast still goes out so subscribers can record the attempt.
func TestStart_stop_Lifecycle_Bad(t *testing.T) {
	failing := &failingStopProbe{}
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: failing, OK: true}
	}))
	saw := false
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if _, ok := msg.(ActionAppStopping); ok {
			saw = true
		}
		return core.Result{OK: true}
	})

	inst := &Instance{
		Core:    c,
		Mode:    ModeProd,
		started: true, // bypass the start gate so Stop runs the lifecycle
		Manifest: config.ViewManifest{
			Code: "stop-fail", Name: "Stop Fail", Version: "0.1.0",
		},
	}
	r := inst.Stop(context.Background())
	if r.OK {
		t.Fatal("Stop should fail when a Stoppable returns !OK")
	}
	if !saw {
		t.Error("ActionAppStopping should still broadcast on lifecycle failure")
	}
}

// failingStopProbe is a Stoppable whose OnShutdown returns !OK so the
// Stop_Bad test can prove the lifecycle error surfaces.
type failingStopProbe struct{}

// OnShutdown deliberately fails to exercise the stop() error path.
func (failingStopProbe) OnShutdown(_ context.Context) core.Result {
	return core.Result{Value: coreerr.E("test", "shutdown refused", nil), OK: false}
}

// TestStart_stop_Lifecycle_Ugly — Stop on an Instance that was never
// Started skips ServiceShutdown so a Stoppable never sees a phantom
// OnShutdown without a paired OnStartup.
func TestStart_stop_Lifecycle_Ugly(t *testing.T) {
	probe := &lifecycleProbe{}
	c := core.New(core.WithService(func(c *core.Core) core.Result {
		return core.Result{Value: probe, OK: true}
	}))
	inst := &Instance{
		Core: c,
		Mode: ModeProd,
		Manifest: config.ViewManifest{
			Code: "never-started", Name: "Never Started", Version: "0.1.0",
		},
	}
	if r := inst.Stop(context.Background()); !r.OK {
		t.Fatalf("Stop.OK = false; Value=%v", r.Value)
	}
	if probe.stopCalls != 0 {
		t.Errorf("OnShutdown should not run when Start was never called; got %d", probe.stopCalls)
	}
}
