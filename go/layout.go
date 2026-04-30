// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// LayoutSpec is the resolved form of a manifest's HLCRF layout. It
// pairs the canonical variant string with the slot → component map that
// Step 5 of the boot pipeline produced. core/gui (and any other host)
// reads the spec off the booted Instance to compose Web Components
// without re-parsing the manifest.
//
//	spec := inst.Layout
//	for _, slot := range spec.Order { comp := spec.Slots[slot] ... }
//
// Order is the canonical iteration order (H → L → C → R → F) — matches
// the variant string — so hosts render the layout deterministically
// regardless of how the YAML decoder ordered the slots map.
type LayoutSpec struct {
	Variant    string            // the "HLCRF" / "C" / "HCF" string from the manifest
	Slots      map[string]string // slot character → component name
	Order      []string          // slot characters in variant order
	Components []string          // unique component names, variant-ordered
}

// Has reports whether the spec declares a component for the slot
// character. Convenience wrapper so callers don't repeat the map
// lookup dance.
//
//	if spec.Has("H") { ... }
func (s *LayoutSpec) Has(slot string) bool {
	if s == nil {
		return false
	}
	_, ok := s.Slots[slot]
	return ok
}

// layout is Step 5 of the 7-step boot — compose the HLCRF slots from
// the manifest into a Layout that the host can hand to the GUI or
// render server-side. The composition itself (slot → component →
// Web Component) lives in core/go-html; this step collects the spec
// from the manifest so Start + any host-specific follow-up (core/gui
// window composition, Wails coroutine wiring) can consume it.
//
//	err := layout(c, &manifest)
//
// Steps:
//
//  1. Accept an empty layout (headless CLI app) — no-op.
//  2. Validate the layout variant string (HLCRF, HCF, C, …).
//  3. Keep every referenced slot that names a string component.
//  4. Ignore missing, empty, or unreferenced slots — minimal manifests
//     can declare a layout mode before shipping components.
//  5. (owned by caller) stash the resolved spec on the booted Instance
//     so Start / core/gui can pick it up without reparsing YAML.
//
// The returned LayoutSpec is the narrow form of `manifest.slots` —
// already string-typed, with a deterministic iteration order matching
// the variant string. A nil spec means "no layout" (headless CLI).
func layout(c *core.Core, m *config.ViewManifest) error {
	_, err := resolveLayout(c, m)
	return err
}

// resolveLayout is the implementation shared by layout() and Boot().
// Returns the narrow LayoutSpec so the Boot caller can stash it on the
// booted Instance; layout() throws the result away because the test
// suite predates the struct.
//
//	spec, err := resolveLayout(c, m)
func resolveLayout(c *core.Core, m *config.ViewManifest) (*LayoutSpec, error) {
	if c == nil {
		return nil, core.E("app.layout", "nil core", nil)
	}
	if m == nil {
		return nil, core.E("app.layout", "nil manifest", nil)
	}
	if m.Layout == "" {
		// Headless CLI app — no layout to compose.
		return nil, nil
	}

	if err := validateLayoutVariant(m.Layout); err != nil {
		return nil, core.E("app.layout", "invalid layout variant", err)
	}

	spec := &LayoutSpec{
		Variant: m.Layout,
		Slots:   make(map[string]string, len(m.Slots)),
	}
	spec.Order = make([]string, 0, len(m.Layout))

	// Components list — unique, variant-ordered so downstream codegen
	// writes stable output across boots.
	compSeen := map[string]bool{}
	seen := map[string]bool{}
	for _, ch := range m.Layout {
		slot := string(ch)
		if seen[slot] {
			continue
		}
		seen[slot] = true

		component, ok := m.Slots[slot]
		if !ok || component == nil {
			continue
		}

		comp, ok := component.(string)
		if !ok {
			return nil, core.E(
				"app.layout",
				"slot '"+slot+"' must name a string component",
				nil,
			)
		}
		if comp == "" {
			continue
		}

		spec.Order = append(spec.Order, slot)
		spec.Slots[slot] = comp
		if !compSeen[comp] {
			compSeen[comp] = true
			spec.Components = append(spec.Components, comp)
		}
	}
	return spec, nil
}

// containsString reports whether list already carries entry. Small
// helper so resolveLayout doesn't import slices / strings just for one
// lookup.
//
//	containsString([]string{"H","C"}, "C") // true
func containsString(list []string, entry string) bool {
	for _, v := range list {
		if v == entry {
			return true
		}
	}
	return false
}

// validateLayoutVariant accepts any string made up of HLCRF characters
// (Header, Left, Centre, Right, Footer). Matches the go-html validator
// behaviour so the two can be merged once the dep is wired.
//
//	_ = validateLayoutVariant("HLCRF") // nil
//	_ = validateLayoutVariant("C")     // nil
//	_ = validateLayoutVariant("XYZ")   // error
func validateLayoutVariant(v string) error {
	if v == "" {
		return nil
	}
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case 'H', 'L', 'C', 'R', 'F':
			continue
		default:
			return core.E(
				"app.validateLayoutVariant",
				"unknown slot character '"+string(v[i])+"' in layout '"+v+"'",
				nil,
			)
		}
	}
	return nil
}
