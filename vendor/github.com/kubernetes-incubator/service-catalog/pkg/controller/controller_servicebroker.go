/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"time"

	"k8s.io/klog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"

	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/kubernetes-incubator/service-catalog/pkg/metrics"
	"github.com/kubernetes-incubator/service-catalog/pkg/pretty"
	osb "github.com/pmorie/go-open-service-broker-client/v2"
)

// the Message strings have a terminating period and space so they can
// be easily combined with a follow on specific message.
const (
	errorListingServiceClassesReason  string = "ErrorListingServiceClasses"
	errorListingServiceClassesMessage string = "Error listing service classes."
	errorListingServicePlansReason    string = "ErrorListingServicePlans"
	errorListingServicePlansMessage   string = "Error listing service plans."
	errorDeletingServiceClassReason   string = "ErrorDeletingServiceClass"
	errorDeletingServiceClassMessage  string = "Error deleting service class."
	errorDeletingServicePlanReason    string = "ErrorDeletingServicePlan"
	errorDeletingServicePlanMessage   string = "Error deleting service plan."

	successServiceBrokerDeletedReason  string = "DeletedSuccessfully"
	successServiceBrokerDeletedMessage string = "The servicebroker %v was deleted successfully."
)

func (c *controller) serviceBrokerAdd(obj interface{}) {
	// DeletionHandlingMetaNamespaceKeyFunc returns a unique key for the resource and
	// handles the special case where the resource is of DeletedFinalStateUnknown type, which
	// acts a place holder for resources that have been deleted from storage but the watch event
	// confirming the deletion has not yet arrived.
	// Generally, the key is "namespace/name" for namespaced-scoped resources and
	// just "name" for cluster scoped resources.
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		klog.Errorf("Couldn't get key for object %+v: %v", obj, err)
		return
	}
	c.serviceBrokerQueue.Add(key)
}

func (c *controller) serviceBrokerUpdate(oldObj, newObj interface{}) {
	c.serviceBrokerAdd(newObj)
}

func (c *controller) serviceBrokerDelete(obj interface{}) {
	broker, ok := obj.(*v1beta1.ServiceBroker)
	if broker == nil || !ok {
		return
	}

	klog.V(4).Infof("Received delete event for ServiceBroker %v; no further processing will occur", broker.Name)
}

// shouldReconcileServiceBroker determines whether a broker should be reconciled; it
// returns true unless the broker has a ready condition with status true and
// the controller's broker relist interval has not elapsed since the broker's
// ready condition became true, or if the broker's RelistBehavior is set to Manual.
func shouldReconcileServiceBroker(broker *v1beta1.ServiceBroker, now time.Time, defaultRelistInterval time.Duration) bool {
	return shouldReconcileServiceBrokerCommon(
		pretty.NewServiceBrokerContextBuilder(broker),
		&broker.ObjectMeta,
		&broker.Spec.CommonServiceBrokerSpec,
		&broker.Status.CommonServiceBrokerStatus,
		now,
		defaultRelistInterval,
	)
}

func (c *controller) reconcileServiceBrokerKey(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	pcb := pretty.NewContextBuilder(pretty.ServiceBroker, namespace, name, "")
	broker, err := c.serviceBrokerLister.ServiceBrokers(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.Info(pcb.Message("Not doing work because the ServiceBroker has been deleted"))
		c.brokerClientManager.RemoveBrokerClient(NewServiceBrokerKey(namespace, name))
		return nil
	}
	if err != nil {
		klog.Info(pcb.Messagef("Unable to retrieve ServiceBroker: %v", err))
		return err
	}

	return c.reconcileServiceBroker(broker)
}

func (c *controller) updateServiceBrokerClient(broker *v1beta1.ServiceBroker) (osb.Client, error) {
	pcb := pretty.NewServiceBrokerContextBuilder(broker)
	authConfig, err := getAuthCredentialsFromServiceBroker(c.kubeClient, broker)
	if err != nil {
		s := fmt.Sprintf("Error getting broker auth credentials: %s", err)
		klog.Info(pcb.Message(s))
		c.recorder.Event(broker, corev1.EventTypeWarning, errorAuthCredentialsReason, s)
		if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorFetchingCatalogReason, errorFetchingCatalogMessage+s); err != nil {
			return nil, err
		}
		return nil, err
	}

	clientConfig := NewClientConfigurationForBroker(broker.ObjectMeta, &broker.Spec.CommonServiceBrokerSpec, authConfig)

	brokerClient, err := c.brokerClientManager.UpdateBrokerClient(NewServiceBrokerKey(broker.Namespace, broker.Name), clientConfig)
	if err != nil {
		s := fmt.Sprintf("Error creating client for broker %q: %s", broker.Name, err)
		klog.Info(pcb.Message(s))
		c.recorder.Event(broker, corev1.EventTypeWarning, errorAuthCredentialsReason, s)
		if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorFetchingCatalogReason, errorFetchingCatalogMessage+s); err != nil {
			return nil, err
		}
		return nil, err
	}

	return brokerClient, nil
}

// reconcileServiceBroker is the control-loop that reconciles a ServiceBroker. An
// error is returned to indicate that the binding has not been fully
// processed and should be resubmitted at a later time.
func (c *controller) reconcileServiceBroker(broker *v1beta1.ServiceBroker) error {
	pcb := pretty.NewServiceBrokerContextBuilder(broker)
	klog.V(4).Infof(pcb.Message("Processing"))

	// * If the broker's ready condition is true and the RelistBehavior has been
	// set to Manual, do not reconcile it.
	// * If the broker's ready condition is true and the relist interval has not
	// elapsed, do not reconcile it.
	if !shouldReconcileServiceBroker(broker, time.Now(), c.brokerRelistInterval) {
		return nil
	}

	if broker.DeletionTimestamp == nil { // Add or update
		klog.V(4).Info(pcb.Message("Processing adding/update event"))

		brokerClient, err := c.updateServiceBrokerClient(broker)
		if err != nil {
			return err
		}

		// get the broker's catalog
		now := metav1.Now()
		brokerCatalog, err := brokerClient.GetCatalog()
		if err != nil {
			s := fmt.Sprintf("Error getting broker catalog: %s", err)
			klog.Warning(pcb.Message(s))
			c.recorder.Eventf(broker, corev1.EventTypeWarning, errorFetchingCatalogReason, s)
			if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorFetchingCatalogReason, errorFetchingCatalogMessage+s); err != nil {
				return err
			}
			if broker.Status.OperationStartTime == nil {
				toUpdate := broker.DeepCopy()
				toUpdate.Status.OperationStartTime = &now
				if _, err := c.serviceCatalogClient.ServiceBrokers(broker.Namespace).UpdateStatus(toUpdate); err != nil {
					klog.Error(pcb.Messagef("Error updating operation start time: %v", err))
					return err
				}
			} else if !time.Now().Before(broker.Status.OperationStartTime.Time.Add(c.reconciliationRetryDuration)) {
				s := "Stopping reconciliation retries because too much time has elapsed"
				klog.Info(pcb.Message(s))
				c.recorder.Event(broker, corev1.EventTypeWarning, errorReconciliationRetryTimeoutReason, s)
				toUpdate := broker.DeepCopy()
				toUpdate.Status.OperationStartTime = nil
				toUpdate.Status.ReconciledGeneration = toUpdate.Generation
				return c.updateServiceBrokerCondition(toUpdate,
					v1beta1.ServiceBrokerConditionFailed,
					v1beta1.ConditionTrue,
					errorReconciliationRetryTimeoutReason,
					s)
			}
			return err
		}

		klog.V(5).Info(pcb.Messagef("Successfully fetched %v catalog entries", len(brokerCatalog.Services)))

		// set the operation start time if not already set
		if broker.Status.OperationStartTime != nil {
			toUpdate := broker.DeepCopy()
			toUpdate.Status.OperationStartTime = nil
			if _, err := c.serviceCatalogClient.ServiceBrokers(broker.Namespace).UpdateStatus(toUpdate); err != nil {
				klog.Error(pcb.Messagef("Error updating operation start time: %v", err))
				return err
			}
		}

		// get the existing services and plans for this broker so that we can
		// detect when services and plans are removed from the broker's
		// catalog
		existingServiceClasses, existingServicePlans, err := c.getCurrentServiceClassesAndPlansForNamespacedBroker(broker)
		if err != nil {
			return err
		}

		existingServiceClassMap := convertServiceClassListToMap(existingServiceClasses)
		existingServicePlanMap := convertServicePlanListToMap(existingServicePlans)

		// convert the broker's catalog payload into our API objects
		klog.V(4).Info(pcb.Message("Converting catalog response into service-catalog API"))

		payloadServiceClasses, payloadServicePlans, err := convertAndFilterCatalogToNamespacedTypes(broker.Namespace, brokerCatalog, broker.Spec.CatalogRestrictions, existingServiceClassMap, existingServicePlanMap)
		if err != nil {
			s := fmt.Sprintf("Error converting catalog payload for broker %q to service-catalog API: %s", broker.Name, err)
			klog.Warning(pcb.Message(s))
			c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
			if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason, errorSyncingCatalogMessage+s); err != nil {
				return err
			}
			return err
		}

		klog.V(5).Info(pcb.Message("Successfully converted catalog payload from to service-catalog API"))

		// reconcile the serviceClasses that were part of the broker's catalog
		// payload
		for _, payloadServiceClass := range payloadServiceClasses {
			existingServiceClass, _ := existingServiceClassMap[payloadServiceClass.Name]
			delete(existingServiceClassMap, payloadServiceClass.Name)
			if existingServiceClass == nil {
				existingServiceClass, _ = existingServiceClassMap[payloadServiceClass.Spec.ExternalID]
				delete(existingServiceClassMap, payloadServiceClass.Spec.ExternalID)
			}

			klog.V(4).Info(pcb.Messagef("Reconciling %s", pretty.ServiceClassName(payloadServiceClass)))
			if err := c.reconcileServiceClassFromServiceBrokerCatalog(broker, payloadServiceClass, existingServiceClass); err != nil {
				s := fmt.Sprintf(
					"Error reconciling %s (broker %q): %s",
					pretty.ServiceClassName(payloadServiceClass), broker.Name, err,
				)
				klog.Warning(pcb.Message(s))
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
				if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason,
					errorSyncingCatalogMessage+s); err != nil {
					return err
				}
				return err
			}

			klog.V(5).Info(pcb.Messagef("Reconciled %s", pretty.ServiceClassName(payloadServiceClass)))
		}

		// handle the serviceClasses that were not in the broker's payload;
		// mark these as having been removed from the broker's catalog
		for _, existingServiceClass := range existingServiceClassMap {
			if existingServiceClass.Status.RemovedFromBrokerCatalog {
				continue
			}

			klog.V(4).Info(pcb.Messagef("%s has been removed from broker's catalog; marking", pretty.ServiceClassName(existingServiceClass)))
			existingServiceClass.Status.RemovedFromBrokerCatalog = true
			_, err := c.serviceCatalogClient.ServiceClasses(broker.Namespace).UpdateStatus(existingServiceClass)
			if err != nil {
				s := fmt.Sprintf(
					"Error updating status of %s: %v",
					pretty.ServiceClassName(existingServiceClass), err,
				)
				klog.Warning(pcb.Message(s))
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
				if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason,
					errorSyncingCatalogMessage+s); err != nil {
					return err
				}
				return err
			}
		}

		// reconcile the plans that were part of the broker's catalog payload
		for _, payloadServicePlan := range payloadServicePlans {
			existingServicePlan, _ := existingServicePlanMap[payloadServicePlan.Name]
			delete(existingServicePlanMap, payloadServicePlan.Name)
			if existingServicePlan == nil {
				existingServicePlan, _ = existingServicePlanMap[payloadServicePlan.Spec.ExternalID]
				delete(existingServicePlanMap, payloadServicePlan.Spec.ExternalID)
			}

			klog.V(4).Infof(
				"ServiceBroker %q: reconciling %s",
				broker.Name, pretty.ServicePlanName(payloadServicePlan),
			)
			if err := c.reconcileServicePlanFromServiceBrokerCatalog(broker, payloadServicePlan, existingServicePlan); err != nil {
				s := fmt.Sprintf(
					"Error reconciling %s: %s",
					pretty.ServicePlanName(payloadServicePlan), err,
				)
				klog.Warning(pcb.Message(s))
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
				c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason,
					errorSyncingCatalogMessage+s)
				return err
			}
			klog.V(5).Info(pcb.Messagef("Reconciled %s", pretty.ServicePlanName(payloadServicePlan)))

		}

		// handle the servicePlans that were not in the broker's payload;
		// mark these as deleted
		for _, existingServicePlan := range existingServicePlanMap {
			if existingServicePlan.Status.RemovedFromBrokerCatalog {
				continue
			}
			klog.V(4).Info(pcb.Messagef("%s has been removed from broker's catalog; marking", pretty.ServicePlanName(existingServicePlan)))
			existingServicePlan.Status.RemovedFromBrokerCatalog = true
			_, err := c.serviceCatalogClient.ServicePlans(broker.Namespace).UpdateStatus(existingServicePlan)
			if err != nil {
				s := fmt.Sprintf(
					"Error updating status of %s: %v",
					pretty.ServicePlanName(existingServicePlan),
					err,
				)
				klog.Warning(pcb.Message(s))
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
				if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason,
					errorSyncingCatalogMessage+s); err != nil {
					return err
				}
				return err
			}
		}

		// everything worked correctly; update the broker's ready condition to
		// status true
		if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionTrue, successFetchedCatalogReason, successFetchedCatalogMessage); err != nil {
			return err
		}

		c.recorder.Event(broker, corev1.EventTypeNormal, successFetchedCatalogReason, successFetchedCatalogMessage)

		// Update metrics with the number of serviceclass and serviceplans from this broker
		metrics.BrokerServiceClassCount.WithLabelValues(broker.Name).Set(float64(len(payloadServiceClasses)))
		metrics.BrokerServicePlanCount.WithLabelValues(broker.Name).Set(float64(len(payloadServicePlans)))

		return nil
	}

	// All updates not having a DeletingTimestamp will have been handled above
	// and returned early. If we reach this point, we're dealing with an update
	// that's actually a soft delete-- i.e. we have some finalization to do.
	if finalizers := sets.NewString(broker.Finalizers...); finalizers.Has(v1beta1.FinalizerServiceCatalog) {
		klog.V(4).Info(pcb.Message("Finalizing"))

		existingServiceClasses, existingServicePlans, err := c.getCurrentServiceClassesAndPlansForNamespacedBroker(broker)
		if err != nil {
			return err
		}

		klog.V(4).Info(pcb.Messagef("Found %d ServiceClasses and %d ServicePlans to delete", len(existingServiceClasses), len(existingServicePlans)))

		for _, plan := range existingServicePlans {
			klog.V(4).Info(pcb.Messagef("Deleting %s", pretty.ServicePlanName(&plan)))
			err := c.serviceCatalogClient.ServicePlans(broker.Namespace).Delete(plan.Name, &metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				s := fmt.Sprintf("Error deleting %s: %s", pretty.ServicePlanName(&plan), err)
				klog.Warning(pcb.Message(s))
				c.updateServiceBrokerCondition(
					broker,
					v1beta1.ServiceBrokerConditionReady,
					v1beta1.ConditionUnknown,
					errorDeletingServicePlanMessage,
					errorDeletingServicePlanReason+s,
				)
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorDeletingServicePlanReason, "%v %v", errorDeletingServicePlanMessage, s)
				return err
			}
		}

		for _, svcClass := range existingServiceClasses {
			klog.V(4).Info(pcb.Messagef("Deleting %s", pretty.ServiceClassName(&svcClass)))
			err = c.serviceCatalogClient.ServiceClasses(broker.Namespace).Delete(svcClass.Name, &metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				s := fmt.Sprintf("Error deleting %s: %s", pretty.ServiceClassName(&svcClass), err)
				klog.Warning(pcb.Message(s))
				c.recorder.Eventf(broker, corev1.EventTypeWarning, errorDeletingServiceClassReason, "%v %v", errorDeletingServiceClassMessage, s)
				if err := c.updateServiceBrokerCondition(
					broker,
					v1beta1.ServiceBrokerConditionReady,
					v1beta1.ConditionUnknown,
					errorDeletingServiceClassMessage,
					errorDeletingServiceClassReason+s,
				); err != nil {
					return err
				}
				return err
			}
		}

		if err := c.updateServiceBrokerCondition(
			broker,
			v1beta1.ServiceBrokerConditionReady,
			v1beta1.ConditionFalse,
			successServiceBrokerDeletedReason,
			"The broker was deleted successfully",
		); err != nil {
			return err
		}
		// Clear the finalizer
		finalizers.Delete(v1beta1.FinalizerServiceCatalog)
		c.updateServiceBrokerFinalizers(broker, finalizers.List())

		c.recorder.Eventf(broker, corev1.EventTypeNormal, successServiceBrokerDeletedReason, successServiceBrokerDeletedMessage, broker.Name)
		klog.V(5).Info(pcb.Message("Successfully deleted"))

		// delete the metrics associated with this broker
		metrics.BrokerServiceClassCount.DeleteLabelValues(broker.Name)
		metrics.BrokerServicePlanCount.DeleteLabelValues(broker.Name)
		return nil
	}

	return nil
}

// reconcileServiceClassFromServiceBrokerCatalog reconciles a
// ServiceClass after the ServiceBroker's catalog has been re-
// listed. The serviceClass parameter is the serviceClass from the broker's
// catalog payload. The existingServiceClass parameter is the serviceClass
// that already exists for the given broker with this serviceClass' k8s name.
func (c *controller) reconcileServiceClassFromServiceBrokerCatalog(broker *v1beta1.ServiceBroker, serviceClass, existingServiceClass *v1beta1.ServiceClass) error {
	pcb := pretty.NewServiceBrokerContextBuilder(broker)
	serviceClass.Spec.ServiceBrokerName = broker.Name

	if existingServiceClass == nil {
		otherServiceClass, err := c.serviceClassLister.ServiceClasses(broker.Namespace).Get(serviceClass.Name)
		if err != nil {
			// we expect _not_ to find a service class this way, so a not-
			// found error is expected and legitimate.
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			// we do not expect to find an existing service class if we were
			// not already passed one; the following if statement will almost
			// certainly evaluate to true.
			if otherServiceClass.Spec.ServiceBrokerName != broker.Name {
				errMsg := fmt.Sprintf("%s already exists for Broker %q",
					pretty.ServiceClassName(serviceClass), otherServiceClass.Spec.ServiceBrokerName,
				)
				klog.Error(pcb.Message(errMsg))
				return fmt.Errorf(errMsg)
			}
		}

		klog.V(5).Info(pcb.Messagef("Fresh %s; creating", pretty.ServiceClassName(serviceClass)))
		if _, err := c.serviceCatalogClient.ServiceClasses(broker.Namespace).Create(serviceClass); err != nil {
			klog.Error(pcb.Messagef("Error creating %s: %v", pretty.ServiceClassName(serviceClass), err))
			return err
		}

		return nil
	}

	if existingServiceClass.Spec.ExternalID != serviceClass.Spec.ExternalID {
		errMsg := fmt.Sprintf(
			"%s already exists with OSB guid %q, received different guid %q",
			pretty.ServiceClassName(serviceClass), existingServiceClass.Name, serviceClass.Name,
		)
		klog.Error(pcb.Message(errMsg))
		return fmt.Errorf(errMsg)
	}

	klog.V(5).Info(pcb.Messagef("Found existing %s; updating", pretty.ServiceClassName(serviceClass)))

	// There was an existing service class -- project the update onto it and
	// update it.
	toUpdate := existingServiceClass.DeepCopy()
	toUpdate.Spec.BindingRetrievable = serviceClass.Spec.BindingRetrievable
	toUpdate.Spec.Bindable = serviceClass.Spec.Bindable
	toUpdate.Spec.PlanUpdatable = serviceClass.Spec.PlanUpdatable
	toUpdate.Spec.Tags = serviceClass.Spec.Tags
	toUpdate.Spec.Description = serviceClass.Spec.Description
	toUpdate.Spec.Requires = serviceClass.Spec.Requires
	toUpdate.Spec.ExternalName = serviceClass.Spec.ExternalName
	toUpdate.Spec.ExternalMetadata = serviceClass.Spec.ExternalMetadata

	updatedServiceClass, err := c.serviceCatalogClient.ServiceClasses(broker.Namespace).Update(toUpdate)
	if err != nil {
		klog.Error(pcb.Messagef("Error updating %s: %v", pretty.ServiceClassName(serviceClass), err))
		return err
	}

	if updatedServiceClass.Status.RemovedFromBrokerCatalog {
		klog.V(4).Info(pcb.Messagef("Resetting RemovedFromBrokerCatalog status on %s", pretty.ServiceClassName(serviceClass)))
		updatedServiceClass.Status.RemovedFromBrokerCatalog = false
		_, err := c.serviceCatalogClient.ServiceClasses(broker.Namespace).UpdateStatus(updatedServiceClass)
		if err != nil {
			s := fmt.Sprintf("Error updating status of %s: %v", pretty.ServiceClassName(updatedServiceClass), err)
			klog.Warning(pcb.Message(s))
			c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
			if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason, errorSyncingCatalogMessage+s); err != nil {
				return err
			}
			return err
		}
	}

	return nil
}

// reconcileServicePlanFromServiceBrokerCatalog reconciles a
// ServicePlan after the ServiceClass's catalog has been re-listed.
func (c *controller) reconcileServicePlanFromServiceBrokerCatalog(broker *v1beta1.ServiceBroker, servicePlan, existingServicePlan *v1beta1.ServicePlan) error {
	pcb := pretty.NewServiceBrokerContextBuilder(broker)
	servicePlan.Spec.ServiceBrokerName = broker.Name

	if existingServicePlan == nil {
		otherServicePlan, err := c.servicePlanLister.ServicePlans(broker.Namespace).Get(servicePlan.Name)
		if err != nil {
			// we expect _not_ to find a service class this way, so a not-
			// found error is expected and legitimate.
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			// we do not expect to find an existing service class if we were
			// not already passed one; the following if statement will almost
			// certainly evaluate to true.
			if otherServicePlan.Spec.ServiceBrokerName != broker.Name {
				errMsg := fmt.Sprintf(
					"%s already exists for Broker %q",
					pretty.ServicePlanName(servicePlan), otherServicePlan.Spec.ServiceBrokerName,
				)
				klog.Error(pcb.Message(errMsg))
				return fmt.Errorf(errMsg)
			}
		}

		// An error returned from a lister Get call means that the object does
		// not exist.  Create a new ServicePlan.
		if _, err := c.serviceCatalogClient.ServicePlans(broker.Namespace).Create(servicePlan); err != nil {
			klog.Error(pcb.Messagef("Error creating %s: %v", pretty.ServicePlanName(servicePlan), err))
			return err
		}

		return nil
	}

	if existingServicePlan.Spec.ExternalID != servicePlan.Spec.ExternalID {
		errMsg := fmt.Sprintf(
			"%s already exists with OSB guid %q, received different guid %q",
			pretty.ServicePlanName(servicePlan), existingServicePlan.Spec.ExternalID, servicePlan.Spec.ExternalID,
		)
		klog.Error(pcb.Message(errMsg))
		return fmt.Errorf(errMsg)
	}

	klog.V(5).Info(pcb.Messagef("Found existing %s; updating", pretty.ServicePlanName(servicePlan)))

	// There was an existing service plan -- project the update onto it and
	// update it.
	toUpdate := existingServicePlan.DeepCopy()
	toUpdate.Spec.Description = servicePlan.Spec.Description
	toUpdate.Spec.Bindable = servicePlan.Spec.Bindable
	toUpdate.Spec.Free = servicePlan.Spec.Free
	toUpdate.Spec.ExternalName = servicePlan.Spec.ExternalName
	toUpdate.Spec.ExternalMetadata = servicePlan.Spec.ExternalMetadata
	toUpdate.Spec.InstanceCreateParameterSchema = servicePlan.Spec.InstanceCreateParameterSchema
	toUpdate.Spec.InstanceUpdateParameterSchema = servicePlan.Spec.InstanceUpdateParameterSchema
	toUpdate.Spec.ServiceBindingCreateParameterSchema = servicePlan.Spec.ServiceBindingCreateParameterSchema

	updatedPlan, err := c.serviceCatalogClient.ServicePlans(broker.Namespace).Update(toUpdate)
	if err != nil {
		klog.Error(pcb.Messagef("Error updating %s: %v", pretty.ServicePlanName(servicePlan), err))
		return err
	}

	if updatedPlan.Status.RemovedFromBrokerCatalog {
		updatedPlan.Status.RemovedFromBrokerCatalog = false
		klog.V(4).Info(pcb.Messagef("Resetting RemovedFromBrokerCatalog status on %s", pretty.ServicePlanName(updatedPlan)))

		_, err := c.serviceCatalogClient.ServicePlans(broker.Namespace).UpdateStatus(updatedPlan)
		if err != nil {
			s := fmt.Sprintf("Error updating status of %s: %v", pretty.ServicePlanName(updatedPlan), err)
			klog.Error(pcb.Message(s))
			c.recorder.Eventf(broker, corev1.EventTypeWarning, errorSyncingCatalogReason, s)
			if err := c.updateServiceBrokerCondition(broker, v1beta1.ServiceBrokerConditionReady, v1beta1.ConditionFalse, errorSyncingCatalogReason, errorSyncingCatalogMessage+s); err != nil {
				return err
			}
			return err
		}
	}

	return nil
}

// updateCommonStatusCondition updates the common ready condition for the given CommonServiceBrokerStatus
// with the given status, reason, and message.
func updateCommonStatusCondition(pcb *pretty.ContextBuilder, meta metav1.ObjectMeta, commonStatus *v1beta1.CommonServiceBrokerStatus, conditionType v1beta1.ServiceBrokerConditionType, status v1beta1.ConditionStatus, reason, message string) {
	newCondition := v1beta1.ServiceBrokerCondition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	t := time.Now()

	if len(commonStatus.Conditions) == 0 {
		klog.Info(pcb.Messagef("Setting lastTransitionTime for condition %q to %v", conditionType, t))
		newCondition.LastTransitionTime = metav1.NewTime(t)
		commonStatus.Conditions = []v1beta1.ServiceBrokerCondition{newCondition}
	} else {
		for i, cond := range commonStatus.Conditions {
			if cond.Type == conditionType {
				if cond.Status != newCondition.Status {
					klog.Info(pcb.Messagef(
						"Found status change for condition %q: %q -> %q; setting lastTransitionTime to %v",
						conditionType, cond.Status, status, t,
					))
					newCondition.LastTransitionTime = metav1.NewTime(t)
				} else {
					newCondition.LastTransitionTime = cond.LastTransitionTime
				}

				commonStatus.Conditions[i] = newCondition
				break
			}
		}
	}

	// Set status.ReconciledGeneration && status.LastCatalogRetrievalTime if updating ready condition to true
	if conditionType == v1beta1.ServiceBrokerConditionReady && status == v1beta1.ConditionTrue {
		commonStatus.ReconciledGeneration = meta.Generation
		now := metav1.NewTime(t)
		commonStatus.LastCatalogRetrievalTime = &now
	}
}

// updateServiceBrokerCondition updates the ready condition for the given ServiceBroker
// with the given status, reason, and message.
func (c *controller) updateServiceBrokerCondition(broker *v1beta1.ServiceBroker, conditionType v1beta1.ServiceBrokerConditionType, status v1beta1.ConditionStatus, reason, message string) error {
	toUpdate := broker.DeepCopy()

	pcb := pretty.NewServiceBrokerContextBuilder(toUpdate)
	updateCommonStatusCondition(pcb, toUpdate.ObjectMeta, &toUpdate.Status.CommonServiceBrokerStatus, conditionType, status, reason, message)

	klog.V(4).Info(pcb.Messagef("Updating ready condition to %v", status))
	_, err := c.serviceCatalogClient.ServiceBrokers(broker.Namespace).UpdateStatus(toUpdate)
	if err != nil {
		klog.Error(pcb.Messagef("Error updating ready condition: %v", err))
	} else {
		klog.V(5).Info(pcb.Messagef("Updated ready condition to %v", status))
	}

	return err
}

// updateServiceBrokerFinalizers updates the given finalizers for the given Broker.
func (c *controller) updateServiceBrokerFinalizers(
	broker *v1beta1.ServiceBroker,
	finalizers []string) error {
	pcb := pretty.NewServiceBrokerContextBuilder(broker)

	// Get the latest version of the broker so that we can avoid conflicts
	// (since we have probably just updated the status of the broker and are
	// now removing the last finalizer).
	broker, err := c.serviceCatalogClient.ServiceBrokers(broker.Namespace).Get(broker.Name, metav1.GetOptions{})
	if err != nil {
		klog.Error(pcb.Messagef("Error finalizing: %v", err))
	}

	toUpdate := broker.DeepCopy()
	toUpdate.Finalizers = finalizers

	logContext := fmt.Sprint(pcb.Messagef("Updating finalizers to %v", finalizers))

	klog.V(4).Info(pcb.Messagef("Updating %v", logContext))
	_, err = c.serviceCatalogClient.ServiceBrokers(broker.Namespace).UpdateStatus(toUpdate)
	if err != nil {
		klog.Error(pcb.Messagef("Error updating %v: %v", logContext, err))
	}
	return err
}

func (c *controller) getCurrentServiceClassesAndPlansForNamespacedBroker(broker *v1beta1.ServiceBroker) ([]v1beta1.ServiceClass, []v1beta1.ServicePlan, error) {
	fieldSet := fields.Set{
		v1beta1.FilterSpecServiceBrokerName: broker.Name,
	}
	fieldSelector := fields.SelectorFromSet(fieldSet).String()
	listOpts := metav1.ListOptions{FieldSelector: fieldSelector}

	existingServiceClasses, err := c.serviceCatalogClient.ServiceClasses(broker.Namespace).List(listOpts)
	if err != nil {
		c.recorder.Eventf(broker, corev1.EventTypeWarning, errorListingServiceClassesReason, "%v %v", errorListingServiceClassesMessage, err)
		if err := c.updateServiceBrokerCondition(
			broker,
			v1beta1.ServiceBrokerConditionReady,
			v1beta1.ConditionUnknown,
			errorListingServiceClassesReason,
			errorListingServiceClassesMessage,
		); err != nil {
			return nil, nil, err
		}

		return nil, nil, err
	}

	existingServicePlans, err := c.serviceCatalogClient.ServicePlans(broker.Namespace).List(listOpts)
	if err != nil {
		c.recorder.Eventf(broker, corev1.EventTypeWarning, errorListingServicePlansReason, "%v %v", errorListingServicePlansMessage, err)
		if err := c.updateServiceBrokerCondition(
			broker,
			v1beta1.ServiceBrokerConditionReady,
			v1beta1.ConditionUnknown,
			errorListingServicePlansReason,
			errorListingServicePlansMessage,
		); err != nil {
			return nil, nil, err
		}

		return nil, nil, err
	}

	return existingServiceClasses.Items, existingServicePlans.Items, nil
}

func convertServiceClassListToMap(list []v1beta1.ServiceClass) map[string]*v1beta1.ServiceClass {
	ret := make(map[string]*v1beta1.ServiceClass, len(list))

	for i := range list {
		ret[list[i].Name] = &list[i]
	}

	return ret
}

func convertServicePlanListToMap(list []v1beta1.ServicePlan) map[string]*v1beta1.ServicePlan {
	ret := make(map[string]*v1beta1.ServicePlan, len(list))

	for i := range list {
		ret[list[i].Name] = &list[i]
	}

	return ret
}
