package app

import (
	"flag"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/pkg/snoopy"
	prom_api "github.com/prometheus/client_golang/api"
	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func NewFromFlags(name string, flagset *flag.FlagSet, arguments []string) (*snoopy.Controller, error) {
	logOpts := options.LoggerOptions{}
	options.BindLoggerFlags(&logOpts, flagset)

	promAddress := flagset.String("prom-address", "http://prom-ops.monitoring:9090", "The address of the Prometheus server to pull from")
	statsdAddress := flagset.String("statsd-address", "statsd:8125", "The address of the StatsD server to push to")
	flag.Parse()

	// Prometheus Client
	client, err := prom_api.NewClient(prom_api.Config{
		Address: *promAddress,
	})
	if err != nil {
		return nil, err
	}
	prometheus := prom_v1.NewAPI(client)

	// StatsD client
	sd, err := statsd.New(*statsdAddress)
	if err != nil {
		return nil, err
	}
	sd.Namespace = "voyager."

	// Controller
	c := &snoopy.Controller{
		Logger:     options.LoggerFromOptions(logOpts),
		Prometheus: prometheus,
		StatsD:     sd,
	}

	return c, nil
}
