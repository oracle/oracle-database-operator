# Oracle RAC Controller QuickStart Guide

Use this quickstart to help you deploy 2 Node Oracle RAC Database using Oracle RAC Controller on a Kuberentes Cluster (OCNE).

- [Oracle RAC Controller QuickStart Guide](#oracle-rac-controller-quickstart-guide)
  - [Before you begin](#before-you-begin)
  - [Deploy Oracle 2 Node RAC Database](#deploy-oracle-2-node-rac-database)
  - [Check the Details of Oracle RAC Database](#check-the-details-of-oracle-rac-database)
  - [Database Connection](./database_connection.md)
  - [Cleanup the environment](#clean-up-the-environment)
  - [Copyright](#copyright)

## Before you begin
Before proceeding further, make sure you have completed the steps: [Prerequisites for using Oracle RAC Database Controller](../README.md#prerequisites-for-using-oracle-rac-database-controller). It includes the required steps like:

- Oracle Grid Infrastructure and Oracle Database Software
- Network Setup
- Worker Node Preparation
- Storage requirements
- RBAC Rules for deployment of Oracle RAC Database
- Oracle RAC Slim Image
- Kuberentes Secrets

**Note:** The example in this Quickstart Guide deploys two Node RAC Database with Node Port Service.

## Deploy Oracle 2 Node RAC Database

Follow the below steps to quickly deploy 2 Node Oracle RAC Database using RAC Controller on an OCNE Cluster:

- Copy the [racdb_prov_quickstart.yaml](./racdb_prov_quickstart.yaml) file from in your working directory. 
- Stage the Oracle Grid Infrastructure and Oracle RDBMS Binaries in the location specified by parameter `hostSwStageLocation` on your worker nodes. 
- The parameter `racHostSwLocation` must point to the software location on the worker nodes. The GI Home and RDBMS Home in the Oracle RAC Pods will be mounted using this lcoation on the corresponding worker nodes. 
- Prebuilt Oracle RAC Database Slim Image used is: `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim` 
- The Shared Disks on the worker nodes for ASM are `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2` 
- Deploy 2 Node Oracle RAC Database using file `racdb_prov_quickstart.yaml` file:
    ```sh
    kubectl apply -f racdb_prov_quickstart.yaml
    ```
- Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n rac
    
    # Check the logs of a particular pod. For example, to check status of pod "racnode1-0":    
    kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ===================================
    ORACLE RAC DATABASE IS READY TO USE
    ===================================
    ```

## Check the Details of Oracle RAC Database
You can get the details of the Oracle RAC Database after the deployment is completed using below commands:
```bash
- Check the status of the Oracle RAC Database Pods:
$ kubectl get all -n rac -o wide

- Get the details of the "racdatabases.database.oracle.com" object created:
$ kubectl get racdatabases.database.oracle.com -n rac

- Describe the "racdatabases.database.oracle.com" object for its details:
$ kubectl describe racdatabases.database.oracle.com/racdbprov-sample -n rac

- Switch to the RAC Database Nodes:
$ kubectl exec -it pod/racnode1-0 -n rac /bin/bash
$ kubectl exec -it pod/racnode2-0 -n rac /bin/bash

- Once inside the RAC Database Pod, switch to "grid" user and run the commands as "grid" user:
[grid@racnode1-0 ~]$ /u01/app/19c/grid/bin/crsctl stat res -t
[grid@racnode2-0 ~]$ /u01/app/19c/grid/bin/crsctl stat res -t

[grid@racnode1-0 ~]$ /u01/app/19c/grid/bin/srvctl config nodeapps
[grid@racnode2-0 ~]$ /u01/app/19c/grid/bin/srvctl config nodeapps

- Once inside the RAC Database Pod, switch to "oracle" user and run the commands as "oracle" user:
[oracle@racnode1-0 ~]$ srvctl status database -d PORCLCDB -v
[oracle@racnode2-0 ~]$ srvctl status database -d PORCLCDB -v

[oracle@racnode1-0 ~]$ srvctl config service -s soepdb -d PORCLCDB
[oracle@racnode2-0 ~]$ srvctl config service -s soepdb -d PORCLCDB
```

## Database Connection
After the Oracle RAC Database has been provisioned using the Oracle RAC Database Controller in Oracle Database Operator, you can follow the steps in this document to connect to the Oracle RAC Database: [Database Connectivity](./database_connection.md)

## Clean up the environment

If you want to clean up the Oracle RAC Database deployed above, then run the following command-
```bash
kubectl apply -f racdb_prov_quickstart.yaml
```

This command will clean up the Oracle RAC Database deployed earlier with the Oracle RAC Controller.

**Note:** In order to reuse the same worker nodes and shared storage disks for a new deployment, you will need to clear the storage location pointed by the parameter `racHostSwLocation` on the worker nodes and also clear the shared disks used for ASM.

## Copyright

Copyright (c) 2022 - 2024 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at https://oss.oracle.com/licenses/upl/