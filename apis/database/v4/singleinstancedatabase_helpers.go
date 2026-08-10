package v4

import "strings"

// DefaultSIDBAdminSecretKey is the default data-entry name inside a Kubernetes
// Secret that stores the SIDB admin password. It is not cryptographic key
// material; the password value itself is always loaded from the external Secret.
const DefaultSIDBAdminSecretKey = "oracle_pwd"

// ResolveSIDBAdminSecretRef resolves the admin password Kubernetes Secret
// reference (Secret name + data-entry name) from the preferred grouped field
// first and then falls back to the deprecated legacy field for compatibility.
//
// The second return value is the Secret data-entry name (map key), not an
// encryption key. Password material is never hardcoded here; it is always
// obtained from the referenced Kubernetes Secret at runtime.
func ResolveSIDBAdminSecretRef(sidb *SingleInstanceDatabase) (string, string, bool) {
	if sidb == nil {
		return "", "", false
	}

	if sidb.Spec.Security != nil && sidb.Spec.Security.Secrets != nil && sidb.Spec.Security.Secrets.Admin != nil {
		if secretName, dataEntry, ok := resolveSIDBAdminPasswordRef(sidb.Spec.Security.Secrets.Admin); ok {
			return secretName, dataEntry, true
		}
	}

	return resolveSIDBAdminPasswordRef(&sidb.Spec.AdminPassword)
}

// resolveSIDBAdminPasswordRef returns the Kubernetes Secret name and data-entry
// name used to locate the admin password. The data-entry name always resolves
// to a non-empty value when a Secret name is present.
func resolveSIDBAdminPasswordRef(ref *SingleInstanceDatabaseAdminPassword) (string, string, bool) {
	if ref == nil {
		return "", "", false
	}

	secretName := strings.TrimSpace(ref.SecretName)
	if secretName == "" {
		return "", "", false
	}

	return secretName, resolveSIDBAdminSecretDataEntry(ref.SecretKey), true
}

// resolveSIDBAdminSecretDataEntry returns the Kubernetes Secret data-entry name
// for the admin password. When the CR does not specify one, the documented
// default entry name is used. This is never encryption-key material.
func resolveSIDBAdminSecretDataEntry(configured string) string {
	if dataEntry := strings.TrimSpace(configured); dataEntry != "" {
		return dataEntry
	}
	return DefaultSIDBAdminSecretKey
}
