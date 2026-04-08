// Package specguard contains immutable/guarded spec field sets for validation.
package specguard

// RestrictedConfigParamFields returns immutable config parameter paths enforced across RAC and Oracle Restart.
func RestrictedConfigParamFields() map[string]struct{} {
	return map[string]struct{}{
		"ConfigParams.DbName":                         {},
		"ConfigParams.GridBase":                       {},
		"ConfigParams.GridHome":                       {},
		"ConfigParams.DbBase":                         {},
		"ConfigParams.DbHome":                         {},
		"ConfigParams.CrsAsmDiskDg":                   {},
		"ConfigParams.CrsAsmDiskDgRedundancy":         {},
		"ConfigParams.DBAsmDiskDgRedundancy":          {},
		"ConfigParams.DbCharSet":                      {},
		"ConfigParams.DbConfigType":                   {},
		"ConfigParams.DbDataFileDestDg":               {},
		"ConfigParams.DbUniqueName":                   {},
		"ConfigParams.DbRecoveryFileDest":             {},
		"ConfigParams.DbRedoFileSize":                 {},
		"ConfigParams.DbStorageType":                  {},
		"ConfigParams.DbSwZipFile":                    {},
		"ConfigParams.GridSwZipFile":                  {},
		"ConfigParams.GridResponseFile.ConfigMapName": {},
		"ConfigParams.GridResponseFile.Name":          {},
		"ConfigParams.DbResponseFile.ConfigMapName":   {},
		"ConfigParams.DbResponseFile.Name":            {},
	}
}
