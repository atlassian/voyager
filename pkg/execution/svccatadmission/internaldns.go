package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"net/http"
	"sync"

	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns/api"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InternalDNSAdmitFunc checks existing DNS alias ownership via micros server API, for both InternalDNS Services and Ingress Resources
func InternalDNSAdmitFunc(ctx context.Context, microsServerClient microsServerClient, scClient serviceCentralClient, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {

	glog.Infof("Received request for %v", admissionReview.Request)

	admissionRequest := admissionReview.Request

	// populating a chanel of domains to check owner
	var domainsToCheck chan string
	switch admissionRequest.Resource {
	case ingressResource:
		ingress := ext_v1b1.Ingress{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, &ingress); err != nil {
			return nil, errors.Errorf("error parsing Ingress resource")
		}
		if len(ingress.Spec.Rules) == 0 {
			return nil, errors.Errorf("can not process Ingress with empty rules list")
		}
		domainsToCheck = make(chan string, len(ingress.Spec.Rules))
		for _, ingressRule := range ingress.Spec.Rules {
			domainsToCheck <- ingressRule.Host
		}
	case serviceInstanceResource:
		serviceInstance := sc_v1b1.ServiceInstance{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
			return nil, errors.Wrap(err, "error parsing ServiceInstance")
		}
		var internalDNSSpec apiinternaldns.Spec
		if err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &internalDNSSpec); err != nil {
			return nil, errors.Wrap(err, "error parsing InternalDNS spec")
		}
		if len(internalDNSSpec.Aliases) == 0 {
			return nil, errors.Errorf("can not process InternalDNS with empty aliases list")
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
	var domainsOwnership []domainOwnership

	// worker groups to parallel fetch requestedDomain owners
	numWorkers := 5
	wg := &sync.WaitGroup{}
	wg.Add(numWorkers)
	var mutex = &sync.Mutex{}
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for domain := range domainsToCheck {
				aliasInfo, err := microsServerClient.GetAlias(ctx, domain)
				mutex.Lock()
				domainsOwnership = append(domainsOwnership, domainOwnership{
					requestedDomain:         domain,
					existingDomainAliasInfo: aliasInfo,
					err:                     err,
				})
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()

	// checking if domains are allowed to be migrated
	for _, domainOwnership := range domainsOwnership {
		if domainOwnership.err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("error requesting alias info for %q from micros server", domainOwnership.requestedDomain))
		}
		if domainOwnership.existingDomainAliasInfo != nil {
			if domainOwnership.existingDomainAliasInfo.Service.Owner != serviceCentralData.ServiceOwner.Username {
				return &admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Message: fmt.Sprintf(
							"requested dns alias %q is currently owned by %q via service %q, and can not be migrated to service %q owned by different owner %q",
							domainOwnership.requestedDomain,
							domainOwnership.existingDomainAliasInfo.Service.Owner,
							domainOwnership.existingDomainAliasInfo.Service.Name,
							serviceCentralData.ServiceName,
							serviceCentralData.ServiceOwner.Username,
						),
						Code: http.StatusForbidden,
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
