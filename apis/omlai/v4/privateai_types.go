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
	PaiConfigFile           *PaiConfigMap                `json:"paiConfigFile,omitempty"`
	PaiEnableAuthentication bool                         `json:"paiEnableAuthentication,omitempty"`
	PaiSecret               *PaiSecretSpec               `json:"paiSecret,omitempty"`
	IsExternalSvc           bool                         `json:"isExternalSvc,omitempty"`
	StorageClass            string                       `json:"storageClass,omitempty"`
	PvcList                 map[string]string            `json:"pvcList,omitempty"`
	PaiImage                string                       `json:"paiImage,omitempty"`
	PaiImagePullSecret      string                       `json:"paiImagePullSecret,omitempty"`
	IsDebug                 bool                         `json:"isDebug,omitempty"`
	ReadinessCheckPeriod    int                          `json:"readinessCheckPeriod,omitempty"`
	LivenessCheckPeriod     int                          `json:"livenessCheckPeriod,omitempty"`
	IsDownloadScripts       bool                         `json:"isDownloadScripts,omitempty"`
	PaiService              PaiServiceSpec               `json:"paiService,omitempty"`
	StorageSizeInGb         int32                        `json:"storageSizeInGb,omitempty"`
	EnvVars                 []EnvironmentVariable        `json:"envVars,omitempty"`
	Replicas                int32                        `json:"replicas,omitempty"`
	Resources               *corev1.ResourceRequirements `json:"resources,omitempty"`
	NodePortSvc             []PaiNodePortSvc             `json:"nodePortSvc,omitempty"`
	PortMappings            []PaiPortMapping             `json:"portMappings,omitempty"`
	IsDeleteOraPvc          bool                         `json:"isDeleteOraPvc,omitempty"`
	PaiLogLocation          string                       `json:"paiLogLocation,omitempty"`
	PaiHTTPEnabled          bool                         `json:"paiHTTPEnabled,omitempty"`
	PaiHTTPSEnabled         bool                         `json:"paiHTTPSEnabled,omitempty"`
	PaiHTTPPort             int32                        `json:"paiHTTPPort,omitempty"`
	PaiHTTPSPort            int32                        `json:"paiHTTPSPort,omitempty"`
	PaiAuthentication       bool                         `json:"paiAuthentication,omitempty"`
	PaiLBPort               int32                        `json:"paiLBPort,omitempty"`
	PaiLBIP                 string                       `json:"paiLBIP,omitempty"`
}

// Secret Details
type PaiSecretSpec struct {
	Name          string `json:"name,omitempty"`
	MountLocation string `json:"mountLocation,omitempty"`
}

// Env Variable
type EnvironmentVariable struct {
	Name  string `json:"name"`  // Name of the variable. Must be a C_IDENTIFIER.
	Value string `json:"value"` // Value of the variable, as defined in Kubernetes core API.
}

// Service Spec
type PaiServiceSpec struct {
	PortMappings []PaiPortMapping `json:"portMappings,omitempty"` // Port mappings for the service that is created
	SvcName      string           `json:"name,omitempty"`
	SvcType      string           `json:"svcType,omitempty"`
}

// Config Map
type PaiConfigMap struct {
	Name          string `json:"name,omitempty"`
	MountLocation string `json:"mountLocation,omitempty"`
}

// Node Port Svc
type PaiNodePortSvc struct {
	PortMappings []PaiPortMapping `json:"portMappings,omitempty"` // Port mappings for the service
	SvcName      string           `json:"name,omitempty"`
	SvcType      string           `json:"svcType,omitempty"`
}

// Port Mapping
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
	Status         string `json:"status,omitempty"`
	Replicas       int    `json:"replicas,omitempty"`
	ReleaseUpdate  string `json:"releaseUpdate,omitempty"`
	ApiKey         string `json:"apiKey,omitempty"`
	Certpem        string `json:"certpem,omitempty"`
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
	PodIP          string `json:"podIP,omitempty"`
	NodeIP         string `json:"NodeIP,omitempty"`
	ClusterIP      string `json:"clusterIP,omitempty"`
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
//+kubebuilder:printcolumn:JSONPath=".status.replicas",name="Replicas",type=number
//+kubebuilder:printcolumn:JSONPath=".status.apikey",name="ApiKey",type=string
//+kubebuilder:printcolumn:JSONPath=".status.podip",name="PodIP",type=string
//+kubebuilder:printcolumn:JSONPath=".status.loadbalancerip",name="LbIP",type=string
//+kubebuilder:printcolumn:JSONPath=".status.ReleaseUpdate",name="ReleaseUpdate",type=string,priority=1

// PrivateAi is the Schema for the privateais API.
type PrivateAi struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateAiSpec   `json:"spec,omitempty"`
	Status PrivateAiStatus `json:"status,omitempty"`
}

const ReconcileError string = "ReconcileError"

const ReconcileErrorReason string = "LastReconcileCycleFailed"

const ReconcileQueued string = "ReconcileQueued"

const ReconcileQueuedReason string = "LastReconcileCycleQueued"

const ReconcileCompelete string = "ReconcileComplete"

const ReconcileCompleteReason string = "LastReconcileCycleCompleted"

const ReconcileBlocked string = "ReconcileBlocked"

const ReconcileBlockedReason string = "LastReconcileCycleBlocked"

const StatusPending string = "Pending"

const StatusCreating string = "Creating"

const StatusNotReady string = "Unhealthy"

const StatusPatching string = "Patching"

const StatusUpdating string = "Updating"

const StatusReady string = "Healthy"

const StatusError string = "Error"

const StatusUnknown string = "Unknown"

const ValueUnavailable string = "Unavailable"

const NoExternalIp string = "Node ExternalIP unavailable"

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
