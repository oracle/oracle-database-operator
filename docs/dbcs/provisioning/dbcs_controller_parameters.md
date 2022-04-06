# Oracle DB Operator DBCS Controller - Parameters to use in .yaml file

This page has the details of the parameters to define the specs related to an operation to be performed for an OCI DBCS System to be managed using Oracle DB Operator DBCS Controller.

| Parameter Name | Description | Mandatory Parameter? (Y/N) | Parameter Value type | Default Value (If Any) | Allowed Values (If Any) | 
| -------------- | ----------  | ------- | ------- | ------- | ------- |
| ociConfigMap | Kubernetes Configmap created for OCI account in the prerequisites steps. | Y | String | | |
| ociSecret | Kubernetes Secret created using PEM Key for OCI account in the prerequisites steps. | Y | String | | |
| availabilityDomain | Availability Domain of the OCI region where you want to provision the DBCS System. | Y | String | | Please refer to this link: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm |
| compartmentId | OCID of the OCI Compartment. | Y | String | | |
| dbAdminPaswordSecret | Kubernetes Secret created for DB Admin Account in prerequisites steps. | Y | String | | A strong password for SYS, SYSTEM, and PDB Admin. The password must be at least nine characters and contain at least two uppercase, two lowercase, two numbers, and two special characters. The special characters must be _, #, or -.|
| autoBackupEnabled | Whether to enable automatic backup or not. | N | Boolean | | True or False |
| autoBackupWindow | Time window selected for initiating automatic backup for the database system. There are twelve available two-hour time windows. | N | String | | Please refer to this link: https://docs.oracle.com/en-us/iaas/api/#/en/database/20160918/datatypes/DbBackupConfig |
| recoveryWindowsInDays | Number of days between the current and the earliest point of recoverability covered by automatic backups.  | N | Integer | | Minimum: 1 and Maximum: 60 |
| dbEdition | Oracle Database Software Edition. | N | String | | STANDARD_EDITION or ENTERPRISE_EDITION or ENTERPRISE_EDITION_HIGH_PERFORMANCE or ENTERPRISE_EDITION_EXTREME_PERFORMANCE |
| dbName | The database name. | Y | String | | The database name cannot be longer than 8 characters. It can only contain alphanumeric characters. |
| dbVersion | The Oracle Database software version. | Y | String | | Min lenght: 1 and Max length: 255 |
| dbWorkload | The database workload type. | Y | String | | OLTP or DSS |
| diskRedundancy | The type of redundancy configured for the DB system. NORMAL is 2-way redundancy. HIGH is 3-way redundancy. | N | String | | HIGH or NORMAL |
| displayName | The user-friendly name for the DB system. The name does not have to be unique. | N | String | | Min length: 1 and Max length: 255 |
| hostName | The hostname for the DB system. | Y | String | | Hostname can contain only alphanumeric and hyphen (-) characters. |
| initialDataStorageSizeInGB | Size (in GB) of the initial data volume that will be created and attached to a virtual machine DB system.  | N | Integer | | Min Value in GB: 2 |
| licenseModel | The Oracle license model that applies to all the databases on the DB system. | N | String | LICENSE_INCLUDED | LICENSE_INCLUDED or BRING_YOUR_OWN_LICENSE |
| nodeCount | The number of nodes in the DB system. For RAC DB systems, the value is greater than 1. | N | Integer | | Minimum: 1 |
| pdbName | The name of the pluggable database. The name must begin with an alphabetic character and can contain a maximum of thirty alphanumeric characters. Special characters are not permitted. | N | String | | The PDB name can contain only alphanumeric and underscore (_) characters. |
| privateIp | A private IP address of your choice. Must be an available IP address within the subnet's CIDR. If you don't specify a value, Oracle automatically assigns a private IP address from the subnet. | N | String | | Min length: 1 and Max length: 46 |
| shape | The shape of the DB system. The shape determines resources to allocate to the DB system. | Y | String | | Please refer to this link for the available shapes: https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/overview.htm |
| sshPublicKeys | Kubernetes secret created with the Public Key portion of the key pair created to access the DB System. | Y | String | | |
| storageManagement | The storage option used in DB system. ASM - Automatic storage management LVM - Logical Volume management. | N | String | | ASM or LVM |
| subnetId | The OCID of the subnet the DB system is associated with. | Y | String | | |
| tags | Tags for the DB System resource. Each tag is a simple key-value pair with no predefined name, type, or namespace.  | N | String | | |
| tdeWalletPasswordSecret | The Kubernetes secret for the TDE Wallet password. | N | String | |  |
| timeZone | The time zone of the DB system. | N | String | | Please refer to this link: https://docs.oracle.com/en-us/iaas/Content/Database/References/timezones.htm#Time_Zone_Options |
