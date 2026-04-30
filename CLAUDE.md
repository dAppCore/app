# CLAUDE.md — core/app

Reference: `docs/RFC.md` — **keystone spec**. Every other Core RFC exists to make this work.

## Identity

`dappco.re/go/app` — CoreApp runtime. Reads `.core/view.yaml`, runs the 7-step boot, enforces manifest permissions on Named Actions, composes HLCRF layout into the gui window, starts the app.

## The 7-step boot (RFC §4)

1. **Discover** — find `.core/view.yaml` via go-scm manifest loader
2. **Verify** — check ed25519 signature via go-scm
3. **Permissions** — bind manifest `permissions:` to Core action gates (fs.read, store.get, net.fetch, etc.)
4. **Modules** — load declared modules on the Core instance
5. **Layout** — compose HLCRF slots from manifest (`slots:`) via go-html
6. **Config** — apply template + vars via core/config
7. **Start** — hand the composed window to core/gui + unblock entry action

## Repo Layout

Go module now lives under `go/` and the repo root is cross-language / metadata only:

```
core/app/
├── go/                                  ← Go module root (dappco.re/go/app)
│   ├── *.go
│   ├── go.mod, go.sum
│   ├── cmd/
│   ├── tests/
│   ├── README.md                        → ../README.md
│   ├── CLAUDE.md                        → ../CLAUDE.md
│   ├── AGENTS.md                        → ../AGENTS.md
│   └── docs                             → ../docs
├── docs/                                ← cross-language docs source
├── README.md                            ← repo index / overview
├── CLAUDE.md                            ← this repo guidance
├── AGENTS.md                            ← repo-level instructions
├── LICENSE/LICENCE
├── .woodpecker.yml
├── sonar-project.properties
└── .gitignore
```

## Go Resolution Modes

This repo is intentionally single-module from `go/` with no `go.work` in use:

| Mode | When | What runs |
|------|------|-----------|
| **Module mode (default)** | Local work from `go/` | `go test`, `go vet`, `go test ./...`, and CLI builds use `go.mod` directly. |
| **Repro/CI mode** | Verification scripts | Use explicit override: `GOWORK=off GOPROXY=direct GOSUMDB=off GOFLAGS=-mod=mod` for stable behavior and strict cache isolation. |

## Dependency boundary

CoreApp is the orchestrator. It does NOT own:
- Manifest parsing / signing / verification → **go-scm**
- Config loading / discovery / watch → **core/config**
- Window creation → **core/gui**
- SASE sandbox / Sigil encryption → **go-io**
- Permissioned KV → **go-store**
- Primitives → **core/go**

## Scope boundary

- IN: boot orchestration, permission → action gate binding, `core run <app-code>` CLI, workspace lifecycle management, `.core/` directory discovery and enforcement, manifest-driven module + layout composition
- OUT: anything a dependency owns (see above)

## Core conventions

- Banned imports: fmt, errors, os, os/exec, strings, path/filepath, encoding/json, log → core primitives
- Tests: `TestFilename_Function_{Good,Bad,Ugly}` — one test file per source file, all three mandatory
- UK English, usage-example comments, predictable names
- Never manually edit `go.mod` — use `go get`, `go mod tidy`

## Blocker state

CoreApp cannot land until these are spec-complete:
- ✅ core/config — 8 opus passes, near alpha.1
- 🟡 go-scm — mini + opus rotating, manifest/sign/verify/marketplace mostly done
- 🟡 core/gui — ~60% spec coverage, Wails stub incomplete
- ✅ go-io — Cube Medium + Named Actions shipped
- ✅ go-store — RFC fully implemented (last audit)

When go-scm hits alpha.1 and the `core compile` + `core sign` + `core pkg install` commands are wired into the `core` binary, CoreApp has the substrate it needs.
