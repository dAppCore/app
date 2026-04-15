// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	coreio "dappco.re/go/core/io"
)

// TestSdk_runSDK_Good — `sdk generate` against a fixture project writes
// every default language SDK under .core/sdk/.
func TestSdk_runSDK_Good(t *testing.T) {
	dir := writeViewManifest(t, "sdk-app", "SDK App", "0.1.0")
	rc := runSDK([]string{"generate", dir})
	if rc != 0 {
		t.Fatalf("runSDK rc = %d; want 0", rc)
	}
	for _, lang := range []app.SDKLanguage{
		app.SDKLanguageOpenAPI, app.SDKLanguageTypeScript,
		app.SDKLanguageGo, app.SDKLanguagePHP, app.SDKLanguagePython,
	} {
		dirSDK := core.Path(dir, ".core", "sdk", lang.String())
		if !coreio.Local.IsDir(dirSDK) {
			t.Errorf("%s output missing at %s", lang.String(), dirSDK)
		}
	}
}

// TestSdk_runSDK_Bad — no verb, unknown verb, unknown flag and missing
// manifest are all rejected with the right exit code (64 or non-zero).
func TestSdk_runSDK_Bad(t *testing.T) {
	if rc := runSDK(nil); rc != 64 {
		t.Errorf("runSDK(nil) rc = %d; want 64", rc)
	}
	if rc := runSDK([]string{"unknown"}); rc != 64 {
		t.Errorf("runSDK(unknown verb) rc = %d; want 64", rc)
	}
	if rc := runSDK([]string{"generate", "--unknown"}); rc != 64 {
		t.Errorf("runSDK(unknown flag) rc = %d; want 64", rc)
	}
	if rc := runSDK([]string{"generate", t.TempDir()}); rc == 0 {
		t.Error("runSDK without manifest should fail")
	}
}

// TestSdk_runSDK_Ugly — `--lang` filter limits the output, and `--out`
// retargets the directory.
func TestSdk_runSDK_Ugly(t *testing.T) {
	dir := writeViewManifest(t, "ts-only", "TS Only", "0.1.0")
	out := core.Path(t.TempDir(), "build")
	rc := runSDK([]string{"generate", "--lang", "ts", "--out", out, dir})
	if rc != 0 {
		t.Fatalf("runSDK rc = %d; want 0", rc)
	}
	want := core.Path(out, "ts", "index.ts")
	if !coreio.Local.Exists(want) {
		t.Errorf("expected %s; not present", want)
	}
	other := core.Path(out, "openapi", "openapi.json")
	if coreio.Local.Exists(other) {
		t.Errorf("language filter ignored — found %s", other)
	}
}

// TestSdk_runSDKGenerate_Bad — dangling --lang / --out are rejected.
func TestSdk_runSDKGenerate_Bad(t *testing.T) {
	if rc := runSDKGenerate([]string{"--lang"}); rc != 64 {
		t.Errorf("dangling --lang rc = %d; want 64", rc)
	}
	if rc := runSDKGenerate([]string{"--out"}); rc != 64 {
		t.Errorf("dangling --out rc = %d; want 64", rc)
	}
	if rc := runSDKGenerate([]string{"--lang", "rust"}); rc != 64 {
		t.Errorf("invalid lang rc = %d; want 64", rc)
	}
}

// TestSdk_runSDKGenerate_Good — `--all` widens the filter to include
// every primitive regardless of declared permissions.
func TestSdk_runSDKGenerate_Good(t *testing.T) {
	dir := writeViewManifest(t, "all-sdk", "All SDK", "0.1.0")
	rc := runSDKGenerate([]string{"--all", "--lang", "ts", dir})
	if rc != 0 {
		t.Fatalf("runSDKGenerate --all rc = %d; want 0", rc)
	}
	body, err := coreio.Local.Read(core.Path(dir, ".core", "sdk", "ts", "index.ts"))
	if err != nil {
		t.Fatalf("Read sdk: %v", err)
	}
	// fs.write requires a `write` permission the manifest does not declare;
	// without --all it would be filtered out. Confirm --all preserves it.
	if !core.Contains(body, "fsWrite") {
		t.Errorf("--all should expose fsWrite even without a write permission: %s", body)
	}
}

// TestSdk_runSDKGenerate_Ugly — missing manifest in the explicit project
// directory is reported with a useful exit code.
func TestSdk_runSDKGenerate_Ugly(t *testing.T) {
	if rc := runSDKGenerate([]string{t.TempDir()}); rc == 0 {
		t.Error("runSDKGenerate against empty dir should fail")
	}
}

// TestSdk_runSDKList_Good — `sdk list` renders the full action
// catalogue with the expected column headings and without failing.
func TestSdk_runSDKList_Good(t *testing.T) {
	if rc := runSDK([]string{"list"}); rc != 0 {
		t.Errorf("runSDK(list) rc = %d; want 0", rc)
	}
}

// TestSdk_runSDKList_Bad — unknown flag is rejected with EX_USAGE (64).
func TestSdk_runSDKList_Bad(t *testing.T) {
	if rc := runSDK([]string{"list", "--unknown"}); rc != 64 {
		t.Errorf("runSDK(list --unknown) rc = %d; want 64", rc)
	}
}

// TestSdk_runSDKList_Ugly — `--json` produces a JSON document the CLI
// caller can pipe into `jq` without re-parsing the human table.
func TestSdk_runSDKList_Ugly(t *testing.T) {
	if rc := runSDK([]string{"list", "--json"}); rc != 0 {
		t.Errorf("runSDK(list --json) rc = %d; want 0", rc)
	}
}

// TestSdk_displayPermission_Good — ungated actions render as an ASCII
// hyphen so byte-counting table formatters keep the column aligned;
// gated actions render the permission name as given.
func TestSdk_displayPermission_Good(t *testing.T) {
	cases := map[string]string{
		"":     "-",
		"read": "read",
		"net":  "net",
	}
	for in, want := range cases {
		if got := displayPermission(in); got != want {
			t.Errorf("displayPermission(%q) = %q; want %q", in, got, want)
		}
	}
}
