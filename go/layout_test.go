// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
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

// TestLayout_layout_Bad — a present slot with a non-string component
// fails cleanly instead of leaking a malformed spec to the host.
func TestLayout_layout_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Layout: "C",
		Slots: map[string]any{
			"C": 42,
		},
	}
	err := layout(c, m)
	if err == nil {
		t.Fatal("layout should reject a non-string slot component")
	}
	if !strings.Contains(err.Error(), "must name a string component") {
		t.Fatalf("layout error should name the malformed slot; got %v", err)
	}
}

// TestLayout_layout_Ugly — extra manifest slots are ignored when the
// layout variant does not reference them.
func TestLayout_layout_Ugly(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Layout: "HC",
		Slots: map[string]any{
			"H": "nav-breadcrumb",
			"C": "photo-grid",
			"R": "metadata-panel",
		},
	}
	spec, err := resolveLayout(c, m)
	if err != nil {
		t.Fatalf("resolveLayout: %v", err)
	}
	if spec == nil {
		t.Fatal("expected composed spec")
	}
	if spec.Has("R") {
		t.Fatalf("extra slot R should be ignored; got %+v", spec.Slots)
	}
	want := []string{"H", "C"}
	if len(spec.Order) != len(want) {
		t.Fatalf("Order = %v; want %v", spec.Order, want)
	}
	for i, slot := range want {
		if spec.Order[i] != slot {
			t.Fatalf("Order[%d] = %q; want %q", i, spec.Order[i], slot)
		}
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

// TestLayout_resolveLayout_Good — a populated manifest produces a
// LayoutSpec whose slots, order and components list match the variant
// string. Iteration through spec.Order is deterministic and mirrors
// the HLCRF prefix order.
func TestLayout_resolveLayout_Good(t *testing.T) {
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
	spec, err := resolveLayout(c, m)
	if err != nil {
		t.Fatalf("resolveLayout: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec for populated manifest")
	}
	if spec.Variant != "HLCRF" {
		t.Errorf("Variant = %q; want HLCRF", spec.Variant)
	}
	if got := spec.Slots["H"]; got != "nav-breadcrumb" {
		t.Errorf("Slots[H] = %q; want nav-breadcrumb", got)
	}
	want := []string{"H", "L", "C", "R", "F"}
	if len(spec.Order) != len(want) {
		t.Fatalf("Order length = %d; want %d (%v)", len(spec.Order), len(want), spec.Order)
	}
	for i, v := range want {
		if spec.Order[i] != v {
			t.Errorf("Order[%d] = %q; want %q", i, spec.Order[i], v)
		}
	}
	// Components list should mirror variant order without duplicates.
	if len(spec.Components) != 5 {
		t.Errorf("Components = %v; want 5 unique names", spec.Components)
	}
	if spec.Components[0] != "nav-breadcrumb" {
		t.Errorf("Components[0] = %q; want nav-breadcrumb", spec.Components[0])
	}
	if !spec.Has("C") {
		t.Error("Has(\"C\") should be true")
	}
	if spec.Has("X") {
		t.Error("Has(\"X\") should be false — unknown slot")
	}
}

// TestLayout_resolveLayout_Bad — a slot with a non-string component is
// rejected; a nil manifest and a nil core also error.
func TestLayout_resolveLayout_Bad(t *testing.T) {
	c := core.New()
	m := &config.ViewManifest{
		Layout: "C",
		Slots: map[string]any{
			"C": 42, // int, not string
		},
	}
	if _, err := resolveLayout(c, m); err == nil {
		t.Fatal("resolveLayout should reject non-string component")
	}
	if _, err := resolveLayout(c, nil); err == nil {
		t.Fatal("resolveLayout should reject nil manifest")
	}
	if _, err := resolveLayout(nil, m); err == nil {
		t.Fatal("resolveLayout should reject nil core")
	}
}

// TestLayout_resolveLayout_Ugly — an empty manifest returns (nil, nil)
// so headless CLI apps don't leak a spec object. Missing and extra slots
// are ignored during composition.
func TestLayout_resolveLayout_Ugly(t *testing.T) {
	c := core.New()
	if spec, err := resolveLayout(c, &config.ViewManifest{}); err != nil || spec != nil {
		t.Errorf("empty manifest should produce nil spec; got (%v, %v)", spec, err)
	}

	m := &config.ViewManifest{
		Layout: "HC",
		Slots: map[string]any{
			"H": "a",
			"C": "b",
			"X": "c", // declared outside variant — should be ignored
		},
	}
	spec, err := resolveLayout(c, m)
	if err != nil {
		t.Fatalf("resolveLayout: %v", err)
	}
	if spec.Has("X") {
		t.Errorf("slot outside variant should be ignored; got %v", spec.Slots)
	}
	if containsString(spec.Order, "X") {
		t.Errorf("slot outside variant should not appear in Order; got %v", spec.Order)
	}

	partial := &config.ViewManifest{
		Layout: "HCF",
		Slots: map[string]any{
			"H": "a",
			"F": "c",
		},
	}
	spec, err = resolveLayout(c, partial)
	if err != nil {
		t.Fatalf("partial resolveLayout: %v", err)
	}
	if spec.Has("C") {
		t.Errorf("missing slot C should be skipped; got %v", spec.Slots)
	}
	if containsString(spec.Order, "C") {
		t.Errorf("missing slot C should not appear in Order; got %v", spec.Order)
	}
}

// TestLayout_LayoutSpec_Has_Good — Has reports membership correctly.
func TestLayout_LayoutSpec_Has_Good(t *testing.T) {
	s := &LayoutSpec{Slots: map[string]string{"C": "main"}}
	if !s.Has("C") {
		t.Error("Has(C) should be true when slot is declared")
	}
}

// TestLayout_LayoutSpec_Has_Bad — unknown slot returns false; nil spec
// returns false without panicking.
func TestLayout_LayoutSpec_Has_Bad(t *testing.T) {
	s := &LayoutSpec{Slots: map[string]string{"C": "main"}}
	if s.Has("H") {
		t.Error("Has(H) should be false for undeclared slot")
	}
	var nilSpec *LayoutSpec
	if nilSpec.Has("C") {
		t.Error("nil spec Has should return false")
	}
}

// TestLayout_LayoutSpec_Has_Ugly — empty slot string falls through the
// map lookup and returns false deterministically.
func TestLayout_LayoutSpec_Has_Ugly(t *testing.T) {
	s := &LayoutSpec{Slots: map[string]string{"": "mystery"}}
	if !s.Has("") {
		t.Error("Has(\"\") should honour the literal empty slot mapping")
	}
}

// TestLayout_containsString_Good — positive hits.
func TestLayout_containsString_Good(t *testing.T) {
	if !containsString([]string{"a", "b", "c"}, "b") {
		t.Error("containsString should find 'b'")
	}
}

// TestLayout_containsString_Bad — misses return false.
func TestLayout_containsString_Bad(t *testing.T) {
	if containsString([]string{"a"}, "z") {
		t.Error("containsString should miss 'z'")
	}
}

// TestLayout_containsString_Ugly — nil / empty slices miss cleanly.
func TestLayout_containsString_Ugly(t *testing.T) {
	if containsString(nil, "x") {
		t.Error("containsString(nil) should miss")
	}
	if containsString([]string{}, "x") {
		t.Error("containsString([]) should miss")
	}
}
