#

## Oracle Autonomous Database (ADB) Prerequisites

Oracle Database Operator for Kubernetes must have access to OCI services.

To provide access, choose **one of the following approaches**:

* The provider uses [API Key authentication](#authorized-with-api-key-authentication)

* The Kubernetes cluster nodes are [granted with Instance Principal](#authorized-with-instance-principal)

## Authorized with API Key Authentication

API keys are supplied by users to authenticate the operator accessing Oracle Cloud Infrastructure (OCI) services. The operator reads the credintials of the OCI user from a ConfigMap and a Secret. If you're using Oracle Container Engine for Kubernetes (OKE), you may alternatively use [Instance Principal](#authorized-with-instance-principal) to avoid the need to configure user credentails or a configuration file. If the operator is deployed in a third-party Kubernetes cluster, then the credentials or a configuration file are needed, since Instance principal authorization applies only to instances that are running in the OCI.

Oracle recommends using the helper script `set_ocicredentials.sh` in the root directory of the repository; this script will generate a ConfigMap and a Secret with the OCI credentials. By default, the script parses the **DEFAULT** profile in `~/.oci/config`. The default names of the ConfigMap and the Secret are, respectively: `oci-cred` and `oci-privatekey`.

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

## Authorized with Instance Principal

Instance principal authorization enables the operator to make API calls from an instance (that is, a node) without requiring the `ociConfigMap`,  and `ociSecret` attributes in the `.yaml` file. This approach applies only to instances that are running in the Oracle Cloud Infrastructure (OCI). In addition, this approach grants permissions to the nodes that matche the rules, which means that all the pods in the nodes can make the service calls.

To set up the instance princials, you will have to:

* [Define dynamic group that includes the nodes in which the operator runs](#define-dynamic-group)
* [Define policies that grant to the dynamic group the required permissions for the operator to its OCI interactions](#define-policies)

### Define Dynamic Group

1. Go to the **Dynamic Groups** page, and click **Create Dynamic Group**.

    ![instance-principal-1](/images/adb/instance-principal-1.png)

2. In the **Matching Rules** section, write rules the to include the OKE nodes in the dynamic group.

    Example 1 : enables **all** the instances, including OKE nodes in the compartment, to be members of the dynamic group.

    ```sh
    All {instance.compartment.id = '<compartment-OCID>'}
    ```

    ![instance-principal-2](/images/adb/instance-principal-2.png)

    Example 2 : enables the specific OKE nodes in the compartment, to be members of the dynamic group.

    ```sh
    Any {instance.id = '<oke-node1-instance-OCID>', instance.id = '<oke-node2-instance-OCID>', instance.id = '<oke-node3-instance-OCID>'}
    ```

    ![instance-principal-3](/images/adb/instance-principal-3.png)

3. To apply the rules, click **Create**.

### Define Policies

1. Get the `compartment name` where the database resides:

    > Note: You may skip this step if the database is in the root compartment.

    Go to **Autonomous Database** in the Cloud Console.

    ![adb-id-1](/images/adb/adb-id-1.png)

    Copy the name of the compartment in the details page.

    ![instance-principal-4](/images/adb/instance-principal-4.png)

2. Set up policies for dynamic groups to grant access to its OCI interactions. Use the dynamic group name is from the [Define Dynamic Group](#define-dynamic-group) section, and the compartment name from the previous step:

    Go to **Policies**, and click **Create Policy**.

    ![instance-principal-5](/images/adb/instance-principal-5.png)

    Example 1: enable the dynamic group to manage **all** the resources in a compartment

    ```sh
    Allow dynamic-group <dynamic-group-name> to manage all-resources in compartment <compartment-name>
    ```

    Example 2: enable the dynamic group to manage **all** the resources in your tenancy (root compartment).

    ```sh
    Allow dynamic-group <dynamic-group-name> to manage all-resources in tenancy
    ```

    Example 3: enable a particular resouce access for the dynamic group to manage Oracle Autonomous Database in a given compartment

    ```sh
    Allow dynamic-group <dynamic-group-name> to manage autonomous-database-family in compartment <compartment-name>
    ```

3. To apply the policy, click Create.

At this stage, the instances where the operator deploys have been granted sufficient permissions to call OCI services. You can now proceed to the installation.
