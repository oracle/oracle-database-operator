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

package autonomousdatabase

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v51/common"
	"github.com/oracle/oci-go-sdk/v51/database"
	"github.com/oracle/oci-go-sdk/v51/workrequests"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/oci"
	"github.com/oracle/oracle-database-operator/commons/oci/ociutil"
)

func RestoreAutonomousDatabaseAndWait(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	restore *dbv1alpha1.AutonomousDatabaseRestore) (lifecycleState workrequests.WorkRequestStatusEnum, err error) {

	var restoreTime *common.SDKTime
	var adbOCID string

	if restore.Spec.BackupName != "" {
		backup := &dbv1alpha1.AutonomousDatabaseBackup{}
		namespacedName := types.NamespacedName{Namespace: restore.Namespace, Name: restore.Spec.BackupName}
		if err := kubeClient.Get(context.TODO(), namespacedName, backup); err != nil {
			return "", err
		}

		if backup.Status.TimeEnded == "" {
			return "", errors.New("broken backup: ended time is missing in the AutonomousDatabaseBackup " + backup.GetName())
		}
		restoreTime, err = ociutil.ParseDisplayTime(backup.Status.TimeEnded)
		if err != nil {
			return "", err
		}

		adbOCID = backup.Status.AutonomousDatabaseOCID

	} else if restore.Spec.PointInTime.TimeStamp != "" {
		// The validation of the pitr.timestamp has been handled by the webhook, so the error return is ignored
		restoreTime, _ = ociutil.ParseDisplayTime(restore.Spec.PointInTime.TimeStamp)
		adbOCID = restore.Spec.PointInTime.AutonomousDatabaseOCID
	}

	resp, err := oci.RestoreAutonomousDatabase(dbClient, adbOCID, restoreTime)
	if err != nil {
		logger.Error(err, "Fail to restore Autonomous Database")
		return "", nil
	}

	// Update status and wait for the work finish if a request is sent. Note that some of the requests (e.g. update displayName) won't return a work request ID.
	// It's important to update the status by reference otherwise Reconcile() won't be able to get the latest values
	status := &restore.Status
	status.DisplayName = *resp.AutonomousDatabase.DisplayName
	status.DbName = *resp.AutonomousDatabase.DbName
	status.AutonomousDatabaseOCID = *resp.AutonomousDatabase.Id
	
	workResp, err := oci.GetWorkRequest(workClient, resp.OpcWorkRequestId, nil)
	if err != nil {
		logger.Error(err, "Fail to get the work status. opcWorkRequestID = "+*resp.OpcWorkRequestId)
		return workResp.Status, nil
	}
	status.LifecycleState = workResp.Status
	
	UpdateAutonomousDatabaseRestoreStatus(kubeClient, restore)

	// Wait until the work is done
	lifecycleState, err = oci.GetWorkStatusAndWait(logger, workClient, resp.OpcWorkRequestId)
	if err != nil {
		logger.Error(err, "Fail to restore Autonomous Database. opcWorkRequestID = "+*resp.OpcWorkRequestId)
		return lifecycleState, nil
	}

	logger.Info("Restoration of Autonomous Database" + *resp.DisplayName + " finished")

	return lifecycleState, nil
}

// UpdateAutonomousDatabaseBackupStatus updates the status subresource of AutonomousDatabaseBackup
func UpdateAutonomousDatabaseRestoreStatus(kubeClient client.Client, adbRestore *dbv1alpha1.AutonomousDatabaseRestore) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseRestore{}

		namespacedName := types.NamespacedName{
			Namespace: adbRestore.GetNamespace(),
			Name:      adbRestore.GetName(),
		}

		if err := kubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Status = adbRestore.Status
		return kubeClient.Status().Update(context.TODO(), curBackup)
	})
}