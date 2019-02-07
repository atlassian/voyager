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
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	clusterServiceClassExternalID                      = "875d4a87-e887-4838-a0b5-b64491dbf9cb"
	clusterServicePlanExternalID                       = "d8048a2d-49de-4fda-b7ef-328de171cd32"
	ResourceType                  voyager.ResourceType = "datadog-alarm"
)

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

var (
	serviceInstanceGVK = sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")
)

func WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool, error) {
	err := validateRequest(stateResource, context)
	if err != nil {
		return nil, false, err
	}
	kubeComputeDependency, err := context.TheOnlyDependency()
	if err != nil {
		return nil, false, err
	}
	setOfScalingShape, found, err := knownshapes.FindSetOfPScalingShape(kubeComputeDependency.Contract.Shapes)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, errors.Errorf("failed to find shape %q in contract of %q", knownshapes.SetOfScalingShape, kubeComputeDependency.Name)
	}

	var wiredResources []smith_v1.Resource

	deploymentResourceName := setOfScalingShape.Data.DeploymentResourceName
	CPUServiceInstance, err := constructServiceInstance(stateResource, context, deploymentResourceName, CPU)
	if err != nil {
		return nil, false, err
	}
	wiredResources = append(wiredResources, CPUServiceInstance)

	memoryServiceInstance, err := constructServiceInstance(stateResource, context, deploymentResourceName, Memory)
	if err != nil {
		return nil, false, err
	}
	wiredResources = append(wiredResources, memoryServiceInstance)

	result := &wiringplugin.WiringResult{
		Resources: wiredResources,
	}

	return result, false, nil
}

func constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext, deploymentResourceName smith_v1.ResourceName, alarmType AlarmType) (smith_v1.Resource, error) {
	instanceID, err := svccatentangler.InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	threshold := AlarmThresholds{
		Critical: 90,
		Warning:  80,
	}
	query := generateQuerySpec(context, alarmType, threshold)
	alarmsAtt := createFinalAlarmSpec(resource, &context.StateContext, query)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	resourceName, err := oap.ResourceName(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	if resourceName == "" {
		resourceName = string(resource.Name)
	}

	resourceName = resourceName + "--" + string(alarmType)
	serviceInstanceSpec := ServiceInstanceSpec{
		ServiceName: context.StateContext.ServiceName,
		Attributes:  alarmsAtt,
		Environment: context.StateContext.Location.EnvType,
		Region:      context.StateContext.Location.Region,
	}

	serviceInstanceSpecBytes, err := json.Marshal(&serviceInstanceSpec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	smithResourceName := voyager.ResourceName(resourceName)
	alarmInstanceResource := smith_v1.Resource{
		Name: wiringutil.ServiceInstanceResourceName(smithResourceName),
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
					Name: wiringutil.ServiceInstanceMetaName(smithResourceName),
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

func generateQuerySpec(context *wiringplugin.WiringContext, alarmType AlarmType, threshold AlarmThresholds) QueryParams {
	return QueryParams{
		Env:            string(context.StateContext.Location.EnvType),
		Region:         string(context.StateContext.Location.Region),
		KubeDeployment: string(context.StateMeta.Name),
		KubeNamespace:  string(context.StateMeta.Namespace),
		AlarmType:      alarmType,
		Threshold:      &threshold,
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

func createFinalAlarmSpec(resource *orch_v1.StateResource, stateContext *wiringplugin.StateContext, query QueryParams) Alarm {
	alarmOption := &AlarmOption{
		EscalationMessage: query.generateMessage(&stateContext.ServiceProperties.Notifications),
		NotifyNOData:      false,
		RequireFullWindow: true,
		Thresholds:        *query.Threshold,
	}
	alarmSpec := &Alarm{
		Name:    string(stateContext.ServiceName) + "-" + string(resource.Name) + "-" + string(query.AlarmType),
		Type:    string(Metric),
		Query:   query.generateQuery(),
		Message: query.generateMessage(&stateContext.ServiceProperties.Notifications),
		Option:  *alarmOption,
	}
	return *alarmSpec
}

func (q *QueryParams) generateQuery() string {
	if q.AlarmType == CPU {
		cpuUsageString := fmt.Sprintf("avg(last_5m):( avg:kubernetes.cpu.usage.total{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} ", q.Env, q.KubeNamespace, q.KubeDeployment, q.Region)
		cpuLimitString := fmt.Sprintf("/ ( avg:kubernetes.cpu.limits{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} * 1000000 ) ) * 100 > %d", q.Env, q.KubeNamespace, q.KubeDeployment, q.Region, q.Threshold.Critical)
		return cpuUsageString + cpuLimitString
	}

	memoryUsageString := fmt.Sprintf("avg(last_5m):( avg:kubernetes.memory.usage.total{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} ", q.Env, q.KubeNamespace, q.KubeDeployment, q.Region)
	memoryLimitString := fmt.Sprintf("/ ( avg:kubernetes.memory.limits{env:%s,kube_namespace:%s,kube_deployment:%s,region:%s} by {container_id} * 1000000 ) ) * 100 > %d", q.Env, q.KubeNamespace, q.KubeDeployment, q.Region, q.Threshold.Critical)
	return memoryUsageString + memoryLimitString

}

func (q *QueryParams) generateMessage(notificationProp *meta_orch.Notifications) string {
	msg := fmt.Sprintf("High %s usage for deployment %s in %s %s @%s", strings.ToUpper(string(q.AlarmType)),
		q.KubeDeployment, q.Region, q.Env, notificationProp.PagerdutyEndpoint)
	return msg
}