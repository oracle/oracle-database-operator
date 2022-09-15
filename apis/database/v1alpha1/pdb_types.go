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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PDBSpec defines the desired state of PDB
type PDBSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	PDBTlsKey PDBTLSKEY `json:"pdbTlsKey,omitempty"`
	PDBTlsCrt PDBTLSCRT `json:"pdbTlsCrt,omitempty"`
	PDBTlsCat PDBTLSCAT `json:"pdbTlsCat,omitempty"`

	// Name of the CDB Custom Resource that runs the ORDS container
	CDBResName string `json:"cdbResName,omitempty"`
	// Name of the CDB
	CDBName string `json:"cdbName,omitempty"`
	// The name of the new PDB. Relevant for both Create and Plug Actions.
	PDBName string `json:"pdbName,omitempty"`
	// Name of the Source PDB from which to clone
	SrcPDBName string `json:"srcPdbName,omitempty"`
	// The administrator username for the new PDB. This property is required when the Action property is Create.
	AdminName PDBAdminName `json:"adminName,omitempty"`
	// The administrator password for the new PDB. This property is required when the Action property is Create.
	AdminPwd PDBAdminPassword `json:"adminPwd,omitempty"`
	// Relevant for Create and Plug operations. As defined in the  Oracle Multitenant Database documentation. Values can be a filename convert pattern or NONE.
	FileNameConversions string `json:"fileNameConversions,omitempty"`
	// This property is required when the Action property is Plug. As defined in the Oracle Multitenant Database documentation. Values can be a source filename convert pattern or NONE.
	SourceFileNameConversions string `json:"sourceFileNameConversions,omitempty"`
	// XML metadata filename to be used for Plug or Unplug operations
	XMLFileName string `json:"xmlFileName,omitempty"`
	// To copy files or not while cloning a PDB
	// +kubebuilder:validation:Enum=COPY;NOCOPY;MOVE
	CopyAction string `json:"copyAction,omitempty"`
	// Specify if datafiles should be removed or not. The value can be INCLUDING or KEEP (default).
	// +kubebuilder:validation:Enum=INCLUDING;KEEP
	DropAction string `json:"dropAction,omitempty"`
	// A Path specified for sparse clone snapshot copy. (Optional)
	SparseClonePath string `json:"sparseClonePath,omitempty"`
	// Whether to reuse temp file
	ReuseTempFile *bool `json:"reuseTempFile,omitempty"`
	// Relevant for Create and Plug operations. True for unlimited storage. Even when set to true, totalSize and tempSize MUST be specified in the request if Action is Create.
	UnlimitedStorage *bool `json:"unlimitedStorage,omitempty"`
	// Indicate if 'AS CLONE' option should be used in the command to plug in a PDB. This property is applicable when the Action property is PLUG but not required.
	AsClone *bool `json:"asClone,omitempty"`
	// Relevant for create and plug operations. Total size as defined in the Oracle Multitenant Database documentation. See size_clause description in Database SQL Language Reference documentation.
	TotalSize string `json:"totalSize,omitempty"`
	// Relevant for Create and Clone operations. Total size for temporary tablespace as defined in the Oracle Multitenant Database documentation. See size_clause description in Database SQL Language Reference documentation.
	TempSize string `json:"tempSize,omitempty"`
	// TDE import for plug operations
	TDEImport *bool `json:"tdeImport,omitempty"`
	// TDE export for unplug operations
	TDEExport *bool `json:"tdeExport,omitempty"`
	// TDE password if the tdeImport or tdeExport flag is set to true. Can be used in create, plug or unplug operations
	TDEPassword TDEPwd `json:"tdePassword,omitempty"`
	// TDE keystore path is required if the tdeImport or tdeExport flag is set to true. Can be used in plug or unplug operations.
	TDEKeystorePath string `json:"tdeKeystorePath,omitempty"`
	// TDE secret is required if the tdeImport or tdeExport flag is set to true. Can be used in plug or unplug operations.
	TDESecret TDESecret `json:"tdeSecret,omitempty"`
	// Whether you need the script only or execute the script
	GetScript *bool `json:"getScript,omitempty"`
	// Action to be taken: Create/Clone/Plug/Unplug/Delete/Modify/Status/Map. Map is used to map a Databse PDB to a Kubernetes PDB CR.
	// +kubebuilder:validation:Enum=Create;Clone;Plug;Unplug;Delete;Modify;Status;Map
	Action string `json:"action"`
	// Extra options for opening and closing a PDB
	// +kubebuilder:validation:Enum=IMMEDIATE;NORMAL;READ ONLY;READ WRITE;RESTRICTED
	ModifyOption string `json:"modifyOption,omitempty"`
	// The target state of the PDB
	// +kubebuilder:validation:Enum=OPEN;CLOSE
	PDBState string `json:"pdbState,omitempty"`
}

// PDBAdminName defines the secret containing Sys Admin User mapped to key 'adminName' for PDB
type PDBAdminName struct {
	Secret PDBSecret `json:"secret"`
}

// PDBAdminPassword defines the secret containing Sys Admin Password mapped to key 'adminPwd' for PDB
type PDBAdminPassword struct {
	Secret PDBSecret `json:"secret"`
}

// TDEPwd defines the secret containing TDE Wallet Password mapped to key 'tdePassword' for PDB
type TDEPwd struct {
	Secret PDBSecret `json:"secret"`
}

// TDESecret defines the secret containing TDE Secret to key 'tdeSecret' for PDB
type TDESecret struct {
	Secret PDBSecret `json:"secret"`
}

// PDBSecret defines the secretName
type PDBSecret struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}

type PDBTLSKEY struct {
	Secret PDBSecret `json:"secret"`
}

type PDBTLSCRT struct {
	Secret PDBSecret `json:"secret"`
}

type PDBTLSCAT struct {
	Secret PDBSecret `json:"secret"`
}

// PDBStatus defines the observed state of PDB
type PDBStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PDB Connect String
	ConnString string `json:"connString,omitempty"`
	// Phase of the PDB Resource
	Phase string `json:"phase"`
	// PDB Resource Status
	Status bool `json:"status"`
	// Total size of the PDB
	TotalSize string `json:"totalSize,omitempty"`
	// Open mode of the PDB
	OpenMode string `json:"openMode,omitempty"`
	// Modify Option of the PDB
	ModifyOption string `json:"modifyOption,omitempty"`
	// Message
	Msg string `json:"msg,omitempty"`
	// Last Completed Action
	Action string `json:"action,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.connString",name="Connect_String",type="string",description="The connect string to be used"
// +kubebuilder:printcolumn:JSONPath=".spec.cdbName",name="CDB Name",type="string",description="Name of the CDB"
// +kubebuilder:printcolumn:JSONPath=".spec.pdbName",name="PDB Name",type="string",description="Name of the PDB"
// +kubebuilder:printcolumn:JSONPath=".status.openMode",name="PDB State",type="string",description="PDB Open Mode"
// +kubebuilder:printcolumn:JSONPath=".status.totalSize",name="PDB Size",type="string",description="Total Size of the PDB"
// +kubebuilder:printcolumn:JSONPath=".status.phase",name="Status",type="string",description="Status of the PDB Resource"
// +kubebuilder:printcolumn:JSONPath=".status.msg",name="Message",type="string",description="Error message, if any"
// PDB is the Schema for the pdbs API
type PDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PDBSpec   `json:"spec,omitempty"`
	Status PDBStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PDBList contains a list of PDB
type PDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PDB{}, &PDBList{})
}
