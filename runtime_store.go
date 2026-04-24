// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"sync"

	enchantrix "forge.lthn.ai/Snider/Enchantrix/pkg/enchantrix"

	core "dappco.re/go/core"
	corestore "dappco.re/go/io/store"
	coreerr "dappco.re/go/log"
)

const workspaceStoreDBName = "store.db"

// workspaceObjectStore is the runtime object-store adapter backed by the
// SQLite KV store shipped in core/io/store. Group + key identifiers are
// hashed, and values are encrypted through Enchantrix before they hit disk.
type workspaceObjectStore struct {
	ws *Workspace

	mu    sync.Mutex
	kv    *corestore.KeyValueStore
	key   []byte
	sigil *enchantrix.ChaChaPolySigil
}

func newWorkspaceObjectStore(ws *Workspace) *workspaceObjectStore {
	return &workspaceObjectStore{ws: ws}
}

func (store *workspaceObjectStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.kv == nil {
		return nil
	}
	err := store.kv.Close()
	store.kv = nil
	store.key = nil
	store.sigil = nil
	return err
}

func (store *workspaceObjectStore) Get(group, key string) (string, error) {
	if group == "" || key == "" {
		return "", coreerr.E("app.workspaceObjectStore.Get", "group and key are required", nil)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensureLocked(); err != nil {
		return "", err
	}
	value, err := store.kv.Get(store.hashLocked("group", group), store.hashLocked("key", group+":"+key))
	if err != nil {
		return "", coreerr.E("app.workspaceObjectStore.Get", "read failed", err)
	}
	decoded, err := store.decryptLocked(value)
	if err != nil {
		return "", coreerr.E("app.workspaceObjectStore.Get", "decrypt failed", err)
	}
	return decoded, nil
}

func (store *workspaceObjectStore) Set(group, key, value string) error {
	if group == "" || key == "" {
		return coreerr.E("app.workspaceObjectStore.Set", "group and key are required", nil)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensureLocked(); err != nil {
		return err
	}
	encoded, err := store.encryptLocked(value)
	if err != nil {
		return coreerr.E("app.workspaceObjectStore.Set", "encrypt failed", err)
	}
	if err := store.kv.Set(store.hashLocked("group", group), store.hashLocked("key", group+":"+key), encoded); err != nil {
		return coreerr.E("app.workspaceObjectStore.Set", "write failed", err)
	}
	return nil
}

func (store *workspaceObjectStore) Delete(group, key string) error {
	if group == "" || key == "" {
		return coreerr.E("app.workspaceObjectStore.Delete", "group and key are required", nil)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensureLocked(); err != nil {
		return err
	}
	if err := store.kv.Delete(store.hashLocked("group", group), store.hashLocked("key", group+":"+key)); err != nil {
		return coreerr.E("app.workspaceObjectStore.Delete", "delete failed", err)
	}
	return nil
}

func (store *workspaceObjectStore) ensureLocked() error {
	if store == nil {
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "nil store", nil)
	}
	if store.ws == nil {
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "workspace unavailable", nil)
	}
	if store.kv != nil && store.sigil != nil && len(store.key) == 32 {
		return nil
	}

	secret, err := deriveWorkspaceSecret(store.ws)
	if err != nil {
		return err
	}
	dbPath := store.ws.Resolve(WorkspaceLayoutStore, workspaceStoreDBName)
	kv, err := corestore.New(corestore.Options{Path: dbPath})
	if err != nil {
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "open store database failed", err)
	}
	sigil, err := enchantrix.NewChaChaPolySigilWithObfuscator(secret, &enchantrix.ShuffleMaskObfuscator{})
	if err != nil {
		_ = kv.Close()
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "initialise sigil failed", err)
	}

	store.kv = kv
	store.key = secret
	store.sigil = sigil
	return nil
}

func (store *workspaceObjectStore) hashLocked(scope, value string) string {
	mac := hmac.New(sha256.New, store.key)
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (store *workspaceObjectStore) encryptLocked(value string) (string, error) {
	ciphertext, err := store.sigil.In([]byte(value))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (store *workspaceObjectStore) decryptLocked(value string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	plaintext, err := store.sigil.Out(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func deriveWorkspaceSecret(ws *Workspace) ([]byte, error) {
	if ws == nil {
		return nil, coreerr.E("app.deriveWorkspaceSecret", "nil workspace", nil)
	}

	switch secret := core.Trim(core.Env("CORE_WORKSPACE_KEY")); secret {
	case "":
	default:
		return deriveWorkspaceSecretMaterial(ws.Code, []byte(secret)), nil
	}
	switch password := core.Trim(core.Env("CORE_WORKSPACE_PASSWORD")); password {
	case "":
	default:
		return deriveWorkspaceSecretMaterial(ws.Code, []byte(password)), nil
	}

	home := workspaceHomeFromRoot(ws.Root)
	if home == "" {
		return nil, coreerr.E("app.deriveWorkspaceSecret", "cannot resolve workspace home", nil)
	}
	priv, err := ensureDefaultPrivateKey(ws.medium, home)
	if err != nil {
		return nil, coreerr.E("app.deriveWorkspaceSecret", "resolve workspace key failed", err)
	}
	return deriveWorkspaceSecretMaterial(ws.Code, []byte(priv)), nil
}

func deriveWorkspaceSecretMaterial(code string, material []byte) []byte {
	h := sha256.New()
	h.Write([]byte("core.app.workspace"))
	h.Write([]byte{0})
	h.Write(material)
	h.Write([]byte{0})
	h.Write([]byte(code))
	sum := h.Sum(nil)
	out := make([]byte, len(sum))
	copy(out, sum)
	return out
}

func workspaceHomeFromRoot(root string) string {
	root = core.Trim(root)
	if root == "" {
		return ""
	}
	// <home>/.core/data/<code>
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(root))))
}
