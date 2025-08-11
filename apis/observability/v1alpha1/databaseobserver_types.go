/*
** Copyright (c) 2024 Oracle and/or its affiliates.
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

package v1alpha1

import (
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatusEnum string

// DatabaseObserverSpec defines the desired state of DatabaseObserver
type DatabaseObserverSpec struct {
	Database       DatabaseConfig                 `json:"database,omitempty"`
	Databases      map[string]MultiDatabaseConfig `json:"databases,omitempty"`
	Wallet         WalletSecret                   `json:"wallet,omitempty"`
	Deployment     ExporterDeployment             `json:"deployment,omitempty"`
	Service        ExporterService                `json:"service,omitempty"`
	ServiceMonitor ExporterServiceMonitor         `json:"serviceMonitor,omitempty"`
	ExporterConfig ExporterConfig                 `json:"exporterConfig,omitempty"`
	OCIConfig      OCIConfig                      `json:"ociConfig,omitempty"`
	AzureConfig    AzureConfig                    `json:"azureConfig,omitempty"`
	Metrics        MetricsConfig                  `json:"metrics,omitempty"`
	InheritLabels  []string                       `json:"inheritLabels,omitempty"`
	Log            LogConfig                      `json:"log,omitempty"`
	Sidecar        SidecarConfig                  `json:"sidecar,omitempty"`
	Replicas       int32                          `json:"replicas,omitempty"`
}

// SidecarConfig defines sidecar containers and volumes to add
type SidecarConfig struct {
	Containers []corev1.Container `json:"containers,omitempty"`
	Volumes    []corev1.Volume    `json:"volumes,omitempty"`
}

// ExporterConfig defines configMap used for exporter configuration
type ExporterConfig struct {
	ConfigMap ConfigMapDetails `json:"configMap,omitempty"`
	MountPath string           `json:"mountPath,omitempty"`
}

// LogConfig defines the configuration details relation to the logs of DatabaseObserver
type LogConfig struct {
	Disable     bool      `json:"disable,omitempty"`
	Destination string    `json:"destination,omitempty"`
	Filename    string    `json:"filename,omitempty"`
	Volume      LogVolume `json:"volume,omitempty"`
}

// LogVolume defines the shared volume between the exporter container and other containers
type LogVolume struct {
	Name                  string       `json:"name,omitempty"`
	PersistentVolumeClaim LogVolumePVC `json:"persistentVolumeClaim,omitempty"`
}

// LogVolumePVC defines the PVC in which to store the logs
type LogVolumePVC struct {
	ClaimName string `json:"claimName,omitempty"`
}

// DatabaseConfig defines the database details used for DatabaseObserver
type DatabaseConfig struct {
	DBUser             DBSecret     `json:"dbUser,omitempty"`
	DBPassword         DBSecret     `json:"dbPassword,omitempty"`
	DBConnectionString DBSecret     `json:"dbConnectionString,omitempty"`
	OCIVault           DBOCIVault   `json:"oci,omitempty"`
	AzureVault         DBAzureVault `json:"azure,omitempty"`
}

// MultiDatabaseConfig defines each database details used for DatabaseObserver
type MultiDatabaseConfig struct {
	DBUser             DBSecret `json:"dbUser,omitempty"`
	DBPassword         DBSecret `json:"dbPassword,omitempty"`
	DBConnectionString DBSecret `json:"dbConnectionString,omitempty"`
}

// DBAzureVault defines Azure Vault details
type DBAzureVault struct {
	VaultID             string `json:"vaultID,omitempty"`
	VaultUsernameSecret string `json:"vaultUsernameSecret,omitempty"`
	VaultPasswordSecret string `json:"vaultPasswordSecret,omitempty"`
}

// DBOCIVault defines OCI Vault details
type DBOCIVault struct {
	VaultID             string `json:"vaultID,omitempty"`
	VaultPasswordSecret string `json:"vaultPasswordSecret,omitempty"`
}

// ExporterDeployment defines the exporter deployment component of DatabaseObserver
type ExporterDeployment struct {
	ExporterImage         string                     `json:"image,omitempty"`
	SecurityContext       *corev1.SecurityContext    `json:"securityContext,omitempty"`
	PodSecurityContext    *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	ExporterArgs          []string                   `json:"args,omitempty"`
	ExporterCommands      []string                   `json:"commands,omitempty"`
	ExporterEnvs          map[string]string          `json:"env,omitempty"`
	Labels                map[string]string          `json:"labels,omitempty"`
	DeploymentPodTemplate DeploymentPodTemplate      `json:"podTemplate,omitempty"`
}

// DeploymentPodTemplate defines the labels for the DatabaseObserver pods component of a deployment
type DeploymentPodTemplate struct {
	Labels map[string]string `json:"labels,omitempty"`
}

// ExporterService defines the exporter service component of DatabaseObserver
type ExporterService struct {
	Ports  []corev1.ServicePort `json:"ports,omitempty"`
	Labels map[string]string    `json:"labels,omitempty"`
}

// ExporterServiceMonitor defines DatabaseObserver servicemonitor spec
type ExporterServiceMonitor struct {
	Labels            map[string]string            `json:"labels,omitempty"`
	NamespaceSelector *monitorv1.NamespaceSelector `json:"namespaceSelector,omitempty"`
	Endpoints         []monitorv1.Endpoint         `json:"endpoints,omitempty"`
}

// DBSecret  defines secrets used in reference
type DBSecret struct {
	Key        string `json:"key,omitempty"`
	SecretName string `json:"secret,omitempty"`
	EnvName    string `json:"envName,omitempty"`
}

// WalletSecret defines secret and where the wallet will be mounted if provided
type WalletSecret struct {
	SecretName        string                    `json:"secret,omitempty"`
	MountPath         string                    `json:"mountPath,omitempty"`
	AdditionalWallets []AdditionalWalletSecrets `json:"additional,omitempty"`
}

// AdditionalWalletSecrets defines multiple other secrets and where the wallet will be mounted if provided
type AdditionalWalletSecrets struct {
	Name       string `json:"name,omitempty"`
	SecretName string `json:"secret,omitempty"`
	MountPath  string `json:"mountPath,omitempty"`
}

// MetricsConfig defines configMap used for multiple metrics TOML configuration
type MetricsConfig struct {
	Configmap []ConfigMapDetails `json:"configMap,omitempty"`
}

// ConfigMapDetails defines the configmap name used by the exporterConfig and metricsConfig
type ConfigMapDetails struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

// OCIConfig defines the configmap name and secret name used for connecting to OCI
type OCIConfig struct {
	ConfigMap  ConfigMapDetails `json:"configMap,omitempty"`
	PrivateKey ConfigPrivateKey `json:"privateKey,omitempty"`
	MountPath  string           `json:"mountPath,omitempty"`
}

type ConfigPrivateKey struct {
	SecretName string `json:"secret,omitempty"`
}

// AzureConfig defines the configmap name and secret name used for connecting to Azure
type AzureConfig struct {
	ConfigMap ConfigMapDetails `json:"configMap,omitempty"`
}

// DatabaseObserverStatus defines the observed state of DatabaseObserver
type DatabaseObserverStatus struct {
	Conditions    []metav1.Condition `json:"conditions"`
	Status        string             `json:"status,omitempty"`
	MetricsConfig string             `json:"metricsConfig"`
	Version       string             `json:"version"`
	Replicas      int                `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="dbobserver";"dbobservers"

// DatabaseObserver is the Schema for the databaseobservers API
// +kubebuilder:printcolumn:JSONPath=".status.metricsConfig",name="MetricsConfig",type=string
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.version",name="Version",type=string
type DatabaseObserver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseObserverSpec   `json:"spec,omitempty"`
	Status DatabaseObserverStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseObserverList contains a list of DatabaseObserver
type DatabaseObserverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseObserver `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseObserver{}, &DatabaseObserverList{})
}
