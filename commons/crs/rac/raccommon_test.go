package commons

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type pvcDeleteTrackingClient struct {
	client.Client
	ops              []string
	updateFinalizers []string
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
	return c.Client.Delete(ctx, obj, opts...)
}

func TestPodListValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		podList          *corev1.PodList
		sfName           string
		wantExists       bool
		wantReadyPodName string
		wantNotReadyName string
	}{
		{
			name:   "ready pod accepted via pod ready condition",
			sfName: "racnode1",
			podList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "racnode1-0"},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
							Conditions: []corev1.PodCondition{
								{Type: corev1.PodReady, Status: corev1.ConditionTrue},
							},
							ContainerStatuses: []corev1.ContainerStatus{
								{Name: "oracle", Ready: false},
							},
						},
					},
				},
			},
			wantExists:       true,
			wantReadyPodName: "racnode1-0",
		},
		{
			name:   "first matching not ready pod returned when no ready pod exists",
			sfName: "racnode1",
			podList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "racnode1-0"},
						Status: corev1.PodStatus{
							Phase: corev1.PodPending,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "other-0"},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
							Conditions: []corev1.PodCondition{
								{Type: corev1.PodReady, Status: corev1.ConditionTrue},
							},
						},
					},
				},
			},
			wantExists:       false,
			wantNotReadyName: "racnode1-0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotExists, gotPod, gotNotReady := PodListValidation(tt.podList, tt.sfName, &racdb.RacDatabase{}, nil)
			if gotExists != tt.wantExists {
				t.Fatalf("PodListValidation exists=%v, want %v", gotExists, tt.wantExists)
			}
			if tt.wantReadyPodName == "" {
				if gotPod != nil {
					t.Fatalf("expected nil ready pod, got %s", gotPod.Name)
				}
			} else if gotPod == nil || gotPod.Name != tt.wantReadyPodName {
				t.Fatalf("expected ready pod %q, got %#v", tt.wantReadyPodName, gotPod)
			}
			if tt.wantNotReadyName == "" {
				if gotNotReady != nil {
					t.Fatalf("expected nil notReady pod, got %s", gotNotReady.Name)
				}
			} else if gotNotReady == nil || gotNotReady.Name != tt.wantNotReadyName {
				t.Fatalf("expected notReady pod %q, got %#v", tt.wantNotReadyName, gotNotReady)
			}
		})
	}
}

func TestVolumePVCForASMDefaultsToReadWriteManyForMultiNodeStorageClass(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 2,
			},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               racdb.CrsAsmDiskDg,
					Disks:              []string{"/dev/asm-disk1"},
					StorageClass:       "oci-bv",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	pvc := VolumePVCForASM(instance, 0, 0, "/dev/asm-disk1", "DATA", "50Gi")
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Fatalf("expected storageClass-backed ASM PVC accessModes=[ReadWriteMany], got %v", pvc.Spec.AccessModes)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "oci-bv" {
		t.Fatalf("expected storageClassName=oci-bv, got %v", pvc.Spec.StorageClassName)
	}
	if pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeBlock {
		t.Fatalf("expected volumeMode=Block, got %v", pvc.Spec.VolumeMode)
	}
}

func TestVolumePVCForASMHonorsExplicitReadWriteOnceOverride(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 3,
			},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               racdb.CrsAsmDiskDg,
					Disks:              []string{"/dev/asm-disk1"},
					StorageClass:       "oci-bv",
					AccessMode:         string(corev1.ReadWriteOnce),
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	pvc := VolumePVCForASM(instance, 0, 0, "/dev/asm-disk1", "DATA", "50Gi")
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Fatalf("expected explicit ASM PVC accessModes=[ReadWriteOnce], got %v", pvc.Spec.AccessModes)
	}
}

func TestVolumePVCForASMDefaultsToReadWriteManyForRawDisks(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:  "DATA",
					Type:  racdb.CrsAsmDiskDg,
					Disks: []string{"/dev/asm-disk1"},
				},
			},
		},
	}

	pvc := VolumePVCForASM(instance, 0, 0, "/dev/asm-disk1", "DATA", "50Gi")
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Fatalf("expected raw ASM PVC accessModes=[ReadWriteMany], got %v", pvc.Spec.AccessModes)
	}
	if pvc.Spec.Selector == nil {
		t.Fatalf("expected selector for raw ASM PVC")
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "" {
		t.Fatalf("expected empty storageClassName to prevent default class injection, got %v", pvc.Spec.StorageClassName)
	}
}

func TestSoftwareStorageClassUsesClaimTemplateVolumeName(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
			},
			ConfigParams: &racdb.RacInitParams{
				SwMountLocation: "/u01",
			},
			Image: "example.com/rac:19c",
			SshKeySecret: &racdb.RACSshSecretDetails{
				Name:             "ssh-key-secret",
				KeyMountLocation: "/mnt/.ssh",
			},
			SwStorageClass:       "oci-bv-ext4",
			SwLocStorageSizeInGb: 300,
		},
	}

	wantClaimTemplateName := GetClusterSwPvcName("racnode1")
	claims := VolumeClaimTemplatesForRacCluster(instance, instance.Spec.ClusterDetails, 0)
	if len(claims) != 1 || claims[0].Name != wantClaimTemplateName {
		t.Fatalf("expected software claim template %q, got %#v", wantClaimTemplateName, claims)
	}
	if claims[0].Namespace != "" {
		t.Fatalf("expected software claim template namespace to be empty, got %q", claims[0].Namespace)
	}
	if claims[0].Spec.VolumeMode == nil || *claims[0].Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Fatalf("expected software claim template volumeMode to be Filesystem, got %#v", claims[0].Spec.VolumeMode)
	}

	volumes := buildVolumeSpecForRacCluster(instance, instance.Spec.ClusterDetails, 0)
	for _, vol := range volumes {
		if vol.Name == "racnode1-oradata-sw-vol" {
			t.Fatalf("unexpected explicit software PVC volume found in storageClass mode: %#v", vol)
		}
	}

	volumeMounts := buildVolumeMountSpecForRacCluster(instance, instance.Spec.ClusterDetails, 0)
	foundMount := false
	for _, vm := range volumeMounts {
		if vm.MountPath == "/u01" {
			foundMount = true
			if vm.Name != wantClaimTemplateName {
				t.Fatalf("expected /u01 mount to use claim template volume %q, got %q", wantClaimTemplateName, vm.Name)
			}
		}
	}
	if !foundMount {
		t.Fatal("expected /u01 volume mount to be present")
	}
}

func TestVolumeClaimTemplatesForRacCluster_DoesNotCreateAsmClaimTemplates(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
			},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               racdb.CrsAsmDiskDg,
					Disks:              []string{"/dev/asm-disk1"},
					StorageClass:       "oci-bv",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	claims := VolumeClaimTemplatesForRacCluster(instance, instance.Spec.ClusterDetails, 0)
	if len(claims) != 0 {
		t.Fatalf("expected no ASM claim templates for shared ASM PVCs, got %#v", claims)
	}
}

func TestDeletePVCIfExistsRemovesFinalizersBeforeDelete(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "racnode1-oradata-sw-pvc-racnode1-0",
			Namespace:  "rac",
			Finalizers: []string{"kubernetes.io/pvc-protection"},
		},
	}
	unrelatedPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1-oradata-sw-pvc-racnode1-0-unrelated",
			Namespace: "rac",
		},
	}

	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(instance, pvc, unrelatedPVC).Build()
	trackingClient := &pvcDeleteTrackingClient{Client: baseClient}

	if err := deletePVCIfExists(instance, pvc.Name, trackingClient, logr.Discard()); err != nil {
		t.Fatalf("deletePVCIfExists returned error: %v", err)
	}
	if !reflect.DeepEqual(trackingClient.ops, []string{"update", "delete"}) {
		t.Fatalf("expected update then delete, got %v", trackingClient.ops)
	}
	if len(trackingClient.updateFinalizers) != 0 {
		t.Fatalf("expected finalizers to be cleared before update, got %v", trackingClient.updateFinalizers)
	}

	pvcLookup := &corev1.PersistentVolumeClaim{}
	err := baseClient.Get(context.Background(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvcLookup)
	if err == nil {
		t.Fatalf("expected PVC %s to be deleted", pvc.Name)
	}

	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: unrelatedPVC.Name, Namespace: unrelatedPVC.Namespace}, &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("expected unrelated PVC %s to remain, got error: %v", unrelatedPVC.Name, err)
	}
}
