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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbcsv1 "github.com/oracle/oracle-database-operator/commons/annotations"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DbcsSystemSpec defines the desired state of DbcsSystem
type DbcsSystemSpec struct {
	DbSystem     DbSystemDetails `json:"dbSystem,omitempty"`
	Id           *string         `json:"id,omitempty"`
	OCIConfigMap string          `json:"ociConfigMap"`
	OCISecret    string          `json:"ociSecret,omitempty"`
	HardLink     bool            `json:"hardLink,omitempty"`
}

// DbSystemDetails Spec

type DbSystemDetails struct {
	CompartmentId              string            `json:"compartmentId"`
	AvailabilityDomain         string            `json:"availabilityDomain"`
	SubnetId                   string            `json:"subnetId"`
	Shape                      string            `json:"shape"`
	SshPublicKeys              []string          `json:"sshPublicKeys"`
	HostName                   string            `json:"hostName"`
	CpuCoreCount               int               `json:"cpuCoreCount,omitempty"`
	FaultDomains               []string          `json:"faultDomains,omitempty"`
	DisplayName                string            `json:"displayName,omitempty"`
	BackupSubnetId             string            `json:"backupSubnetId,omitempty"`
	TimeZone                   string            `json:"timeZone,omitempty"`
	NodeCount                  *int              `json:"nodeCount,omitempty"`
	PrivateIp                  string            `json:"privateIp,omitempty"`
	Domain                     string            `json:"domain,omitempty"`
	InitialDataStorageSizeInGB int               `json:"initialDataStorageSizeInGB,omitempty"`
	ClusterName                string            `json:"clusterName,omitempty"`
	KmsKeyId                   string            `json:"kmsKeyId,omitempty"`
	KmsKeyVersionId            string            `json:"kmsKeyVersionId,omitempty"`
	DbAdminPaswordSecret       string            `json:"dbAdminPaswordSecret"`
	DbName                     string            `json:"dbName,omitempty"`
	PdbName                    string            `json:"pdbName,omitempty"`
	DbDomain                   string            `json:"dbDomain,omitempty"`
	DbUniqueName               string            `json:"dbUniqueName,omitempty"`
	StorageManagement          string            `json:"storageManagement,omitempty"`
	DbVersion                  string            `json:"dbVersion,omitempty"`
	DbEdition                  string            `json:"dbEdition,omitempty"`
	DiskRedundancy             string            `json:"diskRedundancy,omitempty"`
	DbWorkload                 string            `json:"dbWorkload,omitempty"`
	LicenseModel               string            `json:"licenseModel,omitempty"`
	TdeWalletPasswordSecret    string            `json:"tdeWalletPasswordSecret,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	DbBackupConfig             Backupconfig      `json:"dbBackupConfig,omitempty"`
}

// DB Backup COnfig Network Struct
type Backupconfig struct {
	AutoBackupEnabled        *bool   `json:"autoBackupEnabled,omitempty"`
	RecoveryWindowsInDays    *int    `json:"recoveryWindowsInDays,omitempty"`
	AutoBackupWindow         *string `json:"autoBackupWindow,omitempty"`
	BackupDestinationDetails *string `json:"backupDestinationDetails,omitempty"`
}

// DbcsSystemStatus defines the observed state of DbcsSystem
type DbcsSystemStatus struct {
	Id                 *string `json:"id,omitempty"`
	DisplayName        string  `json:"displayName,omitempty"`
	AvailabilityDomain string  `json:"availabilityDomain,omitempty"`
	SubnetId           string  `json:"subnetId,omitempty"`
	StorageManagement  string  `json:"storageManagement,omitempty"`
	NodeCount          int     `json:"nodeCount,omitempty"`
	CpuCoreCount       int     `json:"cpuCoreCount,omitempty"`

	DbEdition             string `json:"dbEdition,omitempty"`
	TimeZone              string `json:"timeZone,omitempty"`
	DataStoragePercentage *int   `json:"dataStoragePercentage,omitempty"`
	LicenseModel          string `json:"licenseModel,omitempty"`
	DataStorageSizeInGBs  *int   `json:"dataStorageSizeInGBs,omitempty"`
	RecoStorageSizeInGB   *int   `json:"recoStorageSizeInGB,omitempty"`

	Shape        *string          `json:"shape,omitempty"`
	State        LifecycleState   `json:"state"`
	DbInfo       []DbStatus       `json:"dbInfo,omitempty"`
	Network      VmNetworkDetails `json:"network,omitempty"`
	WorkRequests []DbWorkrequests `json:"workRequests,omitempty"`
}

// DbcsSystemStatus defines the observed state of DbcsSystem
type DbStatus struct {
	Id           *string `json:"id,omitempty"`
	DbName       string  `json:"dbName,omitempty"`
	DbUniqueName string  `json:"dbUniqueName,omitempty"`
	DbWorkload   string  `json:"dbWorkload,omitempty"`
	DbHomeId     string  `json:"dbHomeId,omitempty"`
}

type DbWorkrequests struct {
	OperationType   *string `json:"operationType,omitmpty"`
	OperationId     *string `json:"operationId,omitemty"`
	PercentComplete string  `json:"percentComplete,omitempty"`
	TimeAccepted    string  `json:"timeAccepted,omitempty"`
	TimeStarted     string  `json:"timeStarted,omitempty"`
	TimeFinished    string  `json:"timeFinished,omitempty"`
}

type VmNetworkDetails struct {
	VcnName      *string `json:"vcnName,omitempty"`
	SubnetName   *string `json:"clientSubnet,omitempty"`
	ScanDnsName  *string `json:"scanDnsName,omitempty"`
	HostName     string  `json:"hostName,omitempty"`
	DomainName   string  `json:"domainName,omitempty"`
	ListenerPort *int    `json:"listenerPort,omitempty"`
	NetworkSG    string  `json:"networkSG,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DbcsSystem is the Schema for the dbcssystems API
type DbcsSystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DbcsSystemSpec   `json:"spec,omitempty"`
	Status DbcsSystemStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DbcsSystemList contains a list of DbcsSystem
type DbcsSystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DbcsSystem `json:"items"`
}

type LifecycleState string

const (
	Available LifecycleState = "AVAILABLE"
	Failed    LifecycleState = "FAILED"
	Update    LifecycleState = "UPDATING"
	Provision LifecycleState = "PROVISIONING"
	Terminate LifecycleState = "TERMINATED"
)

const lastSuccessfulSpec = "lastSuccessfulSpec"

// GetLastSuccessfulSpec returns spec from the lass successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulSpec.
func (dbcs *DbcsSystem) GetLastSuccessfulSpec() (*DbcsSystemSpec, error) {
	val, ok := dbcs.GetAnnotations()[lastSuccessfulSpec]
	if !ok {
		return nil, nil
	}

	specBytes := []byte(val)
	sucSpec := DbcsSystemSpec{}

	err := json.Unmarshal(specBytes, &sucSpec)
	if err != nil {
		return nil, err
	}

	return &sucSpec, nil
}

// UpdateLastSuccessfulSpec updates lastSuccessfulSpec with the current spec.
func (dbcs *DbcsSystem) UpdateLastSuccessfulSpec(kubeClient client.Client) error {
	specBytes, err := json.Marshal(dbcs.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		lastSuccessfulSpec: string(specBytes),
	}

	//	return dbcsv1.SetAnnotations(kubeClient, dbcs, anns)
	return dbcsv1.PatchAnnotations(kubeClient, dbcs, anns)

}

func init() {
	SchemeBuilder.Register(&DbcsSystem{}, &DbcsSystemList{})
}
