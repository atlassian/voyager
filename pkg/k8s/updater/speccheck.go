package updater

import (
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type SpecCheck interface {
	CompareActualVsSpec(logger *zap.Logger, spec, actual runtime.Object) (*unstructured.Unstructured, bool /*match*/, string /* diff */, error)
}
