package commons

import (
	"strconv"
	"strings"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedresources "github.com/oracle/oracle-database-operator/commons/resources"
	corev1 "k8s.io/api/core/v1"
)

const (
	initSgaSizeEnv = "INIT_SGA_SIZE"
	initPgaSizeEnv = "INIT_PGA_SIZE"
)

func applyOracleMemorySysctls(
	podSecurityContext *corev1.PodSecurityContext,
	resources *corev1.ResourceRequirements,
	envVars []databasev4.EnvironmentVariable,
) *corev1.PodSecurityContext {
	if podSecurityContext == nil {
		podSecurityContext = &corev1.PodSecurityContext{}
	}

	sgaBytes := extractInitMemoryBytes(envVars, initSgaSizeEnv)
	pgaBytes := extractInitMemoryBytes(envVars, initPgaSizeEnv)
	memLimit, hugePages := sharedresources.ExtractMemoryAndHugePagesBytes(resources)
	userShmmax, userShmall := parseUserOracleSysctlOverrides(podSecurityContext.Sysctls)

	sysctls, err := sharedresources.CalculateOracleSysctls(
		sgaBytes,
		pgaBytes,
		memLimit,
		hugePages,
		userShmmax,
		userShmall,
	)
	if err != nil || len(sysctls) == 0 {
		return podSecurityContext
	}

	podSecurityContext.Sysctls = mergeOracleSysctls(podSecurityContext.Sysctls, sysctls)
	return podSecurityContext
}

func extractInitMemoryBytes(envVars []databasev4.EnvironmentVariable, envName string) int64 {
	for i := range envVars {
		if !strings.EqualFold(strings.TrimSpace(envVars[i].Name), envName) {
			continue
		}
		return parseInitMemoryValue(envVars[i].Value)
	}
	return 0
}

// parseInitMemoryValue parses INIT_* memory values.
// For sharding env vars, plain numbers are interpreted as MiB.
func parseInitMemoryValue(raw string) int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return v * 1024 * 1024
	}
	v, err := sharedresources.ParseBytesOrMemory(trimmed)
	if err != nil {
		return 0
	}
	return v
}

func parseUserOracleSysctlOverrides(sysctls []corev1.Sysctl) (int64, int64) {
	var shmmax int64
	var shmall int64

	for i := range sysctls {
		switch strings.TrimSpace(sysctls[i].Name) {
		case "kernel.shmmax":
			if v, err := sharedresources.ParseBytesOrMemory(sysctls[i].Value); err == nil {
				shmmax = v
			}
		case "kernel.shmall":
			if v, err := strconv.ParseInt(strings.TrimSpace(sysctls[i].Value), 10, 64); err == nil {
				shmall = v
			}
		}
	}

	return shmmax, shmall
}

func mergeOracleSysctls(existing, calculated []corev1.Sysctl) []corev1.Sysctl {
	out := make([]corev1.Sysctl, 0, len(existing)+len(calculated))
	indexByName := make(map[string]int, len(existing)+len(calculated))

	for i := range existing {
		name := strings.TrimSpace(existing[i].Name)
		indexByName[name] = len(out)
		out = append(out, existing[i])
	}

	for i := range calculated {
		name := strings.TrimSpace(calculated[i].Name)
		if idx, ok := indexByName[name]; ok {
			// Respect explicit user overrides for sem/shmmni.
			if name == "kernel.sem" || name == "kernel.shmmni" {
				continue
			}
			out[idx].Value = calculated[i].Value
			continue
		}
		indexByName[name] = len(out)
		out = append(out, calculated[i])
	}

	return out
}
