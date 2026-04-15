// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"dappco.re/go/app"
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// runSDK dispatches the `sdk` subverbs. Today the only verb is
// `generate`; future iterations will add `list` (catalogue dump) and
// `validate` (lint a hand-written manifest against a target language).
//
//	core-app sdk generate
//	core-app sdk generate --lang ts --out ./build/sdk
func runSDK(args []string) int {
	if len(args) == 0 {
		sdkUsage()
		return 64
	}
	switch args[0] {
	case "generate":
		return runSDKGenerate(args[1:])
	case "--help", "-h":
		sdkUsage()
		return 0
	default:
		core.Error("sdk: unknown verb", "verb", args[0])
		sdkUsage()
		return 64
	}
}

// sdkUsage prints the available `sdk` subverbs.
//
//	sdkUsage()
func sdkUsage() {
	core.Println("core-app sdk <verb> [flags]")
	core.Println("  generate [--lang ts|go|php|python|openapi] [--out DIR] [--all] [project]")
	core.Println("           emit client SDKs from .core/view.yaml; default emits all five")
}

// sdkGenerateArgs captures the flags the `sdk generate` subverb
// understands. Mirrors the SDKGenerateOptions surface but with CLI
// types (slice of language tokens, string out dir).
type sdkGenerateArgs struct {
	Start    string
	OutDir   string
	Langs    []app.SDKLanguage
	Includes bool
}

// runSDKGenerate parses flags, loads the project's manifest and calls
// app.SDKGenerate. Exits non-zero on any failure so a CI pipeline can
// gate publication on a successful generation.
//
//	core-app sdk generate
//	core-app sdk generate --lang ts --out ./build/sdk
//	core-app sdk generate --lang openapi ./photo-browser
func runSDKGenerate(args []string) int {
	opts := sdkGenerateArgs{Start: "./"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lang":
			if i+1 >= len(args) {
				core.Error("--lang requires a value")
				return 64
			}
			i++
			lang, ok := app.ParseSDKLanguage(args[i])
			if !ok {
				core.Error("sdk generate: unknown --lang", "value", args[i])
				return 64
			}
			opts.Langs = append(opts.Langs, lang)
		case "--out":
			if i+1 >= len(args) {
				core.Error("--out requires a directory")
				return 64
			}
			i++
			opts.OutDir = args[i]
		case "--all":
			opts.Includes = true
		case "--help", "-h":
			core.Println("core-app sdk generate [--lang LANG] [--out DIR] [--all] [project]")
			core.Println("  --lang   one of openapi, ts, go, php, python (repeat for multiple)")
			core.Println("  --out    output directory (default: <project>/.core/sdk/)")
			core.Println("  --all    include every primitive Action regardless of permission")
			core.Println("  project  directory holding .core/view.yaml (default: ./)")
			return 0
		default:
			if core.HasPrefix(args[i], "-") {
				core.Error("sdk generate: unknown flag", "flag", args[i])
				return 64
			}
			opts.Start = args[i]
		}
	}

	medium := coreio.Local
	manifestPath := config.FindManifest(medium, opts.Start, config.FileView)
	if manifestPath == "" {
		core.Error("sdk generate: no .core/view.yaml found", "start", opts.Start)
		return 1
	}

	var manifest config.ViewManifest
	if err := config.LoadManifest(medium, manifestPath, &manifest); err != nil {
		core.Error("sdk generate: parse manifest failed", "path", manifestPath, "err", err)
		return 1
	}

	root := core.PathDir(core.PathDir(manifestPath))

	if err := app.SDKGenerate(medium, root, &manifest, app.SDKGenerateOptions{
		Languages:            opts.Langs,
		OutDir:               opts.OutDir,
		IncludeAllPrimitives: opts.Includes,
	}); err != nil {
		core.Error("sdk generate: failed", "err", err)
		return 1
	}

	out := opts.OutDir
	if out == "" {
		out = core.Path(root, ".core", "sdk")
	}
	core.Info("sdk generated",
		"code", manifest.Code,
		"version", manifest.Version,
		"out", out,
	)
	return 0
}
