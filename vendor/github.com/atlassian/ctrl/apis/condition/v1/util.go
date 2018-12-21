package v1

func FindCondition(conditions []Condition, conditionType ConditionType) (int /* index */, *Condition) {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}

func CheckIfConditionChanged(currentCond, newCond *Condition) bool {
	return currentCond == nil ||
		currentCond.Status != newCond.Status ||
		currentCond.Reason != newCond.Reason ||
		currentCond.Message != newCond.Message
}

func PrepareCondition(currentConditions []Condition, newCondition *Condition) bool {
	needsUpdate := true

	// Try to find resource condition
	_, oldCondition := FindCondition(currentConditions, newCondition.Type)

	if oldCondition != nil {
		// We are updating an existing condition, so we need to check if it has changed.
		if newCondition.Status == oldCondition.Status {
			newCondition.LastTransitionTime = oldCondition.LastTransitionTime
		}

		needsUpdate = CheckIfConditionChanged(oldCondition, newCondition)
	}
	return needsUpdate
}
