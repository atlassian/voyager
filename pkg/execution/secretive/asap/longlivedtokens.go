package asap

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"time"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

const KeyID = "kid"

func (a *Client) makeClaims(audience string, lifeTime time.Duration) jws.Claims {
	now := time.Now()
	jit := uuid.NewV4().String()
	exp := now.Add(lifeTime)

	claims := jws.Claims{}
	claims.SetIssuer(a.asap.ServiceID)
	claims.SetJWTID(jit)
	claims.SetIssuedAt(now)
	claims.SetExpiration(exp)
	claims.SetAudience(audience)

	return claims
}

func (a *Client) signClaims(claims jws.Claims, privateKey interface{}, signingMethod crypto.SigningMethod) (token []byte, err error) {
	jwt := jws.NewJWT(claims, signingMethod)

	// Need to hack the kid attribute into the right JWS header part, since jose
	// doesn't support adding to that yet.
	jwt.(jws.JWS).Protected().Set(KeyID, a.asap.KeyID)

	return jwt.Serialize(privateKey)
}

func (a *Client) LongSign(audience string, lifeTime time.Duration, privateKey interface{}) (token []byte, err error) {

	var signingMethod crypto.SigningMethod

	switch privateKey.(type) {
	case *rsa.PrivateKey:
		signingMethod = crypto.SigningMethodRS256
	case *ecdsa.PrivateKey:
		signingMethod = crypto.SigningMethodES256
	default:
		return nil, errors.New("bad private key")
	}

	claims := a.makeClaims(audience, lifeTime)
	return a.signClaims(claims, privateKey, signingMethod)
}
