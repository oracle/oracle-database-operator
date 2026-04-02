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

// Package commons provides shared RAC helper utilities used by the controller
// to build Kubernetes resources. The helpers align with docs/rac guidance and
// Kubernetes API conventions referenced in the quickstart manifests.
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
package commons

import (
	"context"
	"fmt"
	"strconv"
	"time"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedasm "github.com/oracle/oracle-database-operator/commons/crs/asm"
	utils "github.com/oracle/oracle-database-operator/commons/crs/rac/utils"
	shareddiskcheck "github.com/oracle/oracle-database-operator/commons/crs/shared/diskcheck"
	sharedinstanceutil "github.com/oracle/oracle-database-operator/commons/crs/shared/instanceutil"
	sharednaming "github.com/oracle/oracle-database-operator/commons/crs/shared/naming"
	sharednetutil "github.com/oracle/oracle-database-operator/commons/crs/shared/netutil"
	sharedoracmd "github.com/oracle/oracle-database-operator/commons/crs/shared/oracmd"
	sharedrsp "github.com/oracle/oracle-database-operator/commons/crs/shared/rsp"
	sharedk8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"

	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildEnvVarsSpec returns a copy of the provided env vars slice for container configuration.
func buildEnvVarsSpec(envVariables []corev1.EnvVar) []corev1.EnvVar {
	var result []corev1.EnvVar

	/**
		if len(envVariables) > 0 {
			result = append(result, corev1.EnvVar{Name: "container", Value: "true"})
		} else {
			result = append(result, corev1.EnvVar{Name: "container", Value: "true"})
		}
	**/

	return result
}

// buildRacSvcPortsDef converts node port service mappings into Kubernetes service ports.
func buildRacSvcPortsDef(npsvc racdb.RacNodePortSvc) []corev1.ServicePort {
	var result []corev1.ServicePort

	for _, portMapping := range npsvc.PortMappings {
		servicePort :=
			corev1.ServicePort{
				Protocol: portMapping.Protocol,
				Port:     portMapping.Port,
				Name:     generatePortMapping(portMapping),
			}
		if portMapping.TargetPort > 0 {
			servicePort.TargetPort = intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: portMapping.TargetPort,
			}
		}
		if portMapping.NodePort > 0 {
			servicePort.NodePort = portMapping.NodePort
		}

		result = append(result, servicePort)
	}

	return result
}

// buildRacSvcPortsDefForCluster builds service ports using the provided cluster port list.
func buildRacSvcPortsDefForCluster(portList []struct {
	Protocol                   corev1.Protocol
	Port, TargetPort, NodePort int32
	Name                       string
}) []corev1.ServicePort {
	var result []corev1.ServicePort
	for _, p := range portList {
		sp := corev1.ServicePort{
			Protocol: p.Protocol,
			Port:     p.Port,
			Name:     p.Name,
		}
		if p.TargetPort > 0 {
			sp.TargetPort = intstr.FromInt(int(p.TargetPort))
		}
		if p.NodePort > 0 {
			sp.NodePort = p.NodePort
		}
		result = append(result, sp)
	}
	return result
}

// generateName creates a random suffix based name capped to Kubernetes limits.
func generateName(base string) string {
	maxNameLength := 50
	randomLength := 5
	maxGeneratedLength := maxNameLength - randomLength
	if len(base) > maxGeneratedLength {
		base = base[:maxGeneratedLength]
	}
	return fmt.Sprintf("%s%s", base, rand.String(randomLength))
}

// generatePortMapping produces a unique name for a port mapping entry.
func generatePortMapping(portMapping racdb.RacPortMapping) string {
	return generateName(fmt.Sprintf("%s-%d-%d-", "tcp",
		portMapping.Port, portMapping.TargetPort))
}

// LogMessages logs information or debug messages based on the instance debug status.
func LogMessages(msgtype string, msg string, err error, instance *racdb.RacDatabase, logger logr.Logger) {
	// setting logrus formatter
	//logrus.SetFormatter(&logrus.JSONFormatter{})
	//logrus.SetOutput(os.Stdout)

	if msgtype == "DEBUG" && utils.CheckStatusFlag(instance.Spec.IsDebug) {
		if err != nil {
			logger.Error(err, msg)
		} else {
			logger.Info(msg)
		}
	} else if msgtype == "INFO" {
		logger.Info(msg)
	}
}

// GetRacPodName returns the pod name for the provided RAC name.
func GetRacPodName(racName string) string {
	return racName
}

// BuildLabelsForRac returns stable labels shared by RAC cluster resources.
func BuildLabelsForRac(instance *racdb.RacDatabase, component string) map[string]string {
	return map[string]string{
		"app":                            "oracle-database-operator",
		"database.oracle.com/name":       instance.Name,
		"database.oracle.com/component":  component,
		"database.oracle.com/managed-by": "racdatabase-controller",
	}
}

// getlabelsForRac builds common RAC labels for internal operations.
func getlabelsForRac(instance *racdb.RacDatabase) map[string]string {
	return buildLabelsForRac(instance, "RAC")
}

func buildLabelsForRac(instance *racdb.RacDatabase, component string) map[string]string {
	return BuildLabelsForRac(instance, component)
}

// BuildLabelsForDaemonSet prepares labels for RAC daemonset resources.
func BuildLabelsForDaemonSet(instance *racdb.RacDatabase, label string) map[string]string {
	return shareddiskcheck.BuildLabelsForDaemonSet(instance, label)
}

func buildLabelsForAsmPv(instance *racdb.RacDatabase, diskName string) map[string]string {
	baseName := diskName[strings.LastIndex(diskName, "/")+1:]
	asmVol := "block-asm-pv-" + instance.Name + "-" + baseName
	return map[string]string{
		"asm_vol":                      asmVol,
		"app.kubernetes.io/name":       "oracle-rac",
		"app.kubernetes.io/instance":   sanitizeK8sName(instance.Name),
		"app.kubernetes.io/component":  "asm-pv",
		"app.kubernetes.io/managed-by": "oracle-database-operator",
	}
}

func getAsmNodeAffinity(instance *racdb.RacDatabase) *corev1.VolumeNodeAffinity {
	nodeAffinity := &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{},
		},
	}
	if instance.Spec.ClusterDetails == nil || len(instance.Spec.ClusterDetails.WorkerNodeSelector) == 0 {
		return nodeAffinity
	}

	var matchExpr []corev1.NodeSelectorRequirement
	for key, value := range instance.Spec.ClusterDetails.WorkerNodeSelector {
		matchExpr = append(matchExpr, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{value},
		})
	}
	nodeAffinity.Required.NodeSelectorTerms = []corev1.NodeSelectorTerm{{MatchExpressions: matchExpr}}
	return nodeAffinity
}

func flattenAsmDisks(racDbSpec *racdb.RacDatabaseSpec) []string {
	var groups [][]string
	for _, dg := range racDbSpec.AsmStorageDetails {
		groups = append(groups, dg.Disks)
	}
	return sharedasm.FlattenUniqueDiskGroups(groups)
}

func VolumePVForASM(
	instance *racdb.RacDatabase,
	dgIndex, diskIdx int,
	diskName, diskGroupName, size string,
) *corev1.PersistentVolume {
	_ = dgIndex
	_ = diskIdx
	_ = diskGroupName
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
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			VolumeMode:  &volumeBlock,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}
	if len(instance.Spec.StorageClass) == 0 {
		asmPv.Spec.NodeAffinity = getAsmNodeAffinity(instance)
		asmPv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			Local: &corev1.LocalVolumeSource{Path: diskName},
		}
	} else {
		asmPv.Spec.StorageClassName = instance.Spec.StorageClass
	}
	return asmPv
}

func VolumePVCForASM(
	instance *racdb.RacDatabase,
	dgIndex, diskIdx int,
	diskName, diskGroupName, size string,
) *corev1.PersistentVolumeClaim {
	_ = dgIndex
	_ = diskIdx
	_ = diskGroupName
	volumeBlock := corev1.PersistentVolumeBlock
	asmPvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAsmPvcName(diskName, instance.Name),
			Namespace: instance.Namespace,
			Labels:    buildLabelsForAsmPv(instance, diskName),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			VolumeMode:  &volumeBlock,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}
	if len(instance.Spec.StorageClass) == 0 {
		asmPvc.Spec.Selector = &metav1.LabelSelector{MatchLabels: buildLabelsForAsmPv(instance, diskName)}
	} else {
		asmPvc.Spec.StorageClassName = &instance.Spec.StorageClass
	}
	return asmPvc
}

func ConfigMapSpecs(instance *racdb.RacDatabase, cmData map[string]string, cmName string) *corev1.ConfigMap {
	return sharedk8sobjects.ConfigMapSpec(instance.Namespace, instance.Name, cmName, cmData)
}

func BuildDiskCheckDaemonSet(racDatabase *racdb.RacDatabase) *appsv1.DaemonSet {
	labels := BuildLabelsForDaemonSet(racDatabase, "disk-check")
	var nodeAffinity *corev1.NodeAffinity
	if racDatabase.Spec.ClusterDetails != nil && len(racDatabase.Spec.ClusterDetails.WorkerNodeSelector) > 0 {
		nodeAffinity = shareddiskcheck.NodeAffinityForSelectorMap(racDatabase.Spec.ClusterDetails.WorkerNodeSelector)
	}
	disks := flattenAsmDisks(&racDatabase.Spec)
	cmd := shareddiskcheck.BuildDiskCheckCommand(disks)
	volumeMounts, volumes := shareddiskcheck.BuildDiskHostPathVolumes(disks, sanitizeK8sName)
	return shareddiskcheck.BuildDaemonSet(racDatabase.Namespace, racDatabase.Spec.Image, labels, nodeAffinity, cmd, volumeMounts, volumes)
}

const maxNameLen = 63

// sanitizeK8sName normalizes a string so it can be used as a Kubernetes object name.
func sanitizeK8sName(name string) string {
	return sharednaming.SanitizeK8sName(name, maxNameLen)
}

// shortHash returns a deterministic truncated SHA-1 checksum for the provided text.
func shortHash(text string, n int) string {
	return sharednaming.ShortHash(text, n)
}

// GetAsmPvcName builds the PVC name for the specified ASM disk and database.
func GetAsmPvcName(diskPath, dbName string) string {
	return sharednaming.AsmPVCName(diskPath, dbName, maxNameLen)
}

// GetAsmPvName builds the PV name for the specified ASM disk and database.
func GetAsmPvName(diskPath, dbName string) string {
	return sharednaming.AsmPVName(diskPath, dbName, maxNameLen)
}

// CheckSfset retrieves the named StatefulSet if present, returning nil when not found.
func CheckSfset(sfsetName string, instance *racdb.RacDatabase, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfSetFound := &appsv1.StatefulSet{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      sfsetName,
		Namespace: instance.Namespace,
	}, sfSetFound)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err // not an error, just means it doesn't exist
		}
		return nil, err
	}

	return sfSetFound, nil
}

// GetRacK8sClientConfig lazily initializes and returns Kubernetes config and client for RAC operations.
func GetRacK8sClientConfig(kClient client.Client) (clientcmd.ClientConfig, kubernetes.Interface, error) {
	var err1 error
	var kubeConfig clientcmd.ClientConfig
	var kubeClient kubernetes.Interface

	racdb.KubeConfigOnce.Do(func() {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		config, err := kubeConfig.ClientConfig()
		if err != nil {
			err1 = err
		}
		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			err1 = err
		}

	})
	return kubeConfig, kubeClient, err1
}

// racDBValidationCmd builds the command slice for validating the RAC database setup.
func racDBValidationCmd() []string {

	oraScriptMount1 := getOraScriptMount()
	var oraShardValidateCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--checkliveness=true ", "--optype=primaryRACDB"}
	return oraShardValidateCmd
}

// racNodeDelCmd constructs the command used to delete a RAC node via scripts.
func racNodeDelCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	var oraRacNodeDelCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--delracnode=\"del_rachome=true;del_gridnode=true\""}
	return oraRacNodeDelCmd
}

// racDBLsnrSetup builds the command for configuring the RAC database listener.
func racDBLsnrSetup() []string {
	oraScriptMount1 := getOraScriptMount()
	var oraRacNodeDelCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--setupdblsnr=\"del_rachome=true;del_gridnode=true\""}
	return oraRacNodeDelCmd
}

// getAsmCmd returns the command that fetches CRS ASM device entries from the env file.
func getAsmCmd() []string {
	asmCmd := []string{"bash", "-c", "cat /etc/rac_env_vars/envfile | grep CRS_ASM_DEVICE_LIST"}
	return asmCmd
}

// getDbAsmCmd returns the command that fetches DB ASM device entries from the env file.
func getDbAsmCmd() []string {
	asmCmd := []string{"bash", "-c", "cat /etc/rac_env_vars/envfile | grep DB_ASM_DEVICE_LIST"}
	return asmCmd
}

// getOraScriptMount returns the shared scripts mount path constant.
func getOraScriptMount() string {
	return sharedoracmd.ScriptMount
}

// getOraDbUser returns the Oracle database OS user name.
func getOraDbUser() string {
	return sharedoracmd.DBUser
}

// getOraGiUser returns the Oracle Grid Infrastructure OS user name.
func getOraGiUser() string {
	return sharedoracmd.GIUser
}

// getOraPythonCmd provides the Python interpreter path for RAC scripts.
func getOraPythonCmd() string {
	return sharedoracmd.Python3Cmd
}

// UpdateScanEP updates SCAN endpoints within the specified pod using provided GI home and scan name.
func UpdateScanEP(gihome string, scanname string, podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	time.Sleep(60 * time.Second)
	_, _, err := ExecCommand(podName, getUpdateScanEpCmd(gihome, scanname), NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating the scan endpoints")
	}

	return nil
}

// UpdateCDP reconciles Cluster Domain Services configuration inside the pod without enabling it.
func UpdateCDP(
	gihome string,
	podName string,
	instance *racdb.RacDatabase,
	kubeClient kubernetes.Interface,
	kubeconfig clientcmd.ClientConfig,
	logger logr.Logger,
) error {

	// Small delay to allow cluster topology to stabilize
	time.Sleep(60 * time.Second)

	_, _, err := ExecCommand(
		podName,
		getUpdateCdpCmd(gihome),
		NewExecCommandResp(kubeClient, kubeconfig),
		instance,
		logger,
	)

	if err != nil {
		return fmt.Errorf(
			"error occurred while reconciling CDP for operation",
		)
	}

	return nil
}

// UpdateTCPPort applies TCP listener port configuration within the specified pod.
func UpdateTCPPort(gihome string, portlist string, lsnrname string, podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, getUpdateTCPPortCmd(gihome, portlist, lsnrname), NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating TCP listener ports")
	}

	return nil
}

// UpdateAsmCount aligns ASM cardinality for the cluster within the provided pod.
func UpdateAsmCount(gihome string, podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, getUpdateAsmCount(gihome), NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating TCP listener ports")
	}

	return nil
}

// ValidateDbSetup runs database validation scripts inside the target pod.
func ValidateDbSetup(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, racDBValidationCmd(), NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while validating the DB Setup")
	}
	return nil
}

// DelRacNode invokes the deletion routine for a RAC node on the specified pod.
func DelRacNode(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	delcmd := racNodeDelCmd()
	_, _, err := ExecCommand(podName, delcmd, NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
	if err != nil {
		return fmt.Errorf("error occurred while deleting the RAC node: %w", err)
	}

	return nil
}

// CheckAsmList retrieves configured CRS ASM devices from the pod environment.
func CheckAsmList(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	return CheckAsmListWithResp(podName, NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
}

// CheckAsmListWithResp retrieves configured CRS ASM devices from the pod environment using bundled exec context.
func CheckAsmListWithResp(podName string, resp *ExecCommandResp, instance *racdb.RacDatabase, logger logr.Logger) (string, error) {
	output, _, err := ExecCommandWithResp(podName, getAsmCmd(), resp, instance, logger)
	if err != nil {
		return "", err
	}
	return parseAsmDeviceListFromOutput(output)
}

// func getASMListDisks(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
// 	output, _, err := ExecCommand(podName, getAsmCmd(), NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
// 	if err != nil {
// 		return "", err
// 	}

// 	parts := strings.SplitN(output, "=", 2)
// 	if len(parts) < 2 {
// 		return "", fmt.Errorf("unable to parse ASM device list from output: %s", output)
// 	}

//		// Trim the \r and \n characters from the end of the string
//		deviceList := strings.TrimSpace(parts[1])
//		deviceList = strings.ReplaceAll(deviceList, "\r", "")
//		return deviceList, nil
//	}
//
// CheckDbAsmList retrieves configured DB ASM devices from the pod environment.
// CheckDbAsmList provides documentation for the CheckDbAsmList function.
func CheckDbAsmList(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	return CheckDbAsmListWithResp(podName, NewExecCommandResp(kubeClient, kubeconfig), instance, logger)
}

// CheckDbAsmListWithResp retrieves configured DB ASM devices from the pod environment using bundled exec context.
func CheckDbAsmListWithResp(podName string, resp *ExecCommandResp, instance *racdb.RacDatabase, logger logr.Logger) (string, error) {
	output, _, err := ExecCommandWithResp(podName, getDbAsmCmd(), resp, instance, logger)
	if err != nil {
		return "", err
	}
	return parseAsmDeviceListFromOutput(output)
}

func parseAsmDeviceListFromOutput(output string) (string, error) {
	_, value, ok := strings.Cut(output, "=")
	if !ok {
		return "", fmt.Errorf("unable to parse ASM device list from output: %s", output)
	}

	deviceList := strings.TrimSpace(value)
	deviceList = strings.ReplaceAll(deviceList, "\r", "")
	return deviceList, nil
}

// checkPvc fetches the named PVC from the instance namespace if it exists.
func checkPvc(pvcName string, instance *racdb.RacDatabase, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
	pvcFound := &corev1.PersistentVolumeClaim{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      pvcName,
		Namespace: instance.Namespace,
	}, pvcFound)
	if err != nil {
		return pvcFound, err
	}
	return pvcFound, nil
}

// checkPv fetches the named PV from the instance namespace if it exists.
func checkPv(pvName string, instance *racdb.RacDatabase, kClient client.Client) (*corev1.PersistentVolume, error) {
	pvFound := &corev1.PersistentVolume{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      pvName,
		Namespace: instance.Namespace,
	}, pvFound)
	if err != nil {
		return pvFound, err
	}
	return pvFound, nil
}

// DelRacSwPvcClusterStyle deletes the software PVC for the given cluster node index.
func DelRacSwPvcClusterStyle(instance *racdb.RacDatabase, clusterSpec *racdb.RacClusterDetailSpec, nodeIndex int, kClient client.Client, logger logr.Logger) error {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	pvcName := nodeName + "-oradata-sw-vol"

	LogMessages("DEBUG", "Inside DelRacSwPvcClusterStyle and received param: "+GetFmtStr(pvcName), nil, instance, logger)
	return deletePVCIfExists(instance, pvcName, kClient, logger)
}

func deletePVCIfExists(instance *racdb.RacDatabase, pvcName string, kClient client.Client, logger logr.Logger) error {
	pvcFound, err := checkPvc(pvcName, instance, kClient)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in finding the pvc claim!", nil, instance, logger)
		return nil
	}
	err = kClient.Delete(context.Background(), pvcFound)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in deleting the pvc claim!", nil, instance, logger)
		return err
	}
	return nil
}

// DelRacPvc removes the ASM PVC associated with the specified disk.
func DelRacPvc(instance *racdb.RacDatabase, diskName, dgName string, kClient client.Client, logger logr.Logger) error {
	pvcName := GetAsmPvcName(diskName, instance.Name)
	logger.Info("Deleting PVC", "pvcName", pvcName)

	pvc := &corev1.PersistentVolumeClaim{}
	err := kClient.Get(context.Background(), types.NamespacedName{Namespace: instance.Namespace, Name: pvcName}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("PVC not found, nothing to delete", "pvcName", pvcName)
			return nil
		}
		return err
	}

	// Remove finalizers
	if len(pvc.Finalizers) > 0 {
		pvc.Finalizers = nil
		if err := kClient.Update(context.Background(), pvc); err != nil {
			return err
		}
	}

	// Delete PVC
	if err := kClient.Delete(context.Background(), pvc); err != nil {
		return err
	}

	return nil
}

// DelRacPv removes the ASM persistent volume linked to the provided disk.
func DelRacPv(instance *racdb.RacDatabase, diskName, dgName string, kClient client.Client, logger logr.Logger) error {
	pvName := GetAsmPvName(diskName, instance.Name)
	logger.Info("Deleting PV", "pvName", pvName)

	pv := &corev1.PersistentVolume{}
	err := kClient.Get(context.Background(), types.NamespacedName{Name: pvName}, pv)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("PV not found, nothing to delete", "pvName", pvName)
			return nil
		}
		return err
	}

	// Remove finalizers
	if len(pv.Finalizers) > 0 {
		pv.Finalizers = nil
		if err := kClient.Update(context.Background(), pv); err != nil {
			return err
		}
	}

	// // Clear claimRef for Retain PVs
	// if pv.Spec.ClaimRef != nil {
	// 	pv.Spec.ClaimRef = nil
	// 	if err := kClient.Update(context.Background(), pv); err != nil {
	// 		return err
	// 	}
	// }

	// Delete PV
	if err := kClient.Delete(context.Background(), pv); err != nil {
		return err
	}

	return nil
}

// CheckRacSvcForCluster retrieves a RAC cluster service for the given node index and type.
func CheckRacSvcForCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	svcType string,
	svcName string, // Optional: for nodeport overrides
	kClient client.Client,
) (*corev1.Service, error) {
	var name string

	if svcType == "nodeport" && svcName != "" {
		name = svcName
	} else {
		name = GetClusterSvcName(instance, clusterSpec, nodeIndex, svcType)
	}

	return getServiceIfExists(kClient, instance.Namespace, name)
}

func getServiceIfExists(kClient client.Client, namespace, name string) (*corev1.Service, error) {
	svcFound := &corev1.Service{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, svcFound)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return svcFound, nil
}

// PodListValidation checks pods matching the StatefulSet name and returns readiness information.
func PodListValidation(podList *corev1.PodList, sfName string, instance *racdb.RacDatabase, kClient client.Client) (bool, *corev1.Pod, *corev1.Pod) {
	_ = instance
	_ = kClient

	var notReadyPod *corev1.Pod

	for i := range podList.Items {
		pod := &podList.Items[i]
		if !podBelongsToStatefulSet(pod.Name, sfName) {
			continue
		}
		if isPodReadyForValidation(pod) {
			return true, pod.DeepCopy(), nil
		}
		if notReadyPod == nil {
			notReadyPod = pod.DeepCopy()
		}
	}

	// Return false if no ready pod was found, and the first not ready pod (if any)
	return false, nil, notReadyPod
}

func podBelongsToStatefulSet(podName, sfName string) bool {
	if sfName == "" {
		return false
	}
	return strings.HasPrefix(podName, sfName+"-")
}

func isPodReadyForValidation(pod *corev1.Pod) bool {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	if len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false
		}
	}
	return true
}

// checkPodStatus ensures the pod is running and ready.
func checkPodStatus(pod *corev1.Pod, kClient client.Client,
) error {
	var msg string
	for _, condition := range pod.Status.Conditions {
		if pod.Status.Phase == corev1.PodRunning {
			if condition.Type == corev1.PodReady {
				// msg = "Pod Status is running"
				// LogMessages("DEBUG", msg)
				return nil
			}
		} else {
			msg = "Pod is not scheduled or ready " + pod.Name + ".Describe the pod to check the detailed message"
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}

// checkContainerStatus verifies at least one container in the pod is running.
func checkContainerStatus(pod *corev1.Pod, kClient client.Client,
) error {
	_ = kClient

	isRunning := false
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Running != nil {
			isRunning = true
			break
		}
	}
	if !isRunning {
		return fmt.Errorf("Container is not in running state%s.Describe the pod to check the detailed message", pod.Name)
	}
	return nil
}

// NewNamespace creates a corev1.Namespace object using the provided name.
// NewNamespace constructs a corev1.Namespace object for the specified name.
func NewNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// getOwnerRef creates a controller owner reference for the given RAC instance.
func getOwnerRef(instance *racdb.RacDatabase,
) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{Kind: instance.GroupVersionKind().Kind, APIVersion: instance.APIVersion, Name: instance.Name, UID: types.UID(instance.UID)},
	}
}

// getRacInitContainerCmd builds the init container shell script for RAC networking setup.
func getRacInitContainerCmd(resType string, name string, oraScriptMount string, intf1 string, intf2 string) string {
	_ = name

	// fallback (should normally be overridden by annotations)
	if intf1 == "" {
		intf1 = "ens1"
	}
	if intf2 == "" {
		intf2 = "ens2"
	}

	var initCmd string

	if oraScriptMount != "NOLOC" {
		initCmd = `
			chown -R 54321:54321 ` + oraScriptMount + ` && 
			chmod 755 ` + oraScriptMount + `/* && 
		`
	}

	// dynamically use annotated interfaces
	initCmd += `
		IP1=$(ip addr show ` + intf1 + ` | awk '/inet /{print $2}' | cut -d/ -f1) && 
		IP2=$(ip addr show ` + intf2 + ` | awk '/inet /{print $2}' | cut -d/ -f1) && 

		# Write environment file (overwrite + append)
		if [ -n "$IP1" ]; then
			echo "CRS_PRIVATE_IP1=$IP1" > /etc/rac_env_vars_writable/envfile;
		fi;

		if [ -n "$IP2" ]; then
			echo "CRS_PRIVATE_IP2=$IP2" >> /etc/rac_env_vars_writable/envfile;
		fi;

	`

	initCmd += resType
	return initCmd
}

// GetFmtStr wraps the provided string in brackets for logging clarity.
func GetFmtStr(pstr string,
) string {
	return fmt.Sprintf("[%s]", pstr)
}

// getClusterState queries the cluster health for the specified pod.
func getClusterState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	_ = specidx
	return getCommandStateOrPending(podName, getGiHealthCmd(), instance, kubeClient, kubeConfig, logger)
}

// getDbInstState returns the database state for the given RAC pod.
func getDbInstState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	_ = specidx
	return getCommandStateOrPending(podName, getRacDbModeCmd(), instance, kubeClient, kubeConfig, logger)
}

func getCommandStateOrPending(
	podName string,
	cmd []string,
	instance *racdb.RacDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, cmd, NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// GetAsmInstState collects ASM disk group details for the specified pod.
func GetAsmInstState(
	podName string,
	instance *racdb.RacDatabase,
	specidx int,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) []racdb.AsmDiskGroupStatus {
	return buildAsmInstState(podName, instance, specidx, kubeClient, kubeConfig, logger, false)
}

// stringsToAsmDiskStatus converts disk names into RAC ASM disk status objects.
func stringsToAsmDiskStatus(disks []string) []racdb.AsmDiskStatus {
	var result []racdb.AsmDiskStatus
	for _, disk := range disks {
		result = append(result, racdb.AsmDiskStatus{Name: disk})
	}
	return result
}

// getAsmInstState mirrors GetAsmInstState providing disk group state discovery.
func getAsmInstState(
	podName string,
	racDatabase *racdb.RacDatabase,
	specidx int,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) []racdb.AsmDiskGroupStatus {
	return buildAsmInstState(podName, racDatabase, specidx, kubeClient, kubeConfig, logger, true)
}

func buildAsmInstState(
	podName string,
	racDatabase *racdb.RacDatabase,
	specidx int,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	includeStoredDiskSize bool,
) []racdb.AsmDiskGroupStatus {
	var diskGroups []racdb.AsmDiskGroupStatus

	diskGroup := GetAsmDiskgroup(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
	if diskGroup == "Pending" || diskGroup == "" {
		return diskGroups
	}

	dglist := strings.Split(diskGroup, ",")

	for _, dg := range dglist {
		asmdg := racdb.AsmDiskGroupStatus{}
		asmdg.Name = strings.TrimSpace(dg)

		diskNames := getAsmDisks(podName, dg, racDatabase, specidx, kubeClient, kubeConfig, logger)
		if includeStoredDiskSize {
			for _, disk := range diskNames {
				diskName := strings.TrimSpace(disk)
				asmdg.Disks = append(asmdg.Disks, racdb.AsmDiskStatus{
					Name:     diskName,
					SizeInGb: getExistingAsmDiskSize(racDatabase, asmdg.Name, diskName),
					Valid:    true,
				})
			}
		} else {
			asmdg.Disks = stringsToAsmDiskStatus(diskNames)
		}

		asmdg.Redundancy = getAsmDgRedundancy(podName, dg, racDatabase, specidx, kubeClient, kubeConfig, logger)

		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
			if specDG.Name == asmdg.Name {
				asmdg.Type = specDG.Type
				asmdg.AutoUpdate = specDG.AutoUpdate
				asmdg.StorageClass = specDG.StorageClass
				break
			}
		}

		diskGroups = append(diskGroups, asmdg)
	}

	return diskGroups
}

// getExistingAsmDiskSize returns the recorded size of an ASM disk from status, if present.
func getExistingAsmDiskSize(
	racDatabase *racdb.RacDatabase,
	dgName string,
	diskName string,
) int {

	for _, dg := range racDatabase.Status.AsmDiskGroups {
		if dg.Name != dgName {
			continue
		}
		for _, d := range dg.Disks {
			if d.Name == diskName {
				return d.SizeInGb // already int
			}
		}
	}
	return 0
}

// GetAsmDiskgroup queries the ASM disk group list from the running pod.
func GetAsmDiskgroup(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmDiskgroupCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getAsmDisks retrieves detailed disk names for the given disk group.
func getAsmDisks(podName string, dg string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) []string {

	stdoutput, _, err := ExecCommand(podName, getAsmDisksCmd(dg), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return strings.Split(msg, ",")
	}
	cleanOutput := strings.ReplaceAll(stdoutput, "\r", "")
	lines := strings.Split(strings.TrimSpace(cleanOutput), "\n")
	var disks []string
	for _, line := range lines {
		for _, disk := range strings.Split(line, ",") {
			disk = strings.TrimSpace(disk)
			if disk != "" {
				disks = append(disks, disk)
			}
		}
	}
	return disks
}

// getAsmDgRedundancy fetches the redundancy setting for the specified disk group.
func getAsmDgRedundancy(podName string, dg string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmDgRedundancyCmd(dg), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getAsmInstName returns the ASM instance name reported by the pod.
func getAsmInstName(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmInstNameCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getAsmInstStatus returns the status of the ASM instance in the pod.
func getAsmInstStatus(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmInstStatusCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getRacInstStateFile interprets the orchestration state from the RAC state file.
func getRacInstStateFile(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getRacInstStateFileCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	//Possible values are "provisioning", "addnode", "failed", "completed"
	if strings.ToLower(strings.TrimSpace(stdoutput)) == "provisioning" {
		return string(racdb.RACProvisionState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "completed" {
		return string(racdb.RACAvailableState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "addnode" {
		return string(racdb.RACAddInstState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "failed" {
		return string(racdb.RACFailedState)
	} else {
		return string(racdb.RACPendingState)
	}
}

// getRacInstStateFileForCluster reads the RAC state file for clustered deployments.
func getRacInstStateFileForCluster(
	podName string,
	instance *racdb.RacDatabase,
	nodeIndex int, // you might keep this for consistency/logging, but it's unused here
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getRacInstStateFileCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}

	switch strings.ToLower(strings.TrimSpace(stdoutput)) {
	case "provisioning":
		return string(racdb.RACProvisionState)
	case "completed":
		return string(racdb.RACAvailableState)
	case "addnode":
		return string(racdb.RACAddInstState)
	case "failed":
		return string(racdb.RACFailedState)
	default:
		return string(racdb.RACPendingState)
	}
}

// getDBVersion fetches the database version string from the pod.
func getDBVersion(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getRacDbVersionCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	if strings.Contains(stdoutput, "ERROR") {
		return "NOTAVAILABLE"
	} else {
		return strings.TrimSpace(stdoutput)
	}
}

// getDbState returns the health status of the database instance.
func getDbState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getRACHealthCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getDbRole reports the database role (primary/standby) for the instance.
func getDbRole(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getDbRoleCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getConnStr fetches the RAC connect string from the pod.
func getConnStr(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getConnStrCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getExternalConnStr constructs the external SCAN listener connect string.
func getExternalConnStr(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	// stdoutput, _, err := ExecCommand(podName, getConnStrCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	// if err != nil {
	// 	msg := "Pending"
	// 	LogMessages("DEBUG", msg, err, instance, logger)
	// 	return msg
	// }
	// return strings.TrimSpace(stdoutput)
	// Fetch the racnode-scan-lsnr service
	svc, err := kubeClient.CoreV1().Services(instance.Namespace).Get(context.Background(), "racnode-scan-lsnr", metav1.GetOptions{})
	if err != nil {
		msg := "Failed to get racnode-scan-lsnr service"
		LogMessages("DEBUG", msg, err, instance, logger)
		return "Pending"
	}

	// Find the NodePort for port 1521
	var nodePort int32
	for _, port := range svc.Spec.Ports {
		if port.Port == 1521 {
			nodePort = port.NodePort
			break
		}
	}

	if nodePort == 0 {
		msg := "Failed to find NodePort for port 1521 in racnode-scan-lsnr service"
		LogMessages("DEBUG", msg, err, instance, logger)
		return "Pending"
	}

	// Construct the external connect string
	externalConnectString := fmt.Sprintf("racnode-scan.%s.svc.cluster.local:%d/%s", instance.Namespace, nodePort, instance.Spec.ServiceDetails.Name)
	return externalConnectString
}

// getClientEtcHost builds the `/etc/hosts` entries for client connectivity.
func getClientEtcHost(podNames []string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger, nodeDetails map[string]*corev1.Node) []string {
	// Prepare to update ClientEtcHost
	var clientEtcHost []string

	// Loop through all pod names
	for _, podName := range podNames {
		node := nodeDetails[podName]
		if node == nil {
			logger.Error(nil, "Node details not found for pod", "podName", podName)
			continue
		}

		// Get nodeIP from node details
		nodeIP := getNodeIPFromNodeDetails(node)

		// Construct the line to be added to ClientEtcHost
		line := fmt.Sprintf("%s    %s.rac.svc.cluster.local %s-vip.rac.svc.cluster.local    racnode-scan.rac.svc.cluster.local", nodeIP, podName, podName)
		clientEtcHost = append(clientEtcHost, line)
	}

	// Update instance status with the new ClientEtcHost
	instance.Status.ClientEtcHost = clientEtcHost

	// Assuming you are returning clientEtcHost as well for any further processing
	return clientEtcHost
}

// getNodeIPFromNodeDetails extracts the preferred IP for the provided node.
func getNodeIPFromNodeDetails(node *corev1.Node) string {
	return sharednetutil.PreferredNodeIP(node)
}

// readHostsFile reads the `/etc/hosts` file from the privileged RAC pod.
func readHostsFile(podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *racdb.RacDatabase, logger logr.Logger) (string, error) {

	cmd := []string{"cat", "/etc/hosts"} // Assuming this matches the MountPath in buildContainerSpecForRac

	stdOutput, _, err := ExecCommand(podName, cmd, NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Failed to read /etc/hosts from pod" + podName
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}
	return strings.TrimSpace(stdOutput), nil
}

// parseHostsContent returns the non-comment lines from an `/etc/hosts` file.
func parseHostsContent(content string) string {
	return sharednetutil.ParseHostsContent(content)
}

// getSvcState reports the status of the configured database service.
func getSvcState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getDBServiceStatus(instance.Status.ConfigParams.DbHome, instance.Status.ConfigParams.DbName, instance.Status.ServiceDetails.Name), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getPdbConnStr retrieves the PDB connect string from the RAC pod.
func getPdbConnStr(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getPdbConnStrCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getGridHome retrieves the `GRID_HOME` environment variable from the specified pod.
func getGridHome(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	stdoutput, _, err := ExecCommand(podName, getGridHomeCmd(), NewExecCommandResp(kubeClient, kubeConfig), instance, logger)
	if err != nil {
		msg := "Error retrieving GRID_HOME"
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}

	return strings.TrimSpace(stdoutput), nil
}

// GetCrsNodesForCluster reports CRS node details for clustered RAC deployments.
func GetCrsNodesForCluster(
	instance *racdb.RacDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	kClient client.Client,
) (string, string, string, string, string) {

	var newCrsNodes []string
	var newCrsNodesList []string
	var existingCrsNodesHealthy []string
	var existingCrsNodesNotHealthy []string
	var installNode string
	var installNodeFlag bool

	cd := instance.Spec.ClusterDetails
	if cd != nil && cd.NodeCount > 0 {
		// Check for StatefulSet per expected node
		for i := 0; i < cd.NodeCount; i++ {
			nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
			statefulset, _ := CheckSfset(nodeName, instance, kClient)
			if statefulset == nil {
				// You may use a convention for VIP here if needed, else skip
				vipName := fmt.Sprintf("%s-0-vip", nodeName) // Modify as per your VIP naming convention if needed
				newCrsNodes = append(newCrsNodes, "pubhost:"+nodeName+"-0"+","+"viphost:"+vipName)
				newCrsNodesList = append(newCrsNodesList, nodeName+"-0")
				if !installNodeFlag {
					installNode = nodeName + "-0"
					installNodeFlag = true
				}
			}
		}

		// Update Racnode status (removed nodes)
		if len(instance.Status.RacNodes) > 0 {
			for _, racStatus := range instance.Status.RacNodes {
				nodeName := strings.Split(racStatus.Name, "-")[0]
				_, err := CheckSfset(nodeName, instance, kClient)
				if err != nil {
					newRacStatus := delRacNodeStatus(instance, racStatus.Name)
					instance.Status.RacNodes = newRacStatus
				}
			}
		}

		// Check for healthy and non-healthy nodes
		for _, racStatus := range instance.Status.RacNodes {
			if racStatus.NodeDetails.ClusterState == "HEALTHY" {
				if !checkElem(existingCrsNodesHealthy, racStatus.Name) {
					existingCrsNodesHealthy = append(existingCrsNodesHealthy, racStatus.Name)
				}
			} else {
				if !checkElem(existingCrsNodesNotHealthy, racStatus.Name) {
					existingCrsNodesNotHealthy = append(existingCrsNodesNotHealthy, racStatus.Name)
				}
			}
		}
	}

	return strings.Join(newCrsNodes, ";"),
		strings.Join(existingCrsNodesHealthy, ","),
		strings.Join(existingCrsNodesNotHealthy, ","),
		installNode,
		strings.Join(newCrsNodesList, ",")
}

// GetAsmDevices returns ASM device list from the instance specification.
func GetAsmDevices(instance *racdb.RacDatabase) string {
	var groups [][]string
	if instance.Spec.AsmStorageDetails != nil {
		for _, dg := range instance.Spec.AsmStorageDetails {
			groups = append(groups, dg.Disks)
		}
	}
	return sharedinstanceutil.JoinAsmDiskGroups(groups)
}

// GetScanname constructs the SCAN service name for the RAC instance.
func GetScanname(instance *racdb.RacDatabase) string {
	return instance.Spec.ScanSvcName
}

// GetSSHkey checks for presence and readiness of the RAC SSH secret.
func GetSSHkey(instance *racdb.RacDatabase, name string, kClient client.Client) (bool, bool) {
	secretFound, err := CheckSecret(instance, name, kClient)
	if err != nil {
		return false, false
	}
	return sharedinstanceutil.SSHKeyFlags(
		secretFound.Data,
		instance.Spec.SshKeySecret.PrivKeySecretName,
		instance.Spec.SshKeySecret.PubKeySecretName,
	)
}

// GetDbSecret determines if required database secrets exist and contain keys.
func GetDbSecret(instance *racdb.RacDatabase, name string, kClient client.Client) (bool, bool, bool) {
	secretFound, err := CheckSecret(instance, name, kClient)
	if err != nil {
		return false, false, false
	}
	return sharedinstanceutil.DBSecretFlags(
		secretFound.Data,
		instance.Spec.DbSecret.PwdFileName,
		instance.Spec.DbSecret.KeyFileName,
	)
}

// GetTdeWalletSecret validates the TDE wallet secret presence and keys.
func GetTdeWalletSecret(instance *racdb.RacDatabase, name string, kClient client.Client) (bool, bool, bool) {

	var commonospassflag, commonpwdfile, pwdkeyflag bool
	secretFound, err := CheckSecret(instance, name, kClient)

	if err != nil {
		return false, false, false
	} else {
		for key := range secretFound.Data {
			switch key {
			case instance.Spec.TdeWalletSecret.PwdFileName:
				commonospassflag = true
			case instance.Spec.TdeWalletSecret.KeyFileName:
				pwdkeyflag = true
			case "pwdfile":
				commonpwdfile = true
			}
		}
	}
	return commonospassflag, pwdkeyflag, commonpwdfile
}

// CheckSecret retrieves a named secret from the RAC namespace.
func CheckSecret(instance *racdb.RacDatabase, secretName string, kClient client.Client) (*corev1.Secret, error) {

	secretFound := &corev1.Secret{}

	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: instance.Namespace,
	}, secretFound)

	if err != nil {
		return secretFound, err
	}

	return secretFound, nil
}

// GetGiResponseFile fetches the Grid Infrastructure response file configmap data.
func GetGiResponseFile(instance *racdb.RacDatabase, kClient client.Client) (bool, map[string]string) {

	var giFileFlag bool
	cName := instance.Spec.ConfigParams.GridResponseFile.ConfigMapName

	configMapFound, err := CheckConfigMap(instance, cName, kClient)
	if err != nil {

	} else {
		for key := range configMapFound.Data {
			if key == instance.Spec.ConfigParams.GridResponseFile.Name {
				giFileFlag = true
			}
		}
	}
	return giFileFlag, configMapFound.Data
}

// GetDbResponseFile fetches the database response file configmap data.
func GetDbResponseFile(instance *racdb.RacDatabase, kClient client.Client) (bool, map[string]string) {

	var dbFileFlag bool
	cName := instance.Spec.ConfigParams.DbResponseFile.ConfigMapName

	configMapFound, err := CheckConfigMap(instance, cName, kClient)
	if err != nil {

	} else {
		for key := range configMapFound.Data {
			if key == instance.Spec.ConfigParams.DbResponseFile.Name {
				dbFileFlag = true
			}
		}
	}
	return dbFileFlag, configMapFound.Data
}

// CheckConfigMap retrieves a configmap by name from the RAC namespace.
func CheckConfigMap(instance *racdb.RacDatabase, configMapName string, kClient client.Client) (*corev1.ConfigMap, error) {

	configMapFound := &corev1.ConfigMap{}

	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      configMapName,
		Namespace: instance.Namespace,
	}, configMapFound)

	if err != nil {
		return configMapFound, err
	}

	return configMapFound, nil

}

// checkElem returns true when the element exists in the provided slice.
func checkElem(list1 []string, element string) bool {

	if element != "" {
		if len(list1) > 0 {
			for _, v := range list1 {
				if v == element {
					return true
				}
			}
		}
	}

	return false
}

// ValidateNetInterface ensures the provided network exists within response data.
func ValidateNetInterface(net string, instance *racdb.RacDatabase, rspNetData string) error {

	var err error

	if net != "" {
		if !strings.Contains(rspNetData, net) {
			err = fmt.Errorf("Error occurred during retreiving network card detail from grid responsefile: %s", "The key does not exist")
		}
	}

	return err
}

// CheckRspData retrieves a value from a response file configmap entry.
func CheckRspData(instance *racdb.RacDatabase, kClient client.Client, key string, cName string, fname string) (string, error) {
	configMapFound, _ := CheckConfigMap(instance, cName, kClient)
	data := configMapFound.Data[fname]
	return sharedrsp.ParseValue(data, key)
}

// GetServiceParams formats service parameter information for status reporting.
func GetServiceParams(instance *racdb.RacDatabase) string {
	var sparams string

	sparams = "service:" + instance.Spec.ServiceDetails.Name
	notficationFlag, _ := strconv.ParseBool(instance.Spec.ServiceDetails.Notification)
	if notficationFlag {
		sparams = sparams + ";notification:" + "True"
	}
	if instance.Spec.ServiceDetails.Cardinality != "" {
		sparams = sparams + ";cardinality:" + instance.Spec.ServiceDetails.Cardinality
	}
	if len(instance.Spec.ServiceDetails.Preferred) > 0 {
		sparams = sparams + ";preferred:" + strings.Join(instance.Spec.ServiceDetails.Preferred[:], ",")
	}
	if len(instance.Spec.ServiceDetails.Available) > 0 {
		sparams = sparams + ";available:" + strings.Join(instance.Spec.ServiceDetails.Available[:], ",")
	}
	if instance.Spec.ServiceDetails.Pdb != "" {
		sparams = sparams + ";pdb:" + instance.Spec.ServiceDetails.Pdb
	}

	if instance.Spec.ServiceDetails.ClbGoal != "" {
		sparams = sparams + ";clbgoal:" + instance.Spec.ServiceDetails.ClbGoal
	}

	if instance.Spec.ServiceDetails.RlbGoal != "" {
		sparams = sparams + ";rlbgoal:" + instance.Spec.ServiceDetails.RlbGoal
	}

	if instance.Spec.ServiceDetails.FailOverRestore != "" {
		sparams = sparams + ";failover_restore:" + instance.Spec.ServiceDetails.FailOverRestore
	}

	if instance.Spec.ServiceDetails.FailBack != "" {
		sparams = sparams + ";failback:" + instance.Spec.ServiceDetails.FailBack
	}

	cmdFlag, _ := strconv.ParseBool(instance.Spec.ServiceDetails.CommitOutCome)
	if cmdFlag {
		sparams = sparams + ";commit_outcome:" + "True"
	}

	cmtPathFlag, _ := strconv.ParseBool(instance.Spec.ServiceDetails.CommitOutComeFastPath)
	if cmtPathFlag {
		sparams = sparams + ";commit_outcome_fastpath:" + "True"
	}

	if instance.Spec.ServiceDetails.FailBack != "" {
		sparams = sparams + ";failback:" + instance.Spec.ServiceDetails.FailBack
	}

	if instance.Spec.ServiceDetails.FailOverType != "" {
		sparams = sparams + ";failovertype:" + instance.Spec.ServiceDetails.FailOverType
	}

	if instance.Spec.ServiceDetails.FailOverDelay > 0 {
		sparams = sparams + ";failoverdelay:" + strconv.FormatInt(int64(instance.Spec.ServiceDetails.FailOverDelay), 10)
	}

	if instance.Spec.ServiceDetails.FailOverRetry > 0 {
		sparams = sparams + ";failoverretry:" + strconv.FormatInt(int64(instance.Spec.ServiceDetails.FailOverRetry), 10)
	}

	if instance.Spec.ServiceDetails.DrainTimeOut > 0 {
		sparams = sparams + ";drain_timeout:" + strconv.FormatInt(int64(instance.Spec.ServiceDetails.DrainTimeOut), 10)
	}

	if instance.Spec.ServiceDetails.Dtp != "" {
		sparams = sparams + ";dtp:" + instance.Spec.ServiceDetails.Dtp
	}

	if instance.Spec.ServiceDetails.Role != "" {
		sparams = sparams + ";role:" + instance.Spec.ServiceDetails.Role
	}

	if instance.Spec.ServiceDetails.Retention > 0 {
		sparams = sparams + ";retention:" + strconv.FormatInt(int64(instance.Spec.ServiceDetails.Retention), 10)
	}

	return sparams
}

// . This function get the healthy node name from instance.status

// GetHealthyNode returns the name of a healthy RAC node if available.
func GetHealthyNode(instance *racdb.RacDatabase) (string, error) {
	var i int32

	if len(instance.Status.RacNodes) > 0 {
		for i = 0; i < int32(len(instance.Status.RacNodes)); i++ {
			if instance.Status.RacNodes[i].NodeDetails != nil {
				if instance.Status.RacNodes[i].NodeDetails.ClusterState == "HEALTHY" {
					return instance.Status.RacNodes[i].Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no healthy node exist")
}

// GetHealthyNodeCounts returns the number of healthy RAC nodes.
func GetHealthyNodeCounts(instance *racdb.RacDatabase) (int, error) {
	var i, totalNodes, healthyNodeCount int

	totalNodes = len(instance.Status.RacNodes)

	if totalNodes > 0 {
		for i = 0; i < totalNodes; i++ {
			if instance.Status.RacNodes[i].NodeDetails != nil {
				if instance.Status.RacNodes[i].NodeDetails.ClusterState == "HEALTHY" {
					healthyNodeCount++
				}
			}
		}
	}

	if totalNodes == healthyNodeCount {
		return healthyNodeCount, nil
	}
	return 0, fmt.Errorf("healthy cluster node counts are not matching with total cluster nodes")
}

// GetDBLsnrEndPoints builds database listener endpoint strings for the instance.
// GetDBLsnrEndPointsForCluster builds listener endpoints for clustered deployments.
func GetDBLsnrEndPointsForCluster(instance *racdb.RacDatabase) (string, string, error) {
	var endp string
	var locallsnr string

	cd := instance.Spec.ClusterDetails
	if cd != nil && cd.NodeCount > 0 {
		for i := 0; i < cd.NodeCount; i++ {
			nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
			// Prefer LsnrLocalPort if modeled in your ClusterDetails, else use calculated LsnrTargetPort
			var lsnrPort int32
			// Adjust this if you add a []int32 LsnrLocalPorts field to ClusterDetails in future
			if cd.BaseLsnrTargetPort > 0 {
				lsnrPort = cd.BaseLsnrTargetPort + int32(i)
				endp += fmt.Sprintf("%d,", lsnrPort)
				locallsnr += fmt.Sprintf("%s-0:%d;", nodeName, lsnrPort)
			}
		}
		// Remove trailing commas/semicolons
		if strings.HasSuffix(endp, ",") {
			endp = endp[:len(endp)-1]
		}
		if strings.HasSuffix(locallsnr, ";") {
			locallsnr = locallsnr[:len(locallsnr)-1]
		}
	}
	return locallsnr, endp, nil
}

// GetAsmDevicesForCluster aggregates ASM device information for cluster nodes.
func GetAsmDevicesForCluster(
	instance *racdb.RacDatabase,
	dgType racdb.AsmDiskDGTypes,
) string {

	var disks []string

	if instance.Spec.AsmStorageDetails == nil {
		return ""
	}

	for _, dg := range instance.Spec.AsmStorageDetails {
		if dg.Type != dgType {
			continue
		}

		for _, d := range dg.Disks {
			if d != "" {
				disks = append(disks, d)
			}
		}
	}

	return strings.Join(disks, ",")
}

// AsmDevicesByType filters ASM disks by disk group type and returns CSV output.
func AsmDevicesByType(groups []racdb.AsmDiskGroupStatus, targetType racdb.AsmDiskDGTypes) string {
	var disks []string
	for _, group := range groups {
		if group.Type == targetType {
			for _, disk := range group.Disks {
				if disk.Valid {
					disks = append(disks, disk.Name)
				}
			}
		}
	}
	// If disks are device paths and might have spaces, you may want to quote or clean here if needed
	return strings.Join(disks, ",")
}
