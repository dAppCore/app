// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"time"

	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreio "dappco.re/go/core/io"
)

// DefaultWatchInterval is the poll cadence the dev-mode watcher uses
// when WatchOptions.Interval is zero. 500ms matches a developer's
// edit-save-iterate rhythm without burning CPU in the background.
//
//	interval := app.DefaultWatchInterval // 500 * time.Millisecond
const DefaultWatchInterval = 500 * time.Millisecond

// WatchOptions tunes Watch. Zero values produce the dev-mode default —
// poll `.core/view.yaml` every DefaultWatchInterval and broadcast
// ActionManifestChanged on every content change. Callers can shorten
// the interval for a responsive editor integration or extend it for a
// CI-style sanity check.
//
//	opts := app.WatchOptions{
//	    Interval: 100 * time.Millisecond,
//	    Paths:    []string{".core/view.yaml", "conf/thumbs.json.tmpl"},
//	}
type WatchOptions struct {
	// Interval is the poll cadence. Zero → DefaultWatchInterval.
	Interval time.Duration
	// Paths is the optional project-relative watch list. When empty
	// the watcher polls `.core/view.yaml` alone — the canonical trigger
	// RFC §4.2 names. Each entry is resolved relative to inst.Root so
	// template sources (`conf/*.tmpl`) can be watched alongside the
	// manifest without the caller knowing the absolute layout.
	Paths []string
	// Medium overrides inst.medium. Defaults to the medium the boot
	// used so MockMedium / MemoryMedium tests round-trip cleanly.
	Medium coreio.Medium
}

// ActionManifestChanged is the IPC broadcast the dev-mode watcher fires
// whenever a watched file's content hash changes. Hosts (core/gui,
// core-agent, the CLI dev loop) subscribe via c.RegisterAction and drive
// whatever reload strategy they prefer — restart the entire boot, reload
// the affected slot, re-render templates, …
//
//	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
//	    if evt, ok := msg.(app.ActionManifestChanged); ok {
//	        core.Info("manifest change", "path", evt.Path)
//	    }
//	    return core.Result{OK: true}
//	})
//
// Rules:
//
//   - Path is the absolute file path that changed (`.core/view.yaml`
//     or any entry from WatchOptions.Paths resolved against inst.Root).
//
//   - Kind is "created", "modified" or "deleted" so subscribers can
//     pick the right branch without re-stat-ing the file.
//
//   - Code and Version are snapshotted from inst.Manifest at dispatch
//     time so subscribers can tie the event back to the boot that
//     emitted it.
//
//   - ObservedAt is the timestamp the watcher detected the change.
//     Keeps log traces ordered even when many changes land in one poll
//     cycle.
type ActionManifestChanged struct {
	Code       string    // manifest.code at boot time
	Version    string    // manifest.version at boot time
	Path       string    // absolute file path that changed
	Kind       string    // "created" | "modified" | "deleted"
	ObservedAt time.Time // when the watcher detected the change
}

// Watch is Step 4.2 of the RFC — the dev-mode hot-reload primitive.
// It starts a goroutine that polls the watch list on Interval and
// broadcasts ActionManifestChanged whenever a file's content hash
// changes. Returns a cancel function the caller invokes to stop the
// watcher (typically deferred alongside the boot).
//
//	stop := inst.Watch(ctx, app.WatchOptions{})
//	defer stop()
//
// Rules:
//
//   - ModeProd instances refuse to watch — the signed manifest is the
//     trust boundary, and hot-swapping it would let an attacker
//     mutate the app's permissions mid-flight. Callers get a no-op
//     cancel + a typed log warning so a misconfigured prod deployment
//     doesn't silently spin a goroutine.
//
//   - The returned cancel is idempotent; calling it twice is safe.
//
//   - The watcher honours ctx — cancellation stops the poll loop
//     immediately. The returned cancel is a convenience so callers
//     that already hold a parent ctx don't need to wire a second one.
//
//   - File absence is itself a state — the watcher fires a "deleted"
//     event on the first cycle after a previously-present file
//     disappears, then stays quiet until it re-appears.
//
//   - The watcher never touches inst.Manifest — reload strategy is
//     the subscriber's concern. That keeps the RFC §4.2 rule ("hot
//     reload on file changes") a primitive the framework publishes
//     rather than a policy the framework enforces.
func (inst *Instance) Watch(ctx context.Context, opts WatchOptions) (cancel func()) {
	if inst == nil || inst.Core == nil {
		return func() {}
	}
	if inst.Mode != ModeDev {
		core.Warn("app.Watch refused in prod mode — dev-only primitive",
			"code", inst.Manifest.Code)
		return func() {}
	}

	medium := opts.Medium
	if medium == nil {
		medium = inst.medium
	}
	if medium == nil {
		medium = coreio.Local
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultWatchInterval
	}

	paths := resolveWatchPaths(inst, opts.Paths)
	if len(paths) == 0 {
		core.Warn("app.Watch has no resolvable paths — nothing to watch",
			"code", inst.Manifest.Code, "root", inst.Root)
		return func() {}
	}

	watchCtx, stop := context.WithCancel(ctx)
	go runWatcher(watchCtx, inst, medium, paths, interval)
	return stop
}

// runWatcher is the poll loop that Watch spawns. Extracted so the
// goroutine body is a named function — easier to trace in a panic
// stack and testable via a direct call when the ticker is synthesised.
//
//	go runWatcher(ctx, inst, medium, paths, interval)
func runWatcher(ctx context.Context, inst *Instance, medium coreio.Medium, paths []string, interval time.Duration) {
	// First pass establishes the baseline — every present file is
	// recorded so the second pass only broadcasts genuine changes. The
	// watcher does NOT fire a "created" event for files that existed at
	// Watch() time; that would flood subscribers on boot.
	state := snapshotWatched(medium, paths)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next := snapshotWatched(medium, paths)
			diffAndBroadcast(inst, state, next)
			state = next
		}
	}
}

// watchEntry captures the ModTime + Size of a watched file so the poll
// loop can detect content changes without reading the file body on
// every cycle. A missing file is represented by a zero-value entry so
// the diff logic can treat "gone" and "never there" the same way.
//
//	state := map[string]watchEntry{".core/view.yaml": {ModTime: t, Size: 120}}
type watchEntry struct {
	Exists  bool      // false → the file was absent at the snapshot
	ModTime time.Time // from medium.Stat().ModTime()
	Size    int64     // from medium.Stat().Size() — cheap content hint
}

// snapshotWatched collects the current state of every watched path.
// Mediums that don't support Stat fall back to Exists + Read-length so
// MockMedium / MemoryMedium tests can still detect changes via the
// body byte count.
//
//	state := snapshotWatched(medium, []string{"/abs/.core/view.yaml"})
func snapshotWatched(medium coreio.Medium, paths []string) map[string]watchEntry {
	out := make(map[string]watchEntry, len(paths))
	for _, p := range paths {
		out[p] = statWatched(medium, p)
	}
	return out
}

// statWatched captures a single file's watch entry. Pulled out so the
// snapshot loop doesn't repeat the Exists / Stat / Read fallback logic.
//
//	entry := statWatched(medium, ".core/view.yaml")
func statWatched(medium coreio.Medium, path string) watchEntry {
	if !medium.Exists(path) {
		return watchEntry{}
	}
	info, err := medium.Stat(path)
	if err == nil && info != nil {
		return watchEntry{Exists: true, ModTime: info.ModTime(), Size: info.Size()}
	}
	// Medium without a real Stat (MockMedium) → fall back to a body
	// length read so content edits still register as a diff. Failure
	// here is absorbed: the file is present (Exists returned true) but
	// we cannot size it; record the Exists bit alone.
	body, readErr := medium.Read(path)
	if readErr != nil {
		return watchEntry{Exists: true}
	}
	return watchEntry{Exists: true, Size: int64(len(body))}
}

// diffAndBroadcast compares two snapshots and broadcasts
// ActionManifestChanged for every differing path. Emits "created",
// "modified" or "deleted" depending on the transition so subscribers
// can branch without re-inspecting the filesystem.
//
//	diffAndBroadcast(inst, prev, next)
func diffAndBroadcast(inst *Instance, prev, next map[string]watchEntry) {
	now := time.Now()
	for path, cur := range next {
		old := prev[path]
		kind := classifyChange(old, cur)
		if kind == "" {
			continue
		}
		inst.Core.ACTION(ActionManifestChanged{
			Code:       inst.Manifest.Code,
			Version:    inst.Manifest.Version,
			Path:       path,
			Kind:       kind,
			ObservedAt: now,
		})
	}
	// Paths that disappeared between snapshots (no longer enumerated)
	// — the resolveWatchPaths input is stable across cycles, so this
	// branch is defensive. If the caller edits opts.Paths between
	// Watch() calls they'll pay the deletion notice on the first cycle
	// after the new resolution.
	for path, old := range prev {
		if _, still := next[path]; still {
			continue
		}
		if !old.Exists {
			continue
		}
		inst.Core.ACTION(ActionManifestChanged{
			Code:       inst.Manifest.Code,
			Version:    inst.Manifest.Version,
			Path:       path,
			Kind:       "deleted",
			ObservedAt: now,
		})
	}
}

// classifyChange names the transition between two watch entries.
//
//	classifyChange(old, cur) // "created" | "modified" | "deleted" | ""
func classifyChange(old, cur watchEntry) string {
	switch {
	case !old.Exists && cur.Exists:
		return "created"
	case old.Exists && !cur.Exists:
		return "deleted"
	case !old.Exists && !cur.Exists:
		return ""
	}
	// Both snapshots saw the file — compare content signatures. ModTime
	// changes win even when Size stays the same because most editors
	// rewrite the file on save, bumping ModTime without touching size.
	if !old.ModTime.Equal(cur.ModTime) {
		return "modified"
	}
	if old.Size != cur.Size {
		return "modified"
	}
	return ""
}

// resolveWatchPaths expands WatchOptions.Paths (project-relative) to
// absolute paths under inst.Root and tacks on the canonical
// `.core/view.yaml` when the list is empty. Entries that would escape
// the project root (absolute paths, `..` segments) are skipped so the
// watcher never strays outside its manifest source — defence in depth
// in case a host passes user-supplied paths directly.
//
//	paths := resolveWatchPaths(inst, []string{"conf/thumbs.tmpl"})
func resolveWatchPaths(inst *Instance, rel []string) []string {
	if inst == nil || inst.Root == "" {
		return nil
	}
	if len(rel) == 0 {
		return []string{core.Path(inst.Root, ".core", "view.yaml")}
	}

	out := make([]string, 0, len(rel))
	seen := map[string]bool{}
	for _, p := range rel {
		if p == "" {
			continue
		}
		if core.PathIsAbs(p) {
			// Absolute paths only allowed when they sit inside the root
			// — protects the watcher from being steered at arbitrary
			// files via a crafted manifest.
			if !core.HasPrefix(p, inst.Root) {
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
			continue
		}
		if err := rejectPathTraversal("watch", p); err != nil {
			continue
		}
		abs := core.Path(inst.Root, p)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// WatchManifest is a convenience that spins a dev-mode watcher around
// `.core/view.yaml` alone and hooks a reload handler into
// ActionManifestChanged. The handler is called with the freshly-parsed
// manifest so subscribers don't re-run the YAML loader themselves.
// Matches the RFC §4.2 ergonomics — "hot reload on file changes" with
// no ceremony beyond an instance.
//
//	stop := inst.WatchManifest(ctx, func(m config.ViewManifest) {
//	    core.Info("manifest reloaded", "version", m.Version)
//	})
//	defer stop()
//
// Rules:
//
//   - Prod-mode → no-op + warning (same as Watch).
//
//   - The reload closure runs inline in the action bus — keep it quick
//     or dispatch the heavy lifting to a worker. A handler that blocks
//     here blocks the next cycle of the watcher.
//
//   - Parse failures on reload are logged (`app.WatchManifest: parse
//     failed`) and the previous manifest remains the caller's. The
//     watcher stays alive so a developer fixing a typo re-triggers
//     immediately.
func (inst *Instance) WatchManifest(ctx context.Context, reload func(config.ViewManifest)) (cancel func()) {
	if inst == nil || inst.Core == nil {
		return func() {}
	}
	if inst.Mode != ModeDev {
		core.Warn("app.WatchManifest refused in prod mode", "code", inst.Manifest.Code)
		return func() {}
	}
	medium := inst.medium
	if medium == nil {
		medium = coreio.Local
	}
	path := core.Path(inst.Root, ".core", "view.yaml")
	inst.Core.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		evt, ok := msg.(ActionManifestChanged)
		if !ok {
			return core.Result{OK: true}
		}
		if evt.Path != path {
			return core.Result{OK: true}
		}
		if evt.Kind == "deleted" {
			core.Warn("app.WatchManifest: manifest file deleted", "path", path)
			return core.Result{OK: true}
		}
		var manifest config.ViewManifest
		if err := LoadViewManifest(medium, path, &manifest); err != nil {
			core.Error("app.WatchManifest: parse failed", "path", path, "err", err)
			return core.Result{OK: true}
		}
		if reload != nil {
			reload(manifest)
		}
		return core.Result{OK: true}
	})
	return inst.Watch(ctx, WatchOptions{Medium: medium})
}
