// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/hkdf" // Note: AX-6 - structural KDF primitive for workspace key separation; no core HKDF wrapper exists.
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/io/sigil"
	corestore "dappco.re/go/io/store"
	coreerr "dappco.re/go/log"
	"golang.org/x/crypto/argon2" // Note: AX-6 - password KDF primitive required to slow offline workspace-password attacks.
)

const workspaceStoreDBName = "store.db"

const (
	workspaceSecretKeyBytes = 32

	workspaceArgon2Memory      uint32 = 64 * 1024
	workspaceArgon2Time        uint32 = 3
	workspaceArgon2Parallelism uint8  = 4
)

type workspaceSecretMaterialMode string

const (
	workspaceSecretMaterialKeyfile  workspaceSecretMaterialMode = "keyfile"
	workspaceSecretMaterialPassword workspaceSecretMaterialMode = "password"
)

type workspaceSecretKeys struct {
	encryption []byte
	hmac       []byte
}

// workspaceObjectStore is the runtime object-store adapter backed by the
// SQLite KV store shipped in core/io/store. Group + key identifiers are
// hashed, and values are encrypted through Sigil before they hit disk.
type workspaceObjectStore struct {
	ws *Workspace

	mu    sync.Mutex
	kv    *corestore.KeyValueStore
	enc   []byte
	hmac  []byte
	sigil *sigil.ChaChaPolySigil
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
	store.enc = nil
	store.hmac = nil
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
	if store.kv != nil && store.sigil != nil &&
		len(store.enc) == workspaceSecretKeyBytes &&
		len(store.hmac) == workspaceSecretKeyBytes {
		return nil
	}

	keys, err := deriveWorkspaceSecrets(store.ws)
	if err != nil {
		return err
	}
	dbPath := store.ws.Resolve(WorkspaceLayoutStore, workspaceStoreDBName)
	kv, err := corestore.New(corestore.Options{Path: dbPath})
	if err != nil {
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "open store database failed", err)
	}
	cipherSigil, err := sigil.NewChaChaPolySigil(keys.encryption, &sigil.ShuffleMaskObfuscator{})
	if err != nil {
		if closeErr := kv.Close(); closeErr != nil {
			return coreerr.E("app.workspaceObjectStore.ensureLocked", "initialise sigil failed and close failed", err)
		}
		return coreerr.E("app.workspaceObjectStore.ensureLocked", "initialise sigil failed", err)
	}

	store.kv = kv
	store.enc = keys.encryption
	store.hmac = keys.hmac
	store.sigil = cipherSigil
	return nil
}

func (store *workspaceObjectStore) hashLocked(scope, value string) string {
	mac := hmac.New(sha256.New, store.hmac)
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
	keys, err := deriveWorkspaceSecrets(ws)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(keys.encryption))
	copy(out, keys.encryption)
	return out, nil
}

func deriveWorkspaceSecrets(ws *Workspace) (workspaceSecretKeys, error) {
	if ws == nil {
		return workspaceSecretKeys{}, coreerr.E("app.deriveWorkspaceSecrets", "nil workspace", nil)
	}
	salt, err := ensureWorkspaceSecretSalt(ws)
	if err != nil {
		return workspaceSecretKeys{}, coreerr.E("app.deriveWorkspaceSecrets", "resolve workspace salt failed", err)
	}

	switch secret := core.Trim(core.Env("CORE_WORKSPACE_KEY")); secret {
	case "":
	default:
		return deriveWorkspaceSecretMaterial(ws.Code, workspaceSecretMaterialKeyfile, []byte(secret), salt)
	}
	switch password := core.Trim(core.Env("CORE_WORKSPACE_PASSWORD")); password {
	case "":
	default:
		return deriveWorkspaceSecretMaterial(ws.Code, workspaceSecretMaterialPassword, []byte(password), salt)
	}

	home := workspaceHomeFromRoot(ws.Root)
	if home == "" {
		return workspaceSecretKeys{}, coreerr.E("app.deriveWorkspaceSecrets", "cannot resolve workspace home", nil)
	}
	priv, err := ensureDefaultPrivateKey(ws.medium, home)
	if err != nil {
		return workspaceSecretKeys{}, coreerr.E("app.deriveWorkspaceSecrets", "resolve workspace key failed", err)
	}
	return deriveWorkspaceSecretMaterial(ws.Code, workspaceSecretMaterialKeyfile, []byte(priv), salt)
}

func deriveWorkspaceSecretMaterial(code string, mode workspaceSecretMaterialMode, material, salt []byte) (workspaceSecretKeys, error) {
	switch mode {
	case workspaceSecretMaterialKeyfile:
		return deriveWorkspaceSecretSubKeys(code, mode, material, salt)
	case workspaceSecretMaterialPassword:
		master := argon2.IDKey(
			material,
			salt,
			workspaceArgon2Time,
			workspaceArgon2Memory,
			workspaceArgon2Parallelism,
			workspaceSecretKeyBytes,
		)
		return deriveWorkspaceSecretSubKeys(code, mode, master, salt)
	default:
		return workspaceSecretKeys{}, coreerr.E("app.deriveWorkspaceSecretMaterial", "unknown secret material mode", nil)
	}
}

func deriveWorkspaceSecretSubKeys(code string, mode workspaceSecretMaterialMode, material, salt []byte) (workspaceSecretKeys, error) {
	enc, err := deriveWorkspaceSecretSubKey(code, mode, "enc", material, salt)
	if err != nil {
		return workspaceSecretKeys{}, err
	}
	mac, err := deriveWorkspaceSecretSubKey(code, mode, "hmac", material, salt)
	if err != nil {
		return workspaceSecretKeys{}, err
	}
	return workspaceSecretKeys{encryption: enc, hmac: mac}, nil
}

func deriveWorkspaceSecretSubKey(code string, mode workspaceSecretMaterialMode, purpose string, material, salt []byte) ([]byte, error) {
	info := "core.app.workspace.v1\x00" + string(mode) + "\x00" + purpose + "\x00" + code
	key, err := hkdf.Key(sha256.New, material, salt, info, workspaceSecretKeyBytes)
	if err != nil {
		return nil, coreerr.E("app.deriveWorkspaceSecretSubKey", "HKDF-SHA256 failed", err)
	}
	return key, nil
}

func workspaceHomeFromRoot(root string) string {
	root = core.Trim(root)
	if root == "" {
		return ""
	}
	// <home>/.core/data/<code>
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(root))))
}
