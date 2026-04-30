// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	// AX-6 fetch-boundary exception: the pinned core module has no HTTP
	// client wrapper, and the batch sandbox blocks shelling out to curl.
	"net/http"
	neturl "net/url"
	"time"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
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

// localPWAManifestNames is the ordered set of manifest filenames a
// local or repo-backed PWA may carry. `manifest.json` stays first
// because RFC §16.1 uses that name in examples, but real-world apps
// commonly ship `manifest.webmanifest`.
var localPWAManifestNames = []string{"manifest.json", "manifest.webmanifest"}

// pwaFetchTimeout caps the manifest-fetch HTTP call so a slow origin
// cannot hang `core pkg wrap`. 15s matches the dAppServer marketplace
// install-poll timeout.
//
//	fetchPWAURL(ctx, "https://app.example.com/manifest.json")
const pwaFetchTimeout = 15 * time.Second

// FetchPWAManifest performs an HTTP GET against the supplied URL and
// decodes the body as a PWAManifest. Used by `core pkg wrap --pwa <url>`
// when the caller points at either a live manifest URL or an app URL.
//
//	manifest, err := app.FetchPWAManifest(ctx, "https://app.example.com/manifest.json")
//
// Rules:
//
//   - Returns a core.E-wrapped error for non-2xx responses, network
//     failures, and decode errors.
//
//   - Trims whitespace from the URL so CLI shells pasting trailing
//     spaces don't 400.
//
//   - When the supplied URL is an app root rather than a direct
//     manifest URL, the helper also probes `manifest.json` and
//     `manifest.webmanifest` beneath that URL so the RFC §16 examples
//     (`core pkg wrap --pwa https://app.example.com`) work without the
//     caller knowing the exact manifest path upfront.
func FetchPWAManifest(ctx context.Context, url string) (
	*PWAManifest, error,
) {
	url = core.Trim(url)
	if url == "" {
		return nil, core.E("app.FetchPWAManifest", "empty URL", nil)
	}

	candidates := pwaManifestCandidates(url)
	var lastErr error

	for i, candidate := range candidates {
		body, err := fetchPWAURL(ctx, candidate)
		if err != nil {
			lastErr = err
			// A direct manifest URL should fail loudly; an app URL can
			// fall through to the conventional manifest paths.
			if i == 0 && looksLikePWAManifestURL(url) {
				break
			}
			continue
		}

		manifest, err := decodePWAManifest(body)
		if err == nil {
			return manifest, nil
		}
		lastErr = err

		// The caller already pointed at something that looks like a
		// manifest file; no point probing fallback locations after a
		// hard decode failure.
		if i == 0 && looksLikePWAManifestURL(url) {
			break
		}
	}

	if lastErr == nil {
		lastErr = core.E("app.FetchPWAManifest", "no manifest candidates resolved", nil)
	}
	return nil, core.E("app.FetchPWAManifest", "manifest fetch failed for "+url, lastErr)
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
		Version: config.ViewVersion(version),
		Layout:  "C", // PWA single-surface app — Centre slot only
	}

	// Network permission derived from the resolved app URL, preserving
	// any explicit port and defaulting to the scheme's conventional port.
	if hostPort := hostPortOfURL(url); hostPort != "" {
		m.Permissions.Net = []string{hostPort}
	}

	// Map standard PWA permission strings to CoreApp permission slots.
	applyPWAPermissionMapping(m, src.Permissions)

	// PWA service worker replacement (RFC §16.1.3) — every wrapped PWA
	// gets the `store` service (localStorage / IndexedDB polyfill via
	// go-store) and, when notifications were declared, the `notification`
	// service (push handler routed through core-notify). The store
	// permission is implicitly granted via Config["store"] so the
	// permission gate accepts store.* actions without ViewPermissions
	// growing a typed Store bool.
	services := []any{"store"}
	cfg := m.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg["type"] = PackageTypePWA.String()
	cfg["url"] = url
	cfg["display"] = src.Display
	cfg["short_name"] = src.ShortName
	cfg["store"] = true
	cfg["pwa"] = defaultPWARuntimeConfig(m)
	if m.Permissions.Notifications {
		services = append(services, "notification")
	}
	cfg["services"] = services

	// RFC §16.1 — map PWA `display` to a CoreApp window mode so the host
	// (CoreGUI) knows whether to chrome the window, hide it, or render it
	// fullscreen. Recorded under Config["window_mode"] so the value
	// survives the wrap → install round-trip without forcing a new typed
	// field on ViewManifest.
	//
	//   standalone → window  (chrome-less app window — the W3C default)
	//   fullscreen → kiosk   (no chrome, occupies the whole display)
	//   minimal-ui → window  (small chrome — treated like standalone)
	//   browser    → tab     (open in the user's browser)
	if mode := pwaWindowMode(src.Display); mode != "" {
		cfg["window_mode"] = mode
	}

	if src.ThemeColor != "" || src.BackgroundColor != "" {
		cfg["theme"] = map[string]any{
			"primary":    src.ThemeColor,
			"background": src.BackgroundColor,
		}
	}
	if src.Lang != "" {
		cfg["locale"] = src.Lang
	}
	if icon := largestIcon(src.Icons); icon != "" {
		cfg["icon"] = icon
	}
	m.Config = cfg

	return m
}

// ResolvePWAAppURL turns a caller-supplied PWA source URL into the app
// entry-point URL that should be recorded in the wrapped manifest. When
// the source URL already points at a manifest file, the PWA's
// `start_url` is resolved relative to that file. When the source URL is
// an app root, the same relative-resolution logic yields the final app
// entry point beneath that root.
func ResolvePWAAppURL(sourceURL string, manifest *PWAManifest) string {
	if manifest == nil {
		return core.Trim(sourceURL)
	}
	return resolvePWAStartURL(sourceURL, manifest.StartURL)
}

// FindLocalPWAManifest returns the first recognised PWA manifest file
// beneath `dir`. Local and repo install flows use this so they handle
// both `manifest.json` and `manifest.webmanifest`, matching the HTTP
// fetch path's candidate probing.
//
//	path, ok := app.FindLocalPWAManifest(coreio.Local, "./play")
func FindLocalPWAManifest(medium coreio.Medium, dir string) (string, bool) {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return "", false
	}
	for _, name := range localPWAManifestNames {
		path := core.Path(dir, name)
		if medium.Exists(path) {
			return path, true
		}
	}
	return "", false
}

// pwaWindowMode maps a PWA `display` field to a CoreApp window mode.
// Empty inputs and unknown values return "" so the caller leaves the
// field unset rather than recording a value the host can't act on.
//
//	pwaWindowMode("standalone") // "window"
//	pwaWindowMode("fullscreen") // "kiosk"
//	pwaWindowMode("browser")    // "tab"
//	pwaWindowMode("")           // ""
//
// Rules:
//
//   - standalone → window  (chrome-less app window — RFC §16.1 default)
//
//   - minimal-ui → window  (W3C "minimal browser chrome" — same window
//     mode as standalone for our purposes; the underlying renderer
//     decides whether to draw a thin top bar)
//
//   - fullscreen → kiosk   (no chrome, fills the display)
//
//   - browser    → tab     (the PWA prefers the user's browser tab —
//     CoreGUI surfaces this as a "open externally" hint)
//
//   - anything else        → "" so the host applies its default
func pwaWindowMode(display string) string {
	switch core.Lower(core.Trim(display)) {
	case "standalone", "minimal-ui":
		return "window"
	case "fullscreen":
		return "kiosk"
	case "browser":
		return "tab"
	}
	return ""
}

// WritePWAWrap materialises a wrapped PWA as `<dest>/.core/view.yaml`.
// Convenience helper for CLI wiring and tests; it does nothing that
// Marshal+medium.Write can't do directly.
//
//	err := app.WritePWAWrap(coreio.Local, "/Users/me/.core/apps/play", manifest)
func WritePWAWrap(
	medium coreio.Medium, dest string, manifest *config.ViewManifest,
) error {
	if manifest == nil {
		return core.E("app.WritePWAWrap", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	if err := materializeWrappedRuntimeAssets(medium, dest, manifest); err != nil {
		return core.E("app.WritePWAWrap", "materialise runtime assets failed", err)
	}
	if err := writeWrappedManifest(medium, dest, manifest); err != nil {
		return core.E("app.WritePWAWrap", "write failed", err)
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
			mergeManifestGUIGate(m, "gui.notification.send")
		case "clipboard-read", "clipboard-write":
			m.Permissions.Clipboard = true
			if p == "clipboard-read" {
				mergeManifestGUIGate(m, "gui.clipboard.read")
			} else {
				mergeManifestGUIGate(m, "gui.clipboard.write")
			}
		case "camera":
			m.Permissions.Camera = true
			mergeManifestDeviceGate(m, "device.camera")
		case "microphone":
			m.Permissions.Microphone = true
			mergeManifestDeviceGate(m, "device.microphone")
		case "geolocation":
			// ViewPermissions has no location slot yet — record the
			// RFC-native key and mirror it into the legacy Run list so the
			// runtime gate accepts both persisted and in-memory forms.
			appendUniqueString(&m.Permissions.Run, "device.location")
			mergeManifestDeviceGate(m, "device.location")
		case "storage", "persistent-storage":
			// Local storage is available by default in PWAs under
			// CoreGUI (ts RFC §5). No-op for now; when ViewPermissions
			// grows a Store bool this flips it.
			_ = p
		}
	}
}

// fetchPWAURL performs one HTTP GET and returns the response body.
// Kept separate from FetchPWAManifest so the caller can try multiple
// candidate URLs (root, /manifest.json, /manifest.webmanifest)
// without duplicating the request plumbing.
func fetchPWAURL(ctx context.Context, url string) (
	[]byte, error,
) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, core.E("app.fetchPWAURL", "request build failed", err)
	}
	req.Header.Set("Accept", "application/manifest+json, application/json")

	client := &http.Client{Timeout: pwaFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, core.E("app.fetchPWAURL", "HTTP GET failed", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if closeErr := resp.Body.Close(); closeErr != nil {
			core.Warn("app.fetchPWAURL: response body close failed", "url", url, "err", closeErr)
		}
		return nil, core.E(
			"app.fetchPWAURL",
			"non-2xx status: "+core.Sprint(resp.StatusCode),
			nil,
		)
	}

	body := core.ReadAll(resp.Body)
	if !body.OK {
		cause, _ := body.Value.(error)
		return nil, core.E("app.fetchPWAURL", "read body failed", cause)
	}
	payload, _ := body.Value.(string)
	return []byte(payload), nil
}

// decodePWAManifest narrows a JSON body into the subset of the Web App
// Manifest we care about. At least one identity-bearing field must be
// present so a random HTML page or API response is not misclassified as
// a valid PWA manifest.
func decodePWAManifest(body []byte) (
	*PWAManifest, error,
) {
	var m PWAManifest
	r := core.JSONUnmarshal(body, &m)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, core.E("app.decodePWAManifest", "decode manifest body failed", cause)
	}
	if m.StartURL == "" && m.Name == "" && m.ShortName == "" {
		return nil, core.E("app.decodePWAManifest", "body is not a web app manifest", nil)
	}
	return &m, nil
}

// pwaManifestCandidates returns the URL list FetchPWAManifest should
// try in order. A direct manifest URL is tried as-is; an app URL also
// gets conventional manifest paths appended so callers can hand us the
// site root instead of the exact manifest file path.
func pwaManifestCandidates(raw string) []string {
	if raw == "" {
		return nil
	}
	out := []string{raw}
	if looksLikePWAManifestURL(raw) {
		return out
	}
	for _, name := range []string{"manifest.json", "manifest.webmanifest"} {
		if next, ok := joinManifestURL(raw, name); ok {
			appendUniqueString(&out, next)
		}
	}
	return out
}

// looksLikePWAManifestURL reports whether the URL already names a
// manifest resource rather than an app/document root.
func looksLikePWAManifestURL(raw string) bool {
	parsed, err := neturl.Parse(core.Trim(raw))
	if err != nil {
		return false
	}
	path := core.Lower(parsed.Path)
	return core.HasSuffix(path, ".webmanifest") ||
		core.HasSuffix(path, ".json") ||
		core.Contains(path, "/manifest")
}

// joinManifestURL resolves `name` relative to the supplied app URL,
// treating the URL as a directory-like base when it does not already
// end in `/`.
func joinManifestURL(base, name string) (string, bool) {
	if base == "" || name == "" {
		return "", false
	}
	u, err := neturl.Parse(base)
	if err != nil {
		return "", false
	}
	if u.Path == "" {
		u.Path = "/"
	} else if !core.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	ref, err := neturl.Parse(name)
	if err != nil {
		return "", false
	}
	return u.ResolveReference(ref).String(), true
}

// resolvePWAStartURL turns the caller-supplied target URL and the PWA
// manifest's start_url into the app entry-point URL we record in the
// wrapped CoreApp manifest. Relative start_url values are resolved
// against the caller's URL, whether that URL is the app root or the
// manifest file path itself.
func resolvePWAStartURL(targetURL, startURL string) string {
	targetURL = core.Trim(targetURL)
	startURL = core.Trim(startURL)
	if startURL == "" {
		return targetURL
	}
	if isLocalSource(targetURL) {
		return resolveLocalPWAStartPath(targetURL, startURL)
	}

	ref, err := neturl.Parse(startURL)
	if err == nil && ref.IsAbs() {
		return ref.String()
	}
	if targetURL == "" {
		return startURL
	}
	base, err := neturl.Parse(targetURL)
	if err != nil {
		return startURL
	}
	if ref == nil {
		ref, err = neturl.Parse(startURL)
		if err != nil {
			return startURL
		}
	}
	return base.ResolveReference(ref).String()
}

// resolveLocalPWAStartPath resolves a local manifest path plus a PWA
// start_url into the local entry file beneath the same source tree.
//
//	resolveLocalPWAStartPath("/tmp/app/manifest.json", "/index.html")
func resolveLocalPWAStartPath(targetURL, startURL string) string {
	base := targetURL
	low := core.Lower(base)
	if core.HasSuffix(low, ".json") || core.HasSuffix(low, ".webmanifest") {
		base = core.PathDir(base)
	}
	startURL = core.TrimPrefix(startURL, "./")
	startURL = core.TrimPrefix(startURL, "/")
	if startURL == "" || startURL == "." {
		return base
	}
	return core.Path(base, startURL)
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

// hostPortOfURL returns the host:port portion of an app URL. Explicit
// ports are preserved; otherwise the conventional port for the scheme
// is filled in (`https` → 443, `http` → 80).
//
//	hostPortOfURL("https://app.example.com/manifest.json") // "app.example.com:443"
//	hostPortOfURL("http://localhost:3000/app")            // "localhost:3000"
func hostPortOfURL(raw string) string {
	raw = core.Trim(raw)
	if raw == "" {
		return ""
	}
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		// Bare host/path without a scheme — default to HTTPS to match
		// the RFC examples and the PWA expectation.
		u, err = neturl.Parse("https://" + raw)
		if err != nil {
			return ""
		}
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		switch core.Lower(u.Scheme) {
		case "http":
			port = "80"
		case "https", "":
			port = "443"
		}
	}
	if port == "" {
		return host
	}
	return host + ":" + port
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
