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
	svccatentangler.SvcCatEntangler
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
	setOfDatadogShape, found, err := knownshapes.FindSetOfDatadogShape(kubeComputeDependency.Contract.Shapes)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	if !found {
		return nil, false, errors.Errorf("failed to find shape %q in contract of %q", knownshapes.SetOfDatadogShape, kubeComputeDependency.Name)
	}

	var wiredResources []smith_v1.Resource

	deploymentResourceName := setOfDatadogShape.Data.DeploymentResourceName
	cpuServiceInstance, err := constructServiceInstance(stateResource, context, deploymentResourceName, CPU)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	wiredResources = append(wiredResources, cpuServiceInstance)

	memoryServiceInstance, err := constructServiceInstance(stateResource, context, deploymentResourceName, Memory)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	wiredResources = append(wiredResources, memoryServiceInstance)

	wiringResult := &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: []wiringplugin.Shape{},
		},
		Resources: wiredResources,
	}
	return wiringResult, false, nil
}

func constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext, deploymentResourceName smith_v1.ResourceName, alarmType AlarmType) (smith_v1.Resource, error) {
	instanceID, err := svccatentangler.InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}
	threshold := AlarmThresholds{
		Critical: 90,
		Warning:  80,
	}
	query := generateQuerySpec(context, alarmType, threshold, deploymentResourceName)
	alarmsAtt := createFinalAlarmSpec(resource, &context.StateContext, query)
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}
	serviceInstanceSpec := OSBInstanceParameters{
		ServiceName: context.StateContext.ServiceName,
		Attributes:  alarmsAtt,
		Environment: context.StateContext.Location.EnvType,
		Region:      context.StateContext.Location.Region,
	}

	serviceInstanceSpecBytes, err := json.Marshal(&serviceInstanceSpec)
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}

	alarmInstanceResource := smith_v1.Resource{
		Name: createAlarmNamesForSmithResource(resource.Name, deploymentResourceName, alarmType),
		References: []smith_v1.Reference{
			{
				Resource: deploymentResourceName,
			},
		},
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.ServiceInstanceWithPostfixMetaName(resource.Name, string(alarmType)),
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

func generateQuerySpec(context *wiringplugin.WiringContext, alarmType AlarmType, threshold AlarmThresholds, deploymentResourceName smith_v1.ResourceName) QueryParams {
	return QueryParams{

		KubeDeployment: string(deploymentResourceName),
		KubeNamespace:  context.StateMeta.Namespace,
		AlarmType:      alarmType,
		Threshold:      &threshold,
		Location: voyager.Location{
			EnvType: context.StateContext.Location.EnvType,
			Region:  context.StateContext.Location.Region,
		},
	}
}

func validateRequest(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) error {
	if stateResource.Type != ResourceType {
		return errors.Errorf("invalid resource type: %q", stateResource.Type)
	}
	if len(context.Dependencies) != 1 {
		return errors.New("default alarm should only dependent on one KubeCompute")
	}
	if stateResource.Spec != nil {
		return errors.Errorf("default alarm does not accept any user parameters")
	}
	return nil
}

func createFinalAlarmSpec(resource *orch_v1.StateResource, stateContext *wiringplugin.StateContext, query QueryParams) AlarmAttributes {
	alarmOption := &AlarmOptions{
		EscalationMessage: query.generateMessage(&stateContext.ServiceProperties.Notifications),
		NotifyNoData:      false,
		RequireFullWindow: true,
		Thresholds:        query.Threshold,
	}
	alarmSpec := &AlarmAttributes{
		Name: createAlarmNameforDatadog(string(stateContext.ServiceName), string(resource.Name), string(query.AlarmType),
			string(stateContext.Location.EnvType), string(stateContext.Location.Label)),
		Type:    string(Metric),
		Query:   query.generateQuery(),
		Message: query.generateMessage(&stateContext.ServiceProperties.Notifications),
		Options: *alarmOption,
	}
	return *alarmSpec
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
	msg := fmt.Sprintf("High %s usage for deployment %s in %s %s", strings.ToUpper(string(q.AlarmType)),
		q.KubeDeployment, q.Location.Region, q.Location.EnvType)
	messageType := fmt.Sprintf(" [[#is_warning]] @%s [[/is_warning]] [[#is_warning_recovery]] @%s [[/is_warning_recovery]] [[#is_alert]] @%s [[/is_alert]][[#is_alert_recovery]] @%s [[/is_alert_recovery]]",
		notificationProp.LowPriorityPagerdutyEndpoint, notificationProp.LowPriorityPagerdutyEndpoint, notificationProp.PagerdutyEndpoint, notificationProp.PagerdutyEndpoint)
	return msg + messageType
}

func createAlarmNamesForSmithResource(resourceName voyager.ResourceName, deploymentResourceName smith_v1.ResourceName, alarmType AlarmType) smith_v1.ResourceName {
	nameList := strings.Join([]string{string(deploymentResourceName), string(alarmType)}, "--")
	return wiringutil.ServiceInstanceWithPostfixResourceName(resourceName, nameList)

}

func createAlarmNameforDatadog(serviceName string, resourceName string, alarmType string, env string, label string) string {
	nameList := []string{serviceName, resourceName, alarmType, env, label}
	return strings.Join(nameList, "-")
}
