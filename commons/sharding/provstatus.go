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

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// statusMapKey composes the flattened status map key format: "<name>_<field>".
func statusMapKey(name, key string) string {
	return name + "_" + key
}

// ensureMap initializes a status map when it is nil.
func ensureMap(m *map[string]string) {
	if *m == nil {
		*m = make(map[string]string)
	}
}

// upsertStatusKey sets a key/value pair on a flattened status map.
func upsertStatusKey(m *map[string]string, name, key, value string) {
	ensureMap(m)
	(*m)[statusMapKey(name, key)] = value
}

// removeStatusKey removes a key from a flattened status map if present.
func removeStatusKey(m map[string]string, name, key string) {
	if m != nil {
		delete(m, statusMapKey(name, key))
	}
}

// dbServiceNames returns pod and service DNS names for a "<name>-0" workload.
func dbServiceNames(name, namespace string) (podName, internalSvc, externalSvc string) {
	base := name + "-0." + name
	podName = name + "-0"
	internalSvc = base + "." + namespace + ".svc.cluster.local"
	externalSvc = base + "0-svc." + namespace + ".svc.cluster.local"
	return podName, internalSvc, externalSvc
}

// updateDbStatusData updates flattened status entries shared by catalog and shard.
func updateDbStatusData(
	instance *databasev4.ShardingDatabase,
	name string,
	envVars []databasev4.EnvironmentVariable,
	state string,
	openMode string,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	logger logr.Logger,
	upsert func(string, string, string),
	remove func(string, string),
) {
	if state == string(databasev4.AvailableState) {
		ns := getInstanceNs(instance)
		podName, internalSvc, externalSvc := dbServiceNames(name, ns)
		_, internalIP, _ := GetSvcIp(podName, internalSvc, instance, kubeClient, kubeConfig, logger)
		_, externalIP, _ := GetSvcIp(podName, externalSvc, instance, kubeClient, kubeConfig, logger)
		role := GetDbRole(podName, instance, kubeClient, kubeConfig, logger)
		oracleSid := GetSidName(envVars, name)
		oraclePdb := GetPdbName(envVars, name)

		upsert(name, string(databasev4.Name), name)
		upsert(name, string(databasev4.DbPasswordSecret), instance.Spec.DbSecret.Name)
		if instance.Spec.IsExternalSvc {
			upsert(name, string(databasev4.K8sExternalSvc), externalSvc)
			upsert(name, string(databasev4.K8sExternalSvcIP), externalIP)
		}
		upsert(name, string(databasev4.K8sInternalSvc), internalSvc)
		upsert(name, string(databasev4.K8sInternalSvcIP), internalIP)
		upsert(name, string(databasev4.State), state)
		upsert(name, string(databasev4.OracleSid), oracleSid)
		upsert(name, string(databasev4.OraclePdb), oraclePdb)
		upsert(name, string(databasev4.Role), role)
		upsert(name, string(databasev4.OpenMode), openMode)
		return
	}

	if state == string(databasev4.Terminated) {
		remove(name, string(databasev4.State))
		remove(name, string(databasev4.Name))
		remove(name, string(databasev4.K8sInternalSvc))
		remove(name, string(databasev4.K8sExternalSvc))
		remove(name, string(databasev4.K8sExternalSvcIP))
		remove(name, string(databasev4.K8sInternalSvcIP))
		remove(name, string(databasev4.Role))
		remove(name, string(databasev4.OraclePdb))
		remove(name, string(databasev4.OracleSid))
		remove(name, string(databasev4.OpenMode))
		return
	}

	upsert(name, string(databasev4.State), state)
	upsert(name, string(databasev4.Name), name)
	upsert(name, string(databasev4.OpenMode), openMode)
	remove(name, string(databasev4.K8sInternalSvc))
	remove(name, string(databasev4.K8sExternalSvc))
	remove(name, string(databasev4.K8sExternalSvcIP))
	remove(name, string(databasev4.K8sInternalSvcIP))
	remove(name, string(databasev4.Role))
	remove(name, string(databasev4.OraclePdb))
	remove(name, string(databasev4.OracleSid))
}

// UpdateGsmStatusData refreshes GSM status details for the given spec index/state.
func UpdateGsmStatusData(instance *databasev4.ShardingDatabase, specIdx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	name := instance.Spec.Gsm[specIdx].Name
	if state == string(databasev4.AvailableState) {
		ns := getInstanceNs(instance)
		podName, internalSvc, externalSvc := dbServiceNames(name, ns)
		_, internalIP, _ := GetSvcIp(podName, internalSvc, instance, kubeClient, kubeConfig, logger)
		_, externalIP, _ := GetSvcIp(podName, externalSvc, instance, kubeClient, kubeConfig, logger)
		instance.Status.Gsm.Services = GetGsmServices(podName, instance, kubeClient, kubeConfig, logger)

		insertOrUpdateGsmKeys(instance, name, string(databasev4.Name), name)
		insertOrUpdateGsmKeys(instance, name, string(databasev4.DbPasswordSecret), instance.Spec.DbSecret.Name)
		if instance.Spec.IsExternalSvc {
			insertOrUpdateGsmKeys(instance, name, string(databasev4.K8sExternalSvc), externalSvc)
			insertOrUpdateGsmKeys(instance, name, string(databasev4.K8sExternalSvcIP), externalIP)
		}
		insertOrUpdateGsmKeys(instance, name, string(databasev4.K8sInternalSvc), internalSvc)
		insertOrUpdateGsmKeys(instance, name, string(databasev4.K8sInternalSvcIP), internalIP)
		insertOrUpdateGsmKeys(instance, name, string(databasev4.State), state)
		return
	}

	if state == string(databasev4.Terminated) {
		removeGsmKeys(instance, name, string(databasev4.Name))
		removeGsmKeys(instance, name, string(databasev4.K8sInternalSvc))
		removeGsmKeys(instance, name, string(databasev4.K8sExternalSvc))
		removeGsmKeys(instance, name, string(databasev4.K8sExternalSvcIP))
		removeGsmKeys(instance, name, string(databasev4.K8sInternalSvcIP))
		removeGsmKeys(instance, name, string(databasev4.Role))
		instance.Status.Gsm.Services = ""
		return
	}

	insertOrUpdateGsmKeys(instance, name, string(databasev4.Name), name)
	removeGsmKeys(instance, name, string(databasev4.K8sInternalSvc))
	removeGsmKeys(instance, name, string(databasev4.K8sExternalSvc))
	removeGsmKeys(instance, name, string(databasev4.K8sExternalSvcIP))
	removeGsmKeys(instance, name, string(databasev4.K8sInternalSvcIP))
	removeGsmKeys(instance, name, string(databasev4.Role))
	instance.Status.Gsm.Services = ""
}

// UpdateCatalogStatusData refreshes catalog status details for the given spec index/state.
func UpdateCatalogStatusData(instance *databasev4.ShardingDatabase, specIdx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	name := instance.Spec.Catalog[specIdx].Name
	mode := GetDbOpenMode(name+"-0", instance, kubeClient, kubeConfig, logger)
	updateDbStatusData(
		instance,
		name,
		instance.Spec.Catalog[specIdx].EnvVars,
		state,
		mode,
		kubeClient,
		kubeConfig,
		logger,
		func(n, k, v string) { insertOrUpdateCatalogKeys(instance, n, k, v) },
		func(n, k string) { removeCatalogKeys(instance, n, k) },
	)
}

// UpdateShardStatusData refreshes shard status details for the given spec index/state.
func UpdateShardStatusData(instance *databasev4.ShardingDatabase, specIdx int, state string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, logger logr.Logger,
) {
	name := instance.Spec.Shard[specIdx].Name
	mode := GetDbOpenMode(name+"-0", instance, kubeClient, kubeConfig, logger)
	updateDbStatusData(
		instance,
		name,
		instance.Spec.Shard[specIdx].EnvVars,
		state,
		mode,
		kubeClient,
		kubeConfig,
		logger,
		func(n, k, v string) { insertOrUpdateShardKeys(instance, n, k, v) },
		func(n, k string) { removeShardKeys(instance, n, k) },
	)
}

// insertOrUpdateShardKeys updates one flattened shard status entry.
func insertOrUpdateShardKeys(instance *databasev4.ShardingDatabase, name string, key string, value string) {
	upsertStatusKey(&instance.Status.Shard, name, key, value)
}

// removeShardKeys removes one flattened shard status entry.
func removeShardKeys(instance *databasev4.ShardingDatabase, name string, key string) {
	removeStatusKey(instance.Status.Shard, name, key)
}

// insertOrUpdateCatalogKeys updates one flattened catalog status entry.
func insertOrUpdateCatalogKeys(instance *databasev4.ShardingDatabase, name string, key string, value string) {
	upsertStatusKey(&instance.Status.Catalog, name, key, value)
}

// removeCatalogKeys removes one flattened catalog status entry.
func removeCatalogKeys(instance *databasev4.ShardingDatabase, name string, key string) {
	removeStatusKey(instance.Status.Catalog, name, key)
}

// insertOrUpdateGsmKeys updates one flattened GSM details entry.
func insertOrUpdateGsmKeys(instance *databasev4.ShardingDatabase, name string, key string, value string) {
	upsertStatusKey(&instance.Status.Gsm.Details, name, key, value)
}

// removeGsmKeys removes one flattened GSM details entry.
func removeGsmKeys(instance *databasev4.ShardingDatabase, name string, key string) {
	removeStatusKey(instance.Status.Gsm.Details, name, key)
}

// getInstanceNs returns the instance namespace or "default" when empty.
func getInstanceNs(instance *databasev4.ShardingDatabase) string {
	var namespace string
	if instance.Namespace == "" {
		namespace = "default"
	} else {
		namespace = instance.Namespace
	}
	return namespace
}

// CheckGsmStatus validates GSM director readiness in the given pod.
func CheckGsmStatus(gname string, instance *databasev4.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
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

// ValidateDbSetup validates DB setup scripts from the target pod.
func ValidateDbSetup(podName string, instance *databasev4.ShardingDatabase, kubeClient kubernetes.Interface, kubeconfig clientcmd.ClientConfig, logger logr.Logger,
) error {

	_, _, err := ExecCommand(podName, shardValidationCmd(), kubeClient, kubeconfig, instance, logger)
	if err != nil {
		return fmt.Errorf("error occurred while validating the DB setup")
	}
	return nil
}

// UpdateGsmShardStatus updates per-shard GSM membership status.
func UpdateGsmShardStatus(instance *databasev4.ShardingDatabase, name string, state string) {
	if instance.Status.Gsm.Shards == nil {
		instance.Status.Gsm.Shards = make(map[string]string)
	}
	instance.Status.Gsm.Shards[name] = state
	if state == string(databasev4.Terminated) {
		delete(instance.Status.Gsm.Shards, name)
	}
}

// GetGsmShardStatus returns the shard state from GSM shard status map.
func GetGsmShardStatus(instance *databasev4.ShardingDatabase, name string) string {
	if _, ok := instance.Status.Gsm.Shards[name]; ok {
		return instance.Status.Gsm.Shards[name]

	}
	return "NOSTATE"

}

// GetGsmShardStatusKey returns a value from flattened shard status map.
func GetGsmShardStatusKey(instance *databasev4.ShardingDatabase, key string) string {
	if _, ok := instance.Status.Shard[key]; ok {
		return instance.Status.Shard[key]

	}
	return "NOSTATE"

}

// GetGsmCatalogStatusKey returns a value from flattened catalog status map.
func GetGsmCatalogStatusKey(instance *databasev4.ShardingDatabase, key string) string {
	if _, ok := instance.Status.Catalog[key]; ok {
		return instance.Status.Catalog[key]

	}
	return "NOSTATE"

}
