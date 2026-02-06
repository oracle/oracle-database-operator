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
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	utils "github.com/oracle/oracle-database-operator/commons/rac/utils"

	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// buildContainerPortsDef assembles the container port definitions for the RAC instance.
func buildContainerPortsDef(instance *racdb.RacDatabase, oraRacSpex racdb.RacInstDetailSpec) []corev1.ContainerPort {
	var result []corev1.ContainerPort
	if len(oraRacSpex.PortMappings) > 0 {
		for _, portMapping := range oraRacSpex.PortMappings {
			containerPort :=
				corev1.ContainerPort{
					Protocol:      portMapping.Protocol,
					ContainerPort: portMapping.Port,
					Name:          generatePortMapping(portMapping),
				}
			result = append(result, containerPort)
		}
	} else {
		result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraDBPort, Name: generateName(fmt.Sprintf("%s-%d", "tcp", utils.OraDBPort))})
		result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLsnrPort, Name: generateName(fmt.Sprintf("%s-%d", "tcp", utils.OraLsnrPort))})
		result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraSSHPort, Name: generateName(fmt.Sprintf("%s-%d", "tcp", utils.OraSSHPort))})
		result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLocalOnsPort, Name: generateName(fmt.Sprintf("%s-%d", "tcp", utils.OraLocalOnsPort))})
		result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraOemPort, Name: generateName(fmt.Sprintf("%s-%d", "tcp", utils.OraOemPort))})
	}

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
	podName := racName
	return podName
}

// getlabelsForRac builds common RAC labels for internal operations.
func getlabelsForRac(instance *racdb.RacDatabase) map[string]string {
	return buildLabelsForRac(instance, "RAC")
}

const maxNameLen = 63

// sanitizeK8sName normalizes a string so it can be used as a Kubernetes object name.
func sanitizeK8sName(name string) string {
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	sanitized := re.ReplaceAllString(strings.ToLower(name), "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > maxNameLen {
		sanitized = sanitized[:maxNameLen]
	}
	return sanitized
}

// shortHash returns a deterministic truncated SHA-1 checksum for the provided text.
func shortHash(text string, n int) string {
	h := sha1.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))[:n]
}

// GetAsmPvcName builds the PVC name for the specified ASM disk and database.
func GetAsmPvcName(diskPath, dbName string) string {
	// Use a hash of the device path for uniqueness, keep it short but collision-resistant
	hash := shortHash(diskPath, 8)
	base := fmt.Sprintf("asm-pvc-%s-%s", hash, sanitizeK8sName(dbName))
	if len(base) > maxNameLen {
		base = base[:maxNameLen]
	}
	return base
}

// GetAsmPvName builds the PV name for the specified ASM disk and database.
func GetAsmPvName(diskPath, dbName string) string {
	hash := shortHash(diskPath, 8)
	base := fmt.Sprintf("asm-pv-%s-%s", hash, sanitizeK8sName(dbName))
	if len(base) > maxNameLen {
		base = base[:maxNameLen]
	}
	return base
}

// CheckSfset retrieves the named StatefulSet if present, returning nil when not found.
func CheckSfset(sfsetName string, instance *racdb.RacDatabase, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfSetFound := &appsv1.StatefulSet{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
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

	return utils.OraScriptMount

}

// getOraDbUser returns the Oracle database OS user name.
func getOraDbUser() string {

	return utils.OraDBUser

}

// getOraGiUser returns the Oracle Grid Infrastructure OS user name.
func getOraGiUser() string {

	return utils.OraGridUser

}

// getOraPythonCmd provides the Python interpreter path for RAC scripts.
func getOraPythonCmd() string {

	return "/bin/python3"

}

// UpdateScanEP updates SCAN endpoints within the specified pod using provided GI home and scan name.
func UpdateScanEP(gihome string, scanname string, podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	time.Sleep(60 * time.Second)
	_, _, err := ExecCommand(podName, getUpdateScanEpCmd(gihome, scanname), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating the scan endpoints")
	}

	return nil
}

// UpdateCDP reconciles CDP configuration within the specified pod.
// CDP is recreated if required but NOT enabled or started.
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
		kubeClient,
		kubeconfig,
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

	_, _, err := ExecCommand(podName, getUpdateTCPPortCmd(gihome, portlist, lsnrname), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating TCP listener ports")
	}

	return nil
}

// UpdateAsmCount aligns ASM cardinality for the cluster within the provided pod.
func UpdateAsmCount(gihome string, podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, getUpdateAsmCount(gihome), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating TCP listener ports")
	}

	return nil
}

// ValidateDbSetup runs database validation scripts inside the target pod.
func ValidateDbSetup(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, racDBValidationCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while validating the DB Setup")
	}
	return nil
}

// DelRacNode invokes the deletion routine for a RAC node on the specified pod.
func DelRacNode(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	delcmd := racNodeDelCmd()
	_, _, err := ExecCommand(podName, delcmd, kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error occurred while deleting the RAC node: %w", err)
	}

	return nil
}

// CheckAsmList retrieves configured CRS ASM devices from the pod environment.
func CheckAsmList(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	output, _, err := ExecCommand(podName, getAsmCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(output, "=", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("unable to parse ASM device list from output: %s", output)
	}

	// Trim the \r and \n characters from the end of the string
	deviceList := strings.TrimSpace(parts[1])
	deviceList = strings.ReplaceAll(deviceList, "\r", "")
	return deviceList, nil
}

// func getASMListDisks(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
// 	output, _, err := ExecCommand(podName, getAsmCmd(), kubeClient, kubeconfig, instance, logger)
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
	output, _, err := ExecCommand(podName, getDbAsmCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(output, "=", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("unable to parse ASM device list from output: %s", output)
	}

	// Trim the \r and \n characters from the end of the string
	deviceList := strings.TrimSpace(parts[1])
	deviceList = strings.ReplaceAll(deviceList, "\r", "")
	return deviceList, nil
}

// checkPvc fetches the named PVC from the instance namespace if it exists.
func checkPvc(pvcName string, instance *racdb.RacDatabase, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
	pvcFound := &corev1.PersistentVolumeClaim{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
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
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      pvName,
		Namespace: instance.Namespace,
	}, pvFound)
	if err != nil {
		return pvFound, err
	}
	return pvFound, nil
}

// DelRacSwPvc removes the software staging PVC associated with the RAC instance.
func DelRacSwPvc(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client, logger logr.Logger) error {

	pvcName := OraRacSpex.Name + "-oradata-sw-vol-" + OraRacSpex.Name + "-0"
	LogMessages("DEBUG", "Inside the delPvc and received param: "+GetFmtStr(pvcName), nil, instance, logger)
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

// DelRacSwPvcClusterStyle deletes the software PVC for the given cluster node index.
func DelRacSwPvcClusterStyle(instance *racdb.RacDatabase, clusterSpec *racdb.RacClusterDetailSpec, nodeIndex int, kClient client.Client, logger logr.Logger) error {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	pvcName := nodeName + "-oradata-sw-vol"

	LogMessages("DEBUG", "Inside DelRacSwPvcClusterStyle and received param: "+GetFmtStr(pvcName), nil, instance, logger)
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

// CheckRacSvc attempts to retrieve a RAC service by type and name.
func CheckRacSvc(instance *racdb.RacDatabase, svcType string, oraRacSpex racdb.RacInstDetailSpec, svcName string, kClient client.Client) (*corev1.Service, error) {
	svcFound := &corev1.Service{}
	var name string

	if svcType == "nodeport" {
		name = svcName
	} else {
		name = getRacSvcName(instance, oraRacSpex, svcType)
	}

	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: instance.Namespace,
	}, svcFound)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // not an error, service already gone
		}
		return nil, err // real error
	}

	return svcFound, nil
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
	svcFound := &corev1.Service{}
	var name string

	if svcType == "nodeport" && svcName != "" {
		name = svcName
	} else {
		name = GetClusterSvcName(instance, clusterSpec, nodeIndex, svcType)
	}

	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: instance.Namespace,
	}, svcFound)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // Not an error, service may just be gone
		}
		return nil, err // Real error
	}

	return svcFound, nil
}

// PodListValidation checks pods matching the StatefulSet name and returns readiness information.
func PodListValidation(podList *corev1.PodList, sfName string, instance *racdb.RacDatabase, kClient client.Client) (bool, *corev1.Pod, *corev1.Pod) {
	var notReadyPod *corev1.Pod

	for _, pod := range podList.Items {
		if strings.Contains(pod.Name, sfName) {
			// Check pod status
			if pod.Status.Phase != corev1.PodRunning {
				if notReadyPod == nil {
					notReadyPod = &pod
				}
				continue
			}

			// Check container readiness
			allContainersReady := true
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					allContainersReady = false
					break
				}
			}

			if allContainersReady {
				// Return the pod if it is ready
				return true, &pod, nil
			} else {
				// Return the first not ready pod found
				if notReadyPod == nil {
					notReadyPod = &pod
				}
			}
		}
	}

	// Return false if no ready pod was found, and the first not ready pod (if any)
	return false, nil, notReadyPod
}

// GetPodList lists pods associated with the provided RAC specification selector.
func GetPodList(sfsetName string, instance *racdb.RacDatabase, kClient client.Client, oraRacSpex racdb.RacInstDetailSpec,
) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	//labelSelector := labels.SelectorFromSet(getlabelsForRAC(instance))
	//labelSelector := map[string]labels.Selector{}
	var labelSelector labels.Selector = labels.SelectorFromSet(getSvcLabelsForRac(-1, oraRacSpex))

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), podList, listOps)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

// checkPod refreshes pod information from the cluster ensuring it exists.
func checkPod(instance *racdb.RacDatabase, pod *corev1.Pod, kClient client.Client,
) error {
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      pod.Name,
		Namespace: instance.Namespace,
	}, pod)

	if err != nil {
		// Pod Doesn't exist
		return err
	}

	return nil
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
			return fmt.Errorf(msg)
		}
	}
	return nil
}

// checkContainerStatus verifies at least one container in the pod is running.
func checkContainerStatus(pod *corev1.Pod, kClient client.Client,
) error {

	var statuses []corev1.ContainerStatus
	var msg string
	//	msg = "Inside the function checkContainerStatus"
	//	LogMessages("DEBUG", msg)
	statuses = pod.Status.ContainerStatuses
	var isRunning bool = false
	for _, status := range statuses {
		if status.State.Running == nil {
			isRunning = false
		} else {
			isRunning = true
			break
		}
	}
	msg = "Container is not in running state" + pod.Name + ".Describe the pod to check the detailed message"
	if isRunning {
		return nil
	} else {
		return fmt.Errorf(msg)
	}
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

	var ownerRef []metav1.OwnerReference
	ownerRef = append(ownerRef, metav1.OwnerReference{Kind: instance.GroupVersionKind().Kind, APIVersion: instance.APIVersion, Name: instance.Name, UID: types.UID(instance.UID)})
	return ownerRef
}

// getRacInitContainerCmd builds the init container shell script for RAC networking setup.
func getRacInitContainerCmd(resType string, name string, oraScriptMount string, intf1 string, intf2 string) string {

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
	return "[" + pstr + "]"
}

// getClusterState queries the cluster health for the specified pod.
func getClusterState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getGiHealthCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getDbInstState returns the database state for the given RAC pod.
func getDbInstState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getRacDbModeCmd(), kubeClient, kubeConfig, instance, logger)
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
	var diskGroups []racdb.AsmDiskGroupStatus
	diskGroup := GetAsmDiskgroup(podName, instance, specidx, kubeClient, kubeConfig, logger)
	if diskGroup == "Pending" || diskGroup == "" {
		return diskGroups // return empty slice if pending or absent
	}

	dglist := strings.Split(diskGroup, ",")
	for _, dg := range dglist {
		asmdg := racdb.AsmDiskGroupStatus{}
		asmdg.Name = strings.TrimSpace(dg)
		asmdg.Disks = stringsToAsmDiskStatus(
			getAsmDisks(podName, dg, instance, specidx, kubeClient, kubeConfig, logger),
		)
		asmdg.Redundancy = getAsmDgRedundancy(podName, dg, instance, specidx, kubeClient, kubeConfig, logger)
		// Optionally fill other fields (Type, AutoUpdate, StorageClass) if available in spec:
		for _, specDG := range instance.Spec.AsmStorageDetails {
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

	var diskGroups []racdb.AsmDiskGroupStatus

	diskGroup := GetAsmDiskgroup(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
	if diskGroup == "Pending" || diskGroup == "" {
		return diskGroups
	}

	dglist := strings.Split(diskGroup, ",")

	for _, dg := range dglist {
		asmdg := racdb.AsmDiskGroupStatus{}
		asmdg.Name = strings.TrimSpace(dg)

		diskNames := getAsmDisks(
			podName, dg, racDatabase, specidx, kubeClient, kubeConfig, logger,
		)

		for _, disk := range diskNames {
			diskName := strings.TrimSpace(disk)

			asmdg.Disks = append(asmdg.Disks, racdb.AsmDiskStatus{
				Name:     diskName,
				SizeInGb: getExistingAsmDiskSize(racDatabase, asmdg.Name, diskName),
				Valid:    true,
			})
		}

		asmdg.Redundancy = getAsmDgRedundancy(
			podName, dg, racDatabase, specidx, kubeClient, kubeConfig, logger,
		)

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

	stdoutput, _, err := ExecCommand(podName, getAsmDiskgroupCmd(), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getAsmDisksCmd(dg), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getAsmDgRedundancyCmd(dg), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getAsmInstNameCmd(), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getAsmInstStatusCmd(), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getRacInstStateFileCmd(), kubeClient, kubeConfig, instance, logger)
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

	stdoutput, _, err := ExecCommand(podName, getRacInstStateFileCmd(), kubeClient, kubeConfig, instance, logger)
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
	stdoutput, _, err := ExecCommand(podName, getRacDbVersionCmd(), kubeClient, kubeConfig, instance, logger)
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
	stdoutput, _, err := ExecCommand(podName, getRACHealthCmd(), kubeClient, kubeConfig, instance, logger)
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
	stdoutput, _, err := ExecCommand(podName, getDbRoleCmd(), kubeClient, kubeConfig, instance, logger)
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
	stdoutput, _, err := ExecCommand(podName, getConnStrCmd(), kubeClient, kubeConfig, instance, logger)
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
	// stdoutput, _, err := ExecCommand(podName, getConnStrCmd(), kubeClient, kubeConfig, instance, logger)
	// if err != nil {
	// 	msg := "Pending"
	// 	LogMessages("DEBUG", msg, err, instance, logger)
	// 	return msg
	// }
	// return strings.TrimSpace(stdoutput)
	// Fetch the racnode-scan-lsnr service
	svc, err := kubeClient.CoreV1().Services(instance.Namespace).Get(context.TODO(), "racnode-scan-lsnr", metav1.GetOptions{})
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
	var internalIP, externalIP string

	// Extract internal and external IPs from node details
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			internalIP = addr.Address
		} else if addr.Type == corev1.NodeExternalIP {
			externalIP = addr.Address
		}
	}

	// Return external IP if it exists, otherwise return internal IP
	if externalIP != "" {
		return externalIP
	}
	return internalIP
}

// readHostsFile reads the `/etc/hosts` file from the privileged RAC pod.
func readHostsFile(podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *racdb.RacDatabase, logger logr.Logger) (string, error) {

	cmd := []string{"cat", "/etc/hosts"} // Assuming this matches the MountPath in buildContainerSpecForRac

	stdOutput, _, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Failed to read /etc/hosts from pod" + podName
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}
	return strings.TrimSpace(stdOutput), nil
}

// parseHostsContent returns the non-comment lines from an `/etc/hosts` file.
func parseHostsContent(content string) string {
	var uncommentedLines string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
			uncommentedLines = trimmedLine
		}
	}

	return uncommentedLines
}

// getSvcState reports the status of the configured database service.
func getSvcState(podName string, instance *racdb.RacDatabase, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getDBServiceStatus(instance.Status.ConfigParams.DbHome, instance.Status.ConfigParams.DbName, instance.Status.ServiceDetails.Name), kubeClient, kubeConfig, instance, logger)
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
	stdoutput, _, err := ExecCommand(podName, getPdbConnStrCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getGridHome retrieves the `GRID_HOME` environment variable from the specified pod.
func getGridHome(podName string, instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	stdoutput, _, err := ExecCommand(podName, getGridHomeCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Error retrieving GRID_HOME"
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}

	return strings.TrimSpace(stdoutput), nil
}

// GetCrsNodes categorizes CRS nodes into new, healthy, and unhealthy sets.
func GetCrsNodes(instance *racdb.RacDatabase, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kClient client.Client) (string, string, string, string, string) {

	var new_crs_nodes []string
	var new_crs_nodes_list []string
	var existing_crs_nodes_healthy []string
	var existing_crs_nodes_not_healthy []string
	var install_node string
	var install_node_flag bool

	if len(instance.Spec.InstDetails) > 0 {
		for _, oraRacSpex := range instance.Spec.InstDetails {
			if !utils.CheckStatusFlag(oraRacSpex.IsDelete) {
				sfset, _ := CheckSfset(oraRacSpex.Name, instance, kClient)
				if sfset == nil {
					new_crs_nodes = append(new_crs_nodes, ("pubhost:" + oraRacSpex.Name + "-0" + "," + "viphost:" + oraRacSpex.VipSvcName))
					new_crs_nodes_list = append(new_crs_nodes_list, oraRacSpex.Name+"-0")
					if !install_node_flag {
						install_node = oraRacSpex.Name + "-0"
						install_node_flag = true
					}
				}
			}
		}
		// Updating Racnode status

		if len(instance.Status.RacNodes) > 0 {
			for _, oraRacStatusSpex := range instance.Status.RacNodes {
				_, err := CheckSfset(strings.Split(oraRacStatusSpex.Name, "-")[0], instance, kClient)
				if err != nil {
					newRacStatus := delRacNodeStatus(instance, oraRacStatusSpex.Name)
					instance.Status.RacNodes = newRacStatus
				}
			}
			// ====Updating the Racnode status block ends here====

			/// The loop check for healthy and non healthy nodes

			for _, oraRacStatusSpex := range instance.Status.RacNodes {
				if oraRacStatusSpex.NodeDetails.ClusterState == "HEALTHY" {
					if !checkElem(existing_crs_nodes_healthy, oraRacStatusSpex.Name) {
						existing_crs_nodes_healthy = append(existing_crs_nodes_healthy, oraRacStatusSpex.Name)
					}
				} else {
					if !checkElem(existing_crs_nodes_not_healthy, oraRacStatusSpex.Name) {
						existing_crs_nodes_not_healthy = append(existing_crs_nodes_not_healthy, oraRacStatusSpex.Name)
					}
				}
			}
		}

	}
	//External for loop ends here

	// Main if condition ends here
	return strings.Join(new_crs_nodes[:], ";"), strings.Join(existing_crs_nodes_healthy, ","), strings.Join(existing_crs_nodes_not_healthy, ","), install_node, strings.Join(new_crs_nodes_list, ",")

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
	var asmDisks []string
	if instance.Spec.AsmStorageDetails != nil {
		for _, dg := range instance.Spec.AsmStorageDetails {
			asmDisks = append(asmDisks, dg.Disks...)
		}
	}
	return strings.Join(asmDisks, ",")
}

// GetScanname constructs the SCAN service name for the RAC instance.
func GetScanname(instance *racdb.RacDatabase) string {
	return instance.Spec.ScanSvcName
}

// GetSSHkey checks for presence and readiness of the RAC SSH secret.
func GetSSHkey(instance *racdb.RacDatabase, name string, kClient client.Client) (bool, bool) {

	var privKeyFlag, pubKeyFlag bool
	secretFound, err := CheckSecret(instance, name, kClient)

	if err != nil {

	} else {
		for key := range secretFound.Data {
			switch key {
			case instance.Spec.SshKeySecret.PrivKeySecretName:
				privKeyFlag = true
			case instance.Spec.SshKeySecret.PubKeySecretName:
				pubKeyFlag = true
			}
		}
	}

	return privKeyFlag, pubKeyFlag

}

// GetDbSecret determines if required database secrets exist and contain keys.
func GetDbSecret(instance *racdb.RacDatabase, name string, kClient client.Client) (bool, bool, bool) {

	var commonospassflag, commonpwdfile, pwdkeyflag bool
	secretFound, err := CheckSecret(instance, name, kClient)

	if err != nil {
		return false, false, false
	} else {
		for key := range secretFound.Data {
			switch key {
			case instance.Spec.DbSecret.PwdFileName:
				commonospassflag = true
			case instance.Spec.DbSecret.KeyFileName:
				pwdkeyflag = true
			case "pwdfile":
				commonpwdfile = true
			}
		}
	}

	return commonospassflag, pwdkeyflag, commonpwdfile

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

	err := kClient.Get(context.TODO(), types.NamespacedName{
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

	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      configMapName,
		Namespace: instance.Namespace,
	}, configMapFound)

	if err != nil {
		return configMapFound, err
	}

	return configMapFound, nil

}

// GetConfigList lists configmaps matching the RAC StatefulSet label selector.
func GetConfigList(sfsetName string, instance *racdb.RacDatabase, kClient client.Client, oraRacSpex racdb.RacInstDetailSpec,
) (*corev1.ConfigMapList, error) {
	cmapList := &corev1.ConfigMapList{}
	//labelSelector := labels.SelectorFromSet(getlabelsForRAC(instance))
	//labelSelector := map[string]labels.Selector{}
	var labelSelector labels.Selector = labels.SelectorFromSet(getSvcLabelsForRac(-1, oraRacSpex))

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), cmapList, listOps)
	if err != nil {
		return nil, err
	}
	return cmapList, nil
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
	scanner := bufio.NewScanner(strings.NewReader(data))

	searchKey := strings.ToLower(strings.TrimSpace(key))
	keyHasDot := strings.Contains(key, ".")

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		splitIndex := strings.Index(line, "=")
		if splitIndex == -1 {
			continue
		}
		fullKey := strings.TrimSpace(line[:splitIndex])
		fullKeyLower := strings.ToLower(fullKey)
		if fullKeyLower == "variables" {
			value := strings.TrimSpace(line[splitIndex+1:])

			// If the search key is exactly "variables", return the whole thing
			if searchKey == "variables=" {
				fmt.Println("Key = variables, value = " + value)
				return value, nil
			}
		}

		if keyHasDot {
			// For keys with dot, match the whole key as-is (case-insensitive)
			if fullKeyLower == searchKey {
				value := strings.TrimSpace(line[splitIndex+1:])
				fmt.Print("Key = " + fullKey + " value= " + value)
				return value, nil
			}
		} else {
			// For keys without dot, match only the last segment
			var lastKey string
			if idx := strings.LastIndex(fullKey, "."); idx != -1 {
				lastKey = fullKey[idx+1:]
			} else {
				lastKey = fullKey
			}
			lastKeyLower := strings.ToLower(strings.TrimSpace(lastKey))
			if lastKeyLower == searchKey {
				value := strings.TrimSpace(line[splitIndex+1:])
				fmt.Print("Key = " + lastKey + " value= " + value)
				return value, nil
			}
		}
	}
	return "", errors.New("the " + key + " key and value does not exist in grid responsefile. Invalid grid responsefile.")
}

// GetPrivNetDetails prepares private network details for RAC provisioning.
func GetPrivNetDetails(instance *racdb.RacDatabase, OraRacSpex racdb.RacInstDetailSpec, kClient client.Client, privnet racdb.PrivIpDetailSpec) racdb.RacNetworkDetailSpec {
	var network racdb.RacNetworkDetailSpec
	network.Interface = privnet.Interface
	network.Name = privnet.Name

	// If IP is provided, use it, otherwise leave it empty for dynamic IP assignment
	if privnet.IP != "" {
		network.IPs = []string{privnet.IP}
	} else {
		// Leave the IPs empty if dynamic IP assignment is needed
		network.IPs = []string{}
	}

	// Set the MAC address if it's available
	if privnet.Mac != "" {
		network.Mac = privnet.Mac
	}

	// Set the namespace if it's provided, otherwise use the instance's namespace
	if privnet.Namespace != "" {
		network.Namespace = privnet.Namespace
	} else {
		network.Namespace = instance.Namespace
	}

	return network
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
func GetDBLsnrEndPoints(instance *racdb.RacDatabase) (string, string, error) {

	// Settings for DB_LISTENER_PORT
	var endp string = ""
	var locallsnr string = ""

	if len(instance.Spec.InstDetails) > 0 {
		for index, _ := range instance.Spec.InstDetails {
			if !utils.CheckStatusFlag(instance.Spec.InstDetails[index].IsDelete) {
				if instance.Spec.InstDetails[index].LsnrLocalPort != nil {
					endp = endp + fmt.Sprint(*instance.Spec.InstDetails[index].LsnrLocalPort) + ","
					locallsnr = locallsnr + instance.Spec.InstDetails[index].Name + "-0" + ":" + fmt.Sprint(*instance.Spec.InstDetails[index].LsnrLocalPort) + ";"

				} else {
					if instance.Spec.InstDetails[index].LsnrTargetPort != nil {
						endp = endp + fmt.Sprint(*instance.Spec.InstDetails[index].LsnrTargetPort) + ","
						locallsnr = locallsnr + instance.Spec.InstDetails[index].Name + "-0" + ":" + fmt.Sprint(*instance.Spec.InstDetails[index].LsnrTargetPort) + ";"
					}

				}
			}
		}
		if endp != "" {
			var suffix string = ","
			if strings.HasSuffix(endp, suffix) {
				endp = endp[:len(endp)-len(suffix)]
			}
		}

		if locallsnr != "" {
			var suffix string = ";"
			if strings.HasSuffix(locallsnr, suffix) {
				locallsnr = locallsnr[:len(locallsnr)-len(suffix)]
			}
		}
	}
	// LsnrTargetPort and LsnrLocalPort are optional fields
	// if locallsnr != "" && endp != "" {
	return locallsnr, endp, nil
	// }
	// return "", "", fmt.Errorf("error occurred in generating database listener endpoints")

}

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
