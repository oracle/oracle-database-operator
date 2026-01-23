# Debugging and Troubleshooting

When the Oracle RAC Database is provisioned using the Oracle RAC controller, the debugging of an issue with the deployment depends on at which stage the issue has been seen.

Below are the possible cases and the steps to debug such an issue:

## Failure during the provisioning of Kubernetes Pods

In case the failure occurs during the provisioning, we need to check the status of the Kubernetes Pod which has failed to deployed.

Use the below command to check the logs of the Pod which has a failure. For example, for failure in case of Pod `pod/racnode1-0`, use below command:

```sh
kubectl logs -f pod/racnode1-0 -n rac
```

In case the Pod has failed to provision due to an issue with the Docker Image Pull, you will see the error `Error: ErrImagePull` in above logs.

If the Pod has not yet got initialized, use the below command to find the reason for it:

```sh
kubectl describe pod/racnode1-0 -n rac
```

You will need to further troubleshoot depending upon the issue/error seen from the above step.

## Failure in the provisioning of the RAC Database

In case the failure occures after the Kubernetes Pods are created but during the execution of the scripts to create the RAC database, you will need to trobleshoot that at the individual Pod level.

Initially, check the logs of the Kubernetes Pod using the command like below (change the name of the Pod with the actual Pod)

```sh
kubectl logs -f pod/racnode1-0 -n rac
```

To check the details of the CRS logs or the RDBMS instance logs at the host level, switch to the corresponding Kubernetes container using the command like below:

```sh
kubectl exec -it racnode1-0 -n rac /bin/bash
```

Now, you can troubleshooting the corresponding component using the alert log or the trace files etc just like a normal RAC Database Deployment. Please refer to [Oracle RAC Database Documentation](https://docs.oracle.com/en/database/oracle/oracle-database/21/racad/troubleshooting-oracle-rac.html#GUID-F23CDEC1-ECA1-4963-8D31-2E8F0857EE24) and [Oracle Clusterware](https://docs.oracle.com/en/database/oracle/oracle-database/21/cwadd/troubleshooting-oracle-clusterware.html#GUID-5D0B8A16-31FA-4376-BCFC-DC77F6CEC60A) for this purpose.
