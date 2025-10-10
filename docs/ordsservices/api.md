# API Reference

Packages:

- [database.oracle.com/v1](#databaseoraclecomv1)

# database.oracle.com/v1

Resource Types:

- [OrdsSrvs](#ordssrvs)




## OrdsSrvs
<sup><sup>[↩ Parent](#databaseoraclecomv1 )</sup></sup>






OrdsSrvs is the Schema for the ordssrvs API

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>database.oracle.com/v1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>OrdsSrvs</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspec">spec</a></b></td>
        <td>object</td>
        <td>
          OrdsSrvsSpec defines the desired state of OrdsSrvs<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsstatus">status</a></b></td>
        <td>object</td>
        <td>
          OrdsSrvsStatus defines the observed state of OrdsSrvs<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec
<sup><sup>[↩ Parent](#ordssrvs)</sup></sup>



OrdsSrvsSpec defines the desired state of OrdsSrvs

<table>
<thead> <tr>
<th>Name</th>
<th>Type</th>
<th>Description</th>
<th>Required</th>
</tr>
</thead> <tbody>
<tr>
<td><b><a href="#ordssrvsspecglobalsettings">globalSettings<!--
a--></a></b></td>
<td>object</td>
<td> Contains settings that are configured across the entire
ORDS instance.<br>
</td>
<td>true</td>
</tr>
<tr>
<td><b>image</b></td>
<td>string</td>
<td> Specifies the ORDS container image<br>
</td>
<td>true</td>
</tr>
<tr>
<td><b>forceRestart</b></td>
<td>boolean</td>
<td> Specifies whether to restart pods when Global or Pool
configurations change<br>
</td>
<td>false</td>
</tr>
<tr>
<td><b>imagePullPolicy</b></td>
<td>enum</td>
<td> Specifies the ORDS container image pull policy<br>
<br>
<i>Enum</i>: IfNotPresent, Always, Never<br>
<i>Default</i>: IfNotPresent<br>
</td>
<td>false</td>
</tr>
<tr>
<td><b>imagePullSecrets</b></td>
<td>string</td>
<td> Specifies the Secret Name for pulling the ORDS container
image<br>
</td>
<td>false</td>
</tr>
<tr>
<td><b><a href="#ordssrvsspecpoolsettingsindex">poolSettings&lt;
a&gt;</a></b></td>
<td>[]object</td>
<td> Contains settings for individual pools/databases<br>
</td>
<td>false</td>
</tr>
<tr>
<td><b>replicas</b></td>
<td>integer</td>
<td> Defines the number of desired Replicas when workloadType
Deployment or StatefulSet<br>
<br>
<i>Format</i>: int32<br>
<i>Default</i>: 1<br>
<i>Minimum</i>: 1<br>
</td>
<td>false</td>
</tr>
<tr>
<td><b>workloadType</b></td>
<td>enum</td>
<td> Specifies the desired Kubernetes Workload<br>
<br>
<i>Enum</i>: Deployment, StatefulSet, DaemonSet<br>
<i>Default</i>: Deployment<br>
</td>
<td>false</td>
</tr>
<tr>
<td valign="top"><b>encPrivKey</b><b><br>
</b></td>
<td valign="top">secret<br>
</td>
<td valign="top"><b>secretName</b>: string&nbsp; <b>passwordKey:
</b>string Define the private key to decrypt passwords<br>
</td>
<td valign="top">true<br>
</td>
</tr>
</tbody>
</table>

### OrdsSrvs.spec.globalSettings
<sup><sup>[↩ Parent](#ordssrvsspec)</sup></sup>



Contains settings that are configured across the entire ORDS instance.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>cache.metadata.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies the setting to enable or disable metadata caching.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.graphql.expireAfterAccess</b></td>
        <td>integer</td>
        <td>
          Specifies the duration after a GraphQL schema is not accessed from the cache that it expires.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.graphql.expireAfterWrite</b></td>
        <td>integer</td>
        <td>
          Specifies the duration after a GraphQL schema is cached that it expires and has to be loaded again.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.jwks.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies the setting to enable or disable JWKS caching.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.jwks.expireAfterAccess</b></td>
        <td>integer</td>
        <td>
          Specifies the duration after a JWK is not accessed from the cache that it expires. By default this is disabled.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.jwks.expireAfterWrite</b></td>
        <td>integer</td>
        <td>
          Specifies the duration after a JWK is cached, that is, it expires and has to be loaded again.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.jwks.initialCapacity</b></td>
        <td>integer</td>
        <td>
          Specifies the initial capacity of the JWKS cache.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.jwks.maximumSize</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum capacity of the JWKS cache.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cache.metadata.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the setting to determine for how long a metadata record remains in the cache. Longer duration means, it takes longer to view the applied changes. The formats accepted are based on the ISO-8601 duration format.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspecglobalsettingscertsecret">certSecret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret containing the SSL Certificates Replaces: standalone.https.cert and standalone.https.cert.key<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>database.api.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies whether the Database API is enabled.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>database.api.management.services.disabled</b></td>
        <td>boolean</td>
        <td>
          Specifies to disable the Database API administration related services. Only applicable when Database API is enabled.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.invalidPoolTimeout</b></td>
        <td>integer</td>
        <td>
          Specifies how long to wait before retrying an invalid pool.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr>
      <tr>
        <td><b>debug.printDebugToScreen</b></td>
        <td>boolean</td>
        <td>
          Specifies whether to display error messages on the browser.<br/>
        </td>
        <td>false</td>
      </tr>
      <tr>
        <td><b>downloadAPEX</b></td>
        <td>boolean</td>
        <td>
          Specifies whether to downloan APEX installation files.<br/>
        </td>
        <td>false</td>
      </tr>
      <tr>
        <td><b>downloadUrlAPEX</b></td>
        <td>string</td>
        <td>
          Specifies the URL to downloan APEX installation files.<br/>
        </td>
        <td>https://download.oracle.com/otn_software/apex/apex-latest.zip</td>
      </tr>
      <tr>
        <td><b>enable.mongo.access.log</b></td>
        <td>boolean</td>
        <td>
          Specifies if HTTP request access logs should be enabled If enabled, logs will be written to /opt/oracle/sa/log/global<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>enable.standalone.access.log</b></td>
        <td>boolean</td>
        <td>
          Specifies if HTTP request access logs should be enabled If enabled, logs will be written to /opt/oracle/sa/log/global<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>error.responseFormat</b></td>
        <td>string</td>
        <td>
          Specifies how the HTTP error responses must be formatted. html - Force all responses to be in HTML format json - Force all responses to be in JSON format auto - Automatically determines most appropriate format for the request (default).<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>feature.grahpql.max.nesting.depth</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum join nesting depth limit for GraphQL queries.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>icap.port</b></td>
        <td>integer</td>
        <td>
          Specifies the Internet Content Adaptation Protocol (ICAP) port to virus scan files. Either icap.port or icap.secure.port are required to have a value.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>icap.secure.port</b></td>
        <td>integer</td>
        <td>
          Specifies the Internet Content Adaptation Protocol (ICAP) port to virus scan files. Either icap.port or icap.secure.port are required to have a value. If values for both icap.port and icap.secure.port are provided, then the value of icap.port is ignored.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>icap.server</b></td>
        <td>string</td>
        <td>
          Specifies the Internet Content Adaptation Protocol (ICAP) server name or IP address to virus scan files. The icap.server is required to have a value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>log.procedure</b></td>
        <td>boolean</td>
        <td>
          Specifies whether procedures are to be logged.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mongo.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies to enable the API for MongoDB.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mongo.idle.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum idle time for a Mongo connection in milliseconds.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mongo.op.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum time for a Mongo database operation in milliseconds.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mongo.port</b></td>
        <td>integer</td>
        <td>
          Specifies the API for MongoDB listen port.<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Default</i>: 27017<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>request.traceHeaderName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the HTTP request header that uniquely identifies the request end to end as it passes through the various layers of the application stack. In Oracle this header is commonly referred to as the ECID (Entity Context ID).<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.credentials.attempts</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of unsuccessful password attempts allowed. Enabled by setting a positive integer value.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.credentials.lock.time</b></td>
        <td>integer</td>
        <td>
          Specifies the period to lock the account that has exceeded maximum attempts.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.disableDefaultExclusionList</b></td>
        <td>boolean</td>
        <td>
          If this value is set to true, then the Oracle REST Data Services internal exclusion list is not enforced. Oracle recommends that you do not set this value to true.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.exclusionList</b></td>
        <td>string</td>
        <td>
          Specifies a pattern for procedures, packages, or schema names which are forbidden to be directly executed from a browser.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.externalSessionTrustedOrigins</b></td>
        <td>string</td>
        <td>
          Specifies to trust Access from originating domains<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.forceHTTPS</b></td>
        <td>boolean</td>
        <td>
          Specifies to force HTTPS; this is set to default to false as in real-world TLS should terminiate at the LoadBalancer<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.httpsHeaderCheck</b></td>
        <td>string</td>
        <td>
          Specifies that the HTTP Header contains the specified text Usually set to 'X-Forwarded-Proto: https' coming from a load-balancer<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.inclusionList</b></td>
        <td>string</td>
        <td>
          Specifies a pattern for procedures, packages, or schema names which are allowed to be directly executed from a browser.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.maxEntries</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of cached procedure validations. Set this value to 0 to force the validation procedure to be invoked on each request.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.verifySSL</b></td>
        <td>boolean</td>
        <td>
          Specifies whether HTTPS is available in your environment.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>standalone.context.path</b></td>
        <td>string</td>
        <td>
          Specifies the context path where ords is located.<br/>
          <br/>
            <i>Default</i>: /ords<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>standalone.http.port</b></td>
        <td>integer</td>
        <td>
          Specifies the HTTP listen port.<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Default</i>: 8080<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>standalone.https.host</b></td>
        <td>string</td>
        <td>
          Specifies the SSL certificate hostname.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>standalone.https.port</b></td>
        <td>integer</td>
        <td>
          Specifies the HTTPS listen port.<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Default</i>: 8443<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>standalone.stop.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the period for Standalone Mode to wait until it is gracefully shutdown.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.globalSettings.certSecret
<sup><sup>[↩ Parent](#ordssrvsspecglobalsettings)</sup></sup>



Specifies the Secret containing the SSL Certificates Replaces: standalone.https.cert and standalone.https.cert.key

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>cert</b></td>
        <td>string</td>
        <td>
          Specifies the Certificate<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>key</b></td>
        <td>string</td>
        <td>
          Specifies the Certificate Key<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the certificate Secret<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index]
<sup><sup>[↩ Parent](#ordssrvsspec)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#ordssrvsspecpoolsettingsindexdbsecret">db.secret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret with the dbUsername and dbPassword values for the connection.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>poolName</b></td>
        <td>string</td>
        <td>
          Specifies the Pool Name<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>apex.security.administrator.roles</b></td>
        <td>string</td>
        <td>
          Specifies the comma delimited list of additional roles to assign authenticated APEX administrator type users.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>apex.security.user.roles</b></td>
        <td>string</td>
        <td>
          Specifies the comma delimited list of additional roles to assign authenticated regular APEX users.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>autoUpgradeAPEX</b></td>
        <td>boolean</td>
        <td>
          Specify whether to perform APEX installation/upgrades automatically The db.adminUser and db.adminUser.secret must be set, otherwise setting is ignored This setting will be ignored for ADB<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>autoUpgradeORDS</b></td>
        <td>boolean</td>
        <td>
          Specify whether to perform ORDS installation/upgrades automatically The db.adminUser and db.adminUser.secret must be set, otherwise setting is ignored This setting will be ignored for ADB<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.adminUser</b></td>
        <td>string</td>
        <td>
          Specifies the username for the database account that ORDS uses for administration operations in the database.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspecpoolsettingsindexdbadminusersecret">db.adminUser.secret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret with the dbAdminUser (SYS) and dbAdminPassword values for the database account that ORDS uses for administration operations in the database. replaces: db.adminUser.password<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.cdb.adminUser</b></td>
        <td>string</td>
        <td>
          Specifies the username for the database account that ORDS uses for the Pluggable Database Lifecycle Management.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspecpoolsettingsindexdbcdbadminusersecret">db.cdb.adminUser.secret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret with the dbCdbAdminUser (SYS) and dbCdbAdminPassword values Specifies the username for the database account that ORDS uses for the Pluggable Database Lifecycle Management. Replaces: db.cdb.adminUser.password<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.connectionType</b></td>
        <td>enum</td>
        <td>
          The type of connection.<br/>
          <br/>
            <i>Enum</i>: basic, tns, customurl<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.credentialsSource</b></td>
        <td>enum</td>
        <td>
          Specifies the source for database credentials when creating a direct connection for running SQL statements. Value can be one of pool or request. If the value is pool, then the credentials defined in this pool is used to create a JDBC connection. If the value request is used, then the credentials in the request is used to create a JDBC connection and if successful, grants the requestor SQL Developer role.<br/>
          <br/>
            <i>Enum</i>: pool, request<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.customURL</b></td>
        <td>string</td>
        <td>
          Specifies the JDBC URL connection to connect to the database.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.hostname</b></td>
        <td>string</td>
        <td>
          Specifies the host system for the Oracle database.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.poolDestroyTimeout</b></td>
        <td>integer</td>
        <td>
          Indicates how long to wait to gracefully destroy a pool before moving to forcefully destroy all connections including borrowed ones.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.port</b></td>
        <td>integer</td>
        <td>
          Specifies the database listener port.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.servicename</b></td>
        <td>string</td>
        <td>
          Specifies the network service name of the database.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.sid</b></td>
        <td>string</td>
        <td>
          Specifies the name of the database.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.tnsAliasName</b></td>
        <td>string</td>
        <td>
          Specifies the TNS alias name that matches the name in the tnsnames.ora file.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.username</b></td>
        <td>string</td>
        <td>
          Specifies the name of the database user for the connection. For non-ADB this will default to ORDS_PUBLIC_USER For ADBs this must be specified and not ORDS_PUBLIC_USER If ORDS_PUBLIC_USER is specified for an ADB, the workload will fail<br/>
          <br/>
            <i>Default</i>: ORDS_PUBLIC_USER<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>db.wallet.zip.service</b></td>
        <td>string</td>
        <td>
          Specifies the service name in the wallet archive for the pool.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspecpoolsettingsindexdbwalletsecret">dbWalletSecret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret containing the wallet archive containing connection details for the pool. Replaces: db.wallet.zip<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>debug.trackResources</b></td>
        <td>boolean</td>
        <td>
          Specifies to enable tracking of JDBC resources. If not released causes in resource leaks or exhaustion in the database. Tracking imposes a performance overhead.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>feature.openservicebroker.exclude</b></td>
        <td>boolean</td>
        <td>
          Specifies to disable the Open Service Broker services available for the pool.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>feature.sdw</b></td>
        <td>boolean</td>
        <td>
          Specifies to enable the Database Actions feature.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>http.cookie.filter</b></td>
        <td>string</td>
        <td>
          Specifies a comma separated list of HTTP Cookies to exclude when initializing an Oracle Web Agent environment.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.DriverType</b></td>
        <td>enum</td>
        <td>
          Specifies the JDBC driver type.<br/>
          <br/>
            <i>Enum</i>: thin, oci8<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.InactivityTimeout</b></td>
        <td>integer</td>
        <td>
          Specifies how long an available connection can remain idle before it is closed. The inactivity connection timeout is in seconds.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.InitialLimit</b></td>
        <td>integer</td>
        <td>
          Specifies the initial size for the number of connections that will be created. The default is low, and should probably be set higher in most production environments.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.MaxConnectionReuseCount</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of times to reuse a connection before it is discarded and replaced with a new connection.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.MaxConnectionReuseTime</b></td>
        <td>integer</td>
        <td>
          Sets the maximum connection reuse time property.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.MaxLimit</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of connections. Might be too low for some production environments.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.MaxStatementsLimit</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of statements to cache for each connection.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.MinLimit</b></td>
        <td>integer</td>
        <td>
          Specifies the minimum number of connections.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.SecondsToTrustIdleConnection</b></td>
        <td>integer</td>
        <td>
          Sets the time in seconds to trust an idle connection to skip a validation test.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.auth.admin.role</b></td>
        <td>string</td>
        <td>
          Identifies the database role that indicates that the database user must get the SQL Administrator role.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.auth.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies if the PL/SQL Gateway calls can be authenticated using database users. If the value is true then this feature is enabled. If the value is false, then this feature is disabled. Oracle recommends not to use this feature. This feature used only to facilitate customers migrating from mod_plsql.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.cleanup.mode</b></td>
        <td>string</td>
        <td>
          Specifies how a pooled JDBC connection and corresponding database session, is released when a request has been processed.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>jdbc.statementTimeout</b></td>
        <td>integer</td>
        <td>
          Specifies a timeout period on a statement. An abnormally long running query or script, executed by a request, may leave it in a hanging state unless a timeout is set on the statement. Setting a timeout on the statement ensures that all the queries automatically timeout if they are not completed within the specified time period.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>misc.defaultPage</b></td>
        <td>string</td>
        <td>
          Specifies the default page to display. The Oracle REST Data Services Landing Page.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>misc.pagination.maxRows</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of rows that will be returned from a query when processing a RESTful service and that will be returned from a nested cursor in a result set. Affects all RESTful services generated through a SQL query, regardless of whether the resource is paginated.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>owa.trace.sql</b></td>
        <td>boolean</td>
        <td>
          If it is true, then it causes a trace of the SQL statements performed by Oracle Web Agent to be echoed to the log.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>plsql.gateway.mode</b></td>
        <td>enum</td>
        <td>
          Indicates if the PL/SQL Gateway functionality should be available for a pool or not. Value can be one of disabled, direct, or proxied. If the value is direct, then the pool serves the PL/SQL Gateway requests directly. If the value is proxied, the PLSQL_GATEWAY_CONFIG view is used to determine the user to whom to proxy.<br/>
          <br/>
            <i>Enum</i>: disabled, direct, proxied<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>procedure.preProcess</b></td>
        <td>string</td>
        <td>
          Specifies the procedure name(s) to execute prior to executing the procedure specified on the URL. Multiple procedure names must be separated by commas.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>procedure.rest.preHook</b></td>
        <td>string</td>
        <td>
          Specifies the function to be invoked prior to dispatching each Oracle REST Data Services based REST Service. The function can perform configuration of the database session, perform additional validation or authorization of the request. If the function returns true, then processing of the request continues. If the function returns false, then processing of the request is aborted and an HTTP 403 Forbidden status is returned.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>procedurePostProcess</b></td>
        <td>string</td>
        <td>
          Specifies the procedure name(s) to execute after executing the procedure specified on the URL. Multiple procedure names must be separated by commas.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>restEnabledSql.active</b></td>
        <td>boolean</td>
        <td>
          Specifies whether the REST-Enabled SQL service is active.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwks.connection.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum amount of time before timing-out when accessing a JWK url.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwks.read.timeout</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum amount of time reading a response from the JWK url before timing-out.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwks.refresh.interval</b></td>
        <td>integer</td>
        <td>
          Specifies the minimum interval between refreshing the JWK cached value.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwks.size</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum number of bytes read from the JWK url.<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwt.allowed.age</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum allowed age of a JWT in seconds, regardless of expired claim. The age of the JWT is taken from the JWT issued at claim.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwt.allowed.skew</b></td>
        <td>integer</td>
        <td>
          Specifies the maximum skew the JWT time claims are accepted. This is useful if the clock on the JWT issuer and ORDS differs by a few seconds.<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.jwt.profile.enabled</b></td>
        <td>boolean</td>
        <td>
          Specifies whether the JWT Profile authentication is available. Supported values:<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.requestAuthenticationFunction</b></td>
        <td>string</td>
        <td>
          Specifies an authentication function to determine if the requested procedure in the URL should be allowed or disallowed for processing. The function should return true if the procedure is allowed; otherwise, it should return false. If it returns false, Oracle REST Data Services will return WWW-Authenticate in the response header.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.requestValidationFunction</b></td>
        <td>string</td>
        <td>
          Specifies a validation function to determine if the requested procedure in the URL should be allowed or disallowed for processing. The function should return true if the procedure is allowed; otherwise, return false.<br/>
          <br/>
            <i>Default</i>: ords_util.authorize_plsql_gateway<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>security.validationFunctionType</b></td>
        <td>enum</td>
        <td>
          Indicates the type of security.requestValidationFunction: javascript or plsql.<br/>
          <br/>
            <i>Enum</i>: plsql, javascript<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>soda.defaultLimit</b></td>
        <td>string</td>
        <td>
          When using the SODA REST API, specifies the default number of documents returned for a GET request on a collection when a limit is not specified in the URL. Must be a positive integer, or "unlimited" for no limit.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>soda.maxLimit</b></td>
        <td>string</td>
        <td>
          When using the SODA REST API, specifies the maximum number of documents that will be returned for a GET request on a collection URL, regardless of any limit specified in the URL. Must be a positive integer, or "unlimited" for no limit.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ordssrvsspecpoolsettingsindextnsadminsecret">tnsAdminSecret</a></b></td>
        <td>object</td>
        <td>
          Specifies the Secret containing the TNS_ADMIN directory Replaces: db.tnsDirectory<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index].db.secret
<sup><sup>[↩ Parent](#ordssrvsspecpoolsettingsindex)</sup></sup>



Specifies the Secret with the dbUsername and dbPassword values for the connection.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the password Secret<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>passwordKey</b></td>
        <td>string</td>
        <td>
          Specifies the key holding the value of the Secret<br/>
          <br/>
            <i>Default</i>: password<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index].db.adminUser.secret
<sup><sup>[↩ Parent](#ordssrvsspecpoolsettingsindex)</sup></sup>



Specifies the Secret with the dbAdminUser (SYS) and dbAdminPassword values for the database account that ORDS uses for administration operations in the database. replaces: db.adminUser.password

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the password Secret<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>passwordKey</b></td>
        <td>string</td>
        <td>
          Specifies the key holding the value of the Secret<br/>
          <br/>
            <i>Default</i>: password<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index].db.cdb.adminUser.secret
<sup><sup>[↩ Parent](#ordssrvsspecpoolsettingsindex)</sup></sup>



Specifies the Secret with the dbCdbAdminUser (SYS) and dbCdbAdminPassword values Specifies the username for the database account that ORDS uses for the Pluggable Database Lifecycle Management. Replaces: db.cdb.adminUser.password

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the password Secret<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>passwordKey</b></td>
        <td>string</td>
        <td>
          Specifies the key holding the value of the Secret<br/>
          <br/>
            <i>Default</i>: password<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index].dbWalletSecret
<sup><sup>[↩ Parent](#ordssrvsspecpoolsettingsindex)</sup></sup>



Specifies the Secret containing the wallet archive containing connection details for the pool. Replaces: db.wallet.zip

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the Database Wallet Secret<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>walletName</b></td>
        <td>string</td>
        <td>
          Specifies the Secret key name containing the Wallet<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### OrdsSrvs.spec.poolSettings[index].tnsAdminSecret
<sup><sup>[↩ Parent](#ordssrvsspecpoolsettingsindex)</sup></sup>



Specifies the Secret containing the TNS_ADMIN directory Replaces: db.tnsDirectory

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>secretName</b></td>
        <td>string</td>
        <td>
          Specifies the name of the TNS_ADMIN Secret<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### OrdsSrvs.status
<sup><sup>[↩ Parent](#ordssrvs)</sup></sup>



OrdsSrvsStatus defines the observed state of OrdsSrvs

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>restartRequired</b></td>
        <td>boolean</td>
        <td>
          Indicates if the resource is out-of-sync with the configuration<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#ordssrvsstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>httpPort</b></td>
        <td>integer</td>
        <td>
          Indicates the HTTP port of the resource exposed by the pods<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>httpsPort</b></td>
        <td>integer</td>
        <td>
          Indicates the HTTPS port of the resource exposed by the pods<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mongoPort</b></td>
        <td>integer</td>
        <td>
          Indicates the MongoAPI port of the resource exposed by the pods (if enabled)<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>ordsVersion</b></td>
        <td>string</td>
        <td>
          Indicates the ORDS version<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>string</td>
        <td>
          Indicates the current status of the resource<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>workloadType</b></td>
        <td>string</td>
        <td>
          Indicates the current Workload type of the resource<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### OrdsSrvs.status.conditions[index]
<sup><sup>[↩ Parent](#ordssrvsstatus)</sup></sup>



Condition contains details for one aspect of the current state of this API Resource. --- This struct is intended for direct use as an array at the field path .status.conditions.  For example, 
 type FooStatus struct{ // Represents the observations of a foo's current state. // Known .status.conditions.type are: "Available", "Progressing", and "Degraded" // +patchMergeKey=type // +patchStrategy=merge // +listType=map // +listMapKey=type Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"` 
 // other fields }

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>lastTransitionTime</b></td>
        <td>string</td>
        <td>
          lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          message is a human readable message indicating details about the transition. This may be an empty string.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>reason</b></td>
        <td>string</td>
        <td>
          reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>enum</td>
        <td>
          status of the condition, one of True, False, Unknown.<br/>
          <br/>
            <i>Enum</i>: True, False, Unknown<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>string</td>
        <td>
          type of condition in CamelCase or in foo.example.com/CamelCase. --- Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be useful (see .node.status.conditions), the ability to deconflict is important. The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>observedGeneration</b></td>
        <td>integer</td>
        <td>
          observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance.<br/>
          <br/>
            <i>Format</i>: int64<br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>
