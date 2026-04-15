// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// TestPkgPwa_WrapPWA_Good turns a complete PWA manifest into a
// ViewManifest and checks that every mapped field lands in the right
// slot per RFC §16.1.
func TestPkgPwa_WrapPWA_Good(t *testing.T) {
	src := &app.PWAManifest{
		Name:            "Play Example",
		ShortName:       "Play",
		StartURL:        "https://play.example.com/",
		Display:         "standalone",
		ThemeColor:      "#6200ea",
		BackgroundColor: "#ffffff",
		Lang:            "en-GB",
		Icons: []app.PWAIcon{
			{Src: "/icons/192.png", Sizes: "192x192"},
			{Src: "/icons/512.png", Sizes: "512x512"},
		},
		Permissions: []string{"notifications", "clipboard-read", "storage"},
	}

	m := app.WrapPWA(src, app.WrapPWAOptions{TargetURL: "https://play.example.com/"})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}

	// Identity.
	if m.Code != "play" {
		t.Errorf("Code = %q; want %q", m.Code, "play")
	}
	if m.Name != "Play Example" {
		t.Errorf("Name = %q; want %q", m.Name, "Play Example")
	}
	if m.Version == "" {
		t.Error("Version is empty; want default 0.1.0")
	}

	// Net permission derived from the URL host.
	if len(m.Permissions.Net) == 0 || m.Permissions.Net[0] != "play.example.com:443" {
		t.Errorf("Permissions.Net = %v; want [play.example.com:443]", m.Permissions.Net)
	}

	// Mapped ViewPermission booleans.
	if !m.Permissions.Notifications {
		t.Error("Permissions.Notifications = false; want true (notifications perm)")
	}
	if !m.Permissions.Clipboard {
		t.Error("Permissions.Clipboard = false; want true (clipboard-read perm)")
	}

	// Extra fields survive via Config.
	if m.Config["type"] != "pwa" {
		t.Errorf("Config[type] = %v; want pwa", m.Config["type"])
	}
	if m.Config["url"] != "https://play.example.com/" {
		t.Errorf("Config[url] = %v; want the target URL", m.Config["url"])
	}
	if m.Config["icon"] != "/icons/512.png" {
		t.Errorf("Config[icon] = %v; want largest icon", m.Config["icon"])
	}
	theme, ok := m.Config["theme"].(map[string]any)
	if !ok {
		t.Fatalf("Config[theme] not a map; got %T", m.Config["theme"])
	}
	if theme["primary"] != "#6200ea" {
		t.Errorf("theme[primary] = %v; want #6200ea", theme["primary"])
	}

	// Service worker replacement (RFC §16.1.3) — every PWA wrap must
	// declare the `store` service. When notifications are mapped, the
	// `notification` service joins the list to take the push handler.
	storeFlag, ok := m.Config["store"].(bool)
	if !ok || !storeFlag {
		t.Errorf("Config[store] = %v; want true (RFC §16.1 store: true)", m.Config["store"])
	}
	services, ok := m.Config["services"].([]any)
	if !ok {
		t.Fatalf("Config[services] not a []any; got %T", m.Config["services"])
	}
	wantService := func(name string) bool {
		for _, s := range services {
			if v, ok := s.(string); ok && v == name {
				return true
			}
		}
		return false
	}
	if !wantService("store") {
		t.Errorf("Config[services] missing 'store'; got %v", services)
	}
	if !wantService("notification") {
		t.Errorf("Config[services] missing 'notification' (notifications perm declared); got %v", services)
	}

	// RFC §16.1 — `display: standalone` should produce a "window" mode
	// hint so CoreGUI knows to render a chrome-less app window rather
	// than embedding the PWA in a tab.
	if mode, ok := m.Config["window_mode"].(string); !ok || mode != "window" {
		t.Errorf("Config[window_mode] = %v; want 'window' (display=standalone)", m.Config["window_mode"])
	}
}

// TestPkgPwa_WrapPWA_WindowMode covers RFC §16.1's `display → layout`
// table — every defined PWA display value maps to the matching CoreApp
// window mode and unknown values leave the field unset.
func TestPkgPwa_WrapPWA_WindowMode_Good(t *testing.T) {
	cases := []struct {
		display string
		want    string // "" → window_mode key absent
	}{
		{display: "standalone", want: "window"},
		{display: "minimal-ui", want: "window"},
		{display: "fullscreen", want: "kiosk"},
		{display: "browser", want: "tab"},
	}
	for _, tc := range cases {
		t.Run(tc.display, func(t *testing.T) {
			m := app.WrapPWA(&app.PWAManifest{
				Name:     "x",
				StartURL: "https://x.example.com/",
				Display:  tc.display,
			}, app.WrapPWAOptions{})
			if m == nil {
				t.Fatal("WrapPWA returned nil")
			}
			got, _ := m.Config["window_mode"].(string)
			if got != tc.want {
				t.Errorf("display=%q: window_mode = %q; want %q", tc.display, got, tc.want)
			}
		})
	}
}

// TestPkgPwa_WrapPWA_WindowMode_Bad confirms unknown / empty display
// values leave window_mode unset so CoreGUI applies its host default
// rather than recording an unhandlable string.
func TestPkgPwa_WrapPWA_WindowMode_Bad(t *testing.T) {
	cases := []string{"", "  ", "unknown", "kiosk-mode", "popup"}
	for _, display := range cases {
		t.Run(display, func(t *testing.T) {
			m := app.WrapPWA(&app.PWAManifest{
				Name:     "x",
				StartURL: "https://x.example.com/",
				Display:  display,
			}, app.WrapPWAOptions{})
			if m == nil {
				t.Fatal("WrapPWA returned nil")
			}
			if _, ok := m.Config["window_mode"]; ok {
				t.Errorf("display=%q: window_mode key present; want absent", display)
			}
		})
	}
}

// TestPkgPwa_WrapPWA_WindowMode_Ugly confirms case insensitivity and
// surrounding whitespace — PWA manifests in the wild sometimes have
// noisy display values from hand-edited JSON.
func TestPkgPwa_WrapPWA_WindowMode_Ugly(t *testing.T) {
	cases := []struct {
		display string
		want    string
	}{
		{display: "STANDALONE", want: "window"},
		{display: "  Fullscreen  ", want: "kiosk"},
		{display: "Minimal-UI", want: "window"},
	}
	for _, tc := range cases {
		t.Run(tc.display, func(t *testing.T) {
			m := app.WrapPWA(&app.PWAManifest{
				Name:     "x",
				StartURL: "https://x.example.com/",
				Display:  tc.display,
			}, app.WrapPWAOptions{})
			if m == nil {
				t.Fatal("WrapPWA returned nil")
			}
			got, _ := m.Config["window_mode"].(string)
			if got != tc.want {
				t.Errorf("display=%q: window_mode = %q; want %q", tc.display, got, tc.want)
			}
		})
	}
}

// TestPkgPwa_WrapPWA_Bad confirms nil input returns nil without a
// panic, and a PWA with no identifiable name still gets a fallback code.
func TestPkgPwa_WrapPWA_Bad(t *testing.T) {
	if app.WrapPWA(nil, app.WrapPWAOptions{}) != nil {
		t.Error("WrapPWA(nil) returned non-nil")
	}

	src := &app.PWAManifest{} // all-empty manifest
	m := app.WrapPWA(src, app.WrapPWAOptions{})
	if m == nil {
		t.Fatal("WrapPWA returned nil on empty manifest")
	}
	if m.Code != "pwa-app" {
		t.Errorf("Code = %q; want fallback 'pwa-app'", m.Code)
	}
}

// TestPkgPwa_WrapPWA_Services_Bad confirms that a PWA without
// notification permission only declares the `store` service. Push
// handler routing only joins the list when notifications are mapped.
func TestPkgPwa_WrapPWA_Services_Bad(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Quiet",
		StartURL: "https://quiet.example.com/",
		// no Permissions — no notifications
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}
	services, ok := m.Config["services"].([]any)
	if !ok {
		t.Fatalf("Config[services] not a []any; got %T", m.Config["services"])
	}
	if len(services) != 1 || services[0] != "store" {
		t.Errorf("Config[services] = %v; want [store] only (no notifications declared)", services)
	}
}

func TestPkgPwa_WrapPWA_RuntimeConfig_Good(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Offline Play",
		StartURL: "https://play.example.com/app",
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{TargetURL: "https://play.example.com/app"})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}

	pwaCfg, ok := m.Config["pwa"].(map[string]any)
	if !ok {
		t.Fatalf("Config[pwa] = %T; want map[string]any", m.Config["pwa"])
	}
	serviceWorker, ok := pwaCfg["service_worker"].(map[string]any)
	if !ok {
		t.Fatalf("pwa.service_worker = %T; want map[string]any", pwaCfg["service_worker"])
	}
	if serviceWorker["path"] != "./core-sw.js" {
		t.Errorf("service_worker.path = %v; want ./core-sw.js", serviceWorker["path"])
	}

	storeMirror, ok := pwaCfg["store_mirror"].(map[string]any)
	if !ok {
		t.Fatalf("pwa.store_mirror = %T; want map[string]any", pwaCfg["store_mirror"])
	}
	if storeMirror["driver"] != "indexeddb" {
		t.Errorf("store_mirror.driver = %v; want indexeddb", storeMirror["driver"])
	}

	syncCfg, ok := pwaCfg["sync"].(map[string]any)
	if !ok {
		t.Fatalf("pwa.sync = %T; want map[string]any", pwaCfg["sync"])
	}
	if syncCfg["strategy"] != "last-write-wins" {
		t.Errorf("sync.strategy = %v; want last-write-wins", syncCfg["strategy"])
	}

	installPrompt, ok := pwaCfg["install_prompt"].(map[string]any)
	if !ok {
		t.Fatalf("pwa.install_prompt = %T; want map[string]any", pwaCfg["install_prompt"])
	}
	if installPrompt["enabled"] != true {
		t.Errorf("install_prompt.enabled = %v; want true", installPrompt["enabled"])
	}
}

// TestPkgPwa_WrapPWA_PermissionGates_Good confirms wrapped PWAs keep the
// RFC-native action-level permission keys when written back to
// `.core/view.yaml`.
func TestPkgPwa_WrapPWA_PermissionGates_Good(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Sensors",
		StartURL: "https://sensors.example.com/",
		Permissions: []string{
			"notifications",
			"clipboard-read",
			"clipboard-write",
			"camera",
			"microphone",
			"geolocation",
		},
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}

	guiGates, ok := m.Config["gui_gates"].(map[string]any)
	if !ok {
		t.Fatalf("Config[gui_gates] = %T; want map[string]any", m.Config["gui_gates"])
	}
	for _, key := range []string{
		"gui.notification.send",
		"gui.clipboard.read",
		"gui.clipboard.write",
	} {
		if guiGates[key] != true {
			t.Errorf("Config[gui_gates][%q] = %v; want true", key, guiGates[key])
		}
	}

	deviceGates, ok := m.Config["device_gates"].(map[string]any)
	if !ok {
		t.Fatalf("Config[device_gates] = %T; want map[string]any", m.Config["device_gates"])
	}
	for _, key := range []string{
		"device.camera",
		"device.microphone",
		"device.location",
	} {
		if deviceGates[key] != true {
			t.Errorf("Config[device_gates][%q] = %v; want true", key, deviceGates[key])
		}
	}

	dir := t.TempDir()
	if err := app.WritePWAWrap(coreio.Local, dir, m); err != nil {
		t.Fatalf("WritePWAWrap: %v", err)
	}
	body, err := coreio.Local.Read(core.Path(dir, ".core", "view.yaml"))
	if err != nil {
		t.Fatalf("Read view.yaml: %v", err)
	}
	out := body
	for _, want := range []string{
		"gui.notification.send: true",
		"gui.clipboard.read: true",
		"gui.clipboard.write: true",
		"device.camera: true",
		"device.microphone: true",
		"device.location: true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wrapped PWA YAML missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "- device.location") {
		t.Errorf("wrapped PWA YAML should not leak device.location through permissions.run:\n%s", out)
	}
}

// TestPkgPwa_WrapPWA_Ugly handles the edge cases: target URL override,
// explicit code override, and unusual characters in the name.
func TestPkgPwa_WrapPWA_Ugly(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Mötörhead!",
		StartURL: "https://default.example.com/",
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{
		TargetURL: "https://override.example.com/",
		Code:      "motorhead",
		Version:   "2.0.0",
	})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}
	if m.Code != "motorhead" {
		t.Errorf("Code = %q; want explicit override", m.Code)
	}
	if m.Version != "2.0.0" {
		t.Errorf("Version = %q; want explicit override", m.Version)
	}
	if len(m.Permissions.Net) == 0 || m.Permissions.Net[0] != "override.example.com:443" {
		t.Errorf("Permissions.Net = %v; want override host", m.Permissions.Net)
	}
}

// TestPkgPwa_WrapPWA_StartURLResolution_Good confirms the wrapper
// resolves a relative start_url against the caller-supplied URL and
// preserves an explicit non-default port in the derived net
// permission.
func TestPkgPwa_WrapPWA_StartURLResolution_Good(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Port App",
		StartURL: "/app",
	}
	resolved := app.ResolvePWAAppURL("http://localhost:3000/manifest.json", src)
	if resolved != "http://localhost:3000/app" {
		t.Fatalf("ResolvePWAAppURL = %q; want http://localhost:3000/app", resolved)
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{TargetURL: resolved})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}
	if got := m.Config["url"]; got != resolved {
		t.Errorf("Config[url] = %v; want http://localhost:3000/app", got)
	}
	if len(m.Permissions.Net) != 1 || m.Permissions.Net[0] != "localhost:3000" {
		t.Errorf("Permissions.Net = %v; want [localhost:3000]", m.Permissions.Net)
	}
}

// TestPkgPwa_WrapPWA_StartURLResolution_Local confirms that local PWA
// sources resolve `start_url` inside the same source tree rather than
// treating `/` as the filesystem root.
func TestPkgPwa_WrapPWA_StartURLResolution_Local(t *testing.T) {
	src := &app.PWAManifest{
		Name:     "Local Port App",
		StartURL: "/next/index.html",
	}
	resolved := app.ResolvePWAAppURL("/tmp/local-app/manifest.json", src)
	if resolved != "/tmp/local-app/next/index.html" {
		t.Fatalf("ResolvePWAAppURL(local) = %q; want /tmp/local-app/next/index.html", resolved)
	}
	m := app.WrapPWA(src, app.WrapPWAOptions{TargetURL: resolved})
	if m == nil {
		t.Fatal("WrapPWA returned nil")
	}
	if got := m.Config["url"]; got != resolved {
		t.Errorf("Config[url] = %v; want %q", got, resolved)
	}
	if len(m.Permissions.Net) != 0 {
		t.Errorf("Permissions.Net = %v; want no network declaration for local source", m.Permissions.Net)
	}
}

// TestPkgPwa_WritePWAWrap_Good confirms the wrapped manifest lands on
// disk at `<dest>/.core/view.yaml` and parses back cleanly.
func TestPkgPwa_WritePWAWrap_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	src := &app.PWAManifest{
		Name:     "Play",
		StartURL: "https://play.example.com/",
	}
	manifest := app.WrapPWA(src, app.WrapPWAOptions{})

	if err := app.WritePWAWrap(medium, dir, manifest); err != nil {
		t.Fatalf("WritePWAWrap: %v", err)
	}
	path := core.Path(dir, ".core", "view.yaml")
	if !medium.Exists(path) {
		t.Fatalf("view.yaml not written at %s", path)
	}

	// Round-trip parse.
	var round config.ViewManifest
	if err := config.LoadManifest(medium, path, &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if round.Code != manifest.Code {
		t.Errorf("round-trip Code = %q; want %q", round.Code, manifest.Code)
	}
}

func TestPkgPwa_WritePWAWrap_RuntimeAssets_Good(t *testing.T) {
	dir := t.TempDir()
	manifest := app.WrapPWA(&app.PWAManifest{
		Name:     "Offline Ready",
		StartURL: "https://play.example.com/",
	}, app.WrapPWAOptions{})
	if manifest == nil {
		t.Fatal("WrapPWA returned nil")
	}

	if err := app.WritePWAWrap(coreio.Local, dir, manifest); err != nil {
		t.Fatalf("WritePWAWrap: %v", err)
	}
	for _, tc := range []struct {
		path  string
		parts []string
	}{
		{path: core.Path(dir, "core-sw.js"), parts: []string{"core.json", "components"}},
		{path: core.Path(dir, "core-pwa.js"), parts: []string{"beforeinstallprompt", "indexedDB", "last-write-wins"}},
	} {
		body, err := coreio.Local.Read(tc.path)
		if err != nil {
			t.Fatalf("Read %s: %v", tc.path, err)
		}
		for _, part := range tc.parts {
			if !strings.Contains(body, part) {
				t.Errorf("%s missing %q", tc.path, part)
			}
		}
	}
}

func TestPkgPwa_WriteWrappedAppWithOptions_InjectsBootstrap_Good(t *testing.T) {
	srcDir := t.TempDir()
	if err := coreio.Local.Write(core.Path(srcDir, "index.html"), "<html><head><title>X</title></head><body>Hello</body></html>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	manifest := app.WrapPWA(&app.PWAManifest{
		Name:     "Injected",
		StartURL: "https://play.example.com/index.html",
	}, app.WrapPWAOptions{})
	if manifest == nil {
		t.Fatal("WrapPWA returned nil")
	}

	dest := t.TempDir()
	if err := app.WriteWrappedAppWithOptions(coreio.Local, dest, manifest, app.WriteWrappedOptions{
		AssetSource: srcDir,
	}); err != nil {
		t.Fatalf("WriteWrappedAppWithOptions: %v", err)
	}

	body, err := coreio.Local.Read(core.Path(dest, "index.html"))
	if err != nil {
		t.Fatalf("Read injected index.html: %v", err)
	}
	if !strings.Contains(body, `data-core-pwa`) {
		t.Errorf("index.html missing bootstrap injection:\n%s", body)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(coreio.Local, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadViewManifest: %v", err)
	}
	if hash, _ := round.Config["asset_hash"].(string); hash == "" {
		t.Fatal("asset_hash missing from wrapped PWA written through the shared writer")
	}
}

// TestPkgPwa_WritePWAWrap_Bad catches the nil-manifest guard.
func TestPkgPwa_WritePWAWrap_Bad(t *testing.T) {
	err := app.WritePWAWrap(coreio.Local, t.TempDir(), nil)
	if err == nil {
		t.Fatal("WritePWAWrap(nil) returned no error")
	}
}

// TestPkgPwa_FetchPWAManifest_Good spins up a mini HTTP server that
// serves a manifest.json and confirms the fetch returns the decoded
// struct.
func TestPkgPwa_FetchPWAManifest_Good(t *testing.T) {
	body := `{
		"name": "Play",
		"short_name": "play",
		"start_url": "/",
		"theme_color": "#111111",
		"icons": [{"src":"/icon.png","sizes":"512x512"}]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m, err := app.FetchPWAManifest(context.Background(), srv.URL+"/manifest.json")
	if err != nil {
		t.Fatalf("FetchPWAManifest: %v", err)
	}
	if m.Name != "Play" {
		t.Errorf("Name = %q; want 'Play'", m.Name)
	}
	if m.ThemeColor != "#111111" {
		t.Errorf("ThemeColor = %q; want '#111111'", m.ThemeColor)
	}
}

// TestPkgPwa_FetchPWAManifest_RootURL_Good confirms the fetch path also
// accepts an app URL and falls back to the conventional manifest.json
// path beneath it.
func TestPkgPwa_FetchPWAManifest_RootURL_Good(t *testing.T) {
	body := `{"name":"Root Play","short_name":"root-play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><head><title>Play</title></head></html>"))
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/manifest+json")
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m, err := app.FetchPWAManifest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchPWAManifest(root URL): %v", err)
	}
	if m.Name != "Root Play" {
		t.Errorf("Name = %q; want Root Play", m.Name)
	}
}

// TestPkgPwa_FetchPWAManifest_Bad rejects empty URLs and non-2xx
// responses.
func TestPkgPwa_FetchPWAManifest_Bad(t *testing.T) {
	if _, err := app.FetchPWAManifest(context.Background(), ""); err == nil {
		t.Error("FetchPWAManifest(\"\") returned no error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := app.FetchPWAManifest(context.Background(), srv.URL+"/manifest.json"); err == nil {
		t.Error("404 response produced no error")
	}
}

// TestPkgPwa_FetchPWAManifest_Ugly handles a successful 200 with body
// that cannot be decoded — should fail with a decode error, not a
// partial struct.
func TestPkgPwa_FetchPWAManifest_Ugly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()
	if _, err := app.FetchPWAManifest(context.Background(), srv.URL); err == nil {
		t.Error("non-JSON body produced no error")
	}
}
