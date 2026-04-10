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
	Image            string
	ImagePullSecrets []string
	WalletMountPath  string
	TNSAdminPath     string
	Source           string
}

type dataguardExecutionCandidate struct {
	Status dbapi.DataguardExecutionStatus
	Source string
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
		WalletMountPath: dataguardBrokerDefaultExecutionWalletMountPath,
		TNSAdminPath:    dataguardBrokerDefaultExecutionTNSAdminPath,
	}
	if broker.Spec.Execution != nil {
		if mount := strings.TrimSpace(broker.Spec.Execution.WalletMountPath); mount != "" {
			execution.WalletMountPath = mount
		}
		if path := strings.TrimSpace(broker.Spec.Execution.TNSAdminPath); path != "" {
			execution.TNSAdminPath = path
		}
		if image := strings.TrimSpace(broker.Spec.Execution.Image); image != "" {
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
	execution.Source = candidates[0].Source
	return execution, true, fmt.Sprintf("resolved execution runtime from %s", execution.Source), nil
}

func validateDataguardTopologyWallets(topology *dbapi.DataguardTopologySpec) (bool, string) {
	if topology == nil {
		return true, ""
	}
	for i := range topology.Members {
		member := topology.Members[i]
		if member.TCPS != nil && member.TCPS.Enabled && strings.TrimSpace(member.TCPS.ClientWalletSecret) == "" {
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
		if sidb.Status.Dataguard == nil || sidb.Status.Dataguard.Execution == nil || strings.TrimSpace(sidb.Status.Dataguard.Execution.Image) == "" {
			return dataguardExecutionCandidate{}, false, nil
		}
		status := *sidb.Status.Dataguard.Execution
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("SingleInstanceDatabase/%s/%s", ns, refName)}, true, nil
	case "ShardingDatabase":
		var sharding dbapi.ShardingDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: refName}, &sharding); err != nil {
			return dataguardExecutionCandidate{}, false, clientIgnoreNotFound(err)
		}
		if sharding.Status.Dataguard == nil || sharding.Status.Dataguard.Execution == nil || strings.TrimSpace(sharding.Status.Dataguard.Execution.Image) == "" {
			return dataguardExecutionCandidate{}, false, nil
		}
		status := *sharding.Status.Dataguard.Execution
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("ShardingDatabase/%s/%s", ns, refName)}, true, nil
	case "RacDatabase":
		var rac dbapi.RacDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: refName}, &rac); err != nil {
			return dataguardExecutionCandidate{}, false, clientIgnoreNotFound(err)
		}
		if rac.Status.Dataguard == nil || rac.Status.Dataguard.Execution == nil || strings.TrimSpace(rac.Status.Dataguard.Execution.Image) == "" {
			return dataguardExecutionCandidate{}, false, nil
		}
		status := *rac.Status.Dataguard.Execution
		status.ImagePullSecrets = uniqueSortedStrings(status.ImagePullSecrets)
		return dataguardExecutionCandidate{Status: status, Source: fmt.Sprintf("RacDatabase/%s/%s", ns, refName)}, true, nil
	default:
		_ = apiVersion
		return dataguardExecutionCandidate{}, false, nil
	}
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
	return strings.TrimSpace(candidate.Image) + "|" + strings.Join(uniqueSortedStrings(candidate.ImagePullSecrets), ",")
}

func ensureDataguardBrokerRunnerPod(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) (bool, string, error) {
	if broker == nil || runtime == nil || strings.TrimSpace(runtime.Image) == "" {
		return false, "execution image is not resolved", nil
	}
	podName := dataguardBrokerRunnerPodName(broker)
	var existing corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Namespace: broker.Namespace, Name: podName}, &existing); err != nil {
		if !isNotFound(err) {
			return false, "", err
		}
		pod := buildDataguardBrokerRunnerPod(broker, runtime)
		if err := ctrl.SetControllerReference(broker, pod, r.Scheme); err != nil {
			return false, "", err
		}
		if err := r.Create(ctx, pod); err != nil {
			return false, "", err
		}
		return false, "created topology execution runner pod", nil
	}
	if runnerPodNeedsRefresh(&existing, broker, runtime) {
		if err := r.Delete(ctx, &existing); err != nil {
			return false, "", err
		}
		return false, "refreshed topology execution runner pod", nil
	}
	switch existing.Status.Phase {
	case corev1.PodRunning:
		return true, "topology execution runner pod is ready", nil
	case corev1.PodPending:
		return false, "topology execution runner pod is pending", nil
	case corev1.PodFailed:
		return false, "topology execution runner pod failed; deleting for recreation", r.Delete(ctx, &existing)
	default:
		return false, fmt.Sprintf("topology execution runner pod is in phase %s", existing.Status.Phase), nil
	}
}

func buildDataguardBrokerRunnerPod(broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) *corev1.Pod {
	labels := map[string]string{
		"database.oracle.com/dataguard-broker": broker.Name,
		"database.oracle.com/component":        "execution-runner",
	}
	annotations := map[string]string{
		"database.oracle.com/runtime-image": strings.TrimSpace(runtime.Image),
	}
	if broker.Status.ObservedTopologyHash != "" {
		annotations["database.oracle.com/topology-hash"] = broker.Status.ObservedTopologyHash
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
	seenWallets := map[string]struct{}{}
	if broker.Spec.Topology != nil {
		for i := range broker.Spec.Topology.Members {
			member := broker.Spec.Topology.Members[i]
			if member.TCPS == nil || !member.TCPS.Enabled {
				continue
			}
			secretName := strings.TrimSpace(member.TCPS.ClientWalletSecret)
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
			Name:        dataguardBrokerRunnerPodName(broker),
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

func dataguardBrokerRunnerPodName(broker *dbapi.DataguardBroker) string {
	if broker == nil {
		return "dataguard-runner"
	}
	return broker.Name + "-runner"
}

func runnerPodNeedsRefresh(pod *corev1.Pod, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) bool {
	if pod == nil || broker == nil || runtime == nil {
		return false
	}
	if len(pod.Spec.Containers) == 0 || strings.TrimSpace(pod.Spec.Containers[0].Image) != strings.TrimSpace(runtime.Image) {
		return true
	}
	if strings.TrimSpace(pod.Annotations["database.oracle.com/topology-hash"]) != strings.TrimSpace(broker.Status.ObservedTopologyHash) {
		return true
	}
	currentSecrets := make([]string, 0, len(pod.Spec.ImagePullSecrets))
	for _, secret := range pod.Spec.ImagePullSecrets {
		currentSecrets = append(currentSecrets, strings.TrimSpace(secret.Name))
	}
	return !reflect.DeepEqual(uniqueSortedStrings(currentSecrets), uniqueSortedStrings(runtime.ImagePullSecrets))
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
