package wiring

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util"
	"github.com/atlassian/smith/pkg/util/graph"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/ghodss/yaml"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	serviceBindingGVK  = sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding")
	serviceInstanceGVK = sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")
)

const (
	// This is the 'old' Micros name of the environment (e.g. ddev)
	// We hack this Atlassianism in here rather than putting in user
	// tags in the configmap because it's at the same level as legacyConfig
	// (i.e. want to make it obvious they should be removed at the same time).
	legacyEnvironmentTagName = "environment"

	wiringResourcesDirectlyIsDeprecatedMsg = "wiring dependencies as SmithResources is deprecated, use ResourceContracts instead"
)

// EntanglerContext contains information that is required by autowiring.
// Everything in this context can only be obtained by reading Kubernetes objects.
type EntanglerContext struct {
	// ServiceName
	ServiceName voyager.ServiceName

	// Label
	Label voyager.Label

	// Config is the configuration pulled from the ConfigMap
	Config map[string]string
}

type TagNames struct {
	ServiceNameTag     voyager.Tag
	BusinessUnitTag    voyager.Tag
	ResourceOwnerTag   voyager.Tag
	PlatformTag        voyager.Tag
	EnvironmentTypeTag voyager.Tag
}

type Entangler struct {
	Plugins             map[voyager.ResourceType]wiringplugin.WiringPlugin
	ClusterLocation     voyager.ClusterLocation
	ClusterConfig       wiringplugin.ClusterConfig
	TagNames            TagNames
	GetLegacyConfigFunc func(voyager.Location) *legacy.Config
}

type wiredStateResource struct {
	Name         voyager.ResourceName
	Type         voyager.ResourceType
	WiringResult wiringplugin.WiringResult
}

func parseConfigMap(data map[string]string) (*orch_meta.ServiceProperties, error) {
	serviceProperties := orch_meta.ServiceProperties{}
	configMapConfigData, ok := data[orch_meta.ConfigMapConfigKey]
	if !ok {
		return nil, errors.Errorf("ConfigMap does not contain expected field %q", orch_meta.ConfigMapConfigKey)
	}
	err := yaml.Unmarshal([]byte(configMapConfigData), &serviceProperties)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &serviceProperties, nil
}

func (en *Entangler) Entangle(state *orch_v1.State, context *EntanglerContext) (*smith_v1.Bundle, bool /*retriable*/, error) {
	g, sorted, err := sortStateResources(state.Spec.Resources)
	if err != nil {
		return nil, false, err
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

	// Atlassian Specific Things
	legacyConfigFunc := en.GetLegacyConfigFunc
	if legacyConfigFunc == nil {
		return nil, false, errors.New("missing legacy config")
	}
	legacyConfig := legacyConfigFunc(location)
	if legacyConfig == nil {
		return nil, false, errors.Errorf("no legacy config for %s", location)
	}

	serviceProperties, err := parseConfigMap(context.Config)
	if err != nil {
		return nil, false, err
	}

	tags := make(map[voyager.Tag]string)
	for tag, val := range serviceProperties.UserTags {
		tags[tag] = val
	}

	tags[en.TagNames.ServiceNameTag] = string(context.ServiceName)
	tags[en.TagNames.BusinessUnitTag] = serviceProperties.BusinessUnit
	tags[en.TagNames.ResourceOwnerTag] = serviceProperties.ResourceOwner
	tags[en.TagNames.EnvironmentTypeTag] = string(location.EnvType)
	tags[en.TagNames.PlatformTag] = "voyager"
	tags[legacyEnvironmentTagName] = legacyConfig.MicrosEnv

	stateContext := wiringplugin.StateContext{
		Location:          location,
		ClusterConfig:     en.ClusterConfig,
		LegacyConfig:      *legacyConfig,
		ServiceName:       context.ServiceName,
		ServiceProperties: *serviceProperties,
		Tags:              tags,
	}

	// Visit vertices in sorted order
	for _, v := range sorted {
		resource := g.Vertices[v].Data.(*orch_v1.StateResource)
		dependants := getDependants(resource.Name, g.Vertices[v].IncomingEdges, state.Spec.Resources)
		retriable, entErr := w.entangle(resource, &state.ObjectMeta, &stateContext, dependants)
		if entErr != nil {
			return nil, retriable, errors.Wrapf(entErr, "failed to wire up resource %q of type %q", resource.Name, resource.Type)
		}
	}
	processedResources, err := postProcessResources(w.allWiredResourcesList)
	if err != nil {
		return nil, false, err
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

	return bundle, false, nil
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

func (w *worker) entangle(resource *orch_v1.StateResource, stateMeta *meta_v1.ObjectMeta, context *wiringplugin.StateContext, dependants []wiringplugin.DependantResource) (bool /*retriable*/, error) {
	if w.allWiredResources[resource.Name] != nil {
		return false, errors.New("resource with same name already exists")
	}
	plugin := w.plugins[resource.Type]
	if plugin == nil {
		return false, errors.New("no plugin for resources type is registered")
	}
	deps := make([]wiringplugin.WiredDependency, 0, len(resource.DependsOn))
	for _, dep := range resource.DependsOn {
		res := w.allWiredResources[dep.Name]
		if res == nil {
			// This can only happen if there is a bug! Dependency on a missing resource should have been detected by
			// the topological sort.
			return false, errors.Errorf("resource %q of type %q has a dependency that has not been wired yet: %q", resource.Name, resource.Type, dep)
		}
		deps = append(deps, wiringplugin.WiredDependency{
			Name:       res.Name,
			Type:       res.Type,
			Contract:   res.WiringResult.Contract,
			Attributes: dep.Attributes,
		})
	}
	wiringContext := &wiringplugin.WiringContext{
		StateContext: *context,
		Dependencies: deps,
		Dependants:   dependants,
	}
	stateMeta.DeepCopyInto(&wiringContext.StateMeta)
	result, retriable, err := plugin.WireUp(resource, wiringContext)
	if err != nil {
		return retriable, errors.Wrap(err, "error invoking plugin")
	}
	if w.allWiredResources[resource.Name] != nil {
		return false, errors.New("internal error in wiring plugin - duplicate resource name received from plugin")
	}
	w.allWiredResources[resource.Name] = &wiredStateResource{
		Name:         resource.Name,
		Type:         resource.Type,
		WiringResult: *result,
	}
	for _, wiredResource := range result.Resources {
		w.allWiredResourcesList = append(w.allWiredResourcesList, wiredResource)
	}
	return false, nil
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
