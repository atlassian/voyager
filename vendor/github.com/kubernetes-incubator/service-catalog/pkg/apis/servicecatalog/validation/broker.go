/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	sc "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/kubernetes-incubator/service-catalog/pkg/filter"
)

// validateCommonServiceBrokerName is the validation function for common
// broker names.
var validateCommonServiceBrokerName = apivalidation.NameIsDNSSubdomain

// ValidateClusterServiceBroker implements the validation rules for a
// ClusterServiceBroker.
func ValidateClusterServiceBroker(broker *sc.ClusterServiceBroker) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs,
		apivalidation.ValidateObjectMeta(&broker.ObjectMeta,
			false, /* namespace required */
			validateCommonServiceBrokerName,
			field.NewPath("metadata"))...)

	allErrs = append(allErrs, validateClusterServiceBrokerSpec(&broker.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateClusterServiceBrokerSpec(spec *sc.ClusterServiceBrokerSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// if there is auth information, check it to make sure that it's properly formatted
	if spec.AuthInfo != nil {
		if spec.AuthInfo.Basic != nil {
			secretRef := spec.AuthInfo.Basic.SecretRef
			if secretRef != nil {
				for _, msg := range apivalidation.ValidateNamespaceName(secretRef.Namespace, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "basic", "secretRef", "namespace"), secretRef.Namespace, msg))
				}
				for _, msg := range apivalidation.NameIsDNSSubdomain(secretRef.Name, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "basic", "secretRef", "name"), secretRef.Name, msg))
				}
			} else {
				allErrs = append(
					allErrs,
					field.Required(fldPath.Child("authInfo", "basic", "secretRef"), "a basic auth secret is required"),
				)
			}
		} else if spec.AuthInfo.Bearer != nil {
			secretRef := spec.AuthInfo.Bearer.SecretRef
			if secretRef != nil {
				for _, msg := range apivalidation.ValidateNamespaceName(secretRef.Namespace, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "bearer", "secretRef", "namespace"), secretRef.Namespace, msg))
				}
				for _, msg := range apivalidation.NameIsDNSSubdomain(secretRef.Name, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "bearer", "secretRef", "name"), secretRef.Name, msg))
				}
			} else {
				allErrs = append(
					allErrs,
					field.Required(fldPath.Child("authInfo", "bearer", "secretRef"), "a basic auth secret is required"),
				)
			}
		} else {
			// Authentication
			allErrs = append(
				allErrs,
				field.Required(fldPath.Child("authInfo"), "auth config is required"),
			)
		}
	}

	commonErrs := validateCommonServiceBrokerSpec(&spec.CommonServiceBrokerSpec, fldPath, true)

	if len(commonErrs) != 0 {
		allErrs = append(allErrs, commonErrs...)
	}

	return allErrs
}

// ValidateServiceBroker implements the validation rules for a
// ServiceBroker.
func ValidateServiceBroker(broker *sc.ServiceBroker) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs,
		apivalidation.ValidateObjectMeta(&broker.ObjectMeta,
			true, /* namespace required */
			validateCommonServiceBrokerName,
			field.NewPath("metadata"))...)

	allErrs = append(allErrs, validateServiceBrokerSpec(&broker.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateServiceBrokerSpec(spec *sc.ServiceBrokerSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// if there is auth information, check it to make sure that it's properly formatted
	if spec.AuthInfo != nil {
		if spec.AuthInfo.Basic != nil {
			secretRef := spec.AuthInfo.Basic.SecretRef
			if secretRef != nil {
				for _, msg := range apivalidation.NameIsDNSSubdomain(secretRef.Name, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "basic", "secretRef", "name"), secretRef.Name, msg))
				}
			} else {
				allErrs = append(
					allErrs,
					field.Required(fldPath.Child("authInfo", "basic", "secretRef"), "a basic auth secret is required"),
				)
			}
		} else if spec.AuthInfo.Bearer != nil {
			secretRef := spec.AuthInfo.Bearer.SecretRef
			if secretRef != nil {
				for _, msg := range apivalidation.NameIsDNSSubdomain(secretRef.Name, false /* prefix */) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("authInfo", "bearer", "secretRef", "name"), secretRef.Name, msg))
				}
			} else {
				allErrs = append(
					allErrs,
					field.Required(fldPath.Child("authInfo", "bearer", "secretRef"), "a basic auth secret is required"),
				)
			}
		} else {
			// Authentication
			allErrs = append(
				allErrs,
				field.Required(fldPath.Child("authInfo"), "auth config is required"),
			)
		}
	}

	commonErrs := validateCommonServiceBrokerSpec(&spec.CommonServiceBrokerSpec, fldPath, false)

	if len(commonErrs) != 0 {
		allErrs = append(allErrs, commonErrs...)
	}

	return allErrs
}

func validateCommonServiceBrokerSpec(spec *sc.CommonServiceBrokerSpec, fldPath *field.Path, isClusterServiceBroker bool) field.ErrorList {
	commonErrs := field.ErrorList{}

	if "" == spec.URL {
		commonErrs = append(commonErrs,
			field.Required(fldPath.Child("url"),
				"brokers must have a remote url to contact"))
	}

	if spec.InsecureSkipTLSVerify && len(spec.CABundle) > 0 {
		commonErrs = append(commonErrs, field.Invalid(fldPath.Child("caBundle"), spec.CABundle, "caBundle cannot be used when insecureSkipTLSVerify is true"))
	}

	if "" == spec.RelistBehavior {
		commonErrs = append(commonErrs,
			field.Required(fldPath.Child("relistBehavior"),
				"relist behavior is required"))
	}

	isValidRelistBehavior := spec.RelistBehavior == sc.ServiceBrokerRelistBehaviorDuration ||
		spec.RelistBehavior == sc.ServiceBrokerRelistBehaviorManual
	if !isValidRelistBehavior {
		errMsg := "relist behavior must be \"Manual\" or \"Duration\""
		commonErrs = append(
			commonErrs,
			field.Required(fldPath.Child("relistBehavior"), errMsg),
		)
	}

	if spec.RelistRequests < 0 {
		commonErrs = append(
			commonErrs,
			field.Required(fldPath.Child("relistRequests"), "relistRequests must be greater than zero"),
		)
	}

	if spec.RelistDuration != nil {
		zeroDuration := metav1.Duration{Duration: 0}
		if spec.RelistDuration.Duration <= zeroDuration.Duration {
			commonErrs = append(
				commonErrs,
				field.Required(fldPath.Child("relistDuration"), "relistDuration must be greater than zero"),
			)
		}
	}

	if spec.CatalogRestrictions != nil && len(spec.CatalogRestrictions.ServiceClass) > 0 {
		// confirm that the restrictions can turn into a predicate.
		_, err := filter.CreatePredicate(spec.CatalogRestrictions.ServiceClass)
		if err != nil {
			commonErrs = append(commonErrs,
				field.Invalid(fldPath.Child("catalogRestrictions", "serviceClass"),
					spec.CatalogRestrictions.ServiceClass, err.Error()))
		} else {
			for _, restriction := range spec.CatalogRestrictions.ServiceClass {
				p := filter.ExtractProperty(restriction)
				if !isClusterServiceBroker && !v1beta1.IsValidServiceClassProperty(p) ||
					isClusterServiceBroker && !v1beta1.IsValidClusterServiceClassProperty(p) {
					commonErrs = append(commonErrs,
						field.Invalid(fldPath.Child("catalogRestrictions", "serviceClass"),
							spec.CatalogRestrictions.ServiceClass, fmt.Sprintf("Invalid property: %s", p)))
				}
			}
		}
	}
	if spec.CatalogRestrictions != nil && len(spec.CatalogRestrictions.ServicePlan) > 0 {
		// confirm that the restrictions can turn into a predicate.
		_, err := filter.CreatePredicate(spec.CatalogRestrictions.ServicePlan)
		if err != nil {
			commonErrs = append(commonErrs,
				field.Invalid(fldPath.Child("catalogRestrictions", "servicePlan"),
					spec.CatalogRestrictions.ServicePlan, err.Error()))
		} else {
			for _, restriction := range spec.CatalogRestrictions.ServicePlan {
				p := filter.ExtractProperty(restriction)
				if !isClusterServiceBroker && !v1beta1.IsValidServicePlanProperty(p) ||
					isClusterServiceBroker && !v1beta1.IsValidClusterServicePlanProperty(p) {
					commonErrs = append(commonErrs,
						field.Invalid(fldPath.Child("catalogRestrictions", "servicePlan"),
							spec.CatalogRestrictions.ServicePlan, fmt.Sprintf("Invalid property: %s", p)))
				}
			}
		}
	}

	return commonErrs
}

// ValidateClusterServiceBrokerUpdate checks that when changing from an older broker to a newer broker is okay ?
func ValidateClusterServiceBrokerUpdate(new *sc.ClusterServiceBroker, old *sc.ClusterServiceBroker) field.ErrorList {
	allErrs := validateCommonServiceBrokerUpdate(&new.Spec.CommonServiceBrokerSpec, &old.Spec.CommonServiceBrokerSpec)
	allErrs = append(allErrs, ValidateClusterServiceBroker(new)...)
	return allErrs
}

// ValidateServiceBrokerUpdate checks that when changing from an older broker to a newer broker is okay ?
func ValidateServiceBrokerUpdate(new *sc.ServiceBroker, old *sc.ServiceBroker) field.ErrorList {
	allErrs := validateCommonServiceBrokerUpdate(&new.Spec.CommonServiceBrokerSpec, &old.Spec.CommonServiceBrokerSpec)
	allErrs = append(allErrs, ValidateServiceBroker(new)...)
	return allErrs
}

// validateCommonServiceBrokerUpdate checks that when changing from an older broker to a newer broker is okay ?
func validateCommonServiceBrokerUpdate(new *sc.CommonServiceBrokerSpec, old *sc.CommonServiceBrokerSpec) field.ErrorList {
	commonErrs := field.ErrorList{}

	// RelistRequests can be increasing to relist the broker, or equal to update other fields
	if new.RelistRequests < old.RelistRequests {
		commonErrs = append(commonErrs, field.Invalid(field.NewPath("spec").Child("relistRequests"), old.RelistRequests, "RelistRequests must be strictly increasing"))
	}

	return commonErrs
}

// ValidateClusterServiceBrokerStatusUpdate checks that when changing from an older broker to a newer broker is okay.
func ValidateClusterServiceBrokerStatusUpdate(new *sc.ClusterServiceBroker, old *sc.ClusterServiceBroker) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateClusterServiceBrokerUpdate(new, old)...)
	return allErrs
}

// ValidateServiceBrokerStatusUpdate checks that when changing from an older broker to a newer broker is okay.
func ValidateServiceBrokerStatusUpdate(new *sc.ServiceBroker, old *sc.ServiceBroker) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateServiceBrokerUpdate(new, old)...)
	return allErrs
}
