package iam

import (
	"testing"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNames(t *testing.T) {
	t.Parallel()

	var computeName voyager.ResourceName = "compute"

	iamInst, err := PluginServiceInstance(EC2ComputeType, computeName, "", false, nil, &wiringplugin.WiringContext{}, []string{}, []string{}, &oap.VPCEnvironment{})
	require.NoError(t, err)

	iamBinding := ServiceBinding(computeName, iamInst.Name)

	var someProducerName voyager.ResourceName = "iamrole"
	protoReference := libshapes.ProtoReference{
		Resource: "instance1",
	}
	potentiallyConflictingBinding := wiringutil.ConsumerProducerServiceBinding(computeName, someProducerName, protoReference.ToReference())

	assert.NotEqual(t, potentiallyConflictingBinding.Name, iamBinding.Name)
	assert.NotEqual(t, potentiallyConflictingBinding.Spec.Object.(meta_v1.Object).GetName(),
		iamBinding.Spec.Object.(meta_v1.Object).GetName())
}
