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
	Database         DatabaseObserverDatabase       `json:"database,omitempty"`
	Exporter         DatabaseObserverExporterConfig `json:"exporter,omitempty"`
	ExporterConfig   DatabaseObserverConfigMap      `json:"configuration,omitempty"`
	Prometheus       PrometheusConfig               `json:"prometheus,omitempty"`
	OCIConfig        OCIConfigSpec                  `json:"ociConfig,omitempty"`
	Replicas         int32                          `json:"replicas,omitempty"`
	Log              LogConfig                      `json:"log,omitempty"`
	InheritLabels    []string                       `json:"inheritLabels,omitempty"`
	ExporterSidecars []corev1.Container             `json:"sidecars,omitempty"`
	SideCarVolumes   []corev1.Volume                `json:"sidecarVolumes,omitempty"`
}

// LogConfig defines the configuration details relation to the logs of DatabaseObserver
type LogConfig struct {
	Disable 		bool 	  `json:"disable,omitempty"`
	Destination     string    `json:"destination,omitempty"`
	Filename 		string 	  `json:"filename,omitempty"`
	Volume   		LogVolume `json:"volume,omitempty"`
}


type LogVolume struct {
	PersistentVolumeClaim LogVolumePVClaim `json:"persistentVolumeClaim,omitempty"`
}

type LogVolumePVClaim struct {
	ClaimName string `json:"claimName,omitempty"`
}

// DatabaseObserverDatabase defines the database details used for DatabaseObserver
type DatabaseObserverDatabase struct {
	DBUser             DBSecret          `json:"dbUser,omitempty"`
	DBPassword         DBSecretWithVault `json:"dbPassword,omitempty"`
	DBWallet           DBSecret          `json:"dbWallet,omitempty"`
	DBConnectionString DBSecret          `json:"dbConnectionString,omitempty"`
}

// DatabaseObserverExporterConfig defines the configuration details related to the exporters of DatabaseObserver
type DatabaseObserverExporterConfig struct {
	Deployment DatabaseObserverDeployment `json:"deployment,omitempty"`
	Service    DatabaseObserverService    `json:"service,omitempty"`
}

// DatabaseObserverDeployment defines the exporter deployment component of DatabaseObserver
type DatabaseObserverDeployment struct {
	ExporterImage         string                  `json:"image,omitempty"`
	SecurityContext       *corev1.SecurityContext `json:"securityContext,omitempty"`
	ExporterArgs          []string                `json:"args,omitempty"`
	ExporterCommands      []string                `json:"commands,omitempty"`
	ExporterEnvs          map[string]string       `json:"env,omitempty"`
	Labels                map[string]string       `json:"labels,omitempty"`
	DeploymentPodTemplate DeploymentPodTemplate   `json:"podTemplate,omitempty"`
}

// DeploymentPodTemplate defines the labels for the DatabaseObserver pods component of a deployment
type DeploymentPodTemplate struct {
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Labels          map[string]string          `json:"labels,omitempty"`
}

// DatabaseObserverService defines the exporter service component of DatabaseObserver
type DatabaseObserverService struct {
	Ports  []corev1.ServicePort `json:"ports,omitempty"`
	Labels map[string]string    `json:"labels,omitempty"`
}

// PrometheusConfig defines the generated resources for Prometheus
type PrometheusConfig struct {
	ServiceMonitor PrometheusServiceMonitor `json:"serviceMonitor,omitempty"`
}

// PrometheusServiceMonitor defines DatabaseObserver servicemonitor spec
type PrometheusServiceMonitor struct {
	Labels            map[string]string            `json:"labels,omitempty"`
	NamespaceSelector *monitorv1.NamespaceSelector `json:"namespaceSelector,omitempty"`
	Endpoints         []monitorv1.Endpoint         `json:"endpoints,omitempty"`
}

// DBSecret  defines secrets used in reference
type DBSecret struct {
	Key        string `json:"key,omitempty"`
	SecretName string `json:"secret,omitempty"`
}

// DBSecretWithVault  defines secrets used in reference with vault fields
type DBSecretWithVault struct {
	Key             string `json:"key,omitempty"`
	SecretName      string `json:"secret,omitempty"`
	VaultOCID       string `json:"vaultOCID,omitempty"`
	VaultSecretName string `json:"vaultSecretName,omitempty"`
}

// DatabaseObserverConfigMap defines configMap used for metrics configuration
type DatabaseObserverConfigMap struct {
	Configmap ConfigMapDetails `json:"configMap,omitempty"`
}

// ConfigMapDetails defines the configmap name
type ConfigMapDetails struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

// OCIConfigSpec defines the configmap name and secret name used for connecting to OCI
type OCIConfigSpec struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	SecretName    string `json:"secretName,omitempty"`
}

// DatabaseObserverStatus defines the observed state of DatabaseObserver
type DatabaseObserverStatus struct {
	Conditions     []metav1.Condition `json:"conditions"`
	Status         string             `json:"status,omitempty"`
	ExporterConfig string             `json:"exporterConfig"`
	Version        string             `json:"version"`
	Replicas       int                `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="dbobserver";"dbobservers"

// DatabaseObserver is the Schema for the databaseobservers API
// +kubebuilder:printcolumn:JSONPath=".status.exporterConfig",name="ExporterConfig",type=string
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
