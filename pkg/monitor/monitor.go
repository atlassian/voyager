package monitor

import (
	"context"
	"time"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	comp_v1_client "github.com/atlassian/voyager/pkg/composition/client/typed/composition/v1"
	creator_v1_client "github.com/atlassian/voyager/pkg/creator/client/typed/creator/v1"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
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
	Location               voyager.Location
	ExpectedProcessingTime time.Duration
	ServiceSpec            creator_v1.ServiceSpec
	ServiceDescriptor      string

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
	sd, err = buildServiceDescriptor(m.ServiceDescriptor)
	if err != nil {
		return err
	}

	err = m.createServiceDescriptor(ctx, sd)
	if err != nil {
		return err
	}

	m.Logger.Info("Initialized ServiceDescriptor for synthetic checks")

	err = wait.PollImmediate(pollDelay, m.ExpectedProcessingTime, m.verifyServiceDescriptorStatus(sd.ObjectMeta.Name))
	if err != nil {
		return err
	}

	m.Logger.Info("ServiceDescriptor Status verification has succeeded")
	return nil
}

func (m *Monitor) verifyServiceDescriptorStatus(name string) func() (bool, error) {
	return func() (bool, error) {
		var sd *comp_v1.ServiceDescriptor

		sd, err := m.ServiceDescriptorClient.Get(name, meta_v1.GetOptions{})
		if err != nil {
			return false, err
		}

		_, ready := cond_v1.FindCondition(sd.Status.Conditions, cond_v1.ConditionReady)
		if ready == nil {
			// Probably fine, this can trigger before the SD is processed by the cluster
			return false, nil
		}

		if ready.Status != cond_v1.ConditionTrue {
			return false, nil
		}

		return true, nil
	}
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
	err := wait.PollImmediate(pollDelay, m.ExpectedProcessingTime, func() (bool, error) {
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

	m.Logger.Error("monitor job failure", serviceDescriptor, service, zap.Error(retErr))
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
