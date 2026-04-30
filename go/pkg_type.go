// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// PackageType enumerates the wrapping modes `core pkg` understands. Each
// value corresponds to an RFC §16 detection pattern:
//
//   - PackageTypeNative — a source directory already carrying .core/view.yaml.
//
//   - PackageTypePWA    — a web app with a standards PWA manifest
//     (`manifest.json` or `manifest.webmanifest`).
//
//   - PackageTypeElectron — an Electron app whose renderer assets we
//     download from GitHub releases.
//
//   - PackageTypeWeb    — a plain web directory we wrap as a CoreApp.
//
//     t := app.DetectPackageType(coreio.Local, "./my-app")
//     switch t {
//     case app.PackageTypeNative:
//     case app.PackageTypePWA:
//     }
type PackageType int

const (
	// PackageTypeUnknown is the zero value — detection failed.
	PackageTypeUnknown PackageType = iota
	// PackageTypeNative is a directory with .core/view.yaml.
	PackageTypeNative
	// PackageTypePWA is a web app with a standards PWA manifest.
	PackageTypePWA
	// PackageTypeElectron is an Electron app (package.json declares
	// `electron` in dependencies or devDependencies).
	PackageTypeElectron
	// PackageTypeWeb is any other bundle of web assets (HTML/CSS/JS).
	PackageTypeWeb
)

// String returns the lowercase name used in manifests and CLI output.
//
//	app.PackageTypePWA.String() // "pwa"
func (t PackageType) String() string {
	switch t {
	case PackageTypeNative:
		return "native"
	case PackageTypePWA:
		return "pwa"
	case PackageTypeElectron:
		return "electron"
	case PackageTypeWeb:
		return "web"
	default:
		return "unknown"
	}
}

// ParsePackageType maps a string (from marketplace index, CLI flag, etc.)
// back to a PackageType. Returns PackageTypeUnknown for any unrecognised
// value so the caller can decide on a fallback.
//
//	t := app.ParsePackageType("pwa") // PackageTypePWA
func ParsePackageType(s string) PackageType {
	switch core.Lower(core.Trim(s)) {
	case "native":
		return PackageTypeNative
	case "pwa":
		return PackageTypePWA
	case "electron":
		return PackageTypeElectron
	case "web":
		return PackageTypeWeb
	default:
		return PackageTypeUnknown
	}
}

// DetectPackageType inspects a local directory and returns the best-fit
// PackageType. Matches the RFC §16.4 detection table for local sources —
// native wins over PWA wins over Electron wins over Web. Returns
// PackageTypeUnknown if nothing recognisable is present.
//
//	t := app.DetectPackageType(coreio.Local, "./my-app")
//
// Rules (first match wins):
//
//  1. Directory contains `.core/view.yaml`     → PackageTypeNative
//  2. Directory contains `manifest.json` or
//     `manifest.webmanifest` that parses as a PWA
//     (has `start_url`)                         → PackageTypePWA
//  3. Directory contains `package.json` with
//     an `electron` dependency                  → PackageTypeElectron
//  4. Directory contains `index.html` (any html) → PackageTypeWeb
//  5. Otherwise                                  → PackageTypeUnknown
func DetectPackageType(medium coreio.Medium, dir string) PackageType {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return PackageTypeUnknown
	}

	if medium.Exists(core.Path(dir, ".core", "view.yaml")) {
		return PackageTypeNative
	}

	if manifestPath, ok := FindLocalPWAManifest(medium, dir); ok {
		body, err := medium.Read(manifestPath)
		if err == nil && detectsLocalPWA(body) {
			return PackageTypePWA
		}
	}

	pkgJSON := core.Path(dir, "package.json")
	if medium.Exists(pkgJSON) {
		body, err := medium.Read(pkgJSON)
		if err == nil && looksLikeElectronPackageJSON(body) {
			return PackageTypeElectron
		}
	}

	if medium.Exists(core.Path(dir, "index.html")) {
		return PackageTypeWeb
	}

	return PackageTypeUnknown
}

// looksLikeElectronPackageJSON returns true when a package.json body
// mentions electron in a dependency or devDependency position. A raw
// substring check is deliberate — pulling in encoding/json would violate
// the core banned-import rule (core.JSONUnmarshal is the approved path),
// and the manifest is used only for type detection.
//
//	if looksLikeElectronPackageJSON(body) { /* Electron app */ }
func looksLikeElectronPackageJSON(body string) bool {
	if body == "" {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	r := core.JSONUnmarshal([]byte(body), &pkg)
	if !r.OK {
		// Fall back to a token-level scan if the JSON is malformed — an
		// Electron app is an Electron app even when package.json is
		// syntactically imperfect.
		return core.Contains(body, "\"electron\":") ||
			core.Contains(body, "\"electron-builder\":")
	}
	if _, ok := pkg.Dependencies["electron"]; ok {
		return true
	}
	if _, ok := pkg.DevDependencies["electron"]; ok {
		return true
	}
	return false
}

// DetectPackageTypeFromManifestJSON inspects a raw PWA manifest body
// (typically fetched from a URL or loaded from `manifest.json` /
// `manifest.webmanifest`) and classifies it as a PWA if the bytes parse
// as a Web App Manifest.
//
//	t := app.DetectPackageTypeFromManifestJSON(body)
func DetectPackageTypeFromManifestJSON(body string) PackageType {
	if body == "" {
		return PackageTypeUnknown
	}
	var pwa PWAManifest
	r := core.JSONUnmarshal([]byte(body), &pwa)
	if !r.OK {
		return PackageTypeUnknown
	}
	if pwa.StartURL == "" && pwa.Name == "" && pwa.ShortName == "" {
		return PackageTypeUnknown
	}
	return PackageTypePWA
}

// detectsLocalPWA is the stricter local-directory counterpart to
// DetectPackageTypeFromManifestJSON. The local RFC §16.4 detection table
// requires an explicit `start_url` before a manifest file upgrades the
// directory to PackageTypePWA; otherwise a directory with a decorative
// manifest and plain index.html continues through the Web path.
func detectsLocalPWA(body string) bool {
	if body == "" {
		return false
	}
	var pwa PWAManifest
	r := core.JSONUnmarshal([]byte(body), &pwa)
	if !r.OK {
		return false
	}
	return pwa.StartURL != ""
}

// errDetect is the canonical error for unknown-type detection — kept as
// a helper so callers wiring CLI exits don't duplicate the string.
//
//	return errDetect("./path")
func errDetect(
	where string,
) error {
	return core.E(
		"app.DetectPackageType",
		"cannot determine package type at "+where+" (no .core/view.yaml, manifest.json, manifest.webmanifest, package.json, or index.html)",
		nil,
	)
}
