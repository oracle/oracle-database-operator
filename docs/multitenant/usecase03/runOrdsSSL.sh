#!/bin/bash

cat <<EOF
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
EOF

echo "ORDSVERSIN:$ORDSVERSION"

export ORDS=/usr/local/bin/ords
export ETCFILE=/etc/ords.conf
export CONFIG=`cat /etc/ords.conf |grep -v ^#|grep ORDS_CONFIG| awk -F= '{print $2}'`
export export _JAVA_OPTIONS="-Xms1126M -Xmx1126M"
export ERRORFOLDER=/opt/oracle/ords/error
export KEYSTORE=~/keystore
export OPENSSL=/usr/bin/openssl
export PASSFILE=${KEYSTORE}/PASSWORD
export HN=`hostname`
export KEY=$ORDS_HOME/secrets/$TLSKEY
export CERTIFICATE=$ORDS_HOME/secrets/$TLSCRT
export TNS_ADMIN=/opt/oracle/ords/
export TNSNAME=${TNS_ADMIN}/tnsnames.ora 
export TNSALIAS=ordstns
echo "${TNSALIAS}=${DBTNSURL}" >$TNSNAME


function SetParameter() {
  ##ords config info <--- Use this command to get the list

[[ ! -z "${ORACLE_HOST}" && -z "${DBTNSURL}" ]] && {
  $ORDS --config ${CONFIG} config    set   db.hostname                                 ${ORACLE_HOST:-racnode1} 
  $ORDS --config ${CONFIG} config    set   db.port                                     ${ORACLE_PORT:-1521} 
  $ORDS --config ${CONFIG} config    set   db.servicename                              ${ORACLE_SERVICE:-TESTORDS}
}

[[  -z "${ORACLE_HOST}" && ! -z "${DBTNSURL}" ]] && {
  #$ORDS --config ${CONFIG}  config    set   db.tnsAliasName                           ${TNSALIAS}
  #$ORDS --config ${CONFIG}  config    set   db.tnsDirectory                           ${TNS_ADMIN}
  #$ORDS --config ${CONFIG}  config    set   db.connectionType                         tns
 
   $ORDS --config ${CONFIG}  config    set   db.connectionType                         customurl
   $ORDS --config ${CONFIG}  config    set   db.customURL                              jdbc:oracle:thin:@${DBTNSURL}
}

  $ORDS --config ${CONFIG} config    set   security.requestValidationFunction          false   
  $ORDS --config ${CONFIG} config    set   jdbc.MaxLimit                               100
  $ORDS --config ${CONFIG} config    set   jdbc.InitialLimit                           50
  $ORDS --config ${CONFIG} config    set   error.externalPath                          ${ERRORFOLDER}
  $ORDS --config ${CONFIG} config    set   standalone.access.log                       /home/oracle
  $ORDS --config ${CONFIG} config    set   standalone.https.port                       8888
  $ORDS --config ${CONFIG} config    set   standalone.https.cert                       ${CERTIFICATE}
  $ORDS --config ${CONFIG} config    set   standalone.https.cert.key                   ${KEY}
  $ORDS --config ${CONFIG} config    set   restEnabledSql.active                       true
  $ORDS --config ${CONFIG} config    set   security.verifySSL                          true
  $ORDS --config ${CONFIG} config    set   database.api.enabled                        true
  $ORDS --config ${CONFIG} config    set   plsql.gateway.mode                          disabled
  $ORDS --config ${CONFIG} config    set   database.api.management.services.disabled   false
  $ORDS --config ${CONFIG} config    set   misc.pagination.maxRows                     1000
  $ORDS --config ${CONFIG} config    set   db.cdb.adminUser                            "${CDBADMIN_USER:-C##DBAPI_CDB_ADMIN} AS SYSDBA"
  $ORDS --config ${CONFIG} config    secret --password-stdin db.cdb.adminUser.password << EOF
${CDBADMIN_PWD:-PROVIDE_A_PASSWORD}
EOF

$ORDS --config ${CONFIG} config  user add --password-stdin ${WEBSERVER_USER:-ordspdbadmin} "SQL Administrator, System Administrator" <<EOF
${WEBSERVER_PASSWORD:-welcome1}
EOF

}


function setupOrds() {

echo "===================================================="
echo CONFIG=$CONFIG

export ORDS_LOGS=/tmp

 [ -f $ORDS_HOME/secrets/$WEBSERVER_USER_KEY ]     && 
  {
    WEBSERVER_USER=`cat $ORDS_HOME/secrets/$WEBSERVER_USER_KEY` 
  }

 [ -f $ORDS_HOME/secrets/$WEBSERVER_PASSWORD_KEY ] && 
  { 
    WEBSERVER_PASSWORD=`cat $ORDS_HOME/secrets/$WEBSERVER_PASSWORD_KEY` 
  }

 [ -f $ORDS_HOME/secrets/$CDBADMIN_USER_KEY ]      && 
  {
     CDBADMIN_USER=`cat $ORDS_HOME/secrets/$CDBADMIN_USER_KEY` 
  }

 [ -f $ORDS_HOME/secrets/$CDBADMIN_PWD_KEY ]       && 
  { 
     CDBADMIN_PWD=`cat $ORDS_HOME/secrets/$CDBADMIN_PWD_KEY` 
  }

 [ -f $ORDS_HOME/secrets/$ORACLE_PWD_KEY ]         && 
  { 
    SYSDBA_PASSWORD=`cat $ORDS_HOME/secrets/$ORACLE_PWD_KEY`
   }

 [ -f $ORDS_HOME/secrets/$ORACLE_PWD_KEY ]         && 
  { 
    ORDS_PASSWORD=`cat $ORDS_HOME/secrets/$ORDS_PWD_KEY`
  }


SetParameter;
$ORDS --config                           ${CONFIG} install                 \
      --admin-user                       ${SYSDBA_USER:-"SYS AS SYSDBA"}   \
      --feature-db-api                   true                              \
      --feature-rest-enabled-sql         true                              \
      --log-folder                       ${ORDS_LOGS}                      \
      --proxy-user                                                         \
      --password-stdin <<EOF
${SYSDBA_PASSWORD:-PROVIDE_A_PASSWORD}
${ORDS_PASSWORD:-PROVIDE_A_PASSWORD}
EOF



if [ $? -ne 0 ]
then
  echo "Installation error"
  exit 1
fi

}

export CKF=/tmp/checkfile

$ORDS --config $CONFIG config list 1>${CKF} 2>&1 
echo "checkfile" >> ${CKF}
NOT_INSTALLED=`cat ${CKF} | grep "INFO: The" |wc -l `
echo NOT_INSTALLED=$NOT_INSTALLED


function StartUp () {
  $ORDS  --config $CONFIG serve     --port 8888 --secure
}

# Check whether ords is already setup
if [ $NOT_INSTALLED -ne 0 ]
then
  echo " SETUP "
  setupOrds;
  StartUp;
fi

if [ $NOT_INSTALLED -eq 0 ]
then
  echo " STARTUP "
  StartUp;
fi


