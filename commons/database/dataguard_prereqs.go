package commons

import (
	"fmt"
	"strings"
)

// ShellQuote wraps a string for safe single-quoted shell usage.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// BuildDataguardPrereqsCommand returns a shell command that configures database-side DG prerequisites.
func BuildDataguardPrereqsCommand(action, brokerConfigDir, standbyRedoSize string) string {
	trimmedAction := strings.TrimSpace(action)
	if trimmedAction == "" {
		trimmedAction = "configure"
	}

	lines := make([]string, 0, 4)
	lines = append(lines, "export DG_ENABLE_BROKER=true")
	if strings.TrimSpace(brokerConfigDir) != "" {
		lines = append(lines, fmt.Sprintf("export DG_BROKER_CONFIG_DIR=%s", ShellQuote(strings.TrimSpace(brokerConfigDir))))
	}
	if strings.TrimSpace(standbyRedoSize) != "" {
		lines = append(lines, fmt.Sprintf("export STANDBY_REDO_SIZE=%s", ShellQuote(strings.TrimSpace(standbyRedoSize))))
	}
	lines = append(lines, fmt.Sprintf("$ORACLE_BASE/${CONFIG_DG_PREREQS_FILE:-configDataguardPrereqs.sh} %s", trimmedAction))
	return strings.Join(lines, "; ")
}
