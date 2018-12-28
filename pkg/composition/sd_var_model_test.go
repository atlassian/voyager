package composition

import (
	"testing"

	"github.com/atlassian/voyager/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhenKeyNotInConfigThenError(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_key_nil_or_missing.yml")

	_, err := cfg.getVar([]string{"dev", "nil_or_missing"}, "compute.scaling.missingmsg")
	require.Error(t, err)

	_, isErrVariableNotFound := err.(*util.ErrVariableNotFound)
	assert.True(t, isErrVariableNotFound, "Incorrect error returned")
}

func TestNilKeyValueIsNil(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_key_nil_or_missing.yml")

	theVal, err := cfg.getVar([]string{"dev", "nil_or_missing"}, "compute.scaling.nilmsg")
	require.NoError(t, err)
	assert.Nil(t, theVal, "Did not expect a value")
}

func TestVarLookupIncompleteHierarchyDefined(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_var_lookup.yml")
	theVal, err := cfg.getVar([]string{"dev", "regionWithNoLabel", "locationLabel", "locationAccount"}, "compute.scaling.msg")
	require.NoError(t, err)
	assert.Equal(t, "global-dev-regionWithNoLabel", theVal, "Did not retrieve correct value")
}

func TestAccountOverridesType(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_var_lookup.yml")
	theVal, err := cfg.getVar([]string{"dev", "TestAccountOverridesType", "myLabel", "A234"}, "compute.scaling.msg")
	require.NoError(t, err)
	assert.Equal(t, "accountLevelMsg", theVal, "Did not retrieve correct value")
}

func TestMapsMerged(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_var_lookup.yml")

	theVal, err := cfg.getVar([]string{"dev", "TestMapsMerged", "myLabel", "A234"}, "compute.scaling")
	expected := map[string]interface{}{
		"msg-region":  "regionLevelMsg",
		"msg-label":   "labelLevelMsg",
		"msg-account": "accountLevelMsg",
		"msg":         "dev",
	}
	require.NoError(t, err)
	assert.Equal(t, expected, theVal, "Did not retrieve correct value")
}

func TestIsNumber(t *testing.T) {
	t.Parallel()

	cfg := readConfig(t, "test_var_lookup.yml")

	numberItem, err := cfg.getVar([]string{"dev", "TestMapsMerged", "myLabel", "A234"}, "numberItem")
	require.NoError(t, err)
	assert.Equal(t, 5.0, numberItem, "Expected a number")
}

func TestVarLookupListDoesntMergeButOverwrites(t *testing.T) {
	t.Parallel()

	expectedVarValue := []interface{}{"r-1", "r-2", "g-1", "g-2"}

	cfg := readConfig(t, "test_var_lookup.yml")
	listResult, err := cfg.getVar([]string{"dev", "R-ListTest", "myLabel", "A234"}, "listItem")

	require.NoError(t, err)
	assert.Equal(t, expectedVarValue, listResult)
}

func TestVarNotFound(t *testing.T) {
	t.Parallel()
	cfg := readConfig(t, "test_variable_does_not_exist_in_scope.yml")

	_, err := cfg.getVar([]string{"dev", "us-west-1", "myLabel", "A234"}, "compute.scalang.msg")
	require.EqualError(t, err, "variable not defined: \"compute.scalang.msg\", did you mean \"scaling\"")

	_, err = cfg.getVar([]string{"dev", "us-west-1", "myLabel", "A234"}, "compute.scaling.mso")
	require.EqualError(t, err, "variable not defined: \"compute.scaling.mso\", did you mean \"msg\"")
}
