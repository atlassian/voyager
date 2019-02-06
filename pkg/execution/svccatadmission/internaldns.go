package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/platformdns/api"
	"github.com/atlassian/voyager/pkg/util/logz"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func isInternalDNSServiceClass(serviceInstance *sc_v1b1.ServiceInstance) bool {
	return serviceInstance.Spec.ClusterServiceClassExternalID == string(apiplatformdns.ClusterServiceClassExternalID)
}

// InternalDNSAdmitFunc checks existing DNS alias ownership via micros server API, for both InternalDNS Services and Ingress Resources
func InternalDNSAdmitFunc(ctx context.Context, microsServerClient microsServerClient, scClient serviceCentralClient, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	logger := logz.RetrieveLoggerFromContext(ctx).Sugar()
	admissionRequest := admissionReview.Request

	// populating a chanel of domains to check owner
	var domainsToCheck chan string
	switch admissionRequest.Resource {
	case k8s.IngressGVR:
		ingress := ext_v1b1.Ingress{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, &ingress); err != nil {
			return nil, errors.Errorf("error parsing Ingress resource")
		}
		if len(ingress.Spec.Rules) == 0 {
			return nil, errors.Errorf("cannot process Ingress with empty rules list")
		}
		domainsToCheck = make(chan string, len(ingress.Spec.Rules))
		for _, ingressRule := range ingress.Spec.Rules {
			domainsToCheck <- ingressRule.Host
		}
	case k8s.ServiceInstanceGVR:
		serviceInstance := sc_v1b1.ServiceInstance{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
			return nil, errors.Wrap(err, "error parsing ServiceInstance")
		}
		if !isInternalDNSServiceClass(&serviceInstance) {
			return &admissionv1beta1.AdmissionResponse{
				Allowed: true,
				Result: &metav1.Status{
					Message: "requested ServiceInstance is not InternalDNS type",
				},
			}, nil
		}
		var internalDNSSpec apiplatformdns.Spec
		if err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &internalDNSSpec); err != nil {
			return nil, errors.Wrap(err, "error parsing InternalDNS spec")
		}
		if len(internalDNSSpec.Aliases) == 0 {
			return nil, errors.Errorf("cannot process InternalDNS with empty aliases list")
		}
		domainsToCheck = make(chan string, len(internalDNSSpec.Aliases))
		for _, alias := range internalDNSSpec.Aliases {
			domainsToCheck <- alias.Name
		}
	default:
		return nil, errors.Errorf("unsupported resource, got %v", admissionRequest.Resource)
	}
	close(domainsToCheck)

	// fetching service data from service central
	serviceName := getServiceNameFromNamespace(admissionRequest.Namespace)
	serviceCentralData, err := getServiceData(ctx, scClient, serviceName)
	if err != nil {
		return nil, errors.Errorf("error fetching service central data for serviceName %q", serviceName)
	}

	// types to store parallel domain owner fetches
	type domainOwnership struct {
		requestedDomain         string
		existingDomainAliasInfo *microsserver.AliasInfo
		err                     error
	}
	domainsOwnership := make([]domainOwnership, 0, cap(domainsToCheck))

	// worker groups to parallel fetch requestedDomain owners
	numWorkers := 5
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	var mutex sync.Mutex
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for domain := range domainsToCheck {
				aliasInfo, err := microsServerClient.GetAlias(ctx, domain)
				// concurrent safe append
				func() {
					mutex.Lock()
					defer mutex.Unlock()
					domainsOwnership = append(domainsOwnership, domainOwnership{
						requestedDomain:         domain,
						existingDomainAliasInfo: aliasInfo,
						err:                     err,
					})
				}()
			}
		}()
	}
	wg.Wait()

	// checking if domains are allowed to be migrated
	for _, domainOwnership := range domainsOwnership {
		if domainOwnership.err != nil {
			return nil, errors.Wrapf(err, "error requesting alias info for %q from micros server", domainOwnership.requestedDomain)
		}
		if domainOwnership.existingDomainAliasInfo != nil {
			// if we get a domain name with empty Service object from micros-server, it means is a domain provided via OSB for a micros 2 service
			// in that case micros-server does not have ownership information, so we allow it and let the provider update logic deals with the situation
			if domainOwnership.existingDomainAliasInfo.Alias.DomainName != "" && domainOwnership.existingDomainAliasInfo.Service.Owner == "" {
				logger.Infof("domain %q returned no ownership information from micros server, so it is a domain provided via OSB, allowing", domainOwnership.requestedDomain)
				continue
			}
			if domainOwnership.existingDomainAliasInfo.Service.Owner != serviceCentralData.ServiceOwner.Username {
				return &admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Message: fmt.Sprintf(
							"requested dns alias %q is currently owned by %q via service %q, and cannot be migrated to service %q owned by different owner %q",
							domainOwnership.requestedDomain,
							domainOwnership.existingDomainAliasInfo.Service.Owner,
							domainOwnership.existingDomainAliasInfo.Service.Name,
							serviceCentralData.ServiceName,
							serviceCentralData.ServiceOwner.Username,
						),
						Code:   http.StatusForbidden,
						Reason: metav1.StatusReasonForbidden,
					},
				}, nil
			}
		}
	}

	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: "requested domain name(s) allowed for use",
		},
	}, nil

}
