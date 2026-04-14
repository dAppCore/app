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
