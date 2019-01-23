package svccatadmission

import (
	"context"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"

	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	serviceInstance = "serviceinstances"
	serviceBinding  = "servicebindings"
	ingress         = "ingresses"
)

var (
	serviceInstanceResource = metav1.GroupVersionResource{
		Group:    sc_v1b1.SchemeGroupVersion.Group,
		Version:  sc_v1b1.SchemeGroupVersion.Version,
		Resource: serviceInstance,
	}
	serviceBindingResource = metav1.GroupVersionResource{
		Group:    sc_v1b1.SchemeGroupVersion.Group,
		Version:  sc_v1b1.SchemeGroupVersion.Version,
		Resource: serviceBinding,
	}
	ingressResource = metav1.GroupVersionResource{
		Group:    ext_v1b1.SchemeGroupVersion.Group,
		Version:  ext_v1b1.SchemeGroupVersion.Version,
		Resource: ingress,
	}
)

type serviceCentralClient interface {
	ListServices(ctx context.Context, user auth.OptionalUser, search string) ([]servicecentral.ServiceData, error)
}

type microsServerClient interface {
	GetAlias(ctx context.Context, domainName string) (*microsserver.AliasInfo, error)
}
