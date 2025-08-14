# Using Oracle Restart with Oracle Database Operator for Kubernetes

Oracle Restart is an option to the award-winning Oracle Database Enterprise Edition. Oracle Restart is a feature introduced in Oracle 11gR2 that automatically restarts Oracle components, such as the database instance, listener, and Oracle ASM, after a failure or system reboot. It ensures that these components are started in the correct order and that they are managed by Oracle's High Availability Services (HAS). This enhances the availability of Oracle databases in a standalone server environment. Refer [this documentation](https://docs.oracle.com/cd/E18283_01/server.112/e17120/restart001.htm)

For more information on Oracle Restart Database 19c refer to the [Oracle Database Documentation](http://docs.oracle.com/en/database/).

Kubernetes provides infrastructure building blocks such as compute, storage and networks. Kubernetes makes the infrastructure available as code. It enables rapid provisioning of multi-node topolgies. Additionally, Kubernetes also provides statefulsets, which are the workload API objects that are used to manage stateful applications like Oracle Restarts, Single Instance Oracle Database, and other Oracle features and configurations.

The Oracle Restart Database Controller in Oracle Database Operator deploys Oracle Restart as a statefulset in the Kubernetes Clusters, using Oracle Restart Slim Image. The Oracle Restart Database Controller manages the typical lifecycle of Oracle Restart Database in a Kubernetes cluster, as shown below:

* Create Oracle Restart database
  * Install and Configure Oracle Grid Infrastructure
  * Install and Configure Oracle Restart Database
* Create Persistent Storage, along with Statefulset
* Create Services
* Oracle Restart Instances Cleanup

The Oracle Restart Database Controller provides end-to-end automation of Oracle Restart Database Deployment in a Kubernetes Cluster.

## Using Oracle Restart Database Controller

To create a Oracle database, complete the steps in the following sections:

1. [Prerequisites for running Oracle Restart Database Controller](#prerequisites-for-running-oracle-restart-database-controller)  
2. [Provisioning Oracle Restart database in a Oracle Kubernetes Engine Environment](#provisioning-oracle-restart-database-in-a-oracle-kubernetes-engine-environment)
3. [Connecting to Oracle Restart Database](#connecting-to-oracle-restart-database)
4. [Known Issues](#known-issues)
5. [Debugging and Troubleshooting](#debugging-and-troubleshooting)

**Note** Before proceeding to the next section, you must complete the instructions given in each section based on your enviornment.

### Prerequisites for running Oracle Restart Database Controller

**IMPORTANT :** You must make the changes specified in this section before you proceed to the next section.

In order to become familiar with Oracle Restart on containers, you can refer [this documentation](https://github.com/oracle/docker-images/blob/main/OracleDatabase/RAC/OracleRealApplicationClusters/docs/orestart/README.md) before proceeding further.

[Pre-requisites for running Oracle Restart Database Controller](./provisioning/prerequisites_oracle_restart_db.md)

## Provisioning Oracle Restart database in a Oracle Kubernetes Engine Environment

Deploy Oracle Restart Database YAML files using Kubernetes Cluster on your Oracle Kubernetes Engine Environment (OKE). There are multiple use case possible for deploying the Oracle Restart Database.

[1. Provisioning an Oracle Restart Database](./provisioning/provisioning_oracle_restart_db.md)  
[2. Provisioning an Oracle Restart Database with NodePort Service](./provisioning/provisioning_oracle_restart_db_nodeport.md)  
[3. Provisioning an Oracle Restart Database with OnsPort Service](./provisioning/provisioning_oracle_restart_db_onsport.md)  
[4. Provisioning an Oracle Restart Database with RU Patch on FileSystem](./provisioning/provisioning_oracle_restart_db_rupatch.md)  
[5. Provisioning an Oracle Restart Database with Custom Storage Class](./provisioning/provisioning_oracle_restart_storage_class.md)  
[6. Provisioning an Oracle Restart Database with RU Patch on Existing PVC](./provisioning/provisioning_oracle_restart_rupatch_pvc.md)  
[7. Adding ASM Disks - Add ASM Disks to an existing Oracle Restart Database](./provisioning/add_asm_disk_to_an_existing_restart_database.md)  
[8. Deleting ASM Disks - Delete ASM Disks from an existing Oracle Restart Database](./provisioning/delete_asm_disks_from_an_existing_restart_database.md)

## Connecting to Oracle Restart Database

After the Oracle Restart Database has been provisioned using the Oracle Restart Database Controller in Oracle Database Operator, you can follow the steps in this document to connect to the Oracle Restart Database: [Database Connectivity](./provisioning/database_connection.md)

## Cleanup

Steps to cleanup Oracle Restart Database Controller deployed using above document in Oracle Database Kubernetes Operator are documented in this page: [Cleanup](./provisioning/cleanup.md)


## Debugging and Troubleshooting

To debug the Oracle Restart database provisioned using the Oracle Restart Database Controller in Oracle Database Kubernetes Operator, follow this document: [Debugging and troubleshooting](./provisioning/debugging.md)