// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// Named Action prefixes gated by manifest permissions. Keeping the
// mapping as a table means new Actions can join the gate by appending a
// row — no code changes elsewhere.
//
//	"fs.read"            → requires permissions.read  (path list)
//	"fs.write"           → requires permissions.write (path list)
//	"net.*"              → requires permissions.net   (host:port list)
//	"process.*"          → requires permissions.run   (binary list)
//	"store.*"            → requires permissions.store (boolean)
//	"gui.notification.*" → requires permissions.notifications (boolean)
//	"gui.clipboard.read" → requires permissions.clipboard (boolean)
//	"gui.clipboard.write"→ requires permissions.clipboard (boolean)
//	"gui.dialog.open"    → requires permissions.gui.dialog.open (bool gate)
//	"gui.dialog.save"    → requires permissions.gui.dialog.save (bool gate)
//	"gui.browser.open"   → requires permissions.gui.browser.open (bool gate)
//	"device.camera"      → requires permissions.camera (boolean)
//	"device.microphone"  → requires permissions.microphone (boolean)
//	"device.location"    → requires permissions.location (Run-list legacy entry)
//	"ipc.*"              → no permission gate (intra-process bus is always allowed
//	                       but plugins can only see channels their host wires up)
//	"auth.*"             → no permission gate (identity is host-managed; the
//	                       handler decides whether to accept the credential payload)
//	"crypto.*"           → no permission gate (pure compute over caller-supplied
//	                       material; no IO touched)
var actionPermissionMap = []actionGate{
	{prefix: "fs.read", field: fieldRead},
	{prefix: "fs.list", field: fieldRead},
	{prefix: "fs.write", field: fieldWrite},
	{prefix: "fs.delete", field: fieldWrite},
	{prefix: "net.fetch", field: fieldNet},
	{prefix: "net.ws", field: fieldNet},
	{prefix: "process.run", field: fieldRun},
	{prefix: "process.add", field: fieldRun},
	{prefix: "process.start", field: fieldRun},
	{prefix: "process.stop", field: fieldRun},
	{prefix: "process.kill", field: fieldRun},
	{prefix: "process.get", field: fieldRun},
	{prefix: "process.list", field: fieldRun},
	{prefix: "process.stdout.subscribe", field: fieldRun},
	{prefix: "process.stdin.write", field: fieldRun},
	{prefix: "store.get", field: fieldStore},
	{prefix: "store.set", field: fieldStore},
	{prefix: "store.delete", field: fieldStore},
	{prefix: "gui.notification.send", field: fieldNotification},
	{prefix: "gui.clipboard.read", field: fieldClipboardRead},
	{prefix: "gui.clipboard.write", field: fieldClipboardWrite},
	{prefix: "gui.dialog.open", field: fieldDialogOpen},
	{prefix: "gui.dialog.save", field: fieldDialogSave},
	{prefix: "gui.browser.open", field: fieldBrowserOpen},
	{prefix: "device.camera", field: fieldCamera},
	{prefix: "device.microphone", field: fieldMicrophone},
	{prefix: "device.location", field: fieldLocation},
	// `brain.recall` dispatches a query to OpenBrain — an upstream
	// network service — so RFC §9.3 lists it under the `net` permission.
	// Listing the prefix here keeps the action behind the same gate as
	// `net.fetch` / `net.ws` / `gui.browser.open`; a caller without the
	// `net` declaration gets the same "capability not declared" denial.
	{prefix: "brain.recall", field: fieldNet},
}

// permissionField names a slot in ViewPermissions.
type permissionField int

const (
	fieldRead permissionField = iota
	fieldWrite
	fieldNet
	fieldRun
	fieldStore
	fieldNotification
	fieldClipboardRead
	fieldClipboardWrite
	fieldDialogOpen
	fieldDialogSave
	fieldBrowserOpen
	fieldCamera
	fieldMicrophone
	fieldLocation
)

// String returns the manifest key name — used in denial messages so a
// developer can match the error to their view.yaml edit.
//
//	fieldRead.String() // "read"
func (f permissionField) String() string {
	switch f {
	case fieldRead:
		return "read"
	case fieldWrite:
		return "write"
	case fieldNet:
		return "net"
	case fieldRun:
		return "run"
	case fieldStore:
		return "store"
	case fieldNotification:
		return "notifications"
	case fieldClipboardRead:
		return "clipboard"
	case fieldClipboardWrite:
		return "clipboard"
	case fieldDialogOpen:
		return "gui.dialog.open"
	case fieldDialogSave:
		return "gui.dialog.save"
	case fieldBrowserOpen:
		return "gui.browser.open"
	case fieldCamera:
		return "camera"
	case fieldMicrophone:
		return "microphone"
	case fieldLocation:
		return "location"
	default:
		return "unknown"
	}
}

type actionGate struct {
	prefix string
	field  permissionField
}

// permissions is Step 3 of the 7-step boot — install an entitlement
// checker on Core that gates Named Actions by the manifest's
// `permissions:` declarations. Undeclared capabilities = denied.
//
//	err := permissions(c, &manifest, app.ModeProd)
//
// Two layers cooperate to enforce the RFC §2.2 / §10.1 rules:
//
//   - This function installs the coarse gate — a Named Action whose
//     family is not declared at all (the app never asked for `read`)
//     is rejected before the handler runs.
//
//   - `CheckActionAccess` (access.go) performs the per-argument match
//     — fs.read's `path` is checked against `permissions.read[]`,
//     net.fetch's `host:port` is checked against `permissions.net[]`,
//     etc. Handlers in go-io / core-net / go-process call it once
//     with the caller-supplied argument before performing any
//     sensitive IO.
//
// Both layers are required: the coarse gate keeps the entitlement bus
// honest (so an ungated action costs nothing at runtime), and the
// per-arg layer keeps the sandbox honest (a manifest granting
// `./photos/` must not admit `./photos/../etc/passwd`).
func permissions(
	c *core.Core, m *config.ViewManifest, mode Mode,
) error {
	if c == nil {
		return core.E("app.permissions", "nil core", nil)
	}
	if m == nil {
		return core.E("app.permissions", "nil manifest", nil)
	}

	checker := newCheckerForManifest(m, mode)
	c.SetEntitlementChecker(checker)
	return nil
}

// newChecker builds an EntitlementChecker closed over the manifest's
// permission declarations. The closure is small and stateless — safe to
// call from many goroutines under core.Action.Run().
//
// In prod mode, a denied action returns Allowed=false with the reason set
// to the missing permission field. In dev mode the check still runs but
// always returns Allowed=true; the reason explains why the call would
// have been denied in prod, which is what the developer wants to see in
// logs while they iterate.
func newChecker(p config.ViewPermissions, mode Mode) core.EntitlementChecker {
	return newCheckerForManifest(&config.ViewManifest{Permissions: p}, mode)
}

// newCheckerForManifest is the manifest-aware constructor that
// permissions() uses to honour the store-bool flag stored in
// Config["store"] (see hasManifestStorePermission).
//
//	c.SetEntitlementChecker(newCheckerForManifest(&manifest, ModeProd))
//
// Dev-mode logging — the RFC §4.2 promise ("Permission violations
// logged as warnings, not errors") is enforced here. Each undeclared
// action surfaces a single `core.Warn` the first time it is attempted
// so a developer iterating on view.yaml sees exactly which permission
// they forgot without drowning in repeated messages on a hot loop. The
// dedup map is keyed by the manifest code + action so two plugins
// running in the same host don't cross-talk each other's warnings.
func newCheckerForManifest(m *config.ViewManifest, mode Mode) core.EntitlementChecker {
	if m == nil {
		m = &config.ViewManifest{}
	}
	p := m.Permissions
	storeDeclared := hasManifestStorePermission(m)
	writeDeclared := len(manifestWriteList(m)) > 0
	dialogOpenDeclared := manifestHasGUIGate(m, "gui.dialog.open")
	dialogSaveDeclared := manifestHasGUIGate(m, "gui.dialog.save")
	browserOpenDeclared := manifestHasGUIGate(m, "gui.browser.open")
	locationDeclared := hasManifestLocationPermission(m)
	code := m.Code
	// Dev-mode dedup — a single Warn per (code, action) so a 500ms
	// hot-reload loop polling the same handler does not produce one
	// log line per tick.
	warned := map[string]bool{}
	var warnMu sync.Mutex
	return func(action string, _ int, _ context.Context) core.Entitlement {
		gate, ok := gateFor(action)
		if !ok {
			// Unknown / ungated action — always allowed. GUI actions
			// (gui.window.create, gui.dialog.confirm) fall through here
			// since the RFC says they have no permission requirement.
			return core.Entitlement{Allowed: true, Unlimited: true}
		}

		declared := hasPermission(p, gate.field)
		// Store has its own wider check — Config["store"] satisfies it
		// even when ViewPermissions doesn't expose a typed field.
		if !declared && gate.field == fieldStore {
			declared = storeDeclared
		}
		// Write has a parallel wider check — Config["write"] (per-path
		// list) satisfies it even when ViewPermissions stays on the
		// catch-all Filesystem flag.
		if !declared && gate.field == fieldWrite {
			declared = writeDeclared
		}
		if !declared && gate.field == fieldDialogOpen {
			declared = dialogOpenDeclared
		}
		if !declared && gate.field == fieldDialogSave {
			declared = dialogSaveDeclared
		}
		if !declared && gate.field == fieldBrowserOpen {
			// Backwards-compat: older manifests that treated browser-open
			// as part of the broader `net` capability still pass.
			declared = browserOpenDeclared || hasPermission(p, fieldNet)
		}
		if !declared && gate.field == fieldLocation {
			declared = locationDeclared
		}
		if declared {
			return core.Entitlement{Allowed: true, Unlimited: true}
		}

		reason := "manifest does not declare " + gate.field.String() + " permission for " + action
		if mode == ModeDev {
			// RFC §4.2 — warning, not error. The call still goes through
			// so hot-reload iteration keeps flowing, but a single log
			// line per (code, action) flags the gap so the developer
			// knows to add the permission before shipping.
			warnMu.Lock()
			key := code + "/" + action
			seen := warned[key]
			if !seen {
				warned[key] = true
			}
			warnMu.Unlock()
			if !seen {
				core.Warn("permission gate bypassed in dev mode",
					"code", code,
					"action", action,
					"missing", gate.field.String(),
					"reason", reason)
			}
			return core.Entitlement{Allowed: true, Unlimited: true, Reason: reason}
		}
		return core.Entitlement{Allowed: false, Reason: reason}
	}
}

// gateFor looks up the permission field required for a named action.
//
//	gate, ok := gateFor("fs.read")
//	gate.field.String() // "read"
func gateFor(action string) (actionGate, bool) {
	for _, gate := range actionPermissionMap {
		if action == gate.prefix || core.HasPrefix(action, gate.prefix+".") {
			return gate, true
		}
	}
	return actionGate{}, false
}

// hasPermission returns true if the manifest declares any value in the
// named permission slot. An empty list == not declared.
//
//	declared := hasPermission(perms, fieldRead)
func hasPermission(p config.ViewPermissions, field permissionField) bool {
	switch field {
	case fieldRead:
		return len(p.Read) > 0 || p.Filesystem
	case fieldWrite:
		// ViewPermissions does not yet expose a write-path list; the
		// Filesystem bool covers read+write together for the typed
		// schema. The wider check via newCheckerForManifest also honours
		// Config["write"] so manifests can declare per-path writes
		// without flipping Filesystem.
		return p.Filesystem
	case fieldNet:
		return len(p.Net) > 0 || p.Network
	case fieldRun:
		return len(p.Run) > 0
	case fieldStore:
		// ViewPermissions does not yet expose a store boolean. The
		// manifest-aware closure checks Config["store"] separately, so the
		// typed slot alone can never satisfy store access.
		return false
	case fieldNotification:
		return p.Notifications
	case fieldClipboardRead, fieldClipboardWrite:
		return p.Clipboard
	case fieldCamera:
		return p.Camera
	case fieldMicrophone:
		return p.Microphone
	case fieldDialogOpen, fieldDialogSave, fieldBrowserOpen:
		// GUI gate booleans currently live under Config["gui_gates"], so
		// the manifest-aware closure handles them separately.
		return false
	case fieldLocation:
		// ViewPermissions has no location bool yet — the wrap pipeline
		// stashes the capability in Run as `device.location` so the
		// gate can detect the explicit declaration. Mirrors the
		// pattern hasManifestStorePermission uses for the legacy store
		// flag.
		for _, entry := range p.Run {
			if entry == "device.location" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// hasManifestStorePermission inspects the wider ViewManifest for the
// explicit `permissions.store: true` flag stored under Config["store"].
// Kept separate from hasPermission so the entitlement closure can call
// it without dragging the manifest into the inner switch.
//
//	declared := hasManifestStorePermission(&manifest)
func hasManifestStorePermission(m *config.ViewManifest) bool {
	if m == nil {
		return false
	}
	if m.Config == nil {
		return false
	}
	if v, ok := m.Config["store"].(bool); ok {
		return v
	}
	return false
}
