// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
)

// TestPkgRepo_FetchRepoSourceURL_Good downloads a repo archive,
// extracts it, and returns the resolved project root inside the wrapper
// directory.
func TestPkgRepo_FetchRepoSourceURL_Good(t *testing.T) {
	body := zipArchive(t, map[string]string{
		"owner-repo-sha/manifest.json": `{"name":"Play","short_name":"play","start_url":"/index.html"}`,
		"owner-repo-sha/index.html":    "<html>play</html>",
	})
	srv := zipServer(body)
	defer srv.Close()

	scratch := t.TempDir()
	root, err := FetchRepoSourceURL(context.Background(), coreio.Local, srv.URL+"/repo.zip", scratch, "repo-source.zip")
	if err != nil {
		t.Fatalf("FetchRepoSourceURL: %v", err)
	}
	want := core.Path(scratch, "repo-source", "owner-repo-sha")
	if root != want {
		t.Fatalf("root = %q; want %q", root, want)
	}
	if DetectPackageType(coreio.Local, root) != PackageTypePWA {
		t.Fatalf("DetectPackageType(root) = %v; want PackageTypePWA", DetectPackageType(coreio.Local, root))
	}
}

// TestPkgRepo_FetchRepoSourceURL_Bad rejects empty inputs and malformed
// repo references before any extraction work begins.
func TestPkgRepo_FetchRepoSourceURL_Bad(t *testing.T) {
	if _, err := FetchRepoSourceURL(context.Background(), coreio.Local, "", t.TempDir(), "repo.zip"); err == nil {
		t.Error("empty URL produced no error")
	}
	if _, err := FetchRepoSourceURL(context.Background(), coreio.Local, "https://example.com/repo.zip", "", "repo.zip"); err == nil {
		t.Error("empty scratch dir produced no error")
	}
	if _, err := FetchRepoSource(context.Background(), coreio.Local, "", t.TempDir()); err == nil {
		t.Error("empty repo ref produced no error")
	}
}

// TestPkgRepo_FetchRepoSourceURL_Ugly confirms the resolver collapses
// multiple single-child wrapper directories produced by source archives.
func TestPkgRepo_FetchRepoSourceURL_Ugly(t *testing.T) {
	body := zipArchive(t, map[string]string{
		"outer/inner/manifest.json": `{"name":"Play","short_name":"play","start_url":"/"}`,
		"outer/inner/index.html":    "<html/>",
	})
	srv := zipServer(body)
	defer srv.Close()

	scratch := t.TempDir()
	root, err := FetchRepoSourceURL(context.Background(), coreio.Local, srv.URL+"/repo.zip", scratch, "repo-source.zip")
	if err != nil {
		t.Fatalf("FetchRepoSourceURL: %v", err)
	}
	want := core.Path(scratch, "repo-source", "outer", "inner")
	if root != want {
		t.Fatalf("root = %q; want %q", root, want)
	}
}

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for path, body := range files {
		f, err := w.Create(path)
		if err != nil {
			t.Fatalf("zip create %s: %v", path, err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", path, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return out.Bytes()
}

func zipServer(body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(body)
	}))
}
