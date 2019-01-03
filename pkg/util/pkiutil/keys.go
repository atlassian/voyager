package pkiutil

import (
	"crypto"
	"crypto/x509"

	"github.com/pkg/errors"
	"github.com/vincent-petithory/dataurl"
)

func DecodePKCS8PrivateKey(privateKey string) (crypto.PrivateKey, error) {
	dataURL, err := dataurl.DecodeString(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(dataURL.Data)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse private key as PKCS8")
	}

	return key, nil
}

func EncodePKCS8PrivateKey(privateKey crypto.PrivateKey, privateKeyID string) (string, error) {
	if privateKeyID == "" {
		return "", errors.New("private key ID was empty")
	}

	cert, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", errors.Wrap(err, "could not marshal private key")
	}

	return dataurl.New(cert, "application/pkcs8", "kid", privateKeyID).String(), nil
}
