package kubeingress

// Spec for a KubeIngress resource
type Spec struct {
	IngressTimeout *int `json:"timeout,omitempty"`
}
