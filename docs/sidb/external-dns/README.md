# ExternalDNS for Private OCI DNS

These manifests are a sanitized, reusable version of the `dns-working-files*` setup used for SIDB and True Cache private hostnames.

They configure `external-dns` on an OKE cluster with OCI IAM workload identity so an internal `LoadBalancer` Service annotated with `external-dns.alpha.kubernetes.io/hostname` publishes a private DNS record into an OCI private zone.

Use the same manifest set once per cluster. Before applying it, update:

- `docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml`
  - `auth.region`
  - `compartment`
- `docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml`
  - `--txt-owner-id`
  - `--domain-filter`
  - `--zone-id-filter`

Apply order:

```sh
kubectl apply -f docs/sidb/external-dns/01-namespace-serviceaccount.yaml
kubectl apply -f docs/sidb/external-dns/02-rbac.yaml
kubectl apply -f docs/sidb/external-dns/03-external-dns-secret-workload-identity.yaml
kubectl apply -f docs/sidb/external-dns/04-external-dns-deployment-workload-identity.yaml
```

IAM policy guidance:

- Run OCI IAM policy updates in the tenancy home region.
- Prefer a workload-identity-scoped policy tied to the `external-dns` service account and cluster OCID.
- Keep `txt-owner-id` stable for the life of a cluster.

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

Verification:

```sh
kubectl logs -n external-dns deploy/external-dns
kubectl get svc -A
oci dns record rrset get \
  --zone-name-or-id <private-zone-ocid> \
  --domain <host>.internal.example.com \
  --rtype A \
  --scope PRIVATE
```
