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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/oracle/oci-go-sdk/v63/common"
	"github.com/oracle/oci-go-sdk/v63/database"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	controllers "github.com/oracle/oracle-database-operator/controllers/database"
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

// To avoid dot import
var (
	BeforeSuite  = ginkgo.BeforeSuite
	AfterSuite   = ginkgo.AfterSuite
	Describe     = ginkgo.Describe
	PDescribe    = ginkgo.PDescribe
	AfterEach    = ginkgo.AfterEach
	By           = ginkgo.By
	It           = ginkgo.It
	FIt          = ginkgo.FIt
	PIt          = ginkgo.PIt
	Expect       = gomega.Expect
	Succeed      = gomega.Succeed
	HaveOccurred = gomega.HaveOccurred
	BeNil        = gomega.BeNil
	Equal        = gomega.Equal
)

var cfg *rest.Config
var k8sClient client.Client
var configProvider common.ConfigurationProvider
var dbClient database.DatabaseClient
var testEnv *envtest.Environment

const configFileName = "test_config.yaml"
const ADBNamespace string = "default"

var SharedOCIConfigMapName = "oci-cred"
var SharedOCISecretName = "oci-privatekey"
var SharedPlainTextAdminPassword = "Welcome_1234"
var SharedPlainTextNewAdminPassword = "Welcome_1234_new"
var SharedPlainTextWalletPassword = "Welcome_1234"
var SharedCompartmentOCID string

var SharedKeyOCID string
var SharedAdminPasswordOCID string
var SharedInstanceWalletPasswordOCID string
var SharedSubnetOCID string
var SharedNsgOCID string

var SharedBucketUrl string
var SharedAuthToken string
var SharedOciUser string

const SharedAdminPassSecretName string = "adb-admin-password"
const SharedNewAdminPassSecretName string = "new-adb-admin-password"
const SharedWalletPassSecretName string = "adb-wallet-password"

func TestAPIs(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("../..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = databasev1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.AutonomousDatabaseReconciler{
		KubeClient: k8sManager.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabase_test"),
		Scheme:     k8sManager.GetScheme(),
		Recorder:   k8sManager.GetEventRecorderFor("AutonomousDatabase_test"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.AutonomousDatabaseBackupReconciler{
		KubeClient: k8sManager.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseBakcup_test"),
		Scheme:     k8sManager.GetScheme(),
		Recorder:   k8sManager.GetEventRecorderFor("AutonomousDatabaseBakcup_test"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.AutonomousDatabaseRestoreReconciler{
		KubeClient: k8sManager.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseRestore_test"),
		Scheme:     k8sManager.GetScheme(),
		Recorder:   k8sManager.GetEventRecorderFor("AutonomousDatabaseRestore_test"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.AutonomousContainerDatabaseReconciler{
		KubeClient: k8sManager.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousContainerDatabase_test"),
		Scheme:     k8sManager.GetScheme(),
		Recorder:   k8sManager.GetEventRecorderFor("AutonomousContainerDatabase_test"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer ginkgo.GinkgoRecover()
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		gexec.KillAndWait(4 * time.Second)

		// Teardown the test environment once controller is fnished.
		// Otherwise from Kubernetes 1.21+, teardon timeouts waiting on
		// kube-apiserver to return
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	}()

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
	SharedSubnetOCID = testConfig.SubnetOCID
	SharedNsgOCID = testConfig.NsgOCID
	SharedBucketUrl = testConfig.BucketURL
	SharedAuthToken = testConfig.AuthToken
	SharedOciUser = testConfig.OciUser

	By("checking if the required parameters exist")
	Expect(testConfig.OCIConfigFile).ToNot(Equal(""))
	Expect(testConfig.CompartmentOCID).ToNot(Equal(""))
	Expect(testConfig.AdminPasswordOCID).ToNot(Equal(""))
	Expect(testConfig.InstanceWalletPasswordOCID).ToNot(Equal(""))
	Expect(testConfig.SubnetOCID).ToNot(Equal(""))
	Expect(testConfig.NsgOCID).ToNot(Equal(""))
	Expect(testConfig.BucketURL).ToNot(Equal(""))
	Expect(testConfig.AuthToken).ToNot(Equal(""))
	Expect(testConfig.OciUser).ToNot(Equal(""))

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

	By("Creating a k8s secret to hold new admin password", func() {
		data := map[string]string{
			SharedNewAdminPassSecretName: SharedPlainTextNewAdminPassword,
		}
		newAdminSecret, err := e2eutil.CreateKubeSecret(ADBNamespace, SharedNewAdminPassSecretName, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(context.TODO(), newAdminSecret)).To(Succeed())
	})

	By("Creating a k8s secret to hold wallet password", func() {
		data := map[string]string{
			SharedWalletPassSecretName: SharedPlainTextWalletPassword,
		}
		walletSecret, err := e2eutil.CreateKubeSecret(ADBNamespace, SharedWalletPassSecretName, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(context.TODO(), walletSecret)).To(Succeed())
	})
})

var _ = AfterSuite(func() {
	/*
			From Kubernetes 1.21+, when it tries to cleanup the test environment, there is
			a clash if a custom controller is created during testing. It would seem that
			the controller is still running and kube-apiserver will not respond to shutdown.
			This is the reason why teardown happens in BeforeSuite() after controller has stopped.
			The error shown is as documented in:
			https://github.com/kubernetes-sigs/controller-runtime/issues/1571
		/*
		/*
		By("tearing down the test environment")
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	*/

	By("Delete the resources that are created during the tests")
	adbList := &databasev1alpha1.AutonomousDatabaseList{}
	options := &client.ListOptions{
		Namespace: ADBNamespace,
	}
	k8sClient.List(context.TODO(), adbList, options)
	By(fmt.Sprintf("Found %d AutonomousDatabase(s)", len(adbList.Items)))

	for _, adb := range adbList.Items {
		if adb.Spec.Details.AutonomousDatabaseOCID != nil {
			By("Terminating database " + *adb.Spec.Details.DbName)
			Expect(e2eutil.DeleteAutonomousDatabase(dbClient, adb.Spec.Details.AutonomousDatabaseOCID)).Should(Succeed())
		}
	}

	// Delete sqlcl-latest.zip and sqlcl folder if exists
	os.Remove("sqlcl-latest.zip")
	os.RemoveAll("sqlcl")
})
