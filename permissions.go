// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// Named Action prefixes gated by manifest permissions. Keeping the
// mapping as a table means new Actions can join the gate by appending a
// row — no code changes elsewhere.
//
//	"fs.read"  → requires permissions.read  (path list)
//	"fs.write" → requires permissions.write (path list)
//	"net.*"    → requires permissions.net   (host:port list)
//	"process.*"→ requires permissions.run   (binary list)
//	"store.*"  → requires permissions.store (boolean)
var actionPermissionMap = []actionGate{
	{prefix: "fs.read", field: fieldRead},
	{prefix: "fs.list", field: fieldRead},
	{prefix: "fs.write", field: fieldWrite},
	{prefix: "fs.delete", field: fieldWrite},
	{prefix: "net.fetch", field: fieldNet},
	{prefix: "net.ws", field: fieldNet},
	{prefix: "process.run", field: fieldRun},
	{prefix: "process.start", field: fieldRun},
	{prefix: "process.stop", field: fieldRun},
	{prefix: "store.get", field: fieldStore},
	{prefix: "store.set", field: fieldStore},
	{prefix: "store.delete", field: fieldStore},
}

// permissionField names a slot in ViewPermissions.
type permissionField int

const (
	fieldRead permissionField = iota
	fieldWrite
	fieldNet
	fieldRun
	fieldStore
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
// TODO(app): each declared path / host / binary is currently treated as
// a presence check — "has the manifest asked for any read access?" — not
// a per-argument match. Proper per-call matching lives inside the
// individual Action handlers (fs.read checks its `path` argument against
// permissions.read[]; net.fetch checks its `host:port` against
// permissions.net[]). That enforcement layer is owned by the handlers in
// go-io / core-net / go-process; this file wires the gate, not the rule.
func permissions(c *core.Core, m *config.ViewManifest, mode Mode) error {
	if c == nil {
		return coreerr.E("app.permissions", "nil core", nil)
	}
	if m == nil {
		return coreerr.E("app.permissions", "nil manifest", nil)
	}

	checker := newChecker(m.Permissions, mode)
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
	return func(action string, _ int, _ context.Context) core.Entitlement {
		gate, ok := gateFor(action)
		if !ok {
			// Unknown / ungated action — always allowed. GUI actions
			// (gui.window.create, gui.dialog.confirm) fall through here
			// since the RFC says they have no permission requirement.
			return core.Entitlement{Allowed: true, Unlimited: true}
		}

		declared := hasPermission(p, gate.field)
		if declared {
			return core.Entitlement{Allowed: true, Unlimited: true}
		}

		reason := "manifest does not declare " + gate.field.String() + " permission for " + action
		if mode == ModeDev {
			// Dev mode — log the denial reason but let the call through
			// so hot-reload iteration keeps flowing.
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
		// Filesystem bool covers read+write together. See TODO below.
		return p.Filesystem
	case fieldNet:
		return len(p.Net) > 0 || p.Network
	case fieldRun:
		return len(p.Run) > 0
	case fieldStore:
		// ViewPermissions does not yet expose a store boolean; store
		// access is declared in view.yaml via `permissions.store: true`
		// and the CoreApp RFC §2.2 documents it — config.ViewPermissions
		// needs to grow the field. TODO(config).
		return false
	default:
		return false
	}
}
