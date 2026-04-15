// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
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
