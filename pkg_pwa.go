// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
	"gopkg.in/yaml.v3"
)

// PWAIcon is one entry in a PWA manifest's icons array. The wrapper
// selects the largest (by parsed width) when choosing an app icon.
//
//	{ "src": "/icons/512.png", "sizes": "512x512", "type": "image/png" }
type PWAIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type,omitempty"`
	Purpose string `json:"purpose,omitempty"`
}

// PWAManifest is the subset of the W3C Web App Manifest we consume when
// wrapping a PWA as a CoreApp. Only fields referenced by RFC §16.1 are
// represented; unknown keys in the source manifest are ignored (encoding
// /json.Unmarshal skips them).
//
//	var m PWAManifest
//	_ = core.JSONUnmarshal(body, &m)
type PWAManifest struct {
	Name            string    `json:"name"`
	ShortName       string    `json:"short_name,omitempty"`
	StartURL        string    `json:"start_url,omitempty"`
	Scope           string    `json:"scope,omitempty"`
	Display         string    `json:"display,omitempty"`
	Orientation     string    `json:"orientation,omitempty"`
	ThemeColor      string    `json:"theme_color,omitempty"`
	BackgroundColor string    `json:"background_color,omitempty"`
	Lang            string    `json:"lang,omitempty"`
	Description     string    `json:"description,omitempty"`
	Icons           []PWAIcon `json:"icons,omitempty"`
	Permissions     []string  `json:"permissions,omitempty"`
}

// pwaFetchTimeout caps the manifest-fetch HTTP call so a slow origin
// cannot hang `core pkg wrap`. 15s matches the dAppServer marketplace
// install-poll timeout.
//
//	client := &http.Client{Timeout: pwaFetchTimeout}
const pwaFetchTimeout = 15 * time.Second

// FetchPWAManifest performs an HTTP GET against the supplied URL and
// decodes the body as a PWAManifest. Used by `core pkg wrap --pwa <url>`
// when the caller points at a live manifest rather than a local file.
//
//	manifest, err := app.FetchPWAManifest(ctx, "https://app.example.com/manifest.json")
//
// Rules:
//
//   - Returns a coreerr.E-wrapped error for non-2xx responses, network
//     failures, and decode errors.
//
//   - Trims whitespace from the URL so CLI shells pasting trailing
//     spaces don't 400.
func FetchPWAManifest(ctx context.Context, url string) (*PWAManifest, error) {
	url = core.Trim(url)
	if url == "" {
		return nil, coreerr.E("app.FetchPWAManifest", "empty URL", nil)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, coreerr.E("app.FetchPWAManifest", "request build failed", err)
	}
	req.Header.Set("Accept", "application/manifest+json, application/json")

	client := &http.Client{Timeout: pwaFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, coreerr.E("app.FetchPWAManifest", "HTTP GET failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, coreerr.E(
			"app.FetchPWAManifest",
			"non-2xx status: "+core.Sprint(resp.StatusCode),
			nil,
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, coreerr.E("app.FetchPWAManifest", "read body failed", err)
	}

	var m PWAManifest
	r := core.JSONUnmarshal(body, &m)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, coreerr.E("app.FetchPWAManifest", "decode manifest.json failed", cause)
	}
	return &m, nil
}

// WrapPWAOptions tunes WrapPWA. A zero value is fine — the canonical
// slug falls back to the manifest's short_name or a slug derived from
// `name`, and `TargetURL` is only required when the manifest itself
// omits `start_url` (rare but permitted).
//
//	opts := app.WrapPWAOptions{
//	    TargetURL: "https://play.example.com",
//	    Code:      "play",
//	}
type WrapPWAOptions struct {
	// TargetURL overrides the manifest's `start_url`. Useful when the
	// fetch URL and the runtime URL diverge (CDN manifest vs app root).
	TargetURL string
	// Code overrides the auto-derived slug. Accepted forms: kebab-case
	// ASCII. Empty → derive from manifest.short_name / manifest.name.
	Code string
	// Version pins the generated manifest's version. Empty → "0.1.0"
	// (first-run default).
	Version string
}

// WrapPWA projects a fetched/loaded PWAManifest into a CoreApp
// ViewManifest per RFC §16.1. The generated manifest is unsigned — the
// caller signs it after wrap (via `app.Sign`) or leaves it for
// `core pkg install --sign` to do on the way to disk.
//
//	manifest := app.WrapPWA(pwa, app.WrapPWAOptions{TargetURL: url})
//
// Rules:
//
//   - `permissions.net` always contains the host:port derived from
//     TargetURL (or StartURL) — without it the app couldn't navigate.
//
//   - `permissions.store = true` via ViewPermissions — PWAs typically
//     need localStorage. Represented by setting `Filesystem=false` and
//     appending an explicit `store` entry to `Run` only if requested.
//     (ViewPermissions does not yet expose a Store bool; see
//     access.go TODO.)
func WrapPWA(src *PWAManifest, opts WrapPWAOptions) *config.ViewManifest {
	if src == nil {
		return nil
	}

	url := core.Trim(opts.TargetURL)
	if url == "" {
		url = src.StartURL
	}

	code := core.Trim(opts.Code)
	if code == "" {
		code = slugify(src.ShortName)
	}
	if code == "" {
		code = slugify(src.Name)
	}
	if code == "" {
		code = "pwa-app"
	}

	version := opts.Version
	if version == "" {
		version = "0.1.0"
	}

	m := &config.ViewManifest{
		Code:    code,
		Name:    coalesce(src.Name, src.ShortName, code),
		Version: version,
		Layout:  "C", // PWA single-surface app — Centre slot only
	}

	// Network permission derived from the app URL. Host:443 is the
	// default since PWAs are HTTPS by spec.
	if host := hostOfURL(url); host != "" {
		m.Permissions.Net = []string{host + ":443"}
	}

	// Map standard PWA permission strings to CoreApp permission slots.
	applyPWAPermissionMapping(m, src.Permissions)

	// Store the fields ViewManifest cannot yet type in `Config` so
	// downstream readers (marketplace index, SDK codegen) can still
	// see them — lossless round-trip for wrapped PWAs.
	m.Config = map[string]any{
		"type":       PackageTypePWA.String(),
		"url":        url,
		"display":    src.Display,
		"short_name": src.ShortName,
	}
	if src.ThemeColor != "" || src.BackgroundColor != "" {
		m.Config["theme"] = map[string]any{
			"primary":    src.ThemeColor,
			"background": src.BackgroundColor,
		}
	}
	if src.Lang != "" {
		m.Config["locale"] = src.Lang
	}
	if icon := largestIcon(src.Icons); icon != "" {
		m.Config["icon"] = icon
	}

	return m
}

// WritePWAWrap materialises a wrapped PWA as `<dest>/.core/view.yaml`.
// Convenience helper for CLI wiring and tests; it does nothing that
// Marshal+medium.Write can't do directly.
//
//	err := app.WritePWAWrap(coreio.Local, "/Users/me/.core/apps/play", manifest)
func WritePWAWrap(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if manifest == nil {
		return coreerr.E("app.WritePWAWrap", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return coreerr.E("app.WritePWAWrap", "marshal failed", err)
	}
	path := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return coreerr.E("app.WritePWAWrap", "ensure dir failed", err)
	}
	if err := medium.Write(path, string(body)); err != nil {
		return coreerr.E("app.WritePWAWrap", "write failed", err)
	}
	return nil
}

// applyPWAPermissionMapping maps RFC §16.1 PWA permission strings onto
// the ViewPermissions struct slots that actually exist today. Unknown
// permissions are silently dropped (forward-compat).
//
//	applyPWAPermissionMapping(m, []string{"notifications", "clipboard-read"})
func applyPWAPermissionMapping(m *config.ViewManifest, perms []string) {
	if m == nil {
		return
	}
	seen := map[string]bool{}
	for _, p := range perms {
		p = core.Lower(core.Trim(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		switch p {
		case "notifications":
			m.Permissions.Notifications = true
		case "clipboard-read", "clipboard-write":
			m.Permissions.Clipboard = true
		case "camera":
			m.Permissions.Camera = true
		case "microphone":
			m.Permissions.Microphone = true
		case "geolocation":
			// ViewPermissions has no location slot yet — record in Run
			// so the field survives the round-trip and the access gate
			// can surface a precise denial reason.
			m.Permissions.Run = append(m.Permissions.Run, "device.location")
		case "storage", "persistent-storage":
			// Local storage is available by default in PWAs under
			// CoreGUI (ts RFC §5). No-op for now; when ViewPermissions
			// grows a Store bool this flips it.
			_ = p
		}
	}
}

// slugify turns a display name into a kebab-case ASCII slug suitable
// for `code` identifiers. Keeps alphanumerics and hyphens, lower-cases,
// and collapses runs of other characters into single hyphens.
//
//	slugify("Play Example!") // "play-example"
func slugify(in string) string {
	if in == "" {
		return ""
	}
	in = core.Lower(in)
	b := core.NewBuilder()
	lastDash := false
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			// non-ASCII / punctuation — treated as separator
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := b.String()
	out = core.TrimSuffix(out, "-")
	return out
}

// hostOfURL returns the host[:port] portion of an HTTP/HTTPS URL with
// no scheme or path. Returns an empty string for malformed input so the
// caller can decide on a fallback.
//
//	hostOfURL("https://app.example.com/manifest.json") // "app.example.com"
func hostOfURL(url string) string {
	if url == "" {
		return ""
	}
	u := core.TrimPrefix(url, "https://")
	u = core.TrimPrefix(u, "http://")
	if idx := indexOf(u, "/"); idx > 0 {
		u = u[:idx]
	}
	return u
}

// indexOf returns the first index of needle in haystack, or -1. Small
// helper kept local to this file — core.Contains already exists for
// truthy checks, but RFC §16 explicitly slices on the first "/".
//
//	indexOf("app.example.com/manifest.json", "/") // 15
func indexOf(s, needle string) int {
	if needle == "" {
		return -1
	}
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// largestIcon picks the icon with the largest width from a PWA icons
// list by parsing the `sizes` field. Returns the Src URL.
//
//	largestIcon(icons) // "/icons/512.png"
func largestIcon(icons []PWAIcon) string {
	if len(icons) == 0 {
		return ""
	}
	best := icons[0]
	bestWidth := sizeWidth(best.Sizes)
	for _, ic := range icons[1:] {
		if w := sizeWidth(ic.Sizes); w > bestWidth {
			best = ic
			bestWidth = w
		}
	}
	return best.Src
}

// sizeWidth parses "512x512" → 512. Returns 0 for "any" or malformed
// inputs so they never outrank a real size.
//
//	sizeWidth("512x512") // 512
//	sizeWidth("any")     // 0
func sizeWidth(s string) int {
	s = core.Trim(s)
	if s == "" || s == "any" {
		return 0
	}
	// sizes can be space-separated: "192x192 512x512" — take the first.
	if idx := indexOf(s, " "); idx > 0 {
		s = s[:idx]
	}
	idx := indexOf(s, "x")
	if idx <= 0 {
		return 0
	}
	n := 0
	for _, r := range s[:idx] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// coalesce returns the first non-empty string from the supplied list.
// Keeps WrapPWA readable when three fallbacks apply.
//
//	coalesce("", "", "fallback") // "fallback"
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
