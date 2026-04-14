package commons

import (
	"testing"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestVolumePVCForASMUsesReadWriteOnceForStorageClass(t *testing.T) {
	t.Parallel()

	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdb",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
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
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Fatalf("expected storageClass-backed ASM PVC accessModes=[ReadWriteOnce], got %v", pvc.Spec.AccessModes)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "oci-bv" {
		t.Fatalf("expected storageClassName=oci-bv, got %v", pvc.Spec.StorageClassName)
	}
}

func TestVolumePVCForASMUsesReadWriteOnceForRawDisks(t *testing.T) {
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
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Fatalf("expected raw ASM PVC accessModes=[ReadWriteOnce], got %v", pvc.Spec.AccessModes)
	}
	if pvc.Spec.Selector == nil {
		t.Fatalf("expected selector for raw ASM PVC")
	}
}
