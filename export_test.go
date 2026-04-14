// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"

	"dappco.re/go/core/config"
)

// SignManifestForTest is a test-only shim that exposes the internal
// signManifest helper to black-box tests (package app_test). Lives in
// an `_test.go` file so the helper does not leak into production
// binaries.
//
// Usage (test-only):
//
//	err := app.SignManifestForTest(&manifest, priv)
func SignManifestForTest(m *config.ViewManifest, priv ed25519.PrivateKey) error {
	return signManifest(m, priv)
}
