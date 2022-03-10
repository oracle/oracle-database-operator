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
	"errors"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

// Returns the first AutonomousDatabase resource that matches the AutonomousDatabaseOCID of the backup
// If the AutonomousDatabase doesn't exist, returns a nil
func FetchAutonomousDatabase(kubeClient client.Client, namespace string, ocid string) (*dbv1alpha1.AutonomousDatabase, error) {
	adbList, err := fetchAutonomousDatabases(kubeClient, namespace)
	if err != nil {
		return nil, err
	}

	for _, adb := range adbList.Items {
		if adb.Spec.Details.AutonomousDatabaseOCID != nil && *adb.Spec.Details.AutonomousDatabaseOCID == ocid {
			return &adb, nil
		}
	}

	return nil, nil
}

func fetchAutonomousDatabases(kubeClient client.Client, namespace string) (*dbv1alpha1.AutonomousDatabaseList, error) {
	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	adbList := &dbv1alpha1.AutonomousDatabaseList{}

	if err := kubeClient.List(context.TODO(), adbList, &client.ListOptions{Namespace: namespace}); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if !apiErrors.IsNotFound(err) {
			return adbList, err
		}
	}

	return adbList, nil
}

func FetchAutonomousDatabaseBackups(kubeClient client.Client, namespace string) (*dbv1alpha1.AutonomousDatabaseBackupList, error) {
	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList := &dbv1alpha1.AutonomousDatabaseBackupList{}

	if err := kubeClient.List(context.TODO(), backupList, &client.ListOptions{Namespace: namespace}); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if !apiErrors.IsNotFound(err) {
			return backupList, err
		}
	}

	return backupList, nil
}

func FetchConfigMap(kubeClient client.Client, namespace string, name string) (*corev1.ConfigMap, error) {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	configMap := &corev1.ConfigMap{}
	if err := kubeClient.Get(context.TODO(), namespacedName, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

func FetchSecret(kubeClient client.Client, namespace string, name string) (*corev1.Secret, error) {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	secret := &corev1.Secret{}
	if err := kubeClient.Get(context.TODO(), namespacedName, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func GetSecretValue(kubeClient client.Client, namespace string, name string, key string) (string, error) {
	secret, err := FetchSecret(kubeClient, namespace, name)
	if err != nil {
		return "", err
	}

	val, ok := secret.Data[key]
	if !ok {
		return "", errors.New("Secret key not found: " + key)
	}
	return string(val), nil
}
