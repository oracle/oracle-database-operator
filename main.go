/*
** Copyright (c) 2022, 2026 Oracle and/or its affiliates.
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

// Package main bootstraps the Oracle Database Operator manager and registers
// controllers for CRDs including AutonomousDatabase, AutonomousDatabaseBackup,
// AutonomousDatabaseRestore, AutonomousContainerDatabase, SingleInstanceDatabase,
// ShardingDatabase, DbcsSystem, OracleRestDataService, OracleRestart, LRPDB,
// LREST, DataguardBroker, OrdsSrvs, RacDatabase, and PrivateAi.
package main

import (
	"context"
	"crypto/tls"
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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	observabilityv1 "github.com/oracle/oracle-database-operator/apis/observability/v1"
	observabilityv1alpha1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	observabilityv4 "github.com/oracle/oracle-database-operator/apis/observability/v4"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	databasecontroller "github.com/oracle/oracle-database-operator/controllers/database"
	dataguardcontroller "github.com/oracle/oracle-database-operator/controllers/dataguard"
	networkcontroller "github.com/oracle/oracle-database-operator/controllers/network"
	observabilitycontroller "github.com/oracle/oracle-database-operator/controllers/observability"
	privateaiv4controller "github.com/oracle/oracle-database-operator/controllers/privateai"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// init registers all API schemas used by the manager.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(observabilityv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitorv1.AddToScheme(scheme))
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(databasev4.AddToScheme(scheme))
	utilruntime.Must(networkv4.AddToScheme(scheme))
	utilruntime.Must(observabilityv1.AddToScheme(scheme))
	utilruntime.Must(observabilityv4.AddToScheme(scheme))
	utilruntime.Must(privateaiv4.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// main configures and starts the controller manager for the Oracle Database Operator.
func main() {
	metricsAddr, enableLeaderElection := parseFlags()
	configureLogger()

	watchNamespaces, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get watch namespaces")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), buildManagerOptions(metricsAddr, enableLeaderElection, watchNamespaces))
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := setupControllers(mgr, parseReconcileInterval()); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := setupWebhooks(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook")
			os.Exit(1)
		}
	}

	if err := setupIndexes(mgr); err != nil {
		setupLog.Error(err, "unable to create index")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseFlags() (string, bool) {
	var metricsAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(
		&enableLeaderElection,
		"enable-leader-election",
		false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.",
	)
	flag.Parse()

	return metricsAddr, enableLeaderElection
}

func configureLogger() {
	options := &ctrlzap.Options{
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}
	ctrl.SetLogger(ctrlzap.New(func(o *ctrlzap.Options) { *o = *options }))
}

func buildManagerOptions(metricsAddr string, enableLeaderElection bool, watchNamespaces map[string]cache.Config) ctrl.Options {
	disableHTTP2 := func(c *tls.Config) {
		c.NextProtos = []string{"http/1.1"}
	}

	return ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:    metricsAddr,
			SecureServing:  envBoolOrDefault("METRICS_SECURE", true),
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			CertDir:        envOrDefault("METRICS_CERT_DIR", "/metrics-certs"),
			CertName:       envOrDefault("METRICS_CERT_NAME", "tls.crt"),
			KeyName:        envOrDefault("METRICS_KEY_NAME", "tls.key"),
			TLSOpts:        []func(*tls.Config){disableHTTP2},
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
	}
}

type controllerSetupFn func(ctrl.Manager, int64) error

func setupControllers(mgr ctrl.Manager, interval int64) error {
	type controllerRegistration struct {
		name     string
		required bool
		setup    controllerSetupFn
	}

	controllers := []controllerRegistration{
		{name: "AutonomousDatabase", required: true, setup: setupAutonomousDatabaseController},
		{name: "AutonomousDatabaseBackup", required: true, setup: setupAutonomousDatabaseBackupController},
		{name: "AutonomousDatabaseRestore", required: true, setup: setupAutonomousDatabaseRestoreController},
		{name: "AutonomousContainerDatabase", required: true, setup: setupAutonomousContainerDatabaseController},
		{name: "SingleInstanceDatabase", required: true, setup: setupSingleInstanceDatabaseController},
		{name: "ShardingDatabase", required: true, setup: setupShardingDatabaseController},
		{name: "DbcsSystem", required: true, setup: setupDbcsSystemController},
		{name: "OracleRestDataService", required: true, setup: setupOrdsController},
		{name: "OracleRestart", required: true, setup: setupOracleRestartController},
		{name: "PrivateAi", required: true, setup: setupPrivateAiController},
		{name: "TrafficManager", required: true, setup: setupTrafficManagerController},
		{name: "LRPDB", required: true, setup: setupLRPDBController},
		{name: "LREST", required: true, setup: setupLRESTController},
		{name: "DataguardBroker", required: true, setup: setupDataguardBrokerController},
		{name: "OrdsSrvs", required: false, setup: setupOrdsSrvsController},
		{name: "DatabaseObserver", required: true, setup: setupDatabaseObserverController},
		// +kubebuilder:scaffold:builder
		{name: "RacDatabase", required: true, setup: setupRacDatabaseController},
		// +kubebuilder:scaffold:builder
	}

	for _, controller := range controllers {
		if err := controller.setup(mgr, interval); err != nil {
			if controller.required {
				return annotate(controller.name, err)
			}
			setupLog.Error(err, "unable to create controller", "controller", controller.name)
		}
	}

	return nil
}

func setupAutonomousDatabaseController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.AutonomousDatabaseReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("database").WithName("AutonomousDatabase"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabase"),
	}).SetupWithManager(mgr)
}

func setupAutonomousDatabaseBackupController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.AutonomousDatabaseBackupReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseBackup"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseBackup"),
	}).SetupWithManager(mgr)
}

func setupAutonomousDatabaseRestoreController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.AutonomousDatabaseRestoreReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousDatabaseRestore"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousDatabaseRestore"),
	}).SetupWithManager(mgr)
}

func setupAutonomousContainerDatabaseController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.AutonomousContainerDatabaseReconciler{
		KubeClient: mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AutonomousContainerDatabase"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("AutonomousContainerDatabase"),
	}).SetupWithManager(mgr)
}

func setupSingleInstanceDatabaseController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.SingleInstanceDatabaseReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("SingleInstanceDatabase"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("SingleInstanceDatabase"),
	}).SetupWithManager(mgr)
}

func setupShardingDatabaseController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.ShardingDatabaseReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("database").WithName("ShardingDatabase"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ShardingDatabase"),
	}).SetupWithManager(mgr)
}

func setupDbcsSystemController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.DbcsSystemReconciler{
		KubeClient: mgr.GetClient(),
		Logger:     ctrl.Log.WithName("controllers").WithName("database").WithName("DbcsSystem"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("DbcsSystem"),
	}).SetupWithManager(mgr)
}

func setupOrdsController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.OracleRestDataServiceReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("OracleRestDataService"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("OracleRestDataService"),
	}).SetupWithManager(mgr)
}

func setupOracleRestartController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.OracleRestartReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("OracleRestart"),
		Scheme: mgr.GetScheme(),
		Config: mgr.GetConfig(),
	}).SetupWithManager(mgr)
}

func setupPrivateAiController(mgr ctrl.Manager, _ int64) error {
	return (&privateaiv4controller.PrivateAiReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("privateai").WithName("PrivateAi"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("PrivateAi"),
		Config:   mgr.GetConfig(),
	}).SetupWithManager(mgr)
}

func setupTrafficManagerController(mgr ctrl.Manager, _ int64) error {
	return (&networkcontroller.TrafficManagerReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("network").WithName("TrafficManager"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("TrafficManager"),
		Config:   mgr.GetConfig(),
	}).SetupWithManager(mgr)
}

func setupLRPDBController(mgr ctrl.Manager, interval int64) error {
	return (&databasecontroller.LRPDBReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("LRPDB"),
		Interval: time.Duration(interval),
		Recorder: mgr.GetEventRecorderFor("LRPDB"),
	}).SetupWithManager(mgr)
}

func setupLRESTController(mgr ctrl.Manager, interval int64) error {
	return (&databasecontroller.LRESTReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Log:      ctrl.Log.WithName("controllers").WithName("LREST"),
		Interval: time.Duration(interval),
		Recorder: mgr.GetEventRecorderFor("LREST"),
	}).SetupWithManager(mgr)
}

func setupDataguardBrokerController(mgr ctrl.Manager, _ int64) error {
	return (&dataguardcontroller.DataguardBrokerReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("dataguard").WithName("DataguardBroker"),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("DataguardBroker"),
	}).SetupWithManager(mgr)
}

func setupOrdsSrvsController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.OrdsSrvsReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("OrdsSrvs"),
	}).SetupWithManager(mgr)
}

func setupDatabaseObserverController(mgr ctrl.Manager, _ int64) error {
	return (&observabilitycontroller.DatabaseObserverReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("observability").WithName("DatabaseObserver"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("DatabaseObserver"),
	}).SetupWithManager(mgr)
}

func setupRacDatabaseController(mgr ctrl.Manager, _ int64) error {
	return (&databasecontroller.RacDatabaseReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("controllers").WithName("RacDatabase"),
		Scheme:   mgr.GetScheme(),
		Recorder: record.NewFakeRecorder(0), // disable event spams
	}).SetupWithManager(mgr)
}

type webhookSetupFn func(ctrl.Manager) error

func setupWebhooks(mgr ctrl.Manager) error {
	type webhookRegistration struct {
		name       string
		apiVersion string
		deprecated bool
		setup      webhookSetupFn
	}

	webhooks := []webhookRegistration{
		{name: "OracleRestDataService", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1OracleRestDataServiceWebhook},
		{name: "LRPDB", setup: setupV4LRPDBWebhook},
		{name: "LREST", setup: setupV4LRESTWebhook},
		{name: "AutonomousDatabase", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1AutonomousDatabaseWebhook},
		{name: "AutonomousDatabaseBackup", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1AutonomousDatabaseBackupWebhook},
		{name: "AutonomousDatabaseRestore", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1AutonomousDatabaseRestoreWebhook},
		{name: "AutonomousContainerDatabase", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1AutonomousContainerDatabaseWebhook},
		{name: "AutonomousDatabase", setup: setupV4AutonomousDatabaseWebhook},
		{name: "AutonomousDatabaseBackup", setup: setupV4AutonomousDatabaseBackupWebhook},
		{name: "AutonomousDatabaseRestore", setup: setupV4AutonomousDatabaseRestoreWebhook},
		{name: "AutonomousContainerDatabase", setup: setupV4AutonomousContainerDatabaseWebhook},
		{name: "DataguardBroker", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1DataguardBrokerWebhook},
		{name: "DbcsSystem", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1DbcsSystemWebhook},
		{name: "ShardingDatabase", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1ShardingDatabaseWebhook},
		{name: "ShardingDatabase", setup: setupV4ShardingDatabaseWebhook},
		{name: "DatabaseObserver", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1DatabaseObserverWebhook},
		{name: "DbcsSystem", setup: setupV4DbcsSystemWebhook},
		{name: "DatabaseObserver", setup: setupV1DatabaseObserverWebhook},
		{name: "DatabaseObserver", setup: setupV4DatabaseObserverWebhook},
		{name: "SingleInstanceDatabase", apiVersion: "v1alpha1", deprecated: true, setup: setupV1Alpha1SingleInstanceDatabaseWebhook},
		{name: "SingleInstanceDatabase", setup: setupV4SingleInstanceDatabaseWebhook},
		{name: "DataguardBroker", setup: setupV4DataguardBrokerWebhook},
		{name: "OracleRestDataService", setup: setupV4OracleRestDataServiceWebhook},
		{name: "OracleRestart", setup: setupV4OracleRestartWebhook},
		{name: "PrivateAi", setup: setupV4PrivateAiWebhook},
		{name: "TrafficManager", setup: setupV4TrafficManagerWebhook},
		{name: "RacDatabase", setup: setupV4RacDatabaseWebhook},
	}

	deprecatedRegistered := make([]string, 0)
	for _, webhook := range webhooks {
		if webhook.deprecated {
			deprecatedRegistered = append(deprecatedRegistered, webhook.name+"/"+webhook.apiVersion)
		}
		if err := webhook.setup(mgr); err != nil {
			return annotate(webhook.name, err)
		}
	}
	if len(deprecatedRegistered) > 0 {
		setupLog.Info(
			"deprecated webhooks registered for backward compatibility; plan migration to v4",
			"count", len(deprecatedRegistered),
			"registrations", strings.Join(deprecatedRegistered, ","),
		)
	}

	return nil
}

func setupV1Alpha1OracleRestDataServiceWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.OracleRestDataService{}).SetupWebhookWithManager(mgr)
}

func setupV4LRPDBWebhook(mgr ctrl.Manager) error {
	return (&databasev4.LRPDB{}).SetupWebhookWithManager(mgr)
}

func setupV4LRESTWebhook(mgr ctrl.Manager) error {
	return (&databasev4.LREST{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1AutonomousDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.AutonomousDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1AutonomousDatabaseBackupWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1AutonomousDatabaseRestoreWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1AutonomousContainerDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV4AutonomousDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev4.AutonomousDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV4AutonomousDatabaseBackupWebhook(mgr ctrl.Manager) error {
	return (&databasev4.AutonomousDatabaseBackup{}).SetupWebhookWithManager(mgr)
}

func setupV4AutonomousDatabaseRestoreWebhook(mgr ctrl.Manager) error {
	return (&databasev4.AutonomousDatabaseRestore{}).SetupWebhookWithManager(mgr)
}

func setupV4AutonomousContainerDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev4.AutonomousContainerDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1DataguardBrokerWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.DataguardBroker{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1DbcsSystemWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.DbcsSystem{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1ShardingDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.ShardingDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV4ShardingDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev4.ShardingDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1DatabaseObserverWebhook(mgr ctrl.Manager) error {
	return (&observabilityv1alpha1.DatabaseObserver{}).SetupWebhookWithManager(mgr)
}

func setupV4DbcsSystemWebhook(mgr ctrl.Manager) error {
	return (&databasev4.DbcsSystem{}).SetupWebhookWithManager(mgr)
}

func setupV1DatabaseObserverWebhook(mgr ctrl.Manager) error {
	return (&observabilityv1.DatabaseObserver{}).SetupWebhookWithManager(mgr)
}

func setupV4DatabaseObserverWebhook(mgr ctrl.Manager) error {
	return (&observabilityv4.DatabaseObserver{}).SetupWebhookWithManager(mgr)
}

func setupV1Alpha1SingleInstanceDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev1alpha1.SingleInstanceDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV4SingleInstanceDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev4.SingleInstanceDatabase{}).SetupWebhookWithManager(mgr)
}

func setupV4DataguardBrokerWebhook(mgr ctrl.Manager) error {
	return (&databasev4.DataguardBroker{}).SetupWebhookWithManager(mgr)
}

func setupV4OracleRestDataServiceWebhook(mgr ctrl.Manager) error {
	return (&databasev4.OracleRestDataService{}).SetupWebhookWithManager(mgr)
}

func setupV4OracleRestartWebhook(mgr ctrl.Manager) error {
	return (&databasev4.OracleRestart{}).SetupWebhookWithManager(mgr)
}

func setupV4PrivateAiWebhook(mgr ctrl.Manager) error {
	return (&privateaiv4.PrivateAi{}).SetupPrivateAiWebhookWithManager(mgr)
}

func setupV4TrafficManagerWebhook(mgr ctrl.Manager) error {
	return (&networkv4.TrafficManager{}).SetupWebhookWithManager(mgr)
}

func setupV4RacDatabaseWebhook(mgr ctrl.Manager) error {
	return (&databasev4.RacDatabase{}).SetupWebhookWithManager(mgr)
}

func setupIndexes(mgr ctrl.Manager) error {
	indexFunc := func(obj client.Object) []string {
		return []string{obj.(*databasev4.LRPDB).Spec.LRPDBName}
	}
	if err := mgr.GetCache().IndexField(context.TODO(), &databasev4.LRPDB{}, "spec.pdbName", indexFunc); err != nil {
		return annotate("LRPDB", err)
	}
	return nil
}

// parseReconcileInterval reads RECONCILE_INTERVAL and falls back to 15.
func parseReconcileInterval() int64 {
	interval := os.Getenv("RECONCILE_INTERVAL")
	i, err := strconv.ParseInt(interval, 10, 64)
	if err != nil {
		i = 15
		setupLog.Info("setting default reconcile period for database-controller", "Secs", i)
	}
	return i
}

// getWatchNamespace reads WATCH_NAMESPACE and returns the cache configuration per namespace.
func getWatchNamespace() (map[string]cache.Config, error) {
	const watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found || strings.TrimSpace(ns) == "" {
		setupLog.Info(":CLUSTER SCOPED:")
		return nil, nil
	}

	values := strings.Split(ns, ",")
	nsmap := make(map[string]cache.Config, len(values))
	for _, namespace := range values {
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			continue
		}
		nsmap[namespace] = cache.Config{}
	}

	if len(nsmap) == 0 {
		setupLog.Info(":CLUSTER SCOPED:")
		return nil, nil
	}

	setupLog.Info(":NAMESPACE SCOPED:", "WATCH LIST", values)
	return nsmap, nil
}

func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBoolOrDefault(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}

	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return def
	}
}

func annotate(name string, err error) error {
	return fmt.Errorf("%s: %w", name, err)
}
