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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OracleRestDataServiceSpec defines the desired state of OracleRestDataService
type OracleRestDataServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	DatabaseRef        string                                   `json:"databaseRef"`
	LoadBalancer       bool                                     `json:"loadBalancer,omitempty"`
	NodeSelector       map[string]string                        `json:"nodeSelector,omitempty"`
	Image              OracleRestDataServiceImage               `json:"image,omitempty"`
	OrdsPassword       OracleRestDataServicePassword            `json:"ordsPassword"`
	ApexPassword       OracleRestDataServicePassword            `json:"apexPassword,omitempty"`
	AdminPassword      OracleRestDataServicePassword            `json:"adminPassword"`
	OrdsUser           string                                   `json:"ordsUser,omitempty"`
	RestEnableSchemas  []OracleRestDataServiceRestEnableSchemas `json:"restEnableSchemas,omitempty"`
	OracleService      string                                   `json:"oracleService,omitempty"`
	ServiceAccountName string                                   `json:"serviceAccountName,omitempty"`
	Persistence        OracleRestDataServicePersistence         `json:"persistence,omitempty"`

	// +k8s:openapi-gen=true
	// +kubebuilder:validation:Minimum=1
	Replicas int `json:"replicas,omitempty"`
}

// OracleRestDataServicePersistence defines the storage releated params
type OracleRestDataServicePersistence struct {
	Size         string `json:"size,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`

	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	AccessMode string `json:"accessMode,omitempty"`
}

// OracleRestDataServiceImage defines the Image source and pullSecrets for POD
type OracleRestDataServiceImage struct {
	Version     string `json:"version,omitempty"`
	PullFrom    string `json:"pullFrom"`
	PullSecrets string `json:"pullSecrets,omitempty"`
}

// OracleRestDataServicePassword defines the secret containing Password mapped to secretKey
type OracleRestDataServicePassword struct {
	SecretName string `json:"secretName"`
	// +kubebuilder:default:="oracle_pwd"
	SecretKey  string `json:"secretKey,omitempty"`
	KeepSecret *bool   `json:"keepSecret,omitempty"`
}

// OracleRestDataServicePDBSchemas defines the PDB Schemas to be ORDS Enabled
type OracleRestDataServiceRestEnableSchemas struct {
	PdbName        string `json:"pdbName,omitempty"`
	SchemaName     string `json:"schemaName"`
	UrlMapping     string `json:"urlMapping,omitempty"`
	Enable         bool   `json:"enable"`
}

// OracleRestDataServiceStatus defines the observed state of OracleRestDataService
type OracleRestDataServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Status             string `json:"status,omitempty"`
	DatabaseApiUrl     string `json:"databaseApiUrl,omitempty"`
	LoadBalancer       string `json:"loadBalancer,omitempty"`
	DatabaseRef        string `json:"databaseRef,omitempty"`
	ServiceIP          string `json:"serviceIP,omitempty"`
	DatabaseActionsUrl string `json:"databaseActionsUrl,omitempty"`
	OrdsInstalled      bool   `json:"ordsInstalled,omitempty"`
	ApexConfigured     bool   `json:"apexConfigured,omitempty"`
	ApxeUrl            string `json:"apexUrl,omitempty"`
	CommonUsersCreated bool   `json:"commonUsersCreated,omitempty"`
	Replicas           int    `json:"replicas,omitempty"`

	Image OracleRestDataServiceImage `json:"image,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type="string"
// +kubebuilder:printcolumn:JSONPath=".spec.databaseRef",name="Database",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.databaseApiUrl",name="Database API URL",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.databaseActionsUrl",name="Database Actions URL",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.apexUrl",name="Apex URL",type="string"

// OracleRestDataService is the Schema for the oraclerestdataservices API
type OracleRestDataService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OracleRestDataServiceSpec   `json:"spec,omitempty"`
	Status OracleRestDataServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OracleRestDataServiceList contains a list of OracleRestDataService
type OracleRestDataServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OracleRestDataService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OracleRestDataService{}, &OracleRestDataServiceList{})
}
