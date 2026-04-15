// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
	"gopkg.in/yaml.v3"
)

// yamlMarshalBytes is a thin wrapper over gopkg.in/yaml.v3 Marshal so
// every packaging file shares one entry point. Keeps the encoder swap
// cheap — change this one function and every wrap/install path
// follows.
//
//	body, err := yamlMarshalBytes(manifest)
func yamlMarshalBytes(v any) ([]byte, error) {
	switch m := v.(type) {
	case *config.ViewManifest:
		return marshalViewManifest(m)
	case config.ViewManifest:
		return marshalViewManifest(&m)
	}
	return yaml.Marshal(v)
}

// marshalViewManifest emits a config.ViewManifest using the RFC-native
// field layout where possible:
//
//   - `permissions.write` and `permissions.store` are hoisted out of
//     Config and back into the permissions map.
//
//   - spec-facing top-level fields such as `services`, `type`, `url`,
//     `theme`, `locale`, `icon`, `shim` and `ipc_channels` are hoisted
//     out of Config.
//
// Internal bookkeeping keys (`source`, `category`, `window_mode`,
// `gui_gates`, ...) remain under `config:` because they are framework
// metadata, not part of the public manifest contract.
func marshalViewManifest(m *config.ViewManifest) ([]byte, error) {
	if m == nil {
		return yaml.Marshal(nil)
	}

	doc := map[string]any{}
	if m.Code != "" {
		doc["code"] = m.Code
	}
	if m.Name != "" {
		doc["name"] = m.Name
	}
	if m.Version != "" {
		doc["version"] = m.Version
	}
	if m.Sign != "" {
		doc["sign"] = m.Sign
	}
	if m.Title != "" {
		doc["title"] = m.Title
	}
	if m.Width != 0 {
		doc["width"] = m.Width
	}
	if m.Height != 0 {
		doc["height"] = m.Height
	}
	if m.Resizable {
		doc["resizable"] = true
	}
	if m.Layout != "" {
		doc["layout"] = m.Layout
	}
	if len(m.Slots) > 0 {
		doc["slots"] = m.Slots
	}
	if len(m.Modules) > 0 {
		doc["modules"] = append([]string(nil), m.Modules...)
	}

	if perms := manifestPermissionsForYAML(m); len(perms) > 0 {
		doc["permissions"] = perms
	}

	for _, key := range manifestTopLevelConfigKeys {
		if v, ok := manifestConfigValue(m, key); ok {
			doc[key] = v
		}
	}

	if cfg := manifestConfigForYAML(m); len(cfg) > 0 {
		doc["config"] = cfg
	}

	return yaml.Marshal(doc)
}

// yamlUnmarshalImpl is the matching wrapper for yaml.Unmarshal so the
// encoder swap stays mechanical. Used by marketplace_verify.go and any
// other path that needs to round-trip a manifest body in memory.
//
//	_ = yamlUnmarshalImpl(body, &manifest)
func yamlUnmarshalImpl(body []byte, dst any) error {
	return yaml.Unmarshal(body, dst)
}

// LoadViewManifest reads a `.core/view.yaml` manifest from disk and
// hydrates RFC-native fields that config.ViewManifest does not yet type
// directly:
//
//   - `permissions.write`   → Config["write"]
//   - `permissions.store`   → Config["store"]
//   - top-level `services`  → Config["services"]
//   - top-level `type/url/theme/...` → Config mirror
//
// The loader is intentionally app-local so core/app can honor the RFC
// immediately without waiting for the upstream schema to grow.
func LoadViewManifest(medium coreio.Medium, path string, dst *config.ViewManifest) error {
	if medium == nil {
		medium = coreio.Local
	}
	if path == "" {
		return coreerr.E("app.LoadViewManifest", "empty path", nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return coreerr.E("app.LoadViewManifest", "read "+path+" failed", err)
	}
	if err := UnmarshalViewManifest([]byte(body), dst); err != nil {
		return coreerr.E("app.LoadViewManifest", "parse "+path+" failed", err)
	}
	return nil
}

// UnmarshalViewManifest is the in-memory counterpart to
// LoadViewManifest. It first hydrates the typed config.ViewManifest, then
// folds RFC-native compatibility fields into Config / ViewPermissions so
// the rest of core/app can consume one consistent shape.
func UnmarshalViewManifest(body []byte, dst *config.ViewManifest) error {
	if dst == nil {
		return coreerr.E("app.UnmarshalViewManifest", "nil destination", nil)
	}
	if err := yamlUnmarshalImpl(body, dst); err != nil {
		return err
	}

	var raw map[string]any
	if err := yamlUnmarshalImpl(body, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}

	mergeManifestTopLevelExtras(dst, raw)
	mergeManifestPermissionExtras(dst, raw["permissions"])
	if permissionListContains(dst.Permissions.Net, "*") {
		dst.Permissions.Network = true
	}
	return nil
}

var manifestTypedTopLevelKeys = map[string]bool{
	"code":        true,
	"name":        true,
	"version":     true,
	"sign":        true,
	"title":       true,
	"width":       true,
	"height":      true,
	"resizable":   true,
	"layout":      true,
	"slots":       true,
	"modules":     true,
	"permissions": true,
	"config":      true,
}

var manifestTopLevelConfigKeys = []string{
	"services",
	"type",
	"url",
	"theme",
	"locale",
	"icon",
	"shim",
	"ipc_channels",
}

func mergeManifestTopLevelExtras(dst *config.ViewManifest, raw map[string]any) {
	if dst == nil || len(raw) == 0 {
		return
	}
	cfg := ensureManifestConfig(dst)
	for key, value := range raw {
		if manifestTypedTopLevelKeys[key] {
			continue
		}
		cfg[key] = value
	}
}

func mergeManifestPermissionExtras(dst *config.ViewManifest, raw any) {
	if dst == nil || raw == nil {
		return
	}
	perms, ok := raw.(map[string]any)
	if !ok {
		return
	}
	cfg := ensureManifestConfig(dst)
	for key, value := range perms {
		switch key {
		case "write":
			cfg["write"] = value
		case "store":
			cfg["store"] = value
		case "location", "device.location":
			if truthy(value) {
				appendUniqueString(&dst.Permissions.Run, "device.location")
			}
		case "device.camera":
			if truthy(value) {
				dst.Permissions.Camera = true
			}
		case "device.microphone":
			if truthy(value) {
				dst.Permissions.Microphone = true
			}
		default:
			if !truthy(value) || !core.HasPrefix(key, "gui.") {
				continue
			}
			mergeManifestGUIGate(dst, key)
			switch key {
			case "gui.notification.send":
				dst.Permissions.Notifications = true
			case "gui.clipboard.read", "gui.clipboard.write":
				dst.Permissions.Clipboard = true
			case "gui.browser.open":
				dst.Permissions.Network = true
			}
		}
	}
}

func ensureManifestConfig(m *config.ViewManifest) map[string]any {
	if m.Config == nil {
		m.Config = map[string]any{}
	}
	return m.Config
}

func mergeManifestGUIGate(m *config.ViewManifest, gate string) {
	if m == nil || gate == "" {
		return
	}
	cfg := ensureManifestConfig(m)
	switch cur := cfg["gui_gates"].(type) {
	case map[string]any:
		cur[gate] = true
	case map[string]bool:
		next := make(map[string]any, len(cur)+1)
		for k, v := range cur {
			next[k] = v
		}
		next[gate] = true
		cfg["gui_gates"] = next
	default:
		cfg["gui_gates"] = map[string]any{gate: true}
	}
}

func permissionListContains(list []string, want string) bool {
	for _, entry := range list {
		if entry == want {
			return true
		}
	}
	return false
}

func appendUniqueString(dst *[]string, value string) {
	if dst == nil || value == "" {
		return
	}
	for _, existing := range *dst {
		if existing == value {
			return
		}
	}
	*dst = append(*dst, value)
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func manifestPermissionsForYAML(m *config.ViewManifest) map[string]any {
	if m == nil {
		return nil
	}
	perms := map[string]any{}
	guiGates := manifestGUIGates(m)
	if m.Permissions.Clipboard &&
		!truthy(guiGates["gui.clipboard.read"]) &&
		!truthy(guiGates["gui.clipboard.write"]) {
		perms["clipboard"] = true
	}
	if m.Permissions.Filesystem {
		perms["filesystem"] = true
	}
	if m.Permissions.Network && !permissionListContains(m.Permissions.Net, "*") {
		perms["network"] = true
	}
	if m.Permissions.Notifications && !truthy(guiGates["gui.notification.send"]) {
		perms["notifications"] = true
	}
	if m.Permissions.Camera {
		perms["camera"] = true
	}
	if m.Permissions.Microphone {
		perms["microphone"] = true
	}
	if len(m.Permissions.Read) > 0 {
		perms["read"] = append([]string(nil), m.Permissions.Read...)
	}
	if len(m.Permissions.Net) > 0 {
		perms["net"] = append([]string(nil), m.Permissions.Net...)
	}
	if len(m.Permissions.Run) > 0 {
		perms["run"] = append([]string(nil), m.Permissions.Run...)
	}
	if v, ok := manifestConfigValue(m, "write"); ok {
		perms["write"] = v
	}
	if v, ok := manifestConfigValue(m, "store"); ok {
		perms["store"] = v
	}
	if len(guiGates) > 0 {
		for key, value := range guiGates {
			perms[key] = value
		}
	}
	if len(perms) == 0 {
		return nil
	}
	return perms
}

func manifestConfigForYAML(m *config.ViewManifest) map[string]any {
	if m == nil || len(m.Config) == 0 {
		return nil
	}
	out := make(map[string]any, len(m.Config))
	for key, value := range m.Config {
		out[key] = value
	}
	for _, key := range manifestTopLevelConfigKeys {
		delete(out, key)
	}
	delete(out, "write")
	delete(out, "store")
	if len(out) == 0 {
		return nil
	}
	return out
}

func manifestConfigValue(m *config.ViewManifest, key string) (any, bool) {
	if m == nil || m.Config == nil {
		return nil, false
	}
	v, ok := m.Config[key]
	if !ok {
		return nil, false
	}
	return v, true
}

func manifestGUIGates(m *config.ViewManifest) map[string]any {
	if m == nil || m.Config == nil {
		return nil
	}
	switch raw := m.Config["gui_gates"].(type) {
	case map[string]any:
		out := map[string]any{}
		for key, value := range raw {
			if truthy(value) {
				out[key] = true
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]bool:
		out := map[string]any{}
		for key, value := range raw {
			if value {
				out[key] = true
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}
