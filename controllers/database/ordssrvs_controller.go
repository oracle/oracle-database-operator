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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	//	dbapi "example.com/oracle-ords-operator/api/v1"
	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	"github.com/oracle/oracle-database-operator/commons/k8s"
)

// Definitions of Standards
const (
	ordsSABase            = "/opt/oracle/sa"
	serviceHTTPPortName   = "svc-http-port"
	serviceHTTPSPortName  = "svc-https-port"
	serviceMongoPortName  = "svc-mongo-port"
	targetHTTPPortName    = "pod-http-port"
	targetHTTPSPortName   = "pod-https-port"
	targetMongoPortName   = "pod-mongo-port"
	controllerLabelKey    = "oracle.com/ords-operator-filter"
	controllerLabelVal    = "oracle-database-operator"
	specHashLabel         = "oracle.com/ords-operator-spec-hash"
	APEXInstallationPV    = "apex-installation-pv"
	APEXInstallationPVC   = "apex-installation-pvc"
	APEXInstallationMount = "/opt/oracle/apex"
)

// Definitions to manage status conditions
const (
	// typeAvailableORDS represents the status of the Workload reconciliation
	typeAvailableORDS = "Available"
	// typeUnsyncedORDS represents the status used when the configuration has changed but the Workload has not been restarted.
	typeUnsyncedORDS = "Unsynced"
)

// OrdsSrvsReconciler reconciles a OrdsSrvs object
type OrdsSrvsReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Log       logr.Logger
}

// OrdsSrvsReconcileState stores intermediate reconcile state for OrdsSrvs.
type OrdsSrvsReconcileState struct {
	ordssrvsScriptsConfigMapName        string
	ordssrvsGlobalSettingsConfigMapName string
	APEXInstallationExternal            string
	passwordEncryption                  bool
	specChanged                         bool

	// Loggers
	logger    logr.Logger
	specInfo  logr.Logger
	specDebug logr.Logger

	// Trigger a restart of Pods on Config Changes
	RestartPods bool
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=ordssrvs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=ordssrvs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=ordssrvs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaim,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaim/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=deployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=daemonsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=statefulsets/status,verbs=get;update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *OrdsSrvsReconciler) SetupWithManager(mgr ctrl.Manager) error {

	r.APIReader = mgr.GetAPIReader()

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.OrdsSrvs{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

// Reconcile reconciles an OrdsSrvs resource.
func (r *OrdsSrvsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("Reconcile")
	ordssrvs := &dbapi.OrdsSrvs{}

	rState := &OrdsSrvsReconcileState{}
	rState.RestartPods = false

	// Check if resource exists or was deleted
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Resource deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error retrieving resource")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute}, err
	}

	rState.specChanged = ordssrvs.Status.ObservedGeneration != ordssrvs.Generation

	// build a logger that is either real or no-op
	rState.logger = logger
	rState.specInfo = logr.New(nil)  // no-op
	rState.specDebug = logr.New(nil) // no-op
	if rState.specChanged {
		rState.specInfo = logger
		rState.specDebug = logger.V(1)

		rState.specInfo.Info("Spec changed",
			"generation", ordssrvs.Generation,
			"resourceVersion", ordssrvs.ResourceVersion,
			"name", ordssrvs.Name,
			"namespace", ordssrvs.Namespace,
		)
	}

	rState.specInfo.Info("Reconciling", "uid", ordssrvs.UID, "gen", ordssrvs.Generation, "rv", ordssrvs.ResourceVersion)

	// Set the status as Unknown when no status are available
	if len(ordssrvs.Status.Conditions) == 0 {
		condition := metav1.Condition{Type: typeUnsyncedORDS,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting reconciliation"}
		if err := r.UpdateStatus(ctx, req, rState, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	// empty encryption key
	if ordssrvs.Spec.EncPrivKey == (dbapi.PasswordSecret{}) {
		rState.passwordEncryption = false
	} else {
		rState.passwordEncryption = true
		rState.specDebug.Info("Password Encryption key (EncPrivKey)", "secret", ordssrvs.Spec.EncPrivKey.SecretName, "key", ordssrvs.Spec.EncPrivKey.PasswordKey)
	}

	// Central Configuration
	if ordssrvs.Spec.GlobalSettings.CentralConfigURL != "" {
		rState.specDebug.Info("Using Central Configuration from URL", "URL", ordssrvs.Spec.GlobalSettings.CentralConfigURL)
	}

	// empty Global Settings
	if ordssrvs.Spec.GlobalSettings == (dbapi.GlobalSettings{}) {
		rState.specDebug.Info("Global Settings is empty, using default values")
	}

	// APEXInstallationExternal
	// true: persistence volume
	// false: no persistence volume
	if ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName != "" || ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.StorageClass != "" {
		rState.APEXInstallationExternal = "true"
	} else {
		rState.APEXInstallationExternal = "false"
	}
	rState.specDebug.Info("Setting env external_apex to " + rState.APEXInstallationExternal)

	rState.ordssrvsScriptsConfigMapName = ordssrvs.Name + "-scripts-config-map"
	rState.ordssrvsGlobalSettingsConfigMapName = ordssrvs.Name + "-global-settings-config-map"

	// ConfigMap - Scripts
	if err := r.ConfigMapReconcile(ctx, ordssrvs, rState, rState.ordssrvsScriptsConfigMapName, 0); err != nil {
		logger.Error(err, "Error in ConfigMapReconcile (init-script)")
		return ctrl.Result{}, err
	}

	if rState.APEXInstallationExternal == "true" {
		// ApexInstallation PVC
		if err := r.ApexInstallationPVCReconcile(ctx, ordssrvs, rState); err != nil {
			logger.Error(err, "Error in ApexInstallation PVC reconcile")
			return ctrl.Result{}, err
		}
	} else {
		rState.specDebug.Info("no external APEX installation files")
	}

	// ConfigMap - Global Settings
	if err := r.ConfigMapReconcile(ctx, ordssrvs, rState, rState.ordssrvsGlobalSettingsConfigMapName, 0); err != nil {
		logger.Error(err, "Error in ConfigMapReconcile (Global)")
		return ctrl.Result{}, err
	}

	// ConfigMap - Pool Settings
	definedPools := make(map[string]bool)
	definedPoolKeys := map[string]string{}
	for i := 0; i < len(ordssrvs.Spec.PoolSettings); i++ {
		poolName := ordssrvs.Spec.PoolSettings[i].PoolName
		poolKey := poolNameToK8sKey(poolName)
		if prev, exists := definedPoolKeys[poolKey]; exists {
			return ctrl.Result{}, fmt.Errorf("poolName %q and %q sanitize to the same key %q", prev, poolName, poolKey)
		}
		definedPoolKeys[poolKey] = poolName
		poolConfigMapName := ordssrvs.Name + "-cfg-pool-" + poolKey
		definedPools[poolConfigMapName] = true
		if err := r.ConfigMapReconcile(ctx, ordssrvs, rState, poolConfigMapName, i); err != nil {
			logger.Error(err, "Error in ConfigMapReconcile (Pools)")
			return ctrl.Result{}, err
		}
	}
	if err := r.ConfigMapDelete(ctx, req, ordssrvs, rState, definedPools); err != nil {
		logger.Error(err, "Error in ConfigMapDelete (Pools)")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		logger.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	// Set the Type as Unsynced when a pod restart is required
	if rState.RestartPods {
		condition := metav1.Condition{Type: typeUnsyncedORDS,
			Status:  metav1.ConditionTrue,
			Reason:  "Unsynced",
			Message: "Configurations have changed"}
		if err := r.UpdateStatus(ctx, req, rState, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Workloads
	if err := r.WorkloadReconcile(ctx, req, ordssrvs, rState, ordssrvs.Spec.WorkloadType); err != nil {
		logger.Error(err, "Error in WorkloadReconcile")
		return ctrl.Result{}, err
	}
	if err := r.WorkloadDelete(ctx, req, ordssrvs, ordssrvs.Spec.WorkloadType); err != nil {
		logger.Error(err, "Error in WorkloadDelete")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		logger.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	// Service
	if err := r.ServiceReconcile(ctx, ordssrvs, rState); err != nil {
		logger.Error(err, "Error in ServiceReconcile")
		return ctrl.Result{}, err
	}

	// Set the Type as Available when a pod restart is not required
	if !rState.RestartPods {
		condition := metav1.Condition{Type: typeAvailableORDS,
			Status:  metav1.ConditionTrue,
			Reason:  "Available",
			Message: "Workload in Sync"}
		if err := r.UpdateStatus(ctx, req, rState, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		logger.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ApexInstallationPVCReconcile reconciles the PVC used for APEX installation.
func (r *OrdsSrvsReconciler) ApexInstallationPVCReconcile(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) (err error) {

	logger := log.FromContext(ctx).WithName("ApexInstallationPVCReconcile")

	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: APEXInstallationPVC, Namespace: ordssrvs.Namespace}, pvc)
	if err == nil {
		rState.specDebug.Info("Found APEX Installation PVC : " + APEXInstallationPVC)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	volumeName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName
	pvc, err = r.APEXInstallationPVCDefine(ordssrvs, rState)
	if err != nil {
		logger.Error(err, "Invalid APEX installation PVC definition")
		return err
	}

	message := fmt.Sprintf("APEX Installation PVC : %s for PV : %s", APEXInstallationPVC, volumeName)
	rState.specDebug.Info("Creating " + message)
	err = r.Create(ctx, pvc)
	if err != nil {
		logger.Error(err, "Failed to create "+message)
	}

	return err
}

// ConfigMapReconcile reconciles the ConfigMap for OrdsSrvs.
func (r *OrdsSrvsReconciler) ConfigMapReconcile(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, configMapName string, poolIndex int) (err error) {

	desiredConfigMap, err := r.ConfigMapDefine(ctx, ordssrvs, rState, configMapName, poolIndex)
	if err != nil {
		return err
	}

	// Create if ConfigMap not found
	definedConfigMap := &corev1.ConfigMap{}
	if err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ordssrvs.Namespace}, definedConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredConfigMap); err != nil {
				return err
			}
			rState.specDebug.Info("Created: " + configMapName)
			rState.RestartPods = true
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "ConfigMap %s Created", configMapName)
			return nil
		}
		return err
	}
	if !equality.Semantic.DeepEqual(definedConfigMap.Data, desiredConfigMap.Data) {
		if err = r.Update(ctx, desiredConfigMap); err != nil {
			return err
		}
		rState.specDebug.Info("Updated: " + configMapName)
		rState.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "ConfigMap %s Updated", configMapName)
	}
	return nil
}

// WorkloadReconcile reconciles the workload resource for OrdsSrvs.
func (r *OrdsSrvsReconciler) WorkloadReconcile(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, kind string) (err error) {
	logger := log.FromContext(ctx).WithName("WorkloadReconcile")

	labels := getSystemCommonWorkloadLabels(ordssrvs, rState)
	annotations := getSystemCommonWorkloadAnnotations(ordssrvs, rState)

	objectMeta := objectMetaDefine(ordssrvs, ordssrvs.Name, labels, annotations)
	selector := selectorDefine(ordssrvs)
	template, err := r.podTemplateSpecDefine(ctx, ordssrvs, rState, req)
	if err != nil {
		return err
	}

	var desiredWorkload client.Object
	var desiredSpecHash string
	var definedSpecHash string

	var ProgressDeadlineSeconds int32 = 3600

	switch kind {
	case "StatefulSet":
		desiredWorkload = &appsv1.StatefulSet{
			ObjectMeta: objectMeta,
			Spec: appsv1.StatefulSetSpec{
				Replicas: &ordssrvs.Spec.Replicas,
				Selector: &selector,
				Template: template,
			},
		}
		desiredSpecHash = generateSpecHash(desiredWorkload.(*appsv1.StatefulSet).Spec)
		desiredWorkload.(*appsv1.StatefulSet).Labels[specHashLabel] = desiredSpecHash
	case "DaemonSet":
		desiredWorkload = &appsv1.DaemonSet{
			ObjectMeta: objectMeta,
			Spec: appsv1.DaemonSetSpec{
				Selector: &selector,
				Template: template,
			},
		}
		desiredSpecHash = generateSpecHash(desiredWorkload.(*appsv1.DaemonSet).Spec)
		desiredWorkload.(*appsv1.DaemonSet).Labels[specHashLabel] = desiredSpecHash
	default:
		desiredWorkload = &appsv1.Deployment{
			ObjectMeta: objectMeta,
			Spec: appsv1.DeploymentSpec{
				Replicas:                &ordssrvs.Spec.Replicas,
				Selector:                &selector,
				Template:                template,
				ProgressDeadlineSeconds: &ProgressDeadlineSeconds,
			},
		}
		desiredSpecHash = generateSpecHash(desiredWorkload.(*appsv1.Deployment).Spec)
		desiredWorkload.(*appsv1.Deployment).Labels[specHashLabel] = desiredSpecHash
	}

	if err := ctrl.SetControllerReference(ordssrvs, desiredWorkload, r.Scheme); err != nil {
		return err
	}

	definedWorkload := reflect.New(reflect.TypeOf(desiredWorkload).Elem()).Interface().(client.Object)
	if err = r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, definedWorkload); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredWorkload); err != nil {
				condition := metav1.Condition{
					Type:    typeAvailableORDS,
					Status:  metav1.ConditionFalse,
					Reason:  "Reconciling",
					Message: fmt.Sprintf("Failed to create %s for the custom resource (%s): (%s)", kind, ordssrvs.Name, err),
				}
				if statusErr := r.UpdateStatus(ctx, req, rState, condition); statusErr != nil {
					return statusErr
				}
				return err
			}
			logger.Info("Created: OrdsSrvs " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "Created %s", kind)
			return nil
		}
		return err
	}

	definedLabelsField := reflect.ValueOf(definedWorkload).Elem().FieldByName("ObjectMeta").FieldByName("Labels")
	if !definedLabelsField.IsValid() {
		return fmt.Errorf("%s %s/%s missing ObjectMeta.Labels field", kind, ordssrvs.Namespace, ordssrvs.Name)
	}

	specHashValue := definedLabelsField.MapIndex(reflect.ValueOf(specHashLabel))
	if !specHashValue.IsValid() {
		return fmt.Errorf("%s %s/%s missing label %q", kind, ordssrvs.Namespace, ordssrvs.Name, specHashLabel)
	}

	definedSpecHash, _ = specHashValue.Interface().(string)
	if definedSpecHash == "" {
		return fmt.Errorf("%s %s/%s has empty label %q", kind, ordssrvs.Namespace, ordssrvs.Name, specHashLabel)
	}

	if desiredSpecHash != definedSpecHash {
		logger.Info("Syncing Workload " + kind + " with new configuration")
		if err := r.Update(ctx, desiredWorkload); err != nil {
			return err
		}
		rState.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "Updated %s", kind)
	}

	if rState.RestartPods && ordssrvs.Spec.ForceRestart {
		logger.Info("Cycling: " + kind)
		labelsField := reflect.ValueOf(desiredWorkload).Elem().FieldByName("Spec").FieldByName("Template").FieldByName("ObjectMeta").FieldByName("Labels")
		if labelsField.IsValid() {
			labels := labelsField.Interface().(map[string]string)
			labels["configMapChanged"] = time.Now().Format("20060102T150405Z")
			labelsField.Set(reflect.ValueOf(labels))
			if err := r.Update(ctx, desiredWorkload); err != nil {
				return err
			}
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Restart", "Restarted %s", kind)
			rState.RestartPods = false
		}
	}

	return nil
}

// ServiceReconcile reconciles the Service for OrdsSrvs.
func (r *OrdsSrvsReconciler) ServiceReconcile(ctx context.Context, ords *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) (err error) {
	logr := log.FromContext(ctx).WithName("ServiceReconcile")

	// Defensive fallback to CRD defaults in case pointers are nil
	const (
		defaultHTTPPort  int32 = 8080
		defaultHTTPSPort int32 = 8443
		defaultMongoPort int32 = 27017
	)

	HTTPport := defaultHTTPPort
	if ords.Spec.GlobalSettings.StandaloneHTTPPort != nil {
		HTTPport = *ords.Spec.GlobalSettings.StandaloneHTTPPort
	}

	HTTPSport := defaultHTTPSPort
	if ords.Spec.GlobalSettings.StandaloneHTTPSPort != nil {
		HTTPSport = *ords.Spec.GlobalSettings.StandaloneHTTPSPort
	}

	MongoPort := defaultMongoPort
	if ords.Spec.GlobalSettings.MongoPort != nil {
		MongoPort = *ords.Spec.GlobalSettings.MongoPort
	}

	desiredService, err := r.ServiceDefine(ords, rState, HTTPport, HTTPSport, MongoPort)
	if err != nil {
		return err
	}

	key := types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}
	definedService := &corev1.Service{}
	if err := r.Get(ctx, key, definedService); err != nil {

		if !apierrors.IsNotFound(err) {
			return err
		}

		// get failed IsNotFound -> create
		createErr := r.Create(ctx, desiredService)
		if createErr == nil {
			logr.Info("Created: Service")
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Create", "Service %s Created", ords.Name)
			return nil
		}

		if !apierrors.IsAlreadyExists(createErr) {
			return createErr
		}

		// get error IsNotFound, try to create, IsAlreadyExists. cache problem.
		// reread without cache
		if err := r.APIReader.Get(ctx, key, definedService); err != nil {
			return err
		}

	}

	desiredPortCount := len(desiredService.Spec.Ports)
	definedPortCount := len(definedService.Spec.Ports)
	needsUpdate := desiredPortCount != definedPortCount

	if !needsUpdate {
		desiredByName := make(map[string]int32, len(desiredService.Spec.Ports))
		for _, p := range desiredService.Spec.Ports {
			desiredByName[p.Name] = p.Port
		}

		for _, existingPort := range definedService.Spec.Ports {
			desiredPort, ok := desiredByName[existingPort.Name]
			if !ok || existingPort.Port != desiredPort {
				needsUpdate = true
				break
			}
		}
	}

	if needsUpdate {
		if err := r.Update(ctx, desiredService); err != nil {
			return err
		}
		logr.Info("Updated: Service")
		r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "Service %s Updated", ords.Name)
	}

	return nil
}

/*
************************************************
  - Definers

*************************************************
*/
func objectMetaDefine(
	ordssrvs *dbapi.OrdsSrvs,
	name string,
	labels map[string]string,
	annotations map[string]string,
) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        name,
		Namespace:   ordssrvs.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
}

func selectorDefine(ordssrvs *dbapi.OrdsSrvs) metav1.LabelSelector {
	labels := getSelectorLabels(ordssrvs)
	return metav1.LabelSelector{
		MatchLabels: labels,
	}
}

func (r *OrdsSrvsReconciler) podTemplateSpecDefine(
	ctx context.Context,
	ordssrvs *dbapi.OrdsSrvs,
	rState *OrdsSrvsReconcileState,
	_ ctrl.Request) (corev1.PodTemplateSpec, error) {
	specVolumes, specVolumeMounts := r.VolumesDefine(ordssrvs, rState)

	const (
		defaultHTTPPort  int32 = 8080
		defaultHTTPSPort int32 = 8443
		defaultMongoPort int32 = 27017
	)

	httpPort := defaultHTTPPort
	if ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort != nil {
		httpPort = *ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort
	}

	httpsPort := defaultHTTPSPort
	if ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort != nil {
		httpsPort = *ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort
	}

	envPorts := []corev1.ContainerPort{
		{
			ContainerPort: httpPort,
			Name:          targetHTTPPortName,
		},
		{
			ContainerPort: httpsPort,
			Name:          targetHTTPSPortName,
		},
	}

	if ordssrvs.Spec.GlobalSettings.MongoEnabled {
		mongoPort := defaultMongoPort
		if ordssrvs.Spec.GlobalSettings.MongoPort != nil {
			mongoPort = *ordssrvs.Spec.GlobalSettings.MongoPort
		}
		envPorts = append(envPorts, corev1.ContainerPort{
			ContainerPort: mongoPort,
			Name:          targetMongoPortName,
		})
	}

	var mainRes corev1.ResourceRequirements
	if ordssrvs.Spec.Resources != nil {
		mainRes = *ordssrvs.Spec.Resources
	}

	podLabels := getSystemCommonPodLabels(ordssrvs, rState)
	podAnnotations := getSystemCommonPodAnnotations(ordssrvs, rState)
	initEnv, err := r.envDefine(ctx, ordssrvs, rState, true)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	mainEnv, err := r.envDefine(ctx, ordssrvs, rState, false)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	podSpecTemplate :=
		corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      podLabels,
				Annotations: podAnnotations,
			},
			Spec: corev1.PodSpec{
				Volumes:         specVolumes,
				SecurityContext: podSecurityContextDefine(),
				InitContainers: []corev1.Container{{
					Image:           ordssrvs.Spec.Image,
					Name:            ordssrvs.Name + "-init",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Resources:       mainRes,
					SecurityContext: securityContextDefine(),
					Command:         []string{"/bin/bash", "-c", ordsSABase + "/scripts/ords_init.sh"},
					Env:             initEnv,
					VolumeMounts:    specVolumeMounts,
				}},
				Containers: []corev1.Container{{
					Image:           ordssrvs.Spec.Image,
					Name:            ordssrvs.Name,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Resources:       mainRes,
					SecurityContext: securityContextDefine(),
					Ports:           envPorts,
					Command:         []string{"/bin/bash", "-c", ordsSABase + "/scripts/ords_start.sh"},
					Env:             mainEnv,
					VolumeMounts:    specVolumeMounts,
				}},
				ServiceAccountName: ordssrvs.Spec.ServiceAccountName,
			},
		}

	return podSpecTemplate, nil
}

// VolumesDefine defines the volumes for OrdsSrvs.
func (r *OrdsSrvsReconciler) VolumesDefine(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) ([]corev1.Volume, []corev1.VolumeMount) {

	// Initialize the slice to hold specifications
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	// scripts
	scriptsVolume := configMapvolumeBuild(rState.ordssrvsScriptsConfigMapName, rState.ordssrvsScriptsConfigMapName, 0770)
	scriptsVolumeMount := volumeMountBuild(rState.ordssrvsScriptsConfigMapName, ordsSABase+"/scripts", true)
	volumes = append(volumes, scriptsVolume)
	volumeMounts = append(volumeMounts, scriptsVolumeMount)

	if rState.passwordEncryption {
		secretName := ordssrvs.Spec.EncPrivKey.SecretName
		encryptionKeyVolumeName := ordssrvs.Name + "-secret-" + secretName
		encryptionKeyVolume := secretVolumeBuild(encryptionKeyVolumeName, secretName)
		encryptionKeyVolumeMount := volumeMountBuild(encryptionKeyVolumeName, "/opt/oracle/sa/encryptionPrivateKey/", true)

		volumes = append(volumes, encryptionKeyVolume)
		volumeMounts = append(volumeMounts, encryptionKeyVolumeMount)
	}

	if rState.APEXInstallationExternal == "true" {
		// volume for APEX installation, same optional folder as for ORDS image
		apexInstallationVolume := r.APEXInstallationVolumeDefine(rState)
		apexInstallationReadOnly := false
		apexInstallationVolumeMount := volumeMountBuild(APEXInstallationPV, APEXInstallationMount, apexInstallationReadOnly)
		volumes = append(volumes, apexInstallationVolume)
		volumeMounts = append(volumeMounts, apexInstallationVolumeMount)
	}

	if ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName != "" {
		secretName := ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName
		rState.specDebug.Info("ZipWalletsSecretName : " + secretName)
		globalCertVolume := secretVolumeBuild(secretName, secretName)
		globalCertVolumeMount := volumeMountBuild(secretName, ordsSABase+"/zipwallets/", true)

		volumes = append(volumes, globalCertVolume)
		volumeMounts = append(volumeMounts, globalCertVolumeMount)
	}

	// Build volume specifications for globalSettings
	globalVolume := emptyDirVolumeBuild("global")
	globalVolumeMount := volumeMountBuild("global", ordsSABase+"/config/global/", false)

	globalLogVolume := emptyDirVolumeBuild("sa-log-global")
	globalLogVolumeMount := volumeMountBuild("sa-log-global", ordsSABase+"/log/global/", false)

	globalConfigVolume := configMapvolumeBuild(rState.ordssrvsGlobalSettingsConfigMapName, rState.ordssrvsGlobalSettingsConfigMapName)
	globalConfigVolumeMount := volumeMountBuild(rState.ordssrvsGlobalSettingsConfigMapName, ordsSABase+"/config/stage/", true)

	volumes = append(volumes, globalVolume, globalLogVolume, globalConfigVolume)
	volumeMounts = append(volumeMounts, globalVolumeMount, globalLogVolumeMount, globalConfigVolumeMount)

	// Certificates
	if ordssrvs.Spec.GlobalSettings.CertSecret != nil {
		secretName := ordssrvs.Spec.GlobalSettings.CertSecret.SecretName
		globalCertVolume := secretVolumeBuild(secretName, secretName)
		globalCertVolumeMount := volumeMountBuild(secretName, ordsSABase+"/config/certficate/", true)

		volumes = append(volumes, globalCertVolume)
		volumeMounts = append(volumeMounts, globalCertVolumeMount)
	}

	// Build volume specifications for each pool in poolSettings
	definedVolumes := make(map[string]bool)

	for i := 0; i < len(ordssrvs.Spec.PoolSettings); i++ {
		poolName := ordssrvs.Spec.PoolSettings[i].PoolName
		poolKey := poolNameToK8sKey(poolName)

		// /opt/oracle/sa/config/databases/POOL/
		poolConfigName := ordssrvs.Name + "-cfg-pool-" + poolKey
		poolConfigVolume := configMapvolumeBuild(poolConfigName, poolConfigName)
		poolConfigVolumeMount := volumeMountBuild(poolConfigName, ordsSABase+"/config/databases/"+poolName+"/", true)
		volumes = append(volumes, poolConfigVolume)
		volumeMounts = append(volumeMounts, poolConfigVolumeMount)

		// PoolWalletSecret -> /opt/oracle/sa/config/databases/POOL/wallet/
		poolWalletVolumeName := ordssrvs.Name + "-pool-wallet-" + poolKey
		poolWalletVolumePath := ordsSABase + "/config/databases/" + poolName + "/wallet/"

		//if ( ( ordssrvs.Spec.PoolSettings[i].PoolWalletSecret == nil )  ){
		poolWalletVolume := emptyDirVolumeBuild(poolWalletVolumeName)
		poolWalletVolumeMount := volumeMountBuild(poolWalletVolumeName, poolWalletVolumePath, false)
		volumes = append(volumes, poolWalletVolume)
		volumeMounts = append(volumeMounts, poolWalletVolumeMount)
		//} else {
		//	poolWalletSecretName := ordssrvs.Spec.PoolSettings[i].PoolWalletSecret.SecretName
		//	if !definedVolumes[poolWalletVolumeName] {
		//		poolWalletVolume := secretVolumeBuild(poolWalletVolumeName, poolWalletSecretName)
		//		volumes = append(volumes, poolWalletVolume)
		//		definedVolumes[poolWalletVolumeName] = true
		//	}
		//	poolWalletVolumeMount := volumeMountBuild(poolWalletVolumeName, poolWalletVolumePath, true)
		//	volumeMounts = append(volumeMounts, poolWalletVolumeMount)
		//}

		// DBWalletSecret -> /opt/oracle/sa/config/databases/POOL/network/admin/
		if ordssrvs.Spec.PoolSettings[i].DBWalletSecret != nil {
			dbWalletSecretName := ordssrvs.Spec.PoolSettings[i].DBWalletSecret.SecretName
			volumeName := ordssrvs.Name + "-pool-zipwallet-" + poolKey
			if !definedVolumes[volumeName] {
				poolDBWalletVolume := secretVolumeBuild(volumeName, dbWalletSecretName)
				volumes = append(volumes, poolDBWalletVolume)
				definedVolumes[volumeName] = true
			}
			poolDBWalletVolumeMount := volumeMountBuild(volumeName, ordsSABase+"/config/databases/"+poolName+"/network/admin/", true)
			volumeMounts = append(volumeMounts, poolDBWalletVolumeMount)
		}

		// TNSAdminSecret -> /opt/oracle/sa/config/databases/POOL/network/admin/
		if ordssrvs.Spec.PoolSettings[i].TNSAdminSecret != nil {
			if ordssrvs.Spec.PoolSettings[i].DBWalletSecret == nil {
				tnsSecretName := ordssrvs.Spec.PoolSettings[i].TNSAdminSecret.SecretName
				poolTNSAdminVolumeName := ordssrvs.Name + "-pool-netadmin-" + poolKey
				if !definedVolumes[poolTNSAdminVolumeName] {
					poolTNSAdminVolume := secretVolumeBuild(poolTNSAdminVolumeName, tnsSecretName)
					volumes = append(volumes, poolTNSAdminVolume)
					definedVolumes[poolTNSAdminVolumeName] = true
				}
				poolTNSAdminVolumeMount := volumeMountBuild(poolTNSAdminVolumeName, ordsSABase+"/config/databases/"+poolName+"/network/admin/", true)
				volumeMounts = append(volumeMounts, poolTNSAdminVolumeMount)
			} else {
				rState.specDebug.Info("Attribute TNSAdminSecret ignored, using DBWalletSecret for pool " + poolName)
			}
		}

	}
	return volumes, volumeMounts
}

func volumeMountBuild(name string, path string, readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: path,
		ReadOnly:  readOnly,
	}
}

func secretVolumeBuild(volumeName string, secretName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
}

func emptyDirVolumeBuild(volumeName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func configMapvolumeBuild(volumeName string, mapName string, mode ...int32) corev1.Volume {
	defaultMode := int32(0660)
	if len(mode) > 0 {
		defaultMode = mode[0]
	}
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				DefaultMode: &defaultMode,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mapName,
				},
			},
		},
	}
}

// ServiceDefine defines the Service for OrdsSrvs.
func (r *OrdsSrvsReconciler) ServiceDefine(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, HTTPport int32, HTTPSport int32, MongoPort int32) (*corev1.Service, error) {

	servicePorts := []corev1.ServicePort{
		{
			Name:       serviceHTTPPortName,
			Protocol:   corev1.ProtocolTCP,
			Port:       HTTPport,
			TargetPort: intstr.FromString(targetHTTPPortName),
		},
		{
			Name:       serviceHTTPSPortName,
			Protocol:   corev1.ProtocolTCP,
			Port:       HTTPSport,
			TargetPort: intstr.FromString(targetHTTPSPortName),
		},
	}

	if ordssrvs.Spec.GlobalSettings.MongoEnabled {
		mongoServicePort := corev1.ServicePort{
			Name:       serviceMongoPortName,
			Protocol:   corev1.ProtocolTCP,
			Port:       MongoPort,
			TargetPort: intstr.FromString(targetMongoPortName),
		}
		servicePorts = append(servicePorts, mongoServicePort)
	}

	selectorLabels := getSelectorLabels(ordssrvs)
	labels := getSystemCommonServiceLabels(ordssrvs, rState)
	annotations := getSystemCommonServiceAnnotations(ordssrvs, rState)
	objectMeta := objectMetaDefine(ordssrvs, ordssrvs.Name, labels, annotations)
	def := &corev1.Service{
		ObjectMeta: objectMeta,
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports:    servicePorts,
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ordssrvs, def, r.Scheme); err != nil {
		return nil, fmt.Errorf("set owner reference for service %s/%s: %w", ordssrvs.Namespace, ordssrvs.Name, err)
	}
	return def, nil
}

func podSecurityContextDefine() *corev1.PodSecurityContext {

	return &corev1.PodSecurityContext{
		RunAsNonRoot: k8s.BoolPointer(true),
		RunAsUser:    k8s.Int64Pointer(dbcommons.ORACLE_UID),
		RunAsGroup:   k8s.Int64Pointer(dbcommons.ORACLE_GUID),
		FSGroup:      k8s.Int64Pointer(dbcommons.ORACLE_GUID),
	}

}

func securityContextDefine() *corev1.SecurityContext {

	return &corev1.SecurityContext{
		RunAsNonRoot:             k8s.BoolPointer(true),
		RunAsUser:                k8s.Int64Pointer(dbcommons.ORACLE_UID),
		RunAsGroup:               k8s.Int64Pointer(dbcommons.ORACLE_GUID),
		AllowPrivilegeEscalation: k8s.BoolPointer(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}

}

func addEnvVar(envVars []corev1.EnvVar, name string, value string) []corev1.EnvVar {
	newEnvVar := corev1.EnvVar{
		Name:  name,
		Value: value,
	}
	return append(envVars, newEnvVar)
}

func (r *OrdsSrvsReconciler) addSecretEnvVar(ctx context.Context, envVars []corev1.EnvVar, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, envName string, secretName string, secretKey string) ([]corev1.EnvVar, error) {

	logger := log.FromContext(ctx).WithName("addSecretEnvVar")

	message := fmt.Sprintf("Setting Secret env variable '%s' from secret '%s', key '%s' ", envName, secretName, secretKey)
	rState.specDebug.Info(message)

	// check secret exists
	var secret corev1.Secret
	err := r.Get(ctx, client.ObjectKey{Namespace: ordssrvs.Namespace, Name: secretName}, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e := fmt.Errorf("required secret %s/%s not found", ordssrvs.Namespace, secretName)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeWarning, "SecretNotFound", "Failed to resolve env %q: %v", envName, e)
			logger.Error(e, "SecretNotFound")
			return envVars, e
		}
		e := fmt.Errorf("get secret %s/%s: %w", ordssrvs.Namespace, secretName, err)
		logger.Error(e, "SecretReadError")
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeWarning, "SecretReadError", "Failed to resolve env %q: %v", envName, e)
		return envVars, e
	}

	// check key exists
	if _, exists := secret.Data[secretKey]; !exists {
		e := fmt.Errorf("secret %s/%s missing key %q", ordssrvs.Namespace, secretName, secretKey)
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeWarning, "SecretKeyMissing", "Failed to resolve env %q: %v", envName, e)
		logger.Error(e, "SecretKeyMissing")
		return envVars, e
	}

	newEnvVar := corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  secretKey,
			},
		},
	}

	return append(envVars, newEnvVar), nil
}

// Sets environment variables in the containers
func (r *OrdsSrvsReconciler) envDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, initContainer bool) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}

	// Central Configuration
	if ordssrvs.Spec.GlobalSettings.CentralConfigURL != "" {
		envVars = addEnvVar(envVars, "central_config_url", ordssrvs.Spec.GlobalSettings.CentralConfigURL)
		//envVars = addEnvVar(envVars, "central_config_wallet", ordssrvs.Spec.GlobalSettings.CentralConfigWallet)
	}

	// ORDS_CONFIG
	ordsConfig := ordsSABase + "/config"
	rState.specInfo.Info("Setting ORDS_CONFIG to " + ordsConfig)
	envVars = addEnvVar(envVars, "ORDS_CONFIG", ordsConfig)

	// adding info for private key
	if rState.passwordEncryption && initContainer {
		passwordKey := ordssrvs.Spec.EncPrivKey.PasswordKey
		rState.specDebug.Info("Setting ENC_PRV_KEY env variable to " + passwordKey)
		envVars = addEnvVar(envVars, "ENC_PRV_KEY", passwordKey)
	}

	// avoid Java warning about JAVA_TOOL_OPTIONS
	envVars = addEnvVar(envVars, "JAVA_TOOL_OPTIONS", "-Doracle.ml.version_check=false")

	// JDK_JAVA_OPTIONS
	if ordssrvs.Spec.JdkJavaOptions != "" {
		envVars = addEnvVar(envVars, "JDK_JAVA_OPTIONS", ordssrvs.Spec.JdkJavaOptions)
	}

	// Limitation case for ADB/mTLS/OraOper edge
	if len(ordssrvs.Spec.PoolSettings) == 1 {
		poolName := ordssrvs.Spec.PoolSettings[0].PoolName
		tnsAdmin := ordsSABase + "/config/databases/" + poolName + "/network/admin/"
		envVars = addEnvVar(envVars, "TNS_ADMIN", tnsAdmin)
	}

	// init container only
	if initContainer {

		envVars = addEnvVar(envVars, "download_apex", strconv.FormatBool(ordssrvs.Spec.GlobalSettings.APEXDownload))
		envVars = addEnvVar(envVars, "download_url_apex", ordssrvs.Spec.GlobalSettings.APEXDownloadURL)
		envVars = addEnvVar(envVars, "external_apex", rState.APEXInstallationExternal)

		instanceAPIAdminUser := ordssrvs.Spec.GlobalSettings.InstanceAPIAdminUser
		instanceAPIAdminSecretName := ordssrvs.Spec.GlobalSettings.InstanceAPIAdminSecret.SecretName
		if instanceAPIAdminUser != "" && instanceAPIAdminSecretName != "" {
			envVars = addEnvVar(envVars, "instance_api_admin_user", instanceAPIAdminUser)
			passwordKey := ordssrvs.Spec.GlobalSettings.InstanceAPIAdminSecret.PasswordKey
			var err error
			envName := "instance_api_admin_password"
			envVars, err = r.addSecretEnvVar(ctx, envVars, ordssrvs, rState, envName, instanceAPIAdminSecretName, passwordKey)
			if err != nil {
				return envVars, fmt.Errorf("resolve env %q: %w", envName, err)
			}
		}

		// passwords are set for the init container only
		for i := 0; i < len(ordssrvs.Spec.PoolSettings); i++ {
			poolVarName := poolNameToEnvPrefix(ordssrvs.Spec.PoolSettings[i].PoolName)
			rState.specInfo.Info("Preparing env for pool " + poolVarName)

			// dbconnectiontype
			// if set the init container will test the connection
			// if not set and provided by Central Configuration, init script will skip the connection test
			if ordssrvs.Spec.PoolSettings[i].DBConnectionType != "" {
				envVars = addEnvVar(envVars, poolVarName+"_dbconnectiontype", ordssrvs.Spec.PoolSettings[i].DBConnectionType)
			}

			// Zip Wallet (shared in )
			if ordssrvs.Spec.PoolSettings[i].DBConnectionType == "" && ordssrvs.Spec.PoolSettings[i].ZipWalletService != "" {
				envVars = addEnvVar(envVars, poolVarName+"_dbconnectiontype", "zipWallet")
			}

			// dbusername
			if ordssrvs.Spec.PoolSettings[i].DBUsername != "" {
				envVars = addEnvVar(envVars, poolVarName+"_dbusername", ordssrvs.Spec.PoolSettings[i].DBUsername)

				// dbpassword
				envName := poolVarName + "_dbpassword"
				// it can be provided by a wallet
				if ordssrvs.Spec.PoolSettings[i].DBSecret.SecretName != "" {
					secretName := ordssrvs.Spec.PoolSettings[i].DBSecret.SecretName
					secretKey := ordssrvs.Spec.PoolSettings[i].DBSecret.PasswordKey
					var err error
					envVars, err = r.addSecretEnvVar(ctx, envVars, ordssrvs, rState, envName, secretName, secretKey)
					if err != nil {
						return envVars, fmt.Errorf("resolve env %q: %w", envName, err)
					}
				}

			}

			// dbadminuser
			if ordssrvs.Spec.PoolSettings[i].DBAdminUser != "" {
				envVars = addEnvVar(envVars, poolVarName+"_dbadminuser", ordssrvs.Spec.PoolSettings[i].DBAdminUser)
				// autoupgrade only if dbAdminUser provided
				envVars = addEnvVar(envVars, poolVarName+"_autoupgrade_ords", strconv.FormatBool(ordssrvs.Spec.PoolSettings[i].AutoUpgradeORDS))
				envVars = addEnvVar(envVars, poolVarName+"_autoupgrade_apex", strconv.FormatBool(ordssrvs.Spec.PoolSettings[i].AutoUpgradeAPEX))

				// dbadminuserpassword
				if ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.SecretName != "" {
					envName := poolVarName + "_dbadminuserpassword"
					secretName := ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.SecretName
					secretKey := ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.PasswordKey
					var err error
					envVars, err = r.addSecretEnvVar(ctx, envVars, ordssrvs, rState, envName, secretName, secretKey)
					if err != nil {
						return envVars, fmt.Errorf("resolve env %q: %w", envName, err)
					}
				}

			}

			// dbcdbadminuser
			if ordssrvs.Spec.PoolSettings[i].DBCDBAdminUser != "" {
				envVars = addEnvVar(envVars, poolVarName+"_dbcdbadminuser", ordssrvs.Spec.PoolSettings[i].DBCDBAdminUser)

				// dbcdbadminuserpassword
				if ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName != "" {
					envName := poolVarName + "_dbcdbadminuserpassword"
					secretName := ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName
					secretKey := ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.PasswordKey
					var err error
					envVars, err = r.addSecretEnvVar(ctx, envVars, ordssrvs, rState, envName, secretName, secretKey)
					if err != nil {
						return envVars, fmt.Errorf("resolve env %q: %w", envName, err)
					}
				}
			}
		}
	}

	return envVars, nil
}

// APEXInstallationVolumeDefine defines the volume used for APEX installation.
func (r *OrdsSrvsReconciler) APEXInstallationVolumeDefine(rState *OrdsSrvsReconcileState) corev1.Volume {

	var vs corev1.VolumeSource
	if rState.APEXInstallationExternal == "false" {
		vs = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
		rState.specDebug.Info("APEXInstallationVolumeDefine - APEX installation on empty dir")
	} else {

		vs = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: APEXInstallationPVC,
				ReadOnly:  false,
			},
		}
		rState.specDebug.Info("APEXInstallationVolumeDefine - APEX installation PVC : " + APEXInstallationPVC)
	}

	volume := corev1.Volume{
		Name:         APEXInstallationPV,
		VolumeSource: vs,
	}

	return volume
}

// APEXInstallationPVCDefine defines the PVC used for APEX installation.
func (r *OrdsSrvsReconciler) APEXInstallationPVCDefine(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) (*corev1.PersistentVolumeClaim, error) {

	size := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.Size

	volumeName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName
	storageClassName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.StorageClass
	accessMode := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.AccessMode

	message := fmt.Sprintf("Preparing PVC definition, volumeName %s, storageClass %s, size %s, accessMode %s", volumeName, storageClassName, size, accessMode)
	rState.specDebug.Info(message)

	requests := corev1.ResourceList{}
	if size != "" {
		sizeQuantity, err := resource.ParseQuantity(size)
		if err != nil {
			return nil, fmt.Errorf("invalid APEX Installation PVC size %q: %w", size, err)
		}
		requests[corev1.ResourceStorage] = sizeQuantity
	}

	// PVC Definition
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        APEXInstallationPVC,
			Namespace:   ordssrvs.Namespace,
			Labels:      getSystemCommonLabels(ordssrvs, rState),
			Annotations: getSystemCommonAnnotations(ordssrvs, rState),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(accessMode)},
			Resources: corev1.VolumeResourceRequirements{
				Requests: requests,
			},
			VolumeName:       volumeName,
			StorageClassName: &storageClassName,
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ordssrvs, pvc, r.Scheme); err != nil {
		return nil, fmt.Errorf("set owner reference for PVC %s/%s: %w", ordssrvs.Namespace, APEXInstallationPVC, err)
	}

	return pvc, nil

}

/*************************************************
 * Deletions
 **************************************************/

// ConfigMapDelete deletes the ConfigMap for OrdsSrvs.
func (r *OrdsSrvsReconciler) ConfigMapDelete(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, reconcileState *OrdsSrvsReconcileState, definedPools map[string]bool) (err error) {
	// Delete Undefined Pool ConfigMaps
	configMapList := &corev1.ConfigMapList{}
	if err := r.List(ctx, configMapList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ordssrvs.Name}),
	); err != nil {
		return err
	}

	for _, configMap := range configMapList.Items {
		if configMap.Name == reconcileState.ordssrvsGlobalSettingsConfigMapName || configMap.Name == reconcileState.ordssrvsScriptsConfigMapName {
			continue
		}

		// ignore config maps created internally
		if strings.HasPrefix(configMap.Name, ordssrvs.Name+"-pool-sqlnet") {
			continue
		}

		if _, exists := definedPools[configMap.Name]; !exists {
			if err := r.Delete(ctx, &configMap); err != nil {
				return err
			}
			reconcileState.RestartPods = ordssrvs.Spec.ForceRestart
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "ConfigMap %s Deleted", configMap.Name)
		}
	}

	return nil
}

// WorkloadDelete deletes the workload resource for OrdsSrvs.
func (r *OrdsSrvsReconciler) WorkloadDelete(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, kind string) (err error) {
	logr := log.FromContext(ctx).WithName("WorkloadDelete")

	// Get Workloads
	deploymentList := &appsv1.DeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ordssrvs.Name}),
	); err != nil {
		return err
	}

	statefulSetList := &appsv1.StatefulSetList{}
	if err := r.List(ctx, statefulSetList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ordssrvs.Name}),
	); err != nil {
		return err
	}

	daemonSetList := &appsv1.DaemonSetList{}
	if err := r.List(ctx, daemonSetList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ordssrvs.Name}),
	); err != nil {
		return err
	}

	switch kind {
	case "StatefulSet":
		for _, deleteDaemonSet := range daemonSetList.Items {
			if err := r.Delete(ctx, &deleteDaemonSet); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteDeployment := range deploymentList.Items {
			if err := r.Delete(ctx, &deleteDeployment); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	case "DaemonSet":
		for _, deleteDeployment := range deploymentList.Items {
			if err := r.Delete(ctx, &deleteDeployment); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteStatefulSet := range statefulSetList.Items {
			if err := r.Delete(ctx, &deleteStatefulSet); err != nil {
				return err
			}
			logr.Info("Deleted StatefulSet: " + deleteStatefulSet.Name)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	default:
		for _, deleteStatefulSet := range statefulSetList.Items {
			if err := r.Delete(ctx, &deleteStatefulSet); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteDaemonSet := range daemonSetList.Items {
			if err := r.Delete(ctx, &deleteDaemonSet); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	}
	return nil
}

/*************************************************
 * Helpers
 **************************************************/

var reservedLabelKeys = map[string]struct{}{
	"app":                          {},
	"app.kubernetes.io/name":       {},
	"app.kubernetes.io/instance":   {},
	"app.kubernetes.io/managed-by": {},
	"app.kubernetes.io/component":  {},
	controllerLabelKey:             {},
}

func mergeLabels(dst map[string]string, src map[string]string, rState *OrdsSrvsReconcileState) {
	for k, v := range src {
		if _, reserved := reservedLabelKeys[k]; reserved {
			rState.specInfo.Info("Ignoring reserved label", "label", k)
			continue // operator wins
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
			continue
		}
		rState.specInfo.Info("Overriding existing label", "label", k)
		// non-system conflict: allow override
		dst[k] = v
	}
}

func mergeAnnotations(dst map[string]string, src map[string]string, _ *OrdsSrvsReconcileState) {
	for k, v := range src {
		dst[k] = v
	}
}

// Immutable, Never expand selector keys in future releases unless you also implement explicit migration logic.
func getSelectorLabels(ordssrvs *dbapi.OrdsSrvs) map[string]string {
	return map[string]string{
		"app":                        ordssrvs.Name,
		"app.kubernetes.io/instance": ordssrvs.Name,
		controllerLabelKey:           controllerLabelVal,
	}
}

func getSystemLabels(ordssrvs *dbapi.OrdsSrvs) map[string]string {
	systemLabels := getSelectorLabels(ordssrvs)
	systemLabels["app.kubernetes.io/name"] = "ordssrvs"
	systemLabels["app.kubernetes.io/managed-by"] = "oracle-database-operator"
	systemLabels["app.kubernetes.io/component"] = "ords"
	return systemLabels
}

func getSystemAnnotations(_ *dbapi.OrdsSrvs) map[string]string {
	return map[string]string{}
}

func getSystemCommonLabels(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	labels := getSystemLabels(ordssrvs)
	if ordssrvs.Spec.CommonMetadata != nil {
		mergeLabels(labels, ordssrvs.Spec.CommonMetadata.AdditionalLabels, rState)
	}
	return labels
}

func getSystemCommonWorkloadLabels(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	labels := getSystemCommonLabels(ordssrvs, rState)
	if ordssrvs.Spec.Workload != nil && ordssrvs.Spec.Workload.Metadata != nil {
		mergeLabels(labels, ordssrvs.Spec.Workload.Metadata.AdditionalLabels, rState)
	}
	return labels
}

func getSystemCommonPodLabels(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	labels := getSystemCommonLabels(ordssrvs, rState)
	if ordssrvs.Spec.PodTemplate != nil && ordssrvs.Spec.PodTemplate.Metadata != nil {
		mergeLabels(labels, ordssrvs.Spec.PodTemplate.Metadata.AdditionalLabels, rState)
	}
	return labels
}

func getSystemCommonServiceLabels(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	labels := getSystemCommonLabels(ordssrvs, rState)
	if ordssrvs.Spec.Service != nil && ordssrvs.Spec.Service.Metadata != nil {
		mergeLabels(labels, ordssrvs.Spec.Service.Metadata.AdditionalLabels, rState)
	}
	return labels
}

func getSystemCommonAnnotations(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	annotations := getSystemAnnotations(ordssrvs)
	if ordssrvs.Spec.CommonMetadata != nil {
		mergeAnnotations(annotations, ordssrvs.Spec.CommonMetadata.AdditionalAnnotations, rState)
	}
	return annotations
}

func getSystemCommonWorkloadAnnotations(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	annotations := getSystemCommonAnnotations(ordssrvs, rState)
	if ordssrvs.Spec.Workload != nil && ordssrvs.Spec.Workload.Metadata != nil {
		mergeAnnotations(annotations, ordssrvs.Spec.Workload.Metadata.AdditionalAnnotations, rState)
	}
	return annotations
}

func getSystemCommonPodAnnotations(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	annotations := getSystemCommonAnnotations(ordssrvs, rState)
	if ordssrvs.Spec.PodTemplate != nil && ordssrvs.Spec.PodTemplate.Metadata != nil {
		mergeAnnotations(annotations, ordssrvs.Spec.PodTemplate.Metadata.AdditionalAnnotations, rState)
	}
	return annotations
}

func getSystemCommonServiceAnnotations(ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) map[string]string {
	annotations := getSystemCommonAnnotations(ordssrvs, rState)
	if ordssrvs.Spec.Service != nil && ordssrvs.Spec.Service.Metadata != nil {
		mergeAnnotations(annotations, ordssrvs.Spec.Service.Metadata.AdditionalAnnotations, rState)
	}
	return annotations
}

func generateSpecHash(spec interface{}) string {
	byteArray, err := json.Marshal(spec)
	if err != nil {
		return ""
	}

	hash := sha256.New()
	_, err = hash.Write(byteArray)
	if err != nil {
		return ""
	}

	hashBytes := hash.Sum(nil)
	hashString := hex.EncodeToString(hashBytes[:8])

	return hashString
}

// CreateOrUpdateConfigMap creates the ConfigMap if missing, or updates it if Data changed.
// It sets the owner reference and toggles r.RestartPods when content changes.
func (r *OrdsSrvsReconciler) CreateOrUpdateConfigMap(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, cm *corev1.ConfigMap) error {

	// Ensure owner reference
	if err := ctrl.SetControllerReference(ordssrvs, cm, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	switch {
	case apierrors.IsNotFound(err):
		if err := r.Create(ctx, cm); err != nil {
			return err
		}
		rState.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "ConfigMap %s created", cm.Name)
		rState.specInfo.Info("Created ConfigMap", "name", cm.Name)
		return nil
	case err != nil:
		return err
	default:
		// Compare data (and binary data if you use it)
		if equality.Semantic.DeepEqual(existing.Data, cm.Data) &&
			equality.Semantic.DeepEqual(existing.BinaryData, cm.BinaryData) &&
			equality.Semantic.DeepEqual(existing.Labels, cm.Labels) &&
			equality.Semantic.DeepEqual(existing.Annotations, cm.Annotations) {
			return nil
		}
		// Preserve resourceVersion on update
		cm.ResourceVersion = existing.ResourceVersion
		if err := r.Update(ctx, cm); err != nil {
			return err
		}
		rState.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "ConfigMap %s updated", cm.Name)
		rState.specInfo.Info("Updated ConfigMap", "name", cm.Name)
		return nil
	}
}

// Helpers for string conversion

// poolName to k8s key to be used for configmap, volume names and volume mounts
var dashRuns = regexp.MustCompile(`-+`)

func poolNameToK8sKey(poolName string) string {
	s := strings.TrimSpace(poolName)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = dashRuns.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "pool"
	}
	return s
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9_]`)

// poolName to env prefix for pool env variables
func poolNameToEnvPrefix(poolName string) string {
	s := strings.TrimSpace(poolName)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = nonIdent.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "pool"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "p_" + s
	}
	return s
}
