# Configuring Network Access of Autonomous Database

This documentation describes how to configure network access with public access, access control rules (ACLs), or private endpoints. Also describes how to configure the TLS connections (require mutual TLS only or allow both 1 way TLS and mutual TLS). For more information, please visit [this page](https://docs.oracle.com/en/cloud/paas/autonomous-database/adbsa/autonomous-network-access.html#GUID-D2D468C3-CA2D-411E-92BC-E122F795A413).

## Supported Features

### Types of Network Access

There are three types of network access supported by Autonomous Database:

* **PUBLIC**:

  This option allows secure access from anywhere. The network access type is PUBLIC if no option is specified in the spec. With this option, mutual TLS (mTLS) authentication is always required to connect to the database. This option is available only for databases on shared Exadata infrastructure.

* **RESTRICTED**:

  This option restricts connections to the database according to the access control lists (ACLs) you specify. This option is available only for databases on shared Exadata infrastructure.
  
  You can add the following to your ACL:
  * **IP Address**: Specify one or more individual public IP address. Use commas to separate your addresses in the input field.
  * **CIDR Block**: Specify one or more ranges of public IP addresses using CIDR notation. Use commas to separate your CIDR block entries in the input field.
  * **Virtual Cloud Network (OCID)** (applies to Autonomous Databases on shared Exadata infrastructure): Specify the OCID of a virtual cloud network (VCN). If you want to specify multiple IP addresses or CIDR ranges within the same VCN, then do not create multiple access control list entries. Use one access control list entry with the values for the multiple IP addresses or CIDR ranges separated by commas.

* **PRIVATE**:

  This option creates a private endpoint for your database within a specified VCN. This option is available for databases on shared Exadata infrastructure and is the only available option for databases on dedicated Exadata infrastructure.

  * **Autonomous Databases on shared Exadata infrastructure**:
  
    This option allows the access through private enpoints by specifying the OCIDs of a subnet and network security groups (NSGs) under the same VCN in the spec.

  * **Autonomous Databases on dedicated Exadata infrastructure**:

    The network path to a dedicated Autonomous Database is through a VCN and subnet defined by the dedicated infrastucture hosting the database. Usually, the subnet is defined as private, meaning that there is no public Internet access to databases.

    Autonomous Database supports restricted access using a ACL. You can optionally enabling an ACL by setting the `isAccessControlEnabled` parameter. If disabled, database access is defined by the network security rules. If enabled, database access is restricted to the IP addresses and CIDR blocks defined in the ACL. Note that enabling an ACL with an empty list of IP addresses makes the database inaccessible. See [Autonomous Database with Private Endpoint](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adbsprivateaccess.htm) for overview and examples for private endpoint.

### Allowing TLS or Require Only Mutual TLS (mTLS) Authentication

If your Autonomous Database instance is configured to only allow mTLS connections, you can update the instance to allow both mTLS and TLS connections. When you update your configuration to allow both mTLS and TLS, you can use both authentication types at the same time and connections are no longer restricted to require mTLS authentication.

This option only applies to Autonomous Databases on shared Exadata infrastructure, and you can allow TLS connections when network access type is configured as follows:

* **RESTRICTED**: with ACLs defined.
* **PRIVATE**: with a private endpoint defined.

## Sample YAML

You can always configure the network access options when you create an Autonomous Database, or update the settings after the creation. Following are some sample YAMLs which configure the networking with different newtwork access options.

For Autonomous Databases on shared Exadata infrastructure, you can:

* Configure network access [with PUBLIC access type](#autonomous-database-with-public-access-type-on-shared-exadata-infrastructure)
* Configure network access [with  RESTRICTED access type](#autonomous-database-with-restricted-access-type-on-shared-exadata-infrastructure)
* Configure network access [with  PRIVATE access type](#autonomous-database-with-private-access-type-on-shared-exadata-infrastructure)
* [Change the mutual TLS (mTLS) authentication setting](#allow-both-tls-and-mutual-tls-mtls-authentication-of-autonomous-database-on-shared-exadata-infrastructure)

For Autonomous Databases on dedicated Exadata infrastructure, you can:

* Configure network access [with access control list enabled](#autonomous-database-with-access-control-list-enabled-on-dedicated-exadata-infrastructure)

> Note:
>
> * The above operations require an `AutonomousDatabase` object to be in your cluster. This example assumes either the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.
> * If you are creating an Autonomous Database, see step 4 of [Provision an Autonomous Database](./README.md#provision-an-autonomous-database) in [Managing Oracle Autonomous Databases with Oracle Database Operator for Kubernetes](./README.md) topic to return to provisioning instructions.

### Autonomous Database with PUBLIC access type on shared Exadata infrastructure

Follow the steps to configure the network with PUBLIC access type.

1. Add the following parameters to the spec. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enum value which defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |

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

Follow the steps to configure the network with RESTRICTED access type.

1. Add the following parameters to the spec. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enum value which defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |
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

Follow the steps to configure the network with RESTRICTED access type.

1. Visit [Overview of VCNs and Subnets](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/managingVCNs_topic-Overview_of_VCNs_and_Subnets.htm#console) and [Network Security Groups](https://docs.oracle.com/en-us/iaas/Content/Network/Concepts/networksecuritygroups.htm#working) to see how to create VCNs, subnets, and network security groups (NSGs) if you haven't created them yet. The subnet and the NSG has to be in the same VCN.

2. Copy and paste the OCIDs of the subnet and NSG to the corresponding parameters. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.accessType` | string | An enum value which defines how the database can be accessed. The value can be PUBLIC, RESTRICTED or PRIVATE. See [Types of Network Access](#types-of-network-access) for more descriptions. | Yes |
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

If you are using the RESTRICTED or the PRIVATE network access option, you can choose whether to allow both TLS and mutual TLS (mTLS) authentication, or to allow only mTLS authentication. Follow the steps to change the mTLS authentication setting.

1. Add the following parameters to the spec. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_mtls.yaml`](./../../config/samples/adb/autonomousdatabase_update_mtls.yaml):

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

Follow the steps to configure the network with RESTRICTED access type.

1. Add the following parameters to the spec. An example file is availble here: [`config/samples/adb/autonomousdatabase_update_network_access.yaml`](./../../config/samples/adb/autonomousdatabase_update_network_access.yaml):

    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `networkAccess.isAccessControlEnabled` | boolean | Indicates if the database-level access control is enabled.<br><br>If disabled, database access is defined by the network security rules.<br><br>If enabled, database access is restricted to the IP addresses defined by the rules specified with the `accessControlList` property. While specifying `accessControlList` rules is optional, if database-level access control is enabled and no rules are specified, the database will become inaccessible. The rules can be added later using the `UpdateAutonomousDatabase` API operation or edit option in console.<br><br>When creating a database clone, the desired access control setting should be specified. By default, database-level access control will be disabled for the clone.<br>This property is applicable only to Autonomous Databases on the Exadata Cloud@Customer platform. | Yes |
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
