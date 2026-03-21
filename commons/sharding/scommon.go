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
	"encoding/json"
	"fmt"
	"slices"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

	"regexp"
	"strconv"
	"strings"

	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"
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
	oraDbScriptMount          = "/opt/oracle/scripts/sharding"
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
	TmpLoc                    = "/var/tmp"
	connectFailureMaxTries    = 5
	errorDialingBackendEOF    = "error dialing backend: EOF"
)

var nonDigitRegex = regexp.MustCompile("[^0-9]+")

func upsertEnv(env []corev1.EnvVar, v corev1.EnvVar) []corev1.EnvVar {
	for i := range env {
		if env[i].Name == v.Name {
			env[i] = v
			return env
		}
	}
	return append(env, v)
}

// buildExecProbe creates a Kubernetes exec probe from a command vector.
func buildExecProbe(command []string, initialDelay, period, timeout, failure int32) *corev1.Probe {
	return &corev1.Probe{
		FailureThreshold:    failure,
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
		TimeoutSeconds:      timeout,
		SuccessThreshold:    1,
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: command,
			},
		},
	}
}

// buildShellExecProbe creates a shell-based exec probe.
func buildShellExecProbe(cmd string, initialDelay, period, timeout, failure int32) *corev1.Probe {
	return buildExecProbe([]string{"/bin/sh", "-c", cmd}, initialDelay, period, timeout, failure)
}

// Function to build the env var specification
func buildEnvVarsSpec(
	instance *databasev4.ShardingDatabase,
	variables []databasev4.EnvironmentVariable,
	name string,
	restype string,
	masterFlag bool,
	directorParams string,
	deployAs string,
	primaryRef *databasev4.DatabaseRef, // for standby linking
) []corev1.EnvVar {
	result := make([]corev1.EnvVar, 0, len(variables)+32)
	varinfo := ""

	isShard := restype == "SHARD"
	isCatalog := restype == "CATALOG"
	isGSM := restype == "GSM"
	isFreeEdition := strings.EqualFold(instance.Spec.DbEdition, "free")
	isUserSharding := strings.EqualFold(instance.Spec.ShardingType, "USER")

	var sidFlag bool
	var pdbValue string
	var pdbFlag bool
	var sDirectParam bool
	var sGroup1Params bool
	var catalogParams bool
	var oldPdbFlag bool
	var oldSidFlag bool
	var archiveLogFlag bool
	var shardSetupFlag bool
	var dbUnameFlag bool
	var ofreePdbFlag bool
	var standbyDbFlag bool

	for _, variable := range variables {
		switch variable.Name {
		case "ORACLE_SID":
			sidFlag = true
		case "ORACLE_PDB":
			pdbFlag = true
			pdbValue = variable.Value
		case "SHARD_DIRECTOR_PARAMS":
			sDirectParam = true
		case "SHARD1_GROUP_PARAMS":
			sGroup1Params = true
		case "CATALOG_PARAMS":
			catalogParams = true
		case "OLD_ORACLE_SID":
			oldSidFlag = true
		case "OLD_ORACLE_PDB":
			oldPdbFlag = true
		case "SHARD_SETUP":
			shardSetupFlag = true
		case "ENABLE_ARCHIVELOG":
			archiveLogFlag = true
		case "DB_UNIQUE_NAME":
			dbUnameFlag = true
		case "ORACLE_FREE_PDB":
			ofreePdbFlag = true
		case "STANDBY_DB":
			standbyDbFlag = true
		}
		result = append(result, corev1.EnvVar{Name: variable.Name, Value: variable.Value})
	}

	if !dbUnameFlag && (isShard || isCatalog || isFreeEdition) {
		result = append(result, corev1.EnvVar{Name: "DB_UNIQUE_NAME", Value: strings.ToUpper(name)})
	}

	if !ofreePdbFlag && isFreeEdition {
		if pdbFlag {
			result = append(result, corev1.EnvVar{Name: "ORACLE_FREE_PDB", Value: pdbValue})
		} else {
			result = append(result, corev1.EnvVar{Name: "ORACLE_FREE_PDB", Value: strings.ToUpper(name) + "PDB"})
		}
	}

	if !shardSetupFlag && (isShard || isCatalog || isGSM) {
		result = append(result, corev1.EnvVar{Name: "SHARD_SETUP", Value: "true"})
	}

	if !archiveLogFlag && (isShard || isCatalog) {
		result = append(result, corev1.EnvVar{Name: "ENABLE_ARCHIVELOG", Value: "true"})
	}

	if !sidFlag {
		if isFreeEdition {
			result = append(result, corev1.EnvVar{Name: "ORACLE_SID", Value: "FREE"})
		} else if isShard || isCatalog {
			result = append(result, corev1.EnvVar{Name: "ORACLE_SID", Value: strings.ToUpper(name)})
		}
	}

	if !pdbFlag {
		if isFreeEdition {
			result = append(result, corev1.EnvVar{Name: "ORACLE_PDB", Value: "FREEPDB"})
		} else if isShard || isCatalog {
			result = append(result, corev1.EnvVar{Name: "ORACLE_PDB", Value: strings.ToUpper(name) + "PDB"})
		}
	}

	if !strings.EqualFold(instance.Spec.DbSecret.EncryptionType, "base64") {
		result = append(result, corev1.EnvVar{Name: "PWD_KEY", Value: instance.Spec.DbSecret.KeyFileName})
		result = append(result, corev1.EnvVar{Name: "COMMON_OS_PWD_FILE", Value: instance.Spec.DbSecret.PwdFileName})
	} else {
		result = append(result, corev1.EnvVar{Name: "PASSWORD_FILE", Value: instance.Spec.DbSecret.PwdFileName})
	}

	if len(instance.Spec.DbSecret.PwdFileMountLocation) != 0 {
		result = append(result, corev1.EnvVar{Name: "SECRET_VOLUME", Value: instance.Spec.DbSecret.PwdFileMountLocation})
	} else {
		result = append(result, corev1.EnvVar{Name: "SECRET_VOLUME", Value: oraSecretMount})
	}

	if len(instance.Spec.DbSecret.KeyFileMountLocation) != 0 {
		result = append(result, corev1.EnvVar{Name: "KEY_SECRET_VOLUME", Value: instance.Spec.DbSecret.KeyFileMountLocation})
	} else {
		result = append(result, corev1.EnvVar{Name: "KEY_SECRET_VOLUME", Value: oraSecretMount})
	}

	if checkTdeWalletFlag(instance) {
		result = append(result, corev1.EnvVar{Name: "TDE_PWD_KEY", Value: instance.Spec.DbSecret.TdeKeyFileName})
		result = append(result, corev1.EnvVar{Name: "TDE_PWD_FILE", Value: instance.Spec.DbSecret.TdePwdFileName})
	}

	if isGSM {
		if !sDirectParam {
			result = append(result, corev1.EnvVar{Name: "SHARD_DIRECTOR_PARAMS", Value: directorParams})
		}

		if !isUserSharding {
			if !sGroup1Params {
				for i := range instance.Spec.ShardGroup {
					groupName := instance.Spec.ShardGroup[i].Name
					region := instance.Spec.ShardGroup[i].Region
					switch strings.ToUpper(instance.Spec.ShardGroup[i].DeployAs) {
					case "PRIMARY":
						varinfo = "group_name=" + groupName + ";" + "deploy_as=primary;" + "group_region=" + region
						result = append(result, corev1.EnvVar{Name: "SHARD1_GROUP_PARAMS", Value: varinfo})
					case "STANDBY":
						varinfo = "group_name=" + groupName + ";" + "deploy_as=standby;" + "group_region=" + region
						result = append(result, corev1.EnvVar{Name: "SHARD2_GROUP_PARAMS", Value: varinfo})
					}
				}
			} else {
				result = append(result, corev1.EnvVar{Name: "SHARD1_GROUP_PARAMS", Value: "group_name=shardgroup1;deploy_as=primary;group_region=primary"})
			}
		}

		if isUserSharding {
			result = append(result, corev1.EnvVar{Name: "SHARDING_TYPE", Value: "USER"})
		}

		for i := range instance.Spec.GsmService {
			svc := "service_name=" + instance.Spec.GsmService[i].Name
			if len(instance.Spec.GsmService[i].Role) != 0 {
				svc += ";service_role=" + instance.Spec.GsmService[i].Role
			} else {
				svc += ";service_role=primary"
			}
			if len(instance.Spec.GsmService[i].RuMode) != 0 {
				svc += ";service_mode=" + instance.Spec.GsmService[i].RuMode
			}
			result = append(result, corev1.EnvVar{Name: "SERVICE" + fmt.Sprint(i) + "_PARAMS", Value: svc})
		}

		if !strings.EqualFold(instance.Spec.GsmDevMode, "false") {
			result = append(result, corev1.EnvVar{Name: "DEV_MODE", Value: "TRUE"})
		}

		invitedSubnetFlag := strings.TrimSpace(instance.Spec.InvitedNodeSubnetFlag)
		if invitedSubnetFlag == "" || !strings.EqualFold(invitedSubnetFlag, "false") {
			result = append(result, corev1.EnvVar{Name: "INVITED_NODE_SUBNET_FLAG", Value: "TRUE"})
			if strings.TrimSpace(instance.Spec.InvitedNodeSubnet) != "" {
				result = append(result, corev1.EnvVar{Name: "INVITED_NODE_SUBNET", Value: instance.Spec.InvitedNodeSubnet})
			}
		}

		if !catalogParams {
			result = append(result, corev1.EnvVar{Name: "CATALOG_PARAMS", Value: buildCatalogParams(instance)})
		}

		if masterFlag {
			result = append(result, corev1.EnvVar{Name: "MASTER_GSM", Value: "true"})
		}

		result = append(result, corev1.EnvVar{Name: "CATALOG_SETUP", Value: "true"})
		result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "gsm"})
		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})
	}

	if isShard {
		role := strings.ToUpper(strings.TrimSpace(deployAs))
		switch role {
		case "STANDBY":
			result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "standbyshard"})
		case "ACTIVE_STANDBY":
			result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "active_standby_shard"})
		default:
			result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "primaryshard"})
		}

		if !standbyDbFlag && (role == "STANDBY" || role == "ACTIVE_STANDBY") {
			result = append(result, corev1.EnvVar{Name: "STANDBY_DB", Value: "true"})
		}

		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})

		dbu := strings.ToUpper(strings.TrimSpace(name))
		for _, v := range result {
			if v.Name == "DB_UNIQUE_NAME" && strings.TrimSpace(v.Value) != "" {
				dbu = strings.ToUpper(strings.TrimSpace(v.Value))
				break
			}
		}
		cfg1 := fmt.Sprintf("%s/dbconfig/%s/dr1%s.dat", oraDataMount, dbu, dbu)
		cfg2 := fmt.Sprintf("%s/dbconfig/%s/dr2%s.dat", oraDataMount, dbu, dbu)
		result = upsertEnv(result, corev1.EnvVar{Name: "DG_BROKER_CONFIG_FILE1", Value: cfg1})
		result = upsertEnv(result, corev1.EnvVar{Name: "DG_BROKER_CONFIG_FILE2", Value: cfg2})

		if (role == "STANDBY" || role == "ACTIVE_STANDBY") &&
			primaryRef != nil &&
			strings.TrimSpace(primaryRef.Host) != "" {

			host := strings.TrimSpace(primaryRef.Host)
			port := "1521"
			if primaryRef.Port > 0 {
				port = fmt.Sprint(primaryRef.Port)
			}

			result = append(result, corev1.EnvVar{Name: "PRIMARY_DB_HOST", Value: host})
			result = append(result, corev1.EnvVar{Name: "PRIMARY_DB_PORT", Value: port})
			if strings.TrimSpace(primaryRef.CdbName) != "" {
				result = append(result, corev1.EnvVar{Name: "PRIMARY_CDB_NAME", Value: strings.TrimSpace(primaryRef.CdbName)})
			}
			if strings.TrimSpace(primaryRef.PdbName) != "" {
				result = append(result, corev1.EnvVar{Name: "PRIMARY_PDB_NAME", Value: strings.TrimSpace(primaryRef.PdbName)})
			}

			svc := strings.TrimSpace(primaryRef.CdbName)
			if svc == "" {
				svc = strings.TrimSpace(primaryRef.PdbName)
			}

			connNoSlash := host + ":" + port
			connWithSlash := "//" + host + ":" + port
			if svc != "" {
				connNoSlash = connNoSlash + "/" + svc
				connWithSlash = connWithSlash + "/" + svc
			}

			result = upsertEnv(result, corev1.EnvVar{Name: "PRIMARY_DB_CONN_STR", Value: connNoSlash})
			result = upsertEnv(result, corev1.EnvVar{Name: "PRIMARY_CONNECT", Value: connNoSlash})
			result = upsertEnv(result, corev1.EnvVar{Name: "PRIMARY_DB_CONN_STR_NOSLASH", Value: connNoSlash})
			result = upsertEnv(result, corev1.EnvVar{Name: "PRIMARY_DB_CONN_STR_WITHSLASH", Value: connWithSlash})
		}
	}

	if isCatalog {
		result = append(result, corev1.EnvVar{Name: "OP_TYPE", Value: "catalog"})
		result = append(result, corev1.EnvVar{Name: "KUBE_SVC", Value: name})
	}

	if instance.Spec.IsClone {
		result = append(result, corev1.EnvVar{Name: "CLONE_DB", Value: "true"})
		if isShard || isCatalog {
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
func buildSvcPortsDef(instance *databasev4.ShardingDatabase, resType string) []corev1.ServicePort {
	if len(instance.Spec.PortMappings) > 0 {
		result := make([]corev1.ServicePort, 0, len(instance.Spec.PortMappings))
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
		return result
	}

	defaultDataPort := int32(oraDBPort)
	if resType == "GSM" {
		defaultDataPort = int32(oraGSMPort)
	}
	result := make([]corev1.ServicePort, 0, 4)
	result = append(result, corev1.ServicePort{
		Protocol:   corev1.ProtocolTCP,
		Port:       defaultDataPort,
		Name:       generateName(fmt.Sprintf("%s-%d-%d-", "tcp", defaultDataPort, defaultDataPort)),
		TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: defaultDataPort},
	})
	result = append(result, corev1.ServicePort{
		Protocol:   corev1.ProtocolTCP,
		Port:       oraRemoteOnsPort,
		Name:       generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraRemoteOnsPort, oraRemoteOnsPort)),
		TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraRemoteOnsPort},
	})
	result = append(result, corev1.ServicePort{
		Protocol:   corev1.ProtocolTCP,
		Port:       oraLocalOnsPort,
		Name:       generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraLocalOnsPort, oraLocalOnsPort)),
		TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraLocalOnsPort},
	})
	result = append(result, corev1.ServicePort{
		Protocol:   corev1.ProtocolTCP,
		Port:       oraAgentPort,
		Name:       generateName(fmt.Sprintf("%s-%d-%d-", "tcp", oraAgentPort, oraAgentPort)),
		TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: oraAgentPort},
	})
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
func generatePortMapping(portMapping databasev4.PortMapping) string {
	return generateName(fmt.Sprintf("%s-%d-%d-", "tcp",
		portMapping.Port, portMapping.TargetPort))
}

func LogMessages(msgtype string, msg string, err error, instance *databasev4.ShardingDatabase, logger logr.Logger) {
	level := strings.ToUpper(strings.TrimSpace(msgtype))
	log := logger.WithValues("component", "sharding")

	if instance != nil {
		log = log.WithValues("namespace", instance.Namespace, "name", instance.Name)
	}

	switch level {
	case "DEBUG":
		if instance != nil && instance.Spec.IsDebug {
			if err != nil {
				log.Error(err, msg, "level", level)
			} else {
				log.Info(msg, "level", level)
			}
		}
	case "ERROR", "ERR", "FATAL", "WARN", "WARNING":
		// Preserve backward compatibility: route warning-like/error-like types through Error
		// because historical callers relied on prominent error visibility.
		log.Error(err, msg, "level", level)
	default:
		if err != nil {
			log.Info(msg, "level", level, "error", err.Error())
		} else {
			log.Info(msg, "level", level)
		}
	}
}

func GetSidName(variables []databasev4.EnvironmentVariable, name string) string {
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

func GetPdbName(variables []databasev4.EnvironmentVariable, name string) string {
	var result string

	for _, variable := range variables {
		if variable.Name == "ORACLE_PDB" {
			result = variable.Value
		}
	}
	if result == "" {
		result = strings.ToUpper(name) + "PDB"
	}
	return result
}

func getlabelsForGsm(instance *databasev4.ShardingDatabase) map[string]string {
	return LabelsForProvShardKind(instance, "gsm")
}

func getlabelsForShard(instance *databasev4.ShardingDatabase) map[string]string {
	return LabelsForProvShardKind(instance, "shard")
}

func getlabelsForCatalog(instance *databasev4.ShardingDatabase) map[string]string {
	return LabelsForProvShardKind(instance, "catalog")
}

func LabelsForProvShardKind(instance *databasev4.ShardingDatabase, sftype string,
) map[string]string {
	switch strings.ToLower(strings.TrimSpace(sftype)) {
	case "shard":
		return buildLabelsForShard(instance, "sharding", "shard")
	case "catalog":
		return buildLabelsForCatalog(instance, "sharding", "catalog")
	case "gsm":
		return buildLabelsForGsm(instance, "sharding", "gsm")
	default:
		return map[string]string{}
	}
}

func CheckSfset(sfsetName string, instance *databasev4.ShardingDatabase, kClient client.Client) (*appsv1.StatefulSet, error) {
	sfSetFound := &appsv1.StatefulSet{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      sfsetName,
		Namespace: instance.Namespace,
	}, sfSetFound)
	if err != nil {
		return sfSetFound, err
	}
	return sfSetFound, nil
}

func checkPvc(pvcName string, instance *databasev4.ShardingDatabase, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
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

func DelPvc(pvcName string, instance *databasev4.ShardingDatabase, kClient client.Client, logger logr.Logger) error {

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

func CheckSvc(svcName string, instance *databasev4.ShardingDatabase, kClient client.Client) (*corev1.Service, error) {
	svcFound := &corev1.Service{}
	err := kClient.Get(context.Background(), types.NamespacedName{
		Name:      svcName,
		Namespace: instance.Namespace,
	}, svcFound)
	if err != nil {
		return svcFound, err
	}
	return svcFound, nil
}

func PodListValidation(podList *corev1.PodList, sfName string, instance *databasev4.ShardingDatabase, kClient client.Client,
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

func GetPodList(sfsetName string, resType string, instance *databasev4.ShardingDatabase, kClient client.Client,
) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	//labelSelector := labels.SelectorFromSet(getlabelsForGsm(instance))
	//labelSelector := map[string]labels.Selector{}
	var labelSelector labels.Selector

	//labels.SelectorFromSet()

	switch resType {
	case "GSM":
		labelSelector = labels.SelectorFromSet(getlabelsForGsm(instance))
	case "SHARD":
		labelSelector = labels.SelectorFromSet(getlabelsForShard(instance))
	case "CATALOG":
		labelSelector = labels.SelectorFromSet(getlabelsForCatalog(instance))
	default:
		err1 := fmt.Errorf("wrong resources type passed. Supported values are SHARD,GSM and CATALOG")
		return nil, err1
	}

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}

	err := kClient.List(context.TODO(), podList, listOps)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

func checkPod(instance *databasev4.ShardingDatabase, pod *corev1.Pod, kClient client.Client,
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

func getOwnerRef(instance *databasev4.ShardingDatabase,
) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(instance, databasev4.GroupVersion.WithKind("ShardingDatabase")),
	}
}

func buildCatalogParams(instance *databasev4.ShardingDatabase) string {
	var variables []databasev4.EnvironmentVariable = instance.Spec.Catalog[0].EnvVars
	var result string
	var varinfo string
	var sidFlag bool = false
	var pdbFlag bool = false
	var portFlag bool = false
	var cnameFlag bool = false
	var chunksFlag bool = false
	var sidName string
	var pdbName string
	var cport string
	var cname string
	var catchunks string
	var catalog_region, shard_space string

	result = "catalog_host=" + instance.Spec.Catalog[0].Name + "-0" + "." + instance.Spec.Catalog[0].Name + ";"

	//Checking if replcia type set to native
	var sspace_arr []string
	if strings.ToUpper(instance.Spec.ShardingType) == "USER" {
		shard_space = ""
		result = result + "sharding_type=user;"
		for i := 0; i < len(instance.Spec.Shard); i++ {
			sspace_arr = append(sspace_arr, instance.Spec.Shard[i].ShardSpace)
		}
		slices.Sort(sspace_arr)
		sspace_arr = slices.Compact(sspace_arr) //[a b c d]
		for i := 0; i < len(sspace_arr); i++ {
			shard_space = shard_space + sspace_arr[i] + ","
		}
		shard_space = strings.TrimSuffix(shard_space, ",")
		result = result + "shard_space=" + shard_space + ";"
	} else if strings.ToUpper(instance.Spec.ReplicationType) == "NATIVE" {
		result = result + "repl_type=native;"
	} else {
		fmt.Fprintln(os.Stdout, []any{""}...)
	}

	var region_arr []string
	for i := 0; i < len(instance.Spec.Shard); i++ {
		region_arr = append(region_arr, instance.Spec.Shard[i].ShardRegion)
	}

	for i := 0; i < len(instance.Spec.Gsm); i++ {
		region_arr = append(region_arr, instance.Spec.Gsm[i].Region)
	}

	slices.Sort(region_arr)
	region_arr = slices.Compact(region_arr) //[a b c d]
	for i := 0; i < len(region_arr); i++ {
		catalog_region = catalog_region + region_arr[i] + ","
	}
	catalog_region = strings.TrimSuffix(catalog_region, ",")
	result = result + "catalog_region=" + catalog_region + ";"

	if len(instance.Spec.ShardConfigName) != 0 {
		result = result + "shard_configname=" + instance.Spec.ShardConfigName + ";"
	}

	for _, variable := range variables {
		if variable.Name == "DB_UNIQUE_NAME" {
			sidFlag = true
			sidName = variable.Value
		} else {
			if variable.Name == "ORACLE_SID" {
				sidFlag = true
				sidName = variable.Value
			}
		}
		if variable.Name == "ORACLE_FREE_PDB" {
			if strings.ToLower(instance.Spec.DbEdition) == "free" {
				pdbFlag = true
				pdbName = variable.Value
			}
		}
		if strings.ToLower(instance.Spec.DbEdition) != "free" {
			if variable.Name == "ORACLE_PDB" {
				pdbFlag = true
				pdbName = variable.Value
			}
		}
		if variable.Name == "CATALOG_PORT" {
			portFlag = true
			cport = variable.Value
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
		if strings.ToLower(instance.Spec.DbEdition) == "free" {
			varinfo = "catalog_db=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + ";"
			result = result + varinfo
		} else {
			varinfo = "catalog_db=" + strings.ToUpper(sidName) + ";"
			result = result + varinfo
		}
	}

	if !pdbFlag {
		varinfo = "catalog_pdb=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + "PDB" + ";"
		result = result + varinfo
	} else {
		if strings.ToLower(instance.Spec.DbEdition) == "free" {
			varinfo = "catalog_pdb=" + strings.ToUpper(instance.Spec.Catalog[0].Name) + "PDB" + ";"
			result = result + varinfo
		} else {
			varinfo = "catalog_pdb=" + strings.ToUpper(pdbName) + ";"
			result = result + varinfo
		}
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
	} else {
		if strings.ToLower(instance.Spec.DbEdition) == "free" && strings.ToUpper(instance.Spec.ShardingType) != "USER" && strings.ToUpper(instance.Spec.ShardingType) != "NATIVE" {
			result = result + "catalog_chunks=12;"
		}
	}
	result = strings.TrimSuffix(result, ";")
	return result
}

func buildDirectorParams(instance *databasev4.ShardingDatabase, oraGsmSpex databasev4.GsmSpec, idx int) string {
	var variables []databasev4.EnvironmentVariable
	var result string
	var varinfo string
	var dnameFlag bool = false
	var dportFlag bool = false
	var dname string
	var dport string

	// Get the GSM Spec and build director params. idx feild is very important to build the unique director name and regiod. Idx is GSM array index.
	variables = oraGsmSpex.EnvVars
	for _, variable := range variables {
		if variable.Name == "DIRECTOR_NAME" {
			dnameFlag = true
			dname = variable.Value
		}
		if variable.Name == "DIRECTOR_PORT" {
			dportFlag = true
			dport = variable.Value
		}
	}
	if !dnameFlag {
		varinfo = "director_name=sharddirector" + oraGsmSpex.Name + ";"
		result = result + varinfo
	} else {
		varinfo = "director_name=" + dname + ";"
		result = result + varinfo
	}

	if oraGsmSpex.Region != "" {
		varinfo = "director_region=" + oraGsmSpex.Region + ";"
		result = result + varinfo
	} else {
		switch idx {
		case 0:
			varinfo = "director_region=primary;"
			result = result + varinfo
		case 1:
			varinfo = "director_region=standby;"
			result = result + varinfo
		default:
			// Do nothing
		}
		result = result + varinfo
	}

	if !dportFlag {
		varinfo = "director_port=1522"
		result = result + varinfo
	} else {
		varinfo = "director_port=" + dport
		result = result + varinfo
	}

	result = strings.TrimSuffix(result, ";")
	return result
}

func BuildShardParams(instance *databasev4.ShardingDatabase, sfSet *appsv1.StatefulSet, OraShardSpex databasev4.ShardSpec) string {
	shardName := strings.TrimSpace(OraShardSpex.Name)
	if sfSet != nil && strings.TrimSpace(sfSet.Name) != "" {
		shardName = sfSet.Name
	}

	var variables []corev1.EnvVar
	if sfSet != nil && len(sfSet.Spec.Template.Spec.Containers) > 0 {
		variables = sfSet.Spec.Template.Spec.Containers[0].Env
	}

	var (
		dbUniqueName string
		sidName      string
		freePdbValue string
		pdbValue     string
		shardPort    string
	)

	for _, variable := range variables {
		switch variable.Name {
		case "DB_UNIQUE_NAME":
			dbUniqueName = strings.TrimSpace(variable.Value)
		case "ORACLE_SID":
			sidName = strings.TrimSpace(variable.Value)
		case "ORACLE_FREE_PDB":
			freePdbValue = strings.TrimSpace(variable.Value)
		case "ORACLE_PDB":
			pdbValue = strings.TrimSpace(variable.Value)
		case "SHARD_PORT":
			shardPort = strings.TrimSpace(variable.Value)
		}
	}

	isFreeEdition := strings.EqualFold(strings.TrimSpace(instance.Spec.DbEdition), "free")
	params := make([]string, 0, 8)
	params = append(params, "shard_host="+shardName+"-0."+shardName)

	if dbUniqueName != "" {
		params = append(params, "shard_db="+dbUniqueName)
	} else if sidName != "" {
		if !isFreeEdition {
			params = append(params, "shard_db="+sidName)
		} else {
			params = append(params, "shard_db="+shardName)
		}
	} else if !isFreeEdition {
		params = append(params, "shard_db="+shardName)
	}

	if isFreeEdition {
		if freePdbValue != "" {
			params = append(params, "shard_pdb="+freePdbValue)
		}
	} else if pdbValue != "" {
		params = append(params, "shard_pdb="+pdbValue)
	}

	if v := strings.TrimSpace(OraShardSpex.ShardGroup); v != "" {
		params = append(params, "shard_group="+v)
	}
	if v := strings.TrimSpace(OraShardSpex.ShardSpace); v != "" {
		params = append(params, "shard_space="+v)
	}
	if v := strings.TrimSpace(OraShardSpex.ShardRegion); v != "" {
		params = append(params, "shard_region="+v)
	}

	if shardPort == "" {
		shardPort = "1521"
	}
	params = append(params, "shard_port="+shardPort)

	return strings.Join(params, ";")
}

func BuildShardParamsForAdd(
	instance *databasev4.ShardingDatabase,
	sfSet *appsv1.StatefulSet,
	OraShardSpex databasev4.ShardSpec,
) string {
	// start with existing params (NO deploy_as)
	p := BuildShardParams(instance, sfSet, OraShardSpex)

	// append deploy_as only for addshard command
	deployAs := strings.ToLower(strings.TrimSpace(OraShardSpex.DeployAs))
	switch deployAs {
	case "standby", "active_standby":
		// keep
	case "", "primary":
		deployAs = "primary"
	default:
		deployAs = "primary"
	}

	if p != "" {
		p += ";"
	}
	p += "deploy_as=" + deployAs
	return p
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

// func GetPreStandbySetupCmd(primaryPodName string) []string {
// 	shard_host := primaryPodName + "." + strings.Split(primaryPodName, "-0")[0]
// 	var preStandbyCmd = []string{
// 		oraDbScriptMount + "/cmdExec",
// 		"/bin/python",
// 		oraDbScriptMount + "/main.py ",
// 		"--prestandbysetup=" + strconv.Quote(shard_host),
// 		"--optype=primaryshard",
// 	}
// 	return preStandbyCmd
// }

func getCancelChunksCmd(sparamStr string) []string {
	var cancelChunkCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--cancelchunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return cancelChunkCmd
}

func getMoveChunksCmd(sparamStr string) []string {
	var moveChunkCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--movechunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return moveChunkCmd
}

func getNoChunksCmd(sparamStr string) []string {
	var noChunkCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--validatenochunks=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return noChunkCmd
}

func shardValidationCmd() []string {

	var oraShardValidateCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkliveness=true ", "--optype=primaryshard"}
	return oraShardValidateCmd
}

func getShardCheckCmd(sparamStr string) []string {
	var checkShardCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkgsmshard=" + strconv.Quote(sparamStr), "--optype=gsm"}
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
		livenessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkliveness=true", "--optype=primaryshard"}
	}
	if resType == "CATALOG" {
		livenessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkliveness=true", "--optype=catalog"}
	}
	if resType == "GSM" {
		livenessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=gsm"}
	}
	if resType == "STANDBY" {
		livenessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkliveness=true", "--optype=standbyshard"}
	}
	return livenessCmd
}

func getReadinessCmd(resType string) []string {
	var readynessCmd []string
	if resType == "SHARD" {
		readynessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkreadyness=true", "--optype=primaryshard"}
	}
	if resType == "CATALOG" {
		readynessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkreadyness=true", "--optype=catalog"}
	}
	if resType == "GSM" {
		readynessCmd = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkreadyness=true", "--optype=gsm"}
	}
	if resType == "STANDBY" {
		readynessCmd = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--checkreadyness=true", "--optype=standbyshard"}
	}
	return readynessCmd
}

func getOnlineShardCmd(sparamStr string) []string {
	var onlineCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkonlineshard=" + strconv.Quote(sparamStr), "--optype=gsm"}
	return onlineCmd
}

func getdeployShardCmd() []string {
	var depCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--deployshard=true", "--optype=gsm"}
	return depCmd
}

func getGsmvalidateCmd() []string {
	var depCmd []string = []string{oraScriptMount + "/cmdExec", "/bin/python", oraScriptMount + "/main.py ", "--checkliveness=true", "--optype=gsm"}
	return depCmd
}

func getExportTDEKeyCmd(sparamStr string) []string {
	var exportTDEKeyCmd []string = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--exporttdekey=" + strconv.Quote(sparamStr)}
	return exportTDEKeyCmd
}

func getImportTDEKeyCmd(sparamStr string) []string {
	var importTDEKeyCmd []string = []string{oraDbScriptMount + "/cmdExec", "/bin/python", oraDbScriptMount + "/main.py ", "--importtdekey=" + strconv.Quote(sparamStr)}
	return importTDEKeyCmd
}

func getInitContainerCmd(resType string, name string,
) string {
	var initCmd string
	if resType == "WEB" {
		initCmd = "chown -R 54321:54321 " + oraDbScriptMount + ";chmod 755 " + oraDbScriptMount + "/*;chown -R 54321:54321 /opt/oracle/oradata;chmod 750 /opt/oracle/oradata"
	} else {
		initCmd = resType + ";chown -R 54321:54321 " + oraDbScriptMount + ";chmod 755 " + oraDbScriptMount + "/*;chown -R 54321:54321 /opt/oracle/oradata;chmod 750 /opt/oracle/oradata"
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

func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Function to check shadrd in GSM
func CheckShardInGsm(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {

	_, _, err := ExecCommand(gsmPodName, getShardCheckCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Did not find the shard " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to check the online Shard
func CheckOnlineShardInGsm(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {

	_, _, err := ExecCommand(gsmPodName, getOnlineShardCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Shard: " + GetFmtStr(sparams) + " is not online in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to move the chunks
func MoveChunks(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {

	_, _, err := ExecCommand(gsmPodName, getMoveChunksCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred in during Chunk movement command submission for shard: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to verify the chunks
func CheckChunksRemaining(
	gsmPodName string,
	sparams string,
	instance *databasev4.ShardingDatabase,
	kubeconfig *rest.Config,
	logger logr.Logger,
) (bool, string, error) {
	stdout, stderr, err := ExecCommand(gsmPodName, getNoChunksCmd(sparams), kubeconfig, instance, logger)

	if strings.TrimSpace(stdout) != "" {
		LogMessages("DEBUG", "CheckChunksRemaining stdout: "+strings.TrimSpace(stdout), nil, instance, logger)
	}
	if strings.TrimSpace(stderr) != "" {
		LogMessages("DEBUG", "CheckChunksRemaining stderr: "+strings.TrimSpace(stderr), nil, instance, logger)
	}

	// no chunks remain
	if err == nil {
		return false, "", nil
	}

	errStr := err.Error()

	// existing behavior: command returns exit 127 while chunks still remain
	if strings.Contains(errStr, "exit code 127") {
		summary := strings.TrimSpace(stdout)
		if summary == "" {
			summary = strings.TrimSpace(stderr)
		}
		return true, summary, nil
	}

	return false, "", err
}

// Function to verify the chunks
func AddShardInGsm(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {
	_, _, err := ExecCommand(gsmPodName, getShardAddCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while adding a shard " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to deploy the Shards
func DeployShardInGsm(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {
	_, _, err := ExecCommand(gsmPodName, getdeployShardCmd(), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while deploying the shard in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

// Function to delete the shard
func RemoveShardFromGsm(gsmPodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) error {
	_, _, err := ExecCommand(gsmPodName, getShardDelCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while cancelling the chunks: " + GetFmtStr(sparams) + " in GSM."
		LogMessages("INFO", msg, nil, instance, logger)
		return err
	}
	return nil
}

func GetSvcIp(PodName string, sparams string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) (string, string, error) {
	stdoutput, stderror, err := ExecCommand(PodName, GetIpCmd(sparams), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the IP for k8s service " + GetFmtStr(sparams)
		LogMessages("INFO", msg, nil, instance, logger)
		return strings.Replace(stdoutput, "\r\n", "", -1), strings.Replace(stderror, "/r/n", "", -1), err
	}
	return strings.Replace(stdoutput, "\r\n", "", -1), strings.Replace(stderror, "/r/n", "", -1), nil
}

func GetGsmServices(PodName string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(PodName, getGsmSvcCmd(), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the services from the GSM "
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return stdoutput
}

func GetDbRole(PodName string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(PodName, getDbRoleCmd(), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the DB role from the database"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func GetDbOpenMode(PodName string, instance *databasev4.ShardingDatabase, kubeconfig *rest.Config, logger logr.Logger,
) string {
	stdoutput, _, err := ExecCommand(PodName, getDbModeCmd(), kubeconfig, instance, logger)
	if err != nil {
		msg := "Error occurred while getting the DB mode from the database"
		LogMessages("DEBUG", msg, err, instance, logger)
		return msg
	}
	return strings.TrimSpace(stdoutput)
}

func SfsetLabelPatch(sfSetFound *appsv1.StatefulSet, sfSetPod *corev1.Pod, instance *databasev4.ShardingDatabase, kClient client.Client,
) error {

	//var msg string
	//status = false
	var err error

	sfsetCopy := sfSetFound.DeepCopy()
	if sfsetCopy.Labels == nil {
		sfsetCopy.Labels = map[string]string{}
	}
	sfsetCopy.Labels[string(databasev4.ShardingDelLabelKey)] = string(databasev4.ShardingDelLabelTrueValue)
	patch := client.MergeFrom(sfSetFound)
	err = kClient.Patch(context.Background(), sfsetCopy, patch)
	if err != nil {
		return err
	}

	podCopy := sfSetPod.DeepCopy()
	if podCopy.Labels == nil {
		podCopy.Labels = map[string]string{}
	}
	podCopy.Labels[string(databasev4.ShardingDelLabelKey)] = string(databasev4.ShardingDelLabelTrueValue)
	podPatch := client.MergeFrom(sfSetPod.DeepCopy())
	err = kClient.Patch(context.Background(), podCopy, podPatch)
	if err != nil {
		return err
	}

	return nil
}

func InstanceShardPatch(obj client.Object, instance *databasev4.ShardingDatabase, kClient client.Client, id int32, field string, value string,
) error {
	instSpec := instance.Spec

	if id < 0 || int(id) >= len(instSpec.Shard) {
		return fmt.Errorf("invalid shard index %d", id)
	}

	switch strings.ToLower(strings.TrimSpace(field)) {
	case "isdelete":
		if strings.TrimSpace(value) == "" {
			instSpec.Shard[id].IsDelete = "failed"
		} else {
			instSpec.Shard[id].IsDelete = value
		}
	default:
		return fmt.Errorf("unsupported shard patch field %q", field)
	}

	instshardM, err := json.Marshal(struct {
		Spec *databasev4.ShardingDatabaseSpec `json:"spec":`
	}{
		Spec: &instSpec,
	})
	if err != nil {
		return err
	}

	patch1 := client.RawPatch(types.MergePatchType, instshardM)
	err = kClient.Patch(context.Background(), obj, patch1)

	if err != nil {
		return err
	}

	return nil
}

func checkTdeWalletFlag(instance *databasev4.ShardingDatabase) bool {
	if strings.ToLower(instance.Spec.IsTdeWallet) == "enable" {
		return true
	}
	return false
}

func CheckIsTDEWalletFlag(instance *databasev4.ShardingDatabase, logger logr.Logger) bool {
	LogMessages("INFO", "CheckIsTDEWalletFlag():isTdeWallet=["+instance.Spec.IsTdeWallet+"].", nil, instance, logger)
	if strings.ToLower(instance.Spec.IsTdeWallet) == "enable" {
		LogMessages("INFO", "CheckIsTDEWalletFlag():Returning true", nil, instance, logger)
		return true
	}
	return false
}

func CheckIsDeleteFlag(delStr string, instance *databasev4.ShardingDatabase, logger logr.Logger) bool {
	if strings.ToLower(delStr) == "enable" {
		return true
	}
	if strings.ToLower(delStr) == "failed" {
		// LogMessages("INFO", "manual intervention required", nil, instance, logger)
	}
	return false
}

func getTdeWalletMountLoc(instance *databasev4.ShardingDatabase) string {
	if len(instance.Spec.TdeWalletPvcMountLocation) > 0 {
		return instance.Spec.TdeWalletPvcMountLocation
	}
	return "/tdewallet/" + instance.Name
}

func Int64Pointer(d int64) *int64 {
	return &d
}

func BoolPointer(d bool) *bool {
	return &d
}
