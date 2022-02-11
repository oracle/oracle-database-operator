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
	"reflect"
	"strconv"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildLabelsForCatalog(instance *databasev1alpha1.ShardingDatabase, label string) map[string]string {
	return map[string]string{
		"app":      "OracleSharding",
		"type":     "Catalog",
		"oralabel": getLabelForCatalog(instance),
	}
}

func getLabelForCatalog(instance *databasev1alpha1.ShardingDatabase) string {

	//  if len(OraCatalogSpex.Label) !=0 {
	//     return OraCatalogSpex.Label
	//   }

	return instance.Name
}

func BuildStatefulSetForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) *appsv1.StatefulSet {
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForCatalog(),
		ObjectMeta: builObjectMetaForCatalog(instance, OraCatalogSpex),
		Spec:       *buildStatefulSpecForCatalog(instance, OraCatalogSpex),
	}

	return sfset
}

// Function to build TypeMeta
func buildTypeMetaForCatalog() metav1.TypeMeta {
	// building TypeMeta
	typeMeta := metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}
	return typeMeta
}

// Function to build ObjectMeta
func builObjectMetaForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) metav1.ObjectMeta {
	// building objectMeta
	objmeta := metav1.ObjectMeta{
		Name:            OraCatalogSpex.Name,
		Namespace:       instance.Spec.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildLabelsForCatalog(instance, "sharding"),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) *appsv1.StatefulSetSpec {
	// building Stateful set Specs

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraCatalogSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForCatalog(instance, "sharding"),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildLabelsForCatalog(instance, "sharding"),
			},
			Spec: *buildPodSpecForCatalog(instance, OraCatalogSpex),
		},
		VolumeClaimTemplates: volumeClaimTemplatesForCatalog(instance, OraCatalogSpex),
	}
	//    if OraCatalogSpex.OraCatalogSize == 0  {
	//           OraCatalogSpex.OraCatalogSize = 1
	//           sfsetspec.Replicas = &OraCatalogSpex.OraCatalogSize
	//     } else {
	//           sfsetspec.Replicas = &OraCatalogSpex.OraCatalogSize
	//      }

	return sfsetspec
}

// Function to build PodSpec

func buildPodSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) *corev1.PodSpec {

	user := oraRunAsUser
	group := oraFsGroup
	spec := &corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: &user,
			FSGroup:   &group,
		},
		InitContainers: buildInitContainerSpecForCatalog(instance, OraCatalogSpex),
		Containers:     buildContainerSpecForCatalog(instance, OraCatalogSpex),
		Volumes:        buildVolumeSpecForCatalog(instance, OraCatalogSpex),
	}
	if len(instance.Spec.DbImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: instance.Spec.DbImagePullSecret,
			},
		}
	}

	if len(OraCatalogSpex.NodeSelector) > 0 {
		spec.NodeSelector = make(map[string]string)
		for key, value := range OraCatalogSpex.NodeSelector {
			spec.NodeSelector[key] = value
		}
	}
	return spec
}

// Function to build Volume Spec
func buildVolumeSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) []corev1.Volume {
	var result []corev1.Volume
	result = []corev1.Volume{
		{
			Name: OraCatalogSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.Secret,
				},
			},
		},
		{
			Name: OraCatalogSpex.Name + "orascript-vol5",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: OraCatalogSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if len(OraCatalogSpex.PvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "oradata-vol4", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: OraCatalogSpex.PvcName}}})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}

	return result
}

// Function to build the container Specification
func buildContainerSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	containerSpec := corev1.Container{
		Name:  OraCatalogSpex.Name,
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"NET_RAW"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: make(map[corev1.ResourceName]resource.Quantity),
		},
		VolumeMounts: buildVolumeMountSpecForCatalog(instance, OraCatalogSpex),
		LivenessProbe: &corev1.Probe{
			// TODO: Investigate if it's ok to call status every 10 seconds
			FailureThreshold:    int32(30),
			PeriodSeconds:       int32(240),
			InitialDelaySeconds: int32(300),
			TimeoutSeconds:      int32(60),
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("CATALOG"),
				},
			},
		},
		/**
		// Disabling this because the pod is not reachable till the time startup probe completes and without network pod configuration cannot be completed.
		StartupProbe: &corev1.Probe{
			// Initial delay should be big, because shard setup takes time
			FailureThreshold: int32(30),
			PeriodSeconds:    int32(120),
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: getLivenessCmd("CATALOG"),
				},
			},
		},
		**/
		Env: buildEnvVarsSpec(instance, OraCatalogSpex.EnvVars, OraCatalogSpex.Name, "CATALOG", false, ""),
	}
	if instance.Spec.IsClone {
		containerSpec.Command = []string{orainitCmd3}
	}

	if OraCatalogSpex.Resources != nil {
		containerSpec.Resources = *OraCatalogSpex.Resources
	}
	// building Complete Container Spec
	result = []corev1.Container{
		containerSpec,
	}
	return result
}

//Function to build the init Container Spec
func buildInitContainerSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) []corev1.Container {
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
		Name:  OraCatalogSpex.Name + "-init1",
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
		VolumeMounts: buildVolumeMountSpecForCatalog(instance, OraCatalogSpex),
	}

	// building Complete Init Container Spec
	if OraCatalogSpex.ImagePulllPolicy != nil {
		init1spec.ImagePullPolicy = *OraCatalogSpex.ImagePulllPolicy
	}
	result = []corev1.Container{
		init1spec,
	}
	return result
}

func buildVolumeMountSpecForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "secretmap-vol3", MountPath: oraSecretMount, ReadOnly: true})
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "-oradata-vol4", MountPath: oraDataMount})
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "orascript-vol5", MountPath: oraScriptMount})
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	return result
}

func volumeClaimTemplatesForCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec) []corev1.PersistentVolumeClaim {

	var claims []corev1.PersistentVolumeClaim

	if len(OraCatalogSpex.PvcName) != 0 {
		return claims
	}

	claims = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            OraCatalogSpex.Name + "-oradata-vol4",
				Namespace:       instance.Spec.Namespace,
				OwnerReferences: getOwnerRef(instance),
				Labels:          buildLabelsForCatalog(instance, "sharding"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(OraCatalogSpex.StorageSizeInGb), 10) + "Gi"),
					},
				},
			},
		},
	}

	if len(OraCatalogSpex.PvAnnotations) > 0 {
		claims[0].ObjectMeta.Annotations = make(map[string]string)
		for key, value := range OraCatalogSpex.PvAnnotations {
			claims[0].ObjectMeta.Annotations[key] = value
		}
	}

	if len(OraCatalogSpex.PvMatchLabels) > 0 {
		claims[0].Spec.Selector = &metav1.LabelSelector{MatchLabels: OraCatalogSpex.PvMatchLabels}
	}

	return claims
}

func BuildServiceDefForCatalog(instance *databasev1alpha1.ShardingDatabase, replicaCount int32, OraCatalogSpex databasev1alpha1.CatalogSpec, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForCatalog(instance, replicaCount, OraCatalogSpex, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// Check if user want External Svc on each replica pod
	if svctype == "external" {
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Spec.Selector = getSvcLabelsForCatalog(replicaCount, OraCatalogSpex)
	}

	if svctype == "local" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = buildLabelsForCatalog(instance, "sharding")
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.
	service.Spec.Ports = buildSvcPortsDef(instance, "CATALOG")
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForCatalog(instance *databasev1alpha1.ShardingDatabase, replicaCount int32, OraCatalogSpex databasev1alpha1.CatalogSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	var svcName string
	if svctype == "local" {
		svcName = OraCatalogSpex.Name
	}

	if svctype == "external" {
		svcName = OraCatalogSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Spec.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildLabelsForCatalog(instance, "sharding"),
	}
	return objmeta
}

func getSvcLabelsForCatalog(replicaCount int32, OraCatalogSpex databasev1alpha1.CatalogSpec) map[string]string {

	var labelStr map[string]string = make(map[string]string)
	if replicaCount == -1 {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraCatalogSpex.Name + "-0"
	} else {
		labelStr["statefulset.kubernetes.io/pod-name"] = OraCatalogSpex.Name + "-" + strconv.FormatInt(int64(replicaCount), 10)
	}

	//  fmt.Println("Service Selector String Specification", labelStr)
	return labelStr
}

// ======================== update Section ========================
func UpdateProvForCatalog(instance *databasev1alpha1.ShardingDatabase,
	OraCatalogSpex databasev1alpha1.CatalogSpec, kClient client.Client, sfSet *appsv1.StatefulSet, catalogPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {

	var isUpdate bool = false
	var err error
	var i int
	var msg string

	//msg = "Inside the updateProvForCatalog"
	//reqLogger := r.Log.WithValues("Instance.Namespace", instance.Spec.Namespace, "Instance.Name", instance.Name)
	LogMessages("DEBUG", msg, nil, instance, logger)

	// Memory Check
	//resources := corev1.Pod.Spec.Containers
	for i = 0; i < len(catalogPod.Spec.Containers); i++ {
		if catalogPod.Spec.Containers[i].Name == sfSet.Name {
			shardContaineRes := catalogPod.Spec.Containers[i].Resources
			oraSpexRes := OraCatalogSpex.Resources

			if !reflect.DeepEqual(shardContaineRes, oraSpexRes) {
				isUpdate = true
			}
		}
	}

	/**

	for i = 0; i < len(sfSet.Spec.VolumeClaimTemplates); i++ {
		if sfSet.Spec.VolumeClaimTemplates[i].Name == OraCatalogSpex.Name+"-oradata-vol4" {
			volumeSize := sfSet.Spec.VolumeClaimTemplates[i].Size()
			if volumeSize != int(OraCatalogSpex.StorageSizeInGb) {
				isUpdate = true
			}

		}
	}
	**/

	if isUpdate {
		err = kClient.Update(context.Background(), BuildStatefulSetForCatalog(instance, OraCatalogSpex))
		if err != nil {
			msg = "Failed to update Catalog StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, nil, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}
