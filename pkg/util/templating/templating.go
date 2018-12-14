package templating

import (
	"encoding/json"
	"strings"

	"github.com/atlassian/voyager/pkg/util"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/schollz/closestmatch"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	TransformerKeywordInline = "${inline}"
	errMsgNotMap             = "only maps can be inherited"
)

// Used to expand "templated" variables in Specs
type SpecExpander struct {
	VarResolver      VariableResolver
	RequiredPrefix   string
	ReservedPrefixes []string // If the source prefix isn't matched and isn't any of these then it's an error
}

// Resolves all the variables used in the resource spec
func (e *SpecExpander) Expand(spec *runtime.RawExtension) (*runtime.RawExtension, *util.ErrorList) {
	errorList := util.NewErrorList()
	if spec == nil {
		return nil, errorList
	}

	specMap, err := specToMap(spec)
	if err != nil {
		errorList.Add(err)
		return nil, errorList
	}

	expandedSpec, expandErr := e.expandValue(specMap)
	if expandErr != nil {
		errorList.AddErrorList(expandErr)
		return nil, errorList
	}

	asJSON, err := json.Marshal(expandedSpec)
	if err != nil {
		errorList.Add(err)
		return nil, errorList
	}

	return &runtime.RawExtension{
		Raw: asJSON,
	}, nil
}

func (e *SpecExpander) expandValue(vars interface{}) (interface{}, *util.ErrorList) {
	switch varsToExpand := vars.(type) {
	case map[string]interface{}:
		return e.expandMapValue(varsToExpand)
	case []interface{}:
		return e.expandListValue(varsToExpand)
	case string:
		return e.expandStringValue(varsToExpand)
	default:
		return varsToExpand, nil
	}
}

// Iterates through a map value and expands all the values in there.
// If a key has a special value of "$inline" and the value is a map, it will merge the value's keys with the existing map
func (e *SpecExpander) expandMapValue(mapVar map[string]interface{}) (interface{}, *util.ErrorList) {
	var expanded = make(map[string]interface{})
	errorList := util.NewErrorList()

	// We first handle the "$inline" keyword so that the expanded map
	// starts with the "inlineKeyword" keys. The other keys in the map
	// can then override the values in the inlineKeyword map
	if inlineKeyword, found := mapVar[TransformerKeywordInline]; found {
		expandedValue, err := e.expandValue(inlineKeyword)
		if err != nil {
			errorList.Add(err)
			if !err.CanRecover() {
				return nil, errorList
			}
		}

		// We must check if it's an inline block with a reserved key
		stringExpanded, isType := expandedValue.(string)
		if isType {
			if isReservedPrefix(stringExpanded, e.ReservedPrefixes) {
				expanded[TransformerKeywordInline] = stringExpanded
			}
		} else {
			// We can only inline "maps"
			expandedValueTyped, isType := expandedValue.(map[string]interface{})
			if !isType {
				errorList.Add(errorList, errors.New(errMsgNotMap))
				return nil, errorList
			}
			result, mergeErr := Merge(expandedValueTyped, expanded)
			if mergeErr != nil {
				errorList.Add(mergeErr)
				return nil, errorList
			}
			expanded = result.(map[string]interface{})
		}
	}

	for k, v := range mapVar {
		if k == TransformerKeywordInline {
			// We already processed the inline keyword above
			continue
		}

		expandedValue, err := e.expandValue(v)
		if err != nil {
			errorList.Add(err)
			if !err.CanRecover() {
				return nil, errorList
			}
		}

		if _, ok := expandedValue.(map[string]interface{}); ok {
			if val, found := expanded[k]; found {
				if asMap, isMap := val.(map[string]interface{}); isMap {
					result, err := Merge(expandedValue, asMap)
					if err != nil {
						errorList.Add(err)
						return nil, errorList
					}
					expanded[k] = result
					continue
				}
			}
		}

		expanded[k] = expandedValue
	}

	if errorList.HasErrors() {
		return nil, errorList
	}

	return expanded, nil
}

// Iterates through a list value and expands all the elements in there
func (e *SpecExpander) expandListValue(listVar []interface{}) (interface{}, *util.ErrorList) {
	expanded := make([]interface{}, len(listVar))
	errorList := util.NewErrorList()

	for idx, listValue := range listVar {
		expandedValue, err := e.expandValue(listValue)
		if err != nil {
			errorList.AddErrorList(err)
			if !err.CanRecover() {
				return nil, errorList
			}
		}

		expanded[idx] = expandedValue
	}

	if errorList.HasErrors() {
		return nil, errorList
	}

	return expanded, nil
}

// Expands a string (replacexpandValuee keywords or the jsonpath element) or return the string as is there is no expansion
func (e *SpecExpander) expandStringValue(stringVar string) (interface{}, *util.ErrorList) {
	ve := NewVariableExpander(e.VarResolver, e.RequiredPrefix)
	res, err := ve.Expand(stringVar)
	if err != nil && err.HasErrors() {
		if !ve.ValidPrefix(stringVar) {
			if isReservedPrefix(stringVar, e.ReservedPrefixes) {
				return stringVar, nil
			}
			err.Add(errors.Errorf("%s was not one of the expected prefixes: %s", stringVar, strings.Join(e.ReservedPrefixes, ", ")))
		}
		return nil, err
	}

	return res, nil
}

func isReservedPrefix(stringVar string, reservedPrefixes []string) bool {
	for _, reserved := range reservedPrefixes {
		if NewVariableExpander(func(varName string) (interface{}, error) {
			return varName, nil
		}, reserved).ValidPrefix(stringVar) {
			return true
		}
	}
	return false
}

func specToMap(spec *runtime.RawExtension) (map[string]interface{}, error) {
	var res map[string]interface{}

	err := json.Unmarshal(spec.Raw, &res)

	return res, err
}

func Merge(winner, loser interface{}) (interface{}, error) {
	if loser == nil {
		return winner, nil
	}

	if winner == nil {
		return loser, nil
	}

	switch typedLoser := loser.(type) {
	case map[string]interface{}:
		dst := make(map[string]interface{})

		// Basically make a deep copy
		if err := mergo.Merge(&dst, winner); err != nil {
			return nil, err
		}

		// Only add the items that are not already there
		if err := mergo.Merge(&dst, loser); err != nil {
			return nil, err
		}

		return dst, nil

	case []interface{}:
		if typedWinner, isList := winner.([]interface{}); isList {
			return append(typedWinner, typedLoser...), nil
		}
		return nil, errors.New("cannot merge different types")
	default:
		return winner, nil
	}
}

// Recursively search in a map of maps for the specified key. keyPath contains the keys to
// use in the applicable (child)map
func FindInMapRecursive(src map[string]interface{}, keyPath []string) (interface{}, error) {
	top := keyPath[0]
	restOfPath := keyPath[1:]
	var value interface{}
	var found bool

	if value, found = src[top]; !found {
		var keys []string
		for key := range src {
			keys = append(keys, key)
		}
		closest := closestmatch.New(keys, []int{1}).Closest(top)
		return nil, util.NewErrVariableNotFound(strings.Join(keyPath, "."), closest)
	}

	if len(restOfPath) == 0 {
		return value, nil
	}

	var varMap map[string]interface{}
	var isMap bool

	varMap, isMap = value.(map[string]interface{})
	if !isMap {
		return nil, errors.New("key must refer to map")
	}

	return FindInMapRecursive(varMap, restOfPath)
}
