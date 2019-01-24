package iamrole

import (
	"encoding/json"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/pkg/errors"
)

const (
	EC2ComputeType  ComputeType = "ec2Compute"
	KubeComputeType ComputeType = "kubeCompute"
)

type Spec struct {
	ServiceName           voyager.ServiceName    `json:"serviceId,omitempty"`
	OAPResourceName       string                 `json:"oapResourceName"`
	CreateInstanceProfile bool                   `json:"createInstanceProfile,omitempty"`
	AssumeRoles           []string               `json:"assumeRoles,omitempty"`
	ManagedPolicies       []string               `json:"managedPolicies,omitempty"`
	ServiceEnvironment    oap.ServiceEnvironment `json:"serviceEnvironment"`
	ComputeType           ComputeType            `json:"computeType"`
	PolicySnippets        map[string]string      `json:"policySnippets"`
}

type ComputeType string

type CfnAttributes struct {
	Template     string `json:"template"`
	TemplateBody string `json:"templateBody"`
}

type IamPolicyDocument struct {
	Version   string               `json:"Version"`
	ID        string               `json:"Id,omitempty"`
	Statement []IamPolicyStatement `json:"Statement"`
}

type IamPolicy struct {
	PolicyName     string            `json:"PolicyName"`
	PolicyDocument IamPolicyDocument `json:"PolicyDocument"`
}

type IamAssumeRoleStatement struct {
	Effect    string                 `json:"Effect"`
	Principal IamAssumeRolePrincipal `json:"Principal"`
	Action    string                 `json:"Action"`
}

type IamAssumeRolePrincipal struct {
	AWS     string   `json:"AWS,omitempty"`
	Service []string `json:"Service,omitempty"`
}

// This is an IamPolicyStatement which doesn't allow non-array elements.
// See UnmarshalJSON below.
type IamPolicyStatement struct {
	Sid          *string          `json:",omitempty"`
	Principal    *json.RawMessage `json:",omitempty"`
	NotPrincipal *json.RawMessage `json:",omitempty"`

	NotAction []string `json:",omitempty"`
	Action    []string `json:",omitempty"`

	Effect string

	Resource    []string `json:",omitempty"`
	NotResource []string `json:",omitempty"`

	Condition *json.RawMessage `json:",omitempty"`
}

// Convert annoying IAM 'array or string' format to array only on Unmarshal
// so we can have nice types.
func (s *IamPolicyStatement) UnmarshalJSON(b []byte) error {
	var rawStatement struct {
		Sid    *string
		Effect string

		Principal    *json.RawMessage
		NotPrincipal *json.RawMessage

		Condition *json.RawMessage

		NotAction *json.RawMessage
		Action    *json.RawMessage

		Resource    *json.RawMessage
		NotResource *json.RawMessage
	}

	if err := json.Unmarshal(b, &rawStatement); err != nil {
		return err
	}

	s.Sid = rawStatement.Sid
	s.Effect = rawStatement.Effect
	s.Principal = rawStatement.Principal
	s.NotPrincipal = rawStatement.Principal
	s.Condition = rawStatement.Condition

	if err := copyJSONToSlice(&s.NotAction, rawStatement.NotAction); err != nil {
		return errors.Wrap(err, "failed to convert IAM NotAction JSON to array")
	}
	if err := copyJSONToSlice(&s.Action, rawStatement.Action); err != nil {
		return errors.Wrap(err, "failed to convert IAM Action JSON to array")
	}
	if err := copyJSONToSlice(&s.Resource, rawStatement.Resource); err != nil {
		return errors.Wrap(err, "failed to convert IAM Resource JSON to array")
	}
	if err := copyJSONToSlice(&s.NotResource, rawStatement.NotResource); err != nil {
		return errors.Wrap(err, "failed to convert IAM NotResource JSON to array")
	}
	return nil
}

// Convert annoying IAM 'array or string' format to array only on Unmarshal
// so we can have nice types.
func (p *IamPolicyDocument) UnmarshalJSON(b []byte) error {
	var rawPolicy struct {
		Version   string
		ID        string `json:"Id,omitempty"`
		Statement *json.RawMessage
	}

	if err := json.Unmarshal(b, &rawPolicy); err != nil {
		return err
	}

	p.Version = rawPolicy.Version
	p.ID = rawPolicy.ID

	if rawPolicy.Statement == nil {
		return nil
	}

	// Attempt to interpret Statement as Object
	var statement IamPolicyStatement
	if err := json.Unmarshal(*rawPolicy.Statement, &statement); err == nil {
		p.Statement = []IamPolicyStatement{statement}
		return nil
	}

	// If we fail, try as array (and report error now, since this is the normal case)
	if err := json.Unmarshal(*rawPolicy.Statement, &p.Statement); err != nil {
		return errors.Wrap(err, "expecting object or array for IAM Statement")
	}

	return nil
}

func copyJSONToSlice(target *[]string, source *json.RawMessage) error {
	if source == nil {
		return nil
	}
	var vArray []string
	if err := json.Unmarshal(*source, &vArray); err == nil {
		*target = vArray
		return nil
	}

	var vString string
	if err := json.Unmarshal(*source, &vString); err != nil {
		return errors.Wrap(err, "expecting string or array")
	}

	*target = []string{vString}

	return nil
}
