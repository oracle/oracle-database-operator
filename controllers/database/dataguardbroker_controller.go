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
	"strings"
	"time"

	"github.com/go-logr/logr"
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

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
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
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services;nodes;events,verbs=create;delete;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataguardBroker object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DataguardBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	r.Log.Info("Reconcile requested")

	dataguardBroker := &dbapi.DataguardBroker{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, dataguardBroker)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted")
			return requeueN, nil
		}
		return requeueN, err
	}

	// Manage SingleInstanceDatabase Deletion
	result, err := r.manageDataguardBrokerDeletion(req, ctx, dataguardBroker)
	if result.Requeue || err != nil {
		r.Log.Info("Reconcile queued")
		return result, err
	}

	// Fetch Primary Database Reference
	singleInstanceDatabase := &dbapi.SingleInstanceDatabase{}
	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: dataguardBroker.Spec.PrimaryDatabaseRef}, singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted")
			return requeueN, nil
		}
		return requeueN, err
	}

	/* Initialize Status */
	if dataguardBroker.Status.Status == "" {
		dataguardBroker.Status.Status = dbcommons.StatusCreating
		dataguardBroker.Status.ExternalConnectString = dbcommons.ValueUnavailable
		dataguardBroker.Status.ClusterConnectString = dbcommons.ValueUnavailable
		r.Status().Update(ctx, dataguardBroker)
	}

	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, dataguardBroker)

	// Create Service to point to primary database always
	result = r.createSVC(ctx, req, dataguardBroker)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Validate if Primary Database Reference is ready
	result, sidbReadyPod, adminPassword := r.validateSidbReadiness(dataguardBroker, singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Setup the DG Configuration
	result = r.setupDataguardBrokerConfiguration(dataguardBroker, singleInstanceDatabase, sidbReadyPod, adminPassword, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Set a particular database as primary
	result = r.SetAsPrimaryDatabase(singleInstanceDatabase.Spec.Sid, dataguardBroker.Spec.SetAsPrimaryDatabase, dataguardBroker,
		singleInstanceDatabase, adminPassword, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// If LoadBalancer = true , ensure Connect String is updated
	if dataguardBroker.Status.ExternalConnectString == dbcommons.ValueUnavailable {
		return requeueY, nil
	}

	dataguardBroker.Status.Status = dbcommons.StatusReady

	r.Log.Info("Reconcile completed")
	return ctrl.Result{}, nil

}

// #####################################################################################################
//
//	Validate Readiness of the primary DB specified
//
// #####################################################################################################
func (r *DataguardBrokerReconciler) validateSidbReadiness(m *dbapi.DataguardBroker,
	n *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) (ctrl.Result, corev1.Pod, string) {

	log := r.Log.WithValues("validateSidbReadiness", req.NamespacedName)
	adminPassword := ""
	// ## FETCH THE SIDB REPLICAS .
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
		n.Spec.Image.PullFrom, n.Name, n.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod, adminPassword
	}

	if n.Status.Status != dbcommons.StatusReady {

		eventReason := "Waiting"
		eventMsg := "Waiting for " + n.Name + " to be Ready"
		r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
		return requeueY, sidbReadyPod, adminPassword
	}

	// Validate databaseRef Admin Password
	adminPasswordSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: n.Spec.AdminPassword.SecretName, Namespace: n.Namespace}, adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			//m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for secret : " + n.Spec.AdminPassword.SecretName + " to get created"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + n.Spec.AdminPassword.SecretName + " Not Found")
			return requeueY, sidbReadyPod, adminPassword
		}
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod, adminPassword
	}
	adminPassword = string(adminPasswordSecret.Data[n.Spec.AdminPassword.SecretKey])

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.ValidateAdminPassword, adminPassword), dbcommons.GetSqlClient(n.Spec.Edition)))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, sidbReadyPod, adminPassword
	}
	if strings.Contains(out, "USER is \"SYS\"") {
		log.Info("validated Admin password successfully")
	} else if strings.Contains(out, "ORA-01017") {
		//m.Status.Status = dbcommons.StatusError
		eventReason := "Logon denied"
		eventMsg := "invalid databaseRef admin password. secret: " + n.Spec.AdminPassword.SecretName
		r.Recorder.Eventf(m, corev1.EventTypeWarning, eventReason, eventMsg)
		return requeueY, sidbReadyPod, adminPassword
	} else {
		return requeueY, sidbReadyPod, adminPassword
	}

	return requeueN, sidbReadyPod, adminPassword
}

// #############################################################################
//
//	Instantiate Service spec from StandbyDatabase spec
//
// #############################################################################
func (r *DataguardBrokerReconciler) instantiateSVCSpec(m *dbapi.DataguardBroker) *corev1.Service {
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
					Name:     "listener",
					Port:     1521,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     "xmldb",
					Port:     5500,
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

// #############################################################################
//
//	Create a Service for StandbyDatabase
//
// #############################################################################
func (r *DataguardBrokerReconciler) createSVC(ctx context.Context, req ctrl.Request,
	m *dbapi.DataguardBroker) ctrl.Result {

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
		//err = r.Update(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return requeueY
		} else {
			timeout := 30
			// Waiting for Service to get created as sometimes it takes some time to create a service . 30 seconds TImeout
			err = dbcommons.WaitForStatusChange(r, svc.Name, m.Namespace, ctx, req, time.Duration(timeout)*time.Second, "svc", "creation")
			if err != nil {
				log.Error(err, "Error in Waiting for svc status for Creation", "svc.Namespace", svc.Namespace, "SVC.Name", svc.Name)
				return requeueY
			}
			log.Info("Succesfully Created New Service ", "Service.Name : ", svc.Name)
		}
		time.Sleep(10 * time.Second)

	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return requeueY
	} else if err == nil {
		log.Info(" ", "Found Existing Service ", svc.Name)
	}

	// update service status
	log.Info("Updating the service status...")
	m.Status.ClusterConnectString = svc.Name + "." + svc.Namespace + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
	if m.Spec.LoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			lbAddress := svc.Status.LoadBalancer.Ingress[0].Hostname
			if lbAddress == "" {
				lbAddress = svc.Status.LoadBalancer.Ingress[0].IP
			}
			m.Status.ExternalConnectString = lbAddress + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
		}
	} else {
		nodeip := dbcommons.GetNodeIp(r, ctx, req)
		if nodeip != "" {
			m.Status.ExternalConnectString = nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/DATAGUARD"
		}
	}
	r.Status().Update(ctx, m)

	return requeueN
}

// #############################################################################
//
//	Setup the requested DG Configuration
//
// #############################################################################
func (r *DataguardBrokerReconciler) setupDataguardBrokerConfiguration(m *dbapi.DataguardBroker, n *dbapi.SingleInstanceDatabase,
	sidbReadyPod corev1.Pod, adminPassword string, ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("setupDataguardBrokerConfiguration", req.NamespacedName)

	for i := 0; i < len(m.Spec.StandbyDatabaseRefs); i++ {

		standbyDatabase := &dbapi.SingleInstanceDatabase{}
		err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.StandbyDatabaseRefs[i]}, standbyDatabase)
		if err != nil {
			if apierrors.IsNotFound(err) {
				eventReason := "Warning"
				eventMsg := m.Spec.StandbyDatabaseRefs[i] + "not found"
				r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
				continue
			}
			log.Error(err, err.Error())
			return requeueY
		}

		// Check if dataguard broker is already configured for the standby database
		if standbyDatabase.Status.DgBrokerConfigured {
			log.Info("Dataguard broker for standbyDatabase : " + standbyDatabase.Name + " is already configured")
			continue
		}

		m.Status.Status = dbcommons.StatusCreating
		r.Status().Update(ctx, m)

		// ## FETCH THE STANDBY REPLICAS .
		standbyDatabaseReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
			n.Spec.Image.PullFrom, standbyDatabase.Name, standbyDatabase.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}

		if standbyDatabase.Status.Status != dbcommons.StatusReady {

			eventReason := "Waiting"
			eventMsg := "Waiting for " + standbyDatabase.Name + " to be Ready"
			r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)
			return requeueY

		}

		result := r.setupDataguardBrokerConfigurationForGivenDB(m, n, standbyDatabase, standbyDatabaseReadyPod, sidbReadyPod, ctx, req, adminPassword)
		if result.Requeue {
			return result
		}

		// Update Databases
		r.updateReconcileStatus(m, sidbReadyPod, ctx, req)
	}

	eventReason := "DG Configuration up to date"
	eventMsg := ""

	// Patch DataguardBroker Service to point selector to Current Primary Name
	result := r.patchService(m, sidbReadyPod, n, ctx, req)
	if result.Requeue {
		return result
	}

	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	return requeueN
}

// #############################################################################
//
//	Patch DataguardBroker Service to point selector to Current Primary Name
//
// #############################################################################
func (r *DataguardBrokerReconciler) patchService(m *dbapi.DataguardBroker, sidbReadyPod corev1.Pod, n *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request) ctrl.Result {
	log := r.Log.WithValues("patchService", req.NamespacedName)
	databases, out, err := dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	if !strings.Contains(out, "ORA-") {
		primarySid := strings.ToUpper(dbcommons.GetPrimaryDatabase(databases))
		primaryName := n.Name
		if primarySid != n.Spec.Sid {
			primaryName = n.Status.StandbyDatabases[primarySid]
		}

		// Patch DataguardBroker Service to point selector to Current Primary Name
		svc := &corev1.Service{}
		err = r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, svc)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		svc.Spec.Selector["app"] = primaryName
		err = r.Update(ctx, svc)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}

		m.Status.ClusterConnectString = svc.Name + "." + svc.Namespace + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
		if m.Spec.LoadBalancer {
			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				lbAddress := svc.Status.LoadBalancer.Ingress[0].Hostname
				if lbAddress == "" {
					lbAddress = svc.Status.LoadBalancer.Ingress[0].IP
				}
				m.Status.ExternalConnectString = lbAddress + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
			}
		} else {
			nodeip := dbcommons.GetNodeIp(r, ctx, req)
			if nodeip != "" {
				m.Status.ExternalConnectString = nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/DATAGUARD"
			}
		}
	}
	return requeueN
}

// #############################################################################
//
//	Set up DG Configuration for a given StandbyDatabase
//
// #############################################################################
func (r *DataguardBrokerReconciler) setupDataguardBrokerConfigurationForGivenDB(m *dbapi.DataguardBroker, n *dbapi.SingleInstanceDatabase, standbyDatabase *dbapi.SingleInstanceDatabase,
	standbyDatabaseReadyPod corev1.Pod, sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request, adminPassword string) ctrl.Result {

	log := r.Log.WithValues("setupDataguardBrokerConfigurationForGivenDB", req.NamespacedName)

	if standbyDatabaseReadyPod.Name == "" || sidbReadyPod.Name == "" {
		return requeueY
	}

	// ## CHECK IF DG CONFIGURATION AVAILABLE IN PRIMARY DATABSE##
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.DBShowConfigCMD))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("ShowConfiguration Output")
	log.Info(out)

	if strings.Contains(out, "ORA-16525") {
		log.Info("ORA-16525: The Oracle Data Guard broker is not yet available on Primary")
		return requeueY
	}

	_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("DB Admin pwd file created")

	if strings.Contains(out, "ORA-16532") {
		//  ORA-16532: Oracle Data Guard broker configuration does not exist , so create one
		if m.Spec.ProtectionMode == "MaxPerformance" {
			// Construct the password file and dgbroker command file
			out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerMaxPerformanceCMD))
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
			log.Info("DGMGRL command file creation output")
			log.Info(out)

			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
			out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				"dgmgrl sys@${PRIMARY_DB_CONN_STR} @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd")
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
			log.Info("DgConfigurationMaxPerformance Output")
			log.Info(out)

		} else if m.Spec.ProtectionMode == "MaxAvailability" {
			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAX AVAILABILITY ##
			out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerMaxAvailabilityCMD))
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
			log.Info("DGMGRL command file creation output")
			log.Info(out)

			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
			out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				"dgmgrl sys@${PRIMARY_DB_CONN_STR} @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd")
			if err != nil {
				log.Error(err, err.Error())
				return requeueY
			}
			log.Info("DgConfigurationMaxAvailability Output")
			log.Info(out)

		} else {
			log.Info("SPECIFY correct Protection Mode . Either MaxAvailability or MaxPerformance")
			return requeueY
		}

		// ## SHOW CONFIGURATION DG
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.DBShowConfigCMD))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		} else {
			log.Info("ShowConfiguration Output")
			log.Info(out)
		}
		// Set DG Configured status to true for this standbyDatabase. so that in next reconcilation, we dont configure this again
		standbyDatabase.Status.DgBrokerConfigured = true
		r.Status().Update(ctx, standbyDatabase)

		// Remove admin pwd file
		_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			dbcommons.RemoveAdminPasswordFile)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("DB Admin pwd file removed")

		return requeueN
	}

	// DG Configuration Exists . So add the standbyDatabase to the existing DG Configuration
	databases, _, err := dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// ## ADD DATABASE TO DG CONFIG , IF NOT PRESENT
	found, _ := dbcommons.IsDatabaseFound(standbyDatabase.Spec.Sid, databases, "")
	if found {
		return requeueN
	}
	primarySid := dbcommons.GetPrimaryDatabase(databases)

	// If user adds a new standby to a dg config when failover happened to one ot the standbys, we need to have current primary connect string
	primaryConnectString := n.Name + ":1521/" + primarySid
	if !strings.EqualFold(primarySid, n.Spec.Sid) {
		primaryConnectString = n.Status.StandbyDatabases[primarySid] + ":1521/" + primarySid
	}

	if m.Spec.ProtectionMode == "MaxPerformance" {
		// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerAddDBMaxPerformanceCMD))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("DGMGRL command file creation output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd ", primaryConnectString))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("DgConfigurationMaxPerformance Output")
		log.Info(out)

	} else if m.Spec.ProtectionMode == "MaxAvailability" {
		// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAX AVAILABILITY ##
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerAddDBMaxAvailabilityCMD))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("DGMGRL command file creation output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd ", primaryConnectString))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("DgConfigurationMaxAvailability Output")
		log.Info(out)

	} else {
		log.Info("SPECIFY correct Protection Mode . Either MaxAvailability or MaxPerformance")
		log.Error(err, err.Error())
		return requeueY
	}

	databases, _, err = dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// ## SET PROPERTY FASTSTARTFAILOVERTARGET FOR EACH DATABASE TO ALL OTHER DATABASES IN DG CONFIG .
	for i := 0; i < len(databases); i++ {
		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s \"EDIT DATABASE %s SET PROPERTY FASTSTARTFAILOVERTARGET=%s\"< admin.pwd", primaryConnectString,
				strings.Split(databases[i], ":")[0], getFSFOTargets(i, databases)))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("SETTING FSFO TARGET OUTPUT")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s \"SHOW DATABASE %s FASTSTARTFAILOVERTARGET\" < admin.pwd", primaryConnectString, strings.Split(databases[i], ":")[0]))
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
		log.Info("FSFO TARGETS OF " + databases[i])
		log.Info(out)

	}
	// Remove admin pwd file
	_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		dbcommons.RemoveAdminPasswordFile)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("DB Admin pwd file removed")

	// Set DG Configured status to true for this standbyDatabase. so that in next reconcilation, we dont configure this again
	standbyDatabase.Status.DgBrokerConfigured = true
	r.Status().Update(ctx, standbyDatabase)

	return requeueN
}

// #############################################################################
//
//	Return FSFO targets of each StandbyDatabase
//	Concatenation of all strings in databases slice expecting that of index 1
//
// #############################################################################
func getFSFOTargets(index int, databases []string) string {
	fsfotargets := ""
	for i := 0; i < len(databases); i++ {
		if i != index {
			splitstr := strings.Split(databases[i], ":")
			if fsfotargets == "" {
				fsfotargets = splitstr[0]
			} else {
				fsfotargets = fsfotargets + "," + splitstr[0]
			}
		}
	}
	return fsfotargets
}

// #####################################################################################################
//
//	Switchovers to 'sid' db to make 'sid' db primary
//
// #####################################################################################################
func (r *DataguardBrokerReconciler) SetAsPrimaryDatabase(sidbSid string, targetSid string, m *dbapi.DataguardBroker, n *dbapi.SingleInstanceDatabase,
	adminPassword string, ctx context.Context, req ctrl.Request) ctrl.Result {

	log := r.Log.WithValues("SetAsPrimaryDatabase", req.NamespacedName)
	if targetSid == "" {
		log.Info("Specified sid is nil")
		return requeueN
	}

	// Fetch the SIDB Ready Pod
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, n.Spec.Image.Version,
		(n.Spec.Image.PullFrom), n.Name, n.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// Fetch databases in dataguard broker configuration
	databases, _, err := dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}

	// Fetch the current Primary database
	primarySid := dbcommons.GetPrimaryDatabase(databases)
	if strings.EqualFold(primarySid, targetSid) {
		log.Info(targetSid + " is already Primary")
		return requeueN
	}

	m.Status.Status = dbcommons.StatusUpdating
	r.Status().Update(ctx, m)

	found, _ := dbcommons.IsDatabaseFound(targetSid, databases, "")
	if !found {
		log.Info(targetSid + " not yet set in DG config")
		return requeueY
	}

	// Fetch the PrimarySid Ready Pod to create chk file
	var primaryReq ctrl.Request
	var primaryReadyPod corev1.Pod
	if !strings.EqualFold(primarySid, sidbSid) {
		primaryReq = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: req.Namespace,
				Name:      n.Status.StandbyDatabases[strings.ToUpper(primarySid)],
			},
		}
		primaryReadyPod, _, _, _, err = dbcommons.FindPods(r, "", "", primaryReq.Name, primaryReq.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
	} else {
		primaryReadyPod = sidbReadyPod
	}

	// Fetch the targetSid Ready Pod to create chk file
	var targetReq ctrl.Request
	var targetReadyPod corev1.Pod
	if !strings.EqualFold(targetSid, sidbSid) {
		targetReq = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: req.Namespace,
				Name:      n.Status.StandbyDatabases[strings.ToUpper(targetSid)],
			},
		}
		targetReadyPod, _, _, _, err = dbcommons.FindPods(r, "", "", targetReq.Name, targetReq.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY
		}
	} else {
		targetReadyPod = sidbReadyPod
	}

	// Create a chk File so that no other pods take the lock during Switchover .
	out, err := dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("Successfully Created chk file " + out)
	out, err = dbcommons.ExecCommand(r, r.Config, targetReadyPod.Name, targetReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("Successfully Created chk file " + out)

	eventReason := "Waiting"
	eventMsg := "Switchover In Progress"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSid' db to make 'targetSid' db primary
	_, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("DB Admin pwd file created")

	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("dgmgrl sys@%s \"SWITCHOVER TO %s\" < admin.pwd", primarySid, targetSid))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("SWITCHOVER TO " + targetSid + " Output")
	log.Info(out)

	//Delete pwd file
	_, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		dbcommons.RemoveAdminPasswordFile)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("DB Admin pwd file removed")

	eventReason = "Success"
	eventMsg = "Switchover Completed Successfully"
	r.Recorder.Eventf(m, corev1.EventTypeNormal, eventReason, eventMsg)

	// Remove the chk File .
	_, err = dbcommons.ExecCommand(r, r.Config, primaryReadyPod.Name, primaryReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.RemoveChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	out, err = dbcommons.ExecCommand(r, r.Config, targetReadyPod.Name, targetReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.RemoveChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY
	}
	log.Info("Successfully Removed chk file " + out)

	// Update Databases
	r.updateReconcileStatus(m, sidbReadyPod, ctx, req)

	// Update status of Primary true/false on 'primary' db (From which switchover initiated)
	if !strings.EqualFold(primarySid, sidbSid) {

		standbyDatabase := &dbapi.SingleInstanceDatabase{}
		err = r.Get(ctx, primaryReq.NamespacedName, standbyDatabase)
		if err != nil {
			return requeueN
		}
		out, err := dbcommons.GetDatabaseRole(primaryReadyPod, r, r.Config, ctx, primaryReq, n.Spec.Edition)
		if err == nil {
			standbyDatabase.Status.Role = strings.ToUpper(out)
		}
		r.Status().Update(ctx, standbyDatabase)

	} else {
		sidbReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: req.Namespace,
				Name:      n.Name,
			},
		}
		out, err := dbcommons.GetDatabaseRole(sidbReadyPod, r, r.Config, ctx, sidbReq, n.Spec.Edition)
		if err == nil {
			n.Status.Role = strings.ToUpper(out)
		}
		r.Status().Update(ctx, n)
	}

	// Update status of Primary true/false on 'sid' db (To which switchover initiated)
	if !strings.EqualFold(targetSid, sidbSid) {

		standbyDatabase := &dbapi.SingleInstanceDatabase{}
		err = r.Get(ctx, targetReq.NamespacedName, standbyDatabase)
		if err != nil {
			return requeueN
		}
		out, err := dbcommons.GetDatabaseRole(targetReadyPod, r, r.Config, ctx, targetReq, n.Spec.Edition)
		if err == nil {
			standbyDatabase.Status.Role = strings.ToUpper(out)
		}
		r.Status().Update(ctx, standbyDatabase)

	} else {
		sidbReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: req.Namespace,
				Name:      n.Name,
			},
		}
		out, err := dbcommons.GetDatabaseRole(sidbReadyPod, r, r.Config, ctx, sidbReq, n.Spec.Edition)
		if err == nil {
			n.Status.Role = strings.ToUpper(out)
		}
		r.Status().Update(ctx, n)
	}

	// Patch DataguardBroker Service to point selector to Current Primary Name and updates client db connection strings on dataguardBroker
	result := r.patchService(m, sidbReadyPod, n, ctx, req)
	if result.Requeue {
		return result
	}

	return requeueN
}

// #############################################################################
//
//	Update Reconcile Status
//
// #############################################################################
func (r *DataguardBrokerReconciler) updateReconcileStatus(m *dbapi.DataguardBroker, sidbReadyPod corev1.Pod,
	ctx context.Context, req ctrl.Request) (err error) {

	// ConnectStrings updated in PatchService()
	var databases []string
	databases, _, err = dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err == nil {
		primaryDatabase := ""
		standbyDatabases := ""
		for i := 0; i < len(databases); i++ {
			splitstr := strings.Split(databases[i], ":")
			if strings.ToUpper(splitstr[1]) == "PRIMARY" {
				primaryDatabase = strings.ToUpper(splitstr[0])
			}
			if strings.ToUpper(splitstr[1]) == "PHYSICAL_STANDBY" {
				if standbyDatabases != "" {
					standbyDatabases += "," + strings.ToUpper(splitstr[0])
				} else {
					standbyDatabases = strings.ToUpper(splitstr[0])
				}
			}
		}
		m.Status.PrimaryDatabase = primaryDatabase
		m.Status.StandbyDatabases = standbyDatabases
	}

	m.Status.PrimaryDatabaseRef = m.Spec.PrimaryDatabaseRef
	m.Status.ProtectionMode = m.Spec.ProtectionMode
	return
}

// #############################################################################
//
//	Manage Finalizer to cleanup before deletion of DataguardBroker
//
// #############################################################################
func (r *DataguardBrokerReconciler) manageDataguardBrokerDeletion(req ctrl.Request, ctx context.Context, m *dbapi.DataguardBroker) (ctrl.Result, error) {
	log := r.Log.WithValues("manageDataguardBrokerDeletion", req.NamespacedName)

	// Check if the DataguardBroker instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDataguardBrokerMarkedToBeDeleted := m.GetDeletionTimestamp() != nil
	if isDataguardBrokerMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(m, dataguardBrokerFinalizer) {
			// Run finalization logic for dataguardBrokerFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			result, err := r.cleanupDataguardBroker(req, ctx, m)
			if result.Requeue {
				return result, err
			}

			// Remove dataguardBrokerFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(m, dataguardBrokerFinalizer)
			err = r.Update(ctx, m)
			if err != nil {
				log.Error(err, err.Error())
				return requeueY, err
			}
		}
		return requeueY, errors.New("deletion pending")
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(m, dataguardBrokerFinalizer) {
		controllerutil.AddFinalizer(m, dataguardBrokerFinalizer)
		err := r.Update(ctx, m)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
	}
	return requeueN, nil
}

// #############################################################################
//
//	Finalization logic for DataguardBrokerFinalizer
//
// #############################################################################
func (r *DataguardBrokerReconciler) cleanupDataguardBroker(req ctrl.Request, ctx context.Context, m *dbapi.DataguardBroker) (ctrl.Result, error) {
	log := r.Log.WithValues("cleanupDataguardBroker", req.NamespacedName)

	// Fetch Primary Database Reference
	singleInstanceDatabase := &dbapi.SingleInstanceDatabase{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.PrimaryDatabaseRef}, singleInstanceDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource deleted. No need to remove dataguard configuration")
			return requeueN, nil
		}
		return requeueY, err
	}

	// Validate if Primary Database Reference is ready
	result, sidbReadyPod, _ := r.validateSidbReadiness(m, singleInstanceDatabase, ctx, req)
	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}

	// Get Primary database to remove dataguard configuration
	_, out, err := dbcommons.GetDatabasesInDgConfig(sidbReadyPod, r, r.Config, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, err
	}

	//primarySid := dbcommons.GetPrimaryDatabase(databases)

	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.RemoveDataguardConfiguration))
	if err != nil {
		log.Error(err, err.Error())
		return requeueY, err
	}
	log.Info("RemoveDataguardConfiguration Output")
	log.Info(out)

	for i := 0; i < len(m.Spec.StandbyDatabaseRefs); i++ {

		standbyDatabase := &dbapi.SingleInstanceDatabase{}
		err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: m.Spec.StandbyDatabaseRefs[i]}, standbyDatabase)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, err.Error())
			return requeueY, err
		}

		// Set DgBrokerConfigured to false
		standbyDatabase.Status.DgBrokerConfigured = false
		r.Status().Update(ctx, standbyDatabase)
	}

	log.Info("Successfully cleaned up Dataguard Broker")
	return requeueN, nil
}

// #############################################################################
//
//	SetupWithManager sets up the controller with the Manager
//
// #############################################################################
func (r *DataguardBrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.DataguardBroker{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of DataguardBroker Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
