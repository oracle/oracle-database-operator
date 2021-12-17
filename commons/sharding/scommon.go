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
	"fmt"
	databasealphav1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v51/common"
	"github.com/oracle/oci-go-sdk/v51/ons"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants for hello-stateful StatefulSet & Volumes
const (
	oraImagePullPolicy        = corev1.PullAlways
	orainitCmd1               = "set -ex;" + "touch /tmp/test_cmd1.txt"
	orainitCmd2               = "set -ex; curl https://codeload.github.com/oracle/db-sharding/tar.gz/master |   tar -xz --strip=4 db-sharding-master/docker-based-sharding-deployment/dockerfiles/19.3.0/scripts; cp -i -r scripts/* /opt/oracle/scripts/setup;chmod 777 /opt/oracle/scripts/setup/*"
	orainitCmd3               = "/opt/oracle/runOracle.sh.sharding"
	orainitCmd4               = "set -ex;" + "touch /tmp/test_cmd4.txt"
	orainitCmd5               = "set -ex;" + "[[ `hostname` =~ -([0-9]+)$ ]] || exit 1 ;" + "ordinal=${BASH_REMATCH[1]};" + "cp /mnt/config-map/envfile  /mnt/conf.d/; cat /mnt/conf.d/envfile | awk -v env_var=$ordinal -F '=' '{print \"export \" $1\"=\"$2 env_var }' > /tmp/test.env; mv /tmp/test.env /mnt/conf.d/envfile"
	oraShardAddCmd            = "/bin/python /opt/oracle/scripts/sharding/main.py"
	oraRunAsNonRoot           = true
	oraRunAsUser              = int64(54321)
	oraFsGroup                = int64(54321)
	oraScriptMount            = "/opt/oracle/scripts/sharding/scripts"
	oraDataMount              = "/opt/oracle/oradata"
	oraGsmDataMount           = "/opt/oracle/gsmdata"
	oraConfigMapMount         = "/mnt/config-map"
	oraEnvFileMount           = "/mnt/conf.d"
	oraEnvFile                = "/mnt/conf.d/envfile"
	oraSecretMount            = "/mnt/secrets"
	oraShm                    = "/dev/shm"
	oraStage                  = "/mnt/stage"
	oraDBPort                 = 1521
	oraGSMPort                = 1522
	oraRemoteOnsPort          = 6234
	oraLocalOnsPort           = 6123
	oraAgentPort              = 8080
	ShardingDatabaseFinalizer = "Shardingdb.oracle.com"
)

// Function to build the env var specification
func buildEnvVarsSpec(instance *databasealphav1.ShardingDatabase, variables []databasealphav1.EnvironmentVariable, name string, restype string, masterFlag bool, directorParams string) []corev1.EnvVar {
	var result []corev1.EnvVar
	var varinfo string
	var sidFlag bool = false
	var secretFlag bool = false
	var pwdFileFLag bool = false
	var pwdKeyFlag bool = false
	var pdbFlag bool = false
	var sDirectParam bool = false
	var sGroup1Params bool = false
	var sGroup2Params bool = false
	var catalogParams bool = false
	var oldPdbFlag bool = false
	var oldSidFlag bool = false
	var archiveLogFlag bool = false
	var shardSetupFlag bool = false

	for _, variable := range variables {
		if variable.Name == "ORACLE_SID" {
			sidFlag = true
		}
		if variable.Name == "SECRET_VOLUME" {
			secretFlag = true
		}
		if variable.Name == "COMMON_OS_PWD_FILE" {
			pwdFileFLag = true
		}
		if variable.Name == "PWD_KEY" {
			pwdKeyFlag = true
		}
		if variable.Name == "ORACLE_PDB" {
			pdbFlag = true
		}
		if variable.Name == "SHARD_DIRECTOR_PARAMS" {
			sDirectParam = true
		}
		if variable.Name == "SHARD1_GROUP_PARAMS" {
			sGroup1Params = true
		}
		if variable.Name == "SHARD2_GROUP_PARAMS" {
			sGroup2Params = true
		}
		if variable.Name == "CATALOG_PARAMS" {
			catalogParams = true
		}
		if variable.Name == "OLD_ORACLE_SID" {
			oldSidFlag = true
		}
		if variable.Name == "OLD_ORACLE_PDB" {
			oldPdbFlag = true
		}
		if variable.Name == "SHARD_SETUP" {
			shardSetupFlag = true
		}
		if variable.Name == "OLD_ORACLE_PDB" {
			archiveLogFlag = true
		}
		result = append(result, corev1.EnvVar{Name: variable.Name, Value: variable.Value})
	}
	if !shardSetupFlag {
		if restype == "SHARD" {
			result = append(result, corev1.EnvVar{Name: "SHARD_SETUP", Value: "true"})
		}
		if restype == "CATALOG" {
			result = append(result, corev1.EnvVar{Name: "SHARD_SETUP", Value: "true"})
		}
		if restype == "GSM" {
			result = append(result, corev1.EnvVar{Name: "SHARD_SETUP", Value: "true"})
		}
	}
	if !archiveLogFlag {
		if restype == "SHARD" {
			result = append(result, corev1.EnvVar{Name: "ENABLE_ARCHIVELOG", Value: "true"})
		}
		if restype == "CATALOG" {
			result = append(result, corev1.EnvVar{Name: "ENABLE_ARCHIVELOG", Value: "true"})
		}
	}

	if !sidFlag {
		if restype == "SHARD" {
			result = append(result, corev1.EnvVar{Name: "ORACLE_SID", Value: strings.ToUpper(name)})
		}
		if restype == "CATALOG" {
			result = append(result, corev1.EnvVar{Name: "ORACLE_SID", Value: strings.ToUpper(name)})
		}
	}
	if !pdbFlag {
		if restype == "SHARD" {
			result = append(result, corev1.EnvVar{Name: "ORACLE_PDB", Value: strings.ToUpper(name) + "PDB"})
		}
		if restype == "CATALOG" {
			result = append(result, corev1.EnvVar{Name: "ORACLE_PDB", Value: strings.ToUpper(name) + "PDB"})
		}
	}
	if !secretFlag {
		result = append(result, corev1.EnvVar{Name: "SECRET_VOLUME", Value: "/mnt/secrets"})
	}
	if !pwdFileFLag {
		result = append(result, corev1.EnvVar{Name: "COMMON_OS_PWD_FILE", Value: "common_os_pwdfile.enc"})
	}
	if !pwdKeyFlag {
		result = append(result, corev1.EnvVar{Name: "PWD_KEY", Value: "pwd.key"})
	}
	if restype == "GSM" {
		if !sDirectParam {
			// varinfo = "director_name=sharddirector" + sDirectorCounter + ";director_region=primary;director_port=1521"
			varinfo = directorParams
			result = append(result, corev1.EnvVar{Name: "SHARD_DIRECTOR_PARAMS", Value: varinfo})
		}
		if !sGroup1Params {
			varinfo = "group_name=shardgroup1;deploy_as=primary;group_region=primary"
			result = append(result, corev1.EnvVar{Name: "SHARD1_GROUP_PARAMS", Value: varinfo})
		}
		if instance.Spec.IsDataGuard {
			if !sGroup2Params {
				varinfo = "group_name=shardgroup2;deploy_as=standby;group_region=standby"
				result = append(result, corev1.EnvVar{Name: "SHARD2_GROUP_PARAMS", Value: varinfo})
			}
		}
		if !catalogParams {
			varinfo = buildCatalogParams(instance)
			result = append(result, corev1.EnvVar{Name: "CATALOG_PARAMS", Value: varinfo})
		}

		if masterFlag == true {
			result = append(result, corev1.EnvVar{Name: "MASTER_GSM", Value: "true"})
		}
		result = append(result, corev1.EnvVar{Name: "CATALOG_SETUP", Value: "true"})
		result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "gsm"})
		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})
	}

	if restype == "SHARD" {
		result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "primaryshard"})
		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})
	}

	if restype == "CATALOG" {
		result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "catalog"})
		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})
	}

	if instance.Spec.IsClone {
		result = append(result, corev1.EnvVar{Name: "CLONE_DB", Value: "true"})
		if restype == "SHARD" {
			if !oldSidFlag {
				result = append(result, corev1.EnvVar{Name: "OLD_ORACLE_SID", Value: "GOLDCDB"})
			}
			if !oldPdbFlag {
				result = append(result, corev1.EnvVar{Name: "OLD_ORACLE_PDB", Value: "GOLDPDB"})
			}
		}
		if restype == "CATALOG" {
			if !oldSidFlag {
				result = append(result, corev1.EnvVar{Name: "OLD_ORACLE_SID", Value: "GOLDCDB"})
			}
			if !oldPdbFlag {
				result = append(result, corev1.EnvVar{Name: "OLD_ORACLE_PDB", Value: "GOLDPDB"})
			}
		}
	}

	return result
}

// FUnction to build the svc definition for catalog/shard and GSM
func buildSvcPortsDef(instance *databasealphav1.ShardingDatabase, resType string) []corev1.ServicePort {
	var result []corev1.ServicePort
	if len(instance.Spec.PortMappings) > 0 {
		for _, portMapping := range instance.Spec.PortMappings {
			servicePort :=
				corev1.ServicePort{
					Protocol: portMapping.Protocol,
					Port:     portMapping.Port,
					Name:     generatePortMapping(portMapping),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: portMapping.TargetPort,
					},
				}
			result = append(result, servicePort)
		}
	} else {
		if resType == "GSM" {
			result = append(result, corev1.ServicePort{Protocol: corev1.ProtocolTCP, Port: oraGSMPort, Name: generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraGSMPort, oraGSMPort)), TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraGSMPort}})
		} else {
			result = append(result, corev1.ServicePort{Protocol: corev1.ProtocolTCP, Port: oraDBPort, Name: generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraDBPort, oraDBPort)), TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraDBPort}})
		}
		result = append(result, corev1.ServicePort{Protocol: corev1.ProtocolTCP, Port: oraRemoteOnsPort, Name: generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraRemoteOnsPort, oraRemoteOnsPort)), TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraRemoteOnsPort}})
		result = append(result, corev1.ServicePort{Protocol: corev1.ProtocolTCP, Port: oraLocalOnsPort, Name: generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraLocalOnsPort, oraLocalOnsPort)), TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraLocalOnsPort}})
		result = append(result, corev1.ServicePort{Protocol: corev1.ProtocolTCP, Port: oraAgentPort, Name: generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraAgentPort, oraAgentPort)), TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraAgentPort}})
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
func generatePortMapping(portMapping databasealphav1.PortMapping) string {
	return generateName(fmt.Sprintf("%s-%d-%d-", "tcp",
		portMapping.Port, portMapping.TargetPort))
}

func LogMessages(msgtype string, msg string, err error, instance *databasealphav1.ShardingDatabase, logger logr.Logger) {
	// setting logrus formatter
	//logrus.SetFormatter(&logrus.JSONFormatter{})
	//logrus.SetOutput(os.Stdout)

	if msgtype == "DEBUG" && instance.Spec.IsDebug == true {
		if err != nil {
			logger.Error(err, msg)
		} else {
			logger.Info(msg)
		}
	} else if msgtype == "INFO" {
		logger.Info(msg)
	}
}

func GetGsmPodName(gsmName string) string {
	podName := gsmName
	return podName
}

func GetSidName(variables []databasealphav1.EnvironmentVariable, name string) string {
	var result string

	for _, variable := range variables {
		if variable.Name == "ORACLE_SID" {
			result = variable.Value
		}
	}
	if result == "" {
		result = strings.ToUpper(name)
	}
	return result
}

func GetPdbName(variables []databasealphav1.EnvironmentVariable, name string) string {
	var result string

	for _, variable := range variables {
		if variable.Name == "ORACLE_SID" {
			result = variable.Value
		}
	}
	if result == "" {
		result = strings.ToUpper(name) + "PDB"
	}
	return result
}

func getlabelsForGsm(instance *databasealphav1.ShardingDatabase) map[string]string {
	return buildLabelsForGsm(instance, "sharding")
}

func getlabelsForShard(instance *databasealphav1.ShardingDatabase) map[string]string {
	return buildLabelsForShard(instance, "sharding")
}

func getlabelsForCatalog(instance *databasealphav1.ShardingDatabase) map[string]string {
	return buildLabelsForCatalog(instance, "sharding")
}

func LabelsForProvShardKind(instance *databasealphav1.ShardingDatabase, sftype string,
) map[string]string {

	if sftype == "shard" {
		return buildLabelsForShard(instance, "sharding")
	}

	return nil

}

func CheckSfset(sfsetName string, instance *databasealphav1.ShardingDatabase, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfSetFound := &appsv1.StatefulSet{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      sfsetName,
		Namespace: instance.Spec.Namespace,
	}, sfSetFound)
	if err != nil {
		return sfSetFound, err
	}
	return sfSetFound, nil
}

func checkPvc(pvcName string, instance *databasealphav1.ShardingDatabase, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
	pvcFound := &corev1.PersistentVolumeClaim{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      pvcName,
		Namespace: instance.Spec.Namespace,
	}, pvcFound)
	if err != nil {
		return pvcFound, err
	}
	return pvcFound, nil
}

func DelPvc(pvcName string, instance *databasealphav1.ShardingDatabase, kClient client.Client, logger logr.Logger) error {

	LogMessages("DEBUG", "Inside the delPvc and received param: "+GetFmtStr(pvcName), nil, instance, logger)
	pvcFound, err := checkPvc(pvcName, instance, kClient)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in finding the pvc claim!", nil, instance, logger)
		return err
	}
	err = kClient.Delete(context.Background(), pvcFound)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in deleting the pvc claim!", nil, instance, logger)
		return err
	}
	return nil
}

func DelSvc(pvcName string, instance *databasealphav1.ShardingDatabase, kClient client.Client, logger logr.Logger) error {

	LogMessages("DEBUG", "Inside the delPvc and received param: "+GetFmtStr(pvcName), nil, instance, logger)
	pvcFound, err := checkPvc(pvcName, instance, kClient)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in finding the pvc claim!", nil, instance, logger)
		return err
	}
	err = kClient.Delete(context.Background(), pvcFound)
	if err != nil {
		LogMessages("DEBUG", "Error occurred in deleting the pvc claim!", nil, instance, logger)
		return err
	}
	return nil
}

func CheckSvc(svcName string, instance *databasealphav1.ShardingDatabase, kClient client.Client) (*corev1.Service, error) {
	svcFound := &corev1.Service{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      svcName,
		Namespace: instance.Spec.Namespace,
	}, svcFound)
	if err != nil {
		return svcFound, err
	}
	return svcFound, nil
}

func PodListValidation(podList *corev1.PodList, sfName string, instance *databasealphav1.ShardingDatabase, kClient client.Client,
) (bool, *corev1.Pod) {

	var isPodExist bool = false
	podInfo := &corev1.Pod{}
	var podNameStr string
	var err error
	if sfName != "" {
		podNameStr = sfName + "-"
	} else {
		podNameStr = "-"
	}

	for _, pod := range podList.Items {
		if strings.Contains(pod.Name, podNameStr) {
			err = checkPod(instance, &pod, kClient)
			if err != nil {
				isPodExist = false
			}
			err = checkPodStatus(&pod, kClient)
			if err != nil {
				isPodExist = false
			}
			err = checkContainerStatus(&pod, kClient)
			if err != nil {
				isPodExist = false
			} else {
				isPodExist = true
				podInfo = &pod
				break
			}
		}
	}
	return isPodExist, podInfo
}

func GetPodList(sfsetName string, resType string, instance *databasealphav1.ShardingDatabase, kClient client.Client,
) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(getlabelsForGsm(instance))
	if resType == "GSM" {
		labelSelector = labels.SelectorFromSet(getlabelsForGsm(instance))
	} else if resType == "SHARD" {
		labelSelector = labels.SelectorFromSet(getlabelsForShard(instance))
	} else if resType == "CATALOG" {
		labelSelector = labels.SelectorFromSet(getlabelsForCatalog(instance))
	} else {
		err1 := fmt.Errorf("Wrong resources type passed. Supported values are SHARD,GSM and CATALOG")
		return nil, err1
	}

	listOps := &client.ListOptions{Namespace: instance.Spec.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), podList, listOps)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

func checkPod(instance *databasealphav1.ShardingDatabase, pod *corev1.Pod, kClient client.Client,
) error {
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      pod.Name,
		Namespace: instance.Spec.Namespace,
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

//  Namespace related function

func AddNamespace(instance *databasealphav1.ShardingDatabase, kClient client.Client, logger logr.Logger,
) error {
	var msg string
	ns := &corev1.Namespace{}
	err := kClient.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.Namespace}, ns)
	if err != nil {
		//msg = "Namespace " + instance.Spec.Namespace + " doesn't exist! creating namespace"
		if errors.IsNotFound(err) {
			err = kClient.Create(context.TODO(), NewNamespace(instance.Spec.Namespace))
			if err != nil {
				msg = "Error in creating namespace!"
				LogMessages("Error", msg, nil, instance, logger)
				return err
			}
		} else {
			msg = "Error in finding namespace!"
			LogMessages("Error", msg, nil, instance, logger)
			return err
		}
	}
	return nil
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

func getOwnerRef(instance *databasealphav1.ShardingDatabase,
) []metav1.OwnerReference {

	var ownerRef []metav1.OwnerReference
	ownerRef = append(ownerRef, metav1.OwnerReference{Kind: instance.GroupVersionKind().Kind, APIVersion: instance.APIVersion, Name: instance.Name, UID: types.UID(instance.UID)})
	return ownerRef
}

func buildCatalogParams(instance *databasealphav1.ShardingDatabase) string {
	var variables []databasealphav1.EnvironmentVariable
	variables = instance.Spec.Catalog[0].EnvVars
	var result string
	var varinfo string
	var sidFlag bool = false
	var pdbFlag bool = false
	var portFlag bool = false
	var regionFlag bool = false
	var cnameFlag bool = false
	var chunksFlag bool = false
	var sidName string
	var pdbName string
	var cport string
	var cregion string
	var cname string
	var catchunks string

	result = "catalog_host=" + instance.Spec.Catalog[0].Name + "-0" + "." + instance.Spec.Catalog[0].Name + ";"
	for _, variable := range variables {
		if variable.Name == "ORACLE_SID" {
			sidFlag = true
			sidName = variable.Value
		}
		if variable.Name == "ORACLE_PDB" {
			pdbFlag = true
			pdbName = variable.Value
		}
		if variable.Name == "CATALOG_PORT" {
			portFlag = true
			cport = variable.Value
		}
		if variable.Name == "CATALOG_REGION" {
			regionFlag = true
			cregion = variable.Value
		}
		if variable.Name == "CATALOG_NAME" {
			cnameFlag = true
			cname = variable.Value
		}
		if variable.Name == "CATALOG_CHUNKS" {
			chunksFlag = true
			catchunks = variable.Value
		}
	}

	if !sidFlag {
		varinfo = "catalog_db=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + ";"
		result = result + varinfo
	} else {
		varinfo = "catalog_db=" + strings.ToUpper(sidName) + ";"
		result = result + varinfo
	}

	if !pdbFlag {
		varinfo = "catalog_pdb=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + "PDB" + ";"
		result = result + varinfo
	} else {
		varinfo = "catalog_pdb=" + strings.ToUpper(pdbName) + ";"
		result = result + varinfo
	}

	if !portFlag {
		varinfo = "catalog_port=" + "1521" + ";"
		result = result + varinfo
	} else {
		varinfo = "catalog_port=" + cport + ";"
		result = result + varinfo
	}

	if !cnameFlag {
		varinfo = "catalog_name=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + ";"
		result = result + varinfo
	} else {
		varinfo = "catalog_name=" + strings.ToUpper(cname) + ";"
		result = result + varinfo
	}
	if chunksFlag {
		result = result + "catalog_chunks=" + catchunks + ";"
	}

	if !regionFlag {
		varinfo = "catalog_region=primary,standby"
		result = result + varinfo
	} else {
		varinfo = "catalog_region=" + cregion
		result = result + varinfo
	}

	return result
}

func buildDirectorParams(instance *databasealphav1.ShardingDatabase, oraGsmSpex databasealphav1.GsmSpec, idx int) string {
	var variables []databasealphav1.EnvironmentVariable
	var result string
	var varinfo string
	var dnameFlag bool = false
	var dportFlag bool = false

	// Get the GSM Spec and build director params. idx feild is very important to build the unique director name and regiod. Idx is GSM array index.
	variables = oraGsmSpex.EnvVars
	for _, variable := range variables {
		if variable.Name == "DIRECTOR_NAME" {
			dnameFlag = true
		}
		if variable.Name == "DIRECTOR_PORT" {
			dportFlag = true
		}
	}
	if !dnameFlag {
		varinfo = "director_name=sharddirector" + strconv.Itoa(idx) + ";"
		result = result + varinfo
	}

	if idx == 0 {
		varinfo = "director_region=primary;"
		result = result + varinfo
	} else if idx == 1 {
		varinfo = "director_region=standby;"
		result = result + varinfo
	} else {
		// Do nothing
	}

	if !dportFlag {
		varinfo = "director_port=1522"
		result = result + varinfo
	}

	return result
}

func BuildShardParams(sfSet *appsv1.StatefulSet) string {
	var variables []corev1.EnvVar
	variables = sfSet.Spec.Template.Spec.Containers[0].Env
	var result string
	var varinfo string
	var isShardPort bool = false
	var isShardGrp bool = false

	result = "shard_host=" + sfSet.Name + "-0" + "." + sfSet.Name + ";"
	for _, variable := range variables {
		if variable.Name == "ORACLE_SID" {
			varinfo = "shard_db=" + variable.Value + ";"
			result = result + varinfo
		}
		if variable.Name == "ORACLE_PDB" {
			varinfo = "shard_pdb=" + variable.Value + ";"
			result = result + varinfo
		}
		if variable.Name == "SHARD_PORT" {
			varinfo = "shard_port=" + variable.Value + ";"
			result = result + varinfo
			isShardPort = true
		}
		if variable.Name == "SHARD_GROUP" {
			varinfo = "shard_group=" + variable.Value + ";"
			result = result + varinfo
			isShardGrp = true
		}
	}

	if !isShardPort {
		varinfo = "shard_port=" + "1521" + ";"
		result = result + varinfo
	}

	if !isShardGrp {
		varinfo = "shard_group=" + "shardgroup1"
		result = result + varinfo
	}

	return result
}

func labelsForShardingDatabaseKind(instance *databasealphav1.ShardingDatabase, sftype string,
) map[string]string {

	if sftype == "shard" {
		return buildLabelsForShard(instance, "sharding")
	}

	return nil

}

func removeAlpha(numStr string,
) string {

	reg, _ := regexp.Compile("[^0-9]+")
	processedString := reg.ReplaceAllString(numStr, "")
	numDigit := processedString + "Gi"
	return numDigit
}

func GetIpCmd(svcName string) []string {
	grepStr := " | grep PING | sed -e 's/).*//' | sed -e 's/.*(//'"
	var oragetIpCmd = []string{"/bin/bash", "-c", " ping -q -c 1 -t 1 " + svcName + grepStr}
	return oragetIpCmd
}

func getGsmSvcCmd() []string {
	var oragetGsmSvcCmd = []string{"/bin/bash", "-c", " $ORACLE_HOME/bin/gdsctl services | grep Service | awk -F ' ' '{ print $2 }' | tr '\n' ' ' "}
	return oragetGsmSvcCmd
}

func getDbRoleCmd() []string {
	sqlCmd := "echo -e 'set feedback off; \n set heading off; \n select database_role from v$database;' | sqlplus -S '/as sysdba' | tr '\n' ' '"
	var oraSqlCmd = []string{"/bin/bash", "-c", sqlCmd}
	return oraSqlCmd
}

func getDbModeCmd() []string {
	sqlCmd := "echo -e 'set feedback off; \n set heading off; \n select open_mode from v$database;' | sqlplus -S '/as sysdba' | tr '\n' ' '"
	var oraSqlCmd = []string{"/bin/bash", "-c", sqlCmd}
	return oraSqlCmd
}

func GetShardInviteNodeCmd(shardName string) []string {
	shard_host := shardName + "." + strings.Split(shardName, "-0")[0]
	var oraShardInviteCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--invitednode=" + strconv.Quote(shard_host), "--optype=gsm"}
	return oraShardInviteCmd
}

func getCancelChunksCmd(sparamStr string) []string {
	var cancelChunkCmd []string
	cancelChunkCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--cancelchunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return cancelChunkCmd
}

func getMoveChunksCmd(sparamStr string) []string {
	var moveChunkCmd []string
	moveChunkCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--movechunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return moveChunkCmd
}

func getNoChunksCmd(sparamStr string) []string {
	var noChunkCmd []string
	noChunkCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--validatenochunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return noChunkCmd
}

func shardValidationCmd() []string {

	var oraShardValidateCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true ", "--optype=primaryshard"}
	return oraShardValidateCmd
}

func getShardCheckCmd(sparamStr string) []string {
	var checkShardCmd []string
	checkShardCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkgsmshard=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return checkShardCmd
}

func getShardAddCmd(sparams string) []string {

	sparamStr := "--addshard=" + strconv.Quote(sparams)
	var addShardCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", sparamStr, "--optype=gsm"}
	return addShardCmd

}

func getShardDelCmd(sparams string) []string {
	sparamStr := "--deleteshard=" + strconv.Quote(sparams)
	var delShardCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", sparamStr}
	return delShardCmd
}

func getLivenessCmd(resType string) []string {
	var livenessCmd []string
	if resType == "SHARD" {
		livenessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=primaryshard"}
	}
	if resType == "CATALOG" {
		livenessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=catalog"}
	}
	if resType == "GSM" {
		livenessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=gsm"}
	}
	if resType == "STANDBY" {
		livenessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=standbyshard"}
	}
	return livenessCmd
}

func getGsmShardValidateCmd(shardName string) []string {
	var validateCmd []string
	validateCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--validateshard=" + strconv.Quote(shardName), "--optype=gsm"}
	return validateCmd
}

func getOnlineShardCmd(sparamStr string) []string {
	var onlineCmd []string
	onlineCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkonlineshard=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return onlineCmd
}

func getGsmAddShardGroupCmd(sparamStr string) []string {
	var addSgroupCmd []string
	addSgroupCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", sparamStr, "--optype=gsm"}
	return addSgroupCmd
}

func getdeployShardCmd() []string {
	var depCmd []string
	depCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--deployshard=true", "--optype=gsm"}
	return depCmd
}

func getGsmvalidateCmd() []string {
	var depCmd []string
	depCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=gsm"}
	return depCmd
}

func getInitContainerCmd(resType string, name string,
) string {
	var initCmd string
	if resType == "WEB" {
		initCmd = "chown -R 54321:54321 " + oraScriptMount + ";chmod 755 " + oraScriptMount + "/*;chown -R 54321:54321 /opt/oracle/oradata;chmod 750 /opt/oracle/oradata"
	} else {
		initCmd = resType + ";chown -R 54321:54321 " + oraScriptMount + ";chmod 755 " + oraScriptMount + "/*;chown -R 54321:54321 /opt/oracle/oradata;chmod 750 /opt/oracle/oradata"
	}
	return initCmd
}

func getGsmInitContainerCmd(resType string, name string,
) string {
	var initCmd string
	if resType == "WEB" {
		initCmd = "chown -R 54321:54321 " + oraScriptMount + ";chmod 755 " + oraScriptMount + "/*;chown -R 54321:54321 /opt/oracle/gsmdata;chmod 750 /opt/oracle/gsmdata"
	} else {
		initCmd = resType + ";chown -R 54321:54321 " + oraScriptMount + ";chmod 755 " + oraScriptMount + "/*;chown -R 54321:54321 /opt/oracle/gsmdata;chmod 750 /opt/oracle/gsmdata"
	}
	return initCmd
}

func GetFmtStr(pstr string,
) string {
	return "[" + pstr + "]"
}

func ReadConfigMap(cmName string, instance *databasealphav1.ShardingDatabase, kClient client.Client, logger logr.Logger,
) (string, string, string, string, string, string) {

	var region, fingerprint, user, tenancy, passphrase, str1, topicid, k, value string
	cm := &corev1.ConfigMap{}
	var err error

	// Reding a config map
	err = kClient.Get(context.TODO(), types.NamespacedName{
		Name:      cmName,
		Namespace: instance.Spec.Namespace,
	}, cm)

	if err != nil {
		return "NONE", "NONE", "NONE", "NONE", "NONE", "None"
	}

	// ConfigMap evaluation
	cmMap1 := cm.Data
	for k, value = range cmMap1 {
		LogMessages("DEBUG", "Key : "+GetFmtStr(k)+" Value : "+GetFmtStr(value), nil, instance, logger)
		str1 = value
	}

	for _, line := range strings.Split(strings.TrimSuffix(str1, "\n"), "\n") {
		s := strings.Index(line, "=")
		if s == -1 {
			continue
		}
		k = line[:s]
		value = line[s+1:]

		LogMessages("DEBUG", "Key : "+GetFmtStr(k)+" Value : "+GetFmtStr(value), nil, instance, logger)
		if k == "region" {
			region = value
		} else if k == "fingerprint" {
			fingerprint = value
		} else if k == "user" {
			user = value
		} else if k == "tenancy" {
			tenancy = value
		} else if k == "passpharase" {
			passphrase = value
		} else if k == "topicid" {
			topicid = value
		} else {
			LogMessages("DEBUG", GetFmtStr(k)+" is not matching with any required value for ONS.", nil, instance, logger)
		}
	}
	return region, user, tenancy, passphrase, fingerprint, topicid
}

func ReadSecret(secName string, instance *databasealphav1.ShardingDatabase, kClient client.Client, logger logr.Logger,
) string {

	var value string
	sc := &corev1.Secret{}
	var err error

	// Reading a Secret
	err = kClient.Get(context.TODO(), types.NamespacedName{
		Name:      secName,
		Namespace: instance.Spec.Namespace,
	}, sc)

	if err != nil {
		return "NONE"
	}

	// Secret Evaluation
	for k, val := range sc.Data {
		if k == "privatekey" {
			LogMessages("DEBUG", "Key : "+GetFmtStr(k)+" Value : "+GetFmtStr(value)+"   Val: "+GetFmtStr(string(val)), nil, instance, logger)
		}
	}

	return string(sc.Data["privatekey"])
}

func GetK8sClientConfig(kClient client.Client) (clientcmd.ClientConfig, kubernetes.Interface, error) {
	var err1 error
	var kubeConfig clientcmd.ClientConfig
	var kubeClient kubernetes.Interface

	databasealphav1.KubeConfigOnce.Do(func() {
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

func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Function to check shadrd in GSM
func CheckShardInGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	err, _, _ := ExecCommand(gsmPodName, getShardCheckCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Did not find the shard " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to check the online Shard
func CheckOnlineShardInGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	err, _, _ := ExecCommand(gsmPodName, getOnlineShardCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Shard: " + GetFmtStr(sparams) + " is not onine in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to move the chunks
func MoveChunks(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	err, _, _ := ExecCommand(gsmPodName, getMoveChunksCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred in during Chunk movement command submission for shard: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to verify the chunks
func VerifyChunks(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	err, _, _ := ExecCommand(gsmPodName, getNoChunksCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Chunks are not moved completely from the shard: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to verify the chunks
func AddShardInGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	err, _, _ := ExecCommand(gsmPodName, getShardAddCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while adding a shard " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to deploy the Shards
func DeployShardInGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	err, _, _ := ExecCommand(gsmPodName, getdeployShardCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while deploying the shard in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to verify the chunks
func CancelChunksInGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	err, _, _ := ExecCommand(gsmPodName, getCancelChunksCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while cancelling the chunks: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to delete the shard
func RemoveShardFromGsm(gsmPodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	err, _, _ := ExecCommand(gsmPodName, getShardDelCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while cancelling the chunks: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

func GetSvcIp(PodName string, sparams string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) (error, string, string) {
	err, stdoutput, stderror := ExecCommand(PodName, GetIpCmd(sparams), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the IP for k8s service " + GetFmtStr(sparams)
		LogMessages("INFO", msg, nil, instance, logger)
		return err, strings.Replace(stdoutput, "\r\n", "", -1), strings.Replace(stderror, "/r/n", "", -1)
	}
	return nil, strings.Replace(stdoutput, "\r\n", "", -1), strings.Replace(stderror, "/r/n", "", -1)
}

func GetGsmServices(PodName string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	err, stdoutput, _ := ExecCommand(PodName, getGsmSvcCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the services from the GSM "
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return stdoutput
}

func GetDbRole(PodName string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	err, stdoutput, _ := ExecCommand(PodName, getDbRoleCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the DB role from the database"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func GetDbOpenMode(PodName string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) string {
	err, stdoutput, _ := ExecCommand(PodName, getDbModeCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the DB mode from the database"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func SfsetLabelPatch(sfSetFound *appsv1.StatefulSet, sfSetPod *corev1.Pod, instance *databasealphav1.ShardingDatabase, kClient client.Client,
) error {

	//var msg string
	//status = false
	var err error

	sfsetCopy := sfSetFound.DeepCopy()
	sfsetCopy.Labels[string(databasealphav1.ShardingDelLabelKey)] = string(databasealphav1.ShardingDelLabelTrueValue)
	patch := client.MergeFrom(sfSetFound)
	err = kClient.Patch(context.Background(), sfsetCopy, patch)
	if err != nil {
		return err
	}

	podCopy := sfSetPod.DeepCopy()
	podCopy.Labels[string(databasealphav1.ShardingDelLabelKey)] = string(databasealphav1.ShardingDelLabelTrueValue)
	podPatch := client.MergeFrom(sfSetPod.DeepCopy())
	err = kClient.Patch(context.Background(), podCopy, podPatch)
	if err != nil {
		return err
	}

	return nil
}

// Send Notification

func SendNotification(title string, body string, instance *databasealphav1.ShardingDatabase, topicId string, rclient ons.NotificationDataPlaneClient, logger logr.Logger,
) {
	var msg string
	req := ons.PublishMessageRequest{TopicId: common.String(topicId),
		MessageDetails: ons.MessageDetails{
			Title: common.String(title),
			Body:  common.String(body)}}

	// Send the request using the service client
	_, err := rclient.PublishMessage(context.Background(), req)
	if err != nil {
		msg = "Error occurred in sending the message. Title: " + GetFmtStr(title)
		logger.Error(err, "Error occurred while sending a notification")
		LogMessages("DEBUG", msg, nil, instance, logger)
	}
}
