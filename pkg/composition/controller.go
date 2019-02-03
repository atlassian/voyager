package composition

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/ctrl/logz"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	compclient "github.com/atlassian/voyager/pkg/composition/client"
	formclient "github.com/atlassian/voyager/pkg/formation/client"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/atlassian/voyager/pkg/util/layers"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
)

const (
	nsServiceNameIndex = "nsServiceNameIndex"

	errTerminal  = "TerminalError"
	errRetriable = "RetriableError"
)

type Controller struct {
	clock        clock.Clock
	logger       *zap.Logger
	readyForWork func()
	namespace    string

	formationClient   formclient.Interface
	compositionClient compclient.Interface
	sdTransformer     SdTransformer
	location          options.Location

	nsUpdater updater.ObjectUpdater
	ldUpdater updater.ObjectUpdater
	ldIndexer cache.Indexer
	nsIndexer cache.Indexer

	serviceDescriptorTransitionsCounter *prometheus.CounterVec
}

type formationObjectResult struct {
	namespace *core_v1.Namespace
	ld        *form_v1.LocationDescriptor
}

func (c *Controller) Run(ctx context.Context) {
	defer c.logger.Info("Shutting down Composition controller")
	c.logger.Info("Starting the Composition controller")
	c.readyForWork()
	<-ctx.Done()
}

func (c *Controller) Process(ctx *ctrl.ProcessContext) (bool /* retriable */, error) {
	sd := ctx.Object.(*comp_v1.ServiceDescriptor)
	serviceName := sd.Name
	logger := ctx.Logger.With(zap.String("sd_name", sd.Name))
	logger.Debug("Started processing ServiceDescriptor")

	// If SD is marked for deletion and SD finalizer is missing, we have nothing to do
	if sd.ObjectMeta.DeletionTimestamp != nil && !hasServiceDescriptorFinalizer(sd) {
		logger.Sugar().Infof("No SD finalizer for %q. Skipping", sd.Name)
		return false, nil
	}

	// If SD finalizer is missing, add it and finish the processing iteration
	if sd.ObjectMeta.DeletionTimestamp == nil && !hasServiceDescriptorFinalizer(sd) {
		sd.Finalizers = addServiceDescriptorFinalizer(sd.GetFinalizers())
		_, retriableStatus, errStatus := c.updateServiceDescriptor(logger, sd)
		if errStatus != nil {
			return retriableStatus, errStatus
		}
	}

	// First filter to filter out the service descriptor if namespace is specified, to avoid
	// treading on compositions trying to handleProcessResult and overwriting each other.
	if c.namespace != "" {
		nsServiceName, _ := deconstructNamespaceName(c.namespace)
		if serviceName != nsServiceName {
			logger.Sugar().Infof("Namespace %q can't possibly be created by SD %q. Skipping.", c.namespace, sd.Name)
			return false, nil
		}
	}

	formationObjectDefList, err := c.sdTransformer.CreateFormationObjectDef(sd)
	foResults := make([]formationObjectResult, 0, len(formationObjectDefList))
	retriable := false

	if err != nil {
		conflict, processRetriable, processErr := c.handleProcessResult(ctx.Logger, serviceName, sd, foResults, false, retriable, err)
		if conflict {
			return false, nil
		}

		return processRetriable, processErr
	}

	deleteFinished := true
	for _, formationObjectDef := range formationObjectDefList {
		formLogger := logger.With(logz.NamespaceName(formationObjectDef.Namespace))
		formLogger.Debug("Processing LocationDescriptor for ServiceDescriptor")

		// filter the formationObjectDefList so we only handle the namespace we know
		if formationObjectDef.Namespace != c.namespace && c.namespace != "" {
			formLogger.Sugar().Infof("Namespace %q doesn't match namespace %q observed by the controller. Skipping formationObjectDef %v.",
				formationObjectDef.Namespace, c.namespace, formationObjectDef.Location.Label)
			continue
		}

		finished, conflict, processRetriable, foResult, processErr := c.processFormationObjectDef(formLogger, sd, &formationObjectDef)
		if conflict {
			// Something ran into a conflict during processing
			// The entire ServiceDescriptor will be processed anyway so we just exit
			return false, nil
		}
		if processErr != nil {
			// stop the presses
			deleteFinished = false
			err = processErr
			retriable = processRetriable
			break
		}
		if sd.DeletionTimestamp != nil {
			deleteFinished = deleteFinished && finished
		}

		if foResult != nil {
			// Save the results from processing the location descriptor. We'll
			// handle the result after this loop
			foResults = append(foResults, *foResult)
		}
	}

	conflict, retriable, err := c.handleProcessResult(ctx.Logger, serviceName, sd, foResults, deleteFinished, retriable, err)
	if conflict {
		return false, nil
	}
	if err != nil {
		return retriable, err
	}

	return false, nil
}

func (c *Controller) processFormationObjectDef(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, formationObjectDef *FormationObjectInfo) (bool /* finished */, bool /* conflict */, bool /* retriable */, *formationObjectResult, error) {
	if sd.ObjectMeta.DeletionTimestamp != nil {
		return c.processDeleteFormationObjectDef(logger, sd, formationObjectDef)
	}

	conflict, retriable, foResult, err := c.processNormalFormationObjectDef(logger, sd, formationObjectDef)
	return false, conflict, retriable, foResult, err
}

func (c *Controller) processNormalFormationObjectDef(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, formationObjectDef *FormationObjectInfo) (bool /* conflict */, bool /* retriable */, *formationObjectResult, error) {
	conflict, retriable, ns, err := c.createOrUpdateNamespace(logger, formationObjectDef, sd)
	if err != nil || conflict {
		return conflict, retriable, nil, err
	}

	conflict, retriable, ld, err := c.createOrUpdateLocationDescriptor(logger, formationObjectDef)
	if err != nil || conflict {
		return conflict, retriable, nil, err
	}

	return false, false, &formationObjectResult{namespace: ns, ld: ld}, nil
}

func (c *Controller) processDeleteFormationObjectDef(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, formationObjectDef *FormationObjectInfo) (bool /* finished */, bool /* conflict */, bool /* retriable */, *formationObjectResult, error) {
	// Delete LocationDescriptor with Foreground policy first
	conflict, retriable, ld, err := c.deleteLocationDescriptor(logger, formationObjectDef)
	if err != nil {
		return false, conflict, retriable, nil, err
	}
	if ld != nil {
		// We need to reprocess SD once LD has been deleted
		ns, exists, err := c.nsIndexer.GetByKey(ld.Namespace)
		if err != nil {
			return false, false, false, nil, errors.WithStack(err)
		}
		if !exists {
			return false, false, false, nil, errors.Errorf("unexpectedly can't find Namespace %q", ld.Namespace)
		}
		return false, false, false, &formationObjectResult{namespace: ns.(*core_v1.Namespace), ld: ld}, nil
	}

	// Once LocationDescriptor is gone, we don't need to propagate status anymore, delete namespace
	conflict, retriable, ns, err := c.deleteNamespace(logger, formationObjectDef.Namespace)
	if err != nil {
		return false, conflict, retriable, nil, err
	}
	if ns != nil {
		// We need to reprocess SD once namespace has been deleted
		return false, false, false, nil, nil
	}

	// We return the "finished" flag to distinguish this case from the ones above
	// if we got to this point, it means we have cleaned up everything for this LD
	return true, false, false, nil, nil
}

func (c *Controller) handleProcessResult(logger *zap.Logger, serviceName string, sd *comp_v1.ServiceDescriptor, foResults []formationObjectResult, deleteFinished bool, retriable bool, err error) (bool /* conflict */, bool /* retriable */, error) {
	logger.Debug("Handling results of processing")

	inProgressCond := cond_v1.Condition{
		Type:   cond_v1.ConditionInProgress,
		Status: cond_v1.ConditionFalse,
	}
	readyCond := cond_v1.Condition{
		Type:   cond_v1.ConditionReady,
		Status: cond_v1.ConditionFalse,
	}
	errorCond := cond_v1.Condition{
		Type:   cond_v1.ConditionError,
		Status: cond_v1.ConditionFalse,
	}
	locationStatuses := sd.Status.LocationStatuses

	finalizersUpdated := false
	if err != nil {
		errorCond.Status = cond_v1.ConditionTrue
		errorCond.Message = err.Error()
		if retriable {
			errorCond.Reason = errRetriable
			inProgressCond.Status = cond_v1.ConditionTrue
		} else {
			errorCond.Reason = errTerminal
		}
	} else {
		locationStatuses, err = c.calculateLocationStatuses(serviceName, foResults, retriable)
		if err != nil { // Super-unexpected; don't bother recovering.
			return false, false, errors.WithStack(err)
		}

		inProgressConditions := filterConditionsByType(locationStatuses, cond_v1.ConditionInProgress)
		readyConditions := filterConditionsByType(locationStatuses, cond_v1.ConditionReady)
		errorConditions := filterConditionsByType(locationStatuses, cond_v1.ConditionError)

		// Calculate the ServiceDescriptor conditions from the LocationDescriptor conditions
		if len(locationStatuses) == 0 {
			readyCond.Status = cond_v1.ConditionTrue
			readyCond.Message = "No locations matching cluster location, nothing to process"
		} else {
			if len(inProgressConditions) == 1 {
				inProgressConditions[0].DeepCopyInto(&inProgressCond)
			} else {
				inProgressCond = cond_v1.Condition{
					Status: k8s.CalculateConditionAny(inProgressConditions),
					Type:   cond_v1.ConditionInProgress,
				}
			}
			if len(readyConditions) == 1 {
				readyConditions[0].DeepCopyInto(&readyCond)
			} else {
				readyCond = cond_v1.Condition{
					Status: k8s.CalculateConditionAll(readyConditions),
					Type:   cond_v1.ConditionReady,
				}
			}
			if len(errorConditions) == 1 {
				errorConditions[0].DeepCopyInto(&errorCond)
			} else {
				// grab locations in the error state
				var msg string
				status := k8s.CalculateConditionAny(errorConditions)
				if status == cond_v1.ConditionTrue {
					var locations []voyager.Location
					for _, locationStatus := range locationStatuses {
						_, found := cond_v1.FindCondition(locationStatus.Conditions, cond_v1.ConditionError)
						if found != nil && found.Status == cond_v1.ConditionTrue {
							locations = append(locations, locationStatus.Location)
						}
					}
					if len(locations) > 0 {
						msg = fmt.Sprintf("error processing location(s): %q", locations)
					}
				}

				errorCond = cond_v1.Condition{
					Status:  status,
					Type:    cond_v1.ConditionError,
					Message: msg,
				}
			}
		}

		// If deletion is complete, remove finalizer
		if sd.ObjectMeta.DeletionTimestamp != nil && deleteFinished {
			sd.Finalizers = removeServiceDescriptorFinalizer(sd.GetFinalizers())
			finalizersUpdated = true
		}
	}

	inProgressUpdated := c.updateCondition(sd, inProgressCond)
	readyUpdated := c.updateCondition(sd, readyCond)
	errorUpdated := c.updateCondition(sd, errorCond)
	resourcesUpdated := c.updateLocationStatuses(sd, locationStatuses)

	if inProgressUpdated || readyUpdated || errorUpdated || resourcesUpdated || finalizersUpdated {
		conflictStatus, retriableStatus, errStatus := c.updateServiceDescriptor(logger, sd)
		if errStatus != nil {
			if err != nil {
				logger.Info("Failed to set ServiceDescriptor status", zap.Error(errStatus))
				return false, retriableStatus || retriable, err
			}
			return false, retriableStatus, errStatus
		}
		if conflictStatus {
			return true, false, nil
		}
	}

	return false, retriable, err
}

func filterConditionsByType(statuses []comp_v1.LocationStatus, condType cond_v1.ConditionType) []cond_v1.Condition {
	conditions := []cond_v1.Condition{}

	for _, status := range statuses {
		_, found := cond_v1.FindCondition(status.Conditions, condType)
		if found != nil {
			conditions = append(conditions, *found)
		}
	}

	return conditions
}

func copyCondition(ld *form_v1.LocationDescriptor, condType cond_v1.ConditionType, cond *cond_v1.Condition) {
	_, ldCond := cond_v1.FindCondition(ld.Status.Conditions, condType)

	if ldCond == nil {
		cond.Status = cond_v1.ConditionUnknown
		cond.Reason = "FormationInteropError"
		cond.Message = "Formation not reporting state for this condition"
		return
	}

	cond.Reason = ldCond.Reason
	cond.Message = ldCond.Message
	switch ldCond.Status {
	case cond_v1.ConditionTrue:
		cond.Status = cond_v1.ConditionTrue
	case cond_v1.ConditionUnknown:
		cond.Status = cond_v1.ConditionUnknown
	case cond_v1.ConditionFalse:
		cond.Status = cond_v1.ConditionFalse
	default:
		cond.Status = cond_v1.ConditionUnknown
		cond.Reason = "FormationInteropError"
		cond.Message = fmt.Sprintf("Unexpected ConditionStatus %q", ldCond.Status)
	}

	cond.LastTransitionTime = ldCond.LastTransitionTime
}

func (c *Controller) calculateLocationStatuses(serviceName string, results []formationObjectResult, retriable bool) ([]comp_v1.LocationStatus, error) {
	// Make sure to collect Lds that we aren't touching as well
	// (sd should show status of _everything_ it is responsible for, even
	// if not currently referenced).
	// We do a two stage lookup here (rather than having a direct
	// serviceName -> ld index) because when we insert things into
	// the index we may not have information about the namespace yet (cache
	// might be out of date), and we need the namespace because it's the source
	// of truth about our servicename; we don't want to have an admission controller
	// on LocationDescriptor to stop creation of LocationDescriptors with the wrong
	// serviceName label or similar...
	// We could find the servicename by deconstructing the namespace name
	// (split on '--'), but that solution was voted of the architectural purity island.
	nsList, err := c.nsIndexer.ByIndex(nsServiceNameIndex, serviceName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// i.e. all results ever
	allResults := make(map[string]formationObjectResult)
	for _, ns := range nsList {
		namespace := ns.(*core_v1.Namespace)
		existingLds, err := c.ldIndexer.ByIndex(cache.NamespaceIndex, namespace.Name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// we only expect one LD here, but...
		for _, objLd := range existingLds {
			existingLd := objLd.(*form_v1.LocationDescriptor)
			allResults[ldKeyOf(existingLd)] = formationObjectResult{
				namespace: namespace,
				ld:        existingLd,
			}
		}
	}
	// Replace anything from the indexer with what we know
	for _, result := range results {
		allResults[ldKeyOf(result.ld)] = result
	}

	locationStatuses := make([]comp_v1.LocationStatus, 0, len(allResults))
	// Now we need to sort things so we have a stable output
	keys := make([]string, 0, len(allResults))
	for k := range allResults {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		result := allResults[key]
		ld := result.ld
		label := layers.ServiceLabelFromNamespaceLabels(result.namespace.Labels)
		location := c.location.ClusterLocation().Location(label)
		inProgressCond := cond_v1.Condition{
			Type:   cond_v1.ConditionInProgress,
			Status: cond_v1.ConditionFalse,
		}
		readyCond := cond_v1.Condition{
			Type:   cond_v1.ConditionReady,
			Status: cond_v1.ConditionFalse,
		}
		errorCond := cond_v1.Condition{
			Type:   cond_v1.ConditionError,
			Status: cond_v1.ConditionFalse,
		}

		// Check if LocationDescriptor has a status at all. If not, it has no
		// conditions, append empty to all.
		if len(ld.Status.Conditions) == 0 {
			locationStatuses = append(locationStatuses, comp_v1.LocationStatus{
				DescriptorName:      ld.Name,
				DescriptorNamespace: ld.Namespace,
				Location:            location,
				Conditions:          []cond_v1.Condition{inProgressCond, readyCond, errorCond},
			})
			continue
		}

		copyCondition(ld, cond_v1.ConditionInProgress, &inProgressCond)
		copyCondition(ld, cond_v1.ConditionReady, &readyCond)
		copyCondition(ld, cond_v1.ConditionError, &errorCond)

		locationStatuses = append(locationStatuses, comp_v1.LocationStatus{
			DescriptorName:      ld.Name,
			DescriptorNamespace: ld.Namespace,
			Location:            location,
			Conditions:          []cond_v1.Condition{inProgressCond, readyCond, errorCond},
		})
	}

	return locationStatuses, nil
}

func (c *Controller) updateCondition(sd *comp_v1.ServiceDescriptor, condition cond_v1.Condition) bool /* updated */ {
	var needsUpdate bool
	i, oldCondition := cond_v1.FindCondition(sd.Status.Conditions, condition.Type)
	needsUpdate = k8s.FillCondition(c.clock, oldCondition, &condition)

	if needsUpdate {
		if i == -1 {
			sd.Status.Conditions = append(sd.Status.Conditions, condition)
		} else {
			sd.Status.Conditions[i] = condition
		}
		if condition.Status == cond_v1.ConditionTrue {
			c.serviceDescriptorTransitionsCounter.
				WithLabelValues(sd.Name, string(condition.Type), condition.Reason).
				Inc()
		}
		return true
	}

	return false
}

func (c *Controller) updateServiceDescriptor(logger *zap.Logger, sd *comp_v1.ServiceDescriptor) (bool /* conflict */, bool /* retriable */, error) {
	logger.Info("Writing status")
	_, err := c.compositionClient.CompositionV1().ServiceDescriptors().Update(sd)
	if err != nil {
		if api_errors.IsConflict(err) {
			return true, false, nil
		}
		return false, true, errors.Wrap(err, "failed to update ServiceDescriptor")
	}
	return false, false, nil
}

func (c *Controller) updateLocationStatuses(sd *comp_v1.ServiceDescriptor, locationStatuses []comp_v1.LocationStatus) bool /* updated */ {
	// this changes the structure of the existing LocationStatus list in the SD to form a map of name -> locationStatus.
	existing := sd.Status.LocationStatuses
	nameToLocationStatus := make(map[string]*comp_v1.LocationStatus, len(existing))
	for i := range existing {
		if existing[i].DescriptorName == "" {
			// this is an invalid location status OR an old one.
			// For now, we continue. In the future, VYGR-283
			continue
		}
		nameToLocationStatus[ldKeyFromStatus(&existing[i])] = &existing[i]
	}

	// for each of the new locationStatus, dig into the map to see if it exists already.
	// then perform an update.
	var newStatuses []comp_v1.LocationStatus
	var changed bool
	for _, locationStatus := range locationStatuses {
		existingLocationStatus, hasExistingStatus := nameToLocationStatus[ldKeyFromStatus(&locationStatus)]
		if hasExistingStatus {
			changed = k8s.FillNewConditions(c.clock, existingLocationStatus.Conditions, locationStatus.Conditions) || changed
		} else {
			changed = k8s.FillNewConditions(c.clock, nil, locationStatus.Conditions) || changed
		}

		newStatuses = append(newStatuses, locationStatus)
	}

	if changed {
		sd.Status.LocationStatuses = newStatuses
		return true
	}

	return false
}

func (c *Controller) createOrUpdateNamespace(logger *zap.Logger, fo *FormationObjectInfo, parent *comp_v1.ServiceDescriptor) (bool /* conflict */, bool /* retriable */, *core_v1.Namespace, error) {
	// Ideally name would be a voyager.ServiceName, but we don't have that in the SD
	logger.Sugar().Debugf("Attempting to create or update namespace %q", fo.Namespace)

	nsSpec := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       k8s.NamespaceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: fo.Namespace,
			OwnerReferences: []meta_v1.OwnerReference{
				sdOwnerReference(parent),
			},
			Labels: map[string]string{
				voyager.ServiceNameLabel:  string(fo.ServiceName),
				voyager.ServiceLabelLabel: string(fo.Location.Label),
			},
		},
	}

	conflict, retriable, obj, err := c.nsUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			meta := r.(meta_v1.Object)
			if !meta_v1.IsControlledBy(meta, parent) {
				return errors.Errorf("namespace %q is not owned by service descriptor %q", meta.GetName(), parent.GetName())
			}
			return nil
		},
		nsSpec,
	)

	var ns *core_v1.Namespace
	if obj != nil {
		ns = obj.(*core_v1.Namespace)
	}
	return conflict, retriable, ns, err
}

func (c *Controller) deleteNamespace(logger *zap.Logger, nsName string) (bool /* conflict  */, bool /* retriable */, *core_v1.Namespace, error) {
	logger.Sugar().Debugf("Attempting to delete namespace %q", nsName)

	conflict, retriable, obj, err := c.ldUpdater.DeleteAndGet(logger, "", nsName)

	var ns *core_v1.Namespace
	if obj != nil {
		ns = obj.(*core_v1.Namespace)
	}

	return conflict, retriable, ns, err
}

func (c *Controller) createOrUpdateLocationDescriptor(logger *zap.Logger, fo *FormationObjectInfo) (bool /* conflict */, bool /* retriable */, *form_v1.LocationDescriptor, error) {
	logger.Sugar().Infof("Ensuring location descriptor object for namespace %q is present", fo.Namespace)

	spec := form_v1.LocationDescriptorSpec{
		ConfigMapName: apisynchronization.DefaultServiceMetadataConfigMapName,
		ConfigMapNames: form_v1.LocationDescriptorConfigMapNames{
			Release: releases.DefaultReleaseMetadataConfigMapName,
		},
		Resources: convertToLocationDescriptorResources(fo.Resources),
	}

	ldSpec := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: form_v1.LocationDescriptorResourceAPIVersion,
			Kind:       form_v1.LocationDescriptorResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      fo.Name,
			Namespace: fo.Namespace,
		},
		Spec: spec,
	}

	conflict, retriable, obj, err := c.ldUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			// The namespace is only successfully created or updated after a
			// precondition check that ensures the ServiceDescriptor owns the
			// Namespace. In processNormalFormationObjectDef, we check the namespace
			// has been created or updated first, before calling this function.
			return nil
		},
		ldSpec,
	)

	var ld *form_v1.LocationDescriptor
	if obj != nil {
		ld = obj.(*form_v1.LocationDescriptor)
	}
	return conflict, retriable, ld, err
}

func (c *Controller) deleteLocationDescriptor(logger *zap.Logger, fo *FormationObjectInfo) (bool /* conflict */, bool /* retriable */, *form_v1.LocationDescriptor, error) {
	logger.Sugar().Infof("Deleting location descriptor object for namespace %q", fo.Namespace)

	conflict, retriable, obj, err := c.ldUpdater.DeleteAndGet(logger, fo.Namespace, fo.Name)

	var ld *form_v1.LocationDescriptor
	if obj != nil {
		ld = obj.(*form_v1.LocationDescriptor)
	}

	return conflict, retriable, ld, err
}

func convertToLocationDescriptorResources(resources []comp_v1.ServiceDescriptorResource) []form_v1.LocationDescriptorResource {
	frs := make([]form_v1.LocationDescriptorResource, 0, len(resources))

	// we assume any inapplicable resourceGroups have previously been filtered out
	for _, sr := range resources {
		fdeps := make([]form_v1.LocationDescriptorDependency, 0, len(sr.DependsOn))
		for _, sdep := range sr.DependsOn {
			fdeps = append(fdeps, form_v1.LocationDescriptorDependency{Name: sdep.Name, Attributes: sdep.Attributes})
		}

		fr := form_v1.LocationDescriptorResource{
			Name:      sr.Name,
			Type:      sr.Type,
			Spec:      sr.Spec,
			DependsOn: fdeps,
		}
		frs = append(frs, fr)
	}

	return frs
}

func generateNamespaceName(serviceName string, label voyager.Label) string {
	if label == "" {
		return serviceName
	}
	return fmt.Sprintf("%s--%s", serviceName, label)
}

func generateLdName(serviceName string, label voyager.Label) string {
	// This is currently identical to the namespace name.
	return generateNamespaceName(serviceName, label)
}

// Avoid using this. Prefer instead to lookup the serviceName/label on the namespace.
func deconstructNamespaceName(namespaceName string) (string, voyager.Label) {
	split := strings.SplitN(namespaceName, "--", 2)
	label := ""
	if len(split) > 1 {
		label = split[1]
	}

	return split[0], voyager.Label(label)
}

func sdOwnerReference(sd *comp_v1.ServiceDescriptor) meta_v1.OwnerReference {
	trueVar := true
	return meta_v1.OwnerReference{
		APIVersion:         sd.APIVersion,
		Kind:               sd.Kind,
		Name:               sd.Name,
		UID:                sd.UID,
		Controller:         &trueVar,
		BlockOwnerDeletion: &trueVar,
	}
}

func nsServiceNameIndexFunc(obj interface{}) ([]string, error) {
	// Look, we could look up the owner here and find it's the SD, but we're
	// trying to pick up any random orphans (i.e. maybe the SD was recreated and
	// has a new UUID, or something weird has happened, or the user has created
	// LDs in the namespace that aren't owned by the SD _but_ since the namespace
	// was created by the SD it's reasonable to report them), so we fall back on
	// our string lookups.
	ns := obj.(*core_v1.Namespace)
	serviceName, err := layers.ServiceNameFromNamespaceLabels(ns.Labels)
	if err != nil {
		// Non service namespace
		return nil, nil
	}

	return []string{string(serviceName)}, nil
}

func ldKey(namespace string, name string) string {
	// using a space as the separator here makes sure, when we sort the output
	// keys, that the ones with no label on their namespace come after
	// the ones with a label (i.e. 'basic basic' comes before
	// 'basic--pdev basic--pdev')
	return fmt.Sprintf("%s %s", namespace, name)
}

func ldKeyFromStatus(ls *comp_v1.LocationStatus) string {
	return ldKey(ls.DescriptorNamespace, ls.DescriptorName)
}

func ldKeyOf(ld *form_v1.LocationDescriptor) string {
	return ldKey(ld.Namespace, ld.Name)
}
