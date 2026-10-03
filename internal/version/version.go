// Package version holds the relay binary version string embedded at link time.
package version

// Version is the running relay release (NIP-11 "version" field and admin UI).
// The repo-root VERSION file is the release number. Builds override this default, e.g.:
//
//	go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=1.2.3" ./cmd/congee
//
// CI stamps the plain VERSION on main, and appends -rc or -nightly for those image tags.
// Exact git SHA is also on the OCI image revision label.
var Version = "0.0.0-dev"
