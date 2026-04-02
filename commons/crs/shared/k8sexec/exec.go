package k8sexec

import (
	"bytes"
	"context"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecCommandResp wraps Kubernetes clients required for pod exec.
type ExecCommandResp struct {
	KubeClient kubernetes.Interface
	KubeConfig clientcmd.ClientConfig
}

// NewExecCommandResp returns an initialized exec context wrapper.
func NewExecCommandResp(kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig) *ExecCommandResp {
	return &ExecCommandResp{KubeClient: kubeClient, KubeConfig: kubeConfig}
}

// StreamExec executes a command in the given pod/container and returns stdout/stderr.
func StreamExec(
	ctx context.Context,
	resp *ExecCommandResp,
	namespace string,
	podName string,
	container string,
	cmd []string,
	tty bool,
) (string, string, error) {
	var execOut bytes.Buffer
	var execErr bytes.Buffer

	req := resp.KubeClient.CoreV1().RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
			TTY:       tty,
		}, scheme.ParameterCodec)

	config, err := resp.KubeConfig.ClientConfig()
	if err != nil {
		return "", "", err
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}

	exec, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    tty,
	})
	return execOut.String(), execErr.String(), err
}
