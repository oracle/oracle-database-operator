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
	checkInterval = 30 * time.Second
	timeout       = 15 * time.Minute
)

func CreateAndGetDbcsId(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient, kmsDetails *databasev4.KMSDetailsStatus) (string, error) {

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
	dbHomeReq, err := GetDbHomeDetails(kubeClient, dbClient, dbcs)
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
	if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
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

func CloneAndGetDbcsId(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {
	ctx := context.TODO()
	var err error
	dbAdminPassword := ""
	// tdePassword := ""
	logger.Info("Starting the clone process for DBCS", "dbcs", dbcs)
	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPaswordSecret != "" {
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
	if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
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

// CloneFromBackupAndGetDbcsId clones a DB system from a backup and returns the new DB system's OCID.
func CloneFromBackupAndGetDbcsId(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {
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

	// Fetch the existing DB system details
	existingDbSystem, err := dbClient.GetDbSystem(ctx, database.GetDbSystemRequest{
		DbSystemId: dbSystemId,
	})
	if err != nil {
		return "", err
	}
	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPaswordSecret != "" {
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
		return "", err
	}

	dbcs.Status.DbCloneStatus.Id = response.DbSystem.Id

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *response.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *response.DbSystem.Id, nil
}

// Sync the DbcsSystem Database details
func CloneFromDatabaseAndGetDbcsId(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {
	ctx := context.TODO()
	var err error
	dbAdminPassword := ""
	tdePassword := ""
	logger.Info("Starting the clone process for Database", "dbcs", dbcs)

	// Get the admin password from Kubernetes secret
	if dbcs.Spec.DbClone.DbAdminPaswordSecret != "" {
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
		return "", err
	}

	dbcs.Status.DbCloneStatus.Id = response.DbSystem.Id

	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}

	// Check the state
	_, err = CheckResourceState(logger, dbClient, *response.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
	if err != nil {
		return "", err
	}

	return *response.DbSystem.Id, nil
}

// Get admin password from Secret then OCI valut secret
func GetCloningAdminPassword(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (string, error) {
	if dbcs.Spec.DbClone.DbAdminPaswordSecret != "" {
		// Get the Admin Secret
		adminSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbClone.DbAdminPaswordSecret,
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
	if dbcs.Spec.DbSystem.DbAdminPaswordSecret != "" {
		// Get the Admin Secret
		adminSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.DbAdminPaswordSecret,
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
func SetLifecycleState(kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, state databasev4.LifecycleState, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {
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
		if statusErr := SetDBCSStatus(dbClient, dbcs, nwClient, wrClient); statusErr != nil {
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

func SetDBCSDatabaseLifecycleState(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

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
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Available, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
	} else if string(resp.LifecycleState) == string(databasev4.Provision) {
		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Provision, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Provision), string(databasev4.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev4.Update) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev4.Update), string(databasev4.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev4.Failed) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Failed, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("DbSystem is in Failed State")
	} else if string(resp.LifecycleState) == string(databasev4.Terminated) {
		// Change the phase to "Terminated"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Terminate, nwClient, wrClient); statusErr != nil {
			return statusErr
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

	dbcs.Spec.DbSystem.CompartmentId = *response.CompartmentId
	if response.DisplayName != nil {
		dbcs.Spec.DbSystem.DisplayName = *response.DisplayName
	}

	if response.Hostname != nil {
		dbcs.Spec.DbSystem.HostName = *response.Hostname
	}
	if response.CpuCoreCount != nil {
		dbcs.Spec.DbSystem.CpuCoreCount = *response.CpuCoreCount
	}
	dbcs.Spec.DbSystem.NodeCount = response.NodeCount
	if response.ClusterName != nil {
		dbcs.Spec.DbSystem.ClusterName = *response.ClusterName
	}
	//dbcs.Spec.DbSystem.DbUniqueName = *response.DbUniqueName
	if string(response.DbSystem.DatabaseEdition) != "" {
		dbcs.Spec.DbSystem.DbEdition = string(response.DatabaseEdition)
	}
	if string(response.DiskRedundancy) != "" {
		dbcs.Spec.DbSystem.DiskRedundancy = string(response.DiskRedundancy)
	}

	//dbcs.Spec.DbSystem.DbVersion = *response.

	if response.BackupSubnetId != nil {
		dbcs.Spec.DbSystem.BackupSubnetId = *response.BackupSubnetId
	}
	dbcs.Spec.DbSystem.Shape = *response.Shape
	dbcs.Spec.DbSystem.SshPublicKeys = []string(response.SshPublicKeys)
	if response.FaultDomains != nil {
		dbcs.Spec.DbSystem.FaultDomains = []string(response.FaultDomains)
	}
	dbcs.Spec.DbSystem.SubnetId = *response.SubnetId
	dbcs.Spec.DbSystem.AvailabilityDomain = *response.AvailabilityDomain
	if response.KmsKeyId != nil {
		dbcs.Status.KMSDetailsStatus.KeyId = *response.KmsKeyId
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
	CompartmentId := dbcs.Spec.DbSystem.CompartmentId

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

func UpdateDbcsSystemIdInst(log logr.Logger, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, kubeClient client.Client, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient, databaseID string) error {
	log.Info("Existing DB System Getting Updated with new details in UpdateDbcsSystemIdInst")
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
	log.Info("Details of updateFlag -> " + fmt.Sprint(updateFlag))

	if dbcs.Spec.DbSystem.CpuCoreCount > 0 && dbcs.Spec.DbSystem.CpuCoreCount != oldSpec.DbSystem.CpuCoreCount {
		log.Info("DB System cpu core count is: " + fmt.Sprint(dbcs.Spec.DbSystem.CpuCoreCount) + " DB System old cpu count is: " + fmt.Sprint(oldSpec.DbSystem.CpuCoreCount))
		updateDbcsDetails.CpuCoreCount = common.Int(dbcs.Spec.DbSystem.CpuCoreCount)
		updateFlag = true
	}
	if dbcs.Spec.DbSystem.Shape != "" && dbcs.Spec.DbSystem.Shape != oldSpec.DbSystem.Shape {
		// log.Info("DB System desired shape is :" + string(dbcs.Spec.DbSystem.Shape) + "DB System old shape is " + string(oldSpec.DbSystem.Shape))
		updateDbcsDetails.Shape = common.String(dbcs.Spec.DbSystem.Shape)
		updateFlag = true
	}

	if dbcs.Spec.DbSystem.LicenseModel != "" && dbcs.Spec.DbSystem.LicenseModel != oldSpec.DbSystem.LicenseModel {
		licenceModel := getLicenceModel(dbcs)
		// log.Info("DB System desired License Model is :" + string(dbcs.Spec.DbSystem.LicenseModel) + "DB Sytsem old License Model is " + string(oldSpec.DbSystem.LicenseModel))
		updateDbcsDetails.LicenseModel = database.UpdateDbSystemDetailsLicenseModelEnum(licenceModel)
		updateFlag = true
	}

	if dbcs.Spec.DbSystem.InitialDataStorageSizeInGB != 0 && dbcs.Spec.DbSystem.InitialDataStorageSizeInGB != oldSpec.DbSystem.InitialDataStorageSizeInGB {
		// log.Info("DB System desired Storage Size is :" + fmt.Sprint(dbcs.Spec.DbSystem.InitialDataStorageSizeInGB) + "DB System old Storage Size is " + fmt.Sprint(oldSpec.DbSystem.InitialDataStorageSizeInGB))
		updateDbcsDetails.DataStorageSizeInGBs = &dbcs.Spec.DbSystem.InitialDataStorageSizeInGB
		updateFlag = true
	}

	// // Check and update KMS details if necessary
	if (dbcs.Spec.KMSConfig != databasev4.KMSConfig{}) {
		if dbcs.Spec.KMSConfig != oldSpec.DbSystem.KMSConfig {
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
			if dbcs.Spec.DbSystem.DbAdminPaswordSecret != "" {
				dbAdminPassword, err = GetAdminPassword(kubeClient, dbcs)
				if err != nil {
					log.Error(err, "Failed to get DB Admin password")
				}
			}

			// Assign all available fields to KMSConfig
			dbcs.Spec.DbSystem.KMSConfig = databasev4.KMSConfig{
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
			// // Wait for the database to reach the desired state after migration
			// err = WaitForDatabaseState(log, dbClient, databaseID, "AVAILABLE")
			// if err != nil {
			// 	log.Error(err, "Database did not reach the desired state after migration")
			// 	return err
			// }

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

			// // Wait for the database to reach the desired state after migration
			// err = WaitForDatabaseState(log, dbClient, databaseID, "AVAILABLE")
			// if err != nil {
			// 	log.Error(err, "Database did not reach the desired state after migration")
			// 	return err
			// }

			log.Info("KMS migration process completed successfully")
			updateFlag = true
		}
	}

	log.Info("Details of updateFlag after validations is " + fmt.Sprint(updateFlag))
	if updateFlag {
		updateDbcsRequest := database.UpdateDbSystemRequest{
			DbSystemId:            common.String(*dbcs.Spec.Id),
			UpdateDbSystemDetails: updateDbcsDetails,
		}

		if _, err := dbClient.UpdateDbSystem(context.TODO(), updateDbcsRequest); err != nil {
			return err
		}

		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev4.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// // Check the State
		// _, err = CheckResourceState(log, dbClient, *dbcs.Spec.Id, "UPDATING", "AVAILABLE")
		// if err != nil {
		// 	return err
		// }
	}

	return nil
}

func WaitForDatabaseState(log logr.Logger, dbClient database.DatabaseClient, dbHomeID string, desiredState database.DbHomeLifecycleStateEnum) error {
	deadline := time.Now().Add(timeout)
	client, err := database.NewDatabaseClientWithConfigurationProvider(common.DefaultConfigProvider())
	if err != nil {
		log.Error(err, "Failed to get DBHome")
		return err
	}
	for time.Now().Before(deadline) {
		dbHomeReq := database.GetDbHomeRequest{
			DbHomeId: common.String(dbHomeID),
		}

		log.Info("Sending GetDbHome request", "dbHomeID", dbHomeID)

		dbHomeResp, err := client.GetDbHome(context.TODO(), dbHomeReq)
		if err != nil {
			log.Error(err, "Failed to get DBHome")
			return err
		}

		if dbHomeResp.DbHome.LifecycleState == desiredState {
			log.Info("DBHome reached desired state", "DBHomeID", dbHomeID, "State", desiredState)
			return nil
		}

		log.Info("Waiting for DBHome to reach desired state", "DBHomeID", dbHomeID, "CurrentState", dbHomeResp.DbHome.LifecycleState, "DesiredState", desiredState)
		time.Sleep(checkInterval)
	}
	return fmt.Errorf("timed out waiting for DBHome to reach desired state: %s", desiredState)

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

func CheckResourceState(logger logr.Logger, dbClient database.DatabaseClient, Id string, currentState string, expectedState string) (string, error) {
	// The database OCID is not available when the provisioning is onging.
	// Retry until the new DbcsSystem is ready.

	var state string
	var err error
	for {
		state, err = GetResourceState(logger, dbClient, Id)
		if err != nil {
			logger.Info("Error occurred while collecting the resource life cycle state")
			return "", err
		}
		if string(state) == expectedState {
			break
		} else if string(state) == currentState {
			logger.Info("DB System current state is still:" + string(state) + ". Sleeping for 60 seconds.")
			time.Sleep(60 * time.Second)
			continue
		} else {
			msg := "DB System current state " + string(state) + " is not matching " + expectedState
			logger.Info(msg)
			return "", errors.New(msg)
		}
	}

	return "", nil
}

func GetResourceState(logger logr.Logger, dbClient database.DatabaseClient, Id string) (string, error) {

	dbcsId := Id
	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	response, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return "", err
	}

	state := string(response.LifecycleState)

	return state, nil
}

func SetDBCSStatus(dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

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
	if dbcs.Spec.KMSConfig.CompartmentId != "" {
		dbcs.Status.KMSDetailsStatus.CompartmentId = dbcs.Spec.KMSConfig.CompartmentId
		dbcs.Status.KMSDetailsStatus.VaultName = dbcs.Spec.KMSConfig.VaultName
	}
	dbcs.Status.State = databasev4.LifecycleState(resp.LifecycleState)
	if dbcs.Spec.KMSConfig.CompartmentId != "" {
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

	dbWorks, err := getWorkRequest(*resp.OpcRequestId, wrClient, dbcs)
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

	dbHomes, err := getDbHomeList(dbClient, dbcs)

	if err == nil {
		for _, dbHome := range dbHomes {
			dbDetails, err := getDList(dbClient, dbcs, dbHome.Id)
			for _, dbDetail := range dbDetails {
				if err == nil {
					dbStatus.Id = dbDetail.Id
					dbStatus.DbHomeId = *dbDetail.DbHomeId
					dbStatus.DbName = *dbDetail.DbName
					dbStatus.DbUniqueName = *dbDetail.DbUniqueName
					dbStatus.DbWorkload = *dbDetail.DbWorkload
				}
				dbcs.Status.DbInfo = append(dbcs.Status.DbInfo, dbStatus)
				dbStatus = databasev4.DbStatus{}
			}
		}
	}
	return nil
}

func getDbHomeList(dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem) ([]database.DbHomeSummary, error) {

	var items []database.DbHomeSummary
	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.ListDbHomesRequest{
		DbSystemId:    &dbcsId,
		CompartmentId: &dbcs.Spec.DbSystem.CompartmentId,
	}

	resp, err := dbClient.ListDbHomes(context.TODO(), dbcsReq)
	if err != nil {
		return items, err
	}

	return resp.Items, nil
}

func getDList(dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, dbHomeId *string) ([]database.DatabaseSummary, error) {

	dbcsId := *dbcs.Spec.Id
	var items []database.DatabaseSummary
	dbcsReq := database.ListDatabasesRequest{
		SystemId:      &dbcsId,
		CompartmentId: &dbcs.Spec.DbSystem.CompartmentId,
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
