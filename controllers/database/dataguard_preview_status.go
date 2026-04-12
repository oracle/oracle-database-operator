package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	shardingv1 "github.com/oracle/oracle-database-operator/commons/sharding"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dataguardPreviewPhaseDisabled              = "Disabled"
	dataguardPreviewPhaseNotApplicable         = "NotApplicable"
	dataguardPreviewPhaseWaitingForSource      = "WaitingForPrimarySource"
	dataguardPreviewPhaseWaitingForTopology    = "WaitingForTopology"
	dataguardPreviewPhaseWaitingForUserInput   = "WaitingForUserInput"
	dataguardPreviewPhaseReady                 = "PreviewReady"
	dataguardPreviewPhaseManagedNotImplemented = "ManagedNotImplemented"
	dataguardPreviewDefaultSecretKey           = "oracle_pwd"
	dataguardPreviewExternalSecretPlaceholder  = "replace-with-external-admin-secret"
	dataguardPreviewExternalSecretKey          = "password"
	dataguardPreviewExternalPrimaryWalletPH    = "replace-with-primary-client-wallet-secret"
)

func dataguardPreviewReadyMessage(note string) string {
	msg := "resolved Data Guard topology is ready to be copied into DataguardBroker.spec.topology"
	note = strings.TrimSpace(note)
	if note == "" {
		return msg
	}
	return msg + "; " + note
}

func dataguardProducerMode(spec *dbapi.DataguardProducerSpec) dbapi.DataguardProducerMode {
	return dbapi.EffectiveDataguardProducerMode(spec)
}

func syncSIDBDataguardPreviewStatus(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) {
	if m == nil {
		return
	}

	mode := dataguardProducerMode(m.Spec.Dataguard)
	if mode == dbapi.DataguardProducerModeDisabled {
		m.Status.Dataguard = nil
		return
	}

	current := m.Status.Dataguard
	next := &dbapi.ProducerDataguardStatus{}
	if current != nil {
		next.BrokerRef = current.BrokerRef
		next.Conditions = append([]metav1.Condition(nil), current.Conditions...)
	}

	if mode == dbapi.DataguardProducerModeManaged {
		next.Phase = dataguardPreviewPhaseManagedNotImplemented
		next.ReadyForBroker = false
		setDataguardPreviewCondition(&next.Conditions, m.Generation, false, "ManagedNotImplemented", "Managed mode is reserved for future DataguardBroker automation")
		m.Status.Dataguard = next
		return
	}

	memberRole := sidbPreviewMemberRole(m)
	if memberRole == "" {
		next.Phase = dataguardPreviewPhaseNotApplicable
		next.ReadyForBroker = false
		setDataguardPreviewCondition(&next.Conditions, m.Generation, false, "NotApplicable", "Data Guard preview is not applicable for this createAs mode")
		m.Status.Dataguard = next
		return
	}

	topology, memberName, primaryMemberName, previewMessage, previewReady := buildSIDBPreviewTopology(m, rp)
	next.MemberName = memberName
	next.PrimaryMemberName = primaryMemberName
	next.Role = memberRole
	next.DBUniqueName = strings.ToUpper(strings.TrimSpace(m.Spec.Sid))
	next.Endpoints = buildSIDBPreviewEndpoints(m, m.Name, strings.TrimSpace(m.Spec.Sid))
	next.Execution = buildSIDBPreviewExecutionStatus(m)
	next.TCPS = buildSIDBPreviewTCPSConfig(m)
	next.TopologyLocked = sidbPreviewTopologyLocked(m)
	next.Topology = topology
	next.RenderedBrokerSpec = buildRenderedBrokerPreviewStatus(m.Name, m.Namespace, topology, next.Execution, "", false)
	if topology == nil {
		next.Phase = dataguardPreviewPhaseWaitingForSource
		next.ReadyForBroker = false
		next.TopologyHash = ""
		next.PublishedTopologyHash = ""
		next.LastPublishedTime = nil
		msg := "resolved Data Guard topology is not ready yet"
		if strings.TrimSpace(previewMessage) != "" {
			msg = previewMessage
		}
		setDataguardPreviewCondition(&next.Conditions, m.Generation, false, "WaitingForPrimarySource", msg)
		m.Status.Dataguard = next
		return
	}

	next.TopologyHash = dataguardTopologyHash(topology)
	next.RenderedBrokerSpec = buildRenderedBrokerPreviewStatus(m.Name, m.Namespace, topology, next.Execution, next.TopologyHash, previewReady)
	if previewReady {
		next.Phase = dataguardPreviewPhaseReady
		next.ReadyForBroker = true
		next.PublishedTopologyHash = next.TopologyHash
		now := metav1.Now()
		next.LastPublishedTime = &now
		setDataguardPreviewCondition(&next.Conditions, m.Generation, true, "PreviewReady", dataguardPreviewReadyMessage(previewMessage))
		m.Status.Dataguard = next
		return
	}

	next.Phase = dataguardPreviewPhaseWaitingForUserInput
	next.ReadyForBroker = false
	next.PublishedTopologyHash = ""
	next.LastPublishedTime = nil
	msg := "resolved Data Guard topology requires additional user input before it can be copied into DataguardBroker.spec.topology"
	if strings.TrimSpace(previewMessage) != "" {
		msg = previewMessage
	}
	setDataguardPreviewCondition(&next.Conditions, m.Generation, false, "WaitingForUserInput", msg)
	m.Status.Dataguard = next
}

func sidbPreviewMemberRole(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(m.Spec.CreateAs)) {
	case "standby":
		return "PHYSICAL_STANDBY"
	default:
		return ""
	}
}

func sidbPreviewTopologyLocked(m *dbapi.SingleInstanceDatabase) bool {
	if m == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(m.Status.CreatedAs)) {
	case "clone":
		return true
	case "standby":
		return strings.EqualFold(strings.TrimSpace(m.Status.DatafilesCreated), "true") ||
			(isPreviewStatusValuePopulated(m.Status.Role) && !strings.EqualFold(strings.TrimSpace(m.Status.Role), "PRIMARY"))
	case "truecache":
		return strings.EqualFold(strings.TrimSpace(m.Status.DatafilesCreated), "true") ||
			hasPreviewConditionTrue(m.Status.Conditions, "TrueCacheBlobSourceReady") ||
			hasPreviewConditionTrue(m.Status.Conditions, "TrueCacheBlobReady")
	default:
		return false
	}
}

func buildSIDBPreviewTopology(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) (*dbapi.DataguardTopologySpec, string, string, string, bool) {
	if m == nil {
		return nil, "", "", "", false
	}

	memberRole := sidbPreviewMemberRole(m)
	if memberRole == "" {
		return nil, "", "", "", false
	}

	memberName := sanitizeDataguardMemberName(m.Name, "sidb-member")
	member := dbapi.DataguardTopologyMember{
		Name:         memberName,
		Role:         memberRole,
		DBUniqueName: strings.ToUpper(strings.TrimSpace(m.Spec.Sid)),
		LocalRef: &dbapi.DataguardLocalRef{
			APIVersion: dbapi.GroupVersion.String(),
			Kind:       "SingleInstanceDatabase",
			Namespace:  m.Namespace,
			Name:       m.Name,
		},
		Endpoints: buildSIDBPreviewEndpoints(m, m.Name, strings.TrimSpace(m.Spec.Sid)),
		TCPS:      buildSIDBPreviewTCPSConfig(m),
	}
	previewReady := true
	previewMessages := make([]string, 0, 2)
	if adminSecretRef, msg, ok := buildSIDBPreviewLocalAdminSecretRef(m); ok {
		member.AdminSecretRef = adminSecretRef
	} else if strings.TrimSpace(msg) != "" {
		previewReady = false
		previewMessages = append(previewMessages, msg)
	}

	primaryMember, primaryMemberName, primaryMessage, primaryReady := buildSIDBPreviewPrimaryMember(m, rp)
	if primaryMember == nil || primaryMemberName == "" {
		return nil, memberName, "", primaryMessage, false
	}
	if strings.TrimSpace(primaryMessage) != "" {
		previewMessages = append(previewMessages, primaryMessage)
	}
	previewReady = previewReady && primaryReady

	topology := &dbapi.DataguardTopologySpec{
		SourceKind: "SingleInstanceDatabase",
		SourceRef: &dbapi.DataguardSourceRef{
			APIVersion: dbapi.GroupVersion.String(),
			Kind:       "SingleInstanceDatabase",
			Namespace:  m.Namespace,
			Name:       m.Name,
		},
		Members: []dbapi.DataguardTopologyMember{*primaryMember, member},
		Pairs: []dbapi.DataguardTopologyPair{{
			Primary: primaryMemberName,
			Standby: memberName,
			Type:    "PHYSICAL",
		}},
	}
	return topology, memberName, primaryMemberName, strings.Join(previewMessages, "; "), previewReady
}

func buildSIDBPreviewPrimaryMember(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) (*dbapi.DataguardTopologyMember, string, string, bool) {
	if m == nil {
		return nil, "", "", false
	}

	source := resolvePrimaryDatabaseSource(m)
	if !source.hasSource() {
		return nil, "", "", false
	}

	memberName := sanitizeDataguardMemberName(GetPrimaryDatabaseDisplayName(m, rp), "primary")
	member := &dbapi.DataguardTopologyMember{
		Name:         memberName,
		Role:         "PRIMARY",
		DBUniqueName: strings.ToUpper(strings.TrimSpace(GetPrimaryDatabaseSid(m, rp))),
	}

	if source.isLocalReference() && rp != nil {
		member.LocalRef = &dbapi.DataguardLocalRef{
			APIVersion: dbapi.GroupVersion.String(),
			Kind:       "SingleInstanceDatabase",
			Namespace:  rp.Namespace,
			Name:       rp.Name,
		}
		member.Endpoints = buildSIDBPreviewEndpoints(rp, rp.Name, strings.TrimSpace(rp.Spec.Sid))
		member.TCPS = buildSIDBPreviewTCPSConfig(rp)
		if adminSecretRef, msg, ok := buildSIDBPreviewLocalAdminSecretRef(rp); ok {
			member.AdminSecretRef = adminSecretRef
			return member, memberName, "", true
		} else {
			return member, memberName, msg, false
		}
	}

	host := GetPrimaryDatabaseHost(m, rp)
	port := GetPrimaryDatabasePort(m)
	service := GetPrimaryDatabaseSid(m, rp)
	if strings.TrimSpace(host) == "" || strings.TrimSpace(service) == "" {
		return nil, "", "", false
	}

	member.Endpoints = []dbapi.DataguardEndpointSpec{{
		Name:        "tcp",
		Protocol:    "TCP",
		Host:        host,
		Port:        int32(port),
		ServiceName: strings.ToUpper(strings.TrimSpace(service)),
	}}
	previewMessages := []string{
		fmt.Sprintf("topology member %q is external; rendered DataguardBroker includes placeholder adminSecretRef values, so replace adminSecretRef.secretName with the correct admin password secret before applying DataguardBroker", memberName),
	}
	if inferredEndpoint, inferredTCPS, ok := buildSIDBPreviewExternalPrimaryTCPSPlaceholders(m, host, service); ok {
		member.Endpoints = append(member.Endpoints, *inferredEndpoint)
		member.TCPS = inferredTCPS
		previewMessages = append(previewMessages, fmt.Sprintf("topology member %q inferred TCPS preview settings from standby TCPS configuration; replace tcps.clientWalletSecret placeholder with the correct primary client wallet secret before applying DataguardBroker", memberName))
	}
	member.AdminSecretRef = &dbapi.DataguardSecretRef{
		SecretName: dataguardPreviewExternalSecretPlaceholder,
		SecretKey:  dataguardPreviewExternalSecretKey,
	}
	return member, memberName, strings.Join(previewMessages, "; "), true
}

func buildSIDBPreviewExternalPrimaryTCPSPlaceholders(standby *dbapi.SingleInstanceDatabase, host, service string) (*dbapi.DataguardEndpointSpec, *dbapi.DataguardTCPSConfig, bool) {
	if standby == nil || !getTcpsEnabled(standby) {
		return nil, nil, false
	}

	port := int32(getTcpsListenerPort(standby))
	if port == 0 {
		port = int32(dbcommons.CONTAINER_TCPS_PORT)
	}
	endpoint := &dbapi.DataguardEndpointSpec{
		Name:        "tcps",
		Protocol:    "TCPS",
		Host:        strings.TrimSpace(host),
		Port:        port,
		ServiceName: strings.ToUpper(strings.TrimSpace(service)),
	}
	tcps := &dbapi.DataguardTCPSConfig{
		Enabled:            true,
		ClientWalletSecret: dataguardPreviewExternalPrimaryWalletPH,
	}
	return endpoint, tcps, true
}

func buildSIDBPreviewLocalAdminSecretRef(m *dbapi.SingleInstanceDatabase) (*dbapi.DataguardSecretRef, string, bool) {
	if m == nil {
		return nil, "singleinstancedatabase is nil", false
	}
	secretName, secretKey, ok := dbapi.ResolveSIDBAdminSecretRef(m)
	if !ok {
		return nil, fmt.Sprintf("singleinstancedatabase %q does not publish admin password secret metadata", m.Name), false
	}
	return &dbapi.DataguardSecretRef{
		SecretName: secretName,
		SecretKey:  secretKey,
	}, "", true
}

func buildSIDBPreviewEndpoints(m *dbapi.SingleInstanceDatabase, hostHint, serviceHint string) []dbapi.DataguardEndpointSpec {
	if m == nil {
		return nil
	}
	host := strings.TrimSpace(hostHint)
	if host == "" {
		host = m.Name
	}
	serviceName := strings.ToUpper(strings.TrimSpace(serviceHint))
	if serviceName == "" {
		serviceName = strings.ToUpper(strings.TrimSpace(m.Spec.Sid))
	}
	if serviceName == "" {
		serviceName = "ORCLCDB"
	}

	endpoints := []dbapi.DataguardEndpointSpec{{
		Name:        "tcp",
		Protocol:    "TCP",
		Host:        host,
		Port:        int32(dbcommons.CONTAINER_LISTENER_PORT),
		ServiceName: serviceName,
	}}
	if getTcpsEnabled(m) {
		endpoints = append(endpoints, dbapi.DataguardEndpointSpec{
			Name:        "tcps",
			Protocol:    "TCPS",
			Host:        host,
			Port:        int32(getTcpsListenerPort(m)),
			ServiceName: serviceName,
		})
	}
	return endpoints
}

func buildSIDBPreviewTCPSConfig(m *dbapi.SingleInstanceDatabase) *dbapi.DataguardTCPSConfig {
	if m == nil || !getTcpsEnabled(m) {
		return nil
	}
	clientWalletSecret := ""
	if override := strings.TrimSpace(getTcpsClientWalletSecretOverride(m)); override != "" {
		clientWalletSecret = override
	} else if m.Status.IsTcpsEnabled && strings.TrimSpace(m.Status.ClientWalletLoc) != "" {
		clientWalletSecret = strings.TrimSpace(getDataguardClientWalletSecretName(m))
	}
	return &dbapi.DataguardTCPSConfig{
		Enabled:            true,
		ClientWalletSecret: clientWalletSecret,
	}
}

func buildSIDBPreviewExecutionStatus(m *dbapi.SingleInstanceDatabase) *dbapi.DataguardExecutionStatus {
	if m == nil {
		return nil
	}
	image := strings.TrimSpace(m.Spec.Image.PullFrom)
	if image == "" {
		return nil
	}
	status := &dbapi.DataguardExecutionStatus{Image: image}
	if secret := strings.TrimSpace(m.Spec.Image.PullSecrets); secret != "" {
		status.ImagePullSecrets = []string{secret}
	}
	return status
}

func (r *ShardingDatabaseReconciler) syncShardingDataguardPreviewStatus(instance *dbapi.ShardingDatabase, st *dbapi.ShardingDatabaseStatus) {
	if st == nil {
		return
	}
	mode := dataguardProducerMode(instance.Spec.Dataguard)
	if mode == dbapi.DataguardProducerModeDisabled || shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) != "DG" {
		st.Dataguard = nil
		return
	}

	current := st.Dataguard
	next := &dbapi.ShardingDataguardStatus{}
	if current != nil {
		next.BrokerRef = current.BrokerRef
		next.Conditions = append([]metav1.Condition(nil), current.Conditions...)
		next.TopologyLocked = current.TopologyLocked
	}

	if mode == dbapi.DataguardProducerModeManaged {
		next.Phase = dataguardPreviewPhaseManagedNotImplemented
		next.ReadyForBroker = false
		setDataguardPreviewCondition(&next.Conditions, instance.Generation, false, "ManagedNotImplemented", "Managed mode is reserved for future DataguardBroker automation")
		st.Dataguard = next
		return
	}

	topology, members, pairs, previewMessage, previewReason, ready := r.buildShardingPreviewTopology(instance)
	next.Topology = topology
	next.Execution = buildShardingPreviewExecutionStatus(instance)
	next.Members = members
	next.Pairs = pairs
	next.RenderedBrokerSpec = buildRenderedBrokerPreviewStatus(instance.Name, instance.Namespace, topology, next.Execution, "", false)
	if topology == nil {
		next.Phase = dataguardPreviewPhaseWaitingForTopology
		next.ReadyForBroker = false
		next.TopologyHash = ""
		next.PublishedTopologyHash = ""
		next.LastPublishedTime = nil
		msg := "resolved Data Guard topology is not ready yet"
		if strings.TrimSpace(previewMessage) != "" {
			msg = previewMessage
		}
		setDataguardPreviewCondition(&next.Conditions, instance.Generation, false, "WaitingForTopology", msg)
		st.Dataguard = next
		return
	}
	next.TopologyHash = dataguardTopologyHash(topology)
	next.RenderedBrokerSpec = buildRenderedBrokerPreviewStatus(instance.Name, instance.Namespace, topology, next.Execution, next.TopologyHash, ready)
	if ready {
		next.Phase = dataguardPreviewPhaseReady
		next.ReadyForBroker = true
		next.PublishedTopologyHash = next.TopologyHash
		now := metav1.Now()
		next.LastPublishedTime = &now
		setDataguardPreviewCondition(&next.Conditions, instance.Generation, true, "PreviewReady", dataguardPreviewReadyMessage(previewMessage))
	} else if previewReason == "WaitingForUserInput" {
		next.Phase = dataguardPreviewPhaseWaitingForUserInput
		next.ReadyForBroker = false
		next.PublishedTopologyHash = ""
		next.LastPublishedTime = nil
		msg := "resolved Data Guard topology requires additional user input before it can be copied into DataguardBroker.spec.topology"
		if strings.TrimSpace(previewMessage) != "" {
			msg = previewMessage
		}
		setDataguardPreviewCondition(&next.Conditions, instance.Generation, false, "WaitingForUserInput", msg)
	} else {
		next.Phase = dataguardPreviewPhaseWaitingForTopology
		next.ReadyForBroker = false
		next.PublishedTopologyHash = ""
		next.LastPublishedTime = nil
		msg := "resolved Data Guard topology is still incomplete"
		if strings.TrimSpace(previewMessage) != "" {
			msg = previewMessage
		}
		setDataguardPreviewCondition(&next.Conditions, instance.Generation, false, "WaitingForTopology", msg)
	}
	if !next.TopologyLocked {
		next.TopologyLocked = false
	}
	st.Dataguard = next
}

func buildShardingPreviewExecutionStatus(instance *dbapi.ShardingDatabase) *dbapi.DataguardExecutionStatus {
	if instance == nil {
		return nil
	}
	image := strings.TrimSpace(instance.Spec.DbImage)
	if image == "" {
		return nil
	}
	status := &dbapi.DataguardExecutionStatus{Image: image}
	if secret := strings.TrimSpace(instance.Spec.DbImagePullSecret); secret != "" {
		status.ImagePullSecrets = []string{secret}
	}
	return status
}

func (r *ShardingDatabaseReconciler) buildShardingPreviewTopology(instance *dbapi.ShardingDatabase) (*dbapi.DataguardTopologySpec, []dbapi.ShardingDataguardMemberStatus, []dbapi.DataguardPairStatus, string, string, bool) {
	if instance == nil || shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) != "DG" {
		return nil, nil, nil, "", "WaitingForTopology", false
	}

	topology := &dbapi.DataguardTopologySpec{
		SourceKind: "ShardingDatabase",
		SourceRef: &dbapi.DataguardSourceRef{
			APIVersion: dbapi.GroupVersion.String(),
			Kind:       "ShardingDatabase",
			Namespace:  instance.Namespace,
			Name:       instance.Name,
		},
	}

	memberStatuses := []dbapi.ShardingDataguardMemberStatus{}
	pairStatuses := []dbapi.DataguardPairStatus{}
	memberIndex := map[string]int{}
	ready := true
	previewMessages := []string{}
	previewReason := "PreviewReady"

	addMember := func(member dbapi.DataguardTopologyMember, shard dbapi.ShardSpec, primaryMemberName, phase, message string) string {
		name := sanitizeDataguardMemberName(member.Name, "member")
		member.Name = name
		if idx, ok := memberIndex[name]; ok {
			if message != "" && idx < len(memberStatuses) && strings.TrimSpace(memberStatuses[idx].Message) == "" {
				memberStatuses[idx].Message = message
			}
			if phase != "" && phase != dataguardPreviewPhaseReady && idx < len(memberStatuses) {
				memberStatuses[idx].Phase = phase
			}
			return name
		}
		memberIndex[name] = len(topology.Members)
		topology.Members = append(topology.Members, member)
		statusPhase := strings.TrimSpace(phase)
		if statusPhase == "" {
			statusPhase = dataguardPreviewPhaseReady
		}
		memberStatuses = append(memberStatuses, dbapi.ShardingDataguardMemberStatus{
			Name:              name,
			Role:              member.Role,
			DBUniqueName:      member.DBUniqueName,
			ShardGroup:        strings.TrimSpace(shard.ShardGroup),
			ShardSpace:        strings.TrimSpace(shard.ShardSpace),
			PrimaryMemberName: primaryMemberName,
			Endpoints:         append([]dbapi.DataguardEndpointSpec(nil), member.Endpoints...),
			TCPS:              member.TCPS,
			Phase:             statusPhase,
			Message:           message,
		})
		return name
	}

	for i := range instance.Spec.Shard {
		shard := instance.Spec.Shard[i]
		role := strings.ToUpper(strings.TrimSpace(shard.DeployAs))
		if role != "STANDBY" && role != "ACTIVE_STANDBY" {
			continue
		}

		standbyMember := buildShardingLocalTopologyMember(instance, shard, "PHYSICAL_STANDBY")
		if adminSecretRef, msg, ok := buildShardingPreviewAdminSecretRef(instance); ok {
			standbyMember.AdminSecretRef = adminSecretRef
		} else {
			ready = false
			previewReason = "WaitingForTopology"
			if strings.TrimSpace(msg) != "" {
				previewMessages = append(previewMessages, msg)
			}
		}
		standbyName := addMember(standbyMember, shard, "", "", "")

		primaryMember, primaryName, msg, resolved, pairReady := r.buildShardingPrimaryPreviewMember(instance, shard)
		if !resolved {
			ready = false
			previewReason = "WaitingForTopology"
			if strings.TrimSpace(msg) != "" {
				previewMessages = append(previewMessages, msg)
			}
			pairStatuses = append(pairStatuses, dbapi.DataguardPairStatus{
				Primary: "",
				Standby: standbyName,
				State:   "Pending",
				Message: msg,
			})
			idx := memberIndex[standbyName]
			memberStatuses[idx].Phase = dataguardPreviewPhaseWaitingForTopology
			memberStatuses[idx].Message = msg
			continue
		}
		if strings.TrimSpace(msg) != "" {
			previewMessages = append(previewMessages, msg)
		}
		primaryPhase := dataguardPreviewPhaseReady
		standbyPhase := dataguardPreviewPhaseReady
		if !pairReady {
			ready = false
			previewReason = "WaitingForUserInput"
			primaryPhase = dataguardPreviewPhaseWaitingForUserInput
			standbyPhase = dataguardPreviewPhaseWaitingForUserInput
		}
		primaryName = addMember(*primaryMember, dbapi.ShardSpec{}, "", primaryPhase, msg)
		idx := memberIndex[standbyName]
		memberStatuses[idx].PrimaryMemberName = primaryName
		memberStatuses[idx].Phase = standbyPhase
		memberStatuses[idx].Message = msg

		topology.Pairs = append(topology.Pairs, dbapi.DataguardTopologyPair{
			Primary: primaryName,
			Standby: standbyName,
			Type:    "PHYSICAL",
		})
		pairStatuses = append(pairStatuses, dbapi.DataguardPairStatus{
			Primary: primaryName,
			Standby: standbyName,
			State:   "Resolved",
			Message: msg,
		})
	}

	if len(topology.Members) == 0 || len(topology.Pairs) == 0 {
		return nil, memberStatuses, pairStatuses, strings.Join(previewMessages, "; "), previewReason, false
	}
	return topology, memberStatuses, pairStatuses, strings.Join(previewMessages, "; "), previewReason, ready
}

func buildShardingLocalTopologyMember(instance *dbapi.ShardingDatabase, shard dbapi.ShardSpec, role string) dbapi.DataguardTopologyMember {
	dbUniqueName := strings.ToUpper(strings.TrimSpace(shard.Name))
	host, port := (&ShardingDatabaseReconciler{}).resolveShardHostPort(instance, shard)
	member := dbapi.DataguardTopologyMember{
		Name:         sanitizeDataguardMemberName(shard.Name, "shard"),
		Role:         role,
		DBUniqueName: dbUniqueName,
		Endpoints: []dbapi.DataguardEndpointSpec{{
			Name:        "tcp",
			Protocol:    "TCP",
			Host:        host,
			Port:        port,
			ServiceName: shardingv1.BuildDgmgrlServiceName(dbUniqueName),
		}},
	}
	if instance.Spec.EnableTCPS {
		member.Endpoints = append(member.Endpoints, dbapi.DataguardEndpointSpec{
			Name:        "tcps",
			Protocol:    "TCPS",
			Host:        host,
			Port:        2484,
			ServiceName: shardingv1.BuildDgmgrlServiceName(dbUniqueName),
		})
		member.TCPS = &dbapi.DataguardTCPSConfig{
			Enabled: true,
		}
	}
	return member
}

func (r *ShardingDatabaseReconciler) buildShardingPrimaryPreviewMember(instance *dbapi.ShardingDatabase, standby dbapi.ShardSpec) (*dbapi.DataguardTopologyMember, string, string, bool, bool) {
	if instance == nil {
		return nil, "", "instance is nil", false, false
	}

	desiredPairs := r.buildDgPairsFromStandbyConfig(instance)
	if pair := findPreviewPairForStandby(desiredPairs, standby.Name); pair != nil {
		if p := r.findPrimaryByPair(instance, *pair); p != nil {
			member := buildShardingLocalTopologyMember(instance, *p, "PRIMARY")
			if adminSecretRef, msg, ok := buildShardingPreviewAdminSecretRef(instance); ok {
				member.AdminSecretRef = adminSecretRef
				return &member, member.Name, strings.TrimSpace(pair.Message), true, true
			} else {
				return &member, member.Name, msg, true, false
			}
		}
		if primaryMember := buildShardingExternalPrimaryPreviewMember(pair); primaryMember != nil {
			primaryMember.AdminSecretRef = &dbapi.DataguardSecretRef{
				SecretName: dataguardPreviewExternalSecretPlaceholder,
				SecretKey:  dataguardPreviewExternalSecretKey,
			}
			message := fmt.Sprintf("topology member %q is external; rendered DataguardBroker includes placeholder adminSecretRef values, so replace adminSecretRef.secretName with the correct admin password secret before applying DataguardBroker", primaryMember.Name)
			return primaryMember, primaryMember.Name, message, true, true
		}
	}

	p, err := r.findPrimaryForStandby(instance, standby)
	if err != nil {
		return nil, "", err.Error(), false, false
	}
	if p == nil {
		return nil, "", fmt.Sprintf("no primary resolved for standby %s", standby.Name), false, false
	}
	member := buildShardingLocalTopologyMember(instance, *p, "PRIMARY")
	if adminSecretRef, msg, ok := buildShardingPreviewAdminSecretRef(instance); ok {
		member.AdminSecretRef = adminSecretRef
		return &member, member.Name, "resolved from sharding topology", true, true
	} else {
		return &member, member.Name, msg, true, false
	}
}

func buildShardingPreviewAdminSecretRef(instance *dbapi.ShardingDatabase) (*dbapi.DataguardSecretRef, string, bool) {
	if instance == nil {
		return nil, "shardingdatabase is nil", false
	}
	if instance.Spec.DbSecret == nil {
		return nil, fmt.Sprintf("shardingdatabase %q does not publish spec.dbSecret", instance.Name), false
	}
	secretName := strings.TrimSpace(instance.Spec.DbSecret.Name)
	if secretName == "" {
		return nil, fmt.Sprintf("shardingdatabase %q does not publish spec.dbSecret.name", instance.Name), false
	}
	secretKey := strings.TrimSpace(instance.Spec.DbSecret.DbAdmin.PasswordKey)
	if secretKey == "" {
		return nil, fmt.Sprintf("shardingdatabase %q does not publish spec.dbSecret.dbAdmin.passwordKey", instance.Name), false
	}
	return &dbapi.DataguardSecretRef{
		SecretName: secretName,
		SecretKey:  secretKey,
	}, "", true
}

func findPreviewPairForStandby(pairs []dbapi.DgPairStatus, standbyName string) *dbapi.DgPairStatus {
	target := strings.TrimSpace(standbyName)
	for i := range pairs {
		if strings.TrimSpace(pairs[i].StandbyShardName) == target {
			return &pairs[i]
		}
	}
	return nil
}

func buildShardingExternalPrimaryPreviewMember(pair *dbapi.DgPairStatus) *dbapi.DataguardTopologyMember {
	if pair == nil {
		return nil
	}
	connect := strings.TrimSpace(pair.PrimaryConnectString)
	if connect == "" {
		connect = strings.TrimSpace(pair.PrimaryKey)
	}
	if connect == "" {
		return nil
	}
	host, port, service := parsePreviewConnectIdentifier(connect)
	if host == "" {
		return nil
	}
	memberName := sanitizeDataguardMemberName(pair.PrimaryKey, "external-primary")
	if strings.TrimSpace(service) == "" {
		service = strings.ToUpper(strings.TrimSpace(memberName))
	}
	return &dbapi.DataguardTopologyMember{
		Name:         memberName,
		Role:         "PRIMARY",
		DBUniqueName: strings.ToUpper(strings.TrimSpace(memberName)),
		Endpoints: []dbapi.DataguardEndpointSpec{{
			Name:        "tcp",
			Protocol:    "TCP",
			Host:        host,
			Port:        port,
			ServiceName: service,
		}},
	}
}

func parsePreviewConnectIdentifier(connect string) (string, int32, string) {
	raw := strings.TrimSpace(connect)
	if raw == "" {
		return "", 0, ""
	}
	trimmed := strings.TrimPrefix(strings.TrimPrefix(raw, "//"), "tcp://")
	hostPort := trimmed
	service := ""
	if slash := strings.LastIndex(trimmed, "/"); slash >= 0 {
		hostPort = strings.TrimSpace(trimmed[:slash])
		service = strings.ToUpper(strings.TrimSpace(trimmed[slash+1:]))
	}
	host := hostPort
	port := int32(1521)
	if colon := strings.LastIndex(hostPort, ":"); colon >= 0 && colon < len(hostPort)-1 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(hostPort[colon+1:])); err == nil {
			port = int32(parsed)
			host = strings.TrimSpace(hostPort[:colon])
		}
	}
	return strings.TrimSpace(host), port, service
}

func dataguardTopologyHash(topology *dbapi.DataguardTopologySpec) string {
	if topology == nil {
		return ""
	}
	canonical := topology.DeepCopy()
	sort.Slice(canonical.Members, func(i, j int) bool {
		return canonical.Members[i].Name < canonical.Members[j].Name
	})
	for i := range canonical.Members {
		sort.Slice(canonical.Members[i].Endpoints, func(a, b int) bool {
			left := canonical.Members[i].Endpoints[a]
			right := canonical.Members[i].Endpoints[b]
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			if left.Protocol != right.Protocol {
				return left.Protocol < right.Protocol
			}
			if left.Host != right.Host {
				return left.Host < right.Host
			}
			if left.Port != right.Port {
				return left.Port < right.Port
			}
			return left.ServiceName < right.ServiceName
		})
	}
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

func sanitizeDataguardMemberName(raw, fallback string) string {
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
		name = "member"
	}
	return name
}

func isPreviewStatusValuePopulated(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed != dbcommons.ValueUnavailable
}

func hasPreviewConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for i := range conditions {
		if conditions[i].Type == conditionType && conditions[i].Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func setDataguardPreviewCondition(conditions *[]metav1.Condition, generation int64, ready bool, reason, message string) {
	if conditions == nil {
		return
	}
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               "TopologyPreviewReady",
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	})
}

func buildRenderedBrokerPreviewStatus(resourceName, namespace string, topology *dbapi.DataguardTopologySpec, execution *dbapi.DataguardExecutionStatus, topologyHash string, ready bool) *dbapi.DataguardRenderedBrokerStatus {
	if topology == nil {
		return nil
	}
	spec := &dbapi.DataguardRenderedBrokerSpec{
		Topology: topology.DeepCopy(),
	}
	if execution != nil && strings.TrimSpace(execution.Image) != "" {
		spec.Execution = &dbapi.DataguardExecutionSpec{
			Image:            strings.TrimSpace(execution.Image),
			ImagePullSecrets: append([]string(nil), execution.ImagePullSecrets...),
		}
	}
	now := metav1.Now()
	return &dbapi.DataguardRenderedBrokerStatus{
		Name:         buildRenderedBrokerName(resourceName),
		Namespace:    strings.TrimSpace(namespace),
		Spec:         spec,
		TopologyHash: strings.TrimSpace(topologyHash),
		GeneratedAt:  &now,
		Ready:        ready,
	}
}

func buildRenderedBrokerName(resourceName string) string {
	base := sanitizeDataguardMemberName(resourceName, "dataguard")
	if strings.HasSuffix(base, "-dg") {
		return base
	}
	return base + "-dg"
}
