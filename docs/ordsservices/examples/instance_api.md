# OrdsSrvs Controller: Instance API

This example shows how to enable the ORDS Instance API for `OrdsSrvs` and bootstrap the Instance API administrator credential from a Kubernetes Secret.

## Prerequisites

See [OrdsSrvs prerequisites](../README.md#prerequisites).

## Create the Instance API Secret

Create a Secret for the Instance API administrator credential:

```bash
kubectl create secret generic testcase-iapi \
  --from-literal=iapiAuth='<instance-api-admin-credential>' \
  -n NAMESPACE
```

## Example Manifest

Save the following manifest as `instance_api.yaml`:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-base
  namespace: NAMESPACE
spec:
  image: container-registry.oracle.com/database/ords:<ords-version>

  globalSettings:
    instance.api.enabled: true
    instanceAPIAdminUser: iapi_user
    instanceAPIAdminSecret:
      secretName: testcase-iapi
      passwordKey: iapiAuth

  poolSettings:
    - poolName: default
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//CONNECTSTRING
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: <secret-name>
        passwordKey: dbAuth
```

Replace `<secret-name>` with an existing Secret in the resource namespace
containing the ORDS runtime user password under `dbAuth`.

Apply the manifest:

```bash
kubectl apply -f instance_api.yaml
kubectl get ordssrvs ordssrvs-base -n NAMESPACE -w
```

## Test the Instance API

Open a port-forward in a separate terminal:

```bash
kubectl port-forward service/ordssrvs-base 8443:8443 -n NAMESPACE
```

Test the Instance API through that local port. Enter the Instance API
administrator password when prompted:

```bash
curl -sS -f -k -u iapi_user -H 'Accept: application/json' -H "Host: localhost" https://localhost:8443/ords/_/instance-api/stable/status -w '\n'
```

> If `standalone.https.host` is configured, replace the `Host: localhost` header with that hostname.

## Conclusion

This example enables the ORDS Instance API with:

* `instance.api.enabled: true` to turn on the Instance API
* `instanceAPIAdminUser: iapi_user` to define the bootstrap administrator user
* `instanceAPIAdminSecret.secretName: testcase-iapi` and `instanceAPIAdminSecret.passwordKey: iapiAuth` to read the bootstrap credential from a Kubernetes Secret
* `db.secret.passwordKey: dbAuth` to read the ORDS database user credential from the Secret selected by `db.secret.secretName`
