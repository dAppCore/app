// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"
)

func TestRuntimeStore_ObjectStore_Close_Good(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "close-good"))
	if err := store.Set("prefs", "theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.kv != nil || store.sigil != nil || store.enc != nil || store.hmac != nil {
		t.Fatal("Close did not clear store internals")
	}
}

func TestRuntimeStore_ObjectStore_Close_Bad(t *testing.T) {
	var store *workspaceObjectStore
	if err := store.Close(); err != nil {
		t.Fatalf("nil Close should be a no-op: %v", err)
	}
}

func TestRuntimeStore_ObjectStore_Close_Ugly(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "close-ugly"))
	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close should be idempotent: %v", err)
	}
}

func TestRuntimeStore_ObjectStore_Get_Good(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "get-good"))
	if err := store.Set("prefs", "theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("prefs", "theme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "dark" {
		t.Fatalf("Get = %q; want dark", got)
	}
}

func TestRuntimeStore_ObjectStore_Get_Bad(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "get-bad"))
	if _, err := store.Get("", "theme"); err == nil {
		t.Fatal("empty group should fail")
	}
	if _, err := store.Get("prefs", ""); err == nil {
		t.Fatal("empty key should fail")
	}
}

func TestRuntimeStore_ObjectStore_Get_Ugly(t *testing.T) {
	store := &workspaceObjectStore{}
	if _, err := store.Get("prefs", "theme"); err == nil {
		t.Fatal("store without workspace should fail")
	}
}

func TestRuntimeStore_ObjectStore_Set_Good(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "set-good"))
	if err := store.Set("prefs", "theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("prefs", "theme")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != "dark" {
		t.Fatalf("stored value = %q; want dark", got)
	}
}

func TestRuntimeStore_ObjectStore_Set_Bad(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "set-bad"))
	if err := store.Set("", "theme", "dark"); err == nil {
		t.Fatal("empty group should fail")
	}
	if err := store.Set("prefs", "", "dark"); err == nil {
		t.Fatal("empty key should fail")
	}
}

func TestRuntimeStore_ObjectStore_Set_Ugly(t *testing.T) {
	store := &workspaceObjectStore{}
	if err := store.Set("prefs", "theme", "dark"); err == nil {
		t.Fatal("store without workspace should fail")
	}
}

func TestRuntimeStore_ObjectStore_Delete_Good(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "delete-good"))
	if err := store.Set("prefs", "theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete("prefs", "theme"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("prefs", "theme"); err == nil {
		t.Fatal("deleted key should not read back")
	}
}

func TestRuntimeStore_ObjectStore_Delete_Bad(t *testing.T) {
	store := newWorkspaceObjectStore(openRuntimeStoreTestWorkspace(t, "delete-bad"))
	if err := store.Delete("", "theme"); err == nil {
		t.Fatal("empty group should fail")
	}
	if err := store.Delete("prefs", ""); err == nil {
		t.Fatal("empty key should fail")
	}
}

func TestRuntimeStore_ObjectStore_Delete_Ugly(t *testing.T) {
	store := &workspaceObjectStore{}
	if err := store.Delete("prefs", "theme"); err == nil {
		t.Fatal("store without workspace should fail")
	}
}
