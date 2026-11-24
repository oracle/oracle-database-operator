package connectionmanager

import (
	"reflect"
	"strings"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	connectionManagerMaintenanceShell            = "/bin/sh"
	connectionManagerMaintenanceLoop             = "while :; do sleep 3600; done"
	connectionManagerMaintenanceReadinessCommand = "exit 1"
)

func buildConnectionManagerDeployment(inst *networkv4.ConnectionManager, configChecksum, restSecretChecksum string) *appsv1.Deployment {
	labels := connectionManagerLabels(inst)
	annotations := map[string]string{"network.oracle.com/config-hash": configChecksum}
	if restSecretChecksum != "" {
		annotations["network.oracle.com/cman-rest-secret-hash"] = restSecretChecksum
	}
	volumeMounts, volumes := buildConnectionManagerVolumes(inst)
	container := corev1.Container{
		Name:            connectionManagerDeploymentName(inst),
		Image:           inst.Spec.Runtime.Image,
		ImagePullPolicy: imagePullPolicyOrDefault(inst.Spec.Runtime.ImagePullPolicy),
		Ports:           buildConnectionManagerContainerPorts(inst),
		Env:             buildConnectionManagerEnvVars(inst),
		VolumeMounts:    volumeMounts,
		SecurityContext: inst.Spec.Runtime.ContainerSecurityContext,
		ReadinessProbe:  connectionManagerTCPProbe(inst, 5),
		LivenessProbe:   connectionManagerTCPProbe(inst, 30),
	}
	if connectionManagerStartupScriptEnabled(inst) {
		container.Command = []string{connectionManagerStartupScriptMountDir + "/" + connectionManagerStartupScriptName}
	}
	if connectionManagerMaintenanceEnabled(inst) {
		container.Command = []string{connectionManagerMaintenanceShell}
		container.Args = []string{"-c", connectionManagerMaintenanceLoop}
		container.ReadinessProbe = connectionManagerMaintenanceReadinessProbe()
		container.LivenessProbe = nil
	}
	if inst.Spec.Runtime.Resources != nil {
		container.Resources = *inst.Spec.Runtime.Resources
	}
	podSpec := corev1.PodSpec{
		ServiceAccountName: inst.Spec.Runtime.ServiceAccountName,
		SecurityContext:    inst.Spec.Runtime.PodSecurityContext,
		NodeSelector:       inst.Spec.Runtime.NodeSelector,
		Affinity:           inst.Spec.Runtime.Affinity,
		Containers:         []corev1.Container{container},
		Volumes:            volumes,
	}
	if len(inst.Spec.Runtime.ImagePullSecrets) > 0 {
		podSpec.ImagePullSecrets = make([]corev1.LocalObjectReference, 0, len(inst.Spec.Runtime.ImagePullSecrets))
		for _, name := range inst.Spec.Runtime.ImagePullSecrets {
			if strings.TrimSpace(name) == "" {
				continue
			}
			podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
		}
	}
	strategyType := appsv1.RollingUpdateDeploymentStrategyType
	if connectionManagerMaintenanceEnabled(inst) {
		strategyType = appsv1.RecreateDeploymentStrategyType
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectionManagerDeploymentName(inst),
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(inst.Spec.Runtime.Replicas),
			Strategy: appsv1.DeploymentStrategy{Type: strategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: podSpec,
			},
		},
	}
}

func connectionManagerTCPProbe(inst *networkv4.ConnectionManager, initialDelaySeconds int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(connectionManagerPrimaryPort(inst)))},
		},
		InitialDelaySeconds: initialDelaySeconds,
		TimeoutSeconds:      2,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}
}

func connectionManagerMaintenanceReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: []string{connectionManagerMaintenanceShell, "-c", connectionManagerMaintenanceReadinessCommand}},
		},
		TimeoutSeconds:   2,
		PeriodSeconds:    10,
		FailureThreshold: 1,
	}
}

func syncConnectionManagerDeployment(found, desired *appsv1.Deployment) bool {
	if found == nil || desired == nil {
		return false
	}
	updated := false
	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.OwnerReferences, desired.OwnerReferences) {
		found.OwnerReferences = desired.OwnerReferences
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Replicas, desired.Spec.Replicas) {
		found.Spec.Replicas = desired.Spec.Replicas
		updated = true
	}
	if found.Spec.Strategy.Type != desired.Spec.Strategy.Type {
		found.Spec.Strategy = desired.Spec.Strategy
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Selector, desired.Spec.Selector) {
		found.Spec.Selector = desired.Spec.Selector
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Labels, desired.Spec.Template.Labels) {
		found.Spec.Template.Labels = desired.Spec.Template.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Annotations, desired.Spec.Template.Annotations) {
		found.Spec.Template.Annotations = desired.Spec.Template.Annotations
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		found.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.ImagePullSecrets, desired.Spec.Template.Spec.ImagePullSecrets) {
		found.Spec.Template.Spec.ImagePullSecrets = desired.Spec.Template.Spec.ImagePullSecrets
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.NodeSelector, desired.Spec.Template.Spec.NodeSelector) {
		found.Spec.Template.Spec.NodeSelector = desired.Spec.Template.Spec.NodeSelector
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Affinity, desired.Spec.Template.Spec.Affinity) {
		found.Spec.Template.Spec.Affinity = desired.Spec.Template.Spec.Affinity
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		found.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		updated = true
	}
	return updated
}
