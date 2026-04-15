// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// MarketplaceIndexFileName is the top-level index file every category
// directory carries. Matches RFC §6.1 ("`index.json` {version,
// modules[], categories[]}").
//
//	body, _ := medium.Read(core.Path(root, app.MarketplaceIndexFileName))
const MarketplaceIndexFileName = "index.json"

// MarketplaceIndex is the parsed form of the top-level `index.json`.
// Each listed category is itself a directory with its own `index.json`
// whose shape is MarketplaceCategoryIndex.
//
//	var idx app.MarketplaceIndex
//	r := core.JSONUnmarshal([]byte(body), &idx)
type MarketplaceIndex struct {
	Version    int      `json:"version"`
	Categories []string `json:"categories,omitempty"`
	Modules    []string `json:"modules,omitempty"`
}

// MarketplaceCategoryIndex is the shape of a category-level index.json
// (e.g. `media/index.json`). Each Entry is a single package.
//
//	var cat app.MarketplaceCategoryIndex
//	r := core.JSONUnmarshal(body, &cat)
type MarketplaceCategoryIndex struct {
	Version  int                  `json:"version"`
	Category string               `json:"category"`
	Entries  []MarketplaceListing `json:"entries"`
}

// MarketplaceListing is one row in a category index. The shape matches
// what `core pkg install <vendor>/<name>` needs to resolve a package
// without extra round-trips.
//
//	listing := MarketplaceListing{
//	    Code: "photo-browser",
//	    Type: "native",
//	    Repo: "github.com/core/photo-browser",
//	}
type MarketplaceListing struct {
	Code        string `json:"code"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Type        string `json:"type,omitempty"` // "native" | "pwa" | "electron" | "web"
	Repo        string `json:"repo,omitempty"` // clone URL / github path
	URL         string `json:"url,omitempty"`  // for PWAs — the live manifest URL
	Description string `json:"description,omitempty"`
	SignKey     string `json:"sign_key,omitempty"` // hex-encoded ed25519 public key
}

// LoadMarketplaceIndex reads and parses the top-level index.json from a
// local cache directory. Callers typically point at a cloned copy of
// the marketplace repo so offline lookups work.
//
//	idx, err := app.LoadMarketplaceIndex(coreio.Local, "/Users/me/.core/marketplace")
func LoadMarketplaceIndex(medium coreio.Medium, root string) (*MarketplaceIndex, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if root == "" {
		return nil, coreerr.E("app.LoadMarketplaceIndex", "empty root", nil)
	}
	path := core.Path(root, MarketplaceIndexFileName)
	if !medium.Exists(path) {
		return nil, coreerr.E("app.LoadMarketplaceIndex", "index not found: "+path, nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, coreerr.E("app.LoadMarketplaceIndex", "read failed: "+path, err)
	}
	var idx MarketplaceIndex
	r := core.JSONUnmarshal([]byte(body), &idx)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, coreerr.E("app.LoadMarketplaceIndex", "decode failed: "+path, cause)
	}
	return &idx, nil
}

// LoadMarketplaceCategory reads a category-level index.json. The
// category name is an entry from MarketplaceIndex.Categories.
//
//	cat, err := app.LoadMarketplaceCategory(medium, root, "media")
func LoadMarketplaceCategory(medium coreio.Medium, root, category string) (*MarketplaceCategoryIndex, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if root == "" || category == "" {
		return nil, coreerr.E("app.LoadMarketplaceCategory", "empty root or category", nil)
	}
	path := core.Path(root, category, MarketplaceIndexFileName)
	if !medium.Exists(path) {
		return nil, coreerr.E("app.LoadMarketplaceCategory", "category index not found: "+path, nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, coreerr.E("app.LoadMarketplaceCategory", "read failed: "+path, err)
	}
	var cat MarketplaceCategoryIndex
	r := core.JSONUnmarshal([]byte(body), &cat)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, coreerr.E("app.LoadMarketplaceCategory", "decode failed: "+path, cause)
	}
	return &cat, nil
}

// MarketplaceSearch walks every category in the index and returns the
// listings whose code, name or description contains the needle. Case
// insensitive substring match — matches the CLI `search` ergonomics.
//
//	results, err := app.MarketplaceSearch(medium, root, "photo")
func MarketplaceSearch(medium coreio.Medium, root, needle string) ([]MarketplaceListing, error) {
	idx, err := LoadMarketplaceIndex(medium, root)
	if err != nil {
		return nil, err
	}
	needle = core.Lower(core.Trim(needle))
	var out []MarketplaceListing
	for _, cat := range idx.Categories {
		c, err := LoadMarketplaceCategory(medium, root, cat)
		if err != nil {
			// Skip categories the walker cannot read — a partial search
			// is more useful than a failed search.
			continue
		}
		for _, entry := range c.Entries {
			if needle == "" || listingMatches(entry, needle) {
				out = append(out, entry)
			}
		}
	}
	return out, nil
}

// MarketplaceResolve walks every category looking for an exact code
// match. Returns the first hit — codes are globally unique in the
// marketplace (enforced by the submission process).
//
//	listing, err := app.MarketplaceResolve(medium, root, "photo-browser")
func MarketplaceResolve(medium coreio.Medium, root, code string) (*MarketplaceListing, error) {
	if code == "" {
		return nil, coreerr.E("app.MarketplaceResolve", "empty code", nil)
	}
	idx, err := LoadMarketplaceIndex(medium, root)
	if err != nil {
		return nil, err
	}
	for _, cat := range idx.Categories {
		c, err := LoadMarketplaceCategory(medium, root, cat)
		if err != nil {
			continue
		}
		for i, entry := range c.Entries {
			if entry.Code == code {
				return &c.Entries[i], nil
			}
		}
	}
	return nil, coreerr.E("app.MarketplaceResolve", "no marketplace entry with code "+code, nil)
}

// listingMatches performs a case-insensitive substring match across the
// listing's searchable fields.
//
//	listingMatches(entry, "photo") // true when entry.Code contains "photo"
func listingMatches(entry MarketplaceListing, needle string) bool {
	for _, field := range []string{entry.Code, entry.Name, entry.Description} {
		if core.Contains(core.Lower(field), needle) {
			return true
		}
	}
	return false
}

// MarketplaceFetchOptions tunes MarketplaceFetch. The zero value clones
// the default marketplace URL (kept external so the repo isn't pinned
// inside the binary).
//
//	opts := app.MarketplaceFetchOptions{
//	    URL: "https://forge.lthn.ai/core/marketplace.git",
//	    Dir: "/Users/me/.core/marketplace",
//	}
type MarketplaceFetchOptions struct {
	URL string // git clone URL of the marketplace repo
	Dir string // local cache directory
}

// MarketplaceFetch clones or updates the marketplace index repo so the
// local cache is fresh. Delegates to `git` via the Core process
// primitive — the host binary must have `git` on PATH.
//
//	err := app.MarketplaceFetch(ctx, c, app.MarketplaceFetchOptions{
//	    URL: "https://forge.lthn.ai/core/marketplace.git",
//	    Dir: "/Users/me/.core/marketplace",
//	})
//
// Rules:
//
//   - Returns an error when the process primitive is missing (no `c`).
//
//   - First invocation clones; subsequent invocations `git pull` in the
//     existing directory.
func MarketplaceFetch(ctx context.Context, c *core.Core, opts MarketplaceFetchOptions) error {
	if c == nil {
		return coreerr.E("app.MarketplaceFetch", "nil core", nil)
	}
	if opts.URL == "" {
		return coreerr.E("app.MarketplaceFetch", "empty marketplace URL", nil)
	}
	if opts.Dir == "" {
		return coreerr.E("app.MarketplaceFetch", "empty marketplace dir", nil)
	}

	proc := c.Process()
	if proc == nil {
		return coreerr.E("app.MarketplaceFetch", "core.Process() is nil", nil)
	}

	medium := coreio.Local
	if medium.IsDir(core.Path(opts.Dir, ".git")) {
		// Existing clone → pull.
		r := proc.RunIn(ctx, opts.Dir, "git", "pull", "--depth=1", "--ff-only")
		if !r.OK {
			return coreerr.E("app.MarketplaceFetch", "git pull failed", extractErr(r))
		}
		return nil
	}

	if err := medium.EnsureDir(core.PathDir(opts.Dir)); err != nil {
		return coreerr.E("app.MarketplaceFetch", "ensure parent dir failed", err)
	}
	r := proc.Run(ctx, "git", "clone", "--depth=1", opts.URL, opts.Dir)
	if !r.OK {
		return coreerr.E("app.MarketplaceFetch", "git clone failed", extractErr(r))
	}
	return nil
}

// MarketplaceInstall resolves a marketplace listing and installs it
// into `<home>/.core/apps/<code>/`. For native listings it clones the
// package repo; for PWA and Electron listings it delegates to the
// wrapping primitives declared in the app package.
//
// Updates use the same code path — when an install exists, Force=true
// replaces it.
//
//	err := app.MarketplaceInstall(ctx, c, app.MarketplaceInstallOptions{
//	    Root:    "/Users/me/.core/marketplace",
//	    Home:    "/Users/me",
//	    Code:    "photo-browser",
//	})
func MarketplaceInstall(ctx context.Context, c *core.Core, opts MarketplaceInstallOptions) (string, error) {
	if c == nil {
		return "", coreerr.E("app.MarketplaceInstall", "nil core", nil)
	}
	listing, err := MarketplaceResolve(coreio.Local, opts.Root, opts.Code)
	if err != nil {
		return "", err
	}

	home := opts.Home
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return "", coreerr.E("app.MarketplaceInstall", "cannot resolve home dir", nil)
	}

	dest := core.Path(home, ".core", AppsDirName, listing.Code)
	medium := coreio.Local
	if medium.IsDir(dest) && !opts.Force {
		return dest, coreerr.E(
			"app.MarketplaceInstall",
			"already installed at "+dest+" (use Force to replace)",
			nil,
		)
	}

	switch ParsePackageType(listing.Type) {
	case PackageTypeNative:
		if err := installNativeFromRepo(ctx, c, listing, dest); err != nil {
			return dest, err
		}
		if err := verifyListingAfterInstall(medium, dest, listing, opts); err != nil {
			return dest, err
		}
		return dest, nil
	case PackageTypePWA:
		pwa, err := FetchPWAManifest(ctx, listing.URL)
		if err != nil {
			return dest, err
		}
		m := WrapPWA(pwa, WrapPWAOptions{TargetURL: listing.URL, Code: listing.Code})
		if m == nil {
			return dest, coreerr.E("app.MarketplaceInstall", "WrapPWA returned nil", nil)
		}
		installed, err := InstallWrappedPWA(medium, m, PkgInstallOptions{
			Home:   home,
			Force:  opts.Force,
			Source: "marketplace:" + listing.Code,
		})
		if err != nil {
			return installed, err
		}
		// PWA wraps are unsigned by construction (the CoreApp signs the
		// wrapped manifest only when the user supplies a key) so there
		// is nothing to verify against the listing's `sign_key` here.
		// The listing's own pinned key is used by `pkg install --sign`
		// when invoked.
		return installed, nil
	case PackageTypeWeb, PackageTypeElectron, PackageTypeUnknown:
		// Electron handled via repo clone (renderer assets only) — same
		// path as native for now; the CLI-side flag drives a subsequent
		// scan + re-wrap pass.
		if err := installNativeFromRepo(ctx, c, listing, dest); err != nil {
			return dest, err
		}
		if err := verifyListingAfterInstall(medium, dest, listing, opts); err != nil {
			return dest, err
		}
		return dest, nil
	}
	return dest, coreerr.E("app.MarketplaceInstall", "unreachable type switch", nil)
}

// verifyListingAfterInstall runs VerifyListing as the install's final
// step unless the caller opted out (SkipVerify=true). A failed
// verification rolls back the install — leaving an unverified package
// on disk would let the user accidentally boot it later.
//
//	if err := verifyListingAfterInstall(medium, dest, listing, opts); err != nil {
//	    return dest, err
//	}
func verifyListingAfterInstall(medium coreio.Medium, dest string, listing *MarketplaceListing, opts MarketplaceInstallOptions) error {
	if opts.SkipVerify {
		return nil
	}
	if err := VerifyListing(medium, dest, listing); err != nil {
		// Roll back — the install was incomplete from a security
		// standpoint, so we delete the destination so the next install
		// attempt starts clean.
		_ = medium.DeleteAll(dest)
		return err
	}
	return nil
}

// MarketplaceInstallOptions tunes MarketplaceInstall. Force replaces an
// existing install; Home overrides $DIR_HOME.
type MarketplaceInstallOptions struct {
	Root       string // marketplace cache root (where index.json lives)
	Home       string // user home; defaults to $DIR_HOME
	Code       string // listing code to install
	Force      bool   // overwrite an existing install
	SkipVerify bool   // bypass the post-install signature check (test-only)
}

// installNativeFromRepo clones a native-type marketplace entry into the
// destination directory. Delegated to `git clone --depth=1`.
//
//	err := installNativeFromRepo(ctx, c, listing, dest)
func installNativeFromRepo(ctx context.Context, c *core.Core, listing *MarketplaceListing, dest string) error {
	if listing == nil || listing.Repo == "" {
		return coreerr.E("app.installNativeFromRepo", "empty repo in listing", nil)
	}
	medium := coreio.Local
	if medium.IsDir(dest) {
		if err := medium.DeleteAll(dest); err != nil {
			return coreerr.E("app.installNativeFromRepo", "remove existing failed", err)
		}
	}
	if err := medium.EnsureDir(core.PathDir(dest)); err != nil {
		return coreerr.E("app.installNativeFromRepo", "ensure dir failed", err)
	}
	proc := c.Process()
	if proc == nil {
		return coreerr.E("app.installNativeFromRepo", "core.Process() is nil", nil)
	}
	r := proc.Run(ctx, "git", "clone", "--depth=1", listing.Repo, dest)
	if !r.OK {
		return coreerr.E("app.installNativeFromRepo", "git clone failed", extractErr(r))
	}

	// Stamp the source into .core/view.yaml so `core pkg list` can show
	// it. Failure is non-fatal — the clone itself succeeded.
	if err := stampSource(medium, dest, "marketplace:"+listing.Code); err != nil {
		// Don't wrap — this is best-effort metadata.
		_ = err
	}
	return nil
}

// stampSource re-writes the installed view.yaml with
// `Config["source"] = source`. Keeps `core pkg list` honest about where
// the package came from without forcing every marketplace entry to
// know about the field.
//
//	_ = stampSource(medium, dest, "marketplace:photo-browser")
func stampSource(medium coreio.Medium, dest, source string) error {
	path := core.Path(dest, ".core", "view.yaml")
	if !medium.Exists(path) {
		return nil
	}
	var manifest config.ViewManifest
	if err := config.LoadManifest(medium, path, &manifest); err != nil {
		return err
	}
	if manifest.Config == nil {
		manifest.Config = map[string]any{}
	}
	manifest.Config["source"] = source
	body, err := yamlMarshal(&manifest)
	if err != nil {
		return err
	}
	return medium.Write(path, body)
}

// yamlMarshal is a thin wrapper so tests (and stampSource) don't depend
// on the yaml import path directly — keeps the signature stable if we
// swap encoders later.
//
//	body, err := yamlMarshal(&manifest)
func yamlMarshal(v any) (string, error) {
	out, err := yamlMarshalBytes(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// extractErr pulls an error from a core.Result, returning nil when the
// value wasn't an error (e.g. a string payload).
//
//	err := extractErr(r) // r.Value.(error)
func extractErr(r core.Result) error {
	if r.OK {
		return nil
	}
	if e, ok := r.Value.(error); ok {
		return e
	}
	return coreerr.E("app.extractErr", core.Sprint(r.Value), nil)
}
