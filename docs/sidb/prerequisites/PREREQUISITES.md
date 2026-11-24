# Deployment Prerequisites

This guide describes the prerequisites for deploying Oracle Single Instance Database (SIDB) on Kubernetes using Oracle Database Operator (OraOperator). Complete these steps before creating any SIDB resources.

## How to use this guide

If you are new to SIDB or True Cache, follow this order:

1. Work through the [Quick Checklist](#quick-checklist) top to bottom.
2. Open each linked section only for the items that apply to your path.
3. Create Secrets in the **same namespace** as the SIDB that will use them.
4. When you edit YAML, use the [SIDB Manifest Checklist](#sidb-manifest-checklist)
   tables as a field reference and open the linked full sample YAML for your
   path so you can see the fields in context. You do not need to memorize every
   field first.
5. For True Cache, open [True Cache prerequisites](#true-cache-prerequisites)
   only for the linked secret/image checks, then follow the order table in
   [`README.md` — True Cache](../README.md#true-cache). Use
   [`TRUECACHE.md`](../truecache/TRUECACHE.md) for remote/peered paths and final
   verification — do not read every helper detail before the first deploy.

Create every Secret in the **same namespace** as the SIDB, True Cache, ORDS, or
other OraOperator resource that references it. Do not create Secrets in
`default` (or any other namespace) unless that is also the resource’s
`metadata.namespace` and the operator watches it. In the examples below, set
`NS` to that namespace and pass `-n "$NS"` on every `kubectl` secret command.

When you open one of these Secret links directly, define `NS` first. The SIDB
and True Cache samples use `default`; replace it when your resource uses a
different namespace (for example, the ORDS Quick Start uses `ordsnamespace`):

```sh
export NS=default
```

* ## Quick Checklist

  Complete every **Always required** item before any SIDB deploy. Complete each
  **Required for your scenario** item only when that feature or path applies.
  Use the **When authoring YAML** item as a field lookup while you edit
  manifests; it is not a separate cluster setup step.

  **Always required** (complete in this order)

  1. [Oracle Database container images](#prepare-oracle-database-container-images)
  2. [Kubernetes worker-node disk space](#ensure-sufficient-disk-space-in-kubernetes-worker-nodes)
  3. [Kubernetes cluster and persistent storage](#set-up-kubernetes-and-volumes-for-database-persistence)
  4. [Valid StorageClass](#set-up-kubernetes-and-volumes-for-database-persistence)
  5. [Database admin secret](#database-admin-secret)
  6. Operator-watched namespace for the resource being deployed

  **Required for your scenario** (complete only the items that apply)

  1. [Image pull secret](#image-pull-secret) — private registry or licensed OCR images (for example `enterprise_ru`)
  2. [TDE secret](#tde-secret) — when the manifest sets `spec.security.secrets.tde` (**required for True Cache**)
  3. [TCPS TLS secret](#tcps-tls-secret) — when the manifest enables `spec.security.tcps`
  4. [ORDS password secret](#ords-password-secret) — when deploying ORDS / OrdsSrvs
  5. [Primary source](#primary-source) — when `createAs` is `clone`, `standby`, or `truecache`
  6. [Data Guard](#data-guard) — when configuring Data Guard / standby / `DataguardBroker`
  7. [True Cache prerequisites](#true-cache-prerequisites) and [True Cache](#true-cache) — when deploying True Cache
  8. [SIDB Pod kernel sysctls](#configure-sidb-pod-kernel-sysctls) — when the manifest specifies Oracle sysctls or requests HugePages
  9. [SIDB HugePages](#configure-sidb-hugepages) — when requesting `hugepages-2Mi`

  **When authoring YAML**

  1. [SIDB Manifest Checklist](#sidb-manifest-checklist) — field-by-field reference for the SIDB you are writing

* ## Prepare Oracle Database Container Images

  Obtain a supported Oracle Database Single Instance image before creating a
  SIDB.

  > **Recommended:** Use the published prebuilt images from [Oracle Container
  > Registry (OCR)](https://container-registry.oracle.com). Configure registry
  > access by following [Pull Images from Oracle Container
  > Registry](../../../docs/registry/README.md), then reference the image and its
  > image pull Secret in the SIDB manifest.

  Choose one of the following image sources:

  1. **Recommended: use a prebuilt OCR image.** This is the supported path for
     most SIDB deployments and avoids building and publishing a custom image.
     True Cache samples use
     `container-registry.oracle.com/database/enterprise_ru:latest` (see
     [Pull Images from Oracle Container Registry](../../../docs/registry/README.md)).
  2. **Alternative: build an image from source.** Follow the [Oracle Database
     Single Instance container-image instructions](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance),
     then publish the image to a registry reachable from the SIDB namespace.
     Keep the version list below aligned with that upstream README; it is the
     source of truth for buildable releases.

  Database releases offered by the upstream Single Instance container build
  (see that README for the current `-v` values such as `23.26.0`, `21.3.0`,
  `19.3.0`):

  - **Enterprise Edition and Standard Edition 2:** Oracle Database 26ai
    (23.26.0), 21c (21.3.0), and 19c (19.3.0). Upstream also still ships older
    12c Dockerfiles; prefer 19c or later for OraOperator SIDB samples.
  - **Express Edition (XE):** Oracle Database 21c (21.3.0) only.
  - **Oracle Database Free:** Oracle Database 26ai Free (23.26.0 Free image).

  **True Cache** requires a True Cache capable Enterprise image from Oracle
  Database **26ai onwards** (upstream documents True Cache from 26ai; samples
  use OCR `database/enterprise_ru`).

  For Oracle REST Data Services (ORDS), prefer a current OCR ORDS image such as
  `container-registry.oracle.com/database/ords:<ords-version>`, or build from
  [OracleRestDataServices](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices).
  OraOperator `OrdsSrvs` supports ORDS **25.1.0 and later**; see
  [ORDS Services](../../ordsservices/README.md). Do not use the obsolete 21.4.2 pin
  from older SIDB docs.

* ## Ensure Sufficient Disk Space in Kubernetes Worker Nodes

  Provision Kubernetes worker nodes. Oracle recommends you provision them with 250 GB or more free disk space, which is required for pulling the base and patched database container images. If you are doing a Cloud deployment, then you can choose to increase the custom boot volume size of the worker nodes.

* ## Set Up Kubernetes and Volumes for Database Persistence

  Set up an on-premises Kubernetes cluster, or subscribe to a managed Kubernetes service, such as Oracle Cloud Infrastructure Container Engine for Kubernetes. Use a dynamic volume provisioner or pre-provision static persistent volumes manually. These volumes are required for persistent storage of the database files.

  For more information about creating persistent volumes, see: [https://kubernetes.io/docs/concepts/storage/persistent-volumes/](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

* ## Configure SIDB Pod Kernel Sysctls

  SIDB applies Oracle shared-memory and semaphore sysctls through
  `spec.securityContext.sysctls`. Oracle sysctl processing is enabled when
  the manifest explicitly provides an Oracle sysctl or requests HugePages.
  A regular memory limit or SGA/PGA targets alone do not cause sysctls to
  be injected.

  **Shared-memory settings**

  | Configuration | Behavior |
  | --- | --- |
  | Explicit `kernel.shmmax` | Preserve the value after validating that it is at least the effective SGA size. In HugePages mode, it is not capped by regular container memory. |
  | HugePages with no explicit `kernel.shmmax` | Set `kernel.shmmax` to the effective `hugepages-2Mi` allocation. |
  | HugePages with no explicit `kernel.shmall` | Set `kernel.shmall` to `ceil(kernel.shmmax / 4096)`. |
  | HugePages with explicit `kernel.shmall` | Require at least `ceil(kernel.shmmax / 4096)` and preserve the accepted value. |
  | Regular memory, with or without explicit Oracle sysctls | Preserve user-provided shared-memory sysctls, subject to validation. Do not automatically add a missing `kernel.shmmax` or `kernel.shmall`. |

  The `4096` divisor represents a 4KiB base page in the operator's
  calculation; `kernel.shmall` is not a count of 2MiB HugePages.

  When Oracle sysctl processing is enabled, the defaults for omitted
  semaphore and segment-count settings are:

  * `kernel.sem`: `"250 32000 100 128"`
  * `kernel.shmmni`: `"4096"`

  For example, a `16Gi` HugePages allocation without explicit
  shared-memory overrides produces:

  * `kernel.shmmax`: `"17179869184"` bytes.
  * `kernel.shmall`: `"4194304"` base pages.

  To specify these values explicitly in the SIDB manifest:

  ```yaml
  spec:
    securityContext:
      sysctls:
        - name: kernel.shmmax
          value: "17179869184"
        - name: kernel.shmall
          value: "4194304"
        - name: kernel.sem
          value: "250 32000 100 128"
        - name: kernel.shmmni
          value: "4096"
  ```

  These explicit settings are optional when using the HugePages configuration
  above: SIDB calculates the shared-memory values and supplies the defaults
  for omitted semaphore and segment-count settings.

  **Worker configuration**

  Before scheduling a Pod that uses these unsafe sysctls, configure each
  eligible worker's kubelet to allow them. The following is a
  KubeletConfiguration fragment:

  ```yaml
  allowedUnsafeSysctls:
    - "kernel.shm*"
    - "kernel.sem"
  ```

  Apply this configuration through the cluster's supported node-management
  procedure. Any applicable admission policy, including an OpenShift SCC,
  must also permit the requested sysctls.

  Verify the worker's active kubelet configuration:

  ```sh
  kubectl get --raw \
    /api/v1/nodes/<node-name>/proxy/configz |
    jq '.kubeletconfig.allowedUnsafeSysctls'
  ```

  The SIDB sample uses only `kernel.shmmax`, `kernel.shmall`, `kernel.sem`,
  and `kernel.shmmni`. Before adding `net.core.rmem_*` or `net.core.wmem_*`
  sysctls, verify that the worker runtime exposes the corresponding files
  inside Pod network namespaces. Kubelet allowlisting alone does not
  guarantee that the runtime can apply these settings.

* ## Configure SIDB HugePages

  Complete this section when the SIDB requests `hugepages-2Mi`.

  SIDB supports Linux 2MiB HugePages through the Kubernetes resource
  `hugepages-2Mi`. The operator validates the SIDB configuration and
  prepares Pod sysctls; worker HugePages allocation and container-runtime
  configuration must be completed separately.

  See [Database Pod Resources and Init Parameters](../README.md#database-pod-resources-and-init-parameters)
  for the SIDB manifest and memory-validation rules.

  **Prepare eligible workers**

  Preallocate HugePages on every worker where the SIDB can run. Size the
  pool for all HugePages workloads that may run together on that worker.

  The README example requests `16Gi` of HugePages per SIDB Pod. One such
  Pod requires 8,192 pages of 2MiB each. Two such Pods on the same worker
  require at least 16,384 pages.

  On a Linux worker managed directly by the administrator, run the
  following as root. This example assumes a 2MiB default HugePage size
  and Oracle group ID `54321`; verify both for your environment:

  ```sh
  grep Hugepagesize /proc/meminfo

  cat > /etc/sysctl.d/99-sidb-hugepages.conf <<'EOF'
  vm.nr_hugepages = 8192
  vm.hugetlb_shm_group = 54321
  EOF

  sysctl -p /etc/sysctl.d/99-sidb-hugepages.conf

  grep -E 'HugePages|Hugepagesize|Hugetlb' /proc/meminfo
  sysctl vm.hugetlb_shm_group
  ```

  `vm.nr_hugepages` specifies the worker's total target pool, not an
  additional allocation. Adjust it for existing workloads before applying
  the example. Confirm that the requested pages were actually allocated;
  allocation after boot can be limited by memory fragmentation.

  The Oracle process must belong to the group configured in
  `vm.hugetlb_shm_group`.

  For managed workers, including OpenShift-managed nodes, persist these
  settings through the supported node or node-pool configuration mechanism.

  If the HugePages allocation changes after kubelet starts, restart
  kubelet through the cluster's node-management procedure so it reports
  the updated capacity.

  **Verify Kubernetes capacity**

  Run from a client with cluster access:

  ```sh
  kubectl get nodes \
    -o custom-columns='NAME:.metadata.name,HUGEPAGES_2MI:.status.allocatable.hugepages-2Mi'

  kubectl describe node <node-name>
  ```

  For the README example, an eligible worker must advertise at least
  `16Gi` of allocatable `hugepages-2Mi` and have sufficient unallocated
  capacity for the Pod. Node allocatable capacity does not subtract
  existing Pod requests; review the node's allocated resources as well.

  HugePages and regular memory require separate capacity planning. Leave
  enough physical RAM for both workload budgets, other Pods, and node
  services.

  **Configure container memory locking**

  Ensure the database process receives soft and hard `memlock` limits
  sufficient for its maximum SGA allocation. Changing a worker's login
  limits alone does not establish the limits used by a container.

  For CRI-O workers, the following example sets a 16GiB memory-lock limit.
  It retains the example `nofile` setting; preserve any other existing
  `default_ulimits` entries used by your cluster.

  ```sh
  crio status config | grep -A 8 'default_ulimits'

  cat > /etc/crio/crio.conf.d/99-oracle-memlock.conf <<'EOF'
  [crio.runtime]
  default_ulimits = [
    "nofile=1048576:1048576",
    "memlock=17179869184:17179869184"
  ]
  EOF

  systemctl restart crio

  crio status config | grep -A 8 'default_ulimits'
  ```

  CRI-O `memlock` values are bytes. Apply runtime changes during a
  maintenance window and recreate existing database containers to inherit
  the new limits. Use the corresponding supported configuration for
  other container runtimes.

  Restarting kubelet is needed when refreshing a changed HugePages pool;
  it does not apply new runtime memory-lock limits to existing containers.

  **Verify database use**

  Configure the database to use large pages, with AMM disabled. Do not
  configure `spec.initParams.memoryTarget` with HugePages.

  After startup, inspect the latest page-allocation table in the database
  alert log. A positive `ALLOCATED_PAGES` value for the `2048K` row confirms
  HugePages use. Check that the allocation matches the expected page
  count and that the row reports no allocation errors.

  A Kubernetes HugePages request or limit alone does not confirm that
  Oracle allocated its SGA from HugePages.

  For worker selection and optional taints, see
  [Pod Placement](../README.md#pod-placement).

* ## Secret Management

  Create every Secret in the same namespace as the resource that references it
  (SIDB, True Cache SIDB, ORDS / OrdsSrvs, or another extension). A Secret in a
  different namespace is not visible to that Pod. Use the exact secret names and
  keys from the selected manifest.

  ```sh
  NS=<namespace-of-the-sidb-or-extension>
  ```

  ### Quick Start secrets

  For the SIDB [Quick Start](../README.md#quick-start), create only these secrets
  in that same `$NS`:

  - [Image pull secret](#image-pull-secret)
  - [Database admin secret](#database-admin-secret)

  Create the TDE and TCPS TLS secrets only when the selected manifest enables
  those features.

  ### Image pull secret

  Create this when your database image is hosted in a private registry such as
  Oracle Container Registry. Licensed OCR images used by True Cache / SIDB
  samples (`container-registry.oracle.com/database/enterprise_ru:latest` and
  other Enterprise paths) require authenticated pull: create `ocr-reg-cred` and
  set `spec.image.pullSecrets: ocr-reg-cred` on the SIDB. Anonymous pull of
  those images is not supported. Create `ocr-reg-cred` in **each** namespace
  where those SIDBs run; the pull Secret must exist in the same namespace as the
  SIDB Pod. See [Pull Images from Oracle Container Registry](../../../docs/registry/README.md).

  ```sh
  kubectl create secret docker-registry ocr-reg-cred \
    --docker-server=container-registry.oracle.com \
    --docker-username='<oracle-username>' \
    --docker-password='<auth-token>' \
    -n "$NS"
  ```

  ### Database admin secret

  Create the admin password secret referenced by `spec.security.secrets.admin`,
  for example `db-admin-secret`, in the same namespace as the SIDB:

  Set the namespace before creating the Secret. The shipped SIDB and True Cache
  samples use `default`:

  ```sh
  export NS=default
  ```

  If the manifest uses another `metadata.namespace`, set `NS` to that value.
  For a remote-primary True Cache deployment, create the admin Secret in the
  primary namespace and in the True Cache namespace.

  ```sh
  kubectl create secret generic db-admin-secret \
    --from-literal=oracle_pwd='<database-password>' \
    -n "$NS"
  ```

  You can also start from the shipped sample:

  - [`../../../config/samples/sidb/singleinstancedatabase_secrets.yaml`](../../../config/samples/sidb/singleinstancedatabase_secrets.yaml)

  That sample also includes the alternate secret names used by the prebuilt,
  Express, and Free SIDB samples.

  <a id="tde-secret"></a>
  <a id="tde-secret-if-required"></a>

  ### TDE secret

  Create this when the selected SIDB or True Cache manifest includes
  `spec.security.secrets.tde`, for example `tde-wallet-secret` with key
  `tde_wallet_pwd`. **True Cache requires this secret** on both the primary
  (when it is operator-managed) and the True Cache SIDB. README and workflow
  pages link here only for those scenarios.

  Set the namespace before creating the Secret. The shipped SIDB and True Cache
  samples use `default`:

  ```sh
  export NS=default
  ```

  If the manifest uses another `metadata.namespace`, set `NS` to that value.
  For a remote-primary True Cache deployment, run this command once in the
  primary namespace and once in the True Cache namespace.

  ```sh
  kubectl create secret generic tde-wallet-secret \
    --from-literal=tde_wallet_pwd='<tde-wallet-password>' \
    -n "$NS"
  ```

  TDE and TCPS are separate setup concerns.

  <a id="tcps-tls-secret"></a>
  <a id="tcps-tls-secret-if-required"></a>

  ### TCPS TLS secret

  TCPS is the TLS-encrypted Oracle listener path (port `2484` in the samples).
  README and workflow pages link here only for TCPS scenarios; TCP-only paths
  do not need this Secret.

  This Secret is a `kubernetes.io/tls` certificate/key pair. It is **not** the
  database admin password Secret and **not** the TDE wallet password Secret.
  Create it when the manifest sets `spec.security.tcps.enabled: true` and
  `spec.security.tcps.tlsSecret`.

  Set the namespace before creating the Secret. The shipped SIDB and True Cache
  samples use `default`:

  ```sh
  export NS=default
  ```

  If the manifest uses another `metadata.namespace`, set `NS` to that value.
  For a remote-primary True Cache deployment, create the TLS Secret in each
  namespace that presents a TCPS certificate.

  Sample secret names:

  | SIDB role | Typical `tlsSecret` name |
  | --- | --- |
  | Primary | `sidb-primary-tcps-tls` or `sidb-tcps-tls` |
  | Standby | `sidb-standby-tcps-tls` |
  | True Cache | `sidb-truecache-tcps-tls` (or share the primary secret when SANs cover both names) |

  Create the TLS Secret in **each** SIDB namespace that presents a certificate.
  Use a dedicated secret per SIDB when certificate SANs differ; same-namespace
  peers may share one secret when the SANs cover every presented hostname.

  > [!IMPORTANT]
  >
  > Create or issue the TLS Secret before applying the SIDB that references it.
  > If clients or peer databases connect by hostname, the certificate SANs must
  > match those hostnames. When the certificate chain includes intermediate
  > CAs, `tls.crt` must contain the full chain (leaf, then intermediates, then
  > root as required by your PKI).

  Follow this order:

  1. Choose private PKI or cert-manager (below).
  2. Create the TLS Secret in `$NS`.
  3. Confirm the Secret type is `kubernetes.io/tls`.
  4. Set `spec.security.tcps.enabled: true` and `spec.security.tcps.tlsSecret`
     to that Secret name in the SIDB YAML.
  5. Enable a TCPS service endpoint on port `2484` (see
     [`README.md` — Enable TCPS](../README.md#enable-tcps)).
  6. Apply the SIDB only after steps 1–5 are done.

  **Private PKI (manual Secret):** after the certificate and private key files
  are ready, create a `kubernetes.io/tls` Secret in the SIDB namespace:

  ```sh
  NS=<operator-watched-namespace>
  kubectl create secret tls sidb-primary-tcps-tls \
    --cert=/path/to/tls.crt \
    --key=/path/to/tls.key \
    -n "$NS"
  ```

  Confirm the Secret type is `kubernetes.io/tls` before applying the SIDB:

  ```sh
  kubectl get secret sidb-primary-tcps-tls -n "$NS" \
    -o jsonpath='{.type}{"\n"}'
  ```

  **cert-manager:** follow the [TCPS cert-manager
  procedure](../tcps-cert-manager/README.md). cert-manager creates the TLS
  Secret; do not recreate it with `kubectl create secret tls`. Wait until the
  Certificate is `Ready=True`, then confirm the referenced Secret is type
  `kubernetes.io/tls` before applying the SIDB:

  ```sh
  NS=<operator-watched-namespace>
  kubectl get certificate <certificate-name> -n "$NS"
  kubectl describe certificate <certificate-name> -n "$NS"
  kubectl get secret <tls-secret-name> -n "$NS" \
    -o jsonpath='{.type}{"\n"}'
  ```

  <a id="ords-password-secret"></a>
  <a id="ords-password-secret-if-required"></a>

  ### ORDS password secret

  Create this when deploying Oracle REST Data Services and the ORDS manifest
  references `ords-secret`. README and ORDS pages link here only for those
  legacy deployments. Use the same namespace as the ORDS resource. In the
  ORDS Quick Start, this is the namespace created earlier:

  ```sh
  export NS=ordsnamespace
  ```

  If `$NS` is already set by the Quick Start, reuse that value; do not create
  another namespace.

  ```sh
  kubectl create secret generic ords-secret \
    --from-literal=oracle_pwd='<ords-password>' \
    -n "$NS"
  ```

  Create this Secret in the same namespace as the ORDS / OrdsSrvs resource (which
  may differ from the SIDB namespace when ORDS runs elsewhere).

  You can also start from the shipped ORDS secret sample, which uses the same
  `ordsnamespace` example namespace:

  - [`../../../config/samples/sidb/oraclerestdataservice_secrets.yaml`](../../../config/samples/sidb/oraclerestdataservice_secrets.yaml)

  ### Validate secret references

  This section applies to ORDS resources only. Before applying an `OrdsSrvs`
  or legacy `OracleRestDataService` manifest, verify that every referenced
  Secret exists in the **same namespace as that ORDS resource**. The namespace
  may differ from the SIDB namespace when ORDS connects to a database in
  another namespace or to a remote database. The operator must watch the ORDS
  resource namespace.

  For `OrdsSrvs`, check the Secret references used by the manifest:

  - `spec.poolSettings[].db.secret.secretName` and `passwordKey`
  - `spec.poolSettings[].db.adminUser.secret.secretName` and `passwordKey`
    when ORDS or APEX auto-upgrade is enabled
  - `spec.encPrivKey.secretName` when encrypted credentials are configured
  - `spec.imagePullSecrets` when an image-pull Secret is configured

  For the ORDS Quick Start, verify the combined credentials Secret in the
  namespace created for `OrdsSrvs`:

  ```sh
  ORDS_NS=ordsnamespace
  kubectl get secret ordssrvs-auth -n "$ORDS_NS"
  ```

  For a legacy `OracleRestDataService` resource, verify `ords-secret` in the
  namespace declared by that resource:

  ```sh
  kubectl get secret ords-secret -n "$ORDS_NS"
  ```

  Do not use the `default` namespace unless the ORDS resource is deployed
  there and the operator is configured to watch it. The storage class in the
  manifest must also exist in the cluster.

* ## True Cache prerequisites

  This section is **association and helper detail**, not a second copy of the
  SIDB deploy checklist. Reading order:

  **Namespace for True Cache Secret commands:** The shipped in-cluster TCP and
  TCPS samples use `default` for both the primary and True Cache. Set this
  before opening a linked Secret instruction:

  ```sh
  export NS=default
  ```

  If you change `metadata.namespace`, set `NS` to that namespace and use it for
  the primary, True Cache, and their referenced Secrets. For a remote-primary
  path, set `NS` separately for the namespace in each cluster.

  | Path | Do this |
  | --- | --- |
  | In-cluster TCP | Finish [`README.md` — Before You Begin for True Cache](../README.md#before-you-begin-for-true-cache), then apply [`README.md` — True Cache Quick Start](../README.md#true-cache-quick-start). Keep `autoTCServiceRegistration: true`. Skip the subsections below unless association fails. |
  | In-cluster TCPS | Same Before You Begin, then [`README.md` — True Cache Setup with TDE and TCPS](../README.md#true-cache-setup-with-tde-and-tcps) |
  | Remote primary | Same Before You Begin, then [`README.md` — Create True Cache for a Remote Primary](../README.md#create-true-cache-for-a-remote-primary) |
  | Opened this page first | Go to [`README.md` — True Cache](../README.md#true-cache), complete Before You Begin, then pick Quick Start / TCPS / Remote from that Contents table — do not deploy from this page. |
  | External / non-OCR primary, or `autoTCServiceRegistration: false` | Use [Automatic path](#automatic-path-autotcserviceregistration-true), [Helper install](#true-cache-helper-install), [Manual PDB service](#create-the-primary-pdb-service-manually-advanced), and [Manual association](#manual-true-cache-service-association) below. |

  In-cluster Quick Start apply:

  1. Apply
     [`singleinstancedatabase_truecache_primary.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml)
     → wait Healthy + blob ConfigMap.
  2. Apply
     [`singleinstancedatabase_truecache.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml)
     directly with `autoTCServiceRegistration: true`. The OCR `enterprise_ru`
     image helper creates/starts the primary PDB service if needed — no manual
     `CREATE_SERVICE` before apply.
  3. Return to
     [`README.md` — True Cache Quick Start](../README.md#true-cache-quick-start)
     for waits and verify commands.

  True Cache requires [admin](#database-admin-secret) and [TDE](#tde-secret)
  secrets in each SIDB namespace, referenced as `spec.security.secrets.admin`
  and `spec.security.secrets.tde`.

  ### Primary PDB service

  `trueCache.trueCacheServices` uses
  `PRIMARY_PDB:PRIMARY_SERVICE:TRUECACHE_SERVICE` (for example
  `APPPDB1:tpdb_primary:tpdb_cache`). The middle value is the primary-side
  workload service. The third value is the True Cache service created during
  association.

  #### Automatic path (`autoTCServiceRegistration: true`)

  Prefer this for the in-cluster sample path. Set
  `spec.trueCache.autoTCServiceRegistration: true` on the True Cache SIDB
  (sample default). During provisioning, the True Cache workflow invokes
  [`configure-primary-truecache-service.sh`](https://github.com/oracle/docker-images/blob/main/OracleDatabase/SingleInstance/extensions/truecache/configure-primary-truecache-service.sh)
  on the primary through `DBMS_SCHEDULER`. That helper:

  1. Creates and starts the primary PDB service if it is missing.
  2. Associates that primary service with the True Cache service.

  You do not need to run `DBMS_SERVICE.CREATE_SERVICE` yourself on this path.
  Prefer this when the helper prerequisites below are met (OCR `enterprise_ru`
  SIDB primaries already ship the helper; external or non-OCR primaries must
  install it).

  Automatic association works for every True Cache path: **in-cluster SIDB**
  and **remote primary** (SIDB in another cluster, or external host SI or RAC).
  Whether you must install the helper on the primary depends on how that
  primary is hosted.

  ```yaml
  spec:
    trueCache:
      autoTCServiceRegistration: true
  ```

  <a id="true-cache-helper-install"></a>

  - **Helper path and when to install it**

    The default path used by the operator is
    `/home/oracle/configure-primary-truecache-service.sh`. The helper must exist
    at that path (or at `PRIMARY_TC_SERVICE_SCRIPT_PATH`) on every primary node
    where the scheduler job might run. For multi-node RAC, use the same path on
    each node.

    | Primary type | Install / copy the helper? |
    | --- | --- |
    | Primary SIDB using the OCR SIDB image with True Cache support (in-cluster or remote primary), for example `image.pullFrom: container-registry.oracle.com/database/enterprise_ru:latest` as in the True Cache primary samples | **No.** That image already prebakes `/home/oracle/configure-primary-truecache-service.sh`. Confirm it is present and executable; do not copy it again. |
    | External host SI or RAC, or any primary that does not use that OCR SIDB image | **Yes.** Copy the [sample script](https://github.com/oracle/docker-images/blob/main/OracleDatabase/SingleInstance/extensions/truecache/configure-primary-truecache-service.sh) onto every primary node at the path above (or your custom path). |

    Optional custom path (mainly external / non-OCR): if you install the helper
    somewhere other than the default path on the primary host, set
    `PRIMARY_TC_SERVICE_SCRIPT_PATH` on the **True Cache** SIDB through top-level
    `spec.envVars` (not under `spec.trueCache`). The operator passes that env var
    into the True Cache container; the image uses it as the path
    `DBMS_SCHEDULER` must execute on the primary. OCR `enterprise_ru` primaries
    that keep the prebaked default path do not need this override.

    ```yaml
    spec:
      envVars:
        - name: PRIMARY_TC_SERVICE_SCRIPT_PATH
          value: /custom/path/configure-primary-truecache-service.sh
    ```

  - **Keep the script owned by the Oracle software owner and executable**
    (for example `chown oracle:oinstall` and mode `755`, or `750`). The helper
    must be runnable by the same OS user that `DBMS_SCHEDULER` will use.

  - **Verify `$ORACLE_HOME/rdbms/admin/externaljob.ora`** in the primary DB home
    runs external jobs as that Oracle software owner when no scheduler
    credential is used. For example:

    ```text
    run_user = oracle
    run_group = oinstall
    ```

    Oracle's default is often `run_user = nobody` / `run_group = nobody`. That
    configuration can fail for automatic registration even when the helper
    works in an interactive `oracle` shell. If you set
    `trueCache.primarySchedulerCredentialName`, the named credential's OS user
    is what matters instead; that user must still own or be able to execute the
    helper and `dbca`.

  - **Smoke-test the scheduler** so a real executable job runs as the Oracle DB
    software owner before you rely on automatic registration. Checking
    `externaljob.ora` alone is not enough.

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
      -- SI quick check: use_current_session => TRUE
      -- RAC / closer to auto-registration: use_current_session => FALSE
      -- (True Cache auto-reg uses TRUE for non-RAC primaries and FALSE for RAC.)
      DBMS_SCHEDULER.RUN_JOB('EXTJOB_ID_TEST', use_current_session => FALSE);
    END;
    /
    ```

    Then verify on the node where the job ran:

    ```bash
    cat /tmp/extjob_id_test.out
    ```

    Expected output includes the Oracle DB software owner, for example
    `uid=... (oracle)`. If the file shows any other OS user (for example
    `nobody` or `grid`), fix the scheduler runtime before relying on automatic
    registration.

  #### Create the primary PDB service manually (advanced)

  Use this when you want a dedicated primary application service before deploy,
  need to validate the exact listener-registered name (RAC / `db_domain`), or
  when automatic association is off. The middle `trueCacheServices` value must
  match the service you create. Sample names such as `APPPDB1` and
  `tpdb_primary` are examples only.

  On an operator-managed primary:

  ```sh
  PRIMARY_POD=$(kubectl get pod -n "$NS" -l "app=$PRIMARY" \
    -o jsonpath='{.items[0].metadata.name}')
  kubectl exec -n "$NS" -it "$PRIMARY_POD" -- bash -lc 'sqlplus / as sysdba'
  ```

  ```sql
  ALTER SESSION SET CONTAINER=APPPDB1;
  EXEC DBMS_SERVICE.CREATE_SERVICE(service_name => 'tpdb_primary', network_name => 'tpdb_primary');
  EXEC DBMS_SERVICE.START_SERVICE('tpdb_primary');
  ALTER SYSTEM REGISTER;
  ```

  Confirm the service is advertised (`lsnrctl status LISTENER`). For RAC or
  `db_domain`, use the exact listener-registered name. Do not create the True
  Cache service (third `trueCacheServices` value) on the primary before
  association; that can fail with `ORA-16000`.

  #### Manual True Cache service association

  If automatic association is disabled or unavailable
  (`autoTCServiceRegistration` is `false` or omitted), create and start the
  primary PDB service first (or let a later manual helper run create it), then
  run the primary-host association command after True Cache is created.

  After the True Cache SIDB reports `DATABASE IS READY TO USE`, inspect the True
  Cache pod logs for the manual primary-host helper reminder. Search for the
  helper script name or the disabled auto-registration notice, for example:

  ```sh
  kubectl logs -n "$NS" -l "app=$TRUECACHE" --tail=500 \
    | grep -E 'configure-primary-truecache-service\.sh|AUTO_TC_SVC_REGISTRATION=false'
  ```

  The reminder prints a command in this shape (values come from your
  `trueCacheServices` mapping and the True Cache endpoint the image resolved):

  ```sh
  /home/oracle/configure-primary-truecache-service.sh \
    "<PRIMARY_SERVICE>" "<TRUECACHE_SERVICE>" "<PRIMARY_PDB>" \
    "<TC_HOST>:1521/<TC_CALLBACK_SERVICE>" \
    "<PRIMARY_DB_NAME>" "<PRIMARY_DB_UNIQUE_NAME>" "<PRIMARY_IS_RAC>" \
    '<PRIMARY_SYS_PASSWORD_OR_WALLET_SOURCE>'
  ```

  Confirm the endpoint and service names, then run that command on the primary
  host or primary pod as the Oracle software owner. Do not put the SYS password
  in this document or in shell history; use the password or wallet mechanism
  supported by the image.

  Then verify the mapped primary service shows the expected True Cache service
  name in `TRUE_CACHE_SERVICE`. See [Final verification](../truecache/TRUECACHE.md#final-verification-every-path)
  in [`TRUECACHE.md`](../truecache/TRUECACHE.md).

* ## SIDB Manifest Checklist

  Use this checklist with the current SIDB API:

  ```yaml
  apiVersion: database.oracle.com/v4
  kind: SingleInstanceDatabase
  ```

  The tables list every non-deprecated SIDB-specific `spec` field and every
  field in its custom child objects. Fields containing standard Kubernetes
  types (`metadata.labels`, `metadata.annotations`, `resources`,
  `securityContext`, `capabilities`, `hostAliases`, and `envVars`) accept the
  corresponding Kubernetes fields. Use only current fields in new manifests;
  see [Deprecated parameters](#deprecated-parameters) before updating an older
  manifest.

  Open a full sample YAML while you read the tables so you can see the fields
  in context. Start with the sample that matches your path:

  | Path | Full sample YAML |
  | --- | --- |
  | Primary (TCP) | [`singleinstancedatabase_create.yaml`](../../../config/samples/sidb/singleinstancedatabase_create.yaml) |
  | Primary (TCPS + TDE) | [`singleinstancedatabase_tcps_tde.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_tde.yaml) |
  | Data Guard standby (in-cluster) | [`singleinstancedatabase_standby.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby.yaml) |
  | True Cache (in-cluster TCP) | [primary](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml) · [True Cache](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml) |
  | True Cache (remote primary) | [`singleinstancedatabase_truecache_external.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml) |
  | Restore from backup | [`singleinstancedatabase_restore_from_backup_template.yaml`](../../../config/samples/sidb/singleinstancedatabase_restore_from_backup_template.yaml) |

  More samples (clone, Express/Free, NodePort/LB Data Guard, TCPS True Cache)
  live under [`config/samples/sidb/`](../../../config/samples/sidb/). The README
  [True Cache Sample Files](../README.md#true-cache-sample-files) table indexes
  every True Cache pair.

  ### Resource and database settings

  | YAML field | Description and checks |
  | --- | --- |
  | `metadata.name` | SIDB resource name; the operator also uses it when naming generated resources. |
  | `metadata.namespace` | Must be watched by the operator. Create every referenced Secret, ConfigMap, and PVC here. |
  | `metadata.labels`, `metadata.annotations` | Optional standard Kubernetes metadata for selection, policy, and tooling. |
  | `spec.edition` | Database edition: `standard`, `enterprise`, `express`, or `free`. Select a matching image. True Cache requires `enterprise`. |
  | `spec.sid` | Database SID: alphanumeric only, maximum 12 characters. The webhook supplies an edition-specific default when omitted. |
  | `spec.charset` | Database character set, for example `AL32UTF8`. |
  | `spec.pdbName` | Initial PDB name. The webhook supplies an edition-specific default when omitted. |
  | `spec.createAs` | Provisioning workflow: `primary`, `clone`, `standby`, or `truecache`. Set it explicitly; the default is `primary`. |
  | `spec.replicas` | SIDB supports exactly one database Pod. Set to `1` or omit it. |
  | `spec.flashBack` | Enables or disables Oracle Flashback Database. |
  | `spec.archiveLog` | Enables or disables ARCHIVELOG mode. Enable it for workflows requiring archived redo, including cloning and True Cache blob generation. |
  | `spec.forceLog` | Enables or disables FORCE LOGGING. |
  | `spec.convertToSnapshotStandby` | Requests physical-to-snapshot or snapshot-to-physical standby conversion. Use only for a supported standby workflow. |

  ### Image, Pod, and runtime settings

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.image.pullFrom` | Required database container image reference. The cluster must be able to pull it. |
  | `spec.image.version` | Optional image version metadata. Keep it aligned with the image tag. |
  | `spec.image.pullSecrets` | Image-pull Secret for a private registry. It must exist in `metadata.namespace`. |
  | `spec.image.imagePullPolicy` | Kubernetes pull policy: `Always`, `IfNotPresent`, or `Never`. |
  | `spec.image.prebuiltDB` | Set to `true` only when the image or mounted data already contains a database. It bypasses normal creation and wallet initialization paths. |
  | `spec.serviceAccountName` | Optional ServiceAccount for the SIDB Pod; use it for OpenShift or workload identity. |
  | `spec.automountServiceAccountToken` | Controls ServiceAccount token mounting. Defaults to `false`; enable only when the Pod requires it. |
  | `spec.nodeSelector` | Standard Kubernetes node-label selector for scheduling the SIDB Pod. |
  | `spec.tolerations[]` | Optional Kubernetes tolerations copied to the SIDB Pod; set them only for taints the SIDB must tolerate. |
  | `spec.hostAliases[]` | Standard Kubernetes host aliases. Each item contains `ip` and `hostnames[]`. |
  | `spec.resources` | Standard container requests and limits. For SGA/PGA memory validation, see [Database Pod Resources and Init Parameters](../README.md#database-pod-resources-and-init-parameters). HugePages require equal `hugepages-2Mi` requests and limits and completion of [Configure SIDB HugePages](#configure-sidb-hugepages). |
  | `spec.securityContext` | Standard Pod security context, including `runAsUser`, `fsGroup`, and `sysctls`. See [Configure SIDB Pod Kernel Sysctls](#configure-sidb-pod-kernel-sysctls). |
  | `spec.capabilities` | Standard Linux container capabilities to add or drop. Keep the set minimal. |
  | `spec.allowPrivilegeEscalation` | Controls privilege escalation in the database container. Defaults to `false`. |
  | `spec.shmSize` | Size of the memory-backed `/dev/shm` volume. Use a positive Kubernetes quantity, for example `2Gi`. |
  | `spec.readinessCheckPeriod` | Readiness and liveness probe interval in seconds. The operator uses 60 seconds when zero or omitted. |
  | `spec.disableDefaultDiagVolumeClaim` | Set to `true` to suppress the operator-created diagnostic PVC. |
  | `spec.envVars[]` | Additional Kubernetes environment variables. Each item uses `name` with either `value` or `valueFrom`; do not override operator-managed variables. |

  ### Storage, scripts, and database initialization

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.persistence.oradata.pvcName` | Existing PVC for database datafiles. The operator uses and preserves it. |
  | `spec.persistence.oradata.size` | Size for an operator-created `oradata` PVC, for example `100Gi`. |
  | `spec.persistence.oradata.storageClass` | StorageClass for dynamic `oradata` provisioning. It must exist in the cluster. |
  | `spec.persistence.oradata.accessMode` | Access mode for an operator-created `oradata` PVC: `ReadWriteOnce` or `ReadWriteMany`. |
  | `spec.persistence.fra.pvcName` | Existing PVC for the Fast Recovery Area (FRA). |
  | `spec.persistence.fra.size` | Size for an operator-created FRA PVC. |
  | `spec.persistence.fra.storageClass` | StorageClass for dynamic FRA provisioning. |
  | `spec.persistence.fra.accessMode` | Access mode for an operator-created FRA PVC: `ReadWriteOnce` or `ReadWriteMany`. |
  | `spec.persistence.fra.mountPath` | In-container mount path for the FRA PVC. |
  | `spec.persistence.fra.recoveryAreaSize` | Value applied to `db_recovery_file_dest_size` for FRA use cases. |
  | `spec.persistence.additionalPVCs[].mountPath` | Required in-container mount path for an additional PVC. |
  | `spec.persistence.additionalPVCs[].pvcName` | Existing additional PVC to mount. |
  | `spec.persistence.additionalPVCs[].storageSizeInGb` | Size in GiB for an operator-created additional PVC when `pvcName` is not set. |
  | `spec.persistence.additionalPVCs[].storageClass` | StorageClass for an operator-created additional PVC. |
  | `spec.persistence.retainOnDelete` | Preserves operator-created PVCs when the SIDB is deleted. User-provided PVCs are always preserved. |
  | `spec.persistence.volumeClaimAnnotation` | Annotation applied to operator-created PVCs; use the exact `key:value` format. |
  | `spec.persistence.setWritePermissions` | Adds an init container to set write permissions on SIDB storage mount paths. Enable only when required by the storage backend. |
  | `spec.scripts.setup.pvcName` | Existing PVC providing setup scripts; the operator mounts its `setup` directory. |
  | `spec.scripts.startup.pvcName` | Existing PVC providing startup scripts; the operator mounts its `startup` directory. |
  | `spec.initParams.sgaTarget` | Oracle SGA target as an integer in MiB; for example, `14336` represents 14GiB. Required explicitly when using HugePages. |
  | `spec.initParams.pgaAggregateTarget` | Oracle PGA aggregate target as an integer in MiB; for example, `4096` represents 4GiB. Required explicitly when using HugePages. |
  | `spec.initParams.memoryTarget` | Oracle AMM target. Do not combine it with `sgaTarget`, `pgaAggregateTarget`, or HugePages. Size regular container memory for the target and database overhead. |
  | `spec.initParams.cpuCount` | Oracle `CPU_COUNT` initialization parameter. |
  | `spec.initParams.processes` | Oracle `PROCESSES` initialization parameter. |
  | `spec.pdbParameters[].pdbName` | Target PDB for a PDB-level initialization parameter. |
  | `spec.pdbParameters[].name` | Oracle parameter name to apply to the target PDB. |
  | `spec.pdbParameters[].value` | Requested PDB-level parameter value. |

  ### Secrets and TCPS

  See these fields together in
  [`singleinstancedatabase_tcps_tde.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_tde.yaml)
  and the secret templates in
  [`singleinstancedatabase_secrets.yaml`](../../../config/samples/sidb/singleinstancedatabase_secrets.yaml).

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.security.secrets.admin.secretName` | Database admin-password Secret. Required for normal database creation and True Cache. |
  | `spec.security.secrets.admin.secretKey` | Admin-password key. Defaults to `oracle_pwd`. |
  | `spec.security.secrets.admin.keepSecret` | Preserves the admin Secret when the SIDB is deleted. Defaults to `true` when configured. |
  | `spec.security.secrets.admin.mountPath` | Optional in-container mount path for the admin Secret. |
  | `spec.security.secrets.admin.skipInitWallet` | Skips operator-managed admin wallet initialization. Use only when another workflow supplies credentials. |
  | `spec.security.secrets.tde.secretName` | Secret containing the TDE wallet password or standby wallet artifacts. |
  | `spec.security.secrets.tde.secretKey` | Key in the TDE Secret containing the password. **Required for 23ai+ standby TDE.** **Omit for Oracle Database 19c standby** (use `walletZipFileKey` only). Primary TDE creation still needs a password key. |
  | `spec.security.secrets.tde.mountPath` | Optional in-container mount path for the TDE Secret. |
  | `spec.security.secrets.tde.walletZipFileKey` | Key containing the zipped standby wallet artifact for TDE import. |
  | `spec.security.secrets.tde.walletRoot` | Destination Oracle wallet root for standby TDE wallet import and open operations. |
  | `spec.security.tcps.enabled` | Enables TCPS listener configuration. |
  | `spec.security.tcps.tlsSecret` | `kubernetes.io/tls` Secret containing `tls.crt` and `tls.key`. Create and validate it before applying the SIDB. |
  | `spec.security.tcps.clientWalletSecret` | Optional Secret that overrides the client wallet published for Data Guard TCPS consumers. |
  | `spec.security.tcps.certMountLocation` | In-container TLS Secret mount path. Defaults to `/run/secrets/tls_secret`. |

  ### Service exposure and TNS aliases

  The in-cluster Service always exposes TCP port 1521. Add an endpoint only
  when a NodePort, LoadBalancer, custom service settings, or TCPS exposure is
  required. Specify no more than three endpoints.

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.services.endpoints[].name` | Endpoint role: `cluster`, `nodeport`, or `loadbalancer`. Use `loadbalancer` instead of deprecated `services.external`. |
  | `spec.services.endpoints[].type` | Kubernetes Service type: `ClusterIP`, `NodePort`, or `LoadBalancer`. Match it to the endpoint name and platform. |
  | `spec.services.endpoints[].isKeep` | Preserves the Service across SIDB deletion and recreation by omitting its controller owner reference. |
  | `spec.services.endpoints[].annotations` | Annotations for the generated Service, such as cloud LoadBalancer settings. |
  | `spec.services.endpoints[].externalTrafficPolicy` | External traffic policy for NodePort or LoadBalancer: `Cluster` or `Local`. |
  | `spec.services.endpoints[].publishNotReadyAddresses` | Publishes endpoints before the Pod is Ready; use only when bootstrap clients must reach the listener. |
  | `spec.services.endpoints[].tcp.enabled` | Enables TCP on this endpoint. |
  | `spec.services.endpoints[].tcp.port` | TCP Service port for `ClusterIP` or `LoadBalancer`. |
  | `spec.services.endpoints[].tcp.nodePort` | Fixed TCP node port when `type: NodePort`. |
  | `spec.services.endpoints[].tcps.enabled` | Enables TCPS on this endpoint. Also enable `security.tcps.enabled` and provide a TLS Secret. |
  | `spec.services.endpoints[].tcps.port` | TCPS Service port for `ClusterIP` or `LoadBalancer`. |
  | `spec.services.endpoints[].tcps.nodePort` | Fixed TCPS node port when `type: NodePort`. |
  | `spec.tnsAliases[].name` | Alias name written to managed `tnsnames.ora`; it must start alphanumeric and can include `.`, `_`, and `-`. |
  | `spec.tnsAliases[].host` | Hostname or IP address for the alias target. |
  | `spec.tnsAliases[].port` | Listener port for the alias target. |
  | `spec.tnsAliases[].serviceName` | Database service name for the alias target. |
  | `spec.tnsAliases[].protocol` | Connection protocol: `TCP` or `TCPS`. |
  | `spec.tnsAliases[].sslServerDN` | Server DN for a TCPS alias when DN matching is configured. |

  ### Primary source

  Use this table when `createAs` is `clone`, `standby`, or `truecache`. Identify
  the source with `databaseRef` (in-cluster: same cluster and namespace) or `connectString`
  (remote primary: another cluster or external host).

  Full samples:
  [`databaseRef` (True Cache)](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml),
  [`databaseRef` (standby)](../../../config/samples/sidb/singleinstancedatabase_standby.yaml),
  [`connectString` (remote True Cache)](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml),
  [`connectString` (standby)](../../../config/samples/sidb/singleinstancedatabase_standby_connectstring.yaml).

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.primarySource.databaseRef` | SIDB name for a primary in the **same Kubernetes cluster and namespace**. Use it for in-cluster True Cache / standby / clone samples. Use it instead of deprecated `primaryDatabaseRef`. A remote primary must use `connectString`. |
  | `spec.primarySource.connectString` | Reachable connect string for a remote primary (clone, standby, or True Cache) in another cluster or on an external host. |
  | `spec.primarySource.tcpsPort` | TCPS port when `connectString` uses a NodePort or other external endpoint. Defaults to 2484 when needed. |
  | `spec.primarySource.dbName` | Primary CDB `DB_NAME` when it differs from the connect-string service name. |
  | `spec.primarySource.pdbName` | Primary PDB name when the source is expressed by `connectString`. |
  | `spec.primarySource.details.host` | External primary hostname or IP address. |
  | `spec.primarySource.details.port` | External primary listener port. |
  | `spec.primarySource.details.sid` | External primary CDB SID. |
  | `spec.primarySource.details.pdbName` | External primary PDB name. |

  ### Data Guard

  Use this table when configuring Data Guard on a primary or standby SIDB, or
  before creating a `DataguardBroker` resource. Also complete the shared SIDB
  secrets, storage, and image prerequisites above.

  Full samples:
  [primary with archive logging](../../../config/samples/sidb/singleinstancedatabase_create.yaml),
  [in-cluster standby](../../../config/samples/sidb/singleinstancedatabase_standby.yaml),
  [TCPS standby](../../../config/samples/sidb/singleinstancedatabase_standby_tcps.yaml).
  See also [Data Guard Quick Start](../README.md#data-guard-quick-start).

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.dataguard.mode` | Producer mode: `Disabled`, `Preview`, or `Managed`. The effective mode is `Preview` when omitted. |
  | `spec.dataguard.prereqs.enabled` | Enables local database prerequisites for Data Guard broker use. |
  | `spec.dataguard.prereqs.brokerConfigDir` | Absolute filesystem or ASM path for broker configuration files. |
  | `spec.dataguard.prereqs.standbyRedoSize` | Standby redo-log size, for example `200M` or `1G`. |
  | `spec.dataguard.authWallet.enabled` | Enables the optional broker authentication wallet lifecycle. |
  | `spec.dataguard.authWallet.passwordSecretRef.secretName` | User-managed authentication wallet password Secret. Omit it to let the operator generate one. |
  | `spec.dataguard.authWallet.passwordSecretRef.secretKey` | Key in the authentication wallet password Secret. |
  | `spec.dataguard.authWallet.rebuildToken` | Change this value to request a broker authentication wallet rebuild. |
  | `spec.dataguard.standbySources[].dbUniqueName` | Data Guard unique name for a standby peer. Configure it on the primary only when the standby source uses external `primarySource.connectString`. |
  | `spec.dataguard.standbySources[].host` | Externally reachable standby host for generated primary-side aliases. |
  | `spec.dataguard.standbySources[].tcpsEnabled` | Selects TCPS for the standby peer alias. |
  | `spec.dataguard.standbySources[].tcpPort` | Externally reachable TCP port for the standby peer. |
  | `spec.dataguard.standbySources[].tcpsPort` | Externally reachable TCPS port; the container port is used for in-cluster routes when omitted. |

  ### True Cache

  Use this table when authoring a True Cache primary or `createAs: truecache`
  SIDB. Also complete [True Cache prerequisites](#true-cache-prerequisites) and
  the [Primary source](#primary-source) fields that identify the primary.

  Full samples:
  [TCP primary](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml) ·
  [TCP True Cache](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml),
  [TCPS primary](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml) ·
  [TCPS True Cache](../../../config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml),
  [remote primary](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml).
  Full index: [True Cache Sample Files](../README.md#true-cache-sample-files).

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.trueCache.truedbUniqueName` | True Cache database unique name (`DB_UNIQUE_NAME`) for `createAs: truecache`. When omitted, the operator defaults to `<sid>_TC`. |
  | `spec.trueCache.generateBlob` | On a primary, creates the True Cache configuration blob in the Pod. |
  | `spec.trueCache.createConfigMap` | On a primary, creates or updates the ConfigMap that publishes the generated blob. |
  | `spec.trueCache.generatePath` | Primary Pod path for the generated blob or ConfigMap source. Defaults to `/tmp/tc_config_blob.tar.gz`; it requires blob generation or ConfigMap creation. |
  | `spec.trueCache.blobConfigMapRef` | On a True Cache consumer, ConfigMap containing the True Cache configuration blob. |
  | `spec.trueCache.blobConfigMapKey` | Key in `blobConfigMapRef` holding the blob. Defaults to `tc_config_blob.tar.gz`. |
  | `spec.trueCache.blobMountPath` | True Cache container path for the mounted blob. Defaults to `/stage/tc_config_blob.tar.gz`. |
  | `spec.trueCache.dbCredentialsWallet.secretName` | Secret containing the primary database credential wallet used by True Cache DBCA. |
  | `spec.trueCache.dbCredentialsWallet.mountPath` | Absolute path at which the primary credential wallet is mounted. |
  | `spec.trueCache.autoTCServiceRegistration` | When `true`, invokes the primary helper through `DBMS_SCHEDULER` to create/start the primary PDB service if needed and associate it with the True Cache service. See [Automatic path](#automatic-path-autotcserviceregistration-true). |
  | `spec.trueCache.primarySchedulerCredentialName` | Optional primary scheduler credential for automatic service registration. |
  | `spec.trueCache.trueCacheServices[]` | Required for `createAs: truecache`. Each mapping identifies the PDB, primary service, and True Cache service. This takes precedence over `spec.trueCacheServices`. |
  | `spec.trueCacheServices[]` | Compatibility service-mapping field for `createAs: truecache`. Use `spec.trueCache.trueCacheServices` in new manifests. |

  ### Restore from backup

  `spec.restore` is supported only with `createAs: primary`. Choose exactly one
  source: `objectStore` or `fileSystem`.

  Full samples:
  [template](../../../config/samples/sidb/singleinstancedatabase_restore_from_backup_template.yaml),
  [object-store CDB](../../../config/samples/sidb/singleinstancedatabase_restore_cdb_objectstore.yaml),
  [filesystem CDB](../../../config/samples/sidb/singleinstancedatabase_restore_cdb_filesystem.yaml).
  See also [Restore Database from RMAN Backup](../restore/RESTORE_DATABASE_FROM_RMAN_BACKUP.md).

  | YAML field | Description and checks |
  | --- | --- |
  | `spec.restore.objectStore.ociConfig.{configMapName,key}` | ConfigMap name and key containing the OCI configuration. |
  | `spec.restore.objectStore.privateKey.{secretName,key}` | Secret name and key containing the OCI API private key. |
  | `spec.restore.objectStore.sourceDbWallet.{secretName,key}` | Secret name and key containing the source database wallet. |
  | `spec.restore.objectStore.sourceDbWalletPassword.{secretName,key}` | Secret name and key containing the source database wallet password. |
  | `spec.restore.objectStore.backupModuleConfig.{configMapName,key}` | ConfigMap name and key containing backup-module configuration. |
  | `spec.restore.objectStore.opcInstallerZip.{configMapName,key}` | ConfigMap name and key containing the backup module installer ZIP. |
  | `spec.restore.objectStore.backupIdentity.bucketName` | Object Storage bucket containing the backup. |
  | `spec.restore.objectStore.backupIdentity.dbid` | DBID of the source database. |
  | `spec.restore.objectStore.backupIdentity.compartmentOcid` | OCI compartment OCID for the backup source. |
  | `spec.restore.objectStore.encryptedBackup.enabled` | Indicates that the object-store backup is encrypted. |
  | `spec.restore.objectStore.encryptedBackup.decryptPasswordSecret.{secretName,key}` | Secret name and key containing the encrypted object-store backup password. |
  | `spec.restore.fileSystem.backupPath` | In-container path to the filesystem backup. |
  | `spec.restore.fileSystem.catalogStartWith` | RMAN `CATALOG START WITH` path used to locate filesystem backup pieces. |
  | `spec.restore.fileSystem.sourceDbWallet.{secretName,key}` | Secret name and key containing the source database wallet. |
  | `spec.restore.fileSystem.sourceDbWalletPassword.{secretName,key}` | Secret name and key containing the source database wallet password. |
  | `spec.restore.fileSystem.encryptedBackup.enabled` | Indicates that the filesystem backup is encrypted. |
  | `spec.restore.fileSystem.encryptedBackup.decryptPasswordSecret.{secretName,key}` | Secret name and key containing the encrypted filesystem backup password. |
  | `spec.restore.target.dataRoot` | Restored database data root override. |
  | `spec.restore.target.walletRoot` | Restored database wallet root override. |
  | `spec.restore.options.sourceDbName` | Source database name used by the restore workflow. |
  | `spec.restore.options.runCrosscheck` | Requests an RMAN crosscheck during restore processing. |
  | `spec.restore.options.runValidateOnly` | Validates the backup without completing database creation. |
  | `spec.restore.options.forceOpcReinstall` | Forces Oracle backup module reinstallation for object-store restore. |

  ### Deprecated parameters

  These fields remain accepted for backward compatibility but must not be used
  in new manifests. Migrate existing manifests to the current fields below.

  | Deprecated field | Replace with / action |
  | --- | --- |
  | `spec.loadBalancer` | Use `spec.services.endpoints[]` with a `loadbalancer` endpoint. |
  | `spec.listenerPort` | Use `spec.services.endpoints[].tcp`. |
  | `spec.tcpsListenerPort` | Use `spec.services.endpoints[].tcps`. |
  | `spec.serviceAnnotations` | Use `spec.services.endpoints[].annotations`. |
  | `spec.services.external` | Use `spec.services.endpoints[]` with `name: loadbalancer`. |
  | `spec.enableTCPS` | Use `spec.security.tcps.enabled`. |
  | `spec.tcpsTlsSecret` | Use `spec.security.tcps.tlsSecret`. |
  | `spec.tcpsCertRenewInterval` | Do not replace it. TLS certificate renewal is managed by the TLS Secret owner. |
  | `spec.security.tcps.certRenewInterval` | Do not use it. TLS certificate renewal is managed by the TLS Secret owner. |
  | `spec.primaryDatabaseRef` | Use `spec.primarySource.databaseRef`. |
  | `spec.adminPassword` | Use `spec.security.secrets.admin`. |
  | `spec.resourceRequirements` | Use `spec.resources`. |
  | `spec.image.pullPolicy` | Use `spec.image.imagePullPolicy`. |
  | `spec.persistence.size` | Use `spec.persistence.oradata.size`. |
  | `spec.persistence.storageClass` | Use `spec.persistence.oradata.storageClass`. |
  | `spec.persistence.accessMode` | Use `spec.persistence.oradata.accessMode`. |
  | `spec.persistence.datafilesVolumeName` | Use `spec.persistence.oradata.pvcName`. |
  | `spec.persistence.scriptsVolumeName` | Use `spec.scripts.setup.pvcName` and `spec.scripts.startup.pvcName`. |
  | `spec.trueCache.generateEnabled` | Use `spec.trueCache.generateBlob` and `spec.trueCache.createConfigMap` independently. |
