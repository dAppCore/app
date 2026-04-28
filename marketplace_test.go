// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
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

func TestMarketplace_LoadMarketplaceCategory_Ugly(t *testing.T) {
	root := t.TempDir()
	cat := core.Path(root, "media")
	if err := coreio.Local.EnsureDir(cat); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(core.Path(cat, app.MarketplaceIndexFileName), "{not json"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := app.LoadMarketplaceCategory(coreio.Local, root, "media"); err == nil {
		t.Fatal("malformed category JSON should fail")
	}
}

// TestMarketplace_MarketplaceSearch_Good builds a two-category index
// and confirms a substring search returns the expected listings with
// the parent category stamped on each row.
func TestMarketplace_MarketplaceSearch_Good(t *testing.T) {
	root := writeTestMarketplace(t)

	results, err := app.MarketplaceSearch(coreio.Local, root, "photo")
	if err != nil {
		t.Fatalf("MarketplaceSearch: %v", err)
	}
	if len(results) != 1 || results[0].Code != "photo-browser" {
		t.Errorf("search 'photo' = %+v; want [photo-browser]", results)
	}
	if results[0].Category != "media" {
		t.Errorf("search hit did not stamp Category: got %q; want 'media'", results[0].Category)
	}

	// Empty needle → every listing. Each row must carry the category it
	// came from so a `core marketplace search ""` renders a complete
	// browse view.
	all, err := app.MarketplaceSearch(coreio.Local, root, "")
	if err != nil {
		t.Fatalf("MarketplaceSearch (empty): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("empty search = %d results; want 3", len(all))
	}
	for _, row := range all {
		if row.Category == "" {
			t.Errorf("empty search left Category blank on %q", row.Code)
		}
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

// TestMarketplace_MarketplaceResolve_Good resolves an exact code match
// and confirms the returned listing carries the parent category.
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
	if listing.Category != "media" {
		t.Errorf("Category = %q; want 'media'", listing.Category)
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

func TestMarketplace_MarketplaceResolve_Ugly(t *testing.T) {
	root := t.TempDir()
	if err := coreio.Local.Write(core.Path(root, app.MarketplaceIndexFileName), `{"version":1,"categories":["missing"]}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := app.MarketplaceResolve(coreio.Local, root, "anything"); err == nil {
		t.Fatal("resolve should fail when categories cannot be loaded")
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
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
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
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if src, _ := round.Config["source"].(string); src != "marketplace:fresh" {
		t.Errorf("Config[source] = %q; want %q", src, "marketplace:fresh")
	}
}

// TestMarketplace_MarketplaceRemove_Good — the marketplace-layer alias
// removes an installed package directory. Equivalent to PkgRemove but
// named per RFC §6.2 so callers reasoning at the marketplace level have
// a one-stop surface.
func TestMarketplace_MarketplaceRemove_Good(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	dir := core.Path(home, ".core", "apps", "rm-good", ".core")
	if err := medium.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dir, "view.yaml"),
		"code: rm-good\nname: Remove Good\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}
	if err := app.MarketplaceRemove(medium, home, "rm-good", false); err != nil {
		t.Fatalf("MarketplaceRemove: %v", err)
	}
	if medium.IsDir(core.Path(home, ".core", "apps", "rm-good")) {
		t.Error("install tree should have been removed")
	}
}

// TestMarketplace_MarketplaceRemove_Bad — calling against a missing
// package surfaces a typed error so the CLI can print a friendly "not
// installed" message rather than silently succeeding.
func TestMarketplace_MarketplaceRemove_Bad(t *testing.T) {
	home := t.TempDir()
	if err := app.MarketplaceRemove(coreio.Local, home, "never-installed", false); err == nil {
		t.Fatal("MarketplaceRemove on missing package should error")
	}
}

// TestMarketplace_MarketplaceRemove_Ugly — Purge=true drops both the
// install tree and the workspace data tree, mirroring
// `core marketplace remove --purge` semantics.
func TestMarketplace_MarketplaceRemove_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local
	dir := core.Path(home, ".core", "apps", "rm-ugly", ".core")
	if err := medium.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(dir, "view.yaml"),
		"code: rm-ugly\nname: Remove Ugly\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}
	// Plant a data tree so Purge has work to do.
	dataDir := core.Path(home, ".core", "data", "rm-ugly")
	if err := medium.EnsureDir(dataDir); err != nil {
		t.Fatalf("EnsureDir data: %v", err)
	}
	if err := medium.Write(core.Path(dataDir, "kv.db"), "x"); err != nil {
		t.Fatalf("Write kv: %v", err)
	}
	if err := app.MarketplaceRemove(medium, home, "rm-ugly", true); err != nil {
		t.Fatalf("MarketplaceRemove: %v", err)
	}
	if medium.IsDir(dataDir) {
		t.Error("purge should have removed the workspace data tree")
	}
}

// TestMarketplace_MarketplaceCategories_Good lists the top-level
// categories in sorted order. Matches the RFC §6.1 category-as-directory
// convention the marketplace commands expose via `core marketplace
// categories`.
func TestMarketplace_MarketplaceCategories_Good(t *testing.T) {
	root := writeTestMarketplace(t)
	cats, err := app.MarketplaceCategories(coreio.Local, root)
	if err != nil {
		t.Fatalf("MarketplaceCategories: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("len = %d; want 2", len(cats))
	}
	if cats[0] != "media" || cats[1] != "tools" {
		t.Errorf("sorted categories = %v; want [media tools]", cats)
	}
}

// TestMarketplace_MarketplaceCategories_Bad propagates the missing
// index error so the CLI can instruct the user to fetch first.
func TestMarketplace_MarketplaceCategories_Bad(t *testing.T) {
	if _, err := app.MarketplaceCategories(coreio.Local, t.TempDir()); err == nil {
		t.Error("missing index produced no error")
	}
}

// TestMarketplace_MarketplaceCategories_Ugly collapses duplicate
// category entries so a misbehaving marketplace never produces
// duplicate rows in the browser.
func TestMarketplace_MarketplaceCategories_Ugly(t *testing.T) {
	medium := coreio.Local
	root := t.TempDir()
	body := `{"version":1,"categories":["media","tools","media",""]}`
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), body); err != nil {
		t.Fatalf("Write index: %v", err)
	}
	cats, err := app.MarketplaceCategories(medium, root)
	if err != nil {
		t.Fatalf("MarketplaceCategories: %v", err)
	}
	if len(cats) != 2 || cats[0] != "media" || cats[1] != "tools" {
		t.Errorf("deduped categories = %v; want [media tools]", cats)
	}
}

// TestMarketplace_MarketplaceBrowse_Good returns every listing in a
// category with the Category slot stamped so the same projection
// covers search + browse without re-walking the tree.
func TestMarketplace_MarketplaceBrowse_Good(t *testing.T) {
	root := writeTestMarketplace(t)
	listings, err := app.MarketplaceBrowse(coreio.Local, root, "media")
	if err != nil {
		t.Fatalf("MarketplaceBrowse: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("len = %d; want 2", len(listings))
	}
	for _, row := range listings {
		if row.Category != "media" {
			t.Errorf("browse row %q has Category %q; want 'media'", row.Code, row.Category)
		}
	}
}

// TestMarketplace_MarketplaceBrowse_Bad rejects empty + unknown
// categories so the CLI can point at `marketplace categories`.
func TestMarketplace_MarketplaceBrowse_Bad(t *testing.T) {
	root := writeTestMarketplace(t)
	if _, err := app.MarketplaceBrowse(coreio.Local, root, ""); err == nil {
		t.Error("empty category produced no error")
	}
	if _, err := app.MarketplaceBrowse(coreio.Local, root, "not-a-category"); err == nil {
		t.Error("unknown category produced no error")
	}
}

// TestMarketplace_MarketplaceBrowse_Ugly — the root-level index is
// absent so the browse path propagates the same "run marketplace
// fetch" hint the other loaders use.
func TestMarketplace_MarketplaceBrowse_Ugly(t *testing.T) {
	if _, err := app.MarketplaceBrowse(coreio.Local, t.TempDir(), "media"); err == nil {
		t.Error("missing index produced no error")
	}
}

// TestMarketplace_StampCategory_Good records the listing category on
// an existing installed manifest so `pkg info` / `pkg list` can surface
// it without re-walking the marketplace index.
func TestMarketplace_StampCategory_Good(t *testing.T) {
	dest := t.TempDir()
	medium := coreio.Local
	viewPath := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, "code: stamp-cat\nname: Stamp Cat\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := app.StampCategoryForTest(medium, dest, "media"); err != nil {
		t.Fatalf("StampCategoryForTest: %v", err)
	}
	body, err := medium.Read(viewPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !core.Contains(body, "category: media") {
		t.Errorf("stamped manifest missing `category: media`:\n%s", body)
	}
}

// TestMarketplace_StampCategory_Bad — empty category is a no-op, no
// write is performed so the caller can pass listing.Category
// unconditionally.
func TestMarketplace_StampCategory_Bad(t *testing.T) {
	dest := t.TempDir()
	medium := coreio.Local
	if err := app.StampCategoryForTest(medium, dest, ""); err != nil {
		t.Errorf("empty category returned %v; want nil", err)
	}
}

// TestMarketplace_StampCategory_Ugly — missing view.yaml at dest is a
// no-op (metadata is advisory; caller never surfaces the error).
func TestMarketplace_StampCategory_Ugly(t *testing.T) {
	dest := t.TempDir()
	if err := app.StampCategoryForTest(coreio.Local, dest, "media"); err != nil {
		t.Errorf("missing manifest returned %v; want nil", err)
	}
}
