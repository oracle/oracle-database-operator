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

package e2eutil

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"time"

	goyaml "gopkg.in/yaml.v2"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// GenerateDBName returns a string DB concatenate 14 digits of the date time
// E.g., DB060102150405 if the curret date-time is 2006.01.02 15:04:05
func GenerateDBName() string {
	timeString := time.Now().Format("2006.01.02 15:04:05")
	trimmed := strings.ReplaceAll(timeString, ":", "") // remove colons
	trimmed = strings.ReplaceAll(trimmed, ".", "")     // remove dots
	trimmed = strings.ReplaceAll(trimmed, " ", "")     // remove spaces
	trimmed = trimmed[2:]                              // remove the first two digits of year (2006 -> 06)
	return "DB" + trimmed
}

func unmarshalFromYamlBytes(bytes []byte, obj interface{}) error {
	jsonBytes, err := yaml.YAMLToJSON(bytes)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonBytes, obj)
}

// LoadTestFixture create an AutonomousDatabase resource from a test fixture
func LoadTestFixture(adb *dbv1alpha1.AutonomousDatabase, filename string) (*dbv1alpha1.AutonomousDatabase, error) {
	filePath := "./resource/" + filename
	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err := unmarshalFromYamlBytes(yamlBytes, adb); err != nil {
		return nil, err
	}

	return adb, nil
}

func createKubeConfigMap(configMapNamespace string, configMapName string, data map[string]string) (*corev1.ConfigMap, error) {
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: configMapNamespace,
			Name:      configMapName,
		},
		Data: data,
	}

	return configmap, nil
}

func CreateKubeSecret(secretNamespace string, secretName string, data map[string]string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Type:       "Opaque",
		StringData: data,
	}

	return secret, nil
}

type testConfiguration struct {
	OCIConfigFile              string `yaml:"ociConfigFile"`
	Profile                    string `yaml:"profile"`
	CompartmentOCID            string `yaml:"compartmentOCID"`
	AdminPasswordOCID          string `yaml:"adminPasswordOCID"`
	InstanceWalletPasswordOCID string `yaml:"instanceWalletPasswordOCID"`
	SubnetOCID                 string `yaml:"subnetOCID"`
	NsgOCID                    string `yaml:"nsgOCID"`
	BucketURL                  string `yaml:"bucketURL"`
	AuthToken                  string `yaml:"authToken"`
	OciUser                    string `yaml:"ociUser"`
}

func GetTestConfig(filename string) (*testConfiguration, error) {
	config := &testConfiguration{}

	filePath := "./resource/" + filename
	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err := goyaml.Unmarshal(yamlBytes, config); err != nil {
		return nil, err
	}

	return config, nil
}
