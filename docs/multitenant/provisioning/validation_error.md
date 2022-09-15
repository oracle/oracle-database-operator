#Validation and Errors

## Kubernetes Events
You can check Kubernetes events for any errors or status updates as shown below:
```sh
$ kubectl get events -A
NAMESPACE                         LAST SEEN   TYPE      REASON               OBJECT                                                             MESSAGE
oracle-database-operator-system   58m         Warning   Failed               pod/cdb-dev-ords-qiigr                                             Error: secret "cdb1-secret" not found
oracle-database-operator-system   56m         Normal    DeletedORDSPod       cdb/cdb-dev                                                        Deleted ORDS Pod(s) for cdb-dev
oracle-database-operator-system   56m         Normal    DeletedORDSService   cdb/cdb-dev                                                        Deleted ORDS Service for cdb-dev
...
oracle-database-operator-system   26m         Warning   OraError             pdb/pdb1                                                            ORA-65016: FILE_NAME_CONVERT must be specified...
oracle-database-operator-system   24m         Warning   OraError             pdb/pdb2                                                            ORA-65011: Pluggable database DEMOTEST does not exist.
...
oracle-database-operator-system   20m         Normal    Created              pdb/pdb1                                                            PDB 'demotest' created successfully
...
oracle-database-operator-system   17m         Warning   OraError             pdb/pdb3                                                            ORA-65012: Pluggable database DEMOTEST already exists...
```

In case of successfull operation, you can see messages like below:

```sh
% kubectl get events -A
NAMESPACE                         LAST SEEN   TYPE      REASON               OBJECT               MESSAGE
kube-system                       33s         Warning   BackOff              pod/kube-apiserver   Back-off restarting failed container
oracle-database-operator-system   59m         Normal    CreatedORDSService   cdb/cdb-dev          Created ORDS Service for cdb-dev
oracle-database-operator-system   51m         Normal    Created              pdb/pdb1-clone       PDB 'pdbnewclone' cloned successfully
oracle-database-operator-system   49m         Normal    Modified             pdb/pdb1-clone       PDB 'pdbnewclone' modified successfully
oracle-database-operator-system   47m         Normal    Deleted              pdb/pdb1-clone       PDB 'pdbnewclone' dropped successfully
oracle-database-operator-system   53m         Normal    Created              pdb/pdb1             PDB 'pdbnew' created successfully
oracle-database-operator-system   44m         Normal    Modified             pdb/pdb1             PDB 'pdbnew' modified successfully
oracle-database-operator-system   42m         Normal    Unplugged            pdb/pdb1             PDB 'pdbnew' unplugged successfully
oracle-database-operator-system   39m         Normal    Created              pdb/pdb1             PDB 'pdbnew' plugged successfully
```

## CDB Validation and Errors

Validation is done at the time of CDB resource creation as shown below:
```sh
$ kubectl apply -f cdb1.yaml
The PDB "cdb-dev" is invalid:
* spec.dbServer: Required value: Please specify Database Server Name or IP Address
* spec.dbPort: Required value: Please specify DB Server Port
* spec.ordsImage: Required value: Please specify name of ORDS Image to be used
```

Apart from events, listing of CDBs will also show the possible reasons why a particular CDB CR could not be created as shown below:
```sh
  $ kubectl get cdbs -A

  NAMESPACE                         NAME      CDB NAME   DB SERVER    DB PORT   SCAN NAME   STATUS   MESSAGE
  oracle-database-operator-system   cdb-dev   devdb      172.17.0.4   1521      devdb       Failed   Secret not found:cdb1-secret
```

## PDB Validation and Errors

Validation is done at the time of PDB resource creation as shown below:
```sh
$ kubectl apply -f pdb1.yaml
The PDB "pdb1" is invalid:
* spec.cdbResName: Required value: Please specify the name of the CDB Kubernetes resource to use for PDB operations
* spec.pdbName: Required value: Please specify name of the PDB to be created
* spec.adminPwd: Required value: Please specify PDB System Administrator Password
* spec.fileNameConversions: Required value: Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE
```

Similarly, for PDBs, listing of PDBs will also show the possible reasons why a particular PDB CR could not be created as shown below:
```sh
$ kubectl get pdbs -A
NAMESPACE                         NAME   CONNECT STRING   CDB NAME   PDB NAME   PDB SIZE   STATUS   MESSAGE
oracle-database-operator-system   pdb1                    democdb    demotest1             Failed   Secret not found:pdb12-secret
oracle-database-operator-system   pdb2                    democdb    demotest2             Failed   ORA-65016: FILE_NAME_CONVERT must be specified...
```
