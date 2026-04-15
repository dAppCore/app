// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
)

// TestPkgType_PackageType_String_Good confirms every valid PackageType
// renders to the lowercase name documented in RFC §16.3 `core pkg list`.
func TestPkgType_PackageType_String_Good(t *testing.T) {
	cases := []struct {
		typ  app.PackageType
		want string
	}{
		{app.PackageTypeNative, "native"},
		{app.PackageTypePWA, "pwa"},
		{app.PackageTypeElectron, "electron"},
		{app.PackageTypeWeb, "web"},
	}
	for _, tc := range cases {
		if got := tc.typ.String(); got != tc.want {
			t.Errorf("%v.String() = %q; want %q", tc.typ, got, tc.want)
		}
	}
}

// TestPkgType_PackageType_String_Bad confirms the zero/unknown value
// renders as "unknown" instead of panicking.
func TestPkgType_PackageType_String_Bad(t *testing.T) {
	if got := app.PackageTypeUnknown.String(); got != "unknown" {
		t.Errorf("PackageTypeUnknown.String() = %q; want %q", got, "unknown")
	}
	// A value outside the declared iota set still returns "unknown".
	if got := app.PackageType(99).String(); got != "unknown" {
		t.Errorf("PackageType(99).String() = %q; want %q", got, "unknown")
	}
}

// TestPkgType_ParsePackageType_Good checks every canonical string
// round-trips back to the matching PackageType.
func TestPkgType_ParsePackageType_Good(t *testing.T) {
	cases := []struct {
		in   string
		want app.PackageType
	}{
		{"native", app.PackageTypeNative},
		{"pwa", app.PackageTypePWA},
		{"electron", app.PackageTypeElectron},
		{"web", app.PackageTypeWeb},
		{"  Native  ", app.PackageTypeNative}, // trimmed + lowercased
		{"ELECTRON", app.PackageTypeElectron},
	}
	for _, tc := range cases {
		if got := app.ParsePackageType(tc.in); got != tc.want {
			t.Errorf("ParsePackageType(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

// TestPkgType_ParsePackageType_Bad confirms that unknown strings map to
// PackageTypeUnknown, not a spurious match.
func TestPkgType_ParsePackageType_Bad(t *testing.T) {
	for _, in := range []string{"", "webby", "flatpak", "???"} {
		if got := app.ParsePackageType(in); got != app.PackageTypeUnknown {
			t.Errorf("ParsePackageType(%q) = %v; want Unknown", in, got)
		}
	}
}

// TestPkgType_DetectPackageType_Good walks the detection precedence:
// .core/view.yaml wins over manifest.json wins over package.json wins
// over index.html.
func TestPkgType_DetectPackageType_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	// Case 1 — native beats everything.
	native := core.Path(dir, "native")
	if err := medium.EnsureDir(core.Path(native, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(native, ".core", "view.yaml"), "code: x\nname: X\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, native); got != app.PackageTypeNative {
		t.Errorf("native case = %v; want Native", got)
	}

	// Case 2 — PWA via manifest.json.
	pwa := core.Path(dir, "pwa")
	if err := medium.EnsureDir(pwa); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(pwa, "manifest.json"), `{"name":"X","start_url":"/"}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, pwa); got != app.PackageTypePWA {
		t.Errorf("pwa case = %v; want PWA", got)
	}

	// Case 3 — Electron via package.json.
	electron := core.Path(dir, "electron")
	if err := medium.EnsureDir(electron); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(electron, "package.json"), `{"name":"x","dependencies":{"electron":"^26.0.0"}}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, electron); got != app.PackageTypeElectron {
		t.Errorf("electron case = %v; want Electron", got)
	}

	// Case 4 — plain Web via index.html only.
	web := core.Path(dir, "web")
	if err := medium.EnsureDir(web); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(web, "index.html"), `<html></html>`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, web); got != app.PackageTypeWeb {
		t.Errorf("web case = %v; want Web", got)
	}
}

// TestPkgType_DetectPackageType_Bad handles the empty-dir path.
func TestPkgType_DetectPackageType_Bad(t *testing.T) {
	dir := t.TempDir()
	if got := app.DetectPackageType(coreio.Local, dir); got != app.PackageTypeUnknown {
		t.Errorf("empty dir = %v; want Unknown", got)
	}
	// Empty input → Unknown (no panic).
	if got := app.DetectPackageType(coreio.Local, ""); got != app.PackageTypeUnknown {
		t.Errorf("empty string = %v; want Unknown", got)
	}
}

// TestPkgType_DetectPackageType_Ugly covers the permutations that trip
// casual detection: manifest.json without start_url (not a PWA),
// package.json without electron (not an Electron app).
func TestPkgType_DetectPackageType_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	// manifest.json with no start_url → falls through to index.html.
	a := core.Path(dir, "a")
	if err := medium.EnsureDir(a); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(a, "manifest.json"), `{"name":"A"}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := medium.Write(core.Path(a, "index.html"), `<html></html>`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, a); got != app.PackageTypeWeb {
		t.Errorf("manifest-no-start_url with html = %v; want Web", got)
	}

	// package.json with no electron dep + no other markers → Unknown.
	b := core.Path(dir, "b")
	if err := medium.EnsureDir(b); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(b, "package.json"), `{"name":"b","dependencies":{"react":"^18"}}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := app.DetectPackageType(medium, b); got != app.PackageTypeUnknown {
		t.Errorf("package.json no electron = %v; want Unknown", got)
	}

	// Malformed package.json with electron substring still detects.
	c := core.Path(dir, "c")
	if err := medium.EnsureDir(c); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(c, "package.json"), `{"deps": "electron"` /* broken */); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// With malformed JSON the fallback substring check sees "electron"
	// but not "\"electron\":" — should be Unknown, not Electron.
	if got := app.DetectPackageType(medium, c); got != app.PackageTypeUnknown {
		t.Errorf("malformed package.json = %v; want Unknown", got)
	}
}

// TestPkgType_DetectPackageTypeFromManifestJSON_Good confirms a clean
// PWA manifest parses as a PWA.
func TestPkgType_DetectPackageTypeFromManifestJSON_Good(t *testing.T) {
	body := `{"name":"Play","short_name":"play","start_url":"/","display":"standalone"}`
	if got := app.DetectPackageTypeFromManifestJSON(body); got != app.PackageTypePWA {
		t.Errorf("PWA manifest = %v; want PWA", got)
	}
}

// TestPkgType_DetectPackageTypeFromManifestJSON_Bad rejects empty and
// garbage input without crashing.
func TestPkgType_DetectPackageTypeFromManifestJSON_Bad(t *testing.T) {
	for _, in := range []string{"", "not json at all", `{"unrelated":"yes"}`} {
		if got := app.DetectPackageTypeFromManifestJSON(in); got != app.PackageTypeUnknown {
			t.Errorf("Detect(%q) = %v; want Unknown", in, got)
		}
	}
}
