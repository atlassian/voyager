package secretenvvar

type PodSpec struct {
	RenameEnvVar   map[string]string `json:"rename,omitempty"`
	IgnoreKeyRegex string            `json:"ignoreKeyRegex"`
}

type Spec struct {
	OutputSecretKey string            `json:"outputSecretKey"`
	OutputJSONKey   string            `json:"outputJsonKey"`
	RenameEnvVar    map[string]string `json:"rename,omitempty"`
	IgnoreKeyRegex  string            `json:"ignoreKeyRegex"`
}
