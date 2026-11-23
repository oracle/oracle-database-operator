/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package commons

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AgentFactoryDatabaseSetupJobSuffix is appended to an AgentFactory name for the setup Job.
	AgentFactoryDatabaseSetupJobSuffix = "-db-setup"
	// AgentFactoryDatabaseSetupGenerationLabel identifies the setup generation.
	AgentFactoryDatabaseSetupGenerationLabel = "agentfactory.oracle.com/db-setup-generation"
	// AgentFactoryDatabaseSetupScriptHashLabel identifies the setup script content hash.
	AgentFactoryDatabaseSetupScriptHashLabel = "agentfactory.oracle.com/db-setup-script-hash"
	// AgentFactoryDatabaseSetupScriptSuffix is appended to an AgentFactory name for the setup ConfigMap.
	AgentFactoryDatabaseSetupScriptSuffix = "-db-setup-script"
	// AgentFactoryDatabaseSetupScriptKey is the setup script ConfigMap key.
	AgentFactoryDatabaseSetupScriptKey = "paf-db-setup"
	// AgentFactoryDatabaseSetupCommand is the setup container command.
	AgentFactoryDatabaseSetupCommand = "/usr/local/bin/paf-db-setup"
	// AgentFactoryDatabaseSetupRuntimeMount is the runtime Secret mount path.
	AgentFactoryDatabaseSetupRuntimeMount = "/run/secrets/paf-runtime"
	// AgentFactoryDatabaseSetupAdminMount is the bootstrap Secret mount path.
	AgentFactoryDatabaseSetupAdminMount = "/run/secrets/paf-bootstrap"
	// AgentFactoryDatabaseSetupWalletFile is the runtime wallet path.
	AgentFactoryDatabaseSetupWalletFile = AgentFactoryDatabaseSetupRuntimeMount + "/db_wallet.zip"
	// AgentFactoryDatabaseSetupScriptMount is the setup script mount path.
	AgentFactoryDatabaseSetupScriptMount = "/usr/local/bin/paf-db-setup"
)

// AgentFactoryDatabaseSetupJobName returns the stable name of the optional setup Job.
func AgentFactoryDatabaseSetupJobName(inst *privateaiv4.AgentFactory) string {
	return inst.Name + AgentFactoryDatabaseSetupJobSuffix
}

// AgentFactoryDatabaseSetupScriptConfigMapName returns the stable name of the
// ConfigMap containing the temporary controller-provided setup script.
func AgentFactoryDatabaseSetupScriptConfigMapName(inst *privateaiv4.AgentFactory) string {
	return inst.Name + AgentFactoryDatabaseSetupScriptSuffix
}

// AgentFactoryDatabaseSetupScriptHash returns the content hash used to make a
// Job restart when the embedded setup script changes.
func AgentFactoryDatabaseSetupScriptHash() string {
	hash := sha256.Sum256([]byte(AgentFactoryDatabaseSetupScript))
	return fmt.Sprintf("%x", hash[:])
}

// BuildAgentFactoryDatabaseSetupScriptConfigMap builds the temporary
// controller-provided setup script ConfigMap. The script contains no secret
// values; credentials are still supplied through mounted Secret files.
func BuildAgentFactoryDatabaseSetupScriptConfigMap(inst *privateaiv4.AgentFactory) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentFactoryDatabaseSetupScriptConfigMapName(inst),
			Namespace: inst.Namespace,
			Labels:    AgentFactoryLabels(inst),
		},
		Data: map[string]string{
			AgentFactoryDatabaseSetupScriptKey: AgentFactoryDatabaseSetupScript,
		},
	}
}

// BuildAgentFactoryDatabaseSetupJob builds the one-time database setup Job.
// The controller provides the setup command through a ConfigMap for temporary
// testing. The setup image supplies Bash and the Oracle client; Wallet mode
// also requires unzip. The operator passes only file paths for passwords and
// the optional wallet; it never places secret values in the Job command,
// arguments, ConfigMap, or annotations. The administrator connection privilege
// is non-secret and is passed separately.
func BuildAgentFactoryDatabaseSetupJob(inst *privateaiv4.AgentFactory) *batchv1.Job {
	database := inst.Spec.Database
	bootstrap := database.Bootstrap
	admin := bootstrap.AdminCredentials
	adminUsernameKey := strings.TrimSpace(admin.UsernameKey)
	if adminUsernameKey == "" {
		adminUsernameKey = "username"
	}
	adminPasswordKey := strings.TrimSpace(admin.PasswordKey)
	if adminPasswordKey == "" {
		adminPasswordKey = "password"
	}
	adminPrivilege := strings.ToUpper(strings.TrimSpace(admin.Privilege))
	if adminPrivilege == "" {
		adminPrivilege = "NORMAL"
	}

	runtimeSecret := agentFactorySecret(inst)
	runtimePasswordKey := "agent_factory_db_password"
	if runtimeSecret != nil {
		runtimePasswordKey = agentFactorySecretKey(runtimeSecret.DBPasswordKey, runtimePasswordKey)
	}
	isWallet := strings.EqualFold(strings.TrimSpace(database.ConnectionType), "Wallet")
	runtimeSecretItems := []corev1.KeyToPath{{Key: runtimePasswordKey, Path: "password"}}
	if isWallet {
		runtimeSecretItems = append(runtimeSecretItems, corev1.KeyToPath{
			Key:  agentFactorySecretKey(runtimeSecret.WalletZipKey, "db_wallet.zip"),
			Path: "db_wallet.zip",
		})
	}
	image := strings.TrimSpace(bootstrap.Image)
	if image == "" {
		image = agentFactoryImage(inst)
	}
	backoffLimit := int32(3)
	activeDeadlineSeconds := int64(3600)
	ttlSecondsAfterFinished := int32(300)
	preserveFailedJob := bootstrap.PreserveFailedJob
	automountServiceAccountToken := false
	setupScriptMode := int32(0555)
	restartPolicy := corev1.RestartPolicyOnFailure
	if preserveFailedJob {
		// Diagnostic mode is opt-in. Keep one failed Pod available for
		// inspection instead of restarting the container and deleting the
		// finished Job after the normal five-minute TTL.
		backoffLimit = 0
		restartPolicy = corev1.RestartPolicyNever
	}
	setupArgs := []string{
		"--runtime-config", AgentFactoryDefaultConfigMount,
		"--admin-username-file", AgentFactoryDatabaseSetupAdminMount + "/username",
		"--admin-password-file", AgentFactoryDatabaseSetupAdminMount + "/password",
		"--admin-privilege", adminPrivilege,
		"--paf-username", strings.TrimSpace(database.Username),
		"--paf-password-file", AgentFactoryDatabaseSetupRuntimeMount + "/password",
	}
	if isWallet {
		setupArgs = append(setupArgs, "--wallet-file", AgentFactoryDatabaseSetupWalletFile)
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{Kind: "Job", APIVersion: "batch/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentFactoryDatabaseSetupJobName(inst),
			Namespace: inst.Namespace,
			Labels:    AgentFactoryLabels(inst),
			Annotations: map[string]string{
				AgentFactoryDatabaseSetupGenerationLabel: strconv.FormatInt(inst.Generation, 10),
				AgentFactoryDatabaseSetupScriptHashLabel: AgentFactoryDatabaseSetupScriptHash(),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: AgentFactoryLabels(inst)},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &automountServiceAccountToken,
					RestartPolicy:                restartPolicy,
					ImagePullSecrets:             agentFactoryJobImagePullSecrets(inst),
					Containers: []corev1.Container{{
						Name:            "paf-db-setup",
						Image:           image,
						ImagePullPolicy: agentFactoryImagePullPolicy(inst),
						Command:         []string{AgentFactoryDatabaseSetupCommand},
						Args:            setupArgs,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "paf-db-setup-script", MountPath: AgentFactoryDatabaseSetupScriptMount, SubPath: AgentFactoryDatabaseSetupScriptKey, ReadOnly: true},
							{Name: "runtime-config", MountPath: AgentFactoryDefaultConfigMount, SubPath: AgentFactoryRuntimeConfigKey, ReadOnly: true},
							{Name: "paf-bootstrap", MountPath: AgentFactoryDatabaseSetupAdminMount, ReadOnly: true},
							{Name: "paf-runtime", MountPath: AgentFactoryDatabaseSetupRuntimeMount, ReadOnly: true},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "paf-db-setup-script",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: AgentFactoryDatabaseSetupScriptConfigMapName(inst)},
								DefaultMode:          &setupScriptMode,
							}},
						},
						{
							Name: "runtime-config",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: AgentFactoryRuntimeConfigMapName(inst)},
							}},
						},
						{
							Name: "paf-bootstrap",
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
								SecretName: admin.SecretName,
								Items: []corev1.KeyToPath{
									{Key: adminUsernameKey, Path: "username"},
									{Key: adminPasswordKey, Path: "password"},
								},
							}},
						},
						{
							Name: "paf-runtime",
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
								SecretName: runtimeSecret.Name,
								Items:      runtimeSecretItems,
							}},
						},
					},
				},
			},
		},
	}
	if preserveFailedJob {
		job.Spec.TTLSecondsAfterFinished = nil
	}
	return job
}

func agentFactoryJobImagePullSecrets(inst *privateaiv4.AgentFactory) []corev1.LocalObjectReference {
	secret := agentFactoryImagePullSecret(inst)
	if secret == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: secret}}
}
