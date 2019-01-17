package secretplugin

type Spec struct {
	JsonData map[string]interface{} `json:"jsondata,omitempty"`
	Data     map[string]string      `json:"data,omitempty"`
}
