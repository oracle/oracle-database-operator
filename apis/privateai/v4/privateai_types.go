/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PrivateAiSpec defines the desired state of PrivateAi.
type PrivateAiSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Security      *PrivateAiSecuritySpec      `json:"security,omitempty"`
	Runtime       *PrivateAiRuntimeSpec       `json:"runtime,omitempty"`
	Configuration *PrivateAiConfigurationSpec `json:"configuration,omitempty"`
	Storage       *PrivateAiStorageSpec       `json:"storage,omitempty"`
	Networking    *PrivateAiNetworkingSpec    `json:"networking,omitempty"`
	Logging       *LoggingSpec                `json:"logging,omitempty"`

	// Deprecated: use spec.configuration.configFile.
	PaiConfigFile *PaiConfigMap `json:"paiConfigFile,omitempty"`
	// +kubebuilder:validation:Enum="true";"false"
	// +kubebuilder:default="false"
	// Deprecated: use spec.security.authEnabled.
	PaiEnableAuthentication string `json:"paiEnableAuthentication,omitempty"`
	// Deprecated: use spec.security.secret.
	PaiSecret *PaiSecretSpec `json:"paiSecret,omitempty"`
	// Deprecated: use spec.networking.service.external.enabled.
	// +kubebuilder:validation:Enum="true";"false"
	IsExternalSvc string `json:"isExternalSvc,omitempty"`
	// Deprecated: use spec.storage.storageClass.
	StorageClass string `json:"storageClass,omitempty"`
	// Deprecated: use spec.storage.pvcList.
	PvcList map[string]string `json:"pvcList,omitempty"`
	// Deprecated: use spec.runtime.image.name.
	PaiImage string `json:"paiImage,omitempty"`
	// Deprecated: use spec.runtime.image.pullSecret.
	PaiImagePullSecret string `json:"paiImagePullSecret,omitempty"`
	// Deprecated: use spec.runtime.debug.
	IsDebug bool `json:"isDebug,omitempty"`
	// Deprecated: use spec.runtime.readinessCheckPeriod.
	ReadinessCheckPeriod int `json:"readinessCheckPeriod,omitempty"`
	// Deprecated: use spec.runtime.livenessCheckPeriod.
	LivenessCheckPeriod int `json:"livenessCheckPeriod,omitempty"`
	// Deprecated: use spec.runtime.downloadScripts.
	IsDownloadScripts bool `json:"isDownloadScripts,omitempty"`
	// Deprecated: use spec.networking.service.
	PaiService PaiServiceSpec `json:"paiService,omitempty"`
	// Deprecated: use spec.storage.sizeInGb.
	StorageSizeInGb int32 `json:"storageSizeInGb,omitempty"`
	// Deprecated: use spec.runtime.env.
	EnvVars []EnvironmentVariable `json:"envVars,omitempty"`
	// Deprecated: use spec.runtime.replicas.
	Replicas int32 `json:"replicas,omitempty"`
	// Deprecated: use spec.runtime.resources.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Deprecated: use spec.networking.nodePortServices.
	NodePortSvc []PaiNodePortSvc `json:"nodePortSvc,omitempty"`
	// Deprecated: use spec.networking.service.portMappings.
	PortMappings []PaiPortMapping `json:"portMappings,omitempty"`
	// Deprecated: use spec.storage.deletePvcOnDelete.
	IsDeleteOraPvc bool `json:"isDeleteOraPvc,omitempty"`
	// Deprecated: use spec.storage.logLocation.
	PaiLogLocation string `json:"paiLogLocation,omitempty"`
	// +kubebuilder:validation:Enum="true";"false"
	// Deprecated: use spec.networking.listeners.http.enabled.
	PaiHTTPEnabled string `json:"paiHTTPEnabled,omitempty"`
	// +kubebuilder:validation:Enum="true";"false"
	// Deprecated: use spec.networking.listeners.https.enabled.
	PaiHTTPSEnabled string `json:"paiHTTPSEnabled,omitempty"`
	// Deprecated: use spec.networking.listeners.http.port.
	PaiHTTPPort int32 `json:"paiHTTPPort,omitempty"`
	// Deprecated: use spec.networking.listeners.https.port.
	PaiHTTPSPort int32 `json:"paiHTTPSPort,omitempty"`
	// Deprecated: use spec.networking.service.external.port.
	PaiLBPort int32 `json:"paiLBPort,omitempty"`
	// Deprecated: use spec.networking.service.external.loadBalancerIP.
	PaiLBIP string `json:"paiLBIP,omitempty"`
	// Deprecated: use spec.networking.service.external.externalTrafficPolicy.
	// +kubebuilder:validation:Enum="local";"cluster"
	PaiLBExternalTrafficPolicy string `json:"paiLBExternalTrafficPolicy,omitempty"`
	// Deprecated: use spec.networking.service.external.internal.
	// +kubebuilder:validation:Enum="true";"false"
	PaiInternalLB string `json:"paiInternalLB,omitempty"`
	// Deprecated: use spec.networking.service.external.annotations.
	PailbAnnotation map[string]string `json:"pailbAnnotation,omitempty"`
	// Deprecated: use spec.runtime.workerNodes.
	WorkerNodes []string `json:"workerNodes,omitempty"`
	// Deprecated: use spec.networking.trafficManager.
	TrafficManager *TrafficManagerRefSpec `json:"trafficManager,omitempty"`
}

// PaiSecretSpec stores secret reference and mount details for PrivateAI.
type PaiSecretSpec struct {
	Name          string            `json:"name,omitempty"`
	MountLocation string            `json:"mountLocation,omitempty"`
	Items         []SecretMountItem `json:"items,omitempty"`
}

// PrivateAiSecuritySpec groups auth secret and TLS settings for PrivateAI.
type PrivateAiSecuritySpec struct {
	AuthEnabled *bool          `json:"authEnabled,omitempty"`
	Secret      *PaiSecretSpec `json:"secret,omitempty"`
	TLS         *PaiTLSSpec    `json:"tls,omitempty"`
}

// PrivateAiRuntimeSpec groups runtime image, scaling, and process settings.
type PrivateAiRuntimeSpec struct {
	Image                *PrivateAiImageSpec          `json:"image,omitempty"`
	Replicas             *int32                       `json:"replicas,omitempty"`
	Env                  []EnvironmentVariable        `json:"env,omitempty"`
	Resources            *corev1.ResourceRequirements `json:"resources,omitempty"`
	Debug                *bool                        `json:"debug,omitempty"`
	ReadinessCheckPeriod *int                         `json:"readinessCheckPeriod,omitempty"`
	LivenessCheckPeriod  *int                         `json:"livenessCheckPeriod,omitempty"`
	DownloadScripts      *bool                        `json:"downloadScripts,omitempty"`
	WorkerNodes          []string                     `json:"workerNodes,omitempty"`
}

// PrivateAiImageSpec defines the container image settings for PrivateAI.
type PrivateAiImageSpec struct {
	Name       string            `json:"name,omitempty"`
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	PullSecret string            `json:"pullSecret,omitempty"`
}

// PrivateAiConfigurationSpec groups config map based runtime configuration.
type PrivateAiConfigurationSpec struct {
	ConfigFile *PaiConfigMap `json:"configFile,omitempty"`
}

// PrivateAiStorageSpec groups storage-related options for PrivateAI.
type PrivateAiStorageSpec struct {
	StorageClass      string            `json:"storageClass,omitempty"`
	PvcList           map[string]string `json:"pvcList,omitempty"`
	SizeInGb          *int32            `json:"sizeInGb,omitempty"`
	DeletePvcOnDelete *bool             `json:"deletePvcOnDelete,omitempty"`
	LogLocation       string            `json:"logLocation,omitempty"`
}

// PrivateAiNetworkingSpec groups listeners, services, and TrafficManager integration.
type PrivateAiNetworkingSpec struct {
	Listeners        *PrivateAiListenersSpec `json:"listeners,omitempty"`
	Service          *PaiServiceSpec         `json:"service,omitempty"`
	NodePortServices []PaiNodePortSvc        `json:"nodePortServices,omitempty"`
	TrafficManager   *TrafficManagerRefSpec  `json:"trafficManager,omitempty"`
}

// PrivateAiListenersSpec defines HTTP/HTTPS listener settings.
type PrivateAiListenersSpec struct {
	HTTP  *PrivateAiListenerSpec `json:"http,omitempty"`
	HTTPS *PrivateAiListenerSpec `json:"https,omitempty"`
}

// PrivateAiListenerSpec defines one listener's enabled flag and port.
type PrivateAiListenerSpec struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Port    *int32 `json:"port,omitempty"`
}

// PaiTLSSpec stores TLS secret reference and mount details for PrivateAI HTTPS.
type PaiTLSSpec struct {
	SecretName    string            `json:"secretName,omitempty"`
	MountLocation string            `json:"mountLocation,omitempty"`
	Items         []SecretMountItem `json:"items,omitempty"`
}

// SecretMountItem maps a secret key into a mounted file path.
// When path is omitted, the key name is used as the mounted filename.
type SecretMountItem struct {
	Key  string `json:"key,omitempty"`
	Path string `json:"path,omitempty"`
}

// EnvironmentVariable defines a name/value environment variable pair.
type EnvironmentVariable struct {
	Name  string `json:"name"`  // Name of the variable. Must be a C_IDENTIFIER.
	Value string `json:"value"` // Value of the variable, as defined in Kubernetes core API.
}

// PaiServiceSpec defines the service shape for the PrivateAI runtime.
type PaiServiceSpec struct {
	PortMappings []PaiPortMapping        `json:"portMappings,omitempty"` // Port mappings for the service that is created
	SvcName      string                  `json:"name,omitempty"`
	SvcType      string                  `json:"svcType,omitempty"`
	External     *PaiExternalServiceSpec `json:"external,omitempty"`
}

// PaiExternalServiceSpec configures optional external exposure for PrivateAI.
type PaiExternalServiceSpec struct {
	Enabled        *bool              `json:"enabled,omitempty"`
	ServiceType    corev1.ServiceType `json:"serviceType,omitempty"`
	Port           int32              `json:"port,omitempty"`
	TargetPort     int32              `json:"targetPort,omitempty"`
	Annotations    map[string]string  `json:"annotations,omitempty"`
	LoadBalancerIP string             `json:"loadBalancerIP,omitempty"`
	// +kubebuilder:validation:Enum="Local";"Cluster";"local";"cluster"
	ExternalTrafficPolicy string `json:"externalTrafficPolicy,omitempty"`
	Internal              *bool  `json:"internal,omitempty"`
}

// GatewayServiceSpec configures one gateway service endpoint.
type GatewayServiceSpec struct {
	Enabled     *bool              `json:"enabled,omitempty"`
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	Port        int32              `json:"port,omitempty"`
	TargetPort  int32              `json:"targetPort,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

// TrafficManagerRefSpec binds a PrivateAI backend to a shared TrafficManager.
type TrafficManagerRefSpec struct {
	Ref       string `json:"ref,omitempty"`
	RoutePath string `json:"routePath,omitempty"`
}

// LoggingSpec configures logging sidecar behavior.
type LoggingSpec struct {
	Enabled         bool   `json:"enabled,omitempty"`
	SidecarImage    string `json:"sidecarImage,omitempty"`
	VolumeName      string `json:"volumeName,omitempty"`
	VolumeMount     string `json:"volumeMount,omitempty"`
	VolumeSizeLimit string `json:"volumeSizeLimit,omitempty"`
}

// PaiConfigMap represents a configMap reference and mount location.
type PaiConfigMap struct {
	Name          string `json:"name,omitempty"`
	MountLocation string `json:"mountLocation,omitempty"`
}

// PaiNodePortSvc configures a nodeport service endpoint for PrivateAI.
type PaiNodePortSvc struct {
	PortMappings []PaiPortMapping `json:"portMappings,omitempty"` // Port mappings for the service
	SvcName      string           `json:"name,omitempty"`
	SvcType      string           `json:"svcType,omitempty"`
}

// PaiPortMapping defines one service port mapping.
type PaiPortMapping struct {
	Port       int32           `json:"port"`
	TargetPort int32           `json:"targetPort"` // Docker image port for the application
	Protocol   corev1.Protocol `json:"protocol"`   // IP protocol for the mapping, e.g., "TCP" or "UDP"
	NodePort   int32           `json:"nodePort,omitempty"`
}

// PrivateAiStatus defines the observed state of PrivateAi.
type PrivateAiStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Status          string                  `json:"status,omitempty"`
	Replicas        int                     `json:"replicas,omitempty"`
	ReleaseUpdate   string                  `json:"releaseUpdate,omitempty"`
	LoadBalancerIP  string                  `json:"loadBalancerIP,omitempty"`
	PodIP           string                  `json:"podIP,omitempty"`
	NodeIP          string                  `json:"NodeIP,omitempty"`
	ClusterIP       string                  `json:"clusterIP,omitempty"`
	LocalService    string                  `json:"localService,omitempty"`
	ExternalService string                  `json:"externalService,omitempty"`
	PaiSecret       SecretStatus            `json:"paiSecret,omitempty"`
	TLSSecret       TLSSecretStatus         `json:"tlsSecret,omitempty"`
	PaiConfigMap    ConfigMapStatus         `json:"paiConfigMap,omitempty"`
	Mode            string                  `json:"mode,omitempty"`
	TrafficManager  TrafficManagerRefStatus `json:"trafficManager,omitempty"`
	Logging         LoggingStatus           `json:"logging,omitempty"`
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// SecretStatus tracks observed status of the configured secret resource.
type SecretStatus struct {
	ResourceVersion string `json:"resourceVersion,omitempty" protobuf:"bytes,6,opt,name=resourceVersion"`
	Name            string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	HasAPIKey       bool   `json:"hasAPIKey,omitempty"`
	HasCertPem      bool   `json:"hasCertPem,omitempty"`
	// Deprecated: retained for compatibility with older status consumers.
	// The JSON key remains "apiKey" for backward compatibility.
	APIKey string `json:"apiKey,omitempty"`
	// Deprecated: retained for compatibility with older status consumers.
	Certpem string `json:"certpem,omitempty"`
}

// TLSSecretStatus tracks observed status of the configured TLS secret resource.
type TLSSecretStatus struct {
	ResourceVersion string `json:"resourceVersion,omitempty" protobuf:"bytes,6,opt,name=resourceVersion"`
	Name            string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}

// ConfigMapStatus tracks observed status of the configured ConfigMap.
type ConfigMapStatus struct {
	ResourceVersion string `json:"resourceVersion,omitempty" protobuf:"bytes,6,opt,name=resourceVersion"`
	Name            string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}

// TrafficManagerRefStatus tracks the resolved TrafficManager binding for PrivateAI.
type TrafficManagerRefStatus struct {
	Ref         string `json:"ref,omitempty"`
	RoutePath   string `json:"routePath,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	PublicURL   string `json:"publicURL,omitempty"`
}

// LoggingStatus tracks observed logging sidecar state.
type LoggingStatus struct {
	Enabled      bool   `json:"enabled,omitempty"`
	SidecarImage string `json:"sidecarImage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
//+kubebuilder:printcolumn:JSONPath=".status.replicas",name="Replicas",type=number
//+kubebuilder:printcolumn:JSONPath=".status.localService",name="LocalSvc",type=string
//+kubebuilder:printcolumn:JSONPath=".status.loadBalancerIP",name="LbIP",type=string
//+kubebuilder:printcolumn:JSONPath=".status.externalService",name="ExtSvc",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.paiSecret.name",name="Secret",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.trafficManager.ref",name="TMRef",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.trafficManager.routePath",name="TMRoute",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.trafficManager.serviceName",name="TMSvc",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.trafficManager.endpoint",name="TMEndpoint",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.trafficManager.publicURL",name="PublicURL",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.releaseUpdate",name="ReleaseUpdate",type=string,priority=1
//+kubebuilder:printcolumn:JSONPath=".status.mode",name="Mode",type=string,priority=1
// +kubebuilder:resource:shortName=pai

// PrivateAi is the Schema for the privateais API.
type PrivateAi struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateAiSpec   `json:"spec,omitempty"`
	Status PrivateAiStatus `json:"status,omitempty"`
}

// ReconcileError indicates reconcile cycle failed.
const ReconcileError string = "ReconcileError"

// ReconcileErrorReason is the reason used for reconcile cycle failure.
const ReconcileErrorReason string = "LastReconcileCycleFailed"

// ReconcileQueued indicates reconcile cycle is queued.
const ReconcileQueued string = "ReconcileQueued"

// ReconcileQueuedReason is the reason used for queued reconcile.
const ReconcileQueuedReason string = "LastReconcileCycleQueued"

// ReconcileCompelete indicates reconcile cycle completed.
const ReconcileCompelete string = "ReconcileComplete"

// ReconcileCompleteReason is the reason used for completed reconcile.
const ReconcileCompleteReason string = "LastReconcileCycleCompleted"

// ReconcileBlocked indicates reconcile cycle is blocked.
const ReconcileBlocked string = "ReconcileBlocked"

// ReconcileBlockedReason is the reason used for blocked reconcile.
const ReconcileBlockedReason string = "LastReconcileCycleBlocked"

// StatusPending indicates resource is pending creation.
const StatusPending string = "Pending"

// StatusCreating indicates resource creation is in progress.
const StatusCreating string = "Creating"

// StatusNotReady indicates resource health check is failing.
const StatusNotReady string = "Unhealthy"

// StatusPatching indicates patch operation is in progress.
const StatusPatching string = "Patching"

// StatusUpdating indicates update operation is in progress.
const StatusUpdating string = "Updating"

// StatusReady indicates resource is healthy and ready.
const StatusReady string = "Healthy"

// StatusError indicates a terminal or current error state.
const StatusError string = "Error"

// StatusUnknown indicates current state could not be determined.
const StatusUnknown string = "Unknown"

// ValueUnavailable indicates expected data is not currently available.
const ValueUnavailable string = "Unavailable"

// NoExternalIP indicates no external IP was available for selection.
const NoExternalIP string = "Node ExternalIP unavailable"

// +kubebuilder:object:root=true

// PrivateAiList contains a list of PrivateAi.
type PrivateAiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateAi `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrivateAi{}, &PrivateAiList{})
}
