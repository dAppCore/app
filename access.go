// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// AccessMode names the capability an action requires. The capital-letter
// enum mirrors the RFC §2.2 / §16.1 permission field names (`read`,
// `write`, `net`, `run`, `store`, `notifications`, `clipboard`, `camera`,
// `microphone`, `location`) but typed so callers never misspell one.
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
	// AccessStore — object store, per `permissions.store: true`
	// recorded in Config["store"]. The argument value is the store
	// group / key tuple; today the gate only checks for declaration.
	AccessStore
	// AccessNotification — desktop / system notifications, per
	// `permissions.notifications: true`. RFC §16.1 maps the PWA
	// `notifications` permission onto this gate.
	AccessNotification
	// AccessClipboardRead — clipboard read, per `permissions.clipboard:
	// true`. Read and write share a single ViewPermissions field today
	// because the Web platform groups them; once a per-direction split
	// lands in core/config, AccessClipboardRead and AccessClipboardWrite
	// can diverge without changing this enum.
	AccessClipboardRead
	// AccessClipboardWrite — clipboard write, mirror of
	// AccessClipboardRead.
	AccessClipboardWrite
	// AccessCamera — camera access, per `permissions.camera: true`.
	AccessCamera
	// AccessMicrophone — microphone access, per `permissions.microphone:
	// true`.
	AccessMicrophone
	// AccessLocation — geolocation, per the legacy `device.location`
	// entry stored in `permissions.run` (see hasPermission for the
	// rationale).
	AccessLocation
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
	case AccessStore:
		return "store"
	case AccessNotification:
		return "notifications"
	case AccessClipboardRead, AccessClipboardWrite:
		return "clipboard"
	case AccessCamera:
		return "camera"
	case AccessMicrophone:
		return "microphone"
	case AccessLocation:
		return "location"
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
//   - AccessRead / AccessWrite also reject path traversal (RFC §5.2 /
//     §10.2) — any `arg` containing a `..` segment is denied regardless
//     of declared prefixes. Prefix matching alone isn't safe: a manifest
//     granting `./data/` would otherwise admit `./data/../etc/passwd`.
//     The legacy Filesystem bool granting "everything" still applies —
//     but even there, an explicit traversal attempt is rejected so the
//     defence holds uniformly.
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
		if err := rejectPathTraversal("read", arg); err != nil {
			return err
		}
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
		if err := rejectPathTraversal("write", arg); err != nil {
			return err
		}
		if m.Permissions.Filesystem {
			return nil
		}
		// Per-path write list lives in Config["write"] until the upstream
		// ViewPermissions schema grows a typed slot. Matches RFC §2.2's
		// `write: ["./photos/.thumbnails/"]` example so the manifest
		// stays expressive without forcing the catch-all Filesystem flag.
		if matchPrefix(manifestWriteList(m), arg) {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"write access to '"+arg+"' not declared in manifest.permissions.write",
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
	case AccessStore:
		if hasManifestStorePermission(m) {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"store access to '"+arg+"' not declared (set permissions.store: true in view.yaml)",
			nil,
		)
	case AccessNotification:
		if m.Permissions.Notifications {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"notification access not declared (set permissions.notifications: true in view.yaml)",
			nil,
		)
	case AccessClipboardRead:
		if m.Permissions.Clipboard {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"clipboard read not declared (set permissions.clipboard: true in view.yaml)",
			nil,
		)
	case AccessClipboardWrite:
		if m.Permissions.Clipboard {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"clipboard write not declared (set permissions.clipboard: true in view.yaml)",
			nil,
		)
	case AccessCamera:
		if m.Permissions.Camera {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"camera access not declared (set permissions.camera: true in view.yaml)",
			nil,
		)
	case AccessMicrophone:
		if m.Permissions.Microphone {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"microphone access not declared (set permissions.microphone: true in view.yaml)",
			nil,
		)
	case AccessLocation:
		// Location lives in the Run-list as `device.location` until
		// ViewPermissions grows a typed slot. matchExact mirrors the
		// AccessRun gate so the convention stays consistent.
		if matchExact(m.Permissions.Run, "device.location") {
			return nil
		}
		return coreerr.E(
			"app.CheckAccess",
			"location access not declared (add 'device.location' to permissions.run in view.yaml)",
			nil,
		)
	default:
		return coreerr.E("app.CheckAccess", "unknown access mode", nil)
	}
}

// manifestWriteList extracts the per-path write list stored under
// Config["write"]. ViewPermissions does not yet expose a typed Write
// slot — until it does, the wrap and conclave pipelines stash the list
// here so CheckAccess and the entitlement gate can both honour it.
//
//	for _, prefix := range manifestWriteList(m) { ... }
//
// Accepts either []string or []any (YAML decodes mixed scalar arrays
// as []any in the Config map). Returns nil for missing or malformed
// entries — the caller falls back to the catch-all Filesystem flag.
func manifestWriteList(m *config.ViewManifest) []string {
	if m == nil || m.Config == nil {
		return nil
	}
	raw, ok := m.Config["write"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// rejectPathTraversal blocks any path argument that contains a parent
// traversal segment (`..`). Matches RFC §5.2 / §10.2 — "Path traversal
// blocked" is a hard rule of the permission gate regardless of how the
// manifest's declared prefixes happen to look. A prefix match like
// `./data/` would otherwise admit `./data/../etc/passwd` because
// HasPrefix is purely textual; this helper short-circuits that before
// any match check runs.
//
//	_ = rejectPathTraversal("read", "./data/../etc/passwd") // error
//	_ = rejectPathTraversal("read", "./data/a.jpg")          // nil
//
// Detected forms:
//
//   - `..` as the whole argument.
//
//   - `../` or `..\\` at the start of the argument.
//
//   - `/../`, `\\..\\` or `/..` anywhere inside.
//
// The check is deliberately conservative — a legitimate filename like
// `double..extension.txt` passes because there's no path-separator
// boundary on either side of `..`.
func rejectPathTraversal(scope, arg string) error {
	if arg == "" {
		return nil
	}
	if arg == ".." ||
		core.HasPrefix(arg, "../") ||
		core.HasPrefix(arg, "..\\") ||
		core.Contains(arg, "/../") ||
		core.Contains(arg, "\\..\\") ||
		core.HasSuffix(arg, "/..") ||
		core.HasSuffix(arg, "\\..") {
		return coreerr.E(
			"app.CheckAccess",
			scope+" access refused: path traversal in '"+arg+"'",
			nil,
		)
	}
	return nil
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
