package connectionmanager

import (
	"context"
	"fmt"
	"strings"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	connectionManagerStorageVolumeName      = "cman-storage"
	connectionManagerStorageClaimSuffix     = "-storage"
	connectionManagerStorageOwnerAnnotation = "network.oracle.com/connection-manager-storage-owner"
)

func connectionManagerStorageClaimName(inst *networkv4.ConnectionManager) string {
	if inst == nil || inst.Spec.Storage == nil {
		return ""
	}
	if name := strings.TrimSpace(inst.Spec.Storage.PvcName); name != "" {
		return name
	}
	return inst.Name + connectionManagerStorageClaimSuffix
}

func connectionManagerReferencesPVC(inst *networkv4.ConnectionManager, pvcName string) bool {
	return inst != nil && strings.TrimSpace(pvcName) != "" && connectionManagerStorageClaimName(inst) == pvcName
}

func (r *ConnectionManagerReconciler) reconcileConnectionManagerStorage(ctx context.Context, inst *networkv4.ConnectionManager) error {
	storage := inst.Spec.Storage
	if storage == nil {
		return nil
	}

	claimName := connectionManagerStorageClaimName(inst)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: claimName, Namespace: inst.Namespace}, pvc)
	if strings.TrimSpace(storage.PvcName) != "" {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("existing storage PVC %q was not found", claimName)
		}
		return err
	}
	if err == nil {
		return r.reconcileManagedConnectionManagerStorage(ctx, inst, pvc)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        claimName,
			Namespace:   inst.Namespace,
			Labels:      connectionManagerLabels(inst),
			Annotations: map[string]string{connectionManagerStorageOwnerAnnotation: connectionManagerStorageOwner(inst)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: *resource.NewQuantity(int64(storage.StorageSizeInGb)*1024*1024*1024, resource.BinarySI),
			}},
			StorageClassName: func() *string {
				value := strings.TrimSpace(storage.StorageClass)
				return &value
			}(),
		},
	}
	if !storage.RetainOnDelete {
		if err := controllerutil.SetControllerReference(inst, pvc, r.Scheme); err != nil {
			return err
		}
	}
	return r.Create(ctx, pvc)
}

func (r *ConnectionManagerReconciler) reconcileManagedConnectionManagerStorage(ctx context.Context, inst *networkv4.ConnectionManager, pvc *corev1.PersistentVolumeClaim) error {
	controllerRef := metav1.GetControllerOf(pvc)
	if controllerRef != nil && !connectionManagerOwnerReferenceMatches(*controllerRef, inst) {
		return fmt.Errorf("managed storage PVC %q is controlled by another resource", pvc.Name)
	}
	ownerAnnotationMatches := pvc.Annotations != nil && pvc.Annotations[connectionManagerStorageOwnerAnnotation] == connectionManagerStorageOwner(inst)
	if !metav1.IsControlledBy(pvc, inst) && !ownerAnnotationMatches {
		return fmt.Errorf("managed storage PVC %q already exists but is not controlled by ConnectionManager %s/%s", pvc.Name, inst.Namespace, inst.Name)
	}

	updated := false
	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	if pvc.Annotations[connectionManagerStorageOwnerAnnotation] != connectionManagerStorageOwner(inst) {
		pvc.Annotations[connectionManagerStorageOwnerAnnotation] = connectionManagerStorageOwner(inst)
		updated = true
	}
	if inst.Spec.Storage.RetainOnDelete {
		if removeConnectionManagerOwnerReference(pvc, inst) {
			updated = true
		}
	} else if !metav1.IsControlledBy(pvc, inst) {
		if err := controllerutil.SetControllerReference(inst, pvc, r.Scheme); err != nil {
			return err
		}
		updated = true
	}
	if updated {
		return r.Update(ctx, pvc)
	}
	return nil
}

func connectionManagerStorageOwner(inst *networkv4.ConnectionManager) string {
	return inst.Namespace + "/" + inst.Name
}

func connectionManagerOwnerReferenceMatches(ref metav1.OwnerReference, inst *networkv4.ConnectionManager) bool {
	return ref.Kind == "ConnectionManager" && ref.Name == inst.Name && ref.UID == inst.UID
}

func removeConnectionManagerOwnerReference(pvc *corev1.PersistentVolumeClaim, inst *networkv4.ConnectionManager) bool {
	if len(pvc.OwnerReferences) == 0 {
		return false
	}
	ownerReferences := make([]metav1.OwnerReference, 0, len(pvc.OwnerReferences))
	removed := false
	for _, ref := range pvc.OwnerReferences {
		if connectionManagerOwnerReferenceMatches(ref, inst) {
			removed = true
			continue
		}
		ownerReferences = append(ownerReferences, ref)
	}
	if removed {
		pvc.OwnerReferences = ownerReferences
	}
	return removed
}

func connectionManagerPVCRequests(ctx context.Context, reader client.Reader, pvc *corev1.PersistentVolumeClaim) []types.NamespacedName {
	list := &networkv4.ConnectionManagerList{}
	if err := reader.List(ctx, list, client.InNamespace(pvc.Namespace)); err != nil {
		return nil
	}
	requests := make([]types.NamespacedName, 0)
	for i := range list.Items {
		inst := &list.Items[i]
		if connectionManagerReferencesPVC(inst, pvc.Name) {
			requests = append(requests, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace})
		}
	}
	return requests
}
