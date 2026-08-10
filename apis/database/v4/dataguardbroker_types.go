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

package v4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DataguardBrokerSpec defines the desired state of DataguardBroker
type DataguardBrokerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	PrimaryDatabaseRef   string            `json:"primaryDatabaseRef,omitempty"`
	StandbyDatabaseRefs  []string          `json:"standbyDatabaseRefs,omitempty"`
	SetAsPrimaryDatabase string            `json:"setAsPrimaryDatabase,omitempty"`
	LoadBalancer         bool              `json:"loadBalancer,omitempty"`
	ServiceAnnotations   map[string]string `json:"serviceAnnotations,omitempty"`
	// +kubebuilder:validation:Enum=MaxPerformance;MaxAvailability
	ProtectionMode string            `json:"protectionMode,omitempty"`
	NodeSelector   map[string]string `json:"nodeSelector,omitempty"`

	FastStartFailover bool                       `json:"fastStartFailover,omitempty"`
	Execution         *DataguardExecutionSpec    `json:"execution,omitempty"`
	Topology          *DataguardTopologySpec     `json:"topology,omitempty"`
	Operations        *DataguardBrokerOperations `json:"operations,omitempty"`
}

// DataguardBrokerOperations contains tokenized one-time broker operations.
type DataguardBrokerOperations struct {
	Switchover     *DataguardBrokerSwitchoverOperation     `json:"switchover,omitempty"`
	Failover       *DataguardBrokerFailoverOperation       `json:"failover,omitempty"`
	ProtectionMode *DataguardBrokerProtectionModeOperation `json:"protectionMode,omitempty"`
	RoleConversion *DataguardBrokerRoleConversionOperation `json:"roleConversion,omitempty"`
}

// DataguardBrokerRoleConversionOperation requests a one-shot standby role conversion.
type DataguardBrokerRoleConversionOperation struct {
	Target    string `json:"target,omitempty"`
	Role      string `json:"role,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// DataguardBrokerSwitchoverOperation requests a one-time switchover.
type DataguardBrokerSwitchoverOperation struct {
	Target    string `json:"target,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// DataguardBrokerFailoverOperation requests a one-time failover.
type DataguardBrokerFailoverOperation struct {
	Target    string `json:"target,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

// DataguardBrokerProtectionModeOperation requests a one-time protection mode change.
type DataguardBrokerProtectionModeOperation struct {
	// +kubebuilder:validation:Enum=MaxPerformance;MaxAvailability
	Mode      string `json:"mode,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// DataguardExecutionSpec defines broker-side runtime settings used when the
// controller needs a dedicated execution pod for topology-native DG actions.
type DataguardExecutionSpec struct {
	// Image is the Oracle client/database image used for DG broker execution.
	Image string `json:"image,omitempty"`
	// ImagePullSecrets lists image pull secret names for the execution runtime.
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`
	// WalletMountPath is where TCPS client wallet secrets are mounted in the runner pod.
	WalletMountPath string `json:"walletMountPath,omitempty"`
	// TNSAdminPath is the generated Oracle Net admin path used by the runner pod.
	TNSAdminPath string `json:"tnsAdminPath,omitempty"`
	// AuthWallet optionally enables explicit broker auth wallet bootstrap/rebuild workflow.
	AuthWallet *DataguardAuthWalletSpec `json:"authWallet,omitempty"`
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

	FastStartFailover          string                           `json:"fastStartFailover,omitempty"`
	DatabasesInDataguardConfig map[string]string                `json:"databasesInDataguardConfig,omitempty"`
	ObservedTopologyHash       string                           `json:"observedTopologyHash,omitempty"`
	ResolvedMembers            []DataguardResolvedMemberStatus  `json:"resolvedMembers,omitempty"`
	AuthWallet                 *DataguardAuthWalletStatus       `json:"authWallet,omitempty"`
	Operations                 *DataguardBrokerOperationsStatus `json:"operations,omitempty"`
	Conditions                 []metav1.Condition               `json:"conditions,omitempty"`
}

// DataguardBrokerOperationsStatus records tokenized operation progress.
type DataguardBrokerOperationsStatus struct {
	Switchover     *DataguardBrokerOperationStatus `json:"switchover,omitempty"`
	Failover       *DataguardBrokerOperationStatus `json:"failover,omitempty"`
	ProtectionMode *DataguardBrokerOperationStatus `json:"protectionMode,omitempty"`
	RoleConversion *DataguardBrokerOperationStatus `json:"roleConversion,omitempty"`
}

// DataguardBrokerOperationStatus records the last observed operation request.
type DataguardBrokerOperationStatus struct {
	ObservedRequestID string       `json:"observedRequestId,omitempty"`
	Target            string       `json:"target,omitempty"`
	Phase             string       `json:"phase,omitempty"`
	Message           string       `json:"message,omitempty"`
	StartedAt         *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt       *metav1.Time `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=dgbroker;dgbrokers
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.primaryDatabase",name="Primary",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.standbyDatabases",name="Standbys",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.protectionMode",name="Protection Mode",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.primaryDatabaseRef",name="Primary Database",type="string", priority=1
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.fastStartFailover",name="FSFO", type="string"

// DataguardBroker is the Schema for the dataguardbrokers API
// +kubebuilder:storageversion
type DataguardBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataguardBrokerSpec   `json:"spec,omitempty"`
	Status DataguardBrokerStatus `json:"status,omitempty"`
}

// //////////////////////////////////////////////////////////////////////////////////////////////////
// Returns the current primary database in the dataguard configuration from the resource status/spec
// //////////////////////////////////////////////////////////////////////////////////////////////////
func (broker *DataguardBroker) GetCurrentPrimaryDatabase() string {
	if broker.Status.PrimaryDatabase != "" && len(broker.Status.DatabasesInDataguardConfig) > 0 {
		if dbRef := broker.Status.DatabasesInDataguardConfig[broker.Status.PrimaryDatabase]; dbRef != "" {
			return dbRef
		}
	}
	return broker.Spec.PrimaryDatabaseRef
}

// //////////////////////////////////////////////////////////////////////////////////////////////////
// Returns databases in Dataguard configuration from the resource status/spec
// //////////////////////////////////////////////////////////////////////////////////////////////////
func (broker *DataguardBroker) GetDatabasesInDataGuardConfiguration() []string {
	var databases []string
	if len(broker.Status.DatabasesInDataguardConfig) > 0 {
		for _, value := range broker.Status.DatabasesInDataguardConfig {
			if value != "" {
				databases = append(databases, value)
			}
		}

		return databases
	}

	databases = append(databases, broker.Spec.PrimaryDatabaseRef)
	databases = append(databases, broker.Spec.StandbyDatabaseRefs...)
	return databases
}

// //////////////////////////////////////////////////////////////////////////////////////////////////
// Returns standby databases in the dataguard configuration from the resource status/spec
// //////////////////////////////////////////////////////////////////////////////////////////////////
func (broker *DataguardBroker) GetStandbyDatabasesInDgConfig() []string {
	var databases []string
	if len(broker.Status.DatabasesInDataguardConfig) > 0 {
		for dbUniqueName, resourceName := range broker.Status.DatabasesInDataguardConfig {
			if resourceName != "" && dbUniqueName != broker.Status.PrimaryDatabase {
				databases = append(databases, resourceName)
			}
		}
		return databases
	}

	databases = append(databases, broker.Spec.StandbyDatabaseRefs...)
	return databases
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
