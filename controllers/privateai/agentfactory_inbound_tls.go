/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package privateai

import (
	"context"
	"crypto/tls"
	"fmt"
	"path"
	"strings"
	"time"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	agentFactoryServerCertificateFileEnv = "AGENT_FACTORY_SERVER_CERTIFICATE_FILE"
	agentFactoryServerPrivateKeyFileEnv  = "AGENT_FACTORY_SERVER_PRIVATE_KEY_FILE"
)

func validateAgentFactoryInboundTLS(inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Runtime != nil {
		for _, item := range inst.Spec.Runtime.Env {
			switch strings.TrimSpace(item.Name) {
			case agentFactoryServerCertificateFileEnv, agentFactoryServerPrivateKeyFileEnv:
				return fmt.Errorf("runtime.env cannot override controller-managed %s or %s", agentFactoryServerCertificateFileEnv, agentFactoryServerPrivateKeyFileEnv)
			}
		}
	}

	if inst.Spec.Security == nil || inst.Spec.Security.InboundTLS == nil {
		return nil
	}
	inboundTLS := inst.Spec.Security.InboundTLS
	if strings.TrimSpace(inboundTLS.SecretName) == "" {
		return fmt.Errorf("spec.security.inboundTLS.secretName is required")
	}
	if strings.TrimSpace(inboundTLS.MountLocation) == "" {
		inboundTLS.MountLocation = aicommons.AgentFactoryDefaultInboundTLSMount
	}
	inboundTLS.MountLocation = strings.TrimRight(strings.TrimSpace(inboundTLS.MountLocation), "/")
	if !path.IsAbs(inboundTLS.MountLocation) || path.Clean(inboundTLS.MountLocation) != inboundTLS.MountLocation || inboundTLS.MountLocation == "/" {
		return fmt.Errorf("spec.security.inboundTLS.mountLocation must be an absolute, clean directory path")
	}
	if strings.TrimSpace(inboundTLS.CertificateKey) == "" {
		inboundTLS.CertificateKey = "tls.crt"
	}
	if strings.TrimSpace(inboundTLS.PrivateKeyKey) == "" {
		inboundTLS.PrivateKeyKey = "tls.key"
	}
	for field, key := range map[string]string{
		"certificateKey": inboundTLS.CertificateKey,
		"privateKeyKey":  inboundTLS.PrivateKeyKey,
	} {
		key = strings.TrimSpace(key)
		if key == "" || key == "." || key == ".." || strings.ContainsAny(key, "/\\\t\r\n") {
			return fmt.Errorf("spec.security.inboundTLS.%s must be a Secret key without path separators", field)
		}
	}
	if strings.TrimSpace(inboundTLS.CertificateKey) == strings.TrimSpace(inboundTLS.PrivateKeyKey) {
		return fmt.Errorf("spec.security.inboundTLS.certificateKey and privateKeyKey must be different")
	}
	return nil
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryInboundTLS(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	configured := inst.Spec.Security != nil && inst.Spec.Security.InboundTLS != nil
	if !configured {
		inst.Status.InboundTLS = privateaiv4.AgentFactoryInboundTLSStatus{}
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryInboundTLSReady,
			Status:             metav1.ConditionFalse,
			Reason:             "Disabled",
			Message:            "AgentFactory inbound TLS is not configured",
			ObservedGeneration: inst.Generation,
		})
		return nil
	}

	inboundTLS := inst.Spec.Security.InboundTLS
	secret := &corev1.Secret{}
	secretName := strings.TrimSpace(inboundTLS.SecretName)
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: inst.Namespace}, secret); err != nil {
		return r.setAgentFactoryInboundTLSFailure(inst, "SecretUnavailable", fmt.Sprintf("inbound TLS Secret %q is unavailable: %v", secretName, err))
	}
	certificateKey := strings.TrimSpace(inboundTLS.CertificateKey)
	privateKeyKey := strings.TrimSpace(inboundTLS.PrivateKeyKey)
	certificate, certificateOK := secret.Data[certificateKey]
	privateKey, privateKeyOK := secret.Data[privateKeyKey]
	if !certificateOK || len(certificate) == 0 {
		return r.setAgentFactoryInboundTLSFailure(inst, "CertificateKeyMissing", fmt.Sprintf("inbound TLS Secret %q must contain a non-empty key %q", secret.Name, certificateKey))
	}
	if !privateKeyOK || len(privateKey) == 0 {
		return r.setAgentFactoryInboundTLSFailure(inst, "PrivateKeyKeyMissing", fmt.Sprintf("inbound TLS Secret %q must contain a non-empty key %q", secret.Name, privateKeyKey))
	}
	if _, err := tls.X509KeyPair(certificate, privateKey); err != nil {
		return r.setAgentFactoryInboundTLSFailure(inst, "CertificateKeyMismatch", fmt.Sprintf("inbound TLS Secret %q certificate and private key do not form a valid pair: %v", secret.Name, err))
	}

	desired := privateaiv4.AgentFactoryInboundTLSStatus{
		SecretName:      secret.Name,
		ResourceVersion: secret.ResourceVersion,
		CertificateKey:  certificateKey,
		PrivateKeyKey:   privateKeyKey,
		MountLocation:   strings.TrimSpace(inboundTLS.MountLocation),
	}
	previous := inst.Status.InboundTLS
	if previous.SecretName == "" || previous.SecretName != desired.SecretName ||
		previous.CertificateKey != desired.CertificateKey || previous.PrivateKeyKey != desired.PrivateKeyKey ||
		previous.MountLocation != desired.MountLocation {
		// The initial configuration and a changed mount/key mapping are consumed
		// by the new Pod template. Secret data rotation is handled below through
		// aaictl importcert before the controller restarts the workload.
		desired.ImportedResourceVersion = desired.ResourceVersion
	} else {
		desired.ImportedResourceVersion = previous.ImportedResourceVersion
	}
	inst.Status.InboundTLS = desired
	if desired.ResourceVersion != desired.ImportedResourceVersion {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryInboundTLSReady,
			Status:             metav1.ConditionFalse,
			Reason:             agentFactoryReasonInboundTLSImportPending,
			Message:            "inbound TLS Secret changed; aaictl certificate import is pending",
			ObservedGeneration: inst.Generation,
		})
		return nil
	}
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryInboundTLSReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Validated",
		Message:            "inbound TLS certificate and private key are mounted into the AgentFactory workload",
		ObservedGeneration: inst.Generation,
	})
	return nil
}

func (r *AgentFactoryReconciler) setAgentFactoryInboundTLSFailure(inst *privateaiv4.AgentFactory, reason, message string) error {
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryInboundTLSReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inst.Generation,
	})
	return fmt.Errorf("%s", message)
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryInboundTLSImport(ctx context.Context, inst *privateaiv4.AgentFactory) (bool, error) {
	if inst.Spec.Security == nil || inst.Spec.Security.InboundTLS == nil {
		return false, nil
	}
	status := &inst.Status.InboundTLS
	if status.ResourceVersion == "" {
		return false, r.setAgentFactoryInboundTLSFailure(inst, "ImportPending", "inbound TLS Secret has not been validated")
	}
	if status.ResourceVersion == status.ImportedResourceVersion {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryInboundTLSReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Imported",
			Message:            "inbound TLS certificate is mounted and active",
			ObservedGeneration: inst.Generation,
		})
		return false, nil
	}

	pod, err := r.readyAgentFactoryPod(ctx, inst)
	if err != nil {
		return false, err
	}
	if pod == nil {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryInboundTLSReady,
			Status:             metav1.ConditionFalse,
			Reason:             agentFactoryReasonInboundTLSImportPending,
			Message:            "inbound TLS certificate import is waiting for a Ready AgentFactory pod",
			ObservedGeneration: inst.Generation,
		})
		return false, nil
	}

	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}}
	command := []string{
		"aaictl", "importcert",
		"-cert", aicommons.AgentFactoryInboundTLSCertificatePath(inst),
		"-key", aicommons.AgentFactoryInboundTLSPrivateKeyPath(inst),
	}
	if _, err := aicommons.ExecCommand(ctx, r.Client, r.Config, pod.Name, pod.Namespace, "agentfactory", request, false, command); err != nil {
		return false, r.setAgentFactoryInboundTLSFailure(inst, "ImportFailed", fmt.Sprintf("failed to import rotated inbound TLS certificate into Pod %q: %v", pod.Name, err))
	}

	if err := r.restartAgentFactoryDeployment(ctx, inst); err != nil {
		return false, r.setAgentFactoryInboundTLSFailure(inst, "RestartFailed", fmt.Sprintf("inbound TLS certificate imported but AgentFactory Deployment restart failed: %v", err))
	}
	status.ImportedResourceVersion = status.ResourceVersion
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryInboundTLSReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Imported",
		Message:            "rotated inbound TLS certificate imported; AgentFactory Pods will be restarted",
		ObservedGeneration: inst.Generation,
	})
	return true, nil
}

func (r *AgentFactoryReconciler) readyAgentFactoryPod(ctx context.Context, inst *privateaiv4.AgentFactory) (*corev1.Pod, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(inst.Namespace), client.MatchingLabels(aicommons.AgentFactoryLabels(inst))); err != nil {
		return nil, err
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning || !agentFactoryPodReady(pod) {
			continue
		}
		return pod, nil
	}
	return nil, nil
}

func agentFactoryPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *AgentFactoryReconciler) restartAgentFactoryDeployment(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, deployment); err != nil {
		return err
	}
	updated := deployment.DeepCopy()
	if updated.Spec.Template.Annotations == nil {
		updated.Spec.Template.Annotations = map[string]string{}
	}
	updated.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	return r.Patch(ctx, updated, client.MergeFrom(deployment))
}

func agentFactoryInboundTLSImportPending(inst *privateaiv4.AgentFactory) bool {
	return inst.Spec.Security != nil && inst.Spec.Security.InboundTLS != nil &&
		inst.Status.InboundTLS.ResourceVersion != "" &&
		inst.Status.InboundTLS.ResourceVersion != inst.Status.InboundTLS.ImportedResourceVersion
}

func (r *AgentFactoryReconciler) reconcileAgentFactoryInboundTLSPending(ctx context.Context, inst *privateaiv4.AgentFactory, restarted bool) (ctrl.Result, error) {
	if restarted {
		inst.Status.Phase = agentFactoryPhaseUpdating
		inst.Status.AppState = "updating"
	} else {
		inst.Status.Phase = agentFactoryPhasePending
		inst.Status.AppState = "pending"
	}
	message := "inbound TLS certificate import is pending"
	reason := agentFactoryReasonInboundTLSImportPending
	if restarted {
		reason = agentFactoryReasonInboundTLSRotation
		message = "inbound TLS certificate imported; AgentFactory Deployment restart is in progress"
	}
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryWorkloadReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inst.Generation,
	})
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               privateaiv4.ReconcileCompelete,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inst.Generation,
	})
	if err := r.updateAgentFactoryStatus(ctx, inst); err != nil {
		return ctrl.Result{}, err
	}
	return requeueY, nil
}
