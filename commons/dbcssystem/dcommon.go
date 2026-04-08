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
	"fmt"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/workrequests"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
)

// GetDbHomeDetails builds database home details for DB system create/clone requests.
func GetDbHomeDetails(kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, _ string) (database.CreateDbHomeDetails, error) {

	dbHomeDetails := database.CreateDbHomeDetails{}
	dbHomeReq, err := GetDbLatestVersion(dbClient, dbcs, "")
	if err != nil {
		return database.CreateDbHomeDetails{}, err
	}
	if dbcs.Spec.Id != nil {
		dbHomeReq, err = GetDbLatestVersion(dbClient, dbcs, *dbcs.Spec.Id)
		if err != nil {
			return database.CreateDbHomeDetails{}, err
		}
	}

	dbHomeDetails.DbVersion = &dbHomeReq

	dbDetailsReq, err := GetDBDetails(kubeClient, dbcs)
	if err != nil {
		return database.CreateDbHomeDetails{}, err
	}

	dbHomeDetails.Database = &dbDetailsReq

	return dbHomeDetails, nil
}

// GetDbLatestVersion retrieves the latest database version available for the specified DBCS system. It sends a request to the Oracle Cloud Infrastructure (OCI) Database service to list the database versions based on the provided criteria, such as compartment ID, shape, and optionally the DB system ID. The function then iterates through the returned list of database versions to find a match with the version specified in the DBCS system's spec. If a match is found, it returns that version; otherwise, it returns an error indicating that no matching database version was found.
func GetDbLatestVersion(dbClient database.DatabaseClient, dbcs *databasev4.DbcsSystem, dbSystemID string) (string, error) {

	//var provisionedDbcsSystemId string
	ctx := context.TODO()
	var version database.DbVersionSummary
	sFlag := 0
	var val int

	dbVersionReq := database.ListDbVersionsRequest{}
	if dbSystemID != "" {
		dbVersionReq.DbSystemId = common.String(dbSystemID)
	}

	dbVersionReq.IsDatabaseSoftwareImageSupported = common.Bool(true)
	dbVersionReq.IsUpgradeSupported = common.Bool(false)
	dbVersionReq.CompartmentId = common.String(dbcs.Spec.DbSystem.CompartmentId)
	dbVersionReq.DbSystemShape = common.String(dbcs.Spec.DbSystem.Shape)
	// Send the request using the service client
	req := database.ListDbVersionsRequest(dbVersionReq)

	resp, err := dbClient.ListDbVersions(ctx, req)

	if err != nil {
		return "", err
	}

	if dbcs.Spec.DbSystem.DbVersion != "" {
		for i := len(resp.Items) - 1; i >= 0; i-- {
			version = resp.Items[i]
			s1 := getStr(*version.Version, 2)
			s2 := getStr(dbcs.Spec.DbSystem.DbVersion, 2)
			if strings.EqualFold(s1, s2) {
				val, _ = strconv.Atoi(s1)
				if val >= 18 && val <= 21 {
					s3 := s1 + "c"
					if strings.EqualFold(s3, dbcs.Spec.DbSystem.DbVersion) {
						sFlag = 1
						break
					}
				} else if val >= 23 {
					s3 := s1 + "ai"
					if strings.EqualFold(s3, dbcs.Spec.DbSystem.DbVersion) {
						sFlag = 1
						break
					}
				} else if val < 18 && val >= 11 {
					s4 := getStr(*version.Version, 4)
					if strings.EqualFold(s4, dbcs.Spec.DbSystem.DbVersion) {
						sFlag = 1
						break
					}
				}

			}
		}
	}

	if sFlag == 1 {
		return *version.Version, nil
	}

	return *version.Version, fmt.Errorf("no database version matched")
}

// getStr is a helper function that extracts a substring from the input string 'str1' starting from index 0 up to the specified length 'num'. It is used in the GetDbLatestVersion function to compare the major version of the database with the version specified in the DBCS system's spec. By extracting the relevant portion of the version string, it allows for accurate comparison and matching of database versions.
func getStr(str1 string, num int) string {
	return str1[0:num]
}

// GetDBDetails retrieves the database details required for creating a database home in the DBCS system. It fetches the Transparent Data Encryption (TDE) wallet password and the admin password from Kubernetes secrets, and it also gathers other database configuration details such as the database name, workload type, pluggable database name, and backup configuration. The function constructs a CreateDatabaseDetails struct with the retrieved information and returns it for use in the database creation process.
func GetDBDetails(kubeClient client.Client, dbcs *databasev4.DbcsSystem) (database.CreateDatabaseDetails, error) {
	dbDetails := database.CreateDatabaseDetails{}
	var val database.CreateDatabaseDetailsDbWorkloadEnum

	if dbcs.Spec.DbSystem.TdeWalletPasswordSecret != "" {
		tdePasswd, err := GetTdePassword(kubeClient, dbcs)
		if err != nil {
			return database.CreateDatabaseDetails{}, err
		}
		tdePassword := strings.Trim(strings.TrimSuffix(tdePasswd, "\n"), "\"")
		dbDetails.TdeWalletPassword = &tdePassword
		//fmt.Print(tdePassword)

	}

	adminPasswd, err := GetAdminPassword(kubeClient, dbcs)
	if err != nil {
		return database.CreateDatabaseDetails{}, err
	}

	adminPassword := strings.Trim(strings.TrimSuffix(adminPasswd, "\n"), "\"")
	dbDetails.AdminPassword = &adminPassword
	//fmt.Print(adminPassword)
	if dbcs.Spec.DbSystem.DbName != "" {
		dbDetails.DbName = common.String(dbcs.Spec.DbSystem.DbName)
	}

	if dbcs.Spec.DbSystem.DbWorkload != "" {
		val, err = getDbWorkLoadType(dbcs)
		if err != nil {
			return dbDetails, err
		}
		dbDetails.DbWorkload = database.CreateDatabaseDetailsDbWorkloadEnum(val)
	}
	dbDetails.DbName = common.String(dbcs.Spec.DbSystem.DbName)
	if dbcs.Spec.DbSystem.PdbName != "" {
		dbDetails.PdbName = &dbcs.Spec.DbSystem.PdbName
	}

	if dbcs != nil &&
		dbcs.Spec.DbSystem != nil &&
		dbcs.Spec.DbSystem.DbBackupConfig != nil &&
		dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled != nil &&
		*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled {

		backupConfig, err := getBackupConfig(kubeClient, dbcs)
		if err != nil {
			return dbDetails, err
		}
		dbDetails.DbBackupConfig = &backupConfig
	}

	return dbDetails, nil
}

// getBackupConfig retrieves the backup configuration for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks if auto-backup is enabled and, if so, it retrieves the auto-backup window and recovery window in days from the DBCS spec. The function then constructs a DbBackupConfig struct with the retrieved values and returns it for use in configuring the database backup settings during database creation or update operations.
func getBackupConfig(_ client.Client, dbcs *databasev4.DbcsSystem) (database.DbBackupConfig, error) {
	backupConfig := database.DbBackupConfig{}

	if dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled != nil {
		if *dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled {
			backupConfig.AutoBackupEnabled = dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled
			val1, err := getBackupWindowEnum(dbcs)
			if err != nil {
				return backupConfig, err
			}
			backupConfig.AutoBackupWindow = database.DbBackupConfigAutoBackupWindowEnum(val1)
		}

		if dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays != nil {
			val1, err := getRecoveryWindowsInDays(dbcs)
			if err != nil {
				return backupConfig, err
			}
			backupConfig.RecoveryWindowInDays = common.Int(val1)

		}
	}

	return backupConfig, nil
}

// getBackupWindowEnum converts the auto-backup window specified in the DBCS system's spec into the corresponding enum value defined in the OCI SDK. It checks the value of AutoBackupWindow in the DBCS spec against known valid values (SLOT_ONE, SLOT_TWO, etc.) and returns the corresponding enum value. If the value does not match any of the expected options, it defaults to SLOT_ONE and returns it without an error. This function ensures that the auto-backup window configuration is correctly interpreted and applied when configuring database backup settings.
func getBackupWindowEnum(dbcs *databasev4.DbcsSystem) (database.DbBackupConfigAutoBackupWindowEnum, error) {

	if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_ONE" {
		return database.DbBackupConfigAutoBackupWindowOne, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_TWO" {
		return database.DbBackupConfigAutoBackupWindowTwo, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_THREE" {
		return database.DbBackupConfigAutoBackupWindowThree, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_FOUR" {
		return database.DbBackupConfigAutoBackupWindowFour, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_FOUR" {
		return database.DbBackupConfigAutoBackupWindowFour, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_FIVE" {
		return database.DbBackupConfigAutoBackupWindowFive, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_SIX" {
		return database.DbBackupConfigAutoBackupWindowSix, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_SEVEN" {
		return database.DbBackupConfigAutoBackupWindowSeven, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_EIGHT" {
		return database.DbBackupConfigAutoBackupWindowEight, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_NINE" {
		return database.DbBackupConfigAutoBackupWindowNine, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_TEN" {
		return database.DbBackupConfigAutoBackupWindowTen, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_ELEVEN" {
		return database.DbBackupConfigAutoBackupWindowEleven, nil
	} else if strings.ToUpper(*dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow) == "SLOT_TWELVE" {
		return database.DbBackupConfigAutoBackupWindowTwelve, nil
	}
	return database.DbBackupConfigAutoBackupWindowOne, nil

	//return database.DbBackupConfigAutoBackupWindowEight, fmt.Errorf("AutoBackupWindow values can be SLOT_ONE|SLOT_TWO|SLOT_THREE|SLOT_FOUR|SLOT_FIVE|SLOT_SIX|SLOT_SEVEN|SLOT_EIGHT|SLOT_NINE|SLOT_TEN|SLOT_ELEVEN|SLOT_TWELEVE. The current value set to " + *dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow)
}

// getRecoveryWindowsInDays validates the RecoveryWindowsInDays value specified in the DBCS system's spec and returns it if it matches one of the allowed values (7, 15, 30, 45, or 60). If the value does not match any of the expected options, it defaults to 30 days and returns that value without an error. This function ensures that the recovery window configuration is correctly interpreted and applied when configuring database backup settings.
func getRecoveryWindowsInDays(dbcs *databasev4.DbcsSystem) (int, error) {

	var days int

	switch *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays {
	case 7:
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	case 15:
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	case 30:
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	case 45:
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	case 60:
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	default:
		days = 30
		return days, nil
	}
	//return days, fmt.Errorf("RecoveryWindowsInDays values can be 7|15|30|45|60 Days.")
}

// GetDBSystemopts retrieves the database system options for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks the storage management option specified in the DBCS spec and maps it to the corresponding enum value defined in the OCI SDK. If the storage management option is not specified or does not match any of the expected values, it defaults to ASM (Automatic Storage Management) and returns that as the storage management option for the database system.
func GetDBSystemopts(
	dbcs *databasev4.DbcsSystem) database.DbSystemOptions {

	dbSystemOpt := database.DbSystemOptions{}

	if dbcs.Spec.DbSystem.StorageManagement != "" {
		switch dbcs.Spec.DbSystem.StorageManagement {
		case "LVM":
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementLvm
		case "ASM":
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
		default:
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
		}
	} else {
		dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
	}

	return dbSystemOpt
}

// getLicenceModel retrieves the license model for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks the license model specified in the DBCS spec and maps it to the corresponding enum value defined in the OCI SDK. If the license model is not specified or does not match "BRING_YOUR_OWN_LICENSE", it defaults to "LICENSE_INCLUDED" and returns that as the license model for the database system.
func getLicenceModel(dbcs *databasev4.DbcsSystem) database.DbSystemLicenseModelEnum {
	if dbcs.Spec.DbSystem.LicenseModel == "BRING_YOUR_OWN_LICENSE" {
		return database.DbSystemLicenseModelBringYourOwnLicense

	}
	return database.DbSystemLicenseModelLicenseIncluded
}

// getDbWorkLoadType retrieves the database workload type for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks the DbWorkload value specified in the DBCS spec and maps it to the corresponding enum value defined in the OCI SDK. If the DbWorkload value is not specified or does not match "OLTP" or "DSS", it defaults to "DSS" and returns that as the database workload type for the database system.
func getDbWorkLoadType(dbcs *databasev4.DbcsSystem) (database.CreateDatabaseDetailsDbWorkloadEnum, error) {

	if strings.ToUpper(dbcs.Spec.DbSystem.DbWorkload) == "OLTP" {

		return database.CreateDatabaseDetailsDbWorkloadOltp, nil
	}
	if strings.ToUpper(dbcs.Spec.DbSystem.DbWorkload) == "DSS" {
		return database.CreateDatabaseDetailsDbWorkloadDss, nil

	}

	return database.CreateDatabaseDetailsDbWorkloadDss, fmt.Errorf("DbWorkload values can be OLTP|DSS. The current value set to %s", dbcs.Spec.DbSystem.DbWorkload)
}

// GetNodeCount retrieves the number of nodes for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks if the NodeCount value is specified in the DBCS spec and returns it; otherwise, it defaults to 1 node and returns that as the node count for the database system.
func GetNodeCount(
	dbcs *databasev4.DbcsSystem) int {

	if dbcs.Spec.DbSystem.NodeCount != nil {
		return *dbcs.Spec.DbSystem.NodeCount
	}
	return 1
}

// GetInitialStorage retrieves the initial storage size in GB for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks if the InitialDataStorageSizeInGB value is specified in the DBCS spec and returns it; otherwise, it defaults to 256 GB and returns that as the initial storage size for the database system.
func GetInitialStorage(
	dbcs *databasev4.DbcsSystem) int {

	if dbcs.Spec.DbSystem.InitialDataStorageSizeInGB > 0 {
		return dbcs.Spec.DbSystem.InitialDataStorageSizeInGB
	}
	return 256
}

// GetDBEdition retrieves the database edition for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks if the ClusterName is specified in the DBCS spec and returns "Enterprise Edition Extreme Performance" if it is; otherwise, it checks the DbEdition value specified in the DBCS spec and maps it to the corresponding enum value defined in the OCI SDK. If the DbEdition value is not specified or does not match any of the expected values, it defaults to "Enterprise Edition" and returns that as the database edition for the database system.
func GetDBEdition(dbcs *databasev4.DbcsSystem) database.LaunchDbSystemDetailsDatabaseEditionEnum {

	if dbcs.Spec.DbSystem.ClusterName != "" {
		return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionExtremePerformance
	}

	if dbcs.Spec.DbSystem.DbEdition != "" {
		switch dbcs.Spec.DbSystem.DbEdition {
		case "STANDARD_EDITION":
			return database.LaunchDbSystemDetailsDatabaseEditionStandardEdition
		case "ENTERPRISE_EDITION":
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
		case "ENTERPRISE_EDITION_HIGH_PERFORMANCE":
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionHighPerformance
		case "ENTERPRISE_EDITION_EXTREME_PERFORMANCE":
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionExtremePerformance
		default:
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
		}
	}

	return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
}

// GetDBbDiskRedundancy retrieves the disk redundancy level for the DBCS system based on the specifications provided in the DbcsSystem resource. It checks if the ClusterName is specified in the DBCS spec and returns "HIGH" redundancy if it is; otherwise, it checks the DiskRedundancy value specified in the DBCS spec and maps it to the corresponding enum value defined in the OCI SDK. If the DiskRedundancy value is not specified or does not match "HIGH" or "NORMAL", it defaults to "NORMAL" and returns that as the disk redundancy level for the database system.
func GetDBbDiskRedundancy(
	dbcs *databasev4.DbcsSystem) database.LaunchDbSystemDetailsDiskRedundancyEnum {

	if dbcs.Spec.DbSystem.ClusterName != "" {
		return database.LaunchDbSystemDetailsDiskRedundancyHigh
	}

	switch dbcs.Spec.DbSystem.DiskRedundancy {
	case "HIGH":
		return database.LaunchDbSystemDetailsDiskRedundancyHigh
	case "NORMAL":
		return database.LaunchDbSystemDetailsDiskRedundancyNormal
	}

	return database.LaunchDbSystemDetailsDiskRedundancyNormal
}

// getWorkRequest retrieves the work request summaries for a given compartment ID, work request ID, and DBCS system. It sends a request to the Oracle Cloud Infrastructure (OCI) Work Requests service to list the work requests based on the provided criteria. The function returns a slice of WorkRequestSummary items that match the specified parameters, allowing for tracking and monitoring of asynchronous operations related to the DBCS system.
func getWorkRequest(compartmentID string, workID string, wrClient workrequests.WorkRequestClient, dbcs *databasev4.DbcsSystem) ([]workrequests.WorkRequestSummary, error) {
	var workReq []workrequests.WorkRequestSummary

	req := workrequests.ListWorkRequestsRequest{CompartmentId: &compartmentID, OpcRequestId: &workID, ResourceId: dbcs.Spec.Id}
	resp, err := wrClient.ListWorkRequests(context.Background(), req)
	if err != nil {
		return workReq, err
	}

	return resp.Items, nil
}

// GetKeyValue is a utility function that extracts the value of the "version" key from a given input string. It splits the input string into a list of key-value pairs, iterates through them to find the pair where the key is "version", and returns the corresponding value. If the "version" key is not found in the input string, it returns "noversion" as a default value. This function is useful for parsing version information from strings that contain multiple key-value pairs.
func GetKeyValue(str1 string) string {
	list1 := strings.Split(str1, " ")
	for _, value := range list1 {
		val1 := strings.Split(value, "=")
		if val1[0] == "version" {
			return val1[1]
		}
	}

	return "noversion"
}

// GetFmtStr is a utility function that formats a given input string by enclosing it in square brackets. It takes a string as input and returns a new string with the input value wrapped in square brackets. This function can be used for consistent formatting of strings, such as when constructing log messages or displaying values in a specific format.
func GetFmtStr(pstr string) string {

	return "[" + pstr + "]"
}

// checkValue checks if a given work request ID exists in the list of work requests associated with the DBCS system. It iterates through the work requests in the DBCS system's status and compares their operation IDs with the provided work request ID. If a match is found, it returns 1 to indicate that the work request ID exists; otherwise, it returns 0 to indicate that it does not exist. This function is useful for tracking the status of asynchronous operations related to the DBCS system by checking for specific work request IDs.
func checkValue(dbcs *databasev4.DbcsSystem, workID *string) int {

	status := 0
	//dbWorkRequest := databasev4.DbWorkrequests{}

	if len(dbcs.Status.WorkRequests) > 0 {
		for _, v := range dbcs.Status.WorkRequests {
			if *v.OperationId == *workID {
				status = 1
			}
		}
	}

	return status
}

// setValue updates the work request information in the status of the DBCS system based on the provided DbWorkrequests object. It iterates through the work requests in the DBCS system's status and compares their operation IDs with the operation ID of the provided DbWorkrequests object. If a match is found, it updates the corresponding fields (operation type, percent complete, time accepted, time finished, time started) in the DBCS system's status with the values from the DbWorkrequests object. This function is useful for keeping the status of the DBCS system up-to-date with the latest information about ongoing work requests.
func setValue(dbcs *databasev4.DbcsSystem, dbWorkRequest databasev4.DbWorkrequests) {

	//var status int = 1
	//dbWorkRequest := databasev4.DbWorkrequests{}
	counter := 0
	if len(dbcs.Status.WorkRequests) > 0 {
		for _, v := range dbcs.Status.WorkRequests {
			if *v.OperationId == *dbWorkRequest.OperationId {
				dbcs.Status.WorkRequests[counter].OperationId = dbWorkRequest.OperationId
				dbcs.Status.WorkRequests[counter].OperationType = dbWorkRequest.OperationType
				dbcs.Status.WorkRequests[counter].PercentComplete = dbWorkRequest.PercentComplete
				dbcs.Status.WorkRequests[counter].TimeAccepted = dbWorkRequest.TimeAccepted
				dbcs.Status.WorkRequests[counter].TimeFinished = dbWorkRequest.TimeFinished
				dbcs.Status.WorkRequests[counter].TimeStarted = dbWorkRequest.TimeStarted
			}
			counter++
		}
	}

}
