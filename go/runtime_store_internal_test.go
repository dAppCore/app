// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	coreio "dappco.re/go/io"
	"golang.org/x/crypto/argon2"
)

func TestRuntimeStore_DeriveWorkspaceSecrets_PasswordMode(t *testing.T) {
	t.Setenv("CORE_WORKSPACE_KEY", "")
	t.Setenv("CORE_WORKSPACE_PASSWORD", "correct horse battery staple")

	ws := openRuntimeStoreTestWorkspace(t, "password-kdf")
	keys, err := deriveWorkspaceSecrets(ws)
	if err != nil {
		t.Fatalf("deriveWorkspaceSecrets password: %v", err)
	}
	assertWorkspaceSecretKeyShape(t, keys)

	if workspaceArgon2Memory != 64*1024 {
		t.Fatalf("workspaceArgon2Memory = %d; want 64 MiB", workspaceArgon2Memory)
	}
	if workspaceArgon2Time != 3 {
		t.Fatalf("workspaceArgon2Time = %d; want 3", workspaceArgon2Time)
	}
	if workspaceArgon2Parallelism != 4 {
		t.Fatalf("workspaceArgon2Parallelism = %d; want 4", workspaceArgon2Parallelism)
	}

	salt := runtimeStoreTestSalt(t, ws)
	master := argon2.IDKey(
		[]byte("correct horse battery staple"),
		salt,
		workspaceArgon2Time,
		workspaceArgon2Memory,
		workspaceArgon2Parallelism,
		workspaceSecretKeyBytes,
	)
	wantEnc := runtimeStoreTestHKDF(t, ws.Code, workspaceSecretMaterialPassword, "enc", master, salt)
	wantHMAC := runtimeStoreTestHKDF(t, ws.Code, workspaceSecretMaterialPassword, "hmac", master, salt)
	if string(keys.encryption) != string(wantEnc) {
		t.Fatal("password encryption key was not derived from Argon2id + HKDF-SHA256")
	}
	if string(keys.hmac) != string(wantHMAC) {
		t.Fatal("password HMAC key was not derived from Argon2id + HKDF-SHA256")
	}

	again, err := deriveWorkspaceSecrets(ws)
	if err != nil {
		t.Fatalf("deriveWorkspaceSecrets password again: %v", err)
	}
	if string(keys.encryption) != string(again.encryption) || string(keys.hmac) != string(again.hmac) {
		t.Fatal("password keys changed after persisted salt was written")
	}
}

func TestRuntimeStore_DeriveWorkspaceSecrets_KeyfileMode(t *testing.T) {
	t.Setenv("CORE_WORKSPACE_KEY", "ed25519-private-key-material")
	t.Setenv("CORE_WORKSPACE_PASSWORD", "ignored-password")

	ws := openRuntimeStoreTestWorkspace(t, "keyfile-kdf")
	keys, err := deriveWorkspaceSecrets(ws)
	if err != nil {
		t.Fatalf("deriveWorkspaceSecrets keyfile: %v", err)
	}
	assertWorkspaceSecretKeyShape(t, keys)

	salt := runtimeStoreTestSalt(t, ws)
	wantEnc := runtimeStoreTestHKDF(t, ws.Code, workspaceSecretMaterialKeyfile, "enc", []byte("ed25519-private-key-material"), salt)
	wantHMAC := runtimeStoreTestHKDF(t, ws.Code, workspaceSecretMaterialKeyfile, "hmac", []byte("ed25519-private-key-material"), salt)
	if string(keys.encryption) != string(wantEnc) {
		t.Fatal("keyfile encryption key was not derived with HKDF-SHA256")
	}
	if string(keys.hmac) != string(wantHMAC) {
		t.Fatal("keyfile HMAC key was not derived with HKDF-SHA256")
	}
}

func TestRuntimeStore_DeriveWorkspaceSecrets_SubKeySeparation(t *testing.T) {
	t.Setenv("CORE_WORKSPACE_KEY", "sub-key-source")
	t.Setenv("CORE_WORKSPACE_PASSWORD", "")

	ws := openRuntimeStoreTestWorkspace(t, "sub-key-separation")
	keys, err := deriveWorkspaceSecrets(ws)
	if err != nil {
		t.Fatalf("deriveWorkspaceSecrets subkeys: %v", err)
	}
	assertWorkspaceSecretKeyShape(t, keys)
	if string(keys.encryption) == string(keys.hmac) {
		t.Fatal("encryption and HMAC sub-keys must be distinct")
	}

	store := &workspaceObjectStore{enc: keys.encryption, hmac: keys.hmac}
	got := store.hashLocked("group", "prefs")
	wantHMAC := runtimeStoreTestHMAC(keys.hmac, "group", "prefs")
	wantEnc := runtimeStoreTestHMAC(keys.encryption, "group", "prefs")
	if got != wantHMAC {
		t.Fatalf("hashLocked used wrong HMAC key: got %s want %s", got, wantHMAC)
	}
	if got == wantEnc {
		t.Fatal("hashLocked reused the encryption key for HMAC")
	}
}

func openRuntimeStoreTestWorkspace(t *testing.T, code string) *Workspace {
	t.Helper()
	ws, err := OpenWorkspace(coreio.Local, t.TempDir(), code)
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	return ws
}

func runtimeStoreTestSalt(t *testing.T, ws *Workspace) []byte {
	t.Helper()
	if !ws.medium.IsFile(workspaceCryptoMetadataPath(ws)) {
		t.Fatalf("workspace crypto metadata was not persisted at %s", workspaceCryptoMetadataPath(ws))
	}
	metadata, err := readWorkspaceCryptoMetadata(ws)
	if err != nil {
		t.Fatalf("readWorkspaceCryptoMetadata: %v", err)
	}
	salt, err := decodeWorkspaceSecretSalt(metadata.Salt)
	if err != nil {
		t.Fatalf("decodeWorkspaceSecretSalt: %v", err)
	}
	if len(salt) != workspaceSecretSaltBytes {
		t.Fatalf("salt length = %d; want %d", len(salt), workspaceSecretSaltBytes)
	}
	return salt
}

func runtimeStoreTestHKDF(t *testing.T, code string, mode workspaceSecretMaterialMode, purpose string, material, salt []byte) []byte {
	t.Helper()
	info := "core.app.workspace.v1\x00" + string(mode) + "\x00" + purpose + "\x00" + code
	key, err := hkdf.Key(sha256.New, material, salt, info, workspaceSecretKeyBytes)
	if err != nil {
		t.Fatalf("HKDF: %v", err)
	}
	return key
}

func runtimeStoreTestHMAC(key []byte, scope, value string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func assertWorkspaceSecretKeyShape(t *testing.T, keys workspaceSecretKeys) {
	t.Helper()
	if len(keys.encryption) != workspaceSecretKeyBytes {
		t.Fatalf("encryption key length = %d; want %d", len(keys.encryption), workspaceSecretKeyBytes)
	}
	if len(keys.hmac) != workspaceSecretKeyBytes {
		t.Fatalf("HMAC key length = %d; want %d", len(keys.hmac), workspaceSecretKeyBytes)
	}
}
