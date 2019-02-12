package reporterreporter

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/ctrl"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	"github.com/atlassian/voyager/pkg/reporter/client"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	retryAttemptsPerReport = 5
	retryDelay             = time.Second * 1
	slurperPath            = "/slurp"
)

type report struct {
	clusterID        string
	httpClient       *http.Client
	kubernetesClient kubernetes.Interface
	logger           *zap.Logger
	mutator          *restclient.RequestMutator
	reporterClient   client.Interface
}

type RequestData struct {
	Cluster   string                      `json:"cluster"`
	Service   string                      `json:"serviceName"`
	Timestamp int64                       `json:"timestamp"`
	Data      reporter_v1.NamespaceReport `json:"data"`
}

func NewReport(slurperURI string, cluster string, reporterClient client.Interface, kubeClient kubernetes.Interface, logger *zap.Logger) ctrl.Server {
	logger.Info("ReporterReporter configured",
		zap.String("cluster_id", cluster),
		zap.String("slurper_uri", slurperURI),
		zap.String("slurper_path", slurperPath))

	return &report{
		clusterID:        cluster,
		httpClient:       util.HTTPClient(),
		kubernetesClient: kubeClient,
		logger:           logger,
		mutator: restclient.NewRequestMutator(
			restclient.BaseURL(slurperURI),
			restclient.JoinPath(slurperPath),
			restclient.Method(http.MethodPost)),
		reporterClient: reporterClient,
	}
}

func isRetriableError(statusCode int) bool {
	_, ok := map[int]bool{
		http.StatusRequestTimeout:  true,
		http.StatusTooManyRequests: true,
	}[statusCode]

	if ok {
		return true
	}

	return statusCode >= 500 && statusCode <= 599
}

// sendData will send the requestData containing the report to slurper
func (r *report) sendData(requestData RequestData) (retriable bool, err error) {
	req, err := r.mutator.NewRequest(restclient.BodyFromJSON(requestData))
	if err != nil {
		return false, errors.Wrap(err, "unable to craft a HTTP request for slurper")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		if urlError, ok := err.(*url.Error); ok {
			if urlError.Timeout() || urlError.Temporary() {
				return true, errors.Wrap(err, "transient error occurred during request to slurper")
			}
		}

		return false, errors.Wrap(err, "error occured during request to slurper")

	}
	defer resp.Body.Close()

	// We don't need to check if resp is nil; it won't be if err is nil
	if resp.StatusCode == http.StatusOK {
		return false, nil
	}

	return isRetriableError(resp.StatusCode), errors.Errorf("sending data to slurper failed")
}

func (r *report) sendNamespaceReportToSlurper(namespaceName string) (notRetriableError error) {
	reports, err := r.reporterClient.ReporterV1().Reports(namespaceName).List(meta_v1.ListOptions{})
	if err != nil {
		r.logger.Error("Could not list reports in namespace", zap.String("namespace", namespaceName), zap.Error(err))
		return nil
	}

	if len(reports.Items) == 0 {
		r.logger.Info("Report for namespace is empty", zap.String("namespace", namespaceName))
		return nil
	}

	now := time.Now().Unix()

	countReportsSent := 0
	countReportsAttempted := 0

	for _, report := range reports.Items {
		requestData := RequestData{
			Cluster:   r.clusterID,
			Service:   namespaceName,
			Timestamp: now,
			Data:      report.Report,
		}
		countReportsAttempted++

		for i := 1; i <= retryAttemptsPerReport; i++ {
			retriable, err := r.sendData(requestData)
			if err == nil {
				countReportsSent++
				break
			}

			if !retriable {
				// If the request can not be retried, then we shouldn't try continue trying requests
				r.logger.Info("Sent namespace reports to slurper",
					zap.String("namespace_name", namespaceName),
					zap.Int("reports_sent", countReportsSent),
					zap.Int("reports_attempted", countReportsAttempted),
					zap.Int("reports_size", len(reports.Items)))
				return err
			}

			r.logger.Warn("Failed to send report to slurper",
				zap.Int("attempt", i),
				zap.Int("max_attempts", retryAttemptsPerReport),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}

	r.logger.Info("Sent namespace reports to slurper",
		zap.String("namespace_name", namespaceName),
		zap.Int("reports_sent", countReportsSent),
		zap.Int("reports_attempted", countReportsAttempted),
		zap.Int("reports_size", len(reports.Items)))

	return nil
}

func (r *report) Run(_ context.Context) error {
	namespaces, err := r.kubernetesClient.CoreV1().Namespaces().List(meta_v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list namespaces")
	}

	for _, namespace := range namespaces.Items {
		if err := r.sendNamespaceReportToSlurper(namespace.Name); err != nil {
			return errors.Wrap(err, "non-retriable error encountered while talking to slurper, stopping all reports")
		}
	}

	return nil
}
