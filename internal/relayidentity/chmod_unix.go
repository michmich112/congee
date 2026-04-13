//go:build !windows

package relayidentity

import "os"

func chmod0600(path string) error {
	return os.Chmod(path, 0o600)
}
