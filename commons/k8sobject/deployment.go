package k8sobjects

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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
	found := &appsv1.Deployment{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return nil, DeploymentReconcileResult{}, err
		}
		return desired.DeepCopy(), DeploymentReconcileResult{Created: true}, nil
	}
	if err != nil {
		return nil, DeploymentReconcileResult{}, err
	}

	if updateFn == nil {
		return found, DeploymentReconcileResult{}, nil
	}
	if !updateFn(found, desired) {
		return found, DeploymentReconcileResult{}, nil
	}
	if err := cl.Update(ctx, found); err != nil {
		return nil, DeploymentReconcileResult{}, err
	}
	return found, DeploymentReconcileResult{Updated: true}, nil
}
