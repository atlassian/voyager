package synchronization

import (
	"testing"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	k8s_testing "github.com/atlassian/voyager/pkg/k8s/testing"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube_testing "k8s.io/client-go/testing"
)

func TestCreatesRoleBindingsFromServiceCentralData(t *testing.T) {
	t.Parallel()

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
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
					SSAMContainerName: "ssam-container-name-blah",
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceNameSc).Return(service, nil)

			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external)
			assert.False(t, retriable)

			actions := tc.mainFake.Actions()

			roleBindings := findCreatedRoleBindings(actions)
			require.Len(t, roleBindings, 4)

			assert.Equal(t, bambooBuildsRoleBinding, roleBindings[0].Name)
			assert.Equal(t, bambooDeploymentsRoleBinding, roleBindings[1].Name)
			assert.Equal(t, teamCrudRoleBinding, roleBindings[2].Name)
			assert.Equal(t, staffViewRoleBinding, roleBindings[3].Name)
			require.Len(t, roleBindings[0].Subjects, 2)
			assert.Equal(t, "builds:bambooBuild:one:two", roleBindings[0].Subjects[0].Name)
			assert.Equal(t, "builds:bambooBuild:three:four", roleBindings[0].Subjects[1].Name)
			require.Len(t, roleBindings[1].Subjects, 1)
			assert.Equal(t, "builds:bambooDeployment:five:six", roleBindings[1].Subjects[0].Name)
			require.Len(t, roleBindings[2].Subjects, 1)
			assert.Equal(t, "ssam-container-name-blah-dl-dev", roleBindings[2].Subjects[0].Name)

			// No point testing the staffView subjects as these aren't dynamic
		},
	}

	tc.run(t)
}

func TestSkipsUpdateOfRoleBindingsIfNoChange(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		&rbac_v1.RoleBinding{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      bambooBuildsRoleBinding,
				Namespace: namespaceName,
			},
			RoleRef: rbac_v1.RoleRef{
				Kind: k8s.ClusterRoleKind,
				Name: namespaceClusterRole,
			},
			Subjects: []rbac_v1.Subject{
				{Kind: "Group", Name: "builds:bambooBuild:one:two"},
				{Kind: "Group", Name: "builds:bambooBuild:three:four"},
			},
		},
		&rbac_v1.RoleBinding{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      bambooDeploymentsRoleBinding,
				Namespace: namespaceName,
			},
			RoleRef: rbac_v1.RoleRef{
				Kind: k8s.ClusterRoleKind,
				Name: namespaceClusterRole,
			},
			Subjects: []rbac_v1.Subject{
				{Kind: "Group", Name: "builds:bambooDeployment:five:six"},
			},
		},
		&rbac_v1.RoleBinding{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      staffViewRoleBinding,
				Namespace: namespaceName,
			},
			RoleRef: rbac_v1.RoleRef{
				Kind: k8s.ClusterRoleKind,
				Name: viewRole,
			},
			Subjects: []rbac_v1.Subject{
				{Kind: "Group", Name: authenticatedUsers},
				{Kind: "Group", Name: serviceAccounts},
			},
		},
		&rbac_v1.RoleBinding{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      teamCrudRoleBinding,
				Namespace: namespaceName,
			},
			RoleRef: rbac_v1.RoleRef{
				Kind: k8s.ClusterRoleKind,
				Name: namespaceClusterRole,
			},
			Subjects: []rbac_v1.Subject{
				{Kind: "Group", Name: "ssam-container-name-blah-dl-dev"},
			},
		},
		existingDefaultDockerSecret(),
	}

	tc := testCase{
		mainClientObjects: objs,
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
						Bamboo: &creator_v1.BambooMetadata{
							Builds: []creator_v1.BambooPlanRef{
								{Server: "one", Plan: "two"},
								{Server: "three", Plan: "four"},
							},
							Deployments: []creator_v1.BambooPlanRef{
								{Server: "five", Plan: "six"},
							},
						},
					},
					SSAMContainerName: "ssam-container-name-blah",
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceNameSc).Return(service, nil)

			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external)
			assert.False(t, retriable)

			actions := tc.mainFake.Actions()

			roleBindings := findCreatedRoleBindings(actions)
			assert.Len(t, roleBindings, 0)

			roleBindings = findUpdatedRoleBindings(actions)
			assert.Len(t, roleBindings, 0)
		},
	}

	tc.run(t)
}

func TestEmptyBuildRoleBindingsWhenListsEmpty(t *testing.T) {
	t.Parallel()

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
						Bamboo: &creator_v1.BambooMetadata{
							Builds:      []creator_v1.BambooPlanRef{},
							Deployments: []creator_v1.BambooPlanRef{},
						},
					},
					SSAMContainerName: "ssam-container-name-blah",
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceNameSc).Return(service, nil)

			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external)
			assert.False(t, retriable)

			actions := tc.mainFake.Actions()

			roleBindings := findCreatedRoleBindings(actions)
			require.Len(t, roleBindings, 4)

			assert.Equal(t, bambooBuildsRoleBinding, roleBindings[0].Name)
			assert.Equal(t, bambooDeploymentsRoleBinding, roleBindings[1].Name)
			assert.Len(t, roleBindings[0].Subjects, 0)
			assert.Len(t, roleBindings[1].Subjects, 0)

			// We don't check the other role bindings we create
		},
	}

	tc.run(t)
}

func TestEmptyRoleBindingsNonExistantBuilds(t *testing.T) {
	t.Parallel()

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret()},
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "ssam-container-name-blah",
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceNameSc).Return(service, nil)

			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external)
			assert.False(t, retriable)

			actions := tc.mainFake.Actions()

			roleBindings := findCreatedRoleBindings(actions)
			require.Len(t, roleBindings, 4)

			assert.Equal(t, bambooBuildsRoleBinding, roleBindings[0].Name)
			assert.Equal(t, bambooDeploymentsRoleBinding, roleBindings[1].Name)
			assert.Len(t, roleBindings[0].Subjects, 0)
			assert.Len(t, roleBindings[1].Subjects, 0)

			// We don't check the other role bindings we create
		},
	}

	tc.run(t)
}

func findCreatedRoleBindings(actions []kube_testing.Action) []*rbac_v1.RoleBinding {
	var rbs []*rbac_v1.RoleBinding
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.RoleBinding); ok {
			rbs = append(rbs, r)
		}
	}
	return rbs
}

func findUpdatedRoleBindings(actions []kube_testing.Action) []*rbac_v1.RoleBinding {
	var rbs []*rbac_v1.RoleBinding
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if r, ok := action.GetObject().(*rbac_v1.RoleBinding); ok {
			rbs = append(rbs, r)
		}
	}
	return rbs
}
