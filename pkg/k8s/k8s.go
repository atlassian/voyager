package k8s

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// these were not found in upstream/vendor packages
	ClusterRoleKind             = "ClusterRole"
	ClusterRoleBindingKind      = "ClusterRoleBinding"
	ConfigMapKind               = "ConfigMap"
	DeploymentKind              = "Deployment"
	NamespaceKind               = "Namespace"
	PodKind                     = "Pod"
	PodDisruptionBudgetKind     = "PodDisruptionBudget"
	ReplicaSetKind              = "ReplicaSet"
	RoleKind                    = "Role"
	RoleBindingKind             = "RoleBinding"
	CronJobKind                 = "CronJob"
	SecretKind                  = "Secret"
	ServiceKind                 = "Service"
	IngressKind                 = "Ingress"
	HorizontalPodAutoscalerKind = "HorizontalPodAutoscaler"
	ServiceInstanceKind         = "ServiceInstance"
	ServiceAccountKind          = "ServiceAccount"
	EventKind                   = "Event"
	ServiceInstanceResource     = "serviceinstances"
	ServiceBindingResource      = "servicebindings"
	IngressResource             = "ingresses"

	CreateVerb           = "create"
	GetVerb              = "get"
	ListVerb             = "list"
	UpdateVerb           = "update"
	PatchVerb            = "patch"
	DeleteVerb           = "delete"
	DeleteCollectionVerb = "deletecollection"
	WatchVerb            = "watch"

	ServiceDescriptorClaimVerb = "claim"

	// Beta labels, from:
	// https://github.com/kubernetes/kubernetes/blob/v1.12.2/pkg/kubelet/apis/well_known_labels.go
	LabelHostname           = "kubernetes.io/hostname"
	LabelZoneFailureDomain  = "failure-domain.beta.kubernetes.io/zone"
	LabelMultiZoneDelimiter = "__"
	LabelZoneRegion         = "failure-domain.beta.kubernetes.io/region"

	LabelInstanceType = "beta.kubernetes.io/instance-type"

	LabelOS   = "beta.kubernetes.io/os"
	LabelArch = "beta.kubernetes.io/arch"
)

var (
	DeploymentGVK              = apps_v1.SchemeGroupVersion.WithKind(DeploymentKind)
	PodGVK                     = core_v1.SchemeGroupVersion.WithKind(PodKind)
	ReplicaSetGVK              = apps_v1.SchemeGroupVersion.WithKind(ReplicaSetKind)
	HorizontalPodAutoscalerGVK = autoscaling_v2b1.SchemeGroupVersion.WithKind(HorizontalPodAutoscalerKind)
	ConfigMapGVK               = core_v1.SchemeGroupVersion.WithKind(ConfigMapKind)
	NamespaceGVK               = core_v1.SchemeGroupVersion.WithKind(NamespaceKind)
	RoleBindingGVK             = rbac_v1.SchemeGroupVersion.WithKind(RoleBindingKind)

	ServiceInstanceGVR = meta_v1.GroupVersionResource{
		Group:    sc_v1b1.SchemeGroupVersion.Group,
		Version:  sc_v1b1.SchemeGroupVersion.Version,
		Resource: ServiceInstanceResource,
	}
	ServiceBindingGVR = meta_v1.GroupVersionResource{
		Group:    sc_v1b1.SchemeGroupVersion.Group,
		Version:  sc_v1b1.SchemeGroupVersion.Version,
		Resource: ServiceBindingResource,
	}
	IngressGVR = meta_v1.GroupVersionResource{
		Group:    ext_v1b1.SchemeGroupVersion.Group,
		Version:  ext_v1b1.SchemeGroupVersion.Version,
		Resource: IngressResource,
	}
)
