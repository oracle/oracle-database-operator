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

func (r *OrdsSrvsReconciler) ConfigMapDefine(ctx context.Context, ords *dbapi.OrdsSrvs, configMapName string, poolIndex int) *corev1.ConfigMap {

	log := ctrllog.FromContext(ctx).WithName("ConfigMapDefine")

	var defData map[string]string
	switch configMapName {
	case ords.Name + "-init-script":
		// Read the file from controller's filesystem
		filePath := "/ords_init.sh"
		scriptData, err := os.ReadFile(filePath)
		if err != nil {
			log.Error(err, "Error reading /ords_init.sh")
			return nil
		}
		log.Info("adding ords_init.sh")
		defData = map[string]string{
			"ords_init.sh": string(scriptData)}
	case ords.Name + "-start-script":
		// Read the file from controller's filesystem
		filePath := "/ords_start.sh"
		scriptData, err := os.ReadFile(filePath)
		if err != nil {
			log.Error(err, "Error reading /ords_start.sh")
			return nil
		}
		log.Info("adding ords_start.sh")
		defData = map[string]string{
			"ords_start.sh": string(scriptData)}
	case ords.Name + "-" + globalConfigMapName:
		// GlobalConfigMap
		var defStandaloneAccessLog string
		if ords.Spec.GlobalSettings.EnableStandaloneAccessLog {
			defStandaloneAccessLog = `  <entry key="standalone.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defMongoAccessLog string
		if ords.Spec.GlobalSettings.EnableMongoAccessLog {
			defMongoAccessLog = `  <entry key="mongo.access.log">` + ordsSABase + `/log/global</entry>` + "\n"
		}
		var defCert string
		if ords.Spec.GlobalSettings.CertSecret != nil {
			defCert = `  <entry key="standalone.https.cert">` + ordsSABase + `/config/certficate/` + ords.Spec.GlobalSettings.CertSecret.Certificate + `</entry>` + "\n" +
				`  <entry key="standalone.https.cert.key">` + ordsSABase + `/config/certficate/` + ords.Spec.GlobalSettings.CertSecret.CertificateKey + `</entry>` + "\n"
		}
		defData = map[string]string{
			"settings.xml": fmt.Sprint(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
				`<!DOCTYPE properties SYSTEM "http://java.sun.com/dtd/properties.dtd">` + "\n" +
				`<properties>` + "\n" +
				conditionalEntry("cache.metadata.graphql.expireAfterAccess", ords.Spec.GlobalSettings.CacheMetadataGraphQLExpireAfterAccess) +
				conditionalEntry("cache.metadata.jwks.enabled", ords.Spec.GlobalSettings.CacheMetadataJWKSEnabled) +
				conditionalEntry("cache.metadata.jwks.initialCapacity", ords.Spec.GlobalSettings.CacheMetadataJWKSInitialCapacity) +
				conditionalEntry("cache.metadata.jwks.maximumSize", ords.Spec.GlobalSettings.CacheMetadataJWKSMaximumSize) +
				conditionalEntry("cache.metadata.jwks.expireAfterAccess", ords.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterAccess) +
				conditionalEntry("cache.metadata.jwks.expireAfterWrite", ords.Spec.GlobalSettings.CacheMetadataJWKSExpireAfterWrite) +
				conditionalEntry("database.api.management.services.disabled", ords.Spec.GlobalSettings.DatabaseAPIManagementServicesDisabled) +
				conditionalEntry("db.invalidPoolTimeout", ords.Spec.GlobalSettings.DBInvalidPoolTimeout) +
				conditionalEntry("feature.graphql.max.nesting.depth", ords.Spec.GlobalSettings.FeatureGraphQLMaxNestingDepth) +
				conditionalEntry("request.traceHeaderName", ords.Spec.GlobalSettings.RequestTraceHeaderName) +
				conditionalEntry("security.credentials.attempts", ords.Spec.GlobalSettings.SecurityCredentialsAttempts) +
				conditionalEntry("security.credentials.lock.time", ords.Spec.GlobalSettings.SecurityCredentialsLockTime) +
				conditionalEntry("standalone.context.path", ords.Spec.GlobalSettings.StandaloneContextPath) +
				conditionalEntry("standalone.http.port", ords.Spec.GlobalSettings.StandaloneHTTPPort) +
				conditionalEntry("standalone.https.host", ords.Spec.GlobalSettings.StandaloneHTTPSHost) +
				conditionalEntry("standalone.https.port", ords.Spec.GlobalSettings.StandaloneHTTPSPort) +
				conditionalEntry("standalone.stop.timeout", ords.Spec.GlobalSettings.StandaloneStopTimeout) +
				conditionalEntry("cache.metadata.timeout", ords.Spec.GlobalSettings.CacheMetadataTimeout) +
				conditionalEntry("cache.metadata.enabled", ords.Spec.GlobalSettings.CacheMetadataEnabled) +
				conditionalEntry("database.api.enabled", ords.Spec.GlobalSettings.DatabaseAPIEnabled) +
				conditionalEntry("debug.printDebugToScreen", ords.Spec.GlobalSettings.DebugPrintDebugToScreen) +
				conditionalEntry("error.responseFormat", ords.Spec.GlobalSettings.ErrorResponseFormat) +
				conditionalEntry("icap.port", ords.Spec.GlobalSettings.ICAPPort) +
				conditionalEntry("icap.secure.port", ords.Spec.GlobalSettings.ICAPSecurePort) +
				conditionalEntry("icap.server", ords.Spec.GlobalSettings.ICAPServer) +
				conditionalEntry("log.procedure", ords.Spec.GlobalSettings.LogProcedure) +
				conditionalEntry("mongo.enabled", ords.Spec.GlobalSettings.MongoEnabled) +
				conditionalEntry("mongo.port", ords.Spec.GlobalSettings.MongoPort) +
				conditionalEntry("mongo.idle.timeout", ords.Spec.GlobalSettings.MongoIdleTimeout) +
				conditionalEntry("mongo.op.timeout", ords.Spec.GlobalSettings.MongoOpTimeout) +
				conditionalEntry("security.disableDefaultExclusionList", ords.Spec.GlobalSettings.SecurityDisableDefaultExclusionList) +
				conditionalEntry("security.exclusionList", ords.Spec.GlobalSettings.SecurityExclusionList) +
				conditionalEntry("security.inclusionList", ords.Spec.GlobalSettings.SecurityInclusionList) +
				conditionalEntry("security.maxEntries", ords.Spec.GlobalSettings.SecurityMaxEntries) +
				conditionalEntry("security.verifySSL", ords.Spec.GlobalSettings.SecurityVerifySSL) +
				conditionalEntry("security.httpsHeaderCheck", ords.Spec.GlobalSettings.SecurityHTTPSHeaderCheck) +
				conditionalEntry("security.forceHTTPS", ords.Spec.GlobalSettings.SecurityForceHTTPS) +
				conditionalEntry("externalSessionTrustedOrigins", ords.Spec.GlobalSettings.SecuirtyExternalSessionTrustedOrigins) +
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
		poolName := strings.ToLower(ords.Spec.PoolSettings[poolIndex].PoolName)
		var defDBNetworkPath string
		if ords.Spec.PoolSettings[poolIndex].DBWalletSecret != nil {
			defDBNetworkPath = `  <entry key="db.wallet.zip.path">` + ordsSABase + `/config/databases/` + poolName + `/network/admin/` + ords.Spec.PoolSettings[poolIndex].DBWalletSecret.WalletName + `</entry>` + "\n" +
				conditionalEntry("db.wallet.zip.service", strings.ToUpper(ords.Spec.PoolSettings[poolIndex].DBWalletZipService)) + "\n"
		} else {
			defDBNetworkPath = `  <entry key="db.tnsDirectory">` + ordsSABase + `/config/databases/` + poolName + `/network/admin/</entry>` + "\n"
		}
		defData = map[string]string{
			"pool.xml": fmt.Sprint(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
				`<!DOCTYPE properties SYSTEM "http://java.sun.com/dtd/properties.dtd">` + "\n" +
				`<properties>` + "\n" +
				`  <entry key="db.username">` + ords.Spec.PoolSettings[poolIndex].DBUsername + `</entry>` + "\n" +
				conditionalEntry("db.adminUser", ords.Spec.PoolSettings[poolIndex].DBAdminUser) +
				conditionalEntry("db.cdb.adminUser", ords.Spec.PoolSettings[poolIndex].DBCDBAdminUser) +
				conditionalEntry("apex.security.administrator.roles", ords.Spec.PoolSettings[poolIndex].ApexSecurityAdministratorRoles) +
				conditionalEntry("apex.security.user.roles", ords.Spec.PoolSettings[poolIndex].ApexSecurityUserRoles) +
				conditionalEntry("db.credentialsSource", ords.Spec.PoolSettings[poolIndex].DBCredentialsSource) +
				conditionalEntry("db.poolDestroyTimeout", ords.Spec.PoolSettings[poolIndex].DBPoolDestroyTimeout) +
				conditionalEntry("debug.trackResources", ords.Spec.PoolSettings[poolIndex].DebugTrackResources) +
				conditionalEntry("feature.openservicebroker.exclude", ords.Spec.PoolSettings[poolIndex].FeatureOpenservicebrokerExclude) +
				conditionalEntry("feature.sdw", ords.Spec.PoolSettings[poolIndex].FeatureSDW) +
				conditionalEntry("http.cookie.filter", ords.Spec.PoolSettings[poolIndex].HttpCookieFilter) +
				conditionalEntry("jdbc.auth.admin.role", ords.Spec.PoolSettings[poolIndex].JDBCAuthAdminRole) +
				conditionalEntry("jdbc.cleanup.mode", ords.Spec.PoolSettings[poolIndex].JDBCCleanupMode) +
				conditionalEntry("owa.trace.sql", ords.Spec.PoolSettings[poolIndex].OwaTraceSql) +
				conditionalEntry("plsql.gateway.mode", ords.Spec.PoolSettings[poolIndex].PlsqlGatewayMode) +
				conditionalEntry("security.jwt.profile.enabled", ords.Spec.PoolSettings[poolIndex].SecurityJWTProfileEnabled) +
				conditionalEntry("security.jwks.size", ords.Spec.PoolSettings[poolIndex].SecurityJWKSSize) +
				conditionalEntry("security.jwks.connection.timeout", ords.Spec.PoolSettings[poolIndex].SecurityJWKSConnectionTimeout) +
				conditionalEntry("security.jwks.read.timeout", ords.Spec.PoolSettings[poolIndex].SecurityJWKSReadTimeout) +
				conditionalEntry("security.jwks.refresh.interval", ords.Spec.PoolSettings[poolIndex].SecurityJWKSRefreshInterval) +
				conditionalEntry("security.jwt.allowed.skew", ords.Spec.PoolSettings[poolIndex].SecurityJWTAllowedSkew) +
				conditionalEntry("security.jwt.allowed.age", ords.Spec.PoolSettings[poolIndex].SecurityJWTAllowedAge) +
				conditionalEntry("db.connectionType", ords.Spec.PoolSettings[poolIndex].DBConnectionType) +
				conditionalEntry("db.customURL", ords.Spec.PoolSettings[poolIndex].DBCustomURL) +
				conditionalEntry("db.hostname", ords.Spec.PoolSettings[poolIndex].DBHostname) +
				conditionalEntry("db.port", ords.Spec.PoolSettings[poolIndex].DBPort) +
				conditionalEntry("db.servicename", ords.Spec.PoolSettings[poolIndex].DBServicename) +
				conditionalEntry("db.sid", ords.Spec.PoolSettings[poolIndex].DBSid) +
				conditionalEntry("db.tnsAliasName", ords.Spec.PoolSettings[poolIndex].DBTnsAliasName) +
				conditionalEntry("jdbc.DriverType", ords.Spec.PoolSettings[poolIndex].JDBCDriverType) +
				conditionalEntry("jdbc.InactivityTimeout", ords.Spec.PoolSettings[poolIndex].JDBCInactivityTimeout) +
				conditionalEntry("jdbc.InitialLimit", ords.Spec.PoolSettings[poolIndex].JDBCInitialLimit) +
				conditionalEntry("jdbc.MaxConnectionReuseCount", ords.Spec.PoolSettings[poolIndex].JDBCMaxConnectionReuseCount) +
				conditionalEntry("jdbc.MaxLimit", ords.Spec.PoolSettings[poolIndex].JDBCMaxLimit) +
				conditionalEntry("jdbc.auth.enabled", ords.Spec.PoolSettings[poolIndex].JDBCAuthEnabled) +
				conditionalEntry("jdbc.MaxStatementsLimit", ords.Spec.PoolSettings[poolIndex].JDBCMaxStatementsLimit) +
				conditionalEntry("jdbc.MinLimit", ords.Spec.PoolSettings[poolIndex].JDBCMinLimit) +
				conditionalEntry("jdbc.statementTimeout", ords.Spec.PoolSettings[poolIndex].JDBCStatementTimeout) +
				conditionalEntry("jdbc.MaxConnectionReuseTime", ords.Spec.PoolSettings[poolIndex].JDBCMaxConnectionReuseTime) +
				conditionalEntry("jdbc.SecondsToTrustIdleConnection", ords.Spec.PoolSettings[poolIndex].JDBCSecondsToTrustIdleConnection) +
				conditionalEntry("misc.defaultPage", ords.Spec.PoolSettings[poolIndex].MiscDefaultPage) +
				conditionalEntry("misc.pagination.maxRows", ords.Spec.PoolSettings[poolIndex].MiscPaginationMaxRows) +
				conditionalEntry("procedure.postProcess", ords.Spec.PoolSettings[poolIndex].ProcedurePostProcess) +
				conditionalEntry("procedure.preProcess", ords.Spec.PoolSettings[poolIndex].ProcedurePreProcess) +
				conditionalEntry("procedure.rest.preHook", ords.Spec.PoolSettings[poolIndex].ProcedureRestPreHook) +
				conditionalEntry("security.requestAuthenticationFunction", ords.Spec.PoolSettings[poolIndex].SecurityRequestAuthenticationFunction) +
				conditionalEntry("security.requestValidationFunction", ords.Spec.PoolSettings[poolIndex].SecurityRequestValidationFunction) +
				conditionalEntry("soda.defaultLimit", ords.Spec.PoolSettings[poolIndex].SODADefaultLimit) +
				conditionalEntry("soda.maxLimit", ords.Spec.PoolSettings[poolIndex].SODAMaxLimit) +
				conditionalEntry("restEnabledSql.active", ords.Spec.PoolSettings[poolIndex].RestEnabledSqlActive) +
				defDBNetworkPath +
				// Disabled (but not forgotten)
				// conditionalEntry("autoupgrade.api.aulocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIAulocation) +
				// conditionalEntry("autoupgrade.api.enabled", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIEnabled) +
				// conditionalEntry("autoupgrade.api.jvmlocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPIJvmlocation) +
				// conditionalEntry("autoupgrade.api.loglocation", ords.Spec.PoolSettings[poolIndex].AutoupgradeAPILoglocation) +
				// conditionalEntry("db.serviceNameSuffix", ords.Spec.PoolSettings[poolIndex].DBServiceNameSuffix) +
				`</properties>`),
		}
	}

	objectMeta := objectMetaDefine(ords, configMapName)
	def := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: objectMeta,
		Data:       defData,
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(ords, def, r.Scheme); err != nil {
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
