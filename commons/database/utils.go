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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// To requeue after 15 secs allowing graceful state changes
var requeueY ctrl.Result = ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}
var requeueN ctrl.Result = ctrl.Result{}

// Filter events that trigger reconcilation
func ResourceEventHandler() predicate.Predicate {
	return predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPodObject, oldOk := e.ObjectOld.(*corev1.Pod)
			newPodObject, newOk := e.ObjectNew.(*corev1.Pod)

			// Handling the Pod Ready Status Changes .
			if oldOk && newOk {
				oldStatus, newStatus := "", ""
				for _, condition := range oldPodObject.Status.Conditions {
					if condition.Type == "Ready" {
						oldStatus = string(condition.Status)
						break
					}
				}
				for _, condition := range newPodObject.Status.Conditions {
					if condition.Type == "Ready" {
						newStatus = string(condition.Status)
						break
					}
				}
				// If Pod Ready Status Changed , reconcile
				if oldStatus != newStatus {
					return true
				}

			}
			// Ignore updates to CR status in which case metadata.Generation does not change
			// Reconcile if object Deletion Timestamp Set
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
				e.ObjectOld.GetDeletionTimestamp() != nil || e.ObjectNew.GetDeletionTimestamp() != nil
		},
	}
}

// getLabelsForController returns the labels for selecting the resources
func GetLabelsForController(version string, name string) map[string]string {
	if version != "" {
		return map[string]string{"app": name, "version": version}
	} else {
		return map[string]string{"app": name}
	}
}

// getPodNames returns the pod names of the array of pods passed in
func GetPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// Poll up to timeout seconds for Object to change state
// Returns an error if the object never changes the state
func WaitForStatusChange(r client.Reader, objName string, namespace string,
	ctx context.Context, req ctrl.Request, timeout time.Duration, object string, statusChange string) error {
	return wait.PollImmediate(time.Second, timeout, IsStatusChanged(r, objName, namespace, ctx, req, object, statusChange))
}

// returns a func() that returns true if an object is confirmed to be created or deleted . else false
func IsStatusChanged(r client.Reader, objName string, namespace string,
	ctx context.Context, req ctrl.Request, object string, statusChange string) wait.ConditionFunc {
	return func() (bool, error) {
		log := ctrllog.FromContext(ctx).WithValues("IsStatusChanged", req.NamespacedName)
		log.Info("", " Waiting for ", object, " ", statusChange, " Name : ", objName)
		var obj client.Object
		if object == "pod" {
			obj = &corev1.Pod{}
		}
		if object == "pvc" {
			obj = &corev1.PersistentVolumeClaim{}
		}
		if object == "svc" {
			obj = &corev1.Service{}
		}
		err := r.Get(ctx, types.NamespacedName{Name: objName, Namespace: namespace}, obj)

		if object == "pod" {
			if statusChange == "deletion" {
				if err != nil && apierrors.IsNotFound(err) {
					log.Error(err, "Pod Already Deleted", "SingleInstanceDatabase.Namespace", namespace, "Pod.Name", objName)
					// No need to wait if Pod already Deleted
					return true, nil

				} else if err != nil {
					log.Error(err, "Failed to get the Pod Details")
					// return the false,err that reconciler failed to get pods
					return false, err

				}
				log.Info("Found the Pod ", "Name :", objName)
				if deletionTimeStamp := obj.GetDeletionTimestamp(); deletionTimeStamp != nil {
					// Pod Found and Status changed . Return true,nil as No wait required
					return true, nil
				} else {
					// Pod Found and Status not changed . Return false,nil as wait required till the status changes
					return false, nil
				}
			}
			if statusChange == "creation" {
				if err != nil && apierrors.IsNotFound(err) {
					log.Info("Creating new POD", "SingleInstanceDatabase.Namespace", namespace, "Obj.Name", objName)
					// wait as Pod is being created
					return false, nil

				} else if err != nil {
					log.Error(err, "Failed to get the Pod Details")
					// return the false,err that reconciler failed to get pod
					return false, err

				}
				log.Info("POD Created ", "Name :", objName)
				return true, nil
			}
		}
		if object == "pvc" {
			if err != nil && apierrors.IsNotFound(err) {
				log.Info("Creating new PVC", "SingleInstanceDatabase.Namespace", namespace, "Obj.Name", objName)
				// wait as Pvc is being created
				return false, nil

			} else if err != nil {
				log.Error(err, "Failed to get the pvc Details")
				// return the false,err that reconciler failed to get pvc
				return false, err

			}
			log.Info("PVC Created ", "Name :", objName)
			return true, nil
		}
		if object == "svc" {
			if err != nil && apierrors.IsNotFound(err) {
				log.Info("Creating new Service", "SingleInstanceDatabase.Namespace", namespace, "Obj.Name", objName)
				// wait as Service is being created
				return false, nil

			} else if err != nil {
				log.Error(err, "Failed to get the Service Details")
				// return the false,err that reconciler failed to get Service
				return false, err

			}
			log.Info("Service Created ", "Name :", objName)
			return true, nil
		}
		return false, nil

	}

}

// Execs into podName and executes command
func ExecCommand(r client.Reader, config *rest.Config, podName string, namespace string, containerName string,
	ctx context.Context, req ctrl.Request, nologCommand bool, command ...string) (string, error) {

	log := ctrllog.FromContext(ctx).WithValues("ExecCommand", req.NamespacedName)
	if !nologCommand {
		log.Info("Executing Command :")
		log.Info(strings.Join(command, " "))
	}
	if config == nil {
		log.Info("r.Config nil")
		return "Error", nil
	}
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod)
	if err != nil {
		return "", fmt.Errorf("could not get pod info: %v", err)
	} else {
		log.Info("Pod Found", "Name : ", podName)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "config error")
	}
	rc := client.CoreV1().RESTClient()
	if rc == nil {
		return "RESTClient Error", nil
	}
	rcreq := rc.Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")
	rcreq.VersionedParams(&corev1.PodExecOptions{
		Command:   command,
		Stdout:    true,
		Stderr:    true,
		Container: containerName,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", rcreq.URL())
	if err != nil {
		return "", fmt.Errorf("failed to init executor: %v", err)
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	if err != nil {
		return "", fmt.Errorf("could not execute: %v", err)
	}
	if execErr.Len() > 0 {
		return "", fmt.Errorf("stderr: %v", execErr.String())
	}
	return execOut.String(), nil
}

// returns a randomString
func GenerateRandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// retuns Ready Pod,No of replicas ( Only running and Pending Pods) ,available pods , Total No of Pods of a particular CRD
func FindPods(r client.Reader, version string, image string, name string, namespace string, ctx context.Context,
	req ctrl.Request) (corev1.Pod, int, []corev1.Pod, []corev1.Pod, error) {

	log := ctrllog.FromContext(ctx).WithValues("FindPods", req.NamespacedName)

	// "available" stores list of pods which can be deleted while scaling down i.e the pods other than one of Ready Pods
	// There are multiple ready pods possible in OracleRestDataService , while others have atmost one readyPod
	var available []corev1.Pod
	var podsMarkedToBeDeleted []corev1.Pod
	var readyPod corev1.Pod // To Store the Ready Pod ( Pod that Passed Readiness Probe . Will be shown as 1/1 Running )

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{client.InNamespace(namespace), client.MatchingLabels(GetLabelsForController(version, name))}

	// List retrieves list of objects for a given namespace and list options.
	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods of "+name, "Namespace", namespace, "Name", name)
		return readyPod, 0, available, podsMarkedToBeDeleted, err
	}

	// r.List() lists all the pods in running, pending,terminating stage matching listOpts . so filter them
	// Fetch the Running and Pending Pods

	for _, pod := range podList.Items {
		// Return pods having Image = image (or) if image = ""(Needed in case when called findpods with "" image)
		if pod.Spec.Containers[0].Image == image || image == "" {
			if pod.ObjectMeta.DeletionTimestamp != nil {
				podsMarkedToBeDeleted = append(podsMarkedToBeDeleted, pod)
				continue
			}
			if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
				if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].Ready && readyPod.Name == "" {
					readyPod = pod
				} else {
					available = append(available, pod)
				}
			}
		}
	}
	podNames := GetPodNames(available)
	replicasFound := len(podNames)

	if readyPod.Name != "" {
		replicasFound = replicasFound + 1 // if one of the pods is ready , its not there in "available" , So do a "+1"
		log.Info("Ready Pod ", "Name :", readyPod.Name)
	} else {
		log.Info("No " + name + " Pod is Ready ")
	}

	log.Info(name+" Pods Available ( Other Than Ready Pod )", " Names :", podNames)
	log.Info("Total No Of "+name+" PODS", "Count", replicasFound)

	return readyPod, replicasFound, available, podsMarkedToBeDeleted, nil
}

// returns flashBackStatus,archiveLogStatus,forceLoggingStatus of Primary Pod
func CheckDBConfig(readyPod corev1.Pod, r client.Reader, config *rest.Config,
	ctx context.Context, req ctrl.Request, edition string) (bool, bool, bool, ctrl.Result) {

	log := ctrllog.FromContext(ctx).WithValues("CheckDBParams", req.NamespacedName)

	var forceLoggingStatus bool
	var flashBackStatus bool
	var archiveLogStatus bool
	if readyPod.Name == "" {
		log.Info("No Pod is Ready")
		// As No pod is ready now , turn on mode when pod is ready . so requeue the request
		return false, false, false, requeueY

	} else {
		out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
			fmt.Sprintf("echo -e  \"%s\"  | %s", CheckModesSQL, SQLPlusCLI))
		if err != nil {
			log.Error(err, "Error in ExecCommand()")
			return false, false, false, requeueY
		} else {
			log.Info("CheckModes Output")
			log.Info(out)

			if strings.Contains(out, "log_mode:NOARCHIVELOG") {
				archiveLogStatus = false
			}
			if strings.Contains(out, "log_mode:ARCHIVELOG") {
				archiveLogStatus = true
			}
			if strings.Contains(out, "flashback_on:NO") {
				flashBackStatus = false
			}
			if strings.Contains(out, "flashback_on:YES") {
				flashBackStatus = true
			}
			if strings.Contains(out, "force_logging:NO") {
				forceLoggingStatus = false
			}
			if strings.Contains(out, "force_logging:YES") {
				forceLoggingStatus = true
			}
		}
		log.Info("FlashBackStatus ", "Status :", flashBackStatus)
		log.Info("ArchiveLogStatus ", "Status :", archiveLogStatus)
		log.Info("ForceLoggingStatus ", "Status :", forceLoggingStatus)
	}

	return flashBackStatus, archiveLogStatus, forceLoggingStatus, requeueN
}

// CHECKS IF SID IN DATABASES SLICE , AND ITS DGROLE
func IsDatabaseFound(sid string, databases []string, dgrole string) (bool, bool) {
	found := false
	isdgrole := false
	for i := 0; i < len(databases); i++ {
		splitstr := strings.Split(databases[i], ":")
		if strings.EqualFold(sid, splitstr[0]) {
			found = true
			if strings.EqualFold(dgrole, splitstr[1]) {
				isdgrole = true
			}
			break
		}
	}
	return found, isdgrole
}

// Returns a Primary Database in "databases" slice
func GetPrimaryDatabase(databases []string) string {
	primary := ""
	for i := 0; i < len(databases); i++ {
		splitstr := strings.Split(databases[i], ":")
		if strings.ToUpper(splitstr[1]) == "PRIMARY" {
			primary = splitstr[0]
			break
		}
	}
	return primary
}

// Returns the databases in DG config .
func GetDatabasesInDgConfig(readyPod corev1.Pod, r client.Reader,
	config *rest.Config, ctx context.Context, req ctrl.Request) ([]string, string, error) {
	log := ctrllog.FromContext(ctx).WithValues("GetDatabasesInDgConfig", req.NamespacedName)

	// ## FIND DATABASES PRESENT IN DG CONFIGURATION
	out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba ", DataguardBrokerGetDatabaseCMD))
	if err != nil {
		return []string{}, "", err
	}
	log.Info("GetDatabasesInDgConfig Output")
	log.Info(out)

	if !strings.Contains(out, "no rows selected") && !strings.Contains(out, "ORA-") {
		out1 := strings.Replace(out, " ", "_", -1)
		// filtering output and storing databses in dg configuration in  "databases" slice
		databases := strings.Fields(out1)

		// first 2 values in the slice will be column name(DATABASES) and a seperator(--------------) . so take the slice from position [2:]
		databases = databases[2:]
		return databases, out, nil
	}
	return []string{}, out, errors.New("databases in DG config is nil")
}

// Returns Database version
func GetDatabaseVersion(readyPod corev1.Pod, r client.Reader,
	config *rest.Config, ctx context.Context, req ctrl.Request, edition string) (string, string, error) {
	log := ctrllog.FromContext(ctx).WithValues("GetDatabaseVersion", req.NamespacedName)

	// ## FIND DATABASES PRESENT IN DG CONFIGURATION
	out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", GetVersionSQL, SQLPlusCLI))
	if err != nil {
		return "", "", err
	}
	log.Info("GetDatabaseVersion Output")
	log.Info(out)

	if !strings.Contains(out, "no rows selected") && !strings.Contains(out, "ORA-") {
		out1 := strings.Replace(out, " ", "_", -1)
		// filtering output and storing databses in dg configuration in  "databases" slice
		out2 := strings.Fields(out1)

		// first 2 values in the slice will be column name(VERSION) and a seperator(--------------) . so the version would be out2[2]
		version := out2[2]
		return version, out, nil
	}
	return "", out, errors.New("database version is nil")

}

// Fetch role by quering the DB
func GetDatabaseRole(readyPod corev1.Pod, r client.Reader,
	config *rest.Config, ctx context.Context, req ctrl.Request, edition string) (string, error) {
	log := ctrllog.FromContext(ctx).WithValues("GetDatabaseRole", req.NamespacedName)

	out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("echo -e  \"%s\"  | %s", GetDatabaseRoleCMD, SQLPlusCLI))
	if err != nil {
		return "", err
	}
	log.Info(out)
	if !strings.Contains(out, "no rows selected") && !strings.Contains(out, "ORA-") {
		out = strings.Replace(out, " ", "_", -1)
		// filtering output and storing databse_role in  "database_role"
		databaseRole := strings.Fields(out)[2]

		// first 2 values in the slice will be column name(DATABASE_ROLE) and a seperator(--------------) .
		return databaseRole, nil
	}
	return "", errors.New("database role is nil")
}

// Returns true if any of the pod in 'pods' is with pod.Status.Phase == phase
func IsAnyPodWithStatus(pods []corev1.Pod, phase corev1.PodPhase) (bool, corev1.Pod) {
	anyPodWithPhase := false
	var podWithPhase corev1.Pod
	for _, pod := range pods {
		if pod.Status.Phase == phase {
			anyPodWithPhase = true
			podWithPhase = pod
			break
		}
	}
	return anyPodWithPhase, podWithPhase
}

// Convert "sqlplus -s " output to array of lines
func StringToLines(s string) (lines []string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(s))
	i := 0
	for scanner.Scan() {
		// store from line 3 as line 0 would be blank, line 1 - column_name , line 2 - seperator (----)
		if i > 2 {
			lines = append(lines, scanner.Text())
		}
		i++
	}
	err = scanner.Err()
	return
}

// Get Node Ip to display in ConnectionString
// Returns Node External Ip if exists ; else InternalIP
func GetNodeIp(r client.Reader, ctx context.Context, req ctrl.Request) string {

	log := ctrllog.FromContext(ctx).WithValues("GetNodeIp", req.NamespacedName)

	readyPod, _, available, _, err := FindPods(r, "", "", req.Name, req.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return ""
	}
	if readyPod.Name != "" {
		available = append(available, readyPod)
	}
	nodeip := ""
	for _, pod := range available {
		if nodeip == "" {
			nodeip = pod.Status.HostIP
		}
		if pod.Status.HostIP < nodeip {
			nodeip = pod.Status.HostIP
		}
	}

	node := &corev1.Node{}
	err = r.Get(ctx, types.NamespacedName{Name: nodeip, Namespace: req.Namespace}, node)

	if err == nil {
		for _, address := range node.Status.Addresses {
			if address.Type == "ExternalIP" {
				nodeip = address.Address
			}
		}
	}

	return nodeip
}

// GetSidPdbEdition to display sid, pdbname, edition in ConnectionString
func GetSidPdbEdition(r client.Reader, config *rest.Config, ctx context.Context, req ctrl.Request) (string, string, string) {

	log := ctrllog.FromContext(ctx).WithValues("GetNodeIp", req.NamespacedName)

	readyPod, _, _, _, err := FindPods(r, "", "", req.Name, req.Namespace, ctx, req)
	if err != nil {
		log.Error(err, err.Error())
		return "", "", ""
	}
	if readyPod.Name != "" {
		out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "",
			ctx, req, false, "bash", "-c", GetSidPdbEditionCMD)
		if err != nil {
			log.Error(err, err.Error())
			return "", "", ""
		}
		splitstr := strings.Split(out, ",")
		if len(splitstr) == 4 {
			return splitstr[0], splitstr[1], splitstr[2]
		}
	}

	return "", "", ""
}

// Get Datapatch Status
func GetSqlpatchStatus(r client.Reader, config *rest.Config, readyPod corev1.Pod, ctx context.Context, req ctrl.Request) (string, string, string, error) {
	log := ctrllog.FromContext(ctx).WithValues("getSqlpatchStatus", req.NamespacedName)

	// GET SQLPATCH STATUS ( INITIALIZE ==> END ==> SUCCESS (or) WITH ERRORS )
	out, err := ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba", GetSqlpatchStatusSQL))
	if err != nil {
		log.Error(err, err.Error())
		return "", "", "", err
	}
	log.Info("GetSqlpatchStatusSQL Output")
	log.Info(out)
	sqlpatchStatuses, err := StringToLines(out)
	if err != nil {
		log.Error(err, err.Error())
		return "", "", "", err
	}
	if len(sqlpatchStatuses) == 0 {
		return "", "", "", nil
	}
	//GET SQLPATCH VERSIONS (SOURCE & TARGET)
	out, err = ExecCommand(r, config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", fmt.Sprintf("echo -e  \"%s\"  | sqlplus -s / as sysdba", GetSqlpatchVersionSQL))
	if err != nil {
		log.Error(err, err.Error())
		return "", "", "", err
	}
	log.Info("GetSqlpatchVersionSQL Output")
	log.Info(out)

	sqlpatchVersions, err := StringToLines(out)
	if err != nil {
		log.Error(err, err.Error())
		return "", "", "", err
	}
	splitstr := strings.Split(sqlpatchVersions[0], ":")
	return sqlpatchStatuses[0], splitstr[0], splitstr[1], nil
}

// Is Source Database On same Cluster
func IsSourceDatabaseOnCluster(cloneFrom string) bool {
	if strings.Contains(cloneFrom, ":") && strings.Contains(cloneFrom, "/") {
		return false
	}
	return true
}
