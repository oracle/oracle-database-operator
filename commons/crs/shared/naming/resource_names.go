package naming

import (
	"crypto/sha1"
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

// ShortHash returns a deterministic SHA-1 prefix of length n.
func ShortHash(text string, n int) string {
	h := sha1.New()
	h.Write([]byte(text))
	sum := hex.EncodeToString(h.Sum(nil))
	if n <= 0 || n >= len(sum) {
		return sum
	}
	return sum[:n]
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
