# Deploy Oracle AI Database Private AI Services on OpenShift

This guide deploys Private AI in namespace `pai` with the dedicated
ServiceAccount `pai-sa`. OpenShift selects an SCC through the ServiceAccount.
Use the cluster-approved SCC by default. The example custom SCC is optional and
is only needed when the image or storage requires fixed UID/GID `2001`.

> **Security:** Creating SCCs and granting SCC use require suitable cluster permissions. Do not grant this SCC to the `default` ServiceAccount. Keep the SCC narrowly restricted and review its permissions with the cluster administrator.

## 1. Verify the operator and create the namespace

Use an OpenShift client configured with `oc` access.

```bash
oc get crd privateais.privateai.oracle.com
oc get deployments -n oracle-database-operator-system
oc get namespace pai >/dev/null 2>&1 || oc create namespace pai
```

Confirm that the Oracle AI Database Operator watches `pai` and has the necessary namespace RBAC. The exact operator installation and watch configuration depends on how the operator was installed.

Check the allocated UID and supplemental-group ranges:

```bash
oc get namespace pai -o yaml | grep -E \
  'openshift.io/sa.scc.uid-range|openshift.io/sa.scc.supplemental-groups'
```

## 2. Prepare the required Secrets

Create the following Secrets **in namespace `pai`** using your existing TLS and authentication generation procedures:

- `paisecret`: Private AI authentication Secret, containing the keys expected by the operator and image.
- `privateai-tls`: TLS Secret containing the server certificate, key, and any additional entries required by your Private AI TLS workflow.
- `oracle-container-registry-secret`: image pull Secret for Oracle Container Registry.

Verify their presence:

```bash
oc get secret -n pai \
  paisecret privateai-tls oracle-container-registry-secret
```

Ensure that the TLS certificate SAN covers the hostname or IP address clients will use.

## 3. Create the dedicated Private AI ServiceAccount

Create `pai-sa` **before** deploying Private AI:

```bash
oc get serviceaccount pai-sa -n pai >/dev/null 2>&1 || \
  oc create serviceaccount pai-sa -n pai
oc get serviceaccount pai-sa -n pai
```

## 4. Optional: create a custom SCC for UID/GID 2001

Skip this step when the cluster-approved SCC supports the Private AI image and
storage requirements. Ask the OpenShift administrator to confirm the selected
SCC before creating a custom one.

If fixed UID/GID `2001` is required, create `pai-uid2001-scc.yaml`:

```yaml
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  name: pai-uid2001-scc
priority: 20
allowPrivilegedContainer: false
allowPrivilegeEscalation: false
allowHostDirVolumePlugin: false
allowHostNetwork: false
allowHostPID: false
allowHostIPC: false
allowHostPorts: false
requiredDropCapabilities:
  - ALL
allowedCapabilities: []
defaultAddCapabilities: []
runAsUser:
  type: MustRunAs
  uid: 2001
fsGroup:
  type: MustRunAs
  ranges:
    - min: 2001
      max: 2001
supplementalGroups:
  type: MustRunAs
  ranges:
    - min: 2001
      max: 2001
seLinuxContext:
  type: MustRunAs
seccompProfiles:
  - runtime/default
readOnlyRootFilesystem: false
volumes:
  - configMap
  - secret
  - projected
  - emptyDir
  - persistentVolumeClaim
  - downwardAPI
```

Validate and create the SCC:

```bash
oc apply --dry-run=server --validate=true -f pai-uid2001-scc.yaml
oc apply -f pai-uid2001-scc.yaml
oc get scc pai-uid2001-scc -o yaml
```

## 5. Authorize `pai-sa` to use the custom SCC

Complete this step only if you created `pai-uid2001-scc` in Step 4. If you are
using an existing cluster-approved SCC, follow your platform administrator's
authorization procedure instead.

```bash
oc adm policy add-scc-to-user pai-uid2001-scc -z pai-sa -n pai

oc auth can-i use scc/pai-uid2001-scc \
  --as=system:serviceaccount:pai:pai-sa -n pai
```

Expected authorization result: `yes`. The SCC is cluster-scoped, so `oc auth can-i` may display a namespace-scoping warning.

Confirm that the namespace's `default` ServiceAccount **has not been granted this custom SCC**:

```bash
oc auth can-i use scc/pai-uid2001-scc \
  --as=system:serviceaccount:pai:default -n pai
```

Expected result: `no`

## 6. Create the PrivateAi custom resource

Create `pai-quickstart.yaml`:

```yaml
apiVersion: privateai.oracle.com/v4
kind: PrivateAi
metadata:
  name: pai-quickstart
  namespace: pai
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
  runtime:
    serviceAccountName: pai-sa
    image:
      name: container-registry.oracle.com/database/private-ai:large-cpu-infer-latest
      pullSecret: oracle-container-registry-secret
    replicas: 1
  networking:
    service:
      ports:
        - name: https
          port: 8443
          targetPort: 8443
          protocol: TCP
```

For the optional fixed-identity path, add the following under `runtime`:

```yaml
securityContext:
  fsGroup: 2001
```

The example sets `fsGroup: 2001` for volume ownership. It does not set the
process UID; the custom SCC supplies UID `2001` through its `runAsUser` policy.
Do not add `runAsUser` or `runAsGroup` to the CR unless the selected SCC allows
those explicit values.

Apply the CR:

```bash
oc apply -f pai-quickstart.yaml
oc get privateai pai-quickstart -n pai
oc get deployment pai-quickstart -n pai
```

## 7. Verify the operator-rendered ServiceAccount

The `serviceAccountName` field is part of the PrivateAi resource. The operator
renders and reconciles it on the generated Deployment; do not patch the
Deployment directly.

```bash
oc get deployment pai-quickstart -n pai \
  -o jsonpath='{.spec.template.spec.serviceAccountName}{"\n"}'
```

Expected: `pai-sa`.

If the ServiceAccount changes, update the PrivateAi resource and allow the
operator to roll out the Deployment. The replacement ServiceAccount must be
authorized to use the required SCC:

```bash
oc create serviceaccount pai-sa-new -n pai
# Grant pai-sa-new the same approved SCC as pai-sa before switching.
oc patch privateai pai-quickstart -n pai --type=merge \
  -p '{"spec":{"runtime":{"serviceAccountName":"pai-sa-new"}}}'
oc rollout status deployment/pai-quickstart -n pai --timeout=300s
```

Update the source manifest or GitOps definition after testing the change.

## Reconciliation and configuration changes

Update the `PrivateAi` resource, not the generated Deployment. The operator
reconciles `serviceAccountName`, `securityContext`, image, resources, and node
affinity. Changes that affect the Pod template create a replacement Pod.

Manual changes to the generated Deployment are not durable across reconciliation
or Deployment recreation.

## 8. Verify SCC selection, runtime identity, and readiness

```bash
oc get deployment,replicaset,pods -n pai
oc get pods -n pai -l app.kubernetes.io/name=pai-quickstart \
  -o custom-columns='NAME:.metadata.name,SCC:.metadata.annotations.openshift\.io/scc,SA:.spec.serviceAccountName'
```

Expected for the Private AI Pod:

- SCC: the cluster-approved SCC; for the optional custom path, `pai-uid2001-scc`
- ServiceAccount: `pai-sa`

Identify the Pod and inspect its identity:

```bash
POD=$(oc get pods -n pai \
  -l app.kubernetes.io/name=pai-quickstart \
  -o jsonpath='{.items[0].metadata.name}')

oc get pod "$POD" -n pai \
  -o jsonpath='{.spec.serviceAccountName}{"\n"}'
oc exec -n pai "$POD" -- id
oc logs -n pai "$POD" --tail=100
oc rollout status deployment/pai-quickstart -n pai --timeout=300s
oc get privateai pai-quickstart -n pai
```

## 9. Test the HTTPS endpoint

Extract the CA Certificate:

```bash
oc -n pai get secret privateai-tls -o json | jq -r '.data["ca.crt"]' | base64 -d > ca.crt
```

Copy this file `ca.crt` to the machine where you want to run the API Call to the Model Endpoint.

In one terminal:

```bash
oc port-forward -n pai service/pai-quickstart 8443:8443
```

In another terminal:

- list the models:

```bash
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt
curl --cacert "$CA_FILE" --noproxy '*' \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  "https://127.0.0.1:8443/v1/models"
```

- Make an API Endpoint call:

```bash
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt
curl --cacert "$CA_FILE" --noproxy '*' -v -X POST \
  --header "Content-Type: application/json"  \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  -d '{"model": "clip-vit-base-patch32-txt","input": ["The quick brown fox jumped over the fence.","Another test sentence"]}' https://127.0.0.1:8443/v1/embeddings
```


## 10. Troubleshooting

**No Private AI Pod created:**

```bash
oc get deployment,replicaset,pods -n pai
oc get events -n pai --sort-by=.lastTimestamp | tail -30
oc describe deployment pai-quickstart -n pai
oc get replicaset -n pai -l app.kubernetes.io/name=pai-quickstart
```

Describe the newest ReplicaSet and inspect `FailedCreate` events:

```bash
oc describe replicaset -n pai \
  -l app.kubernetes.io/name=pai-quickstart
```

If an event reports `fsGroup: Invalid value: 2001`, confirm the selected SCC and actual ServiceAccount. If it reports `pai-uid2001-scc: Forbidden: not usable by user or serviceaccount`, verify the grant to `pai-sa` and check whether the Deployment or Pod reverted to `default`.

**Check ServiceAccount drift:**

```bash
oc get deployment pai-quickstart -n pai \
  -o jsonpath='{.spec.template.spec.serviceAccountName}{"\n"}'
oc get pods -n pai -l app.kubernetes.io/name=pai-quickstart \
  -o custom-columns='NAME:.metadata.name,SA:.spec.serviceAccountName,SCC:.metadata.annotations.openshift\.io/scc'
```
