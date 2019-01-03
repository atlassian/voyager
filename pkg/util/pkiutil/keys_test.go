package pkiutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

func createMockKey(t *testing.T) (crypto.PrivateKey, string) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	keyID := "micros/voyager/test"
	return key, keyID
}

func TestEncodeThenDecode(t *testing.T) {
	t.Parallel()

	key, keyID := createMockKey(t)
	encodedKey, err := EncodePKCS8PrivateKey(key, keyID)
	require.NoError(t, err)

	decodedKey, err := DecodePKCS8PrivateKey(encodedKey)
	require.NoError(t, err)
	require.Equal(t, decodedKey, key)
}
