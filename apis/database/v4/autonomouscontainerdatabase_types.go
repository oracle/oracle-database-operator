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
	"encoding/json"
	"reflect"

	"github.com/oracle/oci-go-sdk/v65/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// name of our custom finalizer
const ACDFinalizer = "database.oracle.com/acd-finalizer"

type AcdActionEnum string

const (
	AcdActionBlank     AcdActionEnum = ""
	AcdActionRestart   AcdActionEnum = "RESTART"
	AcdActionTerminate AcdActionEnum = "TERMINATE"
)

func GetAcdActionEnumFromString(val string) (AcdActionEnum, bool) {
	var mappingAcdActionEnum = map[string]AcdActionEnum{
		"RESTART":   AcdActionRestart,
		"TERMINATE": AcdActionTerminate,
		"":          AcdActionBlank,
	}

	enum, ok := mappingAcdActionEnum[val]
	return enum, ok
}

// AutonomousContainerDatabaseSpec defines the desired state of AutonomousContainerDatabase
type AutonomousContainerDatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	AutonomousContainerDatabaseOCID *string `json:"autonomousContainerDatabaseOCID,omitempty"`
	CompartmentOCID                 *string `json:"compartmentOCID,omitempty"`
	DisplayName                     *string `json:"displayName,omitempty"`
	AutonomousExadataVMClusterOCID  *string `json:"autonomousExadataVMClusterOCID,omitempty"`
	// +kubebuilder:validation:Enum:="RELEASE_UPDATES";"RELEASE_UPDATE_REVISIONS"
	PatchModel database.AutonomousContainerDatabasePatchModelEnum `json:"patchModel,omitempty"`
	// +kubebuilder:validation:Enum:="SYNC";"RESTART";"TERMINATE"
	Action       AcdActionEnum     `json:"action,omitempty"`
	FreeformTags map[string]string `json:"freeformTags,omitempty"`

	OCIConfig OciConfigSpec `json:"ociConfig,omitempty"`
	// +kubebuilder:default:=false
	HardLink *bool `json:"hardLink,omitempty"`
}

// AutonomousContainerDatabaseStatus defines the observed state of AutonomousContainerDatabase
type AutonomousContainerDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LifecycleState database.AutonomousContainerDatabaseLifecycleStateEnum `json:"lifecycleState"`
	TimeCreated    string                                                 `json:"timeCreated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="acd";"acds"
// +kubebuilder:printcolumn:JSONPath=".spec.displayName",name="DisplayName",type=string
// +kubebuilder:printcolumn:JSONPath=".status.lifecycleState",name="State",type=string
// +kubebuilder:printcolumn:JSONPath=".status.timeCreated",name="Created",type=string
// +kubebuilder:storageversion

// AutonomousContainerDatabase is the Schema for the autonomouscontainerdatabases API
type AutonomousContainerDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutonomousContainerDatabaseSpec   `json:"spec,omitempty"`
	Status AutonomousContainerDatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutonomousContainerDatabaseList contains a list of AutonomousContainerDatabase
type AutonomousContainerDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutonomousContainerDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutonomousContainerDatabase{}, &AutonomousContainerDatabaseList{})
}

// Implement conversion.Hub interface, which means any resource version can convert into v4
func (*AutonomousContainerDatabase) Hub() {}

// GetLastSuccessfulSpec returns spec from the lass successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulSpec.
func (acd *AutonomousContainerDatabase) GetLastSuccessfulSpec() (*AutonomousContainerDatabaseSpec, error) {
	val, ok := acd.GetAnnotations()[LastSuccessfulSpec]
	if !ok {
		return nil, nil
	}

	specBytes := []byte(val)
	sucSpec := AutonomousContainerDatabaseSpec{}

	err := json.Unmarshal(specBytes, &sucSpec)
	if err != nil {
		return nil, err
	}

	return &sucSpec, nil
}

func (acd *AutonomousContainerDatabase) UpdateLastSuccessfulSpec() error {
	specBytes, err := json.Marshal(acd.Spec)
	if err != nil {
		return err
	}

	anns := acd.GetAnnotations()

	if anns == nil {
		anns = map[string]string{
			LastSuccessfulSpec: string(specBytes),
		}
	} else {
		anns[LastSuccessfulSpec] = string(specBytes)
	}

	acd.SetAnnotations(anns)

	return nil
}

// UpdateStatusFromOCIACD updates the status subresource
func (acd *AutonomousContainerDatabase) UpdateStatusFromOCIACD(ociObj database.AutonomousContainerDatabase) {
	acd.Status.LifecycleState = ociObj.LifecycleState
	acd.Status.TimeCreated = FormatSDKTime(ociObj.TimeCreated)
}

// UpdateFromOCIADB updates the attributes using database.AutonomousContainerDatabase object
func (acd *AutonomousContainerDatabase) UpdateFromOCIACD(ociObj database.AutonomousContainerDatabase) (specChanged bool) {
	oldACD := acd.DeepCopy()

	/***********************************
	* update the spec
	***********************************/
	acd.Spec.Action = AcdActionBlank
	acd.Spec.AutonomousContainerDatabaseOCID = ociObj.Id
	acd.Spec.CompartmentOCID = ociObj.CompartmentId
	acd.Spec.DisplayName = ociObj.DisplayName
	acd.Spec.AutonomousExadataVMClusterOCID = ociObj.CloudAutonomousVmClusterId
	acd.Spec.PatchModel = ociObj.PatchModel

	// special case: an emtpy map will be nil after unmarshalling while the OCI always returns an emty map.
	if len(ociObj.FreeformTags) != 0 {
		acd.Spec.FreeformTags = ociObj.FreeformTags
	} else {
		acd.Spec.FreeformTags = nil
	}

	/***********************************
	* update the status subresource
	***********************************/
	acd.UpdateStatusFromOCIACD(ociObj)

	return !reflect.DeepEqual(oldACD.Spec, acd.Spec)
}

// RemoveUnchangedSpec removes the unchanged fields in spec, and returns if the spec has been changed.
func (acd *AutonomousContainerDatabase) RemoveUnchangedSpec(prevSpec AutonomousContainerDatabaseSpec) (bool, error) {
	changed, err := RemoveUnchangedFields(prevSpec, &acd.Spec)
	if err != nil {
		return changed, err
	}

	return changed, nil
}

// A helper function which is useful for debugging. The function prints out a structural JSON format.
func (acd *AutonomousContainerDatabase) String() (string, error) {
	out, err := json.MarshalIndent(acd, "", "    ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
