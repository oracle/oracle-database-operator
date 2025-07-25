# Prerequisites for running Oracle Restart Database Controller
 * [Kubernetes Cluster Requirements](#kubernetes-cluster-requirements)
    * [Prerequisites for Oracle Restart on OKE](#prerequisites-for-oracle-restart-on-oke)
    * [Preparing to Install Oracle Restart on OKE](#preparing-to-install-oracle-restart-on-oke)
    * [Worker Node Preparation for Oracle Restart on OKE](#worker-node-preparation-for-oracle-restart-on-oke)
        + [Download Oracle Grid Infrastructure and Oracle Database Software](#download-oracle-grid-infrastructure-and-oracle-database-software)
        + [Prepare the Worker Node for Oracle Restart Deployment](#prepare-the-worker-node-for-oracle-restart-deployment)
        + [Set up SELinux Module on Worker Nodes](#set-up-selinux-module-on-worker-nodes)
    * [Create a namespace for the Oracle Restart Setup](#create-a-namespace-for-the-oracle-restart-setup)
    * [Install Cert Manager and Setup Access Permissions](#install-cert-manager-and-setup-access-permissions)
    * [OpenShift Security Context Constraints](#openshift-security-context-constraints)
    * [Deploy Oracle Database Operator](#deploy-oracle-database-operator)
    * [Oracle Restart Database Slim Image](#oracle-restart-database-slim-image)
    * [Create a Kubernetes secret for the Oracle Restart Database installation owner for the Oracle Restart Database Deployment](#create-a-kubernetes-secret-for-the-oracle-restart-database-installation-owner-for-the-oracle-restart-database-deployment)

To deploy Oracle Restart Database using Oracle Restart Database Controller in Oracle Database Operator, you need a Kubernetes Cluster like an Oracle Kubernetes Engine(OKE). 

If you are using an Oracle Kubernetes Engine (OKE) Kubernetes Cluster, you will require:

  #### Kubernetes Cluster Requirements
  You must ensure that your Kubernetes Cluster meets the necessary requirements for Oracle Restart Database deployment. The minimum required OKE cluster version is 1.33.1 or higher. Refer documentation for details [Oracle Kubernetes Engine](https://docs.oracle.com/en-us/iaas/Content/ContEng/Concepts/contengoverview.htm) cluster.

  #### Prerequisites for Oracle Restart on OKE
  Before proceeding with the Oracle Restart Database deployment, ensure that you have completed the necessary prerequisites on your OKE cluster. This includes setting up the required infrastructure and configuring the necessary components. To use these instructions, you should have background knowledge of the technology and operating system.
  * Verify that all necessary dependencies are installed and up-to-date. You should be familiar with the following technologies:
    * Linux
    * Kubernetes
    * Oracle Kubernetes Engine Environment (OKE)
    * Oracle Real Database installation for Oracle Restart
    * Oracle Grid Infrastructure installation
    * Oracle Automatic Storage Management (Oracle ASM)


  #### Preparing to Install Oracle Restart on OKE
  To prepare for the Oracle Restart Database installation, follow these steps:
  * Ensure that your OKE cluster is properly configured and meets the necessary requirements. 
  Each pod that you deploy as part of your cluster must satisfy the minimum hardware requirements of the Oracle Restart Database and Oracle Grid Infrastructure software. If you are planning to install Oracle Grid Infrastructure and Oracle Restart database software on data volumes exposed from your environment, then you must have at least 50 GB space allocated for the Oracle Restart on OKE Pod.
  
  * In addition to the standard memory (RAM) required for Oracle Linux (Linux-x86-64), the Oracle Grid Infrastructure and Oracle Database instances, Oracle recommends that you provide an additional 2 GB of RAM to each Kubernetes node for the control plane. 
  
  * Database storage for Oracle Database on OKE must use Oracle Automatic Storage Management (Oracle ASM) configured on block storage.
    
  * Oracle Database on Kubernetes is currently supported with the following releases:
    * Oracle Grid Infrastructure Release 19.28 or later release updates
    * Oracle Database Release 19.28 or later
    * Oracle Kubernetes Engine Environment 1.33.1 or later
    * Unbreakable Enterprise Kernel Release 7 UEKR7 (Kernel Release 5.15.0-202.135.2.el9uek.x86_64 ) or later updates
    * Oracle Linux for Operator, control plane, and Worker nodes on Oracle Linux 8 (Linux-x86-64) Update 10 or later updates
  

  #### Worker Node Preparation for Oracle Restart on OKE
  * When configuring your Worker Nodes, follow these guidelines, and see the configuration Oracle used for testing.

    Each OKE worker node must have sufficient resources to support the intended number of Oracle Database Pods, each of which must meet at least the minimum requirements for Oracle Grid Infrastructure servers hosting an Oracle Database node.

  * The Oracle Database Pods in this example configuration were created on the machine worker-1 for Oracle Restart:
    - Oracle Database Node
      - Worker Node: worker-1
      - Container Pod: dbmc1-0

  * Worker node has the following configuration:
    - RAM:16GB
    - Operating system disk:
    - Ensure that your storage has at least the following available space:
        - / (Root): 40 GB
        - /scratch/orestart/: 80 GB (the worker node directory which will be used for /u01 to store Oracle Grid Infrastructure and Oracle Database homes)
        - /var/lib/containers: 50 GB xfs
        - /scratch/software/stage (the worker node directory for staging Oracle Grid Infrastructure and Oracle RDBMS software)
    - Oracle Linux 8.10 with Unbreakable Enterprise Kernel Release UEKR7 (Kernel Release 5.15.0-308.179.6.el8uek.x86_64) or later
    - Network Cards:
        - ens3: Default network interface. The Oracle Restart pod will use this network interface for cluster public network.

    - Block devices:
        - You can use any supported storage options for Oracle Grid Infrastructure. Ensure that your storage has at least the following space available:
          - /dev/oracleoci/oraclevdd (50 GB)
          - /dev/oracleoci/oraclevde (50 GB)
        - **Make sure the devices you are using for ASM Storage are cleared of any data from a previous usage or installation.**
        - **If you want to use the devices from the worker nodes for ASM storage, you will need to mark any default StorageClass as non-default in your Kubernetes Cluster.**


  ##### Download Oracle Grid Infrastructure and Oracle Database Software
  You need to download the Oracle Grid Infrastructure and Oracle Database software and stage it on the worker nodes. The Oracle Restart Database Controller will handle mounting the software inside the Pod.
  * The Oracle Database Container does not contain any Oracle software binaries. Download the following software from the [Oracle Technology Network](https://www.oracle.com/technetwork/database/enterprise-edition/downloads/index.html).
    - Oracle Grid Infrastructure 19c (19.28) for Linux x86-64
    - Oracle Database 19c (19.28) for Linux x86-64

  ##### Prepare the Worker Node for Oracle Restart Deployment
  To prepare the worker node for Oracle Restart Database deployment, follow these steps:
  Before you install Oracle Restart inside the OKE Pods, you must update the system configuration.

  1. Log in as root.
  2. Use the vim editor to update `/etc/sysctl.conf` parameters to the following values:
      ```sh
      fs.file-max = 6815744
      net.core.rmem_default = 262144
      net.core.rmem_max = 4194304
      net.core.wmem_default = 262144
      net.core.wmem_max = 1048576
      fs.aio-max-nr = 1048576
      ```
  3. Run the following commands:
      * `# sysctl -a`
      * `# sysctl –p`
  4. Verify that the swap memory is disabled by running:  
  ```sh
  [root@worker-1~]# free -m
  .....
  Swap:             0           0           0
  ```
  5. Enable kernel parameters at the Kubelet level, so that kernel parameters can be set at the Pod level. This is a one-time activity.
      * In the `/etc/systemd/system/kubelet.service.d/00-default.conf` file of OKE Worker nodes, add below environment variable: 
      ```txt
      Environment="KUBELET_EXTRA_ARGS=--fail-swap-on=false --allowed-unsafe-sysctls='kernel.shm*,net.*,kernel.sem'"
      ```
      * Reload Configurations: `# systemctl daemon-reload`
      * Restart Kubelet: `# systemctl restart kubelet`
      * Check the Kubelet status: `# systemctl status kubelet`
      
      **Note: For openshift worker nodes**, path to edit is `/etc/systemd/system/kubelet.service.d/99-kubelet-extra-args.conf` and add below content in this file:
      ```txt
      [Service]
      Environment="KUBELET_EXTRA_ARGS=--fail-swap-on=false --allowed-unsafe-sysctls='kernel.shm*,net.*,kernel.sem'"
      ```
      * Reload Configurations: `# systemctl daemon-reload`
      * Restart Kubelet: `# systemctl restart kubelet`
      * Check the Kubelet status: `# systemctl status kubelet`

  6. Create the mount points on the worker node to be mounted to Oracle Restart Pod for Oracle GI + RDBMS HOME and the Software Staging Location
      * On worker node:
          + `# mkdir -p /scratch/orestart/`
          + `# mkdir -p /scratch/software/stage`
  
  7. Download the Oracle Grid Infrastructure and Oracle RDBMS Software .zip files. Copy those files to the worker node at the staging location `/scratch/software/stage/`

  ##### Set up SELinux Module on Worker Nodes
  Traditional Unix security uses discretionary access control (DAC). SELinux is an example of mandatory access control. SELinux restricts many commands from the Oracle Restart Pod that are not allowed to run, which results in permission denied errors. To avoid such errors, you must create an SELinux policy package to allow certain commands.

  To set up the SELinux module on the worker node, follow these steps as `root` user to create an SELinux policy package on the worker node: 

  1. Verify that SELinux is enabled on the Worker node. For example: 
  ```sh
  [root@worker-1]# getenforce
  enforcing
  ```
  2. Install the SELinux devel package on the Worker nodes: `[root@worker-1]# dnf install selinux-policy-devel`
  3. Create a file `oradb-oke.te` under `/var/opt` on the Worker nodes with the below content:
    ```sh
    module oradb-oke 1.0;
     
    require {
            type kernel_t;
            class system syslog_read;
            type container_runtime_t;
            type container_init_t;
            class file getattr;
            type container_file_t;
            type lib_t;
            type textrel_shlib_t;
            type bin_t;
            class file { execmod execute map setattr };
    }
     
    #============= container_init_t ==============
    allow container_init_t container_runtime_t:file getattr;
    allow container_init_t bin_t:file map;
    allow container_init_t bin_t:file execute;
    allow container_init_t container_file_t:file execmod;
    allow container_init_t lib_t:file execmod;
    allow container_init_t textrel_shlib_t:file setattr;
    allow container_init_t kernel_t:system syslog_read;
    ```
4. Create and install the policy package: 
      * `# cd /var/opt`
      * `make -f /usr/share/selinux/devel/Makefile oradb-oke.pp`
      * `semodule -i oradb-oke.pp`
      * `semodule -l | grep oradb-oke`
5. Configure the SELinux context for the required worker node directory and files:
      * On worker-1:
          + `# semanage fcontext -a -t container_file_t /scratch/orestart`
          + `# sudo restorecon -vF /scratch/orestart`
          + `# semanage fcontext -a -t container_file_t  /scratch/software/stage/grid_home.zip`
          + `# sudo restorecon -vF /scratch/software/stage/grid_home.zip`
          + `# semanage fcontext -a -t container_file_t  /scratch/software/stage/db_home.zip`
          + `# sudo restorecon -vF /scratch/software/stage/db_home.zip`

**Note**: Change these paths and file names as per location of your environment for setting Oracle Restart and names of the software .zip files.

**Note**: To use Oracle Restart Database Controller, ensure that your system is provisioned with a supported Kubernetes release. Refer to the [Release Status Section](../../README.md#release-status).

### Create a namespace for the Oracle Restart Setup

Create a Kubernetes namespace named `orestart`. All the resources belonging to the Oracle Restart Database will be provisioned in this namespace named `orestart`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns orestart

  #### Check the created namespace 
  kubectl get ns orestart
  ```
If you want, you can choose any name for the namespace to deploy Oracle Restart Database.

### Install Cert Manager and Setup Access Permissions

Before using Oracle Restart Database Controller, ensure you have completed the prerequisites which includes:

* The installation of cert-manager
* Creation of Role Bindings for Access Management

Refer to the section [Prerequisites](../../../README.md#prerequisites)

### OpenShift Security Context Constraints

OpenShift requires additional Security Context Constraints (SCC) for deploying and managing the `oraclerestarts.database.oracle.com` resource. To create the appropriate SCCs before deploying the `oraclerestarts.database.oracle.com` resource, complete these steps:

  1. Apply the file [custom-kubeletconfig.yaml](../../config/samples/orestart/custom-kubeletconfig.yaml) with cluster-admin user privileges.

  ```sh
    oc apply -f custom-kubeletconfig.yaml
  ```
  Watch the worker MCP update:
  ```sh
   watch oc get mcp
   # Wait for: UPDATING = False ,UPDATED = True ,DEGRADED = False
   oc label mcp worker custom-kubelet=enable-unsafe-sysctls
   oc get kubeletconfig
   NAME                    AGE
   enable-unsafe-sysctls   10m
   ```

  **Note:** OpenShift recommends that you should not deploy in namespaces starting with `kube`, `openshift` and the `default` namespace.

  2. Create service account to be used for Openshift cluster to be used for `oraclerestarts.database.oracle.com` resource.
  ```sh
  oc create serviceaccount oraclerestart -n orestart
  ```
  Note: We are using `oraclerestart` as service account name, you can change and make sure to use same in yaml file while creating `oraclerestarts.database.oracle.com` resource and step 3 below.

  3. Apply the file [custom-scc.yaml](../../config/samples/orestart/custom-scc.yaml) with cluster-admin user privileges.

  ```sh
    oc apply -f custom-scc.yaml
    oc adm policy add-scc-to-user privileged -z oraclerestart -n orestart
  ```

### Deploy Oracle Database Operator

After you have completed the prerequisite steps, you can install the operator. To install the operator in the cluster quickly, you can apply the modified `oracle-database-operator.yaml` file from the previous step.

```sh 
kubectl apply -f oracle-database-operator.yaml
```

For more details, please refer to [Install Oracle DB Operator](../../../README.md#install-oracle-db-operator)

### Oracle Restart Database Slim Image
Choose one of the following deployment options: 

  **Use Oracle-Supplied Container Images:**
   The Oracle Restart Database Controller uses Oracle Restart Database Slim Image to provision the Oracle Restart Database.

   You can also download the pre-built Oracle Restart Database Slim Image `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim-0630`. This image is functionally tested and evaluated with various use cases of Oracle Restart Database on an OKE Kubernetes Cluster.

   You can either download this image and push it to your Container Images Repository, or, if your Kubernetes cluster can reach OCR, you can download this image directly from OCR.
   
   **OR**

  **Build your own Oracle Restart Database Slim Image:**
   You can build this image using instructions provided on Oracle's official OraHub Repositories:
   * [Oracle Restart Database Image](https://orahub.oci.oraclecorp.com/rac-docker-dev/rac-docker-images)

After the image is ready, push it to your Container Images Repository, so that you can pull this image during Oracle Restart Database provisioning..

**Note**: In the Oracle Restart Database provisioning sample .yaml files, we are using Oracle Restart Database slim image available on `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim-0630`.

### Create a Kubernetes secret for the Oracle Restart Database installation owner for the Oracle Restart Database Deployment

Create a Kubernetes secret named `db-user-pass` in `orestart` namespace using these steps: [Create Kubernetes Secret](./create_kubernetes_secret_for_db_user.md)

Create a Kubernetes secret named `ssh-key-secret` in `orestart` namespace using these steps: [Create Kubernetes Secret for SSH Key](./create_kubernetes_secret_for_ssh_setup.md)

After you have the above prerequsites completed, you can proceed to the next section for your environment to provision the Oracle Restart Database.
