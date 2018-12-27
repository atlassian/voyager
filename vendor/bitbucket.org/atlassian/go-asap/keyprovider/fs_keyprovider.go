package keyprovider

import (
	"crypto"
	"io/ioutil"
	"path/filepath"
)

// FSKeyProvider provides public and private keys stored on the filesystem.
type FSKeyProvider struct {
	PrivateKeyPath    string
	PublicKeyDir      string
	PublicKeyFilename string
}

// GetPrivateKey gets a private key stored at a known file.
func (kp *FSKeyProvider) GetPrivateKey() (crypto.PrivateKey, error) {
	privateKey, err := ioutil.ReadFile(kp.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	return privateKeyFromBytes(privateKey)
}

// GetPublicKey gets a public key stored in a known directory.
func (kp *FSKeyProvider) GetPublicKey(keyID string) (crypto.PublicKey, error) {
	path := filepath.Join(kp.PublicKeyDir, keyID)

	// Best case scenario, the keyID contains the file.
	publicKey, err := ioutil.ReadFile(path)

	// The public key filename is provided
	if err != nil && kp.PublicKeyFilename != "" {
		publicKey, err = ioutil.ReadFile(filepath.Join(path, kp.PublicKeyFilename))
	}

	// Can't get the public key
	if err != nil {
		return nil, err
	}

	return publicKeyFromBytes(publicKey)
}
