// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"dappco.re/go/io/sigil"
)

// TestRuntimeStore_EncryptDecryptRoundTripWithSigil rescued from primary
// checkout (was untracked since 2026-04-25). Verifies the workspace
// object store's encryptLocked / decryptLocked round-trip via a
// ChaChaPoly + ShuffleMaskObfuscator sigil.
func TestRuntimeStore_EncryptDecryptRoundTripWithSigil(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, workspaceSecretKeyBytes)
	cipherSigil, err := sigil.NewChaChaPolySigil(key, &sigil.ShuffleMaskObfuscator{})
	if err != nil {
		t.Fatalf("NewChaChaPolySigil: %v", err)
	}

	store := &workspaceObjectStore{sigil: cipherSigil}
	value := "value-plaintext-check-123456"
	encoded, err := store.encryptLocked(value)
	if err != nil {
		t.Fatalf("encryptLocked: %v", err)
	}
	if encoded == value {
		t.Fatal("encryptLocked produced plaintext")
	}
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("encrypted output is not base64: %v", err)
	}
	plain, err := store.decryptLocked(encoded)
	if err != nil {
		t.Fatalf("decryptLocked: %v", err)
	}
	if plain != value {
		t.Fatalf("round-trip mismatch: want %q, got %q", value, plain)
	}
	// Sanity — ensure the encoded form is not just a base64 of the value.
	if strings.Contains(encoded, base64.StdEncoding.EncodeToString([]byte(value))) {
		t.Fatal("encoded form contains plaintext base64 — encryption is a no-op")
	}
}
