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

// RacDatabaseSpec captures desired configuration for a RAC database deployment.
type RacDatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	InstDetails          []RacInstDetailSpec          `json:"instDetails,omitempty"`
	ClusterDetails       *RacClusterDetailSpec        `json:"instanceDetails,omitempty"`
	ConfigParams         *RacInitParams               `json:"configParams"`
	AsmStorageDetails    []AsmDiskGroupDetails        `json:"asmDiskGroupDetails"`
	OldAsmStorageDetails *RacAsmDiskDetails           `json:"asmStorageDetails,omitempty"`
	EnvVars              []corev1.EnvVar              `json:"envVars,omitempty"`
	NfsStorageDetails    *corev1.NFSVolumeSource      `json:"nfsStorageDetails,omitempty"`
	UseNfsforSwStorage   string                       `json:"useNfsforSwStorage,omitempty"`
	StorageClass         string                       `json:"storageClass,omitempty"`
	StorageSizeInGB      int                          `json:"storageSizeInGB,omitempty"`
	Image                string                       `json:"image,omitempty"`
	ImagePullSecret      string                       `json:"imagePullSecret,omitempty"`
	ScriptsLocation      string                       `json:"scriptsLocation,omitempty"`
	IsDeleteOraPvc       string                       `json:"isDeleteOraPvc,omitempty"`
	SshKeySecret         *RACSshSecretDetails         `json:"sshKeySecret,omitempty"`
	ImagePullPolicy      *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ReadinessProbe       *corev1.Probe                `json:"readinessProbe,omitempty"`
	ScriptsGetCmd        string                       `json:"scriptsGetCmd,omitempty"`
	IsDebug              string                       `json:"isDebug,omitempty"`
	ScanSvcName          string                       `json:"scanSvcName"`
	SecurityContext      *corev1.PodSecurityContext   `json:"securityContext,omitempty"`
	IsDeleteTopolgy      string                       `json:"isDeleteTopology,omitempty"`
	ExternalSvcType      *string                      `json:"externalSvcType,omitempty"`
	ScanSvcTargetPort    *int32                       `json:"scanSvcTargetPort,omitempty"`
	ScanSvcLocalPort     *int32                       `json:"scanSvcLocalPort,omitempty"`
	DbSecret             *RacDbPwdSecretDetails       `json:"dbSecret,omitempty"`
	TdeWalletSecret      *RacDbPwdSecretDetails       `json:"tdeWalletSecret,omitempty"`
	ServiceDetails       RacServiceSpec               `json:"serviceDetails,omitempty"`
	Resources            *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	IsFailed             bool                         `json:"isFailed,omitempty"`
	IsManual             bool                         `json:"isManual,omitempty"`
	SrvAccountName       string                       `json:"serviceAccountName,omitempty"`
}

// RacAsmDiskDetails captures ASM disk group configuration for legacy specs.
type RacAsmDiskDetails struct {
	DisksBySize []RacDiskBySize `json:"disksBySize,omitempty"`
	WorkerNodes []string        `json:"workerNodes,omitempty"`
	// DisableAutoUpdate bool     `json:"disableAutoUpdate,omitempty"`
	AutoUpdate string `json:"autoUpdate,omitempty"`
}

// RacDiskBySize represents a list of disks grouped by size
type RacDiskBySize struct {
	StorageSizeInGb int      `json:"storageSizeInGb,omitempty"`
	DiskNames       []string `json:"diskNames,omitempty"`
}

// RacInitParams stores initialization parameters for RAC deployment.
type RacInitParams struct {
	GridHome                string           `json:"gridHome,omitempty"`
	DbHome                  string           `json:"dbHome,omitempty"`
	GridBase                string           `json:"gridBase,omitempty"`
	DbBase                  string           `json:"dbBase,omitempty"`
	Inventory               string           `json:"inventory,omitempty"`
	GridResponseFile        *RacResponseFile `json:"gridResponseFile,omitempty"`
	DbResponseFile          *RacResponseFile `json:"dbResponseFile,omitempty"`
	GridSwZipFile           string           `json:"gridSwZipFile,omitempty"`
	DbSwZipFile             string           `json:"dbSwZipFile,omitempty"`
	OPatchSwZipFile         string           `json:"oPatchSwZipFile,omitempty"`
	StagingSoftwareLocation string           `json:"stagingSoftwareLocation,omitempty"`
	OpType                  string           `json:"opType,omitempty"`
	CpuCount                int              `json:"cpuCount,omitempty"`
	SgaSize                 string           `json:"sgaSize,omitempty"`
	PgaSize                 string           `json:"pgaSize,omitempty"`
	Processes               int              `json:"processes,omitempty"`
	HugePages               int              `json:"hugePages,omitempty"`
	DbUniqueName            string           `json:"dbUniqueName,omitempty"`
	DbName                  string           `json:"dbName,omitempty"`
	PdbName                 string           `json:"pdbName,omitempty"`
	DbStorageType           string           `json:"dbStorageType,omitempty"`
	DbCharSet               string           `json:"dbCharSet,omitempty"`
	DbType                  string           `json:"dbType,omitempty"`
	DbConfigType            string           `json:"dbConfigType,omitempty"`
	EnableArchiveLog        string           `json:"enableArchiveLog,omitempty"`
	SwMountLocation         string           `json:"swMountLocation,omitempty"`
	HostSwStageLocation     string           `json:"hostSwStageLocation,omitempty"`
	RuPatchLocation         string           `json:"ruPatchLocation,omitempty"`
	RuFolderName            string           `json:"ruFolderName,omitempty"`
	OPatchLocation          string           `json:"oPatchLocation,omitempty"`
	OneOffLocation          string           `json:"oneOffLocation,omitempty"`
	DbOneOffIds             string           `json:"dbOneOffIds,omitempty"`
	GridOneOffIds           string           `json:"gridOneOffIds,omitempty"`
}

// RacInstDetailSpec describes per-instance configuration in old-style specs.
type RacInstDetailSpec struct {
	Name             string                       `json:"name"`
	HostSwLocation   string                       `json:"hostSwLocation,omitempty"`
	WorkerNode       []string                     `json:"workerNode,omitempty"`
	EnvVars          []corev1.EnvVar              `json:"envVars,omitempty"`
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` //Optional resource requiremen
	Label            string                       `json:"label,omitempty"`
	IsDelete         string                       `json:"isDelete,omitempty"`
	IsForceDelete    string                       `json:"isForceDelete,omitempty"`
	IsKeepPVC        string                       `json:"isKeepPVC,omitempty"`
	PvcName          map[string]string            `json:"pvcName,omitempty"`
	VipSvcName       string                       `json:"vipSvcName"`
	NodePortSvc      []RacNodePortSvc             `json:"nodePortSvc,omitempty"`  // Port mappings for the service that is created. The service is created if
	PortMappings     []RacPortMapping             `json:"portMappings,omitempty"` // Port mappings for the service that is created. The service is created if there is at least
	PrivateIPDetails []PrivIpDetailSpec           `json:"privateIPDetails,omitempty"`
	EnvFile          string                       `json:"envFile,omitempty"`
	OnsTargetPort    *int32                       `json:"onsTargetPort,omitempty"` // Port that will be exposed on the service.
	LsnrTargetPort   *int32                       `json:"lsnrTargetPort,omitempty"`
	OnsLocalPort     *int32                       `json:"onsLocalPort,omitempty"` // Port that will be exposed on the service.
	LsnrLocalPort    *int32                       `json:"lsnrLocalPort,omitempty"`
}

// RacClusterDetailSpec defines cluster-wide configuration for new-style specs.
type RacClusterDetailSpec struct {
	NodeCount          int                `json:"nodeCount"`
	RacHostSwLocation  string             `json:"racHostSwLocation"`
	RacNodeName        string             `json:"racNodeName"`
	BaseOnsTargetPort  int32              `json:"baseOnsTargetPort,omitempty"`
	BaseLsnrTargetPort int32              `json:"baseLsnrTargetPort,omitempty"`
	PrivateIPDetails   []PrivIpDetailSpec `json:"privateIPDetails,omitempty"`
	WorkerNodeSelector map[string]string  `json:"workerNodeSelector,omitempty"`
}

// RacResponseFile Name
type RacResponseFile struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	Name          string `json:"name,omitempty"`
}

// PrivIpDetailSpec captures details for private network interfaces.
type PrivIpDetailSpec struct {
	Name      string `json:"name,omitempty"`
	IP        string `json:"ip,omitempty"`
	Interface string `json:"interface,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Mac       string `json:"mac,omitempty"`
	Type      string `json:"type,omitempty"` // This field is added for future requirements we want to specify which one to be used for private network
}

// RacNetworkDetailSpec defines network attributes for RAC operations.
type RacNetworkDetailSpec struct {
	Name      string   `json:"name,omitempty"`
	IPs       []string `json:"ips,omitempty"`
	Interface string   `json:"interface,omitempty"`
	Namespace string   `json:"namespace,omitempty"`
	Mac       string   `json:"mac,omitempty"`
}

// MacvlanIPAM represents IPAM configuration extracted from a MACVLAN NAD.
type MacvlanIPAM struct {
	Subnet     string `json:"subnet"`
	RangeStart string `json:"rangeStart"`
	RangeEnd   string `json:"rangeEnd"`
	Gateway    string `json:"gateway"`
}

// MacvlanConfig wraps MACVLAN network configuration details.
type MacvlanConfig struct {
	IPAM MacvlanIPAM `json:"ipam"`
}

// RacDbPwdSecretDetails contains secret reference data for database password
// files and keys.
type RacDbPwdSecretDetails struct {
	Name                 string `json:"name,omitempty"`        // Name of the secret.
	KeyFileName          string `json:"keyFileName,omitempty"` // Name of the key.
	PwdFileName          string `json:"pwdFileName,omitempty"`
	PwdFileMountLocation string `json:"pwdFileMountLocation,omitempty"`
	KeyFileMountLocation string `json:"keyFileMountLocation,omitempty"`
	KeySecretName        string `json:"keySecretName,omitempty"`
	EncryptionType       string `json:"encryptionType,omitempty"`
}

// RACSshSecretDetails describes the secret holding SSH keys for RAC access.
type RACSshSecretDetails struct {
	Name              string `json:"name"` // Name of the secret.
	KeyMountLocation  string `json:"keyMountLocation,omitempty"`
	PrivKeySecretName string `json:"privKeySecretName,omitempty"`
	PubKeySecretName  string `json:"pubKeySecretName,omitempty"`
}

// RacServiceSpec defines optional RAC service configuration details.
type RacServiceSpec struct {
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

// RacNodePortSvc defines a NodePort service configuration for RAC instances.
type RacNodePortSvc struct {
	PortMappings []RacPortMapping `json:"portMappings,omitempty"` // Port mappings for the service that is created. The service is created if there is at least
	SvcName      string           `json:"name"`
	SvcType      string           `json:"svcType"`
}

// RacPortMapping represents a single port mapping for RAC services.
type RacPortMapping struct {
	Port       int32           `json:"port"`       // Port that will be exposed on the service.
	TargetPort int32           `json:"targetPort"` // Docker image port for the application.
	Protocol   corev1.Protocol `json:"protocol"`   // IP protocol for the mapping, e.g., "TCP" or "UDP".
	NodePort   int32           `json:"nodePort,omitempty"`
}

// RacNodeStatus summarizes node-level status for RAC instances.
type RacNodeStatus struct {
	Name        string                 `json:"name,omitempty"`
	NodeDetails *RacNodeDetailedStatus `json:"nodeDetails,omitempty"`
}

// RacNodeDetailedStatus captures fine-grained node information and flags.
type RacNodeDetailedStatus struct {
	WorkerNode     string            `json:"workerNode,omitempty"` //Optional Env variables for Shards
	PvcName        map[string]string `json:"pvcName,omitempty"`
	VipDetails     map[string]string `json:"vipDetails,omitempty"`
	NodePortSvc    []RacNodePortSvc  `json:"nodePortSvc,omitempty"` // Port mappings for the service that is created. The service is created if
	PortMappings   []RacPortMapping  `json:"portMappings,omitempty"`
	ClusterState   string            `json:"clusterState,omitempty"`
	InstanceState  string            `json:"InstanceState,omitempty"`
	PodState       string            `json:"PodState,omitempty"`
	IsDelete       string            `json:"isDelete,omitempty"`
	State          string            `json:"state,omitempty"`
	MountedDevices []string          `json:"mountedDevices,omitempty"`
}

// RacDatabaseStatus defines the observed state of RacDatabase
type RacDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DbName                string            `json:"DbName,omitempty"`
	ScanDetails           map[string]string `json:"scanDetails,omitempty"`
	ConnectString         string            `json:"connectString,omitempty"`
	PdbConnectString      string            `json:"pdbConnectString,omitempty"`
	ExternalConnectString string            `json:"externalConnectString,omitempty"`
	RacNodes              []*RacNodeStatus  `json:"racNodes,omitempty"`
	ReleaseUpdate         string            `json:"releaseUpdate,omitempty"`
	Role                  string            `json:"role,omitempty"`
	DbState               string            `json:"dbState,omitempty"`
	State                 string            `json:"state,omitempty"`
	InstallNode           string            `json:"installNode,omitempty"`
	ClientEtcHost         []string          `json:"clientEtcHost,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions   []metav1.Condition   `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	InstDetails  *[]RacInstDetailSpec `json:"instDetails,omitempty"`
	ConfigParams *RacInitParams       `json:"configParams,omitempty"`
	// AsmDetails         *RacAsmInstanceStatus        `json:"asmDetails,omitempty"`
	AsmDiskGroups      []AsmDiskGroupStatus    `json:"asmDiskGroups,omitempty"`
	NfsStorageDetails  *corev1.NFSVolumeSource `json:"nfsStorageDetails,omitempty"`
	UseNfsforSwStorage string                  `json:"useNfsforSwStorage,omitempty"`
	StorageClass       string                  `json:"storageClass,omitempty"` // Optional Accept storage class name
	// StorageSizeInGB    int                          `json:"storageSizeInGB,omitempty"`
	Image              string                       `json:"image,omitempty"`           // Accept DB Image name
	ImagePullSecret    string                       `json:"imagePullSecret,omitempty"` // Optional The name of an image pull secret in case of a private docker
	ScriptsLocation    string                       `json:"scriptsLocation,omitempty"`
	IsDeleteOraPvc     string                       `json:"isDeleteOraPvc,omitempty"`
	SshKeySecret       *RACSshSecretDetails         `json:"sshKeySecret,omitempty"`
	ImagePullPolicy    *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ReadinessProbe     *corev1.Probe                `json:"readinessProbe,omitempty"`
	ScriptsGetCmd      string                       `json:"scriptsGetCmd,omitempty"`
	IsDebug            string                       `json:"isDebug,omitempty"`
	ScanSvcName        string                       `json:"scanSvcName,omitempty"`
	SecurityContext    *corev1.PodSecurityContext   `json:"securityContext,omitempty"`
	IsDeleteTopolgy    string                       `json:"isDeleteTopology,omitempty"`
	ExternalSvcType    *string                      `json:"externalSvcType,omitempty"`
	ScanSvcTargetPort  *int32                       `json:"scanSvcTargetPort,omitempty"`
	ScanSvcLocalPort   *int32                       `json:"scanSvcLocalPort,omitempty"`
	DbSecret           *RacDbPwdSecretDetails       `json:"dbSecret,omitempty"` //  Secret Name to be used with RAC password
	TdeWalletSecret    *RacDbPwdSecretDetails       `json:"tdeWalletSecret,omitempty"`
	ServiceDetails     RacServiceSpec               `json:"serviceDetails,omitempty"`
	Resources          *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` //Optional resource requiremen`
	OldSpec            string                       `json:"oldSpec,omitempty"`
	ObservedGeneration int64                        `json:"observedGeneration,omitempty"`
}

// RacLifecycleState enumerates RAC lifecycle phases.
type RacLifecycleState string

const (
	RACAvailableState      RacLifecycleState = "AVAILABLE"
	RACFailedState         RacLifecycleState = "FAILED"
	RACUpdateState         RacLifecycleState = "UPDATING"
	RACProvisionState      RacLifecycleState = "PROVISIONING"
	RACPendingState        RacLifecycleState = "PENDING"
	RACFieldNotDefined     RacLifecycleState = "NOT_DEFINED"
	RACPodNotReadyState    RacLifecycleState = "PODNOTREADY"
	RACPodFailureState     RacLifecycleState = "PODFAILURE"
	RACPodNotFound         RacLifecycleState = "PODNOTFOUND"
	RACStatefulSetFailure  RacLifecycleState = "STATEFULSETFAILURE"
	RACStatefulSetNotFound RacLifecycleState = "STATEFULSETNOTFOUND"
	RACPodAvailableState   RacLifecycleState = "PODAVAILABLE"
	RACDeletingState       RacLifecycleState = "DELETING"
	RACDeleteErrorState    RacLifecycleState = "DELETE_ERROR"
	RACTerminated          RacLifecycleState = "TERMINATED"
	RACLabelPatchingError  RacLifecycleState = "LABELPATCHINGERROR"
	RACDeletePVCError      RacLifecycleState = "DELETEPVCERROR"
	RACAddInstState        RacLifecycleState = "RAC_INST_ADDITION"
	RACManualState         RacLifecycleState = "MANUAL"
)

// RacCrdReconcileState enumerates reconcile status values for CRDs.
type RacCrdReconcileState string

const (
	RacCrdReconcileErrorState     RacCrdReconcileState = "ReconcileError"
	RacCrdReconcileErrorReason    RacCrdReconcileState = "LastReconcileCycleFailed"
	RacCrdReconcileQueuedState    RacCrdReconcileState = "ReconcileQueued"
	RacCrdReconcileQueuedReason   RacCrdReconcileState = "LastReconcileCycleQueued"
	RacCrdReconcileCompeleteState RacCrdReconcileState = "ReconcileComplete"
	RacCrdReconcileCompleteReason RacCrdReconcileState = "LastReconcileCycleCompleted"
	RacCrdReconcileWaitingState   RacCrdReconcileState = "ReconcileWaiting"
	RacCrdReconcileWaitingReason  RacCrdReconcileState = "LastReconcileCycleWaiting"
)

// var
var RacKubeConfigOnce sync.Once

// RacDatabase represents a clustered Oracle RAC database instance definition.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.configParams.dbName",name="DbName",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.dbState",name="DbState",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.role",name="Role",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.releaseUpdate",name="Version",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.pdbConnectString",name="Pdb Connect Str",type="string"
// +kubebuilder:printcolumn:JSONPath=".status.state",name="State",type="string"
type RacDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RacDatabaseSpec   `json:"spec,omitempty"`
	Status RacDatabaseStatus `json:"status,omitempty"`
}

// RacDatabaseList contains a list of RacDatabase
// +kubebuilder:object:root=true
type RacDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RacDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RacDatabase{}, &RacDatabaseList{})
}
