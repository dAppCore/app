// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	"gopkg.in/yaml.v3"
)

// TestMarketplace_LoadMarketplaceIndex_Good reads a well-formed
// top-level index.json and asserts the parsed shape matches.
func TestMarketplace_LoadMarketplaceIndex_Good(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local

	body := `{"version": 1, "categories": ["media", "tools"]}`
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	idx, err := app.LoadMarketplaceIndex(medium, root)
	if err != nil {
		t.Fatalf("LoadMarketplaceIndex: %v", err)
	}
	if idx.Version != 1 {
		t.Errorf("Version = %d; want 1", idx.Version)
	}
	if len(idx.Categories) != 2 {
		t.Errorf("Categories = %v", idx.Categories)
	}
}

// TestMarketplace_LoadMarketplaceIndex_Bad rejects empty root and
// missing file.
func TestMarketplace_LoadMarketplaceIndex_Bad(t *testing.T) {
	if _, err := app.LoadMarketplaceIndex(coreio.Local, ""); err == nil {
		t.Error("empty root produced no error")
	}
	if _, err := app.LoadMarketplaceIndex(coreio.Local, t.TempDir()); err == nil {
		t.Error("missing index produced no error")
	}
}

// TestMarketplace_LoadMarketplaceIndex_Ugly rejects malformed JSON.
func TestMarketplace_LoadMarketplaceIndex_Ugly(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), "{not json"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := app.LoadMarketplaceIndex(medium, root); err == nil {
		t.Error("malformed JSON produced no error")
	}
}

// TestMarketplace_LoadMarketplaceCategory_Good reads a category-level
// index with two listings and confirms both are parsed.
func TestMarketplace_LoadMarketplaceCategory_Good(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local

	cat := core.Path(root, "media")
	if err := medium.EnsureDir(cat); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := `{
		"version": 1,
		"category": "media",
		"entries": [
			{"code":"photo-browser","type":"native","repo":"https://example.com/pb.git"},
			{"code":"play","type":"pwa","url":"https://play.example.com"}
		]
	}`
	if err := medium.Write(core.Path(cat, app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	c, err := app.LoadMarketplaceCategory(medium, root, "media")
	if err != nil {
		t.Fatalf("LoadMarketplaceCategory: %v", err)
	}
	if len(c.Entries) != 2 {
		t.Fatalf("len(Entries) = %d; want 2", len(c.Entries))
	}
	if c.Entries[0].Code != "photo-browser" {
		t.Errorf("Entries[0].Code = %q", c.Entries[0].Code)
	}
	if c.Entries[1].Type != "pwa" {
		t.Errorf("Entries[1].Type = %q", c.Entries[1].Type)
	}
}

// TestMarketplace_LoadMarketplaceCategory_Bad rejects empty inputs and
// missing category file.
func TestMarketplace_LoadMarketplaceCategory_Bad(t *testing.T) {
	if _, err := app.LoadMarketplaceCategory(coreio.Local, "", "media"); err == nil {
		t.Error("empty root produced no error")
	}
	if _, err := app.LoadMarketplaceCategory(coreio.Local, t.TempDir(), ""); err == nil {
		t.Error("empty category produced no error")
	}
	if _, err := app.LoadMarketplaceCategory(coreio.Local, t.TempDir(), "missing"); err == nil {
		t.Error("missing category produced no error")
	}
}

// TestMarketplace_MarketplaceSearch_Good builds a two-category index
// and confirms a substring search returns the expected listings.
func TestMarketplace_MarketplaceSearch_Good(t *testing.T) {
	root := writeTestMarketplace(t)

	results, err := app.MarketplaceSearch(coreio.Local, root, "photo")
	if err != nil {
		t.Fatalf("MarketplaceSearch: %v", err)
	}
	if len(results) != 1 || results[0].Code != "photo-browser" {
		t.Errorf("search 'photo' = %+v; want [photo-browser]", results)
	}

	// Empty needle → every listing.
	all, err := app.MarketplaceSearch(coreio.Local, root, "")
	if err != nil {
		t.Fatalf("MarketplaceSearch (empty): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("empty search = %d results; want 3", len(all))
	}
}

// TestMarketplace_MarketplaceSearch_Bad propagates a missing index
// error from LoadMarketplaceIndex.
func TestMarketplace_MarketplaceSearch_Bad(t *testing.T) {
	if _, err := app.MarketplaceSearch(coreio.Local, t.TempDir(), "x"); err == nil {
		t.Error("missing index produced no error")
	}
}

// TestMarketplace_MarketplaceSearch_Ugly covers case-insensitive match
// and field coverage (name, description).
func TestMarketplace_MarketplaceSearch_Ugly(t *testing.T) {
	root := writeTestMarketplace(t)

	// Uppercase needle matches lowercase code.
	results, err := app.MarketplaceSearch(coreio.Local, root, "PLAY")
	if err != nil {
		t.Fatalf("MarketplaceSearch: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("search 'PLAY' = %d results; want 1", len(results))
	}

	// Description match.
	results, err = app.MarketplaceSearch(coreio.Local, root, "browser")
	if err != nil {
		t.Fatalf("MarketplaceSearch: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("search 'browser' = %d results; want >=1", len(results))
	}
}

// TestMarketplace_MarketplaceResolve_Good resolves an exact code match.
func TestMarketplace_MarketplaceResolve_Good(t *testing.T) {
	root := writeTestMarketplace(t)
	listing, err := app.MarketplaceResolve(coreio.Local, root, "play")
	if err != nil {
		t.Fatalf("MarketplaceResolve: %v", err)
	}
	if listing.Code != "play" {
		t.Errorf("Code = %q; want 'play'", listing.Code)
	}
	if listing.Type != "pwa" {
		t.Errorf("Type = %q; want 'pwa'", listing.Type)
	}
}

// TestMarketplace_MarketplaceResolve_Bad rejects empty code and
// unresolved lookups.
func TestMarketplace_MarketplaceResolve_Bad(t *testing.T) {
	root := writeTestMarketplace(t)
	if _, err := app.MarketplaceResolve(coreio.Local, root, ""); err == nil {
		t.Error("empty code produced no error")
	}
	if _, err := app.MarketplaceResolve(coreio.Local, root, "missing"); err == nil {
		t.Error("missing code produced no error")
	}
}

// writeTestMarketplace plants a two-category marketplace tree with a
// mix of listings. Returns the root path.
//
//	root := writeTestMarketplace(t)
func writeTestMarketplace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	medium := coreio.Local

	idxBody := `{"version":1, "categories":["media","tools"]}`
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), idxBody); err != nil {
		t.Fatalf("Write index: %v", err)
	}

	mediaBody := `{
		"version":1,
		"category":"media",
		"entries":[
			{"code":"photo-browser","type":"native","description":"A local photo browser","repo":"https://example.com/pb.git"},
			{"code":"play","type":"pwa","description":"Play for Lethean","url":"https://play.example.com"}
		]
	}`
	mediaDir := core.Path(root, "media")
	if err := medium.EnsureDir(mediaDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(mediaDir, app.MarketplaceIndexFileName), mediaBody); err != nil {
		t.Fatalf("Write media index: %v", err)
	}

	toolsBody := `{
		"version":1,
		"category":"tools",
		"entries":[
			{"code":"editor","type":"native","description":"A text editor","repo":"https://example.com/ed.git"}
		]
	}`
	toolsDir := core.Path(root, "tools")
	if err := medium.EnsureDir(toolsDir); err != nil {
		t.Fatalf("EnsureDir tools: %v", err)
	}
	if err := medium.Write(core.Path(toolsDir, app.MarketplaceIndexFileName), toolsBody); err != nil {
		t.Fatalf("Write tools index: %v", err)
	}
	return root
}

// TestMarketplace_ExtractErr_Good — a Result wrapping an error returns
// that error verbatim. Used by the marketplace.go process invocations
// to surface git failures with the original message intact.
func TestMarketplace_ExtractErr_Good(t *testing.T) {
	want := core.NewError("boom")
	got := app.ExtractErrForTest(core.Result{Value: want})
	if got != want {
		t.Errorf("ExtractErr returned %v; want %v", got, want)
	}
}

// TestMarketplace_ExtractErr_Bad — an OK Result returns nil so
// callers do not fabricate phantom errors when the process succeeded.
func TestMarketplace_ExtractErr_Bad(t *testing.T) {
	if err := app.ExtractErrForTest(core.Result{OK: true, Value: "ok"}); err != nil {
		t.Errorf("ExtractErr on OK Result returned %v; want nil", err)
	}
}

// TestMarketplace_ExtractErr_Ugly — a Result with a non-error Value
// (e.g. a string from a CLI process exit message) is wrapped in a
// typed error so callers always have an `error` to bubble up. The
// wrapped string survives in the new error's message so a developer
// can still see the original payload.
func TestMarketplace_ExtractErr_Ugly(t *testing.T) {
	got := app.ExtractErrForTest(core.Result{Value: "git: command not found"})
	if got == nil {
		t.Fatal("ExtractErr returned nil for non-OK Result with string value")
	}
	if !core.Contains(got.Error(), "git: command not found") {
		t.Errorf("wrapped error message %q does not contain the original payload", got.Error())
	}
}

// TestMarketplace_VerifyListingAfterInstall_Good — opt-in skip path
// returns nil without touching disk. Used by tests and CI runs that
// already pinned the signature elsewhere.
func TestMarketplace_VerifyListingAfterInstall_Good(t *testing.T) {
	dest := t.TempDir()
	listing := &app.MarketplaceListing{Code: "photo"}
	err := app.VerifyListingAfterInstallForTest(coreio.Local, dest, listing,
		app.MarketplaceInstallOptions{SkipVerify: true})
	if err != nil {
		t.Errorf("SkipVerify path returned %v; want nil", err)
	}
}

// TestMarketplace_VerifyListingAfterInstall_Bad — verify path runs
// against a real install with a valid signature and a paired key in
// the listing. The signature round-trips and verify succeeds.
func TestMarketplace_VerifyListingAfterInstall_Bad(t *testing.T) {
	medium := coreio.Local
	pub, priv, _ := ed25519.GenerateKey(nil)

	dest := t.TempDir()
	manifest := &config.ViewManifest{
		Code:    "verify-good",
		Name:    "Verify Good",
		Version: "0.1.0",
	}
	if err := app.SignManifestForTest(manifest, priv); err != nil {
		t.Fatalf("SignManifestForTest: %v", err)
	}
	out, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := medium.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dest, ".core", "view.yaml"), string(out)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listing := &app.MarketplaceListing{
		Code:    manifest.Code,
		SignKey: hex.EncodeToString(pub),
	}
	if err := app.VerifyListingAfterInstallForTest(medium, dest, listing,
		app.MarketplaceInstallOptions{}); err != nil {
		t.Errorf("verify with paired key returned %v; want nil", err)
	}
}

// TestMarketplace_VerifyListingAfterInstall_Ugly — verify failure
// rolls back the install. The destination directory is removed so the
// next install attempt starts clean.
func TestMarketplace_VerifyListingAfterInstall_Ugly(t *testing.T) {
	medium := coreio.Local
	pub, _, _ := ed25519.GenerateKey(nil) // declared, never used to sign
	_, otherPriv, _ := ed25519.GenerateKey(nil)

	dest := t.TempDir()
	manifest := &config.ViewManifest{
		Code:    "verify-rollback",
		Name:    "Rollback",
		Version: "0.1.0",
	}
	// Sign with a key that does NOT match the listing's SignKey.
	if err := app.SignManifestForTest(manifest, otherPriv); err != nil {
		t.Fatalf("SignManifestForTest: %v", err)
	}
	out, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := medium.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dest, ".core", "view.yaml"), string(out)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listing := &app.MarketplaceListing{
		Code:    manifest.Code,
		SignKey: hex.EncodeToString(pub), // wrong key for the signature
	}
	if err := app.VerifyListingAfterInstallForTest(medium, dest, listing,
		app.MarketplaceInstallOptions{}); err == nil {
		t.Error("verify with wrong key produced no error")
	}
	// Rollback ran — destination should be gone.
	if medium.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Error("verify failure did not roll back the install")
	}
}

// TestMarketplace_StampSource_Good plants the source field in an
// installed manifest and confirms a re-read sees it.
func TestMarketplace_StampSource_Good(t *testing.T) {
	medium := coreio.Local
	dest := t.TempDir()
	if err := medium.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dest, ".core", "view.yaml"),
		"code: stamp-good\nname: Stamp Good\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := app.StampSourceForTest(medium, dest, "marketplace:photo-browser"); err != nil {
		t.Fatalf("StampSource: %v", err)
	}
	var round config.ViewManifest
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:photo-browser" {
		t.Errorf("Config[source] = %q; want %q", src, "marketplace:photo-browser")
	}
}

// TestMarketplace_StampSource_Bad — a missing manifest is a no-op
// (returns nil) so a partial install does not error out the stamp.
func TestMarketplace_StampSource_Bad(t *testing.T) {
	medium := coreio.Local
	dest := t.TempDir()
	if err := app.StampSourceForTest(medium, dest, "marketplace:none"); err != nil {
		t.Errorf("StampSource on missing manifest returned %v; want nil", err)
	}
}

// TestMarketplace_StampSource_Ugly — stamping over an existing source
// overwrites it (last-write-wins, matches the install pipeline's
// expectation that source can change between updates).
func TestMarketplace_StampSource_Ugly(t *testing.T) {
	medium := coreio.Local
	dest := t.TempDir()
	if err := medium.EnsureDir(core.Path(dest, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := "code: stamp-ugly\nname: Stamp Ugly\nversion: 0.1.0\nconfig:\n  source: old\n"
	if err := medium.Write(core.Path(dest, ".core", "view.yaml"), body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := app.StampSourceForTest(medium, dest, "marketplace:fresh"); err != nil {
		t.Fatalf("StampSource: %v", err)
	}
	var round config.ViewManifest
	if err := config.LoadManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:fresh" {
		t.Errorf("Config[source] = %q; want %q", src, "marketplace:fresh")
	}
}
