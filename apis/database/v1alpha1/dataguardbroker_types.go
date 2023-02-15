/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DataguardBrokerSpec defines the desired state of DataguardBroker
type DataguardBrokerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	PrimaryDatabaseRef   string   `json:"primaryDatabaseRef"`
	StandbyDatabaseRefs  []string `json:"standbyDatabaseRefs"`
	SetAsPrimaryDatabase string   `json:"setAsPrimaryDatabase,omitempty"`
	LoadBalancer         bool     `json:"loadBalancer,omitempty"`

	// +kubebuilder:validation:Enum=MaxPerformance;MaxAvailability
	ProtectionMode    string                           `json:"protectionMode"`
	NodeSelector      map[string]string                `json:"nodeSelector,omitempty"`
	FastStartFailOver DataguardBrokerFastStartFailOver `json:"fastStartFailOver,omitempty"`
}

type DataguardBrokerFastStartFailOver struct {
	Enable   bool                      `json:"enable,omitempty"`
	Strategy []DataguardBrokerStrategy `json:"strategy,omitempty"`
}

// FSFO strategy
type DataguardBrokerStrategy struct {
	SourceDatabaseRef  string `json:"sourceDatabaseRef,omitempty"`
	TargetDatabaseRefs string `json:"targetDatabaseRefs,omitempty"`
}

// DataguardBrokerStatus defines the observed state of DataguardBroker
type DataguardBrokerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	PrimaryDatabaseRef    string `json:"primaryDatabaseRef,omitempty"`
	ProtectionMode        string `json:"protectionMode,omitempty"`
	PrimaryDatabase       string `json:"primaryDatabase,omitempty"`
	StandbyDatabases      string `json:"standbyDatabases,omitempty"`
	ExternalConnectString string `json:"externalConnectString,omitempty"`
	ClusterConnectString  string `json:"clusterConnectString,omitempty"`
	Status                string `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.primaryDatabase",name="Primary",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.standbyDatabases",name="Standbys",type="string"
// +kubebuilder:printcolumn:JSONPath=".spec.protectionMode",name="Protection Mode",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.clusterConnectString",name="Cluster Connect Str",type="string",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.externalConnectString",name="Connect Str",type="string"
// +kubebuilder:printcolumn:JSONPath=".spec.primaryDatabaseRef",name="Primary Database",type="string", priority=1
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type="string"

// DataguardBroker is the Schema for the dataguardbrokers API
type DataguardBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataguardBrokerSpec   `json:"spec,omitempty"`
	Status DataguardBrokerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DataguardBrokerList contains a list of DataguardBroker
type DataguardBrokerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataguardBroker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataguardBroker{}, &DataguardBrokerList{})
}
