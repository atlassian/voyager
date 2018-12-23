/*
Copyright 2017 The Kubernetes Authors.

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

package validation

import (
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	sc "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
)

const commonServiceClassNameMaxLength int = 63

const guidMaxLength int = 63

// validateCommonServiceClassName is the common validation function for
// service class types.
func validateCommonServiceClassName(value string, prefix bool) []string {
	var errs []string
	if len(value) > commonServiceClassNameMaxLength {
		errs = append(errs, utilvalidation.MaxLenError(commonServiceClassNameMaxLength))
	}
	if len(value) == 0 {
		errs = append(errs, utilvalidation.EmptyError())
	}

	return errs
}

// validateExternalID is the validation function for External IDs that
// have been passed in. External IDs used to be OpenServiceBrokerAPI
// GUIDs, so we will retain that form until there is another provider
// that desires a different form.  In the case of the OSBAPI we
// generate GUIDs for ServiceInstances and ServiceBindings, but for
// ServiceClasses and ServicePlan, they are part of the payload returned from
// the ServiceBroker.
func validateExternalID(value string) []string {
	var errs []string
	if len(value) > guidMaxLength {
		errs = append(errs, utilvalidation.MaxLenError(guidMaxLength))
	}
	if len(value) == 0 {
		errs = append(errs, utilvalidation.EmptyError())
	}
	return errs
}

// ValidateClusterServiceClass validates a ClusterServiceClass and returns a list of errors.
func ValidateClusterServiceClass(serviceclass *sc.ClusterServiceClass) field.ErrorList {
	return internalValidateClusterServiceClass(serviceclass)
}

// ValidateClusterServiceClassUpdate checks that when changing from an older
// ClusterServiceClass to a newer ClusterServiceClass is okay.
func ValidateClusterServiceClassUpdate(new *sc.ClusterServiceClass, old *sc.ClusterServiceClass) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, internalValidateClusterServiceClass(new)...)

	return allErrs
}

func internalValidateClusterServiceClass(clusterserviceclass *sc.ClusterServiceClass) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs,
		apivalidation.ValidateObjectMeta(
			&clusterserviceclass.ObjectMeta,
			false, /* namespace required */
			validateCommonServiceClassName,
			field.NewPath("metadata"))...)

	allErrs = append(allErrs, validateClusterServiceClassSpec(&clusterserviceclass.Spec, field.NewPath("spec"), true)...)
	return allErrs
}

func validateClusterServiceClassSpec(spec *sc.ClusterServiceClassSpec, fldPath *field.Path, create bool) field.ErrorList {
	allErrs := field.ErrorList{}

	if "" == spec.ClusterServiceBrokerName {
		allErrs = append(allErrs, field.Required(fldPath.Child("clusterServiceBrokerName"), "clusterServiceBrokerName is required"))
	}

	commonErrs := validateCommonServiceClassSpec(&spec.CommonServiceClassSpec, fldPath, create)

	if len(commonErrs) != 0 {
		allErrs = append(allErrs, commonErrs...)
	}

	return allErrs
}

// ValidateServiceClass validates a ServiceClass and returns a list of errors.
func ValidateServiceClass(serviceclass *sc.ServiceClass) field.ErrorList {
	return internalValidateServiceClass(serviceclass)
}

// ValidateServiceClassUpdate checks that when changing from an older
// ServiceClass to a newer ServiceClass is okay.
func ValidateServiceClassUpdate(new *sc.ServiceClass, old *sc.ServiceClass) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, internalValidateServiceClass(new)...)

	return allErrs
}

func internalValidateServiceClass(clusterserviceclass *sc.ServiceClass) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs,
		apivalidation.ValidateObjectMeta(
			&clusterserviceclass.ObjectMeta,
			true, /* namespace required */
			validateCommonServiceClassName,
			field.NewPath("metadata"))...)

	allErrs = append(allErrs, validateServiceClassSpec(&clusterserviceclass.Spec, field.NewPath("spec"), true)...)
	return allErrs
}

func validateServiceClassSpec(spec *sc.ServiceClassSpec, fldPath *field.Path, create bool) field.ErrorList {
	allErrs := field.ErrorList{}

	if "" == spec.ServiceBrokerName {
		allErrs = append(allErrs, field.Required(fldPath.Child("serviceBrokerName"), "serviceBrokerName is required"))
	}

	commonErrs := validateCommonServiceClassSpec(&spec.CommonServiceClassSpec, fldPath, create)

	if len(commonErrs) != 0 {
		allErrs = append(commonErrs)
	}

	return allErrs
}

func validateCommonServiceClassSpec(spec *sc.CommonServiceClassSpec, fldPath *field.Path, create bool) field.ErrorList {
	commonErrs := field.ErrorList{}

	if "" == spec.ExternalID {
		commonErrs = append(commonErrs, field.Required(fldPath.Child("externalID"), "externalID is required"))
	}

	if "" == spec.Description {
		commonErrs = append(commonErrs, field.Required(fldPath.Child("description"), "description is required"))
	}

	for _, msg := range validateCommonServiceClassName(spec.ExternalName, false /* prefix */) {
		commonErrs = append(commonErrs, field.Invalid(fldPath.Child("externalName"), spec.ExternalName, msg))
	}
	for _, msg := range validateExternalID(spec.ExternalID) {
		commonErrs = append(commonErrs, field.Invalid(fldPath.Child("externalID"), spec.ExternalID, msg))
	}

	return commonErrs
}
