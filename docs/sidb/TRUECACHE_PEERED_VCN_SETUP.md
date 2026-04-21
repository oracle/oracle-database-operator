# True Cache Peered VCN Setup

This document covers the missing infrastructure pieces around the SIDB True Cache samples when the primary and True Cache databases run in different OKE clusters or regions and communicate over peered VCNs with private DNS and TCPS.

It is based on the working `dns-working-files*` and `tcps-cert-manager` flows, but converted into reusable repo-native samples and scripts.

## Components

The complete setup has four moving parts:

1. Primary SIDB manifest with True Cache blob generation, TCPS enabled, and an operator-managed internal NLB.
2. True Cache SIDB manifest with an operator-managed internal NLB.
3. ExternalDNS on each cluster so both operator-managed NLB Services publish into the same authoritative OCI private DNS zone.
4. cert-manager flow to issue the primary and True Cache TCPS certificates.

## SIDB Samples

Primary side:

- [`config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml)

True Cache side:

- [`config/samples/sidb/singleinstancedatabase_truecache_external.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml)

These two SIDB manifests render the private NLB pair through `spec.services.external`:

- Primary external Service: `orcl-production-ext`
- True Cache external Service: `truecache-production-ext`
- Primary DNS hostname: `orcl-production.internal.example.com`
- True Cache DNS hostname: `truecache-production.internal.example.com`

## ExternalDNS Setup

Use the manifests in [`docs/sidb/external-dns`](./external-dns).

Apply the same manifest set on each cluster, but customize the OCI DNS API region, DNS compartment, `txt-owner-id`, and shared zone ID before applying:

```sh
kubectl apply -f docs/sidb/external-dns/01-namespace-serviceaccount.yaml
kubectl apply -f docs/sidb/external-dns/02-rbac.yaml
kubectl apply -f docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml
kubectl apply -f docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml
```

Important notes:

- Use a distinct `txt-owner-id` per cluster.
- Keep `txt-owner-id` stable for the life of a cluster.
- Prefer workload-identity-scoped IAM policies tied to the `external-dns` service account and cluster OCID.
- Point both clusters at the same shared zone ID when you want one authoritative private zone.
- If DNS forwarding or zone association is not ready yet, `hostAliases` in the SIDB manifests can bridge name resolution temporarily.
- If you need to remove stale records during migration or cleanup, use `docs/sidb/external-dns/cleanup-dns.sh` so you clean both the `A` rrset and the ExternalDNS `TXT` owner rrset.

## TCPS Certificate Setup with cert-manager

Use the scripts in [`docs/sidb/tcps-cert-manager`](./tcps-cert-manager).

Typical flow:

```bash
export PRIMARY_CTX=context-primary
export STANDBY_CTX=context-truecache
export NS=default

./docs/sidb/tcps-cert-manager/run-all.sh
```

The defaults match the sample hostnames:

- primary secret: `sidb-primary-tcps-tls`
- True Cache secret: `sidb-standby-tcps-tls`
- primary DNS SANs: `orcl-production.internal.example.com`, `orcl-production`, `orcl-production.default.svc.cluster.local`
- True Cache DNS SANs: `truecache-production.internal.example.com`, `truecache-production`, `truecache-production.default.svc.cluster.local`

Wire those secrets into the SIDB manifests:

- primary manifest uses `spec.security.tcps.tlsSecret: sidb-primary-tcps-tls`
- True Cache manifest uses `spec.security.tcps.tlsSecret: sidb-standby-tcps-tls`

## Suggested Order

1. Install and verify cert-manager on both clusters.
2. Configure ExternalDNS on the primary cluster.
3. Configure ExternalDNS on the True Cache cluster.
4. Run the cert-manager flow to create the TCPS secrets.
5. Apply the primary SIDB manifest and wait for the True Cache blob ConfigMap and `orcl-production-ext`.
6. Apply the True Cache SIDB manifest and wait for `truecache-production-ext`.

## Validation

ExternalDNS and Services:

```sh
kubectl logs -n external-dns deploy/external-dns
kubectl get svc -n default
kubectl get svc -n default orcl-production-ext truecache-production-ext
oci dns record rrset get --zone-name-or-id <zone-ocid> --domain orcl-production.internal.example.com --rtype A --scope PRIVATE
oci dns record rrset get --zone-name-or-id <zone-ocid> --domain truecache-production.internal.example.com --rtype A --scope PRIVATE
```

TCPS secrets:

```sh
./docs/sidb/tcps-cert-manager/11-verify-secrets.sh
```

SIDB:

```sh
kubectl get singleinstancedatabase orcl-production
kubectl get configmap orcl-production-truecache-blob
kubectl get singleinstancedatabase truecache-production
```

Cross-cluster DNS resolution:

```sh
kubectl exec -n default <primary-pod> -- nslookup truecache-production.internal.example.com
kubectl exec -n default <truecache-pod> -- nslookup orcl-production.internal.example.com
```

DNS cleanup during migration or retry:

```sh
bash ./docs/sidb/external-dns/cleanup-dns.sh --check both
bash ./docs/sidb/external-dns/cleanup-dns.sh --apply both
```

If the environment previously used a separate Mumbai private zone, set `LEGACY_ZONE_ID` as well so the same hostnames are removed from both the shared zone and the old local zone.

Customer handoff success criteria:

- `orcl-production-ext` exists and exposes the primary database through an internal load balancer
- `truecache-production-ext` exists and exposes the truecache database through an internal load balancer
- both FQDNs are present in the same shared OCI private zone
- the primary pod resolves `truecache-production.internal.example.com`
- the truecache pod resolves `orcl-production.internal.example.com`
- the primary has created `orcl-production-truecache-blob`
