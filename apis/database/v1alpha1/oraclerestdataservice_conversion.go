package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *OracleRestDataService) ConvertTo(dst conversion.Hub) error {
	return nil
}

// ConvertFrom converts v1 to v1alpha1
func (dst *OracleRestDataService) ConvertFrom(src conversion.Hub) error {
	return nil
}
