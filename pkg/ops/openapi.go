package ops

// Temporary until we find a stable OpenApi3 spec library
type OpenAPISpec struct {
	OpenAPI string                 `json:"openapi"`
	Info    Info                   `json:"info,omitempty"`
	Servers []Server               `json:"servers,omitempty"`
	Paths   map[string]interface{} `json:"paths"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Info struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}
