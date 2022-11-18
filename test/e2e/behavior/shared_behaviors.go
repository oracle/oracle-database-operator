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

package e2ebehavior

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/workrequests"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/test/e2e/util"
	"os"
	"os/exec"
	"strings"
)

/**************************************************************
* This file contains the global behaviors that share across the
* tests. Any values that will change during runtime should be
* passed as pointers otherwise the initial value will be passed
* to the function, which is likely to be nil or zero value.
**************************************************************/

var (
	Describe                = ginkgo.Describe
	By                      = ginkgo.By
	GinkgoWriter            = ginkgo.GinkgoWriter
	Expect                  = gomega.Expect
	BeNil                   = gomega.BeNil
	Eventually              = gomega.Eventually
	Equal                   = gomega.Equal
	Succeed                 = gomega.Succeed
	HaveOccurred            = gomega.HaveOccurred
	BeNumerically           = gomega.BeNumerically
	BeTrue                  = gomega.BeTrue
	changeTimeout           = time.Second * 300
	provisionTimeout        = time.Second * 15
	bindTimeout             = time.Second * 30
	backupTimeout           = time.Minute * 20
	intervalTime            = time.Second * 10
	updateADBTimeout        = time.Minute * 7
	changeLocalStateTimeout = time.Second * 600
	updateACDTimeout        = time.Minute * 3
)

func AssertProvision(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
		// Set provisionTimeout to 15 minutes. The provision operation might take up to 10 minutes
		// if we have already send too many requests to OCI.

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
		}, provisionTimeout, intervalTime).ShouldNot(BeNil())

		fmt.Fprintf(GinkgoWriter, "AutonomousDatabase DbName = %s, and AutonomousDatabaseOCID = %s\n",
			*createdADB.Spec.Details.DbName, *createdADB.Spec.Details.AutonomousDatabaseOCID)
	}
}

func AssertBind(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
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

func compareInt(obj1 *int, obj2 *int) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compareBool(obj1 *bool, obj2 *bool) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compareString(obj1 *string, obj2 *string) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 != nil && obj2 == nil) || (obj1 == nil && obj2 != nil) {
		return false
	}
	return *obj1 == *obj2
}

func compareStringMap(obj1 map[string]string, obj2 map[string]string) bool {
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

// UpdateDetails updates spec.details from local resource and OCI
func UpdateDetails(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, newSecretName string, newAdminPassword *string) func() *dbv1alpha1.AutonomousDatabase {
	return func() *dbv1alpha1.AutonomousDatabase {
		// Considering that there are at most two update requests will be sent during the update
		// From the observation per request takes ~3mins to finish

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())
		Expect(newAdminPassword).NotTo(BeNil())

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
		}, updateADBTimeout, intervalTime).Should(Equal(database.AutonomousDatabaseLifecycleStateAvailable))

		// Update
		var newDisplayName = *expectedADB.Spec.Details.DisplayName + "_new"

		var newCPUCoreCount int
		if *expectedADB.Spec.Details.CPUCoreCount == 1 {
			newCPUCoreCount = 2
		} else {
			newCPUCoreCount = 1
		}

		var newKey = "testKey"
		var newVal = "testVal"

		By(fmt.Sprintf("Updating the ADB with newDisplayName = %s, newCPUCoreCount = %d and newFreeformTag = %s:%s\n",
			newDisplayName, newCPUCoreCount, newKey, newVal))

		expectedADB.Spec.Details.DisplayName = common.String(newDisplayName)
		expectedADB.Spec.Details.CPUCoreCount = common.Int(newCPUCoreCount)
		expectedADB.Spec.Details.FreeformTags = map[string]string{newKey: newVal}
		expectedADB.Spec.Details.AdminPassword.K8sSecret.Name = common.String(newSecretName)

		Expect(derefK8sClient.Update(context.TODO(), expectedADB)).To(Succeed())

		return expectedADB
	}
}

// AssertADBDetails asserts the changes in spec.details
func AssertADBDetails(k8sClient *client.Client,
	dbClient *database.DatabaseClient,
	adbLookupKey *types.NamespacedName,
	expectedADB *dbv1alpha1.AutonomousDatabase) func() {
	return func() {
		// Considering that there are at most two update requests will be sent during the update
		// From the observation per request takes ~3mins to finish

		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefDBClient := *dbClient

		expectedADBDetails := expectedADB.Spec.Details
		Eventually(func() (bool, error) {
			// Fetch the ADB from OCI when it's in AVAILABLE state, and retry if its attributes doesn't match the new ADB's attributes
			retryPolicy := e2eutil.NewLifecycleStateRetryPolicyADB(database.AutonomousDatabaseLifecycleStateAvailable)
			resp, err := e2eutil.GetAutonomousDatabase(derefDBClient, expectedADB.Spec.Details.AutonomousDatabaseOCID, &retryPolicy)
			if err != nil {
				return false, err
			}

			debug := false
			if debug {
				if !compareString(expectedADBDetails.AutonomousDatabaseOCID, resp.AutonomousDatabase.Id) {
					fmt.Fprintf(GinkgoWriter, "Expected OCID: %v\nGot: %v\n", expectedADBDetails.AutonomousDatabaseOCID, resp.AutonomousDatabase.Id)
				}
				if !compareString(expectedADBDetails.CompartmentOCID, resp.AutonomousDatabase.CompartmentId) {
					fmt.Fprintf(GinkgoWriter, "Expected CompartmentOCID: %v\nGot: %v\n", expectedADBDetails.CompartmentOCID, resp.CompartmentId)
				}
				if !compareString(expectedADBDetails.DisplayName, resp.AutonomousDatabase.DisplayName) {
					fmt.Fprintf(GinkgoWriter, "Expected DisplayName: %v\nGot: %v\n", expectedADBDetails.DisplayName, resp.AutonomousDatabase.DisplayName)
				}
				if !compareString(expectedADBDetails.DbName, resp.AutonomousDatabase.DbName) {
					fmt.Fprintf(GinkgoWriter, "Expected DbName: %v\nGot:%v\n", expectedADBDetails.DbName, resp.AutonomousDatabase.DbName)
				}
				if expectedADBDetails.DbWorkload != resp.AutonomousDatabase.DbWorkload {
					fmt.Fprintf(GinkgoWriter, "Expected DbWorkload: %v\nGot: %v\n", expectedADBDetails.DbWorkload, resp.AutonomousDatabase.DbWorkload)
				}
				if !compareBool(expectedADBDetails.IsDedicated, resp.AutonomousDatabase.IsDedicated) {
					fmt.Fprintf(GinkgoWriter, "Expected IsDedicated: %v\nGot: %v\n", expectedADBDetails.IsDedicated, resp.AutonomousDatabase.IsDedicated)
				}
				if !compareString(expectedADBDetails.DbVersion, resp.AutonomousDatabase.DbVersion) {
					fmt.Fprintf(GinkgoWriter, "Expected DbVersion: %v\nGot: %v\n", expectedADBDetails.DbVersion, resp.AutonomousDatabase.DbVersion)
				}
				if !compareInt(expectedADBDetails.DataStorageSizeInTBs, resp.AutonomousDatabase.DataStorageSizeInTBs) {
					fmt.Fprintf(GinkgoWriter, "Expected DataStorageSize: %v\nGot: %v\n", expectedADBDetails.DataStorageSizeInTBs, resp.AutonomousDatabase.DataStorageSizeInTBs)
				}
				if !compareInt(expectedADBDetails.CPUCoreCount, resp.AutonomousDatabase.CpuCoreCount) {
					fmt.Fprintf(GinkgoWriter, "Expected CPUCoreCount: %v\nGot: %v\n", expectedADBDetails.CPUCoreCount, resp.AutonomousDatabase.CpuCoreCount)
				}
				if !compareBool(expectedADBDetails.IsAutoScalingEnabled, resp.AutonomousDatabase.IsAutoScalingEnabled) {
					fmt.Fprintf(GinkgoWriter, "Expected IsAutoScalingEnabled: %v\nGot: %v\n", expectedADBDetails.IsAutoScalingEnabled, resp.AutonomousDatabase.IsAutoScalingEnabled)
				}
				if !compareStringMap(expectedADBDetails.FreeformTags, resp.AutonomousDatabase.FreeformTags) {
					fmt.Fprintf(GinkgoWriter, "Expected FreeformTags: %v\nGot: %v\n", expectedADBDetails.FreeformTags, resp.AutonomousDatabase.FreeformTags)
				}
				if !compareBool(expectedADBDetails.NetworkAccess.IsAccessControlEnabled, resp.AutonomousDatabase.IsAccessControlEnabled) {
					fmt.Fprintf(GinkgoWriter, "Expected IsAccessControlEnabled: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.IsAccessControlEnabled, resp.AutonomousDatabase.IsAccessControlEnabled)
				}
				if !reflect.DeepEqual(expectedADBDetails.NetworkAccess.AccessControlList, resp.AutonomousDatabase.WhitelistedIps) {
					fmt.Fprintf(GinkgoWriter, "Expected AccessControlList: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.AccessControlList, resp.AutonomousDatabase.WhitelistedIps)
				}
				if !compareBool(expectedADBDetails.NetworkAccess.IsMTLSConnectionRequired, resp.AutonomousDatabase.IsMtlsConnectionRequired) {
					fmt.Fprintf(GinkgoWriter, "Expected IsMTLSConnectionRequired: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.IsMTLSConnectionRequired, resp.AutonomousDatabase.IsMtlsConnectionRequired)
				}
				if !compareString(expectedADBDetails.NetworkAccess.PrivateEndpoint.SubnetOCID, resp.AutonomousDatabase.SubnetId) {
					fmt.Fprintf(GinkgoWriter, "Expected SubnetOCID: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.PrivateEndpoint.SubnetOCID, resp.AutonomousDatabase.SubnetId)
				}
				if !reflect.DeepEqual(expectedADBDetails.NetworkAccess.PrivateEndpoint.NsgOCIDs, resp.AutonomousDatabase.NsgIds) {
					fmt.Fprintf(GinkgoWriter, "Expected NsgOCIDs: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.PrivateEndpoint.NsgOCIDs, resp.AutonomousDatabase.NsgIds)
				}
				if !compareString(expectedADBDetails.NetworkAccess.PrivateEndpoint.HostnamePrefix, resp.AutonomousDatabase.PrivateEndpointLabel) {
					fmt.Fprintf(GinkgoWriter, "Expected HostnamePrefix: %v\nGot: %v\n", expectedADBDetails.NetworkAccess.PrivateEndpoint.HostnamePrefix, resp.AutonomousDatabase.PrivateEndpointLabel)
				}
			}

			// Compare the elements one by one rather than doing reflect.DeelEqual(adb1, adb2), since some parameters
			// (e.g. adminPassword, wallet) are missing from e2eutil.GetAutonomousDatabase().
			// We don't compare LifecycleState in this case. We only make sure that the ADB is in AVAIABLE state before
			// proceeding to the next test.
			same := compareString(expectedADBDetails.AutonomousDatabaseOCID, resp.AutonomousDatabase.Id) &&
				compareString(expectedADBDetails.CompartmentOCID, resp.AutonomousDatabase.CompartmentId) &&
				compareString(expectedADBDetails.DisplayName, resp.AutonomousDatabase.DisplayName) &&
				compareString(expectedADBDetails.DbName, resp.AutonomousDatabase.DbName) &&
				expectedADBDetails.DbWorkload == resp.AutonomousDatabase.DbWorkload &&
				compareBool(expectedADBDetails.IsDedicated, resp.AutonomousDatabase.IsDedicated) &&
				compareString(expectedADBDetails.DbVersion, resp.AutonomousDatabase.DbVersion) &&
				compareInt(expectedADBDetails.DataStorageSizeInTBs, resp.AutonomousDatabase.DataStorageSizeInTBs) &&
				compareInt(expectedADBDetails.CPUCoreCount, resp.AutonomousDatabase.CpuCoreCount) &&
				compareBool(expectedADBDetails.IsAutoScalingEnabled, resp.AutonomousDatabase.IsAutoScalingEnabled) &&
				compareStringMap(expectedADBDetails.FreeformTags, resp.AutonomousDatabase.FreeformTags) &&
				compareBool(expectedADBDetails.NetworkAccess.IsAccessControlEnabled, resp.AutonomousDatabase.IsAccessControlEnabled) &&
				reflect.DeepEqual(expectedADBDetails.NetworkAccess.AccessControlList, resp.AutonomousDatabase.WhitelistedIps) &&
				compareBool(expectedADBDetails.NetworkAccess.IsMTLSConnectionRequired, resp.AutonomousDatabase.IsMtlsConnectionRequired) &&
				compareString(expectedADBDetails.NetworkAccess.PrivateEndpoint.SubnetOCID, resp.AutonomousDatabase.SubnetId) &&
				reflect.DeepEqual(expectedADBDetails.NetworkAccess.PrivateEndpoint.NsgOCIDs, resp.AutonomousDatabase.NsgIds) &&
				compareString(expectedADBDetails.NetworkAccess.PrivateEndpoint.HostnamePrefix, resp.AutonomousDatabase.PrivateEndpointLabel)

			return same, nil
		}, updateADBTimeout, intervalTime).Should(BeTrue())

		// IMPORTANT: make sure the local resource has finished reconciling, otherwise the changes will
		// be conflicted with the next test and cause unknow result.
		AssertADBLocalState(k8sClient, adbLookupKey, database.AutonomousDatabaseLifecycleStateAvailable)()
	}
}

func TestNetworkAccessRestricted(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, isMTLSConnectionRequired bool) func() {
	return func() {
		networkRestrictedSpec := dbv1alpha1.NetworkAccessSpec{
			AccessType:               dbv1alpha1.NetworkAccessTypeRestricted,
			IsMTLSConnectionRequired: common.Bool(isMTLSConnectionRequired),
			AccessControlList:        []string{"192.168.0.1"},
		}

		TestNetworkAccess(k8sClient, dbClient, adbLookupKey, networkRestrictedSpec)()
	}
}

/* Runs a script that connects to an ADB */
func AssertAdminPassword(dbClient *database.DatabaseClient, databaseOCID *string, tnsEntry *string, adminPassword *string, walletPassword *string) error {
	By("Downloading wallet zip")
	walletZip, err := e2eutil.DownloadWalletZip(*dbClient, databaseOCID, walletPassword)
	if err != nil {
		fmt.Fprint(GinkgoWriter, err)
		panic(err)
	}
	fmt.Fprint(GinkgoWriter, walletZip+" successfully downloaded.\n")

	By("Installing SQLcl")
	if _, err := os.Stat("sqlcl-latest.zip"); errors.Is(err, os.ErrNotExist) {
		cmd := exec.Command("wget", "https://download.oracle.com/otn_software/java/sqldeveloper/sqlcl-latest.zip")
		_, err = cmd.Output()
		Expect(err).To(BeNil())
		cmd = exec.Command("unzip", "sqlcl-latest.zip")
		_, err = cmd.Output()
		Expect(err).To(BeNil())
	}

	proxy := os.Getenv("HTTP_PROXY")

	By("Verify the adb connection")
	cmd := exec.Command("./sqlcl/bin/sql", "/nolog", "@verify_connection.sql", proxy, walletZip, *adminPassword, strings.ToLower(*tnsEntry))
	stdout, err := cmd.Output()

	fmt.Fprint(GinkgoWriter, string(stdout))

	return err
}

func TestNetworkAccessPrivate(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, isMTLSConnectionRequired bool, subnetOCID *string, nsgOCIDs *string) func() {
	return func() {
		Expect(*subnetOCID).ToNot(Equal(""))
		Expect(*nsgOCIDs).ToNot(Equal(""))

		adb := &dbv1alpha1.AutonomousDatabase{}
		derefK8sClient := *k8sClient
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).Should(Succeed())

		networkPrivateSpec := dbv1alpha1.NetworkAccessSpec{
			AccessType:               dbv1alpha1.NetworkAccessTypePrivate,
			AccessControlList:        []string{},
			IsMTLSConnectionRequired: common.Bool(isMTLSConnectionRequired),
			PrivateEndpoint: dbv1alpha1.PrivateEndpointSpec{
				HostnamePrefix: adb.Spec.Details.DbName,
				NsgOCIDs:       []string{*nsgOCIDs},
				SubnetOCID:     common.String(*subnetOCID),
			},
		}

		TestNetworkAccess(k8sClient, dbClient, adbLookupKey, networkPrivateSpec)()
	}
}

func TestNetworkAccessPublic(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName) func() {
	return func() {
		networkPublicSpec := dbv1alpha1.NetworkAccessSpec{
			AccessType:               dbv1alpha1.NetworkAccessTypePublic,
			IsMTLSConnectionRequired: common.Bool(true),
		}

		TestNetworkAccess(k8sClient, dbClient, adbLookupKey, networkPublicSpec)()
	}
}

func TestNetworkAccess(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, networkSpec dbv1alpha1.NetworkAccessSpec) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		AssertADBState(k8sClient, dbClient, adbLookupKey, database.AutonomousDatabaseLifecycleStateAvailable)()
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())

		adb.Spec.Details.NetworkAccess = networkSpec
		Expect(derefK8sClient.Update(context.TODO(), adb)).To(Succeed())
		AssertADBDetails(k8sClient, dbClient, adbLookupKey, adb)()
	}
}

// UpdateAndAssertDetails changes the below fields:
// displayName: "bar" -> "bar_new"
// adminPassword: "foo" -> "foo_new",
// cpuCoreCount: from 1 to 2, or from 2 to 1
func UpdateAndAssertDetails(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, newSecretName string, newAdminPassword *string, walletPassword *string) func() {
	return func() {
		expectedADB := UpdateDetails(k8sClient, dbClient, adbLookupKey, newSecretName, newAdminPassword)()
		AssertADBDetails(k8sClient, dbClient, adbLookupKey, expectedADB)()

		ocid := expectedADB.Spec.Details.AutonomousDatabaseOCID
		tnsEntry := *expectedADB.Spec.Details.DbName + "_high"
		err := AssertAdminPassword(dbClient, ocid, &tnsEntry, newAdminPassword, walletPassword)
		Expect(err).ShouldNot(HaveOccurred())
	}
}

// UpdateAndAssertADBState updates adb state and then asserts if change is propagated to OCI
func UpdateAndAssertADBState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		UpdateState(k8sClient, adbLookupKey, state)()
		AssertADBState(k8sClient, dbClient, adbLookupKey, state)()
	}
}

// AssertADBState asserts local and remote state
func AssertADBState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		// Waits longer for the local resource to reach the desired state
		AssertADBLocalState(k8sClient, adbLookupKey, state)()

		// Double-check the state of the DB in OCI so the timeout can be shorter
		AssertADBRemoteState(k8sClient, dbClient, adbLookupKey, state)()
	}
}

// AssertHardLinkDelete asserts the database is terminated in OCI when hardLink is set to true
func AssertHardLinkDelete(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())
		Expect(derefK8sClient.Delete(context.TODO(), adb)).To(Succeed())

		By("Checking if the ADB in OCI is in TERMINATING state")
		// Check every 10 secs for total 60 secs
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			retryPolicy := e2eutil.NewLifecycleStateRetryPolicyADB(database.AutonomousDatabaseLifecycleStateTerminating)
			return returnADBRemoteState(derefK8sClient, derefDBClient, adb.Spec.Details.AutonomousDatabaseOCID, &retryPolicy)
		}, changeTimeout).Should(Equal(database.AutonomousDatabaseLifecycleStateTerminating))

		AssertSoftLinkDelete(k8sClient, adbLookupKey)()
	}
}

// AssertSoftLinkDelete asserts the database remains in OCI when hardLink is set to false
func AssertSoftLinkDelete(k8sClient *client.Client, adbLookupKey *types.NamespacedName) func() {
	return func() {
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
		}, changeTimeout, intervalTime).Should(Equal(true))
	}
}

// AssertADBLocalState asserts the lifecycle state of the local resource using adbLookupKey
func AssertADBLocalState(k8sClient *client.Client, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		By("Checking if the lifecycleState of local resource is " + string(state))
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			return returnADBLocalState(derefK8sClient, *adbLookupKey)
		}, changeLocalStateTimeout).Should(Equal(state))
	}
}

func AssertADBRemoteState(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())
		By("Checking if the lifecycleState of remote resource is " + string(state))
		AssertADBRemoteStateOCID(k8sClient, dbClient, adb.Spec.Details.AutonomousDatabaseOCID, state, changeTimeout)()
	}
}

// Backup takes ~15 minutes to complete, this function waits 20 minutes until ADB state is AVAILABLE
func AssertADBRemoteStateForBackupRestore(k8sClient *client.Client, dbClient *database.DatabaseClient, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		adb := &dbv1alpha1.AutonomousDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *adbLookupKey, adb)).To(Succeed())
		By("Checking if the lifecycleState of remote resource is " + string(state))
		AssertADBRemoteStateOCID(k8sClient, dbClient, adb.Spec.Details.AutonomousDatabaseOCID, state, backupTimeout)()
	}
}

func AssertADBRemoteStateOCID(k8sClient *client.Client, dbClient *database.DatabaseClient, adbID *string, state database.AutonomousDatabaseLifecycleStateEnum, timeout time.Duration) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(adbID).NotTo(BeNil())

		fmt.Fprintf(GinkgoWriter, "ADB ID is %s\n", *adbID)

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		By("Checking if the lifecycleState of the ADB in OCI is " + string(state))
		Eventually(func() (database.AutonomousDatabaseLifecycleStateEnum, error) {
			return returnADBRemoteState(derefK8sClient, derefDBClient, adbID, nil)
		}, timeout, intervalTime).Should(Equal(state))
	}
}

// UpdateState updates state from local resource and OCI
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

func returnADBLocalState(k8sClient client.Client, adbLookupKey types.NamespacedName) (database.AutonomousDatabaseLifecycleStateEnum, error) {
	adb := &dbv1alpha1.AutonomousDatabase{}
	err := k8sClient.Get(context.TODO(), adbLookupKey, adb)
	if err != nil {
		return "", err
	}
	return adb.Status.LifecycleState, nil
}

func returnADBRemoteState(k8sClient client.Client, dbClient database.DatabaseClient, adbID *string, retryPolicy *common.RetryPolicy) (database.AutonomousDatabaseLifecycleStateEnum, error) {
	resp, err := e2eutil.GetAutonomousDatabase(dbClient, adbID, retryPolicy)
	if err != nil {
		return "", err
	}
	return resp.LifecycleState, nil
}

func returnACDLocalState(k8sClient client.Client, acdLookupKey types.NamespacedName) (database.AutonomousContainerDatabaseLifecycleStateEnum, error) {
	acd := &dbv1alpha1.AutonomousContainerDatabase{}
	err := k8sClient.Get(context.TODO(), acdLookupKey, acd)
	if err != nil {
		return "", err
	}
	return acd.Status.LifecycleState, nil
}

func returnACDRemoteState(k8sClient client.Client, dbClient database.DatabaseClient, acdID *string, retryPolicy *common.RetryPolicy) (database.AutonomousContainerDatabaseLifecycleStateEnum, error) {
	resp, err := e2eutil.GetAutonomousContainerDatabase(dbClient, acdID, retryPolicy)
	if err != nil {
		return "", err
	}
	return resp.LifecycleState, nil
}

/* Runs a script that connects to an ADB and configures the backup bucket  */
func ConfigureADBBackup(dbClient *database.DatabaseClient, databaseOCID *string, tnsEntry *string, adminPassword *string, walletPassword *string, bucket *string, authToken *string, ociUser *string) error {

	By("Downloading wallet zip")
	walletZip, err := e2eutil.DownloadWalletZip(*dbClient, databaseOCID, walletPassword)
	if err != nil {
		fmt.Fprint(GinkgoWriter, err)
		panic(err)
	}
	fmt.Fprint(GinkgoWriter, walletZip+" successfully downloaded.\n")

	By("Installing SQLcl")
	if _, err := os.Stat("sqlcl-latest.zip"); errors.Is(err, os.ErrNotExist) {
		cmd := exec.Command("wget", "https://download.oracle.com/otn_software/java/sqldeveloper/sqlcl-latest.zip")
		_, err = cmd.Output()
		Expect(err).To(BeNil())
		cmd = exec.Command("unzip", "sqlcl-latest.zip")
		_, err = cmd.Output()
		Expect(err).To(BeNil())
	}

	proxy := os.Getenv("HTTP_PROXY")

	By("Configuring adb backup bucket")
	cmd := exec.Command("./sqlcl/bin/sql", "/nolog", "@backup.sql", proxy, walletZip, *adminPassword, strings.ToLower(*tnsEntry), *bucket, *ociUser, *authToken)
	stdout, err := cmd.Output()

	fmt.Fprint(GinkgoWriter, string(stdout))

	return err
}

func AssertBackupRestore(k8sClient *client.Client, dbClient *database.DatabaseClient, backupRestoreLookupKey *types.NamespacedName, adbLookupKey *types.NamespacedName, state database.AutonomousDatabaseLifecycleStateEnum) func() {
	return func() {
		// After creating a backup, ADB status will change to BACKUP IN PROGRESS
		// for ~7 minutes. After that time, the state should return to AVAILBLE
		derefK8sClient := *k8sClient

		AssertADBRemoteState(k8sClient, dbClient, adbLookupKey, state)()

		By("Wait until ADB state returns to AVAILABLE")
		AssertADBRemoteStateForBackupRestore(k8sClient, dbClient, adbLookupKey, database.AutonomousDatabaseLifecycleStateAvailable)()

		if state == database.AutonomousDatabaseLifecycleStateBackupInProgress {
			By("Checking adb backup State is ACTIVE")
			createdBackup := &dbv1alpha1.AutonomousDatabaseBackup{}
			Eventually(func() (database.AutonomousDatabaseBackupLifecycleStateEnum, error) {
				derefK8sClient.Get(context.TODO(), *backupRestoreLookupKey, createdBackup)
				return createdBackup.Status.LifecycleState, nil
			}, backupTimeout, time.Second*20).Should(Equal(database.AutonomousDatabaseBackupLifecycleStateActive))
		} else {
			By("Checking adb restore State is SUCCEEDED")
			createdRestore := &dbv1alpha1.AutonomousDatabaseRestore{}
			Eventually(func() (workrequests.WorkRequestStatusEnum, error) {
				derefK8sClient.Get(context.TODO(), *backupRestoreLookupKey, createdRestore)
				return createdRestore.Status.Status, nil
			}, backupTimeout, time.Second*20).Should(Equal(workrequests.WorkRequestStatusSucceeded))
		}
	}
}

func AssertACDState(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName, state database.AutonomousContainerDatabaseLifecycleStateEnum, timeout time.Duration) func() {
	return func() {
		AssertACDLocalState(k8sClient, acdLookupKey, state, timeout)()
		AssertACDRemoteState(k8sClient, dbClient, acdLookupKey, state, timeout)()
	}
}

func AssertACDLocalState(k8sClient *client.Client, acdLookupKey *types.NamespacedName, state database.AutonomousContainerDatabaseLifecycleStateEnum, timeout time.Duration) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(acdLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient

		By("Checking if the lifecycleState of local resource is " + string(state))
		Eventually(func() (database.AutonomousContainerDatabaseLifecycleStateEnum, error) {
			return returnACDLocalState(derefK8sClient, *acdLookupKey)
		}, timeout).Should(Equal(state))
	}
}

func AssertACDRemoteState(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName, state database.AutonomousContainerDatabaseLifecycleStateEnum, timeout time.Duration) func() {
	return func() {
		derefK8sClient := *k8sClient

		acd := &dbv1alpha1.AutonomousContainerDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *acdLookupKey, acd)).To(Succeed())
		By("Checking if the lifecycleState of remote resource is " + string(state))
		AssertACDRemoteStateOCID(k8sClient, dbClient, acd.Spec.AutonomousContainerDatabaseOCID, state, timeout)()
	}
}

func AssertACDRemoteStateOCID(k8sClient *client.Client, dbClient *database.DatabaseClient, acdID *string, state database.AutonomousContainerDatabaseLifecycleStateEnum, timeout time.Duration) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(acdID).NotTo(BeNil())

		fmt.Fprintf(GinkgoWriter, "ACD ID is %s\n", *acdID)

		derefK8sClient := *k8sClient
		derefDBClient := *dbClient

		By("Checking if the lifecycleState of the ACD in OCI is " + string(state))
		Eventually(func() (database.AutonomousContainerDatabaseLifecycleStateEnum, error) {
			return returnACDRemoteState(derefK8sClient, derefDBClient, acdID, nil)
		}, timeout, intervalTime).Should(Equal(state))
	}
}

func AssertACDBind(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName, state database.AutonomousContainerDatabaseLifecycleStateEnum) func() {
	return func() {

		// ACD state should be AVAILABLE
		acdBindTimeout := time.Minute * 3

		By("Wait until ACD is in state AVAILABLE")
		AssertACDState(k8sClient, dbClient, acdLookupKey, state, acdBindTimeout)()
	}
}

func UpdateAndAssertACDSpec(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName) func() {
	return func() {
		expectedACD := UpdateACDSpec(k8sClient, dbClient, acdLookupKey)()
		AssertACDSpec(k8sClient, dbClient, acdLookupKey, expectedACD)()
	}
}

func UpdateACDSpec(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName) func() *dbv1alpha1.AutonomousContainerDatabase {
	return func() *dbv1alpha1.AutonomousContainerDatabase {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())

		derefK8sClient := *k8sClient
		expectedAcd := &dbv1alpha1.AutonomousContainerDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *acdLookupKey, expectedAcd)).To(Succeed())

		expectedAcd.Spec.DisplayName = common.String(*expectedAcd.Spec.DisplayName + "_new")

		Expect(derefK8sClient.Update(context.TODO(), expectedAcd)).To(Succeed())
		return expectedAcd
	}
}

func AssertACDSpec(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName, expectedACD *dbv1alpha1.AutonomousContainerDatabase) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(acdLookupKey).NotTo(BeNil())

		derefDBClient := *dbClient

		expectedACDSpec := expectedACD.Spec
		Eventually(func() (bool, error) {
			// Fetch the ACD from OCI when it's in AVAILABLE state, and retry if its attributes doesn't match the new ACD's attributes
			retryPolicy := e2eutil.NewLifecycleStateRetryPolicyACD(database.AutonomousContainerDatabaseLifecycleStateAvailable)
			resp, err := e2eutil.GetAutonomousContainerDatabase(derefDBClient, expectedACD.Spec.AutonomousContainerDatabaseOCID, &retryPolicy)
			if err != nil {
				return false, err
			}

			debug := true
			if debug {
				if !compareString(expectedACDSpec.AutonomousContainerDatabaseOCID, resp.AutonomousContainerDatabase.Id) {
					fmt.Fprintf(GinkgoWriter, "Expected OCID: %v\nGot: %v\n", expectedACDSpec.AutonomousContainerDatabaseOCID, resp.AutonomousContainerDatabase.Id)
				}
				if !compareString(expectedACDSpec.CompartmentOCID, resp.AutonomousContainerDatabase.CompartmentId) {
					fmt.Fprintf(GinkgoWriter, "Expected CompartmentOCID: %v\nGot: %v\n", expectedACDSpec.CompartmentOCID, resp.CompartmentId)
				}
				if !compareString(expectedACDSpec.DisplayName, resp.AutonomousContainerDatabase.DisplayName) {
					fmt.Fprintf(GinkgoWriter, "Expected DisplayName: %v\nGot: %v\n", expectedACDSpec.DisplayName, resp.AutonomousContainerDatabase.DisplayName)
				}
				if !compareString(expectedACDSpec.AutonomousExadataVMClusterOCID, resp.AutonomousContainerDatabase.CloudAutonomousVmClusterId) {
					fmt.Fprintf(GinkgoWriter, "Expected AutonomousExadataVMClusterOCID: %v\nGot: %v\n", expectedACDSpec.AutonomousExadataVMClusterOCID, resp.AutonomousContainerDatabase.CloudAutonomousVmClusterId)
				}
				if !compareStringMap(expectedACDSpec.FreeformTags, resp.AutonomousContainerDatabase.FreeformTags) {
					fmt.Fprintf(GinkgoWriter, "Expected FreeformTags: %v\nGot: %v\n", expectedACDSpec.FreeformTags, resp.AutonomousContainerDatabase.FreeformTags)
				}
				if expectedACDSpec.PatchModel != resp.AutonomousContainerDatabase.PatchModel {
					fmt.Fprintf(GinkgoWriter, "Expected PatchModel: %v\nGot: %v\n", expectedACDSpec.PatchModel, resp.AutonomousContainerDatabase.PatchModel)
				}
			}

			// Compare the elements one by one rather than doing reflect.DeelEqual(adb1, adb2), since some parameters
			// (e.g. adminPassword, wallet) are missing from e2eutil.GetAutonomousDatabase().
			// We don't compare LifecycleState in this case. We only make sure that the ADB is in AVAIABLE state before
			// proceeding to the next test.
			same := compareString(expectedACDSpec.AutonomousContainerDatabaseOCID, resp.AutonomousContainerDatabase.Id) &&
				compareString(expectedACDSpec.CompartmentOCID, resp.AutonomousContainerDatabase.CompartmentId) &&
				compareString(expectedACDSpec.DisplayName, resp.AutonomousContainerDatabase.DisplayName) &&
				compareString(expectedACDSpec.AutonomousExadataVMClusterOCID, resp.AutonomousContainerDatabase.CloudAutonomousVmClusterId) &&
				compareStringMap(expectedACDSpec.FreeformTags, resp.AutonomousContainerDatabase.FreeformTags) &&
				expectedACDSpec.PatchModel == resp.AutonomousContainerDatabase.PatchModel

			return same, nil
		}, updateACDTimeout, intervalTime).Should(BeTrue())

		// IMPORTANT: make sure the local resource has finished reconciling, otherwise the changes will
		// be conflicted with the next test and cause unknow result.
		AssertACDLocalState(k8sClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateAvailable, time.Minute*2)()
	}
}

func AssertACDRestart(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(acdLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		acd := &dbv1alpha1.AutonomousContainerDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *acdLookupKey, acd)).To(Succeed())

		acd.Spec.Action = dbv1alpha1.AcdActionRestart

		Expect(derefK8sClient.Update(context.TODO(), acd))

		// Check ACD status is RESTARTING
		AssertACDState(k8sClient, dbClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateRestarting, time.Minute*2)()
		// Wait until restart is completed
		AssertACDState(k8sClient, dbClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateAvailable, time.Minute*7)()
	}
}

func AssertACDTerminate(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(acdLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		acd := &dbv1alpha1.AutonomousContainerDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *acdLookupKey, acd))

		acd.Spec.Action = dbv1alpha1.AcdActionTerminate
		Expect(derefK8sClient.Update(context.TODO(), acd))

		// Check ACD status is TERMINATING
		AssertACDState(k8sClient, dbClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateTerminating, time.Minute*2)()
		// Wait until status is TERMINATED
		AssertACDState(k8sClient, dbClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateTerminated, time.Minute*40)()
	}
}

func AssertACDLocalDelete(k8sClient *client.Client, dbClient *database.DatabaseClient, acdLookupKey *types.NamespacedName) func() {
	return func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(dbClient).NotTo(BeNil())
		Expect(acdLookupKey).NotTo(BeNil())

		derefK8sClient := *k8sClient
		existingAcd := &dbv1alpha1.AutonomousContainerDatabase{}
		Expect(derefK8sClient.Get(context.TODO(), *acdLookupKey, existingAcd)).To(Succeed())
		Expect(derefK8sClient.Delete(context.TODO(), existingAcd))

		By("Checking if the AutonomousContainerDatabase resource is deleted")
		Eventually(func() (isDeleted bool) {
			acd := &dbv1alpha1.AutonomousContainerDatabase{}
			isDeleted = false
			err := derefK8sClient.Get(context.TODO(), *acdLookupKey, acd)
			if err != nil && k8sErrors.IsNotFound(err) {
				isDeleted = true
				return
			}
			return
		}, changeTimeout, intervalTime).Should(Equal(true))

		AssertACDRemoteState(k8sClient, dbClient, acdLookupKey, database.AutonomousContainerDatabaseLifecycleStateAvailable, time.Minute*2)
	}
}
