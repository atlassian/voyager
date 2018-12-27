package asap

import (
	"os"
	"time"

	asap "bitbucket.org/atlassian/go-asap"
	"bitbucket.org/atlassian/go-asap/keyprovider"
	"github.com/pkg/errors"
)

type Client struct {
	asap *asap.ASAP
	key  interface{}
}

func (a *Client) Sign(audience string, lifeTime time.Duration) ([]byte, error) {
	return a.LongSign(audience, lifeTime, a.key)
}

func New() (*Client, error) {
	key, err := privateKey()

	if err != nil {
		return nil, err
	}

	issuer, ok := os.LookupEnv("ASAP_ISSUER")

	if !ok {
		return nil, errors.New("ASAP_ISSUER not found")
	}

	keyID, ok := os.LookupEnv("ASAP_KEY_ID")

	if !ok {
		return nil, errors.New("ASAP_KEY_ID not found")
	}

	return &Client{
		asap: asap.NewASAP(keyID, issuer, nil),
		key:  key,
	}, nil
}

func privateKey() (interface{}, error) {
	kp := &keyprovider.EnvironmentPrivateKeyProvider{
		PrivateKeyEnvName: "ASAP_PRIVATE_KEY",
	}

	return kp.GetPrivateKey()
}
