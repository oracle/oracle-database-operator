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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v45/common"
	"github.com/oracle/oci-go-sdk/v45/database"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oracle/oracle-database-operator/test/e2e/behavior"
	"github.com/oracle/oracle-database-operator/test/e2e/util"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

/**
This test suite runs the integration test which checks the following scenario
1. Init the test by creating utililty variables and objects
2. Test ADB provisioned using adminPasswordOCID with hardLink=true
3. Test ADB provisioned using adminPasswordSecret with hardLink=true
5. Test ADB binding with hardLink=true
**/

var dbClient database.DatabaseClient
var configProvider common.ConfigurationProvider

const configFileName = "test_config.yaml"
const ADBNamespace string = "default"

var SharedOCIConfigMapName = "oci-cred"
var SharedOCISecretName = "oci-privatekey"
var SharedPlainTextAdminPassword = "Welcome_1234"
var SharedPlainTextWalletPassword = "Welcome_1234"
var SharedCompartmentOCID string

var SharedKeyOCID string
var SharedAdminPasswordOCID string
var SharedInstanceWalletPasswordOCID string

const SharedAdminPassSecretName string = "adb-admin-password"
const SharedWalletPassSecretName = "adb-wallet-password"

func ADBSetup(k8sClient client.Client) {
	/**************************************************
	 * Custom codes for autonomousdatabase controller
	 **************************************************/
	By("init the test by creating utililty variables and objects")
	testConfig, err := e2eutil.GetTestConfig(configFileName)
	Expect(err).ToNot(HaveOccurred())
	Expect(testConfig).ToNot(BeNil())

	SharedCompartmentOCID = testConfig.CompartmentOCID
	SharedAdminPasswordOCID = testConfig.AdminPasswordOCID
	SharedInstanceWalletPasswordOCID = testConfig.InstanceWalletPasswordOCID

	By("checking if the required parameters exist")
	Expect(testConfig.OCIConfigFile).ToNot(Equal(""))
	Expect(testConfig.CompartmentOCID).ToNot(Equal(""))
	Expect(testConfig.AdminPasswordOCID).ToNot(Equal(""))
	Expect(testConfig.InstanceWalletPasswordOCID).ToNot(Equal(""))

	By("getting OCI provider")
	ociConfigUtil, err := e2eutil.GetOCIConfigUtil(testConfig.OCIConfigFile, testConfig.Profile)
	Expect(err).ToNot(HaveOccurred())
	configProvider, err = ociConfigUtil.GetConfigProvider()
	Expect(err).ToNot(HaveOccurred())

	By("creating a OCI DB client")
	dbClient, err = database.NewDatabaseClientWithConfigurationProvider(configProvider)
	Expect(err).ToNot(HaveOccurred())

	By("creating a configMap for calling OCI")
	ociConfigMap, err := ociConfigUtil.CreateOCIConfigMap(ADBNamespace, SharedOCIConfigMapName)
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient.Create(context.TODO(), ociConfigMap)).To(Succeed())

	By("creating a secret for calling OCI")
	ociSecret, err := ociConfigUtil.CreateOCISecret(ADBNamespace, SharedOCISecretName)
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient.Create(context.TODO(), ociSecret)).To(Succeed())

	By("Creating a k8s secret to hold admin password", func() {
		data := map[string]string{
			SharedAdminPassSecretName: SharedPlainTextAdminPassword,
		}
		adminSecret, err := e2eutil.CreateKubeSecret(ADBNamespace, SharedAdminPassSecretName, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(context.TODO(), adminSecret)).To(Succeed())
	})

	By("Creating a k8s secret to hold wallet password", func() {
		data := map[string]string{
			SharedWalletPassSecretName: SharedPlainTextWalletPassword,
		}
		walletSecret, err := e2eutil.CreateKubeSecret(ADBNamespace, SharedWalletPassSecretName, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(context.TODO(), walletSecret)).To(Succeed())
	})
}

func CleanupADB(k8sClient *client.Client) {
	e2ebehavior.CleanupDB(k8sClient, &dbClient, ADBNamespace)
}
