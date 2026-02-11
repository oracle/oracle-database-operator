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
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	oraclerestart "github.com/oracle/oracle-database-operator/apis/database/v4"
	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	utils "github.com/oracle/oracle-database-operator/commons/oraclerestart/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants
const (
	minContainerMemory = 16 * 1024 * 1024 * 1024 // 16 GB
	pageSize           = 4096                    // 4 KiB
	oneGB              = int64(1024 * 1024 * 1024)
	defaultSem         = "250 32000 100 128"
	defaultShmmni      = "4096"
)

// Constants for rac-stateful StatefulSet & Volumes
// buildLabelsForOracleRestart provides documentation for the buildLabelsForOracleRestart function.
func buildLabelsForOracleRestart(instance *oraclerestartdb.OracleRestart, label string) map[string]string {
	return map[string]string{
		"cluster": "oraclerestart",
	}

	// "oralabel": getLabelForOracleRestart(instance),
}

// buildLabelsForAsmPv provides documentation for the buildLabelsForAsmPv function.
func buildLabelsForAsmPv(instance *oraclerestartdb.OracleRestart, diskName string) map[string]string {
	return map[string]string{
		"asm_vol": "block-asm-pv-" + getLabelForOracleRestart(instance) + "-" + diskName[strings.LastIndex(diskName, "/")+1:],
	}
}

// getLabelForOracleRestart provides documentation for the getLabelForOracleRestart function.
func getLabelForOracleRestart(instance *oraclerestartdb.OracleRestart) string {

	return instance.Name
}

// BuildStatefulSetForOracleRestart provides documentation for the BuildStatefulSetForOracleRestart function.
func BuildStatefulSetForOracleRestart(
	instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec,
	kClient client.Client,
) (*appsv1.StatefulSet, error) {

	sfsetSpec, err := buildStatefulSpecForOracleRestart(instance, OracleRestartSpex, kClient)
	if err != nil {
		return nil, fmt.Errorf("failed to build StatefulSetSpec for OracleRestart %s: %v", OracleRestartSpex.Name, err)
	}

	// Build full StatefulSet
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForOracleRestart(),
		ObjectMeta: builObjectMetaForOracleRestart(instance, OracleRestartSpex),
		Spec:       *sfsetSpec, // dereference after successful build
	}

	return sfset, nil

}

// Function to build TypeMeta
// buildTypeMetaForOracleRestart provides documentation for the buildTypeMetaForOracleRestart function.
func buildTypeMetaForOracleRestart() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
// builObjectMetaForOracleRestart provides documentation for the builObjectMetaForOracleRestart function.
func builObjectMetaForOracleRestart(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:      OracleRestartSpex.Name,
		Namespace: instance.Namespace,
		Labels:    buildLabelsForOracleRestart(instance, "OracleRestart"),
	}
	return objmeta
}

// Function to build Stateful Specs
// buildStatefulSpecForOracleRestart provides documentation for the buildStatefulSpecForOracleRestart function.
func buildStatefulSpecForOracleRestart(
	instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec,
	kClient client.Client,
) (*appsv1.StatefulSetSpec, error) {

	// Build PodSpec first, capture any errors
	podSpec, err := buildPodSpecForOracleRestart(instance, OracleRestartSpex)
	if err != nil {
		return nil, fmt.Errorf("failed to build PodSpec for OracleRestart %s: %v", OracleRestartSpex.Name, err)
	}

	// Add service account name if specified
	if instance.Spec.SrvAccountName != "" {
		podSpec.ServiceAccountName = instance.Spec.SrvAccountName
	}

	// Build StatefulSetSpec
	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: utils.OraSubDomain,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForOracleRestart(instance, "OracleRestart"),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:   OracleRestartSpex.Name,
				Labels: buildLabelsForOracleRestart(instance, "OracleRestart"),
			},
			Spec: *podSpec, // dereference after modification
		},
	}
	// // Add volume claim templates if a storage class is specified
	if len(instance.Spec.DataDgStorageClass) != 0 && !asmPvcsExist(instance, kClient) {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, ASMVolumeClaimTemplatesForDG(instance, OracleRestartSpex, &instance.Spec.DataDgStorageClass)...)
	}

	if len(instance.Spec.CrsDgStorageClass) != 0 && !asmPvcsExist(instance, kClient) {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, ASMVolumeClaimTemplatesForDG(instance, OracleRestartSpex, &instance.Spec.CrsDgStorageClass)...)
	}

	if len(instance.Spec.RecoDgStorageClass) != 0 && !asmPvcsExist(instance, kClient) {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, ASMVolumeClaimTemplatesForDG(instance, OracleRestartSpex, &instance.Spec.RecoDgStorageClass)...)
	}

	if len(instance.Spec.RedoDgStorageClass) != 0 && !asmPvcsExist(instance, kClient) {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, ASMVolumeClaimTemplatesForDG(instance, OracleRestartSpex, &instance.Spec.RedoDgStorageClass)...)
	}

	if len(instance.Spec.SwStorageClass) != 0 && len(instance.Spec.InstDetails.HostSwLocation) == 0 {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, SwVolumeClaimTemplatesForOracleRestart(instance, OracleRestartSpex))
	}
	return sfsetspec, nil
}

// asmPvcsExist provides documentation for the asmPvcsExist function.
func asmPvcsExist(instance *oraclerestart.OracleRestart, kClient client.Client) bool {
	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, diskName := range dg.Disks {
			pvcName := GetAsmPvcName(diskName, instance.Name)
			var pvc corev1.PersistentVolumeClaim
			err := kClient.Get(context.TODO(), types.NamespacedName{
				Name:      pvcName,
				Namespace: instance.Namespace,
			}, &pvc)

			if err != nil {
				if apierrors.IsNotFound(err) {
					// If even one expected PVC is not found, treat as "not all exist"
					return false
				}
				// If error is something else, assume PVCs exist to avoid accidental overwrite
				return true
			}
		}
	}
	return true
}

// Function to build PodSpec

// buildPodSpecForOracleRestart provides documentation for the buildPodSpecForOracleRestart function.
func buildPodSpecForOracleRestart(
	instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec,
) (*corev1.PodSpec, error) {

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

	// Build pod spec
	spec := &corev1.PodSpec{
		Hostname:        OracleRestartSpex.Name + "-0",
		Subdomain:       utils.OraSubDomain,
		InitContainers:  buildInitContainerSpecForOracleRestart(instance, OracleRestartSpex),
		Containers:      buildContainerSpecForOracleRestart(instance, OracleRestartSpex),
		Volumes:         buildVolumeSpecForOracleRestart(instance, OracleRestartSpex),
		Affinity:        getNodeAffinity(instance, OracleRestartSpex),
		SecurityContext: instance.Spec.SecurityContext,
	}
	// ImagePullSecret
	if len(instance.Spec.ImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: instance.Spec.ImagePullSecret},
		}
	}

	return spec, nil
}

// parseSGASizeBytes parses memory config value ("16G", "16Gi", "1024M", "512Mi") and returns int64 bytes
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

// calculateSysctls provides documentation for the calculateSysctls function.
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
// getNodeAffinity provides documentation for the getNodeAffinity function.
func getNodeAffinity(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) *corev1.Affinity {

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
		Values:   OracleRestartSpex.WorkerNode,
	}

	racTerm.MatchExpressions = append(racTerm.MatchExpressions, racMatch)
	nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		racTerm)

	affinity := &corev1.Affinity{NodeAffinity: nodeAffinity}
	return affinity

}

// Function get the Node Affinity
// getAsmNodeAffinity provides documentation for the getAsmNodeAffinity function.
func getAsmNodeAffinity(instance *oraclerestartdb.OracleRestart) *corev1.VolumeNodeAffinity {

	nodeAffinity := &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{},
		},
	}

	racTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{},
	}

	racMatch := corev1.NodeSelectorRequirement{
		Key:      utils.OraNodeKey,
		Operator: utils.OraOperatorKey,
		Values:   instance.Spec.InstDetails.WorkerNode,
	}

	racTerm.MatchExpressions = append(racTerm.MatchExpressions, racMatch)
	nodeAffinity.Required.NodeSelectorTerms = append(nodeAffinity.Required.NodeSelectorTerms,
		racTerm)

	return nodeAffinity

}

// Function to build Volume Spec
// buildVolumeSpecForOracleRestart provides documentation for the buildVolumeSpecForOracleRestart function.
func buildVolumeSpecForOracleRestart(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: OracleRestartSpex.Name + "-oradshm-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
			},
		},
	}

	// Only add the SSH secret volume if SshKeySecret is not nil and Name is not empty/whitespace
	if instance.Spec.SshKeySecret != nil && strings.TrimSpace(instance.Spec.SshKeySecret.Name) != "" {
		result = append([]corev1.Volume{
			{
				Name: OracleRestartSpex.Name + "-ssh-secretmap-vol",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: instance.Spec.SshKeySecret.Name,
					},
				},
			},
		}, result...)
	}

	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-scripts-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}

	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.Name != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-dbsecret-pwd-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.Name}}})
		}

		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-dbsecret-key-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.DbSecret.KeySecretName}}})
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-tdesecret-pwd-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.Name}}})
		}

		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-tdesecret-key-vol", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.TdeWalletSecret.KeySecretName}}})
		}
	}

	if len(instance.Spec.ConfigParams.GridResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-girsp", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.ConfigParams.GridResponseFile.ConfigMapName}}}})
	}

	if len(instance.Spec.ConfigParams.DbResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-dbrsp", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.ConfigParams.DbResponseFile.ConfigMapName}}}})
	}

	if len(OracleRestartSpex.EnvFile) != 0 {
		result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-envfile", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: OracleRestartSpex.EnvFile}}}})
	}

	if len(OracleRestartSpex.HostSwLocation) != 0 {
		result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-sw-vol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: OracleRestartSpex.HostSwLocation}}})
	} else {
		if instance.Spec.SwStorageClass != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-sw-vol", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: GetSwPvcName(instance.Name, instance)}}})
		}
	}

	// Following block checks for HostSwStageLocation, RUPatchLocation nd OpatchLocation
	if instance.Spec.ConfigParams != nil && len(instance.Spec.ConfigParams.SwStagePvc) != 0 {
		// FIrst Check
		result = append(result, corev1.Volume{
			Name:         OracleRestartSpex.Name + "-oradata-swstagepvc-vol",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.ConfigParams.SwStagePvc}},
		})
	} else {
		if len(instance.Spec.ConfigParams.HostSwStageLocation) != 0 {
			result = append(result, corev1.Volume{
				Name: OracleRestartSpex.Name + "-oradata-swstage-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.HostSwStageLocation,
					},
				},
			})
		}
		if instance.Spec.ConfigParams.RuPatchLocation != "" {
			result = append(result, corev1.Volume{
				Name: OracleRestartSpex.Name + "-oradata-rupatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.RuPatchLocation,
					},
				},
			})
		}
		if instance.Spec.ConfigParams.OPatchLocation != "" {
			result = append(result, corev1.Volume{
				Name: OracleRestartSpex.Name + "-oradata-opatch-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.OPatchLocation,
					},
				},
			})
		}
		if instance.Spec.ConfigParams.OneOffLocation != "" {
			result = append(result, corev1.Volume{
				Name: OracleRestartSpex.Name + "-oradata-oneoff-vol",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: instance.Spec.ConfigParams.OneOffLocation,
					},
				},
			})
		}
	}

	if checkHugePagesConfigured(instance) {
		result = append(result, corev1.Volume{
			Name: OracleRestartSpex.Name + "-oradata-hugepages-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumHugePages,
				},
			},
		})
	}

	if len(OracleRestartSpex.PvcName) != 0 {
		for source := range OracleRestartSpex.PvcName {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-ora-vol-" + source, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: source}}})
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

	return result
}

// Function to build the container Specification
// buildContainerSpecForOracleRestart provides documentation for the buildContainerSpecForOracleRestart function.
func buildContainerSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	privileged := false
	failureThreshold := 1
	periodSeconds := 5
	initialDelaySeconds := 120
	oraLsnrPort := 1521

	containerSpec := corev1.Container{
		Name:  OracleRestartSpex.Name,
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
		VolumeDevices: getAsmVolumeDevices(instance, OracleRestartSpex),
		VolumeMounts:  buildVolumeMountSpecForOracleRestart(instance, OracleRestartSpex),
		ReadinessProbe: &corev1.Probe{
			FailureThreshold:    int32(failureThreshold),
			PeriodSeconds:       int32(periodSeconds),
			InitialDelaySeconds: int32(initialDelaySeconds),
			ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(oraLsnrPort))}},
		},
	}
	if instance.Spec.Resources != nil {
		containerSpec.Resources = *instance.Spec.Resources
	}

	if len(OracleRestartSpex.EnvVars) > 0 {
		containerSpec.Env = buildEnvVarsSpec(OracleRestartSpex.EnvVars)
	}

	if instance.Spec.ReadinessProbe != nil {
		containerSpec.ReadinessProbe = instance.Spec.ReadinessProbe
	}
	// building Complete Container Spec
	containerSpec.Ports = buildContainerPortsDef(instance)

	result = []corev1.Container{
		containerSpec,
	}
	return result
}

// resourcesForSGA provides documentation for the resourcesForSGA function.
func resourcesForSGA(sgaSizeStr string) (corev1.ResourceRequirements, error) {
	qty, err := resource.ParseQuantity(sgaSizeStr)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	sgaSizeBytes := qty.Value()

	hugePageSizeBytes := int64(2 * 1024 * 1024) // 2MiB

	numHugePages := (sgaSizeBytes + hugePageSizeBytes - 1) / hugePageSizeBytes
	totalHugePagesMemoryBytes := numHugePages * hugePageSizeBytes

	memQuantity := *resource.NewQuantity(sgaSizeBytes, resource.BinarySI)
	hugePagesQuantity := *resource.NewQuantity(totalHugePagesMemoryBytes, resource.BinarySI)

	return corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory:                memQuantity,
			corev1.ResourceName("hugepages-2Mi"): hugePagesQuantity,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory:                memQuantity,
			corev1.ResourceName("hugepages-2Mi"): hugePagesQuantity,
		},
	}, nil
}

// getAsmVolumeDevices provides documentation for the getAsmVolumeDevices function.
func getAsmVolumeDevices(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) []corev1.VolumeDevice {
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
// buildInitContainerSpecForOracleRestart provides documentation for the buildInitContainerSpecForOracleRestart function.
func buildInitContainerSpecForOracleRestart(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) []corev1.Container {
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

	init1spec := corev1.Container{
		Name:  OracleRestartSpex.Name + "-init1",
		Image: instance.Spec.Image,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privFlag,
			RunAsUser:  &uid,
		},
		Command: []string{
			"/bin/bash",
			"-c",
			getRacInitContainerCmd(scriptsCmd, instance.Name, scriptsLocation),
		},
		VolumeMounts: buildVolumeMountSpecForOracleRestart(instance, OracleRestartSpex),
	}

	// building Complete Init Container Spec
	if instance.Spec.ImagePullPolicy != nil {
		init1spec.ImagePullPolicy = *instance.Spec.ImagePullPolicy
	}
	result = []corev1.Container{
		init1spec,
	}
	return result
}

// buildVolumeMountSpecForOracleRestart provides documentation for the buildVolumeMountSpecForOracleRestart function.
func buildVolumeMountSpecForOracleRestart(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	if instance.Spec.SshKeySecret != nil && strings.TrimSpace(instance.Spec.SshKeySecret.KeyMountLocation) != "" {
		result = append(result, corev1.VolumeMount{
			Name:      OracleRestartSpex.Name + "-ssh-secretmap-vol",
			MountPath: instance.Spec.SshKeySecret.KeyMountLocation,
			ReadOnly:  true,
		})
	}
	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-dbsecret-key-vol", MountPath: instance.Spec.DbSecret.KeyFileMountLocation, ReadOnly: true})
		}
		if instance.Spec.DbSecret.Name != "" {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-dbsecret-pwd-vol", MountPath: instance.Spec.DbSecret.PwdFileMountLocation, ReadOnly: true})
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-tdesecret-pwd-vol", MountPath: instance.Spec.TdeWalletSecret.PwdFileMountLocation, ReadOnly: true})
		}

		if instance.Spec.TdeWalletSecret.KeySecretName != "" {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-tdesecret-key-vol", MountPath: instance.Spec.TdeWalletSecret.KeyFileMountLocation, ReadOnly: true})
		}
	}
	//result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-boot-vol", MountPath: oraBootVol})
	result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradshm-vol", MountPath: utils.OraShm})

	if len(instance.Spec.ConfigParams.GridResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-girsp", MountPath: utils.OraGiRsp})
	}

	if len(instance.Spec.ConfigParams.DbResponseFile.ConfigMapName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-dbrsp", MountPath: utils.OraDbRsp})
	}

	if len(OracleRestartSpex.EnvFile) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-envfile", MountPath: utils.OraEnvFile})
	}

	if len(instance.Spec.ScriptsLocation) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-scripts-vol", MountPath: instance.Spec.ScriptsLocation})
	}
	if len(OracleRestartSpex.HostSwLocation) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-sw-vol", MountPath: instance.Spec.ConfigParams.SwMountLocation})
	}

	//var mountLoc string

	// Check if ConfigParams is not nil
	if instance.Spec.ConfigParams != nil {
		// Check if HostSwStageLocation is provided in ConfigParams
		if len(instance.Spec.ConfigParams.SwStagePvc) != 0 {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-swstagepvc-vol", MountPath: instance.Spec.ConfigParams.SwStagePvcMountLocation})
		} else {
			if instance.Spec.ConfigParams.HostSwStageLocation != "" {
				result = append(result, corev1.VolumeMount{
					Name:      OracleRestartSpex.Name + "-oradata-swstage-vol",
					MountPath: instance.Spec.ConfigParams.HostSwStageLocation,
				})
			}

			if instance.Spec.ConfigParams.RuPatchLocation != "" {
				result = append(result, corev1.VolumeMount{
					Name:      OracleRestartSpex.Name + "-oradata-rupatch-vol",
					MountPath: instance.Spec.ConfigParams.RuPatchLocation,
				})
			}

			if instance.Spec.ConfigParams.OPatchLocation != "" {
				result = append(result, corev1.VolumeMount{
					Name:      OracleRestartSpex.Name + "-oradata-opatch-vol",
					MountPath: instance.Spec.ConfigParams.OPatchLocation,
				})
			}
			if instance.Spec.ConfigParams.OneOffLocation != "" {
				result = append(result, corev1.VolumeMount{
					Name:      OracleRestartSpex.Name + "-oradata-oneoff-vol",
					MountPath: instance.Spec.ConfigParams.OneOffLocation,
				})
			}
		}
	}

	if checkHugePagesConfigured(instance) {
		result = append(result, corev1.VolumeMount{
			Name:      OracleRestartSpex.Name + "-oradata-hugepages-vol",
			MountPath: "/hugepages",
		})
	}

	if len(OracleRestartSpex.PvcName) != 0 {
		for source, target := range OracleRestartSpex.PvcName {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-ora-vol-" + source, MountPath: target})
		}
	}

	return result
}

// VolumePVCForASM provides documentation for the VolumePVCForASM function.
func VolumePVCForASM(instance *oraclerestartdb.OracleRestart, dgIndex, diskIdx int,
	diskName, diskGroupName, size, dgType string, k8sClient client.Client,
) *corev1.PersistentVolumeClaim {
	// Set volume mode to block
	volumeBlock := corev1.PersistentVolumeBlock

	// Create PersistentVolumeClaim
	asmPvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAsmPvcName(diskName, instance.Name),
			Namespace: instance.Namespace,
			Labels:    buildLabelsForOracleRestart(instance, "OracleRestart"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			VolumeMode: &volumeBlock,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size)},
			},
		},
	}
	var scName *string
	switch dgType {
	case "RECO":
		if len(instance.Spec.RecoDgStorageClass) != 0 {
			scName = &instance.Spec.RecoDgStorageClass
		}
	case "REDO":
		if len(instance.Spec.RedoDgStorageClass) != 0 {
			scName = &instance.Spec.RedoDgStorageClass
		}
	case "CRS":
		if len(instance.Spec.CrsDgStorageClass) != 0 {
			scName = &instance.Spec.CrsDgStorageClass
		}
	case "DATA":
		if len(instance.Spec.DataDgStorageClass) != 0 {
			scName = &instance.Spec.DataDgStorageClass
		}
	}

	if scName == nil {
		// Try to fetch the cluster's default StorageClass
		if defaultSC, err := GetDefaultStorageClass(context.TODO(), k8sClient); err == nil && defaultSC != "" {
			scName = &defaultSC
		} else {
			// No StorageClass, so use label selector and statically bound PVs
			asmPvc.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: buildLabelsForAsmPv(instance, string(diskName)),
			}
			scName = nil
		}
	}
	asmPvc.Spec.StorageClassName = scName

	return asmPvc
}

// GetDefaultStorageClass provides documentation for the GetDefaultStorageClass function.
func GetDefaultStorageClass(ctx context.Context, k8sClient client.Client) (string, error) {
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return "", err
	}
	for _, sc := range scList.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return sc.Name, nil
		}
	}
	return "", nil // No default StorageClass found
}

// VolumePVForASM provides documentation for the VolumePVForASM function.
func VolumePVForASM(instance *oraclerestartdb.OracleRestart,
	dgIndex, diskIdx int,
	diskName, diskGroupName, size string,
	k8sClient client.Client,
) *corev1.PersistentVolume {
	volumeBlock := corev1.PersistentVolumeBlock

	asmPvc := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAsmPvName(diskName, instance.Name),
			Namespace: instance.Namespace,
			Labels:    buildLabelsForAsmPv(instance, diskName),
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			VolumeMode: &volumeBlock,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}

	var scName *string
	/*
		if len(instance.Spec.StorageClass) != 0 {
			scName = &instance.Spec.StorageClass
			asmPvc.Spec.NodeAffinity = getAsmNodeAffinity(instance, index, asmStorage)
			asmPvc.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{Local: &corev1.LocalVolumeSource{Path: diskName}}
		} else {
	*/
	// Try to fetch the cluster's default StorageClass
	if defaultSC, err := GetDefaultStorageClass(context.TODO(), k8sClient); err == nil && defaultSC != "" {
		scName = &defaultSC
	} else {
		// No StorageClass, so use label selector and statically bound PVs
		scName = nil
	}
	asmPvc.Spec.NodeAffinity = getAsmNodeAffinity(instance)
	asmPvc.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{Local: &corev1.LocalVolumeSource{Path: diskName}}

	if scName != nil {
		asmPvc.Spec.StorageClassName = *scName
	}

	return asmPvc
}

// BuildServiceDefForOracleRestart provides documentation for the BuildServiceDefForOracleRestart function.
func BuildServiceDefForOracleRestart(instance *oraclerestartdb.OracleRestart, replicaCount int32, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		TypeMeta:   metav1.TypeMeta{Kind: "Service"},
		ObjectMeta: buildSvcObjectMetaForOracleRestart(instance, replicaCount, OracleRestartSpex, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// // Check if user want External Svc on each replica pod
	if strings.ToUpper(svctype) == "VIP" || strings.ToUpper(svctype) == "LOCAL" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = getSvcLabelsForOracleRestart(replicaCount, OracleRestartSpex)

	}

	// if strings.ToUpper(svctype) == "SCAN" {
	// 	service.Spec.ClusterIP = corev1.ClusterIPNone
	// 	service.Spec.Selector = buildLabelsForOracleRestart(instance, "OracleRestart")
	// }

	service.Spec.PublishNotReadyAddresses = true

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.

	return service
}

// BuildExternalServiceDefForOracleRestart provides documentation for the BuildExternalServiceDefForOracleRestart function.
func BuildExternalServiceDefForOracleRestart(instance *oraclerestartdb.OracleRestart, index int32, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, svctype string, opType string) *corev1.Service {
	//service := &corev1.Service{}

	var npSvc oraclerestartdb.OracleRestartNodePortSvc

	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForOracleRestart(instance, index, OracleRestartSpex, opType),
		Spec:       corev1.ServiceSpec{},
	}

	// If user is setting Node Port Service
	if opType == "nodeport" {
		npSvc = instance.Spec.NodePortSvc
		npSvc.PortMappings = instance.Spec.NodePortSvc.PortMappings
		service.Spec.Type = corev1.ServiceTypeNodePort
		if len(instance.Spec.NodePortSvc.SvcAnnotation) != 0 {
			service.Annotations = instance.Spec.NodePortSvc.SvcAnnotation
		}
	} else if opType == "lbservice" {
		npSvc = instance.Spec.LbService
		npSvc.PortMappings = instance.Spec.LbService.PortMappings
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		if len(instance.Spec.LbService.SvcAnnotation) != 0 {
			service.Annotations = instance.Spec.LbService.SvcAnnotation
		}
	}

	//service.Name = npSvc.SvcName

	service.Spec.Selector = getSvcLabelsForOracleRestart(0, OracleRestartSpex)
	service.Spec.Ports = buildOracleRestartSvcPortsDef(npSvc)

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.

	return service
}

// Function to build Service ObjectMeta
// buildSvcObjectMetaForOracleRestart provides documentation for the buildSvcObjectMetaForOracleRestart function.
func buildSvcObjectMetaForOracleRestart(instance *oraclerestartdb.OracleRestart, replicaCount int32, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	//var svcName string

	var labelStr map[string]string
	labelStr = getSvcLabelsForOracleRestart(replicaCount, OracleRestartSpex)

	objmeta := metav1.ObjectMeta{
		Name:      getOracleRestartSvcName(instance, OracleRestartSpex, svctype),
		Namespace: instance.Namespace,
		Labels:    labelStr,
	}

	return objmeta
}

// getOracleRestartSvcName provides documentation for the getOracleRestartSvcName function.
func getOracleRestartSvcName(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, svcType string) string {

	switch svcType {
	case "local":
		return OracleRestartSpex.Name + "-0"
	case "lbservice":
		if instance.Spec.LbService.SvcName != "" {
			return instance.Spec.LbService.SvcName + "-0-lbsvc"
		} else {
			return OracleRestartSpex.Name + "-0-lbsvc"
		}
	case "nodeport":
		if instance.Spec.NodePortSvc.SvcName != "" {
			return instance.Spec.NodePortSvc.SvcName + "-0-npsvc"
		} else {
			return OracleRestartSpex.Name + "-0-npsvc"
		}
	default:
		return OracleRestartSpex.Name
	}
}

// getSvcLabelsForOracleRestart provides documentation for the getSvcLabelsForOracleRestart function.
func getSvcLabelsForOracleRestart(replicaCount int32, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) map[string]string {

	var labelStr map[string]string = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OracleRestartSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OracleRestartSpex.Name + "-0"
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// This function cleanup the shard from GSM
// OraCleanupForOracleRestart provides documentation for the OraCleanupForOracleRestart function.
func OraCleanupForOracleRestart(instance *oraclerestartdb.OracleRestart,
	OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec,
	oldReplicaSize int32,
	newReplicaSize int32,
) string {
	var err1 string
	if oldReplicaSize > newReplicaSize {
		for replicaCount := (oldReplicaSize - 1); replicaCount > (newReplicaSize - 1); replicaCount-- {
			fmt.Println("Deleting the RAC " + OracleRestartSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10))
		}
	}

	err1 = "Test"
	return err1
}

// UpdateProvForOracleRestart provides documentation for the UpdateProvForOracleRestart function.
func UpdateProvForOracleRestart(instance *oraclerestartdb.OracleRestart,
	OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, kClient client.Client, sfSet *appsv1.StatefulSet, gsmPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {

	var msg string
	var size int32 = 1
	var isUpdate bool = false
	// var err error
	//var i int

	msg = "Inside the updateProvForOracleRestart"
	LogMessages("DEBUG", msg, nil, instance, logger)

	// Ensure deployment replicas match the desired state

	if sfSet.Spec.Replicas != nil {
		if *sfSet.Spec.Replicas != size {
			msg = "Current StatefulSet replicas do not match configured Shard Replicas. Gsm is configured with only 1 but current replicas is set with " + strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
			LogMessages("DEBUG", msg, nil, instance, logger)
			isUpdate = true
		}
	}

	if isUpdate {
		sfSet, err := BuildStatefulSetForOracleRestart(instance, OracleRestartSpex, kClient)
		if err != nil {
			msg := fmt.Sprintf("Failed to build StatefulSet for OracleRestart %s: %v", OracleRestartSpex.Name, err)
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

		// Update StatefulSet
		err = kClient.Update(context.Background(), sfSet)
		if err != nil {
			msg := fmt.Sprintf("Failed to update Shard StatefulSet. StatefulSet.Name: %s", sfSet.Name)
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

// ConfigMapSpecs provides documentation for the ConfigMapSpecs function.
func ConfigMapSpecs(instance *oraclerestartdb.OracleRestart, cmData map[string]string, cmName string) *corev1.ConfigMap {
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
func BuildDiskCheckDaemonSet(OracleRestart *oraclerestartdb.OracleRestart) *appsv1.DaemonSet {
	labels := BuildLabelsForDaemonSet(OracleRestart, "disk-check")
	workerNodes := getAllWorkerNodes(OracleRestart)
	privileged := true
	disks := flattenAsmDisks(&OracleRestart.Spec)
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
	for _, disk := range disks {
		volName := sanitizeK8sName(disk) + "-vol"
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: disk, // mount to the same path
		})
	}

	var volumes []corev1.Volume
	for _, disk := range disks {
		volName := sanitizeK8sName(disk) + "-vol"
		hostPathType := corev1.HostPathBlockDev
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: disk,
					Type: &hostPathType, // pointer to the enum
				},
			},
		})

	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk-check-daemonset",
			Namespace: OracleRestart.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
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
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "disk-check",
							Image:   OracleRestart.Spec.Image,
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
					Volumes: volumes,
				},
			},
		},
	}
}

// Helper function to flatten all disk names in AsmStorageDetails, removing duplicates
// flattenAsmDisks provides documentation for the flattenAsmDisks function.
func flattenAsmDisks(racDbSpec *oraclerestartdb.OracleRestartSpec) []string {
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

// CreateServiceAccountIfNotExists provides documentation for the CreateServiceAccountIfNotExists function.
func CreateServiceAccountIfNotExists(instance *oraclerestartdb.OracleRestart, kClient client.Client) error {
	if instance.Spec.SrvAccountName == "" {
		return nil
	}

	ServiceAccountName := instance.Spec.SrvAccountName
	if ServiceAccountName == "" {
		ServiceAccountName = "default"
		return nil
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName,
			Namespace: instance.Namespace,
		},
	}

	existingSA := &corev1.ServiceAccount{}
	err := kClient.Get(context.TODO(), types.NamespacedName{Name: sa.Name, Namespace: sa.Namespace}, existingSA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = kClient.Create(context.TODO(), sa)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

// IsStaticProvisioning provides documentation for the IsStaticProvisioning function.
func IsStaticProvisioning(k8sClient client.Client, instance *oraclerestartdb.OracleRestart) bool {
	if CheckStorageClass(instance) == "NOSC" {
		return false
	}

	var scList storagev1.StorageClassList
	if err := k8sClient.List(context.TODO(), &scList); err != nil {
		return true // fallback to static if we can't query SCs
	}

	// for _, sc := range scList.Items {
	// 	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
	// 		sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
	// 		return false // dynamic provisioning is available
	// 	}
	// }

	return true // no default SC → use static
}

// SwVolumeClaimTemplatesForOracleRestart provides documentation for the SwVolumeClaimTemplatesForOracleRestart function.
func SwVolumeClaimTemplatesForOracleRestart(instance *oraclerestartdb.OracleRestart, OracleRestartSpex oraclerestartdb.OracleRestartInstDetailSpec) corev1.PersistentVolumeClaim {

	// If user-provided PVC name exists, skip volume claim template creation
	//pvcName := GetSwPvcName(OracleRestartSpex.Name)
	// If you are making any change, please refer GetSwPvcName function as we add instance.Spec.InstDetails.Name + "-0"
	pvcName := "odb-sw-pvc-" + instance.Name
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: instance.Namespace,
			Labels:    buildLabelsForOracleRestart(instance, "OracleRestart"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: &instance.Spec.SwStorageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", OracleRestartSpex.SwLocStorageSizeInGb)),
				},
			},
		},
	}
}

// ASMVolumeClaimTemplatesForDG provides documentation for the ASMVolumeClaimTemplatesForDG function.
func ASMVolumeClaimTemplatesForDG(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, StorageClass *string) []corev1.PersistentVolumeClaim {
	var claims []corev1.PersistentVolumeClaim
	mode := corev1.PersistentVolumeBlock
	// If user-provided PVC name exists, skip volume claim template creation
	if len(OracleRestartSpex.PvcName) != 0 {
		return claims
	}

	fmt.Printf("INFO", "working on asm storage class "+*StorageClass)

	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, diskName := range dg.Disks {
			// The folowing peice of code is generating ASM PVC name because by default VolumeCLaim Template add Instance name like -dbmc1-0
			dgType := dg.Type
			disk := diskName[strings.LastIndex(diskName, "/")+1:]
			pvcName := "asm-pvc-" + strings.ToLower(string(dgType)) + "-" + disk + "-" + instance.Name

			claims = append(claims, corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: instance.Namespace,
					Labels:    buildLabelsForOracleRestart(instance, "OracleRestart"),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					VolumeMode:       &mode,
					StorageClassName: StorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							// corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", diskBySize.StorageSizeInGb)),
						},
					},
				},
			})
		}
	}

	return claims
}

// getAllWorkerNodes provides documentation for the getAllWorkerNodes function.
func getAllWorkerNodes(instance *oraclerestartdb.OracleRestart) []string {
	nodeSet := map[string]struct{}{}
	for _, node := range instance.Spec.InstDetails.WorkerNode {
		nodeSet[node] = struct{}{}
	}

	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	return nodes
}

// BuildLabelsForDaemonSet provides documentation for the BuildLabelsForDaemonSet function.
func BuildLabelsForDaemonSet(instance *oraclerestart.OracleRestart, label string) map[string]string {
	return map[string]string{
		"cluster": label,
	}

}
