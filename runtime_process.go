// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	coreerr "dappco.re/go/core/log"
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

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout bytes.Buffer
	stderr bytes.Buffer

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

func (procs *managedProcesses) runOnce(ctx context.Context, command string, args, env []string, dir string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdout := cmd.Stdout.(*bytes.Buffer)
	stderr := cmd.Stderr.(*bytes.Buffer)

	err := cmd.Run()
	exitCode := exitCodeOf(cmd.ProcessState)
	if err != nil {
		var exitErr *exec.ExitError
		if !errorAs(err, &exitErr) {
			return nil, coreerr.E("app.managedProcesses.runOnce", "start process failed", err)
		}
		if exitCode < 0 {
			exitCode = exitErr.ExitCode()
		}
	}
	return map[string]any{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"exit":   exitCode,
	}, nil
}

func (procs *managedProcesses) add(key, command string, args, env []string, dir string) error {
	if procs == nil {
		return coreerr.E("app.managedProcesses.add", "nil registry", nil)
	}
	if key == "" {
		return coreerr.E("app.managedProcesses.add", "key is required", nil)
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

func (procs *managedProcesses) start(ctx context.Context, key string) (bool, error) {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, coreerr.E("app.managedProcesses.start", "process not found: "+key, nil)
	}
	if entry.running {
		procs.mu.Unlock()
		return false, nil
	}

	cmd := exec.CommandContext(ctx, entry.command, entry.args...)
	cmd.Stdout = &entry.stdout
	cmd.Stderr = &entry.stderr
	if entry.dir != "" {
		cmd.Dir = entry.dir
	}
	if len(entry.env) > 0 {
		cmd.Env = append(os.Environ(), entry.env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		procs.mu.Unlock()
		return false, coreerr.E("app.managedProcesses.start", "open stdin failed", err)
	}
	if err := cmd.Start(); err != nil {
		procs.mu.Unlock()
		return false, coreerr.E("app.managedProcesses.start", "start failed", err)
	}

	entry.stdout.Reset()
	entry.stderr.Reset()
	entry.cmd = cmd
	entry.stdin = stdin
	entry.running = true
	entry.exit = -1
	procs.mu.Unlock()

	go procs.wait(key, cmd)
	return true, nil
}

func (procs *managedProcesses) wait(key string, cmd *exec.Cmd) {
	err := cmd.Wait()
	procs.mu.Lock()
	defer procs.mu.Unlock()
	entry, ok := procs.entries[key]
	if !ok || entry.cmd != cmd {
		return
	}
	entry.running = false
	entry.exit = exitCodeOf(cmd.ProcessState)
	if err != nil && entry.exit < 0 {
		entry.exit = -1
	}
	entry.cmd = nil
	entry.stdin = nil
}

func (procs *managedProcesses) stop(key string) (bool, error) {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, coreerr.E("app.managedProcesses.stop", "process not found: "+key, nil)
	}
	cmd := entry.cmd
	running := entry.running
	procs.mu.Unlock()
	if !running || cmd == nil || cmd.Process == nil {
		return false, nil
	}

	if runtime.GOOS == "windows" {
		return procs.kill(key)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return false, coreerr.E("app.managedProcesses.stop", "signal failed", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		procs.mu.Lock()
		stillRunning := procs.entries[key] != nil && procs.entries[key].running
		procs.mu.Unlock()
		if !stillRunning {
			return true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return procs.kill(key)
}

func (procs *managedProcesses) kill(key string) (bool, error) {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return false, coreerr.E("app.managedProcesses.kill", "process not found: "+key, nil)
	}
	cmd := entry.cmd
	running := entry.running
	procs.mu.Unlock()
	if !running || cmd == nil || cmd.Process == nil {
		return false, nil
	}
	if err := cmd.Process.Kill(); err != nil {
		return false, coreerr.E("app.managedProcesses.kill", "kill failed", err)
	}
	return true, nil
}

func (procs *managedProcesses) info(key string) (map[string]any, error) {
	procs.mu.Lock()
	defer procs.mu.Unlock()
	entry, ok := procs.entries[key]
	if !ok {
		return nil, coreerr.E("app.managedProcesses.info", "process not found: "+key, nil)
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

func (procs *managedProcesses) stdoutValue(key string) (string, error) {
	procs.mu.Lock()
	defer procs.mu.Unlock()
	entry, ok := procs.entries[key]
	if !ok {
		return "", coreerr.E("app.managedProcesses.stdoutValue", "process not found: "+key, nil)
	}
	return entry.stdout.String(), nil
}

func (procs *managedProcesses) writeStdin(key, data string) error {
	procs.mu.Lock()
	entry, ok := procs.entries[key]
	if !ok {
		procs.mu.Unlock()
		return coreerr.E("app.managedProcesses.writeStdin", "process not found: "+key, nil)
	}
	stdin := entry.stdin
	procs.mu.Unlock()
	if stdin == nil {
		return coreerr.E("app.managedProcesses.writeStdin", "stdin unavailable for "+key, nil)
	}
	if _, err := io.WriteString(stdin, data); err != nil {
		return coreerr.E("app.managedProcesses.writeStdin", "write failed", err)
	}
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

func exitCodeOf(state *os.ProcessState) int {
	if state == nil {
		return -1
	}
	return state.ExitCode()
}

func errorAs(err error, target any) bool {
	return errors.As(err, target)
}
