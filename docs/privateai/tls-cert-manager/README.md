# PrivateAI TLS with cert-manager

Use this guide to generate a Kubernetes TLS secret for:

- `PrivateAi.spec.security.tls.secretName`

The helper script [tr_cert_manager.sh](./tr_cert_manager.sh) creates or updates a cert-manager `Certificate` and waits for the backing TLS secret.

## Prerequisites

- `cert-manager` is installed in the cluster.
- The target namespace already exists.
- The referenced `Issuer` or `ClusterIssuer` already exists.
- You know the DNS names or IPs clients will use.

## Files

- [tr_cert_manager.sh](./tr_cert_manager.sh) creates or updates the `Certificate` resource

## Choose the target

Specify the target resource for which the TLS secret will be created.

- `TARGET=pai` creates a TLS secret for the `PrivateAi` resource

Default names:

```bash
TARGET=pai   -> CERT_NAME=privateai-cert  and  SECRET_NAME=privateai-tls
```

You can override both names with environment variables.

If you use older provisioning manifests that still reference `paisecret`, set `SECRET_NAME=paisecret` when `TARGET=pai`, or update the manifest to the newer TLS secret name.

## Quick Start

### Create a Bootstrap Self-Signed Issuer

Before issuing TLS certificates for PrivateAI, create a bootstrap  `ClusterIssuer`. This issuer is used by cert-manager to generate an initial self-signed certificate, which can then be used to create a Certificate Authority (CA) and issue application certificates. Create a YAML file `bootstrap-issuer.yaml` as below and apply the manifest:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: tcps-selfsigned-bootstrap
spec:
  selfSigned: {}
```

```sh
kubectl apply -f bootstrap-issuer.yaml
```

You can check the details as below:

```sh
kubectl get clusterissuer tcps-selfsigned-bootstrap
kubectl describe clusterissuer tcps-selfsigned-bootstrap
```

### Create certificate

Create a certificate for `PrivateAi` Deployment.

1. Pick `TARGET=pai`.
2. Set `COMMON_NAME` plus every DNS name or IP that clients will actually use.
3. Run the script to create or update the `Certificate`.
4. Wait for `READY=True`.
5. Reference the generated secret from the matching CR.

**NOTE:** If no `IP` is available (For Example: In case of proviosioning using a Load Balancer where the IP is available only when the Load Balancer is provisioned), you can keep the IP field as blank `""`.

```sh
cd oracle-database-operator/docs/privateai/tls-cert-manager

NAMESPACE=pai \
TARGET=pai \
ISSUER_NAME=tcps-selfsigned-bootstrap \
ISSUER_KIND=ClusterIssuer \
COMMON_NAME=api.example.com \
DNS_NAMES="api.example.com,pai-sample.pai.svc,pai-sample.pai.svc.cluster.local" \
IP_ADDRESSES="<IP_ADRESS>" \
GENERATE_PKCS12=true \
PASSWORD_SECRET_NAME=paisecret \
PASSWORD_SECRET_KEY=privateai-ssl-pwd \
./tr_cert_manager.sh
```

Verify the result:

```bash
kubectl get certificate -n pai
kubectl describe certificate privateai-cert -n pai
kubectl get secret privateai-tls -n pai
kubectl get secret privateai-tls -n pai -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -subject -issuer -dates -ext subjectAltName
```

Use the generated TLS secret in the `PrivateAi` deployment as below:

```yaml
spec:
  security:
    tls:
      secretName: privateai-tls
```

## Renewal and Expiry

The script creates a cert-manager `Certificate` with these defaults:

- `duration=2160h` which is 90 days
- `renewBefore=360h` which is 15 days before expiry
- `rotationPolicy=Always`

It means cert-manager will renew the certificate before `Not After` is reached and will update the same Kubernetes secret automatically.

Check the current validity window:

```bash
kubectl get secret privateai-tls -n pai -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates
```

Force a renewal when needed:

```bash
cmctl renew privateai-cert -n pai
```

**NOTE:** `cmctl` is the cert-manager command-line utility used to manage and troubleshoot cert-manager resources. Please check [cert-manager/cmctl](https://cert-manager.io/docs/reference/cmctl/) for more details.

If you change SANs, issuer, duration, or secret naming, rerun the script with the new values. Cert-manager will reconcile the `Certificate` and update the secret.

## Updating the Secret

For the cert-manager flow, do not patch `tls.crt` and `tls.key` manually unless you are intentionally switching away from cert-manager control. Update the `Certificate` spec instead and let cert-manager rewrite the secret.

When the TLS secret changes:

- `PrivateAi` observes the secret resource version and rolls the deployment

If you must replace the secret manually, keep the same secret name so the CR spec does not need to change:

```bash
kubectl create secret tls privateai-tls \
--cert=/path/to/tls.crt \
--key=/path/to/tls.key \
-n pai \
--dry-run=client -o yaml | kubectl apply -f -
```

## Best Practices

- If the load balancer IP or DNS name is missing from SANs, clients will still see hostname mismatch errors.
- `COMMON_NAME` alone is not enough for modern TLS validation. Put every DNS name and IP in `DNS_NAMES` or `IP_ADDRESSES`.
- Create the secret for the correct consumer: `PrivateAi` TLS and nginx `TrafficManager` TLS are separate secrets.
- If you reuse a secret name with a different purpose, make sure the CR points to the intended secret.
- `TrafficManager` frontend TLS is optional. Only create the nginx certificate when `spec.security.tls.enabled=true`.
- Changing a certificate does not help if clients still connect through a different hostname or IP than the one in SANs.

## Legacy Manual Flow

Older guides in this directory still reference [pai_secret_update_files.sh](../provisioning/pai_secret_update_files.sh) and manual secret patching. Keep using that only if you are not using cert-manager.

For new deployments, prefer this cert-manager flow because renewal and secret updates stay attached to the same `Certificate` resource.
