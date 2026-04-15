// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	"gopkg.in/yaml.v3"
)

// TestMain_parseArgs_Good — the default (no args) lands on ModeProd
// and the current directory. This is the "run me in a CoreApp dir"
// behaviour.
func TestMain_parseArgs_Good(t *testing.T) {
	mode, start := parseArgs(nil)
	if mode != app.ModeProd {
		t.Errorf("mode = %v; want %v", mode, app.ModeProd)
	}
	if start != "./" {
		t.Errorf("start = %q; want %q", start, "./")
	}
}

// TestMain_parseArgs_Bad — a positional arg becomes start, regardless
// of whether it exists. The Boot step surfaces the missing manifest.
func TestMain_parseArgs_Bad(t *testing.T) {
	mode, start := parseArgs([]string{"./never-there"})
	if mode != app.ModeProd {
		t.Errorf("mode = %v; want %v", mode, app.ModeProd)
	}
	if start != "./never-there" {
		t.Errorf("start = %q; want %q", start, "./never-there")
	}
}

// TestMain_parseArgs_Ugly — --dev flag lifts mode, and a trailing
// path still wins as start.
func TestMain_parseArgs_Ugly(t *testing.T) {
	mode, start := parseArgs([]string{"--dev", "./photo-browser"})
	if mode != app.ModeDev {
		t.Errorf("mode = %v; want %v", mode, app.ModeDev)
	}
	if start != "./photo-browser" {
		t.Errorf("start = %q; want %q", start, "./photo-browser")
	}
}

// TestMain_runCompile_Good — compile reads view.yaml and emits
// core.json at the project root. Smoke test for the CLI wiring — the
// Compile/WriteCompiled contract lives in app_test.
func TestMain_runCompile_Good(t *testing.T) {
	dir := writeViewManifest(t, "cli-compile", "CLI Compile", "0.1.0")

	rc := runCompile([]string{dir})
	if rc != 0 {
		t.Fatalf("runCompile returned %d; want 0", rc)
	}

	path := core.Path(dir, app.CompiledFileName)
	if !coreio.Local.Exists(path) {
		t.Fatalf("core.json not created at %q", path)
	}
}

// TestMain_runCompile_Bad — compile in a directory with no view.yaml
// returns non-zero.
func TestMain_runCompile_Bad(t *testing.T) {
	if rc := runCompile([]string{t.TempDir()}); rc == 0 {
		t.Fatal("runCompile without manifest should error")
	}
	if rc := runCompile([]string{"--key"}); rc == 0 {
		t.Fatal("runCompile with dangling --key should error")
	}
}

// TestMain_runCompile_Ugly — an unknown flag is rejected (EX_USAGE).
func TestMain_runCompile_Ugly(t *testing.T) {
	if rc := runCompile([]string{"--what"}); rc != 64 {
		t.Errorf("runCompile unknown flag rc = %d; want 64", rc)
	}
}

// TestMain_runSign_Good — sign signs the view.yaml and the signature
// verifies against the paired public key.
func TestMain_runSign_Good(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	dir := writeViewManifest(t, "cli-sign", "CLI Sign", "0.1.0")
	keyPath := core.Path(t.TempDir(), "app.key")
	if err := coreio.Local.Write(keyPath, hex.EncodeToString(priv)); err != nil {
		t.Fatalf("Write key: %v", err)
	}

	rc := runSign([]string{"--key", keyPath, dir})
	if rc != 0 {
		t.Fatalf("runSign rc = %d; want 0", rc)
	}

	// Re-read and verify.
	body, _ := coreio.Local.Read(core.Path(dir, ".core", "view.yaml"))
	var manifest config.ViewManifest
	_ = yaml.Unmarshal([]byte(body), &manifest)
	if manifest.Sign == "" {
		t.Fatal("Sign field empty after CLI sign")
	}

	// Round-trip through Boot with the paired public key.
	_, err := app.Boot(t.Context(), dir,
		app.WithMode(app.ModeProd),
		app.WithPublicKey(hex.EncodeToString(pub)),
		app.WithoutKeyLoad(),
	)
	if err != nil {
		t.Fatalf("Boot after CLI sign failed: %v", err)
	}
}

// TestMain_runSign_Bad — missing --key, missing manifest and missing
// key file all return non-zero.
func TestMain_runSign_Bad(t *testing.T) {
	if rc := runSign(nil); rc == 0 {
		t.Fatal("runSign without --key should error")
	}
	if rc := runSign([]string{"--key"}); rc == 0 {
		t.Fatal("runSign with dangling --key should error")
	}
	if rc := runSign([]string{"--key", core.Path(t.TempDir(), "missing.key"), t.TempDir()}); rc == 0 {
		t.Fatal("runSign with missing key should error")
	}
}

// TestMain_runSign_Ugly — unknown flag is rejected.
func TestMain_runSign_Ugly(t *testing.T) {
	if rc := runSign([]string{"--nope"}); rc != 64 {
		t.Errorf("runSign unknown flag rc = %d; want 64", rc)
	}
}

// TestMain_runSign_NoFlag_RFCDefault — the RFC §3.2 example `core sign`
// (no flag) resolves to the default keyring path. The test can't
// override core.Env("DIR_HOME") from within the test binary (it's
// captured at init) so this case exercises the failure path — the
// default key lookup surfaces a recognisable error when the key is
// absent, which proves the no-flag branch routed to LoadDefaultPrivateKey
// rather than falling through to the old "--key required" reject.
func TestMain_runSign_NoFlag_RFCDefault(t *testing.T) {
	dir := writeViewManifest(t, "cli-sign-no-flag", "CLI Sign No Flag", "0.1.0")
	// Touch $DIR_HOME/.core/keys out of the way of any real default key
	// by not providing --key or --default. The function should attempt
	// LoadDefaultPrivateKey and fail with a "default key not found"
	// message (or succeed silently if a default key happens to be
	// installed on the host). Either way, the return code must NOT be
	// the old flag-required EX_USAGE=64.
	rc := runSign([]string{dir})
	if rc == 64 {
		t.Fatalf("runSign (no flag) returned EX_USAGE=64; the no-flag branch "+
			"should route to the default key, not reject the invocation (rc=%d)", rc)
	}
}

// TestMain_runKeygen_Good — keygen writes paired files in the target
// directory.
func TestMain_runKeygen_Good(t *testing.T) {
	dir := t.TempDir()
	rc := runKeygen([]string{"--dir", dir, "--name", "app"})
	if rc != 0 {
		t.Fatalf("runKeygen rc = %d; want 0", rc)
	}
	if !coreio.Local.Exists(core.Path(dir, "app.key")) {
		t.Error("app.key missing")
	}
	if !coreio.Local.Exists(core.Path(dir, "app.pub")) {
		t.Error("app.pub missing")
	}
}

// TestMain_runKeygen_Bad — missing --dir is rejected.
func TestMain_runKeygen_Bad(t *testing.T) {
	if rc := runKeygen(nil); rc == 0 {
		t.Fatal("runKeygen without --dir should fail")
	}
	if rc := runKeygen([]string{"--dir"}); rc == 0 {
		t.Fatal("runKeygen with dangling --dir should fail")
	}
}

// TestMain_runKeygen_Ugly — unknown flags rejected (EX_USAGE).
func TestMain_runKeygen_Ugly(t *testing.T) {
	if rc := runKeygen([]string{"--wrong"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
}

// writeViewManifest is a tiny helper for the CLI tests. Keeps fixture
// setup a single line per test.
//
//	dir := writeViewManifest(t, "code", "Name", "0.1.0")
func writeViewManifest(t *testing.T, code, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	path := core.Path(dir, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(path)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body := "code: " + code + "\nname: " + name + "\nversion: " + version + "\n"
	if err := coreio.Local.Write(path, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}
