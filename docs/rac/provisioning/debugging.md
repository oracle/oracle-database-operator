# Debugging and Troubleshooting

When the Oracle RAC Database is provisioned using the Oracle RAC controller, the debugging of an issue with the deployment depends on the stage the issue occurs.

The following are possible cases and the steps to debug the issue:

## Failure during the provisioning of Kubernetes Pods

a failure occurs during provisioning, check the status of the Kubernetes pod that failed to deploy.

Use the following command to check the logs of the Pod that has a failure. For example, if `pod/racnode1-0`fails, then run the following command:

```sh
kubectl logs -f pod/racnode1-0 -n rac
```

If the Pod failed to provision due to an issue with the Docker Image Pull, then you will see the error `Error: ErrImagePull` in the logs.

If the Pod has not been initialized, then use the following command to find the cause:

```sh
kubectl describe pod/racnode1-0 -n rac
```

You will need to further troubleshoot depending upon the issue or error that you see from the pod description. 

## Failure to provision the RAC Database

If the failure occurs after the Kubernetes Pods are created, but while the scripts to create the RAC database are running, then you must troubleshoot the issue at the individual Pod level.

Initially, check the logs of the Kubernetes Pod using a command similar to the following (change the name of the Pod with the actual Pod):

```sh
kubectl logs -f pod/racnode1-0 -n rac
```

To check the details of the CRS logs or the RDBMS instance logs at the host level, switch to the corresponding Kubernetes container using the following command: 

```sh
kubectl exec -it racnode1-0 -n rac /bin/bash
```

Troubleshooting the corresponding component using the alert log, trace files, in the same way as you would for a standard Oracle RAC Database deployment. See [Oracle RAC Database Documentation](https://docs.oracle.com/en/database/oracle/oracle-database/21/racad/troubleshooting-oracle-rac.html#GUID-F23CDEC1-ECA1-4963-8D31-2E8F0857EE24) and [Oracle Clusterware](https://docs.oracle.com/en/database/oracle/oracle-database/21/cwadd/troubleshooting-oracle-clusterware.html#GUID-5D0B8A16-31FA-4376-BCFC-DC77F6CEC60A) for detailed troubleshooting guidance.
