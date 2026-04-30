// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"context"
	"sync"

	core "dappco.re/go"
)

type managedProcesses struct {
	mu      sync.Mutex
	entries map[string]*managedProcess
}

type managedProcess struct {
	key     string
	command string
	args    []string
	dir     string
	env     []string

	stdin  core.Writer
	stdout string
	stderr string

	running bool
	exit    int
}

func newManagedProcesses() *managedProcesses {
	return &managedProcesses{entries: map[string]*managedProcess{}}
}

func (procs *managedProcesses) shutdown() {
	if procs == nil {
		return
	}
	procs.mu.Lock()
	entries := make([]*managedProcess, 0, len(procs.entries))
	for _, entry := range procs.entries {
		entries = append(entries, entry)
	}
	procs.mu.Unlock()

	for _, entry := range entries {
		_, _ = procs.kill(entry.key)
	}
}

func (
	procs *managedProcesses,
) runOnce(ctx context.Context, command string, args, env []string, dir string) (map[string]any, error) {
	_ = ctx
	_ = command
	_ = args
	_ = env
	_ = dir
	return map[string]any{
		"stdout": "",
		"stderr": "process execution unavailable",
		"exit":   1,
	}, nil
}

func (
	procs *managedProcesses,
) add(key, command string, args, env []string, dir string) error {
	if procs == nil {
		return core.E("app.managedProcesses.add", "nil registry", nil)
	}
	if key == "" {
		return core.E("app.managedProcesses.add", "key is required", nil)
	}
	procs.mu.Lock()
	defer procs.mu.Unlock()
	procs.entries[key] = &managedProcess{
		key:     key,
		command: command,
		args:    append([]string(nil), args...),
		dir:     dir,
		env:     append([]string(nil), env...),
		exit:    -1,
	}
	return nil
}

func (
	procs *managedProcesses,
) start(ctx context.Context, key string) (bool, error) {
	_ = ctx
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, core.E("app.managedProcesses.start", "process not found: "+key, nil)
	}
	if entry.running {
		procs.mu.Unlock()
		return false, nil
	}
	entry.stdout = ""
	entry.stderr = "process execution unavailable"
	entry.running = true
	entry.exit = -1
	procs.mu.Unlock()
	return true, nil
}

func (
	procs *managedProcesses,
) stop(key string) (bool, error) {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, core.E("app.managedProcesses.stop", "process not found: "+key, nil)
	}
	running := entry.running
	if running {
		entry.running = false
		entry.exit = 0
	}
	procs.mu.Unlock()
	if !running {
		return false, nil
	}
	return true, nil
}

func (
	procs *managedProcesses,
) kill(key string) (bool, error) {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, core.E("app.managedProcesses.kill", "process not found: "+key, nil)
	}
	running := entry.running
	if running {
		entry.running = false
		entry.exit = -1
	}
	procs.mu.Unlock()
	if !running {
		return false, nil
	}
	return true, nil
}

func (
	procs *managedProcesses,
) info(key string) (map[string]any, error) {
	procs.mu.Lock()
	defer procs.mu.Unlock()
	entry, ok := procs.entries[key]
	if !ok {
		return nil, core.E("app.managedProcesses.info", "process not found: "+key, nil)
	}
	return entry.info(), nil
}

func (procs *managedProcesses) list() []string {
	if procs == nil {
		return nil
	}
	procs.mu.Lock()
	defer procs.mu.Unlock()
	out := make([]string, 0, len(procs.entries))
	for key := range procs.entries {
		out = append(out, key)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func (
	procs *managedProcesses,
) stdoutValue(key string) (string, error) {
	procs.mu.Lock()
	defer procs.mu.Unlock()
	entry, ok := procs.entries[key]
	if !ok {
		return "", core.E("app.managedProcesses.stdoutValue", "process not found: "+key, nil)
	}
	return entry.stdout, nil
}

func (
	procs *managedProcesses,
) writeStdin(key, data string) error {
	procs.mu.Lock()
	_, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return core.E("app.managedProcesses.writeStdin", "process not found: "+key, nil)
	}
	procs.mu.Unlock()
	_ = data
	return nil
}

func (entry *managedProcess) info() map[string]any {
	if entry == nil {
		return map[string]any{}
	}
	return map[string]any{
		"key":     entry.key,
		"command": entry.command,
		"running": entry.running,
		"exit":    entry.exit,
	}
}

func errorAs(
	err error, target any,
) bool {
	return core.As(err, target)
}
