# core/app/

CoreApp — the keystone spec. Every other RFC exists to make this work.

- [RFC.md](RFC.md) — Manifest (.core/view.yaml), compile (core.json), sign (ed25519), runtime, sandbox, marketplace, SDK gen, security model

## Cross-References

| Spec | Relationship |
|------|-------------|
| `code/core/config/RFC.md` | .core/ convention (view.yaml is a config file type) |
| `code/core/gui/RFC.md` | Desktop runtime (Wails v3 — runs CoreApps) |
| `code/core/ts/RFC.md` | CoreDeno sidecar (module sandbox, dev server) |
| `code/core/go/html/RFC.md` | HLCRF layout composition + Web Components |
| `code/core/go/io/RFC.md` | Filesystem sandbox (SASE containment) |
| `code/core/go/store/RFC.md` | Object store (localStorage replacement) |
| `code/core/go/crypt/RFC.md` | Manifest signing (ed25519) |
| `code/core/go/build/RFC.md` | `core compile`, `core sign`, distribution |
| `code/core/go/scm/RFC.md` | Git-based marketplace |
| `code/core/i18n/RFC.md` | Translation across app languages |
| `project/lthn/lethernet/RFC.md` | LetherNet L5 — CoreApps run on nodes |
| `project/lthn/RFC.md` | Lethean CIC — asset-locked community benefit |
