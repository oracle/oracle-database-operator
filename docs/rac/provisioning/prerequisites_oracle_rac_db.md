# Prerequisites for using Oracle RAC Database Controller
Prepare your Kubernetes environment and nodes to deploy Oracle Real Application Clusters (RAC) using the Oracle RAC Database Controller and Oracle Database Operator.

* [Mandatory roles and privileges requirements for Oracle RAC Database Controller](#mandatory-roles-and-privileges-requirements-for-oracle-rac-database-controller) 
* [Kubernetes Cluster: Oracle Cloud Native Environment (OCNE)](#kubernetes-cluster-oracle-cloud-native-environment-ocne) 
   * [Preparing to Install Oracle RAC on OCNE](#preparing-to-install-oracle-rac-on-ocne) 
   * [Software and Storage Requirements for Oracle RAC on OCNE](#software-and-storage-requirements-for-oracle-rac-on-ocne) 
      + [Download Oracle Grid Infrastructure and Oracle Database Software](#download-oracle-grid-infrastructure-and-oracle-database-software) 
      + [Permission on the software files](#permission-on-the-software-files) 
   * [Network Setup Requirements for Oracle RAC on OCNE](#network-setup-requirements-for-oracle-rac-on-ocne) 
   * [OCNE Worker Node Configuration](#ocne-worker-node-configuration) 
   * [Configure Multus to create Oracle RAC Private Network](#configure-multus-to-create-oracle-rac-private-network) 
   * [Set Clock Source on the Worker Node](#set-clock-source-on-the-worker-node) 
   * [Setting up a Network Time Service](#setting-up-a-network-time-service) 
   * [Configuring HugePages for Oracle RAC](#configuring-hugepages-for-oracle-rac) 
   * [Prepare the Worker Node for Oracle RAC Deployment](#prepare-the-worker-node-for-oracle-rac-deployment)
      + [System Requirements](#system-requirements) 
      + [Kernel and System Settings](#kernel-and-system-settings) 
   * [Set up SELinux Module on Worker Nodes](#set-up-selinux-module-on-worker-nodes) 
   * [Using CVU to Validate Readiness for Worker Node](#using-cvu-to-validate-readiness-for-worker-node) 
* [Create a namespace for the Oracle RAC Setup](#create-a-namespace-for-the-oracle-rac-setup) 
* [Complete the prerequisites before installing Oracle Database Operator](#complete-the-prerequisites-before-installing-oracle-database-operator) 
* [Deploy Oracle Database Operator](#deploy-oracle-database-operator) 
* [Oracle RAC Database Slim Image](#oracle-rac-database-slim-image) 
* [Create Kubernetes secrets for the Oracle RAC Database](#create-kubernetes-secrets-for-the-oracle-rac-database) 
* [Create required directories on worker nodes](#create-required-directories-on-worker-nodes) 
* [Label the worker nodes](#label-the-worker-nodes) 
* [ClusterRole and ClusterRoleBinding for Multus Access](#clusterrole-and-clusterrolebinding-for-multus-access) 
* [ClusterRole and ClusterRoleBinding for PersistentVolume Access](#clusterrole-and-clusterrolebinding-for-persistentvolume-access) 

**IMPORTANT :** You must make the changes specified in this section before you proceed to the next section.

## Mandatory roles and privileges requirements for Oracle RAC Database Controller
 Oracle RAC Database Controller requires the following Kubernetes object permissions:


  | Resources | Verbs |
  | --- | --- |
  | Pods | create delete get list patch update watch |
  | Containers | create delete get list patch update watch |
  | PersistentVolumeClaims | create delete get list patch update watch |
  | Services | create delete get list patch update watch |
  | Secrets | create delete get list patch update watch |
  | Events | create patch |

## Kubernetes Cluster: Oracle Cloud Native Environment (OCNE)

Ensure your Kubernetes cluster meets all Oracle RAC deployment prerequisites on OCNE.

To deploy Oracle RAC Database using Oracle RAC Database Controller in Oracle Database Operator, you need a Kubernetes Cluster like a Kubernetes Cluster Deployed on an Oracle Cloud Native Environment (OCNE). 

If you are using a Kubernetes Cluster Deployed on an Oracle Cloud Native Environment (OCNE), you will require:
 
* An On-Premise Kubernetes Cluster deployed using [Oracle Linux Cloud Native Environment (OCNE)](https://docs.oracle.com/en/operating-systems/olcne/).
  * You must have OCNE cluster 1.9 or higher.
  * You need to refer [Oracle Real Application Clusters Installation on Oracle Cloud Native Environment](https://docs.oracle.com/en/database/oracle/oracle-database/19/rackb/install-configure-rac-olcne.html) and complete following sections as mentioned below:
     * Prerequisites for Oracle RAC on OCNE 
        * Preparing to Install Oracle RAC on OCNE 
        * Software and Storage Requirements for Oracle RAC on OCNE 
        * Network Setup Requirements for Oracle RAC on OCNE 
     * Worker Node Preparation for Oracle RAC on OCNE 
        * OCNE Worker Node Configuration 
        * Configure Multus to create Oracle RAC Private Network 
        * Set Clock Source on the Worker Node
        * Setting up a Network Time Service
        * Configuring HugePages for Oracle RAC 
        * Prepare the Worker Node for Oracle RAC Deployment 
        * Set up SELinux Module on Worker Nodes 
        * Using CVU to Validate Readiness for Worker Node 

To use Oracle RAC Database Controller, ensure that your system is provisioned with a supported Kubernetes release. Refer to the [Release Status Section](../../../README.md#release-status).

### Preparing to Install Oracle RAC on OCNE

To effectively use these instructions, you should have background knowledge of both Kubernetes technology and the underlying operating system. These include:
  * Linux
  * Kubernetes
  * Oracle Cloud Native Environment (OCNE)
  * Oracle Real Application Clusters (Oracle RAC) installation
  * Oracle Grid Infrastructure installation
  * Oracle Automatic Storage Management (Oracle ASM)


### Software and Storage Requirements for Oracle RAC on OCNE 

  To prepare for the Oracle RAC Database installation, follow these steps:

   * Ensure that your OCNE cluster is properly configured and meets the necessary requirements.
     Each pod that you deploy as part of your cluster must satisfy the minimum hardware requirements of the Oracle RAC Database and Oracle Grid Infrastructure software. If you are planning to install Oracle Grid Infrastructure and Oracle RAC database software on host local volumes exposed from your environment, then you must have at least 50 GB space allocated for the Oracle RAC on OKE Pod.
   * In addition to the standard memory (RAM) required for Oracle Linux (Linux-x86-64), the Oracle Grid Infrastructure and Oracle Database instances, Oracle recommends that you provide an additional 2 GB of RAM to each Kubernetes node for the control plane.
   * Database storage for Oracle RAC Database on OCNE must use Oracle Automatic Storage Management (Oracle ASM) configured on shared block storage.

   * Prerequisite Software and Environment Versions:
     * Oracle Grid Infrastructure: Release 19.28 or later
     * Oracle Database: Release 19.28 or later
     * In case you want to save time used in patching during the deployment, you can use "Gold Image" for the Grid Infrastructure and Oracle Database Software Installation. "Gold Image" are binaries which includes the base software, latest RU, recommended patches, and any other patches based on customer selection. Please refer to below Oracle Support Documents for more details:
        - [Gold Image How To (Doc ID 2965269.1)](https://support.oracle.com/support/?kmContentId=2965269.1) 
        - [Creating Gold Image for Oracle Database and Grid Infrastructure Installations (Doc ID 2915366.2)](https://support.oracle.com/support/?kmContentId=2915366.2) 
     * For Example: If you want to use the Oracle Grid Infrastructure and Oracle Database Release 19.28 for your deployment, you can build the "Gold Image" using the base software as 19.3.0, the GI RU 19.28.0 (37957391), the recommended patches (38336965 and 34436514) and any other additional patch which you want to apply to your GI and RDBMS Home. 
     * Oracle Cloud Native Environment (OCNE): Version 1.9 or later
     * Unbreakable Enterprise Kernel (UEK): Release 7 UEKR7 (Kernel Release 5.15.0-208.159.3.2.el8uek.x86_64) or later updates
     * Oracle Linux for Operator, control plane, and Worker nodes on Oracle Linux 8 (Linux-x86-64) Update 5 or later updates
     * Oracle Linux for Operator, control plane, and Worker nodes on Oracle Linux 9 (Linux-x86-64) Update 3 or later updates. 

  #### Download Oracle Grid Infrastructure and Oracle Database Software

  Stage Oracle RAC installation media for use in pods.


1. Download the Oracle Grid Infrastructure and Oracle Database software.
2. Copy the software to the staging location on the worker nodes (under directory `/scratch/software/stage` in this case). 
   * Download the Oracle Grid Infrastructure and Oracle Database installation media from Oracle's official sources: [Oracle Technology Network](https://www.oracle.com/technetwork/database/enterprise-edition/downloads/index.html). Ensure sufficient space and security practices when using the worker node's local storage.
3. The Oracle RAC Database Controller will mount the staged software into the Pod at runtime, making the installers available for use during installation or upgrade tasks.
4. In your deployment YAML, define the appropriate volume mount manifests so that the pod containers can access the staged content.

  #### Permission on the software files

Ensure permissions on Oracle RAC installation media are set. 

Whether you are provisioning the Oracle RAC Database using Base Release sofware or you are applying an RU patch or any one-off patch, you must set permissions on the software files in the staging location:

  - Set the permission on the GRID Infrastructure Software and RDBMS Software .zip files to be 755.
  - Set the permission on the Opatch .zip file to be 755
  - Set the permission on the unzipped RU software directory to be 755 recursively.
  - Set the permission on the unzipped oneoff patch software directory to be 755 recursively.     

### Network Setup Requirements for Oracle RAC on OCNE 

Configure network interfaces for Oracle RAC deployment in OCNE.

Complete the Multus and Flannel network requirements for OCNE. An Oracle Clusterware configuration requires at least two interfaces:

1. **Public network** interface: Users and application servers connect to access data on the database server.
2. **Private network** interface: Used for internode communication between cluster member nodes. 

To create these required interfaces, you can use the following pod networking technologies with your Kubernetes cluster:

* **Flannel**: This is the default networking option when you create a Kubernetes module. We use Flannel for the Oracle RAC public network. For Oracle RAC on OCNE, we support default pod networking with Flannel.
* **Multus**: You can set up the Multus module after the Kubernetes module is installed. Multus is installed as a module on top of Flannel. By default in Kubernetes, a pod is configured with single interface (and loopback)-based selected pod networking. To enable Kubernetes' multi-networking capability, Multus creates Custom Resources Definition (CRD)-based network objects, and creates a multi-container networking interface (CNI) plug-in. The Kubernetes application programming interface (API) can be expanded through CRD itself. For the Oracle RAC private network, you must configure Multus to have multiple network cards on different subnets, as described in [_Oracle Real Application Clusters Installation on Oracle Cloud Native Environment Oracle Linux x86-64_](https://docs.oracle.com/en/database/oracle/oracle-database/19/rackb/install-configure-rac-olcne.html#GUID-46911DE1-AA25-4B38-9D3E-845B5528B2FD).

### OCNE Worker Node Configuration 

Configure resources and networking for Oracle RAC node pods.

* Each OCNE worker node must have sufficient resources to support the intended number of Oracle RAC Pods, each of which must meet at least the minimum requirements for Oracle Grid Infrastructure servers hosting an Oracle Real Application Clusters node.

* The Oracle RAC Pods in this example configuration were created on the machines "qck-ocne19-w1", "qck-ocne19-w2" and "qck-ocne19-w3" for Oracle Real Application Clusters (Oracle RAC):

    * Oracle RAC Node 1 
      * Worker node: qck-ocne19-w1 
      * Pod: racnode1-0 

    * Oracle RAC Node 2 
      * Worker node: qck-ocne19-w2 
      * Pod: racnode2-0 

    * Oracle RAC Node 3 
      * Worker node: qck-ocne19-w3 
      * Pod: racnode3-0 

* Each worker node has the following configuration:

  - RAM: 60GB
  - Operating system disk: You can use any supported storage options for Oracle Grid Infrastructure. Ensure that your storage has at least the following available space:
        - Root (/): 40 GB
        - /scratch: 80 GB (the Worker node directory, which will be used for /u01 to store Oracle Grid Infrastructure and Oracle Database homes)
        - /var/lib/containers: 100 GB xfs
        - /scratch/software/stage:(the stage directory for Oracle software)
  - Oracle Linux 9.3 with Unbreakable Enterprise Kernel Release UEKR7 (Kernel Release 5.15.0-208.159.3.2.el9uek.x86_64) or later
  - Oracle recommends that you provide redundant private networks for the Oracle RAC cluster. Each worker node has two interfaces for this purpose. In this document example, we use ens5 and ens6 for the private networks. Each private network interface is connected to a separate and isolated network. If the interface and the underlying network support Jumbo Frame, then Oracle recommends that you configure the ens5 and ens6 interfaces with Jumbo Frame MTU 9000 in the worker node, without configuring any IP addresses on these interfaces. In this case, Multus will expose two network cards inside the pod as eth1 and eth2, based on the ens5 and ens6 network interfaces configured in the worker node. Network Cards:

      - ens3: Default network interface. It will be used by Flannel, and inside the pod, it will be used for the cluster public network.
      - ens5: First private network interface. It will be used by Multus for the cluster first private network.
      - ens6: Second private network interface. It will be used by Multus for the cluster second private network.

* Block devices, shared by both Worker nodes

  You can use any supported storage options for Oracle Grid Infrastructure. Ensure that your storage has at least the following available space:

    - /dev/sdd (50 GB)
    - /dev/sde (50 GB)

**Notes:**
  - Depending on your requirements, you may need additional shared block devices on these worker nodes. 
  - Ensure that the devices used for ASM Storage do not contain data from previous uses.  

### Configure Multus to create Oracle RAC Private Network 

To create an Oracle RAC private network, configure Multus either by using the example configuration file [multus-rac-conf.yaml](./multus-rac-conf.yaml), or by using your own configuration file.

  In the example setup, the Multus module was installed without the optional argument for a configuration file to configure it later. Also refer to "Oracle Cloud Native Environment Container Orchestration for Release 1.9" for step-by-step procedures to create, validate and deploy the Multus module. Oracle supports using Multus config types `macvlan` and `ipvlan`.

After you create your Multus configuration file, apply it to the system. For example, to apply multus-rac-conf.yaml, enter the following command: 
  ```sh
  kubectl apply -f multus-rac-conf.yaml
  ```
Check the Multus network attachment definitions on the OCNE cluster:
  ```sh
  $ kubectl get all -n kube-system -l app=multus
  ```
- Check the network attachment definitions as below:
  ```sh
  $ kubectl get Network-Attachment-Definition -n rac
  $ kubectl describe Network-Attachment-Definition macvlan-conf1 -n rac
  $ kubectl describe Network-Attachment-Definition macvlan-conf2 -n rac
  ```  

### Set Clock Source on the Worker Node 

Oracle recommends that you set the clock source to TSC for better performance in virtual environments (VM) on Linux x86-64.

With container deployments, the worker node containers inherit the clock source of their Linux host. For better performance, and to provide the clock source expected for an Oracle Real Application Clusters (Oracle RAC) database installation, Oracle recommends that you change the clock source setting to TSC. 

1. As the root user, check if the `tsc` clock source is available on your system:
   ```sh
   # cat /sys/devices/system/clocksource/clocksource0/available_clocksource
   kvm-clock tsc acpi_pm 
   ```

2. If the `tsc` clock source is available, then set `tsc` as the current clock source:
   ```sh
   # echo "tsc">/sys/devices/system/clocksource/clocksource0/current_clocksource
   ```

3. Verify that the current clock source is set to `tsc`:
   ```sh
   # cat /sys/devices/system/clocksource/clocksource0/current_clocksource

   tsc
   ```

4. Using any text editor, append the clocksource directive to the `GRUB_CMDLINE_LINUX` line in the `/etc/default/grub` file to retain this clock source setting after a restart:
   ```sh
   GRUB_CMDLINE_LINUX="rd.lvm.lv=ol/root rd.lvm.lv=ol/swap rhgb quiet numa=off transparent_hugepage=never clocksource=tsc" 
   ```

### Setting up a Network Time Service

You must set up the `chronyd` time service for Oracle Real Application Clusters (Oracle RAC).

As a clustering environment, Oracle Cloud Native Environment (OCNE) requires that the system time is synchronized across each Kubernetes control plane and worker node within the cluster. Typically, this can be achieved by installing and configuring a Network Time Protocol (NTP) daemon on each node. Oracle recommends installing and setting up the `chrony` daemon (`chronyd`) for this purpose. 

As noted in [Oracle Cloud Native Environment Release 1.9 installation guide](https://docs.oracle.com/en/operating-systems/olcne/1.9/install/prereq.html#sw):

"The chronyd service is enabled and started by default on Oracle Linux systems.

"Systems running on Oracle Cloud Infrastructure are configured to use the `chronyd` time service by default, so you don't need to add or configure NTP if you're installing into an Oracle Cloud Infrastructure environment. "

### Configuring HugePages for Oracle RAC 

HugePages is a feature integrated into the Linux kernel. For Oracle Database, using HugePages reduces the operating system maintenance of page states and increases Translation Lookaside Buffer (TLB) hit ratio.

To configure HugePages on the Linux operating system on OCNE, complete the following steps on the worker nodes:

1. In the /etc/sysctl.conf file, add the following entry:
   ```sh
   vm.nr_hugepages=16384
   ```

2. As root, run the following commands:
   ```sh
   sysctl -p
   sysctl -a 
   ```

3. Run the following command to display the value of the `Hugepagesize` variable: 
   ```sh
   $ grep Hugepagesize /proc/meminfo
   ```

4. To check the configuration, as root run `grep HugePages_Total` and `grep Hugepagesize` on `/proc/meminfo`:
   ```sh
   # grep HugePages_Total /proc/meminfo
   HugePages_Total:   16384
 
   # grep Hugepagesize: /proc/meminfo
   Hugepagesize:       2048 kB
   ``` 

5. Run the following command to check the available hugepages:
   ```sh
   $ grep HugePages_Total /proc/meminfo
   HugePages_Total: 16384
   HugePages_Free: 16384
   HugePages_Rsvd: 0
   HugePages_Surp: 0
   ```

### Prepare the Worker Node for Oracle RAC Deployment 

  Preparing the worker node is a critical foundation for a secure and successful Oracle RAC Database deployment in a Kubernetes environment. These steps must be run by a Kuberernetes administrator as the root user on worker nodes.

Complete all of these steps:

  #### System Requirements
   * Verify OS and Kernel Versions: Ensure your node’s operating system and kernel version are supported by Oracle Database and Kubernetes.
   * Resource Allocation: Confirm the node has sufficient CPU, memory, and storage for Oracle Grid Infrastructure and the database.

  #### Kernel and System Settings
   * Use the `vim` editor to update `/etc/sysctl.conf` parameters to the following values:
      ```sh
      fs.file-max = 6815744
      net.core.rmem_default = 262144
      net.core.rmem_max = 4194304
      net.core.wmem_default = 262144
      net.core.wmem_max = 1048576
      fs.aio-max-nr = 1048576
      ```
   * Run the following commands:
      * `sysctl -a`
      * `sysctl –p`

   * Verify that the swap memory is disabled by running the following commands:
      ```sh
      free -m
      .....
      Swap:             0           0           0
      ```
     Swap must be disabled, because Kubernetes doesn't allow the kubelet to come up if swap is enabled.

   * Enable kernel parameters at the kubelet level, so that kernel parameters can be set at the Pod level. This is a one-time activity.
      * In the `/etc/sysconfig/kubelet` file, add or replace `--allowed-unsafe-sysctls` under the `KUBELET_EXTRA_ARGS` environment variable as shown below: 
      ```txt
      KUBELET_EXTRA_ARGS="--fail-swap-on=false --allowed-unsafe-sysctls='kernel.shm*,net.*,kernel.sem'"
      ```
      * Reload Configurations:
      `systemctl daemon-reload`
      * Restart the kubelet:
      `systemctl restart kubelet`
      * Check the kubelet status:
      `systemctl status kubelet`

### Set up SELinux Module on Worker Nodes 

To enable command access, create an SELinux policy package on the Worker nodes.

Traditional Unix security uses discretionary access control (DAC). SELinux is an example of mandatory access control. SELinux restricts many commands from the RAC Pods that are not allowed to run, which results in permission denied errors. To avoid such errors, you must create an SELinux policy package to allow certain commands.

  To set up the SELinux module on the worker node, follow these steps as `root` user to create an SELinux policy package on the worker node:

  * 1. Verify that SELinux is enabled on the Worker node. For example:
    ```sh
    # getenforce
    enforcing
    ```
  * 2. Install the SELinux `devel` package on the Worker nodes: `# dnf install selinux-policy-devel`
  * 3. Create a file `rac-ocne.te` under `/var/opt` on the Worker nodes with the following:
    ```sh
    module rac-ocne 1.0;
    
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
* 4. Create and install the policy package:
     * `# cd /var/opt`
     * `make -f /usr/share/selinux/devel/Makefile rac-ocne.pp`
     * `semodule -i rac-ocne.pp`
     * `semodule -l | grep rac-ocne`
* 5. Repeat steps 1 through 4 on all of the other worker nodes. 
* 6. Configure the worker node directory file context for the SELinux module:
     * On worker node:
        + qck-ocne19-w1 `# semanage fcontext -a -t container_file_t /scratch/rac/cluster01`
        + qck-ocne19-w1 `# sudo restorecon -vF /scratch/rac/cluster01`
        + qck-ocne19-w1 `# semanage fcontext -a -t container_file_t /scratch/software/stage`
        + qck-ocne19-w1 `# sudo restorecon -vF /scratch/software/stage`
        + qck-ocne19-w2 `# semanage fcontext -a -t container_file_t /scratch/rac/cluster01`
        + qck-ocne19-w2 `# sudo restorecon -vF /scratch/rac/cluster01`      

  **Notes:**
  * Change these paths as required for the location of your environment for deploying Oracle RAC.
  * If you want to deploy more worker nodes, you will need to repeat those commands for each of these worker nodes.


### Using CVU to Validate Readiness for Worker Node 
Oracle recommends that you use Cluster Verification Utility (CVU) for Container hosts on your Worker Node to help to ensure that the Worker Node is configured correctly.

You can use CVU to assist you with system checks in preparation for creating a container for Oracle Real Application Clusters (Oracle RAC), and installing Oracle RAC inside the containers. CVU runs the appropriate system checks automatically, and prompts you to fix problems that it detects. To obtain the benefits of system checks, you must run the utility on all the Worker Nodes that you want to configure to host the Oracle RAC containers.

For details, see: [Use the latest Cluster Verification Utility (CVU) (Doc ID 2731675.1)](https://support.oracle.com/rs?type=doc&id=2731675.1)

## Create a namespace for the Oracle RAC Setup

  Create a Kubernetes namespace named `rac`. All the resources belonging to the Oracle RAC Database will be provisioned in this namespace named `rac`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns rac

  #### Check the created namespace 
  kubectl get rac
  ```
You can choose any name for the namespace to deploy Oracle RAC Database.

## Complete the prerequisites before installing Oracle Database Operator

Before using Oracle RAC Database Controller, ensure you have completed the prerequisites which includes:

* The installation of cert-manager
* Creation of Role Bindings for Access Management. 

Refer to the section [Prerequisites](../../../README.md#prerequisites).

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

## Deploy Oracle Database Operator

After you have completed the prerequisite steps, you can install the operator. To install the operator in the cluster quickly, you can apply the modified `oracle-database-operator.yaml` file from the previous step.
```
kubectl apply -f oracle-database-operator.yaml
```

For more details, please refer to [Install Oracle DB Operator](../../../README.md#install-oracle-db-operator)

## Oracle RAC Database Slim Image
Choose one of the following deployment options: 

  **Use Oracle-Supplied Container Images:**
   The Oracle RAC Database Controller uses Oracle RAC Database Slim Image to provision the Oracle RAC Database.

   You can also download the pre-built Oracle RAC Database Slim Image `phx.ocir.io/intsanjaysingh/db-repo/oracle/database:19.3.0-slim`. This image is functionally tested and evaluated with various use cases of Oracle RAC Database on an OCNE Kubernetes Cluster.

   You can either download this image and push it to your Container Images Repository, or, if your Kubernetes cluster can reach OCR, you can download this image directly from OCR.
   
   **OR**

  **Build your own Oracle RAC Database Slim Image:**
   You can build this image using instructions provided in below documentation:
   * [Building Oracle RAC Database Container Slim Image](https://github.com/oracle/docker-images/blob/main/OracleDatabase/RAC/OracleRealApplicationClusters/README.md#building-oracle-rac-database-container-slim-image)

After the image is ready, push it to your Container Images Repository, so that you can pull this image during Oracle RAC Database provisioning..

**Note**: In the Oracle RAC Database provisioning sample .yaml files, we are using this RAC Database slim image `phx.ocir.io/intsanjaysingh/db-repo/oracle/database:19.3.0-slim`.

## Create Kubernetes secrets for the Oracle RAC Database

Create Kubernetes secrets for the Oracle RAC Database deployed using Oracle RAC Controller. This includes a secret containing the Oracle Database Password and another secret containing an ssh key pair which will be used for setting up the ssh user equivalence.

Create a Kubernetes secret named `db-user-pass` in `rac` namespace using these steps: [Create Kubernetes Secret](./create_kubernetes_secret_for_db_user.md)

Create a Kubernete secret named `ssh-key-secret` in `rac` namespace using these steps: [Create Kubernetes Secret for SSH Key](./create_kubernetes_secret_for_ssh_setup.md)

After you have the above prerequsites completed, you can proceed to the next section for your environment to provision the Oracle RAC Database.

## Create required directories on worker nodes
Before deploying your Oracle RAC Database using RAC Controller, create directory paths on the worker nodes for each RAC Database node(a pod) as follows:
  + `<racHostSwLocation>/<racNodeName><n>` where `<racHostSwLocation>` is the RAC GI and RDBMS Software Home location on the worker nodes which will be mounted to the RAC Node(a pod)(e.g., `/scratch/rac/cluster01`), 
  + `<racNodeName>` is your RAC Node name prefix (e.g., racnode), and `<n>` is the node number (from 1 to nodeCount). 

For example, if your YAML has: 

    nodeCount: 2
    racHostSwLocation: /scratch/rac/cluster01
    racNodeName: racnode

then run the bellow command on all the worker nodes:    
```bash
for i in $(seq 1 2); do
  dir="/scratch/rac/cluster01/racnode$i"
  sudo mkdir -p "$dir"
  sudo chown -R 54321:54321 "$dir"
  sudo chcon -R -t container_file_t "$dir"
done
```
## Label the worker nodes
Oracle RAC Controller on Kubernetes uses node labels to determine which worker nodes are eligible to deploy RAC Nodes. These labels must match the values defined in your YAML file as:
```yaml
spec:
    # Worker node pool assignment for the RAC Cluster
    # Specify the label of Kubernetes Worker Nodes
    workerNodeSelector:
      raccluster: raccluster01                             # All worker nodes with this label are eligible for deployment of RAC Node
```

This selector means any node labeled with `raccluster=raccluster01` will be elligible for deployment of a RAC Database Node(Pod) by the RAC Controller.

Label each worker node intended to deploy the RAC Database Node(Pod):
```bash
kubectl label node qck-ocne19-w1 raccluster=raccluster01
kubectl label node qck-ocne19-w2 raccluster=raccluster01
```

Ensure the labels are present on the nodes:
```bash
kubectl get nodes --show-labels | grep raccluster
```

Expected output:
```
qck-ocne19-w1   ... raccluster=raccluster01
qck-ocne19-w2   ... raccluster=raccluster01
```

## ClusterRole and ClusterRoleBinding for Multus Access

To enable the operator to read Multus `NetworkAttachmentDefinition` objects (required for validating private interconnect IP ranges), apply the RBAC configuration provided in [`multus-rbac.yaml`](./../rbac/multus-rbac.yaml).

These permissions include the ability to read Network Attachment Definitions:

```sh
kubectl apply -f rbac/multus-rbac.yaml
```
## ClusterRole and ClusterRoleBinding for PersistentVolume Access

The operator creates ASM Persistent Volumes (PV) at the cluster scope.  
To allow this, Kubernetes requires cluster-level permissions for managing `PersistentVolume` resources.

Apply the RBAC configuration provided in [`pv-rbac.yaml`](./../rbac/pv-rbac.yaml):

```sh
kubectl apply -f rbac/pv-rbac.yaml
```