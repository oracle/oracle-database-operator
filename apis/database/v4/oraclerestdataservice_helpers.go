package v4

import "strings"

// DefaultOracleRestDataServiceSecretKey is the default data-entry name inside a
// Kubernetes Secret that stores an ORDS password. It is not cryptographic key
// material; the password value itself is always loaded from the external Secret.
const DefaultOracleRestDataServiceSecretKey = "oracle_pwd"

// ResolveOracleRestDataServiceAdminSecretRef resolves the database admin
// password Secret. The grouped v4 field wins over the deprecated flat field.
//
// The second return value is the Secret data-entry name (map key), not an
// encryption key. Password material is never hardcoded here; it is always
// obtained from the referenced Kubernetes Secret at runtime.
func ResolveOracleRestDataServiceAdminSecretRef(ords *OracleRestDataService) (string, string, bool, bool) {
	if ords == nil {
		return "", "", true, false
	}
	if ords.Spec.Security != nil && ords.Spec.Security.Secrets != nil && ords.Spec.Security.Secrets.DatabaseAdmin != nil {
		if secretName, dataEntry, keepSecret, ok := resolveOracleRestDataServicePasswordRef(ords.Spec.Security.Secrets.DatabaseAdmin); ok {
			return secretName, dataEntry, keepSecret, true
		}
	}
	return resolveOracleRestDataServicePasswordRef(&ords.Spec.AdminPassword)
}

// ResolveOracleRestDataServiceOrdsSecretRef resolves the ORDS public user
// password Secret. The grouped v4 field wins over the deprecated flat field.
//
// The second return value is the Secret data-entry name (map key), not an
// encryption key. Password material is never hardcoded here; it is always
// obtained from the referenced Kubernetes Secret at runtime.
func ResolveOracleRestDataServiceOrdsSecretRef(ords *OracleRestDataService) (string, string, bool, bool) {
	if ords == nil {
		return "", "", true, false
	}
	if ords.Spec.Security != nil && ords.Spec.Security.Secrets != nil && ords.Spec.Security.Secrets.OrdsPublicUser != nil {
		if secretName, dataEntry, keepSecret, ok := resolveOracleRestDataServicePasswordRef(ords.Spec.Security.Secrets.OrdsPublicUser); ok {
			return secretName, dataEntry, keepSecret, true
		}
	}
	return resolveOracleRestDataServicePasswordRef(&ords.Spec.OrdsPassword)
}

// resolveOracleRestDataServicePasswordRef returns the Kubernetes Secret name and
// data-entry name used to locate a password. The data-entry name always
// resolves to a non-empty value when a Secret name is present.
func resolveOracleRestDataServicePasswordRef(ref *OracleRestDataServicePassword) (string, string, bool, bool) {
	if ref == nil {
		return "", "", true, false
	}
	secretName := strings.TrimSpace(ref.SecretName)
	if secretName == "" {
		return "", "", true, false
	}
	keepSecret := true
	if ref.KeepSecret != nil {
		keepSecret = *ref.KeepSecret
	}
	return secretName, resolveOracleRestDataServiceSecretDataEntry(ref.SecretKey), keepSecret, true
}

// resolveOracleRestDataServiceSecretDataEntry returns the Kubernetes Secret
// data-entry name for an ORDS password. When the CR does not specify one, the
// documented default entry name is used. This is never encryption-key material.
func resolveOracleRestDataServiceSecretDataEntry(configured string) string {
	if dataEntry := strings.TrimSpace(configured); dataEntry != "" {
		return dataEntry
	}
	return DefaultOracleRestDataServiceSecretKey
}

func defaultOracleRestDataServicePasswordRef(ref *OracleRestDataServicePassword) {
	if ref == nil || !oracleRestDataServicePasswordFieldsSet(ref) {
		return
	}
	ref.SecretKey = resolveOracleRestDataServiceSecretDataEntry(ref.SecretKey)
	if ref.KeepSecret == nil {
		keepSecret := true
		ref.KeepSecret = &keepSecret
	}
}

func oracleRestDataServicePasswordFieldsSet(ref *OracleRestDataServicePassword) bool {
	if ref == nil {
		return false
	}
	return strings.TrimSpace(ref.SecretName) != "" ||
		strings.TrimSpace(ref.SecretKey) != "" ||
		ref.KeepSecret != nil
}
