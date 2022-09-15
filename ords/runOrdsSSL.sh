#!/bin/bash
#
# Since: June, 2022
# Author: matteo.malvezzi@oracle.com
# Description: Setup and runs Oracle Rest Data Services 22.2.
#
# DO NOT ALTER OR REMOVE COPYRIGHT NOTICES OR THIS HEADER.
#
# Copyright (c) 2014-2017 Oracle and/or its affiliates. All rights reserved.
#
#    MODIFIED    (DD-Mon-YY)
#    mmalvezz    25-Jun-22   - Initial version

export ORDS=/usr/local/bin/ords
export ETCFILE=/etc/ords.conf
export CONFIG=`cat /etc/ords.conf |grep -v ^#|grep ORDS_CONFIG| awk -F= '{print $2}'`
export export _JAVA_OPTIONS="-Xms1126M -Xmx1126M"
export ERRORFOLDER=/opt/oracle/ords/error
export KEYSTORE=~/keystore
export OPENSSL=/usr/bin/openssl
export PASSFILE=${KEYSTORE}/PASSWORD
export HN=`hostname`
#export KEY=${KEYSTORE}/${HN}-key.der
#export CERTIFICATE=${KEYSTORE}/${HN}.der
export KEY=$ORDS_HOME/secrets/$TLSKEY
export CERTIFICATE=$ORDS_HOME/secrets/$TLSCRT

export CUSTOMURL="jdbc:oracle:thin:@(DESCRIPTION = (ADDRESS = (PROTOCOL = TCP)(HOST = racnode1)(PORT = 1521)) (CONNECT_DATA = (SERVER = DEDICATED) (SERVICE_NAME = TESTORDS)))"
echo $CUSTOMURL

function SetParameter() {
  ##ords config info <--- Use this command to get the list

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
  $ORDS --config ${CONFIG} config    set   plsql.gateway.mode                          false
  $ORDS --config ${CONFIG} config    set   database.api.management.services.disabled   false
  $ORDS --config ${CONFIG} config    set   misc.pagination.maxRows                     1000
  $ORDS --config ${CONFIG} config    set   db.cdb.adminUser                            "${CDBADMIN_USER:-C##DBAPI_CDB_ADMIN} AS SYSDBA"
  $ORDS --config ${CONFIG} config    secret --password-stdin db.cdb.adminUser.password << EOF
${CDBADMIN_PWD:-WElcome_12##}
EOF

##  $ORDS --config ${CONFIG} config  set db.username  "SYS  AS SYSDBA"
##  $ORDS --config ${CONFIG} config  secret --password-stdin db.password <<EOF
## WElcome_12##
## EOF

  $ORDS --config ${CONFIG} config  user add --password-stdin ${WEBSERVER_USER:-ordspdbadmin} "SQL Administrator, System Administrator" <<EOF
${WEBSERVER_PASSWORD:-welcome1}
EOF

}


function setupHTTPS() {

rm -rf  ${KEYSTORE}


[ ! -d ${KEYSTORE} ] && {
   mkdir ${KEYSTORE}
}

cd $KEYSTORE

cat <<EOF  >$PASSFILE
welcome1
EOF

## $JAVA_HOME/bin/keytool -genkey -keyalg RSA -alias selfsigned -keystore keystore.jks \
##   -dname "CN=${HN}, OU=Example Department, O=Example Company, L=Birmingham, ST=West Midlands, C=GB" \
##   -storepass welcome1 -validity 3600 -keysize 2048 -keypass welcome1
##
##
## $JAVA_HOME/bin/keytool -importkeystore -srckeystore keystore.jks -srcalias selfsigned -srcstorepass welcome1 \
##   -destkeystore keystore.p12 -deststoretype PKCS12 -deststorepass welcome1 -destkeypass welcome1
##
##
## ${OPENSSL} pkcs12 -in ${KEYSTORE}/keystore.p12 -nodes -nocerts -out ${KEYSTORE}/${HN}-key.pem -passin file:${PASSFILE}
## ${OPENSSL} pkcs12 -in ${KEYSTORE}/keystore.p12 -nokeys -out ${KEYSTORE}/${HN}.pem -passin file:${PASSFILE}
## ${OPENSSL} pkcs8 -topk8 -inform PEM -outform DER -in ${HN}-key.pem -out ${HN}-key.der -nocrypt
## ${OPENSSL} x509 -inform PEM -outform DER -in ${HN}.pem -out ${HN}.der








rm $PASSFILE
ls -ltr $KEYSTORE



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

setupHTTPS;

SetParameter;

$ORDS --config                           ${CONFIG} install                 \
      --admin-user                       ${SYSDBA_USER:-"SYS AS SYSDBA"}   \
      --db-hostname                      ${ORACLE_HOST:-racnode1}          \
      --db-port                          ${ORACLE_PORT:-1521}              \
      --db-servicename                   ${ORACLE_SERVICE:-TESTORDS}       \
      --feature-db-api                   true                              \
      --feature-rest-enabled-sql         true                              \
      --log-folder                       ${ORDS_LOGS}                      \
      --proxy-user                                                         \
      --password-stdin <<EOF
${SYSDBA_PASSWORD:-WElcome_12##}
${ORDS_PASSWORD:-WElcome_12##}
EOF

if [ $? -ne 0 ]
then
  echo "Installation error"
  exit 1
fi

}

NOT_INSTALLED=`$ORDS --config $CONFIG config list | grep "INFO: The" |wc -l `
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


