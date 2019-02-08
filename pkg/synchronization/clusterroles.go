package synchronization

import (
	"fmt"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/composition"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/apis/creator"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ssam"
	"github.com/atlassian/voyager/pkg/util"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	crServiceModifyAccessFormat           = "paas:creator:service:%s:modify"
	crbServiceModifyAccessFormat          = "paas:creator:service:%s:modify"
	crServiceModifyTrebuchetAccessFormat  = "paas:trebuchet:service:%s:modify"
	crbServiceModifyTrebuchetAccessFormat = "paas:trebuchet:service:%s:modify"
	crServiceDescriptorAccessFormat       = "paas:composition:servicedescriptor:%s:crud"
	crbServiceDescriptorAccessFormat      = "paas:composition:servicedescriptor:%s:crud"

	customerLabelKey      = voyager.Domain + "/customer"
	customerLabelValue    = "paas"
	generatedByLabelKey   = voyager.Domain + "/generated_by"
	generatedByLabelValue = "paas-synchronization"

	trebuchetAPIGroup              = "trebuchet.atl-paas.net"
	trebuchetReleasesResource      = "releases"
	trebuchetReleaseGroupsResource = "release-groups"
)

func (c *Controller) createOrUpdateServiceDescriptorAccess(service *creator_v1.Service) error {
	c.Logger.Sugar().Infof("Creating ClusterRoles and ClusterRoleBindings for %q", service.Name)

	bamboo := service.Spec.Metadata.Bamboo
	var groups []string
	if bamboo != nil {
		groups = make([]string, 0, len(bamboo.Builds)+len(bamboo.Deployments))
		for _, b := range bamboo.Builds {
			groups = append(groups, toBuildGroup(b))
		}
		for _, b := range bamboo.Deployments {
			groups = append(groups, toDeploymentGroup(b))
		}
	}

	groups = append(groups, ssam.AccessLevelNameForEnvType(service.Spec.SSAMContainerName, c.ClusterLocation.EnvType))

	err := util.RetryObjectUpdater(c.Logger, c.ClusterRoleUpdater, crSpecServiceDescriptorAccess(service))
	if err != nil {
		return err
	}

	err = util.RetryObjectUpdater(c.Logger, c.ClusterRoleBindingUpdater, crbSpecServiceDescriptorAccess(service, groups))
	if err != nil {
		return err
	}

	err = util.RetryObjectUpdater(c.Logger, c.ClusterRoleUpdater, crSpecServiceModifyTrebuchetAccess(service))
	if err != nil {
		return err
	}

	err = util.RetryObjectUpdater(c.Logger, c.ClusterRoleBindingUpdater, crbSpecServiceModifyTrebuchetAccess(service, groups))
	if err != nil {
		return err
	}

	// Grant permissions to mutate services only in certain environments/clusters
	if c.AllowMutateServices {
		err = util.RetryObjectUpdater(c.Logger, c.ClusterRoleUpdater, crSpecServiceModifyAccess(service))
		if err != nil {
			return err
		}

		err = util.RetryObjectUpdater(c.Logger, c.ClusterRoleBindingUpdater, crbSpecServiceModifyAccess(service, service.Spec.ResourceOwner))
		if err != nil {
			return err
		}
	}

	return nil
}

func crSpecServiceDescriptorAccess(svc *creator_v1.Service) *rbac_v1.ClusterRole {
	cr := rbac_v1.ClusterRole{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: sdClusterRoleName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		Rules: []rbac_v1.PolicyRule{
			rbac_v1.PolicyRule{
				APIGroups:     []string{composition.GroupName},
				Resources:     []string{comp_v1.ServiceDescriptorResourcePlural},
				ResourceNames: []string{svc.GetName()},
				Verbs: []string{
					k8s.ServiceDescriptorClaimVerb, // custom "create"
					k8s.UpdateVerb,
					k8s.PatchVerb,
					k8s.DeleteVerb,
				},
			},
		},
	}

	return &cr
}

func crSpecServiceModifyTrebuchetAccess(svc *creator_v1.Service) *rbac_v1.ClusterRole {
	cr := rbac_v1.ClusterRole{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: trebuchetModifyClusterRoleName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		Rules: []rbac_v1.PolicyRule{
			rbac_v1.PolicyRule{
				APIGroups: []string{trebuchetAPIGroup},
				Resources: trebuchetResources(),
				Verbs:     []string{"*"},
			},
		},
	}

	return &cr
}

func crSpecServiceModifyAccess(svc *creator_v1.Service) *rbac_v1.ClusterRole {
	cr := rbac_v1.ClusterRole{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: svcModifyClusterRoleName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		Rules: []rbac_v1.PolicyRule{
			rbac_v1.PolicyRule{
				APIGroups:     []string{creator.GroupName},
				Resources:     []string{creator_v1.ServiceResourcePlural},
				ResourceNames: []string{svc.GetName()},
				Verbs: []string{
					k8s.UpdateVerb,
					k8s.PatchVerb,
					k8s.DeleteVerb,
				},
			},
		},
	}

	return &cr
}

func crbSpecServiceDescriptorAccess(svc *creator_v1.Service, groups []string) *rbac_v1.ClusterRoleBinding {
	crb := rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: sdClusterRoleBindingName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: sdClusterRoleName(svc.GetName()),
		},
		Subjects: make([]rbac_v1.Subject, 0, len(groups)),
	}

	for _, group := range groups {
		crb.Subjects = append(crb.Subjects, rbac_v1.Subject{
			APIGroup: rbac_v1.GroupName,
			Kind:     rbac_v1.GroupKind,
			Name:     group,
		})
	}

	return &crb
}

func crbSpecServiceModifyTrebuchetAccess(svc *creator_v1.Service, groups []string) *rbac_v1.ClusterRoleBinding {
	crb := rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: trebuchetModifyClusterRoleBindingName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: trebuchetModifyClusterRoleName(svc.GetName()),
		},
		Subjects: make([]rbac_v1.Subject, 0, len(groups)),
	}

	for _, group := range groups {
		crb.Subjects = append(crb.Subjects, rbac_v1.Subject{
			APIGroup: rbac_v1.GroupName,
			Kind:     rbac_v1.GroupKind,
			Name:     group,
		})
	}

	return &crb
}

func crbSpecServiceModifyAccess(svc *creator_v1.Service, owner string) *rbac_v1.ClusterRoleBinding {
	crb := rbac_v1.ClusterRoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: rbac_v1.SchemeGroupVersion.String(),
			Kind:       k8s.ClusterRoleBindingKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: svcModifyClusterRoleBindingName(svc.GetName()),
			Labels: map[string]string{
				customerLabelKey:    customerLabelValue,
				generatedByLabelKey: generatedByLabelValue,
			},
		},
		RoleRef: rbac_v1.RoleRef{
			Kind: k8s.ClusterRoleKind,
			Name: svcModifyClusterRoleName(svc.GetName()),
		},
		Subjects: []rbac_v1.Subject{
			{
				APIGroup: rbac_v1.GroupName,
				Kind:     rbac_v1.UserKind,
				Name:     owner,
			},
		},
	}

	return &crb
}

func sdClusterRoleName(sdName string) string {
	return fmt.Sprintf(crServiceDescriptorAccessFormat, sdName)
}

func trebuchetModifyClusterRoleName(svcName string) string {
	return fmt.Sprintf(crServiceModifyTrebuchetAccessFormat, svcName)
}

func svcModifyClusterRoleName(svcName string) string {
	return fmt.Sprintf(crServiceModifyAccessFormat, svcName)
}

func sdClusterRoleBindingName(sdName string) string {
	return fmt.Sprintf(crbServiceDescriptorAccessFormat, sdName)
}

func trebuchetModifyClusterRoleBindingName(svcName string) string {
	return fmt.Sprintf(crbServiceModifyTrebuchetAccessFormat, svcName)
}

func svcModifyClusterRoleBindingName(svcName string) string {
	return fmt.Sprintf(crbServiceModifyAccessFormat, svcName)
}

func trebuchetResources() []string {
	return []string{trebuchetReleasesResource, trebuchetReleaseGroupsResource}
}
