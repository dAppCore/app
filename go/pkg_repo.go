// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// FetchRepoSource downloads and extracts a GitHub-style repository
// source archive into `scratchDir`, returning the resolved project root
// within the extracted tree.
//
//	root, err := app.FetchRepoSource(ctx, coreio.Local, "github.com/owner/repo", scratch)
func FetchRepoSource(ctx context.Context, medium coreio.Medium, ref, scratchDir string) (
	string, error,
) {
	host, owner, repo, ok := ParseGitHubRepo(ref)
	if !ok {
		return "", core.E("app.FetchRepoSource", "cannot parse repo reference: "+ref, nil)
	}
	if !isGitHubReleaseHost(host) {
		return "", core.E(
			"app.FetchRepoSource",
			"repo host does not expose GitHub archives: "+host,
			nil,
		)
	}
	return FetchRepoSourceURL(ctx, medium, repoArchiveURL(host, owner, repo), scratchDir, repo+"-source.zip")
}

// FetchRepoSourceURL is the testable lower-level counterpart to
// FetchRepoSource. It downloads a repo archive from `url`, extracts it
// into `scratchDir`, then returns the deepest single-root project
// directory that looks like an app source tree.
//
//	root, err := app.FetchRepoSourceURL(ctx, coreio.Local, srv.URL+"/repo.zip", scratch, "repo.zip")
func FetchRepoSourceURL(ctx context.Context, medium coreio.Medium, url, scratchDir, archiveName string) (
	string, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if url == "" {
		return "", core.E("app.FetchRepoSourceURL", "empty archive URL", nil)
	}
	if scratchDir == "" {
		return "", core.E("app.FetchRepoSourceURL", "empty scratch dir", nil)
	}
	if archiveName == "" {
		archiveName = "source.zip"
	}

	if medium.IsDir(scratchDir) {
		if err := medium.DeleteAll(scratchDir); err != nil {
			return "", core.E("app.FetchRepoSourceURL", "clear scratch dir failed", err)
		}
	}
	if err := medium.EnsureDir(scratchDir); err != nil {
		return "", core.E("app.FetchRepoSourceURL", "ensure scratch dir failed", err)
	}

	archivePath, err := DownloadAsset(ctx, medium, GitHubAsset{
		Name:        archiveName,
		DownloadURL: url,
	}, scratchDir)
	if err != nil {
		return "", core.E("app.FetchRepoSourceURL", "archive download failed", err)
	}

	extractDir := ArchiveExtractedDir(scratchDir, archiveName)
	if err := ExtractArchive(medium, archivePath, extractDir); err != nil {
		return "", core.E("app.FetchRepoSourceURL", "archive extract failed", err)
	}

	root, err := resolveRepoSourceRoot(medium, extractDir)
	if err != nil {
		return "", core.E("app.FetchRepoSourceURL", "resolve project root failed", err)
	}
	return root, nil
}

// LoadRepoPWAManifest fetches a repo source archive and returns the
// parsed PWA manifest plus the extracted project root so the caller can
// copy the asset tree into an installed wrap. Accepts both
// `manifest.json` and `manifest.webmanifest`.
//
//	pwa, root, err := app.LoadRepoPWAManifest(ctx, coreio.Local, ref, scratch)
func LoadRepoPWAManifest(ctx context.Context, medium coreio.Medium, ref, scratchDir string) (
	*PWAManifest, string, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	root, err := FetchRepoSource(ctx, medium, ref, scratchDir)
	if err != nil {
		return nil, "", err
	}
	path, ok := FindLocalPWAManifest(medium, root)
	if !ok {
		return nil, "", core.E("app.LoadRepoPWAManifest", "PWA manifest not found under "+root, nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, "", core.E("app.LoadRepoPWAManifest", "read PWA manifest failed", err)
	}
	var manifest PWAManifest
	r := core.JSONUnmarshal([]byte(body), &manifest)
	if !r.OK {
		cause, _ := r.Value.(error)
		return nil, "", core.E("app.LoadRepoPWAManifest", "decode PWA manifest failed", cause)
	}
	return &manifest, root, nil
}

// repoArchiveURL returns the GitHub archive endpoint for a repository's
// source zipball.
//
//	url := repoArchiveURL("github.com", "owner", "repo")
func repoArchiveURL(host, owner, repo string) string {
	if host == "github.com" {
		return "https://api.github.com/repos/" + owner + "/" + repo + "/zipball/HEAD"
	}
	return "https://" + host + "/api/v3/repos/" + owner + "/" + repo + "/zipball/HEAD"
}

// resolveRepoSourceRoot collapses one or more single-child wrapper
// directories produced by repo archives until it reaches the actual
// project root.
//
//	root, err := resolveRepoSourceRoot(coreio.Local, "/tmp/scratch/repo-source")
func resolveRepoSourceRoot(medium coreio.Medium, dir string) (
	string, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return "", core.E("app.resolveRepoSourceRoot", "empty directory", nil)
	}
	if !medium.IsDir(dir) {
		return "", core.E("app.resolveRepoSourceRoot", "not a directory: "+dir, nil)
	}

	cur := dir
	for range 8 {
		if DetectPackageType(medium, cur) != PackageTypeUnknown {
			return cur, nil
		}
		next, ok, err := singleVisibleSubdir(medium, cur)
		if err != nil {
			return "", err
		}
		if !ok {
			return cur, nil
		}
		cur = next
	}
	return cur, nil
}

// singleVisibleSubdir returns the only visible child directory when
// `dir` contains exactly one non-hidden directory and no visible files.
//
//	next, ok, err := singleVisibleSubdir(coreio.Local, "/tmp/root")
func singleVisibleSubdir(medium coreio.Medium, dir string) (
	string, bool, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	entries, err := medium.List(dir)
	if err != nil {
		return "", false, err
	}
	child := ""
	dirs := 0
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		if entry.IsDir() {
			dirs++
			child = core.Path(dir, name)
			continue
		}
		files++
	}
	if dirs == 1 && files == 0 {
		return child, true, nil
	}
	return "", false, nil
}
