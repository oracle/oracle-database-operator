package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *ShardingDatabase) ConvertTo(dst conversion.Hub) error {
	return nil
}

// ConvertFrom converts v1 to v1alpha1
func (dst *ShardingDatabase) ConvertFrom(src conversion.Hub) error {
	return nil
}
