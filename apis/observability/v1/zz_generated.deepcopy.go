//go:build !ignore_autogenerated

/*
** Copyright (c) 2022 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigMapDetails) DeepCopyInto(out *ConfigMapDetails) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigMapDetails.
func (in *ConfigMapDetails) DeepCopy() *ConfigMapDetails {
	if in == nil {
		return nil
	}
	out := new(ConfigMapDetails)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBSecret) DeepCopyInto(out *DBSecret) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBSecret.
func (in *DBSecret) DeepCopy() *DBSecret {
	if in == nil {
		return nil
	}
	out := new(DBSecret)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DBSecretWithVault) DeepCopyInto(out *DBSecretWithVault) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DBSecretWithVault.
func (in *DBSecretWithVault) DeepCopy() *DBSecretWithVault {
	if in == nil {
		return nil
	}
	out := new(DBSecretWithVault)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserver) DeepCopyInto(out *DatabaseObserver) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserver.
func (in *DatabaseObserver) DeepCopy() *DatabaseObserver {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserver)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseObserver) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverConfigMap) DeepCopyInto(out *DatabaseObserverConfigMap) {
	*out = *in
	out.Configmap = in.Configmap
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverConfigMap.
func (in *DatabaseObserverConfigMap) DeepCopy() *DatabaseObserverConfigMap {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverConfigMap)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverDatabase) DeepCopyInto(out *DatabaseObserverDatabase) {
	*out = *in
	out.DBUser = in.DBUser
	out.DBPassword = in.DBPassword
	out.DBWallet = in.DBWallet
	out.DBConnectionString = in.DBConnectionString
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverDatabase.
func (in *DatabaseObserverDatabase) DeepCopy() *DatabaseObserverDatabase {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverDatabase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverDeployment) DeepCopyInto(out *DatabaseObserverDeployment) {
	*out = *in
	if in.ExporterArgs != nil {
		in, out := &in.ExporterArgs, &out.ExporterArgs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExporterCommands != nil {
		in, out := &in.ExporterCommands, &out.ExporterCommands
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExporterEnvs != nil {
		in, out := &in.ExporterEnvs, &out.ExporterEnvs
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.DeploymentPodTemplate.DeepCopyInto(&out.DeploymentPodTemplate)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverDeployment.
func (in *DatabaseObserverDeployment) DeepCopy() *DatabaseObserverDeployment {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverExporterConfig) DeepCopyInto(out *DatabaseObserverExporterConfig) {
	*out = *in
	in.Deployment.DeepCopyInto(&out.Deployment)
	in.Service.DeepCopyInto(&out.Service)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverExporterConfig.
func (in *DatabaseObserverExporterConfig) DeepCopy() *DatabaseObserverExporterConfig {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverExporterConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverList) DeepCopyInto(out *DatabaseObserverList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseObserver, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverList.
func (in *DatabaseObserverList) DeepCopy() *DatabaseObserverList {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseObserverList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverService) DeepCopyInto(out *DatabaseObserverService) {
	*out = *in
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]corev1.ServicePort, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverService.
func (in *DatabaseObserverService) DeepCopy() *DatabaseObserverService {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverSpec) DeepCopyInto(out *DatabaseObserverSpec) {
	*out = *in
	out.Database = in.Database
	in.Exporter.DeepCopyInto(&out.Exporter)
	out.ExporterConfig = in.ExporterConfig
	in.Prometheus.DeepCopyInto(&out.Prometheus)
	out.OCIConfig = in.OCIConfig
	out.Log = in.Log
	if in.InheritLabels != nil {
		in, out := &in.InheritLabels, &out.InheritLabels
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExporterSidecars != nil {
		in, out := &in.ExporterSidecars, &out.ExporterSidecars
		*out = make([]corev1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SideCarVolumes != nil {
		in, out := &in.SideCarVolumes, &out.SideCarVolumes
		*out = make([]corev1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverSpec.
func (in *DatabaseObserverSpec) DeepCopy() *DatabaseObserverSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseObserverStatus) DeepCopyInto(out *DatabaseObserverStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseObserverStatus.
func (in *DatabaseObserverStatus) DeepCopy() *DatabaseObserverStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseObserverStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeploymentPodTemplate) DeepCopyInto(out *DeploymentPodTemplate) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeploymentPodTemplate.
func (in *DeploymentPodTemplate) DeepCopy() *DeploymentPodTemplate {
	if in == nil {
		return nil
	}
	out := new(DeploymentPodTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogConfig) DeepCopyInto(out *LogConfig) {
	*out = *in
	out.Volume = in.Volume
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogConfig.
func (in *LogConfig) DeepCopy() *LogConfig {
	if in == nil {
		return nil
	}
	out := new(LogConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogVolume) DeepCopyInto(out *LogVolume) {
	*out = *in
	out.PersistentVolumeClaim = in.PersistentVolumeClaim
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogVolume.
func (in *LogVolume) DeepCopy() *LogVolume {
	if in == nil {
		return nil
	}
	out := new(LogVolume)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogVolumePVClaim) DeepCopyInto(out *LogVolumePVClaim) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogVolumePVClaim.
func (in *LogVolumePVClaim) DeepCopy() *LogVolumePVClaim {
	if in == nil {
		return nil
	}
	out := new(LogVolumePVClaim)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OCIConfigSpec) DeepCopyInto(out *OCIConfigSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OCIConfigSpec.
func (in *OCIConfigSpec) DeepCopy() *OCIConfigSpec {
	if in == nil {
		return nil
	}
	out := new(OCIConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PrometheusConfig) DeepCopyInto(out *PrometheusConfig) {
	*out = *in
	in.ServiceMonitor.DeepCopyInto(&out.ServiceMonitor)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PrometheusConfig.
func (in *PrometheusConfig) DeepCopy() *PrometheusConfig {
	if in == nil {
		return nil
	}
	out := new(PrometheusConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PrometheusServiceMonitor) DeepCopyInto(out *PrometheusServiceMonitor) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.NamespaceSelector != nil {
		in, out := &in.NamespaceSelector, &out.NamespaceSelector
		*out = new(monitoringv1.NamespaceSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Endpoints != nil {
		in, out := &in.Endpoints, &out.Endpoints
		*out = make([]monitoringv1.Endpoint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PrometheusServiceMonitor.
func (in *PrometheusServiceMonitor) DeepCopy() *PrometheusServiceMonitor {
	if in == nil {
		return nil
	}
	out := new(PrometheusServiceMonitor)
	in.DeepCopyInto(out)
	return out
}
