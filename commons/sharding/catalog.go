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
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildLabelsForCatalog(instance *databasev4.ShardingDatabase, label string, catalogName string) map[string]string {
	// Keep selector labels stable to avoid StatefulSet selector immutability issues.
	return map[string]string{
		"app":      "OracleSharding",
		"type":     "Catalog",
		"oralabel": getLabelForCatalog(instance),
	}
}

func buildOwnerRefForCatalog(instance *databasev4.ShardingDatabase) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(instance, databasev4.GroupVersion.WithKind("ShardingDatabase")),
	}
}

func buildResourceLabelsForCatalog(instance *databasev4.ShardingDatabase, label string, catalogName string) map[string]string {
	labels := buildLabelsForCatalog(instance, label, catalogName)
	labels["app.kubernetes.io/name"] = "oracle-sharding"
	labels["app.kubernetes.io/instance"] = instance.Name
	labels["app.kubernetes.io/component"] = "catalog"
	labels["app.kubernetes.io/managed-by"] = "oracle-database-operator"
	labels["app.kubernetes.io/part-of"] = "oracle-database"
	labels["sharding.oracle.com/database"] = instance.Name
	labels["sharding.oracle.com/catalog"] = catalogName
	if label != "" {
		labels["sharding.oracle.com/kind"] = label
	}
	return labels
}

func getLabelForCatalog(instance *databasev4.ShardingDatabase) string {

	//  if len(OraCatalogSpex.Label) !=0 {
	//     return OraCatalogSpex.Label
	//   }

	return instance.Name
}

func BuildStatefulSetForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) (*appsv1.StatefulSet, error) {
	spec, err := buildStatefulSpecForCatalog(instance, OraCatalogSpex)
	if err != nil {
		return nil, err
	}
	sfset := &appsv1.StatefulSet{
		TypeMeta:   buildTypeMetaForCatalog(),
		ObjectMeta: buildObjectMetaForCatalog(instance, OraCatalogSpex),
		Spec:       *spec,
	}

	return sfset, nil
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
func buildObjectMetaForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) metav1.ObjectMeta {
	objmeta := metav1.ObjectMeta{
		Name:            OraCatalogSpex.Name,
		Namespace:       instance.Namespace,
		OwnerReferences: buildOwnerRefForCatalog(instance),
		Labels:          buildResourceLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name),
	}
	return objmeta
}

// Function to build Stateful Specs
func buildStatefulSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) (*appsv1.StatefulSetSpec, error) {
	// building Stateful set Specs
	podSpec, err := buildPodSpecForCatalog(instance, OraCatalogSpex)
	if err != nil {
		return nil, err
	}
	replicas := shardReplicaCount

	sfsetspec := &appsv1.StatefulSetSpec{
		ServiceName: OraCatalogSpex.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildResourceLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name),
			},
			Spec: *podSpec,
		},
		VolumeClaimTemplates: volumeClaimTemplatesForCatalog(instance, OraCatalogSpex),
	}
	sfsetspec.Replicas = &replicas
	//    if OraCatalogSpex.OraCatalogSize == 0  {
	//           OraCatalogSpex.OraCatalogSize = 1
	//           sfsetspec.Replicas = &OraCatalogSpex.OraCatalogSize
	//     } else {
	//           sfsetspec.Replicas = &OraCatalogSpex.OraCatalogSize
	//      }

	return sfsetspec, nil
}

// Function to build PodSpec

func buildPodSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) (*corev1.PodSpec, error) {

	user := oraRunAsUser
	group := oraFsGroup
	podSecurityContext := mergePodSecurityContextWithDefaults(&corev1.PodSecurityContext{
		RunAsNonRoot: BoolPointer(true),
		RunAsUser:    &user,
		RunAsGroup:   &group,
		FSGroup:      &group,
	}, OraCatalogSpex.SecurityContext)
	podSecurityContext, err := applyOracleMemorySysctls(podSecurityContext, OraCatalogSpex.Resources, OraCatalogSpex.EnvVars)
	if err != nil {
		return nil, err
	}
	spec := &corev1.PodSpec{
		SecurityContext:    podSecurityContext,
		HostAliases:        cloneHostAliases(instance.Spec.HostAliases),
		Containers:         buildContainerSpecForCatalog(instance, OraCatalogSpex),
		Volumes:            buildVolumeSpecForCatalog(instance, OraCatalogSpex),
		ServiceAccountName: instance.Spec.SrvAccountName,
	}

	if (instance.Spec.IsDownloadScripts) && (instance.Spec.ScriptsLocation != "") {
		spec.InitContainers = buildInitContainerSpecForCatalog(instance, OraCatalogSpex)
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
	return spec, nil
}

// Function to build Volume Spec
func buildVolumeSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) []corev1.Volume {
	var result []corev1.Volume
	dshmSizeLimit := resource.MustParse("4Gi")
	pvcMounts := normalizePVCMountConfigs(OraCatalogSpex.Name, OraCatalogSpex.StorageSizeInGb, instance.Spec.StorageClass, OraCatalogSpex.DisableDefaultLogVolumeClaims, OraCatalogSpex.AdditionalPVCs)
	result = []corev1.Volume{
		{
			Name: OraCatalogSpex.Name + "secretmap-vol3",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.DbSecret.Name,
				},
			},
		},
		{
			Name: OraCatalogSpex.Name + "oradshm-vol6",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: &dshmSizeLimit,
				},
			},
		},
	}

	if OraCatalogSpex.CatalogConfigData != nil && len(OraCatalogSpex.CatalogConfigData.Name) != 0 {
		result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "-oradata-configdata", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: OraCatalogSpex.CatalogConfigData.Name}}}})
	}

	for _, pvcMount := range pvcMounts {
		if pvcMount.pvcName == "" {
			continue
		}
		result = append(result, corev1.Volume{
			Name:         pvcMount.volumeName,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcMount.pvcName}},
		})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "orastage-vol7", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Spec.StagePvcName}}})
	}

	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "orascript-vol5", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}

	if checkTdeWalletFlag(instance) {
		if walletPVC := getTdeWalletPVCName(instance); len(instance.Spec.FssStorageClass) == 0 && walletPVC != "" {
			result = append(result, corev1.Volume{Name: OraCatalogSpex.Name + "shared-storage-vol8", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: walletPVC}}})
		}
	}

	return result
}

// Function to build the container Specification
func buildContainerSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) []corev1.Container {
	// building Continer spec
	var result []corev1.Container
	user := oraRunAsUser
	group := oraFsGroup
	dbCheckCmd := "if [ -f $ORACLE_BASE/checkDBLockStatus.sh ]; then $ORACLE_BASE/checkDBLockStatus.sh ; else $ORACLE_BASE/checkDBStatus.sh; fi "
	containerSpec := corev1.Container{
		Name:  OraCatalogSpex.Name,
		Image: instance.Spec.DbImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             BoolPointer(true),
			RunAsUser:                &user,
			RunAsGroup:               &group,
			AllowPrivilegeEscalation: BoolPointer(false),
			Capabilities: mergeCapabilitiesWithDefaults(&corev1.Capabilities{
				Add:  []corev1.Capability{corev1.Capability("NET_ADMIN"), corev1.Capability("SYS_NICE")},
				Drop: []corev1.Capability{"ALL"},
			}, OraCatalogSpex.Capabilities),
		},
		VolumeMounts: buildVolumeMountSpecForCatalog(instance, OraCatalogSpex),
		LivenessProbe: buildShellExecProbe(
			dbCheckCmd,
			30,
			func() int32 {
				if instance.Spec.LivenessCheckPeriod > 0 {
					return int32(instance.Spec.LivenessCheckPeriod)
				}
				return 60
			}(),
			30,
			3,
		),
		/**
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					//Command: getReadinessCmd("CATALOG"),
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
		StartupProbe: buildShellExecProbe(dbCheckCmd, 30, 40, 0, 120),
		Env:          buildEnvVarsSpec(instance, OraCatalogSpex.EnvVars, OraCatalogSpex.Name, "CATALOG", false, "NONE", "", nil),
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

// Function to build the init Container Spec
func buildInitContainerSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) []corev1.Container {
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
		Name:  OraCatalogSpex.Name + "-init1",
		Image: instance.Spec.DbImage,
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

func buildVolumeMountSpecForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) []corev1.VolumeMount {
	result := make([]corev1.VolumeMount, 0, 8)
	pvcMounts := normalizePVCMountConfigs(OraCatalogSpex.Name, OraCatalogSpex.StorageSizeInGb, instance.Spec.StorageClass, OraCatalogSpex.DisableDefaultLogVolumeClaims, OraCatalogSpex.AdditionalPVCs)
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "secretmap-vol3", MountPath: getDbSecretMountPath(instance), ReadOnly: true})
	for _, pvcMount := range pvcMounts {
		result = append(result, corev1.VolumeMount{Name: pvcMount.volumeName, MountPath: pvcMount.mountPath})
	}
	if instance.Spec.IsDownloadScripts {
		result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "orascript-vol5", MountPath: oraDbScriptMount})
	}
	result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "oradshm-vol6", MountPath: oraShm})

	if OraCatalogSpex.CatalogConfigData != nil && len(OraCatalogSpex.CatalogConfigData.Name) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "-oradata-configdata", MountPath: OraCatalogSpex.CatalogConfigData.MountPath})
	}

	if len(instance.Spec.StagePvcName) != 0 {
		result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "orastage-vol7", MountPath: oraStage})
	}

	if checkTdeWalletFlag(instance) {
		walletPVC := getTdeWalletPVCName(instance)
		if len(instance.Spec.FssStorageClass) > 0 && walletPVC == "" {
			result = append(result, corev1.VolumeMount{Name: instance.Name + "shared-storage", MountPath: getTdeWalletMountLoc(instance)})
		} else if len(instance.Spec.FssStorageClass) == 0 && walletPVC != "" {
			result = append(result, corev1.VolumeMount{Name: OraCatalogSpex.Name + "shared-storage-vol8", MountPath: getTdeWalletMountLoc(instance)})
		}
	}
	return result
}

func volumeClaimTemplatesForCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec) []corev1.PersistentVolumeClaim {
	pvcMounts := normalizePVCMountConfigs(OraCatalogSpex.Name, OraCatalogSpex.StorageSizeInGb, instance.Spec.StorageClass, OraCatalogSpex.DisableDefaultLogVolumeClaims, OraCatalogSpex.AdditionalPVCs)
	claims := make([]corev1.PersistentVolumeClaim, 0, len(pvcMounts)+1)
	for _, pvcMount := range pvcMounts {
		if strings.TrimSpace(pvcMount.pvcName) != "" {
			continue
		}
		claim := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:            pvcMount.volumeName,
				Namespace:       instance.Namespace,
				OwnerReferences: buildOwnerRefForCatalog(instance),
				Labels:          buildResourceLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name),
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

	if checkTdeWalletFlag(instance) {
		if walletPVC := getTdeWalletPVCName(instance); len(instance.Spec.FssStorageClass) > 0 && walletPVC == "" {
			claims = append(claims, corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:            instance.Name + "shared-storage",
					Namespace:       instance.Namespace,
					OwnerReferences: buildOwnerRefForCatalog(instance),
					Labels:          buildResourceLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteMany,
					},
					StorageClassName: storageClassNamePtr(instance.Spec.FssStorageClass),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(OraCatalogSpex.StorageSizeInGb), 10) + "Gi"),
						},
					},
				},
			})
		}
	}

	return claims
}

func BuildServiceDefForCatalog(instance *databasev4.ShardingDatabase, replicaCount int32, OraCatalogSpex databasev4.CatalogSpec, svctype string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForCatalog(instance, replicaCount, OraCatalogSpex, svctype),
		Spec: corev1.ServiceSpec{
			Selector: getSvcLabelsForCatalog(replicaCount, OraCatalogSpex),
			Ports:    buildSvcPortsDef(instance, "CATALOG"),
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
func buildSvcObjectMetaForCatalog(instance *databasev4.ShardingDatabase, replicaCount int32, OraCatalogSpex databasev4.CatalogSpec, svctype string) metav1.ObjectMeta {
	// building objectMeta
	var svcName string
	if svctype == shardServiceTypeLocal {
		svcName = OraCatalogSpex.Name
	}

	if svctype == shardServiceTypeExternal {
		svcName = OraCatalogSpex.Name + strconv.FormatInt(int64(replicaCount), 10) + "-svc"
	}
	labels := buildResourceLabelsForCatalog(instance, "sharding", OraCatalogSpex.Name)
	labels["sharding.oracle.com/service-type"] = svctype

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Namespace,
		OwnerReferences: buildOwnerRefForCatalog(instance),
		Labels:          labels,
		Annotations:     resolveCatalogServiceAnnotations(instance, OraCatalogSpex, svctype),
	}
	return objmeta
}

func getSvcLabelsForCatalog(replicaCount int32, OraCatalogSpex databasev4.CatalogSpec) map[string]string {

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
func UpdateProvForCatalog(instance *databasev4.ShardingDatabase,
	OraCatalogSpex databasev4.CatalogSpec, kClient client.Client, sfSet *appsv1.StatefulSet, catalogPod *corev1.Pod, logger logr.Logger,
) (ctrl.Result, error) {
	_ = catalogPod

	requiresUpdate := false
	var msg string

	//msg = "Inside the updateProvForCatalog"
	//reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	LogMessages("DEBUG", msg, nil, instance, logger)

	if sfSet.Spec.Replicas == nil || *sfSet.Spec.Replicas != shardReplicaCount {
		requiresUpdate = true
	}

	if OraCatalogSpex.Resources != nil {
		for i := range sfSet.Spec.Template.Spec.Containers {
			if sfSet.Spec.Template.Spec.Containers[i].Name != OraCatalogSpex.Name {
				continue
			}
			if sfSet.Spec.Template.Spec.Containers[i].Resources.String() != OraCatalogSpex.Resources.String() {
				requiresUpdate = true
			}
			break
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

	if requiresUpdate {
		desired, err := BuildStatefulSetForCatalog(instance, OraCatalogSpex)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = kClient.Update(context.Background(), desired)
		if err != nil {
			msg = "Failed to update Catalog StatefulSet " + "StatefulSet.Name : " + sfSet.Name
			LogMessages("Error", msg, nil, instance, logger)
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

func ExportTDEKey(podName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger) error {
	var msg string

	msg = ""
	_, _, err := ExecCommand(podName, getExportTDEKeyCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg = "Error executing getExportTDEKeyCmd : podName=[" + podName + "]. errMsg=" + err.Error()
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}
