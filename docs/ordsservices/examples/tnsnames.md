# OrdsSrvs Example: custom tnsnames.ora

This example demonstrates how to provide an external tnsnames.ora to the OrdsSrvs controller via a Kubernetes Secret.

## Create a tnsnames.ora Secret
Prepare a folder named "resources" containing a tnsnames.ora file, then create a Kubernetes Secret from it.

```bash
mkdir -p resources
cat > resources/tnsnames.ora <<EOF
TESTCASE =
  (DESCRIPTION =
    (ADDRESS = (PROTOCOL = TCP)(HOST = <cluster-ip>)(PORT = <port>))
    (CONNECT_DATA =
      (SERVICE_NAME = <service-name>)
    )
  )
EOF

kubectl create secret -n <namespace> generic myresources-tns-admin --from-file=./resources/tnsnames.ora
```

Replace \<cluster-ip\>, \<port\>, \<service-name\>, and \<namespace\> with your values.


## Create the OrdsSrvs Resource

Below is a snippet (not a complete manifest) showing how to reference the external tnsnames.ora in a pool of the OrdsSrvs resource.

```yaml
...
- poolName: example-pool
  db.connectionType: tns
  db.tnsAliasName: TESTCASE
  tnsAdminSecret:
    secretName: myresources-tns-admin
  db.username: ORDS_PUBLIC_USER
...  
```

## Notes

- Ensure the db.tnsAliasName matches the alias present in tnsnames.ora (TESTCASE in this example).
- Handle Kubernetes Secrets and wallet files in accordance with your organization’s security and compliance policies.
