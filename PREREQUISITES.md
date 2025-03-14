#

## Prerequisites for Using Oracle Database Operator for Kubernetes

Oracle Database Operator for Kubernetes (`OraOperator`) manages all Cloud deployments of Oracle Database, including:

* Oracle Autonomous Database (ADB)
* Containerized Oracle Database Single Instance (SIDB)
* Containerized Oracle Globally Distributed Database (GDD)

### Setting Up a Kubernetes Cluster and Volumes
Review and complete each step as needed.

#### Setting Up an OKE Cluster on Oracle Cloud Infrastructure (OCI)

To set up a Kubernetes cluster on Oracle Cloud Infrastructure:

1. Log in to OCI
1. Create an OKE Cluster
1. Provision persistent storage for data files (NFS or Block)

Note: If you intend to deploy containerized databases over the OKE cluster, then you must provision persistent storage.

### Prerequites for Oracle Autonomous Database (ADB)

If you intend to use `OraOperator` to handle Oracle Autonomous Database lifecycles, then read [Oracle Autonomous Database prerequisites](./docs/adb/ADB_PREREQUISITES.md)

### Prerequites for Single Instance Databases (SIDB)

If you intend to use `OraOperator` to handle Oracle Database Single Instance lifecycles, then read [Single Instance Database Prerequisites](./docs/sidb/PREREQUISITES.md)

### Prerequites for Oracle Globally Distributed Databases(GDD)

 If you intent to use OraOperator to handle the lifecycle of Oracle Globally Distributed Database(GDD), then read [Oracle Globally Distributed Database Prerequisites](./docs/sharding/README.md#prerequsites-for-running-oracle-sharding-database-controller)
