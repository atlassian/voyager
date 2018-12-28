package snoopy

import (
	"context"
	"testing"
	"time"

	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prom_model "github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type Prometheus struct {
	Value prom_model.Vector
}

func (p *Prometheus) Query(ctx context.Context, query string, ts time.Time) (prom_model.Value, error) {
	return p.Value, nil
}
func (p *Prometheus) QueryRange(ctx context.Context, query string, r prom_v1.Range) (prom_model.Value, error) {
	return prom_model.Vector{}, nil
}
func (p *Prometheus) LabelValues(ctx context.Context, label string) (prom_model.LabelValues, error) {
	return prom_model.LabelValues{}, nil
}

func newPrometheus(name string, value float64) Prometheus {
	return Prometheus{
		Value: prom_model.Vector{
			&prom_model.Sample{
				Metric: prom_model.Metric{
					"__name__": prom_model.LabelValue(name),
				},
				Value:     prom_model.SampleValue(value),
				Timestamp: 1,
			},
		},
	}
}

type StatsD struct {
	Name  string
	Value float64
}

func (s *StatsD) Gauge(name string, value float64, tags []string, rate float64) error {
	s.Name = name
	s.Value = value
	return nil
}

func TestController_Process(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mockProm   Prometheus
		mockStatsd StatsD
	}{
		{
			mockProm:   newPrometheus("TEST 1", 9001),
			mockStatsd: StatsD{},
		},
		{
			mockProm:   newPrometheus("TEST 2", 9002),
			mockStatsd: StatsD{},
		},
		{
			mockProm:   newPrometheus("TEST 3", 9003),
			mockStatsd: StatsD{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	for _, test := range tests {
		cont := Controller{
			Logger:     zaptest.NewLogger(t),
			Prometheus: &test.mockProm,
			StatsD:     &test.mockStatsd,
		}
		cont.Process(ctx)
		require.Equal(t, string(test.mockProm.Value[0].Metric["__name__"]), test.mockStatsd.Name)
		require.Equal(t, float64(test.mockProm.Value[0].Value), test.mockStatsd.Value)
	}
}
