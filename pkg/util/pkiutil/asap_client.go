package pkiutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"

	asap "bitbucket.org/atlassian/go-asap"
	"github.com/SermoDigital/jose/jws"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/validation"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
)

type ASAP interface {
	GenerateToken(audience string, subject string) ([]byte, error)
	GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error)
	KeyID() string
	KeyIssuer() string
}

type ASAPClientConfig struct {
	PrivateKey   crypto.PrivateKey
	PrivateKeyID string `validate:"required"`
	Issuer       string `validate:"required"`
}

func NewASAPClientConfig(keyID, issuer string, key crypto.PrivateKey) (*ASAPClientConfig, error) {
	config := &ASAPClientConfig{
		PrivateKey:   key,
		PrivateKeyID: keyID,
		Issuer:       issuer,
	}

	v := validation.New()
	err := v.Validate(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate ASAPClientConfig")
	}

	err = config.validateKey()
	if err != nil {
		return nil, errors.Wrap(err, "could not validate private key, did you call the right constructor?")
	}

	return config, nil
}

const (
	dataKeyForID         = "id"
	dataKeyForIssuer     = "issuer"
	dataKeyForPrivateKey = "key"
)

func NewASAPClientConfigFromKubernetesSecret(secret *core_v1.Secret) (*ASAPClientConfig, error) {
	for _, field := range []string{dataKeyForPrivateKey, dataKeyForIssuer, dataKeyForID} {
		_, ok := secret.Data[field]
		if !ok {
			return nil, errors.Errorf("secret is missing %q", field)
		}
	}

	encodedKey := secret.Data[dataKeyForPrivateKey]

	key, err := DecodePKCS8PrivateKey(string(encodedKey))
	if err != nil {
		return nil, err
	}

	return NewASAPClientConfig(string(secret.Data[dataKeyForID]), string(secret.Data[dataKeyForIssuer]), key)
}

// NewASAPClientConfigFromEncodedKey is used when you have an encoded PKCS8 private key, encoded with dataurl
// e.g. `dataurl.New(marshalledPrivateKey, "application/pkcs8", "kid", keyID)`
func NewASAPClientConfigFromEncodedKey(keyID, issuer, encodedPrivateKey string) (*ASAPClientConfig, error) {
	key, err := DecodePKCS8PrivateKey(encodedPrivateKey)
	if err != nil {
		return nil, err
	}
	return NewASAPClientConfig(keyID, issuer, key)
}

func NewASAPClientConfigFromMicrosEnv() (ASAP, error) {
	const (
		envKeyID      = "ASAP_KEY_ID"
		envIssuer     = "ASAP_ISSUER"
		envPrivateKey = "ASAP_PRIVATE_KEY"
	)

	vars, err := util.EnvironmentVariablesAsMap(envKeyID, envIssuer, envPrivateKey)
	if err != nil {
		return nil, err
	}
	return NewASAPClientConfigFromEncodedKey(vars[envKeyID], vars[envIssuer], vars[envPrivateKey])
}

func (a *ASAPClientConfig) KeyID() string {
	return a.PrivateKeyID
}

func (a *ASAPClientConfig) KeyIssuer() string {
	return a.Issuer
}

func (a *ASAPClientConfig) GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error) {
	asap := asap.NewASAP(a.PrivateKeyID, a.Issuer, nil)

	if subject != "" {
		claims.SetSubject(subject)
	}

	token, err := asap.SignCustomClaims(audience, claims, a.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign custom claim containing subject")
	}
	return token, nil
}

// GenerateToken creates a token with an audience claim and a subject claim. If subject is not provided, it will not be part of the claim.
func (a *ASAPClientConfig) GenerateToken(audience string, subject string) ([]byte, error) {
	return a.GenerateTokenWithClaims(audience, subject, jws.Claims{})
}

// validateKey validates that the privateKey by attempting to sign a dummy request
func (a *ASAPClientConfig) validateKey() error {
	token, err := a.GenerateToken("audience", "subject")

	if err != nil {
		return errors.Wrap(err, "failed to sign dummy request")
	}

	if len(token) == 0 {
		return errors.New("ASAP header value failed to generate")
	}

	return nil
}

func (a *ASAPClientConfig) PublicKey() (crypto.PublicKey, error) {
	switch k := a.PrivateKey.(type) {
	case *rsa.PrivateKey:
		return k.Public(), nil
	case *ecdsa.PrivateKey:
		return k.Public(), nil
	default:
		return nil, errors.New("private key type is not supported (only rsa, ecdsa are supported)")
	}
}
