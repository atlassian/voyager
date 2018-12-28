package monitor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	comp_v1_client "github.com/atlassian/voyager/pkg/composition/client/typed/composition/v1"
	creator_v1_client "github.com/atlassian/voyager/pkg/creator/client/typed/creator/v1"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	sc_v1b1_client "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Monitor struct {
	ServiceDescriptorName  string
	Logger                 *zap.Logger
	Location               *voyager.Location
	ExpectedProcessingTime time.Duration
	ServiceSpec            creator_v1.ServiceSpec
	Version                string

	ServiceDescriptorClient comp_v1_client.ServiceDescriptorInterface
	ServiceCatalogClient    sc_v1b1_client.Interface
	CreatorServiceClient    creator_v1_client.ServiceInterface
}

func (m *Monitor) Run(ctx context.Context) (retErr error) {
	defer func() {
		err := m.cleanup(ctx, m.ServiceDescriptorName)
		if err != nil {
			m.Logger.Error("Clean up failed", zap.Error(err))
		} else {
			m.Logger.Info("Clean up complete")
		}
	}()
	// Log failure details before cleaning up
	defer func() {
		if retErr != nil {
			m.logFailureDetails(ctx, retErr)
		}
	}()
	m.Logger.Info("Beginning monitor job")

	_, exists, err := m.checkInitialState(ctx)
	if err != nil {
		return err
	}

	if exists {
		err = m.deleteServiceDescriptor(ctx, m.ServiceDescriptorName)
		if err != nil {
			return err
		}
	}

	var sd *comp_v1.ServiceDescriptor
	sd, err = buildServiceDescriptor(m.ServiceDescriptorName, m.Location, m.Version)
	if err != nil {
		return err
	}

	err = m.createServiceDescriptor(ctx, sd)
	if err != nil {
		return err
	}

	m.Logger.Info("Initialized ServiceDescriptor for synthetic checks")

	// Wait until the corresponding ServiceInstance is created.
	// Sleeps for a short period of time, which for now should be enough I think
	time.Sleep(m.ExpectedProcessingTime)

	err = m.verifyUpsServiceInstance(ctx, m.ServiceDescriptorName, m.Version)
	if err != nil {
		m.Logger.Error("ServiceInstance verification has failed", zap.Error(err))
		return err
	}

	m.Logger.Info("ServiceInstance verification has succeeded")
	return nil
}

func (m *Monitor) verifyUpsServiceInstance(ctx context.Context, namespace, expectedVersion string) error {
	si, err := m.serviceInstance(ctx, resourceName, namespace)
	if err != nil {
		return errors.Wrapf(err, "could not get ServiceInstance %q", resourceName)
	}

	err = verifySpec(si.Spec, namespace, expectedVersion)
	if err != nil {
		return errors.WithMessage(err, "unexpected Spec")
	}

	err = verifyStatus(si.Status)
	if err != nil {
		return errors.WithMessage(err, "unexpected Status")
	}

	return nil
}

func verifySpec(spec v1beta1.ServiceInstanceSpec, namespace, expectedVersion string) error {
	var data map[string]string
	err := json.Unmarshal(spec.Parameters.Raw, &data)
	if err != nil {
		return errors.Wrapf(err, "could not unmarshal spec's parameters")
	}

	foundVersion, ok := data[versionParameter]
	if !ok {
		return errors.Errorf("version parameter missing from ServiceInstance %q in %q", resourceName, namespace)
	}
	if foundVersion != expectedVersion {
		return errors.Errorf("found ServiceInstance with embedded version %q but expecting %q", foundVersion, expectedVersion)
	}

	return nil
}

func verifyStatus(status v1beta1.ServiceInstanceStatus) error {
	i, condition := findCondition(status.Conditions, sc_v1b1.ServiceInstanceConditionReady)
	if i == -1 {
		return errors.New("this ServiceInstance does not have any valid conditions")
	}
	if condition.Status != sc_v1b1.ConditionTrue {
		return errors.Errorf("not ready - %q: %q", condition.Reason, condition.Message)
	}

	return nil
}

func (m *Monitor) cleanup(ctx context.Context, sdName string) error {
	// Delete SD and wait for it to complete
	return m.deleteServiceDescriptor(ctx, sdName)
}

func (m *Monitor) checkInitialState(ctx context.Context) (*comp_v1.ServiceDescriptor, bool /* exists */, error) {
	var sd *comp_v1.ServiceDescriptor

	m.Logger.Info("Checking initial state of Service Descriptor")
	sd, err := m.ServiceDescriptorClient.Get(m.ServiceDescriptorName, meta_v1.GetOptions{})

	exists := true
	if err != nil {
		if !api_errors.IsNotFound(err) {
			return nil, false, err
		}
		exists = false
	}

	return sd, exists, nil
}

func (m *Monitor) createServiceDescriptor(ctx context.Context, sd *comp_v1.ServiceDescriptor) error {
	client := m.ServiceDescriptorClient
	_, err := client.Create(sd)
	return err
}

func (m *Monitor) deleteServiceDescriptor(ctx context.Context, name string) error {
	client := m.ServiceDescriptorClient
	err := client.Delete(name, &meta_v1.DeleteOptions{})
	if err != nil {
		if !api_errors.IsNotFound(err) {
			return errors.Wrap(err, "error while deleting service descriptor")
		}
		m.Logger.Info("ServiceDescriptor was not deleted because it was already gone")
		return nil
	}

	// Wait for Service Descriptor to be gone
	return m.waitForServiceDescriptorDeletion(ctx, name)
}

func (m *Monitor) waitForServiceDescriptorDeletion(ctx context.Context, name string) error {
	client := m.ServiceDescriptorClient
	err := wait.PollImmediate(pollDelay, serviceDescriptorDeletionTimeout, func() (bool, error) {
		sd, err := client.Get(name, meta_v1.GetOptions{})
		if err != nil {
			if api_errors.IsNotFound(err) {
				return true, nil
			}

			return true, errors.Wrap(err, "unexpected error while trying to get service descriptor")
		}
		if sd.DeletionTimestamp == nil {
			return true, errors.New("service descriptor doesn't have DeletionTimestamp set")
		}
		return false, nil
	})
	if err != nil {
		return errors.Wrap(err, "error while waiting for service descriptor deletion to complete")
	}
	return nil
}

func (m *Monitor) logFailureDetails(ctx context.Context, retErr error) {
	var serviceInstance zap.Field
	si, err := m.serviceInstance(ctx, resourceName, m.ServiceDescriptorName)
	if err != nil {
		serviceInstance = ServiceInstanceError(errors.Wrap(err, "si"))
	} else {
		serviceInstance = ServiceInstance(si)
	}

	var serviceDescriptor zap.Field
	sd, err := m.serviceDescriptor(ctx, m.ServiceDescriptorName)
	if err != nil {
		serviceDescriptor = ServiceDescriptorError(errors.Wrap(err, "sd"))
	} else {
		serviceDescriptor = ServiceDescriptor(sd)
	}

	var service zap.Field
	srv, err := m.service(ctx, m.ServiceDescriptorName)
	if err != nil {
		service = ServiceError(errors.Wrap(err, "service"))
	} else {
		service = Service(srv)
	}

	m.Logger.Error("monitor job failure", serviceInstance, serviceDescriptor, service, zap.Error(retErr))
}

func (m *Monitor) serviceInstance(ctx context.Context, name, namespace string) (*v1beta1.ServiceInstance, error) {
	client := m.ServiceCatalogClient.ServicecatalogV1beta1().ServiceInstances(namespace)
	return client.Get(name, meta_v1.GetOptions{})
}

func (m *Monitor) serviceDescriptor(ctx context.Context, name string) (*comp_v1.ServiceDescriptor, error) {
	client := m.ServiceDescriptorClient
	return client.Get(name, meta_v1.GetOptions{})
}

func (m *Monitor) service(ctx context.Context, name string) (*creator_v1.Service, error) {
	client := m.CreatorServiceClient
	return client.Get(name, meta_v1.GetOptions{})
}

// Right now a ServiceInstance always has only a single ServiceInstanceCondition, but that might change in the future
func findCondition(conditions []v1beta1.ServiceInstanceCondition, conditionType v1beta1.ServiceInstanceConditionType) (int /* index */, *v1beta1.ServiceInstanceCondition) {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}
