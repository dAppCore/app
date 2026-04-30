// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"strings"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

const (
	manifestConfigKeyPWA = "pwa"
	pwaServiceWorkerFile = "core-sw.js"
	pwaBootstrapFile     = "core-pwa.js"
)

func defaultPWARuntimeConfig(m *config.ViewManifest) map[string]any {
	code := "pwa-app"
	version := "0.1.0"
	if m != nil {
		if trimmed := core.Trim(m.Code); trimmed != "" {
			code = trimmed
		}
		if trimmed := core.Trim(string(m.Version)); trimmed != "" {
			version = trimmed
		}
	}
	return map[string]any{
		"bootstrap": map[string]any{
			"inject": true,
			"path":   "./" + pwaBootstrapFile,
		},
		"install_prompt": map[string]any{
			"enabled": true,
			"event":   "beforeinstallprompt",
		},
		"service_worker": map[string]any{
			"cache":            []any{"./core.json", "./" + pwaBootstrapFile},
			"cache_components": true,
			"enabled":          true,
			"path":             "./" + pwaServiceWorkerFile,
			"scope":            "./",
			"strategy":         "stale-while-revalidate",
		},
		"store_mirror": map[string]any{
			"database":      "core-pwa-" + code + "-" + version,
			"driver":        "indexeddb",
			"entries_store": "entries",
			"queue_store":   "pending",
		},
		"sync": map[string]any{
			"enabled":      true,
			"endpoint":     "./core-sync",
			"on_reconnect": true,
			"strategy":     "last-write-wins",
		},
	}
}

func ensurePWARuntimeConfig(m *config.ViewManifest) map[string]any {
	if m == nil {
		return nil
	}
	if m.Config == nil {
		m.Config = map[string]any{}
	}
	defaults := defaultPWARuntimeConfig(m)
	current, _ := m.Config[manifestConfigKeyPWA].(map[string]any)
	if current == nil {
		current = map[string]any{}
		m.Config[manifestConfigKeyPWA] = current
	}
	for key, value := range defaults {
		defaultMap, ok := value.(map[string]any)
		if !ok {
			if _, exists := current[key]; !exists {
				current[key] = value
			}
			continue
		}
		target, _ := current[key].(map[string]any)
		if target == nil {
			target = map[string]any{}
			current[key] = target
		}
		for childKey, childValue := range defaultMap {
			if _, exists := target[childKey]; !exists {
				target[childKey] = childValue
			}
		}
	}
	return current
}

func materializeWrappedRuntimeAssets(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if medium == nil {
		medium = coreio.Local
	}
	switch packageTypeFromManifest(manifest) {
	case PackageTypePWA:
		return materializePWARuntimeAssets(medium, dest, manifest)
	default:
		return nil
	}
}

func materializePWARuntimeAssets(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if manifest == nil {
		return core.E("app.materializePWARuntimeAssets", "nil manifest", nil)
	}
	if medium == nil {
		medium = coreio.Local
	}
	if dest == "" {
		return core.E("app.materializePWARuntimeAssets", "empty dest", nil)
	}

	pwaCfg := ensurePWARuntimeConfig(manifest)
	if pwaCfg == nil {
		return core.E("app.materializePWARuntimeAssets", "pwa config unavailable", nil)
	}

	for path, body := range map[string]string{
		core.Path(dest, pwaServiceWorkerFile): renderPWAServiceWorker(manifest, pwaCfg),
		core.Path(dest, pwaBootstrapFile):     renderPWABootstrap(manifest, pwaCfg),
	} {
		if err := medium.EnsureDir(core.PathDir(path)); err != nil {
			return core.E("app.materializePWARuntimeAssets", "ensure dir failed", err)
		}
		if err := medium.Write(path, body); err != nil {
			return core.E("app.materializePWARuntimeAssets", "write runtime asset failed", err)
		}
	}
	if err := injectPWABootstrap(medium, dest, manifest); err != nil {
		return err
	}
	return nil
}

func renderPWAServiceWorker(manifest *config.ViewManifest, pwaCfg map[string]any) string {
	payload := map[string]any{
		"code":    coalesce(manifest.Code, "pwa-app"),
		"version": coalesce(string(manifest.Version), "0.1.0"),
		"pwa":     pwaCfg,
	}
	body := core.JSONMarshalString(payload)
	return `const CORE_PWA = ` + body + `;
const SW = CORE_PWA.pwa && CORE_PWA.pwa.service_worker ? CORE_PWA.pwa.service_worker : {};
const CACHE_NAME = "core-pwa-" + CORE_PWA.code + "-" + CORE_PWA.version;
const PRECACHE = Array.isArray(SW.cache) ? SW.cache.filter(Boolean) : ["./core.json", "./core-pwa.js"];

async function seedCache() {
  const cache = await caches.open(CACHE_NAME);
  for (const asset of PRECACHE) {
    try {
      await cache.add(asset);
    } catch (_) {
    }
  }
}

function isBundlePath(pathname) {
  return pathname.endsWith("/core.json") ||
    pathname.endsWith(".js") ||
    pathname.endsWith(".mjs") ||
    pathname.endsWith(".css") ||
    pathname.endsWith(".html") ||
    pathname.includes("/components/");
}

self.addEventListener("install", (event) => {
  event.waitUntil(seedCache().then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys
      .filter((key) => key.startsWith("core-pwa-" + CORE_PWA.code + "-") && key !== CACHE_NAME)
      .map((key) => caches.delete(key)));
    await self.clients.claim();
  })());
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (!request || request.method !== "GET") {
    return;
  }
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || !isBundlePath(url.pathname)) {
    return;
  }

  event.respondWith((async () => {
    const cache = await caches.open(CACHE_NAME);
    const cached = await cache.match(request, {ignoreSearch: url.pathname.endsWith("/core.json")});
    const fresh = fetch(request).then((response) => {
      if (response && response.ok) {
        cache.put(request, response.clone()).catch(() => {});
      }
      return response;
    }).catch(() => null);

    if (cached) {
      event.waitUntil(fresh);
      return cached;
    }
    const response = await fresh;
    if (response) {
      return response;
    }
    return cached || Response.error();
  })());
});
`
}

func renderPWABootstrap(manifest *config.ViewManifest, pwaCfg map[string]any) string {
	payload := map[string]any{
		"code":    coalesce(manifest.Code, "pwa-app"),
		"name":    coalesce(manifest.Name, manifest.Code, "PWA"),
		"version": coalesce(string(manifest.Version), "0.1.0"),
		"pwa":     pwaCfg,
	}
	body := core.JSONMarshalString(payload)
	return `(() => {
  const CORE_PWA = ` + body + `;
  const PWA = CORE_PWA.pwa || {};
  const STORE = PWA.store_mirror || {};
  const SYNC = PWA.sync || {};
  const INSTALL = PWA.install_prompt || {};
  const SW = PWA.service_worker || {};
  const DB_NAME = STORE.database || ("core-pwa-" + CORE_PWA.code + "-" + CORE_PWA.version);
  const ENTRY_STORE = STORE.entries_store || "entries";
  const QUEUE_STORE = STORE.queue_store || "pending";
  let installPromptEvent = null;
  const installListeners = new Set();

  function emitInstallState() {
    installListeners.forEach((listener) => {
      try {
        listener(Boolean(installPromptEvent));
      } catch (_) {
      }
    });
  }

  function openDatabase() {
    return new Promise((resolve, reject) => {
      const request = indexedDB.open(DB_NAME, 1);
      request.onupgradeneeded = () => {
        const database = request.result;
        if (!database.objectStoreNames.contains(ENTRY_STORE)) {
          database.createObjectStore(ENTRY_STORE, {keyPath: "id"});
        }
        if (!database.objectStoreNames.contains(QUEUE_STORE)) {
          database.createObjectStore(QUEUE_STORE, {keyPath: "id", autoIncrement: true});
        }
      };
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
  }

  async function withStore(name, mode, work) {
    const db = await openDatabase();
    return new Promise((resolve, reject) => {
      const tx = db.transaction(name, mode);
      const store = tx.objectStore(name);
      let result;
      tx.oncomplete = () => resolve(result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
      Promise.resolve(work(store)).then((value) => {
        result = value;
      }).catch(reject);
    }).finally(() => db.close());
  }

  function recordId(group, key) {
    return group + "\u0000" + key;
  }

  async function getRecord(group, key) {
    return withStore(ENTRY_STORE, "readonly", (store) => new Promise((resolve, reject) => {
      const request = store.get(recordId(group, key));
      request.onsuccess = () => resolve(request.result || null);
      request.onerror = () => reject(request.error);
    }));
  }

  async function putRecord(group, key, value) {
    const now = Date.now();
    await withStore(ENTRY_STORE, "readwrite", (store) => store.put({
      id: recordId(group, key),
      group,
      key,
      value,
      updated_at: now,
    }));
    await withStore(QUEUE_STORE, "readwrite", (store) => store.add({
      group,
      key,
      type: "set",
      value,
      updated_at: now,
    }));
    return value;
  }

  async function deleteRecord(group, key) {
    const now = Date.now();
    await withStore(ENTRY_STORE, "readwrite", (store) => store.delete(recordId(group, key)));
    await withStore(QUEUE_STORE, "readwrite", (store) => store.add({
      group,
      key,
      type: "delete",
      updated_at: now,
    }));
    return true;
  }

  async function listPending() {
    return withStore(QUEUE_STORE, "readonly", (store) => new Promise((resolve, reject) => {
      const request = store.getAll();
      request.onsuccess = () => resolve(request.result || []);
      request.onerror = () => reject(request.error);
    }));
  }

  async function clearPending(ids) {
    if (!Array.isArray(ids) || ids.length === 0) {
      return;
    }
    await withStore(QUEUE_STORE, "readwrite", (store) => Promise.all(ids.map((id) => new Promise((resolve, reject) => {
      const request = store.delete(id);
      request.onsuccess = () => resolve();
      request.onerror = () => reject(request.error);
    }))));
  }

  function coalesceOperations(operations) {
    if (!Array.isArray(operations) || SYNC.strategy === "crdt") {
      return operations || [];
    }
    const latest = new Map();
    for (const operation of operations) {
      latest.set(recordId(operation.group, operation.key), operation);
    }
    return Array.from(latest.values()).sort((a, b) => (a.updated_at || 0) - (b.updated_at || 0));
  }

  async function flushPending() {
    if (!SYNC.enabled || !SYNC.on_reconnect || !navigator.onLine) {
      return {sent: 0, skipped: true};
    }
    const endpoint = typeof SYNC.endpoint === "string" ? SYNC.endpoint : "";
    if (!endpoint) {
      return {sent: 0, skipped: true};
    }
    const pending = await listPending();
    if (pending.length === 0) {
      return {sent: 0};
    }
    const operations = coalesceOperations(pending);
    const response = await fetch(endpoint, {
      method: "POST",
      headers: {"content-type": "application/json"},
      body: JSON.stringify({
        code: CORE_PWA.code,
        strategy: SYNC.strategy || "last-write-wins",
        operations,
      }),
    });
    if (!response.ok) {
      throw new Error("pwa sync failed: " + response.status);
    }
    const ids = pending.map((operation) => operation.id).filter((id) => typeof id === "number");
    await clearPending(ids);
    return {sent: operations.length, status: response.status};
  }

  async function mirrorLocalStorage() {
    if (!window.localStorage) {
      return;
    }
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (!key) {
        continue;
      }
      const value = window.localStorage.getItem(key);
      await withStore(ENTRY_STORE, "readwrite", (store) => store.put({
        id: recordId("browser.localStorage", key),
        group: "browser.localStorage",
        key,
        value,
        updated_at: Date.now(),
      }));
    }
  }

  if ("serviceWorker" in navigator && SW.enabled !== false) {
    window.addEventListener("load", () => {
      navigator.serviceWorker.register(SW.path || "./core-sw.js", {scope: SW.scope || "./"}).catch(() => {});
    }, {once: true});
  }

  if (INSTALL.enabled !== false) {
    window.addEventListener("beforeinstallprompt", (event) => {
      event.preventDefault();
      installPromptEvent = event;
      emitInstallState();
    });
  }

  window.addEventListener("online", () => {
    flushPending().catch(() => {});
  });
  window.addEventListener("storage", () => {
    mirrorLocalStorage().catch(() => {});
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && navigator.onLine) {
      flushPending().catch(() => {});
    }
  });

  window.CorePWA = {
    config: CORE_PWA,
    install: {
      available() {
        return Boolean(installPromptEvent);
      },
      async prompt() {
        if (!installPromptEvent) {
          return false;
        }
        installPromptEvent.prompt();
        const result = await installPromptEvent.userChoice.catch(() => null);
        installPromptEvent = null;
        emitInstallState();
        return !!(result && result.outcome === "accepted");
      },
      onChange(listener) {
        if (typeof listener !== "function") {
          return () => {};
        }
        installListeners.add(listener);
        listener(Boolean(installPromptEvent));
        return () => installListeners.delete(listener);
      },
    },
    store: {
      async get(group, key) {
        const record = await getRecord(group, key);
        return record ? record.value : null;
      },
      set: putRecord,
      delete: deleteRecord,
      pending: listPending,
      flush: flushPending,
    },
    sync: {
      flush: flushPending,
    },
  };

  mirrorLocalStorage().catch(() => {});
  if (navigator.onLine) {
    flushPending().catch(() => {});
  }
})();` + "\n"
}

func injectPWABootstrap(medium coreio.Medium, dest string, manifest *config.ViewManifest) error {
	if medium == nil {
		medium = coreio.Local
	}
	path := pwaBootstrapTarget(medium, dest, manifest)
	if path == "" {
		return nil
	}
	body, err := medium.Read(path)
	if err != nil {
		return core.E("app.injectPWABootstrap", "read html entry failed", err)
	}
	if core.Contains(body, "data-core-pwa") {
		return nil
	}

	tag := `<script src="./` + pwaBootstrapFile + `" data-core-pwa defer></script>`
	lower := core.Lower(body)
	if idx := strings.LastIndex(lower, "</head>"); idx >= 0 {
		body = body[:idx] + tag + "\n" + body[idx:]
	} else if idx := strings.LastIndex(lower, "</body>"); idx >= 0 {
		body = body[:idx] + tag + "\n" + body[idx:]
	} else {
		body += "\n" + tag + "\n"
	}
	if err := medium.Write(path, body); err != nil {
		return core.E("app.injectPWABootstrap", "write html entry failed", err)
	}
	return nil
}

func pwaBootstrapTarget(medium coreio.Medium, dest string, manifest *config.ViewManifest) string {
	candidates := []string{core.Path(dest, "index.html")}
	if manifest != nil && manifest.Config != nil {
		if raw, ok := manifest.Config["url"].(string); ok {
			for _, rel := range pwaBootstrapCandidatesFromURL(raw) {
				candidates = append(candidates, core.Path(dest, rel))
			}
		}
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		ext := core.Lower(core.PathExt(candidate))
		if ext != ".html" && ext != ".htm" {
			continue
		}
		if medium.Exists(candidate) {
			return candidate
		}
	}

	entries, err := medium.List(dest)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := core.Lower(entry.Name())
		if core.HasSuffix(name, ".html") || core.HasSuffix(name, ".htm") {
			return core.Path(dest, entry.Name())
		}
	}
	return ""
}

func pwaBootstrapCandidatesFromURL(raw string) []string {
	raw = core.Trim(raw)
	if raw == "" {
		return nil
	}
	path := raw
	if isLocalSource(raw) {
		path = trimLocalPrefix(raw)
	} else if core.Contains(raw, "://") {
		if parsed, ok := splitURLPath(raw); ok {
			path = parsed
		}
	}
	path = core.TrimPrefix(path, "/")
	path = core.TrimPrefix(path, "./")
	path = core.TrimPrefix(path, "../")
	if path == "" {
		return nil
	}
	out := []string{path}
	if core.PathExt(path) == "" {
		out = append(out, core.Path(path, "index.html"))
	}
	out = append(out, core.PathBase(path))
	return out
}

func splitURLPath(raw string) (string, bool) {
	for i := 0; i < len(raw); i++ {
		if raw[i] == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			rest := raw[i+2:]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				return rest[slash+1:], true
			}
			return "", false
		}
	}
	return "", false
}
