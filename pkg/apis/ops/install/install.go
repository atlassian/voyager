package install

import (
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// Install registers the API group and adds types to a scheme
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(ops_v1.AddToScheme(scheme))
}
