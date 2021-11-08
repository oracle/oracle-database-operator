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

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

func newOwnerReference(owner client.Object) []metav1.OwnerReference {
	ownerRef := []metav1.OwnerReference{
		{
			Kind:       owner.GetObjectKind().GroupVersionKind().Kind,
			APIVersion: owner.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Name:       owner.GetName(),
			UID:        owner.GetUID(),
		},
	}
	return ownerRef
}

// func newOwnerReference(adb *dbv1alpha1.AutonomousDatabase) []metav1.OwnerReference {
// 	ownerRef := []metav1.OwnerReference{
// 		{
// 			Kind:       adb.GroupVersionKind().Kind,
// 			APIVersion: adb.APIVersion,
// 			Name:       adb.Name,
// 			UID:        types.UID(adb.UID),
// 		},
// 	}
// 	return ownerRef
// }

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

func fetchAutonomousDatabaseBackups(kubeClient client.Client, namespace string) (*dbv1alpha1.AutonomousDatabaseBackupList, error) {
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
