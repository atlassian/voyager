package testing

import (
	kube_testing "k8s.io/client-go/testing"
)

func FilterCreateActions(actions []kube_testing.Action) []kube_testing.CreateAction {
	var result []kube_testing.CreateAction
	for _, action := range actions {
		if create, ok := action.(kube_testing.CreateAction); ok {
			if create.GetVerb() == "create" {
				result = append(result, create)
			}
		}
	}
	return result
}

func FilterUpdateActions(actions []kube_testing.Action) []kube_testing.UpdateAction {
	var result []kube_testing.UpdateAction
	for _, action := range actions {
		if update, ok := action.(kube_testing.UpdateAction); ok {
			if update.GetVerb() == "update" {
				result = append(result, update)
			}
		}
	}
	return result
}
