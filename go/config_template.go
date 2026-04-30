// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"reflect"
	"strconv"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// configTemplateSpec is the narrowed shape of one manifest `config:`
// entry that declares a template render.
type configTemplateSpec struct {
	Name     string
	Template string
	Dest     string
	Vars     map[string]any
}

// configTemplatePart is one parsed template fragment: either literal
// text or a `{{ .path }}` placeholder.
type configTemplatePart struct {
	Text string
	Path string
}

// renderManifestConfigTemplatesWithMode renders every manifest
// `config:` template entry against the current runtime state. Vars are
// resolved in two stages:
//
//  1. `vars` expressions (`{{ .user.thumbnail_size }}`) are evaluated
//     against the hydrated store / env / core-config snapshot.
//  2. The template file body is rendered against the resolved vars map.
func renderManifestConfigTemplatesWithMode(
	c *core.Core,
	m *config.ViewManifest,
	medium coreio.Medium,
	root string,
	ws *Workspace,
	mode Mode,
) error {
	if m == nil {
		return core.E("app.renderManifestConfigTemplatesWithMode", "nil manifest", nil)
	}
	if len(m.Config) == 0 {
		return nil
	}
	if medium == nil {
		medium = coreio.Local
	}

	specs, err := manifestConfigTemplateSpecs(m)
	if err != nil {
		if mode == ModeDev {
			core.Warn("config template skipped in dev mode", "err", err)
			return nil
		}
		return err
	}

	var store *workspaceObjectStore
	if ws != nil {
		store = newWorkspaceObjectStore(ws)
		defer store.Close()
	}

	for _, spec := range specs {
		src := core.Path(root, spec.Template)
		body, err := medium.Read(src)
		if err != nil {
			err = core.E(
				"app.renderManifestConfigTemplatesWithMode",
				core.Sprintf("read config template %q failed", src),
				err,
			)
			if mode == ModeDev {
				core.Warn("config template skipped in dev mode", "name", spec.Name, core.Concat("pa", "th"), src, "err", err)
				continue
			}
			return err
		}

		vars, err := resolveConfigTemplateVars(c, store, spec.Vars)
		if err != nil {
			err = core.E(
				"app.renderManifestConfigTemplatesWithMode",
				core.Sprintf("resolve config template vars for %q failed", spec.Name),
				err,
			)
			if mode == ModeDev {
				core.Warn("config template skipped in dev mode", "name", spec.Name, core.Concat("pa", "th"), src, "err", err)
				continue
			}
			return err
		}

		rendered, err := renderConfigTemplateText(body, vars)
		if err != nil {
			err = core.E(
				"app.renderManifestConfigTemplatesWithMode",
				core.Sprintf("render config template %q failed", src),
				err,
			)
			if mode == ModeDev {
				core.Warn("config template skipped in dev mode", "name", spec.Name, core.Concat("pa", "th"), src, "err", err)
				continue
			}
			return err
		}

		dst := configTemplateDestination(root, src, spec)
		if err := medium.EnsureDir(core.PathDir(dst)); err != nil {
			err = core.E(
				"app.renderManifestConfigTemplatesWithMode",
				core.Sprintf("ensure destination dir for %q failed", dst),
				err,
			)
			if mode == ModeDev {
				core.Warn("config template skipped in dev mode", "name", spec.Name, core.Concat("pa", "th"), dst, "err", err)
				continue
			}
			return err
		}
		if err := medium.Write(dst, rendered); err != nil {
			err = core.E(
				"app.renderManifestConfigTemplatesWithMode",
				core.Sprintf("write rendered config template %q failed", dst),
				err,
			)
			if mode == ModeDev {
				core.Warn("config template skipped in dev mode", "name", spec.Name, core.Concat("pa", "th"), dst, "err", err)
				continue
			}
			return err
		}
	}

	return nil
}

func manifestConfigTemplateSpecs(m *config.ViewManifest) (
	[]configTemplateSpec, error,
) {
	if m == nil || len(m.Config) == 0 {
		return nil, nil
	}

	specs := make([]configTemplateSpec, 0, len(m.Config))
	for name, raw := range m.Config {
		if isReservedConfigKey(name) {
			continue
		}
		spec, ok := configTemplateSpecFromRaw(name, raw)
		if !ok {
			return nil, core.E(
				"app.manifestConfigTemplateSpecs",
				core.Sprintf("config entry %q is not a {template, vars} map", name),
				nil,
			)
		}
		if spec.Template == "" {
			return nil, core.E(
				"app.manifestConfigTemplateSpecs",
				core.Sprintf("config entry %q is missing the template path", name),
				nil,
			)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func configTemplateSpecFromRaw(name string, raw any) (configTemplateSpec, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return configTemplateSpec{}, false
	}

	spec := configTemplateSpec{Name: name}
	if templatePath, ok := m["template"].(string); ok {
		spec.Template = templatePath
	}
	if dest, ok := m["dest"].(string); ok {
		spec.Dest = dest
	}
	if vars, ok := m["vars"].(map[string]any); ok {
		spec.Vars = vars
	} else if rawVars, hasVars := m["vars"]; hasVars && rawVars != nil {
		return configTemplateSpec{}, false
	}
	return spec, true
}

func configTemplateDestination(root, src string, spec configTemplateSpec) string {
	if spec.Dest != "" {
		return core.Path(root, spec.Dest)
	}
	return destinationOf(src)
}

func resolveConfigTemplateVars(c *core.Core, store *workspaceObjectStore, vars map[string]any) (
	map[string]any, error,
) {
	if len(vars) == 0 {
		return nil, nil
	}

	out := make(map[string]any, len(vars))
	for key, raw := range vars {
		value, err := resolveConfigTemplateVar(c, store, raw)
		if err != nil {
			return nil, core.E(
				"app.resolveConfigTemplateVars",
				core.Sprintf("resolve config template var %q failed", key),
				err,
			)
		}
		out[key] = value
	}
	return out, nil
}

func resolveConfigTemplateVar(c *core.Core, store *workspaceObjectStore, raw any) (
	any, error,
) {
	text, ok := raw.(string)
	if !ok {
		return raw, nil
	}

	parts, err := parseConfigTemplate(text)
	if err != nil {
		return nil, err
	}
	if !configTemplateHasPath(parts) {
		return text, nil
	}

	scope, err := configTemplateScopeForRefs(c, store, parts)
	if err != nil {
		return nil, err
	}
	if len(parts) == 1 && parts[0].Path != "" {
		return resolveConfigTemplatePath(scope, parts[0].Path)
	}
	return renderParsedConfigTemplate(parts, scope)
}

func configTemplateHasPath(parts []configTemplatePart) bool {
	for _, part := range parts {
		if part.Path != "" {
			return true
		}
	}
	return false
}

func configTemplateScopeForRefs(c *core.Core, store *workspaceObjectStore, parts []configTemplatePart) (
	map[string]any, error,
) {
	scope := map[string]any{}
	seen := map[string]bool{}

	for _, part := range parts {
		if part.Path == "" || seen[part.Path] {
			continue
		}
		value, err := resolveConfigTemplateReference(c, store, part.Path)
		if err != nil {
			return nil, err
		}
		insertConfigTemplateValue(scope, part.Path, value)
		seen[part.Path] = true
	}

	return scope, nil
}

func resolveConfigTemplateReference(c *core.Core, store *workspaceObjectStore, path string) (
	any, error,
) {
	switch {
	case core.HasPrefix(path, "env."):
		key := core.Trim(path[len("env."):])
		if key == "" {
			return nil, missingConfigTemplatePath(path)
		}
		value := core.Env(key)
		if value == "" {
			return nil, missingConfigTemplatePath(path)
		}
		return value, nil
	case core.HasPrefix(path, "config."):
		key := core.Trim(path[len("config."):])
		if key == "" {
			return nil, missingConfigTemplatePath(path)
		}
		value, ok := coreConfigValue(c, key)
		if !ok {
			return nil, missingConfigTemplatePath(path)
		}
		return value, nil
	default:
		return resolveStoreConfigTemplateReference(store, path)
	}
}

func resolveStoreConfigTemplateReference(store *workspaceObjectStore, path string) (
	any, error,
) {
	segments, err := configTemplatePathSegments(path)
	if err != nil {
		return nil, err
	}
	if len(segments) < 2 {
		return nil, missingConfigTemplatePath(path)
	}
	if store == nil {
		return nil, core.E(
			"app.resolveStoreConfigTemplateReference",
			core.Sprintf("config template variable %q requires a hydrated workspace store", path),
			nil,
		)
	}

	raw, err := store.Get(segments[0], segments[1])
	if err != nil {
		return nil, missingConfigTemplatePath(path)
	}

	value := decodeConfigTemplateValue(raw)
	if len(segments) == 2 {
		return value, nil
	}
	return resolveConfigTemplateChild(value, path, segments[2:])
}

func renderConfigTemplateText(text string, scope map[string]any) (
	string, error,
) {
	parts, err := parseConfigTemplate(text)
	if err != nil {
		return "", err
	}
	return renderParsedConfigTemplate(parts, scope)
}

func renderParsedConfigTemplate(parts []configTemplatePart, scope map[string]any) (
	string, error,
) {
	builder := core.NewBuilder()
	for _, part := range parts {
		if part.Path == "" {
			builder.WriteString(part.Text)
			continue
		}
		value, err := resolveConfigTemplatePath(scope, part.Path)
		if err != nil {
			return "", err
		}
		text, err := configTemplateString(value)
		if err != nil {
			return "", err
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func parseConfigTemplate(text string) (
	[]configTemplatePart, error,
) {
	if text == "" {
		return nil, nil
	}

	var parts []configTemplatePart
	rest := text
	for len(rest) > 0 {
		open := stringIndex(rest, "{{")
		close := stringIndex(rest, "}}")
		if close >= 0 && (open < 0 || close < open) {
			return nil, core.E("app.parseConfigTemplate", "malformed template: unexpected closing delimiter", nil)
		}
		if open < 0 {
			parts = append(parts, configTemplatePart{Text: rest})
			break
		}
		if open > 0 {
			parts = append(parts, configTemplatePart{Text: rest[:open]})
		}

		tail := rest[open+2:]
		end := stringIndex(tail, "}}")
		if end < 0 {
			return nil, core.E("app.parseConfigTemplate", "malformed template: unclosed action", nil)
		}

		rawPath := core.Trim(tail[:end])
		if core.Contains(rawPath, "{{") || core.Contains(rawPath, "}}") {
			return nil, core.E("app.parseConfigTemplate", "malformed template: nested delimiters", nil)
		}

		path, err := normaliseConfigTemplatePath(rawPath)
		if err != nil {
			return nil, err
		}
		parts = append(parts, configTemplatePart{Path: path})
		rest = tail[end+2:]
	}

	return parts, nil
}

func normaliseConfigTemplatePath(path string) (
	string, error,
) {
	path = core.Trim(path)
	path = core.TrimPrefix(path, ".")
	if path == "" || stringIndexAny(path, " \t\r\n") >= 0 {
		return "", core.E("app.normaliseConfigTemplatePath", "malformed template action", nil)
	}

	segments, err := configTemplatePathSegments(path)
	if err != nil {
		return "", err
	}
	return core.Join(".", segments...), nil
}

func configTemplatePathSegments(path string) (
	[]string, error,
) {
	path = core.Trim(path)
	path = core.TrimPrefix(path, ".")
	if path == "" {
		return nil, core.E("app.configTemplatePathSegments", "empty template path", nil)
	}

	raw := core.Split(path, ".")
	segments := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = core.Trim(segment)
		if segment == "" {
			return nil, core.E(
				"app.configTemplatePathSegments",
				core.Sprintf("malformed template path %q", path),
				nil,
			)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func resolveConfigTemplatePath(scope map[string]any, path string) (
	any, error,
) {
	if len(scope) == 0 {
		return nil, missingConfigTemplatePath(path)
	}

	segments, err := configTemplatePathSegments(path)
	if err != nil {
		return nil, err
	}
	return resolveConfigTemplateChild(scope, path, segments)
}

func resolveConfigTemplateChild(current any, path string, segments []string) (
	any, error,
) {
	if len(segments) == 0 {
		return current, nil
	}

	switch node := current.(type) {
	case map[string]any:
		next, ok := node[segments[0]]
		if !ok {
			return nil, missingConfigTemplatePath(path)
		}
		return resolveConfigTemplateChild(next, path, segments[1:])
	case map[string]string:
		next, ok := node[segments[0]]
		if !ok {
			return nil, missingConfigTemplatePath(path)
		}
		return resolveConfigTemplateChild(next, path, segments[1:])
	case []any:
		index, err := strconv.Atoi(segments[0])
		if err != nil || index < 0 || index >= len(node) {
			return nil, missingConfigTemplatePath(path)
		}
		return resolveConfigTemplateChild(node[index], path, segments[1:])
	case string:
		var decoded any
		if r := core.JSONUnmarshal([]byte(node), &decoded); !r.OK {
			return nil, missingConfigTemplatePath(path)
		}
		return resolveConfigTemplateChild(decoded, path, segments)
	default:
		return nil, missingConfigTemplatePath(path)
	}
}

func insertConfigTemplateValue(scope map[string]any, path string, value any) {
	segments, err := configTemplatePathSegments(path)
	if err != nil || len(segments) == 0 {
		return
	}

	current := scope
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}

func missingConfigTemplatePath(
	path string,
) error {
	return core.E(
		"app.missingConfigTemplatePath",
		core.Sprintf("missing config template variable %q", path),
		nil,
	)
}

func decodeConfigTemplateValue(raw string) any {
	if core.Trim(raw) == "" {
		return ""
	}

	var decoded any
	if r := core.JSONUnmarshal([]byte(raw), &decoded); r.OK {
		return decoded
	}
	return raw
}

func configTemplateString(value any) (
	string, error,
) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return text, nil
	}

	kind := reflect.TypeOf(value).Kind()
	switch kind {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		r := core.JSONMarshal(value)
		if !r.OK {
			cause, _ := r.Value.(error)
			return "", core.E("app.configTemplateString", "marshal template value failed", cause)
		}
		body, _ := r.Value.([]byte)
		return string(body), nil
	default:
		return core.Sprint(value), nil
	}
}
