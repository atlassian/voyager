package opsgenie

type IntegrationsResponse struct {
	Integrations []Integration `json:"integrations"`
}

type Integration struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	TeamID   string `json:"teamId"`
	TeamName string `json:"teamName"`
	Priority string `json:"priority"`
	APIKey   string `json:"apiKey"`
	Endpoint string `json:"endpoint"`
	EnvType  string `json:"envType"`
}
