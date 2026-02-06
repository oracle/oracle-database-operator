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

package commons

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	utils "github.com/oracle/oracle-database-operator/commons/rac/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants for rac-stateful StatefulSet & Volumes
// buildLabelsForRac creates basic labels shared by RAC resources.
// buildLabelsForRac provides documentation for the buildLabelsForRac function.
func buildLabelsForRac(instance *racdb.RacDatabase, label string) map[string]string {
	return map[string]string{
		"cluster": instance.Spec.ScanSvcName,
	}

	// "oralabel": getLabelForRac(instance),
}

// BuildLabelsForRac exposes label construction for external callers.
func BuildLabelsForRac(instance *racdb.RacDatabase, label string) map[string]string {
	return map[string]string{
		"cluster": instance.Spec.ScanSvcName,
	}

	// "oralabel": getLabelForRac(instance),
}

// BuildLabelsForDaemonSet prepares labels for RAC daemonset resources.
func BuildLabelsForDaemonSet(instance *racdb.RacDatabase, label string) map[string]string {
	return map[string]string{
		"cluster": label,
	}

}

// buildLabelsForAsmPv returns labels applied to ASM persistent volumes.
func buildLabelsForAsmPv(instance *racdb.RacDatabase, diskName string) map[string]string {
	return map[string]string{
		"asm_vol": "block-asm-pv-" + getLabelForRac(instance) + "-" + diskName[strings.LastIndex(diskName, "/")+1:],
	}
}

// getLabelForRac resolves the primary label identifier for RAC resources.
func getLabelForRac(instance *racdb.RacDatabase) string {

	//  if len(OraRacSpex.Label) !=0 {
	//     return OraRacSpex.Label
	//   }

	return instance.Name
}

// BuildStatefulSetForRac constructs the StatefulSet manifest for a RAC node.
func BuildStatefulSetForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfsetSpec, err := buildStatefulSpecForRac(instance, OraRacSpex, kClient)
	if err != nil {
		return nil, fmt.Errorf("failed to build StatefulSetSpec for OracleRestart %s: %v", OraRacSpex.Name, err)
	}

	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForRac(),
		ObjectMeta: builObjectMetaForRac(instance, OraRacSpex),
		Spec:       *sfsetSpec,
	}
	return sfset, nil
}

// Function to build TypeMeta
// buildTypeMetaForRac prepares standard StatefulSet type metadata.
// buildTypeMetaForRac provides documentation for the buildTypeMetaForRac function.
func buildTypeMetaForRac() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
// builObjectMetaForRac creates metadata with appropriate labels and namespace.
// builObjectMetaForRac provides documentation for the builObjectMetaForRac function.
func builObjectMetaForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:      OraRacSpex.Name,
		Namespace: instance.Namespace,
		Labels:    buildLabelsForRac(instance, "RAC"),
	}
	return objmeta
}

// Function to build Stateful Specs
// buildStatefulSpecForRac constructs the StatefulSet spec for RAC deployment.
// buildStatefulSpecForRac provides documentation for the buildStatefulSpecForRac function.
func buildStatefulSpecForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client) (*appsv1.StatefulSetSpec, error) {
	// building Stateful set Specs
	sfsetSpec, err := buildPodSpecForRac(instance, OraRacSpex, kClient)
	if err != nil {
		return nil, fmt.Errorf("failed to build StatefulSetSpec for OracleRestart %s: %v", OraRacSpex.Name, err)
	}

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: utils.OraSubDomain,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForRac(instance, "RAC"),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:   OraRacSpex.Name,
				Labels: buildLabelsForRac(instance, "RAC"),
			},
			Spec: *sfsetSpec,
		},
	}

	if len(instance.Spec.StorageClass) != 0 {
		sfsetspec.VolumeClaimTemplates = VolumeClaimTemplatesForRac(instance, OraRacSpex)
	}
	sfsetspec.Template.Annotations = generateNetworkDetails(instance, OraRacSpex, kClient)
	return sfsetspec, nil
}

// Function to build PodSpec

// buildPodSpecForRac prepares the pod spec, including containers, volumes, and sysctls.
func buildPodSpecForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client) (*corev1.PodSpec, error) {
	const hugePageSizeBytes int64 = 2 * 1024 * 1024

	var (
		sgaBytes          int64
		pgaBytes          int64
		containerMemBytes int64
		hugePagesBytes    int64
		userShmmax        int64
		userShmall        int64
		hasUserSysctls    bool
	)

	// Parse SGA and PGA sizes
	if instance.Spec.ConfigParams != nil {
		if instance.Spec.ConfigParams.SgaSize != "" {
			sgaBytes = parseSGASizeBytes(instance.Spec.ConfigParams.SgaSize)
		}

		if instance.Spec.ConfigParams.PgaSize != "" {
			pgaBytes = parseSGASizeBytes(instance.Spec.ConfigParams.PgaSize)
		}
	}

	// Parse container memory
	if instance != nil && instance.Spec.Resources != nil && instance.Spec.Resources.Limits != nil {
		if memQty, ok := instance.Spec.Resources.Limits["memory"]; ok {
			containerMemBytes = memQty.Value()
		}
	}

	// Parse HugePages
	if instance != nil && instance.Spec.Resources != nil && instance.Spec.Resources.Limits != nil {
		if hugeQty, ok := instance.Spec.Resources.Limits["hugepages-2Mi"]; ok {
			hugePagesBytes = hugeQty.Value()
		}
	}

	// Check for user-provided sysctls
	if instance.Spec.SecurityContext != nil && len(instance.Spec.SecurityContext.Sysctls) > 0 {
		for _, sysctl := range instance.Spec.SecurityContext.Sysctls {
			switch sysctl.Name {
			case "kernel.shmmax":
				userShmmax = parseSGASizeBytes(sysctl.Value)
				hasUserSysctls = true
			case "kernel.shmall":
				val, err := strconv.ParseInt(sysctl.Value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid user-provided kernel.shmall value: %v", err)
				}
				userShmall = val
				hasUserSysctls = true
			}
		}
	}

	// Compute sysctls if user did not provide them
	var sysctls []corev1.Sysctl
	var err error
	if !hasUserSysctls {
		sysctls, err = calculateSysctls(sgaBytes, pgaBytes, containerMemBytes, hugePagesBytes, userShmmax, userShmall)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate sysctls: %v", err)
		}
	} else {
		sysctls = instance.Spec.SecurityContext.Sysctls
	}
	// Ensure PodSecurityContext exists
	if instance.Spec.SecurityContext == nil {
		instance.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	instance.Spec.SecurityContext.Sysctls = sysctls

	spec := &corev1.PodSpec{
		Hostname:       OraRacSpex.Name + "-0",
		Subdomain:      utils.OraSubDomain,
		InitContainers: buildInitContainerSpecForRac(instance, OraRacSpex),
		Containers:     buildContainerSpecForRac(instance, OraRacSpex),
		Volumes:        buildVolumeSpecForRac(instance, OraRacSpex),
		Affinity:       getNodeAffinity(instance, OraRacSpex),
	}

	if instance.Spec.SecurityContext != nil {
		spec.SecurityContext = instance.Spec.SecurityContext
	}

	if len(instance.Spec.ImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: instance.Spec.ImagePullSecret,
			},
		}
	}
	return spec, nil
}

// parseSGASizeBytes parses memory config value ("16G", "16Gi", "1024M", "512Mi") and returns int64 bytes
// parseSGASizeBytes converts string SGA sizes like `16Gi` into bytes.
func parseSGASizeBytes(sga string) int64 {
	s := strings.ToUpper(strings.TrimSpace(sga))

	var multiplier int64
	switch {
	case strings.HasSuffix(s, "GI"):
		s = strings.TrimSuffix(s, "GI")
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(s, "MI"):
		s = strings.TrimSuffix(s, "MI")
		multiplier = 1024 * 1024
	case strings.HasSuffix(s, "GB"):
		s = strings.TrimSuffix(s, "GB")
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(s, "G"):
		s = strings.TrimSuffix(s, "G")
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(s, "M"):
		s = strings.TrimSuffix(s, "M")
		multiplier = 1024 * 1024
	default:
		// Unknown unit
		return 0
	}

	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}

	return val * multiplier
}

// calculateSysctls computes recommended kernel sysctl settings for RAC pods.
func calculateSysctls(
	sgaBytes, pgaBytes, containerMemBytes, hugePagesBytes, userShmmax, userShmall int64,
) ([]corev1.Sysctl, error) {

	//  Minimum container memory check (only if memory is given)
	if containerMemBytes > 0 && containerMemBytes < minContainerMemory {
		return nil, fmt.Errorf("container memory (%d) is less than minimum required (%d)", containerMemBytes, minContainerMemory)
	}

	// 2Case 1: No container memory and no SGA/PGA
	if containerMemBytes == 0 && sgaBytes == 0 && pgaBytes == 0 {
		return []corev1.Sysctl{}, nil
	}

	// Case 2: No container memory, but SGA/PGA provided
	if containerMemBytes == 0 {
		shmmax := sgaBytes + oneGB

		if userShmmax > 0 {
			// validate user-provided shmmax
			if userShmmax < sgaBytes {
				return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than SGA_TARGET (%d)", userShmmax, sgaBytes)
			}
			shmmax = userShmmax
		}

		shmall := (shmmax + pageSize - 1) / pageSize
		if userShmall > 0 {
			if userShmall < shmall {
				return nil, fmt.Errorf("user-provided shmall (%d) is too small; min required=%d pages", userShmall, shmall)
			}
			shmall = userShmall
		}

		return []corev1.Sysctl{
			{Name: "kernel.shmmax", Value: fmt.Sprintf("%d", shmmax)},
			{Name: "kernel.shmall", Value: fmt.Sprintf("%d", shmall)},
			{Name: "kernel.sem", Value: defaultSem},
			{Name: "kernel.shmmni", Value: defaultShmmni},
		}, nil
	}

	// Case 3: Container memory provided
	if pgaBytes > containerMemBytes {
		return nil, fmt.Errorf("PGA_TARGET (%d) cannot be greater than container memory (%d)", pgaBytes, containerMemBytes)
	}
	if sgaBytes > containerMemBytes {
		return nil, fmt.Errorf("SGA_TARGET (%d) cannot be greater than container memory (%d)", sgaBytes, containerMemBytes)
	}

	var shmmax int64
	if userShmmax > 0 {
		// Validate user-provided shmmax
		if userShmmax < sgaBytes {
			return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than SGA_TARGET (%d)", userShmmax, sgaBytes)
		}
		if hugePagesBytes > 0 && userShmmax < hugePagesBytes {
			return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than hugePages memory (%d)", userShmmax, hugePagesBytes)
		}
		if userShmmax > containerMemBytes-oneGB {
			return nil, fmt.Errorf("user-provided shmmax (%d) must be < container memory - 1GB (%d)", userShmmax, containerMemBytes-oneGB)
		}
		shmmax = userShmmax
	} else if hugePagesBytes > 0 {
		if hugePagesBytes < sgaBytes {
			return nil, fmt.Errorf("huge pages (%d) must be >= SGA_TARGET (%d)", hugePagesBytes, sgaBytes)
		}
		shmmax = hugePagesBytes
	} else if sgaBytes < (containerMemBytes / 2) {
		shmmax = containerMemBytes / 2
	} else {
		shmmax = sgaBytes + oneGB
	}

	// Ensure shmmax < container memory
	if shmmax >= containerMemBytes {
		shmmax = containerMemBytes - oneGB
	}

	// Compute shmall
	shmall := (shmmax + pageSize - 1) / pageSize
	if userShmall > 0 {
		if userShmall < shmall {
			return nil, fmt.Errorf("user-provided shmall (%d) is too small; min required=%d pages", userShmall, shmall)
		}
		shmall = userShmall
	}

	return []corev1.Sysctl{
		{Name: "kernel.shmmax", Value: fmt.Sprintf("%d", shmmax)},
		{Name: "kernel.shmall", Value: fmt.Sprintf("%d", shmall)},
		{Name: "kernel.sem", Value: defaultSem},
		{Name: "kernel.shmmni", Value: defaultShmmni},
	}, nil
}

// Function get the Node Affinity
// getNodeAffinity generates node affinity rules for RAC pods.
// getNodeAffinity provides documentation for the getNodeAffinity function.
func getNodeAffinity(instance *racdb.RacDatabase, oraRacSpex racdb.RacInstDetailSpec) *corev1.Affinity {

	nodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{},
		},
	}

	racTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{},
	}

	racMatch := corev1.NodeSelectorRequirement{
		Key:      utils.OraNodeKey,
		Operator: utils.OraOperatorKey,
		Values:   oraRacSpex.WorkerNode,
	}

	racTerm.MatchExpressions = append(racTerm.MatchExpressions, racMatch)
	nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		racTerm)

	affinity := &corev1.Affinity{NodeAffinity: nodeAffinity}
	return affinity

}

// getAsmNodeAffinity produces node affinity for ASM volumes based on configuration style.
func getAsmNodeAffinity(instance *racdb.RacDatabase, oldStyle bool) *corev1.VolumeNodeAffinity {
	nodeAffinity := &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{},
		},
	}

	var terms []corev1.NodeSelectorTerm

	// ----------------------------------------------------------------------
	// OLD STYLE (retain existing behavior exactly)
	// ----------------------------------------------------------------------
	if oldStyle {
		workerNodes := getAllWorkerNodes(instance) // hostname-based
		if len(workerNodes) == 0 {
			return nodeAffinity
		}

		terms = append(terms, corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      utils.OraNodeKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   workerNodes,
				},
			},
		})

		nodeAffinity.Required.NodeSelectorTerms = terms
		return nodeAffinity
	}

	// ----------------------------------------------------------------------
	// NEW STYLE (label-based WorkerNodeSelector)
	// ----------------------------------------------------------------------
	selector := instance.Spec.ClusterDetails.WorkerNodeSelector
	if len(selector) == 0 {
		return nodeAffinity // no affinity if not provided
	}

	var matchExpr []corev1.NodeSelectorRequirement
	for key, value := range selector {
		matchExpr = append(matchExpr, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{value},
		})
	}

	terms = append(terms, corev1.NodeSelectorTerm{
		MatchExpressions: matchExpr,
	})

	nodeAffinity.Required.NodeSelectorTerms = terms
	return nodeAffinity
}

// getAllWorkerNodes aggregates unique worker node names from the spec.
func getAllWorkerNodes(instance *racdb.RacDatabase) []string {
	nodeSet := map[string]struct{}{}
	for _, inst := range instance.Spec.InstDetails {
		for _, node := range inst.WorkerNode {
			nodeSet[node] = struct{}{}
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	return nodes
}

// Function to build Volume Spec
// buildVolumeSpecForRac defines required volumes for RAC pods.
// buildVolumeSpecForRac provides documentation for the buildVolumeSpecForRac function.
func buildVolumeSpecForRac(instance *racdb.RacDatabase, oraRacSpex racdb.RacInstDetailSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: oraRacSpex.Name + "-ssh-secretmap-vol",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.SshKeySecret.Name,
				},
			},
		},
		{
			Name: oraRacSpex.Name + "-oradshm-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
			},
		},
	}

	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-scripts-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}

	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.Name != "" {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-dbsecret-pwd-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.Name}}})
		}

		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-dbsecret-key-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.KeySecretName}}})
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-tdesecret-pwd-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.Name}}})
		}

		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-tdesecret-key-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.KeySecretName}}})
		}
	}

	if len(instance.Spec.ConfigParams.GridResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-girsp", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.ConfigParams.GridResponseFile.ConfigMapName}}}})
	}

	if len(instance.Spec.ConfigParams.DbResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-dbrsp", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.ConfigParams.DbResponseFile.ConfigMapName}}}})
	}

	if len(oraRacSpex.EnvFile) != 0 {
		result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-envfile", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: oraRacSpex.EnvFile}}}})
	}

	if len(oraRacSpex.HostSwLocation) != 0 {
		result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-sw-vol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: oraRacSpex.HostSwLocation}}})
	}

	if instance.Spec.ConfigParams != nil {
		if instance.Spec.ConfigParams.HostSwStageLocation != "" {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-oradata-swstage-vol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: instance.Spec.ConfigParams.HostSwStageLocation}}})
		}
	}

	if len(oraRacSpex.PvcName) != 0 {
		for source := range oraRacSpex.PvcName {
			result = append(result, corev1.Volume{Name: oraRacSpex.Name + "-ora-vol-" + source, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: source}}})
		}
	}

	if instance.Spec.ConfigParams != nil {
		if len(instance.Spec.ConfigParams.RuPatchLocation) != 0 {
			result = append(result, corev1.Volume{
				Name: oraRacSpex.Name + "-oradata-rupatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.RuPatchLocation,
					},
				},
			})
		}

		if len(instance.Spec.ConfigParams.OPatchLocation) != 0 {
			result = append(result, corev1.Volume{
				Name: oraRacSpex.Name + "-oradata-opatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.OPatchLocation,
					},
				},
			})
		}
	}
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
	result = append(result, corev1.Volume{
		Name: oraRacSpex.Name + "-oradata-envfile-writable",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	return result
}

// GetDeviceDGName provides documentation for the GetDeviceDGName function.
func GetDeviceDGName(disk string, spec *racdb.RacDatabaseSpec) string {
	for _, dg := range spec.AsmStorageDetails {
		for _, dgDisk := range dg.Disks {
			if strings.TrimSpace(dgDisk) == strings.TrimSpace(disk) {
				return string(dg.Name)
			}
		}
	}
	return ""
}

// Function to build the container Specification
// buildContainerSpecForRac provides documentation for the buildContainerSpecForRac function.
func buildContainerSpecForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	privileged := false
	failureThreshold := 1
	periodSeconds := 5
	initialDelaySeconds := 120
	oraLsnrPort := 1521

	// Get the Idx

	containerSpec := corev1.Container{
		Name:  OraRacSpex.Name,
		Image: instance.Spec.Image,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{corev1.Capability("NET_ADMIN"), corev1.Capability("SYS_NICE"), corev1.Capability("SYS_RESOURCE"), corev1.Capability("AUDIT_WRITE"), corev1.Capability("NET_RAW"), corev1.Capability("AUDIT_CONTROL"), corev1.Capability("SYS_CHROOT")},
			},
		},
		Command: []string{
			"/usr/sbin/init",
		},
		VolumeDevices: getAsmVolumeDevices(instance, OraRacSpex),
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForRac(instance, OraRacSpex),
		ReadinessProbe: &corev1.Probe{
			// TODO: Investigate if it's ok to call status every 10 seconds
			FailureThreshold:    int32(failureThreshold),
			PeriodSeconds:       int32(periodSeconds),
			InitialDelaySeconds: int32(initialDelaySeconds),
			ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(oraLsnrPort))}},
		},
	}
	if instance.Spec.Resources != nil {
		containerSpec.Resources = *instance.Spec.Resources
	}

	if len(OraRacSpex.EnvVars) > 0 {
		containerSpec.Env = buildEnvVarsSpec(OraRacSpex.EnvVars)
	}

	if instance.Spec.ReadinessProbe != nil {
		containerSpec.ReadinessProbe = instance.Spec.ReadinessProbe
	}
	// building Complete Container Spec
	containerSpec.Ports = buildContainerPortsDef(instance, OraRacSpex)

	result = []corev1.Container{
		containerSpec,
	}
	return result
}

// getAsmVolumeDevices provides documentation for the getAsmVolumeDevices function.
func getAsmVolumeDevices(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec) []corev1.VolumeDevice {
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

// Function to build the init Container Spec
// buildInitContainerSpecForRac provides documentation for the buildInitContainerSpecForRac function.
func buildInitContainerSpecForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec) []corev1.Container {
	var result []corev1.Container
	// building the init Container Spec
	privFlag := true
	var uid int64 = 0
	var scriptsCmd string
	var scriptsLocation string

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

	// Initialize the first container (init container)
	init1spec := corev1.Container{
		Name:  OraRacSpex.Name + "-init1",
		Image: instance.Spec.Image,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privFlag,
			RunAsUser:  &uid,
		},
		Command: []string{
			"/bin/bash",
			"-c",
			// Call the refactored `getRacInitContainerCmd` to handle all commands
			getRacInitContainerCmd(scriptsCmd, instance.Name, scriptsLocation, OraRacSpex.PrivateIPDetails[0].Interface, OraRacSpex.PrivateIPDetails[1].Interface),
		},
		VolumeMounts: buildVolumeMountSpecForRac(instance, OraRacSpex),
	}

	// building Complete Init Container Spec
	if instance.Spec.ImagePullPolicy != nil {
		init1spec.ImagePullPolicy = *instance.Spec.ImagePullPolicy
	}

	// Add the init container to the result
	result = append(result, init1spec)

	return result
}

// buildVolumeMountSpecForRac provides documentation for the buildVolumeMountSpecForRac function.
func buildVolumeMountSpecForRac(instance *racdb.RacDatabase, oraRacSpex racdb.RacInstDetailSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	//Defaults from webhook
	if instance.Spec.ImagePullPolicy == nil || *instance.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		instance.Spec.ImagePullPolicy = &policy
	}

	if instance.Spec.SshKeySecret != nil {
		if instance.Spec.SshKeySecret.KeyMountLocation == "" {
			instance.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSshSecretMount
		}
	}

	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		if instance.Spec.DbSecret.PwdFileMountLocation == "" {
			instance.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
		}
		if instance.Spec.DbSecret.KeyFileMountLocation == "" {
			instance.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
		}
	}

	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		if instance.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
		}
		if instance.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
		}
	}

	result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-ssh-secretmap-vol", MountPath: instance.Spec.SshKeySecret.KeyMountLocation, ReadOnly: true})
	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-dbsecret-key-vol", MountPath: instance.Spec.DbSecret.KeyFileMountLocation, ReadOnly: true})
		}
		if instance.Spec.DbSecret.Name != "" {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-dbsecret-pwd-vol", MountPath: instance.Spec.DbSecret.PwdFileMountLocation, ReadOnly: true})
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-tdesecret-pwd-vol", MountPath: instance.Spec.TdeWalletSecret.PwdFileMountLocation, ReadOnly: true})
		}

		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-tdesecret-key-vol", MountPath: instance.Spec.TdeWalletSecret.KeyFileMountLocation, ReadOnly: true})
		}
	}
	//result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-boot-vol", MountPath: oraBootVol})
	result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradshm-vol", MountPath: utils.OraShm})

	if len(instance.Spec.ConfigParams.GridResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-girsp", MountPath: utils.OraGiRsp})
	}

	if len(instance.Spec.ConfigParams.DbResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-dbrsp", MountPath: utils.OraDbRsp})
	}

	if len(oraRacSpex.EnvFile) != 0 {
		result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-envfile", MountPath: utils.OraEnvFile})
	}

	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-scripts-vol", MountPath: instance.Spec.ScriptsLocation})
	}

	// Add the **shared emptyDir mount** for /etc/rac_env_vars
	result = append(result, corev1.VolumeMount{
		Name:      oraRacSpex.Name + "-oradata-envfile-writable",
		MountPath: utils.OraWritableEnvFile,
	})

	if len(oraRacSpex.HostSwLocation) != 0 {
		swMountPath := instance.Spec.ConfigParams.SwMountLocation
		if swMountPath == "" {
			swMountPath = utils.OraSwLocation
		}
		result = append(result, corev1.VolumeMount{
			Name:      oraRacSpex.Name + "-oradata-sw-vol",
			MountPath: swMountPath,
		})

	} else if len(instance.Spec.StorageClass) != 0 {
		result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-sw-vol", MountPath: instance.Spec.ConfigParams.SwMountLocation})
	} else {
		fmt.Println("No Location is passed for the software storage in" + oraRacSpex.Name)
	}

	if instance.Spec.ConfigParams != nil {
		if len(instance.Spec.ConfigParams.HostSwStageLocation) != 0 {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-swstage-vol", MountPath: utils.OraSwStageLocation})
		}
	}

	if instance.Spec.ConfigParams != nil {
		if len(instance.Spec.ConfigParams.OPatchLocation) != 0 {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-opatch-vol", MountPath: utils.OraOPatchStageLocation})
		}
	}
	if instance.Spec.ConfigParams != nil {
		if len(instance.Spec.ConfigParams.RuPatchLocation) != 0 {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-oradata-rupatch-vol", MountPath: utils.OraRuPatchStageLocation})
		}
	}

	if len(oraRacSpex.PvcName) != 0 {
		for source, target := range oraRacSpex.PvcName {
			result = append(result, corev1.VolumeMount{Name: oraRacSpex.Name + "-ora-vol-" + source, MountPath: target})
		}
	}

	return result
}

// VolumePVForASM provides documentation for the VolumePVForASM function.
func VolumePVForASM(
	instance *racdb.RacDatabase,
	dgIndex, diskIdx int,
	diskName, diskGroupName, size string, // size from disk check
	oldStyle bool,
) *corev1.PersistentVolume {
	volumeBlock := corev1.PersistentVolumeBlock

	pvName := GetAsmPvName(diskName, instance.Name)

	labels := buildLabelsForAsmPv(instance, diskName)

	asmPv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			VolumeMode: &volumeBlock,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}

	// Local disk
	if len(instance.Spec.StorageClass) == 0 {
		asmPv.Spec.NodeAffinity = getAsmNodeAffinity(instance, oldStyle)
		asmPv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			Local: &corev1.LocalVolumeSource{Path: diskName},
		}
	} else {
		asmPv.Spec.StorageClassName = instance.Spec.StorageClass
	}

	return asmPv
}

// VolumePVCForASM provides documentation for the VolumePVCForASM function.
func VolumePVCForASM(
	instance *racdb.RacDatabase,
	dgIndex, diskIdx int,
	diskName, diskGroupName, size string,
) *corev1.PersistentVolumeClaim {
	volumeBlock := corev1.PersistentVolumeBlock

	asmPvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAsmPvcName(diskName, instance.Name),
			Namespace: instance.Namespace,
			Labels:    buildLabelsForAsmPv(instance, diskName),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			VolumeMode: &volumeBlock,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	// Use selector for auto-binding to PV
	if len(instance.Spec.StorageClass) == 0 {
		asmPvc.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: buildLabelsForAsmPv(instance, diskName),
		}
	} else {
		asmPvc.Spec.StorageClassName = &instance.Spec.StorageClass
	}

	return asmPvc
}

// generateNetworkDetails provides documentation for the generateNetworkDetails function.
func generateNetworkDetails(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client) map[string]string {
	var annstr map[string]string = make(map[string]string)
	var networks []racdb.RacNetworkDetailSpec

	// Loop through each PrivateIPDetail and generate network configurations
	for i := 0; i < len(OraRacSpex.PrivateIPDetails); i++ {
		networks = append(networks, GetPrivNetDetails(instance, OraRacSpex, kClient, OraRacSpex.PrivateIPDetails[i]))
	}

	// Marshal the networks into JSON and prepare the annotation
	networksJSON, _ := json.Marshal(networks)
	annstr["k8s.v1.cni.cncf.io/networks"] = string(networksJSON)

	return annstr
}

// VolumeClaimTemplatesForRac provides documentation for the VolumeClaimTemplatesForRac function.
func VolumeClaimTemplatesForRac(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec) []corev1.PersistentVolumeClaim {
	var claims []corev1.PersistentVolumeClaim

	// If PvcName is provided, return the empty claims early
	if len(OraRacSpex.PvcName) != 0 {
		return claims
	}

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
	/*
	   if len(OraRacSpex.PvMatchLabels) > 0 && len(claims) > 0 {
	       claims[0].Spec.Selector = &metav1.LabelSelector{MatchLabels: OraRacSpex.PvMatchLabels}
	   }
	*/

	return claims
}

// BuildServiceDefForRac provides documentation for the BuildServiceDefForRac function.
func BuildServiceDefForRac(instance *racdb.RacDatabase, replicaCount int32, OraRacSpex racdb.RacInstDetailSpec, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		TypeMeta:   metav1.TypeMeta{Kind: "Service"},
		ObjectMeta: buildSvcObjectMetaForRac(instance, replicaCount, OraRacSpex, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// Check if user want External Svc on each replica pod
	if strings.ToUpper(svctype) == "VIP" || strings.ToUpper(svctype) == "LOCAL" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = getSvcLabelsForRac(replicaCount, OraRacSpex)

	}

	if strings.ToUpper(svctype) == "SCAN" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = buildLabelsForRac(instance, "RAC")
	}

	service.Spec.PublishNotReadyAddresses = true

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.

	return service
}

// BuildExternalServiceDefForRac provides documentation for the BuildExternalServiceDefForRac function.
func BuildExternalServiceDefForRac(instance *racdb.RacDatabase, index int32, OraRacSpex racdb.RacInstDetailSpec, svctype string, opType string) *corev1.Service {
	//service := &corev1.Service{}

	var npSvc racdb.RacNodePortSvc

	if OraRacSpex.PortMappings != nil {
		npSvc = OraRacSpex.NodePortSvc[index]
	}

	var exSvc racdb.RacNodePortSvc
	var exSvcPorts racdb.RacPortMapping

	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForRac(instance, index, OraRacSpex, opType),
		Spec:       corev1.ServiceSpec{},
	}

	if opType == "scansvc" {

		exSvc.SvcType = svctype
		exSvcPorts.NodePort = *instance.Spec.ScanSvcTargetPort

		if instance.Spec.ScanSvcLocalPort != nil {
			exSvcPorts.Port = *instance.Spec.ScanSvcLocalPort
		} else {
			exSvcPorts.Port = 1521
		}

		exSvc.PortMappings = append(exSvc.PortMappings, exSvcPorts)
		service.Spec.Selector = buildLabelsForRac(instance, "RAC")
		npSvc = exSvc
	}

	if opType == "onssvc" {
		//exSvc.SvcName = OraRacSpex.Name + "-0-ons"
		exSvc.SvcType = svctype
		exSvcPorts.NodePort = *OraRacSpex.OnsTargetPort
		if OraRacSpex.OnsLocalPort != nil {
			exSvcPorts.Port = *OraRacSpex.OnsLocalPort
		} else {
			exSvcPorts.Port = 6200
		}

		exSvc.PortMappings = append(exSvc.PortMappings, exSvcPorts)
		service.Spec.Selector = getSvcLabelsForRac(0, OraRacSpex)
		npSvc = exSvc
	}

	if opType == "lsnrsvc" {
		//exSvc.SvcName = OraRacSpex.Name + "-0-lsnr"
		exSvc.SvcType = svctype
		exSvcPorts.NodePort = *OraRacSpex.LsnrTargetPort
		//exSvcPorts.Port = index + 1522
		exSvcPorts.Port = *OraRacSpex.LsnrTargetPort

		exSvc.PortMappings = append(exSvc.PortMappings, exSvcPorts)
		service.Spec.Selector = getSvcLabelsForRac(0, OraRacSpex)
		npSvc = exSvc

	}

	if opType == "nodeport" {
		service.Spec.ClusterIP = string(corev1.ServiceTypeNodePort)
		if strings.EqualFold(npSvc.SvcType, "vip") {
			service.Spec.Selector = getSvcLabelsForRac(0, OraRacSpex)
		} else if strings.EqualFold(npSvc.SvcType, "scan") {
			service.Spec.Selector = buildLabelsForRac(instance, "RAC")
		} else {
			service.Spec.Selector = getSvcLabelsForRac(0, OraRacSpex)
		}
	}

	service.Spec.Ports = buildRacSvcPortsDef(npSvc)

	if svctype == "nodeport" {
		service.Spec.Type = corev1.ServiceTypeNodePort
	}

	if svctype == "lbservice" {
		service.Spec.Type = corev1.ServiceTypeClusterIP
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.

	return service
}

// Function to build Service ObjectMeta
// buildSvcObjectMetaForRac provides documentation for the buildSvcObjectMetaForRac function.
func buildSvcObjectMetaForRac(instance *racdb.RacDatabase, replicaCount int32, OraRacSpex racdb.RacInstDetailSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	//var svcName string

	var labelStr map[string]string
	if strings.ToUpper(svctype) == "VIP" || strings.ToUpper(svctype) == "LOCAL" || strings.ToUpper(svctype) == "ONSSVC" || strings.ToUpper(svctype) == "LSNRSVC" {
		labelStr = getSvcLabelsForRac(replicaCount, OraRacSpex)
	} else if strings.ToUpper(svctype) == "SCAN" {
		labelStr = nil
	} else {
		labelStr = nil
	}

	objmeta := metav1.ObjectMeta{
		Name:      getRacSvcName(instance, OraRacSpex, svctype),
		Namespace: instance.Namespace,
	}

	if labelStr != nil {
		objmeta.Labels = labelStr
	}

	return objmeta
}

// getRacSvcName provides documentation for the getRacSvcName function.
func getRacSvcName(instance *racdb.RacDatabase, oraRacSpex racdb.RacInstDetailSpec, svcType string) string {

	switch svcType {
	case "local":
		return oraRacSpex.Name + "-0"
	case "vip":
		return oraRacSpex.VipSvcName
	case "scan":
		return instance.Spec.ScanSvcName
	case "onssvc":
		return oraRacSpex.Name + "-0-ons"
	case "lsnrsvc":
		return oraRacSpex.Name + "-0-lsnr"
	case "scansvc":
		return instance.Spec.ScanSvcName + "-lsnr"
	default:
		return oraRacSpex.Name

	}
}

// getSvcLabelsForRac provides documentation for the getSvcLabelsForRac function.
func getSvcLabelsForRac(replicaCount int32, OraRacSpex racdb.RacInstDetailSpec) map[string]string {

	var labelStr map[string]string = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraRacSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraRacSpex.Name + "-0"
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// This function cleanup the shard from GSM
// OraCleanupForRac provides documentation for the OraCleanupForRac function.
func OraCleanupForRac(instance *racdb.RacDatabase,
	OraRacSpex racdb.RacInstDetailSpec,
	oldReplicaSize int32,
	newReplicaSize int32,
) string {
	var err1 string
	if oldReplicaSize > newReplicaSize {
		for replicaCount := (oldReplicaSize - 1); replicaCount > (newReplicaSize - 1); replicaCount-- {
			fmt.Println("Deleting the RAC " + OraRacSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10))
		}
	}

	err1 = "Test"
	return err1
}

// UpdateProvForRac provides documentation for the UpdateProvForRac function.
func UpdateProvForRac(instance *racdb.RacDatabase,
	OraRacSpex racdb.RacInstDetailSpec, kClient client.Client, sfSet *appsv1.StatefulSet, gsmPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {

	var msg string
	var size int32 = 1
	var isUpdate bool = false
	// var err error
	//var i int

	msg = "Inside the updateProvForRac"
	LogMessages("DEBUG", msg, nil, instance, logger)

	// Ensure deployment replicas match the desired state

	if sfSet.Spec.Replicas != nil {
		if *sfSet.Spec.Replicas != size {
			msg = "Current StatefulSet replicas do not match configured Shard Replicas. Gsm is configured with only 1 but current replicas is set with " + strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
			LogMessages("DEBUG", msg, nil, instance, logger)
			isUpdate = true
		}
	}
	// Memory Check
	//resources := corev1.Pod.Spec.Containers

	/**

	 for i = 0; i < len(sfSet.Spec.VolumeClaimTemplates); i++ {
		 if sfSet.Spec.VolumeClaimTemplates[i].Name == OraRacSpex.Name+"-oradata-vol4" {
			 volumeSize := sfSet.Spec.VolumeClaimTemplates[i].Size()
			 if volumeSize != int(OraRacSpex.StorageSizeInGb) {
				 isUpdate = true
			 }

		 }
	 }

	 **/

	if isUpdate {
		sfSet, err := BuildStatefulSetForRac(instance, OraRacSpex, kClient)
		if err != nil {
			msg := fmt.Sprintf("Failed to build StatefulSet for OracleRestart %s: %v", OraRacSpex.Name, err)
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

		err = kClient.Update(context.Background(), sfSet)
		if err != nil {
			msg = "Failed to update Shard StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

// ConfigMapSpecs provides documentation for the ConfigMapSpecs function.
func ConfigMapSpecs(instance *racdb.RacDatabase, cmData map[string]string, cmName string) *corev1.ConfigMap {
	//cm := &corev1.ConfigMap{}

	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"name": instance.Name + cmName,
			},
		},
		Data: cmData,
	}

}

// BuildDiskCheckDaemonSet provides documentation for the BuildDiskCheckDaemonSet function.
func BuildDiskCheckDaemonSet(racDatabase *racdb.RacDatabase, oldStyle bool) *appsv1.DaemonSet {
	labels := BuildLabelsForDaemonSet(racDatabase, "disk-check")

	var nodeAffinity *corev1.NodeAffinity

	// ----------------------------------------------------------------------
	// OLD STYLE — hostname-based node affinity (unchanged)
	// ----------------------------------------------------------------------
	if oldStyle {

		workerNodes := getAllWorkerNodes(racDatabase)
		if len(workerNodes) > 0 {
			nodeAffinity = &corev1.NodeAffinity{
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

	} else {

		// ------------------------------------------------------------------
		// NEW STYLE — label-based node affinity using WorkerNodeSelector
		// ------------------------------------------------------------------

		if racDatabase.Spec.ClusterDetails != nil &&
			len(racDatabase.Spec.ClusterDetails.WorkerNodeSelector) > 0 {

			selector := racDatabase.Spec.ClusterDetails.WorkerNodeSelector
			var matchExpr []corev1.NodeSelectorRequirement

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

			if len(matchExpr) > 0 {
				nodeAffinity = &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: matchExpr,
							},
						},
					},
				}
			}
		}
	}

	// ----------------------------------------------------------------------
	// Disk-check script & mounts (unchanged)
	// ----------------------------------------------------------------------
	disks := flattenAsmDisks(&racDatabase.Spec)
	diskArray := strings.Join(disks, " ")

	cmd := fmt.Sprintf(`
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

	var volumeMounts []corev1.VolumeMount
	var volumes []corev1.Volume
	for _, disk := range disks {
		volName := sanitizeK8sName(disk) + "-vol"
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

	privileged := true

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk-check-daemonset",
			Namespace: racDatabase.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: nodeAffinity, // ← correctly built
					},
					Volumes: volumes,
					Containers: []corev1.Container{
						{
							Name:    "disk-check",
							Image:   racDatabase.Spec.Image,
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
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
}

// Helper function to flatten all disk names in AsmStorageDetails, removing duplicates
// flattenAsmDisks provides documentation for the flattenAsmDisks function.
func flattenAsmDisks(racDbSpec *racdb.RacDatabaseSpec) []string {
	seen := make(map[string]struct{})
	var allDisks []string
	for _, dg := range racDbSpec.AsmStorageDetails {
		for _, disk := range dg.Disks {
			if _, ok := seen[disk]; !ok {
				allDisks = append(allDisks, disk)
				seen[disk] = struct{}{}
			}
		}
	}
	return allDisks
}
