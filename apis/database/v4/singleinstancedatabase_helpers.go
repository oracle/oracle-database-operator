package v4

import "strings"

const DefaultSIDBAdminSecretKey = "oracle_pwd"

// ResolveSIDBAdminSecretRef resolves the admin password secret metadata from
// the preferred grouped field first and then falls back to the deprecated
// legacy field for backward compatibility.
func ResolveSIDBAdminSecretRef(sidb *SingleInstanceDatabase) (string, string, bool) {
	if sidb == nil {
		return "", "", false
	}

	if sidb.Spec.Security != nil && sidb.Spec.Security.Secrets != nil && sidb.Spec.Security.Secrets.Admin != nil {
		secretName := strings.TrimSpace(sidb.Spec.Security.Secrets.Admin.SecretName)
		if secretName != "" {
			secretKey := strings.TrimSpace(sidb.Spec.Security.Secrets.Admin.SecretKey)
			if secretKey == "" {
				secretKey = DefaultSIDBAdminSecretKey
			}
			return secretName, secretKey, true
		}
	}

	secretName := strings.TrimSpace(sidb.Spec.AdminPassword.SecretName)
	if secretName == "" {
		return "", "", false
	}
	secretKey := strings.TrimSpace(sidb.Spec.AdminPassword.SecretKey)
	if secretKey == "" {
		secretKey = DefaultSIDBAdminSecretKey
	}
	return secretName, secretKey, true
}
