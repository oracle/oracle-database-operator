package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TrafficManagerType string

const (
	TrafficManagerTypeNginx TrafficManagerType = "nginx"
)

type TrafficManagerSpec struct {
	// +kubebuilder:validation:Enum=nginx
	Type     TrafficManagerType         `json:"type,omitempty"`
	Runtime  TrafficManagerRuntimeSpec  `json:"runtime,omitempty"`
	Service  TrafficManagerServiceSpec  `json:"service,omitempty"`
	Security TrafficManagerSecuritySpec `json:"security,omitempty"`
	Nginx    *NginxTrafficManagerSpec   `json:"nginx,omitempty"`
}

type TrafficManagerRuntimeSpec struct {
	Image            string                       `json:"image,omitempty"`
	ImagePullPolicy  corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []string                     `json:"imagePullSecrets,omitempty"`
	Replicas         int32                        `json:"replicas,omitempty"`
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty"`
	EnvVars          []TrafficManagerEnvVar       `json:"envVars,omitempty"`
}

type TrafficManagerEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TrafficManagerServiceSpec struct {
	Internal TrafficManagerServiceEndpointSpec `json:"internal,omitempty"`
	External TrafficManagerServiceEndpointSpec `json:"external,omitempty"`
}

type TrafficManagerServiceEndpointSpec struct {
	Enabled     *bool              `json:"enabled,omitempty"`
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	Port        int32              `json:"port,omitempty"`
	TargetPort  int32              `json:"targetPort,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

type TrafficManagerSecuritySpec struct {
	TLS        TrafficManagerTLSSpec         `json:"tls,omitempty"`
	BackendTLS *TrafficManagerBackendTLSSpec `json:"backendTLS,omitempty"`
}

type TrafficManagerTLSSpec struct {
	Enabled       bool   `json:"enabled,omitempty"`
	SecretName    string `json:"secretName,omitempty"`
	MountLocation string `json:"mountLocation,omitempty"`
}

type TrafficManagerBackendTLSSpec struct {
	TrustSecretName string `json:"trustSecretName,omitempty"`
	MountLocation   string `json:"mountLocation,omitempty"`
	TrustFileName   string `json:"trustFileName,omitempty"`
	Verify          *bool  `json:"verify,omitempty"`
}

type NginxTrafficManagerSpec struct {
	Config *TrafficManagerConfigSpec `json:"config,omitempty"`
}

type TrafficManagerConfigSpec struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	MountLocation string `json:"mountLocation,omitempty"`
}

type TrafficManagerStatus struct {
	Status           string                     `json:"status,omitempty"`
	Type             string                     `json:"type,omitempty"`
	ReadyReplicas    int32                      `json:"readyReplicas,omitempty"`
	InternalService  string                     `json:"internalService,omitempty"`
	ExternalService  string                     `json:"externalService,omitempty"`
	ExternalEndpoint string                     `json:"externalEndpoint,omitempty"`
	Nginx            *NginxTrafficManagerStatus `json:"nginx,omitempty"`
	Conditions       []metav1.Condition         `json:"conditions,omitempty"`
}

type NginxTrafficManagerStatus struct {
	ConfigMapName      string             `json:"configMapName,omitempty"`
	AssociatedBackends []string           `json:"associatedBackends,omitempty"`
	BackendCount       int32              `json:"backendCount,omitempty"`
	ConfigMode         string             `json:"configMode,omitempty"`
	TLSEnabled         bool               `json:"tlsEnabled,omitempty"`
	TLSSecretName      string             `json:"tlsSecretName,omitempty"`
	BackendTLSEnabled  bool               `json:"backendTlsEnabled,omitempty"`
	BackendTrustSecret string             `json:"backendTrustSecret,omitempty"`
	Routes             []NginxRouteStatus `json:"routes,omitempty"`
}

type NginxRouteStatus struct {
	Path           string `json:"path,omitempty"`
	BackendName    string `json:"backendName,omitempty"`
	BackendService string `json:"backendService,omitempty"`
	BackendURL     string `json:"backendURL,omitempty"`
	PublicURL      string `json:"publicURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
//+kubebuilder:printcolumn:JSONPath=".status.type",name="Type",type=string
//+kubebuilder:printcolumn:JSONPath=".status.readyReplicas",name="Ready",type=number
//+kubebuilder:printcolumn:JSONPath=".status.externalEndpoint",name="Endpoint",type=string
//+kubebuilder:printcolumn:JSONPath=".status.nginx.backendCount",name="Backends",type=number
//+kubebuilder:printcolumn:JSONPath=".status.nginx.tlsEnabled",name="TLS",type=boolean
//+kubebuilder:printcolumn:JSONPath=".status.nginx.configMode",name="Config",type=string
//+kubebuilder:printcolumn:JSONPath=".status.internalService",name="IntSvc",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.externalService",name="ExtSvc",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.nginx.configMapName",name="ConfigMap",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.nginx.tlsSecretName",name="TLSSecret",type=string,priority=1
// +kubebuilder:resource:shortName=cman;connectionmanager;trm

type TrafficManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficManagerSpec   `json:"spec,omitempty"`
	Status TrafficManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type TrafficManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TrafficManager{}, &TrafficManagerList{})
}
