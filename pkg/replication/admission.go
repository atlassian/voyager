package replication

import (
	"context"
	"crypto/sha1" // nolint: gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/admission"
	apis_composition "github.com/atlassian/voyager/pkg/apis/composition"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/composition"
	comp_crd "github.com/atlassian/voyager/pkg/composition/crd"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/sets"
	"github.com/atlassian/voyager/pkg/util/validation"
	"github.com/go-chi/chi"
	"github.com/go-openapi/validate"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	authz_v1 "k8s.io/api/authorization/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiservervalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	authz_v1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

const (
	// key is the name of the annotation applied to the ServiceDescriptor
	updatedKey = "mutationTimestamp"
	hashKey    = "mutationHash"

	ReplicateKey = "replicate"

	// enforce limit on the ServiceDescriptor size
	// (since overlarge audit messages are dropped)
	maxServiceDescriptorSizeBytes = 100 * 1024 // 100 KB
)

type RejectionMessage string

// Need to write a thing that takes a router and does things to it
var sdResource = metav1.GroupVersionResource{
	Group:    comp_v1.SchemeGroupVersion.Group,
	Version:  comp_v1.SchemeGroupVersion.Version,
	Resource: comp_v1.ServiceDescriptorResourcePlural,
}

type AdmissionContext struct {
	AuthzClient         authz_v1client.SubjectAccessReviewInterface
	CurrentLocation     voyager.ClusterLocation
	ReplicatedLocations sets.ClusterLocation

	validator *validate.SchemaValidator
}

// SetupAPI handles creating a route for the mutating webhook
func (ac *AdmissionContext) SetupAdmissionWebhooks(r *chi.Mux) error {
	var err error
	if ac.validator, err = setupValidator(); err != nil {
		return err
	}

	r.Post("/admission/servicedescriptorauthz",
		admission.AdmitFuncHandlerFunc("servicedescriptorauthz", ac.servicedescriptorAuthzAdmitFunc))
	r.Post("/admission/servicedescriptorvalidation",
		admission.AdmitFuncHandlerFunc("servicedescriptorvalidation", ac.servicedescriptorMutationAdmitFunc))
	return nil
}

func setupValidator() (*validate.SchemaValidator, error) {
	crValidation := apiextensions.CustomResourceValidation{}
	err := apiext_v1b1.Convert_v1beta1_CustomResourceValidation_To_apiextensions_CustomResourceValidation(comp_crd.ServiceDescriptorCrd().Spec.Validation, &crValidation, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	crValidation.OpenAPIV3Schema.Properties["spec"] = addAdditionalProperties(crValidation.OpenAPIV3Schema.Properties["spec"])
	validator, _, err := apiservervalidation.NewSchemaValidator(&crValidation)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return validator, nil
}

func addAdditionalProperties(props apiextensions.JSONSchemaProps) apiextensions.JSONSchemaProps {
	switch props.Type {
	case "object":
		if props.AdditionalProperties == nil {
			props.AdditionalProperties = &apiextensions.JSONSchemaPropsOrBool{Allows: false}
		}

		for propname, propvalue := range props.Properties {
			props.Properties[propname] = addAdditionalProperties(propvalue)
		}
	case "array":
		if props.Items.Schema != nil {
			newSchema := addAdditionalProperties(*props.Items.Schema)
			props.Items.Schema = &newSchema
		}
		for i, schema := range props.Items.JSONSchemas {
			props.Items.JSONSchemas[i] = addAdditionalProperties(schema)
		}
	}
	return props
}

func (ac *AdmissionContext) servicedescriptorAuthzAdmitFunc(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	// make sure our webhook is setup correctly (has valid operation/resource)
	supportedOperations := []admissionv1beta1.Operation{admissionv1beta1.Create}
	if err := checkRequest(admissionRequest, supportedOperations); err != nil {
		return badRequest(admissionRequest.UID, err.Error()), nil
	}

	sdName, err := getServiceDescriptorName(admissionRequest.Object)
	if err != nil {
		return badRequest(admissionRequest.UID, err.Error()), nil
	}
	if sdName == "" {
		message := "Service Descriptor is missing a name"
		return badRequest(admissionRequest.UID, message), nil
	}

	userInfo := admissionRequest.UserInfo
	extra := make(map[string]authz_v1.ExtraValue, len(userInfo.Extra))
	for k, v := range userInfo.Extra {
		extra[k] = authz_v1.ExtraValue(v)
	}

	sar, err := ac.AuthzClient.Create(&authz_v1.SubjectAccessReview{
		Spec: authz_v1.SubjectAccessReviewSpec{
			User:   userInfo.Username,
			Groups: userInfo.Groups,
			ResourceAttributes: &authz_v1.ResourceAttributes{
				Group:     apis_composition.GroupName,
				Resource:  comp_v1.ServiceDescriptorResourcePlural,
				Namespace: metav1.NamespaceNone, // cluster-scoped
				Name:      sdName,
				// custom verb
				Verb: k8s.ServiceDescriptorClaimVerb,
			},
			Extra: extra,
		},
	})

	if err != nil {
		return nil, err
	}

	if !sar.Status.Allowed {
		message := fmt.Sprintf("RBAC: user not allowed to create ServiceDescriptors with name %q", sdName)
		return forbidden(admissionRequest.UID, message), nil
	}

	return allowedResponse(admissionRequest.UID, sar.Status.Reason), nil
}

func (ac *AdmissionContext) servicedescriptorMutationAdmitFunc(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	// make sure our webhook is setup correctly (has valid operation/resource)
	supportedOperations := []admissionv1beta1.Operation{admissionv1beta1.Create, admissionv1beta1.Update}
	if err := checkRequest(admissionRequest, supportedOperations); err != nil {
		return nil, err
	}

	newSD, admissionResponse, err := ac.validateSD(admissionRequest)
	if err != nil {
		return nil, err
	}
	if admissionResponse != nil {
		return admissionResponse, nil
	}

	patch, err := createAnnotationPatch(admissionRequest.Operation, newSD)
	if err != nil {
		return nil, err
	}

	pt := admissionv1beta1.PatchTypeJSONPatch
	return &admissionv1beta1.AdmissionResponse{
		UID:       admissionRequest.UID,
		Allowed:   true,
		Result:    &metav1.Status{},
		PatchType: &pt,
		Patch:     patch,
	}, nil
}

func checkRequest(admissionRequest *admissionv1beta1.AdmissionRequest, operationsAllowed []admissionv1beta1.Operation) error {
	validOperation := false
	for _, operationAllowed := range operationsAllowed {
		if admissionRequest.Operation == operationAllowed {
			validOperation = true
			break
		}
	}
	if !validOperation {
		return errors.Errorf("unsupported operation %q", string(admissionRequest.Operation))
	}

	if admissionRequest.Resource != sdResource {
		return errors.Errorf("only ServiceDescriptor is supported, got %q", admissionRequest.Resource)
	}

	return nil
}

func (ac *AdmissionContext) validateSD(admissionRequest *admissionv1beta1.AdmissionRequest) (*comp_v1.ServiceDescriptor, *admissionv1beta1.AdmissionResponse, error) {
	if rejectMessage := validateSize(admissionRequest); rejectMessage != "" {
		return nil, rejected(admissionRequest.UID, rejectMessage), nil
	}

	newSD, rejectMessage, err := getServiceDescriptor(ac.validator, admissionRequest.Object)
	if err != nil {
		return nil, nil, err
	}
	if rejectMessage != "" {
		return nil, rejected(admissionRequest.UID, rejectMessage), nil
	}

	if rejectMessage := validateName(newSD.Name); rejectMessage != "" {
		return nil, rejected(admissionRequest.UID, rejectMessage), nil
	}

	if rejectMessage := ac.validateLocationsAndTransforms(newSD); rejectMessage != "" {
		return nil, rejected(admissionRequest.UID, rejectMessage), nil
	}

	var oldSD *comp_v1.ServiceDescriptor
	if admissionRequest.Operation == admissionv1beta1.Update {
		oldSD, rejectMessage, err = getServiceDescriptor(ac.validator, admissionRequest.OldObject)
		if err != nil {
			return nil, nil, err
		}
		if rejectMessage != "" {
			return nil, rejected(admissionRequest.UID, rejectMessage), nil
		}
		rejectMessage, err = validateUpdate(newSD, oldSD)
		if err != nil {
			return nil, nil, err
		}
		if rejectMessage != "" {
			return nil, rejected(admissionRequest.UID, rejectMessage), nil
		}
	}

	return newSD, nil, err
}

func (ac *AdmissionContext) validateLocationsAndTransforms(sd *comp_v1.ServiceDescriptor) RejectionMessage {
	var rejectionMessages []string

	// more than one sdLocation may map to the same cluster
	clusterLocations := make(sets.ClusterLocation)
	for _, sdLocation := range sd.Spec.Locations {
		if sdLocation.Label != "" {
			rejectionMessages = append(rejectionMessages,
				fmt.Sprintf("labels are currently not supported (location: %q)", sdLocation.Name))
		}

		location := sdLocation.VoyagerLocation().ClusterLocation()
		clusterLocations.Insert(location)
		if sdLocation.EnvType == ac.CurrentLocation.EnvType && !ac.ReplicatedLocations.Has(location) {
			rejectionMessages = append(rejectionMessages,
				fmt.Sprintf("location %q does not exist in %q", sdLocation.Name, sdLocation.EnvType))
		}
	}

	resourceGroupsInCurrentEnv := false
	for _, clusterLocation := range clusterLocations.UnsortedList() {
		// we evaluate for every cluster location to throw out rubbish ASAP
		transformer := composition.NewServiceDescriptorTransformer(clusterLocation)

		objs, err := transformer.CreateFormationObjectDef(sd)
		if err != nil {
			// Unfortunately, if we get an error from the transformer, we don't want to continue,
			// because we're likely to accumulate duplicate/nonsense errors as we reapply in
			// different locations.
			rejectionMessages = append(rejectionMessages, err.Error())
			return RejectionMessage(strings.Join(rejectionMessages, ", "))
		}
		if len(objs) == 0 {
			// This does NOT locations with only empty resource groups, only
			// locations that are not referred to by any resource groups.
			rejectionMessages = append(rejectionMessages,
				fmt.Sprintf("no resource groups defined for %q", clusterLocation))
			continue
		}

		if clusterLocation.EnvType == ac.CurrentLocation.EnvType {
			resourceGroupsInCurrentEnv = true
		}
	}

	if !resourceGroupsInCurrentEnv {
		rejectionMessages = append(rejectionMessages,
			fmt.Sprintf("no resource groups to be created for environment type %q", ac.CurrentLocation.EnvType))
	}

	return RejectionMessage(strings.Join(rejectionMessages, ", "))
}

func createAnnotationPatch(operation admissionv1beta1.Operation, newSD *comp_v1.ServiceDescriptor) ([]byte, error) {
	ts, err := generateUpdated(operation, newSD)
	if err != nil {
		return nil, err
	}

	hash, err := generateSpecHash(newSD)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate hash for SD")
	}

	jp := buildAnnotations(newSD, ts, hash)
	patch, err := json.Marshal(jp)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return patch, nil
}

func validateSize(admissionRequest *admissionv1beta1.AdmissionRequest) RejectionMessage {
	actualSize := len(admissionRequest.Object.Raw)
	if actualSize > maxServiceDescriptorSizeBytes {
		return RejectionMessage(fmt.Sprintf(
			"ServiceDescriptor size %v exceeds the limit %v",
			actualSize, maxServiceDescriptorSizeBytes))
	}
	return ""
}

func getServiceDescriptorName(rawObj runtime.RawExtension) (string, error) {
	var obj unstructured.Unstructured
	err := json.Unmarshal(rawObj.Raw, &obj)
	if err != nil {
		return "", errors.Wrap(err, "unable to deserialize ServiceDescriptor from request")
	}
	return obj.GetName(), nil
}

func getServiceDescriptor(validator *validate.SchemaValidator, object runtime.RawExtension) (*comp_v1.ServiceDescriptor, RejectionMessage, error) {
	var rawSd map[string]interface{}
	err := json.Unmarshal(object.Raw, &rawSd)
	if err != nil {
		return nil, "", errors.Wrap(err, "unable to deserialize servicedescriptor from request")
	}
	// We have to run the validation ourselves before we deserialize, because
	// kubernetes hasn't validated us against the CRD schema at this point and
	// the errors that come out of invalid typed deserialisation are cryptic.
	if err := apiservervalidation.ValidateCustomResource(rawSd, validator); err != nil {
		return nil, RejectionMessage(err.Error()), nil
	}
	var sd comp_v1.ServiceDescriptor
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSd, &sd); err != nil {
		return nil, "", errors.Wrap(err, "unable to parse SD from request")
	}

	return &sd, "", nil
}

func validateUpdate(newSD, oldSD *comp_v1.ServiceDescriptor) (RejectionMessage, error) {
	if oldSD.DeletionTimestamp != nil {
		oldHash, err := generateSpecHash(oldSD)
		if err != nil {
			return "", errors.Wrap(err, "unable to generate hash for old SD")
		}
		newHash, err := generateSpecHash(newSD)
		if err != nil {
			return "", errors.Wrap(err, "unable to generate hash for new SD")
		}
		if oldHash != newHash {
			return RejectionMessage("ServiceDescriptor spec cannot be updated during deletion"), nil
		}
	}

	newTS, exists := newSD.Annotations[updatedKey]
	if !exists || newTS == "" {
		return "", nil
	}
	// We should parse/error out immediately if the 'new ts' is in the wrong form
	// to avoid putting bad things in kube.
	new, err := parseTimestamp(newTS)
	if err != nil {
		return RejectionMessage(updatedKey + " annotation can't be parsed: " + err.Error()), nil
	}

	oldTS, exists := oldSD.Annotations[updatedKey]
	if !exists || oldTS == "" {
		// It *shouldn't* be the case that the existing object has no timestamp but the submitted one does
		// Error out if this does happen
		return RejectionMessage("Please remove the " + updatedKey + " from the submitted Service Descriptor"), nil
	}

	// In theory, old TS can't be in wrong form...
	old, err := parseTimestamp(oldTS)
	if err != nil {
		return "", err
	}

	return validateTimes(new, old), nil
}

func parseTimestamp(ts string) (time.Time, error) {
	return time.Parse(time.RFC3339, ts)
}

func validateName(name string) RejectionMessage {
	// this should already be validated by creator, since we cannot create a service
	// with -- in the name. This is an explicit check and is still valuable for
	// the cases where sysadmins go around creating services without ownership checks.
	if validation.HasDoubleDash(name) {
		return RejectionMessage("ServiceDescriptor name should not contain --")
	}

	return ""
}

func validateTimes(new time.Time, old time.Time) RejectionMessage {
	// The "new" object is from the past - abort!
	if new.Before(old) {
		return RejectionMessage(fmt.Sprintf("You are attempting to override a Service Descriptor from %s with an older version from %s",
			new.Format(time.RFC3339), old.Format(time.RFC3339)))
	}

	return ""
}

func buildAnnotations(rawSd *comp_v1.ServiceDescriptor, updated time.Time, hash string) util.JSONPatch {
	var jp util.JSONPatch
	if len(rawSd.Annotations) == 0 {
		jp = append(jp, addAnnotations(map[string]string{
			updatedKey: updated.UTC().Format(time.RFC3339),
			hashKey:    hash,
		}))
	} else {
		jp = append(jp,
			addAnnotation(updatedKey, updated.UTC().Format(time.RFC3339)),
			addAnnotation(hashKey, hash))
	}

	return jp
}

func addAnnotations(annotations map[string]string) util.Patch {
	return util.Patch{
		Operation: util.Add,
		Path:      "/metadata/annotations",
		Value:     annotations,
	}
}

func addAnnotation(key, value string) util.Patch {
	return util.Patch{
		Operation: util.Add,
		Path:      "/metadata/annotations/" + key,
		Value:     value,
	}
}

func generateUpdated(operation admissionv1beta1.Operation, newSD *comp_v1.ServiceDescriptor) (time.Time, error) {
	if operation != admissionv1beta1.Update {
		return time.Now(), nil
	}
	ts, exists := newSD.Annotations[updatedKey]
	if !exists || ts == "" {
		return time.Now(), nil
	}

	return parseTimestamp(ts)
}

func generateSpecHash(sd *comp_v1.ServiceDescriptor) (string, error) {
	j, err := json.Marshal(sd.Spec)
	if err != nil {
		return "", err
	}

	checksum := sha1.Sum(j) // nolint: gosec
	return hex.EncodeToString(checksum[:]), nil
}

func rejected(requestUID types.UID, message RejectionMessage) *admissionv1beta1.AdmissionResponse {
	return rejectedResponse(requestUID, http.StatusBadRequest, string(message))
}

func badRequest(requestUID types.UID, message string) *admissionv1beta1.AdmissionResponse {
	return rejectedResponse(requestUID, http.StatusBadRequest, message)
}

func forbidden(requestUID types.UID, message string) *admissionv1beta1.AdmissionResponse {
	return rejectedResponse(requestUID, http.StatusForbidden, message)
}

func rejectedResponse(requestUID types.UID, code int32, message string) *admissionv1beta1.AdmissionResponse {
	return &admissionv1beta1.AdmissionResponse{
		UID:     requestUID,
		Allowed: false,
		Result: &metav1.Status{
			Message: message,
			Code:    code,
		},
	}
}

func allowedResponse(requestUID types.UID, message string) *admissionv1beta1.AdmissionResponse {
	return &admissionv1beta1.AdmissionResponse{
		UID:     requestUID,
		Allowed: true,
		Result: &metav1.Status{
			Message: message,
			Code:    http.StatusOK,
		},
	}
}
