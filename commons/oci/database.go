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

package oci

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	"github.com/oracle/oracle-database-operator/commons/k8s"
)

type DatabaseService struct {
	logger       logr.Logger
	kubeClient   client.Client
	dbClient     database.DatabaseClient
	vaultService VaultService
}

func NewDatabaseService(
	logger logr.Logger,
	kubeClient client.Client,
	provider common.ConfigurationProvider) (databaseService DatabaseService, err error) {

	dbClient, err := database.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		return databaseService, err
	}

	vaultService, err := NewVaultService(logger, provider)
	if err != nil {
		return databaseService, err
	}

	return DatabaseService{
		logger:       logger.WithName("dbService"),
		kubeClient:   kubeClient,
		dbClient:     dbClient,
		vaultService: vaultService,
	}, nil
}

/********************************
 * Autonomous Database
 *******************************/

// ReadPassword reads the password from passwordSpec, and returns the pointer to the read password string.
// The function returns a nil if nothing is read
func (d *DatabaseService) readPassword(namespace string, passwordSpec dbv4.PasswordSpec) (*string, error) {
	logger := d.logger.WithName("readPassword")

	if passwordSpec.K8sSecret.Name != nil {
		logger.Info(fmt.Sprintf("Getting password from Secret %s", *passwordSpec.K8sSecret.Name))

		key := *passwordSpec.K8sSecret.Name
		password, err := k8s.GetSecretValue(d.kubeClient, namespace, *passwordSpec.K8sSecret.Name, key)
		if err != nil {
			return nil, err
		}

		return common.String(password), nil
	}

	if passwordSpec.OciSecret.Id != nil {
		logger.Info(fmt.Sprintf("Getting password from OCI Vault Secret OCID %s", *passwordSpec.OciSecret.Id))

		password, err := d.vaultService.GetSecretValue(*passwordSpec.OciSecret.Id)
		if err != nil {
			return nil, err
		}
		return common.String(password), nil
	}

	return nil, nil
}

func (d *DatabaseService) readACD_OCID(acd *dbv4.AcdSpec, namespace string) (*string, error) {
	if acd.OciAcd.Id != nil {
		return acd.OciAcd.Id, nil
	}

	if acd.K8sAcd.Name != nil {
		fetchedACD := &dbv4.AutonomousContainerDatabase{}
		if err := k8s.FetchResource(d.kubeClient, namespace, *acd.K8sAcd.Name, fetchedACD); err != nil {
			return nil, err
		}

		return fetchedACD.Spec.AutonomousContainerDatabaseOCID, nil
	}

	return nil, nil
}

// CreateAutonomousDatabase sends a request to OCI to provision a database and returns the AutonomousDatabase OCID.
func (d *DatabaseService) CreateAutonomousDatabase(adb *dbv4.AutonomousDatabase) (resp database.CreateAutonomousDatabaseResponse, err error) {
	adminPassword, err := d.readPassword(adb.Namespace, adb.Spec.Details.AdminPassword)
	if err != nil {
		return resp, err
	}

	acdOCID, err := d.readACD_OCID(&adb.Spec.Details.AutonomousContainerDatabase, adb.Namespace)
	if err != nil {
		return resp, err
	}

	createAutonomousDatabaseDetails := database.CreateAutonomousDatabaseDetails{
		CompartmentId:                 adb.Spec.Details.CompartmentId,
		DbName:                        adb.Spec.Details.DbName,
		CpuCoreCount:                  adb.Spec.Details.CpuCoreCount,
		ComputeModel:                  database.CreateAutonomousDatabaseBaseComputeModelEnum(adb.Spec.Details.ComputeModel),
		ComputeCount:                  adb.Spec.Details.ComputeCount,
		OcpuCount:                     adb.Spec.Details.OcpuCount,
		DataStorageSizeInTBs:          adb.Spec.Details.DataStorageSizeInTBs,
		AdminPassword:                 adminPassword,
		DisplayName:                   adb.Spec.Details.DisplayName,
		IsAutoScalingEnabled:          adb.Spec.Details.IsAutoScalingEnabled,
		IsDedicated:                   adb.Spec.Details.IsDedicated,
		AutonomousContainerDatabaseId: acdOCID,
		DbVersion:                     adb.Spec.Details.DbVersion,
		DbWorkload:                    database.CreateAutonomousDatabaseBaseDbWorkloadEnum(adb.Spec.Details.DbWorkload),
		LicenseModel:                  database.CreateAutonomousDatabaseBaseLicenseModelEnum(adb.Spec.Details.LicenseModel),
		IsFreeTier:                    adb.Spec.Details.IsFreeTier,
		IsAccessControlEnabled:        adb.Spec.Details.IsAccessControlEnabled,
		WhitelistedIps:                adb.Spec.Details.WhitelistedIps,
		IsMtlsConnectionRequired:      adb.Spec.Details.IsMtlsConnectionRequired,
		SubnetId:                      adb.Spec.Details.SubnetId,
		NsgIds:                        adb.Spec.Details.NsgIds,
		PrivateEndpointLabel:          adb.Spec.Details.PrivateEndpointLabel,

		FreeformTags: adb.Spec.Details.FreeformTags,
	}

	retryPolicy := common.DefaultRetryPolicy()

	createAutonomousDatabaseRequest := database.CreateAutonomousDatabaseRequest{
		CreateAutonomousDatabaseDetails: createAutonomousDatabaseDetails,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	resp, err = d.dbClient.CreateAutonomousDatabase(context.TODO(), createAutonomousDatabaseRequest)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (d *DatabaseService) GetAutonomousDatabase(adbOCID string) (database.GetAutonomousDatabaseResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	getAutonomousDatabaseRequest := database.GetAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.GetAutonomousDatabase(context.TODO(), getAutonomousDatabaseRequest)
}

func (d *DatabaseService) UpdateAutonomousDatabase(adbOCID string, adb *dbv4.AutonomousDatabase) (resp database.UpdateAutonomousDatabaseResponse, err error) {
	// Retrieve admin password
	adminPassword, err := d.readPassword(adb.Namespace, adb.Spec.Details.AdminPassword)
	if err != nil {
		return resp, err
	}

	retryPolicy := common.DefaultRetryPolicy()

	updateAutonomousDatabaseRequest := database.UpdateAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		UpdateAutonomousDatabaseDetails: database.UpdateAutonomousDatabaseDetails{
			DisplayName:              adb.Spec.Details.DisplayName,
			DbName:                   adb.Spec.Details.DbName,
			DbVersion:                adb.Spec.Details.DbVersion,
			FreeformTags:             adb.Spec.Details.FreeformTags,
			DbWorkload:               database.UpdateAutonomousDatabaseDetailsDbWorkloadEnum(adb.Spec.Details.DbWorkload),
			LicenseModel:             database.UpdateAutonomousDatabaseDetailsLicenseModelEnum(adb.Spec.Details.LicenseModel),
			AdminPassword:            adminPassword,
			DataStorageSizeInTBs:     adb.Spec.Details.DataStorageSizeInTBs,
			CpuCoreCount:             adb.Spec.Details.CpuCoreCount,
			ComputeModel:             database.UpdateAutonomousDatabaseDetailsComputeModelEnum(adb.Spec.Details.ComputeModel),
			ComputeCount:             adb.Spec.Details.ComputeCount,
			OcpuCount:                adb.Spec.Details.OcpuCount,
			IsAutoScalingEnabled:     adb.Spec.Details.IsAutoScalingEnabled,
			IsFreeTier:               adb.Spec.Details.IsFreeTier,
			IsMtlsConnectionRequired: adb.Spec.Details.IsMtlsConnectionRequired,
			IsAccessControlEnabled:   adb.Spec.Details.IsAccessControlEnabled,
			WhitelistedIps:           adb.Spec.Details.WhitelistedIps,
			SubnetId:                 adb.Spec.Details.SubnetId,
			NsgIds:                   adb.Spec.Details.NsgIds,
			PrivateEndpointLabel:     adb.Spec.Details.PrivateEndpointLabel,
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}
	return d.dbClient.UpdateAutonomousDatabase(context.TODO(), updateAutonomousDatabaseRequest)
}

func (d *DatabaseService) StartAutonomousDatabase(adbOCID string) (database.StartAutonomousDatabaseResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	startRequest := database.StartAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.StartAutonomousDatabase(context.TODO(), startRequest)
}

func (d *DatabaseService) StopAutonomousDatabase(adbOCID string) (database.StopAutonomousDatabaseResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	stopRequest := database.StopAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.StopAutonomousDatabase(context.TODO(), stopRequest)
}

func (d *DatabaseService) DeleteAutonomousDatabase(adbOCID string) (database.DeleteAutonomousDatabaseResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	deleteRequest := database.DeleteAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.DeleteAutonomousDatabase(context.TODO(), deleteRequest)
}

func (d *DatabaseService) DownloadWallet(adb *dbv4.AutonomousDatabase) (resp database.GenerateAutonomousDatabaseWalletResponse, err error) {
	// Prepare wallet password
	walletPassword, err := d.readPassword(adb.Namespace, adb.Spec.Wallet.Password)
	if err != nil {
		return resp, err
	}

	retryPolicy := common.DefaultRetryPolicy()

	// Download a Wallet
	req := database.GenerateAutonomousDatabaseWalletRequest{
		AutonomousDatabaseId: adb.Spec.Details.Id,
		GenerateAutonomousDatabaseWalletDetails: database.GenerateAutonomousDatabaseWalletDetails{
			Password: walletPassword,
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	// Send the request using the service client
	resp, err = d.dbClient.GenerateAutonomousDatabaseWallet(context.TODO(), req)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

/********************************
 * Autonomous Database Restore
 *******************************/

func (d *DatabaseService) RestoreAutonomousDatabase(adbOCID string, sdkTime common.SDKTime) (database.RestoreAutonomousDatabaseResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	request := database.RestoreAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RestoreAutonomousDatabaseDetails: database.RestoreAutonomousDatabaseDetails{
			Timestamp: &sdkTime,
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}
	return d.dbClient.RestoreAutonomousDatabase(context.TODO(), request)
}

/********************************
 * Autonomous Database Backup
 *******************************/

func (d *DatabaseService) ListAutonomousDatabaseBackups(adbOCID string) (database.ListAutonomousDatabaseBackupsResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	listBackupRequest := database.ListAutonomousDatabaseBackupsRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.ListAutonomousDatabaseBackups(context.TODO(), listBackupRequest)
}

func (d *DatabaseService) CreateAutonomousDatabaseBackup(adbBackup *dbv4.AutonomousDatabaseBackup, adbOCID string) (database.CreateAutonomousDatabaseBackupResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	createBackupRequest := database.CreateAutonomousDatabaseBackupRequest{
		CreateAutonomousDatabaseBackupDetails: database.CreateAutonomousDatabaseBackupDetails{
			AutonomousDatabaseId:  common.String(adbOCID),
			IsLongTermBackup:      adbBackup.Spec.IsLongTermBackup,
			RetentionPeriodInDays: adbBackup.Spec.RetentionPeriodInDays,
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	// Use the spec.displayName as the displayName of the backup if is provided,
	// otherwise use the resource name as the displayName.
	if adbBackup.Spec.DisplayName != nil {
		createBackupRequest.DisplayName = adbBackup.Spec.DisplayName
	} else {
		createBackupRequest.DisplayName = common.String(adbBackup.GetName())
	}

	return d.dbClient.CreateAutonomousDatabaseBackup(context.TODO(), createBackupRequest)
}

func (d *DatabaseService) GetAutonomousDatabaseBackup(backupOCID string) (database.GetAutonomousDatabaseBackupResponse, error) {
	retryPolicy := common.DefaultRetryPolicy()

	getBackupRequest := database.GetAutonomousDatabaseBackupRequest{
		AutonomousDatabaseBackupId: common.String(backupOCID),
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.GetAutonomousDatabaseBackup(context.TODO(), getBackupRequest)
}

func (d *DatabaseService) CreateAutonomousDatabaseClone(adb *dbv4.AutonomousDatabase) (resp database.CreateAutonomousDatabaseResponse, err error) {
	adminPassword, err := d.readPassword(adb.Namespace, adb.Spec.Clone.AdminPassword)
	if err != nil {
		return resp, err
	}

	acdOCID, err := d.readACD_OCID(&adb.Spec.Clone.AutonomousContainerDatabase, adb.Namespace)
	if err != nil {
		return resp, err
	}

	retryPolicy := common.DefaultRetryPolicy()
	request := database.CreateAutonomousDatabaseRequest{
		CreateAutonomousDatabaseDetails: database.CreateAutonomousDatabaseCloneDetails{
			CompartmentId:                 adb.Spec.Clone.CompartmentId,
			SourceId:                      adb.Spec.Details.Id,
			AutonomousContainerDatabaseId: acdOCID,
			DisplayName:                   adb.Spec.Clone.DisplayName,
			DbName:                        adb.Spec.Clone.DbName,
			DbWorkload:                    database.CreateAutonomousDatabaseBaseDbWorkloadEnum(adb.Spec.Clone.DbWorkload),
			LicenseModel:                  database.CreateAutonomousDatabaseBaseLicenseModelEnum(adb.Spec.Clone.LicenseModel),
			DbVersion:                     adb.Spec.Clone.DbVersion,
			DataStorageSizeInTBs:          adb.Spec.Clone.DataStorageSizeInTBs,
			CpuCoreCount:                  adb.Spec.Clone.CpuCoreCount,
			ComputeModel:                  database.CreateAutonomousDatabaseBaseComputeModelEnum(adb.Spec.Clone.ComputeModel),
			ComputeCount:                  adb.Spec.Clone.ComputeCount,
			OcpuCount:                     adb.Spec.Clone.OcpuCount,
			AdminPassword:                 adminPassword,
			IsAutoScalingEnabled:          adb.Spec.Clone.IsAutoScalingEnabled,
			IsDedicated:                   adb.Spec.Clone.IsDedicated,
			IsFreeTier:                    adb.Spec.Clone.IsFreeTier,
			IsAccessControlEnabled:        adb.Spec.Clone.IsAccessControlEnabled,
			WhitelistedIps:                adb.Spec.Clone.WhitelistedIps,
			SubnetId:                      adb.Spec.Clone.SubnetId,
			NsgIds:                        adb.Spec.Clone.NsgIds,
			PrivateEndpointLabel:          adb.Spec.Clone.PrivateEndpointLabel,
			IsMtlsConnectionRequired:      adb.Spec.Clone.IsMtlsConnectionRequired,
			FreeformTags:                  adb.Spec.Clone.FreeformTags,
			CloneType:                     adb.Spec.Clone.CloneType,
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	return d.dbClient.CreateAutonomousDatabase(context.TODO(), request)
}
