// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"archive/tar"
	"compress/gzip"
	"io"

	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// ExtractTar unpacks a downloaded `.tar` (uncompressed) or `.tar.gz` /
// `.tgz` archive into `dest`. The detection is by extension — callers
// pass the path the wrapper wrote to disk, the function picks the
// right reader based on suffix.
//
//	err := app.ExtractTar(coreio.Local, "renderer.tar.gz", "/tmp/scratch")
//
// Rules:
//
//   - Empty archive / dest → typed error.
//
//   - Entries that resolve to a path outside `dest` are rejected (zip-
//     slip equivalent — same SASE rule go-io enforces at the medium
//     boundary).
//
//   - Symlink entries are skipped (Electron renderer bundles never need
//     them; honouring them would let a malicious archive escape).
//
//   - Existing files at the destination are overwritten — re-extracting
//     after an upstream release update should not fail because an old
//     copy is in the way.
func ExtractTar(medium coreio.Medium, archive, dest string) error {
	if medium == nil {
		medium = coreio.Local
	}
	if archive == "" {
		return coreerr.E("app.ExtractTar", "empty archive path", nil)
	}
	if dest == "" {
		return coreerr.E("app.ExtractTar", "empty dest path", nil)
	}
	if !medium.Exists(archive) {
		return coreerr.E("app.ExtractTar", "archive not found: "+archive, nil)
	}
	if err := medium.EnsureDir(dest); err != nil {
		return coreerr.E("app.ExtractTar", "ensure dest dir failed", err)
	}

	body, err := medium.Read(archive)
	if err != nil {
		return coreerr.E("app.ExtractTar", "read archive failed", err)
	}
	if body == "" {
		return coreerr.E("app.ExtractTar", "archive is empty: "+archive, nil)
	}

	reader, err := openTarReader(archive, body)
	if err != nil {
		return coreerr.E("app.ExtractTar", "open tar reader failed", err)
	}

	for {
		hdr, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return coreerr.E("app.ExtractTar", "read tar header failed", err)
		}
		if err := extractTarEntry(medium, reader, hdr, dest); err != nil {
			return err
		}
	}
	return nil
}

// openTarReader returns a *tar.Reader wrapping the archive bytes. Picks
// gzip decompression when the archive name ends in `.gz` / `.tgz`,
// otherwise returns a plain tar reader. The function does NOT inspect
// the magic bytes — callers pass real filenames so a name-based check
// is sufficient and avoids buffering the whole archive twice.
//
//	r, err := openTarReader("renderer.tar.gz", body)
func openTarReader(archive, body string) (*tar.Reader, error) {
	low := core.Lower(archive)
	// stringReaderAt also satisfies io.Reader through a thin wrapper:
	// the local readSeekCloser exposes ReadAt + Read so both the gzip
	// and tar layers can stream without buffering twice.
	var src io.Reader = newStringReader(body)
	if core.HasSuffix(low, ".gz") || core.HasSuffix(low, ".tgz") {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return nil, err
		}
		src = gz
	}
	return tar.NewReader(src), nil
}

// stringReader wraps a string body with an io.Reader implementation so
// tar/gzip layers can stream the bytes without depending on the banned
// `strings` package. Mirrors the stringReaderAt helper in
// pkg_electron_extract.go for the zip path.
type stringReader struct {
	body string
	off  int
}

// newStringReader returns a fresh reader positioned at the start of
// `body`. Single allocator — the consumer reads forward only.
//
//	r := newStringReader(body)
func newStringReader(body string) *stringReader {
	return &stringReader{body: body}
}

// Read implements io.Reader against the string body.
//
//	r := newStringReader("hello")
//	buf := make([]byte, 3)
//	n, err := r.Read(buf) // n=3, err=nil
func (r *stringReader) Read(p []byte) (int, error) {
	if r == nil || r.off >= len(r.body) {
		return 0, io.EOF
	}
	n := copy(p, r.body[r.off:])
	r.off += n
	return n, nil
}

// extractTarEntry handles a single tar header — directory creation,
// regular file write, traversal-attack rejection. Pulled out of
// ExtractTar so the loop reads cleanly.
//
//	if err := extractTarEntry(medium, reader, hdr, dest); err != nil { return err }
func extractTarEntry(medium coreio.Medium, reader *tar.Reader, hdr *tar.Header, dest string) error {
	if hdr == nil {
		return nil
	}
	name := hdr.Name
	if name == "" {
		return nil
	}
	// Reject absolute paths and parent traversals — the joined target
	// must stay inside `dest`.
	if core.PathIsAbs(name) || core.HasPrefix(name, "..") || core.Contains(name, "../") {
		return coreerr.E(
			"app.ExtractTar",
			"refusing tar entry outside dest: "+name,
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
		return coreerr.E(
			"app.ExtractTar",
			"refusing tar entry outside dest after join: "+name,
			nil,
		)
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := medium.EnsureDir(target); err != nil {
			return coreerr.E("app.ExtractTar", "ensure dir failed: "+target, err)
		}
		return nil
	case tar.TypeReg, tar.TypeRegA:
		if err := medium.EnsureDir(core.PathDir(target)); err != nil {
			return coreerr.E("app.ExtractTar", "ensure parent dir failed: "+target, err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			return coreerr.E("app.ExtractTar", "read entry failed: "+name, err)
		}
		if err := medium.Write(target, string(body)); err != nil {
			return coreerr.E("app.ExtractTar", "write entry failed: "+target, err)
		}
		return nil
	default:
		// Symlinks, hard links, FIFOs, character/block devices — skipped.
		// An Electron renderer bundle never needs them and honouring them
		// would create attack surface.
		return nil
	}
}

// ExtractArchive picks ExtractZip or ExtractTar based on the file
// extension. Convenience wrapper for the wrap / install paths so they
// don't need to switch on suffix at every call site.
//
//	err := app.ExtractArchive(coreio.Local, "renderer.tgz", "/tmp/scratch")
//
// Supported suffixes: `.zip`, `.tar`, `.tar.gz`, `.tgz`. Anything else
// is rejected with a typed error so an unknown format surfaces at the
// caller rather than silently no-oping.
func ExtractArchive(medium coreio.Medium, archive, dest string) error {
	if archive == "" {
		return coreerr.E("app.ExtractArchive", "empty archive path", nil)
	}
	low := core.Lower(archive)
	switch {
	case core.HasSuffix(low, ".zip"):
		return ExtractZip(medium, archive, dest)
	case core.HasSuffix(low, ".tar.gz"),
		core.HasSuffix(low, ".tgz"),
		core.HasSuffix(low, ".tar"):
		return ExtractTar(medium, archive, dest)
	}
	return coreerr.E(
		"app.ExtractArchive",
		"unsupported archive format: "+archive+" (supported: .zip, .tar, .tar.gz, .tgz)",
		nil,
	)
}

// ArchiveExtractedDir returns the path the archive contents will land in
// when ExtractArchive is run with the same `dest`. Convenience helper for
// CLI logging — the wrapper logs "extracted to <dir>" after extraction
// so the agent knows exactly where to point the next command.
//
//	dir := app.ArchiveExtractedDir("/tmp/scratch", "renderer.tar.gz")
//	// "/tmp/scratch/renderer"
//
// The returned path appends the archive basename (with archive suffix
// stripped) so multiple archives can share a `dest` without colliding.
func ArchiveExtractedDir(dest, archive string) string {
	if archive == "" {
		return dest
	}
	base := core.PathBase(archive)
	low := core.Lower(base)
	for _, suffix := range []string{".tar.gz", ".tgz", ".tar", ".zip"} {
		if core.HasSuffix(low, suffix) {
			return core.Path(dest, core.TrimSuffix(base, base[len(base)-len(suffix):]))
		}
	}
	return core.Path(dest, base)
}
