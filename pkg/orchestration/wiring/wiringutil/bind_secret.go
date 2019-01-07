package wiringutil

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"k8s.io/apimachinery/pkg/runtime"
)

// BindingSecretReference represents bits of information that need to be augmented with more information to
// construct a valid Smith reference to bind secret.
// +k8s:deepcopy-gen=true
type BindSecretReference struct {
	ProducerResource voyager.ResourceName `json:"producerResource"`
	Path             string               `json:"path,omitempty"`
	Example          interface{}          `json:"example,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (r *BindSecretReference) DeepCopyInto(out *BindSecretReference) {
	*out = *r
	out.Example = runtime.DeepCopyJSONValue(r.Example)
}

func (r *BindSecretReference) ToReference(name smith_v1.ReferenceName, consumerResource voyager.ResourceName) smith_v1.Reference {
	bindingResource := ConsumerProducerResourceNameWithPostfix(consumerResource, r.ProducerResource, "binding")
	return smith_v1.Reference{
		Name:     name,
		Resource: bindingResource,
		Path:     r.Path,
		Example:  r.Example,
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
}
