package controllers

import (
	"context"
	"os"
	"strconv"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controller "sigs.k8s.io/controller-runtime/pkg/controller"
)

var maxReconcilesTTCV1 int = 2

// TimesTenClassicReconciler reconciles a TimesTenClassic object
type TimesTenClassicReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=timesten.oracle.com,resources=timestenclassics,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=timesten.oracle.com,resources=timestenclassics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=timesten.oracle.com,resources=timestenclassics/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=,resources=secrets,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=create;get;watch
// +kubebuilder:rbac:groups=,resources=secrets,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=,resources=services,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=,resources=events,verbs=create
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;delete
func (r *TimesTenClassicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res, err := ReconcileClassic(ctx, req, r.Client, r.Scheme)
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *TimesTenClassicReconciler) SetupWithManager(mgr ctrl.Manager) error {

	max, ok := os.LookupEnv("TT_MAX_RECONCILES")
	if ok {
		if val, err := strconv.Atoi(max); err == nil {
			maxReconcilesTTCV1 = val
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.TimesTenClassic{}).
		// Added by SAMDRAKE to cause the operator to watch
		// these object types, which the operator creates
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxReconcilesTTCV1}).
		Complete(r)
}
