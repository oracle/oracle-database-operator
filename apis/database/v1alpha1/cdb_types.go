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

// CDBSpec defines the desired state of CDB
type CDBSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the CDB
	CDBName string `json:"cdbName,omitempty"`
	// Name of the CDB Service
	ServiceName string `json:"serviceName,omitempty"`


	// Password for the CDB System Administrator
	SysAdminPwd CDBSysAdminPassword `json:"sysAdminPwd,omitempty"`
	// User in the root container with sysdba priviledges to manage PDB lifecycle
	CDBAdminUser CDBAdminUser `json:"cdbAdminUser,omitempty"`
	// Password for the CDB Administrator to manage PDB lifecycle
	CDBAdminPwd CDBAdminPassword `json:"cdbAdminPwd,omitempty"`

	CDBTlsKey CDBTLSKEY `json:"cdbTlsKey,omitempty"`
	CDBTlsCrt CDBTLSCRT `json:"cdbTlsCrt,omitempty"`

	// Password for user ORDS_PUBLIC_USER
	ORDSPwd ORDSPassword `json:"ordsPwd,omitempty"`
	// ORDS server port. For now, keep it as 8888. TO BE USED IN FUTURE RELEASE.
	ORDSPort int `json:"ordsPort,omitempty"`
	// ORDS Image Name
	ORDSImage string `json:"ordsImage,omitempty"`
	// The name of the image pull secret in case of a private docker repository.
	ORDSImagePullSecret string `json:"ordsImagePullSecret,omitempty"`
	// ORDS Image Pull Policy
	// +kubebuilder:validation:Enum=Always;Never
	ORDSImagePullPolicy string `json:"ordsImagePullPolicy,omitempty"`
	// Number of ORDS Containers to create
	Replicas int `json:"replicas,omitempty"`
	// Web Server User with SQL Administrator role to allow us to authenticate to the PDB Lifecycle Management REST endpoints
	WebServerUser WebServerUser `json:"webServerUser,omitempty"`
	// Password for the Web Server User
	WebServerPwd WebServerPassword `json:"webServerPwd,omitempty"`
	// SCAN Name
	SCANName string `json:"scanName,omitempty"`
	// Name of the DB server
	DBServer string `json:"dbServer,omitempty"`
	// DB server port
	DBPort int `json:"dbPort,omitempty"`
	// Node Selector for running the Pod
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// CDBSecret defines the secretName
type CDBSecret struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}

// CDBSysAdminPassword defines the secret containing SysAdmin Password mapped to key 'sysAdminPwd' for CDB
type CDBSysAdminPassword struct {
	Secret CDBSecret `json:"secret"`
}

// CDBAdminUser defines the secret containing CDB Administrator User mapped to key 'cdbAdminUser' to manage PDB lifecycle
type CDBAdminUser struct {
	Secret CDBSecret `json:"secret"`
}

// CDBAdminPassword defines the secret containing CDB Administrator Password mapped to key 'cdbAdminPwd' to manage PDB lifecycle
type CDBAdminPassword struct {
	Secret CDBSecret `json:"secret"`
}

// ORDSPassword defines the secret containing ORDS_PUBLIC_USER Password mapped to key 'ordsPwd'
type ORDSPassword struct {
	Secret CDBSecret `json:"secret"`
}

// WebServerUser defines the secret containing Web Server User mapped to key 'webServerUser' to manage PDB lifecycle
type WebServerUser struct {
	Secret CDBSecret `json:"secret"`
}

// WebServerPassword defines the secret containing password for Web Server User mapped to key 'webServerPwd' to manage PDB lifecycle
type WebServerPassword struct {
	Secret CDBSecret `json:"secret"`
}

type CDBTLSKEY struct {
	Secret CDBSecret `json:"secret"`
}

type CDBTLSCRT struct {
	Secret CDBSecret `json:"secret"`
}

// CDBStatus defines the observed state of CDB
type CDBStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase of the CDB Resource
	Phase string `json:"phase"`
	// CDB Resource Status
	Status bool `json:"status"`
	// Message
	Msg string `json:"msg,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.cdbName",name="CDB Name",type="string",description="Name of the CDB"
// +kubebuilder:printcolumn:JSONPath=".spec.dbServer",name="DB Server",type="string",description=" Name of the DB Server"
// +kubebuilder:printcolumn:JSONPath=".spec.dbPort",name="DB Port",type="integer",description="DB server port"
// +kubebuilder:printcolumn:JSONPath=".spec.scanName",name="SCAN Name",type="string",description="SCAN Name"
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",name="Replicas",type="integer",description="Replicas"
// +kubebuilder:printcolumn:JSONPath=".status.phase",name="Status",type="string",description="Status of the CDB Resource"
// +kubebuilder:printcolumn:JSONPath=".status.msg",name="Message",type="string",description="Error message, if any"

// CDB is the Schema for the cdbs API
type CDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CDBSpec   `json:"spec,omitempty"`
	Status CDBStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CDBList contains a list of CDB
type CDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CDB{}, &CDBList{})
}
