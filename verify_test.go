// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"dappco.re/go/core/config"
	"gopkg.in/yaml.v3"
)

// TestVerify_verify_Good — dev mode skips the signature check, even
// when Sign is absent. This is the path developers iterate on most.
func TestVerify_verify_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "ok", Name: "OK", Version: "0.1.0"}
	if err := verify(m, ModeDev); err != nil {
		t.Fatalf("verify (dev, unsigned) should succeed: %v", err)
	}
}

// TestVerify_verify_Bad — prod mode rejects an unsigned manifest. The
// boundary ("no sign = no boot") is the core security property the
// skeleton must honour even before key lookup lands.
func TestVerify_verify_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "no-sign", Name: "No Sign", Version: "0.1.0"}
	err := verify(m, ModeProd)
	if err == nil {
		t.Fatal("verify (prod, unsigned) should fail")
	}
}

// TestVerify_verify_Ugly — a manifest with a malformed signature
// surfaces the decode failure rather than silently passing.
func TestVerify_verify_Ugly(t *testing.T) {
	m := &config.ViewManifest{
		Code:    "bad-sign",
		Name:    "Bad Sign",
		Version: "0.1.0",
		Sign:    "not-base64!@#",
	}
	err := verify(m, ModeProd)
	if err == nil {
		t.Fatal("verify (prod, malformed sign) should fail")
	}
}

// TestVerify_verifyWithKey_Good — a manifest signed with a known key
// round-trips through verifyWithKey.
func TestVerify_verifyWithKey_Good(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := &config.ViewManifest{Code: "signed", Name: "Signed", Version: "0.1.0"}
	msg, err := signableBytes(m)
	if err != nil {
		t.Fatalf("signableBytes: %v", err)
	}
	m.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))

	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("verifyWithKey: %v", err)
	}
}

// TestVerify_verifyWithKey_Bad — a manifest signed with keyA cannot be
// verified with keyB.
func TestVerify_verifyWithKey_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "wrong-key", Name: "Wrong Key", Version: "0.1.0"}
	msg, _ := signableBytes(m)
	m.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))

	if err := verifyWithKey(m, otherPub); err == nil {
		t.Fatal("verifyWithKey with mismatched key should fail")
	}
}

// TestVerify_verifyWithKey_Ugly — a tampered manifest (field edited
// after signing) fails verification.
func TestVerify_verifyWithKey_Ugly(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	m := &config.ViewManifest{Code: "original", Name: "Original", Version: "0.1.0"}
	msg, _ := signableBytes(m)
	m.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))

	// Tamper after signing.
	m.Name = "Tampered"

	if err := verifyWithKey(m, pub); err == nil {
		t.Fatal("verifyWithKey on tampered manifest should fail")
	}
}

// TestVerify_signableBytes_Good — signable canonicalisation clears
// Sign so the signer and verifier see the same input.
func TestVerify_signableBytes_Good(t *testing.T) {
	m := &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0", Sign: "leftover"}
	out, err := signableBytes(m)
	if err != nil {
		t.Fatalf("signableBytes: %v", err)
	}

	// Round-trip the YAML so we can assert the Sign field is empty.
	var back config.ViewManifest
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Sign != "" {
		t.Errorf("canonical Sign = %q; want empty", back.Sign)
	}
}

// TestVerify_signableBytes_Bad — a nil-returning marshal is still
// stable; but the function never nil-panics. This pins the guarantee.
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
