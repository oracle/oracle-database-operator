/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v45/common"
	"github.com/oracle/oci-go-sdk/v45/database"
	"github.com/oracle/oci-go-sdk/v45/secrets"
	"github.com/oracle/oci-go-sdk/v45/workrequests"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

// CreateAutonomousDatabase sends a request to OCI to provision a database and returns the AutonomousDatabase OCID.
func CreateAutonomousDatabase(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, secretClient secrets.SecretsClient, adb *dbv1alpha1.AutonomousDatabase) (*database.CreateAutonomousDatabaseResponse, error) {
	adminPassword, err := getAdminPassword(logger, kubeClient, secretClient, adb)
	if err != nil {
		return nil, err
	}

	createAutonomousDatabaseDetails := database.CreateAutonomousDatabaseDetails{
		CompartmentId:        adb.Spec.Details.CompartmentOCID,
		DbName:               adb.Spec.Details.DbName,
		CpuCoreCount:         adb.Spec.Details.CPUCoreCount,
		DataStorageSizeInTBs: adb.Spec.Details.DataStorageSizeInTBs,
		AdminPassword:        common.String(adminPassword),
		DisplayName:          adb.Spec.Details.DisplayName,
		IsAutoScalingEnabled: adb.Spec.Details.IsAutoScalingEnabled,
		IsDedicated:          adb.Spec.Details.IsDedicated,
		DbVersion:            adb.Spec.Details.DbVersion,
		DbWorkload: database.CreateAutonomousDatabaseBaseDbWorkloadEnum(
			adb.Spec.Details.DbWorkload),
		SubnetId: adb.Spec.Details.SubnetOCID,
		NsgIds:   adb.Spec.Details.NsgOCIDs,
	}

	createAutonomousDatabaseRequest := database.CreateAutonomousDatabaseRequest{
		CreateAutonomousDatabaseDetails: createAutonomousDatabaseDetails,
	}

	resp, err := dbClient.CreateAutonomousDatabase(context.TODO(), createAutonomousDatabaseRequest)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// Get the desired admin password from either Kubernetes Secret or OCI Vault Secret.
func getAdminPassword(logger logr.Logger, kubeClient client.Client, secretClient secrets.SecretsClient, adb *dbv1alpha1.AutonomousDatabase) (string, error) {
	if adb.Spec.Details.AdminPassword.K8sSecretName != nil {
		logger.Info(fmt.Sprintf("Getting admin password from Secret %s", *adb.Spec.Details.AdminPassword.K8sSecretName))

		namespacedName := types.NamespacedName{
			Namespace: adb.GetNamespace(),
			Name:      *adb.Spec.Details.AdminPassword.K8sSecretName,
		}

		key := *adb.Spec.Details.AdminPassword.K8sSecretName
		adminPassword, err := getValueFromKubeSecret(kubeClient, namespacedName, key)
		if err != nil {
			return "", err
		}
		return adminPassword, nil

	} else if adb.Spec.Details.AdminPassword.OCISecretOCID != nil {
		logger.Info(fmt.Sprintf("Getting admin password from OCI Vault Secret OCID %s", *adb.Spec.Details.AdminPassword.OCISecretOCID))

		adminPassword, err := getValueFromVaultSecret(secretClient, *adb.Spec.Details.AdminPassword.OCISecretOCID)
		if err != nil {
			return "", err
		}
		return adminPassword, nil
	}
	return "", errors.New("should provide either AdminPasswordSecret or AdminPasswordOCID")
}

func getValueFromKubeSecret(kubeClient client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	if err := kubeClient.Get(context.TODO(), namespacedName, secret); err != nil {
		return "", err
	}

	val, ok := secret.Data[key]
	if !ok {
		return "", errors.New("Secret key not found: " + key)
	}
	return string(val), nil
}

// GetAutonomousDatabaseResource gets Autonomous Database information from a remote instance
// and return an AutonomousDatabase object
func GetAutonomousDatabaseResource(logger logr.Logger, dbClient database.DatabaseClient, adb *dbv1alpha1.AutonomousDatabase) (*dbv1alpha1.AutonomousDatabase, error) {
	getAutonomousDatabaseRequest := database.GetAutonomousDatabaseRequest{
		AutonomousDatabaseId: adb.Spec.Details.AutonomousDatabaseOCID,
	}

	response, err := dbClient.GetAutonomousDatabase(context.TODO(), getAutonomousDatabaseRequest)
	if err != nil {
		return nil, err
	}

	returnedADB := adb.UpdateAttrFromOCIAutonomousDatabase(response.AutonomousDatabase)

	logger.Info("Get information from remote AutonomousDatabase successfully")
	return returnedADB, nil
}

// isAttrChanged checks if the values of last successful object and current object are different.
// The function returns false if the types are mismatch or unknown.
// The function returns false if the current object has zero value (not applicable for boolean type).
func isAttrChanged(lastSucObj interface{}, curObj interface{}) bool {
	switch curObj.(type) {
	case string: // Enum
		// type check
		lastSucString, ok := lastSucObj.(string)
		if !ok {
			return false
		}
		curString := curObj.(string)

		if curString != "" && (lastSucString != curString) {
			return true
		}
	case *int:
		// type check
		lastSucIntPtr, ok := lastSucObj.(*int)
		if !ok {
			return false
		}
		curIntPtr, ok := curObj.(*int)

		if lastSucIntPtr != nil && curIntPtr != nil && *curIntPtr != 0 && *lastSucIntPtr != *curIntPtr {
			return true
		}
	case *string:
		// type check
		lastSucStringPtr, ok := lastSucObj.(*string)
		if !ok {
			return false
		}
		curStringPtr := curObj.(*string)

		if lastSucStringPtr != nil && curStringPtr != nil && *curStringPtr != "" && *lastSucStringPtr != *curStringPtr {
			return true
		}
	case *bool:
		// type check
		lastSucBoolPtr, ok := lastSucObj.(*bool)
		if !ok {
			return false
		}
		curBoolPtr := curObj.(*bool)

		// For boolean type, we don't have to check zero value
		if lastSucBoolPtr != nil && curBoolPtr != nil && *lastSucBoolPtr != *curBoolPtr {
			return true
		}
	case []string:
		// type check
		lastSucSlice, ok := lastSucObj.([]string)
		if !ok {
			return false
		}

		curSlice := curObj.([]string)
		if curSlice == nil {
			return false
		} else if len(lastSucSlice) != len(curSlice) {
			return true
		}

		for i, v := range lastSucSlice {
			if v != curSlice[i] {
				return true
			}
		}
	case map[string]string:
		// type check
		lastSucMap, ok := lastSucObj.(map[string]string)
		if !ok {
			return false
		}

		curMap := curObj.(map[string]string)
		if curMap == nil {
			return false
		} else if len(lastSucMap) != len(curMap) {
			return true
		}

		for k, v := range lastSucMap {
			if w, ok := curMap[k]; !ok || v != w {
				return true
			}
		}
	}
	return false
}

// UpdateGeneralAndPasswordAttributes updates the general and password attributes of the Autonomous Database.
// Based on the responses from OCI calls, we can split the attributes into the following five categories.
// AutonomousDatabaseOCID, CompartmentOCID, IsDedicated, and LifecycleState are excluded since they not applicable in updateAutonomousDatabaseRequest.
// Except for category 1, category 2 and 3 cannot be updated at the same time, i.e.,we can at most update category 1 plus another category 2,or 3.
// 1. General attribute: including DisplayName, DbName, DbWorkload, DbVersion, freeformTags, subnetOCID, nsgOCIDs, and whitelistedIPs. The general attributes can be updated together with one of the other categories in the same request.
// 2. Scale attribute: includining IsAutoScalingEnabled, CpuCoreCount and DataStorageSizeInTBs.
// 3. Password attribute: including AdminPasswordSecret and AdminPasswordOCID
// From the above rules, we group general and password attributes and send the update together in the same request, and then send the scale update in another request.
func UpdateGeneralAndPasswordAttributes(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient,
	secretClient secrets.SecretsClient, curADB *dbv1alpha1.AutonomousDatabase) (resp database.UpdateAutonomousDatabaseResponse, err error) {
	var shouldSendRequest = false

	lastSucSpec, err := curADB.GetLastSuccessfulSpec()
	if err != nil {
		return resp, err
	}

	// Prepare the update request
	updateAutonomousDatabaseDetails := database.UpdateAutonomousDatabaseDetails{}

	if isAttrChanged(lastSucSpec.Details.DisplayName, curADB.Spec.Details.DisplayName) {
		updateAutonomousDatabaseDetails.DisplayName = curADB.Spec.Details.DisplayName
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.DbName, curADB.Spec.Details.DbName) {
		updateAutonomousDatabaseDetails.DbName = curADB.Spec.Details.DbName
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.DbWorkload, curADB.Spec.Details.DbWorkload) {
		updateAutonomousDatabaseDetails.DbWorkload = database.UpdateAutonomousDatabaseDetailsDbWorkloadEnum(curADB.Spec.Details.DbWorkload)
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.DbVersion, curADB.Spec.Details.DbVersion) {
		updateAutonomousDatabaseDetails.DbVersion = curADB.Spec.Details.DbVersion
		shouldSendRequest = true
	}

	if isAttrChanged(lastSucSpec.Details.FreeformTags, curADB.Spec.Details.FreeformTags) {
		updateAutonomousDatabaseDetails.FreeformTags = curADB.Spec.Details.FreeformTags
		shouldSendRequest = true
	}

	if isAttrChanged(lastSucSpec.Details.FreeformTags, curADB.Spec.Details.FreeformTags) {
		updateAutonomousDatabaseDetails.FreeformTags = curADB.Spec.Details.FreeformTags
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.SubnetOCID, curADB.Spec.Details.SubnetOCID) {
		updateAutonomousDatabaseDetails.SubnetId = curADB.Spec.Details.SubnetOCID
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.NsgOCIDs, curADB.Spec.Details.NsgOCIDs) {
		updateAutonomousDatabaseDetails.NsgIds = curADB.Spec.Details.NsgOCIDs
		shouldSendRequest = true
	}

	if isAttrChanged(lastSucSpec.Details.AdminPassword.K8sSecretName, curADB.Spec.Details.AdminPassword.K8sSecretName) ||
		isAttrChanged(lastSucSpec.Details.AdminPassword.OCISecretOCID, curADB.Spec.Details.AdminPassword.OCISecretOCID) {
		// Get the adminPassword
		var adminPassword string

		adminPassword, err = getAdminPassword(logger, kubeClient, secretClient, curADB)
		if err != nil {
			return
		}
		updateAutonomousDatabaseDetails.AdminPassword = common.String(adminPassword)

		shouldSendRequest = true
	}

	// Send the request only when something changes
	if shouldSendRequest {

		logger.Info("Sending general attributes and ADMIN password update request")

		updateAutonomousDatabaseRequest := database.UpdateAutonomousDatabaseRequest{
			// AutonomousDatabaseId:            common.String(curADB.Spec.Details.AutonomousDatabaseOCID),
			AutonomousDatabaseId:            curADB.Spec.Details.AutonomousDatabaseOCID,
			UpdateAutonomousDatabaseDetails: updateAutonomousDatabaseDetails,
		}

		resp, err = dbClient.UpdateAutonomousDatabase(context.TODO(), updateAutonomousDatabaseRequest)
	}

	return
}

// UpdateScaleAttributes updates the scale attributes of the Autonomous Database
// Refer to UpdateGeneralAndPasswordAttributes for more details about how and why we separate the attributes in different calls.
func UpdateScaleAttributes(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient,
	curADB *dbv1alpha1.AutonomousDatabase) (resp database.UpdateAutonomousDatabaseResponse, err error) {
	var shouldSendRequest = false

	lastSucSpec, err := curADB.GetLastSuccessfulSpec()
	if err != nil {
		return resp, err
	}

	// Prepare the update request
	updateAutonomousDatabaseDetails := database.UpdateAutonomousDatabaseDetails{}

	if isAttrChanged(lastSucSpec.Details.DataStorageSizeInTBs, curADB.Spec.Details.DataStorageSizeInTBs) {
		updateAutonomousDatabaseDetails.DataStorageSizeInTBs = curADB.Spec.Details.DataStorageSizeInTBs
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.CPUCoreCount, curADB.Spec.Details.CPUCoreCount) {
		updateAutonomousDatabaseDetails.CpuCoreCount = curADB.Spec.Details.CPUCoreCount
		shouldSendRequest = true
	}
	if isAttrChanged(lastSucSpec.Details.IsAutoScalingEnabled, curADB.Spec.Details.IsAutoScalingEnabled) {
		updateAutonomousDatabaseDetails.IsAutoScalingEnabled = curADB.Spec.Details.IsAutoScalingEnabled
		shouldSendRequest = true
	}

	// Don't send the request if nothing is changed
	if shouldSendRequest {

		logger.Info("Sending scale attributes update request")

		updateAutonomousDatabaseRequest := database.UpdateAutonomousDatabaseRequest{
			// AutonomousDatabaseId:            common.String(curADB.Spec.Details.AutonomousDatabaseOCID),
			AutonomousDatabaseId:            curADB.Spec.Details.AutonomousDatabaseOCID,
			UpdateAutonomousDatabaseDetails: updateAutonomousDatabaseDetails,
		}

		resp, err = dbClient.UpdateAutonomousDatabase(context.TODO(), updateAutonomousDatabaseRequest)
	}

	return
}

// SetAutonomousDatabaseLifecycleState starts or stops AutonomousDatabase in OCI based on the LifeCycleState attribute
func SetAutonomousDatabaseLifecycleState(logger logr.Logger, dbClient database.DatabaseClient, adb *dbv1alpha1.AutonomousDatabase) (resp interface{}, err error) {
	lastSucSpec, err := adb.GetLastSuccessfulSpec()
	if err != nil {
		return resp, err
	}

	// Return if the desired lifecycle state is the same as the current lifecycle state
	if adb.Spec.Details.LifecycleState == lastSucSpec.Details.LifecycleState {
		return nil, nil
	}

	switch string(adb.Spec.Details.LifecycleState) {
	case string(database.AutonomousDatabaseLifecycleStateAvailable):
		logger.Info("Sending start request to the Autonomous Database " + *adb.Spec.Details.DbName)

		resp, err = startAutonomousDatabase(dbClient, *adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return
		}

	case string(database.AutonomousDatabaseLifecycleStateStopped):
		logger.Info("Sending stop request to the Autonomous Database " + *adb.Spec.Details.DbName)

		resp, err = stopAutonomousDatabase(dbClient, *adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return
		}

	case string(database.AutonomousDatabaseLifecycleStateTerminated):
		// Special case.
		if adb.Spec.Details.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminating {
			break
		}
		logger.Info("Sending teminate request to the Autonomous Database " + *adb.Spec.Details.DbName)

		resp, err = DeleteAutonomousDatabase(dbClient, *adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return
		}

	default:
		err = fmt.Errorf("invalid lifecycleState value: currently the operator only accept %s, %s and %s as the value of the lifecycleState parameter",
			database.AutonomousDatabaseLifecycleStateAvailable,
			database.AutonomousDatabaseLifecycleStateStopped,
			database.AutonomousDatabaseLifecycleStateTerminated)
	}

	return
}

// startAutonomousDatabase starts an Autonomous Database in OCI
func startAutonomousDatabase(dbClient database.DatabaseClient, adbOCID string) (resp database.StartAutonomousDatabaseResponse, err error) {
	startRequest := database.StartAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
	}

	resp, err = dbClient.StartAutonomousDatabase(context.Background(), startRequest)
	return
}

// stopAutonomousDatabase stops an Autonomous Database in OCI
func stopAutonomousDatabase(dbClient database.DatabaseClient, adbOCID string) (resp database.StopAutonomousDatabaseResponse, err error) {
	stopRequest := database.StopAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
	}

	resp, err = dbClient.StopAutonomousDatabase(context.Background(), stopRequest)
	return
}

// DeleteAutonomousDatabase terminates an Autonomous Database in OCI
func DeleteAutonomousDatabase(dbClient database.DatabaseClient, adbOCID string) (resp database.DeleteAutonomousDatabaseResponse, err error) {

	deleteRequest := database.DeleteAutonomousDatabaseRequest{
		AutonomousDatabaseId: common.String(adbOCID),
	}

	resp, err = dbClient.DeleteAutonomousDatabase(context.Background(), deleteRequest)
	return
}

// ListAutonomousDatabaseBackups returns a list of Autonomous Database backups
func ListAutonomousDatabaseBackups(dbClient database.DatabaseClient, adb *dbv1alpha1.AutonomousDatabase) (resp database.ListAutonomousDatabaseBackupsResponse, err error) {
	if adb.Spec.Details.AutonomousDatabaseOCID == nil {
		return resp, nil
	}

	listBackupRequest := database.ListAutonomousDatabaseBackupsRequest{
		AutonomousDatabaseId: adb.Spec.Details.AutonomousDatabaseOCID,
	}

	return dbClient.ListAutonomousDatabaseBackups(context.TODO(), listBackupRequest)
}

// CreateAutonomousDatabaseBackup creates an backup of Autonomous Database
func CreateAutonomousDatabaseBackup(logger logr.Logger, dbClient database.DatabaseClient, adbBackup *dbv1alpha1.AutonomousDatabaseBackup) (resp database.CreateAutonomousDatabaseBackupResponse, err error) {
	logger.Info("Creating Autonomous Database backup " + adbBackup.Spec.DisplayName)

	createBackupRequest := database.CreateAutonomousDatabaseBackupRequest{
		CreateAutonomousDatabaseBackupDetails: database.CreateAutonomousDatabaseBackupDetails{
			DisplayName:          &adbBackup.Spec.DisplayName,
			AutonomousDatabaseId: &adbBackup.Spec.AutonomousDatabaseOCID,
		},
	}

	return dbClient.CreateAutonomousDatabaseBackup(context.TODO(), createBackupRequest)
}

// GetAutonomousDatabaseBackup returns the response of GetAutonomousDatabaseBackupRequest
func GetAutonomousDatabaseBackup(dbClient database.DatabaseClient, backupOCID *string) (resp database.GetAutonomousDatabaseBackupResponse, err error) {
	getBackupRequest := database.GetAutonomousDatabaseBackupRequest{
		AutonomousDatabaseBackupId: backupOCID,
	}

	return dbClient.GetAutonomousDatabaseBackup(context.TODO(), getBackupRequest)
}

func WaitUntilWorkCompleted(logger logr.Logger, workClient workrequests.WorkRequestClient, opcWorkRequestID *string) error {
	if opcWorkRequestID == nil {
		return nil
	}

	logger.Info("Waiting for the work request to finish. opcWorkRequestID = " + *opcWorkRequestID)

	retryPolicy := getCompleteWorkRetryPolicy()
	// Apply wait until work complete retryPolicy
	workRequest := workrequests.GetWorkRequestRequest{
		WorkRequestId: opcWorkRequestID,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	// GetWorkRequest retries until the work status is SUCCEEDED
	if _, err := workClient.GetWorkRequest(context.TODO(), workRequest); err != nil {
		return err
	}

	return nil
}

func getCompleteWorkRetryPolicy() common.RetryPolicy {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if _, isServiceError := common.IsServiceError(r.Error); isServiceError {
			// Don't retry if it's service error. Sometimes it could be network error or other errors which prevents
			// request send to server; we do the retry in these cases.
			return false
		}

		if converted, ok := r.Response.(workrequests.GetWorkRequestResponse); ok {
			// do the retry until WorkReqeut Status is Succeeded  - ignore case (BMI-2652)
			return converted.Status != workrequests.WorkRequestStatusSucceeded
		}

		return true
	}

	return getRetryPolicy(shouldRetry)
}

func getConflictRetryPolicy() common.RetryPolicy {
	// retry for 409 conflict status code
	shouldRetry := func(r common.OCIOperationResponse) bool {
		return r.Error != nil && r.Response.HTTPResponse().StatusCode == 409
	}

	return getRetryPolicy(shouldRetry)
}

func getRetryPolicy(retryOperation func(common.OCIOperationResponse) bool) common.RetryPolicy {
	// maximum times of retry
	attempts := uint(10)

	nextDuration := func(r common.OCIOperationResponse) time.Duration {
		// you might want wait longer for next retry when your previous one failed
		// this function will return the duration as:
		// 1s, 2s, 4s, 8s, 16s, 32s, 64s etc...
		return time.Duration(math.Pow(float64(2), float64(r.AttemptNumber-1))) * time.Second
	}

	return common.NewRetryPolicy(attempts, retryOperation, nextDuration)
}
