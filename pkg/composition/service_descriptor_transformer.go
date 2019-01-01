package composition

// This file contains functions that takes a ServiceDescriptor and produces the information required
// to product a FormationObject. IT DOES NOT create the formation object.
// A separate step should take this definition and produce the real FormationObject

import (
	"fmt"
	"strings"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/formation"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/sets"
	"github.com/atlassian/voyager/pkg/util/templating"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	sdTemplatingPrefix     = "self:"
	errMsgLocationRequired = "at least 1 location must be defined for resourceGroup %q"
	errMsgUnknownLocation  = "location %q not known for resourceGroup %q"
)

type SdTransformer interface {
	CreateFormationObjectDef(sd *comp_v1.ServiceDescriptor) ([]FormationObjectInfo, error)
}

// Information required to generate the formation object (LocationDescriptor)
type FormationObjectInfo struct {
	// Name is the name of the location descriptor (NOT the name of the location)
	Name string
	// Namespace is where the location descriptor should be placed
	Namespace string
	// ServiceName is the service creating the location descriptor (==service descriptor Name)
	ServiceName voyager.ServiceName
	Location    voyager.Location
	Resources   []comp_v1.ServiceDescriptorResource
}

type ServiceDescriptorTransformer struct {
	ClusterLocation voyager.ClusterLocation
}

// Constructor for the ServiceDescriptorTransformer
func NewServiceDescriptorTransformer(location voyager.ClusterLocation) *ServiceDescriptorTransformer {
	return &ServiceDescriptorTransformer{ClusterLocation: location}
}

// The current context for the resourceGroup being evaluated/expanded
type ResourceGroupEvalContext struct {
	Location      comp_v1.ServiceDescriptorLocation
	ResourceGroup comp_v1.ServiceDescriptorResourceGroupName
}

// Returns a map of location name to Location definition
func (t *ServiceDescriptorTransformer) getDefinedLocations(sd comp_v1.ServiceDescriptorSpec) (map[comp_v1.ServiceDescriptorLocationName]comp_v1.ServiceDescriptorLocation, error) {
	definedLocations := make(map[comp_v1.ServiceDescriptorLocationName]comp_v1.ServiceDescriptorLocation, len(sd.Locations))

	for _, locationItem := range sd.Locations {
		definedLocations[locationItem.Name] = locationItem
	}

	return definedLocations, nil
}

// Get the names of all the locations mentioned in the resourceGroup, and return the matching location definitions
func getLocationsForResourceGroup(resourceGroup comp_v1.ServiceDescriptorResourceGroup,
	definedLocations map[comp_v1.ServiceDescriptorLocationName]comp_v1.ServiceDescriptorLocation) ([]comp_v1.ServiceDescriptorLocation, error) {
	var resourceGroupLocations []comp_v1.ServiceDescriptorLocation

	for _, locationRef := range resourceGroup.Locations {
		if foundLocation, found := definedLocations[locationRef]; found {
			resourceGroupLocations = append(resourceGroupLocations, foundLocation)
		} else {
			return nil, errors.Errorf(errMsgUnknownLocation, locationRef, resourceGroup.Name)
		}
	}

	return resourceGroupLocations, nil
}

// Given a service descriptor, returns a list of formation objects definitions per location.
// If there are no resources at the location, it will still return a formation object definition,
// but this definition will only contain location information, and not resource information
func (t *ServiceDescriptorTransformer) CreateFormationObjectDef(sd *comp_v1.ServiceDescriptor) ([]FormationObjectInfo, error) {
	errorList := util.NewErrorList()

	definedLocations, err := t.getDefinedLocations(sd.Spec)
	if err != nil {
		errorList.Add(err)
		return nil, errorList
	}
	sdVars := createVarModelFromSdSpec(&sd.Spec)

	formationObjects := make(map[string]*FormationObjectInfo)
	for _, resourceGroup := range sd.Spec.ResourceGroups {
		resourceGroupLocations, err := getLocationsForResourceGroup(resourceGroup, definedLocations)
		if err != nil {
			errorList.Add(err)
			continue
		}

		// Check whether the resourceGroup has a location defined.
		// You cannot have a resourceGroup without a location
		if len(resourceGroupLocations) == 0 {
			errorList.Add(errors.Errorf(errMsgLocationRequired, resourceGroup.Name))
			continue
		}

		for _, location := range resourceGroupLocations {
			// Only process resourceGroups targeted at the current cluster
			if location.VoyagerLocation().ClusterLocation() != t.ClusterLocation {
				continue
			}

			serviceName := sd.Name
			nsName := generateNamespaceName(serviceName, location.Label)
			ldName := generateLdName(serviceName, location.Label)

			key := fmt.Sprintf("%s/%s", nsName, ldName)
			fo, exists := formationObjects[key]
			if !exists {
				fo = &FormationObjectInfo{
					Name:        nsName,
					Namespace:   nsName,
					ServiceName: voyager.ServiceName(serviceName),
					Location:    location.VoyagerLocation(),
				}
				formationObjects[key] = fo
			}

			// For backwards compatibility we "normalize" any non prefixed variables to prefixed ones before processing.
			normalizeFunc := func(varName string) (interface{}, error) {
				if !strings.Contains(varName, ":") && !strings.HasPrefix(varName, sdTemplatingPrefix) && !strings.HasPrefix(varName, formation.ReleaseTemplatingPrefix) {
					return fmt.Sprintf("${%s%s}", sdTemplatingPrefix, varName), nil
				}
				return fmt.Sprintf("${%s}", varName), nil
			}
			normalizer := templating.SpecExpander{
				VarResolver:      normalizeFunc,
				RequiredPrefix:   "",
				ReservedPrefixes: []string{formation.ReleaseTemplatingPrefix, sdTemplatingPrefix},
			}

			resolveFunc := func(varName string) (interface{}, error) {
				evalContext := ResourceGroupEvalContext{
					ResourceGroup: resourceGroup.Name,
					Location:      location,
				}
				locationToSearch := []string{
					string(evalContext.Location.EnvType),
					string(evalContext.Location.Region),
				}

				locationToSearch = append(locationToSearch, string(evalContext.Location.Label))
				locationToSearch = append(locationToSearch, string(evalContext.Location.Account))

				varVal, err := sdVars.getVar(locationToSearch, varName)
				if err != nil {
					return nil, err
				}

				return varVal, err
			}
			specExpander := templating.SpecExpander{
				VarResolver:      resolveFunc,
				RequiredPrefix:   sdTemplatingPrefix,
				ReservedPrefixes: []string{formation.ReleaseTemplatingPrefix},
			}

			for _, resource := range resourceGroup.Resources {
				var expandedSpec *runtime.RawExtension

				if resource.Spec != nil {
					var errs *util.ErrorList
					normalizedSpec, errs := normalizer.Expand(resource.Spec)
					if errs != nil {
						errorList.Add(errs)
						continue
					}
					expandedSpec, errs = specExpander.Expand(normalizedSpec)
					if errs != nil {
						errorList.Add(errs)
						continue
					}
				}

				resource.Spec = expandedSpec
				fo.Resources = append(fo.Resources, resource)
			}
		}
	}

	// Check that resource naming makes sense for each FO.
	// I'd prefer not to have this here (likewise with the resourceNameExists check
	// above), but we don't yet have a clean way of feeding back likely lower layer
	// errors into the admission webhook, and at least we understand 'dependsOn' here.
	for _, fo := range formationObjects {
		resourceNames := sets.String{}
		for _, resource := range fo.Resources {
			if resourceNames.Has(string(resource.Name)) {
				errorList.Add(errors.Errorf("resource %q appears multiple times for the same location", resource.Name))
			}

			resourceNames.Insert(string(resource.Name))
		}

		for _, resource := range fo.Resources {
			for _, dependency := range resource.DependsOn {
				if !resourceNames.Has(string(dependency.Name)) {
					errorList.Add(errors.Errorf("dependency %q does not exist in this location", dependency.Name))
				}

				if dependency.Name == resource.Name {
					errorList.Add(errors.Errorf("resource %q depends on itself", resource.Name))
				}
			}
		}
	}

	if errorList.HasErrors() {
		return nil, errorList
	}

	foObjectList := make([]FormationObjectInfo, 0, len(formationObjects))
	for _, fo := range formationObjects {
		foObjectList = append(foObjectList, *fo)
	}

	return foObjectList, nil
}
