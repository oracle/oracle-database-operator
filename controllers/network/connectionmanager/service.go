package connectionmanager

import (
	"fmt"
	"strings"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func buildConnectionManagerService(inst *networkv4.ConnectionManager, serviceKind string) *corev1.Service {
	spec := inst.Spec.Service.Internal
	name := connectionManagerInternalServiceName(inst)
	svcType := corev1.ServiceTypeClusterIP
	if serviceKind == "external" {
		spec = inst.Spec.Service.External
		name = connectionManagerExternalServiceName(inst)
		if spec.ServiceType == "" {
			svcType = corev1.ServiceTypeLoadBalancer
		} else {
			svcType = spec.ServiceType
		}
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   inst.Namespace,
			Labels:      connectionManagerLabels(inst),
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: connectionManagerLabels(inst),
			Ports:    buildConnectionManagerServicePorts(inst, serviceKind, spec),
		},
	}
	if serviceKind == "external" {
		if spec.ExternalTrafficPolicy != "" {
			service.Spec.ExternalTrafficPolicy = spec.ExternalTrafficPolicy
		}
		if spec.LoadBalancerIP != "" {
			service.Spec.LoadBalancerIP = spec.LoadBalancerIP
		}
		if spec.LoadBalancerClass != nil && strings.TrimSpace(*spec.LoadBalancerClass) != "" {
			service.Spec.LoadBalancerClass = spec.LoadBalancerClass
		}
	}
	for key, value := range spec.Annotations {
		service.Annotations[key] = value
	}
	return service
}

func buildConnectionManagerServicePorts(inst *networkv4.ConnectionManager, serviceKind string, spec networkv4.ConnectionManagerServiceEndpointSpec) []corev1.ServicePort {
	portSpecs := connectionManagerServicePortSpecs(inst, serviceKind, spec)
	ports := make([]corev1.ServicePort, 0, len(portSpecs))
	for _, portSpec := range portSpecs {
		name := portSpec.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("tcp-%d", portSpec.Port)
		}
		protocol := portSpec.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		servicePort := corev1.ServicePort{
			Name:       name,
			Protocol:   protocol,
			Port:       portSpec.Port,
			TargetPort: intstr.FromInt(int(portSpec.TargetPort)),
		}
		if portSpec.NodePort > 0 {
			servicePort.NodePort = portSpec.NodePort
		}
		ports = append(ports, servicePort)
	}
	return ports
}

func connectionManagerServicePortSpecs(inst *networkv4.ConnectionManager, _ string, spec networkv4.ConnectionManagerServiceEndpointSpec) []networkv4.ConnectionManagerServicePortSpec {
	if len(spec.Ports) > 0 {
		out := make([]networkv4.ConnectionManagerServicePortSpec, 0, len(spec.Ports))
		primaryTargetAssigned := false
		for _, port := range spec.Ports {
			if port.Port <= 0 && port.TargetPort <= 0 {
				continue
			}
			if port.Port <= 0 {
				port.Port = port.TargetPort
			}
			if port.TargetPort <= 0 {
				if !primaryTargetAssigned {
					port.TargetPort = connectionManagerPrimaryPort(inst)
					primaryTargetAssigned = true
				} else {
					port.TargetPort = port.Port
				}
			} else if !primaryTargetAssigned {
				primaryTargetAssigned = true
			}
			out = append(out, port)
		}
		if len(out) > 0 {
			return out
		}
	}
	mainPort := spec.Port
	if mainPort == 0 {
		mainPort = connectionManagerPrimaryPort(inst)
	}
	targetPort := spec.TargetPort
	if targetPort == 0 {
		targetPort = connectionManagerPrimaryPort(inst)
	}
	out := []networkv4.ConnectionManagerServicePortSpec{{
		Name:       "cman",
		Port:       mainPort,
		TargetPort: targetPort,
		Protocol:   corev1.ProtocolTCP,
	}}
	if connectionManagerRestEnabled(inst) {
		out = append(out, networkv4.ConnectionManagerServicePortSpec{
			Name:       "rest",
			Port:       connectionManagerRestPort(inst),
			TargetPort: connectionManagerRestPort(inst),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return out
}

func buildConnectionManagerContainerPorts(inst *networkv4.ConnectionManager) []corev1.ContainerPort {
	seen := map[int32]struct{}{}
	appendPorts := func(portSpecs []networkv4.ConnectionManagerServicePortSpec, out []corev1.ContainerPort) []corev1.ContainerPort {
		for _, portSpec := range portSpecs {
			targetPort := portSpec.TargetPort
			if targetPort <= 0 {
				targetPort = portSpec.Port
			}
			if targetPort <= 0 {
				continue
			}
			if _, ok := seen[targetPort]; ok {
				continue
			}
			seen[targetPort] = struct{}{}
			protocol := portSpec.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}
			out = append(out, corev1.ContainerPort{ContainerPort: targetPort, Protocol: protocol})
		}
		return out
	}

	out := make([]corev1.ContainerPort, 0)
	out = appendPorts(connectionManagerServicePortSpecs(inst, "internal", inst.Spec.Service.Internal), out)
	out = appendPorts(connectionManagerServicePortSpecs(inst, "external", inst.Spec.Service.External), out)
	if len(out) == 0 {
		out = append(out, corev1.ContainerPort{ContainerPort: connectionManagerPrimaryPort(inst), Protocol: corev1.ProtocolTCP})
	}
	return out
}

func connectionManagerServiceEnabled(enabled *bool, defaultValue bool) bool {
	if enabled == nil {
		return defaultValue
	}
	return *enabled
}

func connectionManagerURLBase(host string) string {
	if host == "" {
		return ""
	}
	return "http://" + host
}
