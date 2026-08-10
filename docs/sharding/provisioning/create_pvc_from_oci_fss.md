# Create an Additional PVC from OCI File Storage Service (FSS)

Follow the steps below to create a PersistentVolumeClaim (PVC) backed by OCI File Storage Service (FSS).

## Prerequisites

- An OCI File System and Mount Target have been created in your OCI tenancy.
- The OCI File Storage CSI driver is installed on your OKE cluster.

## Procedure

1. Create an OCI File System and Mount Target in your OCI tenancy.
2. Create a StorageClass using the OCI File Storage CSI provisioner (`fss.csi.oraclecloud.com`) and specify the Mount Target OCID. For example:

   - [storage_class.yaml](./storage_class.yaml)

3. Verify that the OCI File Storage CSI driver is installed:

   ```bash
   kubectl get csidrivers
   ```

   Ensure the output includes:

   ```text
   fss.csi.oraclecloud.com
   ```

4. Create a PersistentVolumeClaim (PVC) using the StorageClass created in the previous step. For example:

   - [persistent_volume_claim.yaml](./persistent_volume_claim.yaml)

5. Mount the PVC in your Oracle GDD Deployment. The shared file system will be mounted inside the Oracle GDD Pods.

## Use Cases

OCI File Storage provides shared (`ReadWriteMany`) storage that can be mounted simultaneously by multiple Pods across multiple worker nodes. Typical use cases include:

- Storing configuration files
- Sharing deployment artifacts
- Any workload requiring shared, persistent storage
