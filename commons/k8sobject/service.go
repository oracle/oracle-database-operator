package k8sobjects

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodePortMergeStrategy controls how existing nodePort assignments are preserved.
type NodePortMergeStrategy string

const (
	// NodePortMergeNone does not preserve existing nodePort assignments.
	NodePortMergeNone NodePortMergeStrategy = "none"
	// NodePortMergeByName preserves nodePort values by matching service-port name.
	NodePortMergeByName NodePortMergeStrategy = "by_name"
	// NodePortMergeByNamePortAndProtocol preserves nodePort values by matching name+port+protocol.
	NodePortMergeByNamePortAndProtocol NodePortMergeStrategy = "by_name_port_protocol"
)

// ServiceSyncOptions controls which mutable service fields are synchronized.
type ServiceSyncOptions struct {
	NodePortMerge             NodePortMergeStrategy
	SyncOwnerReferences       bool
	SyncSessionAffinityCfg    bool
	SyncPublishNotReady       bool
	SyncInternalTrafficPolicy bool
	SyncLoadBalancerFields    bool
	SyncHealthCheckNodePort   bool
}

// EnsureService creates or updates the desired service and returns whether it changed.
func EnsureService(
	ctx context.Context,
	cl client.Client,
	namespace string,
	desired *corev1.Service,
	opts ServiceSyncOptions,
) (changed bool, err error) {
	found := &corev1.Service{}
	getErr := cl.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, found)
	if getErr != nil && apierrors.IsNotFound(getErr) {
		if err := cl.Create(ctx, desired); err != nil {
			return false, err
		}
		return true, nil
	}
	if getErr != nil {
		return false, getErr
	}

	if err := validateServiceOwnership(found, desired, opts); err != nil {
		return false, err
	}

	updated := reconcileServiceFields(found, desired, opts)
	if !updated {
		return false, nil
	}
	if err := cl.Update(ctx, found); err != nil {
		return false, err
	}
	return true, nil
}

func validateServiceOwnership(found, desired *corev1.Service, opts ServiceSyncOptions) error {
	if !opts.SyncOwnerReferences || len(desired.OwnerReferences) == 0 {
		return nil
	}

	desiredOwner := metav1.GetControllerOf(desired)
	if desiredOwner == nil {
		return nil
	}

	foundOwner := metav1.GetControllerOf(found)
	if foundOwner == nil {
		return fmt.Errorf("refusing to update service %s/%s: existing service is not controlled by %s %s/%s",
			found.Namespace, found.Name, desiredOwner.Kind, desiredOwner.APIVersion, desiredOwner.Name)
	}
	if foundOwner.UID != desiredOwner.UID ||
		foundOwner.Name != desiredOwner.Name ||
		foundOwner.Kind != desiredOwner.Kind ||
		foundOwner.APIVersion != desiredOwner.APIVersion {
		return fmt.Errorf("refusing to update service %s/%s: existing service is controlled by %s %s/%s, expected %s %s/%s",
			found.Namespace, found.Name,
			foundOwner.Kind, foundOwner.APIVersion, foundOwner.Name,
			desiredOwner.Kind, desiredOwner.APIVersion, desiredOwner.Name)
	}

	return nil
}

func reconcileServiceFields(found, desired *corev1.Service, opts ServiceSyncOptions) bool {
	updated := false

	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.Annotations, desired.Annotations) {
		found.Annotations = desired.Annotations
		updated = true
	}
	if opts.SyncOwnerReferences && !reflect.DeepEqual(found.OwnerReferences, desired.OwnerReferences) {
		found.OwnerReferences = desired.OwnerReferences
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Selector, desired.Spec.Selector) {
		found.Spec.Selector = desired.Spec.Selector
		updated = true
	}

	desiredPorts := desired.Spec.Ports
	switch opts.NodePortMerge {
	case NodePortMergeByName:
		desiredPorts = MergeServicePortsWithAssignedNodePortByName(found.Spec.Ports, desired.Spec.Ports)
	case NodePortMergeByNamePortAndProtocol:
		desiredPorts = MergeServicePortsWithAssignedNodePortsByNamePortProtocol(found.Spec.Ports, desired.Spec.Ports)
	}
	if !reflect.DeepEqual(found.Spec.Ports, desiredPorts) {
		found.Spec.Ports = desiredPorts
		updated = true
	}

	if found.Spec.Type != desired.Spec.Type {
		found.Spec.Type = desired.Spec.Type
		updated = true
	}
	if found.Spec.ExternalTrafficPolicy != desired.Spec.ExternalTrafficPolicy {
		found.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy
		updated = true
	}
	if found.Spec.SessionAffinity != desired.Spec.SessionAffinity {
		found.Spec.SessionAffinity = desired.Spec.SessionAffinity
		updated = true
	}
	if opts.SyncSessionAffinityCfg && !reflect.DeepEqual(found.Spec.SessionAffinityConfig, desired.Spec.SessionAffinityConfig) {
		found.Spec.SessionAffinityConfig = desired.Spec.SessionAffinityConfig
		updated = true
	}
	if opts.SyncPublishNotReady && found.Spec.PublishNotReadyAddresses != desired.Spec.PublishNotReadyAddresses {
		found.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
		updated = true
	}
	if opts.SyncInternalTrafficPolicy && !reflect.DeepEqual(found.Spec.InternalTrafficPolicy, desired.Spec.InternalTrafficPolicy) {
		found.Spec.InternalTrafficPolicy = desired.Spec.InternalTrafficPolicy
		updated = true
	}
	if opts.SyncLoadBalancerFields {
		if !reflect.DeepEqual(found.Spec.AllocateLoadBalancerNodePorts, desired.Spec.AllocateLoadBalancerNodePorts) {
			found.Spec.AllocateLoadBalancerNodePorts = desired.Spec.AllocateLoadBalancerNodePorts
			updated = true
		}
		if !reflect.DeepEqual(found.Spec.LoadBalancerClass, desired.Spec.LoadBalancerClass) {
			found.Spec.LoadBalancerClass = desired.Spec.LoadBalancerClass
			updated = true
		}
		if !reflect.DeepEqual(found.Spec.LoadBalancerSourceRanges, desired.Spec.LoadBalancerSourceRanges) {
			found.Spec.LoadBalancerSourceRanges = desired.Spec.LoadBalancerSourceRanges
			updated = true
		}
	}
	if opts.SyncHealthCheckNodePort && found.Spec.HealthCheckNodePort != desired.Spec.HealthCheckNodePort {
		found.Spec.HealthCheckNodePort = desired.Spec.HealthCheckNodePort
		updated = true
	}

	return updated
}
