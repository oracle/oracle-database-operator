## ENVIRONMENT VARIABLE DETAILS FOR ORACLE RAC DATABASE USING ORACLE RAC CONTROLLER

| Environment Variable                      | Description                                                                                | Default Value                     |
|-------------------------------------------|--------------------------------------------------------------------------------------------|-----------------------------------|
| apiVersion                                | API version of the RacDatabase Custom Resource                                             | database.oracle.com/v4            |
| kind                                      | Resource type being created                                                                | RacDatabase                       |
| metadata:name                             | Name assigned to RAC database resource being created                                       | racdbprov-sample                  |
| metadata:namespace                        | Namespace where the resource will be created                                               | rac                               |
| spec:instanceDetails:nodeCount            | Number of nodes in the RAC Cluster                                                         | 2                                 |
| spec:instanceDetails:racHostSwLocation    | Base directory for Oracle GI HOME and Oracle RDBMS HOME on the Worker Nodes                |                                   |
| spec:instanceDetails:racNodeName          | Prefix for RAC Cluster Node Hostnames                                                      |                                   |
| spec:instanceDetails:baseOnsTargetPort    | Target port for the ONS                                                                    |                                   |
| spec:instanceDetails:baseLsnrTargetPort   | Target port for the Database Listener                                                      |                                   |
| spec:instanceDetails:privateIPDetails:name| Name of the multus network from which this Network Interface IP will be assigned           |                                   |
| spec:instanceDetails:privateIPDetails:interface| Network interface name assigned from this network to the RAC Node's Pod               |                                   |
| spec:instanceDetails:workerNodeSelector:raccluster| Label for the worker nodes to be eligible for deployment of RAC Node               |                                   |
| spec:envVars:name                         | Name of the Optional Environmental Variable                                                |                                   |
| spec:envVars:value                        | Value of the Optional Environmental Variable                                               |                                   |
| TOTAL_MEMORY                              | Sets the combined target for the SGA and PGA when configuring memory                       |                                   |
| IGNORE_CRS_PREREQS                        | If set to true, it will use flags -ignorePreReq and -ignorePrereqFailure during the CRS Installation | false                   |
| IGNORE_DB_PREREQS                         | If set to true, it will use flags -ignorePrereq and -ignorePrereqFailure during the DB Software Installation | false           |
| spec:asmDiskGroupDetails:name             | Name of the ASM Disk Group                                                                 |                                   |
| spec:asmDiskGroupDetails:redundancy       | ASM Diskgroup Redundancy Level                                                             |                                   |
| spec:asmDiskGroupDetails:type             | Type of disk group. Possible types include 'CRSDG', 'DBDATAFILESDG', 'DBRECOVERY', 'DBREDO' and 'OTHERS'|                                 |
| spec:asmDiskGroupDetails:disks            | Shared Disks from the Worker Nodes to be used for the Disk Group                           |                                   |
| spec:asmDiskGroupDetails:autoUpdate       | If set to true, during disk addition, the disks are not only available inside the Pods, they are also added to the ASM Diskgroup| true                                |
| spec:sshKeySecret:name                    | Name of the kubernetes secret containing SSH keys                                          |                                   |
| spec:sshKeySecret:privKeySecretName       | Private key inside secret                                                                  |                                   |
| spec:sshKeySecret:pubKeySecretName        | Public key inside secret                                                                   |                                   |
| spec:dbSecret:name                        | Secret name containing the Password for Database Users                                     |                                   |
| spec:dbSecret:keyFileName                 | Key file name inside Database Secret                                                       |                                   |
| spec:dbSecret:pwdFileName                 | Password file name inside Database secret                                                  |                                   |
| spec:tdeWalletSecret:name                 | Secret name containing the Password for TDE                                                |                                   |
| spec:tdeWalletSecret:keyFileName          | Key file name inside TDE Secret                                                            |                                   |
| spec:tdeWalletSecret:pwdFileName          | Password file name inside TDE secret                                                       |                                   |
| spec:image                                | Container Slim image to be used for the deploying Oracle RAC Pod                           |                                   |
| spec:imagePullPolicy                      | Container Slim image Pull Policy                                                           | Always                            |
| spec:resources:requests:hugepages-2Mi     | Minimum size for of 2MiB HugePages                                                         |                                   |
| spec:resources:requests:memory            | Minimum memory required                                                                    |                                   |
| spec:resources:requests:cpu               | Minimum cpu required                                                                       |                                   |
| spec:resources:limits:hugepages-2Mi       | Maximum size for of 2MiB HugePages                                                         |                                   |
| spec:resources:limits:memory              | Maximum memory allowed                                                                     |                                   |
| spec:resources:limits:cpu                 | Maximum cpu allowed                                                                        |                                   |
| spec:scanSvcName                          | Name for the SCAN service (Single Client Access Name)                                      |                                   |
| spec:scanSvcTargetPort                    | Port for the SCAN service (Single Client Access Name)                                      |                                   |
| spec:serviceDetails:name                  | Name of the Oracle service (pluggable DB)                                                  |                                   |
| spec:securityContext:sysctls:name         | Name of the Kernel Parameter                                                               |                                   |
| spec:securityContext:sysctls:value        | Name of the Kernel Parameter                                                               |                                   |
| spec:configParams:gridResponseFile:configMapName| Name of the ConfigMap containing the GI Install Response File                        |                                   |
| spec:configParams:gridResponseFile:name   | Name of the response file for GI                                                           |                                   |
| spec:configParams:dbResponseFile:configMapName| Name of the ConfigMap containing the DBCA Response file                                |                                   |
| spec:configParams:dbResponseFile:name     | Name of the DBCA Response file                                                             |                                   |
| spec:configParams:gridHome                | ORACLE Grid Infrastructure Home location                                                   |                                   |
| spec:configParams:gridBase                | ORACLE Grid Infrastructure Base location                                                   |                                   |
| spec:configParams:dbHome                  | ORACLE Database Home location                                                              |                                   |
| spec:configParams:dbBase                  | ORACLE Database Base location                                                              |                                   |
| spec:configParams:inventory               | ORACLE Inventory location                                                                  |                                   |
| spec:configParams:gridSwZipFile           | Grid infrastructure software ZIP                                                           |                                   |
| spec:configParams:dbSwZipFile             | Database software ZIP                                                                      |                                   |
| spec:configParams:sgaSize                 | Size of the System Global Area                                                             |                                   |
| spec:configParams:pgaSize                 | Size of the Program Global Area                                                            |                                   |
| spec:configParams:processes               | Oracle process limit                                                                       |                                   |
| spec:configParams:cpuCount                | Number of CPUs to allocate                                                                 |                                   |
| spec:configParams:dbName                  | Oracle Database Name                                                                       |                                   |
| spec:configParams:hostSwStageLocation     | Host location where Oracle software ZIPs are staged                                        |                                   |
| spec:configParams:oPatchSwZipFile         | Opatch software ZIP                                                                        |                                   |
| spec:configParams:ruPatchLocation         | Directory containing the unzipped RU patch on the worker node                              |                                   |
| spec:configParams:oPatchLocation          | Location of the Opatch Software                                                            |                                   |
| spec:configParams:oneOffLocation          | One-off patch files directory where all the one-off patches for GI and RDBMS Home are unzipped|                                   |
| spec:configParams:gridOneOffIds           | Comma-separated Grid one-off patch IDs                                                     |                                   |
| spec:configParams:dbOneOffIds             | Comma-separated DB one-off patch IDs                                                       |                                   |