# Debugging and Troubleshooting

When the Oracle Restart Database is provisioned using the Oracle Restart controller, the debugging of an issue with the deployment depends on at which stage the issue has been seen.

Below are the possible cases and the steps to debug such an issue:

## Failure during the provisioning of Kubernetes Pods

In case the failure occurs during the provisioning, we need to check the status of the Kubernetes Pod which has failed to deployed.

Use the below command to check the logs of the Pod which has a failure. For example, for failure in case of Pod `pod/dbmc1-0`, use below command:

```sh
kubectl logs -f pod/dbmc1-0 -n orestart
```

In case the Pod has failed to provision due to an issue with the Docker Image Pull, you will see the error `Error: ErrImagePull` in above logs.

If the Pod has not yet got initialized, use the below command to find the reason for it:

```sh
kubectl describe pod/dbmc1-0 -n orestart
```

You will need to further troubleshoot depending upon the issue/error seen from the above step.

## Failure in the provisioning of the Oracle Database

In case the failure occures after the Kubernetes Pods are created but during the execution of the scripts to create the Oracle database, you will need to trobleshoot that at the individual Pod level.

Initially, check the logs of the Kubernetes Pod using the command like below (change the name of the Pod with the actual Pod)

```sh
kubectl logs -f pod/dbmc1-0 -n orestart
```

To check the details of the CRS logs or the RDBMS instance logs at the host level, switch to the corresponding Kubernetes container using the command like below:

```sh
kubectl exec -it dbmc1-0 -n orestart -- tail -f /tmp/orod/oracle_rac_setup.log
```