# OrdsSrvs Controller: Instance API

This example shows how to enable the ORDS Instance API for `OrdsSrvs` and bootstrap the Instance API administrator password from a Kubernetes Secret.

Before testing this example, please verify the prerequisites: [OrdsSrvs prerequisites](../README.md#prerequisites)

## Create the Instance API Secret

Prompt for the password at the console instead of storing it in a script or manifest. In this example the password is kept in a shell variable only long enough to create the Secret, then it is immediately cleared.

```bash
read -rsp "Enter Instance API admin password: " IAPI_PASSWORD
echo

kubectl create secret generic testcase-iapi \
  --from-literal=password="${IAPI_PASSWORD}" \
  -n NAMESPACE

unset IAPI_PASSWORD
```

## Example Manifest

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-base
  namespace: NAMESPACE
spec:
  image: ORDSIMG

  globalSettings:
    instance.api.enabled: true
    instanceAPIAdminUser: iapi_user
    instanceAPIAdminSecret:
      secretName: testcase-iapi
  
  poolSettings:
    - poolName: default
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//CONNECTSTRING
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: db-secret
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
* `instanceAPIAdminSecret.secretName: testcase-iapi` to read the bootstrap password from a Kubernetes Secret
* no explicit `passwordKey`, because the secret uses the default key `password`
