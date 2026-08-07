// Package naming provides deterministic Kubernetes-safe resource naming helpers.
//
// ShortHash is used only to derive stable, collision-resistant name segments for
// Kubernetes objects (for example ASM PV/PVC names). It is not used for
// password hashing, integrity of secrets, or other security-critical crypto.
//
// Note: ShortHash uses SHA-256. Changing the hash algorithm changes generated
// resource names for the same inputs. Clusters that already have ASM PV/PVCs
// created with a previous algorithm will need a naming migration or recreate.
package naming

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// SanitizeK8sName normalizes arbitrary text into a DNS-1123 friendly segment.
func SanitizeK8sName(name string, maxLen int) string {
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	sanitized := re.ReplaceAllString(strings.ToLower(name), "-")
	sanitized = strings.Trim(sanitized, "-")
	if maxLen > 0 && len(sanitized) > maxLen {
		sanitized = sanitized[:maxLen]
	}
	return sanitized
}

// ShortHash returns a deterministic SHA-256 hex prefix of length n.
// This is for Kubernetes resource name uniqueness only, not cryptographic
// verification of untrusted data.
func ShortHash(text string, n int) string {
	sum := sha256.Sum256([]byte(text))
	encoded := hex.EncodeToString(sum[:])
	if n <= 0 || n >= len(encoded) {
		return encoded
	}
	return encoded[:n]
}

// AsmPVCName builds a bounded PVC name for ASM disks.
func AsmPVCName(diskPath, dbName string, maxLen int) string {
	base := fmt.Sprintf("asm-pvc-%s-%s", ShortHash(diskPath, 8), SanitizeK8sName(dbName, maxLen))
	if maxLen > 0 && len(base) > maxLen {
		base = base[:maxLen]
	}
	return base
}

// AsmPVName builds a bounded PV name for ASM disks.
func AsmPVName(diskPath, dbName string, maxLen int) string {
	base := fmt.Sprintf("asm-pv-%s-%s", ShortHash(diskPath, 8), SanitizeK8sName(dbName, maxLen))
	if maxLen > 0 && len(base) > maxLen {
		base = base[:maxLen]
	}
	return base
}
