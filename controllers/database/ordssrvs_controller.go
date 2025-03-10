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
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	//	dbapi "example.com/oracle-ords-operator/api/v1"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
)

// Definitions of Standards
const (
	ordsSABase           = "/opt/oracle/sa"
	serviceHTTPPortName  = "svc-http-port"
	serviceHTTPSPortName = "svc-https-port"
	serviceMongoPortName = "svc-mongo-port"
	targetHTTPPortName   = "pod-http-port"
	targetHTTPSPortName  = "pod-https-port"
	targetMongoPortName  = "pod-mongo-port"
	globalConfigMapName  = "settings-global"
	poolConfigPreName    = "settings-" // Append PoolName
	controllerLabelKey   = "oracle.com/ords-operator-filter"
	controllerLabelVal   = "oracle-database-operator"
	specHashLabel        = "oracle.com/ords-operator-spec-hash"
)

// Definitions to manage status conditions
const (
	// typeAvailableORDS represents the status of the Workload reconciliation
	typeAvailableORDS = "Available"
	// typeUnsyncedORDS represents the status used when the configuration has changed but the Workload has not been restarted.
	typeUnsyncedORDS = "Unsynced"
)

// Trigger a restart of Pods on Config Changes
var RestartPods bool = false

// OrdsSrvsReconciler reconciles a OrdsSrvs object
type OrdsSrvsReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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
		Complete(r)
}

func (r *OrdsSrvsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logr := log.FromContext(ctx)
	ords := &dbapi.OrdsSrvs{}

	// Check if resource exists or was deleted
	if err := r.Get(ctx, req.NamespacedName, ords); err != nil {
		if apierrors.IsNotFound(err) {
			logr.Info("Resource deleted")
			return ctrl.Result{}, nil
		}
		logr.Error(err, "Error retrieving resource")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute}, err
	}

	// Set the status as Unknown when no status are available
	if ords.Status.Conditions == nil || len(ords.Status.Conditions) == 0 {
		condition := metav1.Condition{Type: typeUnsyncedORDS, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"}
		if err := r.SetStatus(ctx, req, ords, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ConfigMap - Init Script
	if err := r.ConfigMapReconcile(ctx, ords, ords.Name+"-"+"init-script", 0); err != nil {
		logr.Error(err, "Error in ConfigMapReconcile (init-script)")
		return ctrl.Result{}, err
	}

	// ConfigMap - Global Settings
	if err := r.ConfigMapReconcile(ctx, ords, ords.Name+"-"+globalConfigMapName, 0); err != nil {
		logr.Error(err, "Error in ConfigMapReconcile (Global)")
		return ctrl.Result{}, err
	}

	// ConfigMap - Pool Settings
	definedPools := make(map[string]bool)
	for i := 0; i < len(ords.Spec.PoolSettings); i++ {
		poolName := strings.ToLower(ords.Spec.PoolSettings[i].PoolName)
		poolConfigMapName := ords.Name + "-" + poolConfigPreName + poolName
		if definedPools[poolConfigMapName] {
			return ctrl.Result{}, errors.New("poolName: " + poolName + " is not unique")
		}
		definedPools[poolConfigMapName] = true
		if err := r.ConfigMapReconcile(ctx, ords, poolConfigMapName, i); err != nil {
			logr.Error(err, "Error in ConfigMapReconcile (Pools)")
			return ctrl.Result{}, err
		}
	}
	if err := r.ConfigMapDelete(ctx, req, ords, definedPools); err != nil {
		logr.Error(err, "Error in ConfigMapDelete (Pools)")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, ords); err != nil {
		logr.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	// // Secrets - Pool Settings
	// for i := 0; i < len(ords.Spec.PoolSettings); i++ {
	// 	if err := r.SecretsReconcile(ctx, ords, i); err != nil {
	// 		logr.Error(err, "Error in SecretsReconcile (Pools)")
	// 		return ctrl.Result{}, err
	// 	}
	// }

	// Set the Type as Unsynced when a pod restart is required
	if RestartPods {
		condition := metav1.Condition{Type: typeUnsyncedORDS, Status: metav1.ConditionTrue, Reason: "Unsynced", Message: "Configurations have changed"}
		if err := r.SetStatus(ctx, req, ords, condition); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Workloads
	if err := r.WorkloadReconcile(ctx, req, ords, ords.Spec.WorkloadType); err != nil {
		logr.Error(err, "Error in WorkloadReconcile")
		return ctrl.Result{}, err
	}
	if err := r.WorkloadDelete(ctx, req, ords, ords.Spec.WorkloadType); err != nil {
		logr.Error(err, "Error in WorkloadDelete")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, ords); err != nil {
		logr.Error(err, "Failed to re-fetch")
		return ctrl.Result{}, err
	}

	// Service
	if err := r.ServiceReconcile(ctx, ords); err != nil {
		logr.Error(err, "Error in ServiceReconcile")
		return ctrl.Result{}, err
	}

	// Set the Type as Available when a pod restart is not required
	if !RestartPods {
		condition := metav1.Condition{Type: typeAvailableORDS, Status: metav1.ConditionTrue, Reason: "Available", Message: "Workload in Sync"}
		if err := r.SetStatus(ctx, req, ords, condition); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.Get(ctx, req.NamespacedName, ords); err != nil {
		logr.Error(err, "Failed to re-fetch")
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
	if readyWorkload == 0 {
		workloadStatus = "Preparing"
	} else if readyWorkload == desiredWorkload {
		workloadStatus = "Healthy"
		ords.Status.OrdsInstalled = true
	} else {
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
	ords.Status.RestartRequired = RestartPods
	if err := r.Status().Update(ctx, ords); err != nil {
		logr.Error(err, "Failed to update Status")
		return err
	}
	return nil
}

/************************************************
 * ConfigMaps
 *************************************************/
func (r *OrdsSrvsReconciler) ConfigMapReconcile(ctx context.Context, ords *dbapi.OrdsSrvs, configMapName string, poolIndex int) (err error) {
	logr := log.FromContext(ctx).WithName("ConfigMapReconcile")
	desiredConfigMap := r.ConfigMapDefine(ctx, ords, configMapName, poolIndex)

	// Create if ConfigMap not found
	definedConfigMap := &corev1.ConfigMap{}
	if err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ords.Namespace}, definedConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredConfigMap); err != nil {
				return err
			}
			logr.Info("Created: " + configMapName)
			RestartPods = true
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Create", "ConfigMap %s Created", configMapName)
			// Requery for comparison
			if err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ords.Namespace}, definedConfigMap); err != nil {
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
		RestartPods = true
		r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "ConfigMap %s Updated", configMapName)
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
func (r *OrdsSrvsReconciler) WorkloadReconcile(ctx context.Context, req ctrl.Request, ords *dbapi.OrdsSrvs, kind string) (err error) {
	logr := log.FromContext(ctx).WithName("WorkloadReconcile")
	objectMeta := objectMetaDefine(ords, ords.Name)
	selector := selectorDefine(ords)
	template := r.podTemplateSpecDefine(ords, ctx, req)

	var desiredWorkload client.Object
	var desiredSpecHash string
	var definedSpecHash string

	switch kind {
	case "StatefulSet":
		desiredWorkload = &appsv1.StatefulSet{
			ObjectMeta: objectMeta,
			Spec: appsv1.StatefulSetSpec{
				Replicas: &ords.Spec.Replicas,
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
				Replicas: &ords.Spec.Replicas,
				Selector: &selector,
				Template: template,
			},
		}
		desiredSpecHash = generateSpecHash(desiredWorkload.(*appsv1.Deployment).Spec)
		desiredWorkload.(*appsv1.Deployment).ObjectMeta.Labels[specHashLabel] = desiredSpecHash
	}

	if err := ctrl.SetControllerReference(ords, desiredWorkload, r.Scheme); err != nil {
		return err
	}

	definedWorkload := reflect.New(reflect.TypeOf(desiredWorkload).Elem()).Interface().(client.Object)
	if err = r.Get(ctx, types.NamespacedName{Name: ords.Name, Namespace: ords.Namespace}, definedWorkload); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredWorkload); err != nil {
				condition := metav1.Condition{
					Type:    typeAvailableORDS,
					Status:  metav1.ConditionFalse,
					Reason:  "Reconciling",
					Message: fmt.Sprintf("Failed to create %s for the custom resource (%s): (%s)", kind, ords.Name, err),
				}
				if statusErr := r.SetStatus(ctx, req, ords, condition); statusErr != nil {
					return statusErr
				}
				return err
			}
			logr.Info("Created: " + kind)
			RestartPods = false
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Create", "Created %s", kind)

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
		RestartPods = true
		r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Update", "Updated %s", kind)
	}

	if RestartPods && ords.Spec.ForceRestart {
		logr.Info("Cycling: " + kind)
		labelsField := reflect.ValueOf(desiredWorkload).Elem().FieldByName("Spec").FieldByName("Template").FieldByName("ObjectMeta").FieldByName("Labels")
		if labelsField.IsValid() {
			labels := labelsField.Interface().(map[string]string)
			labels["configMapChanged"] = time.Now().Format("20060102T150405Z")
			labelsField.Set(reflect.ValueOf(labels))
			if err := r.Update(ctx, desiredWorkload); err != nil {
				return err
			}
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Restart", "Restarted %s", kind)
			RestartPods = false
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

func (r *OrdsSrvsReconciler) podTemplateSpecDefine(ords *dbapi.OrdsSrvs, ctx context.Context, req ctrl.Request) corev1.PodTemplateSpec {
	labels := getLabels(ords.Name)
	specVolumes, specVolumeMounts := VolumesDefine(ords)

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

	// Environment From Source
	podSpecTemplate :=
		corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				Volumes: specVolumes,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &[]bool{true}[0],
					FSGroup:      &[]int64{54321}[0],
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				InitContainers: []corev1.Container{{
					Image:           ords.Spec.Image,
					Name:            ords.Name + "-init",
					ImagePullPolicy: corev1.PullIfNotPresent,
					SecurityContext: securityContextDefine(),
					Command:         []string{"sh", "-c", ordsSABase + "/bin/init_script.sh"},
					Env:             r.envDefine(ords, true, ctx),
					VolumeMounts:    specVolumeMounts,
				}},
				Containers: []corev1.Container{{
					Image:           ords.Spec.Image,
					Name:            ords.Name,
					ImagePullPolicy: corev1.PullIfNotPresent,
					SecurityContext: securityContextDefine(),
					Ports:           envPorts,
					Command:         []string{"/bin/bash", "-c", "ords --config $ORDS_CONFIG serve --apex-images /opt/oracle/apex/$APEX_VER/images --debug"},
					Env:             r.envDefine(ords, false, ctx),
					VolumeMounts:    specVolumeMounts,
				}}},
		}

	return podSpecTemplate
}

// Volumes
func VolumesDefine(ords *dbapi.OrdsSrvs) ([]corev1.Volume, []corev1.VolumeMount) {
	// Initialize the slice to hold specifications
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	// SecretHelper
	secretHelperVolume := volumeBuild(ords.Name+"-"+"init-script", "ConfigMap", 0770)
	secretHelperVolumeMount := volumeMountBuild(ords.Name+"-"+"init-script", ordsSABase+"/bin", true)

	volumes = append(volumes, secretHelperVolume)
	volumeMounts = append(volumeMounts, secretHelperVolumeMount)

	// Build volume specifications for globalSettings
	standaloneVolume := volumeBuild("standalone", "EmptyDir")
	standaloneVolumeMount := volumeMountBuild("standalone", ordsSABase+"/config/global/standalone/", false)

	globalWalletVolume := volumeBuild("sa-wallet-global", "EmptyDir")
	globalWalletVolumeMount := volumeMountBuild("sa-wallet-global", ordsSABase+"/config/global/wallet/", false)

	globalLogVolume := volumeBuild("sa-log-global", "EmptyDir")
	globalLogVolumeMount := volumeMountBuild("sa-log-global", ordsSABase+"/log/global/", false)

	globalConfigVolume := volumeBuild(ords.Name+"-"+globalConfigMapName, "ConfigMap")
	globalConfigVolumeMount := volumeMountBuild(ords.Name+"-"+globalConfigMapName, ordsSABase+"/config/global/", true)

	globalDocRootVolume := volumeBuild("sa-doc-root", "EmptyDir")
	globalDocRootVolumeMount := volumeMountBuild("sa-doc-root", ordsSABase+"/config/global/doc_root/", false)

	volumes = append(volumes, standaloneVolume, globalWalletVolume, globalLogVolume, globalConfigVolume, globalDocRootVolume)
	volumeMounts = append(volumeMounts, standaloneVolumeMount, globalWalletVolumeMount, globalLogVolumeMount, globalConfigVolumeMount, globalDocRootVolumeMount)

	if ords.Spec.GlobalSettings.CertSecret != nil {
		globalCertVolume := volumeBuild(ords.Spec.GlobalSettings.CertSecret.SecretName, "Secret")
		globalCertVolumeMount := volumeMountBuild(ords.Spec.GlobalSettings.CertSecret.SecretName, ordsSABase+"/config/certficate/", true)

		volumes = append(volumes, globalCertVolume)
		volumeMounts = append(volumeMounts, globalCertVolumeMount)
	}

	// Build volume specifications for each pool in poolSettings
	definedWalletSecret := make(map[string]bool)
	definedTNSSecret := make(map[string]bool)
	for i := 0; i < len(ords.Spec.PoolSettings); i++ {
		poolName := strings.ToLower(ords.Spec.PoolSettings[i].PoolName)

		poolWalletName := "sa-wallet-" + poolName
		poolWalletVolume := volumeBuild(poolWalletName, "EmptyDir")
		poolWalletVolumeMount := volumeMountBuild(poolWalletName, ordsSABase+"/config/databases/"+poolName+"/wallet/", false)

		poolConfigName := ords.Name + "-" + poolConfigPreName + poolName
		poolConfigVolume := volumeBuild(poolConfigName, "ConfigMap")
		poolConfigVolumeMount := volumeMountBuild(poolConfigName, ordsSABase+"/config/databases/"+poolName+"/", true)

		volumes = append(volumes, poolWalletVolume, poolConfigVolume)
		volumeMounts = append(volumeMounts, poolWalletVolumeMount, poolConfigVolumeMount)

		if ords.Spec.PoolSettings[i].DBWalletSecret != nil {
			walletSecretName := ords.Spec.PoolSettings[i].DBWalletSecret.SecretName
			if !definedWalletSecret[walletSecretName] {
				// Only create the volume once
				poolDBWalletVolume := volumeBuild(walletSecretName, "Secret")
				volumes = append(volumes, poolDBWalletVolume)
				definedWalletSecret[walletSecretName] = true
			}
			poolDBWalletVolumeMount := volumeMountBuild(walletSecretName, ordsSABase+"/config/databases/"+poolName+"/network/admin/", true)
			volumeMounts = append(volumeMounts, poolDBWalletVolumeMount)
		}

		if ords.Spec.PoolSettings[i].TNSAdminSecret != nil {
			tnsSecretName := ords.Spec.PoolSettings[i].TNSAdminSecret.SecretName
			if !definedTNSSecret[tnsSecretName] {
				// Only create the volume once
				poolTNSAdminVolume := volumeBuild(tnsSecretName, "Secret")
				volumes = append(volumes, poolTNSAdminVolume)
				definedTNSSecret[tnsSecretName] = true
			}
			poolTNSAdminVolumeMount := volumeMountBuild(tnsSecretName, ordsSABase+"/config/databases/"+poolName+"/network/admin/", true)
			volumeMounts = append(volumeMounts, poolTNSAdminVolumeMount)
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

func volumeBuild(name string, source string, mode ...int32) corev1.Volume {
	defaultMode := int32(0660)
	if len(mode) > 0 {
		defaultMode = mode[0]
	}
	switch source {
	case "ConfigMap":
		return corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &defaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		}
	case "Secret":
		return corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: name,
				},
			},
		}
	case "EmptyDir":
		return corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	default:
		return corev1.Volume{}
	}
}

// Service
func (r *OrdsSrvsReconciler) ServiceDefine(ctx context.Context, ords *dbapi.OrdsSrvs, HTTPport int32, HTTPSport int32, MongoPort int32) *corev1.Service {
	labels := getLabels(ords.Name)

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

	if ords.Spec.GlobalSettings.MongoEnabled {
		mongoServicePort := corev1.ServicePort{
			Name:       serviceMongoPortName,
			Protocol:   corev1.ProtocolTCP,
			Port:       MongoPort,
			TargetPort: intstr.FromString(targetMongoPortName),
		}
		servicePorts = append(servicePorts, mongoServicePort)
	}

	objectMeta := objectMetaDefine(ords, ords.Name)
	def := &corev1.Service{
		ObjectMeta: objectMeta,
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ords, def, r.Scheme); err != nil {
		return nil
	}
	return def
}

func securityContextDefine() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             &[]bool{true}[0],
		RunAsUser:                &[]int64{54321}[0],
		AllowPrivilegeEscalation: &[]bool{false}[0],
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}
}

func (r *OrdsSrvsReconciler) envDefine(ords *dbapi.OrdsSrvs, initContainer bool, ctx context.Context) []corev1.EnvVar {
	envVarSecrets := []corev1.EnvVar{
		{
			Name:  "ORDS_CONFIG",
			Value: ordsSABase + "/config",
		},
		{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: "-Doracle.ml.version_check=false",
		},
	}

	// Limitation case for ADB/mTLS/OraOper edge
	if len(ords.Spec.PoolSettings) == 1 {
		poolName := strings.ToLower(ords.Spec.PoolSettings[0].PoolName)
		tnsAdmin := corev1.EnvVar{
			Name:  "TNS_ADMIN",
			Value: ordsSABase + "/config/databases/" + poolName + "/network/admin/",
		}
		envVarSecrets = append(envVarSecrets, tnsAdmin)
	}
	if initContainer {
		for i := 0; i < len(ords.Spec.PoolSettings); i++ {
			poolName := strings.ReplaceAll(strings.ToLower(ords.Spec.PoolSettings[i].PoolName), "-", "_")

			dbSecret := corev1.EnvVar{
				Name:  poolName + "_dbsecret",
				Value: r.CommonDecryptWithPrivKey3(ords, ords.Spec.PoolSettings[i].DBSecret.SecretName, ords.Spec.PoolSettings[i].DBSecret.PasswordKey, ctx),
			}

			envVarSecrets = append(envVarSecrets, dbSecret)

			if ords.Spec.PoolSettings[i].DBAdminUserSecret.SecretName != "" {
				autoUpgradeORDSEnv := corev1.EnvVar{
					Name:  poolName + "_autoupgrade_ords",
					Value: strconv.FormatBool(ords.Spec.PoolSettings[i].AutoUpgradeORDS),
				}
				autoUpgradeAPEXEnv := corev1.EnvVar{
					Name:  poolName + "_autoupgrade_apex",
					Value: strconv.FormatBool(ords.Spec.PoolSettings[i].AutoUpgradeAPEX),
				}

				dbAdminUserSecret := corev1.EnvVar{
					Name:  poolName + "_dbadminusersecret",
					Value: r.CommonDecryptWithPrivKey3(ords, ords.Spec.PoolSettings[i].DBAdminUserSecret.SecretName, ords.Spec.PoolSettings[i].DBAdminUserSecret.PasswordKey, ctx),
				}
				envVarSecrets = append(envVarSecrets, dbAdminUserSecret, autoUpgradeORDSEnv, autoUpgradeAPEXEnv)
			}

			if ords.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName != "" {

				dbCDBAdminUserSecret := corev1.EnvVar{
					Name:  poolName + "_dbcdbadminusersecret",
					Value: r.CommonDecryptWithPrivKey3(ords, ords.Spec.PoolSettings[i].DBCDBAdminUserSecret.SecretName, ords.Spec.PoolSettings[i].DBCDBAdminUserSecret.PasswordKey, ctx),
				}

				envVarSecrets = append(envVarSecrets, dbCDBAdminUserSecret)
			}
		}
	}

	return envVarSecrets
}

/*************************************************
 * Deletions
 **************************************************/
func (r *OrdsSrvsReconciler) ConfigMapDelete(ctx context.Context, req ctrl.Request, ords *dbapi.OrdsSrvs, definedPools map[string]bool) (err error) {
	// Delete Undefined Pool ConfigMaps
	configMapList := &corev1.ConfigMapList{}
	if err := r.List(ctx, configMapList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ords.Name}),
	); err != nil {
		return err
	}

	for _, configMap := range configMapList.Items {
		if configMap.Name == ords.Name+"-"+globalConfigMapName || configMap.Name == ords.Name+"-init-script" {
			continue
		}
		if _, exists := definedPools[configMap.Name]; !exists {
			if err := r.Delete(ctx, &configMap); err != nil {
				return err
			}
			RestartPods = ords.Spec.ForceRestart
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "ConfigMap %s Deleted", configMap.Name)
		}
	}

	return nil
}

func (r *OrdsSrvsReconciler) WorkloadDelete(ctx context.Context, req ctrl.Request, ords *dbapi.OrdsSrvs, kind string) (err error) {
	logr := log.FromContext(ctx).WithName("WorkloadDelete")

	// Get Workloads
	deploymentList := &appsv1.DeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ords.Name}),
	); err != nil {
		return err
	}

	statefulSetList := &appsv1.StatefulSetList{}
	if err := r.List(ctx, statefulSetList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ords.Name}),
	); err != nil {
		return err
	}

	daemonSetList := &appsv1.DaemonSetList{}
	if err := r.List(ctx, daemonSetList, client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{
			controllerLabelKey:           controllerLabelVal,
			"app.kubernetes.io/instance": ords.Name}),
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
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteDeployment := range deploymentList.Items {
			if err := r.Delete(ctx, &deleteDeployment); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	case "DaemonSet":
		for _, deleteDeployment := range deploymentList.Items {
			if err := r.Delete(ctx, &deleteDeployment); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteStatefulSet := range statefulSetList.Items {
			if err := r.Delete(ctx, &deleteStatefulSet); err != nil {
				return err
			}
			logr.Info("Deleted StatefulSet: " + deleteStatefulSet.Name)
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	default:
		for _, deleteStatefulSet := range statefulSetList.Items {
			if err := r.Delete(ctx, &deleteStatefulSet); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
		for _, deleteDaemonSet := range daemonSetList.Items {
			if err := r.Delete(ctx, &deleteDaemonSet); err != nil {
				return err
			}
			logr.Info("Deleted: " + kind)
			r.Recorder.Eventf(ords, corev1.EventTypeNormal, "Delete", "Workload %s Deleted", kind)
		}
	}
	return nil
}

/*************************************************
 * Helpers
 **************************************************/
func getLabels(name string) map[string]string {
	return map[string]string{
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

func CommonDecryptWithPrivKey(Key string, Buffer string) (string, error) {

	Debug := 0
	block, _ := pem.Decode([]byte(Key))
	pkcs8PrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fmt.Printf("Failed to parse private key %s \n", err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("======================================\n")
		fmt.Printf("%s\n", Key)
		fmt.Printf("======================================\n")
	}

	encString64, err := base64.StdEncoding.DecodeString(string(Buffer))
	if err != nil {
		fmt.Printf("Failed to decode encrypted string to base64: %s\n", err.Error())
		return "", err
	}

	if Debug == 1 {
		fmt.Printf("======================================\n")
		fmt.Printf("%s\n", encString64)
		fmt.Printf("======================================\n")
	}

	decryptedB, err := rsa.DecryptPKCS1v15(nil, pkcs8PrivateKey.(*rsa.PrivateKey), encString64)
	if err != nil {
		fmt.Printf("Failed to decrypt string %s\n", err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("[%s]\n", string(decryptedB))
	}
	return strings.TrimSpace(string(decryptedB)), err

}

func (r *OrdsSrvsReconciler) CommonDecryptWithPrivKey3(ords *dbapi.OrdsSrvs, sname string, skey string, ctx context.Context) string {
	logr := log.FromContext(ctx).WithName("CommonDecryptWithPrivKey2")
	secret_par := &corev1.Secret{}
	fmt.Printf("sname: %s\n", sname)
	fmt.Printf("skey: %s\n", skey)
	err := r.Get(ctx, types.NamespacedName{Name: sname, Namespace: ords.Namespace}, secret_par)
	if err != nil {
		logr.Error(err, "Cannot read secret"+sname)
		return ""
	}
	encVal := string(secret_par.Data[skey])
	encVal = strings.TrimSpace(encVal)

	secret_key := &corev1.Secret{}
	/* get private key */
	if err := r.Get(ctx, types.NamespacedName{Name: ords.Spec.EncPrivKey.SecretName,
		Namespace: ords.Namespace}, secret_key); err != nil {
		logr.Error(err, "Cannot get privte key")
		return ""
	}
	PrvKeyVal := string(secret_key.Data[ords.Spec.EncPrivKey.PasswordKey])
	PrvKeyVal = strings.TrimSpace(PrvKeyVal)

	decVal, err := CommonDecryptWithPrivKey(PrvKeyVal, encVal)
	if err != nil {
		logr.Error(err, "Fail to decrypt secret")
		return ""
	}

	logr.Info("Password decryption completed")

	return decVal
}
