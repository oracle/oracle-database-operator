package commons

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ValidateOracleSQLPassword rejects characters that would break the
// controller's current SQL/password interpolation patterns.
func ValidateOracleSQLPassword(value string) error {
	if strings.ContainsRune(value, '"') {
		return fmt.Errorf("password contains unsupported double quote character")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("password contains unsupported newline characters")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("password contains unsupported NUL character")
	}
	return nil
}

// OracleBinarySizeLiteral is a normalized integer Oracle size literal such as
// 50G backed by an equivalent Kubernetes quantity for size comparisons.
type OracleBinarySizeLiteral struct {
	Canonical string
	Quantity  resource.Quantity
}

var oracleBinarySizeLiteralPattern = regexp.MustCompile(`^([0-9]+)([kKmMgGtTpPeE])(?:([iI]))?$`)

// ParseOracleBinarySizeLiteral accepts integer Oracle size literals such as
// 50G and Kubernetes binary quantities such as 50Gi, and normalizes them to a
// canonical Oracle literal for safe SQL formatting.
func ParseOracleBinarySizeLiteral(value string) (OracleBinarySizeLiteral, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return OracleBinarySizeLiteral{}, fmt.Errorf("size literal is empty")
	}

	matches := oracleBinarySizeLiteralPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return OracleBinarySizeLiteral{}, fmt.Errorf("must be an integer size literal using K, M, G, T, P, E or Ki, Mi, Gi, Ti, Pi, Ei")
	}

	number := matches[1]
	unit := strings.ToUpper(matches[2])
	qty, err := resource.ParseQuantity(number + unit + "i")
	if err != nil {
		return OracleBinarySizeLiteral{}, fmt.Errorf("failed to parse size literal: %w", err)
	}

	return OracleBinarySizeLiteral{
		Canonical: number + unit,
		Quantity:  qty,
	}, nil
}
