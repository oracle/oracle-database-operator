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

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	databasecontroller "github.com/oracle/oracle-database-operator/controllers/database"

	observabilityv1alpha1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	observabilitycontroller "github.com/oracle/oracle-database-operator/controllers/observability"
	// +kubebuilder:scaffold:imports
)

var (
	scheme                       = runtime.NewScheme()
	setupLog                     = ctrl.Log.WithName("setup")
	universalInstanceGuid string = "63624bfd-67b9-11ea-81b7-0a580aed61c8"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(observabilityv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitorv1.AddToScheme(scheme))
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	// Initialize new logger Opts
	options := &zap.Options{
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}

	// zap-level=2 enables verbose logging
	dbg, _ := os.LookupEnv("TT_DEBUG")
	if dbg == "1" || dbg == "99" {
		options.Level = zapcore.Level(-2)
	} else {
		options.Level = zapcore.Level(-1)
	}

	ctrl.SetLogger(zap.New(func(o *zap.Options) { *o = *options }))

	watchNamespaces, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "Failed to get watch namespaces")
		os.Exit(1)
	}
	opt := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "a9d608ea.oracle.com",
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			opts.DefaultNamespaces = watchNamespaces
			return cache.New(config, opts)
		},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opt)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Get Cache
	cache := mgr.GetCache()

	// ADB family controllers
	if err = (&databasecontroller.AutonomousDatabaseReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("database").WithName("AutonomousDatabase"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabase"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AutonomousDatabase")
		os.Exit(1)
	}
	if err = (&databasecontroller.AutonomousDatabaseBackupReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseBackup"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseBackup"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AutonomousDatabaseBackup")
		os.Exit(1)
	}
	if err = (&databasecontroller.AutonomousDatabaseRestoreReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseRestore"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseRestore"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AutonomousDatabaseRestore")
		os.Exit(1)
	}
	if err = (&databasecontroller.AutonomousContainerDatabaseReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousContainerDatabase"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousContainerDatabase"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AutonomousContainerDatabase")
		os.Exit(1)
	}

	if err = (&databasecontroller.SingleInstanceDatabaseReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("SingleInstanceDatabase"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("SingleInstanceDatabase"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SingleInstanceDatabase")
		os.Exit(1)
	}
	if err = (&databasecontroller.ShardingDatabaseReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("ShardingDatabase"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ShardingDatabase"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ShardingDatabase")
		os.Exit(1)
	}
	if err = (&databasecontroller.DbcsSystemReconciler{
		KubeClient: mgr.GetClient(),
		Logger:     ctrl.Log.WithName("controllers").WithName("database").WithName("DbcsSystem"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("DbcsSystem"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DbcsSystem")
		os.Exit(1)
	}
	if err = (&databasecontroller.OracleRestDataServiceReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("OracleRestDataService"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("OracleRestDataService"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OracleRestDataService")
		os.Exit(1)
	}

	setupLog.Info("Setting up TT Reconciler.")
	if err = (&databasecontroller.TimesTenClassicReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TimesTenClassic")
		os.Exit(1)
	}
	setupLog.Info("TT Reconciler set.")

	// Set RECONCILE_INTERVAL environment variable if you want to change the default value from 15 secs
	interval := os.Getenv("RECONCILE_INTERVAL")
	i, err := strconv.ParseInt(interval, 10, 64)
	if err != nil {
		i = 15
		setupLog.Info("Setting default reconcile period for database-controller", "Secs", i)
	}

	// Set ENABLE_WEBHOOKS=false when we run locally to skip webhook part when testing just the controller. Not to be used in production.
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&databasev1alpha1.SingleInstanceDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "SingleInstanceDatabase")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.OracleRestDataService{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "OracleRestDataService")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.PDB{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "PDB")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.CDB{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CDB")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.AutonomousDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabase")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabaseBackup")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabaseRestore")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousContainerDatabase")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.DataguardBroker{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DataguardBroker")
			os.Exit(1)
		}
		if err = (&observabilityv1alpha1.DatabaseObserver{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DatabaseObserver")
			os.Exit(1)
		}
	}

	// PDB Reconciler
	if err = (&databasecontroller.PDBReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("PDB"),
		Interval: time.Duration(i),
		Recorder: mgr.GetEventRecorderFor("PDB"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PDB")
		os.Exit(1)
	}

	// CDB Reconciler
	if err = (&databasecontroller.CDBReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Log:      ctrl.Log.WithName("controllers").WithName("CDB"),
		Interval: time.Duration(i),
		Recorder: mgr.GetEventRecorderFor("CDB"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CDB")
		os.Exit(1)
	}
	if err = (&databasecontroller.DataguardBrokerReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("DataguardBroker"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("DataguardBroker"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataguardBroker")
		os.Exit(1)
	}

	// Observability DatabaseObserver Reconciler
	if err = (&observabilitycontroller.DatabaseObserverReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("observability").WithName("DatabaseObserver"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("DatabaseObserver"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseObserver")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// Add index for PDB CR to enable mgr to cache PDBs
	indexFunc := func(obj client.Object) []string {
		return []string{obj.(*databasev1alpha1.PDB).Spec.PDBName}
	}
	if err = cache.IndexField(context.TODO(), &databasev1alpha1.PDB{}, "spec.pdbName", indexFunc); err != nil {
		setupLog.Error(err, "unable to create index function for ", "controller", "PDB")
		os.Exit(1)
	}

	setupLog.Info("Calling setupTimesten")
	timestenHome, err := setupTimesTen()
	if err != nil {
		setupLog.Error(err, "could not create the operator's TimesTen instance")
		os.Exit(1)
	} else {
		os.Setenv("TIMESTEN_HOME", timestenHome)
		setupLog.Info("env TIMESTEN_HOME set to " + timestenHome)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getWatchNamespace() (map[string]cache.Config, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.

	var watchNamespaceEnvVar = "WATCH_NAMESPACE"
	var nsmap map[string]cache.Config
	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	values := strings.Split(ns, ",")
	if len(values) == 1 && values[0] == "" {
		fmt.Printf(":CLUSTER SCOPED:\n")
		return nil, nil
	}
	fmt.Printf(":NAMESPACE SCOPED:\n")
	fmt.Printf("WATCH LIST=%s\n", values)
	nsmap = make(map[string]cache.Config, len(values))
	if !found {
		return nsmap, fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}

	if ns == "" {
		return nil, nil
	}

	for _, ns := range values {
		nsmap[ns] = cache.Config{}
	}

	return nsmap, nil

}

// Command to run a shell command, returning the output to the caller
func runShellCommand(cmd []string) (int, []string, []string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var ourRc int

	c := exec.Command(cmd[0])
	c.Args = cmd[0:]
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err == nil {
		// Command ran with exit code 0
		ourRc = 0
	} else {
		exitError, ok := err.(*exec.ExitError)
		if ok {
			ourRc = exitError.ExitCode()
		} else {
			setupLog.Error(err, "Error starting process: "+err.Error())
			return 254, []string{}, []string{}
		}
	}

	printCmd := ""
	for _, l := range cmd {
		printCmd = printCmd + l + " "
	}
	setupLog.Info("runShellCommand '" + printCmd + "': rc " + strconv.Itoa(ourRc))

	var outStdout []string
	for _, l := range strings.Split(stdout.String(), "\n") {
		outStdout = append(outStdout, l)
	}

	var outStderr []string
	for _, l := range strings.Split(stderr.String(), "\n") {
		outStderr = append(outStderr, l)
	}

	return ourRc, outStdout, outStderr
}

func makeNewTimestenInstallation() (string, error) {
	us := "makeNewTimestenInstallation"
	setupLog.Info(us + " entered")
	defer setupLog.Info(us + " returns")
	mntDistroDir := "/mnt/mizuniga/distros"
	distroZipFile := "timesten2211220.server.linux8664.zip"
	ttDistroVer := "22.1.1.22.0"

	rcCopyZip, stdoutCopyZip, stderrCopyZip := runShellCommand([]string{"cp", mntDistroDir + "/" + distroZipFile, "/tmp/"})
	if rcCopyZip != 0 {
		err := errors.New("Error " + strconv.Itoa(rcCopyZip) + " copying TimesTen binaries")
		setupLog.Error(err, "Error copying binaries", "stdout", stdoutCopyZip, "stderr", stderrCopyZip)
		return "", err
	}

	rclsZip, stdoutlsZip, stderrlsZip := runShellCommand([]string{"ls", "/tmp/" + distroZipFile})
	if rcCopyZip != 0 {
		err := errors.New("Error " + strconv.Itoa(rclsZip) + " verifying TimesTen binaries")
		setupLog.Error(err, "Error verifying binaries", "stdout", stdoutlsZip, "stderr", stderrlsZip)
		return "", err
	}

	rcMkdir, stdoutMkdir, stderrMkdir := runShellCommand([]string{"mkdir", "/tmp/tt"})
	if rcMkdir != 0 {
		err := errors.New("Error " + strconv.Itoa(rcMkdir) + " mkdir /tmp/tt to place TimesTen binaries")
		setupLog.Error(err, "Error mkdir /tmp/tt to place", "stdout", stdoutMkdir, "stderr", stderrMkdir)
		if rcMkdir != 1 {
			return "", err
		}
	}

	rcUnzip, stdoutUnzip, stderrUnzip := runShellCommand([]string{"unzip", "-q", "/tmp/" + distroZipFile, "-d", "/timesten/installations/"})
	if rcUnzip != 0 {
		err := errors.New("Error " + strconv.Itoa(rcUnzip) + " unzip TimesTen binaries")
		setupLog.Error(err, "Error unzipping binaries", "stdout", stdoutUnzip, "stderr", stderrUnzip)
		return "", err
	}
	_, out, _ := runShellCommand([]string{"ls", "-la", "/timesten/installation/"})
	setupLog.Info(strings.Join(out, "\n"))
	_, out2, _ := runShellCommand([]string{"ls", "-la", "/timesten/installations/"})
	setupLog.Info(strings.Join(out2, "\n"))

	//rcCopyUnzip, stdoutCopyUnzip, stderrCopyUnzip := runShellCommand([]string{"cp", "-R", "/tmp/tt/tt" + ttDistroVer + "/", "/timesten/installations/tt" + ttDistroVer + "/"})
	//if rcCopyUnzip != 0 {
	//	err := errors.New("Error " + strconv.Itoa(rcCopyUnzip) + " copy unziped TimesTen binaries")
	//	setupLog.Error(err, "Error copy unziped binaries", "stdout", stdoutCopyUnzip, "stderr", stderrCopyUnzip)
	//	return err
	//}

	rcChmod, stdoutChmod, stderrChmod := runShellCommand([]string{"chmod", "550", "/timesten/installations/tt" + ttDistroVer})
	if rcChmod != 0 {
		err := errors.New("Error " + strconv.Itoa(rcUnzip) + " chmod unziped TimesTen binaries")
		setupLog.Error(err, "Error chmod unziped binaries", "stdout", stdoutChmod, "stderr", stderrChmod)
		return "", err
	}
	return "/timesten/installations/tt" + ttDistroVer, nil
}

// See if a TimesTen installation already exists and create one if not
func findTimesTenInstallation(location string, makeNewOnePlease bool) (string, error) {
	us := "findTimesTenInstallation"
	setupLog.Info(us + " entered")
	defer setupLog.Info(us + " returns")

	xArgs := []string{"-c", "ls -d /*/installation"}
	loc, err := exec.Command("/bin/bash", xArgs...).Output()
	if err != nil {
		setupLog.Error(err, us+": cannot find installation: "+string(loc))
		panic(err)
	}
	loca := strings.TrimSpace(string(loc))
	setupLog.Info(us + ": found '" + loca + "'")
	lsArgs := []string{"-c", "ls " + loca + "/*/bin/ttInstanceCreate"}
	lsBinOut, lsErr := exec.Command("/bin/bash", lsArgs...).Output()

	if lsErr == nil {
		return loca + "/*", nil
	}
	setupLog.Error(lsErr, us+": cannot find installation binaries: "+string(lsBinOut))

	installLoc, newInstallErr := makeNewTimestenInstallation()

	if newInstallErr != nil {
		setupLog.Error(newInstallErr, us+": cannot find installation binaries: "+string(loca))
		panic(newInstallErr)
	}
	return installLoc, nil
}

// Make a TimesTen instance
func makeTimesTenInstance(location string, instanceName string) (string, error) {
	us := "makeTimesTenInstance"
	setupLog.Info(us + " entered")
	defer setupLog.Info(us + " returns")

	installation, err := findTimesTenInstallation(location, true)
	if err != nil {
		return "", err
	}

	rc, stdout, stderr := runShellCommand([]string{installation + "/bin/ttInstanceCreate", "-location", location, "-name", instanceName})
	if rc != 0 {
		err := errors.New("Error " + strconv.Itoa(rc) + " creating TimesTen instance")
		setupLog.Error(err, "Error creating instance", "stdout", stdout, "stderr", stderr)
		return "", err
	}

	// We have to modify the instance guid to a well known value so the
	// agents can read the Oracle Wallets that we create.

	ttc, err := ioutil.ReadFile(location + "/" + instanceName + "/conf/timesten.conf")
	if err != nil {
		setupLog.Error(err, "Error reading "+location+"/"+instanceName+"/conf/timesten.conf")
		return "", err
	}

	ttclines := strings.Split(string(ttc), "\n")
	for i, l := range ttclines {
		if strings.HasPrefix(l, "instance_guid=") {
			ttclines[i] = "instance_guid=" + universalInstanceGuid
		}
	}

	err = ioutil.WriteFile(location+"/"+instanceName+"/conf/timesten.conf", []byte(strings.Join(ttclines, "\n")), 0600)
	if err != nil {
		setupLog.Error(err, "Error writing "+location+"/"+instanceName+"/conf/timesten.conf")
		return "", err
	}

	return location + "/" + instanceName, nil
}

func setupTimesTen() (string, error) {
	us := "setupTimesTen"
	setupLog.Info(us + " entered")
	defer setupLog.Info(us + " returns")

	timestenHome, ok := os.LookupEnv("TIMESTEN_HOME")
	if ok {
		setupLog.Info(us + ": using TimesTen instance in " + timestenHome)
		return timestenHome, nil
	}

	// We want to use an instance in $HOME/instance1

	instanceName := "instance1"

	//reqLogger.Info(us + ": sleep 60")
	//time.Sleep(60 * time.Second)
	//reqLogger.Info(us + ": sleep done")

	// See if there are any TimesTen distributions we can use

	homeDir, ok := os.LookupEnv("HOME")
	if !ok {
		err := errors.New(us + ": TimesTen not configured and HOME directory not set")
		//reqLogger.Error(err, us + ": TimesTen not configured and HOME directory not set")
		setupLog.Error(err, us+": TimesTen not configured and HOME directory not set")
		return "", err
	}

	fullInstanceName := "/timesten" + "/" + instanceName
	st, err := os.Stat(fullInstanceName)
	if err == nil {
		if st.IsDir() {
			// Let's go ahead and use it
			setupLog.Info(us + ": found TimesTen instance " + fullInstanceName)
			os.Setenv("TIMESTEN_HOME", fullInstanceName)
			return fullInstanceName, nil
		} else {
			e := errors.New(fullInstanceName + " exists but is not a directory")
			setupLog.Error(e, "Could not make TimesTen instance")
			panic(e)
		}
	} else {
		if os.IsNotExist(err) {
			// Great, let's make it. First we have to find an installation we can use
			setupLog.Info(us + ": TimesTen instance not found")
			timestenHome, err := makeTimesTenInstance(homeDir, instanceName)
			if err == nil {
				return timestenHome, nil
			} else {
				panic(err)
			}

		} else {
			e := errors.New(fullInstanceName + " exists but is not readable")
			setupLog.Error(e, "Could not make TimesTen instance")
			panic(e)
		}
	}
}
