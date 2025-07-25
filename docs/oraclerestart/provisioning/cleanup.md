# Cleanup an Oracle Restart Database Deployed using Oracle Database Operator

In order to delete and cleanup the Oracle Restart Database deployed using Oracle Database Operator, run below command.

This example uses `oraclerestart_prov.yaml` to cleanup an Oracle Restart Database which was initially used for deployment:


1. Delete the `oraclerestart_prov.yaml` file:
    ```sh
    kubectl delete -f oraclerestart_prov.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample deleted
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n orestart
    ```