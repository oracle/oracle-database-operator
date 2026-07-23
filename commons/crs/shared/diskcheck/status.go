package diskcheck

import (
	"bytes"
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckDaemonSetReadyAndDiskValidation checks daemonset readiness and scans pod logs for invalid-device errors.
// It returns:
// - ready=true when daemonset has all desired pods ready
// - invalidDevice=true when logs show "not a valid block device"
// - err for API/read failures
func CheckDaemonSetReadyAndDiskValidation(
	ctx context.Context,
	cl client.Client,
	kubeClient kubernetes.Interface,
	namespace string,
	daemonsetName string,
	labelSelector string,
) (ready bool, invalidDevice bool, err error) {
	ds := &appsv1.DaemonSet{}
	if err := cl.Get(ctx, types.NamespacedName{Name: daemonsetName, Namespace: namespace}, ds); err != nil {
		return false, false, err
	}

	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.NumberReady > 0 {
		return true, false, nil
	}

	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return false, false, err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		logs, err := kubeClient.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
		if err != nil {
			continue
		}
		if bytes.Contains(logs, []byte("not a valid block device")) {
			return false, true, nil
		}
	}
	return false, false, nil
}
