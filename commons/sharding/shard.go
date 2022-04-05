/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildLabelsForShard(instance *databasev1alpha1.ShardingDatabase, label string) map[string]string {
	return map[string]string{
		"app":      "OracleSharding",
		"type":     "Shard",
		"oralabel": getLabelForShard(instance),
	}
}

func getLabelForShard(instance *databasev1alpha1.ShardingDatabase) string {

	//  if len(OraShardSpex.Label) !=0 {
	//     return OraShardSpex.Label
	//   }

	return instance.Name
}

func BuildStatefulSetForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) *appsv1.StatefulSet {
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
func builObjectMetaForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:            OraShardSpex.Name,
		Namespace:       instance.Spec.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildLabelsForShard(instance, "sharding"),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) *appsv1.StatefulSetSpec {
	// building Stateful set Specs
	var size int32
	size = 1
	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraShardSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForShard(instance, "sharding"),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildLabelsForShard(instance, "sharding"),
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

func buildPodSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) *corev1.PodSpec {

	user := oraRunAsUser
	group := oraFsGroup
	spec := &corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: &user,
			FSGroup:   &group,
		},
		InitContainers: buildInitContainerSpecForShard(instance, OraShardSpex),
		Containers:     buildContainerSpecForShard(instance, OraShardSpex),
		Volumes:        buildVolumeSpecForShard(instance, OraShardSpex),
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
func buildVolumeSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: OraShardSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.Secret,
				},
			},
		},
		{
			Name: OraShardSpex.Name + "orascript-vol5",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: OraShardSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if len(OraShardSpex.PvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "oradata-vol4", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: OraShardSpex.PvcName}}})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraShardSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}

	return result
}

// Function to build the container Specification
func buildContainerSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	containerSpec := corev1.Container{
		Name:  OraShardSpex.Name,
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"NET_RAW"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForShard(instance, OraShardSpex),
		LivenessProbe: &corev1.Probe{
			// TODO: Investigate if it's ok to call status every 10 seconds
			FailureThreshold:    int32(30),
			PeriodSeconds:       int32(240),
			InitialDelaySeconds: int32(300),
			TimeoutSeconds:      int32(120),
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("SHARD"),
				},
			},
		},
		/**
		// Disabling this because ping stop working and sharding topologu never gets configured.
		StartupProbe: &corev1.Probe{
			FailureThreshold: int32(30),
			PeriodSeconds:    int32(180),
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("SHARD"),
				},
			},
		},
		**/
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

//Function to build the init Container Spec
func buildInitContainerSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) []corev1.Container {
	var result []corev1.Container
	privFlag := true
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
			Privileged: &privFlag,
			RunAsUser:  &uid,
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

func buildVolumeMountSpecForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "secretmap-vol3", MountPath: oraSecretMount, ReadOnly: true})
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "-oradata-vol4", MountPath: oraDataMount})
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orascript-vol5", MountPath: oraScriptMount})
	result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraShardSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	return result
}

func volumeClaimTemplatesForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec) []corev1.PersistentVolumeClaim {

	var claims []corev1.PersistentVolumeClaim

	if len(OraShardSpex.PvcName) != 0 {
		return claims
	}

	claims = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            OraShardSpex.Name + "-oradata-vol4",
				Namespace:       instance.Spec.Namespace,
				OwnerReferences: getOwnerRef(instance),
				Labels:          buildLabelsForShard(instance, "sharding"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.ResourceRequirements{
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

func BuildServiceDefForShard(instance *databasev1alpha1.ShardingDatabase, replicaCount int32, OraShardSpex databasev1alpha1.ShardSpec, svctype string) *corev1.Service {
	service := &corev1.Service{}
	service = &corev1.Service{
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
		service.Spec.Selector = buildLabelsForShard(instance, "sharding")
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.
	service.Spec.Ports = buildSvcPortsDef(instance, "SHARD")
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForShard(instance *databasev1alpha1.ShardingDatabase, replicaCount int32, OraShardSpex databasev1alpha1.ShardSpec, svctype string) metav1.ObjectMeta {
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
		Namespace:       instance.Spec.Namespace,
		Labels:          buildLabelsForShard(instance, "sharding"),
		OwnerReferences: getOwnerRef(instance),
	}
	return objmeta
}

func getSvcLabelsForShard(replicaCount int32, OraShardSpex databasev1alpha1.ShardSpec) map[string]string {

	var labelStr map[string]string
	labelStr = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraShardSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10)
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// ======================== Update Section ========================
func UpdateProvForShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec, kClient client.Client, sfSet *appsv1.StatefulSet, shardPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {
	var msg string
	var size int32
	size = 1
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
				isUpdate = true
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
