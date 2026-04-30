// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
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
		if core.Contains(e.Field, "permissions.read") && core.Contains(e.Message, "traversal") {
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
	_ = "RequireSignature"
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
	_ = "RequireSignature"
	err := ValidateManifestErr(&config.ViewManifest{}, ValidateOptions{AllowUnknownModules: true})
	if err == nil {
		t.Error("expected a typed error for an empty manifest")
	}
}

// TestValidate_ModulesWarnings_Good — a manifest referring to a
// registered module does not produce a warning.
func TestValidate_ModulesWarnings_Good(t *testing.T) {
	_ = "ModulesWarnings"
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
		if core.Contains(w.Message, "validate/test-mod") {
			t.Errorf("unexpected warning for registered module: %+v", w)
		}
	}
}

// TestValidate_ModulesWarnings_Bad — an unregistered module surfaces
// as a warning by default so a CI pipeline can spot missing host
// capabilities.
func TestValidate_ModulesWarnings_Bad(t *testing.T) {
	_ = "ModulesWarnings"
	m := &config.ViewManifest{
		Code: "x", Name: "X", Version: "0.1.0",
		Modules: []string{"nobody/not-registered"},
	}
	report := ValidateManifest(m, ValidateOptions{})
	saw := false
	for _, w := range report.Warnings() {
		if core.Contains(w.Message, "nobody/not-registered") {
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
	_ = "ModulesWarnings"
	m := &config.ViewManifest{
		Code: "x", Name: "X", Version: "0.1.0",
		Modules: []string{"only-in-host"},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	for _, w := range report.Warnings() {
		if core.Contains(w.Message, "only-in-host") {
			t.Errorf("unexpected warning with AllowUnknownModules=true: %+v", w)
		}
	}
}

// TestValidate_LayoutSlotConsistency_Good — a manifest declaring a
// slot that matches the layout variant produces no layout/slot warning.
func TestValidate_LayoutSlotConsistency_Good(t *testing.T) {
	_ = "LayoutSlotConsistency"
	m := &config.ViewManifest{
		Code: "ok", Name: "OK", Version: "0.1.0",
		Layout: "HCF",
		Slots: map[string]any{
			"H": "nav", "C": "main", "F": "footer",
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	for _, w := range report.Warnings() {
		if core.HasPrefix(w.Field, "slots.") || w.Field == "layout" {
			t.Errorf("unexpected layout/slot warning on consistent manifest: %+v", w)
		}
	}
}

// TestValidate_LayoutSlotConsistency_Bad — a slot declared outside
// the variant string fires a warning so the developer can spot the
// dead entry before runtime.
func TestValidate_LayoutSlotConsistency_Bad(t *testing.T) {
	_ = "LayoutSlotConsistency"
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
		if w.Field == "slots.R" && core.Contains(w.Message, "not referenced by layout") {
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
	_ = "LayoutSlotConsistency"
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
			if core.Contains(w.Message, "'"+slot+"'") {
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

// TestValidate_ReservedCategoryKey_Good — a manifest carrying the
// marketplace-stamped `Config["category"]` metadata value round-trips
// through the validator without producing a template-shape error. Pins
// the RFC §6 marketplace flow (Search / Resolve / stamp) so a future
// change to the reserved key set never regresses the install path.
func TestValidate_ReservedCategoryKey_Good(t *testing.T) {
	_ = "ReservedCategoryKey"
	m := &config.ViewManifest{
		Code:    "cat-ok",
		Name:    "Category OK",
		Version: "0.1.0",
		Config: map[string]any{
			"category": "media",
			"source":   "marketplace:cat-ok",
		},
	}
	report := ValidateManifest(m, ValidateOptions{AllowUnknownModules: true})
	for _, e := range report.Errors() {
		if core.HasPrefix(e.Field, "config.") {
			t.Errorf("reserved key produced validation error: %+v", e)
		}
	}
}

// TestValidate_ValidateIssueSeverity_String_Good — String returns the
// canonical lowercase name used in CLI output.
func TestValidate_ValidateIssueSeverity_String_Good(t *testing.T) {
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

func TestValidate_ValidateIssueSeverity_String_Bad(t *testing.T) {
	if got := ValidateIssueSeverity(99).String(); got != "unknown" {
		t.Fatalf("ValidateIssueSeverity(99).String() = %q; want unknown", got)
	}
}

func TestValidate_ValidateIssueSeverity_String_Ugly(t *testing.T) {
	if got := ValidateIssueSeverity(-1).String(); got != "unknown" {
		t.Fatalf("ValidateIssueSeverity(-1).String() = %q; want unknown", got)
	}
}

func TestValidate_ValidateReport_OK_Good(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{{Severity: ValidateWarning, Field: "modules"}}}
	if !report.OK() {
		t.Fatal("warning-only report should be OK")
	}
}

func TestValidate_ValidateReport_OK_Bad(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{{Severity: ValidateError, Field: "code"}}}
	if report.OK() {
		t.Fatal("error report should not be OK")
	}
}

func TestValidate_ValidateReport_OK_Ugly(t *testing.T) {
	var report ValidateReport
	if !report.OK() {
		t.Fatal("zero-value report should be OK")
	}
}

func TestValidate_ValidateReport_Errors_Good(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{
		{Severity: ValidateWarning, Field: "modules"},
		{Severity: ValidateError, Field: "code"},
	}}
	errs := report.Errors()
	if len(errs) != 1 || errs[0].Field != "code" {
		t.Fatalf("Errors = %+v; want only code error", errs)
	}
}

func TestValidate_ValidateReport_Errors_Bad(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{{Severity: ValidateWarning, Field: "modules"}}}
	if errs := report.Errors(); len(errs) != 0 {
		t.Fatalf("Errors = %+v; want none", errs)
	}
}

func TestValidate_ValidateReport_Errors_Ugly(t *testing.T) {
	var report ValidateReport
	if errs := report.Errors(); len(errs) != 0 {
		t.Fatalf("zero-value Errors = %+v; want none", errs)
	}
}

func TestValidate_ValidateReport_Warnings_Good(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{
		{Severity: ValidateWarning, Field: "modules"},
		{Severity: ValidateError, Field: "code"},
	}}
	warnings := report.Warnings()
	if len(warnings) != 1 || warnings[0].Field != "modules" {
		t.Fatalf("Warnings = %+v; want only modules warning", warnings)
	}
}

func TestValidate_ValidateReport_Warnings_Bad(t *testing.T) {
	report := ValidateReport{Issues: []ValidateIssue{{Severity: ValidateError, Field: "code"}}}
	if warnings := report.Warnings(); len(warnings) != 0 {
		t.Fatalf("Warnings = %+v; want none", warnings)
	}
}

func TestValidate_ValidateReport_Warnings_Ugly(t *testing.T) {
	var report ValidateReport
	if warnings := report.Warnings(); len(warnings) != 0 {
		t.Fatalf("zero-value Warnings = %+v; want none", warnings)
	}
}

func TestValidate_ValidateManifestErr_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "ok", Name: "OK", Version: "0.1.0"}
	if err := ValidateManifestErr(m, ValidateOptions{}); err != nil {
		t.Fatalf("ValidateManifestErr good manifest: %v", err)
	}
}

func TestValidate_ValidateManifestErr_Bad(t *testing.T) {
	m := &config.ViewManifest{Name: "Missing Code", Version: "0.1.0"}
	if err := ValidateManifestErr(m, ValidateOptions{}); err == nil {
		t.Fatal("ValidateManifestErr should reject missing code")
	}
}

func TestValidate_ValidateManifestErr_Ugly(t *testing.T) {
	if err := ValidateManifestErr(nil, ValidateOptions{}); err == nil {
		t.Fatal("ValidateManifestErr should reject nil manifest")
	}
}
