/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package commons

import (
	"fmt"
	"path"
	"sort"
	"strings"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// AgentFactoryRuntimeConfigKey is the ConfigMap key containing runtime.env.
	AgentFactoryRuntimeConfigKey = "runtime.env"

	// AgentFactoryDefaultConfigMount is the default runtime configuration mount path.
	AgentFactoryDefaultConfigMount = "/etc/agent-factory/runtime.env"
	// AgentFactoryDefaultSecretMount is the default Secret mount path.
	AgentFactoryDefaultSecretMount = "/run/secrets"
	// AgentFactoryWalletPasswordFilename is the fixed filename used inside the
	// AgentFactory container for an optional encrypted-wallet password.
	AgentFactoryWalletPasswordFilename = "agent_factory_db_wallet_password"
	// AgentFactoryDefaultOutboundCAMount is the default outbound CA mount path.
	AgentFactoryDefaultOutboundCAMount = "/run/agentfactory/outbound-ca"
	// AgentFactoryDefaultInboundTLSMount is the default inbound TLS mount path.
	AgentFactoryDefaultInboundTLSMount = "/run/agentfactory/inbound-tls"
	// AgentFactoryDefaultMountPath is the default application mount path.
	AgentFactoryDefaultMountPath = "/mount"
	// AgentFactoryDefaultLogPath is the default application log path.
	AgentFactoryDefaultLogPath = "/var/log/agentfactory"
	// AgentFactoryDefaultHealthPath is the default Agent Factory health endpoint.
	AgentFactoryDefaultHealthPath = "/agentFactory/v1/getAppState"
	// AgentFactoryDefaultRollingUpdateMaxSurge is the default temporary capacity
	// added during an AgentFactory rolling update.
	AgentFactoryDefaultRollingUpdateMaxSurge = "25%"
	// AgentFactoryDefaultTerminationGracePeriodSeconds is the default time a Pod
	// has to terminate gracefully during an AgentFactory rollout.
	AgentFactoryDefaultTerminationGracePeriodSeconds int64 = 3600

	// AgentFactoryConfigRVAnnotation tracks the runtime ConfigMap resource version.
	AgentFactoryConfigRVAnnotation = "agentfactory.oracle.com/config-rv"
	// AgentFactorySecretRVAnnotation tracks the runtime Secret resource version.
	AgentFactorySecretRVAnnotation = "agentfactory.oracle.com/secret-rv"
	// AgentFactoryOutboundTLSRVAnnotation tracks the outbound TLS Secret resource version.
	AgentFactoryOutboundTLSRVAnnotation = "agentfactory.oracle.com/outbound-tls-rv"
	// AgentFactoryImageUpgradeOperationLabel identifies the one-shot image upgrade Pod.
	AgentFactoryImageUpgradeOperationLabel = "agentfactory.oracle.com/operation"
	AgentFactoryImageUpgradeOperation      = "image-upgrade"

	privateLoadBalancerTypeAnnotation     = "oci.oraclecloud.com/load-balancer-type"
	privateLoadBalancerInternalAnnotation = "service.beta.kubernetes.io/oci-load-balancer-internal"
)

// AgentFactoryLabels returns labels shared by generated AgentFactory resources.
func AgentFactoryLabels(inst *privateaiv4.AgentFactory) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "agentfactory",
		"app.kubernetes.io/instance":   inst.Name,
		"app.kubernetes.io/managed-by": "oracle-database-operator",
	}
}

// AgentFactoryRuntimeConfigMapName returns the generated runtime ConfigMap name.
func AgentFactoryRuntimeConfigMapName(inst *privateaiv4.AgentFactory) string {
	return inst.Name + "-runtime"
}

// BuildAgentFactoryRuntimeConfigMap builds the generated runtime.env ConfigMap.
func BuildAgentFactoryRuntimeConfigMap(inst *privateaiv4.AgentFactory) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentFactoryRuntimeConfigMapName(inst),
			Namespace: inst.Namespace,
			Labels:    AgentFactoryLabels(inst),
		},
		Data: map[string]string{
			AgentFactoryRuntimeConfigKey: RenderAgentFactoryRuntimeEnv(inst),
		},
	}
}

// RenderAgentFactoryRuntimeEnv renders runtime.env without secret values.
func RenderAgentFactoryRuntimeEnv(inst *privateaiv4.AgentFactory) string {
	cfg := inst.Spec.Configuration
	db := inst.Spec.Database

	mode := "prod"
	silentInstall := "true"
	uiAdmin := ""
	bindHost := "0.0.0.0"
	if cfg != nil {
		if strings.TrimSpace(cfg.Mode) != "" {
			mode = strings.TrimSpace(cfg.Mode)
		}
		if cfg.SilentInstall != nil && !*cfg.SilentInstall {
			silentInstall = "false"
		}
		uiAdmin = strings.TrimSpace(cfg.UIAdminUsername)
		if strings.TrimSpace(cfg.BindHost) != "" {
			bindHost = strings.TrimSpace(cfg.BindHost)
		}
	}

	connectionType := ""
	dbUser := ""
	airGapped := ""
	swagger := "false"
	statePollInterval := int32(5)
	apiReadyTimeout := int32(600)
	installTimeout := int32(1800)
	readyTimeout := int32(600)
	if cfg != nil {
		if cfg.EnableSwagger != nil && *cfg.EnableSwagger {
			swagger = "true"
		}
		statePollInterval = agentFactoryTimeoutValue(cfg.StatePollIntervalSeconds, statePollInterval)
		apiReadyTimeout = agentFactoryTimeoutValue(cfg.APIReadyTimeoutSeconds, apiReadyTimeout)
		installTimeout = agentFactoryTimeoutValue(cfg.InstallTimeoutSeconds, installTimeout)
		readyTimeout = agentFactoryTimeoutValue(cfg.ReadyTimeoutSeconds, readyTimeout)
	}
	lines := []string{
		"AGENT_FACTORY_MODE=" + mode,
		"AGENT_FACTORY_SILENT_INSTALL=" + silentInstall,
		"AGENT_FACTORY_ADMIN_USERNAME=" + uiAdmin,
		"AGENT_FACTORY_ENABLE_SWAGGER=" + swagger,
		fmt.Sprintf("AGENT_FACTORY_STATE_POLL_INTERVAL_SECONDS=%d", statePollInterval),
		fmt.Sprintf("AGENT_FACTORY_API_READY_TIMEOUT_SECONDS=%d", apiReadyTimeout),
		fmt.Sprintf("AGENT_FACTORY_INSTALL_TIMEOUT_SECONDS=%d", installTimeout),
		fmt.Sprintf("AGENT_FACTORY_READY_TIMEOUT_SECONDS=%d", readyTimeout),
	}
	if db != nil {
		if strings.TrimSpace(db.ConnectionType) != "" {
			connectionType = strings.TrimSpace(db.ConnectionType)
		}
		dbUser = strings.TrimSpace(db.Username)
		if db.AirGapped != nil {
			airGapped = yesNoDefaultTrue(db.AirGapped)
		}
	}
	lines = append(lines,
		"AGENT_FACTORY_DB_CONNECTION_TYPE="+connectionType,
		"AGENT_FACTORY_DB_USERNAME="+dbUser,
		"AGENT_FACTORY_DB_IS_AIR_GAPPED="+airGapped,
	)

	if strings.EqualFold(connectionType, "Details") {
		details := (*privateaiv4.AgentFactoryDatabaseDetailsSpec)(nil)
		if db != nil {
			details = db.Details
		}
		protocol := ""
		host := ""
		port := int32(0)
		serviceName := ""
		if details != nil {
			if strings.TrimSpace(details.Protocol) != "" {
				protocol = strings.TrimSpace(details.Protocol)
			}
			host = strings.TrimSpace(details.Host)
			port = details.Port
			serviceName = strings.TrimSpace(details.ServiceName)
		}
		lines = append(lines,
			"AGENT_FACTORY_DB_USES_WALLET=no",
			"AGENT_FACTORY_DB_PROTOCOL="+protocol,
			"AGENT_FACTORY_DB_HOST="+host,
			fmt.Sprintf("AGENT_FACTORY_DB_PORT=%d", port),
			"AGENT_FACTORY_DB_SERVICE_NAME="+serviceName,
		)
	} else {
		wallet := (*privateaiv4.AgentFactoryWalletSpec)(nil)
		if db != nil {
			wallet = db.Wallet
		}
		hasOciCerts := ""
		tnsAlias := ""
		if wallet != nil {
			if wallet.HasOciCertificates != nil {
				hasOciCerts = yesNoDefaultTrue(wallet.HasOciCertificates)
			}
			tnsAlias = strings.TrimSpace(wallet.SelectedTnsAlias)
		}
		lines = append(lines,
			"AGENT_FACTORY_DB_USES_WALLET=yes",
			"AGENT_FACTORY_DB_WALLET_HAS_OCI_CERTIFICATES="+hasOciCerts,
			"AGENT_FACTORY_DB_SELECTED_TNS_ALIAS="+tnsAlias,
		)
	}

	lines = append(lines, "AGENT_FACTORY_BIND_HOST="+bindHost)
	if cfg != nil {
		appendRuntimeEnvValue(&lines, "AGENT_FACTORY_CERT_FQDN", cfg.CertFQDN)
		appendRuntimeEnvValue(&lines, "AGENT_FACTORY_CERT_IP_ADDRESS", cfg.CertIPAddress)
		appendRuntimeEnvValue(&lines, "AGENT_FACTORY_HTTP_PROXY", cfg.HTTPProxy)
		appendRuntimeEnvValue(&lines, "AGENT_FACTORY_HTTPS_PROXY", cfg.HTTPSProxy)
		appendRuntimeEnvValue(&lines, "AGENT_FACTORY_NO_PROXY", cfg.NoProxy)
		appendProxyRuntimeEnv(&lines, cfg.HTTPProxy, "HTTP_PROXY", "http_proxy")
		appendProxyRuntimeEnv(&lines, cfg.HTTPSProxy, "HTTPS_PROXY", "https_proxy")
		appendProxyRuntimeEnv(&lines, cfg.NoProxy, "NO_PROXY", "no_proxy")
		appendAgentFactoryLLM(&lines, cfg.LLM)
	}
	appendAgentFactoryOutboundTLSEnv(&lines, inst)
	appendAgentFactoryInboundTLSEnv(&lines, inst)
	return strings.Join(lines, "\n") + "\n"
}

func appendRuntimeEnvValue(lines *[]string, key, value string) {
	if strings.TrimSpace(value) != "" {
		*lines = append(*lines, key+"="+strings.TrimSpace(value))
	}
}

func agentFactoryTimeoutValue(value *int32, fallback int32) int32 {
	if value != nil && *value > 0 {
		return *value
	}
	return fallback
}

func appendProxyRuntimeEnv(lines *[]string, value, upper, lower string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*lines = append(*lines, upper+"="+value, lower+"="+value)
	}
}

func appendRuntimeEnvInt32(lines *[]string, key string, value *int32) {
	if value != nil {
		*lines = append(*lines, fmt.Sprintf("%s=%d", key, *value))
	}
}

func appendAgentFactoryLLM(lines *[]string, llm *privateaiv4.AgentFactoryLLMSpec) {
	if llm == nil || strings.TrimSpace(llm.Provider) == "" {
		return
	}
	appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_PROVIDER", llm.Provider)
	name := strings.TrimSpace(llm.Name)
	if name == "" {
		name = "llm_model_entry"
	}
	appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_NAME", name)
	if llm.OCI != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_MODEL_ID", llm.OCI.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_SERVICE_ENDPOINT", llm.OCI.ServiceEndpoint)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_COMPARTMENT_ID", llm.OCI.CompartmentID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_USER_OCID", llm.OCI.UserOCID)
		fingerprintFile := llm.OCI.FingerprintFile
		if strings.TrimSpace(fingerprintFile) == "" {
			fingerprintFile = "agent_factory_llm_oci_fingerprint"
		}
		privateKeyFile := llm.OCI.PrivateKeyFile
		if strings.TrimSpace(privateKeyFile) == "" {
			privateKeyFile = "agent_factory_llm_oci_private_key.pem"
		}
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_FINGERPRINT_FILE", fingerprintFile)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_TENANCY_OCID", llm.OCI.TenancyOCID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_REGION", llm.OCI.Region)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OCI_PRIVATE_KEY_FILE", privateKeyFile)
	}
	if llm.VLLM != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_VLLM_MODEL_ID", llm.VLLM.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_VLLM_HOST", llm.VLLM.Host)
		if llm.VLLM.Port != 0 {
			appendRuntimeEnvInt32(lines, "AGENT_FACTORY_LLM_VLLM_PORT", &llm.VLLM.Port)
		}
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_VLLM_API_KEY_FILE", llm.VLLM.APIKeyFile)
	}
	if llm.OpenAI != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OPENAI_MODEL_ID", llm.OpenAI.ModelID)
		apiKeyFile := llm.OpenAI.APIKeyFile
		if strings.TrimSpace(apiKeyFile) == "" {
			apiKeyFile = "agent_factory_llm_openai_api_key"
		}
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OPENAI_API_KEY_FILE", apiKeyFile)
	}
	if llm.OpenAICompatible != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OPENAI_COMPATIBLE_MODEL_ID", llm.OpenAICompatible.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OPENAI_COMPATIBLE_BASE_URL", llm.OpenAICompatible.BaseURL)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OPENAI_COMPATIBLE_API_KEY_FILE", llm.OpenAICompatible.APIKeyFile)
	}
	if llm.PrivateAIServices != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_MODEL_ID", llm.PrivateAIServices.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_BASE_URL", llm.PrivateAIServices.BaseURL)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_API_KEY_FILE", llm.PrivateAIServices.APIKeyFile)
	}
	if llm.Ollama != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OLLAMA_MODEL_ID", llm.Ollama.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_OLLAMA_HOST", llm.Ollama.Host)
		if llm.Ollama.Port != 0 {
			appendRuntimeEnvInt32(lines, "AGENT_FACTORY_LLM_OLLAMA_PORT", &llm.Ollama.Port)
		}
	}
	if llm.Google != nil {
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_GOOGLE_MODEL_ID", llm.Google.ModelID)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_GOOGLE_CREDENTIALS_TYPE", llm.Google.CredentialsType)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_GOOGLE_API_KEY_FILE", llm.Google.APIKeyFile)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_GOOGLE_REGION", llm.Google.Region)
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_GOOGLE_SERVICE_ACCOUNT_FILE", llm.Google.ServiceAccountFile)
	}
}

func appendAgentFactoryOutboundTLSEnv(lines *[]string, inst *privateaiv4.AgentFactory) {
	bundle := agentFactoryOutboundCABundle(inst)
	if bundle == nil {
		return
	}
	bundlePath := agentFactoryOutboundCABundlePath(bundle)
	appendRuntimeEnvValue(lines, "AGENT_FACTORY_OUTBOUND_CA_BUNDLE", bundlePath)
	if inst.Spec.Configuration != nil && inst.Spec.Configuration.LLM != nil &&
		strings.EqualFold(strings.TrimSpace(inst.Spec.Configuration.LLM.Provider), "private_ai_services") {
		// The image currently consumes this provider-specific environment key.
		// The CA reference itself is owned by security.outboundTLS.
		appendRuntimeEnvValue(lines, "AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_CA_FILE", bundlePath)
	}
}

// BuildAgentFactoryDeployment builds a Deployment for AgentFactory.
func BuildAgentFactoryDeployment(inst *privateaiv4.AgentFactory) *appsv1.Deployment {
	replicas := agentFactoryReplicas(inst)
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        inst.Name,
			Namespace:   inst.Namespace,
			Labels:      AgentFactoryLabels(inst),
			Annotations: agentFactoryWorkloadAnnotations(inst),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: AgentFactoryLabels(inst)},
			Strategy: agentFactoryDeploymentStrategy(inst),
			Template: agentFactoryPodTemplate(inst),
		},
	}
}

// AgentFactoryImageUpgradePodName returns the stable name of the one-shot upgrade Pod.
func AgentFactoryImageUpgradePodName(inst *privateaiv4.AgentFactory) string {
	name := inst.Name + "-image-upgrade"
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

// BuildAgentFactoryImageUpgradePod builds the one-shot Pod that runs the new
// image with the fixed upgrade argument. It intentionally does not carry the
// normal AgentFactory selector labels, so Services and the Deployment cannot
// adopt it while the workload is stopped.
func BuildAgentFactoryImageUpgradePod(inst *privateaiv4.AgentFactory) *corev1.Pod {
	template := agentFactoryPodTemplate(inst)
	podSpec := *template.Spec.DeepCopy()
	container := agentFactoryMainContainer(inst)
	container.Args = []string{"upgrade"}
	container.ReadinessProbe = nil
	container.LivenessProbe = nil
	container.StartupProbe = nil
	podSpec.Containers = []corev1.Container{container}
	podSpec.RestartPolicy = corev1.RestartPolicyNever
	podSpec.TopologySpreadConstraints = nil
	activeDeadlineSeconds := int64(3600)
	if inst.Spec.Upgrade != nil && inst.Spec.Upgrade.ImageUpgrade != nil && inst.Spec.Upgrade.ImageUpgrade.ActiveDeadlineSeconds != nil {
		activeDeadlineSeconds = *inst.Spec.Upgrade.ImageUpgrade.ActiveDeadlineSeconds
	}
	podSpec.ActiveDeadlineSeconds = &activeDeadlineSeconds

	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentFactoryImageUpgradePodName(inst),
			Namespace: inst.Namespace,
			Labels: map[string]string{
				AgentFactoryImageUpgradeOperationLabel: AgentFactoryImageUpgradeOperation,
				"agentfactory.oracle.com/instance":     inst.Name,
			},
		},
		Spec: podSpec,
	}
}

// BuildAgentFactoryPVCs builds shared PVCs managed by the controller.
func BuildAgentFactoryPVCs(inst *privateaiv4.AgentFactory) []*corev1.PersistentVolumeClaim {
	var claims []*corev1.PersistentVolumeClaim
	for _, volume := range agentFactoryVolumes(inst) {
		if volume.Source.PVC == nil {
			continue
		}
		claims = append(claims, buildAgentFactoryPVC(inst, volume.Name, volume.Source.PVC))
	}
	return claims
}

// BuildAgentFactoryServices builds enabled cluster/public/private services.
func BuildAgentFactoryServices(inst *privateaiv4.AgentFactory) []*corev1.Service {
	var services []*corev1.Service
	if agentFactoryClusterServiceEnabled(inst) {
		services = append(services, buildAgentFactoryService(inst, AgentFactoryClusterServiceName(inst), corev1.ServiceTypeClusterIP, nil, agentFactoryClusterServiceAnnotations(inst)))
	}
	if lb := agentFactoryPublicLoadBalancer(inst); lb != nil && boolDefaultFalse(lb.Enabled) {
		services = append(services, buildAgentFactoryService(inst, inst.Name+"-public-lb", corev1.ServiceTypeLoadBalancer, lb, lb.Annotations))
	}
	if lb := agentFactoryPrivateLoadBalancer(inst); lb != nil && boolDefaultFalse(lb.Enabled) {
		annotations := copyAgentFactoryStringMap(lb.Annotations)
		if annotations == nil {
			annotations = map[string]string{}
		}
		if _, ok := annotations[privateLoadBalancerTypeAnnotation]; !ok {
			annotations[privateLoadBalancerTypeAnnotation] = "lb"
		}
		if _, ok := annotations[privateLoadBalancerInternalAnnotation]; !ok {
			annotations[privateLoadBalancerInternalAnnotation] = "true"
		}
		services = append(services, buildAgentFactoryService(inst, inst.Name+"-private-lb", corev1.ServiceTypeLoadBalancer, lb, annotations))
	}
	return services
}

// AgentFactoryClusterServiceName returns the default in-cluster service name.
func AgentFactoryClusterServiceName(inst *privateaiv4.AgentFactory) string {
	return inst.Name
}

func agentFactoryPodTemplate(inst *privateaiv4.AgentFactory) corev1.PodTemplateSpec {
	automountServiceAccountToken := agentFactoryAutomountServiceAccountToken(inst)
	terminationGracePeriodSeconds := agentFactoryTerminationGracePeriodSeconds(inst)
	podSpec := corev1.PodSpec{
		AutomountServiceAccountToken:  &automountServiceAccountToken,
		ServiceAccountName:            agentFactoryServiceAccountName(inst),
		Containers:                    []corev1.Container{agentFactoryMainContainer(inst)},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes:                       agentFactoryVolumesForPod(inst),
	}
	if pullSecret := agentFactoryImagePullSecret(inst); pullSecret != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: pullSecret}}
	}
	if nodeAffinity := agentFactoryNodeAffinity(inst); nodeAffinity != nil {
		podSpec.Affinity = &corev1.Affinity{
			NodeAffinity: nodeAffinity,
		}
	}
	podSpec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		MaxSkew:           1,
		TopologyKey:       corev1.LabelHostname,
		WhenUnsatisfiable: corev1.ScheduleAnyway,
		LabelSelector:     &metav1.LabelSelector{MatchLabels: AgentFactoryLabels(inst)},
	}}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      AgentFactoryLabels(inst),
			Annotations: agentFactoryPodAnnotations(inst),
		},
		Spec: podSpec,
	}
}

func agentFactoryServiceAccountName(inst *privateaiv4.AgentFactory) string {
	if inst.Spec.Runtime == nil {
		return ""
	}
	return strings.TrimSpace(inst.Spec.Runtime.ServiceAccountName)
}

func agentFactoryMainContainer(inst *privateaiv4.AgentFactory) corev1.Container {
	container := corev1.Container{
		Name:            "agentfactory",
		Image:           agentFactoryImage(inst),
		ImagePullPolicy: agentFactoryImagePullPolicy(inst),
		Ports:           agentFactoryContainerPorts(inst),
		Env:             append([]corev1.EnvVar{}, agentFactoryRuntimeEnv(inst)...),
		VolumeMounts:    agentFactoryVolumeMounts(inst),
		ReadinessProbe:  agentFactoryReadinessProbe(inst),
	}
	if resources := agentFactoryResources(inst); resources != nil {
		container.Resources = *resources
	}
	return container
}

func agentFactoryReadinessProbe(inst *privateaiv4.AgentFactory) *corev1.Probe {
	path := AgentFactoryDefaultHealthPath
	if inst.Spec.Database != nil && strings.TrimSpace(inst.Spec.Database.HealthPath) != "" {
		path = strings.TrimSpace(inst.Spec.Database.HealthPath)
	}
	port := intstr.FromInt(8080)
	ports := agentFactoryPorts(inst)
	if len(ports) > 0 && ports[0].TargetPort != 0 {
		port = intstr.FromInt(int(ports[0].TargetPort))
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path:   path,
			Port:   port,
			Scheme: corev1.URISchemeHTTPS,
		}},
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    6,
	}
}

func agentFactoryVolumeMounts(inst *privateaiv4.AgentFactory) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "runtime-config",
			MountPath: AgentFactoryDefaultConfigMount,
			SubPath:   AgentFactoryRuntimeConfigKey,
			ReadOnly:  true,
		},
	}
	if secret := agentFactorySecret(inst); secret != nil && strings.TrimSpace(secret.Name) != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "runtime-secret",
			MountPath: agentFactorySecretMount(secret),
			ReadOnly:  true,
		})
	}
	if agentFactoryOutboundCABundle(inst) != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "outbound-ca",
			MountPath: AgentFactoryDefaultOutboundCAMount,
			ReadOnly:  true,
		})
	}
	if agentFactoryInboundTLS(inst) != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "inbound-tls",
			MountPath: AgentFactoryInboundTLSMount(inst),
			ReadOnly:  true,
		})
	}
	for _, volume := range agentFactoryVolumes(inst) {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: volume.MountPath,
		})
	}
	return mounts
}

func agentFactoryVolumesForPod(inst *privateaiv4.AgentFactory) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "runtime-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: AgentFactoryRuntimeConfigMapName(inst)},
				},
			},
		},
	}
	if secret := agentFactorySecret(inst); secret != nil && strings.TrimSpace(secret.Name) != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "runtime-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: strings.TrimSpace(secret.Name),
					Items:      agentFactorySecretItems(inst, secret),
				},
			},
		})
	}
	if bundle := agentFactoryOutboundCABundle(inst); bundle != nil {
		key := agentFactoryOutboundCABundleKey(bundle)
		volumes = append(volumes, corev1.Volume{
			Name: "outbound-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: strings.TrimSpace(bundle.SecretName),
					Items:      []corev1.KeyToPath{{Key: key, Path: path.Base(key)}},
				},
			},
		})
	}
	if inboundTLS := agentFactoryInboundTLS(inst); inboundTLS != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "inbound-tls",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: strings.TrimSpace(inboundTLS.SecretName),
				Items: []corev1.KeyToPath{
					{Key: AgentFactoryInboundTLSCertificateKey(inboundTLS), Path: "server.crt"},
					{Key: AgentFactoryInboundTLSPrivateKey(inboundTLS), Path: "server.key"},
				},
			}},
		})
	}

	for _, volume := range agentFactoryVolumes(inst) {
		switch {
		case volume.Source.ExistingPVC != nil:
			volumes = append(volumes, corev1.Volume{
				Name: volume.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: volume.Source.ExistingPVC.ClaimName},
				},
			})
		case volume.Source.PVC != nil:
			volumes = append(volumes, corev1.Volume{
				Name: volume.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: agentFactoryPVCName(inst, volume.Name)},
				},
			})
		case volume.Source.EmptyDir != nil:
			volumes = append(volumes, corev1.Volume{
				Name: volume.Name,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: agentFactoryEmptyDir(volume.Source.EmptyDir),
				},
			})
		}
	}
	return volumes
}

func agentFactorySecretItems(inst *privateaiv4.AgentFactory, secret *privateaiv4.AgentFactorySecretSpec) []corev1.KeyToPath {
	items := []corev1.KeyToPath{
		{Key: agentFactorySecretKey(secret.UIAdminPasswordKey, "agent_factory_admin_password"), Path: "agent_factory_admin_password"},
		{Key: agentFactorySecretKey(secret.DBPasswordKey, "agent_factory_db_password"), Path: "agent_factory_db_password"},
	}
	connectionType := "Wallet"
	if inst.Spec.Database != nil && strings.TrimSpace(inst.Spec.Database.ConnectionType) != "" {
		connectionType = strings.TrimSpace(inst.Spec.Database.ConnectionType)
	}
	if strings.EqualFold(connectionType, "Wallet") {
		items = append(items, corev1.KeyToPath{
			Key:  agentFactorySecretKey(secret.WalletZipKey, "db_wallet.zip"),
			Path: "db_wallet.zip",
		})
		if strings.TrimSpace(secret.WalletPasswordKey) != "" {
			items = append(items, corev1.KeyToPath{
				Key:  strings.TrimSpace(secret.WalletPasswordKey),
				Path: AgentFactoryWalletPasswordFilename,
			})
		}
	}
	if inst.Spec.Configuration != nil && inst.Spec.Configuration.LLM != nil {
		items = append(items, agentFactoryLLMSecretItems(inst.Spec.Configuration.LLM)...)
	}
	return items
}

func agentFactorySecretKey(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func agentFactoryLLMSecretItems(llm *privateaiv4.AgentFactoryLLMSpec) []corev1.KeyToPath {
	items := []corev1.KeyToPath{}
	add := func(file string) {
		file = strings.TrimSpace(file)
		if file != "" {
			items = append(items, corev1.KeyToPath{Key: file, Path: file})
		}
	}
	switch strings.ToLower(strings.TrimSpace(llm.Provider)) {
	case "oci":
		if llm.OCI != nil && (llm.OCI.UserOCID != "" || llm.OCI.FingerprintFile != "" || llm.OCI.TenancyOCID != "" || llm.OCI.Region != "" || llm.OCI.PrivateKeyFile != "") {
			add(agentFactorySecretKey(llm.OCI.FingerprintFile, "agent_factory_llm_oci_fingerprint"))
			add(agentFactorySecretKey(llm.OCI.PrivateKeyFile, "agent_factory_llm_oci_private_key.pem"))
		}
	case "vllm":
		if llm.VLLM != nil {
			add(llm.VLLM.APIKeyFile)
		}
	case "openai":
		if llm.OpenAI != nil {
			add(agentFactorySecretKey(llm.OpenAI.APIKeyFile, "agent_factory_llm_openai_api_key"))
		}
	case "openai_compatible":
		if llm.OpenAICompatible != nil {
			add(llm.OpenAICompatible.APIKeyFile)
		}
	case "private_ai_services":
		if llm.PrivateAIServices != nil {
			add(llm.PrivateAIServices.APIKeyFile)
		}
	case "google":
		if llm.Google != nil {
			switch strings.ToUpper(strings.TrimSpace(llm.Google.CredentialsType)) {
			case "API_KEY":
				add(agentFactorySecretKey(llm.Google.APIKeyFile, "agent_factory_llm_google_api_key"))
			case "GOOGLE_SERVICE_ACCOUNT":
				add(agentFactorySecretKey(llm.Google.ServiceAccountFile, "agent_factory_llm_google_service_account.json"))
			}
		}
	}
	return items
}

func agentFactoryOutboundCABundle(inst *privateaiv4.AgentFactory) *privateaiv4.AgentFactoryOutboundCABundleSpec {
	if inst == nil || inst.Spec.Security == nil || inst.Spec.Security.OutboundTLS == nil {
		return nil
	}
	return inst.Spec.Security.OutboundTLS.CABundle
}

func agentFactoryOutboundCABundleKey(bundle *privateaiv4.AgentFactoryOutboundCABundleSpec) string {
	if bundle != nil && strings.TrimSpace(bundle.Key) != "" {
		return strings.TrimSpace(bundle.Key)
	}
	return "ca.crt"
}

func agentFactoryOutboundCABundlePath(bundle *privateaiv4.AgentFactoryOutboundCABundleSpec) string {
	return AgentFactoryDefaultOutboundCAMount + "/" + path.Base(agentFactoryOutboundCABundleKey(bundle))
}

func appendAgentFactoryInboundTLSEnv(lines *[]string, inst *privateaiv4.AgentFactory) {
	if agentFactoryInboundTLS(inst) == nil {
		return
	}
	appendRuntimeEnvValue(lines, "AGENT_FACTORY_SERVER_CERTIFICATE_FILE", AgentFactoryInboundTLSCertificatePath(inst))
	appendRuntimeEnvValue(lines, "AGENT_FACTORY_SERVER_PRIVATE_KEY_FILE", AgentFactoryInboundTLSPrivateKeyPath(inst))
}

func agentFactoryInboundTLS(inst *privateaiv4.AgentFactory) *privateaiv4.AgentFactoryInboundTLSSpec {
	if inst == nil || inst.Spec.Security == nil {
		return nil
	}
	return inst.Spec.Security.InboundTLS
}

func AgentFactoryInboundTLSMount(inst *privateaiv4.AgentFactory) string {
	inboundTLS := agentFactoryInboundTLS(inst)
	if inboundTLS == nil || strings.TrimSpace(inboundTLS.MountLocation) == "" {
		return AgentFactoryDefaultInboundTLSMount
	}
	return strings.TrimRight(strings.TrimSpace(inboundTLS.MountLocation), "/")
}

func AgentFactoryInboundTLSCertificateKey(inboundTLS *privateaiv4.AgentFactoryInboundTLSSpec) string {
	if inboundTLS != nil && strings.TrimSpace(inboundTLS.CertificateKey) != "" {
		return strings.TrimSpace(inboundTLS.CertificateKey)
	}
	return "tls.crt"
}

func AgentFactoryInboundTLSPrivateKey(inboundTLS *privateaiv4.AgentFactoryInboundTLSSpec) string {
	if inboundTLS != nil && strings.TrimSpace(inboundTLS.PrivateKeyKey) != "" {
		return strings.TrimSpace(inboundTLS.PrivateKeyKey)
	}
	return "tls.key"
}

// AgentFactoryInboundTLSCertificatePath returns the fixed file path used in
// runtime.env, independent of the Secret keys selected by the user.
func AgentFactoryInboundTLSCertificatePath(inst *privateaiv4.AgentFactory) string {
	return path.Join(AgentFactoryInboundTLSMount(inst), "server.crt")
}

// AgentFactoryInboundTLSPrivateKeyPath returns the fixed file path used in
// runtime.env, independent of the Secret keys selected by the user.
func AgentFactoryInboundTLSPrivateKeyPath(inst *privateaiv4.AgentFactory) string {
	return path.Join(AgentFactoryInboundTLSMount(inst), "server.key")
}

func buildAgentFactoryPVC(inst *privateaiv4.AgentFactory, name string, spec *privateaiv4.AgentFactoryPVCSource) *corev1.PersistentVolumeClaim {
	accessModes := spec.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	}
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentFactoryPVCName(inst, name),
			Namespace: inst.Namespace,
			Labels:    AgentFactoryLabels(inst),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(spec.Size)},
			},
		},
	}
	if strings.TrimSpace(spec.StorageClass) != "" {
		pvc.Spec.StorageClassName = &spec.StorageClass
	}
	return pvc
}

func buildAgentFactoryService(inst *privateaiv4.AgentFactory, name string, svcType corev1.ServiceType, lb *privateaiv4.PaiLoadBalancerSpec, annotations map[string]string) *corev1.Service {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   inst.Namespace,
			Labels:      AgentFactoryLabels(inst),
			Annotations: copyAgentFactoryStringMap(annotations),
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: AgentFactoryLabels(inst),
			Ports:    agentFactoryServicePorts(inst),
		},
	}
	if lb != nil {
		if strings.TrimSpace(lb.IP) != "" {
			service.Spec.LoadBalancerIP = strings.TrimSpace(lb.IP)
		}
		if strings.EqualFold(lb.ExternalTrafficPolicy, "Local") {
			service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyLocal
		} else {
			service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
		}
	}
	return service
}

func agentFactoryPVCName(inst *privateaiv4.AgentFactory, volumeName string) string {
	return inst.Name + "-" + volumeName
}

func agentFactoryVolumes(inst *privateaiv4.AgentFactory) []privateaiv4.AgentFactoryVolumeSpec {
	if inst.Spec.Storage != nil && len(inst.Spec.Storage.Volumes) > 0 {
		return inst.Spec.Storage.Volumes
	}
	return []privateaiv4.AgentFactoryVolumeSpec{
		{
			Name:      "app-mount",
			MountPath: AgentFactoryDefaultMountPath,
			Source: privateaiv4.AgentFactoryVolumeSource{
				PVC: &privateaiv4.AgentFactoryPVCSource{
					Size:        "100Gi",
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				},
			},
		},
	}
}

func agentFactoryServicePorts(inst *privateaiv4.AgentFactory) []corev1.ServicePort {
	ports := agentFactoryPorts(inst)
	result := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		name := port.Name
		if name == "" {
			name = fmt.Sprintf("tcp-%d-%d", port.Port, port.TargetPort)
		}
		result = append(result, corev1.ServicePort{
			Name:       name,
			Protocol:   protocol,
			Port:       port.Port,
			TargetPort: intstr.FromInt(int(port.TargetPort)),
		})
	}
	return result
}

func agentFactoryContainerPorts(inst *privateaiv4.AgentFactory) []corev1.ContainerPort {
	ports := agentFactoryPorts(inst)
	seen := map[int32]bool{}
	result := make([]corev1.ContainerPort, 0, len(ports))
	for _, port := range ports {
		if seen[port.TargetPort] {
			continue
		}
		seen[port.TargetPort] = true
		result = append(result, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.TargetPort,
			Protocol:      port.Protocol,
		})
	}
	return result
}

func agentFactoryPorts(inst *privateaiv4.AgentFactory) []privateaiv4.PaiServicePortSpec {
	if inst.Spec.Networking != nil && inst.Spec.Networking.Service != nil && len(inst.Spec.Networking.Service.Ports) > 0 {
		return inst.Spec.Networking.Service.Ports
	}
	return []privateaiv4.PaiServicePortSpec{{Name: "https", Port: 443, TargetPort: 8080, Protocol: corev1.ProtocolTCP}}
}

func agentFactoryRuntimeEnv(inst *privateaiv4.AgentFactory) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "AGENT_FACTORY_RUNTIME_ENV_PATH", Value: AgentFactoryDefaultConfigMount},
		{Name: "AGENT_FACTORY_RUNTIME_SECRETS_DIR", Value: agentFactorySecretMount(agentFactorySecret(inst))},
	}
	if inst.Spec.Configuration != nil && strings.TrimSpace(inst.Spec.Configuration.NginxBindHost) != "" {
		env = append(env, corev1.EnvVar{Name: "AGENT_FACTORY_NGINX_BIND_HOST", Value: strings.TrimSpace(inst.Spec.Configuration.NginxBindHost)})
	}
	if inst.Spec.Runtime != nil {
		for _, item := range inst.Spec.Runtime.Env {
			env = append(env, corev1.EnvVar{Name: item.Name, Value: item.Value})
		}
	}
	return env
}

func agentFactoryWorkloadAnnotations(inst *privateaiv4.AgentFactory) map[string]string {
	if inst.Spec.Runtime == nil || inst.Spec.Runtime.Annotations == nil {
		return nil
	}
	return filterAgentFactoryUserAnnotations(inst.Spec.Runtime.Annotations.Workload)
}

func agentFactoryPodAnnotations(inst *privateaiv4.AgentFactory) map[string]string {
	annotations := map[string]string{}
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Annotations != nil {
		for k, v := range filterAgentFactoryUserAnnotations(inst.Spec.Runtime.Annotations.Pod) {
			annotations[k] = v
		}
	}
	if inst.Status.RuntimeConfig.ResourceVersion != "" {
		annotations[AgentFactoryConfigRVAnnotation] = inst.Status.RuntimeConfig.ResourceVersion
	}
	if inst.Status.Secret.ResourceVersion != "" {
		annotations[AgentFactorySecretRVAnnotation] = inst.Status.Secret.ResourceVersion
	}
	outboundTLSVersion := inst.Status.OutboundTLS.CABundleResourceVersion
	if outboundTLSVersion != "" {
		annotations[AgentFactoryOutboundTLSRVAnnotation] = outboundTLSVersion
	}
	if len(annotations) == 0 {
		return nil
	}
	return annotations
}

func filterAgentFactoryUserAnnotations(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := map[string]string{}
	for key, value := range input {
		if strings.HasPrefix(key, "agentfactory.oracle.com/") || strings.HasPrefix(key, "privateai.oracle.com/") {
			continue
		}
		output[key] = value
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func agentFactoryImage(inst *privateaiv4.AgentFactory) string {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Image != nil {
		return strings.TrimSpace(inst.Spec.Runtime.Image.Name)
	}
	return ""
}

func agentFactoryImagePullPolicy(inst *privateaiv4.AgentFactory) corev1.PullPolicy {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Image != nil && inst.Spec.Runtime.Image.PullPolicy != "" {
		return inst.Spec.Runtime.Image.PullPolicy
	}
	return corev1.PullIfNotPresent
}

func agentFactoryImagePullSecret(inst *privateaiv4.AgentFactory) string {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Image != nil {
		return strings.TrimSpace(inst.Spec.Runtime.Image.PullSecret)
	}
	return ""
}

func agentFactoryDeploymentStrategy(inst *privateaiv4.AgentFactory) appsv1.DeploymentStrategy {
	strategyType := appsv1.RollingUpdateDeploymentStrategyType
	if inst.Spec.Upgrade != nil && inst.Spec.Upgrade.Strategy == string(appsv1.RecreateDeploymentStrategyType) {
		strategyType = appsv1.RecreateDeploymentStrategyType
	}
	if strategyType == appsv1.RecreateDeploymentStrategyType {
		return appsv1.DeploymentStrategy{Type: strategyType}
	}

	maxSurge := intstr.FromString(AgentFactoryDefaultRollingUpdateMaxSurge)
	maxUnavailable := intstr.FromInt(0)
	if upgrade := inst.Spec.Upgrade; upgrade != nil && upgrade.RollingUpdate != nil {
		if upgrade.RollingUpdate.MaxSurge != nil {
			maxSurge = *upgrade.RollingUpdate.MaxSurge
		}
		if upgrade.RollingUpdate.MaxUnavailable != nil {
			maxUnavailable = *upgrade.RollingUpdate.MaxUnavailable
		}
	}

	return appsv1.DeploymentStrategy{
		Type: strategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxSurge:       &maxSurge,
			MaxUnavailable: &maxUnavailable,
		},
	}
}

func agentFactoryTerminationGracePeriodSeconds(inst *privateaiv4.AgentFactory) int64 {
	if inst.Spec.Upgrade != nil && inst.Spec.Upgrade.TerminationGracePeriodSeconds != nil {
		return *inst.Spec.Upgrade.TerminationGracePeriodSeconds
	}
	return AgentFactoryDefaultTerminationGracePeriodSeconds
}

func agentFactoryReplicas(inst *privateaiv4.AgentFactory) int32 {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Replicas != nil && *inst.Spec.Runtime.Replicas > 0 {
		return *inst.Spec.Runtime.Replicas
	}
	return 1
}

func agentFactoryAutomountServiceAccountToken(inst *privateaiv4.AgentFactory) bool {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.AutomountServiceAccountToken != nil {
		return *inst.Spec.Runtime.AutomountServiceAccountToken
	}
	return false
}

func agentFactoryResources(inst *privateaiv4.AgentFactory) *corev1.ResourceRequirements {
	if inst.Spec.Runtime == nil {
		return nil
	}
	return inst.Spec.Runtime.Resources
}

func agentFactoryWorkerNodes(inst *privateaiv4.AgentFactory) []string {
	if inst.Spec.Runtime == nil {
		return nil
	}
	return inst.Spec.Runtime.WorkerNodes
}

func agentFactoryWorkerNodeSelector(inst *privateaiv4.AgentFactory) map[string]string {
	if inst.Spec.Runtime == nil {
		return nil
	}
	return inst.Spec.Runtime.WorkerNodeSelector
}

func agentFactoryNodeAffinity(inst *privateaiv4.AgentFactory) *corev1.NodeAffinity {
	if selector := agentFactoryWorkerNodeSelector(inst); len(selector) > 0 {
		keys := make([]string, 0, len(selector))
		for key := range selector {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		requirements := make([]corev1.NodeSelectorRequirement, 0, len(keys))
		for _, key := range keys {
			requirements = append(requirements, corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{selector[key]},
			})
		}
		return &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: requirements}},
			},
		}
	}

	if nodes := agentFactoryWorkerNodes(inst); len(nodes) > 0 {
		return &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      corev1.LabelHostname,
						Operator: corev1.NodeSelectorOpIn,
						Values:   append([]string(nil), nodes...),
					}},
				}},
			},
		}
	}
	return nil
}

func agentFactorySecret(inst *privateaiv4.AgentFactory) *privateaiv4.AgentFactorySecretSpec {
	if inst.Spec.Security == nil {
		return nil
	}
	return inst.Spec.Security.Secret
}

func agentFactorySecretMount(secret *privateaiv4.AgentFactorySecretSpec) string {
	if secret != nil && strings.TrimSpace(secret.MountLocation) != "" {
		return strings.TrimSpace(secret.MountLocation)
	}
	return AgentFactoryDefaultSecretMount
}

func agentFactoryClusterServiceEnabled(inst *privateaiv4.AgentFactory) bool {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil || inst.Spec.Networking.Service.Cluster == nil {
		return true
	}
	return boolDefaultTrue(inst.Spec.Networking.Service.Cluster.Enabled)
}

func agentFactoryClusterServiceAnnotations(inst *privateaiv4.AgentFactory) map[string]string {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil || inst.Spec.Networking.Service.Cluster == nil {
		return nil
	}
	return inst.Spec.Networking.Service.Cluster.Annotations
}

func agentFactoryPublicLoadBalancer(inst *privateaiv4.AgentFactory) *privateaiv4.PaiLoadBalancerSpec {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil {
		return nil
	}
	return inst.Spec.Networking.Service.PublicLoadBalancer
}

func agentFactoryPrivateLoadBalancer(inst *privateaiv4.AgentFactory) *privateaiv4.PaiLoadBalancerSpec {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil {
		return nil
	}
	return inst.Spec.Networking.Service.PrivateLoadBalancer
}

func agentFactoryEmptyDir(src *privateaiv4.AgentFactoryEmptyDirSource) *corev1.EmptyDirVolumeSource {
	emptyDir := &corev1.EmptyDirVolumeSource{}
	if src != nil && strings.TrimSpace(src.SizeLimit) != "" {
		quantity := resource.MustParse(src.SizeLimit)
		emptyDir.SizeLimit = &quantity
	}
	return emptyDir
}

func yesNoDefaultTrue(value *bool) string {
	if value == nil || *value {
		return "yes"
	}
	return "no"
}

func boolDefaultTrue(value *bool) bool {
	return value == nil || *value
}

func boolDefaultFalse(value *bool) bool {
	return value != nil && *value
}

func copyAgentFactoryStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		output[key] = input[key]
	}
	return output
}
