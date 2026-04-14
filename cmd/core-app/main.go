// SPDX-License-Identifier: EUPL-1.2

// Command core-app boots a CoreApp from a `.core/view.yaml` manifest
// and runs the 7-step boot sequence. Usage:
//
//	core-app                 # boot the CoreApp in the current directory
//	core-app ./photo-browser # boot a specific directory
//	core-app --dev ./        # dev mode (no signature, warnings only)
//
// This binary is the thin CLI shell around app.Boot. Real orchestration
// lives in the app package; main is here to parse argv and print the
// booted identity so an agent can verify "did it read my manifest?".
package main

import (
	"context"
	"os"

	"dappco.re/go/app"
	core "dappco.re/go/core"
)

func main() {
	mode, start := parseArgs(os.Args[1:])

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
