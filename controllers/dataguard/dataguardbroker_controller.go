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

package controllers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func dataguardBrokerEventHandler() predicate.Predicate {
	base := dbcommons.ResourceEventHandler()
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Reconcile on create so existing DataguardBroker resources and freshly created
			// runner pods are re-evaluated after controller restarts or runner recreation.
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return base.Delete(e)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPodObject, oldOk := e.ObjectOld.(*corev1.Pod)
			newPodObject, newOk := e.ObjectNew.(*corev1.Pod)
			if oldOk && newOk && oldPodObject.Status.Phase != newPodObject.Status.Phase {
				return true
			}
			oldServiceObject, oldServiceOK := e.ObjectOld.(*corev1.Service)
			newServiceObject, newServiceOK := e.ObjectNew.(*corev1.Service)
			if oldServiceOK && newServiceOK {
				if !reflect.DeepEqual(oldServiceObject.Status, newServiceObject.Status) {
					return true
				}
				if !reflect.DeepEqual(oldServiceObject.Spec.Selector, newServiceObject.Spec.Selector) {
					return true
				}
			}
			oldSIDBObject, oldSIDBOK := e.ObjectOld.(*dbapi.SingleInstanceDatabase)
			newSIDBObject, newSIDBOK := e.ObjectNew.(*dbapi.SingleInstanceDatabase)
			if oldSIDBOK && newSIDBOK && !reflect.DeepEqual(oldSIDBObject.Status, newSIDBObject.Status) {
				return true
			}
			return base.Update(e)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return base.Generic(e)
		},
	}
}

func dataguardBrokerRequestsForSIDB(r client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		sidb, ok := obj.(*dbapi.SingleInstanceDatabase)
		if !ok || sidb == nil {
			return nil
		}
		var brokers dbapi.DataguardBrokerList
		if err := r.List(ctx, &brokers, client.InNamespace(sidb.Namespace)); err != nil {
			return nil
		}
		requests := make([]reconcile.Request, 0)
		for i := range brokers.Items {
			broker := &brokers.Items[i]
			desired := resolveDataguardBrokerDesiredSpec(broker)
			for _, ref := range desired.databaseRefs() {
				if strings.EqualFold(strings.TrimSpace(ref), sidb.Name) {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: broker.Namespace,
							Name:      broker.Name,
						},
					})
					break
				}
			}
		}
		return requests
	}
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services;secrets,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *DataguardBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("controller", "DataguardBroker", "resource", req.NamespacedName)
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

	scope := newDataguardBrokerReconcileScope(req, log, &dataguardBroker)
	scope.initializeDefaults()

	var result ctrl.Result
	var reconcileErr error
	defer func() {
		if reconcileErr != nil {
			scope.markError(dataguardBrokerPhaseResolving, "ReconcileFailed", reconcileErr.Error(), reconcileErr)
		}
		if err := scope.writeStatus(ctx, r); err != nil && reconcileErr == nil {
			reconcileErr = err
		}
	}()

	// Manage dataguardbroker deletion
	if !dataguardBroker.DeletionTimestamp.IsZero() {
		result, reconcileErr = r.reconcileDataguardBrokerDeletion(ctx, scope)
		return result, reconcileErr
	}

	result, reconcileErr = r.reconcileDataguardBrokerNormal(ctx, scope)
	return result, reconcileErr
}

// #############################################################################################################################
//
//	Manage deletion and clean up of the dataguardBroker resource
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) reconcileDataguardBrokerDeletion(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {

	broker := scope.broker
	req := scope.req

	log := r.Log.WithValues("manageDataguardBrokerDeletion", req.NamespacedName)

	log.Info(fmt.Sprintf("Deleting dataguard broker %v", broker.Name))
	// Check if the DataguardBroker instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	if controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {
		if broker.Spec.Topology != nil && len(broker.Status.DatabasesInDataguardConfig) == 0 {
			controllerutil.RemoveFinalizer(broker, dataguardBrokerFinalizer)
			return ctrl.Result{Requeue: false}, r.Update(ctx, broker)
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
func (r *DataguardBrokerReconciler) reconcileDataguardBrokerNormal(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	scope.markReconciling(dataguardBrokerPhaseResolving, "ResolveDesiredState", "resolving broker desired state")
	desired := resolveDataguardBrokerDesiredSpec(scope.broker)
	scope.desired = &desired
	scope.applyDesiredStatus()
	if desired.Path == dataguardBrokerPathTopology {
		scope.setTopologyResolvedCondition(true, "TopologyResolved", "resolved topology members")
	}

	if err := r.reconcileDataguardBrokerFinalizer(ctx, scope); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if err := r.reconcileDataguardBrokerService(ctx, scope); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if scope.desired.Path == dataguardBrokerPathTopology {
		if result, err := r.reconcileDataguardBrokerTopologyRuntime(ctx, scope); err != nil || result.Requeue {
			return result, err
		}
	}
	if result, err := r.reconcileDataguardBrokerValidation(ctx, scope); err != nil || result.Requeue {
		return result, err
	}
	if result, err := r.reconcileDataguardBrokerProvision(ctx, scope); err != nil || result.Requeue {
		return result, err
	}
	if result, err := r.reconcileDataguardBrokerFSFO(ctx, scope); err != nil || result.Requeue {
		return result, err
	}
	if result, err := r.reconcileDataguardBrokerTopologyOperations(ctx, scope); err != nil || result.Requeue {
		return result, err
	}
	if result, err := r.reconcileDataguardBrokerManualSwitchover(ctx, scope); err != nil || result.Requeue {
		return result, err
	}
	ready, message, err := updateReconcileStatus(r, scope.broker, scope.desired, ctx, scope.req)
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}
	if !ready {
		scope.markWaiting(dataguardBrokerPhaseReady, "RuntimeStatusPending", message)
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}
	scope.markReady(dataguardBrokerPhaseReady, "ReconcileComplete", "reconcile completed")
	return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerTopologyRuntime(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	scope.markReconciling(dataguardBrokerPhaseRuntime, "ResolveExecutionRuntime", "resolving topology execution runtime")
	runtime, ready, message, err := resolveDataguardBrokerExecutionRuntime(ctx, r, scope.broker)
	if err != nil {
		scope.setCondition(dataguardBrokerConditionExecutionRuntime, metav1.ConditionFalse, "RuntimeResolutionFailed", err.Error())
		clearDataguardBrokerCondition(&scope.broker.Status.Conditions, dataguardBrokerConditionRunnerReady)
		return ctrl.Result{Requeue: false}, err
	}
	if !ready {
		scope.setCondition(dataguardBrokerConditionExecutionRuntime, metav1.ConditionFalse, "RuntimePending", message)
		clearDataguardBrokerCondition(&scope.broker.Status.Conditions, dataguardBrokerConditionRunnerReady)
		scope.markWaiting(dataguardBrokerPhaseRuntime, "RuntimePending", message)
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}
	scope.setCondition(dataguardBrokerConditionExecutionRuntime, metav1.ConditionTrue, "RuntimeResolved", message)

	scope.markReconciling(dataguardBrokerPhaseRunner, "EnsureRunnerPod", "ensuring topology execution runner pod")
	runnerReady, runnerMessage, err := ensureDataguardBrokerRunnerPod(ctx, r, scope.broker, runtime)
	if err != nil {
		scope.phaseLog(dataguardBrokerPhaseRunner).Error(err, "topology execution runner pod reconciliation failed")
		scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPodFailed", err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	scope.phaseLog(dataguardBrokerPhaseRunner).Info("topology execution runner pod reconciliation result",
		"runnerReady", runnerReady,
		"message", runnerMessage)
	if !runnerReady {
		scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", runnerMessage)
		scope.markWaiting(dataguardBrokerPhaseRunner, "RunnerPending", runnerMessage)
		return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
	}
	scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionTrue, "RunnerReady", runnerMessage)
	if result, err := r.reconcileDataguardBrokerTopologyAuthWallet(ctx, scope, runtime); err != nil || result.Requeue {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerTopologyAuthWallet(ctx context.Context, scope *dataguardBrokerReconcileScope, runtime *dataguardBrokerExecutionRuntime) (ctrl.Result, error) {
	if runtime == nil || !runtime.authWalletEnabled() {
		return ctrl.Result{}, nil
	}
	if scope.broker.Status.AuthWallet == nil {
		scope.broker.Status.AuthWallet = &dbapi.DataguardAuthWalletStatus{}
	}
	status := scope.broker.Status.AuthWallet
	status.WalletSecretName = dataguardBrokerAuthWalletSecretName(scope.broker)

	desiredToken := ""
	if runtime.AuthWallet != nil {
		desiredToken = strings.TrimSpace(runtime.AuthWallet.RebuildToken)
	}
	needsBootstrap := !status.Initialized || strings.TrimSpace(status.WalletSecretName) == ""
	needsRebuild := desiredToken != "" && desiredToken != strings.TrimSpace(status.ObservedRebuildToken)
	if !needsBootstrap && !needsRebuild {
		status.Phase = "Ready"
		if strings.TrimSpace(status.Message) == "" {
			status.Message = "broker auth wallet is initialized"
		}
		return ctrl.Result{}, nil
	}

	status.Phase = "Building"
	status.Message = "building broker auth wallet"
	state, err := resolveDataguardTopologyState(ctx, r, scope.broker, runtime, true)
	if err != nil {
		status.Phase = "PrecheckFailed"
		status.Message = err.Error()
		scope.markWaiting(dataguardBrokerPhaseRuntime, "AuthWalletPrecheckPending", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	password, generatedSecretName, err := r.resolveDataguardBrokerAuthWalletPassword(ctx, scope.broker, runtime)
	if err != nil {
		status.Phase = "PasswordPending"
		status.Message = err.Error()
		scope.markWaiting(dataguardBrokerPhaseRuntime, "AuthWalletPasswordPending", err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}
	if err := r.rebuildDataguardBrokerAuthWalletSecret(ctx, scope.broker, scope.req, runtime, state, password); err != nil {
		if isDataguardBrokerRunnerUnavailable(err) {
			status.Phase = "RunnerPending"
			status.Message = err.Error()
			scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
			scope.markWaiting(dataguardBrokerPhaseRunner, "RunnerPending", err.Error())
			return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		}
		status.Phase = "BuildFailed"
		status.Message = err.Error()
		return ctrl.Result{}, err
	}

	status.Initialized = true
	status.Phase = "Ready"
	status.Message = "broker auth wallet is initialized"
	status.GeneratedPasswordSecretName = generatedSecretName
	status.ObservedRebuildToken = desiredToken
	if !runtime.usesAuthWallet() {
		scope.markWaiting(dataguardBrokerPhaseRuntime, "AuthWalletCreated", "auth wallet created; refreshing runner pod to mount credentials")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func dataguardBrokerAuthWalletSecretName(broker *dbapi.DataguardBroker) string {
	if broker == nil {
		return "dataguard-auth-wallet"
	}
	return broker.Name + "-auth-wallet"
}

func dataguardBrokerGeneratedAuthWalletPasswordSecretName(broker *dbapi.DataguardBroker) string {
	if broker == nil {
		return "dataguard-auth-wallet-password"
	}
	return broker.Name + "-auth-wallet-password"
}

func (r *DataguardBrokerReconciler) resolveDataguardBrokerAuthWalletPassword(ctx context.Context, broker *dbapi.DataguardBroker, runtime *dataguardBrokerExecutionRuntime) (string, string, error) {
	if broker == nil || runtime == nil || runtime.AuthWallet == nil {
		return "", "", fmt.Errorf("auth wallet runtime is not initialized")
	}
	if ref := runtime.AuthWallet.PasswordSecretRef; ref != nil && strings.TrimSpace(ref.SecretName) != "" {
		secretKey := strings.TrimSpace(ref.SecretKey)
		if secretKey == "" {
			secretKey = "password"
		}
		password, err := r.readSecretValue(ctx, broker.Namespace, strings.TrimSpace(ref.SecretName), secretKey)
		return password, "", err
	}

	secretName := dataguardBrokerGeneratedAuthWalletPasswordSecretName(broker)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: broker.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Type == "" {
			secret.Type = corev1.SecretTypeOpaque
		}
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		if len(secret.Data["password"]) == 0 {
			generated, genErr := generateDataguardBrokerAuthWalletPassword()
			if genErr != nil {
				return genErr
			}
			secret.Data["password"] = []byte(generated)
		}
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels["database.oracle.com/managed-by"] = "dataguardbroker-controller"
		secret.Labels["database.oracle.com/auth-wallet-password"] = broker.Name
		return ctrl.SetControllerReference(broker, secret, r.Scheme)
	})
	if err != nil {
		return "", "", err
	}
	password := strings.TrimSpace(string(secret.Data["password"]))
	if password == "" {
		return "", "", fmt.Errorf("generated auth wallet password secret %s/%s is empty", broker.Namespace, secretName)
	}
	return password, secretName, nil
}

func (r *DataguardBrokerReconciler) readSecretValue(ctx context.Context, namespace, name, key string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("secret %s/%s not found", namespace, name)
		}
		return "", err
	}
	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %q", namespace, name, key)
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		return "", fmt.Errorf("secret %s/%s key %q is empty", namespace, name, key)
	}
	return trimmed, nil
}

func generateDataguardBrokerAuthWalletPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(buf) + "Aa1", nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerFinalizer(ctx context.Context, scope *dataguardBrokerReconcileScope) error {
	broker := scope.broker

	// Add finalizer for this dataguardbroker resource
	if !controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {
		r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Updating Resource", "Adding finalizers")
		scope.phaseLog(dataguardBrokerPhaseProvision).Info("Adding finalizer")
		controllerutil.AddFinalizer(broker, dataguardBrokerFinalizer)
		if err := r.Update(ctx, broker); err != nil {
			return err
		}
	}
	return nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerService(ctx context.Context, scope *dataguardBrokerReconcileScope) error {
	broker := scope.broker
	req := scope.req
	desired := scope.desired
	log := scope.phaseLog(dataguardBrokerPhaseProvision)
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
					if len(desired.ServiceAnnotations) != 0 {
						for key, value := range desired.ServiceAnnotations {
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
					if desired.LoadBalancer {
						return corev1.ServiceType("LoadBalancer")
					}
					return corev1.ServiceType("NodePort")
				}()).
				Build()

			// Set the ownership of the service object to the dataguard broker resource object
			if err := ctrl.SetControllerReference(broker, &svc, r.Scheme); err != nil {
				return err
			}

			// create the service for dataguardbroker resource
			if err = r.Create(ctx, &svc); err != nil {
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Service Creation", "service creation failed")
				log.Info("service creation failed")
				return err
			} else {
				timeout := 30
				// Waiting for Service to get created as sometimes it takes some time to create a service . 30 seconds TImeout
				err = dbcommons.WaitForStatusChange(r, svc.Name, broker.Namespace, ctx, req, time.Duration(timeout)*time.Second, "svc", "creation")
				if err != nil {
					log.Error(err, "Error in Waiting for svc status for Creation", "svc.Namespace", svc.Namespace, "SVC.Name", svc.Name)
					return err
				}
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Service Created", "%s", fmt.Sprintf("Succesfully Created New Service %v", svc.Name))
				log.Info("Succesfully Created New Service ", "Service.Name : ", svc.Name)
			}
			time.Sleep(10 * time.Second)
		} else {
			return err
		}
	}

	log.Info(" ", "Found Existing Service ", service.Name)
	return nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerValidation(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	broker := scope.broker
	req := scope.req
	desired := scope.desired
	log := scope.phaseLog(dataguardBrokerPhaseValidating)
	scope.markReconciling(dataguardBrokerPhaseValidating, "ValidateDatabases", "validating broker databases")
	if desired != nil && desired.Path == dataguardBrokerPathTopology {
		runtime, ready, message, err := resolveDataguardBrokerExecutionRuntime(ctx, r, broker)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if !ready {
			scope.markWaiting(dataguardBrokerPhaseValidating, "TopologyRuntimePending", message)
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
		if _, err := resolveDataguardTopologyState(ctx, r, broker, runtime, !runtime.usesAuthWallet()); err != nil {
			scope.markWaiting(dataguardBrokerPhaseValidating, "TopologyValidationPending", err.Error())
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}
	// validate if all the databases have only one replicas
	for _, databaseRef := range desired.databaseRefs() {
		var singleinstancedatabase dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Name: databaseRef, Namespace: broker.Namespace}, &singleinstancedatabase); err != nil {
			if apierrors.IsNotFound(err) {
				broker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", "%s", fmt.Sprintf("SingleInstanceDatabase %s not found", singleinstancedatabase.Name))
				log.Info(fmt.Sprintf("singleinstancedatabase %s not found", databaseRef))
				return ctrl.Result{Requeue: false}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}
		if desired.FastStartFailover && singleinstancedatabase.Status.Replicas > 1 {
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SIDB Not supported", "dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			log.Info("dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			broker.Status.Status = dbcommons.StatusError
			return ctrl.Result{Requeue: false}, nil
		}
	}

	// Get the current primary singleinstancedatabase resourcce
	var sidb dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      desired.currentPrimaryDatabaseRef(broker),
	}
	if err := r.Get(ctx, namespacedName, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", "%s", fmt.Sprintf("SingleInstanceDatabase %s not found", sidb.Name))
			log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ctrl.Result{Requeue: false}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}
	if sidb.Status.Role != "PRIMARY" {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation", "%s", fmt.Sprintf("singleInstanceDatabase %v not in primary role", sidb.Name))
		log.Info(fmt.Sprintf("singleinstancedatabase %s expected to be in primary role", sidb.Name))
		log.Info("updating database status to check for possible FSFO")
		if _, _, err := updateReconcileStatus(r, broker, desired, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
	}

	// validate current primary singleinstancedatabase readiness
	log.Info(fmt.Sprintf("Validating readiness for singleinstancedatabase %v", sidb.Name))
	if err := validateSidbReadiness(r, broker, &sidb, ctx, req); err != nil {
		if err == ErrCurrentPrimaryDatabaseNotReady {
			if broker.Status.Status != "" && strings.EqualFold(broker.Status.FastStartFailover, "true") {
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Possible Failover", "Primary db not in ready state after setting up DG configuration")
			}
			if _, _, err := updateReconcileStatus(r, broker, desired, ctx, req); err != nil {
				log.Info("Error updating Dgbroker status")
			}
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Waiting", "%s", err.Error())
			scope.markWaiting(dataguardBrokerPhaseValidating, "PrimaryNotReady", err.Error())
			return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	return ctrl.Result{Requeue: false}, nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerProvision(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	scope.markReconciling(dataguardBrokerPhaseProvision, "ProvisionDataguard", "reconciling broker configuration")
	scope.setCondition(dataguardBrokerConditionBrokerConfigured, metav1.ConditionFalse, "Provisioning", "broker configuration is being reconciled")
	if scope.desired != nil && scope.desired.Path == dataguardBrokerPathTopology {
		runtime, ready, message, err := resolveDataguardBrokerExecutionRuntime(ctx, r, scope.broker)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if !ready {
			scope.markWaiting(dataguardBrokerPhaseProvision, "TopologyRuntimePending", message)
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
		state, err := resolveDataguardTopologyState(ctx, r, scope.broker, runtime, !runtime.usesAuthWallet())
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if err := ensureDataguardTopologyBrokerConfiguration(ctx, r, scope.broker, scope.desired, scope.req, state); err != nil {
			if isDataguardBrokerRunnerUnavailable(err) {
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseProvision, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			if isDataguardTopologyLocalMemberNotReady(err) {
				scope.setCondition(dataguardBrokerConditionBrokerConfigured, metav1.ConditionFalse, "LocalMemberPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseProvision, "LocalMemberPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}
		scope.setCondition(dataguardBrokerConditionBrokerConfigured, metav1.ConditionTrue, "BrokerConfigured", "broker configuration is up to date")
		return ctrl.Result{}, nil
	}
	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: scope.broker.Namespace,
		Name:      scope.desired.currentPrimaryDatabaseRef(scope.broker),
	}, &sidb); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if err := setupDataguardBrokerConfiguration(r, scope.broker, scope.desired, &sidb, ctx, scope.req); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	scope.setCondition(dataguardBrokerConditionBrokerConfigured, metav1.ConditionTrue, "BrokerConfigured", "broker configuration is up to date")
	return ctrl.Result{}, nil
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerFSFO(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	if scope.desired == nil {
		return ctrl.Result{}, nil
	}
	if scope.desired.Path == dataguardBrokerPathTopology {
		runtime, ready, message, err := resolveDataguardBrokerExecutionRuntime(ctx, r, scope.broker)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if !ready {
			scope.markWaiting(dataguardBrokerPhaseFSFO, "TopologyRuntimePending", message)
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
		state, err := resolveDataguardTopologyState(ctx, r, scope.broker, runtime, !runtime.usesAuthWallet())
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if scope.desired.FastStartFailover {
			scope.markReconciling(dataguardBrokerPhaseFSFO, "ReconcileFSFO", "reconciling fast-start failover")
			for _, member := range state.Members {
				if member.Role == "SNAPSHOT_STANDBY" {
					scope.markWaiting(dataguardBrokerPhaseFSFO, "SnapshotStandby", "FSFO is not supported when a snapshot standby is present")
					return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
				}
			}
			if err := configureDataguardTopologyFSFO(ctx, r, scope.broker, scope.desired, scope.req, state); err != nil {
				if isDataguardBrokerRunnerUnavailable(err) {
					scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
					scope.markWaiting(dataguardBrokerPhaseFSFO, "RunnerPending", err.Error())
					return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
				}
				return ctrl.Result{Requeue: false}, err
			}

			if err := createDataguardTopologyObserverPod(ctx, r, scope.broker, scope.req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			scope.broker.Status.FastStartFailover = "true"
			return ctrl.Result{}, nil
		}

		// Hybrid manual FSFO mode for topology DataguardBroker.
		// When spec.fastStartFailover is false/absent, do not enable/disable FSFO.
		// If user manually enables FSFO in DGMGRL, operator starts observer pod automatically.
		// FSFO status is synced from DGMGRL.
		if err := syncDataguardBrokerTopologyFSFOStatus(ctx, r, scope.broker, scope.req, state); err != nil {
			if isDataguardBrokerRunnerUnavailable(err) {
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseFSFO, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}

		if scope.broker.Status.FastStartFailover == "true" {
			if err := createDataguardTopologyObserverPod(ctx, r, scope.broker, scope.req); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
		} else {
			if err := deleteDataguardTopologyObserverPod(ctx, r, scope.broker); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
		}

		return ctrl.Result{}, nil
	}
	if scope.desired.FastStartFailover {
		scope.markReconciling(dataguardBrokerPhaseFSFO, "ReconcileFSFO", "reconciling fast-start failover")
		for _, dbResource := range scope.broker.Status.DatabasesInDataguardConfig {
			var singleInstanceDatabase dbapi.SingleInstanceDatabase
			if err := r.Get(ctx, types.NamespacedName{Namespace: scope.req.Namespace, Name: dbResource}, &singleInstanceDatabase); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			r.Log.Info("Check the role for database", "database", singleInstanceDatabase.Name, "role", singleInstanceDatabase.Status.Role)
			if singleInstanceDatabase.Status.Role == "SNAPSHOT_STANDBY" {
				r.Recorder.Eventf(scope.broker, corev1.EventTypeWarning, "Enabling FSFO failed", "database %s is a snapshot database", singleInstanceDatabase.Name)
				r.Log.Info("Enabling FSFO failed, one of the database is a snapshot database", "snapshot database", singleInstanceDatabase.Name)
				scope.markWaiting(dataguardBrokerPhaseFSFO, "SnapshotStandby", "FSFO is not supported when a snapshot standby is present")
				return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
			}
		}
		if err := setFSFOTargets(r, scope.broker, ctx, scope.req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if err := enableFSFOForDgConfig(r, scope.broker, ctx, scope.req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if err := createObserverPods(r, scope.broker, ctx, scope.req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		scope.broker.Status.FastStartFailover = "true"
		return ctrl.Result{}, nil
	}

	if err := disableFSFOForDGConfig(r, scope.broker, ctx, scope.req); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	observerReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", scope.broker.Name, scope.broker.Namespace, ctx, scope.req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if observerReadyPod.Name != "" {
		if err := r.Delete(ctx, &observerReadyPod); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}
	r.Recorder.Eventf(scope.broker, corev1.EventTypeNormal, "Observer Deleted", "database observer pod deleted")
	scope.phaseLog(dataguardBrokerPhaseFSFO).Info("database observer deleted")
	scope.broker.Status.FastStartFailover = "false"
	return ctrl.Result{}, nil
}

func ensureDataguardBrokerOperationsStatus(broker *dbapi.DataguardBroker) *dbapi.DataguardBrokerOperationsStatus {
	if broker.Status.Operations == nil {
		broker.Status.Operations = &dbapi.DataguardBrokerOperationsStatus{}
	}
	return broker.Status.Operations
}

func dataguardBrokerOperationAlreadyObserved(status *dbapi.DataguardBrokerOperationStatus, requestID string) bool {
	if status == nil || strings.TrimSpace(status.ObservedRequestID) != strings.TrimSpace(requestID) {
		return false
	}
	phase := strings.TrimSpace(status.Phase)
	return strings.EqualFold(phase, "Succeeded") || strings.EqualFold(phase, "Failed")
}

func markDataguardBrokerOperationRunning(status **dbapi.DataguardBrokerOperationStatus, requestID, target, message string) {
	now := metav1.Now()
	*status = &dbapi.DataguardBrokerOperationStatus{
		ObservedRequestID: strings.TrimSpace(requestID),
		Target:            strings.TrimSpace(target),
		Phase:             "Running",
		Message:           message,
		StartedAt:         &now,
	}
}

func markDataguardBrokerOperationComplete(status *dbapi.DataguardBrokerOperationStatus, phase, message string) {
	if status == nil {
		return
	}
	now := metav1.Now()
	status.Phase = phase
	status.Message = message
	status.CompletedAt = &now
}

func (r *DataguardBrokerReconciler) reconcileDataguardBrokerTopologyOperations(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	if scope.desired == nil || scope.desired.Path != dataguardBrokerPathTopology || scope.broker.Spec.Operations == nil {
		return ctrl.Result{}, nil
	}
	operations := scope.broker.Spec.Operations
	status := ensureDataguardBrokerOperationsStatus(scope.broker)
	if op := operations.ProtectionMode; op != nil && strings.TrimSpace(op.RequestID) != "" && !dataguardBrokerOperationAlreadyObserved(status.ProtectionMode, op.RequestID) {
		markDataguardBrokerOperationRunning(&status.ProtectionMode, op.RequestID, op.Mode, "protection mode change in progress")
		scope.markReconciling(dataguardBrokerPhaseProvision, "ProtectionModeChange", "protection mode change in progress")
		if err := performDataguardTopologyProtectionModeChange(ctx, r, scope.broker, scope.req, op.Mode); err != nil {
			if isDataguardBrokerRunnerUnavailable(err) {
				status.ProtectionMode.Message = err.Error()
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseRunner, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			markDataguardBrokerOperationComplete(status.ProtectionMode, "Failed", err.Error())
			return ctrl.Result{}, err
		}
		markDataguardBrokerOperationComplete(status.ProtectionMode, "Succeeded", "protection mode change completed")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	if op := operations.RoleConversion; op != nil && strings.TrimSpace(op.RequestID) != "" && !dataguardBrokerOperationAlreadyObserved(status.RoleConversion, op.RequestID) {
		markDataguardBrokerOperationRunning(&status.RoleConversion, op.RequestID, op.Target, "standby role conversion in progress")
		scope.markReconciling(dataguardBrokerPhaseProvision, "RoleConversion", "standby role conversion in progress")
		err := performDataguardTopologyRoleConversion(ctx, r, scope.broker, scope.req, op.Target, op.Role)
		if err != nil {
			if errors.Is(err, errDataguardTopologyRoleConversionPending) {
				status.RoleConversion.Message = err.Error()
				scope.markWaiting(dataguardBrokerPhaseProvision, "RoleConversionPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			if isDataguardBrokerRunnerUnavailable(err) {
				status.RoleConversion.Message = err.Error()
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseRunner, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			markDataguardBrokerOperationComplete(status.RoleConversion, "Failed", err.Error())
			return ctrl.Result{}, err
		}
		markDataguardBrokerOperationComplete(status.RoleConversion, "Succeeded", "standby role conversion completed")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	if op := operations.Switchover; op != nil && strings.TrimSpace(op.RequestID) != "" && !dataguardBrokerOperationAlreadyObserved(status.Switchover, op.RequestID) {
		markDataguardBrokerOperationRunning(&status.Switchover, op.RequestID, op.Target, "switchover in progress")
		scope.markReconciling(dataguardBrokerPhaseSwitchover, "SwitchoverRequested", "switchover in progress")
		if err := performDataguardTopologyManualSwitchover(ctx, r, scope.broker, scope.desired, scope.req, op.Target); err != nil {
			if errors.Is(err, errDataguardTopologySwitchoverPending) {
				status.Switchover.Message = err.Error()
				scope.markWaiting(dataguardBrokerPhaseSwitchover, "SwitchoverPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			if isDataguardBrokerRunnerUnavailable(err) {
				status.Switchover.Message = err.Error()
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseSwitchover, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			markDataguardBrokerOperationComplete(status.Switchover, "Failed", err.Error())
			return ctrl.Result{}, err
		}
		markDataguardBrokerOperationComplete(status.Switchover, "Succeeded", "switchover completed")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	if op := operations.Failover; op != nil && strings.TrimSpace(op.RequestID) != "" && !dataguardBrokerOperationAlreadyObserved(status.Failover, op.RequestID) {
		markDataguardBrokerOperationRunning(&status.Failover, op.RequestID, op.Target, "failover in progress")
		scope.markReconciling(dataguardBrokerPhaseSwitchover, "FailoverRequested", "failover in progress")
		if err := performDataguardTopologyFailover(ctx, r, scope.broker, scope.req, op.Target, op.Force); err != nil {
			if isDataguardBrokerRunnerUnavailable(err) {
				status.Failover.Message = err.Error()
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseSwitchover, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			markDataguardBrokerOperationComplete(status.Failover, "Failed", err.Error())
			return ctrl.Result{}, err
		}
		markDataguardBrokerOperationComplete(status.Failover, "Succeeded", "failover completed")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// #############################################################################################################################
//
//	Manange manual switchover to the target database
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) reconcileDataguardBrokerManualSwitchover(ctx context.Context, scope *dataguardBrokerReconcileScope) (ctrl.Result, error) {
	if strings.TrimSpace(scope.broker.Spec.SetAsPrimaryDatabase) == "" || scope.broker.Spec.SetAsPrimaryDatabase == scope.broker.Status.PrimaryDatabase {
		return ctrl.Result{}, nil
	}
	targetSidbSid := scope.broker.Spec.SetAsPrimaryDatabase
	if _, ok := scope.broker.Status.DatabasesInDataguardConfig[targetSidbSid]; !ok {
		r.Recorder.Eventf(scope.broker, corev1.EventTypeWarning, "Cannot Switchover", "%s", fmt.Sprintf("database with SID %v not found in dataguardbroker configuration", targetSidbSid))
		scope.phaseLog(dataguardBrokerPhaseSwitchover).Info(fmt.Sprintf("cannot perform switchover, database with SID %v not found in dataguardbroker configuration", targetSidbSid))
		return ctrl.Result{Requeue: false}, nil
	}
	r.Recorder.Eventf(scope.broker, corev1.EventTypeWarning, "Manual Switchover", "%s", fmt.Sprintf("Switching over to %s database", scope.broker.Status.DatabasesInDataguardConfig[targetSidbSid]))
	scope.phaseLog(dataguardBrokerPhaseSwitchover).Info(fmt.Sprintf("switching over to %s database", scope.broker.Status.DatabasesInDataguardConfig[targetSidbSid]))
	scope.markReconciling(dataguardBrokerPhaseSwitchover, "ManualSwitchover", "manual switchover in progress")
	if scope.desired != nil && scope.desired.Path == dataguardBrokerPathTopology {
		if err := performDataguardTopologyManualSwitchover(ctx, r, scope.broker, scope.desired, scope.req, targetSidbSid); err != nil {
			if errors.Is(err, errDataguardTopologySwitchoverPending) {
				scope.markWaiting(dataguardBrokerPhaseSwitchover, "SwitchoverPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			if isDataguardBrokerRunnerUnavailable(err) {
				scope.setCondition(dataguardBrokerConditionRunnerReady, metav1.ConditionFalse, "RunnerPending", err.Error())
				scope.markWaiting(dataguardBrokerPhaseSwitchover, "RunnerPending", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}
		return ctrl.Result{}, nil
	}
	return r.performDataguardBrokerManualSwitchover(targetSidbSid, scope.broker, ctx, scope.req)
}

func (r *DataguardBrokerReconciler) performDataguardBrokerManualSwitchover(targetSidbSid string, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("SetAsPrimaryDatabase", req.NamespacedName)

	if _, ok := broker.Status.DatabasesInDataguardConfig[targetSidbSid]; !ok {
		eventReason := "Cannot Switchover"
		eventMsg := fmt.Sprintf("Database %s not a part of the dataguard configuration", targetSidbSid)
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, eventReason, "%s", eventMsg)
		return ctrl.Result{Requeue: false}, nil
	}

	// change broker status to updating to indicate manual switchover start
	broker.Status.Status = dbcommons.StatusUpdating

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
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, "%s", eventMsg)

	// Get Admin password for current primary database
	var adminPasswordSecret corev1.Secret
	if err := r.Get(context.TODO(), types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSidbSid' db to make 'targetSidbSid' db primary
	err = writeDataguardPodFileWithInput(r, sidbReadyPod, "admin.pwd", adminPassword, ctx, req)
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
		Owns(&corev1.Service{}).
		Watches(&dbapi.SingleInstanceDatabase{}, handler.EnqueueRequestsFromMapFunc(dataguardBrokerRequestsForSIDB(mgr.GetClient()))).
		WithEventFilter(dataguardBrokerEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
