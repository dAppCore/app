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
//
// This binary is the thin CLI shell around app.Boot, app.Compile and
// app.Sign. Real orchestration lives in the app package; main is here to
// parse argv and print the booted identity so an agent can verify
// "did it read my manifest?".
package main

import (
	"context"
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
		}
	}

	runBoot(args)
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
			core.Println("  compile   compile .core/view.yaml → core.json")
			core.Println("  sign      sign .core/view.yaml with a private key")
			core.Println("  keygen    generate a paired ed25519 keypair")
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
	Start string
	Key   string
}

// runCompile reads `.core/view.yaml`, optionally signs the in-memory
// manifest when --key is supplied, compiles to a CompiledManifest and
// writes `core.json` at the project root.
//
//	core-app compile
//	core-app compile ./photo-browser
//	core-app compile --key ~/.core/keys/app.key
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
		case "--help", "-h":
			core.Println("core-app compile [--key PATH] [project-dir]")
			core.Println("  --key     hex-encoded ed25519 private key (re-sign before compile)")
			core.Println("  project   project root holding .core/view.yaml (default: ./)")
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

	if opts.Key != "" {
		priv, err := app.LoadPrivateKey(medium, opts.Key)
		if err != nil {
			core.Error("compile: private key load failed", "path", opts.Key, "err", err)
			return 1
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
	Start string
	Key   string
}

// runSign signs `.core/view.yaml` in place using the private key at the
// supplied path. Exits non-zero on any failure so a CI pipeline catches
// the mistake before publication.
//
//	core-app sign --key ~/.core/keys/app.key
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
		case "--help", "-h":
			core.Println("core-app sign --key PATH [project-dir]")
			core.Println("  --key     hex-encoded ed25519 private key (.key file)")
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

	if opts.Key == "" {
		core.Error("sign: --key is required")
		return 64
	}

	medium := coreio.Local

	path := config.FindManifest(medium, opts.Start, config.FileView)
	if path == "" {
		core.Error("sign: no .core/view.yaml found", "start", opts.Start)
		return 1
	}

	priv, err := app.LoadPrivateKey(medium, opts.Key)
	if err != nil {
		core.Error("sign: private key load failed", "path", opts.Key, "err", err)
		return 1
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
