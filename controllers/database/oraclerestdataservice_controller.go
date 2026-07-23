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

//nolint:staticcheck,revive // deprecated fields and legacy method signatures are intentionally supported for compatibility.
package controllers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	"github.com/go-logr/logr"
)

const oracleRestDataServiceFinalizer = "database.oracle.com/oraclerestdataservicefinalizer"

// OracleRestDataServiceReconciler reconciles a OracleRestDataService object.
type OracleRestDataServiceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

type ordsPhaseContext struct {
	oracleRestDataService  *dbapi.OracleRestDataService
	singleInstanceDatabase *dbapi.SingleInstanceDatabase
	sidbReadyPod           corev1.Pod
	ordsReadyPod           corev1.Pod
	reconcileID            string
}

func buildOracleRestHereDocWriteCommand(fileName, content string) string {
	return fmt.Sprintf("umask 177\ncat > %s <<'EOF'\n%s\nEOF\numask 022", fileName, content)
}

func runOracleRestSQLPlusScript(
	r *OracleRestDataServiceReconciler,
	pod corev1.Pod,
	sql string,
	ctx context.Context,
	req ctrl.Request,
) (out string, err error) {
	log := r.Log.WithValues("runOracleRestSQLPlusScript", req.NamespacedName, "pod", pod.Name)

	if _, err = dbcommons.ExecCommand(
		r,
		r.Config,
		pod.Name,
		pod.Namespace,
		"",
		ctx,
		req,
		true,
		"bash",
		"-c",
		buildOracleRestHereDocWriteCommand("ords.sql", sql),
	); err != nil {
		log.Error(err, "failed to create sqlplus script")
		return "", err
	}

	defer func() {
		if _, cleanupErr := dbcommons.ExecCommand(
			r,
			r.Config,
			pod.Name,
			pod.Namespace,
			"",
			ctx,
			req,
			true,
			"bash",
			"-c",
			"rm -rf ords.sql",
		); cleanupErr != nil {
			log.Error(cleanupErr, "failed to remove sqlplus script")
			if err == nil {
				err = cleanupErr
			}
		}
	}()

	out, err = dbcommons.ExecCommand(
		r,
		r.Config,
		pod.Name,
		pod.Namespace,
		"",
		ctx,
		req,
		true,
		"bash",
		"-c",
		dbcommons.SQLPlusCLI+" @ords.sql",
	)
	if err != nil {
		log.Error(err, "sqlplus command execution failed")
		return "", err
	}
	upperOut := strings.ToUpper(out)
	if strings.Contains(upperOut, "ORA-") || strings.Contains(upperOut, "PLS-") || strings.Contains(upperOut, "SP2-") {
		err = fmt.Errorf("sqlplus returned an Oracle error: %s", strings.TrimSpace(out))
		log.Error(err, "sqlplus script failed")
		return out, err
	}

	return out, nil
}

func oracleRestPasswordFromSecret(secret *corev1.Secret, secretKey string) (string, error) {
	if strings.TrimSpace(secretKey) == "" {
		return "", errors.New("secret key is empty")
	}
	value, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("secret key %q not found", secretKey)
	}
	password := string(value)
	if password == "" {
		return "", fmt.Errorf("secret key %q contains an empty password", secretKey)
	}
	return password, nil
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OracleRestDataService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *OracleRestDataServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	phaseCtx := &ordsPhaseContext{
		oracleRestDataService:  &dbapi.OracleRestDataService{},
		singleInstanceDatabase: &dbapi.SingleInstanceDatabase{},
		reconcileID:            newOracleRestDataServiceReconcileID(req),
	}
	r.phaseLogger(ctx, req, "reconcile", phaseCtx).Info("Reconcile requested")

	defer r.updateOracleRestDataServiceStatus(ctx, phaseCtx)
	defer r.updateOracleRestDataServiceDatabaseStatus(ctx, phaseCtx)

	result, err := r.runORDSPhase(ctx, req, "fetch_resource", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseFetchOracleRestDataService(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil || phaseCtx.oracleRestDataService.Name == "" {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "initialize_status", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseInitializeOracleRestDataServiceStatus(ctx, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "fetch_database_ref", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseFetchDatabaseRef(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "manage_deletion", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseManageOracleRestDataServiceDeletion(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}
	if phaseCtx.oracleRestDataService.GetDeletionTimestamp() != nil {
		r.phaseLogger(ctx, req, "manage_deletion", phaseCtx).Info("Deletion reconcile complete")
		return requeueN, nil
	}

	result, err = r.runORDSPhase(ctx, req, "validate_spec", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseValidateOracleRestDataServiceSpec(ctx, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "ensure_service", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseEnsureOracleRestDataServiceService(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "ensure_pvc", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseEnsureOracleRestDataServicePVC(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "validate_database_readiness", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseValidateOracleRestDataServiceDatabaseReadiness(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "ensure_pods", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseEnsureOracleRestDataServicePods(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "check_health", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseCheckOracleRestDataServiceHealth(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "rest_enable_schemas", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseRestEnableSchemas(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "configure_apex", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseConfigureOracleRestDataServiceApex(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "enable_mongodb", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseEnableOracleRestDataServiceMongoDB(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "cleanup_secrets", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseCleanupOracleRestDataServiceSecrets(ctx, req, phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.runORDSPhase(ctx, req, "finalize_status", phaseCtx, func() (ctrl.Result, error) {
		return r.phaseFinalizeOracleRestDataServiceStatus(phaseCtx)
	})
	if result.Requeue || err != nil {
		return result, err
	}

	r.phaseLogger(ctx, req, "reconcile", phaseCtx).Info("Reconcile completed")
	return ctrl.Result{}, nil
}

func newOracleRestDataServiceReconcileID(req ctrl.Request) string {
	return fmt.Sprintf("%s/%s-%d", req.Namespace, req.Name, time.Now().UnixNano())
}

func (r *OracleRestDataServiceReconciler) phaseLogger(ctx context.Context, req ctrl.Request, phase string, phaseCtx *ordsPhaseContext) logr.Logger {
	reconcileID := ""
	if phaseCtx != nil {
		reconcileID = phaseCtx.reconcileID
	}
	return ctrllog.FromContext(ctx).WithValues("phase", phase, "oraclerestdataservice", req.NamespacedName, "reconcileID", reconcileID)
}

func (r *OracleRestDataServiceReconciler) runORDSPhase(
	ctx context.Context,
	req ctrl.Request,
	phase string,
	phaseCtx *ordsPhaseContext,
	fn func() (ctrl.Result, error),
) (ctrl.Result, error) {
	log := r.phaseLogger(ctx, req, phase, phaseCtx)
	log.Info("Phase started")
	result, err := fn()
	if err != nil {
		log.Error(err, "Phase failed")
		return result, err
	}
	if result.RequeueAfter > 0 && !result.Requeue {
		result.Requeue = true
	}
	if result.Requeue {
		log.Info("Phase requested requeue", "requeueAfter", result.RequeueAfter)
		return result, nil
	}
	log.Info("Phase completed")
	return result, nil
}

func (r *OracleRestDataServiceReconciler) updateOracleRestDataServiceStatus(ctx context.Context, phaseCtx *ordsPhaseContext) {
	if phaseCtx == nil || phaseCtx.oracleRestDataService == nil || phaseCtx.oracleRestDataService.Name == "" {
		return
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &dbapi.OracleRestDataService{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: phaseCtx.oracleRestDataService.Namespace,
			Name:      phaseCtx.oracleRestDataService.Name,
		}, latest); err != nil {
			return err
		}
		latest.Status = phaseCtx.oracleRestDataService.Status
		return r.Status().Update(ctx, latest)
	})
	if err != nil && !apierrors.IsNotFound(err) {
		r.Log.Error(err, "failed to update oracleRestDataService status", "reconcileID", phaseCtx.reconcileID)
	}
}

func (r *OracleRestDataServiceReconciler) updateOracleRestDataServiceDatabaseStatus(ctx context.Context, phaseCtx *ordsPhaseContext) {
	if phaseCtx == nil || phaseCtx.singleInstanceDatabase == nil || phaseCtx.singleInstanceDatabase.Name == "" {
		return
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &dbapi.SingleInstanceDatabase{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: phaseCtx.singleInstanceDatabase.Namespace,
			Name:      phaseCtx.singleInstanceDatabase.Name,
		}, latest); err != nil {
			return err
		}
		latest.Status = phaseCtx.singleInstanceDatabase.Status
		return r.Status().Update(ctx, latest)
	})
	if err != nil && !apierrors.IsNotFound(err) {
		r.Log.Error(err, "failed to update singleInstanceDatabase status", "reconcileID", phaseCtx.reconcileID)
	}
}

func (r *OracleRestDataServiceReconciler) phaseFetchOracleRestDataService(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, phaseCtx.oracleRestDataService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.phaseLogger(ctx, req, "fetch_resource", phaseCtx).Info("Resource deleted")
			return requeueN, nil
		}
		return requeueY, err
	}
	return requeueN, nil
}

func (r *OracleRestDataServiceReconciler) phaseInitializeOracleRestDataServiceStatus(
	ctx context.Context,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	ords := phaseCtx.oracleRestDataService
	if ords.Status.Status == "" {
		ords.Status.Status = dbcommons.StatusPending
		ords.Status.ApxeUrl = dbcommons.ValueUnavailable
		ords.Status.DatabaseApiUrl = dbcommons.ValueUnavailable
		ords.Status.DatabaseActionsUrl = dbcommons.ValueUnavailable
		if err := r.Status().Update(ctx, ords); err != nil {
			return requeueY, err
		}
	}
	ords.Status.LoadBalancer = strconv.FormatBool(ords.Spec.LoadBalancer)
	ords.Status.Image = ords.Spec.Image
	return requeueN, nil
}

func (r *OracleRestDataServiceReconciler) phaseFetchDatabaseRef(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	ords := phaseCtx.oracleRestDataService
	sidb := phaseCtx.singleInstanceDatabase
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: ords.Spec.DatabaseRef}, sidb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			ords.Status.Status = dbcommons.StatusError
			ords.Status.DatabaseRef = ""
			eventReason := "Error"
			eventMsg := "database reference " + ords.Spec.DatabaseRef + " not found"
			r.Recorder.Eventf(ords, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			r.phaseLogger(ctx, req, "fetch_database_ref", phaseCtx).Info(eventMsg)
			return requeueY, nil
		}
		return requeueY, err
	}
	if ords.Status.DatabaseRef == "" {
		ords.Status.Status = dbcommons.StatusPending
		ords.Status.DatabaseRef = ords.Spec.DatabaseRef
		eventReason := "Database Check"
		eventMsg := "database reference " + ords.Spec.DatabaseRef + " found"
		r.Recorder.Eventf(ords, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
	}
	return requeueN, nil
}

func (r *OracleRestDataServiceReconciler) phaseManageOracleRestDataServiceDeletion(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.manageOracleRestDataServiceDeletion(req, ctx, phaseCtx.oracleRestDataService, phaseCtx.singleInstanceDatabase), nil
}

func (r *OracleRestDataServiceReconciler) phaseValidateOracleRestDataServiceSpec(
	ctx context.Context,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	result, _ := r.validate(phaseCtx.oracleRestDataService, phaseCtx.singleInstanceDatabase, ctx)
	return result, nil
}

func (r *OracleRestDataServiceReconciler) phaseEnsureOracleRestDataServiceService(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.createSVC(ctx, req, phaseCtx.oracleRestDataService, phaseCtx.singleInstanceDatabase), nil
}

func (r *OracleRestDataServiceReconciler) phaseEnsureOracleRestDataServicePVC(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	result, _ := r.createPVC(ctx, req, phaseCtx.oracleRestDataService)
	return result, nil
}

func (r *OracleRestDataServiceReconciler) phaseValidateOracleRestDataServiceDatabaseReadiness(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	result, sidbReadyPod := r.validateSIDBReadiness(phaseCtx.oracleRestDataService, phaseCtx.singleInstanceDatabase, ctx, req)
	phaseCtx.sidbReadyPod = sidbReadyPod
	return result, nil
}

func (r *OracleRestDataServiceReconciler) phaseEnsureOracleRestDataServicePods(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.createPods(phaseCtx.oracleRestDataService, phaseCtx.singleInstanceDatabase, ctx, req), nil
}

func (r *OracleRestDataServiceReconciler) phaseCheckOracleRestDataServiceHealth(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	result, ordsReadyPod := r.checkHealthStatus(
		phaseCtx.oracleRestDataService,
		phaseCtx.singleInstanceDatabase,
		phaseCtx.sidbReadyPod,
		ctx,
		req,
	)
	phaseCtx.ordsReadyPod = ordsReadyPod
	return result, nil
}

func (r *OracleRestDataServiceReconciler) phaseRestEnableSchemas(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.restEnableSchemas(
		phaseCtx.oracleRestDataService,
		phaseCtx.singleInstanceDatabase,
		phaseCtx.sidbReadyPod,
		phaseCtx.ordsReadyPod,
		ctx,
		req,
	), nil
}

func (r *OracleRestDataServiceReconciler) phaseConfigureOracleRestDataServiceApex(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.configureApex(
		phaseCtx.oracleRestDataService,
		phaseCtx.singleInstanceDatabase,
		phaseCtx.sidbReadyPod,
		phaseCtx.ordsReadyPod,
		ctx,
		req,
	), nil
}

func (r *OracleRestDataServiceReconciler) phaseEnableOracleRestDataServiceMongoDB(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	return r.enableMongoDB(
		phaseCtx.oracleRestDataService,
		phaseCtx.singleInstanceDatabase,
		phaseCtx.sidbReadyPod,
		phaseCtx.ordsReadyPod,
		ctx,
		req,
	), nil
}

func (r *OracleRestDataServiceReconciler) phaseCleanupOracleRestDataServiceSecrets(
	ctx context.Context,
	req ctrl.Request,
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	r.deleteSecrets(phaseCtx.oracleRestDataService, ctx, req)
	return requeueN, nil
}

func (r *OracleRestDataServiceReconciler) phaseFinalizeOracleRestDataServiceStatus(
	phaseCtx *ordsPhaseContext,
) (ctrl.Result, error) {
	if phaseCtx.oracleRestDataService.Status.ServiceIP == "" {
		return requeueY, nil
	}
	return requeueN, nil
}

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) validate(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context) (ctrl.Result, error) {

	var err error
	eventReason := "Spec Error"
	var eventMsgs []string

	//First check image pull secrets
	if m.Spec.Image.PullSecrets != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: m.Spec.Image.PullSecrets, Namespace: m.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", err.Error())
				r.Log.Info(err.Error())
				m.Status.Status = dbcommons.StatusError
				return requeueY, err
			}
			r.Log.Error(err, err.Error())
			return requeueY, err
		}
	}

	// If ORDS has no persistence specified, ensure SIDB has datafiles
	// persistence configured, including the newer persistence.oradata.pvcName
	// form.
	if m.Spec.Persistence.Size == "" && !hasOradataPersistence(n) {
		eventMsgs = append(eventMsgs, "cannot configure ORDS for database "+m.Spec.DatabaseRef+" that has no attached persistent volume")
	}
	if !m.Status.OrdsInstalled && n.Status.OrdsReference != "" {
		eventMsgs = append(eventMsgs, "database "+m.Spec.DatabaseRef+" is already configured with ORDS "+n.Status.OrdsReference)
	}
	if m.Status.DatabaseRef != "" && m.Status.DatabaseRef != m.Spec.DatabaseRef {
		eventMsgs = append(eventMsgs, "databaseRef cannot be updated")
	}
	if m.Status.Image.PullFrom != "" && m.Status.Image != m.Spec.Image {
		eventMsgs = append(eventMsgs, "image patching is not available currently")
	}

	if len(eventMsgs) > 0 {
		m.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", strings.Join(eventMsgs, ","))
		r.Log.Info(strings.Join(eventMsgs, "\n"))
		err = errors.New(strings.Join(eventMsgs, ","))
		return requeueY, err
	}

	return requeueN, err
}

// #####################################################################################################
//
//	Validate Readiness of the primary DB specified
//
// #####################################################################################################
func (r *OracleRestDataServiceReconciler) validateSIDBReadiness(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod) {

	log := r.Log.WithValues("validateSidbReadiness", req.NamespacedName)

	// ## FETCH THE SIDB REPLICAS .
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
		n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}

	if m.Status.OrdsInstalled || m.Status.CommonUsersCreated {
		return requeueN, sidbReadyPod
	}

	m.Status.Status = dbcommons.StatusPending
	if sidbReadyPod.Name == "" || n.Status.Status != dbcommons.StatusReady {
		eventReason := "Database Check"
		eventMsg := "status of database " + n.Name + " is not ready, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		return requeueY, sidbReadyPod
	}
	eventReason := "Database Check"
	eventMsg := "status of database " + n.Name + " is ready"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)

	// Validate databaseRef Admin Password
	adminSecretName, adminSecretKey, _, ok := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
	if !ok {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Password"
		eventMsg := "database admin password secret reference is not set"
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		log.Info(eventMsg)
		return requeueY, sidbReadyPod
	}
	adminPasswordSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: adminSecretName, Namespace: m.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			eventReason := "Database Password"
			eventMsg := "password secret " + adminSecretName + " not found, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			r.Log.Info(eventMsg)
			return requeueY, sidbReadyPod
		}
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	adminPassword, err := oracleRestPasswordFromSecret(adminPasswordSecret, adminSecretKey)
	if err != nil {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Password"
		eventMsg := "database admin password secret is invalid: " + err.Error()
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		log.Info(eventMsg, "secret", adminSecretName)
		return requeueY, sidbReadyPod
	}
	if err := dbcommons.ValidateOracleSQLPassword(adminPassword); err != nil {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Password"
		eventMsg := "database admin password secret contains unsupported characters for SQL setup: " + err.Error()
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		log.Info(eventMsg, "secret", adminSecretName)
		return requeueY, sidbReadyPod
	}

	maskedConnectString := fmt.Sprintf(`sys/"********"@%s as sysdba`, n.Spec.Sid)
	log.Info("Validating database admin password", "connectString", maskedConnectString)

	out, err := dbcommons.ExecSQLPlusScript(
		r,
		r.Config,
		sidbReadyPod.Name,
		sidbReadyPod.Namespace,
		"",
		ctx,
		req,
		"validate-admin-password.sql",
		fmt.Sprintf(dbcommons.ValidateAdminPassword, adminPassword, n.Spec.Sid),
	)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	if strings.Contains(out, "USER is \"SYS\"") {
		log.Info("validated Admin password successfully")
	} else if strings.Contains(out, "ORA-01017") {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Check"
		eventMsg := "login denied, invalid database admin password in secret " + adminSecretName
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		log.Info(eventMsg)
		return requeueY, sidbReadyPod
	} else {
		eventMsg := "login attempt failed for database admin password in secret " + adminSecretName
		log.Info(eventMsg)
		return requeueY, sidbReadyPod
	}

	// Create PDB , CDB Admin users and grant permissions. ORDS installation on CDB level
	out, err = dbcommons.ExecSQLPlusScript(
		r,
		r.Config,
		sidbReadyPod.Name,
		sidbReadyPod.Namespace,
		"",
		ctx,
		req,
		"/tmp/set-admin-users.sql",
		fmt.Sprintf(dbcommons.SetAdminUsersSQL, adminPassword),
	)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "ORA-") ||
		strings.Contains(out, "ERROR") && strings.Contains(out, "ORA-01920") {
		m.Status.CommonUsersCreated = true
	}
	return requeueN, sidbReadyPod
}

// #####################################################################################################
//
//	Check ORDS Health Status
//
// #####################################################################################################
func (r *OracleRestDataServiceReconciler) checkHealthStatus(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod) {
	log := r.Log.WithValues("checkHealthStatus", req.NamespacedName)

	readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, readyPod
	}
	if readyPod.Name == "" {
		m.Status.Status = dbcommons.StatusPending
		return requeueY, readyPod
	}

	// Get ORDS Status
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf(dbcommons.GetORDSStatus, func() int32 {
			if m.Spec.HTTPPort == 0 {
				return dbcommons.ORDSDefaultHTTPPort
			}
			return m.Spec.HTTPPort
		}()))
	log.Info("GetORDSStatus Output")
	log.Info(out)
	if err != nil {
		log.Info(err.Error())
		return requeueY, readyPod
	}
	if !strings.Contains(out, "HTTP 200") {
		log.Info("ORDS landing endpoint did not return HTTP 200")
		return requeueY, readyPod
	}

	m.Status.Status = dbcommons.StatusUpdating
	if n.Status.Status == dbcommons.StatusReady || n.Status.Status == dbcommons.StatusUpdating || n.Status.Status == dbcommons.StatusPatching {
		m.Status.Status = dbcommons.StatusReady
	}
	if !m.Status.OrdsInstalled {
		m.Status.OrdsInstalled = true
		n.Status.OrdsReference = m.Name
		if err := r.Status().Update(ctx, n); err != nil {
			log.Error(err, "failed to update SingleInstanceDatabase ORDS reference")
			return requeueY, readyPod
		}
		eventReason := "ORDS Installation"
		eventMsg := "installation of ORDS completed"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.OpenPDBSeed, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
		} else {
			log.Info("Close PDB seed")
			log.Info(out)
		}
	}
	if m.Status.Status == dbcommons.StatusUpdating {
		return requeueY, readyPod
	}
	return requeueN, readyPod
}

// #############################################################################
//
//	Instantiate Service spec from OracleRestDataService spec
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) instantiateSVCSpec(m *dbapi.OracleRestDataService) *corev1.Service {
	ordsHTTPPort := m.Spec.HTTPPort
	if ordsHTTPPort == 0 {
		ordsHTTPPort = dbcommons.ORDSDefaultHTTPPort
	}
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels: map[string]string{
				"app": m.Name,
			},
			Annotations: func() map[string]string {
				annotations := make(map[string]string)
				if len(m.Spec.ServiceAnnotations) != 0 {
					for key, value := range m.Spec.ServiceAnnotations {
						annotations[key] = value
					}
				}
				return annotations
			}(),
		},
		Spec: corev1.ServiceSpec{
			Ports: func() []corev1.ServicePort {
				ports := []corev1.ServicePort{
					{
						Name:       "client",
						Port:       8181,
						TargetPort: intstr.FromInt(int(ordsHTTPPort)),
						Protocol:   corev1.ProtocolTCP,
					},
				}
				// Conditionally add MongoDB port if enabled
				if m.Spec.MongoDbApi {
					ports = append(ports, corev1.ServicePort{
						Name:     "mongodb",
						Port:     27017,
						Protocol: corev1.ProtocolTCP,
					})
				}
				return ports
			}(),
			Selector: map[string]string{
				"app": m.Name,
			},
			Type: corev1.ServiceType(func() string {
				if m.Spec.LoadBalancer {
					return "LoadBalancer"
				}
				return "NodePort"
			}()),
		},
	}
	// Set StandbyDatabase instance as the owner and controller
	_ = ctrl.SetControllerReference(m, svc, r.Scheme)
	return svc
}

// #############################################################################
//
//	Instantiate POD spec from OracleRestDataService spec
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) instantiatePodSpec(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, _ ctrl.Request) *corev1.Pod {
	ordsHTTPPort := m.Spec.HTTPPort
	if ordsHTTPPort == 0 {
		ordsHTTPPort = dbcommons.ORDSDefaultHTTPPort
	}

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
				if m.Spec.Persistence.Size == "" && n.Spec.Persistence.AccessMode == "ReadWriteOnce" {
					// Only allowing pods to be scheduled on the node where SIDB pods are running
					return &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{n.Name}, // Schedule on same host as DB Pod
									}},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
							},
						},
					}
				}
				return nil
			}(),
			Volumes: []corev1.Volume{
				{
					Name: "datamount",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: func() string {
								if m.Spec.Persistence.AccessMode != "" {
									return m.Name
								}
								return n.Name
							}(),
							ReadOnly: false,
						},
					},
				},
				{
					Name: "varmount",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			InitContainers: func() []corev1.Container {
				initContainers := []corev1.Container{}
				if m.Spec.Persistence.Size != "" && m.Spec.Persistence.SetWritePermissions != nil && *m.Spec.Persistence.SetWritePermissions {
					initContainers = append(initContainers, corev1.Container{
						Name:    "init-permissions",
						Image:   m.Spec.Image.PullFrom,
						Command: []string{"/bin/sh", "-c", fmt.Sprintf("chown %d:%d /etc/ords/config/ || true", int(dbcommons.ORACLE_UID), int(dbcommons.DBA_GUID))},
						SecurityContext: &corev1.SecurityContext{
							// User ID 0 means, root user
							RunAsUser: func() *int64 { i := int64(0); return &i }(),
						},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: "/etc/ords/config/",
							Name:      "datamount",
						}},
					})
				}

				initContainers = append(initContainers, corev1.Container{
					Name:    "init-ords",
					Image:   m.Spec.Image.PullFrom,
					Command: []string{"/bin/sh", "-c", dbcommons.InitORDSCMD},
					Env: func() []corev1.EnvVar {
						adminSecretName, adminSecretKey, _, adminSecretRefFound := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
						ordsSecretName, ordsSecretKey, _, ordsSecretRefFound := dbapi.ResolveOracleRestDataServiceOrdsSecretRef(m)
						env := []corev1.EnvVar{
							{Name: "SETUP_ONLY", Value: "true"},
							{Name: "ORACLE_HOST", Value: n.Name},
							{Name: "ORACLE_PORT", Value: "1521"},
							{Name: "DBSERVICENAME", Value: func() string {
								if n.Status.Pdbname != "" {
									return n.Status.Pdbname
								}
								if n.Spec.Pdbname != "" {
									return n.Spec.Pdbname
								}
								return n.Spec.Sid
							}()},
							{Name: "ORACLE_SERVICE", Value: func() string {
								if m.Spec.OracleService != "" {
									return m.Spec.OracleService
								}
								return n.Spec.Sid
							}()},
							{Name: "ORDS_USER", Value: func() string {
								if m.Spec.OrdsUser != "" {
									return m.Spec.OrdsUser
								}
								return "ORDS_PUBLIC_USER"
							}()},
						}
						if adminSecretRefFound {
							env = append(env, corev1.EnvVar{Name: "ORACLE_PWD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: adminSecretName}, Key: adminSecretKey,
							}}})
						}
						if ordsSecretRefFound {
							env = append(env, corev1.EnvVar{Name: "ORDS_PWD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: ordsSecretName}, Key: ordsSecretKey,
							}}})
						}
						return env
					}(),
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/etc/ords/config/",
							Name:      "datamount",
						},
						{
							MountPath: "/opt/oracle/variables/",
							Name:      "varmount",
						},
					},
				})
				return initContainers
			}(),
			Containers: []corev1.Container{{
				Name:    m.Name,
				Image:   m.Spec.Image.PullFrom,
				Command: []string{"/usr/bin/ords"},
				Args:    []string{"--config", "/etc/ords/config", "serve", "--port", strconv.FormatInt(int64(ordsHTTPPort), 10)},
				Ports: func() []corev1.ContainerPort {
					ports := []corev1.ContainerPort{
						{
							ContainerPort: ordsHTTPPort,
						},
					}
					if m.Spec.MongoDbApi {
						ports = append(ports, corev1.ContainerPort{
							ContainerPort: 27017, // MongoDB port
						})
					}
					return ports
				}(),
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c", fmt.Sprintf(dbcommons.ORDSReadinessProbe, ordsHTTPPort)},
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
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						MountPath: "/etc/ords/config/",
						Name:      "datamount",
					},
					{
						MountPath: "/opt/oracle/variables/",
						Name:      "varmount",
					},
				},
				Env: func() []corev1.EnvVar {
					// After ORDS is Installed, we DELETE THE OLD ORDS Pod and create new ones ONLY USING BELOW ENV VARIABLES.
					adminSecretName, adminSecretKey, _, adminSecretRefFound := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
					pdbService := n.Status.Pdbname
					if strings.TrimSpace(pdbService) == "" {
						pdbService = n.Spec.Sid
					}
					env := []corev1.EnvVar{
						{
							Name:  "ORACLE_HOST",
							Value: n.Name,
						},
						{
							Name:  "ORACLE_PORT",
							Value: "1521",
						},
						{
							Name: "ORACLE_SERVICE",
							Value: func() string {
								if m.Spec.OracleService != "" {
									return m.Spec.OracleService
								}
								return n.Spec.Sid
							}(),
						},
						{
							Name: "ORDS_USER",
							Value: func() string {
								if m.Spec.OrdsUser != "" {
									return m.Spec.OrdsUser
								}
								return "ORDS_PUBLIC_USER"
							}(),
						},
					}
					if adminSecretRefFound {
						env = append(env, corev1.EnvVar{Name: "DBHOST", Value: n.Name})
						env = append(env, corev1.EnvVar{Name: "DBPORT", Value: "1521"})
						env = append(env, corev1.EnvVar{Name: "DBSERVICENAME", Value: pdbService})
						env = append(env, corev1.EnvVar{
							Name: "ORACLE_PWD",
							ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: adminSecretName},
								Key:                  adminSecretKey,
							}},
						})
					}
					return env
				}(),
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
			ServiceAccountName: func() string {
				if m.Spec.ServiceAccountName != "" {
					return m.Spec.ServiceAccountName
				}
				return "default"
			}(),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  func() *int64 { i := int64(dbcommons.ORACLE_UID); return &i }(),
				RunAsGroup: func() *int64 { i := int64(dbcommons.DBA_GUID); return &i }(),
				FSGroup:    func() *int64 { i := int64(dbcommons.DBA_GUID); return &i }(),
			},

			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: m.Spec.Image.PullSecrets,
				},
			},
		},
	}

	// Set oracleRestDataService instance as the owner and controller
	// ctrl.SetControllerReference(m, initSecret, r.Scheme)
	_ = ctrl.SetControllerReference(m, pod, r.Scheme)
	return pod
}

//#############################################################################
//    Instantiate POD spec from OracleRestDataService spec
//#############################################################################

// #############################################################################
//
//	Instantiate Persistent Volume Claim spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) instantiatePVCSpec(m *dbapi.OracleRestDataService) *corev1.PersistentVolumeClaim {

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
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: func() []corev1.PersistentVolumeAccessMode {
				var accessMode []corev1.PersistentVolumeAccessMode
				accessMode = append(accessMode, corev1.PersistentVolumeAccessMode(m.Spec.Persistence.AccessMode))
				return accessMode
			}(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					// Requests describes the minimum amount of compute resources required
					"storage": resource.MustParse(m.Spec.Persistence.Size),
				},
			},
			StorageClassName: &m.Spec.Persistence.StorageClass,
			VolumeName:       m.Spec.Persistence.VolumeName,
			Selector: func() *metav1.LabelSelector {
				if m.Spec.Persistence.StorageClass != "oci" {
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

// #############################################################################
//
//	Create a Service for OracleRestDataService
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createSVC(ctx context.Context, req ctrl.Request,
	m *dbapi.OracleRestDataService, _ *dbapi.SingleInstanceDatabase) ctrl.Result {

	log := r.Log.WithValues("createSVC", req.NamespacedName)
	// Check if the Service already exists, if not create a new one
	svc := &corev1.Service{}
	svcDeleted := false
	// Check if the Service already exists, if not create a new one
	// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, svc)
	if err == nil {
		log.Info("Found Existing Service ", "Service.Name", svc.Name)
		svcType := corev1.ServiceType("NodePort")
		if m.Spec.LoadBalancer {
			svcType = corev1.ServiceType("LoadBalancer")
		}

		if svc.Spec.Type != svcType {
			log.Info("Deleting SVC", " name ", svc.Name)
			err = r.Delete(ctx, svc)
			if err != nil {
				r.Log.Error(err, "Failed to delete svc", " Name", svc.Name)
				return requeueN
			}
			svcDeleted = true
		}
	}

	if svcDeleted || (err != nil && apierrors.IsNotFound(err)) {
		// Define a new Service
		svc = r.instantiateSVCSpec(m)
		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY
		}
		eventReason := "Service creation"
		eventMsg := "successfully created service type " + string(svc.Spec.Type)
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
		log.Info(eventMsg)

	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return requeueY
	}

	m.Status.ServiceIP = ""
	if m.Spec.LoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			// 'lbAddress' will contain the Fully Qualified Hostname of the LB. If the hostname is not available it will contain the IP address of the LB
			lbAddress := svc.Status.LoadBalancer.Ingress[0].Hostname
			if lbAddress == "" {
				lbAddress = svc.Status.LoadBalancer.Ingress[0].IP
			}
			m.Status.DatabaseApiUrl = "http://" + lbAddress + ":" +
				fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/" + "{pdb-name}/{schema-name}" + "/_/db-api/stable/"
			m.Status.ServiceIP = lbAddress
			m.Status.DatabaseActionsUrl = "http://" + lbAddress + ":" +
				fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/sql-developer"
			if m.Status.ApexConfigured {
				m.Status.ApxeUrl = "http://" + lbAddress + ":" +
					fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/apex"
			}
			if m.Status.MongoDbApi && len(svc.Spec.Ports) > 1 {
				m.Status.MongoDbApiAccessUrl = "mongodb://[{user}:{password}@]" + lbAddress + ":" +
					fmt.Sprint(svc.Spec.Ports[1].Port) + "/{user}?" +
					"authMechanism=PLAIN&authSource=$external&ssl=true&retryWrites=false&loadBalanced=true"
			} else {
				m.Status.MongoDbApiAccessUrl = ""
			}
		}
		return requeueN
	}
	nodeip := dbcommons.GetNodeIp(r, ctx, req)
	if nodeip != "" {
		m.Status.ServiceIP = nodeip
		m.Status.DatabaseApiUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) +
			"/ords/" + "{pdb-name}/{schema-name}" + "/_/db-api/stable/"
		m.Status.DatabaseActionsUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) +
			"/ords/sql-developer"
		if m.Status.ApexConfigured {
			m.Status.ApxeUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/ords/apex"
		}
		if m.Status.MongoDbApi && len(svc.Spec.Ports) > 1 {
			m.Status.MongoDbApiAccessUrl = "mongodb://[{user}:{password}@]" + nodeip + ":" +
				fmt.Sprint(svc.Spec.Ports[1].NodePort) + "/{user}?" +
				"authMechanism=PLAIN&authSource=$external&ssl=true&retryWrites=false&loadBalanced=true"
		} else {
			m.Status.MongoDbApiAccessUrl = ""
		}
	}
	return requeueN
}

// #############################################################################
//
//	Stake a claim for Persistent Volume
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createPVC(ctx context.Context, req ctrl.Request,
	m *dbapi.OracleRestDataService) (ctrl.Result, error) {

	// PV is shared for ORDS and SIDB
	if m.Spec.Persistence.AccessMode == "" {
		return requeueN, nil
	}
	log := r.Log.WithValues("createPVC", req.NamespacedName)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, pvc)
	if err != nil && apierrors.IsNotFound(err) {
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
	log.Info("PVC already exists")

	return requeueN, nil
}

// #############################################################################
//
//	Function for creating connection sting file
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createConnectionString(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

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

	initStatus := pod.Status.InitContainerStatuses[lastInitContIndex]
	if initStatus.State.Terminated != nil {
		if initStatus.State.Terminated.ExitCode != 0 {
			r.Log.Info("init-ords terminated unsuccessfully", "exitCode", initStatus.State.Terminated.ExitCode)
			return requeueY, nil
		}
		// A successful init container is terminated before the main container starts.
	}

	if initStatus.State.Terminated == nil && initStatus.State.Running == nil {
		// Init container named "init-ords" is not running, so waiting for it to come in running state requeueing the reconcile request
		r.Log.Info("Waiting for init-ords to come in running state...")
		return requeueY, nil
	}

	mainContainerRunning := false
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == m.Name {
			mainContainerRunning = containerStatus.State.Running != nil
			break
		}
	}
	if !mainContainerRunning {
		r.Log.Info("Waiting for the ORDS container to be created and running", "container", m.Name)
		return requeueY, nil
	}

	r.Log.Info("Creating Connection String file...")

	// Querying the secret
	r.Log.Info("Querying the database secret ...")
	adminSecretName, adminSecretKey, _, ok := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
	if !ok {
		eventReason := "Database Password"
		eventMsg := "database admin password secret reference is not set"
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		r.Log.Info(eventMsg)
		m.Status.Status = dbcommons.StatusError
		if updateErr := r.Status().Update(ctx, m); updateErr != nil {
			r.Log.Error(updateErr, "failed to update ORDS status")
		}
		return requeueY, nil
	}
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: adminSecretName, Namespace: m.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			eventReason := "Database Password"
			eventMsg := "database admin password secret " + adminSecretName + " not found, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			r.Log.Info(eventMsg)
			m.Status.Status = dbcommons.StatusError
			if updateErr := r.Status().Update(ctx, m); updateErr != nil {
				r.Log.Error(updateErr, "failed to update ORDS status")
			}
			return requeueY, nil
		}
		r.Log.Error(err, "Unable to get the secret. Requeueing..")
		return requeueY, nil
	}

	// Execing into the pods and creating the Connection String
	adminPassword, err := oracleRestPasswordFromSecret(secret, adminSecretKey)
	if err != nil {
		eventReason := "Database Password"
		eventMsg := "database admin password secret is invalid: " + err.Error()
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		r.Log.Info(eventMsg, "secret", adminSecretName)
		m.Status.Status = dbcommons.StatusError
		if updateErr := r.Status().Update(ctx, m); updateErr != nil {
			r.Log.Error(updateErr, "failed to update ORDS status")
		}
		return requeueY, nil
	}

	_, err = dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, m.Name,
		ctx, req, true, "bash", "-c",
		fmt.Sprintf("mkdir -p /opt/oracle/variables && echo %[1]s > /opt/oracle/variables/%[2]s",
			fmt.Sprintf(dbcommons.DbConnectString, adminPassword, n.Name, n.Status.Pdbname),
			"conn_string.txt"))

	if err != nil {
		r.Log.Error(err, err.Error())
		r.Log.Error(err, "Failed to create connection string in new "+m.Name+" POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
		return requeueY, nil
	}
	r.Log.Info("Succesfully Created connection string in new "+m.Name+" POD", "POD.NAME : ", pod.Name)

	return requeueN, nil
}

// #############################################################################
//
//	Create the requested POD replicas
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createPods(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("createPods", req.NamespacedName)

	readyPod, replicasFound, available, podsMarkedToBeDeleted, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// Recreate new pods only after earlier pods are terminated completely
	for i := 0; i < len(podsMarkedToBeDeleted); i++ {
		r.Log.Info("Force deleting pod ", "name", podsMarkedToBeDeleted[i].Name, "phase", podsMarkedToBeDeleted[i].Status.Phase)
		var gracePeriodSeconds int64
		policy := metav1.DeletePropagationForeground
		if err := r.Delete(ctx, &podsMarkedToBeDeleted[i], &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy}); err != nil {
			r.Log.Error(err, "failed to delete ORDS pod", "name", podsMarkedToBeDeleted[i].Name)
		}
	}

	log.Info(m.Name, " pods other than one of Ready Pods : ", dbcommons.GetPodNames(available))
	log.Info(m.Name, " Ready Pod : ", readyPod.Name)

	replicasReq := m.Spec.Replicas
	if replicasFound == 0 {
		m.Status.Status = dbcommons.StatusPending
	}

	if replicasFound == replicasReq {
		log.Info("No of " + m.Name + " replicas Found are same as Required")
	} else if replicasFound < replicasReq {
		// Create New Pods , Name of Pods are generated Randomly
		for i := replicasFound; i < replicasReq; i++ {
			pod := r.instantiatePodSpec(m, n, req)

			log.Info("Creating a new "+m.Name+" POD", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
			err := r.Create(ctx, pod)
			if err != nil {
				log.Error(err, "Failed to create new "+m.Name+" POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
				return requeueY
			}
			log.Info("Succesfully Created new "+m.Name+" POD", "POD.NAME : ", pod.Name)
		}
	} else {
		// Delete extra pods
		noDeleted := 0
		if readyPod.Name != "" {
			available = append(available, readyPod)
		}
		for _, pod := range available {
			if readyPod.Name == pod.Name {
				continue
			}
			if replicasReq == (len(available) - noDeleted) {
				break
			}
			r.Log.Info("Deleting Pod : ", "POD.NAME", pod.Name)
			var gracePeriodSeconds int64
			policy := metav1.DeletePropagationForeground
			err := r.Delete(ctx, &pod, &client.DeleteOptions{
				GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
			noDeleted++
			if err != nil {
				r.Log.Error(err, "Failed to delete existing POD", "POD.Name", pod.Name)
				// Don't requeue
			}
		}
	}

	// Creating conn string in pods
	result, err := r.createConnectionString(m, n, ctx, req)

	if err != nil {
		return requeueY
	}
	if result.Requeue {
		log.Info("Requeued at connection string creation")
		return requeueY
	}

	m.Status.Replicas = m.Spec.Replicas

	return requeueN
}

// #############################################################################
//
//	Manage Finalizer to cleanup before deletion of OracleRestDataService
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) manageOracleRestDataServiceDeletion(req ctrl.Request, ctx context.Context,
	m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) ctrl.Result {
	log := r.Log.WithValues("manageOracleRestDataServiceDeletion", req.NamespacedName)

	// Check if the OracleRestDataService instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isOracleRestDataServiceMarkedToBeDeleted := m.GetDeletionTimestamp() != nil
	if isOracleRestDataServiceMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(m, oracleRestDataServiceFinalizer) {
			// Run finalization logic for oracleRestDataServiceFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.cleanupOracleRestDataService(req, ctx, m, n); err != nil {
				log.Error(err, err.Error())
				return requeueY
			}

			n.Status.OrdsReference = ""
			// Make sure n.Status.OrdsInstalled is set to false or else it blocks .spec.databaseRef deletion
			for i := 0; i < 10; i++ {
				log.Info("Clearing the OrdsReference from DB", "name", n.Name)
				err := r.Status().Update(ctx, n)
				if err != nil {
					log.Error(err, err.Error())
					time.Sleep(1 * time.Second)
					continue
				}
				break
			}

			// Remove oracleRestDataServiceFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(m, oracleRestDataServiceFinalizer)
			err := r.Update(ctx, m)
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
		}
		return requeueY
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(m, oracleRestDataServiceFinalizer) {
		controllerutil.AddFinalizer(m, oracleRestDataServiceFinalizer)
		err := r.Update(ctx, m)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
	}
	return requeueN
}

// #############################################################################
//
//	Finalization logic for OracleRestDataServiceFinalizer
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) cleanupOracleRestDataService(req ctrl.Request, ctx context.Context,
	m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) error {
	log := r.Log.WithValues("cleanupOracleRestDataService", req.NamespacedName)

	if m.Status.OrdsInstalled {
		// ## FETCH THE SIDB REPLICAS .
		sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
			n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}

		if sidbReadyPod.Name == "" {
			eventReason := "ORDS Uninstallation"
			eventMsg := "skipping ORDS uninstallation as no ready pod for " + n.Name + " is available"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
			return nil
		}

		// Get Session id , serial# for ORDS_PUBLIC_USER to kill the sessions
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s ", dbcommons.GetSessionInfoSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("GetSessionInfoSQL Output : " + out)

		sessionInfos, _ := dbcommons.StringToLines(out)
		killSessions := ""
		for _, sessionInfo := range sessionInfos {
			if !strings.Contains(sessionInfo, ",") {
				// May be a column name or (-----)
				continue
			}
			killSessions += "\n" + fmt.Sprintf(dbcommons.KillSessionSQL, sessionInfo)
		}

		//kill all the sessions with given sid,serial#
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s ", killSessions, dbcommons.SQLPlusCLI))

		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("KillSession Output : " + out)

		// Fetch admin Password of database to uninstall ORDS
		adminPasswordSecret := &corev1.Secret{}
		adminPasswordSecretFound := false
		adminSecretName, adminSecretKey, adminKeepSecret, adminSecretRefFound := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
		if !adminSecretRefFound {
			m.Status.Status = dbcommons.StatusError
			eventReason := "Error"
			eventMsg := "database admin password secret reference is required for ORDS uninstall"
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			r.Log.Info(eventMsg)
		}
		for i := 0; i < 5; i++ {
			if !adminSecretRefFound {
				break
			}
			err := r.Get(ctx, types.NamespacedName{Name: adminSecretName, Namespace: m.Namespace}, adminPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					m.Status.Status = dbcommons.StatusError
					eventReason := "Error"
					eventMsg := "database admin password secret " + adminSecretName + " required for ORDS uninstall not found, retrying..."
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
					r.Log.Info(eventMsg)
					if i < 4 {
						time.Sleep(15 * time.Second)
						continue
					}
				} else {
					log.Error(err, err.Error())
				}
			} else {
				adminPasswordSecretFound = true
				break
			}
		}
		// Find ORDS ready pod
		readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
			m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		if adminPasswordSecretFound && readyPod.Name != "" {
			adminPassword, passwordErr := oracleRestPasswordFromSecret(adminPasswordSecret, adminSecretKey)
			if passwordErr != nil {
				log.Error(passwordErr, "invalid database admin password secret", "secret", adminSecretName)
				return passwordErr
			}
			if n.Status.ApexInstalled {
				//Uninstall Apex
				eventReason := "Apex Uninstallation"
				eventMsg := "Uninstalling Apex..."
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
				log.Info(eventMsg)
				out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
					fmt.Sprintf(dbcommons.UninstallApex, adminPassword, n.Status.Pdbname))
				if err != nil {
					log.Info(err.Error())
				}
				n.Status.ApexInstalled = false // To reinstall Apex when ORDS is reinstalled
				log.Info("Apex uninstall output: " + out)
			}
			//Uninstall ORDS
			eventReason := "ORDS Uninstallation"
			eventMsg := "Uninstalling ORDS..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			log.Info(eventMsg)
			uninstallORDS := fmt.Sprintf(dbcommons.UninstallORDSCMD, adminPassword)
			out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
				uninstallORDS)
			log.Info("ORDS uninstall output: " + out)
			if strings.Contains(strings.ToUpper(out), "ERROR") {
				return errors.New(out)
			}
			if err != nil {
				log.Info(err.Error())
			}
		}

		// Drop Admin Users
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s ", dbcommons.DropAdminUsersSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Info(err.Error())
		}
		log.Info("Drop admin users: " + out)

		//Delete ORDS pod
		var gracePeriodSeconds int64
		policy := metav1.DeletePropagationForeground
		if err := r.Delete(ctx, &readyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy}); err != nil {
			r.Log.Error(err, "failed to delete ready ORDS pod", "name", readyPod.Name)
		}

		//Delete Database Admin Password Secret
		if adminPasswordSecretFound && !adminKeepSecret {
			err = r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				r.Log.Info("Deleted Admin Password Secret :" + adminPasswordSecret.Name)
			}
		}
	}

	// Cleanup steps that the operator needs to do before the CR can be deleted.
	log.Info("Successfully cleaned up OracleRestDataService ")
	return nil
}

// #############################################################################
//
//	Configure APEX
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) configureApex(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	_ corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("verifyApex", req.NamespacedName)

	if m.Status.ApexConfigured {
		return requeueN
	}

	// Obtain admin password of the referred database

	adminSecretName, adminSecretKey, _, ok := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
	if !ok {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Password"
		eventMsg := "database admin password secret reference is not set"
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		log.Info(eventMsg)
		return requeueY
	}
	adminPasswordSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: adminSecretName, Namespace: m.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			m.Status.Status = dbcommons.StatusError
			eventReason := "Database Password"
			eventMsg := "password secret " + adminSecretName + " not found, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			r.Log.Info(eventMsg)
			return requeueY
		}
		log.Error(err, err.Error())
		return requeueY
	}
	sidbPassword, err := oracleRestPasswordFromSecret(adminPasswordSecret, adminSecretKey)
	if err != nil {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Database Password"
		eventMsg := "database admin password secret is invalid: " + err.Error()
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		log.Info(eventMsg, "secret", adminSecretName)
		return requeueY
	}

	// Checking if Apex is installed successfully or not
	out, err := dbcommons.ExecCommand(r, r.Config, ordsReadyPod.Name, ordsReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.IsApexInstalled, sidbPassword, n.Status.Pdbname))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("Is Apex installed: \n" + out)

	apexInstalled := "APEXVERSION:"
	if !strings.Contains(out, apexInstalled) {
		if strings.Contains(strings.ToLower(out), "no rows selected") {
			m.Status.Status = dbcommons.StatusReady
			r.Recorder.Eventf(m, corev1.EventTypeNormal, "Apex Verification", "APEX is not installed; skipping APEX configuration")
			return requeueN
		}
		eventReason := "Apex Verification"
		eventMsg := "Unable to determine Apex version, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		return requeueY
	}

	m.Status.Status = dbcommons.StatusReady
	eventReason := "Apex Verification"
	outArr := strings.Split(out, apexInstalled)
	eventMsg := "Verification of Apex " + strings.TrimSpace(outArr[len(outArr)-1]) + " completed"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
	n.Status.ApexInstalled = true
	m.Status.ApexConfigured = true
	if err := r.Status().Update(ctx, n); err != nil {
		r.Log.Error(err, "failed to update SIDB status for Apex")
		return requeueY
	}
	if err := r.Status().Update(ctx, m); err != nil {
		r.Log.Error(err, "failed to update ORDS status for Apex")
		return requeueY
	}

	return requeueN
}

// #############################################################################
//
//	Delete Secrets
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) deleteSecrets(m *dbapi.OracleRestDataService, ctx context.Context, req ctrl.Request) {
	log := r.Log.WithValues("deleteSecrets", req.NamespacedName)

	adminSecretName, _, adminKeepSecret, adminSecretRefFound := dbapi.ResolveOracleRestDataServiceAdminSecretRef(m)
	if adminSecretRefFound && !adminKeepSecret {
		// Fetch adminPassword Secret
		adminPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: adminSecretName, Namespace: m.Namespace}, adminPasswordSecret)
		if err == nil {
			//Delete Database Admin Password Secret .
			err := r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("Database admin password secret deleted : " + adminPasswordSecret.Name)
			}
		}
	}

	ordsSecretName, _, ordsKeepSecret, ordsSecretRefFound := dbapi.ResolveOracleRestDataServiceOrdsSecretRef(m)
	if ordsSecretRefFound && !ordsKeepSecret {
		// Fetch ordsPassword Secret
		ordsPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: ordsSecretName, Namespace: m.Namespace}, ordsPasswordSecret)
		if err == nil {
			//Delete ORDS Password Secret .
			err := r.Delete(ctx, ordsPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("ORDS password secret deleted : " + ordsPasswordSecret.Name)
			}
		}
	}
}

// #############################################################################
//
//	Enable MongoDB API Support
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) enableMongoDB(m *dbapi.OracleRestDataService, _ *dbapi.SingleInstanceDatabase,
	_ corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("enableMongoDB", req.NamespacedName)

	if (m.Spec.MongoDbApi && !m.Status.MongoDbApi) || // setting MongoDbApi to true
		(!m.Spec.MongoDbApi && m.Status.MongoDbApi) { // setting MongoDbApi to false
		m.Status.Status = dbcommons.StatusUpdating

		out, err := dbcommons.ExecCommand(r, r.Config, ordsReadyPod.Name, ordsReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf(dbcommons.ConfigMongoDb, strconv.FormatBool(m.Spec.MongoDbApi)))
		log.Info("configMongoDB Output: \n" + out)

		if strings.Contains(strings.ToUpper(out), "ERROR") {
			return requeueY
		}
		if err != nil {
			log.Info(err.Error())
			if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
				return requeueY
			}
		}

		m.Status.MongoDbApi = m.Spec.MongoDbApi
		m.Status.Status = dbcommons.StatusReady
		if err := r.Status().Update(ctx, m); err != nil {
			log.Error(err, "failed to update ORDS status for MongoDB API configuration")
			return requeueY
		}
		eventReason := "MongoDB-API Config"
		eventMsg := "configuration of MongoDb API completed!"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, "%s", eventMsg)
		log.Info(eventMsg)

		// ORDS service is resatrted
		r.Log.Info("Restarting ORDS Service : " + m.Name)
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace},
		}
		var gracePeriodSeconds int64
		policy := metav1.DeletePropagationForeground
		err = r.Delete(ctx, svc, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, "Failed to delete ORDS service", "Service Name", m.Name)
			return requeueY
		}

		// ORDS needs to be restarted to configure MongoDB API
		r.Log.Info("Restarting ORDS Pod after configuring MongoDb API : " + ordsReadyPod.Name)
		err = r.Delete(ctx, &ordsReadyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, err.Error())
		}
		return requeueY
	}

	log.Info("MongoDB Already Configured")

	return requeueN
}

// #############################################################################
//
//	Rest Enable/Disable Schemas
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) restEnableSchemas(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("restEnableSchemas", req.NamespacedName)

	if sidbReadyPod.Name == "" || n.Status.Status != dbcommons.StatusReady {
		eventReason := "Database Check"
		eventMsg := "status of database " + n.Name + " is not ready, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		m.Status.Status = dbcommons.StatusNotReady
		return requeueY
	}

	// Get available PDBs
	availablePDBS, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
		ctx, req, true, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.GetPdbsSQL, dbcommons.SQLPlusCLI))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("PDBs found:")
	log.Info(availablePDBS)

	restartORDS := false

	for i := 0; i < len(m.Spec.RestEnableSchemas); i++ {
		if err := dbapi.ValidateOracleRestDataServiceRestEnableSchema(m.Spec.RestEnableSchemas[i]); err != nil {
			eventReason := "Invalid RestEnableSchemas"
			eventMsg := fmt.Sprintf("Invalid restEnableSchemas[%d]: %v", i, err)
			log.Info(eventMsg)
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			m.Status.Status = dbcommons.StatusError
			return requeueN
		}

		pdbName := m.Spec.RestEnableSchemas[i].PdbName
		if pdbName == "" {
			pdbName = n.Spec.Pdbname
		}

		//  If the PDB mentioned in yaml doesnt contain in the database , continue
		if !strings.Contains(strings.ToUpper(availablePDBS), strings.ToUpper(pdbName)) {
			eventReason := "PDB Check"
			eventMsg := "PDB " + pdbName + " not found for specified schema " + m.Spec.RestEnableSchemas[i].SchemaName
			log.Info(eventMsg)
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
			continue
		}

		getOrdsSchemaStatus := fmt.Sprintf(dbcommons.GetUserORDSSchemaStatusSQL, m.Spec.RestEnableSchemas[i].SchemaName, pdbName)

		// Get ORDS Schema status for PDB
		out, err := runOracleRestSQLPlusScript(r, sidbReadyPod, getOrdsSchemaStatus, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}

		// if ORDS already enabled for given PDB
		if strings.Contains(out, "STATUS:ENABLED") {
			if m.Spec.RestEnableSchemas[i].Enable {
				log.Info("Schema already enabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
				continue
			}
		} else if strings.Contains(out, "STATUS:DISABLED") {
			if !m.Spec.RestEnableSchemas[i].Enable {
				log.Info("Schema already disabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
				continue
			}
		} else if m.Spec.RestEnableSchemas[i].Enable {
			OrdsPasswordSecret := &corev1.Secret{}
			ordsSecretName, ordsSecretKey, _, ok := dbapi.ResolveOracleRestDataServiceOrdsSecretRef(m)
			if !ok {
				eventReason := "No Secret"
				eventMsg := "ORDS public user password secret reference is not set"
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
				r.Log.Info(eventMsg)
				m.Status.Status = dbcommons.StatusError
				return requeueY
			}
			// Fetch the secret to get password for database user . Secret has to be created in the same namespace of OracleRestDataService
			err = r.Get(ctx, types.NamespacedName{Name: ordsSecretName, Namespace: m.Namespace}, OrdsPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					eventReason := "No Secret"
					eventMsg := "secret " + ordsSecretName + " Not Found"
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
					r.Log.Info(eventMsg)
					return requeueY
				}
				log.Error(err, err.Error())
				return requeueY
			}
			password, err := oracleRestPasswordFromSecret(OrdsPasswordSecret, ordsSecretKey)
			if err != nil {
				eventReason := "Invalid OrdsPassword"
				eventMsg := "ords password secret is invalid: " + err.Error()
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
				log.Info(eventMsg, "secret", ordsSecretName)
				m.Status.Status = dbcommons.StatusError
				return requeueY
			}
			if err := dbcommons.ValidateOracleSQLPassword(password); err != nil {
				eventReason := "Invalid OrdsPassword"
				eventMsg := "ords password secret contains unsupported characters for SQL setup: " + err.Error()
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
				log.Info(eventMsg, "secret", ordsSecretName)
				m.Status.Status = dbcommons.StatusError
				return requeueY
			}
			// Create users,schemas and grant enableORDS for PDB
			createSchemaSQL := fmt.Sprintf(dbcommons.CreateORDSSchemaSQL, m.Spec.RestEnableSchemas[i].SchemaName, password, pdbName)
			log.Info("Creating schema", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			_, err = runOracleRestSQLPlusScript(r, sidbReadyPod, createSchemaSQL, ctx, req)
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
		} else {
			log.Info("Noop, ignoring", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			continue
		}
		urlMappingPattern := ""
		if m.Spec.RestEnableSchemas[i].UrlMapping == "" {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].SchemaName)
		} else {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].UrlMapping)
		}
		enableORDSSchema := fmt.Sprintf(dbcommons.EnableORDSSchemaSQL, m.Spec.RestEnableSchemas[i].SchemaName,
			strconv.FormatBool(m.Spec.RestEnableSchemas[i].Enable), urlMappingPattern, pdbName)

		// EnableORDS for Schema
		out, err = runOracleRestSQLPlusScript(r, sidbReadyPod, enableORDSSchema, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info(out)
		if m.Spec.RestEnableSchemas[i].Enable {
			log.Info("REST Enabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
		} else {
			log.Info("REST Disabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			restartORDS = true
		}
	}

	if restartORDS {
		r.Log.Info("Restarting ORDS Pod " + ordsReadyPod.Name + " to clear disabled schemas cache")
		var gracePeriodSeconds int64
		policy := metav1.DeletePropagationForeground
		err = r.Delete(ctx, &ordsReadyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, err.Error())
		}
		return requeueY
	}
	return requeueN
}

// SetupWithManager sets up the OracleRestDataService controller with the manager.
func (r *OracleRestDataServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.OracleRestDataService{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of OracleRestDataService Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
