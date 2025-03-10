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
	"encoding/json"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("test AutonomousDatabase webhook", func() {
	Describe("Test ValidateCreate of the AutonomousDatabase validating webhook", func() {
		var (
			resourceName = "testadb"
			namespace    = "default"

			adb *AutonomousDatabase
		)

		BeforeEach(func() {
			adb = &AutonomousDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousDatabaseSpec{
					Details: AutonomousDatabaseDetails{
						AutonomousDatabaseBase: AutonomousDatabaseBase{
							CompartmentId: common.String("fake-compartment-ocid"),
							DbName:        common.String("fake-dbName"),
							DisplayName:   common.String("fake-displayName"),
							CpuCoreCount:  common.Int(1),
							AdminPassword: PasswordSpec{
								K8sSecret: K8sSecretSpec{
									Name: common.String("fake-admin-password"),
								},
							},
							DataStorageSizeInTBs: common.Int(1),
						},
					},
				},
			}
		})

		// Common validation
		It("Should not apply values to adminPassword.k8sSecret and adminPassword.ociSecret at the same time", func() {
			var errMsg string = "cannot apply k8sSecret.name and ociSecret.ocid at the same time"

			adb.Spec.Details.AdminPassword.K8sSecret.Name = common.String("test-admin-password")
			adb.Spec.Details.AdminPassword.OciSecret.Id = common.String("fake.ocid1.vaultsecret.oc1...")

			validateInvalidTest(adb, false, errMsg)
		})

		It("Should not apply values to wallet.password.k8sSecret and wallet.password.ociSecret at the same time", func() {
			var errMsg string = "cannot apply k8sSecret.name and ociSecret.ocid at the same time"

			adb.Spec.Wallet.Password.K8sSecret.Name = common.String("test-wallet-password")
			adb.Spec.Wallet.Password.OciSecret.Id = common.String("fake.ocid1.vaultsecret.oc1...")

			validateInvalidTest(adb, false, errMsg)
		})

		Context("Dedicated Autonomous Database", func() {
			BeforeEach(func() {
				adb.Spec.Details.AutonomousContainerDatabase.K8sAcd.Name = common.String("testACD")
				adb.Spec.Details.AutonomousContainerDatabase.OciAcd.Id = common.String("fake-acd-ocid")
			})

			It("AccessControlList cannot be empty when the network access type is RESTRICTED", func() {
				var errMsg string = "access control list cannot be provided when Autonomous Database's access control is disabled"

				adb.Spec.Details.IsAccessControlEnabled = common.Bool(false)
				adb.Spec.Details.WhitelistedIps = []string{"192.168.1.1"}

				validateInvalidTest(adb, false, errMsg)
			})

			It("AccessControlList cannot be empty when the network access type is RESTRICTED", func() {
				var errMsg string = "isMTLSConnectionRequired is not supported on a dedicated database"

				adb.Spec.Details.IsMtlsConnectionRequired = common.Bool(true)

				validateInvalidTest(adb, false, errMsg)
			})

		})
	})

	// Skip the common and network validations since they're already verified in the test for ValidateCreate
	Describe("Test ValidateUpdate of the AutonomousDatabase validating webhook", func() {
		var (
			resourceName = "testadb"
			namespace    = "default"
			adbLookupKey = types.NamespacedName{Name: resourceName, Namespace: namespace}

			adb *AutonomousDatabase

			timeout = time.Second * 5
		)

		BeforeEach(func() {
			adb = &AutonomousDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousDatabaseSpec{
					Action: "Create",
					Details: AutonomousDatabaseDetails{
						Id: common.String("fake-adb-ocid"),
						AutonomousDatabaseBase: AutonomousDatabaseBase{
							CompartmentId:        common.String("fake-compartment-ocid"),
							DbName:               common.String("fake-dbName"),
							DisplayName:          common.String("fake-displayName"),
							CpuCoreCount:         common.Int(1),
							DataStorageSizeInTBs: common.Int(1),
						},
					},
				},
			}

			specBytes, err := json.Marshal(adb.Spec)
			Expect(err).To(BeNil())

			anns := map[string]string{
				LastSuccessfulSpec: string(specBytes),
			}
			adb.SetAnnotations(anns)

			Expect(k8sClient.Create(context.TODO(), adb)).To(Succeed())

			// Change the lifecycleState to AVAILABLE
			adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateAvailable
			Expect(k8sClient.Status().Update(context.TODO(), adb)).To(Succeed())

			// Make sure the object is created
			Eventually(func() error {
				createdADB := &AutonomousDatabase{}
				return k8sClient.Get(context.TODO(), adbLookupKey, createdADB)
			}, timeout).Should(BeNil())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), adb)).To(Succeed())
		})

		It("Cannot change the spec when the lifecycleState is in an intermdeiate state", func() {
			var errMsg string = "cannot change the spec when the lifecycleState is in an intermdeiate state"

			adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUpdating
			Expect(k8sClient.Status().Update(context.TODO(), adb)).To(Succeed())

			adb.Spec.Details.DbName = common.String("modified-db-name")

			validateInvalidTest(adb, true, errMsg)
		})

		It("AutonomousDatabaseOCID cannot be modified", func() {
			var errMsg string = "autonomousDatabaseOCID cannot be modified"

			adb.Spec.Details.Id = common.String("modified-adb-ocid")

			validateInvalidTest(adb, true, errMsg)
		})
	})
})
