# Use-case Makefile Parameters

`parameters.txt` supplies environment-specific values to `makefile` in this
directory. The makefile reads the file at parse time, then uses the values to
generate manifests, create secrets, and run the multitenant use cases.

## File format and safety

Use one `KEY:value` entry per line. Keys are case-sensitive. Do not add spaces
around the colon: the makefile does not trim values. Lines beginning with `#`
are comments.

The file contains database, registry, and OCI registry credentials. Keep it
local, restrict its file permissions, and replace the sample passwords before
using it. `make check` prints several credential values, so run it only in a
trusted terminal.

## Required environment settings

| Parameter | Values / format | Used for |
| --- | --- | --- |
| `LRESTIMG` | Full LREST container image reference | LREST Pod image. |
| `OPRIMG` | Full operator container image reference | Operator deployment image. |
| `TNSTOPOLOGY` | `FULLDESC` or `TNSNAMES` | Selects direct `dbTnsurl` or the `tnsnames.ora` ConfigMap configuration. |
| `TNSSTRING` | CDB connect descriptor | Used when `TNSTOPOLOGY:FULLDESC` and `USESIDB:FALSE`. |
| `TNSALIAS` | TNS alias | Used with the `TNSNAMES` topology. |
| `USESIDB` | Exact uppercase `TRUE` or `FALSE` | When `TRUE`, overrides `TNSSTRING` with `sidb-sample.$(SIDNAMESPACE):1521/ORCL1`. |
| `PASSPROTECTION` | `NATIVE`, `OPENSSL3`, or `ORAPKI` | Selects the credential-secret and wallet creation flow. |
| `DBUSER` | CDB lifecycle-management user | Creates the LREST database-user credential. |
| `DBPASS` | Password for `DBUSER` | Creates the LREST database-password credential. Treat as a secret. |
| `PDBUSR` | PDB administrator user | Creates the PDB administrator credential. |
| `PDBPWD` | Password for `PDBUSR` | Creates the PDB administrator password credential. Treat as a secret. |
| `PDBNAMESPACE` | Kubernetes namespace name | Namespace for LRPDB resources and PDB-related secrets. |
| `LRSNAMESPACE` | Kubernetes namespace name | Namespace for LREST resources and LREST-related secrets. |
| `SIDNAMESPACE` | Kubernetes namespace name | Namespace for the optional SingleInstanceDatabase (SIDB). |
| `APIVERSION` | Database API version, for example `v4` | `apiVersion` in generated LREST and LRPDB manifests. |
| `CODE_TREE` | Absolute path to this operator checkout | Used by `opsetup` to run `scripts/generate-namespace-install.sh`. |
| `CERT_MANAGER` | cert-manager manifest URL or local path | Applied and deleted by the cert-manager targets. |

## Manifest and runtime options

| Parameter | Values / format | Used for |
| --- | --- | --- |
| `FILENAMECONVERSION` | `NONE` or an Oracle data-file directory | Generates `fileNameConversions` for create and clone LRPDB manifests. Use `NONE` for ASM. |
| `SERVICENAMEACCOUNT` | Kubernetes ServiceAccount name | Written to the LREST manifest only when `OPENSHIFT:true`. |
| `OPENSHIFT` | Lowercase `true` or another value | Enables ServiceAccount assignment when exactly `true`. |
| `PLSQLMAP` | ConfigMap name | Referenced by the SQL/PLSQL ConfigMap test targets. |
| `AUTODISCOVER` | Kubernetes boolean such as `true` or `false` | Sets `spec.autodiscover` in the generated LREST manifest. |
| `LRESTTRC` | Integer bitmask | LREST trace level in generated manifests. |
| `LRPDBTRC` | Integer bitmask | LRPDB trace level in generated manifests. |
| `OCRU` | Oracle Container Registry user name or email | Used by `sidbsecrets` to create the SIDB image-pull secret. |
| `OCRP` | Oracle Container Registry password or token | Used by `sidbsecrets`. Treat as a secret. |

## Entries not currently consumed as expected

| Parameter | Current behavior |
| --- | --- |
| `COMPANY` | Parsed from `parameters.txt`, then overwritten by `COMPANY=oracle` in `makefile`; changing it currently has no effect. |

## Typical configuration

```text
TNSTOPOLOGY:FULLDESC
USESIDB:FALSE
TNSSTRING:(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=db.example)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=ORCL)))
TNSALIAS:orcl
PDBNAMESPACE:pdbnamespace
LRSNAMESPACE:cdbnamespace
SIDNAMESPACE:sidbnamespace
PASSPROTECTION:NATIVE
FILENAMECONVERSION:NONE
APIVERSION:v4
AUTODISCOVER:true
```

For an in-cluster SIDB, set `USESIDB:TRUE`; the makefile derives the connect
string from `SIDNAMESPACE`, so `TNSSTRING` is not used.

## Before running targets

1. Replace all sample passwords, registry tokens, image references, and connect
   details.
2. Verify that the three namespaces do not unintentionally overlap.
3. Confirm that `CODE_TREE` points to the checkout containing the operator
   installation scripts.
4. Use `make -n -f makefile <target>` to inspect an expanded command without
   applying Kubernetes changes.
