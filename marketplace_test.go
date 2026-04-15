// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
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
