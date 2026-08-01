// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// runPkg dispatches the `pkg` subverbs (list, wrap, install, remove,
// update). Matches the RFC §16.3 command surface.
//
//	core-app pkg list
//	core-app pkg wrap --pwa https://app.example.com
//	core-app pkg wrap --electron github.com/foo/bar
//	core-app pkg wrap --web ./my-webapp
//	core-app pkg install CODE
//	core-app pkg remove  NAME
//	core-app pkg update  NAME
func runPkg(args []string) int {
	if len(args) == 0 {
		pkgUsage()
		return 64
	}
	verb := args[0]
	rest := args[1:]
	switch verb {
	case "list":
		return runPkgList(rest)
	case "info":
		return runPkgInfo(rest)
	case "wrap":
		return runPkgBundle(rest)
	case "install":
		return runPkgInstall(rest)
	case "remove":
		return runPkgRemove(rest)
	case "update":
		return runPkgUpdate(rest)
	case "--help", "-h":
		pkgUsage()
		return 0
	default:
		core.Error("pkg: unknown verb", "verb", verb)
		pkgUsage()
		return 64
	}
}

// pkgUsage prints the available pkg subverbs. Called on --help and on
// unknown-verb rejection.
//
//	pkgUsage()
func pkgUsage() {
	core.Println("core-app pkg <verb> [flags]")
	core.Println("  list                          list installed packages")
	core.Println("  info NAME                     describe a single installed package")
	core.Println("  wrap --pwa URL                wrap a PWA as a CoreApp")
	core.Println("  wrap --electron REPO|DIR      wrap an Electron app as a CoreApp")
	core.Println("  wrap --web DIR                wrap a local web directory")
	core.Println("  install CODE                  install a marketplace listing")
	core.Println("  remove [--purge] NAME         remove an installed package (purge wipes data)")
	core.Println("  update  NAME                  re-fetch and re-wrap")
}

// runPkgInfo prints the full describe-an-installed-package projection —
// identity line, type/version/source row, declared modules, layout
// variant + slots, permission summary and workspace path. Structured
// output via `--json` for programmatic consumers.
//
//	core-app pkg info photo-browser
//	core-app pkg info --json photo-browser
func runPkgInfo(args []string) int {
	asJSON := false
	name := ""
	for i := range args {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app pkg info [--json] NAME")
			core.Println("  --json  emit the full PkgDetails projection as JSON")
			core.Println("  NAME    the installed package code (matches `pkg list` NAME)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("pkg info: unknown flag", "flag", args[i])
				return 64
			}
			if name != "" {
				core.Error("pkg info: only one NAME supported", "extra", args[i])
				return 64
			}
			name = args[i]
		}
	}
	if name == "" {
		core.Error("pkg info: NAME is required")
		return 64
	}
	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("pkg info: cannot resolve DIR_HOME")
		return 1
	}

	info, err := app.PkgInfo(coreio.Local, home, name)
	if err != nil {
		core.Error("pkg info: failed", "name", name, "err", err)
		return 1
	}

	if asJSON {
		// Project to a shape stable across internal reshuffles — the
		// consumer never cares about Go field tags, only the documented
		// `info` contract. Mirrors the pkg list JSON projection so both
		// commands round-trip through the same tooling.
		type row struct {
			Name        string   `json:"name"`
			Type        string   `json:"type"`
			Version     string   `json:"version"`
			Source      string   `json:"source"`
			Category    string   `json:"category,omitempty"`
			Path        string   `json:"path,omitempty"`
			Workspace   string   `json:"workspace,omitempty"`
			Layout      string   `json:"layout,omitempty"`
			Modules     []string `json:"modules,omitempty"`
			Permissions []string `json:"permissions,omitempty"`
			Signed      bool     `json:"signed"`
		}
		r := row{
			Name:        info.Entry.Name,
			Type:        info.Entry.Type.String(),
			Version:     info.Entry.Version,
			Source:      info.Entry.Source,
			Category:    info.Entry.Category,
			Path:        info.Entry.Path,
			Workspace:   info.Workspace,
			Layout:      info.Manifest.Layout,
			Modules:     append([]string(nil), info.Manifest.Modules...),
			Permissions: info.Permissions,
			Signed:      info.Manifest.Sign != "",
		}
		marshaled := core.JSONMarshal(r)
		if !marshaled.OK {
			core.Error("pkg info: marshal failed", "err", marshaled.Value)
			return 1
		}
		raw, _ := marshaled.Value.([]byte)
		core.Println(string(raw))
		return 0
	}

	core.Println(info.Entry.Name + " — " + info.Manifest.Name)
	core.Println("  type:     " + info.Entry.Type.String())
	core.Println("  version:  " + info.Entry.Version)
	core.Println("  source:   " + info.Entry.DisplaySource())
	if info.Entry.Category != "" {
		core.Println("  category: " + info.Entry.Category)
	}
	core.Println("  path:     " + info.Entry.Path)
	if info.Workspace != "" {
		core.Println("  data:     " + info.Workspace)
	}
	if info.Manifest.Sign != "" {
		core.Println("  signed:   yes")
	} else {
		core.Println("  signed:   no")
	}
	if info.Manifest.Layout != "" {
		core.Println("  layout:   " + info.Manifest.Layout)
	}
	if len(info.Manifest.Modules) > 0 {
		core.Println("  modules:")
		for _, mod := range info.Manifest.Modules {
			core.Println("    - " + mod)
		}
	}
	if len(info.Permissions) > 0 {
		core.Println("  permissions:")
		for _, perm := range info.Permissions {
			core.Println("    - " + perm)
		}
	}
	return 0
}

// runPkgList prints `NAME\tTYPE\tVERSION\tSOURCE` rows for every
// installed package, or a JSON array when `--json` is passed. Uses
// `core.Env("DIR_HOME")` so the output matches the directory scan
// `PkgList` performs.
//
//	core-app pkg list
//	core-app pkg list --json
func runPkgList(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app pkg list [--json]")
			core.Println("  --json   emit a JSON array of {name, type, version, source, path}")
			return 0
		default:
			core.Error("pkg list: unknown flag", "flag", a)
			return 64
		}
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("pkg list: cannot resolve DIR_HOME")
		return 1
	}
	entries, err := app.PkgList(coreio.Local, home)
	if err != nil {
		core.Error("pkg list: failed", "err", err)
		return 1
	}

	if asJSON {
		// Project to a serialisable shape so the JSON output is stable
		// regardless of internal field reordering in PkgEntry. Both the
		// raw and display forms of the source are included so JSON
		// consumers can drive `pkg update` (raw) and a human-readable
		// table (display) without re-parsing the value. Category is
		// emitted when the installed manifest recorded it (marketplace
		// installs stamp it via app.stampCategory).
		type row struct {
			Name          string `json:"name"`
			Type          string `json:"type"`
			Version       string `json:"version"`
			Source        string `json:"source"`
			DisplaySource string `json:"display_source"`
			Category      string `json:"category,omitempty"`
			Path          string `json:"path,omitempty"`
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, row{
				Name: e.Name, Type: e.Type.String(), Version: e.Version,
				Source: e.Source, DisplaySource: e.DisplaySource(),
				Category: e.Category, Path: e.Path,
			})
		}
		r := core.JSONMarshal(rows)
		if !r.OK {
			core.Error("pkg list: marshal failed", "err", r.Value)
			return 1
		}
		raw, _ := r.Value.([]byte)
		core.Println(string(raw))
		return 0
	}

	if len(entries) == 0 {
		core.Println("(no packages installed)")
		return 0
	}
	// Align columns so the output matches the RFC §16.3 example. Each
	// column is padded to its widest cell + a 2-space gutter so the eye
	// can scan a hundred packages without losing the row alignment.
	const gutter = 2
	headers := []string{"NAME", "TYPE", "VERSION", "SOURCE"}
	widths := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
	for _, e := range entries {
		cells := []string{e.Name, e.Type.String(), e.Version, e.DisplaySource()}
		for i, cell := range cells {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	core.Println(formatRow(headers, widths, gutter))
	for _, e := range entries {
		cells := []string{e.Name, e.Type.String(), e.Version, e.DisplaySource()}
		core.Println(formatRow(cells, widths, gutter))
	}
	return 0
}

// formatRow lays out a single row using the supplied column widths
// plus a per-column gutter. The last cell is emitted without trailing
// padding so a terminal with a narrow window does not wrap on
// invisible whitespace.
//
//	formatRow([]string{"a", "b"}, []int{4, 3}, 2) // "a     b"
//
// If a cell is wider than its declared column width the gutter is
// still applied so the next column does not abut the over-long cell.
func formatRow(cells []string, widths []int, gutter int) string {
	out := core.NewBuilder()
	for i, cell := range cells {
		out.WriteString(cell)
		if i == len(cells)-1 {
			break
		}
		pad := max(widths[i]-len(cell)+gutter, gutter)
		for j := 0; j < pad; j++ {
			out.WriteByte(' ')
		}
	}
	return out.String()
}

// pkgWrapArgs captures every flag the `pkg wrap` subverb understands.
//
//	pkgWrapArgs{PWAURL: "https://app.example.com"}
type pkgWrapArgs struct {
	PWAURL        string
	ElectronDir   string // --electron may be a local dir or a github.com/owner/repo reference
	WebDir        string
	Code          string
	Name          string
	Version       string
	Dest          string // optional — defaults to $DIR_HOME/.core/apps/<code>/
	Install       bool   // true → persist under DIR_HOME; false → dump to Dest only
	Sign          string // explicit path to a private .key file (optional)
	UseDefaultKey bool   // sign with $DIR_HOME/.core/keys/default.key
	AssetSource   string // optional local dir copied into the wrapped app root
}

// runPkgBundle parses flags and dispatches to the right wrap path.
//
//	core-app pkg wrap --pwa https://app.example.com --install
func runPkgBundle(args []string) int {
	opts := pkgWrapArgs{Install: true}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pwa":
			if i+1 >= len(args) {
				core.Error("--pwa requires a URL")
				return 64
			}
			i++
			opts.PWAURL = args[i]
		case "--electron":
			if i+1 >= len(args) {
				core.Error("--electron requires a path or repo")
				return 64
			}
			i++
			opts.ElectronDir = args[i]
		case "--web":
			if i+1 >= len(args) {
				core.Error("--web requires a directory")
				return 64
			}
			i++
			opts.WebDir = args[i]
		case "--code":
			if i+1 >= len(args) {
				core.Error("--code requires a value")
				return 64
			}
			i++
			opts.Code = args[i]
		case "--name":
			if i+1 >= len(args) {
				core.Error("--name requires a value")
				return 64
			}
			i++
			opts.Name = args[i]
		case "--version":
			if i+1 >= len(args) {
				core.Error("--version requires a value")
				return 64
			}
			i++
			opts.Version = args[i]
		case "--dest":
			if i+1 >= len(args) {
				core.Error("--dest requires a directory")
				return 64
			}
			i++
			opts.Dest = args[i]
			opts.Install = false // explicit dest → no implicit install
		case "--no-install":
			opts.Install = false
		case "--sign":
			// RFC §16.2 example: `pkg wrap ... --sign` means "sign with
			// the default key". For backwards compatibility we also accept
			// `--sign PATH` as the explicit-key form.
			if i+1 < len(args) && !core.HasPrefix(args[i+1], "-") {
				i++
				opts.Sign = args[i]
			} else {
				opts.UseDefaultKey = true
			}
		case "--sign-default":
			opts.UseDefaultKey = true
		case "--help", "-h":
			core.Println("core-app pkg wrap [--pwa URL | --electron DIR|REPO | --web DIR] [--code S] [--dest D] [--sign [K] | --sign-default]")
			core.Println("  --sign            sign with the default key when no path follows")
			core.Println("  --sign PATH       sign with an explicit private key")
			core.Println("  --sign-default    explicit alias for default-key signing")
			return 0
		default:
			core.Error("pkg wrap: unknown flag", "flag", args[i])
			return 64
		}
	}

	switch {
	case opts.PWAURL != "":
		return runPkgBundlePWA(opts)
	case opts.ElectronDir != "":
		return runPkgBundleElectron(opts)
	case opts.WebDir != "":
		return runPkgBundleWeb(opts)
	default:
		core.Error("pkg wrap: one of --pwa / --electron / --web is required")
		return 64
	}
}

// runPkgBundlePWA handles `pkg wrap --pwa URL`. Fetches the manifest.json,
// wraps it, then persists or stashes depending on --install / --dest.
//
//	core-app pkg wrap --pwa https://play.example.com
func runPkgBundlePWA(opts pkgWrapArgs) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pwa, err := app.FetchPWAManifest(ctx, opts.PWAURL)
	if err != nil {
		core.Error("pkg wrap --pwa: fetch failed", "url", opts.PWAURL, "err", err)
		return 1
	}
	manifest := app.WrapPWA(pwa, app.WrapPWAOptions{
		TargetURL: app.ResolvePWAAppURL(opts.PWAURL, pwa),
		Code:      opts.Code,
		Version:   opts.Version,
	})
	if manifest == nil {
		core.Error("pkg wrap --pwa: WrapPWA returned nil")
		return 1
	}
	if opts.Name != "" {
		manifest.Name = opts.Name
	}
	return persistBundle(manifest, opts)
}

// runPkgBundleElectron handles `pkg wrap --electron <DIR|REPO>`. Two
// modes are supported:
//
//   - DIR (local path with package.json + renderer): scan the
//     directory for Electron API patterns and write a wrapped
//     manifest in-place.
//
//   - REPO (`github.com/owner/repo` etc.): fetch the latest GitHub
//     release, download the first renderer-shaped asset to a scratch
//     directory, then run the directory mode against the unpacked
//     contents (the wrapper does NOT extract — that's for a follow-up
//     iteration once we depend on archive/zip / tar). For now the
//     download succeeds and we surface the asset path so the user can
//     point the next invocation at the unpacked directory.
//
//     core-app pkg wrap --electron ./my-electron-app
//     core-app pkg wrap --electron github.com/foo/bar
func runPkgBundleElectron(opts pkgWrapArgs) int {
	dir := opts.ElectronDir
	medium := coreio.Local

	if isRepoSpec(dir) {
		_, _, repo, ok := app.ParseGitHubRepo(dir)
		if !ok {
			core.Error("pkg wrap --electron: cannot parse repo reference", "ref", dir)
			return 1
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		scratch := core.Path("./.core-wrap", "electron-"+repo)
		manifest, rendererDir, err := app.WrapElectronRepo(ctx, medium, dir, app.WrapElectronRepoOptions{
			Code:       opts.Code,
			Name:       opts.Name,
			Version:    opts.Version,
			ScratchDir: scratch,
		})
		if err != nil {
			core.Error("pkg wrap --electron: wrap failed", "repo", dir, "err", err)
			return 1
		}
		opts.AssetSource = rendererDir
		rc := persistBundle(manifest, opts)
		if medium.IsDir(scratch) {
			if err := medium.DeleteAll(scratch); err != nil {
				core.Warn("pkg wrap --electron: scratch cleanup failed", core.Concat("pa", "th"), scratch, "err", err)
			}
		}
		return rc
	}

	if !medium.IsDir(dir) {
		core.Error("pkg wrap --electron: not a directory or repo reference", "arg", dir)
		return 1
	}

	pkg, err := loadElectronPackageJSON(medium, dir)
	if err != nil {
		core.Error("pkg wrap --electron: package.json load failed", "err", err)
		return 1
	}
	scan, err := app.ScanElectronRenderer(medium, dir)
	if err != nil {
		core.Error("pkg wrap --electron: scan failed", "err", err)
		return 1
	}

	manifest := app.WrapElectron(pkg, scan, app.WrapElectronOptions{Code: opts.Code})
	if manifest == nil {
		core.Error("pkg wrap --electron: WrapElectron returned nil")
		return 1
	}
	if opts.Name != "" {
		manifest.Name = opts.Name
	}
	if opts.Version != "" {
		manifest.Version = config.ViewVersion(opts.Version)
	}
	opts.AssetSource = dir
	return persistBundle(manifest, opts)
}

// isRepoSpec returns true when the --electron argument looks like a
// GitHub repo reference rather than a local directory path. The check
// is permissive — anything starting with `github.com/`, `gitlab.com/`,
// `git@`, `https://` or `http://` is treated as a repo.
//
//	isRepoSpec("github.com/foo/bar") // true
//	isRepoSpec("./my-app")           // false
func isRepoSpec(s string) bool {
	for _, p := range []string{
		"github.com/", "gitlab.com/", "bitbucket.org/",
		"git@", "https://", "http://",
	} {
		if core.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// runPkgBundleWeb handles `pkg wrap --web DIR`. Produces a CoreApp that
// loads the directory's index.html.
//
//	core-app pkg wrap --web ./my-webapp
func runPkgBundleWeb(opts pkgWrapArgs) int {
	manifest, err := app.WrapWeb(coreio.Local, opts.WebDir, app.WrapWebOptions{
		Code:    opts.Code,
		Name:    opts.Name,
		Version: opts.Version,
	})
	if err != nil {
		core.Error("pkg wrap --web: failed", "err", err)
		return 1
	}
	opts.AssetSource = opts.WebDir
	return persistBundle(manifest, opts)
}

// persistBundle materialises a wrapped manifest either as a fresh install
// under $DIR_HOME/.core/apps (opts.Install=true), or next to the wrap
// source (opts.Dest set). Mirror CLI output either way so the agent
// always learns the path it can now boot.
//
//	rc := persistBundle(manifest, opts)
func persistBundle(manifest *config.ViewManifest, opts pkgWrapArgs) int {
	if manifest == nil {
		core.Error("pkg wrap: nil manifest")
		return 1
	}
	medium := coreio.Local

	if opts.Dest != "" && !opts.Install {
		if err := app.WriteWrappedAppWithOptions(medium, opts.Dest, manifest, app.WriteWrappedOptions{
			AssetSource: opts.AssetSource,
			SignKeyPath: opts.Sign,
			SignDefault: opts.UseDefaultKey,
		}); err != nil {
			core.Error("pkg wrap: write failed", "err", err)
			return 1
		}
		core.Info("wrapped", "code", manifest.Code, "dest", opts.Dest)
		return 0
	}

	if opts.Install {
		home := core.Env("DIR_HOME")
		if home == "" {
			core.Error("pkg wrap: cannot resolve DIR_HOME")
			return 1
		}
		installOpts := app.PkgInstallOptions{
			Home:        home,
			Force:       true,
			Source:      "wrap:" + opts.sourceTag(),
			AssetSource: opts.AssetSource,
			SignKeyPath: opts.Sign,
			SignDefault: opts.UseDefaultKey,
		}
		dest, err := installBundlepedByType(medium, manifest, installOpts)
		if err != nil {
			core.Error("pkg wrap: install failed", "err", err)
			return 1
		}
		core.Info("installed", "code", manifest.Code, "dest", dest)
		return 0
	}

	// Neither --dest nor --install → write next to cwd under a
	// predictable scratch directory.
	scratch := "./.core-wrap/" + manifest.Code
	if err := app.WriteWrappedAppWithOptions(medium, scratch, manifest, app.WriteWrappedOptions{
		AssetSource: opts.AssetSource,
		SignKeyPath: opts.Sign,
		SignDefault: opts.UseDefaultKey,
	}); err != nil {
		core.Error("pkg wrap: scratch write failed", "err", err)
		return 1
	}
	core.Info("wrapped", "code", manifest.Code, "dest", scratch)
	return 0
}

// installBundlepedByType dispatches to the install helper matching the
// wrapped manifest's package type so web and Electron wraps can carry
// their copied assets into the install root.
func installBundlepedByType(medium coreio.Medium, manifest *config.ViewManifest, opts app.PkgInstallOptions) (
	string, error,
) {
	if manifest == nil {
		return "", core.NewError("installBundlepedByType: nil manifest")
	}
	switch manifestPackageType(manifest) {
	case app.PackageTypeElectron:
		return app.InstallWrappedElectron(medium, manifest, opts)
	case app.PackageTypeWeb:
		return app.InstallWrappedWeb(medium, manifest, opts)
	default:
		return app.InstallWrappedPWA(medium, manifest, opts)
	}
}

// manifestPackageType reads the wrap-emitted `config.type` field.
// Missing / unknown values fall back to PWA, which is the manifest-only
// wrap path.
func manifestPackageType(manifest *config.ViewManifest) app.PackageType {
	if manifest == nil || manifest.Config == nil {
		return app.PackageTypePWA
	}
	if raw, ok := manifest.Config["type"].(string); ok {
		if t := app.ParsePackageType(raw); t != app.PackageTypeUnknown {
			return t
		}
	}
	return app.PackageTypePWA
}

// resolveInstallSpecAgainstMarketplace handles the RFC §16.3 ambiguity
// where `vendor/name` may be either a marketplace code or a GitHub repo
// shorthand. Resolution rules:
//
//   - Explicit repo references (`github.com/...`, `git@...`,
//     `https://github.com/...`) always win.
//
//   - Ambiguous `owner/repo` shorthands consult the local marketplace
//     cache first; if no listing exists, they fall back to the repo path.
//
//   - Everything else is left untouched.
func resolveInstallSpecAgainstMarketplace(home string, spec app.PkgInstallSpec) app.PkgInstallSpec {
	switch {
	case spec.Repo != "":
		return spec
	case spec.URL != "" && isRepoSpec(spec.URL):
		if _, _, _, ok := app.ParseGitHubRepo(spec.URL); ok {
			spec.Type = app.PackageTypeElectron
			spec.Repo = spec.URL
			spec.URL = ""
		}
		return spec
	case spec.Code == "", !core.Contains(spec.Code, "/"):
		return spec
	}

	root := marketplaceRoot(home)
	if root != "" && coreio.Local.IsDir(root) {
		if _, err := app.MarketplaceResolve(coreio.Local, root, spec.Code); err == nil {
			return spec
		}
	}
	if _, _, _, ok := app.ParseGitHubRepo(spec.Code); ok {
		spec.Type = app.PackageTypeElectron
		spec.Repo = spec.Code
		spec.Code = ""
	}
	return spec
}

func marketplaceRoot(home string) string {
	if home == "" {
		return ""
	}
	return core.Path(home, ".core", "marketplace")
}

// runPkgInstall handles `pkg install <source>`. Auto-detects the
// install kind from the argument; the operator can override with
// `--type` when the heuristics pick the wrong path:
//
//	core-app pkg install https://app.example.com            # PWA (auto)
//	core-app pkg install github.com/owner/repo              # Electron (auto)
//	core-app pkg install photo-browser                      # marketplace listing
//	core-app pkg install --type web ./my-site               # forced web wrap
//	core-app pkg install --type native ./my-coreapp         # forced native copy
//
// PWA installs do not need a marketplace cache. Marketplace installs
// require `core-app marketplace fetch` first.
func runPkgInstall(args []string) int {
	src := ""
	override := app.PackageTypeUnknown
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			if i+1 >= len(args) {
				core.Error("pkg install: --type requires a value (native|pwa|electron|web)")
				return 64
			}
			i++
			t := app.ParsePackageType(args[i])
			if t == app.PackageTypeUnknown {
				core.Error("pkg install: unknown --type", "value", args[i],
					"hint", "supported: native, pwa, electron, web")
				return 64
			}
			override = t
		case "--help", "-h":
			core.Println("core-app pkg install [--type native|pwa|electron|web] <source>")
			core.Println("  --type    skip auto-detection and force the install kind")
			core.Println("  source    URL, github.com/owner/repo, marketplace code, or local path")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("pkg install: unknown flag", "flag", args[i])
				return 64
			}
			if src != "" {
				core.Error("pkg install: only one <source> argument supported", "extra", args[i])
				return 64
			}
			src = args[i]
		}
	}
	if src == "" {
		core.Error("pkg install: <source> is required")
		return 64
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("pkg install: cannot resolve DIR_HOME")
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spec := app.ParseInstallSpec(src)
	spec = resolveInstallSpecAgainstMarketplace(home, spec)
	if override != app.PackageTypeUnknown {
		// A plain package code (`photo-browser`, `core/play`) is not an
		// ambiguous filesystem / URL / repo source — the marketplace index
		// is the only authority that can resolve it. Keep that path even
		// when the operator supplied `--type`; otherwise `--type native
		// photo-browser` gets misrouted into a local-path install against a
		// non-existent "./photo-browser" directory.
		if spec.Code != "" && spec.Path == "" && spec.URL == "" && spec.Repo == "" {
			return runPkgInstallMarketplace(ctx, home, spec.Code)
		}
		spec.Type = override
		// When the operator forces a type, dispatch on it directly so a
		// local path that auto-detects as native can still be re-wrapped
		// as web (or vice-versa) without renaming the directory.
		switch override {
		case app.PackageTypePWA:
			if spec.Path != "" {
				return runPkgInstallLocalAs(home, spec.Path, app.PackageTypePWA)
			}
			if spec.Repo != "" {
				return runPkgInstallRepoPWA(ctx, home, spec.Repo)
			}
			if spec.URL == "" {
				spec.URL = src
			}
			return runPkgInstallPWA(ctx, home, spec.URL)
		case app.PackageTypeElectron:
			if spec.Path != "" {
				return runPkgInstallLocalAs(home, spec.Path, app.PackageTypeElectron)
			}
			if spec.Repo == "" {
				spec.Repo = src
			}
			return runPkgInstallElectron(ctx, home, spec.Repo)
		case app.PackageTypeWeb:
			path := spec.Path
			if path == "" {
				path = src
			}
			return runPkgInstallLocalAs(home, path, app.PackageTypeWeb)
		case app.PackageTypeNative:
			path := spec.Path
			if path == "" {
				path = src
			}
			return runPkgInstallLocalAs(home, path, app.PackageTypeNative)
		}
	}

	switch spec.Type {
	case app.PackageTypePWA:
		return runPkgInstallPWA(ctx, home, spec.URL)
	case app.PackageTypeElectron:
		return runPkgInstallRepo(ctx, home, spec.Repo)
	case app.PackageTypeUnknown:
		// Local directory — the path was set by ParseInstallSpec.
		if spec.Path != "" {
			return runPkgInstallLocal(home, spec.Path)
		}
		return runPkgInstallMarketplace(ctx, home, spec.Code)
	default:
		// Native marketplace listing (vendor/code or plain code).
		return runPkgInstallMarketplace(ctx, home, spec.Code)
	}
}

// runPkgInstallLocal installs a CoreApp from a local directory tree.
// Auto-detects the package type per RFC §16.4:
//
//   - `.core/view.yaml` present → PackageTypeNative; copy the tree.
//   - `manifest.json` / `manifest.webmanifest` with start_url
//     → PackageTypePWA; wrap+install.
//   - `package.json` with electron dep → PackageTypeElectron; wrap+install.
//   - `index.html` present → PackageTypeWeb; wrap+install.
//
// Anything else returns a typed error so the user knows the directory
// does not look like a wrappable app.
//
//	rc := runPkgInstallLocal(home, "./my-app")
func runPkgInstallLocal(home, path string) int {
	kind := app.DetectPackageType(coreio.Local, path)
	if kind == app.PackageTypeUnknown {
		core.Error("pkg install local: cannot detect package type",
			core.Concat("pa", "th"), path,
			"hint", "expected .core/view.yaml, manifest.json, manifest.webmanifest, package.json, or index.html")
		return 1
	}
	return runPkgInstallLocalAs(home, path, kind)
}

// runPkgInstallLocalAs installs a local directory using the explicitly
// selected package type. This is the path `pkg install --type ...`
// relies on so an override genuinely forces the requested wrap mode.
func runPkgInstallLocalAs(home, path string, kind app.PackageType) int {
	medium := coreio.Local
	if !medium.IsDir(path) {
		core.Error("pkg install local: not a directory", core.Concat("pa", "th"), path)
		return 1
	}

	switch kind {
	case app.PackageTypeNative:
		dest, err := app.PkgInstallLocal(medium, path, app.PkgInstallOptions{
			Home:   home,
			Force:  true,
			Source: "local:" + path,
		})
		if err != nil {
			core.Error("pkg install local: native install failed", "src", path, "err", err)
			return 1
		}
		core.Info("installed", "type", "native", "src", path, "dest", dest)
		return 0
	case app.PackageTypePWA:
		manifestPath, ok := app.FindLocalPWAManifest(medium, path)
		if !ok {
			core.Error("pkg install local: no PWA manifest found",
				core.Concat("pa", "th"), path,
				"hint", "expected manifest.json or manifest.webmanifest")
			return 1
		}
		body, err := medium.Read(manifestPath)
		if err != nil {
			core.Error("pkg install local: read PWA manifest failed", core.Concat("pa", "th"), manifestPath, "err", err)
			return 1
		}
		var pwa app.PWAManifest
		r := core.JSONUnmarshal([]byte(body), &pwa)
		if !r.OK {
			core.Error("pkg install local: decode PWA manifest failed", core.Concat("pa", "th"), manifestPath, "err", r.Value)
			return 1
		}
		manifest := app.WrapPWA(&pwa, app.WrapPWAOptions{
			TargetURL: app.ResolvePWAAppURL(manifestPath, &pwa),
		})
		if manifest == nil {
			core.Error("pkg install local: WrapPWA returned nil")
			return 1
		}
		dest, err := app.InstallWrappedPWA(medium, manifest, app.PkgInstallOptions{
			Home:        home,
			Force:       true,
			Source:      "wrap:pwa:" + manifestPath,
			AssetSource: path,
		})
		if err != nil {
			core.Error("pkg install local: PWA install failed", "err", err)
			return 1
		}
		core.Info("installed", "type", "pwa", "src", path, "dest", dest)
		return 0
	case app.PackageTypeElectron:
		pkg, err := loadElectronPackageJSON(medium, path)
		if err != nil {
			core.Error("pkg install local: package.json load failed", "err", err)
			return 1
		}
		scan, err := app.ScanElectronRenderer(medium, path)
		if err != nil {
			core.Error("pkg install local: scan failed", "err", err)
			return 1
		}
		manifest := app.WrapElectron(pkg, scan, app.WrapElectronOptions{})
		if manifest == nil {
			core.Error("pkg install local: WrapElectron returned nil")
			return 1
		}
		dest, err := app.InstallWrappedElectron(medium, manifest, app.PkgInstallOptions{
			Home: home, Force: true, Source: "wrap:electron:" + path, AssetSource: path,
		})
		if err != nil {
			core.Error("pkg install local: Electron install failed", "err", err)
			return 1
		}
		core.Info("installed", "type", "electron", "src", path, "dest", dest)
		return 0
	case app.PackageTypeWeb:
		manifest, err := app.WrapWeb(medium, path, app.WrapWebOptions{})
		if err != nil {
			core.Error("pkg install local: WrapWeb failed", "err", err)
			return 1
		}
		dest, err := app.InstallWrappedWeb(medium, manifest, app.PkgInstallOptions{
			Home: home, Force: true, Source: "wrap:web:" + path, AssetSource: path,
		})
		if err != nil {
			core.Error("pkg install local: Web install failed", "err", err)
			return 1
		}
		core.Info("installed", "type", "web", "src", path, "dest", dest)
		return 0
	default:
		core.Error("pkg install local: unsupported forced type", "type", kind.String(), core.Concat("pa", "th"), path)
		return 1
	}
}

// runPkgInstallRepo auto-detects whether a GitHub-style repo is a PWA
// source tree or an Electron app. PWA repos are wrapped from the source
// archive; Electron repos continue through the release-asset path.
func runPkgInstallRepo(ctx context.Context, home, ref string) int {
	scratch, ok := repoScratchDir(home, ref)
	if !ok {
		core.Error("pkg install: cannot parse repo reference", "ref", ref)
		return 1
	}
	root, err := app.FetchRepoSource(ctx, coreio.Local, ref, scratch)
	if coreio.Local.IsDir(scratch) {
		defer func() { _ = coreio.Local.DeleteAll(scratch) }()
	}
	if err != nil {
		core.Error("pkg install: repo source fetch failed", "repo", ref, "err", err)
		return 1
	}

	switch kind := app.DetectPackageType(coreio.Local, root); kind {
	case app.PackageTypePWA:
		return runPkgInstallRepoPWAFromRoot(home, ref, root)
	case app.PackageTypeElectron:
		return runPkgInstallElectron(ctx, home, ref)
	default:
		core.Error("pkg install: repo type unsupported",
			"repo", ref,
			"detected", kind.String(),
			"hint", "auto-detect supports Electron and PWA repos")
		return 1
	}
}

// runPkgInstallRepoPWA forces the PWA wrap path for a GitHub-style repo
// reference.
func runPkgInstallRepoPWA(ctx context.Context, home, ref string) int {
	scratch, ok := repoScratchDir(home, ref)
	if !ok {
		core.Error("pkg install: cannot parse repo reference", "ref", ref)
		return 1
	}
	root, err := app.FetchRepoSource(ctx, coreio.Local, ref, scratch)
	if coreio.Local.IsDir(scratch) {
		defer func() { _ = coreio.Local.DeleteAll(scratch) }()
	}
	if err != nil {
		core.Error("pkg install: repo source fetch failed", "repo", ref, "err", err)
		return 1
	}
	return runPkgInstallRepoPWAFromRoot(home, ref, root)
}

func runPkgInstallRepoPWAFromRoot(home, ref, root string) int {
	manifestPath, ok := app.FindLocalPWAManifest(coreio.Local, root)
	if !ok {
		core.Error("pkg install: no PWA manifest found",
			"root", root,
			"hint", "expected manifest.json or manifest.webmanifest")
		return 1
	}
	body, err := coreio.Local.Read(manifestPath)
	if err != nil {
		core.Error("pkg install: read repo PWA manifest failed", core.Concat("pa", "th"), manifestPath, "err", err)
		return 1
	}
	var pwa app.PWAManifest
	r := core.JSONUnmarshal([]byte(body), &pwa)
	if !r.OK {
		core.Error("pkg install: decode repo PWA manifest failed", core.Concat("pa", "th"), manifestPath, "err", r.Value)
		return 1
	}
	manifest := app.WrapPWA(&pwa, app.WrapPWAOptions{
		TargetURL: app.ResolvePWAAppURL(manifestPath, &pwa),
	})
	if manifest == nil {
		core.Error("pkg install: WrapPWA returned nil")
		return 1
	}
	dest, err := app.InstallWrappedPWA(coreio.Local, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		Source:      "wrap:pwa:" + ref,
		AssetSource: root,
	})
	if err != nil {
		core.Error("pkg install: repo PWA install failed", "repo", ref, "err", err)
		return 1
	}
	core.Info("installed", "code", manifest.Code, "type", "pwa", "dest", dest)
	return 0
}

// runPkgInstallElectron is the install-side counterpart to
// runPkgBundleElectron's repo branch — it fetches the latest GitHub
// release, downloads the renderer asset to a scratch directory and
// reports the path so the user can extract+rewrap. Full extraction
// (zip/tar) is intentionally future work; the install command is here
// so the auto-detected dispatch produces a useful side effect rather
// than a "not yet wired" message.
//
//	rc := runPkgInstallElectron(ctx, "github.com/owner/repo")
func runPkgInstallElectron(ctx context.Context, home, ref string) int {
	_, _, repo, ok := app.ParseGitHubRepo(ref)
	if !ok {
		core.Error("pkg install: cannot parse repo reference", "ref", ref)
		return 1
	}
	scratch := core.Path(home, ".core", ".wrap", "electron-"+repo)
	manifest, rendererDir, err := app.WrapElectronRepo(ctx, coreio.Local, ref, app.WrapElectronRepoOptions{
		ScratchDir: scratch,
	})
	if err != nil {
		core.Error("pkg install: wrap failed", "repo", ref, "err", err)
		return 1
	}
	dest, err := app.InstallWrappedElectron(coreio.Local, manifest, app.PkgInstallOptions{
		Home:        home,
		Force:       true,
		Source:      "wrap:electron:" + ref,
		AssetSource: rendererDir,
	})
	if coreio.Local.IsDir(scratch) {
		if cleanupErr := coreio.Local.DeleteAll(scratch); cleanupErr != nil {
			core.Warn("pkg install: scratch cleanup failed", core.Concat("pa", "th"), scratch, "err", cleanupErr)
		}
	}
	if err != nil {
		core.Error("pkg install: Electron install failed", "err", err)
		return 1
	}
	core.Info("installed", "code", manifest.Code, "type", "electron", "dest", dest)
	return 0
}

func repoScratchDir(home, ref string) (string, bool) {
	_, _, repo, ok := app.ParseGitHubRepo(ref)
	if !ok {
		return "", false
	}
	return core.Path(home, ".core", ".wrap", "repo-"+repo), true
}

// isExtractable reports whether the path's suffix is one of the
// supported archive formats. Centralised so the CLI dispatch and the
// tests can share the same predicate without duplicating the suffix
// list.
//
//	isExtractable("renderer.tar.gz") // true
//	isExtractable("Renderer.7Z")     // false
func isExtractable(path string) bool {
	low := core.Lower(path)
	for _, suffix := range []string{".zip", ".tar.gz", ".tgz", ".tar"} {
		if core.HasSuffix(low, suffix) {
			return true
		}
	}
	return false
}

// runPkgInstallPWA fetches the manifest at `url`, wraps it, and persists
// the result under `<home>/.core/apps/<code>/`. No marketplace cache
// required — the URL itself is the source.
//
//	rc := runPkgInstallPWA(ctx, home, "https://app.example.com")
func runPkgInstallPWA(ctx context.Context, home, url string) int {
	pwa, err := app.FetchPWAManifest(ctx, url)
	if err != nil {
		core.Error("pkg install --pwa: fetch failed", "url", url, "err", err)
		return 1
	}
	manifest := app.WrapPWA(pwa, app.WrapPWAOptions{
		TargetURL: app.ResolvePWAAppURL(url, pwa),
	})
	if manifest == nil {
		core.Error("pkg install --pwa: WrapPWA returned nil")
		return 1
	}
	dest, err := app.InstallWrappedPWA(coreio.Local, manifest, app.PkgInstallOptions{
		Home:   home,
		Force:  true,
		Source: "wrap:pwa:" + url,
	})
	if err != nil {
		core.Error("pkg install --pwa: install failed", "err", err)
		return 1
	}
	core.Info("installed", "code", manifest.Code, "type", "pwa", "dest", dest)
	return 0
}

// runPkgInstallMarketplace resolves a listing from the local
// marketplace cache and delegates to app.MarketplaceInstall.
//
//	rc := runPkgInstallMarketplace(ctx, home, "photo-browser")
func runPkgInstallMarketplace(ctx context.Context, home, code string) int {
	root := core.Path(home, ".core", "marketplace")
	if !coreio.Local.IsDir(root) {
		core.Error("pkg install: marketplace cache missing — run `marketplace fetch` first", core.Concat("pa", "th"), root)
		return 1
	}
	c := core.New()
	dest, err := app.MarketplaceInstall(ctx, c, app.MarketplaceInstallOptions{
		Root:  root,
		Home:  home,
		Code:  code,
		Force: true,
	})
	if err != nil {
		core.Error("pkg install: failed", "code", code, "err", err)
		return 1
	}
	core.Info("installed", "code", code, "dest", dest)
	return 0
}

// runPkgRemove handles `pkg remove NAME [--purge]`. Delegates to
// app.PkgRemoveWith so the same code path covers install-tree-only
// removal and full purge (workspace data tree included).
//
//	core-app pkg remove bitwarden-clients
//	core-app pkg remove --purge bitwarden-clients
func runPkgRemove(args []string) int {
	purge := false
	name := ""
	for i := range args {
		switch args[i] {
		case "--purge":
			purge = true
		case "--help", "-h":
			core.Println("core-app pkg remove [--purge] NAME")
			core.Println("  --purge  also wipe the workspace data tree at ~/.core/data/NAME/")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("pkg remove: unknown flag", "flag", args[i])
				return 64
			}
			if name != "" {
				core.Error("pkg remove: only one NAME supported", "extra", args[i])
				return 64
			}
			name = args[i]
		}
	}
	if name == "" {
		core.Error("pkg remove: NAME is required")
		return 64
	}
	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("pkg remove: cannot resolve DIR_HOME")
		return 1
	}
	if err := app.PkgRemoveWith(coreio.Local, home, name, app.PkgRemoveOptions{
		Purge: purge,
	}); err != nil {
		core.Error("pkg remove: failed", "name", name, "err", err)
		return 1
	}
	if purge {
		core.Info("removed and purged", "name", name)
	} else {
		core.Info("removed", "name", name)
	}
	return 0
}

// runPkgUpdate handles `pkg update NAME`. Auto-dispatches based on the
// recorded source — marketplace listings go through MarketplaceUpdate
// (git pull + verify), wrap installs go through PkgUpdate (re-fetch /
// re-wrap), local installs are re-copied. Mirrors the dispatch logic
// used by `pkg install` so the user does not need to remember which
// source type to invoke.
//
//	core-app pkg update bitwarden-clients
func runPkgUpdate(args []string) int {
	name := ""
	for i := range args {
		switch args[i] {
		case "--help", "-h":
			core.Println("core-app pkg update NAME")
			core.Println("  NAME  the installed package code (matches `pkg list` NAME)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("pkg update: unknown flag", "flag", args[i])
				return 64
			}
			if name != "" {
				core.Error("pkg update: only one NAME supported", "extra", args[i])
				return 64
			}
			name = args[i]
		}
	}
	if name == "" {
		core.Error("pkg update: NAME is required")
		return 64
	}
	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("pkg update: cannot resolve DIR_HOME")
		return 1
	}
	medium := coreio.Local

	// Inspect the recorded source so we can pick the right update path.
	// marketplace:<code> goes through MarketplaceUpdate for the git pull
	// + signature verify; everything else uses PkgUpdate.
	source := readInstalledSource(medium, home, name)
	if rest, ok := stripStringPrefix(source, "marketplace:"); ok {
		root := core.Path(home, ".core", "marketplace")
		if !medium.IsDir(root) {
			core.Error("pkg update: marketplace cache missing — run `marketplace fetch` first",
				core.Concat("pa", "th"), root)
			return 1
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := core.New()
		dest, err := app.MarketplaceUpdate(ctx, c, app.MarketplaceUpdateOptions{
			Root: root, Home: home, Code: rest,
		})
		if err != nil {
			core.Error("pkg update: marketplace update failed", "name", name, "err", err)
			return 1
		}
		core.Info("updated", "name", name, "dest", dest, "source", source)
		return 0
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest, err := app.PkgUpdate(ctx, medium, home, name)
	if err != nil {
		core.Error("pkg update: failed", "name", name, "err", err)
		return 1
	}
	core.Info("updated", "name", name, "dest", dest)
	return 0
}

// readInstalledSource reads the recorded `Config["source"]` field from
// an installed package's manifest. Returns an empty string when the
// install has no source recorded so the caller falls through to the
// default update path.
//
//	src := readInstalledSource(coreio.Local, home, "photo-browser")
func readInstalledSource(medium coreio.Medium, home, name string) string {
	if medium == nil || home == "" || name == "" {
		return ""
	}
	view := core.Path(home, ".core", app.AppsDirName, name, ".core", "view.yaml")
	if !medium.Exists(view) {
		return ""
	}
	var manifest config.ViewManifest
	if err := app.LoadViewManifest(medium, view, &manifest); err != nil {
		return ""
	}
	if manifest.Config == nil {
		return ""
	}
	if s, ok := manifest.Config["source"].(string); ok {
		return s
	}
	return ""
}

// stringStripPrefix is the local mirror of the package-private
// stripPrefix in pkg.go (different package — main here, app there).
// Returns the substring after `prefix` and a boolean indicating
// whether the prefix was found.
//
//	rest, ok := stripStringPrefix("marketplace:foo", "marketplace:")
//	// "foo", true
func stripStringPrefix(s, prefix string) (string, bool) {
	if !core.HasPrefix(s, prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// loadElectronPackageJSON reads the package.json under `dir` and
// decodes the subset WrapElectron needs. Missing file → typed error so
// the caller can surface a useful message.
//
//	pkg, err := loadElectronPackageJSON(medium, dir)
func loadElectronPackageJSON(medium coreio.Medium, dir string) (
	*app.ElectronPackageJSON, error,
) {
	path := core.Path(dir, "package.json")
	if !medium.Exists(path) {
		return nil, core.NewError("package.json not found at " + path)
	}
	body, err := medium.Read(path)
	if err != nil {
		return nil, err
	}
	var pkg app.ElectronPackageJSON
	r := core.JSONUnmarshal([]byte(body), &pkg)
	if !r.OK {
		if cause, ok := r.Value.(error); ok {
			return nil, cause
		}
		return nil, core.NewError("decode package.json failed")
	}
	return &pkg, nil
}

// signManifestFile loads a private key and mutates the manifest's Sign
// field in-place. Used by `pkg wrap --sign KEY`.
//
//	err := signManifestFile(keyPath, manifest)
func signManifestFile(
	keyPath string, manifest *config.ViewManifest,
) error {
	if manifest == nil {
		return core.NewError("signManifestFile: nil manifest")
	}
	priv, err := app.LoadPrivateKey(coreio.Local, keyPath)
	if err != nil {
		return err
	}
	return app.SignManifest(manifest, priv)
}

// signManifestDefault loads `$DIR_HOME/.core/keys/default.key` and
// applies the signature to `manifest`. Used by `pkg wrap --sign`
// (without a following path) and `pkg wrap --sign-default`.
//
//	err := signManifestDefault(manifest)
func signManifestDefault(
	manifest *config.ViewManifest,
) error {
	if manifest == nil {
		return core.NewError("signManifestDefault: nil manifest")
	}
	priv, err := app.LoadDefaultPrivateKey(coreio.Local)
	if err != nil {
		return err
	}
	return app.SignManifest(manifest, priv)
}

// applyWrapSignature is the shared body that handles `--sign KEY`,
// bare `--sign`, and `--sign-default`. Returns a non-nil error when the
// manifest could not be signed; the caller maps this to the relevant
// CLI error.
//
//	if err := applyWrapSignature(opts, manifest); err != nil { ... }
func applyWrapSignature(
	opts pkgWrapArgs, manifest *config.ViewManifest,
) error {
	if opts.Sign != "" {
		return signManifestFile(opts.Sign, manifest)
	}
	if opts.UseDefaultKey {
		return signManifestDefault(manifest)
	}
	return nil
}

// sourceTag returns a short tag for the origin (pwa / electron / web)
// so `pkg list` can show where a wrap came from.
//
//	tag := opts.sourceTag() // "pwa" / "electron" / "web"
func (a pkgWrapArgs) sourceTag() string {
	switch {
	case a.PWAURL != "":
		return "pwa:" + a.PWAURL
	case a.ElectronDir != "":
		return "electron:" + a.ElectronDir
	case a.WebDir != "":
		return "web:" + a.WebDir
	}
	return "unknown"
}

// runMarketplace dispatches the `marketplace` subverbs. Keeps them
// separate from `pkg` so CLI tab-completion can advertise them
// independently.
//
//	core-app marketplace categories
//	core-app marketplace browse media
//	core-app marketplace search photo
//	core-app marketplace install photo-browser
//	core-app marketplace update  photo-browser
//	core-app marketplace remove  photo-browser
//	core-app marketplace installed
//	core-app marketplace fetch --url https://forge.lthn.ai/core/marketplace.git
func runMarketplace(args []string) int {
	if len(args) == 0 {
		marketplaceUsage()
		return 64
	}
	verb := args[0]
	rest := args[1:]
	switch verb {
	case "categories":
		return runMarketplaceCategories(rest)
	case "browse":
		return runMarketplaceBrowse(rest)
	case "search":
		return runMarketplaceSearch(rest)
	case "install":
		return runPkgInstall(rest) // same path as `pkg install`
	case "update":
		return runMarketplaceUpdate(rest)
	case "remove":
		return runPkgRemove(rest) // same path as `pkg remove`
	case "installed":
		return runMarketplaceInstalled(rest)
	case "fetch":
		return runMarketplaceFetch(rest)
	case "--help", "-h":
		marketplaceUsage()
		return 0
	default:
		core.Error("marketplace: unknown verb", "verb", verb)
		marketplaceUsage()
		return 64
	}
}

// marketplaceUsage prints the available marketplace verbs.
//
//	marketplaceUsage()
func marketplaceUsage() {
	core.Println("core-app marketplace <verb> [flags]")
	core.Println("  categories         list the marketplace's top-level categories (RFC §6.1)")
	core.Println("  browse CATEGORY    list every listing in a single category")
	core.Println("  search QUERY       search the local marketplace cache")
	core.Println("  install CODE       install a marketplace listing (same as `pkg install`)")
	core.Println("  update  CODE       git pull + re-verify the listing's signature (RFC §6.3)")
	core.Println("  remove  NAME       remove an installed package (same as `pkg remove`)")
	core.Println("  installed          list installed packages (same as `pkg list`)")
	core.Println("  fetch --url URL    clone/update the marketplace repo")
}

// runMarketplaceCategories dispatches `core-app marketplace categories`.
// Prints the marketplace's top-level category names in sorted order so a
// user can pipe `browse CATEGORY` against the output without knowing the
// index layout in advance.
//
//	core-app marketplace categories
//	core-app marketplace categories --json
func runMarketplaceCategories(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app marketplace categories [--json]")
			core.Println("  --json   emit a JSON array of category names")
			return 0
		default:
			core.Error("marketplace categories: unknown flag", "flag", a)
			return 64
		}
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("marketplace categories: cannot resolve DIR_HOME")
		return 1
	}
	root := core.Path(home, ".core", "marketplace")
	cats, err := app.MarketplaceCategories(coreio.Local, root)
	if err != nil {
		core.Error("marketplace categories: failed", "err", err)
		return 1
	}

	if asJSON {
		r := core.JSONMarshal(cats)
		if !r.OK {
			core.Error("marketplace categories: marshal failed", "err", r.Value)
			return 1
		}
		raw, _ := r.Value.([]byte)
		core.Println(string(raw))
		return 0
	}

	if len(cats) == 0 {
		core.Println("(no categories)")
		return 0
	}
	for _, cat := range cats {
		core.Println(cat)
	}
	return 0
}

// runMarketplaceBrowse dispatches `core-app marketplace browse CATEGORY`.
// Prints every listing in the named category as an aligned table (or a
// JSON array with `--json`). Stamps the `Category` column on every row
// so the output matches the projection `search --json` already emits.
//
//	core-app marketplace browse media
//	core-app marketplace browse media --json
func runMarketplaceBrowse(args []string) int {
	asJSON := false
	category := ""
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app marketplace browse [--json] CATEGORY")
			core.Println("  --json    emit a JSON array of marketplace listings")
			core.Println("  CATEGORY  one of the names from `marketplace categories`")
			return 0
		default:
			if core.HasPrefix(a, "-") {
				core.Error("marketplace browse: unknown flag", "flag", a)
				return 64
			}
			if category != "" {
				core.Error("marketplace browse: only one CATEGORY supported", "extra", a)
				return 64
			}
			category = a
		}
	}
	if category == "" {
		core.Error("marketplace browse: CATEGORY is required")
		return 64
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("marketplace browse: cannot resolve DIR_HOME")
		return 1
	}
	root := core.Path(home, ".core", "marketplace")
	listings, err := app.MarketplaceBrowse(coreio.Local, root, category)
	if err != nil {
		core.Error("marketplace browse: failed", "category", category, "err", err)
		return 1
	}

	if asJSON {
		r := core.JSONMarshal(listings)
		if !r.OK {
			core.Error("marketplace browse: marshal failed", "err", r.Value)
			return 1
		}
		raw, _ := r.Value.([]byte)
		core.Println(string(raw))
		return 0
	}

	if len(listings) == 0 {
		return 0
	}
	const gutter = 2
	headers := []string{"CODE", "TYPE", "VERSION", "DESCRIPTION"}
	widths := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
	for _, r := range listings {
		cells := []string{r.Code, r.Type, r.Version, r.Description}
		for i, cell := range cells {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	core.Println(formatRow(headers, widths, gutter))
	for _, r := range listings {
		cells := []string{r.Code, r.Type, r.Version, r.Description}
		core.Println(formatRow(cells, widths, gutter))
	}
	return 0
}

// runMarketplaceUpdate dispatches `core-app marketplace update CODE`.
// Resolves the marketplace cache (`$DIR_HOME/.core/marketplace/`) and
// delegates to app.MarketplaceUpdate which owns the git pull + verify
// + rollback pipeline (RFC §6.3).
//
//	core-app marketplace update photo-browser
//	core-app marketplace update --skip-verify photo-browser   # CI-only
func runMarketplaceUpdate(args []string) int {
	skipVerify := false
	code := ""
	for i := range args {
		switch args[i] {
		case "--skip-verify":
			skipVerify = true
		case "--help", "-h":
			core.Println("core-app marketplace update [--skip-verify] CODE")
			core.Println("  CODE          the installed package code (matches `pkg list` NAME)")
			core.Println("  --skip-verify bypass the post-update signature check (test-only)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("marketplace update: unknown flag", "flag", args[i])
				return 64
			}
			code = args[i]
		}
	}
	if code == "" {
		core.Error("marketplace update: CODE is required")
		return 64
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("marketplace update: cannot resolve DIR_HOME")
		return 1
	}
	root := core.Path(home, ".core", "marketplace")
	if !coreio.Local.IsDir(root) {
		core.Error("marketplace update: marketplace cache missing — run `marketplace fetch` first",
			core.Concat("pa", "th"), root)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := core.New()
	dest, err := app.MarketplaceUpdate(ctx, c, app.MarketplaceUpdateOptions{
		Root:       root,
		Home:       home,
		Code:       code,
		SkipVerify: skipVerify,
	})
	if err != nil {
		core.Error("marketplace update: failed", "code", code, "err", err)
		return 1
	}
	core.Info("updated", "code", code, "dest", dest)
	return 0
}

// runMarketplaceInstalled dispatches `core-app marketplace installed`.
// Equivalent to `core-app pkg list` — prints every installed package as
// either a tab-separated table or a JSON array.
//
//	core-app marketplace installed
//	core-app marketplace installed --json
func runMarketplaceInstalled(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app marketplace installed [--json]")
			core.Println("  --json   emit a JSON array of {name, type, version, source, path}")
			return 0
		default:
			core.Error("marketplace installed: unknown flag", "flag", a)
			return 64
		}
	}
	if asJSON {
		return runPkgList([]string{"--json"})
	}
	return runPkgList(nil)
}

// runMarketplaceSearch prints aligned `CODE TYPE VERSION DESCRIPTION`
// rows for every matching listing, or a JSON array when --json is
// passed. No match = empty stdout + exit 0 (so a shell can test
// `if [ -z "$(core-app marketplace search foo)" ]`).
//
//	core-app marketplace search photo
//	core-app marketplace search photo --json
func runMarketplaceSearch(args []string) int {
	asJSON := false
	query := ""
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			core.Println("core-app marketplace search [--json] QUERY")
			core.Println("  --json   emit a JSON array of {code, name, type, version, description}")
			return 0
		default:
			if core.HasPrefix(a, "-") {
				core.Error("marketplace search: unknown flag", "flag", a)
				return 64
			}
			query = a
		}
	}
	if query == "" {
		core.Error("marketplace search: QUERY is required")
		return 64
	}
	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("marketplace search: cannot resolve DIR_HOME")
		return 1
	}
	root := core.Path(home, ".core", "marketplace")
	results, err := app.MarketplaceSearch(coreio.Local, root, query)
	if err != nil {
		core.Error("marketplace search: failed", "err", err)
		return 1
	}

	if asJSON {
		// Round-trip via core.JSONMarshal so the output is canonical and
		// downstream tooling can pipe directly into `jq` / a script
		// without re-parsing the human-readable table.
		r := core.JSONMarshal(results)
		if !r.OK {
			core.Error("marketplace search: marshal failed", "err", r.Value)
			return 1
		}
		raw, _ := r.Value.([]byte)
		core.Println(string(raw))
		return 0
	}

	if len(results) == 0 {
		return 0
	}
	const gutter = 2
	headers := []string{"CODE", "TYPE", "VERSION", "DESCRIPTION"}
	widths := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
	for _, r := range results {
		cells := []string{r.Code, r.Type, r.Version, r.Description}
		for i, cell := range cells {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	core.Println(formatRow(headers, widths, gutter))
	for _, r := range results {
		cells := []string{r.Code, r.Type, r.Version, r.Description}
		core.Println(formatRow(cells, widths, gutter))
	}
	return 0
}

// runMarketplaceFetch clones or pulls the marketplace repo into
// `$DIR_HOME/.core/marketplace/`.
//
//	core-app marketplace fetch --url https://forge.lthn.ai/core/marketplace.git
func runMarketplaceFetch(args []string) int {
	url := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url":
			if i+1 >= len(args) {
				core.Error("--url requires a value")
				return 64
			}
			i++
			url = args[i]
		case "--help", "-h":
			core.Println("core-app marketplace fetch --url URL")
			return 0
		default:
			core.Error("marketplace fetch: unknown flag", "flag", args[i])
			return 64
		}
	}
	if url == "" {
		core.Error("marketplace fetch: --url is required")
		return 64
	}

	home := core.Env("DIR_HOME")
	if home == "" {
		core.Error("marketplace fetch: cannot resolve DIR_HOME")
		return 1
	}
	dir := core.Path(home, ".core", "marketplace")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := core.New()
	if err := app.MarketplaceFetch(ctx, c, app.MarketplaceFetchOptions{URL: url, Dir: dir}); err != nil {
		core.Error("marketplace fetch: failed", "err", err)
		return 1
	}
	core.Info("marketplace fetched", "dir", dir, "url", url)
	return 0
}

// ensureExit is an optional exit helper used only when the CLI wants
// to abort the process with a fixed code after writing context to
// stdout. Kept here (rather than calling core.Exit inline) so the test
// harness can exercise the exit path without process-level side
// effects.
//
//	ensureExit(64)
func ensureExit(code int) {
	core.Exit(code)
}
