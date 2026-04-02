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
// Package commons provides RAC helper utilities aligned with docs/rac and Kubernetes guidance.
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

func ensureRacStatusDefaults(racDatabase *racdb.RacDatabase) {
	if racDatabase.Status.AsmDiskGroups == nil {
		racDatabase.Status.AsmDiskGroups = []racdb.AsmDiskGroupStatus{}
	}
	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = &racdb.RacInitParams{}
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

	ensureRacStatusDefaults(racDatabase)

	racDatabase.Status.State = state

	// Update status based on state
	switch state {
	case string(racdb.RACAvailableState):
		populateRacNodeDetailsForCluster(
			racDatabase, clusterSpec, nodeIndex, state, podName, nodeName,
			racStatus, racNodeDetails, kClient, kubeClient, kubeConfig, logger,
		)
		addRacNodeStatusForCluster(racDatabase, ctx, req, racStatus, nodeName, nodeIndex, kubeClient, kubeConfig, logger)
	case string(racdb.RACPodAvailableState):
		populateRacNodeDetailsForCluster(
			racDatabase, clusterSpec, nodeIndex, state, podName, nodeName,
			racStatus, racNodeDetails, kClient, kubeClient, kubeConfig, logger,
		)
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

func populateRacNodeDetailsForCluster(
	racDatabase *racdb.RacDatabase,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	state string,
	podName string,
	nodeName string,
	racStatus *racdb.RacNodeStatus,
	racNodeDetails *racdb.RacNodeDetailedStatus,
	kClient client.Client,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
) {
	racNodeDetails.ClusterState = getClusterState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
	racNodeDetails.PodState = state
	racNodeDetails.State = "OPEN"
	racStatus.Name = podName
	racNodeDetails.VipDetails = getVipDetailsForCluster(racDatabase, clusterSpec, nodeIndex, racStatus, kClient)
	racNodeDetails.InstanceState = getDbInstState(podName, racDatabase, nodeIndex, kubeClient, kubeConfig, logger)
	racNodeDetails.MountedDevices = getMountedDevicesForCluster(podName, racDatabase.Namespace, racStatus, nodeName, kClient, kubeConfig, logger, kubeClient)
	racStatus.NodeDetails = racNodeDetails
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
	_ = ctx
	_ = req
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
			if shouldRefreshNodeState(s) {
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

func shouldRefreshNodeState(s string) bool {
	switch s {
	case "PENDING", "ADDNODE", "PROVISIONING", "FAILED", "UPDATE":
		return true
	default:
		return false
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
	return getPodVolumeDevicePaths(podName, namespace, kubeClient, logger)
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

func getPodVolumeDevicePaths(podName, namespace string, kubeClient kubernetes.Interface, logger logr.Logger) []string {
	var asmList []string

	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "Failed to get pod for RAC node", "PodName", podName, "Namespace", namespace)
		return nil
	}

	for _, container := range pod.Spec.Containers {
		for _, volumeDevice := range container.VolumeDevices {
			asmList = append(asmList, volumeDevice.DevicePath)
		}
	}

	return asmList
}

func getAsmDeviceListByMode(
	instance *racdb.RacDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	useCRS bool,
) string {
	if len(instance.Status.RacNodes) == 0 {
		return ""
	}

	podName := instance.Status.RacNodes[0].Name
	resp := NewExecCommandResp(kubeClient, kubeConfig)
	var (
		result string
		err    error
	)
	if useCRS {
		result, err = CheckAsmListWithResp(podName, resp, instance, logger)
	} else {
		result, err = CheckDbAsmListWithResp(podName, resp, instance, logger)
	}
	if err != nil {
		return ""
	}
	return result
}

// delRacNodeStatus provides documentation for the delRacNodeStatus function.
func delRacNodeStatus(instance *racdb.RacDatabase, name string) []*racdb.RacNodeStatus {
	newRacStatus := []*racdb.RacNodeStatus{}
	if len(instance.Status.RacNodes) > 0 {
		for _, value := range instance.Status.RacNodes {
			if value != nil && value.Name != name {
				newRacStatus = append(newRacStatus, value)
			}
		}
	}
	return newRacStatus
}
