/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	"encoding/json"
	"errors"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v63/database"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/annotations"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousContainerDatabaseReconciler reconciles a AutonomousContainerDatabase object
type AutonomousContainerDatabaseReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	dbService oci.DatabaseService
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousContainerDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.AutonomousContainerDatabase{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Complete(r)
}

func (r *AutonomousContainerDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			desiredACD, acdOk := e.ObjectNew.(*dbv1alpha1.AutonomousContainerDatabase)
			if acdOk {
				oldACD := e.ObjectOld.(*dbv1alpha1.AutonomousContainerDatabase)

				if !reflect.DeepEqual(oldACD.Status, desiredACD.Status) ||
					(controllerutil.ContainsFinalizer(oldACD, dbv1alpha1.LastSuccessfulSpec) != controllerutil.ContainsFinalizer(desiredACD, dbv1alpha1.LastSuccessfulSpec)) ||
					(controllerutil.ContainsFinalizer(oldACD, dbv1alpha1.ACDFinalizer) != controllerutil.ContainsFinalizer(desiredACD, dbv1alpha1.ACDFinalizer)) {
					// Don't enqueue if the status, lastSucSpec, or the finalizler changes
					return false
				}

				return true
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the object is deleted from the cluster.
			_, acdOk := e.Object.(*dbv1alpha1.AutonomousContainerDatabase)
			return !acdOk
		},
	}

	return pred
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomouscontainerdatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomouscontainerdatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *AutonomousContainerDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespace/Name", req.NamespacedName)

	var err error
	var ociACD *dbv1alpha1.AutonomousContainerDatabase

	// Get the autonomousdatabase instance from the cluster
	acd := &dbv1alpha1.AutonomousContainerDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, acd); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ACD, so we don't need to update the status
		return emptyResult, err
	}

	/******************************************************************
	* Get OCI database client
	******************************************************************/
	if err := r.setupOCIClients(logger, acd); err != nil {
		logger.Error(err, "Fail to setup OCI clients")

		return r.manageError(logger, acd, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	* Get OCI AutonomousDatabase
	******************************************************************/

	if acd.Spec.AutonomousContainerDatabaseOCID != nil {
		resp, err := r.dbService.GetAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
		if err != nil {
			return r.manageError(logger, acd, err)
		}

		ociACD = &dbv1alpha1.AutonomousContainerDatabase{}
		ociACD.UpdateFromOCIACD(resp.AutonomousContainerDatabase)
	}

	/******************************************************************
	* Requeue if the ACD is in an intermediate state
	* No-op if the ACD OCID is nil
	* To get the latest status, execute before all the reconcile logic
	******************************************************************/
	needsRequeue, err := r.validateLifecycleState(logger, acd, ociACD)
	if err != nil {
		return r.manageError(logger, acd, err)
	}

	if needsRequeue {
		return requeueResult, nil
	}

	/******************************************************************
	* Cleanup the resource if the resource is to be deleted.
	* Deletion timestamp will be added to a object before it is deleted.
	* Kubernetes server calls the clean up function if a finalizer exitsts, and won't delete the real object until
	* all the finalizers are removed from the object metadata.
	* Refer to this page for more details of using finalizers: https://kubernetes.io/blog/2021/05/14/using-finalizers-to-control-deletion/
	******************************************************************/
	exitReconcile, err := r.validateCleanup(logger, acd)
	if err != nil {
		return r.manageError(logger, acd, err)
	}

	if exitReconcile {
		return emptyResult, nil
	}

	/******************************************************************
	* Register/unregister the finalizer
	******************************************************************/
	if err := r.validateFinalizer(acd); err != nil {
		return r.manageError(logger, acd, err)
	}

	/******************************************************************
	* Validate operations
	******************************************************************/
	exitReconcile, result, err := r.validateOperation(logger, acd, ociACD)
	if err != nil {
		return r.manageError(logger, acd, err)
	}
	if exitReconcile {
		return result, nil
	}

	/******************************************************************
	*	Update the status and requeue if it's in an intermediate state
	******************************************************************/
	if err := r.KubeClient.Status().Update(context.TODO(), acd); err != nil {
		return r.manageError(logger, acd, err)
	}

	if dbv1alpha1.IsACDIntermediateState(acd.Status.LifecycleState) {
		logger.WithName("IsIntermediateState").Info("Current lifecycleState is " + string(acd.Status.LifecycleState) + "; reconcile queued")
		return requeueResult, nil
	}

	if err := r.patchLastSuccessfulSpec(acd); err != nil {
		return r.manageError(logger, acd, err)
	}

	logger.Info("AutonomousContainerDatabase reconciles successfully")

	return emptyResult, nil
}

func (r *AutonomousContainerDatabaseReconciler) setupOCIClients(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase) error {
	var err error

	authData := oci.APIKeyAuth{
		ConfigMapName: acd.Spec.OCIConfig.ConfigMapName,
		SecretName:    acd.Spec.OCIConfig.SecretName,
		Namespace:     acd.GetNamespace(),
	}

	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		return err
	}

	r.dbService, err = oci.NewDatabaseService(logger, r.KubeClient, provider)
	if err != nil {
		return err
	}

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) manageError(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase, issue error) (ctrl.Result, error) {
	l := logger.WithName("manageError")

	// Has synced at least once
	if acd.Status.LifecycleState != "" {
		// Send event
		r.Recorder.Event(acd, corev1.EventTypeWarning, "UpdateFailed", issue.Error())

		var finalIssue = issue

		// Roll back
		specChanged, err := r.getACD(logger, acd)
		if err != nil {
			finalIssue = k8s.CombineErrors(finalIssue, err)
		}

		// We don't exit the Reconcile if the spec has changed
		// becasue it will exit anyway after the manageError is called.
		if specChanged {
			if err := r.KubeClient.Update(context.TODO(), acd); err != nil {
				finalIssue = k8s.CombineErrors(finalIssue, err)
			}
		}

		l.Error(finalIssue, "UpdateFailed")

		return emptyResult, nil
	} else {
		// Send event
		r.Recorder.Event(acd, corev1.EventTypeWarning, "CreateFailed", issue.Error())

		return emptyResult, issue
	}
}

// validateLifecycleState gets and validates the current lifecycleState
func (r *AutonomousContainerDatabaseReconciler) validateLifecycleState(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase, ociACD *dbv1alpha1.AutonomousContainerDatabase) (needsRequeue bool, err error) {
	if ociACD == nil {
		return false, nil
	}

	l := logger.WithName("validateLifecycleState")

	// Special case: Once the status changes to AVAILABLE after the provision operation, the reconcile stops.
	// The backup starts right after the provision operation and the controller is not able to track the operation in this case.
	// To prevent this issue, requeue the reconcile if the previous status is PROVISIONING and we ignore the status change
	// until it becomes BACKUP_IN_PROGRESS.
	if acd.Status.LifecycleState == database.AutonomousContainerDatabaseLifecycleStateProvisioning &&
		ociACD.Status.LifecycleState != database.AutonomousContainerDatabaseLifecycleStateBackupInProgress {
		l.Info("Provisioning the ACD and waiting for the backup to start; reconcile queued")
		return true, nil
	}

	acd.Status = ociACD.Status

	if err := r.KubeClient.Status().Update(context.TODO(), acd); err != nil {
		return false, err
	}

	if dbv1alpha1.IsACDIntermediateState(ociACD.Status.LifecycleState) {
		l.Info("LifecycleState is " + string(acd.Status.LifecycleState) + "; reconcile queued")
		return true, nil
	}

	return false, nil
}

func (r *AutonomousContainerDatabaseReconciler) validateCleanup(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase) (exitReconcile bool, err error) {
	l := logger.WithName("validateCleanup")

	isACDToBeDeleted := acd.GetDeletionTimestamp() != nil

	if !isACDToBeDeleted {
		return false, nil
	}

	if controllerutil.ContainsFinalizer(acd, dbv1alpha1.ACDFinalizer) {
		if acd.Status.LifecycleState == database.AutonomousContainerDatabaseLifecycleStateTerminating {
			l.Info("Resource is already in TERMINATING state")
			// Delete in progress, continue with the reconcile logic
			return false, nil
		}

		if acd.Status.LifecycleState == database.AutonomousContainerDatabaseLifecycleStateTerminated {
			// The acd has been deleted. Remove the finalizer and exit the reconcile.
			// Once all finalizers have been removed, the object will be deleted.
			l.Info("Resource is already in TERMINATED state; remove the finalizer")
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return false, err
			}
			return true, nil
		}

		if acd.Spec.AutonomousContainerDatabaseOCID == nil {
			l.Info("Missing AutonomousContainerDatabaseOCID to terminate Autonomous Container Database; remove the finalizer anyway", "Name", acd.Name, "Namespace", acd.Namespace)
			// Remove finalizer anyway.
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return false, err
			}
			return true, nil
		}

		if acd.Spec.Action != dbv1alpha1.AcdActionTerminate {
			// Run finalization logic for finalizer. If the finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			l.Info("Terminating Autonomous Database: " + *acd.Spec.DisplayName)
			acd.Spec.Action = dbv1alpha1.AcdActionTerminate
			if err := r.KubeClient.Update(context.TODO(), acd); err != nil {
				return false, err
			}
			// Exit the reconcile since we have updated the spec
			return true, nil
		}

		// Continue with the reconcile logic
		return false, nil
	}

	// Exit the Reconcile since the to-be-deleted resource doesn't has a finalizer
	return true, nil
}

func (r *AutonomousContainerDatabaseReconciler) validateFinalizer(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	// Delete is not schduled. Update the finalizer for this CR if hardLink is present
	if acd.Spec.HardLink != nil {
		if *acd.Spec.HardLink && !controllerutil.ContainsFinalizer(acd, dbv1alpha1.ACDFinalizer) {
			if err := k8s.AddFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return err
			}
		} else if !*acd.Spec.HardLink && controllerutil.ContainsFinalizer(acd, dbv1alpha1.ACDFinalizer) {
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) validateOperation(
	logger logr.Logger,
	acd *dbv1alpha1.AutonomousContainerDatabase,
	ociACD *dbv1alpha1.AutonomousContainerDatabase) (exitReconcile bool, result ctrl.Result, err error) {

	l := logger.WithName("validateOperation")

	lastSpec, err := acd.GetLastSuccessfulSpec()
	if err != nil {
		return false, emptyResult, err
	}

	// If lastSucSpec is nil, then it's CREATE or BIND opertaion
	if lastSpec == nil {
		if acd.Spec.AutonomousContainerDatabaseOCID == nil {
			l.Info("Create operation")

			err := r.createACD(logger, acd)
			if err != nil {
				return false, emptyResult, err
			}

			// Update the ACD OCID
			if err := r.updateCR(acd); err != nil {
				return false, emptyResult, err
			}

			l.Info("AutonomousContainerDatabaseOCID updated; exit reconcile")
			return true, emptyResult, nil
		} else {
			l.Info("Bind operation")

			_, err := r.getACD(logger, acd)
			if err != nil {
				return false, emptyResult, err
			}

			if err := r.updateCR(acd); err != nil {
				return false, emptyResult, err
			}

			l.Info("spec updated; exit reconcile")
			return false, emptyResult, nil
		}
	}

	// If it's not CREATE or BIND opertaion, then UPDATE or SYNC
	// Compare with the lastSucSpec.details. If the details are different, it means that the user updates the spec.
	lastDifACD := acd.DeepCopy()

	lastDetailsChanged, err := lastDifACD.RemoveUnchangedSpec(*lastSpec)
	if err != nil {
		return false, emptyResult, err
	}

	if lastDetailsChanged {
		l.Info("Update operation")

		// Double check if the user input spec is actually different from the spec in OCI. If so, then update the resource.

		difACD := acd.DeepCopy()

		ociDetailsChanged, err := difACD.RemoveUnchangedSpec(ociACD.Spec)
		if err != nil {
			return false, emptyResult, err
		}

		if ociDetailsChanged {
			ociReqSent, specChanged, err := r.updateACD(logger, acd, difACD)
			if err != nil {
				return false, emptyResult, err
			}

			// Requeue the k8s request if an OCI request is sent, since OCI can only process one request at a time.
			if ociReqSent {
				if specChanged {
					if err := r.KubeClient.Update(context.TODO(), acd); err != nil {
						return false, emptyResult, err
					}

					l.Info("spec updated; exit reconcile")
					return false, emptyResult, nil

				} else {
					l.Info("reconcile queued")
					return true, requeueResult, nil
				}
			}
		}

		// Stop the update and patch the lastSpec when the current ACD matches the oci ACD.
		if err := r.patchLastSuccessfulSpec(acd); err != nil {
			return false, emptyResult, err
		}

		return false, emptyResult, nil

	} else {
		l.Info("No operation specified; sync the resource")

		// The user doesn't change the spec and the controller should pull the spec from the OCI.
		specChanged, err := r.getACD(logger, acd)
		if err != nil {
			return false, emptyResult, err
		}

		if specChanged {
			l.Info("The local spec doesn't match the oci's spec; update the CR")
			if err := r.updateCR(acd); err != nil {
				return false, emptyResult, err
			}

			return true, emptyResult, nil
		}
		return false, emptyResult, nil
	}
}

func (r *AutonomousContainerDatabaseReconciler) updateCR(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	// Update the lastSucSpec
	if err := acd.UpdateLastSuccessfulSpec(); err != nil {
		return err
	}

	if err := r.KubeClient.Update(context.TODO(), acd); err != nil {
		return err
	}
	return nil
}

func (r *AutonomousContainerDatabaseReconciler) patchLastSuccessfulSpec(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	specBytes, err := json.Marshal(acd.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		dbv1alpha1.LastSuccessfulSpec: string(specBytes),
	}

	annotations.PatchAnnotations(r.KubeClient, acd, anns)

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) createACD(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase) error {
	logger.WithName("createACD").Info("Sending CreateAutonomousContainerDatabase request to OCI")

	resp, err := r.dbService.CreateAutonomousContainerDatabase(acd)
	if err != nil {
		return err
	}

	acd.UpdateFromOCIACD(resp.AutonomousContainerDatabase)

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) getACD(logger logr.Logger, acd *dbv1alpha1.AutonomousContainerDatabase) (bool, error) {
	if acd == nil {
		return false, errors.New("AutonomousContainerDatabase OCID is missing")
	}

	logger.WithName("getACD").Info("Sending GetAutonomousContainerDatabase request to OCI")

	// Get the information from OCI
	resp, err := r.dbService.GetAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
	if err != nil {
		return false, err
	}

	specChanged := acd.UpdateFromOCIACD(resp.AutonomousContainerDatabase)

	return specChanged, nil
}

// updateACD returns true if an OCI request is sent.
// The AutonomousContainerDatabase is updated with the returned object from the OCI requests.
func (r *AutonomousContainerDatabaseReconciler) updateACD(
	logger logr.Logger,
	acd *dbv1alpha1.AutonomousContainerDatabase,
	difACD *dbv1alpha1.AutonomousContainerDatabase) (ociReqSent bool, specChanged bool, err error) {

	validations := []func(logr.Logger, *dbv1alpha1.AutonomousContainerDatabase, *dbv1alpha1.AutonomousContainerDatabase) (bool, bool, error){
		r.validateGeneralFields,
		r.validateDesiredLifecycleState,
	}

	for _, op := range validations {
		ociReqSent, specChanged, err := op(logger, acd, difACD)
		if err != nil {
			return false, false, err
		}

		if ociReqSent {
			return true, specChanged, nil
		}
	}

	return false, false, nil
}

func (r *AutonomousContainerDatabaseReconciler) validateGeneralFields(
	logger logr.Logger,
	acd *dbv1alpha1.AutonomousContainerDatabase,
	difACD *dbv1alpha1.AutonomousContainerDatabase) (sent bool, requeue bool, err error) {

	if difACD.Spec.DisplayName == nil &&
		difACD.Spec.PatchModel == "" &&
		difACD.Spec.FreeformTags == nil {
		return false, false, nil
	}

	logger.WithName("validateGeneralFields").Info("Sending UpdateAutonomousDatabase request to OCI")

	resp, err := r.dbService.UpdateAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID, difACD)
	if err != nil {
		return false, false, err
	}

	acd.UpdateStatusFromOCIACD(resp.AutonomousContainerDatabase)

	return true, false, nil
}

func (r *AutonomousContainerDatabaseReconciler) validateDesiredLifecycleState(
	logger logr.Logger,
	acd *dbv1alpha1.AutonomousContainerDatabase,
	difACD *dbv1alpha1.AutonomousContainerDatabase) (sent bool, specChanged bool, err error) {

	if difACD.Spec.Action == dbv1alpha1.AcdActionBlank {
		return false, false, nil
	}

	l := logger.WithName("validateDesiredLifecycleState")

	switch difACD.Spec.Action {
	case dbv1alpha1.AcdActionRestart:
		l.Info("Sending RestartAutonomousContainerDatabase request to OCI")

		resp, err := r.dbService.RestartAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
		if err != nil {
			return false, false, err
		}

		acd.Status.LifecycleState = resp.LifecycleState
	case dbv1alpha1.AcdActionTerminate:
		l.Info("Sending TerminateAutonomousContainerDatabase request to OCI")

		_, err := r.dbService.TerminateAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
		if err != nil {
			return false, false, err
		}

		acd.Status.LifecycleState = database.AutonomousContainerDatabaseLifecycleStateTerminating
	default:
		return false, false, errors.New("unknown lifecycleState")
	}

	acd.Spec.Action = dbv1alpha1.AcdActionBlank

	return true, true, nil
}
