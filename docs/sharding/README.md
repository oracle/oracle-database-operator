# Using Oracle Globally Distributed Database with Oracle Database Operator for Kubernetes

Oracle Globally Distributed Database distributes segments of a data set across many databases (shards) on different computers, either on-premises or in cloud. This feature enables globally distributed, linearly scalable, multimodel databases. It requires no specialized hardware or software. Oracle Globally Distributed Database does all this while rendering the strong consistency, full power of SQL, support for structured and unstructured data, and the Oracle Database ecosystem. It meets data sovereignty requirements, and supports applications that require low latency and high availability.

All of the shards together make up a single logical database, which is referred to as a Oracle Globally Distributed Database (GDD).

Kubernetes provides infrastructure building blocks, such as compute, storage, and networks. Kubernetes makes the infrastructure available as code. It enables rapid provisioning of multi-node topolgies. Additionally, Kubernetes also provides statefulsets, which are the workload API objects that are used to manage stateful applications. This provides us lifecycle management elasticity for databases as a stateful application for various database topologies, such as Oracle Globally Distributed Database, Oracle Real Application Clusters (Oracle RAC), single instance Oracle Database, and other Oracle features and configurations.

The Sharding Database controller in Oracle Database Operator deploys Oracle Globally Distributed Database Topology as a statefulset in the Kubernetes clusters, using Oracle Database and Global Data Services Docker images. The Oracle Sharding database controller manages the typical lifecycle of Oracle Globally Distributed Database topology in the Kubernetes cluster, as shown below:

* Create primary statefulsets shards
* Create master and standby Global Data Services statefulsets
* Create persistent storage, along with statefulset
* Create services
* Create load balancer service
* Provision Oracle Globally Distributed Database topology by creating and configuring the following:
  * Catalog database
  * Shard Databases
  * GSMs
  * Shard scale up and scale down
* Shard topology cleanup

The Oracle Sharding database controller provides end-to-end automation of Oracle Globally Distributed Database topology deployment in Kubernetes clusters.

## Using Oracle Database Operator Sharding Controller

Following sections provide the details for deploying Oracle Globally Distributed Database using Oracle Database Operator Sharding Controller with different use cases:

* [Prerequisites for running Oracle Sharding Database Controller](#prerequisites-for-running-oracle-sharding-database-controller)
* [Oracle Database 23ai Free](#oracle-database-23ai-free)
* [Provisioning Oracle Globally Distributed Database Topology System-Managed Sharding in a Cloud-Based Kubernetes Cluster](#provisioning-oracle-globally-distributed-database-topology-with-system-managed-sharding-in-a-cloud-based-kubernetes-cluster)
* [Provisioning Oracle Globally Distributed Database Topology User Defined Sharding in a Cloud-Based Kubernetes Cluster](#provisioning-oracle-globally-distributed-database-topology-with-user-defined-sharding-in-a-cloud-based-kubernetes-cluster)
* [Provisioning Oracle Globally Distributed Database System-Managed Sharding with Raft replication enabled in a Cloud-Based Kubernetes Cluster](#provisioning-oracle-globally-distributed-database-topology-with-system-managed-sharding-and-raft-replication-enabled-in-a-cloud-based-kubernetes-cluster)
* [Connecting to Shard Databases](#connecting-to-shard-databases)
* [Debugging and Troubleshooting](#debugging-and-troubleshooting)
* [Known Issues](#known-issues)

**Note** Before proceeding to the next section, you must complete the instructions given in each section, based on your enviornment, before proceeding to next section.

## Prerequisites for running Oracle Sharding Database Controller

**IMPORTANT:** You must make the changes specified in this section before you proceed to the next section.

### 1. Kubernetes Cluster: To deploy Oracle Sharding database controller with Oracle Database Operator, you need a Kubernetes Cluster which can be one of the following: 

* A Cloud-based Kubernetes cluster, such as [OCI on Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) or  
* An On-Premises Kubernetes Cluster, such as [Oracle Linux Cloud Native Environment (OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) cluster.

To use Oracle Sharding Database Controller, ensure that your system is provisioned with a supported Kubernetes release. Refer to the [Release Status Section](../../README.md#release-status).

#### Mandatory roles and privileges requirements for Oracle Sharding Database Controller 

  Oracle Sharding Database Controller uses Kubernetes objects such as :-

  | Resources | Verbs |
  | --- | --- |
  | Pods | create delete get list patch update watch | 
  | Containers | create delete get list patch update watch |
  | PersistentVolumeClaims | create delete get list patch update watch | 
  | Services | create delete get list patch update watch | 
  | Secrets | create delete get list patch update watch | 
  | Events | create patch |

### 2. Deploy Oracle Database Operator

To deploy Oracle Database Operator in a Kubernetes cluster, go to the section [Install Oracle DB Operator](../../README.md#install-oracle-db-operator) in the README, and complete the operator deployment before you proceed further. If you have already deployed the operator, then proceed to the next section.

**IMPORTANT:** Make sure you have completed the steps for [Role Binding for access management](../../README.md#role-binding-for-access-management) as well before installing the Oracle DB Operator. 

### 3. Oracle Database and Global Data Services Docker Images
Choose one of the following deployment options: 

  **Use Oracle-Supplied Docker Images:**
   The Oracle Sharding Database controller uses Oracle Global Data Services and Oracle Database images to provision the sharding topology.

   You can also download the pre-built Oracle Global Data Services `container-registry.oracle.com/database/gsm:latest` and Oracle Database images `container-registry.oracle.com/database/enterprise:latest` from [Oracle Container Registry](https://container-registry.oracle.com/ords/f?p=113:10::::::). These images are functionally tested and evaluated with various use cases of Oracle Globally Distributed Database topology by deploying on OKE and OLCNE.
  
   **Note:** You will need to accept Agreement from container-registry.orcale.com to be able to pull the pre-built container images.

   **OR**

  **Build your own Oracle Database and Global Data Services Docker Images:**
   You can build these images using instructions provided on Oracle official GitHub Repositories:
   * [Oracle Global Data Services Image](https://github.com/oracle/db-sharding/tree/master/docker-based-sharding-deployment/dockerfiles)
   * [Oracle Database Image](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance)

After the images are ready, push them to your Docker Images Repository, so that you can pull them during Oracle Globally Distributed Database topology provisioning.

You can either download the images and push them to your Docker Images Repository, or, if your Kubernetes cluster can reach OCR, you can download these images directly from OCR.

**Note**: In the Oracle Globally Distributed Database Topology example yaml files, we are using GDS and database images available on [Oracle Container Registry](https://container-registry.oracle.com/ords/f?p=113:10::::::).

**Note:** In case you want to use the `Oracle Database 23ai Free` Image for Database and GSM, refer to section [Oracle Database 23ai Free](#oracle-database-23ai-free) for more details.

### 4. Create a namespace for the Oracle Globally Distributed Database Setup

  Create a Kubernetes namespace named `shns`. All the resources belonging to the Oracle Globally Distributed Database Topology Setup will be provisioned in this namespace named `shns`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns shns

  #### Check the created namespace 
  kubectl get ns
  ```

### 5. Create a Kubernetes secret for the database installation owner for the Oracle Globally Distributed Database Topology Deployment

Create a Kubernetes secret named `db-user-pass-rsa` using these steps: [Create Kubernetes Secret](./provisioning/create_kubernetes_secret_for_db_user.md)

After you have the above prerequisites completed, you can proceed to the next section for your environment to provision the Oracle Globally Distributed Database Topology.

### 6. Provisioning a Persistent Volume having an Oracle Database Gold Image

This step is needed when you want to provision a Persistent Volume having an Oracle Database Gold Image for Database Cloning. 

In case of an `OCI OKE` cluster, you can use this Persistent Volume during provisioning Shard Databases by cloning in the same Availability Domain or you can use a Full Backup of this Persistent Volume during provisioning Shard Databases by cloning in different Availability Domains.

You can refer [here](./provisioning/provisioning_persistent_volume_having_db_gold_image.md) for the steps involved.

**NOTE:** Provisioning the Oracle Globally Distributed Database using Cloning from Database Gold Image is `NOT` supported with Oracle Database 23ai Free. So, this step will not be needed if you are deploying Oracle Globally Distributed Database using Oracle 23ai Free Database and GSM Images.

## Oracle Database 23ai Free

Please refer to [Oracle Database 23ai Free](https://www.oracle.com/database/free/get-started/) documentation for more details. 

If you want to use Oracle Database 23ai Free Image for Database and GSM for deployment of the Oracle Globally Distributed Database using Sharding Controller in Oracle Database Kubernetes Operator, you need to consider the below points:

* To deploy using the FREE Database and GSM Image, you will need to add the additional parameter `dbEdition: "free"` to the .yaml file.
* Refer to [Sample Oracle Globally Distributed Database Deployment using Oracle 23ai FREE Database and GSM Images](./provisioning/free/sharding_provisioning_with_free_images.md) for an example.
* For Oracle Database 23ai Free, you can control the `CPU` and `Memory` allocation of the PODs using tags `cpu` and `memory` respectively but tags `INIT_SGA_SIZE` and `INIT_PGA_SIZE` to control the SGA and PGA allocation at the database level are `not` supported.
* Provisioning the Oracle Globally Distributed Database using Cloning from Database Gold Image is `NOT` supported with Oracle Database 23ai Free.
* Total number of chunks for FREE Database defaults to `12` if `CATALOG_CHUNKS` parameter is not specified. This default value is determined considering limitation of 12 GB of user data on disk for oracle free database.


## Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding in a Cloud-Based Kubernetes Cluster

Deploy Oracle Globally Distributed Database Topology with `System-Managed Sharding` on your Cloud based Kubernetes cluster. 

In this example, the deployment uses the YAML file based on `OCI OKE` cluster. There are multiple use case possible for deploying the Oracle Globally Distributed Database Topology covered by below examples:

[1. Provisioning Oracle Globally Distributed Database with System-Managed Sharding without Database Gold Image](./provisioning/system_sharding/ssharding_provisioning_without_db_gold_image.md)  
[2. Provisioning Oracle Globally Distributed Database with System-Managed Sharding with number of chunks specified](./provisioning/system_sharding/ssharding_provisioning_with_chunks_specified.md)  
[3. Provisioning Oracle Globally Distributed Database with System-Managed Sharding with additional control on resources like Memory and CPU allocated to Pods](./provisioning/system_sharding/ssharding_provisioning_with_control_on_resources.md)  
[4. Provisioning Oracle Globally Distributed Database with System-Managed Sharding by cloning database from your own Database Gold Image in the same Availability Domain(AD)](./provisioning/system_sharding/ssharding_provisioning_by_cloning_db_gold_image_in_same_ad.md)  
[5. Provisioning Oracle Globally Distributed Database with System-Managed Sharding by cloning database from your own Database Gold Image across Availability Domains(ADs)](./provisioning/system_sharding/ssharding_provisioning_by_cloning_db_from_gold_image_across_ads.md)  
[6. Provisioning Oracle Globally Distributed Database with System-Managed Sharding and send Notification using OCI Notification Service](./provisioning/system_sharding/ssharding_provisioning_with_notification_using_oci_notification.md)  
[7. Scale Out - Add Shards to an existing Oracle Globally Distributed Database provisioned earlier with System-Managed Sharding](./provisioning/system_sharding/ssharding_scale_out_add_shards.md)  
[8. Scale In - Delete an existing Shard from a working Oracle Globally Distributed Database provisioned earlier with System-Managed Sharding](./provisioning/system_sharding/ssharding_scale_in_delete_an_existing_shard.md)


## Provisioning Oracle Globally Distributed Database Topology with User-Defined Sharding in a Cloud-Based Kubernetes Cluster

Deploy Oracle Globally Distributed Database Topology with `User-Defined Sharding` on your Cloud based Kubernetes cluster. 

In this example, the deployment uses the YAML file based on `OCI OKE` cluster. There are multiple use case possible for deploying the Oracle Globally Distributed Database Topology covered by below examples:

[1. Provisioning Oracle Globally Distributed Database with User-Defined Sharding without Database Gold Image](./provisioning/user-defined-sharding/udsharding_provisioning_without_db_gold_image.md)  
[2. Provisioning Oracle Globally Distributed Database with User-Defined Sharding with additional control on resources like Memory and CPU allocated to Pods](./provisioning/user-defined-sharding/udsharding_provisioning_with_control_on_resources.md)  
[3. Provisioning Oracle Globally Distributed Database with User-Defined Sharding by cloning database from your own Database Gold Image in the same Availability Domain(AD)](./provisioning/user-defined-sharding/udsharding_provisioning_by_cloning_db_gold_image_in_same_ad.md)  
[4. Provisioning Oracle Globally Distributed Database with User-Defined Sharding by cloning database from your own Database Gold Image across Availability Domains(ADs)](./provisioning/user-defined-sharding/udsharding_provisioning_by_cloning_db_from_gold_image_across_ads.md)  
[5. Provisioning Oracle Globally Distributed Database with User-Defined Sharding and send Notification using OCI Notification Service](./provisioning/user-defined-sharding/udsharding_provisioning_with_notification_using_oci_notification.md)  
[6. Scale Out - Add Shards to an existing Oracle Globally Distributed Database provisioned earlier with User-Defined Sharding](./provisioning/user-defined-sharding/udsharding_scale_out_add_shards.md)  
[7. Scale In - Delete an existing Shard from a working Oracle Globally Distributed Database provisioned earlier with User-Defined Sharding](./provisioning/user-defined-sharding/udsharding_scale_in_delete_an_existing_shard.md)


## Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled in a Cloud-Based Kubernetes Cluster

Deploy Oracle Globally Distributed Database Topology with `System-Managed Sharding` and with `RAFT Replication` enabled on your Cloud based Kubernetes cluster. 

**NOTE: RAFT Replication Feature is available only for Oracle 23ai RDBMS and Oracle 23ai GSM version.**

In this example, the deployment uses the YAML file based on `OCI OKE` cluster. There are multiple use case possible for deploying the Oracle Globally Distributed Database Topology covered by below examples:

[1. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled without Database Gold Image](./provisioning/snr_system_sharding/snr_ssharding_provisioning_without_db_gold_image.md)  
[2. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled with number of chunks specified](./provisioning/snr_system_sharding/snr_ssharding_provisioning_with_chunks_specified.md)  
[3. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled with additional control on resources like Memory and CPU allocated to Pods](./provisioning/snr_system_sharding/snr_ssharding_provisioning_with_control_on_resources.md)  
[4. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled by cloning database from your own Database Gold Image in the same Availability Domain(AD)](./provisioning/snr_system_sharding/snr_ssharding_provisioning_by_cloning_db_gold_image_in_same_ad.md)  
[5. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled by cloning database from your own Database Gold Image across Availability Domains(ADs)](./provisioning/snr_system_sharding/snr_ssharding_provisioning_by_cloning_db_from_gold_image_across_ads.md)  
[6. Provisioning Oracle Globally Distributed Database Topology with System-Managed Sharding and Raft replication enabled send Notification using OCI Notification Service](./provisioning/snr_system_sharding/snr_ssharding_provisioning_with_notification_using_oci_notification.md)  
[7. Scale Out - Add Shards to an existing Oracle Globally Distributed Database provisioned earlier with System-Managed Sharding and RAFT replication enabled](./provisioning/snr_system_sharding/snr_ssharding_scale_out_add_shards.md)  
[8. Scale In - Delete an existing Shard from a working Oracle Globally Distributed Database provisioned earlier with System-Managed Sharding and RAFT reolication enabled](./provisioning/snr_system_sharding/snr_ssharding_scale_in_delete_an_existing_shard.md)

## Connecting to Oracle Globally Distributed Database

After the Oracle Globally Distributed Database Topology has been provisioned using the Sharding Controller in Oracle Database Kubernetes Operator, you can follow the steps in this document to connect to the Oracle Globally Distributed Database or to the individual Shards: [Database Connectivity](./provisioning/database_connection.md)

## Debugging and Troubleshooting

To debug the Oracle Globally Distributed Database Topology provisioned using the Sharding Controller of Oracle Database Kubernetes Operator, follow this document: [Debugging and troubleshooting](./provisioning/debugging.md)

## Known Issues

* Issue 1: For both ENTERPRISE and FREE Images, if the GSM POD is stopped using "crictl stopp" at the worker node level, it leaves GSM in failed state with the "gdsctl" commands failing with error GSM-45034: Connection to GDS catalog is not established". It is beacause with change, the network namespace is lost if we check from the GSM Pod.
* Issue 2: For both ENTERPRISE and FREE Images, reboot of node running CATALOG using "/sbin/reboot -f" results in "GSM-45076: GSM IS NOT RUNNING". Once you hit this issue, after waiting for a certain time, the "gdsctl" commands start working as the DB connection start working. Once the stack comes up fine after the node reboot, after some time, unexpected restart of GSM Pod is also observed.
* Issue 3: For both ENTERPRISE and FREE Images, reboot of node running the SHARD Pod using "/sbin/reboot -f" or stopping the Shard Database Pod from worker node using "crictl stopp" command leaves the shard in error state.
* Issue 4: For both ENTERPRISE and FREE Images, GSM pod restarts multiple times after force rebooting the node running GSM Pod. Its because when the worker node comes up, the GSM pod was recreated but it does not get DB connection to Catalog and meanwhile, the Liveness Probe fails which restart the Pod.