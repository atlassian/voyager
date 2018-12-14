package oap

import (
	"encoding/json"

	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// For some reason spec is mix of OAP/AWS resource attributes AND Micros2 parameters, we filter out known non-attributes and pass-through all remaining ones as
// OAP/AWS attributes.
type nonAttributesInSpec struct {
	InstanceID   string          `json:"instanceId"` // Common to all OSB things
	ServiceName  string          `json:"serviceName"`
	ResourceName string          `json:"resourceName"`
	Alarms       json.RawMessage `json:"alarms"`
}

// Gets serviceName from resource's spec if present or "" otherwise
// If the spec is empty or does not contain the ServiceName, this returns the empty string.
func ServiceName(resourceSpec *runtime.RawExtension) (string, error) {
	if resourceSpec == nil {
		return "", nil
	}
	spec := nonAttributesInSpec{}
	err := json.Unmarshal(resourceSpec.Raw, &spec)
	if err != nil {
		return "", errors.Wrap(err, "unmarshalling resource failed")
	}

	return spec.ServiceName, nil
}

func ResourceName(resourceSpec *runtime.RawExtension) (string, error) {
	if resourceSpec == nil {
		return "", nil
	}
	spec := nonAttributesInSpec{}
	err := json.Unmarshal(resourceSpec.Raw, &spec)
	if err != nil {
		return "", errors.Wrap(err, "unmarshalling resource failed")
	}

	return spec.ResourceName, nil
}

func Alarms(resourceSpec *runtime.RawExtension) (json.RawMessage, error) {
	if resourceSpec == nil {
		return nil, nil
	}
	spec := nonAttributesInSpec{}
	err := json.Unmarshal(resourceSpec.Raw, &spec)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling resource failed")
	}

	return spec.Alarms, nil
}

// From a resource spec, filters out the serviceName, instanceId and alarms.
func FilterAttributes(resourceSpec *runtime.RawExtension) (map[string]interface{}, error) {
	var rawSpec map[string]interface{}

	if resourceSpec == nil {
		return rawSpec, nil // Well, you gave me a nil spec so I guess there's no attributes
	}
	if err := json.Unmarshal(resourceSpec.Raw, &rawSpec); err != nil {
		return nil, errors.Wrap(err, "unmarshalling resource.Spec.Raw failed")
	}

	wiringutil.StripJSONFields(rawSpec, nonAttributesInSpec{})

	return rawSpec, nil
}

// From a resource spec, performs FilterAttributes() and  then applies defaults.
func BuildAttributes(resourceSpec *runtime.RawExtension, resourceDefaults *runtime.RawExtension) (map[string]interface{}, error) {
	attributes, err := FilterAttributes(resourceSpec)
	if err != nil {
		return nil, err
	}
	if resourceDefaults != nil {
		var defaults map[string]interface{}
		if err = json.Unmarshal(resourceDefaults.Raw, &defaults); err != nil {
			return nil, errors.Wrap(err, "failed to unpack defaults as map[string]interface{}")
		}
		attributes, err = wiringutil.Merge(attributes, defaults)
		if err != nil {
			return nil, err
		}
	}
	return attributes, nil
}
