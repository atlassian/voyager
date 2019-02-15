package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	pagerdutyClient "github.com/PagerDuty/go-pagerduty"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/api/schema"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/creator/luigi"
	"github.com/atlassian/voyager/pkg/creator/ssam"
	ec2compute_v2 "github.com/atlassian/voyager/pkg/orchestration/wiring/ec2compute/v2"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/apiservice"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/uuid"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

const (
	ServiceNameMinimumLength    = 1
	ServiceNameMaximumLength    = 24
	ServiceNameExpr             = schema.ResourceNameSchemaPattern
	EC2ComputeNameMaximumLength = ec2compute_v2.MaximumServiceNameLength
)

var (
	ServiceNameRegExp    = regexp.MustCompile(ServiceNameExpr)
	ServiceGroupResource = creator_v1.Resource(creator_v1.ServiceResourceSingular)
)

// this list corresponds to namespaces we are using inside a cluster that should not be valid service names
var blacklist = []voyager.ServiceName{
	"kube-system",
	"kube-public",
	"micros",
	"voyager",
	"service-catalog",
	"smith",
	"asap-secrets",
	"voyager-access",
	"voyager-monitor",
	"voyager-monitoring",
}

type ServiceHandler struct {
	serviceCentral ServiceCentralStoreInterface
	pagerDuty      PagerDutyClientInterface
	luigi          LuigiClientInterface
	ssam           SSAMClientInterface
}

func NewHandler(config *ExtraConfig) (*ServiceHandler, error) {
	scHTTPClient := util.HTTPClient()
	luigiHTTPClient := util.HTTPClient()

	scClient := servicecentral.NewServiceCentralClient(config.Logger, scHTTPClient, config.ASAPClientConfig, config.ServiceCentralURL)
	luigiClient := luigi.NewLuigiClient(config.Logger, luigiHTTPClient, config.ASAPClientConfig, config.LuigiURL)

	pdClient := pagerdutyClient.NewClient(config.PagerDuty.AuthToken)
	ssamClient := ssam.NewSSAMClient(config.HTTPClient, config.ASAPClientConfig, config.SSAMURL)

	pagerDuty, err := pagerduty.New(config.Logger, pdClient, uuid.Default())
	if err != nil {
		return nil, err
	}

	return &ServiceHandler{
		serviceCentral: servicecentral.NewStore(config.Logger, scClient),
		pagerDuty:      pagerDuty,
		luigi:          luigi.NewCreator(config.Logger, luigiClient),
		ssam:           ssam.NewServiceCreator(ssamClient),
	}, nil
}

func (h *ServiceHandler) createSSAMContainer(ctx context.Context, service *creator_v1.Service) (accessLevels ssam.AccessLevels, wasMutated bool, mutationErr error) {
	metadata := &ssam.ServiceMetadata{
		ServiceName:  voyager.ServiceName(service.Name),
		ServiceOwner: service.Spec.ResourceOwner,
	}

	if service.Spec.SSAMContainerName != "" {
		// TODO - https://trello.com/c/hJfnsDqb/757-support-arbitrary-container-names-in-creator
		if name := h.ssam.GetExpectedServiceContainerName(ctx, metadata); service.Spec.SSAMContainerName != name {
			err := errors.Errorf("An SSAM container %q already exists, but doesn't match expected name %q", service.Spec.SSAMContainerName, name)
			return ssam.AccessLevels{}, false, errors.Wrap(err, "Failed to create service in SSAM")
		}
	}

	// Create or validate existing SSAM container and access levels
	containerName, accessLevels, err := h.ssam.CreateService(ctx, metadata)
	if err != nil {
		return ssam.AccessLevels{}, false, errors.Wrap(err, "Failed to create service in SSAM")
	}
	service.Spec.SSAMContainerName = containerName
	return accessLevels, true, nil
}

func (h *ServiceHandler) createPagerDuty(ctx context.Context, service *creator_v1.Service) (wasMutated bool, mutationErr error) {
	if service.Spec.Metadata.PagerDuty != nil {
		return false, nil
	}
	url, err := pagerduty.GetServiceSearchURL(voyager.ServiceName(service.Name))
	if err != nil {
		return false, errors.Wrap(err, "Failed to generate search URL for PagerDuty")
	}
	// Nevermind the name of the field, it is already being used for storing PagerDuty search URL ¯\_(ツ)_/¯
	service.Spec.PagerDutyServiceID = url
	email := service.Spec.EmailAddress()
	user, err := auth.Named(service.Spec.ResourceOwner)
	if err != nil {
		return false, err
	}
	pagerDutyMetadata, err := h.pagerDuty.FindOrCreate(voyager.ServiceName(service.Name), user, email)

	if err != nil {
		return false, errors.Wrap(err, "Failed to create services in PagerDuty")
	}
	service.Spec.Metadata.PagerDuty = &pagerDutyMetadata
	return true, nil
}

func (h *ServiceHandler) createLuigi(ctx context.Context, service *creator_v1.Service, accessLevels ssam.AccessLevels) (wasMutated bool, mutationErr error) {
	if service.Spec.LoggingID != "" {
		return false, nil
	}

	if accessLevels.Production == "" {
		return false, errors.New("Could not find a Production SSAM Access Level for your service to use in Luigi")
	}

	luigiServiceData, err := h.luigi.FindOrCreateService(ctx, &luigi.ServiceMetadata{
		Name:         service.Name,
		BusinessUnit: service.Spec.BusinessUnit,
		Owner:        service.Spec.ResourceOwner,
		Admins:       accessLevels.Production,
	})
	if err != nil {
		return false, errors.Wrap(err, "Failed to create service in Luigi")
	}

	service.Spec.LoggingID = luigiServiceData.LoggingID
	return true, nil
}

func (h *ServiceHandler) ServiceCreate(ctx context.Context, service *creator_v1.Service) (*creator_v1.Service, error) {
	authUser, err := userFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to extract user from the request: %v", err))
	}

	err = defaultAndValidateService(service, authUser)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to validate service: %v", err))
	}

	actualService, scErr := h.serviceCentral.FindOrCreateService(ctx, authUser, service)
	if scErr != nil {
		if httputil.IsBadRequest(scErr) {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to create service in Service Central: %v", scErr))
		}
		return nil, apierrors.NewInternalError(errors.Wrap(scErr, "Failed to create service in Service Central"))
	}

	serviceCentralUpdateNeeded := false
	accessLevels, mutated, mutateErr := h.createSSAMContainer(ctx, actualService)
	if mutateErr != nil {
		return nil, apierrors.NewInternalError(mutateErr)
	}

	if mutated {
		serviceCentralUpdateNeeded = true
	}

	mutated, mutateErr = h.createPagerDuty(ctx, actualService)
	if mutateErr != nil {
		return nil, apierrors.NewInternalError(mutateErr)
	}

	if mutated {
		serviceCentralUpdateNeeded = true
	}

	mutated, mutateErr = h.createLuigi(ctx, actualService, accessLevels)
	if mutateErr != nil {
		return nil, apierrors.NewInternalError(mutateErr)
	}

	if mutated {
		serviceCentralUpdateNeeded = true
	}

	if serviceCentralUpdateNeeded {
		updateErr := h.serviceCentral.PatchService(ctx, authUser, actualService)
		if updateErr != nil {
			return nil, apierrors.NewInternalError(errors.Wrap(updateErr, "Failed to update PagerDuty metadata in Service Central"))
		}
	}

	return actualService, nil
}

func (h *ServiceHandler) ServiceGet(ctx context.Context, name voyager.ServiceName) (*creator_v1.Service, error) {
	user := maybeUserFromContext(ctx)

	service, err := h.serviceCentral.GetService(ctx, user, servicecentral.ServiceName(name))
	if err != nil {
		if servicecentral.IsNotFound(err) {
			return nil, apierrors.NewNotFound(ServiceGroupResource, string(name))
		}
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to get service from Service Central"))
	}

	return service, nil
}

func (h *ServiceHandler) ServiceList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logz.RetrieveLoggerFromContext(ctx)
	user := auth.MaybeRequestUser(r)

	services, err := h.serviceCentral.ListServices(ctx, user)
	if err != nil {
		apiservice.RespondWithInternalError(logger, w, r, "Failed to get service from Service Central", err)
		return
	}

	serviceListResponse := creator_v1.ServiceList{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: creator_v1.ServiceResourceAPIVersion,
			Kind:       creator_v1.ServiceListResourceKind,
		},
		ListMeta: meta_v1.ListMeta{},
		Items:    services,
	}
	serviceListBytes, err := json.Marshal(serviceListResponse)
	if err != nil {
		apiservice.RespondWithInternalError(logger, w, r, "Failed to marshal response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httputil.WriteOkResponse(logger, w, serviceListBytes)
}

func (h *ServiceHandler) ServiceListNew(ctx context.Context) (*creator_v1.ServiceList, error) {
	user := maybeUserFromContext(ctx)

	services, err := h.serviceCentral.ListServices(ctx, user)
	if err != nil {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to get service from Service Central"))
	}

	serviceListResponse := creator_v1.ServiceList{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: creator_v1.ServiceResourceAPIVersion,
			Kind:       creator_v1.ServiceListResourceKind,
		},
		ListMeta: meta_v1.ListMeta{},
		Items:    services,
	}
	return &serviceListResponse, nil
}

func (h *ServiceHandler) ServiceUpdate(ctx context.Context, name voyager.ServiceName, objInfo rest.UpdatedObjectInfo) (*creator_v1.Service, error) {
	user, err := userFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to extract user from the request: %v", err))
	}

	service, err := h.serviceCentral.GetService(ctx, auth.ToOptionalUser(user), servicecentral.ServiceName(name))
	if err != nil {
		if servicecentral.IsNotFound(err) {
			return nil, apierrors.NewNotFound(ServiceGroupResource, string(name))
		}
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to fetch existing service"))
	}

	updatedObj, err := objInfo.UpdatedObject(ctx, service)
	if err != nil {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to calculate update"))
	}
	updatedService := updatedObj.(*creator_v1.Service)

	updateErr := h.serviceCentral.PatchService(ctx, user, updatedService)
	if updateErr != nil {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to update PagerDuty metadata in Service Central"))
	}

	return updatedService, nil
}

func (h *ServiceHandler) ServiceDelete(ctx context.Context, name voyager.ServiceName) (*creator_v1.Service, error) {
	user, err := userFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to extract user from the request: %v", err))
	}

	service, err := h.serviceCentral.GetService(ctx, auth.ToOptionalUser(user), servicecentral.ServiceName(name))
	if err != nil {
		if servicecentral.IsNotFound(err) {
			return nil, apierrors.NewNotFound(ServiceGroupResource, string(name))
		}
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to fetch existing service"))
	}

	// Note that once we start deleting a service, it may leave a service in a
	// partial state if the deletion fails, meaning the service central entry
	// still exists, but the Service is missing a bunch of PagerDuty/SSAM/etc.

	// Delete Luigi
	if service.Spec.LoggingID != "" {
		err = h.luigi.DeleteService(ctx, service.Spec.LoggingID)
		if err != nil && !httputil.IsNotFound(err) {
			return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to delete Luigi service"))
		}
	}

	// Delete PagerDuty
	err = h.pagerDuty.Delete(name)
	if err != nil && !httputil.IsNotFound(err) {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to delete PagerDuty service"))
	}

	// Delete SSAM container
	err = h.ssam.DeleteService(ctx, &ssam.ServiceMetadata{
		ServiceName:  name,
		ServiceOwner: service.Spec.ResourceOwner,
	})
	if err != nil && !httputil.IsNotFound(err) {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to delete SSAM containers"))
	}

	// Finally delete the service central record
	err = h.serviceCentral.DeleteService(ctx, user, servicecentral.ServiceName(name))
	if err != nil && !httputil.IsNotFound(err) {
		return nil, apierrors.NewInternalError(errors.Wrap(err, "Failed to delete Service Central service"))
	}

	return service, nil
}

func validateServiceName(serviceName voyager.ServiceName) error {
	if contains(blacklist, serviceName) {
		return NewBlackListError("service name", string(serviceName))
	}

	if len(serviceName) > ServiceNameMaximumLength {
		exampleServiceNameSize := 24
		exampleEC2ComputeNameSize := EC2ComputeNameMaximumLength - exampleServiceNameSize - 1
		return errors.Errorf(
			"service name is longer than %d, please keep in mind that service names are used as a prefix "+
				"with the EC2Compute name (by default) to identify EC2Compute resources (i.e. "+
				"<service name>-<ec2compute resource name>). This identifier has a hard limit of %d characters. "+
				"This means when your service name is %d characters, then your EC2Compute name can only be %d "+
				"character (as they are seperated by a dash) to make %d.",
			ServiceNameMaximumLength,
			EC2ComputeNameMaximumLength,
			exampleServiceNameSize,
			exampleEC2ComputeNameSize,
			EC2ComputeNameMaximumLength,
		)
	}

	if len(serviceName) < ServiceNameMinimumLength {
		return errors.Errorf("service name length is less than the minimum %d", ServiceNameMinimumLength)
	}

	if !ServiceNameRegExp.MatchString(string(serviceName)) {
		return errors.Errorf("Your service name is invalid. Only alphanumeric characters and dashes are allowed.")
	}

	return nil
}

func userFromContext(ctx context.Context) (auth.User, error) {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("Failed to extract user from the request")
	}
	authUser, err := auth.Named(user.GetName())
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("Failed to extract user from the request: %v", err))
	}
	return authUser, nil
}

func maybeUserFromContext(ctx context.Context) auth.OptionalUser {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return auth.NoUser()
	}
	return auth.MaybeNamed(user.GetName())
}

func defaultAndValidateService(service *creator_v1.Service, user auth.User) error {
	if err := validateServiceName(voyager.ServiceName(service.Name)); err != nil {
		return err
	}

	if service.Spec.ResourceOwner == "" {
		service.Spec.ResourceOwner = user.Name()
	}

	return nil
}

func contains(list []voyager.ServiceName, key voyager.ServiceName) bool {
	for _, value := range list {
		if value == key {
			return true
		}
	}
	return false
}
