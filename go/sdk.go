// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

// SDKLanguage selects which client SDK the generator emits. Each language
// corresponds to a different set of files written under the SDK output
// directory; see SDKGenerate for the layout convention.
//
//	core-app sdk generate --lang ts
//	core-app sdk generate --lang openapi
type SDKLanguage int

const (
	// SDKLanguageOpenAPI emits an OpenAPI 3.1 spec covering the manifest's
	// declared Action surface. The spec is the source of truth for every
	// downstream codegen and lives at `<out>/openapi.json`.
	SDKLanguageOpenAPI SDKLanguage = iota
	// SDKLanguageTypeScript emits a TypeScript module exporting typed
	// wrappers over the Action surface (`fs.read`, `store.get`, …). The
	// module ships at `<out>/index.ts`.
	SDKLanguageTypeScript
	// SDKLanguageGo emits a Go package mirroring the TypeScript surface so
	// CoreApp authors writing Go services can dispatch Actions with the
	// same typed API they used in the frontend. Lives at `<out>/sdk.go`.
	SDKLanguageGo
	// SDKLanguagePHP emits a PHP class with method-per-action helpers for
	// CorePHP consumers (Laravel facade pattern). Lives at `<out>/Sdk.php`.
	SDKLanguagePHP
	// SDKLanguagePython emits a Python module with one function per Action
	// for CorePy / data-science consumers. Completes the four-language
	// SDK pipeline described in RFC §8 ("Client SDKs (TS, Python, Go,
	// PHP)"). Lives at `<out>/sdk.py`.
	SDKLanguagePython
)

// String returns the lowercase token that callers pass on the CLI and
// that the marketplace index records when surfacing SDK availability.
//
//	app.SDKLanguageTypeScript.String() // "ts"
func (l SDKLanguage) String() string {
	switch l {
	case SDKLanguageOpenAPI:
		return "openapi"
	case SDKLanguageTypeScript:
		return "ts"
	case SDKLanguageGo:
		return "go"
	case SDKLanguagePHP:
		return "php"
	case SDKLanguagePython:
		return "python"
	default:
		return "unknown"
	}
}

// ParseSDKLanguage maps a CLI token back to an SDKLanguage. Returns
// (zero, false) for unknown values so the caller can print a useful
// "supported languages" error rather than silently defaulting.
//
//	lang, ok := app.ParseSDKLanguage("ts")
func ParseSDKLanguage(s string) (SDKLanguage, bool) {
	switch core.Lower(core.Trim(s)) {
	case "openapi", "oas", "swagger":
		return SDKLanguageOpenAPI, true
	case "ts", "typescript":
		return SDKLanguageTypeScript, true
	case "go", "golang":
		return SDKLanguageGo, true
	case "php":
		return SDKLanguagePHP, true
	case "py", "python":
		return SDKLanguagePython, true
	}
	return 0, false
}

// SDKAction describes a single Named Action exposed to consumers. The
// field set mirrors what a typed binding needs: the dotted action name,
// a free-form description, the permission it gates against, and the
// expected request / response argument shapes.
//
//	a := app.SDKAction{
//	    Name: "fs.read", Permission: "read",
//	    Description: "Read a file from the sandbox",
//	    Request: []app.SDKArg{{Name: "path", Type: "string", Required: true}},
//	    Response: []app.SDKArg{{Name: "content", Type: "string"}},
//	}
type SDKAction struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permission  string   `json:"permission,omitempty"`
	Request     []SDKArg `json:"request,omitempty"`
	Response    []SDKArg `json:"response,omitempty"`
}

// SDKArg names one parameter (request or response) of a Named Action.
// Type is the JSON Schema primitive name (`string`, `integer`, `boolean`,
// `object`); the generator does not yet emit nested object schemas — the
// RFC §9 contract is parameter-flat by design.
type SDKArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// SDKGenerateOptions tunes the generator. The zero value emits the
// OpenAPI spec plus all four client SDKs (TS, Go, PHP, Python) into
// `<root>/.core/sdk/<lang>/` next to the manifest source, matching the
// RFC §8 pipeline output.
//
//	opts := app.SDKGenerateOptions{
//	    Languages: []app.SDKLanguage{app.SDKLanguageTypeScript},
//	    OutDir:    "/tmp/sdk",
//	}
type SDKGenerateOptions struct {
	// Languages selects which client SDKs to emit. Empty means all four.
	Languages []SDKLanguage
	// OutDir overrides the default `<root>/.core/sdk/` location.
	OutDir string
	// Actions overrides the default action surface (the Core primitive
	// list from RFC §9.3 filtered by manifest permissions). Hosts that
	// register additional Actions feed them in here so the generated
	// SDK covers their custom surface too.
	Actions []SDKAction
	// IncludeAllPrimitives ignores the manifest's permission gates and
	// emits the full Core primitive surface. Useful for IDE integrations
	// that show the full action catalogue regardless of what an app
	// happens to declare.
	IncludeAllPrimitives bool
}

// DefaultSDKActions returns the Core primitive Actions described in
// RFC §9.3. Returned by value so callers can mutate without aliasing
// the package-level constant.
//
//	primitives := app.DefaultSDKActions()
//	primitives = append(primitives, customAction)
func DefaultSDKActions() []SDKAction {
	return []SDKAction{
		{
			Name: "fs.read", Permission: "read",
			Description: "Read a file from the sandbox",
			Request:     []SDKArg{{Name: "path", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "content", Type: "string"}},
		},
		{
			Name: "fs.write", Permission: "write",
			Description: "Write a file in the sandbox",
			Request: []SDKArg{
				{Name: "path", Type: "string", Required: true},
				{Name: "content", Type: "string", Required: true},
			},
		},
		{
			Name: "fs.list", Permission: "read",
			Description: "List a directory in the sandbox",
			Request:     []SDKArg{{Name: "path", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "entries", Type: "array"}},
		},
		{
			Name: "fs.delete", Permission: "write",
			Description: "Delete a file in the sandbox",
			Request:     []SDKArg{{Name: "path", Type: "string", Required: true}},
		},
		{
			Name: "store.get", Permission: "store",
			Description: "Read a value from the object store",
			Request: []SDKArg{
				{Name: "group", Type: "string", Required: true},
				{Name: "key", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "value", Type: "string"}},
		},
		{
			Name: "store.set", Permission: "store",
			Description: "Write a value to the object store",
			Request: []SDKArg{
				{Name: "group", Type: "string", Required: true},
				{Name: "key", Type: "string", Required: true},
				{Name: "value", Type: "string", Required: true},
			},
		},
		{
			Name: "store.delete", Permission: "store",
			Description: "Delete a value from the object store",
			Request: []SDKArg{
				{Name: "group", Type: "string", Required: true},
				{Name: "key", Type: "string", Required: true},
			},
		},
		{
			Name: "net.fetch", Permission: "net",
			Description: "Perform an HTTP request",
			Request: []SDKArg{
				{Name: "url", Type: "string", Required: true},
				{Name: "method", Type: "string"},
				{Name: "body", Type: "string"},
			},
			Response: []SDKArg{
				{Name: "status", Type: "integer"},
				{Name: "body", Type: "string"},
			},
		},
		{
			Name: "net.ws", Permission: "net",
			Description: "Open a WebSocket connection",
			Request:     []SDKArg{{Name: "url", Type: "string", Required: true}},
		},
		{
			Name: "process.run", Permission: "run",
			Description: "Execute an external command",
			Request: []SDKArg{
				{Name: "command", Type: "string", Required: true},
				{Name: "args", Type: "array"},
			},
			Response: []SDKArg{
				{Name: "stdout", Type: "string"},
				{Name: "stderr", Type: "string"},
				{Name: "exit", Type: "integer"},
			},
		},
		{
			Name: "process.add", Permission: "run",
			Description: "Register a long-lived process under a key (RFC §9.4)",
			Request: []SDKArg{
				{Name: "key", Type: "string", Required: true},
				{Name: "command", Type: "string", Required: true},
				{Name: "args", Type: "array"},
			},
			Response: []SDKArg{{Name: "key", Type: "string"}},
		},
		{
			Name: "process.start", Permission: "run",
			Description: "Start a previously-registered process by key",
			Request:     []SDKArg{{Name: "key", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "started", Type: "boolean"}},
		},
		{
			Name: "process.stop", Permission: "run",
			Description: "Gracefully stop a registered process by key",
			Request:     []SDKArg{{Name: "key", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "stopped", Type: "boolean"}},
		},
		{
			Name: "process.kill", Permission: "run",
			Description: "Forcibly kill a registered process by key",
			Request:     []SDKArg{{Name: "key", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "killed", Type: "boolean"}},
		},
		{
			Name: "process.list", Permission: "run",
			Description: "List the keys of every registered process",
			Response:    []SDKArg{{Name: "keys", Type: "array"}},
		},
		{
			Name: "process.get", Permission: "run",
			Description: "Read a registered process's current info",
			Request:     []SDKArg{{Name: "key", Type: "string", Required: true}},
			Response: []SDKArg{
				{Name: "key", Type: "string"},
				{Name: "command", Type: "string"},
				{Name: "running", Type: "boolean"},
				{Name: "exit", Type: "integer"},
			},
		},
		{
			Name: "process.stdout.subscribe", Permission: "run",
			Description: "Subscribe to stdout events for a registered process",
			Request:     []SDKArg{{Name: "key", Type: "string", Required: true}},
		},
		{
			Name: "process.stdin.write", Permission: "run",
			Description: "Write a chunk to a registered process's stdin",
			Request: []SDKArg{
				{Name: "key", Type: "string", Required: true},
				{Name: "data", Type: "string", Required: true},
			},
		},
		{
			Name:        "gui.window.create",
			Description: "Create a new desktop window",
			Request: []SDKArg{
				{Name: "title", Type: "string"},
				{Name: "width", Type: "integer"},
				{Name: "height", Type: "integer"},
			},
		},
		{
			Name:        "gui.dialog.confirm",
			Description: "Show a confirmation dialog",
			Request:     []SDKArg{{Name: "message", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "confirmed", Type: "boolean"}},
		},
		{
			Name:        "gui.dialog.open",
			Permission:  "gui.dialog.open",
			Description: "Show a file open dialog",
			Request: []SDKArg{
				{Name: "title", Type: "string"},
				{Name: "filters", Type: "array"},
			},
			Response: []SDKArg{{Name: "path", Type: "string"}},
		},
		{
			Name:        "gui.dialog.save",
			Permission:  "gui.dialog.save",
			Description: "Show a file save dialog",
			Request: []SDKArg{
				{Name: "title", Type: "string"},
				{Name: "default_name", Type: "string"},
			},
			Response: []SDKArg{{Name: "path", Type: "string"}},
		},
		{
			Name:        "gui.browser.open",
			Description: "Open a URL in the user's default browser",
			Permission:  "gui.browser.open",
			Request:     []SDKArg{{Name: "url", Type: "string", Required: true}},
		},
		{
			Name:        "gui.notification.send",
			Permission:  "notifications",
			Description: "Show a system notification",
			Request: []SDKArg{
				{Name: "title", Type: "string", Required: true},
				{Name: "body", Type: "string"},
			},
		},
		{
			Name:        "gui.clipboard.read",
			Permission:  "clipboard",
			Description: "Read text from the system clipboard",
			Response:    []SDKArg{{Name: "text", Type: "string"}},
		},
		{
			Name:        "gui.clipboard.write",
			Permission:  "clipboard",
			Description: "Write text to the system clipboard",
			Request:     []SDKArg{{Name: "text", Type: "string", Required: true}},
		},
		{
			Name:        "device.location",
			Permission:  "location",
			Description: "Read the device's geolocation",
			Response: []SDKArg{
				{Name: "latitude", Type: "number"},
				{Name: "longitude", Type: "number"},
				{Name: "accuracy", Type: "number"},
			},
		},
		{
			Name:        "device.camera",
			Permission:  "camera",
			Description: "Access the device camera",
			Request:     []SDKArg{{Name: "facing", Type: "string"}},
		},
		{
			Name:        "device.microphone",
			Permission:  "microphone",
			Description: "Access the device microphone",
		},
		{
			Name:        "i18n.translate",
			Description: "Translate a string",
			Request: []SDKArg{
				{Name: "key", Type: "string", Required: true},
				{Name: "locale", Type: "string"},
			},
			Response: []SDKArg{{Name: "translated", Type: "string"}},
		},
		{
			Name: "brain.recall", Permission: "net",
			Description: "Search OpenBrain memories",
			Request:     []SDKArg{{Name: "query", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "memories", Type: "array"}},
		},
		// IPC / Event Bus (RFC §9.4) — host-managed; no permission gate
		// because the Core IPC bus is intra-process and the host already
		// decides which channels each plugin can see (RFC §11.2).
		{
			Name:        "ipc.pub.publish",
			Description: "Publish a message on a named channel",
			Request: []SDKArg{
				{Name: "channel", Type: "string", Required: true},
				{Name: "message", Type: "object"},
			},
		},
		{
			Name:        "ipc.pub.subscribe",
			Description: "Subscribe to a stream of messages on a channel",
			Request:     []SDKArg{{Name: "channel", Type: "string", Required: true}},
		},
		{
			Name:        "ipc.req.send",
			Description: "Request/response over a named channel",
			Request: []SDKArg{
				{Name: "channel", Type: "string", Required: true},
				{Name: "message", Type: "object"},
			},
			Response: []SDKArg{{Name: "response", Type: "object"}},
		},
		{
			Name:        "ipc.push.send",
			Description: "Fire-and-forget push to the host bus",
			Request:     []SDKArg{{Name: "message", Type: "object", Required: true}},
		},
		// Auth (RFC §9.4) — identity creation / login / deletion. The host
		// owns the credential store; the manifest does not gate identity.
		{
			Name:        "auth.create",
			Description: "Create a new identity (PGP key gen + QuasiSalt hash)",
			Request: []SDKArg{
				{Name: "username", Type: "string", Required: true},
				{Name: "password", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "username", Type: "string"}},
		},
		{
			Name:        "auth.login",
			Description: "Authenticate a user via ZK PGP verify; returns a JWT",
			Request: []SDKArg{
				{Name: "username", Type: "string", Required: true},
				{Name: "encrypted_payload", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "token", Type: "string"}},
		},
		{
			Name:        "auth.delete",
			Description: "Remove an identity (the user must already be logged in)",
			Request:     []SDKArg{{Name: "username", Type: "string", Required: true}},
			Response:    []SDKArg{{Name: "deleted", Type: "boolean"}},
		},
		// Crypto (RFC §9.4) — pure-compute helpers over caller-supplied
		// material. No permission gate because no IO is touched.
		{
			Name:        "crypto.pgp.generateKeyPair",
			Description: "Generate a fresh PGP keypair",
			Request: []SDKArg{
				{Name: "name", Type: "string", Required: true},
				{Name: "email", Type: "string", Required: true},
				{Name: "passphrase", Type: "string", Required: true},
			},
			Response: []SDKArg{
				{Name: "public", Type: "string"},
				{Name: "private", Type: "string"},
			},
		},
		{
			Name:        "crypto.pgp.encrypt",
			Description: "Encrypt a payload with a PGP public key",
			Request: []SDKArg{
				{Name: "data", Type: "string", Required: true},
				{Name: "public_key", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "encrypted", Type: "string"}},
		},
		{
			Name:        "crypto.pgp.decrypt",
			Description: "Decrypt a payload with a PGP private key + passphrase",
			Request: []SDKArg{
				{Name: "data", Type: "string", Required: true},
				{Name: "private_key", Type: "string", Required: true},
				{Name: "passphrase", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "plaintext", Type: "string"}},
		},
		{
			Name:        "crypto.pgp.sign",
			Description: "Sign a payload with a PGP private key + passphrase",
			Request: []SDKArg{
				{Name: "data", Type: "string", Required: true},
				{Name: "private_key", Type: "string", Required: true},
				{Name: "passphrase", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "signature", Type: "string"}},
		},
		{
			Name:        "crypto.pgp.verify",
			Description: "Verify a PGP signature against a public key",
			Request: []SDKArg{
				{Name: "data", Type: "string", Required: true},
				{Name: "signature", Type: "string", Required: true},
				{Name: "public_key", Type: "string", Required: true},
			},
			Response: []SDKArg{{Name: "valid", Type: "boolean"}},
		},
	}
}

// SelectActions returns the subset of `actions` whose Permission matches
// what the manifest declares (or every action with Permission==""). When
// `includeAll` is true the filter is skipped and every action is
// returned — useful for IDE integrations that want the full catalogue.
//
//	allowed := app.SelectActions(app.DefaultSDKActions(), &manifest, false)
//
// Rules:
//
//   - An action with empty Permission is always included (GUI dialogs,
//     i18n, etc. — RFC §9.3 marks them as "—" / no permission required).
//
//   - An action whose Permission matches the manifest's declared field is
//     included.
//
//   - Anything else is filtered out so the generated SDK only exposes
//     calls the runtime would actually accept.
func SelectActions(actions []SDKAction, m *config.ViewManifest, includeAll bool) []SDKAction {
	if includeAll || m == nil {
		out := make([]SDKAction, len(actions))
		copy(out, actions)
		return out
	}
	out := make([]SDKAction, 0, len(actions))
	for _, a := range actions {
		if a.Permission == "" || manifestDeclaresPermission(m, a.Permission) {
			out = append(out, a)
		}
	}
	return out
}

// manifestDeclaresPermission reports whether the manifest's permission
// declarations include the named field. Mirrors hasPermission but takes
// a permission name string so it can drive both the entitlement gate
// and the SDK selector.
//
//	manifestDeclaresPermission(m, "read") // true when m.Permissions.Read is non-empty
func manifestDeclaresPermission(m *config.ViewManifest, name string) bool {
	if m == nil {
		return false
	}
	switch name {
	case "read":
		return len(m.Permissions.Read) > 0 || m.Permissions.Filesystem
	case "write":
		return m.Permissions.Filesystem || len(manifestWriteList(m)) > 0
	case "net":
		return len(m.Permissions.Net) > 0 || m.Permissions.Network
	case "run":
		return len(m.Permissions.Run) > 0
	case "store":
		return hasManifestStorePermission(m)
	case "notifications":
		return m.Permissions.Notifications
	case "gui.notification.send":
		return m.Permissions.Notifications || manifestHasGUIGate(m, "gui.notification.send")
	case "clipboard":
		return m.Permissions.Clipboard
	case "gui.clipboard.read":
		return m.Permissions.Clipboard || manifestHasGUIGate(m, "gui.clipboard.read")
	case "gui.clipboard.write":
		return m.Permissions.Clipboard || manifestHasGUIGate(m, "gui.clipboard.write")
	case "gui.dialog.open":
		return manifestHasGUIGate(m, "gui.dialog.open")
	case "gui.dialog.save":
		return manifestHasGUIGate(m, "gui.dialog.save")
	case "gui.browser.open":
		return manifestHasGUIGate(m, "gui.browser.open") ||
			len(m.Permissions.Net) > 0 || m.Permissions.Network
	case "camera":
		return m.Permissions.Camera
	case "device.camera":
		return m.Permissions.Camera
	case "microphone":
		return m.Permissions.Microphone
	case "device.microphone":
		return m.Permissions.Microphone
	case "location":
		return hasManifestLocationPermission(m)
	case "device.location":
		return hasManifestLocationPermission(m)
	}
	return false
}

// SDKCatalogue returns a deterministic summary of every Core primitive
// Action — handy for `core-app sdk list` and for IDE tooling that wants
// to render the full action catalogue (action name, permission, short
// description) without parsing the DefaultSDKActions struct.
//
//	for _, row := range app.SDKCatalogue() {
//	    core.Println(row.Name, row.Permission, row.Description)
//	}
//
// The returned slice is sorted by action name so CLI table output and
// golden-file tests stay stable across runs.
func SDKCatalogue() []SDKCatalogueEntry {
	actions := DefaultSDKActions()
	out := make([]SDKCatalogueEntry, 0, len(actions))
	for _, a := range actions {
		out = append(out, SDKCatalogueEntry{
			Name:        a.Name,
			Permission:  a.Permission,
			Description: a.Description,
			Tag:         tagOf(a.Name),
		})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Name > out[j].Name; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// SDKCatalogueEntry is one row in the `core-app sdk list` output. The
// shape mirrors the columns the CLI renders (name, tag, permission,
// description) so JSON consumers and table renderers share the same
// projection.
//
//	for _, e := range app.SDKCatalogue() {
//	    fmt.Printf("%-30s %-10s %s\n", e.Name, e.Permission, e.Description)
//	}
type SDKCatalogueEntry struct {
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Permission  string `json:"permission,omitempty"`
	Description string `json:"description,omitempty"`
}

// SDKGenerate emits one or more language SDKs derived from the manifest.
// Files are written via the supplied medium so MockMedium and
// MemoryMedium tests round-trip cleanly.
//
//	err := app.SDKGenerate(coreio.Local, root, &manifest, app.SDKGenerateOptions{
//	    Languages: []app.SDKLanguage{app.SDKLanguageTypeScript},
//	})
//
// Layout (default):
//
//	<root>/.core/sdk/openapi/openapi.json
//	<root>/.core/sdk/ts/index.ts
//	<root>/.core/sdk/go/sdk.go
//	<root>/.core/sdk/php/Sdk.php
//	<root>/.core/sdk/python/sdk.py
//
// Rules:
//
//   - Empty Languages → emit all five (RFC §8 — TS, Python, Go, PHP plus
//     the OpenAPI spec that drives all of them).
//
//   - Empty OutDir → write under `<root>/.core/sdk/`.
//
//   - Empty Actions → use DefaultSDKActions filtered by manifest
//     permissions (or unfiltered if IncludeAllPrimitives=true).
func SDKGenerate(medium coreio.Medium, root string, m *config.ViewManifest, opts SDKGenerateOptions) error {
	if m == nil {
		return coreerr.E("app.SDKGenerate", "nil manifest", nil)
	}
	if root == "" {
		return coreerr.E("app.SDKGenerate", "empty root", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}

	languages := opts.Languages
	if len(languages) == 0 {
		languages = []SDKLanguage{
			SDKLanguageOpenAPI,
			SDKLanguageTypeScript,
			SDKLanguageGo,
			SDKLanguagePHP,
			SDKLanguagePython,
		}
	}

	actions := opts.Actions
	if len(actions) == 0 {
		actions = SelectActions(DefaultSDKActions(), m, opts.IncludeAllPrimitives)
	}

	outDir := opts.OutDir
	if outDir == "" {
		outDir = core.Path(root, ".core", "sdk")
	}

	for _, lang := range languages {
		body, file, err := renderSDK(lang, m, actions)
		if err != nil {
			return coreerr.E("app.SDKGenerate", "render "+lang.String()+" failed", err)
		}
		dest := core.Path(outDir, lang.String(), file)
		if err := medium.EnsureDir(core.PathDir(dest)); err != nil {
			return coreerr.E("app.SDKGenerate", "ensure dir "+core.PathDir(dest)+" failed", err)
		}
		if err := medium.Write(dest, body); err != nil {
			return coreerr.E("app.SDKGenerate", "write "+dest+" failed", err)
		}
	}
	return nil
}

// renderSDK dispatches to the per-language renderer and returns the
// emitted body plus the destination file name. Kept as a single
// dispatcher so the SDKGenerate write loop stays declarative.
//
//	body, file, err := renderSDK(app.SDKLanguageTypeScript, &manifest, actions)
func renderSDK(lang SDKLanguage, m *config.ViewManifest, actions []SDKAction) (string, string, error) {
	switch lang {
	case SDKLanguageOpenAPI:
		body, err := RenderOpenAPI(m, actions)
		return body, "openapi.json", err
	case SDKLanguageTypeScript:
		return RenderTypeScript(m, actions), "index.ts", nil
	case SDKLanguageGo:
		return RenderGo(m, actions), "sdk.go", nil
	case SDKLanguagePHP:
		return RenderPHP(m, actions), "Sdk.php", nil
	case SDKLanguagePython:
		return RenderPython(m, actions), "sdk.py", nil
	}
	return "", "", coreerr.E("app.renderSDK", "unsupported language: "+lang.String(), nil)
}

// RenderOpenAPI produces an OpenAPI 3.1 document describing the manifest's
// Action surface. Each action becomes a POST endpoint under
// `/actions/{action.name}` with the request body shaped from SDKArg.
//
//	body, err := app.RenderOpenAPI(&manifest, actions)
//	_ = os.WriteFile("openapi.json", []byte(body), 0o644)
//
// The returned JSON is pretty-printed via the same indenter
// WriteCompiled uses so a diff review of generated specs stays readable.
func RenderOpenAPI(m *config.ViewManifest, actions []SDKAction) (string, error) {
	if m == nil {
		return "", coreerr.E("app.RenderOpenAPI", "nil manifest", nil)
	}
	doc := openAPIDoc(m, actions)
	r := core.JSONMarshal(doc)
	if !r.OK {
		cause, _ := r.Value.(error)
		return "", coreerr.E("app.RenderOpenAPI", "marshal failed", cause)
	}
	raw, _ := r.Value.([]byte)
	return indentJSON(raw), nil
}

// openAPIDoc builds the OpenAPI map skeleton. Returned as a generic map
// so the caller can serialise without reaching for a heavier OpenAPI
// type library.
//
//	doc := openAPIDoc(&manifest, actions)
//	r := core.JSONMarshal(doc)
func openAPIDoc(m *config.ViewManifest, actions []SDKAction) map[string]any {
	paths := map[string]any{}
	for _, a := range actions {
		paths["/actions/"+a.Name] = map[string]any{
			"post": map[string]any{
				"summary":      a.Description,
				"operationId":  operationIDOf(a.Name),
				"tags":         []string{tagOf(a.Name)},
				"x-permission": permissionForOpenAPI(a.Permission),
				"requestBody": map[string]any{
					"required": len(a.Request) > 0,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": jsonSchemaFromArgs(a.Request),
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "OK",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": jsonSchemaFromArgs(a.Response),
							},
						},
					},
					"403": map[string]any{
						"description": "Permission denied",
					},
				},
			},
		}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       m.Name,
			"version":     m.Version,
			"description": "Auto-generated CoreApp SDK for " + m.Code,
			"x-coreapp": map[string]any{
				"code":    m.Code,
				"version": m.Version,
			},
		},
		"paths": paths,
	}
}

// jsonSchemaFromArgs produces a JSON Schema object whose properties
// mirror the SDKArg slice. Required arguments end up in the `required`
// list. Empty input yields an empty object schema (no properties), which
// the spec accepts.
//
//	schema := jsonSchemaFromArgs(args)
func jsonSchemaFromArgs(args []SDKArg) map[string]any {
	if len(args) == 0 {
		return map[string]any{"type": "object"}
	}
	props := map[string]any{}
	required := []string{}
	for _, arg := range args {
		props[arg.Name] = map[string]any{
			"type":        arg.Type,
			"description": arg.Description,
		}
		if arg.Required {
			required = append(required, arg.Name)
		}
	}
	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// permissionForOpenAPI returns the permission name as a JSON-friendly
// value or `null` (omitted) when the action does not gate. Used by
// downstream API gateways that surface the manifest entitlement to
// callers without re-parsing the manifest themselves.
//
//	v := permissionForOpenAPI("read") // "read"
//	v := permissionForOpenAPI("")     // "none"
func permissionForOpenAPI(p string) string {
	if p == "" {
		return "none"
	}
	return p
}

// operationIDOf turns a dotted action name into a camelCase operationId
// suitable for OpenAPI tooling. `fs.read` → `fsRead`; `gui.dialog.confirm`
// → `guiDialogConfirm`.
//
//	operationIDOf("fs.read") // "fsRead"
func operationIDOf(action string) string {
	if action == "" {
		return ""
	}
	out := core.NewBuilder()
	upperNext := false
	for _, r := range action {
		if r == '.' || r == '_' || r == '-' {
			upperNext = true
			continue
		}
		if upperNext {
			if r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			upperNext = false
		}
		out.WriteRune(r)
	}
	return out.String()
}

// tagOf returns the leading segment of a dotted action name so OpenAPI
// tooling can group endpoints by feature (`fs`, `store`, `net`, `gui`).
//
//	tagOf("fs.read") // "fs"
func tagOf(action string) string {
	if i := indexOf(action, "."); i > 0 {
		return action[:i]
	}
	return action
}

// RenderTypeScript emits an ES module exporting one typed wrapper
// function per action. The generated module imports the global `Core`
// runtime (provided by the host's CoreTS layer) so the developer just
// imports the SDK and calls into the typed surface.
//
//	const ts = app.RenderTypeScript(&manifest, actions)
//	_ = os.WriteFile("index.ts", []byte(ts), 0o644)
//
// The output deliberately avoids bringing in JSDoc generation for the
// permission notes — the description appears as a TS comment block above
// each function so editor tooltips surface it.
func RenderTypeScript(m *config.ViewManifest, actions []SDKAction) string {
	b := core.NewBuilder()
	b.WriteString("// SPDX-License-Identifier: EUPL-1.2\n")
	b.WriteString("// Auto-generated by core-app sdk generate. Do not edit by hand.\n")
	b.WriteString("// CoreApp: " + m.Code + " v" + string(m.Version) + "\n\n")
	b.WriteString("// Core is the global runtime injected by CoreGUI / CoreTS.\n")
	b.WriteString("// Each generated function calls Core.Action(name, opts) and\n")
	b.WriteString("// returns the parsed response.\n")
	b.WriteString("declare const Core: {\n")
	b.WriteString("  Action(name: string, opts?: Record<string, unknown>): Promise<any>;\n")
	b.WriteString("};\n\n")

	for _, a := range actions {
		b.WriteString("/**\n")
		if a.Description != "" {
			b.WriteString(" * " + a.Description + "\n")
		}
		if a.Permission != "" {
			b.WriteString(" * @permission " + a.Permission + "\n")
		}
		b.WriteString(" */\n")
		b.WriteString("export interface " + tsTypeName(a.Name, "Request") + " {\n")
		for _, arg := range a.Request {
			b.WriteString("  " + arg.Name)
			if !arg.Required {
				b.WriteString("?")
			}
			b.WriteString(": " + tsTypeOf(arg.Type) + ";\n")
		}
		b.WriteString("}\n\n")

		if len(a.Response) > 0 {
			b.WriteString("export interface " + tsTypeName(a.Name, "Response") + " {\n")
			for _, arg := range a.Response {
				b.WriteString("  " + arg.Name + ": " + tsTypeOf(arg.Type) + ";\n")
			}
			b.WriteString("}\n\n")
		}

		fnName := tsFnName(a.Name)
		b.WriteString("export async function " + fnName + "(opts: " + tsTypeName(a.Name, "Request") + ")")
		if len(a.Response) > 0 {
			b.WriteString(": Promise<" + tsTypeName(a.Name, "Response") + "> {\n")
			b.WriteString("  return Core.Action(\"" + a.Name + "\", opts) as Promise<" + tsTypeName(a.Name, "Response") + ">;\n")
		} else {
			b.WriteString(": Promise<void> {\n")
			b.WriteString("  await Core.Action(\"" + a.Name + "\", opts);\n")
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// tsFnName turns an action name into a camelCase TypeScript function
// identifier — the same rule as operationIDOf.
//
//	tsFnName("fs.read") // "fsRead"
func tsFnName(action string) string { return operationIDOf(action) }

// tsTypeName builds an interface name like `FsReadRequest` /
// `FsReadResponse` from a dotted action name and a suffix.
//
//	tsTypeName("fs.read", "Request") // "FsReadRequest"
func tsTypeName(action, suffix string) string {
	pascal := operationIDOf(action)
	if pascal == "" {
		return suffix
	}
	if pascal[0] >= 'a' && pascal[0] <= 'z' {
		pascal = string(pascal[0]-('a'-'A')) + pascal[1:]
	}
	return pascal + suffix
}

// tsTypeOf maps the JSON Schema-ish type name into TypeScript.
//
//	tsTypeOf("integer") // "number"
//	tsTypeOf("array")   // "unknown[]"
func tsTypeOf(t string) string {
	switch core.Lower(t) {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "array":
		return "unknown[]"
	case "object":
		return "Record<string, unknown>"
	default:
		return "unknown"
	}
}

// RenderGo emits a Go SDK package with one function per Action. The
// generated code uses a small CoreClient interface so consumers can
// inject the in-process Core (for tests) or a remote dispatcher (for
// CLI tools talking over IPC).
//
//	const goSrc = app.RenderGo(&manifest, actions)
//	_ = os.WriteFile("sdk.go", []byte(goSrc), 0o644)
func RenderGo(m *config.ViewManifest, actions []SDKAction) string {
	b := core.NewBuilder()
	b.WriteString("// SPDX-License-Identifier: EUPL-1.2\n\n")
	b.WriteString("// Code generated by core-app sdk generate. DO NOT EDIT.\n")
	b.WriteString("// CoreApp: " + m.Code + " v" + string(m.Version) + "\n\n")
	b.WriteString("package sdk\n\n")
	b.WriteString("import \"context\"\n\n")
	b.WriteString("// CoreClient is the small surface SDK wrappers depend on.\n")
	b.WriteString("// Implementations: in-process Core (test), Wails IPC (desktop),\n")
	b.WriteString("// HTTP gateway (remote).\n")
	b.WriteString("type CoreClient interface {\n")
	b.WriteString("\tAction(ctx context.Context, name string, opts map[string]any) (map[string]any, error)\n")
	b.WriteString("}\n\n")

	b.WriteString("// SDK wraps a CoreClient with typed action helpers.\n")
	b.WriteString("type SDK struct{ client CoreClient }\n\n")
	b.WriteString("// New constructs an SDK around the supplied CoreClient.\n")
	b.WriteString("func New(c CoreClient) *SDK { return &SDK{client: c} }\n\n")

	for _, a := range actions {
		fn := goFnName(a.Name)
		b.WriteString("// " + fn + " calls the \"" + a.Name + "\" Named Action.\n")
		if a.Description != "" {
			b.WriteString("// " + a.Description + "\n")
		}
		if a.Permission != "" {
			b.WriteString("// Permission: " + a.Permission + "\n")
		}
		b.WriteString("func (s *SDK) " + fn + "(ctx context.Context, opts map[string]any) (map[string]any, error) {\n")
		b.WriteString("\treturn s.client.Action(ctx, \"" + a.Name + "\", opts)\n")
		b.WriteString("}\n\n")
	}
	return b.String()
}

// goFnName converts a dotted action name into PascalCase suitable for a
// Go method.
//
//	goFnName("fs.read") // "FsRead"
func goFnName(action string) string {
	camel := operationIDOf(action)
	if camel == "" {
		return ""
	}
	if camel[0] >= 'a' && camel[0] <= 'z' {
		return string(camel[0]-('a'-'A')) + camel[1:]
	}
	return camel
}

// RenderPHP emits a PHP class with one method per Action. Designed for
// CorePHP consumers wiring up a Laravel facade — the constructor takes
// any object with an `action(string $name, array $opts): array` method
// (CorePHP's `Core` class fits the bill).
//
//	const php = app.RenderPHP(&manifest, actions)
//	_ = os.WriteFile("Sdk.php", []byte(php), 0o644)
func RenderPHP(m *config.ViewManifest, actions []SDKAction) string {
	b := core.NewBuilder()
	b.WriteString("<?php\n")
	b.WriteString("// SPDX-License-Identifier: EUPL-1.2\n")
	b.WriteString("// Auto-generated by core-app sdk generate. Do not edit by hand.\n")
	b.WriteString("// CoreApp: " + m.Code + " v" + string(m.Version) + "\n\n")
	b.WriteString("declare(strict_types=1);\n\n")
	b.WriteString("namespace Core\\Sdk;\n\n")
	b.WriteString("/**\n * Generated CoreApp SDK for " + m.Code + ".\n */\n")
	b.WriteString("class Sdk\n{\n")
	b.WriteString("    public function __construct(private object $core) {}\n\n")

	for _, a := range actions {
		fn := phpFnName(a.Name)
		b.WriteString("    /**\n")
		if a.Description != "" {
			b.WriteString("     * " + a.Description + "\n")
		}
		if a.Permission != "" {
			b.WriteString("     * @permission " + a.Permission + "\n")
		}
		b.WriteString("     */\n")
		b.WriteString("    public function " + fn + "(array $opts = []): array\n")
		b.WriteString("    {\n")
		b.WriteString("        return $this->core->action('" + a.Name + "', $opts);\n")
		b.WriteString("    }\n\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// phpFnName mirrors tsFnName for PHP — camelCase per PSR-12.
//
//	phpFnName("fs.read") // "fsRead"
func phpFnName(action string) string { return operationIDOf(action) }

// RenderPython emits a Python module exposing one function per Action.
// The generated module depends on a small `CoreClient` protocol so
// consumers can inject the in-process Core (for tests) or a remote
// dispatcher (CLI tools talking over IPC / HTTP). Completes the RFC §8
// pipeline ("Client SDKs (TS, Python, Go, PHP)").
//
//	const py = app.RenderPython(&manifest, actions)
//	_ = os.WriteFile("sdk.py", []byte(py), 0o644)
//
// The output targets Python 3.8+ — typed `Dict[str, Any]` signatures, no
// 3.10 `X | Y` union shorthand, and `typing.Protocol` for the client
// contract so callers can duck-type their own implementation.
func RenderPython(m *config.ViewManifest, actions []SDKAction) string {
	b := core.NewBuilder()
	b.WriteString("# SPDX-License-Identifier: EUPL-1.2\n")
	b.WriteString("# Auto-generated by core-app sdk generate. Do not edit by hand.\n")
	b.WriteString("# CoreApp: " + m.Code + " v" + string(m.Version) + "\n\n")
	b.WriteString("from __future__ import annotations\n\n")
	b.WriteString("from typing import Any, Dict, Optional, Protocol\n\n\n")

	b.WriteString("class CoreClient(Protocol):\n")
	b.WriteString("    \"\"\"Minimal surface every SDK wrapper depends on.\n\n")
	b.WriteString("    Implementations include the in-process Core (tests), a Wails IPC\n")
	b.WriteString("    bridge (desktop), and a plain HTTP gateway client (remote).\n")
	b.WriteString("    \"\"\"\n\n")
	b.WriteString("    def action(self, name: str, opts: Dict[str, Any]) -> Dict[str, Any]:\n")
	b.WriteString("        ...\n\n\n")

	b.WriteString("class SDK:\n")
	b.WriteString("    \"\"\"Generated CoreApp SDK for " + m.Code + ".\"\"\"\n\n")
	b.WriteString("    def __init__(self, client: CoreClient) -> None:\n")
	b.WriteString("        self._client = client\n")

	for _, a := range actions {
		fn := pythonFnName(a.Name)
		b.WriteString("\n    def " + fn + "(self, opts: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:\n")
		b.WriteString("        \"\"\"")
		if a.Description != "" {
			b.WriteString(a.Description)
		} else {
			b.WriteString("Call the \"" + a.Name + "\" Named Action.")
		}
		if a.Permission != "" {
			b.WriteString("\n\n        Permission: " + a.Permission)
		}
		b.WriteString("\n        \"\"\"\n")
		b.WriteString("        return self._client.action(\"" + a.Name + "\", opts or {})\n")
	}
	return b.String()
}

// pythonFnName converts a dotted action name into a PEP 8 snake_case
// identifier suitable for a Python method. Mirrors operationIDOf's
// behaviour but swaps camelCase for snake_case per Python convention.
//
//	pythonFnName("fs.read")            // "fs_read"
//	pythonFnName("gui.dialog.confirm") // "gui_dialog_confirm"
func pythonFnName(action string) string {
	if action == "" {
		return ""
	}
	out := core.NewBuilder()
	for _, r := range action {
		switch {
		case r == '.' || r == '-' || r == ' ':
			out.WriteByte('_')
		case r >= 'A' && r <= 'Z':
			// Preserve any casing the caller supplied but normalise to
			// lower so the identifier looks idiomatic in Python.
			out.WriteRune(r + ('a' - 'A'))
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
