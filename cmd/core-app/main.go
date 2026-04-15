// SPDX-License-Identifier: EUPL-1.2

// Command core-app boots a CoreApp from a `.core/view.yaml` manifest
// and runs the 7-step boot sequence. Usage:
//
//	core-app                     # boot the CoreApp in the current directory
//	core-app ./photo-browser     # boot a specific directory
//	core-app --dev ./            # dev mode (no signature, warnings only)
//	core-app compile             # compile .core/view.yaml → core.json
//	core-app sign --key app.key  # sign .core/view.yaml in place
//	core-app keygen --dir ~/.core/keys --name app
//	                              # generate a paired ed25519 keypair
//	core-app pkg list             # list installed packages
//	core-app pkg wrap --pwa URL   # wrap a PWA as a CoreApp
//	core-app pkg wrap --electron REPO
//	                              # wrap an Electron app as a CoreApp
//	core-app pkg wrap --web DIR   # wrap a local web dir as a CoreApp
//	core-app pkg remove NAME      # remove an installed package
//	core-app pkg update NAME      # re-fetch and re-wrap an installed pkg
//	core-app marketplace search Q # search the local marketplace cache
//	core-app marketplace install CODE
//	                              # install a marketplace listing
//	core-app marketplace update CODE
//	                              # git pull + re-verify (RFC §6.3)
//	core-app marketplace remove NAME
//	                              # remove an installed package
//	core-app marketplace installed
//	                              # list installed packages
//	core-app sdk generate         # generate client SDKs from .core/view.yaml
//	core-app sdk generate --lang ts --out ./sdk
//	                              # generate only the TypeScript SDK
//
// This binary is the thin CLI shell around app.Boot, app.Compile and
// app.Sign. Real orchestration lives in the app package; main is here to
// parse argv and print the booted identity so an agent can verify
// "did it read my manifest?".
package main

import (
	"context"
	"crypto/ed25519"
	"os"

	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

func main() {
	args := os.Args[1:]

	// Subcommand dispatch — the first positional that matches a verb
	// takes over. Falls through to the plain "boot" path otherwise so
	// `core-app` without args still boots the CoreApp in the cwd.
	if len(args) > 0 {
		switch args[0] {
		case "compile":
			os.Exit(runCompile(args[1:]))
		case "sign":
			os.Exit(runSign(args[1:]))
		case "keygen":
			os.Exit(runKeygen(args[1:]))
		case "pkg":
			os.Exit(runPkg(args[1:]))
		case "marketplace":
			os.Exit(runMarketplace(args[1:]))
		case "sdk":
			os.Exit(runSDK(args[1:]))
		case "run":
			os.Exit(runInstalled(args[1:]))
		}
	}

	runBoot(args)
}

// runInstalled boots an app by its installed code (RFC §4.0
// `core run <app-code>`). Resolves the path under
// `$DIR_HOME/.core/apps/<code>/` and delegates to runBoot.
//
//	core-app run photo-browser
//	core-app run --dev photo-browser
func runInstalled(args []string) int {
	mode := app.ModeProd
	code := ""
	for _, a := range args {
		switch a {
		case "--dev":
			mode = app.ModeDev
		case "--help", "-h":
			core.Println("core-app run [--dev] CODE")
			core.Println("  CODE   the installed package code (under ~/.core/apps/CODE/)")
			return 0
		default:
			if core.HasPrefix(a, "-") {
				core.Error("run: unknown flag", "flag", a)
				return 64
			}
			code = a
		}
	}
	if code == "" {
		core.Error("run: CODE is required")
		return 64
	}
	dir, err := app.DiscoverInstalled(coreio.Local, "", code)
	if err != nil {
		core.Error("run: discover failed", "code", code, "err", err)
		return 1
	}
	runBootFromMode(mode, dir)
	return 0
}

// runBootFromMode is a small helper that drives runBoot from a
// pre-resolved (mode, start) pair. Keeps the Boot wiring in one place
// even when the caller already knows the directory.
//
//	runBootFromMode(app.ModeDev, "/Users/me/.core/apps/photo-browser")
func runBootFromMode(mode app.Mode, start string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst, err := app.Boot(ctx, start, app.WithMode(mode))
	if err != nil {
		core.Error("boot failed", "start", start, "mode", mode.String(), "err", err)
		os.Exit(1)
	}
	core.Info("CoreApp booted",
		"code", inst.Manifest.Code,
		"name", inst.Manifest.Name,
		"version", inst.Manifest.Version,
		"mode", inst.Mode.String(),
		"root", inst.Root,
	)
	if r := inst.Start(ctx); !r.OK {
		core.Error("start failed", "err", r.Value)
		os.Exit(2)
	}
}

// runBoot drives the "boot a CoreApp" path. Extracted from main so the
// subcommand dispatch reads cleanly.
//
//	runBoot(os.Args[1:])
func runBoot(args []string) {
	mode, start := parseArgs(args)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst, err := app.Boot(ctx, start, app.WithMode(mode))
	if err != nil {
		core.Error("boot failed", "start", start, "mode", mode.String(), "err", err)
		os.Exit(1)
	}

	core.Info("CoreApp booted",
		"code", inst.Manifest.Code,
		"name", inst.Manifest.Name,
		"version", inst.Manifest.Version,
		"mode", inst.Mode.String(),
		"root", inst.Root,
	)

	if r := inst.Start(ctx); !r.OK {
		core.Error("start failed", "err", r.Value)
		os.Exit(2)
	}
}

// parseArgs is a deliberately small argv parser — no cobra, no pflag.
// Supported forms:
//
//	core-app                # mode=prod, start="./"
//	core-app ./path         # mode=prod, start="./path"
//	core-app --dev          # mode=dev,  start="./"
//	core-app --dev ./path   # mode=dev,  start="./path"
//
// Anything else prints usage and exits 64 (EX_USAGE).
func parseArgs(args []string) (app.Mode, string) {
	mode := app.ModeProd
	start := "./"

	for _, a := range args {
		switch a {
		case "--dev":
			mode = app.ModeDev
		case "--help", "-h":
			core.Println("core-app [--dev] [path]")
			core.Println("  --dev   skip signature verification, warn on permission misses")
			core.Println("  path    directory holding .core/view.yaml (default: ./)")
			core.Println("")
			core.Println("Subcommands:")
			core.Println("  compile      compile .core/view.yaml → core.json")
			core.Println("  sign         sign .core/view.yaml with a private key")
			core.Println("  keygen       generate a paired ed25519 keypair")
			core.Println("  sdk          generate client SDKs (openapi, ts, go, php)")
			core.Println("  run CODE     boot an installed package by code")
			core.Println("  pkg ...      manage packages (list, wrap, install, remove, update)")
			core.Println("  marketplace  search/install/fetch from the marketplace")
			os.Exit(0)
		default:
			if core.HasPrefix(a, "-") {
				core.Error("unknown flag", "flag", a)
				os.Exit(64)
			}
			start = a
		}
	}
	return mode, start
}

// compileArgs captures the flags understood by the `compile` subcommand.
type compileArgs struct {
	Start         string
	Key           string
	UseDefaultKey bool
}

// runCompile reads `.core/view.yaml`, optionally signs the in-memory
// manifest when --key or --default is supplied, compiles to a
// CompiledManifest and writes `core.json` at the project root.
//
//	core-app compile
//	core-app compile ./photo-browser
//	core-app compile --key ~/.core/keys/app.key
//	core-app compile --default              # use $DIR_HOME/.core/keys/default.key
func runCompile(args []string) int {
	opts := compileArgs{Start: "./"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key":
			if i+1 >= len(args) {
				core.Error("--key requires a path")
				return 64
			}
			i++
			opts.Key = args[i]
		case "--default":
			opts.UseDefaultKey = true
		case "--help", "-h":
			core.Println("core-app compile [--key PATH | --default] [project-dir]")
			core.Println("  --key      hex-encoded ed25519 private key (re-sign before compile)")
			core.Println("  --default  re-sign with $DIR_HOME/.core/keys/default.key")
			core.Println("  project    project root holding .core/view.yaml (default: ./)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("unknown compile flag", "flag", args[i])
				return 64
			}
			opts.Start = args[i]
		}
	}

	medium := coreio.Local

	path := config.FindManifest(medium, opts.Start, config.FileView)
	if path == "" {
		core.Error("compile: no .core/view.yaml found", "start", opts.Start)
		return 1
	}

	var manifest config.ViewManifest
	if err := config.LoadManifest(medium, path, &manifest); err != nil {
		core.Error("compile: parse failed", "path", path, "err", err)
		return 1
	}

	root := core.PathDir(core.PathDir(path))

	if opts.Key != "" || opts.UseDefaultKey {
		var priv ed25519.PrivateKey
		var err error
		if opts.UseDefaultKey {
			priv, err = app.LoadDefaultPrivateKey(medium)
			if err != nil {
				core.Error("compile: default key load failed", "err", err)
				return 1
			}
		} else {
			priv, err = app.LoadPrivateKey(medium, opts.Key)
			if err != nil {
				core.Error("compile: private key load failed", "path", opts.Key, "err", err)
				return 1
			}
		}
		if err := app.Sign(medium, path, priv); err != nil {
			core.Error("compile: sign failed", "path", path, "err", err)
			return 1
		}
		// Re-read so the compiled manifest carries the fresh Sign.
		if err := config.LoadManifest(medium, path, &manifest); err != nil {
			core.Error("compile: reload after sign failed", "err", err)
			return 1
		}
	}

	cm, err := app.Compile(&manifest, app.CompileOptions{})
	if err != nil {
		core.Error("compile: build failed", "err", err)
		return 1
	}

	if err := app.WriteCompiled(medium, root, cm); err != nil {
		core.Error("compile: write failed", "err", err)
		return 1
	}

	core.Info("compiled",
		"code", cm.Code,
		"version", cm.Version,
		"path", core.Path(root, app.CompiledFileName),
		"compiled_by", cm.CompiledBy,
	)
	return 0
}

// signArgs captures the flags understood by the `sign` subcommand.
type signArgs struct {
	Start         string
	Key           string
	UseDefaultKey bool
}

// runSign signs `.core/view.yaml` in place using the private key at the
// supplied path. Exits non-zero on any failure so a CI pipeline catches
// the mistake before publication.
//
//	core-app sign --key ~/.core/keys/app.key
//	core-app sign --default                 # use $DIR_HOME/.core/keys/default.key
//	core-app sign --key app.key ./photo-browser
func runSign(args []string) int {
	opts := signArgs{Start: "./"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key":
			if i+1 >= len(args) {
				core.Error("--key requires a path")
				return 64
			}
			i++
			opts.Key = args[i]
		case "--default":
			opts.UseDefaultKey = true
		case "--help", "-h":
			core.Println("core-app sign [--key PATH | --default] [project-dir]")
			core.Println("  (no flag) sign with $DIR_HOME/.core/keys/default.key (RFC §3.2)")
			core.Println("  --key     hex-encoded ed25519 private key (.key file)")
			core.Println("  --default explicit opt-in for $DIR_HOME/.core/keys/default.key")
			core.Println("  project   project root holding .core/view.yaml (default: ./)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("unknown sign flag", "flag", args[i])
				return 64
			}
			opts.Start = args[i]
		}
	}

	medium := coreio.Local

	path := config.FindManifest(medium, opts.Start, config.FileView)
	if path == "" {
		core.Error("sign: no .core/view.yaml found", "start", opts.Start)
		return 1
	}

	// Per RFC §3.2 — `core sign` with no flag signs with the default key
	// at $DIR_HOME/.core/keys/default.key. `--key PATH` overrides to a
	// specific file; `--default` is an explicit synonym kept for scripts
	// that want to be unambiguous.
	var priv ed25519.PrivateKey
	switch {
	case opts.Key != "":
		var err error
		priv, err = app.LoadPrivateKey(medium, opts.Key)
		if err != nil {
			core.Error("sign: private key load failed", "path", opts.Key, "err", err)
			return 1
		}
	default:
		// Either `--default` was passed or no flag was supplied — both
		// resolve to the default keyring location so the RFC one-liner
		// `core sign` works without surprises. `opts.UseDefaultKey`
		// stays true for log parity.
		_ = opts.UseDefaultKey
		var err error
		priv, err = app.LoadDefaultPrivateKey(medium)
		if err != nil {
			core.Error("sign: default key load failed "+
				"(run `core-app keygen --dir $DIR_HOME/.core/keys --name default` first)",
				"err", err)
			return 1
		}
	}

	if err := app.Sign(medium, path, priv); err != nil {
		core.Error("sign: failed", "path", path, "err", err)
		return 1
	}

	core.Info("signed", "path", path)
	return 0
}

// keygenArgs captures the flags understood by the `keygen` subcommand.
type keygenArgs struct {
	Dir  string
	Name string
}

// runKeygen creates a paired ed25519 keypair at `<dir>/<name>.key` +
// `<dir>/<name>.pub`. Matches the RFC §3.2 convention — `app.key` is the
// signing key, `app.pub` is the trust-list entry.
//
//	core-app keygen --dir ~/.core/keys --name app
func runKeygen(args []string) int {
	opts := keygenArgs{Name: "default"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 >= len(args) {
				core.Error("--dir requires a path")
				return 64
			}
			i++
			opts.Dir = args[i]
		case "--name":
			if i+1 >= len(args) {
				core.Error("--name requires a value")
				return 64
			}
			i++
			opts.Name = args[i]
		case "--help", "-h":
			core.Println("core-app keygen --dir PATH [--name NAME]")
			core.Println("  --dir     directory to write .key / .pub files (required)")
			core.Println("  --name    basename for the keypair (default: 'default')")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("unknown keygen flag", "flag", args[i])
				return 64
			}
		}
	}

	if opts.Dir == "" {
		core.Error("keygen: --dir is required")
		return 64
	}

	priv, pub, err := app.Keygen(coreio.Local, opts.Dir, opts.Name)
	if err != nil {
		core.Error("keygen: failed", "err", err)
		return 1
	}

	core.Info("keypair generated",
		"private", priv,
		"public", pub,
	)
	return 0
}
