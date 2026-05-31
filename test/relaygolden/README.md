# Relay behavior-parity golden harness

Captures relay WebSocket outputs (`EVENT`, `OK`, `EOSE`, `CLOSED`, `AUTH`, `NOTICE`) and compares them to checked-in fixtures after normalizing relay-generated fields (design **R9**).

## Normalization (R9)

Before diffing, `NormalizeMessage` rewrites unstable fields:

| Field | When rewritten |
|-------|----------------|
| Event `id` | 64-char hex ids not listed in `NormalizeOpts.StableEventIDs` |
| Event `sig` | Always → `<SIG>` |
| Event `created_at` | Always → `<CREATED_AT>` |
| `["AUTH", challenge]` | Challenge → `<CHALLENGE>` |
| `["OK", eventID, …]` | Event id when not in `StableEventIDs` |

Client-signed events built with fixed `created_at` in tests should register their computed `id` in `StableEventIDs` so OK/EVENT lines stay deterministic.

## Adding a case per NIP

1. **Create a scenario test** in `test/relaygolden/<nip>_test.go` (or extend an existing file).
2. **Seed storage** (if needed) and **start** the relay with `Start` / `StartWithIdentity`.
3. **Dial** WebSocket, send client frames (`EVENT`, `REQ`, …), collect replies with `ReadAll`.
4. **Normalize** with `NormalizeLines(frames, NormalizeOpts{StableEventIDs: …})`.
5. **Compare** to `testdata/<case>.golden` using `AssertGolden` (or `-update` to refresh fixtures).

Example skeleton:

```go
func TestGoldenNIP01EventOK(t *testing.T) {
    // open store, build cfg with nips.enabled including your NIP
    srv, err := relaygolden.Start(cfg, st, zerolog.Nop())
    // defer srv.Close()
    c, _, _ := websocket.DefaultDialer.Dial(srv.WSURL, nil)
    // send EVENT / REQ, ReadAll, NormalizeLines, AssertGolden
}
```

6. **Document the NIP** in the test name and fixture path (e.g. `testdata/nip50_search.golden`).
7. When porting a NIP to the plugin registry, run the same scenario before/after and refresh fixtures only if intentional behavior changed.

## Updating fixtures

```bash
go test ./test/relaygolden/... -run TestGolden -update
```

(Tests that support `-update` pass it via a shared flag in the package test file.)
