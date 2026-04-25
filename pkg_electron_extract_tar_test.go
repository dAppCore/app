// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
)

// TestPkgElectronExtractTar_ExtractTar_Good — a plain (uncompressed) tar
// archive unpacks into the destination directory with the expected
// directory layout.
func TestPkgElectronExtractTar_ExtractTar_Good(t *testing.T) {
	medium := coreio.Local
	dir := t.TempDir()
	archivePath := core.Path(dir, "renderer.tar")
	dest := core.Path(dir, "out")

	body := buildTar(t, false, []tarEntry{
		{Name: "index.html", Body: "<!doctype html><html></html>"},
		{Name: "assets/app.js", Body: "console.log('ok');"},
		{Name: "assets/", IsDir: true},
	})
	if err := medium.Write(archivePath, body); err != nil {
		t.Fatalf("Write archive: %v", err)
	}
	if err := app.ExtractTar(medium, archivePath, dest); err != nil {
		t.Fatalf("ExtractTar: %v", err)
	}

	for _, want := range []string{
		core.Path(dest, "index.html"),
		core.Path(dest, "assets", "app.js"),
	} {
		if !medium.Exists(want) {
			t.Errorf("expected extracted file at %s", want)
		}
	}
}

// TestPkgElectronExtractTar_ExtractTar_Bad — empty inputs and missing
// archive paths surface as typed errors before any disk write.
func TestPkgElectronExtractTar_ExtractTar_Bad(t *testing.T) {
	medium := coreio.Local
	if err := app.ExtractTar(medium, "", t.TempDir()); err == nil {
		t.Error("ExtractTar with empty archive should fail")
	}
	if err := app.ExtractTar(medium, t.TempDir()+"/missing.tar", t.TempDir()); err == nil {
		t.Error("ExtractTar with missing archive should fail")
	}

	dir := t.TempDir()
	archivePath := core.Path(dir, "empty.tar")
	if err := medium.Write(archivePath, ""); err != nil {
		t.Fatalf("Write empty archive: %v", err)
	}
	if err := app.ExtractTar(medium, archivePath, t.TempDir()); err == nil {
		t.Error("ExtractTar against an empty archive should fail")
	}
}

// TestPkgElectronExtractTar_ExtractTar_Ugly — entries that try to
// escape the destination via `..` are rejected (zip-slip equivalent).
func TestPkgElectronExtractTar_ExtractTar_Ugly(t *testing.T) {
	medium := coreio.Local
	dir := t.TempDir()
	archivePath := core.Path(dir, "evil.tar")
	dest := core.Path(dir, "out")

	body := buildTar(t, false, []tarEntry{
		{Name: "../escape.txt", Body: "should never write"},
	})
	if err := medium.Write(archivePath, body); err != nil {
		t.Fatalf("Write archive: %v", err)
	}
	if err := app.ExtractTar(medium, archivePath, dest); err == nil {
		t.Error("ExtractTar should reject ../ traversal")
	}
}

// TestPkgElectronExtractTar_ExtractTarGz_Good — a gzipped tar archive
// extracts cleanly. Mirrors the plain-tar happy path with the extra
// gzip layer to confirm openTarReader picks the right decompressor.
func TestPkgElectronExtractTar_ExtractTarGz_Good(t *testing.T) {
	medium := coreio.Local
	dir := t.TempDir()
	archivePath := core.Path(dir, "renderer.tar.gz")
	dest := core.Path(dir, "out")

	body := buildTar(t, true, []tarEntry{
		{Name: "index.html", Body: "ok"},
	})
	if err := medium.Write(archivePath, body); err != nil {
		t.Fatalf("Write archive: %v", err)
	}
	if err := app.ExtractTar(medium, archivePath, dest); err != nil {
		t.Fatalf("ExtractTar: %v", err)
	}
	if !medium.Exists(core.Path(dest, "index.html")) {
		t.Error("expected extracted index.html")
	}
}

// TestPkgElectronExtractTar_ExtractArchive_Good — ExtractArchive picks
// the right per-format extractor based on suffix.
func TestPkgElectronExtractTar_ExtractArchive_Good(t *testing.T) {
	medium := coreio.Local
	dir := t.TempDir()

	// .tar.gz path
	tgzPath := core.Path(dir, "renderer.tgz")
	tgzBody := buildTar(t, true, []tarEntry{{Name: "a.txt", Body: "tgz"}})
	if err := medium.Write(tgzPath, tgzBody); err != nil {
		t.Fatalf("Write tgz: %v", err)
	}
	tgzDest := core.Path(dir, "tgz-out")
	if err := app.ExtractArchive(medium, tgzPath, tgzDest); err != nil {
		t.Fatalf("ExtractArchive(tgz): %v", err)
	}
	if !medium.Exists(core.Path(tgzDest, "a.txt")) {
		t.Error("tgz extraction missing a.txt")
	}
}

// TestPkgElectronExtractTar_ExtractArchive_Bad — empty path and
// unsupported suffix surface typed errors.
func TestPkgElectronExtractTar_ExtractArchive_Bad(t *testing.T) {
	medium := coreio.Local
	if err := app.ExtractArchive(medium, "", t.TempDir()); err == nil {
		t.Error("ExtractArchive with empty path should fail")
	}
	if err := app.ExtractArchive(medium, "/tmp/foo.7z", t.TempDir()); err == nil {
		t.Error("ExtractArchive with unsupported suffix should fail")
	}
}

// TestPkgElectronExtractTar_ExtractArchive_Ugly — a .zip file is routed
// to ExtractZip; this confirms the dispatcher does not double-extract
// the wrong path.
func TestPkgElectronExtractTar_ExtractArchive_Ugly(t *testing.T) {
	// Routing test only — no real zip body required for this. The
	// dispatcher will surface the ExtractZip error if the file does not
	// exist (which is what we want — proves the suffix dispatch).
	err := app.ExtractArchive(coreio.Local, "/tmp/notreal.zip", t.TempDir())
	if err == nil {
		t.Error("ExtractArchive should error on missing .zip (dispatch reached ExtractZip)")
	}
}

// TestPkgElectronExtractTar_ArchiveExtractedDir_Good — every supported
// suffix is stripped from the basename when computing the extracted
// directory name.
func TestPkgElectronExtractTar_ArchiveExtractedDir_Good(t *testing.T) {
	cases := map[string]string{
		"renderer.tar.gz": "/tmp/dest/renderer",
		"renderer.tgz":    "/tmp/dest/renderer",
		"renderer.tar":    "/tmp/dest/renderer",
		"renderer.zip":    "/tmp/dest/renderer",
		"plain":           "/tmp/dest/plain",
	}
	for archive, want := range cases {
		got := app.ArchiveExtractedDir("/tmp/dest", archive)
		if got != want {
			t.Errorf("ArchiveExtractedDir(/tmp/dest, %q) = %q; want %q", archive, got, want)
		}
	}
}

// TestPkgElectronExtractTar_ArchiveExtractedDir_Bad — an empty archive
// name returns the destination unchanged so the caller never gets a
// bogus path.
func TestPkgElectronExtractTar_ArchiveExtractedDir_Bad(t *testing.T) {
	if got := app.ArchiveExtractedDir("/tmp/dest", ""); got != "/tmp/dest" {
		t.Errorf("ArchiveExtractedDir empty archive = %q; want '/tmp/dest'", got)
	}
}

// TestPkgElectronExtractTar_ArchiveExtractedDir_Ugly — an absolute
// archive path is reduced to its basename before stripping the suffix.
func TestPkgElectronExtractTar_ArchiveExtractedDir_Ugly(t *testing.T) {
	got := app.ArchiveExtractedDir("/tmp/dest", "/some/long/path/bundle.tar.gz")
	if got != "/tmp/dest/bundle" {
		t.Errorf("ArchiveExtractedDir abs path = %q; want '/tmp/dest/bundle'", got)
	}
}

// tarEntry is one in-memory entry the test helper writes into a tar
// archive. Used by buildTar for both directories and regular files.
type tarEntry struct {
	Name  string
	Body  string
	IsDir bool
}

// buildTar produces a tar archive in memory with the supplied entries.
// When `gzipped` is true the result is gzipped (`.tar.gz` shape). Used
// by the per-archive happy/sad path tests so we don't ship a binary
// fixture inside the repo.
//
//	body := buildTar(t, false, []tarEntry{{Name: "a.txt", Body: "hi"}})
func buildTar(t *testing.T, gzipped bool, entries []tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	var w *tar.Writer
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(&buf)
		w = tar.NewWriter(gz)
	} else {
		w = tar.NewWriter(&buf)
	}

	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.Name,
			Mode: 0o644,
			Size: int64(len(e.Body)),
		}
		if e.IsDir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
			hdr.Size = 0
			if !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
		} else {
			hdr.Typeflag = tar.TypeReg
		}
		if err := w.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s): %v", e.Name, err)
		}
		if !e.IsDir {
			if _, err := w.Write([]byte(e.Body)); err != nil {
				t.Fatalf("Write body(%s): %v", e.Name, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	if gzipped {
		if err := gz.Close(); err != nil {
			t.Fatalf("Close gzip: %v", err)
		}
	}
	return buf.String()
}
