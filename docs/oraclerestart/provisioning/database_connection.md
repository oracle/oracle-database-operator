# Database Connectivity

## Database Connection to Oracle Restart Database
## Database Connection to Oracle Restart Database with NodePort Service
The Oracle Database with NodePort service deployed by Oracle Restart Controller can be reached using the Worker Node IP and the Port of the Node Port service. Use the below steps:

1. Get the Details of the deployment:
```sh
$ kubectl get all -n orestart -o wide
NAME          READY   STATUS    RESTARTS   AGE     IP            NODE         NOMINATED NODE   READINESS GATES
pod/dbmc1-0   1/1     Running   0          5h46m   10.244.0.52   10.0.10.58   <none>           <none>
 
NAME              TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)          AGE     SELECTOR
service/dbmc1     NodePort    10.96.53.210   <none>        1521:30007/TCP   5h46m   statefulset.kubernetes.io/pod-name=dbmc1-0
service/dbmc1-0   ClusterIP   None           <none>        <none>           171m    statefulset.kubernetes.io/pod-name=dbmc1-0
 
NAME                     READY   AGE     CONTAINERS   IMAGES
statefulset.apps/dbmc1   1/1     5h46m   dbmc1        localhost/oracle/database-rac:19.3.0-slim
```
In this case, the port 1521 from the pod is mapped to port 30007 on the worker node. To make the connection from outside, you will need to open the port 30007 on the worker node for INGRESS.
 
2. For the above deployment, you will be able to make an SQLPLUS database connection to this Oracle Restart Database from a remote client as below:
 
```sh
bash-4.4$ sqlplus system/<Database Password>@//<Worker Node Public IP>:30007/PORCLCDB
 
SQL*Plus: Release 23.0.0.0.0 - for Oracle Cloud and Engineered Systems on Sat Jul 19 04:02:48 2025
Version 23.9.0.25.09
 
Copyright (c) 1982, 2025, Oracle.  All rights reserved.
 
Last Successful login time: Sat Jul 19 2025 00:20:14 +00:00
 
Connected to:
Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production
Version 19.28.0.0.0
 
SQL>
SQL> set lines 200
SQL> col HOST_NAME format a40
SQL> select INSTANCE_NAME,HOST_NAME, DATABASE_TYPE from v$instance;
 
INSTANCE_NAME    HOST_NAME                                DATABASE_TYPE
---------------- ---------------------------------------- ---------------
PORCLCDB          dbmc1-0                                 SINGLE
```

