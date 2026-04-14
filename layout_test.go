// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
)

// TestLayout_layout_Good — an HLCRF manifest with named components
// passes the composition gate.
func TestLayout_layout_Good(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Layout: "HLCRF",
		Slots: map[string]any{
			"H": "nav-breadcrumb",
			"L": "folder-tree",
			"C": "photo-grid",
			"R": "metadata-panel",
			"F": "status-bar",
		},
	}
	if err := layout(c, m); err != nil {
		t.Fatalf("layout: %v", err)
	}
}

// TestLayout_layout_Bad — an unknown variant character fails validation.
func TestLayout_layout_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{Layout: "XYZ"}
	if err := layout(c, m); err == nil {
		t.Fatal("layout should reject unknown variant characters")
	}
}

// TestLayout_layout_Ugly — an empty manifest is a no-op (headless CLI
// apps have no layout).
func TestLayout_layout_Ugly(t *testing.T) {
	c := core.New()
	if err := layout(c, &config.ViewManifest{}); err != nil {
		t.Fatalf("empty layout should be no-op: %v", err)
	}
}

// TestLayout_validateLayoutVariant_Good — every character in the
// canonical HLCRF set is accepted.
func TestLayout_validateLayoutVariant_Good(t *testing.T) {
	for _, v := range []string{"H", "C", "HCF", "HLCRF"} {
		if err := validateLayoutVariant(v); err != nil {
			t.Errorf("variant %q should be valid: %v", v, err)
		}
	}
}

// TestLayout_validateLayoutVariant_Bad — a lowercase or unknown char
// fails.
func TestLayout_validateLayoutVariant_Bad(t *testing.T) {
	for _, v := range []string{"X", "hlcrf", "HX"} {
		if err := validateLayoutVariant(v); err == nil {
			t.Errorf("variant %q should fail validation", v)
		}
	}
}

// TestLayout_validateLayoutVariant_Ugly — empty string is allowed (it
// signals "no layout"). This pins the rule so the headless CLI path
// doesn't regress.
func TestLayout_validateLayoutVariant_Ugly(t *testing.T) {
	if err := validateLayoutVariant(""); err != nil {
		t.Errorf("empty variant should be allowed: %v", err)
	}
}
