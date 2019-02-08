package synchronization

import (
	"testing"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	k8s_testing "github.com/atlassian/voyager/pkg/k8s/testing"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	client_testing "k8s.io/client-go/testing"
	kube_testing "k8s.io/client-go/testing"
)

const (
	firstServiceNameStr string = "first-service" // explicitly string typed constant
	firstServiceName           = voyager.ServiceName(firstServiceNameStr)
	firstServiceNameSc         = servicecentral.ServiceName(firstServiceNameStr)

	secondServiceNameStr string = "second-service" // explicitly string typed constant
	secondServiceName           = voyager.ServiceName(secondServiceNameStr)
	secondServiceNameSc         = servicecentral.ServiceName(secondServiceNameStr)
)

func TestIncrementsCounterWhenAccessUpdateErrors(t *testing.T) {
	t.Parallel()

	sourceService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "new-ssam-container",
		},
	}

	clusterRoleBinding := &rbac_v1.ClusterRoleBinding{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:composition:servicedescriptor:first-service:crud",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{clusterRoleBinding, existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.mainFake.PrependReactor("update", "*", func(action client_testing.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("failed doing things")
			})

			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&sourceService, nil)

			cntrlr.syncServiceMetadata()

			ctr, err := tc.findCounter(fakeUpdateErrorCounter)
			require.NoError(t, err)
			require.NotNil(t, ctr)

			assert.Equal(t, float64(1), ctr.GetValue())
		},
	}

	tc.run(t)
}

func TestCreatesClusterRole(t *testing.T) {
	t.Parallel()

	sourceService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "new-ssam-container",
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&sourceService, nil)

			cntrlr.syncServiceMetadata()

			crs := findCreatedClusterRoles(tc.mainFake.Actions())
			require.Len(t, crs, 3)

			assert.Equal(t, "paas:composition:servicedescriptor:first-service:crud", crs[0].GetName())
			assert.Equal(t, "composition.voyager.atl-paas.net", crs[0].Rules[0].APIGroups[0])
			assert.Equal(t, "servicedescriptors", crs[0].Rules[0].Resources[0])
			assert.EqualValues(t, firstServiceName, crs[0].Rules[0].ResourceNames[0])
			assert.Equal(t, []string{"claim", "update", "patch", "delete"}, crs[0].Rules[0].Verbs)

			assert.Equal(t, "paas:trebuchet:service:first-service:modify", crs[1].GetName())
			assert.Equal(t, "trebuchet.atl-paas.net", crs[1].Rules[0].APIGroups[0])
			assert.Equal(t, "releases", crs[1].Rules[0].Resources[0])
			assert.Equal(t, "release-groups", crs[1].Rules[0].Resources[1])
			assert.Equal(t, []string{"*"}, crs[1].Rules[0].Verbs)

			assert.Equal(t, "paas:creator:service:first-service:modify", crs[2].GetName())
			assert.Equal(t, "creator.voyager.atl-paas.net", crs[2].Rules[0].APIGroups[0])
			assert.Equal(t, "services", crs[2].Rules[0].Resources[0])
		},
	}

	tc.run(t)
}

func TestCreatesMultipleClusterRoles(t *testing.T) {
	t.Parallel()

	sourceService1 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "second-ssam-container",
		},
	}
	sourceService2 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: secondServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "second-ssam-container",
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService1, sourceService2,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&sourceService1, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), secondServiceNameSc).Return(&sourceService2, nil)

			cntrlr.syncServiceMetadata()

			crs := findCreatedClusterRoles(tc.mainFake.Actions())
			require.Len(t, crs, 6)

			// These can appear in any order
			names := sets.NewString()
			for _, cr := range crs {
				names.Insert(cr.GetName())
			}
			assert.True(t, names.HasAll(
				"paas:composition:servicedescriptor:first-service:crud",
				"paas:composition:servicedescriptor:second-service:crud",
				"paas:trebuchet:service:first-service:modify",
				"paas:trebuchet:service:second-service:modify",
				"paas:creator:service:first-service:modify",
				"paas:creator:service:second-service:modify",
			))
		},
	}

	tc.run(t)
}

func TestDoesntCreateServiceClusterRole(t *testing.T) {
	t.Parallel()

	sourceService1 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "second-ssam-container",
		},
	}
	sourceService2 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: secondServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "second-ssam-container",
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			// Reset the flag to prevent creation of service role
			cntrlr.AllowMutateServices = false

			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService1, sourceService2,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&sourceService1, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), secondServiceNameSc).Return(&sourceService2, nil)

			cntrlr.syncServiceMetadata()

			crs := findCreatedClusterRoles(tc.mainFake.Actions())
			require.Len(t, crs, 4)

			// These can appear in any order
			names := sets.NewString()
			for _, cr := range crs {
				names.Insert(cr.GetName())
			}
			// "paas:creator:service:{serviceName}:modify" must be missing
			assert.True(t, names.HasAll(
				"paas:composition:servicedescriptor:first-service:crud",
				"paas:composition:servicedescriptor:second-service:crud",
				"paas:trebuchet:service:first-service:modify",
				"paas:trebuchet:service:second-service:modify",
			))
		},
	}

	tc.run(t)
}

func TestCreatesClusterRoleBinding(t *testing.T) {
	t.Parallel()

	sourceService1 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "first-ssam-container",
		},
	}
	sourceService2 := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: secondServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "second-ssam-container",
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService1, sourceService2,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&sourceService1, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), secondServiceNameSc).Return(&sourceService2, nil)

			cntrlr.syncServiceMetadata()

			crbs := findCreatedClusterRoleBindings(tc.mainFake.Actions())
			require.Len(t, crbs, 6)

			// These can appear in any order
			names := sets.NewString()
			for _, crb := range crbs {
				names.Insert(crb.GetName())
			}
			assert.True(t, names.HasAll(
				"paas:composition:servicedescriptor:first-service:crud",
				"paas:composition:servicedescriptor:second-service:crud",
				"paas:trebuchet:service:first-service:modify",
				"paas:trebuchet:service:second-service:modify",
				"paas:creator:service:first-service:modify",
				"paas:creator:service:second-service:modify",
			))
		},
	}

	tc.run(t)
}

func TestBuildsInClusterRoleBinding(t *testing.T) {
	t.Parallel()

	sourceService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "first-ssam-container",
		},
	}
	fullService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "first-ssam-container",
			Metadata: creator_v1.ServiceMetadata{
				Bamboo: &creator_v1.BambooMetadata{
					Builds: []creator_v1.BambooPlanRef{
						{Server: "one", Plan: "two"},
						{Server: "three", Plan: "four"},
					},
					Deployments: []creator_v1.BambooPlanRef{
						{Server: "five", Plan: "six"},
					},
				},
				PagerDuty: &creator_v1.PagerDutyMetadata{},
			},
		},
	}
	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&fullService, nil)

			cntrlr.syncServiceMetadata()

			crbs := findCreatedClusterRoleBindings(tc.mainFake.Actions())
			require.Len(t, crbs, 3) // 1: sds, 2: trebuchet, 3: services

			assert.Equal(t, "Group", crbs[0].Subjects[0].Kind)
			assert.Equal(t, "builds:bambooBuild:one:two", crbs[0].Subjects[0].Name)
			assert.Equal(t, "Group", crbs[0].Subjects[1].Kind)
			assert.Equal(t, "builds:bambooBuild:three:four", crbs[0].Subjects[1].Name)
			assert.Equal(t, "Group", crbs[0].Subjects[2].Kind)
			assert.Equal(t, "builds:bambooDeployment:five:six", crbs[0].Subjects[2].Name)
			assert.Equal(t, "Group", crbs[0].Subjects[3].Kind)
			assert.Equal(t, "first-ssam-container-dl-dev", crbs[0].Subjects[3].Name)
		},
	}

	tc.run(t)
}

func TestNoUpdatesToClusterRoleBindingWhenNoChanges(t *testing.T) {
	t.Parallel()

	sourceService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "first-ssam-container",
			ResourceOwner:     "an_owner",
		},
	}
	fullService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "first-ssam-container",
			Metadata: creator_v1.ServiceMetadata{
				Bamboo: &creator_v1.BambooMetadata{
					Builds: []creator_v1.BambooPlanRef{
						{Server: "one", Plan: "two"},
						{Server: "three", Plan: "four"},
					},
					Deployments: []creator_v1.BambooPlanRef{
						{Server: "five", Plan: "six"},
					},
				},
				PagerDuty: &creator_v1.PagerDutyMetadata{},
			},
			ResourceOwner: "an_owner",
		},
	}
	sdCrb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:composition:servicedescriptor:first-service:crud",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:composition:servicedescriptor:first-service:crud",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:one:two",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:three:four",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooDeployment:five:six",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "first-ssam-container-dl-dev",
			},
		},
	}
	trebuchetCrb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:trebuchet:service:first-service:modify",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:trebuchet:service:first-service:modify",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:one:two",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:three:four",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooDeployment:five:six",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "first-ssam-container-dl-dev",
			},
		},
	}
	svcCrb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:creator:service:first-service:modify",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:creator:service:first-service:modify",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "an_owner",
			},
		},
	}
	tc := testCase{
		mainClientObjects: []runtime.Object{sdCrb, trebuchetCrb, svcCrb, existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&fullService, nil)

			cntrlr.syncServiceMetadata()

			crbs := findCreatedClusterRoleBindings(tc.mainFake.Actions())
			assert.Len(t, crbs, 0)
			crbs = findUpdatedClusterRoleBindings(tc.mainFake.Actions())
			assert.Len(t, crbs, 0)
		},
	}

	tc.run(t)
}

func TestUpdateClusterRoleBindingWhenChangesToSSAMContainer(t *testing.T) {
	t.Parallel()

	sourceService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "another-ssam-container",
		},
	}
	fullService := creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: firstServiceNameStr,
		},
		Spec: creator_v1.ServiceSpec{
			SSAMContainerName: "another-ssam-container",
			Metadata: creator_v1.ServiceMetadata{
				PagerDuty: &creator_v1.PagerDutyMetadata{},
			},
			ResourceOwner: "an_owner",
		},
	}
	crb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:composition:servicedescriptor:first-service:crud",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:composition:servicedescriptor:first-service:crud",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:one:two",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:three:four",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooDeployment:five:six",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "first-ssam-container-dl-dev",
			},
		},
	}
	trebuchetCrb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:trebuchet:service:first-service:modify",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:trebuchet:service:first-service:modify",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:one:two",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooBuild:three:four",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "builds:bambooDeployment:five:six",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "first-ssam-container-dl-dev",
			},
		},
	}
	svcCrb := &rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "paas:creator:service:first-service:modify",
			Labels: map[string]string{
				"voyager.atl-paas.net/customer":     "paas",
				"voyager.atl-paas.net/generated_by": "paas-synchronization",
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: "paas:creator:service:first-service:modify",
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "an_owner",
			},
		},
	}
	tc := testCase{
		mainClientObjects: []runtime.Object{crb, trebuchetCrb, svcCrb, existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{
				sourceService,
			}, nil)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), firstServiceNameSc).Return(&fullService, nil)

			cntrlr.syncServiceMetadata()

			crbs := findCreatedClusterRoleBindings(tc.mainFake.Actions())
			assert.Len(t, crbs, 0)
			crbs = findUpdatedClusterRoleBindings(tc.mainFake.Actions())
			assert.Len(t, crbs, 2)

			assert.Equal(t, "Group", crbs[0].Subjects[0].Kind)
			assert.Equal(t, "another-ssam-container-dl-dev", crbs[0].Subjects[0].Name)

			assert.Equal(t, "Group", crbs[1].Subjects[0].Kind)
			assert.Equal(t, "another-ssam-container-dl-dev", crbs[0].Subjects[0].Name)
		},
	}

	tc.run(t)
}

func findCreatedClusterRoles(actions []kube_testing.Action) []*rbac_v1.ClusterRole {
	var crs []*rbac_v1.ClusterRole
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.ClusterRole); ok {
			crs = append(crs, r)
		}
	}
	return crs
}

func findCreatedClusterRoleBindings(actions []kube_testing.Action) []*rbac_v1.ClusterRoleBinding {
	var rbs []*rbac_v1.ClusterRoleBinding
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.ClusterRoleBinding); ok {
			rbs = append(rbs, r)
		}
	}
	return rbs
}

func findUpdatedClusterRoles(actions []kube_testing.Action) []*rbac_v1.ClusterRole {
	var crs []*rbac_v1.ClusterRole
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.ClusterRole); ok {
			crs = append(crs, r)
		}
	}
	return crs
}

func findUpdatedClusterRoleBindings(actions []kube_testing.Action) []*rbac_v1.ClusterRoleBinding {
	var rbs []*rbac_v1.ClusterRoleBinding
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.ClusterRoleBinding); ok {
			rbs = append(rbs, r)
		}
	}
	return rbs
}
