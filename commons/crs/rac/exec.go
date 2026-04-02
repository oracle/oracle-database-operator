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

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedk8sexec "github.com/oracle/oracle-database-operator/commons/crs/shared/k8sexec"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ExecCommandResp bundles Kubernetes client/config used by remote exec calls.
type ExecCommandResp = sharedk8sexec.ExecCommandResp

// NewExecCommandResp creates a reusable exec context wrapper.
func NewExecCommandResp(kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig) *ExecCommandResp {
	return sharedk8sexec.NewExecCommandResp(kubeClient, kubeConfig)
}

// ExecCommand executes the specified command inside the first container of the given pod using bundled exec context.
func ExecCommand(podName string, cmd []string, resp *ExecCommandResp, instance *racdb.RacDatabase, logger logr.Logger) (string, string, error) {
	return ExecCommandWithResp(podName, cmd, resp, instance, logger)
}

// ExecCommandWithResp executes the specified command inside the first container of the given pod using bundled client/config.
func ExecCommandWithResp(podName string, cmd []string, resp *ExecCommandResp, instance *racdb.RacDatabase, logger logr.Logger) (string, string, error) {
	if instance == nil {
		return "", "", fmt.Errorf("invalid exec request: instance is nil")
	}
	if strings.TrimSpace(instance.Namespace) == "" {
		return "", "", fmt.Errorf("invalid exec request: instance namespace is empty")
	}
	if strings.TrimSpace(podName) == "" {
		return "", "", fmt.Errorf("invalid exec request: pod name is empty")
	}
	if len(cmd) == 0 {
		return "", "", fmt.Errorf("invalid exec request: command is empty")
	}
	if resp == nil || resp.KubeClient == nil || resp.KubeConfig == nil {
		return "", "", fmt.Errorf("invalid exec context: kube client/config is not initialized")
	}

	pod, err := resp.KubeClient.CoreV1().
		Pods(instance.Namespace).
		Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		// Pod not found or API error → allow delete flow to continue
		logger.Info(
			"Failed to get pod for exec, skipping exec",
			"pod", podName,
			"namespace", instance.Namespace,
			"error", err,
		)
		return "", "", nil
	}

	// ---- Exec eligibility check (pod-first, state-aware) ----
	if pod.Status.Phase != corev1.PodRunning {
		logger.Info(
			"Skipping exec because pod is not Running",
			"pod", podName,
			"podPhase", pod.Status.Phase,
			"racState", instance.Status.State,
			"namespace", instance.Namespace,
		)
		return "", "", nil
	}
	if len(pod.Spec.Containers) == 0 {
		logger.Info(
			"Skipping exec because pod has no containers",
			"pod", podName,
			"namespace", instance.Namespace,
		)
		return "", "", nil
	}

	// // Check container readiness (important for exec)
	// ready := false
	// for _, cond := range pod.Status.Conditions {
	// 	if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
	// 		ready = true
	// 		break
	// 	}
	// }

	// if !ready {
	// 	logger.Info(
	// 		"Skipping exec because pod is not Ready",
	// 		"pod", podName,
	// 		"racState", instance.Status.State,
	// 		"namespace", instance.Namespace,
	// 	)
	// 	return "", "", nil
	// }

	stdOut, stdErr, err := sharedk8sexec.StreamExec(context.Background(), resp, instance.Namespace, podName, pod.Spec.Containers[0].Name, cmd, true)
	if err != nil {
		LogMessages("DEBUG", "Command execution failed inside the container!", err, instance, logger)
		if len(stdOut) > 0 {
			LogMessages("INFO", stdOut, nil, instance, logger)
		}
		if len(stdErr) > 0 {
			LogMessages("INFO", stdErr, nil, instance, logger)
		}
		return stdOut, stdErr, err
	}
	return stdOut, stdErr, nil
}
