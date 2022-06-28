# Configuring Network Access for Oracle Autonomous Database

To configure network access for Oracle Autonomous Database (Autonomous Database), review and complete the procedures in this document. 

Network access for Autonomous Database includes public access, and configuring secure access, either over public networks using access control rules (ACLs), or by using using private endpoints inside a Virtual Cloud Network (VCN) in your tenancy. This document also describes procedures to configure the Transport Layer Security (TLS) connections, with the option either to require mutual TLS only, or to allow both one-way TLS and mutual TLS. 

For more information about these options, see: [Configuring Network Access with Access Control Rules (ACLs) and Private Endpoints ](https://docs.oracle.com/en/cloud/paas/autonomous-database/adbsa/autonomous-network-access.html#GUID-D2D468C3-CA2D-411E-92BC-E122F795A413).

## Supported Features
Review the network access configuration options available to you with Autonomous Database. 

### Types of Network Access

There are three types of network access supported by Autonomous Database:

* **PUBLIC**

  The Public option permits secure access from anywhere. The network access type is PUBLIC if no option is specified in the specification. With this option, mutual TLS (mTLS) authentication is always required to connect to the database. This option is available only for databases on shared Exadata infrastructure.

* **RESTRICTED**

  The Restricted option permits connections to the database only as specified by the access control lists (ACLs) that you create. This option is available only for databases on shared Exadata infrastructure.
  
  You can add the following to your ACL:
  * **IP Address**: Specify one or more individual public IP addresses. Use commas to delimit your addresses in the input field.
  * **CIDR Block**: Specify one or more ranges of public IP addresses using CIDR notation. Use commas to separate your CIDR block entries in the input field.
  * **Virtual Cloud Network (OCID)** (applies to Autonomous Databases on shared Exadata infrastructure): Specify the Oracle Cloud Identifier (OCID) of a virtual cloud network (VCN). If you want to specify multiple IP addresses or CIDR ranges within the same VCN, then do not create multiple access control list entries. Instead, use one access control list entry with the values for the multiple IP addresses or CIDR ranges, separated by commas.

* **PRIVATE**

  The Private option creates a private endpoint for your database within a specified VCN. This option is available for databases on shared Exadata infrastructure, and is the only available option for databases on dedicated Exadata infrastructure. Review the private options for your configuration: 

  * **Autonomous Databases on shared Exadata infrastructure**:
  
    This option permits access through private enpoints by specifying the OCIDs of a subnet and the network security groups (NSGs) under the same VCN in the specification.

  * **Autonomous Databases on dedicated Exadata infrastructure**:

    The network path to a dedicated Autonomous Database is through a VCN and subnet defined by the dedicated infrastucture hosting the database. Usually, the subnet is defined as private, which means that there is no public Internet access to the databases.

    Autonomous Database supports restricted access using an ACL. You have the option to enable an ACL by setting the `isAccessControlEnabled` parameter. If access is disabled, then database access is defined by the network security rules. If enabled, then database access is restricted to the IP addresses and CIDR blocks defined in the ACL. Note that enabling an ACL with an empty list of IP addresses makes the database inaccessible. See [Autonomous Database with Private Endpoint](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adbsprivateaccess.htm) for overview and examples for private endpoint.

### Allowing TLS or Require Only Mutual TLS (mTLS) Authentication

If your Autonomous Database instance is configured to allow only mTLS connections, then you can reconfigure the instance to permit both mTLS and TLS connections. When you reconfigure the instance to permit both mTLS and TLS, you can use both authentication types at the same time, so that connections are no longer restricted to require mTLS authentication.

This option only applies to Autonomous Databases on shared Exadata infrastructure. You can permit TLS connections when network access type is configured by using one of the following options:

* **RESTRICTED**: with ACLs defined.
* **PRIVATE**: with a private endpoint defined.

## Example YAML

You can always configure the network access options when you create an Autonomous Database, or update the settings after you create the database. Following are some example YAMLs that show how to configure the networking with different network access options.

For Autonomous Databases on shared Exadata infrastructure, review the following examples:

* Configure network access [with PUBLIC access type](#autonomous-database-with-public-access-type-on-shared-exadata-infrastructure)
* Configure network access [with  RESTRICTED access type](#autonomous-database-with-restricted-access-type-on-shared-exadata-infrastructure)
* Configure network access [with  PRIVATE access type](#autonomous-database-with-private-access-type-on-shared-exadata-infrastructure)
* [Change the mutual TLS (mTLS) authentication setting](#allow-both-tls-and-mutual-tls-mtls-authentication-of-autonomous-database-on-shared-exadata-infrastructure)

For Autonomous Databases on dedicated Exadata infrastructure, refiew the following examples:

* Configure network access [with access control list enabled](#autonomous-database-with-access-control-list-enabled-on-dedicated-exadata-infrastructure)

> Note:
>
> * Operations on Exadata infrastructure require an `AutonomousDatabase` object to be in your cluster. These examples assume either the provision operation or the bind operation has been done before you begin, and the operator is authorized with API Key Authentication.
> * If you are creating an Autonomous Database, then see step 4 of [Provision an Autonomous Database](./README.md#provision-an-autonomous-database) in [Managing Oracle Autonomous Databases with Oracle Database Operator for Kubernetes](./README.md) topic to return to provisioning instructions.

### Autonomous Database with PUBLIC access type on shared Exadata infrastructure

To configure the network with PUBLIC access type, complete this procedure.

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enumeration (enum) value that defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
          # Allow secure access from everywhere.
          accessType: PUBLIC
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

### Autonomous Database with RESTRICTED access type on shared Exadata infrastructure

To configure the network with RESTRICTED access type, complete this procedure.


1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enumerated (enum) that defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |
    | `networkAccess.accessControlList` | []string | The client IP access control list (ACL). This feature is available for Autonomous Databases on [shared Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adboverview.htm#AEI) and on Exadata Cloud@Customer.<br> Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br><br>For shared Exadata infrastructure, this is an array of CIDR (Classless Inter-Domain Routing) notations for a subnet or VCN OCID.<br>Use a semicolon (;) as a deliminator between the VCN-specific subnets or IPs.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br><br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR (Classless Inter-Domain Routing) notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br><br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
          # Restrict access by defining access control rules in an Access Control List (ACL).
          accessType: RESTRICTED
          accessControlList:
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
    kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured

### Autonomous Database with PRIVATE access type on shared Exadata infrastructure

To configure the network with PRIVATE access type, complete this procedure

1. Visit [Overview of VCNs and Subnets](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/managingVCNs_topic-Overview_of_VCNs_and_Subnets.htm#console) and [Network Security Groups](https://docs.oracle.com/en-us/iaas/Content/Network/Concepts/networksecuritygroups.htm#working) to see how to create VCNs, subnets, and network security groups (NSGs) if you haven't created them yet. The subnet and the NSG has to be in the same VCN.

2. Copy and paste the OCIDs of the subnet and NSG to the corresponding parameters. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enumeration (enum) value that defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |
    | `networkAccess.privateEndpoint.subnetOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the subnet the resource is associated with.<br><br> **Subnet Restrictions:**<br> - For bare metal DB systems and for single node virtual machine DB systems, do not use a subnet that overlaps with 192.168.16.16/28.<br> - For Exadata and virtual machine 2-node RAC systems, do not use a subnet that overlaps with 192.168.128.0/20.<br> - For Autonomous Database, setting this will disable public secure access to the database.<br> These subnets are used by the Oracle Clusterware private interconnect on the database instance.<br> Specifying an overlapping subnet will cause the private interconnect to malfunction.<br> This restriction applies to both the client subnet and the backup subnet. | Yes |
    | `networkAccess.privateEndpoint.nsgOCIDs` | string[] | A list of the [OCIDs](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the network security groups (NSGs) that this resource belongs to. Setting this to an empty array after the list is created removes the resource from all NSGs. For more information about NSGs, see [Security Rules](https://docs.cloud.oracle.com/Content/Network/Concepts/securityrules.htm).<br><br> **NsgOCIDs restrictions:**<br> - Autonomous Databases with private access require at least 1 Network Security Group (NSG). The nsgOCIDs array cannot be empty. | Yes |
    | `networkAccess.privateEndpoint.hostnamePrefix` | string | The hostname prefix for the resource. | No |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
          # Assigns a private endpoint, private IP, and hostname to your database.
          accessType: PRIVATE
          privateEndpoint:
            subnetOCID: ocid1.subnet...
            nsgOCIDs:
            - ocid1.networksecuritygroup...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

### Allow both TLS and mutual TLS (mTLS) authentication of Autonomous Database on shared Exadata infrastructure

If you are using either the RESTRICTED or the PRIVATE network access option, then you can choose whether to permit both TLS and mutual TLS (mTLS) authentication, or to permit only mTLS authentication. To change the mTLS authentication setting, complete the following steps:

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_mtls.yaml`](./../../config/samples/adb/autonomousdatabase_update_mtls.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.isMTLSConnectionRequired` | boolean| Indicates whether the Autonomous Database requires mTLS connections. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
          isMTLSConnectionRequired: false
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_update_mtls.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

### Autonomous Database with access control list enabled on dedicated Exadata infrastructure

To configure the network with RESTRICTED access type using an access control list (ACL), complete this procedure.

1. Add the following parameters to the specification. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.isAccessControlEnabled` | boolean | Indicates if the database-level access control is enabled.<br><br>If disabled, then database access is defined by the network security rules.<br><br>If enabled, then database access is restricted to the IP addresses defined by the rules specified with the `accessControlList` property. While specifying `accessControlList` rules is optional, if database-level access control is enabled, and no rules are specified, then the database will become inaccessible. The rules can be added later by using the `UpdateAutonomousDatabase` API operation, or by using the edit option in console.<br><br>When creating a database clone, you should specify the access control setting that you want the clone database to use. By default, database-level access control will be disabled for the clone.<br>This property is applicable only to Autonomous Databases on the Exadata Cloud@Customer platform. | Yes |
    | `networkAccess.accessControlList` | []string | The client IP access control list (ACL). This feature is available for autonomous databases on [shared Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adboverview.htm#AEI) and on Exadata Cloud@Customer.<br> Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br><br>For shared Exadata infrastructure, this is an array of CIDR (Classless Inter-Domain Routing) notations for a subnet or VCN OCID.<br>Use a semicolon (;) as a deliminator between the VCN-specific subnets or IPs.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br><br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR (Classless Inter-Domain Routing) notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br><br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry. | Yes |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        networkAccess:
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
    kubectl apply -f config/samples/adb/autonomousdatabase_update_network_access.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
