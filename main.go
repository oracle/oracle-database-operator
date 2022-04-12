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

package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	databasecontroller "github.com/oracle/oracle-database-operator/controllers/database"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

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

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "a9d608ea.oracle.com",
	})
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
	if err = (&databasecontroller.DbcsSystemReconciler{
		KubeClient: mgr.GetClient(),
		Logger:     ctrl.Log.WithName("controllers").WithName("database").WithName("DbcsSystem"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("DbcsSystem"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DbcsSystem")
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
