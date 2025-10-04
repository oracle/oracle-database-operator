#!/bin/bash
## Copyright (c) 2006, 2024, Oracle and/or its affiliates.
##
## The Universal Permissive License (UPL), Version 1.0
##
## Subject to the condition set forth below, permission is hereby granted to any
## person obtaining a copy of this software, associated documentation and/or data
## (collectively the "Software"), free of charge and under any and all copyright
## rights in the Software, and any and all patent rights owned or freely
## licensable by each licensor hereunder covering either (i) the unmodified
## Software as contributed to or provided by such licensor, or (ii) the Larger
## Works (as defined below), to deal in both
##
## (a) the Software, and
## (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
## one is included with the Software (each a "Larger Work" to which the Software
## is contributed by such licensors),
##
## without restriction, including without limitation the rights to copy, create
## derivative works of, display, perform, and distribute the Software and make,
## use, sell, offer for sale, import, export, have made, and have sold the
## Software and the Larger Work(s), and to sublicense the foregoing rights on
## either these or other terms.
##
## This license is subject to the following condition:
## The above copyright notice and either this complete permission notice or at
## a minimum a reference to the UPL must be included in all copies or
## substantial portions of the Software.
##
## THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
## IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
## FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
## AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
## LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
## OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
## SOFTWARE.

# not used, avoid messages
unset JAVA_TOOL_OPTIONS

dump_stack(){
_log_date=`date "+%y:%m:%d %H:%M:%S"`
    local frame=0
    local line_no
    local function_name
    local file_name
    echo -e "BACKTRACE [${_log_date}]\n"
    echo -e "filename:line\tfunction "
    echo -e "-------------   --------"
    while caller $frame ;do ((frame++)) ;done | \
    while read line_no function_name file_name;\
    do echo -e "$file_name:$line_no\t$function_name" ;done >&2
}



get_conn_string() {
	local -n _conn_string="${1}"

	echo "== Prepare connect string"

	local -r _admin_user=$($ords_cfg_cmd get --secret db.adminUser | tail -1)
	local _conn_type=$($ords_cfg_cmd get db.connectionType |tail -1)
	if [[ $_conn_type == "customurl" ]]; then
		local -r _conn=$($ords_cfg_cmd get db.customURL | tail -1)
	elif [[ $_conn_type == "tns" ]]; then
		local -r _tns_service=$($ords_cfg_cmd get db.tnsAliasName | tail -1)
		local -r _conn=${_tns_service}
	elif [[ $_conn_type == "basic" ]]; then
		local -r _host=$($ords_cfg_cmd get db.hostname | tail -1)
		local -r _port=$($ords_cfg_cmd get db.port | tail -1)
		local -r _service=$($ords_cfg_cmd get db.servicename | tail -1)
		local -r _sid=$($ords_cfg_cmd get db.sid | tail -1)

		if [[ -n ${_host} ]] && [[ -n ${_port} ]]; then
			if [[ -n ${_service} ]] || [[ -n ${_sid} ]]; then
				local -r _conn=${_host}:${_port}/${_service:-$_sid}
			fi
		fi
	else 
		# wallet
		_conn_type="wallet"
		local -r _wallet_service=$($ords_cfg_cmd get db.wallet.zip.service | tail -1)
		local -r _conn=${_wallet_service}
	fi

	if [[ -n ${_conn} ]]; then
		echo "Connection String (${_conn_type}): ${_conn}"
		_conn_string="${_admin_user%%/ *}/${config["dbadminuserpassword"]}@${_conn}"
		if [[ ${_admin_user%%/ *} == "SYS" ]]; then
			_conn_string="${_conn_string=} AS SYSDBA"
		fi
	fi
}

#------------------------------------------------------------------------------
function setup_sql(){
	local -r _conn_string="${1}"

	echo "== Configuring sql environment"

        ## Get TNS_ADMIN location
        local -r _tns_admin=$($ords_cfg_cmd get db.tnsDirectory 2>&1| tail -1)
        if [[ ! $_tns_admin =~ "Cannot get setting" ]]; then
                echo "Setting: TNS_ADMIN=${_tns_admin}"
                export TNS_ADMIN=${_tns_admin}
        fi

        ## Get ADB Wallet
        #echo "Checking db.wallet.zip.path"
        #echo "$ords_cfg_cmd get db.wallet.zip.path"
        local -r _wallet_zip_path=$($ords_cfg_cmd get db.wallet.zip.path 2>&1| tail -1)
        #echo "wallet_zip_path : \"${_wallet_zip_path}\""
        if [[ ! $_wallet_zip_path =~ "Cannot get setting" ]]; then
                echo "Using: set cloudconfig ${_wallet_zip_path}"
                local -r _cloudconfig="set cloudconfig ${_wallet_zip_path}"
        fi

	return 0

}

function run_sql {
	local -r _conn_string="${1}"
	local -r _sql="${2}"
	local -n _output="${3}"
	local -i _rc=0
	
	if [[ -z ${_sql} ]]; then
		dump_stack
		echo "FATAL: missing SQL calling run_sql" && exit 1
	fi

	echo "Running SQL"

	# NOTE to maintainer; the heredoc must be TAB indented
	_output=$(sql -S -nohistory -noupdates /nolog <<-EOSQL
		WHENEVER SQLERROR EXIT 1
		WHENEVER OSERROR EXIT 1
		${_cloudconfig}
		connect $_conn_string
		set serveroutput on echo off pause off feedback off
		set heading off wrap off linesize 1000 pagesize 0
		SET TERMOUT OFF VERIFY OFF
		${_sql}
		exit;
		EOSQL
	)
	_rc=$?

	if (( ${_rc} > 0 )); then
		dump_stack
		echo "SQLERROR: ${_output}"
	fi
	
	return $_rc
}

#------------------------------------------------------------------------------
function check_adb() {
	local -r _conn_string=$1
	local -n _is_adb=$2

	echo "== ADB check"

	local -r _adb_chk_sql="
		DECLARE
			invalid_column exception;
			pragma exception_init (invalid_column,-00904);
			adb_check integer;
		BEGIN
			EXECUTE IMMEDIATE q'[SELECT COUNT(*) FROM (
			SELECT JSON_VALUE(cloud_identity, '\$.DATABASE_OCID') AS database_ocid 
			  FROM v\$pdbs) t
			 WHERE t.database_ocid like '%AUTONOMOUS%']' INTO adb_check;
			DBMS_OUTPUT.PUT_LINE(adb_check);
		EXCEPTION WHEN invalid_column THEN
			DBMS_OUTPUT.PUT_LINE('0');
		END;
		/"
	echo "Checking if Database is an ADB"
	run_sql "${_conn_string}" "${_adb_chk_sql}" "_adb_check"
	_rc=$?

	if (( ${_rc} == 0 )); then
		_adb_check=${_adb_check//[[:space:]]/}
		if (( ${_adb_check} == 1 )); then
			_is_adb=${_adb_check//[[:space:]]/}
			echo "ADB : yes"
		else
			echo "ADB : no"
		fi
	else
		echo "ADB check failed"
	fi

	return ${_rc}
}

function create_adb_user() {
	local -r _conn_string="${1}"
	local -r _pool_name="${2}"
                        
	local _config_user=$($ords_cfg_cmd get db.username | tail -1)

	if [[ -z ${_config_user} ]] || [[ ${_config_user} == "ORDS_PUBLIC_USER" ]]; then
		echo "FATAL: You must specify a db.username <> ORDS_PUBLIC_USER in pool \"${_pool_name}\""
		dump_stack
		return 1
	fi

	local -r _adb_user_sql="
    DECLARE
      l_user VARCHAR2(255);
      l_cdn  VARCHAR2(255);
    BEGIN
      BEGIN
        SELECT USERNAME INTO l_user FROM DBA_USERS WHERE USERNAME='${_config_user}';
        EXECUTE IMMEDIATE 'ALTER USER \"${_config_user}\" PROFILE ORA_APP_PROFILE';
        EXECUTE IMMEDIATE 'ALTER USER \"${_config_user}\" IDENTIFIED BY \"${config["dbpassword"]}\"';
		DBMS_OUTPUT.PUT_LINE('${_config_user} Exists - Password reset');
      EXCEPTION
        WHEN NO_DATA_FOUND THEN
          EXECUTE IMMEDIATE 'CREATE USER \"${_config_user}\" IDENTIFIED BY \"${config["dbpassword"]}\" PROFILE ORA_APP_PROFILE';
		  DBMS_OUTPUT.PUT_LINE('${_config_user} Created');
      END;
      EXECUTE IMMEDIATE 'GRANT CONNECT TO \"${_config_user}\"';
      BEGIN
        SELECT USERNAME INTO l_user FROM DBA_USERS WHERE USERNAME='ORDS_PLSQL_GATEWAY_OPER';
          EXECUTE IMMEDIATE 'ALTER USER \"ORDS_PLSQL_GATEWAY_OPER\" PROFILE DEFAULT';
          EXECUTE IMMEDIATE 'ALTER USER \"ORDS_PLSQL_GATEWAY_OPER\" NO AUTHENTICATION';
		  DBMS_OUTPUT.PUT_LINE('ORDS_PLSQL_GATEWAY_OPER Exists');
        EXCEPTION
          WHEN NO_DATA_FOUND THEN
            EXECUTE IMMEDIATE 'CREATE USER \"ORDS_PLSQL_GATEWAY_OPER\" NO AUTHENTICATION PROFILE DEFAULT';
			DBMS_OUTPUT.PUT_LINE('ORDS_PLSQL_GATEWAY_OPER Created');
      END;
      EXECUTE IMMEDIATE 'GRANT CONNECT TO \"ORDS_PLSQL_GATEWAY_OPER\"';
      EXECUTE IMMEDIATE 'ALTER USER \"ORDS_PLSQL_GATEWAY_OPER\" GRANT CONNECT THROUGH \"${_config_user}\"';
      ORDS_ADMIN.PROVISION_RUNTIME_ROLE (
          p_user => '${_config_user}'
        ,p_proxy_enabled_schemas => TRUE
      );
      ORDS_ADMIN.CONFIG_PLSQL_GATEWAY (
          p_runtime_user => '${_config_user}'
        ,p_plsql_gateway_user => 'ORDS_PLSQL_GATEWAY_OPER'
      );
	  -- TODO: Only do this if ADB APEX Version <> this ORDS Version
      BEGIN
        SELECT images_version INTO L_CDN
          FROM APEX_PATCHES
        where is_bundle_patch = 'Yes'
        order by patch_version desc
        fetch first 1 rows only;
      EXCEPTION WHEN NO_DATA_FOUND THEN
        select version_no INTO L_CDN
          from APEX_RELEASE;
      END;
      apex_instance_admin.set_parameter(
          p_parameter => 'IMAGE_PREFIX',
          p_value     => 'https://static.oracle.com/cdn/apex/'||L_CDN||'/'
      );
    END;
	/"

	run_sql "${_conn_string}" "${_adb_user_sql}" "_adb_user_sql_output"
	_rc=$?

	echo "Installation Output: ${_adb_user_sql_output}"
	return ${_rc}
}

#------------------------------------------------------------------------------
function apex_compare_versions() {
	local _db_ver=$1
	local _im_ver=$2

	echo "database APEX version : $_db_ver"
	echo "install  APEX version : $_im_ver"

	IFS='.' read -r -a _db_ver_array <<< "$_db_ver"
	IFS='.' read -r -a _im_ver_array <<< "$_im_ver"

	# Compare each component
	local i
	for i in "${!_db_ver_array[@]}"; do
		if [[ "${_db_ver_array[$i]}" -lt "${_im_ver_array[$i]}" ]]; then
		# _db_ver < _im_ver (upgrade)
			return 0
		elif [[ "${_db_ver_array[$i]}" -gt "${_im_ver_array[$i]}" ]]; then
		# _db_ver < _im_ver (do nothing)
			return 1
		fi
	done
	# _db_ver == __im_ver (do nothing)
	return 1
}

#------------------------------------------------------------------------------
ords_client_version(){
	echo "== ORDS client version" 
	echo "ords_client_version : \"$(ords --config $ORDS_CONFIG --version 2>&1 | tail -1)\""
}

#------------------------------------------------------------------------------
set_ords_secret() {
	local -r _pool_name="${1}"
	local -r _config_key="${2}"
	local -r _config_val="${3}"
	local -i _rc=0

	if [[ -n "${_config_val}" ]]; then
		echo "Setting ${_config_key} in pool \"${_pool_name}\""
		ords --config "$ORDS_CONFIG" config --db-pool "${_pool_name}" secret --password-stdin "${_config_key}" 2>&1 <<< "${_config_val}"|tail -1
		_rc=$?
	else
		echo "${_config_key} in pool \"${_pool_name}\" is not defined"
		_rc=0
	fi

	return ${_rc}
}

#------------------------------------------------------------------------------
read_passwords(){
        echo "== Reading passwords"
        for key in dbpassword dbadminuserpassword dbcdbadminuserpassword; do
                var_key="${pool_name//-/_}_${key}"
                echo "Obtaining value from initContainer variable: ${var_key}"
                var_val="${!var_key}"
                config[${key}]="${var_val}"
        done

        # Set ORDS Secrets
        set_ords_secret "${pool_name}" "db.password" "${config["dbpassword"]}"
        rc=$((rc + $?))
        set_ords_secret "${pool_name}" "db.adminUser.password" "${config["dbadminuserpassword"]}"
        rc=$((rc + $?))
        set_ords_secret "${pool_name}" "db.cdb.adminUser.password" "${config["dbcdbadminuserpassword"]}"
        rc=$((rc + $?))

        if (( ${rc} > 0 )); then
                echo "FATAL: Unable to set configuration for pool \"${pool_name}\""
		return 1
        elif [[ -z ${config["dbpassword"]} ]]; then
                echo "FATAL: db.password must be specified for pool \"${pool_name}\""
		return 1
        elif [[ -z ${config["dbadminuserpassword"]} ]]; then
                echo "INFO: No additional configuration for pool \"${pool_name}\""
        fi

	return 0
}

#------------------------------------------------------------------------------
ords_upgrade() {
        local -r _pool_name="${1}"
        local -r _upgrade_key="${2}"
        local -i _rc=0

	echo "== ORDS install/upgrade"

        if [[ -n "${config["dbadminuserpassword"]}" ]]; then
                # Get usernames
                local -r ords_user=$($ords_cfg_cmd get db.username | tail -1)
                local -r ords_admin=$($ords_cfg_cmd get db.adminUser | tail -1)

                echo "Performing ORDS install/upgrade as $ords_admin into $ords_user on pool \"${_pool_name}\""
                if [[ ${_pool_name} == "default" ]]; then
                        ords --config "$ORDS_CONFIG" install --db-only \
                                --admin-user "$ords_admin" --password-stdin <<< "${config["dbadminuserpassword"]}"
                        _rc=$?
                else
                        ords --config "$ORDS_CONFIG" install --db-pool "${_pool_name}" --db-only \
                                --admin-user "$ords_admin" --password-stdin <<< "${config["dbadminuserpassword"]}"
                        _rc=$?
                fi

                # Dar be bugs below deck with --db-user so using the above
                # ords --config "$ORDS_CONFIG" install --db-pool "$1" --db-only \
                #       --admin-user "$ords_admin" --db-user "$ords_user" --password-stdin <<< "${!2}"
        fi

        return $_rc
}


#------------------------------------------------------------------------------
function global_parameters(){

	APEX_INSTALL=/opt/oracle/apex
	# backward compatibility for ORDS images prior to 24.1.x (included)
	# APEX_HOME is used only here and it is set on images <= 24.1.x
	if [[ -n ${APEX_HOME} ]]; then
		APEX_INSTALL=${APEX_HOME}/${APEX_VER}
		echo "WARNING: APEX installation ${APEX_INSTALL}, ORDS image probably older than 24.2"
	fi	
	APEXINS=${APEX_INSTALL}/apexins.sql
	APEX_IMAGES=${APEX_INSTALL}/images
    APEX_VERSION_TXT=${APEX_IMAGES}/apex_version.txt 

	echo "== global parameters"
	echo "external_apex          : ${external_apex}"
	echo "download_apex          : ${download_apex}"
	echo "download_url_apex      : ${download_url_apex}"
	echo "APEX_INSTALL           : $APEX_INSTALL"
	echo "APEX_IMAGES            : $APEX_IMAGES"

}


#------------------------------------------------------------------------------
function get_apex_version() {
	local -r _conn_string="${1}"
	local -n _db_apex_version="${2}"
	local -i _rc=0

	echo "== APEX version check"

	local -r _ver_sql="SELECT VERSION FROM DBA_REGISTRY WHERE COMP_ID='APEX';"
	#local -r _ver_sql="SELECT SCHEMA FROM DBA_REGISTRY WHERE COMP_ID='APEX';"
	run_sql "${_conn_string}" "${_ver_sql}" "_db_apex_version"
	_rc=$?

	if (( $_rc > 0 )); then
		echo "FATAL: Unable to get APEX version"
		dump_stack
		return $_rc
	fi

	_db_apex_version=${_db_apex_version//[^0-9.]/}
	#_db_apex_version="${_db_apex_version//[[:space:]]}"
	if [[ -z "${_db_apex_version}" ]]; then
		_db_apex_version="NotInstalled"
	fi	
	echo "Database APEX Version: ${_db_apex_version}"

	return $_rc
}

#------------------------------------------------------------------------------
function get_apex_action(){
	local -r _conn_string="${1}"
        local -r _db_apex_version="${2}"
        local -n _action="${3}"
        local -i _rc=0

	echo "== APEX installation check"

	_action="error"
	if [[ ( -z "${_db_apex_version}" ) || ( "${_db_apex_version}" == "NotInstalled" ) ]]; then
                echo "Installing APEX ${APEX_VER}"
                _action="install"
        elif apex_compare_versions ${_db_apex_version} ${APEX_VER}; then
                echo "Upgrading from ${_db_apex_version} to ${APEX_VER}"
                _action="upgrade"
        else
                echo "No Installation/Upgrade Required"
                _action="none"
        fi

	return $_rc
}

#------------------------------------------------------------------------------
function check_apex_installation_version(){

    echo "== APEX installation files"

   	if [[ !(-f ${APEX_VERSION_TXT}) ]]; then
		echo "ERROR: ${APEX_VERSION_TXT} not found, APEX installation not found"
		return 1
	fi

	APEX_VER=$(cat ${APEX_VERSION_TXT}|grep Version|cut -f2 -d\:|tr -d '[:space:]')
	echo "APEX_VER: ${APEX_VER}"
}

#------------------------------------------------------------------------------
function apex_external(){
	echo "== APEX external"

	if [[ ${external_apex} != "true" ]]; then
		echo "APEX external disabled"
		return 0
	fi

    id
	df -h $APEX_INSTALL
	ls -ld $APEX_INSTALL
	ls -l $APEX_INSTALL
	ls -l ${APEX_VERSION_TXT}
    echo test >> $APEX_INSTALL/test.txt
	ls -l $APEX_INSTALL/test.txt

    while true
    do
            date +"%Y-%m-%d %H:%M:%S"
            if [[ -f ${APEX_VERSION_TXT} ]]
            then
              echo Found images/apex_version.txt
			  break
            else
              if [[ -f ${APEX_INSTALL}/apex.zip ]]
              then
			    date
                echo Found ${APEX_INSTALL}/apex.zip, extracting ...
                cd ${APEX_INSTALL}
                jar xf apex.zip
                mv ${APEX_INSTALL}/apex/* ${APEX_INSTALL}
				date
				echo completed
              else
			    date
                echo "Missing ${APEX_INSTALL}/apex.zip, manually copy apex.zip in ${APEX_INSTALL} on the init container of the pod"
				echo "e.g. kubectl cp /tmp/apex.zip ordssrvs-697c5698d9-q8gxf:/opt/oracle/apex/ -n testcase -c ordssrvs-init"
                sleep 5
              fi
            fi
    done

}

#------------------------------------------------------------------------------
function apex_download(){

	echo "== APEX download"

	if [[ ${download_apex} != "true" ]]; then
		echo "APEX download disabled"
		return 0
	fi

	mkdir -p ${APEX_INSTALL}
	rm -rf ${APEX_INSTALL}/*
	cd /tmp
	echo "Downloading ${download_url_apex}"
	curl -o apex.zip ${download_url_apex}
	echo "Extracting apex.zip"
	jar xf apex.zip
       	mv /tmp/apex/* ${APEX_INSTALL}

	if [[ !(-f $APEXINS) ]]; then
		echo "ERROR: ${APEXINS} not found, APEX download failed"
		return 1
	fi
	echo "APEX_INSTALL: ${APEX_INSTALL}"

	# config can be read-only, it will be set again at command-line ords start
	echo "== Configuring ORDS images"
	echo "APEX_IMAGES: ${APEX_IMAGES}"
	ords config set standalone.static.path ${APEX_IMAGES}

	return 0
}

#------------------------------------------------------------------------------
function apex_upgrade() {
	local -r _conn_string="${1}"
	local -r _upgrade_key="${2}"
	local -i _rc=0

	echo "== APEX Installation/Upgrade"

	if [[ -z ${APEX_INSTALL} ]]; then
		echo "ERROR: APEX_INSTALL not set"
		return 1
	fi	

	if [[ !( -f ${APEX_INSTALL}/apexins.sql ) ]]; then
		echo "ERROR: ${APEX_INSTALL}/apexins.sql not found"
		return 1
	fi


	if [[ "${!_upgrade_key}" = "true" ]]; then
		echo "Starting Installation of APEX"
		cd ${APEX_INSTALL}
		SEC=${config["dbpassword"]}
		local -r _install_sql="@apxsilentins.sql SYSAUX SYSAUX TEMP /i/ $SEC $SEC $SEC $SEC"
		run_sql "${_conn_string}" "${_install_sql}" "_install_output"
		_rc=$?
		echo "Installation Output: ${_install_output}"
	fi

	return $_rc
}

#------------------------------------------------------------------------------
function apex_housekeeping(){

	echo "== APEX "

	# check database APEX version regardless of APEX parameters
	get_apex_version "${conn_string}" "db_apex_version"
        if [[ -z ${db_apex_version} ]]; then
                echo "FATAL: Unable to get APEX Version for pool \"${pool_name}\""
                return 1
        fi

	# check if apex upgrade is enabled
        if [[ ${apex_upgrade} != "true" ]]; then
                echo "APEX Install/Upgrade not requested for pool \"${pool_name}\""
                return 0
        fi

	# get suggested action
        get_apex_action "${conn_string}" "${db_apex_version}" "db_apex_action"
        if [[ -z ${db_apex_action} ]]; then
                echo "FATAL: Unable to get APEX suggested action for pool \"${pool_name}\""
                return 1
        fi

	# upgrade
        echo "APEX version : \"${db_apex_version}\""
        echo "APEX suggested action : $db_apex_action"
        if [[ ${db_apex_action} != "none" ]]; then
                apex_upgrade "${conn_string}" "${pool_name}_autoupgrade_apex"
                if (( $? > 0 )); then
                        echo "FATAL: Unable to ${db_apex_action} APEX for pool \"${pool_name}\""
			return 1
                fi
        fi


}



#------------------------------------------------------------------------------
function pool_parameters(){

	echo "== pool parameters"
        apex_upgrade_var=${pool_name}_autoupgrade_apex
        apex_upgrade=${!apex_upgrade_var}
	[[ -z $apex_upgrade ]] && apex_upgrade=false
        echo "${pool_name} - autoupgrade_apex  : ${apex_upgrade}"

        ords_upgrade_var=${pool_name}_autoupgrade_ords
        ords_upgrade=${!ords_upgrade_var}
	[[ -z $ords_upgrade ]] && ords_upgrade=false
        echo "${pool_name} - autoupgrade_ords  : ${ords_upgrade}"

}


#------------------------------------------------------------------------------
# INIT
#------------------------------------------------------------------------------
declare -A pool_exit
echo "=== ORDSSRVS init ==="
global_parameters
ords_client_version
apex_download
apex_external

# check APEX installation files version, downloaded or mounted by PVC
check_apex_installation_version

for pool in "$ORDS_CONFIG"/databases/*; do
	rc=0
	pool_name=$(basename "$pool")
	pool_exit[${pool_name}]=0
	ords_cfg_cmd="ords --config $ORDS_CONFIG config --db-pool ${pool_name}"
	echo "=========================================================================="
	echo "Pool: $pool_name"
        declare -A config

	pool_parameters

	read_passwords
	rc=$?
	if (( ${rc} > 0 )); then
                pool_exit[${pool_name}]=1
                continue
        fi

	get_conn_string "conn_string"
	if [[ -z ${conn_string} ]]; then
		echo "FATAL: Unable to get database connect string for pool \"${pool_name}\""
                dump_stack
		pool_exit[${pool_name}]=1
		continue
	fi

	setup_sql "${conn_string}"
        rc=$?
        if (( ${rc} > 0 )); then
                pool_exit[${pool_name}]=1
                continue
        fi

	check_adb "${conn_string}" "is_adb"
	rc=$?
	if (( ${rc} > 0 )); then
		pool_exit[${pool_name}]=1
		continue
	fi

	if (( is_adb )); then
		# Create ORDS User
		echo "Processing ADB in Pool: ${pool_name}"
		create_adb_user "${conn_string}" "${pool_name}"
		continue
	fi	

	# not ADB

	# APEX 
	apex_housekeeping
	rc=$?
	if (( ${rc} > 0 )); then
		echo "FATAL: unable to manage APEX configuration for pool \"${pool_name}\""
		dump_stack
		pool_exit[${pool_name}]=1
		continue
	fi

	# database ORDS 
	echo "== ORDS"
	if [[ ${ords_upgrade} == "true" ]]; then
		ords_upgrade "${pool_name}" "${pool_name}_autoupgrade_ords"
		rc=$?
		if (( $rc > 0 )); then
			echo "FATAL: Unable to perform requested ORDS install/upgrade on pool \"${pool_name}\""
			pool_exit[${pool_name}]=1
			dump_stack
		fi	
	else
		echo "ORDS Install/Upgrade not requested for pool \"${pool_name}\""
	fi
done

echo "=========================================================================="
echo "Exit codes"
for key in "${!pool_exit[@]}"; do
    echo "Pool: $key, Exit Code: ${pool_exit[$key]}"
	if (( ${pool_exit[$key]} > 0 )); then
		rc=1
	fi
done

exit $rc
#exit 0
