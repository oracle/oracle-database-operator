package asm

import "fmt"

// GroupDiffAdapter exposes disk groups for generic diffing.
type GroupDiffAdapter interface {
	GetAsmGroups() []DiskGroup
}

// GetDisksChanged compares old and new disk-group specs and returns added and removed disks.
func GetDisksChanged(newObj, oldObj GroupDiffAdapter) ([]string, []string) {
	added := []string{}
	removed := []string{}

	oldGroups := oldObj.GetAsmGroups()
	if len(oldGroups) == 0 {
		return added, removed
	}

	toSet := func(disks []string) map[string]bool {
		s := make(map[string]bool, len(disks))
		for _, d := range disks {
			if d != "" {
				s[d] = true
			}
		}
		return s
	}

	newMap := make(map[string][]string)
	for _, dg := range newObj.GetAsmGroups() {
		k := fmt.Sprintf("%s-%s", dg.Name, dg.Type)
		newMap[k] = dg.Disks
	}

	oldMap := make(map[string][]string)
	for _, dg := range oldGroups {
		k := fmt.Sprintf("%s-%s", dg.Name, dg.Type)
		oldMap[k] = dg.Disks
	}

	addSet := map[string]bool{}
	remSet := map[string]bool{}

	for k, newDisks := range newMap {
		oldSet := toSet(oldMap[k])
		for _, d := range newDisks {
			if d != "" && !oldSet[d] {
				addSet[d] = true
			}
		}
	}
	for k, oldDisks := range oldMap {
		newSet := toSet(newMap[k])
		for _, d := range oldDisks {
			if d != "" && !newSet[d] {
				remSet[d] = true
			}
		}
	}

	for d := range addSet {
		added = append(added, d)
	}
	for d := range remSet {
		removed = append(removed, d)
	}
	return added, removed
}
