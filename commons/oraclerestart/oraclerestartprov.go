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

// Constants for rac-stateful StatefulSet & Volumes
func buildLabelsForOracleRestart(instance *oraclerestart.OracleRestart, label string) map[string]string {
	return map[string]string{
		"cluster": "oraclerestart",
	}

	// "oralabel": getLabelForOracleRestart(instance),
}

func buildLabelsForAsmPv(instance *oraclerestart.OracleRestart, label string, index int) map[string]string {
	return map[string]string{
		"asm_vol": "block-asm-pv-" + getLabelForOracleRestart(instance) + "-" + fmt.Sprint(index),
	}
}

func getLabelForOracleRestart(instance *oraclerestart.OracleRestart) string {

	return instance.Name
}

func BuildStatefulSetForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, kClient client.Client) *appsv1.StatefulSet {
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForOracleRestart(),
		ObjectMeta: builObjectMetaForOracleRestart(instance, OracleRestartSpex),
		Spec:       *buildStatefulSpecForOracleRestart(instance, OracleRestartSpex, kClient),
	}
	return sfset
}

// Function to build TypeMeta
func buildTypeMetaForOracleRestart() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
func builObjectMetaForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:      OracleRestartSpex.Name,
		Namespace: instance.Namespace,
		Labels:    buildLabelsForOracleRestart(instance, "OracleRestart"),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForOracleRestart(
	instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec,
	kClient client.Client,
) *appsv1.StatefulSetSpec {

	// Build PodSpec first
	podSpec := buildPodSpecForOracleRestart(instance, OracleRestartSpex)

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
	// Add volume claim templates if a storage class is specified
	if len(instance.Spec.StorageClass) != 0 && !asmPvcsExist(instance, kClient) {
		sfsetspec.VolumeClaimTemplates = ASMVolumeClaimTemplatesForOracleRestart(instance, OracleRestartSpex)
	}

	if len(instance.Spec.StorageClass) != 0 && len(instance.Spec.InstDetails.HostSwLocation) == 0 {
		sfsetspec.VolumeClaimTemplates = append(sfsetspec.VolumeClaimTemplates, SwVolumeClaimTemplatesForOracleRestart(instance, OracleRestartSpex))
	}
	// Add annotations to the Pod template
	// sfsetspec.Template.Annotations = generateNetworkDetails(instance, OracleRestartSpex, kClient)

	return sfsetspec
}

func asmPvcsExist(instance *oraclerestart.OracleRestart, kClient client.Client) bool {
	for i := range instance.Spec.AsmStorageDetails.DisksBySize {
		pvcName := GetAsmPvcName(i, instance.Name)
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
	return true
}

// Function to build PodSpec

func buildPodSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) *corev1.PodSpec {

	spec := &corev1.PodSpec{
		Hostname:       OracleRestartSpex.Name + "-0",
		Subdomain:      utils.OraSubDomain,
		InitContainers: buildInitContainerSpecForOracleRestart(instance, OracleRestartSpex),
		Containers:     buildContainerSpecForOracleRestart(instance, OracleRestartSpex),
		Volumes:        buildVolumeSpecForOracleRestart(instance, OracleRestartSpex),
		Affinity:       getNodeAffinity(instance, OracleRestartSpex),
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
	return spec
}

// Function get the Node Affinity
func getNodeAffinity(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) *corev1.Affinity {

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
func getAsmNodeAffinity(instance *oraclerestart.OracleRestart, index int, disk *oraclerestart.AsmDiskDetails) *corev1.VolumeNodeAffinity {

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
func buildVolumeSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: OracleRestartSpex.Name + "-ssh-secretmap-vol",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.SshKeySecret.Name,
				},
			},
		},
		{
			Name: OracleRestartSpex.Name + "-oradshm-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
			},
		},
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
		if instance.Spec.StorageClass != "" {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-oradata-sw-vol", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: OracleRestartSpex.Name + "-oradata-sw-vol-pvc-" + OracleRestartSpex.Name + "-0"}}})
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
	}

	if len(OracleRestartSpex.PvcName) != 0 {
		for source := range OracleRestartSpex.PvcName {
			result = append(result, corev1.Volume{Name: OracleRestartSpex.Name + "-ora-vol-" + source, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: source}}})
		}
	}

	if instance.Spec.AsmStorageDetails != nil {
		// Iterate over the DisksBySize slice
		for _, diskBySize := range instance.Spec.AsmStorageDetails.DisksBySize {
			// For each DiskBySize, append PVCs for the disks in DiskNames
			for index := range diskBySize.DiskNames {
				// Construct PVC name based on index and instance name
				pvcName := getAsmPvcName(index, instance.Name)
				result = append(result, corev1.Volume{
					Name: pvcName + "-pvc",
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
func buildContainerSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	privileged := false
	failureThreshold := 1
	periodSeconds := 5
	initialDelaySeconds := 120
	oraLsnrPort := 1521

	// Get the Idx

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
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForOracleRestart(instance, OracleRestartSpex),
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

func getAsmVolumeDevices(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.VolumeDevice {
	var result []corev1.VolumeDevice

	if instance.Spec.AsmStorageDetails != nil {
		// Iterate over the DisksBySize slice
		for _, diskBySize := range instance.Spec.AsmStorageDetails.DisksBySize {
			// For each disk in DiskNames, create a VolumeDevice
			for _, diskName := range diskBySize.DiskNames {
				// Create PVC name and append VolumeDevice to the result
				pvcName := getAsmPvcName(len(result), instance.Name)
				result = append(result, corev1.VolumeDevice{Name: pvcName + "-pvc", DevicePath: diskName})
			}
		}
	}

	return result
}

// Function to build the init Container Spec
func buildInitContainerSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.Container {
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

func buildVolumeMountSpecForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-ssh-secretmap-vol", MountPath: instance.Spec.SshKeySecret.KeyMountLocation, ReadOnly: true})
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
	} else if len(instance.Spec.StorageClass) != 0 {
		result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-oradata-sw-vol", MountPath: instance.Spec.ConfigParams.SwMountLocation})
	} else {
		fmt.Println("No Location is passed for the software storage in" + OracleRestartSpex.Name)
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
		}
	}

	if len(OracleRestartSpex.PvcName) != 0 {
		for source, target := range OracleRestartSpex.PvcName {
			result = append(result, corev1.VolumeMount{Name: OracleRestartSpex.Name + "-ora-vol-" + source, MountPath: target})
		}
	}

	return result
}

func VolumePVCForASM(instance *oraclerestart.OracleRestart, index int, diskName string, size int, asmStorage *oraclerestart.AsmDiskDetails, k8sClient client.Client) *corev1.PersistentVolumeClaim {
	// Set volume mode to block
	volumeBlock := corev1.PersistentVolumeBlock

	// Create PersistentVolumeClaim
	asmPvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getAsmPvcName(index, instance.Name), // Use size to determine index
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
					corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(size), 10) + "Gi")},
			},
		},
	}

	// Check if a StorageClass is defined and set it
	if len(instance.Spec.StorageClass) != 0 {
		asmPvc.Spec.StorageClassName = &instance.Spec.StorageClass
	} else {
		// If no StorageClass, use the LabelSelector based on disk size
		asmPvc.Spec.Selector = &metav1.LabelSelector{MatchLabels: buildLabelsForAsmPv(instance, string(diskName), index)}
		asmPvc.Spec.StorageClassName = nil
	}

	return asmPvc
}

func VolumePVForASM(instance *oraclerestart.OracleRestart, index int, diskName string, size int, asmStorage *oraclerestart.AsmDiskDetails, k8sClient client.Client) *corev1.PersistentVolume {
	volumeBlock := corev1.PersistentVolumeBlock

	asmPvc := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getAsmPvName(index, instance.Name),
			Namespace: instance.Namespace,
			Labels:    buildLabelsForAsmPv(instance, diskName, index),
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			VolumeMode: &volumeBlock,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(size), 10) + "Gi")},
		},
	}

	if len(instance.Spec.StorageClass) != 0 {
		asmPvc.Spec.StorageClassName = instance.Spec.StorageClass
		asmPvc.Spec.NodeAffinity = getAsmNodeAffinity(instance, index, asmStorage)
		asmPvc.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{Local: &corev1.LocalVolumeSource{Path: diskName}}

	} else {

		asmPvc.Spec.NodeAffinity = getAsmNodeAffinity(instance, index, asmStorage)
		asmPvc.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{Local: &corev1.LocalVolumeSource{Path: diskName}}
	}
	return asmPvc
}

func BuildServiceDefForOracleRestart(instance *oraclerestart.OracleRestart, replicaCount int32, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, svctype string) *corev1.Service {
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

func BuildExternalServiceDefForOracleRestart(instance *oraclerestart.OracleRestart, index int32, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, svctype string, opType string) *corev1.Service {
	//service := &corev1.Service{}

	var npSvc oraclerestart.OracleRestartNodePortSvc

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
func buildSvcObjectMetaForOracleRestart(instance *oraclerestart.OracleRestart, replicaCount int32, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, svctype string) metav1.ObjectMeta {
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

func getOracleRestartSvcName(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, svcType string) string {

	switch svcType {
	case "local":
		return OracleRestartSpex.Name + "-0-local"
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

func getSvcLabelsForOracleRestart(replicaCount int32, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) map[string]string {

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
func OraCleanupForOracleRestart(instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec,
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

func UpdateProvForOracleRestart(instance *oraclerestart.OracleRestart,
	OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec, kClient client.Client, sfSet *appsv1.StatefulSet, gsmPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {

	var msg string
	var size int32 = 1
	var isUpdate bool = false
	var err error
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
		err = kClient.Update(context.Background(), BuildStatefulSetForOracleRestart(instance, OracleRestartSpex, kClient))
		if err != nil {
			msg = "Failed to update Shard StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

func ConfigMapSpecs(instance *oraclerestart.OracleRestart, cmData map[string]string, cmName string) *corev1.ConfigMap {
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

func BuildDiskCheckDaemonSet(OracleRestart *oraclerestart.OracleRestart) *appsv1.DaemonSet {
	labels := buildLabelsForOracleRestart(OracleRestart, "disk-check")

	// Prepare the volume devices based on the PVCs
	var volumeDevices []corev1.VolumeDevice
	var volumes []corev1.Volume
	disks := flattenDisksBySize(&OracleRestart.Spec)
	for index, diskPath := range disks {
		pvcName := GetAsmPvcName(index, OracleRestart.Name)
		volumeName := pvcName + "-pvc"

		volumeDevices = append(volumeDevices, corev1.VolumeDevice{
			Name:       volumeName,
			DevicePath: diskPath,
		})

		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}

	// Join the disk names into a space-separated string
	// Flatten the DisksBySize map to get a single slice of all disk names
	diskNamesSlice := flattenDisksBySize(&OracleRestart.Spec)

	// Join the flattened list of disk names into a single space-separated string
	diskNames := strings.Join(diskNamesSlice, " ")

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
												Values:   OracleRestart.Spec.InstDetails.WorkerNode,
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
							Args: []string{
								"for disk in " + diskNames + "; do " +
									"if [ ! -e $disk ]; then " +
									"echo Disk $disk is not a valid block device; " +
									"exit 1; " +
									"else " +
									"echo Disk $disk is valid; " +
									"fi; " +
									"done; " +
									"sleep 3600",
							},
							VolumeDevices: volumeDevices,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
}

// Helper function to flatten DisksBySize into a single slice of disk names
func flattenDisksBySize(oraclerestartSpec *oraclerestart.OracleRestartSpec) []string {
	disksBySize := oraclerestartSpec.AsmStorageDetails.DisksBySize
	var allDisks []string
	for _, diskBySize := range disksBySize {
		allDisks = append(allDisks, diskBySize.DiskNames...)
	}
	return allDisks
}

func CreateServiceAccountIfNotExists(instance *oraclerestart.OracleRestart, kClient client.Client) error {
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

func IsStaticProvisioning(k8sClient client.Client, instance *oraclerestart.OracleRestart) bool {
	if instance.Spec.StorageClass != "" {
		return false
	}

	var scList storagev1.StorageClassList
	if err := k8sClient.List(context.TODO(), &scList); err != nil {
		return true // fallback to static if we can't query SCs
	}

	for _, sc := range scList.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return false // dynamic provisioning is available
		}
	}

	return true // no default SC → use static
}

func SwVolumeClaimTemplatesForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) corev1.PersistentVolumeClaim {

	// If user-provided PVC name exists, skip volume claim template creation
	pvcName := OracleRestartSpex.Name + "-oradata-sw-vol-pvc"
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
			StorageClassName: &instance.Spec.StorageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", OracleRestartSpex.SwLocStorageSizeInGb)),
				},
			},
		},
	}
}

func ASMVolumeClaimTemplatesForOracleRestart(instance *oraclerestart.OracleRestart, OracleRestartSpex oraclerestart.OracleRestartInstDetailSpec) []corev1.PersistentVolumeClaim {
	var claims []corev1.PersistentVolumeClaim
	mode := corev1.PersistentVolumeBlock
	// If user-provided PVC name exists, skip volume claim template creation
	if len(OracleRestartSpex.PvcName) != 0 {
		return claims
	}

	index := 0
	for _, diskBySize := range instance.Spec.AsmStorageDetails.DisksBySize {
		for range diskBySize.DiskNames {
			pvcName := GetAsmPvcName(index, instance.Name)

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
					StorageClassName: &instance.Spec.StorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", diskBySize.StorageSizeInGb)),
						},
					},
				},
			})
			index++
		}
	}

	return claims
}
