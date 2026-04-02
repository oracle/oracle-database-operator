package k8sobjects

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapReconcileOp string

const (
	ConfigMapOpGet    ConfigMapReconcileOp = "get"
	ConfigMapOpCreate ConfigMapReconcileOp = "create"
	ConfigMapOpUpdate ConfigMapReconcileOp = "update"
)

// ConfigMapReconcileError includes which operation failed while reconciling a ConfigMap.
type ConfigMapReconcileError struct {
	Op  ConfigMapReconcileOp
	Err error
}

func (e *ConfigMapReconcileError) Error() string {
	return fmt.Sprintf("configmap reconcile %s failed: %v", e.Op, e.Err)
}

func (e *ConfigMapReconcileError) Unwrap() error { return e.Err }

// EnsureConfigMapExists creates the desired ConfigMap when it does not exist.
// Returns true when a create occurred.
func EnsureConfigMapExists(ctx context.Context, cl client.Client, namespace string, desired *corev1.ConfigMap) (bool, error) {
	found := &corev1.ConfigMap{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return false, &ConfigMapReconcileError{Op: ConfigMapOpCreate, Err: err}
		}
		return true, nil
	}
	if err != nil {
		return false, &ConfigMapReconcileError{Op: ConfigMapOpGet, Err: err}
	}
	return false, nil
}

// EnsureConfigMapEnvfile creates or updates only the "envfile" key.
// Returns true when object was created or envfile changed.
func EnsureConfigMapEnvfile(ctx context.Context, cl client.Client, namespace string, desired *corev1.ConfigMap) (bool, error) {
	found := &corev1.ConfigMap{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return false, &ConfigMapReconcileError{Op: ConfigMapOpCreate, Err: err}
		}
		return true, nil
	}
	if err != nil {
		return false, &ConfigMapReconcileError{Op: ConfigMapOpGet, Err: err}
	}

	if found.Data == nil {
		found.Data = map[string]string{}
	}
	desiredEnv := desired.Data["envfile"]
	if found.Data["envfile"] == desiredEnv {
		return false, nil
	}
	found.Data["envfile"] = desiredEnv
	if err := cl.Update(ctx, found); err != nil {
		return false, &ConfigMapReconcileError{Op: ConfigMapOpUpdate, Err: err}
	}
	return true, nil
}
