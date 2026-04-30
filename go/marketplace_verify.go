// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// VerifyListing checks that the manifest installed at `dest` from the
// supplied marketplace listing matches the listing's `sign_key`. The
// install pipeline calls this immediately after a `git clone` so the
// user never runs untrusted code (RFC §6.3 — "Signature re-verified
// after pull").
//
//	err := app.VerifyListing(coreio.Local, dest, listing)
//
// Rules:
//
//   - A listing with an empty `sign_key` is considered un-pinned —
//     verification falls back to the trust-keys directory if any keys
//     are loaded; otherwise the manifest's own signature is the only
//     boundary and prod-mode boots will fail at Step 2 anyway.
//
//   - A non-existent .core/view.yaml at `dest` errors — the install
//     itself was incomplete.
//
//   - A listing key + signature mismatch errors with the listing code
//     so the operator can match the rejection back to the marketplace
//     entry.
func VerifyListing(
	medium coreio.Medium, dest string, listing *MarketplaceListing,
) error {
	if listing == nil {
		return core.E("app.VerifyListing", "nil listing", nil)
	}
	if dest == "" {
		return core.E("app.VerifyListing", "empty dest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}

	manifestPath := core.Path(dest, ".core", "view.yaml")
	if !medium.Exists(manifestPath) {
		return core.E(
			"app.VerifyListing",
			"installed manifest missing for listing "+listing.Code+": "+manifestPath,
			nil,
		)
	}

	var manifest config.ViewManifest
	if err := LoadViewManifest(medium, manifestPath, &manifest); err != nil {
		return core.E(
			"app.VerifyListing",
			"parse installed manifest for "+listing.Code+" failed",
			err,
		)
	}

	// Listings without a pinned key delegate trust to the user's
	// keyring; the regular Boot pipeline (Step 2) will surface a
	// rejection if the manifest is signed but no key matches.
	if listing.SignKey == "" {
		return nil
	}

	pub, err := parsePublicKey(listing.SignKey)
	if err != nil {
		return core.E(
			"app.VerifyListing",
			"listing "+listing.Code+" sign_key decode failed",
			err,
		)
	}

	if err := verifyWithKey(&manifest, pub); err != nil {
		return core.E(
			"app.VerifyListing",
			"listing "+listing.Code+" signature did not match pinned sign_key",
			err,
		)
	}
	return nil
}

// VerifyListingBytes is the in-memory counterpart to VerifyListing —
// useful when the caller already has the manifest bytes (e.g. PWA
// wrap path) and wants to confirm they match a pinned sign_key
// without writing to disk first.
//
//	err := app.VerifyListingBytes(yamlBody, listing.SignKey)
func VerifyListingBytes(
	body []byte, hexKey string,
) error {
	if hexKey == "" {
		return nil
	}
	if len(body) == 0 {
		return core.E("app.VerifyListingBytes", "empty manifest body", nil)
	}
	pub, err := parsePublicKey(hexKey)
	if err != nil {
		return core.E("app.VerifyListingBytes", "sign_key decode failed", err)
	}
	var manifest config.ViewManifest
	if err := yamlUnmarshal(body, &manifest); err != nil {
		return core.E("app.VerifyListingBytes", "parse manifest body failed", err)
	}
	if err := verifyWithKey(&manifest, pub); err != nil {
		return core.E("app.VerifyListingBytes", "signature did not match pinned sign_key", err)
	}
	return nil
}

// SignListingKey converts an ed25519 public key into the hex form
// stored in `MarketplaceListing.SignKey`. Mirrors the convention used
// by the pub-file format (`*.pub` files in `$DIR_HOME/.core/keys/`).
//
//	listing.SignKey = app.SignListingKey(pub)
func SignListingKey(pub ed25519.PublicKey) string {
	if len(pub) != ed25519.PublicKeySize {
		return ""
	}
	return hexEncode(pub)
}

// hexEncode is a thin wrapper over encoding/hex.EncodeToString. Kept
// as a single shim so all marketplace key handling routes through one
// helper (matches the style used in sign.go / verify.go).
//
//	hex := hexEncode(pub)
func hexEncode(b []byte) string {
	const lookup = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = lookup[v>>4]
		out[i*2+1] = lookup[v&0x0f]
	}
	return string(out)
}

// yamlUnmarshal is the package-internal entry point for YAML decoding
// of a manifest body. Kept here (rather than calling yaml.Unmarshal
// directly) so the encoder swap is mechanical — change this and every
// marketplace verify path follows.
//
//	_ = yamlUnmarshal(body, &manifest)
func yamlUnmarshal(
	body []byte, dst any,
) error {
	if manifest, ok := dst.(*config.ViewManifest); ok {
		return UnmarshalViewManifest(body, manifest)
	}
	return yamlUnmarshalImpl(body, dst)
}
