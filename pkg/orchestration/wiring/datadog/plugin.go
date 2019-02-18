package datadog

import (
	"encoding/json"
	"fmt"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	meta_orch "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	clusterServiceClassExternalID                      = "875d4a87-e887-4838-a0b5-b64491dbf9cb"
	clusterServicePlanExternalID                       = "d8048a2d-49de-4fda-b7ef-328de171cd32"
	ResourceType                  voyager.ResourceType = "DatadogAlarm"
)

type WiringPlugin struct {
}

func WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResultSuccess, bool, error) {
	err := validateRequest(stateResource, context)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	kubeComputeDependency, err := context.TheOnlyDependency()
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	kubeDeploymentShape, found, err := knownshapes.FindKubeDeploymentShape(kubeComputeDependency.Contract.Shapes)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	if !found {
		return nil, false, errors.Errorf("failed to find shape %q in contract of %q", knownshapes.KubeDeploymentShape, kubeComputeDependency.Name)
	}
	cpuServiceInstance, err := constructServiceInstance(stateResource, context, kubeComputeDependency.Name, kubeDeploymentShape, CPU)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}

	memoryServiceInstance, err := constructServiceInstance(stateResource, context, kubeComputeDependency.Name, kubeDeploymentShape, Memory)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}

	wiringResult := &wiringplugin.WiringResultSuccess{
		Resources: []smith_v1.Resource{cpuServiceInstance, memoryServiceInstance},
	}
	return wiringResult, false, nil
}

func constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext, kubeCompute voyager.ResourceName, kubeDeployment *knownshapes.KubeDeployment, alarmType AlarmType) (smith_v1.Resource, error) {
	instanceID, err := svccatentangler.InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}
	query := QueryParams{
		KubeDeployment: kubeDeployment.Data.DeploymentName,
		KubeNamespace:  context.StateMeta.Namespace,
		AlarmType:      alarmType,
		Threshold: &AlarmThresholds{
			Critical: 90,
			Warning:  80,
		},
		Location: context.StateContext.Location,
	}
	message := query.generateMessage(&context.StateContext.ServiceProperties.Notifications)
	serviceInstanceSpec := OSBInstanceParameters{
		ServiceName: context.StateContext.ServiceName,
		EnvType:     context.StateContext.Location.EnvType,
		Region:      context.StateContext.Location.Region,
		Label:       context.StateContext.Location.Label,
		Attributes: AlarmAttributes{
			Name: createAlarmNameforDatadog(context.StateContext.ServiceName, context.StateContext.Location.EnvType,
				resource.Name, query.AlarmType, context.StateContext.Location.Label),
			Type:    Metric,
			Query:   query.generateQuery(),
			Message: message,
			Options: AlarmOptions{
				EscalationMessage: message,
				NotifyNoData:      false,
				RequireFullWindow: true,
				Thresholds:        query.Threshold,
			},
		},
	}

	serviceInstanceSpecBytes, err := json.Marshal(&serviceInstanceSpec)
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}

	alarmInstanceResource := smith_v1.Resource{
		Name: wiringutil.ConsumerProducerResourceNameWithPostfix(resource.Name, kubeCompute, string(alarmType)),
		References: []smith_v1.Reference{
			{
				Resource: kubeDeployment.Data.DeploymentResourceName,
			},
		},
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.ConsumerProducerMetaNameWithPostfix(resource.Name, kubeCompute, string(alarmType)),
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalID: clusterServiceClassExternalID,
						ClusterServicePlanExternalID:  clusterServicePlanExternalID,
					},
					Parameters: &runtime.RawExtension{
						Raw: serviceInstanceSpecBytes,
					},
					ExternalID: instanceID,
				},
			},
		},
	}
	return alarmInstanceResource, nil
}

func validateRequest(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) error {
	if stateResource.Type != ResourceType {
		return errors.Errorf("invalid resource type: %q", stateResource.Type)
	}
	if stateResource.Spec != nil {
		return errors.New("default alarm does not accept any user parameters")
	}
	return nil
}

func (q *QueryParams) generateQuery() string {
	if q.AlarmType == CPU {
		cpuUsageString := fmt.Sprintf("avg(last_5m):( avg:kubernetes.cpu.usage.total{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} ", q.Location.EnvType, q.KubeNamespace, q.KubeDeployment, q.Location.Region)
		cpuLimitString := fmt.Sprintf("/ ( avg:kubernetes.cpu.limits{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} * 1000000 ) ) * 100 > %d", q.Location.EnvType, q.KubeNamespace, q.KubeDeployment, q.Location.Region, q.Threshold.Critical)
		return cpuUsageString + cpuLimitString
	}

	memoryUsageString := fmt.Sprintf("avg(last_5m):( avg:kubernetes.memory.usage.total{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} ", q.Location.EnvType, q.KubeNamespace, q.KubeDeployment, q.Location.Region)
	memoryLimitString := fmt.Sprintf("/ ( avg:kubernetes.memory.limits{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} * 1000000 ) ) * 100 > %d", q.Location.EnvType, q.KubeNamespace, q.KubeDeployment, q.Location.Region, q.Threshold.Critical)
	return memoryUsageString + memoryLimitString

}

func (q *QueryParams) generateMessage(notificationProp *meta_orch.Notifications) string {
	msg := fmt.Sprintf("High %s usage for deployment %s in %s %s with the namespace %s", strings.ToUpper(string(q.AlarmType)),
		q.KubeDeployment, q.Location.Region, q.Location.EnvType, q.KubeNamespace)
	messageType := fmt.Sprintf(" [[#is_warning]] @%s [[/is_warning]] [[#is_warning_recovery]] @%s [[/is_warning_recovery]] [[#is_alert]] @%s [[/is_alert]][[#is_alert_recovery]] @%s [[/is_alert_recovery]]",
		notificationProp.LowPriorityPagerdutyEndpoint, notificationProp.LowPriorityPagerdutyEndpoint, notificationProp.PagerdutyEndpoint, notificationProp.PagerdutyEndpoint)
	return msg + messageType
}

func createAlarmNameforDatadog(serviceName voyager.ServiceName, envType voyager.EnvType, resourceName voyager.ResourceName, alarmType AlarmType, label voyager.Label) string {
	nameList := []string{string(serviceName), string(envType), string(resourceName), string(alarmType)}
	if label != "" {
		nameList = append(nameList, string(label))
	}
	return strings.Join(nameList, "-")
}
