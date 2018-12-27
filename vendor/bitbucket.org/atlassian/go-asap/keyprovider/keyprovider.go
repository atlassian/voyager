package keyprovider

import "crypto"

// PublicKeyProvider provides public keys given a keyID.
type PublicKeyProvider interface {
	GetPublicKey(keyID string) (crypto.PublicKey, error)
}

// PrivateKeyProvider provides a fixed private key.
type PrivateKeyProvider interface {
	GetPrivateKey() (crypto.PrivateKey, error)
}
