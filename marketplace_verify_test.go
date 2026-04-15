// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"crypto/ed25519"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	"gopkg.in/yaml.v3"
)

// TestMarketplaceVerify_VerifyListing_Good — a manifest signed with
// the listing's pinned key passes verification.
func TestMarketplaceVerify_VerifyListing_Good(t *testing.T) {
	dest := t.TempDir()
	medium := coreio.Local

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	manifest := config.ViewManifest{
		Code:    "verify-good",
		Name:    "Verify Good",
		Version: "0.1.0",
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}
	body, _ := yaml.Marshal(&manifest)

	viewPath := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listing := &app.MarketplaceListing{
		Code:    "verify-good",
		SignKey: app.SignListingKey(pub),
	}

	if err := app.VerifyListing(medium, dest, listing); err != nil {
		t.Errorf("VerifyListing should accept a matching key; got %v", err)
	}
}

// TestMarketplaceVerify_VerifyListing_Bad — a manifest signed with a
// different key fails verification with a useful message.
func TestMarketplaceVerify_VerifyListing_Bad(t *testing.T) {
	dest := t.TempDir()
	medium := coreio.Local

	_, priv, _ := ed25519.GenerateKey(nil)     // signing key
	otherPub, _, _ := ed25519.GenerateKey(nil) // pinned key (different)

	manifest := config.ViewManifest{
		Code:    "verify-bad",
		Name:    "Verify Bad",
		Version: "0.1.0",
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}
	body, _ := yaml.Marshal(&manifest)

	viewPath := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listing := &app.MarketplaceListing{
		Code:    "verify-bad",
		SignKey: app.SignListingKey(otherPub),
	}

	if err := app.VerifyListing(medium, dest, listing); err == nil {
		t.Error("VerifyListing should reject a mismatching key")
	}
}

// TestMarketplaceVerify_VerifyListing_Ugly — a listing with an empty
// SignKey defers verification (returns nil) so the regular Boot
// verification path takes over.
func TestMarketplaceVerify_VerifyListing_Ugly(t *testing.T) {
	dest := t.TempDir()
	medium := coreio.Local

	manifest := config.ViewManifest{
		Code:    "verify-ugly",
		Name:    "Verify Ugly",
		Version: "0.1.0",
	}
	body, _ := yaml.Marshal(&manifest)

	viewPath := core.Path(dest, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listing := &app.MarketplaceListing{
		Code:    "verify-ugly",
		SignKey: "", // no pinned key — defer to Boot's keyring check
	}

	if err := app.VerifyListing(medium, dest, listing); err != nil {
		t.Errorf("empty SignKey should defer verification; got %v", err)
	}

	// Nil listing and missing manifest path both error.
	if err := app.VerifyListing(medium, dest, nil); err == nil {
		t.Error("nil listing should error")
	}
	if err := app.VerifyListing(medium, t.TempDir(), listing); err == nil {
		t.Error("missing manifest should error")
	}
}

// TestMarketplaceVerify_VerifyListingBytes_Good — same shape as
// VerifyListing but operates on in-memory bytes.
func TestMarketplaceVerify_VerifyListingBytes_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := config.ViewManifest{Code: "bytes-good", Version: "0.1.0"}
	_ = app.SignManifestForTest(&m, priv)
	body, _ := yaml.Marshal(&m)

	if err := app.VerifyListingBytes(body, app.SignListingKey(pub)); err != nil {
		t.Errorf("VerifyListingBytes should accept matching key; got %v", err)
	}
}

// TestMarketplaceVerify_VerifyListingBytes_RFCExtras_Good proves the
// in-memory verify path honours RFC-native manifest fields (services,
// url, store, theme, etc.) the same way disk loads do.
func TestMarketplaceVerify_VerifyListingBytes_RFCExtras_Good(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pwa := &app.PWAManifest{
		Name:            "Play Example",
		ShortName:       "play-example",
		StartURL:        "/app",
		Display:         "standalone",
		ThemeColor:      "#6200ea",
		BackgroundColor: "#ffffff",
		Lang:            "en-GB",
		Permissions:     []string{"notifications", "clipboard-write", "storage"},
	}
	manifest := app.WrapPWA(pwa, app.WrapPWAOptions{
		TargetURL: app.ResolvePWAAppURL("https://play.example.com/manifest.webmanifest", pwa),
	})
	if manifest == nil {
		t.Fatal("WrapPWA returned nil")
	}
	if err := app.SignManifestForTest(manifest, priv); err != nil {
		t.Fatalf("SignManifestForTest: %v", err)
	}

	dest := t.TempDir()
	if err := app.WritePWAWrap(coreio.Local, dest, manifest); err != nil {
		t.Fatalf("WritePWAWrap: %v", err)
	}
	body, err := coreio.Local.Read(core.Path(dest, ".core", "view.yaml"))
	if err != nil {
		t.Fatalf("Read view.yaml: %v", err)
	}

	if err := app.VerifyListingBytes([]byte(body), app.SignListingKey(pub)); err != nil {
		t.Errorf("VerifyListingBytes should accept RFC-native manifest bytes; got %v", err)
	}
}

// TestMarketplaceVerify_VerifyListingBytes_Bad — bad inputs surface
// typed errors rather than panic.
func TestMarketplaceVerify_VerifyListingBytes_Bad(t *testing.T) {
	if err := app.VerifyListingBytes(nil, "abc"); err == nil {
		t.Error("empty body should error")
	}
	if err := app.VerifyListingBytes([]byte("invalid"), "not-hex"); err == nil {
		t.Error("invalid hex key should error")
	}
}

// TestMarketplaceVerify_VerifyListingBytes_Ugly — empty hex key returns
// nil so callers can call unconditionally without checking SignKey
// themselves.
func TestMarketplaceVerify_VerifyListingBytes_Ugly(t *testing.T) {
	if err := app.VerifyListingBytes([]byte("anything"), ""); err != nil {
		t.Errorf("empty hex key should defer; got %v", err)
	}
}

// TestMarketplaceVerify_SignListingKey_Good — round-trips a public key
// to and from its hex form.
func TestMarketplaceVerify_SignListingKey_Good(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	hex := app.SignListingKey(pub)
	if hex == "" {
		t.Fatal("SignListingKey produced empty hex")
	}
	if len(hex) != ed25519.PublicKeySize*2 {
		t.Errorf("SignListingKey produced %d chars; want %d", len(hex), ed25519.PublicKeySize*2)
	}
}

// TestMarketplaceVerify_SignListingKey_Bad — wrong-sized input returns
// empty rather than panicking.
func TestMarketplaceVerify_SignListingKey_Bad(t *testing.T) {
	if got := app.SignListingKey(ed25519.PublicKey{1, 2, 3}); got != "" {
		t.Errorf("wrong-sized key should produce empty hex; got %q", got)
	}
}

// TestMarketplaceVerify_SignListingKey_Ugly — nil input returns empty.
func TestMarketplaceVerify_SignListingKey_Ugly(t *testing.T) {
	if got := app.SignListingKey(nil); got != "" {
		t.Errorf("nil key should produce empty hex; got %q", got)
	}
}
