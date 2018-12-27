package keyprovider

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

func privateKeyFromBytes(privateKeyData []byte) (crypto.PrivateKey, error) {
	keyFromDataURL, err := x509.ParsePKCS8PrivateKey(privateKeyData)
	if err == nil {
		return keyFromDataURL, nil
	}

	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return nil, errors.New("No valid PEM data found")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, err
	}

	return x509.ParseECPrivateKey(block.Bytes)
}

func publicKeyFromBytes(publicKeyData []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(publicKeyData)
	if block == nil {
		return nil, errors.New("No valid PEM data found")
	}

	return x509.ParsePKIXPublicKey(block.Bytes)
}
