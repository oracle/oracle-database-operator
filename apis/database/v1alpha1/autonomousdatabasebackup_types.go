/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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

	"github.com/oracle/oci-go-sdk/v51/database"
	"github.com/oracle/oracle-database-operator/commons/oci/ociutil"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AutonomousDatabaseBackupSpec defines the desired state of AutonomousDatabaseBackup
type AutonomousDatabaseBackupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	AutonomousDatabaseOCID       string `json:"autonomousDatabaseOCID"`
	AutonomousDatabaseBackupOCID string `json:"autonomousDatabaseBackupOCID,omitempty"`

	OCIConfig OCIConfigSpec `json:"ociConfig,omitempty"`
}

// AutonomousDatabaseBackupStatus defines the observed state of AutonomousDatabaseBackup
type AutonomousDatabaseBackupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	AutonomousDatabaseBackupOCID string                                              `json:"autonomousDatabaseBackupOCID"`
	CompartmentOCID              string                                              `json:"compartmentOCID"`
	AutonomousDatabaseOCID       string                                              `json:"autonomousDatabaseOCID"`
	Type                         database.AutonomousDatabaseBackupTypeEnum           `json:"type"`
	IsAutomatic                  bool                                                `json:"isAutomatic"`
	LifecycleState               database.AutonomousDatabaseBackupLifecycleStateEnum `json:"lifecycleState"`
	TimeStarted                  string                                              `json:"timeStarted,omitempty"`
	TimeEnded                    string                                              `json:"timeEnded,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="adbbu";"adbbus"
// +kubebuilder:printcolumn:JSONPath=".status.lifecycleState",name="State",type=string
// +kubebuilder:printcolumn:JSONPath=".status.type",name="Type",type=string
// +kubebuilder:printcolumn:JSONPath=".status.timeStarted",name="Started",type=string
// +kubebuilder:printcolumn:JSONPath=".status.timeEnded",name="Ended",type=string

// AutonomousDatabaseBackup is the Schema for the autonomousdatabasebackups API
type AutonomousDatabaseBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutonomousDatabaseBackupSpec   `json:"spec,omitempty"`
	Status AutonomousDatabaseBackupStatus `json:"status,omitempty"`
}

func (backup *AutonomousDatabaseBackup) UpdateStatusFromAutonomousDatabaseBackupResponse(resp database.GetAutonomousDatabaseBackupResponse) {
	backup.Status.AutonomousDatabaseBackupOCID = *resp.Id
	backup.Status.CompartmentOCID = *resp.CompartmentId
	backup.Status.AutonomousDatabaseOCID = *resp.AutonomousDatabaseId
	backup.Status.Type = resp.Type
	backup.Status.IsAutomatic = *resp.IsAutomatic
	backup.Status.LifecycleState = resp.LifecycleState

	if resp.TimeStarted != nil {
		backup.Status.TimeStarted = ociutil.FormatSDKTime(resp.TimeStarted.Time)
	}
	if resp.TimeEnded != nil {
		backup.Status.TimeEnded = ociutil.FormatSDKTime(resp.TimeEnded.Time)
	}
}

//+kubebuilder:object:root=true

// AutonomousDatabaseBackupList contains a list of AutonomousDatabaseBackup
type AutonomousDatabaseBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutonomousDatabaseBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutonomousDatabaseBackup{}, &AutonomousDatabaseBackupList{})
}
