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

	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/workrequests"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AutonomousDatabaseRestoreSpec defines the desired state of AutonomousDatabaseRestore
type AutonomousDatabaseRestoreSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	TargetADB    TargetSpec    `json:"targetADB"`
	Source    SourceSpec    `json:"sourceSpec"`
	OCIConfig OCIConfigSpec `json:"ociConfig,omitempty"`
}

type SourceSpec struct {
	AutonomousDatabaseBackup BackupSourceSpec `json:"autonomousDatabaseBackup,omitempty"`
	PointInTime              string                       `json:"pointInTime,omitempty"`
}

type BackupSourceSpec struct {
	Name string `json:"name,omitempty"`
}

type restoreStatusEnum string

const (
	RestoreStatusInProgress restoreStatusEnum = "IN_PROGRESS"
	RestoreStatusFailed     restoreStatusEnum = "FAILED"
	RestoreStatusSucceeded  restoreStatusEnum = "SUCCEEDED"
)

// AutonomousDatabaseRestoreStatus defines the observed state of AutonomousDatabaseRestore
type AutonomousDatabaseRestoreStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DisplayName            string            `json:"displayName"`
	DbName                 string            `json:"dbName"`
	AutonomousDatabaseOCID string            `json:"autonomousDatabaseOCID"`
	Status                 restoreStatusEnum `json:"status"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName="adbr";"adbrs"
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.displayName",name="DisplayName",type=string
// +kubebuilder:printcolumn:JSONPath=".status.dbName",name="DbName",type=string

// AutonomousDatabaseRestore is the Schema for the autonomousdatabaserestores API
type AutonomousDatabaseRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutonomousDatabaseRestoreSpec   `json:"spec,omitempty"`
	Status AutonomousDatabaseRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutonomousDatabaseRestoreList contains a list of AutonomousDatabaseRestore
type AutonomousDatabaseRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutonomousDatabaseRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutonomousDatabaseRestore{}, &AutonomousDatabaseRestoreList{})
}

// GetPIT returns the spec.pointInTime.timeStamp in SDKTime format
func (r *AutonomousDatabaseRestore) GetPIT() (*common.SDKTime, error) {
	return parseDisplayTime(r.Spec.Source.PointInTime)
}

func (r *AutonomousDatabaseRestore) ConvertWorkRequestStatus(s workrequests.WorkRequestStatusEnum) restoreStatusEnum {
	switch s {
	case workrequests.WorkRequestStatusAccepted:
		fallthrough
	case workrequests.WorkRequestStatusInProgress:
		return RestoreStatusInProgress

	case workrequests.WorkRequestStatusSucceeded:
		return RestoreStatusSucceeded

	case workrequests.WorkRequestStatusFailed:
		return RestoreStatusFailed
	}

	return "UNKNOWN"
}
