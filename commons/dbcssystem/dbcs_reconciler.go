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

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/workrequests"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	"github.com/oracle/oracle-database-operator/commons/annotations"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	checkInterval                                                                                        = 30 * time.Second
	timeout                                                                                              = 15 * time.Minute
	PatchHistoryEntrySummaryLifecycleStateInProgress database.PatchHistoryEntrySummaryLifecycleStateEnum = "IN_PROGRESS"
	PatchHistoryEntrySummaryLifecycleStateSucceeded  database.PatchHistoryEntrySummaryLifecycleStateEnum = "SUCCEEDED"
)

func CreateAndGetDbcsId(compartmentId string, logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient, kmsDetails *databasev4.KMSDetailsStatus) (string, error) {

	ctx := context.TODO()
	// Check if DBCS system already exists using the displayName
	listDbcsRequest := database.ListDbSystemsRequest{
		CompartmentId: common.String(dbcs.Spec.DbSystem.CompartmentId),
		DisplayName:   common.String(dbcs.Spec.DbSystem.DisplayName),
	}

	listDbcsResponse, err := dbClient.ListDbSystems(ctx, listDbcsRequest)
	if err != nil {
		return "", err
	}

	// Check if any DBCS system matches the display name
	if len(listDbcsResponse.Items) > 0 {
		for _, dbcsItem := range listDbcsResponse.Items {
			if dbcsItem.DisplayName != nil && *dbcsItem.DisplayName == dbcs.Spec.DbSystem.DisplayName {
				logger.Info("DBCS system already exists", "DBCS ID", *dbcsItem.Id)
				return *dbcsItem.Id, nil
			}
		}
	}

	// Get the admin password from OCI key
	sshPublicKeys, err := getPublicSSHKey(kubeClient, dbcs)
	if err != nil {
		return "", err
	}

	// Get DB SystemOptions
	dbSystemReq := GetDBSystemopts(dbcs)
	licenceModel := getLicenceModel(dbcs)

	// Get DB Home Details
	dbHomeReq, err := GetDbHomeDetails(kubeClient, dbClient, dbcs, "")
	if err != nil {
		return "", err
	}

	// Determine CpuCoreCount
	cpuCoreCount := 2 // default value
	if dbcs.Spec.DbSystem.CpuCoreCount > 0 {
		cpuCoreCount = dbcs.Spec.DbSystem.CpuCoreCount
	}

	// Set up DB system details
	dbcsDetails := database.LaunchDbSystemDetails{
		AvailabilityDomain:         common.String(dbcs.Spec.DbSystem.AvailabilityDomain),
		CompartmentId:              common.String(dbcs.Spec.DbSystem.CompartmentId),
		SubnetId:                   common.String(dbcs.Spec.DbSystem.SubnetId),
		Shape:                      common.String(dbcs.Spec.DbSystem.Shape),
		Domain:                     common.String(dbcs.Spec.DbSystem.Domain),
		DisplayName:                common.String(dbcs.Spec.DbSystem.DisplayName),
		SshPublicKeys:              []string{sshPublicKeys},
		Hostname:                   common.String(dbcs.Spec.DbSystem.HostName),
		CpuCoreCount:               common.Int(cpuCoreCount),
		NodeCount:                  common.Int(GetNodeCount(dbcs)),
		InitialDataStorageSizeInGB: common.Int(GetInitialStorage(dbcs)),
		DbSystemOptions:            &dbSystemReq,
		DbHome:                     &dbHomeReq,
		DatabaseEdition:            GetDBEdition(dbcs),
		DiskRedundancy:             GetDBbDiskRedundancy(dbcs),
		LicenseModel:               database.LaunchDbSystemDetailsLicenseModelEnum(licenceModel),
	}

	if len(dbcs.Spec.DbSystem.Tags) != 0 {
		dbcsDetails.FreeformTags = dbcs.Spec.DbSystem.Tags
	}

	// Add KMS details if available
	if kmsDetails != nil && kmsDetails.VaultId != "" {
		dbcsDetails.KmsKeyId = common.String(kmsDetails.KeyId)
		dbcsDetails.DbHome.Database.KmsKeyId = common.String(kmsDetails.KeyId)
		dbcsDetails.DbHome.Database.VaultId = common.String(kmsDetails.VaultId)
	}

	// Log dbcsDetails for debugging
	logger.Info("Launching DB System with details", "dbcsDetails", dbcsDetails)

	req := database.LaunchDbSystemRequest{LaunchDbSystemDetails: dbcsDetails}

	// Send the request using the service client
	resp, err := dbClient.LaunchDbSystem(ctx, req)
	if err != nil {
		return " ", err
	}

	dbcs.Spec.Id = resp.DbSystem.Id

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Check the State
	_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *resp.DbSystem.Id, nil
}

func parseLicenseModel(licenseModelStr string) (database.DbSystemLicenseModelEnum, error) {
	switch licenseModelStr {
	case "LICENSE_INCLUDED":
		return database.DbSystemLicenseModelLicenseIncluded, nil
	case "BRING_YOUR_OWN_LICENSE":
		return database.DbSystemLicenseModelBringYourOwnLicense, nil
	default:
		return "", fmt.Errorf("invalid license model: %s", licenseModelStr)
	}
}
func convertLicenseModel(licenseModel database.DbSystemLicenseModelEnum) (database.LaunchDbSystemFromDbSystemDetailsLicenseModelEnum, error) {
	switch licenseModel {
	case database.DbSystemLicenseModelLicenseIncluded:
		return database.LaunchDbSystemFromDbSystemDetailsLicenseModelLicenseIncluded, nil
	case database.DbSystemLicenseModelBringYourOwnLicense:
		return database.LaunchDbSystemFromDbSystemDetailsLicenseModelBringYourOwnLicense, nil
	default:
		return "", fmt.Errorf("unsupported license model: %s", licenseModel)
	}
}

func CloneAndGetDbcsId(compartmentId string, logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {
	ctx := context.TODO()
	var err error
	dbAdminPassword := ""
	// tdePassword := ""
	logger.Info("Starting the clone process for DBCS", "dbcs", dbcs)
	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPasswordSecret != "" {
		dbAdminPassword, err = GetCloningAdminPassword(kubeClient, dbcs)
		if err != nil {
			logger.Error(err, "Failed to get DB Admin password")
		}
		// logger.Info(dbAdminPassword)
	}
	// // Log retrieved passwords
	logger.Info("Retrieved passwords from Kubernetes secrets")

	// // // Retrieve the TDE wallet password from Kubernetes secrets
	// // tdePassword, err := GetTdePassword(kubeClient, dbcs.Namespace, dbcs.Spec.TdeWalletPasswordSecretName)
	// // if err != nil {
	// //     logger.Error(err, "Failed to get TDE wallet password from Kubernetes secret", "namespace", dbcs.Namespace, "secretName", dbcs.Spec.TdeWalletPasswordSecretName)
	// //     return "", err
	// // }
	sshPublicKeys, err := getCloningPublicSSHKey(kubeClient, dbcs)
	if err != nil {
		logger.Error(err, "failed to get SSH public key")
	}

	// Fetch the existing DB system details
	existingDbSystem, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: dbcs.Spec.Id,
	})
	if err != nil {
		return "", err
	}

	if compartmentId == "" {
		compartmentId = *existingDbSystem.CompartmentId
	}
	logger.Info("Retrieved existing Db System Details from OCI using Spec.Id")

	// // Create the clone request payload
	// // Create the DbHome details
	// Prepare CreateDatabaseFromDbSystemDetails
	databaseDetails := &database.CreateDatabaseFromDbSystemDetails{
		AdminPassword: &dbAdminPassword,
		DbName:        &dbcs.Spec.DbClone.DbName,
		DbDomain:      existingDbSystem.DbSystem.Domain,
		DbUniqueName:  &dbcs.Spec.DbClone.DbUniqueName,
		FreeformTags:  existingDbSystem.DbSystem.FreeformTags,
		DefinedTags:   existingDbSystem.DbSystem.DefinedTags,
	}
	licenseModelEnum, err := parseLicenseModel(dbcs.Spec.DbClone.LicenseModel)
	if err != nil {
		return "", err
	}
	launchLicenseModel, err := convertLicenseModel(licenseModelEnum)
	if err != nil {
		return "", err
	}

	cloneRequest := database.LaunchDbSystemFromDbSystemDetails{
		CompartmentId:      existingDbSystem.DbSystem.CompartmentId,
		AvailabilityDomain: existingDbSystem.DbSystem.AvailabilityDomain,
		SubnetId:           &dbcs.Spec.DbClone.SubnetId,
		Shape:              existingDbSystem.DbSystem.Shape,
		SshPublicKeys:      []string{sshPublicKeys},
		Hostname:           &dbcs.Spec.DbClone.HostName,
		CpuCoreCount:       existingDbSystem.DbSystem.CpuCoreCount,
		SourceDbSystemId:   existingDbSystem.DbSystem.Id,
		DbHome: &database.CreateDbHomeFromDbSystemDetails{
			Database:     databaseDetails,
			DisplayName:  existingDbSystem.DbSystem.DisplayName,
			FreeformTags: existingDbSystem.DbSystem.FreeformTags,
			DefinedTags:  existingDbSystem.DbSystem.DefinedTags,
		},
		FaultDomains:          existingDbSystem.DbSystem.FaultDomains,
		DisplayName:           &dbcs.Spec.DbClone.DisplayName,
		BackupSubnetId:        existingDbSystem.DbSystem.BackupSubnetId,
		NsgIds:                existingDbSystem.DbSystem.NsgIds,
		BackupNetworkNsgIds:   existingDbSystem.DbSystem.BackupNetworkNsgIds,
		TimeZone:              existingDbSystem.DbSystem.TimeZone,
		DbSystemOptions:       existingDbSystem.DbSystem.DbSystemOptions,
		SparseDiskgroup:       existingDbSystem.DbSystem.SparseDiskgroup,
		Domain:                &dbcs.Spec.DbClone.Domain,
		ClusterName:           existingDbSystem.DbSystem.ClusterName,
		DataStoragePercentage: existingDbSystem.DbSystem.DataStoragePercentage,
		// KmsKeyId:                     existingDbSystem.DbSystem.KmsKeyId,
		// KmsKeyVersionId:              existingDbSystem.DbSystem.KmsKeyVersionId,
		NodeCount:             existingDbSystem.DbSystem.NodeCount,
		FreeformTags:          existingDbSystem.DbSystem.FreeformTags,
		DefinedTags:           existingDbSystem.DbSystem.DefinedTags,
		DataCollectionOptions: existingDbSystem.DbSystem.DataCollectionOptions,
		LicenseModel:          launchLicenseModel,
	}

	// Execute the clone request
	response, err := dbClient.LaunchDbSystem(ctx, database.LaunchDbSystemRequest{
		LaunchDbSystemDetails: cloneRequest,
	})
	if err != nil {
		return "", err
	}

	dbcs.Status.DbCloneStatus.Id = response.DbSystem.Id

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *response.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *response.DbSystem.Id, nil
	// return "", nil
}
func PatchDBSystem(
	ctx context.Context,
	compartmentId string,
	logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	dbcs *databasev4.DbcsSystem,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
	dbHomeId, patchId string) error {

	dbSystemId := dbcs.Spec.Id
	// Check if patch is already applied or in progress then return
	historyResp, err := dbClient.ListDbSystemPatchHistoryEntries(ctx, database.ListDbSystemPatchHistoryEntriesRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		logger.Error(err, "Failed to get patch history entries", "DBSystemID", dbSystemId)
		return fmt.Errorf("failed to get patch history entries: %w", err)
	}

	for _, entry := range historyResp.Items {
		if entry.PatchId != nil && *entry.PatchId == patchId {
			if entry.LifecycleState == database.PatchHistoryEntrySummaryLifecycleStateSucceeded {
				logger.Info("Patch already applied, skipping", "PatchID", patchId)
				return nil
			}
			if entry.LifecycleState == database.PatchHistoryEntrySummaryLifecycleStateInProgress {
				logger.Info("Patch in progress, waiting until it completes", "PatchID", patchId)
				// Change the phase to "Provisioning"
				if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
					logger.Error(statusErr, "Failed to update lifecycle state to Provisioning")
					return statusErr
				}
				// Wait for the patch to complete
				return CheckPatchState(ctx, logger, dbClient, dbSystemId, patchId)
			}
			if entry.LifecycleState == database.PatchHistoryEntrySummaryLifecycleStateFailed {
				logger.Error(fmt.Errorf("patch failed"), "Patch failed", "PatchID", patchId)
				return fmt.Errorf("patch %s failed", patchId)
			}
		}
	}

	logger.Info("Starting patch process for DB System", "DBSystemID", dbSystemId)

	patchesResp, err := dbClient.ListDbSystemPatches(ctx, database.ListDbSystemPatchesRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		logger.Error(err, "Failed to list patches for DB System", "DBSystemID", dbSystemId)
		return fmt.Errorf("failed to list patches for DB System: %w", err)
	}

	found := false

	if len(patchesResp.Items) == 0 {
		logger.Info("No patches available for this DB System", "DBSystemID", dbSystemId)
	} else {
		logger.Info("Available patches for DB System", "count", len(patchesResp.Items))
		for _, patch := range patchesResp.Items {
			logger.Info("Patch found",
				"PatchID", *patch.Id,
				"Description", *patch.Description,
				"LifecycleState", patch.LifecycleState,
				"ReleaseDate", patch.TimeReleased.String(),
			)

			// Check if patchId matches
			if *patch.Id == patchId {
				found = true
			}
		}
	}

	if !found {
		logger.Error(nil, "Patch ID not found in available patches", "PatchID", patchId)
		return fmt.Errorf("patch ID %s not found in available DB System patches", patchId)
	}

	updateDetails := database.UpdateDbSystemDetails{
		Version: &database.PatchDetails{
			PatchId: common.String(patchId),
			Action:  database.PatchDetailsActionApply,
		},
	}

	updateReq := database.UpdateDbSystemRequest{
		DbSystemId:            dbSystemId,
		UpdateDbSystemDetails: updateDetails,
	}

	updateResp, err := dbClient.UpdateDbSystem(ctx, updateReq)
	if err != nil {
		logger.Error(err, "Failed to apply patch to DB System", "PatchID", patchId, "DBSystemID", dbSystemId)
		return fmt.Errorf("failed to patch DB System: %w", err)
	}

	logger.Info("Patch applied to DB System", "WorkRequestID", *updateResp.OpcWorkRequestId, "PatchID", patchId)

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
		logger.Error(statusErr, "Failed to update lifecycle state to Provisioning")
		return statusErr
	}

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *updateResp.DbSystem.Id, string(databasev4.Update), string(databasev4.Available))
	if err != nil {
		logger.Error(err, "Failed to verify DB System state post patching")
		return err
	}

	logger.Info("DB System patching process completed successfully", "DBSystemID", dbSystemId)
	dbcs.Status.Message = "Patch applied successfully."

	return nil
}

// CheckPatchState waits for a specific patch to finish applying.
// It polls the patch history until the patch is SUCCEEDED or FAILED.
func CheckPatchState(ctx context.Context, logger logr.Logger, dbClient database.DatabaseClient, dbSystemId *string, patchId string) error {
	// Maximum wait duration: 120 minutes
	timeout := 240 * time.Minute
	start := time.Now()

	for {
		// Timeout guard
		if time.Since(start) > timeout {
			msg := fmt.Sprintf("timed out after %v waiting for patch %s to complete", timeout, patchId)
			logger.Error(fmt.Errorf("timeout"), msg, "DBSystemID", *dbSystemId)
			return fmt.Errorf(msg)
		}

		historyResp, err := dbClient.ListDbSystemPatchHistoryEntries(ctx, database.ListDbSystemPatchHistoryEntriesRequest{
			DbSystemId: dbSystemId,
		})
		if err != nil {
			logger.Error(err, "Failed to get patch history entries", "DBSystemID", *dbSystemId)
			return fmt.Errorf("failed to get patch history entries: %w", err)
		}

		var found bool
		for _, entry := range historyResp.Items {
			if entry.PatchId != nil && *entry.PatchId == patchId {
				found = true
				switch entry.LifecycleState {
				case database.PatchHistoryEntrySummaryLifecycleStateSucceeded:
					logger.Info("Patch succeeded", "PatchID", patchId)
					return nil

				case database.PatchHistoryEntrySummaryLifecycleStateFailed:
					logger.Error(fmt.Errorf("patch failed"), "Patch failed", "PatchID", patchId)
					return fmt.Errorf("patch %s failed", patchId)

				case database.PatchHistoryEntrySummaryLifecycleStateInProgress:
					logger.Info("Patch still in progress, waiting", "PatchID", patchId)
					time.Sleep(60 * time.Second)
					continue
				}
			}
		}

		if !found {
			logger.Info("Patch ID not found in history yet, waiting", "PatchID", patchId)
			time.Sleep(60 * time.Second)
			continue
		}
	}
}

func UpgradeDatabaseVersion(
	ctx context.Context,
	compartmentId string,
	logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	dbcs *databasev4.DbcsSystem,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
	databaseId, targetVersion string) error {
	dbSystemId := dbcs.Spec.Id
	logger.Info("Starting GI upgrade", "DbSystemID", dbSystemId, "TargetGI", targetVersion)

	// Step 1: Get current DB system details
	getResp, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		logger.Error(err, "Failed to get DB system details", "DbSystemID", dbSystemId)
		return fmt.Errorf("failed to get DB system: %w", err)
	}
	currentGiVersion := getResp.DbSystem.Version
	logger.Info("Current GI version", "CurrentGI", *currentGiVersion)

	// Step 1: Check current GI version
	if *currentGiVersion == targetVersion {
		logger.Info("GI already at target version. Proceeding...")
		// Do NOT return — continue to DB upgrade
	} else {
		// Step 2: Check for ongoing GI upgrade
		workReqsResp, err := wrClient.ListWorkRequests(ctx, workrequests.ListWorkRequestsRequest{
			CompartmentId: common.String(compartmentId),
			ResourceId:    dbSystemId,
		})
		if err != nil {
			logger.Error(err, "Failed to list work requests")
			return fmt.Errorf("failed to list work requests: %w", err)
		}

		for _, wr := range workReqsResp.Items {
			if wr.OperationType != nil && *wr.OperationType == "Upgrade Db System" &&
				(wr.Status == workrequests.WorkRequestSummaryStatusAccepted ||
					wr.Status == workrequests.WorkRequestSummaryStatusInProgress) {
				logger.Info("GI upgrade already in progress", "WorkRequestID", *wr.Id)
				dbcs.Status.Message = "GI upgrade already in progress"
				return nil // Skip further steps while upgrade is ongoing
			}
		}
		// Step 3: Construct the upgrade request
		upgradeReq := database.UpgradeDbSystemRequest{
			DbSystemId: dbSystemId,
			UpgradeDbSystemDetails: database.UpgradeDbSystemDetails{
				Action:                              database.UpgradeDbSystemDetailsActionUpgrade,
				NewGiVersion:                        common.String(targetVersion),
				SnapshotRetentionPeriodInDays:       common.Int(7),
				IsSnapshotRetentionDaysForceUpdated: common.Bool(false),
			},
		}

		// Step 4: Call the API
		upgradeResp, err := dbClient.UpgradeDbSystem(ctx, upgradeReq)
		if err != nil {
			logger.Error(err, "Failed to initiate GI upgrade")
			dbcs.Status.Message = "Failed to initiate GI upgrade"

			return fmt.Errorf("GI upgrade failed: %w", err)
		}

		logger.Info("GI upgrade initiated", "WorkRequestID", *upgradeResp.OpcWorkRequestId)

		// Step 3: Update status to upgrading
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Upgrade, nwClient, wrClient); statusErr != nil {
			logger.Error(statusErr, "Failed to update lifecycle state to Upgrading")
			dbcs.Status.Message = "Failed to update lifecycle state to Upgrading"

			return statusErr
		}

		// Step 4: Check status
		_, err = CheckResourceState(logger, dbClient, *upgradeResp.Id, string(databasev4.Provision), string(databasev4.Available))
		if err != nil {
			logger.Error(err, "Failed to verify database state post upgrade")
			dbcs.Status.Message = "Failed to verify database state post upgrade"

			return err
		}

		// Step 5: Wait for completion
		workReqId := upgradeResp.OpcWorkRequestId
		for {
			time.Sleep(30 * time.Second)

			getWorkReqResp, err := wrClient.GetWorkRequest(ctx, workrequests.GetWorkRequestRequest{
				WorkRequestId: workReqId,
			})
			if err != nil {
				logger.Error(err, "Error fetching work request status", "WorkRequestID", *workReqId)
				dbcs.Status.Message = "Error fetching work request status"
				return fmt.Errorf("failed to check GI upgrade status: %w", err)
			}

			status := getWorkReqResp.WorkRequest.Status
			logger.Info("GI upgrade work request status", "Status", status)

			if status == workrequests.WorkRequestStatusSucceeded {
				logger.Info("GI upgrade completed successfully")
				dbcs.Status.Message = "GI upgrade completed successfully"

				break
			} else if status == workrequests.WorkRequestStatusFailed {
				logger.Error(nil, "GI upgrade failed", "WorkRequestID", *workReqId)
				dbcs.Status.Message = "GI upgrade failed"

				return fmt.Errorf("GI upgrade failed: work request marked as failed")
			}
		}
	}

	// Step 1: Check if DB is already on the target version
	dbResp, err := dbClient.GetDatabase(ctx, database.GetDatabaseRequest{
		DatabaseId: common.String(databaseId),
	})
	if err != nil {
		logger.Error(err, "Failed to fetch database details", "DatabaseID", databaseId)
		dbcs.Status.Message = "Failed to fetch database details"
		return fmt.Errorf("failed to get database details: %w", err)
	}
	// fmt.Printf("%+v\n", dbResp.Database)
	dbHomeId := *dbResp.Database.DbHomeId
	dbHomeResp, err := dbClient.GetDbHome(ctx, database.GetDbHomeRequest{
		DbHomeId: &dbHomeId,
	})
	if err != nil {
		dbcs.Status.Message = "Failed to get DB Home"
		return fmt.Errorf("failed to get DB Home: %w", err)

	}

	currentDbVersion := dbHomeResp.DbHome.DbVersion
	if currentDbVersion != nil && *currentDbVersion == targetVersion {
		logger.Info("Database already at target version. Skipping database upgrade.", "Version", *currentDbVersion)
		// Continue to post-upgrade or next steps if any
		return nil
	}

	// Step 2: Check for ongoing DB upgrade work requests
	workReqsRespDb, err := wrClient.ListWorkRequests(ctx, workrequests.ListWorkRequestsRequest{
		CompartmentId: common.String(compartmentId),
		ResourceId:    common.String(databaseId),
	})
	if err != nil {
		logger.Error(err, "Failed to list database work requests")
		dbcs.Status.Message = "Failed to list database work requests"
		return fmt.Errorf("failed to list database work requests: %w", err)
	}

	for _, wr := range workReqsRespDb.Items {
		if wr.OperationType != nil && *wr.OperationType == "Upgrade Database" &&
			(wr.Status == workrequests.WorkRequestSummaryStatusAccepted ||
				wr.Status == workrequests.WorkRequestSummaryStatusInProgress) {

			logger.Info("Database upgrade already in progress, waiting for completion", "WorkRequestID", *wr.Id)
			dbcs.Status.Message = "Database upgrade already in progress, waiting for completion."

			// Poll the work request status
			for {
				time.Sleep(60 * time.Second)

				workReqResp, err := wrClient.GetWorkRequest(ctx, workrequests.GetWorkRequestRequest{
					WorkRequestId: wr.Id,
				})
				if err != nil {
					logger.Error(err, "Failed to fetch work request status", "WorkRequestID", *wr.Id)
					dbcs.Status.Message = "Failed to fetch work request status for database"
					return fmt.Errorf("failed to get database upgrade work request status: %w", err)
				}

				status := workReqResp.WorkRequest.Status
				logger.Info("Database upgrade work request status. Checking again in 60 seconds.", "Status", status, "WorkRequestID", *wr.Id)

				if status == workrequests.WorkRequestStatusSucceeded {
					logger.Info("Database upgrade completed successfully", "WorkRequestID", *wr.Id)
					break
				} else if status == workrequests.WorkRequestStatusFailed {
					logger.Error(nil, "Database upgrade failed", "WorkRequestID", *wr.Id)
					dbcs.Status.Message = "Database upgrade failed"
					return fmt.Errorf("database upgrade failed: work request marked as failed")
				}
				// continue polling if still in progress
			}
			return nil
		}
	}

	logger.Info("Starting upgrade process for Database", "DatabaseID", databaseId, "TargetVersion", targetVersion)

	upgradeDetails := database.UpgradeDatabaseDetails{
		DatabaseUpgradeSourceDetails: database.DatabaseUpgradeWithDbVersionDetails{
			DbVersion: common.String(targetVersion),
		},
		Action: database.UpgradeDatabaseDetailsActionUpgrade,
	}

	// Step 3: Submit the upgrade request
	upgradeReqDb := database.UpgradeDatabaseRequest{
		DatabaseId:             common.String(databaseId),
		UpgradeDatabaseDetails: upgradeDetails,
	}

	upgradeRespDb, err := dbClient.UpgradeDatabase(ctx, upgradeReqDb)
	if err != nil {
		logger.Error(err, "Failed to upgrade database version", "DatabaseID", databaseId)
		dbcs.Status.Message = "Failed to upgrade database version"

		return fmt.Errorf("failed to upgrade database: %w", err)
	}

	logger.Info("Upgrade initiated", "WorkRequestID", *upgradeRespDb.OpcWorkRequestId)

	// Step 3: Update status to upgrading
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Upgrade, nwClient, wrClient); statusErr != nil {
		logger.Error(statusErr, "Failed to update lifecycle state to Upgrading")
		dbcs.Status.Message = "Failed to update lifecycle state to Upgrading"

		return statusErr
	}

	// Step 4: Check status
	_, err = CheckResourceState(logger, dbClient, *upgradeRespDb.DbSystemId, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		logger.Error(err, "Failed to verify database state post upgrade")
		dbcs.Status.Message = "Upgrade Database Completed successfully."

		return err
	}

	logger.Info("Database upgrade process completed successfully", "DatabaseID", databaseId)
	dbcs.Status.Message = fmt.Sprintf("Database upgraded successfully to version %s", targetVersion)

	return nil
}

// CloneFromBackupAndGetDbcsId clones a DB system from a backup and returns the new DB system's OCID.
func CloneFromBackupAndGetDbcsId(
	compartmentId string,
	logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	dbcs *databasev4.DbcsSystem,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient) (string, error) {
	ctx := context.TODO()

	var err error
	var dbAdminPassword string
	var tdePassword string
	logger.Info("Starting the clone process for DBCS from backup", "dbcs", dbcs)
	backupResp, err := dbClient.GetBackup(ctx, database.GetBackupRequest{
		BackupId: dbcs.Spec.DbBackupId,
	})

	if err != nil {
		fmt.Println("Error getting backup details:", err)
		return "", err
	}
	// Extract the DatabaseId from the backup details
	databaseId := backupResp.Backup.DatabaseId
	// Fetch the existing Database details
	existingDatabase, err := dbClient.GetDatabase(ctx, database.GetDatabaseRequest{
		DatabaseId: databaseId,
	})
	if err != nil {
		logger.Error(err, "Failed to retrieve existing Database details")
		return "", err
	}
	// Check if DbSystemId is available
	dbSystemId := existingDatabase.DbSystemId
	if dbSystemId == nil {
		// handle the case where DbSystemId is not available
		logger.Error(err, "DBSystemId not found")
		return "", err
	}
	dbcs.Spec.Id = dbSystemId

	if compartmentId == "" {
		compartmentId = *existingDatabase.CompartmentId
	}

	// Fetch the existing DB system details
	existingDbSystem, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		return "", err
	}
	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPasswordSecret != "" {
		dbAdminPassword, err = GetCloningAdminPassword(kubeClient, dbcs)
		if err != nil {
			logger.Error(err, "Failed to get DB Admin password")
		}
		// logger.Info(dbAdminPassword)
	}
	// // // Retrieve the TDE wallet password from Kubernetes secrets to open backup DB using TDE Wallet
	if dbcs.Spec.DbClone.TdeWalletPasswordSecret != "" {
		tdePassword, err = GetCloningTdePassword(kubeClient, dbcs)
		if err != nil {
			logger.Error(err, "Failed to get TDE wallet password from Kubernetes secret")
			return "", err
		}
	}

	sshPublicKeys, err := getCloningPublicSSHKey(kubeClient, dbcs)
	if err != nil {
		logger.Error(err, "failed to get SSH public key")
		return "", err
	}

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Create the clone request payload
	cloneRequest := database.LaunchDbSystemFromBackupDetails{
		CompartmentId:      existingDbSystem.DbSystem.CompartmentId,
		AvailabilityDomain: existingDbSystem.DbSystem.AvailabilityDomain,
		SubnetId:           &dbcs.Spec.DbClone.SubnetId,
		Shape:              existingDbSystem.DbSystem.Shape,
		SshPublicKeys:      []string{sshPublicKeys},
		Hostname:           &dbcs.Spec.DbClone.HostName,
		CpuCoreCount:       existingDbSystem.DbSystem.CpuCoreCount,
		DbHome: &database.CreateDbHomeFromBackupDetails{
			Database: &database.CreateDatabaseFromBackupDetails{ // Corrected type here
				BackupId:          dbcs.Spec.DbBackupId,
				AdminPassword:     &dbAdminPassword,
				BackupTDEPassword: &tdePassword,
				DbName:            &dbcs.Spec.DbClone.DbName,
				// DbDomain:      existingDbSystem.DbSystem.Domain,
				DbUniqueName: &dbcs.Spec.DbClone.DbUniqueName,
				// FreeformTags:  existingDbSystem.DbSystem.FreeformTags,
				// DefinedTags:   existingDbSystem.DbSystem.DefinedTags,
				SidPrefix: &dbcs.Spec.DbClone.SidPrefix,
			},
			DisplayName:  existingDbSystem.DbSystem.DisplayName,
			FreeformTags: existingDbSystem.DbSystem.FreeformTags,
			DefinedTags:  existingDbSystem.DbSystem.DefinedTags,
		},
		FaultDomains:                 existingDbSystem.DbSystem.FaultDomains,
		DisplayName:                  &dbcs.Spec.DbClone.DisplayName,
		BackupSubnetId:               existingDbSystem.DbSystem.BackupSubnetId,
		NsgIds:                       existingDbSystem.DbSystem.NsgIds,
		BackupNetworkNsgIds:          existingDbSystem.DbSystem.BackupNetworkNsgIds,
		TimeZone:                     existingDbSystem.DbSystem.TimeZone,
		DbSystemOptions:              existingDbSystem.DbSystem.DbSystemOptions,
		SparseDiskgroup:              existingDbSystem.DbSystem.SparseDiskgroup,
		Domain:                       &dbcs.Spec.DbClone.Domain,
		ClusterName:                  existingDbSystem.DbSystem.ClusterName,
		DataStoragePercentage:        existingDbSystem.DbSystem.DataStoragePercentage,
		InitialDataStorageSizeInGB:   &dbcs.Spec.DbClone.InitialDataStorageSizeInGB,
		KmsKeyId:                     &dbcs.Spec.DbClone.KmsKeyId,
		KmsKeyVersionId:              &dbcs.Spec.DbClone.KmsKeyVersionId,
		NodeCount:                    existingDbSystem.DbSystem.NodeCount,
		FreeformTags:                 existingDbSystem.DbSystem.FreeformTags,
		DefinedTags:                  existingDbSystem.DbSystem.DefinedTags,
		DataCollectionOptions:        existingDbSystem.DbSystem.DataCollectionOptions,
		DatabaseEdition:              database.LaunchDbSystemFromBackupDetailsDatabaseEditionEnum(existingDbSystem.DbSystem.DatabaseEdition),
		LicenseModel:                 database.LaunchDbSystemFromBackupDetailsLicenseModelEnum(existingDbSystem.DbSystem.LicenseModel),
		StorageVolumePerformanceMode: database.LaunchDbSystemBaseStorageVolumePerformanceModeEnum(existingDbSystem.DbSystem.StorageVolumePerformanceMode),
	}

	// Execute the clone request
	response, err := dbClient.LaunchDbSystem(ctx, database.LaunchDbSystemRequest{
		LaunchDbSystemDetails: cloneRequest,
	})
	if err != nil {
		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient); statusErr != nil {
			return "", err
		}
		return "", err
	}

	dbcs.Status.DbCloneStatus.Id = response.DbSystem.Id

	// // Change the phase to "Provisioning"
	// if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
	// 	return "", statusErr
	// }

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *response.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *response.DbSystem.Id, nil
}

// Sync the DbcsSystem Database details
func CloneFromDatabaseAndGetDbcsId(compartmentId string, logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {
	ctx := context.TODO()
	var err error
	dbAdminPassword := ""
	tdePassword := ""
	logger.Info("Starting the clone process for Database", "dbcs", dbcs)

	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPasswordSecret != "" {
		dbAdminPassword, err = GetCloningAdminPassword(kubeClient, dbcs)
		if err != nil {
			logger.Error(err, "Failed to get DB Admin password")
			return "", err
		}
	}
	// // // Retrieve the TDE wallet password from Kubernetes secrets to open backup DB using TDE Wallet
	if dbcs.Spec.DbClone.TdeWalletPasswordSecret != "" {
		tdePassword, err = GetCloningTdePassword(kubeClient, dbcs)
		if err != nil {
			logger.Error(err, "Failed to get TDE wallet password from Kubernetes secret")
			return "", err
		}
	}

	logger.Info("Retrieved passwords from Kubernetes secrets")

	// Fetch the existing Database details
	existingDatabase, err := dbClient.GetDatabase(ctx, database.GetDatabaseRequest{
		DatabaseId: dbcs.Spec.DatabaseId,
	})
	if err != nil {
		logger.Error(err, "Failed to retrieve existing Database details")
		return "", err
	}
	// Check if DbSystemId is available
	dbSystemId := existingDatabase.DbSystemId
	if dbSystemId == nil {
		// handle the case where DbSystemId is not available
		logger.Error(err, "DBSystemId not found")
		return "", err
	}
	dbcs.Spec.Id = dbSystemId
	if compartmentId == "" {
		compartmentId = *existingDatabase.CompartmentId
	}

	// Fetch the existing DB system details
	existingDbSystem, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		return "", err
	}
	logger.Info("Retrieved existing Database details from OCI", "DatabaseId", dbcs.Spec.DatabaseId)

	// Get SSH public key
	sshPublicKeys, err := getCloningPublicSSHKey(kubeClient, dbcs)
	if err != nil {
		logger.Error(err, "Failed to get SSH public key")
		return "", err
	}
	// Before creating the clone request payload, check if a valid backup exists
	backupsResp, err := dbClient.ListBackups(ctx, database.ListBackupsRequest{
		DatabaseId: dbcs.Spec.DatabaseId,
	})
	if err != nil {
		logger.Error(err, "Failed to list backups for database", "DatabaseId", dbcs.Spec.DatabaseId)
		return "", fmt.Errorf("failed to list backups for database %s: %w", dbcs.Spec.DatabaseId, err)
	}

	if len(backupsResp.Items) == 0 {
		msg := fmt.Sprintf("no backups found for database %s, cannot proceed with cloning", dbcs.Spec.DatabaseId)
		logger.Error(nil, msg)
		return "", fmt.Errorf(msg)
	}

	// Optional: ensure at least one backup is in the same AD
	validBackupFound := false
	for _, backup := range backupsResp.Items {
		if backup.LifecycleState == database.BackupSummaryLifecycleStateActive &&
			backup.AvailabilityDomain != nil &&
			*backup.AvailabilityDomain == *existingDbSystem.DbSystem.AvailabilityDomain {

			validBackupFound = true
			logger.Info("Found valid backup for cloning",
				"BackupId", *backup.Id,
				"AvailabilityDomain", *backup.AvailabilityDomain,
				"LifecycleState", backup.LifecycleState)
			break
		}

	}

	if !validBackupFound {
		msg := fmt.Sprintf("no valid backups for database %s found in same AD %s, cannot proceed with cloning",
			*dbcs.Spec.DatabaseId, *existingDbSystem.DbSystem.AvailabilityDomain)
		logger.Error(nil, msg)
		return "", fmt.Errorf(msg)
	}

	logger.Info("Valid backup found for cloning", "DatabaseId", dbcs.Spec.DatabaseId)

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Create the clone request payload
	cloneRequest := database.LaunchDbSystemFromDatabaseDetails{
		CompartmentId:      existingDatabase.CompartmentId,
		AvailabilityDomain: existingDbSystem.DbSystem.AvailabilityDomain,
		SubnetId:           existingDbSystem.DbSystem.SubnetId,
		Shape:              existingDbSystem.DbSystem.Shape,
		SshPublicKeys:      []string{sshPublicKeys},
		Hostname:           &dbcs.Spec.DbClone.HostName,
		CpuCoreCount:       existingDbSystem.DbSystem.CpuCoreCount,
		DatabaseEdition:    database.LaunchDbSystemFromDatabaseDetailsDatabaseEditionEnum(existingDbSystem.DbSystem.DatabaseEdition),
		DbHome: &database.CreateDbHomeFromDatabaseDetails{
			Database: &database.CreateDatabaseFromAnotherDatabaseDetails{
				// Mandatory fields
				DatabaseId: dbcs.Spec.DatabaseId, // Source database ID
				// Optionally fill in other fields if needed
				DbName:        &dbcs.Spec.DbClone.DbName,
				AdminPassword: &dbAdminPassword, // Admin password for the new database
				// The password to open the TDE wallet.
				BackupTDEPassword: &tdePassword,

				DbUniqueName: &dbcs.Spec.DbClone.DbUniqueName,
			},

			// Provide a display name for the new Database Home
			DisplayName:  existingDbSystem.DbSystem.DisplayName,
			FreeformTags: existingDbSystem.DbSystem.FreeformTags,
			DefinedTags:  existingDbSystem.DbSystem.DefinedTags,
		},

		FaultDomains:        existingDbSystem.DbSystem.FaultDomains,
		DisplayName:         &dbcs.Spec.DbClone.DisplayName,
		BackupSubnetId:      existingDbSystem.DbSystem.BackupSubnetId,
		NsgIds:              existingDbSystem.DbSystem.NsgIds,
		BackupNetworkNsgIds: existingDbSystem.DbSystem.BackupNetworkNsgIds,
		TimeZone:            existingDbSystem.DbSystem.TimeZone,
		KmsKeyId:            &dbcs.Spec.DbClone.KmsKeyId,
		KmsKeyVersionId:     &dbcs.Spec.DbClone.KmsKeyVersionId,
		NodeCount:           existingDbSystem.DbSystem.NodeCount,
		FreeformTags:        existingDbSystem.DbSystem.FreeformTags,
		DefinedTags:         existingDbSystem.DbSystem.DefinedTags,
		// PrivateIp:                    &dbcs.Spec.DbClone.PrivateIp,
		InitialDataStorageSizeInGB:   &dbcs.Spec.DbClone.InitialDataStorageSizeInGB,
		LicenseModel:                 database.LaunchDbSystemFromDatabaseDetailsLicenseModelEnum(existingDbSystem.DbSystem.LicenseModel),
		StorageVolumePerformanceMode: database.LaunchDbSystemBaseStorageVolumePerformanceModeEnum(existingDbSystem.DbSystem.StorageVolumePerformanceMode),
	}

	// logger.Info("Launching database clone", "cloneRequest", cloneRequest)

	// Execute the clone request
	response, err := dbClient.LaunchDbSystem(ctx, database.LaunchDbSystemRequest{
		LaunchDbSystemDetails: cloneRequest,
	})
	if err != nil {
		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient); statusErr != nil {
			return "", err
		}
		return "", err
	}

	dbcs.Status.DbCloneStatus.Id = response.DbSystem.Id

	// // Change the phase to "Provisioning"
	// if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
	// 	return "", statusErr
	// }

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *response.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *response.DbSystem.Id, nil
}

// Get admin password from Secret then OCI valut secret
func GetCloningAdminPassword(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbClone.DbAdminPasswordSecret != "" {
		// Get the Admin Secret
		adminSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbClone.DbAdminPasswordSecret,
		}, adminSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "admin-password"
		if val, ok := adminSecret.Data[key]; ok {
			return strings.TrimSpace(string(val)), nil
		} else {
			msg := "secret item not found: admin-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func GetAdminPassword(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.DbAdminPasswordSecret != "" {
		// Get the Admin Secret
		adminSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.DbAdminPasswordSecret,
		}, adminSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "admin-password"
		if val, ok := adminSecret.Data[key]; ok {
			return strings.TrimSpace(string(val)), nil
		} else {
			msg := "secret item not found: admin-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func GetTdePassword(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.TdeWalletPasswordSecret != "" {
		// Get the Admin Secret
		tdeSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.TdeWalletPasswordSecret,
		}, tdeSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "tde-password"
		if val, ok := tdeSecret.Data[key]; ok {
			return strings.TrimSpace(string(val)), nil
		} else {
			msg := "secret item not found: tde-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func GetCloningTdePassword(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbClone.TdeWalletPasswordSecret != "" {
		// Get the Admin Secret
		tdeSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbClone.TdeWalletPasswordSecret,
		}, tdeSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "tde-password"
		if val, ok := tdeSecret.Data[key]; ok {
			return strings.TrimSpace(string(val)), nil
		} else {
			msg := "secret item not found: tde-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func getPublicSSHKey(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.SshPublicKeys[0] != "" {
		// Get the Admin Secret
		sshkeysecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.SshPublicKeys[0],
		}, sshkeysecret)

		if err != nil {
			return "", err
		}

		// Get the admin password`
		key := "publickey"
		if val, ok := sshkeysecret.Data[key]; ok {
			return string(val), nil
		} else {
			msg := "secret item not found: "
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func getCloningPublicSSHKey(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbClone.SshPublicKeys[0] != "" {
		// Get the Admin Secret
		sshkeysecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbClone.SshPublicKeys[0],
		}, sshkeysecret)

		if err != nil {
			return "", err
		}

		// Get the admin password`
		key := "publickey"
		if val, ok := sshkeysecret.Data[key]; ok {
			return string(val), nil
		} else {
			msg := "secret item not found: "
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Delete DbcsSystem System
func DeleteDbcsSystemSystem(dbClient database.DatabaseClient, Id string) error {

	dbcsId := Id

	dbcsReq := database.TerminateDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	_, err := dbClient.TerminateDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	return nil
}

// SetLifecycleState set status.state of the reosurce.
func SetLifecycleState(compartmentId string, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, state databasev4.LifecycleState, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {
	maxRetries := 5
	retryDelay := time.Second * 2

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest version of the object
		latestInstance := &databasev4.DbcsSystem{}
		err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(dbcs), latestInstance)
		if err != nil {
			// Log and return error if fetching the latest version fails
			return fmt.Errorf("failed to fetch the latest version of DBCS instance: %w", err)
		}

		// Merge the instance fields into latestInstance
		err = mergeInstancesFromLatest(dbcs, latestInstance)
		if err != nil {
			return fmt.Errorf("failed to merge instances: %w", err)
		}

		// Set the status using the dbcs object
		if statusErr := SetDBCSStatus(state, compartmentId, dbClient, dbcs, nwClient, wrClient); statusErr != nil {
			return statusErr
		}

		// Update the ResourceVersion of dbcs from latestInstance to avoid conflict
		dbcs.ResourceVersion = latestInstance.ResourceVersion
		// Attempt to patch the status of the instance
		err = kubeClient.Status().Patch(context.TODO(), dbcs, client.MergeFrom(latestInstance))
		if err != nil {
			if apierrors.IsConflict(err) {
				// Handle the conflict and retry
				time.Sleep(retryDelay)
				continue
			}
			// For other errors, log and return the error
			return fmt.Errorf("failed to update the DBCS instance status: %w", err)
		}

		// If no error, break the loop
		break
	}

	return nil
}
func mergeInstancesFromLatest(instance, latestInstance *databasev4.DbcsSystem) error {
	instanceVal := reflect.ValueOf(instance).Elem()
	latestVal := reflect.ValueOf(latestInstance).Elem()

	// Fields to exclude from merging
	excludeFields := map[string]bool{
		"ReleaseUpdate":    true,
		"AsmStorageStatus": true,
	}

	// Loop through the fields in instance
	for i := 0; i < instanceVal.NumField(); i++ {
		field := instanceVal.Type().Field(i)
		instanceField := instanceVal.Field(i)
		latestField := latestVal.FieldByName(field.Name)

		// Skip unexported fields
		if !isExported(field) {
			continue
		}

		// Ensure latestField is valid
		if !latestField.IsValid() || !instanceField.CanSet() {
			continue
		}

		// Skip fields that are in the exclusion list
		if excludeFields[field.Name] {
			continue
		}

		// Handle pointer fields
		if latestField.Kind() == reflect.Ptr {
			if !latestField.IsNil() && instanceField.IsNil() {
				// If instance's field is nil and latest's field is not nil, set the latest's field value
				instanceField.Set(latestField)
			}
			// If instance's field is not nil, do not overwrite
		} else if latestField.Kind() == reflect.String {
			if latestField.String() != "" && latestField.String() != "NOT_DEFINED" && instanceField.String() == "" {
				// If latest's string field is non-empty and not "NOT_DEFINED", and instance's string field is empty, set the value
				instanceField.Set(latestField)
			}
		} else if latestField.Kind() == reflect.Struct {
			// Handle struct types recursively
			mergeStructFields(instanceField, latestField)
		} else {
			// Handle other types if instance's field is zero value
			if reflect.DeepEqual(instanceField.Interface(), reflect.Zero(instanceField.Type()).Interface()) {
				instanceField.Set(latestField)
			}
		}
	}
	return nil
}

func mergeStructFields(instanceField, latestField reflect.Value) {
	for i := 0; i < instanceField.NumField(); i++ {
		subField := instanceField.Type().Field(i)

		instanceSubField := instanceField.Field(i)
		latestSubField := latestField.Field(i)

		if !isExported(subField) || !instanceSubField.CanSet() {
			continue
		}

		if latestSubField.Kind() == reflect.Ptr {
			if !latestSubField.IsNil() && instanceSubField.IsNil() {
				instanceSubField.Set(latestSubField)
			}
		} else if latestSubField.Kind() == reflect.String {
			if latestSubField.String() != "" && latestSubField.String() != "NOT_DEFINED" && instanceSubField.String() == "" {
				instanceSubField.Set(latestSubField)
			}
		} else if latestSubField.Kind() == reflect.Struct {
			mergeStructFields(instanceSubField, latestSubField)
		} else {
			if reflect.DeepEqual(instanceSubField.Interface(), reflect.Zero(instanceSubField.Type()).Interface()) {
				instanceSubField.Set(latestSubField)
			}
		}
	}

}

func isExported(field reflect.StructField) bool {
	return field.PkgPath == ""
}

// SetDBCSSystem LifeCycle state when state is provisioning

func SetDBCSDatabaseLifecycleState(compartmentId string, logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	resp, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	// Return if the desired lifecycle state is the same as the current lifecycle state
	if string(dbcs.Status.State) == string(resp.LifecycleState) {
		return nil
	} else if string(resp.LifecycleState) == string(databasev4.Available) {
		// Change the phase to "Available"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Available, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
	} else if string(resp.LifecycleState) == string(databasev4.Provision) {
		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev4.Update) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Update), string(databasev4.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev4.Failed) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("DbSystem is in Failed State")
	} else if string(resp.LifecycleState) == string(databasev4.Terminated) {
		// Change the phase to "Terminated"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Terminate, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
	} else if string(resp.LifecycleState) == string(databasev4.Upgrade) {
		// Change the phase to "Upgrading"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Upgrade, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Upgrade), string(databasev4.Available))
		if err != nil {
			return err
		}
	}
	return nil
}

func GetDbSystemId(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem) error {
	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	response, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	if dbcs.Spec.DbSystem == nil {
		dbcs.Spec.DbSystem = &databasev4.DbSystemDetails{}
	}
	if response.DbSystem.CompartmentId != nil {
		dbcs.Spec.DbSystem.CompartmentId = *response.DbSystem.CompartmentId
	}
	if response.DbSystem.DisplayName != nil {
		dbcs.Spec.DbSystem.DisplayName = *response.DbSystem.DisplayName
	}

	if response.DbSystem.Hostname != nil {
		dbcs.Spec.DbSystem.HostName = *response.DbSystem.Hostname
	}
	if response.DbSystem.CpuCoreCount != nil {
		dbcs.Spec.DbSystem.CpuCoreCount = *response.DbSystem.CpuCoreCount
	}
	dbcs.Spec.DbSystem.NodeCount = response.DbSystem.NodeCount
	if response.DbSystem.ClusterName != nil {
		dbcs.Spec.DbSystem.ClusterName = *response.DbSystem.ClusterName
	}
	//dbcs.Spec.DbSystem.DbUniqueName = *response.DbSystem.DbUniqueName
	if string(response.DbSystem.DatabaseEdition) != "" {
		dbcs.Spec.DbSystem.DbEdition = string(response.DbSystem.DatabaseEdition)
	}
	if string(response.DbSystem.DiskRedundancy) != "" {
		dbcs.Spec.DbSystem.DiskRedundancy = string(response.DbSystem.DiskRedundancy)
	}

	//dbcs.Spec.DbSystem.DbVersion = *response.DbSystem.

	if response.DbSystem.BackupSubnetId != nil {
		dbcs.Spec.DbSystem.BackupSubnetId = *response.DbSystem.BackupSubnetId
	}
	dbcs.Spec.DbSystem.Shape = *response.DbSystem.Shape
	dbcs.Spec.DbSystem.SshPublicKeys = []string(response.DbSystem.SshPublicKeys)
	if response.DbSystem.FaultDomains != nil {
		dbcs.Spec.DbSystem.FaultDomains = []string(response.DbSystem.FaultDomains)
	}
	dbcs.Spec.DbSystem.SubnetId = *response.DbSystem.SubnetId
	dbcs.Spec.DbSystem.AvailabilityDomain = *response.DbSystem.AvailabilityDomain
	if response.DbSystem.KmsKeyId != nil {
		dbcs.Status.KMSDetailsStatus.KeyId = *response.DbSystem.KmsKeyId
	}
	if response.DbSystem.Version != nil {
		dbcs.Status.DbVersion = *response.DbSystem.Version
	}
	err = PopulateDBDetails(logger, dbClient, dbcs)
	if err != nil {
		logger.Info("Error Occurred while collecting the DB details")
		return err
	}
	return nil
}

func PopulateDBDetails(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem) error {

	listDbHomeRsp, err := GetListDbHomeRsp(logger, dbClient, dbcs)
	if err != nil {
		logger.Info("Error Occurred while getting List of DBHomes")
		return err
	}
	dbHomeId := listDbHomeRsp.Items[0].Id
	listDBRsp, err := GetListDatabaseRsp(logger, dbClient, dbcs, *dbHomeId)
	if err != nil {
		logger.Info("Error Occurred while getting List of Databases")
		return err
	}

	dbcs.Spec.DbSystem.DbName = *listDBRsp.Items[0].DbName
	dbcs.Spec.DbSystem.DbUniqueName = *listDBRsp.Items[0].DbUniqueName
	dbcs.Spec.DbSystem.DbVersion = *listDbHomeRsp.Items[0].DbVersion

	return nil
}

func GetListDbHomeRsp(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem) (database.ListDbHomesResponse, error) {

	dbcsId := *dbcs.Spec.Id
	var CompartmentId string

	// Check if the CompartmentId is defined in dbcs.Spec.DbSystem.CompartmentId
	if dbcs.Spec.DbSystem.CompartmentId != "" {
		CompartmentId = dbcs.Spec.DbSystem.CompartmentId
	} else {
		// If not defined, call GetDbSystem to fetch the details
		getRequest := database.GetDbSystemRequest{
			DbSystemId: &dbcsId,
		}

		// Call GetDbSystem API using the existing dbClient
		getResponse, err := dbClient.GetDbSystem(context.TODO(), getRequest)
		if err != nil {
			return database.ListDbHomesResponse{}, fmt.Errorf("failed to get DB system details: %v", err)
		}

		// Extract the compartment ID from the DB system details
		CompartmentId = *getResponse.DbSystem.CompartmentId
	}

	dbHomeReq := database.ListDbHomesRequest{
		DbSystemId:    &dbcsId,
		CompartmentId: &CompartmentId,
	}

	response, err := dbClient.ListDbHomes(context.TODO(), dbHomeReq)
	if err != nil {
		return database.ListDbHomesResponse{}, err
	}

	return response, nil
}

func GetListDatabaseRsp(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, dbHomeId string) (database.ListDatabasesResponse, error) {

	CompartmentId := dbcs.Spec.DbSystem.CompartmentId

	dbReq := database.ListDatabasesRequest{
		DbHomeId:      &dbHomeId,
		CompartmentId: &CompartmentId,
	}

	response, err := dbClient.ListDatabases(context.TODO(), dbReq)
	if err != nil {
		return database.ListDatabasesResponse{}, err
	}

	return response, nil
}

func UpdateDbcsSystemIdInst(compartmentId string, log logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, kubeClient client.Client, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient, databaseID string) error {
	// log.Info("Existing DB System Getting Updated with new details in UpdateDbcsSystemIdInst")
	var err error
	updateFlag := false
	updateDbcsDetails := database.UpdateDbSystemDetails{}
	// log.Info("Current annotations", "annotations", dbcs.GetAnnotations())
	oldSpec, err := dbcs.GetLastSuccessfulSpecWithLog(log) // Use the new method
	if err != nil {
		log.Error(err, "Failed to get last successful spec")
		return err
	}

	if oldSpec == nil {
		log.Info("oldSpec is nil")
	} else {
		log.Info("Details of oldSpec", "oldSpec", oldSpec)
	}

	if dbcs.Spec.DbSystem == nil {
		dbcs.Spec.DbSystem = &databasev4.DbSystemDetails{}
	}
	if oldSpec.DbSystem == nil {
		oldSpec.DbSystem = &databasev4.DbSystemDetails{}
	}

	// Fetch latest DB System state from OCI
	dbcsId := dbcs.Spec.Id
	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: dbcsId,
	}

	response, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		log.Error(err, "failed to fetch current DB system from OCI")
		return err
	}

	current := response.DbSystem // OCI's current state
	log.Info("Details of updateFlag -> " + fmt.Sprint(updateFlag))
	if dbcs.Spec.DbSystem == nil {
		dbcs.Spec.DbSystem = &databasev4.DbSystemDetails{}
	}
	// Compare and update CPU Core Count
	if dbcs.Spec.DbSystem.CpuCoreCount > 0 &&
		(dbcs.Spec.DbSystem.CpuCoreCount != oldSpec.DbSystem.CpuCoreCount ||
			dbcs.Spec.DbSystem.CpuCoreCount != *current.CpuCoreCount) {

		log.Info("CPU core count change detected",
			"desired", dbcs.Spec.DbSystem.CpuCoreCount,
			"old", oldSpec.DbSystem.CpuCoreCount,
			"current", *current.CpuCoreCount)

		updateDbcsDetails.CpuCoreCount = common.Int(dbcs.Spec.DbSystem.CpuCoreCount)
		updateFlag = true
	}

	// Compare and update Shape
	if dbcs.Spec.DbSystem.Shape != "" &&
		(dbcs.Spec.DbSystem.Shape != oldSpec.DbSystem.Shape ||
			dbcs.Spec.DbSystem.Shape != *current.Shape) {

		log.Info("Shape change detected",
			"desired", dbcs.Spec.DbSystem.Shape,
			"old", oldSpec.DbSystem.Shape,
			"current", *current.Shape)

		updateDbcsDetails.Shape = common.String(dbcs.Spec.DbSystem.Shape)
		updateFlag = true
	}

	// Compare and update License Model
	if dbcs.Spec.DbSystem.LicenseModel != "" &&
		(dbcs.Spec.DbSystem.LicenseModel != oldSpec.DbSystem.LicenseModel ||
			dbcs.Spec.DbSystem.LicenseModel != string(current.LicenseModel)) {

		log.Info("License model change detected",
			"desired", dbcs.Spec.DbSystem.LicenseModel,
			"old", oldSpec.DbSystem.LicenseModel,
			"current", current.LicenseModel)

		licenseModel := getLicenceModel(dbcs)
		updateDbcsDetails.LicenseModel = database.UpdateDbSystemDetailsLicenseModelEnum(licenseModel)
		updateFlag = true
	}

	// Compare Storage Size only against oldSpec (assumes no dynamic resizing supported)
	if dbcs.Spec.DbSystem.InitialDataStorageSizeInGB > 0 &&
		dbcs.Spec.DbSystem.InitialDataStorageSizeInGB != oldSpec.DbSystem.InitialDataStorageSizeInGB {

		log.Info("Data storage size change detected",
			"desired", dbcs.Spec.DbSystem.InitialDataStorageSizeInGB,
			"old", oldSpec.DbSystem.InitialDataStorageSizeInGB)

		updateDbcsDetails.DataStorageSizeInGBs = &dbcs.Spec.DbSystem.InitialDataStorageSizeInGB
		updateFlag = true
	}

	// // Check and update KMS details if necessary
	if dbcs.Spec.KMSConfig != nil {
		if !reflect.DeepEqual(dbcs.Spec.KMSConfig, oldSpec.DbSystem.KMSConfig) {
			log.Info("Updating KMS details in Existing Database")

			kmsKeyID := dbcs.Status.KMSDetailsStatus.KeyId
			vaultID := dbcs.Status.KMSDetailsStatus.VaultId
			tdeWalletPassword := ""
			if dbcs.Spec.DbSystem.TdeWalletPasswordSecret != "" {
				tdeWalletPassword, err = GetTdePassword(kubeClient, dbcs)
				if err != nil {
					log.Error(err, "Failed to get TDE wallet password")
				}
			} else {
				log.Info("Its mandatory to define Tde wallet password when KMS Vault is defined. Not updating existing database")
				return nil
			}
			dbAdminPassword := ""
			if dbcs.Spec.DbSystem.DbAdminPasswordSecret != "" {
				dbAdminPassword, err = GetAdminPassword(kubeClient, dbcs)
				if err != nil {
					log.Error(err, "Failed to get DB Admin password")
				}
			}

			// Assign all available fields to KMSConfig
			dbcs.Spec.DbSystem.KMSConfig = &databasev4.KMSConfig{
				VaultName:      dbcs.Spec.KMSConfig.VaultName,
				CompartmentId:  dbcs.Spec.KMSConfig.CompartmentId,
				KeyName:        dbcs.Spec.KMSConfig.KeyName,
				EncryptionAlgo: dbcs.Spec.KMSConfig.EncryptionAlgo,
				VaultType:      dbcs.Spec.KMSConfig.VaultType,
			}

			// Create the migrate vault key request
			migrateRequest := database.MigrateVaultKeyRequest{
				DatabaseId: common.String(databaseID),
				MigrateVaultKeyDetails: database.MigrateVaultKeyDetails{
					KmsKeyId: common.String(kmsKeyID),
					VaultId:  common.String(vaultID),
				},
			}
			if tdeWalletPassword != "" {
				migrateRequest.TdeWalletPassword = common.String(tdeWalletPassword)
			}
			if dbAdminPassword != "" {
				migrateRequest.AdminPassword = common.String(dbAdminPassword)
			}
			// Change the phase to "Updating"
			if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
				return statusErr
			}
			// Send the request
			migrateResponse, err := dbClient.MigrateVaultKey(context.TODO(), migrateRequest)
			if err != nil {
				log.Error(err, "Failed to migrate vault key")
				return err
			}

			// // Check for additional response details (if any)
			if migrateResponse.RawResponse.StatusCode != 200 {
				log.Error(fmt.Errorf("unexpected status code"), "Migrate vault key request failed", "StatusCode", migrateResponse.RawResponse.StatusCode)
				return fmt.Errorf("MigrateVaultKey request failed with status code %d", migrateResponse.RawResponse.StatusCode)
			}

			log.Info("MigrateVaultKey request succeeded, waiting for database to reach the desired state")

			// // Wait for the database to reach the desired state after migration, timeout for 2 hours
			// Define timeout and check interval
			timeout := 4 * time.Hour
			checkInterval := 1 * time.Minute

			err = WaitForDatabaseState(log, dbClient, databaseID, "AVAILABLE", timeout, checkInterval)
			if err != nil {
				log.Error(err, "Database did not reach the desired state within the timeout period")
				return err
			}
			// Change the phase to "Available"
			if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Available, nwClient, wrClient); statusErr != nil {
				return statusErr
			}

			log.Info("KMS migration process completed successfully")
		}
	}

	log.Info("Details of updateFlag after validations is " + fmt.Sprint(updateFlag))

	if updateFlag {
		cdbId := *dbcs.Status.DbInfo[0].Id
		// Ensure DB system is AVAILABLE
		if err := waitForDbSystemAvailable(cdbId, dbClient, *dbcs.Spec.Id, 30*time.Minute, log); err != nil {
			return fmt.Errorf("cannot update DB system within 30 minutes, wait failed: %w", err)
		}
		updateDbcsRequest := database.UpdateDbSystemRequest{
			DbSystemId:            common.String(*dbcs.Spec.Id),
			UpdateDbSystemDetails: updateDbcsDetails,
		}

		if _, err := dbClient.UpdateDbSystem(context.TODO(), updateDbcsRequest); err != nil {
			return err
		}

		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(log, dbClient, *dbcs.Spec.Id, "UPDATING", "AVAILABLE")
		if err != nil {
			return err
		}
	}

	return nil
}

func waitForDbSystemAvailable(cdbId string, dbClient database.DatabaseClient, dbSystemId string, maxWait time.Duration, log logr.Logger) error {
	start := time.Now()

	for {
		// 1. Check DB System lifecycle state
		dbSysResp, err := dbClient.GetDbSystem(context.TODO(), database.GetDbSystemRequest{
			DbSystemId: &dbSystemId,
		})
		if err != nil {
			return fmt.Errorf("failed to get DB system: %w", err)
		}
		dbSysState := string(dbSysResp.DbSystem.LifecycleState)

		// 2. Check CDB lifecycle state
		dbResp, err := dbClient.GetDatabase(context.TODO(), database.GetDatabaseRequest{
			DatabaseId: &cdbId,
		})
		if err != nil {
			return fmt.Errorf("failed to get CDB database: %w", err)
		}
		dbState := string(dbResp.Database.LifecycleState)

		log.Info("DBSystem state: %s, CDB state: %s", dbSysState, dbState)

		if dbSysState == "AVAILABLE" && dbState == "AVAILABLE" {
			log.Info("Both Db System and Db home is in available state, proceeding with update")
			return nil // both ready
		}

		if time.Since(start) > maxWait {
			return fmt.Errorf("timeout: DBSystem: %s, CDB: %s", dbSysState, dbState)
		}
		log.Info("Either of Db System or Db home is not in available state, rechecking in 30 seconds")

		time.Sleep(30 * time.Second)
	}
}

func isFieldUpdated[T comparable](specVal T, oldVal T, currentVal T) bool {
	return specVal != oldVal || specVal != currentVal
}

func WaitForDatabaseState(
	log logr.Logger,
	dbClient database.DatabaseClient,
	databaseId string,
	desiredState database.DbHomeLifecycleStateEnum,
	timeout time.Duration,
	checkInterval time.Duration,
) error {
	// Set a deadline for the timeout
	deadline := time.Now().Add(timeout)

	log.Info("Starting to wait for the database to reach the desired state", "DatabaseID", databaseId, "DesiredState", desiredState, "Timeout", timeout)

	for time.Now().Before(deadline) {
		// Prepare the request to fetch database details
		getDatabaseReq := database.GetDatabaseRequest{
			DatabaseId: &databaseId,
		}

		// Fetch database details
		databaseResp, err := dbClient.GetDatabase(context.TODO(), getDatabaseReq)
		if err != nil {
			log.Error(err, "Failed to get database details", "DatabaseID", databaseId)
			return err
		}

		// Log the current database state
		log.Info("Database State", "DatabaseID", databaseId, "CurrentState", databaseResp.LifecycleState)

		// Check if the database has reached the desired state
		if databaseResp.LifecycleState == database.DatabaseLifecycleStateEnum(desiredState) {
			log.Info("Database reached the desired state", "DatabaseID", databaseId, "State", desiredState)
			return nil
		}

		// Wait for the specified interval before checking again
		log.Info("Database not in the desired state yet, waiting...", "DatabaseID", databaseId, "CurrentState", databaseResp.LifecycleState, "DesiredState", desiredState, "NextCheckIn", checkInterval)
		time.Sleep(checkInterval)
	}

	// Return an error if the timeout is reached
	err := fmt.Errorf("timed out waiting for database to reach the desired state: %s", desiredState)
	log.Error(err, "Timeout reached while waiting for the database to reach the desired state", "DatabaseID", databaseId)
	return err
}

func UpdateDbcsSystemId(kubeClient client.Client, dbcs *databasev4.DbcsSystem) error {
	payload := []annotations.PatchValue{{
		Op:    "replace",
		Path:  "/spec/details",
		Value: dbcs.Spec,
	}}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.JSONPatchType, payloadBytes)
	return kubeClient.Patch(context.TODO(), dbcs, patch)
}

// CheckDataGuardAssociationState will check the lifecycle state of the Data Guard Association
// and wait until it reaches the expected state (e.g., "AVAILABLE").
func CheckDataGuardAssociationState(logger logr.Logger, dbClient database.DatabaseClient, associationId string, currentState string, expectedState string, databaseId string) (string, error) {
	// The DataGuard Association OCID is not available when provisioning is ongoing.
	// Retry until the new Data Guard Association is ready.

	var state string
	var err error
	for {
		state, err = GetDataGuardAssociationState(logger, dbClient, associationId, databaseId)
		if err != nil {
			logger.Info("Error occurred while collecting the resource lifecycle state")
			return "", err
		}
		if string(state) == expectedState {
			break
		} else if string(state) != expectedState {
			logger.Info("Data Guard Association current state is still:" + string(state) + ". Sleeping for 60 seconds.")
			time.Sleep(60 * time.Second)
			continue
		}
	}

	return "", nil
}

// GetDataGuardAssociationState retrieves the lifecycle state of the Data Guard Association.
func GetDataGuardAssociationState(logger logr.Logger, dbClient database.DatabaseClient, associationId string, databaseID string) (string, error) { // Context with 2-hour timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	request := database.GetDataGuardAssociationRequest{
		DatabaseId:             common.String(databaseID),
		DataGuardAssociationId: common.String(associationId),
	}
	desiredState := "AVAILABLE"

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 2-hour timeout reached
			return "", context.DeadlineExceeded
		case <-ticker.C:
			logger.Info("Polling Data Guard association state", "DatabaseID", databaseID, "AssociationID", associationId)

			// Make the request using the context
			response, err := dbClient.GetDataGuardAssociation(ctx, request)
			if err != nil {
				logger.Error(err, "Failed to get Data Guard association state")
				return "", err
			}

			state := string(response.LifecycleState)
			logger.Info("Current state", "State", state)

			if state == desiredState {
				return state, nil
			}
		}
	}
}

// CheckResourceState waits until the resource moves from a transient state (e.g. PROVISIONING, UPDATING)
// to the expected state (e.g. AVAILABLE). It retries until success, timeout (120 minutes), or unexpected state occurs.
func CheckResourceState(logger logr.Logger, dbClient database.DatabaseClient, id string, transientState string, expectedState string) (string, error) {
	var state string
	var err error

	timeout := 240 * time.Minute
	start := time.Now()

	for {
		// Timeout guard
		if time.Since(start) > timeout {
			msg := fmt.Sprintf("timed out after %v waiting for DB System %s to reach state %s (last known state: %s)",
				timeout, id, expectedState, state)
			logger.Error(fmt.Errorf("timeout"), msg, "Id", id)
			return state, errors.New(msg)
		}

		state, err = GetResourceState(logger, dbClient, id)
		if err != nil {
			logger.Error(err, "Error occurred while collecting the resource lifecycle state", "Id", id)
			return "", err
		}

		switch state {
		case expectedState:
			logger.Info("DB System reached expected state", "State", state, "Id", id)
			return state, nil
		// Explicitly handle UPDATING as transient state (for patching)
		case string(database.DbSystemLifecycleStateUpdating):
			logger.Info("DB System is still updating ", "State", state, "Id", id)
			time.Sleep(60 * time.Second)
			continue
		case string(database.DbSystemLifecycleStateUpgrading):
			logger.Info("DB System is still upgrading ", "State", state, "Id", id)
			time.Sleep(60 * time.Second)
			continue
		case transientState:
			logger.Info("DB System still in transient state", "State", state, "Id", id)
			time.Sleep(60 * time.Second) // sleep before re-checking
			continue

		default:
			msg := fmt.Sprintf("DB System state %s is not matching expected state %s", state, expectedState)
			logger.Error(errors.New(msg), "Unexpected DB System state", "Id", id)
			return state, errors.New(msg)
		}
	}
}

// GetResourceState fetches the current lifecycle state of the DbSystem from OCI.
func GetResourceState(logger logr.Logger, dbClient database.DatabaseClient, id string) (string, error) {
	req := database.GetDbSystemRequest{
		DbSystemId: common.String(id),
	}

	resp, err := dbClient.GetDbSystem(context.TODO(), req)
	if err != nil {
		return "", err
	}

	state := string(resp.LifecycleState)
	logger.Info("Fetched DB System lifecycle state", "State", state, "Id", id)
	return state, nil
}

func SetDBCSStatus(state databasev4.LifecycleState, compartmentId string, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

	if dbcs.Spec.Id == nil {
		dbcs.Status.State = "FAILED"
		return nil
	}

	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	resp, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}
	compartmentId = *resp.CompartmentId

	dbcs.Status.AvailabilityDomain = *resp.AvailabilityDomain
	dbcs.Status.CpuCoreCount = *resp.CpuCoreCount
	dbcs.Status.DataStoragePercentage = resp.DataStoragePercentage
	dbcs.Status.DataStorageSizeInGBs = resp.DataStorageSizeInGBs
	dbcs.Status.DbEdition = string(resp.DatabaseEdition)
	dbcs.Status.DisplayName = *resp.DisplayName
	dbcs.Status.LicenseModel = string(resp.LicenseModel)
	dbcs.Status.RecoStorageSizeInGB = resp.RecoStorageSizeInGB
	dbcs.Status.NodeCount = *resp.NodeCount
	dbcs.Status.StorageManagement = string(resp.DbSystemOptions.StorageManagement)
	dbcs.Status.Shape = resp.Shape
	dbcs.Status.Id = resp.Id
	dbcs.Status.SubnetId = *resp.SubnetId
	dbcs.Status.TimeZone = *resp.TimeZone
	dbcs.Status.LicenseModel = string(resp.LicenseModel)
	dbcs.Status.Network.ScanDnsName = resp.ScanDnsName
	dbcs.Status.Network.ListenerPort = resp.ListenerPort
	dbcs.Status.Network.HostName = *resp.Hostname
	dbcs.Status.Network.DomainName = *resp.Domain
	if resp.Version != nil {
		dbcs.Status.DbVersion = *resp.Version
	} else {
		dbcs.Status.DbVersion = ""
	}

	if dbcs.Spec.KMSConfig != nil && dbcs.Spec.KMSConfig.CompartmentId != "" {
		dbcs.Status.KMSDetailsStatus.CompartmentId = dbcs.Spec.KMSConfig.CompartmentId
		dbcs.Status.KMSDetailsStatus.VaultName = dbcs.Spec.KMSConfig.VaultName
	}
	if state == "" {
		dbcs.Status.State = databasev4.LifecycleState(resp.LifecycleState)
	} else {
		dbcs.Status.State = state
	}
	if dbcs.Spec.KMSConfig != nil && dbcs.Spec.KMSConfig.CompartmentId != "" {
		dbcs.Status.KMSDetailsStatus.CompartmentId = dbcs.Spec.KMSConfig.CompartmentId
		dbcs.Status.KMSDetailsStatus.VaultName = dbcs.Spec.KMSConfig.VaultName
	}

	sname, vcnId, err := getSubnetName(*resp.SubnetId, nwClient)

	if err == nil {
		dbcs.Status.Network.SubnetName = sname
		vcnName, err := getVcnName(vcnId, nwClient)

		if err == nil {
			dbcs.Status.Network.VcnName = vcnName
		}

	}

	// Work Request Ststaus
	dbWorkRequest := databasev4.DbWorkrequests{}

	dbWorks, err := getWorkRequest(compartmentId, *resp.OpcRequestId, wrClient, dbcs)
	if err == nil {
		for _, dbWork := range dbWorks {
			//status := checkValue(dbcs, dbWork.Id)
			//	if status != 0 {
			dbWorkRequest.OperationId = dbWork.Id
			dbWorkRequest.OperationType = dbWork.OperationType
			dbWorkRequest.PercentComplete = fmt.Sprint(*dbWork.PercentComplete) //strconv.FormatFloat(dbWork.PercentComplete, 'E', -1, 32)
			if dbWork.TimeAccepted != nil {
				dbWorkRequest.TimeAccepted = dbWork.TimeAccepted.String()
			}
			if dbWork.TimeFinished != nil {
				dbWorkRequest.TimeFinished = dbWork.TimeFinished.String()
			}
			if dbWork.TimeStarted != nil {
				dbWorkRequest.TimeStarted = dbWork.TimeStarted.String()
			}

			if dbWorkRequest != (databasev4.DbWorkrequests{}) {
				status := checkValue(dbcs, dbWork.Id)
				if status == 0 {
					dbcs.Status.WorkRequests = append(dbcs.Status.WorkRequests, dbWorkRequest)
					dbWorkRequest = databasev4.DbWorkrequests{}
				} else {
					setValue(dbcs, dbWorkRequest)
				}
			}
			//}
		}
	}

	// DB Home Status
	dbcs.Status.DbInfo = dbcs.Status.DbInfo[:0]
	dbStatus := databasev4.DbStatus{}

	dbHomes, err := getDbHomeList(compartmentId, dbClient, dbcs)

	if err == nil {
		for _, dbHome := range dbHomes {
			dbDetails, err := getDList(compartmentId, dbClient, dbcs, dbHome.Id)
			for _, dbDetail := range dbDetails {
				if err == nil {
					dbStatus.Id = dbDetail.Id
					dbStatus.DbHomeId = *dbDetail.DbHomeId
					dbStatus.DbName = *dbDetail.DbName
					dbStatus.DbUniqueName = *dbDetail.DbUniqueName
					dbStatus.DbWorkload = *dbDetail.DbWorkload
					if dbDetail.ConnectionStrings != nil &&
						dbDetail.ConnectionStrings.CdbDefault != nil &&
						dbDetail.ConnectionStrings.CdbIpDefault != nil {

						dbStatus.ConnectionString = *dbDetail.ConnectionStrings.CdbDefault
						dbStatus.ConnectionStringLong = *dbDetail.ConnectionStrings.CdbIpDefault
					}
				}
				dbcs.Status.DbInfo = append(dbcs.Status.DbInfo, dbStatus)
				dbStatus = databasev4.DbStatus{}
			}
		}
	}
	return nil
}

func getDbHomeList(compartmentId string, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem) ([]database.DbHomeSummary, error) {

	var items []database.DbHomeSummary
	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.ListDbHomesRequest{
		DbSystemId:    &dbcsId,
		CompartmentId: &compartmentId,
	}

	resp, err := dbClient.ListDbHomes(context.TODO(), dbcsReq)
	if err != nil {
		return items, err
	}

	return resp.Items, nil
}

func getDList(compartmentId string, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, dbHomeId *string) ([]database.DatabaseSummary, error) {

	dbcsId := *dbcs.Spec.Id
	var items []database.DatabaseSummary
	dbcsReq := database.ListDatabasesRequest{
		SystemId:      &dbcsId,
		CompartmentId: &compartmentId,
		DbHomeId:      dbHomeId,
	}

	resp, err := dbClient.ListDatabases(context.TODO(), dbcsReq)
	if err != nil {
		return items, err
	}

	return resp.Items, nil
}

func getSubnetName(subnetId string, nwClient core.VirtualNetworkClient) (*string, *string, error) {

	req := core.GetSubnetRequest{SubnetId: common.String(subnetId)}

	// Send the request using the service client
	resp, err := nwClient.GetSubnet(context.Background(), req)

	if err != nil {
		return nil, nil, err
	}
	// Retrieve value from the response.

	return resp.DisplayName, resp.VcnId, nil
}

func getVcnName(vcnId *string, nwClient core.VirtualNetworkClient) (*string, error) {

	req := core.GetVcnRequest{VcnId: common.String(*vcnId)}

	// Send the request using the service client
	resp, err := nwClient.GetVcn(context.Background(), req)

	if err != nil {
		return nil, err
	}
	// Retrieve value from the response.

	return resp.DisplayName, nil
}

// =========== validate Specs ============
func ValidateSpex(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, eRecord record.EventRecorder) error {

	//var str1 string
	var eventMsg string
	var eventErr string = "Spec Error"
	lastSuccSpec, err := dbcs.GetLastSuccessfulSpec()
	if err != nil {
		return err
	}
	// Check if last Successful update nil or not
	if lastSuccSpec == nil {
		if dbcs == nil {
			return fmt.Errorf("DbcsSystem is nil")
		}
		if dbcs.Spec.DbSystem == nil {
			return fmt.Errorf("DbSystem spec is missing in DbcsSystem %s", dbcs.Name)
		}
		if dbcs.Spec.DbSystem.DbVersion != "" {
			_, err = GetDbLatestVersion(dbClient, dbcs, "")
			if err != nil {
				eventMsg = "DBCS CRD resource  " + GetFmtStr(dbcs.Name) + " DbVersion " + GetFmtStr(dbcs.Spec.DbSystem.DbVersion) + " is not matching available DB releases."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		} else {
			eventMsg = "DBCS CRD resource  " + "DbVersion  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbVersion") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}
		if dbcs.Spec.DbSystem.DbWorkload != "" {
			_, err = getDbWorkLoadType(dbcs)
			if err != nil {
				eventMsg = "DBCS CRD resource  " + GetFmtStr(dbcs.Name) + " DbWorkload " + GetFmtStr(dbcs.Spec.DbSystem.DbWorkload) + " is not matching the DBworkload type OLTP|DSS."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		} else {
			eventMsg = "DBCS CRD resource  " + "DbWorkload  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbWorkload") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}

		if dbcs.Spec.DbSystem.NodeCount != nil {
			switch *dbcs.Spec.DbSystem.NodeCount {
			case 1:
			case 2:
			default:
				eventMsg = "DBCS CRD resource  " + "NodeCount  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.NodeCount") + " can be either 1 or 2."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		}

	} else {
		if lastSuccSpec.DbSystem.DbVersion != dbcs.Spec.DbSystem.DbVersion {
			eventMsg = "DBCS CRD resource  " + "DbVersion  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbVersion") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}

	}

	return nil

}

func CreateDbcsBackup(
	compartmentId string,
	logger logr.Logger,
	dbClient database.DatabaseClient,
	dbcs *databasev4.DbcsSystem,
	kubeClient client.Client,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
) (string, error) {

	ctx := context.TODO()

	// Safety check: ensure DB system OCID is set
	if dbcs.Spec.Id == nil || *dbcs.Spec.Id == "" {
		return "", fmt.Errorf("cannot create backup: DbcsSystem ID is not set")
	}
	// Get DB Home Details
	listDbHomeRsp, err := GetListDbHomeRsp(logger, dbClient, dbcs)
	if err != nil {
		logger.Info("Error Occurred while getting List of DBHomes")
		return "", err
	}
	dbHomeId := listDbHomeRsp.Items[0].Id
	// Retrieve the list of databases in the DB system
	listDbsRequest := database.ListDatabasesRequest{
		CompartmentId: &compartmentId,
		SystemId:      dbcs.Spec.Id,
		DbHomeId:      dbHomeId,
	}

	listDbsResponse, err := dbClient.ListDatabases(ctx, listDbsRequest)
	if err != nil {
		logger.Error(err, "Failed to list databases for DB system", "DbcsSystemID", *dbcs.Spec.Id)
		return "", err
	}

	if len(listDbsResponse.Items) == 0 {
		return "", fmt.Errorf("no databases found in DB system with ID: %s", *dbcs.Spec.Id)
	}

	// Assume the first database is the one to back up (customize as needed)
	databaseId := listDbsResponse.Items[0].Id

	// Generate a unique display name for the backup
	// Determine the backup name
	var backupPrefixName string
	if dbcs.Spec.DbSystem.BackupDisplayName != "" {
		backupPrefixName = dbcs.Spec.DbSystem.BackupDisplayName
	} else {
		backupPrefixName = "backup"
	}
	backupName := fmt.Sprintf("%s-%s", backupPrefixName, time.Now().Format("20060102-150405"))

	// Check if backup with prefix already exists
	listBackupsReq := database.ListBackupsRequest{
		DatabaseId: databaseId,
	}
	listBackupsResp, err := dbClient.ListBackups(ctx, listBackupsReq)
	if err != nil {
		logger.Error(err, "Failed to list backups")
		return "", err
	}

	// Check if a backup with same name already tracked in status
	for _, b := range dbcs.Status.Backups {
		if strings.HasPrefix(b.Name, backupPrefixName) {
			logger.Info("Backup already tracked in status", "BackupName", b.Name)
			return b.BackupID, nil
		}
	}

	// Compare against latest backup stored in status or name prefix
	for _, backup := range listBackupsResp.Items {
		if backup.DisplayName != nil && strings.HasPrefix(*backup.DisplayName, backupPrefixName) {
			logger.Info("Backup already exists, skipping new backup creation", "ExistingBackup", *backup.DisplayName)
			return *backup.Id, nil
		}
	}

	// Build the CreateBackupRequest
	createBackupReq := database.CreateBackupRequest{
		CreateBackupDetails: database.CreateBackupDetails{
			DatabaseId:  databaseId,
			DisplayName: common.String(backupName),
		},
	}

	logger.Info("Creating manual backup for database", "DatabaseId", *databaseId, "BackupName", backupName)

	// Change the phase to "Updating"
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}
	// Send the request
	createBackupResp, err := dbClient.CreateBackup(ctx, createBackupReq)
	if err != nil {
		logger.Error(err, "Failed to create backup", "DatabaseId", *databaseId)
		// Change the phase to "Failed"
		if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient); statusErr != nil {
			return "", statusErr
		}
		return "", err
	}

	logger.Info("Backup creation initiated", "BackupID", *createBackupResp.Backup.Id)
	backupID := createBackupResp.Backup.Id
	// ---- Wait for backup to complete ----
	logger.Info("Waiting for backup to reach ACTIVE state...")

	waitDuration := 240 * time.Minute
	pollInterval := 30 * time.Second
	timeout := time.After(waitDuration)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return *backupID, fmt.Errorf("timeout: backup %s did not complete within %v", *backupID, waitDuration)
		case <-ticker.C:
			// Get current backup status
			getBackupReq := database.GetBackupRequest{
				BackupId: backupID,
			}
			getBackupResp, err := dbClient.GetBackup(ctx, getBackupReq)
			if err != nil {
				logger.Error(err, "Failed to fetch backup status", "BackupID", *backupID)
				continue
			}

			lifecycle := getBackupResp.Backup.LifecycleState
			logger.Info("Polling backup status", "BackupID", *backupID, "State", lifecycle)

			if lifecycle == database.BackupLifecycleStateActive {
				logger.Info("Backup completed successfully", "BackupID", *backupID)
				// After successful creation and backup becomes ACTIVE
				listBackupsReq := database.ListBackupsRequest{
					DatabaseId: databaseId,
				}

				listBackupsResp, err := dbClient.ListBackups(ctx, listBackupsReq)
				if err != nil {
					logger.Error(err, "Failed to list backups")
					return *backupID, err
				}

				// Reset and populate status.Backups with up-to-date backup list
				dbcs.Status.Backups = []databasev4.BackupInfo{}
				for _, b := range listBackupsResp.Items {
					if b.Id != nil && b.DisplayName != nil && b.TimeStarted != nil {
						dbcs.Status.Backups = append(dbcs.Status.Backups, databasev4.BackupInfo{
							Name:      *b.DisplayName,
							BackupID:  *b.Id,
							Timestamp: b.TimeStarted.Format(time.RFC3339),
						})
					}
				}
				// Change the phase to "Available"
				if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Available, nwClient, wrClient); statusErr != nil {
					return "", statusErr
				}
				return *backupID, nil
			}

			if lifecycle == database.BackupLifecycleStateFailed {
				return *backupID, fmt.Errorf("backup failed: BackupID=%s", *backupID)
			}
		}
	}

}

func RestoreDbcsToPoint(
	compartmentId string,
	logger logr.Logger,
	dbClient database.DatabaseClient,
	dbcs *databasev4.DbcsSystem,
	restoreOpt databasev4.RestoreConfig,
	kubeClient client.Client,
	nwClient core.VirtualNetworkClient,
	wrClient workrequests.WorkRequestClient,
) error {
	ctx := context.TODO()

	if dbcs.Spec.Id == nil || *dbcs.Spec.Id == "" {
		return fmt.Errorf("cannot restore: DbcsSystem ID is not set")
	}

	// Get DB Home
	dbHomeResp, err := GetListDbHomeRsp(logger, dbClient, dbcs)
	if err != nil || len(dbHomeResp.Items) == 0 {
		return fmt.Errorf("no DB Homes found for DB system: %v", err)
	}
	dbHomeId := dbHomeResp.Items[0].Id

	// Get DB
	listDbsResp, err := dbClient.ListDatabases(ctx, database.ListDatabasesRequest{
		CompartmentId: &compartmentId,
		SystemId:      dbcs.Spec.Id,
		DbHomeId:      dbHomeId,
	})
	if err != nil || len(listDbsResp.Items) == 0 {
		return fmt.Errorf("no databases found to restore")
	}
	dbID := listDbsResp.Items[0].Id

	// Change the phase to "Updating"
	logger.Info("Changing State to Updating for ", "DatabaseID", *dbID, "RestoreOption", restoreOpt)
	if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
		return statusErr
	}

	restoreDetails := database.RestoreDatabaseDetails{}

	if restoreOpt.Latest {
		restoreDetails.Latest = common.Bool(true)
	}
	if restoreOpt.Timestamp != nil {
		restoreDetails.Timestamp = &common.SDKTime{Time: restoreOpt.Timestamp.Time}
	}
	if restoreOpt.SCN != nil {
		restoreDetails.DatabaseSCN = restoreOpt.SCN
	}

	restoreReq := database.RestoreDatabaseRequest{
		DatabaseId:             dbID,
		RestoreDatabaseDetails: restoreDetails,
	}

	logger.Info("Initiating restore operation", "DatabaseID", *dbID, "RestoreOption", restoreOpt)

	restoreResp, err := dbClient.RestoreDatabase(ctx, restoreReq)
	if err != nil {
		logger.Error(err, "Failed to restore database")
		SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient)
		return err
	}

	// Poll for completion
	workRequestId := restoreResp.OpcWorkRequestId
	logger.Info("Restore initiated", "WorkRequestID", *workRequestId)

	timeout := time.After(240 * time.Minute)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("restore timed out after 60 minutes")
		case <-ticker.C:
			dbStateResp, err := dbClient.GetDatabase(ctx, database.GetDatabaseRequest{DatabaseId: dbID})
			if err != nil {
				logger.Error(err, "Polling error")
				continue
			}
			state := dbStateResp.Database.LifecycleState
			logger.Info("Polling Restore Operation", "DatabaseID", *dbID, "State", state)

			if state == database.DatabaseLifecycleStateAvailable {
				logger.Info("Database restore completed", "DatabaseID", *dbID)
				// Change the phase to "Available"
				if statusErr := SetLifecycleState(compartmentId, kubeClient, dbClient, dbcs, databasev4.Available, nwClient, wrClient); statusErr != nil {
					return statusErr
				}
				return nil
			} else if state == database.DatabaseLifecycleStateRestoreFailed {
				return fmt.Errorf("restore failed: DatabaseID=%s", *dbID)
			} else if state == database.DatabaseLifecycleStateFailed {
				return fmt.Errorf("restore failed: DatabaseID=%s", *dbID)
			}
		}
	}
}
