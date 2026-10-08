/*
** Copyright (c) 2024 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package controllers

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"strings"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func readScript(ctx context.Context, filePath string) string {
	log := ctrllog.FromContext(ctx).WithName("readScript")

	// Read the file from controller's filesystem
	scriptData, err := os.ReadFile(filePath)
	if err != nil {
		log.Error(err, "Error reading "+filePath)
		return "error"
	}

	return string(scriptData)
}

// ConfigMapDefine defines a ConfigMap for OrdsSrvs.
func (r *OrdsSrvsReconciler) ConfigMapDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, configMapName string, poolIndex int) (*corev1.ConfigMap, error) {

	//log := ctrllog.FromContext(ctx).WithName("ConfigMapDefine")

	var defData map[string]string
	switch configMapName {
	case rState.ordssrvsScriptsConfigMapName:
		defData = make(map[string]string)
		defData["access_log_forwarder.sh"] = readScript(ctx, "/ordssrvs/access_log_forwarder.sh")
		defData["ords_init.sh"] = readScript(ctx, "/ordssrvs/ords_init.sh")
		defData["ords_start.sh"] = readScript(ctx, "/ordssrvs/ords_start.sh")
		defData["RSADecryptOAEP.java"] = readScript(ctx, "/ordssrvs/RSADecryptOAEP.java")
	case rState.ordssrvsGlobalSettingsConfigMapName:
		// GlobalConfigMap
		var defStandaloneAccessLog string
		if ordssrvs.Spec.GlobalSettings.EnableStandaloneAccessLog {
			defStandaloneAccessLog = `  <entry key="standalone.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defMongoAccessLog string
		if ordssrvs.Spec.GlobalSettings.EnableMongoAccessLog {
			defMongoAccessLog = `  <entry key="mongo.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defCertEntry string
		if rState.httpsEnabled && ordssrvs.Spec.GlobalSettings.CertSecret != nil {
			defCert := escapeXMLText(ordsSABase + `/config/certficate/` + ordssrvs.Spec.GlobalSettings.CertSecret.Certificate)
			defCertEntry = `  <entry key="standalone.https.cert">` + defCert + `</entry>` + "\n"
			defCertKey := escapeXMLText(ordsSABase + `/config/certficate/` + ordssrvs.Spec.GlobalSettings.CertSecret.CertificateKey)
			defCertEntry = defCertEntry + `  <entry key="standalone.https.cert.key">` + defCertKey + `</entry>` + "\n"
		}

		var defStandaloneHTTPSPort string
		if rState.httpsEnabled {
			defStandaloneHTTPSPort = conditionalEntry("standalone.https.port", ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort)
		}
		var defStandaloneHTTPSHost string
		if rState.httpsEnabled {
			defStandaloneHTTPSHost = conditionalEntry("standalone.https.host", ordssrvs.Spec.GlobalSettings.StandaloneHTTPSHost)
		}

		defData = map[string]string{
			"settings.xml": fmt.Sprint(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
				`<!DOCTYPE properties SYSTEM "http://java.sun.com/dtd/properties.dtd">` + "\n" +
				`<properties>` + "\n" +
				conditionalEntry("cache.metadata.graphql.expireAfterAccess", ordssrvs.Spec.GlobalSettings.CacheMetadataGraphQLExpireAfterAccess) +
				conditionalEntry("cache.metadata.graphql.expireAfterWrite", ordssrvs.Spec.GlobalSettings.CacheMetadataGraphQLExpireAfterWrite) +
				conditionalEntry("cache.metadata.jwks.enabled", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSEnabled) +
				conditionalEntry("cache.metadata.jwks.initialCapacity", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSInitialCapacity) +
				conditionalEntry("cache.metadata.jwks.maximumSize", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSMaximumSize) +
				conditionalEntry("cache.metadata.jwks.expireAfterAccess", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterAccess) +
				conditionalEntry("cache.metadata.jwks.expireAfterWrite", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterWrite) +
				conditionalEntry("database.api.management.services.disabled", ordssrvs.Spec.GlobalSettings.DatabaseAPIManagementServicesDisabled) +
				conditionalEntry("db.invalidPoolTimeout", ordssrvs.Spec.GlobalSettings.DBInvalidPoolTimeout) +
				conditionalEntry("db.idlePoolTimeout", ordssrvs.Spec.GlobalSettings.DBIdlePoolTimeout) +
				conditionalEntry("feature.graphql.max.nesting.depth", ordssrvs.Spec.GlobalSettings.FeatureGraphQLMaxNestingDepth) +
				conditionalEntry("request.traceHeaderName", ordssrvs.Spec.GlobalSettings.RequestTraceHeaderName) +
				conditionalEntry("public.properties.url", ordssrvs.Spec.GlobalSettings.PublicPropertiesURL) +
				conditionalEntry("security.credentials.attempts", ordssrvs.Spec.GlobalSettings.SecurityCredentialsAttempts) +
				conditionalEntry("security.credentials.lock.time", ordssrvs.Spec.GlobalSettings.SecurityCredentialsLockTime) +
				conditionalEntry("standalone.context.path", ordssrvs.Spec.GlobalSettings.StandaloneContextPath) +
				conditionalEntry("standalone.http.port", ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort) +
				defStandaloneHTTPSHost +
				defStandaloneHTTPSPort +
				conditionalEntry("standalone.https.san", ordssrvs.Spec.GlobalSettings.StandaloneHTTPSSAN) +
				conditionalEntry("standalone.stop.timeout", ordssrvs.Spec.GlobalSettings.StandaloneStopTimeout) +
				conditionalEntry("standalone.access.log.retainDays", ordssrvs.Spec.GlobalSettings.StandaloneAccessLogRetainDays) +
				conditionalEntry("cache.metadata.timeout", ordssrvs.Spec.GlobalSettings.CacheMetadataTimeout) +
				conditionalEntry("cache.metadata.enabled", ordssrvs.Spec.GlobalSettings.CacheMetadataEnabled) +
				conditionalEntry("database.api.enabled", ordssrvs.Spec.GlobalSettings.DatabaseAPIEnabled) +
				conditionalEntry("instance.api.enabled", ordssrvs.Spec.GlobalSettings.InstanceAPIEnabled) +
				conditionalEntry("debug.printDebugToScreen", ordssrvs.Spec.GlobalSettings.DebugPrintDebugToScreen) +
				conditionalEntry("error.responseFormat", ordssrvs.Spec.GlobalSettings.ErrorResponseFormat) +
				conditionalEntry("icap.port", ordssrvs.Spec.GlobalSettings.ICAPPort) +
				conditionalEntry("icap.secure.port", ordssrvs.Spec.GlobalSettings.ICAPSecurePort) +
				conditionalEntry("icap.server", ordssrvs.Spec.GlobalSettings.ICAPServer) +
				conditionalEntry("log.procedure", ordssrvs.Spec.GlobalSettings.LogProcedure) +
				conditionalEntry("mongo.enabled", ordssrvs.Spec.GlobalSettings.MongoEnabled) +
				conditionalEntry("mongo.port", ordssrvs.Spec.GlobalSettings.MongoPort) +
				conditionalEntry("mongo.idle.timeout", ordssrvs.Spec.GlobalSettings.MongoIdleTimeout) +
				conditionalEntry("mongo.op.timeout", ordssrvs.Spec.GlobalSettings.MongoOpTimeout) +
				conditionalEntry("mongo.tls", ordssrvs.Spec.GlobalSettings.MongoTLS) +
				conditionalEntry("security.disableDefaultExclusionList", ordssrvs.Spec.GlobalSettings.SecurityDisableDefaultExclusionList) +
				conditionalEntry("security.exclusionList", ordssrvs.Spec.GlobalSettings.SecurityExclusionList) +
				conditionalEntry("security.inclusionList", ordssrvs.Spec.GlobalSettings.SecurityInclusionList) +
				conditionalEntry("security.externalHostMappingHeader", ordssrvs.Spec.GlobalSettings.SecurityExternalHostMappingHeader) +
				conditionalEntry("security.externalMappingPathPrefix", ordssrvs.Spec.GlobalSettings.SecurityExternalMappingPathPrefix) +
				conditionalEntry("security.host.headers", ordssrvs.Spec.GlobalSettings.SecurityHostHeaders) +
				conditionalEntry("security.maxEntries", ordssrvs.Spec.GlobalSettings.SecurityMaxEntries) +
				conditionalEntry("security.verifySSL", ordssrvs.Spec.GlobalSettings.SecurityVerifySSL) +
				conditionalEntry("security.httpsHeaderCheck", ordssrvs.Spec.GlobalSettings.SecurityHTTPSHeaderCheck) +
				conditionalEntry("security.forceHTTPS", ordssrvs.Spec.GlobalSettings.SecurityForceHTTPS) +
				conditionalEntry("security.externalSessionTrustedOrigins", ordssrvs.Spec.GlobalSettings.SecurityExternalSessionTrustedOrigins) +
				`  <entry key="standalone.doc.root">` + ordsSABase + `/config/global/doc_root/</entry>` + "\n" +
				// Dynamic
				defStandaloneAccessLog +
				defMongoAccessLog +
				defCertEntry +
				`</properties>`),
			"logging.properties": fmt.Sprintf(`handlers=java.util.logging.FileHandler` + "\n" +
				`.level=SEVERE` + "\n" +
				`java.util.logging.FileHandler.level=ALL` + "\n" +
				`oracle.dbtools.level=FINEST` + "\n" +
				`java.util.logging.FileHandler.pattern = ` + ordsSABase + `/log/global/debug.log` + "\n" +
				`java.util.logging.FileHandler.formatter = java.util.logging.SimpleFormatter`),
		}
	default:
		// PoolConfigMap
		poolName := strings.ToLower(ordssrvs.Spec.PoolSettings[poolIndex].PoolName)

		// tnsadmin
		tnsadminEntry := conditionalEntry("db.tnsDirectory", ordsSABase+"/config/databases/"+poolName+"/network/admin/")

		// Pool Zip Wallet
		var zipWalletPathEntry string
		if ordssrvs.Spec.PoolSettings[poolIndex].DBWalletSecret != nil {
			tnsadminEntry = ""
			zipWalletPathEntry = conditionalEntry("db.wallet.zip.path", ordsSABase+"/config/databases/"+poolName+"/network/admin/"+ordssrvs.Spec.PoolSettings[poolIndex].DBWalletSecret.WalletName)
		}

		// Shared Zip Wallets
		// using shared zip wallet in fixed path /opt/oracle/sa/zipwallets
		sharedZipWalletEntry := ""
		if ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName != "" && ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletName != "" {
			tnsadminEntry = ""
			sharedZipWalletEntry = conditionalEntry("db.wallet.zip.path", "/opt/oracle/sa/zipwallets/"+ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletName)
		}

		defData = map[string]string{
			"pool.xml": fmt.Sprint(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
				`<!DOCTYPE properties SYSTEM "http://java.sun.com/dtd/properties.dtd">` + "\n" +
				`<properties>` + "\n" +
				//`  <entry key="db.username">` + ordssrvs.Spec.PoolSettings[poolIndex].DBUsername + `</entry>` + "\n" +
				conditionalEntry("db.username", ordssrvs.Spec.PoolSettings[poolIndex].DBUsername) +
				conditionalEntry("db.adminUser", ordssrvs.Spec.PoolSettings[poolIndex].DBAdminUser) +
				conditionalEntry("db.cdb.adminUser", ordssrvs.Spec.PoolSettings[poolIndex].DBCDBAdminUser) +
				conditionalEntry("apex.security.administrator.roles", ordssrvs.Spec.PoolSettings[poolIndex].ApexSecurityAdministratorRoles) +
				conditionalEntry("apex.security.user.roles", ordssrvs.Spec.PoolSettings[poolIndex].ApexSecurityUserRoles) +
				conditionalEntry("apex.security.developer.roles", ordssrvs.Spec.PoolSettings[poolIndex].ApexSecurityDeveloperRoles) +
				conditionalEntry("db.credentialsSource", ordssrvs.Spec.PoolSettings[poolIndex].DBCredentialsSource) +
				conditionalEntry("db.poolDestroyTimeout", ordssrvs.Spec.PoolSettings[poolIndex].DBPoolDestroyTimeout) +
				conditionalEntry("db.authProvider", ordssrvs.Spec.PoolSettings[poolIndex].DBAuthProvider) +
				conditionalEntry("db.databaseToolsConnection", ordssrvs.Spec.PoolSettings[poolIndex].DBDatabaseToolsConnection) +
				conditionalEntry("db.description", ordssrvs.Spec.PoolSettings[poolIndex].DBDescription) +
				conditionalEntry("db.ociProfile", ordssrvs.Spec.PoolSettings[poolIndex].DBOCIProfile) +
				conditionalEntry("db.serviceNameSuffix", ordssrvs.Spec.PoolSettings[poolIndex].DBServiceNameSuffix) +
				conditionalEntry("debug.trackResources", ordssrvs.Spec.PoolSettings[poolIndex].DebugTrackResources) +
				conditionalEntry("debug.printOWADebug", ordssrvs.Spec.PoolSettings[poolIndex].DebugPrintOWADebug) +
				conditionalEntry("feature.openservicebroker.exclude", ordssrvs.Spec.PoolSettings[poolIndex].FeatureOpenservicebrokerExclude) +
				conditionalEntry("feature.sdw", ordssrvs.Spec.PoolSettings[poolIndex].FeatureSDW) +
				conditionalEntry("feature.sdw.selfServiceSchema", ordssrvs.Spec.PoolSettings[poolIndex].FeatureSDWSelfServiceSchema) +
				conditionalEntry("feature.graphql", ordssrvs.Spec.PoolSettings[poolIndex].FeatureGraphQL) +
				conditionalEntry("http.cookie.filter", ordssrvs.Spec.PoolSettings[poolIndex].HTTPCookieFilter) +
				conditionalEntry("http.cookie.filter.byValue", ordssrvs.Spec.PoolSettings[poolIndex].HTTPCookieFilterByValue) +
				conditionalEntry("jdbc.auth.admin.role", ordssrvs.Spec.PoolSettings[poolIndex].JDBCAuthAdminRole) +
				conditionalEntry("jdbc.cleanup.mode", ordssrvs.Spec.PoolSettings[poolIndex].JDBCCleanupMode) +
				conditionalEntry("jdbc.ConnectionWaitTimeout", ordssrvs.Spec.PoolSettings[poolIndex].JDBCConnectionWaitTimeout) +
				conditionalEntry("jdbc.driverName", ordssrvs.Spec.PoolSettings[poolIndex].JDBCDriverName) +
				conditionalEntry("jdbc.sessionlesstxn.timeout", ordssrvs.Spec.PoolSettings[poolIndex].JDBCSessionlessTxnTimeout) +
				conditionalEntry("jdbc.ucp.enableJMX", ordssrvs.Spec.PoolSettings[poolIndex].JDBCUCPEnableJMX) +
				conditionalEntry("json.sdo.geometry.output.geojson", ordssrvs.Spec.PoolSettings[poolIndex].JSONSDOGeometryOutputGeoJSON) +
				conditionalEntry("oracle.jdbc.vectorDefaultGetObjectType", ordssrvs.Spec.PoolSettings[poolIndex].OracleJDBCVectorDefaultGetObjectType) +
				conditionalEntry("owa.trace.sql", ordssrvs.Spec.PoolSettings[poolIndex].OwaTraceSQL) +
				conditionalEntry("owa.docTable", ordssrvs.Spec.PoolSettings[poolIndex].OwaDocTable) +
				conditionalEntry("plsql.gateway.mode", ordssrvs.Spec.PoolSettings[poolIndex].PlsqlGatewayMode) +
				conditionalEntry("security.jwt.profile.enabled", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileEnabled) +
				conditionalEntry("security.jwt.profile.mode", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileMode) +
				conditionalEntry("security.jwt.profile.audience", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileAudience) +
				conditionalEntry("security.jwt.profile.issuer", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileIssuer) +
				conditionalEntry("security.jwt.profile.jwk.url", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileJWKURL) +
				conditionalEntry("security.jwt.profile.role.claim.name", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileRoleClaimName) +
				conditionalEntry("security.jwt.profile.allowed.roles", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileAllowedRoles) +
				conditionalEntry("security.jwt.profile.allowed.scopes", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileAllowedScopes) +
				conditionalEntry("security.jwks.size", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSSize) +
				conditionalEntry("security.jwks.allowed.urls", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSAllowedURLs) +
				conditionalEntry("security.jwks.connection.timeout", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSConnectionTimeout) +
				conditionalEntry("security.jwks.read.timeout", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSReadTimeout) +
				conditionalEntry("security.jwks.refresh.interval", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSRefreshInterval) +
				conditionalEntry("security.jwt.allowed.skew", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTAllowedSkew) +
				conditionalEntry("security.jwt.allowed.age", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTAllowedAge) +
				conditionalEntry("db.connectionType", ordssrvs.Spec.PoolSettings[poolIndex].DBConnectionType) +
				conditionalEntry("db.customURL", ordssrvs.Spec.PoolSettings[poolIndex].DBCustomURL) +
				conditionalEntry("db.hostname", ordssrvs.Spec.PoolSettings[poolIndex].DBHostname) +
				conditionalEntry("db.port", ordssrvs.Spec.PoolSettings[poolIndex].DBPort) +
				conditionalEntry("db.servicename", ordssrvs.Spec.PoolSettings[poolIndex].DBServicename) +
				conditionalEntry("db.sid", ordssrvs.Spec.PoolSettings[poolIndex].DBSid) +
				conditionalEntry("db.tnsAliasName", ordssrvs.Spec.PoolSettings[poolIndex].DBTnsAliasName) +
				conditionalEntry("jdbc.DriverType", ordssrvs.Spec.PoolSettings[poolIndex].JDBCDriverType) +
				conditionalEntry("jdbc.InactivityTimeout", ordssrvs.Spec.PoolSettings[poolIndex].JDBCInactivityTimeout) +
				conditionalEntry("jdbc.InitialLimit", ordssrvs.Spec.PoolSettings[poolIndex].JDBCInitialLimit) +
				conditionalEntry("jdbc.MaxConnectionReuseCount", ordssrvs.Spec.PoolSettings[poolIndex].JDBCMaxConnectionReuseCount) +
				conditionalEntry("jdbc.MaxLimit", ordssrvs.Spec.PoolSettings[poolIndex].JDBCMaxLimit) +
				conditionalEntry("jdbc.auth.enabled", ordssrvs.Spec.PoolSettings[poolIndex].JDBCAuthEnabled) +
				conditionalEntry("jdbc.MaxStatementsLimit", ordssrvs.Spec.PoolSettings[poolIndex].JDBCMaxStatementsLimit) +
				conditionalEntry("jdbc.MinLimit", ordssrvs.Spec.PoolSettings[poolIndex].JDBCMinLimit) +
				conditionalEntry("jdbc.statementTimeout", ordssrvs.Spec.PoolSettings[poolIndex].JDBCStatementTimeout) +
				conditionalEntry("jdbc.MaxConnectionReuseTime", ordssrvs.Spec.PoolSettings[poolIndex].JDBCMaxConnectionReuseTime) +
				conditionalEntry("jdbc.SecondsToTrustIdleConnection", ordssrvs.Spec.PoolSettings[poolIndex].JDBCSecondsToTrustIdleConnection) +
				conditionalEntry("misc.defaultPage", ordssrvs.Spec.PoolSettings[poolIndex].MiscDefaultPage) +
				conditionalEntry("misc.pagination.maxRows", ordssrvs.Spec.PoolSettings[poolIndex].MiscPaginationMaxRows) +
				conditionalEntry("procedure.postProcess", ordssrvs.Spec.PoolSettings[poolIndex].ProcedurePostProcess) +
				conditionalEntry("procedure.preProcess", ordssrvs.Spec.PoolSettings[poolIndex].ProcedurePreProcess) +
				conditionalEntry("procedure.rest.preHook", ordssrvs.Spec.PoolSettings[poolIndex].ProcedureRestPreHook) +
				conditionalEntry("security.requestAuthenticationFunction", ordssrvs.Spec.PoolSettings[poolIndex].SecurityRequestAuthenticationFunction) +
				conditionalEntry("security.validationFunctionType", ordssrvs.Spec.PoolSettings[poolIndex].SecurityValidationFunctionType) +
				conditionalEntry("security.requestValidationFunction", ordssrvs.Spec.PoolSettings[poolIndex].SecurityRequestValidationFunction) +
				conditionalEntry("security.oauth.implicitGrantEnabled", ordssrvs.Spec.PoolSettings[poolIndex].SecurityOAuthImplicitGrantEnabled) +
				conditionalEntry("security.par.enabled", ordssrvs.Spec.PoolSettings[poolIndex].SecurityPAREnabled) +
				conditionalEntry("soda.defaultLimit", ordssrvs.Spec.PoolSettings[poolIndex].SODADefaultLimit) +
				conditionalEntry("soda.maxLimit", ordssrvs.Spec.PoolSettings[poolIndex].SODAMaxLimit) +
				conditionalEntry("restEnabledSql.active", ordssrvs.Spec.PoolSettings[poolIndex].RestEnabledSQLActive) +
				conditionalEntry("db.wallet.zip.service", ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletService) +
				tnsadminEntry +
				zipWalletPathEntry +
				sharedZipWalletEntry +
				`</properties>`),
		}
	}

	// ConfigMap do not have specific additionalLabels/additionalAnnotations
	labels := getSystemCommonLabels(ordssrvs, rState)
	annotations := getSystemCommonAnnotations(ordssrvs, rState)
	objectMeta := objectMetaDefine(ordssrvs, configMapName, labels, annotations)
	def := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: objectMeta,
		Data:       defData,
	}

	if err := ctrl.SetControllerReference(ordssrvs, def, r.Scheme); err != nil {
		return nil, fmt.Errorf("set owner reference for configmap %s/%s: %w", ordssrvs.Namespace, def.Name, err)
	}
	return def, nil

}

func escapeXMLText(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

func conditionalEntry(key string, value interface{}) string {
	if value == nil {
		return ""
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		value = rv.Elem().Interface()
	}

	content := fmt.Sprintf("%v", value)
	if s, ok := value.(string); ok {
		if s == "" {
			return ""
		}
		content = s
	}

	return fmt.Sprintf(`  <entry key="%s">%s</entry>`+"\n", key, escapeXMLText(content))
}
