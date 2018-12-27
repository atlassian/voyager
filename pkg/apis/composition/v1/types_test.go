package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &ServiceDescriptorList{}
var _ meta_v1.ListMetaAccessor = &ServiceDescriptorList{}

var _ runtime.Object = &ServiceDescriptor{}
var _ meta_v1.ObjectMetaAccessor = &ServiceDescriptor{}

func TestDeepCopiedResourcesAreDeepEqual(t *testing.T) {
	t.Parallel()

	asJSON := `{ "name": "foo", "type": "sns", "dependsOn": ["just-name", { "name": "with-atts", "attributes": { "foo": "bar"}}]}`
	var orig ServiceDescriptorResource
	err := json.Unmarshal([]byte(asJSON), &orig)

	require.NoError(t, err, "Error unmarshalling from JSON")

	copy := *orig.DeepCopy()

	assert.Equal(t, orig, copy)
}
