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

package e2etest

import (
	"context"
	"time"

	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/test/e2e/behavior"
	"github.com/oracle/oracle-database-operator/test/e2e/util"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("test ADB provisioning", func() {
	AfterEach(func() {
		// IMPORTANT: The operator might have to call reconcile multiple times to finish an operation.
		// If we do the update immediately, the previous reconciliation will overwrite the changes.
		By("Sleeping 20 seconds to wait for reconciliation to finish")
		time.Sleep(time.Second * 20)
	})

	Describe("Using adminPasswordSecret and hardLink=true", func() {
		var dbName string
		const downloadedWallet = "instance-wallet-secret-1"

		const resourceName = "createadb1"
		duplicateAdbResourceName := "duplicateadb"

		var adbLookupKey = types.NamespacedName{Name: resourceName, Namespace: ADBNamespace}
		var dupAdbLookupKey = types.NamespacedName{Name: duplicateAdbResourceName, Namespace: ADBNamespace}

		It("Should create a AutonomousDatabase resource", func() {
			dbName = e2eutil.GenerateDBName()
			adb := &dbv1alpha1.AutonomousDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: ADBNamespace,
				},
				Spec: dbv1alpha1.AutonomousDatabaseSpec{
					Details: dbv1alpha1.AutonomousDatabaseDetails{
						CompartmentOCID: common.String(SharedCompartmentOCID),
						DbName:          common.String(dbName),
						DisplayName:     common.String(dbName),
						CPUCoreCount:    common.Int(1),
						AdminPassword: dbv1alpha1.PasswordSpec{
							K8sSecretName: common.String(SharedAdminPassSecretName),
						},
						DataStorageSizeInTBs: common.Int(1),
						IsAutoScalingEnabled: common.Bool(true),
						Wallet: dbv1alpha1.WalletSpec{
							Name: common.String(downloadedWallet),
							Password: dbv1alpha1.PasswordSpec{
								K8sSecretName: common.String(SharedWalletPassSecretName),
							},
						},
					},
					HardLink: common.Bool(true),
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), adb)).To(Succeed())
		})

		It("Should provision ADB using the admin password from K8s Secret "+SharedAdminPassSecretName, e2ebehavior.AssertProvision(&k8sClient, &adbLookupKey))

		It("Should try to provision ADB with duplicate db name", func() {
			duplicateAdb := &dbv1alpha1.AutonomousDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      duplicateAdbResourceName,
					Namespace: ADBNamespace,
				},
				Spec: dbv1alpha1.AutonomousDatabaseSpec{
					Details: dbv1alpha1.AutonomousDatabaseDetails{
						CompartmentOCID: common.String(SharedCompartmentOCID),
						DbName:          common.String(dbName),
						DisplayName:     common.String(dbName),
						CPUCoreCount:    common.Int(1),
						AdminPassword: dbv1alpha1.PasswordSpec{
							K8sSecretName: common.String(SharedAdminPassSecretName),
						},
						DataStorageSizeInTBs: common.Int(1),
						IsAutoScalingEnabled: common.Bool(true),
					},
					HardLink: common.Bool(true),
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), duplicateAdb)).To(Succeed())
		})

		It("Should check for local resource state UNAVAILABLE", e2ebehavior.AssertLocalState(&k8sClient, &dupAdbLookupKey, database.AutonomousDatabaseLifecycleStateUnavailable))

		It("Should download an instance wallet using the password from K8s Secret "+SharedWalletPassSecretName, e2ebehavior.AssertWallet(&k8sClient, &adbLookupKey))

		It("Should delete the resource in cluster and terminate the database in OCI", e2ebehavior.AssertHardLinkDelete(&k8sClient, &dbClient, &adbLookupKey))
	})

	Describe("Using adminPasswordOCID and hardLink=true", func() {
		var dbName string
		const downloadedWallet = "instance-wallet-secret-2"

		const resourceName = "createadb2"
		var adbLookupKey = types.NamespacedName{Name: resourceName, Namespace: ADBNamespace}

		It("Should create a AutonomousDatabase resource", func() {
			dbName = e2eutil.GenerateDBName()
			adb := &dbv1alpha1.AutonomousDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: ADBNamespace,
				},
				Spec: dbv1alpha1.AutonomousDatabaseSpec{
					Details: dbv1alpha1.AutonomousDatabaseDetails{
						CompartmentOCID: common.String(SharedCompartmentOCID),
						DbName:          common.String(dbName),
						DisplayName:     common.String(dbName),
						CPUCoreCount:    common.Int(1),
						AdminPassword: dbv1alpha1.PasswordSpec{
							OCISecretOCID: common.String(SharedAdminPasswordOCID),
						},
						DataStorageSizeInTBs: common.Int(1),
						IsAutoScalingEnabled: common.Bool(true),

						Wallet: dbv1alpha1.WalletSpec{
							Name: common.String(downloadedWallet),
							Password: dbv1alpha1.PasswordSpec{
								OCISecretOCID: common.String(SharedInstanceWalletPasswordOCID),
							},
						},
					},
					HardLink: common.Bool(true),
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), adb)).To(Succeed())
		})

		It("Should provision ADB using the password from OCI Secret OCID "+SharedAdminPasswordOCID, e2ebehavior.AssertProvision(&k8sClient, &adbLookupKey))

		It("Should download an instance wallet using the password from OCI Secret OCID "+SharedInstanceWalletPasswordOCID, e2ebehavior.AssertWallet(&k8sClient, &adbLookupKey))

		It("Should delete the resource in cluster and terminate the database in OCI", e2ebehavior.AssertHardLinkDelete(&k8sClient, &dbClient, &adbLookupKey))
	})
})
