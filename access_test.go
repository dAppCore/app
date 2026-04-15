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

// TestAccess_CheckAccess_Store — Config["store"]=true and the
// Filesystem fallback both grant the store gate; everything else is
// rejected.
func TestAccess_CheckAccess_Store(t *testing.T) {
	declared := &config.ViewManifest{Config: map[string]any{"store": true}}
	if err := CheckAccess(declared, AccessStore, "prefs:theme"); err != nil {
		t.Errorf("Config[store]=true should grant store: %v", err)
	}

	legacy := &config.ViewManifest{
		Permissions: config.ViewPermissions{Filesystem: true},
	}
	if err := CheckAccess(legacy, AccessStore, "prefs:theme"); err != nil {
		t.Errorf("Filesystem=true should grant store (legacy): %v", err)
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
