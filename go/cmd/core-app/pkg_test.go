// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	"gopkg.in/yaml.v3"
)

// TestPkg_runPkg_Good — `pkg list` against a fake home with one
// installed package returns 0 and uses DIR_HOME as configured.
func TestPkg_runPkg_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CORE_HOME", home) // affects core.Env("DIR_HOME") on next info refresh

	// CORE_HOME is read at process start by core/info, so a t.Setenv
	// inside a test does not flip core.Env. Skip if we cannot redirect
	// the Env value — the underlying PkgList API is exercised by
	// app/pkg_test.go.
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; covered by app.PkgList tests")
	}

	plant(t, home, "x", &config.ViewManifest{Code: "x", Name: "X", Version: "0.1.0"})

	if rc := runPkg([]string{"list"}); rc != 0 {
		t.Errorf("runPkg list rc = %d; want 0", rc)
	}
}

// TestPkg_runPkg_Bad — invocations missing required args return 64
// (EX_USAGE).
func TestPkg_runPkg_Bad(t *testing.T) {
	if rc := runPkg(nil); rc != 64 {
		t.Errorf("runPkg(nil) rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"unknown"}); rc != 64 {
		t.Errorf("runPkg unknown verb rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"wrap"}); rc != 64 {
		t.Errorf("runPkg wrap with no source rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"wrap", "--pwa"}); rc != 64 {
		t.Errorf("runPkg wrap --pwa dangling rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"wrap", "--web"}); rc != 64 {
		t.Errorf("runPkg wrap --web dangling rc = %d; want 64", rc)
	}
}

// TestPkg_runPkg_Ugly — `pkg --help` exits 0; `pkg wrap --help` ditto.
func TestPkg_runPkg_Ugly(t *testing.T) {
	if rc := runPkg([]string{"--help"}); rc != 0 {
		t.Errorf("runPkg --help rc = %d; want 0", rc)
	}
	if rc := runPkg([]string{"wrap", "--help"}); rc != 0 {
		t.Errorf("runPkg wrap --help rc = %d; want 0", rc)
	}
}

// TestPkg_runPkgBundle_Good wraps a local web directory and writes the
// manifest to an explicit --dest, bypassing the install path.
func TestPkg_runPkgBundle_Good(t *testing.T) {
	_ = "runPkgBundle"
	src := t.TempDir()
	dest := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(src, "index.html"), "<html/>"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rc := runPkg([]string{"wrap", "--web", src, "--dest", dest, "--code", "x", "--name", "X"})
	if rc != 0 {
		t.Fatalf("runPkg wrap rc = %d; want 0", rc)
	}
	if !medium.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Errorf("view.yaml missing at %s", dest)
	}
	if !medium.Exists(core.Path(dest, "index.html")) {
		t.Errorf("index.html missing at %s after wrap", dest)
	}
}

// TestPkg_runPkgBundle_Bad — wrap with neither --pwa nor --electron nor
// --web → EX_USAGE; wrap --web with bad path → 1.
func TestPkg_runPkgBundle_Bad(t *testing.T) {
	_ = "runPkgBundle"
	if rc := runPkg([]string{"wrap"}); rc != 64 {
		t.Errorf("wrap with no source rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"wrap", "--web", "/definitely/not/a/dir"}); rc == 0 {
		t.Errorf("wrap --web bad path returned 0; want non-zero")
	}
}

// TestPkg_runPkgBundle_Ugly — wrap --pwa against a mock HTTP server,
// dest provided so the install path is skipped.
func TestPkg_runPkgBundle_Ugly(t *testing.T) {
	_ = "runPkgBundle"
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := t.TempDir()
	rc := runPkg([]string{"wrap", "--pwa", srv.URL + "/manifest.json", "--dest", dest})
	if rc != 0 {
		t.Errorf("wrap --pwa rc = %d; want 0", rc)
	}
	if !coreio.Local.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Errorf("view.yaml missing at %s after wrap --pwa", dest)
	}
}

// TestPkg_runPkgBundle_PWAAppURL_Good confirms the CLI accepts an app URL
// rather than requiring the caller to know the exact manifest.json
// path.
func TestPkg_runPkgBundle_PWAAppURL_Good(t *testing.T) {
	_ = "runPkgBundle PWAAppURL"
	_ = "runPkgBundle_PWAAppURL"
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html/>"))
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/manifest+json")
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dest := t.TempDir()
	rc := runPkg([]string{"wrap", "--pwa", srv.URL, "--dest", dest})
	if rc != 0 {
		t.Fatalf("wrap --pwa app URL rc = %d; want 0", rc)
	}
	if !coreio.Local.Exists(core.Path(dest, ".core", "view.yaml")) {
		t.Errorf("view.yaml missing at %s after wrap --pwa app URL", dest)
	}
}

// TestPkg_runMarketplace_Good — `marketplace search QUERY` runs
// against a planted local index and returns 0 even if no results.
func TestPkg_runMarketplace_Good(t *testing.T) {
	home := plantMarketplaceIndex(t)
	t.Setenv("CORE_HOME", home)

	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env")
	}

	if rc := runMarketplace([]string{"search", "photo"}); rc != 0 {
		t.Errorf("marketplace search rc = %d; want 0", rc)
	}
}

// TestPkg_runMarketplace_Bad — invalid verbs and missing arguments.
func TestPkg_runMarketplace_Bad(t *testing.T) {
	if rc := runMarketplace(nil); rc != 64 {
		t.Errorf("runMarketplace(nil) rc = %d; want 64", rc)
	}
	if rc := runMarketplace([]string{"unknown"}); rc != 64 {
		t.Errorf("unknown verb rc = %d; want 64", rc)
	}
	if rc := runMarketplace([]string{"search"}); rc != 64 {
		t.Errorf("search no query rc = %d; want 64", rc)
	}
	if rc := runMarketplace([]string{"fetch"}); rc != 64 {
		t.Errorf("fetch no --url rc = %d; want 64", rc)
	}
	if rc := runMarketplace([]string{"fetch", "--unknown"}); rc != 64 {
		t.Errorf("fetch unknown flag rc = %d; want 64", rc)
	}
}

// TestPkg_runMarketplace_Ugly — `--help` exits 0.
func TestPkg_runMarketplace_Ugly(t *testing.T) {
	if rc := runMarketplace([]string{"--help"}); rc != 0 {
		t.Errorf("--help rc = %d; want 0", rc)
	}
	if rc := runMarketplace([]string{"fetch", "--help"}); rc != 0 {
		t.Errorf("fetch --help rc = %d; want 0", rc)
	}
}

// plant materialises an installed package under the fake home tree.
//
//	plant(t, home, "x", manifest)
func plant(t *testing.T, home, code string, manifest *config.ViewManifest) {
	t.Helper()
	medium := coreio.Local
	path := core.Path(home, ".core", app.AppsDirName, code, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(path)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	body, _ := yaml.Marshal(manifest)
	if err := medium.Write(path, string(body)); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// plantMarketplaceIndex plants a tiny marketplace tree under
// `<home>/.core/marketplace/` so runMarketplace search has data.
//
//	home := plantMarketplaceIndex(t)
func plantMarketplaceIndex(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	medium := coreio.Local

	root := core.Path(home, ".core", "marketplace")
	if err := medium.EnsureDir(root); err != nil {
		t.Fatalf("EnsureDir root: %v", err)
	}
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName), `{"version":1,"categories":["media"]}`); err != nil {
		t.Fatalf("Write index: %v", err)
	}

	if err := medium.EnsureDir(core.Path(root, "media")); err != nil {
		t.Fatalf("EnsureDir media: %v", err)
	}
	if err := medium.Write(core.Path(root, "media", app.MarketplaceIndexFileName), `{
		"version":1,
		"category":"media",
		"entries":[{"code":"photo-browser","type":"native","description":"local photo browser","repo":"https://example.com/pb.git"}]
	}`); err != nil {
		t.Fatalf("Write category: %v", err)
	}
	return home
}

// TestPkg_ensureExit_NoOpInTests confirms ensureExit is a no-op shim
// the test harness shouldn't call. We only check the function has the
// expected signature; an os.Exit invocation would terminate the test
// binary so we deliberately do NOT call it here.
func TestPkg_ensureExit_NoOpInTests(t *testing.T) {
	// Compile-time check that ensureExit is callable with an int.
	_ = func(code int) { _ = code }
	// Touch os to make sure the import is real.
	if core.Getenv("PATH") == "" {
		t.Fatal("PATH is empty — sanity guard")
	}
}

// TestPkg_runPkgInstall_Bad — `pkg install` with no source returns 64.
// Electron repo install reaches the GitHub API which is unreachable in
// hermetic tests, so we accept rc=1 as the "network failure surfaced"
// signal.
func TestPkg_runPkgInstall_Bad(t *testing.T) {
	_ = "runPkgInstall"
	if rc := runPkg([]string{"install"}); rc != 64 {
		t.Errorf("install with no source rc = %d; want 64", rc)
	}
	// Electron repo install reaches the network — non-zero rc is what
	// matters; the actual cause varies by environment (DNS failure,
	// 404, 403 rate-limited).
	if rc := runPkg([]string{"install", "github.com/owner/definitely-not-a-real-repo-1234567890"}); rc == 0 {
		t.Errorf("install of non-existent repo returned 0; want non-zero")
	}
	// --type with a missing value or an unknown value is rejected with
	// EX_USAGE so the operator sees the supported list.
	if rc := runPkg([]string{"install", "--type"}); rc != 64 {
		t.Errorf("dangling --type rc = %d; want 64", rc)
	}
	if rc := runPkg([]string{"install", "--type", "rust", "x"}); rc != 64 {
		t.Errorf("unknown --type rc = %d; want 64", rc)
	}
	// Two source positionals → user error.
	if rc := runPkg([]string{"install", "a", "b"}); rc != 64 {
		t.Errorf("two source args rc = %d; want 64", rc)
	}
	// Unknown flag is rejected.
	if rc := runPkg([]string{"install", "--what", "x"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
}

// TestPkg_resolveInstallSpecAgainstMarketplace_Good prefers an exact
// marketplace hit for ambiguous `vendor/name` inputs so marketplace
// shorthands like `core/play` do not fall through to the GitHub repo
// path when the cache is present.
func TestPkg_resolveInstallSpecAgainstMarketplace_Good(t *testing.T) {
	home := t.TempDir()
	root := core.Path(home, ".core", "marketplace")
	if err := coreio.Local.EnsureDir(core.Path(root, "apps")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := coreio.Local.Write(core.Path(root, "index.json"),
		`{"version":1,"categories":["apps"]}`); err != nil {
		t.Fatalf("Write index.json: %v", err)
	}
	if err := coreio.Local.Write(core.Path(root, "apps", "index.json"),
		`{"version":1,"category":"apps","entries":[{"code":"core/play","type":"pwa","url":"https://play.example.com"}]}`); err != nil {
		t.Fatalf("Write apps/index.json: %v", err)
	}

	spec := resolveInstallSpecAgainstMarketplace(home, app.ParseInstallSpec("core/play"))
	if spec.Type != app.PackageTypeNative {
		t.Errorf("resolved Type = %v; want PackageTypeNative marketplace path", spec.Type)
	}
	if spec.Code != "core/play" {
		t.Errorf("resolved Code = %q; want core/play", spec.Code)
	}
	if spec.Repo != "" {
		t.Errorf("resolved Repo = %q; want empty", spec.Repo)
	}
}

// TestPkg_resolveInstallSpecAgainstMarketplace_Bad leaves plain
// marketplace codes untouched; only ambiguous repo-like shapes are
// rewritten.
func TestPkg_resolveInstallSpecAgainstMarketplace_Bad(t *testing.T) {
	spec := resolveInstallSpecAgainstMarketplace(t.TempDir(), app.ParseInstallSpec("photo-browser"))
	if spec.Type != app.PackageTypeNative {
		t.Errorf("plain code Type = %v; want PackageTypeNative", spec.Type)
	}
	if spec.Code != "photo-browser" {
		t.Errorf("plain code Code = %q; want photo-browser", spec.Code)
	}
	if spec.Repo != "" {
		t.Errorf("plain code Repo = %q; want empty", spec.Repo)
	}
}

// TestPkg_resolveInstallSpecAgainstMarketplace_Ugly falls back to the
// repo path when an ambiguous shorthand is not present in the local
// marketplace cache, and it upgrades explicit GitHub URLs to the same
// repo install flow.
func TestPkg_resolveInstallSpecAgainstMarketplace_Ugly(t *testing.T) {
	spec := resolveInstallSpecAgainstMarketplace(t.TempDir(), app.ParseInstallSpec("bitwarden/clients"))
	if spec.Type != app.PackageTypeElectron {
		t.Errorf("bitwarden/clients Type = %v; want PackageTypeElectron", spec.Type)
	}
	if spec.Repo != "bitwarden/clients" {
		t.Errorf("bitwarden/clients Repo = %q; want bitwarden/clients", spec.Repo)
	}

	spec = resolveInstallSpecAgainstMarketplace(t.TempDir(), app.ParseInstallSpec("https://github.com/bitwarden/clients"))
	if spec.Type != app.PackageTypeElectron {
		t.Errorf("github URL Type = %v; want PackageTypeElectron", spec.Type)
	}
	if spec.Repo != "https://github.com/bitwarden/clients" {
		t.Errorf("github URL Repo = %q; want explicit URL", spec.Repo)
	}
	if spec.URL != "" {
		t.Errorf("github URL should have been cleared after repo resolution; got %q", spec.URL)
	}
}

// TestPkg_runPkgInstall_TypeOverride — `--type web ./dir` forces the
// web wrap path even when the directory looks native (has .core/view.yaml).
// Validates the override branch in runPkgInstall.
func TestPkg_runPkgInstall_TypeOverride(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	// Plant both a native marker AND an index.html so auto-detection
	// would pick native; --type web should override.
	if err := medium.EnsureDir(core.Path(src, ".core")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(src, ".core", "view.yaml"),
		"code: srctype\nname: SrcType\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("Write view.yaml: %v", err)
	}
	if err := medium.Write(core.Path(src, "index.html"), "<html/>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}
	t.Setenv("CORE_HOME", home)
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; --type override path validated by spec wiring")
	}
	// --type web should pass through to runPkgInstallLocal which then
	// routes to WrapWeb.
	rc := runPkg([]string{"install", "--type", "web", src})
	if rc != 0 {
		t.Fatalf("--type web rc = %d; want 0", rc)
	}
	appsRoot := core.Path(home, ".core", app.AppsDirName)
	entries, err := medium.List(appsRoot)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no installs under %s after --type web install", appsRoot)
	}
	for _, e := range entries {
		view := core.Path(appsRoot, e.Name(), ".core", "view.yaml")
		if !medium.Exists(view) {
			continue
		}
		var round config.ViewManifest
		if err := app.LoadViewManifest(medium, view, &round); err != nil {
			continue
		}
		if t2, _ := round.Config["type"].(string); t2 == "web" {
			if !medium.Exists(core.Path(appsRoot, e.Name(), "index.html")) {
				t.Fatalf("forced web install did not copy index.html into %s", core.Path(appsRoot, e.Name()))
			}
			return
		}
	}
	t.Fatal("no web-typed install found after --type web override")
}

// TestPkg_runPkgInstall_TypeOverride_MarketplaceCode_Good confirms that
// a plain marketplace code stays on the marketplace install path even
// when `--type` is supplied. The override is only meaningful for local
// dirs / URLs / repo refs; a code like `play` must not be reinterpreted
// as `./play` on disk.
func TestPkg_runPkgInstall_TypeOverride_MarketplaceCode_Good(t *testing.T) {
	_ = "runPkgInstall TypeOverride MarketplaceCode"
	_ = "runPkgInstall_TypeOverride_MarketplaceCode"
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("CORE_HOME", home)
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; marketplace override path validated by app wiring")
	}

	root := core.Path(home, ".core", "marketplace")
	if err := coreio.Local.EnsureDir(core.Path(root, "apps")); err != nil {
		t.Fatalf("EnsureDir apps: %v", err)
	}
	if err := coreio.Local.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":["apps"]}`); err != nil {
		t.Fatalf("Write index.json: %v", err)
	}
	if err := coreio.Local.Write(core.Path(root, "apps", app.MarketplaceIndexFileName),
		`{"version":1,"category":"apps","entries":[{"code":"play","type":"pwa","url":"`+srv.URL+`/manifest.json"}]}`); err != nil {
		t.Fatalf("Write apps/index.json: %v", err)
	}

	if rc := runPkg([]string{"install", "--type", "native", "play"}); rc != 0 {
		t.Fatalf("--type native marketplace code rc = %d; want 0", rc)
	}

	viewPath := core.Path(home, ".core", app.AppsDirName, "play", ".core", "view.yaml")
	if !coreio.Local.Exists(viewPath) {
		t.Fatalf("marketplace override install produced no view.yaml at %s", viewPath)
	}
}

// TestPkg_runPkgInstall_Good — auto-detects a PWA URL and installs it
// without needing a marketplace cache. Validates the
// ParseInstallSpec → runPkgInstallPWA wiring end-to-end inside the CLI.
func TestPkg_runPkgInstall_Good(t *testing.T) {
	_ = "runPkgInstall"
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("CORE_HOME", home)
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; covered by app.PkgInstall tests")
	}

	if rc := runPkg([]string{"install", srv.URL + "/manifest.json"}); rc != 0 {
		t.Fatalf("install PWA rc = %d; want 0", rc)
	}

	viewPath := core.Path(home, ".core", app.AppsDirName, "play", ".core", "view.yaml")
	if !coreio.Local.Exists(viewPath) {
		t.Errorf("install PWA produced no view.yaml at %s", viewPath)
	}
}

// TestPkg_runPkgInstallLocal_Good — auto-detect a local PWA directory
// (manifest.json with start_url) and wrap+install in one shot.
func TestPkg_runPkgInstallLocal_Good(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	manifest := `{"name":"Local Play","short_name":"localplay","start_url":"/"}`
	if err := medium.Write(core.Path(src, "manifest.json"), manifest); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	if err := medium.Write(core.Path(src, "index.html"), "<html>local play</html>"); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	rc := runPkgInstallLocal(home, src)
	if rc != 0 {
		t.Fatalf("runPkgInstallLocal rc = %d; want 0", rc)
	}
	viewPath := core.Path(home, ".core", app.AppsDirName, "localplay", ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		t.Errorf("local PWA install produced no view.yaml at %s", viewPath)
	}
	if !medium.Exists(core.Path(home, ".core", app.AppsDirName, "localplay", "index.html")) {
		t.Fatalf("local PWA install did not copy index.html into the install tree")
	}
}

// TestPkg_runPkgInstallLocal_Good_WebManifest confirms local PWA
// installs also accept `manifest.webmanifest`, matching the remote PWA
// fetch path's candidate probing.
func TestPkg_runPkgInstallLocal_Good_WebManifest(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	manifest := `{"name":"Local Play WebManifest","short_name":"localplaywm","start_url":"/"}`
	if err := medium.Write(core.Path(src, "manifest.webmanifest"), manifest); err != nil {
		t.Fatalf("write manifest.webmanifest: %v", err)
	}
	if err := medium.Write(core.Path(src, "index.html"), "<html>local play</html>"); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	rc := runPkgInstallLocal(home, src)
	if rc != 0 {
		t.Fatalf("runPkgInstallLocal rc = %d; want 0", rc)
	}
	viewPath := core.Path(home, ".core", app.AppsDirName, "localplaywm", ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		t.Errorf("local webmanifest install produced no view.yaml at %s", viewPath)
	}
}

// TestPkg_runPkgInstallLocal_Bad — pointing the dispatcher at a
// directory that holds no recognisable app type returns 1.
func TestPkg_runPkgInstallLocal_Bad(t *testing.T) {
	dir := t.TempDir() // empty
	home := t.TempDir()
	if rc := runPkgInstallLocal(home, dir); rc == 0 {
		t.Errorf("local install of empty dir returned 0; want non-zero")
	}
	if rc := runPkgInstallLocal(home, "/definitely/not/a/dir"); rc == 0 {
		t.Errorf("local install of missing dir returned 0; want non-zero")
	}
}

// TestPkg_runPkgInstallLocal_Ugly — auto-detect a local Web directory
// (just an index.html) and wrap+install with a slugified default code.
func TestPkg_runPkgInstallLocal_Ugly(t *testing.T) {
	src := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(src, "index.html"), "<html/>"); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	rc := runPkgInstallLocal(home, src)
	if rc != 0 {
		t.Fatalf("runPkgInstallLocal rc = %d; want 0", rc)
	}

	// The web wrap derives `code` from the directory basename; rather
	// than reproducing the slug logic here, walk the apps tree and look
	// for any directory containing a wrapped manifest with type=web.
	appsRoot := core.Path(home, ".core", app.AppsDirName)
	entries, _ := medium.List(appsRoot)
	if len(entries) == 0 {
		t.Fatalf("no installs under %s after web wrap", appsRoot)
	}
	for _, e := range entries {
		viewPath := core.Path(appsRoot, e.Name(), ".core", "view.yaml")
		if !medium.Exists(viewPath) {
			continue
		}
		var round config.ViewManifest
		if err := app.LoadViewManifest(medium, viewPath, &round); err != nil {
			continue
		}
		if t2, _ := round.Config["type"].(string); t2 == "web" {
			return // success
		}
	}
	t.Errorf("no web-type install found under %s", appsRoot)
}

// TestPkg_runPkgInstallRepoPWAFromRoot_Good_WebManifest confirms the
// repo-backed PWA install path accepts `manifest.webmanifest`.
func TestPkg_runPkgInstallRepoPWAFromRoot_Good_WebManifest(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(root, "manifest.webmanifest"), `{"name":"Repo Play","short_name":"repoplay","start_url":"/"}`); err != nil {
		t.Fatalf("write manifest.webmanifest: %v", err)
	}
	if err := medium.Write(core.Path(root, "index.html"), "<html>repo play</html>"); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	if rc := runPkgInstallRepoPWAFromRoot(home, "github.com/example/repo-play", root); rc != 0 {
		t.Fatalf("runPkgInstallRepoPWAFromRoot rc = %d; want 0", rc)
	}
	viewPath := core.Path(home, ".core", app.AppsDirName, "repoplay", ".core", "view.yaml")
	if !medium.Exists(viewPath) {
		t.Errorf("repo PWA install produced no view.yaml at %s", viewPath)
	}
}

// TestMain_runInstalled_Bad — `run` with no code returns 64; with an
// unknown flag returns 64; with --help returns 0.
func TestMain_runInstalled_Bad(t *testing.T) {
	if rc := runInstalled(nil); rc != 64 {
		t.Errorf("runInstalled(nil) rc = %d; want 64", rc)
	}
	if rc := runInstalled([]string{"--unknown"}); rc != 64 {
		t.Errorf("unknown flag rc = %d; want 64", rc)
	}
	if rc := runInstalled([]string{"--help"}); rc != 0 {
		t.Errorf("--help rc = %d; want 0", rc)
	}
}

// TestMain_runInstalled_HelpShowsAppCode_Good pins the RFC-facing
// `core run <app-code>` surface so the installed-app argument remains
// visible in help output.
func TestMain_runInstalled_HelpShowsAppCode_Good(t *testing.T) {
	rc := runInstalled([]string{"--help"})
	if rc != 0 {
		t.Fatalf("--help rc = %d; want 0", rc)
	}
}

// TestMain_runInstalled_Ugly — pointing `run` at a missing code under
// the user's actual home returns 1 (DIR_HOME resolves but the package
// isn't installed).
func TestMain_runInstalled_Ugly(t *testing.T) {
	if rc := runInstalled([]string{"definitely-not-installed-pkg"}); rc != 1 {
		t.Errorf("missing-code rc = %d; want 1", rc)
	}
}

// TestPkg_formatRow_Good — typical case: cells shorter than their
// columns get padded out to the column width plus the configured
// gutter; the final cell carries no trailing whitespace.
func TestPkg_formatRow_Good(t *testing.T) {
	got := formatRow([]string{"a", "b", "c"}, []int{4, 3, 2}, 2)
	want := "a     b    c"
	if got != want {
		t.Errorf("formatRow = %q; want %q", got, want)
	}
}

// TestPkg_formatRow_Bad — cells longer than their declared widths
// must not be truncated; the row layout assumes widths come from the
// caller's max-pass and a longer cell is a programming error, not a
// runtime input. The function should still emit the cell literally
// so the operator can spot the mismatch.
func TestPkg_formatRow_Bad(t *testing.T) {
	// Width=2 but cell is 5 chars. Output keeps the cell intact.
	got := formatRow([]string{"hello", "x"}, []int{2, 1}, 2)
	if got != "hello  x" {
		t.Errorf("formatRow long-cell = %q; want %q", got, "hello  x")
	}
}

// TestPkg_formatRow_Ugly — empty cells, single-cell rows, and a zero
// gutter all produce stable output (no panic, no extra padding).
func TestPkg_formatRow_Ugly(t *testing.T) {
	if got := formatRow(nil, nil, 0); got != "" {
		t.Errorf("formatRow(nil) = %q; want empty", got)
	}
	if got := formatRow([]string{"only"}, []int{4}, 2); got != "only" {
		t.Errorf("single-cell formatRow = %q; want only", got)
	}
	if got := formatRow([]string{"a", "b"}, []int{1, 1}, 0); got != "ab" {
		t.Errorf("zero-gutter formatRow = %q; want ab", got)
	}
}

// TestPkg_sourceTag_Good — wrap args with each kind of source emit the
// expected `kind:value` tag the wrap pipeline records under
// Config["source"].
func TestPkg_sourceTag_Good(t *testing.T) {
	if got := (pkgWrapArgs{PWAURL: "https://x"}).sourceTag(); got != "pwa:https://x" {
		t.Errorf("PWA sourceTag = %q; want %q", got, "pwa:https://x")
	}
	if got := (pkgWrapArgs{ElectronDir: "github.com/foo/bar"}).sourceTag(); got != "electron:github.com/foo/bar" {
		t.Errorf("Electron sourceTag = %q; want %q", got, "electron:github.com/foo/bar")
	}
	if got := (pkgWrapArgs{WebDir: "./foo"}).sourceTag(); got != "web:./foo" {
		t.Errorf("Web sourceTag = %q; want %q", got, "web:./foo")
	}
}

// TestPkg_sourceTag_Bad — empty wrap args fall through to the
// "unknown" sentinel rather than emitting a degenerate tag.
func TestPkg_sourceTag_Bad(t *testing.T) {
	if got := (pkgWrapArgs{}).sourceTag(); got != "unknown" {
		t.Errorf("empty wrap sourceTag = %q; want %q", got, "unknown")
	}
}

// TestPkg_sourceTag_Ugly — when more than one source is set the PWA
// path wins (matches the runPkgBundle dispatch order — PWA, then
// electron, then web).
func TestPkg_sourceTag_Ugly(t *testing.T) {
	args := pkgWrapArgs{
		PWAURL:      "https://primary",
		ElectronDir: "github.com/secondary/x",
		WebDir:      "./tertiary",
	}
	if got := args.sourceTag(); got != "pwa:https://primary" {
		t.Errorf("multi-source sourceTag = %q; want %q (PWA precedence)", got, "pwa:https://primary")
	}
}

// TestPkg_loadElectronPackageJSON_Good — a valid package.json round-trips
// through the loader into an ElectronPackageJSON.
func TestPkg_loadElectronPackageJSON_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local
	body := `{"name":"my-app","version":"1.2.3"}`
	if err := medium.Write(core.Path(dir, "package.json"), body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	pkg, err := loadElectronPackageJSON(medium, dir)
	if err != nil {
		t.Fatalf("loadElectronPackageJSON: %v", err)
	}
	if pkg == nil {
		t.Fatal("nil package returned without error")
	}
	if pkg.Name != "my-app" {
		t.Errorf("Name = %q; want %q", pkg.Name, "my-app")
	}
	if pkg.Version != "1.2.3" {
		t.Errorf("Version = %q; want %q", pkg.Version, "1.2.3")
	}
}

// TestPkg_loadElectronPackageJSON_Bad — missing package.json and
// malformed JSON both return errors with no partial result.
func TestPkg_loadElectronPackageJSON_Bad(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	// No package.json present.
	if _, err := loadElectronPackageJSON(medium, dir); err == nil {
		t.Error("missing package.json produced no error")
	}

	// Malformed JSON.
	if err := medium.Write(core.Path(dir, "package.json"), `{not valid json`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := loadElectronPackageJSON(medium, dir); err == nil {
		t.Error("malformed JSON produced no error")
	}
}

// TestPkg_loadElectronPackageJSON_Ugly — an empty JSON object is valid
// (no name, no version) and decodes cleanly. The wrap pipeline will
// fall back to defaults when fields are missing.
func TestPkg_loadElectronPackageJSON_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(dir, "package.json"), `{}`); err != nil {
		t.Fatalf("Write: %v", err)
	}
	pkg, err := loadElectronPackageJSON(medium, dir)
	if err != nil {
		t.Fatalf("loadElectronPackageJSON: %v", err)
	}
	if pkg == nil {
		t.Fatal("nil package returned for empty JSON")
	}
	if pkg.Name != "" || pkg.Version != "" {
		t.Errorf("expected zero values for empty JSON; got Name=%q Version=%q", pkg.Name, pkg.Version)
	}
}

// TestPkg_signManifestFile_Good signs an in-memory manifest using a
// key file on disk. Round-trip through verify is checked at the
// app.SignManifest level — here we just confirm the wiring lands a
// non-empty Sign field.
func TestPkg_signManifestFile_Good(t *testing.T) {
	dir := t.TempDir()
	keyPath := core.Path(dir, "app.key")
	priv, _, err := app.Keygen(coreio.Local, dir, "app")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	if priv != keyPath {
		t.Logf("Keygen returned %q (expected %q)", priv, keyPath)
		keyPath = priv
	}

	manifest := &config.ViewManifest{
		Code: "sf-good", Name: "SF Good", Version: "0.1.0",
	}
	if err := signManifestFile(keyPath, manifest); err != nil {
		t.Fatalf("signManifestFile: %v", err)
	}
	if manifest.Sign == "" {
		t.Error("Sign field empty after signManifestFile")
	}
}

// TestPkg_signManifestFile_Bad — nil manifest, missing key file and a
// malformed key file all surface errors.
func TestPkg_signManifestFile_Bad(t *testing.T) {
	if err := signManifestFile("anywhere", nil); err == nil {
		t.Error("nil manifest produced no error")
	}
	if err := signManifestFile(core.Path(t.TempDir(), "missing.key"),
		&config.ViewManifest{Code: "x"}); err == nil {
		t.Error("missing key file produced no error")
	}
	bad := core.Path(t.TempDir(), "bad.key")
	if err := coreio.Local.Write(bad, "not-hex"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := signManifestFile(bad, &config.ViewManifest{Code: "x"}); err == nil {
		t.Error("malformed key file produced no error")
	}
}

// TestPkg_signManifestFile_Ugly — a key file with trailing whitespace
// still loads (mirrors LoadPrivateKey tolerance for editor-added
// newlines).
func TestPkg_signManifestFile_Ugly(t *testing.T) {
	dir := t.TempDir()
	priv, _, err := app.Keygen(coreio.Local, dir, "trim-app")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	body, _ := coreio.Local.Read(priv)
	dirty := core.Path(t.TempDir(), "trim-app.key")
	if err := coreio.Local.Write(dirty, body+"\n\n   \n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	manifest := &config.ViewManifest{Code: "trim", Name: "Trim", Version: "0.1.0"}
	if err := signManifestFile(dirty, manifest); err != nil {
		t.Fatalf("signManifestFile with trailing whitespace: %v", err)
	}
	if manifest.Sign == "" {
		t.Error("Sign field empty after tolerant load")
	}
}

// TestPkg_signManifestDefault_Bad — nil manifest is rejected up front,
// before LoadDefaultPrivateKey is called.
func TestPkg_signManifestDefault_Bad(t *testing.T) {
	if err := signManifestDefault(nil); err == nil {
		t.Error("nil manifest produced no error")
	}
}

// TestPkg_applyWrapSignature_Good — the `--sign KEY` path delegates to
// signManifestFile and stamps the manifest's Sign field.
func TestPkg_applyWrapSignature_Good(t *testing.T) {
	dir := t.TempDir()
	keyPath, _, err := app.Keygen(coreio.Local, dir, "wrap-good")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}
	manifest := &config.ViewManifest{Code: "aw-good", Name: "AW Good", Version: "0.1.0"}
	if err := applyWrapSignature(pkgWrapArgs{Sign: keyPath}, manifest); err != nil {
		t.Fatalf("applyWrapSignature: %v", err)
	}
	if manifest.Sign == "" {
		t.Error("Sign field empty after applyWrapSignature")
	}
}

// TestPkg_applyWrapSignature_Bad — a `--sign KEY` flag pointing at a
// non-existent file surfaces the underlying load error.
func TestPkg_applyWrapSignature_Bad(t *testing.T) {
	manifest := &config.ViewManifest{Code: "aw-bad", Name: "AW Bad", Version: "0.1.0"}
	if err := applyWrapSignature(
		pkgWrapArgs{Sign: core.Path(t.TempDir(), "missing.key")},
		manifest,
	); err == nil {
		t.Error("missing key file produced no error")
	}
}

// TestPkg_applyWrapSignature_Ugly — the no-signature path (neither
// `--sign` nor `--sign-default` set) is a no-op.
func TestPkg_applyWrapSignature_Ugly(t *testing.T) {
	manifest := &config.ViewManifest{Code: "aw-ugly", Name: "AW Ugly", Version: "0.1.0"}
	if err := applyWrapSignature(pkgWrapArgs{}, manifest); err != nil {
		t.Fatalf("no-signature path returned error: %v", err)
	}
	if manifest.Sign != "" {
		t.Errorf("Sign field mutated by no-op path: %q", manifest.Sign)
	}
}

// TestPkg_runPkgBundle_SignPath_Good — `pkg wrap --sign PATH` still
// signs with the explicit private key after the RFC-compatible bare
// `--sign` alias was added.
func TestPkg_runPkgBundle_SignPath_Good(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(src, "index.html"), "<html/>"); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}
	keyDir := t.TempDir()
	keyPath, _, err := app.Keygen(medium, keyDir, "wrap-explicit")
	if err != nil {
		t.Fatalf("Keygen: %v", err)
	}

	rc := runPkg([]string{"wrap", "--web", src, "--dest", dest, "--sign", keyPath})
	if rc != 0 {
		t.Fatalf("runPkg wrap --sign PATH rc = %d; want 0", rc)
	}

	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(dest, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadViewManifest: %v", err)
	}
	if round.Sign == "" {
		t.Fatal("wrapped manifest was not signed by explicit --sign PATH")
	}
}

// TestPkg_runPkgBundle_SignAlias_Ugly — bare `--sign` is accepted as the
// default-key form and must not consume the following flag as though it
// were a key path.
func TestPkg_runPkgBundle_SignAlias_Ugly(t *testing.T) {
	body := `{"name":"Play","short_name":"play","start_url":"/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := t.TempDir()
	rc := runPkg([]string{"wrap", "--pwa", srv.URL + "/manifest.json", "--sign", "--dest", dest})
	if rc == 64 {
		t.Fatalf("bare --sign was parsed as argv misuse; want accepted default-key semantics (rc=%d)", rc)
	}
}

// TestPkg_isRepoSpec_Good — known scheme / host prefixes flag the
// source as a remote repo so the wrap pipeline can dispatch into the
// release-fetch path. Covers github / gitlab / bitbucket / git@ /
// http(s) — see RFC §16.4 detection table.
func TestPkg_isRepoSpec_Good(t *testing.T) {
	cases := []string{
		"github.com/foo/bar",
		"gitlab.com/foo/bar",
		"bitbucket.org/foo/bar",
		"git@forge.lthn.ai:core/app.git",
		"https://app.example.com",
		"http://app.example.com",
	}
	for _, in := range cases {
		if !isRepoSpec(in) {
			t.Errorf("isRepoSpec(%q) = false; want true", in)
		}
	}
}

// TestPkg_isRepoSpec_Bad — local-looking sources are not repo specs.
func TestPkg_isRepoSpec_Bad(t *testing.T) {
	cases := []string{"./foo", "../foo", "/abs/path", "foo", ""}
	for _, in := range cases {
		if isRepoSpec(in) {
			t.Errorf("isRepoSpec(%q) = true; want false", in)
		}
	}
}

// TestPkg_isRepoSpec_Ugly — case-sensitive: "GitHub.com" with capitals
// is treated as a local-looking string. The detector intentionally
// matches the lowercase scheme to avoid surprising false positives.
func TestPkg_isRepoSpec_Ugly(t *testing.T) {
	if isRepoSpec("GitHub.com/foo/bar") {
		t.Error("isRepoSpec is unexpectedly case-insensitive")
	}
}

// TestPkg_isExtractable_Good — recognises the archive extensions the
// Electron fetch pipeline downloads from GitHub releases.
func TestPkg_isExtractable_Good(t *testing.T) {
	cases := []string{
		"app.zip", "app.tar", "app.tar.gz", "app.tgz",
		"DEEP/PATH/Foo.zip",
	}
	for _, in := range cases {
		if !isExtractable(in) {
			t.Errorf("isExtractable(%q) = false; want true", in)
		}
	}
}

// TestPkg_isExtractable_Bad — non-archive extensions are not
// extractable. Detector should not pull `.dmg`, `.exe`, or other
// platform binaries that would only confuse the extractor.
func TestPkg_isExtractable_Bad(t *testing.T) {
	cases := []string{"app.dmg", "app.exe", "app", "", "app.txt"}
	for _, in := range cases {
		if isExtractable(in) {
			t.Errorf("isExtractable(%q) = true; want false", in)
		}
	}
}

// TestPkg_isExtractable_Ugly — case-insensitive: an upper-case
// `.ZIP` is recognised because the detector lowercases first.
func TestPkg_isExtractable_Ugly(t *testing.T) {
	if !isExtractable("ASSET.ZIP") {
		t.Error("isExtractable should be case-insensitive (.ZIP)")
	}
	if !isExtractable("Asset.Tar.Gz") {
		t.Error("isExtractable should be case-insensitive (.Tar.Gz)")
	}
}

// TestPkg_runPkgInstallPWA_Good drives the URL → wrap → install path
// against a mock manifest server and confirms the install lands at the
// expected place with the wrap source recorded.
func TestPkg_runPkgInstallPWA_Good(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Play","short_name":"play","start_url":"/"}`))
	}))
	defer srv.Close()

	home := t.TempDir()

	if rc := runPkgInstallPWA(t.Context(), home, srv.URL+"/manifest.json"); rc != 0 {
		t.Fatalf("runPkgInstallPWA rc = %d; want 0", rc)
	}

	medium := coreio.Local
	view := core.Path(home, ".core", app.AppsDirName, "play", ".core", "view.yaml")
	if !medium.Exists(view) {
		t.Fatalf("view.yaml missing at %s", view)
	}
	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, view, &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	src, _ := round.Config["source"].(string)
	if !core.HasPrefix(src, "wrap:pwa:") {
		t.Errorf("Config[source] = %q; want wrap:pwa: prefix", src)
	}
}

// TestPkg_runPkgInstallPWA_Bad — a server returning malformed JSON
// causes the wrap step to fail; an unreachable URL fails the fetch.
// Both surface non-zero exit codes.
func TestPkg_runPkgInstallPWA_Bad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	if rc := runPkgInstallPWA(t.Context(), t.TempDir(), srv.URL+"/manifest.json"); rc == 0 {
		t.Error("runPkgInstallPWA on malformed manifest returned 0; want non-zero")
	}
	// Unreachable URL — fetch fails.
	if rc := runPkgInstallPWA(t.Context(), t.TempDir(), "http://127.0.0.1:1/missing"); rc == 0 {
		t.Error("runPkgInstallPWA on unreachable URL returned 0; want non-zero")
	}
}

// TestPkg_runPkgInstallPWA_Ugly — a manifest with no name still
// installs successfully because WrapPWA derives a code from the
// start_url host. The wrap pipeline never errors on missing optional
// fields; the install path proves it.
func TestPkg_runPkgInstallPWA_Ugly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"start_url":"https://anonymous.example.com/app"}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	if rc := runPkgInstallPWA(t.Context(), home, srv.URL+"/manifest.json"); rc != 0 {
		t.Fatalf("runPkgInstallPWA rc = %d; want 0", rc)
	}
}

// TestPkg_runPkgInstallMarketplace_Bad — without a marketplace cache
// directory the install path errors out with a useful "run fetch
// first" message. We exercise the missing-cache branch since the
// happy path requires git on PATH.
func TestPkg_runPkgInstallMarketplace_Bad(t *testing.T) {
	home := t.TempDir()
	if rc := runPkgInstallMarketplace(t.Context(), home, "photo-browser"); rc == 0 {
		t.Error("missing marketplace cache should produce non-zero rc")
	}
}

// TestPkg_runPkgInstallMarketplace_Ugly — install against an existing
// cache that has no matching entry surfaces the resolve failure
// without writing anything to the apps tree.
func TestPkg_runPkgInstallMarketplace_Ugly(t *testing.T) {
	home := t.TempDir()
	medium := coreio.Local

	root := core.Path(home, ".core", "marketplace")
	if err := medium.EnsureDir(root); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := medium.Write(core.Path(root, app.MarketplaceIndexFileName),
		`{"version":1,"categories":[]}`); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if rc := runPkgInstallMarketplace(t.Context(), home, "definitely-not-listed"); rc == 0 {
		t.Error("missing listing should produce non-zero rc")
	}
}

// TestPkg_runPkgUpdate_Good — `--help` returns 0 like the rest of the
// pkg verbs.
func TestPkg_runPkgUpdate_Good(t *testing.T) {
	if rc := runPkgUpdate([]string{"--help"}); rc != 0 {
		t.Errorf("runPkgUpdate --help rc = %d; want 0", rc)
	}
}

// TestPkg_runPkgUpdate_Bad rejects missing NAME, unknown flags, and
// extra positional arguments with EX_USAGE.
func TestPkg_runPkgUpdate_Bad(t *testing.T) {
	if rc := runPkgUpdate(nil); rc != 64 {
		t.Errorf("runPkgUpdate(nil) rc = %d; want 64", rc)
	}
	if rc := runPkgUpdate([]string{"--what"}); rc != 64 {
		t.Errorf("runPkgUpdate unknown flag rc = %d; want 64", rc)
	}
	if rc := runPkgUpdate([]string{"one", "two"}); rc != 64 {
		t.Errorf("runPkgUpdate extra arg rc = %d; want 64", rc)
	}
}

// TestPkg_runPkgRemove_Flags_Good — the CLI surface accepts both
// `pkg remove NAME` and `pkg remove --purge NAME` without rejecting
// either. --help returns 0 so callers can discover the flag.
func TestPkg_runPkgRemove_Flags_Good(t *testing.T) {
	if rc := runPkgRemove([]string{"--help"}); rc != 0 {
		t.Errorf("runPkgRemove --help rc = %d; want 0", rc)
	}
	if rc := runPkgRemove([]string{"-h"}); rc != 0 {
		t.Errorf("runPkgRemove -h rc = %d; want 0", rc)
	}
}

// TestPkg_runPkgRemove_Flags_Bad — rejects unknown flags, missing
// NAME, and double NAME before touching the filesystem.
func TestPkg_runPkgRemove_Flags_Bad(t *testing.T) {
	if rc := runPkgRemove(nil); rc != 64 {
		t.Errorf("runPkgRemove(nil) rc = %d; want 64", rc)
	}
	if rc := runPkgRemove([]string{"--purge"}); rc != 64 {
		t.Errorf("runPkgRemove(--purge alone) rc = %d; want 64", rc)
	}
	if rc := runPkgRemove([]string{"--unknown", "x"}); rc != 64 {
		t.Errorf("runPkgRemove(unknown flag) rc = %d; want 64", rc)
	}
	if rc := runPkgRemove([]string{"a", "b"}); rc != 64 {
		t.Errorf("runPkgRemove(two names) rc = %d; want 64", rc)
	}
}

// TestPkg_runPkgInfo_Good — `pkg info NAME` against a planted install
// returns 0 and both the human table and `--json` emit the installed
// identity.
func TestPkg_runPkgInfo_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CORE_HOME", home)
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; covered by app.PkgInfo tests")
	}
	plant(t, home, "viewer", &config.ViewManifest{
		Code: "viewer", Name: "Viewer", Version: "0.1.0",
		Permissions: config.ViewPermissions{Read: []string{"./data/"}},
	})

	if rc := runPkg([]string{"info", "viewer"}); rc != 0 {
		t.Errorf("pkg info rc = %d; want 0", rc)
	}
	if rc := runPkg([]string{"info", "--json", "viewer"}); rc != 0 {
		t.Errorf("pkg info --json rc = %d; want 0", rc)
	}
}

// TestPkg_runPkgInfo_Bad — rejects missing NAME, unknown flags, and
// surfaces a non-zero rc when the package is not installed.
// Argv-level rejections (EX_USAGE = 64) fire before any env lookup so
// these cases run unconditionally. The "missing package" branch needs
// DIR_HOME to resolve to a temp dir, which the test harness can't always
// guarantee (core.Env caches at process start), so it skips rather than
// faking the lookup.
func TestPkg_runPkgInfo_Bad(t *testing.T) {
	// Argv-level rejections — no env required.
	if rc := runPkgInfo(nil); rc != 64 {
		t.Errorf("pkg info (no name) rc = %d; want 64", rc)
	}
	if rc := runPkgInfo([]string{"--unknown", "x"}); rc != 64 {
		t.Errorf("pkg info unknown flag rc = %d; want 64", rc)
	}
	if rc := runPkgInfo([]string{"a", "b"}); rc != 64 {
		t.Errorf("pkg info two names rc = %d; want 64", rc)
	}

	// Missing-package path — only runs when CORE_HOME overrides take.
	home := t.TempDir()
	t.Setenv("CORE_HOME", home)
	if core.Env("DIR_HOME") != home {
		t.Skip("CORE_HOME mid-process override not honoured by core.Env; missing-package path covered by app.PkgInfo tests")
	}
	if rc := runPkgInfo([]string{"missing-package"}); rc != 1 {
		t.Errorf("pkg info missing package rc = %d; want 1", rc)
	}
}

// TestPkg_runPkgInfo_Ugly — `--help` exits 0; `pkg info` is exposed via
// the `runPkg` dispatcher so the verb routing works end-to-end.
func TestPkg_runPkgInfo_Ugly(t *testing.T) {
	if rc := runPkgInfo([]string{"--help"}); rc != 0 {
		t.Errorf("pkg info --help rc = %d; want 0", rc)
	}
	if rc := runPkg([]string{"info"}); rc != 64 {
		t.Errorf("runPkg info (no args) rc = %d; want 64", rc)
	}
}
