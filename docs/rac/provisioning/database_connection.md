# Database Connectivity

To connect to the Oracle RAC Database deployed using Oracle RAC Controller in a Kubernetes Cluster, follow the example that matches your deployment method: NodePort Service or another supported method.

## Database Connection to Oracle RAC Database with NodePort Service
The Oracle RAC Database with NodePort service deployed by Oracle RAC Controller can be reached using the Worker Node IP and the Port of the Node Port service.

Follow these steps:

1. Get the Details of the deployment:
```sh
$ kubectl get all -n rac -o wide
NAME             READY   STATUS    RESTARTS   AGE   IP            NODE            NOMINATED NODE   READINESS GATES
pod/racnode1-0   1/1     Running   0          48m   10.244.1.4    qck-ocne19-w1   <none>           <none>
pod/racnode2-0   1/1     Running   0          48m   10.244.2.87   qck-ocne19-w2   <none>           <none>

NAME                        TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)           AGE   SELECTOR
service/racnode-scan        ClusterIP   None             <none>        <none>            48m   cluster=racnode-scan
service/racnode-scan-lsnr   NodePort    10.99.107.30     <none>        1521:31521/TCP    48m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0          ClusterIP   None             <none>        <none>            48m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-lsnr     NodePort    10.108.117.220   <none>        31522:31522/TCP   48m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-ons      NodePort    10.102.250.240   <none>        6200:30200/TCP    48m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-vip      ClusterIP   None             <none>        <none>            48m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode2-0          ClusterIP   None             <none>        <none>            48m   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-lsnr     NodePort    10.106.33.55     <none>        31523:31523/TCP   48m   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-ons      NodePort    10.104.243.56    <none>        6200:30201/TCP    48m   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-vip      ClusterIP   None             <none>        <none>            48m   statefulset.kubernetes.io/pod-name=racnode2-0

NAME                        READY   AGE   CONTAINERS   IMAGES
statefulset.apps/racnode1   1/1     48m   racnode1-0   phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim
statefulset.apps/racnode2   1/1     48m   racnode2-0   phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim
```
In this case, the port 1521 for Scan Listener from the pod is mapped to port 31521 on the worker node. To make the connection from outside, you must open the port `31521` on the worker node for INGRESS. 

Similarly, if you want to make connection using the port `31522` on worker node `qck-ocne19-w1` or `31523` on worker node `qck-ocne19-w2`, then you will need to allow the ingress for these ports on these worker nodes.
 
With this NodePort Service deployment, you can make a SQL*Plus database connection to this Oracle RAC Database from a remote client.

### Connect from remote SQLPLUS client using Node Public IP and SCAN Port 31521

- Get the Public IPs of the worker nodes on which the RAC Pods are running.
- On the remote client machine `/etc/hosts`file, add the RAC Node hostname mapping to the worker node IPs:
  ```sh
  129.XXX.XX.XX  racnode1-0.rac.svc.cluster.local
  144.XXX.XX.XX  racnode2-0.rac.svc.cluster.local
  129.XXX.XX.XX  racnode-scan
  144.XXX.XX.XX  racnode-scan
  ```
- Connect using any Worker Node IP and SCAN Port `31521` as below:

  ```sh
  $ sqlplus system/<Password>@//129.XXX.XX.XX:31521/soepdb

  OR 

  $ sqlplus system/<Password>@//144.XXX.XX.XX:31521/soepdb
  ```
  
  For Example:
  ```sh
  $ sqlplus system/<Password>@//129.XXX.XX.XX:31521/soepdb

  SQL*Plus: Release 19.0.0.0.0 - Production on Thu Dec 25 01:24:50 2025
  Version 19.8.0.0.0

  Copyright (c) 1982, 2020, Oracle.  All rights reserved.

  Last Successful login time: Thu Dec 25 2025 01:23:40 -05:00

  Connected to:
  Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production
  Version 19.28.0.0.0

  SQL>

  SQL> set lines 200
  SQL> col HOST_NAME format a40
  SQL> select INSTANCE_NAME,HOST_NAME, DATABASE_TYPE from gv$instance;

  INSTANCE_NAME	 HOST_NAME				  DATABASE_TYPE
  ---------------- ---------------------------------------- ---------------
  PORCLCDB2	 racnode2-0				  RAC
  PORCLCDB1	 racnode1-0				  RAC

  SQL>
  ```

### Connect from remote SQLPLUS client using DB Listener Port 31522/31523

- You can directly connect to the Database Instances using the Node IP and Local Database Listener Port as well. 
- In the current setup, 
  + on RAC node `racnode1-0`, DBLSNR is listening on port `31522`
  + on RAC node `racnode2-0`, DBLSNR is listening on port `31523`

  For Example:
  ```sh
  $ sqlplus system/<Password>@//129.XXX.XX.XX:31522/soepdb

  SQL*Plus: Release 19.0.0.0.0 - Production on Thu Dec 25 01:25:09 2025
  Version 19.8.0.0.0

  Copyright (c) 1982, 2020, Oracle.  All rights reserved.

  Last Successful login time: Thu Dec 25 2025 01:24:56 -05:00

  Connected to:
  Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production
  Version 19.28.0.0.0

  SQL>
  ```

## Database Connection to Oracle RAC Database without NodePort Service

In this case, the Oracle RAC Database will not be reachable using the Public IP of the worker node and thus, will not be reachable from outside the Kuberenetes Cluster.

In this case, an application deployed with in the Kubernetes Cluster will be able to reach the Oracle RAC Database on Port 1521.