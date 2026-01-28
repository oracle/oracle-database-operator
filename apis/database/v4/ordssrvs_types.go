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

package v4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OrdsSrvsSpec defines the desired state of OrdsSrvs
// +kubebuilder:resource:shortName="ords"
type OrdsSrvsSpec struct {
	
	// Specifies the desired Kubernetes Workload
	//+kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet
	//+kubebuilder:default=Deployment
	WorkloadType string `json:"workloadType,omitempty"`
	
	// Defines the number of desired Replicas when workloadType is Deployment or StatefulSet
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`
	
	// Specifies whether to restart pods when Global or Pool configurations change
	ForceRestart bool `json:"forceRestart,omitempty"`
	
	// Specifies the ORDS container image
	//+kubecbuilder:default=container-registry.oracle.com/database/ords:latest
	Image string `json:"image"`
	
	// Specifies the ORDS container image pull policy
	//+kubebuilder:validation:Enum=IfNotPresent;Always;Never
	//+kubebuilder:default=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	
	// Specifies the Secret Name for pulling the ORDS container image
	ImagePullSecrets string `json:"imagePullSecrets,omitempty"`
	
	// Contains settings that are configured across the entire ORDS instance.
	//+kubebuilder:default:={}
	GlobalSettings GlobalSettings `json:"globalSettings,omitempty"`
	
	// Private key
	EncPrivKey   PasswordSecret  `json:"encPrivKey,omitempty"`
	
	// Contains settings for individual pools/databases
	PoolSettings []*PoolSettings `json:"poolSettings,omitempty"`

	// ServiceAccount of the OrdsSrvs Pod
	// +k8s:openapi-gen=true
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

}

type GlobalSettings struct {

	// Specifies whether the Instance API is enabled.
	InstanceAPIEnabled *bool `json:"instance.api.enabled,omitempty"`

	// Specifies the setting to enable or disable metadata caching.
	CacheMetadataEnabled *bool `json:"cache.metadata.enabled,omitempty"`

	// Specifies the duration after a GraphQL schema is not accessed from the cache that it expires.
	CacheMetadataGraphQLExpireAfterAccess string `json:"cache.metadata.graphql.expireAfterAccess,omitempty"`

	// Specifies the duration after a GraphQL schema is cached that it expires and has to be loaded again.
	CacheMetadataGraphQLExpireAfterWrite string `json:"cache.metadata.graphql.expireAfterWrite,omitempty"`

	// Specifies the setting to determine for how long a metadata record remains in the cache.
	// Longer duration means, it takes longer to view the applied changes.
	CacheMetadataTimeout string `json:"cache.metadata.timeout,omitempty"`

	// Specifies the setting to enable or disable JWKS caching.
	CacheMetadataJWKSEnabled *bool `json:"cache.metadata.jwks.enabled,omitempty"`

	// Specifies the initial capacity of the JWKS cache.
	CacheMetadataJWKSInitialCapacity *int32 `json:"cache.metadata.jwks.initialCapacity,omitempty"`

	// Specifies the maximum capacity of the JWKS cache.
	CacheMetadataJWKSMaximumSize *int32 `json:"cache.metadata.jwks.maximumSize,omitempty"`

	// Specifies the duration after a JWK is not accessed from the cache that it expires.
	// By default this is disabled.
	CacheMetadataJWKSExpireAfterAccess string `json:"cache.metadata.jwks.expireAfterAccess,omitempty"`

	// Specifies the duration after a JWK is cached, that is, it expires and has to be loaded again.
	CacheMetadataJWKSExpireAfterWrite string `json:"cache.metadata.jwks.expireAfterWrite,omitempty"`

	// Specifies whether the Database API is enabled.
	DatabaseAPIEnabled *bool `json:"database.api.enabled,omitempty"`

	// Specifies to disable the Database API administration related services.
	// Only applicable when Database API is enabled.
	DatabaseAPIManagementServicesDisabled *bool `json:"database.api.management.services.disabled,omitempty"`

	// Specifies how long to wait before retrying an invalid pool.
	DBInvalidPoolTimeout string `json:"db.invalidPoolTimeout,omitempty"`

	// Specifies the maximum join nesting depth limit for GraphQL queries.
	FeatureGraphQLMaxNestingDepth *int32 `json:"feature.grahpql.max.nesting.depth,omitempty"`

	// Specifies the name of the HTTP request header that uniquely identifies the request end to end as
	// it passes through the various layers of the application stack.
	// In Oracle this header is commonly referred to as the ECID (Entity Context ID).
	RequestTraceHeaderName string `json:"request.traceHeaderName,omitempty"`

	// Specifies the maximum number of unsuccessful password attempts allowed.
	// Enabled by setting a positive integer value.
	SecurityCredentialsAttempts *int32 `json:"security.credentials.attempts,omitempty"`

	// Specifies the period to lock the account that has exceeded maximum attempts.
	SecurityCredentialsLockTime string `json:"security.credentials.lock.time,omitempty"`

	// Specifies the HTTP listen port.
	//+kubebuilder:default:=8080
	StandaloneHTTPPort *int32 `json:"standalone.http.port,omitempty"`

	// Specifies the SSL certificate hostname.
	StandaloneHTTPSHost string `json:"standalone.https.host,omitempty"`

	// Specifies the HTTPS listen port.
	//+kubebuilder:default:=8443
	StandaloneHTTPSPort *int32 `json:"standalone.https.port,omitempty"`

	// Specifies the period for Standalone Mode to wait until it is gracefully shutdown.
	StandaloneStopTimeout string `json:"standalone.stop.timeout,omitempty"`

	// Specifies whether to display error messages on the browser.
	DebugPrintDebugToScreen *bool `json:"debug.printDebugToScreen,omitempty"`

	// Specifies how the HTTP error responses must be formatted.
	// html - Force all responses to be in HTML format
	// json - Force all responses to be in JSON format
	// auto - Automatically determines most appropriate format for the request (default).
	ErrorResponseFormat string `json:"error.responseFormat,omitempty"`

	// Specifies the Internet Content Adaptation Protocol (ICAP) port to virus scan files.
	// Either icap.port or icap.secure.port are required to have a value.
	ICAPPort *int32 `json:"icap.port,omitempty"`

	// Specifies the Internet Content Adaptation Protocol (ICAP) port to virus scan files.
	// Either icap.port or icap.secure.port are required to have a value.
	// If values for both icap.port and icap.secure.port are provided, then the value of icap.port is ignored.
	ICAPSecurePort *int32 `json:"icap.secure.port,omitempty"`

	// Specifies the Internet Content Adaptation Protocol (ICAP) server name or IP address to virus scan files.
	// The icap.server is required to have a value.
	ICAPServer string `json:"icap.server,omitempty"`

	// Specifies whether procedures are to be logged.
	LogProcedure bool `json:"log.procedure,omitempty"`

	// Specifies to enable the API for MongoDB.
	//+kubebuider:default=false
	MongoEnabled bool `json:"mongo.enabled,omitempty"`

	// Specifies the API for MongoDB listen port.
	//+kubebuilder:default:=27017
	MongoPort *int32 `json:"mongo.port,omitempty"`

	// Specifies the maximum idle time for a Mongo connection in milliseconds.
	MongoIdleTimeout string `json:"mongo.idle.timeout,omitempty"`

	// Specifies the maximum time for a Mongo database operation in milliseconds.
	MongoOpTimeout string `json:"mongo.op.timeout,omitempty"`

	// If this value is set to true, then the Oracle REST Data Services internal exclusion list is not enforced.
	// Oracle recommends that you do not set this value to true.
	SecurityDisableDefaultExclusionList *bool `json:"security.disableDefaultExclusionList,omitempty"`

	// Specifies a pattern for procedures, packages, or schema names which are forbidden to be directly executed from a browser.
	SecurityExclusionList string `json:"security.exclusionList,omitempty"`

	// Specifies a pattern for procedures, packages, or schema names which are allowed to be directly executed from a browser.
	SecurityInclusionList string `json:"security.inclusionList,omitempty"`

	// Specifies the maximum number of cached procedure validations.
	// Set this value to 0 to force the validation procedure to be invoked on each request.
	SecurityMaxEntries *int32 `json:"security.maxEntries,omitempty"`

	// Specifies whether HTTPS is available in your environment.
	SecurityVerifySSL *bool `json:"security.verifySSL,omitempty"`

	// Specifies the context path where ords is located.
	//+kubebuilder:default:="/ords"
	StandaloneContextPath string `json:"standalone.context.path,omitempty"`

	// Specify whether to download APEX installation files
	// This setting will be ignored for ADB
	//+kubebuilder:default:=false
	APEXDownload bool `json:"apex.download,omitempty"`

	// Specify the url to download APEX installation files
	// This setting will be ignored for ADB
	//+kubebuilder:default:="https://download.oracle.com/otn_software/apex/apex-latest.zip"
	APEXDownloadUrl string `json:"apex.download.url,omitempty"`

	// Specify the storage attributes for PersistenceVolume and PersistenceVolumeClaim
	APEXInstallationPersistence Persistence `json:"apex.installation.persistence,omitempty"`

	// Central Configuration URL
	CentralConfigUrl string `json:"central.config.url,omitempty"`
	
	// Central Configuration Wallet
	//CentralConfigWallet string `json:"central.config.wallet,omitempty"`

	// Specifies the Secret containing one or more wallet.zip archives (whit different names) containing connection details and credentials for the pools.
	// shared zip wallet
	ZipWalletsSecretName string `json:"zipWalletsSecretName,omitempty"`

	/*************************************************
	* Undocumented
	/************************************************/

	// Specifies that the HTTP Header contains the specified text
	// Usually set to 'X-Forwarded-Proto: https' coming from a load-balancer
	SecurityHTTPSHeaderCheck string `json:"security.httpsHeaderCheck,omitempty"`

	// Specifies to force HTTPS; this is set to default to false as in real-world TLS should
	// terminiate at the LoadBalancer
	SecurityForceHTTPS bool `json:"security.forceHTTPS,omitempty"`

	// Specifies to trust Access from originating domains
	SecuirtyExternalSessionTrustedOrigins string `json:"security.externalSessionTrustedOrigins,omitempty"`

	/*************************************************
	* Customised
	/************************************************/
	/* Below are settings with physical path/file locations to be replaced by ConfigMaps/Secrets, Boolean or HardCoded */

	/*
		// Specifies the path to the folder to store HTTP request access logs.
		// If not specified, then no access log is generated.
		// HARDCODED
		// StandaloneAccessLog string `json:"standalone.access.log,omitempty"`
	*/

	// Specifies if HTTP request access logs should be enabled
	// If enabled, logs will be written to /opt/oracle/sa/log/global
	//+kubebuilder:default:=false
	EnableStandaloneAccessLog bool `json:"enable.standalone.access.log,omitempty"`

	// Specifies if HTTP request access logs should be enabled
	// If enabled, logs will be written to /opt/oracle/sa/log/global
	//+kubebuilder:default:=false
	EnableMongoAccessLog bool `json:"enable.mongo.access.log,omitempty"`

	/*
		//Specifies the SSL certificate path.
		// If you are providing the SSL certificate, then you must specify the certificate location.
		// Replaced with: CertSecret *CertificateSecret `json:"certSecret,omitempty"`
		//StandaloneHTTPSCert string `json:"standalone.https.cert"`

		// Specifies the SSL certificate key path.
		// If you are providing the SSL certificate, you must specify the certificate key location.
		// Replaced with: CertSecret *CertificateSecret `json:"certSecret,omitempty"`
		//StandaloneHTTPSCertKey string `json:"standalone.https.cert.key"`
	*/

	// Specifies the Secret containing the SSL Certificates
	// Replaces: standalone.https.cert and standalone.https.cert.key
	CertSecret *CertificateSecret `json:"certSecret,omitempty"`

	/*************************************************
	* Disabled
	/*************************************************
	// Specifies the comma separated list of host names or IP addresses to identify a specific network
	// interface on which to listen.
	//+kubebuilder:default:="0.0.0.0"
	//StandaloneBinds string `json:"standalone.binds,omitempty"`
	// This is disabled as containerised

	// Specifies the file where credentials are stored.
	//SecurityCredentialsFile string `json:"security.credentials.file,omitempty"`
	// WTF does this do?!?!

	// Points to the location where static resources to be served under the / root server path are located.
	// StandaloneDocRoot string `json:"standalone.doc.root,omitempty"`
	// Maybe this gets implemented; difficult to predict valid use case

	// Specifies the path to a folder that contains the custom error page.
	// ErrorExternalPath string `json:"error.externalPath,omitempty"`
	// Can see use-case; but wait for implementation

	// Specifies the Context path where APEX static resources are located.
	//+kubebuilder:default:="/i"
	// StandaloneStaticContextPath string `json:"standalone.static.context.path,omitempty"`
	// Does anyone ever change this?  If so, need to also change the APEX install configmap to update path
	*/

	// Specifies the path to the folder containing static resources required by APEX.
	// StandaloneStaticPath string `json:"standalone.static.path,omitempty"`
	// This is disabled as will use the container image path (/opt/oracle/apex/$ORDS_VER/images)
	// HARDCODED into the entrypoint

	// Specifies a comma separated list of host names or IP addresses to identify a specific
	// network interface on which to listen.
	//+kubebuilder:default:="0.0.0.0"
	// MongoHost string `json:"mongo.host,omitempty"`
	// This is disabled as containerised

	// Specifies the path to the folder where you want to store the API for MongoDB access logs.
	// MongoAccessLog string `json:"mongo.access.log,omitempty"`
	// HARDCODED to global/logs
}

// Specify storage attributes of PV and PVC
type Persistence struct {
	//+kubebuilder:default="2Gi"
	Size         string `json:"size,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	//+kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	//+kubebuilder:default=ReadWriteOnce
	AccessMode string `json:"accessMode,omitempty"`
	VolumeName string `json:"volumeName,omitempty"`
	//VolumeClaimAnnotation string `json:"volumeClaimAnnotation,omitempty"`
	//SetWritePermissions   *bool  `json:"setWritePermissions,omitempty"`
}

type PoolSettings struct {
	// Specifies the Pool Name
	PoolName string `json:"poolName"`

	// Specify whether to perform ORDS installation/upgrades automatically
	// The db.adminUser and db.adminUser.secret must be set, otherwise setting is ignored
	// This setting will be ignored for ADB
	//+kubebuilder:default:=false
	AutoUpgradeORDS bool `json:"autoUpgradeORDS,omitempty"`

	// Specify whether to perform APEX installation/upgrades automatically
	// The db.adminUser and db.adminUser.secret must be set, otherwise setting is ignored
	// This setting will be ignored for ADB
	//+kubebuilder:default:=false
	AutoUpgradeAPEX bool `json:"autoUpgradeAPEX,omitempty"`

	// Specifies the name of the database user for the connection.
	// For ADBs this must be specified and not ORDS_PUBLIC_USER
	// If ORDS_PUBLIC_USER is specified for an ADB, the workload will fail
	// db.username can be empty in case of SEPS zip wallets (Secure External Password Store, orapki credentials, connect /@TNSALIAS)
	DBUsername string `json:"db.username,omitempty"`

	// Specifies the Secret with the db password
	// for the connection.
	DBSecret PasswordSecret `json:"db.secret,omitempty"`

	// Specifies the username for the database account that ORDS uses for administration operations in the database.
	DBAdminUser string `json:"db.adminUser,omitempty"`

	// Specifies the password for the database account that ORDS uses for administration operations in the database.
	// Replaced by: DBAdminUserSecret PasswordSecret `json:"dbAdminUserSecret,omitempty"`
	// DBAdminUserPassword struct{} `json:"db.adminUser.password,omitempty"`

	// Specifies the Secret with the dbAdminUser (SYS) and dbAdminPassword values
	// for the database account that ORDS uses for administration operations in the database.
	// replaces: db.adminUser.password
	DBAdminUserSecret PasswordSecret `json:"db.adminUser.secret,omitempty"`

	// Specifies the username for the database account that ORDS uses for the Pluggable Database Lifecycle Management.
	DBCDBAdminUser string `json:"db.cdb.adminUser,omitempty"`

	// Specifies the password for the database account that ORDS uses for the Pluggable Database Lifecycle Management.
	// Replaced by: DBCdbAdminUserSecret PasswordSecret `json:"dbCdbAdminUserSecret,omitempty"`
	// DBCdbAdminUserPassword struct{} `json:"db.cdb.adminUser.password,omitempty"`

	// Specifies the Secret with the dbCdbAdminUser (SYS) and dbCdbAdminPassword values
	// Specifies the username for the database account that ORDS uses for the Pluggable Database Lifecycle Management.
	// Replaces: db.cdb.adminUser.password
	DBCDBAdminUserSecret PasswordSecret `json:"db.cdb.adminUser.secret,omitempty"`

	// Specifies the comma delimited list of additional roles to assign authenticated APEX administrator type users.
	ApexSecurityAdministratorRoles string `json:"apex.security.administrator.roles,omitempty"`

	// Specifies the comma delimited list of additional roles to assign authenticated regular APEX users.
	ApexSecurityUserRoles string `json:"apex.security.user.roles,omitempty"`

	// Specifies the source for database credentials when creating a direct connection for running SQL statements.
	// Value can be one of pool or request.
	// If the value is pool, then the credentials defined in this pool is used to create a JDBC connection.
	// If the value request is used, then the credentials in the request is used to create a JDBC connection and if successful, grants the requestor SQL Developer role.
	//+kubebuilder:validation:Enum=pool;request
	DBCredentialsSource string `json:"db.credentialsSource,omitempty"`

	// Indicates how long to wait to gracefully destroy a pool before moving to forcefully destroy all connections including borrowed ones.
	DBPoolDestroyTimeout string `json:"db.poolDestroyTimeout,omitempty"`

	// Specifies to enable tracking of JDBC resources.
	// If not released causes in resource leaks or exhaustion in the database.
	// Tracking imposes a performance overhead.
	DebugTrackResources *bool `json:"debug.trackResources,omitempty"`

	// Specifies to disable the Open Service Broker services available for the pool.
	FeatureOpenservicebrokerExclude *bool `json:"feature.openservicebroker.exclude,omitempty"`

	// Specifies to enable the Database Actions feature.
	FeatureSDW *bool `json:"feature.sdw,omitempty"`

	// Specifies a comma separated list of HTTP Cookies to exclude when initializing an Oracle Web Agent environment.
	HttpCookieFilter string `json:"http.cookie.filter,omitempty"`

	// Identifies the database role that indicates that the database user must get the SQL Administrator role.
	JDBCAuthAdminRole string `json:"jdbc.auth.admin.role,omitempty"`

	// Specifies how a pooled JDBC connection and corresponding database session, is released when a request has been processed.
	JDBCCleanupMode string `json:"jdbc.cleanup.mode,omitempty"`

	// If it is true, then it causes a trace of the SQL statements performed by Oracle Web Agent to be echoed to the log.
	OwaTraceSql *bool `json:"owa.trace.sql,omitempty"`

	// Indicates if the PL/SQL Gateway functionality should be available for a pool or not.
	// Value can be one of disabled, direct, or proxied.
	// If the value is direct, then the pool serves the PL/SQL Gateway requests directly.
	// If the value is proxied, the PLSQL_GATEWAY_CONFIG view is used to determine the user to whom to proxy.
	//+kubebuilder:validation:Enum=disabled;direct;proxied
	PlsqlGatewayMode string `json:"plsql.gateway.mode,omitempty"`

	// Specifies whether the JWT Profile authentication is available. Supported values:
	SecurityJWTProfileEnabled *bool `json:"security.jwt.profile.enabled,omitempty"`

	// Specifies the maximum number of bytes read from the JWK url.
	SecurityJWKSSize *int32 `json:"security.jwks.size,omitempty"`

	// Specifies the maximum amount of time before timing-out when accessing a JWK url.
	SecurityJWKSConnectionTimeout string `json:"security.jwks.connection.timeout,omitempty"`

	// Specifies the maximum amount of time reading a response from the JWK url before timing-out.
	SecurityJWKSReadTimeout string `json:"security.jwks.read.timeout,omitempty"`

	// Specifies the minimum interval between refreshing the JWK cached value.
	SecurityJWKSRefreshInterval string `json:"security.jwks.refresh.interval,omitempty"`

	// Specifies the maximum skew the JWT time claims are accepted.
	// This is useful if the clock on the JWT issuer and ORDS differs by a few seconds.
	SecurityJWTAllowedSkew string `json:"security.jwt.allowed.skew,omitempty"`

	// Specifies the maximum allowed age of a JWT in seconds, regardless of expired claim.
	// The age of the JWT is taken from the JWT issued at claim.
	SecurityJWTAllowedAge string `json:"security.jwt.allowed.age,omitempty"`

	// Indicates the type of security.requestValidationFunction: javascript or plsql.
	//+kubebuilder:validation:Enum=plsql;javascript
	SecurityValidationFunctionType string `json:"security.validationFunctionType,omitempty"`

	// The type of connection.
	//+kubebuilder:validation:Enum=basic;tns;customurl
	DBConnectionType string `json:"db.connectionType,omitempty"`

	// Specifies the JDBC URL connection to connect to the database.
	DBCustomURL string `json:"db.customURL,omitempty"`

	// Specifies the host system for the Oracle database.
	DBHostname string `json:"db.hostname,omitempty"`

	// Specifies the database listener port.
	DBPort *int32 `json:"db.port,omitempty"`

	// Specifies the network service name of the database.
	DBServicename string `json:"db.servicename,omitempty"`

	// Specifies the name of the database.
	DBSid string `json:"db.sid,omitempty"`

	// Specifies the TNS alias name that matches the name in the tnsnames.ora file.
	DBTnsAliasName string `json:"db.tnsAliasName,omitempty"`

	// Specifies the service name in the wallet archive for the pool.
	ZipWalletService string `json:"db.wallet.zip.service,omitempty"`

	// Specifies the name of the wallet archive inside the shared zip wallets, defined in ZipWalletsSecretName .
	ZipWalletName string `json:"zipWalletName,omitempty"`

	// Specifies the JDBC driver type.
	//+kubebuilder:validation:Enum=thin;oci8
	JDBCDriverType string `json:"jdbc.DriverType,omitempty"`

	// Specifies how long an available connection can remain idle before it is closed. The inactivity connection timeout is in seconds.
	JDBCInactivityTimeout *int32 `json:"jdbc.InactivityTimeout,omitempty"`

	// Specifies the initial size for the number of connections that will be created.
	// The default is low, and should probably be set higher in most production environments.
	JDBCInitialLimit *int32 `json:"jdbc.InitialLimit,omitempty"`

	// Specifies the maximum number of times to reuse a connection before it is discarded and replaced with a new connection.
	JDBCMaxConnectionReuseCount *int32 `json:"jdbc.MaxConnectionReuseCount,omitempty"`

	// Sets the maximum connection reuse time property.
	JDBCMaxConnectionReuseTime string `json:"jdbc.MaxConnectionReuseTime,omitempty"`

	// Sets the time in seconds to trust an idle connection to skip a validation test.
	JDBCSecondsToTrustIdleConnection *int32 `json:"jdbc.SecondsToTrustIdleConnection,omitempty"`

	// Specifies the maximum number of connections.
	// Might be too low for some production environments.
	JDBCMaxLimit *int32 `json:"jdbc.MaxLimit,omitempty"`

	// Specifies if the PL/SQL Gateway calls can be authenticated using database users.
	// If the value is true then this feature is enabled. If the value is false, then this feature is disabled.
	// Oracle recommends not to use this feature.
	// This feature used only to facilitate customers migrating from mod_plsql.
	JDBCAuthEnabled *bool `json:"jdbc.auth.enabled,omitempty"`

	// Specifies the maximum number of statements to cache for each connection.
	JDBCMaxStatementsLimit *int32 `json:"jdbc.MaxStatementsLimit,omitempty"`

	// Specifies the minimum number of connections.
	JDBCMinLimit *int32 `json:"jdbc.MinLimit,omitempty"`

	// Specifies a timeout period on a statement.
	// An abnormally long running query or script, executed by a request, may leave it in a hanging state unless a timeout is
	// set on the statement. Setting a timeout on the statement ensures that all the queries automatically timeout if
	// they are not completed within the specified time period.
	JDBCStatementTimeout *int32 `json:"jdbc.statementTimeout,omitempty"`

	// Specifies the default page to display. The Oracle REST Data Services Landing Page.
	MiscDefaultPage string `json:"misc.defaultPage,omitempty"`

	// Specifies the maximum number of rows that will be returned from a query when processing a RESTful service
	// and that will be returned from a nested cursor in a result set.
	// Affects all RESTful services generated through a SQL query, regardless of whether the resource is paginated.
	MiscPaginationMaxRows *int32 `json:"misc.pagination.maxRows,omitempty"`

	// Specifies the procedure name(s) to execute after executing the procedure specified on the URL.
	// Multiple procedure names must be separated by commas.
	ProcedurePostProcess string `json:"procedurePostProcess,omitempty"`

	// Specifies the procedure name(s) to execute prior to executing the procedure specified on the URL.
	// Multiple procedure names must be separated by commas.
	ProcedurePreProcess string `json:"procedure.preProcess,omitempty"`

	// Specifies the function to be invoked prior to dispatching each Oracle REST Data Services based REST Service.
	// The function can perform configuration of the database session, perform additional validation or authorization of the request.
	// If the function returns true, then processing of the request continues.
	// If the function returns false, then processing of the request is aborted and an HTTP 403 Forbidden status is returned.
	ProcedureRestPreHook string `json:"procedure.rest.preHook,omitempty"`

	// Specifies an authentication function to determine if the requested procedure in the URL should be allowed or disallowed for processing.
	// The function should return true if the procedure is allowed; otherwise, it should return false.
	// If it returns false, Oracle REST Data Services will return WWW-Authenticate in the response header.
	SecurityRequestAuthenticationFunction string `json:"security.requestAuthenticationFunction,omitempty"`

	// Specifies a validation function to determine if the requested procedure in the URL should be allowed or disallowed for processing.
	// The function should return true if the procedure is allowed; otherwise, return false.
	//+kubebuilder:default:="ords_util.authorize_plsql_gateway"
	SecurityRequestValidationFunction string `json:"security.requestValidationFunction,omitempty"`

	// When using the SODA REST API, specifies the default number of documents returned for a GET request on a collection when a
	// limit is not specified in the URL. Must be a positive integer, or "unlimited" for no limit.
	SODADefaultLimit string `json:"soda.defaultLimit,omitempty"`

	// When using the SODA REST API, specifies the maximum number of documents that will be returned for a GET request on a collection URL,
	// regardless of any limit specified in the URL. Must be a positive integer, or "unlimited" for no limit.
	SODAMaxLimit string `json:"soda.maxLimit,omitempty"`

	// Specifies whether the REST-Enabled SQL service is active.
	RestEnabledSqlActive *bool `json:"restEnabledSql.active,omitempty"`

	/*************************************************
	* Customised
	/************************************************/
	/* Below are settings with physical path/file locations to be replaced by ConfigMaps/Secrets, Boolean or HardCoded */

	/*
		// Specifies the wallet archive (provided in BASE64 encoding) containing connection details for the pool.
		// Replaced with: DBWalletSecret *DBWalletSecret `json:"dbWalletSecret,omitempty"`
		DBWalletZip string `json:"db.wallet.zip,omitempty"`

		// Specifies the path to a wallet archive containing connection details for the pool.
		// HARDCODED
		DBWalletZipPath string `json:"db.wallet.zip.path,omitempty"`
	*/

	// Specifies the Secret containing the wallet archive containing connection details for the pool.
	// Replaces: db.wallet.zip
	// db.wallet.zip in ORDS is a wallet archive provided in BASE64 encoding
	DBWalletSecret *DBWalletSecret `json:"dbWalletSecret,omitempty"`

	/*
		// The directory location of your tnsnames.ora file.
		// Replaced with: TNSAdminSecret *TNSAdminSecret `json:"tnsAdminSecret,omitempty"`
		// DBTnsDirectory string `json:"db.tnsDirectory,omitempty"`
	*/

	// Specifies the Secret containing the TNS_ADMIN directory, expected file tnanames.ora
	// Replaces: db.tnsDirectory
	TNSAdminSecret *TNSAdminSecret `json:"tnsAdminSecret,omitempty"`

	// Pool Wallet
	// Specifies the Secret containing the pool wallet directory, expected file cwallet.sso
	// PoolWalletSecret *PoolWalletSecret `json:"poolWalletSecret,omitempty"`

	/*************************************************
	* Disabled
	/*************************************************
	// specifies a configuration setting for AutoUpgrade.jar location.
	// AutoupgradeAPIAulocation string `json:"autoupgrade.api.aulocation,omitempty"`
	// As of 23.4; AutoUpgrade.jar is not part of the container image

	// Specifies a configuration setting to enable AutoUpgrade REST API features.
	// AutoupgradeAPIEnabled *bool `json:"autoupgrade.api.enabled,omitempty"`
	// Guess this has to do with autoupgrade.api.aulocation which is not implemented

	// Specifies a configuration setting for AutoUpgrade REST API JVM location.
	// AutoupgradeAPIJvmlocation string `json:"autoupgrade.api.jvmlocation,omitempty"`
	// Guess this has to do with autoupgrade.api.aulocation which is not implemented

	// Specifies a configuration setting for AutoUpgrade REST API log location.
	// AutoupgradeAPILoglocation string `json:"autoupgrade.api.loglocation,omitempty"`
	// Guess this has to do with autoupgrade.api.aulocation which is not implemented

	// Specifies that the pool points to a CDB, and that the PDBs connected to that CDB should be made addressable
	// by Oracle REST Data Services
	// DBServiceNameSuffix string `json:"db.serviceNameSuffix,omitempty"`
	// Not sure of use case here?!?
	*/
}

type PriVKey struct {
	Secret PasswordSecret `json:"secret"`
}

// Defines the secret containing Password mapped to secretKey
type PasswordSecret struct {
	// Specifies the name of the password Secret
	SecretName string `json:"secretName"`
	// Specifies the key holding the value of the Secret
	//+kubebuilder:default:="password"
	PasswordKey string `json:"passwordKey,omitempty"`
}

// Defines the secret containing Certificates
type CertificateSecret struct {
	// Specifies the name of the certificate Secret
	SecretName string `json:"secretName"`
	// Specifies the Certificate
	Certificate string `json:"cert"`
	// Specifies the Certificate Key
	CertificateKey string `json:"key"`
}

// Defines a secret containing tns admin folder (network/admin), e.g. tnsnames.ora
type TNSAdminSecret struct {
	// Specifies the name of the Secret
	SecretName string `json:"secretName"`
}

// Defines a secret containing pool wallet, Oracle Wallet with credentials, cwallet.sso
//type PoolWalletSecret struct {
//	// Specifies the name of the Secret
//	SecretName string `json:"secretName"`
//}

// Defines the secret containing wallet.zip
type DBWalletSecret struct {
	// Specifies the name of the Database Wallet Secret
	SecretName string `json:"secretName"`
	// Specifies the Secret key name containing the Wallet
	WalletName string `json:"walletName"`
}

// OrdsSrvsStatus defines the observed state of OrdsSrvs
type OrdsSrvsStatus struct {
	//** PLACE HOLDER
	OrdsInstalled bool `json:"ordsInstalled,omitempty"`
	//** PLACE HOLDER
	// Indicates the current status of the resource
	Status string `json:"status,omitempty"`
	// Indicates the current Workload type of the resource
	WorkloadType string `json:"workloadType,omitempty"`
	// Indicates the ORDS version
	ORDSVersion string `json:"ordsVersion,omitempty"`
	// Indicates the HTTP port of the resource exposed by the pods
	HTTPPort *int32 `json:"httpPort,omitempty"`
	// Indicates the HTTPS port of the resource exposed by the pods
	HTTPSPort *int32 `json:"httpsPort,omitempty"`
	// Indicates the MongoAPI port of the resource exposed by the pods (if enabled)
	MongoPort int32 `json:"mongoPort,omitempty"`
	// Indicates if the resource is out-of-sync with the configuration
	RestartRequired bool `json:"restartRequired"`

	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.status",name="status",type="string"
//+kubebuilder:printcolumn:JSONPath=".status.workloadType",name="workloadType",type="string"
//+kubebuilder:printcolumn:JSONPath=".status.ordsVersion",name="ordsVersion",type="string"
//+kubebuilder:printcolumn:JSONPath=".status.httpPort",name="httpPort",type="integer"
//+kubebuilder:printcolumn:JSONPath=".status.httpsPort",name="httpsPort",type="integer"
//+kubebuilder:printcolumn:JSONPath=".status.mongoPort",name="MongoPort",type="integer"
//+kubebuilder:printcolumn:JSONPath=".status.restartRequired",name="restartRequired",type="boolean"
//+kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="AGE",type="date"
//+kubebuilder:printcolumn:JSONPath=".status.ordsInstalled",name="OrdsInstalled",type="boolean"
//+kubebuilder:resource:path=ordssrvs,scope=Namespaced

// OrdsSrvs is the Schema for the ordssrvs API
type OrdsSrvs struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrdsSrvsSpec   `json:"spec,omitempty"`
	Status OrdsSrvsStatus `json:"status,omitempty"`
	
}

//+kubebuilder:object:root=true

// OrdsSrvsList contains a list of OrdsSrvs
type OrdsSrvsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrdsSrvs `json:"items"`
}


func init() {
	SchemeBuilder.Register(&OrdsSrvs{}, &OrdsSrvsList{})
}
