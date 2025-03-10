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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// ###############################################################################################################
//
//	Clean up necessary resources required prior to dataguardbroker resource deletion
//
// ###############################################################################################################
func cleanupDataguardBroker(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, req ctrl.Request, ctx context.Context) error {
	log := ctrllog.FromContext(ctx).WithValues("cleanupDataguardBroker", req.NamespacedName)

	log.Info(fmt.Sprintf("Cleaning for dataguard broker %v deletion", broker.Name))

	// Fetch Primary Database Reference
	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: broker.GetCurrentPrimaryDatabase()}, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("SingleInstanceDatabase %s deleted.", broker.GetCurrentPrimaryDatabase()))
			return err
		}
		return err
	}

	log.Info(fmt.Sprintf("The current primary database is %v", sidb.Name))

	// Validate if Primary Database Reference is ready
	if err := validateSidbReadiness(r, broker, &sidb, ctx, req); err != nil {
		log.Info("Reconcile queued")
		return err
	}

	log.Info(fmt.Sprintf("The current primary database %v is ready and healthy", sidb.Name))

	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	log.Info(fmt.Sprintf("Ready pod for the sidb %v is %v", sidb.Name, sidbReadyPod.Name))

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.RemoveDataguardConfiguration))
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info("RemoveDataguardConfiguration Output")
	log.Info(out)

	for _, databaseRef := range broker.Status.DatabasesInDataguardConfig {

		var standbyDatabase dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: databaseRef}, &standbyDatabase); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, err.Error())
			return err
		}

		// Set DgBrokerConfigured to false
		standbyDatabase.Status.DgBroker = nil
		if err := r.Status().Update(ctx, &standbyDatabase); err != nil {
			r.Recorder.Eventf(&standbyDatabase, corev1.EventTypeWarning, "Updating Status", "DgBrokerConfigured status updation failed")
			log.Info(fmt.Sprintf("Status updation for sidb %s failed", standbyDatabase.Name))
			return err
		}
	}

	log.Info("Successfully cleaned up Dataguard Broker")
	return nil
}

// #####################################################################################################
//
//	Validate readiness of the primary singleinstancedatabase specified
//
// #####################################################################################################
func validateSidbReadiness(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, sidb *dbapi.SingleInstanceDatabase, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("validateSidbReadiness", req.NamespacedName)

	var adminPassword string
	var sidbReadyPod corev1.Pod

	// Check if current primary singleinstancedatabase is "ready"
	if sidb.Status.Status != dbcommons.StatusReady {
		return ErrCurrentPrimaryDatabaseNotReady
	}

	// ## FETCH THE SIDB REPLICAS .
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	if sidbReadyPod.Name == "" {
		log.Info("No ready pod avail for the singleinstancedatabase")
		return ErrCurrentPrimaryDatabaseNotReady
	}

	log.Info(fmt.Sprintf("Ready pod for the singleInstanceDatabase %s is %s", sidb.Name, sidbReadyPod.Name))

	// Validate databaseRef Admin Password
	var adminPasswordSecret corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			//m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for : " + sidb.Spec.AdminPassword.SecretName + " to get created"
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + sidb.Spec.AdminPassword.SecretName + " Not Found")
			return fmt.Errorf("adminPassword secret for singleinstancedatabase %v not found", sidb.Name)
		}
		log.Error(err, err.Error())
		return err
	}
	adminPassword = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", fmt.Sprintf(dbcommons.ValidateAdminPassword, adminPassword), dbcommons.GetSqlClient(sidb.Spec.Edition)))
	if err != nil {
		fastStartFailoverStatus, _ := strconv.ParseBool(broker.Status.FastStartFailover)
		if strings.Contains(err.Error(), "dialing backend") && broker.Status.Status == dbcommons.StatusReady && fastStartFailoverStatus {
			// Connection to the pod is failing after broker came up and running
			// Might suggest disconnect or pod/vm going down
			log.Info("Dialing connection error")
			if err := updateReconcileStatus(r, broker, ctx, req); err != nil {
				return err
			}
		}
		log.Error(err, err.Error())
		return err
	}

	if strings.Contains(out, "USER is \"SYS\"") {
		log.Info("validated Admin password successfully")
	} else if strings.Contains(out, "ORA-01017") {
		//m.Status.Status = dbcommons.StatusError
		eventReason := "Logon denied"
		eventMsg := "invalid databaseRef admin password. secret: " + sidb.Spec.AdminPassword.SecretName
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, eventReason, eventMsg)
		return fmt.Errorf("logon denied for singleinstancedatabase %v", sidb.Name)
	} else {
		return fmt.Errorf("%v", out)
	}

	return nil
}

// #############################################################################
//
//	Setup the requested dataguard Configuration
//
// #############################################################################
func setupDataguardBrokerConfiguration(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, sidb *dbapi.SingleInstanceDatabase,
	ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("setupDataguardBrokerConfiguration", req.NamespacedName)

	// Get sidb ready pod for current primary database
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	log.Info(fmt.Sprintf("broker.Spec.StandbyDatabaseRefs are %v", broker.Spec.StandbyDatabaseRefs))

	for _, database := range broker.Spec.StandbyDatabaseRefs {

		log.Info(fmt.Sprintf("adding database %v", database))

		// Get the standby database resource
		var standbyDatabase dbapi.SingleInstanceDatabase
		err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: database}, &standbyDatabase)
		if err != nil {
			if apierrors.IsNotFound(err) {
				eventReason := "Warning"
				eventMsg := database + "not found"
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)
				continue
			}
			log.Error(err, err.Error())
			return err
		}

		// validate standby database status
		if standbyDatabase.Status.Status != dbcommons.StatusReady {
			eventReason := "Waiting"
			eventMsg := "Waiting for " + standbyDatabase.Name + " to be Ready"
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)
			log.Info(fmt.Sprintf("single instance database %s not ready yet", standbyDatabase.Name))
			continue
		}

		// Check if dataguard broker is already configured for the standby database
		if standbyDatabase.Status.DgBroker != nil {
			log.Info("Dataguard broker for standbyDatabase : " + standbyDatabase.Name + " is already configured")
			continue
		}

		// Check if dataguard broker already has a database with the same SID
		_, ok := broker.Status.DatabasesInDataguardConfig[strings.ToUpper(standbyDatabase.Status.Sid)]
		if ok {
			log.Info("A database with the same SID is already configured in the DG")
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Error", "A database with the same SID "+standbyDatabase.Status.Sid+" is already configured in the DG")
			continue
		}

		broker.Status.Status = dbcommons.StatusCreating
		r.Status().Update(ctx, broker)

		// ## FETCH THE STANDBY REPLICAS .
		standbyDatabaseReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
			sidb.Spec.Image.PullFrom, standbyDatabase.Name, standbyDatabase.Namespace, ctx, req)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}

		var adminPasswordSecret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
			return err
		}
		var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])
		if err := setupDataguardBrokerConfigurationForGivenDB(r, broker, sidb, &standbyDatabase, standbyDatabaseReadyPod, sidbReadyPod, ctx, req, adminPassword); err != nil {
			log.Error(err, fmt.Sprintf(" Error while setting up DG broker for the Database %v:%v", standbyDatabase.Status.Sid, standbyDatabase.Name))
			return err
		}
		if len(broker.Status.DatabasesInDataguardConfig) == 0 {
			log.Info("DatabasesInDataguardConfig is nil")
			broker.Status.DatabasesInDataguardConfig = make(map[string]string)
		}
		log.Info(fmt.Sprintf("adding %v:%v to the map", standbyDatabase.Status.Sid, standbyDatabase.Name))
		broker.Status.DatabasesInDataguardConfig[standbyDatabase.Status.Sid] = standbyDatabase.Name
		r.Status().Update(ctx, broker)
		// Update Databases
	}
	if len(broker.Status.DatabasesInDataguardConfig) == 0 {
		broker.Status.DatabasesInDataguardConfig = make(map[string]string)
	}
	log.Info(fmt.Sprintf("adding primary database %v:%v to the map", sidb.Status.Sid, sidb.Name))
	broker.Status.DatabasesInDataguardConfig[sidb.Status.Sid] = sidb.Name

	eventReason := "DG Configuration up to date"
	eventMsg := ""

	// Patch DataguardBroker Service to point selector to Current Primary Name
	if err := patchService(r, broker, ctx, req); err != nil {
		log.Error(err, err.Error())
		return err
	}

	r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)

	return nil
}

// #############################################################################
//
//	Set up dataguard Configuration for a given StandbyDatabase
//
// #############################################################################
func setupDataguardBrokerConfigurationForGivenDB(r *DataguardBrokerReconciler, m *dbapi.DataguardBroker, n *dbapi.SingleInstanceDatabase, standbyDatabase *dbapi.SingleInstanceDatabase,
	standbyDatabaseReadyPod corev1.Pod, sidbReadyPod corev1.Pod, ctx context.Context, req ctrl.Request, adminPassword string) error {

	log := r.Log.WithValues("setupDataguardBrokerConfigurationForGivenDB", req.NamespacedName)

	if standbyDatabaseReadyPod.Name == "" || sidbReadyPod.Name == "" {
		return errors.New("no ready Pod for the singleinstancedatabase")
	}

	// ## CHECK IF DG CONFIGURATION AVAILABLE IN PRIMARY DATABSE##
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.DBShowConfigCMD))
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info("ShowConfiguration Output")
	log.Info(out)

	if strings.Contains(out, "ORA-16525") {
		log.Info("ORA-16525: The Oracle Data Guard broker is not yet available on Primary")
		return fmt.Errorf("ORA-16525: The Oracle Data Guard broker is not yet available on Primary database %v", n.Name)
	}

	_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info("DB Admin pwd file created")

	//  ORA-16532: Oracle Data Guard broker configuration does not exist , so create one
	if strings.Contains(out, "ORA-16532") {
		if m.Spec.ProtectionMode == "MaxPerformance" {
			// Construct the password file and dgbroker command file
			out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerMaxPerformanceCMD))
			if err != nil {
				log.Error(err, err.Error())
				return err
			}
			log.Info("DGMGRL command file creation output")
			log.Info(out)

			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
			out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				"dgmgrl sys@${PRIMARY_DB_CONN_STR} @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd")
			if err != nil {
				log.Error(err, err.Error())
				return err
			}
			log.Info("DgConfigurationMaxPerformance Output")
			log.Info(out)
		} else if m.Spec.ProtectionMode == "MaxAvailability" {
			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAX AVAILABILITY ##
			out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerMaxAvailabilityCMD))
			if err != nil {
				log.Error(err, err.Error())
				return err
			}
			log.Info("DGMGRL command file creation output")
			log.Info(out)

			// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
			out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
				"dgmgrl sys@${PRIMARY_DB_CONN_STR} @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd")
			if err != nil {
				log.Error(err, err.Error())
				return err
			}
			log.Info("DgConfigurationMaxAvailability Output")
			log.Info(out)
		} else {
			log.Info("SPECIFY correct Protection Mode . Either MaxAvailability or MaxPerformance")
			return err
		}

		// ## SHOW CONFIGURATION DG
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | dgmgrl / as sysdba ", dbcommons.DBShowConfigCMD))
		if err != nil {
			log.Error(err, err.Error())
			return err
		} else {
			log.Info("ShowConfiguration Output")
			log.Info(out)
		}
		// Set DG Configured status to true for this standbyDatabase and primary Database. so that in next reconcilation, we dont configure this again
		n.Status.DgBroker = &m.Name
		standbyDatabase.Status.DgBroker = &m.Name
		r.Status().Update(ctx, standbyDatabase)
		r.Status().Update(ctx, n)
		// Remove admin pwd file
		_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
			dbcommons.RemoveAdminPasswordFile)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DB Admin pwd file removed")

		return err
	}

	// DG Configuration Exists . So add the standbyDatabase to the existing DG Configuration
	databases, err := GetDatabasesInDataGuardConfigurationWithRole(r, m, ctx, req)
	if err != nil {
		log.Info("Error while setting up the dataguard configuration")
		log.Error(err, err.Error())
		return err
	}

	// ## ADD DATABASE TO DG CONFIG , IF NOT PRESENT
	found, _ := dbcommons.IsDatabaseFound(standbyDatabase.Spec.Sid, databases, "")
	if found {
		return err
	}
	primarySid := dbcommons.GetPrimaryDatabase(databases)

	// If user adds a new standby to a dg config when failover happened to one ot the standbys, we need to have current primary connect string
	primaryConnectString := n.Name + ":1521/" + primarySid
	if !strings.EqualFold(primarySid, n.Spec.Sid) {
		primaryConnectString = m.Status.DatabasesInDataguardConfig[strings.ToUpper(primarySid)] + ":1521/" + primarySid
	}

	if m.Spec.ProtectionMode == "MaxPerformance" {
		// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAXPERFORMANCE ##
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerAddDBMaxPerformanceCMD))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DGMGRL command file creation output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd ", primaryConnectString))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DgConfigurationMaxPerformance Output")
		log.Info(out)

	} else if m.Spec.ProtectionMode == "MaxAvailability" {
		// ## DG CONFIGURATION FOR PRIMARY DB || MODE : MAX AVAILABILITY ##
		out, err := dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf(dbcommons.CreateDGMGRLScriptFile, dbcommons.DataguardBrokerAddDBMaxAvailabilityCMD))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DGMGRL command file creation output")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("dgmgrl sys@%s @dgmgrl.cmd < admin.pwd && rm -rf dgmgrl.cmd ", primaryConnectString))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("DgConfigurationMaxAvailability Output")
		log.Info(out)

	} else {
		log.Info("SPECIFY correct Protection Mode . Either MaxAvailability or MaxPerformance")
		log.Error(err, err.Error())
		return err
	}

	// Remove admin pwd file
	_, err = dbcommons.ExecCommand(r, r.Config, standbyDatabaseReadyPod.Name, standbyDatabaseReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		dbcommons.RemoveAdminPasswordFile)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info("DB Admin pwd file removed")

	// Set DG Configured status to true for this standbyDatabase. so that in next reconcilation, we dont configure this again
	standbyDatabase.Status.DgBroker = &m.Name
	r.Status().Update(ctx, standbyDatabase)

	return nil
}

// ###########################################################################################################
//
//	Patch the service for dataguardbroker resource to point selector to current Primary Name
//
// ###########################################################################################################
func patchService(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) error {
	log := r.Log.WithValues("patchService", req.NamespacedName)

	primaryDatabaseRef := broker.Status.DatabasesInDataguardConfig[broker.Status.PrimaryDatabase]
	var svc *corev1.Service = &corev1.Service{}

	// fetch the k8s service for the dataguardbroker resource
	err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, svc)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Patching Service %s to point to the currPrimaryDatabase %s", svc.Name, primaryDatabaseRef))

	// updating service selector for the primary database pod to attach itself to the service
	svc.Spec.Selector["app"] = primaryDatabaseRef
	if err = r.Update(ctx, svc); err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Patching service %s successful ", svc.Name))

	// updating the dataguardbroker resource connect strings
	broker.Status.ClusterConnectString = svc.Name + "." + svc.Namespace + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
	if broker.Spec.LoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			lbAddress := svc.Status.LoadBalancer.Ingress[0].Hostname
			if lbAddress == "" {
				lbAddress = svc.Status.LoadBalancer.Ingress[0].IP
			}
			broker.Status.ExternalConnectString = lbAddress + ":" + fmt.Sprint(svc.Spec.Ports[0].Port) + "/DATAGUARD"
		}
	} else {
		nodeip := dbcommons.GetNodeIp(r, ctx, req)
		if nodeip != "" {
			broker.Status.ExternalConnectString = nodeip + ":" + fmt.Sprint(svc.Spec.Ports[0].NodePort) + "/DATAGUARD"
		}
	}
	log.Info("Updated connect strings to the dataguard broker")
	return nil
}

// ###########################################################################################################
//
//	Update Reconcile Status
//
// ###########################################################################################################
func updateReconcileStatus(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (err error) {

	log := r.Log.WithValues("updateReconcileStatus", req.NamespacedName)

	// fetch the singleinstancedatabase (database sid) and their role in the dataguard configuration
	var databases []string
	databases, err = GetDatabasesInDataGuardConfigurationWithRole(r, broker, ctx, req)
	if err != nil {
		log.Info("Problem when retrieving the databases in dg config")
		broker.Status.Status = dbcommons.StatusNotReady
		r.Status().Update(ctx, broker)
		return nil
	}

	// loop over all the databases to update the status of the dataguardbroker and the singleinstancedatabase
	var standbyDatabases string = ""
	for i := 0; i < len(databases); i++ {
		splitstr := strings.Split(databases[i], ":")
		database := strings.ToUpper(splitstr[0])
		var singleInstanceDatabase dbapi.SingleInstanceDatabase
		err := r.Get(ctx, types.NamespacedName{Name: broker.Status.DatabasesInDataguardConfig[database], Namespace: req.Namespace}, &singleInstanceDatabase)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("Checking current role of %v is %v and its status is %v", broker.Status.DatabasesInDataguardConfig[database], strings.ToUpper(splitstr[1]), singleInstanceDatabase.Status.Role))
		if singleInstanceDatabase.Status.Role != strings.ToUpper(splitstr[1]) {
			singleInstanceDatabase.Status.Role = strings.ToUpper(splitstr[1])
			r.Status().Update(ctx, &singleInstanceDatabase)
		}
		if strings.ToUpper(splitstr[1]) == "PRIMARY" && strings.ToUpper(database) != strings.ToUpper(broker.Status.PrimaryDatabase) {
			log.Info("primary Database is " + strings.ToUpper(database))
			broker.Status.PrimaryDatabase = strings.ToUpper(database)
			// patch the service with the current primary
		}
		if strings.ToUpper(splitstr[1]) == "PHYSICAL_STANDBY" {
			if standbyDatabases != "" {
				standbyDatabases += "," + strings.ToUpper(splitstr[0])
			} else {
				standbyDatabases = strings.ToUpper(splitstr[0])
			}
		}
	}

	broker.Status.StandbyDatabases = standbyDatabases
	broker.Status.ProtectionMode = broker.Spec.ProtectionMode
	r.Status().Update(ctx, broker)

	// patch the dataguardbroker resource service
	if err := patchService(r, broker, ctx, req); err != nil {
		return err
	}

	return nil
}

// #####################################################################################################
//
//	Get the avail FSFO targets for a given singleinstancedatabase sid
//
// #####################################################################################################
func GetFSFOTargets(databaseSid string, databasesInDgConfig map[string]string) (string, error) {
	if _, ok := databasesInDgConfig[databaseSid]; !ok {
		return "", fmt.Errorf("database %s not in dataguard config", databasesInDgConfig[databaseSid])
	}
	var fsfoTarget []string
	for dbSid, _ := range databasesInDgConfig {
		if strings.Compare(databaseSid, dbSid) != 0 {
			fsfoTarget = append(fsfoTarget, dbSid)
		}
	}
	return strings.Join(fsfoTarget, ","), nil
}

// #####################################################################################################
//
//	Set faststartfailover targets accordingly to dataguard configuration
//
// #####################################################################################################
func setFSFOTargets(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("setFSFOTargets", req.NamespacedName)

	// fetch the current primary singleinstancedatabase
	var currentPrimaryDatabase dbapi.SingleInstanceDatabase
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: broker.GetCurrentPrimaryDatabase()}, &currentPrimaryDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return nil
		}
		r.Log.Error(err, err.Error())
		return err
	}

	log.Info(fmt.Sprintf("current primary database for the dg config is %s", currentPrimaryDatabase.Name))

	// fetch the singleinstancedatabase ready pod
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, currentPrimaryDatabase.Spec.Image.Version,
		currentPrimaryDatabase.Spec.Image.PullFrom, currentPrimaryDatabase.Name, currentPrimaryDatabase.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return fmt.Errorf("error while fetching ready pod for %s", currentPrimaryDatabase.Name)
	}

	log.Info(fmt.Sprintf("current primary database ready pod is %s", sidbReadyPod.Name))

	// fetch singleinstancedatabase admin password
	var adminPasswordSecret corev1.Secret
	if err = r.Get(ctx, types.NamespacedName{Name: currentPrimaryDatabase.Spec.AdminPassword.SecretName, Namespace: currentPrimaryDatabase.Namespace}, &adminPasswordSecret); err != nil {
		if apierrors.IsNotFound(err) {
			//m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for : " + currentPrimaryDatabase.Spec.AdminPassword.SecretName + " to get created"
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + currentPrimaryDatabase.Spec.AdminPassword.SecretName + " Not Found")
			return errors.New("admin password secret not found")
		}
		log.Error(err, err.Error())
		return err
	}
	adminPassword := string(adminPasswordSecret.Data[currentPrimaryDatabase.Spec.AdminPassword.SecretKey])

	for databaseSid, databaseRef := range broker.Status.DatabasesInDataguardConfig {
		// construct FSFO target for this database
		fsfoTargets, err := GetFSFOTargets(databaseSid, broker.Status.DatabasesInDataguardConfig)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("Setting fast start failover target for the database %s to %s", databaseRef, fsfoTargets))
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"EDIT DATABASE %s SET PROPERTY FASTSTARTFAILOVERTARGET=%s \"  | dgmgrl sys/%s@%s ",
				databaseSid, fsfoTargets, adminPassword, currentPrimaryDatabase.Status.Sid))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("SETTING FSFO TARGET OUTPUT")
		log.Info(out)

		out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"SHOW DATABASE %s FASTSTARTFAILOVERTARGET  \"  | dgmgrl sys/%s@%s ", databaseSid, adminPassword, currentPrimaryDatabase.Status.Sid))
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		log.Info("FSFO TARGETS OF " + databaseSid)
		log.Info(out)
	}

	// Set FSFO Targets according to the input yaml of broker
	return nil
}

// #############################################################################
//
//	Setup the requested dataguard configuration
//
// #############################################################################
func createObserverPods(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("createObserverPods", req.NamespacedName)

	// fetch the current primary singleinstancedatabase resourcce
	var currPrimaryDatabase dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      broker.GetCurrentPrimaryDatabase(),
	}
	if err := r.Get(ctx, namespacedName, &currPrimaryDatabase); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", fmt.Sprintf("SingleInstanceDatabase %s not found", namespacedName.Name))
			r.Log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ErrCurrentPrimaryDatabaseNotFound
		}
		return err
	}

	// fetch the dataguardbroker observer replicas
	_, brokerReplicasFound, _, _, err := dbcommons.FindPods(r, "", "", broker.Name, broker.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	if brokerReplicasFound > 0 {
		return nil
	}

	// Stop the already running observer
	// find the avail pods for the currPrimaryDatabase
	log.Info("Need to stop the observer if already running")
	currPrimaryDatabaseReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", currPrimaryDatabase.Name, currPrimaryDatabase.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	if currPrimaryDatabaseReadyPod.Name == "" {
		return errors.New("No ready pods avail ")
	}

	// fetch singleinstancedatabase admin password
	var adminPasswordSecret corev1.Secret
	if err = r.Get(ctx, types.NamespacedName{Name: currPrimaryDatabase.Spec.AdminPassword.SecretName, Namespace: currPrimaryDatabase.Namespace}, &adminPasswordSecret); err != nil {
		if apierrors.IsNotFound(err) {
			//m.Status.Status = dbcommons.StatusError
			eventReason := "Waiting"
			eventMsg := "waiting for : " + currPrimaryDatabase.Spec.AdminPassword.SecretName + " to get created"
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)
			r.Log.Info("Secret " + currPrimaryDatabase.Spec.AdminPassword.SecretName + " Not Found")
			return errors.New("admin password secret not found")
		}
		log.Error(err, err.Error())
		return err
	}
	adminPassword := string(adminPasswordSecret.Data[currPrimaryDatabase.Spec.AdminPassword.SecretKey])

	out, err := dbcommons.ExecCommand(r, r.Config, currPrimaryDatabaseReadyPod.Name, currPrimaryDatabaseReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \" STOP OBSERVER %s \"  | dgmgrl sys/%s@%s ", broker.Name, adminPassword, currPrimaryDatabase.Status.Sid))
	if err != nil {
		log.Error(err, err.Error())
		return err
	}
	log.Info(out)
	// instantiate observer pod specification
	pod := dbcommons.NewRealPodBuilder().
		SetNamespacedName(types.NamespacedName{
			Name:      broker.Name + "-" + dbcommons.GenerateRandomString(5),
			Namespace: broker.Namespace,
		}).
		SetLabels(map[string]string{
			"app":     broker.Name,
			"version": currPrimaryDatabase.Spec.Image.PullSecrets,
		}).
		SetTerminationGracePeriodSeconds(int64(30)).
		SetNodeSelector(func() map[string]string {
			var nsRule map[string]string = map[string]string{}
			if len(broker.Spec.NodeSelector) != 0 {
				for key, value := range broker.Spec.NodeSelector {
					nsRule[key] = value
				}
			}
			return nsRule
		}()).
		SetSecurityContext(corev1.PodSecurityContext{
			RunAsUser: func() *int64 { i := int64(54321); return &i }(),
			FSGroup:   func() *int64 { i := int64(54321); return &i }(),
		}).
		SetImagePullSecrets(currPrimaryDatabase.Spec.Image.PullSecrets).
		AppendContainers(corev1.Container{
			Name:  broker.Name,
			Image: currPrimaryDatabase.Spec.Image.PullFrom,
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c", "/bin/echo -en 'shutdown abort;\n' | env ORACLE_SID=${ORACLE_SID^^} sqlplus -S / as sysdba"},
					},
				},
			},
			ImagePullPolicy: corev1.PullAlways,
			Ports:           []corev1.ContainerPort{{ContainerPort: 1521}, {ContainerPort: 5500}},

			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c", "$ORACLE_BASE/checkDBLockStatus.sh"},
					},
				},
				InitialDelaySeconds: 20,
				TimeoutSeconds:      20,
				PeriodSeconds:       40,
			},
			Env: []corev1.EnvVar{
				{
					Name:  "SVC_HOST",
					Value: broker.Name,
				},
				{
					Name:  "SVC_PORT",
					Value: "1521",
				},
				{
					Name:  "PRIMARY_DB_CONN_STR",
					Value: currPrimaryDatabase.Name + ":1521/" + currPrimaryDatabase.Spec.Sid,
				},
				{
					Name:  "DG_OBSERVER_ONLY",
					Value: "true",
				},
				{
					Name:  "DG_OBSERVER_NAME",
					Value: broker.Name,
				},
				{
					// Sid used here only for Locking mechanism to work .
					Name:  "ORACLE_SID",
					Value: "OBSRVR" + strings.ToUpper(currPrimaryDatabase.Spec.Sid),
				},
				{
					Name: "ORACLE_PWD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: currPrimaryDatabase.Spec.AdminPassword.SecretName,
							},
							Key: currPrimaryDatabase.Spec.AdminPassword.SecretKey,
						},
					},
				},
			},
		}).
		Build()

	// set the ownership and lifecyle of the observer pod to the dataguardbroker resource
	ctrl.SetControllerReference(broker, &pod, r.Scheme)

	log.Info("Creating a new  POD", "POD.Namespace", pod.Namespace, "POD.Name", pod.Name)
	if err = r.Create(ctx, &pod); err != nil {
		log.Error(err, "Failed to create new POD", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
		return err
	}

	// Waiting for Pod to get created as sometimes it takes some time to create a Pod . 30 seconds TImeout
	timeout := 30
	err = dbcommons.WaitForStatusChange(r, pod.Name, broker.Namespace, ctx, req, time.Duration(timeout)*time.Second, "pod", "creation")
	if err != nil {
		log.Error(err, "Error in Waiting for Pod status for Creation", "pod.Namespace", pod.Namespace, "POD.Name", pod.Name)
		return err
	}
	log.Info("Succesfully Created New Pod ", "POD.NAME : ", pod.Name)

	eventReason := "SUCCESS"
	eventMsg := ""
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)

	return nil
}

// #############################################################################
//
//	Enable faststartfailover for the dataguard configuration
//
// #############################################################################
func enableFSFOForDgConfig(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("enableFSFOForDgConfig", req.NamespacedName)

	// Get the current primary singleinstancedatabase resourcce
	var sidb dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      broker.GetCurrentPrimaryDatabase(),
	}
	if err := r.Get(ctx, namespacedName, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", fmt.Sprintf("SingleInstanceDatabase %s not found", sidb.Name))
			log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ErrCurrentPrimaryDatabaseNotFound
		}
		return err
	}

	// fetch singleinstancedatabase ready pod
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	// fetch singleinstancedatabase adminpassword secret
	var adminPasswordSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
		return err
	}
	var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Enabling FastStartFailover", fmt.Sprintf("Enabling FastStartFailover for the dataguard broker %s", broker.Name))
	log.Info(fmt.Sprintf("Enabling FastStartFailover for the dataguard broker %s", broker.Name))

	// enable faststartfailover for the dataguard configuration
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl sys/%s@%s ", dbcommons.EnableFSFOCMD, adminPassword, sidb.Status.Sid))
	if err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Enabling FastStartFailover failed", fmt.Sprintf("Enabling FastStartFailover for the dataguard broker %s failed", broker.Name))
		log.Error(err, err.Error())
		return err
	}
	log.Info("EnableFastStartFailover Output")
	log.Info(out)

	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Enabling FastStartFailover successful", fmt.Sprintf("Enabling FastStartFailover for the dataguard broker %s successful", broker.Name))

	return nil
}

// #############################################################################
//
//	Disable faststartfailover for the dataguard configuration
//
// #############################################################################
func disableFSFOForDGConfig(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) error {

	log := r.Log.WithValues("disableFSFOForDGConfig", req.NamespacedName)

	// Get the current primary singleinstancedatabase resource
	var sidb dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      broker.GetCurrentPrimaryDatabase(),
	}
	if err := r.Get(ctx, namespacedName, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", fmt.Sprintf("SingleInstanceDatabase %s not found", sidb.Name))
			log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ErrCurrentPrimaryDatabaseNotFound
		}
		return err
	}

	// fetch singleinstancedatabase ready pod
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return err
	}

	// fetch admin password for the singleinstancedatabase
	var adminPasswordSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
		return err
	}
	var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Disabling FastStartFailover", fmt.Sprintf("Disabling FastStartFailover for the dataguard broker %s", broker.Name))
	log.Info(fmt.Sprintf("Disabling FastStartFailover for the dataguard broker %s", broker.Name))

	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | dgmgrl sys/%s@%s ", fmt.Sprintf(dbcommons.DisableFSFOCMD, broker.Name), adminPassword, sidb.Status.Sid))
	if err != nil {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Disabling FastStartFailover failed", fmt.Sprintf("Disabling FastStartFailover for the dataguard broker %s failed", broker.Name))
		log.Error(err, err.Error())
		return err
	}
	log.Info("DisableFastStartFailover Output")
	log.Info(out)

	r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Disabling FastStartFailover", "faststartfailover disabled successfully")
	log.Info("faststartfailover disabled successfully")

	return nil
}

// #############################################################################
//
//	Get databases in dataguard configuration along with their roles
//
// #############################################################################
func GetDatabasesInDataGuardConfigurationWithRole(r *DataguardBrokerReconciler, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) ([]string, error) {
	r.Log.Info(fmt.Sprintf("GetDatabasesInDataGuardConfiguration are %v", broker.GetDatabasesInDataGuardConfiguration()))
	for _, database := range broker.GetDatabasesInDataGuardConfiguration() {

		var singleInstanceDatabase dbapi.SingleInstanceDatabase
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: broker.Namespace, Name: database}, &singleInstanceDatabase); err != nil {
			// log about the error while fetching the database
			continue
		}

		// Fetch the primary database ready pod
		sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, singleInstanceDatabase.Spec.Image.Version,
			singleInstanceDatabase.Spec.Image.PullFrom, singleInstanceDatabase.Name, singleInstanceDatabase.Namespace, ctx, req)
		if err != nil || sidbReadyPod.Name == "" {
			continue
		}

		// try out
		out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba ", dbcommons.DataguardBrokerGetDatabaseCMD))
		if err != nil || strings.Contains(out, "no rows selected") && strings.Contains(out, "ORA-") {
			continue
		}

		r.Log.Info(fmt.Sprintf("sidbReadyPod is %v \n output of the exec is %v \n and output contains ORA- is %v", sidbReadyPod.Name, out, strings.Contains(out, "ORA-")))

		out1 := strings.Replace(out, " ", "_", -1)
		// filtering output and storing databses in dg configuration in  "databases" slice
		databases := strings.Fields(out1)

		// first 2 values in the slice will be column name(DATABASES) and a seperator(--------------) . so take the slice from position [2:]
		databases = databases[2:]
		return databases, nil
	}

	return []string{}, errors.New("cannot get databases in dataguard configuration")
}
