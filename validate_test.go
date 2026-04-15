// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"strings"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
)

// TestValidate_ValidateManifest_Good — a well-formed manifest passes
// every rule.
func TestValidate_ValidateManifest_Good(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "photo-browser",
		Name:    "Photo Browser",
		Version: "0.1.0",
		Layout:  "HLCRF",
		Slots:   map[string]any{"H": "nav", "C": "grid"},
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
		},
	}
	report := ValidateManifest(m, ValidateOptions{})
	if !report.OK() {
		t.Fatalf("expected OK report; got errors: %+v", report.Errors())
	}
}

// TestValidate_ValidateManifest_Bad — missing identity fields fail
// the validation with one ValidateError per field.
func TestValidate_ValidateManifest_Bad(t *testing.T) {
	report := ValidateManifest(&config.ViewManifest{}, ValidateOptions{AllowUnknownModules: true})
	if report.OK() {
		t.Fatal("expected errors for missing identity")
	}
	errs := report.Errors()
	wantFields := map[string]bool{"code": false, "name": false, "version": false}
	for _, e := range errs {
		if _, ok := wantFields[e.Field]; ok {
			wantFields[e.Field] = true
		}
	}
	for field, saw := range wantFields {
		if !saw {
			t.Errorf("expected a ValidateError for %q; not found in %+v", field, errs)
		}
	}
}

// TestValidate_ValidateManifest_Ugly — path traversal in permissions
// is a hard error regardless of the rest of the manifest shape.
func TestValidate_ValidateManifest_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "ok",
		Name:    "OK",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Read: []string{"../etc/passwd"},
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	if report.OK() {
		t.Fatal("expected error for traversal in permissions.read")
	}
	found := false
	for _, e := range report.Errors() {
		if strings.Contains(e.Field, "permissions.read") && strings.Contains(e.Message, "traversal") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected traversal error in permissions.read[0]; got %+v", report.Errors())
	}
}

// TestValidate_RequireSignature_Good — an unsigned manifest is a
// warning by default but an error when RequireSignature is set.
func TestValidate_RequireSignature_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0"}
	// Default — warning only.
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	if !report.OK() {
		t.Fatalf("default options should allow unsigned drafts; errors: %+v", report.Errors())
	}
	sawWarn := false
	for _, w := range report.Warnings() {
		if w.Field == "sign" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Error("expected a warning on the sign field")
	}
}

// TestValidate_RequireSignature_Bad — RequireSignature=true promotes
// the sign-is-empty warning to an error.
func TestValidate_RequireSignature_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0"}
	report := ValidateManifest(m, ValidateOptions{
		RequireSignature:    true,
		AllowUnknownModules: true,
	})
	if report.OK() {
		t.Fatal("expected error when RequireSignature=true and sign is empty")
	}
}

// TestValidate_RequireSignature_Ugly — ValidateManifestErr returns
// the first ValidateError as a typed error for callers that want a
// single error value.
func TestValidate_RequireSignature_Ugly(t *testing.T) {
	err := ValidateManifestErr(&config.ViewManifest{}, ValidateOptions{AllowUnknownModules: true})
	if err == nil {
		t.Error("expected a typed error for an empty manifest")
	}
}

// TestValidate_ModulesWarnings_Good — a manifest referring to a
// registered module does not produce a warning.
func TestValidate_ModulesWarnings_Good(t *testing.T) {
	RegisterModule("validate/test-mod", func() core.CoreOption {
		return func(_ *core.Core) core.Result { return core.Result{OK: true} }
	})
	defer UnregisterModule("validate/test-mod")

	m := &config.ViewManifest{
		Code: "x", Name: "X", Version: "0.1.0",
		Modules: []string{"validate/test-mod"},
	}
	report := ValidateManifest(m, ValidateOptions{})
	for _, w := range report.Warnings() {
		if strings.Contains(w.Message, "validate/test-mod") {
			t.Errorf("unexpected warning for registered module: %+v", w)
		}
	}
}

// TestValidate_ModulesWarnings_Bad — an unregistered module surfaces
// as a warning by default so a CI pipeline can spot missing host
// capabilities.
func TestValidate_ModulesWarnings_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Code: "x", Name: "X", Version: "0.1.0",
		Modules: []string{"nobody/not-registered"},
	}
	report := ValidateManifest(m, ValidateOptions{})
	saw := false
	for _, w := range report.Warnings() {
		if strings.Contains(w.Message, "nobody/not-registered") {
			saw = true
		}
	}
	if !saw {
		t.Error("expected a warning for the unknown module")
	}
}

// TestValidate_ModulesWarnings_Ugly — AllowUnknownModules=true skips
// the registry check so a standalone lint (no host context) does not
// spam warnings about modules the lint cannot possibly resolve.
func TestValidate_ModulesWarnings_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code: "x", Name: "X", Version: "0.1.0",
		Modules: []string{"only-in-host"},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	for _, w := range report.Warnings() {
		if strings.Contains(w.Message, "only-in-host") {
			t.Errorf("unexpected warning with AllowUnknownModules=true: %+v", w)
		}
	}
}

// TestValidate_LayoutSlotConsistency_Good — a manifest declaring a
// slot that matches the layout variant produces no layout/slot warning.
func TestValidate_LayoutSlotConsistency_Good(t *testing.T) {
	m := &config.ViewManifest{
		Code: "ok", Name: "OK", Version: "0.1.0",
		Layout: "HCF",
		Slots: map[string]any{
			"H": "nav", "C": "main", "F": "footer",
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	for _, w := range report.Warnings() {
		if strings.HasPrefix(w.Field, "slots.") || w.Field == "layout" {
			t.Errorf("unexpected layout/slot warning on consistent manifest: %+v", w)
		}
	}
}

// TestValidate_LayoutSlotConsistency_Bad — a slot declared outside
// the variant string fires a warning so the developer can spot the
// dead entry before runtime.
func TestValidate_LayoutSlotConsistency_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Code: "ok", Name: "OK", Version: "0.1.0",
		Layout: "HCF",
		Slots: map[string]any{
			"H": "nav", "C": "main", "F": "footer",
			"R": "dead-right-panel", // not in layout variant
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	saw := false
	for _, w := range report.Warnings() {
		if w.Field == "slots.R" && strings.Contains(w.Message, "not referenced by layout") {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected warning for slot R declared outside layout HCF; got %+v", report.Warnings())
	}
}

// TestValidate_LayoutSlotConsistency_Ugly — a layout variant naming a
// slot character with no component fires a warning so core/gui never
// renders an empty slot silently.
func TestValidate_LayoutSlotConsistency_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code: "ok", Name: "OK", Version: "0.1.0",
		Layout: "HLCRF",
		Slots: map[string]any{
			"H": "nav", "C": "main", "F": "footer",
			// L and R missing → empty slots
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	need := map[string]bool{"L": false, "R": false}
	for _, w := range report.Warnings() {
		if w.Field != "layout" {
			continue
		}
		for slot := range need {
			if strings.Contains(w.Message, "'"+slot+"'") {
				need[slot] = true
			}
		}
	}
	for slot, saw := range need {
		if !saw {
			t.Errorf("expected warning for layout slot %q without component; got %+v", slot, report.Warnings())
		}
	}
}

// TestValidate_ValidateIssueSeverity_Good — String returns the
// canonical lowercase name used in CLI output.
func TestValidate_ValidateIssueSeverity_Good(t *testing.T) {
	cases := []struct {
		sev  ValidateIssueSeverity
		want string
	}{
		{ValidateError, "error"},
		{ValidateWarning, "warning"},
		{ValidateIssueSeverity(42), "unknown"},
	}
	for _, c := range cases {
		if got := c.sev.String(); got != c.want {
			t.Errorf("String(%v) = %q; want %q", c.sev, got, c.want)
		}
	}
}
