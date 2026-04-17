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

	"github.com/go-logr/logr"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	piDataMount     = "/stage"
	defaultLogMount = "/privateai/logs"
)

// LogMessages emits structured logs based on message type and debug settings.
func LogMessages(msgtype string, msg string, err error, instance *privateaiv4.PrivateAi, logger logr.Logger) {
	// setting logrus formatter
	//logrus.SetFormatter(&logrus.JSONFormatter{})
	//logrus.SetOutput(os.Stdout)

	if msgtype == "DEBUG" && instance.Spec.IsDebug {
		if err != nil {
			logger.Error(err, msg)
		} else {
			logger.Info(msg)
		}
	} else if msgtype == "INFO" {
		logger.Info(msg)
	} else if msgtype == "Error" {
		logger.Error(err, msg)
	}
}

func getOwnerRef(instance *privateaiv4.PrivateAi,
) []metav1.OwnerReference {

	var ownerRef []metav1.OwnerReference
	ownerRef = append(ownerRef, metav1.OwnerReference{Kind: instance.GroupVersionKind().Kind, APIVersion: instance.APIVersion, Name: instance.Name, UID: types.UID(instance.UID)})
	return ownerRef
}

// FUnction to build the svc definition for catalog/shard and GSM
func buildSvcPortsDef(instance *privateaiv4.PrivateAi, svcType string) []corev1.ServicePort {
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

	if svcType != "external" {
		return result
	}

	external := resolvePaiExternalServiceSettings(instance)
	if external.Port == 0 && external.TargetPort == 0 {
		return result
	}

	if len(result) == 0 {
		targetPort, _, _, _ := resolveServicePort(&instance.Spec)
		if external.TargetPort > 0 {
			targetPort = external.TargetPort
		}
		servicePort := targetPort
		if external.Port > 0 {
			servicePort = external.Port
		}
		return []corev1.ServicePort{{
			Protocol: corev1.ProtocolTCP,
			Port:     servicePort,
			Name:     fmt.Sprintf("tcp-%d-%d", servicePort, targetPort),
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: targetPort,
			},
		}}
	}

	if external.Port > 0 {
		result[0].Port = external.Port
	}
	if external.TargetPort > 0 {
		result[0].TargetPort = intstr.FromInt(int(external.TargetPort))
	}
	result[0].Name = fmt.Sprintf("tcp-%d-%d", result[0].Port, result[0].TargetPort.IntVal)
	return result
}

// Function to generate the port mapping
func generatePortMapping(portMapping privateaiv4.PaiPortMapping) string {
	return fmt.Sprintf("tcp-%d-%d", portMapping.Port, portMapping.TargetPort)
}

// GetFmtStr formats a value for bracketed log display.
func GetFmtStr(pstr string,
) string {
	return "[" + pstr + "]"
}

// CheckDepSet returns the deployment for the given PrivateAi instance.
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

// DelPvc deletes the named PVC in the PrivateAi namespace.
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

// CheckSvc fetches the named service in the PrivateAi namespace.
func CheckSvc(svcName string, instance *privateaiv4.PrivateAi, kClient client.Client) (*corev1.Service, error) {
	// If this is a PrivateAi instance
	//	if instance.Kind == "PrivateAi" && !strings.HasSuffix(svcName, "-svc") {
	//		svcName = instance.Name + "-svc"
	//	}

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

// CheckSecret fetches the named secret in the PrivateAi namespace.
func CheckSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, _ logr.Logger) (*corev1.Secret, error) {

	sc := &corev1.Secret{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      secName,
		Namespace: instance.Namespace,
	}, sc)

	return sc, err
}

// CheckConfigMap fetches the named ConfigMap in the PrivateAi namespace.
func CheckConfigMap(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, _ logr.Logger) (*corev1.ConfigMap, error) {

	sc := &corev1.ConfigMap{}
	err := kClient.Get(context.TODO(), types.NamespacedName{
		Name:      cName,
		Namespace: instance.Namespace,
	}, sc)

	return sc, err
}

// ReadSecret reads key fields from the named secret and returns api-key/cert values.
func ReadSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger,
) (string, string) {

	var apiKeyVal string
	var certPemVal string
	sc, err := CheckSecret(secName, instance, kClient, logger)

	if err != nil {
		return "NONE", "NONE"
	}

	// Secret Evaluation
	for k, val := range sc.Data {
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

// PatchSecret patches ownership labels onto the named secret.
func PatchSecret(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger,
) error {

	// Reading a Secret
	sc, err := CheckSecret(secName, instance, kClient, logger)
	if err != nil {
		return err
	}

	scLabels := sc.GetLabels()
	if len(scLabels) != 0 {
		if _, ok := scLabels["app.kubernetes.io/privateai-resource-name"]; ok {
			return nil
		}
	}

	scCopy := sc.DeepCopy()
	scCopy.Labels = make(map[string]string)
	scCopy.Labels["app.kubernetes.io/privateai-resource-name"] = "PrivateAi-" + instance.Name
	patch := client.MergeFrom(sc)
	err = kClient.Patch(context.Background(), scCopy, patch)
	if err != nil {
		return err
	}
	return nil
}

// PatchConfigMap patches ownership labels onto the named ConfigMap.
func PatchConfigMap(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger,
) error {

	// Reading a configmap
	cc, err := CheckConfigMap(cName, instance, kClient, logger)
	if err != nil {
		return err
	}

	cLabels := cc.GetLabels()
	if len(cLabels) != 0 {
		if _, ok := cLabels["app.kubernetes.io/privateai-resource-name"]; ok {
			return nil
		}
	}

	ccCopy := cc.DeepCopy()
	ccCopy.Labels = make(map[string]string)
	ccCopy.Labels["app.kubernetes.io/privateai-resource-name"] = "PrivateAi-" + instance.Name
	patch := client.MergeFrom(cc)
	err = kClient.Patch(context.Background(), ccCopy, patch)
	if err != nil {
		return err
	}
	return nil
}

// GetSecretResourceVersion returns the resourceVersion for the named secret.
func GetSecretResourceVersion(secName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger,
) string {
	sc, err := CheckSecret(secName, instance, kClient, logger)
	if err != nil {
		return "None"
	}
	return sc.ResourceVersion
}

// GetConfigMapResourceVersion returns the resourceVersion for the named ConfigMap.
func GetConfigMapResourceVersion(cName string, instance *privateaiv4.PrivateAi, kClient client.Client, logger logr.Logger,
) string {
	cc, err := CheckConfigMap(cName, instance, kClient, logger)
	if err != nil {
		return "None"
	}
	return cc.ResourceVersion
}

// TernaryCondition returns trueVal when condition is true, otherwise falseVal.
func TernaryCondition[Y any](condition bool, trueVal, falseVal Y) Y {
	if condition {
		return trueVal
	}
	return falseVal
}

// GetSvcName builds a service name for the given service kind.
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
