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

// LRESTSpec defines the desired state of LREST
type LRESTSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the LREST
	LRESTName string `json:"cdbName,omitempty"`
	// Name of the LREST Service
	ServiceName string `json:"serviceName,omitempty"`
	// Password for the LREST System Administrator
	SysAdminPwd LRESTSysAdminPassword `json:"sysAdminPwd,omitempty"`
	// User in the root container with sysdba priviledges to manage PDB lifecycle
	LRESTAdminUser LRESTAdminUser `json:"cdbAdminUser,omitempty"`
	// Password for the LREST Administrator to manage PDB lifecycle
	LRESTAdminPwd LRESTAdminPassword `json:"cdbAdminPwd,omitempty"`
	// Secret: tls.key
	LRESTTlsKey LRESTTLSKEY `json:"cdbTlsKey,omitempty"`
	// Secret: tls.crt
	LRESTTlsCrt LRESTTLSCRT `json:"cdbTlsCrt,omitempty"`
	// Secret: Pub.key
	LRESTPubKey LRESTPUBKEY `json:"cdbPubKey,omitempty"`
	// Secret: Priv.key
	LRESTPriKey LRESTPRVKEY `json:"cdbPrvKey,omitempty"`
	// Secret: Tls.cat
	LRESTTlsCat LRPDBTLSCAT `json:"cdbTlsCat,omitempty"`
	// Password for user LREST_PUBLIC_USER
	LRESTPwd LRESTPassword `json:"lrestPwd,omitempty"`
	// LREST server port. For now, keep it as 8888. TO BE USED IN FUTURE RELEASE.
	// +kubebuilder:default=8888
	LRESTPort int `json:"lrestPort,omitempty"`
	// LREST Image Name
	LRESTImage string `json:"lrestImage,omitempty"`
	// The name of the image pull secret in case of a private docker repository.
	LRESTImagePullSecret string `json:"lrestImagePullSecret,omitempty"`
	// LREST Image Pull Policy
	// +kubebuilder:validation:Enum=Always;Never
	LRESTImagePullPolicy string `json:"lrestImagePullPolicy,omitempty"`
	// Number of LREST Containers to create
	Replicas int `json:"replicas,omitempty"`
	// Web Server User with SQL Administrator role to allow us to authenticate to the PDB Lifecycle Management REST endpoints
	WebLrestServerUser WebLrestServerUser `json:"webServerUser,omitempty"`
	// Password for the Web Server User
	WebLrestServerPwd WebLrestServerPassword `json:"webServerPwd,omitempty"`
	// Name of the DB server
	DBServer string `json:"dbServer,omitempty"`
	// DB server port
	DBPort int `json:"dbPort,omitempty"`
	// Node Selector for running the Pod
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Container database connect string
	DBTnsurl string `json:"dbTnsurl,omitempty"`
	// lrest server deletion automatically triggers associated pdb deletion
	DeletePDBCascade bool `json:"deletePdbCascade,omitempty"`
	// Name of the service account
	SrvAccountName string `json:"serviceAccountName,omitempty"`
	// Detection of pdb created manually via sqlplus or other cli db interface
	// It automatically associates a resource to the new pdb
	PdbAutoDiscover bool `json:"autodiscover,omitempty"`
	// The namespace assigned by default to the new resource when autodiscover is turned on
	NamesSpaceAutoDiscover string `json:"namespaceAutoDiscover,omitempty"`
	// Specify if cluster ip is required when  corev1.Service starts. Note the lrest server
	// it's an internal component that should never be visible from outside. Use this parameter
	// only if you need to run the operator local.
	// +kubebuilder:default=false
	ClusterIP bool `json:"clusterIp,omitempty"`
	// Create a load balancer: Use this parameter in conjunction with ClusterIP only if you need
	// to run the operator local
	// +kubebuilder:default=false
	LoadBalancer bool `json:"loadBalancer,omitempty"`
	// Turn on the  sqlnet.trace_level_client
	// +kubebuilder:default=0
	SqlNetTrace int `json:"trace_level_client,omitempty"`
}

// LRESTSecret defines the secretName
type LRESTSecret struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}

// LRESTSysAdminPassword defines the secret containing SysAdmin Password mapped to key 'sysAdminPwd' for LREST
type LRESTSysAdminPassword struct {
	Secret LRESTSecret `json:"secret"`
}

// LRESTAdminUser defines the secret containing LREST Administrator User mapped to key 'lrestAdminUser' to manage PDB lifecycle
type LRESTAdminUser struct {
	Secret LRESTSecret `json:"secret"`
}

// LRESTAdminPassword defines the secret containing LREST Administrator Password mapped to key 'lrestAdminPwd' to manage PDB lifecycle
type LRESTAdminPassword struct {
	Secret LRESTSecret `json:"secret"`
}

// LRESTPassword defines the secret containing LREST_PUBLIC_USER Password mapped to key 'ordsPwd'
type LRESTPassword struct {
	Secret LRESTSecret `json:"secret"`
}

// WebLrestServerUser defines the secret containing Web Server User mapped to key 'webServerUser' to manage PDB lifecycle
type WebLrestServerUser struct {
	Secret LRESTSecret `json:"secret"`
}

// WebLrestServerPassword defines the secret containing password for Web Server User mapped to key 'webServerPwd' to manage PDB lifecycle
type WebLrestServerPassword struct {
	Secret LRESTSecret `json:"secret"`
}

type LRESTTLSKEY struct {
	Secret LRESTSecret `json:"secret"`
}

type LRESTTLSCRT struct {
	Secret LRESTSecret `json:"secret"`
}

type LRESTPUBKEY struct {
	Secret LRESTSecret `json:"secret"`
}

type LRESTPRVKEY struct {
	Secret LRESTSecret `json:"secret"`
}

// LRESTStatus defines the observed state of LREST
type LRESTStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase of the LREST Resource
	Phase string `json:"phase"`
	// LREST Resource Status
	Status bool `json:"status"`
	// Message
	Msg string `json:"msg,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.cdbName",name="CDB NAME",type="string",description="Name of the LREST"
// +kubebuilder:printcolumn:JSONPath=".spec.dbServer",name="DB Server",type="string",description=" Name of the DB Server"
// +kubebuilder:printcolumn:JSONPath=".spec.dbPort",name="DB Port",type="integer",description="DB server port"
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",name="Replicas",type="integer",description="Replicas"
// +kubebuilder:printcolumn:JSONPath=".status.phase",name="Status",type="string",description="Status of the LREST Resource"
// +kubebuilder:printcolumn:JSONPath=".status.msg",name="Message",type="string",description="Error message if any"
// +kubebuilder:printcolumn:JSONPath=".spec.dbTnsurl",name="TNS STRING",type="string",description="string of the tnsalias"
// +kubebuilder:resource:path=lrests,scope=Namespaced
// +kubebuilder:storageversion

// LREST is the Schema for the lrests API
type LREST struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LRESTSpec   `json:"spec,omitempty"`
	Status LRESTStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LRESTList contains a list of LREST
type LRESTList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LREST `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LREST{}, &LRESTList{})
}
