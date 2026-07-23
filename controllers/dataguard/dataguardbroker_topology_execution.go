package controllers

import (
	"context"
	"encoding/base64"
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var errDataguardBrokerRunnerUnavailable = errors.New("dataguard broker runner pod is unavailable")
var errDataguardTopologyLocalMemberNotReady = errors.New("dataguard topology local member is not ready")
var errDataguardTopologySwitchoverPending = errors.New("dataguard topology switchover is still in progress")
var errDataguardTopologyRoleConversionPending = errors.New("dataguard topology role conversion is still in progress")
var errDataguardTopologyConfigurationMissing = errors.New("dataguard broker configuration is missing")

type dataguardTopologyResolvedMember struct {
	Name            string
	Role            string
	DBUniqueName    string
	Alias           string
	StaticAlias     string
	ResourceName    string
	LocalRef        *dbapi.DataguardLocalRef
	Endpoint        dbapi.DataguardEndpointSpec
	ConnectString   string
	AdminSecretName string
	AdminSecretKey  string
	AdminPassword   string
	WalletDirectory string
	SSLServerDN     string
	UseAuthWallet   bool
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

func dataguardTopologyUsesTCPS(member *dbapi.DataguardTopologyMember) bool {
	return member != nil && member.TCPS != nil && member.TCPS.Enabled
}

func dataguardTopologyTransportProtocol(member *dbapi.DataguardTopologyMember) string {
	if dataguardTopologyUsesTCPS(member) {
		return "TCPS"
	}
	return "TCP"
}

func dataguardTopologyCanonicalPort(protocol string) int32 {
	if strings.EqualFold(strings.TrimSpace(protocol), "TCPS") {
		return dbcommons.CONTAINER_TCPS_PORT
	}
	return dbcommons.CONTAINER_LISTENER_PORT
}

func selectDataguardEndpointForTransport(member *dbapi.DataguardTopologyMember, protocol string) (*dbapi.DataguardEndpointSpec, error) {
	if member == nil {
		return nil, fmt.Errorf("topology member is nil")
	}
	normalizedProtocol := strings.ToUpper(strings.TrimSpace(protocol))
	if normalizedProtocol == "" {
		normalizedProtocol = "TCP"
	}

	var fallback *dbapi.DataguardEndpointSpec
	for i := range member.Endpoints {
		endpoint := &member.Endpoints[i]
		if fallback == nil && strings.TrimSpace(endpoint.Host) != "" {
			fallback = endpoint
		}
		if strings.EqualFold(strings.TrimSpace(endpoint.Protocol), normalizedProtocol) {
			return endpoint, nil
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("topology member %q does not declare a usable endpoint", strings.TrimSpace(member.Name))
}

func resolveDataguardTopologyState(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, requireAdminPasswords bool) (*dataguardTopologyResolvedState, error) {
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
		resolved, err := resolveDataguardTopologyMember(ctx, r, broker, runtime, &member, requireAdminPasswords)
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

	return state, nil
}

func resolveDataguardTopologyMember(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime, member *dbapi.DataguardTopologyMember, requireAdminPasswords bool) (*dataguardTopologyResolvedMember, error) {
	if member == nil {
		return nil, fmt.Errorf("topology member is nil")
	}

	role := normalizeTopologyMemberRole(member.Role)
	if role != "PRIMARY" && role != "PHYSICAL_STANDBY" && role != "SNAPSHOT_STANDBY" {
		return nil, fmt.Errorf("topology member %q uses unsupported role %q", strings.TrimSpace(member.Name), member.Role)
	}

	protocol := dataguardTopologyTransportProtocol(member)
	endpoint, err := selectDataguardEndpointForTransport(member, protocol)
	if err != nil {
		return nil, err
	}

	dbUniqueName := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
	if dbUniqueName == "" {
		dbUniqueName = strings.ToUpper(strings.TrimSpace(member.Name))
	}
	if err := dbapi.ValidateDataguardDBUniqueName(dbUniqueName); err != nil {
		return nil, fmt.Errorf("topology member %q resolves invalid DB_UNIQUE_NAME %q: %w", strings.TrimSpace(member.Name), dbUniqueName, err)
	}
	serviceName := strings.ToUpper(strings.TrimSpace(endpoint.ServiceName))
	if serviceName == "" {
		serviceName = dbUniqueName
	}
	if err := dbapi.ValidateDataguardServiceName(serviceName); err != nil {
		return nil, fmt.Errorf("topology member %q resolves invalid serviceName %q: %w", strings.TrimSpace(member.Name), serviceName, err)
	}
	host := strings.TrimSpace(endpoint.Host)
	if err := dbapi.ValidateDataguardHost(host); err != nil {
		return nil, fmt.Errorf("topology member %q resolves invalid host %q: %w", strings.TrimSpace(member.Name), host, err)
	}
	sslServerDN := firstNonEmptyString(strings.TrimSpace(endpoint.SSLServerDN), tcpsServerDN(member.TCPS))
	if err := dbapi.ValidateDataguardSingleLineText("sslServerDN", sslServerDN); err != nil {
		return nil, fmt.Errorf("topology member %q resolves invalid sslServerDN: %w", strings.TrimSpace(member.Name), err)
	}
	alias := dbUniqueName
	staticAlias := dbUniqueName + "_DGMGRL"
	if protocol == "TCPS" {
		alias += "TCPS"
		staticAlias = dbUniqueName + "TCPS_DGMGRL"
	}

	resolvedEndpoint := dbapi.DataguardEndpointSpec{
		Name:        strings.TrimSpace(endpoint.Name),
		Protocol:    protocol,
		Host:        host,
		Port:        dataguardTopologyCanonicalPort(protocol),
		ServiceName: serviceName,
		SSLServerDN: strings.TrimSpace(endpoint.SSLServerDN),
	}

	resolved := &dataguardTopologyResolvedMember{
		Name:          strings.TrimSpace(member.Name),
		Role:          role,
		DBUniqueName:  dbUniqueName,
		Alias:         strings.ToUpper(strings.TrimSpace(alias)),
		StaticAlias:   strings.ToUpper(strings.TrimSpace(staticAlias)),
		LocalRef:      member.LocalRef,
		Endpoint:      resolvedEndpoint,
		ConnectString: formatDataguardEndpointConnectString(&resolvedEndpoint),
		SSLServerDN:   sslServerDN,
		UseAuthWallet: runtime != nil && runtime.usesAuthWallet(),
	}

	if member.LocalRef != nil {
		resolved.ResourceName = strings.TrimSpace(member.LocalRef.Name)
	} else {
		resolved.ResourceName = resolved.Name
	}

	if requireAdminPasswords {
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
	}

	if protocol == "TCPS" {
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
	if localNamespace != broker.Namespace {
		return "", "", "", fmt.Errorf("topology member %q localRef.namespace %q must match DataguardBroker namespace %q when deriving admin secrets", strings.TrimSpace(member.Name), localNamespace, broker.Namespace)
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

func (r *DataguardBrokerReconciler) rebuildDataguardBrokerAuthWalletSecret(ctx context.Context, broker *dbapi.DataguardBroker, req ctrl.Request, runtime *dataguardBrokerExecutionRuntime, state *dataguardTopologyResolvedState, walletPassword string) error {
	if broker == nil || runtime == nil || state == nil {
		return fmt.Errorf("auth wallet runtime state is incomplete")
	}
	walletDir := "/tmp/dataguard-auth-wallet"
	command := buildDataguardBrokerAuthWalletBuildCommand(state, walletDir, walletPassword)
	if _, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command); err != nil {
		return fmt.Errorf("failed to build broker auth wallet: %w", err)
	}

	data := map[string][]byte{}
	for _, name := range []string{"cwallet.sso", "ewallet.p12"} {
		content, err := readBase64RunnerFile(ctx, r, broker, req, walletDir+"/"+name)
		if err != nil {
			return err
		}
		if len(content) == 0 {
			return fmt.Errorf("required auth wallet file %q was not created", name)
		}
		data[name] = content
	}

	secretName := dataguardBrokerAuthWalletSecretName(broker)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: broker.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = data
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels["database.oracle.com/managed-by"] = "dataguardbroker-controller"
		secret.Labels["database.oracle.com/auth-wallet"] = broker.Name
		return ctrl.SetControllerReference(broker, secret, r.Scheme)
	})
	return err
}

func buildDataguardBrokerAuthWalletBuildCommand(state *dataguardTopologyResolvedState, walletDir, walletPassword string) string {
	members := append([]*dataguardTopologyResolvedMember(nil), state.Members...)
	sort.Slice(members, func(i, j int) bool {
		return strings.ToUpper(strings.TrimSpace(members[i].DBUniqueName)) < strings.ToUpper(strings.TrimSpace(members[j].DBUniqueName))
	})

	lines := []string{
		"set -euo pipefail",
		fmt.Sprintf("WALLET_DIR=%s", shellQuote(walletDir)),
		"rm -rf \"$WALLET_DIR\"",
		"mkdir -p \"$WALLET_DIR\"",
		fmt.Sprintf("orapki wallet create -wallet \"$WALLET_DIR\" -pwd %s -auto_login", shellQuote(walletPassword)),
		"cat > \"$WALLET_DIR/.wallet.passwd\" <<'__DG_AUTH_WALLET_PWD__'",
		walletPassword,
		"__DG_AUTH_WALLET_PWD__",
	}
	for _, member := range members {
		if member == nil {
			continue
		}

		for _, alias := range []string{
			member.Alias,
			member.StaticAlias,
			member.ConnectString,
		} {
			if strings.TrimSpace(alias) == "" {
				continue
			}

			lines = append(lines, fmt.Sprintf(
				"mkstore -wrl \"$WALLET_DIR\" -createCredential %s %s %s < \"$WALLET_DIR/.wallet.passwd\" >/dev/null",
				shellQuote(strings.TrimSpace(alias)),
				shellQuote("sys"),
				shellQuote(member.AdminPassword),
			))
		}

		// mkstore requires the raw descriptor, not the quoted DGMGRL literal.
		staticDescriptor, err := buildDataguardStaticConnectDescriptor(member)
		if err != nil || strings.TrimSpace(staticDescriptor) == "" {
			continue
		}

		lines = append(lines, fmt.Sprintf(
			"STATIC_CONNECT_IDENTIFIER=%s",
			shellQuote(staticDescriptor),
		))

		lines = append(lines, fmt.Sprintf(
			"mkstore -wrl \"$WALLET_DIR\" -createCredential \"$STATIC_CONNECT_IDENTIFIER\" %s %s < \"$WALLET_DIR/.wallet.passwd\" >/dev/null",
			shellQuote("sys"),
			shellQuote(member.AdminPassword),
		))
	}
	lines = append(lines, "rm -f \"$WALLET_DIR/.wallet.passwd\"")
	return strings.Join(lines, "\n") + "\n"
}

func readBase64RunnerFile(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, path string) ([]byte, error) {
	out, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, fmt.Sprintf("if [ -f %s ]; then base64 -w0 %s; fi", shellQuote(path), shellQuote(path)))
	if err != nil {
		return nil, err
	}
	encoded := strings.TrimSpace(out)
	if encoded == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(encoded)
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
			return fmt.Errorf("%w: local member %s does not have a ready database pod yet", errDataguardTopologyLocalMemberNotReady, member.Name)
		}

		command := dbcommons.BuildDataguardPrereqsCommand(
			"configure",
			dbapi.DataguardProducerBrokerConfigDir(sidb.Spec.Dataguard),
			dbapi.DataguardProducerStandbyRedoSize(sidb.Spec.Dataguard),
		)
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", command)
		if err != nil {
			combined := out + " " + err.Error()

			if strings.Contains(combined, "ORA-16573") {
				r.Log.Info(
					"Broker configuration already enabled; continuing after Data Guard prerequisite reconfiguration",
					"member", member.Name,
					"pod", readyPod.Name,
					"output", out,
					"error", err.Error(),
				)

				if isDataguardTopologyPhysicalStandbyMember(member, out) {
					if applyErr := restartDataguardTopologyStandbyApply(ctx, r, broker, req, state, member); applyErr != nil {
						r.Log.Info(
							"Unable to restart broker-managed standby apply after ORA-16573",
							"member", member.Name,
							"pod", readyPod.Name,
							"error", applyErr.Error(),
						)
					}
				}
			} else {
				return fmt.Errorf("failed to configure Data Guard prerequisites for local member %s: %w", member.Name, err)
			}
		} else {
			r.Log.Info("Configured local database Data Guard prerequisites", "member", member.Name, "pod", readyPod.Name, "output", out)
		}
		logArchiveConfigCommand, err := buildDataguardTopologyLogArchiveConfigCommand(state, sidb.Spec.Edition)
		if err != nil {
			return fmt.Errorf("failed to build log_archive_config command for local member %s: %w", member.Name, err)
		}
		out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", logArchiveConfigCommand)
		if err != nil {
			return fmt.Errorf("failed to configure log_archive_config for local member %s: %w", member.Name, err)
		}
		r.Log.Info("Configured log_archive_config for local database member", "member", member.Name, "pod", readyPod.Name, "output", out)
	}

	return nil
}

func isDataguardTopologyPhysicalStandbyMember(member *dataguardTopologyResolvedMember, out string) bool {
	if member != nil && strings.EqualFold(strings.TrimSpace(member.Role), "PHYSICAL_STANDBY") {
		return true
	}

	upperOut := strings.ToUpper(out)
	return strings.Contains(upperOut, "DATABASE_ROLE=PHYSICAL STANDBY") ||
		strings.Contains(upperOut, "PHYSICAL STANDBY")
}

func restartDataguardTopologyStandbyApply(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	state *dataguardTopologyResolvedState,
	standby *dataguardTopologyResolvedMember,
) error {
	if r == nil || broker == nil || state == nil || state.Primary == nil || standby == nil {
		return nil
	}

	currentPrimary := state.Primary

	currentMembers, connectedMember, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err == nil && len(currentMembers) > 0 {
		if resolvedPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers); resolvedPrimary != nil {
			currentPrimary = resolvedPrimary
		} else if connectedMember != nil {
			currentPrimary = connectedMember
		}
	} else if err != nil {
		r.Log.Info(
			"Unable to resolve current primary before restarting standby apply; using configured primary",
			"standby", standby.Name,
			"error", err.Error(),
		)
	}

	standbyDBUniqueName, err := dataguardDGMGRLIdentifier(standby.DBUniqueName)
	if err != nil {
		return fmt.Errorf("invalid standby database identifier for member %s: %w", standby.Name, err)
	}

	script := fmt.Sprintf("EDIT DATABASE %s SET STATE='APPLY-ON';\n", standbyDBUniqueName)

	out, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, script)
	if err != nil {
		return fmt.Errorf("failed to restart broker-managed apply for standby %s: %w; output: %s", standby.Name, err, out)
	}

	r.Log.Info(
		"Restarted broker-managed standby apply",
		"member", standby.Name,
		"dbUniqueName", standby.DBUniqueName,
		"output", out,
	)

	return nil
}

func isDataguardTopologyLocalMemberNotReady(err error) bool {
	return errors.Is(err, errDataguardTopologyLocalMemberNotReady)
}

func buildDataguardTopologyLogArchiveConfigCommand(state *dataguardTopologyResolvedState, edition string) (string, error) {
	sql, err := buildDataguardTopologyLogArchiveConfigSQL(state)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("echo -e  %s  | %s",
		dbcommons.ShellQuote(sql),
		dbcommons.GetSqlClient(strings.ToLower(strings.TrimSpace(edition))),
	), nil
}

func buildDataguardTopologyLogArchiveConfigSQL(state *dataguardTopologyResolvedState) (string, error) {
	config, err := buildDataguardTopologyLogArchiveConfigValue(state)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER SYSTEM SET log_archive_config='dg_config=(%s)' scope=both sid='*';\nSHOW PARAMETER log_archive_config;", config), nil
}

func buildDataguardTopologyLogArchiveConfigValue(state *dataguardTopologyResolvedState) (string, error) {
	if state == nil {
		return "", nil
	}
	seen := make(map[string]struct{}, len(state.Members))
	members := make([]string, 0, len(state.Members))
	for _, member := range state.Members {
		if member == nil {
			continue
		}
		dbUniqueName := strings.TrimSpace(member.DBUniqueName)
		if dbUniqueName == "" {
			continue
		}
		dbUniqueName, err := dataguardDGMGRLIdentifier(dbUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid log_archive_config DB_UNIQUE_NAME for member %q: %w", member.Name, err)
		}
		if _, ok := seen[dbUniqueName]; ok {
			continue
		}
		seen[dbUniqueName] = struct{}{}
		members = append(members, dbUniqueName)
	}
	sort.Strings(members)
	return strings.Join(members, ","), nil
}

func ensureDataguardTopologyNetConfiguration(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, state *dataguardTopologyResolvedState, req ctrl.Request) error {
	if broker == nil || state == nil || state.Runtime == nil {
		return fmt.Errorf("topology runtime state is incomplete")
	}
	tnsFile := strings.TrimRight(state.Runtime.TNSAdminPath, "/") + "/tnsnames.ora"
	sqlnetFile := strings.TrimRight(state.Runtime.TNSAdminPath, "/") + "/sqlnet.ora"

	var entries []string
	for _, member := range state.Members {
		aliases, err := buildDataguardTopologyTNSAliases(member)
		if err != nil {
			return err
		}
		entries = append(entries, aliases...)
	}
	sort.Strings(entries)
	if err := writeDataguardRunnerFile(ctx, r, broker, req, tnsFile, strings.Join(entries, "\n")); err != nil {
		return err
	}
	return writeDataguardRunnerFile(ctx, r, broker, req, sqlnetFile, buildDataguardTopologySQLNet(state))
}

func buildDataguardTopologyTNSAliasEntry(alias string, member *dataguardTopologyResolvedMember, serviceName string) (string, error) {
	if member == nil {
		return "", nil
	}
	protocol := strings.ToUpper(strings.TrimSpace(member.Endpoint.Protocol))
	if protocol == "" {
		protocol = "TCP"
	}
	serviceName = strings.ToUpper(strings.TrimSpace(serviceName))
	if serviceName == "" {
		serviceName = member.DBUniqueName
	}
	serviceName, err := dataguardTNSServiceName(serviceName)
	if err != nil {
		return "", fmt.Errorf("invalid TNS service name for member %q: %w", member.Name, err)
	}
	host, err := dataguardTNSHost(member.Endpoint.Host)
	if err != nil {
		return "", fmt.Errorf("invalid TNS host for member %q: %w", member.Name, err)
	}
	entry := fmt.Sprintf(`%s =
(DESCRIPTION =
  (ADDRESS = (PROTOCOL = %s)(HOST = %s)(PORT = %d))
  (CONNECT_DATA =
    (SERVER = DEDICATED)
    (SERVICE_NAME = %s)
  )`, alias, protocol, host, member.Endpoint.Port, serviceName)

	if protocol == "TCPS" {
		entry += `
  (SECURITY =`
		sslServerDN := strings.TrimSpace(member.SSLServerDN)
		if err := dbapi.ValidateDataguardSingleLineText("sslServerDN", sslServerDN); err != nil {
			return "", fmt.Errorf("invalid TNS sslServerDN for member %q: %w", member.Name, err)
		}
		if sslServerDN != "" {
			entry += fmt.Sprintf(`
    (SSL_SERVER_DN_MATCH = NO)
    (SSL_SERVER_CERT_DN = %s)`, sslServerDN)
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
	return entry, nil
}

func buildDataguardTopologyTNSAliases(member *dataguardTopologyResolvedMember) ([]string, error) {
	if member == nil {
		return nil, nil
	}
	normalService := strings.ToUpper(strings.TrimSpace(member.Endpoint.ServiceName))
	if normalService == "" {
		normalService = member.DBUniqueName
	}
	staticService := strings.ToUpper(strings.TrimSpace(member.DBUniqueName)) + "_DGMGRL"
	normalAlias, err := buildDataguardTopologyTNSAliasEntry(member.Alias, member, normalService)
	if err != nil {
		return nil, err
	}
	staticAlias, err := buildDataguardTopologyTNSAliasEntry(member.StaticAlias, member, staticService)
	if err != nil {
		return nil, err
	}
	return []string{normalAlias, staticAlias}, nil
}

func dataguardTNSServiceName(value string) (string, error) {
	serviceName := strings.ToUpper(strings.TrimSpace(value))
	if err := dbapi.ValidateDataguardServiceName(serviceName); err != nil {
		return "", err
	}
	return serviceName, nil
}

func dataguardTNSHost(value string) (string, error) {
	host := strings.TrimSpace(value)
	if err := dbapi.ValidateDataguardHost(host); err != nil {
		return "", err
	}
	return host, nil
}

func buildDataguardTopologySQLNet(state *dataguardTopologyResolvedState) string {
	lines := []string{
		"NAMES.DIRECTORY_PATH=(TNSNAMES,EZCONNECT)",
		"DIAG_ADR_ENABLED=OFF",
	}
	if state != nil && state.Runtime != nil && state.Runtime.usesAuthWallet() {
		lines = append(lines,
			fmt.Sprintf("WALLET_LOCATION=(SOURCE=(METHOD=FILE)(METHOD_DATA=(DIRECTORY=%s)))", strings.TrimSpace(state.Runtime.AuthWalletMountPath)),
			"SQLNET.WALLET_OVERRIDE=TRUE",
		)
	}
	if topologyUsesTCPS(state) {
		lines = append(lines, "SSL_SERVER_DN_MATCH=NO")
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
	command := buildDataguardWriteFileCommand(path, content+"\n")
	_, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
	if err != nil {
		return fmt.Errorf("failed to write runner file %s: %w", path, err)
	}
	return nil
}

func buildDataguardWriteFileCommand(path, content string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	return fmt.Sprintf("mkdir -p %s && printf %%s %s | base64 -d > %s\n", shellQuote(strings.TrimRight(pathDir(path), "/")), shellQuote(encoded), shellQuote(path))
}

func writeDataguardRunnerSecretFile(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, path, content string) error {
	command := buildDataguardStdinWriteFileCommand(path)
	_, err := execDataguardBrokerRunnerShellWithInput(ctx, r, broker, req, true, content+"\n", command)
	if err != nil {
		return fmt.Errorf("failed to write runner secret file %s: %w", path, err)
	}
	return nil
}

func buildDataguardStdinWriteFileCommand(path string) string {
	return fmt.Sprintf("mkdir -p %s && umask 177 && cat > %s", shellQuote(strings.TrimRight(pathDir(path), "/")), shellQuote(path))
}

func resolveDataguardBrokerActiveRunnerPod(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker) (*corev1.Pod, error) {
	if broker == nil {
		return nil, fmt.Errorf("%w: broker is nil", errDataguardBrokerRunnerUnavailable)
	}
	runtime, ready, message, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, fmt.Errorf("%w: %s", errDataguardBrokerRunnerUnavailable, message)
	}
	runtimeHash := computeDataguardBrokerRunnerRuntimeHash(broker, runtime)
	podName := dataguardBrokerRunnerPodNameForHash(broker, runtimeHash)

	var pod corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: broker.Namespace}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: pod %s/%s not found", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
		}
		return nil, err
	}
	if !pod.DeletionTimestamp.IsZero() {
		return nil, fmt.Errorf("%w: pod %s/%s is being deleted", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
	}
	if err := validateDataguardBrokerRunnerPodIdentity(&pod, broker, runtimeHash); err != nil {
		return nil, fmt.Errorf("%w: pod %s/%s has invalid identity: %v", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName, err)
	}
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("%w: pod %s/%s is in phase %s", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName, pod.Status.Phase)
	}
	return &pod, nil
}

func execDataguardBrokerRunnerShell(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, nolog bool, command string) (string, error) {
	if broker == nil {
		return "", fmt.Errorf("%w: broker is nil", errDataguardBrokerRunnerUnavailable)
	}
	pod, err := resolveDataguardBrokerActiveRunnerPod(ctx, r, broker)
	if err != nil {
		return "", err
	}
	podName := pod.Name
	out, err := dbcommons.ExecCommand(r, r.Config, podName, broker.Namespace, dataguardBrokerRunnerContainerName, ctx, req, nolog, "bash", "-c", command)
	if err != nil {
		if apierrors.IsNotFound(err) || strings.Contains(err.Error(), fmt.Sprintf("pods %q not found", podName)) {
			return out, fmt.Errorf("%w: pod %s/%s not found", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
		}
	}
	return out, err
}

func execDataguardBrokerRunnerShellWithInput(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, nolog bool, input, command string) (string, error) {
	if broker == nil {
		return "", fmt.Errorf("%w: broker is nil", errDataguardBrokerRunnerUnavailable)
	}
	pod, err := resolveDataguardBrokerActiveRunnerPod(ctx, r, broker)
	if err != nil {
		return "", err
	}
	podName := pod.Name
	out, err := dbcommons.ExecCommandWithInput(r, r.Config, podName, broker.Namespace, dataguardBrokerRunnerContainerName, ctx, req, nolog, input, "bash", "-c", command)
	if err != nil {
		if apierrors.IsNotFound(err) || strings.Contains(err.Error(), fmt.Sprintf("pods %q not found", podName)) {
			return out, fmt.Errorf("%w: pod %s/%s not found", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
		}
	}
	return out, err
}

func isDataguardBrokerRunnerUnavailable(err error) bool {
	return errors.Is(err, errDataguardBrokerRunnerUnavailable)
}

func runDataguardBrokerRunnerDGMGRLScript(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, connectMember *dataguardTopologyResolvedMember, script string) (string, error) {
	if connectMember == nil {
		return "", fmt.Errorf("runner dgmgrl member is nil")
	}
	scriptPath := "/tmp/dgmgrl-topology.cmd"
	scriptContent := buildDataguardRunnerDGMGRLScript(connectMember, script)
	writeFile := writeDataguardRunnerFile
	if !connectMember.UseAuthWallet {
		writeFile = writeDataguardRunnerSecretFile
	}
	if err := writeFile(ctx, r, broker, req, scriptPath, scriptContent); err != nil {
		return "", err
	}
	command := mirrorDataguardRunnerCommandToContainerLogs(
		fmt.Sprintf("dgmgrl -silent @%s", shellQuote(scriptPath)),
		fmt.Sprintf("rm -f %s", shellQuote(scriptPath)),
	)
	return execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
}

func openDataguardTopologyPDBs(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	member *dataguardTopologyResolvedMember,
) error {
	if member == nil {
		return fmt.Errorf("topology member is nil")
	}

	// PDB recovery applies only to local SIDB members.
	if member.LocalRef == nil {
		return nil
	}

	kind := strings.TrimSpace(member.LocalRef.Kind)
	if kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") {
		return nil
	}

	namespace := strings.TrimSpace(member.LocalRef.Namespace)
	if namespace == "" {
		namespace = broker.Namespace
	}

	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(
		ctx,
		types.NamespacedName{
			Namespace: namespace,
			Name:      strings.TrimSpace(member.LocalRef.Name),
		},
		&sidb,
	); err != nil {
		return fmt.Errorf(
			"failed to resolve local SIDB %q for PDB recovery: %w",
			member.Name,
			err,
		)
	}

	// Nothing to open for a non-CDB SIDB.
	if strings.TrimSpace(sidb.Spec.Pdbname) == "" &&
		strings.TrimSpace(sidb.Status.Pdbname) == "" {
		return nil
	}

	alias := strings.TrimSpace(member.Alias)
	if alias == "" {
		return fmt.Errorf(
			"topology member %q does not have a connect alias",
			member.Name,
		)
	}

	connectArg := oracleConnectDescriptor(
		"sys",
		member.AdminPassword,
		alias,
		true,
		member.UseAuthWallet,
	)

	script := fmt.Sprintf(`connect %s
whenever sqlerror exit sql.sqlcode
alter pluggable database all open;
exit
`, connectArg)

	scriptPath := "/tmp/dg-open-pdbs.sql"
	writeFile := writeDataguardRunnerFile
	if !member.UseAuthWallet {
		writeFile = writeDataguardRunnerSecretFile
	}

	if err := writeFile(
		ctx,
		r,
		broker,
		req,
		scriptPath,
		script,
	); err != nil {
		return err
	}

	command := mirrorDataguardRunnerCommandToContainerLogs(
		fmt.Sprintf("sqlplus -s /nolog @%s", shellQuote(scriptPath)),
		fmt.Sprintf("rm -f %s", shellQuote(scriptPath)),
	)

	out, err := execDataguardBrokerRunnerShell(
		ctx,
		r,
		broker,
		req,
		true,
		command,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to open PDBs for topology member %q: %w",
			member.DBUniqueName,
			err,
		)
	}

	if strings.Contains(strings.ToUpper(out), "ORA-") {
		return fmt.Errorf(
			"failed to open PDBs for topology member %q: %s",
			member.DBUniqueName,
			strings.TrimSpace(out),
		)
	}

	return nil
}

func buildDataguardRunnerDGMGRLScript(connectMember *dataguardTopologyResolvedMember, script string) string {
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, connectMember.Alias, false, connectMember.UseAuthWallet)
	return fmt.Sprintf("CONNECT %s;\n%s", connectArg, script)
}

func queryDataguardConfigurationMembers(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, connectMember *dataguardTopologyResolvedMember) (map[string]string, error) {
	if connectMember == nil {
		return nil, fmt.Errorf("query member is nil")
	}
	scriptPath := "/tmp/dg-broker-members.sql"
	script := buildDataguardConfigurationMembersSQLScript(connectMember)
	writeFile := writeDataguardRunnerFile
	if !connectMember.UseAuthWallet {
		writeFile = writeDataguardRunnerSecretFile
	}
	if err := writeFile(ctx, r, broker, req, scriptPath, script); err != nil {
		return nil, err
	}
	command := mirrorDataguardRunnerCommandToContainerLogs(
		fmt.Sprintf("sqlplus -s /nolog @%s", shellQuote(scriptPath)),
		fmt.Sprintf("rm -f %s", shellQuote(scriptPath)),
	)
	out, err := execDataguardBrokerRunnerShell(ctx, r, broker, req, true, command)
	if dataguardBrokerConfigurationMissing(out) || (err != nil && dataguardBrokerConfigurationMissing(err.Error())) {
		return nil, fmt.Errorf("%w: query through %s", errDataguardTopologyConfigurationMissing, connectMember.DBUniqueName)
	}
	if err != nil {
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

func dataguardBrokerOutputError(out string) error {
	for _, code := range []string{"ORA-16532", "ORA-16584", "ORA-16525", "ORA-16596"} {
		if strings.Contains(out, code) {
			return fmt.Errorf("dgmgrl returned %s", code)
		}
	}
	return nil
}

func dataguardBrokerConfigurationMissing(out string) bool {
	return strings.Contains(out, "ORA-16532") ||
		strings.Contains(out, "ORA-16596")
}

func runDataguardTopologyDGMGRLScript(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, state *dataguardTopologyResolvedState, script string) (string, *dataguardTopologyResolvedMember, error) {
	if state == nil {
		return "", nil, fmt.Errorf("topology state is incomplete")
	}

	candidates := make([]*dataguardTopologyResolvedMember, 0, len(state.Members)+1)
	seen := map[string]struct{}{}
	addCandidate := func(member *dataguardTopologyResolvedMember) {
		if member == nil {
			return
		}
		key := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(member.Name))
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, member)
	}
	addCandidate(state.Primary)
	for _, member := range state.Members {
		addCandidate(member)
	}

	var lastOut string
	var lastErr error
	for _, candidate := range candidates {
		out, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, candidate, script)
		if err == nil {
			err = dataguardBrokerOutputError(out)
		}
		if err == nil {
			return out, candidate, nil
		}
		lastOut = out
		lastErr = err
	}
	return lastOut, nil, lastErr
}

// queryDataguardTopologyConfigurationMembers tolerates the configured primary
// being unavailable while a broker switchover is changing database roles.
func queryDataguardTopologyConfigurationMembers(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, state *dataguardTopologyResolvedState) (map[string]string, *dataguardTopologyResolvedMember, error) {
	if state == nil {
		return nil, nil, fmt.Errorf("topology state is incomplete")
	}

	candidates := make([]*dataguardTopologyResolvedMember, 0, len(state.Members)+1)
	seen := map[string]struct{}{}
	addCandidate := func(member *dataguardTopologyResolvedMember) {
		if member == nil {
			return
		}
		key := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(member.Name))
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, member)
	}
	addCandidate(state.Primary)
	for _, member := range state.Members {
		addCandidate(member)
	}

	var lastErr error
	for _, candidate := range candidates {
		members, err := queryDataguardConfigurationMembers(ctx, r, broker, req, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		if len(members) > 0 {
			return members, candidate, nil
		}
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return map[string]string{}, nil, nil
}

type dataguardTopologyObservedConfiguration struct {
	ProtectionMode      string
	ConfigurationStatus string
}

func queryDataguardTopologyObservedConfiguration(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, connectMember *dataguardTopologyResolvedMember) (*dataguardTopologyObservedConfiguration, error) {
	out, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, connectMember, "SHOW CONFIGURATION VERBOSE;\n")
	if err == nil {
		err = dataguardBrokerOutputError(out)
	}
	if err != nil {
		return nil, err
	}
	return parseDataguardTopologyObservedConfiguration(out), nil
}

func parseDataguardTopologyObservedConfiguration(out string) *dataguardTopologyObservedConfiguration {
	observed := &dataguardTopologyObservedConfiguration{}
	pending := ""
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if pending != "" {
			switch pending {
			case "configurationStatus":
				observed.ConfigurationStatus = trimmed
			}
			pending = ""
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "protection mode:"):
			observed.ProtectionMode = strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
		case strings.HasPrefix(lower, "configuration status:"):
			value := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
			if value == "" {
				pending = "configurationStatus"
			} else {
				observed.ConfigurationStatus = value
			}
		}
	}
	return observed
}

func buildDataguardConfigurationMembersSQLScript(connectMember *dataguardTopologyResolvedMember) string {
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, connectMember.Alias, true, connectMember.UseAuthWallet)
	return fmt.Sprintf(`connect %s
set heading off
set feedback off
set verify off
set echo off
set pages 0
set lines 400
SELECT DATABASE || ':' || DATAGUARD_ROLE FROM V$DG_BROKER_CONFIG ORDER BY DATABASE;
exit
`, connectArg)
}

func mirrorDataguardRunnerCommandToContainerLogs(command, cleanup string) string {
	cleanup = strings.TrimSpace(cleanup)
	if cleanup == "" {
		cleanup = ":"
	}
	return fmt.Sprintf(
		"command_status=0; { %s; } > >(tee /proc/1/fd/1) 2> >(tee /proc/1/fd/2 >&2) || command_status=$?; %s; exit $command_status",
		command,
		cleanup,
	)
}

func ensureDataguardTopologyBrokerConfiguration(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	desired *dataguardBrokerDesiredSpec,
	req ctrl.Request,
	state *dataguardTopologyResolvedState,
) error {
	if state == nil || state.Primary == nil {
		return fmt.Errorf("topology state is incomplete")
	}

	// Always keep runner network files current. This does not require DB pods.
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}

	showOut, _, showErr := runDataguardTopologyDGMGRLScript(ctx, r, broker, req, state, "SHOW CONFIGURATION;\n")
	hasConfiguration := true
	if showErr != nil {
		combined := showOut + " " + showErr.Error()

		if dataguardBrokerConfigurationMissing(combined) {
			hasConfiguration = false
		} else if strings.Contains(combined, "ORA-16525") {
			return fmt.Errorf("oracle data guard broker is not yet available on primary member %s", state.Primary.Name)
		} else {
			return showErr
		}
	}

	if !hasConfiguration {
		if dataguardBrokerConfigurationPreviouslyObserved(broker) {
			return fmt.Errorf("%w after broker configuration was previously observed", errDataguardTopologyConfigurationMissing)
		}
		// Initial create path only. Local DB prereqs are needed here.
		if err := ensureDataguardTopologyLocalDatabasePrereqs(ctx, r, broker, state, req); err != nil {
			return err
		}

		if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
			return err
		}

		script, err := buildDataguardTopologyCreateConfigurationScript(desired, state)
		if err != nil {
			return err
		}
		if _, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, script); err != nil {
			return err
		}
	}

	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}

	if len(currentMembers) == 0 {
		return fmt.Errorf("%w after broker query returned no members", errDataguardTopologyConfigurationMissing)
	}

	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	var missing []*dataguardTopologyResolvedMember
	for _, member := range state.DesiredStandbys {
		if member == nil {
			continue
		}
		if _, ok := currentMembers[strings.ToUpper(member.DBUniqueName)]; ok {
			continue
		}
		if member.Role != "PHYSICAL_STANDBY" {
			return fmt.Errorf("topology member %q role %q is not supported for broker add-database workflow", member.Name, member.Role)
		}
		missing = append(missing, member)
	}

	if len(missing) == 0 {
		// Existing broker config path.
		// Do not rerun local DB prereqs here. This allows switchover status refresh
		// without requiring the original primary pod to be Ready.
		return reconcileDataguardTopologyConnectIdentifiers(ctx, r, broker, req, currentPrimary, state, currentMembers)
	}

	// Add-missing-members path. Local prereqs are needed only when changing membership.
	if err := ensureDataguardTopologyLocalDatabasePrereqs(ctx, r, broker, state, req); err != nil {
		return err
	}

	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}

	script, err := buildDataguardTopologyAddDatabaseScript(desired, currentPrimary, missing)
	if err != nil {
		return err
	}
	if _, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, script); err != nil {
		return err
	}

	currentMembers, _, err = queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	if len(currentMembers) == 0 {
		return nil
	}

	currentPrimary = resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	return reconcileDataguardTopologyConnectIdentifiers(ctx, r, broker, req, currentPrimary, state, currentMembers)
}

func buildDataguardTopologyCreateConfigurationScript(desired *dataguardBrokerDesiredSpec, state *dataguardTopologyResolvedState) (string, error) {
	logXptMode := dataguardTopologyLogXptMode(desired)
	primaryDBUniqueName, err := dataguardDGMGRLIdentifier(state.Primary.DBUniqueName)
	if err != nil {
		return "", fmt.Errorf("invalid primary database identifier: %w", err)
	}
	primaryConnectID, err := dataguardDGMGRLConnectIdentifier(state.Primary)
	if err != nil {
		return "", fmt.Errorf("invalid primary connect identifier: %w", err)
	}
	lines := []string{
		fmt.Sprintf("CREATE CONFIGURATION dg_config AS PRIMARY DATABASE IS %s CONNECT IDENTIFIER IS %s;", primaryDBUniqueName, primaryConnectID),
	}
	for _, member := range state.DesiredStandbys {
		if member.Role != "PHYSICAL_STANDBY" {
			continue
		}
		memberDBUniqueName, err := dataguardDGMGRLIdentifier(member.DBUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid standby database identifier: %w", err)
		}
		memberConnectID, err := dataguardDGMGRLConnectIdentifier(member)
		if err != nil {
			return "", fmt.Errorf("invalid standby connect identifier: %w", err)
		}
		lines = append(lines, fmt.Sprintf("ADD DATABASE %s AS CONNECT IDENTIFIER IS %s;", memberDBUniqueName, memberConnectID))
	}
	for _, member := range state.DesiredPhysicalMembers {
		memberDBUniqueName, err := dataguardDGMGRLIdentifier(member.DBUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid database identifier: %w", err)
		}
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", memberDBUniqueName, logXptMode))
		staticID, err := buildDataguardStaticConnectIdentifier(member)
		if err != nil {
			return "", err
		}
		if staticID != "" {
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY STATICCONNECTIDENTIFIER=%s;", memberDBUniqueName, staticID))
		}
	}
	lines = append(lines,
		fmt.Sprintf("EDIT CONFIGURATION SET PROTECTION MODE AS %s;", strings.ToUpper(strings.TrimSpace(firstNonEmptyString(desiredProtectionMode(desired), "MaxPerformance")))),
		"ENABLE CONFIGURATION;",
	)
	return strings.Join(lines, "\n") + "\n", nil
}

func buildDataguardTopologyAddDatabaseScript(desired *dataguardBrokerDesiredSpec, currentPrimary *dataguardTopologyResolvedMember, missing []*dataguardTopologyResolvedMember) (string, error) {
	logXptMode := dataguardTopologyLogXptMode(desired)
	var lines []string
	for _, member := range missing {
		memberDBUniqueName, err := dataguardDGMGRLIdentifier(member.DBUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid missing database identifier: %w", err)
		}
		memberConnectID, err := dataguardDGMGRLConnectIdentifier(member)
		if err != nil {
			return "", fmt.Errorf("invalid missing connect identifier: %w", err)
		}
		lines = append(lines, fmt.Sprintf("ADD DATABASE %s AS CONNECT IDENTIFIER IS %s;", memberDBUniqueName, memberConnectID))
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", memberDBUniqueName, logXptMode))
		staticID, err := buildDataguardStaticConnectIdentifier(member)
		if err != nil {
			return "", err
		}
		if staticID != "" {
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY STATICCONNECTIDENTIFIER=%s;", memberDBUniqueName, staticID))
		}
	}
	if currentPrimary != nil {
		currentPrimaryDBUniqueName, err := dataguardDGMGRLIdentifier(currentPrimary.DBUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid current primary database identifier: %w", err)
		}
		lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY LogXptMode='%s';", currentPrimaryDBUniqueName, logXptMode))
	}
	lines = append(lines, "ENABLE CONFIGURATION;")
	return strings.Join(lines, "\n") + "\n", nil
}

func dataguardBrokerConfigurationPreviouslyObserved(broker *dbapi.DataguardBroker) bool {
	if broker == nil {
		return false
	}
	return strings.TrimSpace(broker.Status.PrimaryDatabase) != "" ||
		strings.TrimSpace(broker.Status.PrimaryDatabaseRef) != "" ||
		len(broker.Status.DatabasesInDataguardConfig) > 0
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

func buildDataguardStaticConnectDescriptor(member *dataguardTopologyResolvedMember) (string, error) {
	if member == nil {
		return "", nil
	}

	host, err := dataguardTNSHost(member.Endpoint.Host)
	if err != nil {
		return "", fmt.Errorf("invalid static connect identifier for member %q: %w", member.Name, err)
	}

	serviceName, err := dataguardTNSServiceName(
		strings.ToUpper(strings.TrimSpace(member.DBUniqueName)) + "_DGMGRL",
	)
	if err != nil {
		return "", fmt.Errorf(
			"invalid static connect identifier service for member %q: %w",
			member.Name,
			err,
		)
	}

	protocol := strings.ToUpper(strings.TrimSpace(member.Endpoint.Protocol))
	if protocol == "" {
		protocol = "TCP"
	}

	if protocol != "TCP" && protocol != "TCPS" {
		return "", fmt.Errorf(
			"invalid static connect identifier protocol for member %q: %q",
			member.Name,
			member.Endpoint.Protocol,
		)
	}

	if member.Endpoint.Port <= 0 {
		return "", fmt.Errorf(
			"invalid static connect identifier port for member %q: %d",
			member.Name,
			member.Endpoint.Port,
		)
	}

	descriptor := fmt.Sprintf(
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=%s)(HOST=%s)(PORT=%d))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=%s)))",
		protocol,
		host,
		member.Endpoint.Port,
		serviceName,
	)

	if protocol == "TCPS" {
		security := "(SECURITY=(SSL_SERVER_DN_MATCH=NO"

		if sslServerDN := strings.TrimSpace(member.SSLServerDN); sslServerDN != "" {
			if err := dbapi.ValidateDataguardSingleLineText("sslServerDN", sslServerDN); err != nil {
				return "", fmt.Errorf(
					"invalid static connect identifier sslServerDN for member %q: %w",
					member.Name,
					err,
				)
			}
			security += fmt.Sprintf(")(SSL_SERVER_CERT_DN=%s", sslServerDN)
		}

		if walletDirectory := strings.TrimSpace(member.WalletDirectory); walletDirectory != "" {
			if err := dbapi.ValidateDataguardSingleLineText("walletDirectory", walletDirectory); err != nil {
				return "", fmt.Errorf(
					"invalid static connect identifier walletDirectory for member %q: %w",
					member.Name,
					err,
				)
			}
			security += fmt.Sprintf(")(MY_WALLET_DIRECTORY=%s", walletDirectory)
		}

		security += "))"
		descriptor = strings.TrimSuffix(descriptor, ")") + security + ")"
	}

	return descriptor, nil
}

func buildDataguardStaticConnectIdentifier(member *dataguardTopologyResolvedMember) (string, error) {
	descriptor, err := buildDataguardStaticConnectDescriptor(member)
	if err != nil {
		return "", err
	}
	if descriptor == "" {
		return "", nil
	}

	return dataguardDGMGRLStringLiteral(descriptor)
}

func dataguardDGMGRLConnectIdentifier(member *dataguardTopologyResolvedMember) (string, error) {
	if member == nil {
		return "", fmt.Errorf("member is required")
	}
	if strings.EqualFold(strings.TrimSpace(member.Endpoint.Protocol), "TCPS") && strings.TrimSpace(member.Alias) != "" {
		alias, err := dataguardDGMGRLIdentifier(member.Alias)
		if err != nil {
			return "", err
		}
		return dataguardDGMGRLStringLiteral(alias)
	}
	if strings.TrimSpace(member.ConnectString) != "" {
		return dataguardDGMGRLStringLiteral(member.ConnectString)
	}
	alias, err := dataguardDGMGRLIdentifier(member.Alias)
	if err != nil {
		return "", err
	}
	return dataguardDGMGRLStringLiteral(alias)
}

func dataguardDGMGRLStringLiteral(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("connect identifier is required")
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f || r == '\'' {
			return "", fmt.Errorf("%q contains unsafe DGMGRL string literal character %q", value, r)
		}
	}
	return "'" + trimmed + "'", nil
}

func dataguardDGMGRLIdentifier(value string) (string, error) {
	identifier := strings.ToUpper(strings.TrimSpace(value))
	if identifier == "" {
		return "", fmt.Errorf("identifier is required")
	}
	for i, r := range identifier {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '_' || r == '$' || r == '#') {
			continue
		}
		return "", fmt.Errorf("%q contains unsafe DGMGRL identifier character %q", value, r)
	}
	return identifier, nil
}

func buildDataguardTopologyRefreshConnectIdentifiersScript(state *dataguardTopologyResolvedState, currentMembers map[string]string) (string, error) {
	if state == nil || len(currentMembers) == 0 {
		return "", nil
	}

	keys := make([]string, 0, len(currentMembers))
	for dbUniqueName := range currentMembers {
		keys = append(keys, strings.ToUpper(strings.TrimSpace(dbUniqueName)))
	}
	sort.Strings(keys)

	var lines []string
	for _, dbUniqueName := range keys {
		member := state.MembersByDBUniqueName[dbUniqueName]
		if member == nil {
			continue
		}
		memberDBUniqueName, err := dataguardDGMGRLIdentifier(member.DBUniqueName)
		if err != nil {
			return "", fmt.Errorf("invalid database identifier: %w", err)
		}
		if strings.TrimSpace(member.ConnectString) != "" || strings.TrimSpace(member.Alias) != "" {
			connectID, err := dataguardDGMGRLConnectIdentifier(member)
			if err != nil {
				return "", fmt.Errorf("invalid connect identifier: %w", err)
			}
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY DGConnectIdentifier=%s;", memberDBUniqueName, connectID))
		}
		staticID, err := buildDataguardStaticConnectIdentifier(member)
		if err != nil {
			return "", err
		}
		if staticID != "" {
			lines = append(lines, fmt.Sprintf("EDIT DATABASE %s SET PROPERTY STATICCONNECTIDENTIFIER=%s;", memberDBUniqueName, staticID))
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func reconcileDataguardTopologyConnectIdentifiers(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, currentPrimary *dataguardTopologyResolvedMember, state *dataguardTopologyResolvedState, currentMembers map[string]string) error {
	script, err := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers)
	if err != nil {
		return err
	}
	if strings.TrimSpace(script) == "" {
		return nil
	}
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, script)
	return err
}

func configureDataguardTopologyFSFO(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, state *dataguardTopologyResolvedState) error {
	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
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
	out, err := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, buildDataguardTopologyDisableFSFOScript(broker.Name))
	if err != nil && strings.Contains(out+err.Error(), "ORA-16873") {
		return nil
	}
	return err
}

func buildDataguardTopologyDisableFSFOScript(brokerName string) string {
	observerName := strings.TrimSpace(brokerName) + "-observer"
	return fmt.Sprintf(dbcommons.DisableFSFOCMD, observerName) + "\n"
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

	currentMembers, connectedMember, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	if len(currentMembers) == 0 {
		broker.Status.Status = dbcommons.StatusNotReady
		return nil
	}
	currentPrimaryMember := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimaryMember == nil {
		currentPrimaryMember = connectedMember
	}
	if currentPrimaryMember == nil {
		currentPrimaryMember = state.Primary
	}
	observed, err := queryDataguardTopologyObservedConfiguration(ctx, r, broker, req, currentPrimaryMember)
	if err != nil {
		return err
	}

	databasesInConfig := map[string]string{}
	var standbys []string
	currentPrimary := ""
	localMemberMessages := map[string]string{}
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
			if err := reconcileLocalSIDBDataguardMemberStatus(
				ctx,
				r,
				broker,
				req,
				member,
				role,
			); err != nil {
				return err
			}
			ready, message, err := dataguardTopologyLocalMemberReady(ctx, r, broker, member)
			if err != nil {
				return err
			}
			if !ready {
				localMemberMessages[strings.ToLower(strings.TrimSpace(member.Name))] = message
			}
		}
	}
	sort.Strings(standbys)

	broker.Status.DatabasesInDataguardConfig = databasesInConfig
	broker.Status.PrimaryDatabase = currentPrimary
	broker.Status.PrimaryDatabaseRef = databasesInConfig[currentPrimary]
	broker.Status.StandbyDatabases = strings.Join(standbys, ",")
	broker.Status.ProtectionMode = observedDataguardTopologyProtectionMode(observed, desired)
	broker.Status.Status = dbcommons.StatusReady

	currentPrimaryMember = state.MembersByDBUniqueName[currentPrimary]
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
				if message := strings.TrimSpace(localMemberMessages[strings.ToLower(strings.TrimSpace(member.Name))]); message != "" {
					memberStatus.Phase = "Pending"
					memberStatus.Message = message
				}
			}
		}
	}

	if missing := dataguardTopologyMissingStandbys(state, currentMembers); len(missing) > 0 {
		broker.Status.Status = dbcommons.StatusNotReady
		return nil
	}
	if len(localMemberMessages) > 0 {
		broker.Status.Status = dbcommons.StatusNotReady
		return nil
	}
	if observed != nil && !dataguardTopologyConfigurationStatusReady(observed.ConfigurationStatus) {
		broker.Status.Status = dbcommons.StatusNotReady
	}

	return nil
}

func observedDataguardTopologyProtectionMode(observed *dataguardTopologyObservedConfiguration, desired *dataguardBrokerDesiredSpec) string {
	if observed != nil && strings.TrimSpace(observed.ProtectionMode) != "" {
		return strings.TrimSpace(observed.ProtectionMode)
	}
	if desired != nil && strings.TrimSpace(desired.ProtectionMode) != "" {
		return strings.TrimSpace(desired.ProtectionMode)
	}
	return "MaxPerformance"
}

func dataguardTopologyConfigurationStatusReady(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	return normalized == "" || strings.HasPrefix(normalized, "SUCCESS")
}

func dataguardTopologyMissingStandbys(state *dataguardTopologyResolvedState, currentMembers map[string]string) []string {
	if state == nil || len(state.DesiredStandbys) == 0 {
		return nil
	}
	missing := make([]string, 0, len(state.DesiredStandbys))
	for _, member := range state.DesiredStandbys {
		if member == nil {
			continue
		}
		dbUniqueName := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
		if dbUniqueName == "" {
			continue
		}
		if _, ok := currentMembers[dbUniqueName]; ok {
			continue
		}
		missing = append(missing, dbUniqueName)
	}
	sort.Strings(missing)
	return missing
}

func reconcileLocalSIDBDataguardMemberStatus(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	member *dataguardTopologyResolvedMember,
	role string,
) error {
	if member == nil || member.LocalRef == nil {
		return nil
	}

	if kind := strings.TrimSpace(member.LocalRef.Kind); kind != "" &&
		!strings.EqualFold(kind, "SingleInstanceDatabase") {
		return nil
	}

	namespace := strings.TrimSpace(member.LocalRef.Namespace)
	if namespace == "" {
		namespace = broker.Namespace
	}

	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(
		ctx,
		types.NamespacedName{
			Namespace: namespace,
			Name:      strings.TrimSpace(member.LocalRef.Name),
		},
		&sidb,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	previousRole := normalizeTopologyMemberRole(sidb.Status.Role)
	observedRole := normalizeTopologyMemberRole(role)

	// DGMGRL standby role conversion can leave user PDBs mounted.
	// Recover the PDB before publishing the newly observed role.
	standbyRoleConversion :=
		(previousRole == "PHYSICAL_STANDBY" &&
			observedRole == "SNAPSHOT_STANDBY") ||
			(previousRole == "SNAPSHOT_STANDBY" &&
				observedRole == "PHYSICAL_STANDBY")

	if standbyRoleConversion {
		if err := openDataguardTopologyPDBs(
			ctx,
			r,
			broker,
			req,
			member,
		); err != nil {
			return fmt.Errorf(
				"failed to recover PDBs for local SIDB %q during role transition %s -> %s: %w",
				sidb.Name,
				previousRole,
				observedRole,
				err,
			)
		}
	}

	updated := false

	if normalizeTopologyMemberRole(sidb.Status.Role) != observedRole {
		sidb.Status.Role = observedRole
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

func dataguardTopologyLocalMemberReady(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, member *dataguardTopologyResolvedMember) (bool, string, error) {
	if member == nil || member.LocalRef == nil {
		return true, "", nil
	}
	if kind := strings.TrimSpace(member.LocalRef.Kind); kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") {
		return true, "", nil
	}

	namespace := strings.TrimSpace(member.LocalRef.Namespace)
	if namespace == "" && broker != nil {
		namespace = broker.Namespace
	}
	name := strings.TrimSpace(member.LocalRef.Name)
	if name == "" {
		return false, "local SingleInstanceDatabase reference is empty", nil
	}
	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("local SingleInstanceDatabase %s/%s not found", namespace, name), nil
		}
		return false, "", err
	}
	if sidb.Status.Status != dbcommons.StatusReady {
		status := strings.TrimSpace(sidb.Status.Status)
		if status == "" {
			status = "unknown"
		}
		return false, fmt.Sprintf("local SingleInstanceDatabase %s/%s is %s; waiting for database recovery", namespace, name, status), nil
	}
	return true, "", nil
}

func cleanupDataguardTopologyBroker(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet())
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

func performDataguardTopologyProtectionModeChange(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, mode string) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet())
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}
	modeToken := dataguardTopologyProtectionModeDGMGRL(mode)
	if modeToken == "" {
		return fmt.Errorf("unsupported protection mode %q", mode)
	}
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, fmt.Sprintf("EDIT CONFIGURATION SET PROTECTION MODE AS %s;\n", modeToken))
	return err
}

func performDataguardTopologyRoleConversion(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, target, role string) error {
	desiredRole := normalizeTopologyMemberRole(role)
	if desiredRole != "PHYSICAL_STANDBY" && desiredRole != "SNAPSHOT_STANDBY" {
		return fmt.Errorf("unsupported role conversion target role %q", role)
	}
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet())
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	targetMember := state.MembersByDBUniqueName[strings.ToUpper(strings.TrimSpace(target))]
	if targetMember == nil {
		targetMember = state.MembersByName[strings.ToLower(strings.TrimSpace(target))]
	}
	if targetMember == nil || targetMember.Role == "PRIMARY" {
		return fmt.Errorf("role conversion target %q is not a standby topology member", target)
	}
	currentMembers, currentPrimary, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	if currentPrimary == nil {
		currentPrimary = resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	}
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}
	currentRole := currentMembers[strings.ToUpper(targetMember.DBUniqueName)]

	if currentRole == desiredRole {
		return reconcileLocalSIDBDataguardMemberStatus(
			ctx,
			r,
			broker,
			req,
			targetMember,
			currentRole,
		)
	}

	if currentRole != "SNAPSHOT_STANDBY" && currentRole != "PHYSICAL_STANDBY" {
		return fmt.Errorf(
			"target %q has unexpected live broker role %q",
			targetMember.DBUniqueName,
			currentRole,
		)
	}

	command := fmt.Sprintf(
		"CONVERT DATABASE %s TO %s;\n",
		targetMember.DBUniqueName,
		strings.ReplaceAll(desiredRole, "_", " "),
	)

	if _, err := runDataguardBrokerRunnerDGMGRLScript(
		ctx, r, broker, req, currentPrimary, command,
	); err != nil {
		return err
	}

	updatedMembers, _, err := queryDataguardTopologyConfigurationMembers(
		ctx, r, broker, req, state,
	)
	if err != nil {
		return fmt.Errorf(
			"%w: unable to verify target role: %v",
			errDataguardTopologyRoleConversionPending,
			err,
		)
	}

	updatedRole := updatedMembers[strings.ToUpper(targetMember.DBUniqueName)]
	if updatedRole != desiredRole {
		return fmt.Errorf(
			"%w: target %q is still %q",
			errDataguardTopologyRoleConversionPending,
			targetMember.DBUniqueName,
			updatedRole,
		)
	}

	return reconcileLocalSIDBDataguardMemberStatus(
		ctx,
		r,
		broker,
		req,
		targetMember,
		updatedRole,
	)
}

func dataguardTopologyProtectionModeDGMGRL(mode string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(mode), "MaxAvailability"):
		return "MAXAVAILABILITY"
	case strings.EqualFold(strings.TrimSpace(mode), "MaxPerformance"):
		return "MAXPERFORMANCE"
	default:
		return ""
	}
}

func performDataguardTopologyManualSwitchover(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, desired *dataguardBrokerDesiredSpec, req ctrl.Request, targetDBUniqueName string) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet())
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	targetDBUniqueName = strings.ToUpper(strings.TrimSpace(targetDBUniqueName))
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}
	if currentMembers[targetDBUniqueName] != "PRIMARY" {
		_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, fmt.Sprintf("SWITCHOVER TO %s;\n", targetDBUniqueName))
		if err != nil {
			return err
		}
	}

	currentMembers, connectedMember, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return fmt.Errorf("%w: unable to query broker after switchover: %v", errDataguardTopologySwitchoverPending, err)
	}
	if currentMembers[targetDBUniqueName] != "PRIMARY" {
		return fmt.Errorf("%w: target database %s is not PRIMARY yet", errDataguardTopologySwitchoverPending, targetDBUniqueName)
	}

	formerPrimary := state.Primary
	if formerPrimary != nil && !strings.EqualFold(formerPrimary.DBUniqueName, targetDBUniqueName) {
		if currentMembers[strings.ToUpper(formerPrimary.DBUniqueName)] != "PHYSICAL_STANDBY" {
			return fmt.Errorf("%w: former primary %s has not become PHYSICAL_STANDBY yet", errDataguardTopologySwitchoverPending, formerPrimary.DBUniqueName)
		}
		if err := restartDataguardTopologyStandbyApply(ctx, r, broker, req, state, formerPrimary); err != nil {
			return fmt.Errorf("%w: unable to start managed recovery on former primary %s: %v", errDataguardTopologySwitchoverPending, formerPrimary.DBUniqueName, err)
		}
		ready, message, err := dataguardTopologyLocalMemberReady(ctx, r, broker, formerPrimary)
		if err != nil {
			return err
		}
		if !ready {
			return fmt.Errorf("%w: %s", errDataguardTopologySwitchoverPending, message)
		}
	}
	postSwitchoverPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if postSwitchoverPrimary == nil {
		postSwitchoverPrimary = connectedMember
	}
	if postSwitchoverPrimary != nil {
		observed, observedErr := queryDataguardTopologyObservedConfiguration(ctx, r, broker, req, postSwitchoverPrimary)
		if observedErr != nil {
			return fmt.Errorf("%w: unable to verify broker after switchover: %v", errDataguardTopologySwitchoverPending, observedErr)
		}
		if observed != nil && !dataguardTopologyConfigurationStatusReady(observed.ConfigurationStatus) {
			return fmt.Errorf("%w: broker configuration status is %q", errDataguardTopologySwitchoverPending, observed.ConfigurationStatus)
		}
	}
	_ = desired
	return nil
}

func performDataguardTopologyFailover(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, targetDBUniqueName string, force bool) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet())
	if err != nil {
		return err
	}
	if err := ensureDataguardTopologyNetConfiguration(ctx, r, broker, state, req); err != nil {
		return err
	}
	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		return err
	}
	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}
	command := fmt.Sprintf("FAILOVER TO %s;\n", strings.ToUpper(strings.TrimSpace(targetDBUniqueName)))
	if force {
		command = fmt.Sprintf("FAILOVER TO %s IMMEDIATE;\n", strings.ToUpper(strings.TrimSpace(targetDBUniqueName)))
	}
	_, err = runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, currentPrimary, command)
	return err
}

func oracleConnectDescriptor(user, password, alias string, asSysdba bool, useAuthWallet bool) string {
	connect := ""
	if useAuthWallet {
		connect = fmt.Sprintf(`/@%s`, alias)
	} else {
		passwordLiteral := strings.ReplaceAll(password, `"`, `\"`)
		connect = fmt.Sprintf(`%s/"%s"@%s`, user, passwordLiteral, alias)
	}
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
	normalized := strings.ToUpper(strings.TrimSpace(role))
	normalized = strings.NewReplacer(" ", "_", "-", "_").Replace(normalized)
	switch normalized {
	case "PRIMARY":
		return "PRIMARY"
	case "PHYSICAL_STANDBY":
		return "PHYSICAL_STANDBY"
	case "SNAPSHOT_STANDBY":
		return "SNAPSHOT_STANDBY"
	default:
		return normalized
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
	state, err := resolveDataguardTopologyState(ctx, r, broker, runtime, true)
	if err != nil {
		return err
	}
	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
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
	containerCommand, err := buildDataguardObserverStartupCommand(state, currentPrimary, observerName)
	if err != nil {
		return err
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

func deleteDataguardTopologyObserverPod(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker) error {
	if r == nil || broker == nil {
		return nil
	}
	observerName := strings.TrimSpace(broker.Name) + "-observer"
	if observerName == "-observer" {
		return nil
	}
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: observerName, Namespace: broker.Namespace}, pod)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if pod.DeletionTimestamp != nil {
		return nil
	}
	return r.Delete(ctx, pod)
}

func buildDataguardObserverStartupCommand(state *dataguardTopologyResolvedState, currentPrimary *dataguardTopologyResolvedMember, observerName string) (string, error) {
	tnsContent := []string{}
	if state != nil {
		for _, member := range state.Members {
			aliases, err := buildDataguardTopologyTNSAliases(member)
			if err != nil {
				return "", err
			}
			tnsContent = append(tnsContent, aliases...)
		}
	}
	sort.Strings(tnsContent)
	sqlnetContent := buildDataguardTopologySQLNet(state)
	tnsAdminPath := ""
	if state != nil && state.Runtime != nil {
		tnsAdminPath = strings.TrimRight(state.Runtime.TNSAdminPath, "/")
	}
	adminPassword := ""
	primaryAlias := ""
	if currentPrimary != nil {
		adminPassword = currentPrimary.AdminPassword
		primaryAlias = strings.TrimSpace(currentPrimary.Alias)
	}
	if strings.TrimSpace(primaryAlias) == "" {
		return "", fmt.Errorf("primary alias is required to start topology FSFO observer")
	}

	if strings.TrimSpace(adminPassword) == "" {
		primaryName := ""
		if currentPrimary != nil {
			primaryName = currentPrimary.Name
		}
		return "", fmt.Errorf("admin password is required to start topology FSFO observer for primary %s", primaryName)
	}
	observerStopCommand := fmt.Sprintf("STOP OBSERVER %s;\n", strings.TrimSpace(observerName))
	observerCommand := fmt.Sprintf("START OBSERVER %s FILE IS /tmp/fsfo.dat LOGFILE IS /tmp/observer.log;\n", strings.TrimSpace(observerName))
	observerStopScriptPath := "/tmp/fsfo-observer-stop.cmd"
	observerScriptPath := "/tmp/fsfo-observer.cmd"
	observerStdoutLog := "/tmp/fsfo_observer_stdout.log"
	observerStopStdoutLog := "/tmp/fsfo_observer_stop_stdout.log"
	return fmt.Sprintf(`%s%s%s%s%s%s%s
observer_pid=''
cleanup_observer() {
  rm -f /tmp/admin.pwd %s %s
  if [ -n "${observer_pid}" ]; then
    kill "${observer_pid}" 2>/dev/null || true
  fi
}
trap 'cleanup_observer; exit 0' TERM INT
echo "Stopping any stale FSFO observer registration before startup"
dgmgrl -echo %s @%s < /tmp/admin.pwd > %s 2>&1 || true
tail -50 %s || true
nohup dgmgrl -echo %s @%s < /tmp/admin.pwd > %s 2>&1 &
observer_pid=$!
sleep 3
ps -ef | grep -i '[d]gmgrl' || true
tail -50 %s || true
if ! kill -0 "${observer_pid}" 2>/dev/null; then
  echo "FSFO observer process exited during startup"
  exit 1
fi
wait "${observer_pid}"
`,
		buildDataguardWriteFileCommand(tnsAdminPath+"/tnsnames.ora", strings.Join(tnsContent, "\n")+"\n"),
		buildDataguardWriteFileCommand(tnsAdminPath+"/sqlnet.ora", sqlnetContent),
		"umask 177\n",
		buildDataguardWriteFileCommand("/tmp/admin.pwd", adminPassword+"\n"),
		buildDataguardWriteFileCommand(observerStopScriptPath, observerStopCommand),
		buildDataguardWriteFileCommand(observerScriptPath, observerCommand),
		"umask 022\n",
		shellQuote(observerStopScriptPath),
		shellQuote(observerScriptPath),
		shellQuote("sys@"+primaryAlias),
		shellQuote(observerStopScriptPath),
		shellQuote(observerStopStdoutLog),
		shellQuote(observerStopStdoutLog),
		shellQuote("sys@"+primaryAlias),
		shellQuote(observerScriptPath),
		shellQuote(observerStdoutLog),
		shellQuote(observerStdoutLog)), nil
}
