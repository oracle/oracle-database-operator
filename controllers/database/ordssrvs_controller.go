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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger

 	ordssrvsScriptsConfigMapName string
    ordssrvsGlobalSettingsConfigMapName string
    APEXInstallationExternal string
    passwordEncryption bool

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

func (r *OrdsSrvsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("Reconcile")
	ordssrvs := &dbapi.OrdsSrvs{}
	r.RestartPods=false

	// Check if resource exists or was deleted
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Resource deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error retrieving resource")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute}, err
	}

	// empty encryption key
	if ordssrvs.Spec.EncPrivKey == (dbapi.PasswordSecret{}) {
		r.passwordEncryption = false
		logger.Info("Password Encryption key (EncPrivKey) not set")
	}else{
		r.passwordEncryption = true
		logger.Info("Password Encryption key (EncPrivKey) from secret "+ordssrvs.Spec.EncPrivKey.SecretName+", key "+ordssrvs.Spec.EncPrivKey.PasswordKey)
	}

	// Central Configuration
	if ordssrvs.Spec.GlobalSettings.CentralConfigUrl != ""{
		logger.Info("Using Central Configuration from URL : "+ ordssrvs.Spec.GlobalSettings.CentralConfigUrl)
	}

	// empty Global Settings
	if ordssrvs.Spec.GlobalSettings == (dbapi.GlobalSettings{}) {
		logger.Info("Global Settings is empty, using default values")
	}

	// APEXInstallationExternal
	// true: persistence volume
	// false: no persistence volume
	if ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName != "" || ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.StorageClass != "" {
		r.APEXInstallationExternal = "true"
	} else {
		r.APEXInstallationExternal = "false"
	}
	logger.Info("Setting env external_apex to " + r.APEXInstallationExternal)

	// Set the status as Unknown when no status are available
	if len(ordssrvs.Status.Conditions) == 0 {
		condition := metav1.Condition{Type: typeUnsyncedORDS, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"}
		if err := r.SetStatus(ctx, req, ordssrvs, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	r.ordssrvsScriptsConfigMapName = ordssrvs.Name + "-scripts-config-map"
	r.ordssrvsGlobalSettingsConfigMapName = ordssrvs.Name + "-global-settings-config-map"

	// ConfigMap - Scripts
	if err := r.ConfigMapReconcile(ctx, ordssrvs, r.ordssrvsScriptsConfigMapName, 0); err != nil {
		logger.Error(err, "Error in ConfigMapReconcile (init-script)")
		return ctrl.Result{}, err
	}

	if r.APEXInstallationExternal == "true" {
		// ApexInstallation PVC
		if err := r.ApexInstallationPVCReconcile(ctx, ordssrvs); err != nil {
			logger.Error(err, "Error in ApexInstallation PVC reconcile")
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("ApexInstallation PVC not defined, no external APEX installation files")
	}

	// ConfigMap - Global Settings
	if err := r.ConfigMapReconcile(ctx, ordssrvs, r.ordssrvsGlobalSettingsConfigMapName, 0); err != nil {
		logger.Error(err, "Error in ConfigMapReconcile (Global)")
		return ctrl.Result{}, err
	}

	// ConfigMap - Pool Settings
	definedPools := make(map[string]bool)
	for i := 0; i < len(ordssrvs.Spec.PoolSettings); i++ {
		poolName := strings.ToLower(ordssrvs.Spec.PoolSettings[i].PoolName)
		poolConfigMapName := ordssrvs.Name + "-cfg-pool-" + poolName
		if definedPools[poolConfigMapName] {
			return ctrl.Result{}, errors.New("poolName: " + poolName + " is not unique")
		}
		definedPools[poolConfigMapName] = true
		if err := r.ConfigMapReconcile(ctx, ordssrvs, poolConfigMapName, i); err != nil {
			logger.Error(err, "Error in ConfigMapReconcile (Pools)")
			return ctrl.Result{}, err
		}
	}
	if err := r.ConfigMapDelete(ctx, req, ordssrvs, definedPools); err != nil {
		logger.Error(err, "Error in ConfigMapDelete (Pools)")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		logger.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	// Set the Type as Unsynced when a pod restart is required
	if r.RestartPods {
		condition := metav1.Condition{Type: typeUnsyncedORDS, Status: metav1.ConditionTrue, Reason: "Unsynced", Message: "Configurations have changed"}
		if err := r.SetStatus(ctx, req, ordssrvs, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Workloads
	if err := r.WorkloadReconcile(ctx, req, ordssrvs, ordssrvs.Spec.WorkloadType); err != nil {
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
	if err := r.ServiceReconcile(ctx, ordssrvs); err != nil {
		logger.Error(err, "Error in ServiceReconcile")
		return ctrl.Result{}, err
	}

	// Set the Type as Available when a pod restart is not required
	if !r.RestartPods {
		condition := metav1.Condition{Type: typeAvailableORDS, Status: metav1.ConditionTrue, Reason: "Available", Message: "Workload in Sync"}
		if err := r.SetStatus(ctx, req, ordssrvs, condition); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.Get(ctx, req.NamespacedName, ordssrvs); err != nil {
		logger.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

/************************************************
 * Status
 *************************************************/
func (r *OrdsSrvsReconciler) SetStatus(ctx context.Context, req ctrl.Request, ords *dbapi.OrdsSrvs, statusCondition metav1.Condition) error {
	logr := log.FromContext(ctx).WithName("SetStatus")

	// Fetch before Status Update
	if err := r.Get(ctx, req.NamespacedName, ords); err != nil {
		logr.Error(err, "Failed to re-fetch")
		return err
	}
	var readyWorkload int32
	var desiredWorkload int32
	switch ords.Spec.WorkloadType {
	//nolint:goconst
	case "StatefulSet":
		workload := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, workload); err != nil {
			logr.Info("StatefulSet not ready")
		}
		readyWorkload = workload.Status.ReadyReplicas
		desiredWorkload = workload.Status.Replicas
	//nolint:goconst
	case "DaemonSet":
		workload := &appsv1.DaemonSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, workload); err != nil {
			logr.Info("DaemonSet not ready")
		}
		readyWorkload = workload.Status.NumberReady
		desiredWorkload = workload.Status.DesiredNumberScheduled
	default:
		workload := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, workload); err != nil {
			logr.Info("Deployment not ready")
		}
		readyWorkload = workload.Status.ReadyReplicas
		desiredWorkload = workload.Status.Replicas
	}

	var workloadStatus string
	switch readyWorkload {
	case 0:
		workloadStatus = "Preparing"
	case desiredWorkload:
		workloadStatus = "Healthy"
		ords.Status.OrdsInstalled = true
	default:
		workloadStatus = "Progressing"
	}

	mongoPort := int32(0)
	if ords.Spec.GlobalSettings.MongoEnabled {
		mongoPort = *ords.Spec.GlobalSettings.MongoPort
	}

	meta.SetStatusCondition(&ords.Status.Conditions, statusCondition)
	ords.Status.Status = workloadStatus
	ords.Status.WorkloadType = ords.Spec.WorkloadType
	ords.Status.ORDSVersion = strings.Split(ords.Spec.Image, ":")[1]
	ords.Status.HTTPPort = ords.Spec.GlobalSettings.StandaloneHTTPPort
	ords.Status.HTTPSPort = ords.Spec.GlobalSettings.StandaloneHTTPSPort
	ords.Status.MongoPort = mongoPort
	ords.Status.RestartRequired = r.RestartPods
	if err := r.Status().Update(ctx, ords); err != nil {
		logr.Error(err, "Failed to update Status")
		return err
	}
	return nil
}

/************************************************
 * APEX Installation PVC Reconcile
 *************************************************/
func (r *OrdsSrvsReconciler) ApexInstallationPVCReconcile(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) (err error) {
	logr := log.FromContext(ctx).WithName("ApexInstallationPVCReconcile")

	if ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.Size == "" {
		msg := "APEX Installation PVC Size not defined"
		err = r.Create(ctx, ordssrvs)
		logr.Error(err, msg)
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: APEXInstallationPVC, Namespace: ordssrvs.Namespace}, pvc)
	if err == nil {
		logr.Info("Found APEX Installation PVC : " + APEXInstallationPVC)
		return nil
	}

	volumeName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName
	pvc = r.APEXInstallationPVCDefine(ctx, ordssrvs)

	message := fmt.Sprintf("APEX Installation PVC : %s for PV : %s", APEXInstallationPVC, volumeName)
	logr.Info("Creating " + message)
	err = r.Create(ctx, pvc)
	if err != nil {
		logr.Error(err, "Failed to create "+message)
	}

	return err
}

/************************************************
 * ConfigMaps
 *************************************************/
func (r *OrdsSrvsReconciler) ConfigMapReconcile(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, configMapName string, poolIndex int) (err error) {
	logr := log.FromContext(ctx).WithName("ConfigMapReconcile")
	desiredConfigMap := r.ConfigMapDefine(ctx, ordssrvs, configMapName, poolIndex)

	// Create if ConfigMap not found
	definedConfigMap := &corev1.ConfigMap{}
	if err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ordssrvs.Namespace}, definedConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredConfigMap); err != nil {
				return err
			}
			logr.Info("Created: " + configMapName)
			r.RestartPods = true
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "ConfigMap %s Created", configMapName)
			// Requery for comparison
			if err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ordssrvs.Namespace}, definedConfigMap); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if !equality.Semantic.DeepEqual(definedConfigMap.Data, desiredConfigMap.Data) {
		if err = r.Update(ctx, desiredConfigMap); err != nil {
			return err
		}
		logr.Info("Updated: " + configMapName)
		r.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "ConfigMap %s Updated", configMapName)
	}
	return nil
}

/************************************************
 * Secrets - TODO (Watch and set RestartPods)
 *************************************************/
// func (r *OrdsSrvsReconciler) SecretsReconcile(ctx context.Context, ords *dbapi.OrdsSrvs, poolIndex int) (err error) {
// 	logr := log.FromContext(ctx).WithName("SecretsReconcile")
// 	definedSecret := &corev1.Secret{}

// 	// Want to set ownership on the Secret for watching; also detects if TNS_ADMIN is needed.
// 	if ords.Spec.PoolSettings[i].DBSecret != nil {
// 	}
// 	if ords.Spec.PoolSettings[i].DBAdminUserSecret != nil {
// 	}
// 	if ords.Spec.PoolSettings[i].DBCDBAdminUserSecret != nil {
// 	}
// 	if ords.Spec.PoolSettings[i].TNSAdminSecret != nil {
// 	}
// 	if ords.Spec.PoolSettings[i].DBWalletSecret != nil {
// 	}

// 	if ords.Spec.PoolSettings[i].TNSAdminSecret != nil {
// 		tnsSecretName := ords.Spec.PoolSettings[i].TNSAdminSecret.SecretName
// 		definedSecret := &corev1.Secret{}
// 		if err = r.Get(ctx, types.NamespacedName{Name: tnsSecretName, Namespace: ords.Namespace}, definedSecret); err != nil {
// 			ojdbcPropertiesData, ok := secret.Data["ojdbc.properties"]
// 			if ok {
// 				if err = r.Update(ctx, desiredConfigMap); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}

// 	return nil
// }

/************************************************
 * Workloads
 *************************************************/
func (r *OrdsSrvsReconciler) WorkloadReconcile(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, kind string) (err error) {
	logr := log.FromContext(ctx).WithName("WorkloadReconcile")
	objectMeta := objectMetaDefine(ordssrvs, ordssrvs.Name)
	selector := selectorDefine(ordssrvs)
	template := r.podTemplateSpecDefine(ordssrvs, ctx, req)

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
		desiredWorkload.(*appsv1.StatefulSet).ObjectMeta.Labels[specHashLabel] = desiredSpecHash
	case "DaemonSet":
		desiredWorkload = &appsv1.DaemonSet{
			ObjectMeta: objectMeta,
			Spec: appsv1.DaemonSetSpec{
				Selector: &selector,
				Template: template,
			},
		}
		desiredSpecHash = generateSpecHash(desiredWorkload.(*appsv1.DaemonSet).Spec)
		desiredWorkload.(*appsv1.DaemonSet).ObjectMeta.Labels[specHashLabel] = desiredSpecHash
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
		desiredWorkload.(*appsv1.Deployment).ObjectMeta.Labels[specHashLabel] = desiredSpecHash
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
				if statusErr := r.SetStatus(ctx, req, ordssrvs, condition); statusErr != nil {
					return statusErr
				}
				return err
			}
			logr.Info("Created: " + kind)
			r.RestartPods = false
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "Created %s", kind)

			return nil
		} else {
			return err
		}
	}

	definedLabelsField := reflect.ValueOf(definedWorkload).Elem().FieldByName("ObjectMeta").FieldByName("Labels")
	if definedLabelsField.IsValid() {
		specHashValue := definedLabelsField.MapIndex(reflect.ValueOf(specHashLabel))
		if specHashValue.IsValid() {
			definedSpecHash = specHashValue.Interface().(string)
		} else {
			return err
		}
	}

	if desiredSpecHash != definedSpecHash {
		logr.Info("Syncing Workload " + kind + " with new configuration")
		if err := r.Client.Update(ctx, desiredWorkload); err != nil {
			return err
		}
		r.RestartPods = true
		r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "Updated %s", kind)
	}

	if r.RestartPods && ordssrvs.Spec.ForceRestart {
		logr.Info("Cycling: " + kind)
		labelsField := reflect.ValueOf(desiredWorkload).Elem().FieldByName("Spec").FieldByName("Template").FieldByName("ObjectMeta").FieldByName("Labels")
		if labelsField.IsValid() {
			labels := labelsField.Interface().(map[string]string)
			labels["configMapChanged"] = time.Now().Format("20060102T150405Z")
			labelsField.Set(reflect.ValueOf(labels))
			if err := r.Update(ctx, desiredWorkload); err != nil {
				return err
			}
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Restart", "Restarted %s", kind)
			r.RestartPods = false
		}
	}

	return nil
}

// Service
func (r *OrdsSrvsReconciler) ServiceReconcile(ctx context.Context, ords *dbapi.OrdsSrvs) (err error) {
	logr := log.FromContext(ctx).WithName("ServiceReconcile")

	HTTPport := *ords.Spec.GlobalSettings.StandaloneHTTPPort
	HTTPSport := *ords.Spec.GlobalSettings.StandaloneHTTPSPort
	MongoPort := *ords.Spec.GlobalSettings.MongoPort

	desiredService := r.ServiceDefine(ctx, ords, HTTPport, HTTPSport, MongoPort)

	definedService := &corev1.Service{}
	if err = r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, definedService); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredService); err != nil {
				return err
			}
			logr.Info("Created: Service")
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Create", "Service %s Created", ords.Name)
			// Requery for comparison
			if err := r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, definedService); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	deisredPortCount := len(desiredService.Spec.Ports)
	definedPortCount := len(definedService.Spec.Ports)

	if deisredPortCount != definedPortCount {
		if err := r.Update(ctx, desiredService); err != nil {
			return err
		}
	}

	for _, existingPort := range definedService.Spec.Ports {
		if existingPort.Name == serviceHTTPPortName {
			if existingPort.Port != HTTPport {
				if err := r.Update(ctx, desiredService); err != nil {
					return err
				}
				logr.Info("Updated HTTP Service Port: " + existingPort.Name)
				r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "Service HTTP Port %s Updated", existingPort.Name)
			}
		}
		if existingPort.Name == serviceHTTPSPortName {
			if existingPort.Port != HTTPSport {
				if err := r.Update(ctx, desiredService); err != nil {
					return err
				}
				logr.Info("Updated HTTPS Service Port: " + existingPort.Name)
				r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "Service HTTPS Port %s Updated", existingPort.Name)
			}
		}
		if existingPort.Name == serviceMongoPortName {
			if existingPort.Port != MongoPort {
				if err := r.Update(ctx, desiredService); err != nil {
					return err
				}
				logr.Info("Updated Mongo Service Port: " + existingPort.Name)
				r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "Service Mongo Port %s Updated", existingPort.Name)
			}
		}
	}
	return nil
}

/*
************************************************
  - Definers

*************************************************
*/
func objectMetaDefine(ords *dbapi.OrdsSrvs, name string) metav1.ObjectMeta {
	labels := getLabels(ords.Name)
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: ords.Namespace,
		Labels:    labels,
	}
}

func selectorDefine(ords *dbapi.OrdsSrvs) metav1.LabelSelector {
	labels := getLabels(ords.Name)
	return metav1.LabelSelector{
		MatchLabels: labels,
	}
}

func (r *OrdsSrvsReconciler) podTemplateSpecDefine(ords *dbapi.OrdsSrvs, ctx context.Context, _ ctrl.Request) corev1.PodTemplateSpec {
	labels := getLabels(ords.Name)
	specVolumes, specVolumeMounts := r.VolumesDefine(ctx, ords)

	envPorts := []corev1.ContainerPort{
		{
			ContainerPort: *ords.Spec.GlobalSettings.StandaloneHTTPPort,
			Name:          targetHTTPPortName,
		},
		{
			ContainerPort: *ords.Spec.GlobalSettings.StandaloneHTTPSPort,
			Name:          targetHTTPSPortName,
		},
	}

	if ords.Spec.GlobalSettings.MongoEnabled {
		mongoPort := corev1.ContainerPort{
			ContainerPort: *ords.Spec.GlobalSettings.MongoPort,
			Name:          targetMongoPortName,
		}
		envPorts = append(envPorts, mongoPort)
	}

	podSpecTemplate :=
		corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				Volumes:         specVolumes,
				SecurityContext: podSecurityContextDefine(),
				InitContainers: []corev1.Container{{
					Image:           ords.Spec.Image,
					Name:            ords.Name + "-init",
					ImagePullPolicy: corev1.PullIfNotPresent,
					SecurityContext: securityContextDefine(),
					Command:         []string{"/bin/bash", "-c", ordsSABase + "/scripts/ords_init.sh"},
					Env:             r.envDefine(ords, true, ctx),
					VolumeMounts:    specVolumeMounts,
				}},
				Containers: []corev1.Container{{
					Image:           ords.Spec.Image,
					Name:            ords.Name,
					ImagePullPolicy: corev1.PullIfNotPresent,
					SecurityContext: securityContextDefine(),
					Ports:           envPorts,
					Command:         []string{"/bin/bash", "-c", ordsSABase + "/scripts/ords_start.sh"},
					// DEBUG mode, change to false
					Env:          r.envDefine(ords, true, ctx),
					VolumeMounts: specVolumeMounts,
				}},
				ServiceAccountName: ords.Spec.ServiceAccountName,
			},
		}

	return podSpecTemplate
}


// Volumes
func (r *OrdsSrvsReconciler) VolumesDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) ([]corev1.Volume, []corev1.VolumeMount) {

	logger := log.FromContext(ctx).WithName("VolumesDefine")

	// Initialize the slice to hold specifications
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	// scripts
	scriptsVolume := configMapvolumeBuild(r.ordssrvsScriptsConfigMapName, r.ordssrvsScriptsConfigMapName, 0770)
	scriptsVolumeMount := volumeMountBuild(r.ordssrvsScriptsConfigMapName, ordsSABase+"/scripts", true)
	volumes = append(volumes, scriptsVolume)
	volumeMounts = append(volumeMounts, scriptsVolumeMount)

	if r.passwordEncryption {
		secretName := ordssrvs.Spec.EncPrivKey.SecretName
		encryptionKeyVolume := secretVolumeBuild(secretName, secretName)
		encryptionKeyVolumeMount := volumeMountBuild(secretName, "/opt/oracle/sa/encryptionPrivateKey/", true)

		volumes = append(volumes, encryptionKeyVolume)
		volumeMounts = append(volumeMounts, encryptionKeyVolumeMount)
	}

	if r.APEXInstallationExternal == "true" {
		// volume for APEX installation, same optional folder as for ORDS image
		apexInstallationVolume := r.APEXInstallationVolumeDefine(ctx, ordssrvs)
		apexInstallationReadOnly := false
		apexInstallationVolumeMount := volumeMountBuild(APEXInstallationPV, APEXInstallationMount, apexInstallationReadOnly)
		volumes = append(volumes, apexInstallationVolume)
		volumeMounts = append(volumeMounts, apexInstallationVolumeMount)
	}

	if ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName != "" {
		secretName := ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName
		logger.Info("ZipWalletsSecretName : "+secretName)
		globalCertVolume := secretVolumeBuild(secretName, secretName)
		globalCertVolumeMount := volumeMountBuild(secretName, ordsSABase+"/zipwallets/", true)

		volumes = append(volumes, globalCertVolume)
		volumeMounts = append(volumeMounts, globalCertVolumeMount)
	}

	// Build volume specifications for globalSettings
	standaloneVolume := emptyDirVolumeBuild("standalone")
	standaloneVolumeMount := volumeMountBuild("standalone", ordsSABase+"/config/global/standalone/", false)

	credentialsVolume := emptyDirVolumeBuild("credentials")
	credentialsVolumeMount := volumeMountBuild("credentials", ordsSABase+"/config/global/credentials/", false)

	globalWalletVolume := emptyDirVolumeBuild("sa-wallet-global")
	globalWalletVolumeMount := volumeMountBuild("sa-wallet-global", ordsSABase+"/config/global/wallet/", false)

	globalLogVolume := emptyDirVolumeBuild("sa-log-global")
	globalLogVolumeMount := volumeMountBuild("sa-log-global", ordsSABase+"/log/global/", false)

	globalConfigVolume := configMapvolumeBuild(r.ordssrvsGlobalSettingsConfigMapName, r.ordssrvsGlobalSettingsConfigMapName)
	globalConfigVolumeMount := volumeMountBuild(r.ordssrvsGlobalSettingsConfigMapName, ordsSABase+"/config/global/", true)

	globalDocRootVolume := emptyDirVolumeBuild("sa-doc-root")
	globalDocRootVolumeMount := volumeMountBuild("sa-doc-root", ordsSABase+"/config/global/doc_root/", false)

	volumes = append(volumes, standaloneVolume, globalWalletVolume, globalLogVolume, globalConfigVolume, globalDocRootVolume, credentialsVolume)
	volumeMounts = append(volumeMounts, standaloneVolumeMount, globalWalletVolumeMount, globalLogVolumeMount, globalConfigVolumeMount, globalDocRootVolumeMount, credentialsVolumeMount)

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
		poolName := strings.ToLower(ordssrvs.Spec.PoolSettings[i].PoolName)

		// /opt/oracle/sa/config/databases/POOL/
		poolConfigName := ordssrvs.Name+"-cfg-pool-" + poolName
		poolConfigVolume := configMapvolumeBuild(poolConfigName, poolConfigName)
		poolConfigVolumeMount := volumeMountBuild(poolConfigName, ordsSABase+"/config/databases/"+poolName+"/", true)
		volumes = append(volumes, poolConfigVolume)
		volumeMounts = append(volumeMounts, poolConfigVolumeMount)

		// PoolWalletSecret -> /opt/oracle/sa/config/databases/POOL/wallet/
		poolWalletVolumeName := ordssrvs.Name+"-pool-wallet-" + poolName
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
			volumeName := ordssrvs.Name + "-pool-zipwallet-" + poolName
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
				poolTNSAdminVolumeName := ordssrvs.Name + "-pool-netadmin-" + poolName
				if !definedVolumes[poolTNSAdminVolumeName] {
					poolTNSAdminVolume := secretVolumeBuild(poolTNSAdminVolumeName, tnsSecretName)
					volumes = append(volumes, poolTNSAdminVolume)
					definedVolumes[poolTNSAdminVolumeName] = true
				}
				poolTNSAdminVolumeMount := volumeMountBuild(poolTNSAdminVolumeName, ordsSABase+"/config/databases/"+poolName+"/network/admin/", true)
				volumeMounts = append(volumeMounts, poolTNSAdminVolumeMount)
			} else {
				logger.Info("Attribute TNSAdminSecret ignored, using DBWalletSecret for pool " + poolName)
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

// Service
func (r *OrdsSrvsReconciler) ServiceDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, HTTPport int32, HTTPSport int32, MongoPort int32) *corev1.Service {
	labels := getLabels(ordssrvs.Name)

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

	objectMeta := objectMetaDefine(ordssrvs, ordssrvs.Name)
	def := &corev1.Service{
		ObjectMeta: objectMeta,
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ordssrvs, def, r.Scheme); err != nil {
		return nil
	}
	return def
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

func (r *OrdsSrvsReconciler) addSecretEnvVar(envVars []corev1.EnvVar, ordssrvs *dbapi.OrdsSrvs, envName string, secretName string, secretKey string, ctx context.Context) []corev1.EnvVar {

	logger := log.FromContext(ctx).WithName("addSecretEnvVar")
	message := fmt.Sprintf("Setting Secret env variable '%s' from secret '%s', key '%s' ", envName, secretName, secretKey)
	logger.Info(message)

	// check secret exists
	var secret corev1.Secret
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: ordssrvs.Namespace, Name: secretName}, &secret)
	if err != nil {
		logger.Error(err, "Secret not found "+secretName)
		return envVars
	}

	// check key exists
	if _, exists := secret.Data[secretKey]; !exists {
		logger.Info("Secret key " + secretKey + " not found for secret " + secretName)
		return envVars
	}

	newEnvVar := corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: secretKey,
			},
		},
	}

	return append(envVars, newEnvVar)
}

// Sets environment variables in the containers
func (r *OrdsSrvsReconciler) envDefine(ordssrvs *dbapi.OrdsSrvs, initContainer bool, ctx context.Context) []corev1.EnvVar {
	logger := log.FromContext(ctx).WithName("envDefine")

	envVars := []corev1.EnvVar{}

	// Central Configuration
	if ordssrvs.Spec.GlobalSettings.CentralConfigUrl != "" {
		envVars = addEnvVar(envVars, "central_config_url", ordssrvs.Spec.GlobalSettings.CentralConfigUrl)
		//envVars = addEnvVar(envVars, "central_config_wallet", ordssrvs.Spec.GlobalSettings.CentralConfigWallet)
	}

	// ORDS_CONFIG
	ORDS_CONFIG := ordsSABase + "/config"
	logger.Info("Setting ORDS_CONFIG to " + ORDS_CONFIG)
	envVars = addEnvVar(envVars, "ORDS_CONFIG", ORDS_CONFIG)

	// adding info for private key
	if r.passwordEncryption && initContainer {
		passwordKey := ordssrvs.Spec.EncPrivKey.PasswordKey
		logger.Info("Setting ENC_PRV_KEY env variable to " + passwordKey)
		envVars = addEnvVar(envVars, "ENC_PRV_KEY", passwordKey)
	}

	// avoid Java warning about JAVA_TOOL_OPTIONS
	envVars = addEnvVar(envVars, "JAVA_TOOL_OPTIONS", "-Doracle.ml.version_check=false")

	// Limitation case for ADB/mTLS/OraOper edge
	if len(ordssrvs.Spec.PoolSettings) == 1 {
		poolName := strings.ToLower(ordssrvs.Spec.PoolSettings[0].PoolName)
		tnsAdmin := ordsSABase + "/config/databases/" + poolName + "/network/admin/"
		envVars = addEnvVar(envVars, "TNS_ADMIN", tnsAdmin)
	}

	// init container only
	if initContainer {

		envVars = addEnvVar(envVars, "download_apex", strconv.FormatBool(ordssrvs.Spec.GlobalSettings.APEXDownload))
		envVars = addEnvVar(envVars, "download_url_apex", ordssrvs.Spec.GlobalSettings.APEXDownloadUrl)
		envVars = addEnvVar(envVars, "external_apex", r.APEXInstallationExternal)

		// passwords are set for the init container only
		for i := 0; i < len(ordssrvs.Spec.PoolSettings); i++ {
			poolName := strings.ReplaceAll(strings.ToLower(ordssrvs.Spec.PoolSettings[i].PoolName), "-", "_")
			logger.Info("Preparing env for pool " + poolName)

			// dbconnectiontype
			// if set the init container will test the connection
			// if not set and provided by Central Configuration, init script will skip the connection test
			if ordssrvs.Spec.PoolSettings[i].DBConnectionType != "" {
				envVars = addEnvVar(envVars, poolName + "_dbconnectiontype", ordssrvs.Spec.PoolSettings[i].DBConnectionType)
			}

			// Zip Wallet (shared in )
			if ordssrvs.Spec.PoolSettings[i].DBConnectionType == "" && ordssrvs.Spec.PoolSettings[i].ZipWalletService != "" {
				envVars = addEnvVar(envVars, poolName + "_dbconnectiontype", "zipWallet")
			}

			// dbusername
			if ordssrvs.Spec.PoolSettings[i].DBUsername != "" {
				envVars = addEnvVar(envVars, poolName + "_dbusername", ordssrvs.Spec.PoolSettings[i].DBUsername)

				// dbpassword
				envName := poolName + "_dbpassword"
				// it can be provided by a wallet
				if ordssrvs.Spec.PoolSettings[i].DBSecret.SecretName != "" {
				secretName := ordssrvs.Spec.PoolSettings[i].DBSecret.SecretName
				secretKey := ordssrvs.Spec.PoolSettings[i].DBSecret.PasswordKey
				envVars = r.addSecretEnvVar(envVars, ordssrvs, envName, secretName, secretKey, ctx)
				}

			}

			// dbadminuser
			if ordssrvs.Spec.PoolSettings[i].DBAdminUser != "" {
				envVars = addEnvVar(envVars, poolName + "_dbadminuser", ordssrvs.Spec.PoolSettings[i].DBAdminUser)
				// autoupgrade only if dbAdminUser provided
				envVars = addEnvVar(envVars, poolName+"_autoupgrade_ords", strconv.FormatBool(ordssrvs.Spec.PoolSettings[i].AutoUpgradeORDS))
				envVars = addEnvVar(envVars, poolName+"_autoupgrade_apex", strconv.FormatBool(ordssrvs.Spec.PoolSettings[i].AutoUpgradeAPEX))

				// dbadminuserpassword
				if ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.SecretName != "" {
					envName := poolName + "_dbadminuserpassword"
					secretName := ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.SecretName
					secretKey := ordssrvs.Spec.PoolSettings[i].DBAdminUserSecret.PasswordKey
					envVars = r.addSecretEnvVar(envVars, ordssrvs, envName, secretName, secretKey, ctx)
				}

			}

			// dbcdbadminuser
			if ordssrvs.Spec.PoolSettings[i].DBCDBAdminUser != "" {
				envVars = addEnvVar(envVars, poolName + "_dbcdbadminuser", ordssrvs.Spec.PoolSettings[i].DBCDBAdminUser)

				// dbcdbadminuserpassword
				if ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName != "" {
					envName := poolName + "_dbcdbadminuserpassword"
					secretName := ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName
					secretKey := ordssrvs.Spec.PoolSettings[i].DBCDBAdminUserSecret.PasswordKey
					envVars = r.addSecretEnvVar(envVars, ordssrvs, envName, secretName, secretKey, ctx)
				}
			}
		}
	}

	return envVars
}

func (r *OrdsSrvsReconciler) APEXInstallationVolumeDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) corev1.Volume {
	logger := log.FromContext(ctx).WithName("APEXInstallationVolumeDefine")

	var vs corev1.VolumeSource
	if r.APEXInstallationExternal == "false" {
		vs = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
		logger.Info("APEX installation on empty dir")
	} else {

		vs = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: APEXInstallationPVC,
				ReadOnly:  false,
			},
		}
		logger.Info("APEX installation PVC : " + APEXInstallationPVC)
	}

	volume := corev1.Volume{
		Name:         APEXInstallationPV,
		VolumeSource: vs,
	}

	return volume
}

func (r *OrdsSrvsReconciler) APEXInstallationPVCDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) *corev1.PersistentVolumeClaim {
	logger := log.FromContext(ctx).WithName("APEXInstallationPVCDefine")

	size := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.Size

	if size == "" {

	}

	volumeName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.VolumeName
	storageClassName := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.StorageClass
	accessMode := ordssrvs.Spec.GlobalSettings.APEXInstallationPersistence.AccessMode

	message := fmt.Sprintf("Preparing PVC definition, volumeName %s, storageClass %s, size %s, accessMode %s", volumeName, storageClassName, size, accessMode)
	logger.Info(message)

	// PVC Definition
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      APEXInstallationPVC,
			Namespace: ordssrvs.Namespace,
			Labels:    getLabels(ordssrvs.Name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(accessMode)},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
			VolumeName:       volumeName,
			StorageClassName: &storageClassName,
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ordssrvs, pvc, r.Scheme); err != nil {
		return nil
	}

	return pvc

}

/*************************************************
 * Deletions
 **************************************************/
func (r *OrdsSrvsReconciler) ConfigMapDelete(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, definedPools map[string]bool) (err error) {
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
		if configMap.Name == r.ordssrvsGlobalSettingsConfigMapName || configMap.Name == r.ordssrvsScriptsConfigMapName {
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
			r.RestartPods = ordssrvs.Spec.ForceRestart
			r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Delete", "ConfigMap %s Deleted", configMap.Name)
		}
	}

	return nil
}

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
func getLabels(name string) map[string]string {
	return map[string]string{
		"app": name,
		"app.kubernetes.io/instance": name,
		controllerLabelKey:           controllerLabelVal,
	}
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
func (r *OrdsSrvsReconciler) CreateOrUpdateConfigMap(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, cm *corev1.ConfigMap) error {
logger := log.FromContext(ctx).WithName("CreateOrUpdateConfigMap")

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
r.RestartPods = true
r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Create", "ConfigMap %s created", cm.Name)
logger.Info("Created ConfigMap", "name", cm.Name)
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
r.RestartPods = true
r.Recorder.Eventf(ordssrvs, corev1.EventTypeNormal, "Update", "ConfigMap %s updated", cm.Name)
logger.Info("Updated ConfigMap", "name", cm.Name)
return nil
}
}
