## Copyright (c) 2022 Oracle and/or its affiliates.
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

FROM  container-registry.oracle.com/java/jdk:latest

# Environment variables required for this build (do NOT change)
# -------------------------------------------------------------
ENV ORDS_HOME=/opt/oracle/ords/ \
    RUN_FILE="runOrdsSSL.sh" \
    ORDSVERSION=23.4.0-8

# Copy binaries
# -------------
COPY   $RUN_FILE $ORDS_HOME 

RUN yum -y install yum-utils bind-utils tree hostname openssl net-tools zip unzip tar wget vim-minimal which sudo expect procps curl lsof && \
    yum-config-manager --add-repo=http://yum.oracle.com/repo/OracleLinux/OL8/oracle/software/x86_64 && \
    yum -y install java-11-openjdk-devel && \
    yum -y install iproute && \
    yum clean all

RUN curl -o /tmp/ords-$ORDSVERSION.el8.noarch.rpm https://yum.oracle.com/repo/OracleLinux/OL8/oracle/software/x86_64/getPackage/ords-$ORDSVERSION.el8.noarch.rpm

RUN rpm -ivh /tmp/ords-$ORDSVERSION.el8.noarch.rpm

# Setup filesystem and oracle user
# --------------------------------
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

