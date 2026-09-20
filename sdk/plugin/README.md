# Congee plugin SDK

Go module for Congee plugin binaries: gRPC/protobuf ABI (`congee.plugin.v1`), `Handler`, and `Serve`.

This is a **nested module**. It does **not** import the relay, Turso, admin UI, or `internal/`. Direct dependencies are gRPC and protobuf only.

```bash
go get github.com/michmich112/congee/sdk/plugin@v0.1.0
```

```go
import sdk "github.com/michmich112/congee/sdk/plugin"

func main() {
	if err := sdk.Serve(ctx, handler); err != nil {
		os.Exit(1)
	}
}
```

Handshake `api_version` must be **1**. Declare capabilities such as `sdk.CapIntercept`, `sdk.CapEventsRead`, `sdk.CapIndexOwn`, `sdk.CapAdminUI`.

Git tags for this module are prefixed: `sdk/plugin/vX.Y.Z`. A root Congee release tag (`v1.2.3`) does **not** version this SDK.

For local ABI work against a Congee checkout, use a `go.work` overlay or `replace` pointing at `./sdk/plugin`. Do not commit a `go.work` that assumes a missing sibling repo.
