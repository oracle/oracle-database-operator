package commons

import (
	"strings"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRenderAgentFactoryRuntimeEnvUsesContainerContract(t *testing.T) {
	enableSwagger := true
	inst := &privateaiv4.AgentFactory{
		Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{
				Mode:                     "prod",
				SilentInstall:            boolPtr(true),
				UIAdminUsername:          "admin@example.com",
				BindHost:                 "0.0.0.0",
				CertFQDN:                 "agent.example.com",
				CertIPAddress:            "10.0.0.10",
				EnableSwagger:            &enableSwagger,
				StatePollIntervalSeconds: int32Ptr(7),
				APIReadyTimeoutSeconds:   int32Ptr(601),
				InstallTimeoutSeconds:    int32Ptr(1801),
				ReadyTimeoutSeconds:      int32Ptr(602),
				HTTPProxy:                "http://proxy.example.com:8080",
				HTTPSProxy:               "https://proxy.example.com:8443",
				NoProxy:                  "localhost,127.0.0.1",
			},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Details",
				Username:       "appuser",
				Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{
					Protocol: "TCPS", Host: "db.example.com", Port: 1522, ServiceName: "appsvc",
				},
			},
		},
	}

	runtimeEnv := RenderAgentFactoryRuntimeEnv(inst)
	for _, key := range []string{
		"AGENT_FACTORY_SILENT_INSTALL=true",
		"AGENT_FACTORY_ADMIN_USERNAME=admin@example.com",
		"AGENT_FACTORY_DB_CONNECTION_TYPE=Details",
		"AGENT_FACTORY_DB_USERNAME=appuser",
		"AGENT_FACTORY_DB_PROTOCOL=TCPS",
		"AGENT_FACTORY_DB_HOST=db.example.com",
		"AGENT_FACTORY_DB_PORT=1522",
		"AGENT_FACTORY_DB_SERVICE_NAME=appsvc",
		"AGENT_FACTORY_CERT_FQDN=agent.example.com",
		"AGENT_FACTORY_ENABLE_SWAGGER=true",
		"AGENT_FACTORY_INSTALL_TIMEOUT_SECONDS=1801",
		"AGENT_FACTORY_HTTP_PROXY=http://proxy.example.com:8080",
	} {
		if !strings.Contains(runtimeEnv, key) {
			t.Fatalf("expected runtime.env to contain %q, got:\n%s", key, runtimeEnv)
		}
	}
	if strings.Contains(runtimeEnv, "PAF_") {
		t.Fatalf("runtime.env must not contain legacy PAF_ keys:\n%s", runtimeEnv)
	}
}

func TestRenderAgentFactoryRuntimeEnvDatabaseModeMatrix(t *testing.T) {
	for _, airGapped := range []bool{true, false} {
		inst := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database:      &privateaiv4.AgentFactoryDatabaseSpec{ConnectionType: "Details", Username: "appuser", AirGapped: &airGapped, Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{Protocol: "TCP", Host: "db.example.com", Port: 1521, ServiceName: "appsvc"}},
		}}
		env := RenderAgentFactoryRuntimeEnv(inst)
		if !strings.Contains(env, "AGENT_FACTORY_DB_CONNECTION_TYPE=Details") || !strings.Contains(env, "AGENT_FACTORY_DB_USES_WALLET=no") {
			t.Fatalf("Details matrix row missing canonical mode fields:\n%s", env)
		}
		if strings.Contains(env, "WALLET_HAS_OCI") || strings.Contains(env, "SELECTED_TNS_ALIAS") {
			t.Fatalf("Details mode must omit Wallet fields:\n%s", env)
		}
	}
	for _, hasOCI := range []bool{true, false} {
		inst := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database:      &privateaiv4.AgentFactoryDatabaseSpec{ConnectionType: "Wallet", Username: "appuser", AirGapped: boolPtr(false), Wallet: &privateaiv4.AgentFactoryWalletSpec{SelectedTnsAlias: "APP", HasOciCertificates: &hasOCI}},
		}}
		env := RenderAgentFactoryRuntimeEnv(inst)
		if !strings.Contains(env, "AGENT_FACTORY_DB_CONNECTION_TYPE=Wallet") || !strings.Contains(env, "AGENT_FACTORY_DB_USES_WALLET=yes") || !strings.Contains(env, "AGENT_FACTORY_DB_SELECTED_TNS_ALIAS=APP") {
			t.Fatalf("Wallet matrix row missing canonical mode fields:\n%s", env)
		}
		wantOCI := "AGENT_FACTORY_DB_WALLET_HAS_OCI_CERTIFICATES=no"
		if hasOCI {
			wantOCI = "AGENT_FACTORY_DB_WALLET_HAS_OCI_CERTIFICATES=yes"
		}
		if !strings.Contains(env, wantOCI) || strings.Contains(env, "AGENT_FACTORY_DB_PROTOCOL=") {
			t.Fatalf("Wallet mode fields incorrect:\n%s", env)
		}
	}
}

func TestAgentFactoryWalletPasswordUsesSecretFileOnly(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-wallet", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Wallet",
				Username:       "paf_user",
				Wallet:         &privateaiv4.AgentFactoryWalletSpec{SelectedTnsAlias: "ORCLPDB1"},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{Secret: &privateaiv4.AgentFactorySecretSpec{
				Name: "runtime-secret", MountLocation: "/custom/secrets",
				UIAdminPasswordKey: "ui-password", DBPasswordKey: "db-password",
				WalletZipKey: "wallet.zip", WalletPasswordKey: "wallet-password-prod",
			}},
		},
	}

	env := RenderAgentFactoryRuntimeEnv(inst)
	if strings.Contains(env, "AGENT_FACTORY_DB_WALLET_PASSWORD=") {
		t.Fatalf("runtime.env must not contain the unsupported wallet password key, got:\n%s", env)
	}

	deployment := BuildAgentFactoryDeployment(inst)
	foundRuntimeSecretsDir := false
	for _, envVar := range deployment.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == "AGENT_FACTORY_RUNTIME_SECRETS_DIR" && envVar.Value == "/custom/secrets" {
			foundRuntimeSecretsDir = true
			break
		}
	}
	if !foundRuntimeSecretsDir {
		t.Fatal("expected AGENT_FACTORY_RUNTIME_SECRETS_DIR to point to the runtime Secret mount")
	}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "runtime-secret" {
			continue
		}
		for _, item := range volume.VolumeSource.Secret.Items {
			if item.Key == "wallet-password-prod" && item.Path == AgentFactoryWalletPasswordFilename {
				return
			}
		}
		t.Fatalf("wallet password key was not mapped to %q: %#v", AgentFactoryWalletPasswordFilename, volume.VolumeSource.Secret.Items)
	}
	t.Fatal("runtime-secret volume was not found")
}

func TestAgentFactoryOutboundCABundleIsMountedOutsideRuntimeSecret(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{
				LLM: &privateaiv4.AgentFactoryLLMSpec{
					Provider: "private_ai_services",
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{
				OutboundTLS: &privateaiv4.AgentFactoryOutboundTLSSpec{
					CABundle: &privateaiv4.AgentFactoryOutboundCABundleSpec{
						SecretName: "private-ai-ca",
						Key:        "private-ai-bundle.pem",
					},
				},
			},
		},
	}

	env := RenderAgentFactoryRuntimeEnv(inst)
	for _, value := range []string{
		"AGENT_FACTORY_OUTBOUND_CA_BUNDLE=/run/agentfactory/outbound-ca/private-ai-bundle.pem",
		"AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_CA_FILE=/run/agentfactory/outbound-ca/private-ai-bundle.pem",
	} {
		if !strings.Contains(env, value) {
			t.Fatalf("expected runtime.env to contain %q, got:\n%s", value, env)
		}
	}

	deployment := BuildAgentFactoryDeployment(inst)
	var foundVolume, foundMount bool
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "outbound-ca" {
			continue
		}
		foundVolume = true
		if volume.Secret == nil || volume.Secret.SecretName != "private-ai-ca" || len(volume.Secret.Items) != 1 ||
			volume.Secret.Items[0].Key != "private-ai-bundle.pem" || volume.Secret.Items[0].Path != "private-ai-bundle.pem" {
			t.Fatalf("unexpected outbound CA volume: %#v", volume)
		}
	}
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == "outbound-ca" {
			foundMount = true
			if mount.MountPath != AgentFactoryDefaultOutboundCAMount || !mount.ReadOnly {
				t.Fatalf("unexpected outbound CA mount: %#v", mount)
			}
		}
	}
	if !foundVolume || !foundMount {
		t.Fatalf("expected outbound CA Secret volume and mount, volume=%v mount=%v", foundVolume, foundMount)
	}
}

func boolPtr(value bool) *bool { return &value }

func int32Ptr(value int32) *int32 { return &value }

func TestBuildAgentFactoryDeploymentMapsLegacySecretKeysToContainerFilenames(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		Spec: privateaiv4.AgentFactorySpec{
			Database: &privateaiv4.AgentFactoryDatabaseSpec{ConnectionType: "Details"},
			Security: &privateaiv4.AgentFactorySecuritySpec{Secret: &privateaiv4.AgentFactorySecretSpec{
				Name: "runtime-secret", UIAdminPasswordKey: "paf_ui_admin_password", DBPasswordKey: "paf_db_password",
			}},
		},
	}
	deployment := BuildAgentFactoryDeployment(inst)
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "runtime-secret" {
			continue
		}
		items := volume.VolumeSource.Secret.Items
		if len(items) != 2 || items[0].Key != "paf_ui_admin_password" || items[0].Path != "agent_factory_admin_password" || items[1].Key != "paf_db_password" || items[1].Path != "agent_factory_db_password" {
			t.Fatalf("unexpected runtime Secret item mapping: %#v", items)
		}
		return
	}
	t.Fatal("runtime-secret volume was not found")
}

func TestBuildAgentFactoryDeploymentDefaultsServiceAccountTokenAutomountFalse(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "shns"},
	}

	deploy := BuildAgentFactoryDeployment(inst)
	got := deploy.Spec.Template.Spec.AutomountServiceAccountToken
	if got == nil {
		t.Fatalf("expected automountServiceAccountToken to be set")
	}
	if *got {
		t.Fatalf("expected automountServiceAccountToken=false by default")
	}
}

func TestBuildAgentFactoryDeploymentHonorsServiceAccountTokenAutomountTrue(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "shns"},
		Spec: privateaiv4.AgentFactorySpec{
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				AutomountServiceAccountToken: &enabled,
			},
		},
	}

	deploy := BuildAgentFactoryDeployment(inst)
	got := deploy.Spec.Template.Spec.AutomountServiceAccountToken
	if got == nil {
		t.Fatalf("expected automountServiceAccountToken to be set")
	}
	if !*got {
		t.Fatalf("expected automountServiceAccountToken=true when explicitly requested")
	}
}

func TestBuildAgentFactoryDeploymentUsesDefaultRolloutSettings(t *testing.T) {
	deployment := BuildAgentFactoryDeployment(&privateaiv4.AgentFactory{})
	strategy := deployment.Spec.Strategy
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType || strategy.RollingUpdate == nil {
		t.Fatalf("expected RollingUpdate strategy, got %#v", strategy)
	}
	if strategy.RollingUpdate.MaxSurge == nil || strategy.RollingUpdate.MaxSurge.Type != intstr.String || strategy.RollingUpdate.MaxSurge.StrVal != "25%" {
		t.Fatalf("expected default maxSurge 25%%, got %#v", strategy.RollingUpdate.MaxSurge)
	}
	if strategy.RollingUpdate.MaxUnavailable == nil || strategy.RollingUpdate.MaxUnavailable.Type != intstr.Int || strategy.RollingUpdate.MaxUnavailable.IntVal != 0 {
		t.Fatalf("expected default maxUnavailable 0, got %#v", strategy.RollingUpdate.MaxUnavailable)
	}
	gracePeriod := deployment.Spec.Template.Spec.TerminationGracePeriodSeconds
	if gracePeriod == nil || *gracePeriod != AgentFactoryDefaultTerminationGracePeriodSeconds {
		t.Fatalf("expected default terminationGracePeriodSeconds %d, got %#v", AgentFactoryDefaultTerminationGracePeriodSeconds, gracePeriod)
	}
}

func TestBuildAgentFactoryDeploymentHonorsRolloutSettings(t *testing.T) {
	maxSurge := intstr.FromInt(2)
	maxUnavailable := intstr.FromString("10%")
	gracePeriod := int64(120)
	inst := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
		Upgrade: &privateaiv4.AgentFactoryUpgradeSpec{
			Strategy: "RollingUpdate",
			RollingUpdate: &privateaiv4.AgentFactoryRollingUpdateSpec{
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavailable,
			},
			TerminationGracePeriodSeconds: &gracePeriod,
		},
	}}

	deployment := BuildAgentFactoryDeployment(inst)
	strategy := deployment.Spec.Strategy
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType || strategy.RollingUpdate == nil ||
		strategy.RollingUpdate.MaxSurge == nil || strategy.RollingUpdate.MaxSurge.IntVal != 2 ||
		strategy.RollingUpdate.MaxUnavailable == nil || strategy.RollingUpdate.MaxUnavailable.StrVal != "10%" {
		t.Fatalf("unexpected rollout strategy: %#v", strategy)
	}
	if got := deployment.Spec.Template.Spec.TerminationGracePeriodSeconds; got == nil || *got != gracePeriod {
		t.Fatalf("expected terminationGracePeriodSeconds %d, got %#v", gracePeriod, got)
	}
}

func TestBuildAgentFactoryDeploymentUsesAllWorkerNodesAndSpreadsReplicas(t *testing.T) {
	replicas := int32(4)
	workerNodes := []string{"10.0.10.121", "10.0.10.219", "10.0.10.60"}
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Replicas:    &replicas,
				WorkerNodes: workerNodes,
			},
		},
	}

	deployment := BuildAgentFactoryDeployment(inst)
	podSpec := deployment.Spec.Template.Spec
	if podSpec.NodeSelector != nil {
		t.Fatalf("expected workerNodes to be represented by node affinity, got nodeSelector=%#v", podSpec.NodeSelector)
	}
	if podSpec.Affinity == nil || podSpec.Affinity.NodeAffinity == nil ||
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("expected required node affinity for workerNodes")
	}

	required := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required.NodeSelectorTerms) != 1 || len(required.NodeSelectorTerms[0].MatchExpressions) != 1 {
		t.Fatalf("unexpected worker node affinity: %#v", required)
	}
	expression := required.NodeSelectorTerms[0].MatchExpressions[0]
	if expression.Key != corev1.LabelHostname || expression.Operator != corev1.NodeSelectorOpIn {
		t.Fatalf("unexpected worker node affinity expression: %#v", expression)
	}
	if len(expression.Values) != len(workerNodes) {
		t.Fatalf("expected all worker nodes in affinity, got %#v", expression.Values)
	}
	for index, workerNode := range workerNodes {
		if expression.Values[index] != workerNode {
			t.Fatalf("worker node %d: expected %q, got %q", index, workerNode, expression.Values[index])
		}
	}

	if len(podSpec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected one topology spread constraint, got %#v", podSpec.TopologySpreadConstraints)
	}
	spread := podSpec.TopologySpreadConstraints[0]
	if spread.MaxSkew != 1 || spread.TopologyKey != corev1.LabelHostname || spread.WhenUnsatisfiable != corev1.ScheduleAnyway {
		t.Fatalf("unexpected topology spread constraint: %#v", spread)
	}
	if spread.LabelSelector == nil {
		t.Fatal("expected topology spread constraint to select AgentFactory Pods")
	}
	for key, value := range AgentFactoryLabels(inst) {
		if spread.LabelSelector.MatchLabels[key] != value {
			t.Fatalf("topology spread selector missing %s=%s: %#v", key, value, spread.LabelSelector.MatchLabels)
		}
	}
}

func TestBuildAgentFactoryDeploymentUsesServiceAccountName(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
			ServiceAccountName: "agentfactory-sa",
		}},
	}

	deployment := BuildAgentFactoryDeployment(inst)
	if got := deployment.Spec.Template.Spec.ServiceAccountName; got != "agentfactory-sa" {
		t.Fatalf("expected agentfactory-sa, got %q", got)
	}
}

func TestBuildAgentFactoryDeploymentUsesWorkerNodeSelectorAndSpreadsReplicas(t *testing.T) {
	replicas := int32(3)
	workerNodeSelector := map[string]string{
		"agentfactory.oracle.com/pool": "paf-test04",
		"kubernetes.io/os":             "linux",
	}
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Replicas:           &replicas,
				WorkerNodeSelector: workerNodeSelector,
			},
		},
	}

	deployment := BuildAgentFactoryDeployment(inst)
	affinity := deployment.Spec.Template.Spec.Affinity
	if affinity == nil || affinity.NodeAffinity == nil ||
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("expected required node affinity for workerNodeSelector")
	}
	required := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required.NodeSelectorTerms) != 1 {
		t.Fatalf("unexpected worker node selector affinity: %#v", required)
	}
	expressions := required.NodeSelectorTerms[0].MatchExpressions
	if len(expressions) != len(workerNodeSelector) {
		t.Fatalf("expected all worker node selector labels in affinity, got %#v", expressions)
	}
	for _, expression := range expressions {
		if expression.Operator != corev1.NodeSelectorOpIn || len(expression.Values) != 1 ||
			expression.Values[0] != workerNodeSelector[expression.Key] {
			t.Fatalf("unexpected worker node selector expression: %#v", expression)
		}
	}

	if len(deployment.Spec.Template.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected one topology spread constraint, got %#v", deployment.Spec.Template.Spec.TopologySpreadConstraints)
	}
	spread := deployment.Spec.Template.Spec.TopologySpreadConstraints[0]
	if spread.MaxSkew != 1 || spread.TopologyKey != corev1.LabelHostname || spread.WhenUnsatisfiable != corev1.ScheduleAnyway {
		t.Fatalf("unexpected topology spread constraint: %#v", spread)
	}
}

func TestBuildAgentFactoryDatabaseSetupJobUsesSeparateAdminAndRuntimeSecrets(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf", Generation: 7},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Image: &privateaiv4.AgentFactoryImageSpec{Name: "paf:test", PullSecret: "registry-secret"},
			},
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Details",
				Username:       "paf_app",
				Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{
					Protocol: "TCP", Host: "db.example.com", Port: 1521, ServiceName: "pafpdb",
				},
				Bootstrap: &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
					Enabled: &enabled,
					AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
						SecretName: "paf-bootstrap-admin", UsernameKey: "admin-user", PasswordKey: "admin-password", Privilege: "SYSDBA",
					},
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{Secret: &privateaiv4.AgentFactorySecretSpec{
				Name: "paf-runtime", DBPasswordKey: "paf-password",
			}},
		},
	}

	job := BuildAgentFactoryDatabaseSetupJob(inst)
	if job.Name != "agentfactory-sample-db-setup" {
		t.Fatalf("unexpected Job name %q", job.Name)
	}
	if job.Annotations[AgentFactoryDatabaseSetupGenerationLabel] != "7" {
		t.Fatalf("unexpected generation annotation: %#v", job.Annotations)
	}
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected one setup container, got %d", len(job.Spec.Template.Spec.Containers))
	}
	container := job.Spec.Template.Spec.Containers[0]
	if len(container.Command) != 1 || container.Command[0] != AgentFactoryDatabaseSetupCommand {
		t.Fatalf("unexpected setup command: %#v", container.Command)
	}
	var foundScriptMount bool
	for _, mount := range container.VolumeMounts {
		if mount.Name == "paf-db-setup-script" {
			foundScriptMount = true
			if mount.MountPath != AgentFactoryDatabaseSetupScriptMount || mount.SubPath != AgentFactoryDatabaseSetupScriptKey || !mount.ReadOnly {
				t.Fatalf("unexpected setup script mount: %#v", mount)
			}
		}
	}
	if !foundScriptMount {
		t.Fatalf("expected controller-provided setup script mount")
	}
	args := strings.Join(container.Args, " ")
	for _, value := range []string{"--admin-username-file", "/run/secrets/paf-bootstrap/username", "--admin-password-file", "/run/secrets/paf-bootstrap/password", "--admin-privilege", "SYSDBA", "--paf-username", "paf_app", "--paf-password-file", "/run/secrets/paf-runtime/password"} {
		if !strings.Contains(args, value) {
			t.Fatalf("setup args do not contain %q: %s", value, args)
		}
	}
	if got := job.Spec.Template.Spec.ImagePullSecrets[0].Name; got != "registry-secret" {
		t.Fatalf("expected image pull secret, got %q", got)
	}
	if got := job.Spec.Template.Spec.AutomountServiceAccountToken; got == nil || *got {
		t.Fatalf("expected setup Job service-account token automount disabled")
	}
	var foundScriptVolume bool

	secrets := map[string]corev1.SecretVolumeSource{}
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.Name == "paf-db-setup-script" {
			foundScriptVolume = true
			if volume.ConfigMap == nil || volume.ConfigMap.Name != AgentFactoryDatabaseSetupScriptConfigMapName(inst) {
				t.Fatalf("unexpected setup script volume: %#v", volume)
			}
			if volume.ConfigMap.DefaultMode == nil || *volume.ConfigMap.DefaultMode != 0555 {
				t.Fatalf("expected setup script volume mode 0555, got %#v", volume.ConfigMap.DefaultMode)
			}
		}
		if volume.Secret != nil {
			secrets[volume.Name] = *volume.Secret
		}
	}
	if !foundScriptVolume {
		t.Fatalf("expected controller-provided setup script volume")
	}
	if got := secrets["paf-bootstrap"].SecretName; got != "paf-bootstrap-admin" {
		t.Fatalf("unexpected admin Secret: %q", got)
	}
	if got := secrets["paf-runtime"].SecretName; got != "paf-runtime" {
		t.Fatalf("unexpected runtime Secret: %q", got)
	}
	if got := job.Spec.Template.Spec.RestartPolicy; got != corev1.RestartPolicyOnFailure {
		t.Fatalf("expected customer default restart policy OnFailure, got %s", got)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 3 {
		t.Fatalf("expected customer default backoff limit 3, got %#v", job.Spec.BackoffLimit)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 300 {
		t.Fatalf("expected customer default finished-job TTL 300, got %#v", job.Spec.TTLSecondsAfterFinished)
	}
}

func TestBuildAgentFactoryDatabaseSetupJobPreservesFailedJobWhenRequested(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-diagnostic", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Image: &privateaiv4.AgentFactoryImageSpec{Name: "paf:test"},
			},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Details",
				Username:       "paf_app",
				Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{
					Protocol: "TCP", Host: "db.example.com", Port: 1521, ServiceName: "pafpdb",
				},
				Bootstrap: &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
					Enabled:           &enabled,
					PreserveFailedJob: true,
					AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
						SecretName: "paf-bootstrap-admin",
					},
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{Secret: &privateaiv4.AgentFactorySecretSpec{
				Name: "paf-runtime", DBPasswordKey: "paf-password",
			}},
		},
	}

	job := BuildAgentFactoryDatabaseSetupJob(inst)
	if got := job.Spec.Template.Spec.RestartPolicy; got != corev1.RestartPolicyNever {
		t.Fatalf("expected diagnostic restart policy Never, got %s", got)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Fatalf("expected diagnostic backoff limit 0, got %#v", job.Spec.BackoffLimit)
	}
	if job.Spec.TTLSecondsAfterFinished != nil {
		t.Fatalf("expected failed diagnostic Job to have no TTL, got %d", *job.Spec.TTLSecondsAfterFinished)
	}
}

func TestBuildAgentFactoryDatabaseSetupJobMountsWallet(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-wallet", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Image: &privateaiv4.AgentFactoryImageSpec{Name: "paf:test"},
			},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Wallet",
				Username:       "paf_app",
				Wallet: &privateaiv4.AgentFactoryWalletSpec{
					SelectedTnsAlias: "ORCLPDB1_TCPS",
				},
				Bootstrap: &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
					Enabled: &enabled,
					AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
						SecretName: "paf-bootstrap-admin",
					},
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{Secret: &privateaiv4.AgentFactorySecretSpec{
				Name: "paf-runtime", DBPasswordKey: "paf-password", WalletZipKey: "wallet-bundle",
			}},
		},
	}

	job := BuildAgentFactoryDatabaseSetupJob(inst)
	args := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")
	if !strings.Contains(args, "--wallet-file "+AgentFactoryDatabaseSetupWalletFile) {
		t.Fatalf("expected Wallet setup argument, got %s", args)
	}

	var runtimeSecret corev1.SecretVolumeSource
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.Name == "paf-runtime" && volume.Secret != nil {
			runtimeSecret = *volume.Secret
			break
		}
	}
	if runtimeSecret.SecretName != "paf-runtime" {
		t.Fatalf("unexpected runtime Secret volume: %#v", runtimeSecret)
	}
	items := map[string]string{}
	for _, item := range runtimeSecret.Items {
		items[item.Path] = item.Key
	}
	if items["password"] != "paf-password" || items["db_wallet.zip"] != "wallet-bundle" {
		t.Fatalf("unexpected runtime Secret items: %#v", runtimeSecret.Items)
	}
}

func TestBuildAgentFactoryDatabaseSetupScriptConfigMap(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
	}

	configMap := BuildAgentFactoryDatabaseSetupScriptConfigMap(inst)
	if configMap.Name != "agentfactory-sample-db-setup-script" {
		t.Fatalf("unexpected setup script ConfigMap name %q", configMap.Name)
	}
	got := configMap.Data[AgentFactoryDatabaseSetupScriptKey]
	if got == "" {
		t.Fatal("expected setup script ConfigMap data")
	}
	for _, expected := range []string{
		"/mount/config/app/latest/backend/source_env.sh",
		"/usr/lib/oracle/23/client64/bin/sqlplus",
		"/bin:/usr/bin:/sbin:/usr/local/bin",
		"--wallet-file",
		"TNS_ADMIN",
		"MY_WALLET_DIRECTORY",
		"normalize_wallet_directory",
		"normalize_sqlnet_wallet_directory",
		"DIRECTORY = ",
		"unzip",
		"--admin-privilege",
		"CREATE USER",
		"DATA_PUMP_DIR",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("setup script does not contain %q", expected)
		}
	}
}

func TestBuildAgentFactoryDeploymentAddsDatabaseReadinessProbe(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Database: &privateaiv4.AgentFactoryDatabaseSpec{HealthPath: "/readyz"},
		},
	}
	deployment := BuildAgentFactoryDeployment(inst)
	probe := deployment.Spec.Template.Spec.Containers[0].ReadinessProbe
	if probe == nil || probe.HTTPGet == nil {
		t.Fatalf("expected HTTP database readiness probe")
	}
	if probe.HTTPGet.Path != "/readyz" || probe.HTTPGet.Port.IntValue() != 8080 || probe.HTTPGet.Scheme != corev1.URISchemeHTTPS {
		t.Fatalf("unexpected readiness probe: %#v", probe.HTTPGet)
	}
}

func TestBuildAgentFactoryDeploymentUsesPAFReadinessDefaults(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
	}
	deployment := BuildAgentFactoryDeployment(inst)
	probe := deployment.Spec.Template.Spec.Containers[0].ReadinessProbe
	if probe == nil || probe.HTTPGet == nil {
		t.Fatalf("expected HTTP readiness probe")
	}
	if probe.HTTPGet.Path != AgentFactoryDefaultHealthPath || probe.HTTPGet.Port.IntValue() != 8080 || probe.HTTPGet.Scheme != corev1.URISchemeHTTPS {
		t.Fatalf("unexpected default readiness probe: %#v", probe.HTTPGet)
	}
}

func TestBuildAgentFactoryPrivateLoadBalancerAddsOCIAnnotations(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "shns"},
		Spec: privateaiv4.AgentFactorySpec{
			Networking: &privateaiv4.AgentFactoryNetworkingSpec{
				Service: &privateaiv4.PaiServiceSpec{
					PrivateLoadBalancer: &privateaiv4.PaiLoadBalancerSpec{
						Enabled: &enabled,
					},
				},
			},
		},
	}

	services := BuildAgentFactoryServices(inst)
	privateLB := findAgentFactoryService(services, "agentfactory-sample-private-lb")
	if privateLB == nil {
		t.Fatalf("expected private LoadBalancer service")
	}
	if privateLB.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected LoadBalancer service, got %s", privateLB.Spec.Type)
	}
	if got := privateLB.Annotations[privateLoadBalancerTypeAnnotation]; got != "lb" {
		t.Fatalf("expected %s=lb, got %q", privateLoadBalancerTypeAnnotation, got)
	}
	if got := privateLB.Annotations[privateLoadBalancerInternalAnnotation]; got != "true" {
		t.Fatalf("expected %s=true, got %q", privateLoadBalancerInternalAnnotation, got)
	}
}

func TestBuildAgentFactoryPrivateLoadBalancerPreservesUserOCIAnnotation(t *testing.T) {
	enabled := true
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "shns"},
		Spec: privateaiv4.AgentFactorySpec{
			Networking: &privateaiv4.AgentFactoryNetworkingSpec{
				Service: &privateaiv4.PaiServiceSpec{
					PrivateLoadBalancer: &privateaiv4.PaiLoadBalancerSpec{
						Enabled: &enabled,
						Annotations: map[string]string{
							privateLoadBalancerTypeAnnotation:     "nlb",
							privateLoadBalancerInternalAnnotation: "true",
						},
					},
				},
			},
		},
	}

	services := BuildAgentFactoryServices(inst)
	privateLB := findAgentFactoryService(services, "agentfactory-sample-private-lb")
	if privateLB == nil {
		t.Fatalf("expected private LoadBalancer service")
	}
	if got := privateLB.Annotations[privateLoadBalancerTypeAnnotation]; got != "nlb" {
		t.Fatalf("expected user-provided %s=nlb to be preserved, got %q", privateLoadBalancerTypeAnnotation, got)
	}
}

func findAgentFactoryService(services []*corev1.Service, name string) *corev1.Service {
	for _, service := range services {
		if service.Name == name {
			return service
		}
	}
	return nil
}

func TestAgentFactoryInboundTLSIsMountedAndPublishedInRuntimeEnv(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Security: &privateaiv4.AgentFactorySecuritySpec{
				InboundTLS: &privateaiv4.AgentFactoryInboundTLSSpec{
					SecretName: "agentfactory-server-tls", MountLocation: "/custom/inbound-tls",
					CertificateKey: "cert.pem", PrivateKeyKey: "key.pem",
				},
			},
		},
	}

	env := RenderAgentFactoryRuntimeEnv(inst)
	for _, value := range []string{
		"AGENT_FACTORY_SERVER_CERTIFICATE_FILE=/custom/inbound-tls/server.crt",
		"AGENT_FACTORY_SERVER_PRIVATE_KEY_FILE=/custom/inbound-tls/server.key",
	} {
		if !strings.Contains(env, value) {
			t.Fatalf("expected runtime.env to contain %q, got:\n%s", value, env)
		}
	}

	deployment := BuildAgentFactoryDeployment(inst)
	var foundVolume, foundMount bool
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "inbound-tls" {
			continue
		}
		foundVolume = true
		if volume.Secret == nil || volume.Secret.SecretName != "agentfactory-server-tls" || len(volume.Secret.Items) != 2 ||
			volume.Secret.Items[0].Key != "cert.pem" || volume.Secret.Items[0].Path != "server.crt" ||
			volume.Secret.Items[1].Key != "key.pem" || volume.Secret.Items[1].Path != "server.key" {
			t.Fatalf("unexpected inbound TLS Secret volume: %#v", volume)
		}
	}
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == "inbound-tls" {
			foundMount = true
			if mount.MountPath != "/custom/inbound-tls" || !mount.ReadOnly {
				t.Fatalf("unexpected inbound TLS mount: %#v", mount)
			}
		}
	}
	if !foundVolume || !foundMount {
		t.Fatalf("expected inbound TLS Secret volume and mount, volume=%v mount=%v", foundVolume, foundMount)
	}
}

func TestBuildAgentFactoryImageUpgradePodUsesTargetImageAndIsolatedSelector(t *testing.T) {
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-upgrade", Namespace: "paf"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{Image: &privateaiv4.AgentFactoryImageSpec{Name: "paf:new", PullPolicy: corev1.PullAlways, PullSecret: "registry-secret"}},
			Upgrade: &privateaiv4.AgentFactoryUpgradeSpec{Strategy: "ImageUpgrade"},
		},
	}

	pod := BuildAgentFactoryImageUpgradePod(inst)
	if pod.Name != "agentfactory-upgrade-image-upgrade" {
		t.Fatalf("unexpected upgrade Pod name %q", pod.Name)
	}
	if got := pod.Labels[AgentFactoryImageUpgradeOperationLabel]; got != AgentFactoryImageUpgradeOperation {
		t.Fatalf("expected image-upgrade operation label, got %q", got)
	}
	if _, selected := AgentFactoryLabels(inst)[AgentFactoryImageUpgradeOperationLabel]; selected {
		t.Fatal("normal AgentFactory selector must not identify upgrade Pods")
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("expected Never restart policy, got %q", pod.Spec.RestartPolicy)
	}
	if pod.Spec.ActiveDeadlineSeconds == nil || *pod.Spec.ActiveDeadlineSeconds != 3600 {
		t.Fatalf("expected default active deadline 3600, got %#v", pod.Spec.ActiveDeadlineSeconds)
	}
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected one upgrade container, got %d", len(pod.Spec.Containers))
	}
	container := pod.Spec.Containers[0]
	if container.Image != "paf:new" || len(container.Args) != 1 || container.Args[0] != "upgrade" {
		t.Fatalf("expected new image and fixed upgrade arg, got image=%q args=%#v", container.Image, container.Args)
	}
	if container.ReadinessProbe != nil || container.LivenessProbe != nil || container.StartupProbe != nil {
		t.Fatal("upgrade Pod must not have normal workload probes")
	}
	if len(pod.Spec.TopologySpreadConstraints) != 0 {
		t.Fatal("upgrade Pod must not carry normal workload spread constraints")
	}
}
