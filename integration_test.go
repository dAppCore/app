// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	"gopkg.in/yaml.v3"
)

// TestIntegration_Boot_Good drives all 7 steps end-to-end with a signed
// manifest, an HLCRF layout, a permissions block and a config template.
// The test is the single "do we still boot a real CoreApp?" contract
// check for the keystone — it deliberately avoids stubbing any step.
//
//  1. Discover   — writes .core/view.yaml under a temp project root.
//  2. Verify     — signs the manifest with a generated ed25519 key;
//     registers the public key via WithPublicKey.
//  3. Permissions— manifest declares read + net, fs.read / net.fetch
//     actions should be allowed after Boot.
//  4. Modules    — empty (registry is still stubbed; keeps the happy
//     path honest).
//  5. Layout     — HLCRF variant with at least one slot → component.
//  6. Config     — a template file that exists on disk; renderTemplate
//     resolves `{{ .var }}` placeholders against the manifest's
//     declared vars and writes the rendered file next to the source.
//  7. Start      — inst.Start must OK and broadcast ActionAppStarted.
func TestIntegration_Boot_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	// Generate the signing key and its hex-encoded public form for the
	// Boot option.
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Build the manifest in-memory (so we can sign it canonically) then
	// YAML-marshal the signed result into .core/view.yaml.
	manifest := config.ViewManifest{
		Code:    "integration-good",
		Name:    "Integration Good",
		Version: "0.1.0",
		Layout:  "HLCRF",
		Slots: map[string]any{
			"H": "nav-bar",
			"C": "content",
			"F": "status-bar",
		},
		Permissions: config.ViewPermissions{
			Read: []string{"./data/"},
			Net:  []string{"api.example.com:443"},
		},
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars": map[string]any{
					"size":    "256",
					"quality": "85",
				},
			},
		},
	}

	// Sign the manifest with the freshly generated private key — the
	// same machinery the eventual `core sign` CLI will call.
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, err := yaml.Marshal(&manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	viewPath := core.Path(dir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir .core: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write view.yaml: %v", err)
	}

	// Drop the config template so Step 6's existence check passes.
	tmplPath := core.Path(dir, "conf", "thumbs.json.tmpl")
	if err := medium.EnsureDir(core.PathDir(tmplPath)); err != nil {
		t.Fatalf("EnsureDir conf: %v", err)
	}
	if err := medium.Write(tmplPath, `{"size":{{ .size }},"quality":{{ .quality }}}`); err != nil {
		t.Fatalf("Write tmpl: %v", err)
	}

	// Boot the CoreApp in prod mode — this exercises every step
	// including signature verification.
	ctx := context.Background()
	inst, err := app.Boot(ctx, dir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithPublicKey(hex.EncodeToString(pub)),
		app.WithoutKeyLoad(),               // keep the test hermetic
		app.WithWorkspaceHome(t.TempDir()), // workspace under temp
	)
	if err != nil {
		t.Fatalf("Boot failed in prod mode: %v", err)
	}
	if inst == nil {
		t.Fatal("Boot returned nil Instance")
	}
	if inst.Workspace == nil {
		t.Error("Workspace should be provisioned in prod mode")
	}

	// Identity — prove the manifest was parsed.
	if inst.Manifest.Code != "integration-good" {
		t.Errorf("Manifest.Code = %q; want %q", inst.Manifest.Code, "integration-good")
	}
	if inst.Root != dir {
		t.Errorf("Instance.Root = %q; want %q", inst.Root, dir)
	}
	if inst.Mode != app.ModeProd {
		t.Errorf("Mode = %v; want prod", inst.Mode)
	}

	// Permission gate — declared actions allowed, undeclared denied.
	if e := inst.Core.Entitled("fs.read"); !e.Allowed {
		t.Errorf("fs.read should be allowed (manifest declared read); reason=%q", e.Reason)
	}
	if e := inst.Core.Entitled("net.fetch"); !e.Allowed {
		t.Errorf("net.fetch should be allowed (manifest declared net); reason=%q", e.Reason)
	}
	if e := inst.Core.Entitled("process.run"); e.Allowed {
		t.Errorf("process.run should be denied (no run permission); reason=%q", e.Reason)
	}

	// GUI actions bypass the gate (no permission keyword in the RFC).
	if e := inst.Core.Entitled("gui.window.create"); !e.Allowed {
		t.Errorf("gui.window.create should bypass the gate; reason=%q", e.Reason)
	}

	// Step 6 sanity — the template should have been rendered next to
	// the source with the manifest's vars substituted in. The integration
	// test pins the renderer's behaviour end-to-end so a regression in
	// applyConfig surfaces here even if the unit tests pass.
	rendered, rerr := medium.Read(core.Path(dir, "conf", "thumbs.json"))
	if rerr != nil {
		t.Fatalf("read rendered template: %v", rerr)
	}
	if rendered != `{"size":256,"quality":85}` {
		t.Errorf("rendered template = %q; want %q", rendered, `{"size":256,"quality":85}`)
	}

	// Step 7 — Start broadcasts and returns OK. We listen for the
	// ActionAppStarted broadcast via RegisterAction so we know the final
	// step really fired.
	var seen bool
	inst.Core.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if evt, ok := msg.(app.ActionAppStarted); ok {
			if evt.Code == "integration-good" && evt.Mode == "prod" {
				seen = true
			}
		}
		return core.Result{OK: true}
	})

	if r := inst.Start(ctx); !r.OK {
		t.Fatalf("Start.OK = false; Value=%v", r.Value)
	}
	if !seen {
		t.Error("ActionAppStarted was not observed by the registered handler")
	}
}

// TestIntegration_Boot_Bad — a prod-mode boot with a signed manifest
// but no trusted key fails fast at Step 2 (Verify), before any of the
// later steps mutate state. Proves the security boundary holds.
func TestIntegration_Boot_Bad(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	_, priv, _ := ed25519.GenerateKey(nil)

	manifest := config.ViewManifest{
		Code:    "integration-bad",
		Name:    "Integration Bad",
		Version: "0.1.0",
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(dir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Prod mode + no trusted keys + no WithPublicKey → must fail.
	_, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithoutKeyLoad(),               // suppress $DIR_HOME/.core/keys scan
		app.WithWorkspaceHome(t.TempDir()), // workspace under temp
	)
	if err == nil {
		t.Fatal("Boot should reject a signed manifest with no trusted keys in prod mode")
	}
}

// TestIntegration_Boot_Ugly — a manifest whose config template path
// does not exist should fail Step 6 even if Steps 1-5 succeed. This
// pins the "config template missing = refuse to boot" rule for prod
// mode (dev mode warns instead; covered elsewhere).
func TestIntegration_Boot_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	pub, priv, _ := ed25519.GenerateKey(nil)

	manifest := config.ViewManifest{
		Code:    "integration-ugly",
		Name:    "Integration Ugly",
		Version: "0.1.0",
		Config: map[string]any{
			"missing": map[string]any{
				"template": "conf/does-not-exist.tmpl",
				"vars":     map[string]any{},
			},
		},
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(dir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, err := app.Boot(context.Background(), dir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithPublicKey(hex.EncodeToString(pub)),
		app.WithoutKeyLoad(),
		app.WithWorkspaceHome(t.TempDir()),
	)
	if err == nil {
		t.Fatal("Boot should fail when a declared config template is missing")
	}
}

// TestIntegration_CompileSignBoot_Good drives the end-to-end flow the
// RFC §3 promises: keygen → compile → sign → boot. It mirrors the
// CLI path (keygen then sign then compile) without going through argv,
// so it stays hermetic while still proving the artifacts round-trip.
//
//  1. Keygen emits app.key + app.pub in a scratch dir.
//  2. view.yaml on disk is signed in-place with app.Sign.
//  3. app.Compile produces a CompiledManifest.
//  4. app.WriteCompiled puts core.json at the project root.
//  5. app.Boot with ModeProd + the keyring dir reads core.json.
func TestIntegration_CompileSignBoot_Good(t *testing.T) {
	projectDir := t.TempDir()
	keysDir := t.TempDir()
	medium := coreio.Local

	// 1. Keygen
	privPath, pubPath, err := app.Keygen(medium, keysDir, "app")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	if !medium.Exists(privPath) || !medium.Exists(pubPath) {
		t.Fatalf("Keygen did not produce both files")
	}

	// 2. Write an unsigned view.yaml
	manifest := config.ViewManifest{
		Code:    "cli-cycle",
		Name:    "CLI Cycle",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Read: []string{"./data/"},
		},
	}
	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(projectDir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write view.yaml: %v", err)
	}

	// 3. Sign via the public API.
	priv, err := app.LoadPrivateKey(medium, privPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if err := app.Sign(medium, viewPath, priv); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// 4. Compile + Write core.json.
	var signed config.ViewManifest
	if err := config.LoadManifest(medium, viewPath, &signed); err != nil {
		t.Fatalf("reload signed manifest: %v", err)
	}
	cm, err := app.Compile(&signed, app.CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := app.WriteCompiled(medium, projectDir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	// 5. Boot from core.json in prod mode using the keyring only.
	//    This proves the RFC §4.1 "prod reads core.json" path works
	//    alongside the $DIR_HOME/.core/keys/*.pub trust model.
	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithTrustedKeysDir(keysDir),
		app.WithWorkspaceHome(t.TempDir()),
	)
	if err != nil {
		t.Fatalf("Boot from core.json: %v", err)
	}
	if inst.Manifest.Code != "cli-cycle" {
		t.Errorf("Boot read wrong identity: %q", inst.Manifest.Code)
	}
	if e := inst.Core.Entitled("fs.read"); !e.Allowed {
		t.Errorf("fs.read should be allowed after compile+sign+boot; reason=%q", e.Reason)
	}
}

// TestIntegration_Compile_Preserves_Config — compile then boot-from-core.json
// must still render config templates. Pins the RFC §3.1 contract that
// core.json is the "distribution-ready artifact" — it must carry every
// field Step 6 needs, not just the identity + permissions.
//
// Without this guarantee, a user running `core compile && core`
// (prod mode reads core.json) would silently skip their declared
// template blocks. Adding Config to CompiledManifest closes that gap.
func TestIntegration_Compile_Preserves_Config(t *testing.T) {
	projectDir := t.TempDir()
	medium := coreio.Local

	pub, priv, _ := ed25519.GenerateKey(nil)

	// Source manifest with a template block. Step 6 must render this
	// even after the compile→core.json→boot round-trip.
	manifest := config.ViewManifest{
		Code:    "compile-config",
		Name:    "Compile Config",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Read: []string{"./data/"},
		},
		Config: map[string]any{
			"thumbnails": map[string]any{
				"template": "conf/thumbs.json.tmpl",
				"vars":     map[string]any{"size": "256"},
			},
		},
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(projectDir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Drop the template on disk so Step 6's existence check passes.
	tmplPath := core.Path(projectDir, "conf", "thumbs.json.tmpl")
	if err := medium.EnsureDir(core.PathDir(tmplPath)); err != nil {
		t.Fatalf("EnsureDir conf: %v", err)
	}
	if err := medium.Write(tmplPath, `{"size":{{ .size }}}`); err != nil {
		t.Fatalf("Write tmpl: %v", err)
	}

	// Compile so prod mode reads core.json, not view.yaml.
	var signed config.ViewManifest
	if err := config.LoadManifest(medium, viewPath, &signed); err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	cm, err := app.Compile(&signed, app.CompileOptions{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := app.WriteCompiled(medium, projectDir, cm); err != nil {
		t.Fatalf("WriteCompiled: %v", err)
	}

	// Delete the YAML so the only way to succeed is through core.json.
	if err := medium.Delete(viewPath); err != nil {
		t.Fatalf("Delete view.yaml: %v", err)
	}
	// Ensure .core/ still exists so CoreDirs finds the project root.
	if err := medium.EnsureDir(core.Path(projectDir, ".core")); err != nil {
		t.Fatalf("EnsureDir .core: %v", err)
	}

	_, err = app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithPublicKey(hex.EncodeToString(pub)),
		app.WithoutKeyLoad(),
		app.WithWorkspaceHome(t.TempDir()),
	)
	if err != nil {
		t.Fatalf("Boot from core.json with template failed: %v", err)
	}

	// Template must have been rendered from the Config carried through
	// compile → core.json → hydrated ViewManifest.
	rendered, rerr := medium.Read(core.Path(projectDir, "conf", "thumbs.json"))
	if rerr != nil {
		t.Fatalf("read rendered template: %v", rerr)
	}
	if rendered != `{"size":256}` {
		t.Errorf("template not rendered from core.json Config; got %q", rendered)
	}
}

// TestIntegration_KeyringLoad_Good — an actual keys/ directory scan
// resolves a dropped `*.pub` file and trusts the signature. This proves
// the CLI convention ("drop a pubkey in ~/.core/keys/") works with the
// real filesystem medium.
func TestIntegration_KeyringLoad_Good(t *testing.T) {
	projectDir := t.TempDir()
	keysDir := t.TempDir()
	medium := coreio.Local

	pub, priv, _ := ed25519.GenerateKey(nil)

	// Drop the public key into the keyring directory.
	keyPath := core.Path(keysDir, "marketplace.pub")
	if err := medium.Write(keyPath, hex.EncodeToString(pub)); err != nil {
		t.Fatalf("Write keyring: %v", err)
	}

	manifest := config.ViewManifest{
		Code:    "keyring-good",
		Name:    "Keyring Good",
		Version: "0.1.0",
	}
	if err := app.SignManifestForTest(&manifest, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, _ := yaml.Marshal(&manifest)
	viewPath := core.Path(projectDir, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// No WithPublicKey — the trust MUST come from the keyring.
	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeProd),
		app.WithMedium(medium),
		app.WithTrustedKeysDir(keysDir),
		app.WithWorkspaceHome(t.TempDir()),
	)
	if err != nil {
		t.Fatalf("Boot failed with keyring: %v", err)
	}
	if inst == nil || inst.Manifest.Code != "keyring-good" {
		t.Fatalf("unexpected instance: %+v", inst)
	}
}
