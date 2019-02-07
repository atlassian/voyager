package reporterreporter

import (
	"context"
	"net/http"
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
)

type Report struct {
	Cluster          string
	HTTPClient       *http.Client
	ReporterClient   client.Interface
	KubernetesClient kubernetes.Interface
	Logger           *zap.Logger

	mutator *restclient.RequestMutator
}

type RequestData struct {
	Cluster   string                      `json:"cluster"`
	Service   string                      `json:"serviceName"`
	Timestamp int64                       `json:"timestamp"`
	Data      reporter_v1.NamespaceReport `json:"data"`
}

func NewReport(slurperURI string, cluster string, reporterClient client.Interface, kubeClient kubernetes.Interface, logger *zap.Logger) ctrl.Server {
	return &Report{
		Cluster:          cluster,
		HTTPClient:       util.HTTPClient(),
		ReporterClient:   reporterClient,
		KubernetesClient: kubeClient,
		Logger:           logger,

		mutator: restclient.NewRequestMutator(
			restclient.BaseURL(slurperURI),
			restclient.JoinPath("/slurp"),
			restclient.Method(http.MethodPost)),
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

func (r *Report) sendData(requestData RequestData) (retriable bool, err error) {
	req, err := r.mutator.NewRequest(restclient.BodyFromJSON(requestData))
	if err != nil {
		return false, errors.Wrap(err, "could not craft a HTTP request")
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return true, errors.Wrap(err, "failed to perform HTTP request to slurper")
	}

	if resp.StatusCode == http.StatusOK {
		return false, nil
	}

	if isRetriableError(resp.StatusCode) {
		return true, errors.Errorf("POSTing data to slurper failed with retriable status code %d", resp.StatusCode)
	}

	return false, errors.Errorf("POSTing data to slurper failed with status code %d", resp.StatusCode)
}

func (r *Report) Run(ctx context.Context) error {
	namespaces, err := r.KubernetesClient.CoreV1().Namespaces().List(meta_v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list namespaces")
	}

	for _, namespace := range namespaces.Items {
		reports, err := r.ReporterClient.ReporterV1().Reports(namespace.Name).List(meta_v1.ListOptions{})
		if err != nil {
			r.Logger.Error("Could not list reports in namespace", zap.String("namespace", namespace.Name), zap.Error(err))
			continue
		}

		if len(reports.Items) == 0 {
			r.Logger.Info("Report for namespace is empty", zap.String("namespace", namespace.Name))
			continue
		}

		for _, report := range reports.Items {
			requestData := RequestData{
				Cluster:   r.Cluster,
				Service:   namespace.Name,
				Timestamp: time.Now().Unix(),
				Data:      report.Report,
			}

			for i := 1; i <= retryAttemptsPerReport; i++ {
				if retriable, err := r.sendData(requestData); err != nil {
					if !retriable {
						// If the request is not retriable, then we shouldn't try the other requests either
						return err
					} else {
						r.Logger.Warn("Failed to send request data to slurper",
							zap.Int("attempt", i),
							zap.Int("max_attempts", retryAttemptsPerReport),
							zap.Error(err))
						time.Sleep(retryDelay)
					}
				}
			}
		}
	}

	return nil
}
