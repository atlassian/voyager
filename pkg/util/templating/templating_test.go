package templating

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestExpandStringValue(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return "value", nil
	}
	stringToExpand := "${foobar.stuff}"
	expectedResult := "value"

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandStringValue(stringToExpand)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)
}

func TestExpandStringValueWithInvalidPrefix(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return "value", nil
	}
	goodString := "${expected_prefix.foobar.stuff}"
	badString := "${bad_prefix.foobar.stuff}"
	expectedResult := "value"

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "expected_prefix.",
		ReservedPrefixes: []string{},
	}

	res, errList := sut.expandStringValue(goodString)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)

	res, errList = sut.expandStringValue(badString)
	assert.NotNil(t, errList)
	assert.Equal(t, nil, res)
}

func TestExpandStringValueWithReservedPrefixes(t *testing.T) {
	t.Parallel()

	expectedResult := "value"
	resolveFunc := func(varName string) (interface{}, error) {
		return expectedResult, nil
	}
	goodString := "${expected_prefix:-_foobar.stuff}"
	goodReservedString := "${reserved_prefix.foobar.stuff}"
	anotherGoodReservedString := "${secret_prefix.foobar.stuff}"
	badString := "${bad_prefix.foobar.stuff}"

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "expected_prefix:-_",
		ReservedPrefixes: []string{"reserved_prefix.", "secret_prefix."},
	}

	res, errList := sut.expandStringValue(goodString)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)

	res, errList = sut.expandStringValue(goodReservedString)
	assert.Nil(t, errList)
	assert.Equal(t, goodReservedString, res)

	res, errList = sut.expandStringValue(anotherGoodReservedString)
	assert.Nil(t, errList)
	assert.Equal(t, anotherGoodReservedString, res)

	res, errList = sut.expandStringValue(badString)
	assert.Error(t, errList)
	assert.Equal(t, nil, res)
}

func TestExpandMapValue(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return "value", nil
	}
	mapToExpand := map[string]interface{}{
		"key1": map[string]interface{}{
			"key": "${expand.me}",
		},
		"key2": map[string]interface{}{
			"key": "${expand.me}",
		},
	}
	expectedResult := map[string]interface{}{
		"key1": map[string]interface{}{
			"key": "value",
		},
		"key2": map[string]interface{}{
			"key": "value",
		},
	}

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandMapValue(mapToExpand)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)
}

func TestExpandMapValueWithInlineWithMerging(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return map[string]interface{}{
			"key": map[string]interface{}{
				"key":  "value",
				"more": "stuff",
				"in":   "here",
			},
		}, nil
	}
	mapToExpand := map[string]interface{}{
		"key1":      "value",
		"${inline}": "${expand.me}",
		"key": map[string]interface{}{
			"should": "merge",
		},
	}
	expectedResult := map[string]interface{}{
		"key1": "value",
		"key": map[string]interface{}{
			"key":    "value",
			"more":   "stuff",
			"in":     "here",
			"should": "merge",
		},
	}

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandMapValue(mapToExpand)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)
}

func TestExpandMapValueWithInlineBlock(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return map[string]interface{}{
			"key": "value",
		}, nil
	}
	mapToExpand := map[string]interface{}{
		"key1": map[string]interface{}{
			"key": "${expand.me}",
		},
		"key2": map[string]interface{}{
			"${inline}": "${expand.me}",
			"more":      "${expand.me}",
			"keys":      "stuff",
		},
	}
	expectedResult := map[string]interface{}{
		"key1": map[string]interface{}{
			"key": map[string]interface{}{
				"key": "value",
			},
		},
		"key2": map[string]interface{}{
			"key": "value",
			"more": map[string]interface{}{
				"key": "value",
			},
			"keys": "stuff",
		},
	}

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandMapValue(mapToExpand)
	assert.Nil(t, errList)
	assert.Equal(t, expectedResult, res)
}

func TestExpandListValue(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return "value", nil
	}
	listVars := []interface{}{
		"${expand.me}",
		"dont_expand_me",
		"${expand.me}",
		"${expand.me.please}",
	}
	expectedList := []interface{}{
		"value",
		"dont_expand_me",
		"value",
		"value",
	}

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandListValue(listVars)
	assert.Nil(t, errList)
	assert.Equal(t, expectedList, res)
}

func TestExpandListValueWithComplexItems(t *testing.T) {
	t.Parallel()

	resolveFunc := func(varName string) (interface{}, error) {
		return "value", nil
	}
	listVars := []interface{}{
		map[string]interface{}{
			"key": "${expand.me}",
		},
		map[string]interface{}{
			"key": "${expand.me}",
			"deeper": map[string]interface{}{
				"key":  "${expand.me.too}",
				"key2": "dont_expand_me",
			},
		},
		map[string]interface{}{
			"key": "dont_expand_me",
		},
		map[string]interface{}{
			"key": "${expand.me}",
		},
	}
	expectedList := []interface{}{
		map[string]interface{}{
			"key": "value",
		},
		map[string]interface{}{
			"key": "value",
			"deeper": map[string]interface{}{
				"key":  "value",
				"key2": "dont_expand_me",
			},
		},
		map[string]interface{}{
			"key": "dont_expand_me",
		},
		map[string]interface{}{
			"key": "value",
		},
	}

	sut := &SpecExpander{
		VarResolver:      resolveFunc,
		RequiredPrefix:   "",
		ReservedPrefixes: []string{},
	}
	res, errList := sut.expandListValue(listVars)
	assert.Nil(t, errList)
	assert.Equal(t, expectedList, res)
}

func TestFindInMapRecursive(t *testing.T) {
	t.Parallel()

	m := map[string]interface{}{
		"top": map[string]interface{}{
			"deeper": map[string]interface{}{
				"key": "value",
			},
			"key": "value",
		},
	}

	keys := []string{"top", "deeper", "key"}
	val, err := FindInMapRecursive(m, keys)
	assert.NoError(t, err)
	assert.Equal(t, val, "value")

	keys = []string{"top", "key"}
	val, err = FindInMapRecursive(m, keys)
	assert.NoError(t, err)
	assert.Equal(t, val, "value")

	keys = []string{"top", "deeper", "fleeb"}
	val, err = FindInMapRecursive(m, keys)
	assert.Error(t, err)
	assert.Equal(t, val, nil)

	keys = []string{"top", "key", "should-not-get-to-here"}
	val, err = FindInMapRecursive(m, keys)
	assert.Error(t, err)
	assert.Equal(t, val, nil)

	keys = []string{"top", "dooper", "key"}
	val, err = FindInMapRecursive(m, keys)
	assert.EqualError(t, err, "variable not defined: \"dooper.key\", did you mean \"deeper\"")
}

func TestNestedMapMerge(t *testing.T) {
	t.Parallel()

	m1 := map[string]interface{}{
		"top": map[string]interface{}{
			"one":   "1",
			"three": "3",
		},
	}

	m2 := map[string]interface{}{
		"top": map[string]interface{}{
			"one":   "not-1",
			"foure": "4",
		},
	}

	expected := map[string]interface{}{
		"top": map[string]interface{}{
			"one":   "not-1",
			"three": "3",
			"foure": "4",
		},
	}

	result, err := Merge(m2, m1)
	require.NoError(t, err, "Couldn't merge")
	assert.Equal(t, expected, result, "Maps don't match")
}

func TestSpecToMapCreatesExpectedStructure(t *testing.T) {
	t.Parallel()
	jsonSpecString := `{
		"sd": {
			"links": {
				"binary": {
					"name": "docker.example.com/micros/node-refapp",
					"tag": "${release:abc.def}",
					"type": "docker"
				}
			}
		}
	}`
	spec := buildMockSpec(jsonSpecString)
	expectedMapVal := map[string]interface{}{
		"sd": map[string]interface{}{
			"links": map[string]interface{}{
				"binary": map[string]interface{}{
					"name": "docker.example.com/micros/node-refapp",
					"tag":  "${release:abc.def}",
					"type": "docker",
				},
			},
		},
	}
	mapval, err := specToMap(spec)
	assert.NoError(t, err)
	assert.Equal(t, mapval, expectedMapVal)
}

func buildMockSpec(jsonString string) *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: createJsonBytes(jsonString),
	}
}

func createJsonBytes(str string) []byte {
	var obj = make(map[string]interface{})
	err := json.Unmarshal([]byte(str), &obj)
	if err == nil {
		res, _ := json.Marshal(obj)
		return res
	}
	panic(fmt.Sprintf("your test data is wrong! Error: %s", err))
}
