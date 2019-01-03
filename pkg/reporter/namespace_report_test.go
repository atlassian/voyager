package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/SermoDigital/jose/jws"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ops"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

type MockASAPConfig struct{}

func (*MockASAPConfig) GenerateToken(audience string, subject string) ([]byte, error) {
	return []byte("ASAP Token"), nil
}
func (*MockASAPConfig) GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error) {
	return []byte("ASAP Token"), nil
}
func (*MockASAPConfig) KeyID() string     { return "" }
func (*MockASAPConfig) KeyIssuer() string { return "" }

type MockProvider struct {
	name     string
	plan     string
	response http.Response
}

func (MockProvider) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (MockProvider) ProxyRequest(asapConfig pkiutil.ASAP, w http.ResponseWriter, r *http.Request, uri string) {
	w.Write([]byte("Successful proxy"))
}

func (m MockProvider) Request(asapConfig pkiutil.ASAP, r *http.Request, uri string, user string) (*http.Response, error) {
	return &m.response, nil
}

func (m MockProvider) Name() string {
	return m.name
}

func (m MockProvider) OwnsPlan(plan string) bool {
	return plan == m.plan
}

func (m MockProvider) ReportAction() string {
	return "info"
}

var (
	currentLocation = voyager.Location{
		Account: "123456789",
		Region:  "us-east-1",
		EnvType: "dev",
	}
	myDev1 = comp_v1.ServiceDescriptorLocation{
		Name:    comp_v1.ServiceDescriptorLocationName("my-dev-1"),
		Account: voyager.Account("123456789"),
		Region:  voyager.Region("us-east-1"),
		EnvType: voyager.EnvType("dev"),
		Label:   voyager.Label("this is a label"),
	}
	myDev2 = comp_v1.ServiceDescriptorLocation{
		Name:    comp_v1.ServiceDescriptorLocationName("my-dev-2"),
		Account: voyager.Account("123456789"),
		Region:  voyager.Region("us-east-1"),
		EnvType: voyager.EnvType("dev"),
		Label:   voyager.Label("this is a label"),
	}
	myStg1 = comp_v1.ServiceDescriptorLocation{
		Name:    comp_v1.ServiceDescriptorLocationName("my-staging"),
		Account: voyager.Account("9874563"),
		Region:  voyager.Region("us-west-1"),
		EnvType: voyager.EnvType("stg"),
		Label:   voyager.Label("this is not a label"),
	}

	blitzTest1 = comp_v1.ServiceDescriptorResourceGroup{
		Name:      "blitztest1",
		Locations: []comp_v1.ServiceDescriptorLocationName{myDev1.Name, myDev2.Name},
	}
	blitzTest2 = comp_v1.ServiceDescriptorResourceGroup{
		Name:      "blitztest2",
		Locations: []comp_v1.ServiceDescriptorLocationName{myDev1.Name, myDev2.Name, myStg1.Name},
	}
	blitzTest2Rep = comp_v1.ServiceDescriptorResourceGroup{
		Name:      "blitztest2",
		Locations: []comp_v1.ServiceDescriptorLocationName{myDev1.Name, myDev2.Name},
	}

	blitzTest3 = comp_v1.ServiceDescriptorResourceGroup{
		Name:      "blitzTest3",
		Locations: []comp_v1.ServiceDescriptorLocationName{myStg1.Name},
	}

	config1 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("global"),
	}
	config2 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("dev.us-east-1"),
	}
	config3 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("dev.us-west-1"),
	}
	config4 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("dev.us-east-1..123456789"),
	}
	config5 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("dev.us-east-1.hello.123456789"),
	}
	config6 = comp_v1.ServiceDescriptorConfigSet{
		Scope: comp_v1.Scope("dev.us-east-1..6789056445"),
	}

	testTimeError                  = meta_v1.Date(2018, time.January, 0, 0, 0, 0, 0, time.UTC)
	testTimeInProgress             = meta_v1.Date(2018, time.February, 0, 0, 0, 0, 0, time.UTC)
	testTimeReady                  = meta_v1.Date(2018, time.March, 0, 0, 0, 0, 0, time.UTC)
	testExpectedReplicaCount int32 = 2
	testObjects                    = []runtime.Object{
		&comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       comp_v1.ServiceDescriptorResourceKind,
				APIVersion: comp_v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test",
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations:      []comp_v1.ServiceDescriptorLocation{},
				Config:         []comp_v1.ServiceDescriptorConfigSet{},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
				Version:        "0.0.1",
			},
			Status: comp_v1.ServiceDescriptorStatus{
				Conditions: []cond_v1.Condition{
					{
						Type:   cond_v1.ConditionError,
						Status: cond_v1.ConditionFalse,
					},
					{
						Type:               cond_v1.ConditionInProgress,
						Status:             cond_v1.ConditionTrue,
						LastTransitionTime: testTimeInProgress,
						Message:            "Still doing a thing",
					},
				},
			},
		},
		&form_v1.LocationDescriptor{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       form_v1.LocationDescriptorResourceKind,
				APIVersion: form_v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test",
			},
			Spec: form_v1.LocationDescriptorSpec{
				ConfigMapName: "cfg",
				Resources: []form_v1.LocationDescriptorResource{
					form_v1.LocationDescriptorResource{
						Name: "old-resource",
						Type: "ec2compute",
						Spec: &runtime.RawExtension{
							Raw: []byte("size:medium"),
						},
					},
				},
			},
			Status: form_v1.LocationDescriptorStatus{
				Conditions: []cond_v1.Condition{
					{
						Type:   cond_v1.ConditionError,
						Status: cond_v1.ConditionFalse,
					},
					{
						Type:               cond_v1.ConditionInProgress,
						Status:             cond_v1.ConditionTrue,
						LastTransitionTime: testTimeInProgress,
						Message:            "Doing a thing",
					},
				},
				ResourceStatuses: []form_v1.ResourceStatus{
					form_v1.ResourceStatus{
						Name: "old-resource",
						Conditions: []cond_v1.Condition{
							{
								Type:   cond_v1.ConditionError,
								Status: cond_v1.ConditionFalse,
							},
							{
								Type:               cond_v1.ConditionInProgress,
								Status:             cond_v1.ConditionTrue,
								LastTransitionTime: testTimeInProgress,
								Message:            "Doing a thing",
							},
						},
					},
				},
			},
		},
		&orch_v1.State{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       orch_v1.StateResourceKind,
				APIVersion: orch_v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test",
			},
			Spec: orch_v1.StateSpec{
				ConfigMapName: "cm1",
				Resources: []orch_v1.StateResource{
					orch_v1.StateResource{
						Name: "messages",
						Type: "sns",
					},
					orch_v1.StateResource{
						Name: "events",
						Type: "sqs",
						DependsOn: []orch_v1.StateDependency{
							orch_v1.StateDependency{
								Name: "messages",
								Attributes: map[string]interface{}{
									"MaxReceiveCount": "100",
								},
							},
						},
						Spec: &runtime.RawExtension{
							Object: unstructuredObject,
						},
					},
				},
			},
			Status: orch_v1.StateStatus{
				Conditions: []cond_v1.Condition{
					{
						Status:             cond_v1.ConditionTrue,
						Type:               cond_v1.ConditionInProgress,
						LastTransitionTime: testTimeInProgress,
					},
				},
			},
		},
		&smith_v1.Bundle{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       smith_v1.BundleResourceKind,
				APIVersion: smith_v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test",
			},
			Status: smith_v1.BundleStatus{
				ResourceStatuses: []smith_v1.ResourceStatus{
					smith_v1.ResourceStatus{
						Name: smith_v1.ResourceName("events--instance"),
						Conditions: []cond_v1.Condition{
							{
								Type:   smith_v1.ResourceInProgress,
								Status: cond_v1.ConditionFalse,
							},
							{
								Type:               smith_v1.ResourceReady,
								Status:             cond_v1.ConditionTrue,
								Message:            "success",
								LastTransitionTime: testTimeReady,
							},
							{
								Type:   smith_v1.ResourceError,
								Status: cond_v1.ConditionFalse,
							},
						},
					},
				},
				PluginStatuses: []smith_v1.PluginStatus{
					smith_v1.PluginStatus{
						Name:   smith_v1.PluginName("compute--iamrole"),
						Status: smith_v1.PluginStatusOk,
					},
				},
				Conditions: []cond_v1.Condition{
					{
						Type:               smith_v1.BundleInProgress,
						LastTransitionTime: testTimeInProgress,
						Status:             cond_v1.ConditionTrue,
					},
					{
						Type:               smith_v1.BundleError,
						Message:            "fluffed it",
						LastTransitionTime: testTimeError,
						Status:             cond_v1.ConditionTrue,
					},
					{
						Type:               smith_v1.BundleReady,
						LastTransitionTime: testTimeReady,
						Status:             cond_v1.ConditionFalse,
					},
				},
			},

			Spec: smith_v1.BundleSpec{
				Resources: []smith_v1.Resource{
					smith_v1.Resource{
						Name: "messages--instance",
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceInstance{
								Spec: sc_v1b1.ServiceInstanceSpec{
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: "cloudformation",
										ClusterServicePlanExternalName:  "default",
									},
								},
							},
						},
					},
					smith_v1.Resource{
						Name: "events--instance",
						References: []smith_v1.Reference{
							smith_v1.Reference{
								Name:     "events--messages--binding-topicarn",
								Resource: "events--messages--binding",
								Path:     "data.TopicArn",
							},
						},
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceInstance{
								Spec: sc_v1b1.ServiceInstanceSpec{
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: "cloudformation",
										ClusterServicePlanExternalName:  "default",
									},
								},
							},
						},
					},
					smith_v1.Resource{
						Name: "compute--iamrole",
						References: []smith_v1.Reference{
							smith_v1.Reference{
								Resource: "compute--messages--binding",
							},
						},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       "iamrole",
								ObjectName: "compute--iamrole",
							},
						},
					},
				},
			},
		},
		&sc_v1b1.ServiceInstance{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       "test",
				Generation: 4,
				Labels: map[string]string{
					"atlassian.io/user": "user",
				},
				Annotations: map[string]string{
					"annotation": "value",
				},
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ServiceInstance",
				APIVersion: "servicecatalog.k8s.io/v1beta1",
			},
			Spec: sc_v1b1.ServiceInstanceSpec{
				PlanReference: sc_v1b1.PlanReference{
					ClusterServiceClassExternalName: "dummy",
					ClusterServicePlanExternalName:  "default",
				},
				ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
					Name: "class-id",
				},
				ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
					Name: "plan-id",
				},
			},
		},
		&sc_v1b1.ServiceBinding{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-binding",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ServiceBinding",
				APIVersion: "servicecatalog.k8s.io/v1beta1",
			},
			Spec: sc_v1b1.ServiceBindingSpec{
				ExternalID: "1234",
			},
		},
		&core_v1.ConfigMap{
			ObjectMeta: meta_v1.ObjectMeta{
				UID:  "configmap-uuid",
				Name: "test",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.ConfigMapKind,
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
			Data: map[string]string{
				"Owner": "User",
			},
		},
		&ext_v1beta1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-ingress",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.IngressKind,
				APIVersion: ext_v1beta1.SchemeGroupVersion.String(),
			},
			Spec: ext_v1beta1.IngressSpec{
				Backend: &ext_v1beta1.IngressBackend{},
				Rules: []ext_v1beta1.IngressRule{
					ext_v1beta1.IngressRule{
						Host:             "test.ap-southeast-2.dev.k8s.atl-paas.net",
						IngressRuleValue: ext_v1beta1.IngressRuleValue{},
					},
				},
			},
		},
		&apps_v1.Deployment{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-deployment",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.DeploymentKind,
				APIVersion: apps_v1.SchemeGroupVersion.String(),
			},
			Spec: apps_v1.DeploymentSpec{
				Template: core_v1.PodTemplateSpec{
					Spec: core_v1.PodSpec{
						Containers: []core_v1.Container{
							core_v1.Container{
								Image: "docker.example.com/foo/bar",
							},
						},
					},
				},
			},
			Status: apps_v1.DeploymentStatus{
				Conditions: []apps_v1.DeploymentCondition{
					{
						Type:   apps_v1.DeploymentAvailable,
						Status: core_v1.ConditionTrue,
					},
				},
			},
		},
		&apps_v1.ReplicaSet{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-replicaset",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.ReplicaSetKind,
				APIVersion: apps_v1.SchemeGroupVersion.String(),
			},
			Spec: apps_v1.ReplicaSetSpec{
				Replicas: &testExpectedReplicaCount,
				Template: core_v1.PodTemplateSpec{
					Spec: core_v1.PodSpec{
						Containers: []core_v1.Container{
							core_v1.Container{
								Image: "docker.example.com/foo/bar",
							},
						},
					},
				},
			},
			Status: apps_v1.ReplicaSetStatus{
				AvailableReplicas: testExpectedReplicaCount,
			},
		},
		&core_v1.Pod{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-pod",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.PodKind,
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
			Spec: core_v1.PodSpec{
				Containers: []core_v1.Container{
					core_v1.Container{
						Image: "docker.example.com/foo/bar",
					},
				},
			},
			Status: core_v1.PodStatus{
				Phase:   core_v1.PodPending,
				Message: "Pod message",
			},
		},
		&autoscaling_v2b1.HorizontalPodAutoscaler{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-hpa",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.HorizontalPodAutoscalerKind,
				APIVersion: autoscaling_v2b1.SchemeGroupVersion.String(),
			},
			Spec: autoscaling_v2b1.HorizontalPodAutoscalerSpec{
				MaxReplicas: 3,
			},
			Status: autoscaling_v2b1.HorizontalPodAutoscalerStatus{
				Conditions: []autoscaling_v2b1.HorizontalPodAutoscalerCondition{
					{
						Type:   autoscaling_v2b1.ScalingActive,
						Status: core_v1.ConditionTrue,
					},
				},
			},
		},
		&core_v1.Event{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.EventKind,
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
			InvolvedObject: core_v1.ObjectReference{
				UID: "configmap-uuid",
			},
			Reason: "I made a ConfigMap, boss!",
		},
	}

	unstructuredObject = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"unstructured": "content",
		},
	}

	expectedReport = &reporter_v1.Report{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Report",
			APIVersion: "reporter.voyager.atl-paas.net/v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service",
			Namespace: "namespace",
		},
		Report: reporter_v1.NamespaceReport{
			Composition: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{
					reporter_v1.Resource{
						Name:         "test",
						ResourceType: "ServiceDescriptor",
						Spec: &comp_v1.ServiceDescriptorSpec{
							Locations:      []comp_v1.ServiceDescriptorLocation{},
							Config:         nil,
							ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
							Version:        "0.0.1",
						},
						Version: "33f850e254770a86848b957ee9e5da9163b72d1b",
						Status: reporter_v1.Status{
							Status:    "InProgress",
							Timestamp: &testTimeInProgress,
							Reason:    "Still doing a thing",
						},
					},
				},
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Timestamp: &testTimeInProgress,
					Reason:    "Still doing a thing",
				},
			},
			Formation: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{
					reporter_v1.Resource{
						Name:         "old-resource",
						ResourceType: "ec2compute",
						References:   []reporter_v1.Reference{},
						Spec: &runtime.RawExtension{
							Raw: []byte("size:medium"),
						},
						Status: reporter_v1.Status{
							Status:    "InProgress",
							Reason:    "Doing a thing",
							Timestamp: &testTimeInProgress,
						},
					},
				},
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Reason:    "Doing a thing",
					Timestamp: &testTimeInProgress,
				},
			},
			Orchestration: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{
					reporter_v1.Resource{
						Name:         "messages",
						Version:      "",
						ResourceType: "sns",
						References:   []reporter_v1.Reference{},
					},
					reporter_v1.Resource{
						Name:         "events",
						Version:      "",
						ResourceType: "sqs",
						References: []reporter_v1.Reference{
							reporter_v1.Reference{
								Name:  "messages",
								Layer: "orchestration",
								Attributes: map[string]interface{}{
									"MaxReceiveCount": "100",
								},
							},
						},
						Spec: unstructuredObject,
					},
				},
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Timestamp: &testTimeInProgress,
				},
			},
			Execution: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{
					reporter_v1.Resource{
						Name: "messages--instance",
						Spec: &sc_v1b1.ServiceInstance{
							Spec: sc_v1b1.ServiceInstanceSpec{
								PlanReference: sc_v1b1.PlanReference{
									ClusterServiceClassExternalName: "cloudformation",
									ClusterServicePlanExternalName:  "default",
								},
							},
						},
						Status: reporter_v1.Status{
							Status: "Unknown",
							Reason: "No active condition found",
						},
						References: []reporter_v1.Reference{},
					},
					reporter_v1.Resource{
						Name: "events--instance",
						Spec: &sc_v1b1.ServiceInstance{
							Spec: sc_v1b1.ServiceInstanceSpec{
								PlanReference: sc_v1b1.PlanReference{
									ClusterServiceClassExternalName: "cloudformation",
									ClusterServicePlanExternalName:  "default",
								},
							},
						},
						Status: reporter_v1.Status{
							Status:    "Ready",
							Reason:    "success",
							Timestamp: &testTimeReady,
						},
						References: []reporter_v1.Reference{
							reporter_v1.Reference{
								Name:  "events--messages--binding",
								Layer: "execution",
								Attributes: map[string]interface{}{
									"name": smith_v1.ReferenceName("events--messages--binding-topicarn"),
									"path": "data.TopicArn",
								},
							},
						},
					},
					reporter_v1.Resource{
						Name: "compute--iamrole",
						Spec: &smith_v1.PluginSpec{
							Name:       "iamrole",
							ObjectName: "compute--iamrole",
							Spec:       nil,
						},
						Status: reporter_v1.Status{
							Status: "Ok",
						},
						References: []reporter_v1.Reference{
							reporter_v1.Reference{
								Name:  "compute--messages--binding",
								Layer: "execution",
							},
						},
					},
				},
				Status: reporter_v1.Status{
					Status:    "Retrying",
					Reason:    "Retrying after failure: fluffed it",
					Timestamp: &testTimeInProgress,
				},
			},
			Objects: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{
					reporter_v1.Resource{
						Name:         "test",
						ResourceType: "ServiceInstance",
						Status: reporter_v1.Status{
							Status: "Unknown",
							Reason: "No active condition found",
						},
						References: []reporter_v1.Reference{},
						Spec: sc_v1b1.ServiceInstanceSpec{
							PlanReference: sc_v1b1.PlanReference{
								ClusterServiceClassExternalName: "dummy",
								ClusterServicePlanExternalName:  "default",
							},
							ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
								Name: "class-id",
							},
							ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
								Name: "plan-id",
							},
						},
						Provider: &reporter_v1.ResourceProvider{
							ClassID:    "class-id",
							PlanID:     "plan-id",
							Namespaced: false,
						},
					},
					reporter_v1.Resource{
						Name:         "test-binding",
						ResourceType: "ServiceBinding",
						Status: reporter_v1.Status{
							Status: "Unknown",
							Reason: "No active condition found",
						},
						References: []reporter_v1.Reference{},
						Spec: sc_v1b1.ServiceBindingSpec{
							ExternalID: "1234",
						},
						UID: "1234",
					},
					reporter_v1.Resource{
						Name:         "test",
						ResourceType: k8s.ConfigMapKind,
						Status: reporter_v1.Status{
							Status: "Ready",
						},
						References: []reporter_v1.Reference{},
						Spec: map[string]string{
							"Owner": "User",
						},
						UID: "configmap-uuid",
						Events: []reporter_v1.Event{
							{Reason: "I made a ConfigMap, boss!"},
						},
					},
					reporter_v1.Resource{
						Name:         "test-ingress",
						ResourceType: k8s.IngressKind,
						Status: reporter_v1.Status{
							Status: "Ready",
						},
						References: []reporter_v1.Reference{},
						Spec: ext_v1beta1.IngressSpec{
							Backend: &ext_v1beta1.IngressBackend{},
							Rules: []ext_v1beta1.IngressRule{
								ext_v1beta1.IngressRule{
									Host:             "test.ap-southeast-2.dev.k8s.atl-paas.net",
									IngressRuleValue: ext_v1beta1.IngressRuleValue{},
								},
							},
						},
					},
					reporter_v1.Resource{
						Name:         "test-deployment",
						ResourceType: k8s.DeploymentKind,
						Status: reporter_v1.Status{
							Status: string(apps_v1.DeploymentAvailable),
						},
						References: []reporter_v1.Reference{},
						Spec: apps_v1.DeploymentSpec{
							Template: core_v1.PodTemplateSpec{
								Spec: core_v1.PodSpec{
									Containers: []core_v1.Container{
										core_v1.Container{
											Image: "docker.example.com/foo/bar",
										},
									},
								},
							},
						},
					},
					reporter_v1.Resource{
						Name:         "test-replicaset",
						ResourceType: k8s.ReplicaSetKind,
						References:   []reporter_v1.Reference{},
						Spec: apps_v1.ReplicaSetSpec{
							Replicas: &testExpectedReplicaCount,
							Template: core_v1.PodTemplateSpec{
								Spec: core_v1.PodSpec{
									Containers: []core_v1.Container{
										core_v1.Container{
											Image: "docker.example.com/foo/bar",
										},
									},
								},
							},
						},
						Status: reporter_v1.Status{
							Status: "Ready",
						},
					},
					reporter_v1.Resource{
						Name:         "test-pod",
						ResourceType: k8s.PodKind,
						References:   []reporter_v1.Reference{},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								core_v1.Container{
									Image: "docker.example.com/foo/bar",
								},
							},
						},
						Status: reporter_v1.Status{
							Status: "Pending",
							Reason: "Pod message",
						},
					},
					reporter_v1.Resource{
						Name:         "test-hpa",
						ResourceType: k8s.HorizontalPodAutoscalerKind,
						References:   []reporter_v1.Reference{},
						Spec: autoscaling_v2b1.HorizontalPodAutoscalerSpec{
							MaxReplicas: 3,
						},
						Status: reporter_v1.Status{
							Status: string(autoscaling_v2b1.ScalingActive),
						},
					},
				},
			},
			Providers: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
		},
	}

	userInfo = &user.DefaultInfo{
		Name: "user",
	}
)

func TestHandlesHappyCreation(t *testing.T) {
	t.Parallel()

	nrh, err := NewNamespaceReportHandler("namespace", "service", copiedTestObject(), RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t))

	report := nrh.GenerateReport(ctx, map[string]ops.ProviderInterface{}, &MockASAPConfig{})

	stripNowTimestamps(&report.Report)
	require.Equal(t, *expectedReport, report)
}

func TestHandlesProviderProxy(t *testing.T) {
	t.Parallel()
	objs := copiedTestObject()
	objs = append(objs, &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: "servicecatalog.k8s.io/v1beta1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sns",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ServiceClassExternalName: "cloudformation",
				ServicePlanExternalName:  "default",
			},
			ServiceClassRef: &sc_v1b1.LocalObjectReference{
				Name: "class-id",
			},
			ServicePlanRef: &sc_v1b1.LocalObjectReference{
				Name: "plan-id",
			},
		},
		Status: sc_v1b1.ServiceInstanceStatus{
			Conditions: []sc_v1b1.ServiceInstanceCondition{
				sc_v1b1.ServiceInstanceCondition{
					Type:    "Ready",
					Status:  "True",
					Message: "all done",
				},
			},
		},
	})

	resp := http.Response{
		StatusCode: 200,
	}

	providerResp, err := json.Marshal(ProviderResponse{
		Name: "instance",
		Status: reporter_v1.Status{
			Status: "complete",
		},
		Properties: map[string]interface{}{"output": "vars"},
		Spec:       map[string]interface{}{"input": "vars"},
		Version:    "latest",
	})
	require.NoError(t, err)
	resp.Body = ioutil.NopCloser(bytes.NewReader(providerResp))

	providers := map[string]ops.ProviderInterface{
		"cloudformation": &MockProvider{
			name:     "cloudformation",
			plan:     "uuid",
			response: resp,
		},
	}
	nrh, err := NewNamespaceReportHandler("namespace", "service", objs, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, providers, &MockASAPConfig{})

	expected := *expectedReport
	expected.Report.Objects.Resources = append(expected.Report.Objects.Resources, reporter_v1.Resource{
		Name:         "sns",
		ResourceType: "ServiceInstance",
		Status: reporter_v1.Status{
			Status: "Ready",
			Reason: "all done",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ServiceClassExternalName: "cloudformation",
				ServicePlanExternalName:  "default",
			},
			ServiceClassRef: &sc_v1b1.LocalObjectReference{
				Name: "class-id",
			},
			ServicePlanRef: &sc_v1b1.LocalObjectReference{
				Name: "plan-id",
			},
		},
		Provider: &reporter_v1.ResourceProvider{
			ClassID:    "class-id",
			PlanID:     "plan-id",
			Namespaced: true,
		},
		References: []reporter_v1.Reference{},
	})

	expected.Report.Providers.Resources = append(expected.Report.Providers.Resources, reporter_v1.Resource{
		Name:    "instance",
		Version: "latest",
		Status: reporter_v1.Status{
			Status: "complete",
		},
		ResourceType: "cloudformation",
		Properties:   map[string]interface{}{"output": "vars"},
		Spec:         map[string]interface{}{"input": "vars"},
		References: []reporter_v1.Reference{
			reporter_v1.Reference{
				Layer: "object",
				Name:  "sns",
			},
		},
	})

	stripNowTimestamps(&report.Report)
	require.Equal(t, expected, report)
}

func TestHandlesInProgressServiceInstance(t *testing.T) {
	t.Parallel()
	objs := copiedTestObject()
	objs = append(objs, &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: "servicecatalog.k8s.io/v1beta1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sns",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServicePlanExternalName: "default",
			},
			ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
				Name: "uuid",
			},
		},
		Status: sc_v1b1.ServiceInstanceStatus{
			AsyncOpInProgress: true,
			Conditions: []sc_v1b1.ServiceInstanceCondition{
				sc_v1b1.ServiceInstanceCondition{
					Type:               "Ready",
					Status:             "false",
					Message:            "Waiting on Cloudformation",
					LastTransitionTime: testTimeInProgress,
				},
			},
		},
	})

	providers := map[string]ops.ProviderInterface{}
	nrh, err := NewNamespaceReportHandler("namespace", "service", objs, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, providers, &MockASAPConfig{})

	expected := *expectedReport
	expected.Report.Objects.Resources = append(expected.Report.Objects.Resources, reporter_v1.Resource{
		Name:         "sns",
		ResourceType: "ServiceInstance",
		Status: reporter_v1.Status{
			Status:    "InProgress",
			Reason:    "Waiting on Cloudformation",
			Timestamp: &testTimeInProgress,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServicePlanExternalName: "default",
			},
			ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
				Name: "uuid",
			},
		},
		References: []reporter_v1.Reference{},
		Provider: &reporter_v1.ResourceProvider{
			PlanID: "uuid",
		},
	})

	stripNowTimestamps(&report.Report)
	require.Equal(t, expected, report)
}

func TestHandlesOtherServiceCatalogConditions(t *testing.T) {
	t.Parallel()
	objs := copiedTestObject()
	objs = append(objs, &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: "servicecatalog.k8s.io/v1beta1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sns",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServicePlanExternalName: "default",
			},
			ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
				Name: "uuid",
			},
		},
		Status: sc_v1b1.ServiceInstanceStatus{
			AsyncOpInProgress: false,
			Conditions: []sc_v1b1.ServiceInstanceCondition{
				sc_v1b1.ServiceInstanceCondition{
					Type:               "Ready",
					Status:             "false",
					Message:            "Deprovisioning Failed: Waiting on Cloudformation",
					LastTransitionTime: testTimeInProgress,
				},
			},
		},
	})

	providers := map[string]ops.ProviderInterface{}
	nrh, err := NewNamespaceReportHandler("namespace", "service", objs, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, providers, &MockASAPConfig{})

	expected := *expectedReport
	expected.Report.Objects.Resources = append(expected.Report.Objects.Resources, reporter_v1.Resource{
		Name:         "sns",
		ResourceType: "ServiceInstance",
		Status: reporter_v1.Status{
			Status:    "InProgress",
			Reason:    "Deprovisioning Failed: Waiting on Cloudformation",
			Timestamp: &testTimeInProgress,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServicePlanExternalName: "default",
			},
			ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
				Name: "uuid",
			},
		},
		References: []reporter_v1.Reference{},
		Provider: &reporter_v1.ResourceProvider{
			PlanID: "uuid",
		},
	})

	stripNowTimestamps(&report.Report)
	require.Equal(t, expected, report)
}

func TestCompositionLocationFiltering(t *testing.T) {
	t.Parallel()

	obj := copiedTestObject()

	testDes := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: comp_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations:      []comp_v1.ServiceDescriptorLocation{myDev1, myDev2, myStg1},
			Config:         []comp_v1.ServiceDescriptorConfigSet{},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{blitzTest1, blitzTest2},
			Version:        "0.0.1",
		},
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					Type:   cond_v1.ConditionError,
					Status: cond_v1.ConditionFalse,
				},
				{
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
					LastTransitionTime: testTimeInProgress,
					Message:            "Still doing a thing",
				},
			},
		},
	}

	obj[0] = testDes

	composition := reporter_v1.ReportLayer{
		Resources: []reporter_v1.Resource{
			reporter_v1.Resource{
				Name:         "test",
				ResourceType: "ServiceDescriptor",
				Version:      "049455bbf9174d9900231288829c87455e75b076",
				Spec: &comp_v1.ServiceDescriptorSpec{
					Locations:      []comp_v1.ServiceDescriptorLocation{myDev1, myDev2},
					Config:         nil,
					ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{blitzTest1, blitzTest2Rep},
					Version:        "0.0.1",
				},
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Timestamp: &testTimeInProgress,
					Reason:    "Still doing a thing",
				},
			},
		},
		Status: reporter_v1.Status{
			Status:    "InProgress",
			Timestamp: &testTimeInProgress,
			Reason:    "Still doing a thing",
		},
	}
	copyReport := *expectedReport
	copyReport.Report.Composition = composition

	nrh, err := NewNamespaceReportHandler("namespace", "service", obj, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, map[string]ops.ProviderInterface{}, &MockASAPConfig{})
	stripNowTimestamps(&report.Report)

	require.Equal(t, copyReport, report)
}

func TestCompositionLocationFilteringShouldNotReturnResourceGroups(t *testing.T) {
	t.Parallel()

	obj := copiedTestObject()

	testDes := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: comp_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations:      []comp_v1.ServiceDescriptorLocation{myDev1, myStg1},
			Config:         []comp_v1.ServiceDescriptorConfigSet{},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{blitzTest3},
			Version:        "0.0.1",
		},
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					Type:   cond_v1.ConditionError,
					Status: cond_v1.ConditionFalse,
				},
				{
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
					LastTransitionTime: testTimeInProgress,
					Message:            "Still doing a thing",
				},
			},
		},
	}

	obj[0] = testDes

	composition := reporter_v1.ReportLayer{
		Resources: []reporter_v1.Resource{
			reporter_v1.Resource{
				Name:         "test",
				ResourceType: "ServiceDescriptor",
				Version:      "fac431f66caa7b3adfce8cc81c8cca29a5aedd43",
				Spec: &comp_v1.ServiceDescriptorSpec{
					Locations:      []comp_v1.ServiceDescriptorLocation{myDev1},
					Config:         nil,
					ResourceGroups: nil,
					Version:        "0.0.1",
				},
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Timestamp: &testTimeInProgress,
					Reason:    "Still doing a thing",
				},
			},
		},
		Status: reporter_v1.Status{
			Status:    "InProgress",
			Timestamp: &testTimeInProgress,
			Reason:    "Still doing a thing",
		},
	}
	copyReport := *expectedReport
	copyReport.Report.Composition = composition

	nrh, err := NewNamespaceReportHandler("namespace", "service", obj, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, map[string]ops.ProviderInterface{}, &MockASAPConfig{})
	stripNowTimestamps(&report.Report)

	require.Equal(t, copyReport, report)
}

func TestCompositionLocationFilteringConfig(t *testing.T) {
	t.Parallel()

	obj := copiedTestObject()

	testDes := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: comp_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations:      []comp_v1.ServiceDescriptorLocation{},
			Config:         []comp_v1.ServiceDescriptorConfigSet{config1, config2, config3, config4, config5, config6},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
			Version:        "0.0.1",
		},
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					Type:   cond_v1.ConditionError,
					Status: cond_v1.ConditionFalse,
				},
				{
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
					LastTransitionTime: testTimeInProgress,
					Message:            "Still doing a thing",
				},
			},
		},
	}

	obj[0] = testDes

	composition := reporter_v1.ReportLayer{
		Resources: []reporter_v1.Resource{
			reporter_v1.Resource{
				Name:         "test",
				ResourceType: "ServiceDescriptor",
				Spec: &comp_v1.ServiceDescriptorSpec{
					Locations:      []comp_v1.ServiceDescriptorLocation{},
					Config:         []comp_v1.ServiceDescriptorConfigSet{config1, config2, config4, config5},
					ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
					Version:        "0.0.1",
				},
				Version: "11ae78cb4f6ff3589763d461463ad133875eff07",
				Status: reporter_v1.Status{
					Status:    "InProgress",
					Timestamp: &testTimeInProgress,
					Reason:    "Still doing a thing",
				},
			},
		},
		Status: reporter_v1.Status{
			Status:    "InProgress",
			Timestamp: &testTimeInProgress,
			Reason:    "Still doing a thing",
		},
	}

	copyRep := *expectedReport
	copyRep.Report.Composition = composition

	nrh, err := NewNamespaceReportHandler("namespace", "service", obj, RequestFilter{}, currentLocation)
	require.NoError(t, err)

	ctx := request.WithUser(logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t)), userInfo)

	report := nrh.GenerateReport(ctx, map[string]ops.ProviderInterface{}, &MockASAPConfig{})
	stripNowTimestamps(&report.Report)

	require.Equal(t, copyRep, report)
}

func copiedTestObject() []runtime.Object {
	objs := make([]runtime.Object, 0, len(testObjects))
	for _, o := range testObjects {
		objs = append(objs, o)
	}
	return objs
}

func stripNowTimestamps(report *reporter_v1.NamespaceReport) {
	report.Composition.Resources = stripResourceTimestamps(report.Composition.Resources)
	report.Formation.Resources = stripResourceTimestamps(report.Formation.Resources)
	report.Orchestration.Resources = stripResourceTimestamps(report.Orchestration.Resources)
	report.Execution.Resources = stripResourceTimestamps(report.Execution.Resources)
	report.Objects.Resources = stripResourceTimestamps(report.Objects.Resources)
	report.Providers.Resources = stripResourceTimestamps(report.Providers.Resources)
}

func stripResourceTimestamps(resources []reporter_v1.Resource) []reporter_v1.Resource {
	for i := range resources {
		if !(resources[i].Status.Timestamp.Equal(&testTimeInProgress) ||
			resources[i].Status.Timestamp.Equal(&testTimeError) ||
			resources[i].Status.Timestamp.Equal(&testTimeReady)) {

			resources[i].Status.Timestamp = nil
		}
	}
	return resources
}
