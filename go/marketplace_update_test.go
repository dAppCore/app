// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// TestMarketplaceUpdate_MarketplaceUpdate_Good — refetches a PWA
// manifest from the recorded URL and rewrites the install in place.
// The server's body changes between fetches so the refresh is
// observable in the rendered manifest.
func TestMarketplaceUpdate_MarketplaceUpdate_Good(t *testing.T) {
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	root := writeTestMarketplaceWithPWA(t, "play", srv.URL+"/manifest.json")
	home := t.TempDir()
	c := core.New()

	// Initial install so MarketplaceUpdate has something to refresh.
	if _, err := app.MarketplaceInstall(context.Background(), c, app.MarketplaceInstallOptions{
		Root: root, Home: home, Code: "play",
	}); err != nil {
		t.Fatalf("initial install: %v", err)
	}

	// Mutate the upstream manifest body so the rewrap is observable.
	body = `{"name":"Play 2","short_name":"play","start_url":"/v2"}`

	dest, err := app.MarketplaceUpdate(context.Background(), c, app.MarketplaceUpdateOptions{
		Root: root, Home: home, Code: "play",
	})
	if err != nil {
		t.Fatalf("MarketplaceUpdate: %v", err)
	}
	if dest == "" {
		t.Fatal("MarketplaceUpdate returned empty dest")
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(coreio.Local, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	if round.Name != "Play 2" {
		t.Errorf("Name = %q; want 'Play 2' (update did not refetch)", round.Name)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:play" {
		t.Errorf("source stamp lost: %q", src)
	}
}

// TestMarketplaceUpdate_MarketplaceUpdate_Bad — required-arg validation.
// Unresolved listings, missing installs, and nil core all fail with a
// typed error.
func TestMarketplaceUpdate_MarketplaceUpdate_Bad(t *testing.T) {
	if _, err := app.MarketplaceUpdate(context.Background(), nil, app.MarketplaceUpdateOptions{}); err == nil {
		t.Error("nil core produced no error")
	}
	c := core.New()
	if _, err := app.MarketplaceUpdate(context.Background(), c, app.MarketplaceUpdateOptions{}); err == nil {
		t.Error("empty code produced no error")
	}
	if _, err := app.MarketplaceUpdate(context.Background(), c, app.MarketplaceUpdateOptions{
		Code: "missing", Home: t.TempDir(),
	}); err == nil {
		t.Error("missing install produced no error")
	}

	// Marketplace cache exists but has no matching listing.
	root := writeTestMarketplace(t)
	home := t.TempDir()
	dest := core.Path(home, ".core", "apps", "ghost")
	if err := coreio.Local.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(core.Path(dest, ".core", "view.yaml"),
		"code: ghost\nname: Ghost\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := app.MarketplaceUpdate(context.Background(), c, app.MarketplaceUpdateOptions{
		Root: root, Home: home, Code: "ghost",
	}); err == nil {
		t.Error("unresolved listing produced no error")
	}
}

// TestMarketplaceUpdate_MarketplaceUpdate_Ugly — an Electron listing
// whose upstream release is unreachable surfaces a typed error rather
// than silently succeeding. The RFC §6.3 pipeline refreshes the
// renderer assets from the upstream GitHub release — a 404 there
// must fail the update so the operator is not left with a stale
// install that the pipeline thinks is fresh.
func TestMarketplaceUpdate_MarketplaceUpdate_Ugly(t *testing.T) {
	root := writeTestMarketplaceWithElectron(t, "ghost-electron")
	home := t.TempDir()
	medium := coreio.Local

	// Plant a fake install so the dest exists.
	dest := core.Path(home, ".core", "apps", "ghost-electron")
	if err := medium.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dest, ".core", "view.yaml"),
		"code: ghost-electron\nname: Ghost\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	c := core.New()
	if _, err := app.MarketplaceUpdate(context.Background(), c, app.MarketplaceUpdateOptions{
		Root: root, Home: home, Code: "ghost-electron",
	}); err == nil {
		t.Error("unreachable Electron release should surface a typed error")
	}
}

// TestMarketplaceUpdate_MarketplaceInstalled_Good — alias for PkgList,
// returns every installed package as a slice of PkgEntry.
func TestMarketplaceUpdate_MarketplaceInstalled_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	for _, code := range []string{"alpha", "beta"} {
		dest := core.Path(home, ".core", "apps", code, ".core")
		if err := medium.EnsureDir(dest); err != nil {
			t.Fatalf("EnsureDir %s: %v", code, err)
		}
		if err := medium.Write(core.Path(dest, "view.yaml"),
			"code: "+code+"\nname: "+code+"\nversion: 0.1.0\n"); err != nil {
			t.Fatalf("Write %s: %v", code, err)
		}
	}

	entries, err := app.MarketplaceInstalled(medium, home)
	if err != nil {
		t.Fatalf("MarketplaceInstalled: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d; want 2", len(entries))
	}
}

// TestMarketplaceUpdate_MarketplaceInstalled_Bad — empty home is rejected.
func TestMarketplaceUpdate_MarketplaceInstalled_Bad(t *testing.T) {
	if _, err := app.MarketplaceInstalled(coreio.Local, ""); err == nil {
		t.Error("empty home produced no error")
	}
}

// TestMarketplaceUpdate_MarketplaceInstalled_Ugly — fresh install (no
// apps directory) returns nil, nil.
func TestMarketplaceUpdate_MarketplaceInstalled_Ugly(t *testing.T) {
	entries, err := app.MarketplaceInstalled(coreio.Local, t.TempDir())
	if err != nil {
		t.Fatalf("MarketplaceInstalled: %v", err)
	}
	if entries != nil {
		t.Errorf("entries = %v; want nil", entries)
	}
}

// writeTestMarketplaceWithPWA plants a marketplace tree containing a
// single PWA listing pointing at the supplied URL. Returns the root.
//
//	root := writeTestMarketplaceWithPWA(t, "play", "https://app.example.com/manifest.json")
func writeTestMarketplaceWithPWA(t *testing.T, code, url string) string {
	t.Helper()
	root := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["media"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	if err := medium.EnsureDir(core.Path(root, "media")); err != nil {
		t.Fatalf("EnsureDir media: %v", err)
	}
	body := `{
		"version":1,
		"category":"media",
		"entries":[{"code":"` + code + `","type":"pwa","url":"` + url + `"}]
	}`
	if err := medium.Write(core.Path(root, "media", app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write category: %v", err)
	}
	return root
}

// writeTestMarketplaceWithElectron plants a marketplace tree with one
// electron listing — used to confirm the update path returns the
// install dir without erroring on non-native types.
//
//	root := writeTestMarketplaceWithElectron(t, "ghost-electron")
func writeTestMarketplaceWithElectron(t *testing.T, code string) string {
	t.Helper()
	root := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["tools"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	if err := medium.EnsureDir(core.Path(root, "tools")); err != nil {
		t.Fatalf("EnsureDir tools: %v", err)
	}
	body := `{
		"version":1,
		"category":"tools",
		"entries":[{"code":"` + code + `","type":"electron","repo":"github.com/example/` + code + `"}]
	}`
	if err := medium.Write(core.Path(root, "tools", app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write tools: %v", err)
	}
	return root
}
