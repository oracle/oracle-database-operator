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
	"reflect"
	"strconv"

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
	return map[string]string{
		"app":        "OracleGsming",
		"shard_name": "Gsm",
		"oralabel":   getLabelForGsm(instance),
	}
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
		ObjectMeta: builObjectMetaForGsm(instance, OraGsmSpex),
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
func builObjectMetaForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:            OraGsmSpex.Name,
		Namespace:       instance.Namespace,
		Labels:          buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
		OwnerReferences: getOwnerRef(instance),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) *appsv1.StatefulSetSpec {
	// building Stateful set Specs

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraGsmSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
			},
			Spec: *buildPodSpecForGsm(instance, OraGsmSpex),
		},
		VolumeClaimTemplates: volumeClaimTemplatesForGsm(instance, OraGsmSpex),
	}
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
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: &user,
			FSGroup:   &group,
		},
		Containers: buildContainerSpecForGsm(instance, OraGsmSpex),
		Volumes:    buildVolumeSpecForGsm(instance, OraGsmSpex),
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

	if len(OraGsmSpex.PvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraGsmSpex.Name + "oradata-vol4", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: OraGsmSpex.PvcName}}})
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
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"NET_RAW"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForGsm(instance, OraGsmSpex),
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
			TimeoutSeconds: int32(20),
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("GSM"),
				},
			},
		},
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
		Env: buildEnvVarsSpec(instance, OraGsmSpex.EnvVars, OraGsmSpex.Name, "GSM", masterGsmFlag, directorParams),
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
	var uid int64 = 0
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
			Privileged: &privFlag,
			RunAsUser:  &uid,
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
	var result []corev1.VolumeMount
	result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "secretmap-vol3", MountPath: oraSecretMount, ReadOnly: true})
	result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "-oradata-vol4", MountPath: oraGsmDataMount})
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "orascript-vol5", MountPath: oraScriptMount})
	}
	result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraGsmSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	return result
}

func volumeClaimTemplatesForGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec) []corev1.PersistentVolumeClaim {

	var claims []corev1.PersistentVolumeClaim

	if len(OraGsmSpex.PvcName) != 0 {
		return claims
	}

	claims = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            OraGsmSpex.Name + "-oradata-vol4",
				Namespace:       instance.Namespace,
				Labels:          buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
				OwnerReferences: getOwnerRef(instance),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(OraGsmSpex.StorageSizeInGb), 10) + "Gi"),
					},
				},
			},
		},
	}

	if len(OraGsmSpex.PvAnnotations) > 0 {
		claims[0].ObjectMeta.Annotations = make(map[string]string)
		for key, value := range OraGsmSpex.PvAnnotations {
			claims[0].ObjectMeta.Annotations[key] = value
		}
	}

	if len(OraGsmSpex.PvMatchLabels) > 0 {
		claims[0].Spec.Selector = &metav1.LabelSelector{MatchLabels: OraGsmSpex.PvMatchLabels}
	}

	return claims
}

func BuildServiceDefForGsm(instance *databasev4.ShardingDatabase, replicaCount int32, OraGsmSpex databasev4.GsmSpec, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForGsm(instance, replicaCount, OraGsmSpex, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// Check if user want External Svc on each replica pod
	if svctype == "external" {
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Spec.Selector = getSvcLabelsForGsm(replicaCount, OraGsmSpex)
	}

	if svctype == "local" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = getSvcLabelsForGsm(replicaCount, OraGsmSpex)
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.
	service.Spec.Ports = buildSvcPortsDef(instance, "GSM")
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForGsm(instance *databasev4.ShardingDatabase, replicaCount int32, OraGsmSpex databasev4.GsmSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	var svcName string
	if svctype == "local" {
		svcName = OraGsmSpex.Name
	}

	if svctype == "external" {
		svcName = OraGsmSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Namespace,
		Labels:          buildLabelsForGsm(instance, "sharding", OraGsmSpex.Name),
		OwnerReferences: getOwnerRef(instance),
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

	var msg string
	var size int32 = 1
	var isUpdate bool = false
	var err error
	var i int

	msg = "Inside the updateProvForGsm"
	LogMessages("DEBUG", msg, nil, instance, logger)

	// Ensure deployment replicas match the desired state

	// Ensure deployment replicas match the desired state
	if sfSet.Spec.Replicas != nil {
		if *sfSet.Spec.Replicas != size {
			msg = "Current StatefulSet replicas do not match configured GSM Replicas. Gsm is configured with only 1 but current replicas is set with " + strconv.FormatInt(int64(*sfSet.Spec.Replicas), 10)
			LogMessages("DEBUG", msg, nil, instance, logger)
			isUpdate = true
		}
	}
	// Memory Check
	//resources := corev1.Pod.Spec.Containers
	for i = 0; i < len(gsmPod.Spec.Containers); i++ {
		if gsmPod.Spec.Containers[i].Name == sfSet.Name {
			shardContaineRes := gsmPod.Spec.Containers[i].Resources
			oraSpexRes := OraGsmSpex.Resources

			if !reflect.DeepEqual(shardContaineRes, oraSpexRes) {
				isUpdate = false
			}
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

	if isUpdate {
		err = kClient.Update(context.Background(), BuildStatefulSetForGsm(instance, OraGsmSpex))
		if err != nil {
			msg = "Failed to update Shard StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, err, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}
