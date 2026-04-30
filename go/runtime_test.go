// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"runtime"
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/app"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	"gopkg.in/yaml.v3"
)

func TestRuntime_BootRegistersAndExecutesDefaultHandlers_Good(t *testing.T) {
	projectDir := t.TempDir()
	workspaceHome := t.TempDir()
	if err := coreio.Local.Write(core.Path(projectDir, "data", "hello.txt"), "runtime hello"); err != nil {
		t.Fatalf("Write data file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fetch":
			_, _ = w.Write([]byte("runtime fetch ok"))
		case "/recall":
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","query":"` + r.URL.Query().Get("q") + `"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("OPENBRAIN_URL", srv.URL+"/recall")

	command, args, wantStdout := runtimeTestCommand()
	manifest := config.ViewManifest{
		Code:    "runtime-good",
		Name:    "Runtime Good",
		Version: "0.1.0",
		Permissions: config.ViewPermissions{
			Read: []string{"./data/"},
			Net:  []string{mustHostPort(t, srv.URL)},
			Run:  []string{command},
		},
		Config: map[string]any{
			"store": true,
		},
	}
	writeViewManifest(t, projectDir, manifest)

	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithWorkspaceHome(workspaceHome),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}

	for _, name := range []string{
		"fs.read",
		"store.get",
		"store.set",
		"net.fetch",
		"process.run",
		"gui.window.create",
		"brain.recall",
	} {
		if !inst.Core.Action(name).Exists() {
			t.Errorf("action %q is not registered", name)
		}
	}

	fsRead := inst.Core.Action("fs.read").Run(context.Background(), core.NewOptions(
		core.Option{Key: "path", Value: "data/hello.txt"},
	))
	if !fsRead.OK {
		t.Fatalf("fs.read failed: %v", fsRead.Value)
	}
	if got := resultMapString(fsRead, "content"); got != "runtime hello" {
		t.Errorf("fs.read content = %q; want runtime hello", got)
	}

	if r := inst.Core.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
		core.Option{Key: "value", Value: "dark"},
	)); !r.OK {
		t.Fatalf("store.set failed: %v", r.Value)
	}
	storeGet := inst.Core.Action("store.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: "prefs"},
		core.Option{Key: "key", Value: "theme"},
	))
	if !storeGet.OK {
		t.Fatalf("store.get failed: %v", storeGet.Value)
	}
	if got := resultMapString(storeGet, "value"); got != "dark" {
		t.Errorf("store.get value = %q; want dark", got)
	}

	netFetch := inst.Core.Action("net.fetch").Run(context.Background(), core.NewOptions(
		core.Option{Key: "url", Value: srv.URL + "/fetch"},
	))
	if !netFetch.OK {
		t.Fatalf("net.fetch failed: %v", netFetch.Value)
	}
	if got := resultMapString(netFetch, "body"); got != "runtime fetch ok" {
		t.Errorf("net.fetch body = %q; want runtime fetch ok", got)
	}

	processRun := inst.Core.Action("process.run").Run(context.Background(), core.NewOptions(
		core.Option{Key: "command", Value: command},
		core.Option{Key: "args", Value: args},
	))
	if !processRun.OK {
		t.Fatalf("process.run failed: %v", processRun.Value)
	}
	if got := strings.TrimSpace(resultMapString(processRun, "stdout")); got != wantStdout {
		t.Errorf("process.run stdout = %q; want %q", got, wantStdout)
	}

	brainRecall := inst.Core.Action("brain.recall").Run(context.Background(), core.NewOptions(
		core.Option{Key: "query", Value: "oak"},
	))
	if !brainRecall.OK {
		t.Fatalf("brain.recall failed: %v", brainRecall.Value)
	}
	memories, ok := resultMapAny(brainRecall, "memories").([]any)
	if !ok || len(memories) != 1 {
		t.Fatalf("brain.recall memories = %T/%v; want one memory", resultMapAny(brainRecall, "memories"), resultMapAny(brainRecall, "memories"))
	}

	guiWindow := inst.Core.Action("gui.window.create").Run(context.Background(), core.NewOptions(
		core.Option{Key: "title", Value: "No Host"},
	))
	if guiWindow.OK {
		t.Fatal("gui.window.create unexpectedly succeeded without a GUI host")
	}
	if strings.Contains(fmt.Sprint(guiWindow.Value), "not registered") {
		t.Fatalf("gui.window.create fell through to no handler: %v", guiWindow.Value)
	}
}

func TestRuntime_WorkspaceStore_EncryptsAtRest_Good(t *testing.T) {
	projectDir := t.TempDir()
	workspaceHome := t.TempDir()
	manifest := config.ViewManifest{
		Code:    "runtime-encrypted-store",
		Name:    "Encrypted Store",
		Version: "0.1.0",
		Config: map[string]any{
			"store": true,
		},
	}
	writeViewManifest(t, projectDir, manifest)

	inst, err := app.Boot(context.Background(), projectDir,
		app.WithMode(app.ModeDev),
		app.WithMedium(coreio.Local),
		app.WithWorkspaceHome(workspaceHome),
	)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}

	group := "group-plaintext-check-123456"
	key := "key-plaintext-check-123456"
	value := "value-plaintext-check-123456"
	if r := inst.Core.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: group},
		core.Option{Key: "key", Value: key},
		core.Option{Key: "value", Value: value},
	)); !r.OK {
		t.Fatalf("store.set failed: %v", r.Value)
	}

	storeDir := core.Path(workspaceHome, ".core", app.DataDirName, manifest.Code, "store")
	for _, path := range []string{
		core.Path(storeDir, "store.db"),
		core.Path(storeDir, "store.db-wal"),
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("Read %s: %v", path, err)
		}
		raw := string(body)
		for _, secret := range []string{group, key, value} {
			if strings.Contains(raw, secret) {
				t.Fatalf("%s leaked plaintext %q", path, secret)
			}
		}
	}
}

func writeViewManifest(t *testing.T, dir string, manifest config.ViewManifest) {
	t.Helper()
	body, err := yaml.Marshal(&manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	viewPath := core.Path(dir, ".core", "view.yaml")
	if err := coreio.Local.EnsureDir(core.PathDir(viewPath)); err != nil {
		t.Fatalf("EnsureDir .core: %v", err)
	}
	if err := coreio.Local.Write(viewPath, string(body)); err != nil {
		t.Fatalf("Write view.yaml: %v", err)
	}
}

func mustHostPort(t *testing.T, raw string) string {
	t.Helper()
	u, err := neturl.Parse(raw)
	if err != nil {
		t.Fatalf("Parse URL %q: %v", raw, err)
	}
	return u.Host
}

func runtimeTestCommand() (string, []string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo runtime-ok"}, "runtime-ok"
	}
	return "sh", []string{"-c", "printf runtime-ok"}, "runtime-ok"
}

func resultMapAny(result core.Result, key string) any {
	m, ok := result.Value.(map[string]any)
	if !ok {
		return nil
	}
	return m[key]
}

func resultMapString(result core.Result, key string) string {
	value, _ := resultMapAny(result, key).(string)
	return value
}
