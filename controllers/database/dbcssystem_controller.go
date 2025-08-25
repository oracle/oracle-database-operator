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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcsv4 "github.com/oracle/oracle-database-operator/commons/dbcssystem"
	"github.com/oracle/oracle-database-operator/commons/finalizer"
	"github.com/oracle/oracle-database-operator/commons/oci"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/keymanagement"
	"github.com/oracle/oci-go-sdk/v65/workrequests"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

// +kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems,verbs=get;list;watch;create;update;patch;delete
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
	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}

	// Get the dbcs instance from the cluster
	dbcsInst := &databasev4.DbcsSystem{}
	r.Logger.Info("Reconciling DbSystemDetails", "name", req.NamespacedName)

	if err := r.KubeClient.Get(ctx, req.NamespacedName, dbcsInst); err != nil {
		if errors.IsNotFound(err) {
			// CR was deleted → stop reconciling
			r.Logger.Info("DbcsSystem resource not found.", "name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Create oci-go-sdk client
	authData := oci.ApiKeyAuth{
		ConfigMapName: dbcsInst.Spec.OCIConfigMap,
		SecretName:    dbcsInst.Spec.OCISecret,
		Namespace:     dbcsInst.GetNamespace(),
	}
	provider, err := oci.GetOciProvider(r.KubeClient, authData)
	if err != nil {
		result := resultNq
		return result, err
	}

	r.dbClient, err = database.NewDatabaseClientWithConfigurationProvider(provider)

	if err != nil {
		result := resultNq
		return result, err
	}

	r.nwClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		result := resultNq
		return result, err
	}

	r.wrClient, err = workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	if err != nil {
		result := resultNq
		return result, err
	}

	var compartmentId string
	if dbcsInst.Spec.DbSystem != nil && dbcsInst.Spec.DbSystem.CompartmentId != "" {
		compartmentId = dbcsInst.Spec.DbSystem.CompartmentId
	} else if dbcsInst.Spec.Id != nil && *dbcsInst.Spec.Id != "" {
		var err error
		compartmentId, err = r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
		if err != nil {
			fmt.Printf("Failed to get compartment ID: %v\n", err)
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
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
		if err := dbcsv4.DeleteDbcsSystemSystem(r.dbClient, *dbcsInst.Spec.Id); err != nil {
			r.Logger.Error(err, "Fail to terminate DbcsSystem Instance")
			dbcsInst.Status.Message = err.Error()
			// Change the status to Failed
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Terminate, r.nwClient, r.wrClient); statusErr != nil {
				result := resultNq
				return result, err
			}
			// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			result := resultNq
			return result, err
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
						dbcsInst.Status.Message = err.Error()
						result := resultNq
						return result, err
					}
					result := resultNq
					return result, err
				}
			}
		}
		// Remove the finalizer and update the object
		finalizer.Unregister(r.KubeClient, dbcsInst)
		r.Logger.Info("Finalizer unregistered successfully.")
		// Stop reconciliation as the item is being deleted
		result := resultNq
		return result, err
	}

	/*
		Determine whether it's a provision or bind operation
	*/
	lastSuccessfullSpec, err := dbcsInst.GetLastSuccessfulSpec()
	if err != nil {
		return ctrl.Result{}, err
	}
	lastSuccessfullKMSConfig, err := dbcsInst.GetLastSuccessfulKMSConfig()
	if err != nil {
		return ctrl.Result{}, err
	}
	lastSuccessfullKMSStatus, err := dbcsInst.GetLastSuccessfulKMSStatus()
	if err != nil {
		return ctrl.Result{}, err
	}

	if lastSuccessfullKMSConfig == nil && lastSuccessfullKMSStatus == nil {

		if dbcsInst.Spec.KMSConfig != nil && dbcsInst.Spec.KMSConfig.KeyName != "" {

			kmsVaultClient, err := keymanagement.NewKmsVaultClientWithConfigurationProvider(provider)

			if err != nil {
				return ctrl.Result{}, err
			}

			// Determine the criteria to identify or locate the vault based on provided information
			// Example: Using displayName as a unique identifier (assumed to be unique in this context)
			displayName := dbcsInst.Spec.KMSConfig.VaultName

			// Check if a vault with the given displayName exists
			getVaultReq := keymanagement.ListVaultsRequest{
				CompartmentId: &dbcsInst.Spec.KMSConfig.CompartmentId, // Assuming compartment ID is known or provided
			}

			listResp, err := kmsVaultClient.ListVaults(ctx, getVaultReq)
			if err != nil {
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, fmt.Errorf("error listing vaults: %v", err)
			}

			var existingVaultId *string
			var existingVaultManagementEndpoint *string
			var kmsClient keymanagement.KmsManagementClient
			// Find the first active vault with matching displayName
			for _, vault := range listResp.Items {
				if vault.LifecycleState == keymanagement.VaultSummaryLifecycleStateActive && *vault.DisplayName == displayName {
					existingVaultId = vault.Id
					existingVaultManagementEndpoint = vault.ManagementEndpoint
					// Create KMS Management client
					kmsClient, err = keymanagement.NewKmsManagementClientWithConfigurationProvider(provider, *existingVaultManagementEndpoint)
					if err != nil {
						return ctrl.Result{}, err
					}
					break
				}
			}

			// If no active vault found, create a new one
			if existingVaultId == nil {

				// Create the KMS vault
				createResp, err := r.createKMSVault(ctx, dbcsInst.Spec.KMSConfig, kmsClient, &dbcsInst.Status.KMSDetailsStatus)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, fmt.Errorf("error creating vault: %v", err)
				}
				existingVaultId = createResp.Id
				r.Logger.Info("Created vault Id", existingVaultId)
			} else {
				// Optionally, perform additional checks or operations if needed
				r.Logger.Info("Found existing active vault with displayName", "DisplayName", displayName, "VaultId", *existingVaultId)
				dbcsInst.Status.KMSDetailsStatus.VaultId = *existingVaultId
				dbcsInst.Status.KMSDetailsStatus.ManagementEndpoint = *existingVaultManagementEndpoint
			}
			if existingVaultId != nil {

				// Find the key ID based on compartmentID in the existing vault

				listKeysReq := keymanagement.ListKeysRequest{
					CompartmentId: &dbcsInst.Spec.KMSConfig.CompartmentId,
				}

				var keyId *string
				var keyName *string

				// Make a single request to list keys
				listKeysResp, err := kmsClient.ListKeys(ctx, listKeysReq)
				if err != nil {
					r.Logger.Error(err, "Error listing keys in existing vault")
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}

				// Iterate over the keys to find the desired key
				for _, key := range listKeysResp.Items {
					if key.DisplayName != nil && *key.DisplayName == dbcsInst.Spec.KMSConfig.KeyName {
						keyId = key.Id
						keyName = key.DisplayName
						dbcsInst.Status.KMSDetailsStatus.KeyId = *key.Id
						dbcsInst.Status.KMSDetailsStatus.KeyName = *key.DisplayName
						break
					}
				}

				if keyId == nil {
					r.Logger.Info("Master key not found in existing vault, creating new key")

					// Create the KMS key in the existing vault
					keyResponse, err := r.createKMSKey(ctx, dbcsInst.Spec.KMSConfig, kmsClient, &dbcsInst.Status.KMSDetailsStatus)
					if err != nil {
						return ctrl.Result{}, err
					}

					// Update the DbSystem with the encryption key ID
					dbcsInst.Status.KMSDetailsStatus.KeyId = *keyResponse.Key.Id
					dbcsInst.Status.KMSDetailsStatus.KeyName = *keyResponse.Key.DisplayName
				} else {
					r.Logger.Info("Found existing master key in vault", "KeyName", dbcsInst.Spec.KMSConfig.KeyName, "KeyId", *keyId)

					// Update the DbSystem with the existing encryption key ID
					dbcsInst.Status.KMSDetailsStatus.KeyId = *keyId
					dbcsInst.Status.KMSDetailsStatus.KeyName = *keyName
				}
			} else {
				r.Logger.Info("Creating new vault")

				// Create the new vault
				vaultResponse, err := r.createKMSVault(ctx, dbcsInst.Spec.KMSConfig, kmsClient, &dbcsInst.Status.KMSDetailsStatus)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}
				dbcsInst.Status.KMSDetailsStatus.VaultId = *vaultResponse.Id
				dbcsInst.Status.KMSDetailsStatus.ManagementEndpoint = *vaultResponse.ManagementEndpoint
				// Create the KMS key in the newly created vault
				keyResponse, err := r.createKMSKey(ctx, dbcsInst.Spec.KMSConfig, kmsClient, &dbcsInst.Status.KMSDetailsStatus)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}

				// Update the DbSystem with the encryption key ID
				dbcsInst.Status.KMSDetailsStatus.KeyId = *keyResponse.Key.Id
				dbcsInst.Status.KMSDetailsStatus.KeyName = *keyResponse.Key.DisplayName

			}
		}
	}
	// Backup Creation
	if dbcsInst.Spec.EnableBackup {
		var compartmentId string
		if dbcsInst.Spec.DbSystem.CompartmentId != "" {
			compartmentId = dbcsInst.Spec.DbSystem.CompartmentId
		} else if dbcsInst.Spec.Id != nil && *dbcsInst.Spec.Id != "" {
			var err error
			compartmentId, err = r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
			if err != nil {
				fmt.Printf("Failed to get compartment ID: %v\n", err)
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}
		} else {
			err := fmt.Errorf("compartment ID or DB system ID must be set")
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
		backupId, err := dbcsv4.CreateDbcsBackup(compartmentId, r.Logger, r.dbClient, dbcsInst, r.KubeClient, r.nwClient, r.wrClient)
		if err != nil {
			r.Logger.Error(err, "Backup creation failed")
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, nil
		} else {

			dbHomeId, err := r.getDbHomeIdByDbSystemID(ctx, compartmentId, *dbcsInst.Spec.Id)
			if err != nil {
				fmt.Printf("Failed to get DB Home ID: %v\n", err)
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}

			databaseIds, err := r.getDatabaseIDByDbSystemID(ctx, *dbcsInst.Spec.Id, compartmentId, dbHomeId)
			if err != nil {
				fmt.Printf("Failed to get database IDs: %v\n", err)
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}

			// Assume the first database is the one to back up (customize as needed)
			databaseId := databaseIds[0]
			// After successful creation and backup becomes ACTIVE
			listBackupsReq := database.ListBackupsRequest{
				DatabaseId: &databaseId,
			}

			listBackupsResp, err := r.dbClient.ListBackups(ctx, listBackupsReq)
			if err != nil {
				r.Logger.Error(err, "Failed to list backups")
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, nil
			}

			// Reset and populate status.Backups with up-to-date backup list
			dbcsInst.Status.Backups = []databasev4.BackupInfo{}
			for _, b := range listBackupsResp.Items {
				if b.Id != nil && b.DisplayName != nil && b.TimeStarted != nil {
					dbcsInst.Status.Backups = append(dbcsInst.Status.Backups, databasev4.BackupInfo{
						Name:      *b.DisplayName,
						BackupID:  *b.Id,
						Timestamp: b.TimeStarted.Format(time.RFC3339),
					})
				}
			}
			r.Logger.Info("Backup Completed successfully", "BackupID", backupId)
		}
	}
	restoreCount := 0
	var restoreInfo *databasev4.RestoreConfig
	if dbcsInst.Spec.DbSystem != nil {
		restoreInfo = dbcsInst.Spec.DbSystem.RestoreConfig
	}
	if restoreInfo != nil {
		if restoreInfo.Latest {
			restoreCount++
		}
		if restoreInfo.Timestamp != nil {
			restoreCount++
		}
		if restoreInfo.SCN != nil {
			restoreCount++
		}

		if restoreCount > 1 {
			return ctrl.Result{}, fmt.Errorf("exactly one of Timestamp, SCN, or Latest must be specified for restore")
		}

		aj, _ := json.Marshal(dbcsInst.Spec)
		bj, _ := json.Marshal(lastSuccessfullSpec)

		if bytes.Equal(aj, bj) {
			r.Logger.Info("Restore already applied — skipping", "DbcsSystem", dbcsInst.Name)
		} else if restoreCount == 1 { // do restore when exactly one restore is new spec and defined correctly
			switch {
			case restoreInfo.Latest:
				err = dbcsv4.RestoreDbcsToPoint(compartmentId, r.Logger, r.dbClient, dbcsInst,
					databasev4.RestoreConfig{Latest: true},
					r.KubeClient, r.nwClient, r.wrClient)

			case restoreInfo.Timestamp != nil:
				err = dbcsv4.RestoreDbcsToPoint(compartmentId, r.Logger, r.dbClient, dbcsInst,
					databasev4.RestoreConfig{Timestamp: restoreInfo.Timestamp},
					r.KubeClient, r.nwClient, r.wrClient)

			case restoreInfo.SCN != nil:
				err = dbcsv4.RestoreDbcsToPoint(compartmentId, r.Logger, r.dbClient, dbcsInst,
					databasev4.RestoreConfig{SCN: restoreInfo.SCN},
					r.KubeClient, r.nwClient, r.wrClient)
			}

			if err != nil {
				r.Logger.Error(err, "Restore failed")
				return ctrl.Result{}, err
			}

			// STEP 4: Mark spec as successfully applied
			if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}

			r.Logger.Info("Restore Operation Completed and lastSuccessfulSpec updated", "DbcsSystem", dbcsInst.Name)
			return ctrl.Result{}, nil
		}
	}

	setupCloning := false
	// Check if SetupDBCloning is true and ensure one of the required fields is provided
	if dbcsInst.Spec.SetupDBCloning {
		// If SetupDBCloning is true, at least one of Id, DbBackupId, or DatabaseId must be non-nil
		if dbcsInst.Spec.Id == nil && dbcsInst.Spec.DbBackupId == nil && dbcsInst.Spec.DatabaseId == nil {
			// If none of the required fields are set, log an error and exit the function
			r.Logger.Error(err, "SetupDBCloning is defined but other necessary details (Id, DbBackupId, DatabaseId) are not present. Refer README.md file for instructions.")
			dbcsInst.Status.Message = "SetupDBCloning is defined but other necessary details (Id, DbBackupId, DatabaseId) are not present. Refer README.md file for instructions."
			return ctrl.Result{}, nil
		}
		// If the condition is met, proceed with cloning setup
		setupCloning = true
	} else {
		// If SetupDBCloning is false, continue as usual without cloning
		setupCloning = false
	}

	switch {
	case dbcsInst.Spec.IsPatch && dbcsInst.Spec.IsUpgrade:
		errMsg := "Both IsPatch and IsUpgrade are set. Only one operation can be performed at a time."
		r.Logger.Error(nil, errMsg)
		dbcsInst.Status.Message = errMsg
		return ctrl.Result{}, nil

	case dbcsInst.Spec.IsPatch:
		if dbcsInst.Spec.Id == nil || dbcsInst.Spec.DbSystem.PatchOCID == "" {
			errMsg := "Patching is requested but Patch Version or DB System ID is missing."
			r.Logger.Error(nil, errMsg)
			dbcsInst.Status.Message = errMsg
			return ctrl.Result{}, nil
		}
		compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
		if err != nil {
			fmt.Printf("Failed to get compartment ID: %v\n", err)
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}

		err = dbcsv4.PatchDBSystem(ctx, compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient, dbcsInst.Spec.DbSystem.DbHomeId, dbcsInst.Spec.DbSystem.PatchOCID)
		if err != nil {
			r.Logger.Error(err, "Fail to patch db system")
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, statusErr
			}
		}

	case dbcsInst.Spec.IsUpgrade:
		if dbcsInst.Spec.Id == nil || dbcsInst.Spec.DbSystem.UpgradeVersion == "" {
			errMsg := "Upgrade is requested but Upgrade Version or DB System ID is missing."
			r.Logger.Error(nil, errMsg)
			dbcsInst.Status.Message = errMsg
			return ctrl.Result{}, nil
		}
		compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
		if err != nil {
			fmt.Printf("Failed to get compartment ID: %v\n", err)
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}

		err = dbcsv4.UpgradeDatabaseVersion(ctx, compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient, *dbcsInst.Spec.DatabaseId, dbcsInst.Spec.DbSystem.UpgradeVersion)
		if err != nil {
			r.Logger.Error(err, "Failed to upgrade DB Home.")
			dbcsInst.Status.Message = fmt.Sprintf("Upgrade failed: %v", err)
			return ctrl.Result{}, err
		}

	}

	isDeleteDataguard := false
	setupDataguard := false

	// Check if DataGuard is marked for deletion
	if dbcsInst.Spec.DataGuard.IsDelete {
		isDeleteDataguard = true
		// If marked for delete, skip DataGuard setup
		setupDataguard = false
	} else if dbcsInst.Spec.DataGuard.Enabled {
		// Proceed to check required fields for DataGuard setup
		if dbcsInst.Spec.DataGuard.PrimaryDatabaseId == nil ||
			dbcsInst.Spec.DataGuard.DbAdminPasswordSecret == nil ||
			dbcsInst.Spec.DataGuard.DisplayName == nil {
			r.Logger.Error(err, "setupDataguard is defined but other necessary details are not present. Refer README.md file for instructions.")
			return ctrl.Result{}, nil
		}
		// All required fields are present, enable setup
		setupDataguard = true
	}
	var dbSystemId string
	// Executing DB Cloning Process, if defined. Do not repeat cloning again when Status has Id present.
	if setupCloning && dbcsInst.Status.DbCloneStatus.Id == nil {
		// if setupCloning {

		switch {

		case dbcsInst.Spec.SetupDBCloning && dbcsInst.Spec.DbBackupId != nil:
			dbSystemId, err = dbcsv4.CloneFromBackupAndGetDbcsId(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient)
			if err != nil {
				r.Logger.Error(err, "Fail to clone db system from backup and get DbcsSystem System ID")
				if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, statusErr
				}

				return ctrl.Result{}, nil
			}
			r.Logger.Info("DB Cloning completed successfully from provided backup DB system")

		case dbcsInst.Spec.SetupDBCloning && dbcsInst.Spec.DatabaseId != nil:
			dbSystemId, err = dbcsv4.CloneFromDatabaseAndGetDbcsId(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient)
			if err != nil {
				r.Logger.Error(err, "Fail to clone db system from DatabaseID provided")
				dbcsInst.Status.Message = err.Error()
				if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {

					return ctrl.Result{}, statusErr
				}

				return ctrl.Result{}, nil
			}
			r.Logger.Info("DB Cloning completed successfully from provided databaseId")

		case dbcsInst.Spec.SetupDBCloning && dbcsInst.Spec.DbBackupId == nil && dbcsInst.Spec.DatabaseId == nil:
			dbSystemId, err = dbcsv4.CloneAndGetDbcsId(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient)
			if err != nil {
				r.Logger.Error(err, "Fail to clone db system and get DbcsSystem System ID")
				if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, nil
			}
			r.Logger.Info("DB Cloning completed successfully from provided db system")
		}
	} else if !setupCloning && !setupDataguard && !isDeleteDataguard {
		if dbcsInst.Spec.Id == nil && lastSuccessfullSpec == nil {
			// If no DbcsSystem ID specified, create a new DB System
			// ======================== Validate Specs ==============
			if dbcsInst == nil {
				// Safety guard
				return ctrl.Result{}, nil
			}
			err = dbcsv4.ValidateSpex(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.Recorder)
			if err != nil {
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}
			r.Logger.Info("DbcsSystem DBSystem provisioning")
			dbcsID, err := dbcsv4.CreateAndGetDbcsId(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient, &dbcsInst.Status.KMSDetailsStatus)
			if err != nil {
				dbcsInst.Status.Message = err.Error()
				r.Logger.Error(err, "Fail to provision and get DbcsSystem System ID")

				// Change the status to Failed
				if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}

			assignDBCSID(dbcsInst, dbcsID)
			// Check if KMSConfig is specified
			kmsConfig := dbcsInst.Spec.KMSConfig
			if kmsConfig != nil {
				// Check if KMSDetailsStatus is uninitialized (zero value)
				if dbcsInst.Spec.DbSystem.KMSConfig != nil && dbcsInst.Spec.KMSConfig != nil &&
					*dbcsInst.Spec.DbSystem.KMSConfig != *dbcsInst.Spec.KMSConfig {
					dbcsInst.Spec.DbSystem.KMSConfig = dbcsInst.Spec.KMSConfig
				}
			}
			if err := dbcsv4.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
				// Change the status to Failed
				assignDBCSID(dbcsInst, dbcsID)
				if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}

			r.Logger.Info("DbcsSystem system provisioned succesfully")
			assignDBCSID(dbcsInst, dbcsID)
			if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
				dbcsInst.Status.Message = err.Error()
				return ctrl.Result{}, err
			}
			assignDBCSID(dbcsInst, dbcsID)
		} else {
			if lastSuccessfullSpec == nil { // first time update after creation of DB
				if err := dbcsv4.GetDbSystemId(r.Logger, r.dbClient, dbcsInst); err != nil {
					// Change the status to Failed
					if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
					return ctrl.Result{}, err
				}
				if err := dbcsv4.SetDBCSDatabaseLifecycleState(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
					// Change the status to required state
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}

				dbSystemId := *dbcsInst.Spec.Id
				if err := dbcsv4.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
					// Change the status to Failed
					assignDBCSID(dbcsInst, dbSystemId)
					if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
					return ctrl.Result{}, err
				}

				r.Logger.Info("Sync information from remote DbcsSystem System successfully")

				dbSystemId = *dbcsInst.Spec.Id
				if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}
				assignDBCSID(dbcsInst, dbSystemId)
			} else {
				dbSystemId := ""
				if dbcsInst.Spec.Id == nil {
					dbcsInst.Spec.Id = lastSuccessfullSpec.Id
					dbSystemId = *dbcsInst.Spec.Id
				} else {
					dbSystemId = *dbcsInst.Spec.Id
				}
				//debugging

				compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
				if err != nil {
					fmt.Printf("Failed to get compartment ID: %v\n", err)
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}
				dbHomeId, err := r.getDbHomeIdByDbSystemID(ctx, compartmentId, *dbcsInst.Spec.Id)
				if err != nil {
					fmt.Printf("Failed to get DB Home ID: %v\n", err)
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}

				databaseIds, err := r.getDatabaseIDByDbSystemID(ctx, *dbcsInst.Spec.Id, compartmentId, dbHomeId)
				if err != nil {
					fmt.Printf("Failed to get database IDs: %v\n", err)
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}
				err = r.getPluggableDatabaseDetails(ctx, dbcsInst, *dbcsInst.Spec.Id, databaseIds)
				if err != nil {
					fmt.Printf("Failed to get pluggable database details: %v\n", err)
					dbcsInst.Status.Message = err.Error()
					return ctrl.Result{}, err
				}

				err = r.getDataGuardStatusAndUpdate(ctx, dbcsInst, databaseIds[0], *dbcsInst.Spec.Id)
				if err != nil {
					fmt.Printf("Failed to get dataguard details: %v\n", err)
					return ctrl.Result{}, err
				}

				if err := dbcsv4.UpdateDbcsSystemIdInst(compartmentId, r.Logger, r.dbClient, dbcsInst, r.KubeClient, r.nwClient, r.wrClient, databaseIds[0]); err != nil {
					r.Logger.Error(err, "Fail to update DbcsSystem Id")
					dbcsInst.Status.Message = err.Error()

					// Change the status to Failed
					if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
					// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
					return ctrl.Result{}, nil
				}
				if err := dbcsv4.SetDBCSDatabaseLifecycleState(compartmentId, r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
					// Change the status to required state
					return ctrl.Result{}, err
				}
				// Update Spec and Status
				result, err := r.updateSpecsAndStatus(ctx, dbcsInst, dbSystemId)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					return result, err
				}
			}
		}
	}

	// Update the Wallet Secret when the secret name is given
	//r.updateWalletSecret(dbcs)
	// Dataguard enablement
	switch {
	case setupDataguard:
		// Data Guard Creation Flow
		compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
		if err != nil {
			fmt.Printf("Failed to get compartment ID: %v\n", err)
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
		if err := r.EnableDataGuard(ctx, compartmentId, r.Logger, r.dbClient, dbcsInst, r.KubeClient, r.nwClient, r.wrClient, *dbcsInst.Spec.DataGuard.PrimaryDatabaseId, &dbcsInst.Spec.DataGuard); err != nil {
			r.Logger.Error(err, "Failed to enable Data Guard and update DbcsSystem ID")

			// Update status to Failed
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, nil
		}

		if err := r.KubeClient.Status().Update(ctx, dbcsInst); err != nil {
			r.Logger.Error(err, "Failed to update DB status")
			return reconcile.Result{}, err
		}

		if dbcsInst.Status.DataGuardStatus != nil && dbcsInst.Status.DataGuardStatus.PeerDbSystemId != nil {
			dbSystemId = *dbcsInst.Status.DataGuardStatus.PeerDbSystemId
			assignDBCSID(dbcsInst, dbSystemId)
		}
		if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
			return ctrl.Result{}, err
		}

	case isDeleteDataguard:
		// Data Guard Deletion Flow
		compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, *dbcsInst.Spec.Id)
		if err != nil {
			fmt.Printf("Failed to get compartment ID: %v\n", err)
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
		if err := r.DeleteDataGuard(ctx, compartmentId, r.Logger, r.dbClient, dbcsInst); err != nil {
			r.Logger.Error(err, "Failed to delete Data Guard")

			// Update status to Failed
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, nil
		}

	}

	// Update the last succesful spec
	if dbcsInst.Spec.Id != nil {
		dbSystemId = *dbcsInst.Spec.Id

		if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
			dbcsInst.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
	} else if dbcsInst.Status.DbCloneStatus.Id != nil {
		dbSystemId = *dbcsInst.Status.DbCloneStatus.Id
	} else if setupDataguard {
		dbSystemId = *dbcsInst.Status.DataGuardStatus.PeerDbSystemId
	}

	// Change the phase to "Available"
	assignDBCSID(dbcsInst, dbSystemId)
	if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcsInst, databasev4.Available, r.nwClient, r.wrClient); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	// r.Logger.Info("DBInst after assignment", "dbcsInst:->", dbcsInst)

	// Check if specified PDB exists or needs to be created
	exists, err := r.validatePDBExistence(dbcsInst)
	if err != nil {
		dbcsInst.Status.Message = err.Error()
		fmt.Printf("Failed to get PDB Details: %v\n", err)
		return ctrl.Result{}, err
	}
	if dbcsInst.Spec.PdbConfigs != nil {
		if !exists {
			for _, pdbConfig := range dbcsInst.Spec.PdbConfigs {
				if pdbConfig.PdbName != nil {
					// Get database details
					// Get DB Home ID by DB System ID
					// Get Compartment ID by DB System ID
					compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, dbSystemId)
					if err != nil {
						dbcsInst.Status.Message = err.Error()
						fmt.Printf("Failed to get compartment ID: %v\n", err)
						return ctrl.Result{}, err
					}
					dbHomeId, err := r.getDbHomeIdByDbSystemID(ctx, compartmentId, dbSystemId)
					if err != nil {
						fmt.Printf("Failed to get DB Home ID: %v\n", err)
						dbcsInst.Status.Message = err.Error()
						return ctrl.Result{}, err
					}
					databaseIds, err := r.getDatabaseIDByDbSystemID(ctx, dbSystemId, compartmentId, dbHomeId)
					if err != nil {
						fmt.Printf("Failed to get database IDs: %v\n", err)
						dbcsInst.Status.Message = err.Error()
						return ctrl.Result{}, err
					}

					// Now you can use dbDetails to access database attributes
					r.Logger.Info("Database details fetched successfully", "DatabaseId", databaseIds)

					// Check if deletion is requested
					if pdbConfig.IsDelete != nil && *pdbConfig.IsDelete {
						// Call deletePluggableDatabase function
						if err := r.deletePluggableDatabase(ctx, pdbConfig, dbSystemId); err != nil {
							dbcsInst.Status.Message = err.Error()
							return ctrl.Result{}, err
						}
						// Continue to the next pdbConfig
						continue
					} else {
						// Call the method to create the pluggable database
						r.Logger.Info("Calling createPluggableDatabase", "ctx:->", ctx, "dbcsInst:->", dbcsInst, "databaseIds:->", databaseIds[0], "compartmentId:->", compartmentId)
						pdbId, err := r.createPluggableDatabase(ctx, dbcsInst, pdbConfig, databaseIds[0], compartmentId, dbSystemId)
						if err != nil {
							// Handle error if required
							dbcsInst.Status.Message = err.Error()
							return ctrl.Result{}, err
						}

						// Create or update the PDBConfigStatus in DbcsSystemStatus
						pdbConfigStatus := databasev4.PDBConfigStatus{
							PdbName:                       pdbConfig.PdbName,
							ShouldPdbAdminAccountBeLocked: pdbConfig.ShouldPdbAdminAccountBeLocked,
							PdbLifecycleState:             databasev4.Available,
							FreeformTags:                  pdbConfig.FreeformTags,
							PluggableDatabaseId:           &pdbId,
						}

						// Create a map to track existing PDBConfigStatus by PdbName
						pdbDetailsMap := make(map[string]databasev4.PDBConfigStatus)

						// Populate the map with existing PDBConfigStatus from dbcsInst.Status.PdbDetailsStatus
						for _, pdbDetails := range dbcsInst.Status.PdbDetailsStatus {
							for _, existingPdbConfig := range pdbDetails.PDBConfigStatus {
								pdbDetailsMap[*existingPdbConfig.PdbName] = existingPdbConfig
							}
						}

						// Update the map with the new or updated PDBConfigStatus
						pdbDetailsMap[*pdbConfig.PdbName] = pdbConfigStatus

						// Convert the map back to a slice of PDBDetailsStatus
						var updatedPdbDetailsStatus []databasev4.PDBDetailsStatus
						for _, pdbConfigStatus := range pdbDetailsMap {
							updatedPdbDetailsStatus = append(updatedPdbDetailsStatus, databasev4.PDBDetailsStatus{
								PDBConfigStatus: []databasev4.PDBConfigStatus{pdbConfigStatus},
							})
						}

						// Assign the updated slice to dbcsInst.Status.PdbDetailsStatus
						dbcsInst.Status.PdbDetailsStatus = updatedPdbDetailsStatus
						err = r.KubeClient.Status().Update(ctx, dbcsInst)
						if err != nil {
							dbcsInst.Status.Message = err.Error()
							r.Logger.Error(err, "Failed to update DB status")
							return reconcile.Result{}, err
						}

					}
				}
			}
		} else {
			r.Logger.Info("No change in PDB configurations or, already existed PDB Status.")
		}
	}
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
					dbcsInst.Status.Message = err.Error()
					fmt.Printf("Failed to get compartment ID: %v\n", err)
					return ctrl.Result{}, err
				}
				dbHomeId, err := r.getDbHomeIdByDbSystemID(ctx, compartmentId, dbSystemId)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					fmt.Printf("Failed to get DB Home ID: %v\n", err)
					return ctrl.Result{}, err
				}
				databaseIds, err := r.getDatabaseIDByDbSystemID(ctx, dbSystemId, compartmentId, dbHomeId)
				if err != nil {
					dbcsInst.Status.Message = err.Error()
					fmt.Printf("Failed to get database IDs: %v\n", err)
					return ctrl.Result{}, err
				}

				// Now you can use dbDetails to access database attributes
				r.Logger.Info("Database details fetched successfully", "DatabaseId", databaseIds)

				// Check if deletion is requested
				if pdbConfig.IsDelete != nil && *pdbConfig.IsDelete {
					// Call deletePluggableDatabase function
					if err := r.deletePluggableDatabase(ctx, pdbConfig, dbSystemId); err != nil {
						dbcsInst.Status.Message = err.Error()
						return ctrl.Result{}, err
					}
					// Continue to the next pdbConfig
					continue
				} else {
					// Call the method to create the pluggable database
					r.Logger.Info("Calling createPluggableDatabase", "ctx:->", ctx, "dbcsInst:->", dbcsInst, "databaseIds:->", databaseIds[0], "compartmentId:->", compartmentId)
					_, err := r.createPluggableDatabase(ctx, dbcsInst, pdbConfig, databaseIds[0], compartmentId, dbSystemId)
					if err != nil {
						// Handle error if required
						dbcsInst.Status.Message = err.Error()
						return ctrl.Result{}, err
					}
				}
			}
		}
	}

	return resultQ, nil

}

func (r *DbcsSystemReconciler) DeleteDataGuard(
	ctx context.Context,
	compartmentId string,
	log logr.Logger,
	dbClient database.DatabaseClient,
	dbcsInst *databasev4.DbcsSystem,
) error {
	dataGuardStatus := dbcsInst.Status.DataGuardStatus
	var peerDbSystemId *string

	// Prefer runtime status, fallback to spec if available
	if dataGuardStatus != nil && dataGuardStatus.PeerDbSystemId != nil {
		peerDbSystemId = dataGuardStatus.PeerDbSystemId
	}
	if dbcsInst.Spec.DataGuard.PeerDbSystemId != nil {
		peerDbSystemId = dbcsInst.Spec.DataGuard.PeerDbSystemId
	}

	if peerDbSystemId == nil {
		msg := "Skipping Data Guard deletion — peer DB System ID is nil"
		log.Info(msg)
		dbcsInst.Status.Message = msg
		dbcsInst.Status.DataGuardStatus = nil
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)
		return fmt.Errorf(msg)
	}
	if dbcsInst.Status.DataGuardStatus == nil {
		dbcsInst.Status.DataGuardStatus = &databasev4.DataGuardStatus{}
	}
	status := dbcsInst.Status.DataGuardStatus

	// Use Spec to fill necessary details
	spec := dbcsInst.Spec.DataGuard

	// Populate Status from Spec (if present)
	status.DbAdminPasswordSecret = spec.DbAdminPasswordSecret
	// status.PeerDbSystemId = spec.PeerDbSystemId
	status.PrimaryDatabaseId = dbcsInst.Spec.Id
	status.ProtectionMode = spec.ProtectionMode
	status.TransportType = spec.TransportType
	status.PeerRole = spec.PeerRole
	status.Shape = spec.Shape
	status.SubnetId = spec.SubnetId

	getDbSystemResp, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: peerDbSystemId,
	})
	if err != nil {
		if svcErr, ok := err.(common.ServiceError); ok && svcErr.GetHTTPStatusCode() == 404 {
			msg := fmt.Sprintf("Peer DB System %s already terminated or not found, cleaning up Data Guardstatus",
				func(s *string) string {
					if s == nil {
						return "<nil>"
					}
					return *s
				}(peerDbSystemId),
			)
			log.Info(msg)
			dbcsInst.Status.Message = msg
			dbcsInst.Status.DataGuardStatus = nil
			_ = r.KubeClient.Status().Update(ctx, dbcsInst)
			return nil
		}

		log.Error(err, "Failed to fetch peer DB system status")
		dbcsInst.Status.Message = "Failed to fetch peer DB system status"
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)
		return err
	}

	var primaryDatabaseId *string

	// List DB Homes
	dbHomesResp, err := dbClient.ListDbHomes(ctx, database.ListDbHomesRequest{
		CompartmentId: &compartmentId,
		DbSystemId:    dbcsInst.Spec.Id,
	})
	if err != nil {
		return fmt.Errorf("failed to list DB Homes for system %s: %w", *dbcsInst.Spec.Id, err)
	}

	// Iterate DB Homes
	for _, home := range dbHomesResp.Items {
		dbsResp, err := dbClient.ListDatabases(ctx, database.ListDatabasesRequest{
			CompartmentId: &compartmentId,
			DbHomeId:      home.Id,
		})
		if err != nil {
			return fmt.Errorf("failed to list databases for DB Home %s: %w", *home.Id, err)
		}

		for _, db := range dbsResp.Items {
			// List DG associations for this DB
			dgResp, err := dbClient.ListDataGuardAssociations(ctx, database.ListDataGuardAssociationsRequest{
				DatabaseId: db.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to list Data Guard associations for DB %s: %w", *db.Id, err)
			}

			for _, assoc := range dgResp.Items {
				if assoc.Role == database.DataGuardAssociationSummaryRolePrimary {
					primaryDatabaseId = db.Id
					if dbcsInst.Status.DataGuardStatus == nil {
						dbcsInst.Status.DataGuardStatus = &databasev4.DataGuardStatus{}
					}
					dbcsInst.Status.DataGuardStatus.PrimaryDatabaseId = primaryDatabaseId
					dbcsInst.Status.DataGuardStatus.PeerDbSystemId = assoc.PeerDbSystemId
					break
				}
			}

			if primaryDatabaseId != nil {
				break
			}
		}

		if primaryDatabaseId != nil {
			break
		}
	}

	if primaryDatabaseId == nil {
		msg := fmt.Sprintf(
			"Skipping Data Guard deletion — peer DB %s is not associated with primary DB %s",
			strVal(peerDbSystemId),
			strVal(primaryDatabaseId),
		)
		log.Info(msg)

		dbcsInst.Status.Message = msg

		dbcsInst.Status.Message = msg
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)
		return fmt.Errorf(msg)
	}

	// Verify Data Guard association exists
	listAssocResp, err := dbClient.ListDataGuardAssociations(ctx, database.ListDataGuardAssociationsRequest{
		DatabaseId: primaryDatabaseId,
	})
	if err != nil {
		log.Error(err, "Failed to list Data Guard associations for primary DB")
		dbcsInst.Status.Message = "Failed to list Data Guard associations"
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)
		return err
	}

	var association *database.DataGuardAssociationSummary
	for _, assoc := range listAssocResp.Items {
		if assoc.PeerDbSystemId != nil && *assoc.PeerDbSystemId == *peerDbSystemId {
			association = &assoc
			break
		}
	}

	if association == nil {
		msg := fmt.Sprintf(
			"Skipping Data Guard deletion — peer DB %s is not associated with primary DB %s",
			*peerDbSystemId, primaryDatabaseId,
		)
		log.Info(msg)
		dbcsInst.Status.Message = msg
		if dbcsInst.Status.DataGuardStatus == nil {
			dbcsInst.Status.DataGuardStatus = &databasev4.DataGuardStatus{}
		}
		dbcsInst.Status.DataGuardStatus.LifecycleDetails = &msg

		_ = r.KubeClient.Status().Update(ctx, dbcsInst)

		return fmt.Errorf(msg)
	}

	// At this point, we know the primary and peer DB are actually in Data Guard
	log.Info("Confirmed Data Guard association, proceeding with deletion",
		"primary", primaryDatabaseId,
		"peer", *peerDbSystemId)

	switch getDbSystemResp.DbSystem.LifecycleState {

	case database.DbSystemLifecycleStateTerminated:
		log.Info("Peer DB system is already terminated.", "peerDbSystemId", *peerDbSystemId)

		terminated := string(database.DbSystemLifecycleStateTerminated)
		dbcsInst.Status.DataGuardStatus.LifecycleState = &terminated
		details := "Peer DB system already terminated"
		dbcsInst.Status.DataGuardStatus.LifecycleDetails = &details

		dbcsInst.Status.Message = "Data Guard peer already terminated"

		if statusErr := dbcsv4.SetLifecycleState(
			compartmentId, r.KubeClient, r.dbClient, dbcsInst,
			databasev4.Available, r.nwClient, r.wrClient,
		); statusErr != nil {
			return statusErr
		}

		return r.KubeClient.Status().Update(ctx, dbcsInst)

	case database.DbSystemLifecycleStateTerminating:
		// Wait until it becomes TERMINATED (with timeout)
		log.Info("Peer DB system is in TERMINATING state, waiting for it to reach TERMINATED...",
			"peerDbSystemId", *peerDbSystemId)
		status := dbcsInst.Status.DataGuardStatus

		// Use Spec to fill necessary details
		spec := dbcsInst.Spec.DataGuard

		// Populate Status from Spec (if present)
		status.DbAdminPasswordSecret = spec.DbAdminPasswordSecret
		// status.PeerDbSystemId = spec.PeerDbSystemId
		status.PrimaryDatabaseId = dbcsInst.Spec.Id
		status.ProtectionMode = spec.ProtectionMode
		status.TransportType = spec.TransportType
		status.PeerRole = spec.PeerRole
		status.Shape = spec.Shape
		status.SubnetId = spec.SubnetId
		terminatingState := string(database.DbSystemLifecycleStateTerminating)
		status.LifecycleState = &terminatingState
		status.LifecycleDetails = common.String("Dataguard Peer DB system is terminating...")
		dbcsInst.Status.State = databasev4.Update
		dbcsInst.Status.Message = "Peer DB system is terminating..."
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)

		pollInterval := 30 * time.Second
		maxWait := 30 * time.Minute
		timeout := time.After(maxWait)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timeout:
				dbcsInst.Status.Message = "Timeout while waiting for peer DB system termination"
				_ = r.KubeClient.Status().Update(ctx, dbcsInst)
				return fmt.Errorf("timed out waiting for peer DB system %s to terminate", *peerDbSystemId)

			case <-time.After(pollInterval):
				latest, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
					DbSystemId: peerDbSystemId,
				})
				if err != nil {
					log.Error(err, "Failed to poll DB system lifecycle state")
					return err
				}

				switch latest.DbSystem.LifecycleState {
				case database.DbSystemLifecycleStateTerminated:
					log.Info("Peer DB system successfully terminated", "peerDbSystemId", *peerDbSystemId)
					goto Cleanup
				case database.DbSystemLifecycleStateFailed:
					dbcsInst.Status.Message = "Peer DB system termination failed"
					dbcsInst.Status.State = databasev4.Available
					_ = r.KubeClient.Status().Update(ctx, dbcsInst)
					return fmt.Errorf("DB system termination failed for peer %s", *peerDbSystemId)
				default:
					log.Info("Peer DB system still terminating...",
						"peerDbSystemId", *peerDbSystemId,
						"lifecycleState", latest.DbSystem.LifecycleState)
				}
			}
		}

	default:
		// Active or other state → initiate termination
		if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient,
			dbcsInst, databasev4.Update, r.nwClient, r.wrClient); statusErr != nil {
			return statusErr
		}

		log.Info("Terminating peer DB system", "peerDbSystemId", *peerDbSystemId)
		status := dbcsInst.Status.DataGuardStatus

		// Use Spec to fill necessary details
		spec := dbcsInst.Spec.DataGuard

		// Populate Status from Spec (if present)
		status.DbAdminPasswordSecret = spec.DbAdminPasswordSecret
		// status.PeerDbSystemId = spec.PeerDbSystemId
		status.PrimaryDatabaseId = dbcsInst.Spec.Id
		status.ProtectionMode = spec.ProtectionMode
		status.TransportType = spec.TransportType
		status.PeerRole = spec.PeerRole
		status.Shape = spec.Shape
		status.SubnetId = spec.SubnetId
		terminatingState := string(database.DbSystemLifecycleStateTerminating)
		status.LifecycleState = &terminatingState
		status.LifecycleDetails = common.String("Dataguard Peer DB system is terminating...")
		dbcsInst.Status.State = databasev4.Update
		dbcsInst.Status.Message = "Peer DB system is terminating..."

		dbcsInst.Status.Message = "Initiating peer DB system termination"
		_ = r.KubeClient.Status().Update(ctx, dbcsInst)

		termSysResp, err := dbClient.TerminateDbSystem(ctx, database.TerminateDbSystemRequest{
			DbSystemId: peerDbSystemId,
		})
		if err != nil {
			log.Error(err, "Failed to initiate termination of peer DB System")
			dbcsInst.Status.Message = "Failed to initiate peer DB system termination"
			_ = r.KubeClient.Status().Update(ctx, dbcsInst)
			return err
		}

		if err := r.waitForWorkRequest(ctx, log, termSysResp.OpcWorkRequestId); err != nil {
			log.Error(err, "Peer DB system termination failed or timed out")
			dbcsInst.Status.Message = "Peer DB system termination failed or timed out"
			_ = r.KubeClient.Status().Update(ctx, dbcsInst)
			return err
		}
		if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient,
			dbcsInst, databasev4.Available, r.nwClient, r.wrClient); statusErr != nil {
			return statusErr
		}
	}

Cleanup:
	// Final cleanup — clear DataGuardStatus
	dbcsInst.Status.Message = "Data Guard peer deleted successfully"
	if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient,
		dbcsInst, databasev4.Available, r.nwClient, r.wrClient); statusErr != nil {
		return statusErr
	}
	if err := r.KubeClient.Status().Update(ctx, dbcsInst); err != nil {
		log.Error(err, "Failed to update DB status after deleting Data Guard")
		return err
	}
	log.Info("Successfully deleted Data Guard association")
	return nil
}
func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *DbcsSystemReconciler) waitForWorkRequest(
	ctx context.Context,
	log logr.Logger,
	workRequestID *string,
) error {
	if workRequestID == nil {
		return fmt.Errorf("missing WorkRequest ID")
	}

	log.Info("Waiting for work request to complete", "workRequestID", *workRequestID)
	timeout := time.After(60 * time.Minute)
	pollInterval := 30 * time.Second

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for WorkRequest %s", *workRequestID)
		default:
			resp, err := r.wrClient.GetWorkRequest(ctx, workrequests.GetWorkRequestRequest{
				WorkRequestId: workRequestID,
			})
			if err != nil {
				log.Error(err, "Failed to get WorkRequest status", "workRequestID", *workRequestID)
				return err
			}

			status := resp.WorkRequest.Status
			log.Info("Polling WorkRequest status", "status", status)

			switch status {
			case workrequests.WorkRequestStatusSucceeded:
				log.Info("WorkRequest succeeded", "workRequestID", *workRequestID)
				return nil
			case workrequests.WorkRequestStatusFailed:
				return fmt.Errorf("work request %s failed", *workRequestID)
			}

			time.Sleep(pollInterval)
		}
	}
}

func (r *DbcsSystemReconciler) updateSpecsAndStatus(ctx context.Context, dbcsInst *databasev4.DbcsSystem, dbSystemId string) (reconcile.Result, error) {

	// Retry mechanism for handling resource version conflicts
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the latest version of the resource
		latestDbcsInst := &databasev4.DbcsSystem{}
		err := r.KubeClient.Get(ctx, types.NamespacedName{
			Name:      dbcsInst.Name,
			Namespace: dbcsInst.Namespace,
		}, latestDbcsInst)
		if err != nil {
			r.Logger.Error(err, "Failed to fetch the latest DB resource")
			return err
		}

		// Update the Spec subresource
		latestDbcsInst.Spec.Id = &dbSystemId
		err = r.KubeClient.Update(ctx, latestDbcsInst)
		if err != nil {
			r.Logger.Error(err, "Failed to update DB Spec")
			return err
		}

		// Update the Status subresource

		// Update the Status subresource
		originalStatus := reflect.ValueOf(&dbcsInst.Status).Elem()
		latestStatus := reflect.ValueOf(&latestDbcsInst.Status).Elem()

		// Iterate over all fields in the Status struct and update them
		for i := 0; i < originalStatus.NumField(); i++ {
			fieldName := originalStatus.Type().Field(i).Name
			latestStatus.FieldByName(fieldName).Set(originalStatus.Field(i))
		}

		err = r.KubeClient.Status().Update(ctx, latestDbcsInst)
		if err != nil {
			r.Logger.Error(err, "Failed to update DB status")
			return err
		}

		return nil
	})

	if retryErr != nil {
		r.Logger.Error(retryErr, "Failed to update DB Spec and Status after retries")
		return reconcile.Result{}, retryErr
	}

	r.Logger.Info("Successfully updated Spec and Status")
	return reconcile.Result{}, nil
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
func (r *DbcsSystemReconciler) validatePDBExistence(dbcs *databasev4.DbcsSystem) (bool, error) {
	r.Logger.Info("Validating PDB existence for all provided PDBs")

	// Iterate over each PDBConfig in Spec.PdbConfigs
	for _, pdbConfig := range dbcs.Spec.PdbConfigs {
		pdbName := pdbConfig.PdbName
		r.Logger.Info("Checking PDB existence in Status", "PDBName", *pdbName)

		found := false

		// Check if the PDB exists in Status.PdbDetailsStatus with a state of "Available"
		for _, pdbDetailsStatus := range dbcs.Status.PdbDetailsStatus {
			for _, pdbStatus := range pdbDetailsStatus.PDBConfigStatus {
				if pdbStatus.PdbName != nil && *pdbStatus.PdbName == *pdbName && pdbStatus.PdbLifecycleState == "AVAILABLE" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			r.Logger.Info("Pluggable database does not exist or is not available in Status.PdbDetailsStatus", "PDBName", *pdbName)
			return false, nil
		}
	}

	// If all PDBs are found and available
	r.Logger.Info("All specified PDBs are available")
	return true, nil
}
func (r *DbcsSystemReconciler) createPluggableDatabase(ctx context.Context, dbcs *databasev4.DbcsSystem, pdbConfig databasev4.PDBConfig, databaseId, compartmentId, dbSystemId string) (string, error) {
	r.Logger.Info("Checking if the pluggable database exists", "PDBName", pdbConfig.PdbName)

	// Check if the pluggable database already exists
	exists, pdbId, err := r.doesPluggableDatabaseExist(ctx, compartmentId, pdbConfig.PdbName, databaseId)
	if err != nil {
		r.Logger.Error(err, "Failed to check if pluggable database exists", "PDBName", pdbConfig.PdbName)
		return "", err
	}
	if exists {
		// Set the PluggableDatabaseId in PDBConfig
		pdbConfig.PluggableDatabaseId = pdbId
		r.Logger.Info("Pluggable database already exists", "PDBName", pdbConfig.PdbName, "PluggableDatabaseId", *pdbConfig.PluggableDatabaseId)
		return *pdbId, nil
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
		return "", err
	}

	if !exists {
		errMsg := fmt.Sprintf("Database does not exist: %s", dbSystemId)
		r.Logger.Error(fmt.Errorf(errMsg), "Database not found")
		return "", fmt.Errorf(errMsg)
	}

	// Fetch secrets for TdeWalletPassword and PdbAdminPassword
	tdeWalletPassword, err := r.getSecret(ctx, dbcs.Namespace, *pdbConfig.TdeWalletPassword)
	// Trim newline character from the password
	tdeWalletPassword = strings.TrimSpace(tdeWalletPassword)
	r.Logger.Info("TDE wallet password retrieved successfully")
	if err != nil {
		r.Logger.Error(err, "Failed to get TDE wallet password secret")
		return "", err
	}

	pdbAdminPassword, err := r.getSecret(ctx, dbcs.Namespace, *pdbConfig.PdbAdminPassword)
	// Trim newline character from the password
	pdbAdminPassword = strings.TrimSpace(pdbAdminPassword)
	r.Logger.Info("PDB admin password retrieved successfully")
	if err != nil {
		r.Logger.Error(err, "Failed to get PDB admin password secret")
		return "", err
	}
	// Change the status to Provisioning
	if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcs, databasev4.Provision, r.nwClient, r.wrClient); statusErr != nil {
		r.Logger.Error(err, "Failed to set DBCS LifeCycle State to Provisioning")
		return "", statusErr
	}
	r.Logger.Info("Updated DBCS LifeCycle State to Provisioning")
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
		return "", err
	}
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
			return "", err
		}

		pdbStatus := getPdbResp.PluggableDatabase.LifecycleState
		r.Logger.Info("Checking pluggable database status", "PDBID", *pdbConfig.PluggableDatabaseId, "Status", pdbStatus)

		if pdbStatus == database.PluggableDatabaseLifecycleStateAvailable {
			r.Logger.Info("Pluggable database successfully created", "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
			// Change the status to Available
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcs, databasev4.Available, r.nwClient, r.wrClient); statusErr != nil {
				return "", statusErr
			}
			return *response.PluggableDatabase.Id, nil
		}

		if pdbStatus == database.PluggableDatabaseLifecycleStateFailed {
			r.Logger.Error(fmt.Errorf("pluggable database creation failed"), "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
			// Change the status to Failed
			if statusErr := dbcsv4.SetLifecycleState(compartmentId, r.KubeClient, r.dbClient, dbcs, databasev4.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return "", statusErr
			}
			return "", fmt.Errorf("pluggable database creation failed")
		}

		time.Sleep(retryInterval * time.Second)
	}

	r.Logger.Error(fmt.Errorf("timed out waiting for pluggable database to become available"), "PDBName", pdbConfig.PdbName, "PDBID", *pdbConfig.PluggableDatabaseId)
	return "", fmt.Errorf("timed out waiting for pluggable database to become available")
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

func (r *DbcsSystemReconciler) deletePluggableDatabase(ctx context.Context, pdbConfig databasev4.PDBConfig, dbSystemId string) error {
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

func (r *DbcsSystemReconciler) getPluggableDatabaseID(ctx context.Context, pdbConfig databasev4.PDBConfig, dbSystemId string) (string, error) {
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

func (r *DbcsSystemReconciler) getDataGuardStatusAndUpdate(
	ctx context.Context,
	dbcsInst *databasev4.DbcsSystem,
	primaryDatabaseId string,
	dbSystemId string,
) error {
	log := r.Logger.WithValues("func", "getDataGuardStatusAndUpdate")

	// compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, dbSystemId)
	// if err != nil {
	// 	log.Error(err, "Failed to get compartment ID for DB System")
	// 	return err
	// }

	listRequest := database.ListDataGuardAssociationsRequest{
		DatabaseId: common.String(primaryDatabaseId),
	}

	listResp, err := r.dbClient.ListDataGuardAssociations(ctx, listRequest)
	if err != nil {
		log.Error(err, "Failed to list Data Guard associations")
		return err
	}

	if len(listResp.Items) == 0 {
		// log.Info("No Data Guard associations found")
		dbcsInst.Status.DataGuardStatus = &databasev4.DataGuardStatus{} // reset to empty
		return nil
	}

	// Assuming one-to-one association
	dg := listResp.Items[0]

	status := databasev4.DataGuardStatus{
		Id:                         dg.Id,
		PeerDatabaseId:             dg.PeerDatabaseId,
		PeerDbSystemId:             dg.PeerDbSystemId,
		PeerDbHomeId:               dg.PeerDbHomeId,
		PeerRole:                   (*string)(&dg.Role), // Cast enum to string pointer
		PrimaryDatabaseId:          dg.DatabaseId,
		TransportType:              (*string)(&dg.TransportType),
		ProtectionMode:             (*string)(&dg.ProtectionMode),
		LifecycleState:             (*string)(&dg.LifecycleState),
		LifecycleDetails:           dg.LifecycleDetails,
		PeerDataGuardAssociationId: dg.Id,
	}

	dbcsInst.Status.DataGuardStatus = &status

	log.Info("Updated DataGuardStatus in CR", "DataGuardAssociationId", *dg.Id)
	return nil
}

func (r *DbcsSystemReconciler) getPluggableDatabaseDetails(ctx context.Context, dbcsInst *databasev4.DbcsSystem, dbSystemId string, databaseIds []string) error {
	compartmentId, err := r.getCompartmentIDByDbSystemID(ctx, dbSystemId)
	if err != nil {
		fmt.Printf("Failed to get compartment ID: %v\n", err)
		return err
	}
	request := database.ListPluggableDatabasesRequest{
		CompartmentId: &compartmentId,
	}

	response, err := r.dbClient.ListPluggableDatabases(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to list Pluggable Databases: %v", err)
	}

	// Create a map to track existing PDBDetailsStatus by PdbName
	pdbDetailsMap := make(map[string]databasev4.PDBConfigStatus)

	// Convert databaseIds array to a set for quick lookup
	databaseIdsSet := make(map[string]struct{})
	for _, id := range databaseIds {
		databaseIdsSet[id] = struct{}{}
	}
	// Update the map with new PDB details from the response
	for _, pdb := range response.Items {
		if pdb.ContainerDatabaseId != nil {
			// Check if the ContainerDatabaseId is in the set of databaseIds
			if _, exists := databaseIdsSet[*pdb.ContainerDatabaseId]; exists {
				pdbConfigStatus := databasev4.PDBConfigStatus{
					PdbName:                       pdb.PdbName,
					ShouldPdbAdminAccountBeLocked: pdb.IsRestricted,
					FreeformTags:                  pdb.FreeformTags,
					PluggableDatabaseId:           pdb.Id,
					PdbLifecycleState:             convertLifecycleState(pdb.LifecycleState),
				}

				// Update the map with the new or updated PDBConfigStatus
				pdbDetailsMap[*pdb.PdbName] = pdbConfigStatus
			}
		}
	}

	// Convert the map back to a slice of PDBDetailsStatus
	var updatedPdbDetailsStatus []databasev4.PDBDetailsStatus
	for _, pdbConfigStatus := range pdbDetailsMap {
		updatedPdbDetailsStatus = append(updatedPdbDetailsStatus, databasev4.PDBDetailsStatus{
			PDBConfigStatus: []databasev4.PDBConfigStatus{pdbConfigStatus},
		})
	}

	// Assign the updated slice to dbcsInst.Status.PdbDetailsStatus
	dbcsInst.Status.PdbDetailsStatus = updatedPdbDetailsStatus

	return nil
}

func convertLifecycleState(state database.PluggableDatabaseSummaryLifecycleStateEnum) databasev4.LifecycleState {
	switch state {
	case database.PluggableDatabaseSummaryLifecycleStateProvisioning:
		return databasev4.Provision
	case database.PluggableDatabaseSummaryLifecycleStateAvailable:
		return databasev4.Available
	case database.PluggableDatabaseSummaryLifecycleStateTerminating:
		return databasev4.Terminate
	case database.PluggableDatabaseSummaryLifecycleStateTerminated:
		return databasev4.LifecycleState(databasev4.Terminated)
	case database.PluggableDatabaseSummaryLifecycleStateUpdating:
		return databasev4.Update
	case database.PluggableDatabaseSummaryLifecycleStateFailed:
		return databasev4.Failed
	default:
		return databasev4.Failed
	}
}

// doesPluggableDatabaseExist checks if a pluggable database with the given name exists
func (r *DbcsSystemReconciler) doesPluggableDatabaseExist(ctx context.Context, compartmentId string, pdbName *string, databaseId string) (bool, *string, error) {
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
		if pdb.ContainerDatabaseId != nil {
			if pdb.PdbName != nil && *pdb.PdbName == *pdbName && pdb.LifecycleState != "TERMINATED" && *pdb.ContainerDatabaseId == databaseId {
				return true, pdb.Id, nil
			}
		}
	}

	return false, nil, nil
}

// Enable Dataguard
func (r *DbcsSystemReconciler) EnableDataGuard(
	ctx context.Context,
	compartmentId string,
	log logr.Logger,
	dbClient database.DatabaseClient,
	dbcsSystem *databasev4.DbcsSystem,
	kubeClient client.Client,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
	databaseID string,
	dataGuardConfig *databasev4.DataGuardConfig,
) error {

	// Check if the `Id` field is set
	if databaseID == "" {
		return fmt.Errorf("DbcsSystem.Spec.DataGuard.peerDBSytemID is not set")
	}

	// Extract last successful spec for comparison (optional)
	oldSpec, err := dbcsSystem.GetLastSuccessfulSpecWithLog(log)
	if err != nil {
		log.Error(err, "Failed to get last successful spec for DbcsSystem")
		return err
	}

	// Compare DataGuard configurations to determine if an update is required
	updateFlag := false

	if oldSpec == nil || !reflect.DeepEqual(oldSpec.DataGuard, dataGuardConfig) {
		updateFlag = true
	}

	databaseAdminPassword, err := r.getSecret(ctx, dbcsSystem.Namespace, *dataGuardConfig.DbAdminPasswordSecret)
	if err != nil {
		return err
	}
	// Trim newline character from the password
	databaseAdminPassword = strings.TrimSpace(databaseAdminPassword)
	if updateFlag {

		exists, err := r.checkExistingDataGuardAssociation(ctx, dbClient, dbcsSystem, databaseID, r.KubeClient, r.nwClient, r.wrClient)
		if err != nil {
			return err
		}

		if exists {
			log.Info("Skipping Data Guard creation as it already exists for the database", "DatabaseID", databaseID)
			return nil
		}
		// Change the phase to "Provisioning"
		if statusErr := dbcsv4.SetLifecycleState(compartmentId, kubeClient, dbClient, dbcsSystem, databasev4.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}

		request := database.CreateDataGuardAssociationRequest{
			DatabaseId: common.String(databaseID),
			CreateDataGuardAssociationDetails: database.CreateDataGuardAssociationWithNewDbSystemDetails{
				// CreationType:          common.String("NewDbSystem"),
				DatabaseAdminPassword: common.String(databaseAdminPassword),
				ProtectionMode:        database.CreateDataGuardAssociationDetailsProtectionModeEnum(*dataGuardConfig.ProtectionMode),
				TransportType:         database.CreateDataGuardAssociationDetailsTransportTypeEnum(*dataGuardConfig.TransportType),
				AvailabilityDomain:    dataGuardConfig.AvailabilityDomain,
				DisplayName:           dataGuardConfig.DisplayName,
				Hostname:              dataGuardConfig.HostName,
				Shape:                 dataGuardConfig.Shape,
				SubnetId:              dataGuardConfig.SubnetId,
			},
		}

		response, err := dbClient.CreateDataGuardAssociation(ctx, request)
		// fmt.Printf("Response:\n%+v\n", response)
		if err != nil {
			r.Logger.Error(err, "DbcsSystem did not reach desired state after DataGuard update")
			return err
		}

		r.Logger.Info("Data Guard association creation started")
		if dbcsSystem.Status.DataGuardStatus == nil {
			dbcsSystem.Status.DataGuardStatus = &databasev4.DataGuardStatus{}
		}
		status := dbcsSystem.Status.DataGuardStatus

		// Use Spec to fill necessary details
		spec := dbcsSystem.Spec.DataGuard

		// Populate Status from Spec (if present)
		if spec.DbAdminPasswordSecret != nil {
			status.DbAdminPasswordSecret = spec.DbAdminPasswordSecret
		}

		if spec.PeerDbSystemId != nil {
			status.PeerDbSystemId = spec.PeerDbSystemId
		}
		if dbcsSystem.Spec.Id != nil {
			status.PrimaryDatabaseId = dbcsSystem.Spec.Id
		}
		if spec.ProtectionMode != nil {
			status.ProtectionMode = spec.ProtectionMode
		}

		if spec.TransportType != nil {
			status.TransportType = spec.TransportType
		}

		if spec.PeerRole != nil {
			status.PeerRole = spec.PeerRole
		}

		if spec.Shape != nil {
			status.Shape = spec.Shape
		}

		if spec.SubnetId != nil {
			status.SubnetId = spec.SubnetId
		}

		// Mark lifecycle as provisioning
		provisioningState := string(database.DbSystemLifecycleStateProvisioning)
		status.LifecycleState = &provisioningState
		status.LifecycleDetails = common.String("Dataguard Peer DB system is Provisioning...")

		dbcsSystem.Status.State = databasev4.Update
		dbcsSystem.Status.Message = "Peer DB system is provisioning..."

		dbcsSystem.Status.Message = "Initiating peer DB system provisioning for Data Guard association"
		_ = r.KubeClient.Status().Update(ctx, dbcsSystem)

		// Extract the DataGuardAssociation ID from the response
		associationId := *response.DataGuardAssociation.Id

		// Wait for the update to be applied and resource state to become "AVAILABLE"
		// _, err = dbcsv4.CheckResourceState(log, dbClient, *dbcsSystem.Spec.Id, "UPDATING", "AVAILABLE")
		_, err = dbcsv4.CheckDataGuardAssociationState(log, dbClient, associationId, "UPDATING", "AVAILABLE", databaseID)
		if err != nil {
			r.Logger.Error(err, "Error checking Data Guard Association state")
		}

		r.Logger.Info("Data Guard Association is now in the 'AVAILABLE' state.")

		r.Logger.Info("DataGuard update successful", "dbSystemId", *dbcsSystem.Spec.Id)
	} else {
		r.Logger.Info("No DataGuard update required; configurations match")
	}

	_, err = r.checkExistingDataGuardAssociation(ctx, dbClient, dbcsSystem, databaseID, r.KubeClient, r.nwClient, r.wrClient)
	if err != nil {
		return err
	}

	return nil
}

// Get Dataguard Details
func (r *DbcsSystemReconciler) checkExistingDataGuardAssociation(
	ctx context.Context,
	dbClient database.DatabaseClient,
	dbcsSystem *databasev4.DbcsSystem,
	databaseID string,
	kubeClient client.Client,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
) (bool, error) {

	request := database.ListDataGuardAssociationsRequest{
		DatabaseId: common.String(databaseID),
	}

	response, err := dbClient.ListDataGuardAssociations(ctx, request)
	if err != nil {
		return false, fmt.Errorf("failed to list Data Guard associations: %w", err)
	}

	if len(response.Items) > 0 {
		item := response.Items[0]

		// r.Logger.Info("Data Guard association found for the database", "DatabaseID", databaseID)

		// Ensure DataGuardStatus struct exists
		if dbcsSystem.Status.DataGuardStatus == nil {
			dbcsSystem.Status.DataGuardStatus = &databasev4.DataGuardStatus{}
		}
		status := dbcsSystem.Status.DataGuardStatus

		if item.PeerDbSystemId != nil {
			status.PeerDbSystemId = item.PeerDbSystemId
		}

		if item.DatabaseId != nil {
			status.PrimaryDatabaseId = item.DatabaseId
		}

		status.DbAdminPasswordSecret = dbcsSystem.Spec.DataGuard.DbAdminPasswordSecret

		if item.IsActiveDataGuardEnabled != nil {
			status.IsActiveDataGuardEnabled = *item.IsActiveDataGuardEnabled
		}

		if item.PeerRole != "" {
			s := string(item.PeerRole)
			status.PeerRole = &s
		}

		if item.ProtectionMode != "" {
			s := string(item.ProtectionMode)
			status.ProtectionMode = &s
		}

		if item.TransportType != "" {
			s := string(item.TransportType)
			status.TransportType = &s
		}

		if item.LifecycleState != "" {
			s := string(item.LifecycleState)
			status.LifecycleState = &s
		}

		if item.PeerDataGuardAssociationId != nil {
			status.PeerDataGuardAssociationId = item.PeerDataGuardAssociationId
		}

		if item.LifecycleDetails != nil {
			status.LifecycleDetails = item.LifecycleDetails
		} else {
			status.LifecycleDetails = common.String("Dataguard association enabled for the database")
		}

		if item.Id != nil {
			status.Id = item.Id
		}

		if item.PeerDatabaseId != nil {
			status.PeerDatabaseId = item.PeerDatabaseId
		}

		if item.LifecycleState == database.DataGuardAssociationSummaryLifecycleStateAvailable {
			status.LifecycleDetails = common.String("Data Guard association is available")

			r.Logger.Info("Data Guard association is available", "DatabaseID", databaseID)

			if err := r.KubeClient.Status().Update(ctx, dbcsSystem); err != nil {
				return false, fmt.Errorf("failed to update DbcsSystem status: %w", err)
			}

			return true, nil
		}

		if item.LifecycleState == database.DataGuardAssociationSummaryLifecycleStateFailed {
			if item.LifecycleDetails != nil {
				status.LifecycleDetails = item.LifecycleDetails
			} else {
				status.LifecycleDetails = common.String("Data Guard association is failed")
			}

			_ = r.KubeClient.Status().Update(ctx, dbcsSystem)

			return false, fmt.Errorf("data guard association failed: %s", *status.LifecycleDetails)
		}

		if item.LifecycleState == database.DataGuardAssociationSummaryLifecycleStateProvisioning {
			status.LifecycleDetails = common.String("Data Guard association is getting provisioned")

			if err := r.KubeClient.Status().Update(ctx, dbcsSystem); err != nil {
				return false, fmt.Errorf("failed to update DbcsSystem status during provisioning: %w", err)
			}

			r.Logger.Info("Data Guard association is provisioning", "DatabaseID", databaseID)
			return true, nil
		}

	} else {
		return false, nil
	}
	return false, nil
}

// Function to create KMS vault
func (r *DbcsSystemReconciler) createKMSVault(ctx context.Context, kmsConfig *databasev4.KMSConfig, kmsClient keymanagement.KmsManagementClient, kmsInst *databasev4.KMSDetailsStatus) (*keymanagement.CreateVaultResponse, error) {
	// Dereference the ConfigurationProvider pointer
	configProvider := *kmsClient.ConfigurationProvider()

	kmsVaultClient, err := keymanagement.NewKmsVaultClientWithConfigurationProvider(configProvider)
	if err != nil {
		r.Logger.Error(err, "Error creating KMS vault client")
		return nil, err
	}
	var vaultType keymanagement.CreateVaultDetailsVaultTypeEnum

	if kmsConfig.VaultType != "" {
		switch kmsConfig.VaultType {
		case "VIRTUAL_PRIVATE":
			vaultType = keymanagement.CreateVaultDetailsVaultTypeVirtualPrivate
		case "EXTERNAL":
			vaultType = keymanagement.CreateVaultDetailsVaultTypeExternal
		case "DEFAULT":
			vaultType = keymanagement.CreateVaultDetailsVaultTypeDefault
		default:
			err := fmt.Errorf("unsupported VaultType specified: %s", kmsConfig.VaultType)
			r.Logger.Error(err, "unsupported VaultType specified")
			return nil, err
		}
	} else {
		// Default to DEFAULT if kmsConfig.VaultType is not defined
		vaultType = keymanagement.CreateVaultDetailsVaultTypeDefault
	}

	createVaultReq := keymanagement.CreateVaultRequest{
		CreateVaultDetails: keymanagement.CreateVaultDetails{
			CompartmentId: common.String(kmsConfig.CompartmentId),
			DisplayName:   common.String(kmsConfig.VaultName),
			VaultType:     vaultType,
		},
	}

	resp, err := kmsVaultClient.CreateVault(ctx, createVaultReq)
	if err != nil {
		r.Logger.Error(err, "Error creating KMS vault")
		return nil, err
	}
	// Wait until vault becomes active or timeout
	timeout := time.After(5 * time.Minute) // Example timeout: 5 minutes
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			r.Logger.Error(err, "timed out waiting for vault to become active")
		case <-ticker.C:
			getVaultReq := keymanagement.GetVaultRequest{
				VaultId: resp.Id,
			}

			getResp, err := kmsVaultClient.GetVault(ctx, getVaultReq)
			if err != nil {
				r.Logger.Error(err, "Error getting vault status")
				return nil, err
			}

			if getResp.LifecycleState == keymanagement.VaultLifecycleStateActive {
				r.Logger.Info("KMS vault created successfully and active")
				// Save the vault details into KMSConfig
				kmsInst.VaultId = *getResp.Vault.Id
				kmsInst.ManagementEndpoint = *getResp.Vault.ManagementEndpoint
				kmsInst.VaultName = *getResp.DisplayName
				kmsInst.CompartmentId = *getResp.CompartmentId
				kmsInst.VaultType = kmsConfig.VaultType
				return &keymanagement.CreateVaultResponse{}, err
			}

			r.Logger.Info(fmt.Sprintf("Vault state: %s, waiting for active state...", string(getResp.LifecycleState)))
		}
	}
}

// Function to create KMS key
func (r *DbcsSystemReconciler) createKMSKey(ctx context.Context, kmsConfig *databasev4.KMSConfig, kmsClient keymanagement.KmsManagementClient, kmsInst *databasev4.KMSDetailsStatus) (*keymanagement.CreateKeyResponse, error) {
	// Determine the KeyShape based on the encryption algorithm
	var algorithm keymanagement.KeyShapeAlgorithmEnum
	var keyLength int
	switch kmsConfig.EncryptionAlgo {
	case "AES":
		algorithm = keymanagement.KeyShapeAlgorithmAes
		keyLength = 32
	case "RSA":
		algorithm = keymanagement.KeyShapeAlgorithmRsa
		keyLength = 512
	default:
		// Default to AES if the provided algorithm is unsupported
		algorithm = keymanagement.KeyShapeAlgorithmAes
		keyLength = 32
		r.Logger.Info("Unsupported encryption algorithm. Defaulting to AES.")
	}

	// Create the key shape with the algorithm
	keyShape := keymanagement.KeyShape{
		Algorithm: algorithm,
		Length:    common.Int(keyLength),
	}

	createKeyReq := keymanagement.CreateKeyRequest{
		CreateKeyDetails: keymanagement.CreateKeyDetails{
			CompartmentId: common.String(kmsConfig.CompartmentId),
			DisplayName:   common.String(kmsConfig.KeyName),
			KeyShape:      &keyShape,
		},
		RequestMetadata: common.RequestMetadata{},
	}

	// Call CreateKey without vaultID
	resp, err := kmsClient.CreateKey(ctx, createKeyReq)
	if err != nil {
		r.Logger.Error(err, "Error creating KMS key:")
		return nil, err
	}

	r.Logger.Info("KMS key created successfully:", resp)
	kmsInst.KeyId = *resp.Key.Id
	kmsInst.EncryptionAlgo = string(algorithm)
	return &resp, nil
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

// Convert DbBackupConfigAutoBackupWindowEnum to *string
func autoBackupWindowEnumToStringPtr(enum *database.DbBackupConfigAutoBackupWindowEnum) *string {
	if enum == nil {
		return nil
	}
	value := string(*enum)
	return &value
}
func (r *DbcsSystemReconciler) stringToDbBackupConfigAutoBackupWindowEnum(value *string) (database.DbBackupConfigAutoBackupWindowEnum, error) {
	// Define a default value
	// Define a default value
	const defaultAutoBackupWindow = database.DbBackupConfigAutoBackupWindowOne

	if value == nil {
		return defaultAutoBackupWindow, nil // Return the default value
	}

	// Convert to enum
	enum, ok := database.GetMappingDbBackupConfigAutoBackupWindowEnum(*value)
	if !ok {
		return "", fmt.Errorf("invalid value for AutoBackupWindow: %s", *value)
	}
	return enum, nil
}

func assignDBCSID(dbcsInst *databasev4.DbcsSystem, dbcsID string) {
	dbcsInst.Spec.Id = &dbcsID
}

func (r *DbcsSystemReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Get the dbName as old dbName when an update event happens
			oldObject := e.ObjectOld.DeepCopyObject().(*databasev4.DbcsSystem)
			newObject := e.ObjectNew.DeepCopyObject().(*databasev4.DbcsSystem)
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
		For(&databasev4.DbcsSystem{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}).
		Complete(r)
}
