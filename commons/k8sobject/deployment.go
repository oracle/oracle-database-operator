package k8sobjects

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentReconcileResult reports whether a Deployment was created or updated.
type DeploymentReconcileResult struct {
	Created bool
	Updated bool
}

// ReconcileDeployment creates the Deployment if missing and optionally applies callback-driven updates.
// updateFn should mutate the found object and return true when an update is required.
func ReconcileDeployment(
	ctx context.Context,
	cl client.Client,
	namespace string,
	desired *appsv1.Deployment,
	updateFn func(found *appsv1.Deployment, desired *appsv1.Deployment) bool,
) (*appsv1.Deployment, DeploymentReconcileResult, error) {
	var found *appsv1.Deployment
	result := DeploymentReconcileResult{}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &appsv1.Deployment{}
		err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, current)
		if err != nil && apierrors.IsNotFound(err) {
			toCreate := desired.DeepCopy()
			if err := cl.Create(ctx, toCreate); err != nil {
				return err
			}
			found = toCreate
			result = DeploymentReconcileResult{Created: true}
			return nil
		}
		if err != nil {
			return err
		}

		found = current
		result = DeploymentReconcileResult{}
		if updateFn == nil || !updateFn(current, desired) {
			return nil
		}
		if err := cl.Update(ctx, current); err != nil {
			return err
		}
		found = current
		result = DeploymentReconcileResult{Updated: true}
		return nil
	})
	if err != nil {
		return nil, DeploymentReconcileResult{}, err
	}
	return found, result, nil
}
