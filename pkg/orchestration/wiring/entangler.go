package wiring

import (
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util"
	"github.com/atlassian/smith/pkg/util/graph"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	serviceBindingGVK  = sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding")
	serviceInstanceGVK = sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")
)

// EntangleContext contains information that is required by autowiring.
// Everything in this context can only be obtained by reading Kubernetes objects.
type EntangleContext struct {
	// ServiceName
	ServiceName voyager.ServiceName

	// Label
	Label voyager.Label

	ServiceProperties orch_meta.ServiceProperties
}

// StatusContext contains information that is required by status autowiring.
type StatusContext struct {
	// BundleResources is the list of resources and their statuses in Bundle.
	// Only resources for a particular StateResource are in the list.
	BundleResources []BundleResource
	// PluginStatuses is a list of statuses for Smith plugins used in a Bundle.
	PluginStatuses []smith_v1.PluginStatus `json:"pluginStatuses,omitempty"`
}

type BundleResource struct {
	// Object is the actual object that has been created as the result of processing an Orchestration StateResource.
	Resource smith_v1.Resource `json:"object"`
	// Status is the status of that object as reported by Smith.
	Status smith_v1.ResourceStatusData `json:"status"`
}

type TagNames struct {
	ServiceNameTag     voyager.Tag
	BusinessUnitTag    voyager.Tag
	ResourceOwnerTag   voyager.Tag
	PlatformTag        voyager.Tag
	EnvironmentTypeTag voyager.Tag
}

type TagGenerator func(
	clusterLocation voyager.ClusterLocation,
	clusterConfig wiringplugin.ClusterConfig,
	location voyager.Location,
	serviceName voyager.ServiceName,
	properties orch_meta.ServiceProperties,
) map[voyager.Tag]string

type Entangler struct {
	Plugins         map[voyager.ResourceType]wiringplugin.WiringPlugin
	ClusterLocation voyager.ClusterLocation
	ClusterConfig   wiringplugin.ClusterConfig
	Tags            TagGenerator
}

type wiredStateResource struct {
	Name      voyager.ResourceName
	Type      voyager.ResourceType
	Contract  wiringplugin.ResourceContract
	Resources []smith_v1.Resource
}

type EntangleResultType string
type StatusResultType string

const (
	EntangleResultSuccessType EntangleResultType = "success"
	EntangleResultFailureType EntangleResultType = "failure"
	StatusResultSuccessType   StatusResultType   = "success"
	StatusResultFailureType   StatusResultType   = "failure"
)

type EntangleResult interface {
	StatusType() EntangleResultType
}

type EntangleResultSuccess struct {
	Bundle *smith_v1.Bundle
}

func (e *EntangleResultSuccess) StatusType() EntangleResultType {
	return EntangleResultSuccessType
}

type EntangleResultFailure struct {
	Error            error
	IsRetriableError bool
	IsExternalError  bool
}

func (e *EntangleResultFailure) StatusType() EntangleResultType {
	return EntangleResultFailureType
}

type StatusResult interface {
	StatusType() StatusResultType
}

type StatusResultSuccess struct {
	ResourceStatusData orch_v1.ResourceStatusData
}

func (e *StatusResultSuccess) StatusType() StatusResultType {
	return StatusResultSuccessType
}

type StatusResultFailure struct {
	Error           error
	IsExternalError bool
}

func (e *StatusResultFailure) StatusType() StatusResultType {
	return StatusResultFailureType
}

func (en *Entangler) Entangle(state *orch_v1.State, context *EntangleContext) EntangleResult {
	g, sorted, err := sortStateResources(state.Spec.Resources)
	if err != nil {
		// failures here are caused by a cycle or invalid dependency specified by the user
		return &EntangleResultFailure{
			Error:           err,
			IsExternalError: true,
		}
	}

	w := worker{
		plugins:           en.Plugins,
		allWiredResources: make(map[voyager.ResourceName]*wiredStateResource, len(state.Spec.Resources)),
	}

	location := voyager.Location{
		Region:  en.ClusterLocation.Region,
		Account: en.ClusterLocation.Account,
		EnvType: en.ClusterLocation.EnvType,
		Label:   context.Label,
	}

	tags := make(map[voyager.Tag]string)
	for tag, val := range context.ServiceProperties.UserTags {
		tags[tag] = val
	}

	stateContext := wiringplugin.StateContext{
		Location:          location,
		ClusterConfig:     en.ClusterConfig,
		ServiceName:       context.ServiceName,
		ServiceProperties: context.ServiceProperties,
		Tags:              en.Tags(en.ClusterLocation, en.ClusterConfig, location, context.ServiceName, context.ServiceProperties),
	}

	// Visit vertices in sorted order
	for _, v := range sorted {
		resource := g.Vertices[v].Data.(*orch_v1.StateResource)
		dependants := getDependants(resource.Name, g.Vertices[v].IncomingEdges, state.Spec.Resources)
		external, retriable, entErr := w.entangle(resource, &state.ObjectMeta, &stateContext, dependants)
		if entErr != nil {
			return &EntangleResultFailure{
				Error:            errors.Wrapf(entErr, "failed to wire up resource %q of type %q", resource.Name, resource.Type),
				IsRetriableError: retriable,
				IsExternalError:  external,
			}
		}
	}
	processedResources, err := postProcessResources(w.allWiredResourcesList)
	if err != nil {
		return &EntangleResultFailure{
			Error: err,
		}
	}
	trueVar := true
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      state.Name,
			Namespace: state.Namespace,
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         state.APIVersion,
					Kind:               state.Kind,
					Name:               state.Name,
					UID:                state.UID,
					Controller:         &trueVar,
					BlockOwnerDeletion: &trueVar,
				},
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: processedResources,
		},
	}

	return &EntangleResultSuccess{
		Bundle: bundle,
	}
}

func (en *Entangler) Status(resource *orch_v1.StateResource, context *StatusContext) StatusResult {
	plugin, ok := en.Plugins[resource.Type]
	if !ok {
		// The plugin not existing is an external issue, the service descriptor contains
		// an invalid resource type.
		return &StatusResultFailure{
			Error:           errors.New("unknown resource type"),
			IsExternalError: true,
		}
	}
	// We don't want to expose types from plugins to the entangler consumer so that they are decoupled.
	bundleResources := make([]wiringplugin.BundleResource, 0, len(context.BundleResources))
	for _, res := range context.BundleResources {
		bundleResources = append(bundleResources, wiringplugin.BundleResource{
			Resource: res.Resource,
			Status:   res.Status,
		})
	}
	result := plugin.Status(resource, &wiringplugin.StatusContext{
		BundleResources: bundleResources,
		PluginStatuses:  context.PluginStatuses,
	})
	switch r := result.(type) {
	case *wiringplugin.StatusResultSuccess:
		return &StatusResultSuccess{
			ResourceStatusData: r.ResourceStatusData,
		}
	case *wiringplugin.StatusResultFailure:
		return &StatusResultFailure{
			Error:           errors.Wrap(r.Error, "error invoking autowiring plugin"),
			IsExternalError: r.IsExternalError,
		}
	default:
		return &StatusResultFailure{
			Error: errors.Errorf("unknown status result type %q", r.StatusType()),
		}
	}
}

// postProcessResources converts resources to Unstructured and cleans up some fields:
// - "status", "metadata.creationTimestamp".
func postProcessResources(resources []smith_v1.Resource) ([]smith_v1.Resource, error) {
	results := make([]smith_v1.Resource, 0, len(resources))
	for _, resource := range resources {
		if resource.Spec.Object != nil {
			resUnstr, err := util.RuntimeToUnstructured(resource.Spec.Object)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to convert object %s in resource %q to Unstructured",
					resource.Spec.Object.GetObjectKind().GroupVersionKind(), resource.Name)
			}
			delete(resUnstr.Object, "status")
			unstructured.RemoveNestedField(resUnstr.Object, "metadata", "creationTimestamp")

			resGVK := resUnstr.GetObjectKind().GroupVersionKind()
			switch resGVK {
			case serviceBindingGVK, serviceInstanceGVK:
				id, ok, err := unstructured.NestedString(resUnstr.Object, "spec", "externalID")
				if err == nil && ok && id == "" {
					// Remove if present and is an empty string
					unstructured.RemoveNestedField(resUnstr.Object, "spec", "externalID")
				}
				if resGVK == serviceInstanceGVK {
					updateRequests, ok, err := unstructured.NestedInt64(resUnstr.Object, "spec", "updateRequests")
					if err == nil && ok && updateRequests == 0 {
						// Remove if present and is zero
						unstructured.RemoveNestedField(resUnstr.Object, "spec", "updateRequests")
					}
				}
			}
			resource.Spec.Object = resUnstr
		}
		results = append(results, resource)
	}
	return results, nil
}

type worker struct {
	plugins           map[voyager.ResourceType]wiringplugin.WiringPlugin
	allWiredResources map[voyager.ResourceName]*wiredStateResource
	// To preserve deterministic order (map above has random iteration order)
	allWiredResourcesList []smith_v1.Resource
}

func (w *worker) entangle(resource *orch_v1.StateResource, stateMeta *meta_v1.ObjectMeta, context *wiringplugin.StateContext, dependants []wiringplugin.DependantResource) (bool /* external */, bool /*retriable*/, error) {
	if w.allWiredResources[resource.Name] != nil {
		return true, false, errors.New("resource with same name already exists")
	}
	plugin := w.plugins[resource.Type]
	if plugin == nil {
		return true, false, errors.New("no plugin for resources type is registered")
	}
	deps := make([]wiringplugin.WiredDependency, 0, len(resource.DependsOn))
	for _, dep := range resource.DependsOn {
		res := w.allWiredResources[dep.Name]
		if res == nil {
			// This can only happen if there is a bug! Dependency on a missing resource should have been detected by
			// the topological sort.
			return false, false, errors.Errorf("resource %q of type %q has a dependency that has not been wired yet: %q", resource.Name, resource.Type, dep)
		}
		deps = append(deps, wiringplugin.WiredDependency{
			Name:       res.Name,
			Contract:   res.Contract,
			Attributes: dep.Attributes,
		})
	}
	wiringContext := &wiringplugin.WiringContext{
		StateContext: *context,
		Dependencies: deps,
		Dependants:   dependants,
	}
	stateMeta.DeepCopyInto(&wiringContext.StateMeta)
	result := plugin.WireUp(resource, wiringContext)
	switch r := result.(type) {
	case *wiringplugin.WiringResultSuccess:
		// Sanity check plugin output
		// If the plugin itself is returning garbage we consider it an internal error
		err := w.validateWireUp(resource, r)
		if err != nil {
			return false, false, err
		}

		w.allWiredResources[resource.Name] = &wiredStateResource{
			Name:      resource.Name,
			Type:      resource.Type,
			Resources: r.Resources,
			Contract:  r.Contract,
		}
		w.allWiredResourcesList = append(w.allWiredResourcesList, r.Resources...)
		return false, false, nil

	case *wiringplugin.WiringResultFailure:
		return r.IsExternalError, r.IsRetriableError, errors.Wrap(r.Error, "error invoking plugin")
	default:
		return false, false, errors.Errorf("unexpected wiring result status type %q", r.StatusType())
	}
}

func (w *worker) validateWireUp(resource *orch_v1.StateResource, result *wiringplugin.WiringResultSuccess) error {
	if shapeNames := findDuplicateShapeNames(result.Contract.Shapes); len(shapeNames) != 0 {
		return errors.Errorf("internal error in wiring plugin - duplicate shapes received from plugin: %s",
			strings.Join(shapeNames, ", "))
	}

	return validateResources(resource, result.Resources)
}

func sortStateResourcesInternal(g *graph.Graph, stateResources []orch_v1.StateResource) ([]graph.V, error) {
	for _, res := range stateResources {
		res := res
		g.AddVertex(res.Name, &res)
	}

	for _, res := range stateResources {
		for _, d := range res.DependsOn {
			if err := g.AddEdge(res.Name, d.Name); err != nil {
				return nil, err
			}
		}
	}

	return g.TopologicalSort()
}

func sortStateResources(stateResources []orch_v1.StateResource) (*graph.Graph, []graph.V, error) {
	g := graph.NewGraph(len(stateResources))
	sorted, err := sortStateResourcesInternal(g, stateResources)
	if err != nil {
		return nil, nil, err
	}
	return g, sorted, nil
}

func getDependants(resourceName voyager.ResourceName, dependantVertices []graph.V, allResources []orch_v1.StateResource) []wiringplugin.DependantResource {
	dependantResources := make([]wiringplugin.DependantResource, 0, len(dependantVertices))
	for _, v := range dependantVertices {
		for _, resource := range allResources {
			if v == resource.Name {
				// Find attributes
				var attrs map[string]interface{}
				for _, dependency := range resource.DependsOn {
					if dependency.Name == resourceName {
						attrs = runtime.DeepCopyJSON(dependency.Attributes)
						break
					}
				}
				dependantResources = append(dependantResources, wiringplugin.DependantResource{
					Name:       resource.Name,
					Type:       resource.Type,
					Attributes: attrs,
					Resource:   resource,
				})
				break
			}
		}
	}
	return dependantResources
}

func findDuplicateShapeNames(shapes []wiringplugin.Shape) []string {
	set := make(map[wiringplugin.ShapeName]bool)
	duplicates := make([]string, 0, len(shapes))
	for _, shape := range shapes {
		if set[shape.Name()] {
			duplicates = append(duplicates, string(shape.Name()))
		} else {
			set[shape.Name()] = true
		}
	}
	return duplicates
}

func validateResources(stateResource *orch_v1.StateResource, resources []smith_v1.Resource) error {
	resourceNames := sets.NewString()
	stateResourceName := string(stateResource.Name)

	for _, resource := range resources {
		// check bundle resource name
		smithResourceName := string(resource.Name)
		if smithResourceName != stateResourceName && !strings.HasPrefix(smithResourceName, stateResourceName+"--") {
			return errors.Errorf("resource %q does not have valid resource name", smithResourceName)
		}
		if resourceNames.Has(smithResourceName) {
			return errors.Errorf("resource %q already declared by wiring function", smithResourceName)
		}
		resourceNames.Insert(smithResourceName)

		// check object resource name
		var metaName string
		switch {
		case resource.Spec.Object != nil:
			metaName = resource.Spec.Object.(meta_v1.Object).GetName()
		case resource.Spec.Plugin != nil:
			metaName = resource.Spec.Plugin.ObjectName
		default:
			return errors.Errorf("resource in smith bundle %q has an invalid spec missing plugin or object", resource.Name)
		}
		if metaName != stateResourceName && !strings.HasPrefix(metaName, stateResourceName+"--") {
			return errors.Errorf("object %q does not have valid object name", metaName)
		}
	}

	return nil
}
