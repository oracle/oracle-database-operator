#LICENSE UPL 1.0
#
# Copyright (c) 1982-2017 Oracle and/or its affiliates. All rights reserved.
#
# ORACLE DOCKERFILES PROJECT
# --------------------------
# This is the Dockerfile for Oracle Rest Data Services 22.2
#
FROM  container-registry.oracle.com/java/jdk:latest

# Environment variables required for this build (do NOT change)
# -------------------------------------------------------------
ENV ORDS_HOME=/opt/oracle/ords/ \
    RUN_FILE="runOrdsSSL.sh" 

#RUN_FILE_NOSSL="runOrdsNOSSL.sh"

# Copy binaries
# -------------
COPY   $RUN_FILE $ORDS_HOME 
#COPY   $RUN_FILE_NOSSL $ORDS_HOME

RUN yum -y install yum-utils bind-utils tree hostname openssl net-tools zip unzip tar wget vim-minimal which sudo expect procps && \
    yum-config-manager --add-repo=http://yum.oracle.com/repo/OracleLinux/OL8/oracle/software/x86_64 && \
    yum -y install java-11-openjdk-devel && \
    yum -y install ords && \
    yum -y install iproute && \
    yum clean all

# Setup filesystem and oracle user
# ------------------------------------------------------------
RUN mkdir -p  $ORDS_HOME/doc_root                                  && \
    mkdir -p  $ORDS_HOME/error                                     && \
    mkdir -p  $ORDS_HOME/secrets                                   && \
    chmod ug+x $ORDS_HOME/*.sh                                     && \
    groupadd -g 54322 dba                                          && \
    usermod -u 54321 -d /home/oracle -g dba -m -s /bin/bash oracle && \
    chown -R oracle:dba $ORDS_HOME                                 && \
    echo "oracle  ALL=(ALL)       NOPASSWD: ALL" >> /etc/sudoers

# Finalize setup
# -------------------
USER oracle
WORKDIR /home/oracle

VOLUME ["$ORDS_HOME/config/ords"]
EXPOSE 8888

# Define default command to start Ords Services
CMD $ORDS_HOME/$RUN_FILE

## ONLY FOR DEVELOPMENT STAGE
#CMD ["/usr/sbin/init"]
