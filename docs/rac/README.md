# Using Oracle Real Application Cluster with Oracle Database Operator for Kubernetes

Oracle Real Application Clusters (Oracle RAC) is an option to the award-winning Oracle Database Enterprise Edition. Oracle RAC is a cluster database with a shared cache architecture that overcomes the limitations of traditional shared-nothing and shared-disk approaches to provide highly scalable and available database solutions for all business applications. Oracle RAC uses Oracle Clusterware as a portable cluster software that allows clustering of independent servers so that they cooperate as a single system and Oracle Automatic Storage Management (Oracle ASM) to provide simplified storage management that is consistent across all servers and storage platforms. Oracle Clusterware and Oracle ASM are part of the Oracle Grid Infrastructure, which bundles both solutions in an easy to deploy software package.


For more information on Oracle RAC Database 19c refer to the [Oracle Database Documentation](http://docs.oracle.com/en/database/).

Kubernetes provides infrastructure building blocks such as compute, storage and networks. Kubernetes makes the infrastructure available as code. It enables rapid provisioning of multi-node topolgies. Additionally, Kubernetes also provides statefulsets, which are the workload API objects that are used to manage stateful applications like Oracle Real Application Clusters (Oracle RAC), Single Instance Oracle Database, and other Oracle features and configurations.

The Oracle Real Application Cluster (RAC) Database Controller in Oracle Database Operator deploys Oracle RAC as a statefulset in the Kubernetes Clusters, using Oracle Real Application Cluster Slim Image. The Oracle RAC Database Controller manages the typical lifecycle of Oracle RAC Database in a Kubernetes cluster, as shown below:

* Create RAC database
  * Install and Configure Oracle Grid Infrastructure
  * Install and Configure Oracle RAC Database
* Create Persistent Storage, along with Statefulset
* Create Services
* Create Node Port Service
* Oracle RAC Database Scale Up/Scale Down
* Adding ASM Disk Devices to an existing Oracle RAC Database
* Deleting ASM Disk Devices from an existing Oracle RAC Database
* RAC Instances Cleanup

The Oracle RAC Database Controller provides end-to-end automation of Oracle RAC Database Deployment in a Kubernetes Cluster.

## Using Oracle RAC Database Controller

To create a Oracle database, complete the steps in the following sections:

* [Prerequisites for running Oracle RAC Database Controller](#prerequisites-for-using-oracle-rac-database-controller)  
* [Provisioning RAC database in a Oracle Cloud Native Enviornment (OCNE)](#provisioning-oracle-rac-database-using-kubernetes-cluster-in-an-oracle-cloud-native-environment-ocne)  
* [Connecting to Oracle RAC Database](#connecting-to-an-oracle-rac-database-provisioned-using-oracle-rac-database-controller)  
* [Known Issues](#known-issues)  
* [Debugging and Troubleshooting](#debugging-and-troubleshooting)  

**Note** Before proceeding to the next section, you must complete the instructions given in each section based on your enviornment.

## Prerequisites for using Oracle RAC Database Controller

**IMPORTANT :** You must make the changes specified in this section before you proceed to the next section.

To become familiar with Oracle Restart in containerized environments, review the [this documentation](https://github.com/oracle/docker-images/blob/main/OracleDatabase/RAC/OracleRealApplicationClusters/README.md) before proceeding further

[Prerequisites for using Oracle RAC Database Controller](./provisioning/prerequisites_oracle_rac_db.md)

## QuickStart

Oracle recommends that you start with the Quickstart to become familiar with Deployment of Oracle RAC Database using Oracle RAC Database Controller on Kubernetes. See: [QuickStart documentation](./provisioning/QUICKSTART.md).

After you become familiar with Deployment of Oracle RAC Database using Oracle RAC Database Controller on Kubernetes, you can explore more advanced setups, deployments, features, and so on, as explained in detail in the next section.

## Provisioning Oracle RAC Database using Kubernetes Cluster in an Oracle Cloud Native Environment (OCNE)

Deploy Oracle RAC Database YAML files using Kubernetes Cluster on your Oracle Cloud Native Environment (OCNE). There are multiple use case possible for deploying the Oracle RAC Database.

[1. Provisioning an Oracle RAC Database](./provisioning/provisioning_oracle_rac_db.md)  
[2. Provisioning an Oracle RAC Database with Security Contexts](./provisioning/provisioning_oracle_rac_db_contexts.md)  
[3. Provisioning an Oracle RAC Database with additional control on resources like CPU and Memory allocated to the Pods](./provisioning/provisioning_with_control_on_resources.md)   
[4. Provisioning an Oracle RAC Database with different ASM disk groups for CRS and RDBMS](./provisioning/provisioning_oracle_rac_database_using_diff_dg.md)  
[5. Provisioning an Oracle RAC Database with different ASM disk groups for CRS and RDBMS with different redundancy levels](./provisioning/provisioning_oracle_rac_database_different_redundancy.md)  
[6. Provisioning an Oracle RAC Database with Node Port Service](./provisioning/provisioning_oracle_rac_db_with_node_port.md)  
[7. Scale Out - Add Node to an existing Oracle RAC Database Cluster](./provisioning/scale_out_an_existing_rac_database.md)    
[8. Scale In - Delete a node from an existing Oracle RAC Database Cluster](./provisioning/scale_in_delete_an_existing_rac_database_instance.md)    
[9. Adding ASM Disks - Add ASM Disks to an existing Oracle RAC Database](./provisioning/add_asm_disk_to_an_existing_rac_database.md)  
[10. Deleting ASM Disks - Delete ASM Disks from an existing Oracle RAC Database](./provisioning/delete_asm_disks_from_an_existing_rac_database.md)  
[11. Provisioning an Oracle RAC Database with RU Patch](./provisioning/provisioning_oracle_rac_db_rupatch.md)  
[12. Provisioning an Oracle RAC Database with RU Patch and One Offs](./provisioning/provisioning_oracle_rac_db_rupatch_oneoff.md)  
[13. Provisioning an Oracle RAC Database using a Response File](./provisioning/provisioning_oracle_rac_db_using_response_file.md)  
[14. Provisioning an Oracle RAC Database with Huge Page allocation](./provisioning/provisioning_oracle_rac_db_hugepages.md)   

## Connecting to an Oracle RAC Database provisioned using Oracle RAC Database Controller

After the Oracle RAC Database has been provisioned using the Oracle RAC Database Controller in Oracle Database Operator, you can follow the steps in this document to connect to the Oracle RAC Database: [Database Connectivity](./provisioning/database_connection.md)

## Environment Variables Explained

Refer to [Environment Variables Details for Oracle RAC Database using RAC Controller](./provisioning/ENVVARIABLES.md) for the explanation of all the environment variables related to Oracle RAC Database Deployment using RAC Controller. Change or Set these environment variables as required for your environment.

## Known Issues

The known issues for the current version of the Oracle RAC Database Controller in Oracle Database Kubernetes Operator are documented in this page: [Known Issues](./provisioning/known_issues.md)

## Debugging and Troubleshooting

To debug the Oracle RAC database provisioned using the Oracle RAC Database Controller in Oracle Database Kubernetes Operator, follow this document: [Debugging and troubleshooting](./provisioning/debugging.md)
