# Oracle Database Operator for Kubernetes

## Make Oracle Database Kubernetes Native

As part of Oracle's resolution to make Oracle Database Kubernetes native (that is, observable and operable by Kubernetes), Oracle released the  _Oracle Database Operator for Kubernetes_ (`OraOperator` or the operator). OraOperator extends the Kubernetes API with custom resources and controllers for automating the management of the Oracle Database lifecycle.

## What's New in v2.0.0

* **RedHat OpenShift Certification**
  * Validation of OraOperator and Controllers
  * Inclusion in RedHat Operators Catalog
* **Restart Controller**
  * Provision, add & delete asm disks, and more
* **ORDS Service**
  * ServiceAccount and OpenShift support
  * Auto download of APEX installation files and APEX image on a Persistent Volume
* **Integrations**
  * Private Cloud Appliance (PCA)
  * Compute Cloud@Customer (C3)
* **Bug fixes**
  * Bugs filed through Oracle Support
  * GitHub issues
 

## Supported Database Configurations v2.0.0
In this v2.0 production release, `OraOperator` supports the following database configurations and controllers:

* **Oracle Autonomous Database:**
  * Oracle Autonomous Database shared Oracle Cloud Infrastructure (OCI) (ADB-S)
  * Oracle Autonomous Database on dedicated Cloud infrastructure (ADB-D)
  * Oracle Autonomous Container Database (ACD), the infrastructure for provisioning Autonomous Databases
* **Containerized Single Instance databases (SIDB)** deployed in the Oracle Kubernetes Engine (OKE) and any Kubernetes where OraOperator is deployed
* **Containerized Oracle Globally Distributed Databases (GDD)** deployed in OKE and any Kubernetes where OraOperator is deployed
* **Oracle Multitenant Databases (CDB/PDBs)**
* **Oracle Base Database Service (OBDS)** on Oracle Cloud Infrastructure (OCI)
* **Oracle Data Guard**
* **Oracle Database Observability**
* **Oracle Database Rest Service (ORDS) instances**
* **Oracle Restart**
* **Oracle Globally Distributed Database**

---

## Lifecycle Features (Controllers Enhancements)
* **ORDS Services**
  - Install on SIDB and ADB
  - Provision and delete ORDS instances
* **SIDB**
  - Oracle Database 23ai Free support
  - Oracle Database 23ai Free-lite support
  - SIDB resource management
  - True Cache support for Free SIDB databases (Preview)
  - Observer for FastStartFailover with Data Guard
  - Snapshot Standby support in Data Guard setup
* **Globally Distributed Database**
  - Support for Oracle Database 23ai Raft replication
  - Oracle Database 23ai Free support
* **Autonomous Database**
  - Support for Database manual failover and switchover
* **Multitenant DB**
  - ORDS-based Controller: Assertive deletion policy
  - New LRES-based Controller (ARM & AM)
    - PDBs settings with init parameters config map
    - Assertive deletion policy
* **Database Observability **
  - Support for Database Logs and Metrics
  - Support for the latest Exporter container images
 
* **Oracle Base Database Service**
  - Support for Oracle Database 23ai Cloning, using KMS Vaults
  - PDB creation
  - Clone, Backup, Restore
  - Data Guard Setup
  - Patching and Upgrade
* **Oracle Restart**
  - Support for Oracle Database 19c

---

## Product Features
* Upgraded Kubernetes API version to v4
* Published on `operatorhub.io`
* Operator Lifecycle Manager (OLM) support (install from `operatorhub.io`)
* OpenShift certified

---

## Overall Features Summary
As of v2.0.0, the Oracle Database Operator for Kubernetes (`OraOperator`) supports the following lifecycle operations:

* **ADB-S/ADB-D**: Provision, bind, start, stop, terminate (soft/hard), scale (up/down), long-term backup, manual restore, cloning, manual failover, switchover
* **ACD**: Provision, bind, restart, terminate (soft/hard)
* **SIDB**: Provision, clone, patch (in-place/out-of-place), update database initialization parameters, update database configuration (Flashback, archiving), Oracle Enterprise Manager (EM) Express (basic console), Oracle REST Data Service (ORDS), PDB management, SQL Developer Web, Application Express (Apex), Resource management, True Cache, Observer for FastStartFailover (Data Guard), Snapshot Standby (Data Guard)
* **ORDS Services**: Provision and delete ORDS instances
* **Globally Distributed (Sharded)**: Provision/deploy sharded databases and topology, add/delete shards, Raft replication
* **Oracle Multitenant Database**: Bind to a CDB, create/plug/unplug/delete/clone/open/close PDBs, assertive deletion policy
* **Oracle Base Database Service (OBDS)**: Provision, scale shape/storage, terminate/update license, cloning, PDB creation, using KMS Vaults, backup/restore, Data Guard setup, patching/upgrade
* **Oracle Data Guard**: Provision standby for SIDB, create Data Guard configuration, perform switchover, patch primary/standby
* **Oracle Database Observability**: Create, patch, delete `databaseObserver` (logs and metrics)
* **Oracle Restart**: Provision, add & delete asm disks, enable ons ports, custom storage class support, existing pvc support, load balancer  support
* Watch namespaces using the `WATCH_NAMESPACE` environment variable


## Release Status

This production release has been installed and tested on:
* [Oracle Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) with Kubernetes 1.30 or later
* [Redhat Openshift](https://www.redhat.com/en/technologies/cloud-computing/openshift) with version v4.16 or later
* [Oracle Linux Cloud Native Environment(OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) 1.9 or later
* [Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/docs)
* [Azure Kubernetes Service](https://azure.microsoft.com/en-us/services/kubernetes-service/)
* [Amazon Elastic Kubernetes Service](https://aws.amazon.com/eks/)
* [Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/docs)
* [Red Hat OKD](https://www.okd.io/)
* [Minikube](https://minikube.sigs.k8s.io/docs/) with version v1.29.0


## Prerequisites

Oracle strongly recommends that you ensure your system meets the following [Prerequisites](./PREREQUISITES.md).

* ### Install cert-manager

  The operator uses webhooks for validating user input before persisting it in `etcd`. Webhooks require TLS certificates that are generated and managed by a certificate manager.

  Install the certificate manager with the following command:

  ```sh
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
  ```

* ### Create Role Bindings for Access Management

  OraOperator supports the following two modes of deployment:
  ##### 1. Cluster Scoped Deployment

    This is the default mode, in which OraOperator is deployed to operate in a cluster, and to monitor all the namespaces in the cluster.

  - Grant the `serviceaccount:oracle-database-operator-system:default` clusterwide access for the resources by applying [cluster-role-binding.yaml](./rbac/cluster-role-binding.yaml)

    ```sh
      kubectl apply -f rbac/cluster-role-binding.yaml
    ```

  - Next, apply the [oracle-database-operator.yaml](./oracle-database-operator.yaml) to deploy the Operator

    ```sh
      kubectl apply -f oracle-database-operator.yaml
    ```

  ##### 2. Namespace Scoped Deployment

   In this mode, `OraOperator` can be deployed to operate in a namespace, and to monitor one or many namespaces.

  - Grant `serviceaccount:oracle-database-operator-system:default` service account with resource access in the required namespaces. For example, to monitor only the default namespace, apply the [`default-ns-role-binding.yaml`](./rbac/default-ns-role-binding.yaml)

    ```sh
      kubectl apply -f rbac/default-ns-role-binding.yaml
    ```
    To watch additional namespaces, create different role binding files for each namespace, using [default-ns-role-binding.yaml](./rbac/default-ns-role-binding.yaml) as a template, and changing the `metadata.name` and `metadata.namespace` fields

  - Next, edit the [`oracle-database-operator.yaml`](./oracle-database-operator.yaml) to add the required namespaces under `WATCH_NAMESPACE`. Use comma-delimited values for multiple namespaces.

    ```sh
    - name: WATCH_NAMESPACE
      value: "default"
    ```
  - Finally, apply the edited [`oracle-database-operator.yaml`](./oracle-database-operator.yaml) to deploy the Operator

    ```sh
      kubectl apply -f oracle-database-operator.yaml
    ```

* ### ClusterRole and ClusterRoleBinding for NodePort services

  To expose services on each node's IP and port (the NodePort), apply the [`node-rbac.yaml`](./rbac/node-rbac.yaml). Note that this step is not required for LoadBalancer services.

  ```sh
    kubectl apply -f rbac/node-rbac.yaml
  ```

## Install Oracle DB Operator

  After you have completed the preceding prerequisite changes, you can install the operator using one of the following methods:

### Option 1: Install Using `oracle-database-operator.yaml`

  To install the operator in the cluster quickly, you can apply the modified `oracle-database-operator.yaml` file from the preceding step.

  Run the following command

  ```sh
  kubectl apply -f oracle-database-operator.yaml
  ```

  Ensure that the operator pods are up and running. For high availability, operator pod replicas are set to a default of 3. You can scale this setting up or down.

  ```sh
  $ kubectl get pods -n oracle-database-operator-system

    NAME                                                                 READY   STATUS    RESTARTS   AGE
    pod/oracle-database-operator-controller-manager-78666fdddb-s4xcm     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-5k6n4     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-t6bzb     1/1     Running   0          11d

  ```

### Option 2: Install via OperatorHub.io

  You can also install the Oracle DB Operator from [OperatorHub.io](https://operatorhub.io/operator/oracle-database-operator).

  1. Visit the [Oracle Database Operator](https://operatorhub.io/operator/oracle-database-operator) page on OperatorHub.io.

  2. Click the **Install** button to view and follow the step-by-step installation instructions for your Kubernetes environment.

### Check the resources

You should see that the operator is up and running, along with the shipped controllers.

For more details, see [Oracle Database Operator Installation Instructions](./docs/installation/OPERATOR_INSTALLATION_README.md).

## Getting Started with the Operator (Quickstart)

The following quickstarts are designed for specific database configurations:

* [Oracle Autonomous Database](./docs/adb/README.md)
* [Oracle Autonomous Container Database](./docs/adb/ACD.md)
* [Containerized Oracle Single Instance Database and Data Guard](./docs/sidb/README.md)
* [Containerized Oracle Globally Distributed Database](./docs/sharding/README.md)
* [Oracle Multitenant Database](./docs/multitenant/README.md)
* [Oracle Base Database Service (OBDS)](./docs/dbcs/README.md)
* [ORDS Services (ORDSSRVS)](./docs/ordsservices/README.md)
* [Oracle Real Application Cluster](./docs/rac/README.md)
* [Oracle Restart](./docs/oraclerestart/README.md)


The following quickstart is designed for non-database configurations:
* [Oracle Database Observability](./docs/observability/README.md)

## Examples
YAML file templates are available under [`/config/samples`](./config/samples/). You can copy and edit these template files to configure them for your use cases.

## Uninstall the Operator

  To uninstall the operator, the final step consists of deciding whether you want to delete the custom resource definitions (CRDs) and Kubernetes `APIServices` introduced into the cluster by the operator. Choose one of the following options:

* ### Delete the CRDs and APIServices

  To delete all the CRD instances deployed to cluster by the operator, run the following commands, where <namespace> is the namespace of the cluster object:

  ```sh
  kubectl delete oraclerestdataservice.database.oracle.com --all -n <namespace>
  kubectl delete singleinstancedatabase.database.oracle.com --all -n <namespace>
  kubectl delete shardingdatabase.database.oracle.com --all -n <namespace>
  kubectl delete dbcssystem.database.oracle.com --all -n <namespace>
  kubectl delete autonomousdatabase.database.oracle.com --all -n <namespace>
  kubectl delete autonomousdatabasebackup.database.oracle.com --all -n <namespace>
  kubectl delete autonomousdatabaserestore.database.oracle.com --all -n <namespace>
  kubectl delete autonomouscontainerdatabase.database.oracle.com --all -n <namespace>
  kubectl delete cdb.database.oracle.com --all -n <namespace>
  kubectl delete pdb.database.oracle.com --all -n <namespace>
  kubectl delete dataguardbrokers.database.oracle.com --all -n <namespace>
  kubectl delete databaseobserver.observability.oracle.com --all -n <namespace>
  ```

* ### Delete the RBACs

  ```sh
  cat rbac/* | kubectl delete -f -
  ```

* ### Delete the Deployment

  #### Option1: Delete `oracle-database-operator.yaml`

  Use this option if you install the operator using `oracle-database-operator.yaml`

  After all CRD instances are deleted, it is safe to remove the CRDs, APIServices and operator deployment. To remove these files, use the following command:

  ```sh
  kubectl delete -f oracle-database-operator.yaml --ignore-not-found=true
  ```

  Note: If the CRD instances are not deleted, and the operator is deleted by using the preceding command, then operator deployment and instance objects (pods, services, PVCs, and so on) are deleted. However, if that happens, then the CRD deletion stops responding. This is because the CRD instances have properties that prevent their deletion, and that can only be removed by the operator pod, which is deleted when the APIServices are deleted.

  #### Option2: Delete the Operator’s ClusterServiceVersion (CSV)

  Use this option if you install the operation from OperatorHub.io.

  First, identify the name of the installed operator’s ClusterServiceVersion (CSV) using the following command:

  ```sh
  kubectl clusterserviceversion -n operators
  ```

  Look for a CSV name similar to oracle-database-operator.vx.x.x.

  Once identified, delete the ClusterServiceVersion with the following command (replace the placeholder with the actual CSV name):

  ```sh
  kubectl delete clusterserviceversion oracle-database-operator.vx.x.x -n operators
  ```


## Documentation for the supported Oracle Database configurations

* [Oracle Autonomous Database](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adboverview.htm)
* [Components of Dedicated Autonomous Database](https://docs.oracle.com/en-us/iaas/autonomous-database/doc/components.html)
* [Oracle Database Single Instance](https://docs.oracle.com/en/database/oracle/oracle-database/)
* [Oracle Globally Distributed Database](https://docs.oracle.com/en/database/oracle/oracle-database/21/shard/index.html)
* [Oracle Database Cloud Service](https://docs.oracle.com/en/database/database-cloud-services.html)


## Contributing

This project welcomes contributions from the community. Before submitting a pull request, please [review our contribution guide](./CONTRIBUTING.md)

## Help

You can submit a GitHub issue, or submit an issue and then file an [Oracle Support service](https://support.oracle.com/portal/) request. To file an issue or a service request, use the following product ID: 14430.

## Security

Please consult the [security guide](./SECURITY.md) for our responsible security vulnerability disclosure process

### Managing Sensitive Data

Kubernetes secrets are the usual means for storing credentials or passwords input for access. The operator reads the Secrets programmatically, which limits exposure of sensitive data. However, to protect your sensitive data, Oracle strongly recommends that you set and get sensitive data from Oracle Cloud Infrastructure Vault, or from third-party Vaults.

The following is an example of a YAML file fragment for specifying Oracle Cloud Infrastructure Vault as the repository for the admin password.

```yaml
 adminPassword:
      ociSecretOCID: ocid1.vaultsecret.oc1...
```

Examples in this repository where passwords are entered on the command line are for demonstration purposes only.

### Reporting a Security Issue

See [Reporting security vulnerabilities](./SECURITY.md)

## License

Copyright (c) 2022, 2025 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at [https://oss.oracle.com/licenses/upl/](https://oss.oracle.com/licenses/upl/)
