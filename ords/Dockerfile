# LICENSE UPL 1.0
#
# Copyright (c) 1982-2017 Oracle and/or its affiliates. All rights reserved.
#
# ORACLE DOCKERFILES PROJECT
# --------------------------
# This is the Dockerfile for Oracle Rest Data Services
#
# REQUIRED FILES TO BUILD THIS IMAGE
# ----------------------------------
# (1) ords.3.0.10.165.06.53.zip
#     Download Oracle Rest Data Services from
#     http://www.oracle.com/technetwork/developer-tools/rest-data-services/downloads/index.html
#
# HOW TO BUILD THIS IMAGE
# -----------------------
# Put the downloaded file in the same directory as this Dockerfile
# Run: 
#      $ docker build -t oracle/restdataservices:3.0.10 . 
#
# Pull base image
# ---------------
FROM container-registry.oracle.com/java/jdk:latest

# Environment variables required for this build (do NOT change)
# -------------------------------------------------------------
ENV ORDS_HOME=/opt/oracle/ords \
    INSTALL_FILE=ords*.zip \
    CONFIG_PROPS="ords_params.properties.tmpl" \
    STANDALONE_PROPS="standalone.properties.tmpl" \
    CDBADMIN_PROPS="cdbadmin.properties.tmpl" \
    SETUP_WEBUSER="setupwebuser.sh" \
    RUN_FILE="runOrds.sh"

# Copy binaries
# -------------
COPY $INSTALL_FILE $CONFIG_PROPS $STANDALONE_PROPS $CDBADMIN_PROPS $RUN_FILE $SETUP_WEBUSER $ORDS_HOME/

RUN yum -y install bind-utils net-tools zip unzip tar wget vim-minimal which sudo expect procps && \
    yum clean all

# Setup filesystem and oracle user
# Adjust file permissions, go to /opt/oracle as user 'oracle' to proceed with ORDS installation
# ------------------------------------------------------------
RUN mkdir -p  $ORDS_HOME/doc_root && \
    chmod ug+x $ORDS_HOME/*.sh && \
    groupadd -g 54322 dba && \
    useradd -u 54321 -d /home/oracle -g dba -m -s /bin/bash oracle && \
    cd $ORDS_HOME && \
    jar -xf $INSTALL_FILE && \
    rm $INSTALL_FILE && \
    mkdir -p $ORDS_HOME/config/ords && \
    mkdir -p $ORDS_HOME/secrets && \
    java -jar $ORDS_HOME/ords.war configdir $ORDS_HOME/config && \
    chown -R oracle:dba $ORDS_HOME

# Finalize setup
# -------------------
USER oracle
WORKDIR /home/oracle

VOLUME ["$ORDS_HOME/config/ords"]
EXPOSE 8888

# Define default command to start Oracle Database.
CMD $ORDS_HOME/$RUN_FILE
