# Database Connectivity

The Oracle Database Sharding Topology deployed by Sharding Controller in Oracle Database Operator has an external IP available for each of the containers.

## Below is an example setup with connection details

Check the details of the Sharding Topology provisioned by using the Sharding Controller:

```sh
$ kubectl get all  -n shns
NAME            READY   STATUS    RESTARTS   AGE
pod/catalog-0   1/1     Running   0          10d
pod/gsm1-0      1/1     Running   0          10d
pod/gsm2-0      1/1     Running   0          10d
pod/shard1-0    1/1     Running   0          10d
pod/shard2-0    1/1     Running   0          10d

NAME                   TYPE           CLUSTER-IP      EXTERNAL-IP       PORT(S)                                                       AGE
service/catalog        ClusterIP      None            <none>            1521/TCP,6234/TCP,6123/TCP,8080/TCP                           10d
service/catalog0-svc   LoadBalancer   xx.xx.xx.12     xx.xx.xx.116      1521:30079/TCP,6234:30498/TCP,6123:31764/TCP,8080:31729/TCP   10d
service/gsm1           ClusterIP      None            <none>            1522/TCP,6234/TCP,6123/TCP,8080/TCP                           10d
service/gsm10-svc      LoadBalancer   xx.xx.xx.146    xx.xx.xx.38       1522:31401/TCP,6234:31860/TCP,6123:31383/TCP,8080:31892/TCP   10d
service/gsm2           ClusterIP      None            <none>            1522/TCP,6234/TCP,6123/TCP,8080/TCP                           10d
service/gsm20-svc      LoadBalancer   xx.xx.xx.135    xx.xx.xx.66       1522:30036/TCP,6234:31856/TCP,6123:32095/TCP,8080:32162/TCP   10d
service/shard1         ClusterIP      None            <none>            1521/TCP,6234/TCP,6123/TCP,8080/TCP                           10d
service/shard10-svc    LoadBalancer   xx.xx.xx.44     xx.xx.xx.187      1521:30716/TCP,6234:30246/TCP,6123:32538/TCP,8080:31174/TCP   10d
service/shard2         ClusterIP      None            <none>            1521/TCP,6234/TCP,6123/TCP,8080/TCP                           10d
service/shard20-svc    LoadBalancer   xx.xx.xx.83     xx.xx.xx.197      1521:31399/TCP,6234:32088/TCP,6123:30609/TCP,8080:31978/TCP   10d

NAME                       READY   AGE
statefulset.apps/catalog   1/1     10d
statefulset.apps/gsm1      1/1     10d
statefulset.apps/gsm2      1/1     10d
statefulset.apps/shard1    1/1     10d
statefulset.apps/shard2    1/1     10d
```

After you have the external IP address, you can use the services shown below to make the database connection. Using the preceding example, that file should look as follows:

1. **Direct connection to the CATALOG Database**: Connect to the service `catalogpdb` on catalog container external IP `xx.xx.xx.116` on port `1521`
2. **Direct connection to the shard Database SHARD1**: Connect to the service `shard1pdb` on catalog container external IP `xx.xx.xx.187` on port `1521`
3. **Direct connection to the shard Database SHARD2**: Connect to the service `shard2pdb` on catalog container external IP `xx.xx.xx.197` on port `1521`
4. **Connection to Oracle Globally Distributed Database for DML activity (INSERT/UPDATE/DELETE)**: Connect to the service `oltp_rw_svc.catalog.oradbcloud` either on primary gsm GSM1 container external IP `xx.xx.xx.38` on port `1522` **or** on standby gsm GSM2 container external IP `xx.xx.xx.66` on port `1522`
5. **Connection to the catalog database for DDL activity**: Connect to the service `GDS$CATALOG.oradbcloud` on catalog container external IP `xx.xx.xx.116` on port `1521`
