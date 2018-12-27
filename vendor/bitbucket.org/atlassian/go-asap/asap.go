package asap

import (
	cr "crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"bitbucket.org/atlassian/go-asap/validator"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/SermoDigital/jose/jwt"
	"github.com/satori/go.uuid"
)

const (
	// KeyID is the tag used in a JWT header for a key ID.
	KeyID     = "kid"
	algorithm = "alg"
)

// ASAP is used to manipulate JWTs.
type ASAP struct {
	ServiceID          string
	KeyID              string
	AuthorisedSubjects []string
}

// NewASAP returns a new *ASAP.
func NewASAP(keyIdentifier, serviceID string, authorisedSubjects []string) *ASAP {
	return &ASAP{
		KeyID:              keyIdentifier,
		ServiceID:          serviceID,
		AuthorisedSubjects: authorisedSubjects,
	}
}

func (asap *ASAP) makeClaims(audience string) jws.Claims {
	claims := jws.Claims{}
	asap.setAsapClaims(claims, audience)
	return claims
}

func (asap *ASAP) setAsapClaims(claims jws.Claims, audience string) {
	now := time.Now()
	jit := uuid.NewV4().String()
	exp := now.Add(time.Minute)

	claims.SetIssuer(asap.ServiceID)
	claims.SetJWTID(jit)
	claims.SetIssuedAt(now)
	claims.SetExpiration(exp)
	claims.SetAudience(audience)
}

func (asap *ASAP) signClaims(claims jws.Claims, privateKey cr.PrivateKey, signingMethod crypto.SigningMethod) (token []byte, err error) {
	jwt := jws.NewJWT(claims, signingMethod)

	// Need to hack the kid attribute into the right JWS header part, since jose
	// doesn't support adding to that yet.
	jwt.(jws.JWS).Protected().Set(KeyID, asap.KeyID)

	return jwt.Serialize(privateKey)
}

// Sign generates a signed JWT for a given audience.
func (asap *ASAP) Sign(audience string, privateKey cr.PrivateKey) (token []byte, err error) {
	return asap.SignCustomClaims(audience, jws.Claims{}, privateKey)
}

// SignCustomClaims generates a signed JWT for a given audience and with given custom claims.
func (asap *ASAP) SignCustomClaims(audience string, customClaims jws.Claims, privateKey cr.PrivateKey) (token []byte, err error) {
	var signingMethod crypto.SigningMethod

	switch privateKey.(type) {
	case *rsa.PrivateKey:
		signingMethod = crypto.SigningMethodRS256
	case *ecdsa.PrivateKey:
		signingMethod = crypto.SigningMethodES256
	default:
		return nil, errors.New("bad private key")
	}

	asap.setAsapClaims(customClaims, audience)
	return asap.signClaims(customClaims, privateKey, signingMethod)
}

// Parse parses a raw JWT into a jwt.JWT.
func (asap *ASAP) Parse(token []byte) (jwt.JWT, error) {
	return jws.ParseJWT(token)
}

// Validate validates a JWT against a given public key.
func (asap *ASAP) Validate(jwt jwt.JWT, publicKey cr.PublicKey) error {
	header := jwt.(jws.JWS).Protected()
	kid := header.Get(KeyID).(string)
	alg := header.Get(algorithm).(string)

	signingMethod, err := getSigningMethod(alg)
	if err != nil {
		return err
	}

	return jwt.Validate(publicKey, signingMethod, validator.GenerateValidator(kid, asap.ServiceID))
}

func getSigningMethod(alg string) (crypto.SigningMethod, error) {
	var sm crypto.SigningMethod
	switch alg {
	// ECDSA
	case "ES256":
		sm = crypto.SigningMethodES256
	case "ES384":
		sm = crypto.SigningMethodES384
	case "ES512":
		sm = crypto.SigningMethodES512
	// RSA
	case "RS256":
		sm = crypto.SigningMethodRS256
	case "RS384":
		sm = crypto.SigningMethodRS384
	case "RS512":
		sm = crypto.SigningMethodRS512
	default:
		return nil, fmt.Errorf("Unsupported algorithm: %s", alg)
	}

	return sm, nil
}
