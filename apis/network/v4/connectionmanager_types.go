package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConnectionManagerRuntimeSpec struct {
	Image string `json:"image,omitempty"`
	// Port is the CMAN listener port. It defaults to 1521 when no port is configured.
	// +kubebuilder:validation:Maximum=65535
	Port                     int32                               `json:"port,omitempty"`
	ImagePullPolicy          corev1.PullPolicy                   `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets         []string                            `json:"imagePullSecrets,omitempty"`
	ServiceAccountName       string                              `json:"serviceAccountName,omitempty"`
	Replicas                 int32                               `json:"replicas,omitempty"`
	NodeSelector             map[string]string                   `json:"nodeSelector,omitempty"`
	Affinity                 *corev1.Affinity                    `json:"affinity,omitempty"`
	Resources                *corev1.ResourceRequirements        `json:"resources,omitempty"`
	PodSecurityContext       *corev1.PodSecurityContext          `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext             `json:"containerSecurityContext,omitempty"`
	EnvVars                  []ConnectionManagerEnvVar           `json:"envVars,omitempty"`
	StartupScript            *ConnectionManagerStartupScriptSpec `json:"startupScript,omitempty"`
}

// ConnectionManagerMaintenanceSpec controls whether the CMAN pod runs in operator-managed maintenance mode.
type ConnectionManagerMaintenanceSpec struct {
	Enabled bool `json:"enabled,omitempty"`
}

type ConnectionManagerEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ConnectionManagerServiceSpec struct {
	Internal ConnectionManagerServiceEndpointSpec `json:"internal,omitempty"`
	External ConnectionManagerServiceEndpointSpec `json:"external,omitempty"`
}

type ConnectionManagerServiceEndpointSpec struct {
	Enabled               *bool                                   `json:"enabled,omitempty"`
	ServiceType           corev1.ServiceType                      `json:"serviceType,omitempty"`
	Port                  int32                                   `json:"port,omitempty"`
	TargetPort            int32                                   `json:"targetPort,omitempty"`
	Annotations           map[string]string                       `json:"annotations,omitempty"`
	ExternalTrafficPolicy corev1.ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty"`
	LoadBalancerIP        string                                  `json:"loadBalancerIP,omitempty"`
	LoadBalancerClass     *string                                 `json:"loadBalancerClass,omitempty"`
	Ports                 []ConnectionManagerServicePortSpec      `json:"ports,omitempty"`
}

type ConnectionManagerServicePortSpec struct {
	Name       string          `json:"name,omitempty"`
	Port       int32           `json:"port,omitempty"`
	TargetPort int32           `json:"targetPort,omitempty"`
	NodePort   int32           `json:"nodePort,omitempty"`
	Protocol   corev1.Protocol `json:"protocol,omitempty"`
}

type ConnectionManagerSpec struct {
	Runtime                  ConnectionManagerRuntimeSpec       `json:"runtime,omitempty"`
	Maintenance              *ConnectionManagerMaintenanceSpec  `json:"maintenance,omitempty"`
	Service                  ConnectionManagerServiceSpec       `json:"service,omitempty"`
	LogLevel                 string                             `json:"logLevel,omitempty"`
	TraceLevel               string                             `json:"traceLevel,omitempty"`
	RegistrationInvitedNodes string                             `json:"registrationInvitedNodes,omitempty"`
	ConfigSource             *ConnectionManagerConfigSourceSpec `json:"configSource,omitempty"`
	Storage                  *ConnectionManagerStorageSpec      `json:"storage,omitempty"`
	Rules                    []ConnectionManagerRuleSpec        `json:"rules,omitempty"`
	RestAPI                  *ConnectionManagerRestAPISpec      `json:"restApi,omitempty"`
}

type ConnectionManagerConfigSourceSpec struct {
	ConfigMapRef *ConnectionManagerConfigMapKeyRef `json:"configMapRef,omitempty"`
	MountPath    string                            `json:"mountPath,omitempty"`
}

type ConnectionManagerConfigMapKeyRef struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type ConnectionManagerStartupScriptSpec struct {
	ConfigMapRef *ConnectionManagerConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// ConnectionManagerStorageSpec defines the persistent storage mounted into the
// CMAN container.
type ConnectionManagerStorageSpec struct {
	// MountPath is the absolute path where the PVC is mounted in the CMAN pod.
	MountPath string `json:"mountPath"`
	// PvcName references an existing PVC. It is mutually exclusive with
	// StorageSizeInGb and StorageClass.
	PvcName string `json:"pvcName,omitempty"`
	// StorageSizeInGb requests an operator-managed PVC when PvcName is empty.
	// StorageClass must also be set in that mode.
	// +kubebuilder:validation:Minimum=1
	StorageSizeInGb int32 `json:"storageSizeInGb,omitempty"`
	// StorageClass is used when creating an operator-managed PVC.
	StorageClass string `json:"storageClass,omitempty"`
	// RetainOnDelete prevents the operator-managed PVC from being deleted with
	// the ConnectionManager resource.
	RetainOnDelete bool `json:"retainOnDelete,omitempty"`
}

type ConnectionManagerRuleSpec struct {
	Host   string `json:"host,omitempty"`
	IP     string `json:"ip,omitempty"`
	Src    string `json:"src,omitempty"`
	Dst    string `json:"dst,omitempty"`
	Srv    string `json:"srv,omitempty"`
	Action string `json:"action,omitempty"`
}

type ConnectionManagerRestAPISpec struct {
	Enabled bool   `json:"enabled,omitempty"`
	Host    string `json:"host,omitempty"`
	// +kubebuilder:validation:Maximum=65535
	Port                int32                          `json:"port,omitempty"`
	PasswordSecretRef   *ConnectionManagerSecretKeyRef `json:"passwordSecretRef,omitempty"`
	PrivateKeySecretRef *ConnectionManagerSecretKeyRef `json:"privateKeySecretRef,omitempty"`
}

type ConnectionManagerSecretKeyRef struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type ConnectionManagerStatusDetails struct {
	ConfigMode string `json:"configMode,omitempty"`
	RestHost   string `json:"restHost,omitempty"`
}

type ConnectionManagerStatus struct {
	Status           string                          `json:"status,omitempty"`
	ReadyReplicas    int32                           `json:"readyReplicas,omitempty"`
	InternalService  string                          `json:"internalService,omitempty"`
	ExternalService  string                          `json:"externalService,omitempty"`
	ExternalEndpoint string                          `json:"externalEndpoint,omitempty"`
	Cman             *ConnectionManagerStatusDetails `json:"cman,omitempty"`
	Conditions       []metav1.Condition              `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.readyReplicas",name="Ready",type=number
// +kubebuilder:printcolumn:JSONPath=".status.externalEndpoint",name="Endpoint",type=string
// +kubebuilder:printcolumn:JSONPath=".status.cman.configMode",name="Config",type=string
// +kubebuilder:resource:shortName=cman;connectionmanager
type ConnectionManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConnectionManagerSpec   `json:"spec,omitempty"`
	Status ConnectionManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ConnectionManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConnectionManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConnectionManager{}, &ConnectionManagerList{})
}
