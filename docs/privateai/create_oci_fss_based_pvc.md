# Create an OCI File Storage Service PV and PVC

This procedure creates a PersistentVolume (PV) and PersistentVolumeClaim (PVC) for an existing file system in the Oracle Cloud Infrastructure (OCI) File Storage service (FSS). The PV uses the FSS CSI driver, `fss.csi.oraclecloud.com`.

For more information, see [Provisioning PVCs on the File Storage Service](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengcreatingpersistentvolumeclaim_Provisioning_PVCs_on_FSS.htm).

**Important:** Ensure that the OKE worker nodes can reach the FSS mount target’s private IP address. For a single-VCN setup, create the mount target in the same VCN as the OKE worker nodes. Configure the required routes and network security rules to allow File Storage traffic. See [Configuring VCN Security Rules for File Storage](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/securitylistsfilestorage.htm?utm_source=chatgpt.com).

1. Create an OCI file system, mount target, and export. Create them in a network configuration that allows the OKE worker nodes to reach the mount target. For instructions, see [Creating a File System](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/create-file-system.htm).

2. Record the following details; you will need them to configure the PV:

   - The file system OCID
   - The mount target IP address
   - The export path

3. Review the PV manifest, [`oke-pv.yaml`](./provisioning/oke-pv.yaml). Set `volumeHandle` to the file system OCID, mount target IP address, and export path, separated by colons:

   ```text
   <FileSystemOCID>:<MountTargetIP>:<ExportPath>
   ```

   The manifest must use the FSS CSI driver, `fss.csi.oraclecloud.com`. For static provisioning, set `storageClassName: ""`; this procedure does not create or use a StorageClass.

4. Review the PVC manifest, [`oke-pvc.yaml`](./provisioning/oke-pvc.yaml). Set `volumeName` to the PV name and `storageClassName: ""`. The PVC request must match the storage capacity specified in the PV.

5. Apply the manifests from the repository root:

   ```sh
   kubectl apply -f docs/privateai/provisioning/oke-pv.yaml
   kubectl apply -f docs/privateai/provisioning/oke-pvc.yaml
   ```

6. Verify that the PV and PVC are bound:

   ```sh
   kubectl get pv oke-fsspv
   kubectl get pvc oke-fsspvc -n pai
   ```

   Confirm that both resources show `Bound`.

7. Copy Model Files to the PVC

   Creating the PV and PVC does not copy model files to the file system. Before deploying Private AI with models stored on this PVC, mount the claim in a temporary Pod and copy the model files to it. Verify that the expected files are present and readable from the mounted file system. The files remain in OCI File Storage after the temporary Pod is removed.

8. In the `PrivateAi` resource, map the PVC to the path where Private AI should access the model files. For example:

   ```yaml
   spec:
     pvcList:
       oke-fsspvc: /privateai/models
   ```

Specify the corresponding paths in the model configuration file.

**Note:** The `storage: 100Gi` values in the PV and PVC are Kubernetes binding values. They must match, but they do not set or limit the capacity of the OCI file system.
