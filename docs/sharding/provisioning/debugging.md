# Debugging and Troubleshooting

When the Oracle Database Sharding Topology is provisioned using the Oracle Database Kubernetes Operator, the debugging of an issue with the deployment depends on at which stage the issue has been seen.

Below are the possible cases and the steps to debug such an issue:

## Failure during the provisioning of Kubernetes Pods

In case the failure occurs during the provisioning, we need to check the status of the Kubernetes Pod which has failed to deployed.

Use the below command to check the logs of the Pod which has a failure. For example, for failure in case of Pod `pod/catalog-0`, use below command:

```sh
kubectl logs -f pod/catalog-0 -n shns
```

In case the Pod has failed to provision due to an issue with the Docker Image, you will see the error `Error: ErrImagePull` in above logs.

If the Pod has not yet got initialized, use the below command to find the reason for it:

```sh
kubectl describe pod/catalog-0 -n shns
```

In case the failure is related to the Cloud Infrastructure, you will need to troubleshooting that using the documentation from the cloud provider.

## Failure in the provisioning of the Sharded Database

In case the failure occures after the Kubernetes Pods are created but during the execution of the scripts to create the shard databases, catalog database or the GSM, you will need to trobleshoot that at the individual Pod level.

Initially, check the logs of the Kubernetes Pod using the command like below (change the name of the Pod with the actual Pod)

```sh
kubectl logs -f pod/catalog-0 -n shns
```

To check the logs at the GSM or at the Database level or at the host level, switch to the corresponding Kubernetes container using the command like below:

```sh
kubectl exec -it catalog-0 -n shns /bin/bash
```

Now, you can troubleshooting the corresponding component using the alert log or the trace files etc just like a normal Sharding Database Deployment. Please refer to [Oracle Database Sharding Documentation](https://docs.oracle.com/en/database/oracle/oracle-database/19/shard/sharding-troubleshooting.html#GUID-629262E5-7910-4690-A726-A565C59BA73E) for this purpose.


## Debugging using Database Events 

* You can enable database events as part of the Sharded Database Deployment
* This can be enabled using the `envVars` 
* One example of enabling Database Events is [sharding_provisioning_with_db_events.md](./debugging/sharding_provisioning_with_db_events.md)