#

## Oracle Autonomous Database (ADB) Prerequisites

Oracle Database Operator for Kubernetes must have access to OCI services. 

To provide access, choose **one of the following approaches**:

* The provider uses [API Key authentication](#authorized-with-api-key-authentication)

* The Kubernetes cluster nodes are [granted with Instance Principal](#authorized-with-instance-principal)

### Authorized with API Key Authentication

By default, all pods in the Oracle Container Engine for Kubernetes (OKE) are able to access the instance principal certificates, so that the operator calls OCI REST endpoints without any extra step. If you're using OKE, then please proceed to the installation.
If the operator is deployed in a third-party Kubernetes cluster, then the credentials of the Oracle Cloud Infrastructure (OCI) user are needed. The operator reads these credentials from a ConfigMap and a Secret.

Oracle recommends using the helper script `set_ocicredentials.sh` in the root directory of the repository; This script will generate a ConfigMap and a Secret with the OCI credentials. By default, the script parses the **DEFAULT** profile in `~/.oci/config`. The default names of the ConfigMap and the Secret are, respectively: `oci-cred` and `oci-privatekey`.

```sh
./set_ocicredentials.sh run
```

You can change the default values as follows:

```sh
./set_ocicredentials.sh run -path <oci-config-path> -profile <profile-name> -configmap <configMap-name> -secret <secret-name>
```

Alternatively, you can create these values manually. The ConfigMap should contain the following items: `tenancy`, `user`, `fingerprint`, `region`, `passphrase`. The Secret should contain an entry named `privatekey`.

```sh
kubectl create configmap oci-cred \
--from-literal=tenancy=<TENANCY_OCID> \
--from-literal=user=<USER_OCID> \
--from-literal=fingerprint=<FINGERPRINT> \
--from-literal=region=<REGION> \
--from-literal=passphrase=<PASSPHRASE_STRING>(*)

kubectl create secret generic oci-privatekey \
--from-file=privatekey=<PATH_TO_PRIVATE_KEY>
```

> Note: passphrase is deprecated. You can ignore that line.

After creating the ConfigMap and the Secret, use their names as the values of `ociConfigMap` and `ociSecret` attributes in the yaml files for provisioning, binding, and other operations.

### Authorized with Instance Principal

Instance principal authorization enables the operator to make API calls from an instance (that is, a node) without requiring the `ociConfigMap`,  and `ociSecret` attributes in the `.yaml` file.

> Note: Instance principal authorization applies only to instances that are running in the Oracle Cloud Infrastructure (OCI).

To set up Instance Principle authorization: 

1. Get the `compartment OCID`:

    Log in to the cloud console, and click **Compartment**.

    ![compartment-1](/images/adb/compartment-1.png)

    Choose the compartment where the cluster creates instances, and **copy** the OCID in the details page.

    ![compartment-2](/images/adb/compartment-2.png)

2. Create a dynamic group and matching rules:

    Go to the **Dynamic Groups** page, and click **Create Dynamic Group**.

    ![instance-principal-1](/images/adb/instance-principal-1.png)

    In the **Matching Rules** section, write the following rule. Change `compartment-OCID` to the OCID of your compartment. This rule enables all the resources, including **nodes** in the compartment, to be members of the dynamic group.

    ```sh
    All {instance.compartment.id = 'compartment-OCID'}
    ```

    ![instance-principal-2](/images/adb/instance-principal-2.png)

    To apply the rules, click **Create**.

3. Set up policies for dynamic groups:

    Go to **Policies**, and click **Create Policy**.

    ![instance-principal-3](/images/adb/instance-principal-3.png)

    This example enables the dynamic group to manage all the resources in your tenancy:

    ```sh
    Allow dynamic-group <your-dynamic-group> to manage all-resources in tenancy
    ```

    You can also specify a particular resouce access for the dynamic group. This example enables the dynamic group to manage Oracle Autonomous Database in a given compartment:

    ```sh
    Allow dynamic-group <your-dynamic-group> to manage autonomous-database-family in compartment <your-compartment>
    ```

At this stage, the operator has been granted sufficient permissions to call OCI services. You can now proceed to the installation.
