// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coreio "dappco.re/go/io"
)

func TestEnsureWorkspaceSecretSalt_Concurrent(t *testing.T) {
	const workers = 10

	ws, err := OpenWorkspace(coreio.Local, t.TempDir(), "concurrent-salt")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}

	originalOpen := openWorkspaceCryptoMetadataFile
	originalWrite := writeWorkspaceCryptoMetadataBody
	t.Cleanup(func() {
		openWorkspaceCryptoMetadataFile = originalOpen
		writeWorkspaceCryptoMetadataBody = originalWrite
	})

	var attempts int64
	var creates int64
	var exists int64
	var writes int64
	allAttemptingCreate := make(chan struct{})

	openWorkspaceCryptoMetadataFile = func(path string, flag int, perm os.FileMode) (*os.File, error) {
		if atomic.AddInt64(&attempts, 1) == workers {
			close(allAttemptingCreate)
		}
		select {
		case <-allAttemptingCreate:
		case <-time.After(5 * time.Second):
		}

		file, err := originalOpen(path, flag, perm)
		if err == nil {
			atomic.AddInt64(&creates, 1)
		}
		if errors.Is(err, fs.ErrExist) {
			atomic.AddInt64(&exists, 1)
		}
		return file, err
	}
	writeWorkspaceCryptoMetadataBody = func(w io.Writer, body string) error {
		atomic.AddInt64(&writes, 1)
		return originalWrite(w, body)
	}

	start := make(chan struct{})
	salts := make([][]byte, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			<-start
			salts[i], errs[i] = ensureWorkspaceSecretSalt(ws)
		}(i)
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("ensureWorkspaceSecretSalt[%d]: %v", i, err)
		}
	}
	for i, salt := range salts {
		if len(salt) != workspaceSecretSaltBytes {
			t.Fatalf("salt[%d] length = %d; want %d", i, len(salt), workspaceSecretSaltBytes)
		}
		if !bytes.Equal(salts[0], salt) {
			t.Fatalf("salt[%d] differed from salt[0]", i)
		}
	}

	if got := atomic.LoadInt64(&attempts); got != workers {
		t.Fatalf("atomic create attempts = %d; want %d", got, workers)
	}
	if got := atomic.LoadInt64(&creates); got != 1 {
		t.Fatalf("successful atomic creates = %d; want 1", got)
	}
	if got := atomic.LoadInt64(&exists); got != workers-1 {
		t.Fatalf("EEXIST retries = %d; want %d", got, workers-1)
	}
	if got := atomic.LoadInt64(&writes); got != 1 {
		t.Fatalf("filesystem writes = %d; want 1", got)
	}
}
