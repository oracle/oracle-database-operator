package privateai

import (
	"context"
	"reflect"
	"strings"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type deploymentMissingOnStatusRefreshClient struct {
	client.Client
	deploymentGets int
}

func TestAgentFactoryWorkloadStatusRequiresReadyDeployment(t *testing.T) {
	replicas := int32(1)
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "default"},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: inst.Name, Namespace: inst.Namespace, Generation: 1},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           1,
			ReadyReplicas:      0,
			AvailableReplicas:  0,
		},
	}

	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding appsv1 scheme: %v", err)
	}
	reconciler := &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment).Build()}
	workload, err := reconciler.refreshAgentFactoryWorkloadStatus(context.Background(), inst)
	if err != nil {
		t.Fatalf("failed refreshing workload status: %v", err)
	}
	if workload.Ready {
		t.Fatal("expected a Deployment with no ready pods to be pending")
	}
	if workload.Reason != agentFactoryReasonReplicasUnavailable {
		t.Fatalf("expected reason %q, got %q", agentFactoryReasonReplicasUnavailable, workload.Reason)
	}
	if inst.Status.Replicas != 1 || inst.Status.ReadyReplicas != 0 {
		t.Fatalf("expected replicas 1/0, got %d/%d", inst.Status.Replicas, inst.Status.ReadyReplicas)
	}

	deployment.Status.ReadyReplicas = 1
	deployment.Status.AvailableReplicas = 1
	reconciler = &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment).Build()}
	workload, err = reconciler.refreshAgentFactoryWorkloadStatus(context.Background(), inst)
	if err != nil {
		t.Fatalf("failed refreshing ready workload status: %v", err)
	}
	if !workload.Ready {
		t.Fatal("expected a fully available Deployment to be ready")
	}
	if workload.Reason != agentFactoryReasonWorkloadReady {
		t.Fatalf("expected reason %q, got %q", agentFactoryReasonWorkloadReady, workload.Reason)
	}
}

func TestValidateAgentFactoryOutboundTLS(t *testing.T) {
	instance := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
		Security: &privateaiv4.AgentFactorySecuritySpec{OutboundTLS: &privateaiv4.AgentFactoryOutboundTLSSpec{}},
	}}
	if err := validateAgentFactoryOutboundTLS(instance); err == nil {
		t.Fatal("expected outboundTLS.caBundle validation error")
	}

	instance.Spec.Security.OutboundTLS.CABundle = &privateaiv4.AgentFactoryOutboundCABundleSpec{SecretName: "private-ai-ca"}
	if err := validateAgentFactoryOutboundTLS(instance); err != nil {
		t.Fatalf("unexpected outboundTLS validation error: %v", err)
	}
	if got := instance.Spec.Security.OutboundTLS.CABundle.Key; got != "ca.crt" {
		t.Fatalf("expected default outbound CA key ca.crt, got %q", got)
	}

	instance.Spec.Security.OutboundTLS.CABundle.Key = "nested/ca.crt"
	if err := validateAgentFactoryOutboundTLS(instance); err == nil {
		t.Fatal("expected outbound CA key path validation error")
	}

}

func TestValidateAgentFactoryWorkerPlacement(t *testing.T) {
	instance := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
		Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
			WorkerNodes:        []string{"node-a"},
			WorkerNodeSelector: map[string]string{"agentfactory.oracle.com/pool": "paf-test04"},
		},
	}}
	if err := validateAgentFactoryWorkerPlacement(instance); err == nil {
		t.Fatal("expected workerNodes and workerNodeSelector conflict")
	}

	instance.Spec.Runtime.WorkerNodes = nil
	instance.Spec.Runtime.WorkerNodeSelector = map[string]string{"bad key": "paf-test04"}
	if err := validateAgentFactoryWorkerPlacement(instance); err == nil {
		t.Fatal("expected invalid workerNodeSelector key error")
	}

	instance.Spec.Runtime.WorkerNodeSelector = map[string]string{"agentfactory.oracle.com/pool": ""}
	if err := validateAgentFactoryWorkerPlacement(instance); err == nil {
		t.Fatal("expected empty workerNodeSelector value error")
	}

	instance.Spec.Runtime.WorkerNodeSelector = map[string]string{"agentfactory.oracle.com/pool": "paf-test04"}
	if err := validateAgentFactoryWorkerPlacement(instance); err != nil {
		t.Fatalf("unexpected worker placement validation error: %v", err)
	}
}

func TestSyncAgentFactoryDeploymentReconcilesServiceAccountAndAffinity(t *testing.T) {
	oldAffinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
	newAffinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "agentfactory.oracle.com/pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"paf"},
			}}}},
		},
	}}
	found := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		ServiceAccountName: "default", Affinity: oldAffinity,
	}}}}
	desired := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		ServiceAccountName: "agentfactory-sa", Affinity: newAffinity,
	}}}}

	if !syncAgentFactoryDeployment(found, desired) {
		t.Fatal("expected ServiceAccount and affinity changes to be reconciled")
	}
	if found.Spec.Template.Spec.ServiceAccountName != "agentfactory-sa" {
		t.Fatalf("expected agentfactory-sa, got %q", found.Spec.Template.Spec.ServiceAccountName)
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Affinity, newAffinity) {
		t.Fatalf("expected affinity to be reconciled, got %#v", found.Spec.Template.Spec.Affinity)
	}
}

func TestReconcileAgentFactoryOutboundTLSValidatesSecretKey(t *testing.T) {
	instance := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "default"},
		Spec: privateaiv4.AgentFactorySpec{Security: &privateaiv4.AgentFactorySecuritySpec{
			OutboundTLS: &privateaiv4.AgentFactoryOutboundTLSSpec{CABundle: &privateaiv4.AgentFactoryOutboundCABundleSpec{
				SecretName: "private-ai-ca",
			}},
		}},
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	reconciler := &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "private-ai-ca", Namespace: "default"}, Data: map[string][]byte{"ca.crt": []byte("ca")}},
	).Build()}
	if err := reconciler.reconcileAgentFactoryOutboundTLS(context.Background(), instance); err != nil {
		t.Fatalf("unexpected outbound TLS Secret validation error: %v", err)
	}

	instance.Spec.Security.OutboundTLS.CABundle.Key = "missing.crt"
	if err := reconciler.reconcileAgentFactoryOutboundTLS(context.Background(), instance); err == nil {
		t.Fatal("expected missing outbound CA Secret key error")
	}

	instance.Spec.Security.OutboundTLS.CABundle.Key = "ca.crt"
	secret := &corev1.Secret{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "private-ai-ca", Namespace: "default"}, secret); err != nil {
		t.Fatalf("failed getting test Secret: %v", err)
	}
	secret.Data["ca.crt"] = []byte("invalid-ca-bundle")
	if err := reconciler.Update(context.Background(), secret); err != nil {
		t.Fatalf("failed updating test Secret: %v", err)
	}
	if err := reconciler.reconcileAgentFactoryOutboundTLS(context.Background(), instance); err == nil {
		t.Fatal("expected invalid outbound CA bundle to be rejected")
	}
}

func TestAgentFactoryServicesRemoveDisabledLoadBalancer(t *testing.T) {
	enabled := true
	instance := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agentfactory-sample",
			Namespace: "default",
			UID:       types.UID("agentfactory-sample-uid"),
		},
		Spec: privateaiv4.AgentFactorySpec{
			Networking: &privateaiv4.AgentFactoryNetworkingSpec{
				Service: &privateaiv4.PaiServiceSpec{
					PublicLoadBalancer: &privateaiv4.PaiLoadBalancerSpec{Enabled: &enabled},
				},
			},
		},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(instance).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}

	if err := r.reconcileAgentFactoryServices(context.Background(), instance); err != nil {
		t.Fatalf("failed reconciling enabled services: %v", err)
	}
	publicService := &corev1.Service{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: instance.Name + "-public-lb", Namespace: instance.Namespace}, publicService); err != nil {
		t.Fatalf("expected public LoadBalancer service to be created: %v", err)
	}
	if instance.Status.PublicLoadBalancerService != publicService.Name {
		t.Fatalf("expected public service status %q, got %q", publicService.Name, instance.Status.PublicLoadBalancerService)
	}
	provisioningCondition := meta.FindStatusCondition(instance.Status.Conditions, agentFactoryPublicLoadBalancerReady)
	if provisioningCondition == nil || provisioningCondition.Status != metav1.ConditionFalse || provisioningCondition.Reason != agentFactoryReasonLoadBalancerProvisioning {
		t.Fatalf("expected provisioning public LoadBalancer condition, got %#v", provisioningCondition)
	}

	publicService.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	if err := cl.Status().Update(context.Background(), publicService); err != nil {
		t.Fatalf("failed updating public LoadBalancer status: %v", err)
	}
	if err := r.reconcileAgentFactoryServices(context.Background(), instance); err != nil {
		t.Fatalf("failed reconciling ready services: %v", err)
	}
	readyCondition := meta.FindStatusCondition(instance.Status.Conditions, agentFactoryPublicLoadBalancerReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue || readyCondition.Reason != agentFactoryReasonLoadBalancerReady {
		t.Fatalf("expected ready public LoadBalancer condition, got %#v", readyCondition)
	}
	if instance.Status.LoadBalancerIP != "203.0.113.10" {
		t.Fatalf("expected LoadBalancer IP to be recorded, got %q", instance.Status.LoadBalancerIP)
	}

	disabled := false
	instance.Spec.Networking.Service.PublicLoadBalancer.Enabled = &disabled
	if err := r.reconcileAgentFactoryServices(context.Background(), instance); err != nil {
		t.Fatalf("failed reconciling disabled services: %v", err)
	}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: publicService.Name, Namespace: instance.Namespace}, &corev1.Service{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected disabled public LoadBalancer service to be deleted, got %v", err)
	}
	if instance.Status.PublicLoadBalancerService != "" {
		t.Fatalf("expected public service status to be cleared, got %q", instance.Status.PublicLoadBalancerService)
	}
	condition := meta.FindStatusCondition(instance.Status.Conditions, agentFactoryPublicLoadBalancerReady)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != agentFactoryReasonLoadBalancerDisabled {
		t.Fatalf("expected disabled public LoadBalancer condition, got %#v", condition)
	}
}

func TestAgentFactoryWorkloadStatusClassifiesGenerationLagAndReadinessFailure(t *testing.T) {
	replicas := int32(1)
	inst := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "default"},
	}

	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding appsv1 scheme: %v", err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: inst.Name, Namespace: inst.Namespace, Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           1,
			ReadyReplicas:      1,
			AvailableReplicas:  1,
		},
	}
	reconciler := &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment).Build()}
	workload, err := reconciler.refreshAgentFactoryWorkloadStatus(context.Background(), inst)
	if err != nil {
		t.Fatalf("failed refreshing generation-lagged workload status: %v", err)
	}
	if workload.Ready || workload.Reason != agentFactoryReasonDeploymentGenerationLag {
		t.Fatalf("expected generation lag, got ready=%t reason=%q", workload.Ready, workload.Reason)
	}

	deployment.Generation = 2
	deployment.Status.ObservedGeneration = 2
	deployment.Status.ReadyReplicas = 0
	deployment.Status.AvailableReplicas = 0
	deployment.Status.Conditions = []appsv1.DeploymentCondition{{
		Type:    appsv1.DeploymentProgressing,
		Status:  corev1.ConditionFalse,
		Reason:  "ProgressDeadlineExceeded",
		Message: "ReplicaSet did not become ready",
	}}
	reconciler = &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment).Build()}
	workload, err = reconciler.refreshAgentFactoryWorkloadStatus(context.Background(), inst)
	if err != nil {
		t.Fatalf("failed refreshing readiness-failed workload status: %v", err)
	}
	if workload.Ready || workload.Reason != agentFactoryReasonWorkloadReadiness {
		t.Fatalf("expected readiness failure, got ready=%t reason=%q", workload.Ready, workload.Reason)
	}
	if !strings.Contains(workload.Message, "ReplicaSet did not become ready") {
		t.Fatalf("expected Deployment condition message, got %q", workload.Message)
	}
}

func TestSyncAgentFactoryDeploymentIgnoresAPIDefaultsAndInjectedFields(t *testing.T) {
	instance := validAgentFactoryForValidation()
	desired := aicommons.BuildAgentFactoryDeployment(instance)
	found := desired.DeepCopy()

	progressDeadlineSeconds := int32(600)
	revisionHistoryLimit := int32(10)
	defaultMode := int32(0644)
	found.Spec.ProgressDeadlineSeconds = &progressDeadlineSeconds
	found.Spec.RevisionHistoryLimit = &revisionHistoryLimit
	found.Spec.Template.Annotations = map[string]string{}
	found.Spec.Template.Annotations["webhook.example.com/injected"] = "true"
	found.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
	found.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	found.Spec.Template.Spec.SchedulerName = "default-scheduler"
	found.Spec.Template.Spec.Containers[0].TerminationMessagePath = "/dev/termination-log"
	found.Spec.Template.Spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
	found.Spec.Template.Spec.Containers[0].ReadinessProbe.SuccessThreshold = 1
	found.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.DefaultMode = &defaultMode
	found.Spec.Template.Spec.Containers = append(found.Spec.Template.Spec.Containers, corev1.Container{
		Name:  "injected-sidecar",
		Image: "sidecar:latest",
	})

	if syncAgentFactoryDeployment(found, desired) {
		t.Fatal("expected API defaults and injected fields not to trigger a Deployment update")
	}
	if got := found.Spec.Template.Annotations["webhook.example.com/injected"]; got != "true" {
		t.Fatalf("expected injected annotation to be preserved, got %q", got)
	}
	if len(found.Spec.Template.Spec.Containers) != 2 || found.Spec.Template.Spec.Containers[1].Name != "injected-sidecar" {
		t.Fatalf("expected injected sidecar to be preserved, got %#v", found.Spec.Template.Spec.Containers)
	}
}

func TestSyncAgentFactoryDeploymentUpdatesRolloutSettings(t *testing.T) {
	instance := validAgentFactoryForValidation()
	desired := aicommons.BuildAgentFactoryDeployment(instance)
	found := desired.DeepCopy()
	gracePeriod := int64(30)
	found.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	found.Spec.Template.Spec.TerminationGracePeriodSeconds = &gracePeriod

	if !syncAgentFactoryDeployment(found, desired) {
		t.Fatal("expected rollout settings to be reconciled")
	}
	if found.Spec.Strategy.Type != desired.Spec.Strategy.Type || found.Spec.Strategy.RollingUpdate == nil ||
		found.Spec.Strategy.RollingUpdate.MaxSurge == nil || found.Spec.Strategy.RollingUpdate.MaxSurge.StrVal != "25%" ||
		found.Spec.Strategy.RollingUpdate.MaxUnavailable == nil || found.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal != 0 {
		t.Fatalf("unexpected reconciled rollout strategy: %#v", found.Spec.Strategy)
	}
	if got := found.Spec.Template.Spec.TerminationGracePeriodSeconds; got == nil || *got != 3600 {
		t.Fatalf("expected terminationGracePeriodSeconds 3600, got %#v", got)
	}
}

func (c *deploymentMissingOnStatusRefreshClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*appsv1.Deployment); ok {
		c.deploymentGets++
		if c.deploymentGets > 1 {
			return apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, key.Name)
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestAgentFactoryReconcileTreatsMissingDeploymentStatusAsPending(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding appsv1 scheme: %v", err)
	}

	instance := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "agentfactory-sample",
			Namespace:  "default",
			Finalizers: []string{AgentFactoryFinalizer},
		},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Image: &privateaiv4.AgentFactoryImageSpec{Name: "agentfactory:test"},
			},
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Details",
				AirGapped:      agentFactoryBoolPtr(false),
				Username:       "appuser",
				Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{
					Protocol:    "TCP",
					Host:        "sidb-primary",
					Port:        1521,
					ServiceName: "prodpdb",
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{
				Secret: &privateaiv4.AgentFactorySecretSpec{
					Name:               "agentfactory-runtime-secret",
					UIAdminPasswordKey: "paf_ui_admin_password",
					DBPasswordKey:      "paf_db_password",
				},
			},
			Storage: &privateaiv4.AgentFactoryStorageSpec{
				Volumes: []privateaiv4.AgentFactoryVolumeSpec{{
					Name:      "app-mount",
					MountPath: "/mount",
					Source: privateaiv4.AgentFactoryVolumeSource{
						ExistingPVC: &privateaiv4.AgentFactoryExistingPVCSource{ClaimName: "stage-fss-pvc"},
					},
				}},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-runtime-secret", Namespace: "default"},
		Data: map[string][]byte{
			"paf_ui_admin_password": []byte("welcome1"),
			"paf_db_password":       []byte("welcome1"),
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithStatusSubresource(&privateaiv4.AgentFactory{}).
		WithObjects(instance, secret).
		Build()
	clientWithMissingDeployment := &deploymentMissingOnStatusRefreshClient{Client: baseClient}

	r := &AgentFactoryReconciler{
		Client: clientWithMissingDeployment,
		Scheme: sch,
		Config: &rest.Config{},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if !result.Requeue || result.RequeueAfter == 0 {
		t.Fatalf("expected reconcile to requeue while deployment status is pending, got %#v", result)
	}

	updated := &privateaiv4.AgentFactory{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed reading updated AgentFactory: %v", err)
	}
	if updated.Status.Phase != agentFactoryPhasePending {
		t.Fatalf("expected phase %q, got %q", agentFactoryPhasePending, updated.Status.Phase)
	}
	if updated.Status.AppState != "pending" {
		t.Fatalf("expected appState pending, got %q", updated.Status.AppState)
	}
	if updated.Status.Replicas != 0 || updated.Status.ReadyReplicas != 0 {
		t.Fatalf("expected replicas 0/0 while deployment is missing, got %d/%d", updated.Status.Replicas, updated.Status.ReadyReplicas)
	}
	workloadCondition := meta.FindStatusCondition(updated.Status.Conditions, agentFactoryWorkloadReady)
	if workloadCondition == nil || workloadCondition.Status != metav1.ConditionFalse || workloadCondition.Reason != agentFactoryReasonDeploymentNotFound {
		t.Fatalf("expected DeploymentNotFound workload condition, got %#v", workloadCondition)
	}
	databaseCondition := meta.FindStatusCondition(updated.Status.Conditions, agentFactoryDatabaseReady)
	if databaseCondition != nil && databaseCondition.Status == metav1.ConditionFalse {
		t.Fatalf("expected workload failure not to be reported as database failure, got %#v", databaseCondition)
	}
}

func TestAgentFactoryRetainedPVCIsNotControllerOwned(t *testing.T) {
	controller := true
	instance := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agentfactory-sample",
			Namespace: "default",
			UID:       types.UID("agentfactory-uid"),
		},
		Spec: privateaiv4.AgentFactorySpec{
			Storage: &privateaiv4.AgentFactoryStorageSpec{
				Volumes: []privateaiv4.AgentFactoryVolumeSpec{{
					Name:      "app-mount",
					MountPath: "/mount",
					Source: privateaiv4.AgentFactoryVolumeSource{PVC: &privateaiv4.AgentFactoryPVCSource{
						Size:              "1Gi",
						DeletePvcOnDelete: agentFactoryBoolPtr(false),
					}},
				}},
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agentfactory-sample-app-mount",
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "privateai.oracle.com/v4",
				Kind:       "AgentFactory",
				Name:       instance.Name,
				UID:        instance.UID,
				Controller: &controller,
			}},
		},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(instance, pvc).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}
	if err := r.reconcileAgentFactoryStorage(context.Background(), instance); err != nil {
		t.Fatalf("storage reconcile failed: %v", err)
	}

	updated := &corev1.PersistentVolumeClaim{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, updated); err != nil {
		t.Fatalf("failed reading retained PVC: %v", err)
	}
	if metav1.IsControlledBy(updated, instance) {
		t.Fatal("expected retained PVC not to be controller-owned")
	}
}

func TestAgentFactoryValidateRejectsInvalidPublicLoadBalancerPolicy(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Networking.Service.PublicLoadBalancer = &privateaiv4.PaiLoadBalancerSpec{
		ExternalTrafficPolicy: "Invalid",
	}

	err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "publicLoadBalancer.externalTrafficPolicy must be Local or Cluster") {
		t.Fatalf("expected publicLoadBalancer externalTrafficPolicy validation error, got %v", err)
	}
}

func TestAgentFactoryValidateRejectsInvalidPrivateLoadBalancerPolicy(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Networking.Service.PrivateLoadBalancer = &privateaiv4.PaiLoadBalancerSpec{
		ExternalTrafficPolicy: "Invalid",
	}

	err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "privateLoadBalancer.externalTrafficPolicy must be Local or Cluster") {
		t.Fatalf("expected privateLoadBalancer externalTrafficPolicy validation error, got %v", err)
	}
}

func TestAgentFactoryValidateAllowsLocalAndClusterLoadBalancerPolicies(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Networking.Service.PublicLoadBalancer = &privateaiv4.PaiLoadBalancerSpec{
		ExternalTrafficPolicy: "Local",
	}
	inst.Spec.Networking.Service.PrivateLoadBalancer = &privateaiv4.PaiLoadBalancerSpec{
		ExternalTrafficPolicy: "Cluster",
	}

	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected Local and Cluster externalTrafficPolicy values to validate, got %v", err)
	}
}

func TestAgentFactoryValidateSkipsLLMWithoutModelID(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Configuration.LLM = &privateaiv4.AgentFactoryLLMSpec{Provider: "openai"}

	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected missing model ID to skip optional LLM setup, got %v", err)
	}
	if inst.Spec.Configuration.LLM.Provider != "" {
		t.Fatalf("expected provider to be cleared when model ID is missing, got %q", inst.Spec.Configuration.LLM.Provider)
	}
}

func TestAgentFactoryValidateNormalizesProdMode(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Configuration.Mode = " PROD "
	inst.Spec.Runtime = &privateaiv4.AgentFactoryRuntimeSpec{
		Image: &privateaiv4.AgentFactoryImageSpec{Name: "agentfactory:test"},
		Env:   []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_MODE", Value: "PrOd"}},
	}

	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected case-insensitive prod mode to validate, got %v", err)
	}
	if got := inst.Spec.Configuration.Mode; got != "prod" {
		t.Fatalf("expected configuration mode to normalize to prod, got %q", got)
	}
	if got := inst.Spec.Runtime.Env[0].Value; got != "prod" {
		t.Fatalf("expected AGENT_FACTORY_MODE to normalize to prod, got %q", got)
	}
}

func TestAgentFactoryValidateRejectsUnsupportedMode(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Configuration.Mode = "dev"

	err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "spec.configuration.mode must be prod") {
		t.Fatalf("expected unsupported mode validation error, got %v", err)
	}

	inst = validAgentFactoryForValidation()
	inst.Spec.Runtime = &privateaiv4.AgentFactoryRuntimeSpec{
		Image: &privateaiv4.AgentFactoryImageSpec{Name: "agentfactory:test"},
		Env:   []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_MODE", Value: "dev"}},
	}
	err = (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "AGENT_FACTORY_MODE must be prod") {
		t.Fatalf("expected unsupported AGENT_FACTORY_MODE validation error, got %v", err)
	}
}

func TestAgentFactoryValidateRejectsUnsupportedWalletPasswordRuntimeKey(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{{
		Name:  "AGENT_FACTORY_DB_WALLET_PASSWORD",
		Value: "/run/secrets/agent_factory_db_wallet_password",
	}}

	err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "AGENT_FACTORY_DB_WALLET_PASSWORD is unsupported") {
		t.Fatalf("expected unsupported wallet password runtime key validation error, got %v", err)
	}
}

func TestAgentFactoryValidateSilentInstallAndAdminUsername(t *testing.T) {
	inst := validAgentFactoryForValidation()
	install := false
	inst.Spec.Configuration.SilentInstall = &install
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err == nil || !strings.Contains(err.Error(), "silentInstall must be true") {
		t.Fatalf("expected silentInstall=true validation error, got %v", err)
	}

	inst = validAgentFactoryForValidation()
	inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{
		{Name: "AGENT_FACTORY_SILENT_INSTALL", Value: " YES "},
		{Name: "AGENT_FACTORY_ADMIN_USERNAME", Value: " Admin@example.COM "},
	}
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected true alias and admin override to validate, got %v", err)
	}
	if got := inst.Spec.Runtime.Env[0].Value; got != "true" {
		t.Fatalf("expected silent install alias to normalize to true, got %q", got)
	}
	if got := inst.Spec.Runtime.Env[1].Value; got != "admin@example.com" {
		t.Fatalf("expected admin username to normalize to lowercase, got %q", got)
	}
}

func TestAgentFactoryValidateRejectsInvalidSilentInstallAndAdminUsername(t *testing.T) {
	for _, value := range []string{"false", "0", "no", "off", "maybe", ""} {
		inst := validAgentFactoryForValidation()
		inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_SILENT_INSTALL", Value: value}}
		err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
		if err == nil || !strings.Contains(err.Error(), "AGENT_FACTORY_SILENT_INSTALL must be true") {
			t.Fatalf("expected silent-install rejection for %q, got %v", value, err)
		}
	}

	for _, value := range []string{"admin", "Admin User@example.com", "admin@example", "admin\x00@example.com"} {
		inst := validAgentFactoryForValidation()
		inst.Spec.Configuration.UIAdminUsername = value
		err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
		if err == nil || !strings.Contains(err.Error(), "uiAdminUsername") {
			t.Fatalf("expected invalid admin username rejection for %q, got %v", value, err)
		}
	}
}

func TestAgentFactoryValidateNormalizesCertificateIP(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Spec.Configuration.CertIPAddress = " 2001:0DB8:0:0:0:0:0:1 "
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected valid certificate IP to validate, got %v", err)
	}
	if got := inst.Spec.Configuration.CertIPAddress; got != "2001:db8::1" {
		t.Fatalf("expected certificate IP to normalize, got %q", got)
	}

	inst = validAgentFactoryForValidation()
	inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_CERT_IP_ADDRESS", Value: "192.0.2.10"}}
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected runtime certificate IP to validate, got %v", err)
	}
}

func TestAgentFactoryValidateRejectsInvalidCertificateIP(t *testing.T) {
	for _, value := range []string{"2001:db8::/64", "[2001:db8::1]", "192.0.2.1:443", "agent.example.com", "192.0.2.1,192.0.2.2"} {
		inst := validAgentFactoryForValidation()
		inst.Spec.Configuration.CertIPAddress = value
		err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
		if err == nil || !strings.Contains(err.Error(), "certIpAddress") {
			t.Fatalf("expected invalid certificate IP rejection for %q, got %v", value, err)
		}
	}
}

func TestAgentFactoryValidateCanonicalizesSwagger(t *testing.T) {
	inst := validAgentFactoryForValidation()
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected omitted Swagger setting to validate, got %v", err)
	}
	if inst.Spec.Configuration.EnableSwagger == nil || *inst.Spec.Configuration.EnableSwagger {
		t.Fatalf("expected omitted Swagger setting to normalize to false")
	}

	for _, value := range []string{"true", "TRUE", "1", "yes", "on"} {
		inst = validAgentFactoryForValidation()
		inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_ENABLE_SWAGGER", Value: value}}
		if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
			t.Fatalf("expected Swagger value %q to validate, got %v", value, err)
		}
		if got := inst.Spec.Runtime.Env[0].Value; got != "true" {
			t.Fatalf("expected Swagger value %q to normalize to true, got %q", value, got)
		}
	}

	for _, value := range []string{"false", "0", "no", "off", "invalid", ""} {
		inst = validAgentFactoryForValidation()
		inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{{Name: "AGENT_FACTORY_ENABLE_SWAGGER", Value: value}}
		if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
			t.Fatalf("expected Swagger value %q to behave as false, got %v", value, err)
		}
		if got := inst.Spec.Runtime.Env[0].Value; got != "false" {
			t.Fatalf("expected Swagger value %q to normalize to false, got %q", value, got)
		}
	}
}

func TestAgentFactoryValidateNormalizesTimeoutsAndProxies(t *testing.T) {
	inst := validAgentFactoryForValidation()
	zero := int32(0)
	negative := int32(-1)
	inst.Spec.Configuration.StatePollIntervalSeconds = &zero
	inst.Spec.Configuration.APIReadyTimeoutSeconds = &negative
	inst.Spec.Configuration.HTTPProxy = " http://proxy.example.com:8080 "
	inst.Spec.Configuration.HTTPSProxy = "https://proxy.example.com:8443"
	inst.Spec.Configuration.NoProxy = " localhost, 127.0.0.1, ::1 "
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected timeout and proxy settings to validate, got %v", err)
	}
	if got := *inst.Spec.Configuration.StatePollIntervalSeconds; got != 5 {
		t.Fatalf("expected state poll fallback 5, got %d", got)
	}
	if got := *inst.Spec.Configuration.APIReadyTimeoutSeconds; got != 600 {
		t.Fatalf("expected API ready fallback 600, got %d", got)
	}

	inst = validAgentFactoryForValidation()
	inst.Spec.Runtime.Env = []privateaiv4.EnvironmentVariable{
		{Name: "AGENT_FACTORY_INSTALL_TIMEOUT_SECONDS", Value: "invalid"},
		{Name: "AGENT_FACTORY_READY_TIMEOUT_SECONDS", Value: "0"},
		{Name: "AGENT_FACTORY_HTTP_PROXY", Value: "http://proxy.example.com:8080"},
		{Name: "AGENT_FACTORY_NO_PROXY", Value: "localhost,127.0.0.1,::1"},
	}
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected runtime timeout and proxy settings to validate, got %v", err)
	}
	if got := inst.Spec.Runtime.Env[0].Value; got != "1800" {
		t.Fatalf("expected install timeout fallback 1800, got %q", got)
	}
	if got := inst.Spec.Runtime.Env[1].Value; got != "600" {
		t.Fatalf("expected ready timeout fallback 600, got %q", got)
	}
}

func TestAgentFactoryValidateRejectsInvalidProxyConfiguration(t *testing.T) {
	for _, value := range []string{"proxy.example.com:8080", "http://user:pass.example.com:8080", "ftp://proxy.example.com:8080"} {
		inst := validAgentFactoryForValidation()
		inst.Spec.Configuration.HTTPProxy = value
		err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
		if err == nil || !strings.Contains(err.Error(), "httpProxy") {
			t.Fatalf("expected invalid proxy rejection for %q, got %v", value, err)
		}
	}

	inst := validAgentFactoryForValidation()
	inst.Spec.Configuration.HTTPProxy = "http://proxy.example.com:8080"
	inst.Spec.Configuration.NoProxy = "localhost,127.0.0.1"
	err := (&AgentFactoryReconciler{}).validateAgentFactory(inst)
	if err == nil || !strings.Contains(err.Error(), "noProxy") {
		t.Fatalf("expected required loopback noProxy entries rejection, got %v", err)
	}
}

func TestAgentFactoryValidateDatabaseContract(t *testing.T) {
	inst := validAgentFactoryForValidation()
	if got := inst.Spec.Database.ConnectionType; got != "Details" {
		t.Fatalf("unexpected fixture connection type %q", got)
	}
	inst.Spec.Database.ConnectionType = "details"
	inst.Spec.Database.Details.Protocol = "tcp"
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected case-insensitive Details database settings to validate, got %v", err)
	}
	if got := inst.Spec.Database.ConnectionType; got != "Details" {
		t.Fatalf("expected connection type to normalize to Details, got %q", got)
	}
	if got := inst.Spec.Database.Details.Protocol; got != "TCP" {
		t.Fatalf("expected protocol to normalize to TCP, got %q", got)
	}

	inst = validAgentFactoryForValidation()
	inst.Spec.Database.ConnectionType = "Wallet"
	inst.Spec.Database.AirGapped = agentFactoryBoolPtr(false)
	inst.Spec.Database.Wallet = &privateaiv4.AgentFactoryWalletSpec{
		SelectedTnsAlias: "APP", HasOciCertificates: agentFactoryBoolPtr(true),
	}
	inst.Spec.Database.Details = nil
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected valid Wallet database settings to validate, got %v", err)
	}
}

func TestAgentFactoryValidateRejectsInvalidDatabaseContract(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*privateaiv4.AgentFactory)
	}{
		{"missing connection type", func(inst *privateaiv4.AgentFactory) { inst.Spec.Database.ConnectionType = "" }},
		{"missing air gapped", func(inst *privateaiv4.AgentFactory) { inst.Spec.Database.AirGapped = nil }},
		{"invalid port", func(inst *privateaiv4.AgentFactory) { inst.Spec.Database.Details.Port = 65536 }},
		{"missing protocol", func(inst *privateaiv4.AgentFactory) { inst.Spec.Database.Details.Protocol = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := validAgentFactoryForValidation()
			tc.mutate(inst)
			if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err == nil {
				t.Fatalf("expected database validation error")
			}
		})
	}

	inst := validAgentFactoryForValidation()
	inst.Spec.Database.ConnectionType = "Wallet"
	inst.Spec.Database.AirGapped = agentFactoryBoolPtr(true)
	inst.Spec.Database.Wallet = &privateaiv4.AgentFactoryWalletSpec{SelectedTnsAlias: "APP", HasOciCertificates: agentFactoryBoolPtr(false)}
	inst.Spec.Database.Details = nil
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err == nil || !strings.Contains(err.Error(), "airGapped must be false") {
		t.Fatalf("expected Wallet airGapped rejection, got %v", err)
	}
}

func TestAgentFactoryValidateBootstrapRequiresAdminCredentials(t *testing.T) {
	enabled := true
	inst := validAgentFactoryForValidation()
	inst.Spec.Database.Bootstrap = &privateaiv4.AgentFactoryDatabaseBootstrapSpec{Enabled: &enabled}
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err == nil || !strings.Contains(err.Error(), "adminCredentials.secretName") {
		t.Fatalf("expected bootstrap admin Secret validation error, got %v", err)
	}

	inst.Spec.Database.Bootstrap.AdminCredentials = &privateaiv4.AgentFactoryAdminCredentialsSpec{SecretName: "paf-bootstrap-admin"}
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected valid bootstrap settings, got %v", err)
	}
	if got := inst.Spec.Database.Bootstrap.AdminCredentials.Privilege; got != "NORMAL" {
		t.Fatalf("expected default bootstrap privilege NORMAL, got %q", got)
	}

	inst.Spec.Database.Bootstrap.AdminCredentials.Privilege = "sysdba"
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected SYSDBA bootstrap settings, got %v", err)
	}
	if got := inst.Spec.Database.Bootstrap.AdminCredentials.Privilege; got != "SYSDBA" {
		t.Fatalf("expected bootstrap privilege to normalize to SYSDBA, got %q", got)
	}

	inst.Spec.Database.Bootstrap.AdminCredentials.Privilege = "SYSOPER"
	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err == nil || !strings.Contains(err.Error(), "privilege must be NORMAL or SYSDBA") {
		t.Fatalf("expected invalid bootstrap privilege rejection, got %v", err)
	}
}

func TestAgentFactoryValidateAllowsWalletBootstrap(t *testing.T) {
	enabled := true
	inst := validAgentFactoryForValidation()
	inst.Spec.Database.ConnectionType = "Wallet"
	inst.Spec.Database.Details = nil
	inst.Spec.Database.Wallet = &privateaiv4.AgentFactoryWalletSpec{
		SelectedTnsAlias:   "ORCLPDB1_TCPS",
		HasOciCertificates: agentFactoryBoolPtr(false),
	}
	inst.Spec.Database.Bootstrap = &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
		Enabled: &enabled,
		AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
			SecretName: "paf-bootstrap-admin",
			Privilege:  "SYSDBA",
		},
	}

	if err := (&AgentFactoryReconciler{}).validateAgentFactory(inst); err != nil {
		t.Fatalf("expected Wallet bootstrap settings to validate, got %v", err)
	}
}

func TestAgentFactoryDatabaseSetupCreatesJob(t *testing.T) {
	enabled := true
	instance := validAgentFactoryForValidation()
	instance.Spec.Database.Bootstrap = &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
		Enabled: &enabled,
		AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
			SecretName: "paf-bootstrap-admin",
		},
	}
	runtimeSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-runtime-secret", Namespace: instance.Namespace},
		Data:       map[string][]byte{"paf_db_password": []byte("runtime-password")},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "paf-bootstrap-admin", Namespace: instance.Namespace},
		Data:       map[string][]byte{"username": []byte("SYSTEM"), "password": []byte("admin-password")},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	if err := batchv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding batch scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(instance, runtimeSecret, adminSecret).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}
	ready, err := r.reconcileAgentFactoryDatabaseSetup(context.Background(), instance)
	if err != nil {
		t.Fatalf("database setup reconcile failed: %v", err)
	}
	if ready {
		t.Fatal("expected database setup to remain pending after Job creation")
	}
	job := &batchv1.Job{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "agentfactory-sample-db-setup", Namespace: instance.Namespace}, job); err != nil {
		t.Fatalf("expected setup Job: %v", err)
	}
	if job.Spec.Template.Spec.Containers[0].Command[0] != aicommons.AgentFactoryDatabaseSetupCommand {
		t.Fatalf("unexpected setup command: %#v", job.Spec.Template.Spec.Containers[0].Command)
	}
	setupScript := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: aicommons.AgentFactoryDatabaseSetupScriptConfigMapName(instance), Namespace: instance.Namespace}, setupScript); err != nil {
		t.Fatalf("expected controller-provided setup script ConfigMap: %v", err)
	}
	if setupScript.Data[aicommons.AgentFactoryDatabaseSetupScriptKey] == "" {
		t.Fatal("expected setup script ConfigMap data")
	}
	if job.Annotations[aicommons.AgentFactoryDatabaseSetupScriptHashLabel] != aicommons.AgentFactoryDatabaseSetupScriptHash() {
		t.Fatalf("expected setup script hash annotation, got %#v", job.Annotations)
	}
}

func TestAgentFactoryDatabaseSetupRejectsPrivilegeInUsername(t *testing.T) {
	enabled := true
	instance := validAgentFactoryForValidation()
	instance.Spec.Database.Bootstrap = &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
		Enabled: &enabled,
		AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
			SecretName: "paf-bootstrap-admin",
			Privilege:  "SYSDBA",
		},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "paf-bootstrap-admin", Namespace: instance.Namespace},
		Data:       map[string][]byte{"username": []byte("SYS AS SYSDBA"), "password": []byte("admin-password")},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	r := &AgentFactoryReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(adminSecret).Build(),
		Scheme: sch,
	}
	if err := r.validateAgentFactoryBootstrapSecret(context.Background(), instance); err == nil || !strings.Contains(err.Error(), "must not include an Oracle connection privilege") {
		t.Fatalf("expected connection privilege username rejection, got %v", err)
	}
}

func TestAgentFactoryDatabaseSetupDeletesAdminSecretOnSuccess(t *testing.T) {
	enabled := true
	instance := validAgentFactoryForValidation()
	instance.Spec.Database.Bootstrap = &privateaiv4.AgentFactoryDatabaseBootstrapSpec{
		Enabled:       &enabled,
		CleanupPolicy: "DeleteOnSuccess",
		AdminCredentials: &privateaiv4.AgentFactoryAdminCredentialsSpec{
			SecretName: "paf-bootstrap-admin",
		},
	}
	runtimeSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-runtime-secret", Namespace: instance.Namespace},
		Data:       map[string][]byte{"paf_db_password": []byte("runtime-password")},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "paf-bootstrap-admin", Namespace: instance.Namespace},
		Data:       map[string][]byte{"username": []byte("SYSTEM"), "password": []byte("admin-password")},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	if err := batchv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding batch scheme: %v", err)
	}
	job := aicommons.BuildAgentFactoryDatabaseSetupJob(instance)
	job.Status.Succeeded = 1
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(instance, runtimeSecret, adminSecret, job).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}
	ready, err := r.reconcileAgentFactoryDatabaseSetup(context.Background(), instance)
	if err != nil {
		t.Fatalf("database setup reconcile failed: %v", err)
	}
	if !ready || instance.Status.Database.Phase != agentFactoryDatabasePhaseReady {
		t.Fatalf("expected ready database setup, got ready=%t status=%#v", ready, instance.Status.Database)
	}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: adminSecret.Name, Namespace: adminSecret.Namespace}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected admin Secret to be deleted, got %v", err)
	}
}

func TestValidateAgentFactoryUpgrade(t *testing.T) {
	instance := validAgentFactoryForValidation()
	instance.Spec.Upgrade = &privateaiv4.AgentFactoryUpgradeSpec{Strategy: "imageupgrade", ImageUpgrade: &privateaiv4.AgentFactoryImageUpgradeSpec{}}
	if err := validateAgentFactoryUpgrade(instance); err != nil {
		t.Fatalf("unexpected ImageUpgrade validation error: %v", err)
	}
	if instance.Spec.Upgrade.Strategy != "ImageUpgrade" {
		t.Fatalf("expected canonical ImageUpgrade strategy, got %q", instance.Spec.Upgrade.Strategy)
	}

	instance.Spec.Upgrade.Strategy = "RollingUpdate"
	if err := validateAgentFactoryUpgrade(instance); err == nil {
		t.Fatal("expected imageUpgrade configuration conflict")
	}

	instance.Spec.Upgrade = &privateaiv4.AgentFactoryUpgradeSpec{Strategy: "ImageUpgrade", ImageUpgrade: &privateaiv4.AgentFactoryImageUpgradeSpec{ActiveDeadlineSeconds: int64Ptr(0)}}
	if err := validateAgentFactoryUpgrade(instance); err == nil {
		t.Fatal("expected invalid image-upgrade deadline")
	}
}

func TestReconcileAgentFactoryImageUpgradeUsesNewImageAndRestoresDeployment(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Generation = 1
	inst.Spec.Runtime.Image.Name = "paf:new"
	inst.Spec.Runtime.Replicas = int32Ptr(2)
	inst.Spec.Upgrade = &privateaiv4.AgentFactoryUpgradeSpec{Strategy: "ImageUpgrade"}
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: inst.Name, Namespace: inst.Namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agentfactory", Image: "paf:old"}}}}},
	}

	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}

	handled, err := r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || !handled {
		t.Fatalf("expected scale-to-zero phase, handled=%t err=%v", handled, err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("test fixture replicas mutated unexpectedly: %#v", deployment.Spec.Replicas)
	}
	updatedDeployment := &appsv1.Deployment{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, updatedDeployment); err != nil {
		t.Fatalf("failed reading scaled Deployment: %v", err)
	}
	if updatedDeployment.Spec.Replicas == nil || *updatedDeployment.Spec.Replicas != 0 {
		t.Fatalf("expected Deployment scaled to zero, got %#v", updatedDeployment.Spec.Replicas)
	}

	handled, err = r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || !handled {
		t.Fatalf("expected upgrade Pod creation, handled=%t err=%v", handled, err)
	}
	upgradePod := &corev1.Pod{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: aicommons.AgentFactoryImageUpgradePodName(inst), Namespace: inst.Namespace}, upgradePod); err != nil {
		t.Fatalf("expected upgrade Pod: %v", err)
	}
	if upgradePod.Spec.Containers[0].Image != "paf:new" || len(upgradePod.Spec.Containers[0].Args) != 1 || upgradePod.Spec.Containers[0].Args[0] != "upgrade" {
		t.Fatalf("unexpected upgrade Pod command: image=%q args=%#v", upgradePod.Spec.Containers[0].Image, upgradePod.Spec.Containers[0].Args)
	}

	upgradePod.Status.Phase = corev1.PodSucceeded
	if err := cl.Status().Update(context.Background(), upgradePod); err != nil {
		t.Fatalf("failed marking upgrade Pod succeeded: %v", err)
	}
	handled, err = r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || !handled || inst.Status.ImageUpgrade.Phase != agentFactoryImageUpgradePhaseSucceeded {
		t.Fatalf("expected successful upgrade cleanup, handled=%t phase=%q err=%v", handled, inst.Status.ImageUpgrade.Phase, err)
	}

	handled, err = r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || !handled {
		t.Fatalf("expected Deployment restoration, handled=%t err=%v", handled, err)
	}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, updatedDeployment); err != nil {
		t.Fatalf("failed reading restored Deployment: %v", err)
	}
	if updatedDeployment.Spec.Replicas == nil || *updatedDeployment.Spec.Replicas != 2 || updatedDeployment.Spec.Template.Spec.Containers[0].Image != "paf:new" {
		t.Fatalf("expected restored new-image Deployment, replicas=%#v image=%q", updatedDeployment.Spec.Replicas, updatedDeployment.Spec.Template.Spec.Containers[0].Image)
	}
	handled, err = r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || handled {
		t.Fatalf("expected ImageUpgrade to finish after Deployment restoration, handled=%t err=%v", handled, err)
	}
}

func TestReconcileAgentFactoryImageUpgradeRetainsFailedPod(t *testing.T) {
	inst := validAgentFactoryForValidation()
	inst.Generation = 1
	inst.Spec.Runtime.Image.Name = "paf:new"
	inst.Spec.Upgrade = &privateaiv4.AgentFactoryUpgradeSpec{Strategy: "ImageUpgrade"}
	inst.Status.ImageUpgrade = privateaiv4.AgentFactoryImageUpgradeStatus{Phase: agentFactoryImageUpgradePhaseRunning, SourceImage: "paf:old", TargetImage: "paf:new", PodName: "agentfactory-sample-image-upgrade", ObservedGeneration: 1}
	zero := int32(0)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: inst.Name, Namespace: inst.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &zero, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agentfactory", Image: "paf:old"}}}}}}
	failedPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: inst.Status.ImageUpgrade.PodName, Namespace: inst.Namespace}, Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "UpgradeFailed"}}
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(deployment, failedPod).Build()
	r := &AgentFactoryReconciler{Client: cl, Scheme: sch}
	handled, err := r.reconcileAgentFactoryImageUpgrade(context.Background(), inst)
	if err != nil || !handled || inst.Status.ImageUpgrade.Phase != agentFactoryImageUpgradePhaseFailed {
		t.Fatalf("expected retained failed upgrade, handled=%t phase=%q err=%v", handled, inst.Status.ImageUpgrade.Phase, err)
	}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: failedPod.Name, Namespace: failedPod.Namespace}, &corev1.Pod{}); err != nil {
		t.Fatalf("expected failed upgrade Pod to be retained: %v", err)
	}
}

func int64Ptr(value int64) *int64 { return &value }

func int32Ptr(value int32) *int32 { return &value }

func agentFactoryBoolPtr(value bool) *bool { return &value }

func validAgentFactoryForValidation() *privateaiv4.AgentFactory {
	return &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "default"},
		Spec: privateaiv4.AgentFactorySpec{
			Runtime: &privateaiv4.AgentFactoryRuntimeSpec{
				Image: &privateaiv4.AgentFactoryImageSpec{Name: "agentfactory:test"},
			},
			Configuration: &privateaiv4.AgentFactoryConfigurationSpec{UIAdminUsername: "admin@example.com"},
			Database: &privateaiv4.AgentFactoryDatabaseSpec{
				ConnectionType: "Details",
				AirGapped:      agentFactoryBoolPtr(false),
				Username:       "appuser",
				Details: &privateaiv4.AgentFactoryDatabaseDetailsSpec{
					Protocol:    "TCP",
					Host:        "sidb-primary",
					Port:        1521,
					ServiceName: "prodpdb",
				},
			},
			Security: &privateaiv4.AgentFactorySecuritySpec{
				Secret: &privateaiv4.AgentFactorySecretSpec{
					Name:               "agentfactory-runtime-secret",
					UIAdminPasswordKey: "paf_ui_admin_password",
					DBPasswordKey:      "paf_db_password",
				},
			},
			Storage: &privateaiv4.AgentFactoryStorageSpec{
				Volumes: []privateaiv4.AgentFactoryVolumeSpec{{
					Name:      "app-mount",
					MountPath: "/mount",
					Source: privateaiv4.AgentFactoryVolumeSource{
						ExistingPVC: &privateaiv4.AgentFactoryExistingPVCSource{ClaimName: "stage-fss-pvc"},
					},
				}},
			},
			Networking: &privateaiv4.AgentFactoryNetworkingSpec{
				Service: &privateaiv4.PaiServiceSpec{},
			},
		},
	}
}
func TestValidateAgentFactoryInboundTLS(t *testing.T) {
	instance := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
		Security: &privateaiv4.AgentFactorySecuritySpec{InboundTLS: &privateaiv4.AgentFactoryInboundTLSSpec{}},
	}}
	if err := validateAgentFactoryInboundTLS(instance); err == nil {
		t.Fatal("expected inboundTLS.secretName validation error")
	}

	instance.Spec.Security.InboundTLS.SecretName = "agentfactory-server-tls"
	if err := validateAgentFactoryInboundTLS(instance); err != nil {
		t.Fatalf("unexpected inbound TLS validation error: %v", err)
	}
	inboundTLS := instance.Spec.Security.InboundTLS
	if inboundTLS.MountLocation != aicommons.AgentFactoryDefaultInboundTLSMount || inboundTLS.CertificateKey != "tls.crt" || inboundTLS.PrivateKeyKey != "tls.key" {
		t.Fatalf("expected inbound TLS defaults, got %#v", inboundTLS)
	}

	inboundTLS.MountLocation = "relative/path"
	if err := validateAgentFactoryInboundTLS(instance); err == nil {
		t.Fatal("expected inbound TLS mount path validation error")
	}
	inboundTLS.MountLocation = aicommons.AgentFactoryDefaultInboundTLSMount
	inboundTLS.CertificateKey = "nested/tls.crt"
	if err := validateAgentFactoryInboundTLS(instance); err == nil {
		t.Fatal("expected inbound TLS certificate key path validation error")
	}

	inboundTLS.CertificateKey = "tls.crt"
	instance.Spec.Runtime = &privateaiv4.AgentFactoryRuntimeSpec{Env: []privateaiv4.EnvironmentVariable{{Name: agentFactoryServerCertificateFileEnv, Value: "/tmp/cert"}}}
	if err := validateAgentFactoryInboundTLS(instance); err == nil {
		t.Fatal("expected controller-managed certificate environment override error")
	}
}

func TestReconcileAgentFactoryInboundTLSValidatesSecretKeys(t *testing.T) {
	instance := &privateaiv4.AgentFactory{
		ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-sample", Namespace: "default"},
		Spec: privateaiv4.AgentFactorySpec{Security: &privateaiv4.AgentFactorySecuritySpec{
			InboundTLS: &privateaiv4.AgentFactoryInboundTLSSpec{SecretName: "agentfactory-server-tls", CertificateKey: "tls.crt", PrivateKeyKey: "tls.key"},
		}},
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	reconciler := &AgentFactoryReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "agentfactory-server-tls", Namespace: "default"}, Data: map[string][]byte{}},
	).Build()}
	if err := reconciler.reconcileAgentFactoryInboundTLS(context.Background(), instance); err == nil {
		t.Fatal("expected missing inbound TLS certificate key error")
	}
	condition := meta.FindStatusCondition(instance.Status.Conditions, agentFactoryInboundTLSReady)
	if condition == nil || condition.Reason != "CertificateKeyMissing" {
		t.Fatalf("expected CertificateKeyMissing condition, got %#v", condition)
	}
}

func TestAgentFactoryInboundTLSImportPending(t *testing.T) {
	instance := &privateaiv4.AgentFactory{Spec: privateaiv4.AgentFactorySpec{
		Security: &privateaiv4.AgentFactorySecuritySpec{InboundTLS: &privateaiv4.AgentFactoryInboundTLSSpec{}},
	}, Status: privateaiv4.AgentFactoryStatus{InboundTLS: privateaiv4.AgentFactoryInboundTLSStatus{
		ResourceVersion: "2", ImportedResourceVersion: "1",
	}}}
	if !agentFactoryInboundTLSImportPending(instance) {
		t.Fatal("expected inbound TLS import to be pending")
	}
	instance.Status.InboundTLS.ImportedResourceVersion = "2"
	if agentFactoryInboundTLSImportPending(instance) {
		t.Fatal("did not expect inbound TLS import to remain pending")
	}
}
