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
	Shard              []ShardSpec   `json:"shard"`
	Catalog            []CatalogSpec `json:"catalog"`                      // The catalogSpes accept all the catalog parameters
	Gsm                []GsmSpec     `json:"gsm"`                          // The GsmSpec will accept all the Gsm parameter
	StorageClass       string        `json:"storageClass,omitempty"`       // Optional Accept storage class name
	DbImage            string        `json:"dbImage"`                      // Accept DB Image name
	DbImagePullSecret  string        `json:"dbImagePullSecret,omitempty"`  // Optional The name of an image pull secret in case of a private docker repository.
	GsmImage           string        `json:"gsmImage"`                     // Acccept the GSM image name
	GsmImagePullSecret string        `json:"gsmImagePullSecret,omitempty"` // Optional  The name of an image pull secret in case of a private docker repository.
	Secret             string        `json:"secret"`                       //  Secret Name to be used with Shard
	StagePvcName       string        `json:"stagePvcName,omitempty"`       // the Stagepvc  for the backup of cluster
	PortMappings       []PortMapping `json:"portMappings,omitempty"`       // Port mappings for the service that is created. The service is created if there is at least
	Namespace          string        `json:"namespace,omitempty"`          // Target namespace of the application.
	IsDebug            bool          `json:"isDebug,omitempty"`            // Optional parameter to enable logining
	IsExternalSvc      bool          `json:"isExternalSvc,omitempty"`
	IsClone            bool          `json:"isClone,omitempty"`
	IsDataGuard        bool          `json:"isDataGuard,omitempty"`
	ScriptsLocation    string        `json:"scriptsLocation,omitempty"`
	NsConfigMap        string        `json:"nsConfigMap,omitempty"`
	NsSecret           string        `json:"nsSecret,omitempty"`
	IsDeleteOraPvc     bool          `json:"isDeleteOraPvc,omitempty"`
}

// To understand Metav1.Condition, please refer the link https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1
// ShardingDatabaseStatus defines the observed state of ShardingDatabase
type ShardingDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Shard   map[string]string `json:"shards,omitempty"`
	Catalog map[string]string `json:"catalogs,omitempty"`
	Gsm     GsmStatus         `json:"gsm,omitempty"`

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

// ShardingDatabase is the Schema for the shardingdatabases API
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
	Name             string                       `json:"name"`                                                      // Shard name that will be used deploy StatefulSet
	StorageSizeInGb  int32                        `json:"storageSizeInGb,omitempty"`                                 // Optional Shard Storage Size
	EnvVars          []EnvironmentVariable        `json:"envVars,omitempty"`                                         //Optional Env variables for Shards
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` //Optional resource requirement for the container.
	PvcName          string                       `json:"pvcName,omitempty"`
	Label            string                       `json:"label,omitempty"`
	IsDelete         bool                         `json:"isDelete,omitempty"`
	NodeSelector     map[string]string            `json:"nodeSelector,omitempty"`
	PvAnnotations    map[string]string            `json:"pvAnnotations,omitempty"`
	PvMatchLabels    map[string]string            `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
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
	IsDelete         bool                         `json:"isDelete,omitempty"`
	NodeSelector     map[string]string            `json:"nodeSelector,omitempty"`
	PvAnnotations    map[string]string            `json:"pvAnnotations,omitempty"`
	PvMatchLabels    map[string]string            `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
}

// GsmSpec defines the desired state of GsmSpec
// +k8s:openapi-gen=true
type GsmSpec struct {
	Name string `json:"name"` // Gsm name that will be used deploy StatefulSet

	Replicas         int32                        `json:"replicas,omitempty"`                                        // Gsm Replicas. If you set OraGsmPvcName then it is set default to 1.
	EnvVars          []EnvironmentVariable        `json:"envVars,omitempty"`                                         //Optional Env variables for GSM
	StorageSizeInGb  int32                        `json:"storageSizeInGb,omitempty"`                                 // This parameter will not be used if you use OraGsmPvcName
	Resources        *corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"` // Optional resource requirement for the container.
	PvcName          string                       `json:"pvcName,omitempty"`
	Label            string                       `json:"label,omitempty"` // Optional GSM Label
	IsDelete         bool                         `json:"isDelete,omitempty"`
	NodeSelector     map[string]string            `json:"nodeSelector,omitempty"`
	PvMatchLabels    map[string]string            `json:"pvMatchLabels,omitempty"`
	ImagePulllPolicy *corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
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

const lastSuccessfulSpec = "lastSuccessfulSpec"

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

	return annsv1.SetAnnotations(kubeClient, shardingv1, anns)
}

func init() {
	SchemeBuilder.Register(&ShardingDatabase{}, &ShardingDatabaseList{})
}
