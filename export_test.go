// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/ed25519"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
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

// ExtractErrForTest exposes the internal extractErr helper to
// black-box tests so the marketplace.go branches that wrap process
// failures can be covered without spinning up a real `git` process.
//
//	err := app.ExtractErrForTest(core.Result{Value: io.EOF})
func ExtractErrForTest(r core.Result) error {
	return extractErr(r)
}

// VerifyListingAfterInstallForTest exposes the post-install verify
// helper so tests can pin the rollback-on-verify-failure behaviour
// without invoking `git`.
//
//	err := app.VerifyListingAfterInstallForTest(coreio.Local, dest, listing, opts)
func VerifyListingAfterInstallForTest(medium coreio.Medium, dest string, listing *MarketplaceListing, opts MarketplaceInstallOptions) error {
	return verifyListingAfterInstall(medium, dest, listing, opts)
}

// StampSourceForTest exposes the package-private stamp helper so
// black-box tests can plant a `Config["source"]` field on an installed
// manifest without re-implementing the marshal logic.
//
//	err := app.StampSourceForTest(coreio.Local, dest, "marketplace:foo")
func StampSourceForTest(medium coreio.Medium, dest, source string) error {
	return stampSource(medium, dest, source)
}
