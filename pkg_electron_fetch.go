// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

// electronReleaseTimeout caps the GitHub release HTTP call so a slow
// origin cannot hang `core pkg wrap --electron`.
//
//	client := &http.Client{Timeout: electronReleaseTimeout}
const electronReleaseTimeout = 30 * time.Second

// GitHubAsset is one entry in a GitHub release's `assets` array. We
// keep only the fields the wrapper consumes.
//
//	{ "name": "renderer.zip", "browser_download_url": "https://..." }
type GitHubAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size,omitempty"`
}

// GitHubRelease is the subset of GitHub's release JSON the wrapper
// consumes. Used by FetchElectronRelease to enumerate downloadable
// assets.
//
//	var rel GitHubRelease
//	r := core.JSONUnmarshal(body, &rel)
type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name,omitempty"`
	Assets  []GitHubAsset `json:"assets,omitempty"`
}

// FetchElectronRelease retrieves the latest GitHub release metadata for
// `owner/repo`. The host argument is `github.com` for the public host or
// a self-hosted GitHub Enterprise hostname; tests can pass an
// `http://127.0.0.1:port` URL via FetchElectronReleaseURL instead.
//
//	rel, err := app.FetchElectronRelease(ctx, "github.com", "bitwarden", "clients")
//	for _, asset := range rel.Assets {
//	    if app.IsRendererAsset(asset.Name) { /* download */ }
//	}
//
// Rules:
//
//   - Empty owner or repo → typed error.
//
//   - Non-2xx response → typed error including the status.
//
//   - The function never downloads asset bodies — call DownloadAsset
//     for each renderer asset you actually need.
func FetchElectronRelease(ctx context.Context, host, owner, repo string) (*GitHubRelease, error) {
	if owner == "" || repo == "" {
		return nil, coreerr.E("app.FetchElectronRelease", "owner and repo are required", nil)
	}
	if host == "" {
		host = "github.com"
	}
	url := releaseAPIURL(host, owner, repo)
	return FetchElectronReleaseURL(ctx, url)
}

// FetchElectronReleaseURL fetches a GitHub-style release JSON from an
// arbitrary URL. Used directly by tests against an httptest server and
// by callers needing to override the API host.
//
//	rel, err := app.FetchElectronReleaseURL(ctx, srv.URL+"/rel.json")
func FetchElectronReleaseURL(ctx context.Context, url string) (*GitHubRelease, error) {
	if url == "" {
		return nil, coreerr.E("app.FetchElectronReleaseURL", "empty URL", nil)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, coreerr.E("app.FetchElectronReleaseURL", "request build failed", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: electronReleaseTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, coreerr.E("app.FetchElectronReleaseURL", "HTTP GET failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, coreerr.E(
			"app.FetchElectronReleaseURL",
			"non-2xx status: "+core.Sprint(resp.StatusCode)+" for "+url,
			nil,
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, coreerr.E("app.FetchElectronReleaseURL", "read body failed", err)
	}

	var rel GitHubRelease
	r := core.JSONUnmarshal(body, &rel)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, coreerr.E("app.FetchElectronReleaseURL", "decode release JSON failed", cause)
	}
	return &rel, nil
}

// releaseAPIURL builds the GitHub releases endpoint for `host/owner/repo`.
// Public github.com → https://api.github.com/repos/owner/repo/releases/latest
// GHES (e.g. ghe.example.com) → https://ghe.example.com/api/v3/...
//
//	url := releaseAPIURL("github.com", "owner", "repo")
func releaseAPIURL(host, owner, repo string) string {
	if host == "github.com" {
		return "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
	}
	// GitHub Enterprise — API path is /api/v3/.
	return "https://" + host + "/api/v3/repos/" + owner + "/" + repo + "/releases/latest"
}

// IsRendererAsset returns true when an asset name looks like a web /
// renderer bundle that's safe to extract into a CoreApp wrap. Matches
// the RFC §16.2 rule "downloads latest release assets (HTML/CSS/JS —
// NOT the Electron binary)" — installers, dmgs, exes are rejected; tar
// /zip archives are accepted (they are unpacked downstream).
//
//	if app.IsRendererAsset("renderer.zip") { /* download */ }
//
// The match is conservative: anything that looks platform-specific
// (`.dmg`, `.exe`, `.AppImage`, `.deb`, `.rpm`, `.msi`, `.snap`, `.pkg`,
// `.zip` named like an installer) is rejected. The wrapper prefers
// explicit "renderer", "asar", "web" tokens in the asset name.
func IsRendererAsset(name string) bool {
	if name == "" {
		return false
	}
	low := core.Lower(name)
	// PathExt(low) is already lowercase — comparisons below match that
	// convention (writing ".appimage" rather than ".AppImage" so the
	// switch arm actually fires on `.AppImage` assets in the wild).
	switch core.PathExt(low) {
	case ".dmg", ".exe", ".msi", ".pkg", ".deb", ".rpm", ".snap", ".appimage":
		return false
	case ".asar":
		return true
	case ".tar", ".tgz", ".gz", ".bz2", ".xz", ".zip":
		// Plain web-asset archives are accepted; installer archives
		// usually carry a platform marker in the basename ("Setup",
		// "win-x64", "darwin-arm64") which we reject.
		if hasPlatformMarker(low) {
			return false
		}
		return true
	}
	return false
}

// hasPlatformMarker returns true when an asset name carries a platform
// suffix that suggests an installer rather than a renderer bundle.
//
//	hasPlatformMarker("foo-win-x64.zip")     // true
//	hasPlatformMarker("renderer-bundle.zip") // false
func hasPlatformMarker(low string) bool {
	for _, marker := range []string{
		"-win-", "_win_", ".win.", "-windows-",
		"-mac-", "_mac_", ".mac.", "-darwin-", "-osx-",
		"-linux-", "_linux_", ".linux.",
		"-x64", "-x86", "-arm64",
		"-setup", "-installer",
	} {
		if core.Contains(low, marker) {
			return true
		}
	}
	return false
}

// DownloadAsset writes a release asset to disk via the supplied medium.
// Returns the absolute path of the written file. Caller decides whether
// to extract / move the asset afterwards.
//
//	path, err := app.DownloadAsset(ctx, coreio.Local, asset, "/tmp/scratch")
//
// Rules:
//
//   - Empty asset URL → typed error.
//
//   - Non-2xx response → typed error.
//
//   - The function streams the body via io.Copy so multi-MB renderer
//     bundles do not balloon memory.
func DownloadAsset(ctx context.Context, medium coreio.Medium, asset GitHubAsset, dir string) (string, error) {
	if asset.DownloadURL == "" {
		return "", coreerr.E("app.DownloadAsset", "empty asset URL", nil)
	}
	if dir == "" {
		return "", coreerr.E("app.DownloadAsset", "empty directory", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", coreerr.E("app.DownloadAsset", "request build failed", err)
	}

	client := &http.Client{Timeout: electronReleaseTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", coreerr.E("app.DownloadAsset", "HTTP GET failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", coreerr.E(
			"app.DownloadAsset",
			"non-2xx status: "+core.Sprint(resp.StatusCode),
			nil,
		)
	}

	if err := medium.EnsureDir(dir); err != nil {
		return "", coreerr.E("app.DownloadAsset", "ensure dir failed", err)
	}
	dest := core.Path(dir, asset.Name)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", coreerr.E("app.DownloadAsset", "read body failed", err)
	}
	if err := medium.Write(dest, string(body)); err != nil {
		return "", coreerr.E("app.DownloadAsset", "write failed", err)
	}
	return dest, nil
}

// SelectRendererAsset picks the first asset from a release whose name
// passes IsRendererAsset. Convenience wrapper for callers that just want
// "the asset I should download" without scanning the slice themselves.
//
//	asset, ok := app.SelectRendererAsset(rel)
//	if !ok { return errors.New("no renderer asset found") }
func SelectRendererAsset(rel *GitHubRelease) (GitHubAsset, bool) {
	if rel == nil {
		return GitHubAsset{}, false
	}
	for _, a := range rel.Assets {
		if IsRendererAsset(a.Name) {
			return a, true
		}
	}
	return GitHubAsset{}, false
}

// ParseGitHubRepo splits a `github.com/owner/repo` (or `owner/repo`)
// reference into its host, owner and repo components. The third return
// value reports whether parsing succeeded — an unrecognised input does
// not panic.
//
//	host, owner, repo, ok := app.ParseGitHubRepo("github.com/owner/repo")
//
// Accepted shapes:
//
//   - `github.com/owner/repo`
//   - `gitlab.com/owner/repo`
//   - `owner/repo` (defaults to host=github.com)
//   - `git@github.com:owner/repo.git`
//   - `https://github.com/owner/repo`
//   - `https://github.com/owner/repo.git`
func ParseGitHubRepo(in string) (host, owner, repo string, ok bool) {
	in = core.Trim(in)
	if in == "" {
		return "", "", "", false
	}

	// Strip schemes / git@ prefix.
	in = core.TrimPrefix(in, "https://")
	in = core.TrimPrefix(in, "http://")
	if core.HasPrefix(in, "git@") {
		// git@github.com:owner/repo.git → github.com/owner/repo
		rest := in[len("git@"):]
		if i := indexOf(rest, ":"); i > 0 {
			in = rest[:i] + "/" + rest[i+1:]
		}
	}
	in = core.TrimSuffix(in, ".git")

	parts := splitN(in, "/", 3)
	switch len(parts) {
	case 2:
		// owner/repo → default host github.com
		return "github.com", parts[0], parts[1], true
	case 3:
		// host/owner/repo
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return "", "", "", false
		}
		return parts[0], parts[1], parts[2], true
	default:
		return "", "", "", false
	}
}

// splitN splits s on sep into at most n parts. Mirrors strings.SplitN
// without importing the banned package.
//
//	splitN("a/b/c", "/", 2) // ["a", "b/c"]
func splitN(s, sep string, n int) []string {
	if sep == "" || n == 0 {
		return nil
	}
	out := []string{}
	for {
		if n == 1 {
			out = append(out, s)
			return out
		}
		i := indexOf(s, sep)
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i])
		s = s[i+len(sep):]
		n--
	}
}
