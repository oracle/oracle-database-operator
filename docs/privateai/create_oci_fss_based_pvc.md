# Create OCI FSS based PVC

The Persistent Volume (PV) is created using `oci-fss` Storage Class. For more information, see [Creating Persistent Volume Claims](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengcreatingpersistentvolumeclaim_Provisioning_PVCs_on_FSS.htm) for OCI File Storage. A Persistent Volume Claim (PVC) is created for this PV. To use the PV with `oci-fss` Storage Class, you must create an OCI File System Storage and a Mount Target in OCI before creating the PV.

**Important:** For the OKE Cluster Nodes to access the FSS Mount Target, the VCN of the OKE Cluster and the File System Storage in OCI must be the same.

Complete the following steps:

1. Provision an OCI File Systems in the same VCN that is used by the OKE Cluster on which you want to deploy the PrivateAI Container. For more information, see [Create File System](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/create-file-system.htm) for the details of this step.

2. After the OCI File System is created, obtain the details of the `Export Path` and `Mount Target` (the IP and OCID of Mount Target). This information is required in the next step.

3. Create a YAML file for the Storage Class. To see how to do this, refer to the following example file: [StorageClass.yaml](./provisioning/StorageClass.yaml). You will need to provide the `OCID` of the mount target in this file.

4. Create a YAML file for the Persistent Volume (PV). This configuration will use the `oci-fss` storage class. To see how to do this, refer to the following example file: [oke-pv.yaml](./provisioning/oke-pv.yaml). You will need to provide the `Export Path` and Mount Target `IP` in this file.

5. Create a YAML file for the Persistent Volume Claim (PVC). In this file, we use the PV created in the previous step. To see how to do this, refer to the following example file: [oke-pvc.yaml](./provisioning/oke-pvc.yaml)

6. Apply the YAML files you have created in the preceding steps to create the PVC using `oci-fss` based File System:
    ```sh
    kubectl apply -f StorageClass.yaml
    kubectl apply -f oke-pv.yaml
    kubectl apply -f oke-pvc.yaml
    ```

7. Check the details of the PVC created using `oci-fss` based File System:
    ```sh
    kubectl get sc
    kubectl get pv -n pai
    kubectl get pvc -n pai
    ```    