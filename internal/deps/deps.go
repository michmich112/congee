// Package deps records direct module requirements for Phase 2 scaffolding.
// Narrow or remove this file once application packages import these modules.
package deps

import (
	_ "github.com/btcsuite/btcd/btcec/v2"
	_ "github.com/gobwas/ws"
	_ "github.com/onsi/ginkgo/v2"
	_ "github.com/onsi/gomega"
	_ "github.com/rs/zerolog"
	_ "github.com/uptrace/bun"
	_ "github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)
