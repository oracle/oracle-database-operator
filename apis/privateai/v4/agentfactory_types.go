/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AgentFactorySpec defines the desired state of AgentFactory.
type AgentFactorySpec struct {
	Runtime       *AgentFactoryRuntimeSpec       `json:"runtime,omitempty"`
	Configuration *AgentFactoryConfigurationSpec `json:"configuration,omitempty"`
	Database      *AgentFactoryDatabaseSpec      `json:"database,omitempty"`
	Security      *AgentFactorySecuritySpec      `json:"security,omitempty"`
	Storage       *AgentFactoryStorageSpec       `json:"storage,omitempty"`
	Networking    *AgentFactoryNetworkingSpec    `json:"networking,omitempty"`
	Logging       *AgentFactoryLoggingSpec       `json:"logging,omitempty"`
	Upgrade       *AgentFactoryUpgradeSpec       `json:"upgrade,omitempty"`
}

// AgentFactoryRuntimeSpec groups image, scaling, resources, and pod metadata.
type AgentFactoryRuntimeSpec struct {
	Image       *AgentFactoryImageSpec       `json:"image,omitempty"`
	Replicas    *int32                       `json:"replicas,omitempty"`
	Resources   *corev1.ResourceRequirements `json:"resources,omitempty"`
	Env         []EnvironmentVariable        `json:"env,omitempty"`
	WorkerNodes []string                     `json:"workerNodes,omitempty"`
	// WorkerNodeSelector restricts AgentFactory Pods to nodes carrying all of
	// the specified labels. It is mutually exclusive with workerNodes.
	// +kubebuilder:validation:MinProperties=1
	WorkerNodeSelector map[string]string `json:"workerNodeSelector,omitempty"`
	// ServiceAccountName selects the ServiceAccount used by generated Pods. If
	// omitted, Kubernetes uses the namespace default ServiceAccount.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +kubebuilder:default=false
	AutomountServiceAccountToken *bool               `json:"automountServiceAccountToken,omitempty"`
	Annotations                  *PaiAnnotationsSpec `json:"annotations,omitempty"`
}

// AgentFactoryImageSpec defines the Agent Factory image settings.
type AgentFactoryImageSpec struct {
	Name       string            `json:"name,omitempty"`
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	PullSecret string            `json:"pullSecret,omitempty"`
}

// AgentFactoryConfigurationSpec defines runtime.env values that are not secret.
type AgentFactoryConfigurationSpec struct {
	// +kubebuilder:default=prod
	Mode string `json:"mode,omitempty"`
	// +kubebuilder:default=true
	SilentInstall   *bool  `json:"silentInstall,omitempty"`
	UIAdminUsername string `json:"uiAdminUsername,omitempty"`
	// +kubebuilder:default="0.0.0.0"
	BindHost      string `json:"bindHost,omitempty"`
	CertFQDN      string `json:"certFqdn,omitempty"`
	CertIPAddress string `json:"certIpAddress,omitempty"`
	// +kubebuilder:default=false
	EnableSwagger            *bool                `json:"enableSwagger,omitempty"`
	StatePollIntervalSeconds *int32               `json:"statePollIntervalSeconds,omitempty"`
	APIReadyTimeoutSeconds   *int32               `json:"apiReadyTimeoutSeconds,omitempty"`
	InstallTimeoutSeconds    *int32               `json:"installTimeoutSeconds,omitempty"`
	ReadyTimeoutSeconds      *int32               `json:"readyTimeoutSeconds,omitempty"`
	HTTPProxy                string               `json:"httpProxy,omitempty"`
	HTTPSProxy               string               `json:"httpsProxy,omitempty"`
	NoProxy                  string               `json:"noProxy,omitempty"`
	NginxBindHost            string               `json:"nginxBindHost,omitempty"`
	LLM                      *AgentFactoryLLMSpec `json:"llm,omitempty"`
}

// AgentFactoryLLMSpec defines the optional provider-specific LLM setup.
type AgentFactoryLLMSpec struct {
	Provider          string                                `json:"provider,omitempty"`
	Name              string                                `json:"name,omitempty"`
	OCI               *AgentFactoryLLMOCISpec               `json:"oci,omitempty"`
	VLLM              *AgentFactoryLLMVLLMSpec              `json:"vllm,omitempty"`
	OpenAI            *AgentFactoryLLMOpenAISpec            `json:"openai,omitempty"`
	OpenAICompatible  *AgentFactoryLLMOpenAICompatibleSpec  `json:"openaiCompatible,omitempty"`
	PrivateAIServices *AgentFactoryLLMPrivateAIServicesSpec `json:"privateAiServices,omitempty"`
	Ollama            *AgentFactoryLLMOllamaSpec            `json:"ollama,omitempty"`
	Google            *AgentFactoryLLMGoogleSpec            `json:"google,omitempty"`
}

type AgentFactoryLLMOCISpec struct {
	ModelID         string `json:"modelId,omitempty"`
	ServiceEndpoint string `json:"serviceEndpoint,omitempty"`
	CompartmentID   string `json:"compartmentId,omitempty"`
	UserOCID        string `json:"userOcid,omitempty"`
	FingerprintFile string `json:"fingerprintFile,omitempty"`
	TenancyOCID     string `json:"tenancyOcid,omitempty"`
	Region          string `json:"region,omitempty"`
	PrivateKeyFile  string `json:"privateKeyFile,omitempty"`
}

type AgentFactoryLLMVLLMSpec struct {
	ModelID    string `json:"modelId,omitempty"`
	Host       string `json:"host,omitempty"`
	Port       int32  `json:"port,omitempty"`
	APIKeyFile string `json:"apiKeyFile,omitempty"`
}

type AgentFactoryLLMOpenAISpec struct {
	ModelID    string `json:"modelId,omitempty"`
	APIKeyFile string `json:"apiKeyFile,omitempty"`
}

type AgentFactoryLLMOpenAICompatibleSpec struct {
	ModelID    string `json:"modelId,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
	APIKeyFile string `json:"apiKeyFile,omitempty"`
}

type AgentFactoryLLMPrivateAIServicesSpec struct {
	ModelID    string `json:"modelId,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
	APIKeyFile string `json:"apiKeyFile,omitempty"`
}

type AgentFactoryLLMOllamaSpec struct {
	ModelID string `json:"modelId,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int32  `json:"port,omitempty"`
}

type AgentFactoryLLMGoogleSpec struct {
	ModelID            string `json:"modelId,omitempty"`
	CredentialsType    string `json:"credentialsType,omitempty"`
	APIKeyFile         string `json:"apiKeyFile,omitempty"`
	Region             string `json:"region,omitempty"`
	ServiceAccountFile string `json:"serviceAccountFile,omitempty"`
}

// AgentFactoryDatabaseSpec defines Agent Factory database connection settings.
type AgentFactoryDatabaseSpec struct {
	// +kubebuilder:validation:Enum=Wallet;Details
	ConnectionType string                             `json:"connectionType,omitempty"`
	Username       string                             `json:"username,omitempty"`
	AirGapped      *bool                              `json:"airGapped,omitempty"`
	Wallet         *AgentFactoryWalletSpec            `json:"wallet,omitempty"`
	Details        *AgentFactoryDatabaseDetailsSpec   `json:"details,omitempty"`
	Bootstrap      *AgentFactoryDatabaseBootstrapSpec `json:"bootstrap,omitempty"`
	// +kubebuilder:default="/agentFactory/v1/getAppState"
	HealthPath string `json:"healthPath,omitempty"`
}

// AgentFactoryWalletSpec defines wallet-mode database settings.
type AgentFactoryWalletSpec struct {
	SelectedTnsAlias   string `json:"selectedTnsAlias,omitempty"`
	HasOciCertificates *bool  `json:"hasOciCertificates,omitempty"`
}

// AgentFactoryDatabaseDetailsSpec defines host/service database settings.
type AgentFactoryDatabaseDetailsSpec struct {
	// +kubebuilder:validation:Enum=TCP;TCPS
	Protocol    string `json:"protocol,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int32  `json:"port,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

// AgentFactoryDatabaseBootstrapSpec controls the optional one-time database setup Job.
// The Job uses the administrator credentials to create the PAF user and the
// PAF credentials from spec.security.secret to create and validate the schema.
type AgentFactoryDatabaseBootstrapSpec struct {
	// +kubebuilder:default=false
	Enabled *bool `json:"enabled,omitempty"`

	AdminCredentials *AgentFactoryAdminCredentialsSpec `json:"adminCredentials,omitempty"`

	// +kubebuilder:validation:Enum=Retain;DeleteOnSuccess
	// +kubebuilder:default=Retain
	CleanupPolicy string `json:"cleanupPolicy,omitempty"`

	// PreserveFailedJob enables diagnostic mode for the bootstrap Job. When
	// enabled, the controller keeps the failed Job and Pod instead of applying
	// the normal retry and finished-Job cleanup settings. The default remains
	// false for customer workloads.
	// +kubebuilder:default=false
	PreserveFailedJob bool `json:"preserveFailedJob,omitempty"`

	// Image is the image used by the setup Job. When empty, the AgentFactory
	// runtime image is used and the controller-provided setup script is mounted
	// at /usr/local/bin/paf-db-setup.
	Image string `json:"image,omitempty"`
}

// AgentFactoryAdminCredentialsSpec identifies a Secret containing the temporary
// database administrator username and password used by the setup Job. Privilege
// selects the Oracle connection mode independently from the username.
type AgentFactoryAdminCredentialsSpec struct {
	SecretName  string `json:"secretName"`
	UsernameKey string `json:"usernameKey,omitempty"`
	PasswordKey string `json:"passwordKey,omitempty"`
	// +kubebuilder:validation:Enum=NORMAL;SYSDBA
	// +kubebuilder:default=NORMAL
	// Privilege is a connection mode, not part of the username. For example,
	// SYS AS SYSDBA is represented as username SYS and privilege SYSDBA.
	Privilege string `json:"privilege,omitempty"`
}

// AgentFactorySecuritySpec groups runtime and inbound/outbound TLS Secret references.
type AgentFactorySecuritySpec struct {
	Secret      *AgentFactorySecretSpec      `json:"secret,omitempty"`
	InboundTLS  *AgentFactoryInboundTLSSpec  `json:"inboundTLS,omitempty"`
	OutboundTLS *AgentFactoryOutboundTLSSpec `json:"outboundTLS,omitempty"`
}

// AgentFactoryOutboundTLSSpec defines the CA bundle used to trust outbound
// HTTPS services. The bundle may contain one or more PEM-encoded CA
// certificates and is mounted separately from the runtime credential Secret.
// It must not contain a private key.
type AgentFactoryOutboundTLSSpec struct {
	CABundle *AgentFactoryOutboundCABundleSpec `json:"caBundle,omitempty"`
}

// AgentFactoryOutboundCABundleSpec references a Secret key containing the
// outbound CA bundle. The key defaults to ca.crt when omitted.
type AgentFactoryOutboundCABundleSpec struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key,omitempty"`
}

// AgentFactorySecretSpec defines the secret mounted at /run/secrets by default.
type AgentFactorySecretSpec struct {
	Name               string `json:"name,omitempty"`
	MountLocation      string `json:"mountLocation,omitempty"`
	UIAdminPasswordKey string `json:"uiAdminPasswordKey,omitempty"`
	DBPasswordKey      string `json:"dbPasswordKey,omitempty"`
	WalletZipKey       string `json:"walletZipKey,omitempty"`
	// Optional Secret data key containing the password for an encrypted wallet
	// private key. The controller projects it into the fixed
	// agent_factory_db_wallet_password filename without exposing the value in
	// runtime.env, status, or annotations.
	WalletPasswordKey string `json:"walletPasswordKey,omitempty"`
}

// AgentFactoryStorageSpec defines volumes mounted into Agent Factory pods.
type AgentFactoryStorageSpec struct {
	Volumes []AgentFactoryVolumeSpec `json:"volumes,omitempty"`
}

// AgentFactoryVolumeSpec defines one named volume and mount path.
type AgentFactoryVolumeSpec struct {
	Name      string                   `json:"name"`
	MountPath string                   `json:"mountPath"`
	Source    AgentFactoryVolumeSource `json:"source"`
}

// AgentFactoryVolumeSource selects exactly one Kubernetes volume source.
type AgentFactoryVolumeSource struct {
	ExistingPVC         *AgentFactoryExistingPVCSource         `json:"existingPVC,omitempty"`
	PVC                 *AgentFactoryPVCSource                 `json:"pvc,omitempty"`
	VolumeClaimTemplate *AgentFactoryVolumeClaimTemplateSource `json:"volumeClaimTemplate,omitempty"`
	EmptyDir            *AgentFactoryEmptyDirSource            `json:"emptyDir,omitempty"`
}

// AgentFactoryExistingPVCSource mounts a user-managed PVC.
type AgentFactoryExistingPVCSource struct {
	ClaimName string `json:"claimName"`
}

// AgentFactoryPVCSource creates one PVC that is mounted by all replicas.
type AgentFactoryPVCSource struct {
	StorageClass string                              `json:"storageClass,omitempty"`
	Size         string                              `json:"size"`
	AccessModes  []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// +kubebuilder:default=false
	DeletePvcOnDelete *bool `json:"deletePvcOnDelete,omitempty"`
}

// AgentFactoryVolumeClaimTemplateSource is not supported by the AgentFactory Deployment controller.
type AgentFactoryVolumeClaimTemplateSource struct {
	StorageClass string                              `json:"storageClass,omitempty"`
	Size         string                              `json:"size"`
	AccessModes  []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// +kubebuilder:default=false
	DeletePvcOnDelete *bool `json:"deletePvcOnDelete,omitempty"`
}

// AgentFactoryEmptyDirSource creates pod-local ephemeral storage.
type AgentFactoryEmptyDirSource struct {
	SizeLimit string `json:"sizeLimit,omitempty"`
}

// AgentFactoryNetworkingSpec groups listeners and services.
type AgentFactoryNetworkingSpec struct {
	Listeners *AgentFactoryListenersSpec `json:"listeners,omitempty"`
	Service   *PaiServiceSpec            `json:"service,omitempty"`
}

// AgentFactoryListenersSpec defines HTTP/HTTPS listener settings.
type AgentFactoryListenersSpec struct {
	HTTP  *PrivateAiListenerSpec `json:"http,omitempty"`
	HTTPS *PrivateAiListenerSpec `json:"https,omitempty"`
}

// AgentFactoryLoggingSpec defines Agent Factory logging options.
type AgentFactoryLoggingSpec struct {
	Enabled bool                            `json:"enabled,omitempty"`
	Path    string                          `json:"path,omitempty"`
	Sidecar *AgentFactoryLoggingSidecarSpec `json:"sidecar,omitempty"`
}

// AgentFactoryLoggingSidecarSpec describes an optional log-forwarder sidecar.
type AgentFactoryLoggingSidecarSpec struct {
	Image     string `json:"image,omitempty"`
	ConfigMap string `json:"configMap,omitempty"`
}

// AgentFactoryUpgradeSpec controls rollout behavior when inputs change.
type AgentFactoryUpgradeSpec struct {
	// Strategy selects the Deployment strategy type.
	// ImageUpgrade performs a one-shot schema upgrade in a dedicated Pod using
	// the new image before the normal Deployment is updated.
	// +kubebuilder:validation:Enum=RollingUpdate;Recreate;ImageUpgrade
	// +kubebuilder:default=RollingUpdate
	Strategy string `json:"strategy,omitempty"`
	// RollingUpdate configures the Deployment rolling-update behavior. It is
	// used only when strategy is RollingUpdate.
	RollingUpdate *AgentFactoryRollingUpdateSpec `json:"rollingUpdate,omitempty"`
	// TerminationGracePeriodSeconds is the time allowed for an AgentFactory Pod
	// to terminate gracefully during a rollout.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3600
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`
	// +kubebuilder:validation:Enum=Rollout;NoRestart
	// +kubebuilder:default=Rollout
	ConfigChangePolicy string `json:"configChangePolicy,omitempty"`
	// +kubebuilder:validation:Enum=Rollout;NoRestart
	// +kubebuilder:default=Rollout
	SecretChangePolicy string `json:"secretChangePolicy,omitempty"`
	// ImageUpgrade configures the dedicated Pod used when strategy is
	// ImageUpgrade. The Pod always runs the new image with the fixed `upgrade`
	// argument and is never part of the normal Service or Deployment selector.
	ImageUpgrade *AgentFactoryImageUpgradeSpec `json:"imageUpgrade,omitempty"`
}

// AgentFactoryImageUpgradeSpec controls the one-shot image upgrade Pod.
type AgentFactoryImageUpgradeSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3600
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
	// Failed upgrade Pods are retained by default for diagnosis. A successful
	// upgrade Pod is always deleted after the command completes.
	// +kubebuilder:default=true
	PreserveFailedPod *bool `json:"preserveFailedPod,omitempty"`
}

// AgentFactoryRollingUpdateSpec defines the rolling-update limits for an
// AgentFactory Deployment. Each value accepts either an absolute number or a
// percentage, following the Kubernetes Deployment API.
type AgentFactoryRollingUpdateSpec struct {
	// +kubebuilder:default="25%"
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
	// +kubebuilder:default=0
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// AgentFactoryStatus defines the observed state of AgentFactory.
type AgentFactoryStatus struct {
	Phase                      string                         `json:"phase,omitempty"`
	Replicas                   int32                          `json:"replicas,omitempty"`
	ReadyReplicas              int32                          `json:"readyReplicas,omitempty"`
	Image                      string                         `json:"image,omitempty"`
	AppState                   string                         `json:"appState,omitempty"`
	WorkloadType               string                         `json:"workloadType,omitempty"`
	LocalService               string                         `json:"localService,omitempty"`
	PublicLoadBalancerService  string                         `json:"publicLoadBalancerService,omitempty"`
	PrivateLoadBalancerService string                         `json:"privateLoadBalancerService,omitempty"`
	LoadBalancerIP             string                         `json:"loadBalancerIP,omitempty"`
	RuntimeConfig              AgentFactoryConfigMapStatus    `json:"runtimeConfig,omitempty"`
	Secret                     AgentFactorySecretStatus       `json:"secret,omitempty"`
	Database                   AgentFactoryDatabaseStatus     `json:"database,omitempty"`
	InboundTLS                 AgentFactoryInboundTLSStatus   `json:"inboundTLS,omitempty"`
	OutboundTLS                AgentFactoryOutboundTLSStatus  `json:"outboundTLS,omitempty"`
	ImageUpgrade               AgentFactoryImageUpgradeStatus `json:"imageUpgrade,omitempty"`
	Storage                    []AgentFactoryVolumeStatus     `json:"storage,omitempty"`
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// AgentFactoryImageUpgradeStatus reports the dedicated image-upgrade Pod.
// Secret values are never stored in status.
type AgentFactoryImageUpgradeStatus struct {
	Phase              string `json:"phase,omitempty"`
	SourceImage        string `json:"sourceImage,omitempty"`
	TargetImage        string `json:"targetImage,omitempty"`
	PodName            string `json:"podName,omitempty"`
	Replicas           int32  `json:"replicas,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	Message            string `json:"message,omitempty"`
}

// AgentFactoryOutboundTLSStatus tracks the Secret versions used for outbound
// TLS. A resource-version change causes the workload template to roll out.
// The imported version is updated only after the Agent Factory certificate API
// accepts the CA bundle.
type AgentFactoryOutboundTLSStatus struct {
	CABundleSecretName              string `json:"caBundleSecretName,omitempty"`
	CABundleResourceVersion         string `json:"caBundleResourceVersion,omitempty"`
	CABundleImportedResourceVersion string `json:"caBundleImportedResourceVersion,omitempty"`
}

// AgentFactoryDatabaseStatus reports the optional database setup result.
type AgentFactoryDatabaseStatus struct {
	Phase            string `json:"phase,omitempty"`
	BootstrapJobName string `json:"bootstrapJobName,omitempty"`
	PAFUser          string `json:"pafUser,omitempty"`
	Message          string `json:"message,omitempty"`
}

// AgentFactoryConfigMapStatus tracks the generated runtime ConfigMap.
type AgentFactoryConfigMapStatus struct {
	Name            string `json:"name,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// AgentFactorySecretStatus tracks the referenced runtime Secret.
type AgentFactorySecretStatus struct {
	Name               string `json:"name,omitempty"`
	ResourceVersion    string `json:"resourceVersion,omitempty"`
	HasUIAdminPassword bool   `json:"hasUiAdminPassword,omitempty"`
	HasDBPassword      bool   `json:"hasDbPassword,omitempty"`
	HasWalletZip       bool   `json:"hasWalletZip,omitempty"`
	HasWalletPassword  bool   `json:"hasWalletPassword,omitempty"`
}

// AgentFactoryVolumeStatus tracks resolved storage.
type AgentFactoryVolumeStatus struct {
	Name       string `json:"name,omitempty"`
	MountPath  string `json:"mountPath,omitempty"`
	ClaimName  string `json:"claimName,omitempty"`
	Managed    bool   `json:"managed,omitempty"`
	PerReplica bool   `json:"perReplica,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.phase",name="Phase",type=string
// +kubebuilder:printcolumn:JSONPath=".status.readyReplicas",name="Ready",type=number
// +kubebuilder:printcolumn:JSONPath=".status.replicas",name="Replicas",type=number
// +kubebuilder:printcolumn:JSONPath=".status.workloadType",name="Workload",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.localService",name="ClusterSvc",type=string,priority=1
// +kubebuilder:resource:shortName=af

// AgentFactory is the Schema for the agentfactories API.
type AgentFactory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentFactorySpec   `json:"spec,omitempty"`
	Status AgentFactoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentFactoryList contains a list of AgentFactory.
type AgentFactoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentFactory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentFactory{}, &AgentFactoryList{})
}

// AgentFactoryInboundTLSSpec references the Secret containing the certificate
// and private key Agent Factory uses for inbound HTTPS. The Secret is mounted
// read-only and the controller exposes the resulting file paths through
// AGENT_FACTORY_SERVER_CERTIFICATE_FILE and
// AGENT_FACTORY_SERVER_PRIVATE_KEY_FILE.
type AgentFactoryInboundTLSSpec struct {
	SecretName string `json:"secretName"`
	// +kubebuilder:default="/run/agentfactory/inbound-tls"
	MountLocation string `json:"mountLocation,omitempty"`
	// +kubebuilder:default="tls.crt"
	CertificateKey string `json:"certificateKey,omitempty"`
	// +kubebuilder:default="tls.key"
	PrivateKeyKey string `json:"privateKeyKey,omitempty"`
}

// AgentFactoryInboundTLSStatus tracks the inbound TLS Secret version mounted
// into the workload and the version successfully imported before the latest
// restart. Secret data is never stored in status.
type AgentFactoryInboundTLSStatus struct {
	SecretName              string `json:"secretName,omitempty"`
	ResourceVersion         string `json:"resourceVersion,omitempty"`
	CertificateKey          string `json:"certificateKey,omitempty"`
	PrivateKeyKey           string `json:"privateKeyKey,omitempty"`
	MountLocation           string `json:"mountLocation,omitempty"`
	ImportedResourceVersion string `json:"importedResourceVersion,omitempty"`
}
