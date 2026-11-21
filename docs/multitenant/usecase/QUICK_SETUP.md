
# QUICK SETUP EXAMPLE

This is a simple example of multitenant controller setup and utilization based on a container database managed by singinstancedatabse controller.

- In order to generate file for quick setup edit [parameter file](./parameters.txt) by setting the following parameters and then run **make genyaml**

```text
USESIDB:TRUE
PDBNAMESPACE:pdbnamespace
CDBNAMESPACE:cdbnamespace
SIDNAMESPACE:sidbnamespace
PASSPROTECTION:NATIVE
## Oracle container registry credential
OCRU:...your credential...
OCRP:...your credential..

```

- yaml file used by this examples
  - [sidb.yaml](./sidb.yaml)
  - [creale_lrest_pod.yaml](./create_lrest_pod.yaml)
  - [create_pdb1_resource.yaml](./create_pdb1_resource.yaml)

## Configure the operator

```bash
git clone git@github.com:oracle/oracle-database-operator.git
oracle-database-operator/scripts/generate-namespace-install.sh oracle-database-operator-system,cdbnamespace,pdbnamespace,sidbnamespace  --output-dir ./myworkdir

/usr/bin/kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.20.2/cert-manager.yaml

/usr/bin/kubectl wait --for jsonpath='{.webhooks[0].clientConfig.caBundle'} \
 validatingwebhookconfiguration cert-manager-webhook --timeout=3m

/usr/bin/kubectl create namespace cdbnamespace

/usr/bin/kubectl create namespace pdbnamespace

/usr/bin/kubectl create namespace sidbnamespace

/usr/bin/kubectl apply -f oracle-database-operator-rbac.yaml

/usr/bin/kubectl apply -f oracle-database-operator-system.yaml

/usr/bin/kubectl wait --for=condition=Available deployment/oracle-database-operator-controller-manager \
        -n oracle-database-operator-system --timeout=300s

/usr/bin/kubectl rollout restart -n oracle-database-operator-system deployment oracle-database-operator-controller-manager

/usr/bin/kubectl wait --for=condition=Available deployment/oracle-database-operator-controller-manager \
        -n oracle-database-operator-system --timeout=300s

/usr/bin/kubectl get deployment oracle-database-operator-controller-manager -n oracle-database-operator-system \
        -o jsonpath='{.spec.template.spec.containers[*].image}{"\n"}'

/usr/bin/kubectl get pods -n oracle-database-operator-system
```

## Configure secrets

```bash
/usr/bin/kubectl create secret docker-registry oracle-container-registry-secret \
  --docker-server=container-registry.oracle.com \
  --docker-username='myname@mycomapny.com' \
  --docker-password='............' \
  --docker-email='myname@mycompany' -n sidbnamespace 

/usr/bin/kubectl create secret generic db-admin-secret \
  --from-literal=oracle_pwd='iIvrUxDYypLHe' -n sidbnamespace

/usr/bin/kubectl create secret generic appusersecret \
        --from-literal=usr01='db_admin01' \
        --from-literal=pwd01='contmasc' \
        --from-literal=grt01='dba,session,create anyuser' \
        --from-literal=usr02='db_admin02' \
        --from-literal=pwd02='sorperoz' \
        --from-literal=grt02='connect,select catalog role'  \
        --from-literal=usr03='db_admin03' \
        --from-literal=pwd03='archmelan' \
        --from-literal=grt03='connect' -n  pdbnamespace

/usr/bin/kubectl create secret generic dbuser --from-literal=e_dbuser.txt=restdba -n  cdbnamespace

/usr/bin/kubectl create secret generic dbpass --from-literal=e_dbpass.txt=restdbapwd -n  cdbnamespace

/usr/bin/kubectl create secret generic pdbusr --from-literal=e_pdbusr.txt=pdbadmin -n  pdbnamespace

/usr/bin/kubectl create secret generic pdbpwd --from-literal=e_pdbpwd.txt=pdbadminpwd -n  pdbnamespace
```

## Create SingleInstance

```bash

cat <<EOF>/home/oracle/resetdba.sql

alter session set "_oracle_script"=true;

create user ${DBUSER} identified by ${DBPASS};

grant create session to restdba container=all;

grant sysdba to restdba container=all;

exit;
EOF

/usr/bin/kubectl apply -f sidb.yaml

/usr/bin/kubectl wait --for jsonpath='{.status.status'}='Healthy' singleinstancedatabase sidb-sample -n sidbnamespace --timeout=40m

/usr/bin/kubectl wait --for jsonpath='{.status.phase'}='Running'  pod  -l app=sidb-sample -n sidbnamespace

/usr/bin/kubectl get  pod  -l app=sidb-sample -n sidbnamespace

export PODSIDB=$(kubectl get pod -l app=sidb-sample -n sidbnamespace  -o custom-columns=":metadata.name" --no-headers|sed 's/ //g')

/usr/bin/kubectl cp resetdba.sql ${PODSIDB}:/home/oracle  -n sidbnamespace

/usr/bin/kubectl exec  ${PODSIDB} -n sidbnamespace -- sqlplus -s '/as sysdba'  @/home/oracle/resetdba.sql

/usr/bin/rm resetdba.sql

/usr/bin/kubectl exec  ${PODSIDB} -n sidbnamespace -- rm /home/oracle/resetdba.sql

```

## Create LREST

```bash
#cleanup existing installation
/usr/bin/kubectl delete lrest cdb-dev -n cdbnamespace

/usr/bin/kubectl delete replicaset.apps/cdb-dev-lrest-rs -n cdbnamespace

/usr/bin/kubectl wait --for=delete lrest cdb-dev -n cdbnamespace --timeout=10m

/usr/bin/kubectl wait --for=delete pod/* -n cdbnamespace --timeout=10m

/usr/bin/kubectl apply -f create_lrest_pod.yaml 

/usr/bin/kubectl wait --for jsonpath='{.status.phase'}="POD/LREST creation completed" lrest cdb-dev -n cdbnamespace --timeout=3m

/usr/bin/kubectl get lrest -n  cdbnamespace

/usr/bin/kubectl get pod -n  cdbnamespace

while /usr/bin/kubectl get pods -n cdbnamespace --no-headers | grep -q "Terminating"; do echo "Waiting for terminating pod(s) to disappear..."; sleep 5; done

/usr/bin/kubectl logs  `/usr/bin/kubectl get pods -o custom-columns=:metadata.name -n cdbnamespace  --field-selector=status.phase=Running --no-headers |grep lrest ` -n cdbnamespace
```

## Create LRPDB

```bash
/usr/bin/kubectl apply -f create_pdb1_resource.yaml

/usr/bin/kubectl wait --for jsonpath='{.status.msg'}="open:[op. completed]" lrpdb pdb1 -n pdbnamespace --timeout=3m

/usr/bin/kubectl get lrpdb pdb1 -n pdbnamespace 
```

## Close PDB

```bash
/usr/bin/kubectl patch lrpdb pdb1 -n pdbnamespace -p \
        '{"spec":{"pdbState":"CLOSE","modifyOption":"IMMEDIATE"}}' --type=merge

/usr/bin/kubectl wait --for jsonpath='{.status.msg'}="close:[op. completed]" lrpdb pdb1 -n pdbnamespace --timeout=3m

/usr/bin/kubectl get lrpdb pdb1 -n pdbnamespace 
```
