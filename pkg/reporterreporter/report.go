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
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

func (r *Report) getNamespaces() (*v1.NamespaceList, error) {
	namespaces, err := r.KubernetesClient.CoreV1().Namespaces().List(meta_v1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list namespaces")
	}
	return namespaces, nil
}

func (r *Report) getReporterReports(namespace *v1.Namespace) (*reporter_v1.ReportList, error) {
	list, err := r.ReporterClient.ReporterV1().Reports(namespace.Name).List(meta_v1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not list reports in namespace %q", namespace.Name)
	}
	return list, nil
}

func (r *Report) Run(ctx context.Context) error {
	namespaces, err := r.getNamespaces()
	if err != nil {
		return err
	}

	for _, namespace := range namespaces.Items {
		reports, err := r.getReporterReports(&namespace)
		if err != nil {
			return err
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

			req, err := r.mutator.NewRequest(restclient.BodyFromJSON(requestData))
			if err != nil {
				return errors.Wrap(err, "could not craft request")
			}

			resp, err := r.HTTPClient.Do(req)
			if err != nil {
				return errors.Wrap(err, "failed to perform HTTP request to slurper")
			}
			if resp.StatusCode != http.StatusOK {
				return errors.New("failed to POST the data to the slurper")
			}
		}

	}
	return nil
}
