package k8sobjects

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigMapSpec builds a standard ConfigMap object used by RAC and Oracle Restart.
func ConfigMapSpec(namespace, instanceName, cmName string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
			Labels: map[string]string{
				"name": instanceName + cmName,
			},
		},
		Data: data,
	}
}
