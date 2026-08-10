package asm

// FlattenUniqueDiskGroups flattens grouped disk lists while preserving first-seen order and removing duplicates.
func FlattenUniqueDiskGroups(groups [][]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, group := range groups {
		for _, disk := range group {
			if _, ok := seen[disk]; ok {
				continue
			}
			seen[disk] = struct{}{}
			out = append(out, disk)
		}
	}
	return out
}
