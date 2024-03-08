# Oracle Database Operator for Kubernetes

## Make Oracle Database Kubernetes Native

As part of Oracle's resolution to make Oracle Database Kubernetes native (that is, observable and operable by Kubernetes), Oracle released _Oracle Database Operator for Kubernetes_ (`OraOperator` or the operator). OraOperator extends the Kubernetes API with custom resources and controllers for automating Oracle Database lifecycle management.

In this v1.0.0 production release, `OraOperator` supports the following database configurations and infrastructure:

* Oracle Autonomous Database:
  * Oracle Autonomous Database shared Oracle Cloud Infrastructure (OCI) (ADB-S)
  * Oracle Autonomous Database on dedicated Cloud infrastructure (ADB-D)
  * Oracle Autonomous Container Database (ACD) (infrastructure) the infrastructure for provisioning Autonomous Databases.
* Containerized Single Instance databases (SIDB) deployed in the Oracle Kubernetes Engine (OKE) and any k8s where OraOperator is deployed
* Containerized Sharded databases (SHARDED) deployed in OKE and any k8s where OraOperator is deployed
* Oracle Multitenant Databases (CDB/PDBs)
* Oracle Base Database Cloud Service (BDBCS)
* Oracle Data Guard (Preview status)
* Oracle Database Metrics Exporter (Preview status)

Oracle will continue to extend `OraOperator` to support additional Oracle Database configurations.

## Features Summary

This release of Oracle Database Operator for Kubernetes (the operator) supports the following lifecycle operations:

* ADB-S/ADB-D: Provision, bind, start, stop, terminate (soft/hard), scale (up/down), on-demand backup, manual restore
* ACD: provision, bind, restart, terminate (soft/hard)
* SIDB: Provision, clone, patch (in-place/out-of-place), update database initialization parameters, update database configuration (Flashback, archiving), Oracle Enterprise Manager (EM) Express (a basic observability console), Oracle REST Data Service (ORDS) to support REST based SQL, PDB management, SQL Developer Web, and Application Express (Apex)
* SHARDED: Provision/deploy sharded databases and the shard topology, Add a new shard, Delete an existing shard
* Oracle Multitenant Database: Bind to a CDB, Create a  PDB, Plug a  PDB, Unplug a PDB, Delete a PDB, Clone a PDB, Open/Close a PDB
* Oracle Base Database Cloud Service (BDBCS): provision, bind, scale shape Up/Down, Scale Storage Up, Terminate and Update License
* Oracle Data Guard: Provision a Standby for the SIDB resource, Create a Data Guard Configuration, Perform a Switchover, Patch Primary and Standby databases in Data Guard Configuration
* Oracle Database Metrics Exporter: create, patch, delete
* Watch over a set of namespaces or all the namespaces in the cluster using the "WATCH_NAMESPACE" env variable of the operator deployment

The upcoming releases will support new configurations, operations and capabilities.

## Release Status

This production release has been installed and tested on the following Kubernetes platforms:

* [Oracle Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) with Kubernetes 1.24
* [Oracle Linux Cloud Native Environment(OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) 1.6
* [Minikube](https://minikube.sigs.k8s.io/docs/) with version v1.29.0
* [Azure Kubernetes Service](https://azure.microsoft.com/en-us/services/kubernetes-service/) 
* [Amazon Elastic Kubernetes Service](https://aws.amazon.com/eks/)
* [Red Hat OKD](https://www.okd.io/)
* [Red Hat OpenShift](https://www.redhat.com/en/technologies/cloud-computing/openshift/)

## Prerequisites

Oracle strongly recommends that you ensure your system meets the following [Prerequisites](./PREREQUISITES.md).

* ### Install cert-manager

  The operator uses webhooks for validating user input before persisting it in etcd. Webhooks require TLS certificates that are generated and managed by a certificate manager.

  Install the certificate manager with the following command:

  ```sh
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
  ```

* ### Role Binding for access management

  When the Oracle DB Operator is dealing with multiple namespaces and access management is required, you need to complete additional steps:
  - To have the Oralce DB Operator monitor only certain defined namespaces which you want it to monitor.
  - To apply role binding for those namespaces for access control.

  Follow the below steps:

  1. Download the [oracle-database-operator.yaml](https://github.com/oracle/oracle-database-operator/blob/main/oracle-database-operator.yaml) file.
  2. Add the comma separated namespaces under `WATCH_NAMESPACE`. For example:
     ```sh
     - name: WATCH_NAMESPACE
       value: "oracle-database-operator-system,shns"
     ```
     This is needed when you want the DB Operator to monitor the namespaces `shns` and `oracle-database-operator-system`.

     If you are going to work with more namespaces, then you will need to add them under `WATCH_NAMESPACE`. 

     Save the `oracle-database-operator-system.yaml` file after this change.
  3. Create a role binding file for the namespace you are going to work in. 

     In this case, its the namespace `shns` and you need to create a file like below for it:

     ```sh
     ---
     apiVersion: rbac.authorization.k8s.io/v1
     kind: RoleBinding
     metadata:
       name: oracle-database-operator-oracle-database-operator-manager-rolebinding1
       namespace: shns
     roleRef:
       apiGroup: rbac.authorization.k8s.io
       kind: ClusterRole
       name: oracle-database-operator-manager-role
     subjects:
     - kind: ServiceAccount
       name: default
       namespace: oracle-database-operator-system
     ```

     If you are testing with multiple namespaces, make sure you create that many binding files and apply them before applying `oracle-database-operator.yaml`. 

     NOTE: You need to change the namespace in that binding file from `shns` and also change the binding name `oracle-database-operator-oracle-database-operator-manager-rolebinding1` to a new binding name.

  4. Apply the role binding file like below:
     ```sh
     kubectl apply -f shns_binding.yaml
     ```

## Install Oracle DB Operator

   Once the above prerequisite changes have been done, to install the operator in the cluster quickly, you can apply the modified `oracle-database-operator.yaml` file from above step.

   Run the following command

   ```sh
   kubectl apply -f oracle-database-operator.yaml
   ```

  Ensure that the operator pods are up and running. For high availability, Operator pod replicas are set to a default of 3. You can scale this setting up or down.

  ```sh
  $ kubectl get pods -n oracle-database-operator-system
  
    NAME                                                                 READY   STATUS    RESTARTS   AGE
    pod/oracle-database-operator-controller-manager-78666fdddb-s4xcm     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-5k6n4     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-t6bzb     1/1     Running   0          11d

  ```

* Check the resources

You should see that the operator is up and running, along with the shipped controllers.

For more details, see [Oracle Database Operator Installation Instructions](./docs/installation/OPERATOR_INSTALLATION_README.md).

## Getting Started with the Operator (Quickstart)

The quickstarts are designed for specific database configurations:

* [Oracle Autonomous Database](./docs/adb/README.md)
* [Oracle Autonomous Container Database](./docs/adb/ACD.md)
* [Containerized Oracle Single Instance Database and Data Guard](./docs/sidb/README.md)
* [Containerized Oracle Sharded Database](./docs/sharding/README.md)
* [Oracle Multitenant Database](./docs/multitenant/README.md)
* [Oracle Base Database Cloud Service (BDBCS)](./docs/dbcs/README.md)


The quickstarts are designed for non-database configurations:
* [Oracle Database Metrics Exporter](./docs/observability/README.md)

YAML file templates are available under [`/config/samples`](./config/samples/). You can copy and edit these template files to configure them for your use cases.

## Uninstall the Operator

  To uninstall the operator, the final step consists of deciding whether you want to delete the custom resource definitions (CRDs) and Kubernetes APIServices introduced into the cluster by the operator. Choose one of the following options:

* ### Deleting the CRDs and APIServices

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

  After all CRD instances are deleted, it is safe to remove the CRDs, APIServices and operator deployment. To remove these files, use the following command:

  ```sh
  kubectl delete -f oracle-database-operator.yaml --ignore-not-found=true
  ```

  Note: If the CRD instances are not deleted, and the operator is deleted by using the preceding command, then operator deployment and instance objects (pods, services, PVCs, and so on) are deleted. However, if that happens, then the CRD deletion stops responding. This is because the CRD instances have properties that prevent their deletion, and that can only be removed by the operator pod, which is deleted when the APIServices are deleted.

## Docs of the supported Oracle Database configurations

* [Oracle Autonomous Database](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adboverview.htm)
* [Components of Dedicated Autonomous Database](https://docs.oracle.com/en-us/iaas/autonomous-database/doc/components.html)
* [Oracle Database Single Instance](https://docs.oracle.com/en/database/oracle/oracle-database/)
* [Oracle Database Sharding](https://docs.oracle.com/en/database/oracle/oracle-database/21/shard/index.html)
* [Oracle Database Cloud Service](https://docs.oracle.com/en/database/database-cloud-services.html)

## Contributing

See [Contributing to this Repository](./CONTRIBUTING.md)

## Support

You can submit a GitHub issue, and/or you file an [Oracle Support service](https://support.oracle.com/portal/) request, using this product ID: 14430.

## Security

Secure platforms are an important basis for general system security. Ensure that your deployment is in compliance with common security practices.

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

Copyright (c) 2022, 2023 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at [https://oss.oracle.com/licenses/upl/](https://oss.oracle.com/licenses/upl/)
