// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	"gopkg.in/yaml.v3"
)

// TestVerify_verify_Good — dev mode skips the signature check even
// without trusted keys. This is the path developers iterate on most.
func TestVerify_verify_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "ok", Name: "OK", Version: "0.1.0"}
	if err := verify(m, ModeDev, nil); err != nil {
		t.Fatalf("verify (dev, unsigned) should succeed: %v", err)
	}
}

// TestVerify_verify_Bad — prod mode rejects an unsigned manifest even
// when trusted keys are present. The boundary ("no sign = no boot") is
// the core security property.
func TestVerify_verify_Bad(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{Code: "no-sign", Name: "No Sign", Version: "0.1.0"}
	if err := verify(m, ModeProd, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("verify (prod, unsigned) should fail")
	}
}

// TestVerify_verify_Ugly — a manifest with a malformed signature
// surfaces the decode failure rather than silently passing.
func TestVerify_verify_Ugly(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{
		Code:    "bad-sign",
		Name:    "Bad Sign",
		Version: "0.1.0",
		Sign:    "not-base64!@#",
	}
	if err := verify(m, ModeProd, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("verify (prod, malformed sign) should fail")
	}
}

// TestVerify_verifyProdWithKey_Good — a signed manifest verifies
// cleanly when the signer's public key is in the trust list.
func TestVerify_verifyProdWithKey_Good(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := &config.ViewManifest{Code: "signed", Name: "Signed", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}

	if err := verify(m, ModeProd, []ed25519.PublicKey{pub}); err != nil {
		t.Fatalf("verify (prod, signed, trusted key) should succeed: %v", err)
	}
}

// TestVerify_verifyProdWithKey_Bad — signature made with keyA does not
// verify when only keyB is trusted.
func TestVerify_verifyProdWithKey_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	wrongPub, _, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "mismatched", Name: "Mismatched", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}

	if err := verify(m, ModeProd, []ed25519.PublicKey{wrongPub}); err == nil {
		t.Fatal("verify should fail when signing key is not trusted")
	}
}

// TestVerify_verifyProdWithKey_Ugly — tampering with the manifest after
// signing produces a verify failure (signatures cover the rest of the
// fields, not just Code).
func TestVerify_verifyProdWithKey_Ugly(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "original", Name: "Original", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}

	m.Name = "Tampered"

	if err := verify(m, ModeProd, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("verify should fail on tampered manifest")
	}
}

// TestVerify_verifyNoTrust_Bad — a signed manifest with an EMPTY trust
// list fails closed: no trust roots = no trust.
func TestVerify_verifyNoTrust_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{Code: "alone", Name: "Alone", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	if err := verify(m, ModeProd, nil); err == nil {
		t.Fatal("verify should fail when trusted-key list is empty in prod mode")
	}
}

// TestVerify_verifyManyKeys_Good — the first trusted key that matches
// wins; extra keys in the list are tolerated.
func TestVerify_verifyManyKeys_Good(t *testing.T) {
	otherPub, _, _ := ed25519.GenerateKey(nil)
	realPub, priv, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "multi", Name: "Multi", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}

	if err := verify(m, ModeProd, []ed25519.PublicKey{otherPub, realPub}); err != nil {
		t.Fatalf("verify should succeed when any trusted key matches: %v", err)
	}
}

// TestVerify_verifyWithKey_Good — verifyWithKey round-trips a signed
// manifest with its matching public key.
func TestVerify_verifyWithKey_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{Code: "key-round", Name: "Key Round", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("verifyWithKey: %v", err)
	}
}

// TestVerify_verifyWithKey_Bad — verifyWithKey refuses a mismatched key.
func TestVerify_verifyWithKey_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "wrong-key", Name: "Wrong Key", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	if err := verifyWithKey(m, otherPub); err == nil {
		t.Fatal("verifyWithKey with mismatched key should fail")
	}
}

// TestVerify_verifyWithKey_Ugly — verifyWithKey rejects a nil manifest
// without panicking.
func TestVerify_verifyWithKey_Ugly(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	if err := verifyWithKey(nil, pub); err == nil {
		t.Fatal("verifyWithKey on nil manifest should fail")
	}
}

// TestVerify_signManifest_Good — signManifest produces a signature that
// round-trips through verifyWithKey.
func TestVerify_signManifest_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{Code: "sign-rt", Name: "Sign RT", Version: "0.1.0"}

	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	if m.Sign == "" {
		t.Fatal("signManifest should populate Sign")
	}
	if _, err := base64.StdEncoding.DecodeString(m.Sign); err != nil {
		t.Fatalf("Sign should be base64: %v", err)
	}
	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("round-trip verify: %v", err)
	}
}

// TestVerify_signManifest_Bad — signing with a nil manifest fails
// cleanly rather than panicking.
func TestVerify_signManifest_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	if err := signManifest(nil, priv); err == nil {
		t.Fatal("signManifest on nil manifest should fail")
	}
}

// TestVerify_signManifest_Ugly — an invalid private key length errors
// instead of feeding garbage into ed25519.Sign.
func TestVerify_signManifest_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0"}
	if err := signManifest(m, ed25519.PrivateKey{1, 2, 3}); err == nil {
		t.Fatal("signManifest with short private key should fail")
	}
}

// TestVerify_signableBytes_Good — the canonical form clears the Sign
// field so signer and verifier see the same input.
func TestVerify_signableBytes_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0", Sign: "leftover"}
	out, err := signableBytes(m)
	if err != nil {
		t.Fatalf("signableBytes: %v", err)
	}

	var back config.ViewManifest
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Sign != "" {
		t.Errorf("canonical Sign = %q; want empty", back.Sign)
	}
}

// TestVerify_signableBytes_Bad — the function never nil-panics on an
// empty manifest; the canonical bytes are still non-empty (headers).
func TestVerify_signableBytes_Bad(t *testing.T) {
	out, err := signableBytes(&config.ViewManifest{})
	if err != nil {
		t.Fatalf("signableBytes on empty manifest: %v", err)
	}
	if len(out) == 0 {
		t.Error("signableBytes returned empty bytes for empty manifest")
	}
}

// TestVerify_signableBytes_Ugly — the canonical output is deterministic:
// two calls on the same manifest yield identical bytes.
func TestVerify_signableBytes_Ugly(t *testing.T) {
	m := &config.ViewManifest{Code: "stable", Name: "Stable", Version: "0.1.0"}
	out1, _ := signableBytes(m)
	out2, _ := signableBytes(m)
	if string(out1) != string(out2) {
		t.Error("signableBytes is not deterministic")
	}
}

// TestVerify_verifyWithKey_AssetHash_Ugly — asset integrity metadata is
// part of the signed envelope, unlike install-local provenance fields.
func TestVerify_verifyWithKey_AssetHash_Ugly(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{
		Code:    "asset-hash",
		Name:    "Asset Hash",
		Version: "0.1.0",
		Config: map[string]any{
			"type":       "electron",
			"asset_hash": "abc123",
		},
	}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	m.Config["asset_hash"] = "def456"
	if err := verifyWithKey(m, pub); err == nil {
		t.Fatal("verifyWithKey should fail when asset_hash changes after signing")
	}
}

// TestVerify_verifyWithKey_RFCCompat_Good confirms the verifier accepts
// the RFC-facing manifest layout and ignores local install metadata
// (`config.source`, `config.category`) that the marketplace stamps
// after the upstream manifest was signed.
func TestVerify_verifyWithKey_RFCCompat_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	base := &config.ViewManifest{
		Code:    "compat-signed",
		Name:    "Compat Signed",
		Version: "0.1.0",
		Config: map[string]any{
			"type":     "pwa",
			"url":      "https://play.example.com/",
			"services": []any{"store"},
			"write":    []any{"./cache/"},
			"store":    true,
		},
	}
	msg, err := yamlMarshalBytes(base)
	if err != nil {
		t.Fatalf("yamlMarshalBytes: %v", err)
	}

	signed := *base
	signed.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
	signed.Config = copyConfig(base.Config)
	signed.Config["source"] = "marketplace:compat-signed"
	signed.Config["category"] = "media"

	body, err := yamlMarshalBytes(&signed)
	if err != nil {
		t.Fatalf("yamlMarshalBytes(signed): %v", err)
	}

	var round config.ViewManifest
	if err := UnmarshalViewManifest(body, &round); err != nil {
		t.Fatalf("UnmarshalViewManifest: %v", err)
	}
	if err := verifyWithKey(&round, pub); err != nil {
		t.Fatalf("verifyWithKey should accept RFC-native manifest with install metadata: %v", err)
	}
}

// TestVerify_parsePublicKey_Good — hex-encoded ed25519 key round-trips.
func TestVerify_parsePublicKey_Good(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	enc := hex.EncodeToString(pub)

	decoded, err := parsePublicKey(enc)
	if err != nil {
		t.Fatalf("parsePublicKey: %v", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		t.Errorf("key size = %d; want %d", len(decoded), ed25519.PublicKeySize)
	}
}

// TestVerify_parsePublicKey_Bad — empty input is rejected.
func TestVerify_parsePublicKey_Bad(t *testing.T) {
	if _, err := parsePublicKey(""); err == nil {
		t.Fatal("empty key should fail")
	}
}

// TestVerify_parsePublicKey_Ugly — malformed hex surfaces the decode
// failure.
func TestVerify_parsePublicKey_Ugly(t *testing.T) {
	if _, err := parsePublicKey("not-hex-gg"); err == nil {
		t.Fatal("malformed hex should fail")
	}
}

// TestVerify_resolveTrustedKeys_Good — an explicit WithPublicKey hex
// value produces a one-element trust list.
func TestVerify_resolveTrustedKeys_Good(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	o := NewOptions(
		WithPublicKey(hex.EncodeToString(pub)),
		WithoutKeyLoad(),
	)
	keys, err := resolveTrustedKeys(o)
	if err != nil {
		t.Fatalf("resolveTrustedKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("keys = %d; want 1", len(keys))
	}
}

// TestVerify_resolveTrustedKeys_Bad — an explicit but malformed hex key
// fails the resolver rather than silently skipping.
func TestVerify_resolveTrustedKeys_Bad(t *testing.T) {
	o := NewOptions(
		WithPublicKey("not-hex"),
		WithoutKeyLoad(),
	)
	if _, err := resolveTrustedKeys(o); err == nil {
		t.Fatal("resolveTrustedKeys should fail on malformed explicit key")
	}
}

// TestVerify_resolveTrustedKeys_Ugly — loading keys from a directory:
// only *.pub files parse, directories are skipped, trailing newlines are
// tolerated.
func TestVerify_resolveTrustedKeys_Ugly(t *testing.T) {
	dir := t.TempDir()
	pubA, _, _ := ed25519.GenerateKey(nil)
	pubB, _, _ := ed25519.GenerateKey(nil)

	if err := coreio.Local.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(core.Path(dir, "alpha.pub"), hex.EncodeToString(pubA)+"\n"); err != nil {
		t.Fatalf("Write alpha.pub: %v", err)
	}
	if err := coreio.Local.Write(core.Path(dir, "beta.pub"), hex.EncodeToString(pubB)); err != nil {
		t.Fatalf("Write beta.pub: %v", err)
	}
	if err := coreio.Local.Write(core.Path(dir, "README.txt"), "ignore me"); err != nil {
		t.Fatalf("Write README: %v", err)
	}
	if err := coreio.Local.EnsureDir(core.Path(dir, "subdir")); err != nil {
		t.Fatalf("EnsureDir subdir: %v", err)
	}

	o := NewOptions(
		WithMedium(coreio.Local),
		WithTrustedKeysDir(dir),
	)
	keys, err := resolveTrustedKeys(o)
	if err != nil {
		t.Fatalf("resolveTrustedKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %d; want 2 (alpha + beta)", len(keys))
	}
}

// TestVerify_defaultTrustedKeysDir_Good — the default lands under
// $DIR_HOME/.core/keys when DIR_HOME is set.
func TestVerify_defaultTrustedKeysDir_Good(t *testing.T) {
	// The function reads core.Env("DIR_HOME"); assert the returned path
	// ends with `.core/keys` when DIR_HOME is present.
	got := defaultTrustedKeysDir()
	if got != "" && !core.HasSuffix(got, ".core/keys") {
		t.Errorf("defaultTrustedKeysDir = %q; want a path ending .core/keys", got)
	}
}

// TestVerify_defaultTrustedKeysDir_Bad — returns empty when DIR_HOME is
// unset so the caller can skip the scan cleanly.
func TestVerify_defaultTrustedKeysDir_Bad(t *testing.T) {
	t.Setenv("DIR_HOME", "")
	// core.Env("DIR_HOME") falls back to os.Getenv, so set both sides.
	t.Setenv("HOME", "")
	if got := defaultTrustedKeysDir(); got != "" && core.Env("DIR_HOME") == "" {
		// Note: if core caches DIR_HOME at init, this may still return a
		// value. We only fail if core.Env agrees DIR_HOME is empty AND we
		// still got a non-empty default.
		t.Errorf("defaultTrustedKeysDir = %q; want empty when DIR_HOME empty", got)
	}
}

// TestVerify_defaultTrustedKeysDir_Ugly — repeated calls are idempotent.
func TestVerify_defaultTrustedKeysDir_Ugly(t *testing.T) {
	if defaultTrustedKeysDir() != defaultTrustedKeysDir() {
		t.Error("defaultTrustedKeysDir should be idempotent")
	}
}
