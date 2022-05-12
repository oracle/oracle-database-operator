/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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
package v1alpha1

import (
	"context"
	"time"

	"github.com/oracle/oci-go-sdk/v63/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("test AutonomousDatabaseBackup webhook", func() {
	Describe("Test ValidateCreate of the AutonomousDatabaseBackup validating webhook", func() {
		var (
			resourceName = "testadbbackup"
			namespace    = "default"

			backup *AutonomousDatabaseBackup
		)

		BeforeEach(func() {
			backup = &AutonomousDatabaseBackup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabaseBackup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousDatabaseBackupSpec{
					Target: TargetSpec{},
				},
			}
		})

		It("Should specify at least one of the k8sADB and ociADB", func() {
			var errMsg string = "target ADB is empty"

			backup.Spec.Target.K8sADB.Name = nil
			backup.Spec.Target.OCIADB.OCID = nil

			validateInvalidTest(backup, false, errMsg)
		})

		It("Should specify either k8sADB or ociADB, but not both", func() {
			var errMsg string = "specify either k8sADB or ociADB, but not both"

			backup.Spec.Target.K8sADB.Name = common.String("fake-target-adb")
			backup.Spec.Target.OCIADB.OCID = common.String("fake.ocid1.autonomousdatabase.oc1...")

			validateInvalidTest(backup, false, errMsg)
		})
	})

	Describe("Test ValidateUpdate of the AutonomousDatabaseBackup validating webhook", func() {
		var (
			resourceName    = "testadbbackup"
			namespace       = "default"
			backupLookupKey = types.NamespacedName{Name: resourceName, Namespace: namespace}

			backup *AutonomousDatabaseBackup

			timeout = time.Second * 5
		)

		BeforeEach(func() {
			backup = &AutonomousDatabaseBackup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabaseBackup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousDatabaseBackupSpec{
					AutonomousDatabaseBackupOCID: common.String("fake.ocid1.autonomousdatabasebackup.oc1..."),
					DisplayName:                  common.String("fake-displayName"),
				},
			}
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(context.TODO(), backup)).To(Succeed())

			// Make sure the object is created
			Eventually(func() error {
				createdBackup := &AutonomousDatabaseBackup{}
				return k8sClient.Get(context.TODO(), backupLookupKey, createdBackup)
			}, timeout).Should(BeNil())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), backup)).To(Succeed())
		})

		Context("The bakcup is using target.k8sADB.name", func() {
			BeforeEach(func() {
				backup.Spec.Target.K8sADB.Name = common.String("fake-target-adb")
			})

			It("Cannot assign a new name to the target", func() {
				var errMsg string = "cannot assign a new name to the target"

				backup.Spec.Target.K8sADB.Name = common.String("modified-target-adb")

				validateInvalidTest(backup, true, errMsg)
			})

			It("Cannot assign a new displayName to this backup", func() {
				var errMsg string = "cannot assign a new displayName to this backup"

				backup.Spec.DisplayName = common.String("modified-displayName")

				validateInvalidTest(backup, true, errMsg)
			})
		})

		Context("The bakcup is using target.ociADB.ocid", func() {
			BeforeEach(func() {
				backup.Spec.Target.OCIADB.OCID = common.String("fake.ocid1.autonomousdatabase.oc1...")
			})

			It("Cannot assign a new ocid to the target", func() {
				var errMsg string = "cannot assign a new ocid to the target"

				backup.Spec.Target.OCIADB.OCID = common.String("modified.ocid1.autonomousdatabase.oc1...")

				validateInvalidTest(backup, true, errMsg)
			})
		})
	})
})
