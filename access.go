// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// AccessMode names the capability an action requires. The capital-letter
// enum mirrors the RFC §2.2 permission field names (`read`, `write`,
// `net`, `run`, `store`) but typed so callers never misspell one.
//
//	if err := app.CheckAccess(manifest, app.AccessRead, "./photos/a.jpg"); err != nil { ... }
type AccessMode int

const (
	// AccessRead — filesystem read, per `permissions.read: [...]`.
	AccessRead AccessMode = iota
	// AccessWrite — filesystem write, per `permissions.write: [...]`
	// (currently satisfied by Filesystem=true until core/config grows
	// the explicit list).
	AccessWrite
	// AccessNet — outbound network, per `permissions.net: [...]`.
	AccessNet
	// AccessRun — process execution, per `permissions.run: [...]`.
	AccessRun
)

// String returns the lowercase permission field name.
//
//	AccessRead.String() // "read"
func (a AccessMode) String() string {
	switch a {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	case AccessNet:
		return "net"
	case AccessRun:
		return "run"
	default:
		return "unknown"
	}
}

// CheckAccess validates that `arg` (a path, host:port or binary name)
// matches the manifest's declared permissions for the given mode. Named
// Action handlers call this with the caller-supplied argument before
// performing any sensitive operation.
//
// Rules:
//
//   - AccessRead: `arg` must be prefixed by any entry in
//     `permissions.read[]`. A `permissions.filesystem: true` legacy bool
//     is treated as "everything allowed" for backwards compatibility.
//
//   - AccessWrite: today only honours `permissions.filesystem: true`
//     (ViewPermissions has no write list yet — see package comment).
//     Once the `write:` list lands in core/config, this function will
//     grow the per-path check without changing its signature.
//
//   - AccessNet: `arg` must match any entry in `permissions.net[]`
//     exactly. A `permissions.network: true` legacy bool means
//     everything allowed.
//
//   - AccessRun: `arg` must match any entry in `permissions.run[]`
//     exactly.
//
// Returns nil when access is granted.
//
//	if err := app.CheckAccess(manifest, app.AccessRead, "./photos/a.jpg"); err != nil {
//	    return core.Result{Value: err, OK: false}
//	}
func CheckAccess(m *config.ViewManifest, mode AccessMode, arg string) error {
	if m == nil {
		return coreerr.E("app.CheckAccess", "nil manifest", nil)
	}

	switch mode {
	case AccessRead:
		if m.Permissions.Filesystem {
			return nil
		}
		if matchPrefix(m.Permissions.Read, arg) {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"read access to '"+arg+"' not declared in manifest.permissions.read",
			nil,
		)
	case AccessWrite:
		if m.Permissions.Filesystem {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"write access to '"+arg+"' not declared in manifest.permissions",
			nil,
		)
	case AccessNet:
		if m.Permissions.Network {
			return nil
		}
		if matchExact(m.Permissions.Net, arg) {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"net access to '"+arg+"' not declared in manifest.permissions.net",
			nil,
		)
	case AccessRun:
		if matchExact(m.Permissions.Run, arg) {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"run access to '"+arg+"' not declared in manifest.permissions.run",
			nil,
		)
	default:
		return coreerr.E("app.CheckAccess", "unknown access mode", nil)
	}
}

// matchPrefix returns true when `arg` starts with any entry in list.
// Used for path-based permissions where a declared "./photos/" grants
// access to "./photos/sunset.jpg", "./photos/thumbs/a.webp" etc.
//
//	matchPrefix([]string{"./photos/"}, "./photos/sunset.jpg") // true
func matchPrefix(list []string, arg string) bool {
	for _, entry := range list {
		if entry == "" {
			continue
		}
		if core.HasPrefix(arg, entry) {
			return true
		}
	}
	return false
}

// matchExact returns true when `arg` equals any entry in list. Used for
// host:port, binary and other non-path permissions where substring
// matching would be dangerous ("api.example.com" must not leak access
// to "evil-api.example.com").
//
//	matchExact([]string{"api.example.com:443"}, "api.example.com:443") // true
func matchExact(list []string, arg string) bool {
	for _, entry := range list {
		if entry == arg {
			return true
		}
	}
	return false
}
