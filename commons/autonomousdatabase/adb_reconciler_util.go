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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/database"
	"github.com/oracle/oci-go-sdk/v54/secrets"
	"github.com/oracle/oci-go-sdk/v54/workrequests"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

func UpdateAutonomousDatabaseDetails(logger logr.Logger, kubeClient client.Client, adb *dbv1alpha1.AutonomousDatabase) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curADB := &dbv1alpha1.AutonomousDatabase{}

		namespacedName := types.NamespacedName{
			Namespace: adb.GetNamespace(),
			Name:      adb.GetName(),
		}

		if err := kubeClient.Get(context.TODO(), namespacedName, curADB); err != nil {
			return err
		}

		curADB.Spec.Details = adb.Spec.Details
		return kubeClient.Update(context.TODO(), curADB)
	}); err != nil {
		return err
	}

	// Update status
	if statusErr := UpdateAutonomousDatabaseStatus(kubeClient, adb); statusErr != nil {
		return statusErr
	}
	logger.Info("Update local resource AutonomousDatabase successfully")
	return nil
}

func UpdateGeneralAndPasswordAttributesAndWait(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	secretClient secrets.SecretsClient,
	workClient workrequests.WorkRequestClient,
	adb *dbv1alpha1.AutonomousDatabase) error {

	updateGenPassResp, err := oci.UpdateGeneralAndPasswordAttributes(logger, kubeClient, dbClient, secretClient, adb)
	if err != nil {
		logger.Error(err, "Fail to update Autonomous Database")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := UpdateAutonomousDatabaseStatus(kubeClient, adb); statusErr != nil {
			return statusErr
		}
		// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
		return nil
	}

	// Wait for the work finish if a request is sent. Note that some of the requests (e.g. update displayName) won't return a work request ID.
	if updateGenPassResp.OpcWorkRequestId != nil {
		if err := UpdateAutonomousDatabaseStatusAndWait(logger, kubeClient, workClient, adb, updateGenPassResp.AutonomousDatabase.LifecycleState, updateGenPassResp.OpcWorkRequestId); err != nil {
			logger.Error(err, "Fail to update Autonomous Database. opcWorkRequestID = "+*updateGenPassResp.OpcWorkRequestId)
		}

		logger.Info("Update AutonomousDatabase " + *adb.Spec.Details.DbName + " succesfully")
	}

	return nil
}

func UpdateScaleAttributesAndWait(logger logr.Logger, kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	adb *dbv1alpha1.AutonomousDatabase) error {

	scaleResp, err := oci.UpdateScaleAttributes(logger, kubeClient, dbClient, adb)
	if err != nil {
		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := UpdateAutonomousDatabaseStatus(kubeClient, adb); statusErr != nil {
			return statusErr
		}
		// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
		return err
	}

	if scaleResp.OpcWorkRequestId != nil {
		if err := UpdateAutonomousDatabaseStatusAndWait(logger, kubeClient, workClient, adb, scaleResp.AutonomousDatabase.LifecycleState, scaleResp.OpcWorkRequestId); err != nil {
			logger.Error(err, "Fail to scale the Autonomous Database. opcWorkRequestID = "+*scaleResp.OpcWorkRequestId)
		}

		logger.Info("Scale AutonomousDatabase " + *adb.Spec.Details.DbName + " succesfully")
	}

	return nil
}

// The logic of updating the network access configurations is as follows:
// 1. Shared databases:
// 	 If the network access type changes
//   a. to PUBLIC:
//     was RESTRICTED: re-enable IsMTLSConnectionRequired if it's not. Then set WhitelistedIps to an array with a single empty string entry.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if it's not. Then set PrivateEndpointLabel to an emtpy string.
//   b. to RESTRICTED:
//     was PUBLIC: set WhitelistedIps to desired IPs/CIDR blocks/VCN OCID. Configure the IsMTLSConnectionRequired settings if it is set to disabled.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if it's not. Set the type to PUBLIC first, and then configure the WhitelistedIps. Finally resume the IsMTLSConnectionRequired settings if it was, or is configured as disabled.
//   c. to PRIVATE:
//     was PUBLIC: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//     was RESTRICTED: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//
// 	 Otherwise, if the network access type remains the same, apply the network configuration, and then set the IsMTLSConnectionRequired.
//
// 2. Dedicated databases:
//   Apply the configs directly
func UpdateNetworkAttributes(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	curADB *dbv1alpha1.AutonomousDatabase) error {

	lastSucSpec, err := curADB.GetLastSuccessfulSpec()
	if err != nil {
		return err
	}

	if !*curADB.Spec.Details.IsDedicated {
		var lastAccessType = lastSucSpec.Details.NetworkAccess.AccessType
		var curAccessType = curADB.Spec.Details.NetworkAccess.AccessType

		if oci.IsAttrChanged(lastAccessType, curAccessType) {
			switch curAccessType {
			case dbv1alpha1.NetworkAccessTypePublic:
				// OCI validation requires IsMTLSConnectionRequired to be enabled before changing the network access type to PUBLIC
				if !*lastSucSpec.Details.NetworkAccess.IsMTLSConnectionRequired {
					curADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = common.Bool(true)
					if err := updateMTLSAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
						return err
					}
				}

				if err := setNetworkAccessPublicAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
					return err
				}
			case dbv1alpha1.NetworkAccessTypeRestricted:
				// If the access type was PRIVATE, then OCI validation requires IsMTLSConnectionRequired
				// to be enabled before setting ACL. Also we can only change the network access type from
				// PRIVATE to PUBLIC.
				if lastAccessType == dbv1alpha1.NetworkAccessTypePrivate {
					if !*lastSucSpec.Details.NetworkAccess.IsMTLSConnectionRequired {
						var oldMTLS bool = *curADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired
						curADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = common.Bool(true)
						if err := updateMTLSAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
							return err
						}
						// restore IsMTLSConnectionRequired
						curADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = &oldMTLS
					}

					if err := setNetworkAccessPublicAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
						return err
					}
				}

				if err := updateNetworkAccessAttributesAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
					return err
				}

				if err := updateMTLSAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
					return err
				}
			case dbv1alpha1.NetworkAccessTypePrivate:
				if err := updateNetworkAccessAttributesAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
					return err
				}

				if err := updateMTLSAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
					return err
				}
			}
		} else {
			// Access type doesn't change
			if err := updateNetworkAccessAttributesAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
				return err
			}

			if err := updateMTLSAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
				return err
			}
		}
	} else {
		// Dedicated database
		if err := updateNetworkAccessAttributesAndWait(logger, kubeClient, dbClient, workClient, curADB); err != nil {
			return err
		}
	}

	return nil
}

func updateMTLSAndWait(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	curADB *dbv1alpha1.AutonomousDatabase) error {

	resp, err := oci.UpdateMTLSConnectionRequired(logger, dbClient, curADB)
	if err != nil {
		return err
	}

	if resp.OpcWorkRequestId != nil {
		if err := UpdateAutonomousDatabaseStatusAndWait(logger, kubeClient, workClient, curADB, resp.AutonomousDatabase.LifecycleState, resp.OpcWorkRequestId); err != nil {
			logger.Error(err, "Fail to update Autonomous Database MTLS. opcWorkRequestID = "+*resp.OpcWorkRequestId)
		}
	}

	return nil
}

func setNetworkAccessPublicAndWait(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	curADB *dbv1alpha1.AutonomousDatabase) error {

	resp, err := oci.SetNetworkAccessPublic(logger, dbClient, curADB)
	if err != nil {
		return err
	}

	if resp.OpcWorkRequestId != nil {
		if err := UpdateAutonomousDatabaseStatusAndWait(logger, kubeClient, workClient, curADB, resp.AutonomousDatabase.LifecycleState, resp.OpcWorkRequestId); err != nil {
			logger.Error(err, "Fail to update the network access options to public. opcWorkRequestID = "+*resp.OpcWorkRequestId)
		}
	}

	return nil
}

func updateNetworkAccessAttributesAndWait(logger logr.Logger,
	kubeClient client.Client,
	dbClient database.DatabaseClient,
	workClient workrequests.WorkRequestClient,
	curADB *dbv1alpha1.AutonomousDatabase) error {

	resp, err := oci.UpdateNetworkAccessAttributes(logger, dbClient, curADB)
	if err != nil {
		return err
	}

	if resp.OpcWorkRequestId != nil {
		if err := UpdateAutonomousDatabaseStatusAndWait(logger, kubeClient, workClient, curADB, resp.AutonomousDatabase.LifecycleState, resp.OpcWorkRequestId); err != nil {
			logger.Error(err, "Fail to update the network access of Autonomous Database. opcWorkRequestID = "+*resp.OpcWorkRequestId)
		}
	}

	return nil
}

func UpdateAutonomousDatabaseStatusAndWait(logger logr.Logger, kubeClient client.Client,
	workClient workrequests.WorkRequestClient,
	adb *dbv1alpha1.AutonomousDatabase,
	desiredLifecycleState database.AutonomousDatabaseLifecycleStateEnum,
	opcWorkRequestID *string) error {

	// Update status.state
	adb.Status.LifecycleState = desiredLifecycleState
	if statusErr := UpdateAutonomousDatabaseStatus(kubeClient, adb); statusErr != nil {
		return statusErr
	}

	if _, err := oci.GetWorkStatusAndWait(logger, workClient, opcWorkRequestID); err != nil {
		return err
	}

	return nil
}

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

// SyncBackupResources get the list of AutonomousDatabasBackups and
// create a backup object if it's not found in the same namespace
func SyncBackupResources(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, adb *dbv1alpha1.AutonomousDatabase) error {
	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := fetchAutonomousDatabaseBackups(kubeClient, adb.Namespace)
	if err != nil {
		return err
	}

	curBackupNames := make(map[string]bool)
	curBackupOCIDs := make(map[string]bool)

	for _, backup := range backupList.Items {
		curBackupNames[backup.Name] = true

		if backup.Status.AutonomousDatabaseBackupOCID != "" {
			curBackupOCIDs[backup.Status.AutonomousDatabaseBackupOCID] = true
		}
	}

	resp, err := oci.ListAutonomousDatabaseBackups(dbClient, adb)
	if err != nil {
		return err
	}

	for _, backupSummary := range resp.Items {
		// Create the resource if the AutonomousDatabaseBackupOCID doesn't exist
		_, ok := curBackupOCIDs[*backupSummary.Id]
		if !ok {
			// Convert the string to lowercase, and replace spaces, commas, and colons with hyphens
			backupName := *backupSummary.DisplayName
			backupName = strings.ToLower(backupName)

			re, err := regexp.Compile(`[^-a-zA-Z0-9]`)
			if err != nil {
				return err
			}
			backupName = re.ReplaceAllString(backupName, "-")
			backupName = getValidName(backupName, curBackupNames)

			if err := createBackupResource(kubeClient, backupName, backupSummary, adb); err != nil {
				return err
			}

			// Add the used names and ocids
			curBackupNames[backupName] = true
			curBackupOCIDs[*backupSummary.AutonomousDatabaseId] = true

			logger.Info("Create AutonomousDatabaseBackup " + backupName)
		}
	}

	return nil
}

func createBackupResource(kubeClient client.Client,
	backupName string,
	backupSummary database.AutonomousDatabaseBackupSummary,
	adb *dbv1alpha1.AutonomousDatabase) error {

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

	return nil
}
