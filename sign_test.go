// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	"gopkg.in/yaml.v3"
)

// TestSign_Sign_Good — a view.yaml on disk is read, signed, and
// written back with a valid Sign field. The result round-trips through
// verifyWithKey against the paired public key.
func TestSign_Sign_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	path := core.Path(dir, "view.yaml")

	body, _ := yaml.Marshal(config.ViewManifest{
		Code:    "sign-good",
		Name:    "Sign Good",
		Version: "0.1.0",
	})
	if err := coreio.Local.Write(path, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := Sign(coreio.Local, path, priv); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	after, _ := coreio.Local.Read(path)
	var manifest config.ViewManifest
	if err := yaml.Unmarshal([]byte(after), &manifest); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if manifest.Sign == "" {
		t.Fatal("Sign field empty after Sign call")
	}
	if err := verifyWithKey(&manifest, pub); err != nil {
		t.Fatalf("round-trip verify failed: %v", err)
	}
}

// TestSign_Sign_Bad — invalid paths, missing files and bad keys are
// rejected with no side effects.
func TestSign_Sign_Bad(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)

	if err := Sign(coreio.Local, "", priv); err == nil {
		t.Fatal("empty path should fail")
	}
	if err := Sign(coreio.Local, core.Path(t.TempDir(), "nope.yaml"), priv); err == nil {
		t.Fatal("missing file should fail")
	}
	if err := Sign(coreio.Local, "whatever", ed25519.PrivateKey{1, 2, 3}); err == nil {
		t.Fatal("short private key should fail")
	}
}

// TestSign_Sign_Ugly — signing a file that parses as YAML but isn't a
// view.yaml (e.g. missing code/name/version) still succeeds — we only
// touch the Sign field. The caller owns manifest validation.
func TestSign_Sign_Ugly(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	path := core.Path(dir, "weird.yaml")

	// Missing required fields — still valid YAML, just not a complete
	// manifest. Sign should still stamp the signature field.
	if err := coreio.Local.Write(path, `code: partial`); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := Sign(coreio.Local, path, priv); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	after, _ := coreio.Local.Read(path)
	var manifest config.ViewManifest
	_ = yaml.Unmarshal([]byte(after), &manifest)
	if manifest.Sign == "" {
		t.Error("Sign field still empty on partial manifest")
	}
}

// TestSign_LoadPrivateKey_Good — round-trip hex encoding through
// Write + Load.
func TestSign_LoadPrivateKey_Good(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	path := core.Path(t.TempDir(), "app.key")

	if err := WritePrivateKey(coreio.Local, path, priv); err != nil {
		t.Fatalf("WritePrivateKey: %v", err)
	}

	back, err := LoadPrivateKey(coreio.Local, path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if !bytesEqual(back, priv) {
		t.Error("round-trip mismatch")
	}
}

// TestSign_LoadPrivateKey_Bad — missing path, missing file, short key
// file.
func TestSign_LoadPrivateKey_Bad(t *testing.T) {
	if _, err := LoadPrivateKey(coreio.Local, ""); err == nil {
		t.Fatal("empty path should error")
	}
	if _, err := LoadPrivateKey(coreio.Local, core.Path(t.TempDir(), "missing.key")); err == nil {
		t.Fatal("missing file should error")
	}
	// Short hex.
	shortPath := core.Path(t.TempDir(), "short.key")
	if err := coreio.Local.Write(shortPath, "aabb"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := LoadPrivateKey(coreio.Local, shortPath); err == nil {
		t.Fatal("short key should error")
	}
}

// TestSign_LoadPrivateKey_Ugly — file with trailing whitespace still
// decodes.
func TestSign_LoadPrivateKey_Ugly(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	path := core.Path(t.TempDir(), "app.key")
	if err := coreio.Local.Write(path, hex.EncodeToString(priv)+"\n\n  "); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := LoadPrivateKey(coreio.Local, path); err != nil {
		t.Fatalf("LoadPrivateKey should tolerate trailing whitespace: %v", err)
	}
}

// TestSign_WritePublicKey_Good — round-trip a public key via Write + the
// resolveTrustedKeys reader which also parses `.pub` entries.
func TestSign_WritePublicKey_Good(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	path := core.Path(dir, "trust.pub")

	if err := WritePublicKey(coreio.Local, path, pub); err != nil {
		t.Fatalf("WritePublicKey: %v", err)
	}

	o := NewOptions(WithMedium(coreio.Local), WithTrustedKeysDir(dir))
	keys, err := resolveTrustedKeys(o)
	if err != nil {
		t.Fatalf("resolveTrustedKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key loaded, got %d", len(keys))
	}
}

// TestSign_WritePublicKey_Bad — empty path / short key rejected.
func TestSign_WritePublicKey_Bad(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	if err := WritePublicKey(coreio.Local, "", pub); err == nil {
		t.Fatal("empty path should fail")
	}
	if err := WritePublicKey(coreio.Local, core.Path(t.TempDir(), "x.pub"), ed25519.PublicKey{1, 2}); err == nil {
		t.Fatal("short key should fail")
	}
}

// TestSign_WritePublicKey_Ugly — nil medium falls back to Local.
func TestSign_WritePublicKey_Ugly(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	path := core.Path(t.TempDir(), "fallback.pub")
	if err := WritePublicKey(nil, path, pub); err != nil {
		t.Fatalf("nil medium fallback failed: %v", err)
	}
}

// TestSign_Keygen_Good — Keygen creates paired .key + .pub files in the
// target directory.
func TestSign_Keygen_Good(t *testing.T) {
	dir := t.TempDir()

	privPath, pubPath, err := Keygen(coreio.Local, dir, "app")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	if !coreio.Local.Exists(privPath) || !coreio.Local.Exists(pubPath) {
		t.Fatalf("Keygen did not write both files: priv=%s pub=%s", privPath, pubPath)
	}

	// Load and verify the pair actually matches.
	priv, err := LoadPrivateKey(coreio.Local, privPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	// Sign & verify something trivial.
	m := &config.ViewManifest{Code: "keygen", Name: "Keygen", Version: "0.1.0"}
	if err := signManifest(m, priv); err != nil {
		t.Fatalf("signManifest: %v", err)
	}
	// Parse the pub file the same way resolveTrustedKeys does.
	pubBody, _ := coreio.Local.Read(pubPath)
	pub, err := parsePublicKey(core.Trim(pubBody))
	if err != nil {
		t.Fatalf("parsePublicKey: %v", err)
	}
	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("paired keys do not verify: %v", err)
	}
}

// TestSign_Keygen_Bad — empty directory rejected; default name used when
// blank.
func TestSign_Keygen_Bad(t *testing.T) {
	if _, _, err := Keygen(coreio.Local, "", "app"); err == nil {
		t.Fatal("empty dir should fail")
	}
}

// TestSign_Keygen_Ugly — empty name falls through to the "default"
// convention.
func TestSign_Keygen_Ugly(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath, err := Keygen(coreio.Local, dir, "")
	if err != nil {
		t.Fatalf("Keygen with empty name: %v", err)
	}
	if !core.HasSuffix(privPath, "default.key") {
		t.Errorf("expected default.key filename; got %q", privPath)
	}
	if !core.HasSuffix(pubPath, "default.pub") {
		t.Errorf("expected default.pub filename; got %q", pubPath)
	}
}

// bytesEqual compares two ed25519 key-length byte slices without
// pulling a fresh primitive dependency.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSign_DefaultPrivateKeyPath_Good resolves to
// `$DIR_HOME/.core/keys/default.key` when DIR_HOME is non-empty.
func TestSign_DefaultPrivateKeyPath_Good(t *testing.T) {
	got := DefaultPrivateKeyPath()
	if got == "" {
		t.Skip("DIR_HOME is empty in this environment")
	}
	// Path ends with the conventional suffix.
	if !core.HasSuffix(got, core.Path(".core", "keys", DefaultKeyName)) {
		t.Errorf("DefaultPrivateKeyPath() = %q; want suffix .core/keys/%s", got, DefaultKeyName)
	}
}

// TestSign_SignManifest_Good — the in-memory ViewManifest is mutated
// with a base64 ed25519 signature that round-trips through the paired
// public key. Mirrors what wrap pipelines (PWA, Electron, Web) call
// before persisting the manifest to disk.
func TestSign_SignManifest_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{
		Code:    "sign-manifest-good",
		Name:    "Sign Manifest Good",
		Version: "0.1.0",
	}

	if err := SignManifest(m, priv); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	if m.Sign == "" {
		t.Fatal("Sign field empty after SignManifest")
	}
	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("paired key did not verify signed manifest: %v", err)
	}
}

// TestSign_SignManifest_Bad — a wrong-length private key is rejected
// before the manifest is mutated. The Sign field stays empty.
func TestSign_SignManifest_Bad(t *testing.T) {
	m := &config.ViewManifest{Code: "sign-manifest-bad"}

	if err := SignManifest(m, ed25519.PrivateKey{1, 2, 3}); err == nil {
		t.Fatal("short private key should error")
	}
	if m.Sign != "" {
		t.Errorf("Sign field mutated on error path: %q", m.Sign)
	}
}

// TestSign_SignManifest_Ugly — re-signing an already-signed manifest
// overwrites the Sign field with a fresh value. The signable bytes
// hash the manifest content with Sign cleared, so re-signing produces
// the same signature for the same key + content but the function must
// still tolerate a non-empty starting Sign.
func TestSign_SignManifest_Ugly(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &config.ViewManifest{
		Code:    "sign-manifest-ugly",
		Name:    "Sign Manifest Ugly",
		Version: "0.1.0",
		Sign:    "stale-signature-from-a-previous-key",
	}

	if err := SignManifest(m, priv); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	if m.Sign == "stale-signature-from-a-previous-key" {
		t.Fatal("Sign field still holds the stale value after re-signing")
	}
	if err := verifyWithKey(m, pub); err != nil {
		t.Fatalf("re-signed manifest does not verify: %v", err)
	}
}

// TestSign_LoadDefaultPrivateKey_Bad — when the default key does not
// exist, the error message should hint at the keygen command instead
// of being a bare ENOENT.
func TestSign_LoadDefaultPrivateKey_Bad(t *testing.T) {
	// Point at a temp HOME so the default-key file definitely does not
	// exist (we use t.Setenv but accept the test may skip if core/info
	// snapshots DIR_HOME at process start).
	if DefaultPrivateKeyPath() == "" {
		t.Skip("DIR_HOME is empty in this environment")
	}
	// We don't try to install a fake DIR_HOME — instead just call the
	// loader and confirm the typed error comes back when no key exists
	// at the default location. If the user already has one we skip.
	if coreio.Local.Exists(DefaultPrivateKeyPath()) {
		t.Skip("user has a real default key; skipping the missing-key path")
	}
	if _, err := LoadDefaultPrivateKey(coreio.Local); err == nil {
		t.Error("LoadDefaultPrivateKey produced no error when no key file exists")
	}
}
