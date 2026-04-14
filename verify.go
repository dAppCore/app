// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
	"gopkg.in/yaml.v3"
)

// verify is Step 2 of the 7-step boot — confirm the manifest's ed25519
// signature covers the rest of the manifest content. Dev mode skips the
// check and logs a warning instead.
//
// The signature scheme matches core/go/scm/manifest.Sign: the `sign`
// field is cleared, the remaining struct is YAML-marshalled, and the
// bytes are fed through ed25519.Verify against any of the supplied
// trusted keys. A single matching key is sufficient.
//
//	err := verify(&manifest, app.ModeProd, trustedKeys)
//
// Rules:
//
//   - ModeDev: never fails (warning state).
//
//   - ModeProd + no Sign: rejected ("unsigned manifest").
//
//   - ModeProd + Sign + no trusted keys: rejected (no trust roots = no
//     trust — the operator must supply a key via WithPublicKey or drop a
//     `.pub` file into `$DIR_HOME/.core/keys/`).
//
//   - ModeProd + Sign + trusted keys: accepts the manifest iff any key
//     validates the signature.
func verify(m *config.ViewManifest, mode Mode, trusted []ed25519.PublicKey) error {
	if m == nil {
		return coreerr.E("app.verify", "nil manifest", nil)
	}

	if mode == ModeDev {
		// Development mode — no signature required. Loud enough to spot
		// in a log sweep but not fatal.
		return nil
	}

	if m.Sign == "" {
		return coreerr.E("app.verify", "unsigned manifest rejected in prod mode", nil)
	}

	// Decode the signature so a malformed base64 fails fast.
	sig, err := base64.StdEncoding.DecodeString(m.Sign)
	if err != nil {
		return coreerr.E("app.verify", "decode signature failed", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return coreerr.E("app.verify", "signature is not an ed25519 signature", nil)
	}

	msg, err := signableBytes(m)
	if err != nil {
		return coreerr.E("app.verify", "canonical marshal failed", err)
	}

	if len(trusted) == 0 {
		return coreerr.E(
			"app.verify",
			"no trusted keys available — set WithPublicKey or populate $DIR_HOME/.core/keys/",
			nil,
		)
	}

	for _, pub := range trusted {
		if len(pub) != ed25519.PublicKeySize {
			continue // skip malformed entries rather than fail the whole boot
		}
		if ed25519.Verify(pub, msg, sig) {
			return nil
		}
	}

	return coreerr.E(
		"app.verify",
		"signature does not match any trusted key ("+core.Sprint(len(trusted))+" checked)",
		nil,
	)
}

// signableBytes returns the canonical YAML bytes that a signature covers
// (the manifest with sign cleared).
//
//	msg, _ := signableBytes(&manifest)
func signableBytes(m *config.ViewManifest) ([]byte, error) {
	tmp := *m
	tmp.Sign = ""
	return yaml.Marshal(&tmp)
}

// verifyWithKey runs the full ed25519 verification against an explicitly
// provided public key. Used by callers (CLI, marketplace, tests) that
// already know which key to trust.
//
//	pub, _ := hex.DecodeString(indexEntry.SignKey)
//	err := verifyWithKey(&manifest, pub)
func verifyWithKey(m *config.ViewManifest, pub ed25519.PublicKey) error {
	if m == nil {
		return coreerr.E("app.verifyWithKey", "nil manifest", nil)
	}
	if m.Sign == "" {
		return coreerr.E("app.verifyWithKey", "no signature present", nil)
	}
	if len(pub) != ed25519.PublicKeySize {
		return coreerr.E("app.verifyWithKey", "invalid public key length", nil)
	}

	sig, err := base64.StdEncoding.DecodeString(m.Sign)
	if err != nil {
		return coreerr.E("app.verifyWithKey", "decode signature failed", err)
	}
	msg, err := signableBytes(m)
	if err != nil {
		return coreerr.E("app.verifyWithKey", "canonical marshal failed", err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return coreerr.E("app.verifyWithKey", "signature does not match manifest content", nil)
	}
	return nil
}

// parsePublicKey decodes a hex-encoded ed25519 public key. Kept here
// (rather than in a cli package) so Boot callers composing their own key
// fetch can share the helper.
//
//	pub, err := parsePublicKey("3b6a…")
func parsePublicKey(hexKey string) (ed25519.PublicKey, error) {
	if hexKey == "" {
		return nil, coreerr.E("app.parsePublicKey", "empty key", nil)
	}
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, coreerr.E("app.parsePublicKey", "decode hex failed", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, coreerr.E("app.parsePublicKey", "wrong ed25519 public key size", nil)
	}
	return ed25519.PublicKey(raw), nil
}

// signManifest is the signer-side counterpart to verify — sign the
// canonical manifest bytes with the caller's private key and stamp the
// base64-encoded signature into the manifest.Sign field. Used by tests
// and by whichever CLI eventually owns `core sign`.
//
//	ed25519Priv, _ := ed25519.GenerateKey(nil) // keep the priv secure
//	_ = signManifest(&manifest, priv)
func signManifest(m *config.ViewManifest, priv ed25519.PrivateKey) error {
	if m == nil {
		return coreerr.E("app.signManifest", "nil manifest", nil)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return coreerr.E("app.signManifest", "invalid private key length", nil)
	}
	msg, err := signableBytes(m)
	if err != nil {
		return coreerr.E("app.signManifest", "canonical marshal failed", err)
	}
	m.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
	return nil
}

// resolveTrustedKeys gathers every public key the verify step should
// trust. Sources (in order):
//
//  1. `Options.PublicKeyHex` — explicit key passed by the caller.
//
//  2. `Options.TrustedKeysDir` — every `*.pub` file in the directory
//     (one hex-encoded key per file, trimmed).
//
// Returns an empty slice (not an error) when no keys resolve — the
// caller decides what that means given the mode.
//
//	trusted, err := resolveTrustedKeys(o)
//
// Errors surface malformed keys, not missing directories; a missing
// directory is treated as "no keys here" so a fresh install still boots
// dev mode and only prod mode complains.
func resolveTrustedKeys(o Options) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey

	if o.PublicKeyHex != "" {
		pub, err := parsePublicKey(o.PublicKeyHex)
		if err != nil {
			return nil, coreerr.E("app.resolveTrustedKeys", "explicit key decode failed", err)
		}
		keys = append(keys, pub)
	}

	if o.DisableKeyLoad || o.TrustedKeysDir == "" {
		return keys, nil
	}

	medium := o.Medium
	if medium == nil {
		medium = coreio.Local
	}
	if !medium.IsDir(o.TrustedKeysDir) {
		return keys, nil
	}

	entries, err := medium.List(o.TrustedKeysDir)
	if err != nil {
		return nil, coreerr.E("app.resolveTrustedKeys", "list trusted keys dir failed", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !core.HasSuffix(name, ".pub") {
			continue
		}
		path := core.Path(o.TrustedKeysDir, name)
		body, err := medium.Read(path)
		if err != nil {
			return nil, coreerr.E("app.resolveTrustedKeys", "read key "+path+" failed", err)
		}
		pub, err := parsePublicKey(core.Trim(body))
		if err != nil {
			return nil, coreerr.E("app.resolveTrustedKeys", "decode key "+path+" failed", err)
		}
		keys = append(keys, pub)
	}
	return keys, nil
}
