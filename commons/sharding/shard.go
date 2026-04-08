/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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

package commons

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const standbySqlnetOra = `NAMES.DIRECTORY_PATH=(TNSNAMES,EZCONNECT,HOSTNAME)`
const pmonCheckCmd = `pgrep -fa "ora_pmon_${ORACLE_SID}" >/dev/null`
const shardReplicaCount int32 = 1
const shardServiceTypeLocal = "local"
const shardServiceTypeExternal = "external"
const standbyWalletSecretMountDefault = "/mnt/standby-wallet"

// buildLabelsForShard returns common labels applied to shard resources.
func buildLabelsForShard(instance *databasev4.ShardingDatabase, _ string, _ string) map[string]string {
	// Keep this selector label set stable for backwards compatibility.
	// It is used in StatefulSet selectors, which are immutable after creation.
	return map[string]string{
		"app":      "OracleSharding",
		"type":     "Shard",
		"oralabel": getLabelForShard(instance),
	}
}

func buildOwnerRefForShard(instance *databasev4.ShardingDatabase) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(instance, databasev4.GroupVersion.WithKind("ShardingDatabase")),
	}
}

// buildResourceLabelsForShard adds Kubernetes recommended labels on top of selector labels.
func buildResourceLabelsForShard(instance *databasev4.ShardingDatabase, label string, shardName string) map[string]string {
	labels := buildLabelsForShard(instance, label, shardName)
	labels["app.kubernetes.io/name"] = "oracle-sharding"
	labels["app.kubernetes.io/instance"] = instance.Name
	labels["app.kubernetes.io/component"] = "shard"
	labels["app.kubernetes.io/managed-by"] = "oracle-database-operator"
	labels["app.kubernetes.io/part-of"] = "oracle-database"
	labels["sharding.oracle.com/database"] = instance.Name
	labels["sharding.oracle.com/shard"] = shardName
	if strings.TrimSpace(label) != "" {
		labels["sharding.oracle.com/kind"] = strings.TrimSpace(label)
	}
	return labels
}

// getLabelForShard returns the label value used for grouping shard resources.
func getLabelForShard(instance *databasev4.ShardingDatabase) string {

	//  if len(OraShardSpex.Label) !=0 {
	//     return OraShardSpex.Label
	//   }

	return instance.Name
}

// BuildStatefulSetForShard builds the desired StatefulSet for a shard.
func BuildStatefulSetForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) (*appsv1.StatefulSet, error) {
	spec, err := buildStatefulSpecForShard(instance, OraShardSpex)
	if err != nil {
		return nil, err
	}
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForShard(),
		ObjectMeta: buildObjectMetaForShard(instance, OraShardSpex),
		Spec:       *spec,
	}

	return sfset, nil
}

// buildTypeMetaForShard returns static TypeMeta for StatefulSet.
func buildTypeMetaForShard() metav1.TypeMeta {
	return metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
}

// buildObjectMetaForShard returns metadata for shard StatefulSet resources.
func buildObjectMetaForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            OraShardSpex.Name,
		Namespace:       instance.Namespace,
		OwnerReferences: buildOwnerRefForShard(instance),
		Labels:          buildResourceLabelsForShard(instance, "sharding", OraShardSpex.Name),
	}
}

// buildStatefulSpecForShard constructs the StatefulSet spec for one shard.
func buildStatefulSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) (*appsv1.StatefulSetSpec, error) {
	podSpec, err := buildPodSpecForShard(instance, OraShardSpex)
	if err != nil {
		return nil, err
	}
	replicas := shardReplicaCount
	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraShardSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildResourceLabelsForShard(instance, "sharding", OraShardSpex.Name),
			},
			Spec: *podSpec,
		},
		VolumeClaimTemplates: volumeClaimTemplatesForShard(instance, OraShardSpex),
	}

	sfsetspec.Replicas = &replicas
	return sfsetspec, nil
}

// isStandbyShard reports whether the shard role requires standby bootstrap flow.
func isStandbyShard(role string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(role))
	return normalized == "STANDBY" || normalized == "ACTIVE_STANDBY"
}

// livenessPeriod resolves the configured liveness interval with a safe default.
func livenessPeriod(instance *databasev4.ShardingDatabase) int32 {
	if instance.Spec.LivenessCheckPeriod > 0 {
		return int32(instance.Spec.LivenessCheckPeriod)
	}
	return 60
}

// buildStandbyNetInitContainerForShard builds the standby-only init container that
// prepares Oracle Net files before DB bootstrap.
func buildStandbyNetInitContainerForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) (corev1.Container, bool) {
	if !isStandbyShard(OraShardSpex.DeployAs) {
		return corev1.Container{}, false
	}

	if OraShardSpex.PrimaryDatabaseRef == nil ||
		strings.TrimSpace(OraShardSpex.PrimaryDatabaseRef.Host) == "" ||
		OraShardSpex.PrimaryDatabaseRef.Port == 0 ||
		strings.TrimSpace(OraShardSpex.PrimaryDatabaseRef.CdbName) == "" {
		return corev1.Container{}, false
	}

	privFlag := false
	uid := oraRunAsUser
	gid := oraFsGroup

	primarySid := strings.ToUpper(strings.TrimSpace(OraShardSpex.PrimaryDatabaseRef.CdbName))
	primaryHost := strings.TrimSpace(OraShardSpex.PrimaryDatabaseRef.Host)
	primaryPort := 1521
	if OraShardSpex.PrimaryDatabaseRef.Port > 0 {
		primaryPort = int(OraShardSpex.PrimaryDatabaseRef.Port)
	}

	// ------------------ DGMGRL aliases for broker ------------------
	standbySid := strings.ToUpper(strings.TrimSpace(OraShardSpex.Name)) // e.g. SSHARD2

	primaryDgmgrlSvc := fmt.Sprintf("%s_DGMGRL", primarySid)

	primaryDgmgrlEntry := fmt.Sprintf(`
%s_DGMGRL =
  (DESCRIPTION =
    (ADDRESS = (PROTOCOL = TCP)(HOST = %s)(PORT = %d))
    (CONNECT_DATA =
      (SERVER = DEDICATED)
      (SERVICE_NAME = %s)
    )
  )
`, primarySid, primaryHost, primaryPort, primaryDgmgrlSvc)

	// Standby uses stable ordinal-0 pod DNS
	standbyHost := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", OraShardSpex.Name, OraShardSpex.Name, instance.Namespace)

	standbyDgmgrlSvc := fmt.Sprintf("%s_DGMGRL", standbySid)

	standbyDgmgrlEntry := fmt.Sprintf(`
%s_DGMGRL =
  (DESCRIPTION =
    (ADDRESS = (PROTOCOL = TCP)(HOST = %s)(PORT = 1521))
    (CONNECT_DATA =
      (SERVER = DEDICATED)
      (SERVICE_NAME = %s)
    )
  )
`, standbySid, standbyHost, standbyDgmgrlSvc)
	// ----------------------------------------------------------------------

	// Writes into:
	// 1) /opt/oracle/oradata/dbconfig/<SID>  (SIDB style)
	// 2) $ORACLE_HOME/network/admin          (so DBCA/RMAN can resolve aliases during bootstrap)
	script := `set -euo pipefail

SID_UPPER="$(echo "${ORACLE_SID:-}" | tr '[:lower:]' '[:upper:]')"
if [ -z "${SID_UPPER}" ]; then
  echo "ERROR: ORACLE_SID is empty"; exit 1
fi

# Fallback ORACLE_HOME (19c common). If image exports ORACLE_HOME already, it will be used.
if [ -z "${ORACLE_HOME:-}" ]; then
  export ORACLE_HOME="/u01/app/oracle/product/19c/dbhome_1"
fi

CFG_DIR="` + oraDataMount + `/dbconfig/${SID_UPPER}"
NET_ADMIN="${ORACLE_HOME}/network/admin"

mkdir -p "${CFG_DIR}" "${NET_ADMIN}"

# listener.ora (SIDB constant) -> static services for NOMOUNT + DGMGRL
cat > "${CFG_DIR}/listener.ora" <<EOF
` + dbcommons.ListenerEntry + `
EOF

# tnsnames.ora -> PRIMARY + DGMGRL aliases (PRIMARY + STANDBY)
cat > "${CFG_DIR}/tnsnames.ora" <<EOF
` + dbcommons.PrimaryTnsnamesEntrySharding + `
` + primaryDgmgrlEntry + `
` + standbyDgmgrlEntry + `
EOF

# sqlnet.ora -> allow TNSNAMES + EZCONNECT resolution
cat > "${CFG_DIR}/sqlnet.ora" <<EOF
` + standbySqlnetOra + `
EOF

# Make bootstrap tools read from ORACLE_HOME/network/admin too
ln -sf "${CFG_DIR}/listener.ora" "${NET_ADMIN}/listener.ora"
ln -sf "${CFG_DIR}/tnsnames.ora" "${NET_ADMIN}/tnsnames.ora"
ln -sf "${CFG_DIR}/sqlnet.ora" "${NET_ADMIN}/sqlnet.ora"

chown -R 54321:54321 "${CFG_DIR}" "${NET_ADMIN}" || true
chmod 0644 "${CFG_DIR}/"*.ora || true

echo "[standby-net-init] prepared Oracle Net files in ${CFG_DIR} and ${NET_ADMIN}"
ls -l "${CFG_DIR}" || true
`

	c := corev1.Container{
		Name:  OraShardSpex.Name + "-standby-net-init",
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			AllowPrivilegeEscalation: BoolPointer(false),
			Privileged:               &privFlag,
			RunAsUser:                &uid,
			RunAsGroup:               &gid,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Command:      []string{"/bin/bash", "-lc", script},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),
		Env: []corev1.EnvVar{
			{Name: "ORACLE_SID", Value: strings.ToUpper(OraShardSpex.Name)},
			{Name: "PRIMARY_SID", Value: primarySid},
			{Name: "PRIMARY_IP", Value: primaryHost},
			{Name: "PRIMARY_DB_PORT", Value: strconv.Itoa(primaryPort)},
			{Name: "PRIMARY_DB_CONN_STR", Value: fmt.Sprintf("//%s:%d/%s", primaryHost, primaryPort, primarySid)},
		},
	}

	if OraShardSpex.ImagePulllPolicy != nil {
		c.ImagePullPolicy = *OraShardSpex.ImagePulllPolicy
	}
	return c, true
}

// buildPodSpecForShard builds the PodSpec for shard StatefulSet pods.
func buildPodSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) (*corev1.PodSpec, error) {

	user := oraRunAsUser
	group := oraFsGroup
	podSecurityContext := mergePodSecurityContextWithDefaults(&corev1.PodSecurityContext{
		RunAsNonRoot: BoolPointer(true),
		RunAsUser:    &user,
		RunAsGroup:   &group,
		FSGroup:      &group,
	}, OraShardSpex.SecurityContext)
	podSecurityContext, err := applyOracleMemorySysctls(podSecurityContext, OraShardSpex.Resources, OraShardSpex.EnvVars)
	if err != nil {
		return nil, err
	}
	spec := &corev1.PodSpec{
		SecurityContext:    podSecurityContext,
		HostAliases:        cloneHostAliases(instance.Spec.HostAliases),
		Containers:         buildContainerSpecForShard(instance, OraShardSpex),
		Volumes:            buildVolumeSpecForShard(instance, OraShardSpex),
		ServiceAccountName: instance.Spec.SrvAccountName,
	}

	// Compose init containers in execution order.
	var initList []corev1.Container

	if c, ok := buildStandbyNetInitContainerForShard(instance, OraShardSpex); ok {
		initList = append(initList, c)
	}

	if (instance.Spec.IsDownloadScripts) && (instance.Spec.ScriptsLocation != "") {
		initList = append(initList, buildInitContainerSpecForShard(instance, OraShardSpex)...)
	}

	if len(initList) > 0 {
		spec.InitContainers = initList
	}

	if len(instance.Spec.DbImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: instance.Spec.DbImagePullSecret,
			},
		}
	}

	if len(OraShardSpex.NodeSelector) > 0 {
		spec.NodeSelector = make(map[string]string)
		for key, value := range OraShardSpex.NodeSelector {
			spec.NodeSelector[key] = value
		}
	}

	return spec, nil
}

// buildVolumeSpecForShard returns all volumes mounted by shard containers.
func buildVolumeSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Volume {
	var result []corev1.Volume
	dshmSizeLimit := resource.MustParse("4Gi")
	pvcMounts := normalizePVCMountConfigs(OraShardSpex.Name, OraShardSpex.StorageSizeInGb, instance.Spec.StorageClass, OraShardSpex.DisableDefaultLogVolumeClaims, OraShardSpex.AdditionalPVCs)
	result = []corev1.Volume{
		{
			Name: OraShardSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.DbSecret.Name,
				},
			},
		},
		{
			Name: OraShardSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: &dshmSizeLimit,
				},
			},
		},
	}

	if OraShardSpex.ShardConfigData != nil && len(OraShardSpex.ShardConfigData.Name) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "-oradata-configdata", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: OraShardSpex.ShardConfigData.Name}}}})
	}

	for _, pvcMount := range pvcMounts {
		if strings.TrimSpace(pvcMount.pvcName) == "" {
			continue
		}
		result = append(result, corev1.Volume{
			Name: pvcMount.volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcMount.pvcName},
			},
		})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "orascript-vol5", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}

	if checkTdeWalletFlag(instance) {
		if walletPVC := getTdeWalletPVCName(instance); len(instance.Spec.FssStorageClass) == 0 && walletPVC != "" {
			result = append(result, corev1.Volume{Name: OraShardSpex.Name + "shared-storage-vol8", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: walletPVC}}})
		}
	}
	if standbyWalletSecret := getStandbyWalletSecretRefForShard(instance, OraShardSpex); standbyWalletSecret != "" {
		secretSource := corev1.SecretVolumeSource{SecretName: standbyWalletSecret}
		if zipKey := getStandbyWalletZipFileKeyForShard(instance, OraShardSpex); zipKey != "" {
			secretSource.Items = []corev1.KeyToPath{
				{
					Key:  zipKey,
					Path: "standby-wallet.zip",
				},
			}
		}
		result = append(result, corev1.Volume{
			Name: OraShardSpex.Name + "standby-wallet-vol9",
			VolumeSource: corev1.VolumeSource{
				Secret: &secretSource,
			},
		})
	}

	return result
}

// buildContainerSpecForShard builds the main shard container spec.
func buildContainerSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Container {
	user := oraRunAsUser
	group := oraFsGroup
	capsAdd := []corev1.Capability{corev1.Capability("NET_ADMIN"), corev1.Capability("SYS_NICE")}
	capsDrop := []corev1.Capability{"ALL"}
	// Unified DB readiness/liveness command used for catalog and shard.
	dbCheckCmd := "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "

	containerSpec := corev1.Container{
		Name:  OraShardSpex.Name,
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			RunAsUser:                &user,
			RunAsGroup:               &group,
			AllowPrivilegeEscalation: BoolPointer(false),
			Capabilities: mergeCapabilitiesWithDefaults(&corev1.Capabilities{
				Add:  capsAdd,
				Drop: capsDrop,
			}, OraShardSpex.Capabilities),
		},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),

		// Liveness: simple + reliable (PMON exists).
		LivenessProbe: buildShellExecProbe(
			dbCheckCmd,
			30,
			livenessPeriod(instance),
			30,
			3,
		),

		StartupProbe: buildShellExecProbe(dbCheckCmd, 30, 20, 30, 60),

		ReadinessProbe: buildShellExecProbe(dbCheckCmd, 60, 20, 30, 6),
		// Previously shard probes used pmonCheckCmd:
		// LivenessProbe:  buildShellExecProbe(pmonCheckCmd, 30, livenessPeriod(instance), 10, 3)
		// StartupProbe:   buildShellExecProbe(pmonCheckCmd, 30, 20, 10, 60)
		// ReadinessProbe: buildShellExecProbe(pmonCheckCmd, 60, 20, 10, 6)

		Env: buildEnvVarsSpec(
			instance,
			OraShardSpex.EnvVars,
			OraShardSpex.Name,
			"SHARD",
			false,
			"NONE",
			OraShardSpex.DeployAs,
			OraShardSpex.PrimaryDatabaseRef,
		),
	}
	if standbyWalletSecret := getStandbyWalletSecretRefForShard(instance, OraShardSpex); standbyWalletSecret != "" {
		containerSpec.Env = append(containerSpec.Env,
			corev1.EnvVar{Name: "STANDBY_TDE_WALLET_SECRET", Value: standbyWalletSecret},
			corev1.EnvVar{Name: "STANDBY_TDE_WALLET_MOUNT_PATH", Value: getStandbyWalletMountPathForShard(instance, OraShardSpex)},
			corev1.EnvVar{Name: "STANDBY_TDE_WALLET_ROOT", Value: getStandbyTDEWalletRootForShard(instance, OraShardSpex)},
		)
		if zipKey := getStandbyWalletZipFileKeyForShard(instance, OraShardSpex); zipKey != "" {
			containerSpec.Env = append(containerSpec.Env, corev1.EnvVar{
				Name:  "STANDBY_TDE_WALLET_ZIP_PATH",
				Value: strings.TrimRight(getStandbyWalletMountPathForShard(instance, OraShardSpex), "/") + "/standby-wallet.zip",
			})
		}
	}

	// Preserve shard-level image pull policy override when provided.
	if OraShardSpex.ImagePulllPolicy != nil {
		containerSpec.ImagePullPolicy = *OraShardSpex.ImagePulllPolicy
	}

	// DBCA/RMAN to use our dbconfig tnsnames (standby only)
	if isStandbyShard(OraShardSpex.DeployAs) {

		// Use per-DB dbconfig path
		containerSpec.Env = append(containerSpec.Env,
			corev1.EnvVar{
				Name:  "TNS_ADMIN",
				Value: oraDataMount + "/dbconfig/" + strings.ToUpper(OraShardSpex.Name),
			},
		)

		// IMPORTANT:
		// - /opt/oracle/dbs must exist BEFORE DBCA post steps to avoid standby failing once.
		// - Also avoid "set -u" killing the script if env vars are missing.
		// - Make this idempotent and never fail the container if chown/chmod aren't allowed.
		containerSpec.Command = []string{
			"/bin/bash",
			"-lc",
			`set -eEo pipefail

# ------------------------------------------------------------------------------------
# FIX: DBCA/RMAN expects ORACLE_HOME/dbs to exist (spfile/orapw symlinks).
# In some images /opt/oracle/dbs may not exist -> DBCA post step fails once (DBT-05505 etc).
# Make it idempotent and best-effort (never crash container if perms differ).
# ------------------------------------------------------------------------------------
( mkdir -p /opt/oracle/dbs \
  && chown oracle:oinstall /opt/oracle/dbs 2>/dev/null || true \
  && chmod 775 /opt/oracle/dbs 2>/dev/null || true ) || true

# ------------------------------------------------------------------------------------
# FINAL (plain secret file only):
# Expect secret mounted at ${SECRET_VOLUME}/oracle_pwd (or ${SECRET_VOLUME}/${PASSWORD_FILE})
# Your CR should set:
#   dbSecret.name: db-admin-password
#   dbSecret.dbAdmin.mode: secret-value
#   dbSecret.dbAdmin.passwordKey: oracle_pwd
# so buildEnvVarsSpec sets:
#   SECRET_VOLUME=/mnt/secrets
#   PASSWORD_FILE=oracle_pwd
# ------------------------------------------------------------------------------------

if [ -z "${ORACLE_PWD:-}" ]; then
  if [ -n "${PASSWORD_FILE:-}" ] && [ -f "${SECRET_VOLUME:-/mnt/secrets}/${PASSWORD_FILE}" ]; then
    export ORACLE_PWD="$(base64 -d "${SECRET_VOLUME:-/mnt/secrets}/${PASSWORD_FILE}")"
  elif [ -f "${SECRET_VOLUME:-/mnt/secrets}/oracle_pwd" ]; then
    export ORACLE_PWD="$(base64 -d "${SECRET_VOLUME:-/mnt/secrets}/oracle_pwd")"
  else
    echo "ERROR: password file missing at ${SECRET_VOLUME:-/mnt/secrets}/${PASSWORD_FILE:-oracle_pwd}" >&2
    exit 1
  fi
fi

exec /opt/oracle/runOracle.sh`,
		}
	}

	if instance.Spec.IsClone {
		containerSpec.Command = []string{orainitCmd3}
	}

	if OraShardSpex.Resources != nil {
		containerSpec.Resources = *OraShardSpex.Resources
	}

	return []corev1.Container{containerSpec}
}

// buildInitContainerSpecForShard builds the optional script-download init container.
func buildInitContainerSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Container {
	var result []corev1.Container
	privFlag := true
	// var uid int64 = 0
	uid := oraRunAsUser

	// building the init Container Spec
	var scriptLoc string
	if len(instance.Spec.ScriptsLocation) != 0 {
		scriptLoc = instance.Spec.ScriptsLocation
	} else {
		scriptLoc = "WEB"
	}

	init1spec := corev1.Container{
		Name:  OraShardSpex.Name + "-init1",
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(false),
			AllowPrivilegeEscalation: BoolPointer(true),
			Privileged:               &privFlag,
			RunAsUser:                &uid,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Command: []string{
			"/bin/bash",
			"-c",
			getInitContainerCmd(scriptLoc, instance.Name),
		},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),
	}

	// building Complete Init Container Spec
	if OraShardSpex.ImagePulllPolicy != nil {
		init1spec.ImagePullPolicy = *OraShardSpex.ImagePulllPolicy
	}
	result = []corev1.Container{
		init1spec,
	}
	return result
}

// buildVolumeMountSpecForShard returns volume mounts for shard containers.
func buildVolumeMountSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.VolumeMount {
	result := make([]corev1.VolumeMount, 0, 8)
	pvcMounts := normalizePVCMountConfigs(OraShardSpex.Name, OraShardSpex.StorageSizeInGb, instance.Spec.StorageClass, OraShardSpex.DisableDefaultLogVolumeClaims, OraShardSpex.AdditionalPVCs)
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "secretmap-vol3", MountPath: getDbSecretMountPath(instance), ReadOnly: true})
	for _, pvcMount := range pvcMounts {
		result = append(result, corev1.VolumeMount{Name: pvcMount.volumeName, MountPath: pvcMount.mountPath})
	}
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orascript-vol5", MountPath: oraDbScriptMount})
	}
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if OraShardSpex.ShardConfigData != nil && len(OraShardSpex.ShardConfigData.Name) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "-oradata-configdata", MountPath: OraShardSpex.ShardConfigData.MountPath})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	if checkTdeWalletFlag(instance) {
		walletPVC := getTdeWalletPVCName(instance)
		if len(instance.Spec.FssStorageClass) > 0 && walletPVC == "" {
			if len(instance.Spec.Catalog) > 0 {
				result = append(result, corev1.VolumeMount{
					Name:      instance.Name + "shared-storage" + instance.Spec.Catalog[0].Name + "-0",
					MountPath: getTdeWalletMountLoc(instance),
				})
			}
		} else if len(instance.Spec.FssStorageClass) == 0 && walletPVC != "" {
			result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "shared-storage-vol8", MountPath: getTdeWalletMountLoc(instance)})
		}
	}
	if standbyWalletSecret := getStandbyWalletSecretRefForShard(instance, OraShardSpex); standbyWalletSecret != "" {
		result = append(result, corev1.VolumeMount{
			Name:      OraShardSpex.Name + "standby-wallet-vol9",
			MountPath: getStandbyWalletMountPathForShard(instance, OraShardSpex),
			ReadOnly:  true,
		})
	}

	return result
}

func getStandbyWalletSecretRefForShard(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	deployAs := strings.ToUpper(strings.TrimSpace(shard.DeployAs))
	if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
		return ""
	}

	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if ref := strings.TrimSpace(shard.StandbyConfig.TDEWallet.SecretRef); ref != "" {
				return ref
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if ref := strings.TrimSpace(info.StandbyConfig.TDEWallet.SecretRef); ref != "" {
			return ref
		}
	}
	return ""
}

func getStandbyWalletMountPathForShard(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if p := strings.TrimSpace(shard.StandbyConfig.TDEWallet.MountPath); p != "" {
				return p
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if p := strings.TrimSpace(info.StandbyConfig.TDEWallet.MountPath); p != "" {
			return p
		}
	}
	return standbyWalletSecretMountDefault
}

func getStandbyWalletZipFileKeyForShard(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if k := strings.TrimSpace(shard.StandbyConfig.TDEWallet.ZipFileKey); k != "" {
				return k
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if k := strings.TrimSpace(info.StandbyConfig.TDEWallet.ZipFileKey); k != "" {
			return k
		}
	}
	return ""
}

func getStandbyTDEWalletRootForShard(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if p := strings.TrimSpace(shard.StandbyConfig.TDEWallet.WalletRoot); p != "" {
				return p
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if p := strings.TrimSpace(info.StandbyConfig.TDEWallet.WalletRoot); p != "" {
			return p
		}
	}
	return getTdeWalletMountLoc(instance)
}

func matchShardInfoByLongestPrefix(instance *databasev4.ShardingDatabase, shardName string) *databasev4.ShardingDetails {
	if instance == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(shardName))
	if name == "" {
		return nil
	}
	best := -1
	bestLen := -1
	for i := range instance.Spec.ShardInfo {
		prefix := strings.ToLower(strings.TrimSpace(instance.Spec.ShardInfo[i].ShardPreFixName))
		if prefix == "" || !strings.HasPrefix(name, prefix) {
			continue
		}
		if len(prefix) > bestLen {
			best = i
			bestLen = len(prefix)
		}
	}
	if best < 0 {
		return nil
	}
	return &instance.Spec.ShardInfo[best]
}

// volumeClaimTemplatesForShard returns PVC templates when an existing PVC is not provided.
func volumeClaimTemplatesForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.PersistentVolumeClaim {
	pvcMounts := normalizePVCMountConfigs(OraShardSpex.Name, OraShardSpex.StorageSizeInGb, instance.Spec.StorageClass, OraShardSpex.DisableDefaultLogVolumeClaims, OraShardSpex.AdditionalPVCs)
	claims := make([]corev1.PersistentVolumeClaim, 0, len(pvcMounts))

	for _, pvcMount := range pvcMounts {
		if strings.TrimSpace(pvcMount.pvcName) != "" {
			continue
		}
		claim := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:            pvcMount.volumeName,
				Namespace:       instance.Namespace,
				OwnerReferences: buildOwnerRefForShard(instance),
				Labels:          buildResourceLabelsForShard(instance, "sharding", OraShardSpex.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: storageClassNamePtr(pvcMount.storageClass),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(pvcMount.storageSizeInGb), 10) + "Gi"),
					},
				},
			},
		}

		claims = append(claims, claim)
	}

	return claims
}

// BuildServiceDefForShard builds either the local headless service or per-pod external service.
func BuildServiceDefForShard(instance *databasev4.ShardingDatabase, replicaCount int32, OraShardSpex databasev4.ShardSpec, svctype string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForShard(instance, replicaCount, OraShardSpex, svctype),
		Spec: corev1.ServiceSpec{
			Selector: getSvcLabelsForShard(replicaCount, OraShardSpex),
			Ports:    buildSvcPortsDef(instance, "SHARD"),
		},
	}

	switch svctype {
	case shardServiceTypeExternal:
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
	case shardServiceTypeLocal:
		service.Spec.ClusterIP = corev1.ClusterIPNone
		// publish DNS for NotReady endpoints (needed for DBCA/RMAN duplicate bootstrap)
		service.Spec.PublishNotReadyAddresses = true
	}

	return service
}

// buildSvcObjectMetaForShard returns metadata for shard Services.
func buildSvcObjectMetaForShard(instance *databasev4.ShardingDatabase, replicaCount int32, OraShardSpex databasev4.ShardSpec, svctype string) metav1.ObjectMeta {
	labels := buildResourceLabelsForShard(instance, "sharding", OraShardSpex.Name)
	labels["sharding.oracle.com/service-type"] = svctype
	return metav1.ObjectMeta{
		Name:            shardServiceName(replicaCount, OraShardSpex, svctype),
		Namespace:       instance.Namespace,
		Labels:          labels,
		Annotations:     resolveShardServiceAnnotations(instance, OraShardSpex, svctype),
		OwnerReferences: buildOwnerRefForShard(instance),
	}
}

func shardServiceName(replicaCount int32, OraShardSpex databasev4.ShardSpec, svctype string) string {
	if svctype == shardServiceTypeExternal {
		return OraShardSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}
	return OraShardSpex.Name
}

// getSvcLabelsForShard returns the selector targeting a specific StatefulSet pod.
func getSvcLabelsForShard(replicaCount int32, OraShardSpex databasev4.ShardSpec) map[string]string {
	labelStr := make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10)
	}
	return labelStr
}

// UpdateProvForShard reconciles mutable shard StatefulSet fields and updates if drift is detected.
func UpdateProvForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec, kClient client.Client, sfSet *appsv1.StatefulSet, shardPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {
	_ = shardPod
	requiresUpdate := false

	// Ensure replicas match the shard topology contract.
	if sfSet.Spec.Replicas == nil || *sfSet.Spec.Replicas != shardReplicaCount {
		msg := "Current StatefulSet replicas do not match configured shard replicas. expected=1 current=" +
			func() string {
				if sfSet.Spec.Replicas == nil {
					return "nil"
				}
				return strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
			}()
		LogMessages("DEBUG", msg, nil, instance, logger)
		requiresUpdate = true
	}

	// If explicit resources are provided in spec, compare with StatefulSet template.
	if OraShardSpex.Resources != nil {
		for i := range sfSet.Spec.Template.Spec.Containers {
			if sfSet.Spec.Template.Spec.Containers[i].Name != OraShardSpex.Name {
				continue
			}
			if sfSet.Spec.Template.Spec.Containers[i].Resources.String() != OraShardSpex.Resources.String() {
				requiresUpdate = true
			}
			break
		}
	}

	if requiresUpdate {
		desired, err := BuildStatefulSetForShard(instance, OraShardSpex)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := kClient.Update(context.Background(), desired); err != nil {
			msg := "Failed to update Shard StatefulSet StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, nil, instance, logger)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// ImportTDEKey runs the TDE key import command on the target shard pod.
func ImportTDEKey(podName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger) error {
	_, _, err := ExecCommand(podName, getImportTDEKeyCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error executing getImportTDEKeyCmd : podName=[" + podName + "]. errMsg=" + err.Error()
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}

	importArr := getImportTDEKeyCmd(sparams)
	importCmd := strings.Join(importArr, " ")
	msg := "Executed getImportTDEKeyCmd[" + importCmd + "] on pod " + podName
	LogMessages("INFO", msg, nil, instance, logger)
	return nil
}
