// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// applyConfig is Step 6 of the 7-step boot — render any `config:`
// templates declared in the manifest and persist them to their target
// paths on the storage medium. The manifest shape (from RFC §2):
//
//	config:
//	  thumbnails:
//	    template: conf/thumbs.json.tmpl
//	    vars:
//	      size: "{{ .user.thumbnail_size }}"
//	      quality: "{{ .user.quality }}"
//
// Each entry resolves to:
//
//  1. Read `template` relative to the project root via coreio.Medium.
//
//  2. Expand `vars` using the resolved core/config store (user prefs,
//     env overrides) — `{{ .user.X }}` → config.Get("user.X", ...).
//
//  3. Write the rendered output next to the template (trim ".tmpl").
//
//     err := applyConfig(c, &manifest, coreio.Local, root)
//
// TODO(app): the skeleton validates shape (entry → template path) and
// confirms the template exists, but does NOT expand the template until
// core/config has a stable template renderer. Until then we:
//
//   - refuse to boot if a declared template is missing (prod mode)
//   - log a warning and continue in dev mode
//   - leave the actual {{ .x }} expansion to a future dep call
//
// The rendering engine itself belongs in core/config (issue: does
// ViewManifest.Config carry the template renderer contract, or does
// core/config grow a RenderTemplate(medium, src, dst, vars) helper?).
func applyConfig(c *core.Core, m *config.ViewManifest, medium coreio.Medium, root string) error {
	if c == nil {
		return coreerr.E("app.applyConfig", "nil core", nil)
	}
	if m == nil {
		return coreerr.E("app.applyConfig", "nil manifest", nil)
	}
	if len(m.Config) == 0 {
		return nil
	}
	if medium == nil {
		medium = coreio.Local
	}

	for name, raw := range m.Config {
		entry, ok := asTemplateEntry(raw)
		if !ok {
			return coreerr.E(
				"app.applyConfig",
				"config entry '"+name+"' is not a {template, vars} map",
				nil,
			)
		}
		if entry.Template == "" {
			return coreerr.E(
				"app.applyConfig",
				"config entry '"+name+"' is missing the template path",
				nil,
			)
		}

		full := core.Path(root, entry.Template)
		if !medium.Exists(full) {
			return coreerr.E(
				"app.applyConfig",
				"config template '"+full+"' (declared by '"+name+"') does not exist",
				nil,
			)
		}

		// TODO(config): render `entry.Template` with `entry.Vars`
		// against c.Config() and write the result to the template path
		// minus the ".tmpl" suffix.
	}
	return nil
}

// templateEntry is the decoded shape of a single `config:` map entry.
// The YAML side comes through as a map[string]any; asTemplateEntry does
// the narrowing.
type templateEntry struct {
	Template string         // relative path to the .tmpl source
	Vars     map[string]any // variables to expand inside the template
}

// asTemplateEntry narrows a map-shaped YAML value into a templateEntry.
// Returns false if the value doesn't look like a template entry (so the
// caller can produce a clean error message instead of a type-assert
// panic).
//
//	entry, ok := asTemplateEntry(raw)
//	if !ok { return errBadShape }
func asTemplateEntry(raw any) (templateEntry, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return templateEntry{}, false
	}

	var out templateEntry
	if t, ok := m["template"].(string); ok {
		out.Template = t
	}
	if v, ok := m["vars"].(map[string]any); ok {
		out.Vars = v
	}
	return out, true
}
