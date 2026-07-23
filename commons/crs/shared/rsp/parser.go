// Package rsp parses Oracle response files used by provisioning workflows.
package rsp

import (
	"errors"
	"strings"
)

// ParseValue extracts a response file value for a key.
// Keys with dots are matched exactly; keys without dots match the last segment.
func ParseValue(content, key string) (string, error) {
	searchKey := strings.ToLower(strings.TrimSpace(key))
	keyHasDot := strings.Contains(key, ".")

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		splitIndex := strings.Index(line, "=")
		if splitIndex == -1 {
			continue
		}

		fullKey := strings.TrimSpace(line[:splitIndex])
		fullKeyLower := strings.ToLower(fullKey)
		value := strings.TrimSpace(line[splitIndex+1:])

		if fullKeyLower == "variables" && searchKey == "variables=" {
			return value, nil
		}

		if keyHasDot {
			if fullKeyLower == searchKey {
				return value, nil
			}
			continue
		}

		lastKey := fullKey
		if idx := strings.LastIndex(fullKey, "."); idx != -1 {
			lastKey = fullKey[idx+1:]
		}
		if strings.ToLower(strings.TrimSpace(lastKey)) == searchKey {
			return value, nil
		}
	}

	return "", errors.New("the " + key + " key and value does not exist in grid responsefile. Invalid grid responsefile.")
}
