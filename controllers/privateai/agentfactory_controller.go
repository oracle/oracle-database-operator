/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package privateai

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/go-logr/logr"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	k8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	sharedk8sutil "github.com/oracle/oracle-database-operator/commons/k8sutil"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AgentFactoryReconciler reconciles an AgentFactory object.
type AgentFactoryReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Config   *rest.Config
	Recorder record.EventRecorder
}

const AgentFactoryFinalizer = "privateai.oracle.com/agentfactoryfinalizer"

const (
	agentFactoryPhaseReady    = "Ready"
	agentFactoryPhasePending  = "Pending"
	agentFactoryPhaseBlocked  = "Blocked"
	agentFactoryPhaseUpdating = "Updating"
	agentFactoryPhaseError    = "Error"
)

const (
	agentFactoryDatabasePhaseDisabled = "Disabled"
	agentFactoryDatabasePhasePending  = "Pending"
	agentFactoryDatabasePhaseRunning  = "Running"
	agentFactoryDatabasePhaseReady    = "Ready"
	agentFactoryDatabasePhaseFailed   = "Failed"
	agentFactoryDatabaseReady         = "DatabaseReady"
	agentFactoryWorkloadReady         = "WorkloadReady"

	agentFactoryReasonDatabaseSetupPending     = "DatabaseSetupPending"
	agentFactoryReasonDeploymentNotFound       = "DeploymentNotFound"
	agentFactoryReasonDeploymentGenerationLag  = "DeploymentGenerationLag"
	agentFactoryReasonReplicasUnavailable      = "ReplicasUnavailable"
	agentFactoryReasonWorkloadReadiness        = "WorkloadReadinessFailure"
	agentFactoryReasonWorkloadReady            = "WorkloadReady"
	agentFactoryPublicLoadBalancerReady        = "PublicLoadBalancerReady"
	agentFactoryPrivateLoadBalancerReady       = "PrivateLoadBalancerReady"
	agentFactoryReasonLoadBalancerDisabled     = "Disabled"
	agentFactoryReasonLoadBalancerProvisioning = "Provisioning"
	agentFactoryReasonLoadBalancerReady        = "LoadBalancerReady"
	agentFactoryOutboundTLSReady               = "OutboundTLSReady"
	agentFactoryInboundTLSReady                = "InboundTLSReady"
	agentFactoryReasonInboundTLSRotation       = "InboundTLSRotation"
	agentFactoryReasonInboundTLSImportPending  = "InboundTLSImportPending"
	agentFactoryImageUpgradeReady              = "ImageUpgradeReady"
	agentFactoryImageUpgradePhaseStopping      = "Stopping"
	agentFactoryImageUpgradePhaseRunning       = "Running"
	agentFactoryImageUpgradePhaseSucceeded     = "Succeeded"
	agentFactoryImageUpgradePhaseFailed        = "Failed"
	agentFactoryReasonImageUpgradePending      = "ImageUpgradePending"
	agentFactoryReasonImageUpgradeFailed       = "ImageUpgradeFailed"
)

type agentFactoryWorkloadStatus struct {
	Ready              bool
	Reason             string
	Message            string
	DesiredReplicas    int32
	Replicas           int32
	ReadyReplicas      int32
	AvailableReplicas  int32
	Generation         int64
	ObservedGeneration int64
}

// +kubebuilder:rbac:groups=privateai.oracle.com,resources=agentfactories,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=agentfactories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=agentfactories/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves AgentFactory resources toward the desired state.
func (r *AgentFactoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithName("AgentFactory").WithValues("namespace", req.Namespace, "name", req.Name)

	inst := &privateaiv4.AgentFactory{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found")
			return requeueN, nil
		}
		return ctrl.Result{}, err
	}
	previousPhase := inst.Status.Phase
	previousReconcileCondition := copyAgentFactoryCondition(meta.FindStatusCondition(inst.Status.Conditions, privateaiv4.ReconcileCompelete))
	log.V(1).Info("AgentFactory reconciliation started", "generation", inst.Generation, "phase", inst.Status.Phase)

	if done, err := r.manageAgentFactoryDeletion(ctx, inst); done || err != nil {
		return requeueN, err
	}

	if err := r.validateAgentFactory(inst); err != nil {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               privateaiv4.ReconcileBlocked,
			Status:             metav1.ConditionTrue,
			Reason:             "ValidationFailed",
			Message:            err.Error(),
			ObservedGeneration: inst.Generation,
		})
		inst.Status.Phase = agentFactoryPhaseBlocked
		r.logAgentFactoryStatusTransition(log, inst, previousPhase, previousReconcileCondition)
		if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
			return ctrl.Result{}, err
		}
		return requeueN, nil
	}

	if err := r.reconcileAgentFactoryRuntimeConfig(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if err := r.reconcileAgentFactorySecret(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if err := r.reconcileAgentFactoryOutboundTLS(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if err := r.reconcileAgentFactoryInboundTLS(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if err := r.reconcileAgentFactoryStorage(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	databaseReady, err := r.reconcileAgentFactoryDatabaseSetup(ctx, inst)
	if err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if !databaseReady {
		inst.Status.Phase = agentFactoryPhasePending
		inst.Status.AppState = "pending"
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryDatabaseReady,
			Status:             metav1.ConditionFalse,
			Reason:             agentFactoryReasonDatabaseSetupPending,
			Message:            "AgentFactory database setup is pending",
			ObservedGeneration: inst.Generation,
		})
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryWorkloadReady,
			Status:             metav1.ConditionUnknown,
			Reason:             agentFactoryReasonDatabaseSetupPending,
			Message:            "AgentFactory workload readiness is not evaluated until database setup completes",
			ObservedGeneration: inst.Generation,
		})
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               privateaiv4.ReconcileCompelete,
			Status:             metav1.ConditionFalse,
			Reason:             agentFactoryReasonDatabaseSetupPending,
			Message:            "AgentFactory database setup is pending",
			ObservedGeneration: inst.Generation,
		})
		log.Info("AgentFactory database setup is pending")
		r.logAgentFactoryStatusTransition(log, inst, previousPhase, previousReconcileCondition)
		if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
			return ctrl.Result{}, err
		}
		return requeueY, nil
	}
	imageUpgradeHandled, err := r.reconcileAgentFactoryImageUpgrade(ctx, inst)
	if err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if imageUpgradeHandled {
		return r.finishAgentFactoryImageUpgradeReconcile(ctx, inst)
	}
	if err := r.reconcileAgentFactoryServices(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if err := r.reconcileAgentFactoryWorkload(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}

	workload, err := r.refreshAgentFactoryWorkloadStatus(ctx, inst)
	if err != nil {
		if apierrors.IsNotFound(err) {
			inst.Status.Phase = agentFactoryPhasePending
			inst.Status.AppState = "pending"
			inst.Status.Replicas = 0
			inst.Status.ReadyReplicas = 0
			workload = agentFactoryWorkloadStatus{
				Reason:  agentFactoryReasonDeploymentNotFound,
				Message: fmt.Sprintf("AgentFactory Deployment %q is not available yet", inst.Name),
			}
			r.setAgentFactoryCondition(inst, metav1.Condition{
				Type:               agentFactoryWorkloadReady,
				Status:             metav1.ConditionFalse,
				Reason:             workload.Reason,
				Message:            workload.Message,
				ObservedGeneration: inst.Generation,
			})
			r.setAgentFactoryCondition(inst, metav1.Condition{
				Type:               privateaiv4.ReconcileCompelete,
				Status:             metav1.ConditionFalse,
				Reason:             workload.Reason,
				Message:            workload.Message,
				ObservedGeneration: inst.Generation,
			})
			log.Info("AgentFactory workload is pending", "reason", workload.Reason, "message", workload.Message)
			r.logAgentFactoryStatusTransition(log, inst, previousPhase, previousReconcileCondition)
			if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
				return ctrl.Result{}, err
			}
			return requeueY, nil
		}
		return r.agentFactoryError(ctx, inst, err)
	}
	if !workload.Ready {
		inst.Status.Phase = agentFactoryPhasePending
		inst.Status.AppState = "pending"
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryWorkloadReady,
			Status:             metav1.ConditionFalse,
			Reason:             workload.Reason,
			Message:            workload.Message,
			ObservedGeneration: inst.Generation,
		})
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               privateaiv4.ReconcileCompelete,
			Status:             metav1.ConditionFalse,
			Reason:             workload.Reason,
			Message:            workload.Message,
			ObservedGeneration: inst.Generation,
		})
		log.Info("AgentFactory workload is pending",
			"reason", workload.Reason,
			"message", workload.Message,
			"desiredReplicas", workload.DesiredReplicas,
			"replicas", workload.Replicas,
			"readyReplicas", workload.ReadyReplicas,
			"availableReplicas", workload.AvailableReplicas,
			"generation", workload.Generation,
			"observedGeneration", workload.ObservedGeneration)
		r.logAgentFactoryStatusTransition(log, inst, previousPhase, previousReconcileCondition)
		if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
			return ctrl.Result{}, err
		}
		return requeueY, nil
	}
	inboundTLSRestarted, err := r.reconcileAgentFactoryInboundTLSImport(ctx, inst)
	if err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if inboundTLSRestarted || agentFactoryInboundTLSImportPending(inst) {
		return r.reconcileAgentFactoryInboundTLSPending(ctx, inst, inboundTLSRestarted)
	}
	if err := r.reconcileAgentFactoryOutboundTLSImport(ctx, inst); err != nil {
		return r.agentFactoryError(ctx, inst, err)
	}
	if inst.Status.ImageUpgrade.Phase == agentFactoryImageUpgradePhaseSucceeded {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryImageUpgradeReady,
			Status:             metav1.ConditionTrue,
			Reason:             agentFactoryImageUpgradePhaseSucceeded,
			Message:            inst.Status.ImageUpgrade.Message,
			ObservedGeneration: inst.Generation,
		})
	}
	if inst.Status.ImageUpgrade.Phase == agentFactoryImageUpgradePhaseSucceeded {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               privateaiv4.ReconcileError,
			Status:             metav1.ConditionFalse,
			Reason:             agentFactoryImageUpgradePhaseSucceeded,
			Message:            inst.Status.ImageUpgrade.Message,
			ObservedGeneration: inst.Generation,
		})
	}
	inst.Status.Phase = agentFactoryPhaseReady
	inst.Status.AppState = "ready"
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryWorkloadReady,
		Status:             metav1.ConditionTrue,
		Reason:             agentFactoryReasonWorkloadReady,
		Message:            workload.Message,
		ObservedGeneration: inst.Generation,
	})
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryDatabaseReady,
		Status:             metav1.ConditionTrue,
		Reason:             "DatabaseReady",
		Message:            "AgentFactory database setup and workload readiness checks succeeded",
		ObservedGeneration: inst.Generation,
	})
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               privateaiv4.ReconcileCompelete,
		Status:             metav1.ConditionTrue,
		Reason:             privateaiv4.ReconcileCompleteReason,
		Message:            "AgentFactory resources reconciled",
		ObservedGeneration: inst.Generation,
	})
	log.Info("AgentFactory is ready",
		"desiredReplicas", workload.DesiredReplicas,
		"replicas", workload.Replicas,
		"readyReplicas", workload.ReadyReplicas,
		"availableReplicas", workload.AvailableReplicas,
		"generation", workload.Generation,
		"observedGeneration", workload.ObservedGeneration)
	r.logAgentFactoryStatusTransition(log, inst, previousPhase, previousReconcileCondition)
	if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
		return ctrl.Result{}, err
	}
	return requeueN, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentFactoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&privateaiv4.AgentFactory{}).
		Owns(&batchv1.Job{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.agentFactoryRequestsForConfigMap(ctx, obj)
		})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.agentFactoryRequestsForSecret(ctx, obj)
		})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *AgentFactoryReconciler) validateAgentFactory(inst *privateaiv4.AgentFactory) error {
	if strings.TrimSpace(agentFactoryImageName(inst)) == "" {
		return fmt.Errorf("spec.runtime.image.name is required")
	}
	if err := validateAgentFactoryDatabase(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryRuntimeInputs(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryWorkerPlacement(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryUpgrade(inst); err != nil {
		return err
	}
	secret := agentFactorySecretSpec(inst)
	if secret == nil || strings.TrimSpace(secret.Name) == "" {
		return fmt.Errorf("spec.security.secret.name is required")
	}
	if strings.TrimSpace(agentFactoryUIAdminPasswordKey(secret)) == "" || strings.TrimSpace(agentFactoryDBPasswordKey(secret)) == "" {
		return fmt.Errorf("spec.security.secret uiAdminPasswordKey and dbPasswordKey are required")
	}
	if strings.EqualFold(agentFactoryConnectionType(inst), "Wallet") && strings.TrimSpace(agentFactoryWalletZipKey(secret)) == "" {
		return fmt.Errorf("spec.security.secret.walletZipKey is required for Wallet database connectionType")
	}
	if !strings.EqualFold(agentFactoryConnectionType(inst), "Wallet") && strings.TrimSpace(secret.WalletPasswordKey) != "" {
		return fmt.Errorf("spec.security.secret.walletPasswordKey is only supported for Wallet database connectionType")
	}
	if err := validateAgentFactoryInboundTLS(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryVolumes(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryLLM(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryOutboundTLS(inst); err != nil {
		return err
	}
	if err := validateAgentFactoryLoadBalancerPolicy("publicLoadBalancer", agentFactoryPublicLB(inst)); err != nil {
		return err
	}
	if err := validateAgentFactoryLoadBalancerPolicy("privateLoadBalancer", agentFactoryPrivateLB(inst)); err != nil {
		return err
	}
	if lb := agentFactoryPrivateLB(inst); lb != nil {
		if value, ok := lb.Annotations["service.beta.kubernetes.io/oci-load-balancer-internal"]; ok && strings.EqualFold(value, "false") {
			return fmt.Errorf("privateLoadBalancer cannot set service.beta.kubernetes.io/oci-load-balancer-internal=false")
		}
	}
	return nil
}

func validateAgentFactoryUpgrade(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Upgrade == nil {
		return nil
	}
	strategy := strings.TrimSpace(inst.Spec.Upgrade.Strategy)
	switch strings.ToLower(strategy) {
	case "":
		inst.Spec.Upgrade.Strategy = "RollingUpdate"
	case "rollingupdate":
		inst.Spec.Upgrade.Strategy = "RollingUpdate"
	case "recreate":
		inst.Spec.Upgrade.Strategy = "Recreate"
	case "imageupgrade":
		inst.Spec.Upgrade.Strategy = "ImageUpgrade"
	default:
		return fmt.Errorf("spec.upgrade.strategy must be RollingUpdate, Recreate, or ImageUpgrade")
	}
	if inst.Spec.Upgrade.ImageUpgrade != nil {
		if inst.Spec.Upgrade.Strategy != "ImageUpgrade" {
			return fmt.Errorf("spec.upgrade.imageUpgrade is only supported when spec.upgrade.strategy is ImageUpgrade")
		}
		if deadline := inst.Spec.Upgrade.ImageUpgrade.ActiveDeadlineSeconds; deadline != nil && *deadline < 1 {
			return fmt.Errorf("spec.upgrade.imageUpgrade.activeDeadlineSeconds must be greater than zero")
		}
	}
	return nil
}

func validateAgentFactoryDatabase(inst *privateaiv4.AgentFactory) error {
	db := inst.Spec.Database
	if db == nil {
		return fmt.Errorf("spec.database is required")
	}
	connectionType := strings.ToLower(strings.TrimSpace(db.ConnectionType))
	switch connectionType {
	case "details":
		db.ConnectionType = "Details"
	case "wallet":
		db.ConnectionType = "Wallet"
	default:
		return fmt.Errorf("spec.database.connectionType must be Details or Wallet")
	}
	if strings.TrimSpace(db.Username) == "" {
		return fmt.Errorf("spec.database.username is required")
	}
	if db.AirGapped == nil {
		return fmt.Errorf("spec.database.airGapped is required")
	}
	if healthPath := strings.TrimSpace(db.HealthPath); healthPath != "" && !strings.HasPrefix(healthPath, "/") {
		return fmt.Errorf("spec.database.healthPath must be an absolute HTTP path")
	}
	if databaseBootstrapEnabledSpec(db.Bootstrap) {
		if db.Bootstrap.AdminCredentials == nil || strings.TrimSpace(db.Bootstrap.AdminCredentials.SecretName) == "" {
			return fmt.Errorf("spec.database.bootstrap.adminCredentials.secretName is required when bootstrap is enabled")
		}
		privilege := strings.ToUpper(strings.TrimSpace(db.Bootstrap.AdminCredentials.Privilege))
		if privilege == "" {
			privilege = "NORMAL"
		}
		switch privilege {
		case "NORMAL", "SYSDBA":
			db.Bootstrap.AdminCredentials.Privilege = privilege
		default:
			return fmt.Errorf("spec.database.bootstrap.adminCredentials.privilege must be NORMAL or SYSDBA")
		}
		if cleanupPolicy := strings.TrimSpace(db.Bootstrap.CleanupPolicy); cleanupPolicy != "" && cleanupPolicy != "Retain" && cleanupPolicy != "DeleteOnSuccess" {
			return fmt.Errorf("spec.database.bootstrap.cleanupPolicy must be Retain or DeleteOnSuccess")
		}
	}
	if connectionType == "details" {
		if db.Details == nil {
			return fmt.Errorf("spec.database.details is required for Details connectionType")
		}
		details := db.Details
		protocol := strings.ToLower(strings.TrimSpace(details.Protocol))
		switch protocol {
		case "tcp":
			details.Protocol = "TCP"
		case "tcps":
			details.Protocol = "TCPS"
		default:
			return fmt.Errorf("spec.database.details.protocol must be TCP or TCPS for Details connectionType")
		}
		if strings.TrimSpace(details.Host) == "" {
			return fmt.Errorf("spec.database.details.host is required for Details connectionType")
		}
		if details.Port < 1 || details.Port > 65535 {
			return fmt.Errorf("spec.database.details.port must be between 1 and 65535 for Details connectionType")
		}
		if strings.TrimSpace(details.ServiceName) == "" {
			return fmt.Errorf("spec.database.details.serviceName is required for Details connectionType")
		}
		return nil
	}
	if *db.AirGapped {
		return fmt.Errorf("spec.database.airGapped must be false for Wallet connectionType")
	}
	if db.Wallet == nil {
		return fmt.Errorf("spec.database.wallet is required for Wallet connectionType")
	}
	if db.Wallet.HasOciCertificates == nil {
		return fmt.Errorf("spec.database.wallet.hasOciCertificates is required for Wallet connectionType")
	}
	if strings.TrimSpace(db.Wallet.SelectedTnsAlias) == "" {
		return fmt.Errorf("spec.database.wallet.selectedTnsAlias is required for Wallet connectionType")
	}
	return nil
}

// validateAgentFactoryRuntimeInputs validates the image bootstrap inputs and
// normalizes accepted values before generated runtime configuration is built.
func validateAgentFactoryRuntimeInputs(inst *privateaiv4.AgentFactory) error {
	if err := validateAgentFactoryMode(inst); err != nil {
		return err
	}
	if inst.Spec.Configuration == nil {
		return fmt.Errorf("spec.configuration is required")
	}
	if inst.Spec.Configuration.SilentInstall != nil && !*inst.Spec.Configuration.SilentInstall {
		return fmt.Errorf("spec.configuration.silentInstall must be true for image-based AgentFactory deployments")
	}
	admin, err := normalizeAgentFactoryAdminUsername(inst.Spec.Configuration.UIAdminUsername)
	if err != nil {
		return fmt.Errorf("spec.configuration.uiAdminUsername: %w", err)
	}
	inst.Spec.Configuration.UIAdminUsername = admin
	certIP, err := normalizeAgentFactoryCertIP(inst.Spec.Configuration.CertIPAddress)
	if err != nil {
		return fmt.Errorf("spec.configuration.certIpAddress: %w", err)
	}
	inst.Spec.Configuration.CertIPAddress = certIP
	swagger := false
	if inst.Spec.Configuration.EnableSwagger != nil {
		swagger = *inst.Spec.Configuration.EnableSwagger
	}
	inst.Spec.Configuration.EnableSwagger = &swagger
	statePollInterval := normalizeAgentFactoryTimeout(inst.Spec.Configuration.StatePollIntervalSeconds, 5)
	inst.Spec.Configuration.StatePollIntervalSeconds = &statePollInterval
	apiReadyTimeout := normalizeAgentFactoryTimeout(inst.Spec.Configuration.APIReadyTimeoutSeconds, 600)
	inst.Spec.Configuration.APIReadyTimeoutSeconds = &apiReadyTimeout
	installTimeout := normalizeAgentFactoryTimeout(inst.Spec.Configuration.InstallTimeoutSeconds, 1800)
	inst.Spec.Configuration.InstallTimeoutSeconds = &installTimeout
	readyTimeout := normalizeAgentFactoryTimeout(inst.Spec.Configuration.ReadyTimeoutSeconds, 600)
	inst.Spec.Configuration.ReadyTimeoutSeconds = &readyTimeout
	httpProxy, err := normalizeAgentFactoryProxy(inst.Spec.Configuration.HTTPProxy)
	if err != nil {
		return fmt.Errorf("spec.configuration.httpProxy: %w", err)
	}
	inst.Spec.Configuration.HTTPProxy = httpProxy
	httpsProxy, err := normalizeAgentFactoryProxy(inst.Spec.Configuration.HTTPSProxy)
	if err != nil {
		return fmt.Errorf("spec.configuration.httpsProxy: %w", err)
	}
	inst.Spec.Configuration.HTTPSProxy = httpsProxy
	noProxy, err := normalizeAgentFactoryNoProxy(inst.Spec.Configuration.NoProxy, false)
	if err != nil {
		return fmt.Errorf("spec.configuration.noProxy: %w", err)
	}
	inst.Spec.Configuration.NoProxy = noProxy
	effectiveNoProxy := noProxy
	if inst.Spec.Runtime != nil {
		for i := range inst.Spec.Runtime.Env {
			env := &inst.Spec.Runtime.Env[i]
			switch env.Name {
			case "AGENT_FACTORY_SILENT_INSTALL":
				if !isAgentFactoryTrueValue(env.Value) {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_SILENT_INSTALL must be true for image-based AgentFactory deployments")
				}
				env.Value = "true"
			case "AGENT_FACTORY_ADMIN_USERNAME":
				admin, err := normalizeAgentFactoryAdminUsername(env.Value)
				if err != nil {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_ADMIN_USERNAME: %w", err)
				}
				env.Value = admin
			case "AGENT_FACTORY_CERT_IP_ADDRESS":
				certIP, err := normalizeAgentFactoryCertIP(env.Value)
				if err != nil {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_CERT_IP_ADDRESS: %w", err)
				}
				env.Value = certIP
			case "AGENT_FACTORY_ENABLE_SWAGGER":
				env.Value = canonicalAgentFactoryBool(env.Value)
			case "AGENT_FACTORY_STATE_POLL_INTERVAL_SECONDS":
				env.Value = strconv.FormatInt(int64(parseAgentFactoryTimeout(env.Value, 5)), 10)
			case "AGENT_FACTORY_API_READY_TIMEOUT_SECONDS":
				env.Value = strconv.FormatInt(int64(parseAgentFactoryTimeout(env.Value, 600)), 10)
			case "AGENT_FACTORY_INSTALL_TIMEOUT_SECONDS":
				env.Value = strconv.FormatInt(int64(parseAgentFactoryTimeout(env.Value, 1800)), 10)
			case "AGENT_FACTORY_READY_TIMEOUT_SECONDS":
				env.Value = strconv.FormatInt(int64(parseAgentFactoryTimeout(env.Value, 600)), 10)
			case "AGENT_FACTORY_HTTP_PROXY":
				httpProxy, err = normalizeAgentFactoryProxy(env.Value)
				if err != nil {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_HTTP_PROXY: %w", err)
				}
				env.Value = httpProxy
			case "AGENT_FACTORY_HTTPS_PROXY":
				httpsProxy, err = normalizeAgentFactoryProxy(env.Value)
				if err != nil {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_HTTPS_PROXY: %w", err)
				}
				env.Value = httpsProxy
			case "AGENT_FACTORY_NO_PROXY":
				effectiveNoProxy, err = normalizeAgentFactoryNoProxy(env.Value, false)
				if err != nil {
					return fmt.Errorf("spec.runtime.env AGENT_FACTORY_NO_PROXY: %w", err)
				}
				env.Value = effectiveNoProxy
			case "AGENT_FACTORY_DB_WALLET_PASSWORD":
				return fmt.Errorf("spec.runtime.env AGENT_FACTORY_DB_WALLET_PASSWORD is unsupported; configure spec.security.secret.walletPasswordKey")
			}
		}
	}
	if httpProxy != "" || httpsProxy != "" {
		if _, err := normalizeAgentFactoryNoProxy(effectiveNoProxy, true); err != nil {
			return fmt.Errorf("proxy configuration requires noProxy to include localhost, 127.0.0.1, and ::1: %w", err)
		}
	}
	return nil
}

func validateAgentFactoryWorkerPlacement(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Runtime == nil {
		return nil
	}
	runtimeSpec := inst.Spec.Runtime
	if len(runtimeSpec.WorkerNodes) > 0 && len(runtimeSpec.WorkerNodeSelector) > 0 {
		return fmt.Errorf("spec.runtime.workerNodes and spec.runtime.workerNodeSelector are mutually exclusive")
	}
	for key, value := range runtimeSpec.WorkerNodeSelector {
		if errors := validationutil.IsQualifiedName(key); len(errors) > 0 {
			return fmt.Errorf("spec.runtime.workerNodeSelector key %q is invalid: %s", key, strings.Join(errors, "; "))
		}
		if errors := validationutil.IsValidLabelValue(value); len(errors) > 0 {
			return fmt.Errorf("spec.runtime.workerNodeSelector value for key %q is invalid: %s", key, strings.Join(errors, "; "))
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("spec.runtime.workerNodeSelector value for key %q must not be empty", key)
		}
	}
	return nil
}

func normalizeAgentFactoryTimeout(value *int32, fallback int32) int32 {
	if value != nil && *value > 0 {
		return *value
	}
	return fallback
}

func parseAgentFactoryTimeout(value string, fallback int32) int32 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed < 1 {
		return fallback
	}
	return int32(parsed)
}

func normalizeAgentFactoryProxy(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("must be an absolute http or https proxy URI without credentials")
	}
	return value, nil
}

func normalizeAgentFactoryNoProxy(value string, requireLoopback bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if requireLoopback {
			return "", fmt.Errorf("must include localhost, 127.0.0.1, and ::1")
		}
		return "", nil
	}
	parts := strings.Split(value, ",")
	seen := map[string]bool{}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return "", fmt.Errorf("must be a comma-separated host, domain, or IP list")
		}
		seen[parts[i]] = true
	}
	if requireLoopback && (!seen["localhost"] || !seen["127.0.0.1"] || !seen["::1"]) {
		return "", fmt.Errorf("must include localhost, 127.0.0.1, and ::1")
	}
	return strings.Join(parts, ","), nil
}

func canonicalAgentFactoryBool(value string) string {
	if isAgentFactoryTrueValue(value) {
		return "true"
	}
	return "false"
}

func normalizeAgentFactoryCertIP(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return "", fmt.Errorf("must be a valid IPv4 or IPv6 literal without CIDR, brackets, port, hostname, or list notation")
	}
	return ip.String(), nil
}

func isAgentFactoryTrueValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func normalizeAgentFactoryAdminUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(norm.NFC.String(value)))
	if value == "" {
		return "", fmt.Errorf("must be a valid email address")
	}
	if len(value) > 254 {
		return "", fmt.Errorf("must be at most 254 characters")
	}
	for _, r := range value {
		if r <= 0x1f || r == 0x7f || unicode.IsControl(r) {
			return "", fmt.Errorf("must not contain ASCII control characters")
		}
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || parsed.Name != "" {
		return "", fmt.Errorf("must be a valid email address")
	}
	return value, nil
}

// validateAgentFactoryMode enforces the mode supported by image-based
// AgentFactory deployments. The image bootstrap only supports prod.
func validateAgentFactoryMode(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Configuration != nil {
		mode := strings.TrimSpace(inst.Spec.Configuration.Mode)
		if mode != "" {
			if !strings.EqualFold(mode, "prod") {
				return fmt.Errorf("spec.configuration.mode must be prod for image-based AgentFactory deployments")
			}
			inst.Spec.Configuration.Mode = "prod"
		}
	}
	if inst.Spec.Runtime == nil {
		return nil
	}
	for i := range inst.Spec.Runtime.Env {
		env := &inst.Spec.Runtime.Env[i]
		if env.Name != "AGENT_FACTORY_MODE" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(env.Value), "prod") {
			return fmt.Errorf("spec.runtime.env AGENT_FACTORY_MODE must be prod for image-based AgentFactory deployments")
		}
		env.Value = "prod"
	}
	return nil
}

func validateAgentFactoryLLM(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Configuration == nil || inst.Spec.Configuration.LLM == nil {
		return nil
	}
	llm := inst.Spec.Configuration.LLM
	provider := strings.ToLower(strings.TrimSpace(llm.Provider))
	switch provider {
	case "", "oci", "instance_principal", "api_key", "vllm", "openai", "openai_compatible", "private_ai_services", "ollama", "google":
	default:
		llm.Provider = ""
		return nil
	}
	if provider == "instance_principal" || provider == "api_key" {
		provider = "oci"
	}
	if provider == "" {
		llm.Provider = ""
		return nil
	}
	llm.Provider = provider
	name := strings.TrimSpace(llm.Name)
	if name == "" {
		name = "llm_model_entry"
	}
	if len(name) > 255 {
		return fmt.Errorf("configuration.llm.name must be at most 255 characters")
	}
	llm.Name = name
	modelID := func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			llm.Provider = ""
			return nil
		}
		if len(value) > 255 {
			return fmt.Errorf("configuration.llm modelId must be at most 255 characters")
		}
		return nil
	}
	file := func(name, value string, required bool) error {
		value = strings.TrimSpace(value)
		if value == "" {
			if required {
				return fmt.Errorf("configuration.llm.%s is required", name)
			}
			return nil
		}
		if strings.IndexAny(value, "\\\t\r\n") >= 0 || value == "." || value == ".." || (!strings.HasPrefix(value, "/") && strings.Contains(value, "/")) {
			return fmt.Errorf("configuration.llm.%s must be an absolute path or filename relative to the runtime secrets directory", name)
		}
		return nil
	}
	missing := func(value string) bool { return strings.TrimSpace(value) == "" }
	switch provider {
	case "oci":
		if llm.OCI == nil || missing(llm.OCI.ModelID) || missing(llm.OCI.ServiceEndpoint) || missing(llm.OCI.CompartmentID) {
			return modelID("")
		}
		if err := modelID(llm.OCI.ModelID); err != nil {
			return err
		}
		apiKey := llm.OCI.UserOCID != "" || llm.OCI.FingerprintFile != "" || llm.OCI.TenancyOCID != "" || llm.OCI.Region != "" || llm.OCI.PrivateKeyFile != ""
		if apiKey {
			if missing(llm.OCI.UserOCID) || missing(llm.OCI.FingerprintFile) || missing(llm.OCI.TenancyOCID) || missing(llm.OCI.Region) {
				return fmt.Errorf("configuration.llm.oci API-key fields userOcid, fingerprintFile, tenancyOcid, and region are required together")
			}
			if err := file("oci.fingerprintFile", llm.OCI.FingerprintFile, false); err != nil {
				return err
			}
			if err := file("oci.privateKeyFile", llm.OCI.PrivateKeyFile, false); err != nil {
				return err
			}
		}
	case "vllm":
		if llm.VLLM == nil || missing(llm.VLLM.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.VLLM.ModelID); err != nil {
			return err
		}
		if missing(llm.VLLM.Host) || llm.VLLM.Port < 1 || llm.VLLM.Port > 65535 {
			return fmt.Errorf("configuration.llm.vllm host and port are required")
		}
		return file("vllm.apiKeyFile", llm.VLLM.APIKeyFile, false)
	case "openai":
		if llm.OpenAI == nil || missing(llm.OpenAI.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.OpenAI.ModelID); err != nil {
			return err
		}
		return file("openai.apiKeyFile", func() string {
			if llm.OpenAI == nil {
				return ""
			}
			return llm.OpenAI.APIKeyFile
		}(), true)
	case "openai_compatible":
		if llm.OpenAICompatible == nil || missing(llm.OpenAICompatible.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.OpenAICompatible.ModelID); err != nil {
			return err
		}
		if missing(llm.OpenAICompatible.BaseURL) {
			return fmt.Errorf("configuration.llm.openaiCompatible baseUrl is required")
		}
		return file("openaiCompatible.apiKeyFile", llm.OpenAICompatible.APIKeyFile, false)
	case "private_ai_services":
		if llm.PrivateAIServices == nil || missing(llm.PrivateAIServices.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.PrivateAIServices.ModelID); err != nil {
			return err
		}
		if missing(llm.PrivateAIServices.BaseURL) {
			return fmt.Errorf("configuration.llm.privateAiServices baseUrl is required")
		}
		if err := file("privateAiServices.apiKeyFile", llm.PrivateAIServices.APIKeyFile, false); err != nil {
			return err
		}
	case "ollama":
		if llm.Ollama == nil || missing(llm.Ollama.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.Ollama.ModelID); err != nil {
			return err
		}
		if missing(llm.Ollama.Host) || llm.Ollama.Port < 1 || llm.Ollama.Port > 65535 {
			return fmt.Errorf("configuration.llm.ollama host and port are required")
		}
	case "google":
		if llm.Google == nil || missing(llm.Google.ModelID) {
			return modelID("")
		}
		if err := modelID(llm.Google.ModelID); err != nil {
			return err
		}
		credentials := strings.ToUpper(strings.TrimSpace(llm.Google.CredentialsType))
		llm.Google.CredentialsType = credentials
		switch credentials {
		case "API_KEY":
			return file("google.apiKeyFile", llm.Google.APIKeyFile, true)
		case "GOOGLE_SERVICE_ACCOUNT":
			if missing(llm.Google.Region) {
				return fmt.Errorf("configuration.llm.google region is required for GOOGLE_SERVICE_ACCOUNT")
			}
			return file("google.serviceAccountFile", llm.Google.ServiceAccountFile, true)
		default:
			return fmt.Errorf("configuration.llm.google credentialsType must be API_KEY or GOOGLE_SERVICE_ACCOUNT")
		}
	}
	return nil
}

func validateAgentFactoryOutboundTLS(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Security == nil || inst.Spec.Security.OutboundTLS == nil {
		return nil
	}
	bundle := inst.Spec.Security.OutboundTLS.CABundle
	if bundle == nil {
		return fmt.Errorf("spec.security.outboundTLS.caBundle is required when outboundTLS is provided")
	}
	if strings.TrimSpace(bundle.SecretName) == "" {
		return fmt.Errorf("spec.security.outboundTLS.caBundle.secretName is required")
	}
	key := strings.TrimSpace(bundle.Key)
	if key == "" {
		key = "ca.crt"
		bundle.Key = key
	}
	if key == "." || key == ".." || strings.ContainsAny(key, "/\\\t\r\n") {
		return fmt.Errorf("spec.security.outboundTLS.caBundle.key must be a Secret key without path separators")
	}
	return nil
}

func agentFactoryOutboundCABundleKey(bundle *privateaiv4.AgentFactoryOutboundCABundleSpec) string {
	if bundle != nil && strings.TrimSpace(bundle.Key) != "" {
		return strings.TrimSpace(bundle.Key)
	}
	return "ca.crt"
}

func validateAgentFactoryLoadBalancerPolicy(name string, lb *privateaiv4.PaiLoadBalancerSpec) error {
	if lb == nil {
		return nil
	}
	policy := strings.TrimSpace(lb.ExternalTrafficPolicy)
	if policy == "" || strings.EqualFold(policy, "Local") || strings.EqualFold(policy, "Cluster") {
		return nil
	}
	return fmt.Errorf("%s.externalTrafficPolicy must be Local or Cluster", name)
}

func validateAgentFactoryVolumes(inst *privateaiv4.AgentFactory) error {
	seenNames := map[string]bool{}
	seenMounts := map[string]bool{}
	hasMount := false
	for _, volume := range agentFactoryVolumeSpecs(inst) {
		if strings.TrimSpace(volume.Name) == "" || strings.TrimSpace(volume.MountPath) == "" {
			return fmt.Errorf("storage volume name and mountPath are required")
		}
		if seenNames[volume.Name] {
			return fmt.Errorf("storage volume name %q is duplicated", volume.Name)
		}
		if seenMounts[volume.MountPath] {
			return fmt.Errorf("storage volume mountPath %q is duplicated", volume.MountPath)
		}
		seenNames[volume.Name] = true
		seenMounts[volume.MountPath] = true
		if volume.MountPath == aicommons.AgentFactoryDefaultMountPath {
			hasMount = true
		}
		sourceCount := 0
		if volume.Source.ExistingPVC != nil {
			sourceCount++
		}
		if volume.Source.PVC != nil {
			sourceCount++
			if strings.TrimSpace(volume.Source.PVC.Size) == "" {
				return fmt.Errorf("storage volume %q pvc.size is required", volume.Name)
			}
		}
		if volume.Source.VolumeClaimTemplate != nil {
			return fmt.Errorf("storage volume %q volumeClaimTemplate is not supported for AgentFactory Deployment; use pvc or existingPVC", volume.Name)
		}
		if volume.Source.EmptyDir != nil {
			sourceCount++
			if volume.MountPath == aicommons.AgentFactoryDefaultMountPath {
				return fmt.Errorf("%s cannot use emptyDir", aicommons.AgentFactoryDefaultMountPath)
			}
		}
		if sourceCount != 1 {
			return fmt.Errorf("storage volume %q must set exactly one of existingPVC, pvc, or emptyDir", volume.Name)
		}
	}
	if !hasMount {
		return fmt.Errorf("storage must include a volume mounted at %s", aicommons.AgentFactoryDefaultMountPath)
	}
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryRuntimeConfig(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	desired := aicommons.BuildAgentFactoryRuntimeConfigMap(inst)
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return err
	}
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return err
		}
		inst.Status.RuntimeConfig.Name = desired.Name
		inst.Status.RuntimeConfig.ResourceVersion = desired.ResourceVersion
		return nil
	}
	if err != nil {
		return err
	}
	updated := false
	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if found.Data == nil {
		found.Data = map[string]string{}
	}
	if found.Data[aicommons.AgentFactoryRuntimeConfigKey] != desired.Data[aicommons.AgentFactoryRuntimeConfigKey] {
		found.Data[aicommons.AgentFactoryRuntimeConfigKey] = desired.Data[aicommons.AgentFactoryRuntimeConfigKey]
		updated = true
	}
	if updated {
		if err := r.Update(ctx, found); err != nil {
			return err
		}
	}
	inst.Status.RuntimeConfig.Name = found.Name
	inst.Status.RuntimeConfig.ResourceVersion = found.ResourceVersion
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactorySecret(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	secretSpec := agentFactorySecretSpec(inst)
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: strings.TrimSpace(secretSpec.Name), Namespace: inst.Namespace}, secret); err != nil {
		return err
	}
	uiAdminKey := agentFactoryUIAdminPasswordKey(secretSpec)
	dbKey := agentFactoryDBPasswordKey(secretSpec)
	walletKey := agentFactoryWalletZipKey(secretSpec)
	_, hasUIAdmin := secret.Data[uiAdminKey]
	_, hasDB := secret.Data[dbKey]
	_, hasWallet := secret.Data[walletKey]
	walletPasswordKey := strings.TrimSpace(secretSpec.WalletPasswordKey)
	_, hasWalletPassword := secret.Data[walletPasswordKey]
	hasWalletPassword = walletPasswordKey != "" && hasWalletPassword && len(secret.Data[walletPasswordKey]) > 0
	if !hasUIAdmin || !hasDB {
		return fmt.Errorf("secret %s must contain keys %s and %s", secret.Name, uiAdminKey, dbKey)
	}
	if strings.EqualFold(agentFactoryConnectionType(inst), "Wallet") {
		if !hasWallet {
			return fmt.Errorf("secret %s must contain wallet key %s for Wallet connectionType", secret.Name, walletKey)
		}
		if walletPasswordKey != "" && !hasWalletPassword {
			return fmt.Errorf("secret %s must contain a non-empty wallet password key %s when spec.security.secret.walletPasswordKey is configured", secret.Name, walletPasswordKey)
		}
		if ok, err := agentFactoryWalletContainsAlias(secret.Data[walletKey], inst.Spec.Database.Wallet.SelectedTnsAlias); err != nil {
			return fmt.Errorf("failed to inspect wallet TNS aliases: %w", err)
		} else if !ok {
			return fmt.Errorf("wallet does not contain TNS alias %q", inst.Spec.Database.Wallet.SelectedTnsAlias)
		}
	}
	inst.Status.Secret.Name = secret.Name
	inst.Status.Secret.ResourceVersion = secret.ResourceVersion
	inst.Status.Secret.HasUIAdminPassword = hasUIAdmin
	inst.Status.Secret.HasDBPassword = hasDB
	inst.Status.Secret.HasWalletZip = hasWallet
	inst.Status.Secret.HasWalletPassword = hasWalletPassword
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryOutboundTLS(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	configured := inst.Spec.Security != nil && inst.Spec.Security.OutboundTLS != nil
	if !configured {
		inst.Status.OutboundTLS = privateaiv4.AgentFactoryOutboundTLSStatus{}
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryOutboundTLSReady,
			Status:             metav1.ConditionFalse,
			Reason:             "Disabled",
			Message:            "AgentFactory outbound TLS is not configured",
			ObservedGeneration: inst.Generation,
		})
		return nil
	}
	outboundTLS := inst.Spec.Security.OutboundTLS
	if outboundTLS.CABundle == nil {
		return r.setAgentFactoryOutboundTLSFailure(inst, "ConfigurationInvalid", "outboundTLS.caBundle is required")
	}

	desiredStatus := privateaiv4.AgentFactoryOutboundTLSStatus{}
	if bundle := outboundTLS.CABundle; bundle != nil {
		secret := &corev1.Secret{}
		secretName := strings.TrimSpace(bundle.SecretName)
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: inst.Namespace}, secret); err != nil {
			return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleSecretUnavailable", fmt.Sprintf("outbound CA Secret %q is unavailable: %v", secretName, err))
		}
		key := agentFactoryOutboundCABundleKey(bundle)
		data, ok := secret.Data[key]
		if !ok {
			return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleKeyMissing", fmt.Sprintf("outbound CA Secret %q must contain key %q", secret.Name, key))
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleEmpty", fmt.Sprintf("outbound CA Secret %q key %q is empty", secret.Name, key))
		}
		if bytes.Contains(bytes.ToUpper(data), []byte("PRIVATE KEY")) {
			return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleContainsPrivateKey", fmt.Sprintf("outbound CA Secret %q key %q must contain CA certificates only", secret.Name, key))
		}
		desiredStatus.CABundleSecretName = secret.Name
		desiredStatus.CABundleResourceVersion = secret.ResourceVersion
	}

	previousStatus := inst.Status.OutboundTLS
	if previousStatus.CABundleSecretName == desiredStatus.CABundleSecretName &&
		previousStatus.CABundleResourceVersion == desiredStatus.CABundleResourceVersion {
		desiredStatus.CABundleImportedResourceVersion = previousStatus.CABundleImportedResourceVersion
	}
	inst.Status.OutboundTLS = desiredStatus
	reason := "Validated"
	if desiredStatus.CABundleResourceVersion != desiredStatus.CABundleImportedResourceVersion {
		reason = "ImportPending"
	} else if previousStatus != desiredStatus {
		reason = "SecretChanged"
	}
	message := "Outbound TLS Secret references are valid and mounted into the AgentFactory workload"
	conditionStatus := metav1.ConditionTrue
	if desiredStatus.CABundleResourceVersion != desiredStatus.CABundleImportedResourceVersion {
		conditionStatus = metav1.ConditionFalse
		message = "Outbound CA bundle is mounted; Agent Factory certificate registration is pending"
	}
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryOutboundTLSReady,
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inst.Generation,
	})
	return nil
}

func (r *AgentFactoryReconciler) setAgentFactoryOutboundTLSFailure(inst *privateaiv4.AgentFactory, reason, message string) error {
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryOutboundTLSReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inst.Generation,
	})
	return fmt.Errorf("%s", message)
}

func agentFactoryWalletContainsAlias(data []byte, alias string) (bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, err
	}
	for _, file := range reader.File {
		if !strings.EqualFold(path.Base(file.Name), "tnsnames.ora") {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return false, err
		}
		content, readErr := io.ReadAll(handle)
		closeErr := handle.Close()
		if readErr != nil {
			return false, readErr
		}
		if closeErr != nil {
			return false, closeErr
		}
		for _, line := range strings.Split(string(content), "\n") {
			left, _, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(alias)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryStorage(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	statuses := []privateaiv4.AgentFactoryVolumeStatus{}
	for _, volume := range agentFactoryVolumeSpecs(inst) {
		status := privateaiv4.AgentFactoryVolumeStatus{Name: volume.Name, MountPath: volume.MountPath}
		switch {
		case volume.Source.ExistingPVC != nil:
			status.ClaimName = volume.Source.ExistingPVC.ClaimName
		case volume.Source.PVC != nil:
			claim := aicommons.BuildAgentFactoryPVCs(inst)
			for _, pvc := range claim {
				if pvc.Name != inst.Name+"-"+volume.Name {
					continue
				}
				if _, _, err := k8sobjects.EnsurePersistentVolumeClaim(ctx, r.Client, pvc); err != nil {
					return err
				}
				if err := r.reconcileAgentFactoryPVCOwnerReference(ctx, inst, pvc.Name, boolDefaultFalse(volume.Source.PVC.DeletePvcOnDelete)); err != nil {
					return err
				}
				status.ClaimName = pvc.Name
				status.Managed = true
				break
			}
		}
		statuses = append(statuses, status)
	}
	inst.Status.Storage = statuses
	return nil
}

// reconcileAgentFactoryPVCOwnerReference makes the delete policy authoritative
// for existing claims as well as newly created claims. In particular, a claim
// retained by policy must not remain controller-owned from an older reconcile.
func (r *AgentFactoryReconciler) reconcileAgentFactoryPVCOwnerReference(ctx context.Context, inst *privateaiv4.AgentFactory, name string, deleteOnDelete bool) error {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, pvc); err != nil {
		return err
	}

	if deleteOnDelete {
		if metav1.IsControlledBy(pvc, inst) {
			return nil
		}
		if err := controllerutil.SetControllerReference(inst, pvc, r.Scheme); err != nil {
			return err
		}
		return r.Update(ctx, pvc)
	}

	if len(pvc.OwnerReferences) == 0 || inst.UID == "" {
		return nil
	}
	ownerReferences := pvc.OwnerReferences[:0]
	changed := false
	for _, owner := range pvc.OwnerReferences {
		if owner.UID == inst.UID && owner.Name == inst.Name && owner.Kind == "AgentFactory" {
			changed = true
			continue
		}
		ownerReferences = append(ownerReferences, owner)
	}
	if !changed {
		return nil
	}
	pvc.OwnerReferences = ownerReferences
	return r.Update(ctx, pvc)
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryDatabaseSetup(ctx context.Context, inst *privateaiv4.AgentFactory) (bool, error) {
	database := inst.Spec.Database
	inst.Status.Database.PAFUser = strings.TrimSpace(database.Username)
	bootstrap := database.Bootstrap
	if !databaseBootstrapEnabledSpec(bootstrap) {
		inst.Status.Database.Phase = agentFactoryDatabasePhaseDisabled
		inst.Status.Database.BootstrapJobName = ""
		inst.Status.Database.Message = "Database setup Job is disabled; the PAF readiness probe will validate the user-created schema"
		return true, nil
	}
	if err := r.reconcileAgentFactoryDatabaseSetupScript(ctx, inst); err != nil {
		return false, err
	}

	job := aicommons.BuildAgentFactoryDatabaseSetupJob(inst)
	if err := controllerutil.SetControllerReference(inst, job, r.Scheme); err != nil {
		return false, err
	}
	inst.Status.Database.BootstrapJobName = job.Name
	found := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, found)
	if err == nil {
		if found.Annotations[aicommons.AgentFactoryDatabaseSetupGenerationLabel] != job.Annotations[aicommons.AgentFactoryDatabaseSetupGenerationLabel] ||
			found.Annotations[aicommons.AgentFactoryDatabaseSetupScriptHashLabel] != job.Annotations[aicommons.AgentFactoryDatabaseSetupScriptHashLabel] {
			if err := r.Delete(ctx, found); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
			inst.Status.Database.Phase = agentFactoryDatabasePhasePending
			inst.Status.Database.Message = "Database setup Job is being recreated for the current AgentFactory generation"
			return false, nil
		}
		if found.Status.Succeeded > 0 {
			if err := r.cleanupAgentFactoryBootstrapSecret(ctx, inst); err != nil {
				return false, err
			}
			inst.Status.Database.Phase = agentFactoryDatabasePhaseReady
			inst.Status.Database.Message = "Database setup completed and the PAF users and grants were validated"
			return true, nil
		}
		if found.Status.Failed > 0 {
			inst.Status.Database.Phase = agentFactoryDatabasePhaseFailed
			inst.Status.Database.Message = "Database setup Job failed; inspect the Job and PAF setup container logs"
			return false, fmt.Errorf("database setup Job %s failed", found.Name)
		}
		inst.Status.Database.Phase = agentFactoryDatabasePhaseRunning
		inst.Status.Database.Message = "Database setup Job is running"
		return false, nil
	}
	if !apierrors.IsNotFound(err) {
		return false, err
	}

	if err := r.validateAgentFactoryBootstrapSecret(ctx, inst); err != nil {
		return false, err
	}
	if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, err
	}
	inst.Status.Database.Phase = agentFactoryDatabasePhasePending
	inst.Status.Database.Message = "Database setup Job was created"
	return false, nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryDatabaseSetupScript(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	desired := aicommons.BuildAgentFactoryDatabaseSetupScriptConfigMap(inst)
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return err
	}

	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	updated := false
	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.Data, desired.Data) {
		found.Data = desired.Data
		updated = true
	}
	if !updated {
		return nil
	}
	return r.Update(ctx, found)
}

func (r *AgentFactoryReconciler) validateAgentFactoryBootstrapSecret(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	bootstrap := inst.Spec.Database.Bootstrap
	if bootstrap == nil || bootstrap.AdminCredentials == nil {
		return fmt.Errorf("spec.database.bootstrap.adminCredentials is required when bootstrap is enabled")
	}
	admin := bootstrap.AdminCredentials
	usernameKey := strings.TrimSpace(admin.UsernameKey)
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := strings.TrimSpace(admin.PasswordKey)
	if passwordKey == "" {
		passwordKey = "password"
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: strings.TrimSpace(admin.SecretName), Namespace: inst.Namespace}, secret); err != nil {
		return err
	}
	adminUsername := strings.TrimSpace(string(secret.Data[usernameKey]))
	if len(secret.Data[usernameKey]) == 0 || adminUsername == "" {
		return fmt.Errorf("bootstrap Secret %s must contain a non-empty username key %s", secret.Name, usernameKey)
	}
	if strings.Contains(strings.ToUpper(adminUsername), " AS ") {
		return fmt.Errorf("bootstrap Secret %s username must not include an Oracle connection privilege; use username SYS and adminCredentials.privilege SYSDBA", secret.Name)
	}
	if len(secret.Data[passwordKey]) == 0 {
		return fmt.Errorf("bootstrap Secret %s must contain a non-empty password key %s", secret.Name, passwordKey)
	}
	return nil
}

func (r *AgentFactoryReconciler) cleanupAgentFactoryBootstrapSecret(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	bootstrap := inst.Spec.Database.Bootstrap
	if bootstrap == nil || !strings.EqualFold(strings.TrimSpace(bootstrap.CleanupPolicy), "DeleteOnSuccess") || bootstrap.AdminCredentials == nil {
		return nil
	}
	secret := &corev1.Secret{}
	admin := bootstrap.AdminCredentials
	err := r.Get(ctx, types.NamespacedName{Name: strings.TrimSpace(admin.SecretName), Namespace: inst.Namespace}, secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryServices(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	services := aicommons.BuildAgentFactoryServices(inst)
	desiredServiceNames := make(map[string]struct{}, len(services))
	inst.Status.LocalService = ""
	inst.Status.PublicLoadBalancerService = ""
	inst.Status.PrivateLoadBalancerService = ""
	inst.Status.LoadBalancerIP = ""
	for _, service := range services {
		desiredServiceNames[service.Name] = struct{}{}
		if err := controllerutil.SetControllerReference(inst, service, r.Scheme); err != nil {
			return err
		}
		if _, err := k8sobjects.EnsureService(ctx, r.Client, service.Namespace, service, k8sobjects.ServiceSyncOptions{
			NodePortMerge:          k8sobjects.NodePortMergeByNamePortAndProtocol,
			SyncOwnerReferences:    true,
			SyncLoadBalancerFields: true,
		}); err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(service.Name, "-public-lb"):
			inst.Status.PublicLoadBalancerService = service.Name
		case strings.HasSuffix(service.Name, "-private-lb"):
			inst.Status.PrivateLoadBalancerService = service.Name
		default:
			inst.Status.LocalService = service.Name
		}
	}

	// BuildAgentFactoryServices returns only the currently requested services.
	// Remove previously generated services that are no longer desired, such as
	// a public LoadBalancer after the user disables it in the AgentFactory.
	for _, serviceName := range []string{
		aicommons.AgentFactoryClusterServiceName(inst),
		inst.Name + "-public-lb",
		inst.Name + "-private-lb",
	} {
		if _, desired := desiredServiceNames[serviceName]; desired {
			continue
		}
		stale := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: inst.Namespace}, stale)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !metav1.IsControlledBy(stale, inst) {
			continue
		}
		if err := r.Delete(ctx, stale); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if err := r.reconcileAgentFactoryLoadBalancerCondition(ctx, inst,
		agentFactoryPublicLoadBalancerReady, inst.Name+"-public-lb",
		hasDesiredAgentFactoryService(desiredServiceNames, inst.Name+"-public-lb")); err != nil {
		return err
	}
	if err := r.reconcileAgentFactoryLoadBalancerCondition(ctx, inst,
		agentFactoryPrivateLoadBalancerReady, inst.Name+"-private-lb",
		hasDesiredAgentFactoryService(desiredServiceNames, inst.Name+"-private-lb")); err != nil {
		return err
	}
	return nil
}

func hasDesiredAgentFactoryService(desiredServiceNames map[string]struct{}, serviceName string) bool {
	_, exists := desiredServiceNames[serviceName]
	return exists
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryLoadBalancerCondition(ctx context.Context, inst *privateaiv4.AgentFactory, conditionType, serviceName string, enabled bool) error {
	condition := metav1.Condition{
		Type:               conditionType,
		ObservedGeneration: inst.Generation,
	}
	if !enabled {
		condition.Status = metav1.ConditionFalse
		condition.Reason = agentFactoryReasonLoadBalancerDisabled
		condition.Message = fmt.Sprintf("AgentFactory service %q is not enabled", serviceName)
		r.setAgentFactoryCondition(inst, condition)
		return nil
	}

	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: inst.Namespace}, service)
	if apierrors.IsNotFound(err) {
		condition.Status = metav1.ConditionFalse
		condition.Reason = agentFactoryReasonLoadBalancerProvisioning
		condition.Message = fmt.Sprintf("AgentFactory LoadBalancer service %q is being created", serviceName)
		r.setAgentFactoryCondition(inst, condition)
		return nil
	}
	if err != nil {
		return err
	}
	if len(service.Status.LoadBalancer.Ingress) == 0 {
		condition.Status = metav1.ConditionFalse
		condition.Reason = agentFactoryReasonLoadBalancerProvisioning
		condition.Message = fmt.Sprintf("AgentFactory LoadBalancer service %q has no assigned ingress; inspect the Service events", serviceName)
		r.setAgentFactoryCondition(inst, condition)
		return nil
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = agentFactoryReasonLoadBalancerReady
	condition.Message = fmt.Sprintf("AgentFactory LoadBalancer service %q has assigned ingress", serviceName)
	r.setAgentFactoryCondition(inst, condition)
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			inst.Status.LoadBalancerIP = ingress.IP
			break
		}
	}
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryImageUpgrade(ctx context.Context, inst *privateaiv4.AgentFactory) (bool, error) {
	upgrade := inst.Spec.Upgrade
	if upgrade == nil || !strings.EqualFold(strings.TrimSpace(upgrade.Strategy), "ImageUpgrade") {
		return false, nil
	}

	targetImage := agentFactoryImageName(inst)
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, deployment)
	if apierrors.IsNotFound(err) {
		// The first reconcile creates the normal Deployment. ImageUpgrade is
		// only needed after an existing image is being replaced.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	currentImage := ""
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "agentfactory" || currentImage == "" {
			currentImage = container.Image
		}
		if container.Name == "agentfactory" {
			break
		}
	}
	if currentImage == "" || currentImage == targetImage {
		return false, nil
	}

	status := &inst.Status.ImageUpgrade
	if status.TargetImage != targetImage || status.ObservedGeneration != inst.Generation {
		upgradePod := &corev1.Pod{}
		err := r.Get(ctx, types.NamespacedName{Name: aicommons.AgentFactoryImageUpgradePodName(inst), Namespace: inst.Namespace}, upgradePod)
		if err == nil {
			if deleteErr := r.Delete(ctx, upgradePod); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				return false, deleteErr
			}
			return true, nil
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
		*status = privateaiv4.AgentFactoryImageUpgradeStatus{
			Phase:              agentFactoryImageUpgradePhaseStopping,
			SourceImage:        currentImage,
			TargetImage:        targetImage,
			PodName:            aicommons.AgentFactoryImageUpgradePodName(inst),
			Replicas:           agentFactoryRequestedReplicas(inst),
			ObservedGeneration: inst.Generation,
			Message:            fmt.Sprintf("stopping the existing Deployment before running image upgrade from %q to %q", currentImage, targetImage),
		}
	}

	if status.PodName == "" {
		status.PodName = aicommons.AgentFactoryImageUpgradePodName(inst)
	}

	if status.Phase == agentFactoryImageUpgradePhaseFailed && status.ObservedGeneration == inst.Generation && status.TargetImage == targetImage {
		return true, nil
	}

	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
		updated := deployment.DeepCopy()
		zero := int32(0)
		updated.Spec.Replicas = &zero
		if err := r.Update(ctx, updated); err != nil {
			return false, err
		}
		status.Phase = agentFactoryImageUpgradePhaseStopping
		status.Message = fmt.Sprintf("stopping existing AgentFactory replicas before running image upgrade from %q to %q", status.SourceImage, status.TargetImage)
		return true, nil
	}

	if waiting, err := r.agentFactoryNormalPodsStopping(ctx, inst); err != nil {
		return false, err
	} else if waiting {
		status.Phase = agentFactoryImageUpgradePhaseStopping
		status.Message = "waiting for existing AgentFactory Pods to terminate before running image upgrade"
		return true, nil
	}

	upgradePod := &corev1.Pod{}
	err = r.Get(ctx, types.NamespacedName{Name: status.PodName, Namespace: inst.Namespace}, upgradePod)
	if apierrors.IsNotFound(err) {
		if status.Phase == agentFactoryImageUpgradePhaseSucceeded {
			desired := aicommons.BuildAgentFactoryDeployment(inst)
			if syncAgentFactoryDeployment(deployment, desired) {
				if err := r.Update(ctx, deployment); err != nil {
					return false, err
				}
			}
			status.Message = fmt.Sprintf("image upgrade succeeded; restored AgentFactory Deployment with image %q and %d replicas", targetImage, status.Replicas)
			return true, nil
		}
		newPod := aicommons.BuildAgentFactoryImageUpgradePod(inst)
		if err := controllerutil.SetControllerReference(inst, newPod, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, newPod); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, err
		}
		status.Phase = agentFactoryImageUpgradePhaseRunning
		status.Message = fmt.Sprintf("running image upgrade command in Pod %q using target image %q", newPod.Name, targetImage)
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !upgradePod.DeletionTimestamp.IsZero() {
		status.Phase = agentFactoryImageUpgradePhaseSucceeded
		status.Message = fmt.Sprintf("image upgrade Pod %q is being deleted before the normal Deployment is restored", upgradePod.Name)
		return true, nil
	}

	switch upgradePod.Status.Phase {
	case corev1.PodSucceeded:
		if err := r.Delete(ctx, upgradePod); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
		status.Phase = agentFactoryImageUpgradePhaseSucceeded
		status.Message = fmt.Sprintf("image upgrade command completed successfully in Pod %q; deleting the Pod before restoring the workload", upgradePod.Name)
		return true, nil
	case corev1.PodFailed:
		status.Phase = agentFactoryImageUpgradePhaseFailed
		status.Message = fmt.Sprintf("image upgrade Pod %q failed: %s", upgradePod.Name, agentFactoryPodFailureMessage(upgradePod))
		if !agentFactoryPreserveFailedImageUpgradePod(inst) {
			if err := r.Delete(ctx, upgradePod); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		return true, nil
	default:
		status.Phase = agentFactoryImageUpgradePhaseRunning
		status.Message = fmt.Sprintf("image upgrade Pod %q is %s using target image %q", upgradePod.Name, strings.ToLower(string(upgradePod.Status.Phase)), targetImage)
		return true, nil
	}
}

func agentFactoryRequestedReplicas(inst *privateaiv4.AgentFactory) int32 {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Replicas != nil && *inst.Spec.Runtime.Replicas > 0 {
		return *inst.Spec.Runtime.Replicas
	}
	return 1
}

func (r *AgentFactoryReconciler) agentFactoryNormalPodsStopping(ctx context.Context, inst *privateaiv4.AgentFactory) (bool, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(inst.Namespace), client.MatchingLabels(aicommons.AgentFactoryLabels(inst))); err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		if !pod.DeletionTimestamp.IsZero() || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		return true, nil
	}
	return false, nil
}

func agentFactoryPreserveFailedImageUpgradePod(inst *privateaiv4.AgentFactory) bool {
	if inst.Spec.Upgrade == nil || inst.Spec.Upgrade.ImageUpgrade == nil || inst.Spec.Upgrade.ImageUpgrade.PreserveFailedPod == nil {
		return true
	}
	return *inst.Spec.Upgrade.ImageUpgrade.PreserveFailedPod
}

func agentFactoryPodFailureMessage(pod *corev1.Pod) string {
	if message := strings.TrimSpace(pod.Status.Message); message != "" {
		return message
	}
	if reason := strings.TrimSpace(pod.Status.Reason); reason != "" {
		return reason
	}
	return "the Pod terminated unsuccessfully"
}

func (r *AgentFactoryReconciler) finishAgentFactoryImageUpgradeReconcile(ctx context.Context, inst *privateaiv4.AgentFactory) (ctrl.Result, error) {
	message := inst.Status.ImageUpgrade.Message
	if inst.Status.ImageUpgrade.Phase == agentFactoryImageUpgradePhaseFailed {
		inst.Status.Phase = agentFactoryPhaseError
		inst.Status.AppState = "error"
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: agentFactoryImageUpgradeReady, Status: metav1.ConditionFalse, Reason: agentFactoryReasonImageUpgradeFailed, Message: message, ObservedGeneration: inst.Generation})
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: privateaiv4.ReconcileError, Status: metav1.ConditionTrue, Reason: agentFactoryReasonImageUpgradeFailed, Message: message, ObservedGeneration: inst.Generation})
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: privateaiv4.ReconcileCompelete, Status: metav1.ConditionFalse, Reason: agentFactoryReasonImageUpgradeFailed, Message: message, ObservedGeneration: inst.Generation})
	} else {
		inst.Status.Phase = agentFactoryPhaseUpdating
		inst.Status.AppState = "pending"
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: agentFactoryImageUpgradeReady, Status: metav1.ConditionFalse, Reason: agentFactoryReasonImageUpgradePending, Message: message, ObservedGeneration: inst.Generation})
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: privateaiv4.ReconcileCompelete, Status: metav1.ConditionFalse, Reason: agentFactoryReasonImageUpgradePending, Message: message, ObservedGeneration: inst.Generation})
		r.setAgentFactoryCondition(inst, metav1.Condition{Type: privateaiv4.ReconcileError, Status: metav1.ConditionFalse, Reason: agentFactoryReasonImageUpgradePending, Message: message, ObservedGeneration: inst.Generation})
	}
	if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
		return ctrl.Result{}, err
	}
	if inst.Status.ImageUpgrade.Phase == agentFactoryImageUpgradePhaseFailed {
		return requeueN, nil
	}
	return requeueY, nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryWorkload(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	desired := aicommons.BuildAgentFactoryDeployment(inst)
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return err
	}
	_, res, err := k8sobjects.ReconcileDeployment(ctx, r.Client, inst.Namespace, desired, func(found, desired *appsv1.Deployment) bool {
		return syncAgentFactoryDeployment(found, desired)
	})
	if err != nil {
		return err
	}
	if res.Created || res.Updated {
		inst.Status.Phase = agentFactoryPhaseUpdating
	}
	inst.Status.WorkloadType = "Deployment"
	return nil
}

// syncAgentFactoryDeployment updates only fields owned by AgentFactory. The API
// server defaults other Deployment and Pod fields, and admission webhooks may
// inject sidecars or additional fields. Replacing the complete spec would
// continually remove those values and cause a reconcile/update loop.
func syncAgentFactoryDeployment(found, desired *appsv1.Deployment) bool {
	updated := syncAgentFactoryStringMap(&found.Labels, desired.Labels)
	updated = syncAgentFactoryStringMap(&found.Annotations, desired.Annotations, aicommons.AgentFactoryConfigRVAnnotation, aicommons.AgentFactorySecretRVAnnotation) || updated
	updated = syncAgentFactoryDeploymentSpec(&found.Spec, &desired.Spec) || updated
	return updated
}

func syncAgentFactoryDeploymentSpec(found, desired *appsv1.DeploymentSpec) bool {
	updated := false
	if !reflect.DeepEqual(found.Replicas, desired.Replicas) {
		found.Replicas = desired.Replicas
		updated = true
	}
	if !reflect.DeepEqual(found.Selector, desired.Selector) {
		found.Selector = desired.Selector
		updated = true
	}
	if !reflect.DeepEqual(found.Strategy, desired.Strategy) {
		found.Strategy = desired.Strategy
		updated = true
	}

	updated = syncAgentFactoryStringMap(&found.Template.Labels, desired.Template.Labels) || updated
	updated = syncAgentFactoryStringMap(&found.Template.Annotations, desired.Template.Annotations, aicommons.AgentFactoryConfigRVAnnotation, aicommons.AgentFactorySecretRVAnnotation, aicommons.AgentFactoryOutboundTLSRVAnnotation) || updated

	foundPodSpec := &found.Template.Spec
	desiredPodSpec := &desired.Template.Spec
	if !reflect.DeepEqual(foundPodSpec.AutomountServiceAccountToken, desiredPodSpec.AutomountServiceAccountToken) {
		foundPodSpec.AutomountServiceAccountToken = desiredPodSpec.AutomountServiceAccountToken
		updated = true
	}
	if foundPodSpec.ServiceAccountName != desiredPodSpec.ServiceAccountName {
		foundPodSpec.ServiceAccountName = desiredPodSpec.ServiceAccountName
		updated = true
	}
	if !reflect.DeepEqual(foundPodSpec.TerminationGracePeriodSeconds, desiredPodSpec.TerminationGracePeriodSeconds) {
		foundPodSpec.TerminationGracePeriodSeconds = desiredPodSpec.TerminationGracePeriodSeconds
		updated = true
	}
	updated = syncAgentFactoryStringMap(&foundPodSpec.NodeSelector, desiredPodSpec.NodeSelector) || updated
	if !reflect.DeepEqual(foundPodSpec.Affinity, desiredPodSpec.Affinity) {
		if desiredPodSpec.Affinity == nil {
			foundPodSpec.Affinity = nil
		} else {
			foundPodSpec.Affinity = desiredPodSpec.Affinity.DeepCopy()
		}
		updated = true
	}
	updated = syncAgentFactoryLocalObjectReferences(&foundPodSpec.ImagePullSecrets, desiredPodSpec.ImagePullSecrets) || updated
	updated = syncAgentFactoryContainers(&foundPodSpec.Containers, desiredPodSpec.Containers) || updated
	updated = syncAgentFactoryVolumes(&foundPodSpec.Volumes, desiredPodSpec.Volumes) || updated
	return updated
}

// syncAgentFactoryStringMap merges desired keys and removes only keys in
// reserved prefixes that are no longer desired. Other keys are preserved for
// mutating webhooks and other controllers that share the object.
func syncAgentFactoryStringMap(found *map[string]string, desired map[string]string, reservedPrefixes ...string) bool {
	updated := false
	if len(desired) > 0 && *found == nil {
		*found = map[string]string{}
	}
	for key, value := range desired {
		if current, ok := (*found)[key]; !ok || current != value {
			(*found)[key] = value
			updated = true
		}
	}
	for key := range *found {
		if _, ok := desired[key]; ok || !agentFactoryHasReservedPrefix(key, reservedPrefixes) {
			continue
		}
		delete(*found, key)
		updated = true
	}
	return updated
}

func agentFactoryHasReservedPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func syncAgentFactoryLocalObjectReferences(found *[]corev1.LocalObjectReference, desired []corev1.LocalObjectReference) bool {
	updated := false
	for _, wanted := range desired {
		index := -1
		for i := range *found {
			if (*found)[i].Name == wanted.Name {
				index = i
				break
			}
		}
		if index < 0 {
			*found = append(*found, wanted)
			updated = true
		}
	}
	return updated
}

func syncAgentFactoryContainers(found *[]corev1.Container, desired []corev1.Container) bool {
	updated := false
	for i := range desired {
		wanted := &desired[i]
		foundIndex := -1
		for j := range *found {
			if (*found)[j].Name != wanted.Name {
				continue
			}
			foundIndex = j
			break
		}
		if foundIndex < 0 {
			*found = append(*found, *wanted)
			updated = true
			continue
		}
		updated = syncAgentFactoryContainer(&(*found)[foundIndex], wanted) || updated
	}
	return updated
}

func syncAgentFactoryContainer(found, desired *corev1.Container) bool {
	updated := false
	if found.Image != desired.Image {
		found.Image = desired.Image
		updated = true
	}
	if found.ImagePullPolicy != desired.ImagePullPolicy {
		found.ImagePullPolicy = desired.ImagePullPolicy
		updated = true
	}
	updated = syncAgentFactoryContainerPorts(&found.Ports, desired.Ports) || updated
	updated = syncAgentFactoryEnv(&found.Env, desired.Env) || updated
	updated = syncAgentFactoryVolumeMounts(&found.VolumeMounts, desired.VolumeMounts) || updated
	updated = syncAgentFactoryReadinessProbe(&found.ReadinessProbe, desired.ReadinessProbe) || updated
	if agentFactoryResourcesSpecified(desired.Resources) && !reflect.DeepEqual(found.Resources, desired.Resources) {
		found.Resources = desired.Resources
		updated = true
	}
	return updated
}

func syncAgentFactoryReadinessProbe(found **corev1.Probe, desired *corev1.Probe) bool {
	if desired == nil {
		if *found == nil {
			return false
		}
		*found = nil
		return true
	}
	merged := desired.DeepCopy()
	// successThreshold and terminationGracePeriodSeconds are not set by the
	// AgentFactory builder. Preserve API/webhook values for those fields.
	if merged.SuccessThreshold == 0 && *found != nil {
		merged.SuccessThreshold = (*found).SuccessThreshold
	}
	if merged.TerminationGracePeriodSeconds == nil && *found != nil {
		merged.TerminationGracePeriodSeconds = (*found).TerminationGracePeriodSeconds
	}
	if reflect.DeepEqual(*found, merged) {
		return false
	}
	*found = merged
	return true
}

func agentFactoryResourcesSpecified(resources corev1.ResourceRequirements) bool {
	return len(resources.Limits) > 0 || len(resources.Requests) > 0 || len(resources.Claims) > 0
}

func syncAgentFactoryContainerPorts(found *[]corev1.ContainerPort, desired []corev1.ContainerPort) bool {
	updated := false
	for _, wanted := range desired {
		if wanted.Protocol == "" {
			wanted.Protocol = corev1.ProtocolTCP
		}
		index := -1
		for i := range *found {
			if wanted.Name != "" && (*found)[i].Name == wanted.Name {
				index = i
				break
			}
			if wanted.Name == "" && (*found)[i].Name == "" && (*found)[i].ContainerPort == wanted.ContainerPort && (*found)[i].Protocol == wanted.Protocol {
				index = i
				break
			}
		}
		if index < 0 {
			*found = append(*found, wanted)
			updated = true
			continue
		}
		if !reflect.DeepEqual((*found)[index], wanted) {
			(*found)[index] = wanted
			updated = true
		}
	}
	return updated
}

func syncAgentFactoryEnv(found *[]corev1.EnvVar, desired []corev1.EnvVar) bool {
	updated := false
	for _, wanted := range desired {
		index := -1
		for i := range *found {
			if (*found)[i].Name == wanted.Name {
				index = i
				break
			}
		}
		if index < 0 {
			*found = append(*found, wanted)
			updated = true
			continue
		}
		if !reflect.DeepEqual((*found)[index], wanted) {
			(*found)[index] = wanted
			updated = true
		}
	}
	return updated
}

func syncAgentFactoryVolumeMounts(found *[]corev1.VolumeMount, desired []corev1.VolumeMount) bool {
	updated := false
	for _, wanted := range desired {
		index := -1
		for i := range *found {
			if (*found)[i].Name == wanted.Name {
				index = i
				break
			}
		}
		if index < 0 {
			*found = append(*found, wanted)
			updated = true
			continue
		}
		if !reflect.DeepEqual((*found)[index], wanted) {
			(*found)[index] = wanted
			updated = true
		}
	}
	return updated
}

func syncAgentFactoryVolumes(found *[]corev1.Volume, desired []corev1.Volume) bool {
	updated := false
	for _, wanted := range desired {
		index := -1
		for i := range *found {
			if (*found)[i].Name == wanted.Name {
				index = i
				break
			}
		}
		if index < 0 {
			*found = append(*found, wanted)
			updated = true
			continue
		}
		merged := syncAgentFactoryVolume((*found)[index], wanted)
		if !reflect.DeepEqual((*found)[index], merged) {
			(*found)[index] = merged
			updated = true
		}
	}
	return updated
}

func syncAgentFactoryVolume(found, desired corev1.Volume) corev1.Volume {
	merged := desired
	if desired.ConfigMap != nil && desired.ConfigMap.DefaultMode == nil && found.ConfigMap != nil {
		merged.ConfigMap = desired.ConfigMap.DeepCopy()
		merged.ConfigMap.DefaultMode = found.ConfigMap.DefaultMode
	}
	if desired.Secret != nil && desired.Secret.DefaultMode == nil && found.Secret != nil {
		merged.Secret = desired.Secret.DeepCopy()
		merged.Secret.DefaultMode = found.Secret.DefaultMode
	}
	return merged
}

func (r *AgentFactoryReconciler) refreshAgentFactoryWorkloadStatus(ctx context.Context, inst *privateaiv4.AgentFactory) (agentFactoryWorkloadStatus, error) {
	inst.Status.Image = agentFactoryImageName(inst)
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, deploy); err != nil {
		return agentFactoryWorkloadStatus{}, err
	}
	inst.Status.Replicas = deploy.Status.Replicas
	inst.Status.ReadyReplicas = deploy.Status.ReadyReplicas

	desiredReplicas := int32(1)
	if deploy.Spec.Replicas != nil {
		desiredReplicas = *deploy.Spec.Replicas
	}
	status := agentFactoryWorkloadStatus{
		DesiredReplicas:    desiredReplicas,
		Replicas:           deploy.Status.Replicas,
		ReadyReplicas:      deploy.Status.ReadyReplicas,
		AvailableReplicas:  deploy.Status.AvailableReplicas,
		Generation:         deploy.Generation,
		ObservedGeneration: deploy.Status.ObservedGeneration,
	}
	if status.ObservedGeneration < status.Generation {
		status.Reason = agentFactoryReasonDeploymentGenerationLag
		status.Message = fmt.Sprintf(
			"Deployment generation %d has not been observed yet; observed generation is %d",
			status.Generation, status.ObservedGeneration)
		return status, nil
	}
	if message, ok := agentFactoryDeploymentReadinessFailure(deploy); ok {
		status.Reason = agentFactoryReasonWorkloadReadiness
		status.Message = message
		return status, nil
	}
	if status.ReadyReplicas < status.DesiredReplicas || status.AvailableReplicas < status.DesiredReplicas {
		status.Reason = agentFactoryReasonReplicasUnavailable
		status.Message = fmt.Sprintf(
			"Deployment replicas are unavailable: desired=%d, replicas=%d, ready=%d, available=%d",
			status.DesiredReplicas, status.Replicas, status.ReadyReplicas, status.AvailableReplicas)
		return status, nil
	}
	status.Ready = true
	status.Reason = agentFactoryReasonWorkloadReady
	status.Message = fmt.Sprintf(
		"Deployment generation %d is observed and all desired replicas are ready and available",
		status.Generation)
	return status, nil
}

func agentFactoryDeploymentReadinessFailure(deploy *appsv1.Deployment) (string, bool) {
	for _, condition := range deploy.Status.Conditions {
		if condition.Type != appsv1.DeploymentProgressing || condition.Status != corev1.ConditionFalse {
			continue
		}
		if condition.Reason != "ProgressDeadlineExceeded" {
			continue
		}
		message := strings.TrimSpace(condition.Message)
		if message == "" {
			message = condition.Reason
		}
		return fmt.Sprintf("Deployment rollout failed readiness: %s", message), true
	}
	return "", false
}

func (r *AgentFactoryReconciler) manageAgentFactoryDeletion(ctx context.Context, inst *privateaiv4.AgentFactory) (bool, error) {
	if inst.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(inst, AgentFactoryFinalizer) {
			return true, sharedk8sutil.AddFinalizerAndPatch(r.Client, inst, AgentFactoryFinalizer)
		}
		return false, nil
	}
	if !controllerutil.ContainsFinalizer(inst, AgentFactoryFinalizer) {
		return true, nil
	}
	if err := r.deleteAgentFactoryManagedPVCs(ctx, inst); err != nil {
		return true, err
	}
	return true, sharedk8sutil.RemoveFinalizerAndPatch(r.Client, inst, AgentFactoryFinalizer)
}

func (r *AgentFactoryReconciler) deleteAgentFactoryManagedPVCs(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	for _, volume := range agentFactoryVolumeSpecs(inst) {
		if volume.Source.PVC == nil || !boolDefaultFalse(volume.Source.PVC.DeletePvcOnDelete) {
			continue
		}
		pvc := &corev1.PersistentVolumeClaim{}
		err := r.Get(ctx, types.NamespacedName{Name: inst.Name + "-" + volume.Name, Namespace: inst.Namespace}, pvc)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *AgentFactoryReconciler) agentFactoryError(ctx context.Context, inst *privateaiv4.AgentFactory, err error) (ctrl.Result, error) {
	logf.FromContext(ctx).WithName("AgentFactory").WithValues("namespace", inst.Namespace, "name", inst.Name).Error(err, "AgentFactory reconciliation failed")
	inst.Status.Phase = agentFactoryPhaseError
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               privateaiv4.ReconcileError,
		Status:             metav1.ConditionTrue,
		Reason:             privateaiv4.ReconcileErrorReason,
		Message:            err.Error(),
		ObservedGeneration: inst.Generation,
	})
	if statusErr := r.updateAgentFactoryStatus(ctx, inst); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, err
}

func (r *AgentFactoryReconciler) setAgentFactoryCondition(inst *privateaiv4.AgentFactory, condition metav1.Condition) {
	meta.RemoveStatusCondition(&inst.Status.Conditions, condition.Type)
	meta.SetStatusCondition(&inst.Status.Conditions, condition)
}

func copyAgentFactoryCondition(condition *metav1.Condition) *metav1.Condition {
	if condition == nil {
		return nil
	}
	copy := *condition
	return &copy
}

func (r *AgentFactoryReconciler) logAgentFactoryStatusTransition(log logr.Logger, inst *privateaiv4.AgentFactory, previousPhase string, previousCondition *metav1.Condition) {
	currentCondition := meta.FindStatusCondition(inst.Status.Conditions, privateaiv4.ReconcileCompelete)
	if previousPhase == inst.Status.Phase && agentFactoryConditionEqual(previousCondition, currentCondition) {
		return
	}
	keys := []interface{}{
		"phase", inst.Status.Phase,
		"appState", inst.Status.AppState,
	}
	if currentCondition != nil {
		keys = append(keys,
			"conditionStatus", currentCondition.Status,
			"conditionReason", currentCondition.Reason,
			"conditionMessage", currentCondition.Message,
		)
	}
	log.Info("AgentFactory status changed", keys...)
}

func agentFactoryConditionEqual(left, right *metav1.Condition) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Status == right.Status && left.Reason == right.Reason && left.Message == right.Message
}

func (r *AgentFactoryReconciler) updateAgentFactoryStatus(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	desiredStatus := inst.Status.DeepCopy()
	key := types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &privateaiv4.AgentFactory{}
		if err := r.Get(ctx, key, latest); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		base := latest.DeepCopy()
		latest.Status = *desiredStatus
		return r.Status().Patch(ctx, latest, client.MergeFrom(base))
	})
}

func (r *AgentFactoryReconciler) agentFactoryRequestsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}
	list := &privateaiv4.AgentFactoryList{}
	if err := r.Client.List(ctx, list, client.InNamespace(configMap.Namespace)); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for _, item := range list.Items {
		if aicommons.AgentFactoryRuntimeConfigMapName(&item) == configMap.Name {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace}})
		}
	}
	return requests
}

func (r *AgentFactoryReconciler) agentFactoryRequestsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	list := &privateaiv4.AgentFactoryList{}
	if err := r.Client.List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for _, item := range list.Items {
		spec := agentFactorySecretSpec(&item)
		usesRuntimeSecret := spec != nil && strings.TrimSpace(spec.Name) == secret.Name
		usesOutboundTLSSecret := item.Spec.Security != nil && item.Spec.Security.OutboundTLS != nil &&
			item.Spec.Security.OutboundTLS.CABundle != nil &&
			strings.TrimSpace(item.Spec.Security.OutboundTLS.CABundle.SecretName) == secret.Name
		usesInboundTLSSecret := item.Spec.Security != nil && item.Spec.Security.InboundTLS != nil &&
			strings.TrimSpace(item.Spec.Security.InboundTLS.SecretName) == secret.Name
		if usesRuntimeSecret || usesInboundTLSSecret || usesOutboundTLSSecret {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace}})
		}
	}
	return requests
}

func agentFactoryVolumeSpecs(inst *privateaiv4.AgentFactory) []privateaiv4.AgentFactoryVolumeSpec {
	if inst.Spec.Storage != nil && len(inst.Spec.Storage.Volumes) > 0 {
		return inst.Spec.Storage.Volumes
	}
	return []privateaiv4.AgentFactoryVolumeSpec{{
		Name:      "app-mount",
		MountPath: aicommons.AgentFactoryDefaultMountPath,
		Source: privateaiv4.AgentFactoryVolumeSource{
			PVC: &privateaiv4.AgentFactoryPVCSource{
				Size:        "100Gi",
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			},
		},
	}}
}

func agentFactorySecretSpec(inst *privateaiv4.AgentFactory) *privateaiv4.AgentFactorySecretSpec {
	if inst.Spec.Security == nil {
		return nil
	}
	return inst.Spec.Security.Secret
}

func agentFactoryConnectionType(inst *privateaiv4.AgentFactory) string {
	if inst.Spec.Database != nil && strings.TrimSpace(inst.Spec.Database.ConnectionType) != "" {
		return inst.Spec.Database.ConnectionType
	}
	return "Wallet"
}

func agentFactoryImageName(inst *privateaiv4.AgentFactory) string {
	if inst.Spec.Runtime != nil && inst.Spec.Runtime.Image != nil {
		return strings.TrimSpace(inst.Spec.Runtime.Image.Name)
	}
	return ""
}

func agentFactoryUIAdminPasswordKey(secret *privateaiv4.AgentFactorySecretSpec) string {
	if strings.TrimSpace(secret.UIAdminPasswordKey) != "" {
		return strings.TrimSpace(secret.UIAdminPasswordKey)
	}
	return "agent_factory_admin_password"
}

func agentFactoryDBPasswordKey(secret *privateaiv4.AgentFactorySecretSpec) string {
	if strings.TrimSpace(secret.DBPasswordKey) != "" {
		return strings.TrimSpace(secret.DBPasswordKey)
	}
	return "agent_factory_db_password"
}

func agentFactoryWalletZipKey(secret *privateaiv4.AgentFactorySecretSpec) string {
	if strings.TrimSpace(secret.WalletZipKey) != "" {
		return strings.TrimSpace(secret.WalletZipKey)
	}
	return "db_wallet.zip"
}

func agentFactoryPrivateLB(inst *privateaiv4.AgentFactory) *privateaiv4.PaiLoadBalancerSpec {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil {
		return nil
	}
	return inst.Spec.Networking.Service.PrivateLoadBalancer
}

func agentFactoryPublicLB(inst *privateaiv4.AgentFactory) *privateaiv4.PaiLoadBalancerSpec {
	if inst.Spec.Networking == nil || inst.Spec.Networking.Service == nil {
		return nil
	}
	return inst.Spec.Networking.Service.PublicLoadBalancer
}

func boolDefaultFalse(value *bool) bool {
	return value != nil && *value
}

func databaseBootstrapEnabledSpec(value *privateaiv4.AgentFactoryDatabaseBootstrapSpec) bool {
	return value != nil && boolDefaultFalse(value.Enabled)
}
