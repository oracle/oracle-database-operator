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

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
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
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, oracleRestDataService)

	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, oracleRestDataService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		return requeueY, err
	}

	/* Initialize Status */
	if oracleRestDataService.Status.Status == "" {
		oracleRestDataService.Status.Status = dbcommons.StatusPending
		oracleRestDataService.Status.ApxeUrl = dbcommons.ValueUnavailable
		oracleRestDataService.Status.DatabaseApiUrl = dbcommons.ValueUnavailable
		oracleRestDataService.Status.DatabaseActionsUrl = dbcommons.ValueUnavailable
		r.Status().Update(ctx, oracleRestDataService)
	}
	oracleRestDataService.Status.LoadBalancer = strconv.FormatBool(oracleRestDataService.Spec.LoadBalancer)
	oracleRestDataService.Status.Image = oracleRestDataService.Spec.Image

	// Fetch Primary Database Reference
	singleInstanceDatabase := &dbapi.SingleInstanceDatabase{}
	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, singleInstanceDatabase)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: oracleRestDataService.Spec.DatabaseRef}, singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			oracleRestDataService.Status.Status = dbcommons.StatusError
			oracleRestDataService.Status.DatabaseRef = ""
			eventReason := "Error"
			eventMsg := "database reference " + oracleRestDataService.Spec.DatabaseRef + " not found"
			r.Recorder.Eventf(oracleRestDataService, corev1.EventTypeWarning, eventReason, eventMsg)
			r.Log.Info(eventMsg)
			return requeueY, nil
		}
		r.Log.Error(err, err.Error())
		return requeueY, err
	} else {
		if oracleRestDataService.Status.DatabaseRef == "" {
			oracleRestDataService.Status.Status = dbcommons.StatusPending
			oracleRestDataService.Status.DatabaseRef = oracleRestDataService.Spec.DatabaseRef
			eventReason := "Database Check"
			eventMsg := "database reference " + oracleRestDataService.Spec.DatabaseRef + " found"
			r.Recorder.Eventf(oracleRestDataService, corev1.EventTypeNormal, eventReason, eventMsg)
		}
	}

	// Manage OracleRestDataService Deletion
	result := r.manageOracleRestDataServiceDeletion(req, ctx, oracleRestDataService, singleInstanceDatabase)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// First validate
	result, err = r.validate(oracleRestDataService, singleInstanceDatabase, ctx)
	if result.Requeue || err != nil {
		r.Log.Info("Spec validation failed")
		return result, nil
	}

	// Create Service
	result = r.createSVC(ctx, req, oracleRestDataService, singleInstanceDatabase)
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
	result, sidbReadyPod := r.validateSIDBReadiness(oracleRestDataService, singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Create ORDS Pods
	result = r.createPods(oracleRestDataService, singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	var ordsReadyPod corev1.Pod
	result, ordsReadyPod = r.checkHealthStatus(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	result = r.restEnableSchemas(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ordsReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Configure Apex
	result = r.configureApex(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ordsReadyPod, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Configure MongoDB
	result = r.enableMongoDB(oracleRestDataService, singleInstanceDatabase, sidbReadyPod, ordsReadyPod, ctx, req)
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

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) validate(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context) (ctrl.Result, error) {

	var err error
	eventReason := "Spec Error"
	var eventMsgs []string

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

	// If ORDS has no peristence specified, ensure SIDB has persistence configured
	if m.Spec.Persistence.Size == "" && n.Spec.Persistence.AccessMode == "" {
		eventMsgs = append(eventMsgs, "cannot configure ORDS for database "+m.Spec.DatabaseRef+" that has no attached persistent volume")
	}
	if !m.Status.OrdsInstalled && n.Status.OrdsReference != "" {
		eventMsgs = append(eventMsgs, "database "+m.Spec.DatabaseRef+" is already configured with ORDS "+n.Status.OrdsReference)
	}
	if m.Status.DatabaseRef != "" && m.Status.DatabaseRef != m.Spec.DatabaseRef {
		eventMsgs = append(eventMsgs, "databaseRef cannot be updated")
	}
	if m.Status.Image.PullFrom != "" && m.Status.Image != m.Spec.Image {
		eventMsgs = append(eventMsgs, "image patching is not available currently")
	}

	if len(eventMsgs) > 0 {
		m.Status.Status = dbcommons.StatusError
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, strings.Join(eventMsgs, ","))
		r.Log.Info(strings.Join(eventMsgs, "\n"))
		err = errors.New(strings.Join(eventMsgs, ","))
		return requeueY, err
	}

	return requeueN, err
}

// #####################################################################################################
//
//	Validate Readiness of the primary DB specified
//
// #####################################################################################################
func (r *OracleRestDataServiceReconciler) validateSIDBReadiness(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod) {

	log := r.Log.WithValues("validateSidbReadiness", req.NamespacedName)

	// ## FETCH THE SIDB REPLICAS .
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
		n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}

	if m.Status.OrdsInstalled || m.Status.CommonUsersCreated {
		return requeueN, sidbReadyPod
	}

	m.Status.Status = dbcommons.StatusPending
	if sidbReadyPod.Name == "" || n.Status.Status != dbcommons.StatusReady {
		eventReason := "Database Check"
		eventMsg := "status of database " + n.Name + " is not ready, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY, sidbReadyPod
	} else {
		eventReason := "Database Check"
		eventMsg := "status of database " + n.Name + " is ready"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
	}

	// Validate databaseRef Admin Password
	adminPasswordSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			eventReason := "Database Password"
			eventMsg := "password secret " + m.Spec.AdminPassword.SecretName + " not found, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			r.Log.Info(eventMsg)
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
		eventReason := "Database Check"
		eventMsg := "login denied, invalid database admin password in secret " + m.Spec.AdminPassword.SecretName
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		log.Info(eventMsg)
		return requeueY, sidbReadyPod
	} else {
		eventMsg := "login attempt failed for database admin password in secret " + m.Spec.AdminPassword.SecretName
		log.Info(eventMsg)
		return requeueY, sidbReadyPod
	}

	// Create PDB , CDB Admin users and grant permissions. ORDS installation on CDB level
	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.SetAdminUsersSQL, adminPassword), dbcommons.SQLPlusCLI))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "ORA-") ||
		strings.Contains(out, "ERROR") && strings.Contains(out, "ORA-01920") {
		m.Status.CommonUsersCreated = true
	}
	return requeueN, sidbReadyPod
}

// #####################################################################################################
//
//	Check ORDS Health Status
//
// #####################################################################################################
func (r *OracleRestDataServiceReconciler) checkHealthStatus(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod) {
	log := r.Log.WithValues("checkHealthStatus", req.NamespacedName)

	readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, readyPod
	}
	if readyPod.Name == "" {
		m.Status.Status = dbcommons.StatusPending
		return requeueY, readyPod
	}

	// Get ORDS Status
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		dbcommons.GetORDSStatus)
	log.Info("GetORDSStatus Output")
	log.Info(out)
	if strings.Contains(strings.ToUpper(out), "ERROR") {
		return requeueY, readyPod
	}
	if err != nil {
		log.Info(err.Error())
		if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
			return requeueY, readyPod
		}
	}

	m.Status.Status = dbcommons.StatusUpdating
	if strings.Contains(out, "HTTP/1.1 200 OK") || strings.Contains(strings.ToUpper(err.Error()), "HTTP/1.1 200 OK") {
		if n.Status.Status == dbcommons.StatusReady || n.Status.Status == dbcommons.StatusUpdating || n.Status.Status == dbcommons.StatusPatching {
			m.Status.Status = dbcommons.StatusReady
		}
		if !m.Status.OrdsInstalled {
			m.Status.OrdsInstalled = true
			n.Status.OrdsReference = m.Name
			r.Status().Update(ctx, n)
			eventReason := "ORDS Installation"
			eventMsg := "installation of ORDS completed"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
				ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.OpenPDBSeed, dbcommons.SQLPlusCLI))
			if err != nil {
				log.Error(err, err.Error())
			} else {
				log.Info("Close PDB seed")
				log.Info(out)
			}
		}
	}
	if m.Status.Status == dbcommons.StatusUpdating {
		return requeueY, readyPod
	}
	return requeueN, readyPod
}

// #############################################################################
//
//	Instantiate Service spec from OracleRestDataService spec
//
// #############################################################################
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
			Ports: func() []corev1.ServicePort {
				ports := []corev1.ServicePort{
					{
						Name:     "client",
						Port:     8181,
						Protocol: corev1.ProtocolTCP,
					},
				}
				// Conditionally add MongoDB port if enabled
				if m.Spec.MongoDbApi {
					ports = append(ports, corev1.ServicePort{
						Name:     "mongodb",
						Port:     27017,
						Protocol: corev1.ProtocolTCP,
					})
				}
				return ports
			}(),
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

// #############################################################################
//
//	Instantiate POD spec from OracleRestDataService spec
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) instantiatePodSpec(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, req ctrl.Request) *corev1.Pod {

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
				if m.Spec.Persistence.Size == "" && n.Spec.Persistence.AccessMode == "ReadWriteOnce" {
					// Only allowing pods to be scheduled on the node where SIDB pods are running
					return &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{n.Name}, // Schedule on same host as DB Pod
									}},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
							},
						},
					}
				}
				return nil
			}(),
			Volumes: []corev1.Volume{
				{
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
				},
				{
					Name: "varmount",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			InitContainers: func() []corev1.Container {
				initContainers := []corev1.Container{}
				if m.Spec.Persistence.Size != "" && m.Spec.Persistence.SetWritePermissions != nil && *m.Spec.Persistence.SetWritePermissions {
					initContainers = append(initContainers, corev1.Container{
						Name:    "init-permissions",
						Image:   m.Spec.Image.PullFrom,
						Command: []string{"/bin/sh", "-c", fmt.Sprintf("chown %d:%d /etc/ords/config/ || true", int(dbcommons.ORACLE_UID), int(dbcommons.DBA_GUID))},
						SecurityContext: &corev1.SecurityContext{
							// User ID 0 means, root user
							RunAsUser: func() *int64 { i := int64(0); return &i }(),
						},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: "/etc/ords/config/",
							Name:      "datamount",
						}},
					})
				}

				initContainers = append(initContainers, corev1.Container{
					Name:    "init-ords",
					Image:   m.Spec.Image.PullFrom,
					Command: []string{"/bin/sh"},
					Args: []string{
						"-c",
						fmt.Sprintf("while [ ! -f /opt/oracle/variables/%s ]; do sleep 0.5; done", "conn_string.txt"),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/etc/ords/config/",
							Name:      "datamount",
						},
						{
							MountPath: "/opt/oracle/variables/",
							Name:      "varmount",
						},
					},
				})
				return initContainers
			}(),
			Containers: []corev1.Container{{
				Name:  m.Name,
				Image: m.Spec.Image.PullFrom,
				Ports: func() []corev1.ContainerPort {
					ports := []corev1.ContainerPort{
						{
							ContainerPort: 8181, // Default application port
						},
					}
					if m.Spec.MongoDbApi {
						ports = append(ports, corev1.ContainerPort{
							ContainerPort: 27017, // MongoDB port
						})
					}
					return ports
				}(),
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c", dbcommons.ORDSReadinessProbe},
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
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						MountPath: "/etc/ords/config/",
						Name:      "datamount",
					},
					{
						MountPath: "/opt/oracle/variables/",
						Name:      "varmount",
					},
				},
				Env: func() []corev1.EnvVar {
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
				FSGroup:    func() *int64 { i := int64(dbcommons.DBA_GUID); return &i }(),
			},

			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: m.Spec.Image.PullSecrets,
				},
			},
		},
	}

	// Set oracleRestDataService instance as the owner and controller
	// ctrl.SetControllerReference(m, initSecret, r.Scheme)
	ctrl.SetControllerReference(m, pod, r.Scheme)
	return pod
}

//#############################################################################
//    Instantiate POD spec from OracleRestDataService spec
//#############################################################################

// #############################################################################
//
//	Instantiate Persistent Volume Claim spec from SingleInstanceDatabase spec
//
// #############################################################################
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
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					// Requests describes the minimum amount of compute resources required
					"storage": resource.MustParse(m.Spec.Persistence.Size),
				},
			},
			StorageClassName: &m.Spec.Persistence.StorageClass,
			VolumeName:       m.Spec.Persistence.VolumeName,
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
//	Create a Service for OracleRestDataService
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createSVC(ctx context.Context, req ctrl.Request,
	m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) ctrl.Result {

	log := r.Log.WithValues("createSVC", req.NamespacedName)
	// Check if the Service already exists, if not create a new one
	svc := &corev1.Service{}
	svcDeleted := false
	// Check if the Service already exists, if not create a new one
	// Get retrieves an obj ( a struct pointer ) for the given object key from the Kubernetes Cluster.
	err := r.Get(ctx, types.NamespacedName{Name: m.Name, Namespace: m.Namespace}, svc)
	if err == nil {
		log.Info("Found Existing Service ", "Service.Name", svc.Name)
		svcType := corev1.ServiceType("NodePort")
		if m.Spec.LoadBalancer {
			svcType = corev1.ServiceType("LoadBalancer")
		}

		if svc.Spec.Type != svcType {
			log.Info("Deleting SVC", " name ", svc.Name)
			err = r.Delete(ctx, svc)
			if err != nil {
				r.Log.Error(err, "Failed to delete svc", " Name", svc.Name)
				return requeueN
			}
			svcDeleted = true
		}
	}

	if svcDeleted || (err != nil && apierrors.IsNotFound(err)) {
		// Define a new Service
		svc = r.instantiateSVCSpec(m)
		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY
		} else {
			eventReason := "Service creation"
			eventMsg := "successfully created service type " + string(svc.Spec.Type)
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			log.Info(eventMsg)
		}

	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return requeueY
	}

	m.Status.ServiceIP = ""
	if m.Spec.LoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			// 'lbAddress' will contain the Fully Qualified Hostname of the LB. If the hostname is not available it will contain the IP address of the LB
			lbAddress := svc.Status.LoadBalancer.Ingress[0].Hostname
			if lbAddress == "" {
				lbAddress = svc.Status.LoadBalancer.Ingress[0].IP
			}
			m.Status.DatabaseApiUrl = "http://" + lbAddress + ":" +
				fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/" + "{schema-name}" + "/_/db-api/stable/"
			m.Status.ServiceIP = lbAddress
			m.Status.DatabaseActionsUrl = "http://" + lbAddress + ":" +
				fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/sql-developer"
			if m.Status.ApexConfigured {
				m.Status.ApxeUrl = "http://" + lbAddress + ":" +
					fmt.Sprint(svc.Spec.Ports[0].Port) + "/ords/apex"
			}
			if m.Status.MongoDbApi && len(svc.Spec.Ports) > 1 {
				m.Status.MongoDbApiAccessUrl = "mongodb://[{user}:{password}@]" + lbAddress + ":" +
					fmt.Sprint(svc.Spec.Ports[1].Port) + "/{user}?" +
					"authMechanism=PLAIN&authSource=$external&ssl=true&retryWrites=false&loadBalanced=true"
			} else {
				m.Status.MongoDbApiAccessUrl = ""
			}
		}
		return requeueN
	}
	nodeip := dbcommons.GetNodeIp(r, ctx, req)
	if nodeip != "" {
		m.Status.ServiceIP = nodeip
		m.Status.DatabaseApiUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) +
			"/ords/" + "{schema-name}" + "/_/db-api/stable/"
		m.Status.DatabaseActionsUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) +
			"/ords/sql-developer"
		if m.Status.ApexConfigured {
			m.Status.ApxeUrl = "http://" + nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/ords/apex"
		}
		if m.Status.MongoDbApi && len(svc.Spec.Ports) > 1 {
			m.Status.MongoDbApiAccessUrl = "mongodb://[{user}:{password}@]" + nodeip + ":" +
				fmt.Sprint(svc.Spec.Ports[1].NodePort) + "/{user}?" +
				"authMechanism=PLAIN&authSource=$external&ssl=true&retryWrites=false&loadBalanced=true"
		} else {
			m.Status.MongoDbApiAccessUrl = ""
		}
	}
	return requeueN
}

// #############################################################################
//
//	Stake a claim for Persistent Volume
//
// #############################################################################
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

// #############################################################################
//
//	Function for creating connection sting file
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createConnectionString(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

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
		// Init container named "init-ords" has completed it's execution, hence return and don't requeue
		return requeueN, nil
	}

	if pod.Status.InitContainerStatuses[lastInitContIndex].State.Running == nil {
		// Init container named "init-ords" is not running, so waiting for it to come in running state requeueing the reconcile request
		r.Log.Info("Waiting for init-ords to come in running state...")
		return requeueY, nil
	}

	r.Log.Info("Creating Connection String file...")

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

	// Execing into the pods and creating the Connection String
	adminPassword := string(secret.Data[m.Spec.AdminPassword.SecretKey])

	_, err = dbcommons.ExecCommand(r, r.Config, pod.Name, pod.Namespace, "init-ords",
		ctx, req, true, "bash", "-c",
		fmt.Sprintf("mkdir -p /opt/oracle/variables && echo %[1]s > /opt/oracle/variables/%[2]s",
			fmt.Sprintf(dbcommons.DbConnectString, adminPassword, n.Name, n.Status.Pdbname),
			"conn_string.txt"))

	if err != nil {
		r.Log.Error(err, err.Error())
		r.Log.Error(err, "Failed to create connection string in new "+m.Name+" POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
		return requeueY, nil
	}
	r.Log.Info("Succesfully Created connection string in new "+m.Name+" POD", "POD.NAME : ", pod.Name)

	return requeueN, nil
}

// #############################################################################
//
//	Create the requested POD replicas
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) createPods(m *dbapi.OracleRestDataService,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("createPods", req.NamespacedName)

	readyPod, replicasFound, available, podsMarkedToBeDeleted, err := dbcommons.FindPods(r, m.Spec.Image.Version,
		m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// Recreate new pods only after earlier pods are terminated completely
	for i := 0; i < len(podsMarkedToBeDeleted); i++ {
		r.Log.Info("Force deleting pod ", "name", podsMarkedToBeDeleted[i].Name, "phase", podsMarkedToBeDeleted[i].Status.Phase)
		var gracePeriodSeconds int64 = 0
		policy := metav1.DeletePropagationForeground
		r.Delete(ctx, &podsMarkedToBeDeleted[i], &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
	}

	log.Info(m.Name, " pods other than one of Ready Pods : ", dbcommons.GetPodNames(available))
	log.Info(m.Name, " Ready Pod : ", readyPod.Name)

	replicasReq := m.Spec.Replicas
	if replicasFound == 0 {
		m.Status.Status = dbcommons.StatusPending
	}

	if replicasFound == replicasReq {
		log.Info("No of " + m.Name + " replicas Found are same as Required")
	} else if replicasFound < replicasReq {
		// Create New Pods , Name of Pods are generated Randomly
		for i := replicasFound; i < replicasReq; i++ {
			// Obtain admin password of the referred database
			adminPasswordSecret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{Name: n.Spec.AdminPassword.SecretName, Namespace: n.Namespace}, adminPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					m.Status.Status = dbcommons.StatusError
					eventReason := "Database Password"
					eventMsg := "password secret " + m.Spec.AdminPassword.SecretName + " not found, retrying..."
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
					r.Log.Info(eventMsg)
					return requeueY
				}
				log.Error(err, err.Error())
				return requeueY
			}

			pod := r.instantiatePodSpec(m, n, req)

			log.Info("Creating a new "+m.Name+" POD", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
			err = r.Create(ctx, pod)
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
			var gracePeriodSeconds int64 = 0
			policy := metav1.DeletePropagationForeground
			err := r.Delete(ctx, &pod, &client.DeleteOptions{
				GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
			noDeleted += 1
			if err != nil {
				r.Log.Error(err, "Failed to delete existing POD", "POD.Name", pod.Name)
				// Don't requeue
			}
		}
	}

	// Creating conn string in pods
	result, err := r.createConnectionString(m, n, ctx, req)

	if err != nil {
		return requeueY
	}
	if result.Requeue {
		log.Info("Requeued at connection string creation")
		return requeueY
	}

	m.Status.Replicas = m.Spec.Replicas

	return requeueN
}

// #############################################################################
//
//	Manage Finalizer to cleanup before deletion of OracleRestDataService
//
// #############################################################################
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
				log.Info("Clearing the OrdsReference from DB", "name", n.Name)
				err := r.Status().Update(ctx, n)
				if err != nil {
					log.Error(err, err.Error())
					time.Sleep(1 * time.Second)
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

// #############################################################################
//
//	Finalization logic for OracleRestDataServiceFinalizer
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) cleanupOracleRestDataService(req ctrl.Request, ctx context.Context,
	m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase) error {
	log := r.Log.WithValues("cleanupOracleRestDataService", req.NamespacedName)

	if m.Status.OrdsInstalled {
		// ## FETCH THE SIDB REPLICAS .
		sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
			n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}

		if sidbReadyPod.Name == "" {
			eventReason := "ORDS Uninstallation"
			eventMsg := "skipping ORDS uninstallation as no ready pod for " + n.Name + " is available"
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

		// Fetch admin Password of database to uninstall ORDS
		adminPasswordSecret := &corev1.Secret{}
		adminPasswordSecretFound := false
		for i := 0; i < 5; i++ {
			err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: n.Namespace}, adminPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					m.Status.Status = dbcommons.StatusError
					eventReason := "Error"
					eventMsg := "database admin password secret " + m.Spec.AdminPassword.SecretName + " required for ORDS uninstall not found, retrying..."
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
					r.Log.Info(eventMsg)
					if i < 4 {
						time.Sleep(15 * time.Second)
						continue
					}
				} else {
					log.Error(err, err.Error())
				}
			} else {
				adminPasswordSecretFound = true
				break
			}
		}
		// Find ORDS ready pod
		readyPod, _, _, _, err := dbcommons.FindPods(r, m.Spec.Image.Version,
			m.Spec.Image.PullFrom, m.Name, m.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		if adminPasswordSecretFound && readyPod.Name != "" {
			adminPassword := string(adminPasswordSecret.Data[m.Spec.AdminPassword.SecretKey])
			if n.Status.ApexInstalled {
				//Uninstall Apex
				eventReason := "Apex Uninstallation"
				eventMsg := "Uninstalling Apex..."
				r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
				log.Info(eventMsg)
				out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
					fmt.Sprintf(dbcommons.UninstallApex, adminPassword, n.Status.Pdbname))
				if err != nil {
					log.Info(err.Error())
				}
				n.Status.ApexInstalled = false // To reinstall Apex when ORDS is reinstalled
				log.Info("Apex uninstall output: " + out)
			}
			//Uninstall ORDS
			eventReason := "ORDS Uninstallation"
			eventMsg := "Uninstalling ORDS..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			log.Info(eventMsg)
			uninstallORDS := fmt.Sprintf(dbcommons.UninstallORDSCMD, adminPassword)
			out, err = dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "", ctx, req, true, "bash", "-c",
				uninstallORDS)
			log.Info("ORDS uninstall output: " + out)
			if strings.Contains(strings.ToUpper(out), "ERROR") {
				return errors.New(out)
			}
			if err != nil {
				log.Info(err.Error())
			}
		}

		// Drop Admin Users
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s ", dbcommons.DropAdminUsersSQL, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Info(err.Error())
		}
		log.Info("Drop admin users: " + out)

		//Delete ORDS pod
		var gracePeriodSeconds int64 = 0
		policy := metav1.DeletePropagationForeground
		r.Delete(ctx, &readyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})

		//Delete Database Admin Password Secret
		if !*m.Spec.AdminPassword.KeepSecret {
			err = r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				r.Log.Info("Deleted Admin Password Secret :" + adminPasswordSecret.Name)
			}
		}
	}

	// Cleanup steps that the operator needs to do before the CR can be deleted.
	log.Info("Successfully cleaned up OracleRestDataService ")
	return nil
}

// #############################################################################
//
//	Configure APEX
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) configureApex(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("verifyApex", req.NamespacedName)

	if m.Status.ApexConfigured {
		return requeueN
	}

	// Obtain admin password of the referred database

	adminPasswordSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			m.Status.Status = dbcommons.StatusError
			eventReason := "Database Password"
			eventMsg := "password secret " + m.Spec.AdminPassword.SecretName + " not found, retrying..."
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			r.Log.Info(eventMsg)
			return requeueY
		}
		log.Error(err, err.Error())
		return requeueY
	}
	sidbPassword := string(adminPasswordSecret.Data[m.Spec.AdminPassword.SecretKey])

	// Checking if Apex is installed successfully or not
	out, err := dbcommons.ExecCommand(r, r.Config, ordsReadyPod.Name, ordsReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.IsApexInstalled, sidbPassword, n.Status.Pdbname))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("Is Apex installed: \n" + out)

	apexInstalled := "APEXVERSION:"
	if !strings.Contains(out, apexInstalled) {
		eventReason := "Apex Verification"
		eventMsg := "Unable to determine Apex version, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY
	}

	m.Status.Status = dbcommons.StatusReady
	eventReason := "Apex Verification"
	outArr := strings.Split(out, apexInstalled)
	eventMsg := "Verification of Apex " + strings.TrimSpace(outArr[len(outArr)-1]) + " completed"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
	n.Status.ApexInstalled = true
	m.Status.ApexConfigured = true
	r.Status().Update(ctx, n)
	r.Status().Update(ctx, m)

	return requeueN
}

// #############################################################################
//
//	Delete Secrets
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) deleteSecrets(m *dbapi.OracleRestDataService, ctx context.Context, req ctrl.Request) {
	log := r.Log.WithValues("deleteSecrets", req.NamespacedName)

	if !*m.Spec.AdminPassword.KeepSecret {
		// Fetch adminPassword Secret
		adminPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.AdminPassword.SecretName, Namespace: m.Namespace}, adminPasswordSecret)
		if err == nil {
			//Delete Database Admin Password Secret .
			err := r.Delete(ctx, adminPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("Database admin password secret deleted : " + adminPasswordSecret.Name)
			}
		}
	}

	if !*m.Spec.OrdsPassword.KeepSecret {
		// Fetch ordsPassword Secret
		ordsPasswordSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: m.Spec.OrdsPassword.SecretName, Namespace: m.Namespace}, ordsPasswordSecret)
		if err == nil {
			//Delete ORDS Password Secret .
			err := r.Delete(ctx, ordsPasswordSecret, &client.DeleteOptions{})
			if err == nil {
				log.Info("ORDS password secret deleted : " + ordsPasswordSecret.Name)
			}
		}
	}
}

// #############################################################################
//
//	Enable MongoDB API Support
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) enableMongoDB(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("enableMongoDB", req.NamespacedName)

	if (m.Spec.MongoDbApi && !m.Status.MongoDbApi) || // setting MongoDbApi to true
		(!m.Spec.MongoDbApi && m.Status.MongoDbApi) { // setting MongoDbApi to false
		m.Status.Status = dbcommons.StatusUpdating

		out, err := dbcommons.ExecCommand(r, r.Config, ordsReadyPod.Name, ordsReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf(dbcommons.ConfigMongoDb, strconv.FormatBool(m.Spec.MongoDbApi)))
		log.Info("configMongoDB Output: \n" + out)

		if strings.Contains(strings.ToUpper(out), "ERROR") {
			return requeueY
		}
		if err != nil {
			log.Info(err.Error())
			if strings.Contains(strings.ToUpper(err.Error()), "ERROR") {
				return requeueY
			}
		}

		m.Status.MongoDbApi = m.Spec.MongoDbApi
		m.Status.Status = dbcommons.StatusReady
		r.Status().Update(ctx, m)
		eventReason := "MongoDB-API Config"
		eventMsg := "configuration of MongoDb API completed!"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		log.Info(eventMsg)

		// ORDS service is resatrted
		r.Log.Info("Restarting ORDS Service : " + m.Name)
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace},
		}
		var gracePeriodSeconds int64 = 0
		policy := metav1.DeletePropagationForeground
		err = r.Delete(ctx, svc, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, "Failed to delete ORDS service", "Service Name", m.Name)
			return requeueY
		}

		// ORDS needs to be restarted to configure MongoDB API
		r.Log.Info("Restarting ORDS Pod after configuring MongoDb API : " + ordsReadyPod.Name)
		err = r.Delete(ctx, &ordsReadyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, err.Error())
		}
		return requeueY

	} else {
		log.Info("MongoDB Already Configured")
	}

	return requeueN
}

// #############################################################################
//
//	Rest Enable/Disable Schemas
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) restEnableSchemas(m *dbapi.OracleRestDataService, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, ordsReadyPod corev1.Pod, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("restEnableSchemas", req.NamespacedName)

	if sidbReadyPod.Name == "" || n.Status.Status != dbcommons.StatusReady {
		eventReason := "Database Check"
		eventMsg := "status of database " + n.Name + " is not ready, retrying..."
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		m.Status.Status = dbcommons.StatusNotReady
		return requeueY
	}

	// Get available PDBs
	availablePDBS, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "",
		ctx, req, true, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | %s", dbcommons.GetPdbsSQL, dbcommons.SQLPlusCLI))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	} else {
		log.Info("PDBs found:")
		log.Info(availablePDBS)
	}

	restartORDS := false

	for i := 0; i < len(m.Spec.RestEnableSchemas); i++ {

		pdbName := m.Spec.RestEnableSchemas[i].PdbName
		if pdbName == "" {
			pdbName = n.Spec.Pdbname
		}

		//  If the PDB mentioned in yaml doesnt contain in the database , continue
		if !strings.Contains(strings.ToUpper(availablePDBS), strings.ToUpper(pdbName)) {
			eventReason := "PDB Check"
			eventMsg := "PDB " + pdbName + " not found for specified schema " + m.Spec.RestEnableSchemas[i].SchemaName
			log.Info(eventMsg)
			r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
			continue
		}

		getOrdsSchemaStatus := fmt.Sprintf(dbcommons.GetUserORDSSchemaStatusSQL, m.Spec.RestEnableSchemas[i].SchemaName, pdbName)

		// Get ORDS Schema status for PDB
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", getOrdsSchemaStatus, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}

		// if ORDS already enabled for given PDB
		if strings.Contains(out, "STATUS:ENABLED") {
			if m.Spec.RestEnableSchemas[i].Enable {
				log.Info("Schema already enabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
				continue
			}
		} else if strings.Contains(out, "STATUS:DISABLED") {
			if !m.Spec.RestEnableSchemas[i].Enable {
				log.Info("Schema already disabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
				continue
			}
		} else if m.Spec.RestEnableSchemas[i].Enable {
			OrdsPasswordSecret := &corev1.Secret{}
			// Fetch the secret to get password for database user . Secret has to be created in the same namespace of OracleRestDataService
			err = r.Get(ctx, types.NamespacedName{Name: m.Spec.OrdsPassword.SecretName, Namespace: m.Namespace}, OrdsPasswordSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					eventReason := "No Secret"
					eventMsg := "secret " + m.Spec.OrdsPassword.SecretName + " Not Found"
					r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
					r.Log.Info(eventMsg)
					return requeueY
				}
				log.Error(err, err.Error())
				return requeueY
			}
			password := string(OrdsPasswordSecret.Data[m.Spec.OrdsPassword.SecretKey])
			// Create users,schemas and grant enableORDS for PDB
			createSchemaSQL := fmt.Sprintf(dbcommons.CreateORDSSchemaSQL, m.Spec.RestEnableSchemas[i].SchemaName, password, pdbName)
			log.Info("Creating schema", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			_, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
				fmt.Sprintf("echo -e  \"%s\"  | %s", createSchemaSQL, dbcommons.SQLPlusCLI))
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
		} else {
			log.Info("Noop, ignoring", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			continue
		}
		urlMappingPattern := ""
		if m.Spec.RestEnableSchemas[i].UrlMapping == "" {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].SchemaName)
		} else {
			urlMappingPattern = strings.ToLower(m.Spec.RestEnableSchemas[i].UrlMapping)
		}
		enableORDSSchema := fmt.Sprintf(dbcommons.EnableORDSSchemaSQL, m.Spec.RestEnableSchemas[i].SchemaName,
			strconv.FormatBool(m.Spec.RestEnableSchemas[i].Enable), urlMappingPattern, pdbName)

		// EnableORDS for Schema
		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", enableORDSSchema, dbcommons.SQLPlusCLI))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info(out)
		if m.Spec.RestEnableSchemas[i].Enable {
			log.Info("REST Enabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
		} else {
			log.Info("REST Disabled", "schema", m.Spec.RestEnableSchemas[i].SchemaName)
			restartORDS = true
		}
	}

	if restartORDS {
		r.Log.Info("Restarting ORDS Pod " + ordsReadyPod.Name + " to clear disabled schemas cache")
		var gracePeriodSeconds int64 = 0
		policy := metav1.DeletePropagationForeground
		err = r.Delete(ctx, &ordsReadyPod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds, PropagationPolicy: &policy})
		if err != nil {
			r.Log.Error(err, err.Error())
		}
		return requeueY
	}
	return requeueN
}

// #############################################################################
//
//	SetupWithManager sets up the controller with the Manager.
//
// #############################################################################
func (r *OracleRestDataServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.OracleRestDataService{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of OracleRestDataService Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
