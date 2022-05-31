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

	"github.com/oracle/oci-go-sdk/v64/common"
	"github.com/oracle/oci-go-sdk/v64/database"
	"github.com/oracle/oci-go-sdk/v64/workrequests"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

func GetDbHomeDetails(kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem) (database.CreateDbHomeDetails, error) {

	dbHomeDetails := database.CreateDbHomeDetails{}

	dbHomeReq, err := GetDbLatestVersion(dbClient, dbcs, "")
	if err != nil {
		return database.CreateDbHomeDetails{}, err
	}
	dbHomeDetails.DbVersion = &dbHomeReq

	dbDetailsReq, err := GetDBDetails(kubeClient, dbcs)
	if err != nil {
		return database.CreateDbHomeDetails{}, err
	}

	dbHomeDetails.Database = &dbDetailsReq

	return dbHomeDetails, nil
}

func GetDbLatestVersion(dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, dbSystemId string) (string, error) {

	//var provisionedDbcsSystemId string
	ctx := context.TODO()
	var version database.DbVersionSummary
	var sFlag int = 0
	var val int

	dbVersionReq := database.ListDbVersionsRequest{}
	if dbSystemId != "" {
		dbVersionReq.DbSystemId = common.String(dbSystemId)
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
				if val >= 18 {
					s3 := s1 + "c"
					if strings.EqualFold(s3, dbcs.Spec.DbSystem.DbVersion) {
						sFlag = 1
						break
					}
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

	if sFlag == 1 {
		return *version.Version, nil
	}
	return *version.Version, fmt.Errorf("no database version matched")
}

func getStr(str1 string, num int) string {
	return str1[0:num]
}

func GetDBDetails(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) (database.CreateDatabaseDetails, error) {
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
		} else {
			dbDetails.DbWorkload = database.CreateDatabaseDetailsDbWorkloadEnum(val)
		}
	}
	dbDetails.DbName = common.String(dbcs.Spec.DbSystem.DbName)
	if dbcs.Spec.DbSystem.PdbName != "" {
		dbDetails.PdbName = &dbcs.Spec.DbSystem.PdbName
	}

	//backup configuration
	if dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled != nil {
		if *dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled {
			backupConfig, err := getBackupConfig(kubeClient, dbcs)
			if err != nil {
				return dbDetails, err
			} else {
				dbDetails.DbBackupConfig = &backupConfig
			}
		}
	}

	return dbDetails, nil
}

func getBackupConfig(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) (database.DbBackupConfig, error) {
	backupConfig := database.DbBackupConfig{}

	if dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled != nil {
		if *dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled {
			backupConfig.AutoBackupEnabled = dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupEnabled
			val1, err := getBackupWindowEnum(dbcs)
			if err != nil {
				return backupConfig, err
			} else {
				backupConfig.AutoBackupWindow = database.DbBackupConfigAutoBackupWindowEnum(val1)
			}
		}

		if dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays != nil {
			val1, err := getRecoveryWindowsInDays(dbcs)
			if err != nil {
				return backupConfig, err
			} else {
				backupConfig.RecoveryWindowInDays = common.Int(val1)
			}

		}
	}

	return backupConfig, nil
}

func getBackupWindowEnum(dbcs *databasev1alpha1.DbcsSystem) (database.DbBackupConfigAutoBackupWindowEnum, error) {

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
	} else {
		return database.DbBackupConfigAutoBackupWindowOne, nil
	}

	//return database.DbBackupConfigAutoBackupWindowEight, fmt.Errorf("AutoBackupWindow values can be SLOT_ONE|SLOT_TWO|SLOT_THREE|SLOT_FOUR|SLOT_FIVE|SLOT_SIX|SLOT_SEVEN|SLOT_EIGHT|SLOT_NINE|SLOT_TEN|SLOT_ELEVEN|SLOT_TWELEVE. The current value set to " + *dbcs.Spec.DbSystem.DbBackupConfig.AutoBackupWindow)
}

func getRecoveryWindowsInDays(dbcs *databasev1alpha1.DbcsSystem) (int, error) {

	var days int

	if *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays == 7 {
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	} else if *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays == 15 {
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	} else if *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays == 30 {
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	} else if *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays == 45 {
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	} else if *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays == 60 {
		return *dbcs.Spec.DbSystem.DbBackupConfig.RecoveryWindowsInDays, nil
	} else {
		days = 30
		return days, nil
	}
	//return days, fmt.Errorf("RecoveryWindowsInDays values can be 7|15|30|45|60 Days.")
}

func GetDBSystemopts(
	dbcs *databasev1alpha1.DbcsSystem) database.DbSystemOptions {

	dbSystemOpt := database.DbSystemOptions{}

	if dbcs.Spec.DbSystem.StorageManagement != "" {
		if dbcs.Spec.DbSystem.StorageManagement == "LVM" {
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementLvm
		} else if dbcs.Spec.DbSystem.StorageManagement == "ASM" {
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
		} else {
			dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
		}
	} else {
		dbSystemOpt.StorageManagement = database.DbSystemOptionsStorageManagementAsm
	}

	return dbSystemOpt
}

func getLicenceModel(dbcs *databasev1alpha1.DbcsSystem) database.DbSystemLicenseModelEnum {
	if dbcs.Spec.DbSystem.LicenseModel == "BRING_YOUR_OWN_LICENSE" {
		return database.DbSystemLicenseModelBringYourOwnLicense

	}
	return database.DbSystemLicenseModelLicenseIncluded
}

func getDbWorkLoadType(dbcs *databasev1alpha1.DbcsSystem) (database.CreateDatabaseDetailsDbWorkloadEnum, error) {

	if strings.ToUpper(dbcs.Spec.DbSystem.DbWorkload) == "OLTP" {

		return database.CreateDatabaseDetailsDbWorkloadOltp, nil
	}
	if strings.ToUpper(dbcs.Spec.DbSystem.DbWorkload) == "DSS" {
		return database.CreateDatabaseDetailsDbWorkloadDss, nil

	}

	return database.CreateDatabaseDetailsDbWorkloadDss, fmt.Errorf("DbWorkload values can be OLTP|DSS. The current value set to " + dbcs.Spec.DbSystem.DbWorkload)
}

func GetNodeCount(
	dbcs *databasev1alpha1.DbcsSystem) int {

	if dbcs.Spec.DbSystem.NodeCount != nil {
		return *dbcs.Spec.DbSystem.NodeCount
	} else {
		return 1
	}
}

func GetInitialStorage(
	dbcs *databasev1alpha1.DbcsSystem) int {

	if dbcs.Spec.DbSystem.InitialDataStorageSizeInGB > 0 {
		return dbcs.Spec.DbSystem.InitialDataStorageSizeInGB
	}
	return 256
}

func GetDBEdition(dbcs *databasev1alpha1.DbcsSystem) database.LaunchDbSystemDetailsDatabaseEditionEnum {

	if dbcs.Spec.DbSystem.ClusterName != "" {
		return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionExtremePerformance
	}

	if dbcs.Spec.DbSystem.DbEdition != "" {
		if dbcs.Spec.DbSystem.DbEdition == "STANDARD_EDITION" {
			return database.LaunchDbSystemDetailsDatabaseEditionStandardEdition
		} else if dbcs.Spec.DbSystem.DbEdition == "ENTERPRISE_EDITION" {
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
		} else if dbcs.Spec.DbSystem.DbEdition == "ENTERPRISE_EDITION_HIGH_PERFORMANCE" {
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionHighPerformance
		} else if dbcs.Spec.DbSystem.DbEdition == "ENTERPRISE_EDITION_EXTREME_PERFORMANCE" {
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEditionExtremePerformance
		} else {
			return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
		}
	}

	return database.LaunchDbSystemDetailsDatabaseEditionEnterpriseEdition
}

func GetDBbDiskRedundancy(
	dbcs *databasev1alpha1.DbcsSystem) database.LaunchDbSystemDetailsDiskRedundancyEnum {

	if dbcs.Spec.DbSystem.ClusterName != "" {
		return database.LaunchDbSystemDetailsDiskRedundancyHigh
	}

	if dbcs.Spec.DbSystem.DiskRedundancy == "HIGH" {
		return database.LaunchDbSystemDetailsDiskRedundancyHigh
	} else if dbcs.Spec.DbSystem.DiskRedundancy == "NORMAL" {
		return database.LaunchDbSystemDetailsDiskRedundancyNormal
	}

	return database.LaunchDbSystemDetailsDiskRedundancyNormal
}

func getWorkRequest(workId string, wrClient workrequests.WorkRequestClient, dbcs *databasev1alpha1.DbcsSystem) ([]workrequests.WorkRequestSummary, error) {
	var workReq []workrequests.WorkRequestSummary

	req := workrequests.ListWorkRequestsRequest{CompartmentId: &dbcs.Spec.DbSystem.CompartmentId, OpcRequestId: &workId, ResourceId: dbcs.Spec.Id}
	resp, err := wrClient.ListWorkRequests(context.Background(), req)
	if err != nil {
		return workReq, err
	}

	return resp.Items, nil
}

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

func GetFmtStr(pstr string) string {

	return "[" + pstr + "]"
}

func checkValue(dbcs *databasev1alpha1.DbcsSystem, workId *string) int {

	var status int = 0
	//dbWorkRequest := databasev1alpha1.DbWorkrequests{}

	if len(dbcs.Status.WorkRequests) > 0 {
		for _, v := range dbcs.Status.WorkRequests {
			if *v.OperationId == *workId {
				status = 1
			}
		}
	}

	return status
}
func setValue(dbcs *databasev1alpha1.DbcsSystem, dbWorkRequest databasev1alpha1.DbWorkrequests) {

	//var status int = 1
	//dbWorkRequest := databasev1alpha1.DbWorkrequests{}
	var counter int = 0
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
			counter = counter + 1
		}
	}

}
