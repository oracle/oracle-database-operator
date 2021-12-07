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
	"errors"
	//"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
)

// CDBReconciler reconciles a CDB object
type CDBReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Log      logr.Logger
	Interval time.Duration
	Recorder record.EventRecorder
}

var (
	cdbPhaseInit    = "Initializing"
	cdbPhasePod     = "CreatingPod"
	cdbPhaseValPod  = "ValidatingPods"
	cdbPhaseService = "CreatingService"
	cdbPhaseSecrets = "DeletingSecrets"
	cdbPhaseReady   = "Ready"
	cdbPhaseDelete  = "Deleting"
)

const CDBFinalizer = "database.oracle.com/CDBfinalizer"

//+kubebuilder:rbac:groups=database.oracle.com,resources=cdbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=cdbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=cdbs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;services;configmaps;events;replicasets,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=core,resources=pods;secrets;services;configmaps;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *CDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("onpremdboperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	requeueN := ctrl.Result{}

	var err error
	cdb := &dbapi.CDB{}

	// Execute for every reconcile
	defer func() {
		//log.Info("DEFER", "Name", cdb.Name, "Phase", cdb.Status.Phase, "Status", strconv.FormatBool(cdb.Status.Status))
		if !cdb.Status.Status {
			if err := r.Status().Update(ctx, cdb); err != nil {
				log.Error(err, "Failed to update status for :"+cdb.Name, "err", err.Error())
			}
		}
	}()

	err = r.Client.Get(context.TODO(), req.NamespacedName, cdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("CDB Resource Not found", "Name", cdb.Name)
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			cdb.Status.Status = true
			return requeueN, nil
		}
		// Error reading the object - requeue the req.
		return requeueY, err
	}

	//log.Info("Res Status:", "Name", cdb.Name, "Phase", cdb.Status.Phase, "Status", strconv.FormatBool(cdb.Status.Status))

	// Finalizer section
	err = r.manageCDBDeletion(ctx, req, cdb)
	if err != nil {
		log.Info("Reconcile queued")
		return requeueY, nil
	}

	// If post-creation, CDB spec is changed, check and take appropriate action
	if (cdb.Status.Phase == cdbPhaseReady) && cdb.Status.Status {
		r.evaluateSpecChange(ctx, req, cdb)
	}

	if !cdb.Status.Status {
		phase := cdb.Status.Phase
		log.Info("Current Phase:"+phase, "Name", cdb.Name)

		switch phase {
		case cdbPhaseInit:
			cdb.Status.Phase = cdbPhasePod
		case cdbPhasePod:
			// Create ORDS POD
			//err = r.createORDSPod(ctx, req, cdb)
			err = r.createORDSInstances(ctx, req, cdb)
			if err != nil {
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			cdb.Status.Phase = cdbPhaseValPod
		case cdbPhaseValPod:
			// Validate ORDS PODs
			err = r.validateORDSPods(ctx, req, cdb)
			if err != nil {
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			cdb.Status.Phase = cdbPhaseService
		case cdbPhaseService:
			// Create ORDS Service
			err = r.createORDSSVC(ctx, req, cdb)
			if err != nil {
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			//cdb.Status.Phase = cdbPhaseSecrets
			cdb.Status.Phase = cdbPhaseReady
		case cdbPhaseSecrets:
			// Delete CDB Secrets
			//r.deleteSecrets(ctx, req, cdb)
			cdb.Status.Phase = cdbPhaseReady
			cdb.Status.Msg = "Success"
		case cdbPhaseReady:
			cdb.Status.Status = true
			r.Status().Update(ctx, cdb)
			return requeueN, nil
		default:
			cdb.Status.Phase = cdbPhaseInit
			log.Info("DEFAULT:", "Name", cdb.Name, "Phase", phase, "Status", strconv.FormatBool(cdb.Status.Status))
		}

		if err := r.Status().Update(ctx, cdb); err != nil {
			log.Error(err, "Failed to update status for :"+cdb.Name, "err", err.Error())
		}
		return requeueY, nil
	}

	log.Info("Reconcile completed")
	return requeueN, nil
}

/**********************************************************
 * Create a ReplicaSet for pods based on the ORDS container
 /********************************************************/
func (r *CDBReconciler) createORDSInstances(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {

	log := r.Log.WithValues("createORDSInstances", req.NamespacedName)

	replicaSet := r.createReplicaSetSpec(cdb)

	foundRS := &appsv1.ReplicaSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: replicaSet.Name, Namespace: cdb.Namespace}, foundRS)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Creating ORDS Replicaset: " + replicaSet.Name)
		err = r.Create(ctx, replicaSet)
		if err != nil {
			log.Error(err, "Failed to create ReplicaSet for :"+cdb.Name, "Namespace", replicaSet.Namespace, "Name", replicaSet.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Replicaset : "+replicaSet.Name+" already exists.")
		return err
	}

	// Set CDB instance as the owner and controller
	ctrl.SetControllerReference(cdb, replicaSet, r.Scheme)

	log.Info("Created ORDS ReplicaSet successfully")
	r.Recorder.Eventf(cdb, corev1.EventTypeNormal, "CreatedORDSReplicaSet", "Created ORDS Replicaset (Replicas - %s) for %s", strconv.Itoa(cdb.Spec.Replicas), cdb.Name)
	return nil
}

/*************************************************
 * Validate ORDS Pod. Check if there are any errors
 /************************************************/
func (r *CDBReconciler) validateORDSPods(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {

	log := r.Log.WithValues("validateORDSPod", req.NamespacedName)

	log.Info("Validating Pod creation for :" + cdb.Name)

	podName := cdb.Name + "-ords"
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{client.InNamespace(req.Namespace), client.MatchingLabels{"name": podName}}

	// List retrieves list of objects for a given namespace and list options.
	err := r.List(ctx, podList, listOpts...)
	if err != nil {
		log.Info("Failed to list pods of: "+podName, "Namespace", req.Namespace)
		return err
	}

	if len(podList.Items) == 0 {
		log.Info("No pods found for: "+podName, "Namespace", req.Namespace)
		cdb.Status.Msg = "Waiting for ORDS Pod(s) to start"
		return errors.New("Waiting for ORDS pods to start")
	}

	getORDSStatus := " curl -sSkv -k -X GET https://localhost:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/stable/metadata-catalog/ || curl  -sSkv -X GET http://localhost:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/stable/metadata-catalog/ "
	readyPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Get ORDS Status
			out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", getORDSStatus)
			if strings.Contains(out, "HTTP/1.1 200 OK") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 200 OK") {
				readyPods++
			}
		}
	}

	if readyPods != cdb.Spec.Replicas {
		log.Info("Replicas: "+strconv.Itoa(cdb.Spec.Replicas), "Ready Pods: ", readyPods)
		cdb.Status.Msg = "Waiting for ORDS Pod(s) to be ready"
		return errors.New("Waiting for ORDS pods to be ready")
	}

	cdb.Status.Msg = ""
	return nil
}

/************************
 * Create Pod spec
/************************/
func (r *CDBReconciler) createPodSpec(cdb *dbapi.CDB) corev1.PodSpec {

	podSpec := corev1.PodSpec{
		Volumes: []corev1.Volume{{
			Name: "secrets",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					DefaultMode: func() *int32 { i := int32(0666); return &i }(),
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.SysAdminPwd.Secret.SecretName,
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.CDBAdminUser.Secret.SecretName,
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.CDBAdminPwd.Secret.SecretName,
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.ORDSPwd.Secret.SecretName,
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.WebServerUser.Secret.SecretName,
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cdb.Spec.WebServerPwd.Secret.SecretName,
								},
							},
						},
					},
				},
			},
		}},
		Containers: []corev1.Container{{
			Name:  cdb.Name + "-ords",
			Image: cdb.Spec.ORDSImage,
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "/opt/oracle/ords/secrets",
				Name:      "secrets",
				ReadOnly:  true,
			}},
			Env: func() []corev1.EnvVar {
				return []corev1.EnvVar{
					{
						Name:  "ORACLE_HOST",
						Value: cdb.Spec.DBServer,
					},
					{
						Name:  "ORACLE_PORT",
						Value: strconv.Itoa(cdb.Spec.DBPort),
					},
					{
						Name:  "ORDS_PORT",
						Value: strconv.Itoa(cdb.Spec.ORDSPort),
					},
					{
						Name:  "ORACLE_SERVICE",
						Value: cdb.Spec.ServiceName,
					},
					{
						Name:  "ORACLE_PWD_KEY",
						Value: cdb.Spec.SysAdminPwd.Secret.Key,
					},
					{
						Name:  "CDBADMIN_USER_KEY",
						Value: cdb.Spec.CDBAdminUser.Secret.Key,
					},
					{
						Name:  "CDBADMIN_PWD_KEY",
						Value: cdb.Spec.CDBAdminPwd.Secret.Key,
					},
					{
						Name:  "ORDS_PWD_KEY",
						Value: cdb.Spec.ORDSPwd.Secret.Key,
					},
					{
						Name:  "WEBSERVER_USER_KEY",
						Value: cdb.Spec.WebServerUser.Secret.Key,
					},
					{
						Name:  "WEBSERVER_PASSWORD_KEY",
						Value: cdb.Spec.WebServerPwd.Secret.Key,
					},
				}
			}(),
		}},

		NodeSelector: func() map[string]string {
			ns := make(map[string]string)
			if len(cdb.Spec.NodeSelector) != 0 {
				for key, value := range cdb.Spec.NodeSelector {
					ns[key] = value
				}
			}
			return ns
		}(),
	}

	if len(cdb.Spec.ORDSImagePullSecret) > 0 {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: cdb.Spec.ORDSImagePullSecret,
			},
		}
	}

	podSpec.Containers[0].ImagePullPolicy = corev1.PullAlways

	if len(cdb.Spec.ORDSImagePullPolicy) > 0 {
		if strings.ToUpper(cdb.Spec.ORDSImagePullPolicy) == "NEVER" {
			podSpec.Containers[0].ImagePullPolicy = corev1.PullNever
		}
	}

	return podSpec
}

/************************
 * Create ReplicaSet spec
/************************/
func (r *CDBReconciler) createReplicaSetSpec(cdb *dbapi.CDB) *appsv1.ReplicaSet {

	replicas := int32(cdb.Spec.Replicas)
	podSpec := r.createPodSpec(cdb)

	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdb.Name + "-ords-rs",
			Namespace: cdb.Namespace,
			Labels: map[string]string{
				"name": cdb.Name + "-ords-rs",
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cdb.Name + "-ords",
					Namespace: cdb.Namespace,
					Labels: map[string]string{
						"name": cdb.Name + "-ords",
					},
				},
				Spec: podSpec,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": cdb.Name + "-ords",
				},
			},
		},
	}

	return replicaSet
}

/**********************************************************
 * Evaluate change in Spec post creation and instantiation
 /********************************************************/
func (r *CDBReconciler) evaluateSpecChange(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {
	log := r.Log.WithValues("evaluateSpecChange", req.NamespacedName)

	// List the Pods matching the PodTemplate Labels
	podName := cdb.Name + "-ords"
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{client.InNamespace(req.Namespace), client.MatchingLabels{"name": podName}}

	// List retrieves list of objects for a given namespace and list options.
	err := r.List(ctx, podList, listOpts...)
	if err != nil {
		log.Info("Failed to list pods of: "+podName, "Namespace", req.Namespace)
		return err
	}

	var foundPod corev1.Pod
	for _, pod := range podList.Items {
		foundPod = pod
		break
	}

	ordsSpecChange := false
	for _, envVar := range foundPod.Spec.Containers[0].Env {
		if envVar.Name == "ORACLE_HOST" && envVar.Value != cdb.Spec.DBServer {
			ordsSpecChange = true
		} else if envVar.Name == "ORACLE_PORT" && envVar.Value != strconv.Itoa(cdb.Spec.DBPort) {
			ordsSpecChange = true
		} else if envVar.Name == "ORDS_PORT" && envVar.Value != strconv.Itoa(cdb.Spec.ORDSPort) {
			ordsSpecChange = true
		}
	}

	if ordsSpecChange {
		// Delete existing ReplicaSet
		k_client, err := kubernetes.NewForConfig(r.Config)
		if err != nil {
			log.Error(err, "Kubernetes Config Error")
			return err
		}
		replicaSetName := cdb.Name + "-ords-rs"
		err = k_client.AppsV1().ReplicaSets(cdb.Namespace).Delete(context.TODO(), replicaSetName, metav1.DeleteOptions{})
		if err != nil {
			log.Info("Could not delete ReplicaSet", "RS Name", replicaSetName, "err", err.Error())
			if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
				return err
			}
		} else {
			log.Info("Successfully deleted ORDS ReplicaSet", "RS Name", replicaSetName)
		}

		// Create new ReplicaSet
		replicaSet := r.createReplicaSetSpec(cdb)
		log.Info("Re-Creating ORDS Replicaset: " + replicaSet.Name)
		err = r.Create(ctx, replicaSet)
		if err != nil {
			log.Error(err, "Failed to re-create ReplicaSet for :"+cdb.Name, "Namespace", replicaSet.Namespace, "Name", replicaSet.Name)
			return err
		}
		// Set CDB instance as the owner and controller
		ctrl.SetControllerReference(cdb, replicaSet, r.Scheme)
		log.Info("Successfully re-created ORDS ReplicaSet", "RS Name", replicaSetName)

		cdb.Status.Phase = cdbPhaseValPod
		cdb.Status.Status = false
		r.Status().Update(ctx, cdb)
	} else {
		// If only the value of replicas is changed, update the RS only
		replicaSetName := cdb.Name + "-ords-rs"

		foundRS := &appsv1.ReplicaSet{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: replicaSetName, Namespace: cdb.Namespace}, foundRS)
		if err != nil {
			log.Error(err, "Unable to get ORDS Replicaset: "+replicaSetName)
			return err
		}

		// Check if replicas have changed
		replicas := int32(cdb.Spec.Replicas)
		if cdb.Spec.Replicas != int(*(foundRS.Spec.Replicas)) {
			log.Info("Existing Replicas: " + strconv.Itoa(int(*(foundRS.Spec.Replicas))) + ", New Replicas: " + strconv.Itoa(cdb.Spec.Replicas))
			foundRS.Spec.Replicas = &replicas
			err = r.Update(ctx, foundRS)
			if err != nil {
				log.Error(err, "Failed to update ReplicaSet for :"+cdb.Name, "Namespace", cdb.Namespace, "Name", replicaSetName)
				return err
			}
			cdb.Status.Phase = cdbPhaseValPod
			cdb.Status.Status = false
			r.Status().Update(ctx, cdb)
		}
	}

	return nil
}

/*************************************************
 * Create a Cluster Service for ORDS CDB Pod
 /************************************************/
func (r *CDBReconciler) createORDSSVC(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {

	log := r.Log.WithValues("createORDSSVC", req.NamespacedName)

	foundSvc := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: cdb.Name + "-ords", Namespace: cdb.Namespace}, foundSvc)
	if err != nil && apierrors.IsNotFound(err) {
		svc := r.createSvcSpec(cdb)

		log.Info("Creating a new Cluster Service for: "+cdb.Name, "Svc.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err := r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Cluster Service for: "+cdb.Name, "Svc.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return err
		}

		log.Info("Created ORDS Cluster Service successfully")
		r.Recorder.Eventf(cdb, corev1.EventTypeNormal, "CreatedORDSService", "Created ORDS Service for %s", cdb.Name)
	} else {
		log.Info("ORDS Cluster Service already exists")
	}

	return nil
}

/************************
 * Create Service spec
 /************************/
func (r *CDBReconciler) createSvcSpec(cdb *dbapi.CDB) *corev1.Service {

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdb.Name + "-ords",
			Namespace: cdb.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": cdb.Name + "-ords",
			},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	// Set CDB instance as the owner and controller
	ctrl.SetControllerReference(cdb, svc, r.Scheme)
	return svc
}

/*************************************************
 *   Check CDB deletion
 /************************************************/
func (r *CDBReconciler) manageCDBDeletion(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {
	log := r.Log.WithValues("manageCDBDeletion", req.NamespacedName)

	// Check if the PDB instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isCDBMarkedToBeDeleted := cdb.GetDeletionTimestamp() != nil
	if isCDBMarkedToBeDeleted {
		log.Info("Marked to be deleted")
		cdb.Status.Phase = cdbPhaseDelete
		cdb.Status.Status = true
		r.Status().Update(ctx, cdb)
		if controllerutil.ContainsFinalizer(cdb, CDBFinalizer) {
			// Run finalization logic for CDBFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			err := r.deleteCDBInstance(ctx, req, cdb)
			if err != nil {
				log.Info("Could not delete CDB Resource", "CDB Name", cdb.Spec.CDBName, "err", err.Error())
				return err
			}

			// Remove CDBFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			log.Info("Removing finalizer")
			controllerutil.RemoveFinalizer(cdb, CDBFinalizer)
			err = r.Update(ctx, cdb)
			if err != nil {
				log.Info("Could not remove finalizer", "err", err.Error())
				return err
			}

			log.Info("Successfully removed CDB Resource")
			return nil
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(cdb, CDBFinalizer) {
		log.Info("Adding finalizer")

		cdb.Status.Phase = cdbPhaseInit
		cdb.Status.Status = false
		controllerutil.AddFinalizer(cdb, CDBFinalizer)
		err := r.Update(ctx, cdb)
		if err != nil {
			log.Info("Could not add finalizer", "err", err.Error())
			return err
		}
	}
	return nil
}

/*************************************************
 * Delete CDB Resource
/************************************************/
func (r *CDBReconciler) deleteCDBInstance(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) error {

	log := r.Log.WithValues("deleteCDBInstance", req.NamespacedName)

	k_client, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		log.Error(err, "Kubernetes Config Error")
	}

	replicaSetName := cdb.Name + "-ords-rs"

	err = k_client.AppsV1().ReplicaSets(cdb.Namespace).Delete(context.TODO(), replicaSetName, metav1.DeleteOptions{})
	if err != nil {
		log.Info("Could not delete ReplicaSet", "RS Name", replicaSetName, "err", err.Error())
		if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
			return err
		}
	} else {
		log.Info("Successfully deleted ORDS ReplicaSet", "RS Name", replicaSetName)
	}

	r.Recorder.Eventf(cdb, corev1.EventTypeNormal, "DeletedORDSReplicaSet", "Deleted ORDS ReplicaSet for %s", cdb.Name)

	svcName := cdb.Name + "-ords"

	err = k_client.CoreV1().Services(cdb.Namespace).Delete(context.TODO(), svcName, metav1.DeleteOptions{})
	if err != nil {
		log.Info("Could not delete Service", "Service Name", svcName, "err", err.Error())
		if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
			return err
		}
	} else {
		r.Recorder.Eventf(cdb, corev1.EventTypeNormal, "DeletedORDSService", "Deleted ORDS Service for %s", cdb.Name)
		log.Info("Successfully deleted ORDS Service", "Service Name", svcName)
	}

	log.Info("Successfully deleted CDB resource", "CDB Name", cdb.Spec.CDBName)
	return nil
}

/*************************************************
 * Delete Secrets
 /************************************************/
func (r *CDBReconciler) deleteSecrets(ctx context.Context, req ctrl.Request, cdb *dbapi.CDB) {

	log := r.Log.WithValues("deleteSecrets", req.NamespacedName)

	log.Info("Deleting CDB secrets")
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cdb.Spec.SysAdminPwd.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.SysAdminPwd.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.CDBAdminUser.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.CDBAdminUser.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.CDBAdminPwd.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.CDBAdminPwd.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.ORDSPwd.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.ORDSPwd.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.WebServerUser.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.WebServerUser.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.WebServerPwd.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + cdb.Spec.WebServerPwd.Secret.SecretName)
		}
	}
}

/**************************************************************
 * SetupWithManager sets up the controller with the Manager.
 /*************************************************************/
func (r *CDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.CDB{}).
		Owns(&appsv1.ReplicaSet{}). //Watch for deleted RS owned by this controller
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Ignore updates to CR status in which case metadata.Generation does not change
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				//return !e.DeleteStateUnknown
				return false
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		Complete(r)
}
