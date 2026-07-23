# Deployment Prerequisites

This guide describes the prerequisites for deploying Oracle Single Instance Database (SIDB) on Kubernetes using Oracle Database Operator (OraOperator). Complete these steps before creating any SIDB resources.

Unless noted otherwise, the examples below use the `default` namespace. Create the required secrets in the same namespace where you plan to create the SIDB resources.

* ## Quick Checklist

  Before deploying SIDB, verify that you have completed the following:

  * Oracle Database container images
  * Kubernetes cluster
  * Persistent storage
  * Image pull secret
  * Database admin secret
  * TDE secret (if required)
  * TCPS TLS secret (if required)
  * ORDS secret (if required)
  * Valid StorageClass
  * Operator-watched namespace

* ## Prepare Oracle Database Container Images

  You can either build Single Instance Database Container Images from the source, following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance), or you can use the the pre-built images available at [https://container-registry.oracle.com](https://container-registry.oracle.com) by signing in and accepting the required license agreement.

  Oracle Database Releases Supported: Enterprise and Standard Edition for Oracle Database 19c, and later releases. Express Edition for Oracle Database 21.3.0 only. Oracle Database Free 23.2.0 and later Free releases
  
  Build Oracle REST Data Service Container Images from source following the instructions at [https://github.com/oracle/docker-images/tree/main/OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).
  The supported Oracle REST Data Service version is 21.4.2

* ## Ensure Sufficient Disk Space in Kubernetes Worker Nodes

  Provision Kubernetes worker nodes. Oracle recommends you provision them with 250 GB or more free disk space, which is required for pulling the base and patched database container images. If you are doing a Cloud deployment, then you can choose to increase the custom boot volume size of the worker nodes.

* ## Set Up Kubernetes and Volumes for Database Persistence

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes. Use a dynamic volume provisioner or pre-provision static persistent volumes manually. These volumes are required for persistent storage of the database files.

  For more more information about creating persistent volumes, see: [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

* ## Create Required Kubernetes Secrets for SIDB

  Before you apply SIDB manifests, create the secrets referenced by the sample you plan to use.

  **Image pull secret**

  Create this when your database image is hosted in a private registry such as Oracle Container Registry and your SIDB manifest sets `spec.image.pullSecrets`, for example `oracle-container-registry-secret`.

  ```sh
  kubectl create secret docker-registry oracle-container-registry-secret \
    --docker-server=container-registry.oracle.com \
    --docker-username='<registry-username>' \
    --docker-password='<registry-password>' \
    --docker-email='<email-address>'
  ```

  **Database admin password secret**

  Create the admin password secret referenced by `spec.security.secrets.admin`, for example `db-admin-secret`.

  ```sh
  kubectl create secret generic db-admin-secret \
    --from-literal=oracle_pwd='<database-password>'
  ```

  You can also start from the shipped sample:

  * [`../../config/samples/sidb/singleinstancedatabase_secrets.yaml`](../../config/samples/sidb/singleinstancedatabase_secrets.yaml)

  That sample also includes the alternate secret names used by the prebuilt, Express, and Free SIDB samples.

* ## Create Scenario-Specific Secrets When the Sample Requires Them

  Some workflows reference additional secrets. Create them before applying the manifest that uses them.

  **TDE wallet password secret**

  Create this when your chosen SIDB or True Cache manifest includes `spec.security.secrets.tde`, for example `tde-wallet-secret` with key `tde_wallet_pwd`.

  ```sh
  kubectl create secret generic tde-wallet-secret \
    --from-literal=tde_wallet_pwd='<tde-wallet-password>'
  ```

  This is separate from TCPS. TDE and TCPS are different setup concerns.

  **Primary auto-registration prerequisite for True Cache**

  If your True Cache manifest enables `spec.trueCache.autoTCServiceRegistration=true`,
  the operator launches a helper script on the primary through `DBMS_SCHEDULER`.
  This applies to every True Cache path: **same-cluster SIDB**, **cross-cluster SIDB**,
  and **external host SI or RAC** primaries (single-instance and RAC).

  - **Ensure `configure-primary-truecache-service.sh` is available on every primary
    node** where the scheduler job might run (for multi-node RAC, the same path on
    each node).

    The default path used by the operator is
    `/home/oracle/configure-primary-truecache-service.sh`.

    When the primary uses the True Cache extension image (built from
    `docker-images/OracleDatabase/SingleInstance/extensions/truecache`), that
    script is **already prebaked** at the default path — do **not** copy it again;
    only confirm it is present. Copy the sample script only when the primary is
    **outside** that extension-image workflow.

    To use another path, set `PRIMARY_TC_SERVICE_SCRIPT_PATH` on the True Cache
    SIDB (for example via `spec.envVars`):

    ```yaml
    envVars:
      - name: PRIMARY_TC_SERVICE_SCRIPT_PATH
        value: /custom/path/configure-primary-truecache-service.sh
    ```

  - **Keep the script owned by the Oracle software owner and executable**
    (for example mode `750` or `755`).

  - **Verify `$ORACLE_HOME/rdbms/admin/externaljob.ora`** in the primary DB home
    runs external jobs as that Oracle software owner. For example:

    ```text
    run_user = oracle
    run_group = oinstall
    ```

    A default `run_user = nobody` / `run_group = nobody` configuration can fail
    even when the helper script works in an interactive `oracle` shell.

  - **Smoke-test the scheduler** so a real executable job runs as `oracle` before
    you rely on automatic registration.

    Connect to the primary through the listener. For example:

    ```
    sqlplus sys@<PRIMARY_TNS_ALIAS> as sysdba
    ```

    where `<PRIMARY_TNS_ALIAS>` is a TNS alias that points to the primary listener
    (for RAC, the SCAN listener).

    Then run:

    ```sql
    BEGIN
      DBMS_SCHEDULER.CREATE_JOB(
        job_name            => 'EXTJOB_ID_TEST',
        job_type            => 'EXECUTABLE',
        job_action          => '/bin/bash',
        number_of_arguments => 2,
        enabled             => FALSE,
        auto_drop           => FALSE
      );
      DBMS_SCHEDULER.SET_JOB_ARGUMENT_VALUE('EXTJOB_ID_TEST', 1, '-lc');
      DBMS_SCHEDULER.SET_JOB_ARGUMENT_VALUE(
        'EXTJOB_ID_TEST', 2,
        'id > /tmp/extjob_id_test.out; echo ORACLE_HOME=$ORACLE_HOME >> /tmp/extjob_id_test.out; echo ORACLE_SID=$ORACLE_SID >> /tmp/extjob_id_test.out'
      );
      DBMS_SCHEDULER.RUN_JOB('EXTJOB_ID_TEST', use_current_session => FALSE);
    END;
    /
    ```

    Then verify on the node where the job ran:

    ```bash
    cat /tmp/extjob_id_test.out
    ```

    Expected output includes the Oracle DB software owner, for example
    `uid=... (oracle)`. If the file shows any other OS user, fix the scheduler
    runtime before relying on automatic registration.

  **TLS secret for TCPS**

  Create this when you enable `spec.security.tcps` and set `spec.security.tcps.tlsSecret`, for example `sidb-primary-tcps-tls` or `sidb-truecache-tcps-tls`. This secret is created as part of the SIDB TCPS cert-manager single-script flow. Please refer to [`tcps-cert-manager/README.md`](./tcps-cert-manager/README.md) for more details.

  ```sh
  kubectl create secret tls sidb-primary-tcps-tls \
    --cert=/path/to/tls.crt \
    --key=/path/to/tls.key
  ```

  Create the TLS secret before applying the SIDB manifest that references it. If clients or peer clusters connect by hostname, ensure the certificate SANs match those hostnames.

  **ORDS password secret**

  Create this when you deploy Oracle REST Data Services and the ORDS manifest references `ords-secret`.

  ```sh
  kubectl create secret generic ords-secret \
    --from-literal=oracle_pwd='<ords-password>'
  ```

  You can also start from the shipped ORDS secret sample:

  * [`../../config/samples/sidb/oraclerestdataservice_secrets.yaml`](../../config/samples/sidb/oraclerestdataservice_secrets.yaml)

* ## Validate Secret Names Before Applying a Manifest

  Before applying a SIDB or ORDS manifest, confirm that the secret names and keys in the YAML match the secrets you created. In particular, check:

  * `spec.security.secrets.admin.secretName`
  * `spec.security.secrets.admin.secretKey`
  * `spec.security.secrets.tde.secretName`
  * `spec.security.secrets.tde.secretKey`
  * `spec.security.tcps.tlsSecret`
  * `spec.image.pullSecrets`

  Also confirm:

  * The target namespace is watched by the operator
  * The admin password secret exists in the same namespace
  * The image pull secret exists in the same namespace, if the YAML uses `image.pullSecrets`
  * The TDE secret exists in the same namespace, if the YAML uses `security.secrets.tde`
  * The TCPS TLS secret exists in the same namespace, if the YAML uses `security.tcps.tlsSecret`
  * The storage class in the YAML exists in the cluster
  * The secret names and keys in the YAML match the Kubernetes secrets exactly

  > Do not use `default` namespace unless the operator is configured to watch `default`.

  Set the namespace first:

  ```sh
  NS=<operator-watched-namespace>
  ```

  If your SIDB YAML contains Database Admin Secret:

  ```yaml
  security:
    secrets:
      admin:
        secretName: db-admin-secret
        secretKey: oracle_pwd

  image:
    pullSecrets: oracle-container-registry-secret
  ```

  then both secrets must exist in the same namespace:

  ```sh
  kubectl get secret db-admin-secret -n $NS
  kubectl get secret oracle-container-registry-secret -n $NS
  ```

  If your SIDB YAML contains TDE Secret:

  ```yaml
  security:
    secrets:
      tde:
        secretName: tde-wallet-secret
        secretKey: tde_wallet_pwd
  ```

  verify the TDE secret:

  ```sh
  kubectl get secret tde-wallet-secret -n $NS
  ```

  Similarly, if your SIDB YAML contains TCPS Secret:

  ```yaml
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-primary-tcps-tls
  ```

  verify the TLS secret:

  ```sh
  kubectl get secret sidb-primary-tcps-tls -n $NS
  ```

* ## SIDB Manifest Checklist

  Review these fields before applying any SIDB YAML.

  | YAML field | What to verify |
  | --- | --- |
  | `metadata.namespace` | Must be a namespace watched by the operator |
  | `spec.serviceAccountName` | Required for OpenShift or custom service account usage |
  | `spec.sid` | Use only alphanumeric characters |
  | `spec.edition` | Must match the image and supported database edition |
  | `spec.createAs` | Must match the intended workflow: `primary`, `clone`, `standby`, or `truecache` |
  | `spec.security.secrets.admin.secretName` | Admin secret must exist in the same namespace |
  | `spec.security.secrets.admin.secretKey` | Key must exist inside the admin secret |
  | `spec.security.secrets.tde.secretName` | TDE secret must exist if this field is present |
  | `spec.security.secrets.tde.secretKey` | Key must exist inside the TDE secret |
  | `spec.security.tcps.tlsSecret` | TLS secret must exist if TCPS is enabled |
  | `spec.image.pullFrom` | Image must be accessible from the cluster |
  | `spec.image.pullSecrets` | Image pull secret must exist if the registry requires authentication |
  | `spec.persistence.oradata.storageClass` | Storage class must exist in the cluster |
  | `spec.persistence.oradata.size` | Requested size must be available |
  | `spec.primarySource` | Required for clone, standby, and True Cache workflows |
  | `spec.trueCache.blobConfigMapRef` | Required for True Cache consumers |
  | `spec.services.endpoints` | Required when exposing TCP or TCPS service externally |

* ## Minikube Cluster Environment
  
  By default, when you create a cluster using the `minicube start` command, Minikube creates a node with 2GB RAM, 2 CPUs, and 20GB disk space. However, these resources (particularly disk space and RAM) may not be sufficient for running and managing Oracle Database using the OraOperator. For better performance, Oracle recommends that you configure the cluster to have a larger RAM and disk space than the Minikube default. For example, the following command creates a Minikube cluster with 8GB RAM and 100GB disk space for the Minikube VM:
  
  ```sh
  minikube start --memory=8g --disk-size=100g
  ```
