package k8sobjects

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatefulSetReconcileResult reports whether a StatefulSet was created or updated.
type StatefulSetReconcileResult struct {
	Created bool
	Updated bool
}

// ReconcileStatefulSet creates the StatefulSet if missing and optionally applies callback-driven updates.
// updateFn should mutate the found object and return true when an update is required.
func ReconcileStatefulSet(
	ctx context.Context,
	cl client.Client,
	namespace string,
	desired *appsv1.StatefulSet,
	updateFn func(found *appsv1.StatefulSet, desired *appsv1.StatefulSet) bool,
) (StatefulSetReconcileResult, error) {
	found := &appsv1.StatefulSet{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return StatefulSetReconcileResult{}, err
		}
		return StatefulSetReconcileResult{Created: true}, nil
	}
	if err != nil {
		return StatefulSetReconcileResult{}, err
	}

	if updateFn == nil {
		return StatefulSetReconcileResult{}, nil
	}
	if !updateFn(found, desired) {
		return StatefulSetReconcileResult{}, nil
	}
	if err := cl.Update(ctx, found); err != nil {
		return StatefulSetReconcileResult{}, err
	}
	return StatefulSetReconcileResult{Updated: true}, nil
}
