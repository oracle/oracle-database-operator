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
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

// Returns the first AutonomousDatabase resource that matches the AutonomousDatabaseOCID of the backup
// If the AutonomousDatabase doesn't exist, returns a nil
func getOwnerAutonomousDatabase(kubeClient client.Client, namespace string, adbOCID string) (*dbv1alpha1.AutonomousDatabase, error) {
	adbList, err := fetchAutonomousDatabases(kubeClient, namespace)
	if err != nil {
		return nil, err
	}

	for _, adb := range adbList.Items {
		if adb.Spec.Details.AutonomousDatabaseOCID != nil && *adb.Spec.Details.AutonomousDatabaseOCID == adbOCID {
			return &adb, nil
		}
	}

	return nil, nil
}

// SetOwnerAutonomousDatabase sets the owner of the AutonomousDatabaseBackup if the AutonomousDatabase resource with the same database OCID is found
func SetOwnerAutonomousDatabase(logger logr.Logger, kubeClient client.Client, backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	adb, err := getOwnerAutonomousDatabase(kubeClient, backup.Namespace, backup.Status.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	if adb != nil {
		backup.SetOwnerReferences(newOwnerReference(adb))
		updateAutonomousDatabaseBackupResource(logger, kubeClient, backup)
		logger.Info(fmt.Sprintf("Set the owner of %s to %s", backup.Name, adb.Name))
	}

	return nil
}

// update the spec and the objectMeta
func updateAutonomousDatabaseBackupResource(logger logr.Logger, kubeClient client.Client, backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseBackup{}

		namespacedName := types.NamespacedName{
			Namespace: backup.GetNamespace(),
			Name:      backup.GetName(),
		}

		if err := kubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Spec = *backup.Spec.DeepCopy()
		curBackup.ObjectMeta = *backup.ObjectMeta.DeepCopy()
		return kubeClient.Update(context.TODO(), curBackup)
	}); err != nil {
		return err
	}

	// Update status
	if statusErr := UpdateAutonomousDatabaseBackupStatus(kubeClient, backup); statusErr != nil {
		return statusErr
	}
	logger.Info("Update local resource AutonomousDatabase successfully")

	return nil
}

// UpdateAutonomousDatabaseBackupStatus updates the status subresource of AutonomousDatabaseBackup
func UpdateAutonomousDatabaseBackupStatus(kubeClient client.Client, adbBackup *dbv1alpha1.AutonomousDatabaseBackup) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseBackup{}

		namespacedName := types.NamespacedName{
			Namespace: adbBackup.GetNamespace(),
			Name:      adbBackup.GetName(),
		}

		if err := kubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Status = adbBackup.Status
		return kubeClient.Status().Update(context.TODO(), curBackup)
	})
}
