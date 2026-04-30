// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// WrapWebOptions tunes the plain-web wrap. Web wrapping is the
// lowest-common-denominator — any directory of HTML/CSS/JS becomes a
// CoreApp that loads `index.html` inside CoreGUI.
//
//	opts := app.WrapWebOptions{Code: "lthn-landing", Name: "Lthn Landing"}
type WrapWebOptions struct {
	Code    string
	Name    string
	Version string
	// Entry overrides the default `index.html` entrypoint.
	Entry string
}

// WrapWeb produces a CoreApp ViewManifest for a directory of web
// assets. The directory MUST contain the entry file (index.html by
// default); absence is an error because a wrapped web app that can't
// load is worse than no app.
//
//	manifest, err := app.WrapWeb(coreio.Local, "./my-site", opts)
//
// Rules:
//
//   - Code defaults to a slug of the directory basename.
//   - Name defaults to a title-cased form of the directory basename.
//   - Entry defaults to "index.html"; any path may be passed (useful
//     when the site splits its entry per environment).
func WrapWeb(medium coreio.Medium, dir string, opts WrapWebOptions) (
	*config.ViewManifest, error,
) {
	if medium == nil {
		medium = coreio.Local
	}
	if dir == "" {
		return nil, core.E("app.WrapWeb", "empty directory", nil)
	}
	if !medium.IsDir(dir) {
		return nil, core.E("app.WrapWeb", "not a directory: "+dir, nil)
	}

	entry := opts.Entry
	if entry == "" {
		entry = "index.html"
	}

	entryPath := core.Path(dir, entry)
	if !medium.Exists(entryPath) {
		return nil, core.E("app.WrapWeb", "entry not found: "+entryPath, nil)
	}

	code := core.Trim(opts.Code)
	if code == "" {
		code = slugify(core.PathBase(dir))
	}
	if code == "" {
		code = "web-app"
	}

	name := opts.Name
	if name == "" {
		name = defaultWebWrapName(dir)
	}
	if name == "" {
		name = code
	}

	version := opts.Version
	if version == "" {
		version = "0.1.0"
	}

	m := &config.ViewManifest{
		Code:    code,
		Name:    name,
		Version: config.ViewVersion(version),
		Layout:  "C",
		Permissions: config.ViewPermissions{
			Read: []string{"./"}, // static assets served from app root
		},
		Config: map[string]any{
			"type":  PackageTypeWeb.String(),
			"entry": entry,
		},
	}
	return m, nil
}

// WriteWebWrap materialises a wrapped web manifest at
// `<dest>/.core/view.yaml`. Symmetric with WritePWAWrap /
// WriteElectronWrap for consistent CLI wiring.
//
//	err := app.WriteWebWrap(coreio.Local, dest, manifest)
func WriteWebWrap(
	medium coreio.Medium, dest string, manifest *config.ViewManifest,
) error {
	if manifest == nil {
		return core.E("app.WriteWebWrap", "nil manifest", nil)
	}
	return writeWrappedManifest(medium, dest, manifest)
}

func defaultWebWrapName(dir string) string {
	base := core.Trim(core.PathBase(dir))
	if base == "" {
		return ""
	}
	base = core.Replace(base, "-", " ")
	base = core.Replace(base, "_", " ")
	base = core.Replace(base, ".", " ")
	rawParts := core.Split(base, " ")
	parts := make([]string, 0, len(rawParts))
	for _, raw := range rawParts {
		part := core.Trim(raw)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = core.Upper(part[:1]) + part[1:]
	}
	return core.Join(" ", parts...)
}
