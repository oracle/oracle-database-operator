# DBCS provisioning E2E

DBCS E2E runs one real `DbcsSystem` **create, bind, then terminate** cycle on
the OKE cluster selected by the protected environment. It follows the Flex
sample shape used in
`docs/dbcs/provisioning/dbcs_service_with_pdb.yaml`.

Run this case through the generic environment-scoped operator E2E workflow:

```text
RUN_OPERATOR_E2E_ON_DEMAND=true
E2E_TARGET_ENV=saokedbcs
E2E_TEST_PRESET=dbcs/test01
```

The protected variables scoped to `e2e/saokedbcs` provide the OCI configuration,
kubeconfig/context, Bastion tunnel values, DBCS OCI identity, database
settings, and DBCS SSH public-key file. These values remain in protected CI/CD
variables rather than this repository.

The job uses the stable `dbcs-ns` namespace. The shared operator E2E setup
includes `dbcs-ns` in the operator watch list and pre-cleans stale namespaced
resources there before installing the operator.

## What the test does

1. Opens the shared Bastion SSH tunnel and installs or reuses the operator with
   `WATCH_NAMESPACE` including the E2E namespace.
2. Creates `oci-cred`, `oci-privatekey`, `admin-password`, and `oci-publickey`.
3. Renders and server-validates the `DbcsSystem` YAML.
4. **Create:** applies the soft-link CR and waits for `status.state=AVAILABLE`
   with a non-empty `status.id` (OCI DB System appears in Ashburn).
5. **Bind for termination:** renders and applies a separate termination CR with
   the captured OCID and `hardLink: true`, matching the documented workflow.
6. **Terminate:** deletes the termination CR from the rendered YAML, then waits
   for that exact OCI DB system to reach lifecycle state `TERMINATED`.

Create and delete are asserted in order in the same job so weekly runs do not
accumulate DB systems.

For a recurring run, configure the CI schedule or API with the same
variables. Use `E2E_CLEANUP_POLICY=both` so the run performs create and
termination in one job.

## Required protected/file variables

- `KUBE_CONTEXT_FILE` and `KUBE_CONTEXT_NAME`
- `BASTION_*` tunnel variables, including `BASTION_SSH_PRIVATE_KEY_FILE` or an
  available `SSH_AUTH_SOCK`
- `OCI_CONFIG_FILE` and `OCI_API_PRIVATE_KEY_FILE`
- DBCS `DBCS_*` provisioning values and `DBCS_SSH_PUBLIC_KEY_FILE`

## Defaults

The DBCS test still requires the same provisioning values as the previous
workflow, but they are now supplied by the selected protected environment.
No cluster endpoint, Bastion address, OCI identifier, password, or key path is
defined in this repository.
