# Using Oracle Restart with Oracle Database Operator for Kubernetes

Oracle Restart is an option to the award-winning Oracle Database Enterprise Edition. Oracle Restart is a feature introduced in Oracle 11gR2 that automatically restarts Oracle components, such as the database instance, listener, and Oracle ASM, after a failure or system reboot. It ensures that these components are started in the correct order and that they are managed by Oracle's High Availability Services (HAS). This enhances the availability of Oracle databases in a standalone server environment. Refer [this documentation](https://docs.oracle.com/cd/E18283_01/server.112/e17120/restart001.htm)

For more information on Oracle Restart, refer to the [Oracle Database Documentation](http://docs.oracle.com/en/database/).

Kubernetes provides essential infrastructure building blocks, including compute, storage, and networking resources, and exposes them as code for infrastructure automation. This approach enables rapid provisioning of multi-node topologies. Additionally, Kubernetes offers the **StatefulSet** workload API object—ideal for managing stateful applications such as Oracle Restart, Single Instance Oracle Databases, and other Oracle features or configurations that require persistent storage and stable network identities.

The Oracle Restart Controller in the Oracle Database Operator deploys Oracle databases as a StatefulSet within Kubernetes clusters, using the Oracle Restart Slim Image. The Oracle Restart Controller manages the typical lifecycle operations of an Oracle database in a Kubernetes environment, including deployment, monitoring, scaling, upgrades, and deletion, as illustrated below:

* Create Oracle Database
  * Install and Configure Oracle Grid Infrastructure
  * Install and Configure Oracle Database
* Create Persistent Storage, along with Statefulset
* Create Services
* Oracle Restart Instances Cleanup

## Using Oracle Restart Controller

To create an Oracle database, complete the steps in the following sections:

1. [Prerequisites for running Oracle Restart Controller](#prerequisites-for-running-oracle-restart-controller)  
2. [Provisioning Oracle Restart database in a Oracle Kubernetes Engine Environment](#provisioning-oracle-restart-database-in-an-oracle-kubernetes-engine-environment)
3. [Connecting to Oracle Restart Database](#connecting-to-oracle-restart-database)
4. [Known Issues](#known-issues)
5. [Cleanup](#cleanup)
6. [Debugging and Troubleshooting](#debugging-and-troubleshooting)

**Note** Before proceeding to the next section, you must complete the instructions given in each section based on your enviornment.

### Prerequisites for running Oracle Restart Controller

**IMPORTANT :** You must make the changes specified in this section before you proceed to the next section.

To become familiar with Oracle Restart in containerized environments, review the [this documentation](https://github.com/oracle/docker-images/blob/main/OracleDatabase/RAC/OracleRealApplicationClusters/docs/orestart/README.md) before proceeding further

[Prerequisites for running Oracle Restart Controller](./provisioning/prerequisites_oracle_restart_db.md)

## Provisioning Oracle Restart database in an Oracle Kubernetes Engine Environment

Deploy Oracle Restart Database YAML files using Kubernetes Cluster on your Oracle Kubernetes Engine Environment (OKE). There are multiple use case possible for deploying the Oracle Restart Database.

[1. Provisioning an Oracle Restart Database](./provisioning/provisioning_oracle_restart_db.md)  
[2. Provisioning an Oracle Restart Database with Security Contexts](./provisioning/provisioning_oracle_restart_db_securitycontexts.md)  
[3. Provisioning an Oracle Restart Database with NodePort Service](./provisioning/provisioning_oracle_restart_db_nodeport.md)  
[4. Provisioning an Oracle Restart Database with Load Balancer](./provisioning/provisioning_oracle_restart_db_loadbalancer.md)  
[5. Change Memory and CPU allocation for an earlier provisioned Oracle Restart Database](./provisioning/change_memory_cpu_for_oracle_restart_db.md)  
[6. Change the size of Software Storage Location for an existing Oracle Restart Database](./provisioning/change_sw_storage_size_for_oracle_restart_db.md)  
[7. Provisioning an Oracle Restart Database with Custom Storage Class](./provisioning/provisioning_oracle_restart_storage_class.md)  
[8. Provisioning an Oracle Restart Database with RU Patch on FileSystem](./provisioning/provisioning_oracle_restart_db_rupatch.md)  
[9. Provisioning an Oracle Restart Database with RU Patch on Existing PVC](./provisioning/provisioning_oracle_restart_rupatch_pvc.md)  
[10. Provisioning an Oracle Restart Database with RU Patch and One Offs with Custom Storage Class](./provisioning/provisioning_oracle_restart_db_rupatch_oneoffs.md)  
[11. Provisioning an Oracle Restart Database with multiple diskgroups](./provisioning/provisioning_oracle_restart_multiple_diskgroups.md)  
[12. Provisioning an Oracle Restart Database with multiple diskgroups with different redundancy](./provisioning/provisioning_oracle_restart_multiple_diskgroups_with_redundancy.md)   
[13. Provisioning an Oracle Restart Database with multiple diskgroups with different redundancy and option to specify separate storage class](./provisioning/oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.md)  
[14. Adding ASM Disks - Add ASM Disks to an existing Oracle Restart Database](./provisioning/add_asm_disk_to_an_existing_restart_database.md)  
[15. Deleting ASM Disks - Delete ASM Disks from an existing Oracle Restart Database](./provisioning/delete_asm_disks_from_an_existing_restart_database.md)  
[16. Provisioning an Oracle Restart Database with Huge Page allocation ](./provisioning/provisioning_oracle_restart_hugepages.md)


**NOTE:** Resizing of the `ASM Disks` is _not_ allowed. You can add new ASM Disks to an exising Oracle Restart Database.  

## Connecting to Oracle Restart Database

After the Oracle Restart database has been provisioned using the Oracle Restart Controller in Oracle Database Operator, you can follow the steps in this document to connect to the Oracle Restart Database: [Database Connectivity](./provisioning/database_connection.md)

## Known Issues

Refer to the Known Issues document for assistance related to issues deploying Oracle Restart Database using Oracle Restart Controller: [Known Issues](./provisioning/known_issues.md)

## Cleanup

Steps to clean up Oracle Restart Database deployed using Oracle Restart Controller in this document in Oracle Database Kubernetes Operator are documented in this page: [Cleanup](./provisioning/cleanup.md)


## Debugging and Troubleshooting

To debug the Oracle Restart Database provisioned using the Oracle Restart Controller in Oracle Database Kubernetes Operator, follow this document: [Debugging and troubleshooting](./provisioning/debugging.md)
