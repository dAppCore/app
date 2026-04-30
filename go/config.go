// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

// TemplateSuffix is the canonical extension for config template
// sources. The renderer trims it from the source path to derive the
// rendered file name (e.g. `conf/thumbs.json.tmpl` →
// `conf/thumbs.json`).
//
//	out := core.TrimSuffix("conf/thumbs.json.tmpl", app.TemplateSuffix)
//	// "conf/thumbs.json"
const TemplateSuffix = ".tmpl"

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
//  2. Expand `{{ .var }}` placeholders using the entry's `vars` map and
//     fall back to `c.Config()` lookups for `{{ .config.X }}` / env
//     variables for `{{ .env.X }}` so the same template covers ad-hoc
//     and environment-driven cases.
//
//  3. Write the rendered output next to the template (trim ".tmpl").
//
//     err := applyConfig(c, &manifest, coreio.Local, root)
//
// Rules:
//
//   - The template engine is intentionally simple — `{{ .name }}`
//     placeholders only, no conditionals or loops. Apps that need a
//     full Go-template engine should ship their own renderer; this
//     pipeline just provides the convention.
//
//   - A missing template file fails the boot in prod mode and logs a
//     warning in dev mode. Both modes keep the rest of the manifest
//     pipeline running (so a developer iterating on view.yaml can fix
//     the template in-place).
//
//   - The rendered output is written via the same medium used for
//     reads, so MockMedium and MemoryMedium round-trip cleanly in
//     tests.
func applyConfig(c *core.Core, m *config.ViewManifest, medium coreio.Medium, root string) error {
	return applyConfigWithMode(c, m, medium, root, ModeProd)
}

// applyConfigWithMode mirrors applyConfig but honours the boot mode so
// dev-mode config misses warn instead of aborting the boot.
func applyConfigWithMode(c *core.Core, m *config.ViewManifest, medium coreio.Medium, root string, mode Mode) error {
	if c == nil {
		return coreerr.E("app.applyConfigWithMode", "nil core", nil)
	}
	if m == nil {
		return coreerr.E("app.applyConfigWithMode", "nil manifest", nil)
	}
	if len(m.Config) == 0 {
		return nil
	}
	if medium == nil {
		medium = coreio.Local
	}

	for name, raw := range m.Config {
		// `services` and `source` are reserved Config keys — the wrap
		// pipeline writes them to the manifest so PluginBoot / pkg list
		// can resolve them. Skip anything that does not look like a
		// template entry rather than failing the boot.
		entry, ok := asTemplateEntry(raw)
		if !ok {
			if isReservedConfigKey(name) {
				continue
			}
			err := coreerr.E(
				"app.applyConfigWithMode",
				"config entry '"+name+"' is not a {template, vars} map",
				nil,
			)
			if mode == ModeDev {
				core.Warn("config entry skipped in dev mode", "name", name, "err", err)
				continue
			}
			return err
		}
		if entry.Template == "" {
			// Reserved keys may have an empty template (legacy config
			// values). Skip silently rather than asserting structure.
			if isReservedConfigKey(name) {
				continue
			}
			err := coreerr.E(
				"app.applyConfigWithMode",
				"config entry '"+name+"' is missing the template path",
				nil,
			)
			if mode == ModeDev {
				core.Warn("config entry skipped in dev mode", "name", name, "err", err)
				continue
			}
			return err
		}

		full := core.Path(root, entry.Template)
		if !medium.Exists(full) {
			err := coreerr.E(
				"app.applyConfigWithMode",
				"config template '"+full+"' (declared by '"+name+"') does not exist",
				nil,
			)
			if mode == ModeDev {
				core.Warn("config template missing in dev mode", "name", name, "path", full)
				continue
			}
			return err
		}

		body, err := medium.Read(full)
		if err != nil {
			err = coreerr.E(
				"app.applyConfigWithMode",
				"read template '"+full+"' (declared by '"+name+"') failed",
				err,
			)
			if mode == ModeDev {
				core.Warn("config template read failed in dev mode", "name", name, "path", full, "err", err)
				continue
			}
			return err
		}

		rendered := renderTemplate(body, resolveTemplateVars(c, entry.Vars))

		dst := destinationOf(full)
		if err := medium.EnsureDir(core.PathDir(dst)); err != nil {
			err = coreerr.E(
				"app.applyConfigWithMode",
				"ensure destination dir for '"+dst+"' failed",
				err,
			)
			if mode == ModeDev {
				core.Warn("config destination ensure failed in dev mode", "name", name, "path", dst, "err", err)
				continue
			}
			return err
		}
		if err := medium.Write(dst, rendered); err != nil {
			err = coreerr.E(
				"app.applyConfigWithMode",
				"write rendered template '"+dst+"' failed",
				err,
			)
			if mode == ModeDev {
				core.Warn("config write failed in dev mode", "name", name, "path", dst, "err", err)
				continue
			}
			return err
		}
	}
	return nil
}

// resolveTemplateVars expands any vars-map values that are themselves
// config/env references, for example `{{ .env.PORT }}` or
// `{{ .user.thumbnail_size }}`.
func resolveTemplateVars(c *core.Core, vars map[string]any) map[string]any {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]any, len(vars))
	for key, raw := range vars {
		out[key] = resolveTemplateVar(c, raw)
	}
	return out
}

// resolveTemplateVar resolves a single vars entry through Core config or
// environment lookup when the raw value is a simple placeholder. Values
// that are not placeholders are returned unchanged.
func resolveTemplateVar(c *core.Core, raw any) any {
	ref, ok := templateReference(raw)
	if !ok {
		return raw
	}
	if core.HasPrefix(ref, "env.") {
		key := ref[len("env."):]
		if key == "" {
			return raw
		}
		return core.Env(key)
	}
	if core.HasPrefix(ref, "config.") {
		key := ref[len("config."):]
		if key == "" {
			return raw
		}
		if value, ok := coreConfigValue(c, key); ok {
			return value
		}
		return raw
	}
	if value, ok := coreConfigValue(c, ref); ok {
		return value
	}
	return raw
}

// templateReference extracts `foo.bar` from a raw string like
// `{{ .foo.bar }}`. Returns false when the value is not a simple
// placeholder-only string.
func templateReference(raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = core.Trim(s)
	if !core.HasPrefix(s, "{{") || !core.HasSuffix(s, "}}") {
		return "", false
	}
	s = core.Trim(core.TrimSuffix(core.TrimPrefix(s, "{{"), "}}"))
	s = core.TrimPrefix(s, ".")
	if s == "" || core.Contains(s, " ") {
		return "", false
	}
	return s, true
}

// coreConfigValue reads a config value from the active Core instance.
func coreConfigValue(c *core.Core, key string) (any, bool) {
	if c == nil || c.Config() == nil || key == "" {
		return nil, false
	}
	r := c.Config().Get(key)
	if !r.OK {
		return nil, false
	}
	return r.Value, true
}

// renderTemplate performs `{{ .name }}` placeholder substitution
// against the supplied vars map. Whitespace around the variable name
// is tolerated (`{{ .name }}` and `{{.name}}` both resolve). Unknown
// placeholders are left in place so the rendered output makes the gap
// obvious — better a visible `{{ .missing }}` than a silently dropped
// value.
//
//	rendered := renderTemplate("port: {{ .port }}", map[string]any{"port": 9000})
//	// "port: 9000"
func renderTemplate(body string, vars map[string]any) string {
	if body == "" || len(vars) == 0 {
		return body
	}
	out := body
	for key, raw := range vars {
		value := stringOf(raw)
		// Three common spellings: tight, padded, and dot-prefixed (the
		// Go-template style the manifest examples show).
		for _, form := range []string{
			"{{." + key + "}}",
			"{{ ." + key + " }}",
			"{{." + key + " }}",
			"{{ ." + key + "}}",
			"{{" + key + "}}",
			"{{ " + key + " }}",
		} {
			out = core.Replace(out, form, value)
		}
	}
	return out
}

// destinationOf computes the rendered file's path from the template
// path. Strips the `.tmpl` suffix when present, otherwise appends a
// `.rendered` extension so the source is never overwritten by accident.
//
//	destinationOf("conf/thumbs.json.tmpl") // "conf/thumbs.json"
//	destinationOf("conf/thumbs.json")      // "conf/thumbs.json.rendered"
func destinationOf(src string) string {
	if core.HasSuffix(src, TemplateSuffix) {
		return core.TrimSuffix(src, TemplateSuffix)
	}
	return src + ".rendered"
}

// stringOf coerces a vars map value into the string form the template
// substitution expects. Numbers, booleans and other primitives all
// surface via core.Sprint so the renderer never panics on a typed map
// the YAML decoder produced.
//
//	stringOf(42)    // "42"
//	stringOf(true)  // "true"
//	stringOf("hi")  // "hi"
func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return core.Sprint(v)
}

// isReservedConfigKey reports whether a manifest Config entry is one
// of the framework-owned keys (services list, package source tag,
// PWA-wrap theme block, conclave isolation tag etc.) that should not be
// parsed as a template entry. Keeps applyConfig from rejecting wraps the
// package writes.
//
//	isReservedConfigKey("services") // true
//	isReservedConfigKey("isolation") // true (set by NewConclave)
func isReservedConfigKey(name string) bool {
	switch name {
	case "services", "source", "type", "url", "display",
		"short_name", "theme", "locale", "icon", "shim", "pwa", "gui_gates",
		"device_gates",
		"ipc_channels", "main", "entry",
		"write", "store", "window_mode", manifestAssetHashKey,
		"isolation", "write_paths",
		"category":
		return true
	}
	return false
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
