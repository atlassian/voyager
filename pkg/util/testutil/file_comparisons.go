//

package testutil

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

type FileName string

// Marshals both structs into JSON, compares them and prints text diff between them
func JSONCompare(t *testing.T, expectedData, actualData interface{}) bool {
	return JSONCompareContext(t, FileName("expected"), expectedData, actualData)
}

// Marshals both structs into JSON, compares them and prints text diff between them.
// Accepts expectedFileName to print Unix patch that can be applied on top of the file.
func JSONCompareContext(t *testing.T, expectedFileName FileName, expectedData, actualData interface{}) bool {
	expectedResult, err := json.MarshalIndent(expectedData, "", "  ")
	require.NoError(t, err)
	actualResult, err := json.MarshalIndent(actualData, "", "  ")
	require.NoError(t, err)
	return TextCompare(t, string(expectedFileName), "actual", string(expectedResult), string(actualResult))
}

// Marshals both structs into YAML, compares them and prints text diff between them
func YAMLCompare(t *testing.T, expectedData, actualData interface{}) bool {
	return YAMLCompareContext(t, FileName("expected"), expectedData, actualData)
}

// Marshals both structs into YAML, compares them and prints text diff between them.
// Accepts expectedFileName to print Unix patch that can be applied on top of the file.
func YAMLCompareContext(t *testing.T, expectedFileName FileName, expectedData, actualData interface{}) bool {
	expectedResult, err := yaml.Marshal(expectedData)
	require.NoError(t, err)
	actualResult, err := yaml.Marshal(actualData)
	require.NoError(t, err)
	return TextCompare(t, string(expectedFileName), "actual", string(expectedResult), string(actualResult))
}

func ReadCompare(t *testing.T, expectedFileName, actualFileName FileName, actualData string) bool {
	expectedData, err := ioutil.ReadFile(string(expectedFileName))
	require.NoError(t, err)
	return FileCompare(t, expectedFileName, actualFileName, string(expectedData), actualData)
}

// functions like assert in that it returns true/false if there is no difference and marks the test as failed
// while allowing the test to continue
func FileCompare(t *testing.T, expectedFileName, actualFileName FileName, expectedData, actualData string) bool {
	expectedLines := difflib.SplitLines(strings.TrimSpace(expectedData))
	actualLines := difflib.SplitLines(strings.TrimSpace(actualData))
	return TextLinesCompare(t, string(expectedFileName), string(actualFileName), expectedLines, actualLines)
}

func TextCompare(t *testing.T, expectedTextName, actualTextName string, expectedText, actualText string) bool {
	return TextLinesCompare(t, expectedTextName, actualTextName, difflib.SplitLines(expectedText), difflib.SplitLines(actualText))
}

func TextLinesCompare(t *testing.T, expectedTextName, actualTextName string, expectedLines, actualLines []string) bool {
	diff := difflib.UnifiedDiff{
		A:        expectedLines,
		B:        actualLines,
		FromFile: expectedTextName,
		ToFile:   actualTextName,
		Context:  3,
	}

	text, err := difflib.GetUnifiedDiffString(diff)
	require.NoError(t, err)

	if text != "" {
		t.Log(text)
		assert.Fail(t, "comparison failed")
		return false
	}
	return true
}
