# True Cache Peered VCN Setup

This document covers only the OCI network peering setup between the primary VCN and the True Cache VCN.

Use this when the primary database runs in one OKE cluster or region and the True Cache database runs in another, and both sides must communicate over private IPs.

## Components

The OCI networking part has one goal:

1. Establish private connectivity between the two VCNs so the later SIDB and True Cache setup can use private endpoints.

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

Do not proceed to the Kubernetes setup until:

- both RPCs show `PEERED`
- both DRGs have the expected remote VCN routes
- both subnet route tables point remote CIDRs to the local DRG
- security rules allow traffic between the two VCN CIDRs

After the VCN peering is validated, continue with the Kubernetes-side setup in these docs:

- [`README.md`](./README.md) for the SIDB sample entry points
- [`external-dns/README.md`](./external-dns/README.md) for shared private DNS publication
- [`tcps-cert-manager/README.md`](./tcps-cert-manager/README.md) for TCPS certificate issuance
