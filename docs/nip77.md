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

## Private inboxes and mixed requests

Enabling NIP-17 protects kinds `1059` and `21059` without disabling public queries.
Wildcard, ID-only, mixed-kind, and multi-filter `REQ`s return the events the
connection may read. An anonymous request that combines products and gift wraps
still receives its public products and `EOSE`; it does not receive gift wraps.
The relay may issue an `AUTH` challenge alongside those public results. Only an
explicit request entirely for authentication-gated kinds is rejected with
`auth-required:`. After authenticating, clients can repeat the request to obtain
historical inbox events. Existing broad live subscriptions use the connection's
authenticated keys as they change.

Gift wraps require one canonical recipient `p` tag. With NIP-17 enabled, any key
successfully authenticated on the connection may read its own wrappers. Multiple
authenticated keys remain supported. Previously stored wrappers remain hidden
when NIP-17 is disabled. Kind `21059` is ephemeral and is protected on live
delivery; it does not provide stored history.

The same authorization applies to inbound Negentropy. Anonymous broad or mixed
filters reconcile public events; authenticated recipients can reconcile their
own stored `1059`s, including with an ID-only filter. Visibility is applied in
storage before query limits and record counts, and rechecked before constructing
the vector so protected IDs and timestamps are not exposed by reconciliation.

Publishing a valid signed gift wrap does not require recipient authentication.
The outer signing key is a random wrapper key, not proof of sender identity.
An operator can still explicitly configure `nip42.require_auth_publish_kinds`;
leave `1059` and `21059` out of that list to accept unauthenticated deliveries.
The subscription policy does not imply a publishing policy.

## REQ priority

NIP-77 is **best-effort background work**:

- NEG-OPEN DB loads run on a separate worker queue (not the REQ read queue)
- New NEG-OPEN requests are rejected when the REQ queue is deep (`backpressure_req_queue_depth`)
- NEG traffic uses separate rate limiters from REQ

## Upstream pull sync

Configure `nip77.upstreams` with `wss://` URLs, JSON filters, and `interval_seconds` (minimum 60). Congee connects as a negentropy client, reconciles ID sets, and imports missing events via `REQ`.

Each newly stored import is delivered to plugins whose listen subscriptions match the event kind (`OnStoredEvent` and/or `Observe` EVENT), the same match rules as a client `EVENT`. Imports are not run through the WebSocket EVENT validator chain. Duplicate IDs already in the store are skipped. On multi-instance PostgreSQL, other processes receive the same plugin notify via imported-event fanout (same-origin LISTEN is filtered, so the importer notifies plugins at persist time).

If the upstream sends a NIP-42 `["AUTH", challenge]` (on connect or during sync), Congee signs a kind-22242 AUTH event with **this relay’s** identity (`relay.secrets.json` / NIP-11 `self`) and replies. Relays that do not challenge are unchanged (a 2s wait after connect). The upstream may still reject AUTH if it only allows listed pubkeys.

Configured wildcard, mixed-kind, and gift-wrap upstream filters remain available.
Congee omits its local gift-wrap IDs and timestamps from the vector advertised
upstream, while reconciling public events normally. It can import gift wraps the
upstream offers and checks local presence before fetching an ID, so previously
stored wraps are not downloaded again. The upstream may advertise those known
IDs again on subsequent runs because they are intentionally absent from the
local advertised vector. A relay identity does not authenticate as an inbox
recipient; private upstream relays may withhold other users' messages. Sync can
recover only history the selected upstreams retain and authorize for this client.

## Observability

- **Logs**: `nip77 neg-open complete`, blocked sessions, upstream job results (`conn_id`, `sub_id`, `record_count`, `duration_ms`). Upstream NEG wait timeouts log `upstream negentropy message timeout` (`timeout_seconds`, `round`) then `upstream sync failed`.
- **Audit log**: `neg_open`, `neg_complete`, `neg_blocked`, `neg_err`, `neg_upstream_sync_*`
- **Metrics** (`GET /api/stats`): `neg_open_total`, `neg_msg_total`, `neg_blocked_total`, upstream import counters
- **Live connections** (`GET /api/audit/connections`): `total_neg_open`, `total_neg_msg`, open `neg_sessions`

## Limitations

- NIP-50 search filters are rejected for negentropy
- NIP-29 membership is rechecked before inbound vector construction; the preliminary record cap is applied before that membership check
- Persistent negentropy caches (strfry-style) are not implemented
