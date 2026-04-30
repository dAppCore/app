// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"io"
	"net/http"
	neturl "net/url"
	"os/exec"
	"path"
	"runtime"
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type runtimeBindings struct {
	inst *Instance

	storeMu sync.Mutex
	store   *workspaceObjectStore

	processes *managedProcesses
}

func registerRuntimeActions(inst *Instance) error {
	if inst == nil {
		return core.E("app.registerRuntimeActions", "nil instance", nil)
	}
	if inst.Core == nil {
		return core.E("app.registerRuntimeActions", "nil core", nil)
	}
	if inst.runtime != nil {
		return nil
	}

	rt := &runtimeBindings{
		inst:      inst,
		processes: newManagedProcesses(),
	}
	inst.runtime = rt

	rt.register("fs.read", rt.handleFSRead)
	rt.register("fs.write", rt.handleFSWrite)
	rt.register("fs.list", rt.handleFSList)
	rt.register("fs.delete", rt.handleFSDelete)

	rt.register("store.get", rt.handleStoreGet)
	rt.register("store.set", rt.handleStoreSet)
	rt.register("store.delete", rt.handleStoreDelete)

	rt.register("net.fetch", rt.handleNetFetch)
	rt.register("net.ws", rt.handleNetWS)
	rt.register("brain.recall", rt.handleBrainRecall)

	rt.register("process.run", rt.handleProcessRun)
	rt.register("process.add", rt.handleProcessAdd)
	rt.register("process.start", rt.handleProcessStart)
	rt.register("process.stop", rt.handleProcessStop)
	rt.register("process.kill", rt.handleProcessKill)
	rt.register("process.list", rt.handleProcessList)
	rt.register("process.get", rt.handleProcessGet)
	rt.register("process.stdout.subscribe", rt.handleProcessStdout)
	rt.register("process.stdin.write", rt.handleProcessStdinWrite)

	rt.register("gui.window.create", rt.handleGUIWindowCreate)
	rt.register("gui.dialog.confirm", rt.handleGUIDialogConfirm)
	rt.register("gui.dialog.open", rt.handleGUIDialogOpen)
	rt.register("gui.dialog.save", rt.handleGUIDialogSave)
	rt.register("gui.browser.open", rt.handleGUIBrowserOpen)
	rt.register("gui.notification.send", rt.handleGUINotificationSend)
	rt.register("gui.clipboard.read", rt.handleGUIClipboardRead)
	rt.register("gui.clipboard.write", rt.handleGUIClipboardWrite)

	rt.register("device.location", rt.handleDeviceLocation)
	rt.register("device.camera", rt.handleDeviceCamera)
	rt.register("device.microphone", rt.handleDeviceMicrophone)

	rt.register("i18n.translate", rt.handleI18NTranslate)
	return nil
}

func (rt *runtimeBindings) register(name string, handler core.ActionHandler) {
	if rt == nil || rt.inst == nil || rt.inst.Core == nil || name == "" || handler == nil {
		return
	}
	if rt.inst.Core.Action(name).Exists() {
		return
	}
	rt.inst.Core.Action(name, handler)
}

func (rt *runtimeBindings) shutdown() {
	if rt == nil {
		return
	}
	if rt.processes != nil {
		rt.processes.shutdown()
	}

	rt.storeMu.Lock()
	store := rt.store
	rt.store = nil
	rt.storeMu.Unlock()
	if store != nil {
		if err := store.Close(); err != nil {
			core.Warn("app.runtimeBindings.shutdown: store close failed", "err", err)
		}
	}
}

func (rt *runtimeBindings) workspaceStore() (*workspaceObjectStore, error) {
	if rt == nil {
		return nil, core.E("app.runtimeBindings.workspaceStore", "nil runtime", nil)
	}
	rt.storeMu.Lock()
	defer rt.storeMu.Unlock()
	if rt.store != nil && rt.store.ws == rt.inst.Workspace {
		return rt.store, nil
	}
	if rt.store != nil {
		if err := rt.store.Close(); err != nil {
			return nil, core.E("app.runtimeBindings.workspaceStore", "close previous store failed", err)
		}
		rt.store = nil
	}
	rt.store = newWorkspaceObjectStore(rt.inst.Workspace)
	return rt.store, nil
}

func (rt *runtimeBindings) handleFSRead(_ context.Context, opts core.Options) core.Result {
	accessPath, mediumPath, err := rt.normalisedProjectPath(opts.String("path"))
	if err != nil {
		return resultError("app.runtime.handleFSRead", "invalid path", err)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "fs.read", accessPath); err != nil {
		return resultError("app.runtime.handleFSRead", "access denied", err)
	}
	medium, target, err := rt.projectMediumPath(mediumPath)
	if err != nil {
		return resultError("app.runtime.handleFSRead", "resolve project medium failed", err)
	}
	body, err := medium.Read(target)
	if err != nil {
		return resultError("app.runtime.handleFSRead", "read failed", err)
	}
	return resultValue(map[string]any{"content": body})
}

func (rt *runtimeBindings) handleFSWrite(_ context.Context, opts core.Options) core.Result {
	accessPath, mediumPath, err := rt.normalisedProjectPath(opts.String("path"))
	if err != nil {
		return resultError("app.runtime.handleFSWrite", "invalid path", err)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "fs.write", accessPath); err != nil {
		return resultError("app.runtime.handleFSWrite", "access denied", err)
	}
	medium, target, err := rt.projectMediumPath(mediumPath)
	if err != nil {
		return resultError("app.runtime.handleFSWrite", "resolve project medium failed", err)
	}
	if err := medium.EnsureDir(core.PathDir(target)); err != nil {
		return resultError("app.runtime.handleFSWrite", "ensure dir failed", err)
	}
	if err := medium.Write(target, opts.String("content")); err != nil {
		return resultError("app.runtime.handleFSWrite", "write failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleFSList(_ context.Context, opts core.Options) core.Result {
	accessPath, mediumPath, err := rt.normalisedProjectPath(opts.String("path"))
	if err != nil {
		return resultError("app.runtime.handleFSList", "invalid path", err)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "fs.list", accessPath); err != nil {
		return resultError("app.runtime.handleFSList", "access denied", err)
	}
	medium, target, err := rt.projectMediumPath(mediumPath)
	if err != nil {
		return resultError("app.runtime.handleFSList", "resolve project medium failed", err)
	}
	entries, err := medium.List(target)
	if err != nil {
		return resultError("app.runtime.handleFSList", "list failed", err)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		out = append(out, map[string]any{
			"name":   entry.Name(),
			"is_dir": entry.IsDir(),
		})
	}
	return resultValue(map[string]any{"entries": out})
}

func (rt *runtimeBindings) handleFSDelete(_ context.Context, opts core.Options) core.Result {
	accessPath, mediumPath, err := rt.normalisedProjectPath(opts.String("path"))
	if err != nil {
		return resultError("app.runtime.handleFSDelete", "invalid path", err)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "fs.delete", accessPath); err != nil {
		return resultError("app.runtime.handleFSDelete", "access denied", err)
	}
	medium, target, err := rt.projectMediumPath(mediumPath)
	if err != nil {
		return resultError("app.runtime.handleFSDelete", "resolve project medium failed", err)
	}
	if err := medium.Delete(target); err != nil {
		return resultError("app.runtime.handleFSDelete", "delete failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleStoreGet(_ context.Context, opts core.Options) core.Result {
	if err := CheckActionAccess(&rt.inst.Manifest, "store.get", storeAccessTarget(opts.String("group"), opts.String("key"))); err != nil {
		return resultError("app.runtime.handleStoreGet", "access denied", err)
	}
	store, err := rt.workspaceStore()
	if err != nil {
		return resultError("app.runtime.handleStoreGet", "workspace store unavailable", err)
	}
	value, err := store.Get(opts.String("group"), opts.String("key"))
	if err != nil {
		return resultError("app.runtime.handleStoreGet", "read failed", err)
	}
	return resultValue(map[string]any{"value": value})
}

func (rt *runtimeBindings) handleStoreSet(_ context.Context, opts core.Options) core.Result {
	if err := CheckActionAccess(&rt.inst.Manifest, "store.set", storeAccessTarget(opts.String("group"), opts.String("key"))); err != nil {
		return resultError("app.runtime.handleStoreSet", "access denied", err)
	}
	store, err := rt.workspaceStore()
	if err != nil {
		return resultError("app.runtime.handleStoreSet", "workspace store unavailable", err)
	}
	if err := store.Set(opts.String("group"), opts.String("key"), opts.String("value")); err != nil {
		return resultError("app.runtime.handleStoreSet", "write failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleStoreDelete(_ context.Context, opts core.Options) core.Result {
	if err := CheckActionAccess(&rt.inst.Manifest, "store.delete", storeAccessTarget(opts.String("group"), opts.String("key"))); err != nil {
		return resultError("app.runtime.handleStoreDelete", "access denied", err)
	}
	store, err := rt.workspaceStore()
	if err != nil {
		return resultError("app.runtime.handleStoreDelete", "workspace store unavailable", err)
	}
	if err := store.Delete(opts.String("group"), opts.String("key")); err != nil {
		return resultError("app.runtime.handleStoreDelete", "delete failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleNetFetch(ctx context.Context, opts core.Options) core.Result {
	rawURL := core.Trim(opts.String("url"))
	if rawURL == "" {
		return resultError("app.runtime.handleNetFetch", "url is required", nil)
	}
	host := hostPortOfURL(rawURL)
	if host == "" {
		return resultError("app.runtime.handleNetFetch", "invalid url", nil)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "net.fetch", host); err != nil {
		return resultError("app.runtime.handleNetFetch", "access denied", err)
	}

	method := core.Upper(core.Trim(opts.String("method")))
	body := opts.String("body")
	if method == "" {
		if body != "" {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, core.NewReader(body))
	if err != nil {
		return resultError("app.runtime.handleNetFetch", "build request failed", err)
	}
	for key, value := range stringMap(opts.Get("headers")) {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return resultError("app.runtime.handleNetFetch", "request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resultError("app.runtime.handleNetFetch", "read body failed", err)
	}
	return resultValue(map[string]any{
		"status": resp.StatusCode,
		"body":   string(payload),
	})
}

func (rt *runtimeBindings) handleNetWS(_ context.Context, opts core.Options) core.Result {
	rawURL := core.Trim(opts.String("url"))
	if rawURL == "" {
		return resultError("app.runtime.handleNetWS", "url is required", nil)
	}
	if host := hostPortOfURL(rawURL); host != "" {
		if err := CheckActionAccess(&rt.inst.Manifest, "net.ws", host); err != nil {
			return resultError("app.runtime.handleNetWS", "access denied", err)
		}
	}
	return resultError("app.runtime.handleNetWS", "websocket runtime not implemented by this host", nil)
}

func (rt *runtimeBindings) handleBrainRecall(ctx context.Context, opts core.Options) core.Result {
	query := core.Trim(opts.String("query"))
	if query == "" {
		return resultError("app.runtime.handleBrainRecall", "query is required", nil)
	}

	endpoint := core.Trim(core.Env("OPENBRAIN_URL"))
	if endpoint == "" {
		endpoint = "https://api.openbrain/recall"
	}
	host := hostPortOfURL(endpoint)
	if host == "" {
		return resultError("app.runtime.handleBrainRecall", "invalid OpenBrain endpoint", nil)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "brain.recall", host); err != nil {
		return resultError("app.runtime.handleBrainRecall", "access denied", err)
	}

	base, err := neturl.Parse(endpoint)
	if err != nil {
		return resultError("app.runtime.handleBrainRecall", "parse endpoint failed", err)
	}
	params := base.Query()
	params.Set("q", query)
	base.RawQuery = params.Encode()

	result := rt.handleNetFetch(ctx, core.NewOptions(
		core.Option{Key: "url", Value: base.String()},
		core.Option{Key: "method", Value: http.MethodGet},
	))
	if !result.OK {
		return result
	}
	payload, _ := result.Value.(map[string]any)
	body, _ := payload["body"].(string)

	var decoded any
	if r := core.JSONUnmarshal([]byte(body), &decoded); !r.OK {
		return resultValue(map[string]any{"memories": []any{body}})
	}
	switch value := decoded.(type) {
	case map[string]any:
		if memories, ok := value["memories"]; ok {
			return resultValue(map[string]any{"memories": memories})
		}
		return resultValue(map[string]any{"memories": []any{value}})
	case []any:
		return resultValue(map[string]any{"memories": value})
	default:
		return resultValue(map[string]any{"memories": []any{value}})
	}
}

func (rt *runtimeBindings) handleProcessRun(ctx context.Context, opts core.Options) core.Result {
	command := core.Trim(opts.String("command"))
	if command == "" {
		return resultError("app.runtime.handleProcessRun", "command is required", nil)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "process.run", command); err != nil {
		return resultError("app.runtime.handleProcessRun", "access denied", err)
	}
	dir, err := rt.processDir(opts.String("dir"))
	if err != nil {
		return resultError("app.runtime.handleProcessRun", "invalid working directory", err)
	}
	result, err := rt.processes.runOnce(ctx, command, stringSlice(opts.Get("args")), stringSlice(opts.Get("env")), dir)
	if err != nil {
		return resultError("app.runtime.handleProcessRun", "run failed", err)
	}
	return resultValue(result)
}

func (rt *runtimeBindings) handleProcessAdd(_ context.Context, opts core.Options) core.Result {
	command := core.Trim(opts.String("command"))
	key := core.Trim(opts.String("key"))
	if key == "" || command == "" {
		return resultError("app.runtime.handleProcessAdd", "key and command are required", nil)
	}
	if err := CheckActionAccess(&rt.inst.Manifest, "process.add", command); err != nil {
		return resultError("app.runtime.handleProcessAdd", "access denied", err)
	}
	dir, err := rt.processDir(opts.String("dir"))
	if err != nil {
		return resultError("app.runtime.handleProcessAdd", "invalid working directory", err)
	}
	if err := rt.processes.add(key, command, stringSlice(opts.Get("args")), stringSlice(opts.Get("env")), dir); err != nil {
		return resultError("app.runtime.handleProcessAdd", "register failed", err)
	}
	return resultValue(map[string]any{"key": key})
}

func (rt *runtimeBindings) handleProcessStart(ctx context.Context, opts core.Options) core.Result {
	started, err := rt.processes.start(ctx, core.Trim(opts.String("key")))
	if err != nil {
		return resultError("app.runtime.handleProcessStart", "start failed", err)
	}
	return resultValue(map[string]any{"started": started})
}

func (rt *runtimeBindings) handleProcessStop(_ context.Context, opts core.Options) core.Result {
	stopped, err := rt.processes.stop(core.Trim(opts.String("key")))
	if err != nil {
		return resultError("app.runtime.handleProcessStop", "stop failed", err)
	}
	return resultValue(map[string]any{"stopped": stopped})
}

func (rt *runtimeBindings) handleProcessKill(_ context.Context, opts core.Options) core.Result {
	killed, err := rt.processes.kill(core.Trim(opts.String("key")))
	if err != nil {
		return resultError("app.runtime.handleProcessKill", "kill failed", err)
	}
	return resultValue(map[string]any{"killed": killed})
}

func (rt *runtimeBindings) handleProcessList(_ context.Context, _ core.Options) core.Result {
	return resultValue(map[string]any{"keys": rt.processes.list()})
}

func (rt *runtimeBindings) handleProcessGet(_ context.Context, opts core.Options) core.Result {
	info, err := rt.processes.info(core.Trim(opts.String("key")))
	if err != nil {
		return resultError("app.runtime.handleProcessGet", "lookup failed", err)
	}
	return resultValue(info)
}

func (rt *runtimeBindings) handleProcessStdout(_ context.Context, opts core.Options) core.Result {
	stdout, err := rt.processes.stdoutValue(core.Trim(opts.String("key")))
	if err != nil {
		return resultError("app.runtime.handleProcessStdout", "read stdout failed", err)
	}
	return resultValue(map[string]any{"stdout": stdout})
}

func (rt *runtimeBindings) handleProcessStdinWrite(_ context.Context, opts core.Options) core.Result {
	if err := rt.processes.writeStdin(core.Trim(opts.String("key")), opts.String("data")); err != nil {
		return resultError("app.runtime.handleProcessStdinWrite", "write stdin failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleGUIWindowCreate(_ context.Context, opts core.Options) core.Result {
	return resultError(
		"app.runtime.handleGUIWindowCreate",
		"no GUI host registered for gui.window.create (title="+opts.String("title")+")",
		nil,
	)
}

func (rt *runtimeBindings) handleGUIDialogConfirm(_ context.Context, opts core.Options) core.Result {
	confirmed, err := runDialogConfirm(opts.String("message"))
	if err != nil {
		return resultError("app.runtime.handleGUIDialogConfirm", "dialog failed", err)
	}
	return resultValue(map[string]any{"confirmed": confirmed})
}

func (rt *runtimeBindings) handleGUIDialogOpen(_ context.Context, opts core.Options) core.Result {
	path, err := runDialogOpen(opts.String("title"))
	if err != nil {
		return resultError("app.runtime.handleGUIDialogOpen", "dialog failed", err)
	}
	return resultValue(map[string]any{"path": path})
}

func (rt *runtimeBindings) handleGUIDialogSave(_ context.Context, opts core.Options) core.Result {
	path, err := runDialogSave(opts.String("title"), opts.String("default_name"))
	if err != nil {
		return resultError("app.runtime.handleGUIDialogSave", "dialog failed", err)
	}
	return resultValue(map[string]any{"path": path})
}

func (rt *runtimeBindings) handleGUIBrowserOpen(_ context.Context, opts core.Options) core.Result {
	if err := openBrowser(core.Trim(opts.String("url"))); err != nil {
		return resultError("app.runtime.handleGUIBrowserOpen", "open browser failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleGUINotificationSend(_ context.Context, opts core.Options) core.Result {
	if err := sendNotification(opts.String("title"), opts.String("body")); err != nil {
		return resultError("app.runtime.handleGUINotificationSend", "notification failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleGUIClipboardRead(_ context.Context, _ core.Options) core.Result {
	text, err := readClipboard()
	if err != nil {
		return resultError("app.runtime.handleGUIClipboardRead", "read clipboard failed", err)
	}
	return resultValue(map[string]any{"text": text})
}

func (rt *runtimeBindings) handleGUIClipboardWrite(_ context.Context, opts core.Options) core.Result {
	if err := writeClipboard(opts.String("text")); err != nil {
		return resultError("app.runtime.handleGUIClipboardWrite", "write clipboard failed", err)
	}
	return core.Result{OK: true}
}

func (rt *runtimeBindings) handleDeviceLocation(_ context.Context, _ core.Options) core.Result {
	lat := core.Trim(core.Env("CORE_DEVICE_LATITUDE"))
	lng := core.Trim(core.Env("CORE_DEVICE_LONGITUDE"))
	if lat == "" || lng == "" {
		return resultError("app.runtime.handleDeviceLocation", "device location not configured for this host", nil)
	}
	return resultValue(map[string]any{
		"latitude":  lat,
		"longitude": lng,
		"accuracy":  core.Trim(core.Env("CORE_DEVICE_ACCURACY")),
	})
}

func (rt *runtimeBindings) handleDeviceCamera(_ context.Context, _ core.Options) core.Result {
	return resultError("app.runtime.handleDeviceCamera", "camera host integration not implemented", nil)
}

func (rt *runtimeBindings) handleDeviceMicrophone(_ context.Context, _ core.Options) core.Result {
	return resultError("app.runtime.handleDeviceMicrophone", "microphone host integration not implemented", nil)
}

func (rt *runtimeBindings) handleI18NTranslate(_ context.Context, opts core.Options) core.Result {
	key := core.Trim(opts.String("key"))
	if key == "" {
		return resultError("app.runtime.handleI18NTranslate", "key is required", nil)
	}
	if locale := core.Trim(opts.String("locale")); locale != "" {
		if r := rt.inst.Core.I18n().SetLanguage(locale); !r.OK {
			return resultError("app.runtime.handleI18NTranslate", "set language failed", extractErr(r))
		}
	}
	result := rt.inst.Core.I18n().Translate(key)
	if !result.OK {
		return resultError("app.runtime.handleI18NTranslate", "translation failed", extractErr(result))
	}
	return resultValue(map[string]any{"translated": core.Sprint(result.Value)})
}

func (rt *runtimeBindings) normalisedProjectPath(raw string) (string, string, error) {
	raw = core.Trim(core.Replace(raw, "\\", "/"))
	if raw == "" {
		return "", "", core.E("app.runtime.normalisedProjectPath", "path is required", nil)
	}
	access := raw
	if core.HasPrefix(access, "/") {
		access = "." + access
	}
	if !core.HasPrefix(access, "./") && !core.HasPrefix(access, "../") && access != "." && access != ".." {
		access = "./" + access
	}
	access = path.Clean(access)
	if access == "." {
		access = "./"
	} else if access != ".." && !core.HasPrefix(access, "./") && !core.HasPrefix(access, "../") {
		access = "./" + access
	}
	mediumPath := core.TrimPrefix(access, "./")
	if mediumPath == "." {
		mediumPath = ""
	}
	return access, mediumPath, nil
}

func (rt *runtimeBindings) projectMediumPath(rel string) (coreio.Medium, string, error) {
	if rt == nil || rt.inst == nil {
		return nil, "", core.E("app.runtime.projectMediumPath", "nil runtime", nil)
	}
	if rt.inst.medium == coreio.Local {
		medium, err := coreio.NewSandboxed(rt.inst.Root)
		if err != nil {
			return nil, "", err
		}
		return medium, rel, nil
	}
	return rt.inst.medium, core.Path(rt.inst.Root, rel), nil
}

func (rt *runtimeBindings) processDir(raw string) (string, error) {
	raw = core.Trim(raw)
	if raw == "" {
		return rt.inst.Root, nil
	}
	_, rel, err := rt.normalisedProjectPath(raw)
	if err != nil {
		return "", err
	}
	if rt.inst.medium == coreio.Local {
		return core.Path(rt.inst.Root, rel), nil
	}
	return core.Path(rt.inst.Root, rel), nil
}

func resultValue(value any) core.Result {
	return core.Result{Value: value, OK: true}
}

func resultError(op, message string, err error) core.Result {
	return core.Result{
		Value: core.E(op, message, err),
		OK:    false,
	}
}

func stringSlice(result core.Result) []string {
	if !result.OK || result.Value == nil {
		return nil
	}
	switch value := result.Value.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMap(result core.Result) map[string]string {
	if !result.OK || result.Value == nil {
		return nil
	}
	switch value := result.Value.(type) {
	case map[string]string:
		out := make(map[string]string, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(value))
		for key, item := range value {
			out[key] = core.Sprint(item)
		}
		return out
	default:
		return nil
	}
}

func openBrowser(rawURL string) error {
	if rawURL == "" {
		return core.E("app.runtime.openBrowser", "url is required", nil)
	}
	cmd, err := platformCommand(rawURL, "open", "xdg-open", "rundll32", "url.dll,FileProtocolHandler")
	if err != nil {
		return err
	}
	return cmd.Run()
}

func sendNotification(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := "display notification " + appleScriptString(body) + " with title " + appleScriptString(title)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	default:
		return core.E("app.runtime.sendNotification", "notifications not supported on this host", nil)
	}
}

func readClipboard() (string, error) {
	cmd, err := platformCommand("", "pbpaste", "wl-paste", "powershell", "-Command", "Get-Clipboard")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", core.E("app.runtime.readClipboard", "clipboard command failed", err)
	}
	return core.TrimSuffix(string(out), "\n"), nil
}

func writeClipboard(text string) error {
	switch runtime.GOOS {
	case "darwin":
		return pipeCommand("pbcopy", text)
	case "linux":
		if err := pipeCommand("wl-copy", text); err == nil {
			return nil
		}
		return pipeCommand("xclip", text, "-selection", "clipboard")
	default:
		return core.E("app.runtime.writeClipboard", "clipboard write not supported on this host", nil)
	}
}

func runDialogConfirm(message string) (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, core.E("app.runtime.runDialogConfirm", "confirm dialog not supported on this host", nil)
	}
	if message == "" {
		message = "Continue?"
	}
	cmd := exec.Command(
		"osascript",
		"-e", "set resultButton to button returned of (display dialog "+appleScriptString(message)+" buttons {\"Cancel\", \"OK\"} default button \"OK\")",
		"-e", "resultButton",
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if core.As(err, &exitErr) {
			return false, nil
		}
		return false, core.E("app.runtime.runDialogConfirm", "osascript failed", err)
	}
	return core.Trim(string(out)) == "OK", nil
}

func runDialogOpen(title string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", core.E("app.runtime.runDialogOpen", "open dialog not supported on this host", nil)
	}
	if title == "" {
		title = "Choose a file"
	}
	out, err := exec.Command(
		"osascript",
		"-e", "set selectedFile to choose file with prompt "+appleScriptString(title),
		"-e", "POSIX path of selectedFile",
	).Output()
	if err != nil {
		return "", core.E("app.runtime.runDialogOpen", "osascript failed", err)
	}
	return core.Trim(string(out)), nil
}

func runDialogSave(title, defaultName string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", core.E("app.runtime.runDialogSave", "save dialog not supported on this host", nil)
	}
	if title == "" {
		title = "Save file"
	}
	if defaultName == "" {
		defaultName = "untitled"
	}
	out, err := exec.Command(
		"osascript",
		"-e", "set selectedFile to choose file name with prompt "+appleScriptString(title)+" default name "+appleScriptString(defaultName),
		"-e", "POSIX path of selectedFile",
	).Output()
	if err != nil {
		return "", core.E("app.runtime.runDialogSave", "osascript failed", err)
	}
	return core.Trim(string(out)), nil
}

func pipeCommand(name, input string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = core.NewReader(input)
	if err := cmd.Run(); err != nil {
		return core.E("app.runtime.pipeCommand", "command failed: "+name, err)
	}
	return nil
}

func platformCommand(argument, darwin, linux, windows string, windowsArgs ...string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		if argument == "" {
			return exec.Command(darwin), nil
		}
		return exec.Command(darwin, argument), nil
	case "linux":
		if argument == "" {
			return exec.Command(linux), nil
		}
		return exec.Command(linux, argument), nil
	case "windows":
		args := append([]string(nil), windowsArgs...)
		if argument != "" {
			args = append(args, argument)
		}
		return exec.Command(windows, args...), nil
	default:
		return nil, core.E("app.runtime.platformCommand", "unsupported host OS", nil)
	}
}

func appleScriptString(value string) string {
	return "\"" + core.Replace(value, "\"", "\\\"") + "\""
}
