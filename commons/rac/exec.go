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
	"bytes"
	"context"
	"net/http"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecCommand executes the specified command inside the first container of the given pod.
func ExecCommand(podName string, cmd []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *racdb.RacDatabase, logger logr.Logger) (string, string, error) {

	var msg string
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	pod, err := kubeClient.CoreV1().
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

	req := kubeClient.CoreV1().RESTClient().
		Post().
		Namespace(instance.Namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		}, scheme.ParameterCodec)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return "Error Occurred", "Error Occurred", err
	}

	// Connect to url (constructed from req) using SPDY (HTTP/2) protocol which allows bidirectional streams.
	exec, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, req.URL())
	if err != nil {
		msg = "Error after executing remotecommand.NewSPDYExecutor"
		LogMessages("Error", msg, err, instance, logger)
		return "Error Occurred", "Error Occurred", err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    true,
	})
	if err != nil {
		msg = "Command execution failed inside the container!"
		LogMessages("DEBUG", msg, err, instance, logger)
		if len(execOut.String()) > 0 {
			LogMessages("INFO", execOut.String(), nil, instance, logger)
		}
		if len(execErr.String()) > 0 {
			LogMessages("INFO", execErr.String(), nil, instance, logger)
		}
		return execOut.String(), execErr.String(), err
	}

	return execOut.String(), execErr.String(), nil
}
