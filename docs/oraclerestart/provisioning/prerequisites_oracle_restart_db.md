# Prerequisites for running Oracle Restart Database Controller
 * [Kubernetes Cluster Requirements](#kubernetes-cluster-requirements)
    * [Mandatory roles and privileges requirements for Oracle Restart Database Controller](#mandatory-roles-and-privileges-requirements-for-oracle-restart-database-controller)
    * [Prerequisites for Oracle Restart on OKE](#prerequisites-for-oracle-restart-on-oke)
    * [Preparing to Install Oracle Restart on OKE](#preparing-to-install-oracle-restart-on-oke)
    * [Worker Node Preparation for Oracle Restart on OKE](#worker-node-preparation-for-oracle-restart-on-oke)
        + [Download Oracle Grid Infrastructure and Oracle Database Software](#download-oracle-grid-infrastructure-and-oracle-database-software)
        + [Permission on the software files](#permission-on-the-software-files)
        + [Prepare the Worker Node for Oracle Restart Deployment](#prepare-the-worker-node-for-oracle-restart-deployment)
        + [Set up SELinux Module on Worker Nodes](#set-up-selinux-module-on-worker-nodes)
    * [Create a namespace for the Oracle Restart Setup](#create-a-namespace-for-the-oracle-restart-setup)
    * [Install Cert Manager and Setup Access Permissions](#install-cert-manager-and-setup-access-permissions)
    * [Additional requirements for OpenShift Security Context Constraints](#additional-requirements-for-openshift-security-context-constraints)
    * [Deploy Oracle Database Operator](#deploy-oracle-database-operator)
    * [Oracle Restart Database Slim Image](#oracle-restart-database-slim-image)
    * [Create a Kubernetes secret for the Oracle Restart Database installation owner for the Oracle Restart Database Deployment](#create-a-kubernetes-secret-for-the-oracle-restart-database-installation-owner-for-the-oracle-restart-database-deployment)

To deploy an Oracle Database using the Oracle Restart Database Controller in the Oracle Database Operator, you require a Kubernetes cluster such as Oracle Kubernetes Engine (OKE).

If you are using an Oracle Kubernetes Engine (OKE) Kubernetes Cluster, you will require:

  ## Kubernetes Cluster Requirements
  You must ensure that your Kubernetes Cluster meets the necessary requirements for Oracle Restart Database deployment. The minimum required OKE cluster version is 1.33 or higher. Refer documentation for details [Oracle Kubernetes Engine](https://docs.oracle.com/en-us/iaas/Content/ContEng/Concepts/contengoverview.htm) cluster.

  ## Mandatory roles and privileges requirements for Oracle Restart Database Controller
  Oracle Restart Database Controller uses Kubernetes objects such as :-

  | Resources | Verbs |
  | --- | --- |
  | Pods | create delete get list patch update watch |
  | Containers | create delete get list patch update watch |
  | PersistentVolumeClaims | create delete get list patch update watch |
  | Services | create delete get list patch update watch |
  | Secrets | create delete get list patch update watch |
  | Events | create patch |

  ## Prerequisites for Oracle Restart on OKE
 Before proceeding with the Oracle Database deployment, ensure that you have completed all required prerequisites on your Oracle Kubernetes Engine (OKE) cluster. This includes setting up the necessary infrastructure and configuring all relevant components. To effectively use these instructions, you should have background knowledge of both Kubernetes technology and the underlying operating system.
  * Verify that all necessary dependencies are installed and up-to-date. You should be familiar with the following technologies:
    * Linux
    * Kubernetes
    * Oracle Kubernetes Engine Environment (OKE)
    * Oracle Real Database installation for Oracle Restart
    * Oracle Grid Infrastructure installation
    * Oracle Automatic Storage Management (Oracle ASM)

  ## Preparing to Install Oracle Restart on OKE
  To prepare for the Oracle Restart Database installation, follow these steps:
   * Ensure that your OKE cluster is properly configured and meets the necessary requirements. 
     Each pod that you deploy as part of your cluster must satisfy the minimum hardware requirements of the Oracle Restart Database and Oracle Grid Infrastructure software. If you are planning to install Oracle Grid Infrastructure and Oracle Restart database software on host local volumes exposed from your environment, then you must have at least 50 GB space allocated for the Oracle Restart on OKE Pod.
   * In addition to the standard memory (RAM) required for Oracle Linux (Linux-x86-64), the Oracle Grid Infrastructure and Oracle Database instances, Oracle     recommends that you provide an additional 2 GB of RAM to each Kubernetes node for the control plane. 
   * Database storage for Oracle Database on OKE must use Oracle Automatic Storage Management (Oracle ASM) configured on block storage.
    
   * Prerequisite Software and Environment Versions: 
     * Oracle Grid Infrastructure: Release 19.28 or later
     * Oracle Database: Release 19.28 or later
     * Oracle Kubernetes Engine (OKE): Version 1.33 or later
     * Unbreakable Enterprise Kernel (UEK): Release 7 UEKR7 (Kernel Release 5.15.0-202.135.2.el9uek.x86_64) or later
     * Oracle Linux: For Operator, control plane, and worker nodes—Oracle Linux 8 (Linux-x86-64) Update 10 or later
       
  ## Worker Node Preparation for Oracle Restart on OKE
   * When configuring your Worker Nodes, follow these guidelines, and see the configuration Oracle used for testing.
     Each OKE worker node must have sufficient resources to support the intended number of Oracle Database Pods, each of which must meet at least the minimum requirements for Oracle Grid Infrastructure servers hosting an Oracle Database node.
   * The Oracle Database Pods in this example configuration were created on the machine `10.0.10.58` for Oracle Restart:
      - Oracle Database Node
        - Worker Node: 10.0.10.58
        - Container Pod: dbmc1-0
   * Worker node has the following configuration:
      - RAM:32GB
      - Operating system disk:
      - Ensure that your storage has at least the following available space:
          - / (Root): 40 GB
          - /scratch/orestart/: 80 GB (the worker node directory which will be used for /u01 to store Oracle Grid Infrastructure and Oracle Database homes)
          - /var/lib/containers: 100 GB xfs
      - Oracle Linux 8.10 with Unbreakable Enterprise Kernel Release UEKR7 (Kernel Release 5.15.0-308.179.6.el8uek.x86_64) or later
      - Network Cards:
          - ens3: Default network interface. The Oracle Restart pod will use this network interface for cluster public network.
      - Skip this step **if you are using a StorageClass**, You only need to configure the following storage locations on the worker node for the software volume and ASM disks.    
        - /scratch/software/stage (the worker node directory will be exposed as local host volume to the Oracle Restart pod for staging Oracle Grid Infrastructure and Oracle RDBMS software)
        - Block devices:
          - You can use any supported storage options for Oracle Grid Infrastructure. Ensure that your storage has at least the following space available:
            - /dev/oracleoci/oraclevdd (50 GB)
            - /dev/oracleoci/oraclevde (50 GB)

        **Notes**  
          - Make sure the devices you are using for ASM Storage are cleared of any data from a previous usage or installation.**
          - If you want to use the devices from the worker nodes for ASM storage, you will need to mark any default StorageClass as non-default in your Kubernetes Cluster.**

  ### Download Oracle Grid Infrastructure and Oracle Database Software
  You need to download the Oracle Grid Infrastructure and Oracle Database software, you can make it available inside the Pod using following steps:

  * Prepare the Persistent Volume (PVC) for Staging: 
    * If you plan to use an existing Persistent Volume Claim (PVC) as a staging area, ensure it is pre-created and available before deployment.
    * NFS (Network File System) is commonly used as a backing storage for staging, since it allows multiple pods/nodes to access the staged files.
  * Copy Software to Staging Location: 
    * Download the Oracle Grid Infrastructure and Oracle Database installation media from Oracle's official sources.
    * Copy (stage) these files onto the PVC (or NFS volume) you intend to use. If you opt to stage on a worker node's local storage, ensure sufficient space and security practices.
  * Mounting in Pod: 
    * The Oracle Restart Database Controller is responsible for mounting the staged software into the Pod at runtime, making the installers available for use during installation or upgrade tasks.
    * You should define the appropriate volume mounts in your deployment YAML manifests to ensure the pod sees the staged content.
         
  * The Oracle Database Container does not contain any Oracle software binaries. Download the following software from the [Oracle Technology Network](https://www.oracle.com/technetwork/database/enterprise-edition/downloads/index.html).
    - Oracle Grid Infrastructure 19c (19.28) for Linux x86-64
    - Oracle Database 19c (19.28) for Linux x86-64

  ### Permission on the software files

  Depending on the wheter you are provisioning the Oracle Restart Database using Base Release sofware or you are applying an RU patch or any one-off patch, please set the below permissions on the software files in the staging location:

  - Set the permission on the GRID Infrastructure Software and RDBMS Software .zip files to be 755.
  - Set the permission on the Opatch .zip file to be 755
  - Set the permission on the unzipped RU software directory to be 755 recursively.
  - Set the permission on the unzipped oneoff patch software directory to be 755 recursively.


  ### Worker Node Preparation Checklist
  Preparing the worker node is a critical foundation for a secure and successful Oracle Restart Database deployment in a Kubernetes environmen. These steps need to be executed by Kuberernetes administrator as root user on worker nodes and follow these steps:
  
  #### System Requirements
   * Verify OS and Kernel Versions: Ensure your node’s operating system and kernel version are supported by Oracle Database and Kubernetes.
   * Resource Allocation: Confirm the node has sufficient CPU, memory, and storage for Oracle Grid and Database.
  #### Kernel and System Settings
   * Use the vim editor to update `/etc/sysctl.conf` parameters to the following values:
      ```sh
      fs.file-max = 6815744
      net.core.rmem_default = 262144
      net.core.rmem_max = 4194304
      net.core.wmem_default = 262144
      net.core.wmem_max = 1048576
      fs.aio-max-nr = 1048576
      vm.nr_hugepages=16384
      ```
   * Run the following commands:
      * `# sysctl -a`
      * `# sysctl –p`
   * Verify that the swap memory is disabled by running:  
      ```sh
      # free -m
      .....
      Swap:             0           0           0
      ```
   * Enable kernel parameters at the Kubelet level, so that kernel parameters can be set at the Pod level. This is a one-time activity.
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

   * Skip this step **if you are using a StorageClass**.Otherwise, create the necessary mount points on the worker node. These mount points will be used by the Oracle Restart pod for Oracle Grid Infrastructure and RDBMS Home, as well as for the software staging location.
      * On worker node:
          + `# mkdir -p /scratch/orestart/`
          + `# mkdir -p /scratch/software/stage`
      * For the case where you are installing Oracle Base Release with RU Patch, create the required mount points for Base Release Software, for the location to unzip the RU Patch etc:
        * For Example, for Release 19c with 19.28 RU, on worker node:
          + `# mkdir -p /stage/software/19c/19.3.0`
          + `# mkdir -p /stage/software/19c/19.28`     
  
      * Download the Oracle Grid Infrastructure and Oracle RDBMS Software .zip files. Copy those files to the worker node at the staging location `/scratch/software/stage/`

  #### Security Controls
  Traditional Unix security uses discretionary access control (DAC). SELinux is an example of mandatory access control. SELinux restricts many commands from the Oracle Restart Pod that are not allowed to run, which results in permission denied errors. To avoid such errors, you must create an SELinux policy package to allow certain commands.

  To set up the SELinux module on the worker node, follow these steps as `root` user to create an SELinux policy package on the worker node: 

  * Verify that SELinux is enabled on the Worker node. For example: 
  ```sh
  # getenforce
  enforcing
  ```
  * Install the SELinux devel package on the Worker nodes: `# dnf install selinux-policy-devel`
  * Create a file `oradb-oke.te` under `/var/opt` on the Worker nodes with the below content:
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
* Create and install the policy package: 
  * `# cd /var/opt`
  * `make -f /usr/share/selinux/devel/Makefile oradb-oke.pp`
  * `semodule -i oradb-oke.pp`
  * `semodule -l | grep oradb-oke`
* Skip this step, **if you are using storgaclass**. Configure the SELinux context for the required worker node directory and files:
    * On worker node:
      + `# semanage fcontext -a -t container_file_t /scratch/orestart`
      + `# sudo restorecon -vF /scratch/orestart`
      + `# semanage fcontext -a -t container_file_t  /scratch/software/stage/grid_home.zip`
      + `# sudo restorecon -vF /scratch/software/stage/grid_home.zip`
      + `# semanage fcontext -a -t container_file_t  /scratch/software/stage/db_home.zip`
      + `# sudo restorecon -vF /scratch/software/stage/db_home.zip`

  **Notes:** 
  * Change these paths and file names as per location of your environment for setting Oracle Restart and names of the software .zip files.
  * In case of Oracle Base Release and RU Patch software, you will need to run similar commands for the corresponding .zip files.

## Create a namespace for the Oracle Restart Setup
Create a Kubernetes namespace named `orestart`. All the resources belonging to the Oracle Restart Database will be provisioned in this namespace named `orestart`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns orestart

  #### Check the created namespace 
  kubectl get ns orestart
  ```
If you want, you can choose any name for the namespace to deploy Oracle Restart Database.

## Install Cert Manager and Setup Access Permissions
Before using Oracle Restart Database Controller, ensure you have completed the prerequisites which includes:

* The installation of cert-manager
* Creation of Role Bindings for Access Management

Refer to the section [Prerequisites](../../../README.md#prerequisites)

Apart from the default Role Bindings for access management mentioned in above section, you will require the below mentioned role bindings:

  For exposing the database using Nodeport services, apply [RBAC](../../../rbac/node-rbac.yaml)
  ```sh
    kubectl apply -f rbac/node-rbac.yaml
  ```
  For automatic storage expansion of block volumes, apply [RBAC](../../../rbac/storage-class-rbac.yaml)
  ```sh
    kubectl apply -f rbac/storage-class-rbac.yaml
  ```
  For getting get, list and watch privileges on the persistent volumes, apply [RBAC](../../rbac/persistent-volume-rbac.yaml)
  ```sh
    kubectl apply -f rbac/persistent-volume-rbac.yaml
  ```


## Additional requirements for OpenShift Security Context Constraints
When you deploy Oracle Restart Database using the Oracle Restart Database Controller on an OpenShift cluster, you must account for OpenShift's stricter security model, especially around Security Context Constraints (SCCs). 

Apart from the same steps listed above for setting up Oracle Restart Database using Oracle Restart Database Controller on an OKE Cluster, there are some additional steps required to setup Oracle Restart Database using Oracle Restart Database Controller on an OpenShift Cluster.

OpenShift requires additional Security Context Constraints (SCC) for deploying and managing the `oraclerestarts.database.oracle.com` resource. To create the appropriate SCCs before deploying the `oraclerestarts.database.oracle.com` resource, complete these steps:

  * Apply the file [custom-kubeletconfig.yaml](../../config/samples/orestart/custom-kubeletconfig.yaml) with cluster-admin user privileges.

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

  * Create service account to be used for Openshift cluster to be used for `oraclerestarts.database.oracle.com` resource.
  ```sh
  oc create serviceaccount oraclerestart -n orestart
  ```
  Note: We are using `oraclerestart` as service account name, you can change and make sure to use same in yaml file while creating `oraclerestarts.database.oracle.com` resource and step 3 below.

  * Apply the file [custom-scc.yaml](../../config/samples/orestart/custom-scc.yaml) with cluster-admin user privileges.

  ```sh
  oc apply -f custom-scc.yaml
  oc adm policy add-scc-to-user privileged -z oraclerestart -n orestart
  ```

## Deploy Oracle Database Operator
After you have completed the prerequisite steps, you can install the operator. To install the operator in the cluster quickly, you can apply the modified `oracle-database-operator.yaml` file from the previous step.

```sh 
kubectl apply -f oracle-database-operator.yaml
```

For more details, please refer to [Install Oracle DB Operator](../../../README.md#install-oracle-db-operator)

## Oracle Restart Database Slim Image

  #### Build your own Oracle Restart Database Slim Image
   You can build this image using instructions provided in below documentation:
   * [Building Oracle RAC Database Container Slim Image](https://github.com/oracle/docker-images/blob/main/OracleDatabase/RAC/OracleRealApplicationClusters/README.md#building-oracle-rac-database-container-slim-image)

After the image is ready, push it to your private container images repository, so that you can pull this image during Oracle Restart Database provisioning..

**Note**: In the Oracle Restart Database provisioning sample .yaml files, we are using Oracle Restart Database slim image `odbcir/oracle/database-orestart:19.3.0-slim`.

## Create a Kubernetes secret for the Oracle Restart Database installation owner for the Oracle Restart Database Deployment
 * Create a Kubernetes secret named `db-user-pass` in `orestart` namespace using these steps: [Create Kubernetes Secret](./create_kubernetes_secret_for_db_user.md)
   * Once the setup completes, you can change the password inside the pod for Oracle sys user.
 * Create a Kubernetes secret named `ssh-key-secret` in `orestart` namespace using these steps: [Create Kubernetes Secret for SSH Key](./create_kubernetes_secret_for_ssh_setup.md)

After you have the above prerequsites completed, you can proceed to the next section for your environment to provision the Oracle Restart Database.