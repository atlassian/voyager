package prometheus

import (
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

func RegisterAll(registry prometheus.Registerer, metrics ...prometheus.Collector) error {
	for _, metric := range metrics {
		if err := registry.Register(metric); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
