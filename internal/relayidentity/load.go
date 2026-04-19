package relayidentity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/config"
)

var (
	errEmptySecretHex   = errors.New("relayidentity: secret_key_hex is empty")
	errInvalidSecretHex = errors.New("relayidentity: secret_key_hex is not valid hex")
	errWrongSecretLen   = errors.New("relayidentity: secret_key_hex must decode to 32 bytes")
)

type secretsFile struct {
	SecretKeyHex string `json:"secret_key_hex"`
}

// Load reads or creates the secrets file at path, then returns relay identity.
// If the file is missing, a new key is generated, written atomically, and chmod 0600 on Unix.
func Load(path string) (*Identity, error) {
	if path == "" {
		return nil, errors.New("relayidentity: empty secrets path")
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return generateAndSave(path)
		}
		return nil, fmt.Errorf("relayidentity: stat %s: %w", path, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("relayidentity: %s is a directory", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("relayidentity: read %s: %w", path, err)
	}
	var sf secretsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("relayidentity: json %s: %w", path, err)
	}
	priv, err := parseSecretKeyHex(sf.SecretKeyHex)
	if err != nil {
		return nil, fmt.Errorf("relayidentity: %s: %w", path, err)
	}
	return newIdentityFromPriv(priv)
}

func generateAndSave(path string) (*Identity, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("relayidentity: generate key: %w", err)
	}
	hexSecret := fmt.Sprintf("%x", priv.Serialize())
	body, err := json.Marshal(secretsFile{SecretKeyHex: hexSecret})
	if err != nil {
		return nil, err
	}
	if err := config.WriteFileAtomic(path, body); err != nil {
		return nil, fmt.Errorf("relayidentity: write %s: %w", path, err)
	}
	if err := chmod0600(path); err != nil {
		return nil, fmt.Errorf("relayidentity: chmod %s: %w", path, err)
	}
	return newIdentityFromPriv(priv)
}

// WriteTestSecrets writes a minimal secrets file (for tests). It does not load identity.
func WriteTestSecrets(path string, secretKeyHex string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	body, err := json.Marshal(secretsFile{SecretKeyHex: secretKeyHex})
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(path, body); err != nil {
		return err
	}
	return chmod0600(path)
}
