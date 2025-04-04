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
	Shard                     []ShardSpec         `json:"shard"`
	Catalog                   []CatalogSpec       `json:"catalog"`                      // The catalogSpes accept all the catalog parameters
	Gsm                       []GsmSpec           `json:"gsm"`                          // The GsmSpec will accept all the Gsm parameter
	StorageClass              string              `json:"storageClass,omitempty"`       // Optional Accept storage class name
	DbImage                   string              `json:"dbImage"`                      // Accept DB Image name
	DbImagePullSecret         string              `json:"dbImagePullSecret,omitempty"`  // Optional The name of an image pull secret in case of a private docker repository.
	GsmImage                  string              `json:"gsmImage"`                     // Acccept the GSM image name
	GsmImagePullSecret        string              `json:"gsmImagePullSecret,omitempty"` // Optional  The name of an image pull secret in case of a private docker repository.
	StagePvcName              string              `json:"stagePvcName,omitempty"`       // the Stagepvc  for the backup of cluster
	PortMappings              []PortMapping       `json:"portMappings,omitempty"`       // Port mappings for the service that is created. The service is created if there is at least
	IsDebug                   bool                `json:"isDebug,omitempty"`            // Optional parameter to enable logining
	IsExternalSvc             bool                `json:"isExternalSvc,omitempty"`
	IsClone                   bool                `json:"isClone,omitempty"`
	IsDataGuard               bool                `json:"isDataGuard,omitempty"`
	ScriptsLocation           string              `json:"scriptsLocation,omitempty"`
	IsDeleteOraPvc            bool                `json:"isDeleteOraPvc,omitempty"`
	ReadinessCheckPeriod      int                 `json:"readinessCheckPeriod,omitempty"`
	LivenessCheckPeriod       int                 `json:"liveinessCheckPeriod,omitempty"`
	ReplicationType           string              `json:"replicationType,omitempty"`
	IsDownloadScripts         bool                `json:"isDownloadScripts,omitempty"`
	InvitedNodeSubnetFlag     string              `json:"invitedNodeSubnetFlag,omitempty"`
	InvitedNodeSubnet         string              `json:"InvitedNodeSubnet,omitempty"`
	ShardingType              string              `json:"shardingType,omitempty"`
	GsmShardSpace             []GsmShardSpaceSpec `json:"gsmShardSpace,omitempty"`
	GsmShardGroup             []GsmShardGroupSpec `json:"gsmShardGroup,omitempty"`
	ShardRegion               []string            `json:"shardRegion,omitempty"`
	ShardBuddyRegion          string              `json:"shardBuddyRegion,omitempty"`
	GsmService                []GsmServiceSpec    `json:"gsmService,omitempty"`
	ShardConfigName           string              `json:"shardConfigName,omitempty"`
	GsmDevMode                string              `json:"gsmDevMode,omitempty"`
	DbSecret                  *SecretDetails      `json:"dbSecret,omitempty"` //  Secret Name to be used with Shard
	IsTdeWallet               string              `json:"isTdeWallet,omitempty"`
	TdeWalletPvc              string              `json:"tdeWalletPvc,omitempty"`
	FssStorageClass           string              `json:"fssStorageClass,omitempty"`
	TdeWalletPvcMountLocation string              `json:"tdeWalletPvcMountLocation,omitempty"`
	DbEdition                 string              `json:"dbEdition,omitempty"`
	TopicId                   string              `json:"topicId,omitempty"`
}

// To understand Metav1.Condition, please refer the link https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1
// ShardingDatabaseStatus defines the observed state of ShardingDatabase
type ShardingDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Shard   map[string]string `json:"shards,omitempty"`
	Catalog map[string]string `json:"catalogs,omitempty"`

	Gsm GsmStatus `json:"gsm,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	CrdStatus []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.gsm.state",name="Gsm State",type=string
//+kubebuilder:printcolumn:JSONPath=".status.gsm.services",name="Services",type=string
//+kubebuilder:printcolumn:JSONPath=".status.gsm.shards",name="shards",type=string,priority=1

// ShardingDatabase is the Schema for the shardingdatabases API
// +kubebuilder:resource:path=shardingdatabases,scope=Namespaced
// +kubebuilder:storageversion
type ShardingDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShardingDatabaseSpec   `json:"spec,omitempty"`
	Status ShardingDatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ShardingDatabaseList contains a list of ShardingDatabase
type ShardingDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShardingDatabase `json:"items"`
}

// ShardSpec is a specification of Shards for an application deployment.
// +k8s:openapi-gen=true
type ShardSpec struct {
	Name            string                       `json:"name"`                                                      // Shard name that will be used deploy StatefulSet
	StorageSizeInGb int32                        `json:"storageSizeInGb,omitempty"`                                 // Optional Shard Storage Size
	EnvVars         []EnvironmentVariable        `json:"envVars,omitempty"`                                         //Optional Env variables for Shards
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` //Optional resource requirement for the container.
	PvcName         string                       `json:"pvcName,omitempty"`
	Label           string                       `json:"label,omitempty"`
	// +kubebuilder:validation:Enum=enable;disable;failed;force
	IsDelete         string             `json:"isDelete,omitempty"`
	NodeSelector     map[string]string  `json:"nodeSelector,omitempty"`
	PvAnnotations    map[string]string  `json:"pvAnnotations,omitempty"`
	PvMatchLabels    map[string]string  `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	ShardSpace       string             `json:"shardSpace,omitempty"`
	ShardGroup       string             `json:"shardGroup,omitempty"`
	ShardRegion      string             `json:"shardRegion,omitempty"`
	DeployAs         string             `json:"deployAs,omitempty"`
}

// CatalogSpec defines the desired state of CatalogSpec
// +k8s:openapi-gen=true
type CatalogSpec struct {
	Name             string                       `json:"name"`                                                      // Catalog name that will be used deploy StatefulSet
	StorageSizeInGb  int32                        `json:"storageSizeInGb,omitempty"`                                 // Optional Catalog Storage Size and This parameter will not be used if you use PvcName
	EnvVars          []EnvironmentVariable        `json:"envVars,omitempty"`                                         //Optional Env variables for Catalog
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirement for the container.
	PvcName          string                       `json:"pvcName,omitempty"`
	Label            string                       `json:"label,omitempty"`
	IsDelete         string                       `json:"isDelete,omitempty"`
	NodeSelector     map[string]string            `json:"nodeSelector,omitempty"`
	PvAnnotations    map[string]string            `json:"pvAnnotations,omitempty"`
	PvMatchLabels    map[string]string            `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
}

// GsmSpec defines the desired state of GsmSpec
// +k8s:openapi-gen=true
type GsmSpec struct {
	Name string `json:"name"` // Gsm name that will be used deploy StatefulSet

	//Replicas         int32                        `json:"replicas,omitempty"`                                        // Gsm Replicas. If you set OraGsmPvcName then it is set default to 1.
	EnvVars          []EnvironmentVariable        `json:"envVars,omitempty"`                                         //Optional Env variables for GSM
	StorageSizeInGb  int32                        `json:"storageSizeInGb,omitempty"`                                 // This parameter will not be used if you use OraGsmPvcName
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirement for the container.
	PvcName          string                       `json:"pvcName,omitempty"`
	Label            string                       `json:"label,omitempty"` // Optional GSM Label
	IsDelete         string                       `json:"isDelete,omitempty"`
	NodeSelector     map[string]string            `json:"nodeSelector,omitempty"`
	PvAnnotations    map[string]string            `json:"pvAnnotations,omitempty"`
	PvMatchLabels    map[string]string            `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Region           string                       `json:"region,omitempty"`
	DirectorName     string                       `json:"directorName,omitempty"`
}

// ShardGroupSpec Specification

type GsmShardGroupSpec struct {
	Name     string `json:"name"` // Name of the shardgroup.
	Region   string `json:"region,omitempty"`
	DeployAs string `json:"deployAs,omitempty"`
}

// ShardSpace Specs
type GsmShardSpaceSpec struct {
	Name           string `json:"name"`                     // Name of the shardSpace.
	Chunks         int    `json:"chunks,omitempty"`         //chunks is optional
	ProtectionMode string `json:"protectionMode,omitempty"` // Data guard protection mode
	ShardGroup     string `json:"shardGroup,omitempty"`
}

// Service Definition
type GsmServiceSpec struct {
	Name                 string `json:"name"` // Name of the shardSpace.
	Available            string `json:"available,omitempty"`
	ClbGoal              string `json:"clbGoal,omitempty"`
	CommitOutcome        string `json:"commitOutcome,omitempty"`
	DrainTimeout         string `json:"drainTimeout,omitempty"`
	Dtp                  string `json:"dtp,omitempty"`
	Edition              string `json:"edition,omitempty"`
	FailoverPrimary      string `json:"failoverPrimary,omitempty"`
	FailoverRestore      string `json:"failoverRestore,omitempty"`
	FailoverDelay        string `json:"failoverDelay,omitempty"`
	FailoverMethod       string `json:"failoverMethod,omitempty"`
	FailoverRetry        string `json:"failoverRetry,omitempty"`
	FailoverType         string `json:"failoverType,omitempty"`
	GdsPool              string `json:"gdsPool,omitempty"`
	Role                 string `json:"role,omitempty"`
	SessionState         string `json:"sessionState,omitempty"`
	Lag                  int    `json:"lag,omitempty"`
	Locality             string `json:"locality,omitempty"`
	Notification         string `json:"notification,omitempty"`
	PdbName              string `json:"pdbName,omitempty"`
	Policy               string `json:"policy,omitempty"`
	Preferrred           string `json:"preferred,omitempty"`
	PreferredAll         string `json:"prferredAll,omitempty"`
	RegionFailover       string `json:"regionFailover,omitempty"`
	StopOption           string `json:"stopOption,omitempty"`
	SqlTrasactionProfile string `json:"sqlTransactionProfile,omitempty"`
	TableFamily          string `json:"tableFamily,omitempty"`
	Retention            string `json:"retention,omitempty"`
	TfaPolicy            string `json:"tfaPolicy,omitempty"`
}

// Secret Details
type SecretDetails struct {
	Name                 string `json:"name"`                  // Name of the secret.
	KeyFileName          string `json:"keyFileName,omitempty"` // Name of the key.
	NsConfigMap          string `json:"nsConfigMap,omitempty"`
	NsSecret             string `json:"nsSecret,omitempty"`
	PwdFileName          string `json:"pwdFileName"`
	PwdFileMountLocation string `json:"pwdFileMountLocation,omitempty"`
	KeyFileMountLocation string `json:"keyFileMountLocation,omitempty"`
	KeySecretName        string `json:"keySecretName,omitempty"`
	EncryptionType       string `json:"encryptionType,omitempty"`
}

// EnvironmentVariable represents a named variable accessible for containers.
// +k8s:openapi-gen=true
type EnvironmentVariable struct {
	Name  string `json:"name"`  // Name of the variable. Must be a C_IDENTIFIER.
	Value string `json:"value"` // Value of the variable, as defined in Kubernetes core API.
}

// PortMapping is a specification of port mapping for an application deployment.
// +k8s:openapi-gen=true
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

type CrdReconcileState string

const (
	CrdReconcileErrorState     CrdReconcileState = "ReconcileError"
	CrdReconcileErrorReason    CrdReconcileState = "LastReconcileCycleFailed"
	CrdReconcileQueuedState    CrdReconcileState = "ReconcileQueued"
	CrdReconcileQueuedReason   CrdReconcileState = "LastReconcileCycleQueued"
	CrdReconcileCompeleteState CrdReconcileState = "ReconcileComplete"
	CrdReconcileCompleteReason CrdReconcileState = "LastReconcileCycleCompleted"
	CrdReconcileWaitingState   CrdReconcileState = "ReconcileWaiting"
	CrdReconcileWaitingReason  CrdReconcileState = "LastReconcileCycleWaiting"
)

// var
var KubeConfigOnce sync.Once

// #const lastSuccessfulSpec = "lastSuccessfulSpec"
const lastSuccessfulSpecOnsInfo = "lastSuccessfulSpeOnsInfo"

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

// GetLastSuccessfulOnsInfo returns spec from the lass successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulSpec.
func (shardingv1 *ShardingDatabase) GetLastSuccessfulOnsInfo() ([]byte, error) {
	val, ok := shardingv1.GetAnnotations()[lastSuccessfulSpecOnsInfo]
	if !ok {
		return nil, nil
	}
	specBytes := []byte(val)
	return specBytes, nil
}

// UpdateLastSuccessfulSpec updates lastSuccessfulSpec with the current spec.
func (shardingv1 *ShardingDatabase) UpdateLastSuccessfulSpecOnsInfo(kubeClient client.Client, specBytes []byte) error {

	anns := map[string]string{
		lastSuccessfulSpecOnsInfo: string(specBytes),
	}

	return annsv1.PatchAnnotations(kubeClient, shardingv1, anns)
}

func init() {
	SchemeBuilder.Register(&ShardingDatabase{}, &ShardingDatabaseList{})
}
