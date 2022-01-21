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
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v51/common"
	"github.com/oracle/oci-go-sdk/v51/database"
	"github.com/oracle/oci-go-sdk/v51/secrets"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// UpdateAutonomousDatabaseStatus sets the status subresource of AutonomousDatabase
func UpdateAutonomousDatabaseStatus(kubeClient client.Client, adb *dbv1alpha1.AutonomousDatabase) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curADB := &dbv1alpha1.AutonomousDatabase{}

		namespacedName := types.NamespacedName{
			Namespace: adb.GetNamespace(),
			Name:      adb.GetName(),
		}

		if err := kubeClient.Get(context.TODO(), namespacedName, curADB); err != nil {
			return err
		}

		curADB.Status = adb.Status
		return kubeClient.Status().Update(context.TODO(), curADB)
	})
}

func createWalletSecret(kubeClient client.Client, namespacedName types.NamespacedName, data map[string][]byte) error {
	// Create the secret with the wallet data
	stringData := map[string]string{}
	for key, val := range data {
		stringData[key] = string(val)
	}

	walletSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
		StringData: stringData,
	}

	if err := kubeClient.Create(context.TODO(), walletSecret); err != nil {
		return err
	}
	return nil
}

func CreateWalletSecret(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, secretClient secrets.SecretsClient, adb *dbv1alpha1.AutonomousDatabase) error {
	// Kube Secret which contains Instance Wallet
	walletName := adb.Spec.Details.Wallet.Name
	if walletName == nil {
		walletName = common.String(adb.GetName() + "-instance-wallet")
	}

	// No-op if Wallet is already downloaded
	walletNamespacedName := types.NamespacedName{
		Namespace: adb.GetNamespace(),
		Name:      *walletName,
	}
	walletSecret := &corev1.Secret{}
	if err := kubeClient.Get(context.TODO(), walletNamespacedName, walletSecret); err == nil {
		return nil
	}

	data, err := oci.GetWallet(logger, kubeClient, dbClient, secretClient, adb)
	if err != nil {
		return err
	}

	if err := createWalletSecret(kubeClient, walletNamespacedName, data); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Wallet is stored in the Secret %s", *walletName))
	return nil
}

func getValidName(name string, usedNames map[string]bool) string {
	returnedName := name
	var i = 1

	_, ok := usedNames[returnedName]
	for ok {
		returnedName = fmt.Sprintf("%s-%d", name, i)
		_, ok = usedNames[returnedName]
		i++
	}

	return returnedName
}

// CreateBackupResources creates the all the AutonomousDatabasBackups that appears in the ListAutonomousDatabaseBackups request
// The backup object will not be created if it already exists.
func CreateBackupResources(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, adb *dbv1alpha1.AutonomousDatabase) error {
	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := fetchAutonomousDatabaseBackups(kubeClient, adb.Namespace)
	if err != nil {
		return err
	}

	if err := kubeClient.List(context.TODO(), backupList, &client.ListOptions{Namespace: adb.Namespace}); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if !apiErrors.IsNotFound(err) {
			return err
		}
	}

	usedNames := make(map[string]bool)
	usedBackupOCIDs := make(map[string]bool)

	for _, backup := range backupList.Items {
		usedNames[backup.Name] = true

		// Add both Spec.AutonomousDatabaseBackupOCID and Status.AutonomousDatabaseBackupOCID.
		// If it's a backup created from the operator, it won't have the OCID under the spec.
		// if the backup isn't ready yet, it won't have the OCID under the status.
		if backup.Spec.AutonomousDatabaseBackupOCID != "" {
			usedBackupOCIDs[backup.Spec.AutonomousDatabaseBackupOCID] = true
		}
		if backup.Status.AutonomousDatabaseBackupOCID != "" {
			usedBackupOCIDs[backup.Status.AutonomousDatabaseBackupOCID] = true
		}
	}

	resp, err := oci.ListAutonomousDatabaseBackups(dbClient, adb)
	if err != nil {
		return err
	}

	for _, backupSummary := range resp.Items {
		// Create the resource if the AutonomousDatabaseBackupOCID doesn't exist
		_, ok := usedBackupOCIDs[*backupSummary.Id]
		if !ok {
			// Convert the string to lowercase, and replace spaces, commas, and colons with hyphens
			backupName := *backupSummary.DisplayName
			backupName = strings.ToLower(backupName)

			re, err := regexp.Compile(`[^-a-zA-Z0-9]`)
			if err != nil {
				return err
			}
			backupName = re.ReplaceAllString(backupName, "-")
			backupName = getValidName(backupName, usedNames)

			backup := &dbv1alpha1.AutonomousDatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       adb.GetNamespace(),
					Name:            backupName,
					OwnerReferences: newOwnerReference(adb),
				},
				Spec: dbv1alpha1.AutonomousDatabaseBackupSpec{
					AutonomousDatabaseBackupOCID: *backupSummary.Id,
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: adb.Spec.OCIConfig.ConfigMapName,
						SecretName:    adb.Spec.OCIConfig.SecretName,
					},
				},
			}

			if err := kubeClient.Create(context.TODO(), backup); err != nil {
				return err
			}

			// Add the used names and ocids
			usedNames[backupName] = true
			usedBackupOCIDs[*backupSummary.AutonomousDatabaseId] = true

			logger.Info("Create AutonomousDatabaseBackup " + backupName)
		}
	}

	return nil
}
