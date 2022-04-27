/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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

package commons

const ORACLE_UID int64 = 54321

const ORACLE_GUID int64 = 54321

const DBA_GUID int64 = 54322

const SQLPlusCLI string = "sqlplus -s / as sysdba"

const SQLPlusRemoteCLI string = "sqlplus -s sys/%[1]s@%[2]s as sysdba"

const NoCloneRef string = "Unavailable"

const GetVersionSQL string = "SELECT VERSION_FULL FROM V\\$INSTANCE;"

const CheckModesSQL string = "SELECT 'log_mode:' || log_mode AS log_mode ,'flashback_on:' || flashback_on AS flashback_on ,'force_logging:' || force_logging AS force_logging FROM v\\$database;"

const ListPdbSQL string = "SELECT NAME FROM V\\$PDBS;"

const CreateChkFileCMD string = "touch \"${ORACLE_BASE}/oradata/.${ORACLE_SID}.nochk\" && sync"

const RemoveChkFileCMD string = "rm -f \"${ORACLE_BASE}/oradata/.${ORACLE_SID}.nochk\""

const CreateDBRecoveryDestCMD string = "mkdir -p ${ORACLE_BASE}/oradata/fast_recovery_area"

const SetDBRecoveryDestSQL string = "SHOW PARAMETER db_recovery_file_dest;" +
	"\nALTER SYSTEM SET db_recovery_file_dest_size=50G scope=both sid='*';" +
	"\nALTER SYSTEM SET db_recovery_file_dest='${ORACLE_BASE}/oradata/fast_recovery_area' scope=both sid='*';" +
	"\nSHOW PARAMETER db_recovery_file_dest;"

const ForceLoggingTrueSQL string = "SELECT force_logging FROM v\\$database;" +
	"\nALTER DATABASE FORCE LOGGING;" +
	"\nALTER SYSTEM SWITCH LOGFILE;" +
	"\nSELECT force_logging FROM v\\$database;"

const ForceLoggingFalseSQL string = "SELECT force_logging FROM v\\$database;" +
	"\nALTER DATABASE NO FORCE LOGGING;" +
	"\nSELECT force_logging FROM v\\$database;"

const FlashBackTrueSQL string = "SELECT flashback_on FROM v\\$database;" +
	"\nALTER DATABASE FLASHBACK ON;" +
	"\nSELECT flashback_on FROM v\\$database;"

const FlashBackFalseSQL string = "SELECT flashback_on FROM v\\$database;" +
	"\nALTER DATABASE FLASHBACK OFF;" +
	"\nSELECT flashback_on FROM v\\$database;"

const ArchiveLogTrueCMD string = CreateChkFileCMD + " && " +
	"echo -e  \"SHUTDOWN IMMEDIATE; \n STARTUP MOUNT; \n ALTER DATABASE ARCHIVELOG; \n SELECT log_mode FROM v\\$database; \n ALTER DATABASE OPEN;" +
	" \n ALTER PLUGGABLE DATABASE ALL OPEN; \n ALTER SYSTEM REGISTER;\" | %s && " + RemoveChkFileCMD

const ArchiveLogFalseCMD string = CreateChkFileCMD + " && " +
	"echo -e  \"SHUTDOWN IMMEDIATE; \n STARTUP MOUNT; \n ALTER DATABASE NOARCHIVELOG; \n SELECT log_mode FROM v\\$database; \n ALTER DATABASE OPEN;" +
	" \n ALTER PLUGGABLE DATABASE ALL OPEN; \n ALTER SYSTEM REGISTER;\" | %s && " + RemoveChkFileCMD

const StandbyDatabasePrerequisitesSQL string = "ALTER SYSTEM SET db_create_file_dest='/opt/oracle/oradata/';" +
	"\nALTER SYSTEM SET db_create_online_log_dest_1='/opt/oracle/oradata/';" +
	"\nALTER SYSTEM SWITCH LOGFILE;" +
	"\nALTER DATABASE ADD STANDBY LOGFILE THREAD 1 GROUP 10 SIZE 200M;" +
	"\nALTER DATABASE ADD STANDBY LOGFILE THREAD 1 GROUP 11 SIZE 200M;" +
	"\nALTER DATABASE ADD STANDBY LOGFILE THREAD 1 GROUP 12 SIZE 200M;" +
	"\nALTER DATABASE ADD STANDBY LOGFILE THREAD 1 GROUP 13 SIZE 200M;" +
	"\nALTER SYSTEM SET STANDBY_FILE_MANAGEMENT=AUTO;" +
	"\nALTER SYSTEM SET dg_broker_start=TRUE;"

const StandbyTnsnamesEntry string = `
##STANDBYDATABASE_SID## =
(DESCRIPTION =
(ADDRESS = (PROTOCOL = TCP)(HOST = ##STANDBYDATABASE_SERVICE_EXPOSED## )(PORT = 1521))
(CONNECT_DATA =
(SERVER = DEDICATED)
(SERVICE_NAME = ##STANDBYDATABASE_SID##)
)
)
`
const PDBTnsnamesEntry string = `
##PDB_NAME## =
(DESCRIPTION =
(ADDRESS = (PROTOCOL = TCP)(HOST = 0.0.0.0 )(PORT = 1521))
(CONNECT_DATA =
(SERVER = DEDICATED)
(SERVICE_NAME = ##PDB_NAME##)
)
)
`

const PrimaryTnsnamesEntry string = `
${PRIMARY_SID} =
(DESCRIPTION =
(ADDRESS = (PROTOCOL = TCP)(HOST = ${PRIMARY_IP})(PORT = 1521 ))
(CONNECT_DATA =
 (SERVER = DEDICATED)
 (SERVICE_NAME = ${PRIMARY_SID})
)
)
`

const ListenerEntry string = `LISTENER = 
(DESCRIPTION_LIST = 
(DESCRIPTION = 
(ADDRESS = (PROTOCOL = IPC)(KEY = EXTPROC1)) 
(ADDRESS = (PROTOCOL = TCP)(HOST = 0.0.0.0)(PORT = 1521)) 
)
) 
SID_LIST_LISTENER =
(SID_LIST =
(SID_DESC =
  (GLOBAL_DBNAME = ${ORACLE_SID^^})
  (SID_NAME = ${ORACLE_SID^^})
  (ORACLE_HOME = ${ORACLE_HOME})
)
(SID_DESC =
  (GLOBAL_DBNAME = ${ORACLE_SID^^}_DGMGRL)
  (SID_NAME = ${ORACLE_SID^^})
  (ORACLE_HOME = ${ORACLE_HOME})
  (ENVS="TNS_ADMIN=/opt/oracle/oradata/dbconfig/${ORACLE_SID^^}")
)
)

DEDICATED_THROUGH_BROKER_LISTENER=ON
`

const DataguardBrokerMaxPerformanceCMD string = "CREATE CONFIGURATION dg_config AS PRIMARY DATABASE IS ${PRIMARY_SID} CONNECT IDENTIFIER IS ${PRIMARY_DB_CONN_STR};" +
	"\nADD DATABASE ${ORACLE_SID} AS CONNECT IDENTIFIER IS ${SVC_HOST}:1521/${ORACLE_SID} MAINTAINED AS PHYSICAL;" +
	"\nEDIT DATABASE ${PRIMARY_SID} SET PROPERTY LogXptMode='ASYNC';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY LogXptMode='ASYNC';" +
	"\nEDIT DATABASE ${PRIMARY_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${PRIMARY_IP})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${PRIMARY_SID}_DGMGRL)(INSTANCE_NAME=${PRIMARY_SID})(SERVER=DEDICATED)))';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${SVC_HOST})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${ORACLE_SID}_DGMGRL)(INSTANCE_NAME=${ORACLE_SID})(SERVER=DEDICATED)))';" +
	"\nEDIT CONFIGURATION SET PROTECTION MODE AS MAXPERFORMANCE;" +
	"\nENABLE CONFIGURATION;"

const DataguardBrokerMaxAvailabilityCMD string = "CREATE CONFIGURATION dg_config AS PRIMARY DATABASE IS ${PRIMARY_SID} CONNECT IDENTIFIER IS ${PRIMARY_DB_CONN_STR};" +
	"\nADD DATABASE ${ORACLE_SID} AS CONNECT IDENTIFIER IS ${SVC_HOST}:1521/${ORACLE_SID} MAINTAINED AS PHYSICAL;" +
	"\nEDIT DATABASE ${PRIMARY_SID} SET PROPERTY LogXptMode='SYNC';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY LogXptMode='SYNC';" +
	"\nEDIT DATABASE ${PRIMARY_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${PRIMARY_IP})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${PRIMARY_SID}_DGMGRL)(INSTANCE_NAME=${PRIMARY_SID})(SERVER=DEDICATED)))';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${SVC_HOST})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${ORACLE_SID}_DGMGRL)(INSTANCE_NAME=${ORACLE_SID})(SERVER=DEDICATED)))';" +
	"\nEDIT CONFIGURATION SET PROTECTION MODE AS MAXAVAILABILITY;" +
	"\nENABLE CONFIGURATION;"

const DataguardBrokerAddDBMaxPerformanceCMD string = "ADD DATABASE ${ORACLE_SID} AS CONNECT IDENTIFIER IS ${SVC_HOST}:1521/${ORACLE_SID} MAINTAINED AS PHYSICAL;" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY LogXptMode='ASYNC';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${SVC_HOST})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${ORACLE_SID}_DGMGRL)(INSTANCE_NAME=${ORACLE_SID})(SERVER=DEDICATED)))';" +
	"\nENABLE CONFIGURATION;"

const DataguardBrokerAddDBMaxAvailabilityCMD string = "ADD DATABASE ${ORACLE_SID} AS CONNECT IDENTIFIER IS ${SVC_HOST}:1521/${ORACLE_SID} MAINTAINED AS PHYSICAL;" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY LogXptMode='SYNC';" +
	"\nEDIT DATABASE ${ORACLE_SID} SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=${SVC_HOST})(PORT=1521))" +
	"(CONNECT_DATA=(SERVICE_NAME=${ORACLE_SID}_DGMGRL)(INSTANCE_NAME=${ORACLE_SID})(SERVER=DEDICATED)))';" +
	"\nENABLE CONFIGURATION;"

const DBShowConfigCMD string = "SHOW CONFIGURATION;"

const DataguardBrokerGetDatabaseCMD string = "SELECT DATABASE || ':' || DATAGUARD_ROLE AS DATABASE FROM V\\$DG_BROKER_CONFIG;"

const EnableFSFOCMD string = "ENABLE FAST_START FAILOVER;"

const GetDatabaseRoleCMD string = "SELECT DATABASE_ROLE FROM V\\$DATABASE; "

const RunDatapatchCMD string = " ( while true; do  sleep 60; echo \"Installing patches...\" ; done ) & if ! $ORACLE_HOME/OPatch/datapatch -skip_upgrade_check;" +
	" then echo \"Datapatch execution has failed.\" ; else echo \"DONE: Datapatch execution.\" ; fi ; kill -9 $!;"

const GetSqlpatchDescriptionSQL string = "select TARGET_VERSION || ' (' || PATCH_ID || ')' as patchinfo  from dba_registry_sqlpatch order by action_time desc;"

const GetSqlpatchStatusSQL string = "select status from dba_registry_sqlpatch order by action_time desc;"

const GetSqlpatchVersionSQL string = "select SOURCE_VERSION || ':' || TARGET_VERSION as versions from dba_registry_sqlpatch order by action_time desc;"

const GetCheckpointFileCMD string = "find ${ORACLE_BASE}/oradata -name .${ORACLE_SID}${CHECKPOINT_FILE_EXTN} "

const GetEnterpriseEditionFileCMD string = "if [ -f ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_enterprise ]; then ls ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_enterprise; fi "

const GetStandardEditionFileCMD string = "if [ -f ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_standard ]; then ls ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_standard; fi "

const GetPdbsSQL string = "select name from v\\$pdbs where name not like 'PDB\\$SEED' and open_mode like 'READ WRITE';"

const SetAdminUsersSQL string = "CREATE USER C##DBAPI_CDB_ADMIN IDENTIFIED BY \\\"%[1]s\\\" ACCOUNT UNLOCK CONTAINER=ALL;" +
	"\nalter user C##DBAPI_CDB_ADMIN identified by \\\"%[1]s\\\" account unlock;" +
	"\nGRANT DBA TO C##DBAPI_CDB_ADMIN CONTAINER = ALL;" +
	"\nGRANT PDB_DBA  TO C##DBAPI_CDB_ADMIN CONTAINER = ALL;" +
	"\nCREATE USER C##_DBAPI_PDB_ADMIN IDENTIFIED BY \\\"%[1]s\\\" CONTAINER=ALL ACCOUNT UNLOCK;" +
	"\nalter user C##_DBAPI_PDB_ADMIN identified by \\\"%[1]s\\\" account unlock;" +
	"\nGRANT DBA TO C##_DBAPI_PDB_ADMIN CONTAINER = ALL;" +
	"\nalter pluggable database pdb\\$seed close;" +
	"\nalter pluggable database pdb\\$seed open read write force;"

const GetUserOrdsSchemaStatusSQL string = "alter session set container=%[2]s;" +
	"\nselect 'STATUS:'||status as status from ords_metadata.ords_schemas where upper(parsing_schema) = upper('%[1]s');"

const EnableORDSSchemaSQL string = "\nALTER SESSION SET CONTAINER=%[5]s;" +
	"\nCREATE USER %[1]s IDENTIFIED BY \\\"%[2]s\\\";" +
	"\nGRANT CONNECT, RESOURCE, DBA, PDB_DBA TO %[1]s;" +
	"\nCONN %[1]s/\\\"%[2]s\\\"@localhost:1521/%[5]s;" +
	"\nBEGIN" +
	"\nORDS.enable_schema(p_enabled => %[3]s ,p_schema => '%[1]s',p_url_mapping_type => 'BASE_PATH',p_url_mapping_pattern => '%[4]s',p_auto_rest_auth => FALSE);" +
	"\nCOMMIT;" +
	"\nEND;" +
	"\n/"

	// SetupORDSCMD is run only for the FIRST TIME, ORDS is installed. Once ORDS is installed, we delete the pod that ran SetupORDSCMD and create new ones.
	// Newly created pod doesn't run this SetupORDSCMD.
const SetupORDSCMD string = "$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.auth.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.management.services.disabled false" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.admin.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property dbc.auth.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property restEnabledSql.active true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property db.serviceNameSuffix \"\" " + // Mandatory when ORDS Installing at CDB Level -> Maps PDB's
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.InitialLimit 5" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.MaxLimit 20" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.InactivityTimeout 300" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property feature.sdw true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property security.verifySSL false" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.maxRows 1000" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property pdb.open.asneeded true" +
	"\numask 177" +
	"\necho db.cdb.adminUser=C##DBAPI_CDB_ADMIN AS SYSDBA > cdbAdmin.properties" +
	"\necho db.cdb.adminUser.password=\"%[4]s\" >> cdbAdmin.properties" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_pu cdbAdmin.properties" +
	"\nrm -f cdbAdmin.properties" +
	"\necho db.username=APEX_LISTENER > apexlistener" +
	"\necho db.password=\"%[2]s\" >> apexlistener" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_al apexlistener" +
	"\nrm -f apexlistener" +
	"\necho db.username=APEX_REST_PUBLIC_USER > apexRestPublicUser" +
	"\necho db.password=\"%[2]s\" >> apexRestPublicUser" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_rt apexRestPublicUser" +
	"\nrm -f apexRestPublicUser" +
	"\necho db.username=APEX_PUBLIC_USER > apexPublicUser" +
	"\necho db.password=\"%[2]s\" >> apexPublicUser" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex apexPublicUser" +
	"\nrm -f apexPublicUser" +
	"\necho db.adminUser=C##_DBAPI_PDB_ADMIN > pdbAdmin.properties" +
	"\necho db.adminUser.password=\"%[4]s\">> pdbAdmin.properties" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_pu pdbAdmin.properties" +
	"\nrm -f pdbAdmin.properties" +
	"\necho -e \"%[1]s\n%[1]s\" > sqladmin.passwd" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war user ${ORDS_USER} \"SQL Administrator , System Administrator , SQL Developer , oracle.dbtools.autorest.any.schema \" < sqladmin.passwd" +
	"\nrm -f sqladmin.passwd" +
	"\numask 022" +
	"\nsed -i 's,jetty.port=8888,jetty.secure.port=8443\\nssl.cert=\\nssl.cert.key=\\nssl.host=%[3]s,g' /opt/oracle/ords/config/ords/standalone/standalone.properties " +
	"\nsed -i 's,standalone.static.path=/opt/oracle/ords/doc_root/i,standalone.static.path=/opt/oracle/ords/config/apex/images,g' /opt/oracle/ords/config/ords/standalone/standalone.properties"

const InitORDSCMD string = "if [ -f $ORDS_HOME/config/ords/defaults.xml ]; then exit ;fi;" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.auth.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.management.services.disabled false" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property database.api.admin.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property dbc.auth.enabled true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property restEnabledSql.active true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property db.serviceNameSuffix \"\" " + // Mandatory when ORDS Installing at CDB Level -> Maps PDB's
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.InitialLimit 5" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.MaxLimit 20" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.InactivityTimeout 300" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property feature.sdw true" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property security.verifySSL false" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-property jdbc.maxRows 1000" +
	"\nmkdir -p $ORDS_HOME/config/ords/conf" +
	"\numask 177" +
	"\necho db.cdb.adminUser=C##DBAPI_CDB_ADMIN AS SYSDBA > cdbAdmin.properties" +
	"\necho db.cdb.adminUser.password=\"${ORDS_PWD}\" >> cdbAdmin.properties" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_pu cdbAdmin.properties" +
	"\nrm -f cdbAdmin.properties" +
	"\necho db.adminUser=C##_DBAPI_PDB_ADMIN > pdbAdmin.properties" +
	"\necho db.adminUser.password=\"${ORDS_PWD}\">> pdbAdmin.properties" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_pu pdbAdmin.properties" +
	"\nrm -f pdbAdmin.properties" +
	"\necho -e \"${ORDS_PWD}\n${ORDS_PWD}\" > sqladmin.passwd" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war user ${ORDS_USER} \"SQL Administrator , System Administrator , SQL Developer , oracle.dbtools.autorest.any.schema \" < sqladmin.passwd" +
	"\nrm -f sqladmin.passwd" +
	"\numask 022" +
	"\nexport APEXI=$ORDS_HOME/config/apex/images" +
	"\nset -x" +
	"\n. $ORDS_HOME/runOrds.sh"

const GetSessionInfoSQL string = "select s.sid || ',' || s.serial# as Info FROM v\\$session s, v\\$process p WHERE s.username = 'ORDS_PUBLIC_USER' AND p.addr(+) = s.paddr;"

const KillSessionSQL string = "alter system kill session '%[1]s';"

const DropAdminUsersSQL string = "drop user C##DBAPI_CDB_ADMIN;" +
	"\ndrop user C##_DBAPI_PDB_ADMIN;"

const UninstallORDSCMD string = "\numask 177" +
	"\necho -e \"1\n${ORACLE_HOST}\n${ORACLE_PORT}\n1\n${ORACLE_SERVICE}\nsys\n%[1]s\n%[1]s\n1\" > ords.cred" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war uninstall advanced < ords.cred" +
	"\nrm -f ords.cred" +
	"\numask 022" +
	"\nrm -f /opt/oracle/ords/config/ords/defaults.xml" +
	"\nrm -f /opt/oracle/ords/config/ords/credentials" +
	"\nrm -rf /opt/oracle/ords/config/ords/conf" +
	"\nrm -rf /opt/oracle/ords/config/ords/standalone" +
	"\nrm -rf /opt/oracle/ords/config/ords/apex"


const GetORDSStatus string = "curl -sSkv -k -X GET https://localhost:8443/ords/_/db-api/stable/metadata-catalog/"

const ValidateAdminPassword string = "conn sys/%s@${ORACLE_SID} as sysdba\nshow user"

const ReconcileError string = "ReconcileError"

const ReconcileErrorReason string = "LastReconcileCycleFailed"

const ReconcileQueued string = "ReconcileQueued"

const ReconcileQueuedReason string = "LastReconcileCycleQueued"

const ReconcileCompelete string = "ReconcileComplete"

const ReconcileCompleteReason string = "LastReconcileCycleCompleted"

const ReconcileBlocked string = "ReconcileBlocked"

const ReconcileBlockedReason string = "LastReconcileCycleBlocked"

const StatusPending string = "Pending"

const StatusCreating string = "Creating"

const StatusNotReady string = "Unhealthy"

const StatusPatching string = "Patching"

const StatusUpdating string = "Updating"

const StatusReady string = "Healthy"

const StatusError string = "Error"

const ValueUnavailable string = "Unknown"

const NoExternalIp string = "Node ExternalIP unavailable"

const WalletPwdCMD string = "export WALLET_PWD=\"`openssl rand -base64 8`1\""

const WalletCreateCMD string = "if [[ ! -f ${WALLET_DIR}/ewallet.p12 ]]; then mkdir -p ${WALLET_DIR}/.wallet && (umask 177\ncat > wallet.passwd <<EOF\n${WALLET_PWD}\n${WALLET_PWD}\nEOF\nmkstore -wrl ${WALLET_DIR} -create < wallet.passwd\nrm -f wallet.passwd\numask 022;) fi"

const WalletDeleteCMD string = "rm -rf ${WALLET_DIR}"

const WalletEntriesCMD string = "umask 177\ncat > wallet.passwd <<EOF\n${WALLET_PWD}\nEOF\n mkstore -wrl ${WALLET_DIR} -createEntry oracle.dbsecurity.sysPassword %[1]s -createEntry oracle.dbsecurity.systemPassword %[1]s " +
	"-createEntry oracle.dbsecurity.pdbAdminPassword %[1]s -createEntry oracle.dbsecurity.dbsnmpPassword %[1]s < wallet.passwd\nrm -f wallet.passwd\numask 022;"

const InitWalletCMD string = "if [ ! -f $ORACLE_BASE/oradata/.${ORACLE_SID}${CHECKPOINT_FILE_EXTN} ] || [ ! -f ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_%s ];" +
	" then while [ ! -f ${WALLET_DIR}/ewallet.p12 ] || pgrep -f $WALLET_CLI > /dev/null; do sleep 0.5; done; fi "

const InitPrebuiltDbCMD string = "if [ ! -d /mnt/oradata/${ORACLE_SID} ]; then cp -v $ORACLE_BASE/oradata/.${ORACLE_SID}$CHECKPOINT_FILE_EXTN /mnt/oradata && " +
	" cp -vr $ORACLE_BASE/oradata/${ORACLE_SID} /mnt/oradata && cp -vr $ORACLE_BASE/oradata/dbconfig /mnt/oradata; fi "


const AlterSgaPgaCpuCMD string = "echo -e  \"alter system set sga_target=%dM scope=both; \n alter system set pga_aggregate_target=%dM scope=both; \n alter system set cpu_count=%d; \" | %s "

const AlterProcessesCMD string = "echo -e  \"alter system set processes=%d scope=spfile; \" | %s && " + CreateChkFileCMD + " && " +
	"echo -e  \"SHUTDOWN IMMEDIATE; \n STARTUP MOUNT; \n ALTER DATABASE OPEN; \n ALTER PLUGGABLE DATABASE ALL OPEN; \n ALTER SYSTEM REGISTER;\" | %s && " +
	RemoveChkFileCMD

const GetInitParamsSQL string = "echo -e  \"select name,display_value from v\\$parameter  where name in  ('sga_target','pga_aggregate_target','cpu_count','processes') order by name asc;\" | %s"

const UnzipApexOnSIDBPod string = "if [ -f /opt/oracle/oradata/apex-latest.zip ]; then unzip -o /opt/oracle/oradata/apex-latest.zip -d /opt/oracle/oradata/${ORACLE_SID^^}; else echo \"apex-latest.zip not found\"; fi;"

const UnzipApexOnORDSPod string = "if [ -f /opt/oracle/ords/config/ords/apex-latest.zip ]; then cd /opt/oracle/ords/config/ords && jar -xf /opt/oracle/ords/config/ords/apex-latest.zip; else echo \"apex-latest.zip not found\"; fi;"

const ChownApex string = " chown oracle:oinstall /opt/oracle/oradata/${ORACLE_SID^^}/apex;"

const InstallApex string = "if [ -f /opt/oracle/oradata/${ORACLE_SID^^}/apex/apexins.sql ]; then  ( while true; do  sleep 60; echo \"Installing Apex...\" ; done ) & " +
	" cd /opt/oracle/oradata/${ORACLE_SID^^}/apex && echo -e \"@apexins.sql SYSAUX SYSAUX TEMP /i/\" | %[1]s && kill -9 $!; else echo \"Apex Folder doesn't exist\" ; fi ;"

const InstallApexRemote string = "if [ -e ${ORDS_HOME}/config/apex/apxsilentins.sql ]; then cd ${ORDS_HOME}/config/apex/ && echo -e \"@apxsilentins.sql SYSAUX SYSAUX TEMP /i/ %[2]s %[2]s %[2]s %[2]s\" | %[1]s; else echo \"Apex Folder doesn't exist\" ; fi ;"

const InstallApexInContainer string = "cd ${ORDS_HOME}/config/apex/ && echo -e \"@apxsilentins.sql SYSAUX SYSAUX TEMP /i/ ${ORDS_PWD} ${ORDS_PWD} ${ORDS_PWD} ${ORDS_PWD};"+
	"\n@apex_rest_config_core.sql;\nexec APEX_UTIL.set_workspace(p_workspace => 'INTERNAL');\n"+
	"exec APEX_UTIL.EDIT_USER(p_user_id => APEX_UTIL.GET_USER_ID('ADMIN'), p_user_name  => 'ADMIN', p_change_password_on_first_use => 'Y');\n\"" +
	" | sqlplus -s sys/${ORACLE_PWD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE} as sysdba;"

const IsApexInstalled string = "select 'APEXVERSION:'||version as version FROM DBA_REGISTRY WHERE COMP_ID='APEX';"

const UninstallApex string = "if [ -f /opt/oracle/oradata/${ORACLE_SID^^}/apex/apxremov.sql ]; then  ( while true; do  sleep 60; echo \"Uninstalling Apex...\" ; done ) & " +
	" cd /opt/oracle/oradata/${ORACLE_SID^^}/apex && echo -e \"@apxremov.sql\" | %[1]s && kill -9 $!; else echo \"Apex Folder doesn't exist\" ; fi ;"

const ConfigureApexRest string = "if [ -f ${ORDS_HOME}/config/apex/apex_rest_config.sql ]; then  cd ${ORDS_HOME}/config/apex && " +
	"echo -e \"%[1]s\n%[1]s\" | %[2]s ; else echo \"Apex Folder doesn't exist\" ; fi ;"

const AlterApexUsers string = "echo -e \" ALTER USER APEX_PUBLIC_USER IDENTIFIED BY \\\"%[1]s\\\" ACCOUNT UNLOCK; \n ALTER USER APEX_REST_PUBLIC_USER IDENTIFIED BY \\\"%[1]s\\\" ACCOUNT UNLOCK;" +
	" \n ALTER USER APEX_LISTENER IDENTIFIED BY \\\"%[1]s\\\" ACCOUNT UNLOCK;\" | %[2]s"

const CopyApexImages string = " ( while true; do  sleep 60; echo \"Copying Apex Images...\" ; done ) & mkdir -p /opt/oracle/oradata/${ORACLE_SID^^}_ORDS/apex/images && " +
	" cp -R /opt/oracle/oradata/${ORACLE_SID^^}/apex/images/* /opt/oracle/oradata/${ORACLE_SID^^}_ORDS/apex/images; chown -R oracle:oinstall /opt/oracle/oradata/${ORACLE_SID^^}_ORDS/apex; kill -9 $!;"

const ApexAdmin string = "BEGIN" +
	"\napex_util.set_security_group_id(p_security_group_id => 10); APEX_UTIL.REMOVE_USER(p_user_name => 'ADMIN');" +
	"\nCOMMIT;" +
	"\nEND;" +
	"\n/" +
	"\nBEGIN" +
	"\nAPEX_UTIL.create_user(p_user_name => 'ADMIN',p_email_address   => 'admin@oracle.com',p_web_password => '%[1]s',p_developer_privs => 'ADMIN',p_failed_access_attempts => '5' ," +
	" p_allow_app_building_yn => 'Y' ,p_allow_sql_workshop_yn => 'Y' ,p_allow_websheet_dev_yn => 'Y' , p_allow_team_development_yn => 'Y' , p_change_password_on_first_use  => 'N' );" +
	"apex_util.unlock_account(p_user_name => 'ADMIN'); APEX_UTIL.set_security_group_id( null );" +
	"\nCOMMIT;" +
	"\nEND;" +
	"\n/" +
	"\nALTER SESSION SET CONTAINER=%[2]s;" +
	"\nBEGIN" +
	"\napex_util.set_security_group_id(p_security_group_id => 10); APEX_UTIL.REMOVE_USER(p_user_name => 'ADMIN');" +
	"\nCOMMIT;" +
	"\nEND;" +
	"\n/" +
	"\nBEGIN" +
	"\nAPEX_UTIL.create_user(p_user_name => 'ADMIN',p_email_address   => 'admin@oracle.com',p_web_password => '%[1]s',p_developer_privs => 'ADMIN',p_failed_access_attempts => '5' ," +
	" p_allow_app_building_yn => 'Y' ,p_allow_sql_workshop_yn => 'Y' ,p_allow_websheet_dev_yn => 'Y' , p_allow_team_development_yn => 'Y' , p_change_password_on_first_use  => 'N' );" +
	"apex_util.unlock_account(p_user_name => 'ADMIN'); APEX_UTIL.set_security_group_id( null );" +
	"\nCOMMIT;" +
	"\nEND;" +
	"\n/"

// SetApexUsers is used to set Apex Users, pod that runs SetApexUsers is deleted and new ones is created.
const SetApexUsers string = "\numask 177" +
	"\necho db.username=APEX_LISTENER > apexlistener" +
	"\necho db.password=\"%[1]s\" >> apexlistener" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_al apexlistener" +
	"\nrm -f apexlistener" +
	"\necho db.username=APEX_REST_PUBLIC_USER > apexRestPublicUser" +
	"\necho db.password=\"%[1]s\" >> apexRestPublicUser" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex_rt apexRestPublicUser" +
	"\nrm -f apexRestPublicUser" +
	"\necho db.username=APEX_PUBLIC_USER > apexPublicUser" +
	"\necho db.password=\"%[1]s\" >> apexPublicUser" +
	"\n$JAVA_HOME/bin/java -jar $ORDS_HOME/ords.war set-properties --conf apex apexPublicUser" +
	"\nrm -f apexPublicUser" +
	"\numask 022"

// Get Sid, Pdbname, Edition for prebuilt db
const GetSidPdbEditionCMD string = "echo $ORACLE_SID,$ORACLE_PDB,$ORACLE_EDITION,Edition;"
