package pkitest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"testing"

	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/stretchr/testify/assert"
	"github.com/vincent-petithory/dataurl"
)

func MockASAPClientConfig(t *testing.T) *pkiutil.ASAPClientConfig {
	return MockASAPClientConfigWithOptions(t, "voyager/test/id", "voyager/test")
}

func MockASAPClientConfigWithOptions(t *testing.T, keyID, issuer string) *pkiutil.ASAPClientConfig {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	marshalledKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	assert.NoError(t, err)
	key := dataurl.EncodeBytes(marshalledKey)
	config, err := pkiutil.NewASAPClientConfigFromEncodedKey(keyID, issuer, key)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	return config
}
