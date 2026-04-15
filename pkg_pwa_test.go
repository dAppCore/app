// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
