/*
** Copyright (c) 2024 Oracle and/or its affiliates.
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

//nolint:staticcheck,unused // Compatibility paths intentionally keep deprecated fields and optional helpers.
package controllers

// revive:disable:context-as-argument,unused-parameter,exported,var-naming,indent-error-flow
// Dataguard broker controller keeps legacy signatures/flows for backward compatibility.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DataguardBrokerReconciler reconciles a DataguardBroker object
type DataguardBrokerReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

const dataguardBrokerFinalizer = "database.oracle.com/dataguardbrokerfinalizer"

//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *DataguardBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("reconciler", req.NamespacedName)
	log.Info("Reconcile requested")

	// Get the dataguardbroker resource if already exists
	var dataguardBroker dbapi.DataguardBroker
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &dataguardBroker); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Resource deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	// Manage dataguardbroker deletion
	if !dataguardBroker.DeletionTimestamp.IsZero() {
		return r.manageDataguardBrokerDeletion(&dataguardBroker, ctx, req)
	}

	// initialize the dataguardbroker resource status
	if dataguardBroker.Status.Status == "" {
		r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Status Initialization", "initializing status fields for the resource")
		log.Info("Initializing status fields")
		dataguardBroker.Status.Status = dbcommons.StatusCreating
		dataguardBroker.Status.ExternalConnectString = dbcommons.ValueUnavailable
		dataguardBroker.Status.ClusterConnectString = dbcommons.ValueUnavailable
		dataguardBroker.Status.FastStartFailover = "false"
		if len(dataguardBroker.Status.DatabasesInDataguardConfig) == 0 {
			dataguardBroker.Status.DatabasesInDataguardConfig = map[string]string{}
		}
		if dataguardBroker.Status.ExternalDgMapping == nil {
			dataguardBroker.Status.ExternalDgMapping = map[string]string{}
		}
	}

	// Always refresh status before a reconcile
	defer func() {
		if err := r.Status().Update(ctx, &dataguardBroker); err != nil {
			log.Error(err, "failed to update dataguardbroker status")
		}
	}()

	// Mange DataguardBroker Creation
	// IMPORTANT: manageDataguardBrokerCreation() must NOT try to Get() SIDB when isNonSingleInstanceDatabase=true
	result, err := r.manageDataguardBrokerCreation(&dataguardBroker, ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if result.Requeue {
		return result, nil
	}
	// ----------------------------
	// manual switchover for non-SIDB flow (FSFO must be false)
	// ----------------------------
	if dataguardBroker.Spec.IsNonSingleInstanceDatabase &&
		!dataguardBroker.Spec.FastStartFailover &&
		dataguardBroker.Spec.SetAsPrimaryDatabase != "" &&
		!strings.EqualFold(dataguardBroker.Spec.SetAsPrimaryDatabase, dataguardBroker.Status.PrimaryDatabase) {

		log.Info("Manual Switchover requested (non-SIDB, FSFO disabled)",
			"target", dataguardBroker.Spec.SetAsPrimaryDatabase)

		result, err := r.manageManualSwitchOverExternalDirect(&dataguardBroker, ctx, req)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if result.Requeue {
			return result, nil
		}
	}

	// manage enabling and disabling faststartfailover
	if dataguardBroker.Spec.FastStartFailover {

		if dataguardBroker.Spec.IsNonSingleInstanceDatabase {

			// Validate minimum connect strings
			if len(dataguardBroker.Spec.ExternalDatabaseConnectStrings) < 2 {
				dataguardBroker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Spec Validation",
					"externalDatabaseConnectStrings must have at least 2 entries (primary + standby) when isNonSingleInstanceDatabase=true")
				return ctrl.Result{Requeue: false}, nil
			}

			// runner pod is used to run sqlplus/dgmgrl commands
			runnerPod, _, _, _, err := dbcommons.FindPods(r, "", "", dataguardBroker.Name, dataguardBroker.Namespace, ctx, req)
			if err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// If no runner/observer pod exists yet, create it and requeue.
			// (We can't STOP OBSERVER until we have a pod to exec dgmgrl from.)
			if runnerPod.Name == "" {
				cs := dataguardBroker.Spec.ExternalDatabaseConnectStrings[0]
				if cs.Port == 0 {
					cs.Port = 1521
				}
				bootstrapPrimaryConn := fmt.Sprintf("%s:%d/%s", cs.HostName, cs.Port, cs.SvcName)

				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Creating Runner",
					"creating runner/observer pod for non-SIDB FSFO flow")
				if err := createObserverPodsExternal(r, &dataguardBroker, bootstrapPrimaryConn, ctx, req); err != nil {
					return ctrl.Result{Requeue: false}, err
				}
				return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
			}

			// Collect DB roles + fal_server + log_archive_dest_1 for mapping
			dbInfos, err := collectExternalDbInfo(r, &dataguardBroker, runnerPod, dataguardBroker.Spec.ExternalDatabaseConnectStrings, ctx, req)
			if err != nil {
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Waiting", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
			}

			// Reject snapshot standby
			for _, d := range dbInfos {
				roleU := strings.ToUpper(strings.TrimSpace(d.Role))
				if roleU == "SNAPSHOT STANDBY" || roleU == "SNAPSHOT_STANDBY" {
					r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Enabling FSFO failed",
						"database %s (%s) is a snapshot standby", d.DbUniqueName, d.ConnectString)
					return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
				}
			}

			// Split primaries/standbys
			primaries := map[string]extDbInfo{} // db_unique_name -> info
			standbys := []extDbInfo{}
			for _, d := range dbInfos {
				roleU := strings.ToUpper(strings.TrimSpace(d.Role))
				if roleU == "PRIMARY" {
					primaries[d.DbUniqueName] = d
				} else if strings.Contains(roleU, "STANDBY") {
					standbys = append(standbys, d)
				}
			}

			if len(primaries) == 0 || len(standbys) == 0 {
				dataguardBroker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Role Validation",
					"non-SIDB mode requires at least 1 PRIMARY and 1 STANDBY in externalDatabaseConnectStrings")
				return ctrl.Result{Requeue: false}, nil
			}

			// Map standby->primary using FAL_SERVER / LOG_ARCHIVE_DEST_1 / LOG_ARCHIVE_DEST_2
			mapping, unmapped := mapStandbyToPrimary(primaries, standbys)

			// TEMP fallback for simple 1-primary + 1-standby topology
			if len(mapping) == 0 && len(primaries) == 1 && len(standbys) == 1 {
				for pUniq := range primaries {
					mapping[pUniq] = standbys[0].DbUniqueName
					r.Log.Info("TEMP DEBUG: forcing mapping for simple topology",
						"primary", pUniq,
						"standby", standbys[0].DbUniqueName)
				}
				unmapped = nil
			}

			// If any standby unmapped => do nothing, report error
			if len(unmapped) > 0 {
				dataguardBroker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Mapping Failed",
					"could not map %d standby DB(s) to a primary using FAL_SERVER / LOG_ARCHIVE_DEST_1 / LOG_ARCHIVE_DEST_2", len(unmapped))
				for _, s := range unmapped {
					r.Log.Info("Unmapped standby",
						"db_unique_name", s.DbUniqueName,
						"connect", s.ConnectString,
						"fal_server", s.FalServer,
						"log_archive_dest_1", s.LogArchiveD1,
						"log_archive_dest_2", s.LogArchiveD2)
				}
				return ctrl.Result{Requeue: false}, nil
			}

			// Ensure every primary has a standby
			if len(mapping) != len(primaries) {
				dataguardBroker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Mapping Failed",
					"expected each primary to have a mapped standby. primaries=%d mapped=%d", len(primaries), len(mapping))
				return ctrl.Result{Requeue: false}, nil
			}

			// Store mapping in status (ExternalDgMapping)
			if dataguardBroker.Status.ExternalDgMapping == nil {
				dataguardBroker.Status.ExternalDgMapping = map[string]string{}
			}
			for pUniq, sUniq := range mapping {
				dataguardBroker.Status.ExternalDgMapping[pUniq] = sUniq
			}
			r.Log.Info("non-SIDB DG mapping established", "mapping", mapping)

			// pick primary connect string + sys password for dgmgrl
			primaryConn, err := pickPrimaryConnectString(dbInfos)
			if err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			sysPwd, err := getExternalSysPassword(r, &dataguardBroker, ctx)
			if err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			log.Info("DEBUG_FSFO: calling setFSFOTargetsExternal",
				"primaryConn", primaryConn,
				"mapping", dataguardBroker.Status.ExternalDgMapping)

			// external FSFO calls
			if err := setFSFOTargetsExternal(r, &dataguardBroker, runnerPod, primaryConn, sysPwd, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			log.Info("DEBUG_FSFO: calling enableFSFOForDgConfigExternal",
				"primaryConn", primaryConn)
			if err := enableFSFOForDgConfigExternal(r, &dataguardBroker, runnerPod, primaryConn, sysPwd, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// recycle observer only once after FSFO becomes enabled
			if dataguardBroker.Status.FastStartFailover != "true" {
				// mark FSFO enabled in status BEFORE deleting pod
				dataguardBroker.Status.FastStartFailover = "true"

				// IMPORTANT: avoid ORA-16814 duplicate observer
				// Stop observer (best-effort), then restart pod cleanly.
				_ = stopObserverIfExistsExternal(r, &dataguardBroker, runnerPod, primaryConn, sysPwd, ctx, req)

				// Delete the runner/observer pod so reconcile will recreate it cleanly once
				if runnerPod.Name != "" {
					_ = r.Delete(ctx, &runnerPod)
					return ctrl.Result{Requeue: true, RequeueAfter: 20 * time.Second}, nil
				}

				return ctrl.Result{Requeue: true, RequeueAfter: 20 * time.Second}, nil
			}
			dataguardBroker.Status.Status = dbcommons.StatusReady

		} else {

			// SIDB flow (existing)
			for _, DbResource := range dataguardBroker.Status.DatabasesInDataguardConfig {
				var singleInstanceDatabase dbapi.SingleInstanceDatabase
				if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: DbResource}, &singleInstanceDatabase); err != nil {
					return ctrl.Result{Requeue: false}, err
				}
				r.Log.Info("Check the role for database", "database", singleInstanceDatabase.Name, "role", singleInstanceDatabase.Status.Role)
				if singleInstanceDatabase.Status.Role == "SNAPSHOT_STANDBY" {
					r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Enabling FSFO failed",
						"database %s is a snapshot database", singleInstanceDatabase.Name)
					r.Log.Info("Enabling FSFO failed, one of the database is a snapshot database",
						"snapshot database", singleInstanceDatabase.Name)
					return ctrl.Result{Requeue: true}, nil
				}
			}

			// set faststartfailover targets for all the singleinstancedatabases in the dataguard configuration
			if err := setFSFOTargets(r, &dataguardBroker, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// enable faststartfailover in the dataguard configuration
			if err := enableFSFOForDgConfig(r, &dataguardBroker, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// create Observer Pod
			if err := createObserverPods(r, &dataguardBroker, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// set faststartfailover status to true
			dataguardBroker.Status.FastStartFailover = "true"
		}

	} else {

		// IMPORTANT: In non-SIDB flow, do NOT call SIDB-only disable/delete logic
		if dataguardBroker.Spec.IsNonSingleInstanceDatabase {

			// Find observer/runner pod (same label: app=<broker.Name>)
			runnerPod, _, _, _, err := dbcommons.FindPods(r, "", "", dataguardBroker.Name, dataguardBroker.Namespace, ctx, req)
			if err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// If pod exists, disable FSFO and then delete the observer pod
			if runnerPod.Name != "" {

				// Use first entry as primary connect string target (consistent with your bootstrap behavior)
				cs := dataguardBroker.Spec.ExternalDatabaseConnectStrings[0]
				if cs.Port == 0 {
					cs.Port = 1521
				}
				primaryConn := fmt.Sprintf("%s:%d/%s", cs.HostName, cs.Port, cs.SvcName)

				sysPwd, err := getExternalSysPassword(r, &dataguardBroker, ctx)
				if err != nil {
					return ctrl.Result{Requeue: false}, err
				}

				// Disable FSFO
				if err := disableFSFOForDGConfigExternal(r, &dataguardBroker, runnerPod, primaryConn, sysPwd, ctx, req); err != nil {
					return ctrl.Result{Requeue: false}, err
				}

				// best-effort STOP OBSERVER before deleting pod
				_ = stopObserverIfExistsExternal(r, &dataguardBroker, runnerPod, primaryConn, sysPwd, ctx, req)

				// Delete observer pod
				if err := r.Delete(ctx, &runnerPod); err != nil {
					return ctrl.Result{Requeue: false}, err
				}

				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Observer Deleted", "external observer pod deleted")
				log.Info("external observer deleted")
			}

			// set faststartfailover status to false
			dataguardBroker.Status.FastStartFailover = "false"

		} else {
			// disable faststartfailover (SIDB flow)
			if err := disableFSFOForDGConfig(r, &dataguardBroker, ctx, req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// delete Observer Pod (SIDB flow)
			observerReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", dataguardBroker.Name, dataguardBroker.Namespace, ctx, req)
			if err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			if observerReadyPod.Name != "" {
				if err := r.Delete(ctx, &observerReadyPod); err != nil {
					return ctrl.Result{Requeue: false}, err
				}
			}

			r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Observer Deleted", "database observer pod deleted")
			log.Info("database observer deleted")

			// set faststartfailover status to false
			dataguardBroker.Status.FastStartFailover = "false"
		}
	}

	// manage manual switchover (SIDB only)
	if !dataguardBroker.Spec.IsNonSingleInstanceDatabase &&
		dataguardBroker.Spec.SetAsPrimaryDatabase != "" &&
		dataguardBroker.Spec.SetAsPrimaryDatabase != dataguardBroker.Status.PrimaryDatabase {

		if _, ok := dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]; !ok {
			r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Cannot Switchover",
				fmt.Sprintf("database with SID %v not found in dataguardbroker configuration", dataguardBroker.Spec.SetAsPrimaryDatabase))
			log.Info(fmt.Sprintf("cannot perform switchover, database with SID %v not found in dataguardbroker configuration",
				dataguardBroker.Spec.SetAsPrimaryDatabase))
			return ctrl.Result{Requeue: false}, nil
		}

		r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Manual Switchover",
			fmt.Sprintf("Switching over to %s database", dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]))
		log.Info(fmt.Sprintf("switching over to %s database",
			dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]))

		result, err := r.manageManualSwitchOver(dataguardBroker.Spec.SetAsPrimaryDatabase, &dataguardBroker, ctx, req)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if result.Requeue {
			return result, nil
		}
	}

	// Update Status
	// updateReconcileStatus() is SIDB-specific in most codebases.
	if !dataguardBroker.Spec.IsNonSingleInstanceDatabase {
		if err := updateReconcileStatus(r, &dataguardBroker, ctx, req); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
	}

	dataguardBroker.Status.Status = dbcommons.StatusReady
	log.Info("Reconcile Completed")

	if dataguardBroker.Spec.FastStartFailover {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{Requeue: false}, nil
}

func (r *DataguardBrokerReconciler) manageManualSwitchOverExternal(broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("manageManualSwitchOverExternal", req.NamespacedName)

	// Validate connect strings
	if len(broker.Spec.ExternalDatabaseConnectStrings) < 2 {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation",
			"externalDatabaseConnectStrings must have at least 2 entries (primary + standby) when isNonSingleInstanceDatabase=true")
		return ctrl.Result{Requeue: false}, nil
	}

	target := strings.ToUpper(strings.TrimSpace(broker.Spec.SetAsPrimaryDatabase))
	if target == "" {
		return ctrl.Result{Requeue: false}, nil
	}

	// Ensure runner pod exists (same logic you used in FSFO external flow)
	runnerPod, _, _, _, err := dbcommons.FindPods(r, "", "", broker.Name, broker.Namespace, ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if runnerPod.Name == "" {
		cs := broker.Spec.ExternalDatabaseConnectStrings[0]
		if cs.Port == 0 {
			cs.Port = 1521
		}
		bootstrapPrimaryConn := fmt.Sprintf("%s:%d/%s", cs.HostName, cs.Port, cs.SvcName)

		r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Creating Runner",
			"creating runner/observer pod for non-SIDB switchover flow")
		if err := createObserverPodsExternal(r, broker, bootstrapPrimaryConn, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// Discover current roles from DBs themselves
	dbInfos, err := collectExternalDbInfo(r, broker, runnerPod, broker.Spec.ExternalDatabaseConnectStrings, ctx, req)
	if err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Waiting", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// Find current primary + validate target exists
	var currentPrimary extDbInfo
	foundPrimary := false
	foundTarget := false

	for _, d := range dbInfos {
		roleU := strings.ToUpper(strings.TrimSpace(d.Role))
		if roleU == "PRIMARY" && !foundPrimary {
			currentPrimary = d
			foundPrimary = true
		}
		if strings.ToUpper(strings.TrimSpace(d.DbUniqueName)) == target {
			foundTarget = true
		}
	}

	if !foundPrimary {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Role Validation",
			"could not find PRIMARY in externalDatabaseConnectStrings")
		return ctrl.Result{Requeue: false}, nil
	}
	if !foundTarget {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Target Validation",
			"target %s not found in externalDatabaseConnectStrings", target)
		return ctrl.Result{Requeue: false}, nil
	}

	// sys password from secret
	sysPwd, err := getExternalSysPassword(r, broker, ctx)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Primary connect string for DGMGRL (use the one that matches the ACTIVE primary)
	// NOTE: pickPrimaryConnectString() already does role-based selection in your codebase.
	primaryConn, err := pickPrimaryConnectString(dbInfos)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	// Ensure SRLs exist on BOTH sides (current standby + current primary as future standby)
	if err := ensureStandbyRedoLogsExternal(r, runnerPod, dbInfos, sysPwd, ctx, req); err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SRL Setup Failed", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// DGMGRL database names in config appear as lower-case in your output: pshard1 / sshard2
	targetDgmgrlName := strings.ToLower(target)
	currentPrimaryDgmgrlName := strings.ToLower(strings.TrimSpace(currentPrimary.DbUniqueName))

	// Execute switchover from runner pod (NOT from operator container)
	cmd := fmt.Sprintf(`
set -euo pipefail

SYS_PWD="$(cat ${SECRET_VOLUME}/${PASSWORD_FILE} | base64 -d)"

dgmgrl /nolog <<EOF
connect sys/"${SYS_PWD}"@%s as sysdba
show configuration;
validate database %s;
validate database %s;
switchover to %s;
show configuration;
exit
EOF
`,
		primaryConn,
		currentPrimaryDgmgrlName,
		targetDgmgrlName,
		targetDgmgrlName,
	)

	out, execErr := dbcommons.ExecCommand(r, r.Config, runnerPod.Name, runnerPod.Namespace, "", ctx, req, false, "bash", "-lc", cmd)
	log.Info("DGMGRL switchover output", "out", out)
	if execErr != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Switchover Failed", execErr.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// Update status
	broker.Status.PrimaryDatabase = target
	broker.Status.Status = dbcommons.StatusReady
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Switchover Succeeded",
		"switched primary to %s", target)

	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Manage deletion and clean up of the dataguardBroker resource
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageDataguardBrokerDeletion(broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("manageDataguardBrokerDeletion", req.NamespacedName)

	log.Info(fmt.Sprintf("Deleting dataguard broker %v", broker.Name))
	// Check if the DataguardBroker instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	if controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {

		// IMPORTANT: non-SIDB brokers must NOT run SIDB cleanup logic (cleanupDataguardBroker does Get() on SIDB)
		if broker.Spec.IsNonSingleInstanceDatabase {

			// Best-effort delete external observer/runner pod (if exists)
			runnerPod, _, _, _, err := dbcommons.FindPods(r, "", "", broker.Name, broker.Namespace, ctx, req)
			if err != nil {
				// if we cannot list/find pods, requeue by returning error (keeps finalizer)
				return ctrl.Result{Requeue: false}, err
			}
			if runnerPod.Name != "" {
				if err := r.Delete(ctx, &runnerPod); err != nil {
					// keep finalizer so controller retries deletion next reconcile
					return ctrl.Result{Requeue: false}, err
				}
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Observer Deleted", "external observer pod deleted")
				log.Info("external observer deleted", "pod", runnerPod.Name)
			}

			// Remove dataguardBrokerFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(broker, dataguardBrokerFinalizer)
			if err := r.Update(ctx, broker); err != nil {
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Updating Resource", "Error while removing resource finalizers")
				log.Info("Error while removing resource finalizers")
				return ctrl.Result{Requeue: false}, err
			}

			return ctrl.Result{Requeue: false}, nil
		}

		// Run finalization logic for dataguardBrokerFinalizer. If the
		// finalization logic fails, don't remove the finalizer so
		// that we can retry during the next reconciliation.
		if err := cleanupDataguardBroker(r, broker, req, ctx); err != nil {
			// handle the errors
			return ctrl.Result{Requeue: false}, err
		}

		// Remove dataguardBrokerFinalizer. Once all finalizers have been
		// removed, the object will be deleted.
		controllerutil.RemoveFinalizer(broker, dataguardBrokerFinalizer)
		if err := r.Update(ctx, broker); err != nil {
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Updating Resource", "Error while removing resource finalizers")
			log.Info("Error while removing resource finalizers")
			return ctrl.Result{Requeue: false}, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Manage validation of singleinstancedatabases and creation of the dataguard configuration
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageDataguardBrokerCreation(broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("manageDataguardBrokerCreation", req.NamespacedName)

	// Add finalizer for this dataguardbroker resource
	if !controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {
		r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Updating Resource", "Adding finalizers")
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(broker, dataguardBrokerFinalizer)
		if err := r.Update(ctx, broker); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}

	// Check if a service for the dataguardbroker resources exists
	var service corev1.Service
	if err := r.Get(context.TODO(), types.NamespacedName{Name: broker.Name, Namespace: broker.Namespace}, &service); err != nil {
		// check if the required service is not found then create the service
		if apierrors.IsNotFound(err) {
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Creating Service", "creating service for the resource")
			log.Info("creating service for the dataguardbroker resource")

			// instantiate the service specification
			svc := dbcommons.NewRealServiceBuilder().
				SetName(broker.Name).
				SetNamespace(broker.Namespace).
				SetLabels(map[string]string{
					"app": broker.Name,
				}).
				SetAnnotation(func() map[string]string {
					annotations := make(map[string]string)
					if len(broker.Spec.ServiceAnnotations) != 0 {
						for key, value := range broker.Spec.ServiceAnnotations {
							annotations[key] = value
						}
					}
					return annotations
				}()).
				SetPorts([]corev1.ServicePort{
					{
						Name:     "listener",
						Port:     1521,
						Protocol: corev1.ProtocolTCP,
					},
					{
						Name:     "xmldb",
						Port:     5500,
						Protocol: corev1.ProtocolTCP,
					},
				}).
				SetSelector(map[string]string{
					"app": broker.Name,
				}).
				SetType(func() corev1.ServiceType {
					if broker.Spec.LoadBalancer {
						return corev1.ServiceType("LoadBalancer")
					}
					return corev1.ServiceType("NodePort")
				}()).
				Build()

			// Set the ownership of the service object to the dataguard broker resource object
			if err := ctrl.SetControllerReference(broker, &svc, r.Scheme); err != nil {
				return ctrl.Result{Requeue: false}, err
			}

			// create the service for dataguardbroker resource
			if err = r.Create(ctx, &svc); err != nil {
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Service Creation", "service creation failed")
				log.Info("service creation failed")
				return ctrl.Result{Requeue: false}, err
			} else {
				timeout := 30
				// Waiting for Service to get created as sometimes it takes some time to create a service . 30 seconds TImeout
				err = dbcommons.WaitForStatusChange(r, svc.Name, broker.Namespace, ctx, req, time.Duration(timeout)*time.Second, "svc", "creation")
				if err != nil {
					log.Error(err, "Error in Waiting for svc status for Creation", "svc.Namespace", svc.Namespace, "SVC.Name", svc.Name)
					return ctrl.Result{Requeue: false}, err
				}
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Service Created", fmt.Sprintf("Succesfully Created New Service %v", svc.Name))
				log.Info("Succesfully Created New Service ", "Service.Name : ", svc.Name)
			}
			time.Sleep(10 * time.Second)
		} else {
			return ctrl.Result{Requeue: false}, err
		}
	}

	log.Info(" ", "Found Existing Service ", service.Name)

	// -----------------------------
	// Non-SIDB creation path
	// -----------------------------
	if broker.Spec.IsNonSingleInstanceDatabase {

		// minimal validation only
		if len(broker.Spec.ExternalDatabaseConnectStrings) < 2 {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation",
				"externalDatabaseConnectStrings must have at least 2 entries (primary + standby) when isNonSingleInstanceDatabase=true")
			return ctrl.Result{Requeue: false}, nil
		}

		// If you want the observer/runner pod to exist before FSFO, you can just let reconcile handle it.
		// Creation done for non-SIDB.
		return ctrl.Result{Requeue: false}, nil
	}

	// validate if all the databases have only one replicas
	for _, databaseRef := range broker.GetDatabasesInDataGuardConfiguration() {
		var singleinstancedatabase dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Name: databaseRef, Namespace: broker.Namespace}, &singleinstancedatabase); err != nil {
			if apierrors.IsNotFound(err) {
				broker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found",
					fmt.Sprintf("SingleInstanceDatabase %s not found", databaseRef))
				log.Info(fmt.Sprintf("singleinstancedatabase %s not found", databaseRef))
				return ctrl.Result{Requeue: false}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}
		if broker.Spec.FastStartFailover && singleinstancedatabase.Status.Replicas > 1 {
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SIDB Not supported",
				"dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			log.Info("dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			broker.Status.Status = dbcommons.StatusError
			return ctrl.Result{Requeue: false}, nil
		}
	}

	// Get the current primary singleinstancedatabase resourcce
	var sidb dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      broker.GetCurrentPrimaryDatabase(),
	}
	if err := r.Get(ctx, namespacedName, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found",
				fmt.Sprintf("SingleInstanceDatabase %s not found", namespacedName.Name))
			log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ctrl.Result{Requeue: false}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	if sidb.Status.Role != "PRIMARY" {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation",
			fmt.Sprintf("singleInstanceDatabase %v not in primary role", sidb.Name))
		log.Info(fmt.Sprintf("singleinstancedatabase %s expected to be in primary role", sidb.Name))
		log.Info("updating database status to check for possible FSFO")
		if err := updateReconcileStatus(r, broker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
	}

	// validate current primary singleinstancedatabase readiness
	log.Info(fmt.Sprintf("Validating readiness for singleinstancedatabase %v", sidb.Name))
	if err := validateSidbReadiness(r, broker, &sidb, ctx, req); err != nil {
		if errors.Is(err, ErrCurrentPrimaryDatabaseNotReady) {
			fastStartFailoverStatus, _ := strconv.ParseBool(broker.Status.FastStartFailover)
			if broker.Status.Status != "" && fastStartFailoverStatus {
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Possible Failover",
					"Primary db not in ready state after setting up DG configuration")
			}
			if err := updateReconcileStatus(r, broker, ctx, req); err != nil {
				log.Info("Error updating Dgbroker status")
			}
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Waiting", err.Error())
			return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	// setup dataguard configuration
	log.Info(fmt.Sprintf("setup Dataguard configuration for primary database %v", sidb.Name))
	if err := setupDataguardBrokerConfiguration(r, broker, &sidb, ctx, req); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Manange manual switchover to the target database
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageManualSwitchOver(targetSidbSid string, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	if broker.Spec.IsNonSingleInstanceDatabase {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Cannot Switchover",
			"manual switchover is not supported when isNonSingleInstanceDatabase=true")
		return ctrl.Result{Requeue: false}, nil
	}

	log := r.Log.WithValues("SetAsPrimaryDatabase", req.NamespacedName)

	if _, ok := broker.Status.DatabasesInDataguardConfig[targetSidbSid]; !ok {
		eventReason := "Cannot Switchover"
		eventMsg := fmt.Sprintf("Database %s not a part of the dataguard configuration", targetSidbSid)
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, eventReason, eventMsg)
		return ctrl.Result{Requeue: false}, nil
	}

	// change broker status to updating to indicate manual switchover start
	broker.Status.Status = dbcommons.StatusUpdating
	if err := r.Status().Update(ctx, broker); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(context.TODO(), types.NamespacedName{Name: broker.GetCurrentPrimaryDatabase(), Namespace: broker.Namespace}, &sidb); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Fetch the primary database ready pod to create chk file
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Fetch the target database ready pod to create chk file
	targetReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", broker.Status.DatabasesInDataguardConfig[targetSidbSid], req.Namespace,
		ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: broker.Status.DatabasesInDataguardConfig[targetSidbSid], Namespace: req.Namespace}})
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Create a chk File so that no other pods take the lock during Switchover .
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("Successfully Created chk file " + out)

	out, err = dbcommons.ExecCommand(r, r.Config, targetReadyPod.Name, targetReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("Successfully Created chk file " + out)

	eventReason := "Waiting"
	eventMsg := "Switchover In Progress"
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)

	// Get Admin password for current primary database
	var adminPasswordSecret corev1.Secret
	if err := r.Get(context.TODO(), types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSidbSid' db to make 'targetSidbSid' db primary
	_, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("DB Admin pwd file created")

	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("dgmgrl sys@%s \"SWITCHOVER TO %s\" < admin.pwd", broker.Status.PrimaryDatabase, targetSidbSid))
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("SWITCHOVER TO " + targetSidbSid + " Output")
	log.Info(out)

	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Setup the controller with the Manager
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.DataguardBroker{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of DataguardBroker Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

// execSqlPlusExternal runs sqlplus against a remote DB using sys/<pwd>@<connectString> as sysdba
// and returns non-empty output lines.
func execSqlPlusExternal(
	r *DataguardBrokerReconciler,
	runnerPod corev1.Pod,
	sysPwd string,
	connectString string, // //host:port/svc
	sql string,
	ctx context.Context,
	req ctrl.Request,
) ([]string, string, error) {

	// Use quoted heredoc to avoid shell expansion issues.
	cmd := fmt.Sprintf(`sqlplus -s "sys/%s@%s as sysdba" <<'EOF'
set heading off feedback off pages 0 echo off verify off trimspool on lines 400
%s
exit;
EOF`, sysPwd, connectString, sql)

	out, err := dbcommons.ExecCommand(r, r.Config, runnerPod.Name, runnerPod.Namespace, "", ctx, req, false, "bash", "-c", cmd)
	if err != nil {
		return nil, out, err
	}
	return nonEmptyLines(out), out, nil
}

// ensureStandbyRedoLogsExternal ensures Standby Redo Logs exist on *each* DB in the config,
// so that switchover won't fail/warn (ORA-16789) and standby SRL creation is automatic.
//
// Behavior:
// - Computes ONLINE redo groups + redo size from v$log (thread 1) per DB
// - Checks SRL count + SRL size from v$standby_log (thread 1)
// - If SRLs missing (< online+1) or size mismatch -> add SRLs
// - If DB is currently a standby, cancels MRP before adding SRLs and restarts after (avoids ORA-01156)
func ensureStandbyRedoLogsExternal(
	r *DataguardBrokerReconciler,
	runnerPod corev1.Pod,
	dbInfos []extDbInfo,
	sysPwd string,
	ctx context.Context,
	req ctrl.Request,
) error {
	log := r.Log.WithValues("ensureStandbyRedoLogsExternal", req.NamespacedName)

	if len(dbInfos) < 2 {
		return fmt.Errorf("need at least 2 databases to ensure SRLs; got %d", len(dbInfos))
	}

	for _, d := range dbInfos {
		conn := strings.TrimSpace(d.ConnectString)
		if conn == "" {
			return fmt.Errorf("empty connect string for db_unique_name=%s", d.DbUniqueName)
		}

		// 1) ONLINE groups + redo size (MB) from v$log (thread#=1)
		// returns: "<online_groups>|<mb>"
		lines, raw, err := execSqlPlusExternal(r, runnerPod, sysPwd, conn, `
select to_char(count(*))||'|'||to_char(nvl(round(max(bytes)/1024/1024),0))
from v$log
where thread# = 1;
`, ctx, req)
		if err != nil {
			return fmt.Errorf("failed to query v$log on %s (%s): %w; out=%q", d.DbUniqueName, conn, err, raw)
		}
		if len(lines) < 1 || !strings.Contains(lines[0], "|") {
			return fmt.Errorf("unexpected v$log output on %s (%s): %q", d.DbUniqueName, conn, raw)
		}
		parts := strings.Split(lines[0], "|")
		if len(parts) < 2 {
			return fmt.Errorf("unexpected v$log parsed output on %s: %q", d.DbUniqueName, lines[0])
		}
		onlineCnt, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || onlineCnt <= 0 {
			return fmt.Errorf("invalid online redo group count on %s: %q", d.DbUniqueName, parts[0])
		}
		redoMB, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || redoMB <= 0 {
			return fmt.Errorf("invalid redo log size (MB) on %s: %q", d.DbUniqueName, parts[1])
		}
		required := onlineCnt + 1

		// 2) SRL count + SRL size from v$standby_log (thread#=1)
		// returns: "<srl_count>|<mb>"
		lines, raw, err = execSqlPlusExternal(r, runnerPod, sysPwd, conn, `
select to_char(count(*))||'|'||to_char(nvl(round(max(bytes)/1024/1024),0))
from v$standby_log
where thread# = 1;
`, ctx, req)
		if err != nil {
			return fmt.Errorf("failed to query v$standby_log on %s (%s): %w; out=%q", d.DbUniqueName, conn, err, raw)
		}
		if len(lines) < 1 || !strings.Contains(lines[0], "|") {
			return fmt.Errorf("unexpected v$standby_log output on %s (%s): %q", d.DbUniqueName, conn, raw)
		}
		parts = strings.Split(lines[0], "|")
		if len(parts) < 2 {
			return fmt.Errorf("unexpected v$standby_log parsed output on %s: %q", d.DbUniqueName, lines[0])
		}
		srlCnt, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		srlMB, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

		needCreate := false
		if srlCnt < required {
			needCreate = true
		}
		// If SRLs exist but size differs, still fix by adding more SRLs of correct size.
		// (We won't drop old SRLs automatically here to stay safe.)
		if srlCnt > 0 && srlMB > 0 && srlMB != redoMB {
			needCreate = true
		}

		if !needCreate {
			log.Info("SRLs OK",
				"db_unique_name", d.DbUniqueName,
				"connect", conn,
				"online_groups", onlineCnt,
				"redo_mb", redoMB,
				"srl_count", srlCnt,
				"srl_mb", srlMB)
			continue
		}

		toAdd := required - srlCnt
		if toAdd < 0 {
			// size mismatch case: still add at least 1 correct-size SRL
			toAdd = 1
		}

		roleU := strings.ToUpper(strings.TrimSpace(d.Role))
		isStandby := strings.Contains(roleU, "STANDBY")

		log.Info("SRLs missing/insufficient; creating",
			"db_unique_name", d.DbUniqueName,
			"connect", conn,
			"role", roleU,
			"online_groups", onlineCnt,
			"redo_mb", redoMB,
			"existing_srl_count", srlCnt,
			"existing_srl_mb", srlMB,
			"required_srl", required,
			"to_add", toAdd)

		// 3) Build SRL creation SQL
		var b strings.Builder
		if isStandby {
			// Cancel apply to avoid ORA-01156
			b.WriteString("alter database recover managed standby database cancel;\n")
		}

		for i := 0; i < toAdd; i++ {
			b.WriteString(fmt.Sprintf("alter database add standby logfile thread 1 size %dM;\n", redoMB))
		}

		if isStandby {
			// Restart apply
			b.WriteString("alter database recover managed standby database using current logfile disconnect from session;\n")
		}

		// Verify
		b.WriteString("set pages 200 lines 200\n")
		b.WriteString("select count(*)||'|'||to_char(nvl(round(max(bytes)/1024/1024),0)) from v$standby_log where thread#=1;\n")

		lines, raw, err = execSqlPlusExternal(r, runnerPod, sysPwd, conn, b.String(), ctx, req)
		if err != nil {
			return fmt.Errorf("failed to create SRLs on %s (%s): %w; out=%q", d.DbUniqueName, conn, err, raw)
		}
		if len(lines) < 1 || !strings.Contains(lines[len(lines)-1], "|") {
			// not fatal but suspicious
			log.Info("SRL verify output unexpected", "db_unique_name", d.DbUniqueName, "out", raw)
		} else {
			last := strings.Split(lines[len(lines)-1], "|")
			if len(last) >= 2 {
				newCnt, _ := strconv.Atoi(strings.TrimSpace(last[0]))
				newMB, _ := strconv.Atoi(strings.TrimSpace(last[1]))
				log.Info("SRLs created/verified",
					"db_unique_name", d.DbUniqueName,
					"new_srl_count", newCnt,
					"new_srl_mb", newMB,
					"required_srl", required,
					"redo_mb", redoMB)
			}
		}
	}

	return nil
}
func (r *DataguardBrokerReconciler) manageManualSwitchOverExternalDirect(
	broker *dbapi.DataguardBroker,
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {

	log := r.Log.WithValues("manageManualSwitchOverExternalDirect", req.NamespacedName)

	// Validate connect strings
	if len(broker.Spec.ExternalDatabaseConnectStrings) < 2 {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation",
			"externalDatabaseConnectStrings must have at least 2 entries (primary + standby) when isNonSingleInstanceDatabase=true")
		return ctrl.Result{Requeue: false}, nil
	}

	target := strings.ToUpper(strings.TrimSpace(broker.Spec.SetAsPrimaryDatabase))
	if target == "" {
		return ctrl.Result{Requeue: false}, nil
	}

	// Use actual DB pod, NOT observer/runner pod
	execPodName := getPodNameFromExternalHost(broker.Spec.ExternalDatabaseConnectStrings[0].HostName)
	if execPodName == "" {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Pod Resolution Failed",
			"could not derive DB pod name from externalDatabaseConnectStrings[0].hostName")
		return ctrl.Result{Requeue: false}, nil
	}

	var execPod corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Name: execPodName, Namespace: broker.Namespace}, &execPod); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Discover current roles from DBs themselves
	dbInfos, err := collectExternalDbInfo(r, broker, execPod, broker.Spec.ExternalDatabaseConnectStrings, ctx, req)
	if err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Waiting", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	var currentPrimary extDbInfo
	foundPrimary := false
	foundTarget := false

	for _, d := range dbInfos {
		roleU := strings.ToUpper(strings.TrimSpace(d.Role))
		if roleU == "PRIMARY" && !foundPrimary {
			currentPrimary = d
			foundPrimary = true
		}
		if strings.EqualFold(strings.TrimSpace(d.DbUniqueName), target) {
			foundTarget = true
		}
	}

	if !foundPrimary {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Role Validation",
			"could not find PRIMARY in externalDatabaseConnectStrings")
		return ctrl.Result{Requeue: false}, nil
	}

	if !foundTarget {
		broker.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Target Validation",
			"target %s not found in externalDatabaseConnectStrings", target)
		return ctrl.Result{Requeue: false}, nil
	}

	// Get SYS password from secret
	sysPwd, err := getExternalSysPassword(r, broker, ctx)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Choose current PRIMARY connect string
	primaryConn, err := pickPrimaryConnectString(dbInfos)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// DGMGRL names in config are usually lowercase
	targetDgmgrlName := strings.ToLower(target)
	currentPrimaryDgmgrlName := strings.ToLower(strings.TrimSpace(currentPrimary.DbUniqueName))

	cmd := fmt.Sprintf(`
set -euo pipefail

dgmgrl /nolog <<EOF
connect sys/"%s"@%s as sysdba
show configuration;
show database %s;
show database %s;
validate database %s;
validate database %s;
switchover to %s;
show configuration;
show database %s;
show database %s;
exit
EOF
`,
		sysPwd,
		primaryConn,
		currentPrimaryDgmgrlName,
		targetDgmgrlName,
		currentPrimaryDgmgrlName,
		targetDgmgrlName,
		targetDgmgrlName,
		currentPrimaryDgmgrlName,
		targetDgmgrlName,
	)

	out, execErr := dbcommons.ExecCommand(r, r.Config, execPod.Name, execPod.Namespace, "", ctx, req, false, "bash", "-lc", cmd)
	log.Info("DGMGRL switchover output", "out", out)
	if execErr != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Switchover Failed", execErr.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// Refresh roles after switchover
	dbInfos, err = collectExternalDbInfo(r, broker, execPod, broker.Spec.ExternalDatabaseConnectStrings, ctx, req)
	if err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Post Switchover Validation Failed", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	for _, d := range dbInfos {
		if strings.EqualFold(strings.TrimSpace(d.Role), "PRIMARY") {
			broker.Status.PrimaryDatabase = strings.ToUpper(strings.TrimSpace(d.DbUniqueName))
			break
		}
	}

	broker.Status.Status = dbcommons.StatusReady
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Switchover Succeeded",
		"switched primary to %s", broker.Status.PrimaryDatabase)

	return ctrl.Result{Requeue: false}, nil
}

func getPodNameFromExternalHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	return strings.Split(host, ".")[0]
}
