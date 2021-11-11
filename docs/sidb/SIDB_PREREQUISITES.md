## Prerequisites for Oracle Docker Image Deployment
To deploy Oracle Database Operator for Kubernetes on Oracle Docker images, complete these steps. 

* ### Prepare Oracle Docker Images

  Build SingleInstanceDatabase Docker Images from source, following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance), or
  use the pre-built images available at [https://container-registry.oracle.com](https://container-registry.oracle.com)

  Oracle Database Releases Supported: Oracle Database 19c Enterprise Edition or Standard Edition, and later releases.
  
  Build OracleRestDataService Docker Images from source following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).
  Add 'database.api.enabled=true' entry in 'ords_params.properties.tmpl' file while building OracleRestDataService image .
  OracleRestDataService version 20.4.1 onwards are supported.

* ### Set Up Kubernetes and Volumes

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes, configured with persistent volumes. The persistent volumes are required for storage of the database files.

  More info on creating persistent volumes available at [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
