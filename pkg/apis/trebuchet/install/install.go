package install

import (
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// Install registers the API group and adds types to a scheme
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(trebuchet_v1.AddToScheme(scheme))
}
