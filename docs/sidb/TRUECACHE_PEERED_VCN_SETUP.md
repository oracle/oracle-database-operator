# True Cache Peered VCN Setup

This document covers both parts of a cross-region True Cache deployment:

1. OCI network peering between the primary VCN and the True Cache VCN.
2. The Kubernetes-side SIDB, ExternalDNS, and TCPS setup after network reachability exists.

Use this when the primary database runs in one OKE cluster or region and the True Cache database runs in another, and both sides must communicate over private IPs.

## Components

The complete setup has five moving parts:

1. OCI DRG and remote peering setup between the two VCNs.
2. Primary SIDB manifest with True Cache blob generation, TCPS enabled, and an operator-managed internal NLB.
3. True Cache SIDB manifest with an operator-managed internal NLB.
4. ExternalDNS on each cluster so both operator-managed NLB Services publish into the same authoritative OCI private DNS zone.
5. cert-manager flow to issue the primary and True Cache TCPS certificates.

## OCI VCN Peering Setup

At a minimum you need:

1. A DRG in Ashburn for the primary side.
2. The primary VCN attached to that DRG.
3. A DRG in Mumbai for the True Cache side.
4. One remote peering connection on the Ashburn DRG.
5. One remote peering connection on the Mumbai DRG.
6. The two RPCs connected to each other.

Important OCI routing notes:

- Associate the correct DRG route tables with both the VCN attachments and the RPC attachments. This is especially important with enhanced DRG.
- Ensure the DRG import route distributions learn routes from the remote side.
- Add VCN subnet route rules in each region so traffic for the remote VCN CIDR is sent to the local DRG.
- Update Security Lists or NSGs to allow ingress and egress between the two VCN CIDRs on the required ports.

Typical ports for this setup:

- TCP `1521` for SQL*Net
- TCP `2484` for TCPS
- TCP `5500` if you need OEM Express
- Any additional application or admin ports your environment requires

## Sample OCI CLI Flow

Set environment variables first:

```sh
export COMPARTMENT_ID=ocid1.compartment.oc1..replace-me

export ASH_DRG=ocid1.drg.oc1.iad.replace-me
export MUMBAI_DRG=ocid1.drg.oc1.ap-mumbai-1.replace-me

export ASH_VCN=ocid1.vcn.oc1.iad.replace-me
export MUMBAI_VCN=ocid1.vcn.oc1.ap-mumbai-1.replace-me

export ASH_VCN_CIDR="10.0.0.0/16"
export MUMBAI_VCN_CIDR="10.20.0.0/16"
```

Attach each VCN to its local DRG if that attachment does not already exist:

```sh
oci network drg-attachment create \
  --compartment-id "${COMPARTMENT_ID}" \
  --drg-id "${ASH_DRG}" \
  --network-details "{\"id\":\"${ASH_VCN}\",\"type\":\"VCN\"}" \
  --region us-ashburn-1

oci network drg-attachment create \
  --compartment-id "${COMPARTMENT_ID}" \
  --drg-id "${MUMBAI_DRG}" \
  --network-details "{\"id\":\"${MUMBAI_VCN}\",\"type\":\"VCN\"}" \
  --region ap-mumbai-1
```

Create one RPC on each DRG:

```sh
export MUMBAI_RPC=$(oci network remote-peering-connection create \
  --compartment-id "${COMPARTMENT_ID}" \
  --drg-id "${MUMBAI_DRG}" \
  --display-name mumbai-rpc \
  --region ap-mumbai-1 \
  --query 'data.id' --raw-output)

export ASH_RPC=$(oci network remote-peering-connection create \
  --compartment-id "${COMPARTMENT_ID}" \
  --drg-id "${ASH_DRG}" \
  --display-name ashburn-rpc \
  --region us-ashburn-1 \
  --query 'data.id' --raw-output)
```

Connect the RPCs:

```sh
oci network remote-peering-connection connect \
  --remote-peering-connection-id "${MUMBAI_RPC}" \
  --peer-id "${ASH_RPC}" \
  --peer-region-name us-ashburn-1 \
  --region ap-mumbai-1
```

Check peering status:

```sh
oci network remote-peering-connection get \
  --remote-peering-connection-id "${MUMBAI_RPC}" \
  --region ap-mumbai-1 \
  --query 'data."peering-status"'

oci network remote-peering-connection get \
  --remote-peering-connection-id "${ASH_RPC}" \
  --region us-ashburn-1 \
  --query 'data."peering-status"'
```

Fetch DRG route tables and route distributions:

```sh
export MUMBAI_DRG_RT=$(oci network drg-route-table list \
  --drg-id "${MUMBAI_DRG}" \
  --region ap-mumbai-1 \
  --query 'data[0].id' --raw-output)

export ASH_DRG_RT=$(oci network drg-route-table list \
  --drg-id "${ASH_DRG}" \
  --region us-ashburn-1 \
  --query 'data[0].id' --raw-output)

export MUMBAI_RD=$(oci network drg-route-distribution list \
  --drg-id "${MUMBAI_DRG}" \
  --region ap-mumbai-1 \
  --query 'data[0].id' --raw-output)

export ASH_RD=$(oci network drg-route-distribution list \
  --drg-id "${ASH_DRG}" \
  --region us-ashburn-1 \
  --query 'data[0].id' --raw-output)
```

Add subnet route rules so each VCN points the remote CIDR at its local DRG:

```sh
export ASH_SUBNET_RT=ocid1.routetable.oc1.iad.replace-me
export MUMBAI_SUBNET_RT=ocid1.routetable.oc1.ap-mumbai-1.replace-me

oci network route-table route-rule add \
  --route-table-id "${ASH_SUBNET_RT}" \
  --destination "${MUMBAI_VCN_CIDR}" \
  --destination-type CIDR_BLOCK \
  --network-entity-id "${ASH_DRG}" \
  --region us-ashburn-1

oci network route-table route-rule add \
  --route-table-id "${MUMBAI_SUBNET_RT}" \
  --destination "${ASH_VCN_CIDR}" \
  --destination-type CIDR_BLOCK \
  --network-entity-id "${MUMBAI_DRG}" \
  --region ap-mumbai-1
```

Update Security Lists or NSGs to permit traffic between the two VCN CIDRs. Example for a Security List update:

```sh
export ASH_SECURITY_LIST=ocid1.securitylist.oc1.iad.replace-me
export MUMBAI_SECURITY_LIST=ocid1.securitylist.oc1.ap-mumbai-1.replace-me

oci network security-list update \
  --security-list-id "${ASH_SECURITY_LIST}" \
  --egress-security-rules "[{\"destination\":\"${MUMBAI_VCN_CIDR}\",\"destination-type\":\"CIDR_BLOCK\",\"protocol\":\"all\"}]" \
  --region us-ashburn-1

oci network security-list update \
  --security-list-id "${MUMBAI_SECURITY_LIST}" \
  --egress-security-rules "[{\"destination\":\"${ASH_VCN_CIDR}\",\"destination-type\":\"CIDR_BLOCK\",\"protocol\":\"all\"}]" \
  --region ap-mumbai-1
```

Validate the network layer before touching SIDB:

```sh
oci network drg-route-rule list --drg-route-table-id "${ASH_DRG_RT}" --region us-ashburn-1
oci network drg-route-rule list --drg-route-table-id "${MUMBAI_DRG_RT}" --region ap-mumbai-1
```

If you have test instances or nodes in both VCNs, verify private connectivity in both directions:

```sh
ping <remote-private-ip>
```

Do not proceed to the Kubernetes steps until:

- both RPCs show `PEERED`
- both DRGs have the expected remote VCN routes
- both subnet route tables point remote CIDRs to the local DRG
- security rules allow traffic between the two VCN CIDRs

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
kubectl -n default get certificate sidb-primary-tcps sidb-standby-tcps
kubectl -n default describe certificate sidb-primary-tcps
kubectl -n default describe certificate sidb-standby-tcps
kubectl -n default get secret sidb-primary-tcps-tls sidb-standby-tcps-tls
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
