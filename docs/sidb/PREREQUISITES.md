## Prerequisites for Oracle Docker Image Deployment
To deploy Oracle Database Operator for Kubernetes on Oracle Docker images, complete these steps. 

* ### Prepare Oracle Docker Images

  Build Single Instance Database Docker Images from source, following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance), or
  use the pre-built images available at [https://container-registry.oracle.com](https://container-registry.oracle.com) by signing in and accepting the required license agreement.

  Oracle Database Releases Supported: Enterprise and Standard Edition for Oracle Database 19c, and later releases. Express Edition for Oracle Database 21.3.0  only. Oracle Database Free 23.2.0 and later Free releases
  
  Build Oracle REST Data Service Docker Images from source following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).     
  Supported Oracle REST Data Service version is 21.4.2

* ### Set Up Kubernetes and Volumes

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes. Use a dynamic volume provisioner or pre-provision static persistent volumes manually. These volumes are required for persistent storage of the database files.

  More info on creating persistent volumes available at [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

* ### Minikube Cluster Environment
  
  By default, Minikube creates a node with 2GB RAM, 2 CPUs, and 20GB disk space when a cluster is created using `minikube start` command. However, these resources (particularly disk space and RAM) may not be sufficient for running and managing Oracle Database using the OraOperator. It is recommended to have larger RAM and disk space for better performance. For example, the following command creates a Minikube cluster with 8GB RAM and 100GB disk space for the Minikube VM:
  
  ```
  minikube start --memory=8g --disk-size=100g
  ```

