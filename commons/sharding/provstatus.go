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
	"fmt"
	"strconv"

	databasealphav1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
)

// CHeck if record exist in a struct
func CheckGsmStatusInst(instSpex []databasealphav1.GsmStatusDetails, name string,
) (int, bool) {

	var status bool = false
	var idx int

	for i := 0; i < len(instSpex); i++ {
		if instSpex[i].Name == name {
			status = true
			idx = i
			break
		}
	}

	return idx, status
}

func UpdateGsmStatusData(instance *databasealphav1.ShardingDatabase, Specidx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	if state == string(databasealphav1.AvailableState) {
		// Evaluate following values only if state is set to available
		svcName := instance.Spec.Gsm[Specidx].Name + "-0." + instance.Spec.Gsm[Specidx].Name
		k8sExternalSvcName := svcName + strconv.FormatInt(int64(0), 10) + "-svc." + getInstanceNs(instance) + ".svc.cluster.local"
		K8sInternalSvcName := svcName + "." + getInstanceNs(instance) + ".svc.cluster.local"
		_, K8sInternalSvcIP, _ := GetSvcIp(instance.Spec.Gsm[Specidx].Name+"-0", K8sInternalSvcName, instance, kubeClient, kubeConfig, logger)
		_, K8sExternalSvcIP, _ := GetSvcIp(instance.Spec.Gsm[Specidx].Name+"-0", k8sExternalSvcName, instance, kubeClient, kubeConfig, logger)
		DbPasswordSecret := instance.Spec.DbSecret.Name
		instance.Status.Gsm.Services = GetGsmServices(instance.Spec.Gsm[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)

		//	externIp := strings.Replace(K8sInternalSvcIP, "/r/n", "", -1)
		//	internIp := strings.Replace(K8sExternalSvcIP, "/r/n", "", -1)

		// Populate the Maps
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.Name), instance.Spec.Gsm[Specidx].Name)
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.DbPasswordSecret), DbPasswordSecret)
		if instance.Spec.IsExternalSvc == true {
			insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvc), k8sExternalSvcName)
			insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvcIP), K8sExternalSvcIP)
		}
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvc), K8sInternalSvcName)
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvcIP), K8sInternalSvcIP)
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.State), state)
	} else if state == string(databasealphav1.Terminated) {
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.Name))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.Role))
		instance.Status.Gsm.Services = ""

	} else {
		insertOrUpdateGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.Name), instance.Spec.Gsm[Specidx].Name)
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeGsmKeys(instance, instance.Spec.Gsm[Specidx].Name, string(databasealphav1.Role))
		instance.Status.Gsm.Services = ""
	}

}

func UpdateCatalogStatusData(instance *databasealphav1.ShardingDatabase, Specidx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	mode := GetDbOpenMode(instance.Spec.Catalog[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)
	if state == string(databasealphav1.AvailableState) {
		// Evaluate following values only if state is set to available
		svcName := instance.Spec.Catalog[Specidx].Name + "-0." + instance.Spec.Catalog[Specidx].Name
		k8sExternalSvcName := svcName + strconv.FormatInt(int64(0), 10) + "-svc." + getInstanceNs(instance) + ".svc.cluster.local"
		K8sInternalSvcName := svcName + "." + getInstanceNs(instance) + ".svc.cluster.local"
		_, K8sInternalSvcIP, _ := GetSvcIp(instance.Spec.Catalog[Specidx].Name+"-0", K8sInternalSvcName, instance, kubeClient, kubeConfig, logger)
		_, K8sExternalSvcIP, _ := GetSvcIp(instance.Spec.Catalog[Specidx].Name+"-0", k8sExternalSvcName, instance, kubeClient, kubeConfig, logger)
		DbPasswordSecret := instance.Spec.DbSecret.Name
		oracleSid := GetSidName(instance.Spec.Catalog[Specidx].EnvVars, instance.Spec.Catalog[Specidx].Name)
		oraclePdb := GetPdbName(instance.Spec.Catalog[Specidx].EnvVars, instance.Spec.Catalog[Specidx].Name)
		role := GetDbRole(instance.Spec.Catalog[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)
		//	externIp := strings.Replace(K8sInternalSvcIP, "/r/n", "", -1)
		//	internIp := strings.Replace(K8sExternalSvcIP, "/r/n", "", -1)

		// Populate the Maps
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Name), instance.Spec.Catalog[Specidx].Name)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.DbPasswordSecret), DbPasswordSecret)
		if instance.Spec.IsExternalSvc == true {
			insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvc), k8sExternalSvcName)
			insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvcIP), K8sExternalSvcIP)
		}
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvc), K8sInternalSvcName)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvcIP), K8sInternalSvcIP)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.State), state)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OracleSid), oracleSid)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OraclePdb), oraclePdb)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Role), role)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OpenMode), mode)
	} else if state == string(databasealphav1.Terminated) {
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.State))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Name))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Role))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OraclePdb))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OracleSid))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Role))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OpenMode))

	} else {
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.State), state)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Name), instance.Spec.Catalog[Specidx].Name)
		insertOrUpdateCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OpenMode), mode)
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Role))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OraclePdb))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.OracleSid))
		removeCatalogKeys(instance, instance.Spec.Catalog[Specidx].Name, string(databasealphav1.Role))
	}

}

func UpdateShardStatusData(instance *databasealphav1.ShardingDatabase, Specidx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	mode := GetDbOpenMode(instance.Spec.Shard[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)
	if state == string(databasealphav1.AvailableState) {
		// Evaluate following values only if state is set to available
		svcName := instance.Spec.Shard[Specidx].Name + "-0." + instance.Spec.Shard[Specidx].Name
		k8sExternalSvcName := svcName + strconv.FormatInt(int64(0), 10) + "-svc." + getInstanceNs(instance) + ".svc.cluster.local"
		K8sInternalSvcName := svcName + "." + getInstanceNs(instance) + ".svc.cluster.local"
		_, K8sInternalSvcIP, _ := GetSvcIp(instance.Spec.Shard[Specidx].Name+"-0", K8sInternalSvcName, instance, kubeClient, kubeConfig, logger)
		_, K8sExternalSvcIP, _ := GetSvcIp(instance.Spec.Shard[Specidx].Name+"-0", k8sExternalSvcName, instance, kubeClient, kubeConfig, logger)
		DbPasswordSecret := instance.Spec.DbSecret.Name
		oracleSid := GetSidName(instance.Spec.Shard[Specidx].EnvVars, instance.Spec.Shard[Specidx].Name)
		oraclePdb := GetPdbName(instance.Spec.Shard[Specidx].EnvVars, instance.Spec.Shard[Specidx].Name)
		role := GetDbRole(instance.Spec.Shard[Specidx].Name+"-0", instance, kubeClient, kubeConfig, logger)

		// Populate the Maps
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Name), instance.Spec.Shard[Specidx].Name)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.DbPasswordSecret), DbPasswordSecret)
		if instance.Spec.IsExternalSvc == true {
			insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvc), k8sExternalSvcName)
			insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvcIP), K8sExternalSvcIP)
		}
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvc), K8sInternalSvcName)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvcIP), K8sInternalSvcIP)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.State), state)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OracleSid), oracleSid)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OraclePdb), oraclePdb)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Role), role)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OpenMode), mode)
	} else if state == string(databasealphav1.Terminated) {
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.State))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Name))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Role))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OraclePdb))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OracleSid))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Role))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OpenMode))

	} else {
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.State), state)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Name), instance.Spec.Shard[Specidx].Name)
		insertOrUpdateShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OpenMode), mode)
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvc))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvc))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sExternalSvcIP))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.K8sInternalSvcIP))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Role))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OraclePdb))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.OracleSid))
		removeShardKeys(instance, instance.Spec.Shard[Specidx].Name, string(databasealphav1.Role))
	}

}

func insertOrUpdateShardKeys(instance *databasealphav1.ShardingDatabase, name string, key string, value string) {
	newKey := name + "_" + key
	if len(instance.Status.Shard) > 0 {
		if _, ok := instance.Status.Shard[newKey]; ok {
			instance.Status.Shard[newKey] = value
		} else {
			instance.Status.Shard[newKey] = value
		}
	} else {
		instance.Status.Shard = make(map[string]string)
		instance.Status.Shard[newKey] = value
	}

}

func removeShardKeys(instance *databasealphav1.ShardingDatabase, name string, key string) {
	newKey := name + "_" + key
	if len(instance.Status.Shard) > 0 {
		if _, ok := instance.Status.Shard[newKey]; ok {
			delete(instance.Status.Shard, newKey)
		}

	}
}

func insertOrUpdateCatalogKeys(instance *databasealphav1.ShardingDatabase, name string, key string, value string) {
	newKey := name + "_" + key
	if len(instance.Status.Catalog) > 0 {
		if _, ok := instance.Status.Catalog[newKey]; ok {
			instance.Status.Catalog[newKey] = value
		} else {
			instance.Status.Catalog[newKey] = value
		}
	} else {
		instance.Status.Catalog = make(map[string]string)
		instance.Status.Catalog[newKey] = value
	}

}

func removeCatalogKeys(instance *databasealphav1.ShardingDatabase, name string, key string) {
	newKey := name + "_" + key
	if len(instance.Status.Catalog) > 0 {
		if _, ok := instance.Status.Catalog[newKey]; ok {
			delete(instance.Status.Catalog, newKey)
		}

	}
}

func insertOrUpdateGsmKeys(instance *databasealphav1.ShardingDatabase, name string, key string, value string) {
	newKey := name + "_" + key
	if len(instance.Status.Gsm.Details) > 0 {
		if _, ok := instance.Status.Gsm.Details[newKey]; ok {
			instance.Status.Gsm.Details[newKey] = value
		} else {
			instance.Status.Gsm.Details[newKey] = value
		}
	} else {
		instance.Status.Gsm.Details = make(map[string]string)
		instance.Status.Gsm.Details[newKey] = value
	}

}

func removeGsmKeys(instance *databasealphav1.ShardingDatabase, name string, key string) {
	newKey := name + "_" + key
	if len(instance.Status.Gsm.Details) > 0 {
		if _, ok := instance.Status.Gsm.Details[newKey]; ok {
			delete(instance.Status.Gsm.Details, newKey)
		}

	}
}

func getInstanceNs(instance *databasealphav1.ShardingDatabase) string {
	var namespace string
	if instance.Spec.Namespace == "" {
		namespace = "default"
	} else {
		namespace = instance.Spec.Namespace
	}
	return namespace
}

// File the meta condition and return the meta view
func GetMetaCondition(instance *databasealphav1.ShardingDatabase, result *ctrl.Result, err *error, stateType string, stateMsg string) metav1.Condition {

	return metav1.Condition{
		Type:               stateType,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: instance.GetGeneration(),
		Reason:             stateMsg,
		Message:            fmt.Sprint(*err),
		Status:             metav1.ConditionTrue,
	}
}

// ======================= CHeck GSM Director Status ==============
func CheckGsmStatus(gname string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {
	var err error
	var msg string = "Inside the checkGsmStatus. Checking GSM director in " + GetFmtStr(gname) + " pod."

	LogMessages("DEBUG", msg, nil, instance, logger)

	_, _, err = ExecCommand(gname, getGsmvalidateCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return err
	}

	return nil
}

// ============ Functiont o check the status of the Shard and catalog =========
// ================================ Validate shard ===========================
func ValidateDbSetup(podName string, instance *databasealphav1.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, shardValidationCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error ocurred while validating the DB Setup")
	}
	return nil
}

func UpdateGsmShardStatus(instance *databasealphav1.ShardingDatabase, name string, state string) {
	//smap := make(map[string]string)
	if _, ok := instance.Status.Gsm.Shards[name]; ok {
		instance.Status.Gsm.Shards[name] = state

	} else {
		if len(instance.Status.Gsm.Shards) > 0 {
			instance.Status.Gsm.Shards[name] = state
		} else {
			instance.Status.Gsm.Shards = make(map[string]string)
			instance.Status.Gsm.Shards[name] = state

		}
	}

	if state == "TERMINATED" {

		if _, ok := instance.Status.Gsm.Shards[name]; ok {
			delete(instance.Status.Gsm.Shards, name)
		}
	}

}

func GetGsmShardStatus(instance *databasealphav1.ShardingDatabase, name string) string {
	if _, ok := instance.Status.Gsm.Shards[name]; ok {
		return instance.Status.Gsm.Shards[name]

	}
	return "NOSTATE"

}

func GetGsmShardStatusKey(instance *databasealphav1.ShardingDatabase, key string) string {
	if _, ok := instance.Status.Shard[key]; ok {
		return instance.Status.Shard[key]

	}
	return "NOSTATE"

}

func GetGsmCatalogStatusKey(instance *databasealphav1.ShardingDatabase, key string) string {
	if _, ok := instance.Status.Catalog[key]; ok {
		return instance.Status.Catalog[key]

	}
	return "NOSTATE"

}

func GetGsmDetailsSttausKey(instance *databasealphav1.ShardingDatabase, key string) string {
	if _, ok := instance.Status.Gsm.Details[key]; ok {
		return instance.Status.Gsm.Details[key]

	}
	return "NOSTATE"

}
