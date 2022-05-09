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
	"reflect"

	"github.com/oracle/oci-go-sdk/v63/database"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// name of our custom finalizer
const ADBFinalizer = "database.oracle.com/adb-finalizer"

// AutonomousDatabaseSpec defines the desired state of AutonomousDatabase
// Important: Run "make" to regenerate code after modifying this file
type AutonomousDatabaseSpec struct {
	Details   AutonomousDatabaseDetails `json:"details"`
	OCIConfig OCIConfigSpec             `json:"ociConfig,omitempty"`
	// +kubebuilder:default:=false
	HardLink *bool `json:"hardLink,omitempty"`
}

/************************
*	ACD specs
************************/
type K8sACDSpec struct {
	Name *string `json:"name,omitempty"`
}

type OCIACDSpec struct {
	OCID *string `json:"ocid,omitempty"`
}

// ACDSpec defines the spec of the target for backup/restore runs.
// The name could be the name of an AutonomousDatabase or an AutonomousDatabaseBackup
type ACDSpec struct {
	K8sACD K8sACDSpec `json:"k8sACD,omitempty"`
	OCIACD OCIACDSpec `json:"ociACD,omitempty"`
}

/************************
*	Secret specs
************************/
type K8sSecretSpec struct {
	Name *string `json:"name,omitempty"`
}

type OCISecretSpec struct {
	OCID *string `json:"ocid,omitempty"`
}

type PasswordSpec struct {
	K8sSecret K8sSecretSpec `json:"k8sSecret,omitempty"`
	OCISecret OCISecretSpec `json:"ociSecret,omitempty"`
}

type WalletSpec struct {
	Name     *string      `json:"name,omitempty"`
	Password PasswordSpec `json:"password,omitempty"`
}

/************************
*	Network Access specs
************************/

type NetworkAccessTypeEnum string

const (
	NetworkAccessTypePublic     NetworkAccessTypeEnum = "PUBLIC"
	NetworkAccessTypeRestricted NetworkAccessTypeEnum = "RESTRICTED"
	NetworkAccessTypePrivate    NetworkAccessTypeEnum = "PRIVATE"
)

type NetworkAccessSpec struct {
	// +kubebuilder:validation:Enum:="";"PUBLIC";"RESTRICTED";"PRIVATE"
	AccessType               NetworkAccessTypeEnum `json:"accessType,omitempty"`
	IsAccessControlEnabled   *bool                 `json:"isAccessControlEnabled,omitempty"`
	AccessControlList        []string              `json:"accessControlList,omitempty"`
	PrivateEndpoint          PrivateEndpointSpec   `json:"privateEndpoint,omitempty"`
	IsMTLSConnectionRequired *bool                 `json:"isMTLSConnectionRequired,omitempty"`
}

type PrivateEndpointSpec struct {
	SubnetOCID     *string  `json:"subnetOCID,omitempty"`
	NsgOCIDs       []string `json:"nsgOCIDs,omitempty"`
	HostnamePrefix *string  `json:"hostnamePrefix,omitempty"`
}

// AutonomousDatabaseDetails defines the detail information of AutonomousDatabase, corresponding to oci-go-sdk/database/AutonomousDatabase
type AutonomousDatabaseDetails struct {
	AutonomousDatabaseOCID      *string `json:"autonomousDatabaseOCID,omitempty"`
	CompartmentOCID             *string `json:"compartmentOCID,omitempty"`
	AutonomousContainerDatabase ACDSpec `json:"autonomousContainerDatabase,omitempty"`
	DisplayName                 *string `json:"displayName,omitempty"`
	DbName                      *string `json:"dbName,omitempty"`
	// +kubebuilder:validation:Enum:="OLTP";"DW";"AJD";"APEX"
	DbWorkload database.AutonomousDatabaseDbWorkloadEnum `json:"dbWorkload,omitempty"`
	// +kubebuilder:validation:Enum:="LICENSE_INCLUDED";"BRING_YOUR_OWN_LICENSE"
	LicenseModel         database.AutonomousDatabaseLicenseModelEnum   `json:"licenseModel,omitempty"`
	DbVersion            *string                                       `json:"dbVersion,omitempty"`
	DataStorageSizeInTBs *int                                          `json:"dataStorageSizeInTBs,omitempty"`
	CPUCoreCount         *int                                          `json:"cpuCoreCount,omitempty"`
	AdminPassword        PasswordSpec                                  `json:"adminPassword,omitempty"`
	IsAutoScalingEnabled *bool                                         `json:"isAutoScalingEnabled,omitempty"`
	IsDedicated          *bool                                         `json:"isDedicated,omitempty"`
	LifecycleState       database.AutonomousDatabaseLifecycleStateEnum `json:"lifecycleState,omitempty"`

	NetworkAccess NetworkAccessSpec `json:"networkAccess,omitempty"`

	FreeformTags map[string]string `json:"freeformTags,omitempty"`

	Wallet WalletSpec `json:"wallet,omitempty"`
}

// AutonomousDatabaseStatus defines the observed state of AutonomousDatabase
type AutonomousDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LifecycleState       database.AutonomousDatabaseLifecycleStateEnum `json:"lifecycleState,omitempty"`
	TimeCreated          string                                        `json:"timeCreated,omitempty"`
	AllConnectionStrings []ConnectionStringProfile                     `json:"allConnectionStrings,omitempty"`
}

type TLSAuthenticationEnum string

const (
	tlsAuthenticationTLS  TLSAuthenticationEnum = "TLS"
	tlsAuthenticationMTLS TLSAuthenticationEnum = "Mutual TLS"
)

type ConnectionStringProfile struct {
	TLSAuthentication TLSAuthenticationEnum  `json:"tlsAuthentication,omitempty"`
	ConnectionStrings []ConnectionStringSpec `json:"connectionStrings"`
}

type ConnectionStringSpec struct {
	TNSName          string `json:"tnsName,omitempty"`
	ConnectionString string `json:"connectionString,omitempty"`
}

// AutonomousDatabase is the Schema for the autonomousdatabases API
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName="adb";"adbs"
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.details.displayName",name="Display Name",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.details.dbName",name="Db Name",type=string
// +kubebuilder:printcolumn:JSONPath=".status.lifecycleState",name="State",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.details.isDedicated",name="Dedicated",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.details.cpuCoreCount",name="OCPUs",type=integer
// +kubebuilder:printcolumn:JSONPath=".spec.details.dataStorageSizeInTBs",name="Storage (TB)",type=integer
// +kubebuilder:printcolumn:JSONPath=".spec.details.dbWorkload",name="Workload Type",type=string
// +kubebuilder:printcolumn:JSONPath=".status.timeCreated",name="Created",type=string
type AutonomousDatabase struct {
	metaV1.TypeMeta   `json:",inline"`
	metaV1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutonomousDatabaseSpec   `json:"spec,omitempty"`
	Status AutonomousDatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AutonomousDatabaseList contains a list of AutonomousDatabase
type AutonomousDatabaseList struct {
	metaV1.TypeMeta `json:",inline"`
	metaV1.ListMeta `json:"metadata,omitempty"`
	Items           []AutonomousDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutonomousDatabase{}, &AutonomousDatabaseList{})
}

// GetLastSuccessfulSpec returns spec from the lass successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulSpec.
func (adb *AutonomousDatabase) GetLastSuccessfulSpec() (*AutonomousDatabaseSpec, error) {
	val, ok := adb.GetAnnotations()[LastSuccessfulSpec]
	if !ok {
		return nil, nil
	}

	specBytes := []byte(val)
	sucSpec := AutonomousDatabaseSpec{}

	err := json.Unmarshal(specBytes, &sucSpec)
	if err != nil {
		return nil, err
	}

	return &sucSpec, nil
}

func (adb *AutonomousDatabase) UpdateLastSuccessfulSpec() error {
	specBytes, err := json.Marshal(adb.Spec)
	if err != nil {
		return err
	}

	anns := adb.GetAnnotations()

	if anns == nil {
		anns = map[string]string{
			LastSuccessfulSpec: string(specBytes),
		}
	} else {
		anns[LastSuccessfulSpec] = string(specBytes)
	}

	adb.SetAnnotations(anns)

	return nil
}

// UpdateStatusFromOCIADB updates the status subresource
func (adb *AutonomousDatabase) UpdateStatusFromOCIADB(ociObj database.AutonomousDatabase) {
	adb.Status.LifecycleState = ociObj.LifecycleState
	adb.Status.TimeCreated = FormatSDKTime(ociObj.TimeCreated)

	if *ociObj.IsDedicated {
		conns := make([]ConnectionStringSpec, len(ociObj.ConnectionStrings.AllConnectionStrings))
		for key, val := range ociObj.ConnectionStrings.AllConnectionStrings {
			conns = append(conns, ConnectionStringSpec{TNSName: key, ConnectionString: val})
		}

		adb.Status.AllConnectionStrings = []ConnectionStringProfile{
			{ConnectionStrings: conns},
		}
	} else {
		var mTLSConns []ConnectionStringSpec
		var tlsConns []ConnectionStringSpec

		var conns []ConnectionStringProfile

		for _, profile := range ociObj.ConnectionStrings.Profiles {
			if profile.TlsAuthentication == database.DatabaseConnectionStringProfileTlsAuthenticationMutual {
				mTLSConns = append(mTLSConns, ConnectionStringSpec{TNSName: *profile.DisplayName, ConnectionString: *profile.Value})
			} else {
				tlsConns = append(tlsConns, ConnectionStringSpec{TNSName: *profile.DisplayName, ConnectionString: *profile.Value})
			}
		}

		if len(mTLSConns) > 0 {
			conns = append(conns, ConnectionStringProfile{
				TLSAuthentication: tlsAuthenticationMTLS,
				ConnectionStrings: mTLSConns,
			})
		}

		if len(tlsConns) > 0 {
			conns = append(conns, ConnectionStringProfile{
				TLSAuthentication: tlsAuthenticationTLS,
				ConnectionStrings: tlsConns,
			})
		}

		adb.Status.AllConnectionStrings = conns
	}
}

// UpdateFromOCIADB updates the attributes using database.AutonomousDatabase object
func (adb *AutonomousDatabase) UpdateFromOCIADB(ociObj database.AutonomousDatabase) (specChanged bool) {
	oldADB := adb.DeepCopy()

	/***********************************
	* update the spec
	***********************************/
	adb.Spec.Details.AutonomousDatabaseOCID = ociObj.Id
	adb.Spec.Details.CompartmentOCID = ociObj.CompartmentId
	adb.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID = ociObj.AutonomousContainerDatabaseId
	adb.Spec.Details.DisplayName = ociObj.DisplayName
	adb.Spec.Details.DbName = ociObj.DbName
	adb.Spec.Details.DbWorkload = ociObj.DbWorkload
	adb.Spec.Details.LicenseModel = ociObj.LicenseModel
	adb.Spec.Details.DbVersion = ociObj.DbVersion
	adb.Spec.Details.DataStorageSizeInTBs = ociObj.DataStorageSizeInTBs
	adb.Spec.Details.CPUCoreCount = ociObj.CpuCoreCount
	adb.Spec.Details.IsAutoScalingEnabled = ociObj.IsAutoScalingEnabled
	adb.Spec.Details.IsDedicated = ociObj.IsDedicated
	adb.Spec.Details.LifecycleState = NextADBStableState(ociObj.LifecycleState)
	// Special case: an emtpy map will be nil after unmarshalling while the OCI always returns an emty map.
	if len(ociObj.FreeformTags) != 0 {
		adb.Spec.Details.FreeformTags = ociObj.FreeformTags
	} else {
		adb.Spec.Details.FreeformTags = nil
	}

	// Determine network.accessType
	if *ociObj.IsDedicated {
		adb.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypePrivate
	} else {
		if ociObj.NsgIds != nil {
			adb.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypePrivate
		} else if ociObj.WhitelistedIps != nil {
			adb.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypeRestricted
		} else {
			adb.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypePublic
		}
	}

	adb.Spec.Details.NetworkAccess.IsAccessControlEnabled = ociObj.IsAccessControlEnabled
	if len(ociObj.WhitelistedIps) != 0 {
		adb.Spec.Details.NetworkAccess.AccessControlList = ociObj.WhitelistedIps
	} else {
		adb.Spec.Details.NetworkAccess.AccessControlList = nil
	}
	adb.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = ociObj.IsMtlsConnectionRequired
	adb.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID = ociObj.SubnetId
	if len(ociObj.NsgIds) != 0 {
		adb.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = ociObj.NsgIds
	} else {
		adb.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = nil
	}
	adb.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix = ociObj.PrivateEndpointLabel

	// The admin password is not going to be updated in a bind operation. Erase the field if the lastSucSpec is nil.
	// Leave the wallet field as is because the download wallet operation is independent from the update operation.
	lastSucSpec, _ := adb.GetLastSuccessfulSpec()
	if lastSucSpec == nil {
		adb.Spec.Details.AdminPassword = PasswordSpec{}
	} else {
		adb.Spec.Details.AdminPassword = lastSucSpec.Details.AdminPassword
	}

	/***********************************
	* update the status subresource
	***********************************/
	adb.UpdateStatusFromOCIADB(ociObj)

	return !reflect.DeepEqual(oldADB.Spec, adb.Spec)
}

// RemoveUnchangedDetails removes the unchanged fields in spec.details, and returns if the details has been changed.
func (adb *AutonomousDatabase) RemoveUnchangedDetails(prevSpec AutonomousDatabaseSpec) (bool, error) {

	changed, err := removeUnchangedFields(prevSpec.Details, &adb.Spec.Details)
	if err != nil {
		return changed, err
	}

	return changed, nil
}

// A helper function which is useful for debugging. The function prints out a structural JSON format.
func (adb *AutonomousDatabase) String() (string, error) {
	out, err := json.MarshalIndent(adb, "", "    ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
