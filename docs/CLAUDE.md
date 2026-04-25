# CoreApp — The Application Contract (Keystone Spec)

> Agent context summary for `plans/code/core/app/`. Read this first. Every other RFC exists to make this work.

## What CoreApp Is

The keystone spec. A CoreApp is a directory with a `.core/view.yaml` manifest. Run `core` in that directory — it discovers the manifest, verifies its ed25519 signature, enforces declared permissions, and boots the application. Local-first: data lives on the user's device, network is for federation. Heritage: every `.core/` convention traces to a dAppServer prototype (2021, EUPL-1.2).

## Key Facts

- **Status**: Specced (973 lines), implementing
- **Module**: `dappco.re/go/core` (CoreApp subsystem)
- **Repo**: `core/go` (framework)
- **Depends on**: core/config, core/gui, core/go/ipc, core/go/build, core/go/cli, core/ts, go-io, go-store, go-crypt, go-html
- **Prior art**: dAppServer (2021) — object store, PGP auth, process management, marketplace

## Architecture

### The Contract

```
Developer: .core/view.yaml (what the app needs)
Core:      core.json (compiled, signed, distribution-ready)
User:      core (discovers, verifies, boots)
Comms:     Named Actions (sandboxed by manifest permissions)
Data:      ~/.core/data/{workspace}/ (encrypted at rest via Sigil)
Dist:      Git marketplace (signed manifests)
```

### Boot Sequence

Discover `.core/view.yaml` → Verify ed25519 signature → Parse permissions → Load modules → Compose HLCRF layout → Render config templates → Start CoreGUI (desktop) or serve (web/PWA)

### Permission Enforcement

Manifest declares: `read`, `write`, `net`, `run`, `store`. If not declared, app CAN'T access it. Runtime enforces at the framework level, not the app level. Same principle as Deno's `--allow-*` flags but signed and verified.

### Security Model

1. Manifest signature (ed25519) — app identity
2. Permission declarations — explicit capabilities
3. Runtime enforcement — Core blocks undeclared access
4. Sandbox boundary — CWD = immutable root (SASE containment)
5. Encrypted storage — Sigil (ChaCha20-Poly1305 + Pre-Obfuscation)
6. No TLS needed — PGP key dance for local auth

## Distribution Targets

One manifest, every platform: Desktop (Wails v3), iOS/Android (Wails v3 alpha 74), Web (PWA), CLI (headless), Edge (Deno Deploy).

## Plugin & Services

Plugins are isolated `core.New()` instances with own manifest, services, permissions. `WithServiceLock()` prevents registration beyond manifest. Inter-plugin communication via Named Actions only — never direct calls.

## External App Packaging (`core pkg`)

- **PWA**: `core pkg wrap --pwa <url>` — manifest.json → view.yaml, permission mapping, service worker replaced by Core background services
- **Electron**: `core pkg wrap --electron <repo>` — downloads renderer assets only (not binary), injects Electron shim, auto-detects permissions from source scanning
- **Web**: `core pkg wrap --web <dir>` — local web directory as CoreApp
- **Auto-detection**: `core pkg install` detects type from marketplace index, package.json, or manifest.json

## Marketplace

Git-based, no server. Install = git clone (depth=1) → verify signature → register. Category-as-directory JSON index. Updates = git pull + re-verify. Failed signature = rollback.

## SDK Generation

Manifest → `core compile` → core.json → `core sdk generate` → TypeScript bindings + OpenAPI spec + client SDKs (TS, Python, Go, PHP). Go handler structs ARE the contracts — no separate DTO layer.

## Critical Rules

- **Manifest is source, core.json is compiled output** — core.json lives at project root, not inside .core/
- **Permission = capability** — undeclared = denied. Period.
- **Dev mode**: no signature, hot reload, warnings instead of errors. `core --dev`
- **Plugins never bypass Core** — all communication through Named Actions
- **DTO pattern**: Go structs with json tags, not loose Options. Structs feed codegen
- **Signing covers manifest content minus the sign field** — ed25519, public key in marketplace index

## Spec Index

| File | Scope |
|------|-------|
| [RFC.md](RFC.md) | **Keystone spec** — manifest, compile, sign, runtime, sandbox, storage, marketplace, SDK, plugins, external packaging (973 lines) |
