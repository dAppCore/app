// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// TestStore_RuntimePermissionGate_Good verifies that manifest-declared
// store permission allows store mutations through the runtime action
// layer.
func TestStore_RuntimePermissionGate_Good(t *testing.T) {
	projectDir := t.TempDir()
	workspaceHome := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "store-permission-good",
		Name:    "Store Permission Good",
		Version: "0.1.0",
		Config: map[string]any{
			"store": true,
		},
	}
	writeViewManifest(t, projectDir, manifest)

	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithWorkspaceHome(workspaceHome),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}

	setResult := inst.Core.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
		core.Option{Key: "value", Value: "dark"},
	))
	if !setResult.OK {
		t.Fatalf("store.set failed: %v", setResult.Value)
	}

	getResult := inst.Core.Action("store.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
	))
	if !getResult.OK {
		t.Fatalf("store.get failed: %v", getResult.Value)
	}
	if got := resultMapString(getResult, "value"); got != "dark" {
		t.Errorf("store.get value = %q; want dark", got)
	}

	deleteResult := inst.Core.Action("store.delete").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
	))
	if !deleteResult.OK {
		t.Fatalf("store.delete failed: %v", deleteResult.Value)
	}
}

// TestStore_RuntimePermissionGate_Bad verifies that store mutations are
// denied when permissions.store is absent from the manifest.
func TestStore_RuntimePermissionGate_Bad(t *testing.T) {
	projectDir := t.TempDir()
	workspaceHome := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "store-permission-bad",
		Name:    "Store Permission Bad",
		Version: "0.1.0",
	}
	writeViewManifest(t, projectDir, manifest)

	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithWorkspaceHome(workspaceHome),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}

	setResult := inst.Core.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
		core.Option{Key: "value", Value: "dark"},
	))
	if setResult.OK {
		t.Fatal("store.set unexpectedly succeeded without permissions.store")
	}
	if msg := fmt.Sprint(setResult.Value); !strings.Contains(msg, "set permissions.store: true in view.yaml") {
		t.Fatalf("store.set error = %q; want store permission denial", msg)
	}

	deleteResult := inst.Core.Action("store.delete").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
	))
	if deleteResult.OK {
		t.Fatal("store.delete unexpectedly succeeded without permissions.store")
	}
	if msg := fmt.Sprint(deleteResult.Value); !strings.Contains(msg, "set permissions.store: true in view.yaml") {
		t.Fatalf("store.delete error = %q; want store permission denial", msg)
	}
}
