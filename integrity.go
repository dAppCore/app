// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sort"

	core "dappco.re/go/core"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

const manifestAssetHashKey = "asset_hash"

// bindWrappedAssetHash records a deterministic hash of the staged asset
// tree into the manifest before signing. Wrapped Electron and PWA
// installs both materialise runtime assets alongside the generated
// manifest, so the final on-disk tree must be covered by the
// signature-bound hash.
func bindWrappedAssetHash(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if manifest == nil {
		return coreerr.E("app.bindWrappedAssetHash", "nil manifest", nil)
	}
	switch packageTypeFromManifest(manifest) {
	case PackageTypeElectron, PackageTypePWA:
	default:
		setManifestAssetHash(manifest, "")
		return nil
	}
	if medium == nil {
		medium = coreio.Local
	}
	if !medium.IsDir(dest) {
		setManifestAssetHash(manifest, "")
		return nil
	}

	sum, files, err := assetTreeHash(medium, dest)
	if err != nil {
		return coreerr.E("app.bindWrappedAssetHash", "asset tree hash failed", err)
	}
	if files == 0 {
		setManifestAssetHash(manifest, "")
		return nil
	}
	setManifestAssetHash(manifest, sum)
	return nil
}

// verifyAssetIntegrity checks the installed asset tree against the
// signed hash recorded in the manifest. When no asset hash is present
// the check is skipped for backwards compatibility with older wraps.
func verifyAssetIntegrity(medium coreio.Medium, root string, manifest *config.ViewManifest, mode Mode) error {
	if manifest == nil {
		return coreerr.E("app.verifyAssetIntegrity", "nil manifest", nil)
	}
	if mode == ModeDev {
		return nil
	}
	want := manifestAssetHash(manifest)
	if want == "" {
		return nil
	}
	sum, _, err := assetTreeHash(medium, root)
	if err != nil {
		return coreerr.E("app.verifyAssetIntegrity", "asset tree hash failed", err)
	}
	if sum != want {
		return coreerr.E(
			"app.verifyAssetIntegrity",
			"asset tree hash mismatch for "+manifest.Code,
			nil,
		)
	}
	return nil
}

func manifestAssetHash(m *config.ViewManifest) string {
	if m == nil || m.Config == nil {
		return ""
	}
	hash, _ := m.Config[manifestAssetHashKey].(string)
	return core.Trim(hash)
}

func setManifestAssetHash(m *config.ViewManifest, hash string) {
	if m == nil {
		return
	}
	hash = core.Trim(hash)
	if hash == "" {
		if m.Config != nil {
			delete(m.Config, manifestAssetHashKey)
		}
		return
	}
	if m.Config == nil {
		m.Config = map[string]any{}
	}
	m.Config[manifestAssetHashKey] = hash
}

func packageTypeFromManifest(m *config.ViewManifest) PackageType {
	if m == nil || m.Config == nil {
		return PackageTypeUnknown
	}
	raw, _ := m.Config["type"].(string)
	return ParsePackageType(raw)
}

func assetTreeHash(medium coreio.Medium, root string) (string, int, error) {
	if medium == nil {
		medium = coreio.Local
	}
	if root == "" {
		return "", 0, coreerr.E("app.assetTreeHash", "empty root", nil)
	}
	if !medium.IsDir(root) {
		return "", 0, coreerr.E("app.assetTreeHash", "root is not a directory: "+root, nil)
	}

	hasher := sha256.New()
	files, err := hashAssetDir(medium, hasher, root, root)
	if err != nil {
		return "", files, coreerr.E("app.assetTreeHash", "hash walk failed", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), files, nil
}

func hashAssetDir(medium coreio.Medium, hasher hash.Hash, root, dir string) (int, error) {
	entries, err := medium.List(dir)
	if err != nil {
		return 0, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}
		if entry.IsDir() && name == ".core" {
			continue
		}

		path := core.Path(dir, name)
		rel := assetHashRelativePath(root, path)
		if entry.IsDir() {
			writeAssetHashChunk(hasher, "dir", rel, "")
			n, err := hashAssetDir(medium, hasher, root, path)
			count += n
			if err != nil {
				return count, err
			}
			continue
		}

		body, err := medium.Read(path)
		if err != nil {
			return count, err
		}
		writeAssetHashChunk(hasher, "file", rel, body)
		count++
	}
	return count, nil
}

func writeAssetHashChunk(hasher hash.Hash, kind, rel, body string) {
	_, _ = hasher.Write([]byte(kind))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(rel))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(core.Sprint(len(body))))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(body))
	_, _ = hasher.Write([]byte{0})
}

func assetHashRelativePath(root, path string) string {
	rel := path
	if core.HasPrefix(rel, root) {
		rel = rel[len(root):]
	}
	rel = core.TrimPrefix(rel, "/")
	rel = core.TrimPrefix(rel, "\\")
	return core.Replace(rel, "\\", "/")
}
