# Oracle RAC Controller QuickStart Guide

Use this quickstart to quickly deploy and manage a 2-node Oracle RAC Database on an OCNE Kubernetes cluster.

- [Oracle RAC Controller QuickStart Guide](#oracle-rac-controller-quickstart-guide)
  - [Before you begin](#before-you-begin)
  - [Deploy Oracle 2 Node RAC Database](#deploy-oracle-2-node-rac-database)
  - [Check the Details of Oracle RAC Database](#check-the-details-of-oracle-rac-database)
  - [Database Connection](./database_connection.md)
  - [Cleanup the environment](#clean-up-the-environment)
  - [Copyright](#copyright)

## Before you begin
Ensure you complete the prerequisites before proceeding: [Prerequisites for using Oracle RAC Database Controller](../README.md#prerequisites-for-using-oracle-rac-database-controller). 

These steps include::

- Oracle Grid Infrastructure and Oracle Database Software
- Network Setup
- Worker Node Preparation
- Storage requirements
- RBAC Rules for deployment of Oracle RAC Database
- Oracle RAC Slim Image
- Kuberentes Secrets

**Note:** The example in this QuickStart Guide deploys a two-node Oracle RAC Database with a NodePort Service.

## Deploy Oracle 2 Node RAC Database

Follow these steps to deploy a 2-node Oracle RAC Database using the Oracle RAC Controller on an OCNE Kubernetes Cluster:

- Copy the [racdb_prov_quickstart.yaml](./racdb_prov_quickstart.yaml) file from in your working directory. 
- Stage the Oracle Grid Infrastructure and Oracle RDBMS Binaries in the location specified by the parameter `hostSwStageLocation` on your worker nodes. 
- Ensure the parameter `racHostSwLocation` points to the software location on the worker nodes. The GI Home and RDBMS Home in the Oracle RAC Pods will be mounted using this lcoation on the corresponding worker nodes. 
- Use the following prebuilt Oracle RAC Database Slim Image: `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim` 
- Verify that the shared disks on the worker nodes for ASM are as follows: `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2` 
- Deploy the 2-node Oracle RAC Database using file `racdb_prov_quickstart.yaml` file:
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
After deployment, obtain Oracle RAC Database details with these commands:
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

- When you are logged inside the RAC Database Pod, switch to the "grid" user and run the following commands:
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
After provisioning the Oracle RAC Database with the Oracle RAC Database Controller in Oracle Database Operator, refer to the “Database Connectivity” documentation to connect to the Oracle RAC Database:
[Database Connectivity](./database_connection.md)

## Clean up the environment

If you want to clean up the Oracle RAC Database after deploying it, run the following command: 
```bash
kubectl apply -f racdb_prov_quickstart.yaml
```

This command will clean up the Oracle RAC Database deployed earlier with the Oracle RAC Controller.

**Note:** To reuse the same worker nodes and shared storage disks for a new deployment, you must clear the storage location pointed to by the parameter `racHostSwLocation` on the worker nodes and also clear the shared disks used for ASM.

## Copyright

Copyright (c) 2022 - 2025 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at https://oss.oracle.com/licenses/upl/