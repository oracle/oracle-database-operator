/*
** Copyright (c) 2022-2024 Oracle and/or its affiliates.
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	dbcsv1 "github.com/oracle/oracle-database-operator/commons/annotations"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DbcsSystemSpec defines the desired state of DbcsSystem
type DbcsSystemSpec struct {
	DbSystem       *DbSystemDetails `json:"dbSystem,omitempty"`
	Id             *string          `json:"id,omitempty"`
	OCIConfigMap   *string          `json:"ociConfigMap"`
	OCISecret      *string          `json:"ociSecret,omitempty"`
	DbClone        *DbCloneConfig   `json:"dbClone,omitempty"`
	HardLink       bool             `json:"hardLink,omitempty"`
	PdbConfigs     []PDBConfig      `json:"pdbConfigs,omitempty"`
	SetupDBCloning bool             `json:"setupDBCloning,omitempty"`
	DbBackupId     *string          `json:"dbBackupId,omitempty"`
	DatabaseId     *string          `json:"databaseId,omitempty"`
	KMSConfig      *KMSConfig       `json:"kmsConfig,omitempty"`
	EnableBackup   bool             `json:"enableBackup,omitempty"`
	IsPatch        bool             `json:"isPatch,omitempty"`
	IsUpgrade      bool             `json:"isUpgrade,omitempty"`
	DataGuard      DataGuardConfig  `json:"dataGuard,omitempty"`
}

// DbSystemDetails Spec

type DbSystemDetails struct {
	CompartmentId              string            `json:"compartmentId,omitempty"`
	AvailabilityDomain         string            `json:"availabilityDomain,omitempty"`
	SubnetId                   string            `json:"subnetId,omitempty"`
	Shape                      string            `json:"shape,omitempty"`
	SshPublicKeys              []string          `json:"sshPublicKeys,omitempty"`
	HostName                   string            `json:"hostName,omitempty"`
	CpuCoreCount               int               `json:"cpuCoreCount,omitempty"`
	FaultDomains               []string          `json:"faultDomains,omitempty"`
	DisplayName                string            `json:"displayName,omitempty"`
	BackupDisplayName          string            `json:"backupDisplayName,omitempty"`
	BackupSubnetId             string            `json:"backupSubnetId,omitempty"`
	TimeZone                   string            `json:"timeZone,omitempty"`
	NodeCount                  *int              `json:"nodeCount,omitempty"`
	PrivateIp                  string            `json:"privateIp,omitempty"`
	Domain                     string            `json:"domain,omitempty"`
	InitialDataStorageSizeInGB int               `json:"initialDataStorageSizeInGB,omitempty"`
	ClusterName                string            `json:"clusterName,omitempty"`
	DbAdminPasswordSecret      string            `json:"dbAdminPasswordSecret,omitempty"`
	DbName                     string            `json:"dbName,omitempty"`
	DbHomeId                   string            `json:"dbHomeId,omitempty"`
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
	DbBackupConfig             *Backupconfig     `json:"dbBackupConfig,omitempty"`
	KMSConfig                  *KMSConfig        `json:"kmsConfig,omitempty"`
	RestoreConfig              *RestoreConfig    `json:"restoreConfig,omitempty"`
	PatchOCID                  string            `json:"dbPatchOcid,omitempty"`
	UpgradeVersion             string            `json:"dbUpgradeVersion,omitempty"`
}

type DataGuardConfig struct {
	Enabled               bool              `json:"enabled,omitempty"`
	ProtectionMode        *string           `json:"protectionMode,omitempty"` // Options: "MAXIMUM_PROTECTION", "MAXIMUM_AVAILABILITY", "MAXIMUM_PERFORMANCE"
	TransportType         *string           `json:"transportType,omitempty"`  // Options: "ASYNC", "SYNC"
	PeerRole              *string           `json:"peerRole,omitempty"`       // Options: "STANDBY", "PRIMARY"
	DbAdminPasswordSecret *string           `json:"dbAdminPasswordSecret,omitempty"`
	DbName                *string           `json:"dbName,omitempty"`
	HostName              *string           `json:"hostName,omitempty"`
	DisplayName           *string           `json:"displayName,omitempty"`
	PeerSidPrefix         *string           `json:"sidPrefix,omitempty"`
	PeerDbSystemId        *string           `json:"peerDbSystemId,omitempty"`
	PeerDbHomeId          *string           `json:"peerDbHomeId,omitempty"`
	PrimaryDatabaseId     *string           `json:"primaryDatabaseId,omitempty"`
	AvailabilityDomain    *string           `json:"availabilityDomain,omitempty"` // Availability domain for the new DB system
	SubnetId              *string           `json:"subnetId,omitempty"`           // Subnet ID for the new DB system
	Shape                 *string           `json:"shape,omitempty"`              // Shape of the new DB system
	DbSystemFreeformTags  map[string]string `json:"dbSystemFreeformTags,omitempty"`
	IsDelete              bool              `json:"isDelete,omitempty"`
}

// DB Backup Config Network Struct
type Backupconfig struct {
	AutoBackupEnabled        *bool   `json:"autoBackupEnabled,omitempty"`
	RecoveryWindowsInDays    *int    `json:"recoveryWindowsInDays,omitempty"`
	AutoBackupWindow         *string `json:"autoBackupWindow,omitempty"`
	BackupDestinationDetails *string `json:"backupDestinationDetails,omitempty"`
}

// Manual backup information
type BackupInfo struct {
	Name      string `json:"name"`
	BackupID  string `json:"backupId"`
	Timestamp string `json:"timestamp"` // Optional: for sorting, audit, GC
}

type RestoreConfig struct {
	Timestamp *metav1.Time `json:"timestamp,omitempty"` // Restore to specific point in time
	SCN       *string      `json:"scn,omitempty"`       // Restore to specific SCN (as string)
	Latest    bool         `json:"latest,omitempty"`    // Restore to latest state
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

	Shape            *string            `json:"shape,omitempty"`
	State            LifecycleState     `json:"state"`
	DbInfo           []DbStatus         `json:"dbInfo,omitempty"`
	Network          VmNetworkDetails   `json:"network,omitempty"`
	WorkRequests     []DbWorkrequests   `json:"workRequests,omitempty"`
	KMSDetailsStatus KMSDetailsStatus   `json:"kmsDetailsStatus,omitempty"`
	DbCloneStatus    DbCloneStatus      `json:"dbCloneStatus,omitempty"`
	PdbDetailsStatus []PDBDetailsStatus `json:"pdbDetailsStatus,omitempty"`
	DataGuardStatus  DataGuardStatus    `json:"dataGuardStatus,omitempty"`
	Backups          []BackupInfo       `json:"backups,omitempty"`
	Message          string             `json:"message,omitempty"`
}

// DbcsSystemStatus defines the observed state of DbcsSystem
type DbStatus struct {
	Id                   *string `json:"id,omitempty"`
	DbName               string  `json:"dbName,omitempty"`
	DbUniqueName         string  `json:"dbUniqueName,omitempty"`
	DbWorkload           string  `json:"dbWorkload,omitempty"`
	DbHomeId             string  `json:"dbHomeId,omitempty"`
	ConnectionString     string  `json:"connectionString,omitempty"`
	ConnectionStringLong string  `json:"connectionStringLong,omitempty"`
}

type DbWorkrequests struct {
	OperationType   *string `json:"operationType,omitempty"`
	OperationId     *string `json:"operationId,omitempty"`
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

// DbCloneConfig defines the configuration for the database clone
type DbCloneConfig struct {
	DbAdminPasswordSecret      string   `json:"dbAdminPasswordSecret,omitempty"`
	TdeWalletPasswordSecret    string   `json:"tdeWalletPasswordSecret,omitempty"`
	DbName                     string   `json:"dbName"`
	HostName                   string   `json:"hostName"`
	DbUniqueName               string   `json:"dbDbUniqueName"`
	DisplayName                string   `json:"displayName"`
	LicenseModel               string   `json:"licenseModel,omitempty"`
	Domain                     string   `json:"domain,omitempty"`
	SshPublicKeys              []string `json:"sshPublicKeys,omitempty"`
	SubnetId                   string   `json:"subnetId"`
	SidPrefix                  string   `json:"sidPrefix,omitempty"`
	InitialDataStorageSizeInGB int      `json:"initialDataStorageSizeInGB,omitempty"`
	KmsKeyId                   string   `json:"kmsKeyId,omitempty"`
	KmsKeyVersionId            string   `json:"kmsKeyVersionId,omitempty"`
	PrivateIp                  string   `json:"privateIp,omitempty"`
}

type DataGuardStatus struct {
	Id                         *string `json:"id,omitempty"`
	IsActiveDataGuardEnabled   bool    `json:"isActiveDataGuardEnabled,omitempty"`
	PeerDbSystemId             *string `json:"peerDbSystemId,omitempty"`
	PeerDatabaseId             *string `json:"peerDatabaseId,omitempty"`
	DbName                     *string `json:"dbName,omitempty"`
	DbWorkload                 string  `json:"dbWorkload,omitempty"`
	PeerDbHomeId               *string `json:"peerDbHomeId,omitempty"`
	PeerRole                   *string `json:"peerRole,omitempty"` // Options: "STANDBY", "PRIMARY"
	Shape                      *string `json:"shape,omitempty"`
	SubnetId                   *string `json:"subnetId,omitempty"`
	PrimaryDatabaseId          *string `json:"primaryDatabaseId,omitempty"`
	DbAdminPasswordSecret      *string `json:"dbAdminPasswordSecret,omitempty"`
	TransportType              *string `json:"transportType,omitempty"`
	ProtectionMode             *string `json:"protectionMode,omitempty"` // Options: "MAXIMUM_PROTECTION", "MAXIMUM_AVAILABILITY", "MAXIMUM_PERFORMANCE"
	LifecycleState             *string `json:"lifecycleState,omitempty"`
	PeerDataGuardAssociationId *string `json:"peerDataGuardAssociationId,omitempty"`
	LifecycleDetails           *string `json:"lifecycleDetails,omitempty"`
}

// DbCloneStatus defines the observed state of DbClone
type DbCloneStatus struct {
	Id                    *string  `json:"id,omitempty"`
	DbAdminPasswordSecret string   `json:"dbAdminPasswordSecret,omitempty"`
	DbName                string   `json:"dbName,omitempty"`
	HostName              string   `json:"hostName"`
	DbUniqueName          string   `json:"dbDbUniqueName"`
	DisplayName           string   `json:"displayName,omitempty"`
	LicenseModel          string   `json:"licenseModel,omitempty"`
	Domain                string   `json:"domain,omitempty"`
	SshPublicKeys         []string `json:"sshPublicKeys,omitempty"`
	SubnetId              string   `json:"subnetId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=dbcssystems,scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".status.displayName"
// +kubebuilder:printcolumn:name="DB Name",type="string",JSONPath=".status.dbInfo[0].dbName"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="OCPUs",type="integer",JSONPath=".status.cpuCoreCount"
// +kubebuilder:printcolumn:name="Storage (TB)",type="integer",JSONPath=".status.dataStorageSizeInGBs"
// +kubebuilder:printcolumn:name="Reco Storage (GB)",type="integer",JSONPath=".status.recoStorageSizeInGB"
// +kubebuilder:printcolumn:name="Storage Mgmt",type="string",JSONPath=".status.storageManagement"
// +kubebuilder:printcolumn:name="ConnString",type="string",JSONPath=".status.dbInfo[0].connectionString"
type DbcsSystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DbcsSystemSpec   `json:"spec,omitempty"`
	Status            DbcsSystemStatus `json:"status,omitempty"`
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
	Upgrade   LifecycleState = "UPGRADING"
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
func (dbcs *DbcsSystem) GetLastSuccessfulSpecWithLog(log logr.Logger) (*DbcsSystemSpec, error) {
	val, ok := dbcs.GetAnnotations()[lastSuccessfulSpec]
	if !ok {
		log.Info("No last successful spec annotation found")
		return nil, nil
	}

	specBytes := []byte(val)
	sucSpec := DbcsSystemSpec{}

	err := json.Unmarshal(specBytes, &sucSpec)
	if err != nil {
		log.Error(err, "Failed to unmarshal last successful spec")
		return nil, err
	}

	log.Info("Successfully retrieved last successful spec", "spec", sucSpec)
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
