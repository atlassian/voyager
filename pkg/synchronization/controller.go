package synchronization

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	ctrllogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	compClient "github.com/atlassian/voyager/pkg/composition/client"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/ssam"
	"github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/layers"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	NamespaceByServiceLabelIndexName = "nsByServiceLabelIndex"

	// KubeCompute docker secret
	dockerSecretName      = apik8scompute.DockerImagePullName // #nosec
	dockerSecretNamespace = "voyager"                         // #nosec

	commonSecretName = apik8scompute.CommonSecretName

	// kube2iam allowed roles annotation
	// see https://github.com/jtblin/kube2iam#namespace-restrictions
	allowedRolesAnnotation = "iam.amazonaws.com/allowed-roles"

	maxSyncWorkers = 10
)

const (
	serviceCentralPollPeriod            = 30 * time.Second
	serviceCentralListAllPeriod         = 30 * time.Minute
	serviceCentralListDriftCompensation = 5 * time.Second
	releaseManagementPollPeriod         = 5 * time.Second
	releaseManagementSyncAllPeriod      = 30 * time.Minute
	baseDelayProcSec                    = 15
	rmsPollJitterFactor                 = 1.2
)

type ServiceMetadataStore interface {
	GetService(ctx context.Context, user auth.OptionalUser, name string) (*creator_v1.Service, error)
	ListServices(ctx context.Context, user auth.OptionalUser) ([]creator_v1.Service, error)
	ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]creator_v1.Service, error)
}

type Controller struct {
	Logger       *zap.Logger
	ReadyForWork func()

	MainClient        kubernetes.Interface
	CompClient        compClient.Interface
	NamespaceInformer cache.SharedIndexInformer
	ConfigMapInformer cache.SharedIndexInformer

	ServiceCentral    ServiceMetadataStore
	ReleaseManagement releases.ReleaseManagementStore
	ClusterLocation   voyager.ClusterLocation

	RoleBindingUpdater        updater.ObjectUpdater
	ConfigMapUpdater          updater.ObjectUpdater
	NamespaceUpdater          updater.ObjectUpdater
	ClusterRoleUpdater        updater.ObjectUpdater
	ClusterRoleBindingUpdater updater.ObjectUpdater

	ServiceCentralPollErrorCounter prometheus.Counter
	AccessUpdateErrorCounter       *prometheus.CounterVec

	AllowMutateServices         bool
	LastFetchedAllServices      *time.Time
	LastFetchedModifiedServices *time.Time
	LastFetchedAllReleases      *time.Time
	NextFetchAllReleasesStart   *time.Time
}

func (c *Controller) Run(ctx context.Context) {
	defer c.Logger.Info("Shutting down Synchronization controller")
	c.Logger.Sugar().Infof("Starting the Synchronization controller in %q envtype", c.ClusterLocation.EnvType)
	c.ReadyForWork()

	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithContext(c.startPollingServiceCentral)
	stage.StartWithContext(c.startPollingReleaseManagementSystem)
	<-ctx.Done()
}

// Process will handle any changes to namespaces and ensure we have a ConfigMap
// with the appropriate data in place. It also ensures that roles and rolebindings
// are created for build authentication.
func (c *Controller) Process(ctx *ctrl.ProcessContext) (bool /* retriable */, error) {
	ns := ctx.Object.(*core_v1.Namespace)
	nsLogger := ctx.Logger.With(ctrllogz.NamespaceName(ns.Name))
	if ns.ObjectMeta.DeletionTimestamp != nil {
		// TODO: considering service creation, what should we do here?
		nsLogger.Info("Already deleting namespace, skipping update")
		return false, nil
	}

	serviceName, err := layers.ServiceNameFromNamespaceLabels(ns.Labels)
	if err != nil {
		nsLogger.Debug("Namespace doesn't have a valid service name label, skipping")
		return false, nil
	}

	ctx.Logger.Sugar().Infof("Looking up service data for service %q from Service Central", serviceName)
	serviceData, err := c.getServiceData(auth.NoUser(), string(serviceName))
	if err != nil {
		// should be able to retry SC errors
		return true, err
	}

	operations := []func() (bool, error){
		func() (bool, error) {
			conflict, retriable, err := c.syncPermissions(ctx.Logger, ns, serviceData)
			if conflict {
				return false, nil
			}
			if err != nil {
				return retriable, err
			}
			return false, nil
		},
		func() (bool, error) {
			return c.createOrUpdateServiceMetadata(ctx.Logger, ns, serviceData)
		},
		func() (bool, error) {
			serviceLabel := layers.ServiceLabelFromNamespaceLabels(ns.Labels)
			params := releases.ResolveParams{
				Account:     c.ClusterLocation.Account,
				Region:      c.ClusterLocation.Region,
				Environment: c.ClusterLocation.EnvType,
				ServiceName: serviceName,
				Label:       serviceLabel,
			}
			resolvedRelease, err := c.ReleaseManagement.Resolve(params)
			if err != nil {
				return false, errors.Wrap(err, "failed to sync software release data")
			}
			return c.createOrUpdateReleaseData(ctx.Logger, ns, &resolvedRelease.ResolvedData)
		},
		func() (bool, error) {
			retriable, _, err := c.setupDockerSecret(nsLogger, ns)
			return retriable, err
		},
		func() (bool, error) {
			retriable, _, err := c.createOrUpdateNamespaceAnnotations(ctx.Logger, string(serviceName), ns)
			return retriable, err
		},
		func() (bool, error) {
			return c.ensureCommonSecretExists(ctx.Logger, ns)
		},
	}

	return runOperations(operations)
}

func runOperations(operations []func() (bool, error)) (bool, error) {
	numOps := len(operations)
	retryChan := make(chan bool, numOps)
	errChan := make(chan error, numOps)

	for _, f := range operations {
		go func(fn func() (bool, error)) {
			retry, err := fn()
			// some things return retriable even if they themselves didn't throw an error
			// so we need this additional err != nil in the bool
			retryChan <- err != nil && retry
			errChan <- err
		}(f)
	}

	errs := make([]error, 0, numOps)
	shouldRetry := false
	for i := 0; i < numOps; i++ {
		if err := <-errChan; err != nil {
			errs = append(errs, err)
		}
		shouldRetry = shouldRetry || <-retryChan
	}

	return shouldRetry, utilerrors.NewAggregate(errs)
}

// startPollingServiceCentral asynchronously polls Service Central and processes results.
func (c *Controller) startPollingServiceCentral(ctx context.Context) {
	c.Logger.Sugar().Infof("Starting Service Central polling every %s", serviceCentralPollPeriod)
	// add a bit of offset & jitter so deployments across environments don't spam SC
	time.Sleep(time.Duration(baseDelayProcSec+rand.Intn(baseDelayProcSec)) * time.Second)

	// wait.Until will immediately call the function and poll thereafter
	wait.NonSlidingUntil(c.syncServiceMetadata, serviceCentralPollPeriod, ctx.Done())
	c.Logger.Info("Exiting Service Central polling")
}

// startPollingReleaseManagementSystem asynchronously polls Deployinator and processes results.
func (c *Controller) startPollingReleaseManagementSystem(ctx context.Context) {
	c.Logger.Sugar().Infof("Starting Service Central polling every %s", releaseManagementPollPeriod)
	// add a bit of offset & jitter so deployments across environments don't spam RMS
	time.Sleep(time.Duration(baseDelayProcSec+rand.Intn(baseDelayProcSec)) * time.Second)

	// wait.JitterUntil will call the function and poll thereafter with some jittering
	wait.JitterUntil(c.syncReleasesMetadata, releaseManagementPollPeriod, rmsPollJitterFactor, true, ctx.Done())
	c.Logger.Info("Exiting release management polling")
}

func (c *Controller) syncReleasesMetadata() {
	if c.NextFetchAllReleasesStart == nil || c.LastFetchedAllReleases == nil || time.Since(*c.LastFetchedAllReleases) > releaseManagementSyncAllPeriod {
		// Setting our start time to zero time effectively causes a "full resync" of everything
		c.NextFetchAllReleasesStart = &time.Time{}
	}
	resolvedReleases, nextFetchStart, err := c.ReleaseManagement.ResolveLatest(releases.ResolveBatchParams{
		Account:     c.ClusterLocation.Account,
		Region:      c.ClusterLocation.Region,
		Environment: c.ClusterLocation.EnvType,
		From:        *c.NextFetchAllReleasesStart,
	})
	if err != nil {
		c.Logger.Error("Error retrieving latest release updates", zap.Error(err))
		return
	}
	c.NextFetchAllReleasesStart = &nextFetchStart
	now := time.Now()
	c.LastFetchedAllReleases = &now

	for _, v := range resolvedReleases {
		ns := c.getNamespaceForRelease(&v)
		if ns == nil {
			// Ignore release data available without associated namespace created in this cluster
			continue
		}
		nestedLogger := c.Logger.With(ctrllogz.NamespaceName(ns.Name))
		if _, err := c.createOrUpdateReleaseData(nestedLogger, ns, &v.ResolvedData); err != nil {
			nestedLogger.Error("Failed to create or update release data", zap.Error(err))
		}
	}
}

func (c *Controller) getNamespaceForRelease(release *releases.ResolvedRelease) *core_v1.Namespace {
	nsObj, err := c.NamespaceInformer.GetIndexer().ByIndex(NamespaceByServiceLabelIndexName, ByLabelAndService(voyager.ServiceName(release.ServiceName), release.Label))
	if err != nil {
		c.Logger.Sugar().Error("Failed to retrieve Namespace object", zap.Error(err))
		return nil
	}
	switch len(nsObj) {
	case 0:
		// log and skip this release
		c.Logger.Sugar().Debug("No namespace present for service %q label %q", release.ServiceName, release.Label)
		return nil
	case 1:
		// valid
		return nsObj[0].(*core_v1.Namespace)
	default:
		// unexpected scenario (> 1 interfaces returned in this index?)
		c.Logger.Sugar().Errorf("Unexpected scenario! %d namespaces found for a service and label returned: %v", len(nsObj), nsObj)
		return nil
	}
}

func (c *Controller) syncServiceMetadata() {
	c.Logger.Info("Listing services from service central")
	var services []creator_v1.Service
	var err error
	now := time.Now()
	if c.LastFetchedAllServices == nil || time.Since(*c.LastFetchedAllServices) > serviceCentralListAllPeriod {
		// List ALL Micros2 services over a long period
		services, err = c.ServiceCentral.ListServices(context.Background(), auth.NoUser())
		if err != nil {
			c.Logger.Error("Failed listing services from service central", zap.Error(err))
			c.ServiceCentralPollErrorCounter.Inc()
			return
		}
		c.Logger.Sugar().Infof("ListServices: got %v records back", len(services))
		c.LastFetchedAllServices = &now
		c.LastFetchedModifiedServices = &now // Listing all services includes recently modified ones
	} else {
		// List modified Micros2 services over shorter periods
		modifiedSince := c.LastFetchedModifiedServices.Add(-serviceCentralListDriftCompensation)
		services, err = c.ServiceCentral.ListModifiedServices(context.Background(), auth.NoUser(), modifiedSince)
		if err != nil {
			c.Logger.Error("Failed listing modified services from service central", zap.Error(err))
			c.ServiceCentralPollErrorCounter.Inc()
			return
		}
		c.Logger.Sugar().Infof("ListModifiedServices: got %v records back", len(services))
		c.LastFetchedModifiedServices = &now
	}

	c.Logger.Sugar().Infof("Received %d services from service central", len(services))

	// shoves all the services into a channel. a fixed pool of workers processes
	// this channel until there is nothing left, and then terminates
	svcChan := make(chan creator_v1.Service, len(services))
	for _, service := range services {
		svcChan <- service
	}
	close(svcChan)
	wg := &sync.WaitGroup{}

	wg.Add(maxSyncWorkers)
	for i := 0; i < maxSyncWorkers; i++ {
		go func() {
			defer wg.Done()

			for service := range svcChan {
				// Service Central actually doesn't include misc data so we need to perform
				// additional query to fill out the miscdata for our builds
				fullService, err := c.ServiceCentral.GetService(context.Background(), auth.NoUser(), service.Name)
				if err != nil {
					c.Logger.With(zap.Error(err)).Sugar().Errorf("Error getting full service info for %q", service.Name)
					continue
				}

				err = c.createOrUpdateServiceDescriptorAccess(fullService)
				if err != nil {
					c.Logger.With(zap.Error(err)).Sugar().Errorf("Error creating or updating access for %q", fullService.Name)
					c.AccessUpdateErrorCounter.WithLabelValues(fullService.Name).Inc()
				}
			}
		}()
	}

	wg.Wait()
}

func (c *Controller) createOrUpdateReleaseData(logger *zap.Logger, ns *core_v1.Namespace, rd *releases.ResolvedReleaseData) (bool, error) {
	serviceName, err := layers.ServiceNameFromNamespaceLabels(ns.Labels)
	if err != nil {
		logger.Debug("Namespace doesn't have a valid service name label, skipping")
		return false, nil
	}
	serviceLogger := logger.With(logz.ServiceName(serviceName))

	resolvedTargetsYAML, err := yaml.Marshal(rd)
	if err != nil {
		return false, err
	}

	desired := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      releases.DefaultReleaseMetadataConfigMapName,
			Namespace: ns.Name,
		},
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.ConfigMapKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		Data: map[string]string{
			releases.DataKey: string(resolvedTargetsYAML),
		},
	}

	conflict, retriable, _, err := c.ConfigMapUpdater.CreateOrUpdate(
		serviceLogger,
		func(r runtime.Object) error {
			return nil
		},
		desired,
	)
	if conflict {
		return false, nil
	}
	if err != nil {
		return retriable, err
	}
	return false, nil
}

func (c *Controller) createOrUpdateServiceMetadata(logger *zap.Logger, ns *core_v1.Namespace, serviceData *creator_v1.Service) (bool /* retriable */, error) {
	tags := make(map[voyager.Tag]string, len(serviceData.Spec.ResourceTags))
	for k, v := range serviceData.Spec.ResourceTags {
		tags[k] = v
	}

	notifications, err := c.buildNotifications(serviceData.Spec)
	if err != nil {
		return false, err
	}

	metadata := orch_meta.ServiceProperties{
		ResourceOwner:   serviceData.Spec.ResourceOwner,
		BusinessUnit:    serviceData.Spec.BusinessUnit,
		Notifications:   *notifications,
		LoggingID:       serviceData.Spec.LoggingID,
		SSAMAccessLevel: ssam.AccessLevelNameForEnvType(serviceData.Spec.SSAMContainerName, c.ClusterLocation.EnvType),
		UserTags:        tags,
		Compliance: orch_meta.Compliance{
			PRGBControl: serviceData.Status.Compliance.PRGBControl,
		},
	}
	metaBytes, err := yaml.Marshal(metadata)
	if err != nil {
		return false, errors.Wrap(err, "failed to marshal metadata into YAML")
	}

	cm := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      apisynchronization.DefaultServiceMetadataConfigMapName,
			Namespace: ns.Name,
		},
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.ConfigMapKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		Data: map[string]string{
			orch_meta.ConfigMapConfigKey: string(metaBytes),
		},
	}

	conflict, retriable, _, err := c.ConfigMapUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			return nil
		},
		cm,
	)
	if conflict {
		return false, nil
	}
	if err != nil {
		return retriable, err
	}
	return false, nil
}

func (c *Controller) getServiceData(user auth.OptionalUser, name string) (*creator_v1.Service, error) {
	return c.ServiceCentral.GetService(context.Background(), user, name)
}

func (c *Controller) buildNotifications(spec creator_v1.ServiceSpec) (*orch_meta.Notifications, error) {
	pdEnvMetadata, ok, err := pagerDutyForEnvType(spec.Metadata.PagerDuty, c.ClusterLocation.EnvType)
	if err != nil {
		return nil, errors.Wrap(err, "error building notifications for servicemetadata")
	}

	if ok {
		mainPD, err := convertPagerDuty(pdEnvMetadata.Main)
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert main pagerduty entry")
		}
		lowPriPD, err := convertPagerDuty(pdEnvMetadata.LowPriority)
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert low priority pagerduty entry")
		}

		return &orch_meta.Notifications{
			Email:                        spec.EmailAddress(),
			PagerdutyEndpoint:            *mainPD,
			LowPriorityPagerdutyEndpoint: *lowPriPD,
		}, nil
	}

	// Default values re-used from Micros config.js
	return &orch_meta.Notifications{
		Email: spec.EmailAddress(),
		PagerdutyEndpoint: orch_meta.PagerDuty{
			Generic:    "5d11612f25b840faaf77422edeff9c76",
			CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/124e0f010f214a9b9f30b768e7b18e69",
		},
		LowPriorityPagerdutyEndpoint: orch_meta.PagerDuty{
			Generic:    "5d11612f25b840faaf77422edeff9c76",
			CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/124e0f010f214a9b9f30b768e7b18e69",
		},
	}, nil
}

func (c *Controller) setupDockerSecret(logger *zap.Logger, namespace *core_v1.Namespace) (bool /* retriable */, *core_v1.Secret, error) {
	dockerSecret, err := c.MainClient.CoreV1().Secrets(dockerSecretNamespace).Get(dockerSecretName, meta_v1.GetOptions{})
	if err != nil {
		return true, nil, errors.Wrapf(err, "failed retrieving secret %q in namespace %q", dockerSecretName, dockerSecretNamespace)
	}

	secret := dockerSecret.DeepCopy()

	// Check this secret is actually a docker config
	if secret.Type != core_v1.SecretTypeDockerConfigJson {
		return false, nil, errors.Errorf("secret %q in namespace %q is not a docker config secret", dockerSecretName, dockerSecretNamespace)
	}

	// Set the namespace in the spec to the one we intend to copy to
	secret.Namespace = namespace.Name

	// Reset the UID, owner references and resourceVersion
	secret.UID = ""
	secret.ObjectMeta.OwnerReferences = nil
	secret.ResourceVersion = ""

	return c.createOrUpdateSecret(logger, secret)
}

func (c *Controller) createOrUpdateSecret(logger *zap.Logger, secretSpec *core_v1.Secret) (bool /* retriable */, *core_v1.Secret, error) {
	logger.Sugar().Debugf("Attempting to create or update secret %q", secretSpec.Name)

	// Determine if the secret already exists
	existingSecret, err := c.MainClient.CoreV1().Secrets(secretSpec.Namespace).Get(secretSpec.Name, meta_v1.GetOptions{})
	exists := true
	if err != nil {
		if !api_errors.IsNotFound(err) {
			return true, nil, err
		}
		exists = false
	}

	if exists {
		existingSecret.Type = secretSpec.Type
		existingSecret.Data = secretSpec.Data
		secret, err := c.MainClient.CoreV1().Secrets(secretSpec.Namespace).Update(existingSecret)
		return true, secret, err
	}

	secret, err := c.MainClient.CoreV1().Secrets(secretSpec.Namespace).Create(secretSpec)
	return true, secret, err
}

func (c *Controller) ensureCommonSecretExists(logger *zap.Logger, ns *core_v1.Namespace) (bool /* retriable */, error) {
	_, err := c.MainClient.CoreV1().Secrets(ns.Name).Get(commonSecretName, meta_v1.GetOptions{})

	switch {
	case err == nil:
		// nothing to do, it exists; nothing to update
		logger.Sugar().Debugf("Secret named %q already exists", commonSecretName)
		return false, nil

	case api_errors.IsNotFound(err):
		logger.Sugar().Debugf("Common secret %q does not exist, creating", commonSecretName)
		secret := &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      commonSecretName,
				Namespace: ns.Name,
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind:       k8s.SecretKind,
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
			Type: core_v1.SecretTypeOpaque,
		}

		_, err = c.MainClient.CoreV1().Secrets(ns.Name).Create(secret)
		if err != nil {
			return true, err
		}
		return false, nil

	default:
		// unknown error
		return true, err
	}
}

func (c *Controller) createOrUpdateNamespaceAnnotations(logger *zap.Logger, serviceName string, namespace *core_v1.Namespace) (bool /* retriable */, *core_v1.Namespace, error) {
	// Get the desired value of the annotation
	desiredVal, err := c.getNamespaceAllowedRoles(serviceName)
	if err != nil {
		return true, namespace, err
	}

	// Determine if the annotation needs adding or updating
	val, exists := namespace.Annotations[allowedRolesAnnotation]
	if exists && val == desiredVal {
		return true, namespace, nil
	}

	// Ensure the annotations have been initialised
	if namespace.Annotations == nil {
		namespace.Annotations = make(map[string]string)
	}

	// Add/update the annotation
	namespace.Annotations[allowedRolesAnnotation] = desiredVal
	conflict, retriable, _, err := c.NamespaceUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			return nil
		},
		namespace,
	)
	if conflict {
		return true, namespace, nil
	}
	if err != nil {
		return retriable, namespace, err
	}
	return true, namespace, nil
}

// getNamespaceAllowedRoles returns a JSON encoded string of all the IAM roles pods in a namespace are allowed to assume
func (c *Controller) getNamespaceAllowedRoles(serviceName string) (string, error) {
	roles := []string{
		// We use a wildcard/glob because we don't know what the final role will be called
		generateIamRoleGlob(c.ClusterLocation.Account, serviceName),
	}

	// Kube2iam expects the roles as JSON
	// https://github.com/jtblin/kube2iam/blob/master/namespace.go#L66-L78
	result, err := json.Marshal(roles)
	return string(result), errors.WithStack(err)
}

func generateIamRoleGlob(account voyager.Account, serviceName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/rps-%s-*", account, serviceName)
}

func convertPagerDuty(pdsm creator_v1.PagerDutyServiceMetadata) (*orch_meta.PagerDuty, error) {
	cwURL, err := pagerduty.KeyToCloudWatchURL(pdsm.Integrations.CloudWatch.IntegrationKey)
	if err != nil {
		return nil, err
	}

	return &orch_meta.PagerDuty{
		Generic:    pdsm.Integrations.Generic.IntegrationKey,
		CloudWatch: cwURL,
	}, nil
}

func pagerDutyForEnvType(pagerduty *creator_v1.PagerDutyMetadata, envType voyager.EnvType) (*creator_v1.PagerDutyEnvMetadata, bool, error) {
	if pagerduty == nil {
		return nil, false, errors.New("no pagerduty config present")
	}

	switch envType {
	case voyager.EnvTypeStaging:
		if pagerduty.Staging == (creator_v1.PagerDutyEnvMetadata{}) {
			return nil, false, errors.New("staging pagerduty config is empty but is required")
		}
		return &pagerduty.Staging, true, nil
	case voyager.EnvTypeProduction:
		if pagerduty.Production == (creator_v1.PagerDutyEnvMetadata{}) {
			return nil, false, errors.New("production pagerduty config is empty but is required")
		}
		return &pagerduty.Production, true, nil
	default:
		return nil, false, nil
	}
}

func NsServiceLabelIndexFunc(obj interface{}) ([]string, error) {
	ns := obj.(*core_v1.Namespace)
	serviceName, err := layers.ServiceNameFromNamespaceLabels(ns.Labels)
	if err != nil {
		// Non service namespace
		return nil, nil
	}
	label := layers.ServiceLabelFromNamespaceLabels(ns.Labels)
	return []string{ByLabelAndService(serviceName, label)}, nil
}

func ByLabelAndService(serviceName voyager.ServiceName, label voyager.Label) string {
	return string(serviceName) + ":" + string(label)
}
