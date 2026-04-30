// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"sync"
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

// TestWatch_Instance_Watch_Good exercises the full dev-mode hot-reload loop —
// write a manifest, start the watcher, rewrite the manifest, confirm
// ActionManifestChanged lands. Each subtest targets one branch of the
// public Watch contract described in watch.go.
func TestWatch_Instance_Watch_Good(t *testing.T) {
	t.Run("modified fires ActionManifestChanged", func(t *testing.T) {
		root := t.TempDir()
		medium := coreio.Local
		viewPath := core.Path(root, ".core", "view.yaml")
		if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
			t.Fatalf("ensure dir: %v", err)
		}
		if err := medium.Write(viewPath, "code: watch-app\nname: Watch\nversion: 0.1.0\n"); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		c := core.New()
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "watch-app", Version: "0.1.0"},
			Core:     c,
			Root:     root,
			Mode:     ModeDev,
			medium:   medium,
		}
		var (
			mu       sync.Mutex
			observed []ActionManifestChanged
		)
		c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
			if evt, ok := msg.(ActionManifestChanged); ok {
				mu.Lock()
				observed = append(observed, evt)
				mu.Unlock()
			}
			return core.Result{OK: true}
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stop := inst.Watch(ctx, WatchOptions{Interval: 20 * time.Millisecond})
		defer stop()

		// Wait for baseline to settle before editing.
		time.Sleep(60 * time.Millisecond)

		// Bump the mtime by rewriting with larger content so both
		// ModTime and Size diff — safe across filesystems with
		// second-precision timestamps.
		if err := medium.Write(viewPath, "code: watch-app\nname: Watch\nversion: 0.2.0\ndescription: updated\n"); err != nil {
			t.Fatalf("rewrite manifest: %v", err)
		}
		// Poll for the event so the test is resilient to scheduler
		// jitter — fail only if the watcher misses it within 2s.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(observed)
			mu.Unlock()
			if got > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(observed) == 0 {
			t.Fatal("watcher did not broadcast ActionManifestChanged")
		}
		evt := observed[0]
		if evt.Code != "watch-app" {
			t.Fatalf("Code = %q, want watch-app", evt.Code)
		}
		if evt.Kind != "modified" {
			t.Fatalf("Kind = %q, want modified", evt.Kind)
		}
		if evt.Path != viewPath {
			t.Fatalf("Path = %q, want %q", evt.Path, viewPath)
		}
	})
}

// TestWatch_Instance_Watch_Bad covers the refusal branches — prod mode,
// nil instance, empty root — every path that short-circuits to a
// no-op cancel.
func TestWatch_Instance_Watch_Bad(t *testing.T) {
	t.Run("prod mode returns noop cancel", func(t *testing.T) {
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "prod"},
			Core:     core.New(),
			Root:     t.TempDir(),
			Mode:     ModeProd,
			medium:   coreio.Local,
		}
		stop := inst.Watch(context.Background(), WatchOptions{Interval: 10 * time.Millisecond})
		if stop == nil {
			t.Fatal("Watch must return a cancel even in prod")
		}
		stop() // should be safe to call
	})

	t.Run("nil instance returns noop", func(t *testing.T) {
		var inst *Instance
		stop := inst.Watch(context.Background(), WatchOptions{})
		if stop == nil {
			t.Fatal("Watch on nil instance must still return a cancel")
		}
		stop()
	})

	t.Run("empty root has no paths to watch", func(t *testing.T) {
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "noroot"},
			Core:     core.New(),
			Mode:     ModeDev,
			medium:   coreio.Local,
		}
		stop := inst.Watch(context.Background(), WatchOptions{})
		stop()
	})
}

// TestWatch_Instance_Watch_Ugly exercises the edge cases — file deleted after
// baseline, absolute-path escapes rejected, bad poll interval falls
// back to the default.
func TestWatch_Instance_Watch_Ugly(t *testing.T) {
	t.Run("deleted manifest broadcasts deleted event", func(t *testing.T) {
		root := t.TempDir()
		medium := coreio.Local
		viewPath := core.Path(root, ".core", "view.yaml")
		if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
			t.Fatalf("ensure dir: %v", err)
		}
		if err := medium.Write(viewPath, "code: del-app\nversion: 0.1.0\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		c := core.New()
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "del-app"},
			Core:     c,
			Root:     root,
			Mode:     ModeDev,
			medium:   medium,
		}
		var (
			mu       sync.Mutex
			observed []ActionManifestChanged
		)
		c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
			if evt, ok := msg.(ActionManifestChanged); ok {
				mu.Lock()
				observed = append(observed, evt)
				mu.Unlock()
			}
			return core.Result{OK: true}
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stop := inst.Watch(ctx, WatchOptions{Interval: 15 * time.Millisecond})
		defer stop()
		time.Sleep(45 * time.Millisecond)
		if err := medium.DeleteAll(viewPath); err != nil {
			t.Fatalf("delete: %v", err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(observed)
			mu.Unlock()
			if got > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(observed) == 0 {
			t.Fatal("delete was not observed")
		}
		if observed[0].Kind != "deleted" {
			t.Fatalf("Kind = %q, want deleted", observed[0].Kind)
		}
	})

	t.Run("path traversal entries are rejected", func(t *testing.T) {
		inst := &Instance{Root: t.TempDir()}
		paths := resolveWatchPaths(inst, []string{"../escape.yaml", "./conf/ok.yaml"})
		for _, p := range paths {
			if !core.HasPrefix(p, inst.Root) {
				t.Fatalf("resolveWatchPaths leaked outside root: %q", p)
			}
		}
		if len(paths) != 1 {
			t.Fatalf("expected 1 resolved path, got %d: %v", len(paths), paths)
		}
	})

	t.Run("zero interval falls back to default", func(t *testing.T) {
		root := t.TempDir()
		medium := coreio.Local
		viewPath := core.Path(root, ".core", "view.yaml")
		if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
			t.Fatalf("ensure dir: %v", err)
		}
		if err := medium.Write(viewPath, "code: default\nversion: 0.1.0\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "default"},
			Core:     core.New(),
			Root:     root,
			Mode:     ModeDev,
			medium:   medium,
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Zero interval should use DefaultWatchInterval; the test just
		// confirms Watch returns without spinning — actual timing
		// coverage is in the "modified" subtest.
		stop := inst.Watch(ctx, WatchOptions{Interval: 0})
		stop()
	})
}

// TestWatch_ClassifyChange_Good pins the state-machine that decides
// between created / modified / deleted / no-op transitions.
func TestWatch_ClassifyChange_Good(t *testing.T) {
	cases := map[string]struct {
		old  watchEntry
		cur  watchEntry
		want string
	}{
		"created":          {watchEntry{}, watchEntry{Exists: true, Size: 10}, "created"},
		"deleted":          {watchEntry{Exists: true, Size: 10}, watchEntry{}, "deleted"},
		"modified bySize":  {watchEntry{Exists: true, Size: 10}, watchEntry{Exists: true, Size: 12}, "modified"},
		"unchanged":        {watchEntry{Exists: true, Size: 10}, watchEntry{Exists: true, Size: 10}, ""},
		"never there":      {watchEntry{}, watchEntry{}, ""},
		"modified byMtime": {watchEntry{Exists: true, ModTime: time.Unix(10, 0), Size: 5}, watchEntry{Exists: true, ModTime: time.Unix(20, 0), Size: 5}, "modified"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := classifyChange(tc.old, tc.cur); got != tc.want {
				t.Fatalf("classifyChange(%+v, %+v) = %q, want %q", tc.old, tc.cur, got, tc.want)
			}
		})
	}
}

// TestWatch_ClassifyChange_Bad ensures the function is total — no
// panics for zero values or extreme ModTime values.
func TestWatch_ClassifyChange_Bad(t *testing.T) {
	// Zero values produce the empty-string "no event" result.
	if got := classifyChange(watchEntry{}, watchEntry{}); got != "" {
		t.Fatalf("zero values: got %q, want empty", got)
	}
	// Far-future ModTime change counts as modified.
	future := time.Now().Add(72 * time.Hour)
	got := classifyChange(
		watchEntry{Exists: true, ModTime: time.Unix(0, 0), Size: 1},
		watchEntry{Exists: true, ModTime: future, Size: 1},
	)
	if got != "modified" {
		t.Fatalf("future mtime: got %q, want modified", got)
	}
}

// TestWatch_ClassifyChange_Ugly pins the boundary where old and new
// entries agree on Size but disagree on ModTime — the editor-save case
// we care about most.
func TestWatch_ClassifyChange_Ugly(t *testing.T) {
	old := watchEntry{Exists: true, ModTime: time.Unix(1_700_000_000, 0), Size: 64}
	cur := watchEntry{Exists: true, ModTime: time.Unix(1_700_000_030, 0), Size: 64}
	if got := classifyChange(old, cur); got != "modified" {
		t.Fatalf("mtime-only change should surface as modified, got %q", got)
	}
}

// TestWatch_Instance_WatchManifest_Good confirms WatchManifest calls the supplied
// reload callback when the manifest changes on disk.
func TestWatch_Instance_WatchManifest_Good(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local
	viewPath := core.Path(root, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := medium.Write(viewPath, "code: wm\nname: WM\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := core.New()
	inst := &Instance{
		Manifest: config.ViewManifest{Code: "wm"},
		Core:     c,
		Root:     root,
		Mode:     ModeDev,
		medium:   medium,
	}
	var (
		mu      sync.Mutex
		latest  config.ViewManifest
		arrived int
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := inst.WatchManifest(ctx, func(m config.ViewManifest) {
		mu.Lock()
		latest = m
		arrived++
		mu.Unlock()
	})
	defer stop()
	time.Sleep(45 * time.Millisecond)
	if err := medium.Write(viewPath, "code: wm\nname: WM\nversion: 0.2.0\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := arrived
		mu.Unlock()
		if got > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if arrived == 0 {
		t.Fatal("reload callback was not invoked")
	}
	if latest.Version != "0.2.0" {
		t.Fatalf("latest.Version = %q, want 0.2.0", latest.Version)
	}
}

// TestWatch_Instance_WatchManifest_Bad ensures prod mode and nil instance both
// short-circuit WatchManifest without spinning a watcher.
func TestWatch_Instance_WatchManifest_Bad(t *testing.T) {
	t.Run("prod mode no-ops", func(t *testing.T) {
		inst := &Instance{
			Manifest: config.ViewManifest{Code: "prod"},
			Core:     core.New(),
			Root:     t.TempDir(),
			Mode:     ModeProd,
			medium:   coreio.Local,
		}
		stop := inst.WatchManifest(context.Background(), func(config.ViewManifest) {})
		stop()
	})
	t.Run("nil instance is safe", func(t *testing.T) {
		var inst *Instance
		stop := inst.WatchManifest(context.Background(), nil)
		stop()
	})
}

// TestWatch_Instance_WatchManifest_Ugly drives the delete + bad-parse branches
// of WatchManifest. Both are tolerated — the watcher survives and
// keeps running.
func TestWatch_Instance_WatchManifest_Ugly(t *testing.T) {
	root := t.TempDir()
	medium := coreio.Local
	viewPath := core.Path(root, ".core", "view.yaml")
	if err := medium.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := medium.Write(viewPath, "code: ugly\nversion: 0.1.0\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := core.New()
	inst := &Instance{
		Manifest: config.ViewManifest{Code: "ugly"},
		Core:     c,
		Root:     root,
		Mode:     ModeDev,
		medium:   medium,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 4)
	stop := inst.WatchManifest(ctx, func(config.ViewManifest) { called <- struct{}{} })
	defer stop()
	time.Sleep(45 * time.Millisecond)

	// Write invalid YAML — the parse should log an error but the
	// watcher must stay alive for the next edit.
	if err := medium.Write(viewPath, "this is : not : valid : : yaml ::\n\t\ttab"); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	// Recover with a valid manifest — the reload callback must fire.
	if err := medium.Write(viewPath, "code: ugly\nname: Ugly\nversion: 0.3.0\n"); err != nil {
		t.Fatalf("write good: %v", err)
	}

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("reload callback never fired after recovery")
	}
}

// TestWatch_ResolveWatchPaths_Good pins the happy path — empty list
// yields `.core/view.yaml`, explicit relative entries are joined to
// the root.
func TestWatch_ResolveWatchPaths_Good(t *testing.T) {
	inst := &Instance{Root: "/tmp/app"}

	paths := resolveWatchPaths(inst, nil)
	if len(paths) != 1 {
		t.Fatalf("default list should have 1 entry, got %d", len(paths))
	}
	if paths[0] != core.Path("/tmp/app", ".core", "view.yaml") {
		t.Fatalf("default path mismatch: %q", paths[0])
	}

	paths = resolveWatchPaths(inst, []string{"conf/a.yaml", "conf/b.yaml"})
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
}

// TestWatch_ResolveWatchPaths_Bad exercises nil instance and empty
// entries — both produce no watchable paths.
func TestWatch_ResolveWatchPaths_Bad(t *testing.T) {
	if got := resolveWatchPaths(nil, []string{"a"}); len(got) != 0 {
		t.Fatalf("nil inst: got %v, want empty", got)
	}
	inst := &Instance{Root: "/tmp/app"}
	// An explicit list of all-empty entries should yield zero watchable
	// paths — the default view.yaml fallback is only applied when the
	// caller didn't provide any list at all.
	if got := resolveWatchPaths(inst, []string{"", ""}); len(got) != 0 {
		t.Fatalf("empty entries: got %v, want empty", got)
	}
}

// TestWatch_ResolveWatchPaths_Ugly covers the defence-in-depth cases —
// absolute outside root, duplicates, traversal segments.
func TestWatch_ResolveWatchPaths_Ugly(t *testing.T) {
	inst := &Instance{Root: "/tmp/app"}

	paths := resolveWatchPaths(inst, []string{
		"/etc/passwd",            // absolute outside → dropped
		"/tmp/app/conf/alt.yaml", // absolute inside → kept
		"../escape.yaml",         // traversal → dropped
		"./conf/ok.yaml",         // relative → kept (joins to /tmp/app/conf/ok.yaml)
		"./conf/ok.yaml",         // duplicate relative → dropped
		"/tmp/app/conf/ok2.yaml", // absolute inside → kept
	})
	if len(paths) != 3 {
		t.Fatalf("expected 3 resolved paths, got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if !core.HasPrefix(p, inst.Root) {
			t.Fatalf("resolved path escaped root: %q", p)
		}
	}
}
