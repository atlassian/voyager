package wiringutil

import (
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
)

/*
This file provides helper functions to construct Smith Resource Names and metadata names for Kubernetes objects.
Mainly for use in autowiring functions. IN THE MAJORITY OF CASES, YOU DON'T NEED TO USE THEM. Except for maybe
ReferenceName() and ResourceName().

Terminology:
- Object - a Kubernetes object.
  Example: ConfigMap
- Smith Resource - Smith Bundle contains a list of Objects and/or Smith plugins (more types in the future).
  Each of those things is referred to as a Smith Resource.
  Example: ConfigMap, iam plugin
- Voyager Resource - a logical, highly abstract representation of a resource. This is what we have in ServiceDescriptor,
  LocationDescriptor and State objects.  Single Voyager Resource may be represented
  by 1 or more Smith Resources, depending on what the autowiring function for the Voyager
  Resource's type produces for a particular set of inputs.
  Examples: Postgresql DB, DynamoDb table, SQS, EC2Compute.

Functions below are named to signal what kind of name they construct and whether it is for a Smith Resource
(not Voyager Resource - user provides them) or it is a meta name for an Object, produced by that Smith Resource.

Postfixes should be used by autowiring function to disambiguate objects produced by the function itself.
Postfixes may contain `-` to separate logical pieces if needed.

Implementation details/notes
- All names start with a Voyager Resource name for which the autowiring function is being executed. This provides
  namespacing so that Smith Resources and Objects from different Voyager Resources (i.e. from the corresponding
  autowiring functions) do not clash.
*/

// ReferenceName constructs a Smith resource reference name for a particular resource name and name elements.
func ReferenceName(producer smith_v1.ResourceName, nameElems ...string) smith_v1.ReferenceName {
	allNameElems := append([]string{string(producer)}, nameElems...)
	return smith_v1.ReferenceName(strings.Join(allNameElems, "-"))
}

func ConsumerProducerResourceName(consumer, producer voyager.ResourceName, postfixParts ...string) smith_v1.ResourceName {
	nameParts := append([]string{string(consumer), string(producer)}, postfixPartsToNamePart(postfixParts...)...)
	return smith_v1.ResourceName(joinNameParts(nameParts...))
}

func ResourceName(resource voyager.ResourceName, postfixParts ...string) smith_v1.ResourceName {
	nameParts := append([]string{string(resource)}, postfixPartsToNamePart(postfixParts...)...)
	return smith_v1.ResourceName(joinNameParts(nameParts...))
}

func ConsumerProducerMetaName(consumer, producer voyager.ResourceName, postfixParts ...string) string {
	nameParts := append([]string{string(consumer), string(producer)}, postfixPartsToNamePart(postfixParts...)...)
	return joinNameParts(nameParts...)
}

func MetaName(resource voyager.ResourceName, postfixParts ...string) string {
	nameParts := append([]string{string(resource)}, postfixPartsToNamePart(postfixParts...)...)
	return joinNameParts(nameParts...)
}

// joinNameParts joins pieces of a name.
// voyager.ResourceName cannot contain more than one `-` in a row so it is safe to construct
// smith_v1.ResourceName and meta names for Kubernetes objects using `--` as a delimiter as long as one or more
// starting parts of the name provide namespacing to avoid clashes.
func joinNameParts(parts ...string) string {
	return strings.Join(parts, "--")
}

func postfixPartsToNamePart(postfixParts ...string) []string {
	if len(postfixParts) == 0 {
		return nil
	}
	return []string{strings.Join(postfixParts, "-")}
}
