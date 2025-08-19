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
	"strings"

	"github.com/go-logr/logr"
	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func UpdateOracleRestartInstStatusData(
	OracleRestart *oraclerestartdb.OracleRestart,
	ctx context.Context,
	req ctrl.Request,
	oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec,
	state string,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	kClient client.Client,
) {
	// podName is constructed based on oraRestartSpex.Name
	podName := oraRestartSpex.Name + "-0"

	oracleRestart := &oraclerestartdb.OracleRestartNodestatus{}
	orestartNodeDetails := &oraclerestartdb.OracleRestartNodeDetailedStatus{}
	strMap := make(map[string]string)

	if OracleRestart.Status.AsmDetails == nil {
		OracleRestart.Status.AsmDetails = &oraclerestartdb.AsmInstanceStatus{}
	}

	if OracleRestart.Status.ConfigParams == nil {
		OracleRestart.Status.ConfigParams = &oraclerestartdb.InitParams{}
	}

	if state == string(oraclerestartdb.OracleRestartUpdateState) {
		OracleRestart.Status.State = state
	}
	if state == string(oraclerestartdb.OracleRestartProvisionState) {
		OracleRestart.Status.State = state
	}
	if state == string(oraclerestartdb.OracleRestartFailedState) {
		OracleRestart.Status.State = state
	}
	if state == string(oraclerestartdb.OracleRestartManualState) {
		OracleRestart.Status.State = state
	}
	if state == string(oraclerestartdb.OracleRestartAvailableState) {
		OracleRestart.Status.State = state
		orestartNodeDetails.ClusterState = getClusterState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
		orestartNodeDetails.PodState = state
		orestartNodeDetails.State = "OPEN"
		oracleRestart.Name = podName

		orestartNodeDetails.InstanceState = getDbInstState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
		orestartNodeDetails.MountedDevices = getMountedDevices(podName, OracleRestart.Namespace, oracleRestart, oraRestartSpex, kClient, kubeConfig, logger, kubeClient)
		oracleRestart.NodeDetails = orestartNodeDetails
		addOracleRestartNodestatus(OracleRestart, ctx, req, oracleRestart, oraRestartSpex, 0, kubeClient, kubeConfig, logger)

		if len(oraRestartSpex.PvcName) > 0 {
			orestartNodeDetails.PvcName = getPvcDetails(OracleRestart, oracleRestart, oraRestartSpex, kClient)
		}
	}

	// Update status based on the state
	if state == string(oraclerestartdb.OracleRestartPodAvailableState) {
		OracleRestart.Status.State = state
		orestartNodeDetails.ClusterState = getClusterState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
		orestartNodeDetails.PodState = state
		orestartNodeDetails.State = "OPEN"
		oracleRestart.Name = podName

		// orestartNodeDetails.VipDetails = getVipDetails(OracleRestart, oracleRestart, oraRestartSpex, kClient)
		orestartNodeDetails.InstanceState = getDbInstState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
		orestartNodeDetails.MountedDevices = getMountedDevices(podName, OracleRestart.Namespace, oracleRestart, oraRestartSpex, kClient, kubeConfig, logger, kubeClient)
		oracleRestart.NodeDetails = orestartNodeDetails
		OracleRestart.Status.ReleaseUpdate = "NOTAVAILABLE"
		addOracleRestartNodestatus(OracleRestart, ctx, req, oracleRestart, oraRestartSpex, 0, kubeClient, kubeConfig, logger)

		if len(oraRestartSpex.PvcName) > 0 {
			orestartNodeDetails.PvcName = getPvcDetails(OracleRestart, oracleRestart, oraRestartSpex, kClient)
		}
		if OracleRestart.Spec.ConfigParams.GridHome != "" {
			OracleRestart.Status.ConfigParams.GridHome = OracleRestart.Spec.ConfigParams.GridHome
		}
		if OracleRestart.Spec.ConfigParams.DbHome != "" {
			OracleRestart.Status.ConfigParams.DbHome = OracleRestart.Spec.ConfigParams.DbHome
		}
		// OracleRestart.Status.ConfigParams.CrsAsmDeviceList = OracleRestart.Spec.ConfigParams.CrsAsmDeviceList
		OracleRestart.Status.ConfigParams.CrsAsmDeviceList = getcrsAsmDeviceList(OracleRestart, oracleRestart, oraRestartSpex, kClient, kubeConfig, logger, kubeClient)

		// OracleRestart.Status.ConfigParams.DbAsmDeviceList = OracleRestart.Spec.ConfigParams.DbAsmDeviceList
		OracleRestart.Status.ConfigParams.DbAsmDeviceList = getdbAsmDeviceList(OracleRestart, oracleRestart, oraRestartSpex, kClient, kubeConfig, logger, kubeClient)

	} else if state == string(oraclerestartdb.OracleRestartStatefulSetNotFound) {
		neworacleRestart := delOracleRestartNodestatus(OracleRestart, oraRestartSpex.Name+"-0")
		OracleRestart.Status.OracleRestartNodes = neworacleRestart
		OracleRestart.Status.ReleaseUpdate = "NOTAVAILABLE"

	} else if state == string(oraclerestartdb.PodNotFound) || state == string(oraclerestartdb.PodNotReadyState) || state == string(oraclerestartdb.PodFailureState) {
		orestartNodeDetails.ClusterState = "NOTAVAILABLE"
		orestartNodeDetails.PodState = state
		// orestartNodeDetails.VipDetails = getVipDetails(OracleRestart, oracleRestart, oraRestartSpex, kClient)
		orestartNodeDetails.InstanceState = "NOTAVAILABLE"
		orestartNodeDetails.PvcName = strMap
		OracleRestart.Status.ReleaseUpdate = "NOTAVAILABLE"
	}
}

func addOracleRestartNodestatus(instance *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, oracleRestart *oraclerestartdb.OracleRestartNodestatus, oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, idx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {

	var racState string
	podName := oraRestartSpex.Name + "-0"
	idx, status := contains(instance, oracleRestart)
	if status {
		// racstate need to be read before overwriting the instance.Status.OracleRestartNodes[idx] = oracleRestart
		racState = instance.Status.OracleRestartNodes[idx].NodeDetails.State
		instance.Status.OracleRestartNodes[idx] = oracleRestart
		instState := instance.Status.OracleRestartNodes[idx].NodeDetails.InstanceState
		if instState == "OPEN" {
			instance.Status.OracleRestartNodes[idx].NodeDetails.State = "AVAILABLE"
		} else {
			if (racState == "PENDING") || (racState == "ADDNODE") || (racState == "PROVISIONING") || (racState == "FAILED") || (racState == "UPDATE") {
				instance.Status.OracleRestartNodes[idx].NodeDetails.State = getOracleRestartInstStateFile(podName, instance, 0, kubeClient, kubeConfig, logger)
			} else {
				instance.Status.OracleRestartNodes[idx].NodeDetails.State = "PENDING"
			}

		}

	} else {
		instance.Status.OracleRestartNodes = append(instance.Status.OracleRestartNodes, oracleRestart)
		instance.Status.OracleRestartNodes[idx].NodeDetails.State = "PENDING"
	}

}

func UpdateOracleRestartInstState(instance *oraclerestartdb.OracleRestart, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {

	// if len(instance.Status.OracleRestartNodes) > 0 {
	//	for _, v := range instance.Status.OracleRestartNodes {
	//        if v.NodeDetails.
	//	}
	//}

}

func UpdateoraclerestartdbTopologyState(instance *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {
	OracleRestart := &oraclerestartdb.OracleRestart{}
	if len(instance.Status.OracleRestartNodes) > 0 {
		state := map[string]struct{}{}
		for _, v := range instance.Status.OracleRestartNodes {
			inst_state := strings.ToLower(strings.TrimSpace(v.NodeDetails.State))
			state[inst_state] = struct{}{}
		}
		if len(state) == 0 {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartPendingState)
		} else if _, curr_state := state["failed"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartFailedState)
		} else if _, curr_state := state["pending"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartPendingState)
		} else if _, curr_state := state["provisioning"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartProvisionState)
		} else if _, curr_state := state["update"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartUpdateState)
		} else if _, curr_state := state["addnode"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartAddInstState)
		} else if _, curr_state := state["podavailable"]; curr_state {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartPodAvailableState)
		} else {
			OracleRestart.Status.State = string(oraclerestartdb.OracleRestartAvailableState)
		}
		instance.Status.State = OracleRestart.Status.State
	}

}

func UpdateoraclerestartdbStatusData(OracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podNames []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger, nodeDetails map[string]*corev1.Node,
) {
	//mode := GetDbOpenMode(instance.Spec.Shard[0].Name+"-0", instance, kubeClient, kubeConfig, logger)
	podName := podNames[len(podNames)-1]
	OracleRestart.Status.DbState = getDbState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.Role = getDbRole(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.ReleaseUpdate = getDBVersion(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.ConnectString = getConnStr(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.PdbConnectString = getPdbConnStr(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.ExternalConnectString = getExternalConnStr(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)
	OracleRestart.Status.DbSecret = OracleRestart.Spec.DbSecret
	OracleRestart.Status.AsmDetails = getAsmInstState(podName, OracleRestart, 0, kubeClient, kubeConfig, logger)

	UpdateoraclerestartdbServiceStatus(OracleRestart, ctx, req, podName, kubeClient, kubeConfig, logger)
	UpdateoraclerestartdbTopologyState(OracleRestart, ctx, req, podName, kubeClient, kubeConfig, logger)
}

func UpdateoraclerestartdbServiceStatus(instance *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	//This function update the instance.Status.ServiceDetails states
	if instance.Spec.ServiceDetails.Name != "" {
		instance.Status.ServiceDetails.Name = instance.Spec.ServiceDetails.Name
		instance.Status.ServiceDetails.SvcState = getSvcState(podName, instance, 0, kubeClient, kubeConfig, logger)
	}
}

func contains(instance *oraclerestartdb.OracleRestart, oracleRestart *oraclerestartdb.OracleRestartNodestatus) (int, bool) {
	var index int
	if len(instance.Status.OracleRestartNodes) > 0 {
		for index, v := range instance.Status.OracleRestartNodes {
			if v.Name == oracleRestart.Name {
				return index, true
			}
		}
	}

	return index, false
}

func getcrsAsmDeviceList(instance *oraclerestartdb.OracleRestart, oracleRestart *oraclerestartdb.OracleRestartNodestatus, oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) string {
	asmList := ""
	var err error
	if len(instance.Status.OracleRestartNodes) > 0 {
		asmList, err = CheckAsmList(instance.Status.OracleRestartNodes[0].Name, instance, kubeClient, kubeConfig, logger)
		if err != nil {
			return ""
		}

	}

	return asmList

}
func getdbAsmDeviceList(instance *oraclerestartdb.OracleRestart, oracleRestart *oraclerestartdb.OracleRestartNodestatus, oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) string {
	dbasmList := ""
	var err error
	if len(instance.Status.OracleRestartNodes) > 0 {
		dbasmList, err = CheckDbAsmList(instance.Status.OracleRestartNodes[0].Name, instance, kubeClient, kubeConfig, logger)
		if err != nil {
			return ""
		}

	}

	return dbasmList

}

func getMountedDevices(podName, namespace string, oracleRestart *oraclerestartdb.OracleRestartNodestatus, oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) []string {
	var asmList []string

	// Get the pod associated with the RAC node
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "Failed to get pod for RAC node", "PodName", podName, "Namespace", namespace)
		return nil
	}

	// Loop through the volume devices or mounted volumes in the pod spec
	for _, container := range pod.Spec.Containers {
		for _, volumeDevice := range container.VolumeDevices {
			// Append the device path to the asmList
			asmList = append(asmList, volumeDevice.DevicePath)
		}
	}

	return asmList
}

func delOracleRestartNodestatus(instance *oraclerestartdb.OracleRestart, name string) []*oraclerestartdb.OracleRestartNodestatus {
	neworacleRestart := []*oraclerestartdb.OracleRestartNodestatus{}
	if len(instance.Status.OracleRestartNodes) > 0 {
		for _, value := range instance.Status.OracleRestartNodes {
			if ((value.Name) != (name)) && (value != nil) {
				neworacleRestart = append(neworacleRestart, value)
			}
		}
	}
	return neworacleRestart
}

func getPvcDetails(instance *oraclerestartdb.OracleRestart, oracleRestart *oraclerestartdb.OracleRestartNodestatus, oraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, rclient client.Client) map[string]string {
	strMap := make(map[string]string)
	if len(oraRestartSpex.PvcName) > 0 {
		strMap = oraRestartSpex.PvcName
	}

	return strMap

}
