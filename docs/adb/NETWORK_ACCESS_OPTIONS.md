# Configuring Network Access for Oracle Autonomous Database

To configure network access for Oracle Autonomous Database (Autonomous Database), review and complete the procedures in this document. 

Network access for Autonomous Database includes public access, and configuring secure access, either over public networks using access control rules (ACLs), or by using using private endpoints inside a Virtual Cloud Network (VCN) in your tenancy. This document also describes procedures to configure the Transport Layer Security (TLS) connections, with the option either to require mutual TLS only, or to allow both one-way TLS and mutual TLS. 

For more information about these options, see: [Configuring Network Access with Access Control Rules (ACLs) and Private Endpoints ](https://docs.oracle.com/en/cloud/paas/autonomous-database/adbsa/autonomous-network-access.html#GUID-D2D468C3-CA2D-411E-92BC-E122F795A413).

## Supported Features
Review the following options available to you with Autonomous Database.

* [Configuring Network Access with Allowing Secure Access from Anywhere](#configuring-network-access-with-allowing-secure-access-from-anywhere) on shared Exadata infrastructure
* [Configuring Network Access with Access Control Rules (ACLs)](#configuring-network-access-with-access-control-rules-acls) on shared Exadata infrastructure
* [Configure Network Access with Private Endpoint Access Only](#configure-network-access-with-private-endpoint-access-only) on shared Exadata infrastructure
* [Allowing TLS or Require Only Mutual TLS (mTLS) Authentication](#allowing-tls-or-require-only-mutual-tls-mtls-authentication) on shared Exadata infrastructure
* [Autonomous Database with access control list enabled](#autonomous-database-with-access-control-list-enabled-on-dedicated-exadata-infrastructure) on dedicated Exadata infrastructure

## Configuring Network Access with Allowing Secure Access from Anywhere

Before changing the Network Access to Allowing Secure Access from Anywhere, ensure that your network security protocol requries only mTLS (Mutual TLS) authentication. For more details, see: [Allow both TLS and mutual TLS (mTLS) authentication](#allow-both-tls-and-mutual-tls-mtls-authentication). If mTLS enforcement is already enabled on your Autonomous Database, you can skip this step.

To specify that Autonomous Database can be connected from any location with a valid credential, complete one of the following procedures based on your network access configuration.

### Option 1 - Change the Network Access from "Secure Access from Allowed IPs and VCNs Only" to "Allowing Secure Access from Anywhere"
1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description |
    |----|----|----|
    | `whitelistedIps` | []string | The client IP access control list (ACL). This feature is available for Autonomous Databases on [shared Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adboverview.htm#AEI) and on Exadata Cloud@Customer.<br> Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br><br>For shared Exadata infrastructure, this is an array of CIDR (Classless Inter-Domain Routing) notations for a subnet or VCN OCID.<br>Use a semicolon (;) as a deliminator between the VCN-specific subnets or IPs.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br><br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR (Classless Inter-Domain Routing) notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br><br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry. |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        whitelistedIps:
        -
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

### Option 2 - Change the Network Access from "Private Endpoint Access Only" to "Allowing Secure Access from Anywhere"

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description |
    |----|----|----|
    | `privateEndpointLabel` | string | The hostname prefix for the resource. |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        privateEndpointLabel: ""
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Configuring Network Access with Access Control Rules (ACLs)

To configure Network Access with ACLs, complete this procedure.


1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description |
    |----|----|----|
    | `whitelistedIps` | []string | The client IP access control list (ACL). This feature is available for Autonomous Databases on [shared Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adboverview.htm#AEI) and on Exadata Cloud@Customer.<br> Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br><br>For shared Exadata infrastructure, this is an array of CIDR (Classless Inter-Domain Routing) notations for a subnet or VCN OCID.<br>Use a semicolon (;) as a deliminator between the VCN-specific subnets or IPs.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br><br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR (Classless Inter-Domain Routing) notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br><br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry. |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
          # Restrict access by defining access control rules in an Access Control List (ACL).
          whitelistedIps:
          - 1.1.1.1
          - 1.1.0.0/16
          - ocid1.vcn...
          - ocid1.vcn...;1.1.1.1
          - ocid1.vcn...;1.1.0.0/16
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured

## Configure Network Access with Private Endpoint Access Only

To change the Network Access to Private Endpoint Access Only, complete this procedure

1. Visit [Overview of VCNs and Subnets](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/managingVCNs_topic-Overview_of_VCNs_and_Subnets.htm#console) and [Network Security Groups](https://docs.oracle.com/en-us/iaas/Content/Network/Concepts/networksecuritygroups.htm#working) to see how to create VCNs, subnets, and network security groups (NSGs) if you haven't created them yet. The subnet and the NSG has to be in the same VCN.

2. Copy and paste the OCIDs of the subnet and NSG to the corresponding parameters. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `subnetId` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the subnet the resource is associated with.<br><br> **Subnet Restrictions:**<br> - For bare metal DB systems and for single node virtual machine DB systems, do not use a subnet that overlaps with 192.168.16.16/28.<br> - For Exadata and virtual machine 2-node RAC systems, do not use a subnet that overlaps with 192.168.128.0/20.<br> - For Autonomous Database, setting this will disable public secure access to the database.<br> These subnets are used by the Oracle Clusterware private interconnect on the database instance.<br> Specifying an overlapping subnet will cause the private interconnect to malfunction.<br> This restriction applies to both the client subnet and the backup subnet. | Yes |
    | `nsgIds` | string[] | The list of [OCIDs](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) for the network security groups (NSGs) to which this resource belongs. Setting this to an empty list removes all resources from all NSGs. For more information about NSGs, see [Security Rule](https://docs.cloud.oracle.com/Content/Network/Concepts/securityrules.htm).<br><br>    **NsgIds restrictions:**<br> - A network security group (NSG) is optional for Autonomous Databases with private access. The nsgIds list can be empty. | No |
    | `privateEndpointLabel` | string | The resource's private endpoint label.<br> - Setting the endpoint label to a non-empty string creates a private endpoint database.<br> - Resetting the endpoint label to an empty string, after the creation of the private endpoint database, changes the private endpoint database to a public endpoint database.<br> - Setting the endpoint label to a non-empty string value, updates to a new private endpoint database, when the database is disabled and re-enabled.<br>This setting cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMTLSConnectionRequired, dbWorkload, dbVersion, dbName, or isFreeTier. | No |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        subnetId: ocid1.subnet...
        nsgIds:
        - ocid1.networksecuritygroup...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

## Allowing TLS or Require Only Mutual TLS (mTLS) Authentication

### Require mutual TLS (mTLS) authentication and Disallow TLS Authentication

To configure your Autonomous Database instance to require mTLS connections and disallow TLS connections, complete this procedure.

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_mtls.yaml`](./../../config/samples/adb/autonomousdatabase_update_mtls.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `isMtlsConnectionRequired` | boolean| Indicates whether the Autonomous Database requires mTLS connections. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        isMtlsConnectionRequired: true
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_mtls.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

### Allow both TLS and mutual TLS (mTLS) authentication

If your Autonomous Database instance is configured to allow only mTLS connections, then you can reconfigure the instance to permit both mTLS and TLS connections. When you reconfigure the instance to permit both mTLS and TLS, you can use both authentication types at the same time, so that connections are no longer restricted to require mTLS authentication.

This option only applies to Autonomous Databases on shared Exadata infrastructure. You can permit TLS connections when network access type is configured by using one of the following options:

* **Access Control Rules (ACLs)**: with ACLs defined.
* **Private Endpoint Access Only**: with a private endpoint defined.

Complete this procedure to allow both TLS and mTLS authentication.

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_mtls.yaml`](./../../config/samples/adb/autonomousdatabase_update_mtls.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `isMtlsConnectionRequired` | boolean| Indicates whether the Autonomous Database requires mTLS connections. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        isMtlsConnectionRequired: false
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_mtls.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Autonomous Database with access control list enabled on dedicated Exadata infrastructure

To configure the network access of Autonomous Database with access control list (ACL) on dedicated Exadata infrastructure, complete this procedure.

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `isAccessControlEnabled` | boolean | Indicates if the database-level access control is enabled.<br><br>If disabled, then database access is defined by the network security rules.<br><br>If enabled, then database access is restricted to the IP addresses defined by the rules specified with the `accessControlList` property. While specifying `accessControlList` rules is optional, if database-level access control is enabled, and no rules are specified, then the database will become inaccessible. The rules can be added later by using the `UpdateAutonomousDatabase` API operation, or by using the edit option in console.<br><br>When creating a database clone, you should specify the access control setting that you want the clone database to use. By default, database-level access control will be disabled for the clone.<br>This property is applicable only to Autonomous Databases on the Exadata Cloud@Customer platform. | Yes |
    | `accessControlList` | []string | The client IP access control list (ACL). This feature is available for autonomous databases on [shared Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adboverview.htm#AEI) and on Exadata Cloud@Customer.<br> Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br><br>For shared Exadata infrastructure, this is an array of CIDR (Classless Inter-Domain Routing) notations for a subnet or VCN OCID.<br>Use a semicolon (;) as a deliminator between the VCN-specific subnets or IPs.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br><br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR (Classless Inter-Domain Routing) notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br><br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        isAccessControlEnabled: true
        accessControlList:
        - 1.1.1.1
        - 1.1.0.0/16
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    $ kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
