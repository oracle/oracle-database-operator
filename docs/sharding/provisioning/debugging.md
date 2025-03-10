# Debugging and Troubleshooting

When the Oracle Database Sharding Topology is provisioned using the Oracle Database Kubernetes Operator, debugging an issue with the deployment depends on which stage the issue is seen.

The following sections provide possible issue cases, and the steps to debug such an issue:

## Failure during the provisioning of Kubernetes Pods

If the failure occurs during the provisioning, then check the status of the Kubernetes Pod that has failed to be deployed.

To check the logs of the Pod that has a failure, use the command that follows. In this example, we are checking for failure in provisioning Pod `pod/catalog-0`:

```sh
kubectl logs -f pod/catalog-0 -n shns
```

If the Pod has failed to provision due to an issue with the Docker Image, then you will see the error `Error: ErrImagePull` in the logs displayed by the command.

If the Pod has not yet been initialized, then use the following command to find the reason for it:

```sh
kubectl describe pod/catalog-0 -n shns
```

If the failure is related to the Cloud Infrastructure, then troubleshoot the infrastructure using the documentation from the Cloud infrastructure provider.

## Failure in the provisioning of the Oracle Globally Distributed Database

If the failure occures after the Kubernetes Pods are created but during the execution of the scripts to create the shard databases, catalog database or the GSM, then you must troubleshoot that failure at the individual Pod level.

Initially, check the logs of the Kubernetes Pod using the following command (change the name of the Pod in the command with the actual Pod):

```sh
kubectl logs -f pod/catalog-0 -n shns
```

To check the logs at the GSM level, the database level, or at the host level, switch to the corresponding Kubernetes container. For example:

```sh
kubectl exec -it catalog-0 -n shns /bin/bash
```

When you are in the correct Kubernetes container, you can troubleshooting the corresponding component using the alert log, the trace files, and so on, just as you would with a normal Sharding Database Deployment. For more information, see: [Oracle Database Sharding Documentation](https://docs.oracle.com/en/database/oracle/oracle-database/19/shard/sharding-troubleshooting.html#GUID-629262E5-7910-4690-A726-A565C59BA73E)


## Debugging using Database Events 

* You can enable database events as part of the Sharded Database Deployment
* Enable events using `envVars` 
* One example of enabling Database Events is [sharding_provisioning_with_db_events.md](./debugging/sharding_provisioning_with_db_events.md)
