# ExternalDNS for Shared Private OCI DNS

These manifests are a sanitized, reusable version of the `dns-working-files*` setup used for SIDB and True Cache private hostnames.

They configure `external-dns` on an OKE cluster with OCI IAM workload identity so an internal `LoadBalancer` Service annotated with `external-dns.alpha.kubernetes.io/hostname` publishes a private DNS record into an OCI private zone.

The intended topology is a single authoritative shared private zone, for example `internal.example.com`, that both the Ashburn and Mumbai clusters publish into. Use the same manifest set once per cluster, but give each cluster a distinct `--txt-owner-id`.

Before applying it, update:

- `docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml`
  - `auth.region`
  - `compartment`
- `docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml`
  - `--txt-owner-id`
  - `--domain-filter`
  - `--zone-id-filter`

Notes:

- `auth.region` must be the OCI DNS API region that owns the private zone you publish into. When both clusters publish into a shared Ashburn-hosted zone, this remains `us-ashburn-1` on both clusters.
- `--zone-id-filter` should point to the same shared zone on both clusters when you use one authoritative shared zone.
- Keep `--txt-owner-id` stable for the life of a cluster and distinct per cluster, for example `sidb-ashburn` and `sidb-mumbai`.
- The old build-side `05-*service.yaml` examples are intentionally not copied here because SIDB now renders the external `*-ext` LoadBalancer Service through `spec.services.external`.

Apply order:

```sh
kubectl apply -f docs/sidb/external-dns/01-namespace-serviceaccount.yaml
kubectl apply -f docs/sidb/external-dns/02-rbac.yaml
kubectl apply -f docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml
kubectl apply -f docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml
```

DNS cleanup helper:

- Use [`cleanup-dns.sh`](./cleanup-dns.sh) to inspect or delete stale `A` and ExternalDNS `TXT` records.
- By default it checks the shared zone only. Set `LEGACY_ZONE_ID` if you also want to clean a previous local zone during migration from separate zones to a shared zone.

Examples:

```sh
bash ./docs/sidb/external-dns/cleanup-dns.sh --check both
bash ./docs/sidb/external-dns/cleanup-dns.sh --apply both
LEGACY_ZONE_ID=ocid1.dns-zone.oc1.ap-mumbai-1.replace-me bash ./docs/sidb/external-dns/cleanup-dns.sh --apply truecache
```

Cleanup notes:

- Delete or unannotate the `*-ext` Service first, or stop `external-dns`, otherwise the DNS record can be recreated.
- For shared-zone deployments, point `SHARED_ZONE_ID` at the authoritative shared zone. Do not clean only the old Mumbai zone and assume the record is gone.
- ExternalDNS ownership records are stored as `TXT` rrsets named `a-<hostname>`, so stale `TXT` records should be cleaned alongside the `A` records.

IAM policy guidance:

- Run OCI IAM policy updates in the tenancy home region.
- Prefer a workload-identity-scoped policy tied to the `external-dns` service account and cluster OCID.
- Keep `txt-owner-id` stable for the life of a cluster.
- If two clusters publish into the same shared zone, grant both cluster principals permission to manage DNS in the target compartment.

Example policy shape:

```text
Allow any-user to manage dns in compartment <dns-compartment-name>
where all {
  request.principal.type = 'workload',
  request.principal.namespace = 'external-dns',
  request.principal.service_account = 'external-dns',
  request.principal.cluster_id = '<oke-cluster-ocid>'
}
```

Validation:

```sh
kubectl logs -n external-dns deploy/external-dns
kubectl get svc -A
oci dns record rrset get \
  --zone-name-or-id <private-zone-ocid> \
  --domain <host>.internal.example.com \
  --rtype A \
  --scope PRIVATE
```

For a shared-zone customer handoff, validate all of the following:

```sh
kubectl get svc -n default
kubectl get svc -n default orcl-production-ext truecache-production-ext
oci dns record rrset get --zone-name-or-id <shared-zone-ocid> --domain orcl-production.internal.example.com --rtype A --scope PRIVATE
oci dns record rrset get --zone-name-or-id <shared-zone-ocid> --domain truecache-production.internal.example.com --rtype A --scope PRIVATE
kubectl exec -n default <primary-pod> -- nslookup truecache-production.internal.example.com
kubectl exec -n default <truecache-pod> -- nslookup orcl-production.internal.example.com
```

Expected results:

- each `*-ext` Service exists and has an internal load balancer IP
- both FQDNs exist in the same shared private zone
- the primary pod resolves `truecache-production.internal.example.com`
- the truecache pod resolves `orcl-production.internal.example.com`
