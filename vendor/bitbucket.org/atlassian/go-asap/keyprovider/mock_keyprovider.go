package keyprovider

import (
	"crypto"
	"errors"
)

// MockKeyProvider mocks PublicKeyProvider and PrivateKeyProvider interfaces using in-memory keys.
type MockKeyProvider struct {
	PrivateKey crypto.PrivateKey
	PublicKeys map[string]crypto.PublicKey
	Err        error
}

// GetPublicKey gets a public key from an in-memory map.
func (kp *MockKeyProvider) GetPublicKey(keyID string) (crypto.PublicKey, error) {
	key, ok := kp.PublicKeys[keyID]
	if !ok {
		return nil, errors.New("couldn't find key")
	}

	return key, kp.Err
}

// GetPrivateKey gets an in-memory private key.
func (kp *MockKeyProvider) GetPrivateKey() (crypto.PrivateKey, error) {
	return kp.PrivateKey, kp.Err
}
