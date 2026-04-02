package k8sobjects

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsurePersistentVolume creates the PV when missing and validates local-disk source immutability.
func EnsurePersistentVolume(ctx context.Context, cl client.Client, desired *corev1.PersistentVolume) (string, bool, error) {
	found := &corev1.PersistentVolume{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return "", false, err
		}
		return desired.Name, true, nil
	}
	if err != nil {
		return "", false, err
	}

	if !reflect.DeepEqual(desired.Spec.PersistentVolumeSource.Local, found.Spec.PersistentVolumeSource.Local) {
		return "", false, fmt.Errorf("persistent volume %s has a different disk configuration. Please delete or update the existing PV to proceed", desired.Name)
	}
	return found.Name, false, nil
}

// EnsurePersistentVolumeClaim creates the PVC when missing and returns the existing claim name otherwise.
func EnsurePersistentVolumeClaim(ctx context.Context, cl client.Client, desired *corev1.PersistentVolumeClaim) (string, bool, error) {
	found := &corev1.PersistentVolumeClaim{}
	err := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if err := cl.Create(ctx, desired); err != nil {
			return "", false, err
		}
		return desired.Name, true, nil
	}
	if err != nil {
		return "", false, err
	}
	return found.Name, false, nil
}
