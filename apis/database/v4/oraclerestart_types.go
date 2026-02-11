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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OracleRestartSpec defines the desired state of an Oracle Restart deployment.
// It contains configuration for Oracle instance details, storage, security, and service settings.
// +kubebuilder:object:generate=true
// +kubebuilder:validation:XValidation:rule="self.instDetails != null",message="instDetails is required"
type OracleRestartSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	InstDetails          OracleRestartInstDetailSpec    `json:"instDetails"`
	ConfigParams         *InitParams                    `json:"configParams,omitempty"`
	AsmStorageDetails    []AsmDiskGroupDetails          `json:"asmDiskGroupDetails,omitempty"`
	AsmStorageDetailsOld *AsmDiskDetails                `json:"asmStorageDetails,omitempty"`
	Image                string                         `json:"image,omitempty"`
	ImagePullSecret      string                         `json:"imagePullSecret,omitempty"`
	ScriptsLocation      string                         `json:"scriptsLocation,omitempty"`
	SshKeySecret         *OracleRestartSshSecretDetails `json:"sshKeySecret,omitempty"`
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:validation:default="Always"
	ImagePullPolicy    *corev1.PullPolicy               `json:"imagePullPolicy,omitempty"`
	ReadinessProbe     *corev1.Probe                    `json:"readinessProbe,omitempty"`
	ScriptsGetCmd      string                           `json:"scriptsGetCmd,omitempty"`
	IsDebug            string                           `json:"isDebug,omitempty"`
	SecurityContext    *corev1.PodSecurityContext       `json:"securityContext,omitempty"`
	IsDeleteTopolgy    string                           `json:"isDeleteTopology,omitempty"`
	DbSecret           *OracleRestartDbPwdSecretDetails `json:"dbSecret,omitempty"`
	TdeWalletSecret    *OracleRestartDbPwdSecretDetails `json:"tdeWalletSecret,omitempty"`
	ServiceDetails     ServiceSpec                      `json:"serviceDetails,omitempty"`
	Resources          *corev1.ResourceRequirements     `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	IsFailed           bool                             `json:"isFailed,omitempty"`
	IsManual           bool                             `json:"isManual,omitempty"`
	SrvAccountName     string                           `json:"serviceAccountName,omitempty"`
	DataDgStorageClass string                           `json:"dataDgStorageClass,omitempty"`
	RedoDgStorageClass string                           `json:"redoDgStorageClass,omitempty"`
	RecoDgStorageClass string                           `json:"recoDgStorageClass,omitempty"`
	SwStorageClass     string                           `json:"swDgStorageClass,omitempty"`
	CrsDgStorageClass  string                           `json:"crsDgStorageClass,omitempty"`
	AsmStorageSizeInGb int                              `json:"asmStorageSizeInGb,omitempty"`
	LbService          OracleRestartNodePortSvc         `json:"lbService,omitempty"`
	NodePortSvc        OracleRestartNodePortSvc         `json:"nodePortSvc,omitempty"` // Port mappings for the service that is created. The service is created if
	// +kubebuilder:validation:Enum=enable;disable
	// +kubebuilder:default="enable"
	EnableOns string `json:"enableOns,omitempty"`
}
type AsmDiskDetails struct {
	DisksBySize []DiskBySize `json:"disksBySize,omitempty"`
	AutoUpdate  string       `json:"autoUpdate,omitempty"`
}

// DiskBySize represents a list of disks grouped by size
type DiskBySize struct {
	StorageSizeInGb int      `json:"storageSizeInGb,omitempty"`
	DiskNames       []string `json:"diskNames,omitempty"`
}

// InitParams contains the initialization parameters for Oracle database and Grid Infrastructure setup.
// It specifies CPU, memory allocation, installation paths, response files, software locations,
// database configuration, and patch management settings.
//
// CPU and Memory Configuration:
// - CpuCount: Number of CPUs to allocate
// - SgaSize: System Global Area size
// - PgaSize: Program Global Area size
// - Processes: Maximum number of processes
// - HugePages: Number of huge pages to allocate
//
// Installation Directories:
// - GridHome: Oracle Grid Infrastructure home directory
// - DbHome: Oracle Database home directory
// - GridBase: Grid Infrastructure base directory
// - DbBase: Database base directory
// - Inventory: Oracle inventory directory location
//
// Installation Files and Response Configuration:
// - GridResponseFile: Response file for Grid Infrastructure installation
// - DbResponseFile: Response file for Database installation
// - GridSwZipFile: Grid software zip file path
// - DbSwZipFile: Database software zip file path
// - OPatchSwZipFile: OPatch software zip file path
// - StagingSoftwareLocation: Location where software is staged
//
// Operation and Database Configuration:
// - OpType: Type of operation to perform
// - DbUniqueName: Unique name of the database
// - DbName: Database name
// - PdbName: Pluggable database name
// - DbStorageType: Storage type for database
// - DbCharSet: Database character set
// - DbType: Type of database
// - DbConfigType: Database configuration type
// - EnableArchiveLog: Enable archive logging
//
// Software and Patch Locations:
// - SwMountLocation: Software mount location
// - HostSwStageLocation: Host-level software staging location
// - RuPatchLocation: RU patch location
// - RuFolderName: RU folder name
// - OPatchLocation: OPatch utility location
// - SwStagePvc: Persistent volume claim for software staging
// - SwStagePvcMountLocation: Mount location for software staging PVC
// - OneOffLocation: One-off patch location
// - DbOneOffIds: One-off patch IDs for database
// - GridOneOffIds: One-off patch IDs for Grid Infrastructure
type InitParams struct {
	CpuCount                int          `json:"cpuCount,omitempty"`
	SgaSize                 string       `json:"sgaSize,omitempty"`
	PgaSize                 string       `json:"pgaSize,omitempty"`
	Processes               int          `json:"processes,omitempty"`
	HugePages               int          `json:"hugePages,omitempty"`
	GridHome                string       `json:"gridHome,omitempty"`
	DbHome                  string       `json:"dbHome,omitempty"`
	GridBase                string       `json:"gridBase,omitempty"`
	DbBase                  string       `json:"dbBase,omitempty"`
	Inventory               string       `json:"inventory,omitempty"`
	GridResponseFile        ResponseFile `json:"gridResponseFile,omitempty"`
	DbResponseFile          ResponseFile `json:"dbResponseFile,omitempty"`
	GridSwZipFile           string       `json:"gridSwZipFile,omitempty"`
	DbSwZipFile             string       `json:"dbSwZipFile,omitempty"`
	OPatchSwZipFile         string       `json:"oPatchSwZipFile,omitempty"`
	StagingSoftwareLocation string       `json:"stagingSoftwareLocation,omitempty"`
	OpType                  string       `json:"opType,omitempty"`
	DbUniqueName            string       `json:"dbUniqueName,omitempty"`

	DbName        string `json:"dbName,omitempty"`
	PdbName       string `json:"pdbName,omitempty"`
	DbStorageType string `json:"dbStorageType,omitempty"`

	DbCharSet string `json:"dbCharSet,omitempty"`

	DbType                  string `json:"dbType,omitempty"`
	DbConfigType            string `json:"dbConfigType,omitempty"`
	EnableArchiveLog        string `json:"enableArchiveLog,omitempty"`
	SwMountLocation         string `json:"swMountLocation,omitempty"`
	HostSwStageLocation     string `json:"hostSwStageLocation,omitempty"`
	RuPatchLocation         string `json:"ruPatchLocation,omitempty"`
	RuFolderName            string `json:"ruFolderName,omitempty"`
	OPatchLocation          string `json:"oPatchLocation,omitempty"`
	SwStagePvc              string `json:"swStagePvc,omitempty"`
	SwStagePvcMountLocation string `json:"swStagePvcMountLocation,omitempty"`
	OneOffLocation          string `json:"oneOffLocation,omitempty"`
	DbOneOffIds             string `json:"dbOneOffIds,omitempty"`
	GridOneOffIds           string `json:"gridOneOffIds,omitempty"`
}

// OracleRestartInstDetailSpec defines the specification for an Oracle Restart instance detail.
// It contains configuration parameters for managing Oracle Restart instances including
// storage locations, worker nodes, environment variables, and persistent volume claim settings.
type OracleRestartInstDetailSpec struct {
	Name                 string          `json:"name,omitempty"` // Name of the Oracle Restart Instance
	HostSwLocation       string          `json:"hostSwLocation,omitempty"`
	SwLocStorageSizeInGb int             `json:"swLocStorageSizeInGb,omitempty"`
	WorkerNode           []string        `json:"workerNode,omitempty"`
	EnvVars              []corev1.EnvVar `json:"envVars,omitempty"` //Optional Env variables for Shards
	Label                string          `json:"label,omitempty"`
	IsDelete             string          `json:"isDelete,omitempty"`
	IsForceDelete        string          `json:"isForceDelete,omitempty"`
	EnvFile              string          `json:"envFile,omitempty"`
	// +kubebuilder:validation:default="delete"
	IsKeepPVC string            `json:"isKeepPVC,omitempty"`
	PvcName   map[string]string `json:"pvcName,omitempty"`
}

// Responsefile Name
type ResponseFile struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	Name          string `json:"name,omitempty"`
}

// OracleRestart DB Secret Details
type OracleRestartDbPwdSecretDetails struct {
	Name                 string `json:"name,omitempty"`        // Name of the secret.
	KeyFileName          string `json:"keyFileName,omitempty"` // Name of the key.
	PwdFileName          string `json:"pwdFileName,omitempty"`
	PwdFileMountLocation string `json:"pwdFileMountLocation,omitempty"`
	KeyFileMountLocation string `json:"keyFileMountLocation,omitempty"`
	KeySecretName        string `json:"keySecretName,omitempty"`
	EncryptionType       string `json:"encryptionType,omitempty"`
}

// OracleRestart Ssh secret Details
type OracleRestartSshSecretDetails struct {
	Name              string `json:"name,omitempty"` // Name of the secret.
	KeyMountLocation  string `json:"keyMountLocation,omitempty"`
	PrivKeySecretName string `json:"privKeySecretName,omitempty"`
	PubKeySecretName  string `json:"pubKeySecretName,omitempty"`
}

// Service Definition
type ServiceSpec struct {
	Name                  string   `json:"name"` // Name of the shardSpace.
	Cardinality           string   `json:"cardinality,omitempty"`
	Preferred             []string `json:"preferred,omitempty"`
	TafPolicy             string   `json:"tafPolicy,omitempty"`
	Available             []string `json:"available,omitempty"`
	Role                  string   `json:"role,omitempty"`
	Notification          string   `json:"notification,omitempty"`
	CommitOutCome         string   `json:"commitOutcome,omitempty"`
	CommitOutComeFastPath string   `json:"commitOutComeFastPath,omitempty"`
	Retention             int      `json:"retenion,omitempty"`
	SessionState          string   `json:"sessionState,omitempty"`
	Pdb                   string   `json:"pdb,omitempty"`
	StopOption            string   `json:"stopOption,omitempty"`
	DrainTimeOut          int      `json:"drainTimeOut,omitempty"`
	FailOverType          string   `json:"failOverType,omitempty"`
	FailOverDelay         int      `json:"failOverDelay,omitempty"`
	FailOverRetry         int      `json:"failOverRetry,omitempty"`
	FailBack              string   `json:"failBack,omitempty"`
	FailOverRestore       string   `json:"failOverRestore,omitempty"`
	ClbGoal               string   `json:"clbGoal,omitempty"`
	RlbGoal               string   `json:"rlbGoal,omitempty"`
	Dtp                   string   `json:"dtp,omitempty"`
	Edition               string   `json:"edition,omitempty"`
	SvcState              string   `json:"svcState,omitempty"`
}

// OracleRestartStatus defines the observed state of OracleRestart
type OracleRestartStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DbName                string                     `json:"DbName,omitempty"`
	ConnectString         string                     `json:"connectString,omitempty"`
	PdbConnectString      string                     `json:"pdbConnectString,omitempty"`
	ExternalConnectString string                     `json:"externalConnectString,omitempty"`
	OracleRestartNodes    []*OracleRestartNodestatus `json:"OracleRestartNodes,omitempty"`
	ReleaseUpdate         string                     `json:"releaseUpdate,omitempty"`
	Role                  string                     `json:"role,omitempty"`
	DbState               string                     `json:"dbState,omitempty"`
	State                 string                     `json:"state,omitempty"`
	InstallNode           string                     `json:"installNode,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions         []metav1.Condition               `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	InstDetails        *OracleRestartInstDetailSpec     `json:"instDetails,omitempty"`
	ConfigParams       *InitParams                      `json:"configParams,omitempty"`
	AsmDiskGroups      []AsmDiskGroupStatus             `json:"asmDiskGroups,omitempty"`
	NfsStorageDetails  *corev1.NFSVolumeSource          `json:"nfsStorageDetails,omitempty"`
	UseNfsforSwStorage string                           `json:"useNfsforSwStorage,omitempty"`
	StorageClass       string                           `json:"storageClass,omitempty"`
	StorageSizeInGB    int                              `json:"storageSizeInGB,omitempty"`
	Image              string                           `json:"image,omitempty"`
	ImagePullSecret    string                           `json:"imagePullSecret,omitempty"`
	ScriptsLocation    string                           `json:"scriptsLocation,omitempty"`
	IsDeleteOraPvc     string                           `json:"isDeleteOraPvc,omitempty"`
	SshKeySecret       *OracleRestartSshSecretDetails   `json:"sshKeySecret,omitempty"`
	ImagePullPolicy    *corev1.PullPolicy               `json:"imagePullPolicy,omitempty"`
	ReadinessProbe     *corev1.Probe                    `json:"readinessProbe,omitempty"`
	ScriptsGetCmd      string                           `json:"scriptsGetCmd,omitempty"`
	IsDebug            string                           `json:"isDebug,omitempty"`
	IsDeleteTopolgy    string                           `json:"isDeleteTopology,omitempty"`
	ExternalSvcType    *string                          `json:"externalSvcType,omitempty"`
	DbSecret           *OracleRestartDbPwdSecretDetails `json:"dbSecret,omitempty"`
	TdeWalletSecret    *OracleRestartDbPwdSecretDetails `json:"tdeWalletSecret,omitempty"`
	ServiceDetails     ServiceSpec                      `json:"serviceDetails,omitempty"`
	OldSpec            string                           `json:"oldSpec,omitempty"`
	CpuCount           int                              `json:"cpuCount,omitempty"`
	SgaSize            string                           `json:"sgaSize,omitempty"`
	PgaSize            string                           `json:"pgaSize,omitempty"`
	Processes          int                              `json:"processes,omitempty"`
	HugePages          int                              `json:"hugePages,omitempty"`
	Resources          *corev1.ResourceRequirements     `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	SecurityContext    *corev1.PodSecurityContext       `json:"securityContext,omitempty"`
}

type OracleRestartNodePortSvc struct {
	PortMappings  []OracleRestartPortMapping `json:"portMappings,omitempty"`
	SvcName       string                     `json:"name,omitempty"`
	SvcType       string                     `json:"svcType,omitempty"`
	SvcLBIP       string                     `json:"svcLBIP,omitempty"`
	SvcAnnotation map[string]string          `json:"svcAnnotation,omitempty"`
	OnsTargetPort *int32                     `json:"onsTargetPort,omitempty"` // Port that will be exposed on the service.
	OnsLocalPort  *int32                     `json:"onsLocalPort,omitempty"`  // Port that will be exposed on the service.
}

type OracleRestartPortMapping struct {
	Port       int32 `json:"port,omitempty"`
	TargetPort int32 `json:"targetPort,omitempty"`
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default=TCP
	Protocol corev1.Protocol `json:"protocol,omitempty"`
	NodePort int32           `json:"nodePort,omitempty"`
}

type OracleRestartNodestatus struct {
	Name        string                           `json:"name,omitempty"`
	NodeDetails *OracleRestartNodeDetailedStatus `json:"nodeDetails,omitempty"`
}

type OracleRestartNodeDetailedStatus struct {
	WorkerNode     string                     `json:"workerNode,omitempty"` //Optional Env variables for Shards
	PvcName        map[string]string          `json:"pvcName,omitempty"`
	NodePortSvc    []OracleRestartNodePortSvc `json:"nodePortSvc,omitempty"` // Port mappings for the service that is created. The service is created if
	PortMappings   []OracleRestartPortMapping `json:"portMappings,omitempty"`
	ClusterState   string                     `json:"clusterState,omitempty"`
	InstanceState  string                     `json:"InstanceState,omitempty"`
	PodState       string                     `json:"PodState,omitempty"`
	IsDelete       string                     `json:"isDelete,omitempty"`
	State          string                     `json:"state,omitempty"`
	MountedDevices []string                   `json:"mountedDevices,omitempty"`
}

type OracleRestartLifecycleState string

const (
	OracleRestartAvailableState      OracleRestartLifecycleState = "AVAILABLE"
	OracleRestartFailedState         OracleRestartLifecycleState = "FAILED"
	OracleRestartUpdateState         OracleRestartLifecycleState = "UPDATING"
	OracleRestartProvisionState      OracleRestartLifecycleState = "PROVISIONING"
	OracleRestartPendingState        OracleRestartLifecycleState = "PENDING"
	OracleRestartFieldNotDefined     OracleRestartLifecycleState = "NOT_DEFINED"
	OracleRestartPodNotReadyState    OracleRestartLifecycleState = "PODNOTREADY"
	OracleRestartPodFailureState     OracleRestartLifecycleState = "PODFAILURE"
	OracleRestartPodNotFound         OracleRestartLifecycleState = "PODNOTFOUND"
	OracleRestartStatefulSetFailure  OracleRestartLifecycleState = "STATEFULSETFAILURE"
	OracleRestartStatefulSetNotFound OracleRestartLifecycleState = "STATEFULSETNOTFOUND"
	OracleRestartPodAvailableState   OracleRestartLifecycleState = "PODAVAILABLE"
	OracleRestartDeletingState       OracleRestartLifecycleState = "DELETING"
	OracleRestartDeleteErrorState    OracleRestartLifecycleState = "DELETE_ERROR"
	OracleRestartTerminated          OracleRestartLifecycleState = "TERMINATED"
	OracleRestartLabelPatchingError  OracleRestartLifecycleState = "LABELPATCHINGERROR"
	OracleRestartDeletePVCError      OracleRestartLifecycleState = "DELETEPVCERROR"
	OracleRestartAddInstState        OracleRestartLifecycleState = "OracleRestart_INST_ADDITION"
	OracleRestartManualState         OracleRestartLifecycleState = "MANUAL"
)

type OracleRestartCrdReconcileState string

const (
	OracleRestartCrdReconcileErrorState     OracleRestartCrdReconcileState = "ReconcileError"
	OracleRestartCrdReconcileErrorReason    OracleRestartCrdReconcileState = "LastReconcileCycleFailed"
	OracleRestartCrdReconcileQueuedState    OracleRestartCrdReconcileState = "ReconcileQueued"
	OracleRestartCrdReconcileQueuedReason   OracleRestartCrdReconcileState = "LastReconcileCycleQueued"
	OracleRestartCrdReconcileCompeleteState OracleRestartCrdReconcileState = "ReconcileComplete"
	OracleRestartCrdReconcileCompleteReason OracleRestartCrdReconcileState = "LastReconcileCycleCompleted"
	OracleRestartCrdReconcileWaitingState   OracleRestartCrdReconcileState = "ReconcileWaiting"
	OracleRestartCrdReconcileWaitingReason  OracleRestartCrdReconcileState = "LastReconcileCycleWaiting"
)

// var
var OracleRestartKubeConfigOnce sync.Once

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".status.configParams.dbName",name="DbName",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.dbState",name="DbState",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.role",name="Role",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.releaseUpdate",name="Version",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.pdbConnectString",name="Pdb Connect Str",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.state",name="State",type="string"
// OracleRestart is the Schema for the OracleRestarts API
type OracleRestart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OracleRestartSpec   `json:"spec,omitempty"`
	Status OracleRestartStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OracleRestartList contains a list of OracleRestart
type OracleRestartList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OracleRestart `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OracleRestart{}, &OracleRestartList{})
}
