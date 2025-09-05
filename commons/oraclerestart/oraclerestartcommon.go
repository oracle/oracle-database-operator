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
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	oraclerestart "github.com/oracle/oracle-database-operator/apis/database/v4"
	utils "github.com/oracle/oracle-database-operator/commons/oraclerestart/utils"

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

// Function to build the env var specification
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

func checkAbsPath(location string) bool {
	return filepath.IsAbs(location)
}

// FUnction to build the svc definition for RAC
func buildContainerPortsDef(instance *oraclerestart.OracleRestart) []corev1.ContainerPort {
	var result []corev1.ContainerPort

	/*
		if len(OraRestartSpexPorts) > 0 {
			for _, portMapping := range OraRestartSpexPorts {
				name := generatePortMapping(portMapping)
				if len(name) > 15 {
					name = name[:15]
				}
				containerPort :=
					corev1.ContainerPort{
						Protocol:      portMapping.Protocol,
						ContainerPort: portMapping.Port,
						Name:          name,
					}
				result = append(result, containerPort)
			}
		} else {
	*/

	result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraDBPort, Name: truncateName(fmt.Sprintf("%s-%d", "tcp", utils.OraDBPort))})
	result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLsnrPort, Name: truncateName(fmt.Sprintf("%s-%d", "tcp", utils.OraLsnrPort))})
	result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraSSHPort, Name: truncateName(fmt.Sprintf("%s-%d", "tcp", utils.OraSSHPort))})
	result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraLocalOnsPort, Name: truncateName(fmt.Sprintf("%s-%d", "tcp", utils.OraLocalOnsPort))})
	result = append(result, corev1.ContainerPort{Protocol: corev1.ProtocolTCP, ContainerPort: utils.OraOemPort, Name: truncateName(fmt.Sprintf("%s-%d", "tcp", utils.OraOemPort))})

	return result
}

func truncateName(name string) string {
	if len(name) > 15 {
		return name[:15]
	}
	return name
}

// FUnction to build the svc definition for RAC
func buildOracleRestartSvcPortsDef(npsvc oraclerestart.OracleRestartNodePortSvc) []corev1.ServicePort {
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

// Function to generate the Name
func generateName(base string) string {
	maxNameLength := 50
	randomLength := 5
	maxGeneratedLength := maxNameLength - randomLength
	if len(base) > maxGeneratedLength {
		base = base[:maxGeneratedLength]
	}
	return fmt.Sprintf("%s%s", base, rand.String(randomLength))
}

// Function to generate the port mapping
func generatePortMapping(portMapping oraclerestart.OracleRestartPortMapping) string {
	return generateName(fmt.Sprintf("%s-%d-%d-", "tcp",
		portMapping.Port, portMapping.TargetPort))
}

func LogMessages(msgtype string, msg string, err error, instance *oraclerestart.OracleRestart, logger logr.Logger) {
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

func GetRacPodName(racName string) string {
	podName := racName
	return podName
}

func getlabelsForRac(instance *oraclerestart.OracleRestart) map[string]string {
	return buildLabelsForOracleRestart(instance, "OracleRestart")
}

func getAsmPvcName(index int, name string) string {

	pvcName := "asm-pvc-disk-" + strconv.Itoa(index) + "-" + name

	return pvcName

}
func GetAsmPvcName(index int, name string) string {

	pvcName := "asm-pvc-disk-" + strconv.Itoa(index) + "-" + name

	return pvcName

}

func getAsmPvName(index int, name string) string {

	pvName := "asm-pv-disk-" + strconv.Itoa(index) + "-" + name
	return pvName
}

func GetAsmPvName(index int, name string) string {

	pvName := "asm-pv-disk-" + strconv.Itoa(index) + "-" + name
	return pvName
}

func CheckSfset(sfsetName string, instance *oraclerestart.OracleRestart, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfSetFound := &appsv1.StatefulSet{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      sfsetName,
		Namespace: instance.Namespace,
	}, sfSetFound)
	if err != nil {
		return sfSetFound, err
	}
	return sfSetFound, nil
}

func GetRacK8sClientConfig(kClient client.Client) (clientcmd.ClientConfig, kubernetes.Interface, error) {
	var err1 error
	var kubeConfig clientcmd.ClientConfig
	var kubeClient kubernetes.Interface

	oraclerestart.KubeConfigOnce.Do(func() {
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

func oraclerestartValidationCmd() []string {

	oraScriptMount1 := getOraScriptMount()
	var oraShardValidateCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--checkliveness=true ", "--optype=primaryoraclerestart"}
	return oraShardValidateCmd
}

func OracleRestartNodeDelCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	var oraOracleRestartNodeDelCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--delOracleRestartNode=\"del_rachome=true;del_gridnode=true\""}
	return oraOracleRestartNodeDelCmd
}

func oraclerestartLsnrSetup() []string {
	oraScriptMount1 := getOraScriptMount()
	var oraOracleRestartNodeDelCmd = []string{oraScriptMount1 + "/cmdExec", "/bin/python3", oraScriptMount1 + "/main.py ", "--setupdblsnr=\"del_rachome=true;del_gridnode=true\""}
	return oraOracleRestartNodeDelCmd
}

func getAsmCmd() []string {
	asmCmd := []string{"bash", "-c", "cat /etc/rac_env_vars/envfile | grep CRS_ASM_DEVICE_LIST"}
	return asmCmd
}

func getDbAsmCmd() []string {
	asmCmd := []string{"bash", "-c", "cat /etc/rac_env_vars/envfile | grep DB_ASM_DEVICE_LIST"}
	return asmCmd
}

func getOraScriptMount() string {

	return utils.OraScriptMount

}

func getOraDbUser() string {

	return utils.OraDBUser

}

func getOraGiUser() string {

	return utils.OraGridUser

}

func getOraPythonCmd() string {

	return "/bin/python3"

}

func UpdateAsmCount(gihome string, podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, getUpdateAsmCount(gihome), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while updating TCP listener ports")
	}

	return nil
}

func ValidateDbSetup(podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, oraclerestartValidationCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while validating the DB Setup")
	}
	return nil
}

func DelOracleRestartNode(podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, OracleRestartNodeDelCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while deleting the RAC node")
	}
	return nil
}

func CheckAsmList(podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
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

func CheckDbAsmList(podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
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

func checkPvc(pvcName string, instance *oraclerestart.OracleRestart, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
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

func checkPv(pvName string, instance *oraclerestart.OracleRestart, kClient client.Client) (*corev1.PersistentVolume, error) {
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
func DelORestartPVC(instance *oraclerestart.OracleRestart, index int, diskName string, disk *oraclerestart.AsmDiskDetails, kClient client.Client, logger logr.Logger) error {
	pvcName := getAsmPvcName(index, instance.Name)
	LogMessages("DEBUG", "Attempting to delete PVC: "+GetFmtStr(pvcName), nil, instance, logger)

	pvc, err := checkPvc(pvcName, instance, kClient)
	if err != nil {
		if apierrors.IsNotFound(err) {
			LogMessages("DEBUG", "PVC not found, skipping deletion.", nil, instance, logger)
			return nil
		}
		return err
	}

	// Remove finalizers if any
	if len(pvc.GetFinalizers()) > 0 {
		LogMessages("DEBUG", "Removing PVC finalizers", nil, instance, logger)
		pvc.SetFinalizers([]string{})
		if err := kClient.Update(context.Background(), pvc); err != nil {
			return fmt.Errorf("failed to remove finalizers from PVC %s: %v", pvcName, err)
		}
	}

	if err := kClient.Delete(context.Background(), pvc); err != nil {
		return fmt.Errorf("failed to delete PVC %s: %v", pvcName, err)
	}
	return nil
}

func DelRestartSwPvc(instance *oraclerestart.OracleRestart, OraRestartSpex oraclerestart.OracleRestartInstDetailSpec, kClient client.Client, logger logr.Logger) error {

	pvcName := OraRestartSpex.Name + "-oradata-sw-vol-pvc-" + OraRestartSpex.Name + "-0"
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

func DelORestartPv(instance *oraclerestart.OracleRestart, index int, diskName string, disk *oraclerestart.AsmDiskDetails, kClient client.Client, logger logr.Logger) error {

	pvName := getAsmPvName(index, instance.Name)
	LogMessages("DEBUG", "Inside the delPv and received param: "+GetFmtStr(pvName), nil, instance, logger)
	pvFound, err := checkPv(pvName, instance, kClient)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in finding the pv claim!", nil, instance, logger)
		return nil
	}
	err = kClient.Delete(context.Background(), pvFound)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in deleting the pv claim!", nil, instance, logger)
		return err
	}
	return nil
}

func CheckORestartSvc(instance *oraclerestart.OracleRestart, svcType string, OraRestartSpex oraclerestart.OracleRestartInstDetailSpec, svcName string, kClient client.Client) (*corev1.Service, error) {
	svcFound := &corev1.Service{}
	var name string

	name = getOracleRestartSvcName(instance, OraRestartSpex, svcType)

	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: instance.Namespace,
	}, svcFound)
	if err != nil {
		return svcFound, err
	}
	return svcFound, nil
}

func PodListValidation(podList *corev1.PodList, sfName string, instance *oraclerestart.OracleRestart, kClient client.Client) (bool, *corev1.Pod, *corev1.Pod) {
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

func GetPodList(sfsetName string, instance *oraclerestart.OracleRestart, kClient client.Client, OraRestartSpex oraclerestart.OracleRestartInstDetailSpec,
) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	//labelSelector := labels.SelectorFromSet(getlabelsForRAC(instance))
	//labelSelector := map[string]labels.Selector{}
	var labelSelector labels.Selector = labels.SelectorFromSet(getSvcLabelsForOracleRestart(-1, OraRestartSpex))

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), podList, listOps)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

func checkPod(instance *oraclerestart.OracleRestart, pod *corev1.Pod, kClient client.Client,
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

func getOwnerRef(instance *oraclerestart.OracleRestart,
) []metav1.OwnerReference {

	var ownerRef []metav1.OwnerReference
	ownerRef = append(ownerRef, metav1.OwnerReference{Kind: instance.GroupVersionKind().Kind, APIVersion: instance.APIVersion, Name: instance.Name, UID: types.UID(instance.UID)})
	return ownerRef
}

func getRacInitContainerCmd(resType string, name string, oraScriptMount string,
) string {
	var initCmd string
	if oraScriptMount != "NOLOC" {
		initCmd = resType + ";chown -R 54321:54321 " + oraScriptMount + ";chmod 755 " + oraScriptMount + "/*"
	} else {
		initCmd = resType
	}

	return initCmd
}

func GetFmtStr(pstr string,
) string {
	return "[" + pstr + "]"
}

func getClusterState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getGiHealthCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getDbInstState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getOracleRestartDbModeCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getAsmInstState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) *oraclerestart.AsmInstanceStatus {
	AsmStorageStatus := &oraclerestart.AsmInstanceStatus{}
	diskGroup := getAsmDiskgroup(podName, instance, specidx, kubeClient, kubeConfig, logger)
	if diskGroup == "Pending" {
		return AsmStorageStatus
	}
	dglist := strings.Split(diskGroup, ",")
	for _, dg := range dglist {
		asmdg := oraclerestart.AsmDiskgroupStatus{}
		disks := getAsmDisks(podName, string(dg), instance, specidx, kubeClient, kubeConfig, logger)
		redundancy := getAsmDgRedundancy(podName, string(dg), instance, specidx, kubeClient, kubeConfig, logger)

		asmdg.Name = string(dg)
		asmdg.Disks = disks
		asmdg.Redundancy = redundancy
		AsmStorageStatus.Diskgroup = append(AsmStorageStatus.Diskgroup, asmdg)
	}
	return AsmStorageStatus
}
func GetAsmInstState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) *oraclerestart.AsmInstanceStatus {
	AsmStorageStatus := &oraclerestart.AsmInstanceStatus{}
	diskGroup := getAsmDiskgroup(podName, instance, specidx, kubeClient, kubeConfig, logger)
	if diskGroup == "Pending" {
		return AsmStorageStatus
	}
	dglist := strings.Split(diskGroup, ",")
	for _, dg := range dglist {
		asmdg := oraclerestart.AsmDiskgroupStatus{}
		disks := getAsmDisks(podName, string(dg), instance, specidx, kubeClient, kubeConfig, logger)
		redundancy := getAsmDgRedundancy(podName, string(dg), instance, specidx, kubeClient, kubeConfig, logger)

		asmdg.Name = string(dg)
		asmdg.Disks = disks
		asmdg.Redundancy = redundancy
		AsmStorageStatus.Diskgroup = append(AsmStorageStatus.Diskgroup, asmdg)
	}
	return AsmStorageStatus
}

func getAsmDiskgroup(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmDiskgroupCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getAsmDisks(podName string, dg string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) []string {

	stdoutput, _, err := ExecCommand(podName, getAsmDisksCmd(dg), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return strings.Split(msg, ",")
	}
	cleanOutput := strings.ReplaceAll(stdoutput, "\r", "")
	return strings.Split(strings.TrimSpace(cleanOutput), "\n")
}

func getAsmDgRedundancy(podName string, dg string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmDgRedundancyCmd(dg), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getAsmInstName(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmInstNameCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getAsmInstStatus(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getAsmInstStatusCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getOracleRestartInstStateFile(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getOracleRestartInstStateFileCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	//Possible values are "provisioning", "addnode", "failed", "completed"
	if strings.ToLower(strings.TrimSpace(stdoutput)) == "provisioning" {
		return string(oraclerestart.OracleRestartProvisionState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "completed" {
		return string(oraclerestart.OracleRestartAvailableState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "addnode" {
		return string(oraclerestart.OracleRestartAddInstState)
	} else if strings.ToLower(strings.TrimSpace(stdoutput)) == "failed" {
		return string(oraclerestart.OracleRestartFailedState)
	} else {
		return string(oraclerestart.OracleRestartPendingState)
	}
}

func getDBVersion(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getOracleRestartDbVersionCmd(), kubeClient, kubeConfig, instance, logger)
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

func getDbState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getOracleRestartHealthCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getDbRole(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getDbRoleCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getConnStr(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getConnStrCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getExternalConnStr(
	podName string,
	instance *oraclerestart.OracleRestart,
	specidx int,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) string {
	switch {
	// Case 1: Neither service defined
	case len(instance.Spec.NodePortSvc.PortMappings) == 0 && len(instance.Spec.LbService.PortMappings) == 0:
		return ""

	// Case 2: LoadBalancer service defined, try to use it
	case len(instance.Spec.LbService.PortMappings) != 0:
		lbServiceName := instance.Spec.LbService.SvcName
		lbSvc, err := kubeClient.CoreV1().Services(instance.Namespace).Get(context.TODO(), lbServiceName, metav1.GetOptions{})
		lbExtIP := ""
		var lbPort int32
		if err == nil && lbSvc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			// Extract external IP or hostname
			for _, ingress := range lbSvc.Status.LoadBalancer.Ingress {
				if ingress.IP != "" {
					lbExtIP = ingress.IP
					break
				} else if ingress.Hostname != "" {
					lbExtIP = ingress.Hostname
					break
				}
			}
			// Find port 1521
			for _, port := range lbSvc.Spec.Ports {
				if port.Port == 1521 {
					lbPort = port.Port
					break
				}
			}
		}
		// Prefer LoadBalancer if available
		if lbExtIP != "" && lbPort != 0 {
			serviceName := instance.Spec.ServiceDetails.Name
			return fmt.Sprintf("EXTERNAL: %s:%d/%s", lbExtIP, lbPort, serviceName)
		}
		// fallthrough to NodePort if LB unavailable
		fallthrough

	// Case 3: NodePort defined or fallback
	default:
		svc, err := kubeClient.CoreV1().Services(instance.Namespace).Get(context.TODO(), instance.Spec.InstDetails.Name, metav1.GetOptions{})
		if err != nil {
			msg := "Failed to get dbmc1 service"
			LogMessages("DEBUG", msg, err, instance, logger)
			return "Pending"
		}

		// Find NodePort for 1521
		var nodePort int32
		for _, port := range svc.Spec.Ports {
			if port.Port == 1521 {
				nodePort = port.NodePort
				break
			}
		}
		if nodePort == 0 {
			msg := "Failed to find NodePort for port 1521 in dbmc1 service"
			LogMessages("DEBUG", msg, err, instance, logger)
			return "Pending"
		}

		// Get first node external/internal IP
		nodeList, err := kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil || len(nodeList.Items) == 0 {
			msg := "Failed to list cluster nodes"
			LogMessages("DEBUG", msg, err, instance, logger)
			return "Pending"
		}
		var nodeIP string
		for _, addr := range nodeList.Items[0].Status.Addresses {
			if addr.Type == corev1.NodeExternalIP {
				nodeIP = addr.Address
				break
			} else if addr.Type == corev1.NodeInternalIP && nodeIP == "" {
				nodeIP = addr.Address
			}
		}
		if nodeIP == "" {
			msg := "Failed to get node IP address"
			LogMessages("DEBUG", msg, err, instance, logger)
			return "Pending"
		}

		serviceName := instance.Spec.ServiceDetails.Name
		return fmt.Sprintf("%s:%d/%s", nodeIP, nodePort, serviceName)
	}
}

func getClientEtcHost(podNames []string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger, nodeDetails map[string]*corev1.Node) []string {
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
		line := fmt.Sprintf("%s    %s.rac.svc.cluster.local %s-vip.rac.svc.cluster.local    OracleRestartNode-scan.rac.svc.cluster.local", nodeIP, podName, podName)
		clientEtcHost = append(clientEtcHost, line)
	}

	// Assuming you are returning clientEtcHost as well for any further processing
	return clientEtcHost
}

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

// readHostsFile reads the /etc/hosts file from the pod running in privileged mode
func readHostsFile(podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *oraclerestart.OracleRestart, logger logr.Logger) (string, error) {

	cmd := []string{"cat", "/etc/hosts"} // Assuming this matches the MountPath in buildContainerSpecForRac

	stdOutput, _, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Failed to read /etc/hosts from pod" + podName
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}
	return strings.TrimSpace(stdOutput), nil
}

// Helper function to parse /etc/hosts content
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

func getSvcState(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {

	stdoutput, _, err := ExecCommand(podName, getDBServiceStatus(instance.Status.ConfigParams.DbHome, instance.Status.ConfigParams.DbName, instance.Status.ServiceDetails.Name), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func getPdbConnStr(podName string, instance *oraclerestart.OracleRestart, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(podName, getPdbConnStrCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Pending"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

// getGridHome retrieves the GRID_HOME environment variable from the specified pod
func getGridHome(podName string, instance *oraclerestart.OracleRestart, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) (string, error) {
	stdoutput, _, err := ExecCommand(podName, getGridHomeCmd(), kubeClient, kubeConfig, instance, logger)
	if err != nil {
		msg := "Error retrieving GRID_HOME"
		LogMessages("DEBUG", msg, err, instance, logger)
		return "", err
	}

	return strings.TrimSpace(stdoutput), nil
}

func GetAsmDevices(instance *oraclerestart.OracleRestart) string {
	var asmDisk []string
	if instance.Spec.AsmStorageDetails != nil {
		// Loop through each DiskBySize and add disk names to asmDisk
		for _, diskBySize := range instance.Spec.AsmStorageDetails.DisksBySize {
			asmDisk = append(asmDisk, diskBySize.DiskNames...)
		}
	}
	return strings.Join(asmDisk, ",")
}

// func GetScanname(instance *oraclerestart.OracleRestart) string {
// 	return instance.Spec.ScanSvcName
// }

func GetSSHkey(instance *oraclerestart.OracleRestart, name string, kClient client.Client) (bool, bool) {

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

func GetDbSecret(instance *oraclerestart.OracleRestart, name string, kClient client.Client) (bool, bool, bool) {

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

func GetTdeWalletSecret(instance *oraclerestart.OracleRestart, name string, kClient client.Client) (bool, bool, bool) {

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

func CheckSecret(instance *oraclerestart.OracleRestart, secretName string, kClient client.Client) (*corev1.Secret, error) {

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

func GetGiResponseFile(instance *oraclerestart.OracleRestart, kClient client.Client) (bool, map[string]string) {

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

func GetDbResponseFile(instance *oraclerestart.OracleRestart, kClient client.Client) (bool, map[string]string) {

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

func CheckConfigMap(instance *oraclerestart.OracleRestart, configMapName string, kClient client.Client) (*corev1.ConfigMap, error) {

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

func GetConfigList(sfsetName string, instance *oraclerestart.OracleRestart, kClient client.Client, OraRestartSpex oraclerestart.OracleRestartInstDetailSpec,
) (*corev1.ConfigMapList, error) {
	cmapList := &corev1.ConfigMapList{}
	//labelSelector := labels.SelectorFromSet(getlabelsForRAC(instance))
	//labelSelector := map[string]labels.Selector{}
	var labelSelector labels.Selector = labels.SelectorFromSet(getSvcLabelsForOracleRestart(-1, OraRestartSpex))

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), cmapList, listOps)
	if err != nil {
		return nil, err
	}
	return cmapList, nil
}

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

func ValidateNetInterface(net string, instance *oraclerestart.OracleRestart, rspNetData string) error {

	var err error

	if net != "" {
		if !strings.Contains(rspNetData, net) {
			err = fmt.Errorf("error occurred during retreiving network card detail from grid responsefile: %s", "The key does not exist")
		}
	}

	return err
}

func CheckRspData(instance *oraclerestart.OracleRestart, kClient client.Client, key string, cName string, fname string) (string, error) {

	var rspData string
	configMapFound, _ := CheckConfigMap(instance, cName, kClient)
	var status bool
	var value []string

	data := configMapFound.Data[fname]

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), strings.ToLower(key)) {
			if strings.ToLower(key) == "variables=" {
				str1 := strings.Replace(scanner.Text(), "=", ";", 1)
				value = strings.Split(str1, ";")
			} else {
				value = strings.Split(scanner.Text(), "=")
			}
			rspData = value[1]
			fmt.Print("Key = " + value[0] + " value= " + value[1])
			status = true
			break
		}
	}

	if !status {
		return "", errors.New("the " + key + " key and value does not exist in grid responsefile. Invalid grid responsefile.")
	}

	return rspData, nil
}

func GetServiceParams(instance *oraclerestart.OracleRestart) string {
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

func GetHealthyNode(instance *oraclerestart.OracleRestart) (string, error) {
	var i int32

	if len(instance.Status.OracleRestartNodes) > 0 {
		for i = 0; i < int32(len(instance.Status.OracleRestartNodes)); i++ {
			if instance.Status.OracleRestartNodes[i].NodeDetails != nil {
				if instance.Status.OracleRestartNodes[i].NodeDetails.ClusterState == "HEALTHY" {
					return instance.Status.OracleRestartNodes[i].Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no healthy node exist")
}

func GetHealthyNodeCounts(instance *oraclerestart.OracleRestart) (int, error) {
	var i, totalNodes, healthyNodeCount int

	totalNodes = len(instance.Status.OracleRestartNodes)

	if totalNodes > 0 {
		for i = 0; i < totalNodes; i++ {
			if instance.Status.OracleRestartNodes[i].NodeDetails != nil {
				if instance.Status.OracleRestartNodes[i].NodeDetails.ClusterState == "HEALTHY" {
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
