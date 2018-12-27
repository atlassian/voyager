package computionadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/admission"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	"github.com/atlassian/voyager/pkg/execution/svccatadmission"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/ghodss/yaml"
	"github.com/go-chi/chi"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	admission_v1beta1 "k8s.io/api/admission/v1beta1"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	reasonPRGBCompliantArtifacts       = "compute resource refers to compliant artifacts"
	reasonServiceMetaConfigMapMissing  = "unable to find service meta configmap"
	reasonWrongFormatServiceMetaData   = "wrong format service meta data"
	reasonPRGBComplianceMissing        = "compliance information not found - please answer the compliance questions for your service in Microscope"
	reasonNotEC2ComputeServiceInstance = "service instance is not a valid EC2 compute resource"
	reasonPRGBComplianceNotRequired    = "service does not require enforcement of PRGB compliant artifacts"
	reasonUnrestrictedEnvironment      = "compute resource will occupy an environment with no special compliance requirements"
	reasonUnrestrictedNamespace        = "compute resource will occupy a k8s Namespace with no special compliance requirements"
	reasonNonNamespacedResource        = "non namespaced resources which don't need this admission"
	reasonNonCompliantArtifactFound    = "non-compliant artifact found; all artifacts must come from the SOX namespace. violation(s): "
)

type composeServices struct {
	Docker dockerCompose `json:"docker"`
}

type dockerCompose struct {
	Compose map[string]partialContainerSpec `json:"compose"`
}

type partialContainerSpec struct {
	Image string `json:"image"`
}

// AdmissionContext holds context for compution admission controller like router, informers etc
type AdmissionContext struct {
	ConfigMapInformer       cache.SharedIndexInformer
	NamespaceInformer       cache.SharedIndexInformer
	EnforcePRGB             bool
	CompliantDockerPrefixes []string
}

// SetupAdmissionWebhooks handles creating a route for the mutating webhook
func (ac *AdmissionContext) SetupAdmissionWebhooks(router *chi.Mux) error {
	router.Post("/kubecompute", admission.AdmitFuncHandlerFunc("kubecompute", ac.podAdmitFunc))
	router.Post("/ec2compute", admission.AdmitFuncHandlerFunc("ec2compute", ac.serviceInstanceAdmitFunc))
	return nil
}

// admission Func for anything that includes a PodSpec somewhere
func (ac *AdmissionContext) podAdmitFunc(ctx context.Context, logger *zap.Logger, admissionReview admission_v1beta1.AdmissionReview) (*admission_v1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	res, err := ac.earlyResponseMaybe(admissionRequest)
	if res != nil || err != nil {
		return res, err
	}

	PRGBRequired, res, err := ac.checkPRGBRequired(admissionRequest)
	if res != nil || err != nil {
		return res, err
	}
	if !PRGBRequired {
		return allowWithReason(reasonPRGBComplianceNotRequired), nil
	}

	artifacts, err := extractArtifactsFromRequest(admissionRequest)
	if err != nil {
		return nil, err
	}

	return ac.responseForArtifacts(artifacts)
}

func extractArtifactsFromRequest(req *admission_v1beta1.AdmissionRequest) ([]string, error) {
	// convert from meta_v1 GVK to schema GVK
	gvk := schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}
	switch gvk {
	// we want to cover different kinds of Deployments
	case k8s.DeploymentGVK:
		var deployment apps_v1.Deployment
		err := json.Unmarshal(req.Object.Raw, &deployment)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse apps.Deployment object")
		}
		return extractPodArtifacts(deployment.Spec.Template.Spec)
	case ext_v1beta1.SchemeGroupVersion.WithKind(k8s.DeploymentKind):
		var deployment ext_v1beta1.Deployment
		err := json.Unmarshal(req.Object.Raw, &deployment)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse beta1v1.Deployment object")
		}
		return extractPodArtifacts(deployment.Spec.Template.Spec)
	case k8s.PodGVK:
		var pod core_v1.Pod
		err := json.Unmarshal(req.Object.Raw, &pod)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse Pod object")
		}
		return extractPodArtifacts(pod.Spec)
	}

	return nil, errors.Errorf("unknown GVK of %q provided", gvk)
}

// admission Func for EC2 compute service instance
func (ac *AdmissionContext) serviceInstanceAdmitFunc(ctx context.Context, logger *zap.Logger, admissionReview admission_v1beta1.AdmissionReview) (*admission_v1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	res, err := ac.earlyResponseMaybe(admissionRequest)
	if res != nil || err != nil {
		return res, err
	}

	// make sure it's ClusterServiceClass/micros CLASS ServiceInstance if it's EC2Compute
	// Or it should just accept early
	var si sc_v1b1.ServiceInstance
	err = json.Unmarshal(admissionRequest.Object.Raw, &si)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ServiceInstance object")
	}

	if !svccatadmission.IsMicrosServiceClass(si) {
		return allowWithReason(reasonNotEC2ComputeServiceInstance), nil
	}

	PRGBRequired, res, err := ac.checkPRGBRequired(admissionRequest)
	if res != nil || err != nil {
		return res, err
	}
	if !PRGBRequired {
		return allowWithReason(reasonPRGBComplianceNotRequired), nil
	}

	artifacts, err := extractEC2ComputeArtifacts(&si)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract EC2 compute artifacts")
	}

	return ac.responseForArtifacts(artifacts)
}

func (ac *AdmissionContext) earlyResponseMaybe(ar *admission_v1beta1.AdmissionRequest) (*admission_v1beta1.AdmissionResponse, error) {
	// succeed early when PRGB enforcement is off
	if !ac.EnforcePRGB {
		return allowWithReason(reasonUnrestrictedEnvironment), nil
	}

	// succeed early when we shouldn't police the Namespace
	if ar.Namespace == "" {
		return allowWithReason(reasonNonNamespacedResource), nil
	}
	validVoyagerNamespace, err := ac.isVoyagerNamespace(ar.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to validate Namespace: %s", ar.Namespace)
	}
	if !validVoyagerNamespace {
		return allowWithReason(reasonUnrestrictedNamespace), nil
	}

	return nil, nil
}

// check if the namespace is voyager namespace by looking up namespace label
// as we only need to run admission on resources inside voyager namepsaces
func (ac *AdmissionContext) isVoyagerNamespace(namespace string) (bool, error) {
	nsObj, exists, err := ac.NamespaceInformer.GetIndexer().GetByKey(namespace)
	if err != nil {
		return false, errors.Wrap(err, "failed to retrieve Namespace object")
	}
	if !exists {
		return false, errors.New("Namespace doesn't exist inside informer indexer")
	}
	ns, ok := nsObj.(*core_v1.Namespace)
	if !ok {
		return false, errors.New("failed to assert Namespace object")
	}
	for k := range ns.Labels {
		// check if it includes Voyager service name label
		if k == voyager.ServiceNameLabel {
			return true, nil
		}
	}

	return false, nil
}

// check compliance information inside its service metadata configmap to see if a service needs PRGB compliance
func (ac *AdmissionContext) checkPRGBRequired(ar *admission_v1beta1.AdmissionRequest) (bool, *admission_v1beta1.AdmissionResponse, error) {
	// retrieve cached PRGB compliance information from ConfigMap
	cfgMap, ok, err := ac.lookupConfigMap(ar.Namespace, apisynchronization.DefaultServiceMetadataConfigMapName)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to lookup ConfigMap object")
	}
	if !ok {
		return false, rejectWithReason(reasonServiceMetaConfigMapMissing), nil
	}

	data := cfgMap.Data[orch_meta.ConfigMapConfigKey]
	var config orch_meta.ServiceProperties
	err = yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		return false, rejectWithReason(reasonWrongFormatServiceMetaData), nil
	}

	if config.Compliance.PRGBControl == nil {
		return false, rejectWithReason(reasonPRGBComplianceMissing), nil
	}

	if *config.Compliance.PRGBControl {
		return true, nil, nil
	}
	return false, nil, nil
}

// check if a service has configmap and return a configmap object if it exists
func (ac *AdmissionContext) lookupConfigMap(namespace, cfmName string) (*core_v1.ConfigMap, bool, error) {
	cfmKey := namespace + "/" + cfmName
	cfmObj, exists, err := ac.ConfigMapInformer.GetIndexer().GetByKey(cfmKey)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to get ConfigMap: %s", cfmKey)
	}
	if !exists {
		return nil, false, nil
	}
	cfm, ok := cfmObj.(*core_v1.ConfigMap)
	if !ok {
		return nil, false, errors.New("failed to assert configmap object")
	}
	return cfm, true, nil
}

// validate if an image artifact is PRGB compliance
func (ac *AdmissionContext) isCompliantArtifact(artifact string) bool {
	for _, prefix := range ac.CompliantDockerPrefixes {
		if strings.HasPrefix(artifact, prefix) {
			return true
		}
	}
	return false
}

// validate if a list of image artifacts is PRGB compliance
func (ac *AdmissionContext) responseForArtifacts(refs []string) (*admission_v1beta1.AdmissionResponse, error) {
	nonCompliantArtifacts := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !ac.isCompliantArtifact(ref) {
			nonCompliantArtifacts = append(nonCompliantArtifacts, ref)
		}
	}

	if len(nonCompliantArtifacts) > 0 {
		return rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, strings.Join(nonCompliantArtifacts, ","))), nil
	}

	return allowWithReason(reasonPRGBCompliantArtifacts), nil
}

// respond allow admission response with some predefined reasons
func allowWithReason(reason string) *admission_v1beta1.AdmissionResponse {
	return &admission_v1beta1.AdmissionResponse{
		Allowed: true,
		Result: &meta_v1.Status{
			Message: reason,
		},
	}
}

// respond reject admission response with some predefined reasons
func rejectWithReason(reason string) *admission_v1beta1.AdmissionResponse {
	return &admission_v1beta1.AdmissionResponse{
		Allowed: false,
		Result: &meta_v1.Status{
			Message: reason,
		},
	}
}

// Get images list inside ec2 compute service instance
func extractEC2ComputeArtifacts(si *sc_v1b1.ServiceInstance) ([]string, error) {
	var composeSrvs composeServices

	err := json.Unmarshal(si.Spec.Parameters.Raw, &composeSrvs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ServiceInstance EC2 compose config")
	}
	images := make([]string, 0, len(composeSrvs.Docker.Compose))
	for _, c := range composeSrvs.Docker.Compose {
		images = append(images, c.Image)
	}
	return images, nil
}

// Get images list used inside pod spec
func extractPodArtifacts(podSpec core_v1.PodSpec) ([]string, error) {
	images := make([]string, 0, len(podSpec.InitContainers)+len(podSpec.Containers))
	// add all images for init containers
	for _, c := range podSpec.InitContainers {
		images = append(images, c.Image)
	}
	// add all images for main containers
	for _, c := range podSpec.Containers {
		images = append(images, c.Image)
	}
	return images, nil
}
