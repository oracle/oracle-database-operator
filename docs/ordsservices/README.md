# Oracle Rest Data Services (ORDSSRVS) Controller for Kubernetes -  ORDS Life cycle management


## Description

The ORDSRVS controller extends the Kubernetes API with a Custom Resource (CR) and Controller for automating Oracle Rest Data
Services (ORDS) lifecycle management.  Using the ORDS controller, you can easily migrate existing, or create new, ORDS implementations
into an existing Kubernetes cluster.  

This controller allows you to run what would otherwise be an On-Premises ORDS middle-tier, configured as you require, inside Kubernetes with the additional ability of the controller to perform automatic ORDS/APEX install/upgrades inside the database.

## Features Summary

The custom RestDataServices resource supports the following configurations as a Deployment, StatefulSet, or DaemonSet:

* Single OrdsSrvs resource with one database pool
* Single OrdsSrvs resource with multiple database pools<sup>*</sup>
* Multiple OrdsSrvs resources, each with one database pool
* Multiple OrdsSrvs resources, each with multiple database pools<sup>*</sup>

<sup>*See [Limitations](#limitations)</sup>

It supports the majority of ORDS configuration settings as per the [API Documentation](./api.md).

The ORDS and APEX schemas can be [automatically installed/upgraded](./autoupgrade.md) into the Oracle Database by the ORDS controller.

ORDS Version support: 
* 24.1.1  
(Newer versions of ORDS will be supported in the next update of OraOperator)

Oracle Database Version: 
* 19c
* 23ai (incl. 23ai Free)

### Prerequisites

1. Oracle Database Operator  

    Install the Oracle Database Operator (OraOperator) using the instructions in the [README](https://github.com/oracle/oracle-database-operator/blob/main/README.md) file.

1. Namespace  

    For a dedicated namespace deployment of the ORDSSRVS controller, refer to the "Namespace Scoped Deployment" section in the OraOperator [README](https://github.com/oracle/oracle-database-operator/blob/main/README.md#2-namespace-scoped-deployment).

    The following examples deploy the controller to the 'ordsnamespace' namespace.

    Create the namespace:
    ```bash
    kubectl create namespace ordsnamespace
    ```

    Apply namespace role binding [ordsnamespace-role-binding.yaml](./examples/ordsnamespace-role-binding.yaml):
    ```bash
    kubectl apply -f ordsnamespace-role-binding.yaml
    ```

    Edit OraOperator to add the namespace under WATCH_NAMESPACE:
    ```yaml
    - name: WATCH_NAMESPACE
    value: "default,<your namespaces>,ordsnamespace"
    ```

### Common configuration examples

A few common configuration examples can be used to quickly familiarise yourself with the ORDS Custom Resource Definition.
The "Conclusion" section of each example highlights specific settings to enable functionality that maybe of interest.

Before 

* [Pre-existing Database](./examples/existing_db.md)
* [Containerised Single Instance Database (SIDB)](./examples/sidb_container.md)
* [Multidatabase using a TNS Names file](./examples/multi_pool.md)
* [Autonomous Database using the OraOperator](./examples/adb_oraoper.md) <sup>*See [Limitations](#limitations)</sup>
* [Autonomous Database without the OraOperator](./examples/adb.md)
* [Oracle API for MongoDB Support](./examples/mongo_api.md)

Running through all examples in the same Kubernetes cluster illustrates the ability to run multiple ORDS instances with a variety of different configurations.

If you have a specific use-case that is not covered and would like it to be feel free to contribute it via a Pull Request.

### Limitations

When connecting to a mTLS enabled ADB and using the Oracontroller to retreive the Wallet, it is currently not supported to have multiple, different databases supported by the single RestDataServices resource.  This is due to a requirement to set the `TNS_ADMIN` parameter at the Pod level ([#97](https://github.com/oracle/oracle-database-controller/issues/97)).

### Troubleshooting 
See [Troubleshooting](./TROUBLESHOOTING.md)

## Contributing
See [Contributing to this Repository](./CONTRIBUTING.md)

## Reporting a Security Issue

See [Reporting security vulnerabilities](./SECURITY.md)

## License

Copyright (c) 2025 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at [https://oss.oracle.com/licenses/upl/](https://oss.oracle.com/licenses/upl/)
