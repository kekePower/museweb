# mod_museweb-ai Development Plan

This document outlines a staged technical roadmap for building **`mod_museweb-ai`**, an Apache HTTP Server module that brings MuseWeb-powered AI capabilities directly into the web-server layer.  We start with a minimal Proof-of-Concept (PoC) that you can compile and run today, then iterate toward a production-ready, feature-rich module.

---
## Table of Contents
1. Vision & Goals
2. Prerequisites & Tooling
3. Phase 1 – Proof-of-Concept (PoC)
4. Phase 2 – Core AI Integration
5. Phase 3 – Advanced Features & Hardening
6. Phase 4 – Packaging & Release
7. Long-Term Ideas

---
## 1  Vision & Goals
* **Inline AI Capabilities** – Expose MuseWeb-style AI endpoints (text generation, summarisation, etc.) as first-class Apache handlers and filters.
* **Drop-in Deployment** – Ship as a single shared object (`mod_museweb-ai.so`) installable through `apxs`/`LoadModule`.
* **Performance & Security** – Leverage Apache’s threading/event model, connection pools, and shared memory to achieve low-latency inference with strict resource isolation.
* **Extensibility** – Provide hooks so downstream sites can add custom directives without recompilation.

## 2  Prerequisites & Tooling
| Requirement | Notes |
|-------------|-------|
| Apache HTTP Server 2.4+ | With dev headers (`apache2-dev`/`httpd-devel`) |
| GCC/Clang | C11 or later |
| `apxs` | Ships with Apache dev package (used under-the-hood by Meson install step) |
| Meson & Ninja | Modern build system (`meson ≥ 0.63`, `ninja`) |
| MuseWeb gRPC/REST endpoint | Local or remote service providing AI (can reuse existing Go code) |
| **Optional** – `check` | C unit-testing framework |

---
## 3  Phase 1 – Proof-of-Concept (PoC)
Goal: deliver a minimal handler that returns “Hello from mod_museweb-ai” so the user can compile, load, and hit it via a browser.

### 3.1  Skeleton Module
1. Create `mod_museweb_ai.c` implementing:
   * `static int museweb_handler(request_rec *r)` – handles **POST** requests containing a JSON body `{ "prompt": "..." }` and returns AI completions.
   * Inside the handler, forward the prompt to an **OpenAI-compatible HTTP endpoint** (e.g., MuseWeb’s `/v1/chat/completions` or any local/remote server implementing the spec) using `apr_pool_t` + `apr_socket` or `libcurl`.
   * `static void museweb_register_hooks(apr_pool_t *p)` – registers `museweb_handler` for the `AI` content handler.
2. Provide configuration directives (Phase-1 scope):
   * `MuseWebAIEndpoint`  – OpenAI-compatible server URL (default `http://127.0.0.1:8080/v1/chat/completions`).
   * `MuseWebAIKey`       – API key/token if required.
3. Declare the module struct `module AP_MODULE_DECLARE_DATA museweb_module`.

### 3.2  Build & Install  (Meson/Ninja)
Create `meson.build` with custom target that calls `apxs` to compile the shared object and install it into Apache’s module directory automatically.

```bash
meson setup build
ninja -C build           # compiles mod_museweb-ai.so via apxs
sudo ninja -C build install   # copies .so to modules/ and adds LoadModule if missing
```
The `-a` flag auto-adds a `LoadModule` line to `httpd.conf`.

### 3.3  Configuration Snippet
```apache
# httpd.conf
# Ensure MuseWeb-AI handler is mapped
<Files "ai">
    SetHandler AI
</Files>
```
Or map by extension:
```apache
AddHandler AI .ai
```

### 3.4  Test
```bash
curl http://localhost/ai
# → "Hello from mod_museweb-ai PoC!"
```
If you see the greeting, the PoC works.

### 3.5  Deliverables
* `mod_museweb_ai.c`
* `meson.build`  (Meson/Ninja build definition)
* Quick-start README

---
## 4  Phase 2 – Core AI Integration
Expand the handler to forward requests to a MuseWeb AI backend.

### 4.1  Feature Scope
1. **Reverse-Proxy Style** – For each incoming request, proxy JSON payload to MuseWeb Go service, stream response back.
2. **Config Directives** –
   * `MuseWebEndpoint` – URL of backend (default `http://127.0.0.1:8080/generate`).
   * `MuseWebTimeout` – per-request timeout in ms.
3. **Error Handling** – Graceful 5xx mapping, logging.

### 4.2  Implementation Checklist
- Use `mod_proxy` utilities or raw `apr_socket` for HTTP calls.
- Parse JSON via `jansson` or `cJSON` (add as vendored submodule).
- Add directive parsing table and merge functions.

### 4.3  Milestone Exit Criteria
* Compile-time configurable endpoint.
* Round-trip latency ≤ 200 ms on localhost for 20 token completion.

---
## 5  Phase 3 – Advanced Features & Hardening
| Area | Features |
|------|----------|
| Performance | Connection pool, keep-alive, HTTP/2 push, shared memory cache of frequent prompts |
| Security | Rate limiting, request size limits, authentication tokens |
| Observability | Prometheus exporter, custom Apache log format, tracing via OpenTelemetry |
| Config UX | `MuseWebModel`, `MuseWebCacheTTL`, dynamic reload via `GracefulRestart` |
| Filters | Implement `output filter` for on-the-fly augmentation of HTML responses (e.g., summarise long articles) |
| Streaming | Support SSE / chunked transfer for live tokens |

---
## 6  Phase 4 – Packaging & Release
1. **CI** – GitHub Actions building on Ubuntu, CentOS, macOS.
2. **RPM/DEB Packages** – generate versioned packages.
3. **Semantic Versioning** – `v0.x` until first stable.
4. **Documentation** – Official docs site, examples, screencasts.
5. **Community** – CLA, contribution guide, issue templates.

---
## 7  Long-Term Ideas
* Native LLM inference via ONNX Runtime with GPU offload.
* Lua/Wasmtime hooks so users can script inference chains.
* Cluster-wide caching via memcached/redis.

---
### Timeline (T-shirt Sizing)
| Phase | Est. Duration |
|-------|--------------|
| 1 – PoC | 2 days |
| 2 – Core AI | 1-2 weeks |
| 3 – Advanced | 3-4 weeks |
| 4 – Packaging | 1 week |

---
## Appendix A  PoC Source (code snippet)
> NOTE: Full source lives in the repo. Shown here for reference.
```c
#include "httpd.h"
#include "http_protocol.h"
#include "http_config.h"
#include "ap_config.h"

static int museweb_handler(request_rec *r) {
    if (!r->handler || strcmp(r->handler, "AI")) {
        return DECLINED;
    }
    ap_set_content_type(r, "text/plain;charset=UTF-8");
    ap_rputs("Hello from mod_museweb-ai PoC!\n", r);
    return OK;
}
static void museweb_register_hooks(apr_pool_t *p) {
    ap_hook_handler(museweb_handler, NULL, NULL, APR_HOOK_MIDDLE);
}
module AP_MODULE_DECLARE_DATA museweb_module = {
    STANDARD20_MODULE_STUFF,
    NULL, NULL, NULL, NULL, NULL, museweb_register_hooks
};
```

You can now proceed to implement **Phase 1** and iterate!
