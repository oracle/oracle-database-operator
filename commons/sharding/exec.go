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
	"fmt"
	"net/http"
	"time"

	databasealphav1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/cmd/cp"
	"k8s.io/kubectl/pkg/cmd/util"
)

// ExecCMDInContainer execute command in first container of a pod
func ExecCommand(podName string, cmd []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *databasealphav1.ShardingDatabase, logger logr.Logger) (string, string, error) {

	var msg string
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	req := kubeClient.CoreV1().RESTClient().
		Post().
		Namespace(instance.Spec.Namespace).
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

func GetPodCopyConfig(kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *databasealphav1.ShardingDatabase, logger logr.Logger) (*rest.Config, *kubernetes.Clientset, error) {

	var clientSet *kubernetes.Clientset
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return config, clientSet, err
	}
	clientSet, err = kubernetes.NewForConfig(config)
	config.APIPath = "/api"
	config.GroupVersion = &schema.GroupVersion{Version: "v1"}
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}

	return config, clientSet, err

}

func KctlCopyFile(kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *databasealphav1.ShardingDatabase, restConfig *rest.Config, kclientset *kubernetes.Clientset, logger logr.Logger, src string, dst string, containername string) (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer, error) {

	var in, out, errOut *bytes.Buffer
	var ioStreams genericclioptions.IOStreams
	for count := 0; ; count++ {
		ioStreams, in, out, errOut = genericclioptions.NewTestIOStreams()
		copyOptions := cp.NewCopyOptions(ioStreams)
		copyOptions.ClientConfig = restConfig
		if len(containername) != 0 {
			copyOptions.Container = containername
		}
		configFlags := genericclioptions.NewConfigFlags(false)
		f := util.NewFactory(configFlags)
		cmd := cp.NewCmdCp(f, ioStreams)
		err := copyOptions.Complete(f, cmd, []string{src, dst})
		if err != nil {
			return nil, nil, nil, err
		}

		c := rest.CopyConfig(restConfig)
		cs, err := kubernetes.NewForConfig(c)
		if err != nil {
			return nil, nil, nil, err
		}

		copyOptions.ClientConfig = c
		copyOptions.Clientset = cs

		err = copyOptions.Run()
		if err != nil {
			if !shouldRetry(count, err) {
				return nil, nil, nil, fmt.Errorf("could not run copy operation: %v. Stdout: %v, Stderr: %v", err, out.String(), errOut.String())
			}
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	return in, out, errOut, nil

}

func shouldRetry(count int, err error) bool {
	if count < connectFailureMaxTries {
		return err.Error() == errorDialingBackendEOF
	}
	return false
}
