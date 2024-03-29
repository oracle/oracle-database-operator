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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
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

// SingleInstanceDatabaseReconciler reconciles a SingleInstanceDatabase object
type SingleInstanceDatabaseReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

// To requeue after 15 secs allowing graceful state changes
var requeueY ctrl.Result = ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}
var requeueN ctrl.Result = ctrl.Result{}

// For scheduling reconcile to renew certs if TCPS is enabled
// Default value is requeueN (No reconcile)
var futureRequeue ctrl.Result = requeueN

const singleInstanceDatabaseFinalizer = "database.oracle.com/singleinstancedatabasefinalizer"

var oemExpressUrl string

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

	singleInstanceDatabase := &dbapi.SingleInstanceDatabase{}
	cloneFromDatabase := &dbapi.SingleInstanceDatabase{}
	referredPrimaryDatabase := &dbapi.SingleInstanceDatabase{}

	// Execute for every reconcile
	defer r.updateReconcileStatus(singleInstanceDatabase, ctx, &result, &err, &blocked, &completed)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		return requeueY, err
	}

	/* Initialize Status */
	if singleInstanceDatabase.Status.Status == "" {
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
		r.Status().Update(ctx, singleInstanceDatabase)
	}

	// Manage SingleInstanceDatabase Deletion
	result, err = r.manageSingleInstanceDatabaseDeletion(req, ctx, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Error(err, err.Error())
		return result, err
	}

	// First validate
	result, err = r.validate(singleInstanceDatabase, cloneFromDatabase, referredPrimaryDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Spec validation failed, Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Info("Spec validation failed")
		return result, nil
	}

	// PVC Creation for Datafiles Volume
	result, err = r.createOrReplacePVCforDatafilesVol(ctx, req, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// PVC Creation for customScripts Volume
	result, err = r.createOrReplacePVCforCustomScriptsVol(ctx, req, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// POD creation
	result, err = r.createOrReplacePods(singleInstanceDatabase, cloneFromDatabase, referredPrimaryDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Service creation
	result, err = r.createOrReplaceSVC(ctx, req, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Validate readiness
	result, readyPod, err := r.validateDBReadiness(singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Post DB ready operations

	// Deleting the oracle wallet
	if singleInstanceDatabase.Status.DatafilesCreated == "true" {
		result, err = r.deleteWallet(singleInstanceDatabase, ctx, req)
		if result.Requeue {
			r.Log.Info("Reconcile queued")
			return result, nil
		}
	}

	sidbRole, err := dbcommons.GetDatabaseRole(readyPod, r, r.Config, ctx, req)

	if sidbRole == "PRIMARY" {

		// Update DB config
		result, err = r.updateDBConfig(singleInstanceDatabase, readyPod, ctx, req)
		if result.Requeue {
			r.Log.Info("Reconcile queued")
			return result, nil
		}

		// Update Init Parameters
		result, err = r.updateInitParameters(singleInstanceDatabase, readyPod, ctx, req)
		if result.Requeue {
			r.Log.Info("Reconcile queued")
			return result, nil
		}

		// Configure TCPS
		result, err = r.configTcps(singleInstanceDatabase, readyPod, ctx, req)
		if result.Requeue {
			r.Log.Info("Reconcile queued")
			return result, nil
		}

	} else {
		// Database is in role of standby
		if !singleInstanceDatabase.Status.DgBrokerConfigured {
			err = SetupStandbyDatabase(r, singleInstanceDatabase, referredPrimaryDatabase, ctx, req)
			if err != nil {
				return requeueY, err
			}
		}

		databaseOpenMode, err := dbcommons.GetDatabaseOpenMode(readyPod, r, r.Config, ctx, req, singleInstanceDatabase.Spec.Edition)

		if err != nil {
			r.Log.Error(err, err.Error())
			return requeueY, err
		}
		r.Log.Info("DB openMode Output")
		r.Log.Info(databaseOpenMode)
		if databaseOpenMode == "READ_ONLY" || databaseOpenMode == "MOUNTED" {
			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.ModifyStdbyDBOpenMode, dbcommons.SQLPlusCLI))
			if err != nil {
				r.Log.Error(err, err.Error())
				return requeueY, err
			}
			r.Log.Info("Standby DB open mode modified")
			r.Log.Info(out)
		}

		singleInstanceDatabase.Status.PrimaryDatabase = referredPrimaryDatabase.Name
		// Store all standbyDatabase sid:name in a map to use it during manual switchover.
		if len(referredPrimaryDatabase.Status.StandbyDatabases) == 0 {
			referredPrimaryDatabase.Status.StandbyDatabases = make(map[string]string)
		}
		referredPrimaryDatabase.Status.StandbyDatabases[strings.ToUpper(singleInstanceDatabase.Spec.Sid)] = singleInstanceDatabase.Name
		r.Status().Update(ctx, referredPrimaryDatabase)

	}

	// Run Datapatch
	if strings.ToUpper(singleInstanceDatabase.Status.Role) == "PRIMARY" && singleInstanceDatabase.Status.DatafilesPatched != "true" {
		// add a blocking reconcile condition
		err = errors.New("processing datapatch execution")
		blocked = true
		r.updateReconcileStatus(singleInstanceDatabase, ctx, &result, &err, &blocked, &completed)
		result, err = r.runDatapatch(singleInstanceDatabase, readyPod, ctx, req)
		if result.Requeue {
			r.Log.Info("Reconcile queued")
			return result, nil
		}
	}

	// If LoadBalancer = true , ensure Connect String is updated
	if singleInstanceDatabase.Status.ConnectString == dbcommons.ValueUnavailable {
		r.Log.Info("Connect string not available for the database " + singleInstanceDatabase.Name)
		return requeueY, nil
	}

	// updating singleinstancedatabase Status
	err = r.updateSidbStatus(singleInstanceDatabase, readyPod, ctx, req)
	if err != nil {
		return requeueY, err
	}
	r.updateORDSStatus(singleInstanceDatabase, ctx, req)

	completed = true
	r.Log.Info("Reconcile completed")

	// Scheduling a reconcile for certificate renewal, if TCPS is enabled
	if futureRequeue != requeueN {
		r.Log.Info("Scheduling Reconcile for cert renewal", "Duration(Hours)", futureRequeue.RequeueAfter.Hours())
		copyFutureRequeue := futureRequeue
		futureRequeue = requeueN
		return copyFutureRequeue, nil
	}

	return requeueN, nil
}

// #############################################################################
//
//	Update each reconcile condtion/status
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) updateReconcileStatus(m *dbapi.SingleInstanceDatabase, ctx context.Context,
	result *ctrl.Result, err *error, blocked *bool, completed *bool) {

	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, m)

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
func (r *SingleInstanceDatabaseReconciler) validate(m *dbapi.SingleInstanceDatabase,
	n *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	eventReason := "Spec Error"
	var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	//First check image pull secrets
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
	if m.Spec.Persistence.Size == "" && m.Spec.Replicas > 1 {
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
	if m.Status.OrdsReference != "" && m.Status.Persistence.Size != "" && m.Status.Persistence != m.Spec.Persistence {
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
		err = r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				m.Status.Status = dbcommons.StatusError
				r.Status().Update(ctx, m)
				return requeueY, err
			}
			r.Log.Error(err, "Unable to get the secret. Requeueing..")
			return requeueY, err
		}
	}

	// update status fields
	m.Status.Sid = m.Spec.Sid
	m.Status.Charset = m.Spec.Charset
	m.Status.Pdbname = m.Spec.Pdbname
	m.Status.Persistence = m.Spec.Persistence
	m.Status.PrebuiltDB = m.Spec.Image.PrebuiltDB

	if m.Spec.CreateAs == "clone" {
		// Once a clone database has created , it has no link with its reference
		if m.Status.DatafilesCreated == "true" ||
			!dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
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

	if m.Spec.CreateAs == "standby" {

		// Fetch the Primary database reference, required for all iterations
		err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.PrimaryDatabaseRef}, rp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return requeueN, err
			}
			return requeueY, err
		}

		if m.Spec.Sid == rp.Spec.Sid {
			r.Log.Info("Standby database SID can not be same as the Primary database SID")
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "Spec Error", "Standby and Primary database SID can not be same")
			m.Status.Status = dbcommons.StatusError
			return requeueY, err
		}

		if rp.Status.IsTcpsEnabled {
			r.Recorder.Eventf(m, corev1.EventTypeWarning, "Cannot Create", "Standby for TCPS enabled Primary Database is not supported ")
			m.Status.Status = dbcommons.StatusError
			return requeueY, nil
		}

		if m.Status.DatafilesCreated == "true" ||
			!dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
			return requeueN, nil
		}
		m.Status.Edition = rp.Status.Edition

		err = ValidatePrimaryDatabaseForStandbyCreation(r, m, rp, ctx, req)
		if err != nil {
			return requeueY, err
		}

		r.Log.Info("Settingup Primary Database for standby creation...")
		err = SetupPrimaryDatabase(r, m, rp, ctx, req)
		if err != nil {
			return requeueY, err
		}

	}
	r.Log.Info("Completed reconcile validation")

	return requeueN, nil
}

// #############################################################################
//
//	Instantiate POD spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiatePodSpec(m *dbapi.SingleInstanceDatabase, n *dbapi.SingleInstanceDatabase, rp *dbapi.SingleInstanceDatabase,
	requiredAffinity bool) *corev1.Pod {

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
				if m.Spec.Persistence.AccessMode == "ReadWriteOnce" {
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
			Volumes: []corev1.Volume{{
				Name: "datafiles-vol",
				VolumeSource: func() corev1.VolumeSource {
					if m.Spec.Persistence.Size == "" {
						return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
					}
					/* Persistence is specified */
					return corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: m.Name,
							ReadOnly:  false,
						},
					}
				}(),
			}, {
				Name: "oracle-pwd-vol",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: m.Spec.AdminPassword.SecretName,
						Optional:   func() *bool { i := (m.Spec.Edition != "express" && m.Spec.Edition != "free"); return &i }(),
						Items: []corev1.KeyToPath{{
							Key:  m.Spec.AdminPassword.SecretKey,
							Path: "oracle_pwd",
						}},
					},
				},
			}, {
				Name: "tls-secret-vol",
				VolumeSource: func() corev1.VolumeSource {
					if m.Spec.TcpsTlsSecret == "" {
						return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
					}
					/* tls-secret is specified */
					return corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: m.Spec.TcpsTlsSecret,
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
			}},
			InitContainers: func() []corev1.Container {
				initContainers := []corev1.Container{}
				if m.Spec.Persistence.Size != "" && m.Spec.Persistence.SetWritePermissions != nil && *m.Spec.Persistence.SetWritePermissions {
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
				if (m.Spec.Edition != "express" && m.Spec.Edition != "free") && !m.Spec.Image.PrebuiltDB {
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
								Value: "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet",
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
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c", "/bin/echo -en 'shutdown immediate;\n' | env ORACLE_SID=${ORACLE_SID^^} sqlplus -S / as sysdba"},
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
					mounts := []corev1.VolumeMount{}
					if m.Spec.Persistence.Size != "" {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: "/opt/oracle/oradata",
							Name:      "datafiles-vol",
						})
					}
					if m.Spec.Edition == "express" || m.Spec.Edition == "free" || m.Spec.Image.PrebuiltDB {
						// mounts pwd as secrets for express edition or prebuilt db
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: "/run/secrets/oracle_pwd",
							ReadOnly:  true,
							Name:      "oracle-pwd-vol",
							SubPath:   "oracle_pwd",
						})
					}
					if m.Spec.TcpsTlsSecret != "" {
						mounts = append(mounts, corev1.VolumeMount{
							MountPath: dbcommons.TlsCertsLocation,
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
					return mounts
				}(),
				Env: func() []corev1.EnvVar {
					// adding XE support, useful for dev/test/CI-CD
					if m.Spec.Edition == "express" || m.Spec.Edition == "free" {
						return []corev1.EnvVar{
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
								Name:  "ORACLE_EDITION",
								Value: m.Spec.Edition,
							},
						}
					}
					if m.Spec.CreateAs == "clone" {
						// Clone DB use-case
						return []corev1.EnvVar{
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
								Value: "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet",
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
						}

					} else if m.Spec.CreateAs == "standby" {
						//Standby DB Usecase
						return []corev1.EnvVar{
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
								Value: "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet",
							},
							{
								Name: "PRIMARY_DB_CONN_STR",
								Value: func() string {
									if dbcommons.IsSourceDatabaseOnCluster(m.Spec.PrimaryDatabaseRef) {
										return rp.Name + ":" + strconv.Itoa(int(dbcommons.CONTAINER_LISTENER_PORT)) + "/" + rp.Spec.Sid
									}
									return m.Spec.PrimaryDatabaseRef
								}(),
							},
							{
								Name:  "PRIMARY_SID",
								Value: strings.ToUpper(rp.Spec.Sid),
							},
							{
								Name:  "PRIMARY_IP",
								Value: rp.Name,
							},
							{
								Name: "CREATE_PDB",
								Value: func() string {
									if rp.Spec.Pdbname != "" {
										return "true"
									}
									return "false"
								}(),
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
					}

					return []corev1.EnvVar{
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
								return "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet"
							}(),
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
									return strconv.Itoa(m.Spec.InitParams.SgaTarget)
								}
								return ""
							}(),
						},
						{
							Name:  "SKIP_DATAPATCH",
							Value: "true",
						},
					}

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

			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: func() *int64 {
					i := int64(dbcommons.ORACLE_UID)
					return &i
				}(),
				RunAsGroup: func() *int64 {
					i := int64(dbcommons.ORACLE_GUID)
					return &i
				}(),
				FSGroup: func() *int64 {
					i := int64(dbcommons.ORACLE_GUID)
					return &i
				}(),
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: m.Spec.Image.PullSecrets,
				},
			},
			ServiceAccountName: m.Spec.ServiceAccountName,
		},
	}

	// Adding pod anti-affinity for standby cases
	if m.Spec.CreateAs == "standby" {
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
		if m.Spec.Persistence.AccessMode == "ReadWriteOnce" {
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

	// Set SingleInstanceDatabase instance as the owner and controller
	ctrl.SetControllerReference(m, pod, r.Scheme)
	return pod

}

// #############################################################################
//
//	Instantiate Service spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiateSVCSpec(m *dbapi.SingleInstanceDatabase,
	svcName string, ports []corev1.ServicePort, svcType corev1.ServiceType) *corev1.Service {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
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
			Ports: []corev1.ServicePort{},
			Selector: map[string]string{
				"app": m.Name,
			},
			Type: svcType,
		},
	}
	svc.Spec.Ports = ports
	// Set SingleInstanceDatabase instance as the owner and controller
	ctrl.SetControllerReference(m, svc, r.Scheme)
	return svc
}

// #############################################################################
//
//	Instantiate Persistent Volume Claim spec from SingleInstanceDatabase spec
//
// #############################################################################
func (r *SingleInstanceDatabaseReconciler) instantiatePVCSpec(m *dbapi.SingleInstanceDatabase) *corev1.PersistentVolumeClaim {

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
					strParts := strings.Split(m.Spec.Persistence.VolumeClaimAnnotation, ":")
					annotationMap := make(map[string]string)
					annotationMap[strParts[0]] = strParts[1]
					return annotationMap
				}
				return nil
			}(),
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
			VolumeName:       m.Spec.Persistence.DatafilesVolumeName,
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
	ctrl.SetControllerReference(m, pvc, r.Scheme)
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
		ctrl.SetControllerReference(m, pvc, r.Scheme)

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

	// Don't create PVC if persistence is not chosen
	if m.Spec.Persistence.Size == "" {
		return requeueN, nil
	}

	pvcDeleted := false
	// Check if the PVC already exists using r.Get, if not create a new one using r.Create
	pvc := &corev1.PersistentVolumeClaim{}
	// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, pvc)

	if err == nil {
		if *pvc.Spec.StorageClassName != m.Spec.Persistence.StorageClass ||
			(m.Spec.Persistence.DatafilesVolumeName != "" && pvc.Spec.VolumeName != m.Spec.Persistence.DatafilesVolumeName) ||
			pvc.Spec.AccessModes[0] != corev1.PersistentVolumeAccessMode(m.Spec.Persistence.AccessMode) {
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

		} else if pvc.Spec.Resources.Requests["storage"] != resource.MustParse(m.Spec.Persistence.Size) {
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

			newPVCSize := resource.MustParse(m.Spec.Persistence.Size)
			newPVCSizeAdd := &newPVCSize
			if newPVCSizeAdd.Cmp(pvc.Spec.Resources.Requests["storage"]) < 0 {
				r.Recorder.Eventf(m, corev1.EventTypeWarning, "Cannot Resize PVC", "Forbidden: field can not be less than previous value")
				return requeueN, fmt.Errorf("Resizing PVC to lower size volume not allowed")
			}

			// Expanding the persistent volume claim
			pvc.Spec.Resources.Requests["storage"] = resource.MustParse(m.Spec.Persistence.Size)
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

	// tcpsSvcPort is the intended port for extSvc taken from singleinstancedatabase YAML file for TCPS connection
	// If loadBalancer is true, it would be the listener port otherwise it would be node port
	tcpsSvcPort := func() int32 {
		if m.Spec.TcpsListenerPort != 0 {
			return int32(m.Spec.TcpsListenerPort)
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
		svc := r.instantiateSVCSpec(m, clusterSvcName, ports, corev1.ServiceType("ClusterIP"))
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
		if m.Spec.EnableTCPS && m.Spec.ListenerPort != 0 {
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

		if (m.Spec.ListenerPort != 0 && svcPort != targetPorts[1]) || (m.Spec.EnableTCPS && m.Spec.TcpsListenerPort != 0 && tcpsSvcPort != targetPorts[len(targetPorts)-1]) {
			patchSvc = true
		}

		if m.Spec.LoadBalancer {
			if m.Spec.EnableTCPS {
				if m.Spec.TcpsListenerPort == 0 && tcpsSvcPort != targetPorts[len(targetPorts)-1] {
					patchSvc = true
				}
			} else {
				if m.Spec.ListenerPort == 0 && svcPort != targetPorts[1] {
					patchSvc = true
				}
			}
		} else {
			if m.Spec.EnableTCPS {
				if m.Spec.TcpsListenerPort == 0 && tcpsSvcPort != extSvc.Spec.Ports[len(targetPorts)-1].TargetPort.IntVal {
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
				if m.Spec.EnableTCPS {
					if m.Spec.ListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrPort, svcPort), fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					} else {
						payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					}
				} else {
					payload = fmt.Sprintf(dbcommons.TwoPortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrPort, svcPort))
				}
			} else {
				if m.Spec.EnableTCPS {
					if m.Spec.ListenerPort != 0 && m.Spec.TcpsListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrNodePort, svcPort), fmt.Sprintf(dbcommons.TcpsNodePort, tcpsSvcPort))
					} else if m.Spec.ListenerPort != 0 {
						payload = fmt.Sprintf(dbcommons.ThreePortPayload, extSvcType, fmt.Sprintf(dbcommons.LsnrNodePort, svcPort), fmt.Sprintf(dbcommons.TcpsPort, tcpsSvcPort))
					} else if m.Spec.TcpsListenerPort != 0 {
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
			if m.Spec.EnableTCPS {
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
			if m.Spec.EnableTCPS {
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
				if m.Spec.TcpsListenerPort != 0 {
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
		svc := r.instantiateSVCSpec(m, extSvcName, ports, extSvcType)
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
			oemExpressUrl = "https://" + lbAddress + ":" + fmt.Sprint(extSvc.Spec.Ports[0].Port) + "/em"
			if m.Spec.EnableTCPS {
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
			oemExpressUrl = "https://" + nodeip + ":" + fmt.Sprint(extSvc.Spec.Ports[0].NodePort) + "/em"
			if m.Spec.EnableTCPS {
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
		r.Delete(ctx, &podsMarkedToBeDeleted[i], &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
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
					r.createWallet(m, ctx, req)
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
						r.Delete(ctx, &allAvailable[i], &client.DeleteOptions{
							GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
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
		r.deletePods(ctx, req, m, oldAvailable, corev1.Pod{}, oldReplicasFound, 0)
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
					r.Delete(ctx, &newAvailable[i], &client.DeleteOptions{
						GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
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

	// If InitContainerStatuses[<index_of_init_container>].Ready is true, it means that the init container is successful
	if pod.Status.InitContainerStatuses[lastInitContIndex].Ready {
		// Init container named "init-wallet" has completed it's execution, hence return and don't requeue
		return requeueN, nil
	}

	if pod.Status.InitContainerStatuses[lastInitContIndex].State.Running == nil {
		// Init container named "init-wallet" is not running, so waiting for it to come in running state requeueing the reconcile request
		r.Log.Info("Waiting for init-wallet to come in running state...")
		return requeueY, nil
	}

	if m.Spec.CreateAs != "clone" && m.Spec.Edition != "express" {
		//Check if Edition of m.Spec.Sid is same as m.Spec.Edition
		getEditionFile := dbcommons.GetEnterpriseEditionFileCMD
		eventReason := m.Spec.Sid + " is a enterprise edition"
		if m.Spec.Edition == "enterprise" || m.Spec.Edition == "" {
			getEditionFile = dbcommons.GetStandardEditionFileCMD
			eventReason = m.Spec.Sid + " is a standard edition"
		}
		out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "init-wallet",
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
	err = r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Secret not found")
			m.Status.Status = dbcommons.StatusError
			r.Status().Update(ctx, m)
			return requeueY, nil
		}
		r.Log.Error(err, "Unable to get the secret. Requeueing..")
		return requeueY, nil
	}

	// Execing into the pods and creating the wallet
	adminPassword := string(secret.Data[m.Spec.AdminPassword.SecretKey])

	out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "init-wallet",
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
		pod := r.instantiatePodSpec(m, n, rp, replicaPatching || !firstPod)
		log.Info("Creating a new "+m.Name+" POD", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
		err := r.Create(ctx, pod)
		if err != nil {
			log.Error(err, "Failed to create new "+m.Name+" POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
			return requeueY, err
		}
		m.Status.Replicas += 1
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
		noDeleted += 1
		if err != nil {
			r.Log.Error(err, "Failed to delete existing POD", "POD.Name", availablePod.Name)
			// Don't requeue
		} else {
			m.Status.Replicas -= 1
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
	if m.Spec.AdminPassword.KeepSecret != nil && !*m.Spec.AdminPassword.KeepSecret {
		r.Log.Info("Querying the database secret ...")
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, secret)
		if err == nil {
			err := r.Delete(ctx, secret)
			if err == nil {
				r.Log.Info("Deleted the secret : " + m.Spec.AdminPassword.SecretName)
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
	readyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	eventReason := "Configuring TCPS"

	if (m.Spec.EnableTCPS) &&
		((!m.Status.IsTcpsEnabled) || // TCPS Enabled from a TCP state
			(m.Spec.TcpsTlsSecret != "" && m.Status.TcpsTlsSecret == "") || // TCPS Secret is added in spec
			(m.Spec.TcpsTlsSecret == "" && m.Status.TcpsTlsSecret != "") || // TCPS Secret is removed in spec
			(m.Spec.TcpsTlsSecret != "" && m.Status.TcpsTlsSecret != "" && m.Spec.TcpsTlsSecret != m.Status.TcpsTlsSecret)) { //TCPS secret is changed

		// Set status to Updating, except when an error has been thrown from configTCPS script
		if m.Status.Status != dbcommons.StatusError {
			m.Status.Status = dbcommons.StatusUpdating
		}
		r.Status().Update(ctx, m)

		eventMsg := "Enabling TCPS in the database..."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

		var TcpsCommand = dbcommons.EnableTcpsCMD
		if m.Spec.TcpsTlsSecret != "" { // case when tls secret is either added or changed
			TcpsCommand = "export TCPS_CERTS_LOCATION=" + dbcommons.TlsCertsLocation + " && " + dbcommons.EnableTcpsCMD

			// Checking for tls-secret mount in pods
			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
				ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.PodMountsCmd, dbcommons.TlsCertsLocation))
			r.Log.Info("Mount Check Output")
			r.Log.Info(out)
			if err != nil {
				r.Log.Error(err, err.Error())
				return requeueY, nil
			}

			if (m.Status.TcpsTlsSecret != "") || // case when TCPS Secret is changed
				(!strings.Contains(out, dbcommons.TlsCertsLocation)) { // if mount is not there in pod
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
			r.Status().Update(ctx, m)
			return requeueY, nil
		}
		r.Log.Info("enableTcps Output : \n" + out)
		// Updating the Status and publishing the event
		m.Status.CertCreationTimestamp = time.Now().Format(time.RFC3339)
		m.Status.IsTcpsEnabled = true
		m.Status.ClientWalletLoc = fmt.Sprintf(dbcommons.ClientWalletLocation, m.Spec.Sid)
		// m.Spec.TcpsTlsSecret can be empty or non-empty
		// Store secret name in case of tls-secret addition or change, otherwise would be ""
		if m.Spec.TcpsTlsSecret != "" {
			m.Status.TcpsTlsSecret = m.Spec.TcpsTlsSecret
		} else {
			m.Status.TcpsTlsSecret = ""
		}

		r.Status().Update(ctx, m)

		eventMsg = "TCPS Enabled."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

		requeueDuration, _ := time.ParseDuration(m.Spec.TcpsCertRenewInterval)
		requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
		futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}

		// update clientWallet
		err = r.updateClientWallet(m, readyPod, ctx, req)
		if err != nil {
			r.Log.Error(err, "Error in updating tnsnames.ora in clientWallet...")
			return requeueY, nil
		}
	} else if !m.Spec.EnableTCPS && m.Status.IsTcpsEnabled {
		// Disable TCPS
		m.Status.Status = dbcommons.StatusUpdating
		r.Status().Update(ctx, m)

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

		r.Status().Update(ctx, m)

		eventMsg = "TCPS Disabled."
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	} else if m.Spec.EnableTCPS && m.Status.IsTcpsEnabled && m.Spec.TcpsCertRenewInterval != "" {
		// Cert Renewal Logic
		certCreationTimestamp, _ := time.Parse(time.RFC3339, m.Status.CertCreationTimestamp)
		duration := time.Since(certCreationTimestamp)
		allowdDuration, _ := time.ParseDuration(m.Spec.TcpsCertRenewInterval)
		if duration > allowdDuration {
			m.Status.Status = dbcommons.StatusUpdating
			r.Status().Update(ctx, m)

			out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
				ctx, req, false, "bash", "-c", fmt.Sprintf(dbcommons.EnableTcpsCMD))
			if err != nil {
				r.Log.Error(err, err.Error())
				return requeueY, nil
			}
			r.Log.Info("Cert Renewal Output : \n" + out)
			// Updating the Status and publishing the event
			m.Status.CertCreationTimestamp = time.Now().Format(time.RFC3339)
			r.Status().Update(ctx, m)

			eventMsg := "TCPS Certificates Renewed at time %s,"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg, time.Now().Format(time.RFC3339))

			requeueDuration, _ := time.ParseDuration(m.Spec.TcpsCertRenewInterval)
			requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
			futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}
		}
		if m.Status.CertRenewInterval != m.Spec.TcpsCertRenewInterval {
			requeueDuration, _ := time.ParseDuration(m.Spec.TcpsCertRenewInterval)
			requeueDuration += func() time.Duration { requeueDuration, _ := time.ParseDuration("1s"); return requeueDuration }()
			futureRequeue = ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}

			m.Status.CertRenewInterval = m.Spec.TcpsCertRenewInterval
		}
		// update clientWallet
		err := r.updateClientWallet(m, readyPod, ctx, req)
		if err != nil {
			r.Log.Error(err, "Error in updating tnsnames.ora clientWallet...")
			return requeueY, nil
		}
	} else if m.Spec.EnableTCPS && m.Status.IsTcpsEnabled && m.Spec.TcpsCertRenewInterval == "" {
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
	r.Status().Update(ctx, m)

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
	r.Status().Update(ctx, m)
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

		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", dbcommons.CreateDBRecoveryDestCMD)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
		log.Info("CreateDbRecoveryDest Output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.SetDBRecoveryDestSQL, dbcommons.SQLPlusCLI))
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

	cpu_count, pga_aggregate_target, processes, sga_target, err := dbcommons.CheckDBInitParams(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		return err
	}
	sidbInitParams := dbapi.SingleInstanceDatabaseInitParams{
		SgaTarget:          sga_target,
		PgaAggregateTarget: pga_aggregate_target,
		Processes:          processes,
		CpuCount:           cpu_count,
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
		sidb.Status.OemExpressUrl = oemExpressUrl
	}

	if sidb.Status.Role == "PRIMARY" && sidb.Status.DatafilesPatched != "true" {
		eventReason := "Datapatch Pending"
		eventMsg := "datapatch execution pending"
		r.Recorder.Eventf(sidb, corev1.EventTypeNormal, eventReason, eventMsg)
	}

	// update status to Ready after all operations succeed
	sidb.Status.Status = dbcommons.StatusReady

	r.Status().Update(ctx, sidb)

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
		r.Status().Update(ctx, n)
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

	if m.Status.DgBrokerConfigured {
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

// #############################################################################
//
//	SetupWithManager sets up the controller with the Manager
//
// #############################################################################
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
func GetDatabaseAdminPassword(r client.Reader, d *dbapi.SingleInstanceDatabase, ctx context.Context) (string, error) {

	adminPasswordSecret := &corev1.Secret{}
	adminPassword := ""
	err := r.Get(ctx, types.NamespacedName{Name: d.Spec.AdminPassword.SecretName, Namespace: d.Namespace}, adminPasswordSecret)
	if err != nil {
		return adminPassword, err
	}
	adminPassword = string(adminPasswordSecret.Data[d.Spec.AdminPassword.SecretKey])
	return adminPassword, nil
}

// #############################################################################
//
//	Validate primary singleinstancedatabase admin password
//
// #############################################################################
func ValidatePrimaryDatabaseAdminPassword(r *SingleInstanceDatabaseReconciler, p *dbapi.SingleInstanceDatabase,
	adminPassword string, ctx context.Context, req ctrl.Request) error {

	dbReadyPod, err := GetDatabaseReadyPod(r, p, ctx, req)
	if err != nil {
		return err
	}

	out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.ValidateAdminPassword, adminPassword), dbcommons.GetSqlClient(p.Spec.Edition)))
	if err != nil {
		return err
	}

	if strings.Contains(out, "USER is \"SYS\"") {
		r.Log.Info("validated Admin password successfully")
	} else {
		if strings.Contains(out, "ORA-01017") {
			r.Log.Info("Invalid primary database password, Logon denied")
		}
		return fmt.Errorf("primary database admin password validation failed")
	}

	return nil
}

// #############################################################################
//
//	Validate refered primary database db params are all enabled
//
// #############################################################################
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
func ValidatePrimaryDatabaseForStandbyCreation(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
	primary *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) error {

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

	adminPassword, err := GetDatabaseAdminPassword(r, stdby, ctx)
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
func GetTotalDatabasePods(r client.Reader, d *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (int, error) {
	_, totalPods, _, _, err := dbcommons.FindPods(r, d.Spec.Image.Version,
		d.Spec.Image.PullFrom, d.Name, d.Namespace, ctx, req)

	return totalPods, err
}

// #############################################################################
//
//	Set tns names for primary database for dataguard configuraion
//
// #############################################################################
func SetupTnsNamesPrimaryForDG(r *SingleInstanceDatabaseReconciler, p *dbapi.SingleInstanceDatabase, s *dbapi.SingleInstanceDatabase,
	primaryReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {

	out, err := dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("cat /opt/oracle/oradata/dbconfig/%s/tnsnames.ora", strings.ToUpper(p.Spec.Sid)))
	if err != nil {
		return fmt.Errorf("error obtaining the contents of tnsnames.ora in the primary database %v", p.Name)
	}
	r.Log.Info("tnsnames.ora content is as follows:")
	r.Log.Info(out)

	if strings.Contains(out, "(SERVICE_NAME = "+strings.ToUpper(s.Spec.Sid)+")") {
		r.Log.Info("TNS ENTRY OF " + s.Spec.Sid + " ALREADY EXISTS ON PRIMARY Database ")
	} else {
		tnsnamesEntry := dbcommons.StandbyTnsnamesEntry
		tnsnamesEntry = strings.ReplaceAll(tnsnamesEntry, "##STANDBYDATABASE_SID##", s.Spec.Sid)
		tnsnamesEntry = strings.ReplaceAll(tnsnamesEntry, "##STANDBYDATABASE_SERVICE_EXPOSED##", s.Name)

		out, err = dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | cat >> /opt/oracle/oradata/dbconfig/%s/tnsnames.ora ", tnsnamesEntry, strings.ToUpper(p.Spec.Sid)))
		if err != nil {
			return fmt.Errorf("unable to set tnsnames.ora in the primary database %v", p.Name)
		}
		r.Log.Info("Modifying tnsnames.ora Output")
		r.Log.Info(out)

	}
	return nil
}

// #############################################################################
//
//	Restarting listners in database
//
// #############################################################################
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
	err = SetupTnsNamesPrimaryForDG(r, primary, stdby, primaryDbReadyPod, ctx, req)
	if err != nil {
		return err
	}

	log.Info("Setting up listener.ora in primary database", "primaryDatabase", primary.Name)
	err = SetupListenerPrimaryForDG(r, primary, stdby, primaryDbReadyPod, ctx, req)
	if err != nil {
		return err
	}

	log.Info("Setting up some InitParams for DG in primary database", "primaryDatabase", primary.Name)
	err = SetupInitParamsPrimaryForDG(r, primaryDbReadyPod, ctx, req)
	if err != nil {
		return err
	}

	return nil

}

// #############################################################################
//
//	Get all pdbs in a singleinstancedatabase
//
// #############################################################################
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
func SetupTnsNamesForPDBListInDatabase(r *SingleInstanceDatabaseReconciler, d *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request, pdbList []string) error {
	for _, pdb := range pdbList {
		if pdb == "" {
			continue
		}

		// Get the Tnsnames.ora entries
		out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "",
			ctx, req, false, "bash", "-c", fmt.Sprintf("cat /opt/oracle/oradata/dbconfig/%s/tnsnames.ora", strings.ToUpper(d.Spec.Sid)))
		if err != nil {
			return err
		}
		r.Log.Info("tnsnames.ora Output")
		r.Log.Info(out)

		if strings.Contains(out, "(SERVICE_NAME = "+strings.ToUpper(pdb)+")") {
			r.Log.Info("TNS ENTRY OF " + strings.ToUpper(pdb) + " ALREADY EXISTS ON SIDB ")
		} else {
			tnsnamesEntry := dbcommons.PDBTnsnamesEntry
			tnsnamesEntry = strings.ReplaceAll(tnsnamesEntry, "##PDB_NAME##", strings.ToUpper(pdb))

			// Add Tnsnames.ora For pdb on Standby Database
			out, err = dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf("echo -e  \"%s\"  | cat >> /opt/oracle/oradata/dbconfig/%s/tnsnames.ora ", tnsnamesEntry, strings.ToUpper(d.Spec.Sid)))
			if err != nil {
				return err
			}
			r.Log.Info("Modifying tnsnames.ora for Pdb Output")
			r.Log.Info(out)

		}
	}

	return nil
}

// #############################################################################
//
//	Setup tnsnames.ora in standby database for primary singleinstancedatabase
//
// #############################################################################
func SetupPrimaryDBTnsNamesInStandby(r *SingleInstanceDatabaseReconciler, s *dbapi.SingleInstanceDatabase,
	dbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) error {

	out, err := dbcommons.ExecCommand(r, r.Config, dbReadyPod.Name, dbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | cat >> /opt/oracle/oradata/dbconfig/%s/tnsnames.ora ", dbcommons.PrimaryTnsnamesEntry, strings.ToUpper(s.Spec.Sid)))
	if err != nil {
		return err
	}
	r.Log.Info("Modifying tnsnames.ora Output")
	r.Log.Info(out)

	return nil
}

// #############################################################################
//
//	Enabling flashback in singleinstancedatabase
//
// #############################################################################
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
func SetupStandbyDatabase(r *SingleInstanceDatabaseReconciler, stdby *dbapi.SingleInstanceDatabase,
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
	err = SetupListenerPrimaryForDG(r, stdby, primary, stdbyReadyPod, ctx, req)
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
