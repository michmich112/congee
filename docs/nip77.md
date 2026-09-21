# NIP-77 (Negentropy Syncing)

Congee supports optional [NIP-77](https://github.com/nostr-protocol/nips/blob/master/77.md) negentropy syncing for efficient set reconciliation between clients and the relay, plus scheduled **upstream pull sync** from other relays.

## Enable

Add `77` to `nips.enabled` in config (or use **Admin → Config → Functionalities**):

```json
"nips": { "enabled": [1, 11, 77] }
```

Restart the relay after changing NIPs.

## Configuration (`nip77`)

| Field | Default | Purpose |
|-------|---------|---------|
| `max_records_per_query` | `100000` | Reject oversized sync queries (`0` = unlimited) |
| `session_idle_timeout_seconds` | `7` | Close inactive NEG sessions |
| `frame_size_limit_bytes` | `1048576` | Negentropy frame size limit |
| `max_concurrent_sessions` | `8` | Global open inbound NEG sessions |
| `max_concurrent_loads` | `2` | Concurrent DB vector builds |
| `neg_open_per_minute_per_connection` | `6` | Per-connection NEG-OPEN rate |
| `neg_msg_per_minute_per_connection` | `120` | Per-connection NEG-MSG rate |
| `backpressure_req_queue_depth` | `64` | Reject NEG-OPEN when REQ queue depth exceeds (`0` = off) |
| `upstream_enabled` | `true` | Master switch for scheduled upstream pull |
| `upstream_pause_when_busy` | `true` | Skip upstream jobs when relay is under REQ backpressure |
| `upstream_message_timeout_seconds` | `60` | How long to wait for each upstream `NEG-MSG` (including the first after `NEG-OPEN`). `0` = default 60. Does not apply to inbound sessions (`session_idle_timeout_seconds`) or post-sync `REQ` fetches. |
| `upstreams[]` | `[]` | Scheduled pull from other relays |

## Protocol (inbound)

Clients send:

- `NEG-OPEN` — start sync with filter + initial hex message
- `NEG-MSG` — continue reconciliation
- `NEG-CLOSE` — release session

The relay responds with `NEG-MSG` or `NEG-ERR`. After sync, clients use normal `REQ` / `EVENT` to transfer missing events.

## REQ priority

NIP-77 is **best-effort background work**:

- NEG-OPEN DB loads run on a separate worker queue (not the REQ read queue)
- New NEG-OPEN requests are rejected when the REQ queue is deep (`backpressure_req_queue_depth`)
- NEG traffic uses separate rate limiters from REQ

## Upstream pull sync

Configure `nip77.upstreams` with `wss://` URLs, JSON filters, and `interval_seconds` (minimum 60). Congee connects as a negentropy client, reconciles ID sets, and imports missing events via `REQ`.

Each newly stored import is delivered to plugins whose listen subscriptions match the event kind (`OnStoredEvent` and/or `Observe` EVENT), the same match rules as a client `EVENT`. Imports are not run through the WebSocket EVENT validator chain. Duplicate IDs already in the store are skipped. On multi-instance PostgreSQL, other processes receive the same plugin notify via imported-event fanout (same-origin LISTEN is filtered, so the importer notifies plugins at persist time).

If the upstream sends a NIP-42 `["AUTH", challenge]` (on connect or during sync), Congee signs a kind-22242 AUTH event with **this relay’s** identity (`relay.secrets.json` / NIP-11 pubkey) and replies. Relays that do not challenge are unchanged (a 2s wait after connect). The upstream may still reject AUTH if it only allows listed pubkeys.

## Observability

- **Logs**: `nip77 neg-open complete`, blocked sessions, upstream job results (`conn_id`, `sub_id`, `record_count`, `duration_ms`). Upstream NEG wait timeouts log `upstream negentropy message timeout` (`timeout_seconds`, `round`) then `upstream sync failed`.
- **Audit log**: `neg_open`, `neg_complete`, `neg_blocked`, `neg_err`, `neg_upstream_sync_*`
- **Metrics** (`GET /api/stats`): `neg_open_total`, `neg_msg_total`, `neg_blocked_total`, upstream import counters
- **Live connections** (`GET /api/audit/connections`): `total_neg_open`, `total_neg_msg`, open `neg_sessions`

## Limitations

- NIP-50 search filters are rejected for negentropy
- NIP-29 per-connection visibility is not applied during sync (DB contents matching the filter are used)
- Persistent negentropy caches (strfry-style) are not implemented
