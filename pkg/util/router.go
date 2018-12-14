package util

import (
	"github.com/go-chi/chi"
	"go.uber.org/zap"
)

func NewRouter(serviceName string, logger *zap.Logger) (*chi.Mux, error) {
	r := chi.NewRouter()
	err := DefaultMiddleWare(logger, serviceName, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}
