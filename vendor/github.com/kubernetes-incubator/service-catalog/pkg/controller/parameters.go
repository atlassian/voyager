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

package controller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/peterbourgon/mergemap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// buildParameters generates the parameters JSON structure to be passed
// to the broker.
// The first return value is a map of parameters to send to the Broker, including
// secret values.
// The second return value is a map of parameters with secret values redacted,
// replaced with "<redacted>".
// The third return value is any error that caused the function to fail.
func buildParameters(kubeClient kubernetes.Interface, namespace string, parametersFrom []v1beta1.ParametersFromSource, parameters *runtime.RawExtension) (map[string]interface{}, map[string]interface{}, error) {
	params := make(map[string]interface{})
	paramsWithSecretsRedacted := make(map[string]interface{})
	if parametersFrom != nil {
		for _, p := range parametersFrom {
			fps, err := fetchParametersFromSource(kubeClient, namespace, &p)
			if err != nil {
				return nil, nil, err
			}
			for k, v := range fps {
				if _, ok := params[k]; ok {
					return nil, nil, fmt.Errorf("conflict: duplicate entry for parameter %q", k)
				}
				params[k] = v
				paramsWithSecretsRedacted[k] = "<redacted>"
			}
		}
	}
	if parameters != nil {
		pp, err := UnmarshalRawParameters(parameters.Raw)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range pp {
			if _, ok := params[k]; ok {
				return nil, nil, fmt.Errorf("conflict: duplicate entry for parameter %q", k)
			}
			params[k] = v
			paramsWithSecretsRedacted[k] = v
		}
	}
	// Replace empty map with nil so that the params are omitted from the request
	if len(params) == 0 {
		params = nil
	}
	// Replace empty map with nil so that the params are omitted from the properties state
	if len(paramsWithSecretsRedacted) == 0 {
		paramsWithSecretsRedacted = nil
	}
	return params, paramsWithSecretsRedacted, nil
}

// fetchParametersFromSource fetches data from a specified external source and
// represents it in the parameters map format
func fetchParametersFromSource(kubeClient kubernetes.Interface, namespace string, parametersFrom *v1beta1.ParametersFromSource) (map[string]interface{}, error) {
	var params map[string]interface{}
	if parametersFrom.SecretKeyRef != nil {
		data, err := fetchSecretKeyValue(kubeClient, namespace, parametersFrom.SecretKeyRef)
		if err != nil {
			return nil, err
		}
		p, err := unmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		params = p

	}
	return params, nil
}

// UnmarshalRawParameters produces a map structure from a given raw YAML/JSON input
func UnmarshalRawParameters(in []byte) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	if len(in) > 0 {
		if err := yaml.Unmarshal(in, &parameters); err != nil {
			return parameters, err
		}
	}
	return parameters, nil
}

// MarshalRawParameters marshals the specified map of parameters into JSON
func MarshalRawParameters(in map[string]interface{}) ([]byte, error) {
	if in == nil || len(in) == 0 {
		return nil, nil
	}
	return json.Marshal(in)
}

// unmarshalJSON produces a map structure from a given raw JSON input
func unmarshalJSON(in []byte) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	if err := json.Unmarshal(in, &parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters as JSON object: %v", err)
	}
	return parameters, nil
}

// fetchSecretKeyValue requests and returns the contents of the given secret key
func fetchSecretKeyValue(kubeClient kubernetes.Interface, namespace string, secretKeyRef *v1beta1.SecretKeyReference) ([]byte, error) {
	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(secretKeyRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data[secretKeyRef.Key], nil
}

// generateChecksumOfParameters generates a checksum for the map of parameters.
// This checksum is used to determine if parameters have changed.
func generateChecksumOfParameters(params map[string]interface{}) (string, error) {
	if params == nil || len(params) == 0 {
		return "", nil
	}
	paramsAsJSON, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(paramsAsJSON)
	return fmt.Sprintf("%x", hash), nil
}

// prepareInProgressPropertyParameters generates the required parameters for setting
// the in-progress status of a Type.
// Returns (parameters, parametersChecksum, rawParametersWithRedaction, err) where
// 1 - a map of parameters to send to the Broker, including secret values.
// 2 - a checksum for the map of parameters. This checksum is used to determine if parameters have changed.
// 3 - the map of parameters marshaled into JSON as a RawExtension
// 4 - any error that caused the function to fail.
func prepareInProgressPropertyParameters(kubeClient kubernetes.Interface, namespace string, specParameters *runtime.RawExtension, specParametersFrom []v1beta1.ParametersFromSource) (map[string]interface{}, string, *runtime.RawExtension, error) {
	parameters, parametersWithSecretsRedacted, err := buildParameters(kubeClient, namespace, specParametersFrom, specParameters)
	if err != nil {
		return nil, "", nil, fmt.Errorf(
			"failed to prepare parameters %s: %s",
			specParameters, err,
		)
	}

	parametersChecksum, err := generateChecksumOfParameters(parameters)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to generate the parameters checksum to store in Status: %s", err)
	}

	marshalledParametersWithRedaction, err := MarshalRawParameters(parametersWithSecretsRedacted)
	if err != nil {
		return nil, "", nil, fmt.Errorf(
			"failed to marshal the parameters to store in the Status: %s",
			err,
		)
	}

	var rawParametersWithRedaction *runtime.RawExtension
	if marshalledParametersWithRedaction != nil {
		rawParametersWithRedaction = &runtime.RawExtension{
			Raw: marshalledParametersWithRedaction,
		}
	}

	return parameters, parametersChecksum, rawParametersWithRedaction, err
}

// mergeParameters applies overrides on top of a set of default parameters.
func mergeParameters(params *runtime.RawExtension, defaultParams *runtime.RawExtension) (*runtime.RawExtension, error) {
	if defaultParams == nil || defaultParams.Raw == nil || string(defaultParams.Raw) == "" {
		return params, nil
	}

	if params == nil || params.Raw == nil || string(params.Raw) == "" {
		return defaultParams, nil
	}

	paramsMap := make(map[string]interface{})
	err := json.Unmarshal(params.Raw, &paramsMap)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal parameters %v: %s", string(params.Raw), err)
	}

	defaultParamsMap := make(map[string]interface{})
	err = json.Unmarshal(defaultParams.Raw, &defaultParamsMap)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal default parameters %v: %s", string(defaultParams.Raw), err)
	}

	merged := mergemap.Merge(defaultParamsMap, paramsMap)

	result, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("could not merge parameters %v with %v: %s", string(params.Raw), string(defaultParams.Raw), err)
	}

	return &runtime.RawExtension{Raw: result}, nil
}
