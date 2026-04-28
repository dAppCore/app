// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"archive/zip"
	"bytes"
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"testing"
)

// TestPkgElectronExtract_ExtractZip_Good — a well-formed zip archive
// with regular files unpacks cleanly into the destination.
func TestPkgElectronExtract_ExtractZip_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	// Build a minimal renderer-shaped zip in memory.
	body, err := buildZip(map[string]string{
		"index.html": "<html><body>hi</body></html>",
		"assets/js":  "console.log('hi');",
		"assets/css": "body { margin: 0; }",
	})
	if err != nil {
		t.Fatalf("buildZip: %v", err)
	}

	archivePath := core.Path(dir, "renderer.zip")
	if err := medium.Write(archivePath, body); err != nil {
		t.Fatalf("Write archive: %v", err)
	}

	dest := core.Path(dir, "renderer-out")
	if err := ExtractZip(medium, archivePath, dest); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	for _, want := range []struct {
		path, body string
	}{
		{core.Path(dest, "index.html"), "<html><body>hi</body></html>"},
		{core.Path(dest, "assets", "js"), "console.log('hi');"},
		{core.Path(dest, "assets", "css"), "body { margin: 0; }"},
	} {
		got, err := medium.Read(want.path)
		if err != nil {
			t.Errorf("read %q: %v", want.path, err)
			continue
		}
		if got != want.body {
			t.Errorf("body at %q = %q; want %q", want.path, got, want.body)
		}
	}
}

// TestPkgElectronExtract_ExtractZip_Bad — empty inputs and missing
// archives surface typed errors.
func TestPkgElectronExtract_ExtractZip_Bad(t *testing.T) {
	medium := coreio.Local

	if err := ExtractZip(medium, "", t.TempDir()); err == nil {
		t.Error("empty archive path should error")
	}
	if err := ExtractZip(medium, t.TempDir()+"/nope.zip", t.TempDir()); err == nil {
		t.Error("missing archive should error")
	}

	// An empty file is not a valid archive.
	dir := t.TempDir()
	emptyPath := core.Path(dir, "empty.zip")
	if err := medium.Write(emptyPath, ""); err != nil {
		t.Fatalf("Write empty: %v", err)
	}
	if err := ExtractZip(medium, emptyPath, t.TempDir()); err == nil {
		t.Error("empty archive body should error")
	}

	// Garbage bytes are not a valid archive either.
	junkPath := core.Path(dir, "junk.zip")
	if err := medium.Write(junkPath, "not a zip file"); err != nil {
		t.Fatalf("Write junk: %v", err)
	}
	if err := ExtractZip(medium, junkPath, t.TempDir()); err == nil {
		t.Error("garbage archive body should error")
	}
}

// TestPkgElectronExtract_ExtractZip_Ugly — entries that try to escape
// the destination via parent traversals or absolute paths are
// rejected (zip-slip defence).
func TestPkgElectronExtract_ExtractZip_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	body, err := buildZip(map[string]string{
		"../escape.txt": "naughty",
	})
	if err != nil {
		t.Fatalf("buildZip: %v", err)
	}
	archivePath := core.Path(dir, "evil.zip")
	if err := medium.Write(archivePath, body); err != nil {
		t.Fatalf("Write evil: %v", err)
	}

	dest := core.Path(dir, "out")
	if err := ExtractZip(medium, archivePath, dest); err == nil {
		t.Error("ExtractZip should reject ../ traversal")
	}
}

func TestPkgElectronExtract_ReaderAt_ReadAt_Good(t *testing.T) {
	reader := stringReaderAt("renderer")
	buf := make([]byte, 4)
	n, err := reader.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 4 || string(buf) != "rend" {
		t.Fatalf("ReadAt = (%d,%q); want (4,rend)", n, string(buf))
	}
}

func TestPkgElectronExtract_ReaderAt_ReadAt_Bad(t *testing.T) {
	reader := stringReaderAt("renderer")
	buf := make([]byte, 4)
	n, err := reader.ReadAt(buf, int64(len(reader)))
	if err == nil || n != 0 {
		t.Fatalf("ReadAt at EOF = (%d,%v); want 0,EOF", n, err)
	}
}

func TestPkgElectronExtract_ReaderAt_ReadAt_Ugly(t *testing.T) {
	reader := stringReaderAt("go")
	buf := make([]byte, 4)
	n, err := reader.ReadAt(buf, 1)
	if err == nil || n != 1 || string(buf[:n]) != "o" {
		t.Fatalf("short ReadAt = (%d,%q,%v); want 1,o,EOF", n, string(buf[:n]), err)
	}
}

// buildZip is a tiny helper that turns a map[name]body into an
// in-memory zip archive body. Exists so the zip-extraction tests
// don't depend on a fixture committed to git.
//
//	body, _ := buildZip(map[string]string{"index.html": "<html/>"})
func buildZip(files map[string]string) (string, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Create(name)
		if err != nil {
			_ = w.Close()
			return "", err
		}
		if _, err := fw.Write([]byte(body)); err != nil {
			_ = w.Close()
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}
