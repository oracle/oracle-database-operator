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
	"sync"

	"encoding/json"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	annsv1 "github.com/oracle/oracle-database-operator/commons/annotations"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ShardingDatabaseSpec defines the desired state of ShardingDatabase
type ShardingDatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Shard                         []ShardSpec                  `json:"shard,omitempty"`
	Catalog                       []CatalogSpec                `json:"catalog"`       // The catalogSpes accept all the catalog parameters
	Gsm                           []GsmSpec                    `json:"gsm,omitempty"` // The GsmSpec will accept all the Gsm parameter
	GsmInfo                       *GsmInfo                     `json:"gsmInfo,omitempty"`
	GsmResources                  *corev1.ResourceRequirements `json:"gsmResources,omitempty" protobuf:"bytes,1,opt,name=gsmResources"` // Optional default resources applied to each gsm[] entry when gsm[i].resources is not set
	GsmServiceAnnotations         map[string]string            `json:"gsmServiceAnnotations,omitempty"`
	GsmExternalServiceAnnotations map[string]string            `json:"gsmExternalServiceAnnotations,omitempty"`
	HostAliases                   []corev1.HostAlias           `json:"hostAliases,omitempty"`
	StorageClass                  string                       `json:"storageClass,omitempty"`       // Optional Accept storage class name
	DbImage                       string                       `json:"dbImage"`                      // Accept DB Image name
	DbImagePullSecret             string                       `json:"dbImagePullSecret,omitempty"`  // Optional The name of an image pull secret in case of a private docker repository.
	GsmImage                      string                       `json:"gsmImage"`                     // Acccept the GSM image name
	GsmImagePullSecret            string                       `json:"gsmImagePullSecret,omitempty"` // Optional  The name of an image pull secret in case of a private docker repository.
	StagePvcName                  string                       `json:"stagePvcName,omitempty"`       // the Stagepvc  for the backup of cluster
	PortMappings                  []PortMapping                `json:"portMappings,omitempty"`       // Port mappings for the service that is created. The service is created if there is at least
	IsDebug                       bool                         `json:"isDebug,omitempty"`            // Optional parameter to enable logining
	IsExternalSvc                 bool                         `json:"isExternalSvc,omitempty"`
	IsClone                       bool                         `json:"isClone,omitempty"`
	// Deprecated: retained for backward compatibility with legacy manifests.
	IsDataGuard           bool   `json:"isDataGuard,omitempty"`
	ScriptsLocation       string `json:"scriptsLocation,omitempty"`
	IsDeleteOraPvc        bool   `json:"isDeleteOraPvc,omitempty"`
	ReadinessCheckPeriod  int    `json:"readinessCheckPeriod,omitempty"`
	LivenessCheckPeriod   int    `json:"liveinessCheckPeriod,omitempty"`
	ReplicationType       string `json:"replicationType,omitempty"`
	IsDownloadScripts     bool   `json:"isDownloadScripts,omitempty"`
	InvitedNodeSubnetFlag string `json:"invitedNodeSubnetFlag,omitempty"`
	InvitedNodeSubnet     string `json:"InvitedNodeSubnet,omitempty"`
	ShardingType          string `json:"shardingType,omitempty"`
	// Deprecated: use shardSpace.
	GsmShardSpace []ShardSpaceSpec `json:"gsmShardSpace,omitempty"`
	// Deprecated: use shardGroup.
	GsmShardGroup []ShardGroupSpec `json:"gsmShardGroup,omitempty"`
	// Deprecated: use region.
	ShardRegion []string `json:"shardRegion,omitempty"`
	// Deprecated: use region[].buddy.
	ShardBuddyRegion string           `json:"shardBuddyRegion,omitempty"`
	GsmService       []GsmServiceSpec `json:"gsmService,omitempty"`
	ShardConfigName  string           `json:"shardConfigName,omitempty"`
	GsmDevMode       string           `json:"gsmDevMode,omitempty"`
	DbSecret         *SecretDetails   `json:"dbSecret,omitempty"` //  Secret Name to be used with Shard
	// Deprecated: use tdeWallet.isEnabled.
	IsTdeWallet string `json:"isTdeWallet,omitempty"`
	// Deprecated: use tdeWallet.pvcName.
	TdeWalletPvc    string `json:"tdeWalletPvc,omitempty"`
	FssStorageClass string `json:"fssStorageClass,omitempty"`
	// Deprecated: use tdeWallet.mountPath.
	TdeWalletPvcMountLocation string            `json:"tdeWalletPvcMountLocation,omitempty"`
	TDEWallet                 *TDEWalletConfig  `json:"tdeWallet,omitempty"`
	DbEdition                 string            `json:"dbEdition,omitempty"`
	TopicId                   string            `json:"topicId,omitempty"`
	SrvAccountName            string            `json:"serviceAccountName,omitempty"`
	ShardInfo                 []ShardingDetails `json:"shardInfo,omitempty"`
	// New fields for GDD-style YAML
	Region                []RegionSpec       `json:"region,omitempty"`
	ShardGroup            []ShardGroupSpec   `json:"shardGroup,omitempty"`
	ShardSpace            []ShardSpaceSpec   `json:"shardSpace,omitempty"`
	EnableTCPS            bool               `json:"enableTCPS,omitempty"`
	TcpsCertRenewInterval int                `json:"tcpsCertRenewInterval,omitempty"`
	TcpsTlsSecret         string             `json:"tcpsTlsSecret,omitempty"`
	ImagePulllPolicy      *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Service               []GsmServiceSpec   `json:"service,omitempty"` // alias for `service:` block
}

// To understand Metav1.Condition, please refer the link https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1
// ShardingDatabaseStatus defines the observed state of ShardingDatabase
type ShardingDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Shard   map[string]string `json:"shards,omitempty"`
	Catalog map[string]string `json:"catalogs,omitempty"`

	Gsm GsmStatus `json:"gsm,omitempty"`
	Dg  DgStatus  `json:"dg,omitempty"`
	// TDEKeyRefresh tracks manual token-triggered key refresh progress.
	TDEKeyRefresh *TDEKeyRefreshStatus `json:"tdeKeyRefresh,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	CrdStatus []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// TDEKeyRefreshStatus tracks controller progress for manual TDE key refresh requests.
type TDEKeyRefreshStatus struct {
	RequestedToken     string   `json:"requestedToken,omitempty"`
	Phase              string   `json:"phase,omitempty"` // Pending | Running | Failed | Succeeded
	Exported           bool     `json:"exported,omitempty"`
	CurrentShard       string   `json:"currentShard,omitempty"`
	CompletedShards    []string `json:"completedShards,omitempty"`
	FailedShards       []string `json:"failedShards,omitempty"`
	LastError          string   `json:"lastError,omitempty"`
	UserActionRequired bool     `json:"userActionRequired,omitempty"`
}

// DG status for sharding (tracks broker enable/config per standby shard)
type DgStatus struct {
	// Overall DG state for the topology: PENDING / ENABLED / ERROR
	State string `json:"state,omitempty"`

	// Per-standby shard broker status: shardName -> "true" / "pending" / "error:<msg>"
	Broker map[string]string `json:"broker,omitempty"`

	// Optional extra details
	Details map[string]string `json:"details,omitempty"`

	// Optional standby mapping details: primary -> standby pair status.
	Pairs []DgPairStatus `json:"pairs,omitempty"`
}

type DgPairStatus struct {
	PrimaryKey           string `json:"primaryKey,omitempty"`
	PrimarySource        string `json:"primarySource,omitempty"` // PrimaryDatabaseRef|ConnectString|Endpoint
	PrimaryConnectString string `json:"primaryConnectString,omitempty"`
	StandbyShardName     string `json:"standbyShardName,omitempty"`
	StandbyShardNum      int32  `json:"standbyShardNum,omitempty"`
	State                string `json:"state,omitempty"`
	Message              string `json:"message,omitempty"`
}

type GsmStatus struct {
	InternalconnectStr string            `json:"internalConnectStr,omitempty"`
	ExternalConnectStr string            `json:"externalConnectStr,omitempty"`
	State              string            `json:"state,omitempty"`
	Shards             map[string]string `json:"shards,omitempty"`
	Details            map[string]string `json:"details,omitempty"`
	Services           string            `json:"services,omitempty"`
}

type GsmShardDetails struct {
	Name      string `json:"name,omitempty"`
	Available string `json:"available,omitempty"`
	State     string `json:"State,omitempty"`
}

type GsmStatusDetails struct {
	Name             string `json:"name,omitempty"`
	K8sInternalSvc   string `json:"k8sInternalSvc,omitempty"`
	K8sExternalSvc   string `json:"k8sExternalSvc,omitempty"`
	K8sInternalSvcIP string `json:"k8sInternalIP,omitempty"`
	K8sExternalSvcIP string `json:"k8sExternalIP,omitempty"`
	Role             string `json:"role,omitempty"`
	DbPasswordSecret string `json:"dbPasswordSecret"`
}

type OperationStatus struct {
	TDEExported bool              `json:"tdeExported,omitempty"`
	TDEImported map[string]bool   `json:"tdeImported,omitempty"` // shard -> done
	DGPhase     map[string]string `json:"dgPhase,omitempty"`     // shard -> pending/enabled/error
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.gsm.state",name="Gsm State",type=string
//+kubebuilder:printcolumn:JSONPath=".status.gsm.services",name="Services",type=string
//+kubebuilder:printcolumn:JSONPath=".status.gsm.shards",name="shards",type=string,priority=1
//+kubebuilder:printcolumn:name="DG",type=string,JSONPath=".status.dg.state",priority=0
//+kubebuilder:printcolumn:name="PrimarySID",type=string,JSONPath=".status.primary.sid",priority=0
//+kubebuilder:printcolumn:name="PrimaryRole",type=string,JSONPath=".status.primary.role",priority=0
//+kubebuilder:printcolumn:name="PrimaryMode",type=string,JSONPath=".status.primary.openMode",priority=1
//+kubebuilder:printcolumn:name="PrimarySvc",type=string,JSONPath=".status.primary.serviceName",priority=1
//+kubebuilder:printcolumn:name="PrimaryIP",type=string,JSONPath=".status.primary.podIP",priority=1
//+kubebuilder:printcolumn:name="StandbySID",type=string,JSONPath=".status.standby.sid",priority=1
//+kubebuilder:printcolumn:name="StandbyMode",type=string,JSONPath=".status.standby.openMode",priority=1

// ShardingDatabase is the Schema for the shardingdatabases API
// +kubebuilder:resource:path=shardingdatabases,scope=Namespaced,shortName=gdd
// +kubebuilder:storageversion
type ShardingDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShardingDatabaseSpec   `json:"spec,omitempty"`
	Status ShardingDatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// ShardingDatabaseList contains a list of ShardingDatabase
// +kubebuilder:storageversion
type ShardingDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShardingDatabase `json:"items"`
}

// ShardSpec is a specification of Shards for an application deployment.
type ShardSpec struct {
	// Existing fields
	Name string `json:"name"` // Shard name used for StatefulSet
	// +kubebuilder:default:=50
	StorageSizeInGb int32                        `json:"storageSizeInGb,omitempty"`                                 // Optional shard storage size (GB)
	EnvVars         []EnvironmentVariable        `json:"envVars,omitempty"`                                         // Optional env variables for shard
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirements
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvcName string `json:"pvcName,omitempty"` // Optional PVC name
	Label   string `json:"label,omitempty"`   // Optional label
	// +kubebuilder:validation:Enum=enable;disable;failed;force
	IsDelete     string            `json:"isDelete,omitempty"`     // Deletion flag
	NodeSelector map[string]string `json:"nodeSelector,omitempty"` // Node selector for scheduling
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvAnnotations map[string]string `json:"pvAnnotations,omitempty"` // Annotations for PV
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvMatchLabels    map[string]string  `json:"pvMatchLabels,omitempty"` // Match labels for PV selector
	ImagePulllPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	ShardSpace       string             `json:"shardSpace,omitempty"`  // Shardspace name
	ShardGroup       string             `json:"shardGroup,omitempty"`  // Shardgroup name
	ShardRegion      string             `json:"shardRegion,omitempty"` // Region name
	// +kubebuilder:validation:Enum=PRIMARY;STANDBY;ACTIVE_STANDBY
	DeployAs        string         `json:"deployAs,omitempty"`        // Role for DG (PRIMARY/STANDBY/ACTIVE_STANDBY)
	ShardConfigData *ConfigMapData `json:"shardConfigData,omitempty"` // Extra config via ConfigMap

	// GDD / ADD SHARD specific fields (GDSCTL help)
	CdbName      string `json:"cdbName,omitempty"`      // CDB name (cdb)
	CloneSchemas bool   `json:"cloneSchemas,omitempty"` // Clone schemas to shard (clone_schemas)
	// +kubebuilder:validation:Minimum=1
	CpuThreshold int32 `json:"cpuThreshold,omitempty"` // CPU utilization threshold
	// +kubebuilder:validation:Minimum=1
	DiskThreshold                 int32                      `json:"diskThreshold,omitempty"`   // Disk latency threshold (ms)
	Force                         bool                       `json:"force,omitempty"`           // Force replace existing GDS config
	Rack                          string                     `json:"rack,omitempty"`            // Rack / availability group
	ValidateNetwork               bool                       `json:"validateNetwork,omitempty"` // Enable network validation
	GgService                     string                     `json:"ggService,omitempty"`       // GoldenGate Admin Server URI
	Replace                       string                     `json:"replace,omitempty"`         // Replaced database name
	Pwd                           string                     `json:"pwd,omitempty"`             // GSM user password
	SaveName                      bool                       `json:"savename,omitempty"`        // Store net service name instead of descriptor
	Connect                       string                     `json:"connect,omitempty"`         // Connect identifier / net service name
	AdditionalPVCs                []AdditionalPVCSpec        `json:"additionalPVCs,omitempty"`
	DisableDefaultLogVolumeClaims bool                       `json:"disableDefaultLogVolumeClaims,omitempty"`
	ServiceAnnotations            map[string]string          `json:"serviceAnnotations,omitempty"`
	ExternalServiceAnnotations    map[string]string          `json:"externalServiceAnnotations,omitempty"`
	SecurityContext               *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Capabilities                  *corev1.Capabilities       `json:"capabilities,omitempty"`
	// Extra shard fields for GDD-style sharding
	PdbPreFix            string         `json:"pdbPreFix,omitempty"`
	ReadinessCheckPeriod int            `json:"readinessCheckPeriod,omitempty"`
	LivenessCheckPeriod  int            `json:"livenessCheckPeriod,omitempty"`
	PrimaryDatabaseRef   *DatabaseRef   `json:"primaryDatabaseRef,omitempty"`
	StandbyConfig        *StandbyConfig `json:"standbyConfig,omitempty"`
}

// CatalogSpec defines the desired state of CatalogSpec
type CatalogSpec struct {
	// Core K8s / deployment fields
	Name    string `json:"name"`              // Catalog name used for StatefulSet
	PdbName string `json:"pdbName,omitempty"` // PDB name for catalog
	// +kubebuilder:default:=50
	StorageSizeInGb int32                        `json:"storageSizeInGb,omitempty"`                                 // Catalog storage size (GB)
	Shape           string                       `json:"shape,omitempty"`                                           // DB shape / flavor
	EnvVars         []EnvironmentVariable        `json:"envVars,omitempty"`                                         // Optional env variables
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirements
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvcName      string            `json:"pvcName,omitempty"`      // Optional PVC name
	Label        string            `json:"label,omitempty"`        // Optional label
	IsDelete     string            `json:"isDelete,omitempty"`     // Deletion flag
	NodeSelector map[string]string `json:"nodeSelector,omitempty"` // Node selector for scheduling
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvAnnotations map[string]string `json:"pvAnnotations,omitempty"` // Annotations for PV
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvMatchLabels     map[string]string  `json:"pvMatchLabels,omitempty"` // Match labels for PV selector
	ImagePulllPolicy  *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	CatalogConfigData *ConfigMapData     `json:"catalogConfigData,omitempty"` // Extra catalog configuration

	// GDSCTL CREATE SHARDCATALOG-related fields
	Region     []string `json:"region,omitempty"`     // List of region names
	ConfigName string   `json:"configname,omitempty"` // GDS configuration name (configname)
	// +kubebuilder:validation:Enum=ON;OFF;on;off
	AutoVncr string `json:"autoVncr,omitempty"` // AUTOVNCR ON/OFF
	Force    bool   `json:"force,omitempty"`    // Force overwrite existing GDS config
	GdsPool  string `json:"gdsPool,omitempty"`  // Optional GDS pool name (for docs / usage)
	// Sharding / replication behavior
	// +kubebuilder:validation:Enum=DG;NATIVE;dg;native
	Repl string `json:"repl,omitempty"` // Replication technology: DG | NATIVE
	// +kubebuilder:validation:Enum=SYSTEM;USER;COMPOSITE;system;user;composite
	Sharding string `json:"sharding,omitempty"` // Sharding type: SYSTEM | USER | COMPOSITE
	// +kubebuilder:validation:Minimum=1
	RepFactor int32 `json:"repFactor,omitempty"` // Default replication factor
	// +kubebuilder:validation:Minimum=1
	Chunks int32 `json:"chunks,omitempty"` // Default number of chunks
	// +kubebuilder:validation:Enum=MAXPROTECTION;MAXAVAILABILITY;MAXPERFORMANCE
	ProtectMode string `json:"protectMode,omitempty"` // Default Data Guard protection mode
	// +kubebuilder:validation:Minimum=1
	RepUnits      int32  `json:"repUnits,omitempty"`      // Total replication units (SNR)
	AgentPassword string `json:"agentPassword,omitempty"` // Remote agent registration password
	// +kubebuilder:validation:Minimum=1
	AgentPort       int32  `json:"agentPort,omitempty"`            // XDB/agent port
	ValidateNetwork bool   `json:"validateNetwork,omitempty"`      // Enable network validation for catalog operations
	MultiWriter     bool   `json:"multiwriter,omitempty"`          // Enable multi-writer native replication
	ForFederated    bool   `json:"forFederatedDatabase,omitempty"` // Federated database catalog
	Encryption      string `json:"encryption,omitempty"`           // ANO encryption: AES256 | AES192 | OFF

	// Source SDB (if applicable)
	Sdb string `json:"sdb,omitempty"` // Source sharded database name

	// Database references
	PrimaryDatabaseRef *DatabaseRef `json:"primaryDatabaseRef,omitempty"` // Where to create catalog / primary
	CatalogDatabaseRef *DatabaseRef `json:"catalogDatabaseRef,omitempty"` // Existing catalog DB (reuse)

	// Control flags
	UseExistingCatalog            bool                       `json:"useExistingCatalog,omitempty"` // true to reuse existing catalog
	CreateAs                      string                     `json:"createAs,omitempty"`           // CreateAs mode (if you define semantics)
	AdditionalPVCs                []AdditionalPVCSpec        `json:"additionalPVCs,omitempty"`
	DisableDefaultLogVolumeClaims bool                       `json:"disableDefaultLogVolumeClaims,omitempty"`
	ServiceAnnotations            map[string]string          `json:"serviceAnnotations,omitempty"`
	ExternalServiceAnnotations    map[string]string          `json:"externalServiceAnnotations,omitempty"`
	SecurityContext               *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Capabilities                  *corev1.Capabilities       `json:"capabilities,omitempty"`
}

// GsmSpec defines the desired state of GSM (ADD GSM + deployment info).
type GsmSpec struct {
	Name string `json:"name"` // GSM name used for StatefulSet and GDSCTL

	// Core K8s / deployment fields
	EnvVars []EnvironmentVariable `json:"envVars,omitempty"` // Optional env variables
	// Deprecated: retained for backward compatibility with legacy manifests.
	Replicas int32 `json:"replicas,omitempty"`
	// +kubebuilder:default:=50
	StorageSizeInGb int32                        `json:"storageSizeInGb,omitempty"`                                 // GSM storage size (GB) if PVC is used
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirements
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvcName      string            `json:"pvcName,omitempty"`      // Optional PVC name
	Label        string            `json:"label,omitempty"`        // Optional label
	IsDelete     string            `json:"isDelete,omitempty"`     // Deletion flag
	NodeSelector map[string]string `json:"nodeSelector,omitempty"` // Node selector for scheduling
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvAnnotations map[string]string `json:"pvAnnotations,omitempty"` // Annotations for PV
	// Deprecated: no longer used by the operator. Use additionalPVCs instead.
	PvMatchLabels    map[string]string  `json:"pvMatchLabels,omitempty"` // Match labels for PV selector
	ImagePulllPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Region           string             `json:"region,omitempty"`       // Single region (ADD GSM -region)
	DirectorName     string             `json:"directorName,omitempty"` // Optional director name
	GsmConfigData    *ConfigMapData     `json:"gsmConfigData,omitempty"`

	// Logical GSM fields (from  YAML)

	GsmNum         int32    `json:"gsmNum,omitempty"`         // Number of GSMs to deploy (if used)
	GsmPrefix      string   `json:"gsmPrefix,omitempty"`      // Prefix for GSM names
	Shape          string   `json:"shape,omitempty"`          // DB/GSM compute shape
	Regions        []string `json:"regions,omitempty"`        // Regions to which GSM belongs (for distributon)
	RemoteOns      int32    `json:"remoteOns,omitempty"`      // remoteons: remote ONS port
	LocalOns       int32    `json:"localons,omitempty"`       // localons: local ONS port
	Listener       int32    `json:"listener,omitempty"`       // Listener port (default 1522)
	Endpoint       string   `json:"endpoint,omitempty"`       // endpoint: GSM endpoint
	RemoteEndpoint string   `json:"remoteEndpoint,omitempty"` // remote_endpoint: remote listener endpoint
	TraceLevel     string   `json:"traceLevel,omitempty"`     // trace_level: diagnostic level
	Encryption     string   `json:"encryption,omitempty"`     // encryption: AES256 | AES192 | OFF

	// Catalog / credentials bindings (Add GSM)

	Catalog                       string                     `json:"catalog,omitempty"` // GDS catalog connect string (TNS alias)
	Pwd                           string                     `json:"pwd,omitempty"`     // GSMCATUSER password
	WalletPassword                string                     `json:"wpwd,omitempty"`    // Wallet password
	AdditionalPVCs                []AdditionalPVCSpec        `json:"additionalPVCs,omitempty"`
	DisableDefaultLogVolumeClaims bool                       `json:"disableDefaultLogVolumeClaims,omitempty"`
	ServiceAnnotations            map[string]string          `json:"serviceAnnotations,omitempty"`
	ExternalServiceAnnotations    map[string]string          `json:"externalServiceAnnotations,omitempty"`
	SecurityContext               *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Capabilities                  *corev1.Capabilities       `json:"capabilities,omitempty"`
}

// GsmInfo wraps common GSM defaults and itemized GSM entries.
// When specified, it is used to materialize effective spec.gsm entries.
type GsmInfo struct {
	Gsm                        []GsmSpec                    `json:"gsm,omitempty"`
	Resources                  *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	StorageSizeInGb            int32                        `json:"storageSizeInGb,omitempty"`
	EnvVars                    []EnvironmentVariable        `json:"envVars,omitempty"`
	ImagePulllPolicy           *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ServiceAnnotations         map[string]string            `json:"serviceAnnotations,omitempty"`
	ExternalServiceAnnotations map[string]string            `json:"externalServiceAnnotations,omitempty"`
}

// ShardGroupSpec is used both for top-level shardGroup[] and inside shardInfo.shardGroupDetails.
type ShardGroupSpec struct {
	Name string `json:"name"` // Name of the shardgroup
	// Deprecated: use name.
	ShardGroupName string `json:"shardGroupName,omitempty"`
	Region         string `json:"region,omitempty"`     // Region for this shardgroup
	ShardSpace     string `json:"shardSpace,omitempty"` // Associated shardspace name
	// Deprecated: use shardSpace.
	LegacyShardSpace string `json:"ShardSpace,omitempty"`
	// +kubebuilder:validation:Enum=PRIMARY;STANDBY;ACTIVE_STANDBY
	DeployAs string `json:"deployAs,omitempty"` // Deployment role (DG)
	// +kubebuilder:validation:Minimum=1
	RepFactor int32 `json:"repFactor,omitempty"` // Replication factor (NATIVE replication)
	// +kubebuilder:validation:Enum=READWRITE;READONLY
	RuMode   string `json:"ru_mode,omitempty"`  // SNR shardgroup mode
	IsDelete string `json:"isDelete,omitempty"` // Optional delete flag for managed replicas
}

// ShardSpaceSpec is used both for top-level shardSpace[] and inside shardInfo.shardSpaceDetails.
type ShardSpaceSpec struct {
	Name string `json:"name"` // Name of the shardspace
	// Deprecated: use name.
	ShardSpaceName string `json:"shardSpaceName,omitempty"`
	// +kubebuilder:validation:Enum=PRIMARY;STANDBY;ACTIVE_STANDBY
	DeployAs string `json:"deployAs,omitempty"` // Deployment role for USER sharding shardInfo
	// +kubebuilder:validation:Minimum=1
	Chunks int32 `json:"chunks,omitempty"` // Number of chunks in the shardspace
	// Deprecated: use chunks. Retained for compatibility with legacy typo key.
	Chnuks int32 `json:"Chnuks,omitempty"`
	// +kubebuilder:validation:Enum=MAXPROTECTION;MAXAVAILABILITY;MAXPERFORMANCE
	ProtectMode string `json:"protectMode,omitempty"` // Data Guard protection mode
	// Deprecated: use protectMode.
	ProtectionMode string `json:"protectionMode,omitempty"`
	// +kubebuilder:validation:Minimum=1
	RepFactor int32 `json:"repFactor,omitempty"` // Replication factor (NATIVE replication)
	// +kubebuilder:validation:Minimum=1
	RepUnits int32 `json:"repUnits,omitempty"` // Replication units (SNR only)
}

// RegionSpec (new, for top-level region[])
type RegionSpec struct {
	Name  string `json:"name,omitempty"`
	Buddy string `json:"buddy,omitempty"`
}

// GsmServiceSpec defines a global service (ADD SERVICE).
type GsmServiceSpec struct {
	Name string `json:"name"` // Global service name

	// Pool & placement
	GdsPool      string `json:"gdsPool,omitempty"`      // gdspool: GDS pool name
	Preferred    string `json:"preferred,omitempty"`    // preferred: comma-delimited preferred DB list
	PreferredAll string `json:"preferredAll,omitempty"` // preferred_all: all DBs in pool preferred
	// Deprecated: use preferredAll. Retained for backward compatibility with legacy typo key.
	PrferredAll  string `json:"prferredAll,omitempty"`
	Available    string `json:"available,omitempty"`    // available: comma-delimited available DB list
	Locality     string `json:"locality,omitempty"`     // locality: ANYWHERE | LOCAL_ONLY
	Role         string `json:"role,omitempty"`         // role: DB role for service (PRIMARY/PHYSICAL_STANDBY/etc.)
	Lag          int    `json:"lag,omitempty"`          // lag: seconds (ANY can be handled specially in code)
	Notification string `json:"notification,omitempty"` // notification: TRUE | FALSE (AQ HA notifications)

	// Load balancing & policies
	RlbGoal              string `json:"rlbGoal,omitempty"`               // rlbgoal: SERVICE_TIME | THROUGHPUT
	ClbGoal              string `json:"clbGoal,omitempty"`               // clbgoal: SHORT | LONG
	Dtp                  string `json:"dtp,omitempty"`                   // dtp: TRUE | FALSE (distributed transaction policy)
	SqlTrasactionProfile string `json:"sqlTransactionProfile,omitempty"` // sql_translation_profile (kept old name but tag aligned)
	TafPolicy            string `json:"tafPolicy,omitempty"`             // tafpolicy: BASIC | NONE | PRECONNECT
	Policy               string `json:"policy,omitempty"`                // policy: AUTOMATIC | MANUAL
	FailoverType         string `json:"failoverType,omitempty"`          // failovertype: NONE | SESSION | SELECT | TRANSACTION | AUTO
	FailoverMethod       string `json:"failoverMethod,omitempty"`        // failovermethod: NONE | BASIC
	FailoverRetry        string `json:"failoverRetry,omitempty"`         // failoverretry: number of retries
	FailoverDelay        string `json:"failoverDelay,omitempty"`         // failoverdelay: delay between retries
	FailoverPrimary      string `json:"failoverPrimary,omitempty"`       // failover_primary: enable PHYSICAL_STANDBY failover
	Edition              string `json:"edition,omitempty"`               // edition: database edition
	CommitOutcome        string `json:"commitOutcome,omitempty"`         // commit_outcome: TRUE | FALSE
	Retention            string `json:"retention,omitempty"`             // retention: retention time in seconds
	SessionState         string `json:"sessionState,omitempty"`          // session_state: STATIC | DYNAMIC | AUTO
	ReplayInitTime       string `json:"replayInitTime,omitempty"`        // replay_init_time: in seconds
	PdbName              string `json:"pdbName,omitempty"`               // pdbname: pluggable DB name
	DrainTimeout         string `json:"drainTimeout,omitempty"`          // drain_timeout: drain time in seconds
	StopOption           string `json:"stopOption,omitempty"`            // stop_option: NONE | IMMEDIATE | TRANSACTIONAL
	FailoverRestore      string `json:"failoverRestore,omitempty"`       // failover_restore: NONE | LEVEL1 | AUTO
	TableFamily          string `json:"tableFamily,omitempty"`           // table_family: <schema>.<root_table>
	ResetState           string `json:"resetState,omitempty"`            // reset_state: NONE | LEVEL1 | LEVEL2 | AUTO
	RegionFailover       string `json:"regionFailover,omitempty"`        // region_failover: enable region failover (LOCAL_ONLY)

	// RU mode for SNR
	RuMode            string `json:"ruMode,omitempty"`            // ru_mode: ANY | READWRITE | READONLY
	FailoverReadWrite string `json:"failoverReadwrite,omitempty"` // failover_readwrite: ON | OFF (SNR read-only service → RW shardgroup)

	// Legacy / keep for backward compatibility
	TfaPolicy string `json:"tfaPolicy,omitempty"` // Existing field from older spec (can be used for advanced failover/logging policies)
}

// Secret Details
type SecretDetails struct {
	Name      string `json:"name"`                // Name of the secret mounted into sharding pods.
	MountPath string `json:"mountPath,omitempty"` // Optional mount path for the secret volume.
	// Deprecated: retained for backward compatibility with legacy secret schema.
	KeyFileName string `json:"keyFileName,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	NsConfigMap string `json:"nsConfigMap,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	NsSecret string `json:"nsSecret,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	PwdFileName string `json:"pwdFileName,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	PwdFileMountLocation string `json:"pwdFileMountLocation,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	KeyFileMountLocation string `json:"keyFileMountLocation,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	KeySecretName string `json:"keySecretName,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	EncryptionType string `json:"encryptionType,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	TdeKeyFileName string `json:"tdeKeyFileName,omitempty"`
	// Deprecated: retained for backward compatibility with legacy secret schema.
	TdePwdFileName string `json:"tdePwdFileName,omitempty"`
	// +kubebuilder:default:="true"
	UseGsmWallet  string                `json:"useGsmWallet,omitempty"`  // String flag ("true"/"false") controlling GSM wallet usage.
	GsmWalletRoot string                `json:"gsmWalletRoot,omitempty"` // Wallet root used for GSM when useGsmWallet is true.
	DbAdmin       PasswordSecretConfig  `json:"dbAdmin"`
	TDE           *PasswordSecretConfig `json:"tde,omitempty"`
}

type PasswordSecretConfig struct {
	PasswordKey   string `json:"passwordKey"`             // Secret key containing password/ciphertext payload
	PrivateKeyKey string `json:"privateKeyKey,omitempty"` // Secret key containing private key, required for openssl-rsa-oaep
	Pkeyopt       string `json:"pkeyopt,omitempty"`       // OpenSSL pkeyutl options, semicolon-separated
}

type AdditionalPVCSpec struct {
	MountPath       string `json:"mountPath"`
	PvcName         string `json:"pvcName,omitempty"`
	StorageSizeInGb int32  `json:"storageSizeInGb,omitempty"`
	StorageClass    string `json:"storageClass,omitempty"`
}

const (
	DefaultPkeyopt          = "rsa_padding_mode:oaep;rsa_oaep_md:sha256;rsa_mgf1_md:sha256"
	DefaultSecretMountPath  = "/mnt/secrets"
	DefaultGsmWalletRoot    = "/opt/oracle/gsmdata/walletroot"
	DefaultOraDataMountPath = "/opt/oracle/oradata"
	DefaultGsmDataMountPath = "/opt/oracle/gsmdata"
	DefaultDiagMountPath    = "/opt/oracle/diag"
	DefaultGsmDiagMountPath = "/u01/app/oracle/diag"
	DefaultGddLogMountPath  = "/var/log/gdd"
	DefaultDiagSizeInGb     = int32(50)
	DefaultGddLogSizeInGb   = int32(10)
)

// DatabaseRef is used in catalog/shard primaryDatabaseRef, catalogDatabaseRef
type DatabaseRef struct {
	CdbName string `json:"cdbName,omitempty"` // CDB name
	PdbName string `json:"pdbName,omitempty"` // PDB name
	Host    string `json:"host,omitempty"`    // Hostname or IP
	Port    int32  `json:"port,omitempty"`    // Listener port
}

// PrimaryDatabaseCRRef identifies a primary shard/database object in Kubernetes.
type PrimaryDatabaseCRRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// PrimaryEndpointRef identifies a primary by external endpoint metadata.
type PrimaryEndpointRef struct {
	ConnectString string `json:"connectString,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int32  `json:"port,omitempty"`
	CdbName       string `json:"cdbName,omitempty"`
	PdbName       string `json:"pdbName,omitempty"`
}

// TDEWalletConfig provides common wallet configuration for both global and standby-specific flows.
type TDEWalletConfig struct {
	// +kubebuilder:validation:Enum=enable;disable
	IsEnabled string `json:"isEnabled,omitempty"`
	PVCName   string `json:"pvcName,omitempty"`
	MountPath string `json:"mountPath,omitempty"`

	SecretRef  string `json:"secretRef,omitempty"`
	ZipFileKey string `json:"zipFileKey,omitempty"`
	WalletRoot string `json:"walletRoot,omitempty"`
}

// StandbyConfig defines how standby shards should be mapped from primary inputs.
type StandbyConfig struct {
	// +kubebuilder:validation:Minimum=1
	StandbyPerPrimary int32 `json:"standbyPerPrimary,omitempty"`
	// +kubebuilder:validation:Enum=MAXPROTECTION;MAXAVAILABILITY;MAXPERFORMANCE
	ProtectionMode string `json:"protectionMode,omitempty"`
	// +kubebuilder:validation:Enum=SYNC;ASYNC
	TransportMode string `json:"transportMode,omitempty"`

	PrimaryDatabaseRefs   []PrimaryDatabaseCRRef `json:"primaryDatabaseRefs,omitempty"`
	PrimaryConnectStrings []string               `json:"primaryConnectStrings,omitempty"`
	PrimaryEndpoints      []PrimaryEndpointRef   `json:"primaryEndpoints,omitempty"`
	TDEWallet             *TDEWalletConfig       `json:"tdeWallet,omitempty"`
}

// EnvironmentVariable represents a named variable accessible for containers.
type EnvironmentVariable struct {
	Name  string `json:"name"`  // Name of the variable. Must be a C_IDENTIFIER.
	Value string `json:"value"` // Value of the variable, as defined in Kubernetes core API.
}

// PortMapping is a specification of port mapping for an application deployment.
type PortMapping struct {
	Port       int32           `json:"port"`       // Port that will be exposed on the service.
	TargetPort int32           `json:"targetPort"` // Docker image port for the application.
	Protocol   corev1.Protocol `json:"protocol"`   // IP protocol for the mapping, e.g., "TCP" or "UDP".
}

type SfsetLabel string

const (
	ShardingDelLabelKey        SfsetLabel = "sharding.oracle.com/delflag"
	ShardingDelLabelTrueValue  SfsetLabel = "true"
	ShardingDelLabelFalseValue SfsetLabel = "false"
)

type ConfigMapData struct {
	Name      string `json:"name,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

// EffectiveTDEWalletMountPath returns the current TDE wallet mount path and
// falls back to deprecated fields for backward compatibility.
func (s *ShardingDatabaseSpec) EffectiveTDEWalletMountPath(defaultPath string) string {
	if s == nil {
		return defaultPath
	}
	if s.TDEWallet != nil {
		if mount := strings.TrimSpace(s.TDEWallet.MountPath); mount != "" {
			return mount
		}
	}
	//lint:ignore SA1019 legacy fallback for backward compatibility
	if mount := strings.TrimSpace(s.TdeWalletPvcMountLocation); mount != "" {
		return mount
	}
	return defaultPath
}

// EffectiveTDEWalletPVCName returns the current TDE wallet PVC name and falls
// back to deprecated fields for backward compatibility.
func (s *ShardingDatabaseSpec) EffectiveTDEWalletPVCName() string {
	if s == nil {
		return ""
	}
	if s.TDEWallet != nil {
		if pvc := strings.TrimSpace(s.TDEWallet.PVCName); pvc != "" {
			return pvc
		}
	}
	//lint:ignore SA1019 legacy fallback for backward compatibility
	return strings.TrimSpace(s.TdeWalletPvc)
}

// EffectiveTDEWalletEnabled returns the current TDE wallet mode and falls back
// to deprecated fields for backward compatibility.
func (s *ShardingDatabaseSpec) EffectiveTDEWalletEnabled() string {
	if s == nil {
		return ""
	}
	if s.TDEWallet != nil {
		if mode := strings.TrimSpace(s.TDEWallet.IsEnabled); mode != "" {
			return mode
		}
	}
	//lint:ignore SA1019 legacy fallback for backward compatibility
	return strings.TrimSpace(s.IsTdeWallet)
}

// Shard structures based on managed Replicas
type ShardingDetails struct {
	ShardPreFixName string `json:"shardPreFixName"`
	Shape           string `json:"shape,omitempty"`
	// +kubebuilder:validation:Minimum=1
	ShardNum int32 `json:"shardNum,omitempty"`
	// Deprecated: use shardNum. Kept for backward compatibility.
	Replicas int32 `json:"replicas,omitempty"`

	// +kubebuilder:default:=50
	StorageSizeInGb               int32                        `json:"storageSizeInGb,omitempty"`
	ShardGroupDetails             *ShardGroupSpec              `json:"shardGroupDetails,omitempty"`
	ShardSpaceDetails             *ShardSpaceSpec              `json:"shardSpaceDetails,omitempty"`
	PrimaryDatabaseRef            *DatabaseRef                 `json:"primaryDatabaseRef,omitempty"`
	StandbyConfig                 *StandbyConfig               `json:"standbyConfig,omitempty"`
	EnvVars                       []EnvironmentVariable        `json:"envVars,omitempty"`
	Resources                     *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	AdditionalPVCs                []AdditionalPVCSpec          `json:"additionalPVCs,omitempty"`
	DisableDefaultLogVolumeClaims bool                         `json:"disableDefaultLogVolumeClaims,omitempty"`
	ServiceAnnotations            map[string]string            `json:"serviceAnnotations,omitempty"`
	ExternalServiceAnnotations    map[string]string            `json:"externalServiceAnnotations,omitempty"`
	SecurityContext               *corev1.PodSecurityContext   `json:"securityContext,omitempty"`
	Capabilities                  *corev1.Capabilities         `json:"capabilities,omitempty"`
}

type ShardStatusMapKeys string

const (
	Name             ShardStatusMapKeys = "Name"
	K8sInternalSvc   ShardStatusMapKeys = "K8sInternalSvc"
	K8sExternalSvc   ShardStatusMapKeys = "K8sExternalSvc"
	K8sInternalSvcIP ShardStatusMapKeys = "K8sInternalSvcIP"
	K8sExternalSvcIP ShardStatusMapKeys = "K8sExternalSvcIP"
	OracleSid        ShardStatusMapKeys = "OracleSid"
	OraclePdb        ShardStatusMapKeys = "OraclePdb"
	Role             ShardStatusMapKeys = "Role"
	DbPasswordSecret ShardStatusMapKeys = "DbPasswordSecret"
	State            ShardStatusMapKeys = "State"
	OpenMode         ShardStatusMapKeys = "OpenMode"
)

type ShardLifecycleState string

const (
	AvailableState        ShardLifecycleState = "AVAILABLE"
	FailedState           ShardLifecycleState = "FAILED"
	UpdateState           ShardLifecycleState = "UPDATING"
	ProvisionState        ShardLifecycleState = "PROVISIONING"
	PodNotReadyState      ShardLifecycleState = "PODNOTREADY"
	PodFailureState       ShardLifecycleState = "PODFAILURE"
	PodNotFound           ShardLifecycleState = "PODNOTFOUND"
	StatefulSetFailure    ShardLifecycleState = "STATEFULSETFAILURE"
	StatefulSetNotFound   ShardLifecycleState = "STATEFULSETNOTFOUND"
	DeletingState         ShardLifecycleState = "DELETING"
	DeleteErrorState      ShardLifecycleState = "DELETE_ERROR"
	ChunkMoveError        ShardLifecycleState = "CHUNK_MOVE_ERROR_IN_GSM"
	Terminated            ShardLifecycleState = "TERMINATED"
	LabelPatchingError    ShardLifecycleState = "LABELPATCHINGERROR"
	DeletePVCError        ShardLifecycleState = "DELETEPVCERROR"
	AddingShardState      ShardLifecycleState = "SHARD_ADDITION"
	AddingShardErrorState ShardLifecycleState = "SHARD_ADDITION_ERROR_IN_GSM"
	ShardOnlineErrorState ShardLifecycleState = "SHARD_ONLINE_ERROR_IN_GSM"
	ShardOnlineState      ShardLifecycleState = "ONLINE_SHARD"
	ShardRemoveError      ShardLifecycleState = "SHARD_DELETE_ERROR_FROM_GSM"
)

// var
var KubeConfigOnce sync.Once

// GetLastSuccessfulSpec returns spec from the lass successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulSpec.
func (shardingv1 *ShardingDatabase) GetLastSuccessfulSpec() (*ShardingDatabaseSpec, error) {
	val, ok := shardingv1.GetAnnotations()[lastSuccessfulSpec]
	if !ok {
		return nil, nil
	}

	specBytes := []byte(val)
	sucSpec := ShardingDatabaseSpec{}

	err := json.Unmarshal(specBytes, &sucSpec)
	if err != nil {
		return nil, err
	}

	return &sucSpec, nil
}

// UpdateLastSuccessfulSpec updates lastSuccessfulSpec with the current spec.
func (shardingv1 *ShardingDatabase) UpdateLastSuccessfulSpec(kubeClient client.Client) error {
	specBytes, err := json.Marshal(shardingv1.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		lastSuccessfulSpec: string(specBytes),
	}

	return annsv1.PatchAnnotations(kubeClient, shardingv1, anns)
}
func init() {
	SchemeBuilder.Register(&ShardingDatabase{}, &ShardingDatabaseList{})
}
