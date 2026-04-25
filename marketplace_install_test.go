// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
)

// TestMarketplaceInstall_MarketplaceInstall_Good resolves a PWA listing
// against a locally-planted marketplace and a mock manifest server,
// then asserts the install lands at the expected path with `source` set.
func TestMarketplaceInstall_MarketplaceInstall_Good(t *testing.T) {
	manifestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Play","short_name":"play","start_url":"/"}`))
	}))
	defer manifestSrv.Close()

	root := t.TempDir()
	medium := coreio.Local

	// Plant a marketplace index pointing at the mock PWA.
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["media"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	if err := medium.EnsureDir(core.Path(root, "media")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := `{
		"version":1,
		"category":"media",
		"entries":[{"code":"play","type":"pwa","url":"` + manifestSrv.URL + `/manifest.json"}]
	}`
	if err := medium.Write(core.Path(root, "media", app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write category: %v", err)
	}

	home := t.TempDir()
	c := core.New()
	dest, err := app.MarketplaceInstall(context.Background(), c, app.MarketplaceInstallOptions{
		Root: root,
		Home: home,
		Code: "play",
	})
	if err != nil {
		t.Fatalf("MarketplaceInstall: %v", err)
	}
	if dest == "" {
		t.Fatal("MarketplaceInstall returned empty dest")
	}

	// view.yaml should be installed.
	viewPath := core.Path(dest, ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		t.Fatalf("view.yaml missing at %s", viewPath)
	}
	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, viewPath, &round); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if round.Code != "play" {
		t.Errorf("Code = %q; want play", round.Code)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:play" {
		t.Errorf("source = %q; want marketplace:play", src)
	}
}

// TestMarketplaceInstall_MarketplaceInstall_Bad rejects nil core, empty
// home, missing listing.
func TestMarketplaceInstall_MarketplaceInstall_Bad(t *testing.T) {
	if _, err := app.MarketplaceInstall(context.Background(), nil, app.MarketplaceInstallOptions{}); err == nil {
		t.Error("nil core produced no error")
	}

	root := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), `{"version":1,"categories":[]}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	c := core.New()
	if _, err := app.MarketplaceInstall(context.Background(), c, app.MarketplaceInstallOptions{
		Root: root,
		Home: t.TempDir(),
		Code: "missing",
	}); err == nil {
		t.Error("missing listing produced no error")
	}
}

// TestMarketplaceInstall_MarketplaceFetch_Bad confirms required-arg
// validation — actually invoking `git clone` would talk to the network
// so we only assert the input guards here.
func TestMarketplaceInstall_MarketplaceFetch_Bad(t *testing.T) {
	if err := app.MarketplaceFetch(context.Background(), nil, app.MarketplaceFetchOptions{}); err == nil {
		t.Error("nil core produced no error")
	}
	c := core.New()
	if err := app.MarketplaceFetch(context.Background(), c, app.MarketplaceFetchOptions{}); err == nil {
		t.Error("empty URL produced no error")
	}
	if err := app.MarketplaceFetch(context.Background(), c, app.MarketplaceFetchOptions{
		URL: "https://example.com/x.git",
	}); err == nil {
		t.Error("empty Dir produced no error")
	}
}

// TestMarketplaceInstall_MarketplaceInstall_ElectronUnreachable confirms
// that an Electron listing whose GitHub release is unreachable surfaces
// a typed error rather than falling through to a plain git clone. The
// RFC §16.2 pipeline runs fetch → extract → scan → wrap; a 404 on the
// release means the install cannot complete honestly.
func TestMarketplaceInstall_MarketplaceInstall_ElectronUnreachable(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["tools"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	if err := medium.EnsureDir(core.Path(root, "tools")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := `{
		"version":1,
		"category":"tools",
		"entries":[{"code":"ghost-electron","type":"electron","repo":"github.com/example/ghost-electron"}]
	}`
	if err := medium.Write(core.Path(root, "tools", app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write category: %v", err)
	}

	c := core.New()
	if _, err := app.MarketplaceInstall(context.Background(), c, app.MarketplaceInstallOptions{
		Root: root,
		Home: t.TempDir(),
		Code: "ghost-electron",
	}); err == nil {
		t.Error("Electron listing with unreachable release produced no error")
	}
}

// TestMarketplaceInstall_MarketplaceInstall_ElectronSignatureSkip
// — the Electron install path pins the `sign_key` field on the
// listing but does NOT verify the wrapped manifest against it
// (wraps are unsigned by construction). The listing's `sign_key`
// applies only to native marketplace clones whose upstream manifest
// carries a signature. This test pins that contract so a future
// refactor does not start rejecting Electron wraps as unsigned.
func TestMarketplaceInstall_MarketplaceInstall_ElectronSignatureSkip(t *testing.T) {
	// Can't install an Electron listing without the release pipeline
	// succeeding — but we CAN assert the install fails at the fetch
	// rather than at a post-install signature verification step. When
	// the fetch fails the error message should mention the release,
	// not a signature mismatch.
	root := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["tools"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	if err := medium.EnsureDir(core.Path(root, "tools")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := `{
		"version":1,
		"category":"tools",
		"entries":[{
			"code":"pinned-electron",
			"type":"electron",
			"repo":"github.com/example/pinned-electron",
			"sign_key":"deadbeef"
		}]
	}`
	if err := medium.Write(core.Path(root, "tools", app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write category: %v", err)
	}

	c := core.New()
	_, err := app.MarketplaceInstall(context.Background(), c, app.MarketplaceInstallOptions{
		Root: root,
		Home: t.TempDir(),
		Code: "pinned-electron",
	})
	if err == nil {
		t.Fatal("Electron install with unreachable release produced no error")
	}
	// The error must come from the release fetch, not from a post-install
	// signature verification step that should not run for Electron wraps.
	if msg := err.Error(); !contains(msg, "release") && !contains(msg, "FetchElectronRelease") {
		t.Errorf("error does not mention release fetch: %v", err)
	}
}

// contains is a small local helper so the test file avoids importing
// strings (`core.Contains` would also work but keeping the helper
// local matches the other substring checks in this file).
//
//	contains("hello world", "world") // true
func contains(s, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
