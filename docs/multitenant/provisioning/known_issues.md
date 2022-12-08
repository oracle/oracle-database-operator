# Known Issues

Please refer to the below list of known issues related to Oracle DB Operator On-Prem Controller:

1. **ORA-20002: ERROR: The user ORDS_PUBLIC_USER already exists in the logs of CDB CRD Pods.**

This error is expected when you are deploying `2` replicas of CDB CRD during the deployment of the CDB CRD. Below is the snippet of a possible error:
```
2022-06-22T20:06:32.616Z INFO        Installing Oracle REST Data Services version 21.4.2.r0621806 in CDB$ROOT
2022-06-22T20:06:32.663Z INFO        ... Log file written to /home/oracle/ords_cdb_install_core_CDB_ROOT_2022-06-22_200632_00662.log
2022-06-22T20:06:33.835Z INFO        CDB restart file created in /home/oracle/ords_restart_2022-06-22_200633_00835.properties
2022-06-22T20:06:33.837Z SEVERE      Error executing script: ords_prereq_env.sql Error: ORA-20002: ERROR: The user ORDS_PUBLIC_USER already exists.  You must first uninstall ORDS using ords_uninstall.sql prior to running the install scripts.
ORA-06512: at line 8
ORA-06512: at line 8

 Refer to log file /home/oracle/ords_cdb_install_core_CDB_ROOT_2022-06-22_200632_00662.log for details

java.io.IOException: Error executing script: ords_prereq_env.sql Error: ORA-20002: ERROR: The user ORDS_PUBLIC_USER already exists.  You must first uninstall ORDS using ords_uninstall.sql prior to running the install scripts.
ORA-06512: at line 8
ORA-06512: at line 8
```
This error is seen in the logs of one of the two CDB CRD pods. The other Pod `does not` show this error and the ORDS installation is done successfully. 

To avoid this error, you need to initially deploy the CDB CRD with a single replica and later add another replica as per need.

2. **PDB create failure with error "Failed: Unauthorized"**

It was observed that PDB creation fails with the below error when special characters like "_" or "#" were used in the password for user SQL_ADMIN:
```
2022-06-22T20:10:09Z	INFO	controllers.PDB	ORDS Error - HTTP Status Code :401	{"callAPI": "oracle-database-operator-system/pdb1", "Err": "\n{\n    \"code\": \"Unauthorized\",\n    \"message\": \"Unauthorized\",\n    \"type\": \"tag:oracle.com,2020:error/Unauthorized\",\n    \"instance\": \"tag:oracle.com,2020:ecid/OoqA0Zw3oBWdabzP8wUMcQ\"\n}"}
2022-06-22T20:10:09Z	INFO	controllers.PDB	Reconcile completed	{"onpremdboperator": "oracle-database-operator-system/pdb1"}
2022-06-22T20:10:09Z	DEBUG	events	Warning	{"object": {"kind":"PDB","namespace":"oracle-database-operator-system","name":"pdb1","uid":"19fc98b1-ca7f-4e63-a6c7-fdeb14b8c275","apiVersion":"database.oracle.com/v1alpha1","resourceVersion":"99558229"}, "reason": "ORDSError", "message": "Failed: Unauthorized"}
```

In testing, we have used the password `welcome1` for the user SQL_ADMIN.

To avoid this error, please avoid password `welcome1` for SQL_ADMIN user.


3. **After cloning a PDB from another PDB, PDB SIZE field is show as empty even if the .yaml file used during the PDB cloning specifies the PDB size:**

```sh
% kubectl get pdbs -A
NAMESPACE                         NAME         CONNECT STRING                                                           CDB NAME   PDB NAME      PDB STATE    PDB SIZE   STATUS   MESSAGE
oracle-database-operator-system   pdb1         goldhost-scan.lbsub52b3b1cae.okecluster.oraclevcn.com:1521/pdbnew        goldcdb    pdbnew        READ WRITE   1G         Ready    Success
oracle-database-operator-system   pdb1-clone   goldhost-scan.lbsub52b3b1cae.okecluster.oraclevcn.com:1521/pdbnewclone   goldcdb    pdbnewclone   READ WRITE              Ready    Success
```

In the above example the PDB `pdbnewclone` is cloned from PDB `pdbnew` and is showing the size column as EMPTY. This will be fixed in future version.
