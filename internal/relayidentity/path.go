package relayidentity

import (
	"os"
	"path/filepath"
)

const defaultSecretsFileName = "relay.secrets.json"

// ResolvePath returns the relay secrets file path: RELAY_SECRETS_PATH if set,
// otherwise relay.secrets.json in the same directory as configFilePath.
func ResolvePath(configFilePath string) string {
	if p := os.Getenv("RELAY_SECRETS_PATH"); p != "" {
		return p
	}
	dir := filepath.Dir(configFilePath)
	if dir == "" || dir == "." {
		return defaultSecretsFileName
	}
	return filepath.Join(dir, defaultSecretsFileName)
}
