
# Makefile for the dbcs automation creation 

This [makefile](#makefile) helps to speed up the **DBCS** creation. Edit  all the credentials related to your tenancy  in the configmap target section and  update the **NAMESPACE** variable.  Specify the oci pem key that you have created during ocicli configuration **OCIPEM**

```makefile 
[...]
ONAMESPACE=oracle-database-operator-system
NAMESPACE=[MY_NAMESPACE]
OCIPEN=[PATH_TO_OCI_API_KEY_PEM]
[...]
configmap:
        $(KUBECTL) create configmap oci-cred \
        --from-literal=tenancy=[MY_TENANCY_ID]
        --from-literal=user=[MY_USER_ID] \
        --from-literal=fingerprint=[MY_FINGER_PRINT] \
        --from-literal=region=[MY_REGION]  -n $(NAMESPACE)

[...]
```
Specify the admin password and the tde password in adminpass and tdepass

```makefile
adminpass:
        echo "[SPECIFY_PASSWORD_HERE]" > ./admin-password
        $(KUBECTL) create secret generic admin-password --from-file=./admin-password -n $(NAMESPACE)
        $(RM) ./admin-password

tdepass:
        echo "[SPECIFY_PASSWORD_HERE]" > ./tde-password
        $(KUBECTL) create secret generic tde-password --from-file=./tde-password -n $(NAMESPACE)
        $(RM) ./tde-password
```
 
Execute the following targets step1 step2 step3 step4 step5 to setup secrets and certificates.

```bash
make step1 
make step2 
make step3
make step4
make step5
```

Create the file **dbcs_service_with_minimal_parameters.yaml**

```yaml 
apiVersion: database.oracle.com/v1alpha1
kind: DbcsSystem
metadata:
  name: dbcssystem-create
  namespace: [MY_NAMESPACE]
spec:
  ociConfigMap: "oci-cred"
  ociSecret: "oci-privatekey"
  dbSystem:
    availabilityDomain: "OLou:EU-MILAN-1-AD-1"
    compartmentId: "[MY_COMPARTMENT_ID]"
    dbAdminPaswordSecret: "admin-password"
    dbEdition: "ENTERPRISE_EDITION_HIGH_PERFORMANCE"
    dbName: "testdb"
    displayName: "dbsystem_example"
    licenseModel: "BRING_YOUR_OWN_LICENSE"
    dbVersion: "19c"
    dbWorkload: "OLTP"
    hostName: "host_example_1205"
    shape: "VM.Standard2.1"
    domain: "example.com"
    sshPublicKeys:
     - "oci-publickey"
    subnetId: "[MY_SUBNET_ID]"

```

Execute the target make file create **make create** or apply directly the above yaml file **kubectl apply -f dbcs_service_with_minimal_parameters.yaml** to create DBCS . Verify the DBCS creation by executing **kubectl get DbcsSystem -n [MY_NAMESPACE]**

```
kubectl get DbcsSystem -n [MY_NAMESPACE]
NAME                AGE
dbcssystem-create   52m
```
Use the describe command to verify the status and the attributes of the dbcs system created

```bash
kubectl describe DbcsSystem dbcssystem-create -n [...]
```
```text
Name:         dbcssystem-create
Namespace:    pdbnamespace
Labels:       <none>
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"database.oracle.com/v1alpha1","kind":"DbcsSystem","metadata":{"annotations":{},"name":"dbcssystem-create","namespace":"pdbn...}}

API Version:  database.oracle.com/v1alpha1
Kind:         DbcsSystem
Metadata:
  Creation Timestamp:  2024-03-15T14:53:02Z

  Db System:
    Availability Domain:      OLou:EU-MILAN-1-AD-1
    Compartment Id:           [MY_COMPARTMENT_ID] 
    Db Admin Pasword Secret:  admin-password
    Db Edition:               ENTERPRISE_EDITION_HIGH_PERFORMANCE
    Db Name:                  testdb
    Db Version:               19c
    Db Workload:              OLTP
    Display Name:             "dbsystem_example"
    Domain:                   example.com
    Host Name:                host_example_1205
    License Model:            BRING_YOUR_OWN_LICENSE
    Shape:                    VM.Standard2.1
    Ssh Public Keys:
      oci-publickey
    Subnet Id:    [MY_SUBNET_ID] 
  Oci Config Map:  oci-cred
  Oci Secret:      oci-privatekey
Status:
  Availability Domain:        OLou:EU-MILAN-1-AD-1
  Cpu Core Count:             1
```
## makefile

```Makefile
ONAMESPACE=oracle-database-operator-system
NAMESPACE=[MY_NAMESPACE]
OCIPEN=[PATH_TO_OCI_API_KEY_PEM]
KUBECTL=/usr/bin/kubectl
CERTMANAGER=https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
RM=/usr/bin/rm

certmanager:
	$(KUBECTL) apply -f $(CERTMANAGER)

prereq: step1 step2 step3 step4 step5

step1: configmap
step2: ociprivkey
step3: adminpass
step4: tdepass
step5: ocipubkey


configmap:
	$(KUBECTL) create configmap oci-cred \
	--from-literal=tenancy=[MY_TENANCY_ID]
	--from-literal=user=[MY_USER_ID] \
	--from-literal=fingerprint=[MY_FINGER_PRINT] \
	--from-literal=region=[MY_REGION]  -n $(NAMESPACE)

ociprivkey:
	$(KUBECTL) create secret generic oci-privatekey --from-file=privatekey=[PATH_TO_OCI_API_KEY_PEM]  -n $(NAMESPACE)

adminpass:
	echo "WElcome_12##" > ./admin-password
	$(KUBECTL) create secret generic admin-password --from-file=./admin-password -n $(NAMESPACE)
	$(RM) ./admin-password

tdepass:
	echo "WElcome_12##" > ./tde-password
	$(KUBECTL) create secret generic tde-password --from-file=./tde-password -n $(NAMESPACE) 
	$(RM) ./tde-password

ocipubkey:
	#ssh-keygen -N "" -C "DBCS_System"-`date +%Y%m` -P ""
	$(KUBECTL) create secret generic oci-publickey --from-file=publickey=/home/oracle/.ssh/id_rsa.pub  -n $(NAMESPACE)

clean: delprivkey delpubkey deladminpass delconfigmap deltdepass

delconfigmap:
	$(KUBECTL) delete configmap oci-cred	 -n $(NAMESPACE)
delprivkey:
	$(KUBECTL) delete secret  oci-privatekey   -n $(NAMESPACE)
delpubkey:
	$(KUBECTL) delete secret  oci-publickey  -n $(NAMESPACE)
deltdepass:
	$(KUBECTL) delete secret  tde-password -n  $(NAMESPACE)
deladminpass:
	$(KUBECTL) delete secret  admin-password -n  $(NAMESPACE)
checkmap:
	$(KUBECTL) get configmaps oci-cred -o yaml -n $(NAMESPACE) |grep -A 5 -B 2 "^data:"
checkdbcs:
	$(KUBECTL) describe dbcssystems.database.oracle.com dbcssystem-create -n $(NAMESPACE)
getall:
	$(KUBECTL) get all -n  $(NAMESPACE)
getmaps:
	$(KUBECTL) get configmaps oci-cred -n  $(NAMESPACE) -o yaml
descdbcss:
	$(KUBECTL) describe dbcssystems.database.oracle.com dbcssystem-create -n  $(NAMESPACE)
getdbcs:
	$(KUBECTL) get DbcsSystem -n  $(NAMESPACE)
create:
	$(KUBECTL) apply -f dbcs_service_with_minimal_parameters.yaml -n $(NAMESPACE)
xlog1:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(ONAMESPACE)|grep oracle-database-operator-controller|head -1|cut -d ' ' -f 1` -n $(ONAMESPACE)
xlog2:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(ONAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut -d ' ' -f 1` -n $(ONAMESPACE)
xlog3:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(ONAMESPACE)|grep oracle-database-operator-controller|tail -1|cut -d ' ' -f 1` -n $(ONAMESPACE)
```