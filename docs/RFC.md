---
module: core/app
repo: core/app
lang: multi
tier: consumer
depends:
  - code/core/config
  - code/core/gui
  - code/core/go/ipc
  - code/core/go/build
  - code/core/go/cli
  - code/core/ts
  - code/core/go/io
  - code/core/go/html
tags:
  - application
  - manifest
  - signing
  - distribution
  - keystone
---

# CoreApp RFC — The Application Contract

> The keystone spec. Every other RFC exists to make this work.
> A developer should be able to build, sign, and distribute a CoreApp from this document alone.

**Module:** `dappco.re/go/core` (CoreApp subsystem)
**Repository:** `core/go` (framework)
**Prior art:** dAppServer (2021, EUPL-1.2) — `~/Lethean/` convention, object store, marketplace, PGP auth
**Heritage:** Every `.core/` convention traces to a dAppServer prototype decision

---

## 1. What a CoreApp Is

A CoreApp is a directory with a `.core/view.yaml` manifest. Run `core` in that directory — it discovers the manifest, verifies its signature, and boots the application.

```
Any directory + .core/view.yaml = a CoreApp
```

That's it. No framework lock-in, no build system dependency, no server requirement. The manifest declares what the app IS. The runtime enforces boundaries. The user's data stays on their device.

### 1.1 The Contract

```
Developer creates:     .core/view.yaml (what the app needs)
Core compiles:         core.json (distribution-ready, signed)
User runs:             core (discovers manifest, verifies, boots)
App communicates via:  Named Actions (sandboxed by permissions)
Data stored at:        ~/.core/data/{workspace}/ (encrypted at rest)
Distribution via:      Git marketplace (signed manifests)
```

### 1.2 Design Principle

**Local-first.** The app runs on the user's device. Data lives on their device. The network is for federation, not centralisation. The developer builds around local storage, not remote servers.

### 1.3 Origin

```
~/Lethean/ (2021, dAppServer)           →  ~/.core/ (2026, Core)
~/Lethean/apps/                         →  ~/.core/apps/
~/Lethean/data/{lthnHash}/{lthnHash}    →  ~/.core/data/{workspace}/
$lthn->("fs.read","config.json")        →  c.Action("fs.read").Run(ctx, opts)
localStorage polyfill in WebView        →  go-store (SQLite KV, same API)
.itw3.json manifest                     →  .core/view.yaml
Deno --allow-* flags                    →  Core entitlement system
PGP key dance (no TLS needed)           →  ed25519 manifest signing
Swagger from Deno controllers           →  SDK generation from CoreCommand tree
~/Lethean/apps/ marketplace             →  Git-based signed marketplace
lthnHash/QuasiSalt file encryption      →  Sigil framework (ChaCha20 + Pre-Obfuscation)
```

Every Core convention IS a dAppServer prototype decision, formalised and made production-ready. The IP is timestamped in EUPL-1.2 licensed git repos since 2021.

---

## 2. The Manifest (.core/view.yaml)

```yaml
code: photo-browser
name: Photo Browser
version: 0.1.0
sign: <ed25519 signature>

layout: HLCRF
slots:
  H: nav-breadcrumb
  L: folder-tree
  C: photo-grid
  R: metadata-panel
  F: status-bar

permissions:
  read: ["./photos/"]
  write: ["./photos/.thumbnails/"]
  net: []
  run: []

modules:
  - core/media
  - core/fs

config:
  thumbnails:
    template: conf/thumbs.json.tmpl
    vars:
      size: "{{ .user.thumbnail_size }}"
      quality: "{{ .user.quality }}"
```

### 2.1 Fields

| Field | Required | Purpose |
|-------|----------|---------|
| `code` | Yes | Unique identifier (slug) |
| `name` | Yes | Human-readable name |
| `version` | Yes | Semantic version |
| `sign` | Yes (dist) | ed25519 signature of manifest content |
| `layout` | No | HLCRF slot layout |
| `slots` | No | Component → slot mapping |
| `permissions` | Yes | Capability declarations |
| `modules` | No | Required Core modules |
| `config` | No | Template-driven config generation |

### 2.2 Permissions

```yaml
permissions:
  read: ["./photos/", "./config/"]     # Filesystem read access
  write: ["./photos/.thumbnails/"]      # Filesystem write access
  net: ["api.example.com:443"]          # Network access
  run: ["ffmpeg"]                       # Process execution
  store: true                           # Object store access
```

If a permission isn't declared, the app CAN'T access it. The runtime enforces this — not the app, not the developer, the FRAMEWORK. Same principle as Deno's `--allow-*` flags, but declared in the manifest and verified with the signature.

---

## 3. The Compile Step

```bash
core compile
```

Reads `.core/view.yaml` → generates `core.json` at the project root.

### 3.1 What core.json Contains

```json
{
  "code": "photo-browser",
  "name": "Photo Browser",
  "version": "0.1.0",
  "sign": "<ed25519 signature>",
  "compiled_at": "2026-03-27T10:00:00Z",
  "compiled_by": "core v0.8.0",
  "layout": "HLCRF",
  "slots": { "H": "nav-breadcrumb", "C": "photo-grid" },
  "permissions": { "read": ["./photos/"] },
  "modules": ["core/media", "core/fs"],
  "components": {
    "nav-breadcrumb": { "tag": "nav-breadcrumb", "shadow": true },
    "photo-grid": { "tag": "photo-grid", "shadow": true }
  }
}
```

`core.json` is the distribution-ready artifact. It lives at the project root (not inside `.core/`). It's what the runtime reads. The manifest is the source, `core.json` is the compiled output.

### 3.2 Signing

```bash
core sign                    # Sign with default key
core sign --key ~/.core/keys/app.key  # Explicit key
```

Signs the manifest content (excluding the `sign` field itself) with ed25519. The signature is embedded in the manifest. The public key is distributed via the marketplace index.

### 3.3 Verification

On boot, Core:
1. Reads `core.json` (or `.core/view.yaml` in dev mode)
2. Extracts `sign` field
3. Verifies ed25519 signature against known public keys
4. Rejects unsigned or tampered manifests
5. Enforces declared permissions — nothing more, nothing less

---

## 4. The Runtime

```bash
core                    # Boot app in current directory
core --dev              # Dev mode (hot reload, no signature required)
core run photo-browser  # Run installed app by code
```

### 4.1 Boot Sequence

```
1. Discover    — find .core/view.yaml or core.json
2. Verify      — check ed25519 signature
3. Permissions — parse and enforce permission declarations
4. Modules     — load required modules (core/media, core/fs, etc.)
5. Layout      — compose HLCRF slots with Web Components
6. Config      — render templates from object store values
7. Start       — boot CoreGUI (desktop) or serve (web/PWA)
```

### 4.2 Dev Mode

No signature required. Hot reload on file changes. `.core/view.yaml` read directly (no compile step needed). Permission violations logged as warnings, not errors.

```bash
core --dev    # Developer mode
```

### 4.3 Communication

Apps talk to Core via Named Actions. The permission system gates which Actions the app can invoke.

```
App                              Core Runtime
 │                                │
 ├── c.Action("fs.read")  ──────►│── Permission check: read allowed? ──► Execute
 ├── c.Action("net.fetch") ─────►│── Permission check: net allowed?  ──► DENIED
 ├── c.Action("store.get") ─────►│── Permission check: store allowed? ─► Execute
 │                                │
```

The app never bypasses Core. Every capability goes through the Action system, which checks the manifest permissions, which are signed and verified.

---

## 5. Data Storage

### 5.1 Object Store (go-store)

The dAppServer localStorage polyfill, production-ready:

```
App calls:  c.Action("store.get").Run(ctx, {group: "prefs", key: "theme"})
Core reads: ~/.core/data/{app-code}/store.db (SQLite)
Returns:    "dark"
```

Same API as browser localStorage but backed by SQLite, scoped per app, encrypted at rest.

### 5.2 Filesystem Sandbox

```
App calls:  c.Action("fs.read").Run(ctx, {path: "photos/sunset.jpg"})
Core reads: {app-directory}/photos/sunset.jpg
```

Path traversal blocked. `../../../etc/passwd` → `./etc/passwd` (sandboxed to app root). Same SASE containment model as go-io — the CWD at launch becomes the immutable root boundary.

### 5.3 Encryption at Rest

Files in `~/.core/data/` encrypted via Sigil framework:
- ChaCha20-Poly1305 encryption
- Pre-Obfuscation Layer (XOR/shuffle before encryption — side-channel defence)
- SHA-256 workspace IDs (file names reveal nothing)
- Decryption requires workspace key (derived from user password)

From dAppServer: `~/Lethean/data/{lthnHash/QuasiSalt}/{lthnHash}` → you couldn't even see what the filenames meant until you decrypted the enclave.

---

## 6. The Marketplace

Git-based. No server infrastructure. Manifests are signed.

### 6.1 Registry

```
marketplace/
├── index.json           # {version, modules[], categories[]}
├── media/
│   └── index.json       # Category: media apps
├── tools/
│   └── index.json       # Category: developer tools
└── network/
    └── index.json       # Category: network services
```

### 6.2 Install

```bash
core marketplace search "photo"          # Search
core marketplace install photo-browser   # Install
core marketplace update photo-browser    # Update (git pull + re-verify signature)
core marketplace remove photo-browser    # Remove
core marketplace installed               # List installed
```

Install = git clone (depth=1) → verify signature → register in object store → available at next boot.

For external app packaging (PWAs, Electron apps, vendor apps), see §16. The `core pkg` command extends the marketplace with wrapping capabilities for non-native app formats.

### 6.3 Updates

`git pull` on the app repo. Signature re-verified after pull. If signature fails, rollback to previous commit. The user never runs untrusted code.

---

## 7. Distribution Targets

One app, every platform:

| Target | How | Runtime |
|--------|-----|---------|
| Desktop (macOS/Win/Linux) | CoreGUI (Wails v3) | Native binary |
| iOS | CoreGUI (Wails v3 alpha 74) | Native app |
| Android | CoreGUI (Wails v3 alpha 74) | Native app |
| Web (PWA) | Service worker + Web Components | Browser |
| CLI | `core` binary (headless, no GUI) | Terminal |
| Edge | Deno Deploy or self-hosted | Server |

The manifest is the same for all targets. The runtime adapts.

### 7.1 PWA Specifics

- Service worker caches `core.json` + component bundles
- Object store mirrors to IndexedDB for offline
- Sync on reconnect (last-write-wins or CRDT per app choice)
- Install prompt (Add to Home Screen)
- The PWA talks to `localhost` (Core running on device) or the user's own server

---

## 8. SDK Generation

Developer writes one app. The SDK is generated automatically.

```
.core/view.yaml (manifest)
  → core compile → core.json
    → core sdk generate
      → TypeScript bindings (for frontend)
      → OpenAPI spec (for external consumers)
      → Client SDKs (TS, Python, Go, PHP)
```

From dAppServer: Swagger defined from controllers → `@lethean/api-angular` SDK generated → frontend imported typed bindings. Same pipeline, now automatic from the manifest.

---

## 9. The DTO Contract

DTOs are defined per-package as Go structs (see `code/core/go/RFC.md` Design Philosophy — "DTO Pattern: Structs Not Props"). CoreApp does not define a separate DTO layer — the Go handler structs ARE the contracts. The SDK pipeline generates CoreTS and CorePHP bindings from them.

What "random joe in core/ide" needs to know:

### 9.1 To Build an App

```yaml
# 1. Create .core/view.yaml
code: my-app
name: My App
version: 0.1.0
permissions:
  read: ["./data/"]
  store: true

# 2. Build
# core compile && core sign

# 3. Run
# core --dev (development)
# core (production, requires signature)
```

### 9.2 To Talk to Core

```typescript
// TypeScript (CoreTS / Wails bindings)
const content = await Core.Action("fs.read", { path: "data/config.json" });
const prefs = await Core.Action("store.get", { group: "prefs", key: "theme" });
await Core.Action("store.set", { group: "prefs", key: "theme", value: "dark" });
```

```go
// Go (direct)
r := c.Action("fs.read").Run(ctx, core.NewOptions(
    core.Option{Key: "path", Value: "data/config.json"},
))
```

```php
// PHP (Laravel)
$content = app('core')->action('fs.read', ['path' => 'data/config.json']);
```

Same contract. Three languages. One manifest.

### 9.3 Available Actions (Core Primitives)

| Action | Permission | Purpose |
|--------|-----------|---------|
| `fs.read` | `read` | Read file from sandbox |
| `fs.write` | `write` | Write file in sandbox |
| `fs.list` | `read` | List directory |
| `fs.delete` | `write` | Delete file |
| `store.get` | `store` | Read from object store |
| `store.set` | `store` | Write to object store |
| `store.delete` | `store` | Delete from object store |
| `net.fetch` | `net` | HTTP request |
| `net.ws` | `net` | WebSocket connection |
| `process.run` | `run` | Execute external command |
| `gui.window.create` | — | Create window (desktop only) |
| `gui.dialog.confirm` | — | Show confirmation dialog |
| `gui.notification.send` | — | System notification |
| `gui.clipboard.write` | — | Write to clipboard |
| `i18n.translate` | — | Translate string |
| `brain.recall` | `net` | Search OpenBrain |

### 9.4 Complete RPC Surface (from dAppServer)

Full procedure list extracted from dAppServer test files and controllers. These map to CoreApp Named Actions:

#### Auth
```
auth.create(username, password)         → PGP key gen + QuasiSalt hash
auth.login(username, encryptedPayload)  → ZK PGP verify → JWT
auth.delete(username)                   → Remove account
```

#### Crypto
```
crypto.pgp.generateKeyPair(name, email, passphrase) → {pub, priv}
crypto.pgp.encrypt(data, publicKey)                  → encrypted
crypto.pgp.decrypt(data, privateKey, passphrase)     → plaintext
crypto.pgp.sign(data, privateKey, passphrase)        → signature
crypto.pgp.verify(data, signature, publicKey)        → boolean
```

#### Process
```
process.run(command, args, options) → ProcessHandle
process.add(request)               → key
process.start(key)                 → boolean
process.stop(key)                  → boolean
process.kill(key)                  → boolean
process.list()                     → string[]
process.get(key)                   → ProcessInfo
process.stdout.subscribe(key)      → stream
process.stdin.write(key, data)     → void
```

#### IPC / Event Bus
```
ipc.pub.subscribe(channel)         → stream
ipc.pub.publish(channel, message)  → void
ipc.req.send(channel, message)     → response
ipc.push.send(message)             → void
```

---

## 10. Security Model

### 10.1 Layers

```
1. Manifest signature (ed25519) — app identity verified
2. Permission declarations — capabilities explicitly declared
3. Runtime enforcement — Core blocks undeclared access
4. Sandbox boundary — CWD = immutable root (SASE containment)
5. Encrypted storage — data at rest encrypted via Sigil
6. No TLS needed — PGP key dance for local auth (dAppServer heritage)
```

### 10.2 What Can't Happen

- App can't read files outside its declared paths
- App can't access network if `net` not declared
- App can't run processes if `run` not declared
- Tampered manifest fails signature check → app won't boot
- Path traversal blocked at io.Medium level (`../` → `./`)
- Unsigned apps rejected in production mode (dev mode logs warnings)

### 10.3 What the User Controls

- Which apps to install (marketplace)
- Which apps to trust (signature verification)
- Which workspace to use (data isolation)
- When to sync (federation is opt-in)
- What to share (UEPS consent model from LetherNet)

---

## 11. Plugin & Services Contract

A CoreApp host (CoreGUI, core-agent, or any `core.New()` container) can run multiple plugins simultaneously. Each plugin is an isolated `core.New()` instance with its own manifest, services, and permission boundary.

### 11.1 Service Registration

The manifest `services` field declares which Core services the plugin requires:

```yaml
# .core/view.yaml
name: markdown-editor
services:
  - io        # sandboxed filesystem
  - i18n      # translations
  - crypt     # encryption, signing
  - store     # key-value storage
  - api       # REST client
permissions:
  read: ["./docs/"]
  write: ["./docs/", "./cache/"]
  net: ["api.github.com:443"]
```

The host provides sandboxed instances scoped to the plugin's permissions:

```go
// Host (CoreGUI) boots each plugin from its manifest
for _, manifest := range discoveredApps {
    plugin := core.New(
        core.WithManifest(manifest),    // reads services + permissions
        core.WithServiceLock(),          // plugin cannot register more
    )
    go plugin.Run()
}
```

`WithServiceLock()` prevents the plugin from registering services beyond what the manifest declares. The plugin gets exactly what it asked for — nothing more.

### 11.2 Plugin Communication

Plugins communicate through Named Actions. A plugin never calls another plugin directly — it dispatches an action that Core routes.

```
Plugin A                    Core                     Plugin B
   │                         │                          │
   ├─ Action("editor.save") ─┤                          │
   │                         ├─ permission check ──────►│
   │                         │  (A has write access?)    │
   │                         ├─ route to handler ───────►│
   │                         │                          ├─ handles save
   │                         │◄─── Result ──────────────┤
   │◄──── Result ────────────┤                          │
```

Actions are the ONLY inter-plugin boundary. If a plugin doesn't register an action handler, that action doesn't exist for it. The host can't be bypassed.

### 11.3 The API Contract (OpenAPI Generation)

Each plugin's registered Actions are the source of truth for its API surface. The SDK generation pipeline produces typed bindings:

```
Plugin registers Actions
  → CoreCommand tree built from action names + Options schemas
    → OpenAPI spec generated (JSON Schema for each action)
      → CoreTS generates TypeScript bindings
        → Frontend consumes typed functions
```

```typescript
// Generated: @types/markdown-editor
export interface EditorActions {
    'editor.open': (opts: { path: string }) => Promise<{ content: string }>;
    'editor.save': (opts: { path: string; content: string }) => Promise<void>;
    'editor.preview': (opts: { content: string }) => Promise<{ html: string }>;
}
```

The transport is invisible — Wails coroutines (desktop), WebSocket (browser), HTTP (remote). The contract is the same OpenAPI schema regardless.

### 11.4 Conclave Isolation (IDE Build Tools)

Build tools in core/ide run as conclaves — maximally isolated plugin instances:

```yaml
# .core/view.yaml for a linter conclave
name: phpstan
services: [io, process]
permissions:
  read: ["./"]           # read project files
  run: ["phpstan"]       # execute one binary
  write: []              # no write access
  net: []                # no network
```

Each conclave is a `core.New()` with `WithServiceLock()`. The IDE orchestrates them but can't see inside. Output flows through Actions (`lint.results`, `test.results`). The tool binary runs in a sandboxed `process.run` — it can read the project but can't write to it, can't access the network, can't spawn other processes.

### 11.5 Service Lifecycle in Plugins

Services within a plugin follow the same lifecycle as top-level Core services:

| Interface | When | Purpose |
|-----------|------|---------|
| `Startable` | Plugin boot | Initialise resources (open DB, connect) |
| `Stoppable` | Plugin shutdown | Clean up resources (close DB, flush) |
| `HandleIPCEvents` | Runtime | Respond to Actions from other plugins/host |

The host manages plugin lifecycle — start on demand, stop on idle, restart on crash. The plugin doesn't manage its own lifetime.

---

## 12. Connection to Everything

| RFC | Role in CoreApp |
|-----|----------------|
| **core/config** | `.core/` convention, config.yaml loading |
| **core/gui** | Desktop runtime (Wails v3, windows, tray, dialogs) |
| **core/go/ipc** | Named Actions — the communication contract |
| **core/go/build** | `core compile`, `core sign`, distribution |
| **core/go/cli** | `core` command, semantic output |
| **core/ts** | CoreDeno sidecar, Web Components, browser runtime |
| **core/i18n** | Translation across all app languages |
| **go-io** | Filesystem sandbox (SASE containment) |
| **go-store** | Object store (SQLite KV, localStorage replacement) |
| **go-crypt** | Manifest signing (ed25519), encryption |
| **go-html** | HLCRF layout, Web Component codegen |
| **go-process** | Sandboxed process execution |
| **dAppServer** | The vision — this RFC implements it |
| **CoreGUI** | The framework — this RFC defines what it runs |
| **LetherNet** | The network — CoreApps run on nodes |
| **LEM** | Ethics — models run in protected CoreApps (TIM) |
| **Marketplace** | Distribution — signed manifests in Git repos |
| **Commerce Matrix** | Multi-tenant app distribution (M1/M2/M3) |
| **EaaS** | Content scoring as a CoreApp service |
| **Host SaaS** | Products built AS CoreApps |

---

## 13. Implementation Priority

| Feature | Description |
|---------|-------------|
| `.core/view.yaml` parser | Manifest loading and validation |
| `core.json` compiler | `core compile` command — manifest to distribution artifact |
| ed25519 signing | `core sign` command — manifest content signing |
| Signature verification on boot | Reject unsigned or tampered manifests |
| Permission enforcement | Gate Actions by manifest-declared permissions |
| Filesystem sandbox | go-io SASE containment — CWD as immutable root |
| Object store integration | go-store SQLite KV for app data |
| Config template rendering | Template-driven config generation from object store |
| Encrypted workspace storage | Sigil framework encryption at rest |
| HLCRF slot composition | go-html codegen for Web Component layout |
| CoreGUI window management | Desktop window lifecycle via Wails v3 |
| Web Component registration | Custom element discovery and mounting |
| Dev mode | Hot reload, no signature requirement, warning-only permissions |
| Git marketplace | Install, update, remove apps from Git repos |
| Marketplace index format | Category-as-directory JSON index |
| SDK generation from manifest | Auto-generate typed client SDKs |
| `dAppCore/build` action integration | Build pipeline hookup |
| PWA service worker | Offline-capable web distribution |
| iOS/Android builds | Wails v3 alpha 74 mobile targets |
| CLI headless mode | Terminal-only operation without GUI |
| Edge deployment | Deno Deploy or self-hosted server target |
| `core pkg wrap --pwa` | PWA manifest.json → .core/view.yaml conversion |
| PWA permission mapping | Map PWA permissions to CoreApp permissions |
| PWA service worker replacement | Replace service worker with Core background services |
| `core pkg wrap --electron` | Download Electron renderer assets, inject shim, create manifest |
| Electron permission auto-detection | Scan Electron JS for API usage patterns |
| Electron shim injection | Insert CoreTS Electron compat layer (ts RFC §13) |
| `core pkg wrap --web` | Wrap local web directory as CoreApp |
| `core pkg install` type detection | Auto-detect native, PWA, or Electron from source |
| `core pkg list/remove/update` | Package lifecycle management |

### Deliverables (file locations for Codex)

| Component | Location | Language |
|-----------|----------|----------|
| `.core/view.yaml` loader | `core/pkg/manifest/` | Go |
| Manifest signing/verify | `core/pkg/manifest/sign.go` | Go |
| CoreDeno sidecar manager | `core/pkg/coredeno/` | Go |
| gRPC proto definitions | `core/pkg/coredeno/proto/` | Protobuf |
| gRPC server (Go side) | `core/pkg/coredeno/server.go` | Go |
| Deno client runtime | `core-deno/` (new repo) | TypeScript |
| HLCRF→WC codegen | `go-html/codegen/` | Go (exists) |
| WASM WC registration | `go-html/cmd/wasm/` | Go (exists) |
| Object store | `go-store/` | Go (exists) |
| Marketplace CLI | `core/cli/` | Go |

---

## 14. Prior Art (Timestamped, EUPL-1.2)

| Date | Repository | What It Proves |
|------|-----------|---------------|
| 2021 | `dAppServer/server` | Object store API, PGP auth, process management |
| 2021 | `dAppServer/dappui` | Angular↔Deno bridge, localStorage polyfill |
| 2021 | `dAppServer/mod-auth` | Zero-knowledge PGP auth, QuasiSalt |
| 2021 | `dAppServer/mod-io-process` | Process registry, 3-layer I/O streaming |
| 2021 | `dAppServer/app-marketplace` | Git-as-database, manifest-driven install |
| 2021 | `dAppServer/app-mining` | CLI Bridge, Process-as-Service |
| 2021 | `dAppServer/wails-build-action` | Cross-platform desktop builds |
| 2022 | Borg/Enchantrix/Poindexter | Encrypted storage, sigil crypto, pointer maps |
| 2024 | `core/go` + 26 packages | Framework extraction from monorepo |
| 2025 | `core/gui` | CoreGUI (Wails v3, 17 packages) |
| 2026 | This RFC | Unification of 5 years of design |

All EUPL-1.2. All timestamped in git. All publicly available (except core/gui — the ace). The ideas were implemented before AI agents existed. The agents helped express them as a framework.

### 13.1 Extraction Tiers

#### Tier 1: Extract (Core Architecture)

| Repo | Extract | Target |
|------|---------|--------|
| `server` | Port 36911 bridge, ZeroMQ IPC, air-gapped PGP auth, object store | CoreDeno sidecar, I/O fortress |
| `dappui` | Angular→WC migration, REST+WS+Wails triple, terminal (xterm.js) | Web Component framework |
| `mod-auth` | PGP zero-knowledge auth, QuasiSalt, roles | Signed manifest, identity |
| `mod-io-process` | Process registry, 3-layer I/O streaming | `c.Process()`, event bus |
| `app-marketplace` | Git-as-database, category-as-directory, install pipeline | Module registry |

#### Tier 2: Port (Useful Patterns)

| Repo | Port | Target |
|------|------|--------|
| `auth-server` | Keycloak + PGP fallback | External auth option |
| `mod-docker` | Docker socket client (8 ops) | `c.Process()` |
| `app-mining` | CLI Bridge, Process-as-Service | Generic CLI wrapper |
| `app-directory-browser` | Split-pane layout, lazy tree | `<core-file-tree>` WC |

#### Tier 3: Reference Only

`depends` (Bitcoin Core hermetic build), `app-utils-cyberchef` (manifest-only app), `pwa-native-action` (PWA→Wails proof).

---

## 15. Reference Material

| Resource | Location |
|----------|----------|
| .core/ convention | `code/core/config/RFC.md` |
| CoreGUI desktop | `code/core/gui/RFC.md` |
| IPC / Named Actions | `code/core/go/ipc/RFC.md` |
| Build + signing | `code/core/go/build/RFC.md` |
| CLI framework | `code/core/go/cli/RFC.md` |
| CoreTS sidecar | `code/core/ts/RFC.md` |
| I/O sandbox | `code/core/go/io/RFC.md` |
| HLCRF layout | `code/core/go/html/RFC.md` |
| Marketplace | `code/core/go/scm/RFC.md` |
| ML RFC (scoring, trust) | `code/core/go/ml/RFC.md` |

---

## 16. External App Packaging

The marketplace (§6) handles Core-native apps with `.core/view.yaml` manifests. External apps — PWAs and Electron apps — need wrapping into the CoreApp format. The `core pkg` command handles both patterns, producing a standard `.core/view.yaml` manifest that the runtime treats identically to native CoreApps.

### 16.1 PWA Packaging

`core pkg wrap --pwa` reads a PWA's `manifest.json` and generates a `.core/view.yaml` manifest. The PWA runs inside CoreGUI's WebView with full storage polyfill support (ts RFC §5) and background service mapping (gui RFC §12.4).

```bash
# Wrap a PWA as a CoreApp
#
#   core pkg wrap --pwa https://app.example.com
#   → fetches manifest.json from the URL
#   → generates .core/view.yaml with mapped permissions
#   → installs to ~/.core/apps/app.example.com/
core pkg wrap --pwa https://app.example.com

# Install a curated PWA from the marketplace
#
#   core pkg install core/play
#   → resolves 'core/play' from marketplace index
#   → wraps the PWA manifest into .core/view.yaml
core pkg install core/play
```

#### manifest.json → view.yaml Mapping

| PWA Field | CoreApp Field | Notes |
|-----------|--------------|-------|
| `name` | `name` | Human-readable name |
| `short_name` | `code` | Slug identifier |
| `start_url` | `url` | Entry point URL |
| `display` | `layout` | `standalone` → window, `fullscreen` → kiosk |
| `icons` | `icon` | Largest icon selected |
| `theme_color` | `theme.primary` | Primary colour |
| `background_color` | `theme.background` | Background colour |
| `lang` | `locale` | Default locale |

#### Permission Mapping

| PWA Permission | CoreApp Permission | Action |
|---------------|-------------------|--------|
| `notifications` | `gui.notification.send` | System notifications |
| `clipboard-read` | `gui.clipboard.read` | Read clipboard |
| `clipboard-write` | `gui.clipboard.write` | Write clipboard |
| `geolocation` | `device.location` | Location access |
| `camera` | `device.camera` | Camera access |
| `microphone` | `device.microphone` | Microphone access |
| `storage` | `store` | Persistent storage |

#### Service Worker Replacement

PWA service workers are replaced by Core background services. The service worker's `fetch` handler is replaced by Core's HTTP client. The `push` handler is replaced by core-notify. Cache strategies are handled by go-store's Cache Storage polyfill (ts RFC §5.5).

```yaml
# Auto-generated .core/view.yaml for a wrapped PWA
#
#   core pkg wrap --pwa https://play.example.com
#   → produces this manifest
code: play-example
name: Play Example
version: 0.1.0
url: https://play.example.com
type: pwa

permissions:
  net: ["play.example.com:443"]
  store: true

services:
  - store          # localStorage, IndexedDB polyfills
  - notification   # push notification replacement

theme:
  primary: "#6200ea"
  background: "#ffffff"
```

### 16.2 Vendor App Packaging (Electron)

`core pkg wrap --electron` downloads an Electron app's web/renderer assets from its GitHub releases, injects the CoreTS Electron compatibility shim (ts RFC §13), and creates a `.core/view.yaml` with auto-detected permissions.

```bash
# Wrap an Electron app as a CoreApp
#
#   core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner
#   → downloads latest release assets (HTML/CSS/JS — NOT the Electron binary)
#   → scans main.js for Electron API usage patterns
#   → generates .core/view.yaml with detected permissions
#   → injects Electron compatibility shim
#   → signs with local key
core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner

# Install a curated Electron app from the marketplace
#
#   core pkg install bitwarden/clients
#   → resolves from marketplace, downloads renderer assets
#   → applies Electron shim, creates manifest
core pkg install bitwarden/clients
```

#### What Gets Downloaded

Only the web/renderer assets are downloaded — HTML, CSS, JavaScript, images, fonts. The Electron binary, Node.js runtime, and native modules are NOT downloaded. CoreGUI replaces Electron as the host process; the CoreTS Electron shim (ts RFC §13) bridges the API gap.

#### Permission Auto-Detection

The wrapper scans the Electron app's JavaScript for API usage patterns and maps them to CoreApp permissions:

| Electron Pattern | Detected Permission | CoreApp Permission |
|-----------------|--------------------|--------------------|
| `require('fs')` | Filesystem access | `read: ["./data/"]`, `write: ["./data/"]` |
| `require('net')` | Network access | `net: ["*"]` |
| `clipboard.readText()` | Clipboard read | `gui.clipboard.read` |
| `clipboard.writeText()` | Clipboard write | `gui.clipboard.write` |
| `dialog.showOpenDialog()` | File dialog | `gui.dialog.open` |
| `dialog.showSaveDialog()` | Save dialog | `gui.dialog.save` |
| `shell.openExternal()` | Browser open | `gui.browser.open` |
| `Notification` | Notifications | `gui.notification.send` |
| `ipcRenderer.send` | IPC channels | Listed in `ipc_channels` |

```yaml
# Auto-generated .core/view.yaml for a wrapped Electron app
#
#   core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner
#   → produces this manifest (permissions auto-detected from source)
code: nicehash-quickminer
name: NiceHash QuickMiner
version: 4.1.0
type: electron

permissions:
  read: ["./data/"]
  write: ["./data/"]
  net: ["api.nicehash.com:443", "nicepool.nicehash.com:3333"]
  gui.clipboard.read: true
  gui.notification.send: true

shim:
  electron: true      # enables Electron compatibility layer
  fs_proxy: true       # enables go-io filesystem proxy for require('fs')

ipc_channels:
  - "app:ready"
  - "miner:start"
  - "miner:stop"
  - "hashrate:update"
```

#### Signing

Wrapped apps are signed with the user's local key for trusted execution. The signature covers the manifest content and the renderer assets hash. Re-wrapping after an upstream update re-signs with the same key.

```bash
# Wrap and sign in one step
#
#   core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner --sign
#   → downloads, wraps, signs with default key (~/.core/keys/default.key)
core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner --sign
```

### 16.3 `core pkg` Command

The unified package command handles native CoreApps, PWAs, and Electron apps through a single interface.

```bash
# Install from marketplace or GitHub
#
#   core pkg install {vendor}/{name}
#   → resolves package type (native, pwa, electron) from marketplace index
#   → downloads, wraps if needed, installs to ~/.core/apps/
core pkg install bitwarden/clients          # Electron app from GitHub
core pkg install core/play                   # PWA from marketplace
core pkg install core/photo-browser          # Native CoreApp from marketplace

# Wrap external apps manually
#
#   core pkg wrap --pwa {url}
#   core pkg wrap --electron {repo}
#   core pkg wrap --web {dir}
core pkg wrap --pwa https://app.example.com
core pkg wrap --electron github.com/nicehash/NiceHashQuickMiner
core pkg wrap --web ./my-webapp              # Wrap a local web directory as CoreApp

# List installed packages
#
#   core pkg list
#   → shows all installed apps with type, version, source
#   NAME                  TYPE       VERSION   SOURCE
#   photo-browser         native     0.1.0     marketplace
#   bitwarden-clients     electron   2024.8.1  github.com/bitwarden/clients
#   play-example          pwa        0.1.0     https://play.example.com
core pkg list

# Remove a package
#
#   core pkg remove {name}
#   → removes app directory, cleans up ~/.core/apps/{name}/
core pkg remove bitwarden-clients

# Update a package
#
#   core pkg update {name}
#   → pulls latest version, re-wraps if needed, re-signs
core pkg update bitwarden-clients
```

### 16.4 Package Type Detection

When `core pkg install` resolves a package, it detects the type automatically:

| Source | Detection | Type |
|--------|----------|------|
| Marketplace index | `type` field in index.json | native, pwa, or electron |
| GitHub repo | Scan for `package.json` with `electron` dependency | electron |
| GitHub repo | Scan for `manifest.json` with `start_url` | pwa |
| Local directory | Scan for `.core/view.yaml` | native |
| Local directory | Scan for `manifest.json` | pwa |
| URL | Fetch and inspect response headers + content | pwa |

---

## Changelog

- 2026-04-08: §16 — External App Packaging. PWA wrapping (manifest.json → view.yaml, permission mapping, service worker replacement). Electron vendor app wrapping (renderer asset download, permission auto-detection, shim injection, signing). `core pkg` unified command (install, wrap, list, remove, update). Package type auto-detection
- 2026-03-27: The keystone RFC. Every other RFC exists to make this work. Traces every convention to its dAppServer origin. Defines the CoreApp contract: manifest, compile, sign, runtime, sandbox, storage, marketplace, SDK, security.
