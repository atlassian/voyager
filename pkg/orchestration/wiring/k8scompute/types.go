package k8scompute

import (
	"encoding/json"

	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Spec for the K8SCompute object
type Spec struct {
	Containers []Container `json:"containers"`
	Scaling    Scaling     `json:"scaling,omitempty"`

	// RenameEnvVar represents environment variables to rename; it is passed through to the plugin that handles secrets
	RenameEnvVar map[string]string `json:"rename,omitempty"`
}

// DefaultSpec holds the defaults for the K8SCompute object
type DefaultSpec struct {
	Container Container
	Port      ContainerPort
	Scaling   Scaling
}

type Scaling struct {
	MinReplicas int32    `json:"minReplicas,omitempty"`
	MaxReplicas int32    `json:"maxReplicas,omitempty"`
	Metrics     []Metric `json:"metrics,omitempty"`
}

type Metric struct {
	Type     autoscaling_v2b1.MetricSourceType `json:"type"`
	Resource *ResourceMetric                   `json:"resource,omitempty"`
}

type ObjectMetric struct {
}

type PodsMetric struct {
}

type ResourceMetric struct {
	Name                     core_v1.ResourceName `json:"name"`
	TargetAverageUtilization *int32               `json:"targetAverageUtilization,omitempty"`
	TargetAverageValue       *resource.Quantity   `json:"targetAverageValue,omitempty"`
}

type ExternalMetric struct {
}

type Container struct {
	Name            string               `json:"name"`
	Image           string               `json:"image,omitempty"`
	Command         []string             `json:"command,omitempty"`
	Args            []string             `json:"args,omitempty"`
	WorkingDir      string               `json:"workingDir,omitempty"`
	Ports           []ContainerPort      `json:"ports,omitempty"`
	Env             []EnvVar             `json:"env,omitempty"`
	EnvFrom         []EnvFromSource      `json:"envFrom,omitempty"`
	Resources       ResourceRequirements `json:"resources,omitempty"`
	ImagePullPolicy string               `json:"imagePullPolicy,omitempty"`
	LivenessProbe   *Probe               `json:"livenessProbe,omitempty"`
	ReadinessProbe  *Probe               `json:"readinessProbe,omitempty"`
}

type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

type EnvVar struct {
	Name      string        `json:"name"`
	Value     string        `json:"value"`
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

type EnvVarSource struct {
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

type SecretKeySelector struct {
	LocalObjectReference `json:",inline"`
	Key                  string `json:"key"`
}

type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline"`
	Key                  string `json:"key"`
}

type LocalObjectReference struct {
	Name string `json:"name,omitempty"`
}

type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty"`
}

type ExecAction struct {
	Command []string `json:"command,omitempty" protobuf:"bytes,1,rep,name=command"`
}

type URIScheme string

type HTTPHeader struct {
	Name  string `json:"name" protobuf:"bytes,1,opt,name=name"`
	Value string `json:"value" protobuf:"bytes,2,opt,name=value"`
}

type HTTPGetAction struct {
	Path        string             `json:"path,omitempty" protobuf:"bytes,1,opt,name=path"`
	Port        intstr.IntOrString `json:"port" protobuf:"bytes,2,opt,name=port"`
	Host        string             `json:"host,omitempty" protobuf:"bytes,3,opt,name=host"`
	Scheme      URIScheme          `json:"scheme,omitempty" protobuf:"bytes,4,opt,name=scheme,casttype=URIScheme"`
	HTTPHeaders []HTTPHeader       `json:"httpHeaders,omitempty" protobuf:"bytes,5,rep,name=httpHeaders"`
}

type TCPSocketAction struct {
	Port intstr.IntOrString `json:"port" protobuf:"bytes,1,opt,name=port"`
	Host string             `json:"host,omitempty" protobuf:"bytes,2,opt,name=host"`
}

type Handler struct {
	Exec      *ExecAction      `json:"exec,omitempty" protobuf:"bytes,1,opt,name=exec"`
	HTTPGet   *HTTPGetAction   `json:"httpGet,omitempty" protobuf:"bytes,2,opt,name=httpGet"`
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" protobuf:"bytes,3,opt,name=tcpSocket"`
}

type Probe struct {
	Handler             `json:",inline" protobuf:"bytes,1,opt,name=handler"`
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty" protobuf:"varint,2,opt,name=initialDelaySeconds"`
	TimeoutSeconds      int32 `json:"timeoutSeconds,omitempty" protobuf:"varint,3,opt,name=timeoutSeconds"`
	PeriodSeconds       int32 `json:"periodSeconds,omitempty" protobuf:"varint,4,opt,name=periodSeconds"`
	SuccessThreshold    int32 `json:"successThreshold,omitempty" protobuf:"varint,5,opt,name=successThreshold"`
	FailureThreshold    int32 `json:"failureThreshold,omitempty" protobuf:"varint,6,opt,name=failureThreshold"`
}

type ResourceList map[string]resource.Quantity

type EnvFromSource struct {
	Prefix       string              `json:"prefix,omitempty"`
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty"`
}

type ConfigMapEnvSource struct {
	LocalObjectReference `json:",inline"`
}

func (s *Spec) ApplyDefaults(defaults *runtime.RawExtension) error {
	// Unmarshal the defaults from the resource state
	defaultSpec := &DefaultSpec{}
	if defaults != nil {
		if err := json.Unmarshal(defaults.Raw, &defaultSpec); err != nil {
			return err
		}
	}

	if s.Scaling.MinReplicas == 0 {
		s.Scaling.MinReplicas = defaultSpec.Scaling.MinReplicas
	}

	if s.Scaling.MaxReplicas == 0 {
		s.Scaling.MaxReplicas = defaultSpec.Scaling.MaxReplicas
	}

	s.applyMetricDefaults(defaultSpec)

	for i := range s.Containers {
		container := &s.Containers[i]
		if container.ImagePullPolicy == "" {
			container.ImagePullPolicy = defaultSpec.Container.ImagePullPolicy
		}

		for j := range container.Ports {
			port := &container.Ports[j]
			if port.Protocol == "" {
				port.Protocol = defaultSpec.Port.Protocol
			}
		}

		// Ensure there are cpu/memory limits and requests
		if container.Resources.Requests == nil {
			container.Resources.Requests = map[string]resource.Quantity{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = map[string]resource.Quantity{}
		}

		if _, ok := container.Resources.Requests["cpu"]; !ok {
			container.Resources.Requests["cpu"] = defaultSpec.Container.Resources.Requests["cpu"]
		}
		if _, ok := container.Resources.Requests["memory"]; !ok {
			container.Resources.Requests["memory"] = defaultSpec.Container.Resources.Requests["memory"]
		}

		if _, ok := container.Resources.Limits["cpu"]; !ok {
			container.Resources.Limits["cpu"] = defaultSpec.Container.Resources.Limits["cpu"]
		}
		if _, ok := container.Resources.Limits["memory"]; !ok {
			container.Resources.Limits["memory"] = defaultSpec.Container.Resources.Limits["memory"]
		}

		if container.LivenessProbe != nil {
			container.LivenessProbe.applyProbeDefaults(defaultSpec.Container.LivenessProbe)
		}

		if container.ReadinessProbe != nil {
			container.ReadinessProbe.applyProbeDefaults(defaultSpec.Container.ReadinessProbe)
		}

	}

	return nil
}

func (probe *Probe) applyProbeDefaults(defaultProbe *Probe) {
	// Return if formation hasn't passed in defaults for the probes
	if defaultProbe == nil {
		return
	}

	if probe.TimeoutSeconds == 0 {
		probe.TimeoutSeconds = defaultProbe.TimeoutSeconds
	}
	if probe.PeriodSeconds == 0 {
		probe.PeriodSeconds = defaultProbe.PeriodSeconds
	}
	if probe.SuccessThreshold == 0 {
		probe.SuccessThreshold = defaultProbe.SuccessThreshold
	}
	if probe.FailureThreshold == 0 {
		probe.FailureThreshold = defaultProbe.FailureThreshold
	}

	if probe.HTTPGet != nil {
		if probe.HTTPGet.Path == "" {
			probe.HTTPGet.Path = defaultProbe.HTTPGet.Path
		}
		if probe.HTTPGet.Scheme == "" {
			probe.HTTPGet.Scheme = defaultProbe.HTTPGet.Scheme
		}
	}
}

func (s *Spec) applyMetricDefaults(defaultSpec *DefaultSpec) {
	if len(s.Scaling.Metrics) == 0 {
		s.Scaling.Metrics = defaultSpec.Scaling.Metrics
	}
}

func (metric *Metric) ToKubeMetric() autoscaling_v2b1.MetricSpec {
	hpaMetric := autoscaling_v2b1.MetricSpec{
		Type: metric.Type,
	}

	switch metric.Type {
	case autoscaling_v2b1.MetricSourceType("Resource"):
		hpaMetric.Resource = &autoscaling_v2b1.ResourceMetricSource{
			Name:                     metric.Resource.Name,
			TargetAverageUtilization: metric.Resource.TargetAverageUtilization,
			TargetAverageValue:       metric.Resource.TargetAverageValue,
		}
	default:
		// Ignore unknown metrics
	}

	return hpaMetric
}

func (container *Container) ToKubeContainer(envDefault []core_v1.EnvVar, envFrom []core_v1.EnvFromSource) core_v1.Container {
	// Container.Env
	env := make([]core_v1.EnvVar, 0, len(container.Env))
	for _, envVar := range container.Env {
		env = append(env, envVar.toKubeEnvVar())
	}
	// envDefault
	env = append(env, envDefault...)

	// Container.Ports
	ports := make([]core_v1.ContainerPort, 0, len(container.Ports))
	for _, port := range container.Ports {
		ports = append(ports, port.toKubeContainerPort())
	}

	// Container.EnvFrom
	for _, envFromSource := range container.EnvFrom {
		envFrom = append(envFrom, envFromSource.toKubeEnvFromSource())
	}

	return core_v1.Container{
		Name:                     container.Name,
		Image:                    container.Image,
		Command:                  container.Command,
		Env:                      env,
		EnvFrom:                  envFrom,
		Args:                     container.Args,
		WorkingDir:               container.WorkingDir,
		Ports:                    ports,
		Resources:                container.Resources.toKubeResourceRequirements(),
		ImagePullPolicy:          core_v1.PullPolicy(container.ImagePullPolicy),
		TerminationMessagePath:   core_v1.TerminationMessagePathDefault,
		TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
		LivenessProbe:            container.LivenessProbe.toKubeProbe(),
		ReadinessProbe:           container.ReadinessProbe.toKubeProbe(),
	}
}

func (containerPort *ContainerPort) toKubeContainerPort() core_v1.ContainerPort {
	return core_v1.ContainerPort{
		Name:          containerPort.Name,
		ContainerPort: containerPort.ContainerPort,
		Protocol:      core_v1.Protocol(containerPort.Protocol),
	}
}

func (resourceRequirements *ResourceRequirements) toKubeResourceRequirements() core_v1.ResourceRequirements {
	resourceLimits := core_v1.ResourceList{}
	for k, v := range resourceRequirements.Limits {
		resourceLimits[core_v1.ResourceName(k)] = v
	}

	resourceRequests := core_v1.ResourceList{}
	for k, v := range resourceRequirements.Requests {
		resourceRequests[core_v1.ResourceName(k)] = v
	}

	return core_v1.ResourceRequirements{Limits: resourceLimits, Requests: resourceRequests}
}

func (probe *Probe) toKubeProbe() *core_v1.Probe {
	if probe == nil {
		return nil
	}

	var exec *core_v1.ExecAction
	var httpGet *core_v1.HTTPGetAction
	var tcpSocket *core_v1.TCPSocketAction

	if probe.Exec != nil {
		var command []string
		command = append(command, probe.Exec.Command...)
		exec = &core_v1.ExecAction{
			Command: command,
		}
	}

	if probe.HTTPGet != nil {
		var httpHeaders []core_v1.HTTPHeader
		for _, v := range probe.HTTPGet.HTTPHeaders {
			httpHeaders = append(httpHeaders, core_v1.HTTPHeader{Name: v.Name, Value: v.Value})
		}
		httpGet = &core_v1.HTTPGetAction{
			Path:        probe.HTTPGet.Path,
			Port:        probe.HTTPGet.Port,
			Host:        probe.HTTPGet.Host,
			Scheme:      core_v1.URIScheme(probe.HTTPGet.Scheme),
			HTTPHeaders: httpHeaders,
		}
	}

	if probe.TCPSocket != nil {
		tcpSocket = &core_v1.TCPSocketAction{
			Port: probe.TCPSocket.Port,
			Host: probe.TCPSocket.Host,
		}
	}

	handler := core_v1.Handler{
		Exec:      exec,
		HTTPGet:   httpGet,
		TCPSocket: tcpSocket,
	}

	return &core_v1.Probe{
		Handler:             handler,
		InitialDelaySeconds: probe.InitialDelaySeconds,
		TimeoutSeconds:      probe.TimeoutSeconds,
		PeriodSeconds:       probe.PeriodSeconds,
		SuccessThreshold:    probe.SuccessThreshold,
		FailureThreshold:    probe.FailureThreshold,
	}
}

func (envFromSource *EnvFromSource) toKubeEnvFromSource() core_v1.EnvFromSource {
	kubeEnvFromSource := core_v1.EnvFromSource{
		Prefix: envFromSource.Prefix,
	}

	if envFromSource.ConfigMapRef != nil {
		kubeEnvFromSource.ConfigMapRef = &core_v1.ConfigMapEnvSource{
			LocalObjectReference: core_v1.LocalObjectReference{
				Name: envFromSource.ConfigMapRef.Name,
			},
		}
	}

	return kubeEnvFromSource
}

func (envVar *EnvVar) toKubeEnvVar() core_v1.EnvVar {
	kubeEnvVar := core_v1.EnvVar{
		Name:  envVar.Name,
		Value: envVar.Value,
	}

	if envVar.ValueFrom != nil {
		kubeEnvVarSource := core_v1.EnvVarSource{}

		configMapKeyRef := envVar.ValueFrom.ConfigMapKeyRef
		if configMapKeyRef != nil {
			kubeEnvVarSource.ConfigMapKeyRef = &core_v1.ConfigMapKeySelector{
				Key:                  configMapKeyRef.Key,
				LocalObjectReference: core_v1.LocalObjectReference{Name: configMapKeyRef.Name},
			}
		}

		kubeEnvVar.ValueFrom = &kubeEnvVarSource
	}

	return kubeEnvVar
}
