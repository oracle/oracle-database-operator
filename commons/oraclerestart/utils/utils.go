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
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Constants for RAC StatefulSet & Volumes
const (
	OraImagePullPolicy          = corev1.PullAlways
	OrainitCmd1                 = "set -ex;" + "touch /tmp/test_cmd1.txt"
	OrainitCmd5                 = "set -ex;" + "[[ `hostname` =~ -([0-9]+)$ ]] || exit 1 ;" + "ordinal=${BASH_REMATCH[1]};" + "cp /mnt/config-map/envfile  /mnt/conf.d/; cat /mnt/conf.d/envfile | awk -v env_var=$ordinal -F '=' '{print \"export \" $1\"=\"$2 env_var }' > /tmp/test.env; mv /tmp/test.env /mnt/conf.d/envfile"
	OraConfigMapMount           = "/mnt/config-map"
	OraEnvFileMount             = "/mnt/conf.d"
	OraSubDomain                = "racnode"
	OraEnvFile                  = "/etc/rac_env_vars"
	OraRacSshSecretMount        = "/mnt/.ssh"
	OraGiRsp                    = "/mnt/gridrsp"
	OraDbRsp                    = "/mnt/dbrsp"
	OraEnvVars                  = "/etc/rac_env_vars"
	OraNodeKey                  = "kubernetes.io/hostname"
	OraOperatorKey              = "In"
	OraShm                      = "/dev/shm"
	OraRacDbPwdFileSecretMount  = "/mnt/.dbsecrets"
	OraRacDbKeyFileSecretMount  = "/mnt/.dbsecrets"
	OraRacTdePwdFileSecretMount = "/mnt/.tdesecrets"
	OraRacTdeKeyFileSecretMount = "/mnt/.tdesecrets"
	OraStage                    = "/mnt/stage"
	OraBootVol                  = "/boot"
	OraSwLocation               = "/u01"
	OraScriptMount              = "/opt/scripts/startup/scripts"
	OraSSHPrivKey               = "/mnt/.ssh/ssh-privkey"
	OraSSHPubKey                = "/mnt/.ssh/ssh-pubkey"
	OraSwStageLocation          = "/mnt/stage/software"
	OraRuPatchStageLocation     = "/mnt/stage/rupatch"
	OraOPatchStageLocation      = "/mnt/stage/opatch"
	OraDBPort                   = 1521
	OraLsnrPort                 = 1522
	OraLocalOnsPort             = 6200
	OraSSHPort                  = 22
	OraOemPort                  = 8080
	OraDBUser                   = "oracle"
	OraGridUser                 = "grid"
)

// Fixed Array Values

var serviceCardinality = [...]string{"UNIFORM", "SINGLETON", "DUPLEX"}
var tafPolicy = [...]string{"NONE", "BASIC", "PRECONNECT"}
var serviceRole = [...]string{"PRIMARY", "PHYSICAL_STANDBY", "LOGICAL_STANDBY", "SNAPSHOT_STANDBY"}
var servicePolicy = [...]string{"AUTOMATIC", "MANUAL"}
var serviceResetState = [...]string{"NONE", "LEVEL1"}
var ServiceFailoverType = [...]string{"NONE", "SESSION", "SELECT", "TRANSACTION", "AUTO"}

/// ====== Getter Function Begins here ======= ///

func GetServiceCardinality() []string {
	return serviceCardinality[:]
}

func GetTafPolicy() []string {
	return tafPolicy[:]
}

func ServiceRole() []string {
	return serviceRole[:]
}

func GetServiceRole() []string {
	return serviceRole[:]
}

func GetServiceResetState() []string {
	return serviceResetState[:]
}

func GetServiceFailoverType() []string {
	return ServiceFailoverType[:]
}

/// ====== Getter Function Ends here ======= ///

func CheckStringInList(str1 string, arr []string) bool {

	// iterate using the for loop
	for i := 0; i < len(arr); i++ {
		// check
		if strings.ToLower(arr[i]) == strings.ToLower(str1) {
			// return true
			return true
		}
	}
	return false
}

func CheckStatusFlag(flagStr string) bool {

	if strings.ToLower(flagStr) == "delete" {
		return true
	}

	isTrueFlag, err := strconv.ParseBool(flagStr)
	if err != nil {
		return false
	}
	return isTrueFlag
}

func GetWatchNamespaces() map[string]bool {
	// Fetching the allowed namespaces from env variables
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"
	ns, _ := os.LookupEnv(watchNamespaceEnvVar)
	values := strings.Split(strings.TrimSpace(ns), ",")
	namespaces := make(map[string]bool)
	// put slice values into map
	for _, s := range values {
		namespaces[s] = true
	}
	return namespaces
}

func GetValue(variable string, subkey string) string {

	str2 := ""

	str1 := strings.Split(variable, ",")
	for _, item := range str1 {
		str2 := strings.Split(item, "=")
		if strings.ToLower(str2[0]) == strings.ToLower(subkey) {
			return str2[1]
		}
	}
	return str2
}

func GetDBUser() string {
	return OraDBUser
}

// Contains checks if a string is present in a slice.
func Contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}
