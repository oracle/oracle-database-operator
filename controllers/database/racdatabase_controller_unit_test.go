package controllers

import (
	"context"
	"testing"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHasPendingRacPods(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "default",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				RacNodeName: "racnode",
				NodeCount:   1,
			},
		},
	}

	tests := []struct {
		name string
		pods []corev1.Pod
		want bool
	}{
		{
			name: "no RAC pods pending",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "racnode1-0",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			want: false,
		},
		{
			name: "pending RAC pod detected",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "racnode1-0",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			want: true,
		},
		{
			name: "unrelated pending pod ignored",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "racnode1-0",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-app-0",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			objects := make([]runtime.Object, 0, len(tt.pods)+1)
			objects = append(objects, rac.DeepCopy())
			for i := range tt.pods {
				objects = append(objects, tt.pods[i].DeepCopy())
			}

			r := &RacDatabaseReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(),
			}

			got, err := hasPendingRacPods(context.Background(), r, rac.DeepCopy())
			if err != nil {
				t.Fatalf("hasPendingRacPods returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("hasPendingRacPods = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncRACStatefulSetScopedFields_IgnoresUnrelatedTemplateDiffs(t *testing.T) {
	t.Parallel()

	found := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						racConfigMapHashAnnotation:    "same-hash",
						"k8s.v1.cni.cncf.io/networks": "old-network",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:          "racnode1-0",
							Image:         "old-image",
							VolumeDevices: []corev1.VolumeDevice{{Name: "dev1", DevicePath: "/dev/asm1"}},
						},
					},
				},
			},
		},
	}
	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						racConfigMapHashAnnotation:    "same-hash",
						"k8s.v1.cni.cncf.io/networks": "new-network",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:          "racnode1-0",
							Image:         "new-image",
							VolumeDevices: []corev1.VolumeDevice{{Name: "dev1", DevicePath: "/dev/asm1"}},
						},
					},
				},
			},
		},
	}

	if syncRACStatefulSetScopedFields(found, desired) {
		t.Fatalf("expected no update for unrelated template diffs")
	}
}

func TestComputeRACConfigMapHash_ChangesWithConfigMapContent(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	template := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "racnode1-oradata-envfile",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "rac-cmap"},
						},
					},
				},
				{
					Name: "racnode1-oradata-girsp",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "ignored-cmap"},
						},
					},
				},
			},
		},
	}
	reconciler := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "rac-cmap", Namespace: "default"},
				Data: map[string]string{
					"envfile": "A=1",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ignored-cmap", Namespace: "default"},
				Data: map[string]string{
					"rsp": "x",
				},
			},
		).Build(),
	}

	hash1, err := reconciler.computeRACConfigMapHash(context.Background(), "default", template)
	if err != nil {
		t.Fatalf("computeRACConfigMapHash returned error: %v", err)
	}

	if err := reconciler.Client.Update(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rac-cmap", Namespace: "default"},
		Data: map[string]string{
			"envfile": "A=2",
		},
	}); err != nil {
		t.Fatalf("failed to update ConfigMap: %v", err)
	}

	hash2, err := reconciler.computeRACConfigMapHash(context.Background(), "default", template)
	if err != nil {
		t.Fatalf("computeRACConfigMapHash returned error after update: %v", err)
	}
	if hash1 == hash2 {
		t.Fatalf("expected ConfigMap hash to change when content changes")
	}
}

func TestComputeRACConfigMapHash_IgnoresNonEnvConfigMaps(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	template := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "racnode1-oradata-envfile",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "rac-cmap"},
						},
					},
				},
				{
					Name: "racnode1-oradata-dbrsp",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "ignored-cmap"},
						},
					},
				},
			},
		},
	}
	reconciler := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "rac-cmap", Namespace: "default"},
				Data: map[string]string{
					"envfile": "A=1",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ignored-cmap", Namespace: "default"},
				Data: map[string]string{
					"rsp": "before",
				},
			},
		).Build(),
	}

	hash1, err := reconciler.computeRACConfigMapHash(context.Background(), "default", template)
	if err != nil {
		t.Fatalf("computeRACConfigMapHash returned error: %v", err)
	}

	if err := reconciler.Client.Update(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "ignored-cmap", Namespace: "default"},
		Data: map[string]string{
			"rsp": "after",
		},
	}); err != nil {
		t.Fatalf("failed to update ignored ConfigMap: %v", err)
	}

	hash2, err := reconciler.computeRACConfigMapHash(context.Background(), "default", template)
	if err != nil {
		t.Fatalf("computeRACConfigMapHash returned error after ignored update: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf("expected non-env ConfigMap changes to be ignored")
	}
}

func TestSyncRACStatefulSetScopedFields_UpdatesOnConfigMapHashChange(t *testing.T) {
	t.Parallel()

	found := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{racConfigMapHashAnnotation: "old-hash"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "racnode1-0"}},
				},
			},
		},
	}
	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{racConfigMapHashAnnotation: "new-hash"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "racnode1-0"}},
				},
			},
		},
	}

	if !syncRACStatefulSetScopedFields(found, desired) {
		t.Fatalf("expected update when ConfigMap hash changes")
	}
	if got := found.Spec.Template.Annotations[racConfigMapHashAnnotation]; got != "new-hash" {
		t.Fatalf("expected ConfigMap hash to update to new-hash, got %q", got)
	}
}

func TestSyncRACStatefulSetScopedFields_UpdatesOnVolumeDeviceChange(t *testing.T) {
	t.Parallel()

	found := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{racConfigMapHashAnnotation: "same-hash"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:          "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{{Name: "dev1", DevicePath: "/dev/asm1"}},
					}},
				},
			},
		},
	}
	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{racConfigMapHashAnnotation: "same-hash"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "dev1", DevicePath: "/dev/asm1"},
							{Name: "dev2", DevicePath: "/dev/asm2"},
						},
					}},
				},
			},
		},
	}

	if !syncRACStatefulSetScopedFields(found, desired) {
		t.Fatalf("expected update when volume devices change")
	}
	if got := len(found.Spec.Template.Spec.Containers[0].VolumeDevices); got != 2 {
		t.Fatalf("expected 2 volume devices after update, got %d", got)
	}
}
