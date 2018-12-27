package keyprovider

import (
	"crypto"
	"errors"
	"os"

	"github.com/vincent-petithory/dataurl"
)

// EnvironmentPrivateKeyProvider provides private keys from a given environment variable.
type EnvironmentPrivateKeyProvider struct {
	PrivateKeyEnvName string
}

// GetPrivateKey gets a private key from the given environment variable.
func (kp *EnvironmentPrivateKeyProvider) GetPrivateKey() (crypto.PrivateKey, error) {
	if kp.PrivateKeyEnvName == "" {
		return nil, errors.New("no environment variable")
	}

	privateKey := os.Getenv(kp.PrivateKeyEnvName)
	if privateKey == "" {
		return nil, errors.New("environment variable not set")
	}

	dataURL, err := dataurl.DecodeString(privateKey)
	if err == nil {
		return privateKeyFromBytes(dataURL.Data)
	}

	return privateKeyFromBytes([]byte(privateKey))
}
