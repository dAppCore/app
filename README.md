<!-- SPDX-License-Identifier: EUPL-1.2 -->

# core/app

> Lethean app installer + workspace manager — pkg discovery, manifest, integrity, runtime

[![CI](https://github.com/dappcore/app/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/dappcore/app/actions/workflows/ci.yml)
[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=alert_status)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Coverage](https://codecov.io/gh/dappcore/app/branch/dev/graph/badge.svg)](https://codecov.io/gh/dappcore/app)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=security_rating)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=sqale_rating)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=reliability_rating)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Code Smells](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=code_smells)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=dappcore_app&metric=ncloc)](https://sonarcloud.io/dashboard?id=dappcore_app)
[![Go Reference](https://pkg.go.dev/badge/dappco.re/go/app.svg)](https://pkg.go.dev/dappco.re/go/app)
[![License: EUPL-1.2](https://img.shields.io/badge/License-EUPL--1.2-blue.svg)](https://eupl.eu/1.2/en/)


The CoreApp keystone runtime — reads a `.core/view.yaml` manifest, runs the 7-step boot, enforces permissions, composes layout, starts the app.

> "The keystone spec. Every other RFC exists to make this work."

**Module:** `dappco.re/go/app`
**Spec:** [`docs/RFC.md`](docs/RFC.md)

## The 7-step boot

```
Discover → Verify → Permissions → Modules → Layout → Config → Start
```

Every CoreApp (CorePlay, lthn.ai, OFM agency, BugSETI) runs through this sequence.

## Quick start

```bash
# Discover the CoreApp in the current directory and boot in prod mode.
core-app

# Dev mode: skip signature verification, warn on permission misses,
# hot-reload on `.core/view.yaml` edits.
core-app --dev --watch ./photo-browser

# Compile the manifest to the signed distribution artifact.
core-app compile --verify --sign

# Lint the manifest against RFC §2 rules.
core-app validate --strict --json

# Wrap an external app.
core-app pkg wrap --pwa https://app.example.com
core-app pkg wrap --electron github.com/foo/bar
core-app pkg wrap --web ./my-webapp

# Manage packages.
core-app pkg list
core-app pkg info NAME
core-app pkg remove [--purge] NAME
core-app pkg update NAME

# Generate SDKs for the declared action surface.
core-app sdk list            # show the Core primitive action catalogue
core-app sdk generate        # emit TS, Go, PHP, Python + OpenAPI
core-app sdk generate --lang ts --out ./build/sdk

# Marketplace integration.
core-app marketplace fetch --url https://forge.lthn.ai/core/marketplace.git
core-app marketplace categories
core-app marketplace browse media
core-app marketplace search photo
core-app marketplace install photo-browser
core-app marketplace update  photo-browser
```

## Dependencies

```
core/app → core/config (.core/view.yaml parsing, XDG, features)
        → core/gui (window boot — hand-off hook)
        → core/go/io (SASE sandbox)
        → core/go (primitives, IPC, Process, JSONMarshal)
```

## Scope

- IN: manifest loader, boot orchestrator, permission→action gate wiring,
  manifest-driven window boot, `core run <app-code>`, workspace lifecycle,
  `core compile` / `core sign` / `core validate`, `core pkg` external-app
  packaging (PWA / Electron / Web), marketplace install / update / verify
  with signature rollback, plugin host (RFC §11), conclave isolation
  (RFC §11.4), SDK generation (OpenAPI + TS / Go / PHP / Python).
- OUT: window primitives (core/gui), Wails coroutine wiring, model
  inference (go-mlx, go-inference), encryption-at-rest (Sigil lives in
  core/go/io).

## Status

Feature-complete against RFC §2–§16. Implementation map:

| Area | Files |
|------|-------|
| Boot pipeline (7 steps) | `app.go`, `discover.go`, `verify.go`, `permissions.go`, `modules.go`, `layout.go`, `config.go`, `start.go` |
| Compile + sign + verify | `compile.go`, `sign.go`, `verify.go`, `validate.go` |
| Packaging (§16) | `pkg.go`, `pkg_pwa.go`, `pkg_electron.go`, `pkg_web.go`, `pkg_type.go`, `pkg_electron_fetch.go`, `pkg_electron_extract*.go` |
| Marketplace (§6) | `marketplace.go`, `marketplace_verify.go` |
| Plugin host (§11) | `host.go`, `plugin.go`, `conclave.go`, `registry.go` |
| Dev mode (§4.2) | `watch.go` |
| SDK generation (§8) | `sdk.go` |
| Workspace (§5) | `workspace.go` |
| Access control (§10) | `access.go` |
| CLI | `cmd/core-app/` |

## Licence

EUPL-1.2
