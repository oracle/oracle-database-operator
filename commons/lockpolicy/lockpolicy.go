// Package lockpolicy provides helpers to enforce and override reconcile locks.
package lockpolicy

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultReconcilingConditionType is the condition type used for in-progress reconciliation.
	DefaultReconcilingConditionType = "Reconciling"
	// DefaultUpdateLockReason is the reason text indicating reconcile lock.
	DefaultUpdateLockReason = "UpdateInProgress"
	// DefaultOverrideAnnotation is the annotation key to bypass the update lock.
	DefaultOverrideAnnotation = "database.oracle.com/lock-override"
)

// FindStatusCondition returns the first condition matching condType, or nil.
func FindStatusCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// IsControllerUpdateLocked reports lock state, observed generation, and lock message.
func IsControllerUpdateLocked(conds []metav1.Condition, reconcilingType, updateLockReason string) (bool, int64, string) {
	cond := FindStatusCondition(conds, reconcilingType)
	if cond == nil {
		return false, 0, ""
	}
	if cond.Status != metav1.ConditionTrue {
		return false, 0, ""
	}
	if strings.TrimSpace(cond.Reason) != strings.TrimSpace(updateLockReason) {
		return false, 0, ""
	}
	return true, cond.ObservedGeneration, cond.Message
}

// IsUpdateLockOverrideEnabled returns true when override annotation is explicitly true.
func IsUpdateLockOverrideEnabled(annotations map[string]string, overrideAnnotation string) (bool, string) {
	if len(annotations) == 0 {
		return false, ""
	}
	if !strings.EqualFold(strings.TrimSpace(annotations[overrideAnnotation]), "true") {
		return false, ""
	}
	return true, "override accepted via manual lock-override annotation"
}
