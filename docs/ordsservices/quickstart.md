# Quick Start: OrdsSrvs with a Reachable Oracle Database

This quick start shows the shortest path to a working `OrdsSrvs` deployment on Kubernetes.

Use this guide when you already have an Oracle Database that is reachable from the cluster and you want to run the ORDS middle tier inside Kubernetes.

This quick start does not install or upgrade ORDS in the database.

By the end of this guide, you will:

1. Create a namespace for `OrdsSrvs`.
2. Create a database credential secret.
3. Apply a minimal `OrdsSrvs` manifest.
4. Verify the resource becomes healthy.
5. Open ORDS locally with a port-forward.

## Example Environment

This guide assumes:

1. Oracle Database Operator is already installed.
2. You have an Oracle Database reachable from the cluster.
3. You want to deploy `OrdsSrvs` into the `ordsnamespace` namespace.
4. The database is reachable with a connect string such as `dbhost:1521/FREE`.
5. The target database is already prepared for ORDS use.

If your environment uses different names, update the commands and YAML accordingly.

## 1. Create the Namespace

Create the namespace used in this example:

```bash
kubectl create namespace ordsnamespace
```

If the operator is running in namespace-scoped mode, also grant it access to this namespace as described in the main [README](../../README.md#choose-deployment-scope).

## 2. Create the Database Credential Secret

Create a secret for the ORDS runtime user password.

```bash
kubectl delete secret -n ordsnamespace ordssrvs-auth --ignore-not-found=true

kubectl create secret generic ordssrvs-auth \
  --from-literal=dbAuth='<ords-db-credential>' \
  -n ordsnamespace
```

In this quick start, `db.username` is set to `ORDS_PUBLIC_USER`.

## 3. Create the OrdsSrvs Resource

Save the following manifest as `ords-quickstart.yaml` and replace `<db-host>`, `<port>`, and `<service-name>` with values from your environment:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ords-quickstart
  namespace: ordsnamespace
spec:
  image: container-registry.oracle.com/database/ords:<ords-version>
  globalSettings:
    database.api.enabled: true
  poolSettings:
    - poolName: default
      restEnabledSql.active: true
      plsql.gateway.mode: direct
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//<db-host>:<port>/<service-name>
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: ordssrvs-auth
        passwordKey: dbAuth
```

Apply it:

```bash
kubectl apply -f ords-quickstart.yaml
```

## 4. Verify the Deployment

Watch the `OrdsSrvs` resource until the status is `Healthy`:

```bash
kubectl get ordssrvs ords-quickstart -n ordsnamespace -w
```

If this is the first time pulling the ORDS image, it may take several minutes.

To inspect the pod logs while the service starts:

```bash
POD_NAME=$(kubectl get pod \
  -l app.kubernetes.io/instance=ords-quickstart \
  -n ordsnamespace \
  -o custom-columns=NAME:.metadata.name \
  --no-headers)

kubectl logs "${POD_NAME}" -n ordsnamespace -f
```

You can also confirm the generated workload and service:

```bash
kubectl get pods,svc -n ordsnamespace -l app.kubernetes.io/instance=ords-quickstart
kubectl describe ordssrvs ords-quickstart -n ordsnamespace
```

## 5. Access ORDS

Open a local port-forward to the ORDS service:

```bash
kubectl port-forward service/ords-quickstart -n ordsnamespace 8080:8080
```

Then open:

```text
http://localhost:8080/ords
```

For production deployments, expose ORDS through HTTPS by configuring TLS at the ingress, load balancer, or ORDS standalone HTTPS layer.

## What This Quick Start Configures

This quick start uses a single database pool named `default` and enables:

1. Direct JDBC connectivity with `db.connectionType: customurl`
2. Basic ORDS features such as Database API and REST Enabled SQL
