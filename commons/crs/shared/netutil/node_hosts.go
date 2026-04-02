package netutil

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// PreferredNodeIP returns external IP when present, otherwise internal IP.
func PreferredNodeIP(node *corev1.Node) string {
	var internalIP, externalIP string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			internalIP = addr.Address
		} else if addr.Type == corev1.NodeExternalIP {
			externalIP = addr.Address
		}
	}
	if externalIP != "" {
		return externalIP
	}
	return internalIP
}

// ParseHostsContent returns the last non-empty, non-comment line.
func ParseHostsContent(content string) string {
	var uncommentedLine string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
			uncommentedLine = trimmedLine
		}
	}
	return uncommentedLine
}
