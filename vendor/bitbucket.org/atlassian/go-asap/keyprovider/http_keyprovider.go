package keyprovider

import (
	"crypto"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/gregjones/httpcache"
)

const (
	noCacheControl      = "no-cache"
	defaultCacheControl = "private, max-age=600"
)

var defaultClient = &http.Client{
	Timeout:   time.Second * 2,
	Transport: httpcache.NewMemoryCacheTransport(), // Respect HTTP cache control headers
}

// HTTPPublicKeyProvider provides public keys served by an external HTTP server.
type HTTPPublicKeyProvider struct {
	BaseURL           string
	Client            *http.Client
	CacheTTLInSeconds int
}

// GetPublicKey gets a public key with the given keyID from an HTTP endpoint.
func (kp *HTTPPublicKeyProvider) GetPublicKey(keyID string) (crypto.PublicKey, error) {
	pkURL, err := url.Parse(kp.BaseURL)
	if err != nil {
		return nil, err
	}
	pkURL.Path = path.Join(pkURL.Path, keyID)

	netClient := kp.Client
	if netClient == nil {
		netClient = defaultClient
	}

	req, err := http.NewRequest(http.MethodGet, pkURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Cache-Control", cacheControl(kp))

	resp, err := netClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET '%s' returned status code %d", pkURL.String(), resp.StatusCode)
	}

	publicKey, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return publicKeyFromBytes(publicKey)
}

// ------------------------------------------ PRIVATES ------------------------------------------

func cacheControl(kp *HTTPPublicKeyProvider) string {
	if kp.CacheTTLInSeconds < 0 {
		return noCacheControl
	} else if kp.CacheTTLInSeconds == 0 {
		return defaultCacheControl
	} else {
		return fmt.Sprintf("private, max-age=%d", kp.CacheTTLInSeconds)
	}
}
