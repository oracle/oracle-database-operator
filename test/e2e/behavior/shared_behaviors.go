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

package e2ebehavior

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v45/common"
	"github.com/oracle/oci-go-sdk/v45/database"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/test/e2e/util"
)

/**************************************************************
* This file contains the global behaviors that share across the
* tests. Any values that will change during runtime should be
* passed as pointers otherwise the initial value will be passed
* to the function, which is likely to be nil or zero value.
**************************************************************/

func AssertProvision(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
		// Set the timeout to 15 minutes. The provision operation might take up to 10 minutes
		// if we have already send too many requests to OCI.
		provisionTimeout := time.Minute * 15
		provisionInterval := time.Second * 10

		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		// We'll need to retry until AutonomousDatabaseOCID populates in this newly created ADB, given that provisioning takes time to finish.
		By("Checking the AutonomousDatabaseOCID populates in the AutonomousDatabase resource")
		createdADB := &dbv1alpha1.AutonomousDatabase{}
		Eventually(func() (*string, error) {
			err := derefK8sClient.Get(context.TODO(), *adbLookupKey, createdADB)
			if err != nil {
				return nil, err
			}

			return createdADB.Spec.Details.AutonomousDatabaseOCID, nil
		}, provisionTimeout, provisionInterval).ShouldNot(BeNil())

		fmt.Fprintf(GinkgoWriter, "AutonomousDatabase DbName = %s, and AutonomousDatabaseOCID = %s\n",
			*createdADB.Spec.Details.DbName, *createdADB.Spec.Details.AutonomousDatabaseOCID)
	}
}

func AssertBind(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
		bindTimeout := time.Second * 30

		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		/*
			After creating this AutonomousDatabase resource, let's check that the resource fields match what we expect.
			Note that, because the OCI server may not have finished the bind request we call from earlier, we will use Gomega’s Eventually()
		*/

		// We'll need to retry until other information populates in this newly bound ADB.
		By("Checking the information populates in the AutonomousDatabase resource")
		boundADB := &dbv1alpha1.AutonomousDatabase{}
		Eventually(func() bool {
			err := derefK8sClient.Get(context.TODO(), *adbLookupKey, boundADB)
			if err != nil {
				return false
			}
			return (boundADB.Spec.Details.CompartmentOCID != nil &&
				boundADB.Spec.Details.DbWorkload != "" &&
				boundADB.Spec.Details.DbName != nil)
		}, bindTimeout).Should(Equal(true), "Attributes in the resource should not be empty")

		fmt.Fprintf(GinkgoWriter, "AutonomousDatabase DbName = %s, and AutonomousDatabaseOCID = %s\n",
			*boundADB.Spec.Details.DbName, *boundADB.Spec.Details.AutonomousDatabaseOCID)
	}
}

func AssertWallet(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
		walletTimeout := time.Second * 120

		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		instanceWallet := &corev1.Secret{}
		var walletName string

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())

		// The default name is xxx-instance-wallet
		if adb.Spec.Details.Wallet.Name == nil {
			walletName = adb.Name + "-instance-wallet"
		} else {
			walletName = *adb.Spec.Details.Wallet.Name
		}

		By("Checking the wallet secret " + walletName + " is created and is not empty")
		walletLookupKey := types.NamespacedName{Name: walletName, Namespace: adbLookupKey.Namespace}

		// We'll need to retry until wallet is downloaded
		Eventually(func() bool {
			err := derefK8sClient.Get(context.TODO(), walletLookupKey, instanceWallet)
			return err == nil
		}, walletTimeout).Should(Equal(true))

		Expect(len(instanceWallet.Data)).To(BeNumerically(">", 0))
	}
}

func CleanupDB(k8sClient *client.Client, dbClient *database.DatabaseClient, namespace string) func() {
	return func() {

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		adbList := &dbv1alpha1.AutonomousDatabaseList{}
		options := &client.ListOptions{
			Namespace: namespace,
		}
		derefK8sClient.List(context.TODO(), adbList, options)

		for _, adb := range adbList.Items {
			if adb.Spec.Details.AutonomousDatabaseOCID != nil {
				By("Terminating database " + *adb.Spec.Details.DbName)
				Expect(e2eutil.DeleteAutonomousDatabase(derefDBClient, adb.Spec.Details.AutonomousDatabaseOCID)).Should(Succeed())
			}
		}
	}
}

func compartInt(obj1 *int, obj2 *int) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compartBool(obj1 *bool, obj2 *bool) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compartString(obj1 *string, obj2 *string) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compartStringMap(obj1 map[string]string, obj2 map[string]string) bool {
	if len(obj1) != len(obj2) {
		return false
	}

	for k, v := range obj1 {
		w, ok := obj2[k]
		if !ok || v != w {
			return false
		}
	}

	return true
}

// AssertUpdate changes the displayName from "foo" to "foo_new", and scale the cpuCoreCount to 2
func AssertUpdate(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName) func() {
	return func() {
		// Considering that there are at most two update requests will be sent during the update
		// From the observation per request takes ~3mins to finish
		updateTimeout := time.Minute * 7
		updateInterval := time.Second * 20

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		expectedADB := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, expectedADB)).To(Succeed())

		By("Checking lifecycleState of the ADB is in AVAILABLE state before we do the update")
		// Send the List request here. Sometimes even if Get request shows that the DB is in AVAILABLE state
		// , the List request returns PROVISIONING state. In this case the update request will fail with
		// conflict state error.
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			listResp, err := e2eutil.ListAutonomousDatabases(derefDBClient, expectedADB.Spec.Details.CompartmentOCID, expectedADB.Spec.Details.DisplayName)
			if err != nil {
				return "", err
			}

			if len(listResp.Items) < 1 {
				return "", errors.New("should be at least 1 item in ListAutonomousDatabases response")
			}

			return database.AutonomousDatabaseLifecycleStateEnum(listResp.Items[0].LifecycleState), nil
		}, updateTimeout, updateInterval).Should(Equal(database.AutonomousDatabaseLifecycleStateAvailable))

		// Update
		var newDisplayName = *expectedADB.Spec.Details.DisplayName + "_new"
		var newCPUCoreCount = 2

		By(fmt.Sprintf("Updating the ADB with newDisplayName = %s and newCPUCoreCount = %d\n", newDisplayName, newCPUCoreCount))

		expectedADB.Spec.Details.DisplayName = common.String(newDisplayName)
		expectedADB.Spec.Details.CPUCoreCount = common.Int(newCPUCoreCount)

		Expect(derefK8sClient.Update(context.TODO(), expectedADB)).To(Succeed())

		Eventually(func() (bool, error) {
			// Get the current ADB resource and retry it's not in AVAILABLE state
			currentADB := &dbv1alpha1.AutonomousDatabase{}
			derefK8sClient.Get(context.TODO(), *adbLookupKey, currentADB)
			if currentADB.Spec.Details.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
				return false, nil
			}

			// Fetch the ADB from OCI when it's in AVAILABLE state, and retry if its attributes doesn't match the new ADB's attributes
			retryPolicy := e2eutil.NewLifecycleStateRetryPolicy(database.AutonomousDatabaseLifecycleStateAvailable)
			resp, err := e2eutil.GetAutonomousDatabase(derefDBClient, currentADB.Spec.Details.AutonomousDatabaseOCID, &retryPolicy)
			if err != nil {
				return false, err
			}

			adbDetails := currentADB.Spec.Details

			ociADB := currentADB
			ociADB = currentADB.UpdateAttrFromOCIAutonomousDatabase(resp.AutonomousDatabase)
			ociADBDetails := ociADB.Spec.Details

			// Compare
			same := compartString(adbDetails.AutonomousDatabaseOCID, ociADBDetails.AutonomousDatabaseOCID) &&
				compartString(adbDetails.CompartmentOCID, ociADBDetails.CompartmentOCID) &&
				compartString(adbDetails.DisplayName, ociADBDetails.DisplayName) &&
				compartString(adbDetails.DbName, ociADBDetails.DbName) &&
				adbDetails.DbWorkload == ociADBDetails.DbWorkload &&
				compartBool(adbDetails.IsDedicated, ociADBDetails.IsDedicated) &&
				compartString(adbDetails.DbVersion, ociADBDetails.DbVersion) &&
				compartInt(adbDetails.DataStorageSizeInTBs, ociADBDetails.DataStorageSizeInTBs) &&
				compartInt(adbDetails.CPUCoreCount, ociADBDetails.CPUCoreCount) &&
				compartBool(adbDetails.IsAutoScalingEnabled, ociADBDetails.IsAutoScalingEnabled) &&
				adbDetails.LifecycleState == ociADBDetails.LifecycleState &&
				compartStringMap(adbDetails.FreeformTags, ociADBDetails.FreeformTags) &&
				compartString(adbDetails.SubnetOCID, ociADBDetails.SubnetOCID) &&
				reflect.DeepEqual(adbDetails.NsgOCIDs, ociADBDetails.NsgOCIDs) &&
				compartString(adbDetails.PrivateEndpoint, ociADBDetails.PrivateEndpoint) &&
				compartString(adbDetails.PrivateEndpointLabel, ociADBDetails.PrivateEndpointLabel) &&
				compartString(adbDetails.PrivateEndpointIP, ociADBDetails.PrivateEndpointIP)

			return same, nil
		}, updateTimeout, updateInterval).Should(BeTrue())
	}
}

// Updates adb state and then asserts if change is propagated to OCI
func UpdateAndAssertState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		UpdateState(k8sClient, adbLookupKey, state)()
		AssertState(k8sClient, dbClient, adbLookupKey, state)()
	}
}

// Assert local and remote state
func AssertState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		// Waits longer for the local resource to reach the desired state
		AssertLocalState(k8sClient, adbLookupKey, state)()

		// Double-check the state of the DB in OCI so the timeout can be shorter
		AssertRemoteState(k8sClient, dbClient, adbLookupKey, state)()
	}
}

func AssertHardLinkDelete(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName) func() {
	return func() {
		changeStateTimeout := time.Second * 300

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())
		Expect(derefK8sClient.Delete(context.TODO(), adb)).To(Succeed())

		AssertSoftLinkDelete(k8sClient, adbLookupKey)()

		By("Checking if the ADB in OCI is in TERMINATING state")
		// Check every 10 secs for total 60 secs
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			retryPolicy := e2eutil.NewLifecycleStateRetryPolicy(database.AutonomousDatabaseLifecycleStateTerminating)
			return returnRemoteState(derefK8sClient, derefDBClient, adb.Spec.Details.AutonomousDatabaseOCID, &retryPolicy)
		}, changeStateTimeout).Should(Equal(database.AutonomousDatabaseLifecycleStateTerminating))
	}
}

func AssertSoftLinkDelete(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
		changeStateTimeout := time.Second * 300
		changeStateInterval := time.Second * 10

		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		existingAdb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, existingAdb)).To(Succeed())
		Expect(derefK8sClient.Delete(context.TODO(), existingAdb)).To(Succeed())

		By("Checking if the AutonomousDatabase resource is deleted")
		Eventually(func() (isDeleted bool) {
			adb := &dbv1alpha1.AutonomousDatabase{}
			isDeleted = false
			err := derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)
			if err != nil && k8sErrors.IsNotFound(err) {
				isDeleted = true
				return
			}
			return
		}, changeStateTimeout, changeStateInterval).Should(Equal(true))
	}
}

func AssertLocalState(k8sClient *client.Client, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		changeLocalStateTimeout := time.Second * 600

		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		By("Checking if the lifecycleState of local resource is " + string(state))
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			return returnLocalState(derefK8sClient, *adbLookupKey)
		}, changeLocalStateTimeout).Should(Equal(state))
	}
}

func AssertRemoteState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())

		AssertRemoteStateOCID(k8sClient, dbClient, adb.Spec.Details.AutonomousDatabaseOCID, state)()
	}
}

// Assert remote state using adb OCID
func AssertRemoteStateOCID(k8sClient *client.Client, dbClient *database.DatabaseClient, adbID *string, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		changeRemoteStateTimeout := time.Second * 300
		changeRemoteStateInterval := time.Second * 10

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbID).NotTo(BeNil())

		fmt.Fprintf(GinkgoWriter, "ADB ID is %s", *adbID)

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		By("Checking if the lifecycleState of the ADB in OCI is " + string(state))
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			return returnRemoteState(derefK8sClient, derefDBClient, adbID, nil)
		}, changeRemoteStateTimeout, changeRemoteStateInterval).Should(Equal(state))
	}
}

// Updates state from local resource and OCI
func UpdateState(k8sClient *client.Client, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())

		adb.Spec.Details.LifecycleState = state
		By("Updating adb state to " + string(state))
		Expect(derefK8sClient.Update(context.TODO(), adb)).To(Succeed())
	}
}

func returnLocalState(k8sClient client.Client, adbLookupKey types.NamespacedName) (database.AutonomousDatabaseLifecycleStateEnum, error) {
	adb := &dbv1alpha1.AutonomousDatabase{}
	err := k8sClient.Get(context.TODO(), adbLookupKey, adb)
	if err != nil {
		return "", err
	}
	return adb.Status.LifecycleState, nil
}

func returnRemoteState(k8sClient client.Client, dbClient database.DatabaseClient, adbID *string, retryPolicy *common.RetryPolicy) (database.AutonomousDatabaseLifecycleStateEnum, error) {
	resp, err := e2eutil.GetAutonomousDatabase(dbClient, adbID, retryPolicy)
	if err != nil {
		return "", err
	}
	return resp.LifecycleState, nil
}
