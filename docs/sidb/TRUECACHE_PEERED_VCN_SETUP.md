# True Cache Peered VCN Setup

This document covers the missing infrastructure pieces around the SIDB True Cache samples when the primary and True Cache databases run in different OKE clusters or regions and communicate over peered VCNs with private DNS and TCPS.

It is based on the working `dns-working-files*` and `tcps-cert-manager` flows, but converted into reusable repo-native samples and scripts.

## Components

The complete setup has four moving parts:

1. Primary SIDB manifest with True Cache blob generation and TCPS enabled.
2. Internal NLB Services for both the primary and the True Cache database.
3. ExternalDNS on each cluster so those NLB Services publish private OCI DNS records.
4. cert-manager flow to issue the primary and True Cache TCPS certificates.

## SIDB and Service Samples

Primary side:

- [`config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml)
- [`config/samples/sidb/singleinstancedatabase_primary_external_service.yaml`](../../config/samples/sidb/singleinstancedatabase_primary_external_service.yaml)

True Cache side:

- [`config/samples/sidb/singleinstancedatabase_truecache_external.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml)
- [`config/samples/sidb/singleinstancedatabase_truecache_external_service.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_external_service.yaml)

Use the two Service resources as the private NLB pair:

- Primary NLB publishes `orcl-production.internal.example.com`
- True Cache NLB publishes `truecache-production.internal.example.com`

## ExternalDNS Setup

Use the manifests in [`docs/sidb/external-dns`](./external-dns).

Apply the same manifest set on each cluster, but customize the OCI region, DNS compartment, `txt-owner-id`, and zone ID for that cluster before applying:

```sh
kubectl apply -f docs/sidb/external-dns/01-namespace-serviceaccount.yaml
kubectl apply -f docs/sidb/external-dns/02-rbac.yaml
kubectl apply -f docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml
kubectl apply -f docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml
```

Then apply the appropriate internal NLB Service on each cluster:

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_primary_external_service.yaml
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_external_service.yaml
```

Important notes:

- Use a distinct `txt-owner-id` per cluster.
- Keep `txt-owner-id` stable for the life of a cluster.
- Prefer workload-identity-scoped IAM policies tied to the `external-dns` service account and cluster OCID.
- If DNS forwarding or zone association is not ready yet, `hostAliases` in the SIDB manifests can bridge name resolution temporarily.

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
4. Apply the primary NLB Service and True Cache NLB Service.
5. Run the cert-manager flow to create the TCPS secrets.
6. Apply the primary SIDB manifest and wait for the True Cache blob ConfigMap.
7. Apply the True Cache SIDB manifest.

## Verification

DNS:

```sh
kubectl logs -n external-dns deploy/external-dns
kubectl get svc -A
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
