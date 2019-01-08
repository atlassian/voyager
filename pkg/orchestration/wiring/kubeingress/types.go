package kubeingress

// Spec for a KubeIngress resource
type Spec struct {
	TimeoutSeconds *int `json:"timeoutSeconds,omitempty"`
}
