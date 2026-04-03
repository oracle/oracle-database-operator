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

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants for hello-stateful StatefulSet & Volumes
func buildLabelsForGsm(instance *databasev4.ShardingDatabase, label string, gsmName string) map[string]string {
	// Keep selector labels stable to avoid StatefulSet selector immutability issues.
	return map[string]string{
		"app":        "OracleGsming",
		"shard_name": "Gsm",
		"oralabel":   getLabelForGsm(instance),
	}
}

func buildOwnerRefForGsm(instance *databasev4.ShardingDatabase) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(instance, databasev4.GroupVersion.WithKind("ShardingDatabase")),
	}
}

func buildResourceLabelsForGsm(instance *databasev4.ShardingDatabase, label string, gsmName string) map[string]string {
	labels := buildLabelsForGsm(instance, label, gsmName)
	labels["app.kubernetes.io/name"] = "oracle-sharding"
	labels["app.kubernetes.io/instance"] = instance.Name
	labels["app.kubernetes.io/component"] = "gsm"
	labels["app.kubernetes.io/managed-by"] = "oracle-database-operator"
	labels["app.kubernetes.io/part-of"] = "oracle-database"
	labels["sharding.oracle.com/database"] = instance.Name
	labels["sharding.oracle.com/gsm"] = gsmName
	if label != "" {
		labels["sharding.oracle.com/kind"] = label
	}
	return labels
}

func getLabelForGsm(instance *databasev4.ShardingDatabase) string {

	//  if len(OraGsmSpex.Label) !=0 {
	//     return OraGsmSpex.Label
	//   }

	return instance.Name
}

func BuildStatefulSetForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) *appsv1.StatefulSet {
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForGsm(),
		ObjectMeta: buildObjectMetaForGsm(instance, OraGsmSpex),
		Spec:       *buildStatefulSpecForGsm(instance, OraGsmSpex),
	}
	return sfset
}

// Function to build TypeMeta
func buildTypeMetaForGsm() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
func buildObjectMetaForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) metav1.ObjectMeta {
	objmeta := metav1.ObjectMeta{
		Name:            OraGsmSpex.Name,
		Namespace:       instance.Namespace,
		Labels:          buildResourceLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
		OwnerReferences: buildOwnerRefForGsm(instance),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) *appsv1.StatefulSetSpec {
	// building Stateful set Specs
	replicas := shardReplicaCount

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraGsmSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildResourceLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
			},
			Spec: *buildPodSpecForGsm(instance, OraGsmSpex),
		},
		VolumeClaimTemplates: volumeClaimTemplatesForGsm(instance, OraGsmSpex),
	}
	sfsetspec.Replicas = &replicas
	/**
	if OraGsmSpex.Replicas == 0 {
		OraGsmSpex.Replicas = 1
		sfsetspec.Replicas = &OraGsmSpex.Replicas
	} else {
		OraGsmSpex.Replicas = 1
		sfsetspec.Replicas = &OraGsmSpex.Replicas
	}
	**/

	return sfsetspec
}

// Function to build PodSpec

func buildPodSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) *corev1.PodSpec {

	user := oraRunAsUser
	group := oraFsGroup
	spec := &corev1.PodSpec{
		SecurityContext: mergePodSecurityContextWithDefaults(&corev1.PodSecurityContext{
			RunAsNonRoot: BoolPointer(true),
			RunAsUser:    &user,
			RunAsGroup:   &group,
			FSGroup:      &group,
		}, OraGsmSpex.SecurityContext),
		HostAliases:        cloneHostAliases(instance.Spec.HostAliases),
		Containers:         buildContainerSpecForGsm(instance, OraGsmSpex),
		Volumes:            buildVolumeSpecForGsm(instance, OraGsmSpex),
		ServiceAccountName: instance.Spec.SrvAccountName,
	}

	if (instance.Spec.IsDownloadScripts) && (instance.Spec.ScriptsLocation != "") {
		spec.InitContainers = buildInitContainerSpecForGsm(instance, OraGsmSpex)
	}

	if len(instance.Spec.GsmImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: instance.Spec.GsmImagePullSecret,
			},
		}
	}
	if len(OraGsmSpex.NodeSelector) > 0 {
		spec.NodeSelector = make(map[string]string)
		for key, value := range OraGsmSpex.NodeSelector {
			spec.NodeSelector[key] = value
		}
	}
	return spec
}

// Function to build Volume Spec
func buildVolumeSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.Volume {
	var result []corev1.Volume
	pvcMounts := normalizeGsmPVCMountConfigs(OraGsmSpex.Name, OraGsmSpex.StorageSizeInGb, instance.Spec.StorageClass, OraGsmSpex.DisableDefaultLogVolumeClaims, OraGsmSpex.AdditionalPVCs)
	result = []corev1.Volume{
		{
			Name: OraGsmSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.DbSecret.Name,
				},
			},
		},
		{
			Name: OraGsmSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if OraGsmSpex.GsmConfigData != nil && len(OraGsmSpex.GsmConfigData.Name) != 0 {
		result = append(result, corev1.Volume{Name: OraGsmSpex.Name + "-oradata-configdata", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: OraGsmSpex.GsmConfigData.Name}}}})
	}

	for _, pvcMount := range pvcMounts {
		if strings.TrimSpace(pvcMount.pvcName) == "" {
			continue
		}
		result = append(result, corev1.Volume{
			Name:         pvcMount.volumeName,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcMount.pvcName}},
		})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraGsmSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}

	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.Volume{Name: OraGsmSpex.Name + "orascript-vol5", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}
	return result
}

// Function to build the container Specification
func buildContainerSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	var masterGsmFlag = false
	var idx int
	user := oraRunAsUser
	group := oraFsGroup
	// Get the Idx
	if instance.Spec.Gsm[0].Name == OraGsmSpex.Name {
		masterGsmFlag = true
		idx = 0
	} else {
		masterGsmFlag = false
		idx = 1
	}
	directorParams := buildDirectorParams(instance, OraGsmSpex, idx)

	containerSpec := corev1.Container{
		Name:  OraGsmSpex.Name,
		Image: instance.Spec.GsmImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			RunAsUser:                &user,
			RunAsGroup:               &group,
			AllowPrivilegeEscalation: BoolPointer(false),
			Capabilities: mergeCapabilitiesWithDefaults(&corev1.Capabilities{
				Add:  []corev1.Capability{"NET_RAW"},
				Drop: []corev1.Capability{"ALL"},
			}, OraGsmSpex.Capabilities),
		},
		VolumeMounts: buildVolumeMountSpecForGsm(instance, OraGsmSpex),
		LivenessProbe: buildExecProbe(
			getLivenessCmd("GSM"),
			30,
			func() int32 {
				if instance.Spec.LivenessCheckPeriod > 0 {
					return int32(instance.Spec.LivenessCheckPeriod)
				}
				return 60
			}(),
			20,
			3,
		),
		/**
		StartupProbe: &corev1.Probe{
			FailureThreshold: int32(30),
			PeriodSeconds:    int32(120),
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("GSM"),
				},
			},
		},
		**/
		Env: buildEnvVarsSpec(instance, OraGsmSpex.EnvVars, OraGsmSpex.Name, "GSM", masterGsmFlag, directorParams, "", nil),
	}
	if OraGsmSpex.Resources != nil {
		containerSpec.Resources = *OraGsmSpex.Resources
	}
	// building Complete Container Spec
	result = []corev1.Container{
		containerSpec,
	}
	return result
}

// Function to build the init Container Spec
func buildInitContainerSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.Container {
	var result []corev1.Container
	// building the init Container Spec
	privFlag := true
	// var uid int64 = 0
	uid := oraRunAsUser
	var scriptLoc string
	if len(instance.Spec.ScriptsLocation) != 0 {
		scriptLoc = instance.Spec.ScriptsLocation
	} else {
		scriptLoc = "WEB"
	}

	init1spec := corev1.Container{
		Name:  OraGsmSpex.Name + "-init1",
		Image: instance.Spec.GsmImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(false),
			AllowPrivilegeEscalation: BoolPointer(true),
			Privileged:               &privFlag,
			RunAsUser:                &uid,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Command: []string{
			"/bin/bash",
			"-c",
			getGsmInitContainerCmd(scriptLoc, instance.Name),
		},
		VolumeMounts: buildVolumeMountSpecForGsm(instance, OraGsmSpex),
	}

	// building Complete Init Container Spec
	if OraGsmSpex.ImagePulllPolicy != nil {
		init1spec.ImagePullPolicy = *OraGsmSpex.ImagePulllPolicy
	}
	result = []corev1.Container{
		init1spec,
	}
	return result
}

func buildVolumeMountSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.VolumeMount {
	result := make([]corev1.VolumeMount, 0, 8)
	pvcMounts := normalizeGsmPVCMountConfigs(OraGsmSpex.Name, OraGsmSpex.StorageSizeInGb, instance.Spec.StorageClass, OraGsmSpex.DisableDefaultLogVolumeClaims, OraGsmSpex.AdditionalPVCs)
	result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "secretmap-vol3", MountPath: getDbSecretMountPath(instance), ReadOnly: true})
	for _, pvcMount := range pvcMounts {
		result = append(result, corev1.VolumeMount{Name: pvcMount.volumeName, MountPath: pvcMount.mountPath})
	}
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "orascript-vol5", MountPath: oraScriptMount})
	}
	result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if OraGsmSpex.GsmConfigData != nil && len(OraGsmSpex.GsmConfigData.Name) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "-oradata-configdata", MountPath: OraGsmSpex.GsmConfigData.MountPath})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	return result
}

func volumeClaimTemplatesForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.PersistentVolumeClaim {
	pvcMounts := normalizeGsmPVCMountConfigs(OraGsmSpex.Name, OraGsmSpex.StorageSizeInGb, instance.Spec.StorageClass, OraGsmSpex.DisableDefaultLogVolumeClaims, OraGsmSpex.AdditionalPVCs)
	claims := make([]corev1.PersistentVolumeClaim, 0, len(pvcMounts))
	for _, pvcMount := range pvcMounts {
		if strings.TrimSpace(pvcMount.pvcName) != "" {
			continue
		}
		claim := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:            pvcMount.volumeName,
				Namespace:       instance.Namespace,
				Labels:          buildResourceLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
				OwnerReferences: buildOwnerRefForGsm(instance),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: storageClassNamePtr(pvcMount.storageClass),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(pvcMount.storageSizeInGb), 10) + "Gi"),
					},
				},
			},
		}
		claims = append(claims, claim)
	}

	return claims
}

func BuildServiceDefForGsm(instance *databasev4.ShardingDatabase, replicaCount int32, OraGsmSpex databasev4.GsmSpec, svctype string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForGsm(instance, replicaCount, OraGsmSpex, svctype),
		Spec: corev1.ServiceSpec{
			Selector: getSvcLabelsForGsm(replicaCount, OraGsmSpex),
			Ports:    buildSvcPortsDef(instance, "GSM"),
		},
	}

	switch svctype {
	case shardServiceTypeExternal:
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
	case shardServiceTypeLocal:
		service.Spec.ClusterIP = corev1.ClusterIPNone
	}
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForGsm(instance *databasev4.ShardingDatabase, replicaCount int32, OraGsmSpex databasev4.GsmSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	var svcName string
	if svctype == shardServiceTypeLocal {
		svcName = OraGsmSpex.Name
	}

	if svctype == shardServiceTypeExternal {
		svcName = OraGsmSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}
	labels := buildResourceLabelsForGsm(instance, "sharding", OraGsmSpex.Name)
	labels["sharding.oracle.com/service-type"] = svctype

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Namespace,
		Labels:          labels,
		Annotations:     resolveGsmServiceAnnotations(instance, OraGsmSpex, svctype),
		OwnerReferences: buildOwnerRefForGsm(instance),
	}
	return objmeta
}

func getSvcLabelsForGsm(replicaCount int32, OraGsmSpex databasev4.GsmSpec) map[string]string {

	var labelStr map[string]string = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraGsmSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraGsmSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10)
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// This function cleanup the shard from GSM
func OraCleanupForGsm(instance *databasev4.ShardingDatabase,
	OraGsmSpex databasev4.GsmSpec,
	oldReplicaSize int32,
	newReplicaSize int32,
) string {
	var err1 string
	if oldReplicaSize > newReplicaSize {
		for replicaCount := (oldReplicaSize - 1); replicaCount > (newReplicaSize - 1); replicaCount-- {
			fmt.Println("Deleting the Gsm from GSM" + OraGsmSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10))
		}
	}

	err1 = "Test"
	return err1
}

func UpdateProvForGsm(instance *databasev4.ShardingDatabase,
	OraGsmSpex databasev4.GsmSpec, kClient client.Client, sfSet *appsv1.StatefulSet, gsmPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {
	_ = gsmPod

	var msg string
	requiresUpdate := false

	msg = "Inside the updateProvForGsm"
	LogMessages("DEBUG", msg, nil, instance, logger)

	// Ensure deployment replicas match the desired state

	// Ensure deployment replicas match the desired state
	if sfSet.Spec.Replicas == nil || *sfSet.Spec.Replicas != shardReplicaCount {
		currentReplica := "nil"
		if sfSet.Spec.Replicas != nil {
			currentReplica = strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
		}
		msg = "Current StatefulSet replicas do not match configured GSM Replicas. Gsm is configured with only 1 but current replicas is set with " + currentReplica
		LogMessages("DEBUG", msg, nil, instance, logger)
		requiresUpdate = true
	}

	if OraGsmSpex.Resources != nil {
		for i := range sfSet.Spec.Template.Spec.Containers {
			if sfSet.Spec.Template.Spec.Containers[i].Name != OraGsmSpex.Name {
				continue
			}
			if sfSet.Spec.Template.Spec.Containers[i].Resources.String() != OraGsmSpex.Resources.String() {
				requiresUpdate = true
			}
			break
		}
	}

	/**

	for i = 0; i < len(sfSet.Spec.VolumeClaimTemplates); i++ {
		if sfSet.Spec.VolumeClaimTemplates[i].Name == OraGsmSpex.Name+"-oradata-vol4" {
			volumeSize := sfSet.Spec.VolumeClaimTemplates[i].Size()
			if volumeSize != int(OraGsmSpex.StorageSizeInGb) {
				isUpdate = true
			}

		}
	}

	**/

	if requiresUpdate {
		err := kClient.Update(context.Background(), BuildStatefulSetForGsm(instance, OraGsmSpex))
		if err != nil {
			msg = "Failed to update Shard StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}
