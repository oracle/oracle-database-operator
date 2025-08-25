package v4

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var dbcssystemlog = log.Log.WithName("dbcssystem-resource")

// DbcsSystemWebhook wraps a client for validations/defaults.
type DbcsSystemWebhook struct {
	client.Client
}

// Ensure our webhook struct satisfies the interfaces
var _ webhook.CustomValidator = &DbcsSystemWebhook{}
var _ webhook.CustomDefaulter = &DbcsSystemWebhook{}

// Register the webhook with the manager
func (r *DbcsSystem) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&DbcsSystem{}).
		WithDefaulter(&DbcsSystemWebhook{Client: mgr.GetClient()}).
		WithValidator(&DbcsSystemWebhook{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-database-oracle-com-v4-dbcssystem,mutating=true,failurePolicy=fail,sideEffects=none,groups=database.oracle.com,resources=dbcssystems,verbs=create;update,versions=v4,name=mdbcssystemv4.kb.io,admissionReviewVersions=v1

// Default implements webhook.CustomDefaulter
func (w *DbcsSystemWebhook) Default(ctx context.Context, obj runtime.Object) error {
	dbcssystemlog.Info("defaulting", "gvk", obj.GetObjectKind().GroupVersionKind().Kind)
	return nil
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-dbcssystem,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dbcssystems,versions=v4,name=vdbcssystemv4.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.CustomValidator
func (w *DbcsSystemWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate create")

	dbcs, ok := obj.(*DbcsSystem)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast object to DbcsSystem"))
	}

	// Block creation if resource is already in transitional states
	blockedStates := map[string]bool{
		"PROVISIONING": true,
		"UPDATING":     true,
		"TERMINATING":  true,
	}

	if blockedStates[string(dbcs.Status.State)] {
		return nil, apierrors.NewForbidden(
			schema.GroupResource{
				Group:    "database.oracle.com",
				Resource: "DbcsSystem",
			},
			dbcs.Name,
			fmt.Errorf("creation of DbcsSystem is not allowed while resource is in state %q", dbcs.Status.State),
		)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator
func (w *DbcsSystemWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate update")

	oldDbcs, ok1 := oldObj.(*DbcsSystem)
	newDbcs, ok2 := newObj.(*DbcsSystem)
	if !ok1 || !ok2 {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast objects to DbcsSystem"))
	}

	// Block spec updates in non-available states
	blockedStates := map[string]bool{
		"UPDATING":     true,
		"PROVISIONING": true,
		"TERMINATING":  true,
	}

	if blockedStates[string(newDbcs.Status.State)] {
		if !reflect.DeepEqual(oldDbcs.Spec, newDbcs.Spec) {
			return nil, apierrors.NewForbidden(
				schema.GroupResource{
					Group:    "database.oracle.com",
					Resource: "DbcsSystem",
				},
				newDbcs.Name,
				fmt.Errorf("updates to DbcsSystem Spec are not allowed while resource is in state %q", newDbcs.Status.State),
			)
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator
func (w *DbcsSystemWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate delete")
	// TODO: Add delete validation if needed
	return nil, nil
}
