#!/bin/bash
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
# 
# Since: June, 2017
# Author: gerald.venzl@oracle.com
# Description: Setup and runs Oracle Rest Data Services.
# 
# DO NOT ALTER OR REMOVE COPYRIGHT NOTICES OR THIS HEADER.
# 
# Copyright (c) 2014-2017 Oracle and/or its affiliates. All rights reserved.
#
#    MODIFIED    (DD-Mon-YY)
#       gdbhat    16-Aug-21 - Added CDB Admin properties. Commented out ORACLE_PWD check

function setupOrds() {

  # Check whether the Oracle DB password has been specified
  #if [ "$ORACLE_PWD" == "" ]; then
  #  echo "Error: No ORACLE_PWD specified!"
  #  echo "Please specify Oracle DB password using the ORACLE_PWD environment variable."
  #  exit 1;
  #fi;

  # Defaults
  ORACLE_SERVICE=${ORACLE_SERVICE:-"ORCLPDB1"}
  ORACLE_HOST=${ORACLE_HOST:-"localhost"}
  ORACLE_PORT=${ORACLE_PORT:-"1521"}
  APEXI=${APEXI:-"$ORDS_HOME/doc_root/i"}
  ORDS_PORT=${ORDS_PORT:-"8888"}

  ORDS_PWD=`cat $ORDS_HOME/secrets/$ORDS_PWD_KEY`
  ORACLE_PWD=`cat $ORDS_HOME/secrets/$ORACLE_PWD_KEY`
  CDBADMIN_USER=`cat $ORDS_HOME/secrets/$CDBADMIN_USER_KEY`
  CDBADMIN_PWD=`cat $ORDS_HOME/secrets/$CDBADMIN_PWD_KEY`

  # Make standalone dir
  mkdir -p $ORDS_HOME/config/ords/standalone
  
  # Copy template files
  cp $ORDS_HOME/$CONFIG_PROPS $ORDS_HOME/params/ords_params.properties
  cp $ORDS_HOME/$STANDALONE_PROPS $ORDS_HOME/config/ords/standalone/standalone.properties
  cp $ORDS_HOME/$CDBADMIN_PROPS $ORDS_HOME/cdbadmin.properties

  # Replace DB related variables (ords_params.properties)
  sed -i -e "s|###ORACLE_SERVICE###|$ORACLE_SERVICE|g" $ORDS_HOME/params/ords_params.properties
  sed -i -e "s|###ORACLE_HOST###|$ORACLE_HOST|g" $ORDS_HOME/params/ords_params.properties
  sed -i -e "s|###ORACLE_PORT###|$ORACLE_PORT|g" $ORDS_HOME/params/ords_params.properties
  sed -i -e "s|###ORDS_PWD###|$ORDS_PWD|g" $ORDS_HOME/params/ords_params.properties
  sed -i -e "s|###ORACLE_PWD###|$ORACLE_PWD|g" $ORDS_HOME/params/ords_params.properties
  
  # Replace standalone runtime variables (standalone.properties)
  sed -i -e "s|###PORT###|$ORDS_PORT|g" $ORDS_HOME/config/ords/standalone/standalone.properties
  sed -i -e "s|###DOC_ROOT###|$ORDS_HOME/doc_root|g" $ORDS_HOME/config/ords/standalone/standalone.properties
  sed -i -e "s|###APEXI###|$APEXI|g" $ORDS_HOME/config/ords/standalone/standalone.properties
   
  # Replace CDB Admin runtime variables (cdbadmin.properties)
  sed -i -e "s|###CDBADMIN_USER###|$CDBADMIN_USER|g" $ORDS_HOME/cdbadmin.properties
  sed -i -e "s|###CDBADMIN_PWD###|$CDBADMIN_PWD|g" $ORDS_HOME/cdbadmin.properties

  # Start ORDS setup
  java -jar $ORDS_HOME/ords.war install simple

  # Setup Web Server User
  $ORDS_HOME/$SETUP_WEBUSER
}

############# MAIN ################

# Check whether ords is already setup
if [ ! -f $ORDS_HOME/config/ords/standalone/standalone.properties ]; then
    setupOrds;
    java -jar $ORDS_HOME/ords.war set-properties --conf apex_pu $ORDS_HOME/cdbadmin.properties
fi;

java -jar $ORDS_HOME/ords.war standalone
