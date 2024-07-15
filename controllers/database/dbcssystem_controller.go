/*
** Copyright (c) 2022-2024 Oracle and/or its affiliates.
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
	"fmt"
	"reflect"
	"strings"
	"time"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcsv1 "github.com/oracle/oracle-database-operator/commons/dbcssystem"
	"github.com/oracle/oracle-database-operator/commons/finalizer"
	"github.com/oracle/oracle-database-operator/commons/oci"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/workrequests"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DbcsSystemReconciler reconciles a DbcsSystem object
type DbcsSystemReconciler struct {
	KubeClient client.Client
	Scheme     *runtime.Scheme
	Logv1      logr.Logger
	Logger     logr.Logger
	dbClient   database.DatabaseClient
	nwClient   core.VirtualNetworkClient
	wrClient   workrequests.WorkRequestClient
	Recorder   record.EventRecorder
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DbcsSystem object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DbcsSystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger = log.FromContext(ctx)

	var err error
	// Get the dbcs instance from the cluster
	dbcsInst := &databasev1alpha1.DbcsSystem{}
	r.Logger.Info("Reconciling DbSystemDetails", "name", req.NamespacedName)

	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, dbcsInst); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Create oci-go-sdk client
	authData := oci.APIKeyAuth{
		ConfigMapName: &dbcsInst.Spec.OCIConfigMap,
		SecretName:    &dbcsInst.Spec.OCISecret,
		Namespace:     dbcsInst.GetNamespace(),
	}
	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.dbClient, err = database.NewDatabaseClientWithConfigurationProvider(provider)

	if err != nil {
		return ctrl.Result{}, err
	}

	r.nwClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.wrClient, err = workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Logger.Info("OCI provider configured succesfully")

	/*
	 Using Finalizer for object deletion
	*/

	if dbcsInst.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted
		if dbcsInst.Spec.HardLink && !finalizer.HasFinalizer(dbcsInst) {
			finalizer.Register(r.KubeClient, dbcsInst)
			r.Logger.Info("Finalizer registered successfully.")
		} else if !dbcsInst.Spec.HardLink && finalizer.HasFinalizer(dbcsInst) {
			finalizer.Unregister(r.KubeClient, dbcsInst)
			r.Logger.Info("Finalizer unregistered successfully.")
		}
	} else {
		// The object is being deleted
		r.Logger.Info("Terminate DbcsSystem Database: " + dbcsInst.Spec.DbSystem.DisplayName)
		if err := dbcsv1.DeleteDbcsSystemSystem(r.dbClient, *dbcsInst.Spec.Id); err != nil {
			r.Logger.Error(err, "Fail to terminate DbcsSystem Instance")
			// Change the status to Failed
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Terminate, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			return ctrl.Result{}, nil
		}

		// Check if PDBConfig is defined
		pdbConfigs := dbcsInst.Spec.PdbConfigs
		for _, pdbConfig := range pdbConfigs {
			if pdbConfig.PdbName != nil {
				// Handle PDB deletion if PluggableDatabaseId is defined and isDelete is true
				if pdbConfig.IsDelete != nil && pdbConfig.PluggableDatabaseId != nil && *pdbConfig.IsDelete {
					// Call deletePluggableDatabase function
					dbSystemId := *dbcsInst.Spec.Id
					if err := r.deletePluggableDatabase(ctx, pdbConfig, dbSystemId); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}
		// Remove the finalizer and update the object
		finalizer.Unregister(r.KubeClient, dbcsInst)
		r.Logger.Info("Finalizer unregistered successfully.")
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	/*
	 Determine whether it's a provision or bind operation
	*/
	lastSucSpec, err := dbcsInst.GetLastSuccessfulSpec()
	if err != nil {
		return ctrl.Result{}, err
	}

	if dbcsInst.Spec.Id == nil && lastSucSpec == nil {
		// If no DbcsSystem ID specified, create a DB System
		// ======================== Validate Specs ==============
		err = dbcsv1.ValidateSpex(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.Recorder)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Logger.Info("DbcsSystem DBSystem provisioning")
		dbcsID, err := dbcsv1.CreateAndGetDbcsId(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient)
		if err != nil {
			r.Logger.Error(err, "Fail to provision and get DbcsSystem System ID")

			// Change the status to Failed
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			return ctrl.Result{}, nil
		}

		assignDBCSID(dbcsInst, dbcsID)
		if err := dbcsv1.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
			// Change the status to Failed
			assignDBCSID(dbcsInst, dbcsID)
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, err
		}

		r.Logger.Info("DbcsSystem system provisioned succesfully")
		assignDBCSID(dbcsInst, dbcsID)
		if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
			return ctrl.Result{}, err
		}
		assignDBCSID(dbcsInst, dbcsID)
	} else {
		if lastSucSpec == nil {
			if err := dbcsv1.GetDbSystemId(r.Logger, r.dbClient, dbcsInst); err != nil {
				// Change the status to Failed
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}
			if err := dbcsv1.SetDBCSDatabaseLifecycleState(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
				// Change the status to required state
				return ctrl.Result{}, err
			}

			dbSystemId := *dbcsInst.Spec.Id
			if err := dbcsv1.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
				// Change the status to Failed
				assignDBCSID(dbcsInst, dbSystemId)
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}

			r.Logger.Info("Sync information from remote DbcsSystem System successfully")

			dbSystemId = *dbcsInst.Spec.Id
			if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
				return ctrl.Result{}, err
			}
			assignDBCSID(dbcsInst, dbSystemId)
		} else {
			if dbcsInst.Spec.Id == nil {
				dbcsInst.Spec.Id = lastSucSpec.Id
			}

			if err := dbcsv1.UpdateDbcsSystemIdInst(r.Logger, r.dbClient, dbcsInst, r.KubeClient, r.nwClient, r.wrClient); err != nil {
				r.Logger.Error(err, "Fail to update DbcsSystem Id")

				// Change the status to Failed
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}
			if err := dbcsv1.SetDBCSDatabaseLifecycleState(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
				// Change the status to required state
				return ctrl.Result{}, err
			}
		}
	}

	// Update the Wallet Secret when the secret name is given
	//r.updateWalletSecret(dbcs)

	// Update the last succesful spec
	dbSystemId := *dbcsInst.Spec.Id
	if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
		return ctrl.Result{}, err
	}
	//assignDBCSID(dbcsInst,dbcsI)
	// Change the phase to "Available"
	assignDBCSID(dbcsInst, dbSystemId)
	if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Available, r.nwClient, r.wrClient); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	r.Logger.Info("DBInst after assignment", "dbcsInst:->", dbcsInst)
	// // Check if PDBConfig is defined and needs to be created or deleted
	pdbConfigs := dbcsInst.Spec.PdbConfigs

	if pdbConfigs != nil {
		for _, pdbConfig := range pdbConfigs {
			if pdbConfig.PdbName != nil {
				// Get database details
				// Get DB Home ID by DB System ID
				// Get Compartment ID by DB System ID
				compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, dbSystemId)
				if err != nil {
					fmt.Printf("Failed to get compartment ID: %v\n", err)
					return ctrl.Result{}, err
				}
				dbHomeId, err := r.getDbHomeIdByDbSystemID(ctx, compartmentId, dbSystemId)
				if err != nil {
					fmt.Printf("Failed to get DB Home ID: %v\n", err)
					return ctrl.Result{}, err
				}
				databaseIds, err := r.getDatabaseIDByDbSystemID(ctx, dbSystemId, compartmentId, dbHomeId)
				if err != nil {
					fmt.Printf("Failed to get database IDs: %v\n", err)
					return ctrl.Result{}, err
				}

				// Now you can use dbDetails to access database attributes
				r.Logger.Info("Database details fetched successfully", "DatabaseId", databaseIds)

				// Check if deletion is requested
				if pdbConfig.IsDelete != nil && *pdbConfig.IsDelete {
					// Call deletePluggableDatabase function
					if err := r.deletePluggableDatabase(ctx, pdbConfig, dbSystemId); err != nil {
						return ctrl.Result{}, err
					}
					// Continue to the next pdbConfig
					continue
				} else {
					// Call the method to create the pluggable database
					r.Logger.Info("Calling createPluggableDatabase", "ctx:->", ctx, "dbcsInst:->", dbcsInst, "databaseIds:->", databaseIds[0], "compartmentId:->", compartmentId)
					err := r.createPluggableDatabase(ctx, dbcsInst, pdbConfig, databaseIds[0], compartmentId, dbSystemId)
					if err != nil {
						// Handle error if required
						return ctrl.Result{}, err
					}
				}
			}
		}
	} else {
		r.Logger.Info("No PDB configurations given.")
	}

	return ctrl.Result{}, nil

}

// getDbHomeIdByDbSystemID retrieves the DB Home ID associated with the given DB System ID
func (r *DbcsSystemReconciler) getDbHomeIdByDbSystemID(ctx context.Context, compartmentId, dbSystemId string) (string, error) {
	listRequest := database.ListDbHomesRequest{
		CompartmentId: &compartmentId,
		DbSystemId:    &dbSystemId,
	}

	listResponse, err := r.dbClient.ListDbHomes(ctx, listRequest)
	if err != nil {
		return "", fmt.Errorf("failed to list DB homes: %v", err)
	}

	if len(listResponse.Items) == 0 {
		return "", fmt.Errorf("no DB homes found for DB system ID: %s", dbSystemId)
	}

	return *listResponse.Items[0].Id, nil
}
func (r *DbcsSystemReconciler) getCompartmentIDByDbSystemID(ctx context.Context, dbSystemId string) (string, error) {
	// Construct the GetDbSystem request
	getRequest := database.GetDbSystemRequest{
		DbSystemId: &dbSystemId,
	}

	// Call GetDbSystem API using the existing dbClient
	getResponse, err := r.dbClient.GetDbSystem(ctx, getRequest)
	if err != nil {
		return "", fmt.Errorf("failed to get DB system details: %v", err)
	}

	// Extract the compartment ID from the DB system details
	compartmentId := *getResponse.DbSystem.CompartmentId

	return compartmentId, nil
}
func (r *DbcsSystemReconciler) getDatabaseIDByDbSystemID(ctx context.Context, dbSystemId, compartmentId, dbHomeId string) ([]string, error) {
	// Construct the ListDatabases request
	request := database.ListDatabasesRequest{
		SystemId:      &dbSystemId,
		CompartmentId: &compartmentId,
		DbHomeId:      &dbHomeId,
	}

	// Call ListDatabases API using the existing dbClient
	response, err := r.dbClient.ListDatabases(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %v", err)
	}

	// Extract database IDs from the response
	var databaseIds []string
	for _, dbSummary := range response.Items {
		databaseIds = append(databaseIds, *dbSummary.Id)
	}

	return databaseIds, nil
}

func (r *DbcsSystemReconciler) createPluggableDatabase(ctx context.Context, dbcs *databasev1alpha1.DbcsSystem, pdbConfig databasev1alpha1.PDBConfig, databaseId, compartmentId, dbSystemId string) error {
	r.Logger.Info("Checking if the pluggable database exists", "PDBName", pdbConfig.PdbName)

	// Check if the pluggable database already exists
	exists, pdbId, err := r.doesPluggableDatabaseExist(ctx, compartmentId, pdbConfig.PdbName)
	if err != nil {
		r.Logger.Error(err, "Failed to check if pluggable database exists", "PDBName", pdbConfig.PdbName)
		return err
	}
	if exists {
		// Set the PluggableDatabaseId in PDBConfig
		pdbConfig.PluggableDatabaseId = pdbId
		r.Logger.Info("Pluggable database already exists", "PDBName", pdbConfig.PdbName, "PluggableDatabaseId", *pdbConfig.PluggableDatabaseId)
		return nil
	}

	// Define the DatabaseExists method locally
	databaseExists := func(dbSystemID string) (bool, error) {
		req := database.GetDbSystemRequest{
			DbSystemId: &dbSystemID,
		}
		_, err := r.dbClient.GetDbSystem(ctx, req)
		if err != nil {
			if ociErr, ok := err.(common.ServiceError); ok && ociErr.GetHTTPStatusCode() == 404 {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	exists, err = databaseExists(dbSystemId)
	if err != nil {
		r.Logger.Error(err, "Failed to check database existence")
		return err
	}

	if !exists {
		errMsg := fmt.Sprintf("Database does not exist: %s", dbSystemId)
		r.Logger.Error(fmt.Errorf(errMsg), "Database not found")
		return fmt.Errorf(errMsg)
	}

	// Fetch secrets for TdeWalletPassword and PdbAdminPassword
	tdeWalletPassword, err := r.getSecret(ctx, dbcs.Namespace, *pdbConfig.TdeWalletPassword)
	// Trim newline character from the password
	tdeWalletPassword = strings.TrimSpace(tdeWalletPassword)
	r.Logger.Info("TDE wallet password retrieved successfully")
	if err != nil {
		r.Logger.Error(err, "Failed to get TDE wallet password secret")
		return err
	}

	pdbAdminPassword, err := r.getSecret(ctx, dbcs.Namespace, *pdbConfig.PdbAdminPassword)
	// Trim newline character from the password
	pdbAdminPassword = strings.TrimSpace(pdbAdminPassword)
	r.Logger.Info("PDB admin password retrieved successfully")
	if err != nil {
		r.Logger.Error(err, "Failed to get PDB admin password secret")
		return err
	}

	// Proceed with creating the pluggable database
	r.Logger.Info("Creating pluggable database", "PDBName", pdbConfig.PdbName)
	createPdbReq := database.CreatePluggableDatabaseRequest{
		CreatePluggableDatabaseDetails: database.CreatePluggableDatabaseDetails{
			PdbName:                       pdbConfig.PdbName,
			ContainerDatabaseId:           &databaseId,
			ShouldPdbAdminAccountBeLocked: pdbConfig.ShouldPdbAdminAccountBeLocked,
			PdbAdminPassword:              common.String(pdbAdminPassword),
			TdeWalletPassword:             common.String(tdeWalletPassword),
			FreeformTags:                  pdbConfig.FreeformTags,
		},
	}
	response, err := r.dbClient.CreatePluggableDatabase(ctx, createPdbReq)
	if err != nil {
		r.Logger.Error(err, "Failed to create pluggable database", "PDBName", pdbConfig.PdbName)
		return err
	}
	// Set the PluggableDatabaseId in PDBConfig
	// Set the PluggableDatabaseId in PDBConfig
	pdbConfig.PluggableDatabaseId = response.PluggableDatabase.Id

	r.Logger.Info("Pluggable database creation initiated", "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)

	// Polling mechanism to check PDB status
	const maxRetries = 120   // total 1 hour wait for creation of PDB
	const retryInterval = 30 // in seconds

	for i := 0; i < maxRetries; i++ {
		getPdbReq := database.GetPluggableDatabaseRequest{
			PluggableDatabaseId: pdbConfig.PluggableDatabaseId,
		}

		getPdbResp, err := r.dbClient.GetPluggableDatabase(ctx, getPdbReq)
		if err != nil {
			r.Logger.Error(err, "Failed to get pluggable database status", "PDBID", *pdbConfig.PluggableDatabaseId)
			return err
		}

		pdbStatus := getPdbResp.PluggableDatabase.LifecycleState
		r.Logger.Info("Checking pluggable database status", "PDBID", *pdbConfig.PluggableDatabaseId, "Status", pdbStatus)

		if pdbStatus == database.PluggableDatabaseLifecycleStateAvailable {
			r.Logger.Info("Pluggable database successfully created", "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
			return nil
		}

		if pdbStatus == database.PluggableDatabaseLifecycleStateFailed {
			r.Logger.Error(fmt.Errorf("pluggable database creation failed"), "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
			return fmt.Errorf("pluggable database creation failed")
		}

		time.Sleep(retryInterval * time.Second)
	}

	r.Logger.Error(fmt.Errorf("timed out waiting for pluggable database to become available"), "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
	return fmt.Errorf("timed out waiting for pluggable database to become available")
}

func (r *DbcsSystemReconciler) pluggableDatabaseExists(ctx context.Context, pluggableDatabaseId string) (bool, error) {
	req := database.GetPluggableDatabaseRequest{
		PluggableDatabaseId: &pluggableDatabaseId,
	}
	_, err := r.dbClient.GetPluggableDatabase(ctx, req)
	if err != nil {
		if ociErr, ok := err.(common.ServiceError); ok && ociErr.GetHTTPStatusCode() == 404 {
			// PDB does not exist
			return false, nil
		}
		// Other error occurred
		return false, err
	}
	// PDB exists
	return true, nil
}

func (r *DbcsSystemReconciler) deletePluggableDatabase(ctx context.Context, pdbConfig databasev1alpha1.PDBConfig, dbSystemId string) error {
	if pdbConfig.PdbName == nil {
		return fmt.Errorf("PDB name is not specified")
	}

	r.Logger.Info("Deleting pluggable database", "PDBName", *pdbConfig.PdbName)

	if pdbConfig.PluggableDatabaseId == nil {
		r.Logger.Info("PluggableDatabaseId is not specified, getting pluggable databaseID")
		// Call a function to retrieve PluggableDatabaseId
		pdbID, err := r.getPluggableDatabaseID(ctx, pdbConfig, dbSystemId)
		if err != nil {
			return fmt.Errorf("failed to get PluggableDatabaseId: %v", err)
		}
		pdbConfig.PluggableDatabaseId = &pdbID
	}

	// Now pdbConfig.PluggableDatabaseId should not be nil
	if pdbConfig.PluggableDatabaseId == nil {
		return fmt.Errorf("PluggableDatabaseId is still nil after retrieval attempt. Nothing to delete")
	}

	// Check if PluggableDatabaseId exists in the live system
	exists, err := r.pluggableDatabaseExists(ctx, *pdbConfig.PluggableDatabaseId)
	if err != nil {
		r.Logger.Error(err, "Failed to check if pluggable database exists", "PluggableDatabaseId", *pdbConfig.PluggableDatabaseId)
		return err
	}
	if !exists {
		r.Logger.Info("PluggableDatabaseId does not exist in the live system, nothing to delete", "PluggableDatabaseId", *pdbConfig.PluggableDatabaseId)
		return nil
	}

	// Define the delete request
	deleteReq := database.DeletePluggableDatabaseRequest{
		PluggableDatabaseId: pdbConfig.PluggableDatabaseId,
	}

	// Call OCI SDK to delete the PDB
	_, err = r.dbClient.DeletePluggableDatabase(ctx, deleteReq)
	if err != nil {
		r.Logger.Error(err, "Failed to delete pluggable database", "PDBName", *pdbConfig.PdbName)
		return err
	}

	r.Logger.Info("Successfully deleted pluggable database", "PDBName", *pdbConfig.PdbName)
	return nil
}

func (r *DbcsSystemReconciler) getPluggableDatabaseID(ctx context.Context, pdbConfig databasev1alpha1.PDBConfig, dbSystemId string) (string, error) {
	compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, dbSystemId)
	if err != nil {
		fmt.Printf("Failed to get compartment ID: %v\n", err)
		return "", err
	}
	request := database.ListPluggableDatabasesRequest{
		CompartmentId: &compartmentId,
	}

	response, err := r.dbClient.ListPluggableDatabases(ctx, request)
	if err != nil {
		return "", fmt.Errorf("failed to list Pluggable Databases: %v", err)
	}

	var pdbID string

	for _, pdb := range response.Items {
		if *pdb.PdbName == *pdbConfig.PdbName {
			pdbID = *pdb.Id
			break
		}
	}

	if pdbID == "" {
		return "", fmt.Errorf("pluggable database '%s' not found", *pdbConfig.PdbName)
	}
	return pdbID, nil
}

// doesPluggableDatabaseExist checks if a pluggable database with the given name exists
func (r *DbcsSystemReconciler) doesPluggableDatabaseExist(ctx context.Context, compartmentId string, pdbName *string) (bool, *string, error) {
	if pdbName == nil {
		return false, nil, fmt.Errorf("pdbName is nil")
	}

	listPdbsReq := database.ListPluggableDatabasesRequest{
		CompartmentId: &compartmentId,
	}

	resp, err := r.dbClient.ListPluggableDatabases(ctx, listPdbsReq)
	if err != nil {
		return false, nil, err
	}

	for _, pdb := range resp.Items {
		if pdb.PdbName != nil && *pdb.PdbName == *pdbName && pdb.LifecycleState != "TERMINATED" {
			return true, pdb.Id, nil
		}
	}

	return false, nil, nil
}
func (r *DbcsSystemReconciler) getSecret(ctx context.Context, namespace, secretName string) (string, error) {
	secret := &corev1.Secret{}
	err := r.KubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	// Assume the secret contains only one key-value pair
	for _, value := range secret.Data {
		return string(value), nil
	}

	return "", fmt.Errorf("secret %s is empty", secretName)
}

func assignDBCSID(dbcsInst *databasev1alpha1.DbcsSystem, dbcsID string) {
	dbcsInst.Spec.Id = &dbcsID
}

func (r *DbcsSystemReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Get the dbName as old dbName when an update event happens
			oldObject := e.ObjectOld.DeepCopyObject().(*databasev1alpha1.DbcsSystem)
			newObject := e.ObjectNew.DeepCopyObject().(*databasev1alpha1.DbcsSystem)
			specObject := !reflect.DeepEqual(oldObject.Spec, newObject.Spec)

			deletionTimeStamp := !reflect.DeepEqual(oldObject.GetDeletionTimestamp(), newObject.GetDeletionTimestamp())

			if specObject || deletionTimeStamp {
				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbcsSystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DbcsSystem{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}).
		Complete(r)
}
