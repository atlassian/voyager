package composition

import (
	"fmt"
	"sort"

	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/ctrl/logz"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/atlassian/voyager/pkg/util/layers"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
)

func Locations(fOr []formationObjectResult) []form_v1.LocationDescriptor {
	var result []form_v1.LocationDescriptor
	for _, fo := range fOr {
		result = append(result, *fo.ld)
	}
	return result
}

func Namespaces(fOr []formationObjectResult) []core_v1.Namespace {
	var result []core_v1.Namespace
	for _, fo := range fOr {
		result = append(result, *fo.namespace)
	}
	return result
}

func (c *Controller) Process2(ctx *ctrl.ProcessContext) (bool /* retriable */, error) {
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

	filteredTransformer := &FilteredTransformer{
		Transformer: c.sdTransformer,
		Remove: func(info FormationObjectInfo) bool {
			formLogger := logger.With(logz.NamespaceName(info.Namespace))
			formLogger.Debug("Processing LocationDescriptor for ServiceDescriptor")
			// filter the formationObjectDefList so we only handle the namespace we know
			if info.Namespace != c.namespace && c.namespace != "" {
				formLogger.Sugar().Infof("Namespace %q doesn't match namespace %q observed by the controller. Skipping formationObjectDef %v.",
					info.Namespace, c.namespace, info.Location.Label)
				return true
			}
			return false
		},
	}

	formationObjectDefList, err := ProcessSD(sd, filteredTransformer)

	foResults := make([]formationObjectResult, 0, len(formationObjectDefList))

	if err != nil {
		conflict, processRetryable, processErr := c.handleProcessResult(logger, serviceName, sd, foResults, false, false, err)
		if conflict {
			return false, nil
		}

		return processRetryable, processErr
	}

	var (
		results        []*formationObjectResult
		retryable      = false
		deleteFinished = true
		processFailure error
	)

	for _, result := range foResults {
		formLogger := logger.With(logz.NamespaceName(result.namespace.Namespace))
		formLogger.Debug("Processing LocationDescriptor for ServiceDescriptor")

		var (
			newResult        *formationObjectResult
			processConflict  bool
			processRetryable bool
			processErr       error
		)
		if sd.ObjectMeta.DeletionTimestamp != nil {
			var itemFinished bool
			itemFinished, processConflict, processRetryable, newResult, processErr = c.delete(logger, sd, &result)
			deleteFinished = deleteFinished && itemFinished
		} else {
			processConflict, processRetryable, newResult, processErr = c.processNormalFormationObjectDef2(formLogger, sd, &result)
		}

		if processConflict {
			// Something ran into a conflict during processing
			// The entire ServiceDescriptor will be processed anyway so we just exit
			return false, nil
		}

		if processErr != nil {
			// stop the presses
			deleteFinished = false
			processFailure = processErr
			retryable = processRetryable
			break
		}
		results = append(results, newResult)
	}

	// If deletion is complete, remove finalizer
	if sd.ObjectMeta.DeletionTimestamp != nil && deleteFinished {
		sd.Finalizers = removeServiceDescriptorFinalizer(sd.GetFinalizers())
		deletionDuration := c.clock.Now().Sub(sd.ObjectMeta.DeletionTimestamp.Time)
		logger.Info("ServiceDescriptor deleted", zap.Duration("deletionDuration", deletionDuration))
	}

	nsList, err := c.nsIndexer.ByIndex(nsServiceNameIndex, serviceName)
	if err != nil {
		return false, errors.WithStack(err)
	}
	var existingNamespaces []*core_v1.Namespace
	for _, item := range nsList {
		existingNamespaces = append(existingNamespaces, item.(*core_v1.Namespace))
	}

	existingLD := make(map[*core_v1.Namespace][]*form_v1.LocationDescriptor)
	for _, ns := range existingNamespaces {
		existingLds, err := c.ldIndexer.ByIndex(cache.NamespaceIndex, ns.Name)
		if err != nil {
			return false, errors.WithStack(err)
		}

		for _, obj := range existingLds {
			existingLD[ns] = append(existingLD[ns], obj.(*form_v1.LocationDescriptor))
		}
	}

	st := statusUpdater{
		clock:              c.clock,
		existingNamespaces: existingNamespaces,
		existingLD:         existingLD,
		location:           c.location,

		conditionUpdate: func(name string, condition cond_v1.Condition) {
			c.serviceDescriptorTransitionsCounter.
				WithLabelValues(sd.Name, string(condition.Type), condition.Reason).
				Inc()
		},
	}

	logger.Debug("Handling results of processing")
	sd, updated, err := st.handleProcessResult2(serviceName, sd, foResults, retryable, err)
	if err != nil {
		return retryable, err
	}
	if updated {
		conflict, retryable, err := c.writeSDUpdate(logger, sd, retryable, processFailure)
		if conflict {
			return false, nil
		}
		if err != nil {
			return retryable, err
		}
	}

	return false, nil
}

func ProcessSD(sd *comp_v1.ServiceDescriptor, transformer SdTransformer) ([]formationObjectResult, error) { // nolint
	if sd.ObjectMeta.DeletionTimestamp != nil {
		return nil, nil
	}

	infos, err := transformer.CreateFormationObjectDef(sd)
	if err != nil {
		return nil, err
	}

	var results []formationObjectResult

	for _, formationObjectDef := range infos {
		namespace := createNamespace(&formationObjectDef, sd)
		ld := createLD(&formationObjectDef)
		results = append(results, formationObjectResult{
			ld:        ld,
			namespace: namespace,
		})
	}

	return results, nil
}

func (c *Controller) deleteLD(logger *zap.Logger, result *formationObjectResult) (bool /* conflict */, bool /* retriable */, *form_v1.LocationDescriptor, error) {
	logger.Sugar().Infof("Deleting location descriptor object for namespace %q", result.ld.Namespace)

	conflict, retriable, obj, err := c.ldUpdater.DeleteAndGet(logger, result.ld.Namespace, result.ld.Namespace)

	var ld *form_v1.LocationDescriptor
	if obj != nil {
		ld = obj.(*form_v1.LocationDescriptor)
	}

	return conflict, retriable, ld, err
}

func (c *Controller) delete(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, formationObjectDef *formationObjectResult) (bool /* finished */, bool /* conflict */, bool /* retriable */, *formationObjectResult, error) {
	// Delete LocationDescriptor with Foreground policy first
	conflict, retriable, ld, err := c.deleteLocationDescriptor2(logger, formationObjectDef)
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
	logger.Sugar().Debugf("LocationDescriptor deleted, marking namespace %q for deletion", formationObjectDef.namespace.Namespace)
	conflict, retriable, ns, err := c.deleteNamespace(logger, formationObjectDef.namespace.Namespace)
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

func (c *Controller) deleteLocationDescriptor2(logger *zap.Logger, fo *formationObjectResult) (bool /* conflict */, bool /* retriable */, *form_v1.LocationDescriptor, error) {
	logger.Sugar().Infof("Deleting location descriptor object for namespace %q", fo.ld.Namespace)

	conflict, retriable, obj, err := c.ldUpdater.DeleteAndGet(logger, fo.ld.Namespace, fo.ld.Name)

	var ld *form_v1.LocationDescriptor
	if obj != nil {
		ld = obj.(*form_v1.LocationDescriptor)
	}

	return conflict, retriable, ld, err
}

func (c *Controller) processNormalFormationObjectDef2(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, formationObjectDef *formationObjectResult) (bool /* conflict */, bool /* retriable */, *formationObjectResult, error) {
	conflict, retriable, ns, err := c.createOrUpdateNamespace2(logger, formationObjectDef.namespace, sd)
	if err != nil || conflict {
		return conflict, retriable, nil, err
	}

	conflict, retriable, ld, err := c.createOrUpdateLocationDescriptor2(logger, formationObjectDef.ld)
	if err != nil || conflict {
		return conflict, retriable, nil, err
	}

	return false, false, &formationObjectResult{namespace: ns, ld: ld}, nil
}

func (c *Controller) createOrUpdateNamespace2(logger *zap.Logger, namespace *core_v1.Namespace, parent *comp_v1.ServiceDescriptor) (bool /* conflict */, bool /* retriable */, *core_v1.Namespace, error) {
	// Ideally name would be a voyager.ServiceName, but we don't have that in the SD
	logger.Sugar().Debugf("Attempting to create or update namespace %q", namespace.Name)

	conflict, retriable, obj, err := c.nsUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			meta := r.(meta_v1.Object)
			if !meta_v1.IsControlledBy(meta, parent) {
				return errors.Errorf("namespace %q is not owned by service descriptor %q", meta.GetName(), parent.GetName())
			}
			return nil
		},
		namespace,
	)

	var ns *core_v1.Namespace
	if obj != nil {
		ns = obj.(*core_v1.Namespace)
	}
	return conflict, retriable, ns, err
}

func (c *Controller) createOrUpdateLocationDescriptor2(logger *zap.Logger, locationDescriptor *form_v1.LocationDescriptor) (bool /* conflict */, bool /* retriable */, *form_v1.LocationDescriptor, error) {
	logger.Sugar().Infof("Ensuring location descriptor object for namespace %q is present", locationDescriptor.Namespace)

	conflict, retriable, obj, err := c.ldUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			// TODO this is either a bug not checking an owner reference of either the namespace or the sd or change comment to "namespace owns everything no reference check needed"
			return nil
		},
		locationDescriptor,
	)

	var ld *form_v1.LocationDescriptor
	if obj != nil {
		ld = obj.(*form_v1.LocationDescriptor)
	}
	return conflict, retriable, ld, err
}

func createNamespace(fo *FormationObjectInfo, parent *comp_v1.ServiceDescriptor) *core_v1.Namespace {
	return &core_v1.Namespace{
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
}

func createLD(fo *FormationObjectInfo) *form_v1.LocationDescriptor {

	spec := form_v1.LocationDescriptorSpec{
		ConfigMapName: apisynchronization.DefaultServiceMetadataConfigMapName,
		ConfigMapNames: form_v1.LocationDescriptorConfigMapNames{
			Release: releases.DefaultReleaseMetadataConfigMapName,
		},
		Resources: convertToLocationDescriptorResources(fo.Resources),
	}

	return &form_v1.LocationDescriptor{
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
}

type statusUpdater struct {
	clock              clock.Clock
	existingNamespaces []*core_v1.Namespace
	existingLD         map[*core_v1.Namespace][]*form_v1.LocationDescriptor
	location           options.Location
	conditionUpdate    func(name string, condition cond_v1.Condition)
}

func (c *statusUpdater) handleProcessResult2(serviceName string, sd *comp_v1.ServiceDescriptor, foResults []formationObjectResult, retryable bool, err error) (*comp_v1.ServiceDescriptor, bool /* updated */, error) {
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

	if err != nil {
		errorCond.Status = cond_v1.ConditionTrue
		errorCond.Message = err.Error()
		if retryable {
			errorCond.Reason = errRetriable
			inProgressCond.Status = cond_v1.ConditionTrue
		} else {
			errorCond.Reason = errTerminal
		}
	} else {
		locationStatuses, err = c.calculateLocationStatuses2(serviceName, foResults)
		if err != nil { // Super-unexpected; don't bother recovering.
			return nil, false, errors.WithStack(err)
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
	}

	inProgressUpdated := c.updateCondition(sd, inProgressCond)
	readyUpdated := c.updateCondition(sd, readyCond)
	errorUpdated := c.updateCondition(sd, errorCond)
	resourcesUpdated := c.updateLocationStatuses(sd, locationStatuses)

	return sd, inProgressUpdated || readyUpdated || errorUpdated || resourcesUpdated, nil
}

func (c *Controller) writeSDUpdate(logger *zap.Logger, sd *comp_v1.ServiceDescriptor, retryable bool, err error) (bool /* conflict */, bool /* retriable */, error) {
	conflictStatus, retriableStatus, errStatus := c.updateServiceDescriptor(logger, sd)
	if errStatus != nil {
		if err != nil {
			logger.Info("Failed to set ServiceDescriptor status", zap.Error(errStatus))
			return false, retriableStatus || retryable, err
		}
		return false, retriableStatus, errStatus
	}
	if conflictStatus {
		return true, false, nil
	}

	return false, false, nil
}

func (c *statusUpdater) calculateLocationStatuses2(serviceName string, results []formationObjectResult) ([]comp_v1.LocationStatus, error) {
	// i.e. all results ever
	allResults := make(map[string]formationObjectResult)
	for _, namespace := range c.existingNamespaces {
		existingLds := c.existingLD[namespace]
		// we only expect one LD here, but...
		for _, existingLd := range existingLds {
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

func (c *statusUpdater) updateLocationStatuses(sd *comp_v1.ServiceDescriptor, locationStatuses []comp_v1.LocationStatus) bool /* updated */ {
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

func (c *statusUpdater) updateCondition(sd *comp_v1.ServiceDescriptor, condition cond_v1.Condition) bool /* updated */ {
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
			c.conditionUpdate(sd.Name, condition)
		}
		return true
	}

	return false
}
