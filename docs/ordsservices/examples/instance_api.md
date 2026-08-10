# OrdsSrvs Controller: Instance API

This example shows how to enable the ORDS Instance API for `OrdsSrvs` and bootstrap the Instance API administrator credential from a Kubernetes Secret.

Before testing this example, please verify the prerequisites: [OrdsSrvs prerequisites](../README.md#prerequisites)

## Create the Instance API Secret

Create a Secret for the Instance API administrator credential:

```bash
kubectl create secret generic testcase-iapi \
  --from-literal=iapiAuth='<instance-api-admin-credential>' \
  -n NAMESPACE
```

## Example Manifest

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
        secretName: ordssrvs-auth
        passwordKey: dbAuth
```

Apply the manifest:

```bash
kubectl apply -f instance_api.yaml
kubectl get ordssrvs ordssrvs-base -n NAMESPACE -w
```

## Test the Instance API

Example status URL:

```text
https://ordssrvs-base:8443/ords/_/instance-api/stable/status
```

For example, you can test the Instance API with:

```bash
curl -sS -f -k -u iapi_user -H 'Accept: application/json' -H "Host: localhost" https://ordssrvs-base:8443/ords/_/instance-api/stable/status -w '\n'
```

## Conclusion

This example enables the ORDS Instance API with:

* `instance.api.enabled: true` to turn on the Instance API
* `instanceAPIAdminUser: iapi_user` to define the bootstrap administrator user
* `instanceAPIAdminSecret.secretName: testcase-iapi` and `instanceAPIAdminSecret.passwordKey: iapiAuth` to read the bootstrap credential from a Kubernetes Secret
* `db.secret.passwordKey: dbAuth` to read the ORDS database user credential from `ordssrvs-auth`
