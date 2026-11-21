package controllers

import (
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
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

func TestDataguardBrokerEventHandlerTriggersOnRunnerReadinessChange(t *testing.T) {
	predicate := dataguardBrokerEventHandler()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "runner", Namespace: "dbns"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	newPod := oldPod.DeepCopy()
	newPod.Status.Conditions[0].Status = corev1.ConditionTrue

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected reconcile on pod readiness transition")
	}
}

func TestDataguardBrokerEventHandlerTriggersOnBrokerAnnotationUpdate(t *testing.T) {
	predicate := dataguardBrokerEventHandler()
	oldBroker := &dbapi.DataguardBroker{ObjectMeta: metav1.ObjectMeta{Name: "broker", Namespace: "dbns"}}
	newBroker := oldBroker.DeepCopy()
	newBroker.Annotations = map[string]string{"e2e.oracle.com/broker-wait": "1"}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldBroker, ObjectNew: newBroker}) {
		t.Fatalf("expected reconcile on Broker annotation update")
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
