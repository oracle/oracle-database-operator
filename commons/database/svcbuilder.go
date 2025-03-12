/*
** Copyright (c) 2024 Oracle and/or its affiliates.
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

package commons

import (
	corev1 "k8s.io/api/core/v1"
)

type ServiceBuilder interface {
	SetName(string) *ServiceBuilder
	SetNamespace(string) *ServiceBuilder
	SetLabels(map[string]string) *ServiceBuilder
	SetAnnotation(map[string]string) *ServiceBuilder
	SetPorts([]corev1.ServicePort) *ServiceBuilder
	SetSelector(map[string]string) *ServiceBuilder
	SetPublishNotReadyAddresses(bool) *ServiceBuilder
	SetServiceType(corev1.ServiceType) *ServiceBuilder
	Build() *corev1.Service
}

type RealServiceBuilder struct {
	service corev1.Service
}

func (rsb *RealServiceBuilder) SetName(name string) *RealServiceBuilder {
	rsb.service.ObjectMeta.Name = name
	return rsb
}
func (rsb *RealServiceBuilder) SetNamespace(namespace string) *RealServiceBuilder {
	rsb.service.ObjectMeta.Namespace = namespace
	return rsb
}
func (rsb *RealServiceBuilder) SetLabels(labels map[string]string) *RealServiceBuilder {
	rsb.service.ObjectMeta.Labels = labels
	return rsb
}
func (rsb *RealServiceBuilder) SetAnnotation(annotations map[string]string) *RealServiceBuilder {
	rsb.service.ObjectMeta.Annotations = annotations
	return rsb
}
func (rsb *RealServiceBuilder) SetPorts(ports []corev1.ServicePort) *RealServiceBuilder {
	rsb.service.Spec.Ports = ports
	return rsb
}
func (rsb *RealServiceBuilder) SetSelector(selector map[string]string) *RealServiceBuilder {
	rsb.service.Spec.Selector = selector
	return rsb
}
func (rsb *RealServiceBuilder) SetPublishNotReadyAddresses(flag bool) *RealServiceBuilder {
	rsb.service.Spec.PublishNotReadyAddresses = flag
	return rsb
}
func (rsb *RealServiceBuilder) SetType(serviceType corev1.ServiceType) *RealServiceBuilder {
	rsb.service.Spec.Type = serviceType
	return rsb
}
func (rsb *RealServiceBuilder) Build() corev1.Service {
	return rsb.service
}

func NewRealServiceBuilder() *RealServiceBuilder {
	return &RealServiceBuilder{}
}
