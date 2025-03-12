## Deployment Prerequisites
To deploy Oracle Single Instance Database in Kubernetes using the OraOperator, complete these steps. 

* ### Prepare Oracle Container Images

  You can either build Single Instance Database Container Images from the source, following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance), or you can use the the pre-built images available at [https://container-registry.oracle.com](https://container-registry.oracle.com) by signing in and accepting the required license agreement.

  Oracle Database Releases Supported: Enterprise and Standard Edition for Oracle Database 19c, and later releases. Express Edition for Oracle Database 21.3.0 only. Oracle Database Free 23.2.0 and later Free releases
  
  Build Oracle REST Data Service Container Images from source following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).     
  The supported Oracle REST Data Service version is 21.4.2

* ### Ensure Sufficient Disk Space in Kubernetes Worker Nodes 

  Provision Kubernetes worker nodes. Oracle recommends you provision them with 250 GB or more free disk space, which is required for pulling the base and patched database container images. If you are doing a Cloud deployment, then you can choose to increase the custom boot volume size of the worker nodes. 

* ### Set Up Kubernetes and Volumes for Database Persistence

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes. Use a dynamic volume provisioner or pre-provision static persistent volumes manually. These volumes are required for persistent storage of the database files.

  For more more information about creating persistent volumes, see: [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

* ### Minikube Cluster Environment
  
  By default, when you create a cluster using the `minicube start` command, Minikube creates a node with 2GB RAM, 2 CPUs, and 20GB disk space. However, these resources (particularly disk space and RAM) may not be sufficient for running and managing Oracle Database using the OraOperator. For better performance, Oracle recommends that you configure the cluster to have a larger RAM and disk space than the Minikube default. For example, the following command creates a Minikube cluster with 8GB RAM and 100GB disk space for the Minikube VM:
  
  ```
  minikube start --memory=8g --disk-size=100g
  ```

