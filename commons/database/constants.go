/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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

const NoCloneRef string = "Unavailable"

const GetVersionSQL string = "SELECT VERSION_FULL FROM V\\$INSTANCE;"

const CheckModesSQL string = "SELECT 'log_mode:' || log_mode AS log_mode ,'flashback_on:' || flashback_on AS flashback_on ,'force_logging:' || force_logging AS force_logging FROM v\\$database;"

const ListPdbSQL string = "SELECT NAME FROM V\\$PDBS;"

const CreateChkFileCMD string = "touch \"${ORACLE_BASE}/oradata/.${ORACLE_SID}.nochk\" && sync"

const RemoveChkFileCMD string = "rm -f \"${ORACLE_BASE}/oradata/.${ORACLE_SID}.nochk\""

const CreateDBRecoveryDestCMD string = "mkdir -p ${ORACLE_BASE}/oradata/fast_recovery_area"

const ConfigureOEMSQL string = "exec DBMS_XDB_CONFIG.SETHTTPSPORT(5500);" +
	"\nalter system register;"

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

const GetDatabaseRoleCMD string = "SELECT DATABASE_ROLE FROM V\\$DATABASE; "

const DataguardBrokerGetDatabaseCMD string = "SELECT DATABASE || ':' || DATAGUARD_ROLE AS DATABASE FROM V\\$DG_BROKER_CONFIG;"

const RunDatapatchCMD string = " ( while true; do  sleep 60; echo \"Installing patches...\" ; done ) & if ! $ORACLE_HOME/OPatch/datapatch -skip_upgrade_check;" +
	" then echo \"Datapatch execution has failed.\" ; else echo \"DONE: Datapatch execution.\" ; fi ; kill -9 $!;"

const GetSqlpatchDescriptionSQL string = "select TARGET_VERSION || ' (' || PATCH_ID || ')' as patchinfo  from dba_registry_sqlpatch order by action_time desc;"

const GetSqlpatchStatusSQL string = "select status from dba_registry_sqlpatch order by action_time desc;"

const GetSqlpatchVersionSQL string = "select SOURCE_VERSION || ':' || TARGET_VERSION as versions from dba_registry_sqlpatch order by action_time desc;"

const GetCheckpointFileCMD string = "find ${ORACLE_BASE}/oradata -name .${ORACLE_SID}${CHECKPOINT_FILE_EXTN} "

const GetEnterpriseEditionFileCMD string = "if [ -f ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_enterprise ]; then ls ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_enterprise; fi "

const GetStandardEditionFileCMD string = "if [ -f ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_standard ]; then ls ${ORACLE_BASE}/oradata/dbconfig/$ORACLE_SID/.docker_standard; fi "

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

const AlterSgaPgaCpuCMD string = "echo -e  \"alter system set sga_target=%dM scope=both; \n alter system set pga_aggregate_target=%dM scope=both; \n alter system set cpu_count=%d; \" | %s "

const AlterProcessesCMD string = "echo -e  \"alter system set processes=%d scope=spfile; \" | %s && " + CreateChkFileCMD + " && " +
	"echo -e  \"SHUTDOWN IMMEDIATE; \n STARTUP MOUNT; \n ALTER DATABASE OPEN; \n ALTER PLUGGABLE DATABASE ALL OPEN; \n ALTER SYSTEM REGISTER;\" | %s && " +
	RemoveChkFileCMD

const GetInitParamsSQL string = "echo -e  \"select name,display_value from v\\$parameter  where name in  ('sga_target','pga_aggregate_target','cpu_count','processes') order by name asc;\" | %s"

const UnzipApex string = "if [ -f /opt/oracle/oradata/apex-latest.zip ]; then unzip -o /opt/oracle/oradata/apex-latest.zip -d /opt/oracle/oradata/${ORACLE_SID^^}; else echo \"apex-latest.zip not found\"; fi;"

const ChownApex string = " chown oracle:oinstall /opt/oracle/oradata/${ORACLE_SID^^}/apex;"

const InstallApex string = "if [ -f /opt/oracle/oradata/${ORACLE_SID^^}/apex/apexins.sql ]; then  ( while true; do  sleep 60; echo \"Installing Apex...\" ; done ) & " +
	" cd /opt/oracle/oradata/${ORACLE_SID^^}/apex && echo -e \"@apexins.sql SYSAUX SYSAUX TEMP /i/\" | %[1]s && kill -9 $!; else echo \"Apex Folder doesn't exist\" ; fi ;"

const IsApexInstalled string = "select 'APEXVERSION:'||version as version FROM DBA_REGISTRY WHERE COMP_ID='APEX';"

const UninstallApex string = "if [ -f /opt/oracle/oradata/${ORACLE_SID^^}/apex/apxremov.sql ]; then  ( while true; do  sleep 60; echo \"Uninstalling Apex...\" ; done ) & " +
	" cd /opt/oracle/oradata/${ORACLE_SID^^}/apex && echo -e \"@apxremov.sql\" | %[1]s && kill -9 $!; else echo \"Apex Folder doesn't exist\" ; fi ;"
