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
	"strings"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateRacInstStatusData provides documentation for the UpdateRacInstStatusData function.
func UpdateRacInstStatusData(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	oraRacSpex racdb.RacInstDetailSpec,
	specidx int,
	state string,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	kClient client.Client,
) {
	// podName is constructed based on oraRacSpex.Name
	podName := oraRacSpex.Name + "-0"

	racStatus := &racdb.RacNodeStatus{}
	racNodeDetails := &racdb.RacNodeDetailedStatus{}
	strMap := make(map[string]string)

	if racDatabase.Status.AsmDiskGroups == nil {
		racDatabase.Status.AsmDiskGroups = []racdb.AsmDiskGroupStatus{}
	}
	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = &racdb.RacInitParams{}
	}

	if state == string(racdb.RACUpdateState) {
		racDatabase.Status.State = state
	}
	if state == string(racdb.RACProvisionState) {
		racDatabase.Status.State = state
	}
	if state == string(racdb.RACFailedState) {
		clusterState := getClusterState(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
		instanceState := getDbInstState(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
		if clusterState == "HEALTHY" && instanceState == "OPEN" { // cluster is healthy and database is also fine
			racDatabase.Status.State = string(racdb.RACAvailableState)
			state = string(racdb.RACAvailableState)
		} else {
			racDatabase.Status.State = state
		}

	}
	if state == string(racdb.RACManualState) {
		racDatabase.Status.State = state
	}
	if state == string(racdb.RACAvailableState) {
		racDatabase.Status.State = state
		racNodeDetails.ClusterState = getClusterState(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
		racNodeDetails.PodState = state
		racNodeDetails.State = "OPEN"
		racStatus.Name = podName

		racNodeDetails.VipDetails = getVipDetails(racDatabase, racStatus, oraRacSpex, kClient)
		racNodeDetails.InstanceState = getDbInstState(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
		racNodeDetails.MountedDevices = getMountedDevices(podName, racDatabase.Namespace, racStatus, oraRacSpex, kClient, kubeConfig, logger, kubeClient)
		racStatus.NodeDetails = racNodeDetails
		addRacNodeStatus(racDatabase, ctx, req, racStatus, oraRacSpex, specidx, kubeClient, kubeConfig, logger)

		if len(oraRacSpex.PvcName) > 0 {
			racNodeDetails.PvcName = getPvcDetails(racDatabase, racStatus, oraRacSpex, kClient)
		}
	}

	// Update status based on the state
	if state == string(racdb.RACPodAvailableState) {
		racDatabase.Status.State = state
		racNodeDetails.ClusterState = getClusterState(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
		racNodeDetails.PodState = state
		racNodeDetails.State = "OPEN"
		racStatus.Name = podName

		racNodeDetails.VipDetails = getVipDetails(racDatabase, racStatus, oraRacSpex, kClient)
		racNodeDetails.InstanceState = getDbInstState(podName, racDatabase, specidx, kubeClient, kubeConfig, logger)
		racNodeDetails.MountedDevices = getMountedDevices(podName, racDatabase.Namespace, racStatus, oraRacSpex, kClient, kubeConfig, logger, kubeClient)
		racStatus.NodeDetails = racNodeDetails
		racDatabase.Status.ReleaseUpdate = "NOTAVAILABLE"
		addRacNodeStatus(racDatabase, ctx, req, racStatus, oraRacSpex, specidx, kubeClient, kubeConfig, logger)

		if len(oraRacSpex.PvcName) > 0 {
			racNodeDetails.PvcName = getPvcDetails(racDatabase, racStatus, oraRacSpex, kClient)
		}
		if racDatabase.Spec.ConfigParams.GridHome != "" {
			racDatabase.Status.ConfigParams.GridHome = racDatabase.Spec.ConfigParams.GridHome
		}
		if racDatabase.Spec.ConfigParams.DbHome != "" {
			racDatabase.Status.ConfigParams.DbHome = racDatabase.Spec.ConfigParams.DbHome
		}
		crsDeviceList := GetcrsAsmDeviceList(racDatabase, racStatus, oraRacSpex, kClient, kubeConfig, logger, kubeClient)
		dbDeviceList := GetdbAsmDeviceList(racDatabase, racStatus, oraRacSpex, kClient, kubeConfig, logger, kubeClient)
		// Store CRS device list into status
		SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.CrsAsmDiskDg, crsDeviceList)

		// Store DB device list into status
		SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.DbDataDiskDg, dbDeviceList)
	} else if state == string(racdb.RACStatefulSetNotFound) {
		newRacStatus := delRacNodeStatus(racDatabase, oraRacSpex.Name+"-0")
		racDatabase.Status.RacNodes = newRacStatus
		racDatabase.Status.ReleaseUpdate = "NOTAVAILABLE"

	} else if state == string(racdb.PodNotFound) || state == string(racdb.PodNotReadyState) || state == string(racdb.PodFailureState) {
		racNodeDetails.ClusterState = "NOTAVAILABLE"
		racNodeDetails.PodState = state
		racNodeDetails.VipDetails = getVipDetails(racDatabase, racStatus, oraRacSpex, kClient)
		racNodeDetails.InstanceState = "NOTAVAILABLE"
		racNodeDetails.PvcName = strMap
		racDatabase.Status.ReleaseUpdate = "NOTAVAILABLE"
	}
}

// UpdateRacNodeStatusDataForCluster provides documentation for the UpdateRacNodeStatusDataForCluster function.
func UpdateRacNodeStatusDataForCluster(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	state string,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	kClient client.Client,
) {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	podName := nodeName + "-0" // adjust if your pod naming differs

	racStatus := &racdb.RacNodeStatus{}
	racNodeDetails := &racdb.RacNodeDetailedStatus{}

	// Sanity checks
	if racDatabase.Status.AsmDiskGroups == nil {
		racDatabase.Status.AsmDiskGroups = []racdb.AsmDiskGroupStatus{}
	}
	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = &racdb.RacInitParams{}
	}

	racDatabase.Status.State = state

	// Update status based on state
	switch state {
	case string(racdb.RACAvailableState):
		racNodeDetails.ClusterState = getClusterState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
		racNodeDetails.PodState = state
		racNodeDetails.State = "OPEN"
		racStatus.Name = podName
		racNodeDetails.VipDetails = getVipDetailsForCluster(racDatabase, clusterSpec, nodeIndex, racStatus, kClient)
		racNodeDetails.InstanceState = getDbInstState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
		racNodeDetails.MountedDevices = getMountedDevicesForCluster(podName, racDatabase.Namespace, racStatus, nodeName, kClient, kubeConfig, logger, kubeClient)
		racStatus.NodeDetails = racNodeDetails
		addRacNodeStatusForCluster(racDatabase, ctx, req, racStatus, nodeName, nodeIndex, kubeClient, kubeConfig, logger)
	case string(racdb.RACPodAvailableState):
		racNodeDetails.ClusterState = getClusterState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
		racNodeDetails.PodState = state
		racNodeDetails.State = "OPEN"
		racStatus.Name = podName
		racNodeDetails.VipDetails = getVipDetailsForCluster(racDatabase, clusterSpec, nodeIndex, racStatus, kClient)
		racNodeDetails.InstanceState = getDbInstState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
		racNodeDetails.MountedDevices = getMountedDevicesForCluster(podName, racDatabase.Namespace, racStatus, nodeName, kClient, kubeConfig, logger, kubeClient)
		racStatus.NodeDetails = racNodeDetails
		addRacNodeStatusForCluster(racDatabase, ctx, req, racStatus, nodeName, nodeIndex, kubeClient, kubeConfig, logger)
	case string(racdb.RACStatefulSetNotFound):
		newRacStatus := delRacNodeStatus(racDatabase, nodeName+"-0")
		racDatabase.Status.RacNodes = newRacStatus
		racDatabase.Status.ReleaseUpdate = "NOTAVAILABLE"
	case string(racdb.PodNotFound), string(racdb.PodNotReadyState), string(racdb.PodFailureState):
		racNodeDetails.ClusterState = "NOTAVAILABLE"
		racNodeDetails.PodState = state
		racNodeDetails.VipDetails = getVipDetailsForCluster(racDatabase, clusterSpec, nodeIndex, racStatus, kClient)
		racNodeDetails.InstanceState = "NOTAVAILABLE"
		racNodeDetails.PvcName = map[string]string{}
		racDatabase.Status.ReleaseUpdate = "NOTAVAILABLE"
	default:
		racDatabase.Status.State = state
	}
}

// SetAsmDiskGroupDevices provides documentation for the SetAsmDiskGroupDevices function.
func SetAsmDiskGroupDevices(groups *[]racdb.AsmDiskGroupStatus, groupType racdb.AsmDiskDGTypes, deviceList string) {
	devices := strings.Split(deviceList, ",")
	var diskStatuses []racdb.AsmDiskStatus
	for _, disk := range devices {
		disk = strings.TrimSpace(disk)
		if disk != "" {
			diskStatuses = append(diskStatuses, racdb.AsmDiskStatus{Name: disk, Valid: true})
		}
	}
	// Do nothing if there are no disks
	if len(diskStatuses) == 0 {
		return
	}

	found := false
	for i, group := range *groups {
		if group.Type == groupType {
			(*groups)[i].Disks = diskStatuses
			found = true
			break
		}
	}
	if !found {
		// Optionally add new group
		*groups = append(*groups, racdb.AsmDiskGroupStatus{
			Type:  groupType,
			Name:  string(groupType),
			Disks: diskStatuses,
		})
	}
}

// addRacNodeStatus provides documentation for the addRacNodeStatus function.
func addRacNodeStatus(instance *racdb.RacDatabase, ctx context.Context, req ctrl.Request, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, specidx int, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {

	var racState string
	podName := oraRacSpex.Name + "-0"
	idx, status := contains(instance, racStatus)
	if status {
		// racstate need to be read before overwriting the instance.Status.RacNodes[idx] = racStatus
		racState = instance.Status.RacNodes[idx].NodeDetails.State
		instance.Status.RacNodes[idx] = racStatus
		instState := instance.Status.RacNodes[idx].NodeDetails.InstanceState
		if instState == "OPEN" {
			instance.Status.RacNodes[idx].NodeDetails.State = "AVAILABLE"
		} else {
			if (racState == "PENDING") || (racState == "ADDNODE") || (racState == "PROVISIONING") || (racState == "FAILED") || (racState == "UPDATE") {
				instance.Status.RacNodes[idx].NodeDetails.State = getRacInstStateFile(podName, instance, specidx, kubeClient, kubeConfig, logger)
			} else {
				instance.Status.RacNodes[idx].NodeDetails.State = "PENDING"
			}
			// Block : at this time no code to maintain update and failed state
			//failed state requires human intervention
			//update state must be updated from where it is being called
		}

	} else {
		instance.Status.RacNodes = append(instance.Status.RacNodes, racStatus)
		instance.Status.RacNodes[idx].NodeDetails.State = "PENDING"
	}

}

// addRacNodeStatusForCluster provides documentation for the addRacNodeStatusForCluster function.
func addRacNodeStatusForCluster(
	instance *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	racStatus *racdb.RacNodeStatus,
	nodeName string,
	nodeIndex int,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) {
	// var racState string
	podName := nodeName + "-0"

	idx, status := containsCluster(instance, racStatus)
	if status {
		// racState = instance.Status.RacNodes[idx].NodeDetails.State
		instance.Status.RacNodes[idx] = racStatus
		instState := instance.Status.RacNodes[idx].NodeDetails.InstanceState
		if instState == "OPEN" {
			instance.Status.RacNodes[idx].NodeDetails.State = "AVAILABLE"
		} else {
			s := instance.Status.RacNodes[idx].NodeDetails.State
			if s == "PENDING" || s == "ADDNODE" || s == "PROVISIONING" || s == "FAILED" || s == "UPDATE" {
				instance.Status.RacNodes[idx].NodeDetails.State = getRacInstStateFileForCluster(podName, instance, nodeIndex, kubeClient, kubeConfig, logger)
			} else {
				instance.Status.RacNodes[idx].NodeDetails.State = "PENDING"
			}
		}
	} else {
		instance.Status.RacNodes = append(instance.Status.RacNodes, racStatus)
		instance.Status.RacNodes[len(instance.Status.RacNodes)-1].NodeDetails.State = "PENDING"
	}
}

// getMountedDevicesForCluster provides documentation for the getMountedDevicesForCluster function.
func getMountedDevicesForCluster(
	podName, namespace string,
	racStatus *racdb.RacNodeStatus,
	nodeName string,
	rclient client.Client,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	kubeClient kubernetes.Interface,
) []string {
	var asmList []string

	// Get the pod associated with this RAC cluster node
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "Failed to get pod for RAC node", "PodName", podName, "Namespace", namespace)
		return nil
	}

	// Loop through the volume devices or mounted volumes in the pod spec
	for _, container := range pod.Spec.Containers {
		for _, volumeDevice := range container.VolumeDevices {
			asmList = append(asmList, volumeDevice.DevicePath)
		}
	}
	return asmList
}

// getVipDetailsForCluster provides documentation for the getVipDetailsForCluster function.
func getVipDetailsForCluster(
	instance *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	racStatus *racdb.RacNodeStatus,
	kClient client.Client,
) map[string]string {
	strMap := make(map[string]string)

	// Use your consistent cluster-based service naming:
	vipSvcName := GetClusterSvcName(instance, clusterSpec, nodeIndex, "vip")

	_, err := CheckRacSvcForCluster(instance, clusterSpec, nodeIndex, "vip", vipSvcName, kClient)
	if err == nil {
		strMap["Name"] = vipSvcName
	}
	// Add more info as needed
	return strMap
}

// Example for contains in cluster style
// containsCluster provides documentation for the containsCluster function.
func containsCluster(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus) (int, bool) {
	for idx, n := range instance.Status.RacNodes {
		if n.Name == racStatus.Name {
			return idx, true
		}
	}
	return 0, false
}

// This function update the RACInstState  i.e. instance.Status.RacNodes.state
// Valid values are: provisioning/addnode/update/failed/avialable/pending
// When the provisioning start , first time instance.status.state set to pending
// UpdateRacInstState provides documentation for the UpdateRacInstState function.
func UpdateRacInstState(instance *racdb.RacDatabase, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {

	// if len(instance.Status.RacNodes) > 0 {
	//	for _, v := range instance.Status.RacNodes {
	//        if v.NodeDetails.
	//	}
	//}

}

// This function update the RACTopology state i.e. instance.Status.state
// Valid values are: provisioning/addnode/update/failed/avialable/pending
// When the provisioning start , first time instance.status.state set to pending
// UpdateRacDBTopologyState provides documentation for the UpdateRacDBTopologyState function.
func UpdateRacDBTopologyState(instance *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger) {
	racDatabase := &racdb.RacDatabase{}
	if len(instance.Status.RacNodes) > 0 {
		state := map[string]struct{}{}
		for _, v := range instance.Status.RacNodes {
			inst_state := strings.ToLower(strings.TrimSpace(v.NodeDetails.State))
			state[inst_state] = struct{}{}
		}
		if len(state) == 0 {
			racDatabase.Status.State = string(racdb.RACPendingState)
		} else if _, curr_state := state["failed"]; curr_state {
			racDatabase.Status.State = string(racdb.RACFailedState)
		} else if _, curr_state := state["pending"]; curr_state {
			racDatabase.Status.State = string(racdb.RACPendingState)
		} else if _, curr_state := state["provisioning"]; curr_state {
			racDatabase.Status.State = string(racdb.RACProvisionState)
		} else if _, curr_state := state["update"]; curr_state {
			racDatabase.Status.State = string(racdb.RACUpdateState)
		} else if _, curr_state := state["addnode"]; curr_state {
			racDatabase.Status.State = string(racdb.RACAddInstState)
		} else if _, curr_state := state["podavailable"]; curr_state {
			racDatabase.Status.State = string(racdb.RACPodAvailableState)
		} else {
			racDatabase.Status.State = string(racdb.RACAvailableState)
		}
		instance.Status.State = racDatabase.Status.State
	}

}

// UpdateRacDbStatusData provides documentation for the UpdateRacDbStatusData function.
func UpdateRacDbStatusData(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podNames []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger, nodeDetails map[string]*corev1.Node,
) {
	//mode := GetDbOpenMode(instance.Spec.Shard[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)
	podName := podNames[0]
	racDatabase.Status.DbState = getDbState(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.Role = getDbRole(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.ReleaseUpdate = getDBVersion(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.ConnectString = getConnStr(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.PdbConnectString = getPdbConnStr(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.ExternalConnectString = getExternalConnStr(podName, racDatabase, 0, kubeClient, kubeConfig, logger)
	racDatabase.Status.ClientEtcHost = getClientEtcHost(podNames, racDatabase, 0, kubeClient, kubeConfig, logger, nodeDetails)
	racDatabase.Status.DbSecret = racDatabase.Spec.DbSecret
	racDatabase.Status.AsmDiskGroups = getAsmInstState(podName, racDatabase, 0, kubeClient, kubeConfig, logger)

	UpdateRacDbServiceStatus(racDatabase, ctx, req, podName, kubeClient, kubeConfig, logger)
	UpdateRacDBTopologyState(racDatabase, ctx, req, podName, kubeClient, kubeConfig, logger)
}

// UpdateRacDbServiceStatus provides documentation for the UpdateRacDbServiceStatus function.
func UpdateRacDbServiceStatus(instance *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	//This function update the instance.Status.ServiceDetails states
	if instance.Spec.ServiceDetails.Name != "" {
		instance.Status.ServiceDetails.Name = instance.Spec.ServiceDetails.Name
		instance.Status.ServiceDetails.SvcState = getSvcState(podName, instance, 0, kubeClient, kubeConfig, logger)
	}
}

// contains provides documentation for the contains function.
func contains(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus) (int, bool) {
	var index int
	if len(instance.Status.RacNodes) > 0 {
		for index, v := range instance.Status.RacNodes {
			if v.Name == racStatus.Name {
				return index, true
			}
		}
	}

	return index, false
}

// getVipDetails provides documentation for the getVipDetails function.
func getVipDetails(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, rclient client.Client) map[string]string {
	strMap := make(map[string]string)
	_, err := CheckRacSvc(instance, "vip", oraRacSpex, oraRacSpex.VipSvcName, rclient)
	if err == nil {
		strMap["Name"] = oraRacSpex.VipSvcName
		// See if service already exists and create if it doesn't
	}

	return strMap

}

// GetcrsAsmDeviceList provides documentation for the GetcrsAsmDeviceList function.
func GetcrsAsmDeviceList(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) string {
	asmList := ""
	var err error
	if len(instance.Status.RacNodes) > 0 {
		asmList, err = CheckAsmList(instance.Status.RacNodes[0].Name, instance, kubeClient, kubeConfig, logger)
		if err != nil {
			return ""
		}

	}

	return asmList

}

// GetdbAsmDeviceList provides documentation for the GetdbAsmDeviceList function.
func GetdbAsmDeviceList(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) string {
	dbasmList := ""
	var err error
	if len(instance.Status.RacNodes) > 0 {
		dbasmList, err = CheckDbAsmList(instance.Status.RacNodes[0].Name, instance, kubeClient, kubeConfig, logger)
		if err != nil {
			return ""
		}

	}

	return dbasmList

}

// getMountedDevices provides documentation for the getMountedDevices function.
func getMountedDevices(podName, namespace string, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, rclient client.Client, kubeConfig clientcmd.ClientConfig, logger logr.Logger, kubeClient kubernetes.Interface) []string {
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

// delRacNodeStatus provides documentation for the delRacNodeStatus function.
func delRacNodeStatus(instance *racdb.RacDatabase, name string) []*racdb.RacNodeStatus {
	newRacStatus := []*racdb.RacNodeStatus{}
	if len(instance.Status.RacNodes) > 0 {
		for _, value := range instance.Status.RacNodes {
			if ((value.Name) != (name)) && (value != nil) {
				newRacStatus = append(newRacStatus, value)
			}
		}
	}
	return newRacStatus
}

// getPvcDetails provides documentation for the getPvcDetails function.
func getPvcDetails(instance *racdb.RacDatabase, racStatus *racdb.RacNodeStatus, oraRacSpex racdb.RacInstDetailSpec, rclient client.Client) map[string]string {
	strMap := make(map[string]string)
	if len(oraRacSpex.PvcName) > 0 {
		strMap = oraRacSpex.PvcName
	}

	return strMap

}
