# Provisioning an Oracle RAC Database with Storage Class-backed ASM Storage and PVC-based Software

#### Use Case
* In this use case, Oracle Grid Infrastructure and the Oracle RAC Database are deployed automatically using Oracle RAC Controller. The controller generates the response files from the parameters in the YAML file.
* This example uses [racdb_prov_sc_pvc.yaml](./racdb_prov_sc_pvc.yaml) to provision an Oracle RAC Database with storage-class-backed ASM storage and PVC-based software locations.
* The sample includes:
  * 1 RAC node Pod (`nodeCount: 1`)
  * Headless services for RAC, including the SCAN service and RAC node hostname
  * An ASM disk group whose persistent volumes are created dynamically using the storage class specified in `asmDiskGroupDetails[].storageClass`
  * A software home persistent volume sized by `swLocStorageSizeInGb`
  * A staged software PVC referenced by `configParams.swStagePvc` and mounted at `configParams.swStagePvcMountLocation`
  * Encoded database credentials referenced through `dbSecret.keyFileName` and `dbSecret.pwdFileName`
  * Namespace: `rac`

### In This Example
* The sample uses the image `phx.ocir.io/intsanjaysingh/oracle/database-rac:19.3.0-slim`.
* If you build the RAC image yourself using the files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image), update the `image` field in [racdb_prov_sc_pvc.yaml](./racdb_prov_sc_pvc.yaml) to point to your image.
* The `DATA` ASM disk group uses `/dev/asm-disk1` and `/dev/asm-disk2`, with `asmStorageSizeInGb: 50` and `storageClass: "oci-bv"`.
* The software home PVC size is `swLocStorageSizeInGb: 300`.
* The staged Grid Infrastructure and Database software zip files are expected in the existing PVC `pv-stage-vol-claim`, mounted inside the pod at `/stage/software/19c/1930-new`.
* The database secret uses encoded files referenced as `key.pem` and `pwdfile.enc`.

### Steps: Deploy the Oracle RAC Database
Use the file [racdb_prov_sc_pvc.yaml](./racdb_prov_sc_pvc.yaml) for this use case.

1. Deploy the sample:
   ```sh
   kubectl apply -f racdb_prov_sc_pvc.yaml
   ```
2. Check the deployment status:
   ```sh
   kubectl get all -n rac
   ```
3. Follow the provisioning log from the RAC pod:
   ```sh
   kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
   ===================================
   ORACLE RAC DATABASE IS READY TO USE
   ===================================
   ```
4. See sample controller output in [Logs](./logs/racdb_prov/racdbprov-sample_details.txt) and the corresponding operator logs in [DB Operator Logs](./logs/racdb_prov/operator_logs.txt).
