package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type dataguardBrokerReconcilePath string

const (
	dataguardBrokerPathLegacy   dataguardBrokerReconcilePath = "legacy"
	dataguardBrokerPathTopology dataguardBrokerReconcilePath = "topology"
)

const (
	dataguardBrokerPhaseResolving  = "Resolving"
	dataguardBrokerPhaseValidating = "Validating"
	dataguardBrokerPhaseProvision  = "Provision"
	dataguardBrokerPhaseFSFO       = "FastStartFailover"
	dataguardBrokerPhaseRuntime    = "ExecutionRuntime"
	dataguardBrokerPhaseRunner     = "Runner"
	dataguardBrokerPhaseSwitchover = "Switchover"
	dataguardBrokerPhaseReady      = "Ready"
	dataguardBrokerPhaseDeleting   = "Deleting"
)

const (
	dataguardBrokerConditionReady            = "Ready"
	dataguardBrokerConditionReconciling      = "Reconciling"
	dataguardBrokerConditionDegraded         = "Degraded"
	dataguardBrokerConditionExecutionRuntime = "ExecutionRuntimeResolved"
	dataguardBrokerConditionRunnerReady      = "RunnerReady"
	dataguardBrokerConditionTopologyResolved = "TopologyResolved"
	dataguardBrokerConditionBrokerConfigured = "BrokerConfigured"
)

const (
	dataguardBrokerDefaultExecutionWalletMountPath = "/opt/oracle/dg-wallet"
	dataguardBrokerDefaultExecutionTNSAdminPath    = "/opt/oracle/dg-net"
	dataguardBrokerDefaultAuthWalletMountPath      = "/opt/oracle/dg-auth-wallet"
)

type dataguardBrokerDesiredSpec struct {
	Path                    dataguardBrokerReconcilePath
	PrimaryDatabaseRef      string
	StandbyDatabaseRefs     []string
	ProtectionMode          string
	FastStartFailover       bool
	LoadBalancer            bool
	ServiceAnnotations      map[string]string
	TopologyHash            string
	ResolvedMembers         []dbapi.DataguardResolvedMemberStatus
	ObservedPairs           []dbapi.DataguardPairStatus
	SupportsLegacyExecution bool
	CompatibilityMessage    string
}

type dataguardBrokerExecutionRuntime struct {
	Image                string
	ImagePullSecrets     []string
	WalletMountPath      string
	TNSAdminPath         string
	AuthWallet           *dbapi.DataguardAuthWalletSpec
	AuthWalletSecretName string
	AuthWalletMountPath  string
	Source               string
}

type dataguardExecutionCandidate struct {
	Status dbapi.DataguardExecutionStatus
	Source string
}

func (r *dataguardBrokerExecutionRuntime) authWalletEnabled() bool {
	return r != nil && r.AuthWallet != nil && r.AuthWallet.Enabled
}

func (r *dataguardBrokerExecutionRuntime) usesAuthWallet() bool {
	return r.authWalletEnabled() && strings.TrimSpace(r.AuthWalletSecretName) != ""
}

func (d dataguardBrokerDesiredSpec) databaseRefs() []string {
	refs := make([]string, 0, 1+len(d.StandbyDatabaseRefs))
	if strings.TrimSpace(d.PrimaryDatabaseRef) != "" {
		refs = append(refs, strings.TrimSpace(d.PrimaryDatabaseRef))
	}
	for _, ref := range d.StandbyDatabaseRefs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	return refs
}

func (d dataguardBrokerDesiredSpec) currentPrimaryDatabaseRef(broker *dbapi.DataguardBroker) string {
	if broker != nil {
		currentSID := strings.ToUpper(strings.TrimSpace(broker.Status.PrimaryDatabase))
		if currentSID != "" && len(broker.Status.DatabasesInDataguardConfig) > 0 {
			if ref := strings.TrimSpace(broker.Status.DatabasesInDataguardConfig[currentSID]); ref != "" {
				return ref
			}
		}
	}
	return strings.TrimSpace(d.PrimaryDatabaseRef)
}

type dataguardBrokerReconcileScope struct {
	req            ctrl.Request
	log            logr.Logger
	broker         *dbapi.DataguardBroker
	originalStatus dbapi.DataguardBrokerStatus
	desired        *dataguardBrokerDesiredSpec
}

func newDataguardBrokerReconcileScope(req ctrl.Request, log logr.Logger, broker *dbapi.DataguardBroker) *dataguardBrokerReconcileScope {
	return &dataguardBrokerReconcileScope{
		req:            req,
		log:            log,
		broker:         broker,
		originalStatus: cloneDataguardBrokerStatus(broker.Status),
	}
}

func (s *dataguardBrokerReconcileScope) phaseLog(phase string) logr.Logger {
	path := string(dataguardBrokerPathLegacy)
	if s.desired != nil {
		path = string(s.desired.Path)
	}
	return s.log.WithValues("phase", phase, "path", path)
}

func (s *dataguardBrokerReconcileScope) initializeDefaults() {
	if strings.TrimSpace(s.broker.Status.Status) == "" {
		s.broker.Status.Status = dbcommons.StatusCreating
	}
	if strings.TrimSpace(s.broker.Status.ExternalConnectString) == "" {
		s.broker.Status.ExternalConnectString = dbcommons.ValueUnavailable
	}
	if strings.TrimSpace(s.broker.Status.ClusterConnectString) == "" {
		s.broker.Status.ClusterConnectString = dbcommons.ValueUnavailable
	}
	if strings.TrimSpace(s.broker.Status.FastStartFailover) == "" {
		s.broker.Status.FastStartFailover = "false"
	}
	if s.broker.Status.DatabasesInDataguardConfig == nil {
		s.broker.Status.DatabasesInDataguardConfig = map[string]string{}
	}
}

func (s *dataguardBrokerReconcileScope) applyDesiredStatus() {
	if s.desired == nil {
		return
	}
	if s.desired.Path == dataguardBrokerPathTopology {
		s.broker.Status.ObservedTopologyHash = s.desired.TopologyHash
		s.broker.Status.ResolvedMembers = append([]dbapi.DataguardResolvedMemberStatus(nil), s.desired.ResolvedMembers...)
		s.broker.Status.ObservedPairs = append([]dbapi.DataguardPairStatus(nil), s.desired.ObservedPairs...)
		return
	}
	s.broker.Status.ObservedTopologyHash = ""
	s.broker.Status.ResolvedMembers = nil
	s.broker.Status.ObservedPairs = nil
	clearDataguardBrokerCondition(&s.broker.Status.Conditions, dataguardBrokerConditionTopologyResolved)
	clearDataguardBrokerCondition(&s.broker.Status.Conditions, dataguardBrokerConditionExecutionRuntime)
	clearDataguardBrokerCondition(&s.broker.Status.Conditions, dataguardBrokerConditionRunnerReady)
	clearDataguardBrokerCondition(&s.broker.Status.Conditions, dataguardBrokerConditionBrokerConfigured)
}

func (s *dataguardBrokerReconcileScope) markReconciling(phase, reason, message string) {
	s.broker.Status.Status = dbcommons.StatusCreating
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReconciling,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	s.phaseLog(phase).Info(message)
}

func (s *dataguardBrokerReconcileScope) markWaiting(phase, reason, message string) {
	s.broker.Status.Status = dbcommons.StatusNotReady
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReconciling,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	s.phaseLog(phase).Info(message)
}

func (s *dataguardBrokerReconcileScope) markReady(phase, reason, message string) {
	s.broker.Status.Status = dbcommons.StatusReady
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReconciling,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	s.phaseLog(phase).Info(message)
}

func (s *dataguardBrokerReconcileScope) markError(phase, reason, message string, err error) {
	s.broker.Status.Status = dbcommons.StatusError
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionReconciling,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionDegraded,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
	if err != nil {
		s.phaseLog(phase).Error(err, message)
		return
	}
	s.phaseLog(phase).Info(message)
}

func (s *dataguardBrokerReconcileScope) setTopologyResolvedCondition(ready bool, reason, message string) {
	if s.desired == nil || s.desired.Path != dataguardBrokerPathTopology {
		return
	}
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               dataguardBrokerConditionTopologyResolved,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
}

func (s *dataguardBrokerReconcileScope) setCondition(conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&s.broker.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.broker.Generation,
	})
}

func (s *dataguardBrokerReconcileScope) writeStatus(ctx context.Context, r *DataguardBrokerReconciler) error {
	if reflect.DeepEqual(s.originalStatus, s.broker.Status) {
		return nil
	}
	return r.Status().Update(ctx, s.broker)
}

func resolveDataguardBrokerDesiredSpec(broker *dbapi.DataguardBroker) dataguardBrokerDesiredSpec {
	if broker == nil {
		return dataguardBrokerDesiredSpec{Path: dataguardBrokerPathLegacy}
	}
	if broker.Spec.Topology == nil {
		return dataguardBrokerDesiredSpec{
			Path:               dataguardBrokerPathLegacy,
			PrimaryDatabaseRef: strings.TrimSpace(broker.Spec.PrimaryDatabaseRef),
			StandbyDatabaseRefs: append([]string(nil),
				broker.Spec.StandbyDatabaseRefs...),
			ProtectionMode:          strings.TrimSpace(broker.Spec.ProtectionMode),
			FastStartFailover:       broker.Spec.FastStartFailover,
			LoadBalancer:            broker.Spec.LoadBalancer,
			ServiceAnnotations:      cloneStringMap(broker.Spec.ServiceAnnotations),
			SupportsLegacyExecution: true,
		}
	}

	desired := dataguardBrokerDesiredSpec{
		Path:               dataguardBrokerPathTopology,
		LoadBalancer:       broker.Spec.LoadBalancer,
		ServiceAnnotations: cloneStringMap(broker.Spec.ServiceAnnotations),
	}
	if broker.Spec.Topology.Policy != nil {
		desired.ProtectionMode = strings.TrimSpace(broker.Spec.Topology.Policy.ProtectionMode)
		desired.FastStartFailover = broker.Spec.Topology.Policy.FastStartFailover
	}

	memberByName := make(map[string]dbapi.DataguardTopologyMember, len(broker.Spec.Topology.Members))
	unsupportedReasons := make([]string, 0)
	for i := range broker.Spec.Topology.Members {
		member := broker.Spec.Topology.Members[i]
		memberByName[strings.ToLower(strings.TrimSpace(member.Name))] = member
		resolved := dbapi.DataguardResolvedMemberStatus{
			Name:         strings.TrimSpace(member.Name),
			Role:         strings.ToUpper(strings.TrimSpace(member.Role)),
			DBUniqueName: strings.TrimSpace(member.DBUniqueName),
			Phase:        "Resolved",
		}
		endpoint, protocol := selectPreferredDataguardEndpoint(member.Endpoints)
		if endpoint != nil {
			resolved.ConnectString = formatDataguardEndpointConnectString(endpoint)
		}
		if member.LocalRef != nil {
			kind := strings.TrimSpace(member.LocalRef.Kind)
			switch {
			case kind == "", strings.EqualFold(kind, "SingleInstanceDatabase"):
				localRefName := strings.TrimSpace(member.LocalRef.Name)
				if localRefName == "" {
					resolved.Phase = "Pending"
					resolved.Message = "localRef.name is empty"
				} else {
					if strings.EqualFold(resolved.Role, "PRIMARY") {
						desired.PrimaryDatabaseRef = localRefName
					} else if strings.EqualFold(resolved.Role, "PHYSICAL_STANDBY") {
						desired.StandbyDatabaseRefs = append(desired.StandbyDatabaseRefs, localRefName)
					} else {
						unsupportedReasons = append(unsupportedReasons, fmt.Sprintf("member %s role %s is not yet executable through the legacy SIDB path", resolved.Name, resolved.Role))
					}
				}
			default:
				unsupportedReasons = append(unsupportedReasons, fmt.Sprintf("member %s kind %s is not yet supported by the broker controller", resolved.Name, kind))
			}
		} else {
			unsupportedReasons = append(unsupportedReasons, fmt.Sprintf("member %s is external and requires topology-native broker execution", resolved.Name))
		}
		if member.TCPS != nil && member.TCPS.Enabled {
			unsupportedReasons = append(unsupportedReasons, fmt.Sprintf("member %s enables TCPS and requires topology-native broker execution", resolved.Name))
		}
		if protocol == "TCPS" {
			unsupportedReasons = append(unsupportedReasons, fmt.Sprintf("member %s resolves to a TCPS endpoint and requires topology-native broker execution", resolved.Name))
		}
		desired.ResolvedMembers = append(desired.ResolvedMembers, resolved)
	}

	for i := range broker.Spec.Topology.Pairs {
		pair := broker.Spec.Topology.Pairs[i]
		state := "Resolved"
		message := "resolved from spec.topology"
		if _, ok := memberByName[strings.ToLower(strings.TrimSpace(pair.Primary))]; !ok {
			state = "Pending"
			message = "primary member not found during reconciliation"
		}
		if _, ok := memberByName[strings.ToLower(strings.TrimSpace(pair.Standby))]; !ok {
			state = "Pending"
			message = "standby member not found during reconciliation"
		}
		desired.ObservedPairs = append(desired.ObservedPairs, dbapi.DataguardPairStatus{
			Primary: strings.TrimSpace(pair.Primary),
			Standby: strings.TrimSpace(pair.Standby),
			State:   state,
			Message: message,
		})
	}

	desired.TopologyHash = computeDataguardTopologyHash(broker.Spec.Topology)
	sort.Strings(desired.StandbyDatabaseRefs)
	if desired.PrimaryDatabaseRef != "" && len(desired.StandbyDatabaseRefs) > 0 && len(unsupportedReasons) == 0 {
		desired.SupportsLegacyExecution = true
		desired.CompatibilityMessage = "topology projected to local SIDB execution path"
	} else {
		desired.CompatibilityMessage = strings.Join(uniqueSortedStrings(unsupportedReasons), "; ")
	}
	if desired.CompatibilityMessage == "" {
		desired.CompatibilityMessage = "topology projected to local SIDB execution path"
	}
	return desired
}

func selectPreferredDataguardEndpoint(endpoints []dbapi.DataguardEndpointSpec) (*dbapi.DataguardEndpointSpec, string) {
	var tcp *dbapi.DataguardEndpointSpec
	for i := range endpoints {
		endpoint := &endpoints[i]
		protocol := strings.ToUpper(strings.TrimSpace(endpoint.Protocol))
		if protocol == "TCPS" {
			return endpoint, protocol
		}
		if protocol == "TCP" && tcp == nil {
			tcp = endpoint
		}
	}
	if tcp != nil {
		return tcp, "TCP"
	}
	return nil, ""
}

func formatDataguardEndpointConnectString(endpoint *dbapi.DataguardEndpointSpec) string {
	if endpoint == nil {
		return ""
	}
	host := strings.TrimSpace(endpoint.Host)
	service := strings.TrimSpace(endpoint.ServiceName)
	if host == "" || service == "" || endpoint.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d/%s", host, endpoint.Port, service)
}

func computeDataguardTopologyHash(topology *dbapi.DataguardTopologySpec) string {
	if topology == nil {
		return ""
	}
	canonical := *topology.DeepCopy()
	sort.Slice(canonical.Members, func(i, j int) bool {
		return canonical.Members[i].Name < canonical.Members[j].Name
	})
	sort.Slice(canonical.Pairs, func(i, j int) bool {
		if canonical.Pairs[i].Primary != canonical.Pairs[j].Primary {
			return canonical.Pairs[i].Primary < canonical.Pairs[j].Primary
		}
		if canonical.Pairs[i].Standby != canonical.Pairs[j].Standby {
			return canonical.Pairs[i].Standby < canonical.Pairs[j].Standby
		}
		return canonical.Pairs[i].Type < canonical.Pairs[j].Type
	})
	payload, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func cloneDataguardBrokerStatus(in dbapi.DataguardBrokerStatus) dbapi.DataguardBrokerStatus {
	out := in
	if in.DatabasesInDataguardConfig != nil {
		out.DatabasesInDataguardConfig = cloneStringMap(in.DatabasesInDataguardConfig)
	}
	out.ResolvedMembers = append([]dbapi.DataguardResolvedMemberStatus(nil), in.ResolvedMembers...)
	out.ObservedPairs = append([]dbapi.DataguardPairStatus(nil), in.ObservedPairs...)
	out.Conditions = append([]metav1.Condition(nil), in.Conditions...)
	if in.AuthWallet != nil {
		out.AuthWallet = in.AuthWallet.DeepCopy()
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func clearDataguardBrokerCondition(conditions *[]metav1.Condition, conditionType string) {
	if conditions == nil || len(*conditions) == 0 {
		return
	}
	filtered := (*conditions)[:0]
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			continue
		}
		filtered = append(filtered, (*conditions)[i])
	}
	*conditions = filtered
}

func resolveDataguardBrokerExecutionRuntime(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker) (*dataguardBrokerExecutionRuntime, bool, string, error) {
	if broker == nil || broker.Spec.Topology == nil {
		return nil, true, "", nil
	}

	if ready, message := validateDataguardTopologyWallets(broker.Spec.Topology); !ready {
		return nil, false, message, nil
	}

	execution := &dataguardBrokerExecutionRuntime{
		WalletMountPath:     dataguardBrokerDefaultExecutionWalletMountPath,
		TNSAdminPath:        dataguardBrokerDefaultExecutionTNSAdminPath,
		AuthWalletMountPath: dataguardBrokerDefaultAuthWalletMountPath,
	}
	if broker.Spec.Execution != nil {
		if mount := strings.TrimSpace(broker.Spec.Execution.WalletMountPath); mount != "" {
			execution.WalletMountPath = mount
		}
		if path := strings.TrimSpace(broker.Spec.Execution.TNSAdminPath); path != "" {
			execution.TNSAdminPath = path
		}
		execution.AuthWallet = broker.Spec.Execution.AuthWallet.DeepCopy()
		if image := strings.TrimSpace(broker.Spec.Execution.Image); image != "" {
			if execution.AuthWallet != nil && execution.AuthWallet.Enabled && broker.Status.AuthWallet != nil {
				execution.AuthWalletSecretName = strings.TrimSpace(broker.Status.AuthWallet.WalletSecretName)
			}
			execution.Image = image
			execution.ImagePullSecrets = uniqueSortedStrings(broker.Spec.Execution.ImagePullSecrets)
			execution.Source = "spec.execution"
			return execution, true, "resolved execution runtime from spec.execution", nil
		}
	}

	candidates, err := collectDataguardExecutionCandidates(ctx, r, broker)
	if err != nil {
		return nil, false, "", err
	}
	if len(candidates) == 0 {
		return nil, false, "execution image is not set and no producer published a default dataguard execution image", nil
	}
	if len(candidates) > 1 {
		sources := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			sources = append(sources, candidate.Source)
		}
		sort.Strings(sources)
		return nil, false, fmt.Sprintf("multiple producer execution images were found (%s); set spec.execution.image explicitly", strings.Join(sources, ", ")), nil
	}

	execution.Image = candidates[0].Status.Image
	execution.ImagePullSecrets = uniqueSortedStrings(candidates[0].Status.ImagePullSecrets)
	execution.AuthWallet = candidates[0].Status.AuthWallet.DeepCopy()
	if execution.AuthWallet != nil && execution.AuthWallet.Enabled && broker.Status.AuthWallet != nil {
		execution.AuthWalletSecretName = strings.TrimSpace(broker.Status.AuthWallet.WalletSecretName)
	}
	execution.Source = candidates[0].Source
	return execution, true, fmt.Sprintf("resolved execution runtime from %s", execution.Source), nil
}

func validateDataguardTopologyWallets(topology *dbapi.DataguardTopologySpec) (bool, string) {
	if topology == nil {
		return true, ""
	}
	for i := range topology.Members {
		member := topology.Members[i]
		if member.TCPS != nil && member.TCPS.Enabled && dbapi.ResolveDataguardTopologyMemberClientWalletSecret(topology, &member) == "" {
			return false, fmt.Sprintf("member %s enables TCPS but does not publish tcps.clientWalletSecret", strings.TrimSpace(member.Name))
		}
	}
	return true, ""
}

func collectDataguardExecutionCandidates(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker) ([]dataguardExecutionCandidate, error) {
	if broker == nil || broker.Spec.Topology == nil {
		return nil, nil
	}
	candidateByKey := map[string]dataguardExecutionCandidate{}
	if ref := broker.Spec.Topology.SourceRef; ref != nil {
		if candidate, ok, err := fetchDataguardExecutionStatusFromRef(ctx, r, broker.Namespace, ref.APIVersion, ref.Kind, ref.Namespace, ref.Name); err != nil {
			return nil, err
		} else if ok {
			candidateByKey[executionCandidateKey(candidate.Status)] = candidate
		}
	}
	for i := range broker.Spec.Topology.Members {
		member := broker.Spec.Topology.Members[i]
		if member.LocalRef == nil {
			continue
		}
		candidate, ok, err := fetchDataguardExecutionStatusFromRef(ctx, r, broker.Namespace, member.LocalRef.APIVersion, member.LocalRef.Kind, member.LocalRef.Namespace, member.LocalRef.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			candidateByKey[executionCandidateKey(candidate.Status)] = candidate
		}
	}
	if len(candidateByKey) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(candidateByKey))
	for key := range candidateByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	candidates := make([]dataguardExecutionCandidate, 0, len(keys))
	for _, key := range keys {
		candidates = append(candidates, candidateByKey[key])
	}
	return candidates, nil
}

func fetchDataguardExecutionStatusFromRef(ctx context.Context, r *DataguardBrokerReconciler, defaultNamespace, apiVersion, kind, namespace, name string) (dataguardExecutionCandidate, bool, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = strings.TrimSpace(defaultNamespace)
	}
	refName := strings.TrimSpace(name)
	if ns == "" || refName == "" {
		return dataguardExecutionCandidate{}, false, nil
	}
	switch strings.TrimSpace(kind) {
	case "", "SingleInstanceDatabase":
		var sidb dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: refName}, &sidb); err != nil {
			return dataguardExecutionCandidate{}, false, clientIgnoreNotFound(err)
		}
		status, ok := dataguardExecutionStatusFromRenderedBrokerStatus(sidb.Status.Dataguard)
		if !ok {
			return dataguardExecutionCandidate{}, false, nil
		}
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("SingleInstanceDatabase/%s/%s", ns, refName)}, true, nil
	case "ShardingDatabase":
		var sharding dbapi.ShardingDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: refName}, &sharding); err != nil {
			return dataguardExecutionCandidate{}, false, clientIgnoreNotFound(err)
		}
		status, ok := shardingDataguardExecutionStatusFromRenderedBrokerStatus(sharding.Status.Dataguard)
		if !ok {
			return dataguardExecutionCandidate{}, false, nil
		}
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("ShardingDatabase/%s/%s", ns, refName)}, true, nil
	case "RacDatabase":
		var rac dbapi.RacDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: refName}, &rac); err != nil {
			return dataguardExecutionCandidate{}, false, clientIgnoreNotFound(err)
		}
		status, ok := dataguardExecutionStatusFromRenderedBrokerStatus(rac.Status.Dataguard)
		if !ok {
			return dataguardExecutionCandidate{}, false, nil
		}
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("RacDatabase/%s/%s", ns, refName)}, true, nil
	default:
		_ = apiVersion
		return dataguardExecutionCandidate{}, false, nil
	}
}

func dataguardExecutionStatusFromRenderedBrokerStatus(status *dbapi.ProducerDataguardStatus) (dbapi.DataguardExecutionStatus, bool) {
	if status == nil || status.RenderedBrokerSpec == nil || status.RenderedBrokerSpec.Spec == nil || status.RenderedBrokerSpec.Spec.Execution == nil {
		return dbapi.DataguardExecutionStatus{}, false
	}
	execution := status.RenderedBrokerSpec.Spec.Execution
	if strings.TrimSpace(execution.Image) == "" {
		return dbapi.DataguardExecutionStatus{}, false
	}
	return dbapi.DataguardExecutionStatus{
		Image:            strings.TrimSpace(execution.Image),
		ImagePullSecrets: append([]string(nil), execution.ImagePullSecrets...),
		AuthWallet:       execution.AuthWallet.DeepCopy(),
	}, true
}

func shardingDataguardExecutionStatusFromRenderedBrokerStatus(status *dbapi.ShardingDataguardStatus) (dbapi.DataguardExecutionStatus, bool) {
	if status == nil || status.RenderedBrokerSpec == nil || status.RenderedBrokerSpec.Spec == nil || status.RenderedBrokerSpec.Spec.Execution == nil {
		return dbapi.DataguardExecutionStatus{}, false
	}
	execution := status.RenderedBrokerSpec.Spec.Execution
	if strings.TrimSpace(execution.Image) == "" {
		return dbapi.DataguardExecutionStatus{}, false
	}
	return dbapi.DataguardExecutionStatus{
		Image:            strings.TrimSpace(execution.Image),
		ImagePullSecrets: append([]string(nil), execution.ImagePullSecrets...),
		AuthWallet:       execution.AuthWallet.DeepCopy(),
	}, true
}

func clientIgnoreNotFound(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func executionCandidateKey(candidate dbapi.DataguardExecutionStatus) string {
	authWalletKey := ""
	if candidate.AuthWallet != nil {
		secretName := ""
		secretKey := ""
		if candidate.AuthWallet.PasswordSecretRef != nil {
			secretName = strings.TrimSpace(candidate.AuthWallet.PasswordSecretRef.SecretName)
			secretKey = strings.TrimSpace(candidate.AuthWallet.PasswordSecretRef.SecretKey)
		}
		authWalletKey = fmt.Sprintf("%t|%s|%s|%s",
			candidate.AuthWallet.Enabled,
			strings.TrimSpace(candidate.AuthWallet.RebuildToken),
			secretName,
			secretKey)
	}
	return strings.TrimSpace(candidate.Image) + "|" + strings.Join(uniqueSortedStrings(candidate.ImagePullSecrets), ",") + "|" + authWalletKey
}

func listDataguardBrokerRunnerWalletSecrets(broker *dbapi.DataguardBroker) []string {
	if broker == nil || broker.Spec.Topology == nil {
		return nil
	}
	secrets := make([]string, 0, len(broker.Spec.Topology.Members))
	seen := map[string]struct{}{}
	for i := range broker.Spec.Topology.Members {
		member := broker.Spec.Topology.Members[i]
		if member.TCPS == nil || !member.TCPS.Enabled {
			continue
		}
		secretName := strings.TrimSpace(dbapi.ResolveDataguardTopologyMemberClientWalletSecret(broker.Spec.Topology, &member))
		if secretName == "" {
			continue
		}
		if _, ok := seen[secretName]; ok {
			continue
		}
		seen[secretName] = struct{}{}
		secrets = append(secrets, secretName)
	}
	sort.Strings(secrets)
	return secrets
}

func dataguardBrokerRunnerPodSecretNamesByPrefix(pod *corev1.Pod, prefix string) []string {
	if pod == nil {
		return nil
	}
	names := make([]string, 0, len(pod.Spec.Volumes))
	for _, volume := range pod.Spec.Volumes {
		if !strings.HasPrefix(volume.Name, prefix) {
			continue
		}
		if volume.Secret == nil {
			continue
		}
		if trimmed := strings.TrimSpace(volume.Secret.SecretName); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return uniqueSortedStrings(names)
}

func dataguardBrokerRunnerContainerMountPath(pod *corev1.Pod, volumeName string) string {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return ""
	}
	for _, mount := range pod.Spec.Containers[0].VolumeMounts {
		if mount.Name == volumeName {
			return strings.TrimSpace(mount.MountPath)
		}
	}
	return ""
}

func dataguardBrokerRunnerRecreateReasons(pod *corev1.Pod, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, desiredHash string) []string {
	reasons := make([]string, 0, 8)
	if pod == nil || runtime == nil {
		return []string{"runner runtime is incomplete"}
	}
	currentHash := strings.TrimSpace(pod.Labels["database.oracle.com/runtime-hash"])
	if currentHash != strings.TrimSpace(desiredHash) {
		reasons = append(reasons, fmt.Sprintf("runtime hash changed (%s -> %s)", firstNonEmptyString(currentHash, "<none>"), firstNonEmptyString(strings.TrimSpace(desiredHash), "<none>")))
	}
	currentImage := ""
	if len(pod.Spec.Containers) > 0 {
		currentImage = strings.TrimSpace(pod.Spec.Containers[0].Image)
	}
	if currentImage != strings.TrimSpace(runtime.Image) {
		reasons = append(reasons, fmt.Sprintf("execution image changed (%s -> %s)", firstNonEmptyString(currentImage, "<none>"), firstNonEmptyString(strings.TrimSpace(runtime.Image), "<none>")))
	}
	currentTopologyHash := strings.TrimSpace(pod.Annotations["database.oracle.com/topology-hash"])
	desiredTopologyHash := ""
	if broker != nil {
		desiredTopologyHash = strings.TrimSpace(broker.Status.ObservedTopologyHash)
	}
	if currentTopologyHash != desiredTopologyHash {
		reasons = append(reasons, fmt.Sprintf("topology hash changed (%s -> %s)", firstNonEmptyString(currentTopologyHash, "<none>"), firstNonEmptyString(desiredTopologyHash, "<none>")))
	}
	currentAuthWallet := strings.TrimSpace(pod.Annotations["database.oracle.com/auth-wallet-secret"])
	desiredAuthWallet := strings.TrimSpace(runtime.AuthWalletSecretName)
	if currentAuthWallet != desiredAuthWallet {
		reasons = append(reasons, fmt.Sprintf("auth wallet secret changed (%s -> %s)", firstNonEmptyString(currentAuthWallet, "<none>"), firstNonEmptyString(desiredAuthWallet, "<none>")))
	}
	currentImagePullSecrets := make([]string, 0, len(pod.Spec.ImagePullSecrets))
	for _, secret := range pod.Spec.ImagePullSecrets {
		currentImagePullSecrets = append(currentImagePullSecrets, strings.TrimSpace(secret.Name))
	}
	desiredImagePullSecrets := uniqueSortedStrings(runtime.ImagePullSecrets)
	if !reflect.DeepEqual(uniqueSortedStrings(currentImagePullSecrets), desiredImagePullSecrets) {
		reasons = append(reasons, fmt.Sprintf("imagePullSecrets changed (%v -> %v)", uniqueSortedStrings(currentImagePullSecrets), desiredImagePullSecrets))
	}
	currentWalletMountPath := dataguardBrokerRunnerContainerMountPath(pod, "auth-wallet")
	desiredWalletMountPath := strings.TrimSpace(runtime.AuthWalletMountPath)
	if currentWalletMountPath != desiredWalletMountPath && (currentWalletMountPath != "" || desiredWalletMountPath != "") {
		reasons = append(reasons, fmt.Sprintf("auth wallet mount path changed (%s -> %s)", firstNonEmptyString(currentWalletMountPath, "<none>"), firstNonEmptyString(desiredWalletMountPath, "<none>")))
	}
	currentTNSAdminPath := dataguardBrokerRunnerContainerMountPath(pod, "tns-admin")
	desiredTNSAdminPath := strings.TrimSpace(runtime.TNSAdminPath)
	if currentTNSAdminPath != desiredTNSAdminPath {
		reasons = append(reasons, fmt.Sprintf("TNS admin path changed (%s -> %s)", firstNonEmptyString(currentTNSAdminPath, "<none>"), firstNonEmptyString(desiredTNSAdminPath, "<none>")))
	}
	currentTopologyWallets := dataguardBrokerRunnerPodSecretNamesByPrefix(pod, "wallet-")
	desiredTopologyWallets := listDataguardBrokerRunnerWalletSecrets(broker)
	if !reflect.DeepEqual(currentTopologyWallets, desiredTopologyWallets) {
		reasons = append(reasons, fmt.Sprintf("TCPS wallet secrets changed (%v -> %v)", currentTopologyWallets, desiredTopologyWallets))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "runner spec no longer matches desired runtime")
	}
	return reasons
}

func logDataguardBrokerRunnerCreation(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, desiredPodName, desiredHash string) {
	if r == nil || broker == nil || runtime == nil {
		return
	}
	pods, err := listDataguardBrokerRunnerPods(ctx, r.Client, broker)
	if err != nil {
		r.Log.WithValues("desiredRunnerPod", desiredPodName, "desiredRuntimeHash", desiredHash).
			Info("creating topology execution runner pod", "reason", "unable to list existing runner pods", "listError", err.Error())
		return
	}
	if len(pods) == 0 {
		r.Log.WithValues("desiredRunnerPod", desiredPodName, "desiredRuntimeHash", desiredHash).
			Info("creating topology execution runner pod", "reason", "initial runner bootstrap")
		return
	}

	staleSummaries := make([]map[string]interface{}, 0, len(pods))
	for i := range pods {
		pod := &pods[i]
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if strings.TrimSpace(pod.Labels["database.oracle.com/runtime-hash"]) == strings.TrimSpace(desiredHash) {
			continue
		}
		staleSummaries = append(staleSummaries, map[string]interface{}{
			"runnerPod": pod.Name,
			"reasons":   dataguardBrokerRunnerRecreateReasons(pod, broker, runtime, desiredHash),
		})
	}
	if len(staleSummaries) == 0 {
		r.Log.WithValues("desiredRunnerPod", desiredPodName, "desiredRuntimeHash", desiredHash).
			Info("creating topology execution runner pod", "reason", "desired runner pod does not exist yet")
		return
	}

	r.Log.WithValues("desiredRunnerPod", desiredPodName, "desiredRuntimeHash", desiredHash).
		Info("creating topology execution runner pod", "replaces", staleSummaries)
}

func computeDataguardBrokerRunnerRuntimeHash(broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) string {
	if runtime == nil {
		return ""
	}
	topologyHash := ""
	if broker != nil {
		topologyHash = strings.TrimSpace(broker.Status.ObservedTopologyHash)
	}
	candidate := dbapi.DataguardExecutionStatus{
		Image:            strings.TrimSpace(runtime.Image),
		ImagePullSecrets: append([]string(nil), runtime.ImagePullSecrets...),
	}
	if runtime.AuthWallet != nil {
		candidate.AuthWallet = runtime.AuthWallet.DeepCopy()
	}
	payload := map[string]interface{}{
		"executionKey":     executionCandidateKey(candidate),
		"topologyHash":     topologyHash,
		"authWalletSecret": strings.TrimSpace(runtime.AuthWalletSecretName),
		"walletMountPath":  strings.TrimSpace(runtime.WalletMountPath),
		"tnsAdminPath":     strings.TrimSpace(runtime.TNSAdminPath),
		"authWalletMount":  strings.TrimSpace(runtime.AuthWalletMountPath),
		"topologyWallets":  listDataguardBrokerRunnerWalletSecrets(broker),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v", payload)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func dataguardBrokerRunnerPodBaseName(broker *dbapi.DataguardBroker) string {
	if broker == nil {
		return "dataguard-runner"
	}
	return sanitizeDataguardRunnerName(broker.Name, "dataguard") + "-runner"
}

func dataguardBrokerRunnerPodNameForHash(broker *dbapi.DataguardBroker, runtimeHash string) string {
	base := dataguardBrokerRunnerPodBaseName(broker)
	if runtimeHash == "" {
		return base
	}
	suffix := sanitizeDataguardRunnerName(runtimeHash, "runtime")
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	maxBaseLen := 63 - len(suffix) - 1
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = strings.TrimRight(base[:maxBaseLen], "-")
		if base == "" {
			base = "runner"
		}
	}
	return base + "-" + suffix
}

func listDataguardBrokerRunnerPods(ctx context.Context, r client.Client, broker *dbapi.DataguardBroker) ([]corev1.Pod, error) {
	if broker == nil {
		return nil, nil
	}
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(broker.Namespace),
		client.MatchingLabels{
			"database.oracle.com/dataguard-broker": broker.Name,
			"database.oracle.com/component":        "execution-runner",
		},
	); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func cleanupStaleDataguardBrokerRunnerPods(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desiredPodName, desiredHash string) error {
	pods, err := listDataguardBrokerRunnerPods(ctx, r.Client, broker)
	if err != nil {
		return err
	}
	runtime, _, _, runtimeErr := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	for i := range pods {
		pod := &pods[i]
		if pod.Name == desiredPodName {
			continue
		}
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if strings.TrimSpace(pod.Labels["database.oracle.com/runtime-hash"]) == strings.TrimSpace(desiredHash) {
			continue
		}
		reasons := []string{"stale runner pod cleanup"}
		if runtimeErr == nil {
			reasons = dataguardBrokerRunnerRecreateReasons(pod, broker, runtime, desiredHash)
		}
		r.Log.WithValues("runnerPod", pod.Name, "desiredRunnerPod", desiredPodName, "desiredRuntimeHash", desiredHash).
			Info("deleting stale topology execution runner pod", "reasons", reasons)
		if err := clientIgnoreNotFound(r.Delete(ctx, pod)); err != nil {
			return err
		}
	}
	return nil
}

func ensureDataguardBrokerRunnerPod(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) (bool, string, error) {
	if broker == nil || runtime == nil || strings.TrimSpace(runtime.Image) == "" {
		return false, "execution image is not resolved", nil
	}
	runtimeHash := computeDataguardBrokerRunnerRuntimeHash(broker, runtime)
	podName := dataguardBrokerRunnerPodNameForHash(broker, runtimeHash)
	var existing corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Namespace: broker.Namespace, Name: podName}, &existing); err != nil {
		if !isNotFound(err) {
			return false, "", err
		}
		logDataguardBrokerRunnerCreation(ctx, r, broker, runtime, podName, runtimeHash)
		pod := buildDataguardBrokerRunnerPod(broker, runtime, runtimeHash)
		if err := ctrl.SetControllerReference(broker, pod, r.Scheme); err != nil {
			return false, "", err
		}
		if err := r.Create(ctx, pod); err != nil {
			return false, "", err
		}
		return false, fmt.Sprintf("created topology execution runner pod %s", podName), nil
	}
	switch existing.Status.Phase {
	case corev1.PodRunning:
		if err := cleanupStaleDataguardBrokerRunnerPods(ctx, r, broker, podName, runtimeHash); err != nil {
			return false, "", err
		}
		return true, fmt.Sprintf("topology execution runner pod %s is ready", podName), nil
	case corev1.PodPending:
		return false, fmt.Sprintf("topology execution runner pod %s is pending", podName), nil
	case corev1.PodFailed:
		if err := clientIgnoreNotFound(r.Delete(ctx, &existing)); err != nil {
			return false, "", err
		}
		return false, fmt.Sprintf("topology execution runner pod %s failed; deleting for recreation", podName), nil
	default:
		return false, fmt.Sprintf("topology execution runner pod %s is in phase %s", podName, existing.Status.Phase), nil
	}
}

func buildDataguardBrokerRunnerPod(broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, runtimeHash string) *corev1.Pod {
	podName := dataguardBrokerRunnerPodNameForHash(broker, runtimeHash)
	labels := map[string]string{
		"database.oracle.com/dataguard-broker": broker.Name,
		"database.oracle.com/component":        "execution-runner",
		"database.oracle.com/runtime-hash":     runtimeHash,
	}
	annotations := map[string]string{
		"database.oracle.com/runtime-image": strings.TrimSpace(runtime.Image),
	}
	if broker.Status.ObservedTopologyHash != "" {
		annotations["database.oracle.com/topology-hash"] = broker.Status.ObservedTopologyHash
	}
	if strings.TrimSpace(runtime.AuthWalletSecretName) != "" {
		annotations["database.oracle.com/auth-wallet-secret"] = strings.TrimSpace(runtime.AuthWalletSecretName)
	}

	volumes := []corev1.Volume{{
		Name: "tns-admin",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}}
	volumeMounts := []corev1.VolumeMount{{
		Name:      "tns-admin",
		MountPath: runtime.TNSAdminPath,
	}}
	if strings.TrimSpace(runtime.AuthWalletSecretName) != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "auth-wallet",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: strings.TrimSpace(runtime.AuthWalletSecretName)},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "auth-wallet",
			MountPath: runtime.AuthWalletMountPath,
			ReadOnly:  true,
		})
	}
	seenWallets := map[string]struct{}{}
	if broker.Spec.Topology != nil {
		for i := range broker.Spec.Topology.Members {
			member := broker.Spec.Topology.Members[i]
			if member.TCPS == nil || !member.TCPS.Enabled {
				continue
			}
			secretName := dbapi.ResolveDataguardTopologyMemberClientWalletSecret(broker.Spec.Topology, &member)
			if secretName == "" {
				continue
			}
			if _, ok := seenWallets[secretName]; ok {
				continue
			}
			seenWallets[secretName] = struct{}{}
			volumeName := "wallet-" + sanitizeDataguardRunnerName(secretName, "wallet")
			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: secretName},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: strings.TrimRight(runtime.WalletMountPath, "/") + "/" + sanitizeDataguardRunnerName(secretName, "wallet"),
				ReadOnly:  true,
			})
		}
	}

	imagePullSecrets := make([]corev1.LocalObjectReference, 0, len(runtime.ImagePullSecrets))
	for _, secret := range runtime.ImagePullSecrets {
		if trimmed := strings.TrimSpace(secret); trimmed != "" {
			imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: trimmed})
		}
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   broker.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			NodeSelector:                  cloneStringMap(broker.Spec.NodeSelector),
			ImagePullSecrets:              imagePullSecrets,
			RestartPolicy:                 corev1.RestartPolicyAlways,
			TerminationGracePeriodSeconds: func() *int64 { v := int64(10); return &v }(),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: func() *int64 { v := int64(54321); return &v }(),
				FSGroup:   func() *int64 { v := int64(54321); return &v }(),
			},
			Volumes: volumes,
			Containers: []corev1.Container{{
				Name:            "runner",
				Image:           runtime.Image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"/bin/sh", "-c", "trap : TERM INT; sleep 3600d & wait"},
				Env: []corev1.EnvVar{{
					Name:  "TNS_ADMIN",
					Value: runtime.TNSAdminPath,
				}},
				VolumeMounts: volumeMounts,
			}},
		},
	}
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsNotFound(err)
}

func sanitizeDataguardRunnerName(raw, fallback string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		trimmed = strings.ToLower(strings.TrimSpace(fallback))
	}
	if trimmed == "" {
		return "member"
	}
	var b strings.Builder
	lastDash := false
	for _, ch := range trimmed {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		return "member"
	}
	return name
}
