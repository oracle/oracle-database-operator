package v4

import (
	"fmt"
	"regexp"
)

const (
	oracleRestDataServiceSchemaNamePattern = "^[A-Za-z][A-Za-z0-9_]{0,29}$"
	oracleRestDataServicePDBNamePattern    = "^$|^[A-Za-z][A-Za-z0-9_]{0,29}$"
	oracleRestDataServiceURLMappingPattern = "^$|^[A-Za-z][A-Za-z0-9_-]{0,29}$"
)

var (
	oracleRestDataServiceSchemaNameRegex = regexp.MustCompile(oracleRestDataServiceSchemaNamePattern)
	oracleRestDataServicePDBNameRegex    = regexp.MustCompile(oracleRestDataServicePDBNamePattern)
	oracleRestDataServiceURLMappingRegex = regexp.MustCompile(oracleRestDataServiceURLMappingPattern)
)

func IsValidOracleRestDataServiceSchemaName(value string) bool {
	return oracleRestDataServiceSchemaNameRegex.MatchString(value)
}

func IsValidOracleRestDataServicePDBName(value string) bool {
	return oracleRestDataServicePDBNameRegex.MatchString(value)
}

func IsValidOracleRestDataServiceURLMapping(value string) bool {
	return oracleRestDataServiceURLMappingRegex.MatchString(value)
}

func ValidateOracleRestDataServiceRestEnableSchema(entry OracleRestDataServiceRestEnableSchemas) error {
	if !IsValidOracleRestDataServiceSchemaName(entry.SchemaName) {
		return fmt.Errorf("schemaName must match %s", oracleRestDataServiceSchemaNamePattern)
	}
	if !IsValidOracleRestDataServicePDBName(entry.PdbName) {
		return fmt.Errorf("pdbName must match %s when specified", oracleRestDataServicePDBNamePattern)
	}
	if !IsValidOracleRestDataServiceURLMapping(entry.UrlMapping) {
		return fmt.Errorf("urlMapping must match %s when specified", oracleRestDataServiceURLMappingPattern)
	}
	return nil
}
