package iam

import (
	"testing"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNames(t *testing.T) {
	t.Parallel()

	var computeName voyager.ResourceName = "compute"

	iamInst, err := PluginServiceInstance(EC2ComputeType, computeName, "", false, nil, &wiringplugin.WiringContext{}, []string{}, []string{})
	require.NoError(t, err)

	iamBinding := ServiceBinding(computeName, iamInst.SmithResource.Name)

	var someProducerName voyager.ResourceName = "iamrole"
	potentiallyConflictingBinding := wiringutil.ConsumerProducerServiceBinding(computeName, someProducerName, "instance1")

	assert.NotEqual(t, potentiallyConflictingBinding.SmithResource.Name, iamBinding.SmithResource.Name)
	assert.NotEqual(t, potentiallyConflictingBinding.SmithResource.Spec.Object.(meta_v1.Object).GetName(),
		iamBinding.SmithResource.Spec.Object.(meta_v1.Object).GetName())
}
