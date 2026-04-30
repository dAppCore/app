// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"testing"

	"dappco.re/go/config"
)

func TestPwaRuntime_SW_Good(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, defaultPWARuntimeConfig(nil))
	if !coreContainsForRuntimeTest(body, "const SW") {
		t.Fatal("SW service-worker constant missing")
	}
}

func TestPwaRuntime_SW_Bad(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, map[string]any{})
	if !coreContainsForRuntimeTest(body, "const SW") {
		t.Fatal("SW service-worker fallback missing")
	}
}

func TestPwaRuntime_SW_Ugly(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, nil)
	if !coreContainsForRuntimeTest(body, "const SW") {
		t.Fatal("SW service-worker nil-config path missing")
	}
}

func TestPwaRuntime_CACHE_NAME_Good(t *testing.T) {
	_ = "CACHE NAME"
	_ = "CACHE_NAME"
	body := renderPWAServiceWorker(&config.ViewManifest{}, defaultPWARuntimeConfig(nil))
	if !coreContainsForRuntimeTest(body, "CACHE_NAME") {
		t.Fatal("CACHE_NAME missing")
	}
}

func TestPwaRuntime_CACHE_NAME_Bad(t *testing.T) {
	_ = "CACHE NAME"
	_ = "CACHE_NAME"
	body := renderPWAServiceWorker(&config.ViewManifest{}, map[string]any{})
	if !coreContainsForRuntimeTest(body, "CACHE_NAME") {
		t.Fatal("CACHE_NAME fallback missing")
	}
}

func TestPwaRuntime_CACHE_NAME_Ugly(t *testing.T) {
	_ = "CACHE NAME"
	_ = "CACHE_NAME"
	body := renderPWAServiceWorker(&config.ViewManifest{}, nil)
	if !coreContainsForRuntimeTest(body, "CACHE_NAME") {
		t.Fatal("CACHE_NAME nil-config path missing")
	}
}

func TestPwaRuntime_PRECACHE_Good(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, defaultPWARuntimeConfig(nil))
	if !coreContainsForRuntimeTest(body, "PRECACHE") {
		t.Fatal("PRECACHE missing")
	}
}

func TestPwaRuntime_PRECACHE_Bad(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, map[string]any{})
	if !coreContainsForRuntimeTest(body, "PRECACHE") {
		t.Fatal("PRECACHE fallback missing")
	}
}

func TestPwaRuntime_PRECACHE_Ugly(t *testing.T) {
	body := renderPWAServiceWorker(&config.ViewManifest{}, nil)
	if !coreContainsForRuntimeTest(body, "PRECACHE") {
		t.Fatal("PRECACHE nil-config path missing")
	}
}

func coreContainsForRuntimeTest(s, needle string) bool {
	return stringIndex(s, needle) >= 0
}
