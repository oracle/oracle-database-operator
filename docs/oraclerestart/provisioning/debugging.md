# Debugging and Troubleshooting

When provisioning the Oracle Restart Database with the Oracle Restart Controller, your debugging approach depends on the stage in which the issue occurs. Use the guidance below for targeted troubleshooting.

Below are the possible cases and the steps to debug such an issue:

## Failure during the provisioning of Kubernetes Pods

If the failure occurs while creating Kubernetes Pods, start by checking the status of the affected Pod:

Use the following command to check the logs of the Pod that has a failure. For example, pod `pod/dbmc1-0`, use the following command:

```sh
kubectl logs -f pod/dbmc1-0 -n orestart
```

If the Pod has failed to provision due to an issue with the Docker Image Pull, then you see the error `Error: ErrImagePull` in the logs.

If the Pod has not yet been initialized, then use the following command to find the reason for it:

```sh
kubectl describe pod/dbmc1-0 -n orestart
```

You will need to further troubleshoot depending on further issues or errors seen after you run `kubectl describe pod`.

## Oracle Database Provisioning Failure

If the Oracle Database provisioning fails after the Kubernetes Pods have been created, but during the processing of database creation scripts, then troubleshoot the issue within the affected Pod.

Initially, check the logs of the Kubernetes Pod using the `kubectl logs` command (exchange the name of the Pod in this example with the actual Pod on your system):

```sh
kubectl logs -f pod/dbmc1-0 -n orestart
```

To check the details of the CRS logs or the RDBMS instance logs at the host level, switch to the corresponding Kubernetes container using the following command:

```sh
kubectl exec -it dbmc1-0 -n orestart -- tail -f /tmp/orod/oracle_db_setup.log
```