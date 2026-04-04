package privateai

import (
	"context"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type countingStatusWriter struct {
	client.StatusWriter
	updateCalls int
}

func (w *countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.updateCalls++
	return w.StatusWriter.Update(ctx, obj, opts...)
}

type countingClient struct {
	client.Client
	statusWriter *countingStatusWriter
}

func (c *countingClient) Status() client.StatusWriter {
	return c.statusWriter
}

func TestReconcileSingleStatusWrite(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding appsv1 scheme: %v", err)
	}

	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pai-single-status",
			Namespace: "default",
		},
		Spec: privateaiv4.PrivateAiSpec{
			PaiService: privateaiv4.PaiServiceSpec{
				PortMappings: []privateaiv4.PaiPortMapping{
					{
						Protocol:   corev1.ProtocolTCP,
						Port:       8080,
						TargetPort: 8080,
					},
				},
			},
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithStatusSubresource(&privateaiv4.PrivateAi{}).
		WithObjects(instance).
		Build()

	counting := &countingClient{
		Client: baseClient,
		statusWriter: &countingStatusWriter{
			StatusWriter: baseClient.Status(),
		},
	}

	r := &PrivateAiReconciler{
		Client: counting,
		Scheme: sch,
		Config: &rest.Config{},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if counting.statusWriter.updateCalls != 1 {
		t.Fatalf("expected exactly 1 status update, got %d", counting.statusWriter.updateCalls)
	}
}
