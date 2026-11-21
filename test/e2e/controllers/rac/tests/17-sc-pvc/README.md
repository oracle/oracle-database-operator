# rac/test17 — Storage Class ASM + PVC software (first implementation target)

Maps to docs use case **#17**: `docs/rac/provisioning/provisioning_oracle_rac_storageclass_pvc_softwares.md`.

User-facing sample YAML: `docs/rac/provisioning/racdb_prov_sc_pvc.yaml`.
The E2E case uses the separate placeholder template `test17.yaml`, renders it
from environment variables, validates it with `kubectl apply --dry-run=server`,
and then applies the rendered manifest. This keeps the E2E flow consistent with
the SIDB and Oracle Restart test runners without changing the directly usable
documentation sample.

**Status:** implemented. The test supports `E2E_TARGET_ENV=sarac1` or an
equivalent OpenShift environment with the documented RAC prerequisites. The
current OpenShift validation uses `E2E_TARGET_ENV=ldboper`.

First real implementation priority is **test17** (SC + PVC).
