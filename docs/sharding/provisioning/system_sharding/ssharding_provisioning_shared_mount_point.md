# Deploy Oracle GDD with System-Managed Sharding using a shared mount point

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

To use a shared mount point, first create a PersistentVolume (PV) and a PersistentVolumeClaim (PVC) using a shared storage solution such as OCI File Storage Service (FSS). For instructions, see [Create a PVC from OCI FSS](../create_pvc_from_oci_fss.md).

This example uses the `ssharding_shard_prov_shared_mount.yaml` manifest to provision an Oracle GDD deployment with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* A shared PVC mounted at `/data` for the catalog, shard, and GSM pods
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Use the following manifest:

[`ssharding_shard_prov_shared_mount.yaml`](./ssharding_shard_prov_shared_mount.yaml)

The sample manifest mounts the shared PVC at `/data` for the catalog, shard, and GSM pods.

1. Deploy the `ssharding_shard_prov_shared_mount.yaml` manifest:

    ```sh
    kubectl apply -f ssharding_shard_prov_shared_mount.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns

    # View the logs for a specific pod (for example, "shard1-0"):
    kubectl logs -f pod/shard1-0 -n shns
    ```

3. Once the pods are running, connect to one of the pods and verify that the shared volume is mounted at `/data`:

   ```sh
   # Switch to a specific pod (for example, "shard1-0"):
   kubectl exec -it pod/shard1-0 -n shns /bin/bash

   # Verify that the shared volume is mounted:
   bash-5.1$ df -h /data
   Filesystem                               Size  Used Avail Use% Mounted on
   xx.xx.xx.xxx:/FileSystem_GDD_Deployment  8.0E     0  8.0E   0% /data
   ```
