/*
** Copyright (c) 2026 Oracle and/or its affiliates.
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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DataguardProducerMode string

const (
	DataguardProducerModeDisabled DataguardProducerMode = "Disabled"
	DataguardProducerModePreview  DataguardProducerMode = "Preview"
	DataguardProducerModeManaged  DataguardProducerMode = "Managed"
)

// DataguardPrereqsSpec controls local database prerequisite setup for broker usage.
type DataguardPrereqsSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	// BrokerConfigDir optionally overrides the dg_broker_config_file location root.
	// Use an absolute filesystem path for SIDB or an ASM path such as +DATA/... for RAC/shared storage.
	BrokerConfigDir string `json:"brokerConfigDir,omitempty"`
	// StandbyRedoSize optionally overrides the standby redo log size (for example 200M, 1G).
	StandbyRedoSize string `json:"standbyRedoSize,omitempty"`
}

// DataguardAuthWalletSpec controls optional broker-side auth wallet lifecycle.
type DataguardAuthWalletSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	// PasswordSecretRef optionally points to a user-managed wallet password secret.
	// When omitted, the operator generates and stores a password secret for the broker.
	PasswordSecretRef *DataguardSecretRef `json:"passwordSecretRef,omitempty"`
	// RebuildToken triggers an explicit wallet rebuild when the value changes.
	RebuildToken string `json:"rebuildToken,omitempty"`
}

// DataguardProducerSpec configures how a producer exposes or manages DG topology.
type DataguardProducerSpec struct {
	// +kubebuilder:validation:Enum=Disabled;Preview;Managed
	Mode DataguardProducerMode `json:"mode,omitempty"`
	// Prereqs optionally enables local database prerequisite configuration for Data Guard broker usage.
	Prereqs *DataguardPrereqsSpec `json:"prereqs,omitempty"`
	// AuthWallet optionally publishes default broker auth wallet settings for generated DataguardBroker specs.
	AuthWallet *DataguardAuthWalletSpec `json:"authWallet,omitempty"`
}

func normalizeDataguardProducerMode(spec *DataguardProducerSpec) DataguardProducerMode {
	return EffectiveDataguardProducerMode(spec)
}

func EffectiveDataguardProducerMode(spec *DataguardProducerSpec) DataguardProducerMode {
	if spec == nil || strings.TrimSpace(string(spec.Mode)) == "" {
		return DataguardProducerModePreview
	}
	return DataguardProducerMode(strings.TrimSpace(string(spec.Mode)))
}

func DataguardProducerPrereqsEnabled(spec *DataguardProducerSpec) bool {
	return spec != nil && spec.Prereqs != nil && spec.Prereqs.Enabled
}

func DataguardProducerBrokerConfigDir(spec *DataguardProducerSpec) string {
	if spec == nil || spec.Prereqs == nil {
		return ""
	}
	return strings.TrimSpace(spec.Prereqs.BrokerConfigDir)
}

func DataguardProducerStandbyRedoSize(spec *DataguardProducerSpec) string {
	if spec == nil || spec.Prereqs == nil {
		return ""
	}
	return strings.TrimSpace(spec.Prereqs.StandbyRedoSize)
}

// DataguardSourceRef identifies the producer object that published a DG topology.
type DataguardSourceRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// DataguardPolicySpec holds topology-wide Data Guard policy settings.
type DataguardPolicySpec struct {
	ProtectionMode    string `json:"protectionMode,omitempty"`
	TransportMode     string `json:"transportMode,omitempty"`
	FastStartFailover bool   `json:"fastStartFailover,omitempty"`
}

// DataguardLocalRef points to an in-cluster database resource backing a DG member.
type DataguardLocalRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// DataguardSecretRef identifies a secret used for DG operations.
type DataguardSecretRef struct {
	SecretName string `json:"secretName,omitempty"`
	SecretKey  string `json:"secretKey,omitempty"`
}

// DataguardEndpointSpec declares one reachable endpoint for a DG member.
type DataguardEndpointSpec struct {
	Name        string `json:"name,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int32  `json:"port,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	SSLServerDN string `json:"sslServerDN,omitempty"`
}

// DataguardTCPSConfig normalizes TCPS metadata for DG consumers.
type DataguardTCPSConfig struct {
	Enabled            bool   `json:"enabled,omitempty"`
	ServerTLSSecret    string `json:"serverTLSSecret,omitempty"`
	ClientWalletSecret string `json:"clientWalletSecret,omitempty"`
	WalletKey          string `json:"walletKey,omitempty"`
	SSLServerDN        string `json:"sslServerDN,omitempty"`
}

// DataguardTopologyTCPSDefaults carries topology-wide TCPS defaults.
type DataguardTopologyTCPSDefaults struct {
	ClientWalletSecret string `json:"clientWalletSecret,omitempty"`
}

// DataguardTopologyDefaults carries reusable defaults for topology members.
type DataguardTopologyDefaults struct {
	AdminSecretRef *DataguardSecretRef            `json:"adminSecretRef,omitempty"`
	TCPS           *DataguardTopologyTCPSDefaults `json:"tcps,omitempty"`
}

// DataguardTopologyMember defines one participant in a DG topology.
type DataguardTopologyMember struct {
	Name           string                  `json:"name,omitempty"`
	Role           string                  `json:"role,omitempty"`
	DBUniqueName   string                  `json:"dbUniqueName,omitempty"`
	LocalRef       *DataguardLocalRef      `json:"localRef,omitempty"`
	AdminSecretRef *DataguardSecretRef     `json:"adminSecretRef,omitempty"`
	Endpoints      []DataguardEndpointSpec `json:"endpoints,omitempty"`
	TCPS           *DataguardTCPSConfig    `json:"tcps,omitempty"`
}

// DataguardTopologyPair explicitly maps a primary member to a standby-side member.
type DataguardTopologyPair struct {
	Primary string `json:"primary,omitempty"`
	Standby string `json:"standby,omitempty"`
	Type    string `json:"type,omitempty"`
}

// DataguardObserverSpec carries observer runtime settings for a topology.
type DataguardObserverSpec struct {
	Enabled      bool              `json:"enabled,omitempty"`
	Name         string            `json:"name,omitempty"`
	Image        string            `json:"image,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// DataguardTopologySpec is the generic desired-state DG topology contract.
type DataguardTopologySpec struct {
	SourceKind string                     `json:"sourceKind,omitempty"`
	SourceRef  *DataguardSourceRef        `json:"sourceRef,omitempty"`
	Policy     *DataguardPolicySpec       `json:"policy,omitempty"`
	Defaults   *DataguardTopologyDefaults `json:"defaults,omitempty"`
	Members    []DataguardTopologyMember  `json:"members,omitempty"`
	Pairs      []DataguardTopologyPair    `json:"pairs,omitempty"`
	Observer   *DataguardObserverSpec     `json:"observer,omitempty"`
}

// ResolveDataguardTopologyMemberExplicitAdminSecretRef resolves an explicitly declared
// admin secret reference, preferring the member-level value over topology defaults.
func ResolveDataguardTopologyMemberExplicitAdminSecretRef(topology *DataguardTopologySpec, member *DataguardTopologyMember) (string, string, bool) {
	if member != nil && member.AdminSecretRef != nil {
		return strings.TrimSpace(member.AdminSecretRef.SecretName), strings.TrimSpace(member.AdminSecretRef.SecretKey), true
	}
	if topology != nil && topology.Defaults != nil && topology.Defaults.AdminSecretRef != nil {
		return strings.TrimSpace(topology.Defaults.AdminSecretRef.SecretName), strings.TrimSpace(topology.Defaults.AdminSecretRef.SecretKey), true
	}
	return "", "", false
}

// ResolveDataguardTopologyMemberClientWalletSecret resolves the TCPS client wallet
// secret name, preferring the member-level value over topology defaults.
func ResolveDataguardTopologyMemberClientWalletSecret(topology *DataguardTopologySpec, member *DataguardTopologyMember) string {
	if member != nil && member.TCPS != nil {
		if secretName := strings.TrimSpace(member.TCPS.ClientWalletSecret); secretName != "" {
			return secretName
		}
	}
	if topology != nil && topology.Defaults != nil && topology.Defaults.TCPS != nil {
		return strings.TrimSpace(topology.Defaults.TCPS.ClientWalletSecret)
	}
	return ""
}

// DataguardExecutionStatus publishes the producer's default DG runner image.
type DataguardExecutionStatus struct {
	Image            string                   `json:"image,omitempty"`
	ImagePullSecrets []string                 `json:"imagePullSecrets,omitempty"`
	AuthWallet       *DataguardAuthWalletSpec `json:"authWallet,omitempty"`
}

// DataguardStatusRef points back to the DataguardBroker resource a producer is bound to.
type DataguardStatusRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// DataguardRenderedBrokerSpec is the copy-ready subset of DataguardBroker spec published by producers.
type DataguardRenderedBrokerSpec struct {
	Execution *DataguardExecutionSpec `json:"execution,omitempty"`
	Topology  *DataguardTopologySpec  `json:"topology,omitempty"`
}

// DataguardRenderedBrokerStatus publishes a ready-to-copy DataguardBroker spec snapshot.
type DataguardRenderedBrokerStatus struct {
	Name         string                       `json:"name,omitempty"`
	Namespace    string                       `json:"namespace,omitempty"`
	Spec         *DataguardRenderedBrokerSpec `json:"spec,omitempty"`
	TopologyHash string                       `json:"topologyHash,omitempty"`
	GeneratedAt  *metav1.Time                 `json:"generatedAt,omitempty"`
	Ready        bool                         `json:"ready,omitempty"`
}

// ProducerDataguardStatus is the common DG publication handoff for SIDB and RAC.
type ProducerDataguardStatus struct {
	BrokerRef             *DataguardStatusRef            `json:"brokerRef,omitempty"`
	MemberName            string                         `json:"memberName,omitempty"`
	Role                  string                         `json:"role,omitempty"`
	DBUniqueName          string                         `json:"dbUniqueName,omitempty"`
	PrimaryMemberName     string                         `json:"primaryMemberName,omitempty"`
	RenderedBrokerSpec    *DataguardRenderedBrokerStatus `json:"renderedBrokerSpec,omitempty"`
	Endpoints             []DataguardEndpointSpec        `json:"endpoints,omitempty"`
	TCPS                  *DataguardTCPSConfig           `json:"tcps,omitempty"`
	Phase                 string                         `json:"phase,omitempty"`
	ReadyForBroker        bool                           `json:"readyForBroker,omitempty"`
	TopologyHash          string                         `json:"topologyHash,omitempty"`
	LastPublishedTime     *metav1.Time                   `json:"lastPublishedTime,omitempty"`
	PublishedTopologyHash string                         `json:"publishedTopologyHash,omitempty"`
	TopologyLocked        bool                           `json:"topologyLocked,omitempty"`
	Conditions            []metav1.Condition             `json:"conditions,omitempty"`
}

// ShardingDataguardMemberStatus tracks one resolved DG member from a sharding topology.
type ShardingDataguardMemberStatus struct {
	Name              string                  `json:"name,omitempty"`
	Role              string                  `json:"role,omitempty"`
	DBUniqueName      string                  `json:"dbUniqueName,omitempty"`
	ShardGroup        string                  `json:"shardGroup,omitempty"`
	ShardSpace        string                  `json:"shardSpace,omitempty"`
	PrimaryMemberName string                  `json:"primaryMemberName,omitempty"`
	Endpoints         []DataguardEndpointSpec `json:"endpoints,omitempty"`
	TCPS              *DataguardTCPSConfig    `json:"tcps,omitempty"`
	Phase             string                  `json:"phase,omitempty"`
	Message           string                  `json:"message,omitempty"`
}

// DataguardPairStatus reports observed state for one declared DG relationship.
type DataguardPairStatus struct {
	Primary string `json:"primary,omitempty"`
	Standby string `json:"standby,omitempty"`
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// ShardingDataguardStatus is the producer-side DG publication state for sharding.
type ShardingDataguardStatus struct {
	BrokerRef             *DataguardStatusRef             `json:"brokerRef,omitempty"`
	RenderedBrokerSpec    *DataguardRenderedBrokerStatus  `json:"renderedBrokerSpec,omitempty"`
	Members               []ShardingDataguardMemberStatus `json:"members,omitempty"`
	Pairs                 []DataguardPairStatus           `json:"pairs,omitempty"`
	Phase                 string                          `json:"phase,omitempty"`
	ReadyForBroker        bool                            `json:"readyForBroker,omitempty"`
	TopologyHash          string                          `json:"topologyHash,omitempty"`
	LastPublishedTime     *metav1.Time                    `json:"lastPublishedTime,omitempty"`
	PublishedTopologyHash string                          `json:"publishedTopologyHash,omitempty"`
	TopologyLocked        bool                            `json:"topologyLocked,omitempty"`
	Conditions            []metav1.Condition              `json:"conditions,omitempty"`
}

// DataguardResolvedMemberStatus captures the DG controller's resolved/observed view of one member.
type DataguardResolvedMemberStatus struct {
	Name          string `json:"name,omitempty"`
	Role          string `json:"role,omitempty"`
	DBUniqueName  string `json:"dbUniqueName,omitempty"`
	ConnectString string `json:"connectString,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Message       string `json:"message,omitempty"`
}

// DataguardAuthWalletStatus captures the broker-managed auth wallet lifecycle state.
type DataguardAuthWalletStatus struct {
	Initialized                 bool   `json:"initialized,omitempty"`
	Phase                       string `json:"phase,omitempty"`
	WalletSecretName            string `json:"walletSecretName,omitempty"`
	GeneratedPasswordSecretName string `json:"generatedPasswordSecretName,omitempty"`
	ObservedRebuildToken        string `json:"observedRebuildToken,omitempty"`
	Message                     string `json:"message,omitempty"`
}
