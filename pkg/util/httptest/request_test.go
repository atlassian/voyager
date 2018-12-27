package httptest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonToType(t *testing.T) {
	t.Parallel()

	type JSONTest struct {
		A string
	}

	testCases := []struct {
		name  string
		input interface{}
		typ   reflect.Type
		err   string
	}{
		// jsonMarshalling does returns floats by default, if we used an int this case would fail
		{"map", map[string]interface{}{"a": float64(1)}, reflect.TypeOf(map[string]interface{}{}), "input type must be a pointer, got map[string]interface {}"},
		{"map", &map[string]interface{}{"a": float64(1)}, reflect.TypeOf(&map[string]interface{}{}), ""},
		{"struct", JSONTest{A: "test"}, reflect.TypeOf(JSONTest{}), "input type must be a pointer, got httptest.JSONTest"},
		{"pointer", &JSONTest{A: "test"}, reflect.TypeOf(&JSONTest{}), ""},
	}

	for i := range testCases {
		t.Run(testCases[i].name, func(t *testing.T) {
			body, err := json.Marshal(testCases[i].input)
			require.NoError(t, err)
			result, err := unmarshalToType(body, testCases[i].typ)
			if testCases[i].err != "" {
				require.Error(t, err)
				require.Equal(t, err.Error(), testCases[i].err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCases[i].input, result)
			}
		})
	}
}
