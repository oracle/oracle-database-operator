# Known Issues

Below are the known issues for the current version of the Oracle RAC Controller:

## Issue 1: During Scale In, the Pod was deleted from Kubernetes Cluster successfully but the CRS Cluster was still showing 

With a three node Oracle RAC Database setup with two worker nodes `ocne17-worker1 and ocne17-worker2` with the latter having two pods running, when the Scale In was attempted to remove Oracle RAC Node `racnode2-0` from the cluster, we noticed that while the `pod/racnode2-0` got removed from the Kubernetes Cluster successfully, at the CRS level and at the Database level, the resources from RAC node `racnode1-0` were showing as `OFFLINE` instead of been removed.


```sh
[root@ocne17-oper yamls]# kubectl get all -n rac -o wide
NAME             READY   STATUS    RESTARTS   AGE     IP            NODE             NOMINATED NODE   READINESS GATES
pod/racnode1-0   1/1     Running   0          3h29m   10.244.1.7    ocne17-worker1   <none>           <none>
pod/racnode3-0   0/1     Running   0          3h2m    10.244.3.92   ocne17-worker2   <none>           <none>

NAME                     TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE     SELECTOR
service/racnode-scan     ClusterIP   None         <none>        <none>    3h29m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0       ClusterIP   None         <none>        <none>    3h29m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-vip   ClusterIP   None         <none>        <none>    3h29m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode3-0       ClusterIP   None         <none>        <none>    3h2m    statefulset.kubernetes.io/pod-name=racnode3-0
service/racnode3-0-vip   ClusterIP   None         <none>        <none>    3h2m    statefulset.kubernetes.io/pod-name=racnode3-0

NAME                        READY   AGE     CONTAINERS   IMAGES
statefulset.apps/racnode1   1/1     3h29m   racnode1     phx.ocir.io/intsanjaysingh/db-repo/oracle/database:23.3.0-rac-slim
statefulset.apps/racnode3   0/1     3h2m    racnode3     phx.ocir.io/intsanjaysingh/db-repo/oracle/database:23.3.0-rac-slim


[grid@racnode1-0 trace]$ crsctl stat res -t
--------------------------------------------------------------------------------
Name           Target  State        Server                   State details
--------------------------------------------------------------------------------
Local Resources
--------------------------------------------------------------------------------
ora.LISTENER.lsnr
               ONLINE  ONLINE       racnode1-0               STABLE
               ONLINE  ONLINE       racnode3-0               STABLE
ora.chad
               ONLINE  ONLINE       racnode1-0               STABLE
               ONLINE  ONLINE       racnode3-0               STABLE
ora.net1.network
               ONLINE  ONLINE       racnode1-0               STABLE
               ONLINE  ONLINE       racnode3-0               STABLE
ora.ons
               ONLINE  ONLINE       racnode1-0               STABLE
               ONLINE  ONLINE       racnode3-0               STABLE
--------------------------------------------------------------------------------
Cluster Resources
--------------------------------------------------------------------------------
ora.ASMNET1LSNR_ASM.lsnr(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               STABLE
ora.ASMNET2LSNR_ASM.lsnr(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               STABLE
ora.DATA.dg(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               STABLE
ora.LISTENER_SCAN1.lsnr
      1        ONLINE  ONLINE       racnode1-0               STABLE
ora.LISTENER_SCAN2.lsnr
      1        ONLINE  OFFLINE                               STABLE
ora.asm(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               Started,STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               Started,STABLE
ora.asmnet1.asmnetwork(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               STABLE
ora.asmnet2.asmnetwork(ora.asmgroup)
      1        ONLINE  ONLINE       racnode1-0               STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               STABLE
ora.cdp1.cdp
      1        ONLINE  ONLINE       racnode1-0               STABLE
ora.cdp2.cdp
      1        OFFLINE OFFLINE                               STABLE
ora.cvu
      1        ONLINE  ONLINE       racnode1-0               STABLE
ora.porclcdb.db
      1        ONLINE  ONLINE       racnode1-0               Open,HOME=/u01/app/o
                                                             racle/product/23c/db
                                                             home_1,STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               Open,HOME=/u01/app/o
                                                             racle/product/23c/db
                                                             home_1,STABLE
ora.porclcdb.orclpdb.pdb
      1        ONLINE  ONLINE       racnode1-0               READ WRITE,STABLE
      2        ONLINE  OFFLINE                               STABLE
      3        ONLINE  ONLINE       racnode3-0               READ WRITE,STABLE
ora.racnode1-0.vip
      1        ONLINE  ONLINE       racnode1-0               STABLE
ora.racnode2-0.vip
      1        ONLINE  OFFLINE                               STABLE
ora.racnode3-0.vip
      1        ONLINE  ONLINE       racnode3-0               STABLE
ora.scan1.vip
      1        ONLINE  ONLINE       racnode1-0               STABLE
ora.scan2.vip
      1        ONLINE  OFFLINE                               STABLE
--------------------------------------------------------------------------------




[oracle@racnode1-0 ~]$ srvctl status database -d PORCLCDB
Instance PORCLCDB1 is running on node racnode1-0
Instance PORCLCDB2 is not running on node racnode2-0
Instance PORCLCDB3 is running on node racnode3-0
[oracle@racnode1-0 ~]$
```


## Issue 2: For the Oracle RAC Database Setup with NodePort service using Oracle RAC Controller, we are not able to reach on the Listener Node Port

While we are not able to reach on the listener node port, the Scan Listener Node Port and ONS Node Port is reachable

Below is an example setup using Node ports:

```sh
[root@ocne17-oper ~]# kubectl get all -n rac -o wide
NAME             READY   STATUS    RESTARTS   AGE   IP            NODE             NOMINATED NODE   READINESS GATES
pod/racnode1-0   1/1     Running   0          25m   10.244.1.8    ocne17-worker1   <none>           <none>
pod/racnode2-0   1/1     Running   0          25m   10.244.3.93   ocne17-worker2   <none>           <none>

NAME                        TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)           AGE   SELECTOR
service/racnode-scan        ClusterIP   None             <none>        <none>            25m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode-scan-lsnr   NodePort    10.103.243.3     <none>        1521:31521/TCP    25m   oralabel=racdbprov-sample
service/racnode1-0          ClusterIP   None             <none>        <none>            25m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-lsnr     NodePort    10.102.124.135   <none>        31522:31522/TCP   47s   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-ons      NodePort    10.101.134.178   <none>        6200:30200/TCP    47s   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode1-0-vip      ClusterIP   None             <none>        <none>            25m   statefulset.kubernetes.io/pod-name=racnode1-0
service/racnode2-0          ClusterIP   None             <none>        <none>            25m   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-lsnr     NodePort    10.108.92.112    <none>        31523:31523/TCP   47s   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-ons      NodePort    10.110.24.81     <none>        6200:30201/TCP    47s   statefulset.kubernetes.io/pod-name=racnode2-0
service/racnode2-0-vip      ClusterIP   None             <none>        <none>            25m   statefulset.kubernetes.io/pod-name=racnode2-0

NAME                        READY   AGE   CONTAINERS   IMAGES
statefulset.apps/racnode1   1/1     25m   racnode1     phx.ocir.io/intsanjaysingh/db-repo/oracle/database:23.3.0-rac-slim
statefulset.apps/racnode2   1/1     25m   racnode2     phx.ocir.io/intsanjaysingh/db-repo/oracle/database:23.3.0-rac-slim
[root@ocne17-oper ~]#


[root@ocne17-oper ~]# kubectl get ep -n rac
NAME                ENDPOINTS                          AGE
racnode-scan        10.244.1.8                         94m
racnode-scan-lsnr   10.244.1.8:1521,10.244.3.93:1521   94m
racnode1-0          10.244.1.8                         94m
racnode1-0-lsnr     10.244.1.8:31522                   57s
racnode1-0-ons      10.244.1.8:6200                    57s
racnode1-0-vip      10.244.1.8                         94m
racnode2-0          10.244.3.93                        94m
racnode2-0-lsnr     10.244.3.93:31523                  57s
racnode2-0-ons      10.244.3.93:6200                   57s
racnode2-0-vip      10.244.3.93                        94m

[root@ocne17-oper yamls]# kubectl get nodes -o wide
NAME             STATUS   ROLES           AGE   VERSION         INTERNAL-IP   EXTERNAL-IP   OS-IMAGE                  KERNEL-VERSION                   CONTAINER-RUNTIME
ocne17-master1   Ready    control-plane   19d   v1.26.6+1.el8   10.0.1.170    <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
ocne17-worker1   Ready    <none>          19d   v1.26.6+1.el8   10.0.1.60     <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
ocne17-worker2   Ready    <none>          19d   v1.26.6+1.el8   10.0.1.124    <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
ocne17-worker3   Ready    <none>          19d   v1.26.6+1.el8   10.0.1.44     <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
ocne17-worker4   Ready    <none>          19d   v1.26.6+1.el8   10.0.1.78     <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
ocne17-worker5   Ready    <none>          19d   v1.26.6+1.el8   10.0.1.9      <none>        Oracle Linux Server 8.8   5.15.0-103.114.4.el8uek.x86_64   cri-o://1.26.3
```

Test to reach the node port for ONS and Listener:
```sh
root@ocne17-oper ~]# telnet 10.0.1.60 31521
Trying 10.0.1.60...
Connected to 10.0.1.60.
Escape character is '^]'.
^C


[root@ocne17-oper ~]# telnet 10.0.1.124 31521
Trying 10.0.1.124...
Connected to 10.0.1.124.
Escape character is '^]'.
^C


[root@ocne17-oper ~]#
[root@ocne17-oper ~]# telnet 10.0.1.60 30200
Trying 10.0.1.60...
Connected to 10.0.1.60.
Escape character is '^]'.
^C

[root@ocne17-oper ~]# telnet 10.0.1.124 30201
Trying 10.0.1.124...
Connected to 10.0.1.124.
Escape character is '^]'.
^C

[root@ocne17-oper ~]#
[root@ocne17-oper ~]# telnet 10.0.1.60 31522
Trying 10.0.1.60...
telnet: connect to address 10.0.1.60: Connection refused
[root@ocne17-oper ~]# telnet 10.0.1.124 31523
Trying 10.0.1.124...
telnet: connect to address 10.0.1.124: Connection refused
[root@ocne17-oper ~]#
[root@ocne17-oper ~]#
```