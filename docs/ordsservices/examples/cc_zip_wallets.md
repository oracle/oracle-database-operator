# OrdsSrvs: Central Configuration with Shared ZipWallets

This configuration allows you to store multiple zip wallets in a single Kubernetes Secret, which can then be referenced by different database pools defined in your Central Configuration Manager.

### 1. Create the Shared Secret

Store multiple wallets (containing TCPS certificates, `tnsnames.ora`, and `sqlnet.ora`) in one Secret.

```bash
kubectl create secret -n NAMESPACE generic zipwallets \
  --from-file=wallet_a.zip \
  --from-file=wallet_b.zip

```

**⚠️WARNING⚠️** When using Kubernetes Secrets ensure secrets are protected at the Kubernetes level by following the [Good practices for Kubernetes Secrets](https://kubernetes.io/docs/concepts/security/secrets-good-practices/) in the official Kubernetes documentation.

### 2. OrdsSrvs Specification

The controller mounts the secret to the **fixed path** `/opt/oracle/sa/zipwallets/`.

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-cc
  namespace: NAMESPACE
spec:
  image: ORDSIMG
  globalSettings:
    zipWalletsSecretName: zipwallets
    central.config.url: http://central-config-svc/central/v1/config
  serviceAccountName: ordssrvs-sa
```

### 3. Central Config Pool Example (`pool.json`)

Reference the specific wallet file from the fixed mount directory. Credentials remain separate in the JSON.

```json
{
  "database": {
    "pool": {
      "name": "pool",
      "settings": {
        "db.wallet.zip.path": "/opt/oracle/sa/zipwallets/wallet_b.zip",
        "db.wallet.zip.service": "TCPS_SERVICE_ALIAS",
      }
    }
  }
}

```
>Note: This example is for demo/testing only. Do not use plaintext passwords or HTTP in production.

