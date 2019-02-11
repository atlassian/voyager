package wiringutil

import (
	"fmt"
	"sort"
	"strings"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TemporaryNewWiringMigrationAdapter func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResultSuccess, bool, error)

func (f TemporaryNewWiringMigrationAdapter) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	success, retriable, err := f(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsRetriableError: retriable,
		}
	}
	return success
}

func (f TemporaryNewWiringMigrationAdapter) Status(resource *orch_v1.StateResource, context *wiringplugin.StatusContext) wiringplugin.StatusResult {
	return StatusAdapter(f.WireUp).Status(resource, context)
}

// StatusAdapter provides legacy status behavior for autowiring plugins. This adapter is deprecated, implement the Status() function directly.
type StatusAdapter func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult

func (f StatusAdapter) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	return f(resource, context)
}

func (f StatusAdapter) Status(resource *orch_v1.StateResource, context *wiringplugin.StatusContext) wiringplugin.StatusResult {
	resource2type2condition := newResourceConditionsFromStatusContext(context)

	return &wiringplugin.StatusResultSuccess{
		ResourceStatusData: orch_v1.ResourceStatusData{
			Conditions: []cond_v1.Condition{
				resource2type2condition.aggregateMessages(smith_v1.ResourceInProgress).calculateConditionAny(),
				resource2type2condition.aggregateMessages(smith_v1.ResourceReady).calculateConditionAll(),
				resource2type2condition.aggregateMessages(smith_v1.ResourceError).calculateConditionAny(),
			},
		},
	}
}

type resourceConditions struct {
	resource2type2condition map[*smith_v1.Resource]map[cond_v1.ConditionType]cond_v1.Condition
	pluginStatuses          []smith_v1.PluginStatus
}

func newResourceConditionsFromStatusContext(context *wiringplugin.StatusContext) resourceConditions {
	resource2type2condition := make(map[*smith_v1.Resource]map[cond_v1.ConditionType]cond_v1.Condition, len(context.BundleResources))
	// Group the Smith resourceStatus conditions into the above map
	for i := range context.BundleResources {
		bundleResource := &context.BundleResources[i]
		type2conditions := make(map[cond_v1.ConditionType]cond_v1.Condition, len(bundleResource.Status.Conditions))
		for _, condition := range bundleResource.Status.Conditions {
			type2conditions[condition.Type] = condition
		}
		resource2type2condition[&bundleResource.Resource] = type2conditions
	}
	return resourceConditions{
		resource2type2condition: resource2type2condition,
		pluginStatuses:          context.PluginStatuses,
	}
}

// aggregateMessages aggregates conditions, grouping them by their status.
// Returns formatted messages for each condition status value and boolean flags about those statues, non-exclusive of
// each other (0 or more can be true, 0 or more can be non-empty slices).
func (rc resourceConditions) aggregateMessages(conditionType cond_v1.ConditionType) aggregatedMessages {
	result := aggregatedMessages{
		conditionType: conditionType,
	}
	for smithResource, type2condition := range rc.resource2type2condition {
		condition, ok := type2condition[conditionType]
		if !ok {
			continue
		}
		switch condition.Status {
		case cond_v1.ConditionTrue:
			result.isTrue = true
			result.trueMsgs = rc.maybeAddMessage(result.trueMsgs, smithResource, condition.Reason, condition.Message)
		case cond_v1.ConditionFalse:
			result.isFalse = true
			result.falseMsgs = rc.maybeAddMessage(result.falseMsgs, smithResource, condition.Reason, condition.Message)
		case cond_v1.ConditionUnknown:
			fallthrough
		default:
			// We don't understand the status - it is unknown to us
			result.isUnknown = true
			result.unknownMsgs = rc.maybeAddMessage(result.unknownMsgs, smithResource, condition.Reason, condition.Message)
		}
	}
	return result
}

func (rc resourceConditions) maybeAddMessage(messages []string, smithResource *smith_v1.Resource, reason, message string) []string {
	if reason == "" && message == "" {
		return messages
	}
	var kind, name string
	switch {
	case smithResource.Spec.Object != nil:
		kind = smithResource.Spec.Object.GetObjectKind().GroupVersionKind().Kind
		name = smithResource.Spec.Object.(meta_v1.Object).GetName()
	case smithResource.Spec.Plugin != nil:
		found := false
		for _, pluginStatus := range rc.pluginStatuses {
			if smithResource.Spec.Plugin.Name != pluginStatus.Name {
				continue
			}
			kind = pluginStatus.Kind
			found = true
			break
		}
		if !found {
			return append(messages, fmt.Sprintf("plugin status not found for resource: %q", smithResource.Name))
		}
		name = smithResource.Spec.Plugin.ObjectName
	default:
		return append(messages, fmt.Sprintf("resource is neither an object nor a plugin: %q", smithResource.Name))
	}
	var msg string
	if reason != "" {
		msg = fmt.Sprintf("kind: %s, name: %s, message: %s, reason: %s", kind, name, message, reason)
	} else {
		msg = fmt.Sprintf("kind: %s, name: %s, message: %s", kind, name, message)
	}
	return append(messages, msg)
}

type aggregatedMessages struct {
	conditionType                    cond_v1.ConditionType
	trueMsgs, falseMsgs, unknownMsgs []string
	isTrue, isFalse, isUnknown       bool
}

// If any of the statuses in resourceConditions are true, then this sets the appropriate condition
func (am aggregatedMessages) calculateConditionAny() cond_v1.Condition {
	var condMsgs []string
	var status cond_v1.ConditionStatus
	switch { // Order is important because flags are not exclusive
	case am.isUnknown:
		condMsgs = am.unknownMsgs
		status = cond_v1.ConditionUnknown
	case am.isTrue:
		condMsgs = am.trueMsgs
		status = cond_v1.ConditionTrue
	default:
		condMsgs = am.falseMsgs
		status = cond_v1.ConditionFalse
	}
	return am.fmtCondition(condMsgs, status)
}

// If ALL of the statuses in resourceConditions are true, then this sets the appropriate condition
func (am aggregatedMessages) calculateConditionAll() cond_v1.Condition {
	var condMsgs []string
	var status cond_v1.ConditionStatus
	switch { // Order is important because flags are not exclusive
	case am.isUnknown:
		condMsgs = am.unknownMsgs
		status = cond_v1.ConditionUnknown
	case am.isFalse:
		condMsgs = am.falseMsgs
		status = cond_v1.ConditionFalse
	case am.isTrue:
		condMsgs = am.trueMsgs
		status = cond_v1.ConditionTrue
	default:
		// no conditions
		status = cond_v1.ConditionUnknown
	}
	return am.fmtCondition(condMsgs, status)
}

func (am aggregatedMessages) fmtCondition(condMsgs []string, status cond_v1.ConditionStatus) cond_v1.Condition {
	// resource2type2conditions is a map with non deterministic iteration order.
	// we sort the messages to ensure the final message string is deterministic
	sort.Strings(condMsgs)
	return cond_v1.Condition{
		Type:    am.conditionType,
		Status:  status,
		Message: strings.Join(condMsgs, "\n"),
	}
}
