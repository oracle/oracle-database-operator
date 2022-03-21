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
	"fmt"
	"time"

	"github.com/oracle/oci-go-sdk/v54/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("test ACD binding with hardLink=true", func() {
	// var adbLookupKey types.NamespacedName
	const downloadedWallet = "instance-wallet-secret-1"
	var acdID *string

	AfterEach(func() {
		// IMPORTANT: The operator might have to call reconcile multiple times to finish an operation.
		// If we do the update immediately, the previous reconciliation will overwrite the changes.
		By("Sleeping 20 seconds to wait for reconciliation to finish")
		time.Sleep(time.Second * 20)
	})

	It("should init the test", func() {
		By("Save the database ID for later use")
		acdID = common.String("ocid1.autonomouscontainerdatabase.oc1.phx.anyhqljsfj4qgxaah6iftchkd3i6co6bqudtzvrfuumkbpxcq6gcw5tjodea")
	})

	Describe("ACD binding with HardLink = false", func() {
		It("Should create a AutonomousContainerDatabase resource with HardLink = false", func() {
			acd := &dbv1alpha1.AutonomousContainerDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousContainerDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bindacd",
					Namespace: ADBNamespace,
				},
				Spec: dbv1alpha1.AutonomousContainerDatabaseSpec{
					AutonomousContainerDatabaseOCID: acdID,
					HardLink:                        common.Bool(false),
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			// adbLookupKey = types.NamespacedName{Name: adb.Name, Namespace: adb.Namespace}

			Expect(k8sClient.Create(context.TODO(), acd)).Should(Succeed())
		})

		It("Should update the resource", func() {
			acd := &dbv1alpha1.AutonomousContainerDatabase{}

			acdLookupKey := types.NamespacedName{Name: "bindacd", Namespace: ADBNamespace}

			Expect(k8sClient.Get(context.TODO(), acdLookupKey, acd)).Should(Succeed())

			newDisplayName := "tinglwanACD"
			acd.Spec.DisplayName = common.String(newDisplayName)
			Expect(k8sClient.Update(context.TODO(), acd)).Should(Succeed())

			Eventually(func() (string, error) {
				createdACD := &dbv1alpha1.AutonomousContainerDatabase{}
				err := k8sClient.Get(context.TODO(), acdLookupKey, createdACD)
				if err != nil {
					return "", err
				}

				fmt.Println("============ test: displayName = " + string(*createdACD.Spec.DisplayName))

				return *createdACD.Spec.DisplayName, nil
			}, time.Second*10, time.Second*5).Should(Equal(newDisplayName))
		})
	})
})
