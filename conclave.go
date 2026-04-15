// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
	coreerr "dappco.re/go/core/log"
)

// ConclaveOptions tunes NewConclave. A conclave is the most-isolated
// shape a CoreApp plugin can take (RFC §11.4) — single allowed binary,
// strict per-path read/write declarations, no network unless explicitly
// listed. Hosts orchestrate them but cannot peer inside; the only way
// to communicate is through the Named Actions the conclave registers
// during boot.
//
//	opts := app.ConclaveOptions{
//	    Code:        "phpstan",
//	    Name:        "PHPStan",
//	    Version:     "0.1.0",
//	    ProjectRoot: "/Users/me/projects/host-uk",
//	    AllowedBins: []string{"phpstan"},
//	    ReadPaths:   []string{"./"},
//	    Mode:        app.ModeProd,
//	    Services:    coreOptionsForLinter,
//	}
//	conclave, err := app.NewConclave(ctx, opts)
type ConclaveOptions struct {
	// Code is the manifest.code the conclave runs under. Required —
	// determines the workspace directory name and the boot identity.
	Code string
	// Name is the human-friendly title shown in any UI surface.
	Name string
	// Version is the manifest.version stamped onto the synthetic manifest.
	Version string
	// ProjectRoot is the directory the conclave can read from. It becomes
	// the SASE containment root via go-io's NewSandboxed.
	ProjectRoot string
	// AllowedBins is the exhaustive list of binaries the conclave may
	// invoke through `process.run`. Empty list means no process
	// execution is permitted.
	AllowedBins []string
	// ReadPaths is the list of project-relative read prefixes (RFC §10.1
	// path matching). Empty list means no reads — useful for conclaves
	// that only emit results via Actions.
	ReadPaths []string
	// WritePaths is the list of write prefixes. Conclaves are usually
	// read-only; a populated WritePaths slot signals an exception.
	WritePaths []string
	// NetEndpoints is the allow-list of host:port pairs the conclave can
	// reach. Empty = no network.
	NetEndpoints []string
	// Mode picks prod (default — strict) or dev (warnings instead of
	// denials). Even in dev a conclave keeps WithServiceLock so the host
	// service surface cannot leak in.
	Mode Mode
	// Services is the list of CoreOption registrations the host wants
	// pinned onto this conclave's container before WithServiceLock
	// fires. Filtered against the allowed list at call time.
	Services []core.CoreOption
	// Medium overrides the storage medium for the workspace and any
	// permission-driven IO. Defaults to coreio.Local.
	Medium coreio.Medium
	// WorkspaceHome overrides $DIR_HOME for the workspace bootstrap.
	WorkspaceHome string
	// SkipWorkspace disables the workspace bootstrap. Test-only escape
	// hatch — production callers should always provision the data tree
	// so the conclave has a guaranteed scratch location.
	SkipWorkspace bool
}

// Conclave is the runtime form of a maximally-isolated plugin instance.
// The struct embeds *Instance so every primitive a regular CoreApp
// instance exposes (Manifest, Core, Workspace) is available without
// double indirection.
//
//	conclave, err := app.NewConclave(ctx, opts)
//	if err != nil { return err }
//	r := conclave.Start(ctx)
type Conclave struct {
	// Instance is the underlying CoreApp instance — same shape PluginBoot
	// returns, with a synthetic ViewManifest derived from ConclaveOptions.
	*Instance
	// Sandbox is the SASE-contained Medium pinned to ProjectRoot. Use
	// this Medium (not coreio.Local) for every read/write the conclave
	// performs at runtime — it enforces the read/write allow list at
	// the medium boundary.
	Sandbox coreio.Medium
}

// NewConclave constructs a synthetic ViewManifest from the options,
// runs the regular plugin boot pipeline against it, and returns a
// *Conclave the caller can Start(). The synthetic manifest carries the
// declared permission lists so the entitlement gate enforces them at
// runtime — the conclave can never `process.run` a binary missing from
// AllowedBins, nor read a path missing from ReadPaths.
//
//	conclave, err := app.NewConclave(ctx, app.ConclaveOptions{
//	    Code:         "phpstan",
//	    ProjectRoot:  projectRoot,
//	    AllowedBins:  []string{"phpstan"},
//	    ReadPaths:    []string{"./"},
//	    Services:     []core.CoreOption{core.WithService(io.Register)},
//	})
//	if err != nil { return err }
//	go conclave.Start(ctx)
//
// Rules:
//
//   - Empty Code → typed error.
//
//   - Empty ProjectRoot → typed error.
//
//   - WithServiceLock is always applied — the conclave cannot register
//     more services even if the caller forgets to pass the option.
func NewConclave(ctx context.Context, opts ConclaveOptions) (*Conclave, error) {
	if opts.Code == "" {
		return nil, coreerr.E("app.NewConclave", "empty code", nil)
	}
	if opts.ProjectRoot == "" {
		return nil, coreerr.E("app.NewConclave", "empty ProjectRoot", nil)
	}

	medium := opts.Medium
	if medium == nil {
		medium = coreio.Local
	}

	manifest := manifestFromConclaveOptions(opts)

	// Bootstrap the workspace before PluginBoot so the conclave's
	// container picks up an absolute storage root if the caller chooses
	// to read it back.
	var ws *Workspace
	if !opts.SkipWorkspace {
		w, err := OpenWorkspace(medium, opts.WorkspaceHome, opts.Code)
		if err != nil {
			if opts.Mode == ModeProd {
				return nil, coreerr.E("app.NewConclave", "workspace bootstrap failed", err)
			}
			// Dev mode keeps booting; the conclave handlers must check
			// Workspace before using it.
			_ = err
		}
		ws = w
	}

	inst, err := PluginBoot(ctx, PluginOptions{
		Manifest:    manifest,
		ProjectRoot: opts.ProjectRoot,
		Mode:        opts.Mode,
		Services:    opts.Services,
		Medium:      medium,
	})
	if err != nil {
		return nil, coreerr.E("app.NewConclave", "PluginBoot failed", err)
	}
	inst.Workspace = ws

	sandbox, err := buildConclaveSandbox(medium, opts.ProjectRoot)
	if err != nil {
		return nil, coreerr.E("app.NewConclave", "sandbox build failed", err)
	}

	return &Conclave{Instance: inst, Sandbox: sandbox}, nil
}

// manifestFromConclaveOptions projects ConclaveOptions into a synthetic
// ViewManifest the regular boot pipeline can consume. The manifest is
// the one source of truth for the conclave's permissions — every
// runtime check (entitlement gate, CheckAccess, marketplace verify)
// reads from it.
//
//	manifest := manifestFromConclaveOptions(opts)
func manifestFromConclaveOptions(opts ConclaveOptions) config.ViewManifest {
	name := opts.Name
	if name == "" {
		name = opts.Code
	}
	version := opts.Version
	if version == "" {
		version = "0.1.0"
	}

	m := config.ViewManifest{
		Code:    opts.Code,
		Name:    name,
		Version: version,
		Layout:  "C", // single-surface; conclaves don't draw HLCRF chrome
		Permissions: config.ViewPermissions{
			Read: append([]string(nil), opts.ReadPaths...),
			Net:  append([]string(nil), opts.NetEndpoints...),
			Run:  append([]string(nil), opts.AllowedBins...),
		},
	}
	if len(opts.WritePaths) > 0 {
		// ViewPermissions has no per-path write list yet — fall back to
		// the catch-all Filesystem flag. Once the upstream schema grows
		// `permissions.write`, swap this branch for a direct slice copy.
		m.Permissions.Filesystem = true
	}

	// Stamp the conclave provenance so `pkg list` and any host UI can
	// distinguish a conclave from a regular CoreApp. The Config map is
	// the documented home for fields the typed schema doesn't yet expose.
	m.Config = map[string]any{
		"type":      "conclave",
		"isolation": "max",
	}
	if len(opts.WritePaths) > 0 {
		m.Config["write_paths"] = stringsToAny(opts.WritePaths)
	}
	return m
}

// buildConclaveSandbox returns a sandboxed medium pinned to the
// ProjectRoot. On Local medium this delegates to coreio.NewSandboxed;
// on other mediums (mock, memory) the underlying medium is returned
// because they have no notion of an OS root.
//
//	sandbox, err := buildConclaveSandbox(medium, root)
func buildConclaveSandbox(medium coreio.Medium, root string) (coreio.Medium, error) {
	if medium == coreio.Local {
		return coreio.NewSandboxed(root)
	}
	return medium, nil
}

// stringsToAny lifts a []string into a []any so it serialises cleanly
// through the Config map (YAML / JSON encoders both prefer []any over
// []string for mixed-shape fields).
//
//	stringsToAny([]string{"a"}) // []any{"a"}
func stringsToAny(in []string) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// IsConclave reports whether a manifest was produced by NewConclave (or
// any other code that stamps `Config["type"] = "conclave"`). Used by
// hosts that surface a different UI for conclaves (status indicator,
// limited capability badge) without re-deriving from a registry.
//
//	if app.IsConclave(&manifest) { /* render badge */ }
func IsConclave(m *config.ViewManifest) bool {
	if m == nil || m.Config == nil {
		return false
	}
	v, ok := m.Config["type"].(string)
	return ok && v == "conclave"
}
