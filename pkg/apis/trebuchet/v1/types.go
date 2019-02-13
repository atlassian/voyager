package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ReleaseResourceVersion = "v1"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:noStatus
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Release struct {

	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// artifacts
	Artifacts []*ArtifactReference `json:"artifacts"`

	// A human friendly description for the Release.
	Description string `json:"description,omitempty"`

	// The service this Release was created for.
	// Read Only: true
	Service string `json:"service,omitempty"`

	// Unique ID representing a Release. It's made up by the unique combination of service and artifact references included in the release.
	// Read Only: true
	UUID string `json:"uuid,omitempty"`

	// A human friendly version for the Release.
	Version string `json:"version,omitempty"`
}

// +k8s:deepcopy-gen=true
type ArtifactReference struct {

	// The alias of the Artifact within the given release it can be referenced by.
	Alias string `json:"alias,omitempty"`

	// Name or namespace of the Artifact you're referencing.
	Name string `json:"name,omitempty"`

	// The version of the Artifact you're referencing.
	Version string `json:"version,omitempty"`
}

// ReleaseList is a list of Releases.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ReleaseList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Release `json:"items"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ReleaseGroup struct {

	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// The account location coordinate.
	Account string `json:"account,omitempty"`

	// The environment location coordinate.
	Environment string `json:"environment,omitempty"`

	// The label location coordinate.
	Label string `json:"label,omitempty"`

	// name
	Name string `json:"name,omitempty"`

	// The region location coordinate.
	Region string `json:"region,omitempty"`

	// The id of the release.
	Release string `json:"release,omitempty"`

	// The service this release group belongs to.
	// Read Only: true
	Service string `json:"service,omitempty"`
}

