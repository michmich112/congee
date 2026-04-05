// Package version holds the relay binary version string embedded at link time.
package version

// Version is the running relay release (NIP-11 "version" field). Override when building, e.g.:
//
//	go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=1.2.3" ./cmd/congee
var Version = "0.0.0-dev"
