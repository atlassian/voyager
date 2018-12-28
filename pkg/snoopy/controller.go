package snoopy

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prom_model "github.com/prometheus/common/model"
	"go.uber.org/zap"
)

const VoyagerNamespace = "voyager"

type StatsdClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
}

type Controller struct {
	Logger     *zap.Logger
	Prometheus prom_v1.API
	StatsD     StatsdClient
}

func (c *Controller) Process(ctx context.Context) (retriable bool, err error) {
	metrics := map[string][]prom_model.LabelName{
		"ctrl_process_object_errors_total": []prom_model.LabelName{
			"controller",
		},
		"ctrl_process_object_seconds_count": []prom_model.LabelName{
			"controller",
		},
		fmt.Sprintf("kube_deployment_status_replicas_available{namespace=~%q}",
			VoyagerNamespace): []prom_model.LabelName{
			"deployment",
		},
		fmt.Sprintf(`label_replace(kube_job_status_failed{namespace=~%q}, "cronjob_name", "$1", "job_name", "(.*)-[0-9]+")`, VoyagerNamespace): []prom_model.LabelName{
			"job_name",
			"cronjob_name",
		},
		fmt.Sprintf(`label_replace(kube_job_status_succeeded{namespace=~%q}, "cronjob_name", "$1", "job_name", "(.*)-[0-9]+")`, VoyagerNamespace): []prom_model.LabelName{
			"job_name",
			"cronjob_name",
		},
	}
	for metric, labels := range metrics {
		c.Logger.Sugar().Infof("Processing prometheus metric %q looking for labels %v", metric, labels)
		value, err := c.Prometheus.Query(ctx, metric, time.Now())
		if err != nil {
			return false, errors.WithStack(err)
		}
		retriable, err := c.processMetric(ctx, value, labels)
		if err != nil {
			return retriable, errors.WithStack(err)
		}
	}
	return false, nil
}

func (c *Controller) processMetric(ctx context.Context, value prom_model.Value, labels []prom_model.LabelName) (retriable bool, err error) {
	// the model.Value can be one of several types, we need to cast it correctly
	t := value.Type()
	switch t {
	case prom_model.ValScalar:
		return false, errors.New("scalar metrics not yet supported")
	case prom_model.ValVector:
		return c.processVector(ctx, value.(prom_model.Vector), labels)
	case prom_model.ValMatrix:
		return false, errors.New("matrix metrics not yet supported")
	case prom_model.ValString:
		return false, errors.New("string metrics not yet supported")
	case prom_model.ValNone:
		return false, nil
	}
	return false, errors.Errorf("unknown metric type %q", t)
}

func (c *Controller) processVector(ctx context.Context, vec prom_model.Vector, labels []prom_model.LabelName) (retriable bool, err error) {
	for _, sample := range vec {
		name, exists := sample.Metric["__name__"]
		if !exists {
			// This is possible if our query is an aggregation
			return false, errors.New("unknown metric in query result")
		}

		tags := extractTags(sample.Metric, labels)

		c.Logger.Sugar().Debugf("Submitting %q %v (%v) to StatsD", name, tags, sample.Value)
		err := c.StatsD.Gauge(string(name), float64(sample.Value), tags, 1)
		if err != nil {
			return true, errors.WithStack(err)
		}
	}
	return false, nil
}

func extractTags(metric prom_model.Metric, labels []prom_model.LabelName) []string {
	var extracted []string
	for _, label := range labels {
		value, prs := metric[label]
		if prs {
			extracted = append(extracted, fmt.Sprintf("%s:%s", label, value))
		}
	}
	return extracted
}
