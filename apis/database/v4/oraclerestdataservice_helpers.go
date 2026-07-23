package v4

import "strings"

const DefaultOracleRestDataServiceSecretKey = "oracle_pwd"

// ResolveOracleRestDataServiceAdminSecretRef resolves the database admin
// password Secret. The grouped v4 field wins over the deprecated flat field.
func ResolveOracleRestDataServiceAdminSecretRef(ords *OracleRestDataService) (string, string, bool, bool) {
	if ords == nil {
		return "", "", true, false
	}
	if ords.Spec.Security != nil && ords.Spec.Security.Secrets != nil && ords.Spec.Security.Secrets.DatabaseAdmin != nil {
		if secretName, secretKey, keepSecret, ok := resolveOracleRestDataServicePasswordRef(ords.Spec.Security.Secrets.DatabaseAdmin); ok {
			return secretName, secretKey, keepSecret, true
		}
	}
	return resolveOracleRestDataServicePasswordRef(&ords.Spec.AdminPassword)
}

// ResolveOracleRestDataServiceOrdsSecretRef resolves the ORDS public user
// password Secret. The grouped v4 field wins over the deprecated flat field.
func ResolveOracleRestDataServiceOrdsSecretRef(ords *OracleRestDataService) (string, string, bool, bool) {
	if ords == nil {
		return "", "", true, false
	}
	if ords.Spec.Security != nil && ords.Spec.Security.Secrets != nil && ords.Spec.Security.Secrets.OrdsPublicUser != nil {
		if secretName, secretKey, keepSecret, ok := resolveOracleRestDataServicePasswordRef(ords.Spec.Security.Secrets.OrdsPublicUser); ok {
			return secretName, secretKey, keepSecret, true
		}
	}
	return resolveOracleRestDataServicePasswordRef(&ords.Spec.OrdsPassword)
}

func resolveOracleRestDataServicePasswordRef(ref *OracleRestDataServicePassword) (string, string, bool, bool) {
	if ref == nil {
		return "", "", true, false
	}
	secretName := strings.TrimSpace(ref.SecretName)
	if secretName == "" {
		return "", "", true, false
	}
	secretKey := strings.TrimSpace(ref.SecretKey)
	if secretKey == "" {
		secretKey = DefaultOracleRestDataServiceSecretKey
	}
	keepSecret := true
	if ref.KeepSecret != nil {
		keepSecret = *ref.KeepSecret
	}
	return secretName, secretKey, keepSecret, true
}

func defaultOracleRestDataServicePasswordRef(ref *OracleRestDataServicePassword) {
	if ref == nil || !oracleRestDataServicePasswordFieldsSet(ref) {
		return
	}
	if strings.TrimSpace(ref.SecretKey) == "" {
		ref.SecretKey = DefaultOracleRestDataServiceSecretKey
	}
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
