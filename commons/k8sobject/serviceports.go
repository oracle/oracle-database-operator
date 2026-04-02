package k8sobjects

import corev1 "k8s.io/api/core/v1"

type servicePortKey struct {
	name     string
	port     int32
	protocol corev1.Protocol
}

// MergeServicePortsWithAssignedNodePortsByNamePortProtocol preserves assigned NodePorts for matching name+port+protocol keys.
func MergeServicePortsWithAssignedNodePortsByNamePortProtocol(existing, desired []corev1.ServicePort) []corev1.ServicePort {
	merged := make([]corev1.ServicePort, len(desired))
	copy(merged, desired)

	if len(existing) == 0 || len(desired) == 0 {
		return merged
	}

	existingNodePortByKey := make(map[servicePortKey]int32, len(existing))
	for _, port := range existing {
		if port.NodePort == 0 {
			continue
		}
		key := servicePortKey{name: port.Name, port: port.Port, protocol: port.Protocol}
		existingNodePortByKey[key] = port.NodePort
	}

	for i := range merged {
		if merged[i].NodePort != 0 {
			continue
		}
		key := servicePortKey{name: merged[i].Name, port: merged[i].Port, protocol: merged[i].Protocol}
		if assignedNodePort, ok := existingNodePortByKey[key]; ok {
			merged[i].NodePort = assignedNodePort
		}
	}

	return merged
}

// MergeServicePortsWithAssignedNodePortByName preserves assigned NodePorts for matching service-port names.
func MergeServicePortsWithAssignedNodePortByName(existing, desired []corev1.ServicePort) []corev1.ServicePort {
	nodePortByName := make(map[string]int32, len(existing))
	for _, p := range existing {
		if p.NodePort > 0 {
			nodePortByName[p.Name] = p.NodePort
		}
	}

	out := make([]corev1.ServicePort, 0, len(desired))
	for _, p := range desired {
		if p.NodePort == 0 {
			if np, ok := nodePortByName[p.Name]; ok {
				p.NodePort = np
			}
		}
		out = append(out, p)
	}
	return out
}
