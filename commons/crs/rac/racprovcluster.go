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
// Package commons provides RAC helper utilities aligned with docs/rac and Kubernetes guidance.
//
// Support:
//   - Operator user guide: docs/rac
//   - Kubernetes controller overview: https://kubernetes.io/docs/concepts/architecture/controller/
//
// Contributing:
//   - Repository guidelines: https://github.com/oracle/oracle-database-operator/blob/main/CONTRIBUTING.md
//   - Example manifests: https://github.com/oracle/oracle-database-operator/blob/main/docs/rac/provisioning/racdb_prov_quickstart.yaml
//
// Help:
//   - Issues tracker: https://github.com/oracle/oracle-database-operator/blob/main/README.md#help
//   - Sample CRD walkthrough: https://github.com/oracle/oracle-database-operator/blob/main/docs/rac/README.md

//nolint:unused // Legacy RAC provisioning helpers are retained for planned/optional flows.
package commons

// revive:disable:unused-parameter,var-declaration,exported
// Legacy RAC provisioning helper signatures are preserved for compatibility.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	utils "github.com/oracle/oracle-database-operator/commons/crs/rac/utils"
	sharedk8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	minContainerMemory = 16 * 1024 * 1024 * 1024 // 16 GB
	pageSize           = 4096                    // 4 KiB
	oneGB              = int64(1024 * 1024 * 1024)
	defaultSem         = "250 32000 100 128"
	defaultShmmni      = "4096"
)

// BuildStatefulSpecForRacCluster provides documentation for the BuildStatefulSpecForRacCluster function.
func BuildStatefulSpecForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	kClient client.Client,
) *appsv1.StatefulSetSpec {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	labels := buildLabelsForRac(instance, "RAC")
	swMode, swModeErr := instance.Spec.ResolveSwStorageMode()

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: utils.OraSubDomain,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: labels,
			},
			Spec: *buildPodSpecForRacCluster(kClient, instance, clusterSpec, nodeIndex),
		},
	}

	if swModeErr == nil && swMode == racdb.RacSwStorageStorageClass {
		sfsetspec.VolumeClaimTemplates = VolumeClaimTemplatesForRacCluster(instance, clusterSpec, nodeIndex)
	}
	sfsetspec.Template.Annotations = generateNetworkDetailsForCluster(instance, clusterSpec, nodeIndex, kClient)
	return sfsetspec
}

// buildPodSpecForRacCluster provides documentation for the buildPodSpecForRacCluster function.
func buildPodSpecForRacCluster(
	kClient client.Client,
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) *corev1.PodSpec {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)

	spec := &corev1.PodSpec{
		Hostname:       nodeName + "-0",
		Subdomain:      utils.OraSubDomain,
		InitContainers: buildInitContainerSpecForRacCluster(instance, clusterSpec, nodeIndex),
		Containers:     buildContainerSpecForRacCluster(instance, clusterSpec, nodeIndex),
		Volumes:        buildVolumeSpecForRacCluster(instance, clusterSpec, nodeIndex),
		Affinity:       getNodeAffinityForRacCluster(kClient, instance, clusterSpec, nodeIndex),
	}
	if instance.Spec.SecurityContext != nil {
		spec.SecurityContext = instance.Spec.SecurityContext.DeepCopy()
	} else {
		spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	spec.SecurityContext.Sysctls = mergeRacRequiredSysctls(spec.SecurityContext.Sysctls)
	// Add service account name if specified
	if instance.Spec.SrvAccountName != "" {
		spec.ServiceAccountName = instance.Spec.SrvAccountName
	}

	if len(instance.Spec.ImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: instance.Spec.ImagePullSecret},
		}
	}
	return spec
}

func mergeRacRequiredSysctls(existing []corev1.Sysctl) []corev1.Sysctl {
	required := []corev1.Sysctl{
		{Name: "net.ipv4.conf.all.rp_filter", Value: "2"},
		{Name: "net.ipv4.conf.default.rp_filter", Value: "2"},
	}

	out := make([]corev1.Sysctl, 0, len(existing)+len(required))
	indexByName := make(map[string]int, len(existing)+len(required))

	for i := range existing {
		name := strings.TrimSpace(existing[i].Name)
		indexByName[name] = len(out)
		out = append(out, existing[i])
	}

	for i := range required {
		name := required[i].Name
		if _, ok := indexByName[name]; ok {
			continue
		}
		indexByName[name] = len(out)
		out = append(out, required[i])
	}

	return out
}

// CreateServiceAccountIfNotExists ensures the configured service account exists in the namespace.
func CreateServiceAccountIfNotExists(instance *racdb.RacDatabase, kClient client.Client) error {
	return sharedk8sobjects.EnsureServiceAccountIfNotExists(context.Background(), kClient, instance.Namespace, instance.Spec.SrvAccountName)
}

// VolumeClaimTemplatesForRacCluster provides documentation for the VolumeClaimTemplatesForRacCluster function.
func VolumeClaimTemplatesForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.PersistentVolumeClaim {
	// Generate PVCs based on clusterSpec/nodeIndex and any shared config as needed.
	// For the basic scaffold, you could reuse a simplified, non-per-node structure.
	var claims []corev1.PersistentVolumeClaim

	// Iterate over each disk group and its disks
	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, diskName := range dg.Disks {
			pvcName := GetAsmPvcName(diskName, instance.Name)
			claims = append(claims, corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: instance.Namespace,
					Labels:    buildLabelsForRac(instance, "RAC"),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &instance.Spec.StorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("100Gi"),
						},
					},
				},
			})
		}
	}
	swMode, err := instance.Spec.ResolveSwStorageMode()
	if err == nil && swMode == racdb.RacSwStorageStorageClass {
		nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
		swPVCSizeInGi := instance.Spec.StorageSizeInGB
		if instance.Spec.SwLocStorageSizeInGb > 0 {
			swPVCSizeInGi = instance.Spec.SwLocStorageSizeInGb
		}
		if swPVCSizeInGi <= 0 {
			swPVCSizeInGi = 100
		}
		claims = append(claims, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GetClusterSwPvcName(nodeName),
				Namespace: instance.Namespace,
				Labels:    buildLabelsForRac(instance, "RAC"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", swPVCSizeInGi)),
					},
				},
			},
		})
	}
	return claims
}

// generateNetworkDetailsForCluster provides documentation for the generateNetworkDetailsForCluster function.
func generateNetworkDetailsForCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	kClient client.Client,
) map[string]string {

	ann := make(map[string]string)
	if clusterSpec == nil || len(clusterSpec.PrivateIPDetails) == 0 {
		return ann
	}

	var networks []map[string]interface{}

	for _, netconf := range clusterSpec.PrivateIPDetails {

		ipam, err := getNadIPAM(instance.Namespace, netconf.Name, kClient)
		if err != nil {
			continue // fallback to NAD defaults
		}

		// Compute per-node IP
		finalIP, err := addOffsetToIPv4(ipam.RangeStart, nodeIndex)
		if err != nil {
			continue
		}

		_, cidr, _ := net.ParseCIDR(ipam.Subnet)
		prefix, _ := cidr.Mask.Size()

		networks = append(networks, map[string]interface{}{
			"name":      netconf.Name,
			"namespace": instance.Namespace,
			"interface": netconf.Interface,
			"ips":       []string{fmt.Sprintf("%s/%d", finalIP, prefix)},
		})
	}

	jsonData, _ := json.Marshal(networks)
	ann["k8s.v1.cni.cncf.io/networks"] = string(jsonData)

	return ann
}

// getNadIPAM provides documentation for the getNadIPAM function.
func getNadIPAM(namespace, name string, kClient client.Client) (*racdb.MacvlanIPAM, error) {
	nad := &unstructured.Unstructured{}
	nad.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "k8s.cni.cncf.io",
		Version: "v1",
		Kind:    "NetworkAttachmentDefinition",
	})

	err := kClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nad)
	if err != nil {
		return nil, err
	}

	configStr, found, err := unstructured.NestedString(nad.Object, "spec", "config")
	if err != nil || !found {
		return nil, fmt.Errorf("NAD spec.config not found")
	}

	var mv *racdb.MacvlanConfig
	if err := json.Unmarshal([]byte(configStr), &mv); err != nil {
		return nil, fmt.Errorf("failed to decode NAD config JSON: %v", err)
	}

	return &mv.IPAM, nil
}

// addOffsetToIPv4 provides documentation for the addOffsetToIPv4 function.
func addOffsetToIPv4(baseIP string, offset int) (string, error) {
	ip := net.ParseIP(baseIP).To4()
	if ip == nil {
		return "", fmt.Errorf("invalid IPv4: %s", baseIP)
	}

	ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	ipInt += uint32(offset)

	return fmt.Sprintf("%d.%d.%d.%d",
		byte(ipInt>>24),
		byte(ipInt>>16),
		byte(ipInt>>8),
		byte(ipInt),
	), nil
}

// buildInitContainerSpecForRacCluster provides documentation for the buildInitContainerSpecForRacCluster function.
func buildInitContainerSpecForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.Container {
	var result []corev1.Container
	privFlag := true
	var uid int64 = 0

	var scriptsCmd, scriptsLocation string

	if len(instance.Spec.ScriptsGetCmd) != 0 {
		scriptsCmd = instance.Spec.ScriptsGetCmd
	} else {
		scriptsCmd = "/bin/true"
	}

	if len(instance.Spec.ScriptsLocation) != 0 {
		scriptsLocation = instance.Spec.ScriptsLocation
	} else {
		scriptsLocation = "NOLOC"
	}

	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)

	init1spec := corev1.Container{
		Name:  nodeName + "-init1",
		Image: instance.Spec.Image,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privFlag,
			RunAsUser:  &uid,
		},
		Command: []string{
			"/bin/bash",
			"-c",
			getRacInitContainerCmd(scriptsCmd, instance.Name, scriptsLocation, clusterSpec.PrivateIPDetails[0].Interface, clusterSpec.PrivateIPDetails[1].Interface),
		},
		VolumeMounts: buildVolumeMountSpecForRacCluster(instance, clusterSpec, nodeIndex),
	}
	if instance.Spec.ImagePullPolicy != nil {
		init1spec.ImagePullPolicy = *instance.Spec.ImagePullPolicy
	}
	result = append(result, init1spec)
	return result
}

// buildContainerSpecForRacCluster provides documentation for the buildContainerSpecForRacCluster function.
func buildContainerSpecForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.Container {

	var result []corev1.Container
	privileged := false

	// --- Probe tuning ---
	readinessFailureThreshold := int32(3)
	readinessPeriodSeconds := int32(5)
	readinessInitialDelay := int32(180)

	// Startup probe: allow slow CRS / SCAN bring-up after reboot
	// startupFailureThreshold := int32(60) // 60 * 5s = 5 minutes
	// startupPeriodSeconds := int32(5)

	// Get the name for this node
	nodeName := fmt.Sprintf("%s%d-0", clusterSpec.RacNodeName, nodeIndex+1)

	// Local listener port (default 1522)
	oraLsnrPort := 1522
	if clusterSpec.BaseLsnrTargetPort != 0 {
		oraLsnrPort = int(clusterSpec.BaseLsnrTargetPort)
	}

	containerSpec := corev1.Container{
		Name:  nodeName,
		Image: instance.Spec.Image,

		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN", "SYS_NICE", "SYS_RESOURCE",
					"AUDIT_WRITE", "NET_RAW", "AUDIT_CONTROL", "SYS_CHROOT",
				},
			},
		},

		Command: []string{"/usr/sbin/init"},

		VolumeDevices: getAsmVolumeDevicesForCluster(instance, clusterSpec, nodeIndex),
		VolumeMounts:  buildVolumeMountSpecForRacCluster(instance, clusterSpec, nodeIndex),

		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},

		// // -------------------------------
		// // Startup probe → SCAN (1521)
		// // -------------------------------
		// StartupProbe: &corev1.Probe{
		// 	FailureThreshold: startupFailureThreshold,
		// 	PeriodSeconds:    startupPeriodSeconds,
		// 	ProbeHandler: corev1.ProbeHandler{
		// 		TCPSocket: &corev1.TCPSocketAction{
		// 			Port: intstr.FromInt(1521),
		// 		},
		// 	},
		// },

		// --------------------------------
		// Readiness probe → local listener
		// --------------------------------
		ReadinessProbe: &corev1.Probe{
			FailureThreshold:    readinessFailureThreshold,
			PeriodSeconds:       readinessPeriodSeconds,
			InitialDelaySeconds: readinessInitialDelay,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(oraLsnrPort),
				},
			},
		},
	}

	// Optional overrides
	if instance.Spec.Resources != nil {
		containerSpec.Resources = *instance.Spec.Resources
	}

	if len(instance.Spec.EnvVars) > 0 {
		containerSpec.Env = buildEnvVarsSpec(instance.Spec.EnvVars)
	}

	// Allow user override of readiness probe
	if instance.Spec.ReadinessProbe != nil {
		containerSpec.ReadinessProbe = instance.Spec.ReadinessProbe
	}

	containerSpec.Ports = buildContainerPortsDefForCluster(instance, clusterSpec, nodeIndex)

	result = []corev1.Container{containerSpec}
	return result
}

// getAsmVolumeDevicesForCluster provides documentation for the getAsmVolumeDevicesForCluster function.
func getAsmVolumeDevicesForCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.VolumeDevice {
	var result []corev1.VolumeDevice
	seen := make(map[string]struct{}) // Track seen disk names

	if instance.Spec.AsmStorageDetails != nil {
		for _, dg := range instance.Spec.AsmStorageDetails {
			for _, diskName := range dg.Disks {
				if _, exists := seen[diskName]; exists {
					continue // Skip duplicate
				}
				seen[diskName] = struct{}{}

				pvcName := GetAsmPvcName(diskName, instance.Name)
				result = append(result, corev1.VolumeDevice{
					Name:       pvcName,
					DevicePath: diskName,
				})
			}
		}
	}

	return result
}

// buildVolumeSpecForRacCluster provides documentation for the buildVolumeSpecForRacCluster function.
func buildVolumeSpecForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.Volume {
	var result []corev1.Volume

	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)

	// SSH secret
	result = append(result, corev1.Volume{
		Name: nodeName + "-ssh-secretmap-vol",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: instance.Spec.SshKeySecret.Name,
			},
		},
	})

	// SHM shared memory
	result = append(result, corev1.Volume{
		Name: nodeName + "-oradshm-vol",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
		},
	})

	// Scripts (optional)
	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-scripts-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// DB secrets
	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		result = append(result, corev1.Volume{
			Name: nodeName + "-dbsecret-pwd-vol",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.Name},
			},
		})
		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{
				Name: nodeName + "-dbsecret-key-vol",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.KeySecretName},
				},
			})
		}
	}

	// TDE secrets
	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		result = append(result, corev1.Volume{
			Name: nodeName + "-tdesecret-pwd-vol",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.Name},
			},
		})
		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{
				Name: nodeName + "-tdesecret-key-vol",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.KeySecretName},
				},
			})
		}
	}
	// Grid response and DB response ConfigMaps
	cfg := instance.Spec.ConfigParams

	if cfg != nil {
		// GRID RESPONSE FILE VOLUME
		if cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {
			result = append(result, corev1.Volume{
				Name: nodeName + "-oradata-girsp",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cfg.GridResponseFile.ConfigMapName,
						},
					},
				},
			})
		}

		// DB RESPONSE FILE VOLUME
		if cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {
			result = append(result, corev1.Volume{
				Name: nodeName + "-oradata-dbrsp",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cfg.DbResponseFile.ConfigMapName,
						},
					},
				},
			})

		}
	}

	// Environment file ConfigMap (often named per node)
	cmName := nodeName + instance.Name + "-cmap"

	result = append(result, corev1.Volume{
		Name: nodeName + "-oradata-envfile",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cmName,
				},
			},
		},
	})

	// Host software volumes from ClusterDetails
	swMode, swModeErr := instance.Spec.ResolveSwStorageMode()
	if swModeErr == nil && swMode == racdb.RacSwStorageHostPath {
		hostPath := fmt.Sprintf("%s/%s", clusterSpec.RacHostSwLocation, nodeName)
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-sw-vol",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
				},
			},
		})
	} else if swModeErr == nil && swMode == racdb.RacSwStorageExistingPVC {
		pvcName := fmt.Sprintf("%s%d", instance.Spec.RacSwPrefix, nodeIndex+1)
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-sw-vol",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	} else if swModeErr == nil && swMode == racdb.RacSwStorageStorageClass {
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-sw-vol",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: GetClusterSwPvcName(nodeName),
				},
			},
		})
	}

	// Optionally, software staging source.
	swStageMode, swStageErr := instance.Spec.ConfigParams.ResolveSwStageMode()
	if swStageErr == nil && swStageMode == racdb.RacSwStageExistingPVC {
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-swstage-vol",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: instance.Spec.ConfigParams.SwStagePvc,
				},
			},
		})
	} else if swStageErr == nil && swStageMode == racdb.RacSwStageHostPath {
		result = append(result, corev1.Volume{
			Name: nodeName + "-oradata-swstage-vol",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: instance.Spec.ConfigParams.HostSwStageLocation,
				},
			},
		})
	}

	// Optionally, OPatch/RU locations
	if instance.Spec.ConfigParams != nil {
		if len(instance.Spec.ConfigParams.RuPatchLocation) != 0 {
			result = append(result, corev1.Volume{
				Name: nodeName + "-oradata-rupatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.RuPatchLocation,
					},
				},
			})
		}
		if len(instance.Spec.ConfigParams.OPatchLocation) != 0 {
			result = append(result, corev1.Volume{
				Name: nodeName + "-oradata-opatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.OPatchLocation,
					},
				},
			})
		}
		if instance.Spec.ConfigParams.OneOffLocation != "" {
			result = append(result, corev1.Volume{
				Name: nodeName + "-oradata-oneoff-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.OneOffLocation,
					},
				},
			})
		}
	}

	// PVCs for ASM disks (as needed, with deduplication)
	if instance.Spec.AsmStorageDetails != nil {
		seen := make(map[string]struct{})
		for _, dg := range instance.Spec.AsmStorageDetails {
			for _, diskName := range dg.Disks {
				pvcName := GetAsmPvcName(diskName, instance.Name)
				if _, exists := seen[pvcName]; exists {
					continue // Skip duplicate PVCs
				}
				seen[pvcName] = struct{}{}
				result = append(result, corev1.Volume{
					Name: pvcName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				})
			}
		}
	}

	// Always add an emptyDir for writable envfile
	result = append(result, corev1.Volume{
		Name: nodeName + "-oradata-envfile-writable",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	return result
}

// getNodeAffinityForRacCluster provides documentation for the getNodeAffinityForRacCluster function.
func getNodeAffinityForRacCluster(
	kClient client.Client,
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) *corev1.Affinity {

	if clusterSpec == nil || len(clusterSpec.WorkerNodeSelector) == 0 {
		return nil
	}

	nodes, err := getNodesMatchingSelector(kClient, clusterSpec.WorkerNodeSelector)
	if err != nil || len(nodes) == 0 {
		return nil
	}

	sort.Strings(nodes)

	if nodeIndex >= len(nodes) {
		return nil
	}

	selectedNode := nodes[nodeIndex]

	var selectorExpr []corev1.NodeSelectorRequirement
	for k, v := range clusterSpec.WorkerNodeSelector {
		selectorExpr = append(selectorExpr, corev1.NodeSelectorRequirement{
			Key:      k,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{v},
		})
	}

	selectorExpr = append(selectorExpr, corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/hostname",
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{selectedNode},
	})

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: selectorExpr},
				},
			},
		},
	}
}

// getNodesMatchingSelector provides documentation for the getNodesMatchingSelector function.
func getNodesMatchingSelector(c client.Client, selector map[string]string) ([]string, error) {
	nodeList := &corev1.NodeList{}
	if err := c.List(context.Background(), nodeList); err != nil {
		return nil, err
	}

	var matched []string

	for _, node := range nodeList.Items {
		ok := true

		for key, value := range selector {
			if node.Labels[key] != value {
				ok = false
				break
			}
		}

		if ok {
			host, exists := node.Labels["kubernetes.io/hostname"]
			if exists {
				matched = append(matched, host)
			}
		}
	}

	return matched, nil
}

// buildVolumeMountSpecForRacCluster provides documentation for the buildVolumeMountSpecForRacCluster function.
func buildVolumeMountSpecForRacCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.VolumeMount {
	var result []corev1.VolumeMount

	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)

	// Ensure image pull policy is set
	if instance.Spec.ImagePullPolicy == nil || *instance.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		instance.Spec.ImagePullPolicy = &policy
	}

	// SSH key secret default location
	if instance.Spec.SshKeySecret != nil && instance.Spec.SshKeySecret.KeyMountLocation == "" {
		instance.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSSHSecretMount
	}
	// DB secret default locations
	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		if instance.Spec.DbSecret.PwdFileMountLocation == "" {
			instance.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
		}
		if instance.Spec.DbSecret.KeyFileMountLocation == "" {
			instance.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
		}
	}
	// TDE secret default locations
	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		if instance.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
		}
		if instance.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
		}
	}

	result = append(result, corev1.VolumeMount{
		Name:      nodeName + "-ssh-secretmap-vol",
		MountPath: instance.Spec.SshKeySecret.KeyMountLocation,
		ReadOnly:  true,
	})

	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{
				Name:      nodeName + "-dbsecret-key-vol",
				MountPath: instance.Spec.DbSecret.KeyFileMountLocation,
				ReadOnly:  true,
			})
		}
		if instance.Spec.DbSecret.Name != "" {
			result = append(result, corev1.VolumeMount{
				Name:      nodeName + "-dbsecret-pwd-vol",
				MountPath: instance.Spec.DbSecret.PwdFileMountLocation,
				ReadOnly:  true,
			})
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			result = append(result, corev1.VolumeMount{
				Name:      nodeName + "-tdesecret-pwd-vol",
				MountPath: instance.Spec.TdeWalletSecret.PwdFileMountLocation,
				ReadOnly:  true,
			})
		}
		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{
				Name:      nodeName + "-tdesecret-key-vol",
				MountPath: instance.Spec.TdeWalletSecret.KeyFileMountLocation,
				ReadOnly:  true,
			})
		}
	}

	// SHM mount
	result = append(result, corev1.VolumeMount{
		Name:      nodeName + "-oradshm-vol",
		MountPath: utils.OraShm,
	})

	cfg := instance.Spec.ConfigParams

	// GRID RESPONSE FILE VOLUME MOUNT
	if cfg != nil && cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-girsp",
			MountPath: utils.OraGiRsp,
		})
	}

	// DB RESPONSE FILE VOLUME MOUNT
	if cfg != nil && cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-dbrsp",
			MountPath: utils.OraDbRsp,
		})
	}

	// ENV file configMap mount (named consistently with buildVolumeSpecForRacCluster)
	result = append(result, corev1.VolumeMount{
		Name:      nodeName + "-oradata-envfile",
		MountPath: utils.OraEnvFile,
	})

	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-scripts-vol",
			MountPath: instance.Spec.ScriptsLocation,
		})
	}

	// Always add the shared/writable EmptyDir for environment file
	result = append(result, corev1.VolumeMount{
		Name:      nodeName + "-oradata-envfile-writable",
		MountPath: utils.OraWritableEnvFile,
	})

	// Host software location / existing PVC / StorageClass software volume
	swMode, swModeErr := instance.Spec.ResolveSwStorageMode()
	if swModeErr == nil && swMode != racdb.RacSwStorageNone {
		swMountPath := instance.Spec.ConfigParams.SwMountLocation
		if swMountPath == "" {
			swMountPath = utils.OraSwLocation
		}
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-sw-vol",
			MountPath: swMountPath,
		})
	}

	swStageMode, swStageErr := instance.Spec.ConfigParams.ResolveSwStageMode()
	if swStageErr == nil && swStageMode != racdb.RacSwStageNone {
		mountPath := utils.OraSwStageLocation
		if instance.Spec.ConfigParams.SwStagePvcMountLocation != "" {
			mountPath = instance.Spec.ConfigParams.SwStagePvcMountLocation
		}
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-swstage-vol",
			MountPath: mountPath,
		})
	}

	if instance.Spec.ConfigParams != nil && len(instance.Spec.ConfigParams.OPatchLocation) != 0 {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-opatch-vol",
			MountPath: utils.OraOPatchStageLocation,
		})
	}
	if instance.Spec.ConfigParams != nil && len(instance.Spec.ConfigParams.RuPatchLocation) != 0 {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-rupatch-vol",
			MountPath: utils.OraRuPatchStageLocation,
		})
	}
	if instance.Spec.ConfigParams.OneOffLocation != "" {
		result = append(result, corev1.VolumeMount{
			Name:      nodeName + "-oradata-oneoff-vol",
			MountPath: instance.Spec.ConfigParams.OneOffLocation,
		})
	}

	// PVC-based ASM devices: adapt as needed if you want per-node ASM
	// Otherwise, can leave as is or extend further later.

	// If you want to mount PVCs, can use same naming as in buildVolumeSpecForRacCluster

	return result
}

// buildContainerPortsDefForCluster provides documentation for the buildContainerPortsDefForCluster function.
func buildContainerPortsDefForCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) []corev1.ContainerPort {
	var result []corev1.ContainerPort

	// If you ever add per-cluster PortMappings[], use them here.
	// if len(clusterSpec.PortMappings) > 0 {
	//     for _, portMapping := range clusterSpec.PortMappings {
	//         cp := corev1.ContainerPort{
	//             Protocol:      portMapping.Protocol,
	//             ContainerPort: portMapping.Port,
	//             Name:          generatePortMapping(portMapping),
	//         }
	//         result = append(result, cp)
	//     }
	//     return result
	// }

	// Standard RAC container port set.
	result = append(result,
		corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraDBPort, Name: generateName(fmt.Sprintf("tcp-%d", utils.OraDBPort))},
		corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLsnrPort, Name: generateName(fmt.Sprintf("tcp-%d", utils.OraLsnrPort))},
		corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraSSHPort, Name: generateName(fmt.Sprintf("tcp-%d", utils.OraSSHPort))},
		corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLocalOnsPort, Name: generateName(fmt.Sprintf("tcp-%d", utils.OraLocalOnsPort))},
		corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraOemPort, Name: generateName(fmt.Sprintf("tcp-%d", utils.OraOemPort))},
	)

	// Optionally, add dynamic ONS/Listener ports if your clusterSpec specifies different ones per node:
	if clusterSpec.BaseLsnrTargetPort > 0 {
		port := clusterSpec.BaseLsnrTargetPort + int32(nodeIndex)
		result = append(result,
			corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: port, Name: generateName(fmt.Sprintf("tcp-%d", port))},
		)
	}
	if clusterSpec.BaseOnsTargetPort > 0 {
		port := clusterSpec.BaseOnsTargetPort + int32(nodeIndex)
		result = append(result,
			corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: port, Name: generateName(fmt.Sprintf("tcp-%d", port))},
		)
	}

	return result
}

//		service.Spec.Ports = buildRacSvcPortsDefForCluster(portList)
//		return service
//	}
//
// BuildClusterServiceDefForRac provides documentation for the BuildClusterServiceDefForRac function.
func BuildClusterServiceDefForRac(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	svctype string,
) *corev1.Service {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	objmeta := metav1.ObjectMeta{
		Name:      GetClusterSvcName(instance, clusterSpec, nodeIndex, svctype),
		Namespace: instance.Namespace,
		Labels:    buildLabelsForRac(instance, "RAC"),
	}

	svc := &corev1.Service{
		ObjectMeta: objmeta,
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
		},
	}

	svctypeUpper := strings.ToUpper(svctype)

	switch svctypeUpper {
	case "VIP", "LOCAL":
		svc.Spec.ClusterIP = corev1.ClusterIPNone
		svc.Spec.Selector = map[string]string{
			"statefulset.kubernetes.io/pod-name": nodeName + "-0",
		}
		// LEAVE .Ports nil/empty for strict legacy behavior

	case "SCAN":
		svc.Spec.ClusterIP = corev1.ClusterIPNone
		svc.Spec.Selector = buildLabelsForRac(instance, "RAC")
		// LEAVE .Ports nil/empty for strict legacy behavior
		// WARNING: Kubernetes 1.21+ may reject this; see below.

	default:
		svc.Spec.Selector = map[string]string{
			"statefulset.kubernetes.io/pod-name": nodeName + "-0",
		}
		// Dummy port for K8s compliance or future-proofing
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "default",
			Port:       1521,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(1521),
		}}
	}
	return svc
}

// BuildClusterExternalServiceDefForRac provides documentation for the BuildClusterExternalServiceDefForRac function.
func BuildClusterExternalServiceDefForRac(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	svctype string, // "lsnrsvc", "onssvc", "scansvc", or "nodeport"
	opType string, // same as above
) *corev1.Service {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	serviceName := GetClusterSvcName(instance, clusterSpec, nodeIndex, opType)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    buildLabelsForRac(instance, "RAC"),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"statefulset.kubernetes.io/pod-name": nodeName + "-0",
			},
			PublishNotReadyAddresses: true,
		},
	}

	var servicePorts []corev1.ServicePort

	switch opType {
	case "lsnrsvc":
		port := clusterSpec.BaseLsnrTargetPort + int32(nodeIndex)
		nodePort := port
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "lsnrsvc",
			Protocol:   corev1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
			NodePort:   nodePort,
		})
		service.Spec.Type = corev1.ServiceTypeNodePort

	case "onssvc":
		nodePort := clusterSpec.BaseOnsTargetPort + int32(nodeIndex)
		containerPort := int32(6200)
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "onssvc",
			Protocol:   corev1.ProtocolTCP,
			Port:       containerPort, // Always 6200 for ONS
			TargetPort: intstr.FromInt(int(containerPort)),
			NodePort:   nodePort, // External NodePort
		})
		service.Spec.Type = corev1.ServiceTypeNodePort

	case "scansvc":
		if instance.Spec.ScanSvcTargetPort != nil {
			nodePort := *instance.Spec.ScanSvcTargetPort

			// Use ScanSvcLocalPort if set, else default to 1521
			port := int32(1521)
			if instance.Spec.ScanSvcLocalPort != nil {
				port = *instance.Spec.ScanSvcLocalPort
			}

			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       "scansvc",
				Protocol:   corev1.ProtocolTCP,
				Port:       port,
				TargetPort: intstr.FromInt(int(port)),
				NodePort:   nodePort,
			})
			service.Spec.Type = corev1.ServiceTypeNodePort
		}

	case "nodeport":
		port := clusterSpec.BaseLsnrTargetPort + int32(nodeIndex)
		nodePort := port
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "nodeport",
			Protocol:   corev1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
			NodePort:   nodePort,
		})
		service.Spec.Type = corev1.ServiceTypeNodePort
	}

	service.Spec.Ports = servicePorts
	return service
}

// GetClusterSvcName provides documentation for the GetClusterSvcName function.
func GetClusterSvcName(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	svctype string,
) string {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	switch svctype {
	case "local":
		return nodeName + "-0"
	case "vip":
		return nodeName + "-0-vip"
	case "scan":
		return instance.Spec.ScanSvcName
	case "onssvc":
		return nodeName + "-0-ons"
	case "lsnrsvc":
		return nodeName + "-0-lsnr"
	case "scansvc":
		return instance.Spec.ScanSvcName + "-lsnr"
	default:
		return nodeName
	}
}
