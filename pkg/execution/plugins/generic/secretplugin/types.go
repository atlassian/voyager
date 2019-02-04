package secretplugin

type Spec struct {
	JSONData map[string]interface{} `json:"jsondata,omitempty"`
	Data     map[string]string      `json:"data,omitempty"`
}
