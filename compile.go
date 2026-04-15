// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"time"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// CompiledVersion is the compiler identity stapled into every core.json
// under `compiled_by`. Bump in lockstep with core itself so downstream
// runtimes can reject compiled artifacts older than the minimum supported
// version.
//
//	core.json → "compiled_by": "core v0.8.0-alpha.1"
const CompiledVersion = "core v0.8.0-alpha.1"

// CompiledFileName is the distribution-ready artifact name written at the
// project root (not inside .core/). Matches RFC §3.1.
//
//	coreio.Local.Read(core.Path(root, app.CompiledFileName))
const CompiledFileName = "core.json"

// ComponentSpec describes one Web Component entry in the compiled
// manifest. The name is the slot's mapped value (e.g. "nav-breadcrumb");
// the struct records the custom-element tag and whether a shadow DOM is
// the default mount mode.
//
//	"components": { "nav-breadcrumb": { "tag": "nav-breadcrumb", "shadow": true } }
type ComponentSpec struct {
	Tag    string `json:"tag"`    // Web Component custom element tag
	Shadow bool   `json:"shadow"` // true → shadow DOM isolation
}

// CompiledManifest is the JSON shape of core.json from RFC §3.1. It
// mirrors the signed `view.yaml` fields plus compile-time metadata and a
// resolved `components` map the runtime uses to mount slots without
// parsing YAML on boot.
//
// The struct deliberately does not embed config.ViewManifest verbatim —
// the runtime artefact carries only the fields a CoreApp host needs at
// launch. Fields are JSON-first (yaml tags omitted).
//
//	cm := app.Compile(&manifest, app.CompileOptions{})
//	r := core.JSONMarshal(cm) // "core.json" bytes
//
// Config is copied verbatim from the source manifest so the boot-time
// template renderer (RFC §4.1 step 6) and the permission gate's
// Config-backed flags (`store`, `write`, `services`, …) survive the
// compile→core.json→Boot round-trip. Without this the runtime would
// silently lose any template block or wrap provenance the manifest
// recorded.
type CompiledManifest struct {
	Code        string                   `json:"code"`
	Name        string                   `json:"name"`
	Version     string                   `json:"version"`
	Sign        string                   `json:"sign,omitempty"`
	CompiledAt  string                   `json:"compiled_at"`
	CompiledBy  string                   `json:"compiled_by"`
	Layout      string                   `json:"layout,omitempty"`
	Slots       map[string]string        `json:"slots,omitempty"`
	Permissions config.ViewPermissions   `json:"permissions"`
	Modules     []string                 `json:"modules,omitempty"`
	Components  map[string]ComponentSpec `json:"components,omitempty"`
	Config      map[string]any           `json:"config,omitempty"`
}

// CompileOptions tunes the Compile step. Zero-value options produce a
// deterministic core.json using time.Now().UTC() and CompiledVersion.
//
//	opts := app.CompileOptions{
//	    Now:        func() time.Time { return fixedTime },
//	    CompiledBy: "core v0.8.0",
//	}
type CompileOptions struct {
	// Now lets tests pin the timestamp. Nil → time.Now().
	Now func() time.Time
	// CompiledBy overrides CompiledVersion — useful when a host
	// embeds app and wants its own identity on the artifact.
	CompiledBy string
}

// Compile turns a parsed ViewManifest into a CompiledManifest ready for
// JSON emission. The input manifest is not mutated; Compile copies every
// field it cares about so the caller retains the original source.
//
//	cm, err := app.Compile(&manifest, app.CompileOptions{})
//	if err != nil { return err }
//	j := core.JSONMarshalString(cm) // writable to core.json
//
// Rules:
//
//   - nil manifest → error (no silent zero value).
//
//   - empty code or version → error (the runtime cannot boot without
//     identity).
//
//   - empty name → error (RFC §2.1 marks the human-readable identity as
//     required alongside code + version).
//
//   - layout slots whose component value is not a string → error
//     (config.ViewManifest.Slots is map[string]any for YAML flexibility;
//     the compiled form is strict).
func Compile(m *config.ViewManifest, opts CompileOptions) (*CompiledManifest, error) {
	if m == nil {
		return nil, coreerr.E("app.Compile", "nil manifest", nil)
	}
	if m.Code == "" {
		return nil, coreerr.E("app.Compile", "manifest.code is empty", nil)
	}
	if m.Name == "" {
		return nil, coreerr.E("app.Compile", "manifest.name is empty", nil)
	}
	if m.Version == "" {
		return nil, coreerr.E("app.Compile", "manifest.version is empty", nil)
	}
	if err := validateLayoutVariant(m.Layout); err != nil {
		return nil, coreerr.E("app.Compile", "invalid layout", err)
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}
	compiledBy := opts.CompiledBy
	if compiledBy == "" {
		compiledBy = CompiledVersion
	}

	slots, components, err := resolveSlots(m.Slots)
	if err != nil {
		return nil, coreerr.E("app.Compile", "slot resolution failed", err)
	}

	out := &CompiledManifest{
		Code:        m.Code,
		Name:        m.Name,
		Version:     m.Version,
		Sign:        m.Sign,
		CompiledAt:  now().UTC().Format(time.RFC3339),
		CompiledBy:  compiledBy,
		Layout:      m.Layout,
		Slots:       slots,
		Permissions: m.Permissions,
		Modules:     append([]string(nil), m.Modules...),
		Components:  components,
		Config:      copyConfig(m.Config),
	}
	return out, nil
}

// copyConfig returns a shallow clone of the manifest's Config map so a
// Compile caller can mutate their source manifest afterwards without
// affecting the emitted artefact (and vice-versa). A nil input stays
// nil — we never promote an empty map into core.json for the sake of a
// single level of defensive copying.
//
//	cm.Config = copyConfig(m.Config)
func copyConfig(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// resolveSlots narrows the YAML-flexible `map[string]any` into a strict
// `map[string]string` (slot → component name) and derives the default
// components map the runtime consumes. A component with no explicit
// `shadow` setting defaults to true — matching the RFC §3.1 example.
//
// Returns (slots, components, err).
func resolveSlots(raw map[string]any) (map[string]string, map[string]ComponentSpec, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}

	slots := make(map[string]string, len(raw))
	components := make(map[string]ComponentSpec, len(raw))

	for slot, v := range raw {
		switch value := v.(type) {
		case string:
			slots[slot] = value
			components[value] = ComponentSpec{Tag: value, Shadow: true}
		case nil:
			return nil, nil, coreerr.E(
				"app.resolveSlots",
				"slot '"+slot+"' has nil component",
				nil,
			)
		default:
			return nil, nil, coreerr.E(
				"app.resolveSlots",
				"slot '"+slot+"' must be a string component name",
				nil,
			)
		}
	}
	return slots, components, nil
}

// WriteCompiled writes a CompiledManifest to `<root>/core.json` on the
// supplied medium. Pretty-printed JSON keeps diff-review pleasant and
// matches dAppServer's committed-artifact style.
//
//	err := app.WriteCompiled(coreio.Local, root, cm)
func WriteCompiled(medium coreio.Medium, root string, cm *CompiledManifest) error {
	if cm == nil {
		return coreerr.E("app.WriteCompiled", "nil compiled manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	body, err := marshalPretty(cm)
	if err != nil {
		return coreerr.E("app.WriteCompiled", "marshal failed", err)
	}
	path := core.Path(root, CompiledFileName)
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return coreerr.E("app.WriteCompiled", "ensure dir failed", err)
	}
	if err := medium.Write(path, body); err != nil {
		return coreerr.E("app.WriteCompiled", "write failed", err)
	}
	return nil
}

// LoadCompiled reads `<root>/core.json` and decodes it into a
// CompiledManifest. Returns an error rather than a zero value when the
// file is absent so the Boot path can decide whether to fall back to
// `.core/view.yaml`.
//
//	cm, err := app.LoadCompiled(coreio.Local, root)
func LoadCompiled(medium coreio.Medium, root string) (*CompiledManifest, error) {
	if medium == nil {
		medium = coreio.Local
	}
	path := core.Path(root, CompiledFileName)
	if !medium.Exists(path) {
		return nil, coreerr.E("app.LoadCompiled", CompiledFileName+" not found at "+path, nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, coreerr.E("app.LoadCompiled", "read "+path+" failed", err)
	}
	var cm CompiledManifest
	if r := core.JSONUnmarshal([]byte(body), &cm); !r.OK {
		cause, _ := r.Value.(error)
		return nil, coreerr.E("app.LoadCompiled", "decode "+path+" failed", cause)
	}
	return &cm, nil
}

// marshalPretty produces a two-space-indented JSON string. Kept as a
// package-internal helper rather than a core.* method so the indent
// policy is in one place when core/go grows a JSONMarshalIndent.
//
//	body, _ := marshalPretty(cm)
func marshalPretty(v any) (string, error) {
	r := core.JSONMarshal(v)
	if !r.OK {
		cause, _ := r.Value.(error)
		return "", cause
	}
	raw, _ := r.Value.([]byte)
	// Re-encode with indent via a second pass so we don't import
	// encoding/json directly here (core banned-import rule).
	return indentJSON(raw), nil
}

// indentJSON rewrites compact JSON bytes with two-space indentation. The
// implementation is a tiny stateful walker that tracks depth and string
// scope — enough for the core.json shape (objects, arrays, string/number
// primitives) without pulling a parser.
//
//	indentJSON([]byte(`{"a":1}`)) // "{\n  \"a\": 1\n}"
func indentJSON(in []byte) string {
	var (
		out     []byte
		depth   int
		inStr   bool
		escape  bool
		indent  = []byte("  ")
		newline = byte('\n')
	)
	writeIndent := func() {
		for i := 0; i < depth; i++ {
			out = append(out, indent...)
		}
	}
	for _, b := range in {
		if escape {
			out = append(out, b)
			escape = false
			continue
		}
		if inStr {
			out = append(out, b)
			switch b {
			case '\\':
				escape = true
			case '"':
				inStr = false
			}
			continue
		}
		switch b {
		case '"':
			inStr = true
			out = append(out, b)
		case '{', '[':
			out = append(out, b)
			depth++
			out = append(out, newline)
			writeIndent()
		case '}', ']':
			depth--
			out = append(out, newline)
			writeIndent()
			out = append(out, b)
		case ',':
			out = append(out, b)
			out = append(out, newline)
			writeIndent()
		case ':':
			out = append(out, b, ' ')
		default:
			out = append(out, b)
		}
	}
	return string(out)
}
