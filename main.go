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
	"context"
	"flag"
	"fmt"
	"os"
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
	dataguardcontroller "github.com/oracle/oracle-database-operator/controllers/dataguard"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	observabilityv1alpha1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	observabilitycontroller "github.com/oracle/oracle-database-operator/controllers/observability"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(observabilityv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitorv1.AddToScheme(scheme))
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(databasev4.AddToScheme(scheme))
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
		if err = (&databasev4.AutonomousDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabase")
			os.Exit(1)
		}
		if err = (&databasev4.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabaseBackup")
			os.Exit(1)
		}
		if err = (&databasev4.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousDatabaseRestore")
			os.Exit(1)
		}
		if err = (&databasev4.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AutonomousContainerDatabase")
			os.Exit(1)
		}
		if err = (&databasev1alpha1.DataguardBroker{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DataguardBroker")
			os.Exit(1)
		}
		if err = (&databasev4.ShardingDatabase{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ShardingDatabase")
		}
		if err = (&observabilityv1alpha1.DatabaseObserver{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DatabaseObserver")
			os.Exit(1)
		}
                if err = (&databasev4.DbcsSystem{}).SetupWebhookWithManager(mgr); err != nil {
                        setupLog.Error(err, "unable to create webhook", "webhook", "DbcsSystem")
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
	if err = (&dataguardcontroller.DataguardBrokerReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("dataguard").WithName("DataguardBroker"),
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
