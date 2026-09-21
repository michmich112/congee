# Plugin architecture (gRPC host)

This document is the architecture context for **out-of-process** plugins. It is not the in-tree NIP registry (`internal/nips`).

Human and install detail lives in [plugins.md](plugins.md). Agent conventions: [AGENTS.md](../AGENTS.md) and [`.cursor/rules/plugins.mdc`](../.cursor/rules/plugins.mdc).

## Boundary

| Layer | Owns | Must not |
| --- | --- | --- |
| Host (`internal/plugin`, admin API, plugin settings **chrome**) | Process lifecycle, subscription match, listen queues, **REQ intercept**, fail-open, intercept log, iframe CSP | Call plugin code in-process; put admin tokens in the iframe |
| Plugin binary (`sdk/plugin` only) | Handshake, Observe / InterceptREQ / OnStoredEvent, indexes under `data/` | Import `github.com/michmich112/congee`; assume it sees fail-open intercepts |
| Plugin iframe (`GET /plugin-ui/{id}/`) | Plugin-owned settings UI | Fetch `/api/*` directly (`connect-src 'none'`, sandbox without `allow-same-origin`) |

The parent admin page bridges `postMessage` `{ type: "congee:plugin-api" }` to `/api/plugins/{id}/*` (settings, actions). It never puts `ADMIN_PASSWORD` in the iframe.

## Listen vs intercept

- **Listen** (`Observe`, `OnStoredEvent`): host matches, then non-blocking enqueue. Drops never fail the client.
- **Intercept** (`InterceptREQ`): the **only** synchronous plugin RPC. Host matches, then RPC with a deadline **before** `subs.Add`. Not ready, timeout, or RPC error → **fail-open passthrough**. The plugin is not invoked on those paths.

NIP-77 imported events use the same listen match as live ingest (`OnStoredEvent` / Observe EVENT). They skip the WebSocket EVENT validator chain.

## Intercept log is host-side (decision)

The rolling intercept log on the plugin settings page is implemented in **Congee**, not in Conduit or the SDK.

1. **Completeness.** The host is the only party that sees the full decision, including fail-open `not_ready` / `timeout` / `rpc`. A plugin-side log can only record RPCs that actually arrived.
2. **Non-blocking.** After intercept returns, the host copies REQ + action + response onto a logging goroutine (`select` / default drop, queue 256). The REQ path never waits on the log.
3. **Admin chrome, not the iframe.** The panel is Congee UI (`InterceptLogPanel`). The plugin iframe cannot call `GET/PUT /api/plugins/{id}/intercept-log`.
4. **No ABI / plugin republish.** Every intercepting plugin gets the same window. Do not add intercept-log RPCs to `plugin.proto` or put this panel inside a plugin UI.

Config: `plugins.intercept_log_size` (default **100**, `0` disables, max **10000**). In-memory only (lost on restart). Newest rows first. Host-wide limit, per-plugin buffers.

A plugin may still keep its own durable trace of RPCs it handled; that complements the host log and does not replace it.
