package wiringutil

import (
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
)

// Merge two maps, only use loser's fields if those fields are missing in winner
func Merge(winner, loser map[string]interface{}) (map[string]interface{}, error) {
	if len(loser) == 0 {
		return winner, nil
	}

	if len(winner) == 0 {
		return loser, nil
	}

	dst := make(map[string]interface{})

	if err := mergo.Merge(&dst, winner); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := mergo.Merge(&dst, loser); err != nil {
		return nil, errors.WithStack(err)
	}

	return dst, nil
}

// StripJSONFields remove fields from an obj that are in badStruct.
// It is intended to be used after processing certain fields from a struct
// but the rest should be passed through (i.e. to the ServiceInstance object).
func StripJSONFields(obj map[string]interface{}, badStruct interface{}) {
	badStructType := reflect.ValueOf(badStruct).Type()
	for i := 0; i < badStructType.NumField(); i++ {
		jsonAnnotations := strings.Split(badStructType.Field(i).Tag.Get("json"), ",")
		if len(jsonAnnotations) > 0 && jsonAnnotations[0] != "" {
			delete(obj, jsonAnnotations[0])
		}
	}
}

// Return wiring result with a single object
func SingleWiringResult(name smith_v1.ResourceName, obj runtime.Object, shapes []wiringplugin.Shape, references []smith_v1.Reference) *wiringplugin.WiringResultSuccess {
	smithResource := smith_v1.Resource{
		Name:       name,
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: obj,
		},
	}

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: shapes,
		},
		Resources: []smith_v1.Resource{smithResource},
	}
}
