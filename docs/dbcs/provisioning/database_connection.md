## Database connection 

In order to retrieve the database connection use the kubectl describe command 

```sh
kubectl describe dbcssystems.database.oracle.com dbcssystem-create 
```

You can use the following script (tnsalias.awk) to get a simple tnsalias 

```awk
!#/usr/bin/awk
( $0 ~ / Db Unique Name:/ ) { DB_UNIQUE_NAME=$4 }
( $0 ~ /Domain Name:/ ) { DB_DOMAIN=$3 }
( $0 ~ /Host Name:/ ) { HOSTNAME=$3 }
( $0 ~ /Listener Port:/ ) { PORT=$3 }

END {
  printf ("db_unique_name=%s\n",DB_UNIQUE_NAME);
  printf ("db_domain=%s\n",DB_DOMAIN);
  printf ("hostname=%s\n",HOSTNAME);
  printf ("port=%s\n",PORT);
  printf ("====== TNSALIAS ======\n");
  printf ("(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=%s)(PORT=%s))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=%s.%s)))\n",
         HOSTNAME,PORT,DB_UNIQUE_NAME,DB_DOMAIN);
```

```text
kubectl describe dbcssystems.database.oracle.com dbcssystem-create |awk -f tnsalias.awk
db_unique_name=testdb_fg4_lin
db_domain=vcndns.oraclevcn.com
hostname=host1205
port=1521
====== TNSALIAS ======
(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host1205)(PORT=1521))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=testdb_fg4_lin.vcndns.oraclevcn.com)))

sqlplus scott@"(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host1205)(PORT=1521))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=testdb_fg4_lin.vcndns.oraclevcn.com)))"


SQL*Plus: Release 19.0.0.0.0 - Production on Fri Dec 15 14:16:42 2023
Version 19.15.0.0.0

Copyright (c) 1982, 2022, Oracle.  All rights reserved.

Enter password: 
Last Successful login time: Fri Dec 15 2023 14:14:07 +00:00

Connected to:
Oracle Database 19c EE High Perf Release 19.0.0.0.0 - Production
Version 19.18.0.0.0

SQL> 
```
