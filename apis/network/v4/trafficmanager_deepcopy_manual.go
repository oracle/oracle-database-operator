package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func (in *TrafficManagerSpec) DeepCopyInto(out *TrafficManagerSpec) {
	*out = *in
	in.Runtime.DeepCopyInto(&out.Runtime)
	in.Service.DeepCopyInto(&out.Service)
	in.Security.DeepCopyInto(&out.Security)
	if in.Nginx != nil {
		in, out := &in.Nginx, &out.Nginx
		*out = new(NginxTrafficManagerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Cman != nil {
		in, out := &in.Cman, &out.Cman
		*out = new(CmanTrafficManagerSpec)
		(*in).DeepCopyInto(*out)
	}
}

func (in *TrafficManagerSecuritySpec) DeepCopyInto(out *TrafficManagerSecuritySpec) {
	*out = *in
	out.TLS = in.TLS
	if in.BackendTLS != nil {
		in, out := &in.BackendTLS, &out.BackendTLS
		*out = new(TrafficManagerBackendTLSSpec)
		**out = **in
		if (*in).Verify != nil {
			verify := new(bool)
			*verify = *(*in).Verify
			(*out).Verify = verify
		}
	}
}

func (in *TrafficManagerRuntimeSpec) DeepCopyInto(out *TrafficManagerRuntimeSpec) {
	*out = *in
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.PodSecurityContext != nil {
		in, out := &in.PodSecurityContext, &out.PodSecurityContext
		*out = new(corev1.PodSecurityContext)
		(*in).DeepCopyInto(*out)
	}
	if in.ContainerSecurityContext != nil {
		in, out := &in.ContainerSecurityContext, &out.ContainerSecurityContext
		*out = new(corev1.SecurityContext)
		(*in).DeepCopyInto(*out)
	}
	if in.EnvVars != nil {
		in, out := &in.EnvVars, &out.EnvVars
		*out = make([]TrafficManagerEnvVar, len(*in))
		copy(*out, *in)
	}
}

func (in *TrafficManagerServiceSpec) DeepCopyInto(out *TrafficManagerServiceSpec) {
	*out = *in
	in.Internal.DeepCopyInto(&out.Internal)
	in.External.DeepCopyInto(&out.External)
}

func (in *TrafficManagerServiceEndpointSpec) DeepCopyInto(out *TrafficManagerServiceEndpointSpec) {
	*out = *in
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = new(bool)
		**out = **in
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.LoadBalancerClass != nil {
		in, out := &in.LoadBalancerClass, &out.LoadBalancerClass
		*out = new(string)
		**out = **in
	}
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]TrafficManagerServicePortSpec, len(*in))
		copy(*out, *in)
	}
}

func (in *NginxTrafficManagerSpec) DeepCopyInto(out *NginxTrafficManagerSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = new(TrafficManagerConfigSpec)
		**out = **in
	}
}

func (in *CmanTrafficManagerSpec) DeepCopyInto(out *CmanTrafficManagerSpec) {
	*out = *in
	if in.ConfigSource != nil {
		in, out := &in.ConfigSource, &out.ConfigSource
		*out = new(CmanTrafficManagerConfigSourceSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Rules != nil {
		in, out := &in.Rules, &out.Rules
		*out = make([]CmanTrafficManagerRuleSpec, len(*in))
		copy(*out, *in)
	}
	if in.RestAPI != nil {
		in, out := &in.RestAPI, &out.RestAPI
		*out = new(CmanTrafficManagerRestAPISpec)
		(*in).DeepCopyInto(*out)
	}
}

func (in *CmanTrafficManagerConfigSourceSpec) DeepCopyInto(out *CmanTrafficManagerConfigSourceSpec) {
	*out = *in
	if in.ConfigMapRef != nil {
		in, out := &in.ConfigMapRef, &out.ConfigMapRef
		*out = new(TrafficManagerConfigMapKeyRef)
		**out = **in
	}
}

func (in *CmanTrafficManagerRestAPISpec) DeepCopyInto(out *CmanTrafficManagerRestAPISpec) {
	*out = *in
	if in.PasswordSecretRef != nil {
		in, out := &in.PasswordSecretRef, &out.PasswordSecretRef
		*out = new(TrafficManagerSecretKeyRef)
		**out = **in
	}
	if in.PrivateKeySecretRef != nil {
		in, out := &in.PrivateKeySecretRef, &out.PrivateKeySecretRef
		*out = new(TrafficManagerSecretKeyRef)
		**out = **in
	}
}

func (in *NginxTrafficManagerStatus) DeepCopyInto(out *NginxTrafficManagerStatus) {
	*out = *in
	if in.AssociatedBackends != nil {
		in, out := &in.AssociatedBackends, &out.AssociatedBackends
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Routes != nil {
		in, out := &in.Routes, &out.Routes
		*out = make([]NginxRouteStatus, len(*in))
		copy(*out, *in)
	}
}

func (in *TrafficManagerStatus) DeepCopyInto(out *TrafficManagerStatus) {
	*out = *in
	if in.Nginx != nil {
		in, out := &in.Nginx, &out.Nginx
		*out = new(NginxTrafficManagerStatus)
		(*in).DeepCopyInto(*out)
	}
	if in.Cman != nil {
		in, out := &in.Cman, &out.Cman
		*out = new(CmanTrafficManagerStatus)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *TrafficManager) DeepCopyInto(out *TrafficManager) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *TrafficManager) DeepCopy() *TrafficManager {
	if in == nil {
		return nil
	}
	out := new(TrafficManager)
	in.DeepCopyInto(out)
	return out
}

func (in *TrafficManager) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *TrafficManagerList) DeepCopyInto(out *TrafficManagerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TrafficManager, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *TrafficManagerList) DeepCopy() *TrafficManagerList {
	if in == nil {
		return nil
	}
	out := new(TrafficManagerList)
	in.DeepCopyInto(out)
	return out
}

func (in *TrafficManagerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
