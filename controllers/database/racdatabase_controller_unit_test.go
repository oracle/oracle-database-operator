//nolint:staticcheck // unit tests intentionally validate legacy controller behavior.
package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	raccommon "github.com/oracle/oracle-database-operator/commons/crs/rac"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
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

func TestShouldUpdateClusterListenerEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rac  *racdb.RacDatabase
		want bool
	}{
		{
			name: "nil instance",
			rac:  nil,
			want: false,
		},
		{
			name: "missing cluster details",
			rac:  &racdb.RacDatabase{},
			want: false,
		},
		{
			name: "listener nodeport disabled",
			rac: &racdb.RacDatabase{
				Spec: racdb.RacDatabaseSpec{
					ClusterDetails: &racdb.RacClusterDetailSpec{},
				},
			},
			want: false,
		},
		{
			name: "listener nodeport configured",
			rac: &racdb.RacDatabase{
				Spec: racdb.RacDatabaseSpec{
					ClusterDetails: &racdb.RacClusterDetailSpec{
						BaseLsnrTargetPort: 31522,
					},
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldUpdateClusterListenerEndpoints(tc.rac); got != tc.want {
				t.Fatalf("shouldUpdateClusterListenerEndpoints() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeRacInstancesFromLatest_ClearsReleasedOperationLock(t *testing.T) {
	t.Parallel()

	current := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeDeleteNodes,
				Holder:           "rac/racdbprov-sample",
				TargetGeneration: 3,
			},
		},
	}
	latest := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			Operation: nil,
		},
	}

	if err := mergeRacInstancesFromLatest(current, latest); err != nil {
		t.Fatalf("mergeRacInstancesFromLatest returned error: %v", err)
	}
	if current.Status.Operation != nil {
		t.Fatalf("expected released operation lock to stay cleared, got %#v", current.Status.Operation)
	}
}

func TestMergeRacInstancesFromLatest_ReplacesOperationLockFromLatest(t *testing.T) {
	t.Parallel()

	current := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeDeleteNodes,
				Holder:           "rac/racdbprov-sample",
				TargetGeneration: 3,
			},
		},
	}
	latest := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeAddNodes,
				Holder:           "rac/racdbprov-sample",
				TargetGeneration: 4,
			},
		},
	}

	if err := mergeRacInstancesFromLatest(current, latest); err != nil {
		t.Fatalf("mergeRacInstancesFromLatest returned error: %v", err)
	}
	if current.Status.Operation == nil {
		t.Fatal("expected operation lock copied from latest")
	}
	if current.Status.Operation.Type != racOpTypeAddNodes || current.Status.Operation.TargetGeneration != 4 {
		t.Fatalf("expected latest operation lock to win, got %#v", current.Status.Operation)
	}
}

func TestAcquireRACOperationLock_ReplacesStaleGenerationLock(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "racdbprov-sample", Namespace: "rac"}}
	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       req.Name,
			Namespace:  req.Namespace,
			Generation: 8,
		},
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeDeleteNodes,
				Holder:           req.NamespacedName.String(),
				Phase:            string(racPhaseDeletionAndIntent),
				TargetGeneration: 7,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance.DeepCopy()).WithStatusSubresource(instance.DeepCopy()).Build(),
	}

	if err := r.acquireRACOperationLock(context.Background(), req, instance.DeepCopy(), nil, racOpTypeAddNodes, string(racPhaseDeletionAndIntent)); err != nil {
		t.Fatalf("acquireRACOperationLock returned error: %v", err)
	}

	latest := &racdb.RacDatabase{}
	if err := r.Client.Get(context.Background(), req.NamespacedName, latest); err != nil {
		t.Fatalf("failed to get updated RAC object: %v", err)
	}
	if latest.Status.Operation == nil {
		t.Fatal("expected operation lock to be present")
	}
	if latest.Status.Operation.Type != racOpTypeAddNodes {
		t.Fatalf("expected operation type %q, got %#v", racOpTypeAddNodes, latest.Status.Operation)
	}
	if latest.Status.Operation.TargetGeneration != 8 {
		t.Fatalf("expected target generation 8, got %#v", latest.Status.Operation)
	}
}

func TestAcquireRACOperationLock_BlocksConflictingCurrentGenerationLock(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "racdbprov-sample", Namespace: "rac"}}
	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       req.Name,
			Namespace:  req.Namespace,
			Generation: 8,
		},
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeDeleteNodes,
				Holder:           req.NamespacedName.String(),
				Phase:            string(racPhaseDeletionAndIntent),
				TargetGeneration: 8,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance.DeepCopy()).WithStatusSubresource(instance.DeepCopy()).Build(),
	}

	err := r.acquireRACOperationLock(context.Background(), req, instance.DeepCopy(), nil, racOpTypeAddNodes, string(racPhaseDeletionAndIntent))
	if err == nil {
		t.Fatal("expected conflicting current-generation lock to block acquisition")
	}
	if !strings.Contains(err.Error(), racOpTypeDeleteNodes) {
		t.Fatalf("expected conflict error to mention held operation, got %v", err)
	}
}

func TestMergeClusterNodeStatusFromWorkingCopy_PreservesLatestOperation(t *testing.T) {
	t.Parallel()

	latest := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			ConfigParams: &racdb.RacInitParams{
				GridHome: "NOT_DEFINED",
				DbHome:   "NOT_DEFINED",
			},
			ServiceDetails: racdb.RacServiceSpec{
				Name:     "soepdb",
				SvcState: "service soepdb is running on instance(s) porclcdb1",
			},
		},
	}
	working := &racdb.RacDatabase{
		Status: racdb.RacDatabaseStatus{
			Operation: &racdb.RacOperationStatus{
				Type:             racOpTypeDeleteNodes,
				Holder:           "rac/racdbprov-sample",
				TargetGeneration: 11,
			},
			ConfigParams: &racdb.RacInitParams{
				GridHome: "/u01/app/19c/grid",
				DbHome:   "/u01/app/oracle/product/19c/dbhome_1",
			},
			RacNodes: []*racdb.RacNodeStatus{
				{Name: "racnode1-0"},
			},
			ServiceDetails: racdb.RacServiceSpec{
				Name:     "soepdb",
				SvcState: "service soepdb is running on instance(s) porclcdb1,porclcdb2",
			},
		},
	}

	mergeClusterNodeStatusFromWorkingCopy(latest, working)

	if latest.Status.Operation != nil {
		t.Fatalf("expected latest operation lock to remain cleared, got %#v", latest.Status.Operation)
	}
	if latest.Status.ConfigParams == nil ||
		latest.Status.ConfigParams.GridHome != "/u01/app/19c/grid" ||
		latest.Status.ConfigParams.DbHome != "/u01/app/oracle/product/19c/dbhome_1" {
		t.Fatalf("expected working config params copied onto latest status, got %#v", latest.Status.ConfigParams)
	}
	if latest.Status.ServiceDetails.SvcState != "service soepdb is running on instance(s) porclcdb1" {
		t.Fatalf("expected latest service state preserved, got %#v", latest.Status.ServiceDetails)
	}
	if len(latest.Status.RacNodes) != 1 || latest.Status.RacNodes[0] == nil || latest.Status.RacNodes[0].Name != "racnode1-0" {
		t.Fatalf("expected working RAC nodes copied onto latest status, got %#v", latest.Status.RacNodes)
	}
}

func TestSetCurrentSpecAndObservedGenerationPreservesWorkingStatus(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	stored := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "racdbprov-sample",
			Namespace:  "rac",
			Generation: 3,
		},
		Spec: racdb.RacDatabaseSpec{
			ConfigParams: &racdb.RacInitParams{
				GridHome: "/u01/app/19c/grid",
				DbHome:   "/u01/app/oracle/product/19c/dbhome_1",
			},
		},
		Status: racdb.RacDatabaseStatus{
			ConfigParams: &racdb.RacInitParams{
				GridHome: "NOT_DEFINED",
				DbHome:   "NOT_DEFINED",
			},
			ServiceDetails: racdb.RacServiceSpec{
				Name:     "soepdb",
				SvcState: "NOTAVAILABLE",
			},
		},
	}

	working := stored.DeepCopy()
	working.Status.ConfigParams = &racdb.RacInitParams{
		GridHome: "/u01/app/19c/grid",
		DbHome:   "/u01/app/oracle/product/19c/dbhome_1",
	}
	working.Status.ServiceDetails = racdb.RacServiceSpec{
		Name:     "soepdb",
		SvcState: "service soepdb is running on instance(s) porclcdb1",
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(stored.DeepCopy()).
			WithStatusSubresource(stored.DeepCopy()).
			Build(),
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace}}
	if err := r.SetCurrentSpecAndObservedGeneration(context.Background(), working, req); err != nil {
		t.Fatalf("SetCurrentSpecAndObservedGeneration returned error: %v", err)
	}

	if working.Status.ConfigParams == nil ||
		working.Status.ConfigParams.GridHome != "/u01/app/19c/grid" ||
		working.Status.ConfigParams.DbHome != "/u01/app/oracle/product/19c/dbhome_1" {
		t.Fatalf("expected working status config params to be preserved, got %#v", working.Status.ConfigParams)
	}
	if working.Status.ServiceDetails.SvcState != "service soepdb is running on instance(s) porclcdb1" {
		t.Fatalf("expected working service state to be preserved, got %#v", working.Status.ServiceDetails)
	}

	latest := &racdb.RacDatabase{}
	if err := r.Client.Get(context.Background(), req.NamespacedName, latest); err != nil {
		t.Fatalf("failed to get latest RAC object: %v", err)
	}
	if latest.Annotations[oldRacSpecAnnotation] == "" {
		t.Fatalf("expected current spec annotation to be persisted")
	}
	if latest.Status.ObservedGeneration != latest.Generation {
		t.Fatalf("expected observedGeneration=%d, got %d", latest.Generation, latest.Status.ObservedGeneration)
	}
}

func TestDeleteRACInstDeletesClusterSoftwarePVCOnScaleDown(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdbprov-sample",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			SwStorageClass:       "rook-ceph-block",
			SwLocStorageSizeInGb: 300,
			ConfigParams: &racdb.RacInitParams{
				GridHome: "/u01/app/19c/grid",
			},
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
			},
		},
		Status: racdb.RacDatabaseStatus{
			RacNodes: []*racdb.RacNodeStatus{{
				Name: "racnode1-0",
				NodeDetails: &racdb.RacNodeDetailedStatus{
					ClusterState: "HEALTHY",
				},
			}},
		},
	}

	swPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "racnode2-oradata-sw-pvc-racnode2-0",
			Namespace:  "rac",
			Finalizers: []string{"kubernetes.io/pvc-protection"},
		},
	}

	r := &RacDatabaseReconciler{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac.DeepCopy(), swPVC.DeepCopy()).Build(),
		kubeClient: kubefake.NewSimpleClientset(),
		kubeConfig: clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, &clientcmd.ConfigOverrides{}),
	}

	if err := r.deleteRACInst("racnode2", ctrl.Request{NamespacedName: types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}}, context.Background(), rac.DeepCopy()); err != nil {
		t.Fatalf("deleteRACInst returned error: %v", err)
	}

	found := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: swPVC.Name, Namespace: swPVC.Namespace}, found)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected software PVC to be deleted, got err=%v pvc=%#v", err, found)
	}
}

func TestDeleteRACInstKeepsClusterSoftwarePVCWhenRequested(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdbprov-sample",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			SwStorageClass:       "rook-ceph-block",
			SwLocStorageSizeInGb: 300,
			IsKeep:               true,
			ConfigParams: &racdb.RacInitParams{
				GridHome: "/u01/app/19c/grid",
			},
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
			},
		},
		Status: racdb.RacDatabaseStatus{
			RacNodes: []*racdb.RacNodeStatus{{
				Name: "racnode1-0",
				NodeDetails: &racdb.RacNodeDetailedStatus{
					ClusterState: "HEALTHY",
				},
			}},
		},
	}

	swPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "racnode2-oradata-sw-pvc-racnode2-0",
			Namespace:  "rac",
			Finalizers: []string{"kubernetes.io/pvc-protection"},
		},
	}

	r := &RacDatabaseReconciler{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac.DeepCopy(), swPVC.DeepCopy()).Build(),
		kubeClient: kubefake.NewSimpleClientset(),
		kubeConfig: clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, &clientcmd.ConfigOverrides{}),
	}

	if err := r.deleteRACInst("racnode2", ctrl.Request{NamespacedName: types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}}, context.Background(), rac.DeepCopy()); err != nil {
		t.Fatalf("deleteRACInst returned error: %v", err)
	}

	found := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: swPVC.Name, Namespace: swPVC.Namespace}, found); err != nil {
		t.Fatalf("expected software PVC to be retained, got err=%v", err)
	}
}

func TestCleanupRacDatabaseKeepsAsmPVCWhenRequested(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racdbprov-sample",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   0,
				RacNodeName: "racnode",
			},
			ConfigParams: &racdb.RacInitParams{},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{{
				Name:         "DATA",
				StorageClass: "rook-ceph-block",
				Disks:        []string{"/dev/asm-disk1"},
				IsKeep:       true,
			}},
			ScanSvcName: "racnode-scan",
		},
	}

	asmPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      raccommon.GetAsmPvcName("/dev/asm-disk1", rac.Name),
			Namespace: rac.Namespace,
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac.DeepCopy(), asmPVC.DeepCopy()).Build(),
	}

	if err := r.cleanupRacDatabase(ctrl.Request{NamespacedName: types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}}, context.Background(), rac.DeepCopy()); err != nil {
		t.Fatalf("cleanupRacDatabase returned error: %v", err)
	}

	found := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: asmPVC.Name, Namespace: asmPVC.Namespace}, found); err != nil {
		t.Fatalf("expected ASM PVC to be retained, got err=%v", err)
	}
}

func TestEnsureStatefulSetUpdated_ReplacesOnVolumeClaimTemplateChange(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "rac",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm1", DevicePath: "/dev/asm1"},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "sw1"},
			}},
		},
	}
	desired := existing.DeepCopy()
	desired.Spec.VolumeClaimTemplates = append(
		desired.Spec.VolumeClaimTemplates,
		corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "sw2"}},
	)

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 1,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(rac).
			WithRuntimeObjects(existing, rac).
			Build(),
	}

	result, err := r.ensureStatefulSetUpdated(context.Background(), r.Log, rac, desired, ctrl.Request{})
	if err != nil {
		t.Fatalf("ensureStatefulSetUpdated returned error: %v", err)
	}
	if result.RequeueAfter == 0 && !result.Requeue {
		t.Fatalf("expected requeue after StatefulSet replacement trigger")
	}

	got := &appsv1.StatefulSet{}
	err = r.Get(context.Background(), types.NamespacedName{Name: existing.Name, Namespace: existing.Namespace}, got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected StatefulSet to be deleted for replacement, got err=%v", err)
	}
	updatedRac := &racdb.RacDatabase{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}, updatedRac); err != nil {
		t.Fatalf("failed to fetch updated RAC object: %v", err)
	}
	if got := pendingStatefulSetReplacementName(updatedRac); got != desired.Name {
		t.Fatalf("expected pending StatefulSet replacement to target %q, got %q", desired.Name, got)
	}
}

func TestCreateOrReplaceAsmPvCExpandsExistingStorageClassBackedPVC(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add storagev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	allowExpansion := true
	instance := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{NodeCount: 1},
			ConfigParams:   &racdb.RacInitParams{},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               racdb.CrsAsmDiskDg,
					Disks:              []string{"/dev/asm-disk1"},
					StorageClass:       "rook-ceph-block",
					AccessMode:         string(corev1.ReadWriteOnce),
					AsmStorageSizeInGb: 100,
				},
			},
		},
	}
	existingPVC := raccommon.VolumePVCForASM(instance, 0, 0, "/dev/asm-disk1", "DATA", "50Gi")
	storageClass := &storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "rook-ceph-block"},
		AllowVolumeExpansion: &allowExpansion,
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(instance.DeepCopy(), existingPVC.DeepCopy(), storageClass).
			Build(),
	}

	desiredPVC := raccommon.VolumePVCForASM(instance, 0, 0, "/dev/asm-disk1", "DATA", "100Gi")
	if _, _, err := r.createOrReplaceAsmPvC(context.Background(), instance, desiredPVC, string(racdb.CrsAsmDiskDg)); err != nil {
		t.Fatalf("createOrReplaceAsmPvC returned error: %v", err)
	}

	got := &corev1.PersistentVolumeClaim{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: existingPVC.Name, Namespace: existingPVC.Namespace}, got); err != nil {
		t.Fatalf("failed to fetch updated pvc: %v", err)
	}
	gotSize := got.Spec.Resources.Requests[corev1.ResourceStorage]
	if gotSize.Cmp(resource.MustParse("100Gi")) != 0 {
		t.Fatalf("expected pvc size to expand to 100Gi, got %s", gotSize.String())
	}
}

func TestEnsureStatefulSetUpdated_UpdatesInPlaceOnVolumeDeviceOnlyChange(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "rac",
			Labels:    map[string]string{"app": "old"},
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"hash": "old"},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "asm1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "asm1",
								},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm1", DevicePath: "/dev/asm1"},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "asm1"},
			}},
		},
	}
	desired := existing.DeepCopy()
	desired.Labels = map[string]string{"app": "new"}
	desired.Spec.Template.Annotations = map[string]string{"hash": "new"}
	desired.Spec.Template.Spec.Volumes = append(
		desired.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "asm2",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "asm2",
				},
			},
		},
	)
	desired.Spec.Template.Spec.Containers[0].VolumeDevices = append(
		desired.Spec.Template.Spec.Containers[0].VolumeDevices,
		corev1.VolumeDevice{Name: "asm2", DevicePath: "/dev/asm2"},
	)

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 1,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing, rac).Build(),
	}

	result, err := r.ensureStatefulSetUpdated(context.Background(), r.Log, rac, desired, ctrl.Request{})
	if err != nil {
		t.Fatalf("ensureStatefulSetUpdated returned error: %v", err)
	}
	if !result.Requeue && result.RequeueAfter == 0 {
		t.Fatalf("expected in-place update to requeue for StatefulSet rollout, got %+v", result)
	}

	got := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: existing.Name, Namespace: existing.Namespace}, got); err != nil {
		t.Fatalf("failed to get updated StatefulSet: %v", err)
	}
	if len(got.Spec.Template.Spec.Containers[0].VolumeDevices) != 2 {
		t.Fatalf("expected updated StatefulSet to have 2 volume devices, got %d", len(got.Spec.Template.Spec.Containers[0].VolumeDevices))
	}
	if len(got.Spec.Template.Spec.Volumes) != 2 {
		t.Fatalf("expected updated StatefulSet to have 2 pod volumes, got %d", len(got.Spec.Template.Spec.Volumes))
	}
	if !reflect.DeepEqual(got.Spec.VolumeClaimTemplates, existing.Spec.VolumeClaimTemplates) {
		t.Fatalf("expected volumeClaimTemplates to remain unchanged")
	}
}

func TestEnsureStatefulSetUpdated_DoesNotReplaceOnDefaultedClaimTemplateFields(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	volumeMode := corev1.PersistentVolumeFilesystem
	storageClassName := "oci-bv-ext4"
	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "rac",
			Labels:    map[string]string{"app": "old"},
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"hash": "old"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "racnode1-0",
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "sw1"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: &storageClassName,
					VolumeMode:       &volumeMode,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("300Gi"),
						},
					},
				},
			}},
		},
	}
	desired := existing.DeepCopy()
	desired.Spec.VolumeClaimTemplates[0].Spec.VolumeMode = nil

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 1,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing, rac).Build(),
	}

	result, err := r.ensureStatefulSetUpdated(context.Background(), r.Log, rac, desired, ctrl.Request{})
	if err != nil {
		t.Fatalf("ensureStatefulSetUpdated returned error: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Fatalf("expected no replacement for defaulted claim template fields, got %+v", result)
	}

	got := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: existing.Name, Namespace: existing.Namespace}, got); err != nil {
		t.Fatalf("failed to fetch StatefulSet: %v", err)
	}
	if got.DeletionTimestamp != nil {
		t.Fatalf("expected StatefulSet to remain intact, found deletion timestamp")
	}
}

func TestEqualRACPodStorageWiring_IgnoresVolumeOrdering(t *testing.T) {
	t.Parallel()

	existing := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "asm-pvc-2",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-2"},
							},
						},
						{
							Name: "asm-pvc-1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm-pvc-2", DevicePath: "/dev/asm-disk2"},
							{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
						},
					}},
				},
			},
		},
	}

	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "asm-pvc-1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
							},
						},
						{
							Name: "asm-pvc-2",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-2"},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
							{Name: "asm-pvc-2", DevicePath: "/dev/asm-disk2"},
						},
					}},
				},
			},
		},
	}

	if !equalRACPodStorageWiring(existing, desired) {
		t.Fatalf("expected storage wiring comparison to ignore volume and volumeDevice ordering")
	}
}

func TestEnsureStatefulSetUpdated_DoesNotUpdateOnVolumeOrderingOnly(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "rac",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "asm-pvc-2",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-2"},
							},
						},
						{
							Name: "asm-pvc-1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm-pvc-2", DevicePath: "/dev/asm-disk2"},
							{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "sw1"},
			}},
		},
	}

	desired := existing.DeepCopy()
	desired.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "asm-pvc-1",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
			},
		},
		{
			Name: "asm-pvc-2",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-2"},
			},
		},
	}
	desired.Spec.Template.Spec.Containers[0].VolumeDevices = []corev1.VolumeDevice{
		{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
		{Name: "asm-pvc-2", DevicePath: "/dev/asm-disk2"},
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount: 1,
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing, rac).Build(),
	}

	result, err := r.ensureStatefulSetUpdated(context.Background(), r.Log, rac, desired, ctrl.Request{})
	if err != nil {
		t.Fatalf("ensureStatefulSetUpdated returned error: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Fatalf("expected no update when only volume ordering differs, got %+v", result)
	}
}

func TestEqualRACPodVolumes_IgnoresConfigMapDefaults(t *testing.T) {
	t.Parallel()

	defaultMode := int32(420)
	existing := []corev1.Volume{
		{
			Name: "racnode1-oradata-envfile",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "racnode1racdbprov-sample-cmap"},
					DefaultMode:          &defaultMode,
				},
			},
		},
	}

	desired := []corev1.Volume{
		{
			Name: "racnode1-oradata-envfile",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "racnode1racdbprov-sample-cmap"},
				},
			},
		},
	}

	if !equalRACPodVolumes(existing, desired) {
		t.Fatalf("expected pod volume comparison to ignore ConfigMap defaulted fields")
	}
}

func TestSyncRACPodTemplateScopedFields_IgnoresNonASMVolumeDefaults(t *testing.T) {
	t.Parallel()

	defaultMode := int32(420)
	found := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "racnode1-oradata-envfile",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "racnode1racdbprov-sample-cmap"},
							DefaultMode:          &defaultMode,
						},
					},
				},
				{
					Name: "asm-pvc-1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
					},
				},
			},
			Containers: []corev1.Container{{
				Name: "racnode1-0",
				VolumeDevices: []corev1.VolumeDevice{
					{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
				},
			}},
		},
	}
	desired := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "racnode1-oradata-envfile",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "racnode1racdbprov-sample-cmap"},
						},
					},
				},
				{
					Name: "asm-pvc-1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
					},
				},
			},
			Containers: []corev1.Container{{
				Name: "racnode1-0",
				VolumeDevices: []corev1.VolumeDevice{
					{Name: "asm-pvc-1", DevicePath: "/dev/asm-disk1"},
				},
			}},
		},
	}

	if syncRACPodTemplateScopedFields(found, desired) {
		t.Fatalf("expected pod template sync to ignore non-ASM volume defaults when ASM PVC wiring is unchanged")
	}
}

func TestCanSkipStatefulSetReplacementForAsmDiskUpdate_WhenMountsAlreadyConverged(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	storageClassName := "oci-bv-ext4"
	volumeMode := corev1.PersistentVolumeFilesystem
	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "rac",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "asm1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-1"},
							},
						},
						{
							Name: "asm2",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-2"},
							},
						},
						{
							Name: "asm3",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "asm-pvc-3"},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "racnode1-0",
						VolumeDevices: []corev1.VolumeDevice{
							{Name: "asm1", DevicePath: "/dev/asm-disk1"},
							{Name: "asm2", DevicePath: "/dev/asm-disk2"},
							{Name: "asm3", DevicePath: "/dev/asm-disk3"},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "racnode1-oradata-sw-pvc"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: &storageClassName,
					VolumeMode:       &volumeMode,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("300Gi"),
						},
					},
				},
			}},
		},
	}
	desired := existing.DeepCopy()

	oldSpec := racdb.RacDatabaseSpec{
		Image:                "example.com/rac:19c",
		SwStorageClass:       "oci-bv-ext4",
		SwLocStorageSizeInGb: 300,
		ClusterDetails: &racdb.RacClusterDetailSpec{
			NodeCount:   1,
			RacNodeName: "racnode",
			PrivateIPDetails: []racdb.PrivIpDetailSpec{
				{Name: "macvlan-conf1", Interface: "ens1"},
				{Name: "macvlan-conf2", Interface: "ens2"},
			},
		},
		ConfigParams: &racdb.RacInitParams{SwMountLocation: "/u01"},
		AsmStorageDetails: []racdb.AsmDiskGroupDetails{
			{
				Name:         "DATA",
				Type:         racdb.CrsAsmDiskDg,
				Disks:        []string{"/dev/asm-disk1", "/dev/asm-disk2"},
				StorageClass: "oci-bv",
			},
		},
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			Image:                "example.com/rac:19c",
			SwStorageClass:       "oci-bv-ext4",
			SwLocStorageSizeInGb: 300,
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
				PrivateIPDetails: []racdb.PrivIpDetailSpec{
					{Name: "macvlan-conf1", Interface: "ens1"},
					{Name: "macvlan-conf2", Interface: "ens2"},
				},
			},
			ConfigParams: &racdb.RacInitParams{SwMountLocation: "/u01"},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:         "DATA",
					Type:         racdb.CrsAsmDiskDg,
					Disks:        []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
					StorageClass: "oci-bv",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			RacNodes: []*racdb.RacNodeStatus{
				{
					Name: "racnode1-0",
					NodeDetails: &racdb.RacNodeDetailedStatus{
						MountedDevices: []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
					},
				},
			},
		},
	}

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing, rac).Build(),
	}

	if !r.canSkipStatefulSetReplacementForAsmDiskUpdate(rac, existing, desired, 0, &oldSpec) {
		t.Fatalf("expected ASM disk update to skip StatefulSet replacement when mounts already converged")
	}
}

func TestPendingStatefulSetReplacementConditionHelpers(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
	}
	setPendingStatefulSetReplacementCondition(rac, "racnode1")
	if got := pendingStatefulSetReplacementName(rac); got != "racnode1" {
		t.Fatalf("expected pending StatefulSet replacement name %q, got %q", "racnode1", got)
	}
	clearPendingStatefulSetReplacementCondition(rac)
	if got := pendingStatefulSetReplacementName(rac); got != "" {
		t.Fatalf("expected pending StatefulSet replacement to be cleared, got %q", got)
	}
}

func TestPendingAsmDiskStatusSyncConditionHelpers(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
	}

	setPendingAsmDiskStatusSyncCondition(rac)
	if !hasPendingAsmDiskStatusSyncCondition(rac) {
		t.Fatalf("expected pending ASM disk status sync condition to be set")
	}
	if cond := meta.FindStatusCondition(rac.Status.Conditions, racAsmDiskStatusSyncPendingConditionType); cond == nil || cond.Message != "" {
		t.Fatalf("expected pending ASM disk status sync condition without message, got %#v", cond)
	}

	setPendingAsmDiskStatusSyncConditionWithMessage(rac, "ASM change blocked")
	if cond := meta.FindStatusCondition(rac.Status.Conditions, racAsmDiskStatusSyncPendingConditionType); cond == nil || cond.Message != "ASM change blocked" {
		t.Fatalf("expected pending ASM disk status sync message to be updated, got %#v", cond)
	}

	clearPendingAsmDiskStatusSyncCondition(rac)
	if hasPendingAsmDiskStatusSyncCondition(rac) {
		t.Fatalf("expected pending ASM disk status sync condition to be cleared")
	}
}

func TestBlockedPendingAsmStorageChangeMessage(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:  "DATA",
					Type:  racdb.CrsAsmDiskDg,
					Disks: []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			AsmDiskGroups: []racdb.AsmDiskGroupStatus{
				{
					Name:       "NOT READY",
					Redundancy: "Pending",
					Disks: []racdb.AsmDiskStatus{
						{Name: "Pending", Valid: true},
					},
				},
			},
			RacNodes: []*racdb.RacNodeStatus{
				{
					Name: "racnode1-0",
					NodeDetails: &racdb.RacNodeDetailedStatus{
						MountedDevices: []string{"/dev/asm-disk1", "/dev/asm-disk2"},
					},
				},
			},
		},
	}
	setPendingAsmDiskStatusSyncCondition(rac)

	msg := blockedPendingAsmStorageChangeMessage(rac, []string{"/dev/asm-disk3"}, nil)
	if msg == "" {
		t.Fatalf("expected blocked pending ASM storage change message")
	}
	if !strings.Contains(msg, "blocked and have not been applied") {
		t.Fatalf("expected blocked-change explanation, got %q", msg)
	}
	if !strings.Contains(msg, "/dev/asm-disk3") {
		t.Fatalf("expected requested disk to be listed, got %q", msg)
	}
}

func TestAsmDiskStatusSyncConverged(t *testing.T) {
	t.Parallel()

	converged := &racdb.RacDatabase{
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:         "DATA",
					Type:         racdb.CrsAsmDiskDg,
					Disks:        []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
					StorageClass: "oci-bv",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			AsmDiskGroups: []racdb.AsmDiskGroupStatus{
				{
					Name:         "CRSDG",
					Type:         racdb.CrsAsmDiskDg,
					StorageClass: "oci-bv",
					Disks: []racdb.AsmDiskStatus{
						{Name: "/dev/asm-disk1", Valid: true},
						{Name: "/dev/asm-disk2", Valid: true},
						{Name: "/dev/asm-disk3", Valid: true},
					},
				},
			},
			RacNodes: []*racdb.RacNodeStatus{
				{
					Name: "racnode1-0",
					NodeDetails: &racdb.RacNodeDetailedStatus{
						MountedDevices: []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
					},
				},
			},
		},
	}
	if !asmDiskStatusSyncConverged(converged) {
		t.Fatalf("expected ASM disk status sync to be converged")
	}

	notConverged := converged.DeepCopy()
	notConverged.Status.AsmDiskGroups[0].Disks = notConverged.Status.AsmDiskGroups[0].Disks[:2]
	if asmDiskStatusSyncConverged(notConverged) {
		t.Fatalf("expected ASM disk status sync to remain pending when status still misses a disk")
	}
}

func TestBootstrapPendingStatefulSetReplacement(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				RacNodeName: "racnode",
				NodeCount:   1,
			},
		},
		Status: racdb.RacDatabaseStatus{
			State:              string(racdb.RACProvisionState),
			ObservedGeneration: 1,
		},
	}
	rac.Generation = 2

	r := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(rac).
			WithRuntimeObjects(rac).
			Build(),
	}

	bootstrapped, err := r.bootstrapPendingStatefulSetReplacement(
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}},
		rac,
	)
	if err != nil {
		t.Fatalf("bootstrapPendingStatefulSetReplacement returned error: %v", err)
	}
	if !bootstrapped {
		t.Fatalf("expected bootstrapPendingStatefulSetReplacement to seed pending replacement state")
	}
	if got := pendingStatefulSetReplacementName(rac); got != "racnode1" {
		t.Fatalf("expected local pending replacement name %q, got %q", "racnode1", got)
	}
	updated := &racdb.RacDatabase{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: rac.Name, Namespace: rac.Namespace}, updated); err != nil {
		t.Fatalf("failed to fetch updated RAC object: %v", err)
	}
	if got := pendingStatefulSetReplacementName(updated); got != "racnode1" {
		t.Fatalf("expected persisted pending replacement name %q, got %q", "racnode1", got)
	}
}

func TestCreateDaemonSet_SkipsWhenAsmUsesStorageClass(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "rac",
		},
		Spec: racdb.RacDatabaseSpec{
			Image: "example.com/rac:19c",
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               racdb.DbDataDiskDg,
					Disks:              []string{"data1", "data2"},
					StorageClass:       "oci-bv",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	reconciler := &RacDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac).Build(),
		Scheme: scheme,
	}

	if err := reconciler.createDaemonSet(rac, context.Background()); err != nil {
		t.Fatalf("createDaemonSet returned error: %v", err)
	}

	daemonSet := &appsv1.DaemonSet{}
	err := reconciler.Client.Get(context.Background(), types.NamespacedName{
		Name:      "disk-check-daemonset",
		Namespace: rac.Namespace,
	}, daemonSet)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected no daemonset to be created for storageClass-backed ASM, got err=%v", err)
	}
}

func TestGetDisksToAddStatusforRAC_SkipsAutoUpdateDisabledGroup(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:         "DATA",
					Type:         racdb.CrsAsmDiskDg,
					Disks:        []string{"/dev/asm-disk1", "/dev/asm-disk2", "/dev/asm-disk3"},
					AutoUpdate:   "false",
					StorageClass: "oci-bv",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			AsmDiskGroups: []racdb.AsmDiskGroupStatus{
				{
					Name:         "CRSDG",
					Type:         racdb.CrsAsmDiskDg,
					AutoUpdate:   "false",
					StorageClass: "oci-bv",
					Disks: []racdb.AsmDiskStatus{
						{Name: "/dev/asm-disk1", Valid: true},
						{Name: "/dev/asm-disk2", Valid: true},
					},
				},
			},
		},
	}

	disksToAdd, err := getDisksToAddStatusforRAC(rac)
	if err != nil {
		t.Fatalf("getDisksToAddStatusforRAC returned error: %v", err)
	}
	if len(disksToAdd) != 0 {
		t.Fatalf("expected no ASM disks to add when autoUpdate is disabled, got %v", disksToAdd)
	}
}

func TestGetDisksToRemoveStatusforRAC_SkipsAutoUpdateDisabledGroup(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:         "DATA",
					Type:         racdb.CrsAsmDiskDg,
					Disks:        []string{"/dev/asm-disk1"},
					AutoUpdate:   "false",
					StorageClass: "oci-bv",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			AsmDiskGroups: []racdb.AsmDiskGroupStatus{
				{
					Name:         "CRSDG",
					Type:         racdb.CrsAsmDiskDg,
					AutoUpdate:   "false",
					StorageClass: "oci-bv",
					Disks: []racdb.AsmDiskStatus{
						{Name: "/dev/asm-disk1", Valid: true},
						{Name: "/dev/asm-disk2", Valid: true},
					},
				},
			},
		},
	}

	disksToRemove, err := getDisksToRemoveStatusforRAC(rac)
	if err != nil {
		t.Fatalf("getDisksToRemoveStatusforRAC returned error: %v", err)
	}
	if len(disksToRemove) != 0 {
		t.Fatalf("expected no ASM disks to remove when autoUpdate is disabled, got %v", disksToRemove)
	}
}

func TestComputeDiskChangesFailsClosedWhenAsmProbeReturnsNoState(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
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
				NodeCount:   1,
				RacNodeName: "racnode",
			},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:       "DATA",
					Type:       racdb.CrsAsmDiskDg,
					Disks:      []string{"/dev/asm-disk1"},
					AutoUpdate: "true",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			State: string(racdb.RACAvailableState),
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "default",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1-0",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "StatefulSet",
				Name: "racnode1",
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "oracle"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	oldSpec := rac.Spec
	reconciler := &RacDatabaseReconciler{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac.DeepCopy(), sts, pod).Build(),
		kubeClient: kubefake.NewSimpleClientset(),
		kubeConfig: clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, &clientcmd.ConfigOverrides{}),
	}

	_, _, err := reconciler.computeDiskChanges(rac.DeepCopy(), &oldSpec)
	if err == nil {
		t.Fatalf("expected ASM probe failure to abort disk diff")
	}
	if !strings.Contains(err.Error(), "ASM runtime state is unavailable") {
		t.Fatalf("expected fail-closed ASM probe error, got %v", err)
	}
}

func TestComputeDiskChangesFailsClosedWhenRacSetupIsUnstableAndSpecChangesPresent(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := racdb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add racdb scheme: %v", err)
	}

	oldSpec := racdb.RacDatabaseSpec{
		ClusterDetails: &racdb.RacClusterDetailSpec{
			NodeCount:   1,
			RacNodeName: "racnode",
		},
		AsmStorageDetails: []racdb.AsmDiskGroupDetails{
			{
				Name:       "DATA",
				Type:       racdb.CrsAsmDiskDg,
				Disks:      []string{"/dev/asm-disk1"},
				AutoUpdate: "true",
			},
		},
	}

	rac := &racdb.RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrac",
			Namespace: "default",
		},
		Spec: racdb.RacDatabaseSpec{
			ClusterDetails: &racdb.RacClusterDetailSpec{
				NodeCount:   1,
				RacNodeName: "racnode",
			},
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:       "DATA",
					Type:       racdb.CrsAsmDiskDg,
					Disks:      []string{"/dev/asm-disk1", "/dev/asm-disk2"},
					AutoUpdate: "true",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			State: string(racdb.RACAvailableState),
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1",
			Namespace: "default",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "racnode1-0",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "StatefulSet",
				Name: "racnode1",
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "oracle"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}

	reconciler := &RacDatabaseReconciler{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rac.DeepCopy(), sts, pod).Build(),
		kubeClient: kubefake.NewSimpleClientset(),
		kubeConfig: clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, &clientcmd.ConfigOverrides{}),
	}

	_, _, err := reconciler.computeDiskChanges(rac.DeepCopy(), &oldSpec)
	if err == nil {
		t.Fatalf("expected unstable RAC setup to abort ASM storage changes")
	}
	if !strings.Contains(err.Error(), "RAC setup is not stable") {
		t.Fatalf("expected unstable RAC fail-closed error, got %v", err)
	}
}

func TestGetDisksToAddStatusforRAC_DefaultsAutoUpdateToEnabled(t *testing.T) {
	t.Parallel()

	rac := &racdb.RacDatabase{
		Spec: racdb.RacDatabaseSpec{
			AsmStorageDetails: []racdb.AsmDiskGroupDetails{
				{
					Name:         "DATA",
					Type:         racdb.CrsAsmDiskDg,
					Disks:        []string{"/dev/asm-disk1", "/dev/asm-disk2"},
					StorageClass: "oci-bv",
				},
			},
		},
		Status: racdb.RacDatabaseStatus{
			AsmDiskGroups: []racdb.AsmDiskGroupStatus{
				{
					Name:         "CRSDG",
					Type:         racdb.CrsAsmDiskDg,
					StorageClass: "oci-bv",
					Disks: []racdb.AsmDiskStatus{
						{Name: "/dev/asm-disk1", Valid: true},
					},
				},
			},
		},
	}

	disksToAdd, err := getDisksToAddStatusforRAC(rac)
	if err != nil {
		t.Fatalf("getDisksToAddStatusforRAC returned error: %v", err)
	}
	if !reflect.DeepEqual(disksToAdd, []string{"/dev/asm-disk2"}) {
		t.Fatalf("expected remaining ASM disk to add, got %v", disksToAdd)
	}
}

func TestIsRacDatabaseDeleting(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())

	tests := []struct {
		name   string
		input  *racdb.RacDatabase
		stored *racdb.RacDatabase
		want   bool
	}{
		{
			name: "input already marked for deletion",
			input: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "testrac",
					Namespace:         "rac",
					DeletionTimestamp: &now,
				},
			},
			stored: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testrac",
					Namespace:  "rac",
					Finalizers: []string{racDatabaseFinalizer},
				},
			},
			want: true,
		},
		{
			name: "live object marked for deletion",
			input: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testrac",
					Namespace: "rac",
				},
			},
			stored: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "testrac",
					Namespace:         "rac",
					DeletionTimestamp: &now,
					Finalizers:        []string{racDatabaseFinalizer},
				},
			},
			want: true,
		},
		{
			name: "object not deleting",
			input: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testrac",
					Namespace: "rac",
				},
			},
			stored: &racdb.RacDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testrac",
					Namespace: "rac",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := racdb.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to add racdb scheme: %v", err)
			}

			reconciler := &RacDatabaseReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.stored).Build(),
			}

			got, err := isRacDatabaseDeleting(context.Background(), reconciler, tt.input)
			if err != nil {
				t.Fatalf("isRacDatabaseDeleting returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("isRacDatabaseDeleting = %v, want %v", got, tt.want)
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
