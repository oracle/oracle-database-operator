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

package adbfamily

import (
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VerifyTargetADB searches if the target ADB is in the cluster, and set the owner reference to the ADB if it exists.
// The function returns two values in the following order:
// ocid: the OCID of the target ADB. An empty string is returned if the ocid is nil.
// ownerADB: the resource of the targetADB if it's found in the cluster
func VerifyTargetADB(kubeClient client.Client, target dbv1alpha1.TargetSpec, namespace string) (*dbv1alpha1.AutonomousDatabase, error) {
	var err error
	var ownerADB *dbv1alpha1.AutonomousDatabase

	// Get the target ADB OCID
	if target.K8sADB.Name != nil {
		// Find the target ADB using the name of the k8s ADB
		ownerADB = &dbv1alpha1.AutonomousDatabase{}
		if err := k8s.FetchResource(kubeClient, namespace, *target.K8sADB.Name, ownerADB); err != nil {
			return nil, err
		}

	} else {
		// Find the target ADB using the ADB OCID
		ownerADB, err = k8s.FetchAutonomousDatabaseWithOCID(kubeClient, namespace, *target.OCIADB.OCID)
		if err != nil {
			return nil, err
		}

	}

	return ownerADB, nil
}
