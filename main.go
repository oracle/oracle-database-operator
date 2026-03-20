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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	observabilityv1 "github.com/oracle/oracle-database-operator/apis/observability/v1"
	observabilityv1alpha1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	observabilityv4 "github.com/oracle/oracle-database-operator/apis/observability/v4"
	databasecontroller "github.com/oracle/oracle-database-operator/controllers/database"
	dataguardcontroller "github.com/oracle/oracle-database-operator/controllers/dataguard"
	observabilitycontroller "github.com/oracle/oracle-database-operator/controllers/observability"
	// +kubebuilder:scaffold:imports
)

const (
	watchNamespaceEnvVar = "WATCH_NAMESPACE"
	reconcileIntervalEnv = "RECONCILE_INTERVAL"
	enableWebhooksEnvVar = "ENABLE_WEBHOOKS"
	defaultReconcileSecs = int64(15)
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type managedComponent struct {
	name  string
	setup func() error
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(observabilityv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitorv1.AddToScheme(scheme))
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(databasev4.AddToScheme(scheme))
	utilruntime.Must(observabilityv1.AddToScheme(scheme))
	utilruntime.Must(observabilityv4.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	metricsAddr, enableLeaderElection := parseFlags()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.TimeEncoder = zapcore.RFC3339TimeEncoder
	}))

	setupLog.Info("env check",
		"KUBECONFIG", os.Getenv("KUBECONFIG"),
		"WATCH_NAMESPACE", os.Getenv("WATCH_NAMESPACE"),
	)

	watchNamespaces, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get watch namespaces")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
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
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(record.CorrelatorOptions{
			BurstSize: 10,
			QPS:       1,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := setupControllers(mgr, parseReconcileInterval()); err != nil {
		setupLog.Error(err, "controller setup failed")
		os.Exit(1)
	}

	if os.Getenv(enableWebhooksEnvVar) != "false" {
		if err := setupWebhooks(mgr); err != nil {
			setupLog.Error(err, "webhook setup failed")
			os.Exit(1)
		}
	}

	indexByPDBName := func(obj client.Object) []string {
		return []string{obj.(*databasev4.LRPDB).Spec.LRPDBName}
	}
	if err := mgr.GetCache().IndexField(context.Background(), &databasev4.LRPDB{}, "spec.pdbName", indexByPDBName); err != nil {
		setupLog.Error(err, "unable to create index function", "controller", "LRPDB")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseFlags() (metricsAddr string, enableLeaderElection bool) {
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()
	return metricsAddr, enableLeaderElection
}

func setupControllers(mgr ctrl.Manager, interval time.Duration) error {
	components := []managedComponent{
		{
			name: "AutonomousDatabase",
			setup: func() error {
				return (&databasecontroller.AutonomousDatabaseReconciler{
					KubeClient: mgr.GetClient(),
					Log:        ctrl.Log.WithName("controllers").WithName("database").WithName("AutonomousDatabase"),
					Scheme:     mgr.GetScheme(),
					Recorder:   mgr.GetEventRecorderFor("AutonomousDatabase"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "AutonomousDatabaseBackup",
			setup: func() error {
				return (&databasecontroller.AutonomousDatabaseBackupReconciler{
					KubeClient: mgr.GetClient(),
					Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseBackup"),
					Scheme:     mgr.GetScheme(),
					Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseBackup"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "AutonomousDatabaseRestore",
			setup: func() error {
				return (&databasecontroller.AutonomousDatabaseRestoreReconciler{
					KubeClient: mgr.GetClient(),
					Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseRestore"),
					Scheme:     mgr.GetScheme(),
					Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseRestore"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "AutonomousContainerDatabase",
			setup: func() error {
				return (&databasecontroller.AutonomousContainerDatabaseReconciler{
					KubeClient: mgr.GetClient(),
					Log:        ctrl.Log.WithName("controllers").WithName("AutonomousContainerDatabase"),
					Scheme:     mgr.GetScheme(),
					Recorder:   mgr.GetEventRecorderFor("AutonomousContainerDatabase"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "SingleInstanceDatabase",
			setup: func() error {
				return (&databasecontroller.SingleInstanceDatabaseReconciler{
					Client:   mgr.GetClient(),
					Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("SingleInstanceDatabase"),
					Scheme:   mgr.GetScheme(),
					Config:   mgr.GetConfig(),
					Recorder: mgr.GetEventRecorderFor("SingleInstanceDatabase"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "ShardingDatabase",
			setup: func() error {
				return (&databasecontroller.ShardingDatabaseReconciler{
					Client:    mgr.GetClient(),
					Log:       ctrl.Log.WithName("controllers").WithName("database").WithName("ShardingDatabase"),
					Scheme:    mgr.GetScheme(),
					Recorder:  mgr.GetEventRecorderFor("ShardingDatabase"),
					APIReader: mgr.GetAPIReader(),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "DbcsSystem",
			setup: func() error {
				return (&databasecontroller.DbcsSystemReconciler{
					KubeClient: mgr.GetClient(),
					Logger:     ctrl.Log.WithName("controllers").WithName("database").WithName("DbcsSystem"),
					Scheme:     mgr.GetScheme(),
					Recorder:   mgr.GetEventRecorderFor("DbcsSystem"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "OracleRestDataService",
			setup: func() error {
				return (&databasecontroller.OracleRestDataServiceReconciler{
					Client:   mgr.GetClient(),
					Log:      ctrl.Log.WithName("controllers").WithName("OracleRestDataService"),
					Scheme:   mgr.GetScheme(),
					Config:   mgr.GetConfig(),
					Recorder: mgr.GetEventRecorderFor("OracleRestDataService"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "OracleRestart",
			setup: func() error {
				return (&databasecontroller.OracleRestartReconciler{
					Client: mgr.GetClient(),
					Log:    ctrl.Log.WithName("controllers").WithName("OracleRestart"),
					Scheme: mgr.GetScheme(),
					Config: mgr.GetConfig(),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "LRPDB",
			setup: func() error {
				return (&databasecontroller.LRPDBReconciler{
					Client:   mgr.GetClient(),
					Scheme:   mgr.GetScheme(),
					Log:      ctrl.Log.WithName("controllers").WithName("LRPDB"),
					Interval: interval,
					Recorder: mgr.GetEventRecorderFor("LRPDB"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "LREST",
			setup: func() error {
				return (&databasecontroller.LRESTReconciler{
					Client:   mgr.GetClient(),
					Scheme:   mgr.GetScheme(),
					Config:   mgr.GetConfig(),
					Log:      ctrl.Log.WithName("controllers").WithName("LREST"),
					Interval: interval,
					Recorder: mgr.GetEventRecorderFor("LREST"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "DataguardBroker",
			setup: func() error {
				return (&dataguardcontroller.DataguardBrokerReconciler{
					Client:   mgr.GetClient(),
					Log:      ctrl.Log.WithName("controllers").WithName("dataguard").WithName("DataguardBroker"),
					Scheme:   mgr.GetScheme(),
					Config:   mgr.GetConfig(),
					Recorder: mgr.GetEventRecorderFor("DataguardBroker"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "OrdsSrvs",
			setup: func() error {
				return (&databasecontroller.OrdsSrvsReconciler{
					Client:   mgr.GetClient(),
					Scheme:   mgr.GetScheme(),
					Recorder: mgr.GetEventRecorderFor("OrdsSrvs"),
				}).SetupWithManager(mgr)
			},
		},
		{
			name: "DatabaseObserver",
			setup: func() error {
				return (&observabilitycontroller.DatabaseObserverReconciler{
					Client:   mgr.GetClient(),
					Log:      ctrl.Log.WithName("controllers").WithName("observability").WithName("DatabaseObserver"),
					Scheme:   mgr.GetScheme(),
					Recorder: mgr.GetEventRecorderFor("DatabaseObserver"),
				}).SetupWithManager(mgr)
			},
		},
		// +kubebuilder:scaffold:builder
	}

	for _, c := range components {
		if err := c.setup(); err != nil {
			return componentError("controller", c.name, err)
		}
	}
	return nil
}

func setupWebhooks(mgr ctrl.Manager) error {
	webhooks := []managedComponent{
		{name: "SingleInstanceDatabase(v1alpha1)", setup: func() error { return (&databasev1alpha1.SingleInstanceDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "OracleRestDataService(v1alpha1)", setup: func() error { return (&databasev1alpha1.OracleRestDataService{}).SetupWebhookWithManager(mgr) }},
		{name: "LRPDB(v4)", setup: func() error { return (&databasev4.LRPDB{}).SetupWebhookWithManager(mgr) }},
		{name: "LREST(v4)", setup: func() error { return (&databasev4.LREST{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabase(v1alpha1)", setup: func() error { return (&databasev1alpha1.AutonomousDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabaseBackup(v1alpha1)", setup: func() error { return (&databasev1alpha1.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabaseRestore(v1alpha1)", setup: func() error { return (&databasev1alpha1.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousContainerDatabase(v1alpha1)", setup: func() error { return (&databasev1alpha1.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabase(v4)", setup: func() error { return (&databasev4.AutonomousDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabaseBackup(v4)", setup: func() error { return (&databasev4.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousDatabaseRestore(v4)", setup: func() error { return (&databasev4.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr) }},
		{name: "AutonomousContainerDatabase(v4)", setup: func() error { return (&databasev4.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "DataguardBroker(v1alpha1)", setup: func() error { return (&databasev1alpha1.DataguardBroker{}).SetupWebhookWithManager(mgr) }},
		{name: "ShardingDatabase(v1alpha1)", setup: func() error { return (&databasev1alpha1.ShardingDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "DbcsSystem(v1alpha1)", setup: func() error { return (&databasev1alpha1.DbcsSystem{}).SetupWebhookWithManager(mgr) }},
		{name: "ShardingDatabase(v4)", setup: func() error { return (&databasev4.ShardingDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "DatabaseObserver(v1alpha1)", setup: func() error { return (&observabilityv1alpha1.DatabaseObserver{}).SetupWebhookWithManager(mgr) }},
		{name: "DbcsSystem(v4)", setup: func() error { return (&databasev4.DbcsSystem{}).SetupWebhookWithManager(mgr) }},
		{name: "DatabaseObserver(v1)", setup: func() error { return (&observabilityv1.DatabaseObserver{}).SetupWebhookWithManager(mgr) }},
		{name: "DatabaseObserver(v4)", setup: func() error { return (&observabilityv4.DatabaseObserver{}).SetupWebhookWithManager(mgr) }},
		{name: "SingleInstanceDatabase(v4)", setup: func() error { return (&databasev4.SingleInstanceDatabase{}).SetupWebhookWithManager(mgr) }},
		{name: "DataguardBroker(v4)", setup: func() error { return (&databasev4.DataguardBroker{}).SetupWebhookWithManager(mgr) }},
		{name: "OracleRestDataService(v4)", setup: func() error { return (&databasev4.OracleRestDataService{}).SetupWebhookWithManager(mgr) }},
		{name: "OracleRestart(v4)", setup: func() error { return (&databasev4.OracleRestart{}).SetupWebhookWithManager(mgr) }},
	}

	for _, w := range webhooks {
		if err := w.setup(); err != nil {
			return componentError("webhook", w.name, err)
		}
	}
	return nil
}

func getWatchNamespace() (map[string]cache.Config, error) {
	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found || strings.TrimSpace(ns) == "" {
		setupLog.Info("running in cluster-scoped mode", "env", watchNamespaceEnvVar)
		return nil, nil
	}

	nsMap := make(map[string]cache.Config)
	for _, namespace := range strings.Split(ns, ",") {
		trimmed := strings.TrimSpace(namespace)
		if trimmed == "" {
			continue
		}
		nsMap[trimmed] = cache.Config{}
	}
	if len(nsMap) == 0 {
		return nil, nil
	}

	setupLog.Info("running in namespace-scoped mode", "watchNamespaces", keys(nsMap))
	return nsMap, nil
}

func parseReconcileInterval() time.Duration {
	interval := strings.TrimSpace(os.Getenv(reconcileIntervalEnv))
	if interval == "" {
		setupLog.Info("using default reconcile interval for database controllers", "seconds", defaultReconcileSecs)
		return time.Duration(defaultReconcileSecs) * time.Second
	}

	secs, err := strconv.ParseInt(interval, 10, 64)
	if err != nil || secs <= 0 {
		setupLog.Info("invalid reconcile interval; using default", "env", reconcileIntervalEnv, "value", interval, "seconds", defaultReconcileSecs)
		return time.Duration(defaultReconcileSecs) * time.Second
	}

	setupLog.Info("using configured reconcile interval for database controllers", "seconds", secs)
	return time.Duration(secs) * time.Second
}

func keys(m map[string]cache.Config) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func componentError(kind, name string, err error) error {
	return &componentSetupError{kind: kind, name: name, err: err}
}

type componentSetupError struct {
	kind string
	name string
	err  error
}

func (e *componentSetupError) Error() string {
	return "unable to create " + e.kind + " " + e.name + ": " + e.err.Error()
}

func (e *componentSetupError) Unwrap() error {
	return e.err
}
