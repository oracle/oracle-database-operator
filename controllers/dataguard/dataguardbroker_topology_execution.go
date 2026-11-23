package controllers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
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

const sidbConditionDataguardRoleMismatch = "DataguardRoleMismatch"

// Bound individual runner execs so a stuck attach/stream cannot block the
// DataguardBroker reconcile worker indefinitely (which freezes Ready at the
// last RunnerPending status update).
const dataguardBrokerRunnerExecTimeout = 10 * time.Minute

type dataguardTopologyResolvedMember struct {
	Name            string
	Role            string
	DBUniqueName    string
	Alias           string
	StaticAlias     string
	ResourceName    string
	LocalRef        *dbapi.DataguardLocalRef
	Endpoint        dbapi.DataguardEndpointSpec
	StaticEndpoint  dbapi.DataguardEndpointSpec
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
	staticEndpoint := endpoint
	if tcpEndpoint, tcpErr := selectDataguardEndpointForTransport(member, "TCP"); tcpErr == nil {
		staticEndpoint = tcpEndpoint
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

	port := endpoint.Port
	if port == 0 {
		port = dataguardTopologyCanonicalPort(protocol)
	}

	resolvedEndpoint := dbapi.DataguardEndpointSpec{
		Name:        strings.TrimSpace(endpoint.Name),
		Protocol:    protocol,
		Host:        host,
		Port:        port,
		ServiceName: serviceName,
		SSLServerDN: strings.TrimSpace(endpoint.SSLServerDN),
	}

	resolved := &dataguardTopologyResolvedMember{
		Name:         strings.TrimSpace(member.Name),
		Role:         role,
		DBUniqueName: dbUniqueName,
		Alias:        strings.ToUpper(strings.TrimSpace(alias)),
		StaticAlias:  strings.ToUpper(strings.TrimSpace(staticAlias)),
		LocalRef:     member.LocalRef,
		Endpoint:     resolvedEndpoint,
		StaticEndpoint: dbapi.DataguardEndpointSpec{
			Name:        strings.TrimSpace(staticEndpoint.Name),
			Protocol:    strings.ToUpper(strings.TrimSpace(staticEndpoint.Protocol)),
			Host:        strings.TrimSpace(staticEndpoint.Host),
			Port:        staticEndpoint.Port,
			ServiceName: strings.TrimSpace(staticEndpoint.ServiceName),
			SSLServerDN: strings.TrimSpace(staticEndpoint.SSLServerDN),
		},
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

		aliases := []string{
			member.Alias,
			member.StaticAlias,
			member.ConnectString,
		}
		// DGMGRL accepts Easy Connect identifiers without the leading "//".
		// Keep credentials for both forms because Oracle wallet lookup uses the
		// connect string literally, and topology endpoints may be normalized
		// before DGMGRL consumes them.
		if normalized := strings.TrimPrefix(strings.TrimSpace(member.ConnectString), "//"); normalized != "" && normalized != strings.TrimSpace(member.ConnectString) {
			aliases = append(aliases, normalized)
		}
		seenAliases := map[string]struct{}{}
		for _, alias := range aliases {
			if strings.TrimSpace(alias) == "" {
				continue
			}
			alias = strings.TrimSpace(alias)
			if _, seen := seenAliases[alias]; seen {
				continue
			}
			seenAliases[alias] = struct{}{}

			lines = append(lines, fmt.Sprintf(
				"mkstore -wrl \"$WALLET_DIR\" -createCredential %s %s %s < \"$WALLET_DIR/.wallet.passwd\" >/dev/null",
				shellQuote(alias),
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

		aliasSyncCommand, err := buildDataguardTopologyLocalTNSAliasSyncCommand(state)
		if err != nil {
			return fmt.Errorf("failed to build local Oracle Net alias synchronization for %s: %w", member.Name, err)
		}
		if _, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", aliasSyncCommand); err != nil {
			return fmt.Errorf("failed to synchronize local Oracle Net aliases for %s: %w", member.Name, err)
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
	normalAlias, err := buildDataguardTopologyTNSAliasEntry(dataguardTopologyMemberAlias(member), member, normalService)
	if err != nil {
		return nil, err
	}
	staticMember := *member
	staticMember.Endpoint = member.StaticEndpoint
	if strings.TrimSpace(staticMember.Endpoint.Host) == "" {
		staticMember.Endpoint = member.Endpoint
	}
	staticAlias, err := buildDataguardTopologyTNSAliasEntry(member.StaticAlias, &staticMember, staticService)
	if err != nil {
		return nil, err
	}
	return []string{normalAlias, staticAlias}, nil
}

func dataguardTopologyMemberAlias(member *dataguardTopologyResolvedMember) string {
	if member == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(firstNonEmptyString(member.Alias, member.DBUniqueName)))
}

func dataguardTopologyMemberStaticAlias(member *dataguardTopologyResolvedMember) string {
	if member == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(firstNonEmptyString(member.StaticAlias, member.DBUniqueName+"_DGMGRL")))
}

func buildDataguardTopologyLocalTNSAliasSyncCommand(state *dataguardTopologyResolvedState) (string, error) {
	if state == nil {
		return "", fmt.Errorf("topology state is incomplete")
	}
	lines := []string{
		"set -e",
		`tns_file="${TNS_ADMIN:-${ORACLE_HOME}/network/admin}/tnsnames.ora"`,
		`mkdir -p "$(dirname "$tns_file")"`,
		`tns_helper="${SCRIPT_BASE_DIR:-/opt/oracle/scripts/base}/manageTnsAliases.sh"`,
		`if [ ! -x "$tns_helper" ] && [ -x "${ORACLE_BASE}/manageTnsAliases.sh" ]; then tns_helper="${ORACLE_BASE}/manageTnsAliases.sh"; fi`,
		`if [ ! -x "$tns_helper" ]; then echo "manageTnsAliases.sh is required for Data Guard local alias synchronization" >&2; exit 1; fi`,
	}
	for _, member := range state.Members {
		if member == nil {
			continue
		}
		aliases := []struct {
			alias   string
			service string
		}{
			{alias: dataguardTopologyMemberAlias(member), service: member.Endpoint.ServiceName},
			{alias: dataguardTopologyMemberStaticAlias(member), service: strings.ToUpper(strings.TrimSpace(member.DBUniqueName)) + "_DGMGRL"},
		}
		for _, item := range aliases {
			alias := strings.ToUpper(strings.TrimSpace(item.alias))
			if alias == "" {
				continue
			}
			service := strings.ToUpper(strings.TrimSpace(item.service))
			endpoint := member.Endpoint
			if alias == strings.ToUpper(strings.TrimSpace(member.StaticAlias)) {
				endpoint = member.StaticEndpoint
				if strings.TrimSpace(endpoint.Host) == "" {
					endpoint = member.Endpoint
				}
			}
			protocol := strings.ToUpper(strings.TrimSpace(endpoint.Protocol))
			if protocol == "" {
				protocol = "TCP"
			}
			args := fmt.Sprintf(`--file "$tns_file" --alias %s --upsert --host %s --port %d --service %s --protocol %s --strict-dedupe`,
				shellQuote(alias), shellQuote(endpoint.Host), endpoint.Port, shellQuote(service), shellQuote(protocol))
			if protocol == "TCPS" && strings.TrimSpace(member.SSLServerDN) != "" {
				args += fmt.Sprintf(" --ssl-server-dn %s", shellQuote(member.SSLServerDN))
			}
			lines = append(lines, fmt.Sprintf(`"$tns_helper" %s`, args))
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
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
	if !dataguardBrokerRunnerPodReady(&pod) {
		return nil, fmt.Errorf("%w: pod %s/%s is not ready (phase %s)", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName, pod.Status.Phase)
	}
	return &pod, nil
}

func withDataguardBrokerRunnerExecTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > 0 && time.Until(deadline) <= dataguardBrokerRunnerExecTimeout {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, dataguardBrokerRunnerExecTimeout)
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
	execCtx, cancel := withDataguardBrokerRunnerExecTimeout(ctx)
	defer cancel()
	out, err := dbcommons.ExecCommand(r, r.Config, podName, broker.Namespace, dataguardBrokerRunnerContainerName, execCtx, req, nolog, "bash", "-c", command)
	if err != nil {
		if apierrors.IsNotFound(err) || strings.Contains(err.Error(), fmt.Sprintf("pods %q not found", podName)) {
			return out, fmt.Errorf("%w: pod %s/%s not found", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return out, fmt.Errorf("%w: exec into pod %s/%s timed out after %s", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName, dataguardBrokerRunnerExecTimeout)
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
	execCtx, cancel := withDataguardBrokerRunnerExecTimeout(ctx)
	defer cancel()
	out, err := dbcommons.ExecCommandWithInput(r, r.Config, podName, broker.Namespace, dataguardBrokerRunnerContainerName, execCtx, req, nolog, input, "bash", "-c", command)
	if err != nil {
		if apierrors.IsNotFound(err) || strings.Contains(err.Error(), fmt.Sprintf("pods %q not found", podName)) {
			return out, fmt.Errorf("%w: pod %s/%s not found", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return out, fmt.Errorf("%w: exec into pod %s/%s timed out after %s", errDataguardBrokerRunnerUnavailable, broker.Namespace, podName, dataguardBrokerRunnerExecTimeout)
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

func dataguardRoleConversionPDBSQL(role string) (string, error) {
	role = normalizeTopologyMemberRole(role)
	openMode, cdbMode := "READ WRITE", "READ WRITE"
	switch role {
	case "SNAPSHOT_STANDBY":
	case "PHYSICAL_STANDBY":
		openMode, cdbMode = "READ ONLY", "READ ONLY WITH APPLY"
	default:
		return "", fmt.Errorf("unsupported standby PDB recovery role %q", role)
	}
	// SQL*Plus cannot expand a substitution variable as the first command
	// token. Keep conditional DDL and verification inside one PL/SQL block.
	return fmt.Sprintf(`set define off
set serveroutput on size unlimited
set heading off
set feedback off
set pagesize 0
set linesize 300
set trimspool on
set verify off
set echo off
declare
  expected_role constant varchar2(30) := '%s';
  expected_pdb_mode constant varchar2(10) := '%s';
  expected_cdb_mode constant varchar2(30) := '%s';
  live_role varchar2(30);
  live_mode varchar2(30);
  is_cdb varchar2(3);
  bad_pdbs number;
  mrp_count number;
  procedure verify_role_and_mode is
  begin
    select replace(database_role, ' ', '_'), open_mode, cdb
      into live_role, live_mode, is_cdb from v$database;
    if live_role <> expected_role then
      raise_application_error(-20001, 'Standby role is ' || live_role || '; expected ' || expected_role);
    end if;
    if (is_cdb = 'YES' or expected_role = 'SNAPSHOT_STANDBY')
      and live_mode <> expected_cdb_mode then
      raise_application_error(-20002, 'Standby open mode is ' || live_mode || '; expected ' || expected_cdb_mode);
    end if;
  end;
begin
  verify_role_and_mode;
  if is_cdb = 'YES' then
    select count(*) into bad_pdbs from v$pdbs
      where con_id > 2 and open_mode <> expected_pdb_mode;
    if bad_pdbs > 0 then
      execute immediate 'alter pluggable database all open ' || expected_pdb_mode;
    end if;
  end if;
  verify_role_and_mode;
  select count(*) into bad_pdbs from v$pdbs
    where con_id > 2 and open_mode <> expected_pdb_mode;
  if bad_pdbs > 0 then
    raise_application_error(-20003, 'User PDBs are not all open ' || expected_pdb_mode);
  end if;
  select count(*) into mrp_count from v$managed_standby where process like 'MRP%%';
  if expected_role = 'PHYSICAL_STANDBY' and mrp_count = 0 then
    raise_application_error(-20004, 'Physical standby managed recovery is not running');
  end if;
  if expected_role = 'SNAPSHOT_STANDBY' and mrp_count <> 0 then
    raise_application_error(-20005, 'Snapshot standby still has a managed recovery process');
  end if;
  for user_pdb in (select name, open_mode from v$pdbs where con_id > 2 order by con_id) loop
    dbms_output.put_line('DG_PDB_STATE=' || user_pdb.name || '|' || user_pdb.open_mode);
  end loop;
  dbms_output.put_line('DG_ROLE_PDBS_READY=' || expected_role);
end;
/
exit
`, role, openMode, cdbMode), nil
}

func openDataguardTopologyPDBs(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	member *dataguardTopologyResolvedMember,
	expectedRole string,
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

	if sidb.DeletionTimestamp != nil {
		return fmt.Errorf("local SIDB %s/%s is terminating", namespace, sidb.Name)
	}
	// Discover real user PDBs through v$pdbs. Standby spec/status PDB names
	// can differ from the names cloned from the primary.
	pdbSQL, err := dataguardRoleConversionPDBSQL(expectedRole)
	if err != nil {
		return err
	}

	alias := dataguardTopologyMemberAlias(member)
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

	script := fmt.Sprintf("whenever oserror exit failure\nwhenever sqlerror exit failure\nconnect %s\n%s", connectArg, pdbSQL)

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

	if strings.Contains(strings.ToUpper(out), "ORA-") ||
		strings.Contains(strings.ToUpper(out), "SP2-") ||
		!strings.Contains(out, "DG_ROLE_PDBS_READY="+normalizeTopologyMemberRole(expectedRole)) {
		return fmt.Errorf(
			"failed to open PDBs for topology member %q: %s",
			member.DBUniqueName,
			strings.TrimSpace(out),
		)
	}

	return nil
}

func buildDataguardRunnerDGMGRLScript(connectMember *dataguardTopologyResolvedMember, script string) string {
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, dataguardTopologyMemberAlias(connectMember), false, connectMember.UseAuthWallet)
	return fmt.Sprintf("CONNECT %s;\n%s", connectArg, script)
}

func queryDataguardConfigurationMembers(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	connectMember *dataguardTopologyResolvedMember,
) (map[string]string, error) {
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

	out, err := execDataguardBrokerRunnerShell(
		ctx,
		r,
		broker,
		req,
		true,
		command,
	)

	if dataguardBrokerConfigurationMissing(out) ||
		(err != nil && dataguardBrokerConfigurationMissing(err.Error())) {
		return nil, fmt.Errorf(
			"%w: query through %s",
			errDataguardTopologyConfigurationMissing,
			connectMember.DBUniqueName,
		)
	}

	if err != nil {
		return nil, err
	}

	if outputErr := dataguardBrokerOutputError(out); outputErr != nil {
		return nil, fmt.Errorf(
			"query V$DG_BROKER_CONFIG through %s: %w",
			connectMember.DBUniqueName,
			outputErr,
		)
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

		dbUniqueName := strings.ToUpper(
			strings.TrimSpace(parts[0]),
		)
		if dbUniqueName == "" {
			continue
		}

		role := normalizeTopologyMemberRole(parts[1])

		switch role {
		case "PRIMARY", "PHYSICAL_STANDBY", "SNAPSHOT_STANDBY":
			members[dbUniqueName] = role
		}
	}

	return members, nil
}

func dataguardBrokerOutputError(out string) error {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if errorIndex := strings.Index(strings.ToUpper(trimmed), "ORA-"); errorIndex >= 0 {
			oracleError := strings.TrimSpace(trimmed[errorIndex:])
			upperOracleError := strings.ToUpper(oracleError)

			// DGMGRL reports apply-lag uncertainty as a transient WARNING while
			// the broker is being created. It must not abort reconciliation: the
			// next broker refresh can establish lag and reach SUCCESS.
			if strings.HasPrefix(upperOracleError, "ORA-16854:") {
				continue
			}

			// FSFO can be enabled before the observer is started. Allow
			// reconciliation to continue so that the operator can create
			// the observer pod and establish the observer connection.
			if strings.HasPrefix(upperOracleError, "ORA-16819:") &&
				strings.EqualFold(strings.TrimSpace(trimmed[:errorIndex]), "Warning:") {
				continue
			}

			return fmt.Errorf("data guard command returned %s", oracleError)
		}
	}
	return nil
}

func buildDataguardBrokerTNSNamesInspectionCommand() string {
	return `tns_file="${TNS_ADMIN:-${ORACLE_HOME}/network/admin}/tnsnames.ora"
printf 'TNS_ADMIN=%s\n' "${TNS_ADMIN:-}"
if [ -r "$tns_file" ]; then
  printf 'TNSNAMES.ORA=%s\n' "$tns_file"
  cat "$tns_file"
else
  printf 'TNSNAMES.ORA is not readable: %s\n' "$tns_file"
fi`
}

func dataguardTopologyDiagnosticMembers(state *dataguardTopologyResolvedState) []*dataguardTopologyResolvedMember {
	if state == nil {
		return nil
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
		if _, found := seen[key]; found {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, member)
	}

	addCandidate(state.Primary)
	for _, member := range state.Members {
		addCandidate(member)
	}
	return candidates
}

func truncateDataguardBrokerDiagnosticOutput(out string) string {
	const maxDiagnosticOutputLength = 8192

	trimmed := strings.TrimSpace(out)
	if len(trimmed) <= maxDiagnosticOutputLength {
		return trimmed
	}
	return trimmed[:maxDiagnosticOutputLength] + "\n... output truncated"
}

func logDataguardTopologyBrokerDiagnostics(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	state *dataguardTopologyResolvedState,
	reason string,
) {
	if r == nil || broker == nil || state == nil {
		return
	}

	tnsOutput, tnsErr := execDataguardBrokerRunnerShell(
		ctx,
		r,
		broker,
		req,
		true,
		buildDataguardBrokerTNSNamesInspectionCommand(),
	)
	if tnsErr != nil {
		r.Log.Error(tnsErr, "Unable to inspect Data Guard runner tnsnames.ora", "reason", reason)
	} else {
		r.Log.Info("Data Guard runner tnsnames.ora diagnostic", "reason", reason, "output", truncateDataguardBrokerDiagnosticOutput(tnsOutput))
	}

	for _, member := range dataguardTopologyDiagnosticMembers(state) {
		alias := dataguardTopologyMemberAlias(member)
		showOutput, showErr := runDataguardBrokerRunnerDGMGRLScript(
			ctx,
			r,
			broker,
			req,
			member,
			"SHOW CONFIGURATION VERBOSE;\n",
		)
		if showErr == nil {
			showErr = dataguardBrokerOutputError(showOutput)
		}
		if showErr != nil {
			r.Log.Error(showErr, "Data Guard Broker verbose configuration diagnostic failed", "reason", reason, "database", member.DBUniqueName, "alias", alias, "output", truncateDataguardBrokerDiagnosticOutput(showOutput))
		} else {
			r.Log.Info("Data Guard Broker verbose configuration diagnostic", "reason", reason, "database", member.DBUniqueName, "alias", alias, "output", truncateDataguardBrokerDiagnosticOutput(showOutput))
		}

		members, membersErr := queryDataguardConfigurationMembers(ctx, r, broker, req, member)
		if membersErr != nil {
			r.Log.Error(membersErr, "Data Guard Broker member query diagnostic failed", "reason", reason, "database", member.DBUniqueName, "alias", alias)
			continue
		}
		r.Log.Info("Data Guard Broker member query diagnostic", "reason", reason, "database", member.DBUniqueName, "alias", alias, "members", members)
	}
}

func dataguardBrokerConfigurationMissing(out string) bool {
	return strings.Contains(out, "ORA-16532") ||
		strings.Contains(out, "ORA-16596")
}

func runDataguardTopologyDGMGRLScript(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	state *dataguardTopologyResolvedState,
	script string,
) (string, *dataguardTopologyResolvedMember, error) {
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
		out, err := runDataguardBrokerRunnerDGMGRLScript(
			ctx,
			r,
			broker,
			req,
			candidate,
			script,
		)

		if err == nil {
			err = dataguardBrokerOutputError(out)
		}

		// DGMGRL can return process success even when CONNECT failed.
		// Treat only a connection-level failure, before any configuration
		// output was produced, as a reason to try the next topology member.
		if err == nil {
			upperOut := strings.ToUpper(out)

			hasConfigurationOutput := strings.Contains(
				upperOut,
				"CONFIGURATION -",
			)

			connectionFailed :=
				strings.Contains(upperOut, "ORA-12541") ||
					strings.Contains(upperOut, "SP2-0640")

			if !hasConfigurationOutput && connectionFailed {
				err = fmt.Errorf(
					"unable to query broker through topology member %s",
					candidate.DBUniqueName,
				)
			}
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
// being unavailable or having a stale broker view while database roles change.
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
	var fallbackMembers map[string]string
	var fallbackMember *dataguardTopologyResolvedMember

	for _, candidate := range candidates {
		members, err := queryDataguardConfigurationMembers(ctx, r, broker, req, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		if len(members) == 0 {
			continue
		}

		// Preserve the first usable member view as a fallback so that a
		// degraded broker configuration still exposes the best available
		// topology instead of losing status completely.
		if fallbackMembers == nil {
			fallbackMembers = members
			fallbackMember = candidate
		}

		// A reachable former primary can retain a stale V$DG_BROKER_CONFIG
		// immediately after failover. Prefer the member whose broker
		// configuration reports a ready state.
		observed, err := queryDataguardTopologyObservedConfiguration(ctx, r, broker, req, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		if observed != nil && dataguardTopologyConfigurationStatusReady(observed.ConfigurationStatus) {
			return members, candidate, nil
		}
	}

	if fallbackMembers != nil {
		return fallbackMembers, fallbackMember, nil
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
	connectArg := oracleConnectDescriptor("sys", connectMember.AdminPassword, dataguardTopologyMemberAlias(connectMember), true, connectMember.UseAuthWallet)
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
		out, runErr := runDataguardBrokerRunnerDGMGRLScript(ctx, r, broker, req, state.Primary, script)
		if outputErr := dataguardBrokerOutputError(out); outputErr != nil {
			logDataguardTopologyBrokerDiagnostics(ctx, r, broker, req, state, "broker configuration create command reported an Oracle error")
			return fmt.Errorf("failed to create Data Guard Broker configuration: %w", outputErr)
		}
		if runErr != nil {
			logDataguardTopologyBrokerDiagnostics(ctx, r, broker, req, state, "broker configuration create command execution failed")
			return runErr
		}
	}

	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(ctx, r, broker, req, state)
	if err != nil {
		logDataguardTopologyBrokerDiagnostics(ctx, r, broker, req, state, "broker member query failed")
		return err
	}

	if len(currentMembers) == 0 {
		logDataguardTopologyBrokerDiagnostics(ctx, r, broker, req, state, "broker member query returned no members")
		return fmt.Errorf("%w after broker query returned no members", errDataguardTopologyConfigurationMissing)
	}

	previouslyObserved := dataguardBrokerConfigurationPreviouslyObserved(broker)
	localStable := dataguardTopologyBrokerMemberViewStable(broker, currentMembers)

	r.Log.Info(
		"FSFO topology resolution debug",
		"previouslyObserved", previouslyObserved,
		"localStable", localStable,
		"currentMembers", currentMembers,
	)

	if previouslyObserved && !localStable {
		brokerMembers := parseDataguardTopologyDGMGRLConfigurationMembers(showOut, state)
		brokerStable := dataguardTopologyBrokerMemberViewStable(broker, brokerMembers)

		r.Log.Info(
			"FSFO live Broker topology debug",
			"brokerMembers", brokerMembers,
			"brokerStable", brokerStable,
		)

		if brokerStable {
			r.Log.Info(
				"Using live Broker topology because local broker member view is incomplete during role transition",
				"localMembers", currentMembers,
				"brokerMembers", brokerMembers,
			)
			currentMembers = brokerMembers
		}
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

	// During a switchover, V$DG_BROKER_CONFIG can briefly expose only one
	// member (or expose a member without its final role).  Do not interpret
	// that transient view as a real membership change: the prerequisite helper
	// disables DMON and rewrites the Broker .dat files, which can destroy the
	// active configuration on the new primary.
	if dataguardBrokerConfigurationPreviouslyObserved(broker) &&
		!dataguardTopologyBrokerMemberViewStable(broker, currentMembers) {
		return fmt.Errorf(
			"%w: broker member view is incomplete during role transition; refusing local prerequisite reconfiguration",
			errDataguardTopologySwitchoverPending,
		)
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

func parseDataguardTopologyDGMGRLConfigurationMembers(
	out string,
	state *dataguardTopologyResolvedState,
) map[string]string {
	members := map[string]string{}
	if state == nil {
		return members
	}

	for _, member := range state.Members {
		if member == nil {
			continue
		}

		dbUniqueName := strings.ToUpper(strings.TrimSpace(member.DBUniqueName))
		if dbUniqueName == "" {
			continue
		}

		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			upper := strings.ToUpper(trimmed)

			if !strings.HasPrefix(upper, dbUniqueName+" ") &&
				!strings.HasPrefix(upper, dbUniqueName+" -") {
				continue
			}

			switch {
			case strings.Contains(upper, "SNAPSHOT STANDBY DATABASE"):
				members[dbUniqueName] = "SNAPSHOT_STANDBY"
			case strings.Contains(upper, "PHYSICAL STANDBY DATABASE"):
				members[dbUniqueName] = "PHYSICAL_STANDBY"
			case strings.Contains(upper, "PRIMARY DATABASE"):
				members[dbUniqueName] = "PRIMARY"
			}

			break
		}
	}

	return members
}

func dataguardBrokerConfigurationPreviouslyObserved(broker *dbapi.DataguardBroker) bool {
	if broker == nil {
		return false
	}
	return strings.TrimSpace(broker.Status.PrimaryDatabase) != "" ||
		strings.TrimSpace(broker.Status.PrimaryDatabaseRef) != "" ||
		len(broker.Status.DatabasesInDataguardConfig) > 0
}

func dataguardTopologyBrokerMemberViewStable(broker *dbapi.DataguardBroker, currentMembers map[string]string) bool {
	if broker == nil || len(currentMembers) == 0 {
		return false
	}

	knownMembers := make(map[string]struct{}, len(broker.Status.DatabasesInDataguardConfig)+1)
	for dbUniqueName := range broker.Status.DatabasesInDataguardConfig {
		if name := strings.ToUpper(strings.TrimSpace(dbUniqueName)); name != "" {
			knownMembers[name] = struct{}{}
		}
	}
	if primary := strings.ToUpper(strings.TrimSpace(broker.Status.PrimaryDatabase)); primary != "" {
		knownMembers[primary] = struct{}{}
	}

	for dbUniqueName := range knownMembers {
		if _, ok := currentMembers[dbUniqueName]; !ok {
			return false
		}
	}

	primaryCount := 0
	for _, role := range currentMembers {
		if strings.EqualFold(strings.TrimSpace(role), "PRIMARY") {
			primaryCount++
		}
	}
	return primaryCount == 1
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
	staticEndpoint := member.StaticEndpoint
	if strings.TrimSpace(staticEndpoint.Host) == "" {
		staticEndpoint = member.Endpoint
	}

	host, err := dataguardTNSHost(staticEndpoint.Host)
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

	protocol := strings.ToUpper(strings.TrimSpace(staticEndpoint.Protocol))
	if protocol == "" {
		protocol = "TCP"
	}

	if protocol != "TCP" && protocol != "TCPS" {
		return "", fmt.Errorf(
			"invalid static connect identifier protocol for member %q: %q",
			member.Name,
			staticEndpoint.Protocol,
		)
	}

	if staticEndpoint.Port <= 0 {
		return "", fmt.Errorf(
			"invalid static connect identifier port for member %q: %d",
			member.Name,
			staticEndpoint.Port,
		)
	}

	descriptor := fmt.Sprintf(
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=%s)(HOST=%s)(PORT=%d))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=%s)))",
		protocol,
		host,
		staticEndpoint.Port,
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
	// TCPS cannot use the plain host:port/service Easy Connect form here:
	// DGMGRL would interpret it as TCP and attempt to connect to the TCPS
	// listener without the member wallet.  The runner-local TNS alias carries
	// the TCPS protocol and MY_WALLET_DIRECTORY, so use it for TCPS members.
	if strings.EqualFold(strings.TrimSpace(member.Endpoint.Protocol), "TCPS") {
		alias, err := dataguardDGMGRLIdentifier(dataguardTopologyMemberAlias(member))
		if err != nil {
			return "", err
		}
		return dataguardDGMGRLStringLiteral(alias)
	}
	// Prefer the resolved endpoint. This is the stable cross-cluster path and
	// does not depend on DGMGRL resolving a runner-local TNS alias.
	if strings.TrimSpace(member.ConnectString) != "" {
		// DGMGRL's Easy Connect parser expects host:port/service here.  A
		// leading "//" is valid for other Oracle clients but is parsed by
		// DGMGRL as an empty hostname, resulting in ORA-12262.
		connectString := strings.TrimSpace(member.ConnectString)
		connectString = strings.TrimPrefix(connectString, "//")
		return dataguardDGMGRLStringLiteral(connectString)
	}
	alias, err := dataguardDGMGRLIdentifier(firstNonEmptyString(member.Alias, member.DBUniqueName))
	if err != nil {
		return "", err
	}
	return dataguardDGMGRLStringLiteral(alias)
}

func dataguardDGMGRLStringLiteral(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	// DGMGRL treats a leading "//" as an empty hostname in Easy Connect
	// identifiers. Normalize at the final literal boundary so every caller,
	// including add-database and refresh paths, gets the DGMGRL-safe form.
	trimmed = strings.TrimPrefix(trimmed, "//")
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

func configureDataguardTopologyFSFO(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, _ *dataguardBrokerDesiredSpec, req ctrl.Request, state *dataguardTopologyResolvedState) error {
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
				true,
			); err != nil {
				r.Log.Info(
					"Unable to reconcile local SIDB Data Guard status; continuing with broker topology",
					"member", member.Name,
					"dbUniqueName", member.DBUniqueName,
					"error", err.Error(),
				)
				localMemberMessages[strings.ToLower(strings.TrimSpace(member.Name))] = err.Error()
			} else {
				ready, message, err := dataguardTopologyLocalMemberReady(ctx, r, broker, member)
				if err != nil {
					return err
				}
				if !ready {
					localMemberMessages[strings.ToLower(strings.TrimSpace(member.Name))] = message
				}
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

const sidbConditionDataguardStandbyRoleSynced = "DataguardStandbyRoleSynced"

func dataguardStandbyRoleSyncReason(role string) string {
	switch normalizeTopologyMemberRole(role) {
	case "PHYSICAL_STANDBY":
		return "PhysicalStandby"
	case "SNAPSHOT_STANDBY":
		return "SnapshotStandby"
	default:
		return ""
	}
}

func dataguardStandbyRoleRecoveryNeeded(previousRole, observedRole string, lastSynced *metav1.Condition) bool {
	previousReason := dataguardStandbyRoleSyncReason(previousRole)

	if lastSynced != nil && lastSynced.Status == metav1.ConditionTrue {
		switch lastSynced.Reason {
		case "PhysicalStandby", "SnapshotStandby":
			previousReason = lastSynced.Reason
		}
	}

	observedReason := dataguardStandbyRoleSyncReason(observedRole)

	return previousReason != "" &&
		observedReason != "" &&
		previousReason != observedReason
}

func reconcileLocalSIDBDataguardMemberStatus(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	member *dataguardTopologyResolvedMember,
	role string,
	recoverUnreadyStandby bool,
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

	observedRole := normalizeTopologyMemberRole(role)
	recoveredStandbyPDBs := false
	lastSynced := meta.FindStatusCondition(
		sidb.Status.Conditions,
		sidbConditionDataguardStandbyRoleSynced,
	)

	if recoverUnreadyStandby &&
		dataguardStandbyRoleRecoveryNeeded(sidb.Status.Role, observedRole, lastSynced) {
		if err := openDataguardTopologyPDBs(
			ctx,
			r,
			broker,
			req,
			member,
			observedRole,
		); err != nil {
			return fmt.Errorf(
				"failed to recover PDBs for local SIDB %q during standby role synchronization: %w",
				sidb.Name,
				err,
			)
		}
		recoveredStandbyPDBs = true
	}

	// Read the actual role from the local database. Broker membership describes
	// the broker topology, but during failover the former primary may still be
	// PRIMARY until it is reinstated.
	readyPod, _, _, _, err := dbcommons.FindPods(
		r,
		sidb.Spec.Image.Version,
		"",
		sidb.Name,
		sidb.Namespace,
		ctx,
		req,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to find ready pod for local SIDB %s/%s: %w",
			sidb.Namespace,
			sidb.Name,
			err,
		)
	}

	if strings.TrimSpace(readyPod.Name) == "" {
		return fmt.Errorf(
			"local SIDB %s/%s does not have a ready database pod",
			sidb.Namespace,
			sidb.Name,
		)
	}

	liveRole, err := dbcommons.GetDatabaseRole(
		readyPod,
		r,
		r.Config,
		ctx,
		req,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to read live database role for local SIDB %s/%s: %w",
			sidb.Namespace,
			sidb.Name,
			err,
		)
	}

	normalizedLiveRole := normalizeTopologyMemberRole(liveRole)
	previousRole := normalizeTopologyMemberRole(sidb.Status.Role)

	// DGMGRL standby role conversion can leave user PDBs mounted.
	// Recover the PDB before publishing the newly observed standby role.
	standbyRoleConversion :=
		(previousRole == "PHYSICAL_STANDBY" &&
			normalizedLiveRole == "SNAPSHOT_STANDBY") ||
			(previousRole == "SNAPSHOT_STANDBY" &&
				normalizedLiveRole == "PHYSICAL_STANDBY")

	if standbyRoleConversion && !recoveredStandbyPDBs {
		if err := openDataguardTopologyPDBs(
			ctx,
			r,
			broker,
			req,
			member,
			normalizedLiveRole,
		); err != nil {
			return fmt.Errorf(
				"failed to recover PDBs for local SIDB %q during role transition %s -> %s: %w",
				sidb.Name,
				previousRole,
				normalizedLiveRole,
				err,
			)
		}
	}

	updated := false

	// SIDB role must represent the actual database role, not the broker's
	// intended/member role.
	if normalizeTopologyMemberRole(sidb.Status.Role) != normalizedLiveRole {
		sidb.Status.Role = normalizedLiveRole
		updated = true
	}

	roleMismatch := observedRole != "" &&
		normalizedLiveRole != "" &&
		observedRole != normalizedLiveRole

	if roleMismatch {
		r.Log.Info(
			"Data Guard broker role has not converged with live SIDB role",
			"sidb", sidb.Name,
			"brokerRole", observedRole,
			"liveRole", normalizedLiveRole,
		)

		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               sidbConditionDataguardRoleMismatch,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "BrokerRoleMismatch",
			Message: fmt.Sprintf(
				"Data Guard broker reports role %s while live database reports %s",
				observedRole,
				normalizedLiveRole,
			),
		})

		if sidb.Status.Status == dbcommons.StatusReady {
			sidb.Status.Status = dbcommons.StatusNotReady
		}

		updated = true
	} else {
		mismatchCondition := meta.FindStatusCondition(
			sidb.Status.Conditions,
			sidbConditionDataguardRoleMismatch,
		)

		if mismatchCondition != nil {
			r.Log.Info(
				"Data Guard broker role has converged with live SIDB role",
				"sidb", sidb.Name,
				"role", normalizedLiveRole,
			)

			meta.RemoveStatusCondition(
				&sidb.Status.Conditions,
				sidbConditionDataguardRoleMismatch,
			)

			// Restore only the NotReady state that this broker-role mismatch
			// introduced. Do not overwrite Error/Updating/other states.
			if sidb.Status.Status == dbcommons.StatusNotReady {
				sidb.Status.Status = dbcommons.StatusReady
			}

			updated = true
		}
	}

	if sidb.Status.DgBroker == nil || *sidb.Status.DgBroker != broker.Name {
		sidb.Status.DgBroker = &broker.Name
		updated = true
	}

	// Preserve the last synchronized standby role independently of
	// role refreshes performed by the SIDB controller.
	if observedRole == normalizedLiveRole {
		if reason := dataguardStandbyRoleSyncReason(normalizedLiveRole); reason != "" {
			if meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
				Type:               sidbConditionDataguardStandbyRoleSynced,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: sidb.Generation,
				Reason:             reason,
				Message:            "Last synchronized standby role: " + normalizedLiveRole,
			}) {
				updated = true
			}
		} else if normalizedLiveRole == "PRIMARY" {
			if meta.FindStatusCondition(sidb.Status.Conditions, sidbConditionDataguardStandbyRoleSynced) != nil {
				meta.RemoveStatusCondition(&sidb.Status.Conditions, sidbConditionDataguardStandbyRoleSynced)
				updated = true
			}
		}
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

func performDataguardTopologyProtectionModeChange(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	mode string,
) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}

	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}

	state, err := resolveDataguardTopologyState(
		ctx,
		r,
		broker,
		runtime,
		!runtime.usesAuthWallet(),
	)
	if err != nil {
		return err
	}

	if err := ensureDataguardTopologyNetConfiguration(
		ctx,
		r,
		broker,
		state,
		req,
	); err != nil {
		return err
	}

	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(
		ctx,
		r,
		broker,
		req,
		state,
	)
	if err != nil {
		return err
	}

	currentPrimary := resolveCurrentDataguardTopologyPrimary(
		state,
		currentMembers,
	)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	modeToken := dataguardTopologyProtectionModeDGMGRL(mode)
	if modeToken == "" {
		return fmt.Errorf("unsupported protection mode %q", mode)
	}

	logXptMode := "ASYNC"
	if modeToken == "MAXAVAILABILITY" ||
		modeToken == "MAXPROTECTION" {
		logXptMode = "SYNC"
	}

	// Keep command generation deterministic.
	dbUniqueNames := make([]string, 0, len(currentMembers))
	for dbUniqueName := range currentMembers {
		dbUniqueNames = append(
			dbUniqueNames,
			strings.ToUpper(strings.TrimSpace(dbUniqueName)),
		)
	}
	sort.Strings(dbUniqueNames)

	memberCommands := make([]string, 0, len(dbUniqueNames))

	for _, dbUniqueName := range dbUniqueNames {
		identifier, err := dataguardDGMGRLIdentifier(dbUniqueName)
		if err != nil {
			return fmt.Errorf(
				"invalid database identifier %q: %w",
				dbUniqueName,
				err,
			)
		}

		memberCommands = append(
			memberCommands,
			fmt.Sprintf(
				"EDIT DATABASE %s SET PROPERTY LogXptMode='%s';",
				identifier,
				logXptMode,
			),
		)
	}

	modeCommand := fmt.Sprintf(
		"EDIT CONFIGURATION SET PROTECTION MODE AS %s;",
		modeToken,
	)

	var commands []string

	if modeToken == "MAXAVAILABILITY" ||
		modeToken == "MAXPROTECTION" {
		// Synchronous protection modes:
		// configure SYNC first, then raise/change protection mode.
		commands = append(commands, memberCommands...)
		commands = append(commands, modeCommand)
	} else {
		// MaxPerformance:
		// lower protection mode before switching transport to ASYNC.
		commands = append(commands, modeCommand)
		commands = append(commands, memberCommands...)
	}

	out, err := runDataguardBrokerRunnerDGMGRLScript(
		ctx,
		r,
		broker,
		req,
		currentPrimary,
		strings.Join(commands, "\n")+"\n",
	)
	if err != nil {
		return err
	}

	if err := dataguardBrokerOutputError(out); err != nil {
		return err
	}

	observed, err := queryDataguardTopologyObservedConfiguration(
		ctx,
		r,
		broker,
		req,
		currentPrimary,
	)
	if err != nil {
		return fmt.Errorf(
			"unable to verify protection mode change: %w",
			err,
		)
	}

	observedMode := strings.ToUpper(
		strings.ReplaceAll(
			strings.TrimSpace(observed.ProtectionMode),
			" ",
			"",
		),
	)

	if observedMode != modeToken {
		return fmt.Errorf(
			"protection mode did not change to %s; current mode is %q",
			mode,
			observed.ProtectionMode,
		)
	}

	if !dataguardTopologyConfigurationStatusReady(
		observed.ConfigurationStatus,
	) {
		return fmt.Errorf(
			"protection mode changed to %s but broker configuration status is %q",
			mode,
			observed.ConfigurationStatus,
		)
	}

	return nil
}

func performDataguardTopologyRoleConversion(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, target, role string) error {
	const maxAttempts = 3

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := performDataguardTopologyRoleConversionOnce(ctx, r, broker, req, target, role)
		if err == nil || err.Error() != "unable to upgrade connection: error dialing backend: EOF" || attempt >= maxAttempts {
			return err
		}

		r.Log.Info("Retrying standby role conversion after transient exec connection failure",
			"target", target,
			"role", role,
			"attempt", attempt+1,
			"maxAttempts", maxAttempts,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
}

func performDataguardTopologyRoleConversionOnce(ctx context.Context, r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, target, role string) error {
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
	finishConversion := func() error {
		// Recover before the Ready-Pod-dependent status sync. A role refresh
		// may already have updated sidb.Status.Role, and mounted PDBs can
		// make the Pod non-Ready while its database container still runs.
		if err := openDataguardTopologyPDBs(ctx, r, broker, req, targetMember, desiredRole); err != nil {
			return fmt.Errorf("%w: standby PDB recovery: %v", errDataguardTopologyRoleConversionPending, err)
		}
		if err := reconcileLocalSIDBDataguardMemberStatus(ctx, r, broker, req, targetMember, desiredRole, false); err != nil {
			return fmt.Errorf("%w: standby readiness/status: %v", errDataguardTopologyRoleConversionPending, err)
		}
		return nil
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
	currentRole := normalizeTopologyMemberRole(
		currentMembers[strings.ToUpper(targetMember.DBUniqueName)],
	)

	if currentRole == desiredRole {
		return finishConversion()
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

	updatedRole := normalizeTopologyMemberRole(
		updatedMembers[strings.ToUpper(targetMember.DBUniqueName)],
	)

	if updatedRole != desiredRole {
		return fmt.Errorf(
			"%w: target %q is still %q",
			errDataguardTopologyRoleConversionPending,
			targetMember.DBUniqueName,
			updatedRole,
		)
	}

	return finishConversion()
}

func dataguardTopologyProtectionModeDGMGRL(mode string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(mode), "MaxProtection"):
		return "MAXPROTECTION"
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

func performDataguardTopologyFailover(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
	targetDBUniqueName string,
	force bool,
) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}

	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}

	state, err := resolveDataguardTopologyState(
		ctx,
		r,
		broker,
		runtime,
		!runtime.usesAuthWallet(),
	)
	if err != nil {
		return err
	}

	if err := ensureDataguardTopologyNetConfiguration(
		ctx,
		r,
		broker,
		state,
		req,
	); err != nil {
		return err
	}

	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(
		ctx,
		r,
		broker,
		req,
		state,
	)
	if err != nil {
		return err
	}

	currentPrimary := resolveCurrentDataguardTopologyPrimary(
		state,
		currentMembers,
	)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	targetDBUniqueName = strings.ToUpper(
		strings.TrimSpace(targetDBUniqueName),
	)

	executionMember := currentPrimary

	command := fmt.Sprintf(
		"FAILOVER TO %s;\n",
		targetDBUniqueName,
	)

	if force {
		targetMember := state.MembersByDBUniqueName[targetDBUniqueName]
		if targetMember == nil {
			return fmt.Errorf(
				"failover target %s is not part of the resolved topology",
				targetDBUniqueName,
			)
		}

		targetRole := currentMembers[targetDBUniqueName]
		if targetRole != "PHYSICAL_STANDBY" {
			return fmt.Errorf(
				"force failover target %s is not a physical standby; current role is %q",
				targetDBUniqueName,
				targetRole,
			)
		}

		// An immediate failover must be issued while DGMGRL is
		// connected to the target standby database.
		executionMember = targetMember
		command = fmt.Sprintf(
			"FAILOVER TO %s IMMEDIATE;\n",
			targetDBUniqueName,
		)
	}

	out, err := runDataguardBrokerRunnerDGMGRLScript(
		ctx,
		r,
		broker,
		req,
		executionMember,
		command,
	)
	if err != nil {
		return err
	}

	// DGMGRL can return process success while reporting a broker/Oracle error
	// in stdout. Do not treat that as a successful failover.
	if err := dataguardBrokerOutputError(out); err != nil {
		return err
	}

	// Re-query the live Broker topology. A failover is successful only when
	// the requested target is actually observed as PRIMARY.
	updatedMembers, _, err := queryDataguardTopologyConfigurationMembers(
		ctx,
		r,
		broker,
		req,
		state,
	)
	if err != nil {
		return fmt.Errorf(
			"unable to verify broker after failover: %w",
			err,
		)
	}

	if updatedMembers[targetDBUniqueName] != "PRIMARY" {
		return fmt.Errorf(
			"failover target %s did not become PRIMARY; current role is %q",
			targetDBUniqueName,
			updatedMembers[targetDBUniqueName],
		)
	}

	return nil
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

func createDataguardTopologyObserverPod(
	ctx context.Context,
	r *DataguardBrokerReconciler,
	broker *dbapi.DataguardBroker,
	req ctrl.Request,
) error {
	runtime, ready, _, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("topology execution runtime is not ready")
	}

	state, err := resolveDataguardTopologyState(
		ctx,
		r,
		broker,
		runtime,
		true,
	)
	if err != nil {
		return err
	}

	currentMembers, _, err := queryDataguardTopologyConfigurationMembers(
		ctx,
		r,
		broker,
		req,
		state,
	)
	if err != nil {
		return err
	}

	currentPrimary := resolveCurrentDataguardTopologyPrimary(state, currentMembers)
	if currentPrimary == nil {
		currentPrimary = state.Primary
	}

	// Defaults used for manual/hybrid FSFO mode where topology.observer
	// may be omitted.
	observerName := broker.Name + "-observer"
	observerImage := runtime.Image
	observerNodeSelector := broker.Spec.NodeSelector

	// If an explicit observer configuration is present, honor its
	// overrides while retaining runtime defaults for omitted values.
	if broker.Spec.Topology != nil &&
		broker.Spec.Topology.Observer != nil &&
		broker.Spec.Topology.Observer.Enabled {

		observer := broker.Spec.Topology.Observer

		if strings.TrimSpace(observer.Name) != "" {
			observerName = strings.TrimSpace(observer.Name)
		}

		if strings.TrimSpace(observer.Image) != "" {
			observerImage = strings.TrimSpace(observer.Image)
		}

		if len(observer.NodeSelector) > 0 {
			observerNodeSelector = observer.NodeSelector
		}
	}

	// Check specifically for the observer pod. Other broker-related pods,
	// such as the execution runner, must not suppress observer creation.
	var observerPod corev1.Pod
	if err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      observerName,
			Namespace: broker.Namespace,
		},
		&observerPod,
	); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		return nil
	}

	containerCommand, err := buildDataguardObserverStartupCommand(
		state,
		currentPrimary,
		observerName,
	)
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

			secretName := dbapi.ResolveDataguardTopologyMemberClientWalletSecret(
				broker.Spec.Topology,
				&member,
			)
			if secretName == "" {
				continue
			}

			if _, ok := seenWallets[secretName]; ok {
				continue
			}
			seenWallets[secretName] = struct{}{}

			volumeName := "wallet-" +
				sanitizeDataguardRunnerName(secretName, "wallet")

			volumes = append(
				volumes,
				corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
			)

			volumeMounts = append(
				volumeMounts,
				corev1.VolumeMount{
					Name: volumeName,
					MountPath: strings.TrimRight(
						runtime.WalletMountPath,
						"/",
					) + "/" + sanitizeDataguardRunnerName(
						secretName,
						"wallet",
					),
					ReadOnly: true,
				},
			)
		}
	}

	imagePullSecrets := make(
		[]corev1.LocalObjectReference,
		0,
		len(runtime.ImagePullSecrets),
	)

	for _, secret := range runtime.ImagePullSecrets {
		if strings.TrimSpace(secret) == "" {
			continue
		}

		imagePullSecrets = append(
			imagePullSecrets,
			corev1.LocalObjectReference{
				Name: strings.TrimSpace(secret),
			},
		)
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
			NodeSelector:     cloneStringMap(observerNodeSelector),
			ImagePullSecrets: imagePullSecrets,
			RestartPolicy:    corev1.RestartPolicyAlways,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: func() *int64 {
					v := int64(54321)
					return &v
				}(),
				FSGroup: func() *int64 {
					v := int64(54321)
					return &v
				}(),
			},
			Volumes: volumes,
			Containers: []corev1.Container{{
				Name:            "observer",
				Image:           observerImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"bash",
					"-c",
					containerCommand,
				},
				Env: []corev1.EnvVar{{
					Name:  "TNS_ADMIN",
					Value: runtime.TNSAdminPath,
				}},
				VolumeMounts: volumeMounts,
			}},
		},
	}

	if err := ctrl.SetControllerReference(
		broker,
		&pod,
		r.Scheme,
	); err != nil {
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
