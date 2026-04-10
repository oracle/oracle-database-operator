/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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

// revive:disable:exported,var-naming
// Legacy API field/type names are preserved for backward compatibility.

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SingleInstanceDatabaseSpec defines the desired state of SingleInstanceDatabase
type SingleInstanceDatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Enum=standard;enterprise;express;free
	Edition string `json:"edition,omitempty"`

	// SID must be alphanumeric (no special characters, only a-z, A-Z, 0-9), and no longer than 12 characters.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]+$`
	// +kubebuilder:validation:MaxLength:=12
	Sid string `json:"sid,omitempty"`

	Charset string `json:"charset,omitempty"`
	Pdbname string `json:"pdbName,omitempty"`

	LoadBalancer bool `json:"loadBalancer,omitempty"`
	ListenerPort int  `json:"listenerPort,omitempty"`
	// Deprecated: use spec.tcps.listenerPort.
	TcpsListenerPort   int               `json:"tcpsListenerPort,omitempty"`
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	FlashBack    *bool `json:"flashBack,omitempty"`
	ArchiveLog   *bool `json:"archiveLog,omitempty"`
	ForceLogging *bool `json:"forceLog,omitempty"`

	// Deprecated: use spec.security.tcps.
	// TCPS groups TCPS-related settings. All fields are optional.
	TCPS *SingleInstanceDatabaseTCPS `json:"tcps,omitempty"`

	// Security groups security-related settings (secrets and TCPS).
	Security *SingleInstanceDatabaseSecurity `json:"security,omitempty"`

	// TNSAliases configures explicit tnsnames.ora aliases managed by the operator.
	TNSAliases []SingleInstanceDatabaseTNSAlias `json:"tnsAliases,omitempty"`

	// Deprecated: use spec.tcps.enabled.
	EnableTCPS bool `json:"enableTCPS,omitempty"`
	// Deprecated: use spec.tcps.certRenewInterval.
	TcpsCertRenewInterval string `json:"tcpsCertRenewInterval,omitempty"`
	// Deprecated: use spec.tcps.tlsSecret.
	TcpsTlsSecret string `json:"tcpsTlsSecret,omitempty"`

	// Deprecated: use spec.primarySource.databaseRef.
	PrimaryDatabaseRef string                               `json:"primaryDatabaseRef,omitempty"`
	PrimarySource      *SingleInstanceDatabasePrimarySource `json:"primarySource,omitempty"`
	Dataguard          *DataguardProducerSpec               `json:"dataguard,omitempty"`

	// +kubebuilder:validation:Enum=primary;standby;clone;truecache
	CreateAs string `json:"createAs,omitempty"`

	ReadinessCheckPeriod int      `json:"readinessCheckPeriod,omitempty"`
	ServiceAccountName   string   `json:"serviceAccountName,omitempty"`
	TrueCacheServices    []string `json:"trueCacheServices,omitempty"`

	// +k8s:openapi-gen=true
	Replicas int `json:"replicas,omitempty"`

	NodeSelector map[string]string  `json:"nodeSelector,omitempty"`
	HostAliases  []corev1.HostAlias `json:"hostAliases,omitempty"`
	// Deprecated: use spec.security.secrets.admin.
	AdminPassword SingleInstanceDatabaseAdminPassword `json:"adminPassword,omitempty"`
	Image         SingleInstanceDatabaseImage         `json:"image"`
	Persistence   SingleInstanceDatabasePersistence   `json:"persistence,omitempty"`
	InitParams    *SingleInstanceDatabaseInitParams   `json:"initParams,omitempty"`
	// Deprecated: use resourceRequirements for full Kubernetes resource support including hugepages.
	Resources                     SingleInstanceDatabaseResources `json:"resources,omitempty"`
	ResourceRequirements          *corev1.ResourceRequirements    `json:"resourceRequirements,omitempty" protobuf:"bytes,1,opt,name=resourceRequirements"`
	DisableDefaultDiagVolumeClaim bool                            `json:"disableDefaultDiagVolumeClaim,omitempty"`
	SecurityContext               *corev1.PodSecurityContext      `json:"securityContext,omitempty"`
	Capabilities                  *corev1.Capabilities            `json:"capabilities,omitempty"`

	ConvertToSnapshotStandby bool            `json:"convertToSnapshotStandby,omitempty"`
	EnvVars                  []corev1.EnvVar `json:"envVars,omitempty"`
	// Restore config enables primary creation from backup sources.
	Restore *SingleInstanceDatabaseRestoreSpec `json:"restore,omitempty"`

	// - For primary: enables blob generation and sets generation path
	// - For truecache: references existing blob ConfigMap and sets mount path
	TrueCache *SingleInstanceDatabaseTrueCacheSpec `json:"trueCache,omitempty"`
}

// SingleInstanceDatabaseRestoreSpec defines restore-from-backup settings for primary creation.
type SingleInstanceDatabaseRestoreSpec struct {
	// ObjectStore source parameters. Mutually exclusive with fileSystem.
	ObjectStore *SingleInstanceDatabaseRestoreObjectStoreSpec `json:"objectStore,omitempty"`
	// FileSystem source parameters. Mutually exclusive with objectStore.
	FileSystem *SingleInstanceDatabaseRestoreFileSystemSpec `json:"fileSystem,omitempty"`
	// Target restore layout overrides.
	Target *SingleInstanceDatabaseRestoreTargetSpec `json:"target,omitempty"`
	// Optional restore behavior overrides.
	Options *SingleInstanceDatabaseRestoreOptionsSpec `json:"options,omitempty"`
}

type SingleInstanceDatabaseSecretKeyRef struct {
	SecretName string `json:"secretName,omitempty"`
	Key        string `json:"key,omitempty"`
}

type SingleInstanceDatabaseConfigMapKeyRef struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	Key           string `json:"key,omitempty"`
}

type SingleInstanceDatabaseRestoreObjectStoreSpec struct {
	OCIConfig        *SingleInstanceDatabaseConfigMapKeyRef `json:"ociConfig,omitempty"`
	PrivateKey       *SingleInstanceDatabaseSecretKeyRef    `json:"privateKey,omitempty"`
	SourceDBWallet   *SingleInstanceDatabaseSecretKeyRef    `json:"sourceDbWallet,omitempty"`
	SourceDBWalletPw *SingleInstanceDatabaseSecretKeyRef    `json:"sourceDbWalletPassword,omitempty"`
	BackupModuleConf *SingleInstanceDatabaseConfigMapKeyRef `json:"backupModuleConfig,omitempty"`
	OpcInstallerZip  *SingleInstanceDatabaseConfigMapKeyRef `json:"opcInstallerZip,omitempty"`
	BackupIdentity   *SingleInstanceDatabaseBackupIdentity  `json:"backupIdentity,omitempty"`
	EncryptedBackup  *SingleInstanceDatabaseEncryptedBackup `json:"encryptedBackup,omitempty"`
}

type SingleInstanceDatabaseRestoreFileSystemSpec struct {
	BackupPath       string                                 `json:"backupPath,omitempty"`
	CatalogStartWith string                                 `json:"catalogStartWith,omitempty"`
	SourceDBWallet   *SingleInstanceDatabaseSecretKeyRef    `json:"sourceDbWallet,omitempty"`
	SourceDBWalletPw *SingleInstanceDatabaseSecretKeyRef    `json:"sourceDbWalletPassword,omitempty"`
	EncryptedBackup  *SingleInstanceDatabaseEncryptedBackup `json:"encryptedBackup,omitempty"`
}

type SingleInstanceDatabaseBackupIdentity struct {
	BucketName      string `json:"bucketName,omitempty"`
	DBID            string `json:"dbid,omitempty"`
	CompartmentOCID string `json:"compartmentOcid,omitempty"`
}

type SingleInstanceDatabaseEncryptedBackup struct {
	Enabled               bool                                `json:"enabled,omitempty"`
	DecryptPasswordSecret *SingleInstanceDatabaseSecretKeyRef `json:"decryptPasswordSecret,omitempty"`
}

type SingleInstanceDatabaseRestoreTargetSpec struct {
	DataRoot   string `json:"dataRoot,omitempty"`
	WalletRoot string `json:"walletRoot,omitempty"`
}

type SingleInstanceDatabaseRestoreOptionsSpec struct {
	SourceDBName      string `json:"sourceDbName,omitempty"`
	RunCrosscheck     *bool  `json:"runCrosscheck,omitempty"`
	RunValidateOnly   *bool  `json:"runValidateOnly,omitempty"`
	ForceOpcReinstall *bool  `json:"forceOpcReinstall,omitempty"`
}

// SingleInstanceDatabaseTCPS defines TCPS configuration in a single structure.
type SingleInstanceDatabaseTCPS struct {
	Enabled bool `json:"enabled,omitempty"`
	// ListenerPort is the external NodePort/LoadBalancer port for TCPS.
	ListenerPort int `json:"listenerPort,omitempty"`
	// TlsSecret references the Kubernetes TLS secret containing tls.crt and tls.key.
	TlsSecret string `json:"tlsSecret,omitempty"`
	// ClientWalletSecret optionally overrides the Secret used by DataguardBroker
	// for TCPS client connectivity. When unset, the SIDB controller publishes an
	// operator-generated wallet secret for Data Guard consumers.
	ClientWalletSecret string `json:"clientWalletSecret,omitempty"`
	// CertRenewInterval is used only when self-signed certificates are used.
	CertRenewInterval string `json:"certRenewInterval,omitempty"`
	// CertMountLocation is the in-pod mount path for the TCPS TLS secret.
	// Defaults to /run/secrets/tls_secret when not set.
	CertMountLocation string `json:"certMountLocation,omitempty"`
}

// SingleInstanceDatabaseTNSAliasProtocol identifies the transport protocol for a TNS alias.
type SingleInstanceDatabaseTNSAliasProtocol string

const (
	// SingleInstanceDatabaseTNSAliasProtocolTCP configures a TCP TNS alias.
	SingleInstanceDatabaseTNSAliasProtocolTCP SingleInstanceDatabaseTNSAliasProtocol = "TCP"
	// SingleInstanceDatabaseTNSAliasProtocolTCPS configures a TCPS TNS alias.
	SingleInstanceDatabaseTNSAliasProtocolTCPS SingleInstanceDatabaseTNSAliasProtocol = "TCPS"
)

// SingleInstanceDatabaseTNSAlias defines a managed entry in tnsnames.ora.
type SingleInstanceDatabaseTNSAlias struct {
	Name        string                                 `json:"name,omitempty"`
	Host        string                                 `json:"host,omitempty"`
	Port        int                                    `json:"port,omitempty"`
	ServiceName string                                 `json:"serviceName,omitempty"`
	Protocol    SingleInstanceDatabaseTNSAliasProtocol `json:"protocol,omitempty"`
	SSLServerDN string                                 `json:"sslServerDN,omitempty"`
}

// SingleInstanceDatabaseSecurity defines grouped security configuration.
type SingleInstanceDatabaseSecurity struct {
	// TCPS groups TCPS-related settings.
	TCPS *SingleInstanceDatabaseTCPS `json:"tcps,omitempty"`
	// Secrets groups password/secret references used by SIDB flows.
	Secrets *SingleInstanceDatabaseSecrets `json:"secrets,omitempty"`
}

// SingleInstanceDatabaseSecrets defines grouped secret references.
type SingleInstanceDatabaseSecrets struct {
	// Admin password secret config.
	Admin *SingleInstanceDatabaseAdminPassword `json:"admin,omitempty"`
	// TDE password secret config.
	TDE *SingleInstanceDatabasePasswordSecret `json:"tde,omitempty"`
}

// SingleInstanceDatabasePasswordSecret defines a generic secret ref and optional mount path.
type SingleInstanceDatabasePasswordSecret struct {
	SecretName string `json:"secretName,omitempty"`
	SecretKey  string `json:"secretKey,omitempty"`
	MountPath  string `json:"mountPath,omitempty"`
	// WalletZipFileKey points to the secret key containing the standby wallet zip artifact.
	WalletZipFileKey string `json:"walletZipFileKey,omitempty"`
	// WalletRoot is the destination wallet root used for standby TDE wallet import/open operations.
	WalletRoot string `json:"walletRoot,omitempty"`
}

// Unified sub-struct for TrueCache options
type SingleInstanceDatabaseTrueCacheSpec struct {
	// --- For primary databases (createAs: primary) ---
	TruedbUniqueName string `json:"truedbUniqueName,omitempty"`

	// Enable automatic TrueCache blob generation in the primary pod (default: false).
	// When true, the operator will run dbca to create the blob and store it in a ConfigMap.
	// When false, the operator will not create the blob and the user must provide the ConfigMap for truecache consumers.
	// +optional
	GenerateEnabled bool `json:"generateEnabled,omitempty"`

	// Path inside the primary pod where the blob file is generated when generateEnabled=true
	// (default: /tmp/tc_config_blob.tar.gz).
	// +optional
	GeneratePath string `json:"generatePath,omitempty"`

	// --- For truecache instances (createAs: truecache) ---

	// Name of an existing ConfigMap containing the TrueCache blob file.
	// If set, the operator skips blob creation and uses this ConfigMap.
	// +optional
	BlobConfigMapRef string `json:"blobConfigMapRef,omitempty"`

	// Key within the ConfigMap that holds the blob file content (default: "tc_config_blob.tar.gz").
	// +optional
	BlobConfigMapKey string `json:"blobConfigMapKey,omitempty"`

	// Path inside the truecache container where the blob file is mounted (default: /stage/tc_config_blob.tar.gz).
	// +optional
	BlobMountPath     string   `json:"blobMountPath,omitempty"`
	TrueCacheServices []string `json:"trueCacheServices,omitempty"`
}

type SingleInstanceDatabaseResource struct {
	Cpu    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type SingleInstanceDatabaseResources struct {
	Requests *SingleInstanceDatabaseResource `json:"requests,omitempty"`
	Limits   *SingleInstanceDatabaseResource `json:"limits,omitempty"`
}
type SingleInstanceDatabasePrimaryDetails struct {
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Sid     string `json:"sid,omitempty"`
	Pdbname string `json:"pdbName,omitempty"`
}

type SingleInstanceDatabasePrimarySource struct {
	DatabaseRef   string                                `json:"databaseRef,omitempty"`
	ConnectString string                                `json:"connectString,omitempty"`
	Details       *SingleInstanceDatabasePrimaryDetails `json:"details,omitempty"`
}

// SingleInstanceDatabasePersistence defines the storage size and class for PVC
type SingleInstanceDatabasePersistence struct {
	// Oradata config for primary datafiles volume.
	Oradata *SingleInstanceDatabasePersistenceOradata `json:"oradata,omitempty"`
	// Fra config for fast recovery area volume.
	Fra *SingleInstanceDatabasePersistenceFra `json:"fra,omitempty"`
	// Additional PVCs for custom mount paths.
	AdditionalPVCs []AdditionalPVCSpec `json:"additionalPVCs,omitempty"`

	// Deprecated: use persistence.oradata.size.
	Size string `json:"size,omitempty"`
	// Deprecated: use persistence.oradata.storageClass.
	StorageClass string `json:"storageClass,omitempty"`
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	// Deprecated: use persistence.oradata.accessMode.
	AccessMode            string `json:"accessMode,omitempty"`
	DatafilesVolumeName   string `json:"datafilesVolumeName,omitempty"`
	ScriptsVolumeName     string `json:"scriptsVolumeName,omitempty"`
	VolumeClaimAnnotation string `json:"volumeClaimAnnotation,omitempty"`
	SetWritePermissions   *bool  `json:"setWritePermissions,omitempty"`
}

type SingleInstanceDatabasePersistenceOradata struct {
	PvcName      string `json:"pvcName,omitempty"`
	Size         string `json:"size,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	AccessMode string `json:"accessMode,omitempty"`
}

type SingleInstanceDatabasePersistenceFra struct {
	PvcName      string `json:"pvcName,omitempty"`
	Size         string `json:"size,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	AccessMode string `json:"accessMode,omitempty"`
	MountPath  string `json:"mountPath,omitempty"`
	// RecoveryAreaSize is translated to db_recovery_file_dest_size for FRA use-cases.
	RecoveryAreaSize string `json:"recoveryAreaSize,omitempty"`
}

// SingleInstanceDatabaseInitParams defines the Init Parameters
type SingleInstanceDatabaseInitParams struct {
	SgaTarget          int `json:"sgaTarget,omitempty"`
	PgaAggregateTarget int `json:"pgaAggregateTarget,omitempty"`
	CpuCount           int `json:"cpuCount,omitempty"`
	Processes          int `json:"processes,omitempty"`
}

// SingleInstanceDatabaseImage defines the Image source and pullSecrets for POD
type SingleInstanceDatabaseImage struct {
	Version     string `json:"version,omitempty"`
	PullFrom    string `json:"pullFrom"`
	PullSecrets string `json:"pullSecrets,omitempty"`
	PrebuiltDB  bool   `json:"prebuiltDB,omitempty"`
}

// SingleInsatnceAdminPassword defines the secret containing Admin Password mapped to secretKey for Database
type SingleInstanceDatabaseAdminPassword struct {
	SecretName string `json:"secretName"`
	// +kubebuilder:default:="oracle_pwd"
	SecretKey      string `json:"secretKey,omitempty"`
	KeepSecret     *bool  `json:"keepSecret,omitempty"`
	MountPath      string `json:"mountPath,omitempty"`
	SkipInitWallet bool   `json:"skipInitWallet,omitempty"`
}

// SingleInstanceDatabaseStatus defines the observed state of SingleInstanceDatabase
type SingleInstanceDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Nodes         []string `json:"nodes,omitempty"`
	Role          string   `json:"role,omitempty"`
	Status        string   `json:"status,omitempty"`
	Replicas      int      `json:"replicas,omitempty"`
	ReleaseUpdate string   `json:"releaseUpdate,omitempty"`
	DgBroker      *string  `json:"dgBroker,omitempty"`
	// +kubebuilder:default:="false"
	DatafilesPatched     string            `json:"datafilesPatched,omitempty"`
	ConnectString        string            `json:"connectString,omitempty"`
	ClusterConnectString string            `json:"clusterConnectString,omitempty"`
	TcpsConnectString    string            `json:"tcpsConnectString,omitempty"`
	StandbyDatabases     map[string]string `json:"standbyDatabases,omitempty"`
	// +kubebuilder:default:="false"
	DatafilesCreated     string `json:"datafilesCreated,omitempty"`
	Sid                  string `json:"sid,omitempty"`
	Edition              string `json:"edition,omitempty"`
	Charset              string `json:"charset,omitempty"`
	Pdbname              string `json:"pdbName,omitempty"`
	InitSgaSize          int    `json:"initSgaSize,omitempty"`
	InitPgaSize          int    `json:"initPgaSize,omitempty"`
	CreatedAs            string `json:"createdAs,omitempty"`
	FlashBack            string `json:"flashBack,omitempty"`
	ArchiveLog           string `json:"archiveLog,omitempty"`
	ForceLogging         string `json:"forceLog,omitempty"`
	OemExpressUrl        string `json:"oemExpressUrl,omitempty"`
	OrdsReference        string `json:"ordsReference,omitempty"`
	PdbConnectString     string `json:"pdbConnectString,omitempty"`
	TcpsPdbConnectString string `json:"tcpsPdbConnectString,omitempty"`
	ApexInstalled        bool   `json:"apexInstalled,omitempty"`
	PrebuiltDB           bool   `json:"prebuiltDB,omitempty"`
	// +kubebuilder:default:=false
	IsTcpsEnabled         bool   `json:"isTcpsEnabled"`
	CertCreationTimestamp string `json:"certCreationTimestamp,omitempty"`
	CertRenewInterval     string `json:"certRenewInterval,omitempty"`
	ClientWalletLoc       string `json:"clientWalletLoc,omitempty"`
	PrimaryDatabase       string `json:"primaryDatabase,omitempty"`
	// +kubebuilder:default:=""
	TcpsTlsSecret string                   `json:"tcpsTlsSecret"`
	Dataguard     *ProducerDataguardStatus `json:"dataguard,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	InitParams  SingleInstanceDatabaseInitParams  `json:"initParams,omitempty"`
	Persistence SingleInstanceDatabasePersistence `json:"persistence"`

	ConvertToSnapshotStandby bool `json:"convertToSnapshotStandby,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sidb;sidbs
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:printcolumn:JSONPath=".status.edition",name="Edition",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.sid",name="Sid",type="string",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.status",name="Status",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.role",name="Role",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.releaseUpdate",name="Version",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.connectString",name="Connect Str",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.pdbConnectString",name="Pdb Connect Str",type="string",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.tcpsConnectString",name="TCPS Connect Str",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.tcpsPdbConnectString",name="TCPS Pdb Connect Str",type="string", priority=1
// +kubebuilder:printcolumn:JSONPath=".status.oemExpressUrl",name="Oem Express Url",type="string"

// SingleInstanceDatabase is the Schema for the singleinstancedatabases API
// +kubebuilder:storageversion
type SingleInstanceDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SingleInstanceDatabaseSpec   `json:"spec,omitempty"`
	Status SingleInstanceDatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SingleInstanceDatabaseList contains a list of SingleInstanceDatabase
type SingleInstanceDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SingleInstanceDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SingleInstanceDatabase{}, &SingleInstanceDatabaseList{})
}
