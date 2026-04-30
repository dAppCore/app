// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/hex"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// DefaultKeyName is the filename convention for the user's default app
// signing key. Matches RFC §3.2 (`core sign --key ~/.core/keys/app.key`).
//
//	priv, _ := app.LoadPrivateKey(coreio.Local, core.Path(homeKeyDir, app.DefaultKeyName))
const DefaultKeyName = "default.key"

// Sign re-signs a view.yaml manifest on disk and writes the result back
// to the same path. It reads, signs, re-marshals and overwrites — the
// canonical operation `core sign` performs in the CLI.
//
//	err := app.Sign(coreio.Local, ".core/view.yaml", priv)
//
// Rules:
//
//   - medium may be nil → falls back to coreio.Local.
//
//   - path must point to an existing YAML file; non-existence errors.
//
//   - priv must be an ed25519.PrivateKeySize slice (64 bytes). Anything
//     else is rejected before any filesystem write.
func Sign(
	medium coreio.Medium, path string, priv ed25519.PrivateKey,
) error {
	if medium == nil {
		medium = coreio.Local
	}
	if path == "" {
		return core.E("app.Sign", "empty manifest path", nil)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return core.E("app.Sign", "invalid private key length", nil)
	}

	if !medium.Exists(path) {
		return core.E("app.Sign", "manifest not found at "+path, nil)
	}

	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, path, &manifest); err != nil {
		return core.E("app.Sign", "parse "+path+" failed", err)
	}

	if err := signManifest(&manifest, priv); err != nil {
		return core.E("app.Sign", "signManifest failed", err)
	}

	out, err := yamlMarshalBytes(&manifest)
	if err != nil {
		return core.E("app.Sign", "marshal signed manifest failed", err)
	}
	if err := medium.Write(path, string(out)); err != nil {
		return core.E("app.Sign", "write "+path+" failed", err)
	}
	return nil
}

// DefaultPrivateKeyPath returns the conventional location of the user's
// default signing key — `$DIR_HOME/.core/keys/default.key`. Used by
// `core-app sign --sign-default` and the wrap CLI when no explicit
// `--key` flag is supplied.
//
//	path := app.DefaultPrivateKeyPath() // "/Users/me/.core/keys/default.key"
func DefaultPrivateKeyPath() string {
	home := core.Env("DIR_HOME")
	if home == "" {
		return ""
	}
	return core.Path(home, ".core", "keys", DefaultKeyName)
}

// LoadDefaultPrivateKey reads the user's default signing key from
// `$DIR_HOME/.core/keys/default.key`. Equivalent to calling
// LoadPrivateKey with DefaultPrivateKeyPath but produces a more
// helpful error when the user has not yet run `core-app keygen`.
//
//	priv, err := app.LoadDefaultPrivateKey(coreio.Local)
func LoadDefaultPrivateKey(medium coreio.Medium) (
	ed25519.PrivateKey, error,
) {
	path := DefaultPrivateKeyPath()
	if path == "" {
		return nil, core.E("app.LoadDefaultPrivateKey", "DIR_HOME is empty — cannot resolve default key", nil)
	}
	return loadDefaultPrivateKeyAtPath(medium, path)
}

// loadDefaultPrivateKeyAtPath is the filesystem-aware body shared by the
// DIR_HOME-based loader and the install-time "sign into this alternate
// home tree" path. The caller passes the fully-qualified default.key
// location it wants resolved.
func loadDefaultPrivateKeyAtPath(medium coreio.Medium, path string) (
	ed25519.PrivateKey, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if !medium.Exists(path) {
		return nil, core.E(
			"app.loadDefaultPrivateKeyAtPath",
			"default key not found — run `core-app keygen --dir "+
				core.PathDir(path)+" --name "+core.TrimSuffix(DefaultKeyName, ".key")+"`",
			nil,
		)
	}
	return LoadPrivateKey(medium, path)
}

// LoadPrivateKey reads a hex-encoded ed25519 private key from the
// supplied path. Matches the `.key` counterpart of the `.pub` trust files
// consumed by resolveTrustedKeys.
//
//	priv, err := app.LoadPrivateKey(coreio.Local, "~/.core/keys/app.key")
//
// The file may contain trailing whitespace (editors add newlines); it
// is trimmed before decoding.
func LoadPrivateKey(medium coreio.Medium, path string) (
	ed25519.PrivateKey, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if path == "" {
		return nil, core.E("app.LoadPrivateKey", "empty path", nil)
	}
	if !medium.Exists(path) {
		return nil, core.E("app.LoadPrivateKey", "private key not found at "+path, nil)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, core.E("app.LoadPrivateKey", "read "+path+" failed", err)
	}
	raw, err := hex.DecodeString(core.Trim(body))
	if err != nil {
		return nil, core.E("app.LoadPrivateKey", "decode hex failed", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, core.E("app.LoadPrivateKey", "wrong ed25519 private key size", nil)
	}
	return ed25519.PrivateKey(raw), nil
}

// WritePrivateKey persists a private key as hex on disk. Used by
// `core keygen` bootstrap — the paired public key is emitted by
// WritePublicKey to the corresponding `.pub` file.
//
//	_ = app.WritePrivateKey(coreio.Local, "~/.core/keys/app.key", priv)
func WritePrivateKey(
	medium coreio.Medium, path string, priv ed25519.PrivateKey,
) error {
	if medium == nil {
		medium = coreio.Local
	}
	if path == "" {
		return core.E("app.WritePrivateKey", "empty path", nil)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return core.E("app.WritePrivateKey", "invalid private key length", nil)
	}
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return core.E("app.WritePrivateKey", "ensure dir failed", err)
	}
	if err := medium.Write(path, hex.EncodeToString(priv)); err != nil {
		return core.E("app.WritePrivateKey", "write failed", err)
	}
	return nil
}

// WritePublicKey persists a public key as hex on disk so
// resolveTrustedKeys can pick it up in later boots.
//
//	_ = app.WritePublicKey(coreio.Local, "~/.core/keys/app.pub", pub)
func WritePublicKey(
	medium coreio.Medium, path string, pub ed25519.PublicKey,
) error {
	if medium == nil {
		medium = coreio.Local
	}
	if path == "" {
		return core.E("app.WritePublicKey", "empty path", nil)
	}
	if len(pub) != ed25519.PublicKeySize {
		return core.E("app.WritePublicKey", "invalid public key length", nil)
	}
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		return core.E("app.WritePublicKey", "ensure dir failed", err)
	}
	if err := medium.Write(path, hex.EncodeToString(pub)); err != nil {
		return core.E("app.WritePublicKey", "write failed", err)
	}
	return nil
}

// SignManifest signs a ViewManifest in memory — the canonical YAML
// bytes (with Sign cleared) are fed through ed25519.Sign and the
// base64-encoded signature is stapled into Sign. Same machinery Sign()
// uses against a file on disk; exposed publicly so packaging and
// wrapping paths can sign before writing.
//
//	err := app.SignManifest(&manifest, priv)
//
// The function mutates `m` in place (writes to m.Sign). Callers that
// need the signed bytes should YAML-marshal the manifest afterwards.
func SignManifest(
	m *config.ViewManifest, priv ed25519.PrivateKey,
) error {
	return signManifest(m, priv)
}

// Keygen generates a fresh ed25519 keypair and writes both halves to the
// supplied directory. Returns the absolute paths of the private and
// public key files. The RFC §3.2 default layout is
// `~/.core/keys/<name>.key` + `~/.core/keys/<name>.pub`.
//
//	priv, pub, err := app.Keygen(coreio.Local, keysDir, "app")
func Keygen(medium coreio.Medium, dir, name string) (
	privPath, pubPath string, err error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return "", "", core.E("app.Keygen", "empty directory", nil)
	}
	if name == "" {
		name = "default"
	}
	pub, priv, keyErr := ed25519.GenerateKey(nil)
	if keyErr != nil {
		return "", "", core.E("app.Keygen", "ed25519.GenerateKey failed", keyErr)
	}
	privPath = core.Path(dir, name+".key")
	pubPath = core.Path(dir, name+".pub")
	if err := WritePrivateKey(medium, privPath, priv); err != nil {
		return "", "", err
	}
	if err := WritePublicKey(medium, pubPath, pub); err != nil {
		return "", "", err
	}
	return privPath, pubPath, nil
}

// signManifestForHome signs a wrapped manifest with the default keypair
// rooted under `home/.core/keys/`. If the keypair does not exist yet,
// it is generated on demand so installed wrapped apps are immediately
// bootable in prod mode without a separate `keygen` pre-step.
func signManifestForHome(
	medium coreio.Medium, home string, manifest *config.ViewManifest,
) error {
	if manifest == nil {
		return core.E("app.signManifestForHome", "nil manifest", nil)
	}
	priv, err := ensureDefaultPrivateKey(medium, home)
	if err != nil {
		return core.E("app.signManifestForHome", "resolve default key failed", err)
	}
	if err := SignManifest(manifest, priv); err != nil {
		return core.E("app.signManifestForHome", "sign failed", err)
	}
	return nil
}

// ensureDefaultPrivateKey returns the user's default signing key under
// the supplied home tree, generating `default.key` + `default.pub` on
// first use. Wrapped installs call this so `pkg install` and
// marketplace-driven wraps produce signed manifests by default.
func ensureDefaultPrivateKey(medium coreio.Medium, home string) (
	ed25519.PrivateKey, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	privPath := defaultPrivateKeyPathForHome(home)
	if privPath == "" {
		return nil, core.E("app.ensureDefaultPrivateKey", "empty home directory", nil)
	}
	pubPath := defaultPublicKeyPathForHome(home)

	hasPriv := medium.Exists(privPath)
	hasPub := pubPath != "" && medium.Exists(pubPath)
	switch {
	case hasPriv:
		priv, err := loadDefaultPrivateKeyAtPath(medium, privPath)
		if err != nil {
			return nil, err
		}
		if !hasPub && pubPath != "" {
			pub, _ := priv.Public().(ed25519.PublicKey)
			if err := WritePublicKey(medium, pubPath, pub); err != nil {
				return nil, core.E("app.ensureDefaultPrivateKey", "write derived public key failed", err)
			}
		}
		return priv, nil
	case hasPub:
		return nil, core.E(
			"app.ensureDefaultPrivateKey",
			"default public key exists without matching private key at "+privPath,
			nil,
		)
	default:
		keysDir := core.PathDir(privPath)
		if _, _, err := Keygen(medium, keysDir, core.TrimSuffix(DefaultKeyName, ".key")); err != nil {
			return nil, core.E("app.ensureDefaultPrivateKey", "keygen failed", err)
		}
		return loadDefaultPrivateKeyAtPath(medium, privPath)
	}
}

func defaultPrivateKeyPathForHome(home string) string {
	home = core.Trim(home)
	if home == "" {
		home = core.Env("DIR_HOME")
	}
	if home == "" {
		return ""
	}
	return core.Path(home, ".core", "keys", DefaultKeyName)
}

func defaultPublicKeyPathForHome(home string) string {
	privPath := defaultPrivateKeyPathForHome(home)
	if privPath == "" {
		return ""
	}
	return core.Path(core.PathDir(privPath), core.TrimSuffix(DefaultKeyName, ".key")+".pub")
}
