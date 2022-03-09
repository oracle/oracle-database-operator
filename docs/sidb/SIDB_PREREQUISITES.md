## Prerequisites for Oracle Docker Image Deployment
To deploy Oracle Database Operator for Kubernetes on Oracle Docker images, complete these steps. 

* ### Prepare Oracle Docker Images

  Build SingleInstanceDatabase Docker Images from source, following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance), or
  use the pre-built images available at [https://container-registry.oracle.com](https://container-registry.oracle.com) by signing in and accepting the required license agreement.

  Oracle Database Releases Supported: Oracle Database 19c Enterprise Edition or Standard Edition, and later releases.
  
  Prepare the pre-built database docker image by following the [instructions](https://github.com/oracle/docker-images/blob/main/OracleDatabase/SingleInstance/extensions/prebuiltdb/README.md)

  Build OracleRestDataService Docker Images from source following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).
  OracleRestDataService version 20.4.1 onwards are supported.

* ### Set Up Kubernetes and Volumes

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes, configured with persistent volumes. The persistent volumes are required for storage of the database files.

  More info on creating persistent volumes available at [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

  More info on creating persistent volumes on OCI is available at [https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengcreatingpersistentvolumeclaim.htm](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengcreatingpersistentvolumeclaim.htm)
