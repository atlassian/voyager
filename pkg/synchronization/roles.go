package synchronization

import (
	"fmt"

	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ssam"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	buildGroupPrefix = "builds"
	bambooBuild      = "bambooBuild"
	bambooDeployment = "bambooDeployment"

	bambooBuildsRoleBinding      = "bamboo:builds"
	bambooDeploymentsRoleBinding = "bamboo:deployments"
	staffViewRoleBinding         = "staff:view"
	teamCrudRoleBinding          = "team:crud"

	namespaceClusterRole = "paas:service-namespace" // see ansible-goliath
	authenticatedUsers   = "system:authenticated"
	serviceAccounts      = "system:serviceaccounts"
	viewRole             = "view"
)

func (c *Controller) syncPermissions(logger *zap.Logger, ns *core_v1.Namespace, serviceData *creator_v1.Service) (bool /* conflict */, bool /* retriable */, error) {
	conflict, retriable, _, err := c.createOrUpdateRoleBinding(logger, rbSpecBambooBuilds(ns, serviceData))
	if err != nil || conflict {
		return conflict, retriable, err
	}

	conflict, retriable, _, err = c.createOrUpdateRoleBinding(logger, rbSpecBambooDeployments(ns, serviceData))
	if err != nil || conflict {
		return conflict, retriable, err
	}

	conflict, retriable, _, err = c.createOrUpdateRoleBinding(logger, rbSpecTeamCRUD(ns, serviceData, c.ClusterLocation))
	if err != nil || conflict {
		return conflict, retriable, err
	}

	conflict, retriable, _, err = c.createOrUpdateRoleBinding(logger, rbSpecStaffViewResources(ns, serviceData))
	return conflict, retriable, err
}

func (c *Controller) createOrUpdateRoleBinding(logger *zap.Logger, rbSpec *rbac_v1.RoleBinding) (bool /* conflict */, bool /* retriable */, *rbac_v1.RoleBinding, error) {
	logger.Sugar().Debugf("Attempting to create or update RoleBinding %q", rbSpec.Name)

	conflict, retriable, obj, err := c.RoleBindingUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			return nil
		},
		rbSpec,
	)

	var rb *rbac_v1.RoleBinding
	if obj != nil {
		rb = obj.(*rbac_v1.RoleBinding)
	}
	return conflict, retriable, rb, err
}

// rbSpecTeamCRUD grants the service's team the permissions within the Namespace for the given EnvType.
// We don't currently want to expose all resource types, out of safety concerns.
func rbSpecTeamCRUD(ns *core_v1.Namespace, svc *creator_v1.Service, location voyager.ClusterLocation) *rbac_v1.RoleBinding {
	rbName := teamCrudRoleBinding
	return &rbac_v1.RoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       k8s.RoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      rbName,
			Namespace: ns.Name,
		},
		Subjects: []rbac_v1.Subject{
			{
				Kind: rbac_v1.GroupKind,
				Name: ssam.AccessLevelNameForEnvType(svc.Spec.SSAMContainerName, location.EnvType),
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: namespaceClusterRole,
		},
	}
}

// rbSpecStaffViewResources produces a RoleBinding to give staff read-only
// access to almost all resources within the Namespace.
func rbSpecStaffViewResources(ns *core_v1.Namespace, svc *creator_v1.Service) *rbac_v1.RoleBinding {
	rbName := staffViewRoleBinding
	return &rbac_v1.RoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       k8s.RoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      rbName,
			Namespace: ns.Name,
		},
		Subjects: []rbac_v1.Subject{
			{
				// https://kubernetes.io/docs/admin/authorization/rbac/#referring-to-subjects
				Kind: rbac_v1.GroupKind,
				Name: authenticatedUsers,
			},
			{
				// so bot users can view namespaces
				Kind: rbac_v1.GroupKind,
				Name: serviceAccounts,
			},
		},
		RoleRef: rbac_v1.RoleRef{
			// https://kubernetes.io/docs/admin/authorization/rbac/#default-roles-and-role-bindings
			Kind: k8s.ClusterRoleKind,
			Name: viewRole,
		},
	}
}

func rbSpecBambooBuilds(ns *core_v1.Namespace, svc *creator_v1.Service) *rbac_v1.RoleBinding {
	rb := rbac_v1.RoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       k8s.RoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      bambooBuildsRoleBinding,
			Namespace: ns.Name,
		},
		Subjects: []rbac_v1.Subject{},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: namespaceClusterRole,
		},
	}

	bamboo := svc.Spec.Metadata.Bamboo

	// Instead of deleting the RoleBinding, this simplifies the update logic
	// by avoiding handling the delete case - and instead just clearing the
	// list of subjects in the RoleBinding. For consistency this also happens
	// in the create case.
	if bamboo == nil || len(bamboo.Builds) == 0 {
		return &rb
	}

	for _, planRef := range bamboo.Builds {
		rb.Subjects = append(rb.Subjects, rbac_v1.Subject{
			Kind: rbac_v1.GroupKind,
			Name: toBuildGroup(planRef),
		})
	}

	return &rb
}

func rbSpecBambooDeployments(ns *core_v1.Namespace, svc *creator_v1.Service) *rbac_v1.RoleBinding {
	rb := rbac_v1.RoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       k8s.RoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      bambooDeploymentsRoleBinding,
			Namespace: ns.Name,
		},
		Subjects: []rbac_v1.Subject{},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: namespaceClusterRole,
		},
	}

	bamboo := svc.Spec.Metadata.Bamboo
	if bamboo == nil || len(bamboo.Deployments) == 0 {
		return &rb
	}

	for _, planRef := range bamboo.Deployments {
		rb.Subjects = append(rb.Subjects, rbac_v1.Subject{
			Kind: rbac_v1.GroupKind,
			Name: toDeploymentGroup(planRef),
		})
	}

	return &rb
}

func toDeploymentGroup(planRef creator_v1.BambooPlanRef) string {
	return fmt.Sprintf("%s:%s:%s:%s", buildGroupPrefix, bambooDeployment, planRef.Server, planRef.Plan)
}

func toBuildGroup(planRef creator_v1.BambooPlanRef) string {
	return fmt.Sprintf("%s:%s:%s:%s", buildGroupPrefix, bambooBuild, planRef.Server, planRef.Plan)
}
