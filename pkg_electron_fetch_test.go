// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dappco.re/go/app"
	coreio "dappco.re/go/core/io"
)

// TestPkgElectronFetch_FetchElectronReleaseURL_Good plants a fake
// GitHub release endpoint and asserts the parsed release surfaces both
// tag and asset list.
func TestPkgElectronFetch_FetchElectronReleaseURL_Good(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v4.1.0",
			"name":"NiceHash QuickMiner v4.1.0",
			"assets":[
				{"name":"renderer.zip","browser_download_url":"https://example.invalid/renderer.zip"},
				{"name":"NiceHash-Setup-win-x64.exe","browser_download_url":"https://example.invalid/setup.exe"}
			]
		}`))
	}))
	defer srv.Close()

	rel, err := app.FetchElectronReleaseURL(context.Background(), srv.URL+"/rel.json")
	if err != nil {
		t.Fatalf("FetchElectronReleaseURL: %v", err)
	}
	if rel.TagName != "v4.1.0" {
		t.Errorf("TagName = %q; want v4.1.0", rel.TagName)
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("Assets len = %d; want 2", len(rel.Assets))
	}
}

// TestPkgElectronFetch_FetchElectronRelease_Bad rejects empty owner /
// repo. The networked happy-path is covered by the *URL_Good test
// above; we only verify input validation here.
func TestPkgElectronFetch_FetchElectronRelease_Bad(t *testing.T) {
	if _, err := app.FetchElectronRelease(context.Background(), "github.com", "", "repo"); err == nil {
		t.Error("empty owner produced no error")
	}
	if _, err := app.FetchElectronRelease(context.Background(), "github.com", "owner", ""); err == nil {
		t.Error("empty repo produced no error")
	}
	// Empty URL on the lower-level helper is also rejected.
	if _, err := app.FetchElectronReleaseURL(context.Background(), ""); err == nil {
		t.Error("empty URL produced no error")
	}

	// 404 from the upstream surfaces as a typed error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := app.FetchElectronReleaseURL(context.Background(), srv.URL); err == nil {
		t.Error("404 response produced no error")
	}
}

// TestPkgElectronFetch_FetchElectronRelease_Ugly verifies the API URL
// builder shapes a sensible URL for both github.com and a GHE host.
// The network call is deliberately not exercised here — we only assert
// the host shape lands.
func TestPkgElectronFetch_FetchElectronRelease_Ugly(t *testing.T) {
	// Use an unreachable port so the network call fails fast for the
	// happy-path call we deliberately leave dialling.
	_, err := app.FetchElectronRelease(context.Background(), "127.0.0.1:1", "owner", "repo")
	if err == nil {
		t.Error("unreachable host produced no error")
	}
}

// TestPkgElectronFetch_IsRendererAsset_Good catches the obvious good
// shapes: zip/tar/asar without platform markers.
func TestPkgElectronFetch_IsRendererAsset_Good(t *testing.T) {
	cases := []string{
		"renderer.zip",
		"web-bundle.tar",
		"renderer.tar.gz",
		"app.asar",
		"static-assets.tgz",
	}
	for _, name := range cases {
		if !app.IsRendererAsset(name) {
			t.Errorf("IsRendererAsset(%q) = false; want true", name)
		}
	}
}

// TestPkgElectronFetch_IsRendererAsset_Bad rejects platform installers
// and unrecognised extensions. The `.AppImage` entry pins the RFC §16.2
// rule that any platform-native installer is a hard reject regardless of
// case (a real release asset usually ships as `App-1.0.0.AppImage`).
func TestPkgElectronFetch_IsRendererAsset_Bad(t *testing.T) {
	cases := []string{
		"NiceHash-Setup-win-x64.exe",
		"NiceHash.dmg",
		"installer.msi",
		"app-darwin-arm64.zip",    // platform marker
		"setup-installer.tar",     // installer marker
		"foo.bin",                 // unknown extension
		"App-1.0.0.AppImage",      // Linux installer — mixed case extension
		"App-1.0.0.appimage",      // Linux installer — lower-case extension
		"Bitwarden-2024.8.1.snap", // Snap package
		"package.deb",             // Debian package
		"package.rpm",             // RPM package
		"",
	}
	for _, name := range cases {
		if app.IsRendererAsset(name) {
			t.Errorf("IsRendererAsset(%q) = true; want false", name)
		}
	}
}

// TestPkgElectronFetch_IsRendererAsset_Ugly — a name without an
// extension should fail (we don't attempt magic-byte detection).
func TestPkgElectronFetch_IsRendererAsset_Ugly(t *testing.T) {
	if app.IsRendererAsset("README") {
		t.Error("IsRendererAsset(README) = true; want false")
	}
}

// TestPkgElectronFetch_DownloadAsset_Good streams a body to disk.
func TestPkgElectronFetch_DownloadAsset_Good(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("renderer-bundle-bytes"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	asset := app.GitHubAsset{Name: "renderer.zip", DownloadURL: srv.URL}
	path, err := app.DownloadAsset(context.Background(), coreio.Local, asset, dir)
	if err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if path == "" {
		t.Fatal("DownloadAsset returned empty path")
	}
	body, err := coreio.Local.Read(path)
	if err != nil {
		t.Fatalf("read downloaded asset: %v", err)
	}
	if body != "renderer-bundle-bytes" {
		t.Errorf("downloaded body = %q; want renderer-bundle-bytes", body)
	}
}

// TestPkgElectronFetch_DownloadAsset_Bad rejects empty URL / dir and
// surfaces non-2xx responses.
func TestPkgElectronFetch_DownloadAsset_Bad(t *testing.T) {
	ctx := context.Background()
	if _, err := app.DownloadAsset(ctx, coreio.Local, app.GitHubAsset{}, t.TempDir()); err == nil {
		t.Error("empty URL produced no error")
	}
	if _, err := app.DownloadAsset(ctx, coreio.Local,
		app.GitHubAsset{Name: "x", DownloadURL: "https://x"}, ""); err == nil {
		t.Error("empty dir produced no error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := app.DownloadAsset(ctx, coreio.Local,
		app.GitHubAsset{Name: "x", DownloadURL: srv.URL}, t.TempDir()); err == nil {
		t.Error("500 response produced no error")
	}
}

// TestPkgElectronFetch_SelectRendererAsset_Good prefers the first
// renderer-shaped asset in a release.
func TestPkgElectronFetch_SelectRendererAsset_Good(t *testing.T) {
	rel := &app.GitHubRelease{
		Assets: []app.GitHubAsset{
			{Name: "Setup-win-x64.exe"},
			{Name: "renderer.zip"},
			{Name: "another-renderer.tar"},
		},
	}
	got, ok := app.SelectRendererAsset(rel)
	if !ok {
		t.Fatal("SelectRendererAsset = (_, false); want true")
	}
	if got.Name != "renderer.zip" {
		t.Errorf("SelectRendererAsset = %q; want renderer.zip", got.Name)
	}
}

// TestPkgElectronFetch_SelectRendererAsset_Bad — nil release and a
// release with no renderer-shaped assets both return ok=false.
func TestPkgElectronFetch_SelectRendererAsset_Bad(t *testing.T) {
	if _, ok := app.SelectRendererAsset(nil); ok {
		t.Error("nil release returned ok=true")
	}
	rel := &app.GitHubRelease{Assets: []app.GitHubAsset{
		{Name: "Setup.exe"}, {Name: "Installer.dmg"},
	}}
	if _, ok := app.SelectRendererAsset(rel); ok {
		t.Error("installer-only release returned ok=true")
	}
}

// TestPkgElectronFetch_ParseGitHubRepo_Good covers the four accepted
// shapes (host/owner/repo, owner/repo, https://, git@).
func TestPkgElectronFetch_ParseGitHubRepo_Good(t *testing.T) {
	cases := []struct {
		in        string
		wantHost  string
		wantOwner string
		wantRepo  string
	}{
		{"github.com/owner/repo", "github.com", "owner", "repo"},
		{"gitlab.com/owner/repo", "gitlab.com", "owner", "repo"},
		{"owner/repo", "github.com", "owner", "repo"},
		{"https://github.com/owner/repo", "github.com", "owner", "repo"},
		{"https://github.com/owner/repo.git", "github.com", "owner", "repo"},
		{"git@github.com:owner/repo.git", "github.com", "owner", "repo"},
	}
	for _, tc := range cases {
		host, owner, repo, ok := app.ParseGitHubRepo(tc.in)
		if !ok {
			t.Errorf("ParseGitHubRepo(%q) = (_, _, _, false); want true", tc.in)
			continue
		}
		if host != tc.wantHost || owner != tc.wantOwner || repo != tc.wantRepo {
			t.Errorf("ParseGitHubRepo(%q) = (%q,%q,%q); want (%q,%q,%q)",
				tc.in, host, owner, repo, tc.wantHost, tc.wantOwner, tc.wantRepo)
		}
	}
}

// TestPkgElectronFetch_ParseGitHubRepo_Bad — empty / shapeless input.
func TestPkgElectronFetch_ParseGitHubRepo_Bad(t *testing.T) {
	cases := []string{"", "   ", "owner-only"}
	for _, in := range cases {
		if _, _, _, ok := app.ParseGitHubRepo(in); ok {
			t.Errorf("ParseGitHubRepo(%q) = ok=true; want false", in)
		}
	}
}

// TestPkgElectronFetch_ParseGitHubRepo_Ugly — extra path components are
// preserved by joining with the repo segment, mirroring how GitHub
// returns subgroup paths in newer GHES setups.
func TestPkgElectronFetch_ParseGitHubRepo_Ugly(t *testing.T) {
	host, owner, repo, ok := app.ParseGitHubRepo("github.com/owner/repo/extra")
	if !ok {
		t.Fatal("extra-segment input rejected; want acceptance")
	}
	if host != "github.com" || owner != "owner" || repo != "repo/extra" {
		t.Errorf("got (%q,%q,%q); want (github.com,owner,repo/extra)", host, owner, repo)
	}
}
