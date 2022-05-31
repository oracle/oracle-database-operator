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

package e2etest

import (
	"context"
	"time"

	"github.com/oracle/oci-go-sdk/v64/common"
	"github.com/oracle/oci-go-sdk/v64/database"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/test/e2e/behavior"
	"github.com/oracle/oracle-database-operator/test/e2e/util"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("test ACD binding", func() {
	var acdLookupKey types.NamespacedName
	var acdID string

	AfterEach(func() {
		// IMPORTANT: The operator might have to call reconcile multiple times to finish an operation.
		// If we do the update immediately, the previous reconciliation will overwrite the changes.
		By("Sleeping 20 seconds to wait for reconciliation to finish")
		time.Sleep(time.Second * 20)
	})

	Describe("ACD Provisioning", func() {
		It("Should create an AutonomousContainerDatabase resource and in OCI", func() {
			provisionAcd := &dbv1alpha1.AutonomousContainerDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousContainerDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provisionacd",
					Namespace: ADBNamespace,
				},
				Spec: dbv1alpha1.AutonomousContainerDatabaseSpec{
					DisplayName:                    common.String(e2eutil.GenerateACDName()),
					CompartmentOCID:                common.String(SharedCompartmentOCID),
					AutonomousExadataVMClusterOCID: common.String(SharedExadataVMClusterOCID),
					PatchModel:                     database.AutonomousContainerDatabasePatchModelUpdates,
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			acdLookupKey = types.NamespacedName{Name: provisionAcd.Name, Namespace: provisionAcd.Namespace}

			Expect(k8sClient.Create(context.TODO(), provisionAcd)).Should(Succeed())
		})

		It("Should check ACD status is BACKUP IN PROGRESS", e2ebehavior.AssertACDState(&k8sClient, &dbClient, &acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateBackupInProgress, time.Minute*35))

		It("Should check ACD status is AVAILABLE", e2ebehavior.AssertACDState(&k8sClient, &dbClient, &acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateAvailable, time.Minute*60))

		It("Should save ACD ocid for next test", func() {
			acd := &dbv1alpha1.AutonomousContainerDatabase{}
			Expect(k8sClient.Get(context.TODO(), acdLookupKey, acd)).To(Succeed())
			acdID = *acd.Spec.AutonomousContainerDatabaseOCID
		})

		It("Should delete ACD local resource", e2ebehavior.AssertACDLocalDelete(&k8sClient, &dbClient, &acdLookupKey))
	})

	Describe("ACD Binding", func() {
		It("Should create an AutonomousContainerDatabase resource", func() {
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
					AutonomousContainerDatabaseOCID: common.String(acdID),
					OCIConfig: dbv1alpha1.OCIConfigSpec{
						ConfigMapName: common.String(SharedOCIConfigMapName),
						SecretName:    common.String(SharedOCISecretName),
					},
				},
			}

			acdLookupKey = types.NamespacedName{Name: acd.Name, Namespace: acd.Namespace}

			Expect(k8sClient.Create(context.TODO(), acd)).Should(Succeed())
		})

		It("Should bind to an ACD", e2ebehavior.AssertACDBind(&k8sClient, &dbClient, &acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateAvailable))

		It("Should update the ACD", e2ebehavior.UpdateAndAssertACDSpec(&k8sClient, &dbClient, &acdLookupKey))

		It("Should restart the ACD", e2ebehavior.AssertACDRestart(&k8sClient, &dbClient, &acdLookupKey))

		It("Should terminate the ACD", e2ebehavior.AssertACDTerminate(&k8sClient, &dbClient, &acdLookupKey))
	})
})
