package k8s

import (
	"reflect"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"
)

// FillCondition returns true if the cond is different from oldCondition.
// If the timestamps are null, then this ignores those for comparison and also
// fills them with defaults.
// If oldCondition is null, it considers it a new condition.
// "cond" will be mutated with the timestamps.
func FillCondition(clk clock.Clock, oldCondition, cond *cond_v1.Condition) bool {
	needsUpdate := true
	if oldCondition != nil {
		// We are updating an existing condition, so we need to check if it has changed.
		// This also checks if LastTransitionTime is set.
		// If not:
		// * The LastTransitionTime will be filled with the old condition's
		// * Both timestamps will be ignored for comparison purposes.

		shouldSetTime := cond.LastTransitionTime == meta_v1.Time{}
		if shouldSetTime {
			cond.LastTransitionTime = oldCondition.LastTransitionTime
		}

		isEqual := reflect.DeepEqual(cond, oldCondition)

		needsUpdate = !isEqual
		if needsUpdate && shouldSetTime {
			now := clk.Now()
			if cond.Status != oldCondition.Status {
				cond.LastTransitionTime = meta_v1.Time{Time: now}
			}
		}
	} else if (cond.LastTransitionTime == meta_v1.Time{}) {
		// New condition
		cond.LastTransitionTime = meta_v1.NewTime(clk.Now())
	}

	return needsUpdate
}

func FillNewConditions(c clock.Clock, existingConditions, newConditions []cond_v1.Condition) bool {
	updated := false

	for i := range newConditions {
		var oldCondition *cond_v1.Condition
		if existingConditions != nil {
			_, oldCondition = cond_v1.FindCondition(existingConditions, newConditions[i].Type)
		}
		updated = FillCondition(c, oldCondition, &newConditions[i]) || updated
	}

	return updated || len(existingConditions) != len(newConditions)
}

func CalculateConditionAny(conditions []cond_v1.Condition) cond_v1.ConditionStatus {
	var anyUnknown bool
	for _, condition := range conditions {
		switch condition.Status {
		case cond_v1.ConditionTrue:
			return cond_v1.ConditionTrue
		case cond_v1.ConditionUnknown:
			anyUnknown = true
		}
	}

	if anyUnknown {
		return cond_v1.ConditionUnknown
	}

	return cond_v1.ConditionFalse
}

func CalculateConditionAll(conditions []cond_v1.Condition) cond_v1.ConditionStatus {
	if len(conditions) == 0 {
		return cond_v1.ConditionUnknown
	}

	// If ALL of the statuses in Conditions are true, then this sets the appropriate condition
	var anyFalse bool
	for _, condition := range conditions {
		switch condition.Status {
		case cond_v1.ConditionUnknown:
			return cond_v1.ConditionUnknown
		case cond_v1.ConditionFalse:
			anyFalse = true
		}
	}

	if anyFalse {
		return cond_v1.ConditionFalse
	}

	return cond_v1.ConditionTrue
}
