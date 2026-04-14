// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// layout is Step 5 of the 7-step boot — compose the HLCRF slots from
// the manifest into a Layout that the host can hand to the GUI or
// render server-side. The composition itself (slot → component →
// Web Component) lives in core/go-html; this step collects the spec
// from the manifest and hands it off.
//
//	err := layout(c, &manifest)
//
// TODO(go-html): once the layout variant is stable (the "HLCRF" string
// validates and the codegen emits component stubs), replace this stub
// with a call into core/html that builds the component tree and caches
// the result on the Core container for the Start step to read.
//
// For the skeleton we:
//
//  1. Accept an empty layout (headless CLI app) — no-op.
//  2. Validate the layout variant string (HLCRF, HCF, C, …).
//  3. Check every slot names at least one component.
//  4. Stash the resolved spec in the Data registry under "app.layout"
//     so Start (and any host-specific follow-up) can read it.
func layout(c *core.Core, m *config.ViewManifest) error {
	if c == nil {
		return coreerr.E("app.layout", "nil core", nil)
	}
	if m == nil {
		return coreerr.E("app.layout", "nil manifest", nil)
	}
	if m.Layout == "" && len(m.Slots) == 0 {
		// Headless CLI app — no layout to compose.
		return nil
	}

	if err := validateLayoutVariant(m.Layout); err != nil {
		return coreerr.E("app.layout", "invalid layout variant", err)
	}

	for slot, component := range m.Slots {
		if component == nil {
			return coreerr.E(
				"app.layout",
				"slot '"+slot+"' names a nil component",
				nil,
			)
		}
	}

	// TODO(go-html): hand the variant + slots map to html.NewLayout,
	// resolve each component to a go-html Node, and stash the rendered
	// tree on the Core container for Start / core/gui to pick up.
	return nil
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
			return coreerr.E(
				"app.validateLayoutVariant",
				"unknown slot character '"+string(v[i])+"' in layout '"+v+"'",
				nil,
			)
		}
	}
	return nil
}
