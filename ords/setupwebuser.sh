#!/bin/bash
# Copyright (c) 2022, Oracle and/or its affiliates. All rights reserved.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.

WEBSERVER_USER=`cat $ORDS_HOME/secrets/$WEBSERVER_USER_KEY`
WEBSERVER_PASSWORD=`cat $ORDS_HOME/secrets/$WEBSERVER_PASSWORD_KEY`

export WEBSERVER_USER
export WEBSERVER_PASSWORD

/usr/bin/expect -c '

spawn java -jar $env(ORDS_HOME)/ords.war user $env(WEBSERVER_USER) "SQL Administrator"
expect "Enter a password for user"
send "$env(WEBSERVER_PASSWORD)\n"
expect "Confirm password for user"
send "$env(WEBSERVER_PASSWORD)\n"
expect "Created user"

'