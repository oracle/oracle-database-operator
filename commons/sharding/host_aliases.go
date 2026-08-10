package commons

import corev1 "k8s.io/api/core/v1"

func cloneHostAliases(src []corev1.HostAlias) []corev1.HostAlias {
	if len(src) == 0 {
		return nil
	}
	out := make([]corev1.HostAlias, len(src))
	for i := range src {
		out[i].IP = src[i].IP
		if len(src[i].Hostnames) > 0 {
			out[i].Hostnames = append([]string(nil), src[i].Hostnames...)
		}
	}
	return out
}
