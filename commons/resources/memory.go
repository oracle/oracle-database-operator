// Package resources provides memory sizing and validation helpers.
package resources

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultSafetyPct is the default memory safety threshold.
	DefaultSafetyPct        = 0.80
	MinContainerMemoryBytes = int64(16 * 1024 * 1024 * 1024) // 16GiB
	pageSizeBytes           = int64(4096)
	oneGBBytes              = int64(1024 * 1024 * 1024)
	DefaultSem              = "250 32000 100 128"
	DefaultShmmni           = "4096"
)

// ValidateMemorySize ensures memory strings use supported units and format.
func ValidateMemorySize(sizeStr string) error {
	matched, _ := regexp.MatchString(`^\d+(Gi|Mi|G|M)$`, sizeStr)
	if !matched {
		return fmt.Errorf("memory size must be of form <number>[M|G|Mi|Gi], e.g., 3G, 1024M, 16Gi")
	}
	return nil
}

// ParseMemoryBytes converts memory strings into byte counts for validation.
func ParseMemoryBytes(memStr string) (int64, error) {
	if memStr == "" {
		return 0, nil
	}

	var numStr string
	var multiplier int64
	trimmed := strings.TrimSpace(memStr)
	switch {
	case strings.HasSuffix(trimmed, "Gi"), strings.HasSuffix(trimmed, "gi"):
		numStr = trimmed[:len(trimmed)-2]
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(trimmed, "Mi"), strings.HasSuffix(trimmed, "mi"):
		numStr = trimmed[:len(trimmed)-2]
		multiplier = 1024 * 1024
	case strings.HasSuffix(trimmed, "GB"), strings.HasSuffix(trimmed, "gb"):
		numStr = trimmed[:len(trimmed)-2]
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(trimmed, "G"), strings.HasSuffix(trimmed, "g"):
		numStr = trimmed[:len(trimmed)-1]
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(trimmed, "M"), strings.HasSuffix(trimmed, "m"):
		numStr = trimmed[:len(trimmed)-1]
		multiplier = 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid memory unit in %s", memStr)
	}

	num, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in %s", memStr)
	}

	return num * multiplier, nil
}

// ParseBytesOrMemory parses either raw integer bytes or memory-unit strings.
func ParseBytesOrMemory(val string) (int64, error) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return 0, nil
	}
	if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return v, nil
	}
	return ParseMemoryBytes(trimmed)
}

// ExtractMemoryAndHugePagesBytes extracts memory and hugepages bytes from resources.
func ExtractMemoryAndHugePagesBytes(resources *corev1.ResourceRequirements) (int64, int64) {
	if resources == nil {
		return 0, 0
	}
	var memLimit int64
	if memQ, ok := resources.Limits[corev1.ResourceMemory]; ok {
		memLimit = memQ.Value()
	}
	var hugeMem int64
	if hpQ, ok := resources.Limits[corev1.ResourceName("hugepages-2Mi")]; ok {
		hugeMem = hpQ.Value()
	}
	if hugeMem == 0 {
		if hpQ, ok := resources.Requests[corev1.ResourceName("hugepages-2Mi")]; ok {
			hugeMem = hpQ.Value()
		}
	}
	return memLimit, hugeMem
}

func resolveEffectiveSgaPga(sga, pga, containerMem int64) (int64, int64) {
	if sga == 0 && pga == 0 && containerMem > 0 {
		derivedSga := containerMem / 2
		derivedPga := derivedSga / 4
		return derivedSga, derivedPga
	}
	return sga, pga
}

// ValidateSgaPgaSafety validates memory budget with a fixed 1GiB OS/container overhead.
// When hugepages are used, regular memory must still fit PGA + overhead.
// The safetyPct parameter is retained for API compatibility and ignored.
func ValidateSgaPgaSafety(sga, pga, memLimit, hugeMem int64, safetyPct float64) error {
	_ = safetyPct

	if memLimit <= 0 {
		return nil
	}
	sga, pga = resolveEffectiveSgaPga(sga, pga, memLimit)

	if hugeMem > 0 {
		if hugeMem < sga {
			return fmt.Errorf("HugePages (%d bytes) must be >= SGA size (%d bytes)", hugeMem, sga)
		}
		requiredRegular := pga + oneGBBytes
		if memLimit < requiredRegular {
			return fmt.Errorf(
				"regular container memory (%dB) must be >= PGA (%dB) + overhead (%dB)",
				memLimit, pga, oneGBBytes,
			)
		}
		return nil
	}

	requiredTotal := sga + pga + oneGBBytes
	if memLimit < requiredTotal {
		return fmt.Errorf(
			"container memory (%dB) must be >= SGA (%dB) + PGA (%dB) + overhead (%dB)",
			memLimit, sga, pga, oneGBBytes,
		)
	}
	return nil
}

// ValidateHugePagesAtLeastSga validates hugepages capacity against SGA.
func ValidateHugePagesAtLeastSga(hugeMem, sga int64) error {
	if hugeMem > 0 && sga > 0 && hugeMem < sga {
		return fmt.Errorf("HugePages (%d bytes) must be >= SGA size (%d bytes)", hugeMem, sga)
	}
	return nil
}

// CalculateOracleSysctls computes shmmax/shmall/sem/shmmni values.
func CalculateOracleSysctls(
	sgaBytes, pgaBytes, containerMemBytes, hugePagesBytes, userShmmax, userShmall int64,
) ([]corev1.Sysctl, error) {
	if containerMemBytes > 0 && containerMemBytes < MinContainerMemoryBytes {
		return nil, fmt.Errorf("container memory (%d) is less than minimum required (%d)", containerMemBytes, MinContainerMemoryBytes)
	}
	if containerMemBytes == 0 && sgaBytes == 0 && pgaBytes == 0 {
		return []corev1.Sysctl{}, nil
	}
	sgaBytes, pgaBytes = resolveEffectiveSgaPga(sgaBytes, pgaBytes, containerMemBytes)

	if containerMemBytes == 0 {
		shmmax := sgaBytes + oneGBBytes
		if userShmmax > 0 {
			if userShmmax < sgaBytes {
				return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than SGA_TARGET (%d)", userShmmax, sgaBytes)
			}
			shmmax = userShmmax
		}
		shmall := (shmmax + pageSizeBytes - 1) / pageSizeBytes
		if userShmall > 0 {
			if userShmall < shmall {
				return nil, fmt.Errorf("user-provided shmall (%d) is too small; min required=%d pages", userShmall, shmall)
			}
			shmall = userShmall
		}
		return []corev1.Sysctl{
			{Name: "kernel.shmmax", Value: fmt.Sprintf("%d", shmmax)},
			{Name: "kernel.shmall", Value: fmt.Sprintf("%d", shmall)},
			{Name: "kernel.sem", Value: DefaultSem},
			{Name: "kernel.shmmni", Value: DefaultShmmni},
		}, nil
	}

	if pgaBytes > containerMemBytes {
		return nil, fmt.Errorf("PGA_TARGET (%d) cannot be greater than container memory (%d)", pgaBytes, containerMemBytes)
	}
	if sgaBytes > containerMemBytes {
		return nil, fmt.Errorf("SGA_TARGET (%d) cannot be greater than container memory (%d)", sgaBytes, containerMemBytes)
	}
	if hugePagesBytes > 0 {
		requiredRegular := pgaBytes + oneGBBytes
		if containerMemBytes < requiredRegular {
			return nil, fmt.Errorf(
				"container regular memory (%d) must be >= PGA_TARGET (%d) + overhead (%d)",
				containerMemBytes, pgaBytes, oneGBBytes,
			)
		}
	} else {
		requiredTotal := sgaBytes + pgaBytes + oneGBBytes
		if containerMemBytes < requiredTotal {
			return nil, fmt.Errorf(
				"container memory (%d) must be >= SGA_TARGET (%d) + PGA_TARGET (%d) + overhead (%d)",
				containerMemBytes, sgaBytes, pgaBytes, oneGBBytes,
			)
		}
	}

	var shmmax int64
	if userShmmax > 0 {
		if userShmmax < sgaBytes {
			return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than SGA_TARGET (%d)", userShmmax, sgaBytes)
		}
		if hugePagesBytes > 0 && userShmmax < hugePagesBytes {
			return nil, fmt.Errorf("user-provided shmmax (%d) cannot be less than hugePages memory (%d)", userShmmax, hugePagesBytes)
		}
		if userShmmax > containerMemBytes-oneGBBytes {
			return nil, fmt.Errorf("user-provided shmmax (%d) must be < container memory - 1GB (%d)", userShmmax, containerMemBytes-oneGBBytes)
		}
		shmmax = userShmmax
	} else if hugePagesBytes > 0 {
		if hugePagesBytes < sgaBytes {
			return nil, fmt.Errorf("huge pages (%d) must be >= SGA_TARGET (%d)", hugePagesBytes, sgaBytes)
		}
		shmmax = hugePagesBytes
	} else if sgaBytes < (containerMemBytes / 2) {
		shmmax = containerMemBytes / 2
	} else {
		shmmax = sgaBytes + oneGBBytes
	}

	if shmmax >= containerMemBytes {
		shmmax = containerMemBytes - oneGBBytes
	}

	shmall := (shmmax + pageSizeBytes - 1) / pageSizeBytes
	if userShmall > 0 {
		if userShmall < shmall {
			return nil, fmt.Errorf("user-provided shmall (%d) is too small; min required=%d pages", userShmall, shmall)
		}
		shmall = userShmall
	}

	return []corev1.Sysctl{
		{Name: "kernel.shmmax", Value: fmt.Sprintf("%d", shmmax)},
		{Name: "kernel.shmall", Value: fmt.Sprintf("%d", shmall)},
		{Name: "kernel.sem", Value: DefaultSem},
		{Name: "kernel.shmmni", Value: DefaultShmmni},
	}, nil
}
