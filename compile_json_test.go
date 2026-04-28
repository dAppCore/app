// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"encoding/json"
	"testing"
)

func TestCompileJson_CompiledManifest_MarshalJSON_Good(t *testing.T) {
	cm := CompiledManifest{
		Code:       "json-good",
		Name:       "JSON Good",
		Version:    "0.1.0",
		CompiledAt: "1970-01-01T00:00:01Z",
		Config: map[string]any{
			"type": "pwa",
			"url":  "https://example.com/app",
		},
	}
	body, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if raw["type"] != "pwa" {
		t.Fatalf("top-level type = %v; want pwa", raw["type"])
	}
	if _, ok := raw["permissions"].(map[string]any); !ok {
		t.Fatalf("permissions field missing from compiled JSON: %s", string(body))
	}
}

func TestCompileJson_CompiledManifest_MarshalJSON_Bad(t *testing.T) {
	cm := CompiledManifest{
		Code:   "json-bad",
		Config: map[string]any{"bad": func() {}},
	}
	if _, err := json.Marshal(cm); err == nil {
		t.Fatal("MarshalJSON should reject non-JSON config values")
	}
}

func TestCompileJson_CompiledManifest_MarshalJSON_Ugly(t *testing.T) {
	cm := CompiledManifest{}
	body, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("MarshalJSON zero value: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("zero-value compiled manifest produced invalid JSON: %s", string(body))
	}
}

func TestCompileJson_CompiledManifest_UnmarshalJSON_Good(t *testing.T) {
	var cm CompiledManifest
	body := []byte(`{"code":"json-good","name":"JSON Good","version":"0.1.0","type":"web","permissions":{"net":["api.example.com:443"]}}`)
	if err := json.Unmarshal(body, &cm); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if cm.Code != "json-good" {
		t.Fatalf("Code = %q; want json-good", cm.Code)
	}
	if cm.Config["type"] != "web" {
		t.Fatalf("Config[type] = %v; want web", cm.Config["type"])
	}
	if len(cm.Permissions.Net) != 1 || cm.Permissions.Net[0] != "api.example.com:443" {
		t.Fatalf("Permissions.Net = %v; want api.example.com:443", cm.Permissions.Net)
	}
}

func TestCompileJson_CompiledManifest_UnmarshalJSON_Bad(t *testing.T) {
	var cm CompiledManifest
	if err := json.Unmarshal([]byte(`{"code":`), &cm); err == nil {
		t.Fatal("malformed JSON should fail")
	}
}

func TestCompileJson_CompiledManifest_UnmarshalJSON_Ugly(t *testing.T) {
	var cm *CompiledManifest
	if err := cm.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil receiver should fail")
	}
}
