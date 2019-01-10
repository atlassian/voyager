package sqs

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
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
	ResourceType voyager.ResourceType = "SQS"

	snsTopicArnReferenceNameSuffix = "TopicArn"

	clusterServiceClassExternalName = "sqs"
	clusterServiceClassExternalID   = "06068066-7f66-4297-8683-a1ba0a2b7401"
	clusterServicePlanExternalID    = "56393d2c-d936-4634-a178-19f491a3551a"
)

type snsSubscription struct {
	TopicArn   string                 `json:"topicArn"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type partialSqsAttributes struct {
	// this is a pointer so '==' comparisons to the empty struct work properly.
	Subscriptions *[]snsSubscription `json:"Subscriptions,omitempty"`
}

// WireUp has marked similarities to the aws autowiring functions, but because
// they're entangled with SvcCatEntangler it was unpleasant to reuse them without
// exposing too much. This is a separate function - for the moment - because
// it needs to understand how to wire the dependencies, which is atypical
// for aws-osb-provider resources.
func WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool, error) {
	if stateResource.Type != ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", stateResource.Type)
	}

	var wiredResources []smith_v1.Resource
	var snsSubscriptions []snsSubscription
	var references []smith_v1.Reference

	for _, dependency := range context.Dependencies {
		snsShape, hasSnsShape := dependency.Contract.FindShape(knownshapes.SnsSubscribableShape)
		if !hasSnsShape {
			return nil, false, errors.Errorf("sqs is allowed to depend only on sns resource, but SnsSubscribableShape was not found in %q", dependency.Name)
		}
		snsSubscribableData := snsShape.(*knownshapes.SnsSubscribable).Data

		resourceRef := snsSubscribableData.BindableShapeStruct.ServiceInstanceName
		serviceBinding := wiringutil.ConsumerProducerServiceBindingV2(stateResource.Name, dependency.Name, resourceRef)
		wiredResources = append(wiredResources, serviceBinding)

		referenceName := wiringutil.ReferenceName(serviceBinding.Name, snsTopicArnReferenceNameSuffix)
		topicArnRef := snsSubscribableData.TopicARN.ToReference(referenceName, serviceBinding.Name)
		references = append(references, topicArnRef)
		snsSubscriptions = append(snsSubscriptions, snsSubscription{
			TopicArn:   topicArnRef.Ref(),
			Attributes: dependency.Attributes,
		})
	}

	serviceInstance, err := constructServiceInstance(stateResource, context, references, snsSubscriptions)
	if err != nil {
		return nil, false, err
	}
	wiredResources = append(wiredResources, serviceInstance)

	result := &wiringplugin.WiringResult{
		Contract: wiringplugin.ResourceContract{
			Shapes: []wiringplugin.Shape{
				knownshapes.NewBindableEnvironmentVariables(serviceInstance.Name),
			},
		},
		Resources: wiredResources,
	}

	return result, false, nil
}

func constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext, references []smith_v1.Reference, snsSubscriptions []snsSubscription) (smith_v1.Resource, error) {
	instanceID, err := svccatentangler.InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	serviceName := context.StateContext.ServiceName
	userServiceName, err := oap.ServiceName(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	if userServiceName != "" {
		serviceName = userServiceName
	}
	attributes, alarms, err := constructSqsAttributes(resource, snsSubscriptions)
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

	serviceProperties := context.StateContext.ServiceProperties

	serviceInstanceSpec := oap.ServiceInstanceSpec{
		ServiceName: serviceName,
		Resource: oap.RPSResource{
			Name:       resourceName,
			Type:       clusterServiceClassExternalName,
			Attributes: attributes,
			Alarms:     alarms,
		},
		Environment: oap.ServiceEnvironment{
			LowPriorityPagerdutyEndpoint: serviceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch,
			PagerdutyEndpoint:            serviceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
			Tags:                         context.StateContext.Tags,
		},
	}
	serviceInstanceSpecBytes, err := json.Marshal(&serviceInstanceSpec)
	if err != nil {
		return smith_v1.Resource{}, err
	}

	return smith_v1.Resource{
		Name:       wiringutil.ServiceInstanceResourceName(resource.Name),
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.ServiceInstanceMetaName(resource.Name),
					Annotations: map[string]string{
						voyager.Domain + "/envResourcePrefix": clusterServiceClassExternalName,
					},
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
	}, nil
}

func constructSqsAttributes(resource *orch_v1.StateResource, subscriptions []snsSubscription) (json.RawMessage /* attributes */, json.RawMessage /* alarms */, error) {
	// The user shouldn't be setting anything in our 'partialSqsAttributes', since
	// _we_ control it. So let's make sure they're not and fail ASAP.
	if resource.Spec != nil {
		var currentPartialSpec partialSqsAttributes
		if err := json.Unmarshal(resource.Spec.Raw, &currentPartialSpec); err != nil {
			return nil, nil, errors.Wrap(err, "can't unmarshal state spec into JSON object")
		}
		if currentPartialSpec != (partialSqsAttributes{}) {
			return nil, nil, errors.Errorf("at least one autowired value not empty: %+v", currentPartialSpec)
		}
	}

	var subscriptionSpec map[string]interface{}
	if subscriptions != nil {
		var err error
		subscriptionSpec, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&partialSqsAttributes{
			Subscriptions: &subscriptions,
		})
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	}

	userSpec, err := oap.FilterAttributes(resource.Spec)
	if err != nil {
		return nil, nil, err
	}

	alarms, err := oap.Alarms(resource.Spec)
	if err != nil {
		return nil, nil, err
	}

	sqsAttributes, err := wiringutil.Merge(subscriptionSpec, userSpec)
	if err != nil {
		return nil, nil, err
	}

	var attributes []byte
	if len(sqsAttributes) == 0 {
		attributes = nil
	} else {
		attributes, err = json.Marshal(sqsAttributes)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	}

	return attributes, alarms, err
}
