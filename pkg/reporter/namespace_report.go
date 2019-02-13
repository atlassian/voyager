package reporter

import (
	"context"
	"crypto/sha1" //nolint: gosec
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ops"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
)

const (
	infoURI     = "/v2/service_instances/%s/x-operation_instances/%s"
	global      = "global"
	envTypeItem = 0
	regionItem  = 1
	accountItem = 3
)

var (
	voyagerLayers = map[string]schema.GroupVersionKind{
		reporter_v1.LayerComposition:   comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind),
		reporter_v1.LayerFormation:     form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind),
		reporter_v1.LayerOrchestration: orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind),
		reporter_v1.LayerExecution:     smith_v1.SchemeGroupVersion.WithKind(smith_v1.BundleResourceKind),
	}
	voyagerGVKs = map[schema.GroupVersionKind]string{
		comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind):  reporter_v1.LayerComposition,
		form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind): reporter_v1.LayerFormation,
		orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind):              reporter_v1.LayerOrchestration,
		smith_v1.SchemeGroupVersion.WithKind(smith_v1.BundleResourceKind):           reporter_v1.LayerExecution,
	}
)

type NamespaceReportHandler struct {
	*reporter_v1.NamespaceReport
	name     string
	service  voyager.ServiceName
	filter   RequestFilter
	location voyager.Location
}

type ProviderOpsResponse struct {
	Result *ProviderResponse `json:"result,omitempty"`
}

type ProviderResponse struct {
	Name       string                 `json:"name,omitempty"`
	Status     reporter_v1.Status     `json:"status,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	Version    string                 `json:"version,omitempty"`
}

func NewNamespaceReportHandler(name string, service voyager.ServiceName, objs []runtime.Object, filter RequestFilter, location voyager.Location) (*NamespaceReportHandler, error) {
	nrh := &NamespaceReportHandler{
		name:    name,
		service: service,
		filter:  filter,
		NamespaceReport: &reporter_v1.NamespaceReport{
			Composition: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
			Formation: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
			Orchestration: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
			Execution: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
			Objects: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
			Providers: reporter_v1.ReportLayer{
				Resources: []reporter_v1.Resource{},
			},
		},
		location: location,
	}

	// We collect the events first, so that they can be attached
	// to the objects they are related to.
	events := make(map[types.UID][]*core_v1.Event)
	eventKind := core_v1.SchemeGroupVersion.WithKind(k8s.EventKind).GroupKind()
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().GroupKind() == eventKind {
			event := obj.(*core_v1.Event)
			events[event.InvolvedObject.UID] = append(events[event.InvolvedObject.UID], event)
		}
	}

	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().GroupKind() == eventKind {
			continue
		}

		err := nrh.AddObject(obj, events)
		if err != nil {
			return nil, err
		}
	}

	return nrh, nil
}

func (n *NamespaceReportHandler) FormatReport() reporter_v1.Report {
	report := reporter_v1.Report{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Report",
			APIVersion: reporter_v1.ReportResourceAPIVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      string(n.service),
			Namespace: n.name,
		},
		Report: *n.NamespaceReport,
	}

	return report
}

func (n *NamespaceReportHandler) GenerateReport(ctx context.Context,
	providers map[string]ops.ProviderInterface, asapConfig pkiutil.ASAP) reporter_v1.Report {

	for _, obj := range n.NamespaceReport.Objects.Resources {
		if obj.ResourceType != "ServiceInstance" {
			continue
		}

		p := getProvider(providers, obj.Spec.(sc_v1b1.ServiceInstanceSpec))

		if p == nil {
			continue
		}

		n.NamespaceReport.Providers.Resources = append(n.NamespaceReport.Providers.Resources, n.getResourceFromProvider(ctx, p, obj, asapConfig))
	}

	return n.FormatReport()
}

func getProvider(providers map[string]ops.ProviderInterface, spec sc_v1b1.ServiceInstanceSpec) ops.ProviderInterface {
	var planID string
	if spec.ClusterServicePlanRef != nil {
		planID = spec.ClusterServicePlanRef.Name
	} else if spec.PlanReference.ClusterServicePlanExternalID != "" {
		planID = spec.PlanReference.ClusterServicePlanExternalID
	}

	// Check if any providers are registered for the plan
	for _, provider := range providers {
		if provider.OwnsPlan(planID) {
			return provider
		}
	}

	// Old name method
	p, ok := providers[getClassName(spec)]
	if ok {
		return p
	}

	return nil
}

func (n *NamespaceReportHandler) getResourceFromProvider(ctx context.Context, provider ops.ProviderInterface, obj reporter_v1.Resource, asapConfig pkiutil.ASAP) reporter_v1.Resource {
	logger := logz.RetrieveLoggerFromContext(ctx)

	req, err := http.NewRequest(http.MethodGet, "", nil)
	if err != nil {
		return handleProviderError(logger, obj, err, "setup request")
	}
	newReq := req.WithContext(ctx)

	externalID := obj.Spec.(sc_v1b1.ServiceInstanceSpec).ExternalID

	reporterAction := provider.ReportAction()
	if reporterAction == "" {
		return handleProviderError(logger, obj, errors.Errorf("provider spec isn't tagged with reporting api"), "parse spec")
	}

	userInfo, ok := request.UserFrom(ctx)
	if !ok {
		return handleProviderError(logger, obj, errors.New("missing user from context"), "parse user request")
	}

	resp, err := provider.Request(asapConfig, newReq, fmt.Sprintf(infoURI, externalID, reporterAction), userInfo.GetName())
	if err != nil {
		return handleProviderError(logger, obj, err, "get")
	}
	defer util.CloseSilently(resp.Body)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return handleProviderError(logger, obj, err, "parse body")
	}

	if resp.StatusCode != http.StatusOK {
		return handleProviderError(logger, obj, errors.Errorf("Failed request responded with status %d body %s", resp.StatusCode, string(body)), "request")
	}

	providerOpsResp := ProviderOpsResponse{}
	err = json.Unmarshal(body, &providerOpsResp)
	if err != nil {
		return handleProviderError(logger, obj, err, "unmarshal new reporter response json")
	} else if providerOpsResp.Result == nil {
		// Support migration from report model to ops api spec
		logger.Warn("Provider using old report spec", zap.String("provider", provider.Name()), zap.Error(err))
		providerOpsResp.Result = &ProviderResponse{}
		err = json.Unmarshal(body, providerOpsResp.Result)
		if err != nil {
			return handleProviderError(logger, obj, err, "unmarshal json")
		}
	}

	return reporter_v1.Resource{
		Name:         providerOpsResp.Result.Name,
		ResourceType: getClassName(obj.Spec.(sc_v1b1.ServiceInstanceSpec)),
		References: []reporter_v1.Reference{
			{
				Name:  obj.Name,
				Layer: reporter_v1.LayerObject,
			},
		},
		Properties: providerOpsResp.Result.Properties,
		Spec:       providerOpsResp.Result.Spec,
		Status:     providerOpsResp.Result.Status,
		Version:    providerOpsResp.Result.Version,
	}
}

func (n *NamespaceReportHandler) GenerateSummary(expand string) reporter_v1.Report {
	query := strings.Split(expand, ",")
	var properties, spec bool
	for _, val := range query {
		if strings.TrimSpace(val) == "properties" {
			properties = true
		} else if strings.TrimSpace(val) == "spec" {
			spec = true
		}
	}
	newHandler := NamespaceReportHandler{
		name:    n.name,
		service: n.service,
		NamespaceReport: &reporter_v1.NamespaceReport{
			Composition: reporter_v1.ReportLayer{
				Resources: summariseResources(n.Composition.Resources, properties, spec),
				Status:    n.Composition.Status,
			},
			Formation: reporter_v1.ReportLayer{
				Resources: summariseResources(n.Formation.Resources, properties, spec),
				Status:    n.Formation.Status,
			},
			Orchestration: reporter_v1.ReportLayer{
				Resources: summariseResources(n.Orchestration.Resources, properties, spec),
				Status:    n.Orchestration.Status,
			},
			Execution: reporter_v1.ReportLayer{
				Resources: summariseResources(n.Execution.Resources, properties, spec),
				Status:    n.Execution.Status,
			},
			Objects: reporter_v1.ReportLayer{
				Resources: summariseResources(n.Objects.Resources, properties, spec),
				Status:    n.Objects.Status,
			},
		},
	}
	return newHandler.FormatReport()
}

func summariseResources(resources []reporter_v1.Resource, properties, spec bool) []reporter_v1.Resource {
	resp := make([]reporter_v1.Resource, len(resources))
	copy(resp, resources)

	if properties && spec {
		return resp
	}

	for i := range resp {
		if !properties {
			resp[i].Properties = nil
		}

		if !spec {
			resp[i].Spec = nil
		}
	}
	return resp
}

func (n *NamespaceReportHandler) AddObject(obj runtime.Object, events map[types.UID][]*core_v1.Event) error {
	switch obj.GetObjectKind().GroupVersionKind().GroupKind() {
	case comp_v1.Kind(comp_v1.ServiceDescriptorResourceKind):
		return n.HandleComposition(obj.(*comp_v1.ServiceDescriptor))
	case form_v1.Kind(form_v1.LocationDescriptorResourceKind):
		return n.HandleFormation(obj.(*form_v1.LocationDescriptor))
	case orch_v1.Kind(orch_v1.StateResourceKind):
		return n.HandleOrchestration(obj.(*orch_v1.State))
	case smith_v1.Kind(smith_v1.BundleResourceKind):
		return n.HandleExecution(obj.(*smith_v1.Bundle), events)
	default:
		return n.handleObject(obj, events)
	}
}

func (n *NamespaceReportHandler) parseLocation(obj *comp_v1.ServiceDescriptor) *comp_v1.ServiceDescriptorSpec {
	var locationList []comp_v1.ServiceDescriptorLocation
	var resourceGroupList []comp_v1.ServiceDescriptorResourceGroup
	locationNameSet := sets.NewString()
	specDeepCopy := obj.Spec.DeepCopy()

	for _, loc := range specDeepCopy.Locations {
		if loc.EnvType == n.location.EnvType && loc.Account == n.location.Account && loc.Region == n.location.Region {
			locationNameSet.Insert(string(loc.Name))
			locationList = append(locationList, loc)
		}
	}
	if len(locationList) != 0 {
		specDeepCopy.Locations = locationList
	}
	if locationNameSet.Len() != 0 {
		for i, resourceGroup := range specDeepCopy.ResourceGroups {
			var serviceDescriptorLocationNameList []comp_v1.ServiceDescriptorLocationName
			for _, loc := range specDeepCopy.ResourceGroups[i].Locations {
				if locationNameSet.Has(string(loc)) {
					serviceDescriptorLocationNameList = append(serviceDescriptorLocationNameList, loc)
				}
			}
			if len(serviceDescriptorLocationNameList) != 0 {
				resourceGroup.Locations = serviceDescriptorLocationNameList
				resourceGroupList = append(resourceGroupList, resourceGroup)
			}
		}
		specDeepCopy.ResourceGroups = resourceGroupList
	}

	// filtering the config section based on location
	var configList []comp_v1.ServiceDescriptorConfigSet
	for _, conf := range specDeepCopy.Config {
		scopeList := strings.Split(string(conf.Scope), ".")
		if !n.isScopeListValid(scopeList) {
			continue
		}
		conf.Scope = comp_v1.Scope(strings.Join(scopeList, "."))
		configList = append(configList, conf)
	}
	specDeepCopy.Config = configList
	return specDeepCopy
}

func (n *NamespaceReportHandler) isScopeListValid(scopeList []string) bool {
	switch len(scopeList) - 1 {
	case accountItem:
		if voyager.Account(scopeList[accountItem]) != n.location.Account {
			return false
		}
		fallthrough
	case regionItem:
		if voyager.Region(scopeList[regionItem]) != n.location.Region {
			return false
		}
		fallthrough
	case envTypeItem:
		return scopeList[envTypeItem] == global || voyager.EnvType(scopeList[envTypeItem]) == n.location.EnvType
	default:
		return false
	}
}

func (n *NamespaceReportHandler) HandleComposition(obj *comp_v1.ServiceDescriptor) error {
	spec := n.parseLocation(obj)
	status := mapConditions(obj.Status.Conditions)
	n.Composition = reporter_v1.ReportLayer{
		Resources: []reporter_v1.Resource{
			{
				Name:         obj.Name,
				ResourceType: comp_v1.ServiceDescriptorResourceKind,
				Spec:         spec,
				Version:      hashObj(obj.Spec),
				Status:       status,
			},
		},
		Status: status,
	}
	return nil
}

func (n *NamespaceReportHandler) HandleFormation(obj *form_v1.LocationDescriptor) error {
	resources := make([]reporter_v1.Resource, 0, len(obj.Spec.Resources))
	for _, res := range obj.Spec.Resources {
		newRes := reporter_v1.Resource{
			Name:         string(res.Name),
			ResourceType: string(res.Type),
			References:   mapFormationReferences(res.DependsOn),
			Spec:         res.Spec.DeepCopy(),
			Status:       mapFormationResourceStatuses(string(res.Name), obj.Status.ResourceStatuses),
		}
		resources = append(resources, newRes)
	}
	n.Formation.Resources = resources
	n.Formation.Status = mapConditions(obj.Status.Conditions)
	return nil
}

func mapFormationReferences(refs []form_v1.LocationDescriptorDependency) []reporter_v1.Reference {
	result := make([]reporter_v1.Reference, 0, len(refs))
	for _, dep := range refs {
		result = append(result, reporter_v1.Reference{
			Name:       string(dep.Name),
			Layer:      reporter_v1.LayerFormation,
			Attributes: dep.Attributes,
		})
	}
	return result
}

func mapFormationResourceStatuses(name string, statuses []form_v1.ResourceStatus) reporter_v1.Status {
	for _, status := range statuses {
		if string(status.Name) == name {
			return mapConditions(status.Conditions)
		}
	}

	return mergeKubeStatuses([]reporter_v1.Status{})
}

func mapConditions(statuses []cond_v1.Condition) reporter_v1.Status {
	var active []reporter_v1.Status
	for _, condition := range statuses {
		if condition.Status == cond_v1.ConditionTrue {
			active = append(active, reporter_v1.Status{
				Status:    string(condition.Type),
				Timestamp: copyTime(condition.LastTransitionTime),
				Reason:    condition.Message,
			})
		}
	}

	return mergeKubeStatuses(active)
}

func (n *NamespaceReportHandler) HandleExecution(obj *smith_v1.Bundle, events map[types.UID][]*core_v1.Event) error {
	var resources []reporter_v1.Resource
	for _, res := range obj.Spec.Resources {
		newRes := reporter_v1.Resource{
			Name:       string(res.Name),
			References: mapExecutionReferences(res.References),
		}

		// The events for the bundle have an annotation that contains the resourcename
		for _, v1event := range events[obj.GetUID()] {
			resourceName, ok := v1event.Annotations[smith.EventAnnotationResourceName]
			if !ok || string(res.Name) != resourceName {
				continue
			}
			newRes.Events = append(newRes.Events, *reporter_v1.ConvertV1EventToEvent(v1event))
		}

		if res.Spec.Object != nil {
			newRes.Spec = res.Spec.Object.DeepCopyObject()
			newRes.Status = getResourceStatus(string(res.Name), obj.Status.ResourceStatuses)
		} else if res.Spec.Plugin != nil {
			newRes.Spec = res.Spec.Plugin.DeepCopy()
			newRes.Status = getPluginStatus(string(res.Name), obj.Status.PluginStatuses)
		}

		resources = append(resources, newRes)
	}

	n.Execution.Resources = resources
	n.Execution.Status = mapConditions(obj.Status.Conditions)
	return nil
}

func mapExecutionReferences(refs []smith_v1.Reference) []reporter_v1.Reference {
	result := make([]reporter_v1.Reference, 0, len(refs))
	for _, ref := range refs {
		reportRef := reporter_v1.Reference{
			Name:  string(ref.Resource),
			Layer: reporter_v1.LayerExecution,
		}
		if ref.Name != "" || ref.Path != "" || ref.Modifier != "" {
			reportRef.Attributes = map[string]interface{}{}
			if ref.Name != "" {
				reportRef.Attributes["name"] = ref.Name
			}
			if ref.Path != "" {
				reportRef.Attributes["path"] = ref.Path
			}
			if ref.Modifier != "" {
				reportRef.Attributes["modifier"] = ref.Modifier
			}
		}
		result = append(result, reportRef)
	}
	return result
}

func getResourceStatus(name string, statuses []smith_v1.ResourceStatus) reporter_v1.Status {
	var active []reporter_v1.Status
	for _, status := range statuses {
		if string(status.Name) == name {
			for _, condition := range status.Conditions {
				if condition.Status == cond_v1.ConditionTrue {
					active = append(active, reporter_v1.Status{
						Status:    string(condition.Type),
						Timestamp: copyTime(condition.LastTransitionTime),
						Reason:    condition.Message,
					})
				}
			}
		}
	}

	return mergeKubeStatuses(active)
}

func getPluginStatus(name string, statuses []smith_v1.PluginStatus) reporter_v1.Status {
	var active []reporter_v1.Status
	for _, status := range statuses {
		if string(status.Name) == name {
			return reporter_v1.Status{
				Status: string(status.Status),
			}
		}
	}

	return mergeKubeStatuses(active)
}

func mergeKubeStatuses(statuses []reporter_v1.Status) reporter_v1.Status {
	switch len(statuses) {
	case 0:
		return reporter_v1.Status{
			Status:    "Unknown",
			Timestamp: copyTime(meta_v1.Now()),
			Reason:    "No active condition found",
		}
	case 1:
		return statuses[0]
	default:
		// Error + InProgress is valid -> Retrying
		result := reporter_v1.Status{
			Status:    "Retrying",
			Timestamp: statuses[0].Timestamp,
		}
		for _, v := range statuses {
			if result.Timestamp.Before(v.Timestamp) {
				result.Timestamp = v.Timestamp
			}

			if v.Status == string(smith_v1.ResourceError) {
				result.Reason = fmt.Sprintf("Retrying after failure: %s", v.Reason)
			}
		}

		return result
	}
}

func (n *NamespaceReportHandler) HandleOrchestration(state *orch_v1.State) error {
	var resources []reporter_v1.Resource

	for _, res := range state.Spec.Resources {
		newRes := reporter_v1.Resource{
			Name:         string(res.Name),
			ResourceType: string(res.Type),
			References:   mapReferences(res.DependsOn),
		}
		if res.Spec != nil {
			newRes.Spec = res.Spec.Object
		}
		resources = append(resources, newRes)
	}
	n.Orchestration.Resources = resources
	n.Orchestration.Status = mapConditions(state.Status.Conditions)
	return nil
}

func mapReferences(dependsOn []orch_v1.StateDependency) []reporter_v1.Reference {
	refs := make([]reporter_v1.Reference, len(dependsOn))
	for i, dep := range dependsOn {
		refs[i] = reporter_v1.Reference{
			Name:       string(dep.Name),
			Layer:      reporter_v1.LayerOrchestration,
			Attributes: runtime.DeepCopyJSON(dep.Attributes),
		}
	}
	return refs
}

func (n *NamespaceReportHandler) handleObject(obj runtime.Object, events map[types.UID][]*core_v1.Event) error {
	metaObj := obj.(meta_v1.Object)
	res := reporter_v1.Resource{
		Name:         metaObj.GetName(),
		ResourceType: obj.GetObjectKind().GroupVersionKind().Kind,
		References:   mapOwnerReferences(metaObj.GetOwnerReferences()),
		Status: reporter_v1.Status{
			Status:    "Ready",
			Timestamp: copyTime(metaObj.GetCreationTimestamp()),
		},
	}

	for _, v1event := range events[metaObj.GetUID()] {
		res.Events = append(res.Events, *reporter_v1.ConvertV1EventToEvent(v1event))
	}

	switch obj.GetObjectKind().GroupVersionKind().GroupKind() {
	case sc_v1b1.Kind("ServiceInstance"):
		spec := obj.(*sc_v1b1.ServiceInstance).Spec
		res.Spec = spec
		res.Status = parseSCInstanceCondition(obj.(*sc_v1b1.ServiceInstance).Status)
		res.UID = spec.ExternalID
		res.Provider = &reporter_v1.ResourceProvider{}

		if spec.ClusterServiceClassRef != nil {
			res.Provider.ClassID = spec.ClusterServiceClassRef.Name
		} else if spec.ServiceClassRef != nil {
			res.Provider.ClassID = spec.ServiceClassRef.Name
			res.Provider.Namespaced = true
		}

		if spec.ClusterServicePlanRef != nil {
			res.Provider.PlanID = spec.ClusterServicePlanRef.Name
		} else if spec.ServicePlanRef != nil {
			res.Provider.PlanID = spec.ServicePlanRef.Name
			res.Provider.Namespaced = true
		}
	case sc_v1b1.Kind("ServiceBinding"):
		res.Spec = obj.(*sc_v1b1.ServiceBinding).Spec
		res.Status = parseSCBindingCondition(obj.(*sc_v1b1.ServiceBinding).Status)
		res.UID = obj.(*sc_v1b1.ServiceBinding).Spec.ExternalID
	case core_v1.SchemeGroupVersion.WithKind(k8s.ConfigMapKind).GroupKind():
		conf := obj.(*core_v1.ConfigMap)
		res.Spec = conf.Data
		res.UID = string(conf.ObjectMeta.UID)
		res.Status = reporter_v1.Status{
			Status:    "Ready",
			Timestamp: copyTime(metaObj.GetCreationTimestamp()),
		}
	case ext_v1beta1.SchemeGroupVersion.WithKind(k8s.IngressKind).GroupKind():
		ingress := obj.(*ext_v1beta1.Ingress)
		res.Spec = ingress.Spec
		res.UID = string(ingress.ObjectMeta.UID)
	case k8s.DeploymentGVK.GroupKind():
		deployment := obj.(*apps_v1.Deployment)
		res.Spec = deployment.Spec
		res.Status = convertDeploymentStatus(deployment.Status)
		res.UID = string(deployment.ObjectMeta.UID)
	case k8s.ReplicaSetGVK.GroupKind():
		replicaSet := obj.(*apps_v1.ReplicaSet)
		res.Spec = replicaSet.Spec
		res.Status = convertReplicaSetStatus(replicaSet)
		res.UID = string(replicaSet.ObjectMeta.UID)
	case k8s.PodGVK.GroupKind():
		pod := obj.(*core_v1.Pod)
		res.Spec = pod.Spec
		res.Status.Status = string(pod.Status.Phase)
		res.Status.Reason = pod.Status.Message
		res.UID = string(pod.ObjectMeta.UID)
	case k8s.HorizontalPodAutoscalerGVK.GroupKind():
		hpa := obj.(*autoscaling_v2b1.HorizontalPodAutoscaler)
		res.Spec = hpa.Spec
		res.Status = convertHPAStatus(hpa.Status)
		res.UID = string(hpa.ObjectMeta.UID)
	}

	n.Objects.Resources = append(n.Objects.Resources, res)
	return nil
}

func parseSCInstanceCondition(status sc_v1b1.ServiceInstanceStatus) reporter_v1.Status {
	if len(status.Conditions) == 1 {
		s := "InProgress"
		if status.Conditions[0].Status == sc_v1b1.ConditionTrue {
			s = string(status.Conditions[0].Type)
		}

		return reporter_v1.Status{
			Status:    s,
			Timestamp: copyTime(status.Conditions[0].LastTransitionTime),
			Reason:    status.Conditions[0].Message,
		}
	}

	for _, cond := range status.Conditions {
		if cond.Status == sc_v1b1.ConditionTrue {
			return reporter_v1.Status{
				Status:    string(cond.Type),
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		} else if status.AsyncOpInProgress && cond.Type == "Ready" {
			return reporter_v1.Status{
				Status:    "InProgress",
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		}
	}

	return reporter_v1.Status{
		Status:    "Unknown",
		Timestamp: copyTime(meta_v1.Now()),
		Reason:    "No active condition found",
	}
}

func parseSCBindingCondition(status sc_v1b1.ServiceBindingStatus) reporter_v1.Status {
	for _, cond := range status.Conditions {
		if cond.Status == sc_v1b1.ConditionTrue || (status.AsyncOpInProgress && cond.Type == "InProgress") {
			return reporter_v1.Status{
				Status:    string(cond.Type),
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		}
	}
	return reporter_v1.Status{
		Status:    "Unknown",
		Timestamp: copyTime(meta_v1.Now()),
		Reason:    "No active condition found",
	}
}

func convertDeploymentStatus(status apps_v1.DeploymentStatus) reporter_v1.Status {
	for _, cond := range status.Conditions {
		if cond.Status == core_v1.ConditionTrue {
			return reporter_v1.Status{
				Status:    string(cond.Type),
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		}
	}

	return reporter_v1.Status{
		Status:    "Unknown",
		Timestamp: copyTime(meta_v1.Now()),
		Reason:    "No active condition found",
	}
}

func convertReplicaSetStatus(rs *apps_v1.ReplicaSet) reporter_v1.Status {
	for _, cond := range rs.Status.Conditions {
		if cond.Status == core_v1.ConditionTrue {
			return reporter_v1.Status{
				Status:    string(cond.Type),
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		}
	}

	// assume it's ready if we have available replicas greater than specified replicas
	if rs.Status.AvailableReplicas >= *rs.Spec.Replicas {
		return reporter_v1.Status{
			Status:    "Ready",
			Timestamp: copyTime(meta_v1.Now()),
			Reason:    "",
		}
	}

	// for now, don't try to determine the full state
	return reporter_v1.Status{
		Status:    "Unknown",
		Timestamp: copyTime(meta_v1.Now()),
		Reason:    "No active condition found",
	}
}

func convertHPAStatus(status autoscaling_v2b1.HorizontalPodAutoscalerStatus) reporter_v1.Status {
	for _, cond := range status.Conditions {
		if cond.Status == core_v1.ConditionTrue {
			return reporter_v1.Status{
				Status:    string(cond.Type),
				Timestamp: copyTime(cond.LastTransitionTime),
				Reason:    cond.Message,
			}
		}
	}

	return reporter_v1.Status{
		Status:    "Unknown",
		Timestamp: copyTime(meta_v1.Now()),
		Reason:    "No active condition found",
	}
}

func mapOwnerReferences(refs []meta_v1.OwnerReference) []reporter_v1.Reference {
	result := make([]reporter_v1.Reference, len(refs))
	for i, ref := range refs {
		result[i] = reporter_v1.Reference{
			Name: ref.Name,
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil && gv.WithKind(ref.Kind).GroupKind() == smith_v1.Kind(smith_v1.BundleResourceKind) {
			result[i].Layer = reporter_v1.LayerExecution
		} else {
			result[i].Layer = reporter_v1.LayerObject
			result[i].ResourceType = ref.Kind
		}
	}
	return result
}

func handleProviderError(logger *zap.Logger, obj reporter_v1.Resource, err error, action string) reporter_v1.Resource {
	msg := fmt.Sprintf("Failed attempting to get provider information during %s", action)
	logger.Error(msg, zap.Error(err))
	return reporter_v1.Resource{
		Name:         obj.Name,
		ResourceType: getClassName(obj.Spec.(sc_v1b1.ServiceInstanceSpec)),
		Status: reporter_v1.Status{
			Status:    "ReporterError",
			Reason:    fmt.Sprintf("%s: %v", msg, err),
			Timestamp: copyTime(meta_v1.Now()),
		},
	}
}

func getClassName(spec sc_v1b1.ServiceInstanceSpec) string {
	if spec.PlanReference.ClusterServiceClassExternalName != "" {
		return spec.PlanReference.ClusterServiceClassExternalName
	}

	return spec.PlanReference.ServiceClassExternalName
}

func copyTime(in meta_v1.Time) *meta_v1.Time {
	var time meta_v1.Time
	in.DeepCopyInto(&time)
	return &time
}

func hashObj(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%x", sha1.Sum(b)) //nolint: gosec
}
