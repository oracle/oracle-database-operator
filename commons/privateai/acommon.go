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
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	piDataMount     = "/stage"
	defaultLogMount = "/privateai/logs"
)

// LogMessages writes the provided message to the logger honouring the
// requested severity. Debug messages are emitted only when the instance has
// debugging enabled.
func LogMessages(msgtype string, msg string, err error, instance *privateaiv4.PrivateAi, logger logr.Logger) {
	severity := strings.ToUpper(strings.TrimSpace(msgtype))
	switch severity {
	case "DEBUG":
		if instance != nil && !instance.Spec.IsDebug {
			return
		}
		if err != nil {
			logger.Error(err, msg)
			return
		}
		logger.Info(msg)
	case "ERROR":
		if err != nil {
			logger.Error(err, msg)
			return
		}
		logger.Info(msg)
	default:
		logger.Info(msg)
	}
}

func getOwnerRef(instance *privateaiv4.PrivateAi) []metav1.OwnerReference {
	owner := metav1.OwnerReference{
		Kind:       instance.GroupVersionKind().Kind,
		APIVersion: instance.APIVersion,
		Name:       instance.Name,
		UID:        types.UID(instance.UID),
	}
	return []metav1.OwnerReference{owner}
}

// buildSvcPortsDef builds the service port definition from the instance spec.
func buildSvcPortsDef(instance *privateaiv4.PrivateAi) []corev1.ServicePort {
	var result []corev1.ServicePort
	if len(instance.Spec.PaiService.PortMappings) > 0 {
		for _, portMapping := range instance.Spec.PaiService.PortMappings {
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
	}
	return result
}

// Function to generate the port mapping
func generatePortMapping(portMapping privateaiv4.PaiPortMapping) string {
	return generateName(fmt.Sprintf("%s-%d-%d-", "tcp",
		portMapping.Port, portMapping.TargetPort))
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

// GetFmtStr returns the provided string surrounded by square brackets.
func GetFmtStr(pstr string) string {
	return "[" + pstr + "]"
}

// CheckDepSet retrieves the deployment belonging to the provided instance.
func CheckDepSet(instance *privateaiv4.PrivateAi, kClient client.Client) (*appsv1.Deployment, error) {
	sfSetFound := &appsv1.Deployment{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}, sfSetFound)
	if err != nil {
		return sfSetFound, err
	}
	return sfSetFound, nil
}

// DelPvc removes the requested PersistentVolumeClaim from the instance
// namespace.
func DelPvc(pvcName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) error {

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

// CheckSvc loads the Kubernetes Service associated with the instance.
func CheckSvc(svcName string, instance *privateaiv4.PrivateAi, kClient client.Client) (*corev1.Service, error) {
	// If this is a PrivateAi instance
	if instance.Kind == "PrivateAi" && !strings.HasSuffix(svcName, "-svc") {
		svcName = instance.Name + "-svc"
	}

	svcFound := &corev1.Service{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      svcName,
		Namespace: instance.Namespace,
	}, svcFound)
	if err != nil {
		return svcFound, err
	}
	return svcFound, nil
}

func checkPvc(pvcName string, instance *privateaiv4.PrivateAi, kClient client.Client) (*corev1.PersistentVolumeClaim, error) {
	pvcFound := &corev1.PersistentVolumeClaim{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      pvcName,
		Namespace: instance.Namespace,
	}, pvcFound)
	if err != nil {
		return pvcFound, err
	}
	return pvcFound, nil
}

// CheckSecret retrieves the named secret from the instance namespace.
func CheckSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      secName,
		Namespace: instance.Namespace,
	}, secret)

	return secret, err
}

// CheckConfigMap retrieves the named config map from the instance namespace.
func CheckConfigMap(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      cName,
		Namespace: instance.Namespace,
	}, configMap)

	return configMap, err
}

// ReadSecret extracts the api-key and cert.pem values from the provided secret.
func ReadSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) (string, string) {
	var apiKeyVal string
	var certPemVal string

	secret, err := CheckSecret(secName, instance, kClient, logger)
	if err != nil {
		return "NONE", "NONE"
	}

	for k, val := range secret.Data {
		if k == "api-key" {
			apiKeyVal = string(val)
			LogMessages("DEBUG", "Key : "+GetFmtStr(k)+" Value : "+GetFmtStr(apiKeyVal)+"   Val: "+GetFmtStr(string(val)), nil, instance, logger)
		}
		if k == "cert.pem" {
			certPemVal = string(val)
			LogMessages("DEBUG", "Key : "+GetFmtStr(k)+" Value : "+GetFmtStr(certPemVal)+"   Val: "+GetFmtStr(string(val)), nil, instance, logger)
		}
	}
	if apiKeyVal == "" {
		apiKeyVal = "NONE"
	}
	if certPemVal == "" {
		certPemVal = "NONE"
	}

	return apiKeyVal, certPemVal
}

// PatchSecret ensures that the private AI identifying label exists on the
// secret without overwriting other labels.
func PatchSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) error {
	secret, err := CheckSecret(secName, instance, kClient, logger)
	if err != nil {
		return err
	}

	labels := secret.GetLabels()
	if labels != nil {
		if _, ok := labels["app.kubernetes.io/privateai-resource-name"]; ok {
			return nil
		}
	}

	secretCopy := secret.DeepCopy()
	updatedLabels := make(map[string]string, len(secretCopy.GetLabels())+1)
	for k, v := range secretCopy.GetLabels() {
		updatedLabels[k] = v
	}
	updatedLabels["app.kubernetes.io/privateai-resource-name"] = "PrivateAi-" + instance.Name
	secretCopy.SetLabels(updatedLabels)
	patch := client.MergeFrom(secret)
	return kClient.Patch(context.Background(), secretCopy, patch)
}

// PatchConfigMap ensures that the private AI identifying label exists on the
// config map without removing other labels.
func PatchConfigMap(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) error {
	configMap, err := CheckConfigMap(cName, instance, kClient, logger)
	if err != nil {
		return err
	}

	labels := configMap.GetLabels()
	if labels != nil {
		if _, ok := labels["app.kubernetes.io/privateai-resource-name"]; ok {
			return nil
		}
	}

	configMapCopy := configMap.DeepCopy()
	updatedLabels := make(map[string]string, len(configMapCopy.GetLabels())+1)
	for k, v := range configMapCopy.GetLabels() {
		updatedLabels[k] = v
	}
	updatedLabels["app.kubernetes.io/privateai-resource-name"] = "PrivateAi-" + instance.Name
	configMapCopy.SetLabels(updatedLabels)
	patch := client.MergeFrom(configMap)
	return kClient.Patch(context.Background(), configMapCopy, patch)
}

// GetSecretResourceVersion returns the Kubernetes resource version for the
// secret when available.
func GetSecretResourceVersion(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) string {
	secret, err := CheckSecret(secName, instance, kClient, logger)
	if err != nil {
		return "None"
	}
	return secret.ResourceVersion
}

// GetConfigMapResourceVersion returns the Kubernetes resource version for the
// config map when available.
func GetConfigMapResourceVersion(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger) string {
	configMap, err := CheckConfigMap(cName, instance, kClient, logger)
	if err != nil {
		return "None"
	}
	return configMap.ResourceVersion
}

// TernaryCondition returns trueVal when condition is true and falseVal
// otherwise.
func TernaryCondition[Y any](condition bool, trueVal, falseVal Y) Y {
	if condition {
		return trueVal
	}
	return falseVal
}

// GetSvcName resolves the service name based on its type.
func GetSvcName(name string, svctype string) string {
	var svcName string
	if svctype == "local" {
		svcName = name
	}

	if svctype == "external" {
		svcName = name + "-svc" // consistent single svc name
	}
	return svcName
}
