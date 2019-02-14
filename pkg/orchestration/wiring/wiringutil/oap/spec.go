package oap

import (
	"encoding/json"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// For some reason spec is mix of OAP/AWS resource attributes AND Micros2 parameters, we filter out known non-attributes and pass-through all remaining ones as
// OAP/AWS attributes.
type nonAttributesInSpec struct {
	InstanceID   string              `json:"instanceId"` // Common to all OSB things
	ServiceName  voyager.ServiceName `json:"serviceName"`
	ResourceName string              `json:"resourceName"`
	Alarms       json.RawMessage     `json:"alarms"`
}

// Gets serviceName from resource's spec if present or "" otherwise
// If the spec is empty or does not contain the ServiceName, this returns the empty string.
func ServiceName(resourceSpec *runtime.RawExtension) (voyager.ServiceName, error) {
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

func TemplateName(resourceSpec *runtime.RawExtension) (string, bool /* external */, bool /* retriable */, error) {
	attributes, err := FilterAttributes(resourceSpec)
	if err != nil {
		return "", false, false, err
	}
	templateAttribute, ok := attributes["template"]
	if !ok {
		// this is a user error - the template is missing
		return "", true, false, errors.Errorf("attribute template not found in the spec")
	}
	templateName, ok := templateAttribute.(string)
	if !ok {
		// this is a user errror - the template is not a string
		return "", true, false, errors.Errorf("attribute template must be string")
	}
	return templateName, false, false, nil
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
func BuildAttributes(resourceSpec *runtime.RawExtension, resourceDefaults *runtime.RawExtension) (map[string]interface{}, bool /* external */, bool /* retriable */, error) {
	attributes, err := FilterAttributes(resourceSpec)
	if err != nil {
		return nil, false, false, err
	}
	if resourceDefaults != nil {
		var defaults map[string]interface{}
		if err = json.Unmarshal(resourceDefaults.Raw, &defaults); err != nil {
			return nil, false, false, errors.Wrap(err, "failed to unpack defaults as map[string]interface{}")
		}
		attributes, err = wiringutil.Merge(attributes, defaults)
		if err != nil {
			return nil, false, false, err
		}
	}
	return attributes, false, false, nil
}
