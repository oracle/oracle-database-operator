# Oracle Database Operator for Kubernetes

## Make Oracle Database Kubernetes-Native

As part of Oracle's resolution to make Oracle Database Kubernetes-native (that is, observable and operable by Kubernetes), Oracle is announcing _Oracle Database Operator for Kubernetes_ (`OraOperator`). 

Since Oracle Database 19c, Oracle Database images have been supported in containers (Docker, Podman) for production use and Kubernetes deployment with Helm Charts. This release includes Oracle Database Operator, which is a new open source product that extends the Kubernetes API with custom resources and controllers for automating Oracle Database lifecycle management.

In this release, `OraOperator` supports the following Oracle Database configurations:

* Oracle Autonomous Database on shared Oracle Cloud Infrastructure (OCI), also known as ADB-S
* Containerized Single Instance databases (SIDB) deployed in the Oracle Kubernetes Engine (OKE)
* Containerized Sharded databases (SHARDED) deployed in OKE

Oracle will continue to expand Oracle Database Operator support for additional Oracle Database configurations.

## Features Summary

This release of Oracle Database Operator for Kubernetes (the operator) supports the following lifecycle operations:

* ADB-S: provision, bind, start, stop, terminate (soft/hard), scale (down/up)
* SIDB: provision, clone, patch (in-place/out-of-place), update database initialization parameters, update database configuration (Flashback, archiving), Oracle Enterprise Manager (EM) Express (a basic observability console), Oracle REST Data Service (ORDS) to support REST based SQL, PDB management, SQL Developer Web, and Application Express (Apex)
* SHARDED: provision/deploy sharded databases and the shard topology, add a new shard, delete an existing shard

Upcoming releases will support new configurations, operations and capabilities.

## Release Status

**CAUTION:** The current release of `OraOperator` (v0.1.0) is for development and test only. DO NOT USE IN PRODUCTION.

This release can be deployed on the following platforms:

* [Oracle Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) with Kubernetes 1.17 or later
* In an on-premises [Oracle Linux Cloud Native Environment(OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) 1.3 or later

In upcoming releases, the operator will be certified against third-party Kubernetes clusters.

## Prerequisites

Oracle strongly recommends that you ensure your system meets the following [Prerequisites](./PREREQUISITES.md).

* ### Install cert-manager

  The operator uses webhooks for validating user input before persisting it in Etcd. Webhooks require TLS certificates that are generated and managed by a certificate manager.

  Install the certificate manager with the following command:

  ```sh
  kubectl apply -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
  ```

## Quick Install of the Operator

  To install the operator in the cluster quickly, you can use a single [oracle-database-operator.yaml](https://github.com/oracle/oracle-database-operator/blob/main/oracle-database-operator.yaml) file. Operator pod replicas are set to a default of 3 for High Availability, which can be scaled up and down.

  Run the following command

  ```sh
  kubectl apply -f oracle-database-operator.yaml
  ```

  Ensure that operator pods are up and running

  ```sh
  $ kubectl get pods -n oracle-database-operator-system
  
    NAME                                                                 READY   STATUS    RESTARTS   AGE
    pod/oracle-database-operator-controller-manager-78666fdddb-s4xcm     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-5k6n4     1/1     Running   0          11d
    pod/oracle-database-operator-controller-manager-78666fdddb-t6bzb     1/1     Running   0          11d

  ```

* Check the resources

You should see that the operator is up and running, along with the shipped controllers.

For more details, see [Oracle Database Operator Installation Instrunctions](./docs/installation/OPERATOR_INSTALLATION_README.md).

## Getting Started with the Operator (Quickstart)

The quickstarts are designed for specific database configurations, including:

* [Oracle Autonomous Database](./docs/adb/README.md)
* [Oracle Database Single Instance configuration](./docs/sidb/README.md)
* [Oracle Database configured with Oracle Sharding](./docs/sharding/README.md)

YAML file templates are available under [`/config/samples`](./config/samples/). You can copy and edit these template files to configure them for your use cases. 

## Uninstall the Operator

  To uninstall the operator, the final step consists of deciding whether or not you want to delete the CRDs and APIServices that were introduced to the cluster by the operator. Choose one of the following options:

* ### Deleting the CRDs and APIServices

  To delete all the CRD instances deployed to cluster by the operator, run the following commands, where <namespace> is the namespace of the cluster object:

  ```sh
  kubectl delete singleinstancedatabase.database.oracle.com --all -n <namespace>
  kubectl delete shardingdatabase.database.oracle.com --all -n <namespace>
  kubectl delete autonomousdatabase.database.oracle.com --all -n <namespace>
  ```

  After all CRD instances are deleted, it is safe to remove the CRDs, APISerivces and operator deployment.

  ```sh
  kubectl delete -f oracle-database-operator.yaml --ignore-not-found=true
  ```

  Note: If the CRD instances are not deleted, and the operator is deleted by using the preceding command, then operator deployment and instance objects (pods,services,PVCs, and so on) are deleted. However, the CRD deletion stops responding, because the CRD instances have finalizers that can only be removed by the operator pod, which is deleted when the APIServices are deleted.

* ### Retaining the CRDs and APIservices

  To delete the operator deployment and retain the CRDs, run the following commands:

  ```sh
  kubectl delete deployment.apps/oracle-database-operator-controller-manager -n oracle-database-operator-system
  ```

## Documentation

* [Oracle Autonomous Database](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adboverview.htm)
* [Oracle Database Single Instance](https://docs.oracle.com/en/database/oracle/oracle-database/)
* [Oracle Database Sharding](https://docs.oracle.com/en/database/oracle/oracle-database/21/shard/index.html)

## Contributing

See [Contributing to this Repository](./CONTRIBUTING.md)

## Support

You can submit a GitHub issue, or you can also file an [Oracle Support service](https://support.oracle.com/portal/) request, using the product id: 14430.

## Security

Secure platforms are an important basis for general system security. Ensure that your deployment is in compliance with common security practices.

### Managing Sensitive Data
Kubernetes secrets are the usual means for storing credentials or passwords input for access. The operator reads the Secrets programmatically, which limits exposure of sensitive data. However, to protect your sensitive data, Oracle strongly recommends that you set and get sensitive data from Oracle Cloud Infrastructure Vault, or from third-party Vaults.

The following is an example of a YAML file fragment for specifying Oracle Cloud Infrastructure Vault as the repository for the admin password.
 ```
 adminPassword:
      ociSecretOCID: ocid1.vaultsecret.oc1...
```
Examples in this repository where passwords are entered on the command line are for demonstration purposes only. 

### Reporting a Security Issue

See [Reporting security vulnerabilities](./SECURITY.md)



## License

Copyright (c) 2021 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at [https://oss.oracle.com/licenses/upl/](https://oss.oracle.com/licenses/upl/)
