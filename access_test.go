// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	"dappco.re/go/core/config"
)

// TestAccess_CheckAccess_Good — reading a path inside a declared
// prefix, fetching a declared host:port and running a declared binary
// are all granted.
func TestAccess_CheckAccess_Good(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
			Net:  []string{"api.example.com:443"},
			Run:  []string{"ffmpeg"},
		},
	}

	if err := CheckAccess(m, AccessRead, "./photos/sunset.jpg"); err != nil {
		t.Errorf("read inside declared prefix should be allowed: %v", err)
	}
	if err := CheckAccess(m, AccessNet, "api.example.com:443"); err != nil {
		t.Errorf("net exact match should be allowed: %v", err)
	}
	if err := CheckAccess(m, AccessRun, "ffmpeg"); err != nil {
		t.Errorf("run exact match should be allowed: %v", err)
	}
}

// TestAccess_CheckAccess_Bad — paths outside declared prefixes, hosts
// that differ by even a character, and unknown binaries are all
// rejected with a descriptive error.
func TestAccess_CheckAccess_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
			Net:  []string{"api.example.com:443"},
			Run:  []string{"ffmpeg"},
		},
	}

	if err := CheckAccess(m, AccessRead, "/etc/passwd"); err == nil {
		t.Error("read outside declared prefix should be denied")
	}
	// Exact match — must not leak by substring.
	if err := CheckAccess(m, AccessNet, "evil-api.example.com:443"); err == nil {
		t.Error("similar-but-different host should be denied")
	}
	if err := CheckAccess(m, AccessRun, "rm"); err == nil {
		t.Error("undeclared binary should be denied")
	}
	// nil manifest → safe error, not panic.
	if err := CheckAccess(nil, AccessRead, "whatever"); err == nil {
		t.Error("nil manifest should error")
	}
	// Unknown access mode.
	if err := CheckAccess(m, AccessMode(99), "whatever"); err == nil {
		t.Error("unknown AccessMode should error")
	}
}

// TestAccess_CheckAccess_Ugly — legacy Filesystem / Network booleans
// grant full access (backwards compatibility). Empty declared lists
// correctly deny.
func TestAccess_CheckAccess_Ugly(t *testing.T) {
	legacy := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Filesystem: true,
			Network:    true,
		},
	}
	if err := CheckAccess(legacy, AccessRead, "/anywhere/at/all"); err != nil {
		t.Errorf("Filesystem=true should grant read: %v", err)
	}
	if err := CheckAccess(legacy, AccessWrite, "/anywhere/at/all"); err != nil {
		t.Errorf("Filesystem=true should grant write: %v", err)
	}
	if err := CheckAccess(legacy, AccessNet, "any.host:80"); err != nil {
		t.Errorf("Network=true should grant net: %v", err)
	}

	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessRead, "/tmp/a"); err == nil {
		t.Error("empty manifest should deny read")
	}
	if err := CheckAccess(empty, AccessWrite, "/tmp/a"); err == nil {
		t.Error("empty manifest should deny write")
	}
	if err := CheckAccess(empty, AccessRun, "ls"); err == nil {
		t.Error("empty manifest should deny run")
	}
}

// TestAccess_matchPrefix_Good — declared prefix covers nested paths.
func TestAccess_matchPrefix_Good(t *testing.T) {
	list := []string{"./data/", "./config/"}
	if !matchPrefix(list, "./data/a.txt") {
		t.Error("nested path under declared prefix should match")
	}
}

// TestAccess_matchPrefix_Bad — empty list + empty entries don't
// accidentally match anything.
func TestAccess_matchPrefix_Bad(t *testing.T) {
	if matchPrefix(nil, "./whatever") {
		t.Error("nil list should not match")
	}
	if matchPrefix([]string{""}, "./anything") {
		t.Error("empty-string entries should be ignored")
	}
}

// TestAccess_matchPrefix_Ugly — exact match on the prefix itself also
// counts (a declaration of "./data/" grants access to "./data/").
func TestAccess_matchPrefix_Ugly(t *testing.T) {
	if !matchPrefix([]string{"./data/"}, "./data/") {
		t.Error("prefix equals arg should match")
	}
}

// TestAccess_matchExact_Good — exact equality is required.
func TestAccess_matchExact_Good(t *testing.T) {
	list := []string{"api.example.com:443", "db.example.com:5432"}
	if !matchExact(list, "api.example.com:443") {
		t.Error("exact entry should match")
	}
}

// TestAccess_matchExact_Bad — prefix is NOT enough (this is the whole
// point — hosts can't be approximated).
func TestAccess_matchExact_Bad(t *testing.T) {
	list := []string{"api.example.com:443"}
	if matchExact(list, "api.example.com:444") {
		t.Error("different port should not match")
	}
	if matchExact(list, "api.example.com") {
		t.Error("missing port should not match")
	}
}

// TestAccess_matchExact_Ugly — empty list + empty arg neither panic
// nor accidentally match.
func TestAccess_matchExact_Ugly(t *testing.T) {
	if matchExact(nil, "anything") {
		t.Error("nil list should not match")
	}
	if matchExact([]string{"x"}, "") {
		t.Error("empty arg should not match a non-empty list")
	}
}

// TestAccess_matchNet_Good — exact entries still match and the RFC
// wildcard form `net: ["*"]` grants any host:port.
func TestAccess_matchNet_Good(t *testing.T) {
	if !matchNet([]string{"api.example.com:443"}, "api.example.com:443") {
		t.Error("exact host:port should match")
	}
	if !matchNet([]string{"*"}, "api.example.com:443") {
		t.Error("wildcard net permission should match any host")
	}
}

// TestAccess_matchNet_Bad — different hosts without a wildcard are
// still denied.
func TestAccess_matchNet_Bad(t *testing.T) {
	if matchNet([]string{"api.example.com:443"}, "api.example.com:444") {
		t.Error("different port should not match without wildcard")
	}
}

// TestAccess_matchNet_Ugly — nil / empty inputs stay false and do not
// panic.
func TestAccess_matchNet_Ugly(t *testing.T) {
	if matchNet(nil, "anything") {
		t.Error("nil list should not match")
	}
	if matchNet([]string{""}, "anything") {
		t.Error("empty entry should not match")
	}
}

// TestAccess_AccessMode_String_Good — enum values round-trip through
// their lowercase name.
func TestAccess_AccessMode_String_Good(t *testing.T) {
	cases := map[AccessMode]string{
		AccessRead:           "read",
		AccessWrite:          "write",
		AccessNet:            "net",
		AccessRun:            "run",
		AccessStore:          "store",
		AccessNotification:   "notifications",
		AccessClipboardRead:  "clipboard",
		AccessClipboardWrite: "clipboard",
		AccessCamera:         "camera",
		AccessMicrophone:     "microphone",
		AccessLocation:       "location",
	}
	for mode, want := range cases {
		if got := mode.String(); got != want {
			t.Errorf("%v.String() = %q; want %q", mode, got, want)
		}
	}
}

// TestAccess_CheckAccess_Store — only an explicit store declaration
// grants the store gate; everything else is rejected.
func TestAccess_CheckAccess_Store(t *testing.T) {
	declared := &config.ViewManifest{Config: map[string]any{"store": true}}
	if err := CheckAccess(declared, AccessStore, "prefs:theme"); err != nil {
		t.Errorf("Config[store]=true should grant store: %v", err)
	}

	filesystemOnly := &config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}
	if err := CheckAccess(filesystemOnly, AccessStore, "prefs:theme"); err == nil {
		t.Error("Filesystem=true without store should deny store")
	}

	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessStore, "prefs:theme"); err == nil {
		t.Error("empty manifest should deny store")
	}
}

// TestAccess_AccessMode_String_Bad — an unrecognised int returns
// "unknown" so log lines still show something useful.
func TestAccess_AccessMode_String_Bad(t *testing.T) {
	if got := AccessMode(99).String(); got != "unknown" {
		t.Errorf("unknown mode = %q; want %q", got, "unknown")
	}
}

// TestAccess_AccessMode_String_Ugly — negative values also fall
// through to "unknown".
func TestAccess_AccessMode_String_Ugly(t *testing.T) {
	if got := AccessMode(-1).String(); got != "unknown" {
		t.Errorf("negative mode = %q; want %q", got, "unknown")
	}
}

// TestAccess_CheckAccess_WriteList — Config["write"] grants per-path
// write access without flipping the catch-all Filesystem flag. Mirrors
// RFC §2.2 `write: ["./photos/.thumbnails/"]` example.
func TestAccess_CheckAccess_WriteList(t *testing.T) {
	declared := &config.ViewManifest{
		Config: map[string]any{
			"write": []any{"./photos/.thumbnails/"},
		},
	}
	if err := CheckAccess(declared, AccessWrite, "./photos/.thumbnails/sunset.webp"); err != nil {
		t.Errorf("declared write prefix should grant: %v", err)
	}
	if err := CheckAccess(declared, AccessWrite, "./other/file.txt"); err == nil {
		t.Error("write outside declared prefix should deny")
	}

	// []string also accepted (some callers stash typed slices directly).
	typed := &config.ViewManifest{
		Config: map[string]any{"write": []string{"./out/"}},
	}
	if err := CheckAccess(typed, AccessWrite, "./out/build.log"); err != nil {
		t.Errorf("typed []string write list should grant: %v", err)
	}

	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessWrite, "./out/build.log"); err == nil {
		t.Error("empty manifest should deny write")
	}
}

// TestAccess_CheckAccess_Notification — notifications gate granted only
// when permissions.notifications: true is declared.
func TestAccess_CheckAccess_Notification(t *testing.T) {
	declared := &config.ViewManifest{
		Permissions: config.ViewPermissions{Notifications: true},
	}
	if err := CheckAccess(declared, AccessNotification, ""); err != nil {
		t.Errorf("notifications=true should grant: %v", err)
	}

	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessNotification, ""); err == nil {
		t.Error("empty manifest should deny notification")
	}
}

// TestAccess_CheckAccess_Clipboard — Clipboard=true grants both
// read and write directions; an empty manifest denies both.
func TestAccess_CheckAccess_Clipboard(t *testing.T) {
	declared := &config.ViewManifest{
		Permissions: config.ViewPermissions{Clipboard: true},
	}
	if err := CheckAccess(declared, AccessClipboardRead, ""); err != nil {
		t.Errorf("clipboard=true should grant read: %v", err)
	}
	if err := CheckAccess(declared, AccessClipboardWrite, ""); err != nil {
		t.Errorf("clipboard=true should grant write: %v", err)
	}
	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessClipboardRead, ""); err == nil {
		t.Error("empty manifest should deny clipboard read")
	}
	if err := CheckAccess(empty, AccessClipboardWrite, ""); err == nil {
		t.Error("empty manifest should deny clipboard write")
	}
}

// TestAccess_rejectPathTraversal_Good — the direct traversal forms
// surface a typed error regardless of mode.
func TestAccess_rejectPathTraversal_Good(t *testing.T) {
	cases := []string{
		"..",
		"../etc/passwd",
		"./data/../etc/passwd",
		"/foo/../bar",
		"..\\windows\\system32",
		"C:\\data\\..\\Windows",
		"foo/..",
		"foo\\..",
	}
	for _, arg := range cases {
		if err := rejectPathTraversal("read", arg); err == nil {
			t.Errorf("rejectPathTraversal(%q) = nil; want error", arg)
		}
	}
}

// TestAccess_rejectPathTraversal_Bad — legitimate paths (no `..`
// segment) return nil so the normal match check can proceed.
func TestAccess_rejectPathTraversal_Bad(t *testing.T) {
	cases := []string{
		"",
		"./data/a.jpg",
		"/tmp/file",
		"double..extension.txt", // no path boundary around `..`
		"foo..bar",
	}
	for _, arg := range cases {
		if err := rejectPathTraversal("read", arg); err != nil {
			t.Errorf("rejectPathTraversal(%q) = %v; want nil", arg, err)
		}
	}
}

// TestAccess_rejectPathTraversal_Ugly — scope name is carried into the
// error message so callers can match denial to the right operation.
func TestAccess_rejectPathTraversal_Ugly(t *testing.T) {
	err := rejectPathTraversal("write", "./photos/../etc/shadow")
	if err == nil {
		t.Fatal("expected traversal error")
	}
	if !contains(err.Error(), "write") {
		t.Errorf("error should mention the scope 'write': %v", err)
	}
	if !contains(err.Error(), "./photos/../etc/shadow") {
		t.Errorf("error should echo the offending arg: %v", err)
	}
}

// contains is a tiny substring helper the path-traversal assertions
// use — kept local to the test file so production code doesn't grow a
// duplicate of core.Contains.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestAccess_CheckAccess_Traversal_Good — a declared-prefix manifest
// rejects a traversal attempt even when the textual prefix would
// otherwise match. Proves RFC §5.2 / §10.2 "Path traversal blocked"
// holds for the public gate.
func TestAccess_CheckAccess_Traversal_Good(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
		},
	}
	if err := CheckAccess(m, AccessRead, "./photos/../etc/passwd"); err == nil {
		t.Error("traversal through declared prefix should be denied")
	}
}

// TestAccess_CheckAccess_Traversal_Bad — legacy Filesystem=true still
// rejects a traversal attempt so the "blanket allow" flag does not
// become a footgun. RFC §5.2 is a hard rule of the gate.
func TestAccess_CheckAccess_Traversal_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}
	if err := CheckAccess(m, AccessRead, "./data/../etc/passwd"); err == nil {
		t.Error("Filesystem=true must not bypass traversal defence")
	}
	if err := CheckAccess(m, AccessWrite, "./data/../etc/passwd"); err == nil {
		t.Error("Filesystem=true must not bypass traversal defence on write")
	}
}

// TestAccess_CheckAccess_Traversal_Ugly — Config["write"] list also
// subject to the traversal defence, so a clever manifest can't slip
// past by using the per-path list instead of Filesystem=true.
func TestAccess_CheckAccess_Traversal_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Config: map[string]any{
			"write": []any{"./tmp/"},
		},
	}
	if err := CheckAccess(m, AccessWrite, "./tmp/../etc/passwd"); err == nil {
		t.Error("Config[write] prefix must not bypass traversal defence")
	}
}

// TestAccess_CheckAccess_Devices — Camera, Microphone and Location
// gates honour their respective declarations and deny otherwise. The
// location gate honours the legacy `device.location` entry stored in
// permissions.run because ViewPermissions has no typed slot yet.
func TestAccess_CheckAccess_Devices(t *testing.T) {
	cam := &config.ViewManifest{
		Permissions: config.ViewPermissions{Camera: true},
	}
	if err := CheckAccess(cam, AccessCamera, ""); err != nil {
		t.Errorf("camera=true should grant: %v", err)
	}
	mic := &config.ViewManifest{
		Permissions: config.ViewPermissions{Microphone: true},
	}
	if err := CheckAccess(mic, AccessMicrophone, ""); err != nil {
		t.Errorf("microphone=true should grant: %v", err)
	}
	loc := &config.ViewManifest{
		Permissions: config.ViewPermissions{Run: []string{"device.location"}},
	}
	if err := CheckAccess(loc, AccessLocation, ""); err != nil {
		t.Errorf("location declared via Run should grant: %v", err)
	}

	empty := &config.ViewManifest{}
	if err := CheckAccess(empty, AccessCamera, ""); err == nil {
		t.Error("empty manifest should deny camera")
	}
	if err := CheckAccess(empty, AccessMicrophone, ""); err == nil {
		t.Error("empty manifest should deny microphone")
	}
	if err := CheckAccess(empty, AccessLocation, ""); err == nil {
		t.Error("empty manifest should deny location")
	}
}

// TestAccess_ActionAccessMode_Good covers the dotted-prefix dispatch —
// every action in the RFC §9.3 table maps to the right AccessMode.
func TestAccess_ActionAccessMode_Good(t *testing.T) {
	cases := []struct {
		action string
		want   AccessMode
	}{
		{"fs.read", AccessRead},
		{"fs.list", AccessRead},
		{"fs.write", AccessWrite},
		{"fs.delete", AccessWrite},
		{"net.fetch", AccessNet},
		{"net.ws", AccessNet},
		{"process.run", AccessRun},
		{"process.stdout.subscribe", AccessRun},
		{"store.get", AccessStore},
		{"store.set", AccessStore},
		{"gui.notification.send", AccessNotification},
		{"gui.clipboard.read", AccessClipboardRead},
		{"gui.clipboard.write", AccessClipboardWrite},
		{"gui.browser.open", AccessNet},
		{"device.camera", AccessCamera},
		{"device.microphone", AccessMicrophone},
		{"device.location", AccessLocation},
		{"brain.recall", AccessNet},
	}
	for _, c := range cases {
		got, ok := ActionAccessMode(c.action)
		if !ok {
			t.Errorf("ActionAccessMode(%q) = not gated; want %v", c.action, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("ActionAccessMode(%q) = %v; want %v", c.action, got, c.want)
		}
	}
}

// TestAccess_ActionAccessMode_Bad — ungated actions return ok=false so
// handlers know not to run CheckAccess.
func TestAccess_ActionAccessMode_Bad(t *testing.T) {
	ungated := []string{
		"gui.window.create",
		"gui.dialog.confirm",
		"gui.dialog.open",
		"gui.dialog.save",
		"i18n.translate",
		"ipc.pub.publish",
		"ipc.req.send",
		"auth.create",
		"crypto.pgp.sign",
		"unknown.action",
		"",
	}
	for _, action := range ungated {
		if _, ok := ActionAccessMode(action); ok {
			t.Errorf("ActionAccessMode(%q) returned ok=true; want false (ungated)", action)
		}
	}
}

// TestAccess_CheckActionAccess_Good exercises the one-liner handlers
// use to gate an action+argument in a single call.
func TestAccess_CheckActionAccess_Good(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
			Net:  []string{"api.example.com:443"},
		},
	}
	if err := CheckActionAccess(m, "fs.read", "./photos/a.jpg"); err != nil {
		t.Errorf("fs.read inside declared prefix should succeed: %v", err)
	}
	if err := CheckActionAccess(m, "net.fetch", "api.example.com:443"); err != nil {
		t.Errorf("net.fetch against declared host should succeed: %v", err)
	}
	// Ungated action — the helper returns nil without consulting perms.
	if err := CheckActionAccess(m, "gui.dialog.confirm", "anything"); err != nil {
		t.Errorf("ungated action should return nil: %v", err)
	}
}

// TestAccess_CheckActionAccess_Bad — gated action + undeclared argument
// surfaces the same denial the underlying CheckAccess would produce.
func TestAccess_CheckActionAccess_Bad(t *testing.T) {
	m := &config.ViewManifest{
		Permissions: config.ViewPermissions{
			Read: []string{"./photos/"},
		},
	}
	if err := CheckActionAccess(m, "fs.read", "/etc/passwd"); err == nil {
		t.Error("fs.read on undeclared path should be denied")
	}
	if err := CheckActionAccess(nil, "fs.read", "./x"); err == nil {
		t.Error("nil manifest should error")
	}
}
