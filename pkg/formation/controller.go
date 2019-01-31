package formation

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/voyager"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	form_v1client "github.com/atlassian/voyager/pkg/formation/client/typed/formation/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/templating"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/yaml"
)

const (
	ByReleaseConfigNameIndexName = "releaseConfigMapIndex"
	ReleaseTemplatingPrefix      = "release:"
	releaseConfigMapDataKey      = releases.DataKey
)

func ByReleaseConfigMapNameIndex(obj interface{}) ([]string, error) {
	ld := obj.(*form_v1.LocationDescriptor)
	namespace := ld.GetNamespace()
	releaseConfigMapName := ld.Spec.ConfigMapNames.Release

	return []string{ByConfigMapNameIndexKey(namespace, releaseConfigMapName)}, nil
}

func ByConfigMapNameIndexKey(namespace string, configMapName string) string {
	return namespace + "/" + configMapName
}

const (
	defaultKubeComputeMinReplicas                     int32  = 1
	defaultKubeComputeMinReplicasProd                 int32  = 3
	defaultKubeComputeMaxReplicas                     int32  = 5
	defaultKubeComputeResourceMetricTargetUtilization int32  = 80
	defaultKubeComputeImagePullPolicy                        = string(core_v1.PullIfNotPresent)
	defaultKubeComputeProtocol                               = string(core_v1.ProtocolTCP)
	defaultKubeComputeProbeTimeoutSeconds             int32  = 1
	defaultKubeComputeProbePeriodSeconds              int32  = 10
	defaultKubeComputeProbeSuccessThreshold           int32  = 1
	defaultKubeComputeProbeFailureThreshold           int32  = 3
	defaultKubeComputeHTTPGetPath                     string = "/healthcheck"
	defaultKubeComputeHTTPGetScheme                          = string(core_v1.URISchemeHTTP)
)

// Default limits are equivalent to a t2.micros EC2 instance
var (
	defaultKubeComputeResourceRequestMemory = resource.MustParse("750Mi")
	defaultKubeComputeResourceLimitMemory   = resource.MustParse("1Gi")
	defaultKubeComputeResourceRequestCPU    = resource.MustParse("150m")
	defaultKubeComputeResourceLimitCPU      = resource.MustParse("1")
)

type Controller struct {
	Logger               *zap.Logger
	Clock                clock.Clock
	ReadyForWork         func()
	LDInformer           cache.SharedIndexInformer
	StateInformer        cache.SharedIndexInformer
	LDClient             form_v1client.LocationDescriptorsGetter
	ConfigMapInformer    cache.SharedIndexInformer
	LDTransitionsCounter *prometheus.CounterVec

	Location options.Location

	StateObjectUpdater updater.ObjectUpdater
}

func (c *Controller) Run(ctx context.Context) {
	defer c.Logger.Info("Shutting down Formation controller")
	c.Logger.Info("Starting the Formation controller")
	c.ReadyForWork()
	<-ctx.Done()
}

func (c *Controller) Process(ctx *ctrl.ProcessContext) (bool /* retriable */, error) {
	ld := ctx.Object.(*form_v1.LocationDescriptor)
	if ld.ObjectMeta.DeletionTimestamp != nil {
		// Marked for deletion, do nothing
		return false, nil
	}

	conflict, retriable, state, err := c.processLocationDescriptor(ctx.Logger, ld)
	if conflict {
		return false, nil
	}

	conflict, retriable, err = c.handleProcessResult(ctx.Logger, ld, state, retriable, err)
	if conflict {
		ctx.Logger.Debug("Conflict detected while handling process result", zap.Error(err))
		return false, nil
	}

	return retriable, err
}

func (c *Controller) processLocationDescriptor(logger *zap.Logger, ld *form_v1.LocationDescriptor) (bool /* conflict */, bool /* retriable */, *orch_v1.State, error) {
	name := ld.GetName()
	logger.Sugar().Infof("Ensuring state object for service %q is present", ld.GetName())

	// Grab the Releases ConfigMap
	configMapInterface, exists, err := c.ConfigMapInformer.GetIndexer().GetByKey(
		ByConfigMapNameIndexKey(ld.GetNamespace(), ld.Spec.ConfigMapNames.Release))
	if err != nil {
		return false, false, nil, errors.WithStack(err)
	}
	var releaseData map[string]interface{}
	if exists {
		configMap := configMapInterface.(*core_v1.ConfigMap)
		releaseStr, ok := configMap.Data[releaseConfigMapDataKey]
		if !ok {
			return false, false, nil, errors.Errorf("release config map is missing expected data key: '%s'", releaseConfigMapDataKey)
		}
		err = yaml.Unmarshal([]byte(releaseStr), &releaseData)
		if err != nil {
			return false, false, nil, err
		}
	}

	resources, err := c.convertToStateResources(ld.Spec, releaseData)
	if err != nil {
		return false, false, nil, err
	}
	if ld.Spec.ConfigMapName == "" {
		return false, false, nil, errors.New("configMapName is missing")
	}
	spec := orch_v1.StateSpec{
		ConfigMapName: ld.Spec.ConfigMapName,
		Resources:     resources,
	}

	desired := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: orch_v1.StateResourceAPIVersion,
			Kind:       orch_v1.StateResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: ld.GetNamespace(),
			OwnerReferences: []meta_v1.OwnerReference{
				ldOwnerReference(ld),
			},
		},
		Spec: spec,
	}

	conflict, retriable, state, err := c.StateObjectUpdater.CreateOrUpdate(
		logger,
		func(r runtime.Object) error {
			meta := r.(meta_v1.Object)
			if !meta_v1.IsControlledBy(meta, ld) {
				return errors.Errorf("state %q not owned by ld %q", meta.GetName(), ld.GetName())
			}
			return nil
		},
		desired,
	)
	realState, _ := state.(*orch_v1.State)
	return conflict, retriable, realState, err
}

func copyCondition(state *orch_v1.State, condType cond_v1.ConditionType, condition *cond_v1.Condition) {
	_, stateCond := state.GetCondition(condType)

	if stateCond == nil {
		condition.Status = cond_v1.ConditionUnknown
		condition.Reason = "OrchestrationInteropError"
		condition.Message = "Orchestration not reporting state for this condition"
		return
	}

	if stateCond.Reason != "" {
		condition.Reason = stateCond.Reason
	}
	if stateCond.Message != "" {
		condition.Message = "Orchestration: " + stateCond.Message
	}
	switch stateCond.Status {
	case cond_v1.ConditionTrue:
		condition.Status = cond_v1.ConditionTrue
	case cond_v1.ConditionUnknown:
		condition.Status = cond_v1.ConditionUnknown
	case cond_v1.ConditionFalse:
		condition.Status = cond_v1.ConditionFalse
	default:
		condition.Status = cond_v1.ConditionUnknown
		condition.Reason = "OrchestrationInteropError"
		condition.Message = fmt.Sprintf("Unexpected ConditionStatus %q", stateCond.Status)
	}
}

func (c *Controller) handleProcessResult(logger *zap.Logger, ld *form_v1.LocationDescriptor, state *orch_v1.State, retriable bool, err error) (conflictRet, retriableRet bool, e error) {
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
	resourceStatuses := ld.Status.ResourceStatuses

	if err != nil {
		errorCond.Status = cond_v1.ConditionTrue
		errorCond.Message = err.Error()
		if retriable {
			errorCond.Reason = "RetriableError"
			inProgressCond.Status = cond_v1.ConditionTrue
		} else {
			errorCond.Reason = "TerminalError"
		}
	} else if len(state.Status.Conditions) == 0 {
		inProgressCond.Status = cond_v1.ConditionTrue
		inProgressCond.Reason = "WaitingOnOrchestrationConditions"
		inProgressCond.Message = "Waiting for Orchestration to report Conditions (initial creation?)"
	} else {
		// This just copies the status from State
		copyCondition(state, cond_v1.ConditionInProgress, &inProgressCond)
		copyCondition(state, cond_v1.ConditionReady, &readyCond)
		copyCondition(state, cond_v1.ConditionError, &errorCond)

		// The way we calculate these, we assume State's status would change
		// if any of the resource statuses cause a transition, so there's no
		// need to recalculate the LD condition.
		// However, there is still a need to set the status if the resource
		// status changes (i.e. transition timestamp changes)
		resourceStatuses = make([]form_v1.ResourceStatus, 0, len(state.Status.ResourceStatuses))
		for _, resourceStatus := range state.Status.ResourceStatuses {
			conditions := make([]cond_v1.Condition, 0, len(resourceStatus.Conditions))

			for _, condition := range resourceStatus.Conditions {
				conditions = append(conditions, cond_v1.Condition{
					LastTransitionTime: condition.LastTransitionTime,
					Message:            condition.Message,
					Reason:             condition.Reason,
					Status:             condition.Status,
					Type:               condition.Type,
				})
			}

			resourceStatuses = append(resourceStatuses, form_v1.ResourceStatus{
				Name:       resourceStatus.Name,
				Conditions: conditions,
			})
		}
	}

	inProgressUpdated := c.updateCondition(ld, &inProgressCond)
	readyUpdated := c.updateCondition(ld, &readyCond)
	errorUpdated := c.updateCondition(ld, &errorCond)
	resourceStatusesUpdated := c.updateResourceStatuses(ld, resourceStatuses)

	// Updating the LocationDescriptor status
	if inProgressUpdated || readyUpdated || errorUpdated || resourceStatusesUpdated {
		conflictStatus, retriableStatus, errStatus := c.setStatus(logger, ld)
		if errStatus != nil {
			if err != nil {
				logger.Info("Failed to set LocationDescriptor status", zap.Error(errStatus))
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

func (c *Controller) setStatus(logger *zap.Logger, ld *form_v1.LocationDescriptor) (conflictRet, retriableRet bool, e error) {
	logger.Info("Writing status")
	_, err := c.LDClient.LocationDescriptors(ld.Namespace).Update(ld)
	if err != nil {
		if api_errors.IsConflict(err) {
			return true, false, nil
		}
		if api_errors.IsInvalid(err) {
			return false, true, errors.Wrap(err, "request is invalid")
		}
		return false, true, errors.Wrap(err, "failed to set LocationDescriptor status")
	}
	return false, false, nil
}

func (c *Controller) updateCondition(ld *form_v1.LocationDescriptor, condition *cond_v1.Condition) bool {
	cond := *condition // copy to avoid mutating the original

	var needsUpdate bool
	i, oldCondition := cond_v1.FindCondition(ld.Status.Conditions, cond.Type)
	needsUpdate = k8s.FillCondition(c.Clock, oldCondition, &cond)

	if needsUpdate {
		if i == -1 {
			ld.Status.Conditions = append(ld.Status.Conditions, cond)
		} else {
			ld.Status.Conditions[i] = cond
		}
		if cond.Status == cond_v1.ConditionTrue {
			c.LDTransitionsCounter.
				WithLabelValues(ld.GetNamespace(), ld.GetName(), string(cond.Type), cond.Reason).
				Inc()
		}
		return true
	}

	return false
}

func (c *Controller) updateResourceStatuses(ld *form_v1.LocationDescriptor, newResourceStatuses []form_v1.ResourceStatus) bool {
	existingResourceStatusMap := make(map[voyager.ResourceName]*form_v1.ResourceStatus, len(ld.Status.ResourceStatuses))
	for i := range ld.Status.ResourceStatuses {
		existingResourceStatusMap[ld.Status.ResourceStatuses[i].Name] = &ld.Status.ResourceStatuses[i]
	}

	// perform a comparison to see if things have changed
	var newStatuses []form_v1.ResourceStatus
	var changed bool
	for _, newResourceStatus := range newResourceStatuses {
		existingResourceStatus, hasExistingStatus := existingResourceStatusMap[newResourceStatus.Name]
		if hasExistingStatus {
			changed = k8s.FillNewConditions(
				c.Clock, existingResourceStatus.Conditions, newResourceStatus.Conditions) || changed
		} else {
			changed = true
		}

		newStatuses = append(newStatuses, newResourceStatus)
	}

	if changed {
		ld.Status.ResourceStatuses = newStatuses
		return true
	}

	return false
}

func (c *Controller) convertToStateResources(ldSpec form_v1.LocationDescriptorSpec, releaseData map[string]interface{}) ([]orch_v1.StateResource, error) {
	srs := make([]orch_v1.StateResource, 0, len(ldSpec.Resources))
	releaseDataResolver := func(varName string) (interface{}, error) {
		if releaseData == nil {
			return varName, errors.Errorf("no release data was available, but variable %s was templated in LD", varName)
		}
		res, err := templating.FindInMapRecursive(releaseData, strings.Split(varName, "."))
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	releaseSpecExpander := templating.SpecExpander{
		VarResolver:      releaseDataResolver,
		RequiredPrefix:   ReleaseTemplatingPrefix,
		ReservedPrefixes: []string{},
	}

	errorList := util.NewErrorList()

	for _, ldr := range ldSpec.Resources {
		sdeps := make([]orch_v1.StateDependency, 0, len(ldr.DependsOn))
		for _, sdep := range ldr.DependsOn {
			sdeps = append(sdeps, orch_v1.StateDependency{Name: sdep.Name, Attributes: sdep.Attributes})
		}
		defaults, err := util.ToRawExtension(getDefaults(ldr.Type, c.Location.ClusterLocation()))
		if err != nil {
			return nil, err
		}

		expandedSpec, errs := releaseSpecExpander.Expand(ldr.Spec)
		if errs != nil && errs.HasErrors() {
			errorList.AddErrorList(errs)
		}

		sr := orch_v1.StateResource{
			Name:      ldr.Name,
			Type:      ldr.Type,
			Spec:      expandedSpec,
			DependsOn: sdeps,
			Defaults:  defaults,
		}
		srs = append(srs, sr)
	}

	if errorList.HasErrors() {
		return nil, errorList
	}

	return srs, nil
}

// Hard-coded temporarily. In future, should be versioned, come from provider, etc. etc.
// See src/lib/descriptor.js in micros-server for where these come from
// (should keep these aligned for now!).
func getDefaults(resourceType voyager.ResourceType, location voyager.ClusterLocation) map[string]interface{} {
	switch resourceType {
	case voyager.ResourceType("DynamoDB"):
		return map[string]interface{}{
			"BackupPeriod": "1 hours",
		}
	case voyager.ResourceType("KubeCompute"):
		var minReplicas int32
		switch location.EnvType {
		case voyager.EnvTypeProduction, voyager.EnvTypeStaging:
			minReplicas = defaultKubeComputeMinReplicasProd
		default:
			minReplicas = defaultKubeComputeMinReplicas
		}
		return map[string]interface{}{
			"Scaling": map[string]interface{}{
				"MinReplicas": minReplicas,
				"MaxReplicas": defaultKubeComputeMaxReplicas,
				"Metrics": []map[string]interface{}{
					{
						"Type": "Resource",
						"Resource": map[string]interface{}{
							"Name":                     "cpu",
							"TargetAverageUtilization": defaultKubeComputeResourceMetricTargetUtilization,
						},
					},
				},
			},
			"Container": map[string]interface{}{
				"ImagePullPolicy": defaultKubeComputeImagePullPolicy,
				"LivenessProbe": map[string]interface{}{
					"TimeoutSeconds":   defaultKubeComputeProbeTimeoutSeconds,
					"PeriodSeconds":    defaultKubeComputeProbePeriodSeconds,
					"SuccessThreshold": defaultKubeComputeProbeSuccessThreshold,
					"FailureThreshold": defaultKubeComputeProbeFailureThreshold,
					"HTTPGet": map[string]interface{}{
						"Path":   defaultKubeComputeHTTPGetPath,
						"Scheme": defaultKubeComputeHTTPGetScheme,
					},
				},
				"ReadinessProbe": map[string]interface{}{
					"TimeoutSeconds":   defaultKubeComputeProbeTimeoutSeconds,
					"PeriodSeconds":    defaultKubeComputeProbePeriodSeconds,
					"SuccessThreshold": defaultKubeComputeProbeSuccessThreshold,
					"FailureThreshold": defaultKubeComputeProbeFailureThreshold,
					"HTTPGet": map[string]interface{}{
						"Path":   defaultKubeComputeHTTPGetPath,
						"Scheme": defaultKubeComputeHTTPGetScheme,
					},
				},
				"Resources": map[string]interface{}{
					"Requests": map[string]interface{}{
						"cpu":    defaultKubeComputeResourceRequestCPU,
						"memory": defaultKubeComputeResourceRequestMemory,
					},
					"Limits": map[string]interface{}{
						"cpu":    defaultKubeComputeResourceLimitCPU,
						"memory": defaultKubeComputeResourceLimitMemory,
					},
				},
			},
			"Port": map[string]interface{}{
				"Protocol": defaultKubeComputeProtocol,
			},
		}
	case voyager.ResourceType("KubeIngress"):
		return map[string]interface{}{
			"timeoutSeconds": 60,
		}
	default:
		return map[string]interface{}{}
	}

}

func ldOwnerReference(ld *form_v1.LocationDescriptor) meta_v1.OwnerReference {
	trueVar := true
	return meta_v1.OwnerReference{
		APIVersion:         ld.APIVersion,
		Kind:               ld.Kind,
		Name:               ld.Name,
		UID:                ld.UID,
		Controller:         &trueVar,
		BlockOwnerDeletion: &trueVar,
	}
}
