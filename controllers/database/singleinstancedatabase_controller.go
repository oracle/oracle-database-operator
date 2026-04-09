/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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

//nolint:staticcheck,unused,revive // Compatibility paths intentionally keep deprecated fields and optional helpers.
package controllers

// revive:disable:indent-error-flow,var-declaration,exported,context-as-argument
// Existing control flow is kept stable to minimize regression risk.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	dataguardcommon "github.com/oracle/oracle-database-operator/commons/dataguard"
	dgsidb "github.com/oracle/oracle-database-operator/commons/dataguard/sidb"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	sharedresources "github.com/oracle/oracle-database-operator/commons/resources"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SingleInstanceDatabaseReconciler reconciles a SingleInstanceDatabase object.
type SingleInstanceDatabaseReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

type sidbPhaseContext struct {
	singleInstanceDatabase  *dbapi.SingleInstanceDatabase
	cloneFromDatabase       *dbapi.SingleInstanceDatabase
	referredPrimaryDatabase *dbapi.SingleInstanceDatabase
	readyPod                corev1.Pod
	futureRequeue           ctrl.Result
}

// To requeue after 15 secs allowing graceful state changes.
var requeueY ctrl.Result = ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}
var requeueN ctrl.Result = ctrl.Result{}

const singleInstanceDatabaseFinalizer = "database.oracle.com/singleinstancedatabasefinalizer"

// ErrNotPhysicalStandby indicates the database role is not PHYSICAL_STANDBY.
var ErrNotPhysicalStandby error = errors.New("database not in PHYSICAL_STANDBY role")

// ErrDBNotConfiguredWithDG indicates Data Guard is not configured on the database.
var ErrDBNotConfiguredWithDG error = errors.New("database is not configured with a dataguard configuration")

// ErrFSFOEnabledForDGConfig indicates FSFO is enabled for the Data Guard configuration.
var ErrFSFOEnabledForDGConfig error = errors.New("database is configured with dataguard and FSFO enabled")

// ErrAdminPasswordSecretNotFound indicates the admin password secret could not be found.
var ErrAdminPasswordSecretNotFound error = errors.New("Admin password secret for the database not found")

const sidbInitParamUnitBytes = int64(1024 * 1024)

const (
	restoreOCIConfigMountDir       = "/opt/oracle/ocienv/config"
	restoreOCIPrivateKeyMountDir   = "/opt/oracle/oci/keys"
	restoreOPCInstallerMountDir    = "/mnt/software/opcinstaller"
	restoreSourceWalletMountDir    = "/opt/oracle/source-wallet"
	restoreSourceWalletPwdMountDir = "/run/secrets/source-wallet-pwd"
	restoreBackupModuleMountDir    = "/opt/oracle/oci/installer"
	restoreDecryptPwdMountDir      = "/run/secrets/rman-decrypt"
	restoreDefaultDataRoot         = "/opt/oracle/oradata"
	defaultFRAMountPath            = "/opt/oracle/oradata/fast_recovery_area"
)

type sidbOradataPersistenceConfig struct {
	PvcName         string
	Size            string
	StorageClass    string
	AccessMode      string
	DatafilesVolume string
}

type sidbFraPersistenceConfig struct {
	PvcName          string
	Size             string
	StorageClass     string
	AccessMode       string
	MountPath        string
	RecoveryAreaSize string
}

func getOradataPersistenceConfig(m *dbapi.SingleInstanceDatabase) sidbOradataPersistenceConfig {
	cfg := sidbOradataPersistenceConfig{}
	if m == nil {
		return cfg
	}
	if p := m.Spec.Persistence.Oradata; p != nil {
		cfg.PvcName = strings.TrimSpace(p.PvcName)
		cfg.Size = strings.TrimSpace(p.Size)
		cfg.StorageClass = strings.TrimSpace(p.StorageClass)
		cfg.AccessMode = strings.TrimSpace(p.AccessMode)
	} else {
		cfg.Size = strings.TrimSpace(m.Spec.Persistence.Size)
		cfg.StorageClass = strings.TrimSpace(m.Spec.Persistence.StorageClass)
		cfg.AccessMode = strings.TrimSpace(m.Spec.Persistence.AccessMode)
	}
	cfg.DatafilesVolume = strings.TrimSpace(m.Spec.Persistence.DatafilesVolumeName)
	return cfg
}

func hasOradataPersistence(m *dbapi.SingleInstanceDatabase) bool {
	cfg := getOradataPersistenceConfig(m)
	return cfg.PvcName != "" || cfg.Size != ""
}

func isManagedOradataPVC(m *dbapi.SingleInstanceDatabase) bool {
	cfg := getOradataPersistenceConfig(m)
	return cfg.PvcName == "" && cfg.Size != ""
}

func getOradataClaimName(m *dbapi.SingleInstanceDatabase) string {
	cfg := getOradataPersistenceConfig(m)
	if cfg.PvcName != "" {
		return cfg.PvcName
	}
	return m.Name
}

func getFraPersistenceConfig(m *dbapi.SingleInstanceDatabase) sidbFraPersistenceConfig {
	cfg := sidbFraPersistenceConfig{}
	if m == nil || m.Spec.Persistence.Fra == nil {
		return cfg
	}
	cfg.PvcName = strings.TrimSpace(m.Spec.Persistence.Fra.PvcName)
	cfg.Size = strings.TrimSpace(m.Spec.Persistence.Fra.Size)
	cfg.StorageClass = strings.TrimSpace(m.Spec.Persistence.Fra.StorageClass)
	cfg.AccessMode = strings.TrimSpace(m.Spec.Persistence.Fra.AccessMode)
	cfg.MountPath = strings.TrimSpace(m.Spec.Persistence.Fra.MountPath)
	cfg.RecoveryAreaSize = strings.TrimSpace(m.Spec.Persistence.Fra.RecoveryAreaSize)
	return cfg
}

func hasFraPersistence(m *dbapi.SingleInstanceDatabase) bool {
	cfg := getFraPersistenceConfig(m)
	return cfg.PvcName != "" || cfg.Size != ""
}

func isManagedFraPVC(m *dbapi.SingleInstanceDatabase) bool {
	cfg := getFraPersistenceConfig(m)
	return cfg.PvcName == "" && cfg.Size != ""
}

func getFraClaimName(m *dbapi.SingleInstanceDatabase) string {
	cfg := getFraPersistenceConfig(m)
	if cfg.PvcName != "" {
		return cfg.PvcName
	}
	return m.Name + "-fra"
}

func getFraMountPath(m *dbapi.SingleInstanceDatabase) string {
	cfg := getFraPersistenceConfig(m)
	if cfg.MountPath != "" {
		return cfg.MountPath
	}
	return defaultFRAMountPath
}

func getFraRecoveryAreaSize(m *dbapi.SingleInstanceDatabase) string {
	cfg := getFraPersistenceConfig(m)
	if cfg.RecoveryAreaSize != "" {
		return cfg.RecoveryAreaSize
	}
	if cfg.Size != "" {
		return cfg.Size
	}
	return "50G"
}

func buildSetDBRecoveryDestSQL(location, size string) string {
	escapedLocation := strings.ReplaceAll(location, "'", "''")
	escapedSize := strings.ReplaceAll(size, "'", "''")
	return "SHOW PARAMETER db_recovery_file_dest;" +
		"\nALTER SYSTEM SET db_recovery_file_dest_size=" + escapedSize + " scope=both sid='*';" +
		"\nALTER SYSTEM SET db_recovery_file_dest='" + escapedLocation + "' scope=both sid='*';" +
		"\nSHOW PARAMETER db_recovery_file_dest;"
}

func buildSIDBContainerResources(m *dbapi.SingleInstanceDatabase) corev1.ResourceRequirements {
	if m.Spec.ResourceRequirements != nil {
		return *m.Spec.ResourceRequirements.DeepCopy()
	}
	return corev1.ResourceRequirements{}
}

func sidbInitParamBytes(m *dbapi.SingleInstanceDatabase) (int64, int64) {
	if m.Spec.InitParams == nil {
		return 0, 0
	}
	if m.Spec.InitParams.SgaTarget <= 0 || m.Spec.InitParams.PgaAggregateTarget <= 0 {
		return 0, 0
	}
	sgaBytes := int64(m.Spec.InitParams.SgaTarget) * sidbInitParamUnitBytes
	pgaBytes := int64(m.Spec.InitParams.PgaAggregateTarget) * sidbInitParamUnitBytes
	return sgaBytes, pgaBytes
}

func parseSIDBUserOracleSysctlOverrides(sysctls []corev1.Sysctl) (int64, int64) {
	var shmmax int64
	var shmall int64
	for i := range sysctls {
		switch strings.TrimSpace(sysctls[i].Name) {
		case "kernel.shmmax":
			if v, err := sharedresources.ParseBytesOrMemory(sysctls[i].Value); err == nil {
				shmmax = v
			}
		case "kernel.shmall":
			if v, err := strconv.ParseInt(strings.TrimSpace(sysctls[i].Value), 10, 64); err == nil {
				shmall = v
			}
		}
	}
	return shmmax, shmall
}

func sidbHasOracleSysctlHint(sysctls []corev1.Sysctl) bool {
	for i := range sysctls {
		switch strings.TrimSpace(sysctls[i].Name) {
		case "kernel.shmmax", "kernel.shmall", "kernel.sem", "kernel.shmmni":
			return true
		}
	}
	return false
}

func mergeSIDBOracleSysctls(existing, calculated []corev1.Sysctl) []corev1.Sysctl {
	out := make([]corev1.Sysctl, 0, len(existing)+len(calculated))
	indexByName := make(map[string]int, len(existing)+len(calculated))

	for i := range existing {
		name := strings.TrimSpace(existing[i].Name)
		indexByName[name] = len(out)
		out = append(out, existing[i])
	}

	for i := range calculated {
		name := strings.TrimSpace(calculated[i].Name)
		if idx, ok := indexByName[name]; ok {
			if name == "kernel.sem" || name == "kernel.shmmni" {
				continue
			}
			out[idx].Value = calculated[i].Value
			continue
		}
		indexByName[name] = len(out)
		out = append(out, calculated[i])
	}

	return out
}

func getTcpsEnabled(m *dbapi.SingleInstanceDatabase) bool {
	if m.Spec.Security != nil && m.Spec.Security.TCPS != nil && m.Spec.Security.TCPS.Enabled {
		return true
	}
	if m.Spec.TCPS != nil && m.Spec.TCPS.Enabled {
		return true
	}
	return m.Spec.EnableTCPS
}

func getTcpsListenerPort(m *dbapi.SingleInstanceDatabase) int {
	if m.Spec.Security != nil && m.Spec.Security.TCPS != nil && m.Spec.Security.TCPS.ListenerPort != 0 {
		return m.Spec.Security.TCPS.ListenerPort
	}
	if m.Spec.TCPS != nil && m.Spec.TCPS.ListenerPort != 0 {
		return m.Spec.TCPS.ListenerPort
	}
	return m.Spec.TcpsListenerPort
}

func getTcpsTLSSecret(m *dbapi.SingleInstanceDatabase) string {
	if m.Spec.Security != nil && m.Spec.Security.TCPS != nil && strings.TrimSpace(m.Spec.Security.TCPS.TlsSecret) != "" {
		return strings.TrimSpace(m.Spec.Security.TCPS.TlsSecret)
	}
	if m.Spec.TCPS != nil && strings.TrimSpace(m.Spec.TCPS.TlsSecret) != "" {
		return strings.TrimSpace(m.Spec.TCPS.TlsSecret)
	}
	return strings.TrimSpace(m.Spec.TcpsTlsSecret)
}

func getTcpsCertRenewInterval(m *dbapi.SingleInstanceDatabase) string {
	if m.Spec.Security != nil && m.Spec.Security.TCPS != nil && strings.TrimSpace(m.Spec.Security.TCPS.CertRenewInterval) != "" {
		return strings.TrimSpace(m.Spec.Security.TCPS.CertRenewInterval)
	}
	if m.Spec.TCPS != nil && strings.TrimSpace(m.Spec.TCPS.CertRenewInterval) != "" {
		return strings.TrimSpace(m.Spec.TCPS.CertRenewInterval)
	}
	return strings.TrimSpace(m.Spec.TcpsCertRenewInterval)
}

func getTcpsCertsLocation(m *dbapi.SingleInstanceDatabase) string {
	if m.Spec.Security != nil && m.Spec.Security.TCPS != nil {
		custom := strings.TrimSpace(m.Spec.Security.TCPS.CertMountLocation)
		if custom != "" {
			return custom
		}
	}
	if m.Spec.TCPS != nil {
		custom := strings.TrimSpace(m.Spec.TCPS.CertMountLocation)
		if custom != "" {
			return custom
		}
	}

	defaultLocation := dbcommons.TlsCertsLocation
	for _, env := range m.Spec.EnvVars {
		if env.Name == "TCPS_CERTS_LOCATION" {
			custom := strings.TrimSpace(env.Value)
			if custom != "" {
				return custom
			}
		}
	}
	return defaultLocation
}

func buildSIDBPodSecurityContext(
	m *dbapi.SingleInstanceDatabase,
	containerResources corev1.ResourceRequirements,
) (*corev1.PodSecurityContext, error) {
	defaultRunAsUser := int64(dbcommons.ORACLE_UID)
	defaultRunAsGroup := int64(dbcommons.ORACLE_GUID)
	defaultFSGroup := int64(dbcommons.ORACLE_GUID)

	var podSecurityContext *corev1.PodSecurityContext
	if m.Spec.SecurityContext != nil {
		podSecurityContext = m.Spec.SecurityContext.DeepCopy()
	} else {
		podSecurityContext = &corev1.PodSecurityContext{}
	}
	if podSecurityContext.RunAsUser == nil {
		podSecurityContext.RunAsUser = &defaultRunAsUser
	}
	if podSecurityContext.RunAsGroup == nil {
		podSecurityContext.RunAsGroup = &defaultRunAsGroup
	}
	if podSecurityContext.FSGroup == nil {
		podSecurityContext.FSGroup = &defaultFSGroup
	}

	sgaBytes, pgaBytes := sidbInitParamBytes(m)
	memLimit, hugePages := sharedresources.ExtractMemoryAndHugePagesBytes(&containerResources)
	userTuningHint := hugePages > 0 || sidbHasOracleSysctlHint(podSecurityContext.Sysctls)
	if !userTuningHint {
		if err := sharedresources.ValidateSgaPgaSafety(sgaBytes, pgaBytes, memLimit, hugePages, sharedresources.DefaultSafetyPct); err != nil {
			return nil, err
		}
		return podSecurityContext, nil
	}

	userShmmax, userShmall := parseSIDBUserOracleSysctlOverrides(podSecurityContext.Sysctls)
	sysctls, err := sharedresources.CalculateOracleSysctls(
		sgaBytes,
		pgaBytes,
		memLimit,
		hugePages,
		userShmmax,
		userShmall,
	)
	if err != nil {
		return nil, err
	}
	if len(sysctls) > 0 {
		podSecurityContext.Sysctls = mergeSIDBOracleSysctls(podSecurityContext.Sysctls, sysctls)
	}

	return podSecurityContext, nil
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=singleinstancedatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=singleinstancedatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=singleinstancedatabases/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SingleInstanceDatabase object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *SingleInstanceDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	r.Log.Info("Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false

	phaseCtx := &sidbPhaseContext{
		singleInstanceDatabase:  &dbapi.SingleInstanceDatabase{},
		cloneFromDatabase:       &dbapi.SingleInstanceDatabase{},
		referredPrimaryDatabase: &dbapi.SingleInstanceDatabase{},
		futureRequeue:           requeueN,
	}

	// Execute for every reconcile
	defer r.updateReconcileStatus(phaseCtx.singleInstanceDatabase, ctx, &result, &err, &blocked, &completed)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, phaseCtx.singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		return requeueY, err
	}

	if phaseCtx.singleInstanceDatabase.DeletionTimestamp == nil {
		if locked, lockGen, lockMsg := lockpolicy.IsControllerUpdateLocked(
			phaseCtx.singleInstanceDatabase.Status.Conditions,
			lockpolicy.DefaultReconcilingConditionType,
			lockpolicy.DefaultUpdateLockReason,
		); locked {
			if overrideEnabled, _ := lockpolicy.IsUpdateLockOverrideEnabled(
				phaseCtx.singleInstanceDatabase.GetAnnotations(),
				lockpolicy.DefaultOverrideAnnotation,
			); !overrideEnabled {
				blocked = true
				result = ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}
				r.Log.Info(
					"SIDB reconcile blocked by controller update lock",
					"reason", lockpolicy.DefaultUpdateLockReason,
					"observedGeneration", lockGen,
					"message", lockMsg,
				)
				return result, nil
			}
			r.Log.Info("SIDB update lock override accepted", "annotation", lockpolicy.DefaultOverrideAnnotation)
		}
	}

	result, err = r.runSIDBPhase(req, "initialize_status", func() (ctrl.Result, error) {
		return requeueN, r.phaseInitializeStatus(ctx, phaseCtx.singleInstanceDatabase)
	})
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "manage_deletion", func() (ctrl.Result, error) {
		return r.manageSingleInstanceDatabaseDeletion(req, ctx, phaseCtx.singleInstanceDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Error(err, err.Error())
		return result, err
	}

	result, err = r.runSIDBPhase(req, "validate", func() (ctrl.Result, error) {
		return r.validate(
			phaseCtx.singleInstanceDatabase,
			phaseCtx.cloneFromDatabase,
			phaseCtx.referredPrimaryDatabase,
			ctx,
			req,
		)
	})
	if result.Requeue {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "mode_pre_ready", func() (ctrl.Result, error) {
		return r.phaseModePreReady(ctx, req, phaseCtx)
	})
	if result.Requeue {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_datafiles_pvc", func() (ctrl.Result, error) {
		return r.createOrReplacePVCforDatafilesVol(ctx, req, phaseCtx.singleInstanceDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_fra_pvc", func() (ctrl.Result, error) {
		return r.createOrReplacePVCforFRAVol(ctx, req, phaseCtx.singleInstanceDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_customscripts_pvc", func() (ctrl.Result, error) {
		return r.createOrReplacePVCforCustomScriptsVol(ctx, req, phaseCtx.singleInstanceDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "validate_truecache", func() (ctrl.Result, error) {
		sidb := phaseCtx.singleInstanceDatabase
		if sidb.Spec.CreateAs == "truecache" && sidb.Spec.Edition != "enterprise" {
			err := fmt.Errorf("truecache is only supported in enterprise edition, current edition: %s", sidb.Spec.Edition)
			r.Recorder.Eventf(
				sidb,
				corev1.EventTypeWarning,
				"ValidationError",
				"TrueCache is only supported in Enterprise Edition. Current edition: %s",
				sidb.Spec.Edition,
			)
			sidb.Status.Status = dbcommons.StatusError
			meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
				Type:               dbcommons.ReconcileError,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "InvalidEditionForTrueCache",
				Message:            err.Error(),
			})
			return ctrl.Result{}, err
		}
		return requeueN, nil
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "precheck_truecache_blob", func() (ctrl.Result, error) {
		return r.ensureTrueCacheBlobSourceReady(ctx, req, phaseCtx.singleInstanceDatabase, phaseCtx.referredPrimaryDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_pods", func() (ctrl.Result, error) {
		return r.createOrReplacePods(phaseCtx.singleInstanceDatabase, phaseCtx.cloneFromDatabase, phaseCtx.referredPrimaryDatabase, ctx, req)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_service", func() (ctrl.Result, error) {
		return r.createOrReplaceSVC(ctx, req, phaseCtx.singleInstanceDatabase)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "validate_readiness", func() (ctrl.Result, error) {
		res, readyPod, e := r.validateDBReadiness(phaseCtx.singleInstanceDatabase, ctx, req)
		if e != nil {
			return res, e
		}
		phaseCtx.readyPod = readyPod
		return res, nil
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "mode_post_ready", func() (ctrl.Result, error) {
		return r.phaseModePostReady(ctx, req, phaseCtx)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	// manage snapshot database creation
	result, err = r.runSIDBPhase(req, "snapshot_conversion", func() (ctrl.Result, error) {
		if phaseCtx.singleInstanceDatabase.Spec.ConvertToSnapshotStandby != phaseCtx.singleInstanceDatabase.Status.ConvertToSnapshotStandby {
			res, e := r.manageConvPhysicalToSnapshot(ctx, req)
			if e != nil {
				return requeueN, e
			}
			if res.Requeue {
				return requeueY, nil
			}
		}
		return requeueN, nil
	})
	if result.Requeue {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	// Run Datapatch
	result, err = r.runSIDBPhase(req, "run_datapatch", func() (ctrl.Result, error) {
		if strings.ToUpper(phaseCtx.singleInstanceDatabase.Status.Role) == "PRIMARY" && phaseCtx.singleInstanceDatabase.Status.DatafilesPatched != "true" {
			// add a blocking reconcile condition
			e := errors.New("processing datapatch execution")
			blocked = true
			r.updateReconcileStatus(phaseCtx.singleInstanceDatabase, ctx, &result, &e, &blocked, &completed)
			return r.runDatapatch(phaseCtx.singleInstanceDatabase, phaseCtx.readyPod, ctx, req)
		}
		return requeueN, nil
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "mode_status_sync", func() (ctrl.Result, error) {
		return r.phaseModeStatusSync(ctx, req, phaseCtx)
	})
	if result.Requeue {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	result, err = r.runSIDBPhase(req, "ensure_truecache_blob", func() (ctrl.Result, error) {
		sidb := phaseCtx.singleInstanceDatabase
		if sidb.Spec.CreateAs != "primary" || sidb.Spec.TrueCache == nil || !sidb.Spec.TrueCache.GenerateEnabled {
			return requeueN, nil
		}
		if sidb.Spec.Edition != "enterprise" {
			err := fmt.Errorf("TrueCache (generateEnabled=true) is only supported in Enterprise Edition. Current edition: %s", sidb.Spec.Edition)
			r.Recorder.Eventf(sidb, corev1.EventTypeWarning, "ValidationError", err.Error())
			meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
				Type:               "TrueCacheBlobReady",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "InvalidEdition",
				Message:            err.Error(),
			})
			sidb.Status.Status = dbcommons.StatusError
			return ctrl.Result{}, err
		}
		return r.ensureTrueCacheBlob(ctx, req, sidb)
	})
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		return result, err
	}

	completed = true
	r.Log.Info("Reconcile completed")

	return r.runSIDBPhase(req, "schedule_future_requeue", func() (ctrl.Result, error) {
		return r.phaseScheduleFutureRequeue(phaseCtx)
	})
}

func (r *SingleInstanceDatabaseReconciler) phaseLogger(req ctrl.Request, phase string) logr.Logger {
	return r.Log.WithValues("phase", phase, "singleinstancedatabase", req.NamespacedName)
}

func (r *SingleInstanceDatabaseReconciler) runSIDBPhase(req ctrl.Request, phase string, fn func() (ctrl.Result, error)) (ctrl.Result, error) {
	log := r.phaseLogger(req, phase)
	log.Info("Phase started")
	result, err := fn()
	if err != nil {
		log.Error(err, "Phase failed")
		return result, err
	}
	if result.Requeue {
		log.Info("Phase requested requeue", "requeueAfter", result.RequeueAfter)
		return result, nil
	}
	log.Info("Phase completed")
	return result, nil
}

func (r *SingleInstanceDatabaseReconciler) phaseInitializeStatus(ctx context.Context, singleInstanceDatabase *dbapi.SingleInstanceDatabase) error {
	if singleInstanceDatabase.Status.Status != "" {
		return nil
	}

	singleInstanceDatabase.Status.Status = dbcommons.StatusPending
	if singleInstanceDatabase.Spec.Edition != "" {
		singleInstanceDatabase.Status.Edition = cases.Title(language.English).String(singleInstanceDatabase.Spec.Edition)
	} else {
		singleInstanceDatabase.Status.Edition = dbcommons.ValueUnavailable
	}
	singleInstanceDatabase.Status.Role = dbcommons.ValueUnavailable
	singleInstanceDatabase.Status.ConnectString = dbcommons.ValueUnavailable
	singleInstanceDatabase.Status.PdbConnectString = dbcommons.ValueUnavailable
	singleInstanceDatabase.Status.TcpsConnectString = dbcommons.ValueUnavailable
	singleInstanceDatabase.Status.OemExpressUrl = dbcommons.ValueUnavailable
	singleInstanceDatabase.Status.ReleaseUpdate = dbcommons.ValueUnavailable
	return r.Status().Update(ctx, singleInstanceDatabase)
}

func (r *SingleInstanceDatabaseReconciler) phasePostDBReadyOperations(ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	sidb := phaseCtx.singleInstanceDatabase
	readyPod := phaseCtx.readyPod
	referredPrimaryDatabase := phaseCtx.referredPrimaryDatabase

	if sidb.Status.DatafilesCreated == "true" {
		result, err := r.deleteWallet(sidb, ctx, req)
		if result.Requeue || err != nil {
			return result, err
		}
	}

	sidbRole, err := dbcommons.GetDatabaseRole(readyPod, r, r.Config, ctx, req)
	if err != nil {
		return requeueY, err
	}

	if sidbRole == "PRIMARY" {
		result, err := r.updateDBConfig(sidb, readyPod, ctx, req)
		if result.Requeue || err != nil {
			return result, err
		}
		result, err = r.updateInitParameters(sidb, readyPod, ctx, req)
		if result.Requeue || err != nil {
			return result, err
		}
		result, err = r.configTcps(sidb, readyPod, ctx, req, phaseCtx)
		if result.Requeue || err != nil {
			return result, err
		}
		if err := syncConfiguredTNSAliasesInPod(r, sidb, readyPod, ctx, req); err != nil {
			return requeueY, err
		}
		return requeueN, nil
	}

	if sidb.Status.DgBroker == nil {
		err = SetupStandbyDatabase(r, sidb, referredPrimaryDatabase, ctx, req)
		if err != nil {
			return requeueY, err
		}
	}

	databaseOpenMode, err := dbcommons.GetDatabaseOpenMode(readyPod, r, r.Config, ctx, req, sidb.Spec.Edition)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, err
	}
	r.Log.Info("DB openMode Output")
	r.Log.Info(databaseOpenMode)

	if databaseOpenMode == "READ_ONLY" || databaseOpenMode == "MOUNTED" {
		out, cmdErr := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.ModifyStdbyDBOpenMode, dbcommons.SQLPlusCLI))
		if cmdErr != nil {
			r.Log.Error(cmdErr, cmdErr.Error())
			return requeueY, cmdErr
		}
		r.Log.Info("Standby DB open mode modified")
		r.Log.Info(out)
	}

	if err := syncConfiguredTNSAliasesInPod(r, sidb, readyPod, ctx, req); err != nil {
		return requeueY, err
	}

	sidb.Status.PrimaryDatabase = GetPrimaryDatabaseDisplayName(sidb, referredPrimaryDatabase)
	if !IsExternalPrimaryDatabase(sidb) && referredPrimaryDatabase != nil && referredPrimaryDatabase.Name != "" {
		if len(referredPrimaryDatabase.Status.StandbyDatabases) == 0 {
			referredPrimaryDatabase.Status.StandbyDatabases = make(map[string]string)
		}
		referredPrimaryDatabase.Status.StandbyDatabases[strings.ToUpper(sidb.Spec.Sid)] = sidb.Name
		if err := r.Status().Update(ctx, referredPrimaryDatabase); err != nil {
			return requeueY, err
		}
	}

	return requeueN, nil
}

func (r *SingleInstanceDatabaseReconciler) phaseModePreReady(ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	_ = ctx
	mode := strings.ToLower(strings.TrimSpace(phaseCtx.singleInstanceDatabase.Spec.CreateAs))
	switch mode {
	case "", "primary", "clone", "truecache", "standby":
		r.phaseLogger(req, "mode_pre_ready").Info("Mode pre-ready hooks complete", "mode", mode)
		return requeueN, nil
	default:
		// Keep existing behavior unchanged for unknown mode values validated elsewhere.
		r.phaseLogger(req, "mode_pre_ready").Info("Mode pre-ready skipped for unknown mode", "mode", mode)
		return requeueN, nil
	}
}

func (r *SingleInstanceDatabaseReconciler) phaseModePostReady(ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	return r.phasePostDBReadyOperations(ctx, req, phaseCtx)
}

func (r *SingleInstanceDatabaseReconciler) phaseModeStatusSync(ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	result, err := r.phaseConnectStringGate(phaseCtx.singleInstanceDatabase)
	if result.Requeue || err != nil {
		return result, err
	}
	return r.phaseUpdateFinalStatus(ctx, req, phaseCtx)
}

func (r *SingleInstanceDatabaseReconciler) phaseConnectStringGate(sidb *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {
	// Ensure LB-backed services expose a usable connect string before declaring reconcile success.
	if sidb.Status.ConnectString == dbcommons.ValueUnavailable {
		r.Log.Info("Connect string not available for the database " + sidb.Name)
		return requeueY, nil
	}
	return requeueN, nil
}

func (r *SingleInstanceDatabaseReconciler) phaseUpdateFinalStatus(ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	if err := r.updateSidbStatus(phaseCtx.singleInstanceDatabase, phaseCtx.readyPod, ctx, req); err != nil {
		return requeueY, err
	}
	r.updateORDSStatus(phaseCtx.singleInstanceDatabase, ctx, req)
	return requeueN, nil
}

func (r *SingleInstanceDatabaseReconciler) phaseScheduleFutureRequeue(phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	// Scheduling a reconcile for certificate renewal, if TCPS is enabled.
	if phaseCtx.futureRequeue != requeueN {
		r.Log.Info("Scheduling Reconcile for cert renewal", "Duration(Hours)", phaseCtx.futureRequeue.RequeueAfter.Hours())
		copyFutureRequeue := phaseCtx.futureRequeue
		phaseCtx.futureRequeue = requeueN
		return copyFutureRequeue, nil
	}
	return requeueN, nil
}

func (r *SingleInstanceDatabaseReconciler) ensureTrueCacheBlob(
	ctx context.Context,
	req ctrl.Request,
	sidb *dbapi.SingleInstanceDatabase,
) (ctrl.Result, error) {
	if sidb.Spec.CreateAs != "primary" {
		return ctrl.Result{}, nil
	}
	if sidb.Status.Status != dbcommons.StatusReady {
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "WaitingForDatabaseReady",
			Message:            "waiting for primary database to become ready before generating TrueCache blob",
		})
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if !strings.EqualFold(strings.TrimSpace(sidb.Status.ArchiveLog), "true") {
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "WaitingForArchiveLog",
			Message:            "waiting for archive log mode to be enabled before generating TrueCache blob",
		})
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if !HasTDEPasswordSecret(sidb) {
		err := errors.New("spec.security.secrets.tde.secretName and spec.security.secrets.tde.secretKey are required when trueCache.generateEnabled=true")
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "MissingTDEPassword",
			Message:            err.Error(),
		})
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{}, err
	}

	logger := r.Log.WithValues("reconciler", "truecache-blob", "sidb", sidb.Name)
	blobPath := "/tmp/tc_config_blob.tar.gz"
	if sidb.Spec.TrueCache != nil && strings.TrimSpace(sidb.Spec.TrueCache.GeneratePath) != "" {
		blobPath = strings.TrimSpace(sidb.Spec.TrueCache.GeneratePath)
	}

	pods := &corev1.PodList{}
	if err := r.Client.List(ctx, pods, client.InNamespace(sidb.Namespace), client.MatchingLabels{"app": sidb.Name}); err != nil {
		logger.Error(err, "Failed to list pods")
		return ctrl.Result{}, err
	}

	var primaryPod *corev1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Status.Phase == corev1.PodRunning {
			primaryPod = p
			break
		}
	}
	if primaryPod == nil {
		logger.Info("No running primary pod yet")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cmName := sidb.Name + "-truecache-blob"
	blobKey := "tc_config_blob.tar.gz"
	existingCM := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: sidb.Namespace}, existingCM); err == nil {
		if _, ok := existingCM.BinaryData[blobKey]; ok {
			meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
				Type:               "TrueCacheBlobReady",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: sidb.Generation,
				Reason:             "BlobConfigMapExists",
				Message:            "TrueCache config blob ConfigMap already present",
			})
			_ = r.Status().Update(ctx, sidb)
			return ctrl.Result{}, nil
		}
	}

	tdePasswordFilePath := GetTDEPasswordSecretMountPath(sidb)
	cmd := fmt.Sprintf(
		"cat %q | $ORACLE_HOME/bin/dbca -configureDatabase -prepareTrueCacheConfigFile -sourceDB %[2]s -trueCacheBlobLocation %[3]s -tdeWalletPassword \"$(cat %q)\" -silent",
		tdePasswordFilePath,
		sidb.Spec.Sid,
		blobPath,
		tdePasswordFilePath,
	)
	if out, err := dbcommons.ExecCommand(r, r.Config, primaryPod.Name, primaryPod.Namespace, sidb.Name, ctx, req, false, "sh", "-c", cmd); err != nil {
		logger.Error(err, "TrueCache blob DBCA command failed", "pod", primaryPod.Name, "output", strings.TrimSpace(out))
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "BlobCreationFailed",
			Message:            err.Error(),
		})
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	blobContent, err := dbcommons.ExecCommand(r, r.Config, primaryPod.Name, primaryPod.Namespace, sidb.Name, ctx, req, false, "cat", blobPath)
	if err != nil {
		logger.Error(err, "Failed to read generated TrueCache blob", "pod", primaryPod.Name, "path", blobPath)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: sidb.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[blobKey] = []byte(blobContent)
		return ctrl.SetControllerReference(sidb, cm, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to create/update ConfigMap")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	logger.Info("ConfigMap updated/created", "operation", op, "name", cmName)

	meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
		Type:               "TrueCacheBlobReady",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: sidb.Generation,
		Reason:             "BlobStoredInConfigMap",
		Message:            fmt.Sprintf("TrueCache config blob stored in ConfigMap %s", cmName),
	})
	if err := r.Status().Update(ctx, sidb); err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	return ctrl.Result{}, nil
}

func (r *SingleInstanceDatabaseReconciler) ensureTrueCacheBlobSourceReady(
	ctx context.Context,
	req ctrl.Request,
	sidb *dbapi.SingleInstanceDatabase,
	rp *dbapi.SingleInstanceDatabase,
) (ctrl.Result, error) {
	if sidb.Spec.CreateAs != "truecache" {
		return ctrl.Result{}, nil
	}

	logger := r.Log.WithValues("reconciler", "truecache-blob-precheck", "sidb", sidb.Name)
	cmName, blobKey := resolveTrueCacheBlobConfigMap(sidb, rp)
	if cmName == "" {
		err := errors.New("spec.trueCache.blobConfigMapRef is required for truecache when using an external primary database")
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobSourceReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "MissingBlobConfigMapRef",
			Message:            err.Error(),
		})
		sidb.Status.Status = dbcommons.StatusError
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{}, err
	}

	cm := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: sidb.Namespace}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			msg := trueCacheBlobConfigMapWaitingMessage(sidb, rp, cmName)
			logger.Info(msg, "configMap", cmName)
			meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
				Type:               "TrueCacheBlobSourceReady",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: sidb.Generation,
				Reason:             "WaitingForBlobConfigMap",
				Message:            msg,
			})
			sidb.Status.Status = dbcommons.StatusPending
			_ = r.Status().Update(ctx, sidb)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return requeueY, err
	}

	if !configMapContainsKey(cm, blobKey) {
		msg := fmt.Sprintf("waiting for key %q in ConfigMap %s for TrueCache blob", blobKey, cmName)
		logger.Info(msg, "configMap", cmName, "key", blobKey)
		meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
			Type:               "TrueCacheBlobSourceReady",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: sidb.Generation,
			Reason:             "WaitingForBlobConfigMapKey",
			Message:            msg,
		})
		sidb.Status.Status = dbcommons.StatusPending
		_ = r.Status().Update(ctx, sidb)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	meta.SetStatusCondition(&sidb.Status.Conditions, metav1.Condition{
		Type:               "TrueCacheBlobSourceReady",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: sidb.Generation,
		Reason:             "BlobConfigMapReady",
		Message:            fmt.Sprintf("TrueCache blob ConfigMap %s with key %q is ready", cmName, blobKey),
	})
	_ = r.Status().Update(ctx, sidb)
	return ctrl.Result{}, nil
}

func configMapContainsKey(cm *corev1.ConfigMap, key string) bool {
	if cm == nil {
		return false
	}
	if _, ok := cm.BinaryData[key]; ok {
		return true
	}
	_, ok := cm.Data[key]
	return ok
}

func trueCacheBlobConfigMapWaitingMessage(
	m *dbapi.SingleInstanceDatabase,
	rp *dbapi.SingleInstanceDatabase,
	cmName string,
) string {
	if m != nil && m.Spec.TrueCache != nil && strings.TrimSpace(m.Spec.TrueCache.BlobConfigMapRef) != "" {
		return fmt.Sprintf("waiting for user-provided TrueCache blob ConfigMap %s", cmName)
	}
	if rp != nil && rp.Name != "" && rp.Spec.TrueCache != nil && rp.Spec.TrueCache.GenerateEnabled {
		return fmt.Sprintf("waiting for primary %s to generate TrueCache blob ConfigMap %s", rp.Name, cmName)
	}
	if rp != nil && rp.Name != "" {
		return fmt.Sprintf("waiting for TrueCache blob ConfigMap %s; create it manually or enable spec.trueCache.generateEnabled on primary %s", cmName, rp.Name)
	}
	return fmt.Sprintf("waiting for TrueCache blob ConfigMap %s", cmName)
}

// #############################################################################
//
//	Update each reconcile condtion/status
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateReconcileStatus(m *dbapi.SingleInstanceDatabase, ctx context.Context,
	result *ctrl.Result, err *error, blocked *bool, completed *bool) {

	// Always refresh status before a reconcile
	defer func() {
		if updateErr := r.Status().Update(ctx, m); updateErr != nil {
			r.Log.Error(updateErr, "failed to update singleinstancedatabase status")
		}
	}()

	errMsg := func() string {
		if *err != nil {
			return (*err).Error()
		}
		return "no reconcile errors"
	}()
	var condition metav1.Condition
	if *completed {
		condition = metav1.Condition{
			Type:               dbcommons.ReconcileCompelete,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             dbcommons.ReconcileCompleteReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if *blocked {
		condition = metav1.Condition{
			Type:               dbcommons.ReconcileBlocked,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             dbcommons.ReconcileBlockedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if result.Requeue {
		condition = metav1.Condition{
			Type:               dbcommons.ReconcileQueued,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             dbcommons.ReconcileQueuedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if *err != nil {
		condition = metav1.Condition{
			Type:               dbcommons.ReconcileError,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             dbcommons.ReconcileErrorReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else {
		return
	}
	if len(m.Status.Conditions) > 0 {
		meta.RemoveStatusCondition(&m.Status.Conditions, condition.Type)
	}
	meta.SetStatusCondition(&m.Status.Conditions, condition)
}

// #############################################################################
//
//	Validate the CRD specs
//	m = SingleInstanceDatabase
//	n = CloneFromDatabase
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) validate(
	m *dbapi.SingleInstanceDatabase,
	n *dbapi.SingleInstanceDatabase,
	rp *dbapi.SingleInstanceDatabase,
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var err error
	eventReason := "Spec Error"
	var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	// First check image pull secrets
	if m.Spec.Image.PullSecrets != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: m.Spec.Image.PullSecrets, Namespace: m.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueY, err
			}
			r.Log.Error(err, err.Error())
			return requeueY, err
		}
	}

	//  If Express/Free Edition, ensure Replicas=1
	if (m.Spec.Edition == "express" || m.Spec.Edition == "free") && m.Spec.Replicas > 1 {
		eventMsgs = append(eventMsgs, m.Spec.Edition+" edition supports only one replica")
	}
	//  If no persistence, ensure Replicas=1
	if !hasOradataPersistence(m) && m.Spec.Replicas > 1 {
		eventMsgs = append(eventMsgs, "replicas should be 1 if no persistence is specified")
	}
	if m.Status.Sid != "" && !strings.EqualFold(m.Spec.Sid, m.Status.Sid) {
		eventMsgs = append(eventMsgs, "sid cannot be updated")
	}
	if m.Status.Charset != "" && !strings.EqualFold(m.Status.Charset, m.Spec.Charset) {
		eventMsgs = append(eventMsgs, "charset cannot be updated")
	}
	if m.Status.Pdbname != "" && !strings.EqualFold(m.Status.Pdbname, m.Spec.Pdbname) {
		eventMsgs = append(eventMsgs, "pdbName cannot be updated")
	}
	if m.Status.OrdsReference != "" &&
		(m.Status.Persistence.Oradata != nil || m.Status.Persistence.Size != "") &&
		!reflect.DeepEqual(m.Status.Persistence, m.Spec.Persistence) {
		eventMsgs = append(eventMsgs, "uninstall ORDS to change Peristence")
	}
	if len(eventMsgs) > 0 {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, strings.Join(eventMsgs, ","))
		r.Log.Info(strings.Join(eventMsgs, "\n"))
		err = errors.New(strings.Join(eventMsgs, ","))
		return requeueN, err
	}

	// Validating the secret. Pre-built db doesnt need secret
	if !m.Spec.Image.PrebuiltDB && m.Status.DatafilesCreated != "true" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: GetAdminPasswordSecretName(m), Namespace: m.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				m.Status.Status = dbcommons.StatusError
				if updateErr := r.Status().Update(ctx, m); updateErr != nil {
					r.Log.Error(updateErr, "failed to update status after secret not found")
				}
				return requeueY, err
			}
			r.Log.Error(err, "Unable to get the secret. Requeueing..")
			return requeueY, err
		}
	}

	if err = ValidateRestoreSpecRefs(r, m, ctx); err != nil {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
		m.Status.Status = dbcommons.StatusError
		return requeueN, err
	}
	if oradataPVC := strings.TrimSpace(getOradataPersistenceConfig(m).PvcName); oradataPVC != "" {
		pvc := &corev1.PersistentVolumeClaim{}
		if err = r.Get(ctx, types.NamespacedName{Name: oradataPVC, Namespace: m.Namespace}, pvc); err != nil {
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", fmt.Sprintf("persistence.oradata.pvcName %q not found: %v", oradataPVC, err))
			m.Status.Status = dbcommons.StatusError
			return requeueN, err
		}
	}
	if fraPVC := strings.TrimSpace(getFraPersistenceConfig(m).PvcName); fraPVC != "" {
		pvc := &corev1.PersistentVolumeClaim{}
		if err = r.Get(ctx, types.NamespacedName{Name: fraPVC, Namespace: m.Namespace}, pvc); err != nil {
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", fmt.Sprintf("persistence.fra.pvcName %q not found: %v", fraPVC, err))
			m.Status.Status = dbcommons.StatusError
			return requeueN, err
		}
	}

	// update status fields
	m.Status.Sid = m.Spec.Sid
	m.Status.Charset = m.Spec.Charset
	m.Status.Pdbname = m.Spec.Pdbname
	m.Status.Persistence = m.Spec.Persistence
	m.Status.PrebuiltDB = m.Spec.Image.PrebuiltDB
	if m.Spec.CreateAs == "truecache" {
		if m.Spec.PrimaryDatabaseRef == "" && m.Spec.ExternalPrimaryDatabaseRef == nil {
			err := fmt.Errorf("either primaryDatabaseRef or externalPrimaryDatabaseRef must be specified for truecache")
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "SpecError", err.Error())
			m.Status.Status = dbcommons.StatusError
			return requeueN, err
		}
		if m.Spec.PrimaryDatabaseRef != "" && m.Spec.ExternalPrimaryDatabaseRef != nil {
			err := fmt.Errorf("cannot specify both primaryDatabaseRef and externalPrimaryDatabaseRef")
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "SpecError", err.Error())
			m.Status.Status = dbcommons.StatusError
			return requeueN, err
		}
		if IsExternalPrimaryDatabase(m) {
			if err := ValidateExternalPrimaryDatabaseRef(m); err != nil {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueN, err
			}
			m.Status.PrimaryDatabase = GetPrimaryDatabaseDisplayName(m, nil)
		} else {
			err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.PrimaryDatabaseRef}, rp)
			if err != nil {
				if apierrors.IsNotFound(err) {
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
					r.Log.Info(err.Error())
					return requeueN, err
				}
				return requeueY, err
			}
		}
	}
	if m.Spec.CreateAs == "clone" {
		// Once a clone database has created , it has no link with its reference
		if m.Status.DatafilesCreated == "true" || !dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
			return requeueN, nil
		}

		// Fetch the Clone database reference
		err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.PrimaryDatabaseRef}, n)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return requeueN, err
			}
			return requeueY, err
		}

		if n.Status.Status != dbcommons.StatusReady {
			m.Status.Status = dbcommons.StatusPending
			eventReason := "Source Database Pending"
			eventMsg := "status of database " + n.Name + " is not ready, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			err = errors.New(eventMsg)
			return requeueY, err
		}

		if !*n.Spec.ArchiveLog {
			m.Status.Status = dbcommons.StatusPending
			eventReason := "Source Database Check"
			eventMsg := "enable ArchiveLog for database " + n.Name
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info(eventMsg)
			err = errors.New(eventMsg)
			return requeueY, err
		}

		m.Status.Edition = n.Status.Edition
		m.Status.PrimaryDatabase = n.Name
	}

	if m.Spec.CreateAs == "standby" && m.Status.Role != "PRIMARY" {
		primaryRefName := getPrimaryDatabaseRefName(m)

		// External primary standby support
		if IsExternalPrimaryDatabase(m) {
			err = ValidateExternalPrimaryDatabaseRef(m)
			if err != nil {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueN, err
			}

			if strings.EqualFold(m.Spec.Sid, GetPrimaryDatabaseSid(m, nil)) {
				err = fmt.Errorf("standby and external primary database SID can not be same")
				r.Log.Info(err.Error())
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueN, err
			}

			if m.Status.DatafilesCreated == "true" {
				if err = ValidateStandbyWalletSecretRef(r, m, ctx); err != nil {
					r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
					m.Status.Status = dbcommons.StatusError
					return requeueN, err
				}
				return requeueN, nil
			}

			if m.Spec.Edition != "" {
				m.Status.Edition = cases.Title(language.English).String(m.Spec.Edition)
			} else {
				m.Status.Edition = dbcommons.ValueUnavailable
			}

			m.Status.PrimaryDatabase = GetPrimaryDatabaseDisplayName(m, nil)
			if err = ValidateStandbyWalletSecretRef(r, m, ctx); err != nil {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueN, err
			}

			r.Log.Info("Validated external primary database reference for standby creation")
			r.Log.Info("Completed reconcile validation")
			return requeueN, nil
		}

		// Existing local primary standby flow unchanged
		err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: primaryRefName}, rp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return requeueN, err
			}
			return requeueY, err
		}

		if strings.EqualFold(m.Spec.Sid, rp.Spec.Sid) {
			err = fmt.Errorf("standby and primary database SID can not be same")
			r.Log.Info(err.Error())
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
			m.Status.Status = dbcommons.StatusError
			return requeueN, err
		}

		if m.Status.DatafilesCreated == "true" || !dbcommons.IsSourceDatabaseOnCluster(primaryRefName) {
			if err = ValidateStandbyWalletSecretRef(r, m, ctx); err != nil {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueN, err
			}
			return requeueN, nil
		}

		m.Status.Edition = rp.Status.Edition

		err = ValidatePrimaryDatabaseForStandbyCreation(r, m, rp, ctx, req)
		if err != nil {
			return requeueY, err
		}
		if err = ValidateStandbyWalletSecretRef(r, m, ctx); err != nil {
			return requeueY, err
		}

		r.Log.Info("Setting up Primary Database for standby creation...")
		err = SetupPrimaryDatabase(r, m, rp, ctx, req)
		if err != nil {
			return requeueY, err
		}
	}

	r.Log.Info("Completed reconcile validation")
	return requeueN, nil
}

// ValidateRestoreSpecRefs validates restore secret and config references.
func ValidateRestoreSpecRefs(r *SingleInstanceDatabaseReconciler, m *dbapi.SingleInstanceDatabase, ctx context.Context) error {
	restore := getRestoreSpec(m)
	if restore == nil {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(m.Spec.CreateAs))
	if mode != "" && mode != "primary" {
		return fmt.Errorf("spec.restore is supported only when createAs=primary")
	}

	switch getRestoreSourceType(m) {
	case "objectStore":
		if restore.ObjectStore == nil {
			return fmt.Errorf("spec.restore.objectStore is required")
		}
		if restore.FileSystem != nil {
			return fmt.Errorf("spec.restore.fileSystem must not be set with objectStore")
		}
		if err := validateRestoreConfigMapRefExists(r, ctx, m.Namespace, restore.ObjectStore.OCIConfig); err != nil {
			return fmt.Errorf("spec.restore.objectStore.ociConfig: %w", err)
		}
		if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.ObjectStore.PrivateKey); err != nil {
			return fmt.Errorf("spec.restore.objectStore.privateKey: %w", err)
		}
		if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.ObjectStore.SourceDBWallet); err != nil {
			return fmt.Errorf("spec.restore.objectStore.sourceDbWallet: %w", err)
		}
		if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.ObjectStore.SourceDBWalletPw); err != nil {
			return fmt.Errorf("spec.restore.objectStore.sourceDbWalletPassword: %w", err)
		}
		if err := validateRestoreConfigMapRefExists(r, ctx, m.Namespace, restore.ObjectStore.BackupModuleConf); err != nil {
			return fmt.Errorf("spec.restore.objectStore.backupModuleConfig: %w", err)
		}
		if !hasSIDBEnvVarValue(m.Spec.EnvVars, "OPC_INSTALL_ZIP") {
			if err := validateRestoreConfigMapRefExists(r, ctx, m.Namespace, restore.ObjectStore.OpcInstallerZip); err != nil {
				return fmt.Errorf("spec.restore.objectStore.opcInstallerZip: %w", err)
			}
		}
		if restore.ObjectStore.EncryptedBackup != nil && restore.ObjectStore.EncryptedBackup.Enabled {
			if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.ObjectStore.EncryptedBackup.DecryptPasswordSecret); err != nil {
				return fmt.Errorf("spec.restore.objectStore.encryptedBackup.decryptPasswordSecret: %w", err)
			}
		}
	case "fileSystem":
		if restore.FileSystem == nil {
			return fmt.Errorf("spec.restore.fileSystem is required")
		}
		if restore.ObjectStore != nil {
			return fmt.Errorf("spec.restore.objectStore must not be set with fileSystem")
		}
		backupPath := strings.TrimSpace(restore.FileSystem.BackupPath)
		if backupPath == "" {
			return fmt.Errorf("spec.restore.fileSystem.backupPath is required")
		}
		if !isRestoreFSPathVolumeBacked(m, backupPath) {
			return fmt.Errorf("spec.restore.fileSystem.backupPath %q is not under a mounted persistent path; use /opt/oracle/oradata or an additionalPVC with pvcName", backupPath)
		}
		if catalogStartWith := getRestoreCatalogStartWith(m); catalogStartWith != "" && !isRestoreFSPathVolumeBacked(m, catalogStartWith) {
			return fmt.Errorf("spec.restore.fileSystem.catalogStartWith %q is not under a mounted persistent path", catalogStartWith)
		}
		if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.FileSystem.SourceDBWallet); err != nil {
			return fmt.Errorf("spec.restore.fileSystem.sourceDbWallet: %w", err)
		}
		if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.FileSystem.SourceDBWalletPw); err != nil {
			return fmt.Errorf("spec.restore.fileSystem.sourceDbWalletPassword: %w", err)
		}
		if restore.FileSystem.EncryptedBackup != nil && restore.FileSystem.EncryptedBackup.Enabled {
			if err := validateRestoreSecretRefExists(r, ctx, m.Namespace, restore.FileSystem.EncryptedBackup.DecryptPasswordSecret); err != nil {
				return fmt.Errorf("spec.restore.fileSystem.encryptedBackup.decryptPasswordSecret: %w", err)
			}
		}
	case "invalid":
		return fmt.Errorf("spec.restore.objectStore and spec.restore.fileSystem are mutually exclusive")
	default:
		return fmt.Errorf("exactly one of spec.restore.objectStore or spec.restore.fileSystem must be set")
	}

	return nil
}

func validateRestoreSecretRefExists(r *SingleInstanceDatabaseReconciler, ctx context.Context, namespace string, ref *dbapi.SingleInstanceDatabaseSecretKeyRef) error {
	if ref == nil {
		return nil
	}
	secretName := strings.TrimSpace(ref.SecretName)
	secretKey := strings.TrimSpace(ref.Key)
	if secretName == "" || secretKey == "" {
		return fmt.Errorf("secretName and key are required")
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, secret); err != nil {
		return err
	}
	if _, ok := secret.Data[secretKey]; !ok {
		return fmt.Errorf("key %q not found in secret %q", secretKey, secretName)
	}
	return nil
}

func validateRestoreConfigMapRefExists(r *SingleInstanceDatabaseReconciler, ctx context.Context, namespace string, ref *dbapi.SingleInstanceDatabaseConfigMapKeyRef) error {
	if ref == nil {
		return nil
	}
	configMapName := strings.TrimSpace(ref.ConfigMapName)
	configMapKey := strings.TrimSpace(ref.Key)
	if configMapName == "" || configMapKey == "" {
		return fmt.Errorf("configMapName and key are required")
	}
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: configMapName}, configMap); err != nil {
		return err
	}
	if _, ok := configMap.Data[configMapKey]; !ok {
		return fmt.Errorf("key %q not found in configMap %q", configMapKey, configMapName)
	}
	return nil
}

func hasSIDBEnvVarValue(envs []corev1.EnvVar, name string) bool {
	target := strings.TrimSpace(name)
	if target == "" {
		return false
	}
	for i := range envs {
		if strings.TrimSpace(envs[i].Name) != target {
			continue
		}
		if strings.TrimSpace(envs[i].Value) != "" || envs[i].ValueFrom != nil {
			return true
		}
	}
	return false
}

// #############################################################################
//
//	Instantiate POD spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiatePodSpec(m *dbapi.SingleInstanceDatabase, n *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase,
	requiredAffinity bool) (*corev1.Pod, error) {
	walletDir := GetWalletDirFromSid(m.Spec.Sid)
	oradataCfg := getOradataPersistenceConfig(m)
	containerResources := buildSIDBContainerResources(m)
	podSecurityContext, err := buildSIDBPodSecurityContext(m, containerResources)
	if err != nil {
		return nil, err
	}

	// POD spec
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-" + dbcommons.GenerateRandomString(5),
			Namespace: m.Namespace,
			Labels: map[string]string{
				"app":     m.Name,
				"version": m.Spec.Image.Version,
			},
		},
		Spec: corev1.PodSpec{
			Affinity: func() *corev1.Affinity {
				if oradataCfg.AccessMode == "ReadWriteOnce" {
					if requiredAffinity {
						return &corev1.Affinity{
							PodAffinity: &corev1.PodAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{{
											Key:      "app",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{m.Name},
										}},
									},
									TopologyKey: "kubernetes.io/hostname",
								}},
							},
						}
					} else {
						return &corev1.Affinity{
							PodAffinity: &corev1.PodAffinity{
								PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{{
												Key:      "app",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{m.Name},
											}},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								}},
							},
						}
					}
				}
				// For ReadWriteMany Access, spread out the PODs
				return &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
							Weight: 100,
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{m.Name},
									}},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						}},
					},
				}
			}(),
			Volumes: func() []corev1.Volume {
				adminPwdSecretFileName := GetAdminPasswordSecretFileName(m)
				volumes := []corev1.Volume{{
					Name: "datafiles-vol",
					VolumeSource: func() corev1.VolumeSource {
						if !hasOradataPersistence(m) {
							return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
						}
						/* Persistence is specified */
						return corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: getOradataClaimName(m),
								ReadOnly:  false,
							},
						}
					}(),
				}, {
					Name: "oracle-pwd-vol",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: GetAdminPasswordSecretName(m),
							Optional:   func() *bool { i := (m.Spec.Edition != "express" && m.Spec.Edition != "free"); return &i }(),
							Items: []corev1.KeyToPath{{
								Key:  adminPwdSecretFileName,
								Path: adminPwdSecretFileName,
							}},
						},
					},
				}, {
					Name: "tls-secret-vol",
					VolumeSource: func() corev1.VolumeSource {
						tcpsTLSSecret := getTcpsTLSSecret(m)
						if tcpsTLSSecret == "" {
							return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
						}
						/* tls-secret is specified */
						return corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: tcpsTLSSecret,
								Optional:   func() *bool { i := true; return &i }(),
								Items: []corev1.KeyToPath{
									{
										Key:  "tls.crt",  // Mount the certificate
										Path: "cert.crt", // Mount path inside the container
									},
									{
										Key:  "tls.key",    // Mount the private key
										Path: "client.key", // Mount path inside the container
									},
								},
							},
						}
					}(),
				}, {
					Name: "custom-scripts-vol",
					VolumeSource: func() corev1.VolumeSource {
						if m.Spec.Persistence.ScriptsVolumeName == "" || m.Spec.Persistence.ScriptsVolumeName == m.Spec.Persistence.DatafilesVolumeName {
							return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
						}
						/* Persistence.ScriptsVolumeName is specified */
						return corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: m.Name + "-" + m.Spec.Persistence.ScriptsVolumeName,
								ReadOnly:  false,
							},
						}
					}(),
				}}

				if walletSecretName := GetStandbyWalletSecretRef(m); strings.TrimSpace(walletSecretName) != "" && m.Spec.CreateAs == "standby" {
					secretSrc := corev1.SecretVolumeSource{SecretName: walletSecretName}
					if walletZipKey := GetStandbyWalletZipFileKey(m); walletZipKey != "" {
						secretSrc.Items = []corev1.KeyToPath{
							{
								Key:  walletZipKey,
								Path: "standby-wallet.zip",
							},
						}
					}
					volumes = append(volumes, corev1.Volume{
						Name: "standby-wallet-secret-vol",
						VolumeSource: corev1.VolumeSource{
							Secret: &secretSrc,
						},
					})
				}
				if HasTDEPasswordSecret(m) {
					tdeSecretFileName := GetTDEPasswordSecretFileName(m)
					volumes = append(volumes, corev1.Volume{
						Name: "tde-wallet-pwd-vol",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: GetTDEPasswordSecretName(m),
								Items: []corev1.KeyToPath{{
									Key:  tdeSecretFileName,
									Path: tdeSecretFileName,
								}},
							},
						},
					})
				}
				if hasFraPersistence(m) {
					volumes = append(volumes, corev1.Volume{
						Name: "fra-vol",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: getFraClaimName(m),
								ReadOnly:  false,
							},
						},
					})
				}
				for i := range m.Spec.Persistence.AdditionalPVCs {
					pvcName := strings.TrimSpace(m.Spec.Persistence.AdditionalPVCs[i].PvcName)
					if pvcName == "" {
						continue
					}
					volumes = append(volumes, corev1.Volume{
						Name: sidbAdditionalPVCVolumeName(i),
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pvcName,
							},
						},
					})
				}
				if restore := getRestoreSpec(m); restore != nil {
					if restoreUsesObjectStore(m) && restore.ObjectStore != nil {
						if ref := restore.ObjectStore.OCIConfig; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-oci-config-vol",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: strings.TrimSpace(ref.ConfigMapName)},
										Items:                []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.ObjectStore.PrivateKey; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-private-key-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(ref.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.ObjectStore.SourceDBWallet; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-source-wallet-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(ref.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.ObjectStore.SourceDBWalletPw; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-source-wallet-pwd-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(ref.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.ObjectStore.BackupModuleConf; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-backup-module-vol",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: strings.TrimSpace(ref.ConfigMapName)},
										Items:                []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.ObjectStore.OpcInstallerZip; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-opc-installer-vol",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: strings.TrimSpace(ref.ConfigMapName)},
										Items:                []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if enc := restore.ObjectStore.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil &&
							strings.TrimSpace(enc.DecryptPasswordSecret.SecretName) != "" && strings.TrimSpace(enc.DecryptPasswordSecret.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-rman-decrypt-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(enc.DecryptPasswordSecret.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(enc.DecryptPasswordSecret.Key), Path: strings.TrimSpace(enc.DecryptPasswordSecret.Key)}},
									},
								},
							})
						}
					} else if restoreUsesFileSystem(m) && restore.FileSystem != nil {
						if ref := restore.FileSystem.SourceDBWallet; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-source-wallet-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(ref.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if ref := restore.FileSystem.SourceDBWalletPw; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-source-wallet-pwd-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(ref.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(ref.Key), Path: strings.TrimSpace(ref.Key)}},
									},
								},
							})
						}
						if enc := restore.FileSystem.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil &&
							strings.TrimSpace(enc.DecryptPasswordSecret.SecretName) != "" && strings.TrimSpace(enc.DecryptPasswordSecret.Key) != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "restore-rman-decrypt-vol",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: strings.TrimSpace(enc.DecryptPasswordSecret.SecretName),
										Items:      []corev1.KeyToPath{{Key: strings.TrimSpace(enc.DecryptPasswordSecret.Key), Path: strings.TrimSpace(enc.DecryptPasswordSecret.Key)}},
									},
								},
							})
						}
					}
				}

				return volumes
			}(),
			InitContainers: func() []corev1.Container {
				initContainers := []corev1.Container{}
				if hasOradataPersistence(m) && m.Spec.Persistence.SetWritePermissions != nil && *m.Spec.Persistence.SetWritePermissions {
					initContainers = append(initContainers, corev1.Container{
						Name:    "init-permissions",
						Image:   m.Spec.Image.PullFrom,
						Command: []string{"/bin/sh", "-c", fmt.Sprintf("chown %d:%d /opt/oracle/oradata || true", int(dbcommons.ORACLE_UID), int(dbcommons.ORACLE_GUID))},
						SecurityContext: &corev1.SecurityContext{
							// User ID 0 means, root user
							RunAsUser: func() *int64 { i := int64(0); return &i }(),
						},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: "/opt/oracle/oradata",
							Name:      "datafiles-vol",
						}},
					})
				}
				if m.Spec.Image.PrebuiltDB {
					initContainers = append(initContainers, corev1.Container{
						Name:    "init-prebuiltdb",
						Image:   m.Spec.Image.PullFrom,
						Command: []string{"/bin/sh", "-c", dbcommons.InitPrebuiltDbCMD},
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:  func() *int64 { i := int64(dbcommons.ORACLE_UID); return &i }(),
							RunAsGroup: func() *int64 { i := int64(dbcommons.ORACLE_GUID); return &i }(),
						},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: "/mnt/oradata",
							Name:      "datafiles-vol",
						}},
						Env: []corev1.EnvVar{
							{
								Name:  "ORACLE_SID",
								Value: strings.ToUpper(m.Spec.Sid),
							},
						},
					})
				}
				/* Wallet only for edition barring express and free editions, non-prebuiltDB */
				// Run init-wallet for standby as well, so DBCA can consume seeded DB credentials from wallet.
				// Standby TDE wallet import inputs are independent from DB credential wallet seeding.
				if (m.Spec.Edition != "express" && m.Spec.Edition != "free") && !m.Spec.Image.PrebuiltDB && !GetAdminPasswordSkipInitWallet(m) {
					initContainers = append(initContainers, corev1.Container{
						Name:  "init-wallet",
						Image: m.Spec.Image.PullFrom,
						Env: []corev1.EnvVar{
							{
								Name:  "ORACLE_SID",
								Value: strings.ToUpper(m.Spec.Sid),
							},
							{
								Name:  "WALLET_CLI",
								Value: "mkstore",
							},
							{
								Name:  "WALLET_DIR",
								Value: walletDir,
							},
						},
						Command: []string{"/bin/sh"},
						Args: func() []string {
							edition := ""
							if m.Spec.CreateAs != "clone" {
								edition = m.Spec.Edition
								if m.Spec.Edition == "" {
									edition = "enterprise"
								}
							} else {
								if !dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
									edition = m.Spec.Edition
								} else {
									edition = n.Spec.Edition
									if n.Spec.Edition == "" {
										edition = "enterprise"
									}
								}
							}
							return []string{"-c", fmt.Sprintf(dbcommons.InitWalletCMD, edition)}
						}(),
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:  func() *int64 { i := int64(dbcommons.ORACLE_UID); return &i }(),
							RunAsGroup: func() *int64 { i := int64(dbcommons.ORACLE_GUID); return &i }(),
						},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: "/opt/oracle/oradata",
							Name:      "datafiles-vol",
						}},
					})
				}
				return initContainers
			}(),
			Containers: []corev1.Container{{
				Name:  m.Name,
				Image: m.Spec.Image.PullFrom,
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						// Allow priority elevation for DB processes
						Add: []corev1.Capability{"SYS_NICE"},
					},
				},
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{
							Command: func() []string {
								// For patching use cases shutdown immediate is needed especially for standby databases
								shutdownMode := "immediate"
								if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
									// express/free do not support patching
									// To terminate any zombie instances left over due to forced termination
									shutdownMode = "abort"
								}
								return []string{"/bin/sh", "-c", "/bin/echo -en 'shutdown " + shutdownMode + ";\n' | env ORACLE_SID=${ORACLE_SID^^} sqlplus -S / as sysdba"}
							}(),
						},
					},
				},
				Ports: []corev1.ContainerPort{{ContainerPort: dbcommons.CONTAINER_LISTENER_PORT}, {ContainerPort: 5500}},

				ReadinessProbe: func() *corev1.Probe {
					if m.Spec.CreateAs == "primary" {
						return &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "},
								},
							},
							InitialDelaySeconds: 20,
							TimeoutSeconds:      20,
							PeriodSeconds: func() int32 {
								if m.Spec.ReadinessCheckPeriod > 0 {
									return int32(m.Spec.ReadinessCheckPeriod)
								}
								return 60
							}(),
						}
					} else {
						return &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "if [ -f $ORACLE_BASE/oradata/.$ORACLE_SID$CHECKPOINT_FILE_EXTN ]; then if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi else true; fi "},
								},
							},
							InitialDelaySeconds: 0,
							TimeoutSeconds:      20,
							PeriodSeconds: func() int32 {
								if m.Spec.ReadinessCheckPeriod > 0 {
									return int32(m.Spec.ReadinessCheckPeriod)
								}
								return 60
							}(),
						}
					}
				}(),
				VolumeMounts: func() []corev1.VolumeMount {
					adminPwdSecretFileName := GetAdminPasswordSecretFileName(m)
					mounts := []corev1.VolumeMount{}
					if hasOradataPersistence(m) {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: "/opt/oracle/oradata",
							Name:      "datafiles-vol",
						})
					}
					if shouldMountAdminPasswordSecret(m) {
						// mounts pwd as secrets for express edition, prebuilt db, or explicit secret-mount mode
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: GetAdminPasswordSecretMountPath(m),
							ReadOnly:  true,
							Name:      "oracle-pwd-vol",
							SubPath:   adminPwdSecretFileName,
						})
					}
					if getTcpsTLSSecret(m) != "" {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: getTcpsCertsLocation(m),
							ReadOnly:  true,
							Name:      "tls-secret-vol",
						})
					}
					if m.Spec.Persistence.ScriptsVolumeName != "" {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: "/opt/oracle/scripts/startup/",
							ReadOnly:  true,
							Name: func() string {
								if m.Spec.Persistence.ScriptsVolumeName != m.Spec.Persistence.DatafilesVolumeName {
									return "custom-scripts-vol"
								} else {
									return "datafiles-vol"
								}
							}(),
							SubPath: "startup",
						})
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: "/opt/oracle/scripts/setup/",
							ReadOnly:  true,
							Name: func() string {
								if m.Spec.Persistence.ScriptsVolumeName != m.Spec.Persistence.DatafilesVolumeName {
									return "custom-scripts-vol"
								} else {
									return "datafiles-vol"
								}
							}(),
							SubPath: "setup",
						})
					}
					if walletSecretName := GetStandbyWalletSecretRef(m); strings.TrimSpace(walletSecretName) != "" && m.Spec.CreateAs == "standby" {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: GetStandbyWalletMountPath(m),
							ReadOnly:  true,
							Name:      "standby-wallet-secret-vol",
						})
					}
					if HasTDEPasswordSecret(m) {
						tdeSecretFileName := GetTDEPasswordSecretFileName(m)
						mounts = append(mounts, corev1.VolumeMount{
							Name:      "tde-wallet-pwd-vol",
							MountPath: GetTDEPasswordSecretMountPath(m),
							SubPath:   tdeSecretFileName,
							ReadOnly:  true,
						})
					}
					if hasFraPersistence(m) {
						mounts = append(mounts, corev1.VolumeMount{
							Name:      "fra-vol",
							MountPath: getFraMountPath(m),
						})
					}
					for i := range m.Spec.Persistence.AdditionalPVCs {
						mountPath := strings.TrimSpace(m.Spec.Persistence.AdditionalPVCs[i].MountPath)
						pvcName := strings.TrimSpace(m.Spec.Persistence.AdditionalPVCs[i].PvcName)
						if mountPath == "" || pvcName == "" {
							continue
						}
						mounts = append(mounts, corev1.VolumeMount{
							Name:      sidbAdditionalPVCVolumeName(i),
							MountPath: mountPath,
						})
					}
					if restore := getRestoreSpec(m); restore != nil {
						if restoreUsesObjectStore(m) {
							if restore.ObjectStore != nil {
								if ref := restore.ObjectStore.OCIConfig; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-oci-config-vol", MountPath: restoreOCIConfigMountDir, ReadOnly: true})
								}
								if ref := restore.ObjectStore.PrivateKey; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-private-key-vol", MountPath: restoreOCIPrivateKeyMountDir, ReadOnly: true})
								}
								if ref := restore.ObjectStore.SourceDBWallet; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-source-wallet-vol", MountPath: restoreSourceWalletMountDir, ReadOnly: true})
								}
								if ref := restore.ObjectStore.SourceDBWalletPw; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-source-wallet-pwd-vol", MountPath: restoreSourceWalletPwdMountDir, ReadOnly: true})
								}
								if ref := restore.ObjectStore.BackupModuleConf; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-backup-module-vol", MountPath: restoreBackupModuleMountDir, ReadOnly: true})
								}
								if ref := restore.ObjectStore.OpcInstallerZip; ref != nil && strings.TrimSpace(ref.ConfigMapName) != "" && strings.TrimSpace(ref.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-opc-installer-vol", MountPath: restoreOPCInstallerMountDir, ReadOnly: true})
								}
								if enc := restore.ObjectStore.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil &&
									strings.TrimSpace(enc.DecryptPasswordSecret.SecretName) != "" && strings.TrimSpace(enc.DecryptPasswordSecret.Key) != "" {
									mounts = append(mounts, corev1.VolumeMount{Name: "restore-rman-decrypt-vol", MountPath: restoreDecryptPwdMountDir, ReadOnly: true})
								}
							}
						} else if restoreUsesFileSystem(m) && restore.FileSystem != nil {
							if ref := restore.FileSystem.SourceDBWallet; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
								mounts = append(mounts, corev1.VolumeMount{Name: "restore-source-wallet-vol", MountPath: restoreSourceWalletMountDir, ReadOnly: true})
							}
							if ref := restore.FileSystem.SourceDBWalletPw; ref != nil && strings.TrimSpace(ref.SecretName) != "" && strings.TrimSpace(ref.Key) != "" {
								mounts = append(mounts, corev1.VolumeMount{Name: "restore-source-wallet-pwd-vol", MountPath: restoreSourceWalletPwdMountDir, ReadOnly: true})
							}
							if enc := restore.FileSystem.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil &&
								strings.TrimSpace(enc.DecryptPasswordSecret.SecretName) != "" && strings.TrimSpace(enc.DecryptPasswordSecret.Key) != "" {
								mounts = append(mounts, corev1.VolumeMount{Name: "restore-rman-decrypt-vol", MountPath: restoreDecryptPwdMountDir, ReadOnly: true})
							}
						}
					}
					return mounts
				}(),
				Env: func() []corev1.EnvVar {
					adminPwdSecretFileName := GetAdminPasswordSecretFileName(m)
					adminPwdSecretMountRoot := GetAdminPasswordSecretMountRoot(m)
					if m.Spec.CreateAs == "truecache" {
						envs := []corev1.EnvVar{
							{
								Name:  "SVC_HOST",
								Value: m.Name,
							},
							{
								Name:  "SVC_PORT",
								Value: strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)),
							},
							{
								Name:  "ORACLE_SID",
								Value: GetPrimaryDatabaseSid(m, rp),
							},
							{
								Name:  "ORACLE_CHARACTERSET",
								Value: m.Spec.Charset,
							},
							{
								Name:  "SECRETS_BASE_DIR",
								Value: adminPwdSecretMountRoot,
							},
							{
								Name:  "ORACLE_PWD_SECRET_NAME",
								Value: adminPwdSecretFileName,
							},
							{
								Name:  "ORACLE_EDITION",
								Value: m.Spec.Edition,
							},
							{
								Name:  "TRUE_CACHE",
								Value: "true",
							},
							{
								Name:  "AUTO_TRUE_CACHE_SETUP",
								Value: "false",
							},
							{
								Name:  "TRUEDB_UNIQUE_NAME",
								Value: getTrueCacheUniqueName(m),
							},
							{
								Name:  "TRUE_CACHE_BLOB",
								Value: getTrueCacheBlobMountPath(m),
							},
							{
								Name:  "PRIMARY_DB_CONN_STR",
								Value: GetPrimaryDatabaseConnectString(m, rp),
							},
							{
								Name:  "PDB_TC_SVCS",
								Value: strings.Join(getTrueCacheServices(m), ":"),
							},
							{
								Name:  "ORACLE_HOSTNAME",
								Value: fmt.Sprintf("%s.%s.svc.cluster.local", m.Name, m.Namespace),
							},
						}
						if rp != nil && GetAdminPasswordSecretName(rp) != "" && GetAdminPasswordSecretFileName(rp) != "" {
							envs = append(envs, corev1.EnvVar{
								Name: "ORACLE_PWD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: GetAdminPasswordSecretName(rp)},
										Key:                  GetAdminPasswordSecretFileName(rp),
									},
								},
							})
						} else if GetAdminPasswordSecretName(m) != "" && GetAdminPasswordSecretFileName(m) != "" {
							envs = append(envs, corev1.EnvVar{
								Name: "ORACLE_PWD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: GetAdminPasswordSecretName(m)},
										Key:                  GetAdminPasswordSecretFileName(m),
									},
								},
							})
						}
						if HasTDEPasswordSecret(m) {
							envs = append(envs, corev1.EnvVar{
								Name: "TDE_WALLET_PWD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: GetTDEPasswordSecretName(m)},
										Key:                  GetTDEPasswordSecretFileName(m),
									},
								},
							})
						}
						return mergeSIDBEnvVarsWithSecurity(m, envs)
					}
					// adding XE support, useful for dev/test/CI-CD
					if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
						return mergeSIDBEnvVarsWithSecurity(m, []corev1.EnvVar{
							{
								Name:  "SVC_HOST",
								Value: m.Name,
							},
							{
								Name:  "SVC_PORT",
								Value: strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)),
							},
							{
								Name:  "ORACLE_CHARACTERSET",
								Value: m.Spec.Charset,
							},
							{
								Name:  "SECRETS_BASE_DIR",
								Value: adminPwdSecretMountRoot,
							},
							{
								Name:  "ORACLE_PWD_SECRET_NAME",
								Value: adminPwdSecretFileName,
							},
							{
								Name:  "ORACLE_EDITION",
								Value: m.Spec.Edition,
							},
						})
					}
					if m.Spec.CreateAs == "clone" {
						// Clone DB use-case
						return mergeSIDBEnvVarsWithSecurity(m, []corev1.EnvVar{
							{
								Name:  "SVC_HOST",
								Value: m.Name,
							},
							{
								Name:  "SVC_PORT",
								Value: strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)),
							},
							{
								Name:  "ORACLE_SID",
								Value: strings.ToUpper(m.Spec.Sid),
							},
							{
								Name:  "WALLET_DIR",
								Value: walletDir,
							},
							{
								Name:  "SECRETS_BASE_DIR",
								Value: adminPwdSecretMountRoot,
							},
							{
								Name:  "ORACLE_PWD_SECRET_NAME",
								Value: adminPwdSecretFileName,
							},
							{
								Name: "PRIMARY_DB_CONN_STR",
								Value: func() string {
									if dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
										return n.Name + ":" + strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)) + "/" + n.Spec.Sid
									}
									return m.Spec.PrimaryDatabaseRef
								}(),
							},
							CreateOracleHostnameEnvVarObj(m, n),
							{
								Name:  "CLONE_DB",
								Value: "true",
							},
							{
								Name:  "SKIP_DATAPATCH",
								Value: "true",
							},
						})

					} else if m.Spec.CreateAs == "standby" {
						standbyEnv := []corev1.EnvVar{
							{
								Name:  "SVC_HOST",
								Value: m.Name,
							},
							{
								Name:  "SVC_PORT",
								Value: strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)),
							},
							{
								Name:  "ORACLE_SID",
								Value: strings.ToUpper(m.Spec.Sid),
							},
							{
								Name:  "WALLET_DIR",
								Value: walletDir,
							},
							{
								Name:  "SECRETS_BASE_DIR",
								Value: adminPwdSecretMountRoot,
							},
							{
								Name:  "ORACLE_PWD_SECRET_NAME",
								Value: adminPwdSecretFileName,
							},
							{
								Name:  "PRIMARY_DB_CONN_STR",
								Value: GetPrimaryDatabaseConnectString(m, rp),
							},
							{
								Name:  "PRIMARY_SID",
								Value: GetPrimaryDatabaseSid(m, rp),
							},
							{
								Name:  "PRIMARY_IP",
								Value: GetPrimaryDatabaseHost(m, rp),
							},
							{
								Name:  "PRIMARY_DB_PORT",
								Value: strconv.Itoa(GetPrimaryDatabasePort(m)),
							},
							{
								Name:  "CREATE_PDB",
								Value: ShouldCreatePDBFromPrimary(m, rp),
							},
							{
								Name: "ORACLE_HOSTNAME",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "status.podIP",
									},
								},
							},
							{
								Name:  "STANDBY_DB",
								Value: "true",
							},
							{
								Name:  "SKIP_DATAPATCH",
								Value: "true",
							},
						}
						if walletSecretName := GetStandbyWalletSecretRef(m); walletSecretName != "" {
							standbyEnv = append(standbyEnv,
								corev1.EnvVar{Name: "STANDBY_TDE_WALLET_SECRET", Value: walletSecretName},
								corev1.EnvVar{Name: "STANDBY_TDE_WALLET_MOUNT_PATH", Value: GetStandbyWalletMountPath(m)},
								corev1.EnvVar{Name: "STANDBY_TDE_WALLET_ROOT", Value: GetStandbyTDEWalletRoot(m)},
							)
							if zipKey := GetStandbyWalletZipFileKey(m); zipKey != "" {
								standbyEnv = append(standbyEnv, corev1.EnvVar{
									Name:  "STANDBY_TDE_WALLET_ZIP_PATH",
									Value: strings.TrimRight(GetStandbyWalletMountPath(m), "/") + "/standby-wallet.zip",
								})
							}
						}
						return mergeSIDBEnvVarsWithSecurity(m, standbyEnv)
					}

					return mergeSIDBEnvVarsWithSecurity(m, []corev1.EnvVar{
						{
							Name:  "SVC_HOST",
							Value: m.Name,
						},
						{
							Name:  "SVC_PORT",
							Value: strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)),
						},
						{
							Name: "CREATE_PDB",
							Value: func() string {
								if m.Spec.Pdbname != "" {
									return "true"
								}
								return "false"
							}(),
						},
						{
							Name:  "ORACLE_SID",
							Value: strings.ToUpper(m.Spec.Sid),
						},
						{
							Name: "WALLET_DIR",
							Value: func() string {
								if m.Spec.Image.PrebuiltDB {
									return "" // No wallets for prebuilt DB
								}
								return walletDir
							}(),
						},
						{
							Name:  "SECRETS_BASE_DIR",
							Value: adminPwdSecretMountRoot,
						},
						{
							Name:  "ORACLE_PWD_SECRET_NAME",
							Value: adminPwdSecretFileName,
						},
						{
							Name:  "ORACLE_PDB",
							Value: m.Spec.Pdbname,
						},
						{
							Name:  "ORACLE_CHARACTERSET",
							Value: m.Spec.Charset,
						},
						{
							Name:  "ORACLE_EDITION",
							Value: m.Spec.Edition,
						},
						{
							Name: "INIT_SGA_SIZE",
							Value: func() string {
								if m.Spec.InitParams != nil && m.Spec.InitParams.SgaTarget > 0 && m.Spec.InitParams.PgaAggregateTarget > 0 {
									return strconv.Itoa(m.Spec.InitParams.SgaTarget)
								}
								return ""
							}(),
						},
						{
							Name: "INIT_PGA_SIZE",
							Value: func() string {
								if m.Spec.InitParams != nil && m.Spec.InitParams.SgaTarget > 0 && m.Spec.InitParams.PgaAggregateTarget > 0 {
									return strconv.Itoa(m.Spec.InitParams.PgaAggregateTarget)
								}
								return ""
							}(),
						},
						{
							Name:  "SKIP_DATAPATCH",
							Value: "true",
						},
					})

				}(),

				Resources: containerResources,
			}},

			TerminationGracePeriodSeconds: func() *int64 { i := int64(30); return &i }(),

			NodeSelector: func() map[string]string {
				ns := make(map[string]string)
				if len(m.Spec.NodeSelector) != 0 {
					for key, value := range m.Spec.NodeSelector {
						ns[key] = value
					}
				}
				return ns
			}(),
			HostAliases: func() []corev1.HostAlias {
				if len(m.Spec.HostAliases) == 0 {
					return nil
				}
				hostAliases := make([]corev1.HostAlias, len(m.Spec.HostAliases))
				copy(hostAliases, m.Spec.HostAliases)
				return hostAliases
			}(),

			SecurityContext: podSecurityContext,
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: m.Spec.Image.PullSecrets,
				},
			},
			ServiceAccountName: m.Spec.ServiceAccountName,
		},
	}

	// Adding pod anti-affinity for standby cases
	if m.Spec.CreateAs == "standby" && !IsExternalPrimaryDatabase(m) && rp != nil && rp.Name != "" {
		weightedPodAffinityTerm := corev1.WeightedPodAffinityTerm{
			Weight: 100,
			PodAffinityTerm: corev1.PodAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{rp.Name},
					}},
				},
				TopologyKey: "kubernetes.io/hostname",
			},
		}
		if getOradataPersistenceConfig(m).AccessMode == "ReadWriteOnce" {
			pod.Spec.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					weightedPodAffinityTerm,
				},
			}
		} else {
			pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution =
				append(pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution, weightedPodAffinityTerm)
		}
	}

	r.addTrueCacheBlobVolumeMount(pod, m, rp)
	// Set SingleInstanceDatabase instance as the owner and controller
	if err := ctrl.SetControllerReference(m, pod, r.Scheme); err != nil {
		return nil, err
	}
	return pod, nil

}

// #############################################################################
//
//	Instantiate Service spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiateSVCSpec(m *dbapi.SingleInstanceDatabase,
	svcName string, ports []corev1.ServicePort, svcType corev1.ServiceType, publishNotReadyAddress bool) *corev1.Service {
	svc := dbcommons.NewRealServiceBuilder().
		SetName(svcName).
		SetNamespace(m.Namespace).
		SetLabels(func() map[string]string {
			return map[string]string{
				"app": m.Name,
			}
		}()).
		SetAnnotation(func() map[string]string {
			annotations := make(map[string]string)
			if len(m.Spec.ServiceAnnotations) != 0 {
				for key, value := range m.Spec.ServiceAnnotations {
					annotations[key] = value
				}
			}
			return annotations
		}()).
		SetPorts(ports).
		SetSelector(func() map[string]string {
			return map[string]string{
				"app": m.Name,
			}
		}()).
		SetPublishNotReadyAddresses(publishNotReadyAddress).
		SetType(svcType).
		Build()
	_ = ctrl.SetControllerReference(m, &svc, r.Scheme)
	return &svc
}

// #############################################################################
//
//	Instantiate Persistent Volume Claim spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiatePVCSpec(m *dbapi.SingleInstanceDatabase) *corev1.PersistentVolumeClaim {
	oradataCfg := getOradataPersistenceConfig(m)

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels: map[string]string{
				"app": m.Name,
			},
			Annotations: func() map[string]string {
				if m.Spec.Persistence.VolumeClaimAnnotation != "" {
					strParts := strings.SplitN(m.Spec.Persistence.VolumeClaimAnnotation, ":", 2)
					if len(strParts) != 2 || strings.TrimSpace(strParts[0]) == "" || strings.TrimSpace(strParts[1]) == "" {
						r.Log.Info("Ignoring malformed persistence.volumeClaimAnnotation; expected <key>:<value>", "value", m.Spec.Persistence.VolumeClaimAnnotation, "sidb", m.Name)
						return nil
					}
					annotationMap := make(map[string]string)
					annotationMap[strings.TrimSpace(strParts[0])] = strings.TrimSpace(strParts[1])
					return annotationMap
				}
				return nil
			}(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: func() []corev1.PersistentVolumeAccessMode {
				var accessMode []corev1.PersistentVolumeAccessMode
				accessMode = append(accessMode, corev1.PersistentVolumeAccessMode(oradataCfg.AccessMode))
				return accessMode
			}(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					// Requests describes the minimum amount of compute resources required
					"storage": resource.MustParse(oradataCfg.Size),
				},
			},
			StorageClassName: &oradataCfg.StorageClass,
			VolumeName:       oradataCfg.DatafilesVolume,
			Selector: func() *metav1.LabelSelector {
				if oradataCfg.StorageClass != "oci" {
					return nil
				}
				return &metav1.LabelSelector{
					MatchLabels: func() map[string]string {
						ns := make(map[string]string)
						if len(m.Spec.NodeSelector) != 0 {
							for key, value := range m.Spec.NodeSelector {
								ns[key] = value
							}
						}
						return ns
					}(),
				}
			}(),
		},
	}
	// Set SingleInstanceDatabase instance as the owner and controller
	_ = ctrl.SetControllerReference(m, pvc, r.Scheme)
	return pvc
}

func (r *SingleInstanceDatabaseReconciler) instantiateFRAPVCSpec(m *dbapi.SingleInstanceDatabase) *corev1.PersistentVolumeClaim {
	fraCfg := getFraPersistenceConfig(m)

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getFraClaimName(m),
			Namespace: m.Namespace,
			Labels: map[string]string{
				"app": m.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: func() []corev1.PersistentVolumeAccessMode {
				var accessMode []corev1.PersistentVolumeAccessMode
				accessMode = append(accessMode, corev1.PersistentVolumeAccessMode(fraCfg.AccessMode))
				return accessMode
			}(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse(fraCfg.Size),
				},
			},
			StorageClassName: &fraCfg.StorageClass,
		},
	}
	_ = ctrl.SetControllerReference(m, pvc, r.Scheme)
	return pvc
}

// #############################################################################
//
//	Stake a claim for Persistent Volume for customScript Volume
//
// #############################################################################

func (r *SingleInstanceDatabaseReconciler) createOrReplacePVCforCustomScriptsVol(ctx context.Context, req ctrl.Request,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {

	log := r.Log.WithValues("createPVC CustomScripts Vol", req.NamespacedName)

	// if customScriptsVolumeName is not present or it is same than DatafilesVolumeName
	if m.Spec.Persistence.ScriptsVolumeName == "" || m.Spec.Persistence.ScriptsVolumeName == m.Spec.Persistence.DatafilesVolumeName {
		return requeueN, nil
	}

	pvcDeleted := false
	pvcName := string(m.Name) + "-" + string(m.Spec.Persistence.ScriptsVolumeName)
	// Check if the PVC already exists using r.Get, if not create a new one using r.Create
	pvc := &corev1.PersistentVolumeClaim{}
	// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: m.Namespace}, pvc)

	if err == nil {
		if m.Spec.Persistence.ScriptsVolumeName != "" && pvc.Spec.VolumeName != m.Spec.Persistence.ScriptsVolumeName {
			// call deletePods() with zero pods in avaiable and nil readyPod to delete all pods
			result, err := r.deletePods(ctx, req, m, []corev1.Pod{}, corev1.Pod{}, 0, 0)
			if result.Requeue {
				return result, err
			}

			log.Info("Deleting PVC", " name ", pvc.Name)
			err = r.Delete(ctx, pvc)
			if err != nil {
				r.Log.Error(err, "Failed to delete Pvc", "Pvc.Name", pvc.Name)
				return requeueN, err
			}
			pvcDeleted = true
		} else {
			log.Info("Found Existing PVC", "Name", pvc.Name)
			return requeueN, nil
		}
	}
	if pvcDeleted || err != nil && apierrors.IsNotFound(err) {
		// Define a new PVC

		// get accessMode and storage of pv mentioned to be used in pvc spec
		pv := &corev1.PersistentVolume{}
		pvName := m.Spec.Persistence.ScriptsVolumeName
		// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
		pvErr := r.Get(ctx, types.NamespacedName{Name: pvName, Namespace: m.Namespace}, pv)
		if pvErr != nil {
			log.Error(pvErr, "Failed to get PV")
			return requeueY, pvErr
		}

		volumeQty := pv.Spec.Capacity[corev1.ResourceStorage]

		AccessMode := pv.Spec.AccessModes[0]
		Storage := int(volumeQty.Value())
		StorageClass := ""

		log.Info(fmt.Sprintf("PV storage: %v\n", Storage))
		log.Info(fmt.Sprintf("PV AccessMode: %v\n", AccessMode))

		pvc := &corev1.PersistentVolumeClaim{
			TypeMeta: metav1.TypeMeta{
				Kind: "PersistentVolumeClaim",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: m.Namespace,
				Labels: map[string]string{
					"app": m.Name,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: func() []corev1.PersistentVolumeAccessMode {
					var accessMode []corev1.PersistentVolumeAccessMode
					accessMode = append(accessMode, corev1.PersistentVolumeAccessMode(AccessMode))
					return accessMode
				}(),
				Resources: corev1.VolumeResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{
						// Requests describes the minimum amount of compute resources required
						"storage": *resource.NewQuantity(int64(Storage), resource.BinarySI),
					},
				},
				StorageClassName: &StorageClass,
				VolumeName:       pvName,
			},
		}

		// Set SingleInstanceDatabase instance as the owner and controller
		_ = ctrl.SetControllerReference(m, pvc, r.Scheme)

		log.Info("Creating a new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
		err = r.Create(ctx, pvc)
		if err != nil {
			log.Error(err, "Failed to create new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
			return requeueY, err
		}
		return requeueN, nil
	} else if err != nil {
		log.Error(err, "Failed to get PVC")
		return requeueY, err
	}

	return requeueN, nil
}

// #############################################################################
//
//	Stake a claim for Persistent Volume for Datafiles Volume
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) createOrReplacePVCforDatafilesVol(ctx context.Context, req ctrl.Request,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {

	log := r.Log.WithValues("createPVC Datafiles-Vol", req.NamespacedName)
	oradataCfg := getOradataPersistenceConfig(m)

	// Don't create PVC if persistence is not chosen
	if oradataCfg.PvcName != "" || oradataCfg.Size == "" {
		return requeueN, nil
	}

	pvcDeleted := false
	// Check if the PVC already exists using r.Get, if not create a new one using r.Create
	pvc := &corev1.PersistentVolumeClaim{}
	// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, pvc)

	if err == nil {
		currentStorageClassName := ""
		if pvc.Spec.StorageClassName != nil {
			currentStorageClassName = *pvc.Spec.StorageClassName
		}
		if currentStorageClassName != oradataCfg.StorageClass ||
			(oradataCfg.DatafilesVolume != "" && pvc.Spec.VolumeName != oradataCfg.DatafilesVolume) ||
			pvc.Spec.AccessModes[0] != corev1.PersistentVolumeAccessMode(oradataCfg.AccessMode) {
			// PV change use cases which would trigger recreation of SIDB pods are :-
			// 1. Change in storage class
			// 2. Change in volume name
			// 3. Change in volume access mode

			// deleting singleinstancedatabase resource
			result, err := r.deletePods(ctx, req, m, []corev1.Pod{}, corev1.Pod{}, 0, 0)
			if result.Requeue {
				return result, err
			}

			// deleting persistent volume claim
			log.Info("Deleting PVC", " name ", pvc.Name)
			err = r.Delete(ctx, pvc)
			if err != nil {
				r.Log.Error(err, "Failed to delete Pvc", "Pvc.Name", pvc.Name)
				return requeueN, err
			}
			pvcDeleted = true

		} else if pvc.Spec.Resources.Requests["storage"] != resource.MustParse(oradataCfg.Size) {
			// check the storage class of the pvc
			// if the storage class doesn't support resize the throw an error event and try expanding via deleting and recreating the pv and pods
			if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "PVC not resizable", "Cannot resize pvc as storage class is either nil or default")
				return requeueN, fmt.Errorf("cannot resize pvc as storage class is either nil or default")
			}

			storageClassName := *pvc.Spec.StorageClassName
			storageClass := &storagev1.StorageClass{}
			err := r.Get(ctx, types.NamespacedName{Name: storageClassName}, storageClass)
			if err != nil {
				return requeueY, fmt.Errorf("error while fetching the storage class")
			}

			if storageClass.AllowVolumeExpansion == nil || !*storageClass.AllowVolumeExpansion {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "PVC not resizable", "The storage class doesn't support volume expansion")
				return requeueN, fmt.Errorf("the storage class %s doesn't support volume expansion", storageClassName)
			}

			newPVCSize := resource.MustParse(oradataCfg.Size)
			newPVCSizeAdd := &newPVCSize
			if newPVCSizeAdd.Cmp(pvc.Spec.Resources.Requests["storage"]) < 0 {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Cannot Resize PVC", "Forbidden: field can not be less than previous value")
				return requeueN, fmt.Errorf("Resizing PVC to lower size volume not allowed")
			}

			// Expanding the persistent volume claim
			pvc.Spec.Resources.Requests["storage"] = resource.MustParse(oradataCfg.Size)
			log.Info("Updating PVC", "pvc", pvc.Name, "volume", pvc.Spec.VolumeName)
			r.Recorder.Eventf(m, corev1.EventTypeNormal, "Updating PVC - volume expansion", "Resizing the pvc for storage expansion")
			err = r.Update(ctx, pvc)
			if err != nil {
				log.Error(err, "Error while updating the PVCs")
				return requeueY, fmt.Errorf("error while updating the PVCs")
			}

		} else {

			log.Info("Found Existing PVC", "Name", pvc.Name)
			return requeueN, nil

		}
	}

	if pvcDeleted || err != nil && apierrors.IsNotFound(err) {
		// Define a new PVC
		pvc = r.instantiatePVCSpec(m)
		log.Info("Creating a new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
		err = r.Create(ctx, pvc)
		if err != nil {
			log.Error(err, "Failed to create new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
			return requeueY, err
		}
		return requeueN, nil
	} else if err != nil {
		log.Error(err, "Failed to get PVC")
		return requeueY, err
	}

	return requeueN, nil
}

func (r *SingleInstanceDatabaseReconciler) createOrReplacePVCforFRAVol(ctx context.Context, req ctrl.Request,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {

	log := r.Log.WithValues("createPVC FRA-Vol", req.NamespacedName)
	fraCfg := getFraPersistenceConfig(m)

	if !hasFraPersistence(m) {
		return requeueN, nil
	}

	claimName := getFraClaimName(m)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: claimName, Namespace: m.Namespace}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if !isManagedFraPVC(m) {
				return requeueN, fmt.Errorf("fra pvc %q not found", claimName)
			}
			pvc = r.instantiateFRAPVCSpec(m)
			log.Info("Creating a new FRA PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
			if createErr := r.Create(ctx, pvc); createErr != nil {
				log.Error(createErr, "Failed to create new FRA PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
				return requeueY, createErr
			}
			return requeueN, nil
		}
		log.Error(err, "Failed to get FRA PVC")
		return requeueY, err
	}

	if isManagedFraPVC(m) {
		currentStorageClassName := ""
		if pvc.Spec.StorageClassName != nil {
			currentStorageClassName = *pvc.Spec.StorageClassName
		}
		if currentStorageClassName != fraCfg.StorageClass ||
			(len(pvc.Spec.AccessModes) > 0 && pvc.Spec.AccessModes[0] != corev1.PersistentVolumeAccessMode(fraCfg.AccessMode)) {
			result, delErr := r.deletePods(ctx, req, m, []corev1.Pod{}, corev1.Pod{}, 0, 0)
			if result.Requeue {
				return result, delErr
			}
			log.Info("Deleting FRA PVC for immutable field changes", "name", pvc.Name)
			if delErr = r.Delete(ctx, pvc); delErr != nil {
				log.Error(delErr, "Failed to delete FRA PVC", "Pvc.Name", pvc.Name)
				return requeueN, delErr
			}
			newPVC := r.instantiateFRAPVCSpec(m)
			log.Info("Recreating FRA PVC", "PVC.Namespace", newPVC.Namespace, "PVC.Name", newPVC.Name)
			if createErr := r.Create(ctx, newPVC); createErr != nil {
				log.Error(createErr, "Failed to recreate FRA PVC", "PVC.Namespace", newPVC.Namespace, "PVC.Name", newPVC.Name)
				return requeueY, createErr
			}
			return requeueN, nil
		}
	}
	if fraCfg.PvcName != "" {
		// Referenced FRA PVC is user-managed; controller only validates existence and mounts it.
		return requeueN, nil
	}

	if fraCfg.Size == "" {
		return requeueN, nil
	}
	desiredSize := resource.MustParse(fraCfg.Size)
	currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if desiredSize.Cmp(currentSize) == 0 {
		return requeueN, nil
	}
	if desiredSize.Cmp(currentSize) < 0 {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, "Cannot Resize FRA PVC", "Forbidden: field can not be less than previous value")
		return requeueN, fmt.Errorf("resizing FRA PVC to lower size is not allowed")
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, "FRA PVC not resizable", "Cannot resize FRA PVC as storage class is either nil or default")
		return requeueN, fmt.Errorf("cannot resize FRA PVC as storage class is either nil or default")
	}
	storageClassName := *pvc.Spec.StorageClassName
	storageClass := &storagev1.StorageClass{}
	if err = r.Get(ctx, types.NamespacedName{Name: storageClassName}, storageClass); err != nil {
		return requeueY, fmt.Errorf("error while fetching storage class %q: %w", storageClassName, err)
	}
	if storageClass.AllowVolumeExpansion == nil || !*storageClass.AllowVolumeExpansion {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, "FRA PVC not resizable", "The storage class doesn't support volume expansion")
		return requeueN, fmt.Errorf("the storage class %s doesn't support volume expansion", storageClassName)
	}
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredSize
	log.Info("Updating FRA PVC - volume expansion", "pvc", pvc.Name)
	r.Recorder.Eventf(m, corev1.EventTypeNormal, "Updating FRA PVC - volume expansion", "Resizing the FRA pvc for storage expansion")
	if err = r.Update(ctx, pvc); err != nil {
		log.Error(err, "Error while updating FRA PVC")
		return requeueY, fmt.Errorf("error while updating FRA PVC")
	}

	return requeueN, nil
}

// #############################################################################
//
//	Create Services for SingleInstanceDatabase
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) createOrReplaceSVC(ctx context.Context, req ctrl.Request,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {

	log := r.Log.WithValues("createOrReplaceSVC", req.NamespacedName)

	/** Two k8s services gets created:
		  1. One service is ClusterIP service for cluster only communications on the listener port 1521,
		  2. One service is NodePort/LoadBalancer (according to the YAML specs) for users to connect
	 **/

	// clusterSvc is the cluster-wide service and extSvc is the external service for the users to connect
	clusterSvc := &corev1.Service{}
	extSvc := &corev1.Service{}

	clusterSvcName := m.Name
	extSvcName := m.Name + "-ext"

	// svcPort is the intended port for extSvc taken from singleinstancedatabase YAML file for normal database connection
	// If loadBalancer is true, it would be the listener port otherwise it would be node port
	svcPort := func() int32 {
		if m.Spec.ListenerPort != 0 {
			return int32(m.Spec.ListenerPort)
		} else {
			return dbcommons.CONTAINER_LISTENER_PORT
		}
	}()
	tcpsEnabled := getTcpsEnabled(m)
	tcpsListenerPort := getTcpsListenerPort(m)

	// tcpsSvcPort is the intended port for extSvc taken from singleinstancedatabase YAML file for TCPS connection
	// If loadBalancer is true, it would be the listener port otherwise it would be node port
	tcpsSvcPort := func() int32 {
		if tcpsListenerPort != 0 {
			return int32(tcpsListenerPort)
		} else {
			return dbcommons.CONTAINER_TCPS_PORT
		}
	}()

	// Querying for the K8s service resources
	getClusterSvcErr := r.Get(ctx, types.NamespacedName{Name: clusterSvcName, Namespace: m.Namespace}, clusterSvc)
	getExtSvcErr := r.Get(ctx, types.NamespacedName{Name: extSvcName, Namespace: m.Namespace}, extSvc)

	if getClusterSvcErr != nil && apierrors.IsNotFound(getClusterSvcErr) {
		// Create a new ClusterIP service
		ports := []corev1.ServicePort{{Name: "listener", Port: dbcommons.CONTAINER_LISTENER_PORT, Protocol: corev1.ProtocolTCP}}
		svc := r.instantiateSVCSpec(m, clusterSvcName, ports, corev1.ServiceType("ClusterIP"), true)
		log.Info("Creating a new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err := r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY, err
		}
	} else if getClusterSvcErr != nil {
		// Error encountered in obtaining the clusterSvc service resource
		log.Error(getClusterSvcErr, "Error encountered in obtaining the service", clusterSvcName)
		return requeueY, getClusterSvcErr
	}

	// extSvcType defines the type of the service (LoadBalancer/NodePort) for extSvc as specified in the singleinstancedatabase.yaml file
	extSvcType := corev1.ServiceType("NodePort")
	if m.Spec.LoadBalancer {
		extSvcType = corev1.ServiceType("LoadBalancer")
	}

	isExtSvcFound := true

	if getExtSvcErr != nil && apierrors.IsNotFound(getExtSvcErr) {
		isExtSvcFound = false
	} else if getExtSvcErr != nil {
		// Error encountered in obtaining the extSvc service resource
		log.Error(getExtSvcErr, "Error encountered in obtaining the service", extSvcName)
		return requeueY, getExtSvcErr
	} else {
		// Counting required number of ports in extSvc
		requiredPorts := 2
		if tcpsEnabled && m.Spec.ListenerPort != 0 {
			requiredPorts = 3
		}

		// Obtaining all ports of the extSvc k8s service
		var targetPorts []int32
		for _, port := range extSvc.Spec.Ports {
			if extSvc.Spec.Type == corev1.ServiceType("LoadBalancer") {
				targetPorts = append(targetPorts, port.Port)
			} else if extSvc.Spec.Type == corev1.ServiceType("NodePort") {
				targetPorts = append(targetPorts, port.NodePort)
			}
		}

		patchSvc := false

		// Conditions to determine whether to patch or not
		if extSvc.Spec.Type != extSvcType || len(extSvc.Spec.Ports) != requiredPorts {
			patchSvc = true
		}

		if (m.Spec.ListenerPort != 0 && svcPort != targetPorts[1]) || (tcpsEnabled && tcpsListenerPort != 0 && tcpsSvcPort != targetPorts[len(targetPorts)-1]) {
			patchSvc = true
		}

		if m.Spec.LoadBalancer {
			if tcpsEnabled {
				if tcpsListenerPort == 0 && tcpsSvcPort != targetPorts[len(targetPorts)-1] {
					patchSvc = true
				}
			} else {
				if m.Spec.ListenerPort == 0 && svcPort != targetPorts[1] {
					patchSvc = true
				}
			}
		} else {
			if tcpsEnabled {
				if tcpsListenerPort == 0 && tcpsSvcPort != extSvc.Spec.Ports[len(targetPorts)-1].TargetPort.IntVal {
					patchSvc = true
				}
			} else {
				if m.Spec.ListenerPort == 0 && svcPort != extSvc.Spec.Ports[1].TargetPort.IntVal {
					patchSvc = true
				}
			}
		}

		if patchSvc {
			// Reset connect strings whenever patching happens
			m.Status.Status = dbcommons.StatusUpdating
			m.Status.ConnectString = dbcommons.ValueUnavailable
			m.Status.PdbConnectString = dbcommons.ValueUnavailable
			m.Status.OemExpressUrl = dbcommons.ValueUnavailable
			m.Status.TcpsConnectString = dbcommons.ValueUnavailable
			m.Status.TcpsPdbConnectString = dbcommons.ValueUnavailable

			// Payload formation for patching the service
			var payload string
			if m.Spec.LoadBalancer {
				if tcpsEnabled {
					if m.Spec.ListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrPort, svcPort), fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					} else {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					}
				} else {
					payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrPort, svcPort))
				}
			} else {
				if tcpsEnabled {
					if m.Spec.ListenerPort != 0 && tcpsListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrNodePort, svcPort), fmt.Sprintf(dbcommons.TcpsNodePort, tcpsSvcPort))
					} else if m.Spec.ListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrNodePort, svcPort), fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					} else if tcpsListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.TcpsNodePort, tcpsSvcPort))
					} else {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					}
				} else {
					if m.Spec.ListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrNodePort, svcPort))
					} else {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrPort, svcPort))
					}
				}
			}

			//Attemp Service Pathcing
			log.Info("Patching the service", "Service.Name", extSvc.Name, "payload", payload)
			err := dbcommons.PatchService(r.Config, m.Namespace, ctx, req, extSvcName, payload)
			if err != nil {
				log.Error(err, "Failed to patch Service")
			}
			//Requeue once after patching
			return requeueY, err
		}
	}

	if !isExtSvcFound {
		// Reset connect strings whenever extSvc is recreated
		m.Status.Status = dbcommons.StatusUpdating
		m.Status.ConnectString = dbcommons.ValueUnavailable
		m.Status.PdbConnectString = dbcommons.ValueUnavailable
		m.Status.OemExpressUrl = dbcommons.ValueUnavailable
		m.Status.TcpsConnectString = dbcommons.ValueUnavailable
		m.Status.TcpsPdbConnectString = dbcommons.ValueUnavailable

		// New service has to be created
		ports := []corev1.ServicePort{
			{
				Name:     "xmldb",
				Port:     5500,
				Protocol: corev1.ProtocolTCP,
			},
		}

		if m.Spec.LoadBalancer {
			if tcpsEnabled {
				if m.Spec.ListenerPort != 0 {
					ports = append(ports, corev1.ServicePort{
						Name:       "listener",
						Protocol:   corev1.ProtocolTCP,
						Port:       svcPort,
						TargetPort: intstr.FromInt(int(dbcommons.CONTAINER_LISTENER_PORT)),
					})
				}
				ports = append(ports, corev1.ServicePort{
					Name:       "listener-tcps",
					Protocol:   corev1.ProtocolTCP,
					Port:       tcpsSvcPort,
					TargetPort: intstr.FromInt(int(dbcommons.CONTAINER_TCPS_PORT)),
				})
			} else {
				ports = append(ports, corev1.ServicePort{
					Name:       "listener",
					Protocol:   corev1.ProtocolTCP,
					Port:       svcPort,
					TargetPort: intstr.FromInt(int(dbcommons.CONTAINER_LISTENER_PORT)),
				})
			}
		} else {
			if tcpsEnabled {
				if m.Spec.ListenerPort != 0 {
					ports = append(ports, corev1.ServicePort{
						Name:     "listener",
						Protocol: corev1.ProtocolTCP,
						Port:     dbcommons.CONTAINER_LISTENER_PORT,
						NodePort: svcPort,
					})
				}
				ports = append(ports, corev1.ServicePort{
					Name:     "listener-tcps",
					Protocol: corev1.ProtocolTCP,
					Port:     dbcommons.CONTAINER_TCPS_PORT,
				})
				if tcpsListenerPort != 0 {
					ports[len(ports)-1].NodePort = tcpsSvcPort
				}
			} else {
				ports = append(ports, corev1.ServicePort{
					Name:     "listener",
					Protocol: corev1.ProtocolTCP,
					Port:     dbcommons.CONTAINER_LISTENER_PORT,
				})
				if m.Spec.ListenerPort != 0 {
					ports[len(ports)-1].NodePort = svcPort
				}
			}
		}

		// Create the service
		svc := r.instantiateSVCSpec(m, extSvcName, ports, extSvcType, false)
		log.Info("Creating a new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err := r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY, err
		}
		extSvc = svc
	}

	var sid, pdbName string
	var getSidPdbEditionErr error
	if m.Spec.Image.PrebuiltDB {
		r.Log.Info("Initiliazing database sid, pdb, edition for prebuilt database")
		var edition string
		sid, pdbName, edition, getSidPdbEditionErr = dbcommons.GetSidPdbEdition(r, r.Config, ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: m.Namespace, Name: m.Name}})
		if errors.Is(getSidPdbEditionErr, dbcommons.ErrNoReadyPod) {
			return requeueN, nil
		}
		if getSidPdbEditionErr != nil {
			return requeueY, getSidPdbEditionErr
		}
		r.Log.Info(fmt.Sprintf("Prebuilt database: %s has SID : %s, PDB : %s, EDITION: %s", m.Name, sid, pdbName, edition))
		m.Status.Edition = cases.Title(language.English).String(edition)
	}
	if sid == "" {
		sid = strings.ToUpper(m.Spec.Sid)
	}
	if pdbName == "" {
		pdbName = strings.ToUpper(m.Spec.Pdbname)
	}
	if m.Spec.LoadBalancer {
		m.Status.ClusterConnectString = extSvc.Name + "." + extSvc.Namespace + ":" + fmt.Sprint(extSvc.Spec.Ports[1].Port) + "/" + strings.ToUpper(sid)
		if len(extSvc.Status.LoadBalancer.Ingress) > 0 {
			// 'lbAddress' will contain the Fully Qualified Hostname of the LB. If the hostname is not available it will contain the IP address of the LB
			lbAddress := extSvc.Status.LoadBalancer.Ingress[0].Hostname
			if lbAddress == "" {
				lbAddress = extSvc.Status.LoadBalancer.Ingress[0].IP
			}
			m.Status.ConnectString = lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[1].Port) + "/" + strings.ToUpper(sid)
			m.Status.PdbConnectString = lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[1].Port) + "/" + strings.ToUpper(pdbName)
			m.Status.OemExpressUrl = "https://" + lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[0].Port) + "/em"
			if tcpsEnabled {
				m.Status.TcpsConnectString = lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].Port) + "/" + strings.ToUpper(sid)
				m.Status.TcpsPdbConnectString = lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].Port) + "/" + strings.ToUpper(pdbName)
			}
		}
	} else {
		m.Status.ClusterConnectString = extSvc.Name + "." + extSvc.Namespace + ":" + fmt.Sprint(extSvc.Spec.Ports[1].Port) + "/" + strings.ToUpper(sid)
		nodeip := dbcommons.GetNodeIp(r, ctx, req)
		if nodeip != "" {
			m.Status.ConnectString = nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[1].NodePort) + "/" + strings.ToUpper(sid)
			m.Status.PdbConnectString = nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[1].NodePort) + "/" + strings.ToUpper(pdbName)
			m.Status.OemExpressUrl = "https://" + nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[0].NodePort) + "/em"
			if tcpsEnabled {
				m.Status.TcpsConnectString = nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].NodePort) + "/" + strings.ToUpper(sid)
				m.Status.TcpsPdbConnectString = nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].NodePort) + "/" + strings.ToUpper(pdbName)
			}
		}
	}

	return requeueN, nil
}

// #############################################################################
//
//	Create new Pods or delete old/extra pods
//	m = SingleInstanceDatabase
//	n = CloneFromDatabase
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) createOrReplacePods(m *dbapi.SingleInstanceDatabase, n *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("createOrReplacePods", req.NamespacedName)

	oldVersion := ""
	oldImage := ""

	// call FindPods() to fetch pods all version/images of the same SIDB kind
	readyPod, replicasFound, allAvailable, podsMarkedToBeDeleted, err := dbcommons.FindPods(r, "", "", m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, err
	}

	// Recreate new pods only after earlier pods are terminated completely
	for i := 0; i < len(podsMarkedToBeDeleted); i++ {
		r.Log.Info("Force deleting pod ", "name", podsMarkedToBeDeleted[i].Name, "phase", podsMarkedToBeDeleted[i].Status.Phase)
		var gracePeriodSeconds int64 = 0
		policy := metav1.DeletePropagationForeground
		if err := r.Delete(ctx, &podsMarkedToBeDeleted[i], &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy}); err != nil {
			r.Log.Error(err, "Failed to force delete pod", "name", podsMarkedToBeDeleted[i].Name)
		}
	}

	if readyPod.Name != "" {
		allAvailable = append(allAvailable, readyPod)
	}

	for _, pod := range allAvailable {
		if pod.Labels["version"] != m.Spec.Image.Version {
			oldVersion = pod.Labels["version"]
		}
		if pod.Spec.Containers[0].Image != m.Spec.Image.PullFrom {
			oldImage = pod.Spec.Containers[0].Image
		}

	}

	// podVersion, podImage if old version PODs are found
	imageChanged := oldVersion != "" || oldImage != ""

	if !imageChanged {
		eventReason := ""
		eventMsg := ""
		if replicasFound > m.Spec.Replicas {
			eventReason = "Scaling in pods"
			eventMsg = "from " + strconv.Itoa(replicasFound) + " to " + strconv.Itoa(m.Spec.Replicas)
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			// Delete extra PODs
			return r.deletePods(ctx, req, m, allAvailable, readyPod, replicasFound, m.Spec.Replicas)
		}
		if replicasFound != 0 {
			if replicasFound == 1 {
				if m.Status.DatafilesCreated != "true" {
					log.Info("No datafiles created, single replica found, creating wallet")
					// Creation of Oracle Wallet for Single Instance Database credentials
					walletRes, walletErr := r.createWallet(m, ctx, req)
					if walletErr != nil || walletRes.Requeue {
						return walletRes, walletErr
					}
				}
			}
			if ok, _ := dbcommons.IsAnyPodWithStatus(allAvailable, corev1.PodRunning); !ok {
				eventReason = "Database Pending"
				eventMsg = "waiting for a pod to get to running state"
				log.Info(eventMsg)
				r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
				for i := 0; i < len(allAvailable); i++ {
					r.Log.Info("Pod status: ", "name", allAvailable[i].Name, "phase", allAvailable[i].Status.Phase)
					waitingReason := ""
					var stateWaiting *corev1.ContainerStateWaiting
					if len(allAvailable[i].Status.InitContainerStatuses) > 0 {
						stateWaiting = allAvailable[i].Status.InitContainerStatuses[0].State.Waiting
					} else if len(allAvailable[i].Status.ContainerStatuses) > 0 {
						stateWaiting = allAvailable[i].Status.ContainerStatuses[0].State.Waiting
					}
					if stateWaiting != nil {
						waitingReason = stateWaiting.Reason
					}
					if waitingReason == "" {
						continue
					}
					r.Log.Info("Pod unavailable reason: ", "reason", waitingReason)
					if strings.Contains(waitingReason, "ImagePullBackOff") || strings.Contains(waitingReason, "ErrImagePull") {
						r.Log.Info("Deleting pod", "name", allAvailable[i].Name)
						var gracePeriodSeconds int64 = 0
						policy := metav1.DeletePropagationForeground
						if err := r.Delete(ctx, &allAvailable[i], &client.DeleteOptions{
							GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy}); err != nil {
							r.Log.Error(err, "Failed to delete pod in image pull backoff", "name", allAvailable[i].Name)
						}
					}
				}
				return requeueY, err
			}
		}
		if replicasFound == m.Spec.Replicas {
			return requeueN, nil
		}
		if replicasFound != 0 && replicasFound < m.Spec.Replicas {
			eventReason = "Scaling out pods"
			eventMsg = "from " + strconv.Itoa(replicasFound) + " to " + strconv.Itoa(m.Spec.Replicas)
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		}
		// If version is same , call createPods() with the same version ,  and no of Replicas required
		return r.createPods(m, n, rp, ctx, req, replicasFound, false)
	}

	// Version/Image changed
	// PATCHING START (Only Software Patch)
	log.Info("Pod image change detected, datapatch to be rerun...")
	m.Status.DatafilesPatched = "false"
	// call FindPods() to find pods of older version. Delete all the Pods
	readyPod, oldReplicasFound, oldAvailable, _, err := dbcommons.FindPods(r, oldVersion,
		oldImage, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, err
	}
	if readyPod.Name != "" {
		log.Info("Ready pod marked for deletion", "name", readyPod.Name)
		oldAvailable = append(oldAvailable, readyPod)
	}

	if m.Status.Replicas == 1 {
		if _, err := r.deletePods(ctx, req, m, oldAvailable, corev1.Pod{}, oldReplicasFound, 0); err != nil {
			log.Error(err, "failed to delete old pods during image update")
			return requeueY, err
		}
	}

	// call FindPods() to find pods of newer version . if running , delete the older version replicas.
	readyPod, newReplicasFound, newAvailable, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, nil
	}
	// Findpods() only returns non ready pods
	if readyPod.Name != "" {
		log.Info("New ready pod found", "name", readyPod.Name)
		newAvailable = append(newAvailable, readyPod)
	}

	if newReplicasFound != 0 {
		if ok, _ := dbcommons.IsAnyPodWithStatus(newAvailable, corev1.PodRunning); !ok {
			eventReason := "Database Pending"
			eventMsg := "waiting for pod with changed image to get to running state"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			log.Info(eventMsg)

			for i := 0; i < len(newAvailable); i++ {
				r.Log.Info("Pod status: ", "name", newAvailable[i].Name, "phase", newAvailable[i].Status.Phase)
				waitingReason := ""
				var stateWaiting *corev1.ContainerStateWaiting
				if len(newAvailable[i].Status.InitContainerStatuses) > 0 {
					stateWaiting = newAvailable[i].Status.InitContainerStatuses[0].State.Waiting
				} else if len(newAvailable[i].Status.ContainerStatuses) > 0 {
					stateWaiting = newAvailable[i].Status.ContainerStatuses[0].State.Waiting
				}
				if stateWaiting != nil {
					waitingReason = stateWaiting.Reason
				}
				if waitingReason == "" {
					continue
				}
				r.Log.Info("Pod unavailable reason: ", "reason", waitingReason)
				if strings.Contains(waitingReason, "ImagePullBackOff") || strings.Contains(waitingReason, "ErrImagePull") {
					r.Log.Info("Deleting pod", "name", newAvailable[i].Name)
					var gracePeriodSeconds int64 = 0
					policy := metav1.DeletePropagationForeground
					if err := r.Delete(ctx, &newAvailable[i], &client.DeleteOptions{
						GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy}); err != nil {
						r.Log.Error(err, "Failed to delete pod in image pull backoff", "name", newAvailable[i].Name)
					}
				}
			}
			return requeueY, errors.New(eventMsg)
		}
	}

	// create new Pods with the new Version and no.of Replicas required
	// if m.Status.Replicas > 1, then it is replica based patching
	result, err := r.createPods(m, n, rp, ctx, req, newReplicasFound, m.Status.Replicas > 1)
	if result.Requeue {
		return result, err
	}
	if m.Status.Replicas == 1 {
		return requeueN, nil
	}
	return r.deletePods(ctx, req, m, oldAvailable, corev1.Pod{}, oldReplicasFound, 0)
	// PATCHING END
}

// #############################################################################
//
//	Function for creating Oracle Wallet
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) createWallet(m *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Wallet not supported for Express/Free Database
	if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
		return requeueN, nil
	}

	// No Wallet for Pre-built db
	if m.Spec.Image.PrebuiltDB {
		return requeueN, nil
	}
	// Explicit secret-mount mode bypasses wallet seeding.
	if GetAdminPasswordSkipInitWallet(m) {
		return requeueN, nil
	}

	// Listing all the pods
	readyPod, _, availableFinal, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, nil
	}
	if readyPod.Name != "" {
		return requeueN, nil
	}

	// Wallet is created in persistent volume, hence it only needs to be executed once for all number of pods
	if len(availableFinal) == 0 {
		r.Log.Info("Pods are being created, currently no pods available")
		return requeueY, nil
	}

	// Iterate through the availableFinal (list of pods) to find out the pod whose status is updated about the init containers
	// If no required pod found then requeue the reconcile request
	var pod corev1.Pod
	var podFound bool
	for _, pod = range availableFinal {
		// Check if pod status container is updated about init containers
		if len(pod.Status.InitContainerStatuses) > 0 {
			podFound = true
			break
		}
	}
	if !podFound {
		r.Log.Info("No pod has its status updated about init containers. Requeueing...")
		return requeueY, nil
	}

	lastInitContIndex := len(pod.Status.InitContainerStatuses) - 1
	walletContainer := "init-wallet"

	// If InitContainerStatuses[<index_of_init_container>].Ready is true, it means that the init container is successful
	if pod.Status.InitContainerStatuses[lastInitContIndex].Ready {
		// init-wallet has completed. Continue wallet credential creation in the main container.
		walletContainer = m.Name
	}

	if pod.Status.InitContainerStatuses[lastInitContIndex].State.Running == nil {
		if pod.Status.InitContainerStatuses[lastInitContIndex].State.Terminated != nil {
			// init-wallet has terminated, continue in main container
			walletContainer = m.Name
		} else {
			// Init container named "init-wallet" is not running, so waiting for it to come in running state requeueing the reconcile request
			r.Log.Info("Waiting for init-wallet to come in running state...")
			return requeueY, nil
		}
	}

	if m.Spec.CreateAs != "clone" && m.Spec.Edition != "express" {
		//Check if Edition of m.Spec.Sid is same as m.Spec.Edition
		getEditionFile := dbcommons.GetEnterpriseEditionFileCMD
		eventReason := m.Spec.Sid + " is a enterprise edition"
		if m.Spec.Edition == "enterprise" || m.Spec.Edition == "" {
			getEditionFile = dbcommons.GetStandardEditionFileCMD
			eventReason = m.Spec.Sid + " is a standard edition"
		}
		out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, walletContainer,
			ctx, req, false, "bash", "-c", getEditionFile)
		r.Log.Info("getEditionFile Output : \n" + out)

		if err == nil && out != "" {
			m.Status.Status = dbcommons.StatusError
			eventMsg := "incorrect database edition"
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			return requeueY, errors.New(eventMsg)
		}
	}

	r.Log.Info("Creating Wallet...")

	// Querying the secret
	r.Log.Info("Querying the database secret ...")
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: GetAdminPasswordSecretName(m), Namespace: m.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Secret not found")
			m.Status.Status = dbcommons.StatusError
			if updateErr := r.Status().Update(ctx, m); updateErr != nil {
				r.Log.Error(updateErr, "failed to update status after secret not found")
			}
			return requeueY, nil
		}
		r.Log.Error(err, "Unable to get the secret. Requeueing..")
		return requeueY, nil
	}

	// Execing into the pods and creating the wallet
	adminPassword := string(secret.Data[GetAdminPasswordSecretFileName(m)])

	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, walletContainer,
		ctx, req, true, "bash", "-c", fmt.Sprintf("%s && %s && %s",
			dbcommons.WalletPwdCMD,
			dbcommons.WalletCreateCMD,
			fmt.Sprintf(dbcommons.WalletEntriesCMD, adminPassword)))
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, nil
	}
	r.Log.Info("Creating wallet entry Output : \n" + out)

	return requeueN, nil
}

// ##############################################################################
//
//	Create the requested POD replicas
//	m = SingleInstanceDatabase
//	n = CloneFromDatabase
//	patching =  Boolean variable to differentiate normal usecase with patching
//
// ##############################################################################
func (r *SingleInstanceDatabaseReconciler) createPods(m *dbapi.SingleInstanceDatabase, n *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request, replicasFound int, replicaPatching bool) (ctrl.Result, error) {

	log := r.Log.WithValues("createPods", req.NamespacedName)

	replicasReq := m.Spec.Replicas
	log.Info("Replica Info", "Found", replicasFound, "Required", replicasReq)
	if replicasFound == replicasReq {
		log.Info("No of " + m.Name + " replicas found are same as required")
		return requeueN, nil
	}
	firstPod := false
	if replicasFound == 0 {
		m.Status.Status = dbcommons.StatusPending
		firstPod = true
	}
	if !replicaPatching {
		m.Status.Replicas = replicasFound
	}
	//  if Found < Required, create new pods, name of pods are generated randomly
	for i := replicasFound; i < replicasReq; i++ {
		// mandatory pod affinity if it is replica based patching or not the first pod
		pod, err := r.instantiatePodSpec(m, n, rp, replicaPatching || !firstPod)
		if err != nil {
			log.Error(err, "Failed to instantiate pod spec")
			return requeueY, err
		}
		log.Info("Creating a new "+m.Name+" POD", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
		err = r.Create(ctx, pod)
		if err != nil {
			log.Error(err, "Failed to create new "+m.Name+" POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
			return requeueY, err
		}
		m.Status.Replicas++
		if firstPod {
			log.Info("Requeue for first pod to get to running state", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
			return requeueY, err
		}
	}

	readyPod, _, availableFinal, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, err
	}
	if readyPod.Name != "" {
		availableFinal = append(availableFinal, readyPod)
	}

	podNamesFinal := dbcommons.GetPodNames(availableFinal)
	log.Info("Final "+m.Name+" Pods After Deleting (or) Adding Extra Pods ( Including The Ready Pod ) ", "Pod Names", podNamesFinal)
	log.Info(m.Name+" Replicas Available", "Count", len(podNamesFinal))
	log.Info(m.Name+" Replicas Required", "Count", replicasReq)

	return requeueN, nil
}

// #############################################################################
//
//	Create the requested POD replicas
//	m = SingleInstanceDatabase
//	n = CloneFromDatabase
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) deletePods(ctx context.Context, req ctrl.Request, m *dbapi.SingleInstanceDatabase, available []corev1.Pod,
	readyPod corev1.Pod, replicasFound int, replicasRequired int) (ctrl.Result, error) {
	log := r.Log.WithValues("deletePods", req.NamespacedName)

	var err error
	if len(available) == 0 {
		// As list of pods not avaiable . fetch them ( len(available) == 0 ; Usecase where deletion of all pods required )
		var readyPodToBeDeleted corev1.Pod
		readyPodToBeDeleted, replicasFound, available, _, err = dbcommons.FindPods(r, "",
			"", m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		// Append readyPod to avaiable for deleting all pods
		if readyPodToBeDeleted.Name != "" {
			available = append(available, readyPodToBeDeleted)
		}
	}

	// For deleting all pods , call with readyPod as nil ( corev1.Pod{} ) and append readyPod to available while calling deletePods()
	//  if Found > Required , Delete Extra Pods
	if replicasFound > len(available) {
		// if available does not contain readyPOD, add it
		available = append(available, readyPod)
	}

	noDeleted := 0
	for _, availablePod := range available {
		if readyPod.Name == availablePod.Name && m.Spec.Replicas != 0 {
			continue
		}
		if replicasRequired == (len(available) - noDeleted) {
			break
		}
		r.Log.Info("Deleting Pod : ", "POD.NAME", availablePod.Name)
		var delOpts *client.DeleteOptions = &client.DeleteOptions{}
		if replicasRequired == 0 {
			var gracePeriodSeconds int64 = 0
			policy := metav1.DeletePropagationForeground
			delOpts.GracePeriodSeconds = &gracePeriodSeconds
			delOpts.PropagationPolicy = &policy
		}
		err := r.Delete(ctx, &availablePod, delOpts)
		noDeleted++
		if err != nil {
			r.Log.Error(err, "Failed to delete existing POD", "POD.Name", availablePod.Name)
			// Don't requeue
		} else {
			m.Status.Replicas--
		}
	}

	return requeueN, nil
}

// #############################################################################
//
//	ValidateDBReadiness and return the ready POD
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) validateDBReadiness(sidb *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod, error) {

	log := r.Log.WithValues("validateDBReadiness", req.NamespacedName)

	log.Info("Validating readiness for database")

	sidbReadyPod, _, available, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, sidbReadyPod, err
	}

	if sidbReadyPod.Name == "" {
		sidb.Status.Status = dbcommons.StatusPending
		log.Info("no pod currently in ready state")
		if ok, _ := dbcommons.IsAnyPodWithStatus(available, corev1.PodFailed); ok {
			eventReason := "Database Failed"
			eventMsg := "pod creation failed"
			r.Recorder.Eventf(sidb, corev1.EventTypeNormal, eventReason, eventMsg)
		} else if ok, _ := dbcommons.IsAnyPodWithStatus(available, corev1.PodRunning); ok {

			out, err := dbcommons.ExecCommand(r, r.Config, available[0].Name, sidb.Namespace, "",
				ctx, req, false, "bash", "-c", dbcommons.GetCheckpointFileCMD)
			if err != nil {
				r.Log.Info(err.Error())
			}

			if out != "" {
				log.Info("Database initialzied")
				eventReason := "Database Unhealthy"
				eventMsg := "datafiles exists"
				r.Recorder.Eventf(sidb, corev1.EventTypeNormal, eventReason, eventMsg)
				sidb.Status.DatafilesCreated = "true"
				sidb.Status.Status = dbcommons.StatusNotReady
				r.updateORDSStatus(sidb, ctx, req)
			} else {
				log.Info("Database Creating....", "Name", sidb.Name)
				sidb.Status.Status = dbcommons.StatusCreating
			}

		} else {
			log.Info("Database Pending....", "Name", sidb.Name)
		}
		log.Info("no pod currently in ready state")
		return requeueY, sidbReadyPod, nil
	}

	if sidb.Spec.CreateAs == "clone" {
		// Required since clone creates the datafiles under primary database SID folder
		r.Log.Info("Creating the SID directory link for clone database", "name", sidb.Spec.Sid)
		_, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
			ctx, req, false, "bash", "-c", dbcommons.CreateSIDlinkCMD)
		if err != nil {
			r.Log.Info(err.Error())
		}
	}

	version, err := dbcommons.GetDatabaseVersion(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		return requeueY, sidbReadyPod, err
	}
	dbMajorVersion, err := strconv.Atoi(strings.Split(version, ".")[0])
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, sidbReadyPod, err
	}
	r.Log.Info("DB Major Version is " + strconv.Itoa(dbMajorVersion))
	// Validating that free edition of the database is only supported from database 23c onwards
	if sidb.Spec.Edition == "free" && dbMajorVersion < 23 {
		errMsg := "the Oracle Database Free is only available from version 23c onwards"
		r.Recorder.Eventf(sidb, corev1.EventTypeWarning, "Spec Error", errMsg)
		sidb.Status.Status = dbcommons.StatusError
		return requeueY, sidbReadyPod, errors.New(errMsg)
	}

	available = append(available, sidbReadyPod)
	podNamesFinal := dbcommons.GetPodNames(available)
	r.Log.Info("Final "+sidb.Name+" Pods After Deleting (or) Adding Extra Pods ( Including The Ready Pod ) ", "Pod Names", podNamesFinal)
	r.Log.Info(sidb.Name+" Replicas Available", "Count", len(podNamesFinal))
	r.Log.Info(sidb.Name+" Replicas Required", "Count", sidb.Spec.Replicas)

	eventReason := "Database Ready"
	eventMsg := "database open on pod " + sidbReadyPod.Name + " scheduled on node " + sidbReadyPod.Status.HostIP
	r.Recorder.Eventf(sidb, corev1.EventTypeNormal, eventReason, eventMsg)

	sidb.Status.CreatedAs = sidb.Spec.CreateAs

	return requeueN, sidbReadyPod, nil

}

// #############################################################################
//
//	Function for deleting the Oracle Wallet
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) deleteWallet(m *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Wallet not supported for Express/Free Database
	if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
		return requeueN, nil
	}

	// No Wallet for Pre-built db
	if m.Spec.Image.PrebuiltDB {
		return requeueN, nil
	}

	// Deleting the secret and then deleting the wallet
	// If the secret is not found it means that the secret and wallet both are deleted, hence no need to requeue
	if !GetAdminPasswordKeepSecret(m) {
		r.Log.Info("Querying the database secret ...")
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: GetAdminPasswordSecretName(m), Namespace: m.Namespace}, secret)
		if err == nil {
			err := r.Delete(ctx, secret)
			if err == nil {
				r.Log.Info("Deleted the secret : " + GetAdminPasswordSecretName(m))
			}
		}
	}

	// Getting the ready pod for the database
	readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, err
	}

	// Deleting the wallet
	_, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", dbcommons.WalletDeleteCMD)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, nil
	}
	r.Log.Info("Wallet Deleted !!")
	return requeueN, nil
}

// #############################################################################
//
//	Updating clientWallet when TCPS is enabled
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateClientWallet(m *dbapi.SingleInstanceDatabase,
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	// Updation of tnsnames.ora in clientWallet for HOST and PORT fields
	extSvc := &corev1.Service{}
	extSvcName := m.Name + "-ext"
	getExtSvcErr := r.Get(ctx, types.NamespacedName{Name: extSvcName, Namespace: m.Namespace}, extSvc)

	if getExtSvcErr == nil {
		var host string
		var port int32
		if m.Spec.LoadBalancer {
			if len(extSvc.Status.LoadBalancer.Ingress) > 0 {
				host = extSvc.Status.LoadBalancer.Ingress[0].Hostname
				if host == "" {
					host = extSvc.Status.LoadBalancer.Ingress[0].IP
				}
				port = extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].Port
			}
		} else {
			host = dbcommons.GetNodeIp(r, ctx, req)
			if host != "" {
				port = extSvc.Spec.Ports[len(extSvc.Spec.Ports)-1].NodePort
			}
		}

		r.Log.Info("Updating the client wallet...")
		_, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.ClientWalletUpdate, host, port))
		if err != nil {
			r.Log.Error(err, err.Error())
			return err
		}

	} else {
		r.Log.Info("Unable to get the service while updating the clientWallet", "Service.Namespace", extSvc.Namespace, "Service.Name", extSvcName)
		return getExtSvcErr
	}
	return nil
}

// #############################################################################
//
//	Configuring TCPS
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) configTcps(m *dbapi.SingleInstanceDatabase,
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request, phaseCtx *sidbPhaseContext) (ctrl.Result, error) {
	eventReason := "Configuring TCPS"
	tcpsEnabled := getTcpsEnabled(m)
	tcpsTLSSecret := getTcpsTLSSecret(m)
	tcpsCertRenewInterval := getTcpsCertRenewInterval(m)

	if (tcpsEnabled) &&
		((!m.Status.IsTcpsEnabled) || // TCPS Enabled from a TCP state
			(tcpsTLSSecret != "" && m.Status.TcpsTlsSecret == "") || // TCPS Secret is added in spec
			(tcpsTLSSecret == "" && m.Status.TcpsTlsSecret != "") || // TCPS Secret is removed in spec
			(tcpsTLSSecret != "" && m.Status.TcpsTlsSecret != "" && tcpsTLSSecret != m.Status.TcpsTlsSecret)) { //TCPS secret is changed

		// Set status to Updating, except when an error has been thrown from configTCPS script
		if m.Status.Status != dbcommons.StatusError {
			m.Status.Status = dbcommons.StatusUpdating
		}
		if err := r.Status().Update(ctx, m); err != nil {
			return requeueY, err
		}

		eventMsg := "Enabling TCPS in the database..."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

		var TcpsCommand = dbcommons.EnableTcpsCMD
		if tcpsTLSSecret != "" { // case when tls secret is either added or changed
			tcpsCertsLocation := getTcpsCertsLocation(m)
			TcpsCommand = "export TCPS_CERTS_LOCATION='" + tcpsCertsLocation + "' && " + dbcommons.EnableTcpsCMD

			// Checking for tls-secret mount in pods
			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
				ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.PodMountsCmd, tcpsCertsLocation))
			r.Log.Info("Mount Check Output")
			r.Log.Info(out)
			if err != nil {
				r.Log.Error(err, err.Error())
				return requeueY, nil
			}

			if (m.Status.TcpsTlsSecret != "") || // case when TCPS Secret is changed
				(!strings.Contains(out, tcpsCertsLocation)) { // if mount is not there in pod
				// call deletePods() with zero pods in avaiable and nil readyPod to delete all pods
				result, err := r.deletePods(ctx, req, m, []corev1.Pod{}, corev1.Pod{}, 0, 0)
				if result.Requeue {
					return result, err
				}
				m.Status.TcpsTlsSecret = "" // to avoid reconciled pod deletions, in case of TCPS secret change and it fails
			}
		}

		// Enable TCPS
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", TcpsCommand)
		if err != nil {
			r.Log.Error(err, err.Error())
			eventMsg = "Error encountered in enabling TCPS!"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			m.Status.Status = dbcommons.StatusError
			if updateErr := r.Status().Update(ctx, m); updateErr != nil {
				r.Log.Error(updateErr, "failed to update status after TCPS enable error")
			}
			return requeueY, nil
		}
		r.Log.Info("enableTcps Output : \n" + out)
		// Updating the Status and publishing the event
		m.Status.CertCreationTimestamp = time.Now().Format(time.RFC3339)
		m.Status.IsTcpsEnabled = true
		m.Status.ClientWalletLoc = fmt.Sprintf(dbcommons.ClientWalletLocation, m.Spec.Sid)
		// tcpsTLSSecret can be empty or non-empty
		// Store secret name in case of tls-secret addition or change, otherwise would be ""
		if tcpsTLSSecret != "" {
			m.Status.TcpsTlsSecret = tcpsTLSSecret
		} else {
			m.Status.TcpsTlsSecret = ""
		}

		if err := r.Status().Update(ctx, m); err != nil {
			return requeueY, err
		}

		eventMsg = "TCPS Enabled."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

		requeueDuration, _ := time.ParseDuration(tcpsCertRenewInterval)
		requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
		phaseCtx.futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}

		// update clientWallet
		err = r.updateClientWallet(m, readyPod, ctx, req)
		if err != nil {
			r.Log.Error(err, "Error in updating tnsnames.ora in clientWallet...")
			return requeueY, nil
		}
	} else if !tcpsEnabled && m.Status.IsTcpsEnabled {
		// Disable TCPS
		m.Status.Status = dbcommons.StatusUpdating
		if err := r.Status().Update(ctx, m); err != nil {
			return requeueY, err
		}

		eventMsg := "Disabling TCPS in the database..."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", dbcommons.DisableTcpsCMD)
		if err != nil {
			r.Log.Error(err, err.Error())
			return requeueY, nil
		}
		r.Log.Info("disable TCPS Output : \n" + out)
		// Updating the Status and publishing the event
		m.Status.CertCreationTimestamp = ""
		m.Status.IsTcpsEnabled = false
		m.Status.ClientWalletLoc = ""
		m.Status.TcpsTlsSecret = ""

		if err := r.Status().Update(ctx, m); err != nil {
			return requeueY, err
		}

		eventMsg = "TCPS Disabled."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	} else if tcpsEnabled && m.Status.IsTcpsEnabled && tcpsCertRenewInterval != "" {
		// Cert Renewal Logic
		certCreationTimestamp, _ := time.Parse(time.RFC3339, m.Status.CertCreationTimestamp)
		duration := time.Since(certCreationTimestamp)
		allowdDuration, _ := time.ParseDuration(tcpsCertRenewInterval)
		if duration > allowdDuration {
			m.Status.Status = dbcommons.StatusUpdating
			if err := r.Status().Update(ctx, m); err != nil {
				return requeueY, err
			}

			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
				ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.EnableTcpsCMD))
			if err != nil {
				r.Log.Error(err, err.Error())
				return requeueY, nil
			}
			r.Log.Info("Cert Renewal Output : \n" + out)
			// Updating the Status and publishing the event
			m.Status.CertCreationTimestamp = time.Now().Format(time.RFC3339)
			if err := r.Status().Update(ctx, m); err != nil {
				return requeueY, err
			}

			eventMsg := "TCPS Certificates Renewed at time %s,"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg, time.Now().Format(time.RFC3339))

			requeueDuration, _ := time.ParseDuration(tcpsCertRenewInterval)
			requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
			phaseCtx.futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}
		}
		if m.Status.CertRenewInterval != tcpsCertRenewInterval {
			requeueDuration, _ := time.ParseDuration(tcpsCertRenewInterval)
			requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
			phaseCtx.futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}

			m.Status.CertRenewInterval = tcpsCertRenewInterval
		}
		// update clientWallet
		err := r.updateClientWallet(m, readyPod, ctx, req)
		if err != nil {
			r.Log.Error(err, "Error in updating tnsnames.ora clientWallet...")
			return requeueY, nil
		}
	} else if tcpsEnabled && m.Status.IsTcpsEnabled && tcpsCertRenewInterval == "" {
		// update clientWallet
		err := r.updateClientWallet(m, readyPod, ctx, req)
		if err != nil {
			r.Log.Error(err, "Error in updating tnsnames.ora clientWallet...")
			return requeueY, nil
		}
	}
	return requeueN, nil
}

// #############################################################################
//
//	Execute Datapatch
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) runDatapatch(m *dbapi.SingleInstanceDatabase,
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Datapatch not supported for XE Database
	if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
		eventReason := "Datapatch Check"
		eventMsg := "datapatch not supported for " + m.Spec.Edition + " edition"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		r.Log.Info(eventMsg)
		return requeueN, nil
	}

	m.Status.Status = dbcommons.StatusPatching
	eventReason := "Datapatch Executing"
	eventMsg := "datapatch begins execution"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
	if err := r.Status().Update(ctx, m); err != nil {
		return requeueY, err
	}

	//RUN DATAPATCH
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", dbcommons.RunDatapatchCMD)
	if err != nil {
		r.Log.Error(err, err.Error())
		return requeueY, err
	}
	r.Log.Info("Datapatch output")
	r.Log.Info(out)

	// Get Sqlpatch Description
	out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba ", dbcommons.GetSqlpatchDescriptionSQL))
	releaseUpdate := ""
	if err == nil {
		r.Log.Info("GetSqlpatchDescriptionSQL Output")
		r.Log.Info(out)
		SqlpatchDescriptions, _ := dbcommons.StringToLines(out)
		if len(SqlpatchDescriptions) > 0 {
			releaseUpdate = SqlpatchDescriptions[0]
		}
	}

	eventReason = "Datapatch Done"
	if strings.Contains(out, "Datapatch execution has failed.") {
		eventMsg = "datapatch execution failed"
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueN, errors.New(eventMsg)
	}

	m.Status.DatafilesPatched = "true"
	status, versionFrom, versionTo, _ := dbcommons.GetSqlpatchStatus(r, r.Config, readyPod, ctx, req)
	if versionTo != "" {
		eventMsg = "data files patched from release update " + versionFrom + " to " + versionTo + ", " + status + ": " + releaseUpdate
	} else {
		eventMsg = "datapatch execution completed"
	}
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	return requeueN, nil
}

// #############################################################################
//
//	Update Init Parameters
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateInitParameters(m *dbapi.SingleInstanceDatabase,
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("updateInitParameters", req.NamespacedName)

	if m.Spec.InitParams == nil {
		return requeueN, nil
	}
	if m.Status.InitParams == *m.Spec.InitParams {
		return requeueN, nil
	}

	if (m.Spec.InitParams.PgaAggregateTarget != 0 && (m.Spec.InitParams.PgaAggregateTarget != m.Status.InitParams.PgaAggregateTarget)) || (m.Spec.InitParams.SgaTarget != 0 && (m.Spec.InitParams.SgaTarget != m.Status.InitParams.SgaTarget)) {
		log.Info("Executing alter sga pga command", "pga_size", m.Spec.InitParams.PgaAggregateTarget, "sga_size", m.Spec.InitParams.SgaTarget)
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.AlterSgaPgaCMD, m.Spec.InitParams.SgaTarget,
				m.Spec.InitParams.PgaAggregateTarget, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		// Notify the user about unsucessfull init-parameter value change
		if strings.Contains(out, "ORA-") {
			eventReason := "Invalid init-param value"
			eventMsg := "Unable to change the init-param as specified. Error log: \n" + out
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		}
		log.Info("AlterSgaPgaCpuCMD Output:" + out)
	}

	if (m.Spec.InitParams.CpuCount != 0) && (m.Status.InitParams.CpuCount != m.Spec.InitParams.CpuCount) {
		log.Info("Executing alter cpu count command", "cpuCount", m.Spec.InitParams.CpuCount)
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false,
			"bash", "-c", fmt.Sprintf(dbcommons.AlterCpuCountCMD, m.Spec.InitParams.CpuCount, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		if strings.Contains(out, "ORA-") {
			eventReason := "Invalid init-param value"
			eventMsg := "Unable to change the init-param as specified. Error log: \n" + out
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		}
		log.Info("AlterCpuCountCMD Output:" + out)
	}

	if (m.Spec.InitParams.Processes != 0) && (m.Status.InitParams.Processes != m.Spec.InitParams.Processes) {
		log.Info("Executing alter processes command", "processes", m.Spec.InitParams.Processes)
		// Altering 'Processes' needs database to be restarted
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.AlterProcessesCMD, m.Spec.InitParams.Processes, dbcommons.SQLPlusCLI,
				dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("AlterProcessesCMD Output:" + out)
	}
	return requeueN, nil
}

// #############################################################################
//
//	Update DB config params like FLASHBACK , FORCELOGGING , ARCHIVELOG
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateDBConfig(m *dbapi.SingleInstanceDatabase,
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("updateDBConfig", req.NamespacedName)

	m.Status.Status = dbcommons.StatusUpdating
	if err := r.Status().Update(ctx, m); err != nil {
		return requeueY, err
	}
	var forceLoggingStatus bool
	var flashBackStatus bool
	var archiveLogStatus bool
	var changeArchiveLog bool // True if switching ArchiveLog mode change is needed

	//#################################################################################################
	//                  CHECK FLASHBACK , ARCHIVELOG , FORCELOGGING
	//#################################################################################################

	flashBackStatus, archiveLogStatus, forceLoggingStatus, result := dbcommons.CheckDBConfig(readyPod, r, r.Config, ctx, req, m.Spec.Edition)
	if result.Requeue {
		m.Status.Status = dbcommons.StatusNotReady
		return result, nil
	}

	//#################################################################################################
	//                  TURNING FLASHBACK , ARCHIVELOG , FORCELOGGING TO TRUE
	//#################################################################################################

	if m.Spec.ArchiveLog != nil && *m.Spec.ArchiveLog && !archiveLogStatus {
		fraMountPath := getFraMountPath(m)
		fraRecoveryAreaSize := getFraRecoveryAreaSize(m)

		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf("mkdir -p %q", fraMountPath))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("CreateDbRecoveryDest Output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", buildSetDBRecoveryDestSQL(fraMountPath, fraRecoveryAreaSize), dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("SetDbRecoveryDest Output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.ArchiveLogTrueCMD, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("ArchiveLogTrue Output")
		log.Info(out)

	}

	if m.Spec.ForceLogging != nil && *m.Spec.ForceLogging && !forceLoggingStatus {
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.ForceLoggingTrueSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("ForceLoggingTrue Output")
		log.Info(out)

	}
	if m.Spec.FlashBack != nil && *m.Spec.FlashBack && !flashBackStatus {
		_, archiveLogStatus, _, result := dbcommons.CheckDBConfig(readyPod, r, r.Config, ctx, req, m.Spec.Edition)
		if result.Requeue {
			m.Status.Status = dbcommons.StatusNotReady
			return result, nil
		}
		if archiveLogStatus {
			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.FlashBackTrueSQL, dbcommons.SQLPlusCLI))
			if err != nil {
				log.Error(err, err.Error())
				m.Status.Status = dbcommons.StatusNotReady
				return requeueY, err
			}
			log.Info("FlashBackTrue Output")
			log.Info(out)

		} else {
			// Occurs when flashback is attempted to be turned on without turning on archiving first
			eventReason := "Database Check"
			eventMsg := "enable ArchiveLog to turn on Flashback"
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			log.Info(eventMsg)

			changeArchiveLog = true
		}
	}

	//#################################################################################################
	//                  TURNING FLASHBACK , ARCHIVELOG , FORCELOGGING TO FALSE
	//#################################################################################################

	if m.Spec.FlashBack != nil && !*m.Spec.FlashBack && flashBackStatus {
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.FlashBackFalseSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("FlashBackFalse Output")
		log.Info(out)
	}
	if m.Spec.ArchiveLog != nil && !*m.Spec.ArchiveLog && archiveLogStatus {
		flashBackStatus, _, _, result := dbcommons.CheckDBConfig(readyPod, r, r.Config, ctx, req, m.Spec.Edition)
		if result.Requeue {
			m.Status.Status = dbcommons.StatusNotReady
			return result, nil
		}
		if !flashBackStatus {

			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf(dbcommons.ArchiveLogFalseCMD, dbcommons.SQLPlusCLI))
			if err != nil {
				log.Error(err, err.Error())
				m.Status.Status = dbcommons.StatusNotReady
				return requeueY, err
			}
			log.Info("ArchiveLogFalse Output")
			log.Info(out)

		} else {
			// Occurs when archiving is attempted to be turned off without turning off flashback first
			eventReason := "Database Check"
			eventMsg := "turn off Flashback to disable ArchiveLog"
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			log.Info(eventMsg)

			changeArchiveLog = true
		}
	}
	if m.Spec.ForceLogging != nil && !*m.Spec.ForceLogging && forceLoggingStatus {
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.ForceLoggingFalseSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("ForceLoggingFalse Output")
		log.Info(out)
	}

	//#################################################################################################
	//                  CHECK FLASHBACK , ARCHIVELOG , FORCELOGGING
	//#################################################################################################

	flashBackStatus, archiveLogStatus, forceLoggingStatus, result = dbcommons.CheckDBConfig(readyPod, r, r.Config, ctx, req, m.Spec.Edition)
	if result.Requeue {
		m.Status.Status = dbcommons.StatusNotReady
		return result, nil
	}

	log.Info("Flashback", "Status :", flashBackStatus)
	log.Info("ArchiveLog", "Status :", archiveLogStatus)
	log.Info("ForceLog", "Status :", forceLoggingStatus)

	m.Status.ArchiveLog = strconv.FormatBool(archiveLogStatus)
	m.Status.ForceLogging = strconv.FormatBool(forceLoggingStatus)

	// If Flashback has turned from OFF to ON in this reconcile ,
	// Needs to restart the Non Ready Pods ( Delete old ones and create new ones )
	if m.Status.FlashBack == strconv.FormatBool(false) && flashBackStatus {

		// 	// call FindPods() to fetch pods all version/images of the same SIDB kind
		readyPod, replicasFound, available, _, err := dbcommons.FindPods(r, "", "", m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		// delete non ready Pods as flashback needs restart of pods to make sure failover works in sidbs with multiple replicas
		_, err = r.deletePods(ctx, req, m, available, readyPod, replicasFound, 1)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		return requeueN, err
	}

	m.Status.FlashBack = strconv.FormatBool(flashBackStatus)

	if !changeArchiveLog && ((m.Spec.FlashBack != nil && (flashBackStatus != *m.Spec.FlashBack)) ||
		(m.Spec.ArchiveLog != nil && (archiveLogStatus != *m.Spec.ArchiveLog)) || (m.Spec.ForceLogging != nil && (forceLoggingStatus != *m.Spec.ForceLogging))) {
		return requeueY, nil
	}
	return requeueN, nil
}

// #############################################################################
//
// # Update Single instance database resource status
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateSidbStatus(sidb *dbapi.SingleInstanceDatabase, sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("updateSidbStatus", req.NamespacedName)

	flashBackStatus, archiveLogStatus, forceLoggingStatus, result := dbcommons.CheckDBConfig(sidbReadyPod, r, r.Config, ctx, req, sidb.Spec.Edition)
	if result.Requeue {
		sidb.Status.Status = dbcommons.StatusNotReady
		return fmt.Errorf("could not check the database conifg of %s", sidb.Name)
	}

	log.Info("flashBack", "Status :", flashBackStatus, "Reconcile Step : ", "updateSidbStatus")
	log.Info("ArchiveLog", "Status :", archiveLogStatus, "Reconcile Step : ", "updateSidbStatus")
	log.Info("forceLogging", "Status :", forceLoggingStatus, "Reconcile Step : ", "updateSidbStatus")

	sidb.Status.ArchiveLog = strconv.FormatBool(archiveLogStatus)
	sidb.Status.ForceLogging = strconv.FormatBool(forceLoggingStatus)
	sidb.Status.FlashBack = strconv.FormatBool(flashBackStatus)

	cpuCount, pgaAggregateTarget, processes, sgaTarget, err := dbcommons.CheckDBInitParams(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		return err
	}
	sidbInitParams := dbapi.SingleInstanceDatabaseInitParams{
		SgaTarget:          sgaTarget,
		PgaAggregateTarget: pgaAggregateTarget,
		Processes:          processes,
		CpuCount:           cpuCount,
	}
	// log.Info("GetInitParamsSQL Output:" + out)

	sidb.Status.InitParams = sidbInitParams
	// sidb.Status.InitParams = sidb.Spec.InitParams

	// Get database role and update the status
	sidbRole, err := dbcommons.GetDatabaseRole(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		return err
	}
	log.Info("Database "+sidb.Name, "Database Role : ", sidbRole)
	sidb.Status.Role = sidbRole

	// Get database version and update the status
	version, err := dbcommons.GetDatabaseVersion(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		return err
	}
	log.Info("Database "+sidb.Name, "Database Version : ", version)
	sidb.Status.ReleaseUpdate = version

	dbMajorVersion, err := strconv.Atoi(strings.Split(sidb.Status.ReleaseUpdate, ".")[0])
	if err != nil {
		r.Log.Error(err, err.Error())
		return err
	}
	log.Info("Database "+sidb.Name, "Database Major Version : ", dbMajorVersion)

	// Checking if OEM is supported in the provided Database version
	if dbMajorVersion >= 23 {
		sidb.Status.OemExpressUrl = dbcommons.ValueUnavailable
	} else {
		if strings.TrimSpace(sidb.Status.OemExpressUrl) == "" {
			sidb.Status.OemExpressUrl = dbcommons.ValueUnavailable
		}
	}

	if sidb.Status.Role == "PRIMARY" && sidb.Status.DatafilesPatched != "true" {
		eventReason := "Datapatch Pending"
		eventMsg := "datapatch execution pending"
		r.Recorder.Eventf(sidb, corev1.EventTypeNormal, eventReason, eventMsg)
	}

	// update status to Ready after all operations succeed
	sidb.Status.Status = dbcommons.StatusReady

	if err := r.Status().Update(ctx, sidb); err != nil {
		return err
	}

	return nil
}

// #############################################################################
//
//	Update ORDS Status
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateORDSStatus(m *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) {

	if m.Status.OrdsReference == "" {
		return
	}
	n := &dbapi.OracleRestDataService{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Status.OrdsReference}, n)
	if err != nil {
		return
	}

	if n.Status.OrdsInstalled {
		// Update Status to Healthy/Unhealthy when SIDB turns Healthy/Unhealthy after ORDS is Installed
		n.Status.Status = m.Status.Status
		if err := r.Status().Update(ctx, n); err != nil {
			r.Log.Error(err, "failed to update ORDS status")
		}
		return
	}
}

// #############################################################################
//
//	Manage Finalizer to cleanup before deletion of SingleInstanceDatabase
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) manageSingleInstanceDatabaseDeletion(req ctrl.Request, ctx context.Context,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {
	log := r.Log.WithValues("manageSingleInstanceDatabaseDeletion", req.NamespacedName)

	// Check if the SingleInstanceDatabase instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isSingleInstanceDatabaseMarkedToBeDeleted := m.GetDeletionTimestamp() != nil
	if isSingleInstanceDatabaseMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(m, singleInstanceDatabaseFinalizer) {
			// Run finalization logic for singleInstanceDatabaseFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			result, err := r.cleanupSingleInstanceDatabase(req, ctx, m)
			if result.Requeue {
				return result, err
			}

			// Remove SingleInstanceDatabaseFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(m, singleInstanceDatabaseFinalizer)
			err = r.Update(ctx, m)
			if err != nil {
				log.Error(err, err.Error())
				return requeueY, err
			}
			log.Info("Successfully Removed SingleInstanceDatabase Finalizer")
		}
		return requeueY, errors.New("deletion pending")
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(m, singleInstanceDatabaseFinalizer) {
		controllerutil.AddFinalizer(m, singleInstanceDatabaseFinalizer)
		err := r.Update(ctx, m)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
	}
	return requeueN, nil
}

// #############################################################################
//
//	Finalization logic for singleInstanceDatabaseFinalizer
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) cleanupSingleInstanceDatabase(req ctrl.Request, ctx context.Context,
	m *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {
	log := r.Log.WithValues("cleanupSingleInstanceDatabase", req.NamespacedName)
	// Cleanup steps that the operator needs to do before the CR can be deleted.

	if m.Status.OrdsReference != "" {
		eventReason := "Cannot cleanup"
		eventMsg := "uninstall ORDS to clean this SIDB"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		m.Status.Status = dbcommons.StatusError
		return requeueY, nil
	}

	if m.Status.DgBroker != nil {
		eventReason := "Cannot Delete"
		eventMsg := "database cannot be deleted as it is present in a DataGuard Broker configuration"
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY, errors.New(eventMsg)
	}

	// call deletePods() with zero pods in avaiable and nil readyPod to delete all pods
	result, err := r.deletePods(ctx, req, m, []corev1.Pod{}, corev1.Pod{}, 0, 0)
	if result.Requeue {
		return result, err
	}

	for {
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{client.InNamespace(req.Namespace), client.MatchingLabels(dbcommons.GetLabelsForController("", req.Name))}

		if err := r.List(ctx, podList, listOpts...); err != nil {
			log.Error(err, "Failed to list pods of "+req.Name, "Namespace", req.Namespace)
			return requeueY, err
		}
		if len(podList.Items) == 0 {
			break
		}
		var podNames = ""
		for _, pod := range podList.Items {
			podNames += pod.Name + " "
		}
	}

	log.Info("Successfully cleaned up SingleInstanceDatabase")
	return requeueN, nil
}

// #############################################################################################
//
//	Manage conversion of singleinstancedatabase from PHYSICAL_STANDBY To SNAPSHOT_STANDBY
//
// #############################################################################################
func (r *SingleInstanceDatabaseReconciler) manageConvPhysicalToSnapshot(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("manageConvPhysicalToSnapshot", req.NamespacedName)
	var singleInstanceDatabase dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &singleInstanceDatabase); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("requested resource not found")
			return requeueY, nil
		}
		log.Error(err, err.Error())
		return requeueY, err
	}

	sidbReadyPod, err := GetDatabaseReadyPod(r, &singleInstanceDatabase, ctx, req)
	if err != nil {
		return requeueY, err
	}

	if sidbReadyPod.Name == "" {
		log.Info("No ready Pod for the requested singleinstancedatabase")
		return requeueY, nil
	}

	if singleInstanceDatabase.Spec.ConvertToSnapshotStandby {
		// Convert a PHYSICAL_STANDBY -> SNAPSHOT_STANDBY
		if singleInstanceDatabase.Status.Status != dbcommons.StatusPending {
			singleInstanceDatabase.Status.Status = dbcommons.StatusUpdating
		}

		if err := r.Status().Update(ctx, &singleInstanceDatabase); err != nil {
			return requeueY, err
		}
		if err := convertPhysicalStdToSnapshotStdDB(r, &singleInstanceDatabase, &sidbReadyPod, ctx, req); err != nil {
			singleInstanceDatabase.Status.Status = dbcommons.StatusPending
			if updateErr := r.Status().Update(ctx, &singleInstanceDatabase); updateErr != nil {
				log.Error(updateErr, "failed to update status after conversion failure")
			}
			switch err {
			case ErrNotPhysicalStandby:
				r.Recorder.Event(&singleInstanceDatabase, corev1.EventTypeWarning, "Error: Conversion to Snapshot Standby Not allowed", "Database not in physical standby role")
				log.Info("Error: Conversion to Snapshot Standby not allowed as database not in physical standby role")
				return requeueY, nil
			case ErrDBNotConfiguredWithDG:
				// cannot convert to snapshot database
				r.Recorder.Event(&singleInstanceDatabase, corev1.EventTypeWarning, "Error: Conversion to Snapshot Standby Not allowed", "Database is not configured with dataguard")
				log.Info("Conversion to Snapshot Standby not allowed as requested database is not configured with dataguard")
				return requeueY, nil
			case ErrFSFOEnabledForDGConfig:
				r.Recorder.Event(&singleInstanceDatabase, corev1.EventTypeWarning, "Error: Conversion to Snapshot Standby Not allowed", "Database is a FastStartFailover target")
				log.Info("Conversion to Snapshot Standby Not allowed as database is a FastStartFailover target")
				return requeueY, nil
			case ErrAdminPasswordSecretNotFound:
				r.Recorder.Event(&singleInstanceDatabase, corev1.EventTypeWarning, "Error: Admin Password", "Database admin password secret not found")
				log.Info("Database admin password secret not found")
				return requeueY, nil
			default:
				log.Error(err, err.Error())
				return requeueY, nil
			}
		}
		log.Info(fmt.Sprintf("Database %s converted to snapshot standby", singleInstanceDatabase.Name))
		singleInstanceDatabase.Status.ConvertToSnapshotStandby = true
		singleInstanceDatabase.Status.Status = dbcommons.StatusReady
		// Get database role and update the status
		sidbRole, err := dbcommons.GetDatabaseRole(sidbReadyPod, r, r.Config, ctx, req)
		if err != nil {
			return requeueN, err
		}
		log.Info("Database "+singleInstanceDatabase.Name, "Database Role : ", sidbRole)
		singleInstanceDatabase.Status.Role = sidbRole
		if err := r.Status().Update(ctx, &singleInstanceDatabase); err != nil {
			return requeueY, err
		}
	} else {
		// Convert a SNAPSHOT_STANDBY -> PHYSICAL_STANDBY
		singleInstanceDatabase.Status.Status = dbcommons.StatusUpdating
		if err := r.Status().Update(ctx, &singleInstanceDatabase); err != nil {
			return requeueY, err
		}
		if err := convertSnapshotStdToPhysicalStdDB(r, &singleInstanceDatabase, &sidbReadyPod, ctx, req); err != nil {
			switch err {
			default:
				r.Log.Error(err, err.Error())
				return requeueY, nil
			}
		}
		singleInstanceDatabase.Status.ConvertToSnapshotStandby = false
		singleInstanceDatabase.Status.Status = dbcommons.StatusReady
		// Get database role and update the status
		sidbRole, err := dbcommons.GetDatabaseRole(sidbReadyPod, r, r.Config, ctx, req)
		if err != nil {
			return requeueN, err
		}
		log.Info("Database "+singleInstanceDatabase.Name, "Database Role : ", sidbRole)
		singleInstanceDatabase.Status.Role = sidbRole
		if err := r.Status().Update(ctx, &singleInstanceDatabase); err != nil {
			return requeueY, err
		}
	}

	return requeueN, nil
}

func convertPhysicalStdToSnapshotStdDB(r *SingleInstanceDatabaseReconciler, singleInstanceDatabase *dbapi.SingleInstanceDatabase, sidbReadyPod *corev1.Pod, ctx context.Context, req ctrl.Request) error {
	log := r.Log.WithValues("convertPhysicalStdToSnapshotStdDB", req.NamespacedName)
	log.Info(fmt.Sprintf("Checking the role %s database i.e %s", singleInstanceDatabase.Name, singleInstanceDatabase.Status.Role))
	if singleInstanceDatabase.Status.Role != "PHYSICAL_STANDBY" {
		return ErrNotPhysicalStandby
	}

	var dataguardBroker dbapi.DataguardBroker
	log.Info(fmt.Sprintf("Checking if the database %s is configured with dgbroker or not ?", singleInstanceDatabase.Name))
	if singleInstanceDatabase.Status.DgBroker != nil {
		if err := r.Get(ctx, types.NamespacedName{Namespace: singleInstanceDatabase.Namespace, Name: *singleInstanceDatabase.Status.DgBroker}, &dataguardBroker); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Resource not found")
				return errors.New("Dataguardbroker resource not found")
			}
			return err
		}
		log.Info(fmt.Sprintf("database %s is configured with dgbroker %s", singleInstanceDatabase.Name, *singleInstanceDatabase.Status.DgBroker))
		if fastStartFailoverStatus, _ := strconv.ParseBool(dataguardBroker.Status.FastStartFailover); fastStartFailoverStatus {
			// not allowed to convert to snapshot standby
			return ErrFSFOEnabledForDGConfig
		}
	} else {
		// cannot convert to snapshot database
		return ErrDBNotConfiguredWithDG
	}

	// get singleinstancedatabase ready pod
	// execute the dgmgrl command for conversion to snapshot database
	// Exception handling
	// Get Admin password for current primary database
	var adminPasswordSecret corev1.Secret
	if err := r.Get(context.TODO(), types.NamespacedName{Name: GetAdminPasswordSecretName(singleInstanceDatabase), Namespace: singleInstanceDatabase.Namespace}, &adminPasswordSecret); err != nil {
		return err
	}
	var adminPassword string = string(adminPasswordSecret.Data[GetAdminPasswordSecretFileName(singleInstanceDatabase)])

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSidbSid' db to make 'targetSidbSid' db primary
	if _, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c", fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword)); err != nil {
		return err
	}

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c", fmt.Sprintf("dgmgrl sys@%s \"convert database %s to snapshot standby;\" < admin.pwd", dataguardBroker.Status.PrimaryDatabase, singleInstanceDatabase.Status.Sid))
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Convert to snapshot standby command output \n %s", out))

	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c", fmt.Sprintf("echo -e  \"alter pluggable database %s open;\"  | %s", singleInstanceDatabase.Status.Pdbname, dbcommons.SQLPlusCLI))
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Open pluggable databases output \n %s", out))

	return nil
}

func convertSnapshotStdToPhysicalStdDB(r *SingleInstanceDatabaseReconciler, singleInstanceDatabase *dbapi.SingleInstanceDatabase, sidbReadyPod *corev1.Pod, ctx context.Context, req ctrl.Request) error {
	log := r.Log.WithValues("convertSnapshotStdToPhysicalStdDB", req.NamespacedName)

	var dataguardBroker dbapi.DataguardBroker
	if err := r.Get(ctx, types.NamespacedName{Namespace: singleInstanceDatabase.Namespace, Name: *singleInstanceDatabase.Status.DgBroker}, &dataguardBroker); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.New("dataguardbroker resource not found")
		}
		return err
	}

	var adminPasswordSecret corev1.Secret
	if err := r.Get(context.TODO(), types.NamespacedName{Name: GetAdminPasswordSecretName(singleInstanceDatabase), Namespace: singleInstanceDatabase.Namespace}, &adminPasswordSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrAdminPasswordSecretNotFound
		}
		return err
	}
	var adminPassword string = string(adminPasswordSecret.Data[GetAdminPasswordSecretFileName(singleInstanceDatabase)])

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSidbSid' db to make 'targetSidbSid' db primary
	_, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		return err
	}
	log.Info("Converting snapshot standby to physical standby")
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c", fmt.Sprintf("dgmgrl sys@%s \"convert database %s to physical standby;\" < admin.pwd", dataguardBroker.Status.PrimaryDatabase, singleInstanceDatabase.Status.Sid))
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info(fmt.Sprintf("Database %s converted to physical standby \n %s", singleInstanceDatabase.Name, out))
	log.Info("opening the PDB for the database")
	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c", fmt.Sprintf("echo -e  \"alter pluggable database %s open;\"  | %s", singleInstanceDatabase.Status.Pdbname, dbcommons.SQLPlusCLI))
	if err != nil {
		r.Log.Error(err, err.Error())
		return err
	}
	log.Info(fmt.Sprintf("PDB open command output %s", out))

	return nil
}

// #############################################################################
//
//	SetupWithManager sets up the controller with the Manager
//
// #############################################################################
// SetupWithManager sets up the SingleInstanceDatabase controller with the manager.
func (r *SingleInstanceDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.SingleInstanceDatabase{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of SingleInstanceDatabase Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

// #############################################################################
//
//	Check primary database status
//
// #############################################################################
// CheckPrimaryDatabaseStatus validates that the primary database is in READY status.
func CheckPrimaryDatabaseStatus(p *dbapi.SingleInstanceDatabase) error {

	if p.Status.Status != dbcommons.StatusReady {
		return fmt.Errorf("referred primary database %v is NOT READY", p.Name)
	}
	return nil
}

// #############################################################################
//
//	Check if refered database is the primary database
//
// #############################################################################
// CheckDatabaseRoleAsPrimary validates that the referenced database role is PRIMARY.
func CheckDatabaseRoleAsPrimary(p *dbapi.SingleInstanceDatabase) error {

	if strings.ToUpper(p.Status.Role) != "PRIMARY" {
		return fmt.Errorf("referred database %v is not in PRIMARY role", p.Name)
	}
	return nil
}

// #############################################################################
//
//	Get ready pod for the singleinstancedatabase resource
//
// #############################################################################
// GetDatabaseReadyPod returns a ready database pod for the given SIDB.
func GetDatabaseReadyPod(r client.Reader, d *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (corev1.Pod, error) {

	dbReadyPod, _, _, _, err := dbcommons.FindPods(r, d.Spec.Image.Version,
		d.Spec.Image.PullFrom, d.Name, d.Namespace, ctx, req)

	return dbReadyPod, err
}

// #############################################################################
//
//	Get admin password for singleinstancedatabase
//
// #############################################################################
// GetDatabaseAdminPassword reads the admin password from the configured secret.
func GetDatabaseAdminPassword(r client.Reader, d *dbapi.SingleInstanceDatabase, ctx context.Context) (string, error) {

	adminPasswordSecret := &corev1.Secret{}
	adminPassword := ""
	err := r.Get(ctx, types.NamespacedName{Name: GetAdminPasswordSecretName(d), Namespace: d.Namespace}, adminPasswordSecret)
	if err != nil {
		return adminPassword, err
	}
	adminPassword = string(adminPasswordSecret.Data[GetAdminPasswordSecretFileName(d)])
	return adminPassword, nil
}

// #############################################################################
//
//	Validate primary singleinstancedatabase admin password
//
// #############################################################################
// ValidatePrimaryDatabaseAdminPassword validates SYS admin password against the primary database.
func ValidatePrimaryDatabaseAdminPassword(
	r *SingleInstanceDatabaseReconciler,
	p *dbapi.SingleInstanceDatabase,
	adminPassword string,
	ctx context.Context,
	req ctrl.Request,
) error {

	dbReadyPod, err := GetDatabaseReadyPod(r, p, ctx, req)
	if err != nil {
		r.Log.Error(err, "failed to get ready pod for primary database password validation")
		return err
	}

	sqlCmd := fmt.Sprintf(`sqlplus -s "sys/%s as sysdba" <<'EOF'
show user;
exit;
EOF`, adminPassword)

	r.Log.Info(
		"Validating primary database admin password",
		"database", p.Name,
		"pod", dbReadyPod.Name,
		"namespace", dbReadyPod.Namespace,
	)

	out, err := dbcommons.ExecCommand(
		r,
		r.Config,
		dbReadyPod.Name,
		dbReadyPod.Namespace,
		"",
		ctx,
		req,
		true,
		"bash",
		"-c",
		sqlCmd,
	)
	if err != nil {
		r.Log.Error(err, "failed to execute primary database password validation command", "output", out)
		return err
	}

	r.Log.Info("primary database password validation command output", "output", out)

	if strings.Contains(out, `USER is "SYS"`) {
		r.Log.Info("validated primary database admin password successfully")
		return nil
	}

	if strings.Contains(out, "ORA-01017") {
		return fmt.Errorf("primary database admin password validation failed: ORA-01017 invalid username/password")
	}

	return fmt.Errorf("primary database admin password validation failed, output: %s", out)
}

// #############################################################################
//
//	Validate refered primary database db params are all enabled
//
// #############################################################################
// ValidateDatabaseConfiguration ensures required DB modes are enabled for Data Guard.
func ValidateDatabaseConfiguration(p *dbapi.SingleInstanceDatabase) error {
	var missingModes []string
	if p.Status.ArchiveLog == "false" {
		missingModes = append(missingModes, "ArchiveLog")
	}
	if p.Status.FlashBack == "false" {
		missingModes = append(missingModes, "FlashBack")
	}
	if p.Status.ForceLogging == "false" {
		missingModes = append(missingModes, "ForceLogging")
	}
	if p.Status.ArchiveLog == "false" || p.Status.FlashBack == "false" || p.Status.ForceLogging == "false" {
		return fmt.Errorf("%v modes are not enabled in the primary database %v", strings.Join(missingModes, ","), p.Name)
	}
	return nil
}

// #############################################################################
//
//	Validate refered primary database for standby sidb creation
//
// #############################################################################
// ValidatePrimaryDatabaseForStandbyCreation validates primary readiness for standby creation.
func ValidatePrimaryDatabaseForStandbyCreation(
	r *SingleInstanceDatabaseReconciler,
	stdby *dbapi.SingleInstanceDatabase,
	primary *dbapi.SingleInstanceDatabase,
	ctx context.Context,
	req ctrl.Request,
) error {

	log := r.Log.WithValues("ValidatePrimaryDatabase", req.NamespacedName)

	if stdby.Status.DatafilesCreated == "true" {
		return nil
	}

	log.Info(fmt.Sprintf("Checking primary database %s status...", primary.Name))
	err := CheckPrimaryDatabaseStatus(primary)
	if err != nil {
		stdby.Status.Status = dbcommons.StatusPending
		return err
	}

	log.Info("Checking for referred database role...")
	err = CheckDatabaseRoleAsPrimary(primary)
	if err != nil {
		stdby.Status.Status = dbcommons.StatusError
		return err
	}

	r.Recorder.Eventf(stdby, corev1.EventTypeNormal, "Validation", "Primary database is ready")

	adminPassword, err := GetDatabaseAdminPassword(r, primary, ctx)
	if err != nil {
		stdby.Status.Status = dbcommons.StatusError
		return err
	}

	log.Info(fmt.Sprintf("Validating admin password for the primary Database %s...", primary.Name))
	err = ValidatePrimaryDatabaseAdminPassword(r, primary, adminPassword, ctx, req)
	if err != nil {
		stdby.Status.Status = dbcommons.StatusError
		return err
	}

	log.Info(fmt.Sprintf("Validating primary database %s configuration...", primary.Name))
	err = ValidateDatabaseConfiguration(primary)
	if err != nil {
		r.Recorder.Eventf(stdby, corev1.EventTypeWarning, "Spec Error", err.Error())
		stdby.Status.Status = dbcommons.StatusError
		return err
	}

	r.Recorder.Eventf(stdby, corev1.EventTypeNormal, "Validation", "Successfully validated the primary database admin password and configuration")
	return nil
}

// #############################################################################
//
//	Get total database pods for singleinstancedatabase
//
// #############################################################################
// GetTotalDatabasePods returns the total number of database pods for the SIDB.
func GetTotalDatabasePods(r client.Reader, d *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (int, error) {
	_, totalPods, _, _, err := dbcommons.FindPods(r, d.Spec.Image.Version,
		d.Spec.Image.PullFrom, d.Name, d.Namespace, ctx, req)

	return totalPods, err
}

func getTnsFilePathBySID(sid string) string {
	return fmt.Sprintf("/opt/oracle/oradata/dbconfig/%s/tnsnames.ora", strings.ToUpper(strings.TrimSpace(sid)))
}

func normalizeTNSAliasProtocol(protocol string) string {
	normalized := strings.ToUpper(strings.TrimSpace(protocol))
	if normalized == string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS) {
		return string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS)
	}
	return string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP)
}

func defaultPortForProtocol(protocol string) int {
	if normalizeTNSAliasProtocol(protocol) == string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS) {
		return int(dbcommons.CONTAINER_TCPS_PORT)
	}
	return int(dbcommons.CONTAINER_LISTENER_PORT)
}

func buildLegacyTnsAliasEntry(alias, host string, port int, serviceName string, protocol string, sslDN string) string {
	protocol = normalizeTNSAliasProtocol(protocol)
	entry := fmt.Sprintf(`
%s =
(DESCRIPTION =
  (ADDRESS = (PROTOCOL = %s)(HOST = %s)(PORT = %d))
  (CONNECT_DATA =
    (SERVER = DEDICATED)
    (SERVICE_NAME = %s)
  )`, strings.ToUpper(strings.TrimSpace(alias)), protocol, strings.TrimSpace(host), port, strings.ToUpper(strings.TrimSpace(serviceName)))

	if protocol == string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS) && strings.TrimSpace(sslDN) != "" {
		entry += fmt.Sprintf(`
  (SECURITY =
    (SSL_SERVER_DN_MATCH = YES)
    (SSL_SERVER_CERT_DN = %s)
  )`, strings.TrimSpace(sslDN))
	}

	entry += `
)
`
	return entry
}

func upsertTnsAliasInPod(r *SingleInstanceDatabaseReconciler, pod corev1.Pod, ctx context.Context, req ctrl.Request, tnsFile, alias, host string, port int, serviceName string, protocol, sslDN string) error {
	alias = strings.ToUpper(strings.TrimSpace(alias))
	host = strings.TrimSpace(host)
	serviceName = strings.ToUpper(strings.TrimSpace(serviceName))
	protocol = normalizeTNSAliasProtocol(protocol)
	sslDN = strings.TrimSpace(sslDN)
	if alias == "" || host == "" || serviceName == "" {
		return fmt.Errorf("alias, host and serviceName are required for tns upsert")
	}
	if port <= 0 {
		return fmt.Errorf("port must be > 0 for tns upsert")
	}

	args := fmt.Sprintf("--file %q --alias %q --upsert --host %q --port %d --service %q --protocol %q --strict-dedupe",
		tnsFile, alias, host, port, serviceName, protocol)
	if protocol == string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS) && sslDN != "" {
		args += fmt.Sprintf(" --ssl-server-dn %q", sslDN)
	}

	legacyEntry := buildLegacyTnsAliasEntry(alias, host, port, serviceName, protocol, sslDN)
	legacyCmd := fmt.Sprintf("if ! grep -Eq '^[[:space:]]*%s[[:space:]]*=' %q; then echo -e %q | cat >> %q; fi",
		alias, tnsFile, legacyEntry, tnsFile)

	cmd := fmt.Sprintf("if [ -x \"$ORACLE_BASE/manageTnsAliases.sh\" ]; then \"$ORACLE_BASE/manageTnsAliases.sh\" %s; else %s; fi", args, legacyCmd)

	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", cmd)
	if err != nil {
		return fmt.Errorf("failed to upsert TNS alias %s in %s: %w", alias, tnsFile, err)
	}
	r.Log.Info("TNS alias upsert output", "alias", alias, "tnsFile", tnsFile, "output", out)
	return nil
}

func deleteTnsAliasInPod(r *SingleInstanceDatabaseReconciler, pod corev1.Pod, ctx context.Context, req ctrl.Request, tnsFile, alias string) error {
	alias = strings.ToUpper(strings.TrimSpace(alias))
	if alias == "" {
		return nil
	}
	args := fmt.Sprintf("--file %q --alias %q --delete", tnsFile, alias)
	legacyCmd := fmt.Sprintf("if grep -Eq '^[[:space:]]*%s[[:space:]]*=' %q; then sed -i -E '/^[[:space:]]*%s[[:space:]]*=/{:a;N;/\\n\\)/!ba;d;}' %q; fi",
		alias, tnsFile, alias, tnsFile)
	cmd := fmt.Sprintf("if [ -x \"$ORACLE_BASE/manageTnsAliases.sh\" ]; then \"$ORACLE_BASE/manageTnsAliases.sh\" %s; else %s; fi", args, legacyCmd)
	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", cmd)
	if err != nil {
		return fmt.Errorf("failed to delete TNS alias %s in %s: %w", alias, tnsFile, err)
	}
	r.Log.Info("TNS alias delete output", "alias", alias, "tnsFile", tnsFile, "output", out)
	return nil
}

func readManagedTNSAliasesStateInPod(r *SingleInstanceDatabaseReconciler, pod corev1.Pod, ctx context.Context, req ctrl.Request, stateFile string) ([]string, error) {
	cmd := fmt.Sprintf("if [ -f %q ]; then cat %q; fi", stateFile, stateFile)
	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", cmd)
	if err != nil {
		return nil, err
	}
	lines, _ := dbcommons.StringToLines(out)
	aliases := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for i := range lines {
		name := strings.ToUpper(strings.TrimSpace(lines[i]))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		aliases = append(aliases, name)
	}
	sort.Strings(aliases)
	return aliases, nil
}

func writeManagedTNSAliasesStateInPod(r *SingleInstanceDatabaseReconciler, pod corev1.Pod, ctx context.Context, req ctrl.Request, stateFile string, aliases []string) error {
	sort.Strings(aliases)
	content := strings.Join(aliases, "\n")
	if content != "" {
		content += "\n"
	}
	cmd := fmt.Sprintf("cat > %q <<'EOF'\n%sEOF", stateFile, content)
	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", cmd)
	if err != nil {
		return fmt.Errorf("failed to write managed tns aliases state file %s: %w", stateFile, err)
	}
	r.Log.Info("Managed TNS aliases state updated", "stateFile", stateFile, "output", out)
	return nil
}

func syncConfiguredTNSAliasesInPod(r *SingleInstanceDatabaseReconciler, owner *dbapi.SingleInstanceDatabase, pod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	if owner == nil {
		return nil
	}
	tnsFile := getTnsFilePathBySID(owner.Spec.Sid)
	stateFile := tnsFile + ".operator_aliases"

	desired := make(map[string]dbapi.SingleInstanceDatabaseTNSAlias, len(owner.Spec.TNSAliases))
	desiredNames := make([]string, 0, len(owner.Spec.TNSAliases))
	for i := range owner.Spec.TNSAliases {
		item := owner.Spec.TNSAliases[i]
		name := strings.ToUpper(strings.TrimSpace(item.Name))
		if name == "" {
			continue
		}
		desired[name] = item
		desiredNames = append(desiredNames, name)
	}
	sort.Strings(desiredNames)

	for _, alias := range desiredNames {
		item := desired[alias]
		port := item.Port
		if port == 0 {
			port = defaultPortForProtocol(string(item.Protocol))
		}
		if err := upsertTnsAliasInPod(
			r,
			pod,
			ctx,
			req,
			tnsFile,
			alias,
			item.Host,
			port,
			item.ServiceName,
			string(item.Protocol),
			item.SSLServerDN,
		); err != nil {
			return err
		}
	}

	previousNames, err := readManagedTNSAliasesStateInPod(r, pod, ctx, req, stateFile)
	if err != nil {
		return err
	}
	desiredSet := map[string]struct{}{}
	for _, name := range desiredNames {
		desiredSet[name] = struct{}{}
	}
	for _, oldAlias := range previousNames {
		if _, keep := desiredSet[oldAlias]; keep {
			continue
		}
		if err := deleteTnsAliasInPod(r, pod, ctx, req, tnsFile, oldAlias); err != nil {
			return err
		}
	}

	return writeManagedTNSAliasesStateInPod(r, pod, ctx, req, stateFile, desiredNames)
}

func resolveTNSAliasSettings(owner *dbapi.SingleInstanceDatabase, defaultAlias, defaultHost string, defaultPort int, defaultService string) (alias, host string, port int, serviceName, protocol, sslDN string) {
	alias = strings.ToUpper(strings.TrimSpace(defaultAlias))
	host = strings.TrimSpace(defaultHost)
	port = defaultPort
	serviceName = strings.ToUpper(strings.TrimSpace(defaultService))
	protocol = string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP)
	sslDN = ""

	if owner == nil {
		return
	}
	for i := range owner.Spec.TNSAliases {
		item := owner.Spec.TNSAliases[i]
		if !strings.EqualFold(strings.TrimSpace(item.Name), defaultAlias) {
			continue
		}
		alias = strings.ToUpper(strings.TrimSpace(item.Name))
		if v := strings.TrimSpace(item.Host); v != "" {
			host = v
		}
		if v := strings.TrimSpace(item.ServiceName); v != "" {
			serviceName = strings.ToUpper(v)
		}
		protocol = normalizeTNSAliasProtocol(string(item.Protocol))
		if item.Port > 0 {
			port = item.Port
		} else if port == 0 {
			port = defaultPortForProtocol(protocol)
		}
		if v := strings.TrimSpace(item.SSLServerDN); v != "" {
			sslDN = v
		}
		return
	}

	if port == 0 {
		port = defaultPortForProtocol(protocol)
	}

	return
}

// #############################################################################
//
//	Set tns names for primary database for dataguard configuraion
//
// #############################################################################
// SetupTnsNamesPrimaryForDG configures primary tnsnames alias entries for Data Guard.
func SetupTnsNamesPrimaryForDG(r *SingleInstanceDatabaseReconciler, p *dbapi.SingleInstanceDatabase, s *dbapi.SingleInstanceDatabase,
	primaryReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	tnsFile := getTnsFilePathBySID(p.Spec.Sid)
	alias, host, port, serviceName, protocol, sslDN := resolveTNSAliasSettings(p, s.Spec.Sid, s.Name, 1521, s.Spec.Sid)
	return upsertTnsAliasInPod(r, primaryReadyPod, ctx, req, tnsFile, alias, host, port, serviceName, protocol, sslDN)
}

// #############################################################################
//
//	Restarting listners in database
//
// #############################################################################
// RestartListenerInDatabase restarts the database listener inside the target pod.
func RestartListenerInDatabase(r *SingleInstanceDatabaseReconciler, primaryReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	r.Log.Info("Restarting listener in the database through pod", "primary database pod name", primaryReadyPod.Name)
	out, err := dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", "lsnrctl stop && lsnrctl start")
	if err != nil {
		return fmt.Errorf("unable to restart listener in the database through pod %v", primaryReadyPod.Name)
	}
	r.Log.Info("Listener restart output")
	r.Log.Info(out)
	return nil
}

// #############################################################################
//
//	Setup primary listener for dataguard configuration
//
// #############################################################################
// SetupListenerPrimaryForDG updates primary listener configuration for Data Guard.
func SetupListenerPrimaryForDG(r *SingleInstanceDatabaseReconciler, p *dbapi.SingleInstanceDatabase, s *dbapi.SingleInstanceDatabase,
	primaryReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {

	out, err := dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("cat /opt/oracle/oradata/dbconfig/%s/listener.ora ", strings.ToUpper(p.Spec.Sid)))
	if err != nil {
		return fmt.Errorf("unable to obtain contents of listener.ora in primary database %v", p.Name)
	}
	r.Log.Info("listener.ora Output")
	r.Log.Info(out)

	if strings.Contains(out, strings.ToUpper(p.Spec.Sid)+"_DGMGRL") {
		r.Log.Info("LISTENER.ORA ALREADY HAS " + p.Spec.Sid + "_DGMGRL ENTRY IN SID_LIST_LISTENER ")
	} else {
		out, err = dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | cat > /opt/oracle/oradata/dbconfig/%s/listener.ora ", dbcommons.ListenerEntry, strings.ToUpper(p.Spec.Sid)))
		if err != nil {
			return fmt.Errorf("unable to modify listener.ora in the primary database %v", p.Name)
		}
		r.Log.Info("Modifying listener.ora Output")
		r.Log.Info(out)

		err = RestartListenerInDatabase(r, primaryReadyPod, ctx, req)
		if err != nil {
			return err
		}

	}
	return nil
}

// #############################################################################
//
//	Setup init parameters of primary database for dataguard configuration
//
// #############################################################################
// SetupInitParamsPrimaryForDG sets primary init parameters required for Data Guard.
func SetupInitParamsPrimaryForDG(r *SingleInstanceDatabaseReconciler, primaryReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	r.Log.Info("Running StandbyDatabasePrerequisitesSQL in the primary database")
	out, err := dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.StandbyDatabasePrerequisitesSQL, dbcommons.SQLPlusCLI))
	if err != nil {
		return fmt.Errorf("unable to run StandbyDatabasePrerequisitesSQL in primary database")
	}
	r.Log.Info("StandbyDatabasePrerequisites Output")
	r.Log.Info(out)
	return nil
}

// #############################################################################
//
//	Setup primary database for standby singleinstancedatabase
//
// #############################################################################
// SetupPrimaryDatabase prepares the primary database for standby creation.
func SetupPrimaryDatabase(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
	primary *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("SetupPrimaryDatabase", req.NamespacedName)

	totalStandbyPods, err := GetTotalDatabasePods(r, stdby, ctx, req)
	if err != nil {
		return err
	}
	// NO need to setup primary database if standby database pods are initialized
	if totalStandbyPods > 0 {
		return nil
	}

	primaryDbReadyPod, err := GetDatabaseReadyPod(r, primary, ctx, req)
	if err != nil {
		return err
	}

	log.Info("Setting up tnsnames.ora in primary database", "primaryDatabase", primary.Name)
	workflow := dgsidb.NewStandbyWorkflow(dgsidb.StandbyWorkflowOptions{
		EnsureBrokerFilesAndStart: func() error {
			log.Info("Setting up tnsnames.ora in primary database", "primaryDatabase", primary.Name)
			return SetupTnsNamesPrimaryForDG(r, primary, stdby, primaryDbReadyPod, ctx, req)
		},
		RunPrimaryPrerequisites: func() error {
			log.Info("Setting up listener.ora in primary database", "primaryDatabase", primary.Name)
			return SetupListenerPrimaryForDG(r, primary, stdby, primaryDbReadyPod, ctx, req)
		},
		EnsureStandbyRedoLogs: func() error {
			log.Info("Setting up some InitParams for DG in primary database", "primaryDatabase", primary.Name)
			return SetupInitParamsPrimaryForDG(r, primaryDbReadyPod, ctx, req)
		},
	})
	if e := dataguardcommon.RunStandbyDGBrokerWorkflow(workflow); e != nil {
		if stepErr, ok := e.(*dataguardcommon.StepError); ok {
			return fmt.Errorf("%s: %w", sidbStandbySetupStepMessage(stepErr.Step), stepErr.Err)
		}
		return e
	}

	return nil

}

func sidbStandbySetupStepMessage(step dataguardcommon.WorkflowStep) string {
	switch step {
	case dataguardcommon.StepEnsureBrokerFilesAndStart:
		return "failed to setup tnsnames for standby configuration"
	case dataguardcommon.StepRunPrimaryPrerequisites:
		return "failed to setup listener for standby configuration"
	case dataguardcommon.StepEnsureStandbyRedoLogs:
		return "failed to setup primary init parameters for standby configuration"
	default:
		return "failed to setup standby workflow"
	}
}

// #############################################################################
//
//	Get all pdbs in a singleinstancedatabase
//
// #############################################################################
// GetAllPdbInDatabase returns all PDB names from the target database.
func GetAllPdbInDatabase(r *SingleInstanceDatabaseReconciler, dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ([]string, error) {
	var pdbs []string
	out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba", dbcommons.GetPdbsSQL))
	if err != nil {
		r.Log.Error(err, err.Error())
		return pdbs, err
	}
	r.Log.Info("GetPdbsSQL Output")
	r.Log.Info(out)

	pdbs, _ = dbcommons.StringToLines(out)
	return pdbs, nil
}

// #############################################################################
//
//	Setup tnsnames.ora for all the pdb list in the singleinstancedatabase
//
// #############################################################################
// SetupTnsNamesForPDBListInDatabase upserts TNS aliases for a list of PDBs.
func SetupTnsNamesForPDBListInDatabase(r *SingleInstanceDatabaseReconciler, d *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request, pdbList []string) error {
	tnsFile := getTnsFilePathBySID(d.Spec.Sid)
	localHost := "0.0.0.0"
	for _, pdb := range pdbList {
		pdb = strings.TrimSpace(pdb)
		if pdb == "" {
			continue
		}
		if err := upsertTnsAliasInPod(r, dbReadyPod, ctx, req, tnsFile, pdb, localHost, 1521, pdb, string(dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP), ""); err != nil {
			return err
		}
	}

	return nil
}

// #############################################################################
//
//	Setup tnsnames.ora in standby database for primary singleinstancedatabase
//
// #############################################################################
// SetupPrimaryDBTnsNamesInStandby configures primary DB aliases inside standby tnsnames.ora.
func SetupPrimaryDBTnsNamesInStandby(r *SingleInstanceDatabaseReconciler, s *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	tnsFile := getTnsFilePathBySID(s.Spec.Sid)
	defaultAlias := GetPrimaryDatabaseSid(s, nil)
	defaultHost := GetPrimaryDatabaseHost(s, nil)
	defaultPort := GetPrimaryDatabasePort(s)
	if defaultPort == 0 {
		defaultPort = 1521
	}
	alias, host, port, serviceName, protocol, sslDN := resolveTNSAliasSettings(s, defaultAlias, defaultHost, defaultPort, defaultAlias)
	return upsertTnsAliasInPod(r, dbReadyPod, ctx, req, tnsFile, alias, host, port, serviceName, protocol, sslDN)
}

// #############################################################################
//
//	Enabling flashback in singleinstancedatabase
//
// #############################################################################
// EnableFlashbackInDatabase enables flashback mode for the target database.
func EnableFlashbackInDatabase(r *SingleInstanceDatabaseReconciler, dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.FlashBackTrueSQL, dbcommons.GetSqlClient("enterprise")))
	if err != nil {
		return err
	}
	r.Log.Info("FlashBackTrue Output")
	r.Log.Info(out)
	return nil
}

// #############################################################################
//
//	setup standby database
//
// #############################################################################
// SetupStandbyDatabase performs standby post-creation configuration steps.
func SetupStandbyDatabase(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
	primary *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) error {

	if IsExternalPrimaryDatabase(stdby) {
		return SetupStandbyDatabaseForExternalPrimary(r, stdby, ctx, req)
	}

	return SetupStandbyDatabaseForLocalPrimary(r, stdby, primary, ctx, req)
}

// SetupStandbyDatabaseForLocalPrimary configures standby using an in-cluster primary reference.
func SetupStandbyDatabaseForLocalPrimary(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
	primary *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) error {

	primaryReadyPod, err := GetDatabaseReadyPod(r, primary, ctx, req)
	if err != nil {
		return err
	}
	r.Log.Info("Primary DB Name: " + primaryReadyPod.Name)

	stdbyReadyPod, err := GetDatabaseReadyPod(r, stdby, ctx, req)
	if err != nil {
		return err
	}

	r.Log.Info("Getting the list of all pdbs in primary database")
	pdbListPrimary, err := GetAllPdbInDatabase(r, primaryReadyPod, ctx, req)
	if err != nil {
		return err
	}

	r.Log.Info("Setting up tnsnames in standby database for the pdbs of primary database")
	err = SetupTnsNamesForPDBListInDatabase(r, stdby, stdbyReadyPod, ctx, req, pdbListPrimary)
	if err != nil {
		return err
	}

	r.Log.Info("Setting up tnsnames entry for primary database in standby database")
	err = SetupPrimaryDBTnsNamesInStandby(r, stdby, stdbyReadyPod, ctx, req)
	if err != nil {
		return err
	}

	r.Log.Info("Setting up listener in the standby database")
	err = SetupListenerForDGOnDatabase(r, stdby, stdbyReadyPod, ctx, req)
	if err != nil {
		return err
	}

	flashBackStatus, _, _, result := dbcommons.CheckDBConfig(stdbyReadyPod, r, r.Config, ctx, req, stdby.Spec.Edition)
	if result.Requeue {
		return fmt.Errorf("error in obtaining the Database Config status")
	}
	if !flashBackStatus {
		r.Log.Info("Setting up flashback mode in the standby database")
		err = EnableFlashbackInDatabase(r, stdbyReadyPod, ctx, req)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetupStandbyDatabaseForExternalPrimary configures standby using external primary details.
func SetupStandbyDatabaseForExternalPrimary(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request) error {

	stdbyReadyPod, err := GetDatabaseReadyPod(r, stdby, ctx, req)
	if err != nil {
		return err
	}

	r.Log.Info("Setting up tnsnames entry for external primary database in standby database")
	err = SetupExternalPrimaryDBTnsNamesInStandby(r, stdby, stdbyReadyPod, ctx, req)
	if err != nil {
		return err
	}

	if pdbName := GetPrimaryDatabasePdbName(stdby, nil); pdbName != "" {
		r.Log.Info("Setting up tnsnames entry for external primary pdb in standby database")
		err = SetupTnsNamesForPDBListInDatabase(r, stdby, stdbyReadyPod, ctx, req, []string{pdbName})
		if err != nil {
			return err
		}
	}

	r.Log.Info("Setting up listener in the standby database")
	err = SetupListenerForDGOnDatabase(r, stdby, stdbyReadyPod, ctx, req)
	if err != nil {
		return err
	}

	flashBackStatus, _, _, result := dbcommons.CheckDBConfig(stdbyReadyPod, r, r.Config, ctx, req, stdby.Spec.Edition)
	if result.Requeue {
		return fmt.Errorf("error in obtaining the Database Config status")
	}
	if !flashBackStatus {
		r.Log.Info("Setting up flashback mode in the standby database")
		err = EnableFlashbackInDatabase(r, stdbyReadyPod, ctx, req)
		if err != nil {
			return err
		}
	}

	return nil
}

// #############################################################################
//
//	Create oracle hostname environment variable object to be passed to sidb
//
// #############################################################################
// CreateOracleHostnameEnvVarObj builds ORACLE_HOSTNAME env var based on DB version.
func CreateOracleHostnameEnvVarObj(sidb *dbapi.SingleInstanceDatabase, referedPrimaryDatabase *dbapi.SingleInstanceDatabase) corev1.EnvVar {
	dbMajorVersion, err := strconv.Atoi(strings.Split(referedPrimaryDatabase.Status.ReleaseUpdate, ".")[0])
	if err != nil {
		// r.Log.Error(err, err.Error())
		return corev1.EnvVar{
			Name:  "ORACLE_HOSTNAME",
			Value: "",
		}
	}
	if dbMajorVersion >= 23 {
		return corev1.EnvVar{
			Name:  "ORACLE_HOSTNAME",
			Value: sidb.Name,
		}
	} else {
		return corev1.EnvVar{
			Name: "ORACLE_HOSTNAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		}
	}
}

// SetupExternalPrimaryDBTnsNamesInStandby configures external primary aliases in standby tnsnames.ora.
func SetupExternalPrimaryDBTnsNamesInStandby(r *SingleInstanceDatabaseReconciler, s *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {
	tnsFile := getTnsFilePathBySID(s.Spec.Sid)
	defaultAlias := GetPrimaryDatabaseSid(s, nil)
	defaultHost := GetPrimaryDatabaseHost(s, nil)
	defaultPort := GetPrimaryDatabasePort(s)
	if defaultPort == 0 {
		defaultPort = 1521
	}
	alias, host, port, serviceName, protocol, sslDN := resolveTNSAliasSettings(s, defaultAlias, defaultHost, defaultPort, defaultAlias)
	return upsertTnsAliasInPod(r, dbReadyPod, ctx, req, tnsFile, alias, host, port, serviceName, protocol, sslDN)
}

// SetupListenerForDGOnDatabase updates listener entries needed for Data Guard on a database.
func SetupListenerForDGOnDatabase(r *SingleInstanceDatabaseReconciler, d *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {

	out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("cat /opt/oracle/oradata/dbconfig/%s/listener.ora ", strings.ToUpper(d.Spec.Sid)))
	if err != nil {
		return fmt.Errorf("unable to obtain contents of listener.ora in database %v", d.Name)
	}
	r.Log.Info("listener.ora Output")
	r.Log.Info(out)

	if strings.Contains(out, strings.ToUpper(d.Spec.Sid)+"_DGMGRL") {
		r.Log.Info("LISTENER.ORA ALREADY HAS " + d.Spec.Sid + "_DGMGRL ENTRY IN SID_LIST_LISTENER ")
	} else {
		out, err = dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | cat > /opt/oracle/oradata/dbconfig/%s/listener.ora ", dbcommons.ListenerEntry, strings.ToUpper(d.Spec.Sid)))
		if err != nil {
			return fmt.Errorf("unable to modify listener.ora in database %v", d.Name)
		}
		r.Log.Info("Modifying listener.ora Output")
		r.Log.Info(out)

		err = RestartListenerInDatabase(r, dbReadyPod, ctx, req)
		if err != nil {
			return err
		}
	}
	return nil
}

// IsExternalPrimaryDatabase reports whether standby uses external primary details.
func IsExternalPrimaryDatabase(m *dbapi.SingleInstanceDatabase) bool {
	if m != nil && m.Spec.StandbyConfig != nil && m.Spec.StandbyConfig.PrimaryDetails != nil &&
		strings.TrimSpace(m.Spec.StandbyConfig.PrimaryDetails.Host) != "" {
		return true
	}
	return m != nil &&
		m.Spec.ExternalPrimaryDatabaseRef != nil &&
		strings.TrimSpace(m.Spec.ExternalPrimaryDatabaseRef.Host) != ""
}

// ValidateExternalPrimaryDatabaseRef validates required external primary fields.
func ValidateExternalPrimaryDatabaseRef(m *dbapi.SingleInstanceDatabase) error {
	if !IsExternalPrimaryDatabase(m) {
		return nil
	}

	ref := GetPrimaryDatabaseDetails(m)
	if ref == nil {
		return fmt.Errorf("primaryDetails cannot be empty when external primary is configured")
	}
	if strings.TrimSpace(ref.Host) == "" {
		return fmt.Errorf("primaryDetails.host cannot be empty")
	}
	if strings.TrimSpace(ref.Sid) == "" {
		return fmt.Errorf("primaryDetails.sid cannot be empty")
	}
	if ref.Port < 0 {
		return fmt.Errorf("primaryDetails.port cannot be negative")
	}

	return nil
}

// GetPrimaryDatabaseHost returns primary host from explicit details or referenced resource.
func GetPrimaryDatabaseHost(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if ref := GetPrimaryDatabaseDetails(m); ref != nil && strings.TrimSpace(ref.Host) != "" {
		return strings.TrimSpace(ref.Host)
	}
	if rp != nil {
		return rp.Name
	}
	return ""
}

// GetPrimaryDatabasePort returns primary listener port with default fallback.
func GetPrimaryDatabasePort(m *dbapi.SingleInstanceDatabase) int {
	if ref := GetPrimaryDatabaseDetails(m); ref != nil && ref.Port > 0 {
		return ref.Port
	}
	return int(dbcommons.CONTAINER_LISTENER_PORT)
}

// GetPrimaryDatabaseSid returns primary SID from explicit details or referenced resource.
func GetPrimaryDatabaseSid(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if ref := GetPrimaryDatabaseDetails(m); ref != nil && strings.TrimSpace(ref.Sid) != "" {
		return strings.ToUpper(strings.TrimSpace(ref.Sid))
	}
	if rp != nil {
		return strings.ToUpper(strings.TrimSpace(rp.Spec.Sid))
	}
	return ""
}

// GetPrimaryDatabasePdbName returns primary PDB name from explicit details or referenced resource.
func GetPrimaryDatabasePdbName(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if ref := GetPrimaryDatabaseDetails(m); ref != nil && strings.TrimSpace(ref.Pdbname) != "" {
		return strings.ToUpper(strings.TrimSpace(ref.Pdbname))
	}
	if rp != nil {
		return strings.ToUpper(strings.TrimSpace(rp.Spec.Pdbname))
	}
	return ""
}

// GetPrimaryDatabaseConnectString builds the primary connect string for standby flows.
func GetPrimaryDatabaseConnectString(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if m != nil && m.Spec.StandbyConfig != nil {
		if c := strings.TrimSpace(m.Spec.StandbyConfig.PrimaryConnectString); c != "" {
			return c
		}
	}

	if IsExternalPrimaryDatabase(m) {
		host := GetPrimaryDatabaseHost(m, rp)
		port := GetPrimaryDatabasePort(m)
		sid := GetPrimaryDatabaseSid(m, rp)
		if host == "" || sid == "" {
			return ""
		}
		return host + ":" + strconv.Itoa(port) + "/" + sid
	}

	primaryRef := getPrimaryDatabaseRefName(m)
	if dbcommons.IsSourceDatabaseOnCluster(primaryRef) && rp != nil {
		return rp.Name + ":" + strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)) + "/" + rp.Spec.Sid
	}

	return primaryRef
}

// GetPrimaryDatabaseDisplayName returns user-visible primary identity for logs/events.
func GetPrimaryDatabaseDisplayName(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if IsExternalPrimaryDatabase(m) {
		return GetPrimaryDatabaseHost(m, rp)
	}
	if rp != nil {
		return rp.Name
	}
	return strings.TrimSpace(getPrimaryDatabaseRefName(m))
}

// ShouldCreatePDBFromPrimary determines whether standby should create PDB metadata from primary.
func ShouldCreatePDBFromPrimary(m *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase) string {
	if IsExternalPrimaryDatabase(m) {
		if strings.TrimSpace(GetPrimaryDatabasePdbName(m, rp)) != "" {
			return "true"
		}
		return "false"
	}
	if rp != nil && rp.Spec.Pdbname != "" {
		return "true"
	}
	return "false"
}

func getPrimaryDatabaseRefName(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return ""
	}
	if m.Spec.StandbyConfig != nil {
		if name := strings.TrimSpace(m.Spec.StandbyConfig.PrimaryDatabaseRef); name != "" {
			return name
		}
	}
	return strings.TrimSpace(m.Spec.PrimaryDatabaseRef)
}

// GetPrimaryDatabaseDetails resolves external primary details from current and legacy spec fields.
func GetPrimaryDatabaseDetails(m *dbapi.SingleInstanceDatabase) *dbapi.SingleInstanceDatabaseExternalPrimaryRef {
	if m == nil {
		return nil
	}
	if m.Spec.StandbyConfig != nil && m.Spec.StandbyConfig.PrimaryDetails != nil {
		d := m.Spec.StandbyConfig.PrimaryDetails
		return &dbapi.SingleInstanceDatabaseExternalPrimaryRef{
			Host:    d.Host,
			Port:    d.Port,
			Sid:     d.Sid,
			Pdbname: d.Pdbname,
		}
	}
	return m.Spec.ExternalPrimaryDatabaseRef
}

// GetStandbyWalletSecretRef returns the standby wallet secret reference.
func GetStandbyWalletSecretRef(m *dbapi.SingleInstanceDatabase) string {
	return GetTDEPasswordSecretName(m)
}

// GetStandbyWalletMountPath returns mount path for standby wallet artifacts.
func GetStandbyWalletMountPath(m *dbapi.SingleInstanceDatabase) string {
	if tde := getTDEPasswordConfig(m); tde != nil {
		if mountPath := strings.TrimSpace(tde.MountPath); mountPath != "" {
			return mountPath
		}
	}
	return "/mnt/standby-wallet"
}

// GetStandbyWalletZipFileKey returns the secret key containing standby wallet zip content.
func GetStandbyWalletZipFileKey(m *dbapi.SingleInstanceDatabase) string {
	if tde := getTDEPasswordConfig(m); tde != nil {
		if key := strings.TrimSpace(tde.WalletZipFileKey); key != "" {
			return key
		}
	}
	return ""
}

// GetStandbyTDEWalletRoot returns effective TDE wallet root for standby setup.
func GetStandbyTDEWalletRoot(m *dbapi.SingleInstanceDatabase) string {
	if tde := getTDEPasswordConfig(m); tde != nil {
		if root := strings.TrimSpace(tde.WalletRoot); root != "" {
			return root
		}
	}
	if m == nil || m.Spec.StandbyConfig == nil {
		if m != nil {
			return GetWalletDirFromSid(m.Spec.Sid)
		}
		return "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet"
	}
	return GetWalletDirFromSid(m.Spec.Sid)
}

// GetWalletDirFromSid returns the default wallet directory path for the provided SID.
func GetWalletDirFromSid(sid string) string {
	trimmedSid := strings.ToUpper(strings.TrimSpace(sid))
	if trimmedSid == "" {
		return "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet"
	}
	return fmt.Sprintf("/opt/oracle/oradata/dbconfig/%s/.wallet", trimmedSid)
}

func GetAdminPasswordSecretMountRoot(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return "/run/secrets"
	}
	if mountRoot := strings.TrimSpace(getAdminPasswordSecretMountPathOverride(m)); mountRoot != "" {
		return strings.TrimRight(mountRoot, "/")
	}
	return "/run/secrets"
}

func GetAdminPasswordSecretMountPath(m *dbapi.SingleInstanceDatabase) string {
	return GetAdminPasswordSecretMountRoot(m) + "/" + GetAdminPasswordSecretFileName(m)
}

func GetAdminPasswordSecretFileName(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return "oracle_pwd"
	}
	if secretKey := strings.TrimSpace(getAdminPasswordSecretKey(m)); secretKey != "" {
		return secretKey
	}
	return "oracle_pwd"
}

func GetAdminPasswordSecretName(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(getAdminPasswordSecretName(m))
}

func GetAdminPasswordSkipInitWallet(m *dbapi.SingleInstanceDatabase) bool {
	if m == nil {
		return false
	}
	if admin := getAdminPasswordConfig(m); admin != nil {
		return admin.SkipInitWallet
	}
	return false
}

func GetAdminPasswordKeepSecret(m *dbapi.SingleInstanceDatabase) bool {
	if m == nil {
		return true
	}
	if admin := getAdminPasswordConfig(m); admin != nil && admin.KeepSecret != nil {
		return *admin.KeepSecret
	}
	return true
}

func HasTDEPasswordSecret(m *dbapi.SingleInstanceDatabase) bool {
	return GetTDEPasswordSecretName(m) != "" && GetTDEPasswordSecretFileName(m) != ""
}

func GetTDEPasswordSecretName(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return ""
	}
	if tde := getTDEPasswordConfig(m); tde != nil {
		return strings.TrimSpace(tde.SecretName)
	}
	return ""
}

func GetTDEPasswordSecretFileName(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return ""
	}
	if tde := getTDEPasswordConfig(m); tde != nil {
		return strings.TrimSpace(tde.SecretKey)
	}
	return ""
}

func GetTDEPasswordSecretMountRoot(m *dbapi.SingleInstanceDatabase) string {
	if m == nil {
		return "/run/secrets"
	}
	if tde := getTDEPasswordConfig(m); tde != nil {
		if mountRoot := strings.TrimSpace(tde.MountPath); mountRoot != "" {
			return strings.TrimRight(mountRoot, "/")
		}
	}
	return "/run/secrets"
}

func GetTDEPasswordSecretMountPath(m *dbapi.SingleInstanceDatabase) string {
	return GetTDEPasswordSecretMountRoot(m) + "/" + GetTDEPasswordSecretFileName(m)
}

func getAdminPasswordConfig(m *dbapi.SingleInstanceDatabase) *dbapi.SingleInstanceDatabaseAdminPassword {
	if m == nil {
		return nil
	}
	if m.Spec.Security != nil && m.Spec.Security.Secrets != nil && m.Spec.Security.Secrets.Admin != nil {
		return m.Spec.Security.Secrets.Admin
	}
	return &m.Spec.AdminPassword
}

func getTDEPasswordConfig(m *dbapi.SingleInstanceDatabase) *dbapi.SingleInstanceDatabasePasswordSecret {
	if m == nil {
		return nil
	}
	if m.Spec.Security != nil && m.Spec.Security.Secrets != nil && m.Spec.Security.Secrets.TDE != nil {
		return m.Spec.Security.Secrets.TDE
	}
	return nil
}

func getAdminPasswordSecretName(m *dbapi.SingleInstanceDatabase) string {
	if admin := getAdminPasswordConfig(m); admin != nil {
		return admin.SecretName
	}
	return ""
}

func getAdminPasswordSecretKey(m *dbapi.SingleInstanceDatabase) string {
	if admin := getAdminPasswordConfig(m); admin != nil {
		return admin.SecretKey
	}
	return ""
}

func getAdminPasswordSecretMountPathOverride(m *dbapi.SingleInstanceDatabase) string {
	if admin := getAdminPasswordConfig(m); admin != nil {
		return admin.MountPath
	}
	return ""
}

func getRestoreSpec(m *dbapi.SingleInstanceDatabase) *dbapi.SingleInstanceDatabaseRestoreSpec {
	if m == nil || m.Spec.Restore == nil {
		return nil
	}
	return m.Spec.Restore
}

func isPrimaryCreateAsMode(m *dbapi.SingleInstanceDatabase) bool {
	if m == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(m.Spec.CreateAs))
	return mode == "" || mode == "primary"
}

func shouldMountAdminPasswordSecret(m *dbapi.SingleInstanceDatabase) bool {
	if m == nil {
		return false
	}
	// Existing behavior: direct secret-file mount for express/free, prebuilt DB, or explicit opt-in.
	if m.Spec.Edition == "express" || m.Spec.Edition == "free" || m.Spec.Image.PrebuiltDB || GetAdminPasswordSkipInitWallet(m) {
		return true
	}
	// New behavior: primary restore flows require admin secret mounted at configured mountPath.
	return isPrimaryCreateAsMode(m) && (restoreUsesObjectStore(m) || restoreUsesFileSystem(m))
}

func getRestoreSourceType(m *dbapi.SingleInstanceDatabase) string {
	restore := getRestoreSpec(m)
	if restore == nil {
		return ""
	}
	hasObjectStore := restore.ObjectStore != nil
	hasFileSystem := restore.FileSystem != nil
	switch {
	case hasObjectStore && !hasFileSystem:
		return "objectStore"
	case hasFileSystem && !hasObjectStore:
		return "fileSystem"
	case hasObjectStore && hasFileSystem:
		return "invalid"
	default:
		return ""
	}
}

func restoreUsesObjectStore(m *dbapi.SingleInstanceDatabase) bool {
	return getRestoreSourceType(m) == "objectStore"
}

func restoreUsesFileSystem(m *dbapi.SingleInstanceDatabase) bool {
	return getRestoreSourceType(m) == "fileSystem"
}

func getRestoreTargetDataRoot(m *dbapi.SingleInstanceDatabase) string {
	restore := getRestoreSpec(m)
	if restore != nil && restore.Target != nil {
		if dataRoot := strings.TrimSpace(restore.Target.DataRoot); dataRoot != "" {
			return dataRoot
		}
	}
	return restoreDefaultDataRoot
}

func getRestoreTargetWalletRoot(m *dbapi.SingleInstanceDatabase) string {
	restore := getRestoreSpec(m)
	if restore != nil && restore.Target != nil {
		if walletRoot := strings.TrimSpace(restore.Target.WalletRoot); walletRoot != "" {
			return walletRoot
		}
	}
	sid := strings.ToUpper(strings.TrimSpace(m.Spec.Sid))
	if sid == "" {
		return "/opt/oracle/oradata/${ORACLE_SID}/wallets"
	}
	return fmt.Sprintf("/opt/oracle/oradata/%s/wallets", sid)
}

func getRestoreCatalogStartWith(m *dbapi.SingleInstanceDatabase) string {
	restore := getRestoreSpec(m)
	if restore == nil || restore.FileSystem == nil {
		return ""
	}
	if catalog := strings.TrimSpace(restore.FileSystem.CatalogStartWith); catalog != "" {
		return catalog
	}
	return strings.TrimSpace(restore.FileSystem.BackupPath)
}

func sidbAdditionalPVCVolumeName(index int) string {
	return fmt.Sprintf("additional-pvc-%d", index)
}

func isPathUnder(basePath, targetPath string) bool {
	base := filepath.Clean(strings.TrimSpace(basePath))
	target := filepath.Clean(strings.TrimSpace(targetPath))
	if base == "." || target == "." || base == "" || target == "" {
		return false
	}
	if base == "/" {
		return strings.HasPrefix(target, "/")
	}
	return target == base || strings.HasPrefix(target, base+"/")
}

func isRestoreFSPathVolumeBacked(m *dbapi.SingleInstanceDatabase, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if hasOradataPersistence(m) && isPathUnder(restoreDefaultDataRoot, path) {
		return true
	}
	for i := range m.Spec.Persistence.AdditionalPVCs {
		mountPath := strings.TrimSpace(m.Spec.Persistence.AdditionalPVCs[i].MountPath)
		pvcName := strings.TrimSpace(m.Spec.Persistence.AdditionalPVCs[i].PvcName)
		if mountPath == "" || pvcName == "" {
			continue
		}
		if isPathUnder(mountPath, path) {
			return true
		}
	}
	return false
}

func getTrueCacheServices(m *dbapi.SingleInstanceDatabase) []string {
	if m != nil && m.Spec.TrueCache != nil && len(m.Spec.TrueCache.TrueCacheServices) > 0 {
		return m.Spec.TrueCache.TrueCacheServices
	}
	if m == nil {
		return nil
	}
	return m.Spec.TrueCacheServices
}

func getTrueCacheBlobMountPath(m *dbapi.SingleInstanceDatabase) string {
	if m != nil && m.Spec.TrueCache != nil && strings.TrimSpace(m.Spec.TrueCache.BlobMountPath) != "" {
		return strings.TrimSpace(m.Spec.TrueCache.BlobMountPath)
	}
	return "/stage/tc_config_blob.tar.gz"
}

func getTrueCacheBlobConfigMapKey(m *dbapi.SingleInstanceDatabase) string {
	if m != nil && m.Spec.TrueCache != nil && strings.TrimSpace(m.Spec.TrueCache.BlobConfigMapKey) != "" {
		return strings.TrimSpace(m.Spec.TrueCache.BlobConfigMapKey)
	}
	return "tc_config_blob.tar.gz"
}

func resolveTrueCacheBlobConfigMap(
	m *dbapi.SingleInstanceDatabase,
	rp *dbapi.SingleInstanceDatabase,
) (string, string) {
	blobKey := getTrueCacheBlobConfigMapKey(m)
	if m == nil || m.Spec.CreateAs != "truecache" {
		return "", blobKey
	}
	if m.Spec.TrueCache != nil && strings.TrimSpace(m.Spec.TrueCache.BlobConfigMapRef) != "" {
		return strings.TrimSpace(m.Spec.TrueCache.BlobConfigMapRef), blobKey
	}
	if !IsExternalPrimaryDatabase(m) && rp != nil && rp.Name != "" {
		return rp.Name + "-truecache-blob", blobKey
	}
	return "", blobKey
}

func getTrueCacheUniqueName(m *dbapi.SingleInstanceDatabase) string {
	if m != nil && m.Spec.TrueCache != nil && strings.TrimSpace(m.Spec.TrueCache.TruedbUniqueName) != "" {
		return strings.TrimSpace(m.Spec.TrueCache.TruedbUniqueName)
	}
	if m == nil {
		return ""
	}
	return m.Spec.Sid + "_TC"
}

func (r *SingleInstanceDatabaseReconciler) addTrueCacheBlobVolumeMount(
	pod *corev1.Pod,
	m *dbapi.SingleInstanceDatabase,
	rp *dbapi.SingleInstanceDatabase,
) {
	if m.Spec.CreateAs != "truecache" {
		return
	}

	cmName, blobKey := resolveTrueCacheBlobConfigMap(m, rp)
	if cmName == "" {
		return
	}

	mountPath := getTrueCacheBlobMountPath(m)
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "truecache-blob-vol",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
				Items: []corev1.KeyToPath{{
					Key:  blobKey,
					Path: filepath.Base(mountPath),
				}},
			},
		},
	})
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      "truecache-blob-vol",
		MountPath: mountPath,
		SubPath:   filepath.Base(mountPath),
		ReadOnly:  true,
	})
}

func mergeSIDBEnvVars(base []corev1.EnvVar, extra []corev1.EnvVar) []corev1.EnvVar {
	if len(extra) == 0 {
		return base
	}
	merged := append([]corev1.EnvVar{}, base...)
	indexByName := make(map[string]int, len(merged))
	for i := range merged {
		if merged[i].Name != "" {
			indexByName[merged[i].Name] = i
		}
	}
	for _, env := range extra {
		if env.Name == "" {
			merged = append(merged, env)
			continue
		}
		if idx, ok := indexByName[env.Name]; ok {
			merged[idx] = env
			continue
		}
		indexByName[env.Name] = len(merged)
		merged = append(merged, env)
	}
	return merged
}

func buildSIDBSecurityScriptEnvVars(m *dbapi.SingleInstanceDatabase) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0)

	if getTcpsEnabled(m) {
		envs = append(envs, corev1.EnvVar{Name: "TCPS_ENABLED", Value: "true"})
		tcpsCertsLocation := getTcpsCertsLocation(m)
		if strings.TrimSpace(tcpsCertsLocation) != "" {
			envs = append(envs,
				corev1.EnvVar{Name: "TCPS_CERTS_LOCATION", Value: tcpsCertsLocation},
				corev1.EnvVar{Name: "TCPS_TLS_SECRET_MOUNT_PATH", Value: tcpsCertsLocation},
			)
		}
	}

	if HasTDEPasswordSecret(m) {
		tdeSecretMountRoot := GetTDEPasswordSecretMountRoot(m)
		tdeSecretName := GetTDEPasswordSecretName(m)
		tdeSecretKey := GetTDEPasswordSecretFileName(m)
		envs = append(envs,
			corev1.EnvVar{Name: "TDE_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "SECRET_BASE_DIR", Value: tdeSecretMountRoot},
			corev1.EnvVar{
				Name: "TDE_WALLET_PWD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: tdeSecretName},
						Key:                  tdeSecretKey,
					},
				},
			},
			corev1.EnvVar{Name: "ORACLE_TDE_SECRET_FILE", Value: GetTDEPasswordSecretMountPath(m)},
		)
		if tde := getTDEPasswordConfig(m); tde != nil {
			if walletRoot := strings.TrimSpace(tde.WalletRoot); walletRoot != "" {
				envs = append(envs, corev1.EnvVar{Name: "TDE_WALLET_ROOT", Value: walletRoot})
			}
		}
	}

	return envs
}

func buildSIDBRestoreScriptEnvVars(m *dbapi.SingleInstanceDatabase) []corev1.EnvVar {
	restore := getRestoreSpec(m)
	if restore == nil {
		return nil
	}

	envs := []corev1.EnvVar{
		{Name: "RESTORE_ENABLED", Value: "true"},
		{Name: "RESTORE_SOURCE_TYPE", Value: getRestoreSourceType(m)},
		{Name: "RESTORE_DATA_ROOT", Value: getRestoreTargetDataRoot(m)},
		{Name: "RESTORE_WALLET_ROOT", Value: getRestoreTargetWalletRoot(m)},
	}

	if restore.Options != nil {
		if sourceDBName := strings.TrimSpace(restore.Options.SourceDBName); sourceDBName != "" {
			envs = append(envs, corev1.EnvVar{Name: "SOURCE_DB_NAME", Value: sourceDBName})
		}
		if restore.Options.ForceOpcReinstall != nil {
			envs = append(envs, corev1.EnvVar{Name: "FORCE_OPC_REINSTALL", Value: strconv.FormatBool(*restore.Options.ForceOpcReinstall)})
		}
		if restore.Options.RunCrosscheck != nil {
			envs = append(envs, corev1.EnvVar{Name: "RMAN_RUN_CROSSCHECK", Value: strconv.FormatBool(*restore.Options.RunCrosscheck)})
		}
		if restore.Options.RunValidateOnly != nil {
			envs = append(envs, corev1.EnvVar{Name: "RMAN_VALIDATE_ONLY", Value: strconv.FormatBool(*restore.Options.RunValidateOnly)})
		}
	}

	if restoreUsesObjectStore(m) && restore.ObjectStore != nil {
		envs = append(envs, corev1.EnvVar{Name: "CLONE_DB_FROM_OBJ_BACKUP", Value: "true"})

		if ref := restore.ObjectStore.OCIConfig; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "OCI_CONFIG_FILE", Value: filepath.Join(restoreOCIConfigMountDir, key)})
			}
		}
		if ref := restore.ObjectStore.PrivateKey; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "PVT_KEY_PATH", Value: filepath.Join(restoreOCIPrivateKeyMountDir, key)})
			}
		}
		if ref := restore.ObjectStore.SourceDBWallet; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "SOURCE_DB_WALLET", Value: filepath.Join(restoreSourceWalletMountDir, key)})
			}
		}
		if ref := restore.ObjectStore.SourceDBWalletPw; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "SOURCE_DB_WALLET_PWDFILE", Value: filepath.Join(restoreSourceWalletPwdMountDir, key)})
			}
		}
		if ref := restore.ObjectStore.BackupModuleConf; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "BACKUP_CONFIG_FILE", Value: filepath.Join(restoreBackupModuleMountDir, key)})
			}
		}
		if ref := restore.ObjectStore.OpcInstallerZip; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "OPC_INSTALL_ZIP", Value: filepath.Join(restoreOPCInstallerMountDir, key)})
			}
		}
		if id := restore.ObjectStore.BackupIdentity; id != nil {
			if bucket := strings.TrimSpace(id.BucketName); bucket != "" {
				envs = append(envs, corev1.EnvVar{Name: "BUCKET_NAME", Value: bucket})
			}
			if dbid := strings.TrimSpace(id.DBID); dbid != "" {
				envs = append(envs, corev1.EnvVar{Name: "DBID", Value: dbid})
			}
			if compartment := strings.TrimSpace(id.CompartmentOCID); compartment != "" {
				envs = append(envs, corev1.EnvVar{Name: "COMPARTMENT_OCID", Value: compartment})
			}
		}
		if enc := restore.ObjectStore.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil {
			if key := strings.TrimSpace(enc.DecryptPasswordSecret.Key); key != "" {
				envs = append(envs, corev1.EnvVar{Name: "RMAN_DECRYPT_PWD_FILE", Value: filepath.Join(restoreDecryptPwdMountDir, key)})
			}
		}
	}

	if restoreUsesFileSystem(m) && restore.FileSystem != nil {
		envs = append(envs,
			corev1.EnvVar{Name: "CLONE_DB_FROM_FS_BACKUP", Value: "true"},
			corev1.EnvVar{Name: "FS_BACKUP_PATH", Value: strings.TrimSpace(restore.FileSystem.BackupPath)},
		)
		if catalog := getRestoreCatalogStartWith(m); catalog != "" {
			envs = append(envs, corev1.EnvVar{Name: "FS_BACKUP_CATALOG_START_WITH", Value: catalog})
		}
		if ref := restore.FileSystem.SourceDBWallet; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "SOURCE_DB_WALLET", Value: filepath.Join(restoreSourceWalletMountDir, key)})
			}
		}
		if ref := restore.FileSystem.SourceDBWalletPw; ref != nil {
			key := strings.TrimSpace(ref.Key)
			if key != "" {
				envs = append(envs, corev1.EnvVar{Name: "SOURCE_DB_WALLET_PWDFILE", Value: filepath.Join(restoreSourceWalletPwdMountDir, key)})
			}
		}
		if enc := restore.FileSystem.EncryptedBackup; enc != nil && enc.Enabled && enc.DecryptPasswordSecret != nil {
			if key := strings.TrimSpace(enc.DecryptPasswordSecret.Key); key != "" {
				envs = append(envs, corev1.EnvVar{Name: "RMAN_DECRYPT_PWD_FILE", Value: filepath.Join(restoreDecryptPwdMountDir, key)})
			}
		}
	}

	return envs
}

func mergeSIDBEnvVarsWithSecurity(m *dbapi.SingleInstanceDatabase, base []corev1.EnvVar) []corev1.EnvVar {
	merged := append(base, buildSIDBSecurityScriptEnvVars(m)...)
	merged = append(merged, buildSIDBRestoreScriptEnvVars(m)...)
	return mergeSIDBEnvVars(merged, m.Spec.EnvVars)
}

func ValidateStandbyWalletSecretRef(r *SingleInstanceDatabaseReconciler, m *dbapi.SingleInstanceDatabase, ctx context.Context) error {
	secretName := GetStandbyWalletSecretRef(m)
	if secretName == "" {
		return nil
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: secretName}, secret); err != nil {
		return fmt.Errorf("security.secrets.tde.secretName %q not found: %w", secretName, err)
	}
	if len(secret.Data) == 0 {
		return fmt.Errorf("security.secrets.tde.secretName %q has no data", secretName)
	}
	if zipKey := GetStandbyWalletZipFileKey(m); zipKey != "" {
		if _, ok := secret.Data[zipKey]; !ok {
			return fmt.Errorf("security.secrets.tde.walletZipFileKey %q not found in secret %q", zipKey, secretName)
		}
	}
	return nil
}
