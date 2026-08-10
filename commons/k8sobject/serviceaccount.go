package k8sobjects

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureServiceAccountIfNotExists ensures the named ServiceAccount exists in the provided namespace.
func EnsureServiceAccountIfNotExists(ctx context.Context, kClient client.Client, namespace, name string) error {
	if name == "" {
		return nil
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	existing := &corev1.ServiceAccount{}
	err := kClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	return kClient.Create(ctx, sa)
}
