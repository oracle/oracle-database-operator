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
	"reflect"
	"strconv"
	"strings"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildLabelsForShard(instance *databasev4.ShardingDatabase, label string, shardName string) map[string]string {
	return map[string]string{
		"app":      "OracleSharding",
		"type":     "Shard",
		"oralabel": getLabelForShard(instance),
	}
}

func getLabelForShard(instance *databasev4.ShardingDatabase) string {

	//  if len(OraShardSpex.Label) !=0 {
	//     return OraShardSpex.Label
	//   }

	return instance.Name
}

func BuildStatefulSetForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) *appsv1.StatefulSet {
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForShard(),
		ObjectMeta: builObjectMetaForShard(instance, OraShardSpex),
		Spec:       *buildStatefulSpecForShard(instance, OraShardSpex),
	}

	return sfset
}

// Function to build TypeMeta
func buildTypeMetaForShard() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
func builObjectMetaForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:            OraShardSpex.Name,
		Namespace:       instance.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) *appsv1.StatefulSetSpec {
	// building Stateful set Specs
	var size int32 = 1
	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraShardSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
			},
			Spec: *buildPodSpecForShard(instance, OraShardSpex),
		},
		VolumeClaimTemplates: volumeClaimTemplatesForShard(instance, OraShardSpex),
	}
	//	if OraShardSpex.OraShardSize == 0 {
	//		OraShardSpex.OraShardSize = 1
	sfsetspec.Replicas = &size
	//	} else {
	//		sfsetspec.Replicas = &OraShardSpex.OraShardSize
	//	}

	return sfsetspec
}

// Function to build PodSpec

func buildPodSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) *corev1.PodSpec {

	user := oraRunAsUser
	group := oraFsGroup
	spec := &corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: BoolPointer(true),
			RunAsUser:    &user,
			RunAsGroup:   &group,
			FSGroup:      &group,
		},
		Containers:         buildContainerSpecForShard(instance, OraShardSpex),
		Volumes:            buildVolumeSpecForShard(instance, OraShardSpex),
		ServiceAccountName: instance.Spec.SrvAccountName,
	}

	if (instance.Spec.IsDownloadScripts) && (instance.Spec.ScriptsLocation != "") {
		spec.InitContainers = buildInitContainerSpecForShard(instance, OraShardSpex)
	}

	if len(instance.Spec.DbImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: instance.Spec.DbImagePullSecret,
			},
		}
	}

	if len(OraShardSpex.NodeSelector) > 0 {
		spec.NodeSelector = make(map[string]string)
		for key, value := range OraShardSpex.NodeSelector {
			spec.NodeSelector[key] = value
		}
	}

	return spec
}

// Function to build Volume Spec
func buildVolumeSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: OraShardSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.DbSecret.Name,
				},
			},
		},
		{
			Name: OraShardSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if OraShardSpex.ShardConfigData != nil && len(OraShardSpex.ShardConfigData.Name) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "-oradata-configdata", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: OraShardSpex.ShardConfigData.Name}}}})
	}

	if len(OraShardSpex.PvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "oradata-vol4", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: OraShardSpex.PvcName}}})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "orascript-vol5", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}

	if checkTdeWalletFlag(instance) {
		if len(instance.Spec.FssStorageClass) == 0 && len(instance.Spec.TdeWalletPvc) > 0 {
			result = append(result, corev1.Volume{Name: OraShardSpex.Name + "shared-storage-vol8", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.TdeWalletPvc}}})
		}
	}

	return result
}

// Function to build the container Specification
func buildContainerSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	user := oraRunAsUser
	group := oraFsGroup
	containerSpec := corev1.Container{
		Name:  OraShardSpex.Name,
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			RunAsUser:                &user,
			RunAsGroup:               &group,
			AllowPrivilegeEscalation: BoolPointer(false),
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{corev1.Capability("NET_ADMIN"), corev1.Capability("SYS_NICE")},
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),
		LivenessProbe: &corev1.Probe{
			// TODO: Investigate if it's ok to call status every 10 seconds
			FailureThreshold:    int32(3),
			InitialDelaySeconds: int32(30),
			PeriodSeconds: func() int32 {
				if instance.Spec.LivenessCheckPeriod > 0 {
					return int32(instance.Spec.LivenessCheckPeriod)
				}
				return 60
			}(),
			TimeoutSeconds: int32(30),
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "},
				},
			},
		},
		/**
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					//Command: getReadinessCmd("SHARD"),
					Command: []string{"/bin/sh", "-c", "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "},
				},
			},
			InitialDelaySeconds: 20,
			TimeoutSeconds:      20,
			PeriodSeconds: func() int32 {
				if instance.Spec.ReadinessCheckPeriod > 0 {
					return int32(instance.Spec.ReadinessCheckPeriod)
				}
				return 60
			}(),
		},
		**/
		// Disabling this because ping stop working and sharding topologu never gets configured.
		StartupProbe: &corev1.Probe{
			FailureThreshold:    int32(30),
			PeriodSeconds:       int32(180),
			InitialDelaySeconds: int32(30),
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "},
				},
			},
		},
		Env: buildEnvVarsSpec(instance, OraShardSpex.EnvVars, OraShardSpex.Name, "SHARD", false, "NONE"),
	}

	if instance.Spec.IsClone {
		containerSpec.Command = []string{orainitCmd3}
	}

	if OraShardSpex.Resources != nil {
		containerSpec.Resources = *OraShardSpex.Resources
	}
	// building Complete Container Spec
	result = []corev1.Container{
		containerSpec,
	}
	return result
}

// Function to build the init Container Spec
func buildInitContainerSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.Container {
	var result []corev1.Container
	privFlag := false
	var uid int64 = 0

	// building the init Container Spec
	var scriptLoc string
	if len(instance.Spec.ScriptsLocation) != 0 {
		scriptLoc = instance.Spec.ScriptsLocation
	} else {
		scriptLoc = "WEB"
	}

	init1spec := corev1.Container{
		Name:  OraShardSpex.Name + "-init1",
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			AllowPrivilegeEscalation: BoolPointer(false),
			Privileged:               &privFlag,
			RunAsUser:                &uid,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Command: []string{
			"/bin/bash",
			"-c",
			getInitContainerCmd(scriptLoc, instance.Name),
		},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),
	}

	// building Complete Init Container Spec
	if OraShardSpex.ImagePulllPolicy != nil {
		init1spec.ImagePullPolicy = *OraShardSpex.ImagePulllPolicy
	}
	result = []corev1.Container{
		init1spec,
	}
	return result
}

func buildVolumeMountSpecForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "secretmap-vol3", MountPath: oraSecretMount, ReadOnly: true})
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "-oradata-vol4", MountPath: oraDataMount})
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orascript-vol5", MountPath: oraDbScriptMount})
	}
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if OraShardSpex.ShardConfigData != nil && len(OraShardSpex.ShardConfigData.Name) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "-oradata-configdata", MountPath: OraShardSpex.ShardConfigData.MountPath})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	if checkTdeWalletFlag(instance) {
		if len(instance.Spec.FssStorageClass) > 0 && len(instance.Spec.TdeWalletPvc) == 0 {
			result = append(result, corev1.VolumeMount{Name: instance.Name + "shared-storage" + instance.Spec.Catalog[0].Name + "-0", MountPath: getTdeWalletMountLoc(instance)})
		} else {
			if len(instance.Spec.FssStorageClass) == 0 && len(instance.Spec.TdeWalletPvc) > 0 {
				result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "shared-storage-vol8", MountPath: getTdeWalletMountLoc(instance)})
			}
		}
	}

	return result
}

func volumeClaimTemplatesForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) []corev1.PersistentVolumeClaim {

	var claims []corev1.PersistentVolumeClaim

	if len(OraShardSpex.PvcName) != 0 {
		return claims
	}

	claims = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            OraShardSpex.Name + "-oradata-vol4",
				Namespace:       instance.Namespace,
				OwnerReferences: getOwnerRef(instance),
				Labels:          buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(OraShardSpex.StorageSizeInGb), 10) + "Gi"),
					},
				},
			},
		},
	}

	if len(OraShardSpex.PvAnnotations) > 0 {
		claims[0].ObjectMeta.Annotations = make(map[string]string)
		for key, value := range OraShardSpex.PvAnnotations {
			claims[0].ObjectMeta.Annotations[key] = value
		}
	}

	if len(OraShardSpex.PvMatchLabels) > 0 {
		claims[0].Spec.Selector = &metav1.LabelSelector{MatchLabels: OraShardSpex.PvMatchLabels}
	}

	return claims
}

func BuildServiceDefForShard(instance *databasev4.ShardingDatabase, replicaCount int32, OraShardSpex databasev4.ShardSpec, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForShard(instance, replicaCount, OraShardSpex, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// Check if user want External Svc on each replica pod
	if svctype == "external" {
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Spec.Selector = getSvcLabelsForShard(replicaCount, OraShardSpex)
	}

	if svctype == "local" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = getSvcLabelsForShard(replicaCount, OraShardSpex)
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.
	service.Spec.Ports = buildSvcPortsDef(instance, "SHARD")
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForShard(instance *databasev4.ShardingDatabase, replicaCount int32, OraShardSpex databasev4.ShardSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta

	var svcName string

	if svctype == "local" {
		svcName = OraShardSpex.Name
	}

	if svctype == "external" {
		svcName = OraShardSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Namespace,
		Labels:          buildLabelsForShard(instance, "sharding", OraShardSpex.Name),
		OwnerReferences: getOwnerRef(instance),
	}
	return objmeta
}

func getSvcLabelsForShard(replicaCount int32, OraShardSpex databasev4.ShardSpec) map[string]string {

	var labelStr map[string]string = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10)
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// ======================== Update Section ========================
func UpdateProvForShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec, kClient client.Client, sfSet *appsv1.StatefulSet, shardPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {
	var msg string
	var size int32 = 1
	//size = 1
	var isUpdate bool = false
	var err error
	var i int

	// Ensure deployment replicas match the desired state
	if sfSet.Spec.Replicas != nil {
		if *sfSet.Spec.Replicas != size {
			msg = "Current StatefulSet replicas do not match configured Shard Replicas. Shard is configured with only 1 but current replicas is set with " + strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
			LogMessages("DEBUG", msg, nil, instance, logger)
			isUpdate = true
		}
	}
	// Memory Check
	//resources := corev1.Pod.Spec.Containers
	for i = 0; i < len(shardPod.Spec.Containers); i++ {
		if shardPod.Spec.Containers[i].Name == sfSet.Name {
			shardContaineRes := shardPod.Spec.Containers[i].Resources
			oraSpexRes := OraShardSpex.Resources

			if !reflect.DeepEqual(shardContaineRes, oraSpexRes) {
				isUpdate = false
			}
		}
	}

	/**
	for i = 0; i < len(sfSet.Spec.VolumeClaimTemplates); i++ {
		if sfSet.Spec.VolumeClaimTemplates[i].Name == OraShardSpex.Name+"-oradata-vol4" {
			volResource := sfSet.Spec.VolumeClaimTemplates[i].Spec.Resources
			volumeSize := volResource.Requests.Storage()
			sSize := volumeSize.
			if sSize != int(OraShardSpex.StorageSizeInGb) {
				isUpdate = true
			}

		}
	}
	**/

	if isUpdate {
		err = kClient.Update(context.Background(), BuildStatefulSetForShard(instance, OraShardSpex))
		if err != nil {
			msg = "Failed to update Shard StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, nil, instance, logger)
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func ImportTDEKey(podName string, sparams string, instance *databasev4.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) error {
	var msg string

	msg = ""
	_, _, err := ExecCommand(podName, getImportTDEKeyCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg = "Error executing getImportTDEKeyCmd : podName=[" + podName + "]. errMsg=" + err.Error()
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}

	importArr := getImportTDEKeyCmd(sparams)
	importCmd := strings.Join(importArr, " ")
	msg = "Executed getImportTDEKeyCmd[" + importCmd + "] on pod " + podName
	LogMessages("INFO", msg, nil, instance, logger)
	return nil
}
