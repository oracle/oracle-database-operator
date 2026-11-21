package connectionmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	k8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var connectionManagerRequeue = ctrl.Result{Requeue: true, RequeueAfter: 25 * time.Second}

const (
	connectionManagerStatusError       = "Error"
	connectionManagerStatusReady       = "Healthy"
	connectionManagerStatusMaintenance = "Maintenance"
)

// +kubebuilder:rbac:groups=network.oracle.com,resources=connectionmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.oracle.com,resources=connectionmanagers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

type ConnectionManagerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

func (r *ConnectionManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	inst := &networkv4.ConnectionManager{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return connectionManagerRequeue, err
	}

	configChecksum, err := r.resolveConnectionManagerConfigChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = connectionManagerStatusError
		_ = r.Status().Update(ctx, inst)
		return connectionManagerRequeue, err
	}
	secretChecksum, err := r.resolveConnectionManagerRestSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = connectionManagerStatusError
		_ = r.Status().Update(ctx, inst)
		return connectionManagerRequeue, err
	}
	if err := r.reconcileConnectionManagerStorage(ctx, inst); err != nil {
		inst.Status.Status = connectionManagerStatusError
		_ = r.Status().Update(ctx, inst)
		return connectionManagerRequeue, err
	}

	deploy := buildConnectionManagerDeployment(inst, configChecksum, secretChecksum)
	if err := controllerutil.SetControllerReference(inst, deploy, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundDeploy, depResult, err := k8sobjects.ReconcileDeployment(ctx, r.Client, inst.Namespace, deploy, syncConnectionManagerDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, inst, "internal"); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileService(ctx, inst, "external"); err != nil {
		return ctrl.Result{}, err
	}

	inst.Status.Status = connectionManagerStatusReady
	inst.Status.ReadyReplicas = foundDeploy.Status.ReadyReplicas
	if connectionManagerMaintenanceEnabled(inst) {
		inst.Status.Status = connectionManagerStatusMaintenance
		inst.Status.ReadyReplicas = 0
	}
	inst.Status.InternalService = ""
	inst.Status.ExternalService = ""
	inst.Status.ExternalEndpoint = ""
	if connectionManagerServiceEnabled(inst.Spec.Service.Internal.Enabled, true) {
		inst.Status.InternalService = connectionManagerInternalServiceName(inst)
	}
	if connectionManagerServiceEnabled(inst.Spec.Service.External.Enabled, false) {
		inst.Status.ExternalService = connectionManagerExternalServiceName(inst)
		existing := &corev1.Service{}
		if err := r.Get(ctx, types.NamespacedName{Name: inst.Status.ExternalService, Namespace: inst.Namespace}, existing); err == nil &&
			len(existing.Status.LoadBalancer.Ingress) > 0 {
			ingress := existing.Status.LoadBalancer.Ingress[0]
			if ingress.IP != "" {
				inst.Status.ExternalEndpoint = ingress.IP
			} else {
				inst.Status.ExternalEndpoint = ingress.Hostname
			}
			inst.Status.ExternalEndpoint = connectionManagerURLBase(inst.Status.ExternalEndpoint)
		}
	}
	inst.Status.Cman = &networkv4.ConnectionManagerStatusDetails{
		ConfigMode: connectionManagerConfigMode(inst),
		RestHost:   connectionManagerRestHost(inst),
	}
	if err := r.Status().Update(ctx, inst); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("connection manager status updated", "readyReplicas", inst.Status.ReadyReplicas)
	if depResult.Created || depResult.Updated {
		return connectionManagerRequeue, nil
	}
	return ctrl.Result{RequeueAfter: connectionManagerRequeue.RequeueAfter}, nil
}

func (r *ConnectionManagerReconciler) reconcileService(ctx context.Context, owner *networkv4.ConnectionManager, serviceKind string) error {
	enabled := connectionManagerServiceEnabled(owner.Spec.Service.Internal.Enabled, true)
	name := connectionManagerInternalServiceName(owner)
	if serviceKind == "external" {
		enabled = connectionManagerServiceEnabled(owner.Spec.Service.External.Enabled, false)
		name = connectionManagerExternalServiceName(owner)
	}
	if !enabled {
		return r.deleteService(ctx, owner, name)
	}
	svc := buildConnectionManagerService(owner, serviceKind)
	if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
		return err
	}
	_, err := k8sobjects.EnsureService(ctx, r.Client, owner.Namespace, svc, k8sobjects.ServiceSyncOptions{
		SyncOwnerReferences:    true,
		SyncLoadBalancerFields: true,
	})
	return err
}

func (r *ConnectionManagerReconciler) deleteService(ctx context.Context, owner *networkv4.ConnectionManager, name string) error {
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: owner.Namespace}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(svc, owner) {
		return apierrors.NewConflict(
			schema.GroupResource{Resource: "services"},
			svc.Name,
			fmt.Errorf("service is not controlled by ConnectionManager %s/%s", owner.Namespace, owner.Name),
		)
	}
	uid := svc.UID
	if err := r.Delete(ctx, svc, client.Preconditions{UID: &uid}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ConnectionManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv4.ConnectionManager{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&corev1.PersistentVolumeClaim{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			pvc, ok := obj.(*corev1.PersistentVolumeClaim)
			if !ok {
				return nil
			}
			names := connectionManagerPVCRequests(ctx, mgr.GetClient(), pvc)
			requests := make([]reconcile.Request, 0, len(names))
			for _, name := range names {
				requests = append(requests, reconcile.Request{NamespacedName: name})
			}
			return requests
		})).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			configMap, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return nil
			}
			list := &networkv4.ConnectionManagerList{}
			if err := mgr.GetClient().List(ctx, list, client.InNamespace(configMap.Namespace)); err != nil {
				return nil
			}
			requests := make([]reconcile.Request, 0)
			for i := range list.Items {
				item := &list.Items[i]
				if connectionManagerReferencesConfigMap(item, configMap.Name) {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace}})
				}
			}
			return requests
		})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return nil
			}
			list := &networkv4.ConnectionManagerList{}
			if err := mgr.GetClient().List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
				return nil
			}
			requests := make([]reconcile.Request, 0)
			for i := range list.Items {
				item := &list.Items[i]
				if connectionManagerReferencesSecret(item, secret.Name) {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace}})
				}
			}
			return requests
		})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}
