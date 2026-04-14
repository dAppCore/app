# core/app

The CoreApp keystone runtime — reads a `.core/view.yaml` manifest, runs the 7-step boot, enforces permissions, composes layout, starts the app.

> "The keystone spec. Every other RFC exists to make this work."

**Module:** `dappco.re/go/app`
**Spec:** [`docs/RFC.md`](docs/RFC.md)

## The 7-step boot

```
Discover → Verify → Permissions → Modules → Layout → Config → Start
```

Every CoreApp (CorePlay, lthn.ai, OFM agency, BugSETI) runs through this sequence.

## Dependencies

```
core/app → go-scm (manifest + compile + sign + verify)
        → core/config (.core/view.yaml parsing, XDG, features)
        → core/gui (window boot)
        → go-io (SASE sandbox + Sigil encryption)
        → go-store (permissioned key-value)
        → core/go (primitives)
```

## Scope

- IN: manifest loader, boot orchestrator, permission→action gate wiring, manifest-driven window boot, `core run <app-code>`, workspace lifecycle
- OUT: window-management primitives (core/gui owns), manifest parsing (go-scm owns), model inference (go-mlx, go-inference)

## Status

Bootstrapped repo — spec in `docs/`. Implementation pending. This is the glue between all the primitives that currently live as unconnected libraries.

## Licence

EUPL-1.2
