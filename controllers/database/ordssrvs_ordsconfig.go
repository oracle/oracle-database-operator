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
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

func (r *OrdsSrvsReconciler) ConfigMapDefine(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, configMapName string, poolIndex int) *corev1.ConfigMap {

	//log := ctrllog.FromContext(ctx).WithName("ConfigMapDefine")

	var defData map[string]string
	switch configMapName {
	case r.ordssrvsScriptsConfigMapName:
		defData = make(map[string]string)
		defData["ords_init.sh"] = readScript(ctx, "/ordssrvs/ords_init.sh")
		defData["ords_start.sh"] = readScript(ctx, "/ordssrvs/ords_start.sh")
		defData["RSADecryptOAEP.java"] = readScript(ctx, "/ordssrvs/RSADecryptOAEP.java")
	case r.ordssrvsGlobalSettingsConfigMapName:
		// GlobalConfigMap
		var defStandaloneAccessLog string
		if ordssrvs.Spec.GlobalSettings.EnableStandaloneAccessLog {
			defStandaloneAccessLog = `  <entry key="standalone.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defMongoAccessLog string
		if ordssrvs.Spec.GlobalSettings.EnableMongoAccessLog {
			defMongoAccessLog = `  <entry key="mongo.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defCert string
		if ordssrvs.Spec.GlobalSettings.CertSecret != nil {
			defCert = `  <entry key="standalone.https.cert">` + ordsSABase + `/config/certficate/` + ordssrvs.Spec.GlobalSettings.CertSecret.Certificate + `</entry>` + "\n" +
				`  <entry key="standalone.https.cert.key">` + ordsSABase + `/config/certficate/` + ordssrvs.Spec.GlobalSettings.CertSecret.CertificateKey + `</entry>` + "\n"
		}
		defData = map[string]string{
			"settings.xml": fmt.Sprint(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
				`<!DOCTYPE properties SYSTEM "http://java.sun.com/dtd/properties.dtd">` + "\n" +
				`<properties>` + "\n" +
				conditionalEntry("cache.metadata.graphql.expireAfterAccess", ordssrvs.Spec.GlobalSettings.CacheMetadataGraphQLExpireAfterAccess) +
				conditionalEntry("cache.metadata.jwks.enabled", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSEnabled) +
				conditionalEntry("cache.metadata.jwks.initialCapacity", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSInitialCapacity) +
				conditionalEntry("cache.metadata.jwks.maximumSize", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSMaximumSize) +
				conditionalEntry("cache.metadata.jwks.expireAfterAccess", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterAccess) +
				conditionalEntry("cache.metadata.jwks.expireAfterWrite", ordssrvs.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterWrite) +
				conditionalEntry("database.api.management.services.disabled", ordssrvs.Spec.GlobalSettings.DatabaseAPIManagementServicesDisabled) +
				conditionalEntry("db.invalidPoolTimeout", ordssrvs.Spec.GlobalSettings.DBInvalidPoolTimeout) +
				conditionalEntry("feature.graphql.max.nesting.depth", ordssrvs.Spec.GlobalSettings.FeatureGraphQLMaxNestingDepth) +
				conditionalEntry("request.traceHeaderName", ordssrvs.Spec.GlobalSettings.RequestTraceHeaderName) +
				conditionalEntry("security.credentials.attempts", ordssrvs.Spec.GlobalSettings.SecurityCredentialsAttempts) +
				conditionalEntry("security.credentials.lock.time", ordssrvs.Spec.GlobalSettings.SecurityCredentialsLockTime) +
				conditionalEntry("standalone.context.path", ordssrvs.Spec.GlobalSettings.StandaloneContextPath) +
				conditionalEntry("standalone.http.port", ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort) +
				conditionalEntry("standalone.https.host", ordssrvs.Spec.GlobalSettings.StandaloneHTTPSHost) +
				conditionalEntry("standalone.https.port", ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort) +
				conditionalEntry("standalone.stop.timeout", ordssrvs.Spec.GlobalSettings.StandaloneStopTimeout) +
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
				conditionalEntry("security.disableDefaultExclusionList", ordssrvs.Spec.GlobalSettings.SecurityDisableDefaultExclusionList) +
				conditionalEntry("security.exclusionList", ordssrvs.Spec.GlobalSettings.SecurityExclusionList) +
				conditionalEntry("security.inclusionList", ordssrvs.Spec.GlobalSettings.SecurityInclusionList) +
				conditionalEntry("security.maxEntries", ordssrvs.Spec.GlobalSettings.SecurityMaxEntries) +
				conditionalEntry("security.verifySSL", ordssrvs.Spec.GlobalSettings.SecurityVerifySSL) +
				conditionalEntry("security.httpsHeaderCheck", ordssrvs.Spec.GlobalSettings.SecurityHTTPSHeaderCheck) +
				conditionalEntry("security.forceHTTPS", ordssrvs.Spec.GlobalSettings.SecurityForceHTTPS) +
				conditionalEntry("externalSessionTrustedOrigins", ordssrvs.Spec.GlobalSettings.SecuirtyExternalSessionTrustedOrigins) +
				`  <entry key="standalone.doc.root">` + ordsSABase + `/config/global/doc_root/</entry>` + "\n" +
				// Dynamic
				defStandaloneAccessLog +
				defMongoAccessLog +
				defCert +
				// Disabled (but not forgotten)
				// conditionalEntry("standalone.binds", ords.Spec.GlobalSettings.StandaloneBinds) +
				// conditionalEntry("error.externalPath", ords.Spec.GlobalSettings.ErrorExternalPath) +
				// conditionalEntry("security.credentials.file ", ords.Spec.GlobalSettings.SecurityCredentialsFile) +
				// conditionalEntry("standalone.static.path", ords.Spec.GlobalSettings.StandaloneStaticPath) +
				// conditionalEntry("standalone.doc.root", ords.Spec.GlobalSettings.StandaloneDocRoot) +
				// conditionalEntry("standalone.static.context.path", ords.Spec.GlobalSettings.StandaloneStaticContextPath) +
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
		tnsadminEntry:=conditionalEntry("db.tnsDirectory", ordsSABase + "/config/databases/" + poolName + "/network/admin/");
		
		// Pool Zip Wallet
		var zipWalletPathEntry string
		if ordssrvs.Spec.PoolSettings[poolIndex].DBWalletSecret != nil {
			tnsadminEntry="";
			zipWalletPathEntry = conditionalEntry("db.wallet.zip.path", ordsSABase + "/config/databases/" + poolName + "/network/admin/" + ordssrvs.Spec.PoolSettings[poolIndex].DBWalletSecret.WalletName );
		} 

		// Shared Zip Wallets
		// using shared zip wallet in fixed path /opt/oracle/sa/zipwallets
		sharedZipWalletEntry:=""
		if ordssrvs.Spec.GlobalSettings.ZipWalletsSecretName != "" && ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletName != "" {
		  tnsadminEntry="";
		  sharedZipWalletEntry=conditionalEntry("db.wallet.zip.path", "/opt/oracle/sa/zipwallets/"+ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletName);
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
				conditionalEntry("db.credentialsSource", ordssrvs.Spec.PoolSettings[poolIndex].DBCredentialsSource) +
				conditionalEntry("db.poolDestroyTimeout", ordssrvs.Spec.PoolSettings[poolIndex].DBPoolDestroyTimeout) +
				conditionalEntry("debug.trackResources", ordssrvs.Spec.PoolSettings[poolIndex].DebugTrackResources) +
				conditionalEntry("feature.openservicebroker.exclude", ordssrvs.Spec.PoolSettings[poolIndex].FeatureOpenservicebrokerExclude) +
				conditionalEntry("feature.sdw", ordssrvs.Spec.PoolSettings[poolIndex].FeatureSDW) +
				conditionalEntry("http.cookie.filter", ordssrvs.Spec.PoolSettings[poolIndex].HttpCookieFilter) +
				conditionalEntry("jdbc.auth.admin.role", ordssrvs.Spec.PoolSettings[poolIndex].JDBCAuthAdminRole) +
				conditionalEntry("jdbc.cleanup.mode", ordssrvs.Spec.PoolSettings[poolIndex].JDBCCleanupMode) +
				conditionalEntry("owa.trace.sql", ordssrvs.Spec.PoolSettings[poolIndex].OwaTraceSql) +
				conditionalEntry("plsql.gateway.mode", ordssrvs.Spec.PoolSettings[poolIndex].PlsqlGatewayMode) +
				conditionalEntry("security.jwt.profile.enabled", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWTProfileEnabled) +
				conditionalEntry("security.jwks.size", ordssrvs.Spec.PoolSettings[poolIndex].SecurityJWKSSize) +
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
				conditionalEntry("security.requestValidationFunction", ordssrvs.Spec.PoolSettings[poolIndex].SecurityRequestValidationFunction) +
				conditionalEntry("soda.defaultLimit", ordssrvs.Spec.PoolSettings[poolIndex].SODADefaultLimit) +
				conditionalEntry("soda.maxLimit", ordssrvs.Spec.PoolSettings[poolIndex].SODAMaxLimit) +
				conditionalEntry("restEnabledSql.active", ordssrvs.Spec.PoolSettings[poolIndex].RestEnabledSqlActive) +
				conditionalEntry("db.wallet.zip.service", ordssrvs.Spec.PoolSettings[poolIndex].ZipWalletService) +
				tnsadminEntry +
				zipWalletPathEntry + 
				sharedZipWalletEntry +
				// Disabled (but not forgotten)
				// conditionalEntry("autoupgrade.api.aulocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIAulocation) +
				// conditionalEntry("autoupgrade.api.enabled", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIEnabled) +
				// conditionalEntry("autoupgrade.api.jvmlocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIJvmlocation) +
				// conditionalEntry("autoupgrade.api.loglocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPILoglocation) +
				// conditionalEntry("db.serviceNameSuffix", ords.Spec.PoolSettings[poolIndex].DBServiceNameSuffix) +
				`</properties>`),
		}
	}

	objectMeta := objectMetaDefine(ordssrvs, configMapName)
	def := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: objectMeta,
		Data:       defData,
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ordssrvs, def, r.Scheme); err != nil {
		return nil
	}
	return def
}

func conditionalEntry(key string, value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		if v != "" {
			return fmt.Sprintf(`  <entry key="%s">%s</entry>`+"\n", key, v)
		}
	case *int32:
		if v != nil {
			return fmt.Sprintf(`  <entry key="%s">%d</entry>`+"\n", key, *v)
		}
	case *bool:
		if v != nil {
			return fmt.Sprintf(`  <entry key="%s">%v</entry>`+"\n", key, *v)
		}
	case *time.Duration:
		if v != nil {
			return fmt.Sprintf(`  <entry key="%s">%v</entry>`+"\n", key, *v)
		}
	default:
		return fmt.Sprintf(`  <entry key="%s">%v</entry>`+"\n", key, v)
	}
	return ""
}
