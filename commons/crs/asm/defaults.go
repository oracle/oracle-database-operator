// Package asm provides shared defaults and models for ASM configuration.
package asm

import "strings"

// DiskGroup is a neutral model used by shared ASM helpers.
type DiskGroup struct {
	Name               string
	Type               string
	Redundancy         string
	Disks              []string
	AutoUpdate         string
	StorageClass       string
	AsmStorageSizeInGb int
}

// DefaultsAdapter exposes only the operations required by ASM defaulting logic.
type DefaultsAdapter interface {
	GetDiskGroups() []DiskGroup
	SetDiskGroups([]DiskGroup)

	GetDbCharSet() string
	SetDbCharSet(string)

	LookupRspData(key, cmName, fileName string) (string, error)

	CrsType() string
	DataType() string
	RecoveryType() string

	CrsLookupKeys() []string
	DefaultCrsName() string
	DefaultRedundancy() string
}

// EnsureDefaults applies default ASM disk-group and charset values.
func EnsureDefaults(a DefaultsAdapter, cmName, fileName string) error {
	groups := a.GetDiskGroups()
	groups = ensureCrsDiskGroup(groups, a, cmName, fileName)
	groups = ensureDbDataDiskGroup(groups, a)
	groups = ensureDbRecoveryDiskGroup(groups, a)
	a.SetDiskGroups(groups)

	if strings.TrimSpace(a.GetDbCharSet()) == "" {
		a.SetDbCharSet("AL32UTF8")
	}
	return nil
}

func ensureCrsDiskGroup(groups []DiskGroup, a DefaultsAdapter, cmName, fileName string) []DiskGroup {
	crsFound := false

	for i := range groups {
		dg := groups[i]
		if dg.Name == "" && dg.Redundancy == "" && dg.Type == "" && len(dg.Disks) > 0 {
			groups[i].Name = lookupCrsName(a, cmName, fileName)
			groups[i].Redundancy = lookupRedundancy(a, cmName, fileName)
			groups[i].Type = a.CrsType()
			crsFound = true
			continue
		}
		if dg.Type == a.CrsType() {
			if dg.Name == "" {
				groups[i].Name = lookupCrsName(a, cmName, fileName)
			}
			if dg.Redundancy == "" {
				groups[i].Redundancy = lookupRedundancy(a, cmName, fileName)
			}
			crsFound = true
		}
	}

	if !crsFound {
		groups = append(groups, DiskGroup{
			Name:       a.DefaultCrsName(),
			Type:       a.CrsType(),
			Redundancy: a.DefaultRedundancy(),
			Disks:      []string{},
		})
	}
	return groups
}

func ensureDbDataDiskGroup(groups []DiskGroup, a DefaultsAdapter) []DiskGroup {
	var crsName string
	for _, dg := range groups {
		if dg.Type == a.CrsType() {
			crsName = dg.Name
			break
		}
	}
	for i := range groups {
		if groups[i].Type == a.DataType() {
			if groups[i].Name == "" {
				groups[i].Name = crsName
			}
			return groups
		}
	}
	return append(groups, DiskGroup{Name: crsName, Type: a.DataType()})
}

func ensureDbRecoveryDiskGroup(groups []DiskGroup, a DefaultsAdapter) []DiskGroup {
	var dataName string
	for _, dg := range groups {
		if dg.Type == a.DataType() {
			dataName = dg.Name
			break
		}
	}
	for i := range groups {
		if groups[i].Type == a.RecoveryType() {
			if groups[i].Name == "" {
				groups[i].Name = dataName
			}
			return groups
		}
	}
	return append(groups, DiskGroup{Name: dataName, Type: a.RecoveryType()})
}

func lookupCrsName(a DefaultsAdapter, cmName, fileName string) string {
	for _, k := range a.CrsLookupKeys() {
		v, err := a.LookupRspData(k, cmName, fileName)
		if err != nil {
			continue
		}
		n := NormalizeDGName(v)
		if n != "" {
			return n
		}
	}
	return a.DefaultCrsName()
}

func lookupRedundancy(a DefaultsAdapter, cmName, fileName string) string {
	v, err := a.LookupRspData("redundancy", cmName, fileName)
	if err == nil && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return a.DefaultRedundancy()
}

// NormalizeDGName normalizes DG name variants from response files.
func NormalizeDGName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "+") {
		if i := strings.Index(v, "/"); i > 0 {
			return v[:i]
		}
		return v
	}
	if strings.Contains(v, "/") {
		return strings.SplitN(v, "/", 2)[0]
	}
	return v
}
