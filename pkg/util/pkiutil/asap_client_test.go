package pkiutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	minRSAKeySize = 768
	keyID         = "micros/test/id"
	issuer        = "micros/test"
)

func checkErrorBuilder(t *testing.T) func(crypto.PrivateKey, error) crypto.PrivateKey {
	return func(key crypto.PrivateKey, err error) crypto.PrivateKey {
		require.NoError(t, err)
		return key
	}
}

func TestNewASAPConfigFromEncodedKeyInvalid(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, minRSAKeySize)
	require.NoError(t, err)
	encodedPrivateKey, err := EncodePKCS8PrivateKey(key, keyID)
	require.NoError(t, err)

	testCases := []struct {
		PrivateKeyID      string
		Issuer            string
		EncodedPrivateKey string
		Description       string
	}{
		{keyID, issuer, "", "NewASAPClientConfigFromEncodedKey with missing private key"},
		{keyID, issuer, "foo", "NewASAPClientConfigFromEncodedKey with malformed data URL"},
		{keyID, "", encodedPrivateKey, "NewASAPClientConfigFromEncodedKey with missing issuer"},
		{"", issuer, encodedPrivateKey, "NewASAPClientConfigFromEncodedKey with missing key id"},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			_, err := NewASAPClientConfigFromEncodedKey(tc.PrivateKeyID, tc.Issuer, tc.EncodedPrivateKey)
			require.Error(t, err)
		})
	}
}

func TestNewASAPConfigFromEncodedKeyValid(t *testing.T) {
	t.Parallel()

	encode := func(key crypto.PrivateKey, err error) string {
		require.NoError(t, err)
		encoded, err := EncodePKCS8PrivateKey(key, keyID)
		require.NoError(t, err)
		return encoded
	}

	testCases := []struct {
		PrivateKeyID      string
		Issuer            string
		EncodedPrivateKey string
		Description       string
	}{
		{keyID, issuer, encode(rsa.GenerateKey(rand.Reader, minRSAKeySize)), "NewASAPClientConfigFromEncodedKey with RSA key"},
		{keyID, issuer, encode(ecdsa.GenerateKey(elliptic.P256(), rand.Reader)), "NewASAPClientConfigFromEncodedKey with ECDSA key"},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			_, err := NewASAPClientConfigFromEncodedKey(tc.PrivateKeyID, tc.Issuer, tc.EncodedPrivateKey)
			require.NoError(t, err)
		})
	}
}

func TestNewASAPConfigInvalid(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, minRSAKeySize)
	require.NoError(t, err)

	testCases := []struct {
		PrivateKeyID string
		Issuer       string
		PrivateKey   crypto.PrivateKey
		Description  string
	}{
		{keyID, issuer, "", "NewASAPClientConfig with missing private key"},
		{keyID, issuer, "foo", "NewASAPClientConfig with malformed data URL"},
		{keyID, "", key, "NewASAPClientConfig with missing issuer"},
		{"", issuer, key, "NewASAPClientConfig with missing key id"},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			_, err := NewASAPClientConfig(tc.PrivateKeyID, tc.Issuer, tc.PrivateKey)
			require.Error(t, err)
		})
	}
}

func TestNewASAPConfigValid(t *testing.T) {
	t.Parallel()

	checkEm := checkErrorBuilder(t)

	testCases := []struct {
		PrivateKeyID string
		Issuer       string
		PrivateKey   crypto.PrivateKey
		Description  string
	}{
		{keyID, issuer, checkEm(rsa.GenerateKey(rand.Reader, minRSAKeySize)), "NewASAPClientConfig with RSA key"},
		{keyID, issuer, checkEm(ecdsa.GenerateKey(elliptic.P256(), rand.Reader)), "NewASAPClientConfig with ECDSA key"},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			_, err := NewASAPClientConfig(tc.PrivateKeyID, tc.Issuer, tc.PrivateKey)
			require.NoError(t, err)
		})
	}
}

func TestPublicKey(t *testing.T) {
	t.Parallel()

	checkEm := checkErrorBuilder(t)

	testCases := []struct {
		PrivateKeyID string
		Issuer       string
		PrivateKey   crypto.PrivateKey
		Description  string
	}{
		{keyID, issuer, checkEm(rsa.GenerateKey(rand.Reader, minRSAKeySize)), "NewASAPClientConfig with RSA key"},
		{keyID, issuer, checkEm(ecdsa.GenerateKey(elliptic.P256(), rand.Reader)), "NewASAPClientConfig with ECDSA key"},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			config := &ASAPClientConfig{
				PrivateKey:   tc.PrivateKey,
				PrivateKeyID: tc.PrivateKeyID,
				Issuer:       tc.Issuer,
			}

			_, err := config.PublicKey()
			require.NoError(t, err)
		})
	}
}

func TestNewASAPClientConfigFromKubernetesSecret(t *testing.T) {
	t.Parallel()

	// given
	// testdata generated with `generate_asap_key kube/voyager-creator delete.me asap-creator paas-dev1-ap-southeast-2`
	data, loadErr := testutil.LoadFileFromTestData("voyager-creator.yaml")
	require.NoError(t, loadErr)
	decode := scheme.Codecs.UniversalDeserializer().Decode
	fakeSecret := &v1.Secret{}
	_, _, decodeErr := decode(data, nil, fakeSecret)
	require.NoError(t, decodeErr)

	// when
	asapConfig, err := NewASAPClientConfigFromKubernetesSecret(fakeSecret)
	require.NoError(t, err)

	// The tests in 'then' apply to RSA, but not all crypto-systems.
	_, ok := asapConfig.PrivateKey.(*rsa.PrivateKey)
	require.True(t, ok)

	// then
	// The public key generated from the private key that was loaded should match the public key that was loaded.
	publicKey, err := asapConfig.PublicKey()
	require.NoError(t, err)
	encodedPublicKey, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	var pemBytes = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: encodedPublicKey,
	}
	encodedPemBlock := pem.EncodeToMemory(pemBytes)

	assert.Equal(t, []byte(encodedPemBlock), fakeSecret.Data["public"])
}
