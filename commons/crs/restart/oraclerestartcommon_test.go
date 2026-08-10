package commons

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	oraclerestart "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type pvcDeleteTrackingClient struct {
	client.Client
	ops              []string
	updateFinalizers []string
	deleteFinalizers []string
}

func (c *pvcDeleteTrackingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.ops = append(c.ops, "update")
	if pvc, ok := obj.(*corev1.PersistentVolumeClaim); ok {
		c.updateFinalizers = append([]string(nil), pvc.GetFinalizers()...)
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *pvcDeleteTrackingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.ops = append(c.ops, "delete")
	if pvc, ok := obj.(*corev1.PersistentVolumeClaim); ok {
		c.deleteFinalizers = append([]string(nil), pvc.GetFinalizers()...)
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestBuildEnvVarsSpecCopiesOracleRestartEnvVars(t *testing.T) {
	t.Parallel()

	envVars := []corev1.EnvVar{
		{Name: "LOG_DIR", Value: "/tmp/orod"},
		{Name: "ORA_LOG_MAX_BYTES", Value: "104857"},
	}

	got := buildEnvVarsSpec(envVars)
	if !reflect.DeepEqual(got, envVars) {
		t.Fatalf("expected env vars %v, got %v", envVars, got)
	}

	envVars[0].Value = "/changed"
	if got[0].Value != "/tmp/orod" {
		t.Fatalf("expected env vars to be copied, got %q", got[0].Value)
	}
}

func TestDelORestartPVCDeletesWithoutClearingForeignFinalizers(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	instance := &oraclerestart.OracleRestart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-sample",
			Namespace: "restartns",
		},
	}
	diskName := "/dev/asm-disk1"
	foreignFinalizers := []string{"kubernetes.io/pvc-protection"}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       GetAsmPvcName(diskName, instance.Name),
			Namespace:  instance.Namespace,
			Finalizers: append([]string(nil), foreignFinalizers...),
		},
	}

	cl := &pvcDeleteTrackingClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc.DeepCopy()).Build(),
	}

	if err := DelORestartPVC(instance, 0, 0, diskName, cl, logr.Discard()); err != nil {
		t.Fatalf("DelORestartPVC returned error: %v", err)
	}
	if !reflect.DeepEqual(cl.ops, []string{"delete"}) {
		t.Fatalf("expected only delete operation, got %v", cl.ops)
	}
	if len(cl.updateFinalizers) != 0 {
		t.Fatalf("expected no finalizer-stripping update, got update finalizers %v", cl.updateFinalizers)
	}
	if !reflect.DeepEqual(cl.deleteFinalizers, foreignFinalizers) {
		t.Fatalf("expected delete to preserve finalizers %v, got %v", foreignFinalizers, cl.deleteFinalizers)
	}
}
