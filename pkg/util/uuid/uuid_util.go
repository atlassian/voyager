package uuid

import (
	"k8s.io/apimachinery/pkg/util/uuid"
)

var defaultUUIDGenerator = &uuidGeneratorImpl{}

type Generator interface {
	NewUUID() string
}

type uuidGeneratorImpl struct {
}

func (g *uuidGeneratorImpl) NewUUID() string {
	return string(uuid.NewUUID())
}

func Default() Generator {
	return defaultUUIDGenerator
}
