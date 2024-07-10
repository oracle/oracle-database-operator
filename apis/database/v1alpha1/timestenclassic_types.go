// Copyright (c) 2019, 2023, Oracle and/or its affiliates. All rights reserved.
//
// The v2 TimesTenClassic object definition

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var DropName string = "NO DROP SET"

// TimesTenClassicSpec defines the desired state of TimesTenClassic
// +k8s:openapi-gen=true
type TimesTenClassicSpec struct {
	TTSpec               TimesTenClassicSpecSpec         `json:"ttspec"`
	Template             *corev1.PodTemplateSpec         `json:"template"`
	VolumeClaimTemplates *[]corev1.PersistentVolumeClaim `json:"volumeClaimTemplates"`
	//----------------------------------------------------------------------
	// If you add things here be sure to add them to deploy/crd.yaml as well
	//----------------------------------------------------------------------
}

// Describes the subscriber configuration the user wants
type TimesTenClassicSubscriberSpec struct {
	Replicas    *int                    `json:"replicas"`
	MaxReplicas *int                    `json:"maxReplicas"`
	Name        *string                 `json:"name"`
	Template    *corev1.PodTemplateSpec `json:"template"`
}

// TimesTenClassicSpecSpec describes the tt-specific attributes of the TimesTenClassic
type TimesTenClassicSpecSpec struct {
	Arch                      *string               `json:"arch"`
	Image                     string                `json:"image"` // The image to run in the TimesTen pods
	ImagePullSecret           string                `json:"imagePullSecret"`
	StorageClassName          string                `json:"storageClassName"`
	StorageSize               *string               `json:"storageSize,omitempty"`
	StorageSelector           *metav1.LabelSelector `json:"storageSelector,omitempty"`
	LogStorageClassName       *string               `json:"logStorageClassName,omitempty"`
	LogStorageSize            *string               `json:"logStorageSize,omitempty"`
	LogStorageSelector        *metav1.LabelSelector `json:"logStorageSelector,omitempty"`
	ReplicationTopology       *string               `json:"replicationTopology,omitempty"`
	Replicas                  *int                  `json:"replicas,omitempty"`
	ReplicationCipherSuite    *string               `json:"replicationCipherSuite,omitempty"`
	ReplicationSSLMandatory   *int                  `json:"replicationSSLMandatory,omitempty"`
	PollingInterval           *int                  `json:"pollingInterval,omitempty"`
	UnreachableTimeout        *int                  `json:"unreachableTimeout,omitempty"`
	RepStateTimeout           *int                  `json:"repStateTimeout,omitempty"`
	DbConfigMap               *[]string             `json:"dbConfigMap,omitempty"`
	DbSecret                  *[]string             `json:"dbSecret,omitempty"`
	RepPort                   *int                  `json:"repPort,omitempty"`
	RepCreateStatement        *string               `json:"repCreateStatement,omitempty"`
	RepReturnServiceAttribute *string               `json:"repReturnServiceAttribute,omitempty"`
	RepStoreAttribute         *string               `json:"repStoreAttribute,omitempty"`
	AgentTCPTimeout           *int                  `json:"agentTcpTimeout,omitempty"`
	AgentTLSTimeout           *int                  `json:"agentTlsTimeout,omitempty"`
	AgentGetTimeout           *int                  `json:"agentGetTimeout,omitempty"`
	AgentPostTimeout          *int                  `json:"agentPostTimeout,omitempty"`
	AgentAsyncTimeout         *int                  `json:"agentAsyncTimeout,omitempty"`
	UpgradeDownPodTimeout     *int                  `json:"upgradeDownPodTimeout,omitempty"`
	DaemonLogSidecar          *bool                 `json:"daemonLogSidecar,omitempty"`
	Prometheus                *Prometheus           `json:"prometheus"`
	ImagePullPolicy           *string               `json:"imagePullPolicy,omitempty"`
	ZzTestInfo                *string               `json:"zzTestInfo,omitempty"`
	ZzAgentDebugInfo          int                   `json:"zzAgentDebugInfo"` // Should the agent generate debug info?
	BothDownBehavior          *string               `json:"bothDownBehavior,omitempty"`
	CacheCleanup              *bool                 `json:"cacheCleanup,omitempty"`
	StopManaging              string                `json:"stopManaging"`
	Reexamine                 string                `json:"reexamine"`
	ImageUpgradeStrategy      *string               `json:"imageUpgradeStrategy,omitempty"`
	ResetUpgradeState         string                `json:"resetUpgradeState"`
	MemoryWarningPercent      *int                  `json:"memoryWarningPercent,omitempty"`
	UseHugePages              bool                  `json:"useHugePages"`
	DatabaseCPURequest        *string               `json:"databaseCPURequest,omitempty"`
	DatabaseMemorySize        *string               `json:"databaseMemorySize"`
	AutomaticMemoryRequests   bool                  `json:"automaticMemoryRequests"`
	AdditionalMemoryRequest   string                `json:"additionalMemoryRequest"`
	DaemonLogMemoryRequest    string                `json:"daemonLogMemoryRequest"`
	DaemonLogCPURequest       string                `json:"daemonLogCPURequest"`
	ExporterMemoryRequest     string                `json:"exporterMemoryRequest"`
	ExporterCPURequest        string                `json:"exporterCPURequest"`

	Subscribers    *TimesTenClassicSubscriberSpec    `json:"subscribers"`
	UpdateStrategy *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy"`
	//----------------------------------------------------------------------
	// If you add things here be sure to add them to deploy/crd.yaml as well
	//----------------------------------------------------------------------
}

type CGAutoRefreshStateType struct {
	N        int    `json:"n"`
	Status   string `json:"status"`
	Duration int    `json:"duration"`
	Start    string `json:"start"`
}

type CacheGroupStatusType struct {
	Owner                    string                    `json:"owner"`
	Name                     string                    `json:"name"`
	RefreshMode              string                    `json:"refreshMode"`
	RefreshState             string                    `json:"refreshState"`
	AutoRefreshState         *[]CGAutoRefreshStateType `json:"autorefresh,omitempty"`
	AutoRefreshStatsFetchErr string                    `json:"autorefreshStatsFetchErr,omitempty"`
}

type CacheStatusType struct {
	CacheAgent     string                  `json:"cacheAgent"`
	CacheUidPwdSet bool                    `json:"cacheUidPwdSet"`
	NCacheGroups   int                     `json:"nCacheGroups"`
	AwtBehindMb    *int                    `json:"awtBehindMb"`
	Cachegroups    *[]CacheGroupStatusType `json:"cachegroups,omitempty"`
}

// Since this has pointers the operator SDK can't automatically figure
// out how to copy it, so we have to help.
func (in *CacheStatusType) DeepCopyInto(out *CacheStatusType) {
	*out = *in
	if in.AwtBehindMb != nil {
		temp := *in.AwtBehindMb
		out.AwtBehindMb = &temp
	}
}

// This describes the state that we have kept for a given Pod
// A number of smaller structs lead up to TimesTenPodStatus,
// which combines them all.

type TimesTenPodStatusPodStatus struct {
	PodPhase              string                   `json:"podPhase"`
	PodIP                 string                   `json:"podIP"`
	Agent                 string                   `json:"agent"`
	LastTimeReachable     int64                    `json:"lastTimeReachable"`
	PrevContainerStatuses []corev1.ContainerStatus `json:"lastContainerStatuses,omitempty"`
	PrevUID               *types.UID               `json:"prevUID,omitempty"`
}
type TimesTenPodStatusTimesTenStatus struct {
	Release  string `json:"release"`
	Instance string `json:"instance"`
	Daemon   string `json:"daemon"`
}
type TimesTenPodStatusDbStatus struct {
	Db              string            `json:"db"`
	DbUpdatable     string            `json:"dbUpdatable"`
	DbId            int64             `json:"dbId"`
	DbOpen          bool              `json:"open"`
	DbConfiguration map[string]string `json:"dbConfiguration"`
	Monitor         map[string]string `json:"monitor"`
	SystemStats     map[string]string `json:"systemStats"`
}
type TimesTenPodStatusReplicationStatus struct {
	RepAgent                string `json:"repAgent"`
	RepScheme               string `json:"repScheme"`
	RepState                string `json:"repState"`
	RepPeerPState           string `json:"repPeerPState"`
	RepPeerPStateFetchErr   string `json:"repPeerPStateFetchErr,omitempty"`
	LastTimeRepStateChanged int64  `json:"lastTimeRepStateChanged"`
}
type TimesTenPodStatusScaleoutStatus struct {
	InstanceType string               `json:"instanceType"` // data or mgmt
	MgmtExamine  *ScaleoutMgmtExamine `json:"mgmtExamine,omitempty"`
	DbStatus     *ScaleoutDbStatus    `json:"dbStatus,omitempty"`
}

type TimesTenReplicationStat struct {
	Subscriber string `json:"subscriber"`
	TrackId    int    `json:"trackId"`
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Value      int64  `json:"value"`
	Class      string `json:"class"`
}

type TimesTenPodStatus struct {
	Initialized              bool   `json:"initialized"`
	Name                     string `json:"name"`
	TTPodType                string `json:"ttPodType"`         // Database or Element or MgmtDb or Subscriber
	IntendedState            string `json:"intendedState"`     // Active or Standby
	PrevIntendedState        string `json:"prevIntendedState"` // Active or Standby
	HighLevelState           string `json:"highLevelState"`    // Ready or NotReady
	PrevHighLevelState       string `json:"prevHighLevelState"`
	LastHighLevelStateSwitch int64  `json:"lastHighLevelStateSwitch"`
	PrevImage                string `json:"prevImage"`
	Ready                    bool   `json:"ready"` // For TT pods this is whether TT is ready;
	// for ZK pods whether ZK says it is ready
	PrevReady           bool   `json:"prevReady"`
	Active              bool   `json:"active"` // For TT Classic pods this is whether this is the active db (and is ready)
	PrevActive          bool   `json:"prevActive"`
	HasBeenSeen         bool   `json:"hasBeenSeen"` // Has this Pod ever existed?
	Quiescing           bool   `json:"quiescing,omitempty"`
	NonRepUpgradeFailed bool   `json:"nonRepUpgradeFailed,omitempty"`
	InstallRelease      string `json:"installRelease"`
	ImageRelease        string `json:"imageRelease"`

	// List of metadata files present in the pod
	AdminUserFile   bool `json:"adminUserFile"`
	SchemaFile      bool `json:"schemaFile"`
	CacheGroupsFile bool `json:"cgFile"`
	CacheUserFile   bool `json:"cacheUserFile"`

	UsingTwosafe  *bool `json:"usingTwosafe,omitempty"` // Twosafe replication in use?
	DisableReturn *bool `json:"disableReturn,omitempty"`
	LocalCommit   *bool `json:"localCommit,omitempty"`

	PodStatus         TimesTenPodStatusPodStatus         `json:"podStatus"`
	TimesTenStatus    TimesTenPodStatusTimesTenStatus    `json:"timestenStatus"`
	DbStatus          TimesTenPodStatusDbStatus          `json:"dbStatus"`
	ReplicationStatus TimesTenPodStatusReplicationStatus `json:"replicationStatus"`
	RepStats          []TimesTenReplicationStat          `json:"repStats,omitempty"`
	CacheStatus       CacheStatusType                    `json:"cacheStatus"`
	ScaleoutStatus    TimesTenPodStatusScaleoutStatus    `json:"scaleoutStatus"`
	CGroupInfo        *CGroupMemoryInfo                  `json:"cgroupInfo,omitempty"`
	PrevCGroupInfo    *CGroupMemoryInfo                  `json:"prevCgroupInfo,omitempty"`
}

// Since this has pointers the operator SDK can't automatically figure
// out how to copy it, so we have to help.
func (in *TimesTenPodStatus) DeepCopyInto(out *TimesTenPodStatus) {
	*out = *in
	if in.UsingTwosafe != nil {
		temp := *in.UsingTwosafe
		out.UsingTwosafe = &temp
	}
	if in.DisableReturn != nil {
		temp := *in.DisableReturn
		out.DisableReturn = &temp
	}
}

// TimesTenClassicStatus defines the observed state of TimesTenClassic
// +k8s:openapi-gen=true
type TimesTenClassicStatus struct {
	StatusVersion string `json:"statusVersion"` // Version of the status schema

	ObservedGeneration      int64  `json:"observedGeneration"`      // Generation of spec this relates to
	LastReconcileTime       int64  `json:"lastReconcile"`           // Last time we saw this object
	LastReconcilingOperator string `json:"lastReconcilingOperator"` // Last operator that updated

	HighLevelState             string              `json:"highLevelState"` // The overall highlevel state of this pair
	PrevHighLevelState         string              `json:"prevHighLevelState"`
	LastHighLevelStateSwitch   int64               `json:"lastHighLevelStateSwitch"` // When did it change
	RepCreateStatement         string              `json:"repCreateStatement"`       // The statement we will actually use
	RepPort                    int                 `json:"repPort"`                  // The port we will actually use
	LastEvent                  int                 `json:"lastEvent"`
	ActivePods                 string              `json:"activePods"` // The pod(s) that are currently 'active'
	PodStatus                  []TimesTenPodStatus `json:"podStatus"`
	PrevStopManaging           string              `json:"prevStopManaging"`
	PrevReexamine              string              `json:"prevReexamine"`
	AwtBehindMb                *int                `json:"awtBehindMb,omitempty"`
	RepStartFailCount          int                 `json:"repStartFailCount"`
	UsingTwosafe               bool                `json:"usingTwosafe"`
	BothDownRecoveryIneligible bool                `json:"bothDownRecoveryIneligible"`

	// Duplicated information from pod status for ease of DataDog reporting
	// for Fidelity
	ActiveRepAgent    string `json:"activeRepAgent"`
	ActiveCacheAgent  string `json:"activeCacheAgent"`
	ActivePermSize    *int64 `json:"activePermSize,omitempty"`
	ActivePermInUse   *int64 `json:"activePermInUse,omitempty"`
	StandbyRepAgent   string `json:"standbyRepAgent"`
	StandbyCacheAgent string `json:"standbyCacheAgent"`
	StandbyPermSize   *int64 `json:"standbyPermSize,omitempty"`
	StandbyPermInUse  *int64 `json:"standbyPermInUse,omitempty"`

	DbShmRequirement int64 `json:"dbShmRequirement,omitempty"`

	AsyncStatus          TimesTenClassicAsyncStatus          `json:"asyncStatus"`
	StandbyDownStandbyAS TimesTenClassicStandbyDownStandbyAS `json:"standbyDownStandbyAS"`
	ClassicUpgradeStatus TimesTenClassicUpgradeStatus        `json:"classicUpgradeStatus"`
	Subscriber           TimesTenClassicSubStatus            `json:"subscriber"`
	ExporterSecret       *string                             `json:"exporterSecret"`
}

// Since this has pointers the operator SDK can't automatically figure
// out how to copy it, so we have to help.
func (in *TimesTenClassicStatus) DeepCopyInto(out *TimesTenClassicStatus) {
	*out = *in
	if in.AwtBehindMb != nil {
		temp := *in.AwtBehindMb
		out.AwtBehindMb = &temp
	}
	if in.ActivePermSize != nil {
		temp := *in.ActivePermSize
		out.ActivePermSize = &temp
	}
	if in.ActivePermInUse != nil {
		temp := *in.ActivePermInUse
		out.ActivePermInUse = &temp
	}
	if in.StandbyPermSize != nil {
		temp := *in.StandbyPermSize
		out.StandbyPermSize = &temp
	}
	if in.StandbyPermInUse != nil {
		temp := *in.StandbyPermInUse
		out.StandbyPermInUse = &temp
	}
	if in.ExporterSecret != nil {
		temp := *in.ExporterSecret
		out.ExporterSecret = &temp
	}
}

// Data related to the subscribers in a replicated object which has them

type TimesTenClassicSubStatus struct {
	HLState           string `json:"state"`
	PrevHLState       string `json:"prevState"`
	LastHLStateSwitch int64  `json:"lastStateSwitch"`
	NewReplicas       int    `json:"newReplicas"`
	PrevReplicas      int    `json:"prevReplicas"`
	Surplusing        bool   `json:"surplusing"`
}

type TimesTenClassicAsyncStatus struct {
	Id       string `json:"id"`
	Errno    int    `json:"errno"`
	Errmsg   string `json:"errmsg"`
	Type     string `json:"type"`
	Caller   string `json:"caller"`
	Host     string `json:"host"`
	PodName  string `json:"podName"`
	Running  bool   `json:"running"`
	Complete bool   `json:"complete"`
	Updated  int64  `json:"updated,omitempty"` // time.Unix()
	Started  int64  `json:"started,omitempty"` // time.Unix()
	Ended    int64  `json:"ended,omitempty"`   // time.Unix()
}

type TimesTenClassicStandbyDownStandbyAS struct {
	Id            string `json:"id"`
	AsyncId       string `json:"asyncId"`
	PodName       string `json:"podName"`
	Status        string `json:"status"`
	DestroyDb     bool   `json:"destroyDb"`
	RepDuplicate  bool   `json:"repDuplicate"`
	StartRepAgent bool   `json:"startRepAgent"`
}

type TimesTenClassicUpgradeStatus struct {
	UpgradeState           string `json:"upgradeState"`
	ImageUpdatePending     bool   `json:"imageUpdatePending"`
	UpgradeStartTime       int64  `json:"upgradeStartTime"`
	PrevUpgradeState       string `json:"prevUpgradeState"`
	LastUpgradeStateSwitch int64  `json:"lastUpgradeStateSwitch"`
	StandbyStatus          string `json:"standbyStatus"`
	StandbyStartTime       int64  `json:"standbyStartTime"`
	ActiveStatus           string `json:"activeStatus"`
	ActiveStartTime        int64  `json:"activeStartTime"`
	PrevResetUpgradeState  string `json:"prevResetUpgradeState"`
}

// ttExporter sidecar obj
type Prometheus struct {
	Publish           *bool   `json:"publish,omitempty"`
	Port              *int    `json:"port,omitempty"`
	Insecure          *bool   `json:"insecure,omitempty"`
	LimitRate         *int    `json:"limitRate,omitempty"`
	CertSecret        *string `json:"certSecret,omitempty"`
	CreatePodMonitors *bool   `json:"createPodMonitors"`
}

// TimesTenObject is an interface that all CRD object schemas must
// adhere to ... in particular TimesTenClassic and TimesTenScaleout
// must both meet this basic criteria

// +kubebuilder:object:generate=false
type TimesTenObject interface {
	ObjectNamespace() string
	ObjectType() string
	ObjectName() string
	ObjectUID() types.UID
	ObjectAnnotations() map[string]string
	ObjectLabels() map[string]string
	GetLastEventNum() int
	IncrLastEventNum() int
	GetPrometheus() *Prometheus
	GetAgentTCPTimeout() int
	GetAgentTLSTimeout() int
	GetAgentGetTimeout() int
	GetAgentPostTimeout() int
	GetAgentAsyncTimeout() int
	GetAgentDebugInfo() int
	GetAgentTestInfo() *string
	GetHighLevelState() string
	SetHighLevelState(string)
	GetMemoryWarningPercent() int
}

// Here is a set of helper functions for TimesTenClassic that are needed
// for it to satisfy the TimesTenObject interface

func (ttc TimesTenClassic) ObjectNamespace() string {
	return ttc.Namespace
}

func (ttc TimesTenClassic) ObjectName() string {
	return ttc.Name
}

func (ttc TimesTenClassic) ObjectType() string {
	return "TimesTenClassic"
}

func (ttc TimesTenClassic) ObjectUID() types.UID {
	return ttc.UID
}

func (ttc TimesTenClassic) ObjectAnnotations() map[string]string {
	return ttc.ObjectMeta.Annotations
}

func (ttc TimesTenClassic) ObjectLabels() map[string]string {
	return ttc.ObjectMeta.Labels
}

func (ttc TimesTenClassic) GetLastEventNum() int {
	return ttc.Status.LastEvent
}

func (ttc TimesTenClassic) IncrLastEventNum() int {
	ttc.Status.LastEvent++
	return ttc.Status.LastEvent
}

func (ttc TimesTenClassic) GetAgentTCPTimeout() int {
	if ttc.Spec.TTSpec.AgentTCPTimeout == nil {
		return 10
	} else {
		return *ttc.Spec.TTSpec.AgentTCPTimeout
	}
}

func (ttc TimesTenClassic) GetPrometheus() *Prometheus {
	return ttc.Spec.TTSpec.Prometheus
}

func (ttc TimesTenClassic) GetAgentTLSTimeout() int {
	if ttc.Spec.TTSpec.AgentTLSTimeout == nil {
		return 10
	} else {
		return *ttc.Spec.TTSpec.AgentTLSTimeout
	}
}

func (ttc TimesTenClassic) GetAgentGetTimeout() int {
	if ttc.Spec.TTSpec.AgentGetTimeout == nil {
		return 60
	} else {
		return *ttc.Spec.TTSpec.AgentGetTimeout
	}
}

func (ttc TimesTenClassic) GetAgentPostTimeout() int {
	if ttc.Spec.TTSpec.AgentPostTimeout == nil {
		return 600
	} else {
		return *ttc.Spec.TTSpec.AgentPostTimeout
	}
}

func (ttc TimesTenClassic) GetAgentAsyncTimeout() int {
	if ttc.Spec.TTSpec.AgentAsyncTimeout == nil {
		return ttc.GetAgentPostTimeout()
	} else {
		return *ttc.Spec.TTSpec.AgentAsyncTimeout
	}
}

func (ttc TimesTenClassic) GetAgentDebugInfo() int {
	return ttc.Spec.TTSpec.ZzAgentDebugInfo
}

func (ttc TimesTenClassic) GetAgentTestInfo() *string {
	return ttc.Spec.TTSpec.ZzTestInfo
}

func (ttc TimesTenClassic) GetHighLevelState() string {
	return ttc.Status.HighLevelState
}

func (ttc TimesTenClassic) SetHighLevelState(s string) {
	ttc.Status.HighLevelState = s
}

func (ttc TimesTenClassic) GetMemoryWarningPercent() int {
	if ttc.Spec.TTSpec.MemoryWarningPercent == nil {
		return 90
	} else {
		return *ttc.Spec.TTSpec.MemoryWarningPercent
	}
}

// This next set of types leads up to the definition of the
// output from "ttGridAdmin instanceStatus -o json". If that output
// changes, this must too.

type ScaleoutInstanceStatusInstance struct {
	CSPort       int      `json:"csPort"`
	DaemonPort   int      `json:"daemonPort"`
	Guid         string   `json:"guid"`
	InstanceHome string   `json:"instanceHome"`
	InstanceType string   `json:"instanceType"`
	Member       bool     `json:"member"`
	Name         string   `json:"name"`
	TTStatus     TTStatus `json:"ttStatus"`
}

type ScaleoutInstanceStatusInstallation struct {
	Instances []ScaleoutInstanceStatusInstance `json:"instances"`
	Location  string                           `json:"location"`
	Name      string                           `json:"name"`
	Release   string                           `json:"release"`
}

type ScaleoutInstanceStatusHost struct {
	DataSpaceGroup int                                  `json:"dataSpaceGroup"`
	ExternalAddr   string                               `json:"externalAddress"`
	Installations  []ScaleoutInstanceStatusInstallation `json:"installations"`
	InternalAddr   string                               `json:"internalAddress"`
	Name           string                               `json:"name"`
}

type ScaleoutInstanceStatus struct {
	Hosts  []ScaleoutInstanceStatusHost `json:"hosts"`
	Status int                          `json:"status"`
}

// This next set of types leads up to the definition of the
// output from "ttGridAdmin mgmtStatus -o json". If that output
// changes, this must too.

type ScaleoutMgmtStatusDbActiveOrStandby struct {
	Host            string `json:"host"`
	ImmutableId     int64  `json:"immutableId"`
	Instance        string `json:"instance"`
	InstanceGuid    string `json:"instanceGuid"`
	InstanceHome    string `json:"instanceHome"`
	InstanceId      int64  `json:"instanceId"`
	InternalAddress string `json:"internalAddress"`
}

type ScaleoutMgmtStatusLocal struct {
	Address      string `json:"address"`
	DaemonPort   string `json:"daemonPort"`
	Host         string `json:"host"`
	Id           string `json:"id"`
	ImmutableId  string `json:"immutableId"`
	Instance     string `json:"instance"`
	InstanceGuid string `json:"instanceGuid"`
	InstanceHome string `json:"instanceHome"`
}

type TTStatusStores struct {
	Host     string `json:"host"`
	Instance string `json:"instance"`
	State    string `json:"state"`
	StoreId  int64  `json:"storeId"`
}

type TTStatusConnection struct {
	ConnId         int    `json:"connId"`
	ConnectionName string `json:"connectionName"`
	Context        string `json:"context"`
	ConType        string `json:"contype"`
	Datastore      string `json:"datastore"`
	Pid            int    `json:"pid"`
	Shmid          string `json:"shmid"`
}

type ScaleoutMgmtStatusInstance struct {
	JsonVer           int64                               `json:"jsonVer"`
	DbActive          ScaleoutMgmtStatusDbActiveOrStandby `json:"dbActive"`
	DbStandby         ScaleoutMgmtStatusDbActiveOrStandby `json:"dbStandby"`
	Local             ScaleoutMgmtStatusLocal             `json:"local"`
	MaxSeq            int                                 `json:"maxSeq"`
	RepStarted        bool                                `json:"repStarted"`
	SeqUpdated        bool                                `json:"seqUpdated"`
	SpaceWarn         bool                                `json:"spaceWarn"`
	Status            int                                 `json:"status"`
	TTDataStoreStatus []TTStatusConnection                `json:"ttDatastoreStatus"`
	TTRepStateGet     string                              `json:"ttRepStateGet"`
	TTStoresActive    TTStatusStores                      `json:"ttStoresActive"`
	TTStoresStandby   TTStatusStores                      `json:"ttStoresStandby"`
}

// The output from "ttGridAdmin mgmtStatus -o json"
type ScaleoutMgmtStatus struct {
	One       ScaleoutMgmtStatusInstance `json:"1"`
	Two       ScaleoutMgmtStatusInstance `json:"2"`
	JsonVer   int                        `json:"jsonVer"`
	MaxStatus int                        `json:"maxStatus"`
	Status    int                        `json:"status"`
}

//----------------------------------------------------------------------
// Output from "ttGridAdmin mgmtExamine"
//----------------------------------------------------------------------

type ScaleoutMgmtExamineInstance struct {
	Address      string `json:"address"`
	Errmsg       string `json:"errmsg"`
	Errno        string `json:"errno"`
	Host         string `json:"host"`
	Instance     string `json:"instance"`
	InstanceGuid string `json:"instanceGuid"`
	InstanceHome string `json:"instanceHome"`
	Status       int    `json:"status"`
}

type ScaleoutMgmtExamineThingToStart struct {
	Address      string `json:"address"`
	DaemonPort   int    `json:"daemonPort"`
	Host         string `json:"host"`
	Instance     string `json:"instance"`
	InstanceGuid string `json:"instanceGuid"`
	InstanceHome string `json:"instanceHome"`
}

type ScaleoutMgmtExamine struct {
	Cmds      []string                          `json:"cmds"`
	Instances []ScaleoutMgmtExamineInstance     `json:"instances"`
	JsonVer   int                               `json:"jsonVer"`
	Msgs      []string                          `json:"msgs"`
	Start     []ScaleoutMgmtExamineThingToStart `json:"start"`
	Status    int                               `json:"status"`
}

//----------------------------------------------------------------------
// This next set of types leads up to the definition of the
// output from "ttGridAdmin dbStatus -o json". If that output
// changes, this must too.
//----------------------------------------------------------------------

type ScaleoutDatabaseOverallStatus struct {
	NCreateFailed    int      `json:"nCreateFailed"`
	NCreated         int      `json:"nCreated"`
	NCreating        int      `json:"nCreating"`
	NDown            int      `json:"nDown"`
	NInstances       int      `json:"nInstances"`
	NInstancesInMap  int      `json:"nInstancesInMap"`
	NInstancesLoaded int      `json:"nInstancesLoaded"`
	NInstancesOpened int      `json:"nInstancesOpened"`
	NLoading         int      `json:"nLoading"`
	NReplicaSets     int      `json:"nReplicaSets"`
	NRSCreated       int      `json:"nRSCreated"`
	NRSLoaded        int      `json:"nRSLoaded"`
	Summary          []string `json:"summary"`
}

type ScaleoutDatabaseElementStatus struct {
	AddInNextDistMap    string `json:"addInNextDistMap"`
	Created             string `json:"created"`
	Creating            string `json:"creating"`
	DataSpace           int    `json:"dataSpace"`
	Destroying          string `json:"destroying"`
	ElementNumber       int    `json:"elementNumber"`
	EvictInNextDistMap  string `json:"evictInNextDistMap"`
	InCurDistMap        string `json:"inCurDistMap"`
	InPrevDistMap       string `json:"inPrevDistMap"`
	LastChangeCode      string `json:"lastChangeCode"`
	LastChangeEnd       string `json:"lastChangeEnd"`
	LastChangeStart     string `json:"lastChangeStart"`
	Loaded              string `json:"loaded"`
	Loading             string `json:"loading"`
	RemoveInNextDistMap string `json:"removeInNextDistMap"`
	State               string `json:"state"`
	SyncReplicaSet      int    `json:"syncReplicaSet"`
	Unloaded            string `json:"unloaded"`
	Unloading           string `json:"unloading"`
}

type ScaleoutDatabaseInstanceStatus struct {
	DataSpaceGroup  string                          `json:"dataSpaceGroup"`
	Elements        []ScaleoutDatabaseElementStatus `json:"elements"`
	HostAddr        string                          `json:"hostAddr"`
	HostName        string                          `json:"hostName"`
	InMembership    bool                            `json:"inMembership"`
	InstanceGuid    string                          `json:"instanceGuid"`
	InstanceHome    string                          `json:"instanceHome"`
	InstanceName    string                          `json:"instanceName"`
	InstanceType    string                          `json:"instanceType"`
	LastChangeCode  int                             `json:"lastChangeCode"`
	LastChangeEnd   string                          `json:"lastChangeEnd"`
	LastChangeStart string                          `json:"lastChangeStart"`
	Opened          string                          `json:"opened"`
}

// The output for ONE database from "ttGridAdmin dbStatus -o json"
type ScaleoutDatabaseStatus struct {
	DbGuid        string                           `json:"dbGuid"`
	DbDefGuid     string                           `json:"dbdefGuid"`
	DeletePending string                           `json:"deletePending"`
	Instances     []ScaleoutDatabaseInstanceStatus `json:"instances"`
	Loaded        string                           `json:"loaded"`
	Name          string                           `json:"name"`
	Opened        string                           `json:"opened"`
	Overall       ScaleoutDatabaseOverallStatus    `json:"overall"`
	PtVersion     string                           `json:"ptVersion"`
	Quiesced      string                           `json:"quiesced"`
}

// The output from "ttGridAdmin dbStatus -o json"
type ScaleoutDbStatus struct {
	Databases []ScaleoutDatabaseStatus `json:"databases"`
	JsonVer   int                      `json:"jsonVer"`
	K         int                      `json:"k"`
	SpaceWarn bool                     `json:"spaceWarn"`
	Status    int                      `json:"status"`
}

//----------------------------------------------------------------------
// Interpretation of the output of the TimesTen "ttStatus -json" command
//----------------------------------------------------------------------

type TTStatusServerInfo struct {
	Pid  int `json:"pid"`
	Port int `json:"port"`
}

type TTStatusInstanceInfo struct {
	Pid  int    `json:"pid"`
	Port int    `json:"port"`
	Name string `json:"name"`
}

type TTStatusConnInfo struct {
	ProcType          string `json:"proc type"`
	Pid               int    `json:"pid"`
	Context           int64  `json:"context"`
	Name              string `json:"name"`
	ConnId            int    `json:"conn id"`
	DisconnectPending bool   `json:"disconnect pending"`
}

type TTStatusObsoleteInfo struct {
	Name    string `json:"name"`
	Context int64  `json:"context"`
	Process int    `json:"process"`
	Type    string `json:"type"`
	// There are lots more but this is good enough
}

type TTStatusDbInfo struct {
	Datastore        string                  `json:"data store"`
	NConnections     int                     `json:"# of conns"`
	NUserConnections int                     `json:"# of user conns"`
	NCSConnections   *int                    `json:"# of cs conns"`
	ObsoleteInfo     *[]TTStatusObsoleteInfo `json:"obsolete info"`
	ShmKey           int                     `json:"shmkey"`
	Shmid            int                     `json:"shmid"`
	Note             string                  `json:"note"`
	PLSQLShmKey      int                     `json:"plsql shmkey"`
	PLSQLShmid       int                     `json:"plsql shmid"`
	PLSQLShmAddr     int64                   `json:"plsql shmaddr"`
	Loading          *bool                   `json:"loading"`
	Unloading        *bool                   `json:"unloading"`
	Conn             []TTStatusConnInfo      `json:"conn info"`
	Open             string                  `json:"open"`
	RamPolicy        *string                 `json:"ram policy,omitempty"`
	RamPolicyNote    *string                 `json:"ram policy note,omitempty"`
}

type TTStatus struct {
	Server   TTStatusServerInfo    `json:"server"`
	Instance *TTStatusInstanceInfo `json:"instance info"`
	DbInfo   []TTStatusDbInfo      `json:"db info"`
	AccessBy string                `json:"access by"`
}

// Info from agent about cgroup memory limits / usage
// and huge page availability on the node

type CGroupMemoryInfo struct {
	CGroupVersion          int              `json:"cgroupVersion"`
	MemoryFailCnt          int64            `json:"memoryFailcnt,omitempty"`
	MemoryLimitInBytes     int64            `json:"memoryLimit,omitempty"`
	MemoryMaxUsageInBytes  int64            `json:"memoryMaxUsage,omitempty"`
	MemoryUsageInBytes     int64            `json:"memoryUsage,omitempty"`
	MemorySoftLimitInBytes int64            `json:"memorySoftLimit,omitempty"`
	MemoryStat             map[string]int64 `json:"memoryStat,omitempty"`

	Huge2MBFailCnt         int64 `json:"huge2MBFailcnt,omitempty"`
	Huge2MBLimitInBytes    int64 `json:"huge2MBLimit,omitempty"`
	Huge2MBUsageInBytes    int64 `json:"huge2MBUsage,omitempty"`
	Huge2MBMaxUsageInBytes int64 `json:"huge2MBMaxUsage,omitempty"`
	Huge1GBFailCnt         int64 `json:"huge1GBFailcnt,omitempty"`
	Huge1GBLimitInBytes    int64 `json:"huge1GBLimit,omitempty"`
	Huge1GBUsageInBytes    int64 `json:"huge1GBUsage,omitempty"`
	Huge1GBMaxUsageInBytes int64 `json:"huge1GBMaxUsage,omitempty"`

	NodeHugePagesTotal int64 `json:"nodeHugePagesTotal"` // From /proc/meminfo
	NodeHugePagesFree  int64 `json:"nodeHugePagesFree"`
	NodeAnonHugePages  int64 `json:"nodeAnonHugePages"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// TimesTenClassic is the Schema for the timestenclassics API
type TimesTenClassic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TimesTenClassicSpec   `json:"spec,omitempty"`
	Status TimesTenClassicStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// TimesTenClassicList contains a list of TimesTenClassic
type TimesTenClassicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TimesTenClassic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TimesTenClassic{}, &TimesTenClassicList{})
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
