package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ASAPKeyClusterServiceClassExternalID   = "daa6e8e7-7201-4031-86f2-ef9fdfeae7d6"
	ASAPKeyClusterServiceClassName         = ASAPKeyClusterServiceClassExternalID
	ASAPKeyClusterServiceClassExternalName = "ASAP"
)

func isASAPKeyServiceClass(serviceInstance sc_v1b1.ServiceInstance) bool {
	return serviceInstance.Spec.ClusterServiceClassName == ASAPKeyClusterServiceClassName ||
		serviceInstance.Spec.ClusterServiceClassExternalID == ASAPKeyClusterServiceClassExternalID ||
		serviceInstance.Spec.ClusterServiceClassExternalName == ASAPKeyClusterServiceClassExternalName
}

// AsapKeyAdmitFunc only allow parameter.ServiceName (which ends up being the ASAPKey issuer) prefixed with Micros2 serviceName
func AsapKeyAdmitFunc(ctx context.Context, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {

	admissionRequest := admissionReview.Request

	// Validate supported resource type
	if admissionRequest.Resource != serviceInstanceResource {
		return nil, errors.Errorf("unsupported resource, got %v", admissionRequest.Resource)
	}

	// Validate supported operations
	if admissionRequest.Operation != admissionv1beta1.Create && admissionRequest.Operation != admissionv1beta1.Update {
		return nil, errors.Errorf("only create and update is supported, got %q", admissionRequest.Operation)
	}

	// Namespace is required to check the ASAP issuer
	if admissionRequest.Namespace == "" {
		return nil, errors.New("no namespace in AdmissionReview request")
	}

	// Parses ServiceInstance
	serviceInstance := sc_v1b1.ServiceInstance{}
	if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
		return nil, errors.Wrap(err, "malformed ServiceInstance specification")
	}

	// Allow any ServiceInstance that is not ASAPKey
	if !isASAPKeyServiceClass(serviceInstance) {
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Message: "ServiceInstance is not an ASAP resource",
			},
		}, nil
	}

	var parameters struct {
		ServiceName string `json:"serviceName"`
	}
	err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &parameters)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	serviceName := getServiceNameFromNamespace(admissionRequest.Namespace)
	if !strings.HasPrefix(parameters.ServiceName, serviceName) {
		reason := fmt.Sprintf("serviceName was set to %q, which is not prefixed by namespace %q", parameters.ServiceName, admissionRequest.Namespace)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusForbidden,
			},
		}, nil
	}

	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: "serviceName is prefixed by namespace",
		},
	}, nil

}
