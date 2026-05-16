// Store and DB logging helpers for the relay (see LogStoreErr).
//
// Severity policy (relay-facing):
//   - Error: relay degraded or client received a misleading hard failure (e.g. REQ internal error path).
//   - Warn: policy, overload, or secondary failures worth tailing at default log level.
//   - Debug: parse noise, per-hot-path visibility checks, and companion detail for store errors.
package relay

import (
	"github.com/rs/zerolog"
)

// LogStoreErr logs a store/DB failure at primaryLevel (typically Warn or Error) with operation and err,
// then emits a Debug line with the same err when debugFields is non-nil (must add extra context only there).
func LogStoreErr(log zerolog.Logger, primaryLevel zerolog.Level, operation string, err error, msg string, debugFields func(*zerolog.Event)) {
	if err == nil {
		return
	}
	log.WithLevel(primaryLevel).Err(err).Str("operation", operation).Msg(msg)
	if debugFields != nil {
		e := log.Debug().Err(err).Str("operation", operation)
		debugFields(e)
		e.Msg(msg + " detail")
	}
}
