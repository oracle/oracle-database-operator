# Provisioning an Oracle Restart Database with Huge Page allocation
### In this Usecase:
*	Oracle Grid Infrastructure and the Oracle Restart Database are automatically deployed with Oracle Restart Controller, utilizing a custom storage class.
*	This example configures Huge Pages at the pod level, allocating 2MB per page for memory.
*	For guidance on memory calculation when using Huge Pages to deploy an Oracle Restart Database in a Kubernetes cluster with Oracle Restart Controller. You can refer [HugePages in K8s](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
*	ASM disks are provisioned as persistent volumes via a custom storage class during deployment in this use case.
*	The example utilizes the file [oraclerestart_prov_hugepages.yaml](./oraclerestart_prov_hugepages.yaml) to provision an Oracle Restart Database with Oracle Restart Controller, featuring:
	*	An Oracle Restart pod
	*	Headless services for Oracle Restart, including rac instance hostnames
	*	Node Port 30007 mapped to port 1521 for the database listener
	*	2MB Huge Pages defined by the hugepages-2Mi parameter
	*	The `TOTAL_MEMORY` parameter sets the maximum allowable combined SGA and PGA for the Oracle Database instance
	*	Automatic creation of persistent volumes for ASM disks through the specified storage class
	*	Software and staged software persistent volumes, configured at the relevant worker node locations
	*	Deployment within the orestart namespace
	*	The staged software location on worker nodes is defined by `hostSwStageLocation`, where Grid Infrastructure and RDBMS binaries are copied
	*	GI HOME and RDBMS HOME within the Oracle Restart pod are mounted using a persistent volume, created with the specified storage class, and mounted to /u01 inside the pod. The volume size is determined by `swLocStorageSizeInGb`.
### In this Example:
  * Oracle Restart Database Slim Image `localhost/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `localhost/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_hugepages.yaml` file to point to the container image you have built. 
  * The disks provisioned using customer storage class (specified by `crsDgStorageClass`) for the Oracle Restart storage are `/dev/oracleoci/oraclevdd` and `/dev/oracleoci/oraclevde`. 
  * Specify the size of these devices along with names using the parameter `storageSizeInGb`. Size is by-default in GBs.

  
### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_hugepages.yaml](./oraclerestart_prov_hugepages.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_hugepages.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_hugepages.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample created
    ```
* Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":    
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ===============================
      ORACLE DATABASE IS READY TO USE
    ===============================
    ```
* Check Details of Kubernetes CRD Object as in this [example](./orestart_hugepages_object.txt)
