package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

type dataguardTopologyResolvedMember struct {
	Name            string
	Role            string
	DBUniqueName    string
	Alias           string
	ResourceName    string
	LocalRef        *dbapi.DataguardLocalRef
	Endpoint        dbapi.DataguardEndpointSpec
	ConnectString   string
	AdminSecretName string
	AdminSecretKey  string
	AdminPassword   string
	WalletDirectory string
	SSLServerDN     string
}

type dataguardTopologyResolvedState struct {
	Runtime                *dataguardBrokerExecutionRuntime
	Primary                *dataguardTopologyResolvedMember
	Members                []*dataguardTopologyResolvedMember
	MembersByName          map[string]*dataguardTopologyResolvedMember
	MembersByDBUniqueName  map[string]*dataguardTopologyResolvedMember
	DesiredStandbys        []*dataguardTopologyResolvedMember
	DesiredPhysicalMembers []*dataguardTopologyResolvedMember
}

func resolveDataguardTopologyState(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) (*dataguardTopologyResolvedState, error) {
	if broker == nil || broker.Spec.Topology == nil {
		return nil, fmt.Errorf("spec.topology is not set")
	}
	if runtime == nil || strings.TrimSpace(runtime.Image) == "" {
		return nil, fmt.Errorf("execution runtime is not resolved")
	}

	state := &dataguardTopologyResolvedState{
		Runtime:               runtime,
		MembersByName:         map[string]*dataguardTopologyResolvedMember{},
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{},
	}

	for i := range broker.Spec.Topology.Members {
		member := broker.Spec.Topology.Members[i]
		resolved, err := resolveDataguardTopologyMember(ctx, r, broker, runtime, &member)
		if err != nil {
			return nil, err
		}
		state.Members = append(state.Members, resolved)
		state.MembersByName[strings.ToLower(resolved.Name)] = resolved
		state.MembersByDBUniqueName[strings.ToUpper(resolved.DBUniqueName)] = resolved
		switch resolved.Role {
		case "PRIMARY":
			state.Primary = resolved
			state.DesiredPhysicalMembers = append(state.DesiredPhysicalMembers, resolved)
		case "PHYSICAL_STANDBY":
			state.DesiredStandbys = append(state.DesiredStandbys, resolved)
			state.DesiredPhysicalMembers = append(state.DesiredPhysicalMembers, resolved)
		case "SNAPSHOT_STANDBY":
			state.DesiredStandbys = append(state.DesiredStandbys, resolved)
		}
	}

	if state.Primary == nil {
		return nil, fmt.Errorf("topology does not include a PRIMARY member")
	}

	for i := range broker.Spec.Topology.Pairs {
		pair := broker.Spec.Topology.Pairs[i]
		primary := state.MembersByName[strings.ToLower(strings.TrimSpace(pair.Primary))]
		standby := state.MembersByName[strings.ToLower(strings.TrimSpace(pair.Standby))]
		if primary == nil || standby == nil {
			return nil, fmt.Errorf("topology pair %q -> %q cannot be resolved", pair.Primary, pair.Standby)
		}
		if primary.Role != "PRIMARY" {
			return nil, fmt.Errorf("topology pair primary member %q is not PRIMARY", pair.Primary)
		}
		if standby.Role != "PHYSICAL_STANDBY" && standby.Role != "SNAPSHOT_STANDBY" {
			return nil, fmt.Errorf("topology pair standby member %q uses unsupported role %q", pair.Standby, standby.Role)
		}
	}

	return state, nil
}

func resolveDataguardTopologyMember(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, member *dbapi.DataguardTopologyMember) (*dataguardTopologyResolvedMember, error) {
	if member == nil {
		return nil, fmt.Errorf("topology member is nil")
	}

	role := normalizeTopologyMemberRole(member.Role)
	if role != "PRIMARY" && role != "PHYSICAL_STANDBY" && role != "SNAPSHOT_STANDBY" {
		return nil, fmt.Errorf("topology member %q uses unsupported role %q", strings.TrimSpace(member.Name), member.Role)
	}

	endpoint, _ := selectPreferredDataguardEndpoint(member.Endpoints)
	if endpoint == nil {
		return nil, fmt.Errorf("topology member %q does not declare a usable endpoint", strings.TrimSpace(member.Name))
	}

	dbUniqueName := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
	if dbUniqueName == "" {
		dbUniqueName = strings.ToUpper(strings.TrimSpace(member.Name))
	}
	alias := sanitizeDataguardRunnerName(dbUniqueName, strings.TrimSpace(member.Name))
	alias = strings.ToUpper(alias)

	resolved := &dataguardTopologyResolvedMember{
		Name:          strings.TrimSpace(member.Name),
		Role:          role,
		DBUniqueName:  dbUniqueName,
		Alias:         alias,
		LocalRef:      member.LocalRef,
		Endpoint:      *endpoint,
		ConnectString: formatDataguardEndpointConnectString(endpoint),
		SSLServerDN:   firstNonEmptyString(strings.TrimSpace(endpoint.SSLServerDN), tcpsServerDN(member.TCPS)),
	}

	if member.LocalRef != nil {
		resolved.ResourceName = strings.TrimSpace(member.LocalRef.Name)
	} else {
		resolved.ResourceName = resolved.Name
	}

	secretName, secretKey, secretNamespace, err := resolveDataguardTopologyMemberAdminSecretRef(ctx, r, broker, member)
	if err != nil {
		return nil, err
	}
	resolved.AdminSecretName = secretName
	resolved.AdminSecretKey = secretKey

	adminPassword, err := readDataguardTopologyMemberAdminPassword(ctx, r, secretNamespace, secretName, secretKey)
	if err != nil {
		return nil, err
	}
	resolved.AdminPassword = adminPassword

	if strings.EqualFold(strings.TrimSpace(endpoint.Protocol), "TCPS") {
		walletSecret := dbapi.ResolveDataguardTopologyMemberClientWalletSecret(broker.Spec.Topology, member)
		if walletSecret == "" {
			return nil, fmt.Errorf("topology member %q uses TCPS but tcps.clientWalletSecret is not set", resolved.Name)
		}
		resolved.WalletDirectory = strings.TrimRight(runtime.WalletMountPath, "/") + "/" + sanitizeDataguardRunnerName(walletSecret, "wallet")
	}

	return resolved, nil
}

func resolveDataguardTopologyMemberAdminSecretRef(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, member *dbapi.DataguardTopologyMember) (string, string, string, error) {
	if broker == nil || member == nil {
		return "", "", "", fmt.Errorf("broker or topology member is nil")
	}

	if secretName, secretKey, ok := dbapi.ResolveDataguardTopologyMemberExplicitAdminSecretRef(broker.Spec.Topology, member); ok {
		if secretName == "" {
			return "", "", "", fmt.Errorf("topology member %q adminSecretRef.secretName is empty", strings.TrimSpace(member.Name))
		}
		if secretKey == "" {
			secretKey = "password"
		}
		return secretName, secretKey, broker.Namespace, nil
	}

	if member.LocalRef == nil {
		return "", "", "", fmt.Errorf("topology member %q must set adminSecretRef when localRef is not provided", strings.TrimSpace(member.Name))
	}

	localNamespace := strings.TrimSpace(member.LocalRef.Namespace)
	if localNamespace == "" {
		localNamespace = broker.Namespace
	}

	switch strings.TrimSpace(member.LocalRef.Kind) {
	case "", "SingleInstanceDatabase":
		var sidb dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: localNamespace, Name: strings.TrimSpace(member.LocalRef.Name)}, &sidb); err != nil {
			return "", "", "", err
		}
		secretName, secretKey, ok := dbapi.ResolveSIDBAdminSecretRef(&sidb)
		if !ok {
			return "", "", "", fmt.Errorf("singleinstancedatabase %q does not publish admin password secret metadata", sidb.Name)
		}
		return secretName, secretKey, sidb.Namespace, nil
	default:
		return "", "", "", fmt.Errorf("topology member %q kind %q must set adminSecretRef explicitly", strings.TrimSpace(member.Name), strings.TrimSpace(member.LocalRef.Kind))
	}
}

func readDataguardTopologyMemberAdminPassword(ctx context.Context, r *DataguardBrokerReconciler, namespace, secretName, secretKey string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("secret %s/%s not found", namespace, secretName)
		}
		return "", err
	}
	value, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %q", namespace, secretName, secretKey)
	}
	return string(value), nil
}

func ensureDataguardTopologyLocalDatabasePrereqs(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, state *dataguardTopologyResolvedState, req ctrl.Request) error {
	if state == nil {
		return fmt.Errorf("topology state is incomplete")
	}

	for _, member := range state.Members {
		if member == nil || member.LocalRef == nil {
			continue
		}
		kind := strings.TrimSpace(member.LocalRef.Kind)
		if kind != "" && kind != "SingleInstanceDatabase" {
			continue
		}

		namespace := strings.TrimSpace(member.LocalRef.Namespace)
		if namespace == "" {
			namespace = broker.Namespace
		}

		var sidb dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: strings.TrimSpace(member.LocalRef.Name)}, &sidb); err != nil {
			return err
		}

		readyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version, "", sidb.Name, sidb.Namespace, ctx, req)
		if err != nil {
			return fmt.Errorf("failed to find ready pod for local member %s: %w", member.Name, err)
		}
		if strings.TrimSpace(readyPod.Name) == "" {
			return fmt.Errorf("local member %s does not have a ready database pod yet", member.Name)
		}

		command := dbcommons.BuildDataguardPrereqsCommand(
			"configure",
			dbapi.DataguardProducerBrokerConfigDir(sidb.Spec.Dataguard),
			dbapi.DataguardProducerStandbyRedoSize(sidb.Spec.Dataguard),
		)
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", command)
		if err != nil {
			return fmt.Errorf("failed to configure Data Guard prerequisites for local member %s: %w", member.Name, err)
		}
		r.Log.Info("Configured local database Data Guard prerequisites", "member", member.Name, "pod", readyPod.Name, "output", out)
	}

	return nil
}

func ensureDataguardTopologyNetConfiguration(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, state *dataguardTopologyResolvedState, req ctrl.Request) error {
	if broker == nil || state == nil || state.Runtime == nil {
		return fmt.Errorf("topology runtime state is incomplete")
	}
	tnsFile := strings.TrimRight(state.Runtime.TNSAdminPath, "/") + "/tnsnames.ora"
	sqlnetFile := strings.TrimRight(state.Runtime.TNSAdminPath, "/") + "/sqlnet.ora"

	var entries []string
	for _, member := range state.Members {
		entries = append(entries, buildDataguardTopologyTNSAlias(member))
	}
	sort.Strings(entries)
	if err := writeDataguardRunnerFile(ctx, r, broker, req, tnsFile, strings.Join(entries, "\n")); err != nil {
		return err
	}
	return writeDataguardRunnerFile(ctx, r, broker, req, sqlnetFile, buildDataguardTopologySQLNet(state))
}

func buildDataguardTopologyTNSAlias(member *dataguardTopologyResolvedMember) string {
	if member == nil {
		return ""
	}
	protocol := strings.ToUpper(strings.TrimSpace(member.Endpoint.Protocol))
	if protocol == "" {
		protocol = "TCP"
	}
	entry := fmt.Sprintf(`%s =
(DESCRIPTION =
  (ADDRESS = (PROTOCOL = %s)(HOST = %s)(PORT = %d))
  (CONNECT_DATA =
    (SERVER = DEDICATED)
    (SERVICE_NAME = %s)
  )`, member.Alias, protocol, strings.TrimSpace(member.Endpoint.Host), member.Endpoint.Port, strings.ToUpper(strings.TrimSpace(member.Endpoint.ServiceName)))

	if protocol == "TCPS" {
		entry += `
  (SECURITY =`
		if strings.TrimSpace(member.SSLServerDN) != "" {
			entry += fmt.Sprintf(`
    (SSL_SERVER_DN_MATCH = YES)
    (SSL_SERVER_CERT_DN = %s)`, strings.TrimSpace(member.SSLServerDN))
		}
		if strings.TrimSpace(member.WalletDirectory) != "" {
			entry += fmt.Sprintf(`
    (MY_WALLET_DIRECTORY = %s)`, strings.TrimSpace(member.WalletDirectory))
		}
		entry += `
  )`
	}

	entry += `
)
`
	return entry
}

func buildDataguardTopologySQLNet(state *dataguardTopologyResolvedState) string {
	lines := []string{
		"NAMES.DIRECTORY_PATH=(TNSNAMES,EZCONNECT)",
		"DIAG_ADR_ENABLED=OFF",
	}
	if topologyUsesTCPS(state) {
		lines = append(lines, "SSL_SERVER_DN_MATCH=YES")
	}
	return strings.Join(lines, "\n") + "\n"
}

func topologyUsesTCPS(state *dataguardTopologyResolvedState) bool {
	if state == nil {
		return false
	}
	for _, member := range state.Members {
		if member != nil && strings.EqualFold(strings.TrimSpace(member.Endpoint.Protocol), "TCPS") {
			return true
		}
	}
	return false
}

func writeDataguardRunnerFile(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, path, content string) error {
	command := fmt.Sprintf("mkdir -p %s && cat > %s <<'__CODex_EOF__'\n%s\n__CODex_EOF__\n", shellQuote(strings.TrimRight(pathDir(path), "/")), shellQuote(path), content)
	_, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
	if err != nil {
		return fmt.Errorf("failed to write runner file %s: %w", path, err)
	}
	return nil
}

func execDataguardBrokerRunnerShell(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, nolog bool, command string) (string, error) {
	return dbcommons.ExecCommand(r, r.Config, dataguardBrokerRunnerPodName(broker), broker.Namespace, "runner", ctx, req, nolog, "bash", "-c", command)
}

func runDataguardBrokerRunnerDGMGRLScript(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, connectMember *dataguardTopologyResolvedMember, script string) (string, error) {
	if connectMember == nil {
		return "", fmt.Errorf("runner dgmgrl member is nil")
	}
	scriptPath := "/tmp/dgmgrl-topology.cmd"
	if err := writeDataguardRunnerFile(ctx, r, broker, req, scriptPath, script); err != nil {
		return "", err
	}
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, connectMember.Alias, false)
	command := fmt.Sprintf("dgmgrl -silent %s @%s; rc=$?; rm -f %s; exit $rc", shellQuote(connectArg), shellQuote(scriptPath), shellQuote(scriptPath))
	return execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
}

func queryDataguardConfigurationMembers(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, connectMember *dataguardTopologyResolvedMember) (map[string]string, error) {
	if connectMember == nil {
		return nil, fmt.Errorf("query member is nil")
	}
	scriptPath := "/tmp/dg-broker-members.sql"
	script := `set heading off
set feedback off
set verify off
set echo off
set pages 0
set lines 400
SELECT DATABASE || ':' || DATAGUARD_ROLE FROM V$DG_BROKER_CONFIG ORDER BY DATABASE;
exit
`
	if err := writeDataguardRunnerFile(ctx, r, broker, req, scriptPath, script); err != nil {
		return nil, err
	}
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, connectMember.Alias, true)
	command := fmt.Sprintf("sqlplus -s %s @%s; rc=$?; rm -f %s; exit $rc", shellQuote(connectArg), shellQuote(scriptPath), shellQuote(scriptPath))
	out, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
	if err != nil {
		if strings.Contains(out, "ORA-16532") || strings.Contains(err.Error(), "ORA-16532") {
			return nil, nil
		}
		return nil, err
	}

	members := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(trimmed, "Connected to:") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		members[strings.ToUpper(strings.TrimSpace(parts[0]))] = strings.ToUpper(strings.TrimSpace(parts[1]))
	}
	return members, nil
}

func ensureDataguardTopologyBrokerConfiguration(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, state *dataguardTopologyResolvedState) error {
	if state == nil || state.Primary == nil {
		return fmt.Errorf("topology state is incomplete")
	}
	if err := ensureDataguardTopologyLocalDatabasePrereqs(ctx, r, broker, state, req); err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}

	showOut, showErr := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, "SHOW CONFIGURATION;\n")
	hasConfiguration := true
	if showErr != nil {
		if strings.Contains(showOut, "ORA-16532") || strings.Contains(showErr.Error(), "ORA-16532") {
			hasConfiguration = false
		} else if strings.Contains(showOut, "ORA-16525") || strings.Contains(showErr.Error(), "ORA-16525") {
			return fmt.Errorf("oracle data guard broker is not yet available on primary member %s", state.Primary.Name)
		} else {
			return showErr
		}
	}

	if !hasConfiguration {
		script := buildDataguardTopologyCreateConfigurationScript(desired, state)
		if _, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, script); err != nil {
			return err
		}
		return nil
	}

	currentMembers, err := queryDataguardConfigurationMembers(ctx, r, broker, req, state.Primary)
	if err != nil {
		return err
	}
	if len(currentMembers) == 0 {
		script := buildDataguardTopologyCreateConfigurationScript(desired, state)
		if _, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, script); err != nil {
			return err
		}
		return nil
	}

	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	var missing []*dataguardTopologyResolvedMember
	for _, member := range state.DesiredStandbys {
		if _, ok := currentMembers[strings.ToUpper(member.DBUniqueName)]; ok {
			continue
		}
		if member.Role != "PHYSICAL_STANDBY" {
			return fmt.Errorf("topology member %q role %q is not supported for broker add-database workflow", member.Name, member.Role)
		}
		missing = append(missing, member)
	}
	if len(missing) == 0 {
		return nil
	}

	script := buildDataguardTopologyAddDatabaseScript(desired, currentPrimary, missing)
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, script)
	return err
}

func buildDataguardTopologyCreateConfigurationScript(desired *dataguardBrokerDesiredSpec, state *dataguardTopologyResolvedState) string {
	logXptMode := dataguardTopologyLogXptMode(desired)
	lines := []string{
		fmt.Sprintf("CREATE CONFIGURATION dg_config AS PRIMARY DATABASE IS %s CONNECT IDENTIFIER IS %s;", state.Primary.DBUniqueName, state.Primary.Alias),
	}
	for _, member := range state.DesiredStandbys {
		if member.Role != "PHYSICAL_STANDBY" {
			continue
		}
		lines = append(lines, fmt.Sprintf("ADD DATABASE %s AS CONNECT IDENTIFIER IS %s MAINTAINED AS PHYSICAL;", member.DBUniqueName, member.Alias))
	}
	for _, member := range state.DesiredPhysicalMembers {
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", member.DBUniqueName, logXptMode))
		if staticID := buildDataguardStaticConnectIdentifier(member); staticID != "" {
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY STATICCONNECTIDENTIFIER='%s';", member.DBUniqueName, staticID))
		}
	}
	lines = append(lines,
		fmt.Sprintf("EDIT CONFIGURATION SET PROTECTION MODE AS %s;", strings.ToUpper(strings.TrimSpace(firstNonEmptyString(desiredProtectionMode(desired), "MaxPerformance")))),
		"ENABLE CONFIGURATION;",
	)
	return strings.Join(lines, "\n") + "\n"
}

func buildDataguardTopologyAddDatabaseScript(desired *dataguardBrokerDesiredSpec, currentPrimary *dataguardTopologyResolvedMember, missing []*dataguardTopologyResolvedMember) string {
	logXptMode := dataguardTopologyLogXptMode(desired)
	var lines []string
	for _, member := range missing {
		lines = append(lines, fmt.Sprintf("ADD DATABASE %s AS CONNECT IDENTIFIER IS %s MAINTAINED AS PHYSICAL;", member.DBUniqueName, member.Alias))
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", member.DBUniqueName, logXptMode))
		if staticID := buildDataguardStaticConnectIdentifier(member); staticID != "" {
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY STATICCONNECTIDENTIFIER='%s';", member.DBUniqueName, staticID))
		}
	}
	if currentPrimary != nil {
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", currentPrimary.DBUniqueName, logXptMode))
	}
	lines = append(lines, "ENABLE CONFIGURATION;")
	return strings.Join(lines, "\n") + "\n"
}

func resolveCurrentDataguardTopologyPrimary(state *dataguardTopologyResolvedState, currentMembers map[string]string) *dataguardTopologyResolvedMember {
	for dbUniqueName, role := range currentMembers {
		if role != "PRIMARY" {
			continue
		}
		if member := state.MembersByDBUniqueName[strings.ToUpper(strings.TrimSpace(dbUniqueName))]; member != nil {
			return member
		}
	}
	return nil
}

func dataguardTopologyLogXptMode(desired *dataguardBrokerDesiredSpec) string {
	if desired != nil && strings.EqualFold(strings.TrimSpace(desired.ProtectionMode), "MaxAvailability") {
		return "SYNC"
	}
	return "ASYNC"
}

func desiredProtectionMode(desired *dataguardBrokerDesiredSpec) string {
	if desired == nil {
		return ""
	}
	if strings.TrimSpace(desired.ProtectionMode) == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(desired.ProtectionMode), "MaxAvailability") {
		return "MAXAVAILABILITY"
	}
	return "MAXPERFORMANCE"
}

func buildDataguardStaticConnectIdentifier(member *dataguardTopologyResolvedMember) string {
	if member == nil {
		return ""
	}
	protocol := strings.ToUpper(strings.TrimSpace(member.Endpoint.Protocol))
	if protocol == "" {
		protocol = "TCP"
	}
	serviceName := strings.ToUpper(strings.TrimSpace(member.Endpoint.ServiceName))
	if serviceName == "" {
		serviceName = member.DBUniqueName
	}
	descriptor := fmt.Sprintf("(DESCRIPTION=(ADDRESS=(PROTOCOL=%s)(HOST=%s)(PORT=%d))(CONNECT_DATA=(SERVICE_NAME=%s)(INSTANCE_NAME=%s)(SERVER=DEDICATED))",
		protocol, strings.TrimSpace(member.Endpoint.Host), member.Endpoint.Port, serviceName, member.DBUniqueName)
	if protocol == "TCPS" {
		descriptor += "(SECURITY="
		if strings.TrimSpace(member.SSLServerDN) != "" {
			descriptor += fmt.Sprintf("(SSL_SERVER_DN_MATCH=YES)(SSL_SERVER_CERT_DN=%s)", strings.TrimSpace(member.SSLServerDN))
		}
		if strings.TrimSpace(member.WalletDirectory) != "" {
			descriptor += fmt.Sprintf("(MY_WALLET_DIRECTORY=%s)", strings.TrimSpace(member.WalletDirectory))
		}
		descriptor += ")"
	}
	descriptor += "))"
	return descriptor
}

func configureDataguardTopologyFSFO(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, state *dataguardTopologyResolvedState) error {
	currentMembers, err := queryDataguardConfigurationMembers(ctx, r, broker, req, state.Primary)
	if err != nil {
		return err
	}
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	var lines []string
	for dbUniqueName := range currentMembers {
		targets := dataguardTopologyFSFOTargets(dbUniqueName, currentMembers)
		if targets == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY FASTSTARTFAILOVERTARGET='%s';", dbUniqueName, targets))
	}
	lines = append(lines, dbcommons.EnableFSFOCMD)
	if len(lines) == 1 {
		return nil
	}
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, strings.Join(lines, "\n")+"\n")
	return err
}

func disableDataguardTopologyFSFO(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, state *dataguardTopologyResolvedState) error {
	_, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, fmt.Sprintf(dbcommons.DisableFSFOCMD, broker.Name)+"\n")
	return err
}

func dataguardTopologyFSFOTargets(database string, members map[string]string) string {
	current := strings.ToUpper(strings.TrimSpace(database))
	var targets []string
	for dbUniqueName := range members {
		candidate := strings.ToUpper(strings.TrimSpace(dbUniqueName))
		if candidate == "" || candidate == current {
			continue
		}
		targets = append(targets, candidate)
	}
	sort.Strings(targets)
	return strings.Join(targets, ",")
}

func updateDataguardTopologyReconcileStatus(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, state *dataguardTopologyResolvedState) error {
	if state == nil || state.Primary == nil {
		return fmt.Errorf("topology state is incomplete")
	}

	currentMembers, err := queryDataguardConfigurationMembers(ctx, r, broker, req, state.Primary)
	if err != nil {
		return err
	}
	if len(currentMembers) == 0 {
		broker.Status.Status = dbcommons.StatusNotReady
		return nil
	}

	databasesInConfig := map[string]string{}
	var standbys []string
	currentPrimary := ""
	for dbUniqueName, role := range currentMembers {
		member := state.MembersByDBUniqueName[strings.ToUpper(dbUniqueName)]
		refName := dbUniqueName
		if member != nil && strings.TrimSpace(member.ResourceName) != "" {
			refName = member.ResourceName
		}
		databasesInConfig[strings.ToUpper(dbUniqueName)] = refName
		if role == "PRIMARY" {
			currentPrimary = strings.ToUpper(dbUniqueName)
		}
		if role == "PHYSICAL_STANDBY" || role == "SNAPSHOT_STANDBY" {
			standbys = append(standbys, strings.ToUpper(dbUniqueName))
		}
		if member != nil {
			if err := updateLocalSIDBDataguardMemberStatus(ctx, r, broker, member, role); err != nil {
				return err
			}
		}
	}
	sort.Strings(standbys)

	broker.Status.DatabasesInDataguardConfig = databasesInConfig
	broker.Status.PrimaryDatabase = currentPrimary
	broker.Status.PrimaryDatabaseRef = databasesInConfig[currentPrimary]
	broker.Status.StandbyDatabases = strings.Join(standbys, ",")
	broker.Status.ProtectionMode = desired.ProtectionMode
	broker.Status.Status = dbcommons.StatusReady

	currentPrimaryMember := state.MembersByDBUniqueName[currentPrimary]
	if currentPrimaryMember != nil {
		broker.Status.ClusterConnectString = currentPrimaryMember.ConnectString
		broker.Status.ExternalConnectString = currentPrimaryMember.ConnectString
	}

	if currentPrimaryMember != nil && currentPrimaryMember.LocalRef != nil &&
		(strings.TrimSpace(currentPrimaryMember.LocalRef.Kind) == "" || strings.EqualFold(strings.TrimSpace(currentPrimaryMember.LocalRef.Kind), "SingleInstanceDatabase")) {
		if err := patchService(r, broker, desired, ctx, req); err != nil {
			return err
		}
	}

	if desired != nil {
		for i := range broker.Status.ResolvedMembers {
			memberStatus := &broker.Status.ResolvedMembers[i]
			member := state.MembersByName[strings.ToLower(strings.TrimSpace(memberStatus.Name))]
			if member == nil {
				continue
			}
			memberStatus.ConnectString = member.ConnectString
			if role, ok := currentMembers[strings.ToUpper(member.DBUniqueName)]; ok {
				memberStatus.Role = role
				memberStatus.Phase = "Configured"
				memberStatus.Message = "member is present in broker configuration"
			}
		}
		for i := range broker.Status.ObservedPairs {
			pairStatus := &broker.Status.ObservedPairs[i]
			standby := state.MembersByName[strings.ToLower(strings.TrimSpace(pairStatus.Standby))]
			if standby == nil {
				continue
			}
			if _, ok := currentMembers[strings.ToUpper(standby.DBUniqueName)]; ok {
				pairStatus.State = "Configured"
				pairStatus.Message = "pair is present in broker configuration"
			}
		}
	}

	return nil
}

func updateLocalSIDBDataguardMemberStatus(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, member *dataguardTopologyResolvedMember, role string) error {
	if member == nil || member.LocalRef == nil {
		return nil
	}
	if kind := strings.TrimSpace(member.LocalRef.Kind); kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") {
		return nil
	}

	namespace := strings.TrimSpace(member.LocalRef.Namespace)
	if namespace == "" {
		namespace = broker.Namespace
	}
	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: strings.TrimSpace(member.LocalRef.Name)}, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	updated := false
	normalizedRole := strings.ToUpper(strings.TrimSpace(role))
	if sidb.Status.Role != normalizedRole {
		sidb.Status.Role = normalizedRole
		updated = true
	}
	if sidb.Status.DgBroker == nil || *sidb.Status.DgBroker != broker.Name {
		sidb.Status.DgBroker = &broker.Name
		updated = true
	}
	if !updated {
		return nil
	}
	return r.Status().Update(ctx, &sidb)
}

func cleanupDataguardTopologyBroker(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime)
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	if _, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, dbcommons.RemoveDataguardConfiguration+"\n"); err != nil {
		if strings.Contains(err.Error(), "ORA-16532") {
			return nil
		}
		return err
	}
	for _, member := range state.Members {
		if err := updateLocalSIDBCleanupStatus(ctx, r, member); err != nil {
			return err
		}
	}
	_ = desired
	return nil
}

func updateLocalSIDBCleanupStatus(ctx context.Context, r *DataguardBrokerReconciler, member *dataguardTopologyResolvedMember) error {
	if member == nil || member.LocalRef == nil {
		return nil
	}
	if kind := strings.TrimSpace(member.LocalRef.Kind); kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") {
		return nil
	}
	namespace := strings.TrimSpace(member.LocalRef.Namespace)
	name := strings.TrimSpace(member.LocalRef.Name)
	if namespace == "" || name == "" {
		return nil
	}
	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if sidb.Status.DgBroker == nil {
		return nil
	}
	sidb.Status.DgBroker = nil
	return r.Status().Update(ctx, &sidb)
}

func performDataguardTopologyManualSwitchover(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, targetDBUniqueName string) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime)
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	currentMembers, err := queryDataguardConfigurationMembers(ctx, r, broker, req, state.Primary)
	if err != nil {
		return err
	}
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, fmt.Sprintf("SWITCHOVER TO %s;\n", strings.ToUpper(strings.TrimSpace(targetDBUniqueName))))
	_ = desired
	return err
}

func oracleConnectDescriptor(user, password, alias string, asSysdba bool) string {
	passwordLiteral := strings.ReplaceAll(password, `"`, `\"`)
	connect := fmt.Sprintf(`%s/"%s"@%s`, user, passwordLiteral, alias)
	if asSysdba {
		connect += " as sysdba"
	}
	return connect
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func pathDir(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash <= 0 {
		return "."
	}
	return trimmed[:lastSlash]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func tcpsServerDN(tcps *dbapi.DataguardTCPSConfig) string {
	if tcps == nil {
		return ""
	}
	return strings.TrimSpace(tcps.SSLServerDN)
}

func normalizeTopologyMemberRole(role string) string {
	switch strings.ToUpper(strings.TrimSpace(role)) {
	case "PRIMARY":
		return "PRIMARY"
	case "PHYSICAL_STANDBY":
		return "PHYSICAL_STANDBY"
	case "SNAPSHOT_STANDBY":
		return "SNAPSHOT_STANDBY"
	default:
		return strings.ToUpper(strings.TrimSpace(role))
	}
}

func createDataguardTopologyObserverPod(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime)
	if err != nil {
		return err
	}
	currentMembers, err := queryDataguardConfigurationMembers(ctx, r, broker, req, state.Primary)
	if err != nil {
		return err
	}
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	_, brokerReplicasFound, _, _, err := dbcommons.FindPods(r, "", "", broker.Name, broker.Namespace, ctx, req)
	if err != nil {
		return err
	}
	if brokerReplicasFound > 0 {
		return nil
	}

	observerName := broker.Name + "-observer"
	tnsContent := []string{}
	for _, member := range state.Members {
		tnsContent = append(tnsContent, buildDataguardTopologyTNSAlias(member))
	}
	sort.Strings(tnsContent)
	sqlnetContent := buildDataguardTopologySQLNet(state)
	containerCommand := fmt.Sprintf(`mkdir -p %s
cat > %s/tnsnames.ora <<'__CODex_TNS__'
%s
__CODex_TNS__
cat > %s/sqlnet.ora <<'__CODex_SQLNET__'
%s
__CODex_SQLNET__
umask 177
cat > /tmp/admin.pwd <<'__CODex_PWD__'
%s
__CODex_PWD__
umask 022
trap 'rm -f /tmp/admin.pwd; exit 0' TERM INT
dgmgrl -echo sys@%s "START OBSERVER %s FILE IS /tmp/fsfo.dat LOGFILE IS /tmp/observer.log" < /tmp/admin.pwd
`, shellQuote("/tmp"), state.Runtime.TNSAdminPath, strings.Join(tnsContent, "\n"), state.Runtime.TNSAdminPath, sqlnetContent, currentPrimary.AdminPassword, currentPrimary.Alias, observerName)

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
		if strings.TrimSpace(secret) == "" {
			continue
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: strings.TrimSpace(secret)})
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observerName,
			Namespace: broker.Namespace,
			Labels: map[string]string{
				"app":     broker.Name,
				"version": strings.Join(runtime.ImagePullSecrets, ","),
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector:     cloneStringMap(broker.Spec.NodeSelector),
			ImagePullSecrets: imagePullSecrets,
			RestartPolicy:    corev1.RestartPolicyAlways,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: func() *int64 { v := int64(54321); return &v }(),
				FSGroup:   func() *int64 { v := int64(54321); return &v }(),
			},
			Volumes: volumes,
			Containers: []corev1.Container{{
				Name:            "observer",
				Image:           runtime.Image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"bash", "-c", containerCommand},
				Env: []corev1.EnvVar{{
					Name:  "TNS_ADMIN",
					Value: runtime.TNSAdminPath,
				}},
				VolumeMounts: volumeMounts,
			}},
		},
	}

	if err := ctrl.SetControllerReference(broker, &pod, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, &pod); err != nil {
		return err
	}
	return nil
}
