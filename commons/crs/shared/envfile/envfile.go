package envfile

import (
	"sort"
	"strings"
)

// ParseMap parses CRLF-separated envfile key/value pairs into a map.
func ParseMap(data string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(data, "\r\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

// SerializeMap serializes env vars back to CRLF-separated key/value lines.
// Output is sorted for deterministic reconciliation output.
func SerializeMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+values[k])
	}
	return strings.Join(lines, "\r\n")
}
