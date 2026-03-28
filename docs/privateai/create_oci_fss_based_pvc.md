# Create OCI FSS based PVC

The Persistent Volume (PV) is created using `oci-fss` Storage Class. Please refer to the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengcreatingpersistentvolumeclaim_Provisioning_PVCs_on_FSS.htm) for OCI File Storage. A Persistent Volume Claim (PVC) is created for this PV. For the PV with `oci-fss` Storage Class to be used, an OCI File System Storage and a Mount Target is created in OCI before creating the PV.

**Important:** The VCN of the OKE Cluster and the File System Storage in OCI Must be same in order for the OKE Cluster Nodes to access the FSS Mount Target.

1. Provision an OCI File Systems in the same VCN which is used by the OKE Cluster on which you want to deploy the PrivateAI Container. Please refer [here](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/create-file-system.htm) for the details of this step.

2. Once the OCI File System is created, get the details of the `Export Path` and `Mount Target` (the IP and OCID of Mount Target) information. It will be required in the next step.

3. Create a YAML file for the Storage Class. Please refer to the sample file: [StorageClass.yaml](./provisioning/StorageClass.yaml). You will need to provide the `OCID` of the mount target in this file.

4. Create a YAML file for the Persistent Volume (PV). It will use the `oci-fss` storage class. Please refer to the sample file: [oke-pv.yaml](./provisioning/oke-pv.yaml). You will need to provide the `Export Path` and Mount Target `IP` in this file.

5. Create a YAML file for the Persistent Volume Claim (PVC). It will use the PV created in previous step. Please refer to the sample file: [oke-pvc.yaml](./provisioning/oke-pvc.yaml)

6. Apply the above YAML files to create the PVC using `oci-fss` based File System.
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