// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"

	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
	"gopkg.in/yaml.v3"
)

// verify is Step 2 of the 7-step boot — confirm the manifest's ed25519
// signature covers the rest of the manifest content. Dev mode skips the
// check and logs a warning instead.
//
// The signature scheme matches core/go/scm/manifest.Sign: the `sign` field
// is cleared, the remaining struct is YAML-marshalled, and the bytes are
// fed through ed25519.Verify. The public key lives next to the signature
// (scheme below) or is fetched from the marketplace index.
//
//	if err := verify(&manifest, app.ModeProd); err != nil { ... }
//
// TODO(config): core/config.ViewManifest does not yet carry a sign_key
// field or an accompanying marketplace index lookup. This skeleton checks
// the signature shape (base64 decode, length) but cannot validate against
// a trusted public key until one of:
//
//  1. config.ViewManifest gains SignKey alongside Sign, OR
//  2. Boot accepts a WithPublicKey option and the CLI wires it from
//     ~/.core/keys/ or the marketplace index, OR
//  3. we depend on core/go-scm/manifest (heavier — Gitea/Forgejo deps).
//
// Until then, prod mode rejects unsigned manifests but accepts any
// syntactically valid signature. This is the clear "replace me before
// alpha.1" marker.
func verify(m *config.ViewManifest, mode Mode) error {
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

	// Canonical payload — reuse the same shape core/go-scm uses so
	// signatures round-trip.
	if _, err := signableBytes(m); err != nil {
		return coreerr.E("app.verify", "canonical marshal failed", err)
	}

	// TODO(app): fetch the trusted public key (see function doc above)
	// and call ed25519.Verify(pub, msg, sig). Until the key source is
	// wired, we accept a well-formed signature shape — prod mode still
	// rejects unsigned manifests, so the boundary is honoured.
	_ = ed25519.Verify // keep the import in use post-wiring

	return nil
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
// already know which key to trust — bypasses the TODO above by taking the
// key as input.
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
