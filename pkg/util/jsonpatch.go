package util

type JSONPatch = []Patch

type Patch = struct {
	Operation Operation   `json:"op"`
	From      string      `json:"from,omitempty"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value,omitempty"`
}

type Operation string

// Operation Constants
const (
	Add     Operation = "add"
	Remove  Operation = "remove"
	Replace Operation = "replace"
	Move    Operation = "move"
	Copy    Operation = "copy"
	Test    Operation = "test"
)
