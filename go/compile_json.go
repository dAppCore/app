// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
)

// compiledTypedTopLevelKeys are the fields CompiledManifest models
// directly. Everything else at the JSON top level is mirrored into the
// Config map so spec-facing keys (`services`, `type`, `url`, ...)
// survive the compile → core.json → boot round-trip even before the
// upstream schema grows typed slots for them.
var compiledTypedTopLevelKeys = map[string]bool{
	"code":        true,
	"name":        true,
	"version":     true,
	"sign":        true,
	"compiled_at": true,
	"compiled_by": true,
	"layout":      true,
	"slots":       true,
	"permissions": true,
	"modules":     true,
	"components":  true,
	"config":      true,
}

type compiledManifestAlias CompiledManifest

// MarshalJSON emits the RFC-facing core.json shape. Compatibility
// fields currently stored in Config (permissions.write, permissions.store,
// top-level services/type/url/theme/...) are hoisted back out so the
// compiled artifact mirrors the public manifest contract rather than an
// app-internal storage detail.
func (
	cm CompiledManifest,
) MarshalJSON() ([]byte, error) {
	doc := map[string]any{
		"code":        cm.Code,
		"name":        cm.Name,
		"version":     cm.Version,
		"compiled_at": cm.CompiledAt,
		"compiled_by": cm.CompiledBy,
	}
	if cm.Sign != "" {
		doc["sign"] = cm.Sign
	}
	if cm.Layout != "" {
		doc["layout"] = cm.Layout
	}
	if len(cm.Slots) > 0 {
		doc["slots"] = cm.Slots
	}
	perms := compiledPermissionsForJSON(&cm)
	if perms == nil {
		perms = map[string]any{}
	}
	doc["permissions"] = perms
	if len(cm.Modules) > 0 {
		doc["modules"] = cm.Modules
	}
	if len(cm.Components) > 0 {
		doc["components"] = cm.Components
	}
	for _, key := range manifestTopLevelConfigKeys {
		if value, ok := compiledConfigValue(&cm, key); ok {
			doc[key] = value
		}
	}
	if cfg := compiledConfigForJSON(&cm); len(cfg) > 0 {
		doc["config"] = cfg
	}
	r := core.JSONMarshal(doc)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, cause
	}
	body, _ := r.Value.([]byte)
	return body, nil
}

// UnmarshalJSON accepts both the legacy core.json shape (compatibility
// fields under Config) and the RFC-facing shape where those fields live
// alongside the typed manifest fields.
func (
	cm *CompiledManifest,
) UnmarshalJSON(body []byte) error {
	if cm == nil {
		return core.E("app.CompiledManifest.UnmarshalJSON", "nil receiver", nil)
	}

	var alias compiledManifestAlias
	if r := core.JSONUnmarshal(body, &alias); !r.OK {
		cause, _ := r.Value.(error)
		return cause
	}
	*cm = CompiledManifest(alias)

	var raw map[string]any
	if r := core.JSONUnmarshal(body, &raw); !r.OK {
		cause, _ := r.Value.(error)
		return cause
	}
	if len(raw) == 0 {
		return nil
	}

	mergeCompiledTopLevelExtras(cm, raw)
	mergeCompiledPermissionExtras(cm, raw["permissions"])
	if permissionListContains(cm.Permissions.Net, "*") {
		cm.Permissions.Network = true
	}
	return nil
}

func compiledPermissionsForJSON(cm *CompiledManifest) map[string]any {
	if cm == nil {
		return nil
	}
	view := compiledToManifest(cm)
	return manifestPermissionsForYAML(&view)
}

func compiledConfigForJSON(cm *CompiledManifest) map[string]any {
	if cm == nil {
		return nil
	}
	view := compiledToManifest(cm)
	return manifestConfigForYAML(&view)
}

func compiledConfigValue(cm *CompiledManifest, key string) (any, bool) {
	if cm == nil {
		return nil, false
	}
	view := compiledToManifest(cm)
	return manifestConfigValue(&view, key)
}

func mergeCompiledTopLevelExtras(dst *CompiledManifest, raw map[string]any) {
	if dst == nil || len(raw) == 0 {
		return
	}
	cfg := ensureCompiledConfig(dst)
	for key, value := range raw {
		if compiledTypedTopLevelKeys[key] {
			continue
		}
		cfg[key] = value
	}
}

func mergeCompiledPermissionExtras(dst *CompiledManifest, raw any) {
	if dst == nil || raw == nil {
		return
	}
	view := compiledToManifest(dst)
	mergeManifestPermissionExtras(&view, raw)
	dst.Permissions = view.Permissions
	dst.Config = copyConfig(view.Config)
}

func ensureCompiledConfig(cm *CompiledManifest) map[string]any {
	if cm == nil {
		return nil
	}
	if cm.Config == nil {
		cm.Config = map[string]any{}
	}
	return cm.Config
}
