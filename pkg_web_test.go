// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"testing"

	"dappco.re/go/app"
	"dappco.re/go/config"
	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
)

// TestPkgWeb_WrapWeb_Good wraps a local web directory and asserts the
// resulting ViewManifest satisfies RFC §16.3's `core pkg wrap --web`
// contract.
func TestPkgWeb_WrapWeb_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local

	if err := medium.Write(core.Path(dir, "index.html"), `<html><body>hi</body></html>`); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}

	m, err := app.WrapWeb(medium, dir, app.WrapWebOptions{Code: "lthn-landing", Name: "Lthn Landing"})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}
	if m.Code != "lthn-landing" {
		t.Errorf("Code = %q", m.Code)
	}
	if m.Name != "Lthn Landing" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Config["type"] != "web" {
		t.Errorf("Config[type] = %v; want web", m.Config["type"])
	}
	if m.Config["entry"] != "index.html" {
		t.Errorf("Config[entry] = %v; want index.html", m.Config["entry"])
	}
}

// TestPkgWeb_WrapWeb_Bad rejects missing directory, missing entry.
func TestPkgWeb_WrapWeb_Bad(t *testing.T) {
	if _, err := app.WrapWeb(coreio.Local, "", app.WrapWebOptions{}); err == nil {
		t.Error("WrapWeb with empty dir returned no error")
	}
	if _, err := app.WrapWeb(coreio.Local, "/nonexistent/path/", app.WrapWebOptions{}); err == nil {
		t.Error("WrapWeb with missing dir returned no error")
	}
	// Dir exists but no index.html → error.
	dir := t.TempDir()
	if _, err := app.WrapWeb(coreio.Local, dir, app.WrapWebOptions{}); err == nil {
		t.Error("WrapWeb without entry returned no error")
	}
}

// TestPkgWeb_WrapWeb_Ugly tests the entry-override path with a non-
// standard filename (some static sites call it `main.html`).
func TestPkgWeb_WrapWeb_Ugly(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(dir, "main.html"), "<html/>"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	m, err := app.WrapWeb(medium, dir, app.WrapWebOptions{Entry: "main.html"})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}
	if m.Config["entry"] != "main.html" {
		t.Errorf("entry = %v; want main.html", m.Config["entry"])
	}
	// Auto-code derived from the basename of the directory.
	if m.Code == "" {
		t.Error("Code is empty; expected a slug from the directory basename")
	}
}

// TestPkgWeb_WriteWebWrap_Good round-trips a wrapped web manifest.
func TestPkgWeb_WriteWebWrap_Good(t *testing.T) {
	dir := t.TempDir()
	medium := coreio.Local
	if err := medium.Write(core.Path(dir, "index.html"), "<html/>"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	m, err := app.WrapWeb(medium, dir, app.WrapWebOptions{Code: "x", Name: "X"})
	if err != nil {
		t.Fatalf("WrapWeb: %v", err)
	}

	out := t.TempDir()
	if err := app.WriteWebWrap(medium, out, m); err != nil {
		t.Fatalf("WriteWebWrap: %v", err)
	}
	var round config.ViewManifest
	if err := app.LoadViewManifest(medium, core.Path(out, ".core", "view.yaml"), &round); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if round.Code != "x" {
		t.Errorf("round-trip Code = %q", round.Code)
	}
}

// TestPkgWeb_WriteWebWrap_Bad catches the nil-manifest guard.
func TestPkgWeb_WriteWebWrap_Bad(t *testing.T) {
	if err := app.WriteWebWrap(coreio.Local, t.TempDir(), nil); err == nil {
		t.Error("WriteWebWrap(nil) returned no error")
	}
}
