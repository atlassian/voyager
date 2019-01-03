package pkiutil

import (
	"crypto"

	"bitbucket.org/atlassian/go-asap/keyprovider"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type MirroredPublicKeyProvider struct {
	logger   *zap.Logger
	primary  keyprovider.PublicKeyProvider
	fallback keyprovider.PublicKeyProvider
}

func NewMirroredPublicKeyProvider(logger *zap.Logger, primary, fallback keyprovider.PublicKeyProvider) *MirroredPublicKeyProvider {
	return &MirroredPublicKeyProvider{
		logger:   logger,
		primary:  primary,
		fallback: fallback,
	}
}

func (m *MirroredPublicKeyProvider) GetPublicKey(keyID string) (crypto.PublicKey, error) {
	var primaryError error
	var fallbackError error
	var key crypto.PublicKey

	logger := m.logger.With(zap.String("keyID", keyID))

	// Attempt to retrieve from primary public key provider
	if key, primaryError = m.primary.GetPublicKey(keyID); primaryError == nil {
		logger.Debug("retrieving key succeeded from primary PKP")
		return key, nil
	}

	// Attempt to retrieve from fallback public key provider
	if key, fallbackError = m.fallback.GetPublicKey(keyID); fallbackError == nil {
		logger.Warn("retrieving key failed from primary PKP, but succeeded with fallback PKP", zap.NamedError("primaryError", primaryError))
		return key, nil
	}

	return key, errors.Wrapf(primaryError, "failed to retrieve public key; primary PKP failed. Fallback failed: %q", fallbackError)
}
