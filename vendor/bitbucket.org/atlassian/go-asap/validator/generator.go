package validator

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/SermoDigital/jose/jws"
	"github.com/SermoDigital/jose/jwt"
)

var kidRegex = regexp.MustCompile(`^[\w.\-\+/]*$`)

// GenerateValidator generates a validator for a given key id and audience.
func GenerateValidator(kid string, audience string) *jwt.Validator {
	validationFn := func(clientClaims jws.Claims) error {
		if err := checkMissingClaims(clientClaims); err != nil {
			return err
		}

		if issuer, _ := clientClaims.Issuer(); !validateKid(issuer, kid) {
			return fmt.Errorf("Invalid kid: %v", kid)
		}

		clientAudiences, _ := clientClaims.Audience()
		inAudience := false
		for _, aud := range clientAudiences {
			if strings.Compare(aud, audience) == 0 {
				inAudience = true
				break
			}
		}
		if !inAudience {
			return fmt.Errorf("Missing expected audience %v from JWT", audience)
		}

		issuedAt, _ := clientClaims.IssuedAt()
		expiration, _ := clientClaims.Expiration()

		if issuedAt.Add(time.Hour).Before(expiration) {
			return fmt.Errorf("iat %v is more than an hour before exp %v", issuedAt, expiration)
		}

		return nil
	}

	return jws.NewValidator(jws.Claims{}, 0, 0, validationFn)
}

func checkMissingClaims(claims jws.Claims) error {
	if _, p := claims.Issuer(); p == false {
		return errors.New("Missing iss from JWT")
	}
	if _, p := claims.Expiration(); p == false {
		return errors.New("Missing exp from JWT")
	}
	if _, p := claims.IssuedAt(); p == false {
		return errors.New("Missing iat from JWT")
	}
	if _, p := claims.Audience(); p == false {
		return errors.New("Missing aud from JWT")
	}
	if _, p := claims.JWTID(); p == false {
		return errors.New("Missing jti from JWT")
	}

	return nil
}

func validateKid(issuer, kid string) bool {
	if !strings.HasPrefix(kid, issuer+"/") {
		return false
	}

	for _, s := range strings.Split(kid, "/") {
		if s == "." || s == ".." {
			return false
		}
	}

	return kidRegex.MatchString(kid)
}
