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
* v22.1+

Oracle Database Version: 
* 19c
* 23ai (incl. 23ai Free)


### Common Configurations

A few common configuration examples can be used to quickly familiarise yourself with the ORDS Custom Resource Definition.
The "Conclusion" section of each example highlights specific settings to enable functionality that maybe of interest.

* [Containerised Single Instance Database using the Oracontroller](./examples/sidb_container.md)
* [Multipool, Multidatabase using a TNS Names file](./examples/multi_pool.md)
* [Autonomous Database using the Oracontroller](./examples/adb_oraoper.md) - (Customer Managed ORDS) <sup>*See [Limitations](#limitations)</sup>
* [Autonomous Database without the Oracontroller](./examples/adb.md) - (Customer Managed ORDS)
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
