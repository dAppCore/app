// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"archive/zip"
	"io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

// ExtractZip unpacks a downloaded zip archive into `dest`. Used by
// `core pkg wrap --electron <repo>` after DownloadAsset writes the
// renderer bundle so the wrapper can scan the unpacked contents
// without forcing the user to run a second command.
//
//	err := app.ExtractZip(coreio.Local, "renderer.zip", "/tmp/scratch")
//
// Rules:
//
//   - Empty path / dest → typed error.
//
//   - Entries that resolve to a path outside `dest` are rejected ("zip
//     slip" defence — the same SASE rule go-io enforces at the medium
//     boundary).
//
//   - Symlinks inside the archive are skipped (Electron renderer
//     bundles never need them; honouring them would be a footgun).
//
//   - Existing files at the destination are overwritten — re-extracting
//     after an upstream release update should not fail because an old
//     copy is in the way.
func ExtractZip(medium coreio.Medium, archive, dest string) error {
	if medium == nil {
		medium = coreio.Local
	}
	if archive == "" {
		return coreerr.E("app.ExtractZip", "empty archive path", nil)
	}
	if dest == "" {
		return coreerr.E("app.ExtractZip", "empty dest path", nil)
	}
	if !medium.Exists(archive) {
		return coreerr.E("app.ExtractZip", "archive not found: "+archive, nil)
	}
	if err := medium.EnsureDir(dest); err != nil {
		return coreerr.E("app.ExtractZip", "ensure dest dir failed", err)
	}

	body, err := medium.Read(archive)
	if err != nil {
		return coreerr.E("app.ExtractZip", "read archive failed", err)
	}
	if body == "" {
		return coreerr.E("app.ExtractZip", "archive is empty: "+archive, nil)
	}

	r, err := zip.NewReader(stringReaderAt(body), int64(len(body)))
	if err != nil {
		return coreerr.E("app.ExtractZip", "parse zip failed", err)
	}

	for _, f := range r.File {
		if err := extractZipEntry(medium, f, dest); err != nil {
			return err
		}
	}
	return nil
}

// extractZipEntry handles a single zip entry — directory creation,
// regular file write, traversal-attack rejection. Pulled out of
// ExtractZip so the loop reads cleanly and the per-entry error is
// surfaced with the entry name in scope.
//
//	if err := extractZipEntry(medium, f, dest); err != nil { return err }
func extractZipEntry(medium coreio.Medium, f *zip.File, dest string) error {
	if f == nil {
		return nil
	}
	name := f.Name
	if name == "" {
		return nil
	}
	// Skip absolute paths and parent traversals — the joined path must
	// stay within `dest`.
	if core.PathIsAbs(name) || core.HasPrefix(name, "..") || core.Contains(name, "../") {
		return coreerr.E(
			"app.ExtractZip",
			"refusing zip entry outside dest: "+name,
			nil,
		)
	}
	target := core.Path(dest, name)
	cleanDest := core.Path(dest)
	sep := core.Env("DS")
	if sep == "" {
		sep = "/"
	}
	if !core.HasPrefix(target, cleanDest+sep) && target != cleanDest {
		// Belt-and-braces — the previous PathIsAbs / `..` checks should
		// catch this, but verify the resolved path before any write.
		return coreerr.E(
			"app.ExtractZip",
			"refusing zip entry outside dest after join: "+name,
			nil,
		)
	}

	// Directory entries end with '/'. Create and continue.
	if f.FileInfo().IsDir() {
		if err := medium.EnsureDir(target); err != nil {
			return coreerr.E("app.ExtractZip", "ensure dir failed: "+target, err)
		}
		return nil
	}

	// Skip symlinks and other non-regular entries — Electron renderer
	// bundles do not need them and honouring them would let an attacker
	// point a link outside the dest.
	if !f.FileInfo().Mode().IsRegular() {
		return nil
	}

	if err := medium.EnsureDir(core.PathDir(target)); err != nil {
		return coreerr.E("app.ExtractZip", "ensure parent dir failed: "+target, err)
	}

	rc, err := f.Open()
	if err != nil {
		return coreerr.E("app.ExtractZip", "open entry failed: "+name, err)
	}
	defer func() { _ = rc.Close() }()

	body, err := io.ReadAll(rc)
	if err != nil {
		return coreerr.E("app.ExtractZip", "read entry failed: "+name, err)
	}
	if err := medium.Write(target, string(body)); err != nil {
		return coreerr.E("app.ExtractZip", "write entry failed: "+target, err)
	}
	return nil
}

// stringReaderAt adapts a string into the io.ReaderAt interface
// archive/zip needs. Avoids importing bytes.NewReader (the wrapper
// accepts a []byte but the package convention is to keep stdlib
// imports consolidated here).
type stringReaderAt string

// ReadAt implements io.ReaderAt against a string body.
//
//	r := stringReaderAt("body")
//	n, err := r.ReadAt(buf, 0)
func (s stringReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(s)) {
		return 0, io.EOF
	}
	n := copy(p, s[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
