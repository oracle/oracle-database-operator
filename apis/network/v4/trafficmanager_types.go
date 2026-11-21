package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TrafficManagerType string

const (
	TrafficManagerTypeNginx TrafficManagerType = "nginx"
	TrafficManagerTypeCman  TrafficManagerType = "cman"
)

type TrafficManagerSpec struct {
	// +kubebuilder:validation:Enum=nginx;cman
	// +kubebuilder:validation:XValidation:rule="self == 'nginx' || (self == 'cman' && oldSelf.orValue('') == 'cman')",message="spec.type=cman is only supported for existing TrafficManager resources and cannot be changed; use kind ConnectionManager for new resources",optionalOldSelf=true
	// Type selects the TrafficManager implementation. The cman value is deprecated; use kind ConnectionManager.
	Type     TrafficManagerType         `json:"type,omitempty"`
	Runtime  TrafficManagerRuntimeSpec  `json:"runtime,omitempty"`
	Service  TrafficManagerServiceSpec  `json:"service,omitempty"`
	Security TrafficManagerSecuritySpec `json:"security,omitempty"`
	Nginx    *NginxTrafficManagerSpec   `json:"nginx,omitempty"`
	// Cman contains the legacy CMAN configuration. Deprecated: use kind ConnectionManager.
	Cman *CmanTrafficManagerSpec `json:"cman,omitempty"`
}

type TrafficManagerRuntimeSpec struct {
	Image              string            `json:"image,omitempty"`
	ImagePullPolicy    corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets   []string          `json:"imagePullSecrets,omitempty"`
	ServiceAccountName string            `json:"serviceAccountName,omitempty"`
	Replicas           int32             `json:"replicas,omitempty"`
	// NodeSelector constrains TrafficManager pods to nodes with matching labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Affinity controls node and pod affinity for TrafficManager pods.
	// Preferred pod anti-affinity can spread replicas without blocking rollouts.
	Affinity                 *corev1.Affinity             `json:"affinity,omitempty"`
	Resources                *corev1.ResourceRequirements `json:"resources,omitempty"`
	PodSecurityContext       *corev1.PodSecurityContext   `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext      `json:"containerSecurityContext,omitempty"`
	EnvVars                  []TrafficManagerEnvVar       `json:"envVars,omitempty"`
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
	Enabled               *bool                                   `json:"enabled,omitempty"`
	ServiceType           corev1.ServiceType                      `json:"serviceType,omitempty"`
	Port                  int32                                   `json:"port,omitempty"`
	TargetPort            int32                                   `json:"targetPort,omitempty"`
	Annotations           map[string]string                       `json:"annotations,omitempty"`
	ExternalTrafficPolicy corev1.ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty"`
	LoadBalancerIP        string                                  `json:"loadBalancerIP,omitempty"`
	LoadBalancerClass     *string                                 `json:"loadBalancerClass,omitempty"`
	Ports                 []TrafficManagerServicePortSpec         `json:"ports,omitempty"`
}

type TrafficManagerServicePortSpec struct {
	Name       string          `json:"name,omitempty"`
	Port       int32           `json:"port,omitempty"`
	TargetPort int32           `json:"targetPort,omitempty"`
	NodePort   int32           `json:"nodePort,omitempty"`
	Protocol   corev1.Protocol `json:"protocol,omitempty"`
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

// CmanTrafficManagerSpec contains the legacy CMAN configuration. Deprecated: use kind ConnectionManager.
type CmanTrafficManagerSpec struct {
	// Deprecated: use spec.logLevel on kind ConnectionManager.
	LogLevel string `json:"logLevel,omitempty"`
	// Deprecated: use spec.traceLevel on kind ConnectionManager.
	TraceLevel string `json:"traceLevel,omitempty"`
	// Deprecated: use spec.registrationInvitedNodes on kind ConnectionManager.
	RegistrationInvitedNodes string `json:"registrationInvitedNodes,omitempty"`
	// Deprecated: use spec.configSource on kind ConnectionManager.
	ConfigSource *CmanTrafficManagerConfigSourceSpec `json:"configSource,omitempty"`
	// Deprecated: use spec.rules on kind ConnectionManager.
	Rules []CmanTrafficManagerRuleSpec `json:"rules,omitempty"`
	// Deprecated: use spec.restApi on kind ConnectionManager.
	RestAPI *CmanTrafficManagerRestAPISpec `json:"restApi,omitempty"`
}

// CmanTrafficManagerConfigSourceSpec identifies a legacy CMAN configuration ConfigMap. Deprecated: use kind ConnectionManager.
type CmanTrafficManagerConfigSourceSpec struct {
	// Deprecated: use spec.configSource.configMapRef on kind ConnectionManager.
	ConfigMapRef *TrafficManagerConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// CmanTrafficManagerRuleSpec defines a legacy CMAN routing rule. Deprecated: use kind ConnectionManager.
type CmanTrafficManagerRuleSpec struct {
	// Deprecated: use spec.rules[].host on kind ConnectionManager.
	Host string `json:"host,omitempty"`
	// Deprecated: use spec.rules[].ip on kind ConnectionManager.
	IP string `json:"ip,omitempty"`
	// Deprecated: use spec.rules[].src on kind ConnectionManager.
	Src string `json:"src,omitempty"`
	// Deprecated: use spec.rules[].dst on kind ConnectionManager.
	Dst string `json:"dst,omitempty"`
	// Deprecated: use spec.rules[].srv on kind ConnectionManager.
	Srv string `json:"srv,omitempty"`
	// Deprecated: use spec.rules[].action on kind ConnectionManager.
	Action string `json:"action,omitempty"`
}

// CmanTrafficManagerRestAPISpec defines the legacy CMAN REST API configuration. Deprecated: use kind ConnectionManager.
type CmanTrafficManagerRestAPISpec struct {
	// Deprecated: use spec.restApi.enabled on kind ConnectionManager.
	Enabled bool `json:"enabled,omitempty"`
	// Deprecated: use spec.restApi.host on kind ConnectionManager.
	Host string `json:"host,omitempty"`
	// Deprecated: use spec.restApi.port on kind ConnectionManager.
	Port int32 `json:"port,omitempty"`
	// Deprecated: use spec.restApi.passwordSecretRef on kind ConnectionManager.
	PasswordSecretRef *TrafficManagerSecretKeyRef `json:"passwordSecretRef,omitempty"`
	// Deprecated: use spec.restApi.privateKeySecretRef on kind ConnectionManager.
	PrivateKeySecretRef *TrafficManagerSecretKeyRef `json:"privateKeySecretRef,omitempty"`
}

// TrafficManagerConfigMapKeyRef identifies a legacy CMAN configuration key. Deprecated: use kind ConnectionManager.
type TrafficManagerConfigMapKeyRef struct {
	// Deprecated: use spec.configSource.configMapRef.name on kind ConnectionManager.
	Name string `json:"name,omitempty"`
	// Deprecated: use spec.configSource.configMapRef.key on kind ConnectionManager.
	Key string `json:"key,omitempty"`
}

// TrafficManagerSecretKeyRef identifies a legacy CMAN secret key. Deprecated: use kind ConnectionManager.
type TrafficManagerSecretKeyRef struct {
	// Deprecated: use spec.restApi.*SecretRef.name on kind ConnectionManager.
	Name string `json:"name,omitempty"`
	// Deprecated: use spec.restApi.*SecretRef.key on kind ConnectionManager.
	Key string `json:"key,omitempty"`
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
	// Cman contains legacy CMAN status. Deprecated: use kind ConnectionManager.
	Cman       *CmanTrafficManagerStatus `json:"cman,omitempty"`
	Conditions []metav1.Condition        `json:"conditions,omitempty"`
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

// CmanTrafficManagerStatus contains legacy CMAN status. Deprecated: use kind ConnectionManager.
type CmanTrafficManagerStatus struct {
	// Deprecated: use status.cman.configMode on kind ConnectionManager.
	ConfigMode string `json:"configMode,omitempty"`
	// Deprecated: use status.cman.restHost on kind ConnectionManager.
	RestHost string `json:"restHost,omitempty"`
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
// +kubebuilder:resource:shortName=trm

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
