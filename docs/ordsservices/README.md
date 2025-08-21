# Oracle Rest Data Services (OrdsSrvs) Controller for Kubernetes -  ORDS Life cycle management


## Description

The OrdsSrvs controller extends the Kubernetes API with a Custom Resource (CR) and Controller for automating Oracle Rest Data
Services (ORDS) lifecycle management.  Using the OrdsSrvs controller, you can easily migrate existing, or create new, ORDS implementations
into an existing Kubernetes cluster.  

This controller allows you to run what would otherwise be an On-Premises ORDS middle-tier, configured as you require, inside Kubernetes with the additional ability of the controller to perform automatic ORDS/APEX install/upgrades inside the database.

## Features Summary

The custom RestDataServices resource supports the following configurations as a Deployment, StatefulSet, or DaemonSet:

* Single OrdsSrvs resource with one database pool
* Single OrdsSrvs resource with multiple database pools<sup>*</sup>
* Multiple OrdsSrvs resources, each with one database pool
* Multiple OrdsSrvs resources, each with multiple database pools<sup>*</sup>
* ORDS and APEX database schemas [automatic installation/upgrade](./autoupgrade.md)

<sup>*See [Limitations](#limitations)</sup>

ORDS Version supported : 25.1.0+  
OrdsSrvs controller supports the majority of ORDS configuration settings as per the [API Documentation](./api.md).


### Prerequisites

 This chapter outlines the necessary requirements that must be satisfied to successfully deploy and operate the OrdsSrvs controller within your Kubernetes cluster.

#### Oracle Database Operator  

Before installing the OrdsSrvs controller, ensure that the Oracle Database Operator (OraOperator) is installed in your Kubernetes environment. Please follow the detailed installation steps provided in the [README](https://github.com/oracle/oracle-database-operator/blob/main/README.md) to complete this process. The OraOperator must be properly configured and running, as OrdsSrvs depends on its services for functionality.


#### Namespace Namespace Scoped Deployment

For a dedicated namespace deployment of the OrdsSrvs controller, refer to the "Namespace Scoped Deployment" section in the OraOperator [README](https://github.com/oracle/oracle-database-operator/blob/main/README.md#2-namespace-scoped-deployment).
The following examples demonstrate deploying the controller to the ordsnamespace namespace. 

Create the namespace:

```bash
kubectl create namespace ordsnamespace
```

Apply namespace role binding [ordsnamespace-role-binding.yaml](./ordsnamespace-role-binding.yaml):

```bash
kubectl apply -f ordsnamespace-role-binding.yaml
```

Edit OraOperator to add the namespace under WATCH_NAMESPACE:
```yaml
  - name: WATCH_NAMESPACE
    value: "default,<your namespaces>,ordsnamespace"
```

### OpenShift Security Context Constraints

If you are deploying the OrdsSrvs controller on OpenShift, ensure that the appropriate Security Context Constraints (SCCs) are configured. This involves assigning privileged SCCs to the service accounts used by OrdsSrvs to permit required operations.

#### Create a Service Account

This account will be used to assign the necessary Security Context Constraints (SCCs) for the controller’s operation.
Below is an example [YAML](./examples/ordssrvs-sa.yaml) manifest to create a service account named "ordssrvs-sa" in the "ordsnamespace" namespace:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ordssrvs-sa
  namespace: ordsnamespace
```

#### Create a Custom Security Context Constraint (SCC)

To configure the required security permissions, use the attached [YAML](./examples/ordssrvs-sa-scc.yaml) file to create a custom Security Context Constraint (SCC) and bind it to the "ordssrvs-sa" service account. 
This will ensure the service account has the necessary permissions for the OrdsSrvs controller to operate on OpenShift.

#### Set serviceAccountName in OrdsSrvs 

Ensure that the OrdsSrvs controller uses the dedicated service account you created. In the deployment manifest for OrdsSrvs, specify the serviceAccountName field with the name of your service account (e.g., ordssrvs-sa) as in this [example](./examples/ordssrvs.yaml).

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs
  namespace: ordsnamespace
spec:
  ...
  globalSettings:
    ...
  poolSettings:
    ...
  serviceAccountName: ordssrvs-sa
```


### Common configuration examples

A few common configuration examples can be used to quickly familiarise yourself with the OrdsSrvs Custom Resource Definition.
The "Conclusion" section of each example highlights specific settings to enable functionality that maybe of interest.

* [Pre-existing Database](./examples/existing_db.md)
* [Containerised Single Instance Database (SIDB)](./examples/sidb_container.md)
* [Multidatabase using a TNS Names file](./examples/multi_pool.md)
* [Autonomous Database using the OraOperator](./examples/adb_oraoper.md) <sup>*See [Limitations](#limitations)</sup>
* [Autonomous Database without the OraOperator](./examples/adb.md)
* [Oracle API for MongoDB Support](./examples/mongo_api.md)
* [ORDS and APEX database schemas automatic installation/upgrade](./autoupgrade.md)

Running through all examples in the same Kubernetes cluster illustrates the ability to run multiple ORDS instances with a variety of different configurations.

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
