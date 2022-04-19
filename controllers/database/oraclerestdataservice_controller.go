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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	"github.com/go-logr/logr"
)

const oracleRestDataServiceFinalizer = "database.oracle.com/oraclerestdataservicefinalizer"

// OracleRestDataServiceReconciler reconciles a OracleRestDataService object
type OracleRestDataServiceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=oraclerestdataservices/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services;nodes;events,verbs=create;delete;get;list;patch;update;watch

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
	_ = log.FromContext(ctx)

	oracleRestDataService := &dbapi.OracleRestDataService{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, oracleRestDataService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted")
			return requeueN, nil
		}
		return requeueN, err
	}

	// Fetch Primary Database Reference
	singleInstanceDatabase := &dbapi.SingleInstanceDatabase{}
	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: oracleRestDataService.Spec.DatabaseRef}, singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted")
			return requeueN, nil
		}
		return requeueN, err
	}

	// Manage SingleInstanceDatabase Deletion
	result := r.manageOracleRestDataServiceDeletion(req, ctx, oracleRestDataService, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// First validate
	result, err = r.validate(oracleRestDataService, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Spec validation failed, Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Info("Spec validation failed")
		return result, nil
	}

	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, oracleRestDataService)

	// Create Service
	result = r.createSVC(ctx, req, oracleRestDataService)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// PVC Creation
	result, _ = r.createPVC(ctx, req, oracleRestDataService)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Validate if Primary Database Reference is ready
	result, sidbReadyPod := r.validateSidbReadiness(oracleRestDataService, singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Create ORDS Pods
	result = r.createPods(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Configure Apex
	result = r.configureApex(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	result = r.setupORDS(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	result = r.checkHealthStatus(oracleRestDataService, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Delete Secrets
	r.deleteSecrets(oracleRestDataService, ctx, req)

	if oracleRestDataService.Status.ServiceIP == "" {
		return requeueY, nil
	}

	return ctrl.Result{}, nil
}

//#############################################################################
//    Validate the CRD specs
//#############################################################################
func (r *OracleRestDataServiceReconciler) validate(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) (ctrl.Result, error) {

	var err error
	eventReason := "Spec Error"
	var eventMsgs []string
	//  If using same pvc for ords as sidb, ensure sidb has ReadWriteMany Accessmode
	if n.Spec.Persistence.AccessMode == "ReadWriteOnce" && m.Spec.Persistence.AccessMode == "" {
		eventMsgs = append(eventMsgs, "ords can be installed only on ReadWriteMany Access Mode of : "+m.Spec.DatabaseRef)
	}
	if m.Status.DatabaseRef != "" && m.Status.DatabaseRef != m.Spec.DatabaseRef {
		eventMsgs = append(eventMsgs, "databaseRef cannot be updated")
	}
	if m.Status.LoadBalancer != "" && m.Status.LoadBalancer != strconv.FormatBool(m.Spec.LoadBalancer) {
		eventMsgs = append(eventMsgs, "service patching is not avaiable currently")
	}
	if !n.Status.ApexInstalled && m.Spec.ApexPassword.SecretName != "" {
		eventMsgs = append(eventMsgs, "apex is not installed yet")
	}
	if m.Status.Image.PullFrom != "" && m.Status.Image != m.Spec.Image {
		eventMsgs = append(eventMsgs, "image patching is not avaiable currently")
	}

	m.Status.DatabaseRef = m.Spec.DatabaseRef
	m.Status.LoadBalancer = strconv.FormatBool(m.Spec.LoadBalancer)
	m.Status.Image = m.Spec.Image

	if len(eventMsgs) > 0 {
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, strings.Join(eventMsgs, ","))
		r.Log.Info(strings.Join(eventMsgs, "\n"))
		err = errors.New(strings.Join(eventMsgs, ","))
		return requeueN, err
	}

	return requeueN, err
}

//#####################################################################################################
//    Validate Readiness of the primary DB specified
//#####################################################################################################
func (r *OracleRestDataServiceReconciler) validateSidbReadiness(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod) {

	log := r.Log.WithValues("validateSidbReadiness", req.NamespacedName)

	// ## FETCH THE SIDB REPLICAS .
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
		n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}

	if sidbReadyPod.Name == "" || n.Status.Status != dbcommons.StatusReady {
		eventReason := "Waiting"
		eventMsg := "waiting for " + n.Name + " to be Ready"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		return requeueY, sidbReadyPod
	}
	if m.Status.OrdsInstalled || m.Status.CommonUsersCreated {
		return requeueN, sidbReadyPod
	}

	// Validate databaseRef Admin Password
	adminPasswordSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for secret : " + m.Spec.AdminPassword.SecretName + " to get created"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + m.Spec.AdminPassword.SecretName + " Not Found")
			return requeueY, sidbReadyPod
		}
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	adminPassword := string(adminPasswordSecret.Data[m.Spec.AdminPassword.SecretKey])

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.ValidateAdminPassword, adminPassword), dbcommons.SQLPlusCLI))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	if strings.Contains(out, "USER is \"SYS\"") {
		log.Info("validated Admin password successfully")
	} else if strings.Contains(out, "ORA-01017") {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Logon denied"
		eventMsg := "invalid databaseRef admin password. secret: " + m.Spec.AdminPassword.SecretName
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY, sidbReadyPod
	} else {
		return requeueY, sidbReadyPod
	}

	return requeueN, sidbReadyPod
}

//#####################################################################################################
//    Check ORDS Health Status
//#####################################################################################################
func (r *OracleRestDataServiceReconciler) checkHealthStatus(m *dbapi.OracleRestDataService,
	ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("checkHealthStatus", req.NamespacedName)

	readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	if readyPod.Name == "" {
		return requeueY
	}

	// Get ORDS Status
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		dbcommons.GetORDSStatus)
	log.Info("GetORDSStatus Output")
	log.Info(out)
	if strings.Contains(strings.ToUpper(out), "ERROR") {
		return requeueY
	}
	if err != nil {
		log.Info(err.Error())
		if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
			return requeueY
		}
	}

	if strings.Contains(out, "HTTP/1.1 200 OK") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 200 OK") {
		m.Status.Status = dbcommons.StatusReady
	} else {
		m.Status.Status = dbcommons.StatusNotReady
		return requeueY
	}
	return requeueN
}

//#############################################################################
//    Instantiate Service spec from OracleRestDataService spec
//#############################################################################
func (r *OracleRestDataServiceReconciler) instantiateSVCSpec(m *dbapi.OracleRestDataService) *corev1.Service {
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
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "client",
					Port:     8443,
					Protocol: corev1.ProtocolTCP,
				},
			},
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
	ctrl.SetControllerReference(m, svc, r.Scheme)
	return svc
}

//#############################################################################
//    Instantiate POD spec from OracleRestDataService spec
//#############################################################################
func (r *OracleRestDataServiceReconciler) instantiatePodSpec(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase, ordsInstalled bool) *corev1.Pod {

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
			Volumes: []corev1.Volume{{
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
			}},
			InitContainers: []corev1.Container{{
				Name:    "init-permissions",
				Image:   m.Spec.Image.PullFrom,
				Command: []string{"/bin/sh", "-c", fmt.Sprintf("chown %d:%d /opt/oracle/ords/config/ords", int(dbcommons.ORACLE_UID), int(dbcommons.DBA_GUID))},
				SecurityContext: &corev1.SecurityContext{
					// User ID 0 means, root user
					RunAsUser: func() *int64 { i := int64(0); return &i }(),
				},
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "/opt/oracle/ords/config/ords",
					Name:      "datamount",
					SubPath:   strings.ToUpper(n.Spec.Sid) + "_ORDS",
				}},
			}},
			Containers: []corev1.Container{{
				Name:            m.Name,
				Image:           m.Spec.Image.PullFrom,
				ImagePullPolicy: corev1.PullAlways,
				Ports:           []corev1.ContainerPort{{ContainerPort: 8443}},
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "/opt/oracle/ords/config/ords/",
					Name:      "datamount",
					SubPath:   strings.ToUpper(n.Spec.Sid) + "_ORDS",
				}},
				Env: func() []corev1.EnvVar {
					// ORDS_PWD, ORACLE_PWD is required only during the First ORDS Installation.
					if !ordsInstalled {
						return []corev1.EnvVar{
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
							{
								Name: "ORDS_PWD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: m.Spec.OrdsPassword.SecretName,
										},
										Key: m.Spec.OrdsPassword.SecretKey,
									},
								},
							},
							{
								Name: "ORACLE_PWD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: m.Spec.AdminPassword.SecretName,
										},
										Key: m.Spec.AdminPassword.SecretKey,
									},
								},
							},
						}
					}
					// After ORDS is Installed, we DELETE THE OLD ORDS Pod and create new ones ONLY USING BELOW ENV VARIABLES.
					return []corev1.EnvVar{
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
			},

			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: m.Spec.Image.PullSecrets,
				},
			},
		},
	}

	// Set oracleRestDataService instance as the owner and controller
	ctrl.SetControllerReference(m, pod, r.Scheme)
	return pod
}

//#############################################################################
//    Instantiate Persistent Volume Claim spec from SingleInstanceDatabase spec
//#############################################################################
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
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					// Requests describes the minimum amount of compute resources required
					"storage": resource.MustParse(m.Spec.Persistence.Size),
				},
			},
			StorageClassName: &m.Spec.Persistence.StorageClass,
		},
	}
	// Set SingleInstanceDatabase instance as the owner and controller
	ctrl.SetControllerReference(m, pvc, r.Scheme)
	return pvc
}

//#############################################################################
//    Create a Service for OracleRestDataService
//#############################################################################
func (r *OracleRestDataServiceReconciler) createSVC(ctx context.Context, req ctrl.Request,
	m *dbapi.OracleRestDataService) ctrl.Result {

	log := r.Log.WithValues("createSVC", req.NamespacedName)
	// Check if the Service already exists, if not create a new one
	svc := &corev1.Service{}
	// Get retrieves an obj for the given object key from the Kubernetes Cluster.
	// obj must be a struct pointer so that obj can be updated with the response returned by the Server.
	// Here foundsvc is the struct pointer to corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, svc)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new Service
		svc = r.instantiateSVCSpec(m)
		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY
		} else {
			log.Info("Succesfully Created New Service ", "Service.Name : ", svc.Name)
		}

	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return requeueY
	} else if err == nil {
		log.Info("Found Existing Service ", "Service.Name", svc.Name)
	}

	m.Status.ServiceIP = ""
	if m.Spec.LoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			m.Status.DatabaseApiUrl = "https://" + svc.Status.LoadBalancer.Ingress[0].IP + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/"
			m.Status.ServiceIP = svc.Status.LoadBalancer.Ingress[0].IP
			m.Status.DatabaseActionsUrl = "https://" + svc.Status.LoadBalancer.Ingress[0].IP + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/sql-developer"
		}
		m.Status.ClusterDbApiUrl = "https://" + svc.Name + "." + svc.Namespace + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/"
		return requeueN
	}
	m.Status.ClusterDbApiUrl = "https://" + svc.Name + "." + svc.Namespace + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/"
	nodeip := dbcommons.GetNodeIp(r, ctx, req)
	if nodeip != "" {
		m.Status.ServiceIP = nodeip
		m.Status.DatabaseApiUrl = "https://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/ords/"
		m.Status.DatabaseActionsUrl = "https://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/ords/sql-developer"
	}
	return requeueN
}

//#############################################################################
//    Stake a claim for Persistent Volume
//#############################################################################
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
	} else {
		log.Info("PVC already exists")
	}

	return requeueN, nil
}

//#############################################################################
//    Create the requested POD replicas
//#############################################################################
func (r *OracleRestDataServiceReconciler) createPods(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("createPods", req.NamespacedName)

	readyPod, replicasFound, available, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	log.Info(m.Name, " pods other than one of Ready Pods : ", dbcommons.GetPodNames(available))
	log.Info(m.Name, " Ready Pod : ", readyPod.Name)

	replicasReq := m.Spec.Replicas
	if replicasFound == 0 {
		m.Status.Status = dbcommons.StatusNotReady
		m.Status.DatabaseApiUrl = dbcommons.ValueUnavailable
		m.Status.DatabaseActionsUrl = dbcommons.ValueUnavailable
	}

	if replicasFound == replicasReq {
		log.Info("No of " + m.Name + " replicas Found are same as Required")
	} else if replicasFound < replicasReq {
		// Create New Pods , Name of Pods are generated Randomly
		for i := replicasFound; i < replicasReq; i++ {
			if i > 0 && !m.Status.OrdsInstalled {
				// Wait till the first created pod to installs ORDS .
				break
			}
			ordsInstalled := false
			if m.Status.OrdsInstalled {
				ordsInstalled = true
			}
			pod := r.instantiatePodSpec(m, n, ordsInstalled)
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
			err := r.Delete(ctx, &pod, &client.DeleteOptions{})
			noDeleted += 1
			if err != nil {
				r.Log.Error(err, "Failed to delete existing POD", "POD.Name", pod.Name)
				// Don't requeue
			}
		}
	}
	n.Status.OrdsReference = m.Name
	r.Status().Update(ctx, n)
	m.Status.Replicas = m.Spec.Replicas
	if !m.Status.OrdsInstalled {
		m.Status.Replicas = 1
	}
	return requeueN
}

//#############################################################################
//   Manage Finalizer to cleanup before deletion of OracleRestDataService
//#############################################################################
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
				err := r.Status().Update(ctx, n)
				if err != nil {
					log.Info(err.Error() + "\n updating n.Status.OrdsInstalled = false")
					time.Sleep(5 * time.Second)
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

//#############################################################################
//   Finalization logic for OracleRestDataServiceFinalizer
//#############################################################################
func (r *OracleRestDataServiceReconciler) cleanupOracleRestDataService(req ctrl.Request, ctx context.Context,
	m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) error {
	log := r.Log.WithValues("cleanupOracleRestDataService", req.NamespacedName)

	readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	if m.Status.OrdsInstalled && readyPod.Name != "" {
		// ## FETCH THE SIDB REPLICAS .
		sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
			n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}

		if sidbReadyPod.Name == "" {
			eventReason := "No Ready Pod"
			eventMsg := "ommitting ORDS uninstallation as No Ready Pod of " + n.Name + " available"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
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

		if readyPod.Name == "" {
			eventReason := "Waiting"
			eventMsg := "waiting for " + m.Name + " to be Ready"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			err = errors.New(eventMsg)
			return err
		}

		// Fetch admin Password of database to uninstall ORDS
		adminPasswordSecret := &corev1.Secret{}
		adminPasswordSecretFound := false
		for i := 0; i < 5; i++ {
			err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: n.Namespace}, adminPasswordSecret)
			if err != nil {
				log.Error(err, err.Error())
				if apierrors.IsNotFound(err) {
					eventReason := "Waiting"
					eventMsg := "waiting for admin password secret : " + m.Spec.AdminPassword.SecretName + " to get created"
					r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
					r.Log.Info(eventMsg)
					if i < 4 {
						time.Sleep(15 * time.Second)
						continue
					}
				}
			} else {
				adminPasswordSecretFound = true
				break
			}
		}
		if !adminPasswordSecretFound {
			log.Info("AdminPassword Secret not found . Omitting OracleRestDataService uninstallation")
			return nil
		}

		adminPassword := string(adminPasswordSecret.Data[m.Spec.AdminPassword.SecretKey])
		uninstallORDS := fmt.Sprintf(dbcommons.UninstallORDSCMD, adminPassword)

		out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
			uninstallORDS)
		log.Info("UninstallORDSCMD Output : " + out)
		if strings.Contains(strings.ToUpper(out), "ERROR") {
			return errors.New(out)
		}
		if err != nil {
			log.Info(err.Error())
			if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
				return err
			}
		}

		log.Info("UninstallORDSCMD Output : " + out)

		//Delete Database Admin Password Secret
		if !m.Spec.AdminPassword.KeepSecret {
			err = r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				r.Log.Info("Deleted Admin Password Secret :" + adminPasswordSecret.Name)
			}
		}

		// Drop Admin Users
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s ", dbcommons.DropAdminUsersSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DropAdminUsersSQL Output : " + out)

	}

	// Cleanup steps that the operator needs to do before the CR can be deleted.
	log.Info("Successfully cleaned up OracleRestDataService ")
	return nil
}

//#############################################################################
//             Configure APEX
//#############################################################################
func (r *OracleRestDataServiceReconciler) configureApex(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("configureApex", req.NamespacedName)

	if m.Spec.ApexPassword.SecretName == "" {
		m.Status.ApexConfigured = false
		return requeueN
	}
	if m.Status.ApexConfigured {
		return requeueN
	}
	if !n.Status.ApexInstalled {
		m.Status.Status = dbcommons.StatusError
		eventReason := "Failed"
		eventMsg := "apex is not installed yet on " + m.Spec.DatabaseRef
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY
	}

	// If Seperate PV for ORDS, we need to unzip apex-latest.zip
	if m.Spec.Persistence.AccessMode != "" {
		ordsReadyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
			m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		if ordsReadyPod.Name == "" {
			eventReason := "Waiting"
			eventMsg := "waiting for " + m.Name + " to be Ready"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			return requeueY
		}

		unzipApex := dbcommons.UnzipApexOnORDSPod
		if n.Spec.Edition == "express" {
			unzipApex += dbcommons.ChownApex
		}

		// Unzip /opt/oracle/ords/config/ords/apex-latest.zip to /opt/oracle/ords/config/ords
		out, err := dbcommons.ExecCommand(r, r.Config, ordsReadyPod.Name, ordsReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			unzipApex)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info(" UnzipApex Output : \n" + out)

		if strings.Contains(out, "apex-latest.zip not found") {
			eventReason := "Waiting"
			eventMsg := "apex-latest.zip doesn't exist in the location /opt/oracle/ords/config/ords"
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			return requeueY
		}

	} else {

		// Copy APEX Images to ORACLE_SID only if PV for ORDS and SIDB is shared
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.CopyApexImages))
		if err != nil {
			log.Info(err.Error())
			return requeueY
		}
		log.Info(" CopyApexImages Output : \n" + out)

	}

	apexPasswordSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: m.Spec.ApexPassword.SecretName, Namespace: m.Namespace}, apexPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for secret : " + m.Spec.ApexPassword.SecretName + " to get created"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + m.Spec.ApexPassword.SecretName + " Not Found")
			return requeueY
		}
		log.Error(err, err.Error())
		return requeueY
	}
	// APEX_LISTENER , APEX_REST_PUBLIC_USER , APEX_PUBLIC_USER passwords
	apexPassword := string(apexPasswordSecret.Data[m.Spec.ApexPassword.SecretKey])

	m.Status.Status = dbcommons.StatusUpdating
	r.Status().Update(ctx, m)

	configureApexRestSqlClient := "sqlplus -s / as sysdba @apex_rest_config.sql"
	// Configure APEX
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.ConfigureApexRest, apexPassword, configureApexRestSqlClient))
	if err != nil {
		log.Info(err.Error())
		if !strings.Contains(err.Error(), "catcon.pl: completed") {
			return requeueY
		}
	}
	log.Info(" ConfigureApexRest Output : \n" + out)

	if strings.Contains(out, "Apex Folder doesn't exist") {
		eventReason := "Waiting"
		eventMsg := "apex Folder doesn't exist in the location /opt/oracle/oradata/" + strings.ToUpper(n.Spec.Sid)
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY
	}

	// Alter APEX_LISTENER,APEX_PUBLIC_USER,APEX_REST_PUBLIC_USER
	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true,
		"bash", "-c", fmt.Sprintf(dbcommons.AlterApexUsers, apexPassword, dbcommons.SQLPlusCLI))
	if err != nil {
		log.Info(err.Error())
		return requeueY
	}
	log.Info(" AlterApexUsers Output : \n" + out)

	pdbName := "orclpdb1"
	if n.Spec.Pdbname != "" {
		pdbName = n.Spec.Pdbname
	}

	// Change APEX Admin Password
	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true,
		"bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.ApexAdmin, apexPassword, pdbName), dbcommons.SQLPlusCLI))
	if err != nil {
		log.Info(err.Error())
		return requeueY
	}
	log.Info("Change ApexAdmin Password Output : \n" + out)

	m.Status.ApexConfigured = true
	r.Status().Update(ctx, m)

	if m.Status.OrdsInstalled {

		readyPod, _, available, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
			m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		if readyPod.Name != "" {
			log.Info("Ready Pod marked for deletion")
			available = append(available, readyPod)
		}

		// Set Apex users in apex_rt,apex_al,apex files
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf(dbcommons.SetApexUsers, apexPassword))
		log.Info("SetApexUsers Output: \n" + out)
		if strings.Contains(strings.ToUpper(out), "ERROR") {
			return requeueY
		}
		if err != nil {
			log.Info(err.Error())
			if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
				return requeueY
			}
		}

		for _, pod := range available {
			// ORDS Needs to be restarted to configure APEX
			err = r.Delete(ctx, &pod, &client.DeleteOptions{})
			if err != nil {
				r.Log.Error(err, "Failed to delete existing POD", "POD.Name", pod.Name)
				return requeueY
			}
			r.Log.Info("ORDS Pod Deleted : " + pod.Name)
		}

		return requeueY
	}

	return requeueN
}

//#############################################################################
//             Delete Secrets
//#############################################################################
func (r *OracleRestDataServiceReconciler) deleteSecrets(m *dbapi.OracleRestDataService, ctx context.Context, req ctrl.Request) {
	log := r.Log.WithValues("deleteSecrets", req.NamespacedName)

	if !m.Spec.AdminPassword.KeepSecret {
		// Fetch adminPassword Secret
		adminPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, adminPasswordSecret)
		if err == nil {
			//Delete Database Admin Password Secret .
			err := r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("Database Admin Password secret Deleted : " + adminPasswordSecret.Name)
			}
		}
	}

	if !m.Spec.OrdsPassword.KeepSecret {
		// Fetch ordsPassword Secret
		ordsPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.OrdsPassword.SecretName, Namespace: m.Namespace}, ordsPasswordSecret)
		if err == nil {
			//Delete ORDS Password Secret .
			err := r.Delete(ctx, ordsPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("ORDS Password secret Deleted : " + ordsPasswordSecret.Name)
			}
		}
	}

	if !m.Spec.ApexPassword.KeepSecret {
		// Fetch apexPassword Secret
		apexPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.ApexPassword.SecretName, Namespace: m.Namespace}, apexPasswordSecret)
		if err == nil {
			//Delete APEX Password Secret .
			err := r.Delete(ctx, apexPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("APEX Password secret Deleted : " + apexPasswordSecret.Name)
			}
		}
	}

}

//#############################################################################
//             Setup ORDS in CDB , enable ORDS for PDBs Specified
//#############################################################################
func (r *OracleRestDataServiceReconciler) setupORDS(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("setupORDS", req.NamespacedName)

	readyPod, replicasFound, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	if readyPod.Name == "" {
		eventReason := "Waiting"
		eventMsg := "waiting for " + m.Name + " to be Ready"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		return requeueY
	}
	OrdsPasswordSecret := &corev1.Secret{}
	if !m.Status.OrdsInstalled {
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.OrdsPassword.SecretName, Namespace: m.Namespace}, OrdsPasswordSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				m.Status.Status = dbcommons.StatusError
				eventReason := "Waiting"
				eventMsg := "waiting for secret : " + m.Spec.OrdsPassword.SecretName + " to get created"
				r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
				r.Log.Info("Secret " + m.Spec.OrdsPassword.SecretName + " Not Found")
				return requeueY
			}
			log.Error(err, err.Error())
			return requeueY
		}
		ordsPassword := string(OrdsPasswordSecret.Data[m.Spec.OrdsPassword.SecretKey])

		// APEX_LISTENER , APEX_REST_PUBLIC_USER , APEX_PUBLIC_USER , APEX ADMIN passwords
		apexPassword := ordsPassword
		if m.Spec.ApexPassword.SecretName != "" {
			apexPasswordSecret := &corev1.Secret{}
			err = r.Get(ctx, types.NamespacedName{Name: m.Spec.ApexPassword.SecretName, Namespace: m.Namespace}, apexPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					m.Status.Status = dbcommons.StatusError
					eventReason := "Waiting"
					eventMsg := "waiting for secret : " + m.Spec.ApexPassword.SecretName + " to get created"
					r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
					r.Log.Info("Secret " + m.Spec.ApexPassword.SecretName + " Not Found")
					return requeueY
				}
				log.Error(err, err.Error())
				return requeueY
			}
			apexPassword = string(apexPasswordSecret.Data[m.Spec.ApexPassword.SecretKey])
		}

		// Get ORDS Status
		out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			dbcommons.GetORDSStatus)
		log.Info("GetORDSStatus Output")
		log.Info(out)
		if strings.Contains(strings.ToUpper(out), "ERROR") {
			return requeueY
		}
		if err != nil {
			log.Info(err.Error())
			if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
				return requeueY
			}
		}

		if strings.Contains(out, "HTTP/1.1 200 OK") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 200 OK") {

			if !m.Status.OrdsSetupCompleted {
				cdbAdminPassword := dbcommons.GenerateRandomString(8)
				// Create PDB , CDB Admin users and grant permissions . ORDS installation on CDB level
				out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
					fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.SetAdminUsersSQL, cdbAdminPassword), dbcommons.SQLPlusCLI))
				if err != nil {
					log.Error(err, err.Error())
					return requeueY
				}
				log.Info("SetAdminUsers Output :\n" + out)

				if !strings.Contains(out, "ERROR") || !strings.Contains(out, "ORA-") ||
					strings.Contains(out, "ERROR") && strings.Contains(out, "ORA-01920") {
					m.Status.CommonUsersCreated = true
				}

				// Setup ORDS
				out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
					fmt.Sprintf(dbcommons.SetupORDSCMD, ordsPassword, apexPassword, m.Name, cdbAdminPassword))
				log.Info("SetupORDSCMD Output")
				log.Info(out)
				if strings.Contains(strings.ToUpper(out), "ERROR") {
					return requeueY
				}
				if err != nil {
					log.Info(err.Error())
					if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
						return requeueY
					}
				}

				if !strings.Contains(strings.ToUpper(out), "ERROR") && !strings.Contains(strings.ToUpper(err.Error()), "ERROR") &&
					!strings.Contains(strings.ToUpper(out), "could not execute") && !strings.Contains(strings.ToUpper(err.Error()), "could not execute") {
					m.Status.OrdsSetupCompleted = true
				}
			}

			// ORDS Needs to be restarted to ensure ORDS Installation works fine
			err := r.Delete(ctx, &readyPod, &client.DeleteOptions{})
			if err != nil {
				r.Log.Error(err, "Failed to delete existing POD", "POD.Name", readyPod.Name)
				return requeueY
			}
			r.Log.Info("ORDS Installation Pod Deleted : " + readyPod.Name)

			eventReason := "ORDS Installed"
			eventMsg := ""
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			m.Status.OrdsInstalled = true
			r.Status().Update(ctx, m)

			// Wait for 15 sec to update OrdsInstalled to true
			time.Sleep(15 * time.Second)

		}
		return requeueY
	}

	result := r.restEnableSchemas(m, n, sidbReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result
	}

	if replicasFound < m.Spec.Replicas {
		// Requeue if all replicas not created
		return requeueY
	}
	return requeueN
}

//#############################################################################
//             Rest Enable/Disable Schemas
//#############################################################################
func (r *OracleRestDataServiceReconciler) restEnableSchemas(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("restEnableSchemas", req.NamespacedName)

	// Get Pdbs Available
	availablePDBS, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.GetPdbsSQL, dbcommons.SQLPlusCLI))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	} else {
		log.Info("GetPdbsSQL Output")
		log.Info(availablePDBS)
	}

	for i := 0; i < len(m.Spec.RestEnableSchemas); i++ {

		//  If the PDB mentioned in yaml doesnt contain in the database , continue
		if !strings.Contains(strings.ToUpper(availablePDBS), strings.ToUpper(m.Spec.RestEnableSchemas[i].Pdb)) {
			eventReason := "Warning"
			eventMsg := "enabling ORDS schema for PDB : " + m.Spec.RestEnableSchemas[i].Pdb + " failed ; as pdb not found"
			log.Info(eventMsg)
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			continue
		}

		getOrdsSchemaStatus := fmt.Sprintf(dbcommons.GetUserOrdsSchemaStatusSQL, m.Spec.RestEnableSchemas[i].Schema, m.Spec.RestEnableSchemas[i].Pdb)

		// Get ORDS Schema status for PDB
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", getOrdsSchemaStatus, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		} else {
			log.Info("getOrdsSchemaStatus Output")
			log.Info(out)
		}

		// if ORDS already enabled for given PDB
		if strings.Contains(out, "STATUS:ENABLED") && m.Spec.RestEnableSchemas[i].Enable {
			continue
		}

		// if ORDS already disabled for given PDB
		if !strings.Contains(out, "STATUS:ENABLED") && !m.Spec.RestEnableSchemas[i].Enable {
			continue
		}

		OrdsPasswordSecret := &corev1.Secret{}
		// Fetch the secret to get password for database user . Secret has to be created in the same namespace of OracleRestDataService
		err = r.Get(ctx, types.NamespacedName{Name: m.Spec.OrdsPassword.SecretName, Namespace: m.Namespace}, OrdsPasswordSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				eventReason := "No Secret"
				eventMsg := "secret " + m.Spec.OrdsPassword.SecretName + " Not Found"
				r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
				r.Log.Info(eventMsg)
				return requeueY
			}
			log.Error(err, err.Error())
			return requeueY
		}

		password := string(OrdsPasswordSecret.Data[m.Spec.OrdsPassword.SecretKey])
		urlMappingPattern := ""
		if m.Spec.RestEnableSchemas[i].UrlMapping == "" {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].Schema)
		} else {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].UrlMapping)
		}
		enableORDSSchema := fmt.Sprintf(dbcommons.EnableORDSSchemaSQL, strings.ToUpper(m.Spec.RestEnableSchemas[i].Schema), password,
			strconv.FormatBool(m.Spec.RestEnableSchemas[i].Enable), urlMappingPattern, m.Spec.RestEnableSchemas[i].Pdb)

		// Create users,schemas and grant enableORDS for PDB
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", enableORDSSchema, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("getOrdsSchemaStatus Output : " + out)

	}

	return requeueN

}

//#############################################################################
//        SetupWithManager sets up the controller with the Manager.
//#############################################################################
func (r *OracleRestDataServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.OracleRestDataService{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of OracleRestDataService Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}