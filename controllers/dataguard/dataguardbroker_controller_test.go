package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestDataguardBrokerEventHandlerTriggersOnRunnerPhaseChange(t *testing.T) {
	predicate := dataguardBrokerEventHandler()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "runner", Namespace: "dbns"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
	newPod := oldPod.DeepCopy()
	newPod.Status.Phase = corev1.PodRunning

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected reconcile on pod phase transition")
	}
}

func TestDataguardBrokerEventHandlerIgnoresMetadataOnlyPodUpdate(t *testing.T) {
	predicate := dataguardBrokerEventHandler()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "runner", Namespace: "dbns"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	newPod := oldPod.DeepCopy()
	newPod.Annotations = map[string]string{"note": "changed"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected metadata-only pod update to be ignored")
	}
}
