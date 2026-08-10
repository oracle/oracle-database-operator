// Package diskcheck builds Kubernetes resources for ASM disk validation.
package diskcheck

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// NodeAffinityForHostnames builds node affinity from a fixed list of hostnames.
func NodeAffinityForHostnames(workerNodes []string) *corev1.NodeAffinity {
	if len(workerNodes) == 0 {
		return nil
	}
	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "kubernetes.io/hostname",
							Operator: corev1.NodeSelectorOpIn,
							Values:   workerNodes,
						},
					},
				},
			},
		},
	}
}

// NodeAffinityForSelectorMap builds node affinity from an exact-match selector map.
func NodeAffinityForSelectorMap(selector map[string]string) *corev1.NodeAffinity {
	if len(selector) == 0 {
		return nil
	}

	matchExpr := make([]corev1.NodeSelectorRequirement, 0, len(selector))
	for key, value := range selector {
		if key == "" || value == "" {
			continue
		}
		matchExpr = append(matchExpr, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{value},
		})
	}
	if len(matchExpr) == 0 {
		return nil
	}

	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{MatchExpressions: matchExpr},
			},
		},
	}
}

// BuildDiskCheckCommand creates the shell command used to validate ASM block devices.
func BuildDiskCheckCommand(disks []string) string {
	diskArray := strings.Join(disks, " ")
	return fmt.Sprintf(`
disks=(%s)
for disk in "${disks[@]}"; do
  real_disk=$(readlink -f "$disk")
  if [ -b "$real_disk" ]; then
    size_bytes=$(blockdev --getsize64 "$real_disk")
    size_gb=$((size_bytes / 1024 / 1024 / 1024))
    echo "{\"disk\":\"$disk\",\"valid\":true,\"sizeGb\":$size_gb}"
  else
    echo "{\"disk\":\"$disk\",\"valid\":false,\"sizeGb\":0}"
    exit 1
  fi
done
sleep 3600
`, diskArray)
}

// BuildLabelsForDaemonSet returns a generic disk-check label set for any CR
// that exposes Kubernetes object metadata.
func BuildLabelsForDaemonSet(owner metav1.Object, component string) map[string]string {
	name := ""
	if owner != nil {
		name = owner.GetName()
	}

	component = strings.ToLower(strings.TrimSpace(component))
	if component == "" {
		component = "disk-check"
	}

	return map[string]string{
		"app":                           "oracle-database-operator",
		"database.oracle.com/name":      name,
		"database.oracle.com/component": component,
		"cluster":                       component,
	}
}

// LabelSelectorForDaemonSet serializes the shared disk-check labels into a
// Kubernetes label selector string.
func LabelSelectorForDaemonSet(owner metav1.Object, component string) string {
	return labels.Set(BuildLabelsForDaemonSet(owner, component)).String()
}

// BuildDiskHostPathVolumes creates hostPath volume mounts and volumes for each disk.
func BuildDiskHostPathVolumes(disks []string, sanitizeName func(string) string) ([]corev1.VolumeMount, []corev1.Volume) {
	volumeMounts := make([]corev1.VolumeMount, 0, len(disks))
	volumes := make([]corev1.Volume, 0, len(disks))

	for _, disk := range disks {
		volName := sanitizeName(disk) + "-vol"
		hostPathType := corev1.HostPathBlockDev
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: disk,
		})
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: disk,
					Type: &hostPathType,
				},
			},
		})
	}

	return volumeMounts, volumes
}

// BuildDaemonSet assembles the disk-check daemonset object.
func BuildDaemonSet(namespace, image string, labels map[string]string, nodeAffinity *corev1.NodeAffinity, cmd string, mounts []corev1.VolumeMount, volumes []corev1.Volume) *appsv1.DaemonSet {
	privileged := true

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk-check-daemonset",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{NodeAffinity: nodeAffinity},
					Volumes:  volumes,
					Containers: []corev1.Container{
						{
							Name:    "disk-check",
							Image:   image,
							Command: []string{"/bin/bash", "-c"},
							Args:    []string{cmd},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"NET_ADMIN", "SYS_NICE", "SYS_RESOURCE",
										"AUDIT_WRITE", "NET_RAW", "AUDIT_CONTROL", "SYS_CHROOT",
									},
								},
							},
							VolumeMounts: mounts,
						},
					},
				},
			},
		},
	}
}
