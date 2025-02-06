/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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
	"fmt"

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

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	//lrcommons "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"
)

// LRESTReconciler reconciles a LREST object
type LRESTReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Log      logr.Logger
	Interval time.Duration
	Recorder record.EventRecorder
}

var (
	lrestPhaseInit    = "Initializing"
	lrestPhasePod     = "CreatingPod"
	lrestPhaseValPod  = "ValidatingPods"
	lrestPhaseService = "CreatingService"
	lrestPhaseSecrets = "DeletingSecrets"
	lrestPhaseReady   = "Ready"
	lrestPhaseDelete  = "Deleting"
	lrestPhaseFail    = "Failed"
)

const LRESTFinalizer = "database.oracle.com/LRESTfinalizer"

//+kubebuilder:rbac:groups=database.oracle.com,resources=lrests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrests/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;services;configmaps;events;replicasets,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=core,resources=pods;secrets;services;configmaps;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LREST object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *LRESTReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("multitenantoperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	requeueN := ctrl.Result{}

	var err error
	lrest := &dbapi.LREST{}

	// Execute for every reconcile
	defer func() {
		log.Info("DEFER", "Name", lrest.Name, "Phase", lrest.Status.Phase, "Status", strconv.FormatBool(lrest.Status.Status))
		if !lrest.Status.Status {
			if err := r.Status().Update(ctx, lrest); err != nil {
				log.Error(err, "Failed to update status for :"+lrest.Name, "err", err.Error())
			}
		}
	}()

	err = r.Client.Get(context.TODO(), req.NamespacedName, lrest)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LREST Resource Not found", "Name", lrest.Name)
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			lrest.Status.Status = true
			return requeueN, nil
		}
		// Error reading the object - requeue the req.
		return requeueY, err
	}

	log.Info("Res Status:", "Name", lrest.Name, "Phase", lrest.Status.Phase, "Status", strconv.FormatBool(lrest.Status.Status))

	// Finalizer section
	err = r.manageLRESTDeletion(ctx, req, lrest)
	if err != nil {
		log.Info("Reconcile queued")
		return requeueY, nil
	}

	// If post-creation, LREST spec is changed, check and take appropriate action
	if (lrest.Status.Phase == lrestPhaseReady) && lrest.Status.Status {
		r.evaluateSpecChange(ctx, req, lrest)
	}

	if !lrest.Status.Status {
		phase := lrest.Status.Phase
		log.Info("Current Phase:"+phase, "Name", lrest.Name)

		switch phase {
		case lrestPhaseInit:
			err = r.verifySecrets(ctx, req, lrest)
			if err != nil {
				lrest.Status.Phase = lrestPhaseFail
				return requeueN, nil
			}
			lrest.Status.Phase = lrestPhasePod
		case lrestPhasePod:
			// Create LREST PODs
			err = r.createLRESTInstances(ctx, req, lrest)
			if err != nil {
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			lrest.Status.Phase = lrestPhaseValPod
		case lrestPhaseValPod:
			// Validate LREST PODs
			err = r.validateLRESTPods(ctx, req, lrest)
			if err != nil {
				if lrest.Status.Phase == lrestPhaseFail {
					return requeueN, nil
				}
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			lrest.Status.Phase = lrestPhaseService
		case lrestPhaseService:
			// Create LREST Service
			err = r.createLRESTSVC(ctx, req, lrest)
			if err != nil {
				log.Info("Reconcile queued")
				return requeueY, nil
			}
			//lrest.Status.Phase = lrestPhaseSecrets
			lrest.Status.Phase = lrestPhaseReady
		case lrestPhaseSecrets:
			// Delete LREST Secrets
			//r.deleteSecrets(ctx, req, lrest)
			lrest.Status.Phase = lrestPhaseReady
			lrest.Status.Msg = "Success"
		case lrestPhaseReady:
			lrest.Status.Status = true
			r.Status().Update(ctx, lrest)
			return requeueN, nil
		default:
			lrest.Status.Phase = lrestPhaseInit
			log.Info("DEFAULT:", "Name", lrest.Name, "Phase", phase, "Status", strconv.FormatBool(lrest.Status.Status))
		}

		if err := r.Status().Update(ctx, lrest); err != nil {
			log.Error(err, "Failed to update status for :"+lrest.Name, "err", err.Error())
		}
		return requeueY, nil
	}

	log.Info("Reconcile completed")
	return requeueN, nil
}

/*
*********************************************************
  - Create a ReplicaSet for pods based on the LREST container
    /*******************************************************
*/
func (r *LRESTReconciler) createLRESTInstances(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {

	log := r.Log.WithValues("createLRESTInstances", req.NamespacedName)

	replicaSet := r.createReplicaSetSpec(lrest)

	foundRS := &appsv1.ReplicaSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: replicaSet.Name, Namespace: lrest.Namespace}, foundRS)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Creating LREST Replicaset: " + replicaSet.Name)
		err = r.Create(ctx, replicaSet)
		if err != nil {
			log.Error(err, "Failed to create ReplicaSet for :"+lrest.Name, "Namespace", replicaSet.Namespace, "Name", replicaSet.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Replicaset : "+replicaSet.Name+" already exists.")
		return err
	}

	// Set LREST instance as the owner and controller
	ctrl.SetControllerReference(lrest, replicaSet, r.Scheme)

	log.Info("Created LREST ReplicaSet successfully")
	r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "CreatedLRESTReplicaSet", "Created LREST Replicaset (Replicas - %s) for %s", strconv.Itoa(lrest.Spec.Replicas), lrest.Name)
	return nil
}

/*
************************************************
  - Validate LREST Pod. Check if there are any errors
    /***********************************************
*/
func (r *LRESTReconciler) validateLRESTPods(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {

	log := r.Log.WithValues("validateLRESTPod", req.NamespacedName)

	log.Info("Validating Pod creation for :" + lrest.Name)

	podName := lrest.Name + "-lrest"
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
		lrest.Status.Msg = "Waiting for LREST Pod(s) to start"
		return errors.New("Waiting for LREST pods to start")
	}

	getLRESTStatus := " curl  --cert /opt/oracle/lrest/certificates/tls.crt --cacert /opt/oracle/lrest/certificates/ca.crt --key  /opt/oracle/lrest/certificates/tls.key -u `cat /opt/oracle/lrest/certificates/webserver_user`:`cat /opt/oracle/lrest/certificates/webserver_pwd` -sSkv -k -X GET https://localhost:" + strconv.Itoa(lrest.Spec.LRESTPort) + "/database/pdbs/"
	readyPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Get LREST Status
			out, err := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", getLRESTStatus)
			if strings.Contains(out, "HTTP/1.1 200 OK") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 200 OK") ||
				strings.Contains(out, "HTTP/2") || strings.Contains(strings.ToUpper(err.Error()), " HTTP/2") {
				readyPods++
			} else if strings.Contains(out, "HTTP/1.1 404 Not Found") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 404 NOT FOUND") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/2 404") || strings.Contains(strings.ToUpper(err.Error()), "Failed to connect to localhost") {
				// Check if DB connection parameters are correct
				getLRESTInstallStatus := " grep -q 'Failed to' /tmp/lrest_install.log; echo $?;"
				out, _ := dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "", ctx, req, false, "bash", "-c", getLRESTInstallStatus)
				if strings.TrimSpace(out) == "0" {
					lrest.Status.Msg = "Check DB connection parameters"
					lrest.Status.Phase = lrestPhaseFail
					// Delete existing ReplicaSet
					r.deleteReplicaSet(ctx, req, lrest)
					return errors.New("Check DB connection parameters")
				}
			}
		}
	}

	if readyPods != lrest.Spec.Replicas {
		log.Info("Replicas: "+strconv.Itoa(lrest.Spec.Replicas), "Ready Pods: ", readyPods)
		lrest.Status.Msg = "Waiting for LREST Pod(s) to be ready"
		return errors.New("Waiting for LREST pods to be ready")
	}

	lrest.Status.Msg = ""
	return nil
}

/*
***********************
  - Create Pod spec

/***********************
*/
func (r *LRESTReconciler) createPodSpec(lrest *dbapi.LREST) corev1.PodSpec {

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
									Name: lrest.Spec.LRESTPubKey.Secret.SecretName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  lrest.Spec.LRESTPubKey.Secret.Key,
										Path: lrest.Spec.LRESTPubKey.Secret.Key,
									},
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.LRESTPriKey.Secret.SecretName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  lrest.Spec.LRESTPriKey.Secret.Key,
										Path: lrest.Spec.LRESTPriKey.Secret.Key,
									},
								},
							},
						},

						/***/
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.LRESTTlsKey.Secret.SecretName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  lrest.Spec.LRESTTlsKey.Secret.Key,
										Path: lrest.Spec.LRESTTlsKey.Secret.Key,
									},
								},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.LRESTTlsCrt.Secret.SecretName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  lrest.Spec.LRESTTlsCrt.Secret.Key,
										Path: lrest.Spec.LRESTTlsCrt.Secret.Key,
									},
								},
							},
						},
					},
				},
			},
		}},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &[]bool{true}[0],
			FSGroup:      &[]int64{54321}[0],
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		/*InitContainers: []corev1.Container{{
			Image:           lrest.Spec.LRESTImage,
			Name:            lrest.Name + "-init",
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: securityContextDefineLrest(),
			Command:         []string{"echo test > /opt/oracle/lrest/certificates/tests"},
			Env: func() []corev1.EnvVar {
				return []corev1.EnvVar{
					{
						Name:  "ORACLE_HOST",
						Value: lrest.Spec.DBTnsurl,
					}}
			}(),
			VolumeMounts: []corev1.VolumeMount{
				{
					MountPath: "/opt/oracle/lrest/certificates",
					Name:      "secrets",
					ReadOnly:  false,
				}},
		}},*/
		Containers: []corev1.Container{{
			Image:           lrest.Spec.LRESTImage,
			Name:            lrest.Name + "-lrest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: securityContextDefineLrest(),
			VolumeMounts: []corev1.VolumeMount{
				{
					MountPath: "/opt/oracle/lrest/certificates",
					Name:      "secrets",
					ReadOnly:  true,
				},
			},
			Env: func() []corev1.EnvVar {
				return []corev1.EnvVar{
					{
						Name:  "ORACLE_HOST",
						Value: lrest.Spec.DBServer,
					},
					{
						Name:  "DBTNSURL",
						Value: lrest.Spec.DBTnsurl,
					},
					{
						Name:  "TLSCRT",
						Value: lrest.Spec.LRESTTlsCrt.Secret.Key,
					},
					{
						Name:  "TLSKEY",
						Value: lrest.Spec.LRESTTlsKey.Secret.Key,
					},
					{
						Name:  "PUBKEY",
						Value: lrest.Spec.LRESTPubKey.Secret.Key,
					},
					{
						Name:  "PRVKEY",
						Value: lrest.Spec.LRESTPriKey.Secret.Key,
					},
					{
						Name:  "ORACLE_PORT",
						Value: strconv.Itoa(lrest.Spec.DBPort),
					},
					{
						Name:  "LREST_PORT",
						Value: strconv.Itoa(lrest.Spec.LRESTPort),
					},
					{
						Name:  "ORACLE_SERVICE",
						Value: lrest.Spec.ServiceName,
					},
					{
						Name: "R1",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.LRESTAdminUser.Secret.SecretName,
								},
								Key: lrest.Spec.LRESTAdminUser.Secret.Key,
							},
						},
					},
					{
						Name: "R2",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.LRESTAdminPwd.Secret.SecretName,
								},
								Key: lrest.Spec.LRESTAdminPwd.Secret.Key,
							},
						},
					},
					{
						Name: "R3",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.WebLrestServerUser.Secret.SecretName,
								},
								Key: lrest.Spec.WebLrestServerUser.Secret.Key,
							},
						},
					},
					{
						Name: "R4",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: lrest.Spec.WebLrestServerPwd.Secret.SecretName,
								},
								Key: lrest.Spec.WebLrestServerPwd.Secret.Key,
							},
						},
					},
				}
			}(),
		}},

		NodeSelector: func() map[string]string {
			ns := make(map[string]string)
			if len(lrest.Spec.NodeSelector) != 0 {
				for key, value := range lrest.Spec.NodeSelector {
					ns[key] = value
				}
			}
			return ns
		}(),
	}

	if len(lrest.Spec.LRESTImagePullSecret) > 0 {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: lrest.Spec.LRESTImagePullSecret,
			},
		}
	}

	podSpec.Containers[0].ImagePullPolicy = corev1.PullAlways

	if len(lrest.Spec.LRESTImagePullPolicy) > 0 {
		if strings.ToUpper(lrest.Spec.LRESTImagePullPolicy) == "NEVER" {
			podSpec.Containers[0].ImagePullPolicy = corev1.PullNever
		}
	}

	return podSpec
}

/*
***********************
  - Create ReplicaSet spec

/***********************
*/
func (r *LRESTReconciler) createReplicaSetSpec(lrest *dbapi.LREST) *appsv1.ReplicaSet {

	replicas := int32(lrest.Spec.Replicas)
	podSpec := r.createPodSpec(lrest)

	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lrest.Name + "-lrest-rs",
			Namespace: lrest.Namespace,
			Labels: map[string]string{
				"name": lrest.Name + "-lrest-rs",
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      lrest.Name + "-lrest",
					Namespace: lrest.Namespace,
					Labels: map[string]string{
						"name": lrest.Name + "-lrest",
					},
				},
				Spec: podSpec,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": lrest.Name + "-lrest",
				},
			},
		},
	}

	return replicaSet
}

/*
*********************************************************
  - Evaluate change in Spec post creation and instantiation
    /*******************************************************
*/
func (r *LRESTReconciler) deleteReplicaSet(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {
	log := r.Log.WithValues("deleteReplicaSet", req.NamespacedName)

	k_client, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		log.Error(err, "Kubernetes Config Error")
		return err
	}

	replicaSetName := lrest.Name + "-lrest-rs"
	err = k_client.AppsV1().ReplicaSets(lrest.Namespace).Delete(context.TODO(), replicaSetName, metav1.DeleteOptions{})
	if err != nil {
		log.Info("Could not delete ReplicaSet", "RS Name", replicaSetName, "err", err.Error())
		if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
			return err
		}
	} else {
		log.Info("Successfully deleted LREST ReplicaSet", "RS Name", replicaSetName)
	}

	return nil
}

/*
*********************************************************
  - Evaluate change in Spec post creation and instantiation
    /*******************************************************
*/
func (r *LRESTReconciler) evaluateSpecChange(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {
	log := r.Log.WithValues("evaluateSpecChange", req.NamespacedName)

	// List the Pods matching the PodTemplate Labels
	podName := lrest.Name + "-lrest"
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

	lrestSpecChange := false
	for _, envVar := range foundPod.Spec.Containers[0].Env {
		if envVar.Name == "ORACLE_HOST" && envVar.Value != lrest.Spec.DBServer {
			lrestSpecChange = true
		} else if envVar.Name == "ORACLE_PORT" && envVar.Value != strconv.Itoa(lrest.Spec.DBPort) {
			lrestSpecChange = true
		} else if envVar.Name == "LREST_PORT" && envVar.Value != strconv.Itoa(lrest.Spec.LRESTPort) {
			lrestSpecChange = true
		} else if envVar.Name == "ORACLE_SERVICE" && envVar.Value != lrest.Spec.ServiceName {
			lrestSpecChange = true
		}
	}

	if lrestSpecChange {
		// Delete existing ReplicaSet
		err = r.deleteReplicaSet(ctx, req, lrest)
		if err != nil {
			return err
		}

		lrest.Status.Phase = lrestPhaseInit
		lrest.Status.Status = false
		r.Status().Update(ctx, lrest)
	} else {
		// Update the RS if the value of "replicas" is changed
		replicaSetName := lrest.Name + "-lrest-rs"

		foundRS := &appsv1.ReplicaSet{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: replicaSetName, Namespace: lrest.Namespace}, foundRS)
		if err != nil {
			log.Error(err, "Unable to get LREST Replicaset: "+replicaSetName)
			return err
		}

		// Check if number of replicas have changed
		replicas := int32(lrest.Spec.Replicas)
		if lrest.Spec.Replicas != int(*(foundRS.Spec.Replicas)) {
			log.Info("Existing Replicas: " + strconv.Itoa(int(*(foundRS.Spec.Replicas))) + ", New Replicas: " + strconv.Itoa(lrest.Spec.Replicas))
			foundRS.Spec.Replicas = &replicas
			err = r.Update(ctx, foundRS)
			if err != nil {
				log.Error(err, "Failed to update ReplicaSet for :"+lrest.Name, "Namespace", lrest.Namespace, "Name", replicaSetName)
				return err
			}
			lrest.Status.Phase = lrestPhaseValPod
			lrest.Status.Status = false
			r.Status().Update(ctx, lrest)
		}
	}

	return nil
}

/*
************************************************
  - Create a Cluster Service for LREST LREST Pod
    /***********************************************
*/
func (r *LRESTReconciler) createLRESTSVC(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {

	log := r.Log.WithValues("createLRESTSVC", req.NamespacedName)

	foundSvc := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: lrest.Name + "-lrest", Namespace: lrest.Namespace}, foundSvc)
	if err != nil && apierrors.IsNotFound(err) {
		svc := r.createSvcSpec(lrest)

		log.Info("Creating a new Cluster Service for: "+lrest.Name, "Svc.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err := r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Cluster Service for: "+lrest.Name, "Svc.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return err
		}

		log.Info("Created LREST Cluster Service successfully")
		r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "CreatedLRESTService", "Created LREST Service for %s", lrest.Name)
	} else {
		log.Info("LREST Cluster Service already exists")
	}

	return nil
}

/*
***********************
  - Create Service spec
    /***********************
*/
func (r *LRESTReconciler) createSvcSpec(lrest *dbapi.LREST) *corev1.Service {

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      lrest.Name + "-lrest",
			Namespace: lrest.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": lrest.Name + "-lrest",
			},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	// Set LREST instance as the owner and controller
	ctrl.SetControllerReference(lrest, svc, r.Scheme)
	return svc
}

/*
************************************************
  - Check LREST deletion
    /***********************************************
*/

func (r *LRESTReconciler) manageLRESTDeletion(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {
	log := r.Log.WithValues("manageLRESTDeletion", req.NamespacedName)

	/* REGISTER FINALIZER */
	if lrest.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(lrest, LRESTFinalizer) {
			controllerutil.AddFinalizer(lrest, LRESTFinalizer)
			if err := r.Update(ctx, lrest); err != nil {
				return err
			}
		}

	} else {
		log.Info("lrest set to be deleted")
		lrest.Status.Phase = lrestPhaseDelete
		lrest.Status.Status = true
		r.Status().Update(ctx, lrest)

		if controllerutil.ContainsFinalizer(lrest, LRESTFinalizer) {

			if err := r.DeletePDBS(ctx, req, lrest); err != nil {
				log.Info("Cannot delete lrpdbs")
				return err
			}

			controllerutil.RemoveFinalizer(lrest, LRESTFinalizer)
			if err := r.Update(ctx, lrest); err != nil {
				return err
			}
		}

		err := r.deleteLRESTInstance(ctx, req, lrest)
		if err != nil {
			log.Info("Could not delete LREST Resource", "LREST Name", lrest.Spec.LRESTName, "err", err.Error())
			return err
		}

	}
	return nil
}

/*
************************************************
  - Delete LREST Resource

/***********************************************
*/
func (r *LRESTReconciler) deleteLRESTInstance(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {

	log := r.Log.WithValues("deleteLRESTInstance", req.NamespacedName)

	k_client, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		log.Error(err, "Kubernetes Config Error")
	}

	replicaSetName := lrest.Name + "-lrest-rs"

	err = k_client.AppsV1().ReplicaSets(lrest.Namespace).Delete(context.TODO(), replicaSetName, metav1.DeleteOptions{})
	if err != nil {
		log.Info("Could not delete ReplicaSet", "RS Name", replicaSetName, "err", err.Error())
		if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
			return err
		}
	} else {
		log.Info("Successfully deleted LREST ReplicaSet", "RS Name", replicaSetName)
	}

	r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "DeletedLRESTReplicaSet", "Deleted LREST ReplicaSet for %s", lrest.Name)

	svcName := lrest.Name + "-lrest"

	err = k_client.CoreV1().Services(lrest.Namespace).Delete(context.TODO(), svcName, metav1.DeleteOptions{})
	if err != nil {
		log.Info("Could not delete Service", "Service Name", svcName, "err", err.Error())
		if !strings.Contains(strings.ToUpper(err.Error()), "NOT FOUND") {
			return err
		}
	} else {
		r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "DeletedLRESTService", "Deleted LREST Service for %s", lrest.Name)
		log.Info("Successfully deleted LREST Service", "Service Name", svcName)
	}

	log.Info("Successfully deleted LREST resource", "LREST Name", lrest.Spec.LRESTName)
	return nil
}

/*
************************************************
  - Get Secret Key for a Secret Name
    /***********************************************
*/
func (r *LRESTReconciler) verifySecrets(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {

	log := r.Log.WithValues("verifySecrets", req.NamespacedName)
	/*
		if err := r.checkSecret(ctx, req, lrest, lrest.Spec.SysAdminPwd.Secret.SecretName); err != nil {
			return err
		}*/
	if err := r.checkSecret(ctx, req, lrest, lrest.Spec.LRESTAdminUser.Secret.SecretName); err != nil {
		return err
	}
	if err := r.checkSecret(ctx, req, lrest, lrest.Spec.LRESTAdminPwd.Secret.SecretName); err != nil {
		return err
	}
	/*
		if err := r.checkSecret(ctx, req, lrest, lrest.Spec.LRESTPwd.Secret.SecretName); err != nil {
			return err
		}*/
	if err := r.checkSecret(ctx, req, lrest, lrest.Spec.WebLrestServerUser.Secret.SecretName); err != nil {
		return err
	}
	if err := r.checkSecret(ctx, req, lrest, lrest.Spec.WebLrestServerPwd.Secret.SecretName); err != nil {
		return err
	}

	lrest.Status.Msg = ""
	log.Info("Verified secrets successfully")
	return nil
}

/*
************************************************
  - Get Secret Key for a Secret Name
    /***********************************************
*/
func (r *LRESTReconciler) checkSecret(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST, secretName string) error {

	log := r.Log.WithValues("checkSecret", req.NamespacedName)

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: lrest.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + secretName)
			lrest.Status.Msg = "Secret not found:" + secretName
			return err
		}
		log.Error(err, "Unable to get the secret.")
		return err
	}

	return nil
}

/*
************************************************
  - Delete Secrets
    /***********************************************
*/
func (r *LRESTReconciler) deleteSecrets(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) {

	log := r.Log.WithValues("deleteSecrets", req.NamespacedName)

	log.Info("Deleting LREST secrets")
	secret := &corev1.Secret{}
	/*
		err := r.Get(ctx, types.NamespacedName{Name: lrest.Spec.SysAdminPwd.Secret.SecretName, Namespace: lrest.Namespace}, secret)
		if err == nil {
			err := r.Delete(ctx, secret)
			if err == nil {
				log.Info("Deleted the secret : " + lrest.Spec.SysAdminPwd.Secret.SecretName)
			}
		}
	*/

	err := r.Get(ctx, types.NamespacedName{Name: lrest.Spec.LRESTAdminUser.Secret.SecretName, Namespace: lrest.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + lrest.Spec.LRESTAdminUser.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: lrest.Spec.LRESTAdminPwd.Secret.SecretName, Namespace: lrest.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + lrest.Spec.LRESTAdminPwd.Secret.SecretName)
		}
	}
	/*
		err = r.Get(ctx, types.NamespacedName{Name: lrest.Spec.LRESTPwd.Secret.SecretName, Namespace: lrest.Namespace}, secret)
		if err == nil {
			err := r.Delete(ctx, secret)
			if err == nil {
				log.Info("Deleted the secret : " + lrest.Spec.LRESTPwd.Secret.SecretName)
			}
		}
	*/

	err = r.Get(ctx, types.NamespacedName{Name: lrest.Spec.WebLrestServerUser.Secret.SecretName, Namespace: lrest.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + lrest.Spec.WebLrestServerUser.Secret.SecretName)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: lrest.Spec.WebLrestServerPwd.Secret.SecretName, Namespace: lrest.Namespace}, secret)
	if err == nil {
		err := r.Delete(ctx, secret)
		if err == nil {
			log.Info("Deleted the secret : " + lrest.Spec.WebLrestServerPwd.Secret.SecretName)
		}
	}
}

/*
*************************************************************
  - SetupWithManager sets up the controller with the Manager.
    /************************************************************
*/
func (r *LRESTReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.LREST{}).
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

func securityContextDefineLrest() *corev1.SecurityContext {
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

func (r *LRESTReconciler) DeletePDBS(ctx context.Context, req ctrl.Request, lrest *dbapi.LREST) error {
	log := r.Log.WithValues("DeletePDBS", req.NamespacedName)

	/* =================== DELETE CASCADE ================ */
	if lrest.Spec.DeletePDBCascade == true {
		log.Info("DELETE PDB CASCADE OPTION")
		lrpdbList := &dbapi.LRPDBList{}
		listOpts := []client.ListOption{}
		err := r.List(ctx, lrpdbList, listOpts...)
		if err != nil {
			log.Info("Failed to get the list of pdbs")
		}

		if err == nil {
			for _, pdbitem := range lrpdbList.Items {
				log.Info("pdbitem.Spec.CDBName:" + pdbitem.Spec.CDBName)
				log.Info("lrest.Spec.LRESTName:" + lrest.Spec.LRESTName)
				if pdbitem.Spec.CDBName == lrest.Spec.LRESTName {
					fmt.Printf("DEVPHASE: Call Delete function for %s %s\n", pdbitem.Name, pdbitem.Spec.LRPDBName)

					var objmap map[string]interface{} /* Used for the return payload */
					values := map[string]string{
						"state":        "CLOSE",
						"modifyOption": "ABORT",
					}

					url := "https://" + pdbitem.Spec.CDBResName + "-lrest." + pdbitem.Spec.CDBNamespace + ":" + strconv.Itoa(lrest.Spec.LRESTPort) + "/database/pdbs/" + pdbitem.Spec.LRPDBName

					log.Info("callAPI(URL):" + url)
					log.Info("pdbitem.Status.OpenMode" + pdbitem.Status.OpenMode)

					if pdbitem.Status.OpenMode != "MOUNTED" {

						log.Info("Force pdb closure")
						respData, errapi := NewCallLAPI(r, ctx, req, &pdbitem, url, values, "POST")

						if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
							log.Error(err, "failed to get respData from callAPI", "err", err.Error())
							return err
						}

						pdbitem.Status.SqlCode = int(objmap["sqlcode"].(float64))
						log.Info("pdb closure.......:", "sqlcode", pdbitem.Status.SqlCode)

						if errapi != nil {
							log.Error(err, "callAPI cannot close pdb "+pdbitem.Spec.LRPDBName, "err", err.Error())
							return err
						}

						r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "close pdb", "pdbname=%s", pdbitem.Spec.LRPDBName)
					}

					/* start dropping pdb */
					log.Info("Drop pluggable database")
					values = map[string]string{
						"action": "INCLUDING",
					}
					respData, errapi := NewCallLAPI(r, ctx, req, &pdbitem, url, values, "DELETE")

					if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
						log.Error(err, "failed to get respData from callAPI", "err", err.Error())
						return err
					}

					pdbitem.Status.SqlCode = int(objmap["sqlcode"].(float64))
					log.Info(".......:", "sqlcode", pdbitem.Status.SqlCode)

					if errapi != nil {
						log.Error(err, "callAPI cannot drop pdb "+pdbitem.Spec.LRPDBName, "err", err.Error())
						return err
					}
					r.Recorder.Eventf(lrest, corev1.EventTypeNormal, "drop pdb", "pdbname=%s", pdbitem.Spec.LRPDBName)

					/*
						if controllerutil.ContainsFinalizer(&pdbitem, LRPDBFinalizer) {
							log.Info("Removing finalizer")
							controllerutil.RemoveFinalizer(&pdbitem, LRPDBFinalizer)
							err = r.Update(ctx, &pdbitem)
							if err != nil {
								log.Info("Could not remove finalizer", "err", err.Error())
								return err
							}
						}
					*/

					err = r.Delete(context.Background(), &pdbitem, client.GracePeriodSeconds(1))
					if err != nil {
						log.Info("Could not delete LRPDB resource", "err", err.Error())
						return err
					}

				} /* check pdb name */
			} /* end of loop */
		}

	}
	/* ================================================ */
	return nil
}
