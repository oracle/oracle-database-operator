# Database Connectivity

Depending on whether the Oracle Restart Database has been deployed using Oracle Restart Controller with a NodePort Service or a Load Balancer Service etc, you can refer to the below examples to connect to the Oracle Restart Database:

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
statefulset.apps/dbmc1   1/1     5h46m   dbmc1        localhost/oracle/database-orestart:19.3.0-slim
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

## Database Connection to Oracle Restart Database with Load Balancer

In this case, the Oracle Restart Database is deployed with an External Load Balancer and the deployment has a Public IP Assigned from the External Load Balancer Service. 

Once the deployment is completed, you will be able to make a database connection as below:

1. Get the Details of the deployment:
```sh
$ kubectl get all -n orestart -o wide
NAME          READY   STATUS    RESTARTS   AGE   IP            NODE         NOMINATED NODE   READINESS GATES
pod/dbmc1-0   1/1     Running   0          14m   10.244.0.41   10.0.10.58   <none>           <none>

NAME              TYPE           CLUSTER-IP     EXTERNAL-IP    PORT(S)                         AGE   SELECTOR
service/dbmc1     LoadBalancer   10.96.34.208   XXX.XX.XX.XX   1521:30433/TCP,6200:30656/TCP   14m   statefulset.kubernetes.io/pod-name=dbmc1-0
service/dbmc1-0   ClusterIP      None           <none>         <none>                          14m   statefulset.kubernetes.io/pod-name=dbmc1-0

NAME                     READY   AGE   CONTAINERS   IMAGES
statefulset.apps/dbmc1   1/1     14m   dbmc1        localhost/oracle/database-orestart:19.3.0-slim
```
In this case, you will be able to make a remote database connection using the Load Balancer target port 1521.
 
2. For the above deployment, you will be able to make an SQLPLUS database connection to this Oracle Restart Database from a remote client as below:
 
```sh
bash-4.4$ sqlplus system/<Database Password>@//<Load Balancer Public IP XXX.XX.XX.XX >:1521/PORCLCDB
 
SQL*Plus: Release 21.0.0.0.0 - Production on Tue Sep 2 04:57:56 2025
Version 21.19.0.0.0

Copyright (c) 1982, 2022, Oracle.  All rights reserved.

Last Successful login time: Tue Sep 02 2025 04:53:52 +00:00

Connected to:
Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production
Version 19.28.0.0.0
 
SQL>
SQL> set lines 200
SQL> col HOST_NAME format a40
SQL> select INSTANCE_NAME,HOST_NAME, DATABASE_TYPE from v$instance;

INSTANCE_NAME	 HOST_NAME				  DATABASE_TYPE
---------------- ---------------------------------------- ---------------
PORCLCDB	 dbmc1-0				  SINGLE
```

## Database Connection to Oracle Restart Database without NodePort Service

In this case, the Oracle Restart Database will NOT be reachable using the Public IP of the worker node and thus, will not be reachable from outside the Kuberenetes Cluster.

In this case, an application deployed with in the Kubernetes Cluster will be able to reach the Oracle Restart Database on Port 1521.