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

package e2eutil

import (
	"context"

	"github.com/oracle/oci-go-sdk/v63/common"
	"github.com/oracle/oci-go-sdk/v63/database"
	"io"
	"io/ioutil"
	"time"
)

func CreateAutonomousDatabase(dbClient database.DatabaseClient, compartmentID *string, dbName *string, adminPassword *string) (response database.CreateAutonomousDatabaseResponse, err error) {
	createAutonomousDatabaseDetails := database.CreateAutonomousDatabaseDetails{
		CompartmentId:        compartmentID,
		DbName:               dbName,
		DisplayName:          dbName,
		CpuCoreCount:         common.Int(1),
		DataStorageSizeInTBs: common.Int(1),
		AdminPassword:        adminPassword,
		IsAutoScalingEnabled: common.Bool(true),
		DbWorkload:           database.CreateAutonomousDatabaseBaseDbWorkloadEnum("OLTP"),
	}

	createAutonomousDatabaseRequest := database.CreateAutonomousDatabaseRequest{
		CreateAutonomousDatabaseDetails: createAutonomousDatabaseDetails,
	}

	return dbClient.CreateAutonomousDatabase(context.TODO(), createAutonomousDatabaseRequest)
}

func GetAutonomousDatabase(dbClient database.DatabaseClient, databaseOCID *string, retryPolicy *common.RetryPolicy) (database.GetAutonomousDatabaseResponse, error) {
	getRequest := database.GetAutonomousDatabaseRequest{
		AutonomousDatabaseId: databaseOCID,
	}

	if retryPolicy != nil {
		getRequest.RequestMetadata = common.RequestMetadata{
			RetryPolicy: retryPolicy,
		}
	}

	return dbClient.GetAutonomousDatabase(context.TODO(), getRequest)
}

func ListAutonomousDatabases(dbClient database.DatabaseClient, compartmentOCID *string, displayName *string) (database.ListAutonomousDatabasesResponse, error) {
	listRequest := database.ListAutonomousDatabasesRequest{
		CompartmentId: compartmentOCID,
		DisplayName:   displayName,
	}
	return dbClient.ListAutonomousDatabases(context.TODO(), listRequest)
}

func deleteAutonomousDatabase(dbClient database.DatabaseClient, databaseOCID *string) error {
	if databaseOCID == nil {
		return nil
	}

	req := database.DeleteAutonomousDatabaseRequest{
		AutonomousDatabaseId: databaseOCID,
	}

	if _, err := dbClient.DeleteAutonomousDatabase(context.TODO(), req); err != nil {
		return err
	}

	return nil
}

// DeleteAutonomousDatabase terminates the database if it exists and is not in TERMINATED state
func DeleteAutonomousDatabase(dbClient database.DatabaseClient, databaseOCID *string) error {
	if databaseOCID == nil {
		return nil
	}

	resp, err := GetAutonomousDatabase(dbClient, databaseOCID, nil)
	if err != nil {
		return nil
	}
	if resp.AutonomousDatabase.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminated {
		deleteAutonomousDatabase(dbClient, databaseOCID)
	}

	return nil
}

func generateRetryPolicy(retryFunc func(r common.OCIOperationResponse) bool) common.RetryPolicy {
	// Retry up to 4 times every 10 seconds.
	attempts := uint(6)
	nextDuration := func(r common.OCIOperationResponse) time.Duration {
		return 10 * time.Second
	}
	return common.NewRetryPolicy(attempts, retryFunc, nextDuration)
}

func NewLifecycleStateRetryPolicyADB(lifecycleState database.AutonomousDatabaseLifecycleStateEnum) common.RetryPolicy {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if databaseResponse, ok := r.Response.(database.GetAutonomousDatabaseResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return databaseResponse.LifecycleState != lifecycleState
		}
		return true
	}
	return generateRetryPolicy(shouldRetry)
}

func NewLifecycleStateRetryPolicyACD(lifecycleState database.AutonomousContainerDatabaseLifecycleStateEnum) common.RetryPolicy {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if databaseResponse, ok := r.Response.(database.GetAutonomousContainerDatabaseResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return databaseResponse.LifecycleState != lifecycleState
		}
		return true
	}
	return generateRetryPolicy(shouldRetry)
}

func DownloadWalletZip(dbClient database.DatabaseClient, databaseOCID *string, walletPassword *string) (string, error) {

	req := database.GenerateAutonomousDatabaseWalletRequest{
		AutonomousDatabaseId: common.String(*databaseOCID),
		GenerateAutonomousDatabaseWalletDetails: database.GenerateAutonomousDatabaseWalletDetails{
			Password: common.String(*walletPassword),
		},
	}

	resp, err := dbClient.GenerateAutonomousDatabaseWallet(context.TODO(), req)
	if err != nil {
		return "", err
	}

	// Create a temp file wallet*.zip
	const walletFileName = "wallet*.zip"
	outZip, err := ioutil.TempFile("", walletFileName)
	if err != nil {
		return "", err
	}
	defer outZip.Close()

	// Save the wallet in wallet*.zip
	if _, err := io.Copy(outZip, resp.Content); err != nil {
		return "", err
	}

	return outZip.Name(), nil
}
