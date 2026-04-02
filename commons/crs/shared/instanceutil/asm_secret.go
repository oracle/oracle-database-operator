package instanceutil

import "strings"

// JoinAsmDiskGroups flattens grouped ASM disks and returns a comma-separated list.
func JoinAsmDiskGroups(groups [][]string) string {
	var asmDisks []string
	for _, group := range groups {
		asmDisks = append(asmDisks, group...)
	}
	return strings.Join(asmDisks, ",")
}

// SSHKeyFlags reports whether expected SSH private/public keys exist in secret data.
func SSHKeyFlags(secretData map[string][]byte, privateKeyName, publicKeyName string) (bool, bool) {
	var privateFound, publicFound bool
	for key := range secretData {
		switch key {
		case privateKeyName:
			privateFound = true
		case publicKeyName:
			publicFound = true
		}
	}
	return privateFound, publicFound
}

// DBSecretFlags reports whether expected DB secret keys exist.
func DBSecretFlags(secretData map[string][]byte, pwdFileName, keyFileName string) (bool, bool, bool) {
	var osPwdFound, keyFileFound, legacyPwdFileFound bool
	for key := range secretData {
		switch key {
		case pwdFileName:
			osPwdFound = true
		case keyFileName:
			keyFileFound = true
		case "pwdfile":
			legacyPwdFileFound = true
		}
	}
	return osPwdFound, keyFileFound, legacyPwdFileFound
}
