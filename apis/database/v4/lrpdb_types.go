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
)

// LRPDBSpec defines the desired state of LRPDB
type LRPDBSpec struct {
	// Secret: tls.key
	LRPDBTlsKey LRPDBTLSKEY `json:"lrpdbTlsKey,omitempty"`
	// Secret: tls.crt
	LRPDBTlsCrt LRPDBTLSCRT `json:"lrpdbTlsCrt,omitempty"`
	//	Secret: ca.crt
	LRPDBTlsCat LRPDBTLSCAT `json:"lrpdbTlsCat,omitempty"`
	// Secret for private key
	LRPDBPriKey LRPDBPRVKEY `json:"cdbPrvKey,omitempty"`
	// Namespace of the rest server
	CDBNamespace string `json:"cdbNamespace,omitempty"`
	// Name of the CDB Custom Resource that runs the LREST container
	CDBResName string `json:"cdbResName,omitempty"`
	// Name of the CDB
	CDBName string `json:"cdbName,omitempty"`
	// The name of the new LRPDB. Relevant for both Create and Plug Actions.
	LRPDBName string `json:"pdbName,omitempty"`
	// Name of the Source LRPDB from which to clone
	SrcLRPDBName string `json:"srcPdbName,omitempty"`
	// The administrator username for the new LRPDB. This property is required when the Action property is Create.
	AdminName LRPDBAdminName `json:"adminName,omitempty"`
	// The administrator password for the new LRPDB. This property is required when the Action property is Create.
	AdminPwd LRPDBAdminPassword `json:"adminPwd,omitempty"`
	// PDB Admin user
	AdminpdbUser AdminpdbUser `json:"adminpdbUser,omitempty"`
	// PDB Admin user password
	AdminpdbPass AdminpdbPass `json:"adminpdbPass,omitempty"`
	// Use this parameter on non ASM storage '....path....','pdbname' e.g. '/u01/oradata','dborcl'
	FileNameConversions string `json:"fileNameConversions,omitempty"`
	// This property is required when the Action property is Plug. As defined in the Oracle Multitenant Database documentation. Values can be a source filename convert pattern or NONE.
	SourceFileNameConversions string `json:"sourceFileNameConversions,omitempty"`
	// XML metadata filename to be used for Plug or Unplug operations
	XMLFileName string `json:"xmlFileName,omitempty"`
	// To copy files or not while cloning a LRPDB
	// +kubebuilder:validation:Enum=COPY;NOCOPY;MOVE
	CopyAction string `json:"copyAction,omitempty"`
	// Specify if datafiles should be removed or not. The value can be INCLUDING or KEEP (default).
	// +kubebuilder:validation:Enum=INCLUDING;KEEP
	DropAction string `json:"dropAction,omitempty"`
	// A Path specified for sparse clone snapshot copy. (Optional)
	SparseClonePath string `json:"sparseClonePath,omitempty"`
	// Whether to reuse temp file
	// +kubebuilder:default=true
	ReuseTempFile *bool `json:"reuseTempFile,omitempty"`
	// Relevant for Create and Plug operations. True for unlimited storage. Even when set to true, totalSize and tempSize MUST be specified in the request if Action is Create.
	// +kubebuilder:default=true
	UnlimitedStorage *bool `json:"unlimitedStorage,omitempty"`
	// Indicate if 'AS CLONE' option should be used in the command to plug in a LRPDB. This property is applicable when the Action property is PLUG but not required.
	AsClone *bool `json:"asClone,omitempty"`
	// Relevant for create and plug operations. Total size as defined in the Oracle Multitenant Database documentation. See size_clause description in Database SQL Language Reference documentation.
	TotalSize string `json:"totalSize,omitempty"`
	// Relevant for Create and Clone operations. Total size for temporary tablespace as defined in the Oracle Multitenant Database documentation. See size_clause description in Database SQL Language Reference documentation.
	TempSize string `json:"tempSize,omitempty"`
	// Web Server User with SQL Administrator role to allow us to authenticate to the PDB Lifecycle Management REST endpoints
	WebLrpdbServerUser WebLrpdbServerUser `json:"webServerUser,omitempty"`
	// Password for the Web Server User
	WebLrpdbServerPwd WebLrpdbServerPassword `json:"webServerPwd,omitempty"`
	// TDE import for plug operations
	// +hidefromdoc
	LTDEImport *bool `json:"tdeImport,omitempty"`
	// LTDE export for unplug operations
	// +hidefromdoc
	LTDEExport *bool `json:"tdeExport,omitempty"`
	// TDE password if the tdeImport or tdeExport flag is set to true. Can be used in create, plug or unplug operations
	// +hidefromdoc
	LTDEPassword LTDEPwd `json:"tdePassword,omitempty"`
	// LTDE keystore path is required if the tdeImport or tdeExport flag is set to true. Can be used in plug or unplug operations.
	// +hidefromdoc
	LTDEKeystorePath string `json:"tdeKeystorePath,omitempty"`
	// LTDE secret is required if the tdeImport or tdeExport flag is set to true. Can be used in plug or unplug operations.
	// +hidefromdoc
	LTDESecret LTDESecret `json:"tdeSecret,omitempty"`
	//  Whether you need the script only or execute the script - legacy parameter
	//  +kubebuilder:default=false
	GetScript *bool `json:"getScript,omitempty"`
	// Action to be taken: Create/Clone/Plug/Unplug/Delete/Modify/Status/Map/Alter. Map is used to map a Databse LRPDB to a Kubernetes LRPDB CR.
	// Mainted for backward compatibility. No longer need
	Action string `json:"action,omitempty"`
	// Extra options for opening and closing a LRPDB
	// +kubebuilder:validation:Enum=IMMEDIATE;NORMAL;READ ONLY;READ WRITE;RESTRICTED
	ModifyOption string `json:"modifyOption,omitempty"`
	// Modify Option2 of the LRPDB
	// +kubebuilder:default=NONE
	ModifyOption2 string `json:"modifyOption2,omitempty"`
	// to be used with ALTER option - obsolete do not use
	AlterSystem string `json:"alterSystem,omitempty"`
	// To be used with ALTER option - the name of the parameter
	AlterSystemParameter string `json:"alterSystemParameter,omitempty"`
	// To be used with ALTER option - the  value of the parameter
	AlterSystemValue string `json:"alterSystemValue,omitempty"`
	// Init parameter scope
	ParameterScope string `json:"parameterScope,omitempty"`
	// The target state of the LRPDB
	// +kubebuilder:validation:Enum=OPEN;CLOSE;ALTER;DELETE;UNPLUG;PLUG;CLONE;RESET;NONE
	LRPDBState string `json:"pdbState,omitempty"`
	// Turn on the imperative approach to delete pdb resource
	// kubectl delete pdb command automatically triggers the pluggable database
	// deletion
	ImperativeLrpdbDeletion bool `json:"imperativeLrpdbDeletion,omitempty"`
	// Config map containing the pdb parameters
	PDBConfigMap string `json:"pdbconfigmap,omitempty"`
	// Config map containing sql(ddl)/plsql code
	PLSQLBlock string `json:"codeconfigmap,omitempty"`
	// Spare filed not used
	PLSQLExecMode int `json:"plsqlexemode,omitempty"`
	// For future use - rest bitmask status
	// ++kubebuilder:default=0
	PDBBitMask int `json:"reststate,omitempty"`
	// Debug option , not yet implemented
	Debug int `json:"debug,omitempty"`
}

// LRPDBAdminName defines the secret containing Sys Admin User mapped to key 'adminName' for LRPDB
type LRPDBAdminName struct {
	Secret LRPDBSecret `json:"secret"`
}

// LRPDBAdminPassword defines the secret containing Sys Admin Password mapped to key 'adminPwd' for LRPDB
type LRPDBAdminPassword struct {
	Secret LRPDBSecret `json:"secret"`
}

// TDEPwd defines the secret containing TDE Wallet Password mapped to key 'tdePassword' for LRPDB
type LTDEPwd struct {
	Secret LRPDBSecret `json:"secret"`
}

// TDESecret defines the secret containing TDE Secret to key 'tdeSecret' for LRPDB
type LTDESecret struct {
	Secret LRPDBSecret `json:"secret"`
}

type WebLrpdbServerUser struct {
	Secret LRPDBSecret `json:"secret"`
}

type WebLrpdbServerPassword struct {
	Secret LRPDBSecret `json:"secret"`
}

type AdminpdbUser struct {
	Secret LRPDBSecret `json:"secret"`
}

type AdminpdbPass struct {
	Secret LRPDBSecret `json:"secret"`
}

// LRPDBSecret defines the secretName
type LRPDBSecret struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}

type LRPDBTLSKEY struct {
	Secret LRPDBSecret `json:"secret"`
}

type LRPDBTLSCRT struct {
	Secret LRPDBSecret `json:"secret"`
}

type LRPDBTLSCAT struct {
	Secret LRPDBSecret `json:"secret"`
}

type LRPDBPRVKEY struct {
	Secret LRPDBSecret `json:"secret"`
}

// LRPDBStatus defines the observed state of LRPDB
type LRPDBStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// LRPDB Connect String
	ConnString string `json:"connString,omitempty"`
	// Phase of the LRPDB Resource
	Phase string `json:"phase"`
	// LRPDB Resource Status
	Status bool `json:"status"`
	// Total size of the LRPDB
	TotalSize string `json:"totalSize,omitempty"`
	// Open mode of the LRPDB
	OpenMode string `json:"openMode,omitempty"`
	// Modify Option of the LRPDB
	ModifyOption string `json:"modifyOption,omitempty"`
	// Restricted
	Restricted string `json:"restricted,omitempty"`
	// Message
	Msg string `json:"msg,omitempty"`
	// Last Completed Action
	Action string `json:"action,omitempty"`
	// Last Completed alter system
	PDBBitMask    int    `json:"pdbBitMask,omitempty"`
	PDBBitMaskStr string `json:"pdbBitMaskStr,omitempty"`
	AlterSystem   string `json:"alterSystem,omitempty"`
	// Last ORA-
	SqlCode      int    `json:"sqlCode"`
	LastPLSQL    string `json:"lastplsql,omitempty"`
	CmBitstat    int    `json:"bitstat,omitempty"`    /* Bitmask */
	CmBitStatStr string `json:"bitstatstr,omitempty"` /* Decoded bitmask */
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.cdbName",name="CDB Name",type="string",description="Name of the CDB"
// +kubebuilder:printcolumn:JSONPath=".spec.pdbName",name="PDB Name",type="string",description="Name of the PDB"
// +kubebuilder:printcolumn:JSONPath=".status.openMode",name="PDB State",type="string",description="PDB Open Mode"
// +kubebuilder:printcolumn:JSONPath=".status.totalSize",name="PDB Size",type="string",description="Total Size of the PDB"
// +kubebuilder:printcolumn:JSONPath=".status.msg",name="Message",type="string",description="Error message, if any"
// +kubebuilder:printcolumn:JSONPath=".status.restricted",name="Restricted",type="string",description="open restricted"
// +kubebuilder:printcolumn:JSONPath=".status.sqlCode",name="last sqlcode",type="integer",description="last sqlcode"
// +kubebuilder:printcolumn:JSONPath=".status.lastplsql",name="last PLSQL",type="string",description="last plsql applied"
// +kubebuilder:printcolumn:JSONPath=".status.pdbBitMaskStr",name="BITMASK STATUS",type="string",description="Bitmask status"
// +kubebuilder:printcolumn:JSONPath=".status.connString",name="Connect_String",type="string",description="The connect string to be used"
// +kubebuilder:resource:path=lrpdbs,scope=Namespaced
// +kubebuilder:storageversion

// LRPDB is the Schema for the pdbs API
type LRPDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LRPDBSpec   `json:"spec,omitempty"`
	Status LRPDBStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LRPDBList contains a list of LRPDB
type LRPDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LRPDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LRPDB{}, &LRPDBList{})
}
