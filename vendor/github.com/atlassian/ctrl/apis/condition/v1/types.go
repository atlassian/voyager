package v1

import (
	"bytes"
	"fmt"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConditionType string

// These are some possible conditions of an object.
const (
	ConditionInProgress ConditionType = "InProgress"
	ConditionReady      ConditionType = "Ready"
	ConditionError      ConditionType = "Error"
)

type ConditionStatus string

// These are some possible condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not. In the future, we could add other
// intermediate conditions, e.g. ConditionDegraded.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// +k8s:deepcopy-gen=true
// Condition describes the state of an object at a certain point.
type Condition struct {
	// Type of State condition.
	Type ConditionType `json:"type"`
	// Status of the condition.
	Status ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime meta_v1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

func (c *Condition) String() string {
	var buf bytes.Buffer
	buf.WriteString(string(c.Type))   // nolint: gosec
	buf.WriteByte(' ')                // nolint: gosec
	buf.WriteString(string(c.Status)) // nolint: gosec
	if c.Reason != "" {
		fmt.Fprintf(&buf, " %q", c.Reason) // nolint: errcheck
	}
	if c.Message != "" {
		fmt.Fprintf(&buf, " %q", c.Message) // nolint: errcheck
	}
	return buf.String()
}
