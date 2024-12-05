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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AutonomousDatabaseBackupSpec defines the desired state of AutonomousDatabaseBackup
type AutonomousDatabaseBackupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Target                       TargetSpec    `json:"target,omitempty"`
	DisplayName                  *string       `json:"displayName,omitempty"`
	AutonomousDatabaseBackupOCID *string       `json:"autonomousDatabaseBackupOCID,omitempty"`
	IsLongTermBackup             *bool         `json:"isLongTermBackup,omitempty"`
	RetentionPeriodInDays        *int          `json:"retentionPeriodInDays,omitempty"`
	OCIConfig                    OCIConfigSpec `json:"ociConfig,omitempty"`
}

// AutonomousDatabaseBackupStatus defines the observed state of AutonomousDatabaseBackup
type AutonomousDatabaseBackupStatus struct {
	LifecycleState         database.AutonomousDatabaseBackupLifecycleStateEnum `json:"lifecycleState"`
	Type                   database.AutonomousDatabaseBackupTypeEnum           `json:"type"`
	IsAutomatic            bool                                                `json:"isAutomatic"`
	TimeStarted            string                                              `json:"timeStarted,omitempty"`
	TimeEnded              string                                              `json:"timeEnded,omitempty"`
	AutonomousDatabaseOCID string                                              `json:"autonomousDatabaseOCID"`
	CompartmentOCID        string                                              `json:"compartmentOCID"`
	DBName                 string                                              `json:"dbName"`
	DBDisplayName          string                                              `json:"dbDisplayName"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName="adbbu";"adbbus"
//+kubebuilder:printcolumn:JSONPath=".status.lifecycleState",name="State",type=string
//+kubebuilder:printcolumn:JSONPath=".status.dbDisplayName",name="DB DisplayName",type=string
//+kubebuilder:printcolumn:JSONPath=".status.type",name="Type",type=string
//+kubebuilder:printcolumn:JSONPath=".status.timeStarted",name="Started",type=string
//+kubebuilder:printcolumn:JSONPath=".status.timeEnded",name="Ended",type=string
// +kubebuilder:storageversion

// AutonomousDatabaseBackup is the Schema for the autonomousdatabasebackups API
type AutonomousDatabaseBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutonomousDatabaseBackupSpec   `json:"spec,omitempty"`
	Status AutonomousDatabaseBackupStatus `json:"status,omitempty"`
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

// Implement conversion.Hub interface, which means any resource version can convert into v4
func (*AutonomousDatabaseBackup) Hub() {}

func (b *AutonomousDatabaseBackup) UpdateStatusFromOCIBackup(ociBackup database.AutonomousDatabaseBackup, ociADB database.AutonomousDatabase) {
	b.Status.AutonomousDatabaseOCID = *ociBackup.AutonomousDatabaseId
	b.Status.CompartmentOCID = *ociBackup.CompartmentId
	b.Status.Type = ociBackup.Type
	b.Status.IsAutomatic = *ociBackup.IsAutomatic

	b.Status.LifecycleState = ociBackup.LifecycleState

	b.Status.TimeStarted = FormatSDKTime(ociBackup.TimeStarted)
	b.Status.TimeEnded = FormatSDKTime(ociBackup.TimeEnded)

	b.Status.DBDisplayName = *ociADB.DisplayName
	b.Status.DBName = *ociADB.DbName
}

// GetTimeEnded returns the status.timeEnded in SDKTime format
func (b *AutonomousDatabaseBackup) GetTimeEnded() (*common.SDKTime, error) {
	return parseDisplayTime(b.Status.TimeEnded)
}
