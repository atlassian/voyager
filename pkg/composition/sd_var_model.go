package composition

import (
	"strings"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/templating"
	"github.com/schollz/closestmatch"
)

type VarModel struct {
	SdConfigVars map[comp_v1.Scope]comp_v1.ServiceDescriptorConfigSet
}

func createVarModelFromSdSpec(sd *comp_v1.ServiceDescriptorSpec) *VarModel {
	varModel := make(map[comp_v1.Scope]comp_v1.ServiceDescriptorConfigSet, len(sd.Config))

	for _, scopedVars := range sd.Config {
		varModel[scopedVars.Scope] = scopedVars
	}

	return &VarModel{
		SdConfigVars: varModel,
	}
}

func (m *VarModel) getVar(locationHierarchy []string, varRef string) (interface{}, error) {
	var varVal interface{}
	varWasFound := false
	var varSimilar string

	for idx := range locationHierarchy {
		fullScope := comp_v1.Scope(strings.Join(locationHierarchy[:len(locationHierarchy)-idx], "."))
		scopeVarVal, foundScope, err := m.findVarInScope(fullScope, varRef)
		if err != nil {
			notFound, ok := err.(*util.ErrVariableNotFound)
			if ok {
				if foundScope && len(notFound.Similar) > 0 {
					varSimilar = notFound.Similar
				}
				continue
			} else {
				return nil, err
			}
		}

		varWasFound = true

		if scopeVarVal == nil {
			continue
		}

		if varVal == nil {
			varVal = scopeVarVal
			continue
		}

		varVal, err = templating.Merge(varVal, scopeVarVal)
		if err != nil {
			return nil, err
		}
	}

	globalVar, _, err := m.findVarInScope(voyager.ScopeGlobal, varRef)
	if err != nil {
		if _, ok := err.(*util.ErrVariableNotFound); !ok {
			return nil, err
		}
	}

	if globalVar == nil && varVal == nil && !varWasFound {
		if len(varSimilar) > 0 {
			return nil, util.NewErrVariableNotFound(varRef, varSimilar)
		}
		return nil, util.NewErrVariableNotFound(varRef, "")
	}

	return templating.Merge(varVal, globalVar)
}

func (m *VarModel) findVarInScope(scope comp_v1.Scope, varRef string) (interface{}, bool /* found scope */, error) {
	scopedVars, found := m.SdConfigVars[scope]
	if !found {
		var keys []string
		for key := range m.SdConfigVars {
			keys = append(keys, string(key))
		}
		closest := closestmatch.New(keys, []int{1}).Closest(string(scope))
		return nil, false, util.NewErrVariableNotFound(varRef, closest)
	}

	obj, err := templating.FindInMapRecursive(scopedVars.Vars, strings.Split(varRef, "."))
	return obj, true, err
}
