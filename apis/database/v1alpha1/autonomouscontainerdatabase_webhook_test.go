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

var _ = Describe("test AutonomousContainerDatabase webhook", func() {
	Describe("Test ValidateUpdate of the AutonomousContainerDatabase validating webhook", func() {
		var (
			resourceName = "testacd"
			namespace    = "default"
			acdLookupKey = types.NamespacedName{Name: resourceName, Namespace: namespace}

			acd *AutonomousContainerDatabase

			timeout = time.Second * 5
		)

		BeforeEach(func() {
			acd = &AutonomousContainerDatabase{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousContainerDatabase",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousContainerDatabaseSpec{
					AutonomousContainerDatabaseOCID: common.String("fake-acd-ocid"),
					CompartmentOCID:                 common.String("fake-compartment-ocid"),
					DisplayName:                     common.String("fake-displayName"),
					AutonomousExadataVMClusterOCID:  common.String("fake-vmcluster-ocid"),
					PatchModel:                      database.AutonomousContainerDatabasePatchModelUpdates,
				},
			}

			specBytes, err := json.Marshal(acd.Spec)
			Expect(err).To(BeNil())

			anns := map[string]string{
				LastSuccessfulSpec: string(specBytes),
			}
			acd.SetAnnotations(anns)

			Expect(k8sClient.Create(context.TODO(), acd)).To(Succeed())

			// Change the lifecycleState to AVAILABLE
			acd.Status.LifecycleState = database.AutonomousContainerDatabaseLifecycleStateAvailable
			Expect(k8sClient.Status().Update(context.TODO(), acd)).To(Succeed())

			// Make sure the object is created
			Eventually(func() error {
				createdACD := &AutonomousContainerDatabase{}
				return k8sClient.Get(context.TODO(), acdLookupKey, createdACD)
			}, timeout).Should(BeNil())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), acd)).To(Succeed())
		})

		It("Cannot change the spec when the lifecycleState is in an intermdeiate state", func() {
			var errMsg string = "cannot change the spec when the lifecycleState is in an intermdeiate state"

			acd.Status.LifecycleState = database.AutonomousContainerDatabaseLifecycleStateProvisioning
			Expect(k8sClient.Status().Update(context.TODO(), acd)).To(Succeed())

			acd.Spec.DisplayName = common.String("modified-display-name")

			validateInvalidTest(acd, true, errMsg)
		})
	})
})
