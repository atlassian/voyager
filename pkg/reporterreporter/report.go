package reporterreporter

import (
	"context"
	"net/http"
	"time"

	"bitbucket.org/atlassianlabs/restclient"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	"github.com/atlassian/voyager/pkg/reporter/client"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Report struct {
	RemoteURI        string
	Cluster          string
	HTTPClient       *http.Client
	ReporterClient   client.Interface
	KubernetesClient kubernetes.Interface
	Logger           *zap.Logger
}

type RequestData struct {
	Cluster   string                      `json:"cluster"`
	Service   string                      `json:"serviceName"`
	Timestamp int64                       `json:"timestamp"`
	Data      reporter_v1.NamespaceReport `json:"data"`
}

func (r *Report) Run(ctx context.Context) error {
	namespaces, err := r.KubernetesClient.CoreV1().Namespaces().List(meta_v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list namespaces")
	}
	for _, namespace := range namespaces.Items {
		list, err := r.ReporterClient.ReporterV1().Reports(namespace.Name).List(meta_v1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "could not list reports")
		}
		if len(list.Items) == 0 {
			r.Logger.Sugar().Infof("%q is empty", namespace.Name)
			continue
		}

		for _, report := range list.Items {
			requestData := RequestData{
				Cluster:   r.Cluster,
				Service:   namespace.Name,
				Timestamp: time.Now().Unix(),
				Data:      report.Report,
			}

			rm := restclient.NewRequestMutator(
				restclient.BaseURL(r.RemoteURI),
			)
			req, err := rm.NewRequest(
				restclient.JoinPath("/slurp"),
				restclient.Method(http.MethodPost),
				restclient.BodyFromJSON(requestData),
			)
			if err != nil {
				return errors.Wrap(err, "invalid request")
			}
			resp, err := r.HTTPClient.Do(req)
			if err != nil {
				return errors.Wrap(err, "failed to connect to backend")
			}
			if resp.StatusCode != http.StatusOK {
				return errors.New("failed to POST the data")
			}
		}

	}
	return nil
}
