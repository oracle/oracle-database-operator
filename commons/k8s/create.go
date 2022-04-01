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

package k8s

import (
	"context"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	"github.com/oracle/oci-go-sdk/v63/common"
	"github.com/oracle/oci-go-sdk/v63/database"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateSecret(kubeClient client.Client, namespace string, name string, data map[string][]byte, owner client.Object, label map[string]string) error {
	ownerReference := NewOwnerReference(owner)

	// Create the secret with the wallet data
	stringData := map[string]string{}
	for key, val := range data {
		stringData[key] = string(val)
	}

	walletSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			OwnerReferences: ownerReference,
			Labels:          label,
		},
		StringData: stringData,
	}

	if err := kubeClient.Create(context.TODO(), walletSecret); err != nil {
		return err
	}
	return nil
}

func CreateAutonomousBackup(kubeClient client.Client,
	backupName string,
	backupSummary database.AutonomousDatabaseBackupSummary,
	ownerADB *dbv1alpha1.AutonomousDatabase) error {

	backup := &dbv1alpha1.AutonomousDatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       ownerADB.GetNamespace(),
			Name:            backupName,
			OwnerReferences: NewOwnerReference(ownerADB),
		},
		Spec: dbv1alpha1.AutonomousDatabaseBackupSpec{
			Target: dbv1alpha1.TargetSpec{
				K8sADB: dbv1alpha1.K8sADBSpec{
					Name: common.String(ownerADB.Name),
				},
			},
			DisplayName:                  backupSummary.DisplayName,
			AutonomousDatabaseBackupOCID: backupSummary.Id,
			OCIConfig: dbv1alpha1.OCIConfigSpec{
				ConfigMapName: ownerADB.Spec.OCIConfig.ConfigMapName,
				SecretName:    ownerADB.Spec.OCIConfig.SecretName,
			},
		},
	}

	if err := kubeClient.Create(context.TODO(), backup); err != nil {
		return err
	}

	return nil
}
